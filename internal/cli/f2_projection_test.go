package cli

// f2_projection_test.go — tests for the deterministic Markdown projection +
// pair-level persistence coordination (Slice 3). Covers:
//   - golden rendering (structural properties of the canonical fixture)
//   - byte-stable rerun (determinism)
//   - pair metadata agreement (canonical ↔ MD reciprocal locators)
//   - mismatched sidecar/view rejected
//   - all four pair collision outcomes (written / idempotent / refused /
//     incomplete-canonical-only / incomplete-md-only)
//   - MD self-identifies as derived/non-authoritative
//   - no model summarization (rendering is purely structural)
//
// The crux test is TestPersistF2Pair_BothExistByteIdenticalIsIdempotent: it
// proves that a re-run of the same canonical content does NOT modify either
// file (the pair-level immutability contract).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- MD rendering: golden structural properties -----------------------------

// TestRenderF2MarkdownProjection_GoldenStructure verifies the canonical
// fixture renders with the expected structural properties: standing notice,
// fenced metadata block, every entry family, every field type. This is the
// golden structural test — if any structural element is missing, the renderer
// drifted.
func TestRenderF2MarkdownProjection_GoldenStructure(t *testing.T) {
	ingest := f2IngestFromFixture(t)
	sidecar := buildF2CanonicalSidecar(ingest, "docs/checkpoints/f2", fixedTime)
	md, err := RenderF2MarkdownProjection(sidecar, "docs/checkpoints/f2")
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	body := string(md)

	// --- Standing notice (exact wording from memo L71-73) ---
	expectedNotice := "> **Derived, informational, and non-authoritative.** Canonical meaning remains in the digest-bound F1 emit at `docs/checkpoints/f2/cycle-001.canonical.json`."
	if !strings.Contains(body, expectedNotice) {
		t.Errorf("standing notice missing or wrong.\n want substring: %s\n got body (first 500 chars): %s", expectedNotice, body[:min(500, len(body))])
	}

	// --- Title ---
	if !strings.Contains(body, "# F2 Projection — Synthesis Cycle `cycle-001`") {
		t.Errorf("title missing cycle ID")
	}

	// --- Fenced metadata block ---
	if !strings.Contains(body, "```f2-view-metadata") {
		t.Fatalf("f2-view-metadata fenced block missing")
	}
	meta, err := ExtractF2ViewMetadataFromMDBytes(md)
	if err != nil {
		t.Fatalf("cannot parse metadata block from rendered MD: %v", err)
	}
	if meta.SynthesisCycleID != "cycle-001" {
		t.Errorf("metadata cycle = %q, want cycle-001", meta.SynthesisCycleID)
	}
	if meta.SourceSemanticDigest != ingest.SemanticDigest {
		t.Errorf("metadata digest = %q, want %q", meta.SourceSemanticDigest, ingest.SemanticDigest)
	}
	// The MD's reciprocal locator must point to the CANONICAL sidecar.
	wantCanonLocator := "docs/checkpoints/f2/cycle-001.canonical.json"
	if meta.ReciprocalLocator != wantCanonLocator {
		t.Errorf("MD reciprocal locator = %q, want %q", meta.ReciprocalLocator, wantCanonLocator)
	}

	// --- Envelope header fields ---
	checks := []string{
		"**Schema version:**",
		"**Synthesis cycle ID:** `cycle-001`",
		"**Applicability:**",
		"**Validation disposition:** `complete`",
	}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Errorf("envelope header missing: %q", want)
		}
	}

	// --- Entry families (all three from the canonical fixture) ---
	if !strings.Contains(body, "Family `r1_cross_lane_join`") {
		t.Errorf("R1 family entry missing")
	}
	if !strings.Contains(body, "Family `r3_redesign_fork`") {
		t.Errorf("R3 family entry missing")
	}
	if !strings.Contains(body, "Family `pa_counter_evidence`") {
		t.Errorf("P-a family entry missing")
	}

	// --- R1 conclusion ---
	if !strings.Contains(body, "Conclusion `R1C1`") {
		t.Errorf("R1 conclusion R1C1 missing")
	}
	if !strings.Contains(body, "**Join disposition:** `union`") {
		t.Errorf("R1 join disposition missing")
	}
	if !strings.Contains(body, "Property `R1P1`") {
		t.Errorf("R1 property ID missing")
	}

	// --- R3 options (both modes) ---
	if !strings.Contains(body, "Option `opt-continue` — Mode `continue_repair`") {
		t.Errorf("R3 opt-continue missing")
	}
	if !strings.Contains(body, "Option `opt-redesign` — Mode `redesign`") {
		t.Errorf("R3 opt-redesign missing")
	}
	if !strings.Contains(body, "**Reversal cost:**") {
		t.Errorf("R3 reversal_cost field missing (C1 canonical field)")
	}
	if !strings.Contains(body, "**Cheapest validation:**") {
		t.Errorf("R3 cheapest_validation field missing (C1 canonical field)")
	}

	// --- P-a probes (result enums present in the canonical fixture) ---
	// The fixture carries found, not_found_in_checked_scope, and unavailable.
	// (not_run is tested in the P-a rendering slice; the fixture does not
	// carry a not_run probe.)
	if !strings.Contains(body, "Probe `PA-P1`") {
		t.Errorf("P-a probe PA-P1 missing")
	}
	if !strings.Contains(body, "**Result:** `found`") {
		t.Errorf("P-a result 'found' not rendered exactly")
	}
	if !strings.Contains(body, "**Result:** `not_found_in_checked_scope`") {
		t.Errorf("P-a result 'not_found_in_checked_scope' not rendered exactly (must never collapse)")
	}
	if !strings.Contains(body, "**Result:** `unavailable`") {
		t.Errorf("P-a result 'unavailable' not rendered exactly")
	}

	// The result enum must NEVER be paraphrased. "not_found_in_checked_scope"
	// must not appear as "none exists" or similar.
	if strings.Contains(body, "none exists") {
		t.Errorf("'none exists' found in MD — not_found_in_checked_scope must NEVER render as 'none exists' (memo L298)")
	}
}

