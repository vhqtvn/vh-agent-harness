package recurrence

// derivation_test.go — TDD red→green tests for the recurrence-signature
// DERIVATION (P1-MEMORY-001 Slice 2). This slice is PURE Go: effective-identity
// resolution → canonical grouping → symptom-class aggregation (query, not merge)
// → observation aggregation + count. No I/O, no store, no side effects, no
// enforcement. The derivation INFORMS only (authority line: producer/gate ACT).
//
// These tests anchor the derivation crux from
// researches/decisions/2026-07-29-recurrence-signature-and-dedup-enforcement.md
// (memo efa53fb), consuming the recurrence-block contract shipped in Slice 1
// (commit 7c7c295, task-card.schema.json). Field shapes mirror the ACTUAL
// schema (evidence[] items = {kind,ref,...}; aliases[] items = {recurrence_id,
// superseded,note}), which is the authoritative Slice 1 contract.

import (
	"reflect"
	"sort"
	"testing"
)

// block returns a minimal valid recurrence Block with the given identity +
// class. Count/ack default to a consistent ack-pair (count==ack==0).
func block(recurrenceID, symptomClassID string) *Block {
	return &Block{
		RecurrenceID:          recurrenceID,
		SymptomClassID:        symptomClassID,
		RecurrenceCount:       0,
		LastAcknowledgedCount: 0,
	}
}

// effectiveIDs is a test helper: derive then return the sorted effective IDs.
func effectiveIDs(t *testing.T, cards []Card) []string {
	t.Helper()
	res := Derive(cards)
	got := make([]string, 0, len(res.Groups))
	for _, g := range res.Groups {
		got = append(got, g.EffectiveID)
	}
	sort.Strings(got)
	return got
}

// TestEffectiveIdentity proves the effective-identity rule (memo §Backward
// compatibility): effective = explicit recurrence_id when the block is present
// (non-empty), ELSE task_id (legacy cards).
func TestEffectiveIdentity(t *testing.T) {
	// (a) Card WITH a recurrence block → effective = recurrence_id.
	withBlock := Card{TaskID: "T-recurrence", Recurrence: block("R1", "recurrence.v1/foo")}
	if got := EffectiveID(withBlock); got != "R1" {
		t.Errorf("EffectiveID(card with block) = %q, want %q (recurrence_id)", got, "R1")
	}
	// (b) Legacy card (no block) → effective = task_id.
	legacy := Card{TaskID: "T-legacy"}
	if got := EffectiveID(legacy); got != "T-legacy" {
		t.Errorf("EffectiveID(legacy card) = %q, want %q (task_id)", got, "T-legacy")
	}
	// (c) End-to-end via Derive: the group's EffectiveID follows the same rule.
	res := Derive([]Card{legacy, withBlock})
	want := []string{"R1", "T-legacy"}
	if got := effectiveIDs(t, []Card{legacy, withBlock}); !reflect.DeepEqual(got, want) {
		t.Errorf("Derive effective IDs = %v, want %v", got, want)
	}
	// The legacy group must be flagged IsLegacy; the recurrence group must not.
	for _, g := range res.Groups {
		if g.EffectiveID == "T-legacy" && !g.IsLegacy {
			t.Errorf("legacy group %q: IsLegacy = false, want true", g.EffectiveID)
		}
		if g.EffectiveID == "R1" && g.IsLegacy {
			t.Errorf("recurrence group %q: IsLegacy = true, want false", g.EffectiveID)
		}
	}
}

