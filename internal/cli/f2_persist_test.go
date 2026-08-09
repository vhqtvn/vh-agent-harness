package cli

// f2_persist_test.go — tests for the immutable canonical sidecar persistence
// (Slice 2). Covers the collision contract (memo L137-139): fresh write,
// idempotent rerun, conflicting-content refusal, and the round-trip /
// determinism guarantees.
//
// The crux test is TestPersistF2CanonicalSidecar_IdempotentRerunIsNoOp: it
// proves that a re-run of the same canonical content does NOT touch the file
// (the original bytes and timestamp are preserved) — the load-bearing
// immutability behavior.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// f2IngestFromFixture builds a fresh F2IngestResult from the canonical fixture.
// Reuses the Slice 1 test helper (f2EmitFromFixture) + the ingest gate.
func f2IngestFromFixture(t *testing.T) *F2IngestResult {
	t.Helper()
	emit := f2EmitFromFixture(t)
	result, errs := IngestF1EmitForF2(emit)
	if len(errs) > 0 {
		t.Fatalf("IngestF1EmitForF2 on canonical fixture failed (setup): %v", errs)
	}
	if result == nil {
		t.Fatal("IngestF1EmitForF2 returned nil result with no errors (setup)")
	}
	return result
}

// fixedTime is the deterministic timestamp used across tests so byte-for-byte
// comparisons are stable.
var fixedTime = time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)

// --- Deterministic serialization -------------------------------------------

func TestSerializeF2CanonicalSidecar_DeterministicAcrossCalls(t *testing.T) {
	ingest := f2IngestFromFixture(t)
	sidecar := buildF2CanonicalSidecar(ingest, "docs/checkpoints/f2", fixedTime)

	first, err := SerializeF2CanonicalSidecar(sidecar)
	if err != nil {
		t.Fatalf("first serialization failed: %v", err)
	}
	second, err := SerializeF2CanonicalSidecar(sidecar)
	if err != nil {
		t.Fatalf("second serialization failed: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("deterministic serialization violated: two calls produced different bytes")
	}
}

func TestSerializeF2CanonicalSidecar_RoundTripEquality(t *testing.T) {
	ingest := f2IngestFromFixture(t)
	original := buildF2CanonicalSidecar(ingest, "docs/checkpoints/f2", fixedTime)

	raw, err := SerializeF2CanonicalSidecar(original)
	if err != nil {
		t.Fatalf("serialization failed: %v", err)
	}
	var decoded F2CanonicalSidecar
	if jErr := json.Unmarshal(raw, &decoded); jErr != nil {
		t.Fatalf("round-trip deserialization failed: %v", jErr)
	}

	// Verify the canonical envelope round-trips losslessly.
	reRaw, err := json.Marshal(decoded.CanonicalEnvelope)
	if err != nil {
		t.Fatalf("cannot re-marshal decoded envelope: %v", err)
	}
	origRaw, err := json.Marshal(original.CanonicalEnvelope)
	if err != nil {
		t.Fatalf("cannot marshal original envelope: %v", err)
	}
	if string(reRaw) != string(origRaw) {
		t.Fatalf("round-trip envelope content changed: serialized bytes differ")
	}

	// Verify F2 view metadata round-trips.
	if decoded.F2ViewMetadata.SynthesisCycleID != original.F2ViewMetadata.SynthesisCycleID {
		t.Fatalf("round-trip view metadata cycle mismatch")
	}
	if decoded.F2ViewMetadata.SourceSemanticDigest != original.F2ViewMetadata.SourceSemanticDigest {
		t.Fatalf("round-trip view metadata digest mismatch")
	}
	if decoded.Kind != F2CanonicalSidecarKind {
		t.Fatalf("round-trip kind mismatch: got %q want %q", decoded.Kind, F2CanonicalSidecarKind)
	}
}

// --- Fresh write ------------------------------------------------------------

func TestPersistF2CanonicalSidecar_NewCycleWritesFile(t *testing.T) {
	dir := t.TempDir()
	ingest := f2IngestFromFixture(t)

	outcome, err := PersistF2CanonicalSidecar(ingest, dir, fixedTime)
	if err != nil {
		t.Fatalf("fresh persist failed: %v", err)
	}
	if outcome != F2PersistWritten {
		t.Fatalf("fresh persist outcome = %s, want written", outcome)
	}

	path := F2CanonicalSidecarPath(dir, ingest.SynthesisCycleID)
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("canonical sidecar was not written to %q: %v", path, statErr)
	}

	// The written file must be parseable and carry the right kind.
	sidecar, readErr := ReadF2CanonicalSidecar(path)
	if readErr != nil {
		t.Fatalf("written sidecar is not readable: %v", readErr)
	}
	if sidecar.Kind != F2CanonicalSidecarKind {
		t.Fatalf("written sidecar kind = %q, want %q", sidecar.Kind, F2CanonicalSidecarKind)
	}
}

