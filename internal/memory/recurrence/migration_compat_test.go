package recurrence

// migration_compat_test.go — P1-MEMORY-001 Slice 6 (the FINAL slice): the
// schema-v1 → v2 BOUNDED BACKWARD-COMPAT MIGRATION CONFIRMATION. The spec
// (researches/decisions/2026-07-29-recurrence-signature-and-dedup-enforcement.md,
// memo efa53fb) §Migration (slice 6) + §Backward compatibility states:
//
//   - effective_recurrence_id = recurrence_id when present; otherwise task_id.
//   - Do NOT auto-hash or auto-merge existing cards.
//   - Preserve task_id as the card/report identifier even after recurrence
//     identity is added.
//   - Multi-observation recurrence requires EXPLICIT promotion to the v2 form
//     before it can be acknowledged (no retroactive hash derivation).
//
// Slices 1-5 shipped this backward-compat INCREMENTALLY: EffectiveID (Slice 2)
// already returns the literal authored recurrence_id or task_id; ResolveRecurrence
// (Slice 3) already refuses to auto-merge legacy cards. This file does NOT add
// migration machinery (the design rejects auto-promotion) — it CONFIRMS the
// bounded transition is already explicit by pinning three invariants at the
// pure-derivation layer. The integration coexistence (doctor #21 + release gate
// #12 over one mixed population) is confirmed in internal/cli/.
//
// These are CONFIRMATION tests: they assert behavior the shipped code already
// has, making the migration contract non-droppable and guarding against a future
// regression that adds an auto-hash / auto-derive identity path.

import (
	"testing"
)

// TestMigration_LegacyPromotionIsExplicit is the MIGRATION CONFIRMATION CRUX
// (memo §Backward compatibility + §Stable-signature caution). It models the
// bounded-transition promotion scenario: a legacy card (no recurrence block,
// effective identity = task_id) is later recognized as recurring and an operator
// AUTHORS a recurrence block on it with an explicit recurrence_id. The promoted
// card becomes canonical via the AUTHORED identity — NOT a retroactive hash /
// digest of the task_id or any card content.
//
// This is the load-bearing "no retroactive hashing" proof: the migration path
// from schema-v1 (legacy, task_id-keyed) to v2 (recurrence_id-keyed) is an
// EXPLICIT authoring act. If any auto-derivation existed, the promoted identity
// would be a computed digest unrelated to the operator's chosen string.
func TestMigration_LegacyPromotionIsExplicit(t *testing.T) {
	// (1) BEFORE promotion — legacy card: no block, effective = task_id.
	legacy := Card{TaskID: "defer-bandaid-loop-001"}
	if got := EffectiveID(legacy); got != "defer-bandaid-loop-001" {
		t.Fatalf("legacy EffectiveID = %q, want task_id (exact literal)", got)
	}
	res := Derive([]Card{legacy})
	if len(res.Groups) != 1 {
		t.Fatalf("legacy: want 1 group, got %d", len(res.Groups))
	}
	if !res.Groups[0].IsLegacy {
		t.Errorf("legacy group must be IsLegacy=true; got EffectiveID=%q", res.Groups[0].EffectiveID)
	}
	if res.Groups[0].EffectiveID != "defer-bandaid-loop-001" {
		t.Errorf("legacy group EffectiveID = %q, want task_id", res.Groups[0].EffectiveID)
	}

	// (2) PROMOTION — the operator AUTHORS a recurrence block. The recurrence_id
	//     is an EXPLICIT operator-chosen string with NO structural relation to
	//     the task_id (it is not a hash, not a truncation, not a transform of it).
	promoted := Card{
		TaskID:     "defer-bandaid-loop-001", // task_id PRESERVED (report id)
		Recurrence: block("R-bandaid/v2", "recurrence.v1/band-aid-loop"),
	}
	if got := EffectiveID(promoted); got != "R-bandaid/v2" {
		t.Fatalf("promoted EffectiveID = %q, want AUTHORED recurrence_id %q (not a derived hash)",
			got, "R-bandaid/v2")
	}
	res2 := Derive([]Card{promoted})
	if len(res2.Groups) != 1 {
		t.Fatalf("promoted: want 1 group, got %d", len(res2.Groups))
	}
	g := res2.Groups[0]
	if g.EffectiveID != "R-bandaid/v2" {
		t.Errorf("promoted group EffectiveID = %q, want authored R-bandaid/v2", g.EffectiveID)
	}
	if g.IsLegacy {
		t.Errorf("promoted group must be IsLegacy=false (it now carries a block)")
	}

	// (3) task_id is PRESERVED as the observation identifier after promotion
	//     (memo: "preserve task_id as the card/report identifier even after
	//     recurrence identity is added") — it remains the literal task_id, not a
	//     digest. (The load-bearing "no auto-hash" pin — that EffectiveID is an
	//     exact authored literal, never a computed digest — lives in
	//     TestMigration_EffectiveIDNeverDerives; step (2) above is the promotion
	//     instance of that same exact-literal property.)
	if len(g.Observations) != 1 || g.Observations[0].TaskID != "defer-bandaid-loop-001" {
		t.Errorf("task_id not preserved after promotion; got observations %+v", g.Observations)
	}
}

