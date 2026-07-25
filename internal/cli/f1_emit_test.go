package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// f1_emit_test.go — Slice 5 tests for the F1→F2 emit-boundary: ValidatedF1Emit
// (EmitF1), the F2 consumer interface (F2EnvelopeView / ConsumeF1Emit), the
// derived-field allow-list, and the F1/F2 doctor audit (checkF1F2Consistency).
//
// The crux test (TestEmitF1_Crux_ProducerToValidatorToEmitEndToEnd) exercises
// the FULL load-bearing path with the real producers: R1 join -> R3 fork ->
// P-a coverage -> whole-envelope validation -> ValidatedF1Emit -> F2 consume.
// That is the path the behavioral-closure crux names.

// --- the crux: producer -> validator -> emit, end-to-end -------------------

// TestEmitF1_Crux_ProducerToValidatorToEmitEndToEnd is the load-bearing crux
// for the F1 family. It builds an envelope from the REAL producers (JoinR1-
// CrossLane, GenerateR3Fork, GeneratePAProbes), validates it complete, emits
// it, and consumes the emit — exercising producer -> validator -> emit end to
// end. This is the path Slice 5's behavioral-closure names as proven.
func TestEmitF1_Crux_ProducerToValidatorToEmitEndToEnd(t *testing.T) {
	// 1. R1 producer: one UNION conclusion on property "auth-bypass".
	r1, perr := JoinR1CrossLane(
		[]R1LaneInput{{
			LaneID: "lane-security",
			Findings: []R1LaneFinding{{
				PropertyID: "auth-bypass",
				Position:   "hazard-present",
				Sources:    []F1R1Source{{Locator: "commit-abc", AncestryRoots: []string{"root-1"}}},
			}},
		}},
		nil, nil,
	)
	if len(perr) != 0 || r1 == nil {
		t.Fatalf("R1 producer errors: %v", perr)
	}
	r1ConcID := r1.Conclusions[0].ConclusionID // deterministic: R1C-auth-bypass
	if r1ConcID != "R1C-auth-bypass" {
		t.Fatalf("R1 conclusion ID = %q, want R1C-auth-bypass", r1ConcID)
	}

	// 2. P-a producer: cover the R1 conclusion + both R3 options. The producer
	//    assigns PA-P1.. in TargetRef-sorted order; "R1C-..." < "opt-continue"
	//    < "opt-redesign", so PA-P1->R1C, PA-P2->opt-continue, PA-P3->redesign.
	pa, perr := GeneratePAProbes([]PAProbeInput{
		{TargetRef: r1ConcID, Result: F1PAResultFound,
			FalsificationQuestion: "does the auth-bypass conclusion hold?",
			EvidenceRefs:          []string{"audit-log-1"},
			WeakestClaim:          "single-source basis"},
		{TargetRef: "opt-continue", Result: F1PAResultFound,
			FalsificationQuestion: "is the queued repair the cheapest path?",
			EvidenceRefs:          []string{"repair-cost-est"},
			WeakestClaim:          "cost-bound"},
		{TargetRef: "opt-redesign", Result: F1PAResultNotFoundInCheckedScope,
			FalsificationQuestion: "does a prior materially-distinct redesign exist?",
			Method:                "design-archive grep",
			CheckedScope:          []string{"docs/design/"}},
	})
	if len(perr) != 0 || pa == nil {
		t.Fatalf("P-a producer errors: %v", perr)
	}
	if len(pa.Probes) != 3 {
		t.Fatalf("P-a producer generated %d probes, want 3", len(pa.Probes))
	}

	// 3. R3 producer: a triggered fork. Both options reference the R1
	//    conclusion (SupportRefs) and their P-a probes (CounterEvidenceProbeRefs).
	r3, perr := GenerateR3Fork(R3ForkInput{
		RepairIntent:            F1R3RepairIntentPresent,
		StructuralReviewOutcome: F1R3StructuralReviewNonPass,
		ContinueRepair: &F1R3Option{
			OptionID: "opt-continue", Mode: F1R3ModeContinueRepair,
			Mechanism:                "apply the queued repair patch",
			AffectedProperties:       []string{"auth-bypass"},
			SupportRefs:              []string{r1ConcID},
			CounterEvidenceProbeRefs: []string{"PA-P2"},
			Costs:                    []string{"eng-hours"},
			Risks:                    []string{"recurrence"},
			ReversalCost:             "low",
			CheapestValidation:       "re-run suite",
		},
		Redesign: &F1R3Option{
			OptionID: "opt-redesign", Mode: F1R3ModeRedesign,
			Mechanism:                "redesign the auth boundary so bypass cannot recur",
			AffectedProperties:       []string{"auth-bypass"},
			SupportRefs:              []string{r1ConcID},
			CounterEvidenceProbeRefs: []string{"PA-P3"},
			Costs:                    []string{"eng-weeks"},
			Risks:                    []string{"schedule-slip"},
			ReversalCost:             "high",
			CheapestValidation:       "design-review + suite",
		},
	})
	if len(perr) != 0 || r3 == nil {
		t.Fatalf("R3 producer errors: %v", perr)
	}

	// 4. Assemble one entry per family (all triggered).
	env := &F1SynthesisEnvelope{
		SchemaVersion:    "1",
		SynthesisCycleID: "crux-cycle-001",
		Applicability:    F1ApplicabilityRequired,
		Entries: []F1FamilyEntry{
			{Family: F1FamilyR1CrossLaneJoin, Triggered: F1TriggeredTriggered, EntryID: "entry-r1", R1: r1},
			{Family: F1FamilyR3RedesignFork, Triggered: F1TriggeredTriggered, EntryID: "entry-r3", R3: r3},
			{Family: F1FamilyPACounterEvidence, Triggered: F1TriggeredTriggered, EntryID: "entry-pa", PA: pa},
		},
	}
	d, err := env.ComputeDigest()
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	env.SemanticDigest = d

	// 5. EmitF1: validates content (AssignF1Validation) + emits only complete.
	emit, errs := EmitF1(env)
	if emit == nil || len(errs) != 0 {
		t.Fatalf("EmitF1 failed on a producer-built complete envelope:\n  errs=%v", errs)
	}
	if emit.ValidationDisposition != F1ValidationComplete {
		t.Fatalf("emit disposition = %q, want complete", emit.ValidationDisposition)
	}
	if emit.SemanticDigest != d {
		t.Fatalf("emit digest = %q, want %q", emit.SemanticDigest, d)
	}
	// The emitted envelope is an independent deep copy: mutating the producer's
	// env after emit must NOT leak into the emit's snapshot.
	env.SynthesisCycleID = "tampered-after-emit"
	if emit.CanonicalEnvelope.SynthesisCycleID != "crux-cycle-001" {
		t.Fatalf("emit snapshot leaked producer mutation: SynthesisCycleID = %q", emit.CanonicalEnvelope.SynthesisCycleID)
	}
	env.SynthesisCycleID = "crux-cycle-001" // restore for clarity

	// 6. Consume: F2 builds a view with F2-derived metadata.
	view, err := ConsumeF1Emit(emit, F2ViewMetadata{
		StorageLocator:  "docs/synth/crux-001.md",
		RendererVersion: "f2/v1",
	})
	if err != nil {
		t.Fatalf("ConsumeF1Emit: %v", err)
	}

	// 7. Lossless canonical round-trip (required test #1): the consumed
	//    canonical content re-derives the SAME digest as the emit.
	consumed := view.CanonicalEnvelope()
	consumedDigest, err := consumed.ComputeDigest()
	if err != nil {
		t.Fatalf("consumed digest: %v", err)
	}
	if consumedDigest != d {
		t.Fatalf("lossless round-trip failed: consumed digest %q != emit digest %q", consumedDigest, d)
	}
	if err := view.VerifyCanonical(); err != nil {
		t.Fatalf("VerifyCanonical after round-trip: %v", err)
	}

	// 8. Projection references preserved (required test #5): the view carries
	//    synthesis_cycle_id + entry_ids F2 must retain.
	if view.SynthesisCycleID() != "crux-cycle-001" {
		t.Fatalf("view SynthesisCycleID = %q, want crux-cycle-001", view.SynthesisCycleID())
	}
	gotIDs := view.EntryIDs()
	wantIDs := []string{"entry-pa", "entry-r1", "entry-r3"} // sorted
	if strings.Join(gotIDs, ",") != strings.Join(wantIDs, ",") {
		t.Fatalf("view EntryIDs = %v, want %v", gotIDs, wantIDs)
	}
}

