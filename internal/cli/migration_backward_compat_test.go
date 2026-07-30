package cli

// migration_backward_compat_test.go — P1-MEMORY-001 Slice 6 (the FINAL slice):
// the schema-v1 → v2 BOUNDED BACKWARD-COMPAT MIGRATION CONFIRMATION at the
// INTEGRATION layer (doctor #21 + release gate #12). The pure-derivation half
// of this confirmation lives in internal/memory/recurrence/migration_compat_test.go;
// this file proves the bounded transition holds end-to-end over ONE MIXED
// population through the integrated doctor + gate layers.
//
// Memo efa53fb §Backward compatibility:
//   - Manifest schema v1 accepted during a bounded migration; a legacy entry
//     remains 1:1 by defer_id.
//   - Multi-observation recurrence requires explicit promotion to the v2 form
//     before it can be acknowledged.
//
// Slices 1-5 shipped this incrementally: the gate (Slice 5) reads the ack pair
// ONLY when ack fields are present (pointer-typed decode → structurally-absent
// ack = v1 entry = dormant); doctor (Slice 4) summarizes legacy + recurrence
// cards together. These tests CONFIRM the mixed population coexists without
// breakage — they do not add migration machinery (the design rejects
// auto-promotion). They are TDD-style confirmation tests that pass against the
// shipped code.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mixedManifestRecord is one record for writeMixedManifest. When HasAck is
// false the record is a v1 entry (defer_id only, no ack fields); when true it
// carries the v2 acknowledgement pair (recurrence_count + last_acknowledged_count).
type mixedManifestRecord struct {
	DeferID string
	Count   int  // recurrence_count — written only when HasAck
	Ack     int  // last_acknowledged_count — written only when HasAck
	HasAck  bool // false → v1 (no ack); true → v2 (ack pair present)
}

// writeMixedManifest writes a manifest carrying a MIX of v1 (no ack fields) and
// v2 (ack pair present) records in one file. This exercises the bounded
// transition: a single manifest with both schema shapes coexisting, which is the
// migration state during the v1→v2 transition window.
func writeMixedManifest(t *testing.T, dir string, records []mixedManifestRecord) {
	t.Helper()
	d := filepath.Join(dir, ".vh-agent-harness")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatalf("mkdir manifest dir: %v", err)
	}
	recs := make([]map[string]any, 0, len(records))
	for _, r := range records {
		m := map[string]any{"defer_id": r.DeferID}
		if r.HasAck {
			m["recurrence_count"] = r.Count
			m["last_acknowledged_count"] = r.Ack
		}
		recs = append(recs, m)
	}
	obj := map[string]any{"schema_version": 1, "records": recs}
	raw, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		t.Fatalf("marshal mixed manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(d, "release-defer-dispositions.json"), raw, 0o644); err != nil {
		t.Fatalf("write mixed manifest: %v", err)
	}
}

// TestMigration_Coexistence_MixedPopulation_Doctor proves doctor #21 handles a
// MIXED population — a v2 recurrence-bearing card + a legacy card (no block) —
// cleanly (INFO, both summarized, no defect findings). This is the
// bounded-transition coexistence proof at the doctor diagnostic layer: legacy
// and recurrence cards coexist without cross-merging or breakage.
func TestMigration_Coexistence_MixedPopulation_Doctor(t *testing.T) {
	dir := t.TempDir()
	// v2 recurrence card (well-formed, ack pair present).
	recCard(t, dir, "defer-rec.json", "T-rec",
		recBlockJSON("R1", "recurrence.v1/band-aid-loop", 2, 2))
	// legacy card (no block) — coexists, summarized, never malformed.
	recCard(t, dir, "defer-legacy.json", "T-legacy", "")

	r := checkRecurrenceState(dir)
	assertTier(t, r, tierInfo, "mixed population coexistence (doctor)")
	// Both the recurrence canonical and the legacy card are summarized.
	for _, want := range []string{"R1", "recurrence.v1/band-aid-loop", "legacy"} {
		if !strings.Contains(r.detail, want) {
			t.Errorf("coexistence detail missing %q; got: %s", want, r.detail)
		}
	}
	// A clean coexistence must not carry defect keywords.
	for _, bad := range []string{"malformed", "uncollapsed", "conflict"} {
		if strings.Contains(strings.ToLower(r.detail), bad) {
			t.Errorf("clean coexistence detail unexpectedly mentions %q: %s", bad, r.detail)
		}
	}
}

