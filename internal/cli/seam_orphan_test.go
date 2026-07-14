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

// --- helpers for the v1.1 ship-review fix tests (blocker #1 + #2) ------------

// writeTestSkillFile writes an ADDITIONAL source file into the orphan-test skill
// pack dir (besides SKILL.md), so a single rendered skill dir can carry multiple
// files (the mixed-directory precondition for blocker #2).
func writeTestSkillFile(t *testing.T, root, skill, file, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(overlay.ProjectOverlaysSubdir), "orphan-test", "skills", skill, file)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir skill source dir for %s: %v", file, err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write skill source file %s: %v", file, err)
	}
}

// removeTestSkillFile removes a SINGLE source file from a test skill dir (leaving
// siblings in place) — the file-level removal precondition.
func removeTestSkillFile(t *testing.T, root, skill, file string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(overlay.ProjectOverlaysSubdir), "orphan-test", "skills", skill, file)
	if err := os.Remove(p); err != nil {
		t.Fatalf("remove skill source file %s: %v", file, err)
	}
}

// manifestDestSet reads the rendered-outputs manifest and returns the normalized
// set of recorded destination paths. Returns an empty set when no manifest
// exists (bootstrap).
func manifestDestSet(t *testing.T, root string) map[string]bool {
	t.Helper()
	m, err := renderstate.Read(root)
	if err != nil {
		t.Fatalf("read rendered-outputs manifest: %v", err)
	}
	set := make(map[string]bool)
	if m == nil {
		return set
	}
	for _, e := range m.Entries {
		set[renderstate.NormalizeDestination(e.DestinationPath)] = true
	}
	return set
}

// TestSeamOrphan_ManifestSkipped_OnTrackedSkillWriteFailure (ship-review blocker
// #1, option c): when a TRACKED overlay-skill destination's live write fails
// inside Apply, the rendered-outputs manifest must NOT be advanced — the prior
// manifest is left byte-for-byte intact and a warning is emitted. This guards
// the provenance contract: the manifest records what was rendered AT PERSIST
// TIME, so persisting a fresh manifest that claims success while a tracked skill
// write silently failed would make the manifest lie. Apply's return semantics are
// UNCHANGED (it still returns nil) — only the manifest persist is gated.
//
// Decisiveness: a SECOND skill is added so a successful persist would be
// observable (the manifest gains second-skill's entry). The gate firing means
// second-skill is NOT recorded even though Apply wrote it to disk.
func TestSeamOrphan_ManifestSkipped_OnTrackedSkillWriteFailure(t *testing.T) {
	root := renderBaseline(t) // manifest = {ghost-skill/SKILL.md}
	// A second tracked skill whose write will succeed. If the manifest persists,
	// it gains this entry; if the gate fires, it does not.
	writeTestSkillPack(t, root, "second-skill", "# second-skill\n")
	// Deterministically break the tracked ghost-skill live write (NOT chmod): the
	// rendered dir is replaced by a regular FILE, so MkdirAll for its SKILL.md
	// fails inside Apply.
	ghostDir := renderedSkillDir(root, "ghost-skill")
	if err := os.RemoveAll(ghostDir); err != nil {
		t.Fatalf("remove ghost-skill rendered dir: %v", err)
	}
	if err := os.WriteFile(ghostDir, []byte("BLOCKER"), 0o644); err != nil {
		t.Fatalf("plant blocker file at ghost-skill dir path: %v", err)
	}

	out, err := seamUpdateOut(t, root)
	if err != nil {
		t.Fatalf("update must succeed (Apply returns nil on live-write failure): %v (out=%q)", err, out)
	}

	// The gate fired: prior manifest intact, second-skill NOT recorded.
	dests := manifestDestSet(t, root)
	if dests[".opencode/skills/second-skill/SKILL.md"] {
		t.Errorf("manifest must NOT persist when a tracked skill write failed (second-skill was recorded → gate did not fire)")
	}
	// ghost-skill remains recorded (the baseline manifest was left untouched).
	if !dests[".opencode/skills/ghost-skill/SKILL.md"] {
		t.Errorf("baseline ghost-skill record must remain in the untouched prior manifest")
	}
	// Apply DID run and DID write second-skill to disk (its write succeeded) —
	// confirming the failure was specific to ghost-skill and Apply returned nil.
	if !pathExists(t, filepath.Join(root, ".opencode", "skills", "second-skill", "SKILL.md")) {
		t.Error("second-skill live file must exist on disk (Apply ran and wrote it; only the manifest persist was gated)")
	}
}

