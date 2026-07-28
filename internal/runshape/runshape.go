// Package runshape is the thin reader for .vh-agent-harness/run-shape.yml.
//
// It is deliberately minimal: it parses ONLY the `lifecycle:` block (pointers to
// project-owned shell leaves under scripts/) and validates every entry is a path
// pointer — NEVER inline shell. This is the load-bearing pointer-not-inline rule
// from the run-shape spec §3 ("every lifecycle.* value is a path string; the executable
// semantics live in the referenced leaf, which is project_owned under S2").
//
// The full run-shape schema (runtime/services/env/runners/verbs/proxies) is out
// of scope for the Slice 5 hook-dispatcher proof; only the lifecycle block is read.
package runshape

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// DirName is the project-owned declaration root (.vh-agent-harness/).
	DirName = ".vh-agent-harness"
	// FileName is the run-shape file inside DirName.
	FileName = "run-shape.yml"
)

// LifecycleHook is a fixed lifecycle point name. The dispatcher fires ONLY the
// fixed set (IsKnown). Adding a point requires editing both knownHooks and the
// hooks package's PolicyFor table.
type LifecycleHook string

const (
	HookOnFirstInstall LifecycleHook = "on_first_install"
	HookOnUpdate       LifecycleHook = "on_update"
	HookPreUp          LifecycleHook = "pre_up"
	HookPostUp         LifecycleHook = "post_up"
	HookPreDown        LifecycleHook = "pre_down"
	HookPostDown       LifecycleHook = "post_down"
	HookPreExec        LifecycleHook = "pre_exec"
	HookPostExec       LifecycleHook = "post_exec"
	HookOnUninstall    LifecycleHook = "on_uninstall"
)

// knownHooks is the FIXED lifecycle set the dispatcher may fire. A YAML key
// outside this set is rejected at Load (UnknownLifecycleHookError) — it is never
// silently executed. This is the the run-shape spec §4 table made into code.
var knownHooks = map[LifecycleHook]bool{
	HookOnFirstInstall: true,
	HookOnUpdate:       true,
	HookPreUp:          true,
	HookPostUp:         true,
	HookPreDown:        true,
	HookPostDown:       true,
	HookPreExec:        true,
	HookPostExec:       true,
	HookOnUninstall:    true,
}

// IsKnown reports whether h is one of the fixed lifecycle points.
func IsKnown(h LifecycleHook) bool { return knownHooks[h] }

// KnownHooks returns the fixed lifecycle set in canonical order. Useful for
// error guidance and deterministic iteration.
func KnownHooks() []LifecycleHook {
	return []LifecycleHook{
		HookOnFirstInstall, HookOnUpdate,
		HookPreUp, HookPostUp,
		HookPreDown, HookPostDown,
		HookPreExec, HookPostExec,
		HookOnUninstall,
	}
}

// RunShape is the parsed run-shape, trimmed to what the hook dispatcher + the
// runtime verbs need. A zero/empty RunShape (no file or empty lifecycle) means
// "no hooks" — every dispatch point is a clean no-op.
type RunShape struct {
	// Lifecycle maps a fixed lifecycle point to a project-owned leaf path under
	// scripts/. Absent keys (or "") mean no-op. Only IsKnown keys survive Load.
	Lifecycle map[LifecycleHook]string
	// Runtime carries the declared runtime backend spec when the run-shape has a
	// `runtime:` block. Nil means the block is absent (the runtime verbs then
	// fall back to the legacy manifest). This is the S4 runtime authority
	// (the config-authority model): the runtime verbs read S4 FIRST to resolve the
	// backend, services, and default exec target.
	Runtime *RuntimeSpec
}

// RuntimeSpec is the parsed `runtime:` block — the minimal backend-selection
// surface the runtime verbs consume. It mirrors manifest.Runtime +
// manifest.Project.Slug so the backend selector can treat S4 and the legacy
// manifest uniformly. ComposeFile/DefaultService/ProjectSlug are optional
// (empty => the backend resolves its own default).
type RuntimeSpec struct {
	Backend        string   // host-shell | docker-compose | docker_compose | bare | proxy
	ComposeFile    string   // optional compose file path
	DefaultService string   // optional default exec target
	ProjectSlug    string   // optional project slug (C2 naming)
	ProxyCommand   []string // backend=proxy: argv prefix exec/shell delegate to (e.g. ["./dev.sh","exec"])
}