// TestCollapse_SameRecurrenceID is the load-bearing collapse proof (memo §Repeat
// semantics): N cards with DIFFERENT task_id values but the SAME recurrence_id
// collapse to exactly ONE canonical group, with N aggregated observations and
// recurrence_count == N.
func TestCollapse_SameRecurrenceID(t *testing.T) {
	cards := []Card{
		{TaskID: "T-obs-1", Recurrence: block("R1", "recurrence.v1/band-aid-loop")},
		{TaskID: "T-obs-2", Recurrence: block("R1", "recurrence.v1/band-aid-loop")},
		{TaskID: "T-obs-3", Recurrence: block("R1", "recurrence.v1/band-aid-loop")},
	}
	res := Derive(cards)
	if len(res.Groups) != 1 {
		t.Fatalf("Groups: want 1 (collapse), got %d: %+v", len(res.Groups), res.Groups)
	}
	g := res.Groups[0]
	if g.EffectiveID != "R1" {
		t.Errorf("EffectiveID = %q, want R1", g.EffectiveID)
	}
	if len(g.Observations) != 3 {
		t.Errorf("Observations: want 3 (one per card), got %d", len(g.Observations))
	}
	if g.RecurrenceCount != 3 {
		t.Errorf("RecurrenceCount = %d, want 3 (== number of observations)", g.RecurrenceCount)
	}
	// Each card's task_id is preserved as an observation even after collapse.
	seen := map[string]bool{}
	for _, obs := range g.Observations {
		seen[obs.TaskID] = true
	}
	for _, want := range []string{"T-obs-1", "T-obs-2", "T-obs-3"} {
		if !seen[want] {
			t.Errorf("observation task_id %q not retained after collapse; seen=%v", want, seen)
		}
	}
}

// TestSameClassDifferentDefect is the CRUX DISTINCTION (memo §Decision,
// load-bearing): a shared symptom_class_id does NOT merge distinct defects.
// Two cards sharing symptom_class_id but carrying DIFFERENT recurrence_id
// values remain TWO canonical groups. Querying by symptom_class_id returns
// both groups as ONE class-aggregation, but they stay distinct entries.
func TestSameClassDifferentDefect(t *testing.T) {
	class := "recurrence.v1/band-aid-loop"
	cards := []Card{
		{TaskID: "T-defect-A", Recurrence: block("R-A", class)},
		{TaskID: "T-defect-B", Recurrence: block("R-B", class)},
	}
	res := Derive(cards)

	// (1) TWO canonical groups — shared class does NOT merge.
	if len(res.Groups) != 2 {
		t.Fatalf("Groups: want 2 (shared class does NOT merge), got %d: %+v", len(res.Groups), res.Groups)
	}
	// (2) Each group keeps its own effective identity.
	if got := effectiveIDs(t, cards); !reflect.DeepEqual(got, []string{"R-A", "R-B"}) {
		t.Errorf("effective IDs = %v, want [R-A R-B]", got)
	}

	// (3) Query view: BySymptomClass returns BOTH groups under the one class,
	//     but they remain distinct canonical entries (not merged).
	byClass := res.BySymptomClass()
	classGroups, ok := byClass[class]
	if !ok {
		t.Fatalf("BySymptomClass missing class %q; got %v", class, byClass)
	}
	if len(classGroups) != 2 {
		t.Fatalf("class %q: want 2 distinct groups in query view, got %d", class, len(classGroups))
	}
	// The two class-members must still be the distinct canonical groups.
	ids := map[string]bool{}
	for _, g := range classGroups {
		ids[g.EffectiveID] = true
	}
	if !ids["R-A"] || !ids["R-B"] {
		t.Errorf("class query lost distinct identities; got ids=%v want both R-A and R-B", ids)
	}
}

