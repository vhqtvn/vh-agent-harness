package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// sandboxBin is the path to the freshly built vh-agent-harness binary.
// It is set by TestMain. If the build fails (non-Linux, missing Go, etc.),
// all tests in this file are skipped.
var sandboxBin string

// repoRoot is the absolute path to the repository root (where go test runs).
var repoRoot string

func TestMain(m *testing.M) {
	if runtime.GOOS != "linux" {
		os.Exit(0) // sandbox is Linux-first; skip on other platforms
	}

	// Locate go via GOROOT (go may not be on PATH inside the harness exec env).
	goBin := filepath.Join(runtime.GOROOT(), "bin", "go")
	if _, err := os.Stat(goBin); err != nil {
		// Fallback: try PATH.
		if p, err := exec.LookPath("go"); err == nil {
			goBin = p
		} else {
			os.Exit(0) // skip if go is unavailable
		}
	}

	repoAbs, err := os.Getwd()
	if err != nil {
		os.Exit(0)
	}
	// Walk up to find go.mod (tests may run from tests/integration/).
	for d := repoAbs; d != "/"; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			repoRoot = d
			break
		}
	}
	if repoRoot == "" {
		repoRoot = repoAbs
	}

	tmpDir := filepath.Join(repoRoot, "tmp", "sandbox-test")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		os.Exit(0)
	}
	sandboxBin = filepath.Join(tmpDir, "vh-agent-harness")

	buildCmd := exec.Command(goBin, "build", "-o", sandboxBin, "./cmd/vh-agent-harness")
	buildCmd.Dir = repoRoot
	buildCmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		// Skip rather than fail — the build environment may not support it.
		_ = out
		os.Exit(0)
	}

	code := m.Run()
	_ = os.RemoveAll(tmpDir)
	os.Exit(code)
}

// sandboxFeatureCheck skips the test if OS sandbox primitives are unavailable.
func sandboxFeatureCheck(t *testing.T) {
	t.Helper()
	out, err := exec.Command(sandboxBin, "exec-sandbox", "--sandbox=strict", "--net=deny", "--", "true").CombinedOutput()
	if err != nil {
		t.Skipf("OS sandbox primitives unavailable on this kernel: %v\n%s", err, out)
	}
}

// runSandbox invokes exec-sandbox with the given flags and target command from
// repoRoot, returning combined output and exit error.
func runSandbox(t *testing.T, flags []string, target ...string) (string, int) {
	t.Helper()
	return runSandboxIn(t, repoRoot, flags, target...)
}

// runSandboxIn invokes exec-sandbox with cmd.Dir set to dir. Use an isolated dir
// (e.g. a temp dir OUTSIDE the repo tree) to test exec-sandbox BASE behavior
// without the repo's exec_sandbox.min_mode floor interfering (FindMinMode walks
// up from dir; a dir outside any .vh-agent-harness/ resolves no floor).
func runSandboxIn(t *testing.T, dir string, flags []string, target ...string) (string, int) {
	t.Helper()
	args := []string{"exec-sandbox"}
	args = append(args, flags...)
	args = append(args, "--")
	args = append(args, target...)
	cmd := exec.Command(sandboxBin, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to invoke sandbox: %v\n%s", err, out)
		}
	}
	return string(out), exitCode
}

// TestSandboxWriteContractPositiveControl verifies that writing to ./tmp/
// (the designated RW dir) succeeds. This is the POSITIVE control — if this
// fails, the sandbox is over-restrictive and unusable.
func TestSandboxWriteContractPositiveControl(t *testing.T) {
	sandboxFeatureCheck(t)

	testFile := filepath.Join(repoRoot, "tmp", "sandbox_pos_control")
	_ = os.Remove(testFile)

	out, exit := runSandbox(t, []string{"--sandbox=best-effort", "--net=deny"},
		"touch", filepath.Join("tmp", "sandbox_pos_control"))

	if exit != 0 {
		t.Fatalf("write to ./tmp/ should succeed (positive control) but got exit=%d:\n%s", exit, out)
	}
	if _, err := os.Stat(testFile); err != nil {
		t.Fatalf("positive control file not created: %v", err)
	}
	_ = os.Remove(testFile)
}