// runShapeYAML is the on-disk shape. The lifecycle + runtime blocks are decoded
// here; the rest of the schema (services/env/runners/verbs/proxies) is still
// ignored at this layer (the schema validator in internal/schema lints the full
// envelope; this loader carries only what the hook dispatcher + runtime verbs
// consume).
type runShapeYAML struct {
	Lifecycle map[string]string `yaml:"lifecycle"`
	Runtime   *runtimeYAML      `yaml:"runtime"`
}

// runtimeYAML mirrors the `runtime:` block. Only the backend-selection fields
// are decoded; compose_overlays (C5-mech) and other richer fields are left to a
// future fuller reader.
type runtimeYAML struct {
	Backend        string   `yaml:"backend"`
	ComposeFile    string   `yaml:"compose_file"`
	DefaultService string   `yaml:"default_service"`
	ProjectSlug    string   `yaml:"project_slug"`
	ProxyCommand   []string `yaml:"proxy_command"`
}

// Load parses the run-shape file at path into a RunShape, validating every
// lifecycle pointer. It enforces all three load-bearing rules:
//  1. pointer-not-inline: each value is a path under scripts/, never inline shell;
//  2. fixed-points-only: unknown lifecycle keys are rejected (UnknownLifecycleHookError);
//  3. a malformed YAML is rejected (MalformedRunShapeError) rather than ignored.
//
// A syntactically-valid file with an empty/absent lifecycle block yields a
// zero-value RunShape (no hooks), not an error.
func Load(path string) (*RunShape, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read run-shape %s: %w", path, err)
	}
	var ry runShapeYAML
	if err := yaml.Unmarshal(data, &ry); err != nil {
		return nil, &MalformedRunShapeError{Path: path, Err: err}
	}
	rs := &RunShape{Lifecycle: make(map[LifecycleHook]string)}
	if ry.Runtime != nil {
		rs.Runtime = &RuntimeSpec{
			Backend:        ry.Runtime.Backend,
			ComposeFile:    ry.Runtime.ComposeFile,
			DefaultService: ry.Runtime.DefaultService,
			ProjectSlug:    ry.Runtime.ProjectSlug,
			ProxyCommand:   ry.Runtime.ProxyCommand,
		}
	}
	for rawKey, rawVal := range ry.Lifecycle {
		h := LifecycleHook(rawKey)
		if !IsKnown(h) {
			// Unknown lifecycle key: REJECT with a clear signal. It must NEVER be
			// silently executed (a typo like "pre_upp" must not become a live hook).
			return nil, &UnknownLifecycleHookError{Key: rawKey, Path: path}
		}
		if err := validateLeafPointer(rawVal); err != nil {
			return nil, fmt.Errorf("run-shape %s: lifecycle.%s: %w", path, rawKey, err)
		}
		if rawVal != "" {
			rs.Lifecycle[h] = rawVal
		}
	}
	return rs, nil
}

// LoadForRoot loads the run-shape for the project at projectRoot. The file is
// expected at <projectRoot>/.vh-agent-harness/run-shape.yml. If no file exists,
// it returns a zero RunShape (no hooks) and no error — absent run-shape is the
// common case and is a clean no-op, never an error. This preserves Slices 1–4
// behavior: repos with no run-shape see no hook activity at all.
func LoadForRoot(projectRoot string) (*RunShape, error) {
	candidate := filepath.Join(projectRoot, DirName, FileName)
	if _, err := os.Stat(candidate); err != nil {
		// absent => no hooks (no-op), not an error.
		return &RunShape{Lifecycle: make(map[LifecycleHook]string)}, nil
	}
	return Load(candidate)
}

// minModeYAML is the tolerant decoder for the exec-sandbox floor. It decodes
// the exec_sandbox block as a generic map (NOT a struct), so we can require
// min_mode to be present whenever the block is present — closing the key-typo
// hole (a misspelled key like `min_mdoe: strict` leaves min_mode absent, which
// we treat as fail-closed rather than silently no-floor). Other top-level
// blocks (runtime/lifecycle/...) are ignored entirely, so the floor stays
// decoupled from lifecycle validity. The schema validator (internal/schema)
// still lints the enum at doctor time.
type minModeYAML struct {
	ExecSandbox map[string]any `yaml:"exec_sandbox"`
}

