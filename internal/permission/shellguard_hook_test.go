package permission

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeRunner is a Runner double that returns canned output without spawning
// node. It records what it was called with so tests can assert arg wiring.
type fakeRunner struct {
	stdout     []byte
	stderr     []byte
	exitCode   int
	err        error
	calls      int
	lastArgs   []string
	lastCwd    string
	lastBudget time.Duration
}

func (f *fakeRunner) Run(_ context.Context, _ string, args []string, cwd string, timeout time.Duration) ([]byte, []byte, int, error) {
	f.calls++
	f.lastArgs = append([]string(nil), args...)
	f.lastCwd = cwd
	f.lastBudget = timeout
	return f.stdout, f.stderr, f.exitCode, f.err
}

// newMappingHook builds a ShellGuardHook wired to a fake runner with validate
// bypassed, so the JSON->Action mapping can be asserted without a real node.
func newMappingHook(t *testing.T, runner Runner) *ShellGuardHook {
	t.Helper()
	return NewShellGuardHook(t.TempDir(), WithRunner(runner), withBypassValidate())
}

// --- JSON -> Action mapping (fake runner, no node needed) --------------------

func TestShellGuardHook_AllowMapping(t *testing.T) {
	r := &fakeRunner{stdout: []byte(`{"action":"allow","reason":""}` + "\n")}
	h := newMappingHook(t, r)
	act, reason, err := h.Evaluate(context.Background(), []string{"echo", "hello"})
	if err != nil || act != Allow || reason != "" {
		t.Fatalf("got (%s,%q,%v) want (Allow,\"\",nil)", act, reason, err)
	}
	if r.calls != 1 || len(r.lastArgs) < 2 || r.lastArgs[1] != "echo" {
		t.Errorf("runner args wrong: %v", r.lastArgs)
	}
}

func TestShellGuardHook_DenyMapping(t *testing.T) {
	r := &fakeRunner{stdout: []byte(`{"action":"deny","reason":"blocked: bad"}`)}
	h := newMappingHook(t, r)
	act, reason, err := h.Evaluate(context.Background(), []string{"rm", "-rf"})
	if err != nil || act != Deny || reason != "blocked: bad" {
		t.Fatalf("got (%s,%q,%v) want (Deny,\"blocked: bad\",nil)", act, reason, err)
	}
}

func TestShellGuardHook_AskMapping(t *testing.T) {
	r := &fakeRunner{stdout: []byte(`{"action":"ask","reason":"git mutation"}`)}
	h := newMappingHook(t, r)
	act, reason, err := h.Evaluate(context.Background(), []string{"git", "commit"})
	if err != nil || act != Ask || reason != "git mutation" {
		t.Fatalf("got (%s,%q,%v) want (Ask,\"git mutation\",nil)", act, reason, err)
	}
}

func TestShellGuardHook_ExitNonZero_Denies(t *testing.T) {
	r := &fakeRunner{exitCode: 2, stderr: []byte("engine fault\n")}
	h := newMappingHook(t, r)
	act, _, err := h.Evaluate(context.Background(), []string{"x"})
	if act != Deny || err == nil {
		t.Fatalf("exit2 must yield (Deny,err); got (%s,%v)", act, err)
	}
	if !strings.Contains(err.Error(), "exited 2") {
		t.Errorf("err should mention exited 2; got %v", err)
	}
}

func TestShellGuardHook_RunnerError_Denies(t *testing.T) {
	r := &fakeRunner{err: context.DeadlineExceeded}
	h := newMappingHook(t, r)
	act, _, err := h.Evaluate(context.Background(), []string{"x"})
	if act != Deny || err == nil {
		t.Fatalf("runner err must yield (Deny,err); got (%s,%v)", act, err)
	}
}

func TestShellGuardHook_MalformedJSON_Denies(t *testing.T) {
	r := &fakeRunner{stdout: []byte("not json at all\n")}
	h := newMappingHook(t, r)
	act, _, err := h.Evaluate(context.Background(), []string{"x"})
	if act != Deny || err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("malformed must yield (Deny,malformed err); got (%s,%v)", act, err)
	}
}

