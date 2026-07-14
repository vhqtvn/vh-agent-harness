package renderstate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeChecker is a SourceChecker backed by an explicit set of (pack,src) keys
// that still exist. Everything else is missing.
type fakeChecker struct {
	exist map[string]bool
}

func (f fakeChecker) SourceExists(rec Record) bool {
	return f.exist[rec.OverlayPack+"\x00"+rec.SourceRelativePath]
}

func key(pack, src string) string { return pack + "\x00" + src }

func rec(dest, pack, src, digest string) Record {
	return Record{
		DestinationPath:    dest,
		ProducerKind:       ProducerOverlaySkill,
		OverlayPack:        pack,
		SourceRelativePath: src,
		RenderedDigest:     digest,
	}
}

// writeDest writes a file under root at the repo-relative forward-slashed path rel.
func writeDest(t *testing.T, root, rel string, content []byte) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	if err := os.WriteFile(p, content, 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// --- manifest read/write + schema --------------------------------------------

// TestWriteRead_RoundTrip confirms Write + Read reproduces the manifest and that
// entries are sorted deterministically.
func TestWriteRead_RoundTrip(t *testing.T) {
	root := t.TempDir()
	m := New("render-xyz")
	// Insert out of order to confirm Write sorts.
	m.Entries = []Record{
		rec(".opencode/skills/zzz/SKILL.md", "p2", "skills/zzz/SKILL.md", Digest([]byte("z"))),
		rec(".opencode/skills/aaa/SKILL.md", "p1", "skills/aaa/SKILL.md", Digest([]byte("a"))),
	}
	if err := m.Write(root); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := Read(root)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.ManifestVersion != ManifestVersion {
		t.Errorf("version = %q want %q", got.ManifestVersion, ManifestVersion)
	}
	if got.SuccessfulRenderID != "render-xyz" {
		t.Errorf("render id = %q want render-xyz", got.SuccessfulRenderID)
	}
	if len(got.Entries) != 2 {
		t.Fatalf("entries = %d want 2", len(got.Entries))
	}
	if got.Entries[0].DestinationPath != ".opencode/skills/aaa/SKILL.md" {
		t.Errorf("entries not sorted: first = %s", got.Entries[0].DestinationPath)
	}
}

// TestRead_MissingManifestIsBootstrap confirms a missing manifest returns
// (nil,nil) so the caller treats it as a forward-looking bootstrap.
func TestRead_MissingManifestIsBootstrap(t *testing.T) {
	got, err := Read(t.TempDir())
	if err != nil {
		t.Fatalf("Read empty dir: %v", err)
	}
	if got != nil {
		t.Errorf("missing manifest must return nil, got %+v", got)
	}
}

// TestRead_RejectsBadVersion confirms an unknown manifest_version is rejected
// rather than trusted.
func TestRead_RejectsBadVersion(t *testing.T) {
	root := t.TempDir()
	bad := map[string]any{"manifest_version": "99", "entries": []any{}}
	data, _ := json.Marshal(bad)
	if err := os.MkdirAll(filepath.Join(root, DirName), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(FilePath(root), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(root); err == nil {
		t.Fatal("Read must reject an unsupported manifest_version")
	}
}

// TestValidate_RejectsBadRecords covers the per-field validation.
func TestValidate_RejectsBadRecords(t *testing.T) {
	cases := []struct {
		name string
		m    *Manifest
	}{
		{"empty version", &Manifest{ManifestVersion: ""}},
		{"unknown version", &Manifest{ManifestVersion: "2"}},
		{"bad producer kind", &Manifest{ManifestVersion: ManifestVersion, Entries: []Record{{
			DestinationPath: ".opencode/skills/x/SKILL.md", ProducerKind: ProducerKind("bogus"),
			OverlayPack: "p", SourceRelativePath: "skills/x/SKILL.md", RenderedDigest: Digest(nil),
		}}}},
		{"empty dest", &Manifest{ManifestVersion: ManifestVersion, Entries: []Record{{
			DestinationPath: "", ProducerKind: ProducerOverlaySkill,
			OverlayPack: "p", SourceRelativePath: "skills/x/SKILL.md", RenderedDigest: Digest(nil),
		}}}},
		{"bad digest", &Manifest{ManifestVersion: ManifestVersion, Entries: []Record{{
			DestinationPath: ".opencode/skills/x/SKILL.md", ProducerKind: ProducerOverlaySkill,
			OverlayPack: "p", SourceRelativePath: "skills/x/SKILL.md", RenderedDigest: "md5:deadbeef",
		}}}},
		{"duplicate dest", &Manifest{ManifestVersion: ManifestVersion, Entries: []Record{
			rec(".opencode/skills/x/SKILL.md", "p", "skills/x/SKILL.md", Digest(nil)),
			rec(".opencode/skills/x/SKILL.md", "p", "skills/x/SKILL.md", Digest(nil)),
		}}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			if err := c.m.Validate(); err == nil {
				t.Fatalf("Validate must reject %s", c.name)
			}
		})
	}
}

// TestWrite_Atomic_NoPartialOnFailure confirms that when the manifest directory
// is unwritable (rename/write cannot complete), the prior manifest on disk is
// untouched — never a half-written mix. This is the locked atomicity rule.
func TestWrite_Atomic_NoPartialOnFailure(t *testing.T) {
	root := t.TempDir()
	m := New("r1")
	m.Entries = []Record{rec(".opencode/skills/a/SKILL.md", "p", "skills/a/SKILL.md", Digest([]byte("a")))}
	if err := m.Write(root); err != nil {
		t.Fatalf("seed Write: %v", err)
	}
	before, _ := os.ReadFile(FilePath(root))

	// Make the manifest dir read-only so the new temp file cannot be created.
	dir := filepath.Join(root, DirName)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o755)

	m2 := New("r2")
	m2.Entries = []Record{rec(".opencode/skills/b/SKILL.md", "p", "skills/b/SKILL.md", Digest([]byte("b")))}
	if err := m2.Write(root); err == nil {
		t.Fatal("Write must fail when the manifest dir is unwritable")
	}
	after, _ := os.ReadFile(FilePath(root))
	if string(before) != string(after) {
		t.Errorf("prior manifest must be untouched after a failed write")
	}
}

// TestNormalizeDestination confirms normalization strips ./ and backslashes.
func TestNormalizeDestination(t *testing.T) {
	cases := map[string]string{
		".opencode/skills/x/SKILL.md":    ".opencode/skills/x/SKILL.md",
		"./.opencode/skills/x/SKILL.md":  ".opencode/skills/x/SKILL.md",
		".opencode\\skills\\x\\SKILL.md": ".opencode/skills/x/SKILL.md",
	}
	for in, want := range cases {
		if got := NormalizeDestination(in); got != want {
			t.Errorf("NormalizeDestination(%q) = %q want %q", in, got, want)
		}
	}
}

// --- diff: orphan detection --------------------------------------------------

// TestCompare_SourceMissing_ReportsOrphan is the core positive case: a prior
// record whose source is gone and whose destination is still on disk is reported
// as a definite preserved orphan.
func TestCompare_SourceMissing_ReportsOrphan(t *testing.T) {
	root := t.TempDir()
	content := []byte("# skill body\n")
	writeDest(t, root, ".opencode/skills/tdd/SKILL.md", content)
	prior := &Manifest{ManifestVersion: ManifestVersion, Entries: []Record{
		rec(".opencode/skills/tdd/SKILL.md", "p", "skills/tdd/SKILL.md", Digest(content)),
	}}
	// source "p/skills/tdd/SKILL.md" is NOT in exist → missing.
	chk := fakeChecker{exist: map[string]bool{}}
	findings := Compare(prior, nil, chk, root)
	if len(findings) != 1 {
		t.Fatalf("findings = %d want 1: %+v", len(findings), findings)
	}
	f := findings[0]
	if f.Reason != ReasonSourceMissing {
		t.Errorf("reason = %q want source_missing", f.Reason)
	}
	if f.Action != ActionReportedPreserved {
		t.Errorf("action = %q want reported_preserved", f.Action)
	}
	if f.DestinationState != DestUnchanged {
		t.Errorf("state = %q want unchanged (digest matches)", f.DestinationState)
	}
	if f.SkillDir != ".opencode/skills/tdd" {
		t.Errorf("skill dir = %q want .opencode/skills/tdd", f.SkillDir)
	}
}

// TestCompare_SourcePresent_NotOrphan confirms a record whose source STILL EXISTS
// (pack merely deselected) is NOT flagged. This is the provenance rule that
// separates source-removal from deselection.
func TestCompare_SourcePresent_NotOrphan(t *testing.T) {
	root := t.TempDir()
	writeDest(t, root, ".opencode/skills/tdd/SKILL.md", []byte("x"))
	prior := &Manifest{ManifestVersion: ManifestVersion, Entries: []Record{
		rec(".opencode/skills/tdd/SKILL.md", "p", "skills/tdd/SKILL.md", Digest([]byte("x"))),
	}}
	chk := fakeChecker{exist: map[string]bool{key("p", "skills/tdd/SKILL.md"): true}}
	findings := Compare(prior, nil, chk, root)
	if len(findings) != 0 {
		t.Fatalf("source-present record must NOT be flagged; got %d findings", len(findings))
	}
}

// TestCompare_FreshlyRendered_NotOrphan confirms a record reproduced by the
// current render is never an orphan even if the checker would say missing.
func TestCompare_FreshlyRendered_NotOrphan(t *testing.T) {
	root := t.TempDir()
	writeDest(t, root, ".opencode/skills/tdd/SKILL.md", []byte("x"))
	prior := &Manifest{ManifestVersion: ManifestVersion, Entries: []Record{
		rec(".opencode/skills/tdd/SKILL.md", "p", "skills/tdd/SKILL.md", Digest([]byte("x"))),
	}}
	current := []Record{rec(".opencode/skills/tdd/SKILL.md", "p", "skills/tdd/SKILL.md", Digest([]byte("x")))}
	// checker says missing, but current render reproduced it → not orphan.
	chk := fakeChecker{exist: map[string]bool{}}
	findings := Compare(prior, current, chk, root)
	if len(findings) != 0 {
		t.Fatalf("freshly rendered record must NOT be flagged; got %d findings", len(findings))
	}
}

// TestCompare_NonNormalizedPriorMatchesCurrent confirms that a prior manifest
// record carrying a non-canonical destination path (a "./" prefix here — exactly
// what a hand-authored or corrupted manifest could carry) is still matched
// against the current render via NormalizeDestination, so it is NOT mis-flagged
// as an orphan. This locks the F2/F3 hardening: orphan detection must not slip a
// false positive (or miss a true orphan) just because the prior path was not in
// canonical form.
func TestCompare_NonNormalizedPriorMatchesCurrent(t *testing.T) {
	root := t.TempDir()
	writeDest(t, root, ".opencode/skills/tdd/SKILL.md", []byte("x"))
	// Prior destination carries a "./" prefix (non-canonical).
	prior := &Manifest{ManifestVersion: ManifestVersion, Entries: []Record{
		rec("./.opencode/skills/tdd/SKILL.md", "p", "skills/tdd/SKILL.md", Digest([]byte("x"))),
	}}
	// Current render carries the canonical form.
	current := []Record{rec(".opencode/skills/tdd/SKILL.md", "p", "skills/tdd/SKILL.md", Digest([]byte("x")))}
	chk := fakeChecker{exist: map[string]bool{}}
	findings := Compare(prior, current, chk, root)
	if len(findings) != 0 {
		t.Fatalf("non-normalized prior that matches the current render must NOT be flagged; got %d findings: %+v", len(findings), findings)
	}
}

// TestCompare_NonNormalizedPriorStillDetectedOrphan confirms normalization does
// not weaken detection: a non-canonical prior record whose source is missing and
// whose destination is present is STILL reported (just matched on its canonical
// form for the disk read).
func TestCompare_NonNormalizedPriorStillDetectedOrphan(t *testing.T) {
	root := t.TempDir()
	content := []byte("# orphan\n")
	writeDest(t, root, ".opencode/skills/ghost/SKILL.md", content)
	// Prior destination has a redundant "./" prefix.
	prior := &Manifest{ManifestVersion: ManifestVersion, Entries: []Record{
		rec("./.opencode/skills/ghost/SKILL.md", "p", "skills/ghost/SKILL.md", Digest(content)),
	}}
	chk := fakeChecker{exist: map[string]bool{}}
	findings := Compare(prior, nil, chk, root)
	if len(findings) != 1 {
		t.Fatalf("non-normalized prior orphan must still be detected; got %d findings: %+v", len(findings), findings)
	}
	if findings[0].SkillDir != ".opencode/skills/ghost" {
		t.Errorf("skill dir = %q want .opencode/skills/ghost", findings[0].SkillDir)
	}
}

// TestCompare_DestinationGone_RetiresSilently confirms a source-missing record
// whose destination is also gone is NOT reported (nothing to preserve).
func TestCompare_DestinationGone_RetiresSilently(t *testing.T) {
	root := t.TempDir()
	// destination intentionally NOT written.
	prior := &Manifest{ManifestVersion: ManifestVersion, Entries: []Record{
		rec(".opencode/skills/tdd/SKILL.md", "p", "skills/tdd/SKILL.md", Digest([]byte("x"))),
	}}
	chk := fakeChecker{exist: map[string]bool{}}
	findings := Compare(prior, nil, chk, root)
	if len(findings) != 0 {
		t.Fatalf("destination-gone record must retire silently; got %d findings", len(findings))
	}
}

// TestCompare_ModifiedDestination labels a hand-edited orphan as modified.
func TestCompare_ModifiedDestination(t *testing.T) {
	root := t.TempDir()
	writeDest(t, root, ".opencode/skills/tdd/SKILL.md", []byte("EDITED BY OPERATOR\n"))
	prior := &Manifest{ManifestVersion: ManifestVersion, Entries: []Record{
		rec(".opencode/skills/tdd/SKILL.md", "p", "skills/tdd/SKILL.md", Digest([]byte("original\n"))),
	}}
	chk := fakeChecker{exist: map[string]bool{}}
	findings := Compare(prior, nil, chk, root)
	if len(findings) != 1 || findings[0].DestinationState != DestModified {
		t.Fatalf("want 1 modified finding, got %+v", findings)
	}
}

// TestCompare_ProjectAddedDir_NeverFlagged confirms a skill dir the operator
// created by hand (never recorded in any manifest) is invisible to detection:
// there is no prior record for it, so it cannot be flagged. This is the
// provenance rule against path-prefix / current-classification detection.
func TestCompare_ProjectAddedDir_NeverFlagged(t *testing.T) {
	root := t.TempDir()
	// Operator-added dir on disk, but no prior manifest entry for it.
	writeDest(t, root, ".opencode/skills/my-own/SKILL.md", []byte("mine\n"))
	prior := &Manifest{ManifestVersion: ManifestVersion, Entries: nil}
	chk := fakeChecker{exist: map[string]bool{}}
	findings := Compare(prior, nil, chk, root)
	if len(findings) != 0 {
		t.Fatalf("unrecorded project-added dir must never be flagged; got %d findings", len(findings))
	}
}

// TestCompare_NilPriorIsBootstrap confirms no prior manifest means no findings
// (forward-looking bootstrap — never retroactively adopt existing dirs).
func TestCompare_NilPriorIsBootstrap(t *testing.T) {
	root := t.TempDir()
	writeDest(t, root, ".opencode/skills/old/SKILL.md", []byte("pre-existing\n"))
	chk := fakeChecker{exist: map[string]bool{}}
	if findings := Compare(nil, nil, chk, root); len(findings) != 0 {
		t.Fatalf("nil prior (bootstrap) must surface no findings; got %d", len(findings))
	}
}

// TestCompare_PackGone_TreatedAsSourceMissing confirms a record whose whole pack
// can no longer be opened (SourceExists false) is treated identically to a
// missing source file — both are source_missing.
func TestCompare_PackGone_TreatedAsSourceMissing(t *testing.T) {
	root := t.TempDir()
	writeDest(t, root, ".opencode/skills/tdd/SKILL.md", []byte("x"))
	prior := &Manifest{ManifestVersion: ManifestVersion, Entries: []Record{
		rec(".opencode/skills/tdd/SKILL.md", "gone-pack", "skills/tdd/SKILL.md", Digest([]byte("x"))),
	}}
	// checker has no entry for gone-pack → SourceExists false.
	chk := fakeChecker{exist: map[string]bool{key("other", "skills/x/SKILL.md"): true}}
	findings := Compare(prior, nil, chk, root)
	if len(findings) != 1 || findings[0].Reason != ReasonSourceMissing {
		t.Fatalf("pack-gone must be source_missing; got %+v", findings)
	}
	if findings[0].OverlayPack != "gone-pack" {
		t.Errorf("finding must carry the producing pack name; got %q", findings[0].OverlayPack)
	}
}

// --- next manifest: stale retention ------------------------------------------

// TestNextManifest_RetainsStaleOrphan confirms a stale record (source missing,
// destination present) is carried into the next manifest so the orphan keeps
// reporting across runs.
func TestNextManifest_RetainsStaleOrphan(t *testing.T) {
	root := t.TempDir()
	content := []byte("keep reporting\n")
	writeDest(t, root, ".opencode/skills/tdd/SKILL.md", content)
	prior := &Manifest{ManifestVersion: ManifestVersion, Entries: []Record{
		rec(".opencode/skills/tdd/SKILL.md", "p", "skills/tdd/SKILL.md", Digest(content)),
	}}
	chk := fakeChecker{exist: map[string]bool{}} // source missing
	next := NextManifest(prior, nil, chk, root, "render-2")
	if len(next.Entries) != 1 {
		t.Fatalf("stale orphan must be retained; got %d entries", len(next.Entries))
	}
	if next.Entries[0].DestinationPath != ".opencode/skills/tdd/SKILL.md" {
		t.Errorf("retained entry dest = %q", next.Entries[0].DestinationPath)
	}
	if next.SuccessfulRenderID != "render-2" {
		t.Errorf("render id = %q want render-2", next.SuccessfulRenderID)
	}
}

// TestNextManifest_SourceReturned_DropsStale confirms a stale record whose source
// returned (current render reproduces it) is replaced by the fresh record.
func TestNextManifest_SourceReturned_DropsStale(t *testing.T) {
	root := t.TempDir()
	writeDest(t, root, ".opencode/skills/tdd/SKILL.md", []byte("fresh\n"))
	prior := &Manifest{ManifestVersion: ManifestVersion, Entries: []Record{
		rec(".opencode/skills/tdd/SKILL.md", "p", "skills/tdd/SKILL.md", Digest([]byte("old\n"))),
	}}
	current := []Record{rec(".opencode/skills/tdd/SKILL.md", "p", "skills/tdd/SKILL.md", Digest([]byte("fresh\n")))}
	next := NextManifest(prior, current, nil, root, "render-2")
	if len(next.Entries) != 1 {
		t.Fatalf("want 1 entry (fresh wins); got %d", len(next.Entries))
	}
	if next.Entries[0].RenderedDigest != Digest([]byte("fresh\n")) {
		t.Errorf("fresh record must win over stale")
	}
}

// TestNextManifest_DestinationGone_DropsRecord confirms a stale record whose
// destination disappeared is retired (not carried forward).
func TestNextManifest_DestinationGone_DropsRecord(t *testing.T) {
	root := t.TempDir()
	// destination intentionally absent.
	prior := &Manifest{ManifestVersion: ManifestVersion, Entries: []Record{
		rec(".opencode/skills/tdd/SKILL.md", "p", "skills/tdd/SKILL.md", Digest([]byte("x"))),
	}}
	chk := fakeChecker{exist: map[string]bool{}} // source missing
	next := NextManifest(prior, nil, chk, root, "render-2")
	if len(next.Entries) != 0 {
		t.Fatalf("destination-gone record must be retired; got %d entries", len(next.Entries))
	}
}

// TestNextManifest_NoPrior_Bootstrap confirms a no-prior-manifest bootstrap only
// persists the current render (never retroactively adopts on-disk dirs).
func TestNextManifest_NoPrior_Bootstrap(t *testing.T) {
	root := t.TempDir()
	current := []Record{
		rec(".opencode/skills/tdd/SKILL.md", "p", "skills/tdd/SKILL.md", Digest([]byte("a"))),
	}
	next := NextManifest(nil, current, nil, root, "render-1")
	if len(next.Entries) != 1 || next.Entries[0].DestinationPath != ".opencode/skills/tdd/SKILL.md" {
		t.Fatalf("bootstrap must persist current render only; got %+v", next.Entries)
	}
}

// TestNextManifest_RepeatedReportingAcrossRuns simulates two consecutive runs
// after a source removal and confirms the orphan keeps surfacing (stale record
// survives the first persist and is detected again on the second run).
func TestNextManifest_RepeatedReportingAcrossRuns(t *testing.T) {
	root := t.TempDir()
	content := []byte("body\n")
	writeDest(t, root, ".opencode/skills/tdd/SKILL.md", content)
	prior := &Manifest{ManifestVersion: ManifestVersion, Entries: []Record{
		rec(".opencode/skills/tdd/SKILL.md", "p", "skills/tdd/SKILL.md", Digest(content)),
	}}
	chk := fakeChecker{exist: map[string]bool{}} // source missing

	// Run 1: detect, then persist next manifest.
	f1 := Compare(prior, nil, chk, root)
	if len(f1) != 1 {
		t.Fatalf("run 1: want 1 finding, got %d", len(f1))
	}
	next := NextManifest(prior, nil, chk, root, "render-2")

	// Run 2: compare against the persisted next manifest.
	f2 := Compare(next, nil, chk, root)
	if len(f2) != 1 {
		t.Fatalf("run 2: want the orphan to keep reporting (1 finding), got %d", len(f2))
	}
}

// TestWrite_DeterministicBytes confirms two Write calls produce byte-identical
// output, so a no-op update does not churn the manifest in git.
func TestWrite_DeterministicBytes(t *testing.T) {
	r1 := t.TempDir()
	r2 := t.TempDir()
	m := New("render-1")
	m.Entries = []Record{
		rec(".opencode/skills/b/SKILL.md", "p2", "skills/b/SKILL.md", Digest([]byte("b"))),
		rec(".opencode/skills/a/SKILL.md", "p1", "skills/a/SKILL.md", Digest([]byte("a"))),
	}
	if err := m.Write(r1); err != nil {
		t.Fatal(err)
	}
	if err := m.Write(r2); err != nil {
		t.Fatal(err)
	}
	b1, _ := os.ReadFile(FilePath(r1))
	b2, _ := os.ReadFile(FilePath(r2))
	if string(b1) != string(b2) {
		t.Errorf("Write must be deterministic; outputs differ")
	}
	if !strings.Contains(string(b1), "manifest_version") {
		t.Errorf("output missing manifest_version key")
	}
}