// LoadMinMode reads exec_sandbox.min_mode from <projectRoot>/.vh-agent-harness/
// run-shape.yml. It is tolerant of unrelated blocks (decodes ONLY exec_sandbox)
// but FAIL-CLOSED on anything that looks like a deliberate-but-broken floor:
//
//   - file absent                          => ("", nil) — no floor (run uncontained)
//   - exec_sandbox block absent (null/nil) => ("", nil) — no floor
//   - exec_sandbox block present + min_mode is a valid string
//     => (string, nil) — the floor value
//   - exec_sandbox block present but min_mode KEY absent (incl. a misspelled
//     key like `min_mdoe: strict`, where yaml drops the unknown key leaving
//     min_mode unset)                      => ("", error) — FAIL-CLOSED
//   - min_mode present but NOT a string
//     (sequence/map/int/bool)              => ("", error) — FAIL-CLOSED
//   - document-level YAML syntax error     => ("", error) — FAIL-CLOSED
//   - file present but unreadable          => ("", error) — FAIL-CLOSED
//
// The min_mode-absent-when-block-present rule is the key-typo defense: an
// operator who writes an exec_sandbox block INTENDED a floor, so a missing or
// misspelled min_mode key is a mistake, not "no floor" — refuse rather than
// silently dropping to ModeOff. (Unknown FUTURE keys are tolerated as long as
// min_mode is present, preserving forward compatibility.) The schema validator
// (doctor) catches these at health-check time too; this is the runtime
// defense-in-depth. Returns "" when the file is absent.
func LoadMinMode(projectRoot string) (string, error) {
	candidate := filepath.Join(projectRoot, DirName, FileName)
	info, err := os.Stat(candidate)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // absent => no floor
		}
		return "", fmt.Errorf("run-shape %s: stat: %w", candidate, err)
	}
	if info.IsDir() {
		// A directory at the floor path is a malformed-present floor: fail closed
		// rather than silently treating it as absent (which would drop the floor
		// to ModeOff — an open escape under a strict contract).
		return "", fmt.Errorf("run-shape %s: expected a file, found a directory", candidate)
	}
	data, err := os.ReadFile(candidate)
	if err != nil {
		return "", fmt.Errorf("run-shape %s: read: %w", candidate, err)
	}
	var mm minModeYAML
	if err := yaml.Unmarshal(data, &mm); err != nil {
		return "", fmt.Errorf("run-shape %s: decode exec_sandbox: %w", candidate, err)
	}
	if mm.ExecSandbox == nil {
		return "", nil // block absent => no floor
	}
	// Block present: min_mode MUST be present (a misspelled key leaves it
	// absent). This is the key-typo fail-closed boundary.
	v, ok := mm.ExecSandbox["min_mode"]
	if !ok {
		return "", fmt.Errorf("run-shape %s: exec_sandbox block present but min_mode key absent (misspelled key?); a present exec_sandbox block requires min_mode", candidate)
	}
	s, ok := v.(string)
	if !ok {
		// Present-but-wrong-type: fail closed. A typo like `min_mode: [strict]`
		// must NOT silently collapse the floor to off.
		return "", fmt.Errorf("run-shape %s: exec_sandbox.min_mode must be a string, got %T (%v)", candidate, v, v)
	}
	return s, nil
}

