package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// f1_r1_test.go — Slice 2 tests for the R1 cross-lane join producer, the
// validator's R1-specific checks (join disposition, source-locator integrity,
// shared-ancestry collapse, hazard survival chain), envelope immutability, and
// the doctor's detection of a committed R1 projection whose digest or hazard
// links diverge from the canonical record.
//
// These tests cover the Slice 2 contract:
//   - two lanes same property -> one MERGE join;
//   - different properties stay independent UNION;
//   - contradictions survive serialization;
//   - hazard<->symptom links survive round-trip;
//   - shared-ancestry sources are not double-counted as independent;
//   - missing source locators fail on evidence-bearing claims;
//   - a new gate/verdict event produces a new cycle (no mutation of prior);
//   - doctor detects a committed projection whose digest/hazard-links diverge.

// --- producer: deterministic join ------------------------------------------

func TestJoinR1_TwoLanesSamePropertyIsOneMerge(t *testing.T) {
	summary, perr := JoinR1CrossLane([]R1LaneInput{
		{LaneID: "lane-a", Findings: []R1LaneFinding{{PropertyID: "P1", Position: "agree", Sources: []F1R1Source{{Locator: "src-a"}}}}},
		{LaneID: "lane-b", Findings: []R1LaneFinding{{PropertyID: "P1", Position: "agree", Sources: []F1R1Source{{Locator: "src-b"}}}}},
	}, nil, nil)
	if len(perr) != 0 {
		t.Fatalf("producer errors: %v", perr)
	}
	if len(summary.Conclusions) != 1 {
		t.Fatalf("expected 1 merged conclusion, got %d", len(summary.Conclusions))
	}
	c := summary.Conclusions[0]
	if c.JoinDisposition != F1R1JoinMerge {
		t.Fatalf("join_disposition = %q, want merge", c.JoinDisposition)
	}
	if len(c.Lanes) != 2 {
		t.Fatalf("expected 2 lanes in the merge, got %d", len(c.Lanes))
	}
	if len(c.Sources) != 2 {
		t.Fatalf("expected 2 independent sources, got %d", len(c.Sources))
	}
}

func TestJoinR1_DifferentPropertiesStayIndependentUnion(t *testing.T) {
	summary, perr := JoinR1CrossLane([]R1LaneInput{
		{LaneID: "lane-a", Findings: []R1LaneFinding{{PropertyID: "P1", Sources: []F1R1Source{{Locator: "src-a"}}}}},
		{LaneID: "lane-b", Findings: []R1LaneFinding{{PropertyID: "P2", Sources: []F1R1Source{{Locator: "src-b"}}}}},
	}, nil, nil)
	if len(perr) != 0 {
		t.Fatalf("producer errors: %v", perr)
	}
	if len(summary.Conclusions) != 2 {
		t.Fatalf("expected 2 independent conclusions (one per property), got %d", len(summary.Conclusions))
	}
	for _, c := range summary.Conclusions {
		if c.JoinDisposition != F1R1JoinUnion {
			t.Fatalf("property %s join_disposition = %q, want union (distinct property, single lane)", c.PropertyID, c.JoinDisposition)
		}
		if len(c.Lanes) != 1 {
			t.Fatalf("property %s: union conclusion should have exactly 1 lane, got %d", c.PropertyID, len(c.Lanes))
		}
	}
}

func TestJoinR1_ContradictionsSurviveSerialization(t *testing.T) {
	summary, _ := JoinR1CrossLane([]R1LaneInput{
		{LaneID: "lane-a", Findings: []R1LaneFinding{{PropertyID: "P1", Position: "pass", Sources: []F1R1Source{{Locator: "src-a"}}}}},
		{LaneID: "lane-b", Findings: []R1LaneFinding{{PropertyID: "P1", Position: "fail", Sources: []F1R1Source{{Locator: "src-b"}}}}},
	}, nil, nil)
	if len(summary.Conclusions) != 1 {
		t.Fatalf("expected 1 merged conclusion, got %d", len(summary.Conclusions))
	}
	if len(summary.Conclusions[0].Contradictions) == 0 {
		t.Fatalf("expected a contradiction recorded for differing positions")
	}
	// Round-trip the conclusion through JSON and assert the contradiction survives.
	b, err := json.Marshal(summary.Conclusions[0])
	if err != nil {
		t.Fatal(err)
	}
	var back F1R1Conclusion
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if len(back.Contradictions) != len(summary.Conclusions[0].Contradictions) {
		t.Fatalf("contradiction lost in round-trip: %d -> %d", len(summary.Conclusions[0].Contradictions), len(back.Contradictions))
	}
}