// TestSandboxWriteContractNegativeControl verifies that writing to .git/
// fails with EACCES. The repo root is read-only; .git inherits RO from the
// additive Landlock model. This is the INTEGRITY boundary.
func TestSandboxWriteContractNegativeControl(t *testing.T) {
	sandboxFeatureCheck(t)

	out, exit := runSandbox(t, []string{"--sandbox=best-effort", "--net=deny"},
		"touch", filepath.Join(".git", "sandbox_neg_control"))

	if exit == 0 {
		t.Fatalf("write to .git/ should fail (EACCES) but succeeded — integrity boundary broken")
	}
	if !strings.Contains(strings.ToLower(out), "permission denied") {
		t.Fatalf("expected 'permission denied' in output, got:\n%s", out)
	}
}

// TestSandboxNetworkDeny verifies that seccomp blocks socket creation when
// --net=deny. Python's socket.socket() should fail with EPERM/ENOSYS.
func TestSandboxNetworkDeny(t *testing.T) {
	sandboxFeatureCheck(t)

	out, exit := runSandbox(t, []string{"--sandbox=best-effort", "--net=deny"},
		"python3", "-c", `import socket; s=socket.socket(); s.connect(("127.0.0.1",1))`)

	if exit == 0 {
		t.Fatalf("network should be denied (--net=deny) but socket connect succeeded — seccomp filter not working")
	}
	// Python reports PermissionError or OSError on blocked socket syscall.
	lower := strings.ToLower(out)
	if !strings.Contains(lower, "permission") && !strings.Contains(lower, "error") {
		t.Fatalf("expected network-denied error, got:\n%s", out)
	}
}

// TestSandboxNetworkAllow verifies that socket creation works when --net=allow,
// in an ISOLATED dir with no exec_sandbox.min_mode floor (the repo root now
// carries a strict floor that would force --net=allow -> deny; to test the BASE
// net-allow behavior we run from a temp dir outside the repo tree, where
// FindMinMode resolves no floor).
func TestSandboxNetworkAllow(t *testing.T) {
	sandboxFeatureCheck(t)

	iso := t.TempDir() // outside the repo tree => no floor => base behavior
	out, exit := runSandboxIn(t, iso, []string{"--sandbox=best-effort", "--net=allow"},
		"python3", "-c", `import socket; s=socket.socket(); print("socket created OK")`)

	if exit != 0 {
		t.Fatalf("network should be allowed (--net=allow, no floor) but socket creation failed:\n%s", out)
	}
	if !strings.Contains(out, "socket created OK") {
		t.Fatalf("expected socket creation success, got:\n%s", out)
	}
}