// --- required test #2: partial fragments cannot be emitted as complete ------

// TestEmitF1_RefusesPartialAsComplete verifies that an incomplete envelope
// (a partial family fragment) is NEVER emitted as complete. EmitF1 returns
// (nil, errs). Three distinct incompleteness modes are covered.
func TestEmitF1_RefusesPartialAsComplete(t *testing.T) {
	// Mode A: a required envelope missing the PA family entry.
	missingPA := canonicalF1Fixture()
	missingPA.Entries = filterFamily(missingPA.Entries, F1FamilyPACounterEvidence) // drop PA
	missingPA.SemanticDigest, _ = missingPA.ComputeDigest()                        // re-bind digest
	if emit, errs := EmitF1(missingPA); emit != nil {
		t.Fatalf("Mode A (missing PA family): EmitF1 returned a non-nil emit on an incomplete envelope (errs=%v)", errs)
	} else if len(errs) == 0 {
		t.Fatalf("Mode A: EmitF1 returned nil emit with no errors")
	}

	// Mode B: an uncovered P-a target (an R3 option with no coverage probe).
	uncovered := canonicalF1Fixture()
	// Drop the probe covering opt-redesign (PA-P3) so opt-redesign is uncovered.
	uncovered.Entries = mapPAProbes(uncovered.Entries, func(probes []F1PAProbe) []F1PAProbe {
		out := probes[:0]
		for _, p := range probes {
			if p.TargetRef != "opt-redesign" {
				out = append(out, p)
			}
		}
		return out
	})
	// Also drop the now-dangling CounterEvidenceProbeRef on opt-redesign so the
	// failure is specifically the uncovered-target signal.
	uncovered.Entries = mapR3Options(uncovered.Entries, func(opts []F1R3Option) []F1R3Option {
		for i := range opts {
			if opts[i].OptionID == "opt-redesign" {
				opts[i].CounterEvidenceProbeRefs = nil
			}
		}
		return opts
	})
	uncovered.SemanticDigest, _ = uncovered.ComputeDigest()
	if emit, errs := EmitF1(uncovered); emit != nil {
		t.Fatalf("Mode B (uncovered target): EmitF1 returned a non-nil emit on an incomplete envelope (errs=%v)", errs)
	} else if len(errs) == 0 {
		t.Fatalf("Mode B: EmitF1 returned nil emit with no errors")
	}

	// Mode C: a nil envelope.
	if emit, errs := EmitF1(nil); emit != nil || len(errs) == 0 {
		t.Fatalf("Mode C (nil envelope): EmitF1(nil) = (%v, %v), want (nil, non-empty errs)", emit, errs)
	}
}