func TestJoinR1_HazardLinksSurviveRoundTrip(t *testing.T) {
	summary, _ := JoinR1CrossLane([]R1LaneInput{
		{LaneID: "lane-a", Findings: []R1LaneFinding{{PropertyID: "P1", Sources: []F1R1Source{{Locator: "src-a"}}}}},
	}, []R1HazardInput{{
		PropertyID:           "P1",
		HazardRef:            "HZ-1",
		SymptomRefs:          []string{"SYM-1", "SYM-2"},
		SourceLocators:       []string{"src-a"},
		ConsumingR3OptionIDs: []string{"opt-redesign"},
	}}, nil)
	if len(summary.Conclusions[0].Hazards) != 1 {
		t.Fatalf("expected 1 hazard attached, got %d", len(summary.Conclusions[0].Hazards))
	}
	h := summary.Conclusions[0].Hazards[0]
	if h.HazardRef != "HZ-1" || len(h.SymptomRefs) != 2 {
		t.Fatalf("hazard did not attach correctly: %+v", h)
	}
	// Round-trip through JSON.
	b, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	var back F1R1JoinSummary
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if len(back.Conclusions[0].Hazards) != 1 || back.Conclusions[0].Hazards[0].HazardRef != "HZ-1" {
		t.Fatalf("hazard<->symptom link lost in round-trip: %+v", back.Conclusions[0].Hazards)
	}
}

// --- shared-ancestry collapse ----------------------------------------------

func TestJoinR1_SharedAncestrySourcesCollapsed(t *testing.T) {
	// Two lanes each cite a source that descends from the same ancestry root.
	// They are NOT independent; the producer must collapse them into one.
	summary, _ := JoinR1CrossLane([]R1LaneInput{
		{LaneID: "lane-a", Findings: []R1LaneFinding{{PropertyID: "P1", Sources: []F1R1Source{{Locator: "src-a", AncestryRoots: []string{"root-1"}}}}}},
		{LaneID: "lane-b", Findings: []R1LaneFinding{{PropertyID: "P1", Sources: []F1R1Source{{Locator: "src-b", AncestryRoots: []string{"root-1"}}}}}},
	}, nil, nil)
	if len(summary.Conclusions) != 1 {
		t.Fatalf("expected 1 conclusion, got %d", len(summary.Conclusions))
	}
	c := summary.Conclusions[0]
	if len(c.Sources) != 1 {
		t.Fatalf("shared-ancestry sources must collapse to 1; got %d (double-counted as independent)", len(c.Sources))
	}
}