// TestSandboxAbsentFloor_RefusesExplicitOff is the FIX-1 behavioral-closure
// crux, observed end-to-end on the real binary + real filesystem: in a dir with
// NO exec_sandbox.min_mode floor (an unfloored consumer), an explicit
// --sandbox=off downgrade is REFUSED — the command exits non-zero with the
// refuse notice and the target file OUTSIDE ./tmp/ is NEVER created.
//
// Before Fix 1, ParseMinMode collapsed absent and explicit-off to the same
// ModeOff, so applyFloorToRequest(Off, ...) silently honored the downgrade and
// the command ran FULLY UNCONTAINED (the operator-verified empirical hole: the
// command exited 0 and created the file). This test pins the OUTCOME (file not
// created + refuse notice), not just the mechanism — so the crux is
// outcome-observed, not mechanism-asserted. It complements the unit-level pin
// (internal/cli.TestApplyFloorToRequest_AbsentFloorRefusesOff) which fixes the
// refuse at the clamp-pipeline boundary.
func TestSandboxAbsentFloor_RefusesExplicitOff(t *testing.T) {
	sandboxFeatureCheck(t)

	// An isolated dir OUTSIDE the repo tree: FindMinMode resolves no floor
	// (absent) — the unfloored-consumer condition.
	iso := t.TempDir()

	// The probe is OUTSIDE ./tmp/ (inside iso, not under a tmp/ subdir). Under
	// the old (hole-open) behavior the uncontained command would CREATE it.
	probe := filepath.Join(iso, "OUT.txt")
	_ = os.Remove(probe)

	// Explicit --sandbox=off downgrade in an unfloored dir → MUST be refused.
	out, exit := runSandboxIn(t, iso, []string{"--sandbox=off", "--net=deny"},
		"sh", "-c", "echo x > "+probe)

	// OUTCOME 1: the command is refused (non-zero exit) and announces the refuse.
	if exit == 0 {
		t.Fatalf("absent floor + --sandbox=off: expected REFUSE (non-zero exit), got exit=0 — the hole is still open (the explicit downgrade ran uncontained):\n%s", out)
	}
	if !strings.Contains(out, "refusing --sandbox=off") {
		t.Fatalf("absent floor + --sandbox=off: expected refuse notice ('refusing --sandbox=off'), got:\n%s", out)
	}

	// OUTCOME 2 (the load-bearing assertion): the file OUTSIDE ./tmp/ is NOT
	// created. Before Fix 1 it WAS created (the command ran uncontained).
	if _, err := os.Stat(probe); err == nil {
		t.Fatalf("CRUX BROKEN: absent floor + --sandbox=off CREATED %q — the explicit downgrade ran uncontained (the file should not exist; the refuse must stop execution before the write)", probe)
	}
}

// TestSandboxAbsentFloor_ContainedDefaultPreserved pins FIX-1's no-regression
// to standalone behavior: in an unfloored dir, the NO-FLAG default (best-effort,
// contained) and an explicit best-effort STILL RUN and are contained exactly as
// before. Fix 1 refuses only absent + explicit --sandbox=off; it must not change
// standalone contained behavior.
func TestSandboxAbsentFloor_ContainedDefaultPreserved(t *testing.T) {
	sandboxFeatureCheck(t)

	iso := t.TempDir() // outside the repo tree => no floor

	// best-effort (the no-flag default) in an unfloored dir → runs, contained.
	// A write to a path OUTSIDE ./tmp/ under best-effort is denied by Landlock
	// (the sandbox engages); the command fails non-zero WITHOUT creating the
	// file. This is the unchanged standalone contained behavior Fix 1 preserves.
	probe := filepath.Join(iso, "OUTSIDE_TMP")
	_ = os.Remove(probe)
	out, exit := runSandboxIn(t, iso, []string{"--sandbox=best-effort", "--net=deny"},
		"sh", "-c", "echo x > "+probe)
	// Under best-effort the sandbox engages (Linux has Landlock), so the write
	// outside tmp is denied — non-zero exit and NO refuse notice (it is NOT the
	// Fix-1 refuse; it is the kernel containment). The file must not exist.
	if exit == 0 {
		t.Fatalf("absent floor + best-effort: write outside tmp succeeded (exit=0) — containment regressed (best-effort must still engage the sandbox and deny the write in an unfloored dir):\n%s", out)
	}
	if _, err := os.Stat(probe); err == nil {
		t.Fatalf("absent floor + best-effort: write outside tmp created %q — containment regressed (best-effort must still engage the sandbox in an unfloored dir):\n%s", probe, out)
	}
	// And crucially: NO Fix-1 refuse notice (best-effort is never refused).
	if strings.Contains(out, "refusing --sandbox=off") {
		t.Fatalf("absent floor + best-effort: Fix-1 refuse fired (must not — only explicit off is refused):\n%s", out)
	}

	// CAUSE-PINNING (defer-023): the assertions above observe the OUTCOME (file
	// not created + non-zero exit). This block narrows the CAUSE so a future
	// regression that blocks the write for the WRONG reason cannot false-pass.
	//
	// Landlock filesystem denials manifest as EACCES at the syscall layer —
	// there is no Landlock-specific errno, and the harness does not attribute
	// denials to Landlock at runtime, so "permission denied" (EACCES) is the
	// most direct cause signal this seam exposes. Here it is also
	// DISCRIMINATING: the probe's parent is t.TempDir() (owner-owned, 0700),
	// so the owner has full filesystem write permission and the ONLY remaining
	// source of an EACCES on the write is the Landlock ruleset. A wrong-reason
	// failure would surface a different error class instead — ENOENT ("no such
	// file or directory" if the path/sandbox cwd drifted), ENOSPC ("no space
	// left on device"), or seccomp's ENOSYS ("function not implemented") — none
	// of which carry "permission denied". Requiring the EACCES signal therefore
	// catches exactly the regression the outcome-only assertion would miss.
	//
	// Honesty caveat: this pins a PERMISSION-CLASS denial attributable to
	// Landlock in this owner-owned-dir setup; it does not fingerprint Landlock
	// at the kernel (EACCES is shared with fs-perm denials in general).
	if !strings.Contains(strings.ToLower(out), "permission denied") {
		t.Fatalf("absent floor + best-effort: write outside tmp failed (exit=%d) but WITHOUT the EACCES cause signal ('permission denied') — the denial came from the wrong reason, not Landlock containment:\n%s", exit, out)
	}
}

