package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	corpus "github.com/vhqtvn/vh-agent-harness"
	"github.com/vhqtvn/vh-agent-harness/internal/overlay"
	"github.com/vhqtvn/vh-agent-harness/internal/resolver"
)

// mediaPerceptionLivePaths are the two core-corpus LIVE paths core/media-perception
// owns (must match resolver.CoreCatalog() declaration).
var mediaPerceptionLivePaths = []string{
	".opencode/agents/media-perception.md",
	".opencode/skills/media-perception/SKILL.md",
}

// assertFilesExist asserts every rel path exists under root.
func assertFilesExist(t *testing.T, root string, rels []string, msg string) {
	t.Helper()
	for _, rel := range rels {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Errorf("%s: expected %s to exist: %v", msg, rel, err)
		}
	}
}

// assertFilesAbsent asserts every rel path does NOT exist under root.
func assertFilesAbsent(t *testing.T, root string, rels []string, msg string) {
	t.Helper()
	for _, rel := range rels {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s: expected %s to be ABSENT; stat err=%v", msg, rel, err)
		}
	}
}

// --- Slice 3 CLI integration: greenfield file rendering ---

// TestCapabilityOutput_SelectedRendersFiles proves that when core/media-perception
// IS selected, both owned files render to disk during install/update.
func TestCapabilityOutput_SelectedRendersFiles(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	writeProfile(t, root, "profile: minimal\nfeatures:\n  backlog: true\noverlays: []\npolicy_packs: []\ncapabilities:\n  - core/media-perception\n")
	if _, err := seamUpdateOut(t, root); err != nil {
		t.Fatalf("update with media-perception selected: %v", err)
	}
	assertFilesExist(t, root, mediaPerceptionLivePaths, "selected install")
}

// TestCapabilityOutput_UnselectedGreenfieldOmitsFiles proves that when
// core/media-perception is NOT selected (the default profile), a greenfield
// install does NOT create either file or the media-perception skill directory.
func TestCapabilityOutput_UnselectedGreenfieldOmitsFiles(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	// Default install profile does NOT select media-perception.
	assertFilesAbsent(t, root, mediaPerceptionLivePaths, "greenfield unselected install")
	// The skill directory should not exist either (nothing created it).
	if _, err := os.Stat(filepath.Join(root, ".opencode", "skills", "media-perception")); !os.IsNotExist(err) {
		t.Errorf("greenfield unselected: media-perception skill directory should not exist; stat err=%v", err)
	}
}

// TestCapabilityOutput_UnselectedDoctorClean proves doctor's managed-drift check
// does NOT flag the absent media-perception files as drift when the capability
// is unselected (greenfield). This is the core Slice-3 fix: active ownership
// excludes inactive paths, so they are neither counted nor compared.
func TestCapabilityOutput_UnselectedDoctorClean(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	out := seamDoctorOut(t, root)
	if strings.Contains(out, "FAIL") {
		t.Fatalf("doctor should be clean on greenfield unselected install; got:\n%s", out)
	}
}

// --- Slice 3 CLI integration: selected→deselected residue ---

// TestCapabilityOutput_SelectedThenDeselectedRetainsFiles proves the
// no-retirement contract: after selecting media-perception (files created) then
// deselecting it (update with default profile), the files REMAIN on disk as
// inactive residue — they are NOT deleted.
func TestCapabilityOutput_SelectedThenDeselectedRetainsFiles(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	// Step 1: select media-perception → files created.
	writeProfile(t, root, "profile: minimal\nfeatures:\n  backlog: true\noverlays: []\npolicy_packs: []\ncapabilities:\n  - core/media-perception\n")
	if _, err := seamUpdateOut(t, root); err != nil {
		t.Fatalf("update with media-perception selected: %v", err)
	}
	assertFilesExist(t, root, mediaPerceptionLivePaths, "selected install")

	// Step 2: deselect media-perception → files REMAIN (residue, not deleted).
	writeProfile(t, root, "profile: minimal\nfeatures:\n  backlog: true\noverlays: []\npolicy_packs: []\n")
	if _, err := seamUpdateOut(t, root); err != nil {
		t.Fatalf("update with media-perception deselected: %v", err)
	}
	assertFilesExist(t, root, mediaPerceptionLivePaths, "deselected (residue retained)")
}

