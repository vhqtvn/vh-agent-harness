package cli

// release_gate_recurrence_ack_test.go — TDD red→green tests for the
// recurrence-ack RELEASE-GATE enforcement (P1-MEMORY-001 Slice 5). This is the
// ACTING-AUTHORITY slice: the layer where the gate FAILS CLOSED on an
// unacknowledged recurrence (memo efa53fb, §"Manifest-v2 disposition
// interaction").
//
// Enforcement seam: checkDeferLiveness (doctor #12) derives canonical
// recurrence state from .local/coordinator/tasks/ cards (reusing Slice 4's
// loadRecurrenceCards + internal/memory/recurrence.Derive/Diagnose) and compares
// the derived recurrence_count against the committed manifest entry's
// last_acknowledged_count. If count > ack → unacknowledged → release BLOCKED.
// An uncollapsed duplicate (≥2 cards same effective id, producer-bypass) is
// ALSO a gate-consistency failure → BLOCKED.
//
// BACKWARD-COMPAT (sacred — this is release authority): a manifest entry
// WITHOUT ack fields is unaffected (the ack check is dormant for it). v1
// releases do not break.
//
// Authority line: the gate ACTS (fail-closed). Doctor #21 INFORMS. The
// committed manifest IS the cross-checkout authority for the ack; the live
// card count is the derived state the producer wrote.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ackManifestEntry is one record written by writeAckManifest.
type ackManifestEntry struct {
	DeferID string
	Count   int // recurrence_count on the manifest entry
	Ack     int // last_acknowledged_count on the manifest entry
}

// writeAckManifest writes a minimal manifest at
// <dir>/.vh-agent-harness/release-defer-dispositions.json whose records each
// carry a recurrence acknowledgement pair (recurrence_count +
// last_acknowledged_count). The gate reads last_acknowledged_count as the
// committed ack. No handshake fields: doctor #12 reads the manifest only for
// the ack pair (the release-mode evaluator owns handshake validation).
func writeAckManifest(t *testing.T, dir string, entries []ackManifestEntry) {
	t.Helper()
	d := filepath.Join(dir, ".vh-agent-harness")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatalf("mkdir manifest dir: %v", err)
	}
	records := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		records = append(records, map[string]any{
			"defer_id":                e.DeferID,
			"recurrence_count":        e.Count,
			"last_acknowledged_count": e.Ack,
		})
	}
	obj := map[string]any{"schema_version": 1, "records": records}
	raw, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(d, "release-defer-dispositions.json"), raw, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

// recurrenceAckFixture builds a scratch git repo in a release-IMMINENT state:
// a valid adoption marker, a prior tag (v1.0.0), and an about-to-release
// (untagged) migration note v1.1.0 (so releaseImminent == true). The .local/
// coordinator/tasks/ dir is created empty; tests write recurrence cards into
// it. No release-diff target is committed (the ack enforcement does not use
// the git diff — the F4-C predicate does, and recurrence cards carry no
// path_touched so they are fog/advisory there).
func recurrenceAckFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitInit(t, dir)
	t.Setenv("VH_HARNESS_DEFER_DIFF_SINCE", "")   // force self-derived prior tag
	t.Setenv("VH_HARNESS_DEFER_OVERRIDE_IDS", "") // no live override
	writeAdoptionMarker(t, dir)
	// A baseline commit so v1.0.0 has a HEAD to point at.
	gitCommitFile(t, dir, ".baseline", "x\n", "baseline")
	gitTag(t, dir, "v1.0.0")
	writeMigrationNote(t, dir, "v1.1.0") // untagged → about-to-release
	return dir
}

// --- fail-closed on unacknowledged recurrence (the core RED) ----------------

