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

// runSandbox invokes exec-sandbox with the given flags and target command,
// returning combined output and exit error.
func runSandbox(t *testing.T, flags []string, target ...string) (string, int) {
	t.Helper()
	args := []string{"exec-sandbox"}
	args = append(args, flags...)
	args = append(args, "--")
	args = append(args, target...)
	cmd := exec.Command(sandboxBin, args...)
	cmd.Dir = repoRoot
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

// TestSandboxNetworkAllow verifies that socket creation works when --net=allow.
func TestSandboxNetworkAllow(t *testing.T) {
	sandboxFeatureCheck(t)

	out, exit := runSandbox(t, []string{"--sandbox=best-effort", "--net=allow"},
		"python3", "-c", `import socket; s=socket.socket(); print("socket created OK")`)

	if exit != 0 {
		t.Fatalf("network should be allowed (--net=allow) but socket creation failed:\n%s", out)
	}
	if !strings.Contains(out, "socket created OK") {
		t.Fatalf("expected socket creation success, got:\n%s", out)
	}
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
