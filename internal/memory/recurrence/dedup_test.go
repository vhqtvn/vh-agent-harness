package recurrence

// dedup_test.go — TDD red→green tests for the recurrence-signature PRODUCER/
// DEDUP decision (P1-MEMORY-001 Slice 3). Slice 2 proved the PURE derivation
// collapses N cards into canonical groups; Slice 3 is the WRITE-LAYER crux: on
// a recognized repeat the producer updates the canonical entry instead of
// spawning a new card. The keystone is ResolveRecurrence — the pure decision
// the producer (JS) consults at the task-writing boundary via a Go subcommand.
//
// Authority line (memo efa53fb, §Placement + §Authority-line engagement): the
// producer provides synchronous merge CONVENIENCE and APPLIES the derivation's
// decision; neither the derivation nor the producer is transition authority
// (Slice 5 release gate ACTS / fails closed). This package INFORMS only.
//
// Repeat semantics (memo §Repeat semantics + §Manifest-v2 disposition
// interaction): on a repeat, append a structured recurrence observation to the
// canonical (do NOT spawn a child card), increment recurrence_count, and leave
// last_acknowledged_count unchanged so the disposition becomes unacknowledged
// (recurrence_count > last_acknowledged_count). The canonical's recurrence_id
// is retained; the repeat's evidence folds in.

import (
	"reflect"
	"sort"
	"testing"
)

// canonicalBlock builds a recurrence Block for an EXISTING canonical card with
// an explicit count + ack pair, carrying optional evidence.
func canonicalBlock(recurrenceID, class string, count, ack int, evidence ...Evidence) *Block {
	return &Block{
		RecurrenceID:          recurrenceID,
		SymptomClassID:        class,
		RecurrenceCount:       count,
		LastAcknowledgedCount: ack,
		Evidence:              evidence,
	}
}

// incomingEvidence returns the evidence entries a repeat card would carry.
func incomingEvidence(refs ...string) []Evidence {
	out := make([]Evidence, 0, len(refs))
	for _, r := range refs {
		out = append(out, Evidence{Kind: "path", Ref: r})
	}
	return out
}

// mergedEvidenceRefs flattens a block's evidence to sorted refs for assertion.
func mergedEvidenceRefs(b *Block) []string {
	if b == nil {
		return nil
	}
	refs := make([]string, 0, len(b.Evidence))
	for _, e := range b.Evidence {
		refs = append(refs, e.Kind+":"+e.Ref)
	}
	sort.Strings(refs)
	return refs
}

// TestResolve_NewCard_NoMatch proves the no-match branch: an incoming card
// whose effective id is NOT present among existing cards resolves to NewCard.
// The producer then writes a fresh canonical card.
func TestResolve_NewCard_NoMatch(t *testing.T) {
	existing := []Card{
		{TaskID: "T-canonical", Recurrence: canonicalBlock("R1", "recurrence.v1/foo", 1, 1)},
	}
	incoming := Card{TaskID: "T-new", Recurrence: &Block{
		RecurrenceID: "R2", SymptomClassID: "recurrence.v1/bar",
		RecurrenceCount: 1, LastAcknowledgedCount: 1,
	}}
	dec := ResolveRecurrence(existing, incoming)
	if dec.Action != NewCard {
		t.Errorf("Action = %v, want NewCard (no match)", dec.Action)
	}
	if dec.EffectiveID != "R2" {
		t.Errorf("EffectiveID = %q, want R2", dec.EffectiveID)
	}
	if dec.CanonicalTaskID != "" {
		t.Errorf("CanonicalTaskID = %q, want empty on NewCard", dec.CanonicalTaskID)
	}
	if dec.Merged != nil {
		t.Errorf("Merged = %+v, want nil on NewCard", dec.Merged)
	}
}