// FindMinMode walks upward from startDir through the ENTIRE ancestor chain and
// returns the MAX (most restrictive) exec_sandbox.min_mode floor found at any
// level. This is the floor-root locator: it lets an exec-sandbox invocation
// from ANY subdirectory of a project still discover the project's strict floor
// (closing the cwd-scoped bypass). Returns ("", "", nil) when no run-shape with
// an exec_sandbox block exists between startDir and the filesystem root.
//
// MAX-over-entire-chain (not nearest-wins) is the load-bearing safety property:
// a weakening child floor (e.g. one an agent plants under the RW ./tmp tree with
// `min_mode: off`) CANNOT mask an enclosing parent's strict floor — the parent's
// strict always wins because we take the MAX of ALL ancestors. A child without
// an exec_sandbox block is simply skipped (it does not weaken anything).
//
// FAIL-CLOSED on a malformed-present candidate at ANY level: a directory at the
// floor path, any stat error other than not-exist (e.g. EACCES unreadable), or a
// broken exec_sandbox block (wrong-type min_mode, syntax error, misspelled key)
// returns an error so loadExecSandboxFloor refuses to run uncontained. Only a
// clean IsNotExist continues the upward walk. (FindForRoot has the analogous
// walk for the runtime-verb locator; its stat handling is pre-existing and a
// separate concern — this function is the floor-specific hardening.)
func FindMinMode(startDir string) (projectRoot string, minMode string, err error) {
	// Canonicalize to the PHYSICAL path. os.Getwd() returns a symlink-preserving
	// LOGICAL path (it prefers $PWD), and filepath.Abs does not resolve symlinks
	// — so walking the logical parent chain would let an agent escape the floor
	// by cd-ing into an out-of-tree symlink that targets a nested project dir
	// (FindMinMode would walk the symlink's parents, never the physical repo).
	// EvalSymlinks resolves the whole path to its physical location so the
	// upward walk ascends the REAL project tree. Fail-closed on resolution
	// error: if we cannot establish the physical start, refuse rather than guess.
	abs, err := filepath.Abs(startDir)
	if err != nil {
		return "", "", err
	}
	dir, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", "", fmt.Errorf("run-shape: resolve symlinks in floor-discovery start %q: %w (refusing to run uncontained)", startDir, err)
	}
	bestRank := 0
	bestRaw := ""
	bestRoot := ""
	for {
		candidate := filepath.Join(dir, DirName, FileName)
		info, statErr := os.Stat(candidate)
		switch {
		case statErr == nil && !info.IsDir():
			// proper file at this level → load. LoadMinMode fail-closes on a
			// broken-present exec_sandbox block (wrong-type min_mode, syntax
			// error, misspelled key). A file WITHOUT an exec_sandbox block
			// returns ("", nil) → skip this level and keep walking.
			mm, loadErr := LoadMinMode(dir)
			if loadErr != nil {
				return "", "", loadErr
			}
			if mm != "" {
				rank := floorRank(mm)
				if rank < 0 {
					return "", "", fmt.Errorf("run-shape %s: exec_sandbox.min_mode %q is not a valid floor value (off|best-effort|strict)", candidate, mm)
				}
				// Track the MOST RESTRICTIVE floor across the entire chain.
				// This is the anti-weakening guarantee: a child floor
				// (e.g. planted under ./tmp with min_mode: off) cannot
				// override a stricter enclosing parent.
				if rank > bestRank {
					bestRank = rank
					bestRaw = mm
					bestRoot = dir
				}
			}
			// Continue walking up regardless — a parent might be STRICTER.
		case os.IsNotExist(statErr):
			// clean absent → keep walking up.
		case statErr == nil && info.IsDir():
			// directory at the floor path — malformed-present → fail closed.
			return "", "", fmt.Errorf("run-shape %s: expected a file, found a directory", candidate)
		default:
			// stat error other than not-exist (e.g. EACCES unreadable) → fail
			// closed: a present-but-unreadable floor must not silently drop.
			return "", "", fmt.Errorf("run-shape %s: stat: %w", candidate, statErr)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return bestRoot, bestRaw, nil // reached filesystem root; return MAX floor found
		}
		dir = parent
	}
}

// floorRank maps a raw min_mode string to an integer rank for MAX comparison.
// Higher = more restrictive. Mirrors the off < best-effort < strict ordering in
// internal/execsandbox without importing that package (avoids a cross-package
// dependency). Returns -1 for an invalid value (caller fail-closes).
func floorRank(s string) int {
	switch s {
	case "off":
		return 1
	case "best-effort":
		return 2
	case "strict":
		return 3
	default:
		return -1
	}
}

// FindForRoot walks upward from startDir looking for a run-shape.yml under a
// `.vh-agent-harness/` directory. It returns the resolved project root (the dir
// containing `.vh-agent-harness/`) and the parsed RunShape when found. When no
// run-shape exists between startDir and the filesystem root it returns
// ("", nil, nil). A present-but-unreadable/malformed run-shape is returned as an
// error (mirrors manifest.Find semantics) so callers distinguish "absent" from
// "broken". This is the runtime-verb authority locator: the runtime verbs
// (exec/shell/up/down/logs/ps) call this to resolve S4 before falling back to
// the legacy manifest.
func FindForRoot(startDir string) (projectRoot string, rs *RunShape, err error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", nil, err
	}
	for {
		candidate := filepath.Join(dir, DirName, FileName)
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			parsed, loadErr := Load(candidate)
			if loadErr != nil {
				return dir, nil, loadErr
			}
			return dir, parsed, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil, nil // reached filesystem root
		}
		dir = parent
	}
}

