// config.go — vh-agentd command-line surface: flag parsing, fail-closed
// validation, and the credential rule (API key read from the environment
// variable NAMED by --api-key-env — never a literal flag value).
package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/tools/shell"
)

// engineVersion is the daemon/engine build identity reported by
// --version alongside the wire ProtocolVersion.
const engineVersion = "0.1.0"

// defaultApprovalTimeoutMs bounds each pending approval by default (dsh
// F-PIPE-2 fail-closed posture: an unanswerable approval denies).
const defaultApprovalTimeoutMs = 30000

// Optimizer selections for the offline --compile-prompt step.
const (
	// optimizerLLM is the real adapter-backed optimizer: ONE compile-time
	// LLM call through the configured adapter; the output is a CANDIDATE
	// checked by the compile-time invariants (the authority). The
	// DEFAULT. Requires the --api-key-env variable to be SET for a
	// compile run (fail-closed exit 2 otherwise — never a silent dedup).
	optimizerLLM = "llm"
	// optimizerDedup is the reference fake: deterministic, mechanical,
	// offline, no key — for no-network runs and CI.
	optimizerDedup = "dedup"
)

// Config is the validated daemon configuration.
type Config struct {
	// Adapter is the normalized provider name ("openaicompat" |
	// "anthropic"); the flags "openai" and "openaicompat" are aliases.
	Adapter string
	Model   string
	BaseURL string
	// APIKeyEnv names the environment variable holding the API key.
	APIKeyEnv string
	// SessionDir is the explicit directory for durable session logs.
	// There is deliberately no default: the daemon refuses to start
	// without it (no silent writes). It is also the home of the
	// compiled-prompt artifact cache (SessionDir/compiled-prompts/) and
	// the scheduler state file (SessionDir/scheduler-state.json), so
	// every durable byte the daemon writes lives under one root.
	SessionDir string
	// MaxTokens overrides the adapter max_tokens default (anthropic
	// requires one; 0 takes the adapter defaults).
	MaxTokens int
	// ApprovalTimeoutMs bounds each pending approval.
	ApprovalTimeoutMs int
	// CacheBreakpoints maps --cache-breakpoints: 0 = caching off
	// (default); 1..4 = explicit Anthropic prompt-cache breakpoint
	// budget, wired into the anthropic adapter's CacheConfig. It is
	// REJECTED for openaicompat (see validateCacheBreakpoints).
	CacheBreakpoints int
	// Optimizer is the validated --optimizer value (llm | dedup) for
	// the offline --compile-prompt step. It selects BOTH the compile
	// path AND the artifact family the serving rule hashes for: the
	// llm version string is derived deterministically from the model
	// (prompt.LLMOptimizerVersion), so serving needs no key or network
	// to find an llm-compiled artifact. Default llm.
	Optimizer string
	// SandboxMode is the validated --sandbox value (off | read-only |
	// workspace-write) wired into run_shell's confinement seam. off
	// (the default) keeps Config.Sandbox nil — the loud NO-confinement
	// posture; the confining modes arm the kernel backend
	// (Landlock+seccomp) and fail closed per call when it is
	// unavailable.
	SandboxMode shell.SandboxMode
	// SandboxWritableRoots are the write-allowed directories under
	// workspace-write mode. The daemon default is the session dir plus
	// the OS temp dir (dsh writableRoots vocabulary: the workspace and
	// tmp). Empty for off and read-only.
	SandboxWritableRoots []string
	// SpillMaxInline is the inline budget for oversize tool results
	// (--spill-max-inline). Content above it is spilled FULL to a
	// per-session store (<session-dir>/<session-id>.spill/, 0700/0600,
	// content-addressed) and the committed event carries a bounded
	// preview + opaque locator retrievable via the spill_read tool.
	// Default 65536 (matching run_shell's capture cap); 0 disables the
	// spill entirely (today's always-inline behavior).
	SpillMaxInline int64
	// WorkdirRoots is the symlink-safe confinement root set for the
	// model-facing file tools (read/write/edit/glob/search) AND the
	// run_shell workdir policy (--workdir-roots): every user-supplied
	// file path resolves against these roots — relative paths against
	// the FIRST root, absolute paths must sit under some root, and
	// escapes (lexical or via symlink) reject fail-closed with typed
	// errors and zero filesystem effects. Default: the daemon's
	// working directory resolved absolute. Entries are canonicalized
	// (symlinks resolved) at validation time.
	WorkdirRoots []string
}

