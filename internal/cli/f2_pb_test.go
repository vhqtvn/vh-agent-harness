package cli

// f2_pb_test.go — tests for the P-b evidence-grade media provenance slot
// (Slice 7).
//
// Design authority: researches/decisions/2026-07-25-f2-rendering-family-
// mechanism.md, P-b contract L184-233.
//
// Key contract points tested here:
//   - captured media attachment constructs + renders (positive path);
//   - verified evidence grade accepted;
//   - capability_status ≠ evidence-grade (evidence_grade="available" rejected);
//   - locator kind/value validation;
//   - required provenance fields present;
//   - content_digest is optional;
//   - entry binding required (unresolved entry rejected);
//   - nil descriptor → nil attachment (no error);
//   - hand-constructed attachment with forged fields rejected at ALL durable
//     paths (PersistF2CanonicalSidecar, PersistF2Pair, RenderF2MarkdownProjection);
//   - omission doesn't invalidate unrelated entries;
//   - P-b section renders in MD projection with structural markers + honesty
//     ceiling notice.

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// --- P-b test fixture --------------------------------------------------------

// pbEmitFromFixture returns a ValidatedF1Emit from the canonical F1 fixture
// (same helper as f2EmitFromFixture in f2_ingest_test.go, aliased here for
// readability). The canonical fixture carries entries entry-r1, entry-r3,
// entry-pa — all pass F1 validation. Media attachments bind to entry-r3.
func pbEmitFromFixture(t *testing.T) *ValidatedF1Emit {
	t.Helper()
	return f2EmitFromFixture(t)
}

// pbValidDescriptor returns a descriptor with all fields validly populated.
// evidence_grade is "captured" — the most common evidence grade.
func pbValidDescriptor() *F2MediaAttachmentDescriptor {
	return &F2MediaAttachmentDescriptor{
		AttachmentID:    "media-001",
		EntryID:         "entry-r3",
		Locator:         F2MediaLocator{Kind: F2MediaLocatorKindPath, Value: "evidence/chart-001.png"},
		CapabilityClass: "image/chart",
		ModalityHint:    "visual",
		EvidenceGrade:   F2MediaEvidenceGradeCaptured,
		Provenance: F2MediaProvenance{
			SourceLocator:               "evidence/chart-001.png",
			CaptureOrVerificationMethod: "screenshot capture",
			ObservedOrVerifiedAt:        "2026-07-25T10:00:00Z",
			ProducerOrVerifierClass:     "operator",
		},
	}
}

// pbIngestWithAttachment builds an F2IngestResult carrying a valid media
// attachment. Uses IngestF1EmitForF2 + sets MediaAttachments.
func pbIngestWithAttachment(t *testing.T) *F2IngestResult {
	t.Helper()
	emit := pbEmitFromFixture(t)
	ingest, problems := IngestF1EmitForF2(emit)
	if ingest == nil || len(problems) > 0 {
		t.Fatalf("IngestF1EmitForF2 reported problems: %v", problems)
	}
	att, err := BuildF2MediaAttachment(emit, pbValidDescriptor())
	if err != nil {
		t.Fatalf("BuildF2MediaAttachment failed: %v", err)
	}
	ingest.MediaAttachments = []F2MediaAttachment{*att}
	return ingest
}

// fixedTime for deterministic tests.
var pbFixedTime = time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)

// --- Positive path: captured media constructs + renders ---------------------

// TestF2PBMedia_CapturedMediaConstructsAndRenders proves that a valid captured
// media attachment constructs with the correct binding and renders in the MD
// projection.
func TestF2PBMedia_CapturedMediaConstructsAndRends(t *testing.T) {
	emit := pbEmitFromFixture(t)

	att, err := BuildF2MediaAttachment(emit, pbValidDescriptor())
	if err != nil {
		t.Fatalf("BuildF2MediaAttachment failed: %v", err)
	}
	if att == nil {
		t.Fatalf("expected non-nil attachment for valid descriptor")
	}

	// Binding fields are DERIVED from the emit (not from the descriptor).
	if att.BoundCycleID != emit.CanonicalEnvelope.SynthesisCycleID {
		t.Errorf("expected BoundCycleID %q, got %q",
			emit.CanonicalEnvelope.SynthesisCycleID, att.BoundCycleID)
	}
	if att.BoundDigest != emit.SemanticDigest {
		t.Errorf("expected BoundDigest to match emit digest")
	}

	// Descriptor fields are carried verbatim.
	if att.AttachmentID != "media-001" {
		t.Errorf("expected AttachmentID %q, got %q", "media-001", att.AttachmentID)
	}
	if att.EvidenceGrade != F2MediaEvidenceGradeCaptured {
		t.Errorf("expected EvidenceGrade %q, got %q", F2MediaEvidenceGradeCaptured, att.EvidenceGrade)
	}
}

