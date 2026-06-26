package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/vhqtvn/vh-agent-harness/internal/manifest"
)

// runWithCwd calls fn while the process cwd is temporarily set to dir. Tests
// use this to drive the cwd-based loadManifest() without polluting other tests.
// It must not be used together with t.Parallel().
func runWithCwd(t *testing.T, dir string, fn func()) {
	t.Helper()
	saved, err := os.Getwd()
	if err != nil {
		t.Fatalf("getcwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	defer func() {
		if err := os.Chdir(saved); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	}()
	fn()
}

// readDiskManifest parses the on-disk manifest at root/.opencode/harness-manifest.json.
func readDiskManifest(t *testing.T, root string) *manifest.Manifest {
	t.Helper()
	data, err := os.ReadFile(manifest.FilePath(root))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var m manifest.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	return &m
}

// manifestBytes returns the raw on-disk manifest bytes for byte-identity checks.
func manifestBytes(t *testing.T, root string) []byte {
	t.Helper()
	data, err := os.ReadFile(manifest.FilePath(root))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	return data
}

// newOutCmd returns a cobra.Command with stdout/stderr captured in a buffer.
func newOutCmd() (*cobra.Command, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	cmd := &cobra.Command{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	return cmd, buf
}

// seedLegacyManifest writes a minimal, drift-clean legacy install into root: a
// harness-manifest.json plus the tracked managed files it references (with
// real on-disk content whose sha256 matches the manifest hashes) and the
// .local/ scaffold example uninstall preserves. It replaces the old
// installInto helper (which ran the now-retired installer.Run against the
// harness-root corpus). The verbs that still consume a legacy manifest
// (uninstall/preflight/diff) read this on-disk state; no command writes it
// anymore since the seam became the sole install source.
//
// The two managed files seeded (.opencode/plugins/shell-guard/eval.js and
// .opencode/agents/planner.md) are the surfaces the preflight/uninstall tests
// exercise: eval.js is the shell-guard gate preflight requires, planner.md is
// the managed file drift/uninstall corrupt/remove. Runtime backend is "bare"
// so preflight's runtime check resolves without a docker daemon.
func seedLegacyManifest(t *testing.T, root string) {
	t.Helper()
	managed := map[string]string{
		".opencode/plugins/shell-guard/eval.js": "// shell-guard eval.js (seeded legacy fixture)\n",
		".opencode/agents/planner.md":           "# planner\n\nSeeded legacy managed file.\n",
	}
	m := manifest.New()
	m.HarnessVersion = "test"
	m.Project = manifest.Project{Name: "Demo", Slug: "demo"}
	m.Runtime = manifest.Runtime{Backend: "bare"}
	for rel, content := range managed {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
		sum := sha256.Sum256([]byte(content))
		m.SetFile(rel, manifest.File{Hash: "sha256:" + hex.EncodeToString(sum[:]), Class: manifest.ClassManaged})
	}
	// .local/ scaffold example: on disk only (not manifest-tracked; drift does
	// not scan .local/ and uninstall always preserves it).
	localRel := filepath.Join(root, ".local", "coordinator", "README.md")
	if err := os.MkdirAll(filepath.Dir(localRel), 0o755); err != nil {
		t.Fatalf("mkdir .local/coordinator: %v", err)
	}
	if err := os.WriteFile(localRel, []byte("# coordinator\n\nSeeded .local example.\n"), 0o644); err != nil {
		t.Fatalf("write .local/coordinator/README.md: %v", err)
	}
	if err := m.Write(root); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}