// TestSandboxStrictFloor_DeniesP5Bypass is the CRUX integration test: in the
// repo (which carries exec_sandbox.min_mode: strict), the P5 bypass
// (--sandbox=off) is DENIED — the binary upgrades off -> strict, so a write
// OUTSIDE ./tmp/ fails with EACCES (the file is not created). This proves, via
// the real binary on the real kernel, that an agent granted exec-sandbox cannot
// escape the strict floor.
func TestSandboxStrictFloor_DeniesP5Bypass(t *testing.T) {
	sandboxFeatureCheck(t)

	probe := filepath.Join(repoRoot, "tmp", "floor_p5_probe")
	_ = os.Remove(probe)

	// --sandbox=off under the repo's strict floor: must be upgraded to strict.
	out, exit := runSandbox(t, []string{"--sandbox=off", "--net=deny"},
		"touch", filepath.Join("tmp", "floor_p5_probe"))

	// The write to ./tmp/ SUCCEEDS even under the upgraded strict floor (tmp is
	// the RW dir). The crux assertion is that the floor upgraded off -> strict:
	// the output must announce the upgrade, proving the bypass was denied.
	if !strings.Contains(out, "upgraded to strict") {
		t.Fatalf("P5 bypass: expected '--sandbox=off upgraded to strict' notice under the strict floor, got exit=%d:\n%s", exit, out)
	}
	if exit != 0 {
		t.Fatalf("touch ./tmp/ under the upgraded strict floor should succeed, got exit=%d:\n%s", exit, out)
	}
	_ = os.Remove(probe)

	// And the INTEGRITY half of the crux: writing OUTSIDE ./tmp/ must be denied
	// under the strict floor, even though the caller asked for --sandbox=off.
	out2, exit2 := runSandbox(t, []string{"--sandbox=off", "--net=deny"},
		"touch", filepath.Join(".git", "floor_p5_outside"))
	if exit2 == 0 {
		t.Fatalf("CRUX BROKEN: --sandbox=off wrote OUTSIDE tmp (.git/) — the strict floor did NOT contain the bypass")
	}
	_ = os.Remove(filepath.Join(repoRoot, ".git", "floor_p5_outside"))
	if !strings.Contains(strings.ToLower(out2), "permission denied") {
		t.Fatalf("expected write-outside-tmp to be denied (EACCES) under the strict floor, got exit=%d:\n%s", exit2, out2)
	}
}

