package shell

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// Name is the registered tool name.
const Name = "run_shell"

// Defaults (Config zero value normalizes to these).
const (
	// DefaultMaxCapturedBytes caps in-memory capture PER STREAM
	// (stdout and stderr each), default 64 KiB.
	DefaultMaxCapturedBytes = 64 * 1024
	// DefaultMaxTimeoutMs is the hard ceiling on any per-call timeout
	// (also the Pipeline-level dispatch backstop).
	DefaultMaxTimeoutMs = 600_000
	// DefaultDefaultTimeoutMs applies when args omit timeout_ms.
	DefaultDefaultTimeoutMs = 30_000
	// DefaultShellPath is the invoked shell; resolved via PATH.
	DefaultShellPath = "bash"
)

// parametersSchema is the adapter-facing argument description
// (JSON-schema-ish, same style as the echo/clock dogfood tools).
const parametersSchema = `{"type":"object","properties":{` +
	`"command":{"type":"string","description":"shell command line; executed by bash -c, non-interactive, no profile/rc, no history, no injected shell options"},` +
	`"timeout_ms":{"type":"integer","description":"execution deadline in milliseconds; default 30000, clamped to the configured cap (default 600000). On expiry the whole process group is killed"},` +
	`"workdir":{"type":"string","description":"optional working directory; must be an existing directory. Confinement: relative paths must stay inside the engine working directory; absolute paths are rejected unless the engine configured them under a workdir root"}}` +
	`,"required":["command"],"additionalProperties":false}`

// Config configures one run_shell Definition. The zero value is the
// production default (see the Default* constants); normalize() fills
// unset fields.
type Config struct {
	// EnvAllowlist names extra PARENT variables to pass through to the
	// child (the base env is always built explicitly — see buildEnv).
	// Allowlisted names are STILL subject to the sensitive-name scrub
	// and the engine-credential denylist: the scrub wins.
	EnvAllowlist []string

	// MaxCapturedBytes caps captured output per stream; <=0 ⇒
	// DefaultMaxCapturedBytes (64 KiB). Oversize output is truncated
	// with an in-band marker. This is the CAPTURE limit only — the
	// serialized result is additionally spill-eligible at commit time
	// via the session SpillPolicy (see internal/session/spill.go).
	MaxCapturedBytes int64

	// MaxTimeoutMs is the hard per-call ceiling; <=0 ⇒ 600000. Also
	// installed as the ToolDefinition's pipeline-level backstop.
	MaxTimeoutMs int64

	// DefaultTimeoutMs applies when args omit timeout_ms; <=0 ⇒ 30000.
	DefaultTimeoutMs int64

	// AllowedCommands, when non-empty, restricts commands to entries
	// that match (exact or whole-word-prefix). Default empty = allow.
	// DeniedCommands always denies its matches, regardless of the
	// allowlist. Both are COARSE in-tool hygiene — the Pipeline guards
	// are the policy layer and produce typed denial provenance; a
	// Config-list rejection here surfaces as an ordinary isError
	// result.
	AllowedCommands []string
	DeniedCommands  []string

	// WorkdirRoots confines the optional workdir argument (client/
	// model-controlled). POLICY (conservative default, same class as
	// the session-path confinement):
	//
	//   - empty workdir  — allowed (the engine's own working
	//     directory; pre-existing behavior);
	//   - RELATIVE workdir — allowed only when it stays lexically
	//     INSIDE the engine working directory (no leading "..", not
	//     absolute);
	//   - ABSOLUTE workdir — REJECTED unless WorkdirRoots is non-empty
	//     AND the workdir resolves (symlink-safe) inside one of the
	//     roots.
	//
	// An empty WorkdirRoots (the default) therefore rejects every
	// absolute and every escaping workdir: the tool cannot be steered
	// at an arbitrary directory on the host.
	WorkdirRoots []string

	// Sandbox is the confinement seam; nil = NO CONFINEMENT (see the
	// package doc — loud, deliberate default).
	Sandbox SandboxFunc

	// SandboxName is recorded in every Outcome (default "none") so
	// logs never hide the confinement level.
	SandboxName string

	// ShellPath is the shell binary; default "bash" (resolved via the
	// parent PATH).
	ShellPath string
}

