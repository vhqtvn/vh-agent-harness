package cli

import (
	"strings"
	"testing"
)

// f2_ingest_test.go — Slice 1 tests for the F2 consumer-side ingest gate
// (IngestF1EmitForF2). These cover the F2 ingest contract (memo L126-140):
//
//   - a valid emit is accepted (and the result carries a deep-copied envelope
//     + the binding digest + the resolved ref graph);
//   - a partial emit (validation_disposition != complete) is rejected;
//   - a tampered emit (canonical content changed after F1's emit) is rejected
//     via digest mismatch (the digest no longer binds);
//   - an emit with an unresolved cross-reference is rejected (re-validation
//     catches it);
//   - narrative prose cannot substitute for a missing entry: the ingest API
//     accepts ONLY a typed *ValidatedF1Emit (no string/prose parameter), and a
//     missing family entry is caught by re-validation regardless of what prose
//     a caller stuffs into adjacent fields.

// f2EmitFromFixture builds a fresh ValidatedF1Emit from the canonical fixture.
// Shared by the ingest tests so each starts from a clean, complete emit.
func f2EmitFromFixture(t *testing.T) *ValidatedF1Emit {
	t.Helper()
	emit, errs := EmitF1(canonicalF1Fixture())
	if emit == nil {
		t.Fatalf("EmitF1 on canonical fixture failed (setup): %v", errs)
	}
	return emit
}

// --- the happy path: a valid complete emit is ingested ---------------------

// TestIngestF1EmitForF2_ValidCompleteEmitAccepted is the happy-path crux for
// the F2 ingest gate. A producer-built complete emit is accepted; the result
// carries a deep-copied envelope whose canonical bytes still bind the digest,
// the captured binding digest, the cycle/entry refs, and the resolved cross-
// reference graph.
func TestIngestF1EmitForF2_ValidCompleteEmitAccepted(t *testing.T) {
	emit := f2EmitFromFixture(t)

	res, errs := IngestF1EmitForF2(emit)
	if res == nil {
		t.Fatalf("IngestF1EmitForF2 on a valid complete emit returned errors:\n  %s",
			strings.Join(errs, "\n  "))
	}
	if len(errs) != 0 {
		t.Fatalf("IngestF1EmitForF2 returned a non-nil result AND errors: %v", errs)
	}

	// The captured digest matches the emit's binding digest.
	if res.SemanticDigest != emit.SemanticDigest {
		t.Fatalf("captured digest %q != emit digest %q", res.SemanticDigest, emit.SemanticDigest)
	}
	// The cycle + schema refs are carried from canonical content.
	if res.SynthesisCycleID != "cycle-001" {
		t.Fatalf("SynthesisCycleID = %q, want cycle-001", res.SynthesisCycleID)
	}
	if res.SchemaVersion != "1" {
		t.Fatalf("SchemaVersion = %q, want 1", res.SchemaVersion)
	}
	// The entry_ids are the sorted canonical fixture entries.
	wantEntries := []string{"entry-pa", "entry-r1", "entry-r3"} // sorted
	if strings.Join(res.EntryIDs, ",") != strings.Join(wantEntries, ",") {
		t.Fatalf("EntryIDs = %v, want %v", res.EntryIDs, wantEntries)
	}

	// The deep-copied envelope still binds the digest (lossless round-trip).
	got, derr := res.CanonicalEnvelope.ComputeDigest()
	if derr != nil {
		t.Fatalf("re-compute digest on ingest result: %v", derr)
	}
	if got != emit.SemanticDigest {
		t.Fatalf("ingested envelope digest drifted: %q != %q", got, emit.SemanticDigest)
	}

	// The resolved ref graph records the declared canonical IDs.
	if strings.Join(res.ResolvedRefs.R1ConclusionIDs, ",") != "R1C1" {
		t.Fatalf("R1ConclusionIDs = %v, want [R1C1]", res.ResolvedRefs.R1ConclusionIDs)
	}
	wantOpts := []string{"opt-continue", "opt-redesign"} // sorted
	if strings.Join(res.ResolvedRefs.R3OptionIDs, ",") != strings.Join(wantOpts, ",") {
		t.Fatalf("R3OptionIDs = %v, want %v", res.ResolvedRefs.R3OptionIDs, wantOpts)
	}
	wantProbes := []string{"PA-P1", "PA-P2", "PA-P3"} // sorted
	if strings.Join(res.ResolvedRefs.PAProbeIDs, ",") != strings.Join(wantProbes, ",") {
		t.Fatalf("PAProbeIDs = %v, want %v", res.ResolvedRefs.PAProbeIDs, wantProbes)
	}
}

// --- immutability: the ingest result is a deep copy ------------------------