// --- required test #1: lossless canonical round-trip (focused) -------------

// TestF2View_LosslessCanonicalRoundTrip verifies the emit -> consume -> equals
// contract on the canonical fixture: the consumed envelope's canonical bytes
// equal the emit's canonical bytes (lossless rep).
func TestF2View_LosslessCanonicalRoundTrip(t *testing.T) {
	emit, errs := EmitF1(canonicalF1Fixture())
	if emit == nil {
		t.Fatalf("EmitF1: %v", errs)
	}
	emitBytes, _ := emit.CanonicalBytes()
	view, err := ConsumeF1Emit(emit, F2ViewMetadata{})
	if err != nil {
		t.Fatalf("ConsumeF1Emit: %v", err)
	}
	consumedBytes, _ := view.CanonicalEnvelope().CanonicalBytes()
	if string(consumedBytes) != string(emitBytes) {
		t.Fatalf("lossless round-trip failed: consumed canonical bytes != emit canonical bytes")
	}
}

// --- required test #3: consumer cannot modify canonical content -----------

// TestF2View_ConsumerCannotModifyCanonicalUnderSameDigest verifies that a
// consumer's mutation of the canonical envelope it reads does NOT re-bind the
// digest and does NOT corrupt the emit's snapshot. The canonical content under
// the binding digest is immutable through the consumer interface (there is no
// re-bind API; the only path to a different digest is a new emit).
func TestF2View_ConsumerCannotModifyCanonicalUnderSameDigest(t *testing.T) {
	emit, errs := EmitF1(canonicalF1Fixture())
	if emit == nil {
		t.Fatalf("EmitF1: %v", errs)
	}
	view, err := ConsumeF1Emit(emit, F2ViewMetadata{})
	if err != nil {
		t.Fatalf("ConsumeF1Emit: %v", err)
	}
	bindingDigest := view.CanonicalDigest()

	// The consumer reads a deep copy and mutates it.
	copy1 := view.CanonicalEnvelope()
	copy1.SynthesisCycleID = "consumer-tampered"
	copy1.Entries[0].EntryID = "consumer-rewrote"

	// The binding digest is UNCHANGED (the mutation did not re-bind).
	if view.CanonicalDigest() != bindingDigest {
		t.Fatalf("consumer mutation re-bound the digest: %q != %q", view.CanonicalDigest(), bindingDigest)
	}
	// The emit's snapshot is untouched: VerifyCanonical still passes (the copy
	// the consumer mutated is disposable and never fed back into the binding).
	if err := view.VerifyCanonical(); err != nil {
		t.Fatalf("consumer mutation leaked into the emit snapshot (VerifyCanonical): %v", err)
	}
	// The mutated copy's own digest differs from the binding digest: the
	// consumer CANNOT make the binding digest match its mutated content.
	copyDigest, _ := copy1.ComputeDigest()
	if copyDigest == bindingDigest {
		t.Fatalf("consumer-mutated copy produced the SAME digest as the binding — digest did not actually cover the changed field")
	}
}