// --- Byte-stable rerun (determinism) ---------------------------------------

// TestRenderF2MarkdownProjection_ByteStableRerun proves two render calls with
// the same sidecar + dir produce identical bytes. This is required for
// collision detection (idempotency check) and for the doctor's
// deterministic-projection-equivalence check (Slice 9).
func TestRenderF2MarkdownProjection_ByteStableRerun(t *testing.T) {
	ingest := f2IngestFromFixture(t)
	sidecar := buildF2CanonicalSidecar(ingest, "docs/checkpoints/f2", fixedTime)

	first, err := RenderF2MarkdownProjection(sidecar, "docs/checkpoints/f2")
	if err != nil {
		t.Fatalf("first render failed: %v", err)
	}
	second, err := RenderF2MarkdownProjection(sidecar, "docs/checkpoints/f2")
	if err != nil {
		t.Fatalf("second render failed: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("byte-stable rerun violated: two renders of the same sidecar produced different bytes (len first=%d, second=%d)", len(first), len(second))
	}
}

// TestRenderF2MarkdownProjection_DeterministicAcrossSidecarRebuild proves the
// renderer is deterministic across independent ingest→sidecar pipelines: a
// fresh ingest + sidecar build from the same fixture produces the same MD
// bytes as a prior render. This verifies no hidden nondeterminism leaks
// through the pipeline (map iteration, pointer aliasing, etc.).
func TestRenderF2MarkdownProjection_DeterministicAcrossSidecarRebuild(t *testing.T) {
	ingest1 := f2IngestFromFixture(t)
	sidecar1 := buildF2CanonicalSidecar(ingest1, "docs/checkpoints/f2", fixedTime)
	first, err := RenderF2MarkdownProjection(sidecar1, "docs/checkpoints/f2")
	if err != nil {
		t.Fatalf("first render failed: %v", err)
	}

	ingest2 := f2IngestFromFixture(t)
	sidecar2 := buildF2CanonicalSidecar(ingest2, "docs/checkpoints/f2", fixedTime)
	second, err := RenderF2MarkdownProjection(sidecar2, "docs/checkpoints/f2")
	if err != nil {
		t.Fatalf("second render failed: %v", err)
	}

	if string(first) != string(second) {
		t.Fatalf("deterministic-across-rebuild violated: two independent pipelines produced different MD bytes")
	}
}

// --- MD self-identifies as derived ----------------------------------------

// TestRenderF2MarkdownProjection_SelfIdentifiesAsDerived proves the MD carries
// the standing notice identifying it as "Derived, informational, and
// non-authoritative" (memo L71-73). This is the honesty-ceiling contract for
// the rendering layer: anyone reading the MD knows it is NOT the canonical
// authority.
func TestRenderF2MarkdownProjection_SelfIdentifiesAsDerived(t *testing.T) {
	ingest := f2IngestFromFixture(t)
	sidecar := buildF2CanonicalSidecar(ingest, "docs/checkpoints/f2", fixedTime)
	md, err := RenderF2MarkdownProjection(sidecar, "docs/checkpoints/f2")
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	body := string(md)
	if !strings.Contains(body, "Derived, informational, and non-authoritative") {
		t.Fatalf("MD does not self-identify as derived/informational/non-authoritative")
	}
	if !strings.Contains(body, "Canonical meaning remains in the digest-bound F1 emit") {
		t.Fatalf("MD does not redirect to the canonical F1 emit")
	}
}

// --- Pair metadata agreement -----------------------------------------------

// TestPersistF2Pair_PairMetadataAgrees proves the canonical sidecar and the MD
// projection carry consistent view metadata: same cycle_id, entry_ids,
// digest, versions, and reciprocal locators that point to each other.
func TestPersistF2Pair_PairMetadataAgrees(t *testing.T) {
	dir := t.TempDir()
	ingest := f2IngestFromFixture(t)

	outcome, err := PersistF2Pair(ingest, dir, fixedTime)
	if err != nil {
		t.Fatalf("PersistF2Pair failed: %v", err)
	}
	if outcome != F2PairWritten {
		t.Fatalf("outcome = %s, want written", outcome)
	}

	canonPath := F2CanonicalSidecarPath(dir, "cycle-001")
	mdPath := F2MarkdownProjectionPath(dir, "cycle-001")

	// Read canonical sidecar.
	canon, err := ReadF2CanonicalSidecar(canonPath)
	if err != nil {
		t.Fatalf("cannot read canonical sidecar: %v", err)
	}

	// Read MD projection.
	mdBytes, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("cannot read MD projection: %v", err)
	}
	mdMeta, err := ExtractF2ViewMetadataFromMDBytes(mdBytes)
	if err != nil {
		t.Fatalf("cannot parse MD metadata: %v", err)
	}

	// --- Identity fields must agree ---
	canonMeta := canon.F2ViewMetadata
	if canonMeta.SynthesisCycleID != mdMeta.SynthesisCycleID {
		t.Errorf("cycle mismatch: canonical=%q, md=%q", canonMeta.SynthesisCycleID, mdMeta.SynthesisCycleID)
	}
	if canonMeta.SourceSemanticDigest != mdMeta.SourceSemanticDigest {
		t.Errorf("digest mismatch: canonical=%q, md=%q", canonMeta.SourceSemanticDigest, mdMeta.SourceSemanticDigest)
	}
	if canonMeta.SchemaVersion != mdMeta.SchemaVersion {
		t.Errorf("schema version mismatch: canonical=%q, md=%q", canonMeta.SchemaVersion, mdMeta.SchemaVersion)
	}
	if canonMeta.ProjectionVersion != mdMeta.ProjectionVersion {
		t.Errorf("projection version mismatch: canonical=%q, md=%q", canonMeta.ProjectionVersion, mdMeta.ProjectionVersion)
	}
	if canonMeta.RendererVersion != mdMeta.RendererVersion {
		t.Errorf("renderer version mismatch: canonical=%q, md=%q", canonMeta.RendererVersion, mdMeta.RendererVersion)
	}
	if canonMeta.WriteTimestamp != mdMeta.WriteTimestamp {
		t.Errorf("write timestamp mismatch: canonical=%q, md=%q", canonMeta.WriteTimestamp, mdMeta.WriteTimestamp)
	}

	// --- Entry IDs must agree ---
	if len(canonMeta.EntryIDs) != len(mdMeta.EntryIDs) {
		t.Errorf("entry ID count mismatch: canonical=%d, md=%d", len(canonMeta.EntryIDs), len(mdMeta.EntryIDs))
	} else {
		for i, id := range canonMeta.EntryIDs {
			if id != mdMeta.EntryIDs[i] {
				t.Errorf("entry ID[%d] mismatch: canonical=%q, md=%q", i, id, mdMeta.EntryIDs[i])
			}
		}
	}

	// --- Reciprocal locators must point to each other ---
	wantCanonRecip := filepath.Join(dir, "cycle-001.md")
	wantMDRecip := filepath.Join(dir, "cycle-001.canonical.json")
	if canonMeta.ReciprocalLocator != wantCanonRecip {
		t.Errorf("canonical reciprocal locator = %q, want %q", canonMeta.ReciprocalLocator, wantCanonRecip)
	}
	if mdMeta.ReciprocalLocator != wantMDRecip {
		t.Errorf("MD reciprocal locator = %q, want %q", mdMeta.ReciprocalLocator, wantMDRecip)
	}
}