// inlineShellSignals are substrings that unambiguously indicate a value is inline
// shell, not a path pointer. No legitimate scripts/*.sh path contains any of
// these. The check is a fast, explicit, typed rejection; the scripts/ prefix
// check below is the structural backstop.
var inlineShellSignals = []string{";", "|", "&", "`", "$(", ">", "<", "\n", "\t"}

// validateLeafPointer checks that raw is a safe path pointer to a project-owned
// leaf under scripts/. It rejects:
//   - inline shell (any inlineShellSignals substring) -> InlineShellError;
//   - absolute paths -> NotAPathPointerError;
//   - path traversal (any ".." component) -> NotAPathPointerError;
//   - anything that does not resolve under scripts/ -> NotAPathPointerError.
//
// An empty raw value is valid (it means "absent = no-op" at that point).
func validateLeafPointer(raw string) error {
	if raw == "" {
		return nil
	}
	for _, bad := range inlineShellSignals {
		if strings.Contains(raw, bad) {
			return &InlineShellError{Value: raw, Signal: bad}
		}
	}
	clean := filepath.ToSlash(filepath.Clean(raw))
	if filepath.IsAbs(clean) {
		return &NotAPathPointerError{Value: raw, Reason: "absolute paths are not allowed; use a relative path under scripts/"}
	}
	for _, seg := range strings.Split(clean, "/") {
		if seg == ".." {
			return &NotAPathPointerError{Value: raw, Reason: "path traversal (..) is not allowed; the leaf must stay under scripts/"}
		}
	}
	if !strings.HasPrefix(clean, "scripts/") {
		return &NotAPathPointerError{Value: raw, Reason: "hook pointer must resolve under scripts/ (e.g. scripts/migrate-db.sh)"}
	}
	return nil
}

// --- Typed errors (detectable via errors.As) --------------------------------

// InlineShellError is returned when a lifecycle hook value contains a shell
// metacharacter, i.e. it is inline shell rather than a path pointer. This is the
// explicit "no inline shell in the schema" rejection.
type InlineShellError struct {
	Value  string
	Signal string
}

func (e *InlineShellError) Error() string {
	return fmt.Sprintf(
		"run-shape: hook value %q looks like inline shell (contains %q); "+
			"lifecycle hooks must be path pointers to scripts/ leaves, never inline shell",
		e.Value, e.Signal,
	)
}

// NotAPathPointerError is returned when a lifecycle hook value is not inline shell
// but still not a valid path pointer under scripts/ (absolute, traversal, or
// outside scripts/).
type NotAPathPointerError struct {
	Value  string
	Reason string
}

func (e *NotAPathPointerError) Error() string {
	return fmt.Sprintf("run-shape: hook value %q is not a valid path pointer: %s", e.Value, e.Reason)
}

// UnknownLifecycleHookError is returned when the YAML carries a lifecycle key
// outside the fixed set. The key is rejected (not silently executed).
type UnknownLifecycleHookError struct {
	Key  string
	Path string
}

func (e *UnknownLifecycleHookError) Error() string {
	return fmt.Sprintf(
		"run-shape %s: unknown lifecycle hook %q; only the fixed set {%s} is allowed",
		e.Path, e.Key, strings.Join(knownHookList(), ", "),
	)
}

// MalformedRunShapeError wraps a YAML parse failure.
type MalformedRunShapeError struct {
	Path string
	Err  error
}

func (e *MalformedRunShapeError) Error() string {
	return fmt.Sprintf("run-shape %s: malformed YAML: %v", e.Path, e.Err)
}

// knownHookList returns the fixed hook names as strings for error guidance.
func knownHookList() []string {
	ks := KnownHooks()
	out := make([]string, len(ks))
	for i, k := range ks {
		out[i] = string(k)
	}
	return out
}
