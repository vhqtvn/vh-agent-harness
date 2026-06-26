package drift

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/manifest"
)

// writeFiles is a tiny helper that writes the given files under root.
func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
}

// manifestFor builds a manifest tracking the given paths with their on-disk
// hashes (so they start as `ok`).
func manifestFor(t *testing.T, root string, rels []string) *manifest.Manifest {
	t.Helper()
	m := manifest.New()
	for _, rel := range rels {
		h, err := hashFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("hash %s: %v", rel, err)
		}
		m.Files[rel] = manifest.File{Hash: h, Class: manifest.ClassManaged}
	}
	return m
}

func TestCompute_AllOK(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".opencode/agents/researcher.md": "hello",
		".opencode/scripts/state-lib.js": "lib",
	})
	rels := []string{".opencode/agents/researcher.md", ".opencode/scripts/state-lib.js"}
	m := manifestFor(t, root, rels)

	r, err := Compute(root, m)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if r.HasProblems() {
		t.Errorf("expected no problems, got %+v", r.Counts)
	}
	if r.Counts[OK] != 2 {
		t.Errorf("OK = %d, want 2", r.Counts[OK])
	}
}

func TestCompute_DriftedMissingUnexpected(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".opencode/agents/researcher.md": "original", // will stay ok
		".opencode/agents/planner.md":    "original", // will be edited -> drifted
		".opencode/agents/ghost.md":      "leftover", // unexpected (not in manifest)
	})

	// researcher.md and planner.md start on disk with "original" content; hash
	// them now. missing.md is in the manifest but NOT on disk, so it gets a
	// recorded (dummy) hash without hashing disk.
	researcherHash, _ := hashFile(filepath.Join(root, ".opencode/agents/researcher.md"))
	plannerHash, _ := hashFile(filepath.Join(root, ".opencode/agents/planner.md"))
	m := manifest.New()
	m.Files[".opencode/agents/researcher.md"] = manifest.File{Hash: researcherHash, Class: manifest.ClassManaged}
	m.Files[".opencode/agents/planner.md"] = manifest.File{Hash: plannerHash, Class: manifest.ClassManaged}
	m.Files[".opencode/agents/missing.md"] = manifest.File{Hash: "sha256:dummy-recorded-hash", Class: manifest.ClassManaged}

	// Edit planner.md so its hash no longer matches the manifest.
	if err := os.WriteFile(filepath.Join(root, ".opencode/agents/planner.md"), []byte("EDITED"), 0o644); err != nil {
		t.Fatalf("edit planner: %v", err)
	}

	r, err := Compute(root, m)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if got := r.Counts[OK]; got != 1 {
		t.Errorf("OK = %d, want 1", got)
	}
	if got := r.Counts[Drifted]; got != 1 {
		t.Errorf("Drifted = %d, want 1 (planner.md)", got)
	}
	if got := r.Counts[Missing]; got != 1 {
		t.Errorf("Missing = %d, want 1 (missing.md)", got)
	}
	if got := r.Counts[Unexpected]; got != 1 {
		t.Errorf("Unexpected = %d, want 1 (ghost.md)", got)
	}
	if !r.HasProblems() {
		t.Errorf("HasProblems = false, want true")
	}

	// Locate the specific unexpected entry.
	found := false
	for _, e := range r.Entries {
		if e.Path == ".opencode/agents/ghost.md" && e.Category == Unexpected {
			found = true
		}
	}
	if !found {
		t.Errorf("did not report ghost.md as unexpected")
	}
}

// TestCompute_StateDirNotFlagged verifies runtime-state subtrees
// (.opencode/state, .opencode/plans) and the manifest file itself are NOT
// flagged as unexpected even though they live under .opencode/.
func TestCompute_StateDirNotFlagged(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".opencode/agents/researcher.md":               "x",
		".opencode/state/sessions/abc/memory/brief.md": "runtime",
		".opencode/plans/proj/plan.md":                 "runtime",
		".opencode/harness-manifest.json":              "{}",
	})
	rels := []string{".opencode/agents/researcher.md"}
	m := manifestFor(t, root, rels)

	r, err := Compute(root, m)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if r.Counts[Unexpected] != 0 {
		t.Errorf("Unexpected = %d, want 0 (state/plans/manifest must be excluded); entries: %+v", r.Counts[Unexpected], r.Entries)
	}
}