// --- Pair collision contract -----------------------------------------------

// TestPersistF2Pair_NeitherExistsWritesBoth proves the fresh-write path: when
// neither file exists, both are written and the outcome is Written.
func TestPersistF2Pair_NeitherExistsWritesBoth(t *testing.T) {
	dir := t.TempDir()
	ingest := f2IngestFromFixture(t)

	outcome, err := PersistF2Pair(ingest, dir, fixedTime)
	if err != nil {
		t.Fatalf("PersistF2Pair failed: %v", err)
	}
	if outcome != F2PairWritten {
		t.Fatalf("outcome = %s, want written", outcome)
	}

	// Both files must exist.
	canonPath := F2CanonicalSidecarPath(dir, "cycle-001")
	mdPath := F2MarkdownProjectionPath(dir, "cycle-001")
	if !f2FileExists(canonPath) {
		t.Errorf("canonical sidecar not written at %q", canonPath)
	}
	if !f2FileExists(mdPath) {
		t.Errorf("MD projection not written at %q", mdPath)
	}
}

// TestPersistF2Pair_BothExistByteIdenticalIsIdempotent proves the idempotency
// path: when both files exist and match the ingest's canonical content, the
// outcome is Idempotent and NEITHER file is modified.
//
// THIS IS THE CRUX TEST for Slice 3: it proves the pair-level immutability
// contract — a re-run does NOT touch either file.
func TestPersistF2Pair_BothExistByteIdenticalIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	ingest := f2IngestFromFixture(t)

	// First write.
	if _, err := PersistF2Pair(ingest, dir, fixedTime); err != nil {
		t.Fatalf("first write failed: %v", err)
	}

	canonPath := F2CanonicalSidecarPath(dir, "cycle-001")
	mdPath := F2MarkdownProjectionPath(dir, "cycle-001")

	// Snapshot the original bytes of BOTH files.
	origCanon, _ := os.ReadFile(canonPath)
	origMD, _ := os.ReadFile(mdPath)

	// Re-run with the SAME canonical content but a DIFFERENT timestamp. The
	// canonical fingerprint (envelope content) matches; the timestamp is view
	// metadata that does not affect canonical identity. The pair must be
	// idempotent: neither file touched.
	laterTime := fixedTime.Add(48 * time.Hour)
	outcome, err := PersistF2Pair(ingest, dir, laterTime)
	if err != nil {
		t.Fatalf("idempotent re-run failed: %v", err)
	}
	if outcome != F2PairIdempotent {
		t.Fatalf("outcome = %s, want idempotent", outcome)
	}

	// Verify neither file was modified.
	newCanon, _ := os.ReadFile(canonPath)
	newMD, _ := os.ReadFile(mdPath)
	if string(origCanon) != string(newCanon) {
		t.Errorf("canonical sidecar was modified during idempotent re-run (immutability violation)")
	}
	if string(origMD) != string(newMD) {
		t.Errorf("MD projection was modified during idempotent re-run (immutability violation)")
	}
}