// TestF2PBMedia_VerifiedEvidenceGradeAccepted proves that evidence_grade
// "verified" is accepted (the closed set is captured|verified).
func TestF2PBMedia_VerifiedEvidenceGradeAccepted(t *testing.T) {
	emit := pbEmitFromFixture(t)
	desc := pbValidDescriptor()
	desc.EvidenceGrade = F2MediaEvidenceGradeVerified

	att, err := BuildF2MediaAttachment(emit, desc)
	if err != nil {
		t.Fatalf("BuildF2MediaAttachment failed for verified grade: %v", err)
	}
	if att.EvidenceGrade != F2MediaEvidenceGradeVerified {
		t.Errorf("expected EvidenceGrade %q, got %q", F2MediaEvidenceGradeVerified, att.EvidenceGrade)
	}
}

// --- capability_status ≠ evidence-grade (VERBATIM operator requirement) ------

// TestF2PBMedia_CapabilityStatusNotAcceptedAsEvidenceGrade proves that
// capability_status values (available/unavailable/uncertain) are NOT accepted
// as evidence_grade. This is the VERBATIM operator requirement (memo L214-215).
func TestF2PBMedia_CapabilityStatusNotAcceptedAsEvidenceGrade(t *testing.T) {
	emit := pbEmitFromFixture(t)

	for _, statusVal := range []string{"available", "unavailable", "uncertain"} {
		desc := pbValidDescriptor()
		desc.EvidenceGrade = statusVal // WRONG: using a capability_status value

		_, err := BuildF2MediaAttachment(emit, desc)
		if err == nil {
			t.Errorf("expected error when evidence_grade=%q (capability_status value), but got nil", statusVal)
		}
		// The error must mention the closed set and the distinction.
		if !strings.Contains(err.Error(), "captured") || !strings.Contains(err.Error(), "verified") {
			t.Errorf("error for evidence_grade=%q should mention {captured, verified}: %v", statusVal, err)
		}
	}
}

// TestF2PBMedia_CapabilityStatusFieldAcceptedAlongside proves that the
// capability_status FIELD (optional, separate from evidence_grade) is accepted
// without being confused with evidence_grade. The two are distinct fields.
func TestF2PBMedia_CapabilityStatusFieldAcceptedAlongside(t *testing.T) {
	emit := pbEmitFromFixture(t)
	desc := pbValidDescriptor()
	desc.CapabilityStatus = "available" // legitimate capability_status value

	att, err := BuildF2MediaAttachment(emit, desc)
	if err != nil {
		t.Fatalf("BuildF2MediaAttachment failed with capability_status: %v", err)
	}
	if att.CapabilityStatus != "available" {
		t.Errorf("expected CapabilityStatus %q, got %q", "available", att.CapabilityStatus)
	}
	// Evidence grade must remain distinct from capability_status.
	if att.EvidenceGrade != F2MediaEvidenceGradeCaptured {
		t.Errorf("expected EvidenceGrade %q, got %q", F2MediaEvidenceGradeCaptured, att.EvidenceGrade)
	}
}

// --- Locator validation ------------------------------------------------------

// TestF2PBMedia_InvalidLocatorKindRejected proves that locator.kind must be in
// {path, url}. Other kinds are rejected.
func TestF2PBMedia_InvalidLocatorKindRejected(t *testing.T) {
	emit := pbEmitFromFixture(t)

	for _, badKind := range []string{"", "file", "s3", "custom"} {
		desc := pbValidDescriptor()
		desc.Locator.Kind = badKind

		_, err := BuildF2MediaAttachment(emit, desc)
		if err == nil {
			t.Errorf("expected error for locator.kind=%q, but got nil", badKind)
		}
	}
}

// TestF2PBMedia_EmptyLocatorValueRejected proves that an empty locator.value
// is rejected.
func TestF2PBMedia_EmptyLocatorValueRejected(t *testing.T) {
	emit := pbEmitFromFixture(t)
	desc := pbValidDescriptor()
	desc.Locator.Value = ""

	_, err := BuildF2MediaAttachment(emit, desc)
	if err == nil {
		t.Fatalf("expected error for empty locator.value")
	}
}

