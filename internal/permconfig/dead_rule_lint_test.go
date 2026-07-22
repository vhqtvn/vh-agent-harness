package permconfig

// ---------------------------------------------------------------------------
// Dead-rule detection lint. A "dead rule" is a permission.bash entry whose
// value can NEVER be the findLast outcome for any input it matches — i.e. a
// later entry with a strictly broader (or identical) pattern shadows it for
// every matching invocation. Dead rules are misleading: they claim to allow
// (or ask) something that the actual permission resolution never grants.
//
// The lint is GENERIC: it iterates every agent in CoreLocationRules, emits the
// real bash block via computeBashBlock, and for each allow/ask entry BEFORE the
// always-last DevShCommand scalar ("vh-agent-harness *") it checks whether the
// scalar shadows it for ALL inputs the entry's pattern covers. It is NOT
// specific to the exec-ro case — it catches any future agent that emits an
// allow/ask that the scalar renders moot (e.g. an ExtraBash allow whose verb
// is also a vh-agent-harness subcommand).
//
// ONE documented class of intentional dead entries is exempt: readonly
// command-group members (HarnessReadOnlyCommandsSet) emitted in region 2 for
// NON-read_only agents. The HarnessReadOnlyCommands doc comment (tables.go)
// documents this: "For non-read_only agents, [exec-ro] stays in the
// command-group region as before (harmlessly dead — always shadowed by the
// scalar 'vh-agent-harness *' entry — but consistent with the legacy
// emission)." read_only agents skip these in region 2 (so no dead entry); the
// exemption only covers the non-RO legacy retention.
//
// The exemption is scoped to the audited readonly command set, so it cannot
// mask an ACCIDENTAL dead rule (which would live in ExtraBash or a hand-added
// allow, neither of which is in HarnessReadOnlyCommandsSet).
// ---------------------------------------------------------------------------

import (
	"strings"
	"testing"
)

// patternSubsumedByDevSh reports whether EVERY input matching `pattern` is also
// matched by the DevShCommand scalar ("vh-agent-harness *"). The scalar matches
// the bare "vh-agent-harness" OR "vh-agent-harness <args>". A pattern is
// subsumed iff it only matches vh-agent-harness-prefixed invocations — so a
// wildcard "*" (which also matches `git commit`, `jq`, etc.) is NOT subsumed,
// nor is a non-vh-agent-harness command like ".opencode/scripts/*".
func patternSubsumedByDevSh(pattern string) bool {
	if pattern == DevShCommand || pattern == "vh-agent-harness" {
		return true
	}
	return strings.HasPrefix(pattern, "vh-agent-harness ")
}

// TestDeadRuleLint_NoShadowedAllowOrAsk detects dead permission.bash entries:
// an allow/ask entry that the always-last DevShCommand scalar shadows for every
// matching input, making the entry's value unreachable under findLast. Fails
// with a diagnostic naming the agent, the dead pattern, and the shadowing
// scalar. See the file doc comment for the one documented-intentional exemption.
func TestDeadRuleLint_NoShadowedAllowOrAsk(t *testing.T) {
	for agent, rule := range CoreLocationRules {
		agent, rule := agent, rule
		t.Run(agent, func(t *testing.T) {
			entries := computeBashBlock(rule, agent, Features{}).entries

			// Locate the always-last DevShCommand scalar. Every block has
			// exactly one (computeBashBlock region 4a emits it unconditionally).
			devShIdx := -1
			devShVal := ""
			for i, e := range entries {
				if e.key == DevShCommand {
					devShIdx = i
					devShVal = e.val
				}
			}
			if devShIdx < 0 {
				t.Fatalf("agent %s: emitted block has no DevShCommand scalar %q", agent, DevShCommand)
			}

			nonReadOnly := rule.HarnessPolicy != HarnessPolicyReadOnly
			for i, e := range entries {
				if i >= devShIdx {
					// Only entries BEFORE the scalar can be shadowed by it.
					// Region 4b readonly allows (read_only agents) come AFTER
					// the scalar and are the intended last-match winners.
					break
				}
				if e.val != string(Allow) && e.val != string(Ask) {
					continue
				}
				// Only consider entries whose entire match-set the scalar also
				// covers (i.e. vh-agent-harness-prefixed verbs). A "*" wildcard
				// or a .opencode/scripts/* entry is NOT shadowed by the scalar.
				if !patternSubsumedByDevSh(e.key) {
					continue
				}
				// If the scalar yields the SAME value, the entry is redundant
				// (not dead-to-a-different-value) — e.g. an Allow agent's
				// exec-ro allow under a scalar allow. Redundant-but-consistent
				// is not a dead rule; only a value-CHANGE is misleading.
				if devShVal == e.val {
					continue
				}
				// Documented-intentional exemption: readonly command-group
				// members retained in region 2 for non-read_only agents. See
				// the HarnessReadOnlyCommands doc comment + this file's header.
				if nonReadOnly && HarnessReadOnlyCommandsSet[e.key] {
					continue
				}
				t.Errorf("dead rule detected for agent %s: pattern %q is shadowed by later %q for all matching inputs (entry value %q, scalar value %q)",
					agent, e.key, DevShCommand, e.val, devShVal)
			}
		})
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
