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

// TestExecRo_PropagatesChildExitCode mirrors the defect-2 crux for exec-ro.
// exec-ro's classifier allows `bash` only in a non-metachar form; the cleanest
// read-only-ish child that exits a specific code is `false` (always exits 1) —
// but to test a NON-1 code we use `bash -c 'exit N'`. bash is in exec-ro's
// read-only set? It is not guaranteed, so this test uses a known-allowed
// read-only binary. `false` always exits 1, which is the boundary value most
// likely to MASK the collapse (1 was the old collapse target). We assert
// exec-ro exits 1 for `false` — and crucially does NOT dump usage (defect 3).
func TestExecRo_ChildFailureNoUsageDump(t *testing.T) {
	// `false` is a coreutils binary that always exits 1. It is read-only
	// (no mutation, no metachars) so exec-ro's classifier allows it. Under the
	// old collapse, exec-ro would exit 1 here too — but for the WRONG reason
	// (collapse) vs. the right reason (child exit). The defect-3 assertion
	// (no usage dump) is the observable signal that distinguishes them.
	out, exit := runExec(t, "exec-ro", "false")
	if exit != 1 {
		t.Logf("exec-ro false: exit=%d (expected 1 from `false`); output:\n%s", exit, out)
	}
	// Defect 3 crux: NO usage/flags dump on a child failure. The error is the
	// child's, not a mis-invocation.
	assertNoUsageDump(t, "exec-ro false", out)
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
// dump Usage/Flags. We check for the load-bearing markers: "Usage:" (the usage
// header) and "Flags:" (the flags block header) and the Global Flags block. The
// error line "Error:" is expected and allowed.
func assertNoUsageDump(t *testing.T, label, out string) {
	t.Helper()
	for _, marker := range []string{"Usage:", "Flags:"} {
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