// --- Idempotent rerun (crux) ------------------------------------------------

func TestPersistF2CanonicalSidecar_IdempotentRerunIsNoOp(t *testing.T) {
	dir := t.TempDir()
	ingest := f2IngestFromFixture(t)

	// First write at fixedTime.
	if outcome, err := PersistF2CanonicalSidecar(ingest, dir, fixedTime); err != nil || outcome != F2PersistWritten {
		t.Fatalf("first write failed: outcome=%s err=%v", outcome, err)
	}

	path := F2CanonicalSidecarPath(dir, ingest.SynthesisCycleID)
	originalBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read first-written sidecar: %v", err)
	}

	// Second write at a DIFFERENT time (the timestamp in the new bytes would
	// differ, but the canonical content is identical → must be idempotent).
	laterTime := fixedTime.Add(2 * time.Hour)
	outcome, err := PersistF2CanonicalSidecar(ingest, dir, laterTime)
	if err != nil {
		t.Fatalf("idempotent rerun returned error: %v", err)
	}
	if outcome != F2PersistIdempotent {
		t.Fatalf("idempotent rerun outcome = %s, want idempotent", outcome)
	}

	// CRUX: the file must NOT have been touched. Original bytes preserved.
	afterBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read sidecar after rerun: %v", err)
	}
	if string(originalBytes) != string(afterBytes) {
		t.Fatalf("idempotent rerun modified the file: bytes changed (immutability violation — the original timestamp and bytes must be preserved)")
	}
}

// --- Conflicting write refused ----------------------------------------------

func TestPersistF2CanonicalSidecar_DifferentContentRefused(t *testing.T) {
	dir := t.TempDir()
	ingest := f2IngestFromFixture(t)

	// First write.
	if outcome, err := PersistF2CanonicalSidecar(ingest, dir, fixedTime); err != nil || outcome != F2PersistWritten {
		t.Fatalf("first write failed: outcome=%s err=%v", outcome, err)
	}

	path := F2CanonicalSidecarPath(dir, ingest.SynthesisCycleID)
	originalBytes, _ := os.ReadFile(path)

	// Build a SECOND ingest for the SAME cycle but with DIFFERENT canonical
	// content (mutate a canonical field → different content, same cycle ID).
	// This simulates a changed canonical field under the same cycle — the
	// immutability contract must refuse the overwrite.
	secondIngest := f2IngestFromFixture(t)
	// Mutate a canonical field on the deep-copied envelope. This produces a
	// different canonical content fingerprint for the same cycle ID.
	if len(secondIngest.CanonicalEnvelope.Entries) > 0 {
		secondIngest.CanonicalEnvelope.Entries[0].EntryID = "entry-mutated"
	}

	outcome, err := PersistF2CanonicalSidecar(secondIngest, dir, fixedTime.Add(time.Hour))
	if outcome != F2PersistRefused {
		t.Fatalf("different-content persist outcome = %s, want refused", outcome)
	}
	if err == nil {
		t.Fatal("different-content persist returned nil error (expected a refusal error)")
	}
	if !strings.Contains(err.Error(), "new F1 emit") && !strings.Contains(err.Error(), "new synthesis cycle") {
		t.Fatalf("refusal error does not mention new cycle requirement: %q", err.Error())
	}

	// The existing file must NOT have been overwritten.
	afterBytes, _ := os.ReadFile(path)
	if string(originalBytes) != string(afterBytes) {
		t.Fatalf("refused persist modified the existing file (immutability violation)")
	}
}