func (c *Config) normalize() {
	if c.MaxCapturedBytes <= 0 {
		c.MaxCapturedBytes = DefaultMaxCapturedBytes
	}
	if c.MaxTimeoutMs <= 0 {
		c.MaxTimeoutMs = DefaultMaxTimeoutMs
	}
	if c.DefaultTimeoutMs <= 0 {
		c.DefaultTimeoutMs = DefaultDefaultTimeoutMs
	}
	if c.DefaultTimeoutMs > c.MaxTimeoutMs {
		c.DefaultTimeoutMs = c.MaxTimeoutMs
	}
	if c.ShellPath == "" {
		c.ShellPath = DefaultShellPath
	}
	if c.SandboxName == "" {
		c.SandboxName = "none"
	}
} // sandboxLabel returns the recorded sandbox name after normalization
// ("none" when unset).
func (c *Config) sandboxLabel() string {
	if c.SandboxName == "" {
		return "none"
	}
	return c.SandboxName
}

// Args is the typed tool argument surface.
type Args struct {
	Command   string `json:"command"`
	TimeoutMs *int64 `json:"timeout_ms,omitempty"`
	Workdir   string `json:"workdir,omitempty"`
}

// Definition returns the run_shell ToolDefinition for cfg (zero value
// = production defaults). It is a drop-in for Pipeline.Register:
//
//   - IsConcurrencySafe=false: shell execution mutates host state, so
//     it runs as an EXCLUSIVE BARRIER — the slice-3 scheduler drains
//     the parallel pool around it and runs it alone (comes free; no
//     scheduler change needed here);
//   - TimeoutMs = cfg.MaxTimeoutMs as the pipeline-level dispatch
//     backstop. The per-call timeout_ms arg (clamped to the same cap)
//     is enforced INSIDE the body and normally fires first; the
//     pipeline-level Result.TimedOut is reserved for the cap itself;
//   - Execute returns structured JSON content for exit/signal causes
//     and typed errors for timeout/spawn/invalid-args causes, which
//     the Pipeline normalizes into isError results (never thrown).
func Definition(cfg Config) tools.ToolDefinition {
	cfg.normalize()
	return tools.ToolDefinition{
		Name:              Name,
		Description:       description,
		Parameters:        json.RawMessage(parametersSchema),
		IsConcurrencySafe: false, // exclusive barrier (slice-3 scheduler)
		TimeoutMs:         int(cfg.MaxTimeoutMs),
		Execute: func(ctx context.Context, raw json.RawMessage) (string, error) {
			return execute(ctx, &cfg, raw)
		},
	}
}

// description is factored out for readability.
const description = "Runs a shell command via bash -c (non-interactive, no profile/rc, no history) and returns structured output: {cause, exitCode, signal, timedOut, stdout, stderr, truncated, durationMs, effectiveTimeoutMs, sandbox}. " +
	"cause is exactly one of exit | signal | timeout | error (orthogonal facts, never conflated). " +
	"Non-zero exit codes and in-shell command-not-found (127) are NORMAL outcomes, not tool errors. " +
	"Not concurrency-safe: runs as an exclusive barrier. timeout_ms defaults to 30000 and is capped at 600000; on expiry the whole process group is killed. " +
	"Captured output is capped per stream (64KiB default) with a truncation marker. " +
	"The child env is explicit: PATH, HOME, TERM=dumb, LANG plus a configured allowlist; names matching KEY/SECRET/TOKEN/PASSWORD (case-insensitive) and engine credential vars are never passed. " +
	"Sandbox: check the sandbox field — \"none\" means NO confinement (the command runs with the engine's own privileges); \"read-only\" or \"workspace-write\" mean kernel-enforced confinement (writes outside the configured contract and network access are denied). "

// sensitiveEnvPattern is the dsh SENSITIVE_ENV_PATTERN
// (/KEY|PASSWORD|SECRET|TOKEN/i, name-match drop).
var sensitiveEnvPattern = regexp.MustCompile(`(?i)KEY|PASSWORD|SECRET|TOKEN`)

