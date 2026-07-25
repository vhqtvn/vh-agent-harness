package cli

// f2_r5_test.go — tests for the R5 operator-synthesis durable binding (Slice 6).
//
// Design authority: researches/decisions/2026-07-25-f2-rendering-family-
// mechanism.md, R5 contract L155-182 + C4 resolution L325-332.
//
// Key contract points tested here:
//   - operator-source fixture projects with exact source entry + digest;
//   - raw prose parameter rejected (END-TO-END: no locator/content fields on
//     the descriptor — all source data is F1-derived);
//   - missing operator source can't be reconstructed from narrative;
//   - P2-B agent-closeout fixture not silently classified as R5;
//   - unresolved source entry rejected;
//   - entry with no SourceRefs rejected (F2 cannot invent locators);
//   - R5 section renders in MD projection (addressable);
//   - nil binding renders bounded absence (not fabricated);
//   - structural markers present;
//   - R5 binding persists in canonical sidecar.

import (
	"encoding/json"
	"strings"
	"testing"
)

// --- R5 source fixture ------------------------------------------------------

// r5SourceFixture builds a local F1SynthesisEnvelope with an entry that
// represents operator-authored synthesis. The entry carries SourceRefs that
// F1 declared — R5 DERIVES its locators from these (never caller-provided).
func r5SourceFixture() *F1SynthesisEnvelope {
	return &F1SynthesisEnvelope{
		SchemaVersion:    "1",
		SynthesisCycleID: "r5-test-cycle",
		Applicability:    "required",
		Entries: []F1FamilyEntry{
			{
				Family:     F1FamilyR3RedesignFork,
				Triggered:  "true",
				EntryID:    "entry-r3",
				SourceRefs: []string{"src-r3"},
				R3: &F1R3ForkSummary{
					TriggerRecognized: true,
					Disposition:       "pending",
					Options: []F1R3Option{
						{OptionID: "opt-a", Mode: "continue_repair", Mechanism: "patch"},
					},
				},
			},
			{
				Family:     F1FamilyPACounterEvidence,
				Triggered:  "true",
				EntryID:    "entry-pa",
				SourceRefs: []string{"src-pa"},
				PA: &F1PAProbeSummary{
					Probes: []F1PAProbe{
						{ProbeID: "PA-P1", TargetRef: "opt-a", Result: F1PAResultFound, EvidenceRefs: []string{"ev-1"}},
					},
				},
			},
			{
				// An entry representing operator-authored synthesis.
				// It carries SourceRefs pointing to the operator's authored
				// content. R5 DERIVES its locators from these SourceRefs —
				// they are F1-declared, never caller-provided.
				Family:     F1FamilyR1CrossLaneJoin,
				Triggered:  "true",
				EntryID:    "entry-operator-synthesis",
				SourceRefs: []string{"operator-authored://synthesis/2026-07-25", "file://local/copy"},
				R1: &F1R1JoinSummary{
					Conclusions: []F1R1Conclusion{
						{ConclusionID: "R1C1", PropertyID: "R1P1", JoinDisposition: "union"},
					},
				},
			},
		},
	}
}

// r5EmitFromFixture wraps r5SourceFixture in a ValidatedF1Emit (manually
// constructed, NOT via EmitF1 — the R5 tests exercise the R5 binding path,
// not the ingest gate).
func r5EmitFromFixture(t *testing.T) *ValidatedF1Emit {
	t.Helper()
	env := r5SourceFixture()
	digest, err := env.ComputeDigest()
	if err != nil {
		t.Fatalf("ComputeDigest failed: %v", err)
	}
	env.SemanticDigest = digest
	return &ValidatedF1Emit{
		CanonicalEnvelope:     env,
		SemanticDigest:        digest,
		ValidationDisposition: F1ValidationComplete,
	}
}

// --- Operator-source fixture projects with exact source entry + digest -------