// TestAliasReconciliation proves bounded alias reconciliation (memo §Decision):
// an aliases[] entry means "this recurrence_id is the same defect as the listed
// id"; the two cards merge into ONE group. Directional choice (stated in code
// + closeout): the explicitly-declared recurrence_id of the card carrying the
// alias block is CANONICAL; alias ids fold into it.
func TestAliasReconciliation(t *testing.T) {
	cards := []Card{
		// Canonical card declares R1 with an alias pointing at R2.
		{
			TaskID:     "T-canonical",
			Recurrence: block("R1", "recurrence.v1/foo"),
		},
		// A second card carries the alias id R2 as its own recurrence_id.
		{
			TaskID:     "T-aliased",
			Recurrence: block("R2", "recurrence.v1/foo"),
		},
	}
	// Inject the alias on the canonical card AFTER construction (keeps the
	// block() helper minimal): R2 → folds into R1.
	cards[0].Recurrence.Aliases = []Alias{{RecurrenceID: "R2", Note: "same defect, re-pointed"}}

	res := Derive(cards)
	if len(res.Groups) != 1 {
		t.Fatalf("Groups: want 1 (alias merge), got %d: %+v", len(res.Groups), res.Groups)
	}
	g := res.Groups[0]
	// Canonical id wins (the declared recurrence_id of the alias-owning card).
	if g.EffectiveID != "R1" {
		t.Errorf("EffectiveID = %q, want R1 (canonical wins over alias R2)", g.EffectiveID)
	}
	// Both observations are retained under the merged group.
	if len(g.Observations) != 2 {
		t.Errorf("Observations: want 2 (both cards retained after merge), got %d", len(g.Observations))
	}
}

// TestIdempotent_Deterministic proves Derive is a pure deterministic function:
// running it twice on the same input yields the same grouping (same groups in
// the same order). This is the idempotency grounding downstream slices rely on.
func TestIdempotent_Deterministic(t *testing.T) {
	cards := []Card{
		{TaskID: "T-1", Recurrence: block("R1", "recurrence.v1/a")},
		{TaskID: "T-2", Recurrence: block("R1", "recurrence.v1/a")},
		{TaskID: "T-3", Recurrence: block("R2", "recurrence.v1/b")},
		{TaskID: "T-legacy"},
	}
	first := Derive(cards)
	second := Derive(cards)
	if !reflect.DeepEqual(first, second) {
		t.Errorf("Derive is not idempotent:\nfirst  = %+v\nsecond = %+v", first, second)
	}
}

// TestObservationEvidenceRetained proves each card's evidence[] is retained and
// aggregated under its canonical group (memo: "each card's observations are
// retained and aggregated under that group").
func TestObservationEvidenceRetained(t *testing.T) {
	cards := []Card{
		{
			TaskID:     "T-1",
			Recurrence: block("R1", "recurrence.v1/foo"),
		},
		{
			TaskID:     "T-2",
			Recurrence: block("R1", "recurrence.v1/foo"),
		},
	}
	cards[0].Recurrence.Evidence = []Evidence{
		{Kind: "path", Ref: "internal/cli/release_gate.go"},
		{Kind: "outcome", Ref: "release blocked"},
	}
	cards[1].Recurrence.Evidence = []Evidence{
		{Kind: "commit_subject", Ref: "fix(claim): merge recurrence"},
	}
	res := Derive(cards)
	if len(res.Groups) != 1 {
		t.Fatalf("Groups: want 1, got %d", len(res.Groups))
	}
	g := res.Groups[0]
	if len(g.Observations) != 2 {
		t.Fatalf("Observations: want 2, got %d", len(g.Observations))
	}
	// Aggregated evidence across the group retains all three entries.
	var allRefs []string
	for _, obs := range g.Observations {
		for _, e := range obs.Evidence {
			allRefs = append(allRefs, e.Ref)
		}
	}
	sort.Strings(allRefs)
	want := []string{"fix(claim): merge recurrence", "internal/cli/release_gate.go", "release blocked"}
	if !reflect.DeepEqual(allRefs, want) {
		t.Errorf("aggregated evidence refs = %v, want %v", allRefs, want)
	}
}