// TestCapabilityOutput_ResidueDoctorNoDrift proves doctor's managed-drift check
// does NOT flag the retained residue files as drift after a selected→deselected
// transition. The active ownership map excludes inactive paths; the residue is
// not counted as managed and not byte-compared.
func TestCapabilityOutput_ResidueDoctorNoDrift(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	// Select → create files.
	writeProfile(t, root, "profile: minimal\nfeatures:\n  backlog: true\noverlays: []\npolicy_packs: []\ncapabilities:\n  - core/media-perception\n")
	if _, err := seamUpdateOut(t, root); err != nil {
		t.Fatalf("update selected: %v", err)
	}
	// Deselect → files stay as residue.
	writeProfile(t, root, "profile: minimal\nfeatures:\n  backlog: true\noverlays: []\npolicy_packs: []\n")
	if _, err := seamUpdateOut(t, root); err != nil {
		t.Fatalf("update deselected: %v", err)
	}

	out := seamDoctorOut(t, root)
	if strings.Contains(out, "FAIL") {
		t.Fatalf("doctor should be clean with inactive residue (no drift); got:\n%s", out)
	}
}

// TestCapabilityOutput_ResidueNotUnexpectedInDrift proves computeSeamDrift does
// NOT report the retained residue files as "unexpected" after a
// selected→deselected transition. The exact-path exemption in findSeamUnexpected
// skips inactive known paths.
func TestCapabilityOutput_ResidueNotUnexpectedInDrift(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	// Select → create files.
	writeProfile(t, root, "profile: minimal\nfeatures:\n  backlog: true\noverlays: []\npolicy_packs: []\ncapabilities:\n  - core/media-perception\n")
	if _, err := seamUpdateOut(t, root); err != nil {
		t.Fatalf("update selected: %v", err)
	}
	// Deselect → files stay as residue.
	writeProfile(t, root, "profile: minimal\nfeatures:\n  backlog: true\noverlays: []\npolicy_packs: []\n")
	if _, err := seamUpdateOut(t, root); err != nil {
		t.Fatalf("update deselected: %v", err)
	}

	rep, err := computeSeamDrift(root)
	if err != nil {
		t.Fatalf("computeSeamDrift: %v", err)
	}
	for _, rel := range mediaPerceptionLivePaths {
		for _, u := range rep.unexpected {
			if u == rel {
				t.Errorf("residue %s must NOT be reported as unexpected; unexpected=%v", rel, rep.unexpected)
			}
		}
	}
}

// TestCapabilityOutput_UnknownNearResidueStillUnexpected proves the residue
// exemption is EXACT-PATH only: a different file in the same directory as a
// residue file is still reported as unexpected.
func TestCapabilityOutput_UnknownNearResidueStillUnexpected(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	// Select → create media-perception agent file.
	writeProfile(t, root, "profile: minimal\nfeatures:\n  backlog: true\noverlays: []\npolicy_packs: []\ncapabilities:\n  - core/media-perception\n")
	if _, err := seamUpdateOut(t, root); err != nil {
		t.Fatalf("update selected: %v", err)
	}
	// Deselect → media-perception agent becomes residue.
	writeProfile(t, root, "profile: minimal\nfeatures:\n  backlog: true\noverlays: []\npolicy_packs: []\n")
	if _, err := seamUpdateOut(t, root); err != nil {
		t.Fatalf("update deselected: %v", err)
	}

	// Drop a BOGUS file in the SAME directory as the residue file. The exact-path
	// exemption must NOT swallow it — it is not a declared core output.
	bogusRel := ".opencode/agents/totally-bogus-stranger.md"
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(bogusRel)), []byte("bogus\n"), 0o644); err != nil {
		t.Fatalf("write bogus: %v", err)
	}

	rep, err := computeSeamDrift(root)
	if err != nil {
		t.Fatalf("computeSeamDrift: %v", err)
	}
	found := false
	for _, u := range rep.unexpected {
		if u == bogusRel {
			found = true
		}
	}
	if !found {
		t.Errorf("bogus file %s near residue must STILL be unexpected; unexpected=%v", bogusRel, rep.unexpected)
	}
}