// --- New cycle gets its own file (no collision across cycles) ----------------

func TestPersistF2CanonicalSidecar_DifferentCycleIsIndependentFile(t *testing.T) {
	dir := t.TempDir()
	first := f2IngestFromFixture(t)

	// Write cycle-001.
	if outcome, err := PersistF2CanonicalSidecar(first, dir, fixedTime); err != nil || outcome != F2PersistWritten {
		t.Fatalf("first cycle write failed: outcome=%s err=%v", outcome, err)
	}

	// Build a second ingest with a DIFFERENT cycle ID (different file path →
	// no collision → fresh write).
	second := f2IngestFromFixture(t)
	second.SynthesisCycleID = "cycle-002"
	// Also update the envelope's cycle ID so the canonical content is
	// self-consistent (the sidecar carries both the wrapper cycle and the
	// envelope's own cycle).
	second.CanonicalEnvelope.SynthesisCycleID = "cycle-002"

	outcome, err := PersistF2CanonicalSidecar(second, dir, fixedTime)
	if err != nil {
		t.Fatalf("second cycle write failed: %v", err)
	}
	if outcome != F2PersistWritten {
		t.Fatalf("second cycle outcome = %s, want written (different cycle = independent file)", outcome)
	}

	// Both files must exist.
	firstPath := F2CanonicalSidecarPath(dir, "cycle-001")
	secondPath := F2CanonicalSidecarPath(dir, "cycle-002")
	if _, err := os.Stat(firstPath); err != nil {
		t.Fatalf("first cycle file missing: %v", err)
	}
	if _, err := os.Stat(secondPath); err != nil {
		t.Fatalf("second cycle file missing: %v", err)
	}
}

// --- Nil / invalid ingest rejected ------------------------------------------

func TestPersistF2CanonicalSidecar_NilIngestRejected(t *testing.T) {
	dir := t.TempDir()
	outcome, err := PersistF2CanonicalSidecar(nil, dir, fixedTime)
	if outcome != F2PersistNotAttempted {
		t.Fatalf("nil ingest outcome = %s, want not_attempted", outcome)
	}
	if err == nil {
		t.Fatal("nil ingest returned nil error")
	}
}

// --- Corrupt existing file NOT silently overwritten -------------------------

func TestPersistF2CanonicalSidecar_CorruptExistingFileRefused(t *testing.T) {
	dir := t.TempDir()
	ingest := f2IngestFromFixture(t)
	path := F2CanonicalSidecarPath(dir, ingest.SynthesisCycleID)

	// Pre-create the sidecar path with corrupt (non-JSON) content. This
	// simulates a partially-written or manually-corrupted artifact.
	if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
		t.Fatalf("mkdir failed: %v", mkErr)
	}
	if wErr := os.WriteFile(path, []byte("{not valid json"), 0o644); wErr != nil {
		t.Fatalf("cannot seed corrupt file: %v", wErr)
	}

	// A valid ingest for the same cycle must REFUSE to overwrite the corrupt
	// file (immutability: never silently overwrite). The outcome is Refused,
	// not Written, and the corrupt bytes are preserved.
	outcome, err := PersistF2CanonicalSidecar(ingest, dir, fixedTime)
	if outcome != F2PersistRefused {
		t.Fatalf("corrupt-existing outcome = %s, want refused (must not silently overwrite a corrupt artifact)", outcome)
	}
	if err == nil {
		t.Fatal("corrupt-existing persist returned nil error (expected a refuse error)")
	}

	// The corrupt bytes must be preserved (not overwritten).
	afterBytes, _ := os.ReadFile(path)
	if string(afterBytes) != "{not valid json" {
		t.Fatalf("corrupt file was modified (immutability violation): got %q", string(afterBytes))
	}
}