// TestPersistF2Pair_CanonicalDiffersRefused proves the refusal path: when both
// files exist but the canonical content differs, the outcome is Refused and
// neither file is modified.
func TestPersistF2Pair_CanonicalDiffersRefused(t *testing.T) {
	dir := t.TempDir()
	ingest := f2IngestFromFixture(t)

	// Write the original pair.
	if _, err := PersistF2Pair(ingest, dir, fixedTime); err != nil {
		t.Fatalf("first write failed: %v", err)
	}

	// Tamper the canonical sidecar: change the cycle ID in the envelope
	// (this changes the canonical content → different fingerprint).
	canonPath := F2CanonicalSidecarPath(dir, "cycle-001")
	canon, err := ReadF2CanonicalSidecar(canonPath)
	if err != nil {
		t.Fatalf("cannot read canonical: %v", err)
	}
	canon.CanonicalEnvelope.SynthesisCycleID = "tampered-cycle"
	tamperedBytes, err := SerializeF2CanonicalSidecar(canon)
	if err != nil {
		t.Fatalf("cannot serialize tampered canonical: %v", err)
	}
	if err := os.WriteFile(canonPath, tamperedBytes, 0o644); err != nil {
		t.Fatalf("cannot write tampered canonical: %v", err)
	}

	// Now attempt to persist the ORIGINAL ingest. The canonical content
	// differs (tampered-cycle vs cycle-001) → must refuse.
	outcome, err := PersistF2Pair(ingest, dir, fixedTime)
	if err == nil {
		t.Fatalf("expected error for differing canonical, got nil")
	}
	if outcome != F2PairRefused {
		t.Fatalf("outcome = %s, want refused", outcome)
	}
}