// --- required test #4: F2-only metadata does not enter the digest ----------

// TestF2View_F2MetadataExcludedFromDigest verifies that F2-derived view
// metadata (the allow-list) can be added/changed WITHOUT entering the semantic
// digest. Changing an F2 view field never changes the binding digest.
func TestF2View_F2MetadataExcludedFromDigest(t *testing.T) {
	emit, errs := EmitF1(canonicalF1Fixture())
	if emit == nil {
		t.Fatalf("EmitF1: %v", errs)
	}
	view, err := ConsumeF1Emit(emit, F2ViewMetadata{})
	if err != nil {
		t.Fatalf("ConsumeF1Emit: %v", err)
	}
	digestBefore := view.CanonicalDigest()

	// Attach a full F2 view (all allow-list fields).
	view.SetF2View(F2ViewMetadata{
		StorageLocator:    "docs/synth/001.md",
		WriteTimestamp:    "2026-07-25T00:00:00Z",
		ViewModelVersion:  "v1",
		RendererVersion:   "f2/v1",
		AttachmentMetaRef: "media/att-001.json",
	})
	if view.CanonicalDigest() != digestBefore {
		t.Fatalf("F2 view metadata changed the binding digest: %q != %q", view.CanonicalDigest(), digestBefore)
	}
	if err := view.VerifyCanonical(); err != nil {
		t.Fatalf("F2 view metadata broke VerifyCanonical: %v", err)
	}
	// Directly on the envelope too: setting F2View does not change ComputeDigest.
	env := canonicalF1Fixture()
	withoutF2, _ := env.ComputeDigest()
	env.F2View = &F2ViewMetadata{StorageLocator: "x", WriteTimestamp: "y", RendererVersion: "z"}
	withF2, _ := env.ComputeDigest()
	if withF2 != withoutF2 {
		t.Fatalf("ComputeDigest changed when F2View was set: %q != %q", withF2, withoutF2)
	}
}

// --- required test #5: projection references preserved (focused) -----------

// TestF2View_ProjectionReferencesPreserved verifies the view retains
// synthesis_cycle_id + entry_ids (the digest-binding references F2 must carry).
func TestF2View_ProjectionReferencesPreserved(t *testing.T) {
	emit, errs := EmitF1(canonicalF1Fixture())
	if emit == nil {
		t.Fatalf("EmitF1: %v", errs)
	}
	view, err := ConsumeF1Emit(emit, F2ViewMetadata{})
	if err != nil {
		t.Fatalf("ConsumeF1Emit: %v", err)
	}
	if view.SynthesisCycleID() != "cycle-001" {
		t.Fatalf("SynthesisCycleID = %q, want cycle-001", view.SynthesisCycleID())
	}
	// The canonical fixture has entries entry-r1, entry-r3, entry-pa.
	got := view.EntryIDs()
	want := []string{"entry-pa", "entry-r1", "entry-r3"} // sorted
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("EntryIDs = %v, want %v", got, want)
	}
	// The digest itself is carried (the third binding reference).
	if view.CanonicalDigest() == "" {
		t.Fatalf("CanonicalDigest is empty (the binding digest reference must be retained)")
	}
}

