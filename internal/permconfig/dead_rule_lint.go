package permconfig

// ---------------------------------------------------------------------------
// Dead-rule detection lint. A "dead rule" is a permission.bash entry whose
// value can NEVER be the findLast outcome for any input it matches — i.e. a
// later entry with a strictly broader (or identical) pattern shadows it for
// every matching invocation. Dead rules are misleading: they claim to allow
// (or ask) something that the actual permission resolution never grants.
//
// LintDeadRules is the callable extraction of the inline detection that used to
// live inside TestDeadRuleLint_NoShadowedAllowOrAsk. It exists for two reasons:
//
//   - It lets callers assert a POSITIVE fire-case (feed a constructed dead
//     rule, get a Finding back). The original inline test could only assert
//     "no real CoreLocationRules rule is dead", which passes even when the
//     detector is broken because no real rule currently exercises the dead
//     shape — so a regression in detection would be invisible. See
//     TestDeadRuleLint_PositiveFire for the capability-coverage counter-test.
//   - It keeps the detection algorithm in ONE place (production-shaped
//     non-test code) so the inline integration test cannot drift from a future
//     copy.
//
// The lint is GENERIC: it emits the bash block for one agent via
// computeBashBlock, then for each allow/ask entry BEFORE the always-last
// DevShCommand scalar ("vh-agent-harness *") it checks whether the scalar
// shadows it for ALL inputs the entry's pattern covers. It is NOT specific to
// the exec-ro case — it catches any agent that emits an allow/ask that the
// scalar renders moot (e.g. an ExtraBash allow whose verb is also a
// vh-agent-harness subcommand).
//
// ONE documented class of intentional dead entries is exempt by construction:
// readonly command-group members (HarnessReadOnlyCommandsSet) emitted in region
// 2 for NON-read_only agents. The HarnessReadOnlyCommands doc comment
// (tables.go) documents the legacy retention. The exemption is enforced inside
// computeBashBlock, not here: read_only AND deny agents skip canonical
// read-only verbs in region 2 (so neither emits a value-changing dead entry),
// and for allow/ask agents the trailing scalar AGREES with the readonly allow
// (so the redundant-but-consistent guard below passes them — they are not
// value-changing dead rules). This lint therefore catches the full class of
// ACCIDENTAL dead rules; the exemption cannot mask one, because an accidental
// dead rule would live in ExtraBash or a hand-added allow, neither of which is
// in HarnessReadOnlyCommandsSet.
// ---------------------------------------------------------------------------

import (
	"fmt"
	"strings"
)

// Finding describes one detected dead permission.bash entry.
//
// Agent is the location name the dead entry was emitted under. Pattern is the
// dead entry's bash key (the value that can never win under findLast). Shadow
// is the later entry that subsumes it (always DevShCommand today —
// "vh-agent-harness *"). EntryValue is the dead entry's value (allow or ask);
// ScalarValue is the value the shadowing scalar yields. ScalarValue !=
// EntryValue is exactly what makes the entry dead — a same-value entry is
// redundant-but-consistent, not misleading.
type Finding struct {
	Agent       string
	Pattern     string
	Shadow      string
	EntryValue  string
	ScalarValue string
}

// String formats the finding as the diagnostic the inline test used to emit, so
// callers can `t.Errorf("%s", finding)` (or log it) without re-implementing the
// wording. The string always names the agent, the dead pattern, the shadowing
// scalar, and both values so the diagnostic is actionable when this fires on
// real data.
func (f Finding) String() string {
	return fmt.Sprintf("dead rule detected for agent %s: pattern %q is shadowed by later %q for all matching inputs (entry value %q, scalar value %q)",
		f.Agent, f.Pattern, f.Shadow, f.EntryValue, f.ScalarValue)
}

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

// LintDeadRules runs the dead-rule detection lint for ONE agent in the rules
// map. Returns one Finding per dead allow/ask entry in the agent's emitted bash
// block. An empty (nil) slice means no dead rules were detected — the healthy
// case for every real CoreLocationRules agent.
//
// A "dead rule" is an allow/ask permission.bash entry whose value can NEVER be
// the findLast outcome for any input it matches, because the trailing
// "vh-agent-harness *" scalar entry (DevShCommand) covers every input the
// entry's pattern covers AND yields a DIFFERENT decision. See the file doc
// comment for the full contract and the one documented-intentional exemption
// (which is enforced inside computeBashBlock, not here).
//
// The detection iterates the EMITTED block (computeBashBlock) — not the raw
// LocationRule — so it observes the same findLast ordering OpenCode's
// permission/evaluate.ts will at runtime. A missing agent name returns nil
// (nothing to lint). Features are pinned to the zero value because none of the
// feature flags (currently just Backlog) change the dead-rule shape the lint
// depends on; the canonical agents are exercised under Features{} by the
// integration test, and the synthesized positive-fire input is feature-free.
func LintDeadRules(rules map[string]LocationRule, agent string) []Finding {
	rule, ok := rules[agent]
	if !ok {
		return nil
	}
	entries := computeBashBlock(rule, agent, Features{}).entries

	// Locate the always-last DevShCommand scalar. Every block has exactly one
	// (computeBashBlock region 4a emits it unconditionally). Entries AFTER it
	// (region 4b read_only allows) are the intended last-match winners and are
	// never shadowed by it; only entries BEFORE it can be shadowed.
	devShIdx := -1
	devShVal := ""
	for i, e := range entries {
		if e.key == DevShCommand {
			devShIdx = i
			devShVal = e.val
		}
	}
	if devShIdx < 0 {
		return nil
	}

	var findings []Finding
	for i, e := range entries {
		if i >= devShIdx {
			// Only entries BEFORE the scalar can be shadowed by it. Region 4b
			// readonly allows (read_only agents) come AFTER the scalar and are
			// the intended last-match winners.
			break
		}
		if e.val != string(Allow) && e.val != string(Ask) {
			continue
		}
		// Only consider entries whose entire match-set the scalar also covers
		// (i.e. vh-agent-harness-prefixed verbs). A "*" wildcard or a
		// .opencode/scripts/* entry is NOT shadowed by the scalar.
		if !patternSubsumedByDevSh(e.key) {
			continue
		}
		// If the scalar yields the SAME value, the entry is redundant (not
		// dead-to-a-different-value) — e.g. an Allow agent's exec-ro allow under
		// a scalar allow. Redundant-but-consistent is not a dead rule; only a
		// value-CHANGE is misleading.
		if devShVal == e.val {
			continue
		}
		// No carve-out: ANY allow/ask entry before the scalar that the later
		// DevShCommand scalar shadows to a DIFFERENT decision is a dead rule,
		// for EVERY agent (read_only, allow, ask, deny). computeBashBlock's
		// policy-aware skips plus the redundant-but-consistent guard above
		// already handle the one documented-intentional exemption (canonical
		// readonly verbs in region 2 for non-read_only agents); this lint
		// catches the full class of ACCIDENTAL dead rules, so reintroducing one
		// (e.g. a new agent with an ask/deny scalar but a region-2 readonly
		// allow, or a transform-injected ExtraBash allow on a vh-agent-harness
		// subcommand) is reported here rather than slipping through.
		findings = append(findings, Finding{
			Agent:       agent,
			Pattern:     e.key,
			Shadow:      DevShCommand,
			EntryValue:  e.val,
			ScalarValue: devShVal,
		})
	}
	return findings
}
