package permission

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ShellGuardHook is the slice-4b permission gate: it delegates each candidate
// command to the shell-guard engine via a node subprocess (eval.js). It is the
// production replacement for NoOpHook and is wired as the package default in
// internal/cli (runtime_common.go).
//
// Fail-safe contract: ANY failure to obtain an authoritative verdict — node
// missing, node too old, eval.js missing, spawn error, timeout, non-zero exit,
// empty output, malformed JSON, unknown action — returns (Deny, "", err). The
// hook NEVER falls back to NoOpHook and NEVER returns Allow on an engine fault.
// Only a clean exit-0 with a recognised {"action":...} line yields the mapped
// Action.
//
// Forks (slice 4b, flagged in the closeout report):
//   - node is respawned PER Evaluate (daemon mode deferred).
//   - Ask maps to permission.Ask; the CLI layer treats Ask as deny-by-default
//     (no operator prompt loop yet).
//   - Node>=18 is enforced at construction; override via WithNodeBin (manifest
//     runtime.node_path / --node-bin is a future wiring path).
//   - npm install is NOT automated; missing node_modules makes eval.js fault
//     -> Deny + guidance, never a silent allow.
type ShellGuardHook struct {
	// NodeBin is the absolute path to the node executable used to run eval.js.
	// Resolved from PATH at construction if empty.
	NodeBin string
	// EvalPath is the absolute path to .opencode/plugins/shell-guard/eval.js.
	// Defaults to join(HarnessRoot, evalRelPath).
	EvalPath string
	// HarnessRoot is the project root containing .opencode/ (the cwd for the
	// node subprocess and the anchor for repoRoot-derived fs probes).
	HarnessRoot string
	// Timeout is the per-Evaluate subprocess budget. Defaults to
	// defaultHookTimeout (5s).
	Timeout time.Duration

	// runner is the subprocess executor. Production uses osExecRunner; tests
	// inject a fake to assert the JSON->Action mapping without spawning node.
	runner Runner
	// bridgeErr is set by validate() at construction. When non-nil, Evaluate
	// returns it immediately as a loud Deny (engine unavailable).
	bridgeErr error
	// skipValidate is test-only: it skips the node/eval availability probe so
	// JSON-mapping unit tests can run without a real node binary.
	skipValidate bool
}

// Runner is the subprocess seam ShellGuardHook.Evaluate calls through. It runs
// [nodeBin, args...] with cwd and a timeout, returning captured stdout/stderr,
// the process exit code, and a spawn/timeout error. The production runner is
// osExecRunner; tests inject a fake.
type Runner interface {
	Run(ctx context.Context, nodeBin string, args []string, cwd string, timeout time.Duration) (stdout, stderr []byte, exitCode int, err error)
}

// osExecRunner is the production Runner: exec.CommandContext with stdout/stderr
// capture. Non-zero exit yields the captured output + exit code with err=nil
// (so the hook can distinguish "ran, exited non-zero" from "could not run").
// Spawn errors (binary missing, permission denied) and timeout return err set.
type osExecRunner struct{}

func (osExecRunner) Run(ctx context.Context, nodeBin string, args []string, cwd string, timeout time.Duration) ([]byte, []byte, int, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, nodeBin, args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return stdout.Bytes(), stderr.Bytes(), 0, nil
	}
	// Timeout / cancellation beats exit-code interpretation.
	if runCtx.Err() == context.DeadlineExceeded {
		return stdout.Bytes(), stderr.Bytes(), -1, fmt.Errorf("node eval timed out after %s", timeout)
	}
	// Distinguish a non-zero exit (ran, failed) from a spawn error.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return stdout.Bytes(), stderr.Bytes(), exitErr.ExitCode(), nil
	}
	// Spawn error: binary missing, permission denied, etc.
	return nil, stderr.Bytes(), -1, err
}

// ShellGuardOption configures a ShellGuardHook at construction.
type ShellGuardOption func(*ShellGuardHook)

// WithNodeBin overrides the node binary path (skips PATH lookup). Use for
// future manifest runtime.node_path / --node-bin wiring or for tests that want
// to force a "node missing" verdict.
func WithNodeBin(p string) ShellGuardOption {
	return func(h *ShellGuardHook) { h.NodeBin = p }
}

// WithTimeout overrides the per-Evaluate subprocess budget.
func WithTimeout(d time.Duration) ShellGuardOption {
	return func(h *ShellGuardHook) { h.Timeout = d }
}

// WithRunner injects a subprocess double (tests only).
func WithRunner(r Runner) ShellGuardOption {
	return func(h *ShellGuardHook) { h.runner = r }
}

// withBypassValidate is UNEXPORTED and test-only: it skips the node/eval
// availability probe so the JSON-mapping unit tests can run without a real
// node binary (they inject a fake Runner returning canned stdout).
func withBypassValidate() ShellGuardOption {
	return func(h *ShellGuardHook) { h.skipValidate = true }
}

const (
	// nodeMinMajor is the minimum node major version eval.js supports (ESM
	// + optional top-level await in the rule aggregator).
	nodeMinMajor = 18
	// NodeMinMajor is the exported minimum supported node major version. It is
	// the shared threshold used by the shell-guard bridge AND by `harness
	// preflight`/`vh-agent-harness doctor` so both report the same bar.
	NodeMinMajor = nodeMinMajor
	// defaultHookTimeout is the per-Evaluate subprocess budget.
	defaultHookTimeout = 5 * time.Second
	// evalRelPath is eval.js's location relative to the harness root.
	evalRelPath = ".opencode/plugins/shell-guard/eval.js"
	// EvalRelPath is the exported eval.js location relative to the harness root.
	// preflight/doctor stat it to confirm the shell-guard plugin is installed.
	EvalRelPath = evalRelPath
)