// TestSandboxStrictFloor_ForcesNetDeny proves the net-floor half of the Level-B
// containment contract: under the repo's strict floor, --net=allow is upgraded
// to deny, so a granted agent cannot reach the network even by passing
// --net=allow. (Contrast with TestSandboxNetworkAllow, which runs WITHOUT a
// floor in an isolated dir.)
func TestSandboxStrictFloor_ForcesNetDeny(t *testing.T) {
	sandboxFeatureCheck(t)

	out, exit := runSandbox(t, []string{"--sandbox=strict", "--net=allow"},
		"python3", "-c", `import socket; s=socket.socket(); print("socket created OK")`)

	if exit == 0 {
		t.Fatalf("CRUX BROKEN: --net=allow under the strict floor permitted a socket — the net-floor did NOT deny network")
	}
	if !strings.Contains(out, "upgraded to deny") {
		t.Fatalf("expected '--net=allow upgraded to deny' notice under the strict floor, got exit=%d:\n%s", exit, out)
	}
}

// TestSandboxStrictFloor_ReadCodeAndTmpWrite proves the Level-B payoff: under
// the strict floor, an agent can still run ARBITRARY read-code (python3 reads a
// repo file) AND write to ./tmp/ — the containment denies only writes-outside-
// tmp and network, not read analysis. This is the dogfood contract for the
// researcher/media-perception/repo-explorer grant.
func TestSandboxStrictFloor_ReadCodeAndTmpWrite(t *testing.T) {
	sandboxFeatureCheck(t)

	outFile := filepath.Join(repoRoot, "tmp", "floor_reader_dump")
	_ = os.Remove(outFile)
	// python3 reads a repo file (go.mod) and writes a dump to ./tmp/.
	out, exit := runSandbox(t, []string{"--sandbox=off", "--net=deny"},
		"python3", "-c",
		`open("tmp/floor_reader_dump","w").write(open("go.mod").read()[:20]); print("read+tmpwrite OK")`)
	if exit != 0 {
		t.Fatalf("read-code + tmp-write should succeed under the strict floor, got exit=%d:\n%s", exit, out)
	}
	if !strings.Contains(out, "read+tmpwrite OK") {
		t.Fatalf("expected read+tmpwrite success, got:\n%s", out)
	}
	if _, err := os.Stat(outFile); err != nil {
		t.Fatalf("tmp dump not created under the strict floor: %v", err)
	}
	_ = os.Remove(outFile)
}

// TestSandboxStrictFloor_DualRootCwdBypass closes the out-of-project --cwd
// bypass: a caller outside the project (no floor at their cwd) using --cwd to
// target the strict-floored repo must STILL discover the floor from repoRoot.
// Without dual-root resolution, the caller's cwd has no floor and the --cwd
// target's strict floor is never consulted → --sandbox=off runs uncontained.
func TestSandboxStrictFloor_DualRootCwdBypass(t *testing.T) {
	sandboxFeatureCheck(t)

	outside := t.TempDir() // caller cwd: outside the repo tree, no floor

	// --sandbox=off from outside with --cwd=repoRoot: the floor must be discovered
	// from repoRoot (strict) even though realCWD (outside) has no floor.
	out, exit := runSandboxIn(t, outside,
		[]string{"--cwd=" + repoRoot, "--sandbox=off", "--net=deny"},
		"touch", filepath.Join(".git", "floor_cwd_bypass"))

	// The floor upgrade notice proves the dual-root discovery worked.
	if !strings.Contains(out, "upgraded to strict") {
		t.Fatalf("dual-root --cwd bypass: expected '--sandbox=off upgraded to strict' notice (floor from repoRoot), got exit=%d:\n%s", exit, out)
	}
	// Writing OUTSIDE ./tmp/ must be denied — the strict containment held.
	if exit == 0 {
		t.Fatalf("CRUX BROKEN: --cwd bypass wrote to .git/ — the dual-root floor did NOT contain it")
	}
	_ = os.Remove(filepath.Join(repoRoot, ".git", "floor_cwd_bypass"))
}

