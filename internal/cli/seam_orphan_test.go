package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/overlay"
	"github.com/vhqtvn/vh-agent-harness/internal/renderstate"
)

// These tests cover the P1-LINEAGE-002 report-only orphan-detection surface:
// rendered overlay skills whose source has been removed are surfaced as
// preserved orphans (in both `update` and `update --dry-run`), while pack
// deselection (source still present) and project-added skill dirs (never
// recorded) are correctly NOT flagged. They drive the real seam
// install/update path against a project-local overlay pack.
//
// NOTE on marker choice: the update/dry-run output always echoes the target
// path, and these test names contain "Orphan" (t.TempDir derives the dir name
// from the test function). A naive strings.Contains(out, "orphan") would match
// the PATH, not an actual orphan report. orphanMarkerPresent therefore checks
// for the distinctive marker "preserved orphan skill" which never appears in a
// path, and orphanReported requires that marker AND the skill name.

// testPackSelects is a valid profile that selects the orphan-test pack.
const testPackSelects = "profile: minimal\nmodules: [core]\nfeatures:\n  backlog: false\noverlays: [orphan-test]\npolicy_packs: []\n"

// testPackDeselects is a valid profile with the pack deselected (source remains
// on disk; this must NOT produce an orphan).
const testPackDeselects = "profile: minimal\nmodules: [core]\nfeatures:\n  backlog: false\noverlays: []\npolicy_packs: []\n"

// writeTestSkillPack writes a project-local overlay pack (orphan-test) with a
// single skill file at skills/<skill>/SKILL.md. This is the producer source a
// later removal turns into a reported orphan.
func writeTestSkillPack(t *testing.T, root, skill, body string) {
	t.Helper()
	packDir := filepath.Join(root, filepath.FromSlash(overlay.ProjectOverlaysSubdir), "orphan-test")
	skillDir := filepath.Join(packDir, "skills", skill)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill source dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write skill source: %v", err)
	}
}

// removeTestSkillSource deletes the producer source for a skill, leaving the
// rendered destination in place (the exact bug condition: source gone, rendered
// file still live).
func removeTestSkillSource(t *testing.T, root, skill string) {
	t.Helper()
	src := filepath.Join(root, filepath.FromSlash(overlay.ProjectOverlaysSubdir), "orphan-test", "skills", skill)
	if err := os.RemoveAll(src); err != nil {
		t.Fatalf("remove skill source: %v", err)
	}
}

// renderedSkillDir returns the absolute path to a rendered skill's directory
// under .opencode/skills/.
func renderedSkillDir(root, skill string) string {
	return filepath.Join(root, ".opencode", "skills", skill)
}

// orphanMarkerPresent reports whether the output carries the distinctive
// preserved-orphan marker. This avoids false positives from a temp-dir PATH
// that happens to contain "Orphan" (lowercased "orphan").
func orphanMarkerPresent(out string) bool {
	return strings.Contains(strings.ToLower(out), "preserved orphan skill")
}

// orphanReported reports whether the output surfaces a preserved-orphan notice
// naming the given skill.
func orphanReported(out, skill string) bool {
	return orphanMarkerPresent(out) && strings.Contains(out, skill)
}

// renderBaseline installs, selects the orphan-test pack, renders its one skill,
// and confirms the manifest + rendered file exist. Returns the root.
func renderBaseline(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	seamInstallInto(t, root)
	writeProfile(t, root, testPackSelects)
	writeTestSkillPack(t, root, "ghost-skill", "# ghost-skill (test fixture)\n")
	if out, err := seamUpdateOut(t, root); err != nil {
		t.Fatalf("baseline update with orphan-test pack: %v (out=%q)", err, out)
	}
	// Manifest now exists; the rendered skill is on disk and recorded.
	if _, err := os.Stat(renderstate.FilePath(root)); err != nil {
		t.Fatalf("manifest must exist after baseline render: %v", err)
	}
	if !pathExists(t, renderedSkillDir(root, "ghost-skill")) {
		t.Fatal("rendered ghost-skill dir must exist after baseline render")
	}
	return root
}