// --- Reciprocal locator correctness -----------------------------------------

func TestPersistF2CanonicalSidecar_ReciprocalLocatorPointsToMD(t *testing.T) {
	dir := t.TempDir()
	ingest := f2IngestFromFixture(t)

	if _, err := PersistF2CanonicalSidecar(ingest, dir, fixedTime); err != nil {
		t.Fatalf("persist failed: %v", err)
	}

	path := F2CanonicalSidecarPath(dir, ingest.SynthesisCycleID)
	sidecar, _ := ReadF2CanonicalSidecar(path)

	expected := filepath.Join(dir, ingest.SynthesisCycleID+".md")
	if sidecar.F2ViewMetadata.ReciprocalLocator != expected {
		t.Fatalf("reciprocal locator = %q, want %q", sidecar.F2ViewMetadata.ReciprocalLocator, expected)
	}
}

// --- View metadata completeness ---------------------------------------------

func TestPersistF2CanonicalSidecar_ViewMetadataCarriesAllRequiredFields(t *testing.T) {
	dir := t.TempDir()
	ingest := f2IngestFromFixture(t)

	if _, err := PersistF2CanonicalSidecar(ingest, dir, fixedTime); err != nil {
		t.Fatalf("persist failed: %v", err)
	}

	path := F2CanonicalSidecarPath(dir, ingest.SynthesisCycleID)
	sidecar, _ := ReadF2CanonicalSidecar(path)
	meta := sidecar.F2ViewMetadata

	// All F2 view metadata fields required by memo L66-69 must be present.
	if meta.SynthesisCycleID != ingest.SynthesisCycleID {
		t.Fatalf("view metadata cycle mismatch")
	}
	if len(meta.EntryIDs) != len(ingest.EntryIDs) {
		t.Fatalf("view metadata entry_ids count mismatch: got %d want %d", len(meta.EntryIDs), len(ingest.EntryIDs))
	}
	if meta.SourceSemanticDigest != ingest.SemanticDigest {
		t.Fatalf("view metadata digest mismatch")
	}
	if meta.CanonicalRepresentationVersion != F2CanonicalRepresentationVersion {
		t.Fatalf("canonical representation version mismatch")
	}
	if meta.SchemaVersion != ingest.SchemaVersion {
		t.Fatalf("schema version mismatch")
	}
	if meta.ProjectionVersion != F2ProjectionVersion {
		t.Fatalf("projection version mismatch")
	}
	if meta.RendererVersion != F2RendererVersion {
		t.Fatalf("renderer version mismatch")
	}
	if meta.WriteTimestamp != fixedTime.Format(time.RFC3339) {
		t.Fatalf("write timestamp mismatch: got %q want %q", meta.WriteTimestamp, fixedTime.Format(time.RFC3339))
	}
}

// --- No-join / no-synthesize structural check --------------------------------

func TestPersistF2CanonicalSidecar_NoContentGenerationOperation(t *testing.T) {
	// PersistF2CanonicalSidecar takes a *F2IngestResult and writes it
	// losslessly. There is no API path to add an R1 conclusion, R3 option,
	// P-a probe, or any canonical content through persistence. The signature
	// accepts (*F2IngestResult, string, time.Time) and returns
	// (F2PersistOutcome, error) — no content-producing parameter exists.
	// This test is a compile-time guarantee: if someone adds a content-
	// producing parameter, the test body below would need to change.
	var _ func(*F2IngestResult, string, time.Time) (F2PersistOutcome, error) = PersistF2CanonicalSidecar
}