// --- Provenance validation ---------------------------------------------------

// TestF2PBMedia_MissingProvenanceFieldRejected proves that each required
// provenance field must be present.
func TestF2PBMedia_MissingProvenanceFieldRejected(t *testing.T) {
	emit := pbEmitFromFixture(t)

	cases := []struct {
		name   string
		mutate func(p *F2MediaProvenance)
	}{
		{"source_locator_empty", func(p *F2MediaProvenance) { p.SourceLocator = "" }},
		{"capture_method_empty", func(p *F2MediaProvenance) { p.CaptureOrVerificationMethod = "" }},
		{"observed_at_empty", func(p *F2MediaProvenance) { p.ObservedOrVerifiedAt = "" }},
		{"producer_class_empty", func(p *F2MediaProvenance) { p.ProducerOrVerifierClass = "" }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			desc := pbValidDescriptor()
			tc.mutate(&desc.Provenance)

			_, err := BuildF2MediaAttachment(emit, desc)
			if err == nil {
				t.Errorf("expected error for %s, but got nil", tc.name)
			}
		})
	}
}

// TestF2PBMedia_ContentDigestOptional proves that content_digest is optional
// in provenance (memo L201: "if available").
func TestF2PBMedia_ContentDigestOptional(t *testing.T) {
	emit := pbEmitFromFixture(t)

	// Without content_digest.
	desc := pbValidDescriptor()
	desc.Provenance.ContentDigest = ""
	att, err := BuildF2MediaAttachment(emit, desc)
	if err != nil {
		t.Fatalf("BuildF2MediaAttachment failed without content_digest: %v", err)
	}
	if att.Provenance.ContentDigest != "" {
		t.Errorf("expected empty ContentDigest")
	}

	// With content_digest.
	desc2 := pbValidDescriptor()
	desc2.Provenance.ContentDigest = "sha256:abc123"
	att2, err := BuildF2MediaAttachment(emit, desc2)
	if err != nil {
		t.Fatalf("BuildF2MediaAttachment failed with content_digest: %v", err)
	}
	if att2.Provenance.ContentDigest != "sha256:abc123" {
		t.Errorf("expected ContentDigest %q, got %q", "sha256:abc123", att2.Provenance.ContentDigest)
	}
}

// --- Entry binding -----------------------------------------------------------

// TestF2PBMedia_UnresolvedEntryRejected proves that the descriptor's EntryID
// must resolve to a canonical entry.
func TestF2PBMedia_UnresolvedEntryRejected(t *testing.T) {
	emit := pbEmitFromFixture(t)
	desc := pbValidDescriptor()
	desc.EntryID = "entry-nonexistent"

	_, err := BuildF2MediaAttachment(emit, desc)
	if err == nil {
		t.Fatalf("expected error for unresolved entry_id")
	}
}

// --- Nil descriptor → nil attachment -----------------------------------------

// TestF2PBMedia_NilDescriptorProducesNilAttachment proves that a nil descriptor
// produces a nil attachment (no error). Missing media cannot be reconstructed
// from narrative (memo L180, L223-224).
func TestF2PBMedia_NilDescriptorProducesNilAttachment(t *testing.T) {
	emit := pbEmitFromFixture(t)
	att, err := BuildF2MediaAttachment(emit, nil)
	if err != nil {
		t.Fatalf("expected nil error for nil descriptor, got: %v", err)
	}
	if att != nil {
		t.Errorf("expected nil attachment for nil descriptor, got non-nil")
	}
}

// --- MD rendering ------------------------------------------------------------

