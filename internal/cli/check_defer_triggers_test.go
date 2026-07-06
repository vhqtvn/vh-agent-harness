package cli

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestCheckDeferTriggersScriptLoads is a regression guard for a
// dead-on-arrival import bug. templates/core/.opencode/scripts/check-defer-triggers.js
// once imported repoRoot() from "../../plugins/shell-guard-core.js" — one "../"
// too many — so node resolved it to <repo-root>/plugins/shell-guard-core.js
// (which does not exist) and threw ERR_MODULE_NOT_FOUND before main() ran. The
// entire R6 promotion-aid referenced canonically in AGENTS.md as
// `node .opencode/scripts/check-defer-triggers.js` was therefore dead, and the
// bug shipped to every consumer via the templates/core embed.
//
// This smoke test copies the TEMPLATE script (source of truth, independent of
// whether `make update` has been run) into a hermetic scratch tree and runs it
// against a missing tasks dir. It asserts the module loads, main() reaches the
// degrade path, exits 0, and no module-resolution error escapes — so any future
// broken import fails the suite immediately.
func TestCheckDeferTriggersScriptLoads(t *testing.T) {
	nodeBin, err := exec.LookPath("node")
	if err != nil {
		t.Skipf("node not on PATH; skipping rendered-script load smoke test: %v", err)
	}

	root := findModuleRoot(t)
	src := filepath.Join(root, "templates", "core", ".opencode", "scripts", "check-defer-triggers.js")
	body, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read template script %s: %v", src, err)
	}

	// Place the copy at <scratch>/.opencode/scripts/ so the script's
	// __dirname-based repoRoot() (path.resolve(__dirname, "..", ".."))
	// resolves to <scratch>, keeping the run hermetic and cwd-robust.
	scratch := t.TempDir()
	dst := filepath.Join(scratch, ".opencode", "scripts", "check-defer-triggers.js")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatalf("mkdir scratch scripts dir: %v", err)
	}
	if err := os.WriteFile(dst, body, 0o644); err != nil {
		t.Fatalf("write scratch script: %v", err)
	}

	// --tasks points at a path that does not exist, exercising the degrade
	// branch (the script's contract is: never blocking, always exit 0).
	missingTasks := filepath.Join(scratch, "nonexistent-tasks-for-test")
	cmd := exec.Command(nodeBin, dst, "--tasks", missingTasks)
	cmd.Dir = scratch
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// Bound the run so a hung process fails the test instead of hanging CI.
	timer := time.AfterFunc(30*time.Second, func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})
	defer timer.Stop()

	runErr := cmd.Run()
	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("node spawn error: %v\nstderr: %s", runErr, stderr.String())
		}
	}

	combined := stdout.String() + stderr.String()

	// Regression guard: a broken ESM import throws ERR_MODULE_NOT_FOUND and
	// never reaches main(). Catch both the node error code/message so a
	// re-introduced dangling import fails loudly here.
	if strings.Contains(combined, "ERR_MODULE_NOT_FOUND") ||
		strings.Contains(combined, "Cannot find module") {
		t.Fatalf("script failed to load — broken import regressed:\n%s", combined)
	}

	if exitCode != 0 {
		t.Fatalf("expected exit 0 (script is promoter-use-only and never blocks; "+
			"a missing tasks dir is a non-fatal degrade), got %d\n%s", exitCode, combined)
	}

	// The degrade path must announce the missing dir and that it is non-blocking.
	if !strings.Contains(stdout.String(), "no tasks dir") {
		t.Fatalf("expected degrade message mentioning 'no tasks dir', got stdout:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "never blocking") {
		t.Fatalf("expected degrade message to state 'never blocking', got stdout:\n%s", stdout.String())
	}
}

// findModuleRoot walks up from cwd until it finds go.mod. go test runs with
// cwd = the package source dir (.../internal/cli), so the module root (where
// templates/ lives) is a few parents up. Mirrors the helper in
// internal/permission/shellguard_hook_test.go; duplicated here to keep the
// smoke test self-contained within the cli package.
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