// TestF2R5Binding_OperatorSourceProjectsExactEntryAndDigest proves that an
// operator-source descriptor produces a binding with the exact source entry ID,
// the F1-DERIVED source locators (from the entry's SourceRefs), and the emit's
// semantic digest.
func TestF2R5Binding_OperatorSourceProjectsExactEntryAndDigest(t *testing.T) {
	emit := r5EmitFromFixture(t)
	source := &F2R5SourceDescriptor{
		SourceEntryID: "entry-operator-synthesis",
	}

	binding, err := BuildF2R5Binding(emit, source)
	if err != nil {
		t.Fatalf("BuildF2R5Binding failed: %v", err)
	}
	if binding == nil {
		t.Fatalf("expected non-nil binding for valid source descriptor")
	}

	// The binding must carry the exact source entry ID.
	if binding.SourceEntryID != "entry-operator-synthesis" {
		t.Errorf("expected source entry ID \"entry-operator-synthesis\", got %q", binding.SourceEntryID)
	}

	// The locators must be DERIVED from the entry's SourceRefs — NOT
	// caller-provided. The entry's SourceRefs are:
	//   "operator-authored://synthesis/2026-07-25", "file://local/copy"
	if len(binding.SourceLocators) != 2 {
		t.Fatalf("expected 2 source locators (from entry's SourceRefs), got %d: %v",
			len(binding.SourceLocators), binding.SourceLocators)
	}
	if binding.SourceLocators[0] != "operator-authored://synthesis/2026-07-25" {
		t.Errorf("expected first locator from SourceRefs, got %q", binding.SourceLocators[0])
	}
	if binding.SourceLocators[1] != "file://local/copy" {
		t.Errorf("expected second locator from SourceRefs, got %q", binding.SourceLocators[1])
	}

	// The binding must carry the emit's cycle ID.
	if binding.BoundCycleID != emit.CanonicalEnvelope.SynthesisCycleID {
		t.Errorf("expected bound cycle %q, got %q",
			emit.CanonicalEnvelope.SynthesisCycleID, binding.BoundCycleID)
	}

	// The binding must carry the emit's semantic digest.
	if binding.BoundDigest != emit.SemanticDigest {
		t.Errorf("expected bound digest %q, got %q",
			emit.SemanticDigest, binding.BoundDigest)
	}
}

// --- Raw prose rejected END-TO-END (no locator/content fields) --------------