// usageDoc documents credential handling on the help surface (the key is
// read from the environment only, held in adapter memory, sent only to
// the configured base URL, and never persisted in session logs).
const usageDoc = `vh-agentd — headless agent daemon (host protocol v1 over stdio)

Speaks versioned JSON-RPC over NDJSON on stdin/stdout (stdout is
protocol; diagnostics go to stderr). See docs/native-engine/host-protocol.md.

Credentials:
  The API key is read at startup from the environment variable NAMED by
  --api-key-env (there is intentionally no literal --api-key flag). It is
  held in adapter memory only, sent to the configured --base-url, and
  never written to session logs or diagnostics (refs-not-stored).
  --api-key-env must always NAME a variable. For --compile-prompt the
  variable only needs to be SET when --optimizer llm is active (the
  default): the llm optimizer makes one real call through the
  configured adapter. With --optimizer dedup the compile is fully
  offline and the key value is never read.

Optimizer (--optimizer dedup|llm, for --compile-prompt):
  llm (default) — the real adapter-backed optimizer: ONE compile-time
  LLM call (no retries; a failed compile is a rerun of the command),
  output treated as a CANDIDATE and checked by the fail-closed compile
  invariants. Needs --api-key-env's variable set; without it the
  compile exits 2 (fail-closed — no silent dedup fallback).
  dedup — the reference fake: deterministic, mechanical, offline, no
  key. Artifacts of the two optimizers are separate content-hash
  families; serve with the same --optimizer value you compiled with.

Prompt cache (--cache-breakpoints N):
  Anthropic only. 0 (default) keeps explicit caching off; 1-4 sets the
  cache_control breakpoint budget (breakpoint 1: final tool definition,
  breakpoint 2: the system prompt; the adapter documents placement).
  Rejected for --adapter openai: OpenAI-compatible endpoints cache
  IMPLICITLY via prefix matching — there is no breakpoint knob to map.

Sandbox (--sandbox MODE):
  Confinement for the run_shell tool. off (default) = NO confinement:
  commands run with the daemon's own privileges — this is the loud,
  deliberate pre-slice posture. read-only = kernel-enforced (Landlock +
  seccomp): the whole filesystem stays READABLE but no writes are
  allowed and network syscalls are denied. workspace-write = read-only
  plus writes allowed ONLY under the session dir and the OS temp dir.
  Confinement fail-closes: on a host without the OS primitives (non-
  Linux, or a kernel without landlock+seccomp) every sandboxed
  run_shell call returns a typed sandbox-unavailable error instead of
  running unconfined. There is deliberately no "danger-full-access"
  mode — it is redundant with off.

Spill (--spill-max-inline N):
  Oversize tool results spill to durable per-session files instead of
  flooding the log/context: content above N bytes (default 65536,
  matching the run_shell capture cap) is written FULL to
  <session-dir>/<session-id>.spill/ (0700 dir, 0600 content-addressed
  files) and the committed tool/result event carries a bounded preview
  plus an opaque locator; the spill_read tool pages the stored bytes
  back in bounded windows (offset/length per call, lengths clamped to
  the same inline cap so every page fits inline, hash-validated,
  fail-closed; a call at offset == size returns an empty terminal
  window). 0 disables the spill (always-inline, the pre-spill
  behavior). A spill-store write failure silently keeps the content
  inline — the sidecar never fails the tool result. Spill files are
  durable sidecar state: replay of a session log never touches them
  (loss degrades retrieval, not replay integrity).

Workdir roots (--workdir-roots DIR[,DIR...]):
  The confinement set for the model-facing file tools
  (read/write/edit/glob/search) and run_shell's absolute-workdir
  admission. Relative file-tool paths resolve against the FIRST root;
  absolute paths must sit under some root; lexical escapes (..) and
  symlink crossings reject fail-closed with typed errors and ZERO
  filesystem effects (a rejected write leaves no file and creates no
  parent directories). The walk never follows symlinks. Default: the
  daemon's working directory, resolved absolute — so a daemon started
  in its project tree gets the tree as its workspace. Entries must be
  absolute paths to existing directories; they are canonicalized
  (symlinks resolved) at startup. run_shell's relative-workdir
  behavior (inside the engine working directory) is unchanged; the
  roots EXTEND what absolute workdirs are allowed.

System prompt (compiled-sysprompt model):
  The daemon serves its system prompt from the compiled artifact under
  <session-dir>/compiled-prompts/ when one matches the current content
  hash, and falls back to raw assembly otherwise (the fallback is logged,
  never silent). Populate the artifact with --compile-prompt.

Usage:
  vh-agentd --adapter openai|anthropic --model M --base-url URL
            --api-key-env VAR --session-dir DIR [--max-tokens N]
            [--approval-timeout-ms MS] [--cache-breakpoints N]
            [--sandbox off|read-only|workspace-write]
            [--spill-max-inline N] [--workdir-roots DIR[,DIR...]]
            [--optimizer dedup|llm] [--compile-prompt] [--version]
  vh-agentd --verify-log PATH

  --compile-prompt  run the compile-time prompt compilation with the current
                    config, write the content-hashed artifact under
                    <session-dir>/compiled-prompts/, report to stderr,
                    and exit. With --optimizer llm (the default) this
                    makes ONE real LLM call through the configured
                    adapter and needs the key variable SET; with
                    --optimizer dedup it is offline and keyless. No
                    protocol session either way.

  --verify-log PATH read-only mode: replay the session log at PATH and
                    print ONE JSON line {events, format_version,
                    surface_sha256, messages} (sha256 over the canonical
                    derived-surface JSON) to stdout; exit 1 with the
                    reason on stderr on any replay error. No protocol
                    session, no other flags required. Two runs on the
                    same log print identical bytes — the
                    replay-determinism prover for real produced logs.
`

var envNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// normalizeAdapter maps flag aliases onto the registered adapter names.
func normalizeAdapter(v string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "openai", "openaicompat":
		return "openaicompat", true
	case "anthropic":
		return "anthropic", true
	default:
		return "", false
	}
}

// validateCacheBreakpoints checks the --cache-breakpoints value against
// the adapter selection: 0..4 in range; >0 only with anthropic (explicit
// cache_control breakpoints). The openai rejection is a CLEAR error, not
// a silent ignore — OpenAI-compatible caching is implicit, so a
// breakpoint budget on that adapter would be a no-op the operator
// believes is armed.
func validateCacheBreakpoints(adapter string, n int) error {
	if n == 0 {
		return nil
	}
	if n < 0 || n > 4 {
		return fmt.Errorf("invalid --cache-breakpoints %d: must be 0 (off) or 1..4", n)
	}
	if adapter != "anthropic" {
		return fmt.Errorf("--cache-breakpoints %d is not supported with --adapter %s: explicit cache_control breakpoints are Anthropic-only (OpenAI-compatible endpoints cache implicitly via prefix matching; there is no breakpoint knob)", n, adapter)
	}
	return nil
}

// normalizeOptimizer maps the --optimizer flag onto its validated
// selection; empty means the default (llm).
func normalizeOptimizer(v string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "":
		return optimizerLLM, nil
	case optimizerLLM:
		return optimizerLLM, nil
	case optimizerDedup:
		return optimizerDedup, nil
	default:
		return "", fmt.Errorf("invalid --optimizer %q: must be llm (real adapter-backed compile; default) or dedup (offline reference fake)", v)
	}
}

// parseWorkdirRoots resolves the --workdir-roots flag value: comma-
// separated ABSOLUTE paths to existing DIRECTORIES, canonicalized
// through symlinks and deduped. An empty value defaults to the
// daemon's working directory resolved absolute (the daemon started
// somewhere deliberate; that tree is the natural workspace). Relative
// entries, nonexistent entries, and entries that resolve to a
// non-directory (a regular file, directly or through a symlink —
// EvalSymlinks alone cannot tell) reject fail-closed — a silently
// dropped root would narrow the workspace without notice, and a
// guessed root or a file root would break every relative resolution.
func parseWorkdirRoots(v string) ([]string, error) {
	if strings.TrimSpace(v) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("cannot resolve the default workdir root (the working directory): %w", err)
		}
		real, err := filepath.EvalSymlinks(filepath.Clean(cwd))
		if err != nil {
			return nil, fmt.Errorf("cannot canonicalize the working directory %s: %w", cwd, err)
		}
		if fi, err := os.Stat(real); err != nil {
			return nil, fmt.Errorf("cannot inspect the default workdir root %s: %w", real, err)
		} else if !fi.IsDir() {
			return nil, fmt.Errorf("default workdir root %s is not a directory", real)
		}
		return []string{real}, nil
	}
	seen := map[string]bool{}
	var out []string
	for _, entry := range strings.Split(v, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			return nil, fmt.Errorf("--workdir-roots contains an empty entry")
		}
		if !filepath.IsAbs(entry) {
			return nil, fmt.Errorf("--workdir-roots entry %q must be an absolute path", entry)
		}
		real, err := filepath.EvalSymlinks(filepath.Clean(entry))
		if err != nil {
			return nil, fmt.Errorf("--workdir-roots entry %q cannot be resolved (it must exist): %w", entry, err)
		}
		if fi, err := os.Stat(real); err != nil {
			return nil, fmt.Errorf("--workdir-roots entry %q cannot be inspected: %w", entry, err)
		} else if !fi.IsDir() {
			return nil, fmt.Errorf("--workdir-roots entry %q resolves to %s, which is not a directory (roots must be existing directories)", entry, real)
		}
		if seen[real] {
			continue
		}
		seen[real] = true
		out = append(out, real)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("--workdir-roots resolved to no usable roots")
	}
	return out, nil
}