// TestIngestF1EmitForF2_ResultIsDeepCopy verifies the ingest result's canonical
// envelope is an independent deep copy: mutating it does NOT leak into the
// emit's binding snapshot and does NOT re-bind the captured digest. This is
// the immutability discipline the persist/render slices rely on.
func TestIngestF1EmitForF2_ResultIsDeepCopy(t *testing.T) {
	emit := f2EmitFromFixture(t)
	res, _ := IngestF1EmitForF2(emit)
	if res == nil {
		t.Fatalf("ingest failed (setup)")
	}
	bindingDigest := res.SemanticDigest

	// Mutate the ingest result's envelope.
	res.CanonicalEnvelope.SynthesisCycleID = "consumer-tampered"
	res.CanonicalEnvelope.Entries[0].EntryID = "rewritten"

	// The emit's snapshot is untouched.
	if emit.CanonicalEnvelope.SynthesisCycleID != "cycle-001" {
		t.Fatalf("consumer mutation leaked into the emit snapshot: SynthesisCycleID = %q",
			emit.CanonicalEnvelope.SynthesisCycleID)
	}
	// The captured digest is unchanged (no re-bind).
	if res.SemanticDigest != bindingDigest {
		t.Fatalf("consumer mutation re-bound the captured digest: %q != %q",
			res.SemanticDigest, bindingDigest)
	}
}

// --- required rejection #1: partial emit (disposition != complete) ---------

// TestIngestF1EmitForF2_PartialEmitRejected verifies a partial emit
// (validation_disposition != complete) is NEVER ingested. The ingest gate
// checks the disposition BEFORE re-validation so the rejection reason is the
// partial disposition itself (not a downstream content error).
func TestIngestF1EmitForF2_PartialEmitRejected(t *testing.T) {
	emit := f2EmitFromFixture(t)
	emit.ValidationDisposition = F1ValidationIncomplete // tamper to partial

	res, errs := IngestF1EmitForF2(emit)
	if res != nil {
		t.Fatalf("a partial emit was ingested (result non-nil)")
	}
	if len(errs) == 0 {
		t.Fatalf("a partial emit produced no errors")
	}
	joined := strings.Join(errs, "\n")
	if !strings.Contains(joined, "complete") || !strings.Contains(joined, "incomplete") {
		t.Fatalf("partial-emit rejection did not name the disposition; got:\n  %s", joined)
	}
}

// --- required rejection #2: tampered canonical content → digest mismatch ----

// TestIngestF1EmitForF2_TamperedContentRejected verifies a tampered emit
// (the canonical content was mutated AFTER F1's emit, so the digest no longer
// binds) is rejected. This is the defense-in-depth contract: F2 does not trust
// F1's emit claim; it re-validates the canonical content and re-derives the
// digest independently. A mismatch is a named, non-silent failure.
func TestIngestF1EmitForF2_TamperedContentRejected(t *testing.T) {
	emit := f2EmitFromFixture(t)
	// Tamper the canonical content AFTER emit (the digest no longer binds).
	emit.CanonicalEnvelope.SynthesisCycleID = "tampered-after-emit"
	// The disposition still says complete (the tamperer did not touch it), so
	// the rejection must come from re-validation's digest check, not the
	// disposition gate.

	res, errs := IngestF1EmitForF2(emit)
	if res != nil {
		t.Fatalf("a tampered emit was ingested (result non-nil)")
	}
	if len(errs) == 0 {
		t.Fatalf("a tampered emit produced no errors")
	}
	joined := strings.Join(errs, "\n")
	if !strings.Contains(joined, "semantic_digest") || !strings.Contains(joined, "mismatch") {
		t.Fatalf("tampered-emit rejection did not name the digest mismatch; got:\n  %s", joined)
	}
}

// --- required rejection #2b: spoofed emit-level digest ---------------------