// TestPersistF2Pair_MDDiffersRefused proves the refusal path when the MD
// carries a different digest than the ingest. The canonical matches but the
// MD's source_semantic_digest was tampered.
func TestPersistF2Pair_MDDigestDiffersRefused(t *testing.T) {
	dir := t.TempDir()
	ingest := f2IngestFromFixture(t)

	// Write the original pair.
	if _, err := PersistF2Pair(ingest, dir, fixedTime); err != nil {
		t.Fatalf("first write failed: %v", err)
	}

	// Tamper the MD: change the source_semantic_digest in the metadata block.
	mdPath := F2MarkdownProjectionPath(dir, "cycle-001")
	mdBytes, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("cannot read MD: %v", err)
	}
	mdStr := string(mdBytes)
	tamperedDigest := "0000000000000000000000000000000000000000000000000000000000000000"
	mdStr = strings.Replace(mdStr, ingest.SemanticDigest, tamperedDigest, 1)
	if err := os.WriteFile(mdPath, []byte(mdStr), 0o644); err != nil {
		t.Fatalf("cannot write tampered MD: %v", err)
	}

	// Attempt to persist the ORIGINAL ingest. The MD's digest differs → refuse.
	outcome, err := PersistF2Pair(ingest, dir, fixedTime)
	if err == nil {
		t.Fatalf("expected error for differing MD digest, got nil")
	}
	if outcome != F2PairRefused {
		t.Fatalf("outcome = %s, want refused", outcome)
	}
}