// validate checks the parsed flags fail-closed and returns the
// normalized Config.
func validate(adapter, model, baseURL, apiKeyEnv, sessionDir, optimizer string, maxTokens, approvalTimeoutMs, cacheBreakpoints int, sandboxMode string, spillMaxInline int64, workdirRoots string) (*Config, error) {
	ad, ok := normalizeAdapter(adapter)
	if !ok {
		return nil, fmt.Errorf("invalid --adapter %q: must be openai (openaicompat) or anthropic", adapter)
	}
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("--model is required")
	}
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("--base-url is required")
	}
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("invalid --base-url %q: must be an absolute http(s) URL", baseURL)
	}
	if apiKeyEnv == "" {
		return nil, fmt.Errorf("--api-key-env is required (the API key is read from the environment only)")
	}
	if !envNameRe.MatchString(apiKeyEnv) {
		return nil, fmt.Errorf("invalid --api-key-env %q: not a valid environment variable name", apiKeyEnv)
	}
	if sessionDir == "" {
		return nil, fmt.Errorf("--session-dir is required (there is no default: the daemon refuses silent session writes)")
	}
	if !filepath.IsAbs(sessionDir) {
		return nil, fmt.Errorf("--session-dir %q must be an absolute path: under --sandbox workspace-write the session dir becomes a Landlock RWDir, and a relative path resolves against the sandboxed child's working directory — not the daemon's startup cwd — so session writes would be denied against an unintended root. Pass an absolute path (e.g. /var/lib/vh-agentd/sessions)", sessionDir)
	}
	if maxTokens < 0 {
		return nil, fmt.Errorf("invalid --max-tokens %d: must be >= 0", maxTokens)
	}
	if approvalTimeoutMs < 0 {
		return nil, fmt.Errorf("invalid --approval-timeout-ms %d: must be >= 0", approvalTimeoutMs)
	}
	if err := validateCacheBreakpoints(ad, cacheBreakpoints); err != nil {
		return nil, err
	}
	opt, err := normalizeOptimizer(optimizer)
	if err != nil {
		return nil, err
	}
	mode, err := shell.ParseSandboxMode(strings.TrimSpace(sandboxMode))
	if err != nil {
		return nil, fmt.Errorf("invalid --sandbox %q: %w", sandboxMode, err)
	}
	if spillMaxInline < 0 {
		return nil, fmt.Errorf("invalid --spill-max-inline %d: must be >= 0 (0 disables the oversize-result spill; positive is the inline byte budget)",
			spillMaxInline)
	}
	roots, err := parseWorkdirRoots(workdirRoots)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Adapter:           ad,
		Model:             model,
		BaseURL:           baseURL,
		APIKeyEnv:         apiKeyEnv,
		SessionDir:        sessionDir,
		MaxTokens:         maxTokens,
		ApprovalTimeoutMs: approvalTimeoutMs,
		CacheBreakpoints:  cacheBreakpoints,
		Optimizer:         opt,
		SandboxMode:       mode,
		SpillMaxInline:    spillMaxInline,
		WorkdirRoots:      roots,
	}
	// workspace-write default writable roots: the session dir (every
	// durable byte the daemon owns) plus the OS temp dir. Deduped when
	// they coincide.
	if mode == shell.SandboxWorkspaceWrite {
		cfg.SandboxWritableRoots = []string{sessionDir, os.TempDir()}
		if sessionDir == os.TempDir() {
			cfg.SandboxWritableRoots = []string{sessionDir}
		}
	}
	return cfg, nil
}