// TestSeamOrphan_SourceRemoval_FlaggedAndReported: the core bug fix. After a
// skill's overlay source is removed, `update` must surface the previously-
// rendered skill as a preserved orphan and leave it on disk (no deletion).
func TestSeamOrphan_SourceRemoval_FlaggedAndReported(t *testing.T) {
	root := renderBaseline(t)
	// Remove the producer source. The rendered destination stays in place.
	removeTestSkillSource(t, root, "ghost-skill")

	out, err := seamUpdateOut(t, root)
	if err != nil {
		t.Fatalf("update after source removal: %v (out=%q)", err, out)
	}
	if !orphanReported(out, "ghost-skill") {
		t.Errorf("update must report ghost-skill as a preserved orphan after its source was removed; got:\n%s", out)
	}
	// Report-only: the rendered file is preserved (NOT deleted).
	if !pathExists(t, renderedSkillDir(root, "ghost-skill")) {
		t.Error("rendered ghost-skill dir must STILL EXIST after update (report-only, no deletion)")
	}
}

// TestSeamOrphan_PackDeselection_NotFlagged: deselecting a pack (removing it
// from overlays:) while its source dir is still on disk must NOT produce an
// orphan. The source is still present — only unselected — so the provenance
// check correctly classifies it as not-an-orphan.
func TestSeamOrphan_PackDeselection_NotFlagged(t *testing.T) {
	root := renderBaseline(t)
	// Deselect the pack (source dir remains on disk).
	writeProfile(t, root, testPackDeselects)

	out, err := seamUpdateOut(t, root)
	if err != nil {
		t.Fatalf("update after deselect: %v (out=%q)", err, out)
	}
	if orphanMarkerPresent(out) {
		t.Errorf("deselected-but-source-present must NOT be flagged as a preserved orphan; got:\n%s", out)
	}
}

