package cli

// doctor_f2_test.go — tests for the F2 pair structural-consistency audit
// (doctor check #18).
//
// Covers the memo L362-382 contract: pair presence, digest recompute,
// reciprocal binding, pair metadata agreement, deterministic projection
// equivalence, P-c required structure, P-b provenance presence, P-a enum
// validity, R5 binding consistency.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// f2DoctorFixture writes a VALID F2 pair (canonical.json + .md) to a temp dir
// using PRODUCTION-SHAPE repo-relative locators (logicalF2Dir, not the
// absolute temp dir) and returns the temp-root target + the cycle base name.
// Tests mutate one aspect at a time to produce a specific failure, then call
// checkF2PairConsistency(target).
func f2DoctorFixture(t *testing.T) (target, base string) {
	t.Helper()
	tmp := t.TempDir()
	f2Dir := filepath.Join(tmp, logicalF2Dir)
	if err := os.MkdirAll(f2Dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	ingest := f2IngestFromFixture(t)
	// Build sidecar with the REPO-RELATIVE logical dir (production shape) so
	// ReciprocalLocator values are repo-relative, matching what production
	// PersistF2Pair(..., "docs/checkpoints/f2", ...) would persist.
	sidecar := buildF2CanonicalSidecar(ingest, logicalF2Dir, fixedTime)

	canonicalBytes, err := SerializeF2CanonicalSidecar(sidecar)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	if err := os.WriteFile(filepath.Join(f2Dir, "cycle-001.canonical.json"), canonicalBytes, 0o644); err != nil {
		t.Fatalf("write canonical: %v", err)
	}

	// Render MD with the SAME logical dir so ReciprocalLocator matches.
	mdBytes, err := RenderF2MarkdownProjection(sidecar, logicalF2Dir)
	if err != nil {
		t.Fatalf("render md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(f2Dir, "cycle-001.md"), mdBytes, 0o644); err != nil {
		t.Fatalf("write md: %v", err)
	}

	return tmp, "cycle-001"
}

func TestCheckF2PairConsistency_NoF2DirSkip(t *testing.T) {
	tmp := t.TempDir()
	r := checkF2PairConsistency(tmp)
	if r.tier != tierSkip {
		t.Fatalf("expected SKIP when no f2 dir, got %s: %s", r.tier, r.detail)
	}
}

func TestCheckF2PairConsistency_EmptyDirSkip(t *testing.T) {
	tmp := t.TempDir()
	f2Dir := filepath.Join(tmp, "docs", "checkpoints", "f2")
	os.MkdirAll(f2Dir, 0o755)
	r := checkF2PairConsistency(tmp)
	if r.tier != tierSkip {
		t.Fatalf("expected SKIP when f2 dir is empty, got %s: %s", r.tier, r.detail)
	}
}

func TestCheckF2PairConsistency_ValidPairPass(t *testing.T) {
	tmp, _ := f2DoctorFixture(t)
	r := checkF2PairConsistency(tmp)
	if r.tier != tierPass {
		t.Fatalf("expected PASS for valid pair, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "not semantic truth") {
		t.Fatalf("detail should state structural consistency is not semantic truth: %s", r.detail)
	}
	if !strings.Contains(r.detail, "content/media truth is not verified") {
		t.Fatalf("detail should state content/media truth is not verified: %s", r.detail)
	}
}

func TestCheckF2PairConsistency_MissingSidecarFail(t *testing.T) {
	tmp, base := f2DoctorFixture(t)
	// Delete the canonical.json.
	f2Dir := filepath.Join(tmp, "docs", "checkpoints", "f2")
	os.Remove(filepath.Join(f2Dir, base+".canonical.json"))

	r := checkF2PairConsistency(tmp)
	if r.tier != tierFail {
		t.Fatalf("expected FAIL for missing canonical, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "incomplete pair") {
		t.Fatalf("detail should mention incomplete pair: %s", r.detail)
	}
}

func TestCheckF2PairConsistency_MissingMDFail(t *testing.T) {
	tmp, base := f2DoctorFixture(t)
	f2Dir := filepath.Join(tmp, "docs", "checkpoints", "f2")
	os.Remove(filepath.Join(f2Dir, base+".md"))

	r := checkF2PairConsistency(tmp)
	if r.tier != tierFail {
		t.Fatalf("expected FAIL for missing MD, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "incomplete pair") {
		t.Fatalf("detail should mention incomplete pair: %s", r.detail)
	}
}

func TestCheckF2PairConsistency_ChangedCanonicalBytesFail(t *testing.T) {
	tmp, base := f2DoctorFixture(t)
	f2Dir := filepath.Join(tmp, "docs", "checkpoints", "f2")

	// Read the canonical, mutate the envelope's semantic_digest so recompute
	// mismatches (canonical content drifted under same digest).
	path := filepath.Join(f2Dir, base+".canonical.json")
	raw, _ := os.ReadFile(path)
	var sidecar F2CanonicalSidecar
	json.Unmarshal(raw, &sidecar)
	// Tamper the semantic_digest (keep content unchanged → recompute differs).
	sidecar.CanonicalEnvelope.SemanticDigest = "tampered-digest"
	sidecar.F2ViewMetadata.SourceSemanticDigest = "tampered-digest"
	tampered, _ := json.Marshal(sidecar)
	os.WriteFile(path, tampered, 0o644)

	r := checkF2PairConsistency(tmp)
	if r.tier != tierFail {
		t.Fatalf("expected FAIL for drifted digest, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "semantic_digest mismatch") {
		t.Fatalf("detail should mention digest mismatch: %s", r.detail)
	}
}

func TestCheckF2PairConsistency_MismatchedCycleFail(t *testing.T) {
	tmp, base := f2DoctorFixture(t)
	f2Dir := filepath.Join(tmp, "docs", "checkpoints", "f2")

	// Tamper the MD's metadata block to carry a different cycle ID.
	mdPath := filepath.Join(f2Dir, base+".md")
	mdBytes, _ := os.ReadFile(mdPath)
	mdStr := string(mdBytes)
	// Replace the cycle ID in the metadata JSON block.
	mdStr = strings.Replace(mdStr, `"synthesis_cycle_id": "cycle-001"`, `"synthesis_cycle_id": "cycle-999"`, 1)
	os.WriteFile(mdPath, []byte(mdStr), 0o644)

	r := checkF2PairConsistency(tmp)
	if r.tier != tierFail {
		t.Fatalf("expected FAIL for mismatched cycle, got %s: %s", r.tier, r.detail)
	}
}

func TestCheckF2PairConsistency_StaleProjectionFail(t *testing.T) {
	tmp, base := f2DoctorFixture(t)
	f2Dir := filepath.Join(tmp, "docs", "checkpoints", "f2")

	// Tamper the MD body (add a comment) so it no longer matches the
	// deterministic re-render.
	mdPath := filepath.Join(f2Dir, base+".md")
	mdBytes, _ := os.ReadFile(mdPath)
	mdStr := string(mdBytes) + "\n<!-- hand edit -->\n"
	os.WriteFile(mdPath, []byte(mdStr), 0o644)

	r := checkF2PairConsistency(tmp)
	if r.tier != tierFail {
		t.Fatalf("expected FAIL for stale projection, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "stale") {
		t.Fatalf("detail should mention stale projection: %s", r.detail)
	}
}

func TestCheckF2PairConsistency_MissingPCWeakestClaimFail(t *testing.T) {
	tmp, _ := f2DoctorFixture(t)
	f2Dir := filepath.Join(tmp, "docs", "checkpoints", "f2")

	// Rebuild the sidecar with all P-a WeakestClaim fields emptied, then
	// re-render the MD (so the pair is otherwise consistent — the only finding
	// is the missing P-c weakest claim).
	env := cloneFixtureForStreak()
	for i := range env.Entries {
		if env.Entries[i].Family == F1FamilyPACounterEvidence && env.Entries[i].PA != nil {
			for j := range env.Entries[i].PA.Probes {
				env.Entries[i].PA.Probes[j].WeakestClaim = ""
			}
		}
	}
	d, _ := env.ComputeDigest()
	env.SemanticDigest = d
	emit := &ValidatedF1Emit{CanonicalEnvelope: env, SemanticDigest: d, ValidationDisposition: F1ValidationComplete}
	ingest, errs := IngestF1EmitForF2(emit)
	if len(errs) != 0 {
		t.Fatalf("ingest: %v", errs)
	}
	sidecar := buildF2CanonicalSidecar(ingest, f2Dir, fixedTime)
	canonicalBytes, _ := SerializeF2CanonicalSidecar(sidecar)
	os.WriteFile(filepath.Join(f2Dir, "cycle-001.canonical.json"), canonicalBytes, 0o644)
	mdBytes, _ := RenderF2MarkdownProjection(sidecar, f2Dir)
	os.WriteFile(filepath.Join(f2Dir, "cycle-001.md"), mdBytes, 0o644)

	r := checkF2PairConsistency(tmp)
	if r.tier != tierFail {
		t.Fatalf("expected FAIL for missing P-c weakest claim, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "weakest_claim") {
		t.Fatalf("detail should mention weakest_claim: %s", r.detail)
	}
}

func TestCheckF2PairConsistency_UnknownPAEnumFail(t *testing.T) {
	tmp, _ := f2DoctorFixture(t)
	f2Dir := filepath.Join(tmp, "docs", "checkpoints", "f2")

	// Rebuild the sidecar with a P-a probe carrying an unknown result enum,
	// then re-render the MD (so the pair is otherwise consistent — the only
	// finding is the unknown enum).
	env := cloneFixtureForStreak()
	for i := range env.Entries {
		if env.Entries[i].Family == F1FamilyPACounterEvidence && env.Entries[i].PA != nil {
			env.Entries[i].PA.Probes[0].Result = "definitely-maybe" // unknown enum
		}
	}
	d, _ := env.ComputeDigest()
	env.SemanticDigest = d
	// Note: IngestF1EmitForF2 will reject this (unknown enum), but the doctor
	// doesn't re-run ingest — it reads sidecars from disk. So we build the
	// sidecar directly.
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
			ReciprocalLocator:              filepath.Join(f2Dir, "cycle-001.md"),
			WriteTimestamp:                 fixedTime.Format("2006-01-02T15:04:05Z07:00"),
		},
	}
	canonicalBytes, _ := SerializeF2CanonicalSidecar(sidecar)
	os.WriteFile(filepath.Join(f2Dir, "cycle-001.canonical.json"), canonicalBytes, 0o644)
	mdBytes, _ := RenderF2MarkdownProjection(sidecar, f2Dir)
	os.WriteFile(filepath.Join(f2Dir, "cycle-001.md"), mdBytes, 0o644)

	r := checkF2PairConsistency(tmp)
	if r.tier != tierFail {
		t.Fatalf("expected FAIL for unknown P-a enum, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "unknown result enum") {
		t.Fatalf("detail should mention unknown result enum: %s", r.detail)
	}
}

func TestCheckF2PairConsistency_UnverifiedPbMediaFail(t *testing.T) {
	tmp, _ := f2DoctorFixture(t)
	f2Dir := filepath.Join(tmp, "docs", "checkpoints", "f2")

	// The fixture wrote a valid pair (canonical + MD, no media). Now tamper
	// ONLY the canonical.json to add a hand-constructed MediaAttachment with
	// a FORGED cycle binding. The original valid MD stays untouched. The
	// doctor will find the P-b provenance gate failure (BoundCycleID !=
	// env.SynthesisCycleID).
	path := filepath.Join(f2Dir, "cycle-001.canonical.json")
	raw, _ := os.ReadFile(path)
	var sidecar F2CanonicalSidecar
	json.Unmarshal(raw, &sidecar)
	// Hand-construct an attachment with a FORGED cycle binding. All other
	// fields are valid so the only finding is the cycle-binding mismatch.
	sidecar.MediaAttachments = []F2MediaAttachment{{
		AttachmentID:    "att-forged",
		EntryID:         "entry-r1",
		Locator:         F2MediaLocator{Kind: F2MediaLocatorKindPath, Value: "path/to/media"},
		CapabilityClass: "vision",
		EvidenceGrade:   F2MediaEvidenceGradeCaptured,
		Provenance: F2MediaProvenance{
			SourceLocator:               "src-a",
			CaptureOrVerificationMethod: "test",
			ObservedOrVerifiedAt:        "2026-07-25",
			ProducerOrVerifierClass:     "tester",
		},
		BoundCycleID: "wrong-cycle",
		BoundDigest:  sidecar.CanonicalEnvelope.SemanticDigest,
	}}
	tampered, _ := json.Marshal(sidecar)
	os.WriteFile(path, tampered, 0o644)

	r := checkF2PairConsistency(tmp)
	if r.tier != tierFail {
		t.Fatalf("expected FAIL for unverified P-b media, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "P-b media") || !strings.Contains(r.detail, "provenance gate") {
		t.Fatalf("detail should mention P-b media provenance gate failure: %s", r.detail)
	}
}

func TestCheckF2PairConsistency_ReciprocalBindingForgedFail(t *testing.T) {
	tmp, _ := f2DoctorFixture(t)
	f2Dir := filepath.Join(tmp, "docs", "checkpoints", "f2")

	// Tamper the canonical's ReciprocalLocator to point at a wrong path.
	path := filepath.Join(f2Dir, "cycle-001.canonical.json")
	raw, _ := os.ReadFile(path)
	var sidecar F2CanonicalSidecar
	json.Unmarshal(raw, &sidecar)
	sidecar.F2ViewMetadata.ReciprocalLocator = "wrong/path.md"
	tampered, _ := json.Marshal(sidecar)
	os.WriteFile(path, tampered, 0o644)

	r := checkF2PairConsistency(tmp)
	if r.tier != tierFail {
		t.Fatalf("expected FAIL for forged reciprocal locator, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "ReciprocalLocator") {
		t.Fatalf("detail should mention ReciprocalLocator: %s", r.detail)
	}
}

func TestCheckF2PairConsistency_R5BindingForgedFail(t *testing.T) {
	tmp, _ := f2DoctorFixture(t)
	f2Dir := filepath.Join(tmp, "docs", "checkpoints", "f2")

	// Tamper the canonical to add a forged R5Binding with wrong cycle/digest.
	path := filepath.Join(f2Dir, "cycle-001.canonical.json")
	raw, _ := os.ReadFile(path)
	var sidecar F2CanonicalSidecar
	json.Unmarshal(raw, &sidecar)
	env := sidecar.CanonicalEnvelope
	sidecar.R5Binding = &F2R5Binding{
		SourceEntryID:  "entry-r1",
		SourceLocators: []string{"src-a"},
		BoundCycleID:   "wrong-cycle",
		BoundDigest:    env.SemanticDigest,
	}
	tampered, _ := json.Marshal(sidecar)
	os.WriteFile(path, tampered, 0o644)

	r := checkF2PairConsistency(tmp)
	if r.tier != tierFail {
		t.Fatalf("expected FAIL for forged R5 binding, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "R5 binding") {
		t.Fatalf("detail should mention R5 binding: %s", r.detail)
	}
}

func TestCheckF2PairConsistency_MissingPCMarkersFail(t *testing.T) {
	tmp, _ := f2DoctorFixture(t)
	f2Dir := filepath.Join(tmp, "docs", "checkpoints", "f2")

	// Overwrite the MD with content that has NO P-c markers (but keep the
	// metadata block so the pair metadata check passes).
	ingest := f2IngestFromFixture(t)
	sidecar := buildF2CanonicalSidecar(ingest, f2Dir, fixedTime)
	// Build a minimal MD with metadata block but no P-c section.
	var mdStr strings.Builder
	mdStr.WriteString("# F2 Markdown projection\n\n> Derived, informational, non-authoritative.\n\n")
	mdStr.WriteString("```f2-view-metadata\n")
	metaBytes, _ := json.Marshal(sidecar.F2ViewMetadata)
	mdStr.Write(metaBytes)
	mdStr.WriteString("\n```\n\n(no P-c section)\n")
	os.WriteFile(filepath.Join(f2Dir, "cycle-001.md"), []byte(mdStr.String()), 0o644)

	r := checkF2PairConsistency(tmp)
	if r.tier != tierFail {
		t.Fatalf("expected FAIL for missing P-c markers, got %s: %s", r.tier, r.detail)
	}
	// The findings will include both stale-projection (MD doesn't match
	// re-render) AND missing P-c markers. Both are legitimate.
	if !strings.Contains(r.detail, "P-c headline structural markers") {
		t.Fatalf("detail should mention P-c headline markers: %s", r.detail)
	}
}

func TestCheckF2PairConsistency_BasenameVsCycleIDMismatchFail(t *testing.T) {
	tmp := t.TempDir()
	f2Dir := filepath.Join(tmp, logicalF2Dir)
	os.MkdirAll(f2Dir, 0o755)

	// Build a pair stored as foo.{canonical.json,md} but the envelope declares
	// cycle "bar". BOTH env.SynthesisCycleID AND meta.SynthesisCycleID are set
	// to "bar" (matching each other), but the pair filenames are "foo". The
	// locators point at docs/checkpoints/f2/bar.{md,canonical.json} (derived
	// from the embedded cycle ID), but the actual files are foo.{canonical.json,md}.
	// The doctor derives expected locators from the PAIR BASENAME (foo) and
	// checks env.SynthesisCycleID == pairBase — both must FAIL.
	ingest := f2IngestFromFixture(t)
	ingest.SynthesisCycleID = "bar"
	ingest.CanonicalEnvelope.SynthesisCycleID = "bar"
	d, _ := ingest.CanonicalEnvelope.ComputeDigest()
	ingest.CanonicalEnvelope.SemanticDigest = d
	ingest.SemanticDigest = d

	sidecar := buildF2CanonicalSidecar(ingest, logicalF2Dir, fixedTime)
	canonicalBytes, _ := SerializeF2CanonicalSidecar(sidecar)
	os.WriteFile(filepath.Join(f2Dir, "foo.canonical.json"), canonicalBytes, 0o644)
	mdBytes, _ := RenderF2MarkdownProjection(sidecar, logicalF2Dir)
	os.WriteFile(filepath.Join(f2Dir, "foo.md"), mdBytes, 0o644)

	r := checkF2PairConsistency(tmp)
	if r.tier != tierFail {
		t.Fatalf("expected FAIL for basename vs cycle ID mismatch, got %s: %s", r.tier, r.detail)
	}
	// The envelope cycle "bar" != pair base "foo" — the independent leg.
	if !strings.Contains(r.detail, "envelope SynthesisCycleID") || !strings.Contains(r.detail, "pair filename base") {
		t.Fatalf("detail should mention envelope SynthesisCycleID vs pair filename base mismatch: %s", r.detail)
	}
}

func TestCheckF2PairConsistency_EnvelopeCycleIndependentMismatchFail(t *testing.T) {
	tmp := t.TempDir()
	f2Dir := filepath.Join(tmp, logicalF2Dir)
	os.MkdirAll(f2Dir, 0o755)

	// TWO-FACED SIDECAR: files are foo.{canonical.json,md}, meta+locators+MD
	// all agree on "foo" (matching the filename), BUT env.SynthesisCycleID="bar".
	// This is the gap the prior BLOCK found: env cycle was never compared to
	// pairBase independently. The digest is recomputed over the "bar" envelope
	// so it's internally consistent — only the envelope-cycle-vs-filename leg
	// catches this.
	ingest := f2IngestFromFixture(t)
	// Keep meta on "foo" (matching filename) — only mutate the envelope cycle.
	ingest.CanonicalEnvelope.SynthesisCycleID = "bar"
	d, _ := ingest.CanonicalEnvelope.ComputeDigest()
	ingest.CanonicalEnvelope.SemanticDigest = d
	ingest.SemanticDigest = d

	// Build sidecar with logicalF2Dir so meta/locators reference "foo"
	// (the ingest.SynthesisCycleID is still "cycle-001" at the meta level
	// since we only changed the envelope). But wait — buildF2CanonicalSidecar
	// uses ingest.SynthesisCycleID for meta.SynthesisCycleID. So let's set
	// ingest.SynthesisCycleID = "foo" to keep meta on "foo".
	ingest.SynthesisCycleID = "foo"

	sidecar := buildF2CanonicalSidecar(ingest, logicalF2Dir, fixedTime)
	canonicalBytes, _ := SerializeF2CanonicalSidecar(sidecar)
	os.WriteFile(filepath.Join(f2Dir, "foo.canonical.json"), canonicalBytes, 0o644)
	mdBytes, _ := RenderF2MarkdownProjection(sidecar, logicalF2Dir)
	os.WriteFile(filepath.Join(f2Dir, "foo.md"), mdBytes, 0o644)

	r := checkF2PairConsistency(tmp)
	if r.tier != tierFail {
		t.Fatalf("expected FAIL for two-faced sidecar (env cycle bar, meta+files foo), got %s: %s", r.tier, r.detail)
	}
	// The ONLY finding should be env.SynthesisCycleID "bar" != pairBase "foo".
	if !strings.Contains(r.detail, "envelope SynthesisCycleID") {
		t.Fatalf("detail should mention envelope SynthesisCycleID mismatch: %s", r.detail)
	}
}

func TestCheckF2PairConsistency_MissingR3EntryFail(t *testing.T) {
	tmp := t.TempDir()
	f2Dir := filepath.Join(tmp, logicalF2Dir)
	os.MkdirAll(f2Dir, 0o755)

	// Build a pair with NO r3_redesign_fork entry — the P-c headline requires
	// R3 (decision frame). The doctor should FAIL.
	env := cloneFixtureForStreak()
	// Remove the R3 entry (index 1) and fix cross-refs so the digest is stable.
	var pruned []F1FamilyEntry
	for _, e := range env.Entries {
		if e.Family != F1FamilyR3RedesignFork {
			pruned = append(pruned, e)
		}
	}
	env.Entries = pruned
	d, _ := env.ComputeDigest()
	env.SemanticDigest = d

	// Bypass ingest (it may reject missing R3 when applicable=required).
	sidecar := &F2CanonicalSidecar{
		Kind:              F2CanonicalSidecarKind,
		CanonicalEnvelope: env,
		F2ViewMetadata: F2ArtifactViewMeta{
			SynthesisCycleID:               env.SynthesisCycleID,
			EntryIDs:                       []string{"entry-r1", "entry-pa"},
			SourceSemanticDigest:           d,
			CanonicalRepresentationVersion: F2CanonicalRepresentationVersion,
			SchemaVersion:                  env.SchemaVersion,
			ProjectionVersion:              F2ProjectionVersion,
			RendererVersion:                F2RendererVersion,
			ReciprocalLocator:              filepath.Join(logicalF2Dir, "cycle-001.md"),
			WriteTimestamp:                 fixedTime.Format("2006-01-02T15:04:05Z07:00"),
		},
	}
	canonicalBytes, _ := SerializeF2CanonicalSidecar(sidecar)
	os.WriteFile(filepath.Join(f2Dir, "cycle-001.canonical.json"), canonicalBytes, 0o644)
	mdBytes, _ := RenderF2MarkdownProjection(sidecar, logicalF2Dir)
	os.WriteFile(filepath.Join(f2Dir, "cycle-001.md"), mdBytes, 0o644)

	r := checkF2PairConsistency(tmp)
	if r.tier != tierFail {
		t.Fatalf("expected FAIL for missing R3 entry, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "r3_redesign_fork") {
		t.Fatalf("detail should mention r3_redesign_fork requirement: %s", r.detail)
	}
}

func TestCheckF2PairConsistency_MetadataCycleVsEnvelopeCycleFail(t *testing.T) {
	tmp := t.TempDir()
	f2Dir := filepath.Join(tmp, logicalF2Dir)
	os.MkdirAll(f2Dir, 0o755)

	// TWO-FACED SIDECAR (meta vs env cycle): files are cycle-001.{canonical.json,md},
	// envelope.SynthesisCycleID = "cycle-001" (matches filename), but
	// F2ViewMetadata.SynthesisCycleID = "cycle-999" (diverges). The MD
	// metadata block also carries "cycle-999" (so MD↔canonical meta agree).
	// This passes step 1b (env cycle == pairBase) and step 4 (MD meta ==
	// canonical meta) but FAILS step 2c (meta cycle != env cycle).
	// The digest is recomputed over the "cycle-001" envelope so it's
	// internally consistent — only the meta-cycle-vs-env-cycle leg catches
	// this.
	ingest := f2IngestFromFixture(t)
	// Envelope stays at "cycle-001" (matching the filename).
	sidecar := buildF2CanonicalSidecar(ingest, logicalF2Dir, fixedTime)
	// Tamper ONLY the F2ViewMetadata cycle.
	sidecar.F2ViewMetadata.SynthesisCycleID = "cycle-999"

	canonicalBytes, _ := SerializeF2CanonicalSidecar(sidecar)
	os.WriteFile(filepath.Join(f2Dir, "cycle-001.canonical.json"), canonicalBytes, 0o644)
	mdBytes, _ := RenderF2MarkdownProjection(sidecar, logicalF2Dir)
	os.WriteFile(filepath.Join(f2Dir, "cycle-001.md"), mdBytes, 0o644)

	r := checkF2PairConsistency(tmp)
	if r.tier != tierFail {
		t.Fatalf("expected FAIL for meta cycle != env cycle, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "F2ViewMetadata.SynthesisCycleID") || !strings.Contains(r.detail, "CanonicalEnvelope.SynthesisCycleID") {
		t.Fatalf("detail should mention meta cycle vs env cycle mismatch: %s", r.detail)
	}
}

func TestCheckF2PairConsistency_NilR3PayloadFail(t *testing.T) {
	tmp := t.TempDir()
	f2Dir := filepath.Join(tmp, logicalF2Dir)
	os.MkdirAll(f2Dir, 0o755)

	// Build a pair where the r3_redesign_fork entry EXISTS but has a nil
	// R3 payload. The P-c headline requires a real decision frame +
	// disposition from the R3 payload — a nil payload means the headline
	// would emit placeholder absence strings.
	env := cloneFixtureForStreak()
	for i := range env.Entries {
		if env.Entries[i].Family == F1FamilyR3RedesignFork {
			env.Entries[i].R3 = nil
		}
	}
	d, _ := env.ComputeDigest()
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
			ReciprocalLocator:              filepath.Join(logicalF2Dir, "cycle-001.md"),
			WriteTimestamp:                 fixedTime.Format("2006-01-02T15:04:05Z07:00"),
		},
	}
	canonicalBytes, _ := SerializeF2CanonicalSidecar(sidecar)
	os.WriteFile(filepath.Join(f2Dir, "cycle-001.canonical.json"), canonicalBytes, 0o644)
	mdBytes, _ := RenderF2MarkdownProjection(sidecar, logicalF2Dir)
	os.WriteFile(filepath.Join(f2Dir, "cycle-001.md"), mdBytes, 0o644)

	r := checkF2PairConsistency(tmp)
	if r.tier != tierFail {
		t.Fatalf("expected FAIL for nil R3 payload, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "R3 payload is nil") {
		t.Fatalf("detail should mention nil R3 payload requirement: %s", r.detail)
	}
}
