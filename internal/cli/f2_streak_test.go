package cli

// f2_streak_test.go — tests for the R1-derived operator synthesis streak
// scanner (Slice 8).
//
// Covers the F2 memo L268-286 contract:
//   - Ordering by canonical chronology (cycle_id), NEVER mtime
//   - Bad-digest sidecar excluded
//   - Corrupt sidecar excluded
//   - No new relationship appears
//   - Repeated scan identical (deterministic)
//   - R1 conclusions rendered verbatim
//   - Cycle with no R1 entry included (bounded absence, not removed)
//   - Empty directory → empty view

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// streakSidecarForCycle builds a VALID sidecar (all 3 F1 families present,
// P-a coverage intact) with a distinct cycle ID and a custom R1 conclusion.
// It clones the canonical fixture and overrides cycle + R1 fields so the
// envelope remains F1-valid (the validator requires all families when
// applicability=required + P-a coverage on each R1 conclusion).
func streakSidecarForCycle(t *testing.T, dir, cycleID, filename, conclusionID, propertyID string) string {
	t.Helper()
	env := cloneFixtureForStreak()
	env.SynthesisCycleID = cycleID
	// Override the single R1 conclusion.
	env.Entries[0].R1.Conclusions[0].ConclusionID = conclusionID
	env.Entries[0].R1.Conclusions[0].PropertyID = propertyID
	env.Entries[0].R1.Conclusions[0].Sources = []F1R1Source{{Locator: "src-" + cycleID}}
	// Point the P-a coverage probe at the new conclusion ID.
	env.Entries[2].PA.Probes[0].TargetRef = conclusionID
	// Point R3 options' support refs at the new conclusion ID.
	env.Entries[1].R3.Options[0].SupportRefs = []string{conclusionID}
	env.Entries[1].R3.Options[1].SupportRefs = []string{conclusionID}

	d, err := env.ComputeDigest()
	if err != nil {
		t.Fatalf("digest for %s: %v", cycleID, err)
	}
	env.SemanticDigest = d

	emit := &ValidatedF1Emit{
		CanonicalEnvelope:     env,
		SemanticDigest:        d,
		ValidationDisposition: F1ValidationComplete,
	}
	ingest, errs := IngestF1EmitForF2(emit)
	if len(errs) != 0 {
		t.Fatalf("ingest %s: %v", cycleID, errs)
	}

	sidecar := buildF2CanonicalSidecar(ingest, dir, fixedTime)
	bytes, err := SerializeF2CanonicalSidecar(sidecar)
	if err != nil {
		t.Fatalf("serialize %s: %v", cycleID, err)
	}

	// Write under the EXPLICIT filename (not the canonical path) so tests can
	// simulate wrong filename order (filename != cycle_id order).
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, bytes, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// cloneFixtureForStreak returns a deep-enough copy of canonicalF1Fixture() for
// streak tests to mutate cycle ID + R1 fields without disturbing the shared
// golden fixture.
func cloneFixtureForStreak() *F1SynthesisEnvelope {
	src := canonicalF1Fixture()
	// Marshal/unmarshal for a genuine deep copy.
	b, err := json.Marshal(src)
	if err != nil {
		panic("clone fixture marshal: " + err.Error())
	}
	var dst F1SynthesisEnvelope
	if err := json.Unmarshal(b, &dst); err != nil {
		panic("clone fixture unmarshal: " + err.Error())
	}
	return &dst
}

// writeStreakSidecarBytes writes arbitrary bytes to a .canonical.json path.
func writeStreakSidecarBytes(t *testing.T, dir, filename string, content []byte) string {
	t.Helper()
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestScanF2R1Streak_EmptyDirReturnsEmptyView(t *testing.T) {
	dir := t.TempDir()
	view, diags, err := ScanF2R1Streak(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(view.Cycles) != 0 {
		t.Fatalf("expected 0 cycles, got %d", len(view.Cycles))
	}
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
	if view.ExcludedCount != 0 {
		t.Fatalf("expected excluded=0, got %d", view.ExcludedCount)
	}
	if view.RendererVersion != F2StreakRendererVersion {
		t.Fatalf("renderer version = %q, want %q", view.RendererVersion, F2StreakRendererVersion)
	}
}

func TestScanF2R1Streak_FilenameOrderIgnored_UsesCycleIDChronology(t *testing.T) {
	dir := t.TempDir()
	// Write in REVERSE filename order: file "zzz" carries cycle "001", file
	// "aaa" carries cycle "009". Output MUST follow cycle_id order (001 first).
	streakSidecarForCycle(t, dir, "cycle-001", "zzz-last.canonical.json", "C1", "P1")
	streakSidecarForCycle(t, dir, "cycle-009", "aaa-first.canonical.json", "C9", "P9")

	view, _, err := ScanF2R1Streak(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(view.Cycles) != 2 {
		t.Fatalf("expected 2 cycles, got %d", len(view.Cycles))
	}
	if view.Cycles[0].SynthesisCycleID != "cycle-001" {
		t.Fatalf("first cycle = %q, want cycle-001 (chronological, not filename)", view.Cycles[0].SynthesisCycleID)
	}
	if view.Cycles[1].SynthesisCycleID != "cycle-009" {
		t.Fatalf("second cycle = %q, want cycle-009", view.Cycles[1].SynthesisCycleID)
	}
}

func TestScanF2R1Streak_BadDigestExcluded(t *testing.T) {
	dir := t.TempDir()
	// Valid cycle.
	streakSidecarForCycle(t, dir, "cycle-valid", "valid.canonical.json", "CV", "PV")

	// Sidecar with a tampered digest: take a valid sidecar, mutate the
	// carried SourceSemanticDigest so recompute != carried.
	env := cloneFixtureForStreak()
	env.SynthesisCycleID = "cycle-baddigest"
	env.Entries[0].R1.Conclusions[0].ConclusionID = "CB"
	env.Entries[0].R1.Conclusions[0].PropertyID = "PB"
	env.Entries[2].PA.Probes[0].TargetRef = "CB"
	env.Entries[1].R3.Options[0].SupportRefs = []string{"CB"}
	env.Entries[1].R3.Options[1].SupportRefs = []string{"CB"}
	d, _ := env.ComputeDigest()
	env.SemanticDigest = d
	emit := &ValidatedF1Emit{CanonicalEnvelope: env, SemanticDigest: d, ValidationDisposition: F1ValidationComplete}
	ingest, _ := IngestF1EmitForF2(emit)
	sidecar := buildF2CanonicalSidecar(ingest, dir, fixedTime)
	// Tamper the carried digest (the canonical content is unchanged, so the
	// recompute will mismatch the carried value).
	sidecar.F2ViewMetadata.SourceSemanticDigest = "0" + strings.TrimPrefix(d, "0")
	if strings.HasPrefix(d, "0") {
		sidecar.F2ViewMetadata.SourceSemanticDigest = "1" + d[1:]
	}
	bytes, _ := SerializeF2CanonicalSidecar(sidecar)
	writeStreakSidecarBytes(t, dir, "bad.canonical.json", bytes)

	view, diags, err := ScanF2R1Streak(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(view.Cycles) != 1 {
		t.Fatalf("expected 1 valid cycle (bad-digest excluded), got %d", len(view.Cycles))
	}
	if view.Cycles[0].SynthesisCycleID != "cycle-valid" {
		t.Fatalf("remaining cycle = %q, want cycle-valid", view.Cycles[0].SynthesisCycleID)
	}
	if view.ExcludedCount != 1 {
		t.Fatalf("expected excluded=1, got %d", view.ExcludedCount)
	}
	found := false
	for _, d := range diags {
		if d.Kind == F2StreakExcludedBadDigest && d.CycleID == "cycle-baddigest" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected excluded_bad_digest diagnostic for cycle-baddigest, got: %+v", diags)
	}
}

func TestScanF2R1Streak_CorruptJSONExcluded(t *testing.T) {
	dir := t.TempDir()
	streakSidecarForCycle(t, dir, "cycle-valid", "valid.canonical.json", "CV", "PV")
	writeStreakSidecarBytes(t, dir, "corrupt.canonical.json", []byte("{not valid json"))

	view, diags, err := ScanF2R1Streak(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(view.Cycles) != 1 {
		t.Fatalf("expected 1 valid cycle, got %d", len(view.Cycles))
	}
	if view.ExcludedCount != 1 {
		t.Fatalf("expected excluded=1, got %d", view.ExcludedCount)
	}
	found := false
	for _, d := range diags {
		if d.Kind == F2StreakExcludedCorruptJSON && strings.Contains(d.Path, "corrupt.canonical.json") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected excluded_corrupt_json diagnostic, got: %+v", diags)
	}
}

func TestScanF2R1Streak_MissingEnvelopeExcluded(t *testing.T) {
	dir := t.TempDir()
	streakSidecarForCycle(t, dir, "cycle-valid", "valid.canonical.json", "CV", "PV")

	// Sidecar with nil CanonicalEnvelope.
	badSidecar := &F2CanonicalSidecar{
		Kind:              F2CanonicalSidecarKind,
		CanonicalEnvelope: nil,
		F2ViewMetadata: F2ArtifactViewMeta{
			SynthesisCycleID:               "cycle-noenv",
			SourceSemanticDigest:           "any",
			CanonicalRepresentationVersion: F2CanonicalRepresentationVersion,
			SchemaVersion:                  "1",
			ProjectionVersion:              F2ProjectionVersion,
			RendererVersion:                F2RendererVersion,
			WriteTimestamp:                 fixedTime.Format(time.RFC3339),
		},
	}
	bytes, _ := SerializeF2CanonicalSidecar(badSidecar)
	writeStreakSidecarBytes(t, dir, "noenv.canonical.json", bytes)

	view, diags, err := ScanF2R1Streak(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(view.Cycles) != 1 {
		t.Fatalf("expected 1 valid cycle, got %d", len(view.Cycles))
	}
	found := false
	for _, d := range diags {
		if d.Kind == F2StreakExcludedMissingEnvelope {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected excluded_missing_envelope diagnostic, got: %+v", diags)
	}
}

func TestScanF2R1Streak_NoR1EntryIncludedWithBoundedAbsence(t *testing.T) {
	dir := t.TempDir()
	// A cycle with NO r1_cross_lane_join entry. The streak scanner reads
	// sidecars from disk and verifies digest, not F1 validation — so we
	// construct a sidecar directly with a no-R1-entry envelope (digest
	// recomputed to match). The cycle must be INCLUDED with a warn diagnostic,
	// NOT removed from the streak.
	env := cloneFixtureForStreak()
	env.SynthesisCycleID = "cycle-no-r1"
	// Remove the R1 entry (index 0), keep R3 + P-a. Also re-point the R3
	// option SupportRefs and the P-a probe TargetRef to something still valid
	// (they referenced R1C1 which is now gone; use a synthetic ref — the
	// scanner does not resolve refs, it only checks digest).
	env.Entries = env.Entries[1:] // drop R1 entry
	d, err := env.ComputeDigest()
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	env.SemanticDigest = d

	entryIDs := []string{env.Entries[0].EntryID, env.Entries[1].EntryID}
	sidecar := &F2CanonicalSidecar{
		Kind:              F2CanonicalSidecarKind,
		CanonicalEnvelope: env,
		F2ViewMetadata: F2ArtifactViewMeta{
			SynthesisCycleID:               env.SynthesisCycleID,
			EntryIDs:                       entryIDs,
			SourceSemanticDigest:           d,
			CanonicalRepresentationVersion: F2CanonicalRepresentationVersion,
			SchemaVersion:                  env.SchemaVersion,
			ProjectionVersion:              F2ProjectionVersion,
			RendererVersion:                F2RendererVersion,
			ReciprocalLocator:              filepath.Join(dir, "no-r1.md"),
			WriteTimestamp:                 fixedTime.Format(time.RFC3339),
		},
	}
	bytes, _ := SerializeF2CanonicalSidecar(sidecar)
	writeStreakSidecarBytes(t, dir, "no-r1.canonical.json", bytes)

	view, diags, err := ScanF2R1Streak(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(view.Cycles) != 1 {
		t.Fatalf("expected 1 cycle (no-R1 INCLUDED, not removed), got %d", len(view.Cycles))
	}
	if view.Cycles[0].HasR1Entry {
		t.Fatalf("HasR1Entry should be false")
	}
	if view.Cycles[0].R1EntryID != "" {
		t.Fatalf("R1EntryID should be empty, got %q", view.Cycles[0].R1EntryID)
	}
	if view.ExcludedCount != 0 {
		t.Fatalf("cycle should not be excluded, got excluded=%d", view.ExcludedCount)
	}
	// Warn diagnostic present.
	found := false
	for _, d := range diags {
		if d.Kind == F2StreakWarnNoR1Entry && d.CycleID == "cycle-no-r1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected warn_no_r1_entry diagnostic, got: %+v", diags)
	}
}

func TestScanF2R1Streak_R1ConclusionsVerbatim(t *testing.T) {
	dir := t.TempDir()
	// A cycle with a rich R1 conclusion carrying all field types. The scanner
	// reads sidecars from disk and verifies digest, not F1 validation — so we
	// construct a sidecar directly with the rich R1 conclusion (digest
	// recomputed to match).
	env := cloneFixtureForStreak()
	env.SynthesisCycleID = "cycle-rich"
	// Replace the single R1 conclusion with a rich one.
	env.Entries[0].R1.Conclusions = []F1R1Conclusion{{
		ConclusionID:    "RC1",
		PropertyID:      "RP1",
		JoinDisposition: F1R1JoinMerge,
		Lanes:           []F1R1LaneContrib{{LaneID: "lane-a", ActID: "act-1"}, {LaneID: "lane-b"}},
		Sources:         []F1R1Source{{Locator: "src-a", AncestryRoots: []string{"root-1"}}},
		Agreements:      []string{"agree-1"},
		Contradictions:  []F1R1Contradiction{{ContradictionID: "contra-1", LaneA: "lane-a", LaneB: "lane-b", Detail: "disagree on X"}},
		Gaps:            []F1R1Gap{{GapID: "gap-1", Aspect: "coverage", Detail: "not fully covered"}},
		Hazards: []F1R1HazardLink{{
			HazardRef: "hazard-1", SymptomRefs: []string{"sym-1"},
			SourceLocators: []string{"src-a"}, AncestryRoots: []string{"root-1"},
			ContradictionRef: "contra-1", GapRef: "gap-1",
			ConsumingR3OptionIDs: []string{"opt-x"}, ConsumingPAProbeIDs: []string{"PA-X"},
		}},
	}}
	// Point the P-a coverage + R3 support refs at the new conclusion ID.
	env.Entries[2].PA.Probes[0].TargetRef = "RC1"
	env.Entries[1].R3.Options[0].SupportRefs = []string{"RC1"}
	env.Entries[1].R3.Options[1].SupportRefs = []string{"RC1"}
	d, err := env.ComputeDigest()
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	env.SemanticDigest = d

	sidecar := &F2CanonicalSidecar{
		Kind:              F2CanonicalSidecarKind,
		CanonicalEnvelope: env,
		F2ViewMetadata: F2ArtifactViewMeta{
			SynthesisCycleID:               env.SynthesisCycleID,
			EntryIDs:                       []string{"entry-r1", "entry-r3", "entry-pa"},
			SourceSemanticDigest:           d,
			CanonicalRepresentationVersion: F2CanonicalRepresentationVersion,
			SchemaVersion:                  env.SchemaVersion,
			ProjectionVersion:              F2ProjectionVersion,
			RendererVersion:                F2RendererVersion,
			ReciprocalLocator:              filepath.Join(dir, "rich.md"),
			WriteTimestamp:                 fixedTime.Format(time.RFC3339),
		},
	}
	bytes, _ := SerializeF2CanonicalSidecar(sidecar)
	writeStreakSidecarBytes(t, dir, "rich.canonical.json", bytes)

	view, _, err := ScanF2R1Streak(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(view.Cycles) != 1 || len(view.Cycles[0].R1Conclusions) != 1 {
		t.Fatalf("expected 1 cycle 1 conclusion, got %+v", view.Cycles)
	}
	c := view.Cycles[0].R1Conclusions[0]
	if c.ConclusionID != "RC1" || c.PropertyID != "RP1" || c.JoinDisposition != F1R1JoinMerge {
		t.Fatalf("core fields not verbatim: %+v", c)
	}
	if len(c.Lanes) != 2 || c.Lanes[0].ActID != "act-1" {
		t.Fatalf("lanes not verbatim: %+v", c.Lanes)
	}
	if len(c.Sources) != 1 || len(c.Sources[0].AncestryRoots) != 1 || c.Sources[0].AncestryRoots[0] != "root-1" {
		t.Fatalf("sources not verbatim: %+v", c.Sources)
	}
	if len(c.Agreements) != 1 || c.Agreements[0] != "agree-1" {
		t.Fatalf("agreements not verbatim: %+v", c.Agreements)
	}
	if len(c.Contradictions) != 1 || c.Contradictions[0].ContradictionID != "contra-1" {
		t.Fatalf("contradictions not verbatim: %+v", c.Contradictions)
	}
	if len(c.Gaps) != 1 || c.Gaps[0].GapID != "gap-1" {
		t.Fatalf("gaps not verbatim: %+v", c.Gaps)
	}
	if len(c.Hazards) != 1 || c.Hazards[0].HazardRef != "hazard-1" {
		t.Fatalf("hazards not verbatim: %+v", c.Hazards)
	}
	if len(c.Hazards[0].ConsumingR3OptionIDs) != 1 || c.Hazards[0].ConsumingPAProbeIDs[0] != "PA-X" {
		t.Fatalf("hazard consuming refs not verbatim: %+v", c.Hazards[0])
	}
}

func TestScanF2R1Streak_NoNewRelationshipAppears(t *testing.T) {
	dir := t.TempDir()
	streakSidecarForCycle(t, dir, "cycle-a", "a.canonical.json", "CA", "PA")
	streakSidecarForCycle(t, dir, "cycle-b", "b.canonical.json", "CB", "PB")

	view, _, err := ScanF2R1Streak(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	out, err := RenderF2R1Streak(view)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	rendered := string(out)

	// The streak MUST NOT introduce cross-cycle linkage vocabulary.
	forbidden := []string{
		"streak continues",
		"follows from",
		"supersedes",
		"recurs across",
		"this conclusion is part of a streak",
		"derived recurrence",
	}
	for _, f := range forbidden {
		if strings.Contains(strings.ToLower(rendered), strings.ToLower(f)) {
			t.Fatalf("rendered streak introduces forbidden cross-cycle linkage vocabulary %q", f)
		}
	}

	// Both cycles' conclusions must appear (independently, in order).
	if !strings.Contains(rendered, "cycle-a") || !strings.Contains(rendered, "CA") {
		t.Fatalf("cycle-a / CA missing from streak")
	}
	if !strings.Contains(rendered, "cycle-b") || !strings.Contains(rendered, "CB") {
		t.Fatalf("cycle-b / CB missing from streak")
	}

	// cycle-a MUST appear before cycle-b (chronological).
	idxA := strings.Index(rendered, "cycle-a")
	idxB := strings.Index(rendered, "cycle-b")
	if idxA < 0 || idxB < 0 || idxA > idxB {
		t.Fatalf("chronological order violated: idxA=%d idxB=%d", idxA, idxB)
	}
}

func TestScanF2R1Streak_RepeatedScanIdentical(t *testing.T) {
	dir := t.TempDir()
	streakSidecarForCycle(t, dir, "cycle-001", "z.canonical.json", "C1", "P1")
	streakSidecarForCycle(t, dir, "cycle-002", "y.canonical.json", "C2", "P2")
	streakSidecarForCycle(t, dir, "cycle-003", "x.canonical.json", "C3", "P3")

	view1, diags1, err := ScanF2R1Streak(dir)
	if err != nil {
		t.Fatalf("scan1: %v", err)
	}
	view2, diags2, err := ScanF2R1Streak(dir)
	if err != nil {
		t.Fatalf("scan2: %v", err)
	}

	out1, _ := json.Marshal(view1)
	out2, _ := json.Marshal(view2)
	if string(out1) != string(out2) {
		t.Fatalf("scan not deterministic across calls:\n---1---\n%s\n---2---\n%s", out1, out2)
	}

	d1, _ := json.Marshal(diags1)
	d2, _ := json.Marshal(diags2)
	if string(d1) != string(d2) {
		t.Fatalf("diagnostics not deterministic:\n---1---\n%s\n---2---\n%s", d1, d2)
	}

	out3, err := RenderF2R1Streak(view1)
	if err != nil {
		t.Fatalf("render1: %v", err)
	}
	out4, err := RenderF2R1Streak(view2)
	if err != nil {
		t.Fatalf("render2: %v", err)
	}
	if string(out3) != string(out4) {
		t.Fatalf("render not deterministic")
	}
}

func TestScanF2R1Streak_NonCanonicalFilesIgnored(t *testing.T) {
	dir := t.TempDir()
	streakSidecarForCycle(t, dir, "cycle-valid", "valid.canonical.json", "CV", "PV")
	// Non-canonical files in the same dir must be ignored.
	writeStreakSidecarBytes(t, dir, "notes.txt", []byte("irrelevant"))
	writeStreakSidecarBytes(t, dir, "cycle.md", []byte("# some MD"))
	writeStreakSidecarBytes(t, dir, "data.json", []byte("{}"))

	view, _, err := ScanF2R1Streak(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(view.Cycles) != 1 {
		t.Fatalf("expected only the .canonical.json to be scanned, got %d cycles", len(view.Cycles))
	}
}

func TestScanF2R1Streak_NilRendererReturnsError(t *testing.T) {
	if _, err := RenderF2R1Streak(nil); err == nil {
		t.Fatalf("expected error for nil view")
	}
}

func TestRenderF2R1Streak_EmptyViewRendersBoundedAbsence(t *testing.T) {
	view := &F2R1StreakView{
		Cycles:          nil,
		RendererVersion: F2StreakRendererVersion,
		ExcludedCount:   0,
	}
	out, err := RenderF2R1Streak(view)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	rendered := string(out)
	if !strings.Contains(rendered, "(no valid F2 cycles in checked scope)") {
		t.Fatalf("empty streak should render bounded absence notice")
	}
	if !strings.Contains(rendered, "f2-r1-streak:begin") || !strings.Contains(rendered, "f2-r1-streak:end") {
		t.Fatalf("structural markers missing")
	}
	if !strings.Contains(rendered, F2StreakRendererVersion) {
		t.Fatalf("renderer version not printed")
	}
}

func TestRenderF2R1Streak_DerivedNoticePresent(t *testing.T) {
	dir := t.TempDir()
	streakSidecarForCycle(t, dir, "cycle-1", "c1.canonical.json", "C1", "P1")
	view, _, err := ScanF2R1Streak(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	out, err := RenderF2R1Streak(view)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	rendered := string(out)
	for _, required := range []string{"Derived", "informational", "non-authoritative"} {
		if !strings.Contains(rendered, required) {
			t.Fatalf("standing notice missing %q", required)
		}
	}
	// The fence must NOT claim F2 positively infers/synthesizes. The standing
	// notice DOES list forbidden acts ("it does NOT infer streak continuity"),
	// which is correct language — we check that no line POSITIVELY claims F2
	// performs synthesis, not that the forbidden words don't appear at all.
	for _, line := range strings.Split(rendered, "\n") {
		trimmed := strings.TrimSpace(strings.TrimPrefix(line, ">"))
		lower := strings.ToLower(trimmed)
		// Skip lines that are negations ("does NOT ...") — those are correct.
		if strings.Contains(lower, "does not") || strings.Contains(lower, "never") {
			continue
		}
		for _, forbidden := range []string{"infers", "synthesizes", "creates hazard links across"} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("line positively claims forbidden act %q: %q", forbidden, trimmed)
			}
		}
	}
}

func TestSerializeF2R1StreakView_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	streakSidecarForCycle(t, dir, "cycle-1", "c1.canonical.json", "C1", "P1")
	view, _, err := ScanF2R1Streak(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	bytes, err := SerializeF2R1StreakView(view)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	var decoded F2R1StreakView
	if err := json.Unmarshal(bytes, &decoded); err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}
	if decoded.RendererVersion != view.RendererVersion {
		t.Fatalf("renderer version lost in round-trip")
	}
	if len(decoded.Cycles) != 1 || decoded.Cycles[0].SynthesisCycleID != "cycle-1" {
		t.Fatalf("cycle lost in round-trip: %+v", decoded.Cycles)
	}
}