// TestResolve_Merge_SameRecurrenceID is the LOAD-BEARING write-path crux (memo
// §Repeat semantics): an incoming card with a DIFFERENT task_id but the SAME
// recurrence_id as an existing canonical resolves to Merge. The merged block
// retains the canonical recurrence_id, bumps recurrence_count (N→N+1), folds in
// the incoming evidence, appends a structured recurrence_observation recording
// the incoming task_id, and leaves last_acknowledged_count unchanged.
func TestResolve_Merge_SameRecurrenceID(t *testing.T) {
	existing := []Card{
		{TaskID: "T-canonical", Recurrence: canonicalBlock(
			"R1", "recurrence.v1/foo", 1, 1,
			Evidence{Kind: "path", Ref: "internal/cli/release_gate.go"},
		)},
	}
	incoming := Card{
		TaskID: "T-repeat",
		Recurrence: &Block{
			RecurrenceID: "R1", SymptomClassID: "recurrence.v1/foo",
			RecurrenceCount: 1, LastAcknowledgedCount: 1,
			Evidence: incomingEvidence("internal/memory/claims/claim.go"),
		},
	}
	dec := ResolveRecurrence(existing, incoming)
	if dec.Action != Merge {
		t.Fatalf("Action = %v, want Merge (same recurrence_id)", dec.Action)
	}
	if dec.CanonicalTaskID != "T-canonical" {
		t.Errorf("CanonicalTaskID = %q, want T-canonical", dec.CanonicalTaskID)
	}
	if dec.Merged == nil {
		t.Fatal("Merged block is nil on Merge")
	}
	// Canonical recurrence_id + class retained (NOT the incoming's, even though
	// equal here — the canonical's identity wins).
	if dec.Merged.RecurrenceID != "R1" {
		t.Errorf("Merged.RecurrenceID = %q, want R1 (canonical retained)", dec.Merged.RecurrenceID)
	}
	if dec.Merged.SymptomClassID != "recurrence.v1/foo" {
		t.Errorf("Merged.SymptomClassID = %q, want retained canonical class", dec.Merged.SymptomClassID)
	}
	// Count bumped N→N+1.
	if dec.Merged.RecurrenceCount != 2 {
		t.Errorf("Merged.RecurrenceCount = %d, want 2 (1→2)", dec.Merged.RecurrenceCount)
	}
	// Ack pair unchanged → disposition now unacknowledged (count 2 > ack 1).
	if dec.Merged.LastAcknowledgedCount != 1 {
		t.Errorf("Merged.LastAcknowledgedCount = %d, want 1 (unchanged → unacknowledged)",
			dec.Merged.LastAcknowledgedCount)
	}
	if dec.Merged.RecurrenceCount <= dec.Merged.LastAcknowledgedCount {
		t.Errorf("ack-pair not invalidated: count %d <= ack %d",
			dec.Merged.RecurrenceCount, dec.Merged.LastAcknowledgedCount)
	}
	// Incoming evidence folded in + a recurrence_observation marker recording
	// the incoming task_id is appended.
	gotRefs := mergedEvidenceRefs(dec.Merged)
	wantRefs := []string{
		"path:internal/cli/release_gate.go",             // canonical's existing evidence
		"path:internal/memory/claims/claim.go",          // incoming's folded evidence
		EvidenceKindRecurrenceObservation + ":T-repeat", // structured repeat observation
	}
	sort.Strings(wantRefs)
	if !reflect.DeepEqual(gotRefs, wantRefs) {
		t.Errorf("Merged evidence = %v, want %v", gotRefs, wantRefs)
	}
}

// TestResolve_DifferentRecurrenceID_NewCard is the CRUX DISTINCTION at the
// write layer (memo §Decision): a shared symptom_class_id does NOT cause a
// merge. Two cards sharing a class but carrying DIFFERENT recurrence_id values
// stay separate — the second is a NewCard, not a Merge.
func TestResolve_DifferentRecurrenceID_NewCard(t *testing.T) {
	class := "recurrence.v1/band-aid-loop"
	existing := []Card{
		{TaskID: "T-defect-A", Recurrence: canonicalBlock("R-A", class, 1, 1)},
	}
	incoming := Card{TaskID: "T-defect-B", Recurrence: &Block{
		RecurrenceID: "R-B", SymptomClassID: class,
		RecurrenceCount: 1, LastAcknowledgedCount: 1,
	}}
	dec := ResolveRecurrence(existing, incoming)
	if dec.Action != NewCard {
		t.Errorf("Action = %v, want NewCard (shared class must NOT merge distinct defects)", dec.Action)
	}
	if dec.EffectiveID != "R-B" {
		t.Errorf("EffectiveID = %q, want R-B", dec.EffectiveID)
	}
}

// TestResolve_LegacyIncoming_NewCard proves the legacy fallback (memo §Backward
// compatibility): an incoming card with NO recurrence block never matches — its
// effective id is its own task_id, which no existing card shares (task_ids are
// unique filenames). Legacy writes pass through as NewCard.
func TestResolve_LegacyIncoming_NewCard(t *testing.T) {
	existing := []Card{
		{TaskID: "T-canonical", Recurrence: canonicalBlock("R1", "recurrence.v1/foo", 1, 1)},
	}
	// Legacy incoming: no block. Even though "T-legacy" is a brand-new id, the
	// point is the dedup never merges a legacy card into a recurrence canonical.
	legacy := Card{TaskID: "T-legacy"}
	dec := ResolveRecurrence(existing, legacy)
	if dec.Action != NewCard {
		t.Errorf("Action = %v, want NewCard (legacy incoming never matches)", dec.Action)
	}
	if dec.EffectiveID != "T-legacy" {
		t.Errorf("EffectiveID = %q, want T-legacy (task_id fallback)", dec.EffectiveID)
	}
}

