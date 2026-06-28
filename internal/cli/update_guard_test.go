package cli

// Tests for the interactive uninitialized-target confirmation guard on `update`
// (internal/cli/update.go). The guard is wired through two injectable seams —
// updateStdinIsTTY and updateConfirm — plus the existing updateForce /
// updateDryRun / updateTargetFlag package vars, so every prompt-fires and bypass
// path is exercisable without a real TTY or os.Stdin. Each test restores the
// seam vars via t.Cleanup so no state leaks into sibling tests.

import (
	"io"
	"path/filepath"
	"strings"
	"testing"
)

// runUpdateTarget runs runUpdate against target (via the --target flag seam),
// capturing stdout/stderr in a buffer. It does NOT touch the guard seam vars;
// the caller configures updateStdinIsTTY / updateConfirm / updateForce /
// updateDryRun first.
func runUpdateTarget(t *testing.T, target string) (string, error) {
	t.Helper()
	saved := updateTargetFlag
	t.Cleanup(func() { updateTargetFlag = saved })
	updateTargetFlag = target
	cmd, buf := newOutCmd()
	err := runUpdate(cmd, []string{})
	return buf.String(), err
}

// withGuardSeams sets the interactive-guard seam vars for the test and restores
// their prior values on cleanup, so no test leaks guard state into siblings.
func withGuardSeams(t *testing.T, tty func() bool, confirm func(io.Writer, string) bool) {
	t.Helper()
	savedTTY, savedConfirm := updateStdinIsTTY, updateConfirm
	t.Cleanup(func() {
		updateStdinIsTTY = savedTTY
		updateConfirm = savedConfirm
	})
	updateStdinIsTTY = tty
	updateConfirm = confirm
}

// withForce / withDryRun set the matching flag seam for the test and restore it
// on cleanup.
func withForce(t *testing.T, v bool) {
	t.Helper()
	saved := updateForce
	t.Cleanup(func() { updateForce = saved })
	updateForce = v
}

func withDryRun(t *testing.T, v bool) {
	t.Helper()
	saved := updateDryRun
	t.Cleanup(func() { updateDryRun = saved })
	updateDryRun = v
}

// confirmMustNotFire returns a confirm seam that fails the test if the guard
// ever reaches the prompt. Used to prove a bypass path skips the prompt
// entirely.
func confirmMustNotFire(t *testing.T) func(io.Writer, string) bool {
	t.Helper()
	return func(_ io.Writer, _ string) bool {
		t.Fatal("update guard: confirmation prompt fired when it must be bypassed")
		return false
	}
}

// profilePath returns the absolute harness-profile path under target.
func profilePath(target string) string {
	return filepath.Join(target, harnessProfileName)
}

// --- prompt fires: decline -------------------------------------------------

// TestUpdateGuard_UninitializedInteractiveDecline: an uninitialized target
// (no profile) + interactive TTY + no bypass -> the prompt fires; on decline
// update returns nil (exit 0) and writes nothing.
func TestUpdateGuard_UninitializedInteractiveDecline(t *testing.T) {
	target := t.TempDir() // uninitialized: no profile
	prompted := false
	withGuardSeams(t,
		func() bool { return true },                                        // interactive TTY
		func(_ io.Writer, _ string) bool { prompted = true; return false }, // decline
	)
	out, err := runUpdateTarget(t, target)
	if err != nil {
		t.Fatalf("decline must return nil error (exit 0); got %v (out=%q)", err, out)
	}
	if !prompted {
		t.Fatal("guard must fire the prompt on an uninitialized interactive target")
	}
	if !strings.Contains(out, "No changes made.") {
		t.Errorf("decline must print the no-changes message; got %q", out)
	}
	// Zero writes: profile and .opencode must NOT exist.
	if pathExists(t, profilePath(target)) {
		t.Error("decline wrote the harness profile; expected zero writes")
	}
	if pathExists(t, filepath.Join(target, ".opencode")) {
		t.Error("decline wrote .opencode/; expected zero writes")
	}
}

// --- prompt fires: accept --------------------------------------------------

// TestUpdateGuard_UninitializedInteractiveAccept: an uninitialized target +
// interactive TTY + no bypass -> the prompt fires; on accept update proceeds
// and scaffolds managed files (current adopt behavior).
func TestUpdateGuard_UninitializedInteractiveAccept(t *testing.T) {
	target := t.TempDir()
	prompted := false
	withGuardSeams(t,
		func() bool { return true },                                       // interactive TTY
		func(_ io.Writer, _ string) bool { prompted = true; return true }, // accept
	)
	out, err := runUpdateTarget(t, target)
	if err != nil {
		t.Fatalf("accept must proceed cleanly; got %v (out=%q)", err, out)
	}
	if !prompted {
		t.Fatal("guard must fire the prompt on an uninitialized interactive target")
	}
	// Accept proceeds: managed files scaffolded, including the profile.
	if !pathExists(t, profilePath(target)) {
		t.Errorf("accept must scaffold the harness profile; out=%q", out)
	}
	if !pathExists(t, filepath.Join(target, ".opencode")) {
		t.Errorf("accept must scaffold .opencode/; out=%q", out)
	}
}

// --- bypass: non-TTY -------------------------------------------------------