// --- Transient-locator admission gate (narrow durable-path check) -----------
//
// Design authority: researches/decisions/2026-07-25-f2-rendering-family-
// mechanism.md (transient-locator contract sentence). Before writing an F2
// artifact pair, persistence must refuse any repo-relative canonical provenance
// locator lexically rooted under tmp/agent-runs/. The recovery is a new F1 emit
// under a new synthesis cycle with a durable locator — never an in-place
// rewrite (F2 must NOT rewrite locators).
//
// Test matrix (from the build brief):
//   - lexical normalization: backslash→/; strip one leading "./".
//   - REJECT: tmp/agent-runs (exact), tmp/agent-runs/child, ./tmp/agent-runs/x,
//     tmp\agent-runs\x.
//   - ACCEPT: docs/researches/x.md, researches/x.md, .local/coordinator/x.json,
//     tmp/scratch/x (other tmp root — narrow), https://example.com/x (URL),
//     /tmp/agent-runs/x (absolute — non-goal).
//   - each of the 4 canonical provenance locator fields is walked and the error
//     names the offending field path + locator.
//   - CheckedScope is NOT enumerated (narrow scope — scope-description field).
//   - the canonical fixture (no transient locators) is admitted.
//   - integration: PersistF2CanonicalSidecar + PersistF2Pair refuse pre-write
//     and create NEITHER the JSON nor the MD member.