// TestRecurrenceAck_UnacknowledgedBlocks is the BEHAVIORAL-CLOSURE CRUX
// (REFUSE half): an imminent release whose recurrence card carries
// recurrence_count=3 with the manifest's last_acknowledged_count held at 1 is
// BLOCKED (FAIL). The producer bumped the card count (3) while the committed
// ack stayed at 1 → unacknowledged → release cannot ship.
func TestRecurrenceAck_UnacknowledgedBlocks(t *testing.T) {
	dir := recurrenceAckFixture(t)
	// Canonical card: recurrence_count=3, last_acknowledged_count=1 (producer
	// bumped count, held ack on the card).
	recCard(t, dir, "rec-r1.json", "T-r1",
		recBlockJSON("R1", "recurrence.v1/band-aid-loop", 3, 1))
	// Manifest attests the committed ack at 1 (last adjudication).
	writeAckManifest(t, dir, []ackManifestEntry{{DeferID: "R1", Count: 3, Ack: 1}})

	r := checkDeferLiveness(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierFail {
		t.Fatalf("CRUX want FAIL for unacknowledged recurrence (count 3 > ack 1), got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "R1") {
		t.Errorf("FAIL should name the recurrence id R1; got %q", r.detail)
	}
	if !strings.Contains(strings.ToLower(r.detail), "unacknowledged") {
		t.Errorf("FAIL should flag the unacknowledged class; got %q", r.detail)
	}
}

// --- pass after re-adjudication (ack bumped to count) -----------------------

// TestRecurrenceAck_PassesAfterReadjudication proves the CLEAR half: once the
// operator re-adjudicates (bumps the manifest's last_acknowledged_count to the
// current count), the ack check passes and the release is no longer blocked by
// the ack. The card count is unchanged (3); only the manifest ack moved 1→3.
func TestRecurrenceAck_PassesAfterReadjudication(t *testing.T) {
	dir := recurrenceAckFixture(t)
	recCard(t, dir, "rec-r1.json", "T-r1",
		recBlockJSON("R1", "recurrence.v1/band-aid-loop", 3, 1))
	// Manifest ack now matches the derived count (3) → ack current.
	writeAckManifest(t, dir, []ackManifestEntry{{DeferID: "R1", Count: 3, Ack: 3}})

	r := checkDeferLiveness(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierPass {
		t.Fatalf("want PASS after re-adjudication (ack bumped to count), got %s: %s", r.tier, r.detail)
	}
}

// --- backward-compat: v1 manifest entry without ack fields is unaffected -----

// TestRecurrenceAck_V1ManifestNoAckFieldsUnaffected proves the BACKWARD-COMPAT
// guarantee: a recurrence card whose manifest entry carries NO ack fields is
// not subject to the ack check — the release passes (the ack enforcement is
// dormant for v1 entries). The card itself carries count=3 > ack=1 (producer
// live state), but the gate compares against the MANIFEST ack, and the manifest
// here has none → no comparison → PASS.
func TestRecurrenceAck_V1ManifestNoAckFieldsUnaffected(t *testing.T) {
	dir := recurrenceAckFixture(t)
	recCard(t, dir, "rec-r1.json", "T-r1",
		recBlockJSON("R1", "recurrence.v1/band-aid-loop", 3, 1))
	// v1 manifest: defer_id present, NO ack fields (the existing writeManifest).
	writeManifest(t, dir, []string{"R1"})

	r := checkDeferLiveness(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierPass {
		t.Fatalf("v1 manifest without ack fields must PASS (backward-compat), got %s: %s", r.tier, r.detail)
	}
}

// --- manifest keyed by task_id still blocks (identity-lookup robustness) ----

// TestRecurrenceAck_ManifestKeyedByTaskIDStillBlocks pins the identity-lookup
// contract: a promoted (ack-carrying) manifest entry's canonical keying is by
// the recurrence_id (EffectiveID), BUT the gate is robust to a writer keying it
// by a CONTRIBUTING TASK_ID (a natural v1 habit — legacy entries are 1:1 by
// task_id). When the manifest is keyed by the task_id and the card carries a
// DISTINCT recurrence_id, the gate MUST STILL find the ack and block on a stale
// count > ack. Without the multi-key lookup this would SILENTLY FAIL-OPEN (the
// EffectiveID lookup misses, the backward-compat skip fires) — the exact
// failure mode a release-authority fail-closed gate must never exhibit.
func TestRecurrenceAck_ManifestKeyedByTaskIDStillBlocks(t *testing.T) {
	dir := recurrenceAckFixture(t)
	// Card: task_id="T-r1", recurrence_id="R1" (distinct), count=3, ack=1.
	recCard(t, dir, "rec-r1.json", "T-r1",
		recBlockJSON("R1", "recurrence.v1/band-aid-loop", 3, 1))
	// Manifest keyed by the TASK_ID (not the recurrence_id) — the v1 habit.
	writeAckManifest(t, dir, []ackManifestEntry{{DeferID: "T-r1", Count: 3, Ack: 1}})

	r := checkDeferLiveness(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierFail {
		t.Fatalf("manifest keyed by task_id must STILL BLOCK on count>ack (no silent fail-open), got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(strings.ToLower(r.detail), "unacknowledged") {
		t.Errorf("FAIL should flag the unacknowledged class; got %q", r.detail)
	}
}

// TestRecurrenceAck_ManifestKeyedByTaskID_PassesAfterReadjudication proves the
// task_id-keyed path also clears after re-adjudication (the multi-key lookup
// resolves the ack regardless of keying, so the clear half works too).
func TestRecurrenceAck_ManifestKeyedByTaskID_PassesAfterReadjudication(t *testing.T) {
	dir := recurrenceAckFixture(t)
	recCard(t, dir, "rec-r1.json", "T-r1",
		recBlockJSON("R1", "recurrence.v1/band-aid-loop", 3, 1))
	// Manifest keyed by task_id, ack now current (== count).
	writeAckManifest(t, dir, []ackManifestEntry{{DeferID: "T-r1", Count: 3, Ack: 3}})

	r := checkDeferLiveness(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierPass {
		t.Fatalf("task_id-keyed manifest with current ack must PASS, got %s: %s", r.tier, r.detail)
	}
}

// --- uncollapsed duplicate blocks (producer-bypass signal) ------------------

// TestRecurrenceAck_UncollapsedDuplicateBlocks proves an uncollapsed duplicate
// (≥2 cards sharing an effective_recurrence_id, existing as SEPARATE cards) is a
// gate-consistency failure → BLOCKED at release time. The producer dedup path
// (Slice 3) should have collapsed these into ONE canonical card; their
// separate existence means the dedup was bypassed (manual/direct writes).
func TestRecurrenceAck_UncollapsedDuplicateBlocks(t *testing.T) {
	dir := recurrenceAckFixture(t)
	// Two SEPARATE cards, same recurrence_id R1 → producer-bypass.
	recCard(t, dir, "rec-dup-1.json", "T-dup-1",
		recBlockJSON("R1", "recurrence.v1/band-aid-loop", 1, 1))
	recCard(t, dir, "rec-dup-2.json", "T-dup-2",
		recBlockJSON("R1", "recurrence.v1/band-aid-loop", 1, 1))
	// Manifest ack is current for R1 (ack==count), but the duplicate is a
	// SEPARATE consistency failure independent of the ack.
	writeAckManifest(t, dir, []ackManifestEntry{{DeferID: "R1", Count: 1, Ack: 1}})

	r := checkDeferLiveness(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierFail {
		t.Fatalf("uncollapsed duplicate must BLOCK (FAIL), got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(strings.ToLower(r.detail), "uncollapsed") {
		t.Errorf("FAIL should flag the uncollapsed-duplicate class; got %q", r.detail)
	}
}

// --- dormant when no release imminent (doctor stays HEALTHY in dev) ---------

// TestRecurrenceAck_DormantWhenNoReleaseImminent proves the ack enforcement is
// DORMANT (advisory only, no FAIL) when no release is imminent, even with an
// unacknowledged recurrence. This keeps doctor HEALTHY during ordinary
// development; the BLOCK activates only at release time.
func TestRecurrenceAck_DormantWhenNoReleaseImminent(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	t.Setenv("VH_HARNESS_DEFER_DIFF_SINCE", "")
	t.Setenv("VH_HARNESS_DEFER_OVERRIDE_IDS", "")
	writeAdoptionMarker(t, dir)
	gitCommitFile(t, dir, ".baseline", "x\n", "baseline")
	gitTag(t, dir, "v1.0.0")
	// A RELEASED (tagged) note — no about-to-release → releaseImminent == false.
	writeMigrationNote(t, dir, "v1.0.0")
	recCard(t, dir, "rec-r1.json", "T-r1",
		recBlockJSON("R1", "recurrence.v1/band-aid-loop", 3, 1))
	writeAckManifest(t, dir, []ackManifestEntry{{DeferID: "R1", Count: 3, Ack: 1}})

	r := checkDeferLiveness(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierPass {
		t.Fatalf("want PASS (dormant) when no release imminent, got %s: %s", r.tier, r.detail)
	}
}

// --- e2e dedup crux at the gate (STEP 3 load-bearing proof) -----------------

// TestRecurrenceAck_E2ECrux_FailThenPass is the END-TO-END CRUX verification
// that the gate's fail-closed enforcement actually fires on the producer's
// unacknowledged state and clears after re-adjudication. It exercises the FULL
// path at the gate layer in one scenario:
//
//  1. Producer wrote a repeat: the canonical card's recurrence_count moved
//     N→N+1 (2→3) while last_acknowledged_count was HELD at N (2). The
//     manifest's committed last_acknowledged_count is still N (2).
//  2. Gate FAILS-CLOSED: derived count (3) > manifest ack (2) → unacknowledged.
//  3. Operator re-adjudicates: bumps the manifest's last_acknowledged_count to
//     N+1 (3). The card is unchanged.
//  4. Gate PASSES: derived count (3) == manifest ack (3) → ack current.
//
// This is the load-bearing proof that enforcement actually fires on the
// producer's unacknowledged state (count > ack), not just a shape check.
func TestRecurrenceAck_E2ECrux_FailThenPass(t *testing.T) {
	dir := recurrenceAckFixture(t)

	// (1) Producer wrote a repeat: count 2→3, ack held at 2 on the canonical card.
	recCard(t, dir, "rec-r1.json", "T-r1",
		recBlockJSON("R1", "recurrence.v1/band-aid-loop", 3, 2))
	// Manifest still attests the last adjudication at ack=2 (stale relative to
	// the live count of 3).
	writeAckManifest(t, dir, []ackManifestEntry{{DeferID: "R1", Count: 3, Ack: 2}})

	// (2) Gate FAILS-CLOSED: the unacknowledged state (count 3 > ack 2) blocks.
	r := checkDeferLiveness(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierFail {
		t.Fatalf("CRUX step 2: want FAIL for unacknowledged (count 3 > ack 2), got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "R1") || !strings.Contains(strings.ToLower(r.detail), "unacknowledged") {
		t.Errorf("CRUX step 2: FAIL should name R1 + the unacknowledged class; got %q", r.detail)
	}

	// (3) Operator re-adjudicates: bump the manifest ack to 3 (the live count).
	writeAckManifest(t, dir, []ackManifestEntry{{DeferID: "R1", Count: 3, Ack: 3}})

	// (4) Gate PASSES: ack is now current (count 3 == ack 3).
	r = checkDeferLiveness(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierPass {
		t.Fatalf("CRUX step 4: want PASS after re-adjudication (ack bumped to 3), got %s: %s", r.tier, r.detail)
	}
}

// --- MIN-ack across recurrence_id + task_id keys (fail-closed double-entry) ---

// TestRecurrenceAck_MINAckAcrossRecurrenceAndTaskIDKeys proves the fail-closed
// MIN-ack branch in release_gate_recurrence_ack.go (the loop that resolves a
// group's committed ack by taking the MINIMUM across every matching manifest
// entry — keyed by BOTH the group's EffectiveID/recurrence_id AND any
// contributing task_id). This is the only previously-untested enforcement
// branch: the single-match paths (recurrence_id-only and task_id-only) are
// proven by the tests above, but the DOUBLE-ENTRY shape — one entry keyed by the
// recurrence_id, a second entry keyed by a contributing task_id, carrying
// DIFFERENT ack values — was not.
//
// A task_id-keyed promoted entry that holds a STALER (smaller) ack than the
// recurrence_id-keyed one must pull the effective ack DOWN to the MIN and block,
// even when the recurrence_id-keyed entry ALONE would pass. Without the MIN
// (e.g. first-match-wins or EffectiveID-only lookup), the stale task_id-keyed
// entry would be ignored and the gate would silently fail-open under a stale
// ack — the exact defect a release-authority fail-closed gate must never allow.
//
// Fixture values (chosen so the MIN is load-bearing, not incidental):
//   - card: task_id="T-r1", recurrence_id="R1" (distinct), recurrence_count=4
//     (the derived count the gate compares against).
//   - manifest entry A keyed by recurrence_id "R1" → last_acknowledged_count=5
//     (ALONE this would PASS: derived 4 ≤ 5 — no block).
//   - manifest entry B keyed by contributing task_id "T-r1" →
//     last_acknowledged_count=2 (this is the MIN: derived 4 > 2 → BLOCK).
//
// Result: effective ack = MIN(5, 2) = 2 → derived 4 > 2 → FAIL (unacknowledged).
func TestRecurrenceAck_MINAckAcrossRecurrenceAndTaskIDKeys(t *testing.T) {
	dir := recurrenceAckFixture(t)
	// One recurrence group: task_id T-r1 contributes to recurrence_id R1.
	// recurrence_count=4 is the derived count the gate compares against.
	recCard(t, dir, "rec-r1.json", "T-r1",
		recBlockJSON("R1", "recurrence.v1/band-aid-loop", 4, 2))
	// DOUBLE-ENTRY manifest: one record keyed by the canonical recurrence_id,
	// one keyed by the contributing task_id, carrying DIFFERENT ack values.
	writeAckManifest(t, dir, []ackManifestEntry{
		{DeferID: "R1", Count: 4, Ack: 5},   // recurrence_id-keyed: ALONE would PASS (4 ≤ 5)
		{DeferID: "T-r1", Count: 4, Ack: 2}, // task_id-keyed: the MIN that forces the BLOCK
	})

	r := checkDeferLiveness(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierFail {
		t.Fatalf("CRUX want FAIL under MIN-ack (derived 4 > min-ack MIN(5,2)=2; the recurrence_id-keyed entry alone would PASS since 4 ≤ 5), got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "R1") {
		t.Errorf("FAIL should name the recurrence id R1; got %q", r.detail)
	}
	if !strings.Contains(strings.ToLower(r.detail), "unacknowledged") {
		t.Errorf("FAIL should flag the unacknowledged class; got %q", r.detail)
	}
	// The FAIL detail must attest the MIN ack (2), not the recurrence_id-keyed
	// ack (5) — proving the MIN was taken across both keys, not first-match.
	if !strings.Contains(r.detail, "recurrence_count 4 > last_acknowledged_count 2") {
		t.Errorf("FAIL should report the MIN ack value (2), not the recurrence_id-keyed ack (5); got %q", r.detail)
	}
}