// --- required test #6: digest mismatch detected, new emit required ---------

// TestF2View_DigestMismatchDetected_NewEmitRequired verifies that a changed
// canonical field is DETECTED (VerifyCanonical fails) and that the only path
// to a digest matching changed content is a NEW emit — there is no in-place
// correction API on the view.
func TestF2View_DigestMismatchDetected_NewEmitRequired(t *testing.T) {
	emit, errs := EmitF1(canonicalF1Fixture())
	if emit == nil {
		t.Fatalf("EmitF1: %v", errs)
	}
	view, err := ConsumeF1Emit(emit, F2ViewMetadata{})
	if err != nil {
		t.Fatalf("ConsumeF1Emit: %v", err)
	}
	if err := view.VerifyCanonical(); err != nil {
		t.Fatalf("VerifyCanonical before drift: %v", err)
	}
	bindingDigest := view.CanonicalDigest()

	// Simulate canonical drift (e.g. F2 or persistence edited the bytes in
	// place). Reach past the view into the emit's canonical envelope.
	emit.CanonicalEnvelope.SynthesisCycleID = "drifted-cycle"

	// VerifyCanonical now detects the mismatch.
	if err := view.VerifyCanonical(); err == nil {
		t.Fatalf("VerifyCanonical passed after canonical drift (mismatch not detected)")
	}

	// The binding digest is unchanged (there is no in-place re-bind API): the
	// drift is visible as a mismatch, NOT silently absorbed.
	if view.CanonicalDigest() != bindingDigest {
		t.Fatalf("drift silently re-bound the digest: %q != %q", view.CanonicalDigest(), bindingDigest)
	}

	// The ONLY path to a digest matching changed content is a new emit. Build a
	// fresh envelope with the changed field + a fresh digest, re-emit.
	changed := canonicalF1Fixture()
	changed.SynthesisCycleID = "drifted-cycle"
	changed.SemanticDigest, _ = changed.ComputeDigest()
	emit2, errs2 := EmitF1(changed)
	if emit2 == nil {
		t.Fatalf("EmitF1 on the changed (still-consistent) envelope failed: %v", errs2)
	}
	if emit2.SemanticDigest == bindingDigest {
		t.Fatalf("re-emit on changed content produced the SAME digest as the original — digest did not cover the changed field")
	}
}

// --- required test #7: no F2 adapter performs joins or creates content -----

// TestF2View_NoJoinOrCreateContentOperations verifies the F2 consumer interface
// offers NO operation that joins evidence, merges properties, generates R3/P-a
// content, adds conclusions, or infers gaps. Every F2 view operation leaves the
// canonical content (entry / conclusion / option / probe counts) unchanged.
// The producer acts (JoinR1CrossLane / GenerateR3Fork / GeneratePAProbes) are
// NOT methods on F2EnvelopeView.
func TestF2View_NoJoinOrCreateContentOperations(t *testing.T) {
	emit, errs := EmitF1(canonicalF1Fixture())
	if emit == nil {
		t.Fatalf("EmitF1: %v", errs)
	}
	view, err := ConsumeF1Emit(emit, F2ViewMetadata{})
	if err != nil {
		t.Fatalf("ConsumeF1Emit: %v", err)
	}

	// Snapshot the canonical content shape BEFORE F2 operations.
	before := view.CanonicalEnvelope()
	r1Before, r3Before, paBefore := familyCounts(before)

	// Exercise every F2 view mutation/read operation.
	view.SetF2View(F2ViewMetadata{StorageLocator: "x", RendererVersion: "y"})
	_ = view.CanonicalEnvelope()
	_ = view.CanonicalDigest()
	_ = view.SynthesisCycleID()
	_ = view.EntryIDs()
	_ = view.F2View()
	_ = view.VerifyCanonical()

	// The canonical content shape is UNCHANGED: no join, no new conclusion,
	// no new option, no new probe was created by any F2 operation.
	after := view.CanonicalEnvelope()
	r1After, r3After, paAfter := familyCounts(after)
	if r1After != r1Before || r3After != r3Before || paAfter != paBefore {
		t.Fatalf("F2 operations changed canonical content: R1 %d->%d, R3 %d->%d, PA %d->%d",
			r1Before, r1After, r3Before, r3After, paBefore, paAfter)
	}

	// Structural guarantee (compile-time): the F2EnvelopeView method set is
	// read-only + SetF2View only (CanonicalEnvelope, CanonicalDigest,
	// SynthesisCycleID, EntryIDs, SetF2View, F2View, VerifyCanonical). None of
	// the producer acts (JoinR1CrossLane / GenerateR3Fork / GeneratePAProbes)
	// is a method on the view — they are package-level F1 producer functions.
	// There is therefore no API path to join, fork, or probe through the view;
	// the behavioral assertion above is the runtime witness of that contract.
}

