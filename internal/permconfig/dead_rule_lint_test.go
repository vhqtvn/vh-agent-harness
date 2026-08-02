package permconfig

// ---------------------------------------------------------------------------
// Dead-rule detection lint tests. See dead_rule_lint.go for the detection
// contract: a "dead rule" is a permission.bash entry whose value can NEVER be
// the findLast outcome for any input it matches, because the trailing
// DevShCommand scalar ("vh-agent-harness *") covers every input the entry's
// pattern covers AND yields a DIFFERENT decision.
//
// Two complementary test shapes live here:
//
//   - TestDeadRuleLint_NoShadowedAllowOrAsk iterates every REAL agent in
//     CoreLocationRules and asserts NONE is dead (via the shared LintDeadRules
//     helper). This proves the real permission table is clean — but (as the
//     row that prompted this slice notes) it would pass even if the detector
//     were broken, because no real rule currently exercises the dead shape.
//
//   - TestDeadRuleLint_PositiveFire synthesizes a known-shadowed-allow
//     LocationRule (deny scalar + an ExtraBash allow on a vh-agent-harness
//     subcommand), emits its bash block, and asserts LintDeadRules returns a
//     finding naming the dead pattern. This proves DETECTION CAPABILITY — the
//     test goes red if the detection branch is disabled (mutation control
//     verified during implementation), closing the "test passes even with a
//     broken detector" gap that NoShadowedAllowOrAsk alone could not cover.
//
// The one documented-intentional exemption (canonical readonly command-group
// members emitted in region 2 for non-read_only agents) is preserved
// structurally by computeBashBlock's policy-aware skips and the
// redundant-but-consistent guard inside LintDeadRules; see dead_rule_lint.go
// for detail.
// ---------------------------------------------------------------------------

import (
	"strings"
	"testing"
)

// TestDeadRuleLint_NoShadowedAllowOrAsk asserts that NO real CoreLocationRules
// agent emits a dead allow/ask entry. Detection runs through the shared
// LintDeadRules helper in dead_rule_lint.go. Because this iterates real data
// only, it cannot on its own prove the detector FIRES — pair it with
// TestDeadRuleLint_PositiveFire for capability coverage.
func TestDeadRuleLint_NoShadowedAllowOrAsk(t *testing.T) {
	for agent := range CoreLocationRules {
		agent := agent
		t.Run(agent, func(t *testing.T) {
			for _, f := range LintDeadRules(CoreLocationRules, agent) {
				t.Errorf("%s", f)
			}
		})
	}
}