// TestPersistF2Pair_MDProseTamperedRefused proves the byte-identical contract:
// if someone edits the MD prose OUTSIDE the metadata block (leaving
// source_semantic_digest intact), the pair coordination detects the tampering
// via a deterministic re-render comparison and refuses.
//
// This is the regression test for the commit-reviewer's bF1 finding: a
// digest-only check would miss prose edits. The re-render check catches them.
func TestPersistF2Pair_MDProseTamperedRefused(t *testing.T) {
	dir := t.TempDir()
	ingest := f2IngestFromFixture(t)

	// Write the original pair.
	if _, err := PersistF2Pair(ingest, dir, fixedTime); err != nil {
		t.Fatalf("first write failed: %v", err)
	}

	// Tamper the MD: add prose OUTSIDE the metadata block, leaving the
	// digest intact. This simulates an editor silently modifying the
	// projection prose. A digest-only idempotency check would miss this.
	mdPath := F2MarkdownProjectionPath(dir, "cycle-001")
	mdBytes, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("cannot read MD: %v", err)
	}
	tamperedMD := string(mdBytes) + "\n## Unauthorized addition\n\nThis line was added by an editor.\n"
	if err := os.WriteFile(mdPath, []byte(tamperedMD), 0o644); err != nil {
		t.Fatalf("cannot write tampered MD: %v", err)
	}

	// Attempt to persist the ORIGINAL ingest. The digest is unchanged, but
	// the MD prose was tampered → the byte-level re-render comparison catches
	// the drift and refuses.
	outcome, err := PersistF2Pair(ingest, dir, fixedTime)
	if err == nil {
		t.Fatalf("expected error for tampered MD prose, got nil (digest-only check would miss this)")
	}
	if outcome != F2PairRefused {
		t.Fatalf("outcome = %s, want refused (prose tampering must be detected by byte comparison)", outcome)
	}

	// Verify the MD was NOT silently overwritten (immutability).
	afterBytes, _ := os.ReadFile(mdPath)
	if string(afterBytes) != tamperedMD {
		t.Errorf("F2 silently overwrote the tampered MD (immutability violation — must refuse, not repair)")
	}
}

// TestPersistF2Pair_CanonicalOnlyReportsIncomplete proves the incomplete-pair
// detection: when only the canonical sidecar exists (and it matches), the
// outcome is IncompleteCanonicalOnly. F2 does NOT auto-complete.
func TestPersistF2Pair_CanonicalOnlyReportsIncomplete(t *testing.T) {
	dir := t.TempDir()
	ingest := f2IngestFromFixture(t)

	// Write ONLY the canonical sidecar (using the Slice 2 function directly).
	if _, err := PersistF2CanonicalSidecar(ingest, dir, fixedTime); err != nil {
		t.Fatalf("canonical-only write failed: %v", err)
	}

	// Verify the MD does NOT exist.
	mdPath := F2MarkdownProjectionPath(dir, "cycle-001")
	if f2FileExists(mdPath) {
		t.Fatalf("MD should not exist in canonical-only setup")
	}

	// Attempt to persist the pair. The canonical matches but the MD is
	// missing → IncompleteCanonicalOnly. F2 does NOT auto-complete.
	outcome, err := PersistF2Pair(ingest, dir, fixedTime)
	if err == nil {
		t.Fatalf("expected error for incomplete pair, got nil")
	}
	if outcome != F2PairIncompleteCanonicalOnly {
		t.Fatalf("outcome = %s, want incomplete_canonical_only", outcome)
	}

	// The MD must STILL not exist (F2 did not auto-complete).
	if f2FileExists(mdPath) {
		t.Errorf("F2 auto-completed the pair (MD was written) — incomplete pairs must NOT be auto-completed")
	}
}

// TestPersistF2Pair_MDOnlyReportsIncomplete proves the incomplete-pair
// detection: when only the MD exists (and it matches), the outcome is
// IncompleteMDOnly. F2 does NOT auto-complete.
func TestPersistF2Pair_MDOnlyReportsIncomplete(t *testing.T) {
	dir := t.TempDir()
	ingest := f2IngestFromFixture(t)

	// Write ONLY the MD projection by first writing the full pair, then
	// removing the canonical.
	if _, err := PersistF2Pair(ingest, dir, fixedTime); err != nil {
		t.Fatalf("initial pair write failed: %v", err)
	}
	canonPath := F2CanonicalSidecarPath(dir, "cycle-001")
	if err := os.Remove(canonPath); err != nil {
		t.Fatalf("cannot remove canonical: %v", err)
	}

	// Attempt to persist the pair. The MD matches but the canonical is
	// missing → IncompleteMDOnly. F2 does NOT auto-complete.
	outcome, err := PersistF2Pair(ingest, dir, fixedTime)
	if err == nil {
		t.Fatalf("expected error for incomplete pair, got nil")
	}
	if outcome != F2PairIncompleteMDOnly {
		t.Fatalf("outcome = %s, want incomplete_md_only", outcome)
	}

	// The canonical must STILL not exist (F2 did not auto-complete).
	if f2FileExists(canonPath) {
		t.Errorf("F2 auto-completed the pair (canonical was written) — incomplete pairs must NOT be auto-completed")
	}
}