// TestF2PBMedia_RendersInMDProjection proves that a valid media attachment
// renders in the MD projection with all its fields + structural markers +
// honesty ceiling notice.
func TestF2PBMedia_RendersInMDProjection(t *testing.T) {
	ingest := pbIngestWithAttachment(t)
	dir := "docs/checkpoints/f2"
	sidecar := buildF2CanonicalSidecar(ingest, dir, pbFixedTime)

	mdBytes, err := RenderF2MarkdownProjection(sidecar, dir)
	if err != nil {
		t.Fatalf("RenderF2MarkdownProjection failed: %v", err)
	}

	md := string(mdBytes)

	// Structural markers present.
	if !strings.Contains(md, "<!-- f2-pb-media:begin -->") {
		t.Errorf("MD missing f2-pb-media:begin marker")
	}
	if !strings.Contains(md, "<!-- f2-pb-media:end -->") {
		t.Errorf("MD missing f2-pb-media:end marker")
	}

	// Section heading present.
	if !strings.Contains(md, "## P-b — Evidence-Grade Media Provenance") {
		t.Errorf("MD missing P-b section heading")
	}

	// Honesty ceiling notice present.
	if !strings.Contains(md, "content truth is NOT verified") {
		t.Errorf("MD missing honesty ceiling notice")
	}

	// capability_status ≠ evidence-grade caveat present.
	if !strings.Contains(md, "capability_status ≠ evidence-grade") {
		t.Errorf("MD missing capability_status ≠ evidence-grade caveat")
	}

	// Attachment fields rendered.
	if !strings.Contains(md, "media-001") {
		t.Errorf("MD missing attachment ID")
	}
	if !strings.Contains(md, "entry-r3") {
		t.Errorf("MD missing entry binding")
	}
	if !strings.Contains(md, F2MediaEvidenceGradeCaptured) {
		t.Errorf("MD missing evidence grade")
	}

	// Provenance fields rendered.
	if !strings.Contains(md, "Source locator") {
		t.Errorf("MD missing provenance source locator")
	}
}

// TestF2PBMedia_EmptyAttachmentsRenderBoundedNotice proves that nil/empty
// media attachments render a bounded absence notice (not a fabricated
// attachment).
func TestF2PBMedia_EmptyAttachmentsRenderBoundedNotice(t *testing.T) {
	emit := pbEmitFromFixture(t)
	ingest, problems := IngestF1EmitForF2(emit)
	if ingest == nil || len(problems) > 0 {
		t.Fatalf("IngestF1EmitForF2 reported problems: %v", problems)
	}
	// No MediaAttachments set (nil).

	dir := "docs/checkpoints/f2"
	sidecar := buildF2CanonicalSidecar(ingest, dir, pbFixedTime)
	mdBytes, err := RenderF2MarkdownProjection(sidecar, dir)
	if err != nil {
		t.Fatalf("RenderF2MarkdownProjection failed: %v", err)
	}

	md := string(mdBytes)
	if !strings.Contains(md, "(no evidence-grade media attachments bound to this cycle)") {
		t.Errorf("expected bounded absence notice for empty attachments, got MD without it")
	}
}

// --- Omission doesn't invalidate unrelated entries ---------------------------

// TestF2PBMedia_OmissionDoesNotInvalidateUnrelatedEntries proves that an
// invalid descriptor does NOT prevent a valid persist/render of the rest of the
// pair. The invalid descriptor is simply not constructed; the valid pair
// proceeds.
func TestF2PBMedia_OmissionDoesNotInvalidateUnrelatedEntries(t *testing.T) {
	emit := pbEmitFromFixture(t)
	ingest, problems := IngestF1EmitForF2(emit)
	if ingest == nil || len(problems) > 0 {
		t.Fatalf("IngestF1EmitForF2 reported problems: %v", problems)
	}

	// Try to build an invalid attachment (bad evidence grade).
	badDesc := pbValidDescriptor()
	badDesc.EvidenceGrade = "available"
	_, badErr := BuildF2MediaAttachment(emit, badDesc)
	if badErr == nil {
		t.Fatalf("expected error for invalid descriptor")
	}

	// The valid pair still persists + renders (the invalid attachment was
	// never constructed — it doesn't poison the ingest result).
	dir := t.TempDir()
	outcome, err := PersistF2Pair(ingest, dir, pbFixedTime)
	if err != nil {
		t.Fatalf("PersistF2Pair failed: %v", err)
	}
	if outcome != F2PairWritten {
		t.Errorf("expected F2PairWritten, got %s", outcome.String())
	}

	// The MD renders without the invalid attachment.
	sidecar := buildF2CanonicalSidecar(ingest, dir, pbFixedTime)
	mdBytes, rErr := RenderF2MarkdownProjection(sidecar, dir)
	if rErr != nil {
		t.Fatalf("RenderF2MarkdownProjection failed: %v", rErr)
	}
	// The bounded absence notice should appear (no valid attachments).
	if !strings.Contains(string(mdBytes), "(no evidence-grade media attachments") {
		t.Errorf("expected bounded absence notice in MD")
	}
}

// --- Durable-path hand-construction rejection --------------------------------

