package cli

// Tests for `update --prune-orphans` (Slice B). The flag consumes the existing
// report-only orphan findings and auto-deletes the byte-identical (DestUnchanged)
// ones while refusing hand-edited (DestModified) ones for manual `rm`. These
// tests pin the safe-delete contract:
//
//   - DestUnchanged → deleted (and the orphan clears on the next run);
//   - DestModified  → refused + reported, NOT deleted;
//   - --dry-run     → deletes nothing, previews the would-be actions;
//   - non-orphan files are never touched;
//   - default (no flag) still preserves and reports (regression);
//   - the ownership safety floor (isProjectOwnedOrphan) never lets a
//     project-owned path through to deletion.
//
// The project-owned safety floor is exercised at the predicate seam
// (isProjectOwnedOrphan) because the ownership system structurally guarantees
// no real orphan can resolve project_owned: the rendered-outputs manifest
// records only harness-rendered overlay skill files (never project-owned), and
// ownership.Resolve rejects an override targeting a now-sourceless path, which
// would have aborted the apply before prune ran. The predicate unit test
// therefore constructs a hand-built classifier to prove the gate directly.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/ownership"
	"github.com/vhqtvn/vh-agent-harness/internal/renderstate"
	"github.com/vhqtvn/vh-agent-harness/internal/substrate"
)

// withPruneOrphans sets the --prune-orphans flag seam for the test and restores
// it on cleanup (mirrors withDryRun / withForce).
func withPruneOrphans(t *testing.T, v bool) {
	t.Helper()
	saved := updatePruneOrphans
	t.Cleanup(func() { updatePruneOrphans = saved })
	updatePruneOrphans = v
}

// renderedSkillFile returns the absolute path to a rendered skill's SKILL.md.
func renderedSkillFile(root, skill string) string {
	return filepath.Join(renderedSkillDir(root, skill), "SKILL.md")
}

// pruneSummaryPresent reports whether the output carries the prune summary line.
func pruneSummaryPresent(out string) bool {
	return strings.Contains(out, "prune-orphans summary:")
}

// TestUpdatePruneOrphans_DestUnchanged_Deleted: the core happy path. After a
// skill's overlay source is removed, `update --prune-orphans` deletes the
// byte-identical rendered file (DestUnchanged) and a subsequent plain update
// reports no orphan (the record retires because the destination is gone).
func TestUpdatePruneOrphans_DestUnchanged_Deleted(t *testing.T) {
	root := renderBaseline(t)
	dest := renderedSkillFile(root, "ghost-skill")
	removeTestSkillSource(t, root, "ghost-skill")

	withPruneOrphans(t, true)
	out, err := runUpdateTarget(t, root)
	if err != nil {
		t.Fatalf("prune update: %v (out=%q)", err, out)
	}
	if !strings.Contains(out, "deleted (byte-identical") {
		t.Errorf("prune must report the DestUnchanged orphan as deleted; got:\n%s", out)
	}
	if !pruneSummaryPresent(out) {
		t.Errorf("prune must print a summary; got:\n%s", out)
	}
	// The rendered file is GONE (auto-deleted).
	if pathExists(t, dest) {
		t.Errorf("DestUnchanged orphan must be DELETED by --prune-orphans; file still present at %s", dest)
	}
	// A subsequent plain update reports NO orphan: the record was retired
	// because its destination is now gone (diskDigest == "").
	second, err := seamUpdateOut(t, root)
	if err != nil {
		t.Fatalf("follow-up update: %v (out=%q)", err, second)
	}
	if orphanReported(second, "ghost-skill") {
		t.Errorf("a pruned orphan must not re-report on the next update (record retired); got:\n%s", second)
	}
}