func TestShellGuardHook_UnknownAction_Denies(t *testing.T) {
	r := &fakeRunner{stdout: []byte(`{"action":"maybe","reason":"?"}`)}
	h := newMappingHook(t, r)
	act, _, err := h.Evaluate(context.Background(), []string{"x"})
	if act != Deny || err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("unknown action must yield (Deny,unknown err); got (%s,%v)", act, err)
	}
}

func TestShellGuardHook_EmptyStdout_Denies(t *testing.T) {
	r := &fakeRunner{stdout: nil}
	h := newMappingHook(t, r)
	act, _, err := h.Evaluate(context.Background(), []string{"x"})
	if act != Deny || err == nil {
		t.Fatalf("empty stdout must yield (Deny,err); got (%s,%v)", act, err)
	}
}

// --- Availability (real validate, no bypass) ---------------------------------

func TestShellGuardHook_NodeMissing_Denies(t *testing.T) {
	// A path that cannot exist: validate runs `node --version` against it,
	// exec fails, bridgeErr is set, and Evaluate returns a loud Deny.
	root := t.TempDir()
	h := NewShellGuardHook(root, WithNodeBin("/nonexistent/node-binary-xyz-12345"))
	if h.bridgeErr == nil {
		t.Fatalf("expected bridgeErr for missing node, got nil")
	}
	act, _, err := h.Evaluate(context.Background(), []string{"echo", "hi"})
	if act != Deny || err == nil {
		t.Fatalf("node-missing must yield (Deny,err); got (%s,%v)", act, err)
	}
}

func TestShellGuardHook_EvalMissing_Denies(t *testing.T) {
	// Real node (present in the devcontainer) but a temp root with NO eval.js.
	// validate() reaches the os.Stat step and fails -> loud Deny.
	root := t.TempDir()
	h := NewShellGuardHook(root)
	if h.bridgeErr == nil {
		t.Skipf("node not available in this env (validate passed unexpectedly); skipping eval-missing probe")
	}
	if !strings.Contains(h.bridgeErr.Error(), "eval.js not found") {
		t.Fatalf("expected eval.js-not-found bridgeErr; got %v", h.bridgeErr)
	}
	act, _, err := h.Evaluate(context.Background(), []string{"echo", "hi"})
	if act != Deny || err != h.bridgeErr {
		t.Fatalf("eval-missing must yield (Deny,bridgeErr); got (%s,%v)", act, err)
	}
}

func TestShellGuardHook_NodeMinVersion(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"v18.0.0\n", 18},
		{"v20.11.1\n", 20},
		{"v24.16.0\n", 24},
		{"v8.17.0\n", 8},
		{"v1.22.0\n", 1},
	}
	for _, c := range cases {
		got, err := parseNodeMajor(c.in)
		if err != nil || got != c.want {
			t.Errorf("parseNodeMajor(%q) = (%d,%v), want (%d,nil)", c.in, got, err, c.want)
		}
	}
	if _, err := parseNodeMajor("garbage"); err == nil {
		t.Errorf("parseNodeMajor(garbage) should error")
	}
}

// --- Live bridge: end-to-end node eval.js + WASM + rules ---------------------