// --- Slice 3 CLI integration: unconditional capabilities (non-regression) ---

// TestCapabilityOutput_GatedCommitUnselectedStillRenders proves core/gated-commit
// (which does NOT declare CoreOutputs) retains its current unconditional render
// behavior: its files render regardless of capability selection.
func TestCapabilityOutput_GatedCommitUnselectedStillRenders(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	// gated-commit is NOT selected in the default profile, but its files must
	// still render unconditionally (it does not declare CoreOutputs).
	for _, rel := range []string{
		".opencode/skills/gated-commit/SKILL.md",
		".opencode/agents/commit-message.md",
		".opencode/agents/commit-reviewer.md",
		".opencode/agents/committer.md",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Errorf("gated-commit unselected: %s must still render (unconditional); err=%v", rel, err)
		}
	}
}

// TestCapabilityOutput_DebateUnselectedStillRenders proves core/debate (which
// does NOT declare CoreOutputs) retains its current unconditional render
// behavior.
func TestCapabilityOutput_DebateUnselectedStillRenders(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	for _, rel := range []string{
		".opencode/agents/debate.md",
		".opencode/skills/think-mode/SKILL.md",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Errorf("debate unselected: %s must still render (unconditional); err=%v", rel, err)
		}
	}
}

// --- Declared-source existence (corpus-level guarantee) ---