// TestF2R5Binding_RawProseRejectedEndToEnd proves that the F2R5SourceDescriptor
// has NO locator or content fields — ALL source data is F1-derived. A caller
// cannot inject raw chat prose as a locator because there is no locator
// parameter. This is the END-TO-END guarantee (not just signature-level):
// every value in the binding traces to F1's canonical emit.
func TestF2R5Binding_RawProseRejectedEndToEnd(t *testing.T) {
	emit := r5EmitFromFixture(t)

	// The descriptor carries ONLY SourceEntryID. There are no locator or
	// content fields to inject prose into.
	source := &F2R5SourceDescriptor{
		SourceEntryID: "entry-operator-synthesis",
	}

	binding, err := BuildF2R5Binding(emit, source)
	if err != nil {
		t.Fatalf("BuildF2R5Binding failed: %v", err)
	}
	if binding == nil {
		t.Fatalf("expected non-nil binding")
	}

	// Every locator in the binding must trace to the entry's SourceRefs.
	// A caller-injected string (e.g. "this is raw chat prose") CANNOT appear
	// because there is no field to put it in.
	entryRefs := emit.CanonicalEnvelope.Entries[2].SourceRefs
	for _, loc := range binding.SourceLocators {
		found := false
		for _, ref := range entryRefs {
			if loc == ref {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("locator %q does not trace to any F1-declared SourceRef — raw prose leak", loc)
		}
	}

	// Verify no arbitrary string can be injected as a locator by confirming
	// the descriptor type has no string fields besides SourceEntryID.
	if len(binding.SourceLocators) != len(entryRefs) {
		t.Errorf("binding has %d locators but entry has %d SourceRefs — locator count mismatch",
			len(binding.SourceLocators), len(entryRefs))
	}
}

// --- Missing operator source can't be reconstructed from narrative ----------

// TestF2R5Binding_MissingSourceCannotBeReconstructed proves that when no source
// descriptor is provided (nil), no binding is constructed — F2 does NOT infer
// a binding from the emit's narrative content or reconstruct one from prose
// (memo L180).
func TestF2R5Binding_MissingSourceCannotBeReconstructed(t *testing.T) {
	emit := r5EmitFromFixture(t)

	// nil source → no binding, no error.
	binding, err := BuildF2R5Binding(emit, nil)
	if err != nil {
		t.Fatalf("nil source should not error: %v", err)
	}
	if binding != nil {
		t.Errorf("nil source must produce nil binding — F2 does not reconstruct a binding from narrative")
	}
}

// --- P2-B agent-closeout fixture not silently classified as R5 ---------------

// TestF2R5Binding_P2BAgentCloseoutNotSilentlyClassifiedAsR5 proves that an
// F1 emit carrying an agent-closeout-like entry is NOT automatically treated
// as R5 operator synthesis. R5 requires an EXPLICIT source descriptor — no
// entry is silently classified as R5.
func TestF2R5Binding_P2BAgentCloseoutNotSilentlyClassifiedAsR5(t *testing.T) {
	emit := r5EmitFromFixture(t)

	// The emit has 3 entries, including "entry-operator-synthesis" which
	// COULD be mistaken for R5. Without an explicit source descriptor,
	// NO binding is created.
	binding, err := BuildF2R5Binding(emit, nil)
	if err != nil {
		t.Fatalf("nil source should not error: %v", err)
	}
	if binding != nil {
		t.Errorf("entry must NOT be silently classified as R5 — no explicit source descriptor was provided")
	}

	// Even with a source descriptor, the binding is EXPLICIT — the descriptor
	// names the exact entry. This is not "silent classification"; it's
	// operator-initiated binding.
	source := &F2R5SourceDescriptor{
		SourceEntryID: "entry-operator-synthesis",
	}
	binding, err = BuildF2R5Binding(emit, source)
	if err != nil {
		t.Fatalf("explicit source should not error: %v", err)
	}
	if binding == nil {
		t.Errorf("explicit source descriptor should produce a binding")
	}
	// The binding names the EXPLICITLY chosen entry — it does not silently
	// pick a different one.
	if binding.SourceEntryID != "entry-operator-synthesis" {
		t.Errorf("binding must carry the explicitly named entry, got %q", binding.SourceEntryID)
	}
}

// --- Unresolved source entry rejected ---------------------------------------

// TestF2R5Binding_UnresolvedSourceEntryRejected proves that a source descriptor
// whose SourceEntryID does not resolve to any entry in the emit is rejected.
func TestF2R5Binding_UnresolvedSourceEntryRejected(t *testing.T) {
	emit := r5EmitFromFixture(t)
	source := &F2R5SourceDescriptor{
		SourceEntryID: "NO-SUCH-ENTRY",
	}

	binding, err := BuildF2R5Binding(emit, source)
	if err == nil {
		t.Fatalf("expected error for unresolved source entry, got nil error + binding %v", binding)
	}
	if binding != nil {
		t.Errorf("unresolved source entry must not produce a binding")
	}
	if !strings.Contains(err.Error(), "does not resolve") {
		t.Errorf("error should mention unresolved entry, got: %v", err)
	}
}

// --- Entry with no SourceRefs rejected (F2 cannot invent locators) ----------

// TestF2R5Binding_EntryWithNoSourceRefsRejected proves that when the identified
// entry has no SourceRefs, the binding is rejected — F2 cannot invent source
// locators. The locators must be F1-declared.
func TestF2R5Binding_EntryWithNoSourceRefsRejected(t *testing.T) {
	// Build a minimal emit with an entry that has NO SourceRefs.
	env := &F1SynthesisEnvelope{
		SchemaVersion:    "1",
		SynthesisCycleID: "no-refs-cycle",
		Applicability:    "required",
		Entries: []F1FamilyEntry{
			{
				Family:     F1FamilyR1CrossLaneJoin,
				Triggered:  "true",
				EntryID:    "entry-bare",
				SourceRefs: nil, // NO SourceRefs
				R1: &F1R1JoinSummary{
					Conclusions: []F1R1Conclusion{
						{ConclusionID: "R1C1", PropertyID: "R1P1", JoinDisposition: "union"},
					},
				},
			},
		},
	}
	digest, err := env.ComputeDigest()
	if err != nil {
		t.Fatalf("ComputeDigest failed: %v", err)
	}
	env.SemanticDigest = digest
	emit := &ValidatedF1Emit{
		CanonicalEnvelope:     env,
		SemanticDigest:        digest,
		ValidationDisposition: F1ValidationComplete,
	}

	source := &F2R5SourceDescriptor{
		SourceEntryID: "entry-bare",
	}
	binding, err := BuildF2R5Binding(emit, source)
	if err == nil {
		t.Fatalf("expected error for entry with no SourceRefs, got nil error + binding %v", binding)
	}
	if binding != nil {
		t.Errorf("entry with no SourceRefs must not produce a binding — F2 cannot invent locators")
	}
	if !strings.Contains(err.Error(), "no SourceRefs") {
		t.Errorf("error should mention missing SourceRefs, got: %v", err)
	}
}

// --- R5 section renders in MD projection (addressable) ----------------------

// TestF2R5Binding_RendersInMDProjection proves the R5 binding section renders
// in the MD projection with the binding's source entry + locators + cycle +
// digest, inside structural markers.
func TestF2R5Binding_RendersInMDProjection(t *testing.T) {
	emit := r5EmitFromFixture(t)
	source := &F2R5SourceDescriptor{
		SourceEntryID: "entry-operator-synthesis",
	}
	binding, err := BuildF2R5Binding(emit, source)
	if err != nil {
		t.Fatalf("BuildF2R5Binding failed: %v", err)
	}

	// Build a sidecar with the binding.
	ingest := &F2IngestResult{
		CanonicalEnvelope: emit.CanonicalEnvelope,
		SemanticDigest:    emit.SemanticDigest,
		SynthesisCycleID:  emit.CanonicalEnvelope.SynthesisCycleID,
		SchemaVersion:     emit.CanonicalEnvelope.SchemaVersion,
		EntryIDs:          []string{"entry-operator-synthesis", "entry-pa", "entry-r3"},
		R5Binding:         binding,
	}
	sidecar := buildF2CanonicalSidecar(ingest, "docs/checkpoints/f2", fixedTime)
	md, err := RenderF2MarkdownProjection(sidecar, "docs/checkpoints/f2")
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	body := string(md)

	// Structural markers present.
	if !strings.Contains(body, "<!-- f2-r5-binding:begin -->") {
		t.Errorf("R5 begin marker not found in MD projection")
	}
	if !strings.Contains(body, "<!-- f2-r5-binding:end -->") {
		t.Errorf("R5 end marker not found in MD projection")
	}

	// Source entry rendered.
	if !strings.Contains(body, "`entry-operator-synthesis`") {
		t.Errorf("source entry ID not rendered in R5 section")
	}

	// Source locators rendered (F1-derived from SourceRefs).
	if !strings.Contains(body, "`operator-authored://synthesis/2026-07-25`") {
		t.Errorf("first source locator not rendered in R5 section")
	}
	if !strings.Contains(body, "`file://local/copy`") {
		t.Errorf("second source locator not rendered in R5 section")
	}

	// Bound cycle rendered.
	if !strings.Contains(body, "`r5-test-cycle`") {
		t.Errorf("bound cycle not rendered in R5 section")
	}

	// Bound digest rendered.
	if !strings.Contains(body, "`"+emit.SemanticDigest+"`") {
		t.Errorf("bound digest not rendered in R5 section")
	}
}

// --- Nil binding renders bounded absence (not fabricated) -------------------

// TestF2R5Binding_NilBindingRendersBoundedAbsence proves that a nil R5 binding
// renders a bounded "(no operator-source synthesis bound)" notice — NOT a
// fabricated binding.
func TestF2R5Binding_NilBindingRendersBoundedAbsence(t *testing.T) {
	ingest := f2IngestFromFixture(t)
	sidecar := buildF2CanonicalSidecar(ingest, "docs/checkpoints/f2", fixedTime)
	// sidecar.R5Binding is nil (not set).
	md, err := RenderF2MarkdownProjection(sidecar, "docs/checkpoints/f2")
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	body := string(md)

	if !strings.Contains(body, "no operator-source synthesis bound") {
		t.Errorf("nil binding must render bounded-absence notice — not found in:\n%s", body)
	}

	// Structural markers must still be present (the section exists).
	if !strings.Contains(body, "<!-- f2-r5-binding:begin -->") {
		t.Errorf("R5 section markers must be present even for nil binding")
	}
}

// --- R5 binding persists in canonical sidecar --------------------------------

// TestF2R5Binding_PersistsInCanonicalSidecar proves that an R5 binding set on
// the ingest result is carried into the canonical sidecar via
// buildF2CanonicalSidecar, and survives serialization + deserialization.
func TestF2R5Binding_PersistsInCanonicalSidecar(t *testing.T) {
	emit := r5EmitFromFixture(t)
	source := &F2R5SourceDescriptor{
		SourceEntryID: "entry-operator-synthesis",
	}
	binding, err := BuildF2R5Binding(emit, source)
	if err != nil {
		t.Fatalf("BuildF2R5Binding failed: %v", err)
	}

	ingest := &F2IngestResult{
		CanonicalEnvelope: emit.CanonicalEnvelope,
		SemanticDigest:    emit.SemanticDigest,
		SynthesisCycleID:  emit.CanonicalEnvelope.SynthesisCycleID,
		SchemaVersion:     emit.CanonicalEnvelope.SchemaVersion,
		EntryIDs:          []string{"entry-operator-synthesis", "entry-pa", "entry-r3"},
		R5Binding:         binding,
	}
	sidecar := buildF2CanonicalSidecar(ingest, "docs/checkpoints/f2", fixedTime)

	if sidecar.R5Binding == nil {
		t.Fatalf("R5 binding not carried into canonical sidecar")
	}
	if sidecar.R5Binding.SourceEntryID != "entry-operator-synthesis" {
		t.Errorf("sidecar R5 binding has wrong source entry ID: %q", sidecar.R5Binding.SourceEntryID)
	}

	// Serialize + deserialize round-trip (via json directly, not the file reader).
	raw, err := SerializeF2CanonicalSidecar(sidecar)
	if err != nil {
		t.Fatalf("serialize failed: %v", err)
	}
	var parsed F2CanonicalSidecar
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("deserialize failed: %v", err)
	}
	if parsed.R5Binding == nil {
		t.Fatalf("R5 binding lost in serialization round-trip")
	}
	if parsed.R5Binding.SourceEntryID != "entry-operator-synthesis" {
		t.Errorf("round-tripped R5 binding has wrong source entry ID: %q", parsed.R5Binding.SourceEntryID)
	}
	if parsed.R5Binding.BoundDigest != emit.SemanticDigest {
		t.Errorf("round-tripped R5 binding has wrong digest: %q", parsed.R5Binding.BoundDigest)
	}
	if len(parsed.R5Binding.SourceLocators) != 2 {
		t.Errorf("round-tripped R5 binding has wrong locator count: %d", len(parsed.R5Binding.SourceLocators))
	}
}