// TestSeamOrphan_ManifestPersisted_OnNonSkillWriteFailure (scope boundary for
// blocker #1): a failed NON-skill managed destination must NOT gate the manifest.
// The gate fires ONLY on failed TRACKED overlay-skill destinations; a regular
// managed file (here a rendered agent) failing its write leaves the manifest
// persist path intact. This keeps the v1.1 gate tightly scoped to the orphan-
// skill provenance contract and avoids collateral suppression.
func TestSeamOrphan_ManifestPersisted_OnNonSkillWriteFailure(t *testing.T) {
	root := renderBaseline(t)
	writeTestSkillPack(t, root, "second-skill", "# second-skill\n")
	// Deterministically break a NON-skill managed write: turn a rendered agent
	// file into a DIRECTORY so its WriteFile fails (EISDIR). All tracked skill
	// writes still succeed.
	agentFile := filepath.Join(root, ".opencode", "agents", "build.md")
	if err := os.RemoveAll(agentFile); err != nil {
		t.Fatalf("remove agent file: %v", err)
	}
	if err := os.MkdirAll(agentFile, 0o755); err != nil {
		t.Fatalf("plant blocker dir at agent file path: %v", err)
	}

	out, err := seamUpdateOut(t, root)
	if err != nil {
		t.Fatalf("update must succeed: %v (out=%q)", err, out)
	}

	// Gate did NOT fire for the non-skill failure → manifest persisted with the
	// freshly rendered second-skill.
	dests := manifestDestSet(t, root)
	if !dests[".opencode/skills/second-skill/SKILL.md"] {
		t.Errorf("a failed NON-skill dest must NOT gate the manifest; second-skill missing (gate fired wrongly)")
	}
	// The non-skill write really did fail: build.md is still a directory.
	info, err := os.Stat(agentFile)
	if err != nil || !info.IsDir() {
		t.Errorf("precondition check failed: non-skill write should have failed (build.md still a dir); got info=%v err=%v", info, err)
	}
}

// TestSeamOrphan_MixedDirectory_FileAccurateReporting (ship-review blocker #2):
// when ONE file in a skill directory is orphaned (source removed) while a SIBLING
// file in the SAME directory is still actively rendered, the orphan notice must
// be FILE-ACCURATE: it names the specific orphaned file by its full destination
// path, and the guidance removes only that FILE (warning against whole-directory
// removal while an active file remains). This is the regression guard against the
// old whole-directory "delete .opencode/skills/<name>/" advice, which would have
// nuked the still-active sibling.
func TestSeamOrphan_MixedDirectory_FileAccurateReporting(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	writeProfile(t, root, testPackSelects)
	// One skill dir carrying TWO source files.
	writeTestSkillPack(t, root, "ghost-skill", "# ghost-skill\n")
	writeTestSkillFile(t, root, "ghost-skill", "NOTES.md", "# ghost notes\n")
	if out, err := seamUpdateOut(t, root); err != nil {
		t.Fatalf("baseline render of the two-file skill: %v (out=%q)", err, out)
	}
	// Both files rendered and recorded — the mixed-dir precondition.
	dests := manifestDestSet(t, root)
	if !dests[".opencode/skills/ghost-skill/SKILL.md"] || !dests[".opencode/skills/ghost-skill/NOTES.md"] {
		t.Fatalf("baseline must record BOTH skill files in the dir; got %v", dests)
	}
	// Remove ONLY NOTES.md's source. SKILL.md stays active. NOTES.md becomes an
	// orphan living in a dir that still has an actively-rendered file.
	removeTestSkillFile(t, root, "ghost-skill", "NOTES.md")

	out, err := seamUpdateOut(t, root)
	if err != nil {
		t.Fatalf("update after partial source removal: %v (out=%q)", err, out)
	}
	if !orphanMarkerPresent(out) {
		t.Fatalf("update must report the preserved orphan; got:\n%s", out)
	}
	// File-accurate: the SPECIFIC orphaned file is named by its destination path.
	if !strings.Contains(out, "NOTES.md") {
		t.Errorf("mixed-dir report must name the SPECIFIC orphaned FILE (NOTES.md); got:\n%s", out)
	}
	// The guidance must be file-level (not the old whole-directory advice): it
	// warns to verify EVERY file is orphaned before removing the directory.
	if !strings.Contains(out, "verifying EVERY file in it is orphaned") {
		t.Errorf("guidance must be file-level (verify-every-file before dir removal); got:\n%s", out)
	}
	// The active sibling is untouched on disk (only NOTES.md was orphaned).
	if !pathExists(t, filepath.Join(root, ".opencode", "skills", "ghost-skill", "SKILL.md")) {
		t.Error("the still-active SKILL.md must remain on disk (only NOTES.md was orphaned)")
	}
}