// TestUpdatePruneOrphans_DestModified_RefusedNotDeleted: a hand-edited orphan
// (DestModified) must ALWAYS be refused for manual removal and NEVER deleted,
// even with --prune-orphans.
func TestUpdatePruneOrphans_DestModified_RefusedNotDeleted(t *testing.T) {
	root := renderBaseline(t)
	dest := renderedSkillFile(root, "ghost-skill")
	// Hand-edit the rendered file so its bytes differ from the recorded render
	// digest → DestModified (operator content present).
	if err := os.WriteFile(dest, []byte("# hand-edited by operator\n"), 0o644); err != nil {
		t.Fatalf("hand-edit rendered skill: %v", err)
	}
	removeTestSkillSource(t, root, "ghost-skill")

	withPruneOrphans(t, true)
	out, err := runUpdateTarget(t, root)
	if err != nil {
		t.Fatalf("prune update: %v (out=%q)", err, out)
	}
	if !strings.Contains(out, "refuse (hand-edited") {
		t.Errorf("prune must REFUSE the DestModified orphan and report it for manual removal; got:\n%s", out)
	}
	// The hand-edited file is PRESERVED (never auto-deleted).
	if !pathExists(t, dest) {
		t.Error("DestModified orphan must NOT be deleted (operator content); file is gone")
	}
	// And the summary must show zero deletions, one refused.
	if !strings.Contains(out, "0 file(s) deleted") {
		t.Errorf("summary must show zero deletions for an all-modified prune set; got:\n%s", out)
	}
}

// TestUpdatePruneOrphans_DryRun_DeletesNothing: --prune-orphans composes with
// --dry-run — it previews the would-be actions (would-delete / refuse) and
// deletes nothing.
func TestUpdatePruneOrphans_DryRun_DeletesNothing(t *testing.T) {
	root := renderBaseline(t)
	dest := renderedSkillFile(root, "ghost-skill")
	// Snapshot the manifest: dry-run must not rewrite it.
	manifestPath := renderstate.FilePath(root)
	before, rerr := os.ReadFile(manifestPath)
	if rerr != nil {
		t.Fatalf("read manifest before dry-run: %v", rerr)
	}
	removeTestSkillSource(t, root, "ghost-skill")

	withPruneOrphans(t, true)
	withDryRun(t, true)
	out, err := runUpdateTarget(t, root)
	if err != nil {
		t.Fatalf("prune dry-run: %v (out=%q)", err, out)
	}
	if !strings.Contains(out, "would delete (byte-identical") {
		t.Errorf("dry-run prune must preview the would-be deletion; got:\n%s", out)
	}
	if !strings.Contains(out, "would be deleted") {
		t.Errorf("dry-run prune summary must say 'would be deleted'; got:\n%s", out)
	}
	// Dry-run deleted nothing: the file is still on disk.
	if !pathExists(t, dest) {
		t.Error("dry-run must NOT delete the orphan; file is gone")
	}
	// Dry-run wrote nothing: the manifest bytes are unchanged.
	after, rerr := os.ReadFile(manifestPath)
	if rerr != nil {
		t.Fatalf("read manifest after dry-run: %v", rerr)
	}
	if string(before) != string(after) {
		t.Errorf("dry-run must NOT rewrite the manifest; bytes changed (before=%d after=%d)", len(before), len(after))
	}
}

// TestUpdatePruneOrphans_DefaultStillPreserves: without --prune-orphans the
// existing report-only behavior is unchanged — orphans are listed and left in
// place (regression guard for the else-branch).
func TestUpdatePruneOrphans_DefaultStillPreserves(t *testing.T) {
	root := renderBaseline(t)
	dest := renderedSkillFile(root, "ghost-skill")
	removeTestSkillSource(t, root, "ghost-skill")

	// No --prune-orphans.
	out, err := runUpdateTarget(t, root)
	if err != nil {
		t.Fatalf("default update: %v (out=%q)", err, out)
	}
	if !orphanReported(out, "ghost-skill") {
		t.Errorf("default update must still report the preserved orphan; got:\n%s", out)
	}
	if pruneSummaryPresent(out) {
		t.Errorf("default update must NOT print a prune summary; got:\n%s", out)
	}
	if !pathExists(t, dest) {
		t.Error("default update must preserve the orphan (no deletion)")
	}
}