// TestMigration_EffectiveIDNeverDerives is the NO-AUTO-HASH NEGATIVE test (memo
// §Backward compatibility: "Do NOT auto-hash or auto-merge existing cards"). It
// pins EffectiveID to returning EXACTLY one of two AUTHORED literals — the
// recurrence_id (when the block is present and non-empty) or the task_id — never
// a computed digest, fingerprint, or derived value. This guards against a future
// regression that adds a "smart" hash-based identity path under the migration
// banner.
func TestMigration_EffectiveIDNeverDerives(t *testing.T) {
	cases := []struct {
		name          string
		card          Card
		wantEffective string
	}{
		{
			name:          "legacy card (no block) → exact task_id",
			card:          Card{TaskID: "defer-abc-123"},
			wantEffective: "defer-abc-123",
		},
		{
			name: "recurrence card → exact authored recurrence_id",
			card: Card{
				TaskID:     "defer-abc-123",
				Recurrence: block("R-authored-name", "recurrence.v1/foo"),
			},
			wantEffective: "R-authored-name",
		},
		{
			name: "block present but EMPTY recurrence_id → task_id fallback (not derived)",
			card: Card{
				TaskID:     "defer-abc-123",
				Recurrence: block("", "recurrence.v1/foo"),
			},
			wantEffective: "defer-abc-123",
		},
		{
			name:          "nil block pointer → exact task_id",
			card:          Card{TaskID: "T-solo"},
			wantEffective: "T-solo",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EffectiveID(tc.card)
			if got != tc.wantEffective {
				t.Fatalf("EffectiveID = %q, want exact literal %q (no derivation)", got, tc.wantEffective)
			}
			// The effective id MUST be one of the two AUTHORED literals — never a
			// computed string. A hash would differ from both. This is the core
			// no-derivation assertion: identity is authored, not computed.
			isTaskID := got == tc.card.TaskID
			isAuthoredRecID := tc.card.Recurrence != nil && got == tc.card.Recurrence.RecurrenceID
			if !isTaskID && !isAuthoredRecID {
				t.Errorf("EffectiveID %q is neither the task_id %q nor the authored recurrence_id — derivation present?",
					got, tc.card.TaskID)
			}
		})
	}
}

// TestMigration_DedupNeverAutoMergesLegacyExisting pins the WRITE-BOUNDARY half
// of "Do NOT auto-merge existing cards" (memo §Backward compatibility). The
// producer dedup (ResolveRecurrence, Slice 3) considers ONLY recurrence-bearing
// cards as merge candidates. A legacy EXISTING card (no block) is never a merge
// target, even when its task_id coincides with an incoming recurrence_id — that
// namespace collision is an authoring error for doctor (Slice 4) to surface, NOT
// a silent write-time merge or auto-promotion of the legacy card.
//
// Without this guard the write boundary would silently upgrade a legacy card
// with recurrence semantics, violating the "explicit promotion" contract.
func TestMigration_DedupNeverAutoMergesLegacyExisting(t *testing.T) {
	// Existing population: a LEGACY card whose task_id ("R1") happens to equal
	// the incoming recurrence_id. This is the collision that a naive auto-merge
	// would exploit to silently promote the legacy card.
	existing := []Card{{TaskID: "R1"}} // legacy: no block
	incoming := Card{
		TaskID:     "T-new-obs",
		Recurrence: block("R1", "recurrence.v1/band-aid-loop"),
	}
	dec := ResolveRecurrence(existing, incoming)

	if dec.Action != NewCard {
		t.Fatalf("Action = %v, want NewCard (legacy existing card must never be auto-merged/promoted), merged=%+v",
			dec.Action, dec.Merged)
	}
	if dec.CanonicalTaskID != "" {
		t.Errorf("CanonicalTaskID = %q, want empty (no legacy card promoted to canonical)", dec.CanonicalTaskID)
	}
	if dec.Merged != nil {
		t.Errorf("Merged = %+v, want nil (no auto-merge produced a block)", dec.Merged)
	}
}