// TestMigration_Coexistence_MixedPopulation_Gate is the bounded-transition
// COEXISTENCE CRUX at the release-gate layer (#12). A single release-imminent
// population carries ALL THREE schema shapes at once:
//
//   - a v2 recurrence-bearing card whose manifest entry carries an ack pair that
//     is CURRENT (count == ack) → not a blocker;
//   - a legacy card (no block) → not ack-checked (its effective id is its
//     task_id; the ack check skips IsLegacy groups);
//   - a v1 manifest entry (no ack fields) for the legacy card's task_id →
//     dormant (the ack check only applies to entries carrying ack fields).
//
// The gate MUST PASS: v1 entries are dormant, legacy cards are inert, and the
// v2 ack is current. This proves the bounded migration window does not break
// releases — the three shapes coexist in one manifest + one card population
// without forcing a wholesale migration.
func TestMigration_Coexistence_MixedPopulation_Gate(t *testing.T) {
	dir := recurrenceAckFixture(t)
	// v2 recurrence card: ack current (count == ack == 2).
	recCard(t, dir, "defer-rec.json", "T-rec",
		recBlockJSON("R1", "recurrence.v1/band-aid-loop", 2, 2))
	// legacy card (no block): effective id = its task_id; not ack-checked, no
	// path_touched target (inert for the F4-C predicate), not a released-claim
	// contradiction (no referenced versions, not errata).
	recCard(t, dir, "defer-legacy.json", "T-legacy", "")
	// Mixed manifest: a v2 entry (ack pair, current) for R1 + a v1 entry (no ack
	// fields) for the legacy task_id. Both coexist in ONE manifest file — the
	// bounded-transition state.
	writeMixedManifest(t, dir, []mixedManifestRecord{
		{DeferID: "R1", Count: 2, Ack: 2, HasAck: true}, // v2: ack current
		{DeferID: "T-legacy", HasAck: false},            // v1: no ack fields (dormant)
	})

	r := checkDeferLiveness(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierPass {
		t.Fatalf("mixed population coexistence: want PASS (v1 dormant + v2 ack current + legacy inert), got %s: %s",
			r.tier, r.detail)
	}
}

// TestMigration_Coexistence_MixedPopulation_GateStaleV2StillBlocks proves the
// bounded transition does NOT weaken the v2 enforcement: in the SAME mixed
// population as the passing case, a v2 entry whose ack is STALE (count > ack)
// STILL blocks the release. v1 dormancy is not a blanket pass — each v2 entry is
// enforced on its own ack pair. This is the fail-closed half of the migration
// coexistence contract.
func TestMigration_Coexistence_MixedPopulation_GateStaleV2StillBlocks(t *testing.T) {
	dir := recurrenceAckFixture(t)
	// v2 recurrence card: count=3, ack=1 on the CARD (producer bumped, held ack).
	recCard(t, dir, "defer-rec.json", "T-rec",
		recBlockJSON("R1", "recurrence.v1/band-aid-loop", 3, 1))
	// legacy card coexists (inert).
	recCard(t, dir, "defer-legacy.json", "T-legacy", "")
	// Mixed manifest: v2 entry for R1 with a STALE ack (1 < count 3) + a v1 entry
	// (no ack) for the legacy task_id. The stale v2 entry must block despite the
	// v1 entry being dormant.
	writeMixedManifest(t, dir, []mixedManifestRecord{
		{DeferID: "R1", Count: 3, Ack: 1, HasAck: true}, // v2: STALE ack → blocks
		{DeferID: "T-legacy", HasAck: false},            // v1: dormant
	})

	r := checkDeferLiveness(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierFail {
		t.Fatalf("mixed population: stale v2 ack must STILL BLOCK despite v1 dormancy, got %s: %s",
			r.tier, r.detail)
	}
	if !strings.Contains(strings.ToLower(r.detail), "unacknowledged") {
		t.Errorf("FAIL should flag the unacknowledged class; got %q", r.detail)
	}
}