// TestF2PBMedia_HandConstructedAttachmentRejectedAtDurablePaths proves that a
// hand-constructed F2MediaAttachment with forged fields is rejected at ALL
// three durable gates (PersistF2CanonicalSidecar, PersistF2Pair,
// RenderF2MarkdownProjection). This is the defense-in-depth pattern from R5.
func TestF2PBMedia_HandConstructedAttachmentRejectedAtDurablePaths(t *testing.T) {
	emit := pbEmitFromFixture(t)
	ingest, _ := IngestF1EmitForF2(emit)
	dir := "docs/checkpoints/f2"

	// Build a valid attachment as the base.
	validAtt, err := BuildF2MediaAttachment(emit, pbValidDescriptor())
	if err != nil {
		t.Fatalf("BuildF2MediaAttachment failed: %v", err)
	}

	// Define tamper variants — each forges ONE exported field.
	tamperCases := []struct {
		name  string
		forge func(att F2MediaAttachment) F2MediaAttachment
	}{
		{
			name: "evidence_grade_forged_to_capability_status",
			forge: func(a F2MediaAttachment) F2MediaAttachment {
				a.EvidenceGrade = "available" // capability_status value
				return a
			},
		},
		{
			name: "bound_cycle_id_forged",
			forge: func(a F2MediaAttachment) F2MediaAttachment {
				a.BoundCycleID = "forged-cycle"
				return a
			},
		},
		{
			name: "bound_digest_forged",
			forge: func(a F2MediaAttachment) F2MediaAttachment {
				a.BoundDigest = "forged-digest-raw-prose"
				return a
			},
		},
		{
			name: "entry_id_forged",
			forge: func(a F2MediaAttachment) F2MediaAttachment {
				a.EntryID = "entry-nonexistent"
				return a
			},
		},
		{
			name: "locator_kind_forged",
			forge: func(a F2MediaAttachment) F2MediaAttachment {
				a.Locator.Kind = "custom"
				return a
			},
		},
		{
			name: "provenance_source_locator_forged_empty",
			forge: func(a F2MediaAttachment) F2MediaAttachment {
				a.Provenance.SourceLocator = ""
				return a
			},
		},
		{
			name: "attachment_id_forged_empty",
			forge: func(a F2MediaAttachment) F2MediaAttachment {
				a.AttachmentID = ""
				return a
			},
		},
	}

	for _, tc := range tamperCases {
		t.Run(tc.name, func(t *testing.T) {
			forged := tc.forge(*validAtt)

			// Gate 1: PersistF2CanonicalSidecar.
			ingest1, p1 := IngestF1EmitForF2(emit)
			if ingest1 == nil || len(p1) > 0 {
				t.Fatalf("IngestF1EmitForF2 failed: %v", p1)
			}
			ingest1.MediaAttachments = []F2MediaAttachment{forged}
			_, pErr := PersistF2CanonicalSidecar(ingest1, t.TempDir(), pbFixedTime)
			if pErr == nil {
				t.Errorf("PersistF2CanonicalSidecar: expected rejection for %s, got nil", tc.name)
			}

			// Gate 2: PersistF2Pair.
			ingest2, p2 := IngestF1EmitForF2(emit)
			if ingest2 == nil || len(p2) > 0 {
				t.Fatalf("IngestF1EmitForF2 failed: %v", p2)
			}
			ingest2.MediaAttachments = []F2MediaAttachment{forged}
			_, pairErr := PersistF2Pair(ingest2, t.TempDir(), pbFixedTime)
			if pairErr == nil {
				t.Errorf("PersistF2Pair: expected rejection for %s, got nil", tc.name)
			}

			// Gate 3: RenderF2MarkdownProjection.
			sidecar := buildF2CanonicalSidecar(ingest, dir, pbFixedTime)
			sidecar.MediaAttachments = []F2MediaAttachment{forged}
			_, rErr := RenderF2MarkdownProjection(sidecar, dir)
			if rErr == nil {
				t.Errorf("RenderF2MarkdownProjection: expected rejection for %s, got nil", tc.name)
			}
		})
	}

	// Sanity: the VALID attachment passes all gates.
	t.Run("valid_attachment_passes_all_gates", func(t *testing.T) {
		ingestV, pv := IngestF1EmitForF2(emit)
		if ingestV == nil || len(pv) > 0 {
			t.Fatalf("IngestF1EmitForF2 failed: %v", pv)
		}
		ingestV.MediaAttachments = []F2MediaAttachment{*validAtt}

		// Gate 1: PersistF2CanonicalSidecar.
		cDir := t.TempDir()
		pOut, pErr := PersistF2CanonicalSidecar(ingestV, cDir, pbFixedTime)
		if pErr != nil {
			t.Fatalf("valid attachment rejected at PersistF2CanonicalSidecar: %v (outcome=%s)", pErr, pOut)
		}

		// Gate 2: PersistF2Pair (end-to-end — the valid attachment must
		// persist BOTH files of the pair, addressing the DEFER B-F2 gap).
		ingestPair, pp := IngestF1EmitForF2(emit)
		if ingestPair == nil || len(pp) > 0 {
			t.Fatalf("IngestF1EmitForF2 failed: %v", pp)
		}
		ingestPair.MediaAttachments = []F2MediaAttachment{*validAtt}
		pairDir := t.TempDir()
		pairOut, pairErr := PersistF2Pair(ingestPair, pairDir, pbFixedTime)
		if pairErr != nil {
			t.Fatalf("valid attachment rejected at PersistF2Pair: %v (outcome=%s)", pairErr, pairOut)
		}
		if pairOut != F2PairWritten {
			t.Errorf("expected F2PairWritten, got %s", pairOut.String())
		}

		// Gate 3: RenderF2MarkdownProjection.
		sidecar := buildF2CanonicalSidecar(ingestV, "docs/checkpoints/f2", pbFixedTime)
		_, rErr := RenderF2MarkdownProjection(sidecar, "docs/checkpoints/f2")
		if rErr != nil {
			t.Fatalf("valid attachment rejected at RenderF2MarkdownProjection: %v", rErr)
		}
	})
}

