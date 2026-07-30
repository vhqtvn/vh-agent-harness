package recurrence

// diagnostics_test.go — TDD red→green tests for the recurrence-signature
// DIAGNOSTICS (P1-MEMORY-001 Slice 4). Slice 2 proved Derive collapses N cards
// into canonical groups (the population view); Slice 4 surfaces the four
// diagnostic categories the memo (efa53fb, §Placement "Doctor") requires:
//
//  1. canonical recurrence groups   — informational (surfaced by the caller from
//     Derive; not asserted here, which is the Derive contract).
//  2. malformed identity            — a card's recurrence block fails a
//     load-bearing shape/consistency check (empty id / bad symptom_class_id
//     pattern / negative count / count<ack / missing ack pair).
//  3. conflicting aliases           — the alias map is ambiguous (two cards
//     claim canonical over the same id, or a cycle Derive had to break).
//  4. uncollapsed duplicates        — N recurrence-bearing cards sharing an
//     effective_recurrence_id exist as SEPARATE cards (producer dedup bypass).
//
// Categories 2 is per-card RAW-SHAPE (MalformedBlock, []byte — the typed Block
// cannot represent a structurally-missing ack pair, which decodes to 0).
// Categories 3+4 are CROSS-CARD (Diagnose, []Card) and reuse the SAME alias
// reconciliation as Derive (buildAliasMap + resolveAlias) so diagnostics and
// grouping can never disagree on identity.
//
// Authority line: this package INFORMS only (memo §Authority-line engagement).
// The release gate (Slice 5) ACTs / fails closed. These functions have NO I/O
// and NO side effects.

import (
	"reflect"
	"sort"
	"testing"
)

// rawBlock builds a recurrence-block JSON object from the given field values.
// count/ack are *int so OMIT (nil) is distinct from a real value (including a
// legitimate negative recurrence_count): the "missing ack pair" case must emit
// raw JSON with the key absent, not the zero-valued typed Block. id/class use
// the empty string for "omit" (an empty id is itself a malformed case tested
// separately via an explicit literal).
func rawBlock(id, class string, count, ack *int) []byte {
	parts := []string{}
	add := func(k, v string) { parts = append(parts, "\""+k+"\":"+v) }
	if id != "" {
		add("recurrence_id", "\""+id+"\"")
	}
	if class != "" {
		add("symptom_class_id", "\""+class+"\"")
	}
	if count != nil {
		add("recurrence_count", itoa(*count))
	}
	if ack != nil {
		add("last_acknowledged_count", itoa(*ack))
	}
	return []byte("{" + join(parts, ",") + "}")
}

// iptr returns a pointer to n (nil-safe helper for rawBlock's *int params).
func iptr(n int) *int { return &n }

// itoa is a tiny strconv.Itoa-free helper (keeps the test import list minimal).
func itoa(n int) string {
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

func join(parts []string, sep string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += sep
		}
		out += p
	}
	return out
}

