package permconfig

import (
	"testing"
)

// ---------------------------------------------------------------------------
// Per-agent `vh-agent-harness defer-triggers` grant (ReadOnlyExtraAllows).
//
// defer-triggers is the no-arg contained wrapper that runs ONLY the canonical
// DEFER-trigger predicate checker under ModeStrict + NetDeny + DefaultProfile.
// It is granted per-agent to researcher + worker-read-only via
// ReadOnlyExtraAllows. coordination is covered IMPLICITLY by its broad
// HarnessPolicyAllow wildcard (ReadOnlyExtraAllows is rejected on non-read_only
// agents, so it cannot carry the literal). repo-explorer / media-perception
// keep exec-sandbox but do NOT get defer-triggers (no trigger-currency charter).
//
// These tests pin:
//   - the two granted agents carry the literal allow;
//   - NO other read_only agent leaks the grant (anti-leakage);
//   - the literal is the ONLY admitted form (no wildcard/arg-form — globMatch
//     exact-matches the no-arg key, so "defer-triggers extra-arg" falls through
//     to the 4a deny);
//   - worker-read-only is still charter-bounded (NO exec-sandbox, NO raw
//     node/bash) — defer-triggers does NOT promote it to Level-B.
// ---------------------------------------------------------------------------

// deferTriggersGrantedAgents is the canonical list of read-only agents that
// receive the defer-triggers literal grant.
var deferTriggersGrantedAgents = []string{
	"researcher",
	"worker-read-only",
}

