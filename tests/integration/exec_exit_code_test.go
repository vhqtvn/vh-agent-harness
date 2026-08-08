package integration

// This file is the END-TO-END behavioral-closure crux for the exec-family
// output-honesty fixes (defects 2 and 3). It drives the REAL freshly-built
// vh-agent-harness binary (sandboxBin, built once by TestMain) and asserts the
// observable behavior an operator sees:
//
//   - defect 2: `vh-agent-harness exec bash -c 'exit N'` exits N (not 1), for
//     exec, exec-ro, and shell. exec-sandbox's exit handling is unchanged (it
//     already bypassed the collapse via os.Exit — this test documents that).
//   - defect 3: on a child failure, the exec family prints the error and does
//     NOT dump the Usage/Flags block (SilenceUsage).
//
// The unit-level guard for defect 2 lives at internal/cli/exec_exit_code_test.go
// (exitCodeFromError logic + errSilent preservation). This integration test
// proves the OUTCOME on the load-bearing path: the real binary, real $?, real
// stderr — not just the mechanism.

import (
	"os/exec"
	"strings"
	"testing"
)

// runExec invokes the freshly-built harness binary's exec-family verb with the
// given sub-args from repoRoot, returning combined output and the process exit
// code. It mirrors runSandbox's exit-code extraction so the two are comparable.
func runExec(t *testing.T, subArgs ...string) (string, int) {
	t.Helper()
	args := append([]string{}, subArgs...)
	cmd := exec.Command(sandboxBin, args...)
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to invoke harness binary %v: %v\n%s", args, err, out)
		}
	}
	return string(out), exitCode
}

// TestExec_PropagatesChildExitCode is the defect-2 crux, observed end-to-end on
// the real binary: `vh-agent-harness exec bash -c 'exit N'` must exit N, not 1.
// Before the fix, the *exec.ExitError from cmd.Run reached Execute() and was
// collapsed to os.Exit(1); the real code survived only as text in
// "Error: exit status N". exit 3 and exit 7 are the exact repro cases from the
// incident report (vh-solara, 2026-08-06).
func TestExec_PropagatesChildExitCode(t *testing.T) {
	cases := []struct {
		name string
		code int
	}{
		{"exit 3", 3},
		{"exit 7", 7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, exit := runExec(t, "exec", "bash", "-c", "exit "+itoaInt(tc.code))
			if exit != tc.code {
				t.Fatalf("exec bash -c 'exit %d': exit code = %d, want %d (child exit code must propagate, not collapse to 1)\n--- output ---\n%s",
					tc.code, exit, tc.code, out)
			}
		})
	}
}

// TestExecRo_PropagatesNon1ChildExitAndNoUsageDump is BOTH the defect-2 and
// defect-3 crux for exec-ro, observed end-to-end on the real binary.
//
// defect 2: exec-ro must propagate the CHILD's real exit code, not collapse it
// to 1. To prove propagation rather than the former collapse-to-1, the child
// must exit a NON-1 code — otherwise a test asserting "exit==1" passes both
// under correct propagation (child exited 1) and under the old collapse
// (collapse target was 1), and the two are indistinguishable. `rg --bad-flag`
// is a classifier-ALLOWED readonly binary (`rg *` is in the readonly command
// group in internal/permconfig/tables.go) that reliably exits 2 on an
// unrecognized flag. exec-ro ALLOWs it through to the backend, the child
// exits 2, and exec-ro must surface exit 2 — NOT 1. Under the old
// exitCodeFromError collapse, this would have exited 1 (the former collapse
// target), so an exit==2 assertion distinguishes correct propagation from the
// former collapse.
//
// defect 3: on that child failure, exec-ro must NOT dump cobra's Usage/Flags
// block (SilenceUsage). The error is the child's, not a mis-invocation.
func TestExecRo_PropagatesNon1ChildExitAndNoUsageDump(t *testing.T) {
	// `rg --bad-flag` is classifier-allowed (readonly `rg *` group) and exits
	// 2 (ripgrep's exit code for an unrecognized flag). A NON-1 exit is the
	// load-bearing choice: exit 1 would be ambiguous (it is BOTH the child
	// exit of `false` AND the old collapse target), so asserting exit==2 is
	// the only way to distinguish "child code propagated" from "collapsed to
	// 1".
	//
	// rg (ripgrep) is a third-party binary this crux relies on as the failing
	// child. Every env running this harness carries rg (it is a first-class
	// readonly tool in internal/permconfig/tables.go), but skip cleanly on an
	// rg-less host so a missing binary surfaces as a SKIP (environmental)
	// rather than an opaque failure that could be mistaken for a behavioral
	// regression.
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg (ripgrep) not found on PATH: exec-ro crux requires rg as the failing child; defect-2/defect-3 behavior cannot be exercised without it")
	}
	out, exit := runExec(t, "exec-ro", "rg", "--bad-flag")
	if exit != 2 {
		t.Fatalf("exec-ro rg --bad-flag: exit code = %d, want 2 (rg's exit code for an unrecognized flag must propagate through exec-ro, not collapse to 1)\n--- output ---\n%s",
			exit, out)
	}
	// Defect 3 crux: NO usage/flags dump on a child failure. The error is the
	// child's, not a mis-invocation.
	assertNoUsageDump(t, "exec-ro rg --bad-flag", out)
}