// TestUpdatePruneOrphans_NonOrphanUntouched: an actively-rendered skill (source
// still present) is never an orphan, so --prune-orphans touches nothing.
func TestUpdatePruneOrphans_NonOrphanUntouched(t *testing.T) {
	root := renderBaseline(t) // ghost-skill actively rendered (source present)
	dest := renderedSkillFile(root, "ghost-skill")

	withPruneOrphans(t, true)
	out, err := runUpdateTarget(t, root)
	if err != nil {
		t.Fatalf("prune update on a non-orphan tree: %v (out=%q)", err, out)
	}
	if pruneSummaryPresent(out) {
		t.Errorf("prune must print NO summary when there are no orphans; got:\n%s", out)
	}
	if !pathExists(t, dest) {
		t.Error("a non-orphan actively-rendered skill must NOT be touched by --prune-orphans")
	}
}

// TestIsProjectOwnedOrphan_SafetyFloor: the predicate that gates deletion must
// refuse ONLY a positively-resolved project_owned path. An unclassified path
// (the normal case for a sourceless custom skill — off the ownership map under
// the fail-closed policy) stays eligible, and a nil classifier never refuses.
// This is the safety floor the task requires (mirrors uninstall --force). It is
// tested at the predicate seam because the ownership system structurally
// guarantees no REAL orphan can reach project_owned (an override targeting a
// now-sourceless path is rejected by Resolve and aborts the apply first).
func TestIsProjectOwnedOrphan_SafetyFloor(t *testing.T) {
	const orphanPath = ".opencode/skills/ghost-skill/SKILL.md"

	// project_owned exact entry → refuse (the safety floor fires).
	owned := substrate.NewClassifier(ownership.EffectiveMap{
		orphanPath: {Class: ownership.ClassProjectOwned, Provenance: "test"},
	}, nil)
	if !isProjectOwnedOrphan(owned, orphanPath) {
		t.Error("a project_owned orphan path must be refused by the safety floor")
	}

	// platform_managed exact entry → eligible (NOT refused).
	managed := substrate.NewClassifier(ownership.EffectiveMap{
		orphanPath: {Class: ownership.ClassPlatformManaged, Provenance: "test"},
	}, nil)
	if isProjectOwnedOrphan(managed, orphanPath) {
		t.Error("a platform_managed orphan must stay eligible (safety floor must not fire)")
	}

	// Unclassified (off-map, fail-closed default policy) → eligible. This is the
	// real-world shape: a sourceless custom skill path is not on the ownership
	// map, so Classify returns ok=false and the path is treated as platform-
	// controlled per the structural guarantee.
	empty := substrate.NewClassifier(ownership.EffectiveMap{}, nil)
	if isProjectOwnedOrphan(empty, orphanPath) {
		t.Error("an unclassified orphan path must stay eligible (ok=false is not project_owned)")
	}

	// nil classifier (classifier-build failure path is handled by the caller) →
	// never refuses on its own; the caller refuse-safes instead.
	if isProjectOwnedOrphan(nil, orphanPath) {
		t.Error("a nil classifier must not refuse (caller handles refuse-safe)")
	}
}

// --- B1: path-traversal / absolute-destination safety guard ------------------
//
// applyPruneOrphans computes the os.Remove target as
// filepath.Join(target, filepath.FromSlash(o.DestinationPath)). Without a
// containment check, a manifest record whose DestinationPath traverses (e.g.
// "../victim") — with a matching recorded digest on an external file and a
// missing source — is classified DestUnchanged and os.Remove'd OUTSIDE the
// project. The orphanPathEscapesTarget guard (B1) refuses such a path: it is
// reported for manual review and NEVER deleted, in both live and --dry-run
// modes. These tests pin that contract end-to-end and at the predicate seam.
//
// NOTE on the absolute-path case: filepath.Join concatenates rather than
// resetting on an absolute element, so an absolute DestinationPath actually
// NESTS under target (filepath.Join("/root","/etc/passwd")="/root/etc/passwd")
// and therefore cannot escape via os.Remove. orphanPathEscapesTarget still
// rejects absolute destinations outright (IsAbs) as defense-in-depth — pinned
// at the predicate seam below — but the end-to-end escape that reaches the
// delete site is the "../" traversal, which is what the live/dry-run tests
// exercise (and which FAILS on unpatched code: the external victim would be
// deleted).

