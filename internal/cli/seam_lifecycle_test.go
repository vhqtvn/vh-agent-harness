package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/lineage"
)

// These tests exercise the SEAM path of diff/uninstall/preflight (the default
// since install writes .vh-agent-harness/lineage.yml, not a legacy manifest).
// The legacy-manifest path stays covered by lifecycle_test.go.

const seamManagedProbe = ".opencode/agents/build.md"

// TestSeamDiff_CleanThenDrift: a fresh seam install diffs clean; editing a
// platform_managed file makes diff report it drifted and exit non-zero.
func TestSeamDiff_CleanThenDrift(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	runWithCwd(t, root, func() {
		cmd, buf := newOutCmd()
		if err := runDiff(cmd, nil); err != nil {
			t.Fatalf("clean seam diff: want nil, got %v (out=%q)", err, buf.String())
		}
		if out := buf.String(); !strings.Contains(out, "0 drifted, 0 missing, 0 unexpected") {
			t.Errorf("clean diff summary unexpected: %q", out)
		}
	})

	corruptManaged(t, root, seamManagedProbe)
	runWithCwd(t, root, func() {
		cmd, buf := newOutCmd()
		err := runDiff(cmd, nil)
		if err == nil {
			t.Fatalf("drifted seam diff: want non-nil (errSilent), got nil (out=%q)", buf.String())
		}
		out := buf.String()
		if !strings.Contains(out, "drifted") || !strings.Contains(out, seamManagedProbe) {
			t.Errorf("diff want %s drifted, got %q", seamManagedProbe, out)
		}
	})
}

// TestSeamDiff_DetectsUnexpectedSkipsState: a non-corpus file under .opencode/
// is reported unexpected, but runtime-state subtrees are not flagged.
func TestSeamDiff_DetectsUnexpectedSkipsState(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	rogue := filepath.Join(root, ".opencode", "agents", "rogue.md")
	if err := os.WriteFile(rogue, []byte("rogue\n"), 0o644); err != nil {
		t.Fatalf("write rogue: %v", err)
	}
	stateF := filepath.Join(root, ".opencode", "state", "session.json")
	if err := os.MkdirAll(filepath.Dir(stateF), 0o755); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}
	if err := os.WriteFile(stateF, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}

	runWithCwd(t, root, func() {
		cmd, buf := newOutCmd()
		err := runDiff(cmd, nil)
		if err == nil {
			t.Fatalf("unexpected file: want non-nil, got nil (out=%q)", buf.String())
		}
		out := buf.String()
		if !strings.Contains(out, "unexpected") || !strings.Contains(out, ".opencode/agents/rogue.md") {
			t.Errorf("diff want rogue.md unexpected, got %q", out)
		}
		if strings.Contains(out, "state/session.json") {
			t.Errorf("diff must NOT flag runtime state as unexpected, got %q", out)
		}
	})
}

// TestSeamUninstall_RemovesManagedPreservesOwnedAndLineageLast confirms the
// seam uninstall removes platform-controlled files, preserves a project_owned
// corpus file (.gitignore) and runtime state, and removes lineage last.
func TestSeamUninstall_RemovesManagedPreservesOwnedAndLineageLast(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	managed := filepath.Join(root, filepath.FromSlash(seamManagedProbe))
	gitignore := filepath.Join(root, ".gitignore") // project_owned corpus seed
	if !pathExists(t, managed) {
		t.Fatalf("precondition: %s should exist after install", seamManagedProbe)
	}
	if !pathExists(t, lineage.FilePath(root)) {
		t.Fatalf("precondition: lineage should exist after install")
	}
	stateF := filepath.Join(root, ".opencode", "state", "s.json")
	os.MkdirAll(filepath.Dir(stateF), 0o755)
	os.WriteFile(stateF, []byte("{}"), 0o644)

	runWithCwd(t, root, func() {
		uninstallForce = false
		cmd, buf := newOutCmd()
		if err := runUninstall(cmd, nil); err != nil {
			t.Fatalf("seam uninstall: %v (out=%q)", err, buf.String())
		}
		if out := buf.String(); !strings.Contains(out, "leftover (intentionally preserved)") {
			t.Errorf("want leftover report, got %q", out)
		}
	})

	if pathExists(t, managed) {
		t.Errorf("managed %s still present after uninstall", seamManagedProbe)
	}
	if pathExists(t, lineage.FilePath(root)) {
		t.Errorf("lineage still present after uninstall (should be removed last)")
	}
	if !pathExists(t, gitignore) {
		t.Errorf("project_owned .gitignore removed (should be preserved without --force)")
	}
	if !pathExists(t, stateF) {
		t.Errorf("runtime state removed (should be preserved)")
	}
}

// TestSeamPreflight_PassThenDrift: fresh seam install passes preflight via the
// seam authorities; corrupting a managed file fails it on managed-drift.
func TestSeamPreflight_PassThenDrift(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	runWithCwd(t, root, func() {
		cmd, buf := newOutCmd()
		if err := runPreflight(cmd, nil); err != nil {
			t.Fatalf("seam preflight pass: want nil, got %v (out=%q)", err, buf.String())
		}
		out := buf.String()
		if !strings.Contains(out, "result: PASS") {
			t.Errorf("want 'result: PASS', got %q", out)
		}
		if !strings.Contains(out, "lineage") {
			t.Errorf("seam preflight should run the lineage check, got %q", out)
		}
	})

	corruptManaged(t, root, seamManagedProbe)
	runWithCwd(t, root, func() {
		cmd, buf := newOutCmd()
		if err := runPreflight(cmd, nil); err == nil {
			t.Fatalf("seam preflight drift: want non-nil, got nil (out=%q)", buf.String())
		}
		if out := buf.String(); !strings.Contains(out, "managed-drift") || !strings.Contains(out, "FAIL") {
			t.Errorf("want managed-drift FAIL, got %q", out)
		}
	})
}