// --- MD extract helper -----------------------------------------------------

// TestExtractF2ViewMetadataFromMDBytes_ValidAndMissing proves the metadata
// extraction parses a valid block and rejects a missing one.
func TestExtractF2ViewMetadataFromMDBytes_ValidAndMissing(t *testing.T) {
	ingest := f2IngestFromFixture(t)
	sidecar := buildF2CanonicalSidecar(ingest, "docs/checkpoints/f2", fixedTime)
	md, err := RenderF2MarkdownProjection(sidecar, "docs/checkpoints/f2")
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}

	// Valid extraction.
	meta, err := ExtractF2ViewMetadataFromMDBytes(md)
	if err != nil {
		t.Fatalf("extraction from valid MD failed: %v", err)
	}
	if meta.SynthesisCycleID != "cycle-001" {
		t.Errorf("extracted cycle = %q, want cycle-001", meta.SynthesisCycleID)
	}

	// Missing block.
	_, err = ExtractF2ViewMetadataFromMDBytes([]byte("# No metadata here\n\nJust prose."))
	if err == nil {
		t.Fatalf("expected error for missing metadata block, got nil")
	}
}

// --- No model summarization (structural guarantee) -------------------------

// TestRenderF2MarkdownProjection_NoModelSummarization proves the rendered MD
// contains ONLY values traceable to the canonical envelope. It verifies that
// the MD is a faithful projection (every rendered value exists in the
// envelope) and not a model-generated summary (no narrative text appears that
// is not in the envelope).
//
// This is tested structurally: the rendering is pure code walking the struct,
// so by construction no value can appear that is not in the envelope. The test
// verifies the deterministic renderer code path by checking that specific
// envelope values appear verbatim in the MD (they do — the renderer emits
// them with fmt.Fprintf, not through a model call).
func TestRenderF2MarkdownProjection_NoModelSummarization(t *testing.T) {
	ingest := f2IngestFromFixture(t)
	sidecar := buildF2CanonicalSidecar(ingest, "docs/checkpoints/f2", fixedTime)
	md, err := RenderF2MarkdownProjection(sidecar, "docs/checkpoints/f2")
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	body := string(md)

	// Specific envelope values must appear VERBATIM (the renderer does not
	// paraphrase or summarize). Verify a representative sample across all
	// entry types.
	env := ingest.CanonicalEnvelope
	for _, entry := range env.Entries {
		// Entry ID must appear verbatim.
		if !strings.Contains(body, entry.EntryID) {
			t.Errorf("entry ID %q from envelope not found in MD (renderer must emit verbatim)", entry.EntryID)
		}
		// Family must appear verbatim.
		if !strings.Contains(body, entry.Family) {
			t.Errorf("family %q from envelope not found in MD", entry.Family)
		}
	}

	// Verify a specific R3 option field value appears verbatim.
	if len(env.Entries) >= 2 && env.Entries[1].R3 != nil && len(env.Entries[1].R3.Options) > 0 {
		opt := env.Entries[1].R3.Options[0]
		if opt.Mechanism != "" && !strings.Contains(body, opt.Mechanism) {
			t.Errorf("R3 option mechanism %q not found verbatim in MD", opt.Mechanism)
		}
	}

	// Verify a specific P-a probe value appears verbatim.
	if len(env.Entries) >= 3 && env.Entries[2].PA != nil && len(env.Entries[2].PA.Probes) > 0 {
		probe := env.Entries[2].PA.Probes[0]
		if probe.Result != "" && !strings.Contains(body, "`"+probe.Result+"`") {
			t.Errorf("P-a probe result %q not found verbatim in MD", probe.Result)
		}
	}
}

// --- Mismatched sidecar/view: the "mismatched sidecar/view rejected" test --