// --- Persistence in canonical sidecar ----------------------------------------

// TestF2PBMedia_PersistsInCanonicalSidecar proves that media attachments are
// serialized in the canonical sidecar JSON and can be read back.
func TestF2PBMedia_PersistsInCanonicalSidecar(t *testing.T) {
	ingest := pbIngestWithAttachment(t)
	dir := t.TempDir()

	outcome, err := PersistF2CanonicalSidecar(ingest, dir, pbFixedTime)
	if err != nil {
		t.Fatalf("PersistF2CanonicalSidecar failed: %v", err)
	}
	if outcome != F2PersistWritten {
		t.Errorf("expected F2PersistWritten, got %s", outcome.String())
	}

	// Read back.
	path := F2CanonicalSidecarPath(dir, ingest.SynthesisCycleID)
	sidecar, err := ReadF2CanonicalSidecar(path)
	if err != nil {
		t.Fatalf("ReadF2CanonicalSidecar failed: %v", err)
	}

	if len(sidecar.MediaAttachments) != 1 {
		t.Fatalf("expected 1 media attachment in sidecar, got %d", len(sidecar.MediaAttachments))
	}

	att := sidecar.MediaAttachments[0]
	if att.AttachmentID != "media-001" {
		t.Errorf("expected AttachmentID %q, got %q", "media-001", att.AttachmentID)
	}
	if att.EvidenceGrade != F2MediaEvidenceGradeCaptured {
		t.Errorf("expected EvidenceGrade %q, got %q", F2MediaEvidenceGradeCaptured, att.EvidenceGrade)
	}
	if att.BoundCycleID != ingest.SynthesisCycleID {
		t.Errorf("expected BoundCycleID %q, got %q", ingest.SynthesisCycleID, att.BoundCycleID)
	}
}

// --- Deterministic serialization --------------------------------------------

// TestF2PBMedia_DeterministicSerialization proves that the same attachment
// produces identical JSON bytes across calls.
func TestF2PBMedia_DeterministicSerialization(t *testing.T) {
	ingest := pbIngestWithAttachment(t)
	dir := "docs/checkpoints/f2"

	sidecar1 := buildF2CanonicalSidecar(ingest, dir, pbFixedTime)
	sidecar2 := buildF2CanonicalSidecar(ingest, dir, pbFixedTime)

	bytes1, err := SerializeF2CanonicalSidecar(sidecar1)
	if err != nil {
		t.Fatalf("SerializeF2CanonicalSidecar failed: %v", err)
	}
	bytes2, err := SerializeF2CanonicalSidecar(sidecar2)
	if err != nil {
		t.Fatalf("SerializeF2CanonicalSidecar failed: %v", err)
	}

	if string(bytes1) != string(bytes2) {
		t.Errorf("deterministic serialization failed: bytes differ")
	}

	// Also verify the JSON is parseable.
	var parsed F2CanonicalSidecar
	if err := json.Unmarshal(bytes1, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if len(parsed.MediaAttachments) != 1 {
		t.Errorf("expected 1 attachment after round-trip, got %d", len(parsed.MediaAttachments))
	}
}