// ProbeNode is the shared environment probe used by the shell-guard bridge
// (NewShellGuardHook.validate) and by the `vh-agent-harness preflight` / `harness
// doctor` verbs. It resolves node from PATH and returns its absolute path plus
// major version, or an error describing what is wrong (missing / unparseable).
//
// Factoring this out keeps a single source of truth for the node>=18 bar so the
// CLI verbs cannot drift from the bridge's own availability check.
func ProbeNode() (binPath string, major int, err error) {
	p, err := exec.LookPath("node")
	if err != nil {
		return "", 0, fmt.Errorf("node not found on PATH (install Node>=%d, or set the node path)", nodeMinMajor)
	}
	major, err = nodeMajorVersion(p)
	if err != nil {
		return p, 0, err
	}
	return p, major, nil
}

// NewShellGuardHook constructs a ShellGuardHook anchored at harnessRoot (the
// project root containing .opencode/). It applies options, then probes node +
// eval.js availability; if either is unavailable the returned hook's Evaluate
// ALWAYS returns (Deny, "", bridgeErr). It NEVER returns nil and NEVER falls
// back to NoOpHook.
func NewShellGuardHook(harnessRoot string, opts ...ShellGuardOption) *ShellGuardHook {
	h := &ShellGuardHook{
		HarnessRoot: harnessRoot,
		EvalPath:    filepath.Join(harnessRoot, evalRelPath),
		Timeout:     defaultHookTimeout,
		runner:      osExecRunner{},
	}
	for _, opt := range opts {
		opt(h)
	}
	if !h.skipValidate {
		h.bridgeErr = h.validate()
	}
	return h
}

// validate probes the bridge prerequisites and records the first failure.
// Order: node on PATH (or override) -> node version >=18 -> eval.js present.
func (h *ShellGuardHook) validate() error {
	if h.NodeBin == "" {
		p, err := exec.LookPath("node")
		if err != nil {
			return fmt.Errorf("bridge unavailable: node not found on PATH (install Node>=%d, or set the node path)", nodeMinMajor)
		}
		h.NodeBin = p
	}
	major, err := nodeMajorVersion(h.NodeBin)
	if err != nil {
		return fmt.Errorf("cannot determine node version: %w", err)
	}
	if major < nodeMinMajor {
		return fmt.Errorf("bridge unavailable: node %d<%d (eval.js requires ESM + top-level await)", major, nodeMinMajor)
	}
	if h.EvalPath == "" {
		h.EvalPath = filepath.Join(h.HarnessRoot, evalRelPath)
	}
	if _, err := os.Stat(h.EvalPath); err != nil {
		return fmt.Errorf("eval.js not found at %s (run `vh-agent-harness install`; the shell-guard plugin ships with the harness)", h.EvalPath)
	}
	return nil
}

// nodeMajorVersion runs `node --version` and parses the leading major number.
func nodeMajorVersion(nodeBin string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultHookTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, nodeBin, "--version").Output()
	if err != nil {
		return 0, err
	}
	return parseNodeMajor(string(out))
}

var nodeVersionRe = regexp.MustCompile(`v?(\d+)\.`)

func parseNodeMajor(s string) (int, error) {
	m := nodeVersionRe.FindStringSubmatch(strings.TrimSpace(s))
	if len(m) < 2 {
		return 0, fmt.Errorf("unparseable node version %q", s)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, fmt.Errorf("unparseable node version %q: %w", s, err)
	}
	return n, nil
}

// Evaluate runs the candidate command through the shell-guard node bridge and
// maps the JSON verdict to an Action. Fail-safe: any fault -> (Deny, "", err).
func (h *ShellGuardHook) Evaluate(ctx context.Context, cmd []string) (Action, string, error) {
	if h.bridgeErr != nil {
		return Deny, "", h.bridgeErr
	}
	args := append([]string{h.EvalPath}, cmd...)
	timeout := h.Timeout
	if timeout <= 0 {
		timeout = defaultHookTimeout
	}
	stdout, _, exitCode, err := h.runner.Run(ctx, h.NodeBin, args, h.HarnessRoot, timeout)
	if err != nil {
		return Deny, "", fmt.Errorf("shell-guard bridge failed: %w", err)
	}
	if exitCode != 0 {
		return Deny, "", fmt.Errorf("shell-guard eval.js exited %d", exitCode)
	}
	trimmed := bytes.TrimSpace(stdout)
	if len(trimmed) == 0 {
		return Deny, "", fmt.Errorf("shell-guard eval.js produced no output")
	}
	var res struct {
		Action string `json:"action"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(trimmed, &res); err != nil {
		return Deny, "", fmt.Errorf("malformed shell-guard response %q: %w", string(trimmed), err)
	}
	switch res.Action {
	case "allow":
		return Allow, res.Reason, nil
	case "deny":
		return Deny, res.Reason, nil
	case "ask":
		return Ask, res.Reason, nil
	default:
		return Deny, "", fmt.Errorf("unknown shell-guard action %q", res.Action)
	}
}