func TestF2NormalizeLocator_MinimalLexicalNormalization(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain repo-relative unchanged", "docs/researches/x.md", "docs/researches/x.md"},
		{"backslash to slash", "tmp\\agent-runs\\x.md", "tmp/agent-runs/x.md"},
		{"strip single leading dot-slash", "./tmp/agent-runs/x.md", "tmp/agent-runs/x.md"},
		{"absolute preserved (leading slash stays)", "/tmp/agent-runs/x.md", "/tmp/agent-runs/x.md"},
		{"url preserved", "https://example.com/x", "https://example.com/x"},
		{"repeated dot-slash stripped once (minimal)", "././tmp/agent-runs/x.md", "./tmp/agent-runs/x.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := f2NormalizeLocator(tc.in); got != tc.want {
				t.Fatalf("f2NormalizeLocator(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestF2IsTransientAgentRunsLocator_ExactPrefixAndExclusions(t *testing.T) {
	reject := []string{
		"tmp/agent-runs",
		"tmp/agent-runs/",
		"tmp/agent-runs/x.md",
		"tmp/agent-runs/sub/deep.md",
		"./tmp/agent-runs/x.md",
		"tmp\\agent-runs\\x.md",
	}
	for _, in := range reject {
		if !f2IsTransientAgentRunsLocator(in) {
			t.Fatalf("expected REJECT for %q (repo-relative, rooted under tmp/agent-runs/)", in)
		}
	}
	accept := []string{
		"", // empty locator — not rooted under the disposable root
		"docs/researches/x.md",
		"researches/decisions/x.md",
		".local/coordinator/tasks/x.json",
		"tmp/scratch/x.md",     // other tmp root — narrow check, NOT classified
		"tmp/agent-runs-other", // sibling prefix, not a child of tmp/agent-runs/
		"https://example.com/x",
		"http://example.com/tmp/agent-runs/x",
		"/tmp/agent-runs/x", // absolute — non-goal, NOT classified
		"/home/u/repo/tmp/agent-runs/x",
		"src-a",
	}
	for _, in := range accept {
		if f2IsTransientAgentRunsLocator(in) {
			t.Fatalf("expected ACCEPT for %q (not repo-relative, or not rooted under tmp/agent-runs/)", in)
		}
	}
}

// TestValidateNoTransientProvenanceLocators_RejectsEachLocatorField proves the
// walk reaches every one of the four canonical provenance locator fields and
// the error names the offending field path + locator. Uses hand-constructed
// envelopes (the pure helper takes *F1SynthesisEnvelope directly — no F1
// validation, so each field is isolated without the hazard-source resolution
// constraint).
func TestValidateNoTransientProvenanceLocators_RejectsEachLocatorField(t *testing.T) {
	transient := "tmp/agent-runs/x.md"
	cases := []struct {
		name      string
		env       *F1SynthesisEnvelope
		wantField string // substring of the error's field path
	}{
		{
			name: "entry source_refs",
			env: &F1SynthesisEnvelope{Entries: []F1FamilyEntry{
				{SourceRefs: []string{transient}},
			}},
			wantField: "entries[0].source_refs[0]",
		},
		{
			name: "r1 conclusion source locator",
			env: &F1SynthesisEnvelope{Entries: []F1FamilyEntry{
				{R1: &F1R1JoinSummary{Conclusions: []F1R1Conclusion{
					{Sources: []F1R1Source{{Locator: transient}}},
				}}},
			}},
			wantField: "entries[0].r1.conclusions[0].sources[0].locator",
		},
		{
			name: "r1 hazard source_locators",
			env: &F1SynthesisEnvelope{Entries: []F1FamilyEntry{
				{R1: &F1R1JoinSummary{Conclusions: []F1R1Conclusion{
					{Hazards: []F1R1HazardLink{{SourceLocators: []string{transient}}}},
				}}},
			}},
			wantField: "entries[0].r1.conclusions[0].hazards[0].source_locators[0]",
		},
		{
			name: "pa probe evidence_refs",
			env: &F1SynthesisEnvelope{Entries: []F1FamilyEntry{
				{PA: &F1PAProbeSummary{Probes: []F1PAProbe{
					{EvidenceRefs: []string{transient}},
				}}},
			}},
			wantField: "entries[0].pa.probes[0].evidence_refs[0]",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateNoTransientProvenanceLocators(tc.env)
			if err == nil {
				t.Fatalf("expected a transient-locator rejection, got nil")
			}
			msg := err.Error()
			if !strings.Contains(msg, tc.wantField) {
				t.Fatalf("error does not name the offending field path %q:\n%s", tc.wantField, msg)
			}
			if !strings.Contains(msg, transient) {
				t.Fatalf("error does not name the offending locator %q:\n%s", transient, msg)
			}
		})
	}
}

// TestValidateNoTransientProvenanceLocators_AcceptsNonTransientLocators proves
// the gate does NOT classify other locator roots (the narrow admission check —
// NOT a general durability classifier). docs/, researches/, .local/, another
// tmp root, a URL, and an absolute path are all admitted.
func TestValidateNoTransientProvenanceLocators_AcceptsNonTransientLocators(t *testing.T) {
	env := &F1SynthesisEnvelope{Entries: []F1FamilyEntry{
		{
			SourceRefs: []string{
				"docs/researches/x.md",
				"researches/decisions/y.md",
				".local/coordinator/tasks/z.json",
				"tmp/scratch/other-root.md", // other tmp root — NOT classified
				"https://example.com/evidence",
				"/tmp/agent-runs/absolute.md", // absolute — non-goal, NOT classified
			},
			R1: &F1R1JoinSummary{Conclusions: []F1R1Conclusion{
				{Sources: []F1R1Source{{Locator: "src-a"}}},
			}},
		},
	}}
	if err := validateNoTransientProvenanceLocators(env); err != nil {
		t.Fatalf("non-transient locators were rejected (narrow-scope violation): %v", err)
	}
}

// TestValidateNoTransientProvenanceLocators_DoesNotClassifyCheckedScope proves
// the walk does NOT enumerate F1PAProbe.CheckedScope. CheckedScope is a scope-
// DESCRIPTION field ("the scope that was checked"), not a provenance locator.
// A transient path in CheckedScope is out of scope for this narrow gate.
func TestValidateNoTransientProvenanceLocators_DoesNotClassifyCheckedScope(t *testing.T) {
	env := &F1SynthesisEnvelope{Entries: []F1FamilyEntry{
		{PA: &F1PAProbeSummary{Probes: []F1PAProbe{
			{CheckedScope: []string{"tmp/agent-runs/checked-here.md"}},
		}}},
	}}
	if err := validateNoTransientProvenanceLocators(env); err != nil {
		t.Fatalf("CheckedScope was classified (over-broad — narrow-scope violation): %v", err)
	}
}

// TestValidateNoTransientProvenanceLocators_CanonicalFixtureAdmitted proves the
// golden fixture (no transient locators) passes the gate, so the existing dated
// F2 pair behavior is unchanged.
func TestValidateNoTransientProvenanceLocators_CanonicalFixtureAdmitted(t *testing.T) {
	if err := validateNoTransientProvenanceLocators(canonicalF1Fixture()); err != nil {
		t.Fatalf("canonical fixture was rejected by the transient-locator gate (existing dated pair would break): %v", err)
	}
}

// f2IngestWithMutation deep-copies the canonical fixture, applies mutate, then
// re-derives the digest and runs EmitF1 + IngestF1EmitForF2 so the result is a
// valid ingest result carrying a binding digest with the mutation baked into
// canonical content. Used by the integration tests.
func f2IngestWithMutation(t *testing.T, mutate func(env *F1SynthesisEnvelope)) *F2IngestResult {
	t.Helper()
	fixture := canonicalF1Fixture()
	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	var env F1SynthesisEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	mutate(&env)
	// Recompute the digest after mutation so the envelope is self-consistent.
	d, err := env.ComputeDigest()
	if err != nil {
		t.Fatalf("compute digest after mutation: %v", err)
	}
	env.SemanticDigest = d
	env.Validation = F1ValidationInfo{Disposition: F1ValidationComplete}
	emit, errs := EmitF1(&env)
	if emit == nil {
		t.Fatalf("EmitF1 on mutated fixture failed (setup): %v", errs)
	}
	result, errs := IngestF1EmitForF2(emit)
	if result == nil {
		t.Fatalf("IngestF1EmitForF2 on mutated fixture failed (setup): %v", errs)
	}
	return result
}

// f2R1Entry mutates the canonical fixture's R1 entry (entry-r1). Returns nothing;
// callers use it inside f2IngestWithMutation. The fixture's R1 entry is
// entries[0] with conclusion R1C1 at conclusions[0].
func f2MutateR1SourceLocator(transient string) func(*F1SynthesisEnvelope) {
	return func(env *F1SynthesisEnvelope) {
		for i := range env.Entries {
			if env.Entries[i].Family == F1FamilyR1CrossLaneJoin && env.Entries[i].R1 != nil {
				if len(env.Entries[i].R1.Conclusions) > 0 && len(env.Entries[i].R1.Conclusions[0].Sources) > 0 {
					env.Entries[i].R1.Conclusions[0].Sources[0].Locator = transient
				}
			}
		}
	}
}

// TestPersistF2CanonicalSidecar_TransientLocatorRejectedPreWrite is the
// integration crux for the JSON-only persist entrypoint: an ingest whose
// canonical envelope carries a transient provenance locator is refused BEFORE
// the sidecar file is created. The outcome is NotAttempted (pre-write content
// rejection, not a collision refusal) and the error names the offending field
// + locator.
func TestPersistF2CanonicalSidecar_TransientLocatorRejectedPreWrite(t *testing.T) {
	dir := t.TempDir()
	transient := "tmp/agent-runs/f1-build/SLICE-0-STOP.md"
	ingest := f2IngestWithMutation(t, f2MutateR1SourceLocator(transient))

	outcome, err := PersistF2CanonicalSidecar(ingest, dir, fixedTime)
	if outcome != F2PersistNotAttempted {
		t.Fatalf("transient-locator persist outcome = %s, want not_attempted (pre-write rejection)", outcome)
	}
	if err == nil {
		t.Fatal("transient-locator persist returned nil error (expected a refusal)")
	}
	msg := err.Error()
	if !strings.Contains(msg, "tmp/agent-runs") {
		t.Fatalf("refusal does not name the disposable root:\n%s", msg)
	}
	if !strings.Contains(msg, transient) {
		t.Fatalf("refusal does not name the offending locator %q:\n%s", transient, msg)
	}
	if !strings.Contains(msg, "sources[0].locator") {
		t.Fatalf("refusal does not name the offending field path:\n%s", msg)
	}

	// CRUX: NEITHER the sidecar NOR any file was created.
	path := F2CanonicalSidecarPath(dir, ingest.SynthesisCycleID)
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("transient-locator persist created a file at %q (must refuse pre-write)", path)
	}
}