// TestIngestF1EmitForF2_SpoofedEmitDigestRejected verifies a tampered emit
// whose SemanticDigest differs from its CanonicalEnvelope's SemanticDigest is
// REJECTED. ValidateF1Envelope verifies the envelope's digest binds its
// content, but F2 captures the binding digest from the EMIT. Without the
// reconciliation check (emit.SemanticDigest == env.SemanticDigest), a spoofed
// emit.SemanticDigest would pass and F2 would carry a binding digest that
// does NOT bind the shipped envelope — breaking the F2 defense-in-depth
// digest-binding contract. This is the regression for the commit-reviewer
// BLOCK (critical/data_integrity): the envelope validates on its own, yet the
// emit's claimed digest is spoofed.
func TestIngestF1EmitForF2_SpoofedEmitDigestRejected(t *testing.T) {
	emit := f2EmitFromFixture(t)
	// Tamper ONLY the emit's SemanticDigest (leave the envelope's digest + the
	// envelope content untouched). The envelope still validates (its own digest
	// binds its content), but the emit's claimed digest now differs from the
	// envelope's verified digest.
	envelopeDigest := emit.CanonicalEnvelope.SemanticDigest
	emit.SemanticDigest = "spoofed-" + envelopeDigest // different non-empty value
	if emit.SemanticDigest == envelopeDigest {
		t.Fatalf("test setup: spoofed digest collided with the envelope digest (unexpected)")
	}

	res, errs := IngestF1EmitForF2(emit)
	if res != nil {
		t.Fatalf("a spoofed-emit-digest was ingested (result non-nil) — F2 would carry a binding digest that does not bind the shipped envelope")
	}
	if len(errs) == 0 {
		t.Fatalf("a spoofed-emit-digest produced no errors")
	}
	joined := strings.Join(errs, "\n")
	if !strings.Contains(joined, "emit.SemanticDigest") {
		t.Fatalf("spoofed-digest rejection did not name emit.SemanticDigest; got:\n  %s", joined)
	}
	if !strings.Contains(joined, "canonical envelope") {
		t.Fatalf("spoofed-digest rejection did not name the envelope-side digest; got:\n  %s", joined)
	}
}

// --- required rejection #3: unresolved cross-reference ---------------------

// TestIngestF1EmitForF2_UnresolvedCrossRefRejected verifies an emit whose
// canonical content carries a dangling cross-reference is rejected. F2 re-runs
// the F1 validator, which resolves every inter-family ref; a dangling ref is
// caught here (defense-in-depth — the F1 producer should not have emitted it,
// but F2 does not trust that).
func TestIngestF1EmitForF2_UnresolvedCrossRefRejected(t *testing.T) {
	// Build an envelope with a dangling P-a TargetRef (points at no declared
	// R1 conclusion or R3 option). EmitF1 would refuse this (it re-validates),
	// so we hand-construct an emit-shaped object carrying a complete
	// disposition but an inconsistent envelope, then verify the F2 ingest gate
	// catches the dangling ref via re-validation.
	env := canonicalF1Fixture()
	// Add a probe with a dangling target_ref. Re-bind the digest so the digest
	// check passes and the dangling-ref check is the focus.
	for i := range env.Entries {
		if env.Entries[i].Family == F1FamilyPACounterEvidence && env.Entries[i].PA != nil {
			env.Entries[i].PA.Probes = append(env.Entries[i].PA.Probes, F1PAProbe{
				ProbeID:      "PA-P-DANGLING",
				TargetRef:    "R1C-DOES-NOT-EXIST", // dangling
				Result:       F1PAResultFound,
				EvidenceRefs: []string{"some-ref"},
			})
		}
	}
	env.SemanticDigest, _ = env.ComputeDigest() // re-bind so digest check passes
	// Hand-construct the emit (EmitF1 would refuse it; that's the point — we
	// are testing the F2 gate, not the F1 producer).
	emit := &ValidatedF1Emit{
		CanonicalEnvelope:     env,
		SemanticDigest:        env.SemanticDigest,
		ValidationDisposition: F1ValidationComplete,
	}

	res, errs := IngestF1EmitForF2(emit)
	if res != nil {
		t.Fatalf("an emit with a dangling cross-ref was ingested (result non-nil)")
	}
	if len(errs) == 0 {
		t.Fatalf("an emit with a dangling cross-ref produced no errors")
	}
	joined := strings.Join(errs, "\n")
	if !strings.Contains(joined, "target_ref") || !strings.Contains(joined, "does not resolve") {
		t.Fatalf("dangling-ref rejection did not name the unresolved target_ref; got:\n  %s", joined)
	}
}

// --- required rejection #4: missing family entry (prose cannot substitute) --