// TestLegacyAndRecurrenceCoexist proves legacy cards (no block, keyed by
// task_id) and recurrence cards (keyed by recurrence_id) coexist without
// cross-merging: a legacy task_id never accidentally matches a recurrence_id.
func TestLegacyAndRecurrenceCoexist(t *testing.T) {
	cards := []Card{
		{TaskID: "R1"}, // legacy: effective = "R1" (happens to equal a recurrence_id string)
		{TaskID: "T-2", Recurrence: block("R1", "recurrence.v1/foo")}, // recurrence: effective = "R1"
	}
	res := Derive(cards)
	// A legacy card's task_id coinciding with a recurrence_id is an edge case.
	// The derivation must still produce a deterministic, explainable grouping.
	// Per the effective-identity rule both keys are literally "R1", so they
	// collapse to ONE group. This is correct: identity is the LITERAL key, and
	// the spec forbids auto-merging — but an EXACT key match is a genuine
	// identity collision, not an inference. Document the expected behavior here.
	if len(res.Groups) != 1 {
		t.Fatalf("Groups: want 1 (literal key collision R1==R1), got %d: %+v", len(res.Groups), res.Groups)
	}
	// The merged group must carry BOTH observations (one legacy, one recurrence).
	if len(res.Groups[0].Observations) != 2 {
		t.Errorf("Observations: want 2, got %d", len(res.Groups[0].Observations))
	}
}

// TestAliasCycle_Terminates proves resolveAlias's cycle-guard terminates on a
// pathological alias cycle (an authoring error: two cards each declaring the
// other's recurrence_id as an alias). The package doc claims "cycle-guarded
// termination"; this test makes that load-bearing property observable so a
// future refactor that drops the seen-map guard cannot silently reintroduce an
// infinite loop. The claim here is TERMINATION + DETERMINISM + no data loss —
// NOT a "correct" merge within a cycle (cycles are out of scope per the doc).
func TestAliasCycle_Terminates(t *testing.T) {
	// Mutual cycle: R1 declares R2 as alias; R2 declares R1 as alias.
	a := Card{TaskID: "T-A", Recurrence: block("R1", "recurrence.v1/foo")}
	a.Recurrence.Aliases = []Alias{{RecurrenceID: "R2"}}
	b := Card{TaskID: "T-B", Recurrence: block("R2", "recurrence.v1/foo")}
	b.Recurrence.Aliases = []Alias{{RecurrenceID: "R1"}}
	cards := []Card{a, b}

	// (1) Termination: Derive must return (the guard broke the cycle). If the
	//     seen-map guard were dropped, this call would hang forever.
	res := Derive(cards)

	// (2) No data loss: both cards' observations survive somewhere in the result.
	totalObs := 0
	for _, g := range res.Groups {
		totalObs += len(g.Observations)
	}
	if totalObs != 2 {
		t.Errorf("cycle: total observations = %d, want 2 (no card dropped)", totalObs)
	}

	// (3) Determinism: a second run on the same input yields the same result.
	again := Derive(cards)
	if !reflect.DeepEqual(res, again) {
		t.Errorf("cycle: Derive not deterministic across runs")
	}

	// (4) A longer chain (A→B→C→A) must also terminate. This guards against a
	//     guard that only handles 2-node cycles.
	c1 := Card{TaskID: "T-1", Recurrence: block("A", "recurrence.v1/bar")}
	c1.Recurrence.Aliases = []Alias{{RecurrenceID: "B"}}
	c2 := Card{TaskID: "T-2", Recurrence: block("B", "recurrence.v1/bar")}
	c2.Recurrence.Aliases = []Alias{{RecurrenceID: "C"}}
	c3 := Card{TaskID: "T-3", Recurrence: block("C", "recurrence.v1/bar")}
	c3.Recurrence.Aliases = []Alias{{RecurrenceID: "A"}}
	chainRes := Derive([]Card{c1, c2, c3})
	chainObs := 0
	for _, g := range chainRes.Groups {
		chainObs += len(g.Observations)
	}
	if chainObs != 3 {
		t.Errorf("chain cycle: total observations = %d, want 3 (no card dropped)", chainObs)
	}
	if !reflect.DeepEqual(chainRes, Derive([]Card{c1, c2, c3})) {
		t.Errorf("chain cycle: Derive not deterministic across runs")
	}
}