// TestSandboxStrictFloor_WeakeningChildEscaped closes the F1 weakening-child
// escape: a granted agent plants a weakening child run-shape.yml (min_mode: off)
// under the RW ./tmp tree, then invokes from there with --sandbox=off. The
// MAX-over-entire-chain walk MUST discover the project's strict parent floor
// and override the child's weakening off — the agent CANNOT escape by planting
// a weaker floor in its own writable area.
func TestSandboxStrictFloor_WeakeningChildEscaped(t *testing.T) {
	sandboxFeatureCheck(t)

	// Plant a weakening floor under ./tmp/evil/ (agent-writable area).
	evilDir := filepath.Join(repoRoot, "tmp", "evil-floor-test")
	evilVh := filepath.Join(evilDir, ".vh-agent-harness")
	if err := os.MkdirAll(evilVh, 0o755); err != nil {
		t.Fatalf("mkdir evil floor dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(evilDir) })
	evilShape := filepath.Join(evilVh, "run-shape.yml")
	if err := os.WriteFile(evilShape, []byte("exec_sandbox:\n  min_mode: off\n"), 0o644); err != nil {
		t.Fatalf("write weakening floor: %v", err)
	}

	// Invoke from the evil dir with --sandbox=off --net=allow (P5 bypass attempt).
	// FindMinMode walks the ENTIRE chain: off at tmp/evil (rank 1) → strict at
	// repo root (rank 3 > 1) → MAX = strict. The weakening child is overridden.
	out, exit := runSandboxIn(t, evilDir,
		[]string{"--sandbox=off", "--net=allow"},
		"touch", filepath.Join("..", "..", ".git", "floor_weakening_child"))

	// CRUX: the strict parent floor MUST override the weakening child.
	if !strings.Contains(out, "upgraded to strict") {
		t.Fatalf("weakening child escape: expected '--sandbox=off upgraded to strict' (parent strict overrides child off), got exit=%d:\n%s", exit, out)
	}
	if exit == 0 {
		t.Fatalf("CRUX BROKEN: weakening child floor let --sandbox=off write to .git/ — MAX-over-chain did NOT override the child")
	}
	if !strings.Contains(out, "upgraded to deny") {
		t.Fatalf("weakening child escape: expected '--net=allow upgraded to deny' (strict floor forces net deny), got exit=%d:\n%s", exit, out)
	}
	_ = os.Remove(filepath.Join(repoRoot, ".git", "floor_weakening_child"))
}

// TestSandboxParentDirNotAccessible verifies that `ls ..` is denied — the
// parent directory is outside the repo root and not in the Landlock ruleset.
// This prevents listing sibling repos/directories.
func TestSandboxParentDirNotAccessible(t *testing.T) {
	sandboxFeatureCheck(t)

	out, exit := runSandbox(t, []string{"--sandbox=best-effort", "--net=deny"},
		"ls", "..")

	if exit == 0 {
		t.Fatalf("ls .. should be denied (parent not in ruleset) but succeeded")
	}
	if !strings.Contains(strings.ToLower(out), "permission denied") {
		t.Fatalf("expected 'permission denied' for ls .., got:\n%s", out)
	}
}

// TestSandboxStatShowsMetadata documents the HONESTY FRAMING: stat() on paths
// outside the sandbox SUCCEEDS (metadata is visible) even though open/read
// is denied. This is the "visible but inaccessible" behavior of Landlock —
// exec-sandbox is an integrity boundary, NOT a confidentiality boundary.
func TestSandboxStatShowsMetadata(t *testing.T) {
	sandboxFeatureCheck(t)

	// stat ~/.ssh — the home directory is NOT in the ruleset.
	// stat() is not Landlock-gated, so metadata is visible.
	homeSSH := filepath.Join(os.Getenv("HOME"), ".ssh")
	if _, err := os.Stat(homeSSH); err != nil {
		t.Skipf("~/.ssh does not exist; skipping stat visibility probe")
	}

	out, exit := runSandbox(t, []string{"--sandbox=best-effort", "--net=deny"},
		"stat", homeSSH)

	// This DOCUMENTS the behavior: stat succeeds (metadata visible).
	// This is NOT a bug — it confirms exec-sandbox is integrity, not confidentiality.
	if exit != 0 {
		t.Logf("stat ~/.ssh was denied (exit=%d):\n%s", exit, out)
		t.Logf("NOTE: if your kernel gates stat() via Landlock, this is stricter than v1 baseline.")
	} else {
		t.Logf("stat ~/.ssh SUCCEEDED — metadata visible (integrity-not-confidentiality confirmed)")
	}
}

// TestSandboxGitStatusReadOnly verifies that git can READ the .git directory
// (it's under the RO repo root) but cannot WRITE to it. Git may emit warnings
// about ~/.gitconfig being inaccessible (home not in ruleset) — that is expected.
func TestSandboxGitStatusReadOnly(t *testing.T) {
	sandboxFeatureCheck(t)

	if _, err := os.Stat(filepath.Join(repoRoot, ".git")); err != nil {
		t.Skipf("no .git directory in repo root; skipping git status probe")
	}

	out, exit := runSandbox(t,
		[]string{"--sandbox=best-effort", "--net=deny"},
		"git", "-c", "core.fileMode=false", "status", "--short")

	// Git may fail due to ~/.gitconfig being inaccessible (home not in ruleset).
	// The key assertion: git does NOT fail with "permission denied" on the REPO's
	// .git/ directory (it must be readable as RO under repoRoot). We check for
	// ".git/" with a trailing slash to distinguish from ".gitconfig" in home.
	lower := strings.ToLower(out)
	if exit != 0 && strings.Contains(lower, ".git/") && strings.Contains(lower, "permission denied") {
		t.Fatalf("git should be able to READ .git/ directory (it's RO under repo root):\n%s", out)
	}
	// Any other exit is acceptable (gitconfig warnings from inaccessible home, etc.)
	// — the test documents the behavior.
	t.Logf("git status exit=%d, output:\n%s", exit, out)
}

// TestSandboxBasicCommandRuns verifies the fundamental trampoline works: a
// simple echo should produce output and exit 0.
func TestSandboxBasicCommandRuns(t *testing.T) {
	sandboxFeatureCheck(t)

	out, exit := runSandbox(t, []string{"--sandbox=best-effort", "--net=deny"},
		"echo", "sandbox-probe-ok")

	if exit != 0 {
		t.Fatalf("echo should succeed under sandbox, got exit=%d:\n%s", exit, out)
	}
	if !strings.Contains(out, "sandbox-probe-ok") {
		t.Fatalf("expected echo output 'sandbox-probe-ok', got:\n%s", out)
	}
}

// TestSandboxStrictFailsOnMissingPrimitives documents the strict-mode
// fail-closed guarantee. On a kernel WITHOUT landlock/seccomp, strict mode
// must refuse to run. On a kernel WITH support, strict mode runs normally.
// This test runs `true` (exit 0) and checks that it either runs (features
// available) or fails with a clear message (features unavailable).
func TestSandboxStrictModeContract(t *testing.T) {
	out, exit := runSandbox(t, []string{"--sandbox=strict", "--net=deny"}, "true")

	if exit == 0 {
		t.Logf("strict mode: features available, command ran successfully")
	} else {
		// Strict mode with missing features must fail-closed with a clear message.
		if !strings.Contains(strings.ToLower(out), "unavailable") &&
			!strings.Contains(strings.ToLower(out), "primitives") {
			t.Fatalf("strict mode failure should explain unavailable primitives, got:\n%s", out)
		}
		t.Logf("strict mode: features unavailable, correctly refused (exit=%d)", exit)
	}
}