func TestValidateR1_SharedAncestryDoubleCountFails(t *testing.T) {
	// A hand-built conclusion that lists two sources sharing an ancestry root
	// (the producer would have collapsed them) must fail validation.
	env := canonicalF1Fixture()
	env.Entries[0].R1.Conclusions[0].Sources = []F1R1Source{
		{Locator: "src-a", AncestryRoots: []string{"root-1"}},
		{Locator: "src-b", AncestryRoots: []string{"root-1"}},
	}
	env.SemanticDigest = "" // re-derive after mutation
	d, _ := env.ComputeDigest()
	env.SemanticDigest = d
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, "shared ancestry root") {
		t.Fatalf("shared-ancestry double-count must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

// --- missing source locators -----------------------------------------------

func TestValidateR1_MissingSourceLocatorFailsOnEvidenceBearingClaim(t *testing.T) {
	env := canonicalF1Fixture()
	// An evidence-bearing conclusion (has a lane) with an empty source locator.
	env.Entries[0].R1.Conclusions[0].Sources = []F1R1Source{{Locator: ""}}
	env.SemanticDigest, _ = env.ComputeDigest()
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, "empty locator") {
		t.Fatalf("empty source locator on evidence-bearing conclusion must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

func TestValidateR1_EvidenceBearingConclusionWithoutSourcesFails(t *testing.T) {
	env := canonicalF1Fixture()
	// Drop all sources from an evidence-bearing conclusion (it still has a lane).
	env.Entries[0].R1.Conclusions[0].Sources = nil
	env.SemanticDigest, _ = env.ComputeDigest()
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, "has no sources") {
		t.Fatalf("evidence-bearing conclusion without sources must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

// --- join-disposition consistency ------------------------------------------

func TestValidateR1_MergeWithoutTwoLanesFails(t *testing.T) {
	env := canonicalF1Fixture()
	env.Entries[0].R1.Conclusions[0].JoinDisposition = F1R1JoinMerge // but only 1 lane in fixture
	env.SemanticDigest, _ = env.ComputeDigest()
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, "merge requires >=2 lanes") {
		t.Fatalf("merge without >=2 lanes must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

func TestValidateR1_DuplicatePropertyIDFails(t *testing.T) {
	env := canonicalF1Fixture()
	// Clone the conclusion so two conclusions share property R1P1 (un-merged).
	env.Entries[0].R1.Conclusions = append(env.Entries[0].R1.Conclusions, F1R1Conclusion{
		ConclusionID: "R1C1b", PropertyID: "R1P1", JoinDisposition: F1R1JoinUnion,
		Lanes: []F1R1LaneContrib{{LaneID: "lane-a"}}, Sources: []F1R1Source{{Locator: "src-a"}},
	})
	env.SemanticDigest, _ = env.ComputeDigest()
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, "duplicate property_id") {
		t.Fatalf("duplicate property_id must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

func TestValidateR1_UnknownJoinDispositionFails(t *testing.T) {
	env := canonicalF1Fixture()
	env.Entries[0].R1.Conclusions[0].JoinDisposition = "intersect"
	env.SemanticDigest, _ = env.ComputeDigest()
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, "unknown join_disposition") {
		t.Fatalf("unknown join_disposition must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

// --- hazard survival chain -------------------------------------------------

func TestValidateR1_HazardWithoutSymptomRefsFails(t *testing.T) {
	env := canonicalF1Fixture()
	env.Entries[0].R1.Conclusions[0].Hazards = []F1R1HazardLink{{HazardRef: "HZ-1"}} // no symptom_refs
	env.SemanticDigest, _ = env.ComputeDigest()
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, "hazard has no symptom_refs") {
		t.Fatalf("hazard without symptom_refs must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

func TestValidateR1_HazardSourceLocatorNotOnConclusionFails(t *testing.T) {
	env := canonicalF1Fixture()
	env.Entries[0].R1.Conclusions[0].Hazards = []F1R1HazardLink{{
		HazardRef:      "HZ-1",
		SymptomRefs:    []string{"SYM-1"},
		SourceLocators: []string{"src-a", "FABRICATED-src"}, // src-a is declared; FABRICATED-src is not
	}}
	env.SemanticDigest, _ = env.ComputeDigest()
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, "does not resolve to any declared source") {
		t.Fatalf("hazard source_locator not on the conclusion must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

func TestValidateR1_HazardConsumingR3RefDanglesFails(t *testing.T) {
	env := canonicalF1Fixture()
	env.Entries[0].R1.Conclusions[0].Hazards = []F1R1HazardLink{{
		HazardRef:            "HZ-1",
		SymptomRefs:          []string{"SYM-1"},
		SourceLocators:       []string{"src-a"},
		ConsumingR3OptionIDs: []string{"NO-SUCH-OPTION"},
	}}
	env.SemanticDigest, _ = env.ComputeDigest()
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, "does not resolve to any r3 option_id") {
		t.Fatalf("dangling hazard consuming_r3_option_id must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

// --- immutability: new event -> new cycle, no mutation of prior ------------

func TestWithNewR1Cycle_DoesNotMutatePrior(t *testing.T) {
	prior := canonicalF1Fixture()
	priorCycle := prior.SynthesisCycleID
	priorDigest := prior.SemanticDigest
	priorConclusions := prior.Entries[0].R1.Conclusions[0]

	// A new gate/verdict event produces a NEW join (a changed conclusion).
	newJoin, _ := JoinR1CrossLane([]R1LaneInput{
		{LaneID: "lane-a", Findings: []R1LaneFinding{{PropertyID: "R1P1-NEW", Sources: []F1R1Source{{Locator: "src-x"}}}}},
	}, nil, nil)
	next := WithNewR1Cycle(prior, "cycle-NEW", newJoin)

	// Prior envelope is unchanged.
	if prior.SynthesisCycleID != priorCycle {
		t.Fatalf("prior cycle_id mutated: %q -> %q", priorCycle, prior.SynthesisCycleID)
	}
	if prior.SemanticDigest != priorDigest {
		t.Fatalf("prior digest mutated: %q -> %q", priorDigest, prior.SemanticDigest)
	}
	if prior.Entries[0].R1.Conclusions[0].PropertyID != priorConclusions.PropertyID {
		t.Fatalf("prior conclusion mutated")
	}

	// New envelope has a new cycle + new digest.
	if next.SynthesisCycleID == prior.SynthesisCycleID {
		t.Fatalf("new envelope reused the prior cycle_id (immutability violation)")
	}
	if next.SemanticDigest == prior.SemanticDigest {
		t.Fatalf("new envelope has the same digest as prior (content did not actually change)")
	}
	// The new envelope's R1 entry reflects the new join.
	r1Entry := findFamilyEntry(next, F1FamilyR1CrossLaneJoin)
	if r1Entry == nil || r1Entry.R1 == nil {
		t.Fatal("new envelope's R1 entry is missing")
	}
	if r1Entry.R1.Conclusions[0].PropertyID != "R1P1-NEW" {
		t.Fatalf("new envelope R1 conclusion = %q, want R1P1-NEW", r1Entry.R1.Conclusions[0].PropertyID)
	}
	// NOTE: the new envelope is NOT asserted to be structurally complete.
	// Swapping only the R1 conclusion legitimately orphans the R3/P-a entries
	// that referenced the prior conclusion ID (their cross-refs now dangle),
	// and the validator correctly reports them. A real producer cascade
	// re-derives R3/P-a from the new R1 join (slices 3-4); this slice proves
	// only the immutability + new-cycle contract.
	if errs := validateF1EnvelopeContent(next); len(errs) == 0 {
		t.Fatalf("expected the orphaned R3/P-a cross-refs to be flagged after a partial R1 swap")
	}
}

// --- doctor: digest + hazard-link divergence detection ---------------------

func TestDoctorF1_DetectsHazardChainDivergence(t *testing.T) {
	dir := t.TempDir()
	cp := filepath.Join(dir, "docs", "checkpoints")
	if err := os.MkdirAll(cp, 0o755); err != nil {
		t.Fatal(err)
	}
	// Build a committed projection whose hazard chain is broken (a hazard
	// source_locator that does not resolve to the conclusion's declared
	// sources). The doctor must FAIL this.
	env := canonicalF1Fixture()
	env.Entries[0].R1.Conclusions[0].Hazards = []F1R1HazardLink{{
		HazardRef:      "HZ-1",
		SymptomRefs:    []string{"SYM-1"},
		SourceLocators: []string{"NOT-DECLARED-src"},
	}}
	full, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	block := "```f1-synthesis-envelope\n" + string(full) + "\n```"
	if err := os.WriteFile(filepath.Join(cp, "diverge.md"), []byte("# diverge\n\n"+block+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := checkF1EnvelopeConsistency(dir)
	if r.tier != tierFail {
		t.Fatalf("doctor tier = %q, want FAIL for a projection whose hazard links diverge from canonical; detail=%s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "does not resolve to any declared source") {
		t.Fatalf("FAIL detail must name the divergent hazard source_locator; got:\n%s", r.detail)
	}
}

func TestDoctorF1_DetectsDigestDivergence(t *testing.T) {
	dir := t.TempDir()
	cp := filepath.Join(dir, "docs", "checkpoints")
	if err := os.MkdirAll(cp, 0o755); err != nil {
		t.Fatal(err)
	}
	// A committed projection whose canonical content was edited but whose
	// stored digest was NOT re-derived. The doctor must FAIL.
	env := canonicalF1Fixture()
	env.Entries[0].R1.Conclusions[0].PropertyID = "R1P1-TAMPERED" // canonical content changed
	// env.SemanticDigest intentionally left as the pre-tamper value.
	full, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	block := "```f1-synthesis-envelope\n" + string(full) + "\n```"
	if err := os.WriteFile(filepath.Join(cp, "tamper.md"), []byte("# tamper\n\n"+block+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := checkF1EnvelopeConsistency(dir)
	if r.tier != tierFail {
		t.Fatalf("doctor tier = %q, want FAIL for a digest divergence; detail=%s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "semantic_digest mismatch") {
		t.Fatalf("FAIL detail must name the digest mismatch; got:\n%s", r.detail)
	}
}

// --- helper ----------------------------------------------------------------

func findFamilyEntry(env *F1SynthesisEnvelope, family string) *F1FamilyEntry {
	for i := range env.Entries {
		if env.Entries[i].Family == family {
			return &env.Entries[i]
		}
	}
	return nil
}

// --- B-F1: full hazard survival chain -------------------------------------

func TestValidateR1_HazardWithoutSourceLocatorsFails(t *testing.T) {
	env := canonicalF1Fixture()
	env.Entries[0].R1.Conclusions[0].Hazards = []F1R1HazardLink{{
		HazardRef: "HZ-1", SymptomRefs: []string{"SYM-1"}, // no source_locators
	}}
	env.SemanticDigest, _ = env.ComputeDigest()
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, "hazard has no source_locators") {
		t.Fatalf("hazard without source_locators must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

func TestValidateR1_HazardAncestryRootNotDeclaredFails(t *testing.T) {
	env := canonicalF1Fixture()
	env.Entries[0].R1.Conclusions[0].Sources = []F1R1Source{{Locator: "src-a", AncestryRoots: []string{"root-1"}}}
	env.Entries[0].R1.Conclusions[0].Hazards = []F1R1HazardLink{{
		HazardRef: "HZ-1", SymptomRefs: []string{"SYM-1"}, SourceLocators: []string{"src-a"},
		AncestryRoots: []string{"FABRICATED-root"}, // not among declared source ancestry
	}}
	env.SemanticDigest, _ = env.ComputeDigest()
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, "does not resolve to any declared source ancestry") {
		t.Fatalf("hazard ancestry_root not declared must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

// TestValidateR1_HazardAncestryWithNoDeclaredSourceAncestryFails pins the
// edge case where the conclusion's sources declare NO ancestry roots at all,
// yet a hazard carries a (fabricated) ancestry root. There is no "no declared
// ancestry => anything passes" escape: a non-empty hazard ancestry root must
// ALWAYS resolve to a declared source ancestry, and an empty declared set
// means nothing resolves.
func TestValidateR1_HazardAncestryWithNoDeclaredSourceAncestryFails(t *testing.T) {
	env := canonicalF1Fixture()
	// Source with a locator but NO ancestry roots.
	env.Entries[0].R1.Conclusions[0].Sources = []F1R1Source{{Locator: "src-a"}}
	env.Entries[0].R1.Conclusions[0].Hazards = []F1R1HazardLink{{
		HazardRef: "HZ-1", SymptomRefs: []string{"SYM-1"}, SourceLocators: []string{"src-a"},
		AncestryRoots: []string{"FABRICATED-root"}, // no source declares any ancestry
	}}
	env.SemanticDigest, _ = env.ComputeDigest()
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, "does not resolve to any declared source ancestry") {
		t.Fatalf("fabricated hazard ancestry with no declared source ancestry must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

func TestValidateR1_HazardContradictionRefDanglesFails(t *testing.T) {
	env := canonicalF1Fixture()
	env.Entries[0].R1.Conclusions[0].Contradictions = []F1R1Contradiction{{ContradictionID: "CONTRA-real", LaneA: "lane-a", LaneB: "lane-b"}}
	env.Entries[0].R1.Conclusions[0].Hazards = []F1R1HazardLink{{
		HazardRef: "HZ-1", SymptomRefs: []string{"SYM-1"}, SourceLocators: []string{"src-a"},
		ContradictionRef: "CONTRA-FABRICATED", // no such contradiction_id
	}}
	env.SemanticDigest, _ = env.ComputeDigest()
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, "contradiction_ref") || !hasErrContaining(errs, "does not resolve") {
		t.Fatalf("dangling hazard contradiction_ref must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

func TestValidateR1_HazardGapRefDanglesFails(t *testing.T) {
	env := canonicalF1Fixture()
	env.Entries[0].R1.Conclusions[0].Gaps = []F1R1Gap{{GapID: "GAP-real", Aspect: "coverage"}}
	env.Entries[0].R1.Conclusions[0].Hazards = []F1R1HazardLink{{
		HazardRef: "HZ-1", SymptomRefs: []string{"SYM-1"}, SourceLocators: []string{"src-a"},
		GapRef: "GAP-FABRICATED", // no such gap_id
	}}
	env.SemanticDigest, _ = env.ComputeDigest()
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, "gap_ref") || !hasErrContaining(errs, "does not resolve") {
		t.Fatalf("dangling hazard gap_ref must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

func TestValidateR1_HazardFullChainResolves(t *testing.T) {
	// A hazard whose every leg resolves to a declared source / ancestry /
	// contradiction must PASS (the survival chain is intact).
	env := canonicalF1Fixture()
	env.Entries[0].R1.Conclusions[0].Sources = []F1R1Source{{Locator: "src-a", AncestryRoots: []string{"root-1"}}}
	env.Entries[0].R1.Conclusions[0].Contradictions = []F1R1Contradiction{{ContradictionID: "CONTRA-1", LaneA: "lane-a", LaneB: "lane-b"}}
	env.Entries[0].R1.Conclusions[0].Hazards = []F1R1HazardLink{{
		HazardRef: "HZ-1", SymptomRefs: []string{"SYM-1"}, SourceLocators: []string{"src-a"},
		AncestryRoots: []string{"root-1"}, ContradictionRef: "CONTRA-1",
	}}
	env.SemanticDigest, _ = env.ComputeDigest()
	// Filter to R1-hazard-relevant errors only (ignore the orphaned R3/P-a
	// cross-refs that the contradiction's invented lane-b introduces).
	r1Errs := []string{}
	for _, e := range ValidateF1Envelope(env) {
		if strings.Contains(e, "hazards[") {
			r1Errs = append(r1Errs, e)
		}
	}
	if len(r1Errs) != 0 {
		t.Fatalf("a hazard with a fully-resolving chain must produce no hazard errors; got:\n  %s", strings.Join(r1Errs, "\n  "))
	}
}

// --- B-F2: true deep-copy (aliasing) ---------------------------------------

func TestWithNewR1Cycle_CallerJoinMutationDoesNotLeak(t *testing.T) {
	prior := canonicalF1Fixture()
	newJoin, _ := JoinR1CrossLane([]R1LaneInput{
		{LaneID: "lane-a", Findings: []R1LaneFinding{{PropertyID: "P1", Sources: []F1R1Source{{Locator: "src-x"}}}}},
	}, nil, nil)
	next := WithNewR1Cycle(prior, "cycle-NEW", newJoin)
	// Mutate the caller's newJoin AFTER the call.
	newJoin.Conclusions[0].PropertyID = "TAMPERED"
	// The returned envelope must NOT reflect the post-call mutation.
	r1 := findFamilyEntry(next, F1FamilyR1CrossLaneJoin)
	if r1.R1.Conclusions[0].PropertyID == "TAMPERED" {
		t.Fatal("WithNewR1Cycle leaked a caller mutation into the returned envelope (not a true deep-copy)")
	}
}

func TestWithNewR1Cycle_EntrySourceRefsNotShared(t *testing.T) {
	prior := canonicalF1Fixture()
	// Give a non-R1 entry a SourceRefs slice to probe aliasing.
	prior.Entries[1].SourceRefs = []string{"orig-ref"}
	newJoin, _ := JoinR1CrossLane([]R1LaneInput{
		{LaneID: "lane-a", Findings: []R1LaneFinding{{PropertyID: "P1", Sources: []F1R1Source{{Locator: "src-x"}}}}},
	}, nil, nil)
	next := WithNewR1Cycle(prior, "cycle-NEW", newJoin)
	// Mutate the new envelope's non-R1 entry SourceRefs.
	next.Entries[1].SourceRefs[0] = "MUTATED"
	// The prior envelope must be unaffected.
	if prior.Entries[1].SourceRefs[0] != "orig-ref" {
		t.Fatalf("prior entry SourceRefs was mutated via shared backing array: %q", prior.Entries[1].SourceRefs[0])
	}
}

// --- B-F3: disposition from distinct lanes ---------------------------------

func TestJoinR1_OneLaneTwoFindingsSamePropertyIsUnion(t *testing.T) {
	// One lane contributing two findings on the same property must NOT become
	// a (validator-invalid) single-lane MERGE — it is a UNION of one lane.
	summary, perr := JoinR1CrossLane([]R1LaneInput{
		{LaneID: "lane-a", Findings: []R1LaneFinding{
			{PropertyID: "P1", Sources: []F1R1Source{{Locator: "src-a"}}},
			{PropertyID: "P1", Sources: []F1R1Source{{Locator: "src-b"}}},
		}},
	}, nil, nil)
	if len(perr) != 0 {
		t.Fatalf("producer errors: %v", perr)
	}
	if len(summary.Conclusions) != 1 {
		t.Fatalf("expected 1 conclusion, got %d", len(summary.Conclusions))
	}
	c := summary.Conclusions[0]
	if c.JoinDisposition != F1R1JoinUnion {
		t.Fatalf("join_disposition = %q, want union (one distinct lane)", c.JoinDisposition)
	}
	if len(c.Lanes) != 1 {
		t.Fatalf("expected 1 distinct lane, got %d", len(c.Lanes))
	}
}

// --- B-F4: producer purity (inputs not mutated) ---------------------------

func TestJoinR1_DoesNotMutateInputSlices(t *testing.T) {
	hazards := []R1HazardInput{
		{PropertyID: "P2", HazardRef: "HZ-2"},
		{PropertyID: "P1", HazardRef: "HZ-1"}, // deliberately out of order
	}
	gaps := []R1GapInput{
		{PropertyID: "P2", Aspect: "z-aspect"},
		{PropertyID: "P1", Aspect: "a-aspect"}, // deliberately out of order
	}
	hazardsBefore := []string{hazards[0].PropertyID, hazards[1].PropertyID}
	gapsBefore := []string{gaps[0].PropertyID, gaps[1].PropertyID}
	_, _ = JoinR1CrossLane([]R1LaneInput{
		{LaneID: "lane-a", Findings: []R1LaneFinding{{PropertyID: "P1", Sources: []F1R1Source{{Locator: "src-a"}}}}},
	}, hazards, gaps)
	if hazards[0].PropertyID != hazardsBefore[0] || hazards[1].PropertyID != hazardsBefore[1] {
		t.Fatalf("JoinR1CrossLane mutated the caller's hazards slice order: got %q,%q want %q,%q",
			hazards[0].PropertyID, hazards[1].PropertyID, hazardsBefore[0], hazardsBefore[1])
	}
	if gaps[0].PropertyID != gapsBefore[0] || gaps[1].PropertyID != gapsBefore[1] {
		t.Fatalf("JoinR1CrossLane mutated the caller's gaps slice order: got %q,%q want %q,%q",
			gaps[0].PropertyID, gaps[1].PropertyID, gapsBefore[0], gapsBefore[1])
	}
}

// TestJoinR1_SourceAncestryRootsNotAliased pins the single-source purity
// defect (BLK-1): the producer must not alias the caller's AncestryRoots
// backing array. A single-source conclusion (len(all)<=1 path) used to return
// the caller's slice header directly; mutating the caller's input after the
// call retroactively changed the immutable summary. The union-find rebuild now
// allocates fresh AncestryRoots even for a single source.
func TestJoinR1_SourceAncestryRootsNotAliased(t *testing.T) {
	callerRoots := []string{"root-1", "root-2"}
	summary, _ := JoinR1CrossLane([]R1LaneInput{
		{LaneID: "lane-a", Findings: []R1LaneFinding{{
			PropertyID: "P1",
			Sources:    []F1R1Source{{Locator: "src-a", AncestryRoots: callerRoots}},
		}}},
	}, nil, nil)
	producedRoots := summary.Conclusions[0].Sources[0].AncestryRoots
	// Mutate the caller's slice AFTER the producer call.
	callerRoots[0] = "MUTATED"
	if producedRoots[0] == "MUTATED" {
		t.Fatal("producer aliased the caller's AncestryRoots backing array (single-source path is not a true deep-copy)")
	}
	if producedRoots[0] != "root-1" {
		t.Fatalf("produced AncestryRoots[0] = %q, want root-1 (unaffected by post-call caller mutation)", producedRoots[0])
	}
}

// --- D-F1: transitive ancestry collapse ------------------------------------

func TestJoinR1_TransitiveAncestryCollapse(t *testing.T) {
	// X{root-1}, Y{root-2}, Z{root-1,root-2}: Z shares root-1 with X and
	// root-2 with Y, so all three are one transitive independence class and
	// must collapse to a single source (NOT trip the validator's double-count).
	summary, _ := JoinR1CrossLane([]R1LaneInput{
		{LaneID: "lx", Findings: []R1LaneFinding{{PropertyID: "P1", Sources: []F1R1Source{{Locator: "src-x", AncestryRoots: []string{"root-1"}}}}}},
		{LaneID: "ly", Findings: []R1LaneFinding{{PropertyID: "P1", Sources: []F1R1Source{{Locator: "src-y", AncestryRoots: []string{"root-2"}}}}}},
		{LaneID: "lz", Findings: []R1LaneFinding{{PropertyID: "P1", Sources: []F1R1Source{{Locator: "src-z", AncestryRoots: []string{"root-1", "root-2"}}}}}},
	}, nil, nil)
	if len(summary.Conclusions[0].Sources) != 1 {
		t.Fatalf("transitive ancestry class must collapse to 1 source; got %d: %+v", len(summary.Conclusions[0].Sources), summary.Conclusions[0].Sources)
	}
}

// --- producer -> validator round-trip property -----------------------------

// TestJoinR1_ProducerOutputPassesValidator is the property guard that would
// have caught B-F3 (producer emits validator-invalid MERGE) and D-F1
// (transitive-ancestry producer output trips the double-count check). For a
// spread of inputs, the producer's output MUST pass validateR1Summary — a
// producer that emits structurally-invalid output is a self-inconsistency.
func TestJoinR1_ProducerOutputPassesValidator(t *testing.T) {
	cases := []struct {
		name    string
		lanes   []R1LaneInput
		hazards []R1HazardInput
		gaps    []R1GapInput
	}{
		{"merge two lanes same property", []R1LaneInput{
			{LaneID: "lane-a", Findings: []R1LaneFinding{{PropertyID: "P1", Position: "agree", Sources: []F1R1Source{{Locator: "src-a"}}}}},
			{LaneID: "lane-b", Findings: []R1LaneFinding{{PropertyID: "P1", Position: "agree", Sources: []F1R1Source{{Locator: "src-b"}}}}},
		}, nil, nil},
		{"one lane two findings same property", []R1LaneInput{
			{LaneID: "lane-a", Findings: []R1LaneFinding{
				{PropertyID: "P1", Sources: []F1R1Source{{Locator: "src-a"}}},
				{PropertyID: "P1", Sources: []F1R1Source{{Locator: "src-b"}}},
			}},
		}, nil, nil},
		{"transitive ancestry", []R1LaneInput{
			{LaneID: "lx", Findings: []R1LaneFinding{{PropertyID: "P1", Sources: []F1R1Source{{Locator: "src-x", AncestryRoots: []string{"root-1"}}}}}},
			{LaneID: "ly", Findings: []R1LaneFinding{{PropertyID: "P1", Sources: []F1R1Source{{Locator: "src-y", AncestryRoots: []string{"root-2"}}}}}},
			{LaneID: "lz", Findings: []R1LaneFinding{{PropertyID: "P1", Sources: []F1R1Source{{Locator: "src-z", AncestryRoots: []string{"root-1", "root-2"}}}}}},
		}, nil, nil},
		{"distinct properties union", []R1LaneInput{
			{LaneID: "lane-a", Findings: []R1LaneFinding{{PropertyID: "P1", Sources: []F1R1Source{{Locator: "src-a"}}}}},
			{LaneID: "lane-b", Findings: []R1LaneFinding{{PropertyID: "P2", Sources: []F1R1Source{{Locator: "src-b"}}}}},
		}, nil, nil},
		{"contradiction plus hazard", []R1LaneInput{
			{LaneID: "lane-a", Findings: []R1LaneFinding{{PropertyID: "P1", Position: "pass", Sources: []F1R1Source{{Locator: "src-a"}}}}},
			{LaneID: "lane-b", Findings: []R1LaneFinding{{PropertyID: "P1", Position: "fail", Sources: []F1R1Source{{Locator: "src-b"}}}}},
		}, []R1HazardInput{{PropertyID: "P1", HazardRef: "HZ-1", SymptomRefs: []string{"SYM-1"}, SourceLocators: []string{"src-a", "src-b"}}}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			summary, perr := JoinR1CrossLane(tc.lanes, tc.hazards, tc.gaps)
			if len(perr) != 0 {
				t.Fatalf("producer errors: %v", perr)
			}
			if errs := validateR1Summary("entry.r1", summary); len(errs) != 0 {
				t.Fatalf("producer output must pass the validator (self-consistency); got:\n  %s", strings.Join(errs, "\n  "))
			}
		})
	}
}
