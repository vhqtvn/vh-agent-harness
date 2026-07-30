package cli

// doctor_recurrence_test.go — TDD red→green wiring tests for the recurrence
// DOCTOR diagnostic (P1-MEMORY-001 Slice 4). The pure diagnostics live in
// internal/memory/recurrence (Diagnose + MalformedBlock); these tests prove the
// thin read layer over .local/coordinator/tasks/ + the doctor check surface the
// four categories (memo efa53fb, §Placement "Doctor"):
//
//  1. canonical recurrence groups   — informational report (identity/class/count).
//  2. malformed identity            — a broken recurrence block flagged.
//  3. conflicting aliases           — ambiguous alias map flagged.
//  4. uncollapsed duplicates        — producer-bypass signal flagged.
//
// Authority line: doctor INFORMS only. The check is WARN/INFO/SKIP, never FAIL
// (release enforcement is Slice 5). These tests write fixture cards to a temp
// .local/coordinator/tasks/ dir and assert the checkResult tier + detail.

import (
	"path/filepath"
	"strings"
	"testing"
)

// recBlockJSON builds a recurrence-block JSON object string. A field passed as
// the empty string / -1 sentinel is OMITTED (so the missing-ack-pair case is
// exercised against real raw JSON, not the zero-valued typed Block).
func recBlockJSON(id, class string, count, ack int) string {
	parts := make([]string, 0, 4)
	if id != "" {
		parts = append(parts, `"recurrence_id":"`+id+`"`)
	}
	if class != "" {
		parts = append(parts, `"symptom_class_id":"`+class+`"`)
	}
	if count >= 0 {
		parts = append(parts, `"recurrence_count":`+itoaCLI(count))
	}
	if ack >= 0 {
		parts = append(parts, `"last_acknowledged_count":`+itoaCLI(ack))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func itoaCLI(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// recCard writes a coordinator task card carrying a recurrence block (or a
// legacy card when blockJSON == "") under <dir>/.local/coordinator/tasks/.
func recCard(t *testing.T, dir, fname, taskID, blockJSON string) {
	t.Helper()
	body := `{"task_id":"` + taskID + `"`
	if blockJSON != "" {
		body += `,"recurrence":` + blockJSON
	}
	body += "}"
	writeTaskCard(t, dir, fname, body)
}

// assertTier fails the test unless r.tier == want.
func assertTier(t *testing.T, r checkResult, want, label string) {
	t.Helper()
	if r.tier != want {
		t.Fatalf("%s: tier = %q, want %q (detail: %s)", label, r.tier, want, r.detail)
	}
}

// ---- Category 1: canonical groups (informational) --------------------------

// TestRecurrenceState_CanonicalGroups proves the informational report: a
// recurrence-bearing card + a legacy card surface the canonical group identity,
// symptom class, and observation count, the legacy card is summarized (NOT
// flagged malformed), and the clean state is INFO (advisory-only).
func TestRecurrenceState_CanonicalGroups(t *testing.T) {
	dir := t.TempDir()
	recCard(t, dir, "defer-rec-001.json", "T-rec",
		recBlockJSON("R1", "recurrence.v1/band-aid-loop", 2, 1))
	recCard(t, dir, "defer-legacy.json", "T-legacy", "") // legacy, no block

	r := checkRecurrenceState(dir)
	assertTier(t, r, tierInfo, "canonical groups (clean)")
	for _, want := range []string{"R1", "recurrence.v1/band-aid-loop", "legacy"} {
		if !strings.Contains(r.detail, want) {
			t.Errorf("canonical-groups detail missing %q; got: %s", want, r.detail)
		}
	}
	// A clean INFO result must not carry defect keywords.
	for _, bad := range []string{"malformed", "uncollapsed", "conflict"} {
		if strings.Contains(strings.ToLower(r.detail), bad) {
			t.Errorf("clean canonical-groups detail unexpectedly mentions %q: %s", bad, r.detail)
		}
	}
}

// ---- Category 2: malformed identity ----------------------------------------

// TestRecurrenceState_MalformedIdentity proves a card with an empty
// recurrence_id is flagged as malformed (WARN).
func TestRecurrenceState_MalformedIdentity(t *testing.T) {
	dir := t.TempDir()
	recCard(t, dir, "defer-bad.json", "T-bad",
		recBlockJSON("", "recurrence.v1/foo", 1, 1)) // empty recurrence_id
	r := checkRecurrenceState(dir)
	assertTier(t, r, tierWarn, "malformed identity")
	if !strings.Contains(strings.ToLower(r.detail), "malformed") {
		t.Errorf("malformed detail missing 'malformed'; got: %s", r.detail)
	}
	if !strings.Contains(r.detail, "T-bad") {
		t.Errorf("malformed detail should name the offending task_id T-bad; got: %s", r.detail)
	}
}

// ---- Category 3: conflicting aliases ---------------------------------------

// TestRecurrenceState_ConflictingAliases proves two cards claiming canonical
// over the same alias id are flagged (WARN).
func TestRecurrenceState_ConflictingAliases(t *testing.T) {
	dir := t.TempDir()
	recCard(t, dir, "defer-a.json", "T-A",
		`{"recurrence_id":"R1","symptom_class_id":"recurrence.v1/foo","recurrence_count":1,"last_acknowledged_count":1,"aliases":[{"recurrence_id":"R3"}]}`)
	recCard(t, dir, "defer-b.json", "T-B",
		`{"recurrence_id":"R2","symptom_class_id":"recurrence.v1/foo","recurrence_count":1,"last_acknowledged_count":1,"aliases":[{"recurrence_id":"R3"}]}`)
	r := checkRecurrenceState(dir)
	assertTier(t, r, tierWarn, "conflicting aliases")
	if !strings.Contains(strings.ToLower(r.detail), "conflict") {
		t.Errorf("conflict detail missing 'conflict'; got: %s", r.detail)
	}
}

// ---- Category 4: uncollapsed duplicates (producer-bypass signal) -----------

// TestRecurrenceState_UncollapsedDuplicates proves two recurrence-bearing cards
// sharing an effective_recurrence_id but existing as SEPARATE cards are flagged
// as the producer-bypass signal (WARN). This is the consistency check that
// catches manual/direct writes that bypassed the producer dedup path.
func TestRecurrenceState_UncollapsedDuplicates(t *testing.T) {
	dir := t.TempDir()
	recCard(t, dir, "defer-dup-1.json", "T-1",
		recBlockJSON("R1", "recurrence.v1/foo", 1, 1))
	recCard(t, dir, "defer-dup-2.json", "T-2",
		recBlockJSON("R1", "recurrence.v1/foo", 1, 1))
	r := checkRecurrenceState(dir)
	assertTier(t, r, tierWarn, "uncollapsed duplicates")
	dl := strings.ToLower(r.detail)
	if !strings.Contains(dl, "uncollapsed") && !strings.Contains(dl, "duplicate") {
		t.Errorf("uncollapsed detail missing 'uncollapsed'/'duplicate'; got: %s", r.detail)
	}
	if !strings.Contains(dl, "r1") {
		t.Errorf("uncollapsed detail should name the shared id R1; got: %s", r.detail)
	}
}

// ---- Clean + Skip paths ----------------------------------------------------

// TestRecurrenceState_Clean proves a single well-formed recurrence card yields
// INFO with no defect findings.
func TestRecurrenceState_Clean(t *testing.T) {
	dir := t.TempDir()
	recCard(t, dir, "defer-clean.json", "T-clean",
		recBlockJSON("R1", "recurrence.v1/foo", 3, 3))
	r := checkRecurrenceState(dir)
	assertTier(t, r, tierInfo, "clean single canonical")
	for _, bad := range []string{"malformed", "uncollapsed", "conflict"} {
		if strings.Contains(strings.ToLower(r.detail), bad) {
			t.Errorf("clean detail unexpectedly mentions %q: %s", bad, r.detail)
		}
	}
}

// TestRecurrenceState_NoRecurrenceCards_Skip proves a tree with only legacy
// cards (no recurrence blocks) SKIPs — there is nothing to diagnose. Mirrors
// doctor's existing SKIP-when-nothing-to-check convention.
func TestRecurrenceState_NoRecurrenceCards_Skip(t *testing.T) {
	dir := t.TempDir()
	recCard(t, dir, "defer-legacy-1.json", "T-legacy-1", "")
	recCard(t, dir, "defer-legacy-2.json", "T-legacy-2", "")
	r := checkRecurrenceState(dir)
	assertTier(t, r, tierSkip, "no recurrence cards")
}

// TestRecurrenceState_NoTasksDir_Skip proves a missing .local/coordinator/tasks/
// dir SKIPs cleanly (core-only / fresh checkout).
func TestRecurrenceState_NoTasksDir_Skip(t *testing.T) {
	dir := t.TempDir()
	r := checkRecurrenceState(dir)
	assertTier(t, r, tierSkip, "no tasks dir")
}

// TestRecurrenceState_AbsoluteTarget proves the check resolves a --target path
// that is not cwd (the doctor --target flag contract). Guards against a check
// that silently reads cwd instead of the resolved target.
func TestRecurrenceState_AbsoluteTarget(t *testing.T) {
	dir := t.TempDir()
	recCard(t, dir, "defer-rec.json", "T-rec",
		recBlockJSON("R1", "recurrence.v1/foo", 1, 1))
	// Pass an absolute path distinct from cwd.
	r := checkRecurrenceState(filepath.Join(dir))
	assertTier(t, r, tierInfo, "absolute target")
	if !strings.Contains(r.detail, "R1") {
		t.Errorf("absolute target should still read the card (R1); got: %s", r.detail)
	}
}