// --- doctor audit: checkF1F2Consistency -----------------------------------

func TestCheckF1F2Consistency_SkipWhenNoF2View(t *testing.T) {
	// A projection with NO f2_view is not F2-bearing -> SKIP (nothing to audit).
	body := f1ProjectionMarkdown(canonicalF1FixtureJSON())
	findings, seen := analyzeF1F2Consistency(body)
	if seen != 0 {
		t.Fatalf("seen = %d, want 0 (no F2-bearing projection)", seen)
	}
	if len(findings) != 0 {
		t.Fatalf("expected no findings on a non-F2-bearing projection, got %v", findings)
	}
	// No blocks at all -> also SKIP.
	findings, seen = analyzeF1F2Consistency("no projections here")
	if seen != 0 || len(findings) != 0 {
		t.Fatalf("empty body: seen=%d findings=%v", seen, findings)
	}
}

// TestCheckF1F2Consistency_PassOnConsistentF2View verifies a well-formed F2-
// bearing projection (digest binds, refs present, allow-list respected) PASSes.
func TestCheckF1F2Consistency_PassOnConsistentF2View(t *testing.T) {
	env := canonicalF1Fixture()
	env.F2View = &F2ViewMetadata{
		StorageLocator:  "docs/synth/001.md",
		WriteTimestamp:  "2026-07-25T00:00:00Z",
		RendererVersion: "f2/v1",
	}
	body := f1ProjectionMarkdown(envJSONWithF2View(env))
	findings, seen := analyzeF1F2Consistency(body)
	if seen != 1 {
		t.Fatalf("seen = %d, want 1", seen)
	}
	if len(findings) != 0 {
		t.Fatalf("expected no findings on a consistent F2 view, got %v", findings)
	}
}

// TestCheckF1F2Consistency_FailOnDriftedDigest verifies F2 drifting canonical
// content under the same digest is detected.
func TestCheckF1F2Consistency_FailOnDriftedDigest(t *testing.T) {
	env := canonicalF1Fixture()
	env.F2View = &F2ViewMetadata{StorageLocator: "docs/synth/001.md"}
	// Drift the canonical content WITHOUT re-binding the digest.
	env.SynthesisCycleID = "drifted"
	body := f1ProjectionMarkdown(envJSONWithF2View(env))
	findings, seen := analyzeF1F2Consistency(body)
	if seen != 1 {
		t.Fatalf("seen = %d, want 1", seen)
	}
	if len(findings) == 0 {
		t.Fatalf("expected a digest-mismatch finding on drifted canonical content")
	}
	joined := strings.Join(findings, "\n")
	if !strings.Contains(joined, "semantic_digest mismatch") {
		t.Fatalf("findings did not name the digest mismatch: %v", findings)
	}
}

// TestCheckF1F2Consistency_FailOnForeignDerivedField verifies a foreign field
// on the f2_view object (content smuggled past the allow-list) is detected.
func TestCheckF1F2Consistency_FailOnForeignDerivedField(t *testing.T) {
	env := canonicalF1Fixture()
	// Render a valid envelope, then inject a foreign field into the f2_view
	// object ALONGSIDE a known field (the smuggling-via-mix case).
	base := envJSONWithF2View(env)
	tampered := strings.Replace(base,
		`"f2_view":{"storage_locator":"docs/synth/001.md"}`,
		`"f2_view":{"storage_locator":"docs/synth/001.md","conclusion_override":"F2 cannot do this"}`, 1)
	if tampered == base {
		t.Fatalf("test setup failed: could not inject foreign f2_view field into fixture")
	}
	body := f1ProjectionMarkdown(tampered)
	findings, seen := analyzeF1F2Consistency(body)
	if seen != 1 {
		t.Fatalf("seen = %d, want 1", seen)
	}
	if len(findings) == 0 {
		t.Fatalf("expected a foreign-field finding on a smuggled f2_view key")
	}
	joined := strings.Join(findings, "\n")
	if !strings.Contains(joined, "f2_view") || !strings.Contains(joined, "allow-list") {
		t.Fatalf("findings did not name the allow-list violation: %v", findings)
	}
}

