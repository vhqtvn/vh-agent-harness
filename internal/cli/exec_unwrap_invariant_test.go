package cli

// exec_unwrap_invariant_test.go pins the latent return-path invariant named by
// defer-027: runExecRo, runExec, and runShell return be.Exec's error UNWRAPPED
// (as a raw *exec.ExitError). The defect-2 exit-code propagation fix
// (exitCodeFromError, 174f5ea3) relies on errors.As resolving a *exec.ExitError
// on the error these handlers return up to cobra's Execute(). If a future
// refactor wraps the error with a NON-%w form or changes its type, the
// *exec.ExitError is no longer discoverable and exit-code propagation silently
// breaks (`vh-agent-harness exec bash -c 'exit 3'` would exit 1 again). This
// guard makes the contract explicit and fails loudly if it is violated.
//
// Sibling exec_exit_code_test.go pins the EXTRACTOR (exitCodeFromError) in
// isolation; this file pins the CALLERS' return-path contract — the precise gap
// defer-027 identified (the extractor test does not prove the handlers return
// the error unwrapped). The %w-chain traversal case is already covered there;
// these tests inject a real *exec.ExitError through the established
// recordingBackend seam and assert each RunE handler surfaces it such that
// errors.As resolves AND the child exit code survives.

import (
	"errors"
	"os/exec"
	"strconv"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/runtime"
)

// realExitError produces a genuine *exec.ExitError (a child that exits `code`)
// the same way the runtime does — cmd.Run on a non-zero exit — so the injected
// recordingBackend returns the exact error shape be.Exec would return. This is
// the same construction used by exec_exit_code_test.go.
func realExitError(t *testing.T, code int) error {
	t.Helper()
	err := exec.Command("bash", "-c", "exit "+strconv.Itoa(code)).Run()
	if err == nil {
		t.Fatalf("setup: expected non-zero exit %d, got nil", code)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("setup: expected *exec.ExitError, got %T: %v", err, err)
	}
	return err
}

// assertUnwrappedExitError is the crux assertion for defer-027. The error a
// RunE handler returns must carry the *exec.ExitError so exitCodeFromError
// (defect-2) can propagate the child exit code. errors.As resolving means the
// return-path invariant holds (the error is returned raw / unwrapped today); a
// non-%w wrap or a type change would fail here. The rec.log check proves the
// backend was actually reached (so the assertion exercises the real return
// path, not an earlier deny).
func assertUnwrappedExitError(t *testing.T, err error, wantCode int, rec *recordingBackend) {
	t.Helper()
	if len(rec.log) == 0 {
		t.Fatalf("backend was never reached (rec.log empty) — cannot test the return path; got err=%v", err)
	}
	if err == nil {
		t.Fatalf("handler returned nil error; expected *exec.ExitError (exit %d)", wantCode)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("errors.As(*exec.ExitError) did not resolve on %T — the return-path unwrapping invariant is BROKEN (defect-2 exit-code propagation would silently fail): %v", err, err)
	}
	if got := exitErr.ExitCode(); got != wantCode {
		t.Errorf("child exit code = %d, want %d (ExitCode must survive the return path)", got, wantCode)
	}
}

// TestRunExec_ReturnsBackendErrorUnwrapped pins runExec's return-path invariant:
// the *exec.ExitError from be.Exec must reach cobra such that errors.As resolves
// it and the exit code survives. allowHook{} routes the command past the
// permission gate to the injected backend (the established allow-reaches-backend
// seam); the backend then returns the canned exit error.
func TestRunExec_ReturnsBackendErrorUnwrapped(t *testing.T) {
	const wantCode = 3
	root := t.TempDir()
	writeFixtureManifest(t, root, "docker_compose", "dev")
	rec := &recordingBackend{name: "docker_compose", err: realExitError(t, wantCode)}
	runtimeCmdDeps.backendFor = func(*loadedManifest) (runtime.Backend, error) { return rec, nil }
	runtimeCmdDeps.hook = allowHook{}
	defer resetRuntimeDeps(t)

	runWithCwd(t, root, func() {
		cmd, _ := newOutCmd()
		err := runExec(cmd, []string{"echo", "hi"})
		assertUnwrappedExitError(t, err, wantCode, rec)
	})
}

// TestRunShell_ReturnsBackendErrorUnwrapped pins runShell's return-path
// invariant (same contract as runExec). allowHook{} admits the implicit shell
// intent; the backend returns the canned exit error.
func TestRunShell_ReturnsBackendErrorUnwrapped(t *testing.T) {
	const wantCode = 7
	root := t.TempDir()
	writeFixtureManifest(t, root, "docker_compose", "dev")
	rec := &recordingBackend{name: "docker_compose", err: realExitError(t, wantCode)}
	runtimeCmdDeps.backendFor = func(*loadedManifest) (runtime.Backend, error) { return rec, nil }
	runtimeCmdDeps.hook = allowHook{}
	defer resetRuntimeDeps(t)

	runWithCwd(t, root, func() {
		cmd, _ := newOutCmd()
		err := runShell(cmd, nil)
		assertUnwrappedExitError(t, err, wantCode, rec)
	})
}

// TestRunExecRo_ReturnsBackendErrorUnwrapped pins runExecRo's return-path
// invariant. exec-ro does NOT route through the permission hook (it is
// allowlisted in opencode.jsonc and gates its own payload via execro.Classify);
// `ls` is in the read-only allowlist, so []string{"ls"} reaches the backend,
// which returns the canned exit error. The rec.log check self-diagnoses a
// classifier-deny (which would skip the backend and make the return path
// untestable) instead of silently passing.
func TestRunExecRo_ReturnsBackendErrorUnwrapped(t *testing.T) {
	const wantCode = 5
	root := t.TempDir()
	writeFixtureManifest(t, root, "docker_compose", "dev")
	rec := &recordingBackend{name: "docker_compose", err: realExitError(t, wantCode)}
	runtimeCmdDeps.backendFor = func(*loadedManifest) (runtime.Backend, error) { return rec, nil }
	defer resetRuntimeDeps(t)

	runWithCwd(t, root, func() {
		cmd, _ := newOutCmd()
		err := runExecRo(cmd, []string{"ls"})
		assertUnwrappedExitError(t, err, wantCode, rec)
	})
}