// engineCredentialPrefix names the engine's own credential-variable
// namespace: vars carrying it are dropped even when allowlisted (the
// dsh DSH_* rule, restated for this engine).
const engineCredentialPrefix = "VH_AGENT_HARNESS_"

// isSensitiveEnvName reports whether an env NAME must be dropped:
// sensitive-pattern match OR engine-credential prefix (the dual scrub —
// both are name-match drops; conservative direction: false positives
// drop, never pass).
func isSensitiveEnvName(name string) bool {
	if sensitiveEnvPattern.MatchString(name) {
		return true
	}
	return strings.HasPrefix(name, engineCredentialPrefix)
}

// fallbackPATH is used when the parent PATH is unset (a bare service
// context) so the shell is still findable.
const fallbackPATH = "/usr/local/bin:/usr/bin:/bin"

// buildEnv constructs the child environment EXPLICITLY (default-deny):
// TERM=dumb, PATH, HOME, LANG, plus EnvAllowlist pass-throughs merged
// AFTER the scrub (dsh order: explicit env merges after the scrub; the
// scrub still wins on names). Nothing else from the parent survives —
// no BASH_ENV, no credentials, no incidental state.
func buildEnv(cfg *Config) []string {
	env := []string{"TERM=dumb"}
	if p := os.Getenv("PATH"); p != "" {
		env = append(env, "PATH="+p)
	} else {
		env = append(env, "PATH="+fallbackPATH)
	}
	if h := os.Getenv("HOME"); h != "" {
		env = append(env, "HOME="+h)
	}
	if l := os.Getenv("LANG"); l != "" {
		env = append(env, "LANG="+l)
	} else {
		env = append(env, "LANG=C.UTF-8")
	}
	seen := map[string]bool{"TERM": true, "PATH": true, "HOME": true, "LANG": true}
	for _, name := range cfg.EnvAllowlist {
		if seen[name] || isSensitiveEnvName(name) {
			continue // base env wins; scrub wins over the allowlist
		}
		if v, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+v)
			seen[name] = true
		}
	}
	return env
}

// commandMatches reports whether cmd matches entry as an exact command
// or a whole-word prefix ("rm -rf" matches "rm -rf /tmp/x" but not
// "rm -rfx"). Coarse by design; guards are the policy layer.
func commandMatches(entry, cmd string) bool {
	if entry == "" {
		return false
	}
	if cmd == entry {
		return true
	}
	return strings.HasPrefix(cmd, entry+" ")
}

// policyAllows applies the Config command lists (empty allowlist =
// allow; denylist always wins).
func policyAllows(cfg *Config, command string) (bool, string) {
	for _, d := range cfg.DeniedCommands {
		if commandMatches(d, command) {
			return false, fmt.Sprintf("command matches configured denylist entry %q", d)
		}
	}
	if len(cfg.AllowedCommands) > 0 {
		for _, a := range cfg.AllowedCommands {
			if commandMatches(a, command) {
				return true, ""
			}
		}
		return false, "command does not match any configured allowlist entry"
	}
	return true, ""
}

// resolveTimeout maps the args value onto the effective deadline:
// omitted/nil/0 ⇒ DefaultTimeoutMs; negative ⇒ invalid; above the cap ⇒
// clamped to the cap (the effective value is what runs and what the
// outcome records).
func resolveTimeout(cfg *Config, timeoutMs *int64) (int64, error) {
	if timeoutMs == nil || *timeoutMs == 0 {
		return cfg.DefaultTimeoutMs, nil
	}
	if *timeoutMs < 0 {
		return 0, fmt.Errorf("run_shell: timeout_ms must be >= 0, got %d", *timeoutMs)
	}
	if *timeoutMs > cfg.MaxTimeoutMs {
		return cfg.MaxTimeoutMs, nil
	}
	return *timeoutMs, nil
}

