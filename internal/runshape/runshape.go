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