func TestShellGuardHook_LiveBridge(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live node bridge in -short mode")
	}
	nodeBin, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH; skipping live bridge (JSON-mapping unit tests still cover the hook)")
	}

	// Locate the shipped shell-guard corpus (.opencode) by walking up to the
	// module root. templates/core is the canonical corpus the seam installs.
	modRoot := findModuleRoot(t)
	tmplOpencode := filepath.Join(modRoot, "templates", "core", ".opencode")

	// Stage a scratch install: only the files eval.js pulls in.
	scratch := t.TempDir()
	scratchOpencode := filepath.Join(scratch, ".opencode")
	files := []string{
		"package.json",
		filepath.Join("repo-configs", "allowed-commands.js"),
		filepath.Join("repo-configs", "forbidden-patterns.js"),
		filepath.Join("repo-configs", "forbidden-patterns.core.js"),
		filepath.Join("plugins", "shell-guard-core.js"),
		filepath.Join("plugins", "shell-guard", "eval.js"),
	}
	for _, rel := range files {
		src := filepath.Join(tmplOpencode, filepath.FromSlash(rel))
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("read template %s: %v (template not rendered?)", rel, err)
		}
		dst := filepath.Join(scratchOpencode, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dst, err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			t.Fatalf("write %s: %v", dst, err)
		}
	}

	// Install the WASM deps. If npm is unavailable/offline, skip (the
	// JSON-mapping unit tests still cover the Go hook).
	npmBin, npmErr := exec.LookPath("npm")
	if npmErr != nil {
		t.Skip("npm not on PATH; skipping live bridge")
	}
	install := exec.Command(npmBin, "install", "--no-audit", "--no-fund")
	install.Dir = scratchOpencode
	if out, err := install.CombinedOutput(); err != nil {
		t.Skipf("npm install failed (offline?): %v\n%s", err, out)
	}

	evalPath := filepath.Join(scratchOpencode, "plugins", "shell-guard", "eval.js")

	// 1. allow: echo hello (echo is in COMMANDS.readonly).
	out, code := runNode(t, nodeBin, evalPath, scratch, "echo", "hello")
	if code != 0 {
		t.Fatalf("echo hello: exit %d, stdout=%q stderr-led; expected exit 0", code, out)
	}
	if act := jsonAction(t, out); act != "allow" {
		t.Errorf("echo hello: action %q, want allow (stdout=%q)", act, out)
	}

	// 2. deny: apt-get install foo (matches the apt-install-ad-hoc rule).
	out, code = runNode(t, nodeBin, evalPath, scratch, "apt-get", "install", "foo")
	if code != 0 {
		t.Fatalf("apt-get install: exit %d, expected exit 0 (deny is a decision, not a fault)", code)
	}
	if act := jsonAction(t, out); act != "deny" {
		t.Errorf("apt-get install: action %q, want deny (stdout=%q)", act, out)
	}

	// 3. End-to-end through the Go hook (real osExecRunner) against the scratch
	// install: proves the full Go -> node -> JSON -> Action path.
	h := NewShellGuardHook(scratch)
	if h.bridgeErr != nil {
		t.Fatalf("hook validate failed against scratch install: %v", h.bridgeErr)
	}
	act, _, err := h.Evaluate(context.Background(), []string{"echo", "hello"})
	if err != nil || act != Allow {
		t.Errorf("hook Evaluate(echo hello) = (%s,%v), want (Allow,nil)", act, err)
	}
	act, _, err = h.Evaluate(context.Background(), []string{"apt-get", "install", "foo"})
	if err != nil || act != Deny {
		t.Errorf("hook Evaluate(apt-get install foo) = (%s,%v), want (Deny,nil)", act, err)
	}
}

// findModuleRoot walks up from cwd until it finds go.mod. go test runs with
// cwd = the package source dir (.../internal/permission), so the module root
// (where templates/ lives) is a few parents up.
func findModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for i := 0; i < 20; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("go.mod not found walking up from %s", dir)
	return ""
}

// runNode runs `node evalPath args...` with cwd=scratch and returns (stdout,
// exit code). It fails the test only on a spawn error, not on a non-zero exit
// (eval.js exit 0 for decisions, exit 2 for engine faults — both assertable).
func runNode(t *testing.T, nodeBin, evalPath, cwd string, args ...string) (string, int) {
	t.Helper()
	full := append([]string{evalPath}, args...)
	cmd := exec.Command(nodeBin, full...)
	cmd.Dir = cwd
	var stdout strings.Builder
	cmd.Stdout = &stdout
	var stderr strings.Builder
	cmd.Stderr = &stderr
	// Bound the run so a hung WASM load fails the test instead of hanging CI.
	timer := time.AfterFunc(30*time.Second, func() { _ = cmd.Process.Kill() })
	defer timer.Stop()
	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return stdout.String(), exitErr.ExitCode()
		}
		t.Fatalf("node spawn error: %v\nstderr: %s", err, stderr.String())
	}
	return stdout.String(), 0
}

// jsonAction extracts the "action" field from the single JSON line eval.js
// emits on stdout. Fails the test on malformed output.
func jsonAction(t *testing.T, stdout string) string {
	t.Helper()
	line := strings.TrimSpace(stdout)
	if line == "" {
		t.Fatalf("eval.js produced no stdout; cannot read action")
	}
	// jsonAction is intentionally simple; the hook's json.Unmarshal path is
	// covered separately by the mapping tests.
	var res struct {
		Action string `json:"action"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(line), &res); err != nil {
		t.Fatalf("eval.js stdout not JSON: %q (%v)", line, err)
	}
	return res.Action
}