// plantTraversalOrphan turns the baseline ghost-skill record into a
// path-traversal orphan pointing at an external victim file whose bytes match
// the recorded digest. It returns (victimPath, traversalDest). It is the shared
// setup for the B1 live and dry-run end-to-end tests:
//   - removes the ghost-skill producer source (so Compare classifies the record
//     SourceMissing → definite-orphan candidate);
//   - plants victimBody at filepath.Dir(root)/victim-<unique> (OUTSIDE root);
//   - rewrites the manifest's ghost-skill entry: DestinationPath = the "../"
//     traversal that filepath.Join(root, traversal) resolves to victimPath, and
//     RenderedDigest = Digest(victimBody) so diskDigest classifies it
//     DestUnchanged (the exact preconditions that, unpatched, reach os.Remove).
//
// The victim lives in root's temp-tree parent; it is registered for cleanup so
// nothing leaks into the shared parent across the test.
func plantTraversalOrphan(t *testing.T, root string, victimBody []byte) (victimPath, traversalDest string) {
	t.Helper()
	// Source gone → Compare's CheckSource returns SourceMissing for ghost-skill.
	removeTestSkillSource(t, root, "ghost-skill")

	// Plant the victim OUTSIDE root, in root's parent (a dir Go created for this
	// test's temp tree). The name embeds root's unique leaf so parallel cases
	// under the same parent cannot collide.
	leaf := filepath.Base(root)
	victimName := "victim-b1-" + leaf
	victimPath = filepath.Join(filepath.Dir(root), victimName)
	if err := os.WriteFile(victimPath, victimBody, 0o644); err != nil {
		t.Fatalf("plant external victim at %s: %v", victimPath, err)
	}
	t.Cleanup(func() { _ = os.Remove(victimPath) })
	// Forward-slashed relative traversal that filepath.Join(root, traversal)
	// resolves to victimPath (Join+Clean collapses the "..").
	traversalDest = filepath.ToSlash(filepath.Join("..", victimName))

	// Rewrite the manifest's ghost-skill entry to point at the traversal with a
	// digest matching the victim, so diskDigest == RenderedDigest → DestUnchanged.
	m, err := renderstate.Read(root)
	if err != nil {
		t.Fatalf("read manifest for B1 traversal plant: %v", err)
	}
	if m == nil {
		t.Fatalf("manifest missing after baseline render (B1 plant)")
	}
	const ghostDest = ".opencode/skills/ghost-skill/SKILL.md"
	found := false
	victimDigest := renderstate.Digest(victimBody)
	for i := range m.Entries {
		if renderstate.NormalizeDestination(m.Entries[i].DestinationPath) == ghostDest {
			m.Entries[i].DestinationPath = renderstate.NormalizeDestination(traversalDest)
			m.Entries[i].RenderedDigest = victimDigest
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("B1 plant: ghost-skill entry %q not found in manifest (entries=%d)", ghostDest, len(m.Entries))
	}
	if err := m.Write(root); err != nil {
		t.Fatalf("write manifest for B1 traversal plant: %v", err)
	}
	return victimPath, traversalDest
}

// TestUpdatePruneOrphans_TraversalDestination_RefusedNotDeleted (B1, live): a
// manifest record whose DestinationPath traverses ("../victim") with a matching
// external victim file and a missing source must be REFUSED — the external
// victim file must STILL EXIST after `update --prune-orphans`, and the finding
// must be reported as refused (not deleted). On unpatched code this test FAILS:
// the victim would be os.Remove'd outside the project.
func TestUpdatePruneOrphans_TraversalDestination_RefusedNotDeleted(t *testing.T) {
	root := renderBaseline(t)
	victimBody := []byte("b1-traversal-victim-DO-NOT-DELETE\n")
	victimPath, _ := plantTraversalOrphan(t, root, victimBody)

	withPruneOrphans(t, true)
	out, err := runUpdateTarget(t, root)
	if err != nil {
		t.Fatalf("prune update (traversal): %v (out=%q)", err, out)
	}
	// The guard refused the traversal (not deleted).
	if !strings.Contains(out, "refuse (destination escapes target") {
		t.Errorf("prune must REFUSE the traversal orphan as an escape; got:\n%s", out)
	}
	if strings.Contains(out, "deleted (byte-identical") {
		t.Errorf("prune must NOT delete a traversal orphan; got:\n%s", out)
	}
	// The external victim file SURVIVES (the core B1 guarantee).
	if _, err := os.Stat(victimPath); err != nil {
		t.Errorf("external victim must STILL EXIST after --prune-orphans (B1): %v", err)
	}
	// Summary: zero deleted, one refused.
	if !strings.Contains(out, "prune-orphans summary:") {
		t.Errorf("prune summary missing; got:\n%s", out)
	}
	if !strings.Contains(out, "0 file(s) deleted, 1 refused for manual removal, 0 skipped") {
		t.Errorf("summary must show 0 deleted / 1 refused for the traversal orphan; got:\n%s", out)
	}
}

// TestUpdatePruneOrphans_TraversalDestination_DryRun_Refused (B1, dry-run):
// --prune-orphans composes with --dry-run. The traversal guard runs BEFORE the
// dry-run/live split, so dry-run must report the traversal as REFUSED (not
// "would delete") and must delete nothing — the external victim survives.
func TestUpdatePruneOrphans_TraversalDestination_DryRun_Refused(t *testing.T) {
	root := renderBaseline(t)
	victimBody := []byte("b1-traversal-victim-dryrun\n")
	victimPath, _ := plantTraversalOrphan(t, root, victimBody)

	withPruneOrphans(t, true)
	withDryRun(t, true)
	out, err := runUpdateTarget(t, root)
	if err != nil {
		t.Fatalf("prune dry-run (traversal): %v (out=%q)", err, out)
	}
	// Dry-run refused the traversal (did not preview it as a would-be deletion).
	if !strings.Contains(out, "refuse (destination escapes target") {
		t.Errorf("dry-run prune must REFUSE the traversal orphan as an escape; got:\n%s", out)
	}
	if strings.Contains(out, "would delete (byte-identical") {
		t.Errorf("dry-run must NOT preview a traversal orphan as 'would delete'; got:\n%s", out)
	}
	// Dry-run deletes nothing: the external victim survives.
	if _, err := os.Stat(victimPath); err != nil {
		t.Errorf("external victim must STILL EXIST after dry-run --prune-orphans (B1): %v", err)
	}
	if !strings.Contains(out, "0 file(s) would be deleted, 1 refused for manual removal, 0 skipped") {
		t.Errorf("dry-run summary must show 0 would-be-deleted / 1 refused for the traversal orphan; got:\n%s", out)
	}
}

// TestOrphanPathEscapesTarget pins the B1 guard's lexical-containment contract
// at the predicate seam (covering both the exercisable "../" escape and the
// defense-in-depth absolute rejection that filepath.Join would otherwise nest).
func TestOrphanPathEscapesTarget(t *testing.T) {
	const target = "/tmp/b1-target/001"
	cases := []struct {
		name string
		dest string
		want bool
	}{
		{"relative one-level traversal", "../victim.txt", true},
		{"relative deep traversal", "../../etc/passwd", true},
		{"absolute path (defense-in-depth)", "/etc/passwd", true},
		{"absolute nested path", "/abs/dir/victim.txt", true},
		{"normal in-repo skill file", ".opencode/skills/foo/SKILL.md", false},
		{"normal nested relative", "foo/bar/baz.txt", false},
		{"single segment", "file.txt", false},
		{"dot-only (target itself)", ".", false},
		{"in-repo with internal dot segment", ".opencode/./skills/x", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := orphanPathEscapesTarget(target, tc.dest); got != tc.want {
				t.Errorf("orphanPathEscapesTarget(%q, %q) = %v, want %v", target, tc.dest, got, tc.want)
			}
		})
	}
}