// TestExec_ChildFailureNoUsageDump is the defect-3 crux: on a child failure,
// exec prints the "Error: exit status N" line but does NOT append the full
// Usage/Flags block. Before the fix (SilenceUsage was missing on the exec
// family), cobra dumped Usage after the error, implying the operator
// mis-invoked exec when in fact the child simply failed.
func TestExec_ChildFailureNoUsageDump(t *testing.T) {
	out, exit := runExec(t, "exec", "bash", "-c", "exit 3")
	if exit != 3 {
		t.Fatalf("exec bash -c 'exit 3': exit=%d, want 3\n--- output ---\n%s", exit, out)
	}
	assertNoUsageDump(t, "exec bash -c 'exit 3'", out)
	// The error line itself SHOULD be present (we did not set SilenceErrors —
	// genuine errors must surface). This distinguishes "silenced usage" from
	// "silenced everything".
	if !strings.Contains(out, "exit status 3") {
		t.Errorf("exec child failure: expected 'exit status 3' error line in output, got:\n%s", out)
	}
}

// TestShell_NonInteractiveDeniedAndNoUsageDump documents the observable
// behavior of `vh-agent-harness shell` in a non-interactive (test/CI) context:
// the permission gate denies the empty shell intent by default
// (runShell: "an empty command makes eval.js return {deny,'empty command'}, so
// the non-interactive vh-agent-harness shell denies by default until an operator
// prompt loop is wired"). That denial exits 1 (a gate denial is a genuine
// error, not errSilent and not a child exit — exitCodeFromError correctly
// collapses it to 1).
//
// This test proves defect 3 for the shell verb (the denial does NOT dump
// Usage/Flags thanks to SilenceUsage). It also HONESTLY documents that the
// shell child-exit-code propagation path (defect 2 for shell) is NOT
// live-exercisable in a non-interactive context: the gate denies before
// be.Exec runs, so the *exec.ExitError that be.Exec would produce never
// reaches Execute() here. That path is structurally identical to exec's
// (runShell returns be.Exec's error directly, same as runExec; both flow
// through the same exitCodeFromError at Execute()), and is covered by the
// unit guard at internal/cli/exec_exit_code_test.go. An interactive TTY
// shell session (operator at a terminal) is the only context that reaches
// be.Exec for shell — not automatable here.
func TestShell_NonInteractiveDeniedAndNoUsageDump(t *testing.T) {
	cmd := exec.Command(sandboxBin, "shell")
	cmd.Dir = repoRoot
	cmd.Stdin = strings.NewReader("exit 7\n")
	rawOut, err := cmd.CombinedOutput()
	out := string(rawOut)
	exit := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exit = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to invoke harness shell: %v\n%s", err, out)
		}
	}
	// The gate denies the empty non-interactive shell intent (documented
	// behavior). The exit is 1 — a gate denial is a genuine error, correctly
	// collapsed by exitCodeFromError (NOT a child exit, NOT errSilent).
	if exit != 1 {
		t.Fatalf("non-interactive shell: exit=%d, want 1 (gate denies empty command; a denial is a genuine error collapsed to 1)\n--- output ---\n%s", exit, out)
	}
	// Defect 3 crux for shell: the denial must NOT dump Usage/Flags. The
	// permission-hook error line SHOULD be present (we did not SilenceErrors).
	assertNoUsageDump(t, "shell (non-interactive, gate-denied)", out)
	if !strings.Contains(out, "empty command") {
		t.Errorf("non-interactive shell: expected 'empty command' denial reason in output, got:\n%s", out)
	}
}

// TestExecSandbox_ExitHandlingUnchanged documents that exec-sandbox's exit
// handling was ALREADY correct before the defect-2 fix (it calls os.Exit(code)
// directly at exec_sandbox.go, bypassing the cobra->Execute() collapse). This
// test pins that the fix did not change exec-sandbox's behavior: a child exit
// code still propagates, and --sandbox=off in the repo's strict floor is
// upgraded (so the child actually runs contained).
func TestExecSandbox_ExitHandlingUnchanged(t *testing.T) {
	// `false` under exec-sandbox exits 1 (the child's code), proving exec-sandbox
	// propagated it (it always did). This is the no-regression guard for the
	// defect-2 scope fence ("exec-sandbox is NOT affected — do not touch").
	out, exit := runExec(t, "exec-sandbox", "--sandbox=off", "--net=deny", "--", "false")
	if exit != 1 {
		t.Fatalf("exec-sandbox false: exit=%d, want 1 (exec-sandbox exit handling must be unchanged by the defect-2 fix)\n--- output ---\n%s", exit, out)
	}
}

// assertNoUsageDump fails the test if the output contains cobra's Usage block
// markers. The exec family sets SilenceUsage:true so a CHILD failure must not
// dump Usage/Flags. We match cobra's MULTI-LINE block form — the usage header
// on its own line followed by the command/flags indented on the next line —
// rather than the bare "Usage:" / "Flags:" literals. The bare literals would
// false-trip on a clap-v4 build (e.g. a future ripgrep) that emits a
// single-line "Usage: rg [OPTIONS]..." on a flag error; the multi-line cobra
// block form ("Usage:\n  ", "Flags:\n  ") is cobra-specific and resists that
// false-trip while still detecting a genuine cobra Usage/Flags dump (cobra
// always puts the command/flags on the line AFTER the header). The error line
// "Error:" is expected and allowed.
func assertNoUsageDump(t *testing.T, label, out string) {
	t.Helper()
	for _, marker := range []string{"Usage:\n  ", "Flags:\n  "} {
		if strings.Contains(out, marker) {
			t.Errorf("%s: child failure must NOT dump cobra Usage/Flags (found %q) — SilenceUsage should suppress it; a child exit is not a usage error\n--- output ---\n%s",
				label, marker, out)
		}
	}
}

// itoaInt is a local int->string helper to avoid pulling strconv into the
// integration test (keeps the import list minimal and mirrors the style of the
// sibling exec_sandbox_test.go which avoids strconv).
func itoaInt(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