// TestResolve_Merge_AckInvalidated proves the disposition-invalidation rule in
// isolation (memo §Manifest-v2 disposition interaction): a fully-acknowledged
// canonical (count==ack) becomes unacknowledged after one repeat merge, because
// count increments while ack is held.
func TestResolve_Merge_AckInvalidated(t *testing.T) {
	existing := []Card{
		{TaskID: "T-c", Recurrence: canonicalBlock("R1", "recurrence.v1/x", 3, 3)}, // fully acked
	}
	incoming := Card{TaskID: "T-repeat", Recurrence: &Block{
		RecurrenceID: "R1", SymptomClassID: "recurrence.v1/x",
		RecurrenceCount: 1, LastAcknowledgedCount: 1,
	}}
	dec := ResolveRecurrence(existing, incoming)
	if dec.Action != Merge {
		t.Fatalf("Action = %v, want Merge", dec.Action)
	}
	if dec.Merged.RecurrenceCount != 4 {
		t.Errorf("RecurrenceCount = %d, want 4 (3→4)", dec.Merged.RecurrenceCount)
	}
	if dec.Merged.LastAcknowledgedCount != 3 {
		t.Errorf("LastAcknowledgedCount = %d, want 3 (held)", dec.Merged.LastAcknowledgedCount)
	}
	if dec.Merged.RecurrenceCount <= dec.Merged.LastAcknowledgedCount {
		t.Errorf("ack-pair not invalidated: count %d <= ack %d",
			dec.Merged.RecurrenceCount, dec.Merged.LastAcknowledgedCount)
	}
}

// TestResolve_Merge_Alias proves the dedup is alias-aware (memo §Decision,
// bounded reconciliation): an incoming card whose recurrence_id is an ALIAS of
// an existing canonical merges INTO that canonical (the canonical's declared
// recurrence_id wins). This consumes the SAME alias logic as Derive.
func TestResolve_Merge_Alias(t *testing.T) {
	canonical := Card{TaskID: "T-canonical", Recurrence: canonicalBlock("R1", "recurrence.v1/foo", 1, 1)}
	canonical.Recurrence.Aliases = []Alias{{RecurrenceID: "R2", Note: "re-pointed"}}
	existing := []Card{canonical}
	// Incoming carries the alias id R2 as its own recurrence_id.
	incoming := Card{TaskID: "T-aliased", Recurrence: &Block{
		RecurrenceID: "R2", SymptomClassID: "recurrence.v1/foo",
		RecurrenceCount: 1, LastAcknowledgedCount: 1,
	}}
	dec := ResolveRecurrence(existing, incoming)
	if dec.Action != Merge {
		t.Fatalf("Action = %v, want Merge (alias reconciles into canonical)", dec.Action)
	}
	if dec.CanonicalTaskID != "T-canonical" {
		t.Errorf("CanonicalTaskID = %q, want T-canonical", dec.CanonicalTaskID)
	}
	// Canonical's declared recurrence_id is retained on the merged block.
	if dec.Merged.RecurrenceID != "R1" {
		t.Errorf("Merged.RecurrenceID = %q, want R1 (canonical wins over alias R2)", dec.Merged.RecurrenceID)
	}
}

// TestResolve_Merge_PicksCorrectCanonical proves that when several canonical
// cards exist, the incoming merges into the ONE whose effective id matches (and
// no other). Guards against a scan that returns the wrong canonical or merges
// across identities.
func TestResolve_Merge_PicksCorrectCanonical(t *testing.T) {
	existing := []Card{
		{TaskID: "T-A", Recurrence: canonicalBlock("R-A", "recurrence.v1/x", 2, 2)},
		{TaskID: "T-B", Recurrence: canonicalBlock("R-B", "recurrence.v1/x", 1, 1)},
		{TaskID: "T-C", Recurrence: canonicalBlock("R-C", "recurrence.v1/x", 1, 1)},
	}
	incoming := Card{TaskID: "T-repeat-B", Recurrence: &Block{
		RecurrenceID: "R-B", SymptomClassID: "recurrence.v1/x",
		RecurrenceCount: 1, LastAcknowledgedCount: 1,
	}}
	dec := ResolveRecurrence(existing, incoming)
	if dec.Action != Merge {
		t.Fatalf("Action = %v, want Merge into R-B", dec.Action)
	}
	if dec.CanonicalTaskID != "T-B" {
		t.Errorf("CanonicalTaskID = %q, want T-B", dec.CanonicalTaskID)
	}
	if dec.Merged.RecurrenceCount != 2 {
		t.Errorf("RecurrenceCount = %d, want 2 (R-B was 1→2, NOT R-A's 2→3)", dec.Merged.RecurrenceCount)
	}
}