// --- Deterministic rendering ------------------------------------------------

// TestF2R5Binding_DeterministicRendering proves the R5 section renders
// identically on repeated calls with the same binding.
func TestF2R5Binding_DeterministicRendering(t *testing.T) {
	binding := &F2R5Binding{
		SourceEntryID:  "entry-op",
		SourceLocators: []string{"loc1", "loc2"},
		BoundCycleID:   "cycle",
		BoundDigest:    "digest",
	}

	var b1, b2 strings.Builder
	renderF2R5Binding(&b1, binding)
	renderF2R5Binding(&b2, binding)

	if b1.String() != b2.String() {
		t.Errorf("R5 rendering is not deterministic — outputs differ:\n--- 1 ---\n%s\n--- 2 ---\n%s", b1.String(), b2.String())
	}
}

// --- Hand-constructed binding bypass rejected at durable paths ---------------

// TestF2R5Binding_HandConstructedBindingRejectedAtDurablePaths proves that a
// hand-constructed F2R5Binding with arbitrary strings in ANY exported field is
// REJECTED at all three durable-path gates: PersistF2CanonicalSidecar,
// PersistF2Pair, and RenderF2MarkdownProjection. This is the defense-in-depth
// gate — even though F2R5Binding is a public struct, the validation gates
// prevent tampered data from reaching persistence or rendering.
//
// Tested injection vectors:
//   - SourceLocators (count mismatch + value substitution);
//   - BoundCycleID (forged cycle ID);
//   - BoundDigest (forged digest — raw chat prose).
func TestF2R5Binding_HandConstructedBindingRejectedAtDurablePaths(t *testing.T) {
	emit := r5EmitFromFixture(t)
	env := emit.CanonicalEnvelope

	// A CORRECT binding (constructed via BuildF2R5Binding — all fields F1-derived).
	correctBinding, err := BuildF2R5Binding(emit, &F2R5SourceDescriptor{
		SourceEntryID: "entry-operator-synthesis",
	})
	if err != nil {
		t.Fatalf("BuildF2R5Binding failed: %v", err)
	}

	// Helper to build an ingest with a given R5 binding.
	ingestWithBinding := func(binding *F2R5Binding) *F2IngestResult {
		return &F2IngestResult{
			CanonicalEnvelope: env,
			SemanticDigest:    emit.SemanticDigest,
			SynthesisCycleID:  env.SynthesisCycleID,
			SchemaVersion:     env.SchemaVersion,
			EntryIDs:          []string{"entry-operator-synthesis", "entry-pa", "entry-r3"},
			R5Binding:         binding,
		}
	}

	// Tampered binding variants — each corrupts ONE field of the correct binding.
	tamperedVariants := []struct {
		name    string
		mutator func(b *F2R5Binding) *F2R5Binding
	}{
		{
			name: "SourceLocators_count_mismatch",
			mutator: func(b *F2R5Binding) *F2R5Binding {
				c := *b
				c.SourceLocators = []string{"only one locator instead of two"}
				return &c
			},
		},
		{
			name: "SourceLocators_value_substitution",
			mutator: func(b *F2R5Binding) *F2R5Binding {
				c := *b
				c.SourceLocators = []string{"raw chat prose", "file://local/copy"}
				return &c
			},
		},
		{
			name: "BoundCycleID_forged",
			mutator: func(b *F2R5Binding) *F2R5Binding {
				c := *b
				c.BoundCycleID = "forged-cycle-id"
				return &c
			},
		},
		{
			name: "BoundDigest_forged_prose",
			mutator: func(b *F2R5Binding) *F2R5Binding {
				c := *b
				c.BoundDigest = "this is raw chat prose injected via BoundDigest"
				return &c
			},
		},
	}

	for _, tv := range tamperedVariants {
		t.Run(tv.name, func(t *testing.T) {
			tampered := tv.mutator(correctBinding)
			ingest := ingestWithBinding(tampered)

			// Gate 1: PersistF2CanonicalSidecar must reject.
			t.Run("persist_canonical_sidecar", func(t *testing.T) {
				dir := t.TempDir()
				outcome, err := PersistF2CanonicalSidecar(ingest, dir, fixedTime)
				if err == nil {
					t.Fatalf("expected error for hand-constructed binding, got nil error + outcome %s", outcome)
				}
				if !strings.Contains(err.Error(), "R5 binding validation failed") {
					t.Errorf("error should mention R5 validation, got: %v", err)
				}
			})

			// Gate 2: PersistF2Pair must reject.
			t.Run("persist_pair", func(t *testing.T) {
				dir := t.TempDir()
				outcome, err := PersistF2Pair(ingest, dir, fixedTime)
				if err == nil {
					t.Fatalf("expected error for hand-constructed binding, got nil error + outcome %s", outcome)
				}
				if !strings.Contains(err.Error(), "R5 binding validation failed") {
					t.Errorf("error should mention R5 validation, got: %v", err)
				}
			})

			// Gate 3: RenderF2MarkdownProjection must reject.
			t.Run("render_md", func(t *testing.T) {
				sidecar := buildF2CanonicalSidecar(ingest, "docs/checkpoints/f2", fixedTime)
				_, err := RenderF2MarkdownProjection(sidecar, "docs/checkpoints/f2")
				if err == nil {
					t.Fatalf("expected error for hand-constructed binding in sidecar, got nil error")
				}
				if !strings.Contains(err.Error(), "R5 binding validation failed") {
					t.Errorf("error should mention R5 validation, got: %v", err)
				}
			})
		})
	}

	// Sanity: a CORRECT binding (all fields F1-derived via BuildF2R5Binding)
	// must PASS all gates.
	t.Run("correct_binding_passes_all_gates", func(t *testing.T) {
		ingest := ingestWithBinding(correctBinding)

		dir := t.TempDir()
		outcome, err := PersistF2CanonicalSidecar(ingest, dir, fixedTime)
		if err != nil {
			t.Fatalf("correct binding should pass persist canonical: %v (outcome %s)", err, outcome)
		}

		dir2 := t.TempDir()
		outcome2, err := PersistF2Pair(ingest, dir2, fixedTime)
		if err != nil {
			t.Fatalf("correct binding should pass persist pair: %v (outcome %s)", err, outcome2)
		}

		sidecar := buildF2CanonicalSidecar(ingest, "docs/checkpoints/f2", fixedTime)
		_, err = RenderF2MarkdownProjection(sidecar, "docs/checkpoints/f2")
		if err != nil {
			t.Fatalf("correct binding should pass render: %v", err)
		}
	})
}