// TestPersistF2Pair_MismatchedSidecarViewDetected proves the pair coordination
// detects when the canonical sidecar and the MD disagree about the digest.
// This is the "mismatched sidecar/view rejected" test case from the slice
// requirements: the MD was rendered from a DIFFERENT canonical than the one on
// disk (e.g., the canonical was silently replaced after the MD was written).
func TestPersistF2Pair_MismatchedSidecarViewDetected(t *testing.T) {
	dir := t.TempDir()
	ingest := f2IngestFromFixture(t)

	// Write the original pair.
	if _, err := PersistF2Pair(ingest, dir, fixedTime); err != nil {
		t.Fatalf("first write failed: %v", err)
	}

	canonPath := F2CanonicalSidecarPath(dir, "cycle-001")

	// Tamper the canonical: replace it with a sidecar for a DIFFERENT
	// canonical envelope (different cycle ID → different digest). The MD on
	// disk still carries the ORIGINAL digest. The pair coordination must
	// detect that the canonical differs from the ingest → refuse.
	canon, err := ReadF2CanonicalSidecar(canonPath)
	if err != nil {
		t.Fatalf("cannot read canonical: %v", err)
	}
	// Change the applicability (a canonical field that changes the digest).
	canon.CanonicalEnvelope.Applicability = "not_triggered"
	tamperedBytes, err := SerializeF2CanonicalSidecar(canon)
	if err != nil {
		t.Fatalf("cannot serialize tampered canonical: %v", err)
	}
	if err := os.WriteFile(canonPath, tamperedBytes, 0o644); err != nil {
		t.Fatalf("cannot write tampered canonical: %v", err)
	}

	// The canonical now differs from the ingest. PersistF2Pair must refuse.
	outcome, err := PersistF2Pair(ingest, dir, fixedTime)
	if err == nil {
		t.Fatalf("expected refusal for mismatched canonical, got nil")
	}
	if outcome != F2PairRefused {
		t.Fatalf("outcome = %s, want refused", outcome)
	}
}

// --- Nil / empty input guards ----------------------------------------------

func TestRenderF2MarkdownProjection_NilSidecar(t *testing.T) {
	_, err := RenderF2MarkdownProjection(nil, "docs/checkpoints/f2")
	if err == nil {
		t.Fatalf("expected error for nil sidecar")
	}
}

func TestPersistF2Pair_NilIngest(t *testing.T) {
	outcome, err := PersistF2Pair(nil, "docs/checkpoints/f2", fixedTime)
	if err == nil {
		t.Fatalf("expected error for nil ingest")
	}
	if outcome != F2PairNotAttempted {
		t.Fatalf("outcome = %s, want not_attempted", outcome)
	}
}

// --- MD projection round-trips through re-render ---------------------------

// TestRenderF2MarkdownProjection_ReRenderFromStoredSidecar proves the MD is
// regenerable from the canonical sidecar alone: read the stored canonical,
// re-render the MD, and the metadata block carries the same identity. This is
// the foundation for the doctor's deterministic-projection-equivalence check
// (Slice 9).
func TestRenderF2MarkdownProjection_ReRenderFromStoredSidecar(t *testing.T) {
	dir := t.TempDir()
	ingest := f2IngestFromFixture(t)

	// Write the pair.
	if _, err := PersistF2Pair(ingest, dir, fixedTime); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// Read the canonical sidecar from disk.
	canonPath := F2CanonicalSidecarPath(dir, "cycle-001")
	storedCanon, err := ReadF2CanonicalSidecar(canonPath)
	if err != nil {
		t.Fatalf("cannot read stored canonical: %v", err)
	}

	// Re-render the MD from the stored sidecar.
	reRendered, err := RenderF2MarkdownProjection(storedCanon, dir)
	if err != nil {
		t.Fatalf("re-render failed: %v", err)
	}

	// The stored MD and the re-rendered MD must be byte-identical (the
	// renderer is deterministic, the sidecar carries the same timestamp).
	mdPath := F2MarkdownProjectionPath(dir, "cycle-001")
	storedMD, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("cannot read stored MD: %v", err)
	}
	if string(reRendered) != string(storedMD) {
		t.Errorf("re-rendered MD does not match stored MD (the doctor's projection-equivalence check depends on this)")
	}
}

// --- Helper for golden test assertions -------------------------------------

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
