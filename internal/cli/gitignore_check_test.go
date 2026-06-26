package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitInit makes dir a git work tree, or skips the test if git is unavailable in
// the environment (the check itself degrades to SKIP there, so there is nothing
// deterministic to assert).
func gitInit(t *testing.T, dir string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	if err := exec.Command("git", "-C", dir, "init", "-q").Run(); err != nil {
		t.Skipf("git init unavailable: %v", err)
	}
}

func writeGitignore(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(body), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}
}

// TestRuntimeStateGitignored_PassWhenIgnored: a .gitignore listing the runtime-
// state dirs yields PASS.
func TestRuntimeStateGitignored_PassWhenIgnored(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeGitignore(t, dir, ".opencode/state/\n.opencode/sessions/\n.opencode/plans/\n.opencode/runs/\n")

	r := checkRuntimeStateGitignored(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierPass {
		t.Fatalf("want PASS when all runtime-state dirs ignored, got %s: %s", r.tier, r.detail)
	}
}

// TestRuntimeStateGitignored_WarnWhenMissing: dropping an entry yields a WARN
// (never FAIL) that names the offending dir.
func TestRuntimeStateGitignored_WarnWhenMissing(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	// Ignore everything EXCEPT .opencode/runs.
	writeGitignore(t, dir, ".opencode/state/\n.opencode/sessions/\n.opencode/plans/\n")

	r := checkRuntimeStateGitignored(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierWarn {
		t.Fatalf("want WARN when a runtime-state dir is not ignored, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, ".opencode/runs") {
		t.Errorf("WARN should name the un-ignored dir; got %q", r.detail)
	}
}

// TestRuntimeStateGitignored_SkipOutsideGit: outside a git work tree the check
// is a no-op SKIP, not a false WARN.
func TestRuntimeStateGitignored_SkipOutsideGit(t *testing.T) {
	dir := t.TempDir() // no git init
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	r := checkRuntimeStateGitignored(dir)
	if r.tier != tierSkip {
		t.Fatalf("want SKIP outside a git work tree, got %s: %s", r.tier, r.detail)
	}
}