// hasFinding reports whether any finding's detail contains the substring.
func hasFinding(fs []Finding, substr string) bool {
	for _, f := range fs {
		if contains(f.Detail, substr) {
			return true
		}
	}
	return false
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// categories returns the sorted set of categories present in a finding slice.
func categories(fs []Finding) []string {
	set := map[DiagnosticCategory]bool{}
	for _, f := range fs {
		set[f.Category] = true
	}
	out := make([]string, 0, len(set))
	for c := range set {
		out = append(out, string(c))
	}
	sort.Strings(out)
	return out
}

// ---- Category 2: malformed identity (MalformedBlock, raw shape) -------------

// TestMalformedBlock_Clean proves a well-formed block yields NO malformed
// findings (the green path for per-card shape).
func TestMalformedBlock_Clean(t *testing.T) {
	fs := MalformedBlock(rawBlock("R1", "recurrence.v1/foo", iptr(2), iptr(1)))
	if len(fs) != 0 {
		t.Errorf("clean block: want 0 findings, got %d: %+v", len(fs), fs)
	}
}

// TestMalformedBlock_EmptyID proves an empty recurrence_id is flagged.
func TestMalformedBlock_EmptyID(t *testing.T) {
	// Build raw JSON with an explicit empty recurrence_id (id="" omits it in
	// rawBlock, which would ALSO be empty-id; assert the explicit shape).
	raw := []byte(`{"recurrence_id":"","symptom_class_id":"recurrence.v1/foo","recurrence_count":1,"last_acknowledged_count":1}`)
	fs := MalformedBlock(raw)
	if !hasMalformed(fs, "recurrence_id") {
		t.Errorf("empty id: want a malformed finding mentioning recurrence_id, got %+v", fs)
	}
}

// TestMalformedBlock_OmittedID proves an absent recurrence_id is also flagged
// (the typed Block cannot distinguish absent from empty; raw shape can).
func TestMalformedBlock_OmittedID(t *testing.T) {
	fs := MalformedBlock(rawBlock("", "recurrence.v1/foo", iptr(1), iptr(1)))
	if !hasMalformed(fs, "recurrence_id") {
		t.Errorf("omitted id: want a malformed finding mentioning recurrence_id, got %+v", fs)
	}
}

// TestMalformedBlock_BadSymptomClass proves a symptom_class_id not matching
// ^recurrence.v1/.+$ is flagged (mirrors the schema pattern + Python validator).
func TestMalformedBlock_BadSymptomClass(t *testing.T) {
	fs := MalformedBlock(rawBlock("R1", "bare-class-name", iptr(1), iptr(1)))
	if !hasMalformed(fs, "symptom_class_id") {
		t.Errorf("bad symptom_class_id: want a malformed finding, got %+v", fs)
	}
}

// TestMalformedBlock_NegativeCount proves a negative recurrence_count is flagged
// (schema minimum:0).
func TestMalformedBlock_NegativeCount(t *testing.T) {
	fs := MalformedBlock(rawBlock("R1", "recurrence.v1/foo", iptr(-1), iptr(0)))
	if !hasMalformed(fs, "negative") {
		t.Errorf("negative count: want a malformed finding mentioning negative, got %+v", fs)
	}
}

// TestMalformedBlock_CountLtAck proves recurrence_count < last_acknowledged_count
// is flagged (the cross-field ack-pair invariant draft-07 cannot express).
func TestMalformedBlock_CountLtAck(t *testing.T) {
	fs := MalformedBlock(rawBlock("R1", "recurrence.v1/foo", iptr(1), iptr(2)))
	if !hasMalformed(fs, "ack") {
		t.Errorf("count<ack: want a malformed finding mentioning ack, got %+v", fs)
	}
}

// TestMalformedBlock_MissingAckPair proves a block carrying identity but NO
// acknowledgement state is flagged (the contract requires the pair).
func TestMalformedBlock_MissingAckPair(t *testing.T) {
	fs := MalformedBlock(rawBlock("R1", "recurrence.v1/foo", nil, nil)) // both counts OMITTED
	if !hasMalformed(fs, "ack") {
		t.Errorf("missing ack pair: want a malformed finding mentioning ack, got %+v", fs)
	}
}

// TestMalformedBlock_MissingOneAck proves one half of the pair present without
// the other is also flagged (the counts are a pair).
func TestMalformedBlock_MissingOneAck(t *testing.T) {
	// count present, ack omitted (sentinel -1).
	fs := MalformedBlock(rawBlock("R1", "recurrence.v1/foo", iptr(2), nil)) // ack OMITTED
	if !hasMalformed(fs, "ack") {
		t.Errorf("missing one ack: want a malformed finding mentioning ack, got %+v", fs)
	}
}

// TestMalformedBlock_EmptyRaw proves an empty/legacy raw block yields NO findings
// (a card with no recurrence block is legacy, not malformed).
func TestMalformedBlock_EmptyRaw(t *testing.T) {
	if fs := MalformedBlock(nil); len(fs) != 0 {
		t.Errorf("nil raw: want 0 findings, got %d: %+v", len(fs), fs)
	}
	if fs := MalformedBlock([]byte{}); len(fs) != 0 {
		t.Errorf("empty raw: want 0 findings, got %d: %+v", len(fs), fs)
	}
}

// hasMalformed reports whether any malformed_identity finding's detail contains
// the substring.
func hasMalformed(fs []Finding, substr string) bool {
	for _, f := range fs {
		if f.Category != DiagMalformedIdentity {
			continue
		}
		if contains(f.Detail, substr) {
			return true
		}
	}
	return false
}

// ---- Category 3: conflicting aliases (Diagnose, cross-card) -----------------

// TestDiagnose_ConflictingAliases proves two cards claiming CANONICAL over the
// SAME alias id (R3) are flagged: card A (R1) aliases R3; card B (R2) aliases
// R3. The alias map is ambiguous — first-declaration-wins in Derive silently
// drops one; the diagnostic must surface it.
func TestDiagnose_ConflictingAliases(t *testing.T) {
	a := Card{TaskID: "T-A", Recurrence: block("R1", "recurrence.v1/foo")}
	a.Recurrence.Aliases = []Alias{{RecurrenceID: "R3"}}
	b := Card{TaskID: "T-B", Recurrence: block("R2", "recurrence.v1/foo")}
	b.Recurrence.Aliases = []Alias{{RecurrenceID: "R3"}}

	fs := Diagnose([]Card{a, b})
	if !hasCategory(fs, DiagConflictingAliases, "R3") {
		t.Errorf("conflicting aliases: want a conflicting_aliases finding about R3, got %+v", fs)
	}
}

// TestDiagnose_AliasCycle proves a mutual alias cycle (R1↔R2) is flagged.
// resolveAlias terminates (Slice 2 guard) but must BREAK the cycle; the
// diagnostic surfaces the broken cycle so the operator reconciles explicitly.
func TestDiagnose_AliasCycle(t *testing.T) {
	a := Card{TaskID: "T-A", Recurrence: block("R1", "recurrence.v1/foo")}
	a.Recurrence.Aliases = []Alias{{RecurrenceID: "R2"}}
	b := Card{TaskID: "T-B", Recurrence: block("R2", "recurrence.v1/foo")}
	b.Recurrence.Aliases = []Alias{{RecurrenceID: "R1"}}

	fs := Diagnose([]Card{a, b})
	if !hasCategory(fs, DiagConflictingAliases, "cycle") {
		t.Errorf("alias cycle: want a conflicting_aliases finding mentioning cycle, got %+v", fs)
	}
}

// TestDiagnose_SelfAlias proves a card that declares its OWN recurrence_id as
// an alias is flagged (an authoring error: an identity cannot alias itself).
func TestDiagnose_SelfAlias(t *testing.T) {
	a := Card{TaskID: "T-A", Recurrence: block("R1", "recurrence.v1/foo")}
	a.Recurrence.Aliases = []Alias{{RecurrenceID: "R1"}} // self-alias
	fs := Diagnose([]Card{a})
	if !hasCategory(fs, DiagConflictingAliases, "self-alias") {
		t.Errorf("self-alias: want a conflicting_aliases finding mentioning self-alias, got %+v", fs)
	}
}

// TestDiagnose_ChainAliasCycle proves a 3-node chain cycle (A→B→C→A) is also
// detected (guards against a detector that only handles 2-node cycles).
func TestDiagnose_ChainAliasCycle(t *testing.T) {
	a := Card{TaskID: "T-1", Recurrence: block("A", "recurrence.v1/bar")}
	a.Recurrence.Aliases = []Alias{{RecurrenceID: "B"}}
	bb := Card{TaskID: "T-2", Recurrence: block("B", "recurrence.v1/bar")}
	bb.Recurrence.Aliases = []Alias{{RecurrenceID: "C"}}
	c := Card{TaskID: "T-3", Recurrence: block("C", "recurrence.v1/bar")}
	c.Recurrence.Aliases = []Alias{{RecurrenceID: "A"}}

	fs := Diagnose([]Card{a, bb, c})
	if !hasCategory(fs, DiagConflictingAliases, "cycle") {
		t.Errorf("chain cycle: want a conflicting_aliases finding mentioning cycle, got %+v", fs)
	}
}

// ---- Category 4: uncollapsed duplicates (Diagnose, cross-card) --------------

// TestDiagnose_UncollapsedDuplicates is the LOAD-BEARING producer-bypass signal
// (memo §Placement "Doctor", category 4): TWO recurrence-bearing cards sharing
// the SAME effective_recurrence_id exist as SEPARATE cards. The producer (Slice
// 3) should have merged the 2nd into the 1st; their coexistence as distinct
// cards means the producer dedup path was bypassed (manual/direct writes).
func TestDiagnose_UncollapsedDuplicates(t *testing.T) {
	cards := []Card{
		{TaskID: "T-1", Recurrence: block("R1", "recurrence.v1/foo")},
		{TaskID: "T-2", Recurrence: block("R1", "recurrence.v1/foo")},
	}
	fs := Diagnose(cards)
	if !hasCategory(fs, DiagUncollapsedDupes, "R1") {
		t.Errorf("uncollapsed dupes: want an uncollapsed_duplicates finding about R1, got %+v", fs)
	}
	// The finding must name BOTH colliding task_ids so the operator can find them.
	for _, f := range fs {
		if f.Category == DiagUncollapsedDupes && contains(f.Detail, "R1") {
			if !contains(f.Detail, "T-1") || !contains(f.Detail, "T-2") {
				t.Errorf("uncollapsed dupes finding must name both task_ids; got %q", f.Detail)
			}
			return
		}
	}
}

// TestDiagnose_UncollapsedViaAlias proves two recurrence-bearing cards that
// resolve to the SAME effective id VIA ALIAS (and exist as separate cards) are
// ALSO flagged — the producer's dedup is alias-aware, so alias-collapsed
// coexistence is still a producer bypass.
func TestDiagnose_UncollapsedViaAlias(t *testing.T) {
	a := Card{TaskID: "T-A", Recurrence: block("R1", "recurrence.v1/foo")}
	a.Recurrence.Aliases = []Alias{{RecurrenceID: "R2"}} // R2 folds into R1
	b := Card{TaskID: "T-B", Recurrence: block("R2", "recurrence.v1/foo")}
	fs := Diagnose([]Card{a, b})
	if !hasCategory(fs, DiagUncollapsedDupes, "R1") {
		t.Errorf("uncollapsed via alias: want an uncollapsed_duplicates finding about R1, got %+v", fs)
	}
}

// TestDiagnose_SameClassDifferentDefectsNotUncollapsed proves the CRUX
// DISTINCTION at the diagnostic layer: two cards sharing a symptom_class_id but
// carrying DIFFERENT recurrence_ids are NOT uncollapsed duplicates (they are
// correctly separate canonicals — shared class aggregates for query only).
func TestDiagnose_SameClassDifferentDefectsNotUncollapsed(t *testing.T) {
	class := "recurrence.v1/band-aid-loop"
	cards := []Card{
		{TaskID: "T-A", Recurrence: block("R-A", class)},
		{TaskID: "T-B", Recurrence: block("R-B", class)},
	}
	fs := Diagnose(cards)
	for _, f := range fs {
		if f.Category == DiagUncollapsedDupes {
			t.Errorf("same class / different defect must NOT be uncollapsed; got %q", f.Detail)
		}
	}
}

// TestDiagnose_LegacyNotFlaggedAsUncollapsed proves legacy cards (no recurrence
// block) never trigger the uncollapsed-duplicates check: the producer dedup path
// only applies to recurrence-bearing cards, and a legacy card's effective id is
// its own unique task_id.
func TestDiagnose_LegacyNotFlaggedAsUncollapsed(t *testing.T) {
	cards := []Card{
		{TaskID: "T-legacy-1"},
		{TaskID: "T-legacy-2"},
		{TaskID: "T-rec", Recurrence: block("R1", "recurrence.v1/foo")},
	}
	fs := Diagnose(cards)
	for _, f := range fs {
		if f.Category == DiagUncollapsedDupes {
			t.Errorf("legacy cards must not be flagged uncollapsed; got %q", f.Detail)
		}
	}
}

// ---- Clean path -------------------------------------------------------------

// TestDiagnose_Clean proves a well-formed, fully-collapsed population yields NO
// defect findings (the green path): one recurrence canonical + legacy cards, no
// alias conflicts, no uncollapsed duplicates.
func TestDiagnose_Clean(t *testing.T) {
	cards := []Card{
		{TaskID: "T-only", Recurrence: block("R1", "recurrence.v1/foo")},
		{TaskID: "T-legacy"},
	}
	if fs := Diagnose(cards); len(fs) != 0 {
		t.Errorf("clean population: want 0 findings, got %d: %+v", len(fs), fs)
	}
}

// TestDiagnose_Empty proves an empty/legacy-only population yields nothing.
func TestDiagnose_Empty(t *testing.T) {
	if fs := Diagnose(nil); len(fs) != 0 {
		t.Errorf("nil population: want 0 findings, got %d: %+v", len(fs), fs)
	}
	if fs := Diagnose([]Card{{TaskID: "T-legacy"}}); len(fs) != 0 {
		t.Errorf("legacy-only population: want 0 findings, got %d: %+v", len(fs), fs)
	}
}

// TestDiagnose_Deterministic proves Diagnose is a pure deterministic function:
// running it twice on the same input yields the same findings (input-order
// independence of the result set).
func TestDiagnose_Deterministic(t *testing.T) {
	a := Card{TaskID: "T-A", Recurrence: block("R1", "recurrence.v1/foo")}
	b := Card{TaskID: "T-B", Recurrence: block("R1", "recurrence.v1/foo")}
	c := Card{TaskID: "T-C", Recurrence: block("R2", "recurrence.v1/bar")}
	c.Recurrence.Aliases = []Alias{{RecurrenceID: "R3"}}
	d := Card{TaskID: "T-D", Recurrence: block("R4", "recurrence.v1/baz")}
	d.Recurrence.Aliases = []Alias{{RecurrenceID: "R3"}}
	cards := []Card{a, b, c, d}
	first := Diagnose(cards)
	second := Diagnose(cards)
	if !reflect.DeepEqual(first, second) {
		t.Errorf("Diagnose not deterministic:\nfirst  = %+v\nsecond = %+v", first, second)
	}
}

// hasCategory reports whether any finding of the given category has a detail
// containing substr.
func hasCategory(fs []Finding, cat DiagnosticCategory, substr string) bool {
	for _, f := range fs {
		if f.Category != cat {
			continue
		}
		if contains(f.Detail, substr) {
			return true
		}
	}
	return false
}

// guard: keep categories referenced (avoids unused lints when a category is
// only asserted indirectly).
var _ = []DiagnosticCategory{DiagMalformedIdentity, DiagConflictingAliases, DiagUncollapsedDupes}
var _ = categories