// TestSeamOrphan_ProjectAddedSkillDir_NotFlagged: a skill dir a project adds
// directly under .opencode/skills/ (never produced by an overlay render) is
// never recorded in the manifest, so it can never be flagged as an orphan.
func TestSeamOrphan_ProjectAddedSkillDir_NotFlagged(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	// Project adds its OWN skill dir directly (no overlay pack involved).
	manual := renderedSkillDir(root, "manual-project-skill")
	if err := os.MkdirAll(manual, 0o755); err != nil {
		t.Fatalf("mkdir manual skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(manual, "SKILL.md"), []byte("# manual\n"), 0o644); err != nil {
		t.Fatalf("write manual skill: %v", err)
	}
	// First update: the manual skill is NOT an overlay output, so it is never
	// recorded in the manifest and never compared.
	out, err := seamUpdateOut(t, root)
	if err != nil {
		t.Fatalf("update: %v (out=%q)", err, out)
	}
	if orphanMarkerPresent(out) {
		t.Errorf("project-added skill dir must NEVER be flagged as an orphan; got:\n%s", out)
	}
	// A second update still must not flag it (it was never recorded).
	out2, err := seamUpdateOut(t, root)
	if err != nil {
		t.Fatalf("second update: %v (out=%q)", err, out2)
	}
	if orphanMarkerPresent(out2) {
		t.Errorf("project-added skill dir must NEVER be flagged across runs; got:\n%s", out2)
	}
}

// TestSeamOrphan_NoManifestBootstrap: when there is no prior manifest at all
// (the true bootstrap path — e.g. a project first updated by a pre-feature
// binary), Compare receives a nil prior and must report no orphans. The current
// render establishes the baseline.
func TestSeamOrphan_NoManifestBootstrap(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	// Simulate a project that predates this feature: delete the manifest install
	// wrote, so the first overlay update sees no prior manifest at all.
	if err := os.Remove(renderstate.FilePath(root)); err != nil {
		t.Fatalf("remove manifest to simulate pre-feature project: %v", err)
	}
	writeProfile(t, root, testPackSelects)
	writeTestSkillPack(t, root, "fresh-skill", "# fresh-skill\n")
	out, err := seamUpdateOut(t, root)
	if err != nil {
		t.Fatalf("first overlay update (no prior manifest): %v (out=%q)", err, out)
	}
	if orphanMarkerPresent(out) {
		t.Errorf("no-manifest bootstrap must report NO orphans; got:\n%s", out)
	}
	// The manifest is now established from this render.
	if _, err := os.Stat(renderstate.FilePath(root)); err != nil {
		t.Fatalf("manifest must be written after the first successful update: %v", err)
	}
}

// TestSeamOrphan_FirstOverlayRender_NoOrphans: the normal first-overlay path.
// Install writes an empty manifest (no overlays selected during install). The
// first update that selects a pack renders the skill and records it; because the
// prior manifest had no records, nothing is an orphan.
func TestSeamOrphan_FirstOverlayRender_NoOrphans(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	writeProfile(t, root, testPackSelects)
	writeTestSkillPack(t, root, "ghost-skill", "# ghost-skill\n")
	out, err := seamUpdateOut(t, root)
	if err != nil {
		t.Fatalf("first overlay update: %v (out=%q)", err, out)
	}
	if orphanMarkerPresent(out) {
		t.Errorf("first overlay render (empty prior manifest) must report NO orphans; got:\n%s", out)
	}
}

// TestSeamOrphan_RepeatedReportingAcrossRuns: a reported orphan keeps reporting
// on subsequent runs until the operator removes the destination or restores the
// source (stale-record retention).
func TestSeamOrphan_RepeatedReportingAcrossRuns(t *testing.T) {
	root := renderBaseline(t)
	removeTestSkillSource(t, root, "ghost-skill")

	first, err := seamUpdateOut(t, root)
	if err != nil {
		t.Fatalf("first orphan update: %v (out=%q)", err, first)
	}
	if !orphanReported(first, "ghost-skill") {
		t.Fatalf("first update must report the orphan; got:\n%s", first)
	}
	// Second update: the orphan persists (destination still present, source
	// still missing) and must be reported again.
	second, err := seamUpdateOut(t, root)
	if err != nil {
		t.Fatalf("second orphan update: %v (out=%q)", err, second)
	}
	if !orphanReported(second, "ghost-skill") {
		t.Errorf("second update must STILL report the orphan (stale-record retention); got:\n%s", second)
	}
}

// TestSeamOrphan_SourceRestoration_ClearsOrphan: restoring the producer source
// (even without re-selecting the pack) clears the orphan notice on the next
// update, because the provenance check now finds the source present again.
func TestSeamOrphan_SourceRestoration_ClearsOrphan(t *testing.T) {
	root := renderBaseline(t)
	removeTestSkillSource(t, root, "ghost-skill")
	if first, err := seamUpdateOut(t, root); err != nil || !orphanReported(first, "ghost-skill") {
		t.Fatalf("precondition: first orphan update must report it; err=%v out=\n%s", err, first)
	}
	// Restore the source (pack dir recreated with the skill file).
	writeTestSkillPack(t, root, "ghost-skill", "# ghost-skill (restored)\n")
	// Deselect so it is not re-rendered, but the source is present — provenance
	// must classify it as not-an-orphan.
	writeProfile(t, root, testPackDeselects)

	out, err := seamUpdateOut(t, root)
	if err != nil {
		t.Fatalf("update after source restoration: %v (out=%q)", err, out)
	}
	if orphanReported(out, "ghost-skill") {
		t.Errorf("restored source must CLEAR the orphan notice; got:\n%s", out)
	}
}

// TestSeamOrphan_DryRunSurfacesOrphan: update --dry-run must surface the
// preserved orphan (the bug report required dry-run visibility). Dry-run writes
// nothing, including not rewriting the manifest.
func TestSeamOrphan_DryRunSurfacesOrphan(t *testing.T) {
	root := renderBaseline(t)
	removeTestSkillSource(t, root, "ghost-skill")

	// Snapshot the manifest bytes before the dry-run; dry-run must not rewrite it.
	before, rerr := os.ReadFile(renderstate.FilePath(root))
	if rerr != nil {
		t.Fatalf("read manifest before dry-run: %v", rerr)
	}
	withDryRun(t, true)
	out, err := runUpdateTarget(t, root)
	if err != nil {
		t.Fatalf("dry-run update after source removal: %v (out=%q)", err, out)
	}
	if !orphanReported(out, "ghost-skill") {
		t.Errorf("dry-run must surface the preserved orphan; got:\n%s", out)
	}
	// Dry-run wrote nothing: manifest bytes unchanged.
	after, rerr := os.ReadFile(renderstate.FilePath(root))
	if rerr != nil {
		t.Fatalf("read manifest after dry-run: %v", rerr)
	}
	if string(before) != string(after) {
		t.Errorf("dry-run must NOT rewrite the manifest; bytes changed (before=%d after=%d)", len(before), len(after))
	}
	// And the rendered file is still on disk.
	if !pathExists(t, renderedSkillDir(root, "ghost-skill")) {
		t.Error("rendered ghost-skill dir must still exist after dry-run")
	}
}