// validateWorkdir confines the optional working directory BEFORE any
// process is spawned (a bad one is a clear invalid-args error instead
// of a spawn-class chdir failure deeper in) and checks it exists.
//
// Confinement policy (see Config.WorkdirRoots): the workdir is
// client/model-controlled, so without a policy it is a same-class hole
// as an unconfined session path — it steers WHERE the command runs.
// Conservative default: relative workdirs must stay lexically inside
// the engine working directory; absolute workdirs are admitted ONLY
// when configured under WorkdirRoots and resolved symlink-safe inside
// one of them.
func validateWorkdir(cfg *Config, workdir string) error {
	if workdir == "" {
		return nil
	}
	if filepath.IsAbs(workdir) {
		for _, root := range cfg.WorkdirRoots {
			if root == "" {
				continue
			}
			if err := confinedToRoot(root, workdir); err == nil {
				return checkDir(workdir)
			}
		}
		return fmt.Errorf("run_shell: workdir %q rejected by confinement policy: absolute workdirs require a configured WorkdirRoots entry that contains them (symlink-safe)", workdir)
	}
	clean := filepath.Clean(workdir)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("run_shell: workdir %q rejected by confinement policy: relative workdirs must stay inside the engine working directory", workdir)
	}
	return checkDir(workdir)
}

// checkDir verifies the (already admitted) workdir exists and is a
// directory.
func checkDir(workdir string) error {
	fi, err := os.Stat(workdir)
	if err != nil {
		return fmt.Errorf("run_shell: workdir %q: %v", workdir, err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("run_shell: workdir %q is not a directory", workdir)
	}
	return nil
}

// confinedToRoot reports nil when target resolves inside root: both
// are EvalSymlinks-resolved (the workdir itself must exist for Stat to
// have meaning anyway) and the resolved target must be a strict
// descendant of the resolved root. Resolution failures reject.
func confinedToRoot(root, target string) error {
	realRoot, err := filepath.EvalSymlinks(filepath.Clean(root))
	if err != nil {
		return fmt.Errorf("workdir root %s unresolved: %w", root, err)
	}
	realTarget, err := filepath.EvalSymlinks(filepath.Clean(target))
	if err != nil {
		return fmt.Errorf("workdir %s unresolved: %w", target, err)
	}
	rel, err := filepath.Rel(realRoot, realTarget)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("workdir %s resolves outside the root %s", target, root)
	}
	return nil
}

// execute is the tool body: parse args, resolve policy/deadline, run,
// and map the Outcome onto the Pipeline contract (structured content
// for exit/signal; typed error for timeout/spawn/invalid-args — the
// Pipeline normalizes those into isError results).
func execute(ctx context.Context, cfg *Config, raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", fmt.Errorf("run_shell: args.command is required")
	}
	var a Args
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return "", fmt.Errorf("run_shell: invalid args: %w", err)
	}
	if a.Command == "" {
		return "", fmt.Errorf("run_shell: args.command is required and must be non-empty")
	}
	if err := validateWorkdir(cfg, a.Workdir); err != nil {
		return "", err
	}
	if ok, reason := policyAllows(cfg, a.Command); !ok {
		return "", fmt.Errorf("run_shell: %s", reason)
	}
	timeoutMs, err := resolveTimeout(cfg, a.TimeoutMs)
	if err != nil {
		return "", err
	}

	out := run(ctx, cfg, a.Command, timeoutMs, a.Workdir)
	switch out.Cause {
	case CauseExit, CauseSignal:
		content, err := json.Marshal(out)
		if err != nil {
			return "", fmt.Errorf("run_shell: marshal outcome: %w", err)
		}
		return string(content), nil
	case CauseTimeout:
		return "", &TimeoutError{
			EffectiveTimeoutMs: out.EffectiveTimeoutMs,
			StdoutBytes:        len(out.Stdout),
			StderrBytes:        len(out.Stderr),
		}
	default: // CauseError
		if out.sandboxErr != nil {
			// Typed fail-closed sandbox refusal: the command never ran
			// and the caller can errors.As the specific cause (the
			// Pipeline normalizes it into an isError result whose text
			// carries the unavailable fact).
			return "", out.sandboxErr
		}
		return "", fmt.Errorf("run_shell: spawn failed: %s", out.SpawnError)
	}
}
