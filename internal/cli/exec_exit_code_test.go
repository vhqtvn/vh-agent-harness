package cli

// This file is the regression guard for the exec-family child-exit-code
// propagation fix (defect 2). Before the fix, `vh-agent-harness exec bash -c
// 'exit 3'` exited 1: the *exec.ExitError from runtime.Runner.Run -> cmd.Run
// reached Execute() via the RunE return, but Execute() collapsed every error to
// os.Exit(1). The fix introduces exitCodeFromError, which propagates the real
// child exit code when the error carries one (a *exec.ExitError) and falls back
// to 1 for everything else.
//
// This unit test pins the extractor's logic directly (fast, deterministic),
// including the load-bearing guarantee that errSilent is preserved (it is NOT a
// child exit, so it must keep exiting 1, never be misread as a child code).
// The end-to-end behavior (real binary, real $?) is proven by the integration
// test at tests/integration/exec_exit_code_test.go.

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"testing"
)

// TestExitCodeFromError_PropagatesChildExitCode is the crux unit guard: an
// *exec.ExitError carries the child's real exit code, and the extractor must
// return it verbatim instead of collapsing to 1. This is the defect-2
// regression — `exec bash -c 'exit 3'` must surface 3, `exit 7` must surface 7.
func TestExitCodeFromError_PropagatesChildExitCode(t *testing.T) {
	cases := []struct {
		name string
		code int
	}{
		{"exit 3", 3},
		{"exit 7", 7},
		{"exit 42", 42},
		{"exit 1", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Produce a REAL *exec.ExitError the same way the runtime does
			// (cmd.Run on a non-zero exit). This is the exact error shape that
			// be.Exec returns up through runExec/runExecRo/runShell to Execute().
			err := exec.Command("bash", "-c", "exit "+strconv.Itoa(tc.code)).Run()
			if err == nil {
				t.Fatalf("setup: expected non-zero exit, got nil")
			}
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("setup: expected *exec.ExitError, got %T: %v", err, err)
			}
			got := exitCodeFromError(err)
			if got != tc.code {
				t.Errorf("exitCodeFromError(child exit %d) = %d, want %d", tc.code, got, tc.code)
			}
		})
	}
}

// TestExitCodeFromError_PropagatesThroughPercentWChain proves errors.As
// traverses a wrapped error chain. runExec/runExecRo/runShell currently return
// be.Exec's error UNWRAPPED, but if a future change wraps it
// (fmt.Errorf("exec: %w", be.Exec(...))), or cobra wraps the RunE error, the
// extractor must still find the *exec.ExitError at the central exit point.
func TestExitCodeFromError_PropagatesThroughPercentWChain(t *testing.T) {
	inner := exec.Command("bash", "-c", "exit 5").Run()
	if inner == nil {
		t.Fatalf("setup: expected non-zero exit, got nil")
	}
	// A real %w chain — errors.As traverses it and finds the *exec.ExitError.
	chainWrapped := fmt.Errorf("exec pipeline: %w", inner)
	got := exitCodeFromError(chainWrapped)
	if got != 5 {
		t.Errorf("exitCodeFromError(%%w-wrapped exit error) = %d, want 5 (errors.As must traverse the chain)", got)
	}
	// A NON-%w wrap (plain string concat) is NOT an error chain: errors.As
	// cannot find the *exec.ExitError, so the extractor collapses to 1. This
	// documents the boundary — callers must use %w to preserve exit-code
	// propagation across wrapping.
	plainWrapped := errors.New("exec: " + inner.Error())
	if got := exitCodeFromError(plainWrapped); got != 1 {
		t.Errorf("exitCodeFromError(non-%%w wrap) = %d, want 1 (string concat is not an error chain)", got)
	}
}

// TestExitCodeFromError_PreservesErrSilent is the load-bearing errSilent-
// preservation guard. errSilent is the no-message sentinel used by diff/doctor/
// help-migrate to force a non-zero exit WITHOUT cobra printing "Error:". The
// defect-2 fix MUST NOT misread errSilent as a child exit: it is not an
// *exec.ExitError, so it must keep exiting 1. This pins the coexistence
// contract documented in exitCodeFromError's doc comment.
func TestExitCodeFromError_PreservesErrSilent(t *testing.T) {
	if got := exitCodeFromError(errSilent{}); got != 1 {
		t.Errorf("exitCodeFromError(errSilent{}) = %d, want 1 (errSilent must NOT be misread as a child exit)", got)
	}
	// errSilent wrapped in a %w chain must also still exit 1: errors.As finds
	// errSilent (which it is), never an *exec.ExitError (which it is not).
	wrappedSilent := fmt.Errorf("doctor: %w", errSilent{})
	if got := exitCodeFromError(wrappedSilent); got != 1 {
		t.Errorf("exitCodeFromError(%%w-wrapped errSilent) = %d, want 1", got)
	}
}

// TestExitCodeFromError_NonExitErrorsCollapseTo1 covers the remaining
// non-exit-error paths: a genuine runtime error (hook failure, permission
// denial), a nil error, and a binary-not-found *exec.Error must all fall
// through to the documented default (1 for errors, 0 for nil). *exec.Error is
// distinct from *exec.ExitError and must also exit 1.
func TestExitCodeFromError_NonExitErrorsCollapseTo1(t *testing.T) {
	if got := exitCodeFromError(nil); got != 0 {
		t.Errorf("exitCodeFromError(nil) = %d, want 0", got)
	}
	if got := exitCodeFromError(errors.New("permission hook error")); got != 1 {
		t.Errorf("exitCodeFromError(generic error) = %d, want 1", got)
	}
	// *exec.Error (e.g. binary not found) is a DIFFERENT type from
	// *exec.ExitError; errors.As must not match it, so it collapses to 1.
	binErr := exec.Command("/nonexistent-binary-xyz").Run()
	if binErr == nil {
		t.Skip("unexpected: nonexistent binary ran cleanly")
	}
	if got := exitCodeFromError(binErr); got != 1 {
		t.Errorf("exitCodeFromError(*exec.Error) = %d, want 1 (binary-not-found is not a child exit)", got)
	}
}