// TestCoreCatalog_CoreOutputsSourceExists proves every CoreOutputs declaration in
// CoreCatalog corresponds to a real file in the embedded templates/core corpus
// (trying plain, .tmpl, and .template source forms). A declaration pointing at
// a non-existent source is a catalog bug that must be caught.
func TestCoreCatalog_CoreOutputsSourceExists(t *testing.T) {
	coreSub, err := fs.Sub(corpus.CoreFS, corpus.CoreDir)
	if err != nil {
		t.Fatalf("fs.Sub(core): %v", err)
	}
	// Walk once to build the set of all source paths (forward-slash).
	sources := map[string]bool{}
	_ = fs.WalkDir(coreSub, ".", func(p string, d fs.DirEntry, werr error) error {
		if werr != nil || d.IsDir() {
			return nil
		}
		sources[p] = true
		return nil
	})

	catalog := resolverCoreCatalogForTest(t)
	for _, m := range catalog {
		for _, live := range m.outputs {
			// Try plain, .tmpl, .template — at least one must exist.
			candidates := []string{live, live + ".tmpl", live + ".template"}
			found := false
			for _, c := range candidates {
				if sources[c] {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("CoreOutputs declaration %q (capability %s) has no matching source in templates/core (tried %v)", live, m.id, candidates)
			}
		}
	}
}

// catalogEntry is a minimal projection of a capability + its CoreOutputs for
// the source-existence test (avoids importing resolver into this test helper).
type catalogEntry struct {
	id      string
	outputs []string
}

// resolverCoreCatalogForTest reads the real CoreCatalog via the resolver package
// and projects it to the minimal shape the source-existence test needs.
func resolverCoreCatalogForTest(t *testing.T) []catalogEntry {
	t.Helper()
	merged, err := resolver.MergeCatalogs(resolver.CoreCatalog(), nil)
	if err != nil {
		t.Fatalf("build catalog: %v", err)
	}
	var out []catalogEntry
	for _, id := range merged.IDs() {
		m, ok := merged.Get(id)
		if !ok {
			continue
		}
		if len(m.CoreOutputs) == 0 {
			continue
		}
		out = append(out, catalogEntry{id: id, outputs: m.CoreOutputs})
	}
	return out
}

// --- Runtime declared-source existence (B1 fix) ---

// TestCapabilityOutput_OverlayDeclaresNonexistentCoreOutputFails proves the B1
// runtime fix: an overlay-pack capability manifest declaring a nonexistent
// CoreOutput path fails FAST at plan-compile time with the expected diagnostic.
// Without the runtime check, the phantom path would pass the resolver's
// STRUCTURAL validation (which only checks path form, not existence — the
// resolver is leaf-level with no corpus access) and enter the selection plan's
// inactive set, where findSeamUnexpected would exempt any on-disk file at that
// exact path as "residue" — masking real unexpected-file drift.
//
// This test exercises the MERGED-catalog path (core seeds PLUS an overlay-pack
// contribution): the project-local pack's capability-manifest.yml is discovered
// via discoverPackContributions, merged into the catalog, and then the runtime
// check (validateCoreOutputsSourceExistence, inside renderSeamStaging) rejects
// the nonexistent declaration before any rendering or staging.
func TestCapabilityOutput_OverlayDeclaresNonexistentCoreOutputFails(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)

	// Project-local overlay pack declaring a capability with a nonexistent
	// CoreOutput path. The manifest is structurally VALID (forward-slash,
	// relative, no traversal, no dupes) so it passes resolver structural
	// validation and enters the merged catalog — exactly the gap the runtime
	// check closes. The capability also declares a provides agent and the pack
	// carries a minimal opencode-append so it is well-formed for discovery; the
	// nonexistent core_output entry is the bug under test.
	packDir := filepath.Join(root, filepath.FromSlash(overlay.ProjectOverlaysSubdir), "bad-output-pack")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const badCapID = "project/bad-output"
	const phantomPath = ".opencode/agents/not-real.md"
	manifest := "id: " + badCapID + "\n" +
		"provides:\n  - bad-output-agent\n" +
		"core_outputs:\n  - " + phantomPath + "\n"
	if err := os.WriteFile(filepath.Join(packDir, "capability-manifest.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	// Select the pack via overlays: so its manifest is discovered and its
	// capability enters the merged catalog. The pack is listed in overlays: so
	// activeOverlays returns it and the capability is selected; either way
	// (selected or not) the runtime check fires on the MERGED catalog.
	writeProfile(t, root, "profile: supervised\nfeatures:\n  backlog: true\noverlays: [bad-output-pack]\npolicy_packs: []\n")

	_, err := seamUpdateOut(t, root)
	if err == nil {
		t.Fatalf("update MUST fail when an overlay declares a nonexistent CoreOutput; got nil err (silent acceptance = the B1 regression)")
	}
	// The diagnostic must name the offending capability, the missing path, and
	// the existence-check guarantee so the operator can fix the pack manifest.
	if !strings.Contains(err.Error(), badCapID) {
		t.Errorf("error must name the offending capability id %q; got: %v", badCapID, err)
	}
	if !strings.Contains(err.Error(), phantomPath) {
		t.Errorf("error must name the missing CoreOutput path %q; got: %v", phantomPath, err)
	}
	if !strings.Contains(err.Error(), "does not exist in the core corpus") {
		t.Errorf("error must carry the existence-check diagnostic; got: %v", err)
	}
}

// TestCapabilityOutput_RuntimeCheckPassesForValidCoreCatalog is the non-regression
// guard for the B1 fix: the runtime check must NOT false-positive on the real
// merged catalog (core seeds + embedded release pack + the dogfood project pack
// when present). The real catalog's CoreOutputs all resolve to real source files,
// so a plain update must still succeed. This pins that the check is precise, not
// a blanket reject.
func TestCapabilityOutput_RuntimeCheckPassesForValidCoreCatalog(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	// Default profile — core/media-perception is the only CoreOutputs-declaring
	// capability in the core catalog, and it is unselected here. The runtime
	// check must accept it (both paths exist in the corpus).
	if _, err := seamUpdateOut(t, root); err != nil {
		t.Fatalf("update must succeed on the default catalog (valid CoreOutputs); got: %v", err)
	}
	// And with media-perception SELECTED — the check fires on the same paths.
	writeProfile(t, root, "profile: minimal\nfeatures:\n  backlog: true\noverlays: []\npolicy_packs: []\ncapabilities:\n  - core/media-perception\n")
	if _, err := seamUpdateOut(t, root); err != nil {
		t.Fatalf("update with media-perception selected must succeed (valid CoreOutputs); got: %v", err)
	}
}