// TestDeadRuleLint_PositiveFire proves the detector FIRES on a synthesized dead
// rule — closing the gap that TestDeadRuleLint_NoShadowedAllowOrAsk (which
// iterates real data only) cannot cover on its own.
//
// It constructs a deny-policy LocationRule whose ExtraBash injects an allow on a
// vh-agent-harness-prefixed verb ("vh-agent-harness custom-verb *"). Under
// computeBashBlock this emits the allow in region 3 (BEFORE the trailing
// "vh-agent-harness *": "deny" scalar), so the scalar wins for every matching
// input under findLast — the allow is dead.
//
// The test first pins the emitted dead SHAPE (the injected allow exists, sits
// before the scalar, and carries the value change allow→deny), then asserts
// LintDeadRules returns a Finding naming the dead pattern with the expected
// fields. The shape pin is load-bearing: without it the detection assertion
// could pass for the wrong reason (e.g. a different dead source than the
// injected ExtraBash). A mutation that disables the detection branch (e.g.
// commenting out the `findings = append(...)` line in LintDeadRules) makes this
// test RED because no finding is returned — the mutation control was performed
// during implementation.
func TestDeadRuleLint_PositiveFire(t *testing.T) {
	const (
		deadAgent  = "synthesized-dead-agent"
		deadVerb   = "vh-agent-harness custom-verb *"
		deadScalar = "vh-agent-harness *"
	)
	rule := LocationRule{
		Wildcard:      Deny,
		Readonly:      Allow,
		GitReadonly:   Allow,
		HasGate:       false,
		HarnessPolicy: HarnessPolicyDeny, // region 4a scalar emits "deny"
		// ExtraBash injects an allow on a vh-agent-harness subcommand that is
		// NOT a canonical read-only verb (so it survives computeBashBlock's
		// policy skip and the protectedBashKeys collision check) and is
		// subsumed by the trailing scalar — i.e. dead under findLast.
		ExtraBash: []BashEntry{{Pattern: deadVerb, Decision: Allow}},
	}
	rules := map[string]LocationRule{deadAgent: rule}

	// Sanity: the emitted bash block really has the dead shape — the injected
	// allow BEFORE the trailing deny scalar, carrying the value change
	// allow→deny. This pins the construction+emission contract the detection
	// relies on; rules_test.go::TestSynthesizedDeadLocationRule_* pins the
	// per-field shape that produces this block.
	entries := computeBashBlock(rule, deadAgent, Features{}).entries
	verbIdx, scalarIdx := -1, -1
	for i, e := range entries {
		if e.key == deadVerb {
			verbIdx = i
		}
		if e.key == deadScalar {
			scalarIdx = i
		}
	}
	if verbIdx < 0 {
		t.Fatalf("emitted block missing the injected ExtraBash allow %q", deadVerb)
	}
	if scalarIdx < 0 {
		t.Fatalf("emitted block missing the trailing scalar %q", deadScalar)
	}
	if verbIdx >= scalarIdx {
		t.Fatalf("injected allow %q (idx %d) must precede the scalar %q (idx %d) to be shadowed under findLast",
			deadVerb, verbIdx, deadScalar, scalarIdx)
	}
	if entries[verbIdx].val != string(Allow) || entries[scalarIdx].val != string(Deny) {
		t.Fatalf("dead shape mismatch: allow entry %q = %q, scalar %q = %q (want allow/deny)",
			deadVerb, entries[verbIdx].val, deadScalar, entries[scalarIdx].val)
	}

	// Detection: LintDeadRules must report the dead ExtraBash entry. This is
	// the positive fire — no real CoreLocationRules agent exercises the dead
	// shape, so this synthesized case is the only thing proving detection
	// capability.
	findings := LintDeadRules(rules, deadAgent)
	if len(findings) == 0 {
		t.Fatalf("LintDeadRules returned no findings for synthesized dead rule; the detection branch is not firing")
	}
	var dead *Finding
	for i := range findings {
		if findings[i].Pattern == deadVerb {
			dead = &findings[i]
			break
		}
	}
	if dead == nil {
		t.Fatalf("LintDeadRules did not report a finding for the dead pattern %q; findings = %v",
			deadVerb, findings)
	}
	if dead.Agent != deadAgent {
		t.Errorf("finding Agent = %q, want %q", dead.Agent, deadAgent)
	}
	if dead.Shadow != deadScalar {
		t.Errorf("finding Shadow = %q, want %q", dead.Shadow, deadScalar)
	}
	if dead.EntryValue != string(Allow) {
		t.Errorf("finding EntryValue = %q, want %q", dead.EntryValue, string(Allow))
	}
	if dead.ScalarValue != string(Deny) {
		t.Errorf("finding ScalarValue = %q, want %q", dead.ScalarValue, string(Deny))
	}
	// The finding's String() output must name both the dead pattern and the
	// agent so the diagnostic is actionable when this fires on real data.
	msg := dead.String()
	for _, want := range []string{deadAgent, deadVerb, deadScalar, string(Allow), string(Deny)} {
		if !strings.Contains(msg, want) {
			t.Errorf("finding String() = %q missing %q", msg, want)
		}
	}
}

// TestDeadRuleLint_LintSubsumptionSemantics pins the patternSubsumedByDevSh
// helper's contract so a future refactor cannot silently widen or narrow the
// shadow check.
func TestDeadRuleLint_LintSubsumptionSemantics(t *testing.T) {
	cases := []struct {
		pattern string
		want    bool
		why     string
	}{
		{DevShCommand, true, "the scalar itself"},
		{"vh-agent-harness", true, "bare binary name (matched by the scalar)"},
		{"vh-agent-harness exec-ro *", true, "vh-agent-harness-prefixed verb"},
		{"vh-agent-harness doctor", true, "vh-agent-harness-prefixed exact"},
		{"vh-agent-harness guide *", true, "vh-agent-harness-prefixed verb"},
		{"*", false, "global wildcard also matches non-vh-agent-harness commands"},
		{".opencode/scripts/readonly-scripts.sh *", false, "non-vh-agent-harness command"},
		{"git log *", false, "non-vh-agent-harness command"},
	}
	for _, c := range cases {
		got := patternSubsumedByDevSh(c.pattern)
		if got != c.want {
			t.Errorf("patternSubsumedByDevSh(%q) = %v, want %v (%s)", c.pattern, got, c.want, c.why)
		}
	}
}