// TestCheckF1F2Consistency_FailOnPureForeignF2View is the regression for the
// fail-open defect: an f2_view object carrying ONLY a foreign key (no allow-
// list fields) must FAIL the audit, not SKIP. json.Unmarshal silently drops
// the unknown key, so F2-bearing status MUST be decided from raw "f2_view" key
// presence, not from non-empty known fields on the unmarshaled struct.
func TestCheckF1F2Consistency_FailOnPureForeignF2View(t *testing.T) {
	env := canonicalF1Fixture()
	base := envJSONWithF2View(env)
	// Replace the whole f2_view object with one carrying ONLY a foreign key.
	pureForeign := strings.Replace(base,
		`"f2_view":{"storage_locator":"docs/synth/001.md"}`,
		`"f2_view":{"conclusion_override":"F2 cannot do this, no allow-list keys"}`, 1)
	if pureForeign == base {
		t.Fatalf("test setup failed: could not replace f2_view with a pure-foreign object")
	}
	body := f1ProjectionMarkdown(pureForeign)
	findings, seen := analyzeF1F2Consistency(body)
	if seen != 1 {
		t.Fatalf("pure-foreign f2_view: seen = %d, want 1 (must be audited, not skipped)", seen)
	}
	if len(findings) == 0 {
		t.Fatalf("pure-foreign f2_view must FAIL the allow-list audit (fail-open regression)")
	}
	joined := strings.Join(findings, "\n")
	if !strings.Contains(joined, "conclusion_override") || !strings.Contains(joined, "allow-list") {
		t.Fatalf("findings did not name the pure-foreign allow-list violation: %v", findings)
	}
}

// TestCheckF1F2Consistency_FailOnNullF2View is the regression for the
// null-f2_view fail-open: an explicit "f2_view": null must FAIL the audit.
// Go's json.Decoder.Decode into a non-pointer struct accepts literal null
// silently (no error), so a null f2_view must be rejected BEFORE decoding by
// requiring a leading '{'. (Other non-object values -- string/number/array/
// bool -- make the whole block fail to unmarshal as an F1SynthesisEnvelope,
// so they are caught by check #16 f1-envelope at the block-parse step; null
// is the one non-object value that parses cleanly and reaches #17.)
func TestCheckF1F2Consistency_FailOnNullF2View(t *testing.T) {
	env := canonicalF1Fixture()
	base := envJSONWithF2View(env)
	replaced := strings.Replace(base,
		`"f2_view":{"storage_locator":"docs/synth/001.md"}`,
		`"f2_view":null`, 1)
	if replaced == base {
		t.Fatalf("test setup failed: could not replace f2_view with null")
	}
	body := f1ProjectionMarkdown(replaced)
	findings, seen := analyzeF1F2Consistency(body)
	if seen != 1 {
		t.Fatalf("null f2_view: seen = %d, want 1 (null parses cleanly as an envelope; it must be audited, not skipped)", seen)
	}
	if len(findings) == 0 {
		t.Fatalf("null f2_view must FAIL (a non-object f2_view is not a valid derived-view surface)")
	}
	joined := strings.Join(findings, "\n")
	if !strings.Contains(joined, "must be a JSON object") {
		t.Fatalf("null f2_view: findings did not name the non-object violation: %v", findings)
	}
}

