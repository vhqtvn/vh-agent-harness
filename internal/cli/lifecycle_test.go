package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/manifest"
)

// addProjectOwned writes a project-owned file to disk and records it in the
// manifest so uninstall tests can assert the preserve/force contract.
func addProjectOwned(t *testing.T, root, rel string) {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(abs, []byte("project-owned content\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	m := readDiskManifest(t, root)
	m.SetFile(rel, manifest.File{Hash: "sha256:0", Class: manifest.ClassProjectOwned})
	if err := m.Write(root); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

// corruptManaged overwrites a tracked managed file with junk so drift is real.
func corruptManaged(t *testing.T, root, rel string) {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.WriteFile(abs, []byte("CORRUPTED-BY-TEST\n"), 0o644); err != nil {
		t.Fatalf("corrupt %s: %v", rel, err)
	}
}

// pathExists reports whether abs exists on disk.
func pathExists(t *testing.T, abs string) bool {
	t.Helper()
	_, err := os.Stat(abs)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	t.Fatalf("stat %s: %v", abs, err)
	return false
}

// ---- uninstall -------------------------------------------------------------

func TestUninstall_PreservesStateProjectOwnedRemovesManaged(t *testing.T) {
	root := t.TempDir()
	seedLegacyManifest(t, root)

	const managedRel = ".opencode/agents/planner.md"
	const localRel = ".local/coordinator/README.md"
	const stateRel = ".opencode/state/session.json"
	const ownedRel = "docs/my-notes.md"

	// Add project-owned file + manifest entry.
	addProjectOwned(t, root, ownedRel)
	// Add runtime state.
	stateAbs := filepath.Join(root, filepath.FromSlash(stateRel))
	os.MkdirAll(filepath.Dir(stateAbs), 0o755)
	os.WriteFile(stateAbs, []byte("{}"), 0o644)

	runWithCwd(t, root, func() {
		uninstallForce = false
		cmd, buf := newOutCmd()
		if err := runUninstall(cmd, []string{}); err != nil {
			t.Fatalf("uninstall: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "leftover (intentionally preserved)") {
			t.Errorf("uninstall want leftover report, got %q", out)
		}
		if !strings.Contains(out, ownedRel) || !strings.Contains(out, "project-owned") {
			t.Errorf("leftover report want %s as project-owned, got %q", ownedRel, out)
		}
		if !strings.Contains(out, ".local") {
			t.Errorf("leftover report want .local, got %q", out)
		}
	})

	// Managed file removed.
	if pathExists(t, filepath.Join(root, managedRel)) {
		t.Errorf("managed file %s still on disk after uninstall", managedRel)
	}
	// Project-owned preserved.
	if !pathExists(t, filepath.Join(root, ownedRel)) {
		t.Errorf("project-owned %s removed (should be preserved)", ownedRel)
	}
	// State preserved.
	if !pathExists(t, stateAbs) {
		t.Errorf("state %s removed (should be preserved)", stateRel)
	}
	// .local/ preserved entirely (managed-class scaffold examples included).
	if !pathExists(t, filepath.Join(root, localRel)) {
		t.Errorf(".local file %s removed (should be preserved)", localRel)
	}
	// Manifest removed LAST.
	if pathExists(t, manifest.FilePath(root)) {
		t.Errorf("manifest still on disk after uninstall")
	}
}

func TestUninstall_ForceRemovesProjectOwnedKeepsStateAndLocal(t *testing.T) {
	root := t.TempDir()
	seedLegacyManifest(t, root)

	const ownedRel = "docs/force-owned.md"
	const localRel = ".local/coordinator/README.md"
	const stateRel = ".opencode/state/x.json"
	addProjectOwned(t, root, ownedRel)
	stateAbs := filepath.Join(root, filepath.FromSlash(stateRel))
	os.MkdirAll(filepath.Dir(stateAbs), 0o755)
	os.WriteFile(stateAbs, []byte("{}"), 0o644)

	runWithCwd(t, root, func() {
		uninstallForce = true
		cmd, _ := newOutCmd()
		if err := runUninstall(cmd, []string{}); err != nil {
			t.Fatalf("uninstall --force: %v", err)
		}
	})
	// --force removes project-owned.
	if pathExists(t, filepath.Join(root, ownedRel)) {
		t.Errorf("project-owned %s still on disk after --force", ownedRel)
	}
	// --force STILL preserves state + .local.
	if !pathExists(t, stateAbs) {
		t.Errorf("state %s removed by --force (should be preserved)", stateRel)
	}
	if !pathExists(t, filepath.Join(root, localRel)) {
		t.Errorf(".local %s removed by --force (should be preserved)", localRel)
	}
}

// ---- preflight -------------------------------------------------------------

func TestPreflight_Pass(t *testing.T) {
	root := t.TempDir()
	seedLegacyManifest(t, root)
	runWithCwd(t, root, func() {
		cmd, buf := newOutCmd()
		if err := runPreflight(cmd, []string{}); err != nil {
			t.Fatalf("preflight pass: want nil error, got %v (out=%q)", err, buf.String())
		}
		out := buf.String()
		if !strings.Contains(out, "result: PASS") {
			t.Errorf("preflight want 'result: PASS', got %q", out)
		}
	})
}

func TestPreflight_FailsOnMissingEvalJS(t *testing.T) {
	root := t.TempDir()
	seedLegacyManifest(t, root)
	os.Remove(filepath.Join(root, ".opencode/plugins/shell-guard/eval.js"))
	runWithCwd(t, root, func() {
		cmd, buf := newOutCmd()
		err := runPreflight(cmd, []string{})
		if err == nil {
			t.Fatalf("preflight with missing eval.js: want non-nil error, got nil")
		}
		if out := buf.String(); !strings.Contains(out, "eval.js") || !strings.Contains(out, "FAIL") {
			t.Errorf("preflight want eval.js FAIL, got %q", out)
		}
	})
}

func TestPreflight_FailsOnDrift(t *testing.T) {
	root := t.TempDir()
	seedLegacyManifest(t, root)
	corruptManaged(t, root, ".opencode/agents/planner.md")
	runWithCwd(t, root, func() {
		cmd, buf := newOutCmd()
		if err := runPreflight(cmd, []string{}); err == nil {
			t.Fatalf("preflight with drift: want non-nil error, got nil")
		}
		if out := buf.String(); !strings.Contains(out, "drift") || !strings.Contains(out, "FAIL") {
			t.Errorf("preflight want drift FAIL, got %q", out)
		}
	})
}

func TestPreflight_FailsOnMalformedManifest(t *testing.T) {
	root := t.TempDir()
	seedLegacyManifest(t, root)
	os.WriteFile(manifest.FilePath(root), []byte("{not json"), 0o644)
	runWithCwd(t, root, func() {
		cmd, buf := newOutCmd()
		if err := runPreflight(cmd, []string{}); err == nil {
			t.Fatalf("preflight with malformed manifest: want non-nil error, got nil")
		}
		if out := buf.String(); !strings.Contains(out, "manifest") || !strings.Contains(out, "FAIL") {
			t.Errorf("preflight want manifest FAIL, got %q", out)
		}
	})
}

// ---- doctor / update -------------------------------------------------------
//
// `doctor`/`update` are seam verbs (lineage authority + core corpus). Their
// coverage lives in seam_cli_test.go (seamInstallInto + HEALTHY/UNHEALTHY/
// reconcile/conflict). The render/upgrade verbs and their tests were retired
// with the legacy harness-root install path.