// TestResolve_MergedBlockIndependent proves the returned Merged block is a deep
// copy: mutating it does not mutate the existing canonical's block (the
// producer persists Merged; the in-memory existing population must be
// unaffected). Guards against an aliasing bug where Merged shares a slice
// header with the input.
func TestResolve_MergedBlockIndependent(t *testing.T) {
	canonical := canonicalBlock("R1", "recurrence.v1/foo", 1, 1,
		Evidence{Kind: "path", Ref: "a"})
	existing := []Card{{TaskID: "T-c", Recurrence: canonical}}
	incoming := Card{TaskID: "T-r", Recurrence: &Block{
		RecurrenceID: "R1", SymptomClassID: "recurrence.v1/foo",
		RecurrenceCount: 1, LastAcknowledgedCount: 1,
		Evidence: incomingEvidence("b"),
	}}
	dec := ResolveRecurrence(existing, incoming)
	// Mutate the merged block.
	dec.Merged.Evidence = append(dec.Merged.Evidence, Evidence{Kind: "path", Ref: "tampered"})
	dec.Merged.RecurrenceCount = 999
	// The original canonical must be untouched.
	if len(canonical.Evidence) != 1 || canonical.Evidence[0].Ref != "a" {
		t.Errorf("canonical evidence mutated through Merged aliasing: %+v", canonical.Evidence)
	}
	if canonical.RecurrenceCount != 1 {
		t.Errorf("canonical count mutated through Merged aliasing: %d", canonical.RecurrenceCount)
	}
}

// TestResolve_EmptyExisting proves the degenerate but common case: with no
// existing cards, every incoming recurrence card is a NewCard (the first
// observation establishes the canonical).
func TestResolve_EmptyExisting(t *testing.T) {
	incoming := Card{TaskID: "T-first", Recurrence: &Block{
		RecurrenceID: "R1", SymptomClassID: "recurrence.v1/foo",
		RecurrenceCount: 1, LastAcknowledgedCount: 1,
	}}
	dec := ResolveRecurrence(nil, incoming)
	if dec.Action != NewCard {
		t.Errorf("Action = %v, want NewCard (empty population)", dec.Action)
	}
	if dec.EffectiveID != "R1" {
		t.Errorf("EffectiveID = %q, want R1", dec.EffectiveID)
	}
}

// TestResolve_Merge_IncomingAliasDeclaresExistingID proves the cross-population
// alias reconciliation crux: an incoming card whose aliases[] point at an
// EXISTING card's recurrence_id must merge into that existing canonical (not
// spawn a second card). Without the incoming in the alias map, the alias
// direction would not be reconciled and the decision would be NewCard — causing
// write-time population (2 cards) to disagree with derivation population (1
// group after Derive collapses them).
//
// RE-POINT assertion: because alias reconciliation promotes the incoming's
// explicit id (R2) as the effective canonical, the merged block's
// recurrence_id is RE-POINTED to R2 and the prior canonical id (R1) is recorded
// as an alias. Without this re-point, a later R2 repeat (without an alias)
// would resolve to NewCard — spawning a second canonical card.
func TestResolve_Merge_IncomingAliasDeclaresExistingID(t *testing.T) {
	// Existing canonical: recurrence_id R1.
	existing := []Card{{
		TaskID: "T-canonical",
		Recurrence: &Block{
			RecurrenceID:          "R1",
			SymptomClassID:        "recurrence.v1/foo",
			RecurrenceCount:       1,
			LastAcknowledgedCount: 1,
			Evidence:              incomingEvidence("a"),
		},
	}}
	// Incoming: recurrence_id R2, aliases R1 (meaning R1 folds into R2).
	// Per the memo's alias directional choice, the explicitly-declared id (R2)
	// is canonical; aliases fold INTO it. So R1 → R2. The existing card (R1)
	// resolves to R2, matching the incoming → Merge.
	incoming := Card{TaskID: "T-repeat", Recurrence: &Block{
		RecurrenceID:          "R2",
		SymptomClassID:        "recurrence.v1/foo",
		RecurrenceCount:       1,
		LastAcknowledgedCount: 1,
		Evidence:              incomingEvidence("b"),
		Aliases:               []Alias{{RecurrenceID: "R1"}},
	}}

	dec := ResolveRecurrence(existing, incoming)
	if dec.Action != Merge {
		t.Fatalf("Action = %v, want Merge (incoming aliases R1 → R2, existing R1 matches)", dec.Action)
	}
	if dec.CanonicalTaskID != "T-canonical" {
		t.Errorf("CanonicalTaskID = %q, want T-canonical", dec.CanonicalTaskID)
	}
	if dec.Merged == nil {
		t.Fatal("Merged is nil on Merge")
	}
	if dec.Merged.RecurrenceCount != 2 {
		t.Errorf("RecurrenceCount = %d, want 2 (N→N+1)", dec.Merged.RecurrenceCount)
	}
	// CRUX: merged recurrence_id is RE-POINTED to the resolved effective id (R2),
	// NOT the prior canonical's id (R1). Without this, a later R2 repeat without
	// an alias would fail to match and spawn a second card.
	if dec.Merged.RecurrenceID != "R2" {
		t.Errorf("Merged.RecurrenceID = %q, want R2 (re-pointed to effective canonical)", dec.Merged.RecurrenceID)
	}
	// CRUX: the prior canonical id (R1) is recorded as an alias so future lookups
	// that reference R1 still resolve to this card.
	foundR1Alias := false
	for _, a := range dec.Merged.Aliases {
		if a.RecurrenceID == "R1" {
			foundR1Alias = true
			break
		}
	}
	if !foundR1Alias {
		t.Errorf("Merged.Aliases = %+v, want R1 present (prior canonical recorded as alias)", dec.Merged.Aliases)
	}
}