// TestCheckF1F2Consistency_FailOnMissingBindingRefs verifies a missing
// synthesis_cycle_id / entry_ids on an F2-bearing projection is detected.
func TestCheckF1F2Consistency_FailOnMissingBindingRefs(t *testing.T) {
	env := canonicalF1Fixture()
	env.F2View = &F2ViewMetadata{StorageLocator: "docs/synth/001.md"}
	env.SynthesisCycleID = "" // missing binding reference
	env.SemanticDigest = ""   // also missing -> recompute would fail; set a stale digest instead
	// Set a non-empty stale digest so the missing-cycle_id finding is the focus.
	env.SemanticDigest = "deadbeef"
	base := envJSONWithF2View(env)
	body := f1ProjectionMarkdown(base)
	findings, seen := analyzeF1F2Consistency(body)
	if seen != 1 {
		t.Fatalf("seen = %d, want 1", seen)
	}
	joined := strings.Join(findings, "\n")
	if !strings.Contains(joined, "synthesis_cycle_id") {
		t.Fatalf("findings did not name the missing synthesis_cycle_id: %v", findings)
	}
}

// --- test helpers ----------------------------------------------------------

// familyCounts returns (R1 conclusion count, R3 option count, PA probe count)
// across the envelope's entries. Used to assert F2 operations do not create
// content.
func familyCounts(env *F1SynthesisEnvelope) (int, int, int) {
	var r1, r3, pa int
	for _, e := range env.Entries {
		switch e.Family {
		case F1FamilyR1CrossLaneJoin:
			if e.R1 != nil {
				r1 = len(e.R1.Conclusions)
			}
		case F1FamilyR3RedesignFork:
			if e.R3 != nil {
				r3 = len(e.R3.Options)
			}
		case F1FamilyPACounterEvidence:
			if e.PA != nil {
				pa = len(e.PA.Probes)
			}
		}
	}
	return r1, r3, pa
}

// filterFamily returns entries EXCLUDING the given family (used to drop a
// family entry and make the envelope incomplete).
func filterFamily(entries []F1FamilyEntry, dropFamily string) []F1FamilyEntry {
	out := make([]F1FamilyEntry, 0, len(entries))
	for _, e := range entries {
		if e.Family != dropFamily {
			out = append(out, e)
		}
	}
	return out
}

// mapPAProbes applies fn to the PA entry's probes (returns new entries).
func mapPAProbes(entries []F1FamilyEntry, fn func([]F1PAProbe) []F1PAProbe) []F1FamilyEntry {
	out := make([]F1FamilyEntry, len(entries))
	copy(out, entries)
	for i := range out {
		if out[i].Family == F1FamilyPACounterEvidence && out[i].PA != nil {
			out[i].PA = &F1PAProbeSummary{Probes: fn(out[i].PA.Probes)}
		}
	}
	return out
}

// mapR3Options applies fn to the R3 entry's options (in place on a copy).
func mapR3Options(entries []F1FamilyEntry, fn func([]F1R3Option) []F1R3Option) []F1FamilyEntry {
	out := make([]F1FamilyEntry, len(entries))
	copy(out, entries)
	for i := range out {
		if out[i].Family == F1FamilyR3RedesignFork && out[i].R3 != nil {
			out[i].R3 = &F1R3ForkSummary{
				TriggerRecognized: out[i].R3.TriggerRecognized,
				Options:           fn(out[i].R3.Options),
				Disposition:       out[i].R3.Disposition,
				Selection:         out[i].R3.Selection,
			}
		}
	}
	return out
}

// canonicalF1FixtureJSON returns the canonical fixture rendered as full JSON
// (including semantic_digest + validation, as a committed artifact carries),
// WITHOUT an f2_view.
func canonicalF1FixtureJSON() string {
	env := canonicalF1Fixture()
	b, err := json.Marshal(env)
	if err != nil {
		panic("canonical fixture JSON: " + err.Error())
	}
	return string(b)
}

// envJSONWithF2View returns env rendered as full JSON with a minimal f2_view
// (storage_locator only). Used to build F2-bearing projection bodies.
func envJSONWithF2View(env *F1SynthesisEnvelope) string {
	if env.F2View == nil {
		env.F2View = &F2ViewMetadata{StorageLocator: "docs/synth/001.md"}
	}
	b, err := json.Marshal(env)
	if err != nil {
		panic("env JSON: " + err.Error())
	}
	return string(b)
}

// f1ProjectionMarkdown wraps raw envelope JSON in a fenced
// ```f1-synthesis-envelope block (the form the doctor scanner reads).
func f1ProjectionMarkdown(rawJSON string) string {
	return "```f1-synthesis-envelope\n" + rawJSON + "\n```"
}