// TestDeferTriggers_GrantedAgentsCarryAllow: the two granted agents have
// DeferTriggersCommand in their ReadOnlyExtraAllows in CoreLocationRules.
func TestDeferTriggers_GrantedAgentsCarryAllow(t *testing.T) {
	for _, agent := range deferTriggersGrantedAgents {
		rule, ok := CoreLocationRules[agent]
		if !ok {
			t.Fatalf("agent %q missing from CoreLocationRules", agent)
		}
		found := false
		for _, cmd := range rule.ReadOnlyExtraAllows {
			if cmd == DeferTriggersCommand {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("agent %q: ReadOnlyExtraAllows does not contain %q (grant missing)", agent, DeferTriggersCommand)
		}
	}
}

// TestDeferTriggers_NoOtherReadOnlyAgentHasGrant: every read_only agent NOT in
// the granted set must NOT carry defer-triggers (anti-leakage). repo-explorer
// and media-perception keep exec-sandbox but deliberately do NOT get
// defer-triggers (no trigger-currency charter).
func TestDeferTriggers_NoOtherReadOnlyAgentHasGrant(t *testing.T) {
	grantedSet := make(map[string]bool, len(deferTriggersGrantedAgents))
	for _, a := range deferTriggersGrantedAgents {
		grantedSet[a] = true
	}
	for _, agent := range readOnlySpecialists {
		if grantedSet[agent] {
			continue // this agent IS granted
		}
		rule, ok := CoreLocationRules[agent]
		if !ok {
			t.Fatalf("agent %q missing from CoreLocationRules", agent)
		}
		for _, cmd := range rule.ReadOnlyExtraAllows {
			if cmd == DeferTriggersCommand {
				t.Errorf("agent %q (not in granted set) carries defer-triggers — the grant must NOT leak to agents without a trigger-currency charter", agent)
			}
		}
	}
}

// TestDeferTriggers_CoordinationNotInReadOnlyExtraAllows: coordination is
// non-read_only (HarnessPolicyAllow). ReadOnlyExtraAllows is rejected on
// non-read_only agents, so coordination must NOT carry the literal — it is
// covered IMPLICITLY by its broad "vh-agent-harness *: allow" wildcard. Asserting
// the literal is absent prevents a future authoring bug that tries to add it.
func TestDeferTriggers_CoordinationNotInReadOnlyExtraAllows(t *testing.T) {
	rule, ok := CoreLocationRules["coordination"]
	if !ok {
		t.Fatal("coordination missing from CoreLocationRules")
	}
	for _, cmd := range rule.ReadOnlyExtraAllows {
		if cmd == DeferTriggersCommand {
			t.Fatalf("coordination carries %q in ReadOnlyExtraAllows — it is non-read_only (allow policy); the literal must be absent (covered implicitly by the broad wildcard)", DeferTriggersCommand)
		}
	}
}

// TestDeferTriggers_Emission_GrantedAgentAllowsVerb: computeBashBlock emits
// defer-triggers as "allow" AFTER the 4a deny for both granted agents, so
// findLast resolves the exact no-arg form to allow.
func TestDeferTriggers_Emission_GrantedAgentAllowsVerb(t *testing.T) {
	for _, agent := range deferTriggersGrantedAgents {
		t.Run(agent, func(t *testing.T) {
			rule := CoreLocationRules[agent]
			if err := normalizeHarnessFields(&rule); err != nil {
				t.Fatalf("normalize: %v", err)
			}
			entries := computeBashBlock(rule, agent, Features{}).entries
			got := evalBashFindLast(entries, "vh-agent-harness defer-triggers")
			if got != "allow" {
				t.Fatalf("agent %q: findLast(defer-triggers no-arg) = %q, want allow", agent, got)
			}
		})
	}
}

// TestDeferTriggers_Emission_LiteralOnlyDeniesArgForm: the literal key
// "vh-agent-harness defer-triggers" (no trailing " *") globMatch-exact-matches
// ONLY the exact no-arg invocation. An arg-bearing form ("defer-triggers
// extra-arg") does NOT match the literal and falls through to the 4a deny
// (DevShCommand="vh-agent-harness *"). This is the no-wildcard-admission proof:
// a granted agent cannot run the checker with extra args/flags through this
// surface (and the wrapper itself rejects args via cobra.NoArgs).
func TestDeferTriggers_Emission_LiteralOnlyDeniesArgForm(t *testing.T) {
	for _, agent := range deferTriggersGrantedAgents {
		t.Run(agent, func(t *testing.T) {
			rule := CoreLocationRules[agent]
			if err := normalizeHarnessFields(&rule); err != nil {
				t.Fatalf("normalize: %v", err)
			}
			entries := computeBashBlock(rule, agent, Features{}).entries
			cases := []string{
				"vh-agent-harness defer-triggers extra-arg",
				"vh-agent-harness defer-triggers --mode=release",
				"vh-agent-harness defer-triggers --sandbox=off",
			}
			for _, inv := range cases {
				got := evalBashFindLast(entries, inv)
				if got != "deny" {
					t.Fatalf("agent %q: findLast(%q) = %q, want deny (literal-only admission; no wildcard form)", agent, inv, got)
				}
			}
		})
	}
}

// TestDeferTriggers_Emission_NonGrantedReadOnlyAgentDeniesVerb: a read_only
// agent NOT in the granted set resolves defer-triggers to deny (the 4a catch-all
// wins; no 4b override). planner is a pure-deliberation agent with no
// trigger-currency charter.
func TestDeferTriggers_Emission_NonGrantedReadOnlyAgentDeniesVerb(t *testing.T) {
	rule := CoreLocationRules["planner"]
	if err := normalizeHarnessFields(&rule); err != nil {
		t.Fatalf("normalize: %v", err)
	}
	entries := computeBashBlock(rule, "planner", Features{}).entries
	got := evalBashFindLast(entries, "vh-agent-harness defer-triggers")
	if got != "deny" {
		t.Fatalf("planner: findLast(defer-triggers) = %q, want deny (planner has no trigger-currency charter)", got)
	}
}

// TestDeferTriggers_ResearcherKeepsExecSandbox: granting defer-triggers to
// researcher does not displace its existing exec-sandbox Level-B grant — both
// literals coexist in ReadOnlyExtraAllows.
func TestDeferTriggers_ResearcherKeepsExecSandbox(t *testing.T) {
	rule, ok := CoreLocationRules["researcher"]
	if !ok {
		t.Fatal("researcher missing from CoreLocationRules")
	}
	hasExec, hasDefer := false, false
	for _, cmd := range rule.ReadOnlyExtraAllows {
		if cmd == ExecSandboxCommand {
			hasExec = true
		}
		if cmd == DeferTriggersCommand {
			hasDefer = true
		}
	}
	if !hasExec {
		t.Errorf("researcher lost its exec-sandbox grant when defer-triggers was added — both must coexist")
	}
	if !hasDefer {
		t.Errorf("researcher missing defer-triggers grant")
	}
}

// TestDeferTriggers_WorkerReadOnlyStillLacksExecSandbox: worker-read-only gets
// defer-triggers (charter-bounded trigger-currency inspection) but must STILL
// NOT carry exec-sandbox — defer-triggers runs ONLY the canonical checker
// contained; it does NOT promote worker-read-only to Level-B (arbitrary code).
func TestDeferTriggers_WorkerReadOnlyStillLacksExecSandbox(t *testing.T) {
	rule, ok := CoreLocationRules["worker-read-only"]
	if !ok {
		t.Fatal("worker-read-only missing from CoreLocationRules")
	}
	for _, cmd := range rule.ReadOnlyExtraAllows {
		if cmd == ExecSandboxCommand {
			t.Fatalf("worker-read-only carries exec-sandbox — defer-triggers must NOT promote it to Level-B (defer-triggers runs only the canonical checker, not arbitrary code)")
		}
	}
}

// TestDeferTriggers_WorkerReadOnlyDeniesRawNodeAndBash: worker-read-only has no
// raw node/bash wildcard — defer-triggers is the ONLY code-execution surface it
// reaches, and it runs the canonical checker contained. A bare `node ...` or
// `bash ...` must resolve to deny.
func TestDeferTriggers_WorkerReadOnlyDeniesRawNodeAndBash(t *testing.T) {
	rule := CoreLocationRules["worker-read-only"]
	if err := normalizeHarnessFields(&rule); err != nil {
		t.Fatalf("normalize: %v", err)
	}
	entries := computeBashBlock(rule, "worker-read-only", Features{}).entries
	for _, inv := range []string{
		"node tmp/x.mjs",
		"bash tmp/x.sh",
		"node -e 'console.log(1)'",
	} {
		got := evalBashFindLast(entries, inv)
		if got != "deny" {
			t.Fatalf("worker-read-only: findLast(%q) = %q, want deny (no raw node/bash grant; defer-triggers runs only the canonical checker contained)", inv, got)
		}
	}
}

// TestDeferTriggers_Emission_NoDuplicateKeys: the granted agent's bash block
// must not contain duplicate keys (defer-triggers is not also in the command-
// group region or canonical list).
func TestDeferTriggers_Emission_NoDuplicateKeys(t *testing.T) {
	for _, agent := range deferTriggersGrantedAgents {
		t.Run(agent, func(t *testing.T) {
			rule := CoreLocationRules[agent]
			if err := normalizeHarnessFields(&rule); err != nil {
				t.Fatalf("normalize: %v", err)
			}
			entries := computeBashBlock(rule, agent, Features{}).entries
			seen := map[string]int{}
			for _, e := range entries {
				seen[e.key]++
			}
			for key, count := range seen {
				if count > 1 {
					t.Fatalf("agent %q: duplicate key %q appears %d times in bash block", agent, key, count)
				}
			}
		})
	}
}

// TestDeferTriggers_Validate_CoreTablesPass: the core tables (with the
// defer-triggers grants) pass validate without error.
func TestDeferTriggers_Validate_CoreTablesPass(t *testing.T) {
	locations, tasks, gateExempt := resolveRules(nil)
	if err := validate(locations, tasks, gateExempt); err != nil {
		t.Fatalf("validate failed on core tables with defer-triggers grants: %v", err)
	}
}