// TestOverlayChecker_ProjectLocalPackUnreadable_IsIndeterminate pins the
// tri-state contract's fail-safe for the path where the producer pack's
// project-local dir is UNREADABLE (a non-ErrNotExist stat error). Without the
// direct probe in CheckSource, OpenPackFor swallows that error, falls back to
// an absent embedded pack, and surfaces fs.ErrNotExist — which would
// false-positive as SourceMissing (a definite orphan). The fix classifies such
// a transient/unreadable state as SourceIndeterminate so a transient error
// never produces a false-positive orphan.
//
// The injection is deterministic and chmod-free: making the overlays PARENT a
// regular file causes os.Stat of overlays/<pack> to return ENOTDIR (verified to
// NOT satisfy errors.Is(err, fs.ErrNotExist)). This is a focused checker unit
// test; the downstream effects of SourceIndeterminate (Compare warns + skips the
// finding; NextManifest does not retain the stale record) are covered by
// internal/renderstate/manifest_test.go
// (TestCompare_SourceIndeterminate_NotFlagged_Warns,
// TestNextManifest_SourceIndeterminate_NotRetained).
func TestOverlayChecker_ProjectLocalPackUnreadable_IsIndeterminate(t *testing.T) {
	root := t.TempDir()
	// Normal project-local producer pack with one skill source.
	writeTestSkillPack(t, root, "ghost-skill", "# body\n")
	rec := renderstate.Record{
		DestinationPath:    renderstate.NormalizeDestination(".opencode/skills/ghost-skill/SKILL.md"),
		ProducerKind:       "overlay_skill",
		OverlayPack:        "orphan-test",
		SourceRelativePath: "skills/ghost-skill/SKILL.md",
	}
	checker := overlaySkillSourceChecker{target: root}

	// Baseline: source present and readable -> SourcePresent.
	if got := checker.CheckSource(rec); got != renderstate.SourcePresent {
		t.Fatalf("baseline (source present): got %s want %s", got, renderstate.SourcePresent)
	}

	// Source removed -> confirmed missing (definite-orphan candidate).
	removeTestSkillSource(t, root, "ghost-skill")
	if got := checker.CheckSource(rec); got != renderstate.SourceMissing {
		t.Fatalf("removed source: got %s want %s", got, renderstate.SourceMissing)
	}

	// Now corrupt the producer-pack dir state so os.Stat returns a
	// non-ErrNotExist error (ENOTDIR via a file where the overlays dir is
	// expected). The source is still genuinely absent underneath, but the
	// producer-pack state is indeterminate — the checker must NOT fall through
	// to the embedded pack and false-positive as SourceMissing.
	overlaysDir := filepath.Join(root, filepath.FromSlash(overlay.ProjectOverlaysSubdir))
	if err := os.RemoveAll(overlaysDir); err != nil {
		t.Fatalf("remove overlays dir: %v", err)
	}
	if err := os.WriteFile(overlaysDir, []byte("x"), 0o644); err != nil {
		t.Fatalf("plant file at overlays path: %v", err)
	}
	if got := checker.CheckSource(rec); got != renderstate.SourceIndeterminate {
		t.Fatalf("unreadable project-local pack: got %s want %s (must not false-positive as missing)", got, renderstate.SourceIndeterminate)
	}
}