// TestUpdateGuard_UninitializedNonTTYProceeds: an uninitialized target reached
// via a non-TTY stdin (pipes, agents, CI, make update, /harness) must NOT prompt
// and must proceed with current behavior.
func TestUpdateGuard_UninitializedNonTTYProceeds(t *testing.T) {
	target := t.TempDir()
	withGuardSeams(t,
		func() bool { return false }, // non-TTY
		confirmMustNotFire(t),
	)
	out, err := runUpdateTarget(t, target)
	if err != nil {
		t.Fatalf("non-TTY must proceed cleanly; got %v (out=%q)", err, out)
	}
	if !pathExists(t, profilePath(target)) {
		t.Errorf("non-TTY must scaffold the profile (current adopt behavior); out=%q", out)
	}
}

// --- bypass: RUN_FROM_AGENT=1 ---------------------------------------------

// TestUpdateGuard_UninitializedRunFromAgentProceeds: RUN_FROM_AGENT=1 bypasses
// the prompt even when stdin looks interactive, so agents stay frictionless.
func TestUpdateGuard_UninitializedRunFromAgentProceeds(t *testing.T) {
	target := t.TempDir()
	t.Setenv("RUN_FROM_AGENT", "1")
	withGuardSeams(t,
		func() bool { return true }, // interactive, but env bypass wins
		confirmMustNotFire(t),
	)
	out, err := runUpdateTarget(t, target)
	if err != nil {
		t.Fatalf("RUN_FROM_AGENT=1 must proceed cleanly; got %v (out=%q)", err, out)
	}
	if !pathExists(t, profilePath(target)) {
		t.Errorf("RUN_FROM_AGENT=1 must scaffold the profile; out=%q", out)
	}
}

// --- bypass: --force -------------------------------------------------------

// TestUpdateGuard_UninitializedForceProceeds: --force (-f) bypasses the prompt
// even when stdin looks interactive.
func TestUpdateGuard_UninitializedForceProceeds(t *testing.T) {
	target := t.TempDir()
	withForce(t, true)
	withGuardSeams(t,
		func() bool { return true }, // interactive, but force wins
		confirmMustNotFire(t),
	)
	out, err := runUpdateTarget(t, target)
	if err != nil {
		t.Fatalf("--force must proceed cleanly; got %v (out=%q)", err, out)
	}
	if !pathExists(t, profilePath(target)) {
		t.Errorf("--force must scaffold the profile; out=%q", out)
	}
}

// --- bypass: --dry-run -----------------------------------------------------

// TestUpdateGuard_UninitializedDryRunNoPromptNoWrites: --dry-run never prompts
// (it writes nothing) and performs no writes — safe to run anywhere.
func TestUpdateGuard_UninitializedDryRunNoPromptNoWrites(t *testing.T) {
	target := t.TempDir()
	withDryRun(t, true)
	withGuardSeams(t,
		func() bool { return true }, // interactive, but dry-run writes nothing
		confirmMustNotFire(t),
	)
	out, err := runUpdateTarget(t, target)
	if err != nil {
		t.Fatalf("--dry-run must preview cleanly; got %v (out=%q)", err, out)
	}
	// Dry run writes nothing.
	if pathExists(t, profilePath(target)) {
		t.Errorf("--dry-run must NOT write the profile; out=%q", out)
	}
	if pathExists(t, filepath.Join(target, ".opencode")) {
		t.Errorf("--dry-run must NOT write .opencode/; out=%q", out)
	}
}

// --- initialized: never prompts -------------------------------------------

// TestUpdateGuard_InitializedNeverPrompts: a target that carries the harness
// profile is never prompted, regardless of TTY / flags (current behavior
// preserved). Uses the canonical seam-install fixture so the full initialized
// tree is present.
func TestUpdateGuard_InitializedNeverPrompts(t *testing.T) {
	target := t.TempDir()
	seamInstallInto(t, target) // creates the profile = initialized
	withGuardSeams(t,
		func() bool { return true }, // interactive
		confirmMustNotFire(t),
	)
	// No force, no RUN_FROM_AGENT, dry-run off — yet the profile-present check
	// must keep the guard from firing.
	out, err := runUpdateTarget(t, target)
	if err != nil {
		t.Fatalf("initialized target must update cleanly; got %v (out=%q)", err, out)
	}
	if !pathExists(t, profilePath(target)) {
		t.Errorf("profile must remain after update; out=%q", out)
	}
}

// --- --target resolves before the guard -----------------------------------

// TestUpdateGuard_TargetFlagAtUninitializedUsesResolvedTarget: when --target
// points at an uninitialized directory, the guard fires against THAT resolved
// target — the prompt names its absolute path and decline writes nothing there.
func TestUpdateGuard_TargetFlagAtUninitializedUsesResolvedTarget(t *testing.T) {
	target := t.TempDir() // uninitialized
	var promptedAt string
	withGuardSeams(t,
		func() bool { return true },
		func(_ io.Writer, got string) bool { promptedAt = got; return false }, // decline
	)
	out, err := runUpdateTarget(t, target)
	if err != nil {
		t.Fatalf("decline must return nil; got %v (out=%q)", err, out)
	}
	abs, aerr := filepath.Abs(target)
	if aerr != nil {
		t.Fatalf("resolve expected target: %v", aerr)
	}
	if promptedAt != abs {
		t.Errorf("prompt must name the resolved --target abs path; got %q want %q", promptedAt, abs)
	}
	if pathExists(t, profilePath(target)) {
		t.Error("decline wrote the profile; expected zero writes")
	}
}
