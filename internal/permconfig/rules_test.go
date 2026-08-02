package permconfig

// ---------------------------------------------------------------------------
// Tests pinning the LocationRule construction surface used by the dead-rule
// lint positive fire-case (dead_rule_lint_test.go::TestDeadRuleLint_PositiveFire).
//
// The positive fire-case synthesizes a LocationRule with three load-bearing
// fields:
//
//   - HarnessPolicy = HarnessPolicyDeny, so computeBashBlock region 4a emits
//     the trailing "vh-agent-harness *": "deny" scalar (the shadowing entry).
//   - ExtraBash = [{Pattern: "vh-agent-harness custom-verb *", Decision: Allow}],
//     so region 3 emits a vh-agent-harness-prefixed allow BEFORE the scalar —
//     a value-changing dead entry under findLast.
//   - HasGate = false, so region 2 omits the gate group entirely.
//
// These tests pin each of those fields by name and behavior so a future rename
// or semantics change to any one of them fails HERE with a focused diagnostic
// rather than silently degrading the positive fire-case (which would re-open
// the "test passes even with a broken detector" gap that prompted the row).
//
// They are deliberately separate from TestDeadRuleLint_PositiveFire so that a
// field-level regression is reported against the field/constructor surface
// (these tests) rather than only as a downstream detector failure (the
// positive fire-case). The positive fire-case keeps its own light shape-pin so
// it remains self-contained.
// ---------------------------------------------------------------------------

import (
	"testing"
)

// TestSynthesizedDeadLocationRule_FieldsExist asserts the LocationRule fields
// the positive fire-case constructs are present, named as expected, and round-
// trip the assigned values. A field rename that the rest of the package happens
// to still compile around would break the positive fire-case silently; this
// test makes such a rename fail loudly at the construction site.
func TestSynthesizedDeadLocationRule_FieldsExist(t *testing.T) {
	const deadVerb = "vh-agent-harness custom-verb *"
	rule := LocationRule{
		Wildcard:      Deny,
		Readonly:      Allow,
		GitReadonly:   Allow,
		HasGate:       false,
		HarnessPolicy: HarnessPolicyDeny,
		ExtraBash:     []BashEntry{{Pattern: deadVerb, Decision: Allow}},
	}
	if rule.HarnessPolicy != HarnessPolicyDeny {
		t.Errorf("HarnessPolicy field round-trip failed: got %q want %q", rule.HarnessPolicy, HarnessPolicyDeny)
	}
	if rule.HasGate {
		t.Errorf("HasGate field round-trip failed: got true want false")
	}
	if len(rule.ExtraBash) != 1 {
		t.Fatalf("ExtraBash field round-trip failed: got %d entries want 1", len(rule.ExtraBash))
	}
	if rule.ExtraBash[0].Pattern != deadVerb || rule.ExtraBash[0].Decision != Allow {
		t.Errorf("ExtraBash[0] round-trip failed: got Pattern=%q Decision=%q want %q/%q",
			rule.ExtraBash[0].Pattern, rule.ExtraBash[0].Decision, deadVerb, Allow)
	}
}

// TestSynthesizedDeadLocationRule_DenyScalarEmitted asserts that a
// HarnessPolicyDeny rule emits the trailing "vh-agent-harness *" scalar with
// value "deny" — the shadowing entry that makes a region-3 vh-agent-harness-
// prefixed allow dead under findLast. This pins computeBashBlock's region-4a
// behavior for the deny policy on which the positive fire-case depends; if the
// deny policy ever stopped mapping to a deny scalar, the dead-rule shape would
// silently disappear and the positive fire-case would pass for the wrong
// reason.
func TestSynthesizedDeadLocationRule_DenyScalarEmitted(t *testing.T) {
	rule := LocationRule{
		Wildcard:      Deny,
		Readonly:      Allow,
		GitReadonly:   Allow,
		HasGate:       false,
		HarnessPolicy: HarnessPolicyDeny,
	}
	if got := rule.HarnessPolicy.wildcardDecision(); got != Deny {
		t.Fatalf("HarnessPolicyDeny.wildcardDecision() = %q, want %q", got, Deny)
	}
	entries := computeBashBlock(rule, "test-deny-scalar", Features{}).entries
	var scalarVal string
	found := false
	for _, e := range entries {
		if e.key == DevShCommand {
			scalarVal = e.val
			found = true
		}
	}
	if !found {
		t.Fatalf("computeBashBlock for HarnessPolicyDeny emitted no %q scalar", DevShCommand)
	}
	if scalarVal != string(Deny) {
		t.Errorf("HarnessPolicyDeny scalar value = %q, want %q", scalarVal, string(Deny))
	}
}

// TestSynthesizedDeadLocationRule_ExtraBashAllowEmittedBeforeScalar asserts that
// an ExtraBash allow on a vh-agent-harness-prefixed pattern (the dead-rule
// construction) is emitted in region 3 — BEFORE the trailing scalar — with the
// assigned Allow decision. This is the exact emitted shape LintDeadRules
// detects as a value-changing dead entry; if region 3 ever re-ordered to AFTER
// the scalar, the entry would become the findLast winner and stop being dead.
func TestSynthesizedDeadLocationRule_ExtraBashAllowEmittedBeforeScalar(t *testing.T) {
	const injected = "vh-agent-harness custom-verb *"
	rule := LocationRule{
		Wildcard:      Deny,
		Readonly:      Allow,
		GitReadonly:   Allow,
		HasGate:       false,
		HarnessPolicy: HarnessPolicyDeny,
		ExtraBash:     []BashEntry{{Pattern: injected, Decision: Allow}},
	}
	entries := computeBashBlock(rule, "test-extrabash-before-scalar", Features{}).entries
	verbIdx, scalarIdx := -1, -1
	for i, e := range entries {
		if e.key == injected {
			verbIdx = i
		}
		if e.key == DevShCommand {
			scalarIdx = i
		}
	}
	if verbIdx < 0 {
		t.Fatalf("ExtraBash allow %q not emitted into the block", injected)
	}
	if scalarIdx < 0 {
		t.Fatalf("trailing scalar %q not emitted into the block", DevShCommand)
	}
	if verbIdx >= scalarIdx {
		t.Fatalf("ExtraBash allow %q at idx %d must precede scalar %q at idx %d",
			injected, verbIdx, DevShCommand, scalarIdx)
	}
	if entries[verbIdx].val != string(Allow) {
		t.Errorf("ExtraBash allow emitted value = %q, want %q", entries[verbIdx].val, string(Allow))
	}
}