// TestResolve_Merge_AliasPromotionLaterRepeatProvesN1 is the CRUX REGRESSION
// for the re-point fix: after the first alias-promotion merge (R1 canonical +
// R2 incoming with alias R1 → merged block carries R2), a SUBSEQUENT R2 repeat
// (without any alias) must STILL merge into the same canonical (not spawn a
// second card). This proves the persisted identity is self-consistent for
// future lookups — the N→1 invariant holds across a concrete write sequence.
func TestResolve_Merge_AliasPromotionLaterRepeatProvesN1(t *testing.T) {
	// Step 1: existing canonical R1.
	existing := []Card{{
		TaskID: "T-canonical",
		Recurrence: &Block{
			RecurrenceID:          "R1",
			SymptomClassID:        "recurrence.v1/foo",
			RecurrenceCount:       1,
			LastAcknowledgedCount: 1,
		},
	}}
	// Step 2: incoming R2 with alias R1 → merge (re-point to R2).
	incoming1 := Card{TaskID: "T-repeat-1", Recurrence: &Block{
		RecurrenceID:          "R2",
		SymptomClassID:        "recurrence.v1/foo",
		RecurrenceCount:       1,
		LastAcknowledgedCount: 1,
		Aliases:               []Alias{{RecurrenceID: "R1"}},
	}}
	dec1 := ResolveRecurrence(existing, incoming1)
	if dec1.Action != Merge {
		t.Fatalf("Step 1: Action = %v, want Merge", dec1.Action)
	}
	if dec1.Merged.RecurrenceID != "R2" {
		t.Fatalf("Step 1: Merged.RecurrenceID = %q, want R2 (re-pointed)", dec1.Merged.RecurrenceID)
	}

	// Step 3: simulate the merged block being persisted to disk, then a NEW
	// incoming R2 repeat (no alias) arrives. It must merge into the same card.
	existingAfterMerge := []Card{{
		TaskID:     "T-canonical",
		Recurrence: dec1.Merged, // the persisted merged block (recurrence_id=R2)
	}}
	incoming2 := Card{TaskID: "T-repeat-2", Recurrence: &Block{
		RecurrenceID:          "R2",
		SymptomClassID:        "recurrence.v1/foo",
		RecurrenceCount:       1,
		LastAcknowledgedCount: 1,
	}}
	dec2 := ResolveRecurrence(existingAfterMerge, incoming2)
	if dec2.Action != Merge {
		t.Errorf("Step 2: Action = %v, want Merge (later R2 repeat must find the re-pointed canonical)", dec2.Action)
	}
	if dec2.CanonicalTaskID != "T-canonical" {
		t.Errorf("Step 2: CanonicalTaskID = %q, want T-canonical", dec2.CanonicalTaskID)
	}
	if dec2.Merged.RecurrenceCount != 3 {
		t.Errorf("Step 2: RecurrenceCount = %d, want 3 (2→3)", dec2.Merged.RecurrenceCount)
	}
}
