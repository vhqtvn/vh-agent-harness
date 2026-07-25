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
