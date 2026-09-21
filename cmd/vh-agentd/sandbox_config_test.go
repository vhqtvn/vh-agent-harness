package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/execsandbox"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/subagents"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// TestMain is the test-binary trampoline host, mirroring the daemon's
// run() dispatch: when the sandbox func re-execs the TEST BINARY as
// [testbin, __exec_sandbox_child, --, target, args...], dispatch into
// execsandbox.RunChild instead of running the suite.
//
// It also pins HOME to an empty temp dir for the WHOLE test binary:
// --mcp-config UNSET makes the daemon adopt ~/.config/opencode/
// opencode.json when it exists (the operator feature), which would
// make daemon-level tests machine-dependent on a dev box carrying a
// real opencode config (real servers connected mid-test, advertised
// tool catalog — and therefore prompt-artifact hashes — drifting per
// machine). In-process run() calls and spawned daemon children both
// inherit the pin; the production default is untouched.
func TestMain(m *testing.M) {
	if rest, handled := trampolineArgs(os.Args[1:]); handled {
		if err := execsandbox.RunChild(rest); err != nil {
			fmt.Fprintf(os.Stderr, "sandbox trampoline child: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0) // unreachable on success: syscall.Exec replaced us
	}
	if orig := os.Getenv("HOME"); orig != "" {
		if dir, err := os.MkdirTemp("", "vh-agentd-test-home-"); err == nil {
			defer os.RemoveAll(dir)
			_ = os.Setenv("HOME", dir)
		} else {
			fmt.Fprintf(os.Stderr, "test home isolation: %v\n", err)
		}
	}
	os.Exit(m.Run())
}

// TestValidateSandboxMode pins the --sandbox flag contract: off is the
// default (nil confinement func — pre-slice posture), read-only and
// workspace-write are admitted, and the daemon derives workspace-write
// WritableRoots as session dir + os temp (the documented default).
func TestValidateSandboxMode(t *testing.T) {
	base := func(sandbox string) *Config {
		t.Helper()
		cfg, err := validate("openai", "m", "https://x.example", "KEY_VAR", t.TempDir(), "", 0, defaultApprovalTimeoutMs, 0, sandbox, 65536, "")
		if err != nil {
			t.Fatalf("validate(%q): %v", sandbox, err)
		}
		return cfg
	}

	cfg := base("off")
	if cfg.SandboxMode != "off" || len(cfg.SandboxWritableRoots) != 0 {
		t.Fatalf("off posture wrong: %+v", cfg)
	}
	// An explicitly EMPTY --sandbox is a config bug, not the default:
	// fail closed like every other invalid value (the flag default is
	// the literal "off").
	if _, err := validate("openai", "m", "https://x.example", "KEY_VAR", t.TempDir(), "", 0, defaultApprovalTimeoutMs, 0, "", 65536, ""); err == nil {
		t.Fatalf("validate(\"\") must fail (explicit emptiness is not the off default)")
	}
	cfg = base("read-only")
	if cfg.SandboxMode != "read-only" || len(cfg.SandboxWritableRoots) != 0 {
		t.Fatalf("read-only posture wrong: %+v", cfg)
	}

	tmp := t.TempDir()
	cfg, err := validate("openai", "m", "https://x.example", "KEY_VAR", tmp, "", 0, defaultApprovalTimeoutMs, 0, "workspace-write", 65536, "")
	if err != nil {
		t.Fatalf("validate(workspace-write): %v", err)
	}
	if cfg.SandboxMode != "workspace-write" {
		t.Fatalf("mode = %q", cfg.SandboxMode)
	}
	wantRoots := []string{tmp, os.TempDir()}
	if strings.Join(cfg.SandboxWritableRoots, "|") != strings.Join(wantRoots, "|") {
		t.Fatalf("writable roots = %v, want %v (session dir + os temp)", cfg.SandboxWritableRoots, wantRoots)
	}

	// Refusals: the empty value, the redundant dsh mode and the CLI
	// verb vocabulary.
	for _, bad := range []string{"", "danger-full-access", "best-effort", "strict", "bogus"} {
		if _, err := validate("openai", "m", "https://x.example", "KEY_VAR", t.TempDir(), "", 0, defaultApprovalTimeoutMs, 0, bad, 65536, ""); err == nil {
			t.Fatalf("validate(%q) must fail", bad)
		}
	}
	_, err = validate("openai", "m", "https://x.example", "KEY_VAR", t.TempDir(), "", 0, defaultApprovalTimeoutMs, 0, "danger-full-access", 65536, "")
	if err == nil || !strings.Contains(err.Error(), "redundant with off") {
		t.Fatalf("danger-full-access must be refused as redundant with off: %v", err)
	}
}

// TestRunSandboxFlagInvalidExits2: a bad --sandbox value is a usage
// failure (exit 2), consistent with the other validation failures.
func TestRunSandboxFlagInvalidExits2(t *testing.T) {
	code, _, stderr := runArgs(t, []string{"--adapter", "openai", "--model", "m", "--base-url", "https://x.example",
		"--api-key-env", "K", "--session-dir", t.TempDir(), "--sandbox", "full-access"},
		map[string]string{"K": "v"})
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "danger-full-access") || !strings.Contains(stderr, "redundant with off") {
		t.Fatalf("stderr must carry the mode-vocabulary guidance: %q", stderr)
	}
}

// TestRunTrampolineVerbRouting: the hidden __exec_sandbox_child verb
// is routed to the sandbox trampoline BEFORE flag parsing (the
// trampoline child carries the payload argv, not daemon flags), and
// every other argv falls through to the normal daemon path.
func TestRunTrampolineVerbRouting(t *testing.T) {
	if _, handled := trampolineArgs([]string{}); handled {
		t.Fatalf("empty argv must not be a trampoline invocation")
	}
	if _, handled := trampolineArgs([]string{"--adapter", "openai"}); handled {
		t.Fatalf("flag argv must not be a trampoline invocation")
	}
	rest, handled := trampolineArgs([]string{"__exec_sandbox_child", "--", "/bin/echo", "hi"})
	if !handled {
		t.Fatalf("trampoline verb must be routed")
	}
	if strings.Join(rest, " ") != "/bin/echo hi" {
		t.Fatalf("trampoline payload = %v, want [/bin/echo hi] (verb + -- stripped)", rest)
	}
}

// TestDaemonToolsSandboxWiring: daemonTools wires the sandbox seam from
// the validated config — off yields the nil-Sandbox pre-slice posture,
// read-only yields a confining SandboxFunc whose outcomes carry the
// mode label.
func TestDaemonToolsSandboxWiring(t *testing.T) {
	off, err := validate("openai", "m", "https://x.example", "KEY_VAR", t.TempDir(), "", 0, defaultApprovalTimeoutMs, 0, "off", 65536, "")
	if err != nil {
		t.Fatalf("validate off: %v", err)
	}
	ro, err := validate("openai", "m", "https://x.example", "KEY_VAR", t.TempDir(), "", 0, defaultApprovalTimeoutMs, 0, "read-only", 65536, "")
	if err != nil {
		t.Fatalf("validate read-only: %v", err)
	}

	// off: byte-identical posture — run_shell outcome says "none".
	out := execDaemonRunShell(t, daemonTools(realNow, off, subagents.NewRegistry(), shellConfigFor(off), nil), "printf off-ok")
	if out.Sandbox != "none" || out.Stdout != "off-ok" {
		t.Fatalf("off outcome = %+v, want pre-slice posture (sandbox none)", out)
	}

	// read-only: the sandbox func is armed and labeled (real backend
	// proof lives in internal/tools/shell + the e2e child test; here we
	// assert the WIRING — label + a confined run when the backend
	// exists).
	if !execsandbox.Detect().Available() {
		t.Skipf("landlock+seccomp unavailable; confined wiring exec skipped (fail-closed path covered in internal/tools/shell)")
	}
	out = execDaemonRunShell(t, daemonTools(realNow, ro, subagents.NewRegistry(), shellConfigFor(ro), nil), "printf ro-ok")
	if out.Sandbox != "read-only" {
		t.Fatalf("read-only outcome sandbox = %q, want read-only", out.Sandbox)
	}
	if out.Stdout != "ro-ok" || out.Cause != "exit" {
		t.Fatalf("clean command must still run confined: %+v (stderr=%q)", out, out.Stderr)
	}
}

// execDaemonRunShell finds the run_shell definition in defs and runs
// one command through the real pipeline, decoding the structured
// outcome.
func execDaemonRunShell(t *testing.T, defs []tools.ToolDefinition, command string) daemonOutcome {
	t.Helper()
	p := tools.NewPipeline()
	for _, d := range defs {
		if err := p.Register(d); err != nil {
			t.Fatalf("register %s: %v", d.Name, err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	res := p.Execute(ctx, session.ToolCall{
		ID:   "call-wiring",
		Name: "run_shell",
		Args: json.RawMessage(`{"command":` + strconv.Quote(command) + `}`),
	})
	var out daemonOutcome
	if res.IsError {
		t.Fatalf("run_shell result isError: %+v (%s)", res, res.Content)
	}
	if err := json.Unmarshal([]byte(res.Content), &out); err != nil {
		t.Fatalf("outcome not structured JSON: %v (%s)", err, res.Content)
	}
	return out
}

// daemonOutcome is the decoded run_shell outcome (the fields the
// wiring assertions need).
type daemonOutcome struct {
	Cause    string `json:"cause"`
	ExitCode int    `json:"exitCode"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Sandbox  string `json:"sandbox"`
}