// TestCompute_GuardDepsExempt verifies the shell-guard guard-deps install
// OUTPUT surface is NOT flagged as unexpected, on the same principled footing
// as the local_only state/plans subtrees. The surface is what `npm install`
// produces under .opencode/ from the authored package.json: the node_modules/
// tree AND the package-lock.json lockfile — both external_generated. This is
// the fix for the doctor-UNHEALTHY-after-npm-install gap surfaced by the
// minimal-profile proof.
func TestCompute_GuardDepsExempt(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".opencode/agents/researcher.md":                      "x",
		".opencode/package.json":                              `{"name":"guard"}`, // authored input (tracked below)
		".opencode/node_modules/web-tree-sitter/package.json": `{"name":"web-tree-sitter"}`,
		".opencode/node_modules/.package-lock.json":           `{"lockfileVersion":3}`,
		".opencode/node_modules/node-addon-api/index.js":      "native addon glue",
		".opencode/package-lock.json":                         `{"lockfileVersion":3}`, // npm-install output
		".opencode/harness-manifest.json":                     "{}",
	})
	rels := []string{".opencode/agents/researcher.md", ".opencode/package.json"}
	m := manifestFor(t, root, rels)

	r, err := Compute(root, m)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if r.Counts[Unexpected] != 0 {
		t.Errorf("Unexpected = %d, want 0 (guard-deps node_modules + package-lock.json are external_generated and must be drift-exempt); entries: %+v",
			r.Counts[Unexpected], r.Entries)
	}
}

// TestCompute_GuardDepsExemptIsNarrow proves the guard-deps exemptions are NOT
// blanket masks: a nested node_modules under a non-exempt subtree, a stray
// tracked-source file under .opencode/, AND a same-named lockfile elsewhere are
// all still surfaced as unexpected drift. The node_modules exemption matches
// only the top-level .opencode/node_modules subtree; the lockfile exemption
// names exactly .opencode/package-lock.json.
func TestCompute_GuardDepsExemptIsNarrow(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".opencode/agents/researcher.md":            "x",
		".opencode/agents/node_modules/stray.js":    "stray", // nested, NOT the guard-deps
		".opencode/scripts/ghost.js":                "stray", // stray source in a managed subtree
		".opencode/some-tool/node_modules/stray.js": "stray", // nested under another subtree
		".opencode/some-tool/package-lock.json":     "stray", // same name, NOT the guard-deps lockfile
	})
	rels := []string{".opencode/agents/researcher.md"}
	m := manifestFor(t, root, rels)

	r, err := Compute(root, m)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if r.Counts[Unexpected] != 4 {
		t.Fatalf("Unexpected = %d, want 4 (nested node_modules + ghost.js + some-tool node_modules + some-tool lockfile must all still be caught); entries: %+v",
			r.Counts[Unexpected], r.Entries)
	}
	caught := map[string]bool{}
	for _, e := range r.Entries {
		if e.Category == Unexpected {
			caught[e.Path] = true
		}
	}
	for _, want := range []string{
		".opencode/agents/node_modules/stray.js",
		".opencode/scripts/ghost.js",
		".opencode/some-tool/node_modules/stray.js",
		".opencode/some-tool/package-lock.json",
	} {
		if !caught[want] {
			t.Errorf("expected %s flagged unexpected (narrow exemption must not mask it)", want)
		}
	}
}

// TestCompute_NoManifestFiles verifies that a manifest with zero tracked files
// still scans for unexpected entries without error.
func TestCompute_NoManifestFiles(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".opencode/agents/stray.md": "x",
	})
	m := manifest.New() // empty Files

	r, err := Compute(root, m)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if r.Counts[Unexpected] != 1 {
		t.Errorf("Unexpected = %d, want 1", r.Counts[Unexpected])
	}
}

func TestCategory_String(t *testing.T) {
	cases := map[Category]string{
		OK:         "ok",
		Drifted:    "drifted",
		Missing:    "missing",
		Unexpected: "unexpected",
	}
	for c, want := range cases {
		if got := c.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", c, got, want)
		}
	}
}