// TestPersistF2Pair_TransientLocatorRejectedCreatesNeitherMember is the
// integration crux for the pair persist entrypoint: an ingest whose canonical
// envelope carries a transient provenance locator is refused BEFORE EITHER the
// JSON sidecar OR the MD projection is created. This proves both entrypoints
// converge on the same admission decision via the shared helper.
func TestPersistF2Pair_TransientLocatorRejectedCreatesNeitherMember(t *testing.T) {
	dir := t.TempDir()
	transient := "tmp/agent-runs/f1-build/SLICE-0-STOP.md"
	ingest := f2IngestWithMutation(t, f2MutateR1SourceLocator(transient))

	outcome, err := PersistF2Pair(ingest, dir, fixedTime)
	if outcome != F2PairNotAttempted {
		t.Fatalf("transient-locator pair outcome = %s, want not_attempted (pre-write rejection)", outcome)
	}
	if err == nil {
		t.Fatal("transient-locator pair returned nil error (expected a refusal)")
	}
	if !strings.Contains(err.Error(), transient) {
		t.Fatalf("pair refusal does not name the offending locator %q:\n%s", transient, err.Error())
	}

	// CRUX: NEITHER the canonical sidecar NOR the MD projection was created.
	canonPath := F2CanonicalSidecarPath(dir, ingest.SynthesisCycleID)
	mdPath := F2MarkdownProjectionPath(dir, ingest.SynthesisCycleID)
	if _, statErr := os.Stat(canonPath); !os.IsNotExist(statErr) {
		t.Fatalf("transient-locator pair created the canonical sidecar at %q (must refuse pre-write)", canonPath)
	}
	if _, statErr := os.Stat(mdPath); !os.IsNotExist(statErr) {
		t.Fatalf("transient-locator pair created the MD projection at %q (must refuse pre-write)", mdPath)
	}
}

// TestPersistF2CanonicalSidecar_NonTransientLocatorAccepted proves a durable
// locator is admitted end-to-end (the gate does not over-reject). A locator
// under docs/researches/ passes the gate and the sidecar is written normally.
func TestPersistF2CanonicalSidecar_NonTransientLocatorAccepted(t *testing.T) {
	dir := t.TempDir()
	ingest := f2IngestWithMutation(t, f2MutateR1SourceLocator("docs/researches/decisions/durable.md"))

	outcome, err := PersistF2CanonicalSidecar(ingest, dir, fixedTime)
	if err != nil {
		t.Fatalf("durable-locator persist failed (gate over-rejected): %v", err)
	}
	if outcome != F2PersistWritten {
		t.Fatalf("durable-locator persist outcome = %s, want written", outcome)
	}
	path := F2CanonicalSidecarPath(dir, ingest.SynthesisCycleID)
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("durable-locator sidecar was not written: %v", statErr)
	}
}