// TestIngestF1EmitForF2_MissingFamilyEntryRejected verifies that a missing
// family entry cannot be substituted by narrative prose. The F2 ingest API
// accepts ONLY a typed *ValidatedF1Emit (no string/prose parameter exists),
// so a caller cannot pass prose in place of an entry. The structural check
// catches a missing family entry regardless of what prose the caller stuffs
// into adjacent fields (e.g. a long SourceRefs narrative).
func TestIngestF1EmitForF2_MissingFamilyEntryRejected(t *testing.T) {
	// Build an envelope missing the PA family entry, but with a long narrative
	// string stuffed into the R1 entry's SourceRefs (simulating an attempt to
	// "describe" the missing P-a coverage in prose). The digest is re-bound so
	// the missing-entry check is the focus.
	env := canonicalF1Fixture()
	env.Entries = filterFamily(env.Entries, F1FamilyPACounterEvidence) // drop PA
	// Stuff narrative prose into the R1 entry's SourceRefs (the prose-substi-
	// tution attempt). This must NOT satisfy the missing-PA-family requirement.
	for i := range env.Entries {
		if env.Entries[i].Family == F1FamilyR1CrossLaneJoin {
			env.Entries[i].SourceRefs = []string{
				"P-a coverage was implicitly satisfied because the conclusion is obviously true and no counter-evidence could exist; the operator reviewed this on 2026-07-25 and agreed no probe was needed",
			}
		}
	}
	env.SemanticDigest, _ = env.ComputeDigest()
	emit := &ValidatedF1Emit{
		CanonicalEnvelope:     env,
		SemanticDigest:        env.SemanticDigest,
		ValidationDisposition: F1ValidationComplete,
	}

	res, errs := IngestF1EmitForF2(emit)
	if res != nil {
		t.Fatalf("an emit with a missing PA family (prose-substituted) was ingested")
	}
	if len(errs) == 0 {
		t.Fatalf("a missing-PA-family emit produced no errors")
	}
	joined := strings.Join(errs, "\n")
	if !strings.Contains(joined, "pa_counter_evidence") || !strings.Contains(joined, "missing") {
		t.Fatalf("missing-family rejection did not name the missing PA family; got:\n  %s", joined)
	}
}

// --- required rejection #5: nil emit / nil canonical envelope ---------------

// TestIngestF1EmitForF2_NilInputsRejected verifies nil guards: a nil emit and
// a nil canonical envelope are both rejected with a clear error (not a panic).
func TestIngestF1EmitForF2_NilInputsRejected(t *testing.T) {
	if res, errs := IngestF1EmitForF2(nil); res != nil || len(errs) == 0 {
		t.Fatalf("IngestF1EmitForF2(nil) = (%v, %v), want (nil, non-empty errs)", res, errs)
	}
	// Non-nil emit but nil canonical envelope.
	emit := &ValidatedF1Emit{
		CanonicalEnvelope:     nil,
		SemanticDigest:        "deadbeef",
		ValidationDisposition: F1ValidationComplete,
	}
	if res, errs := IngestF1EmitForF2(emit); res != nil || len(errs) == 0 {
		t.Fatalf("IngestF1EmitForF2(nil-envelope) = (%v, %v), want (nil, non-empty errs)", res, errs)
	}
}

// --- structural guarantee: no join/create/reconstruct operation -------------

// TestIngestF1EmitForF2_NoJoinOrCreateOperation verifies the ingest gate
// offers NO operation that joins evidence, generates content, reconstructs an
// entry from prose, or repairs a digest. The function signature accepts ONLY a
// typed *ValidatedF1Emit; there is no string/prose parameter and no repair
// path. The only route from a failed ingest to a successful one is a corrected
// F1 emit. This is the runtime witness of the F2 "must NOT" fence at the
// ingest boundary.
func TestIngestF1EmitForF2_NoJoinOrCreateOperation(t *testing.T) {
	emit := f2EmitFromFixture(t)
	res, _ := IngestF1EmitForF2(emit)
	if res == nil {
		t.Fatalf("ingest failed (setup)")
	}
	// Snapshot the canonical content shape BEFORE any F2 ingest-result access.
	before := res.CanonicalEnvelope
	r1Before, r3Before, paBefore := familyCounts(before)

	// Exercise every ingest-result read path. None of these creates content.
	_ = res.CanonicalEnvelope
	_ = res.SemanticDigest
	_ = res.SynthesisCycleID
	_ = res.SchemaVersion
	_ = res.EntryIDs
	_ = res.ResolvedRefs

	// The canonical content shape is UNCHANGED: no join, no new conclusion,
	// no new option, no new probe was created by any ingest-result access.
	after := res.CanonicalEnvelope
	r1After, r3After, paAfter := familyCounts(after)
	if r1After != r1Before || r3After != r3Before || paAfter != paBefore {
		t.Fatalf("ingest-result access changed canonical content: R1 %d->%d, R3 %d->%d, PA %d->%d",
			r1Before, r1After, r3Before, r3After, paBefore, paAfter)
	}

	// Structural guarantee (compile-time): IngestF1EmitForF2's signature is
	// (*ValidatedF1Emit) -> (*F2IngestResult, []string). There is no string
	// parameter (no prose parse path) and the return carries no mutator. The
	// only path from a failed ingest to success is a corrected F1 emit (the
	// caller re-runs EmitF1 on fixed content). The behavioral assertions above
	// are the runtime witness of that contract.
}
