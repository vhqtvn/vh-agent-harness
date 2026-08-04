package permconfig

import (
	"encoding/json"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Per-agent exec-sandbox Level-B grant (ReadOnlyExtraAllows).
//
// These tests verify that exec-sandbox is granted per-agent to the three
// Level-B read-only specialists (researcher, repo-explorer, media-perception)
// and DENIED to every other read_only agent and every non-read_only agent.
// The floor (Slice 1) guarantees the grant is kernel-contained; this slice
// grants the verb, the floor makes the grant safe.
// ---------------------------------------------------------------------------

// execSandboxGrantedAgents is the canonical list of read-only agents that
// receive the exec-sandbox Level-B grant. Adding or removing an agent here is
// a reversible role decision recorded in the decision memo.
var execSandboxGrantedAgents = []string{
	"researcher",
	"repo-explorer",
	"media-perception",
}

// TestExecSandbox_GrantedAgentsCarryAllow: the three granted agents have
// ExecSandboxCommand in their ReadOnlyExtraAllows in CoreLocationRules.
func TestExecSandbox_GrantedAgentsCarryAllow(t *testing.T) {
	for _, agent := range execSandboxGrantedAgents {
		rule, ok := CoreLocationRules[agent]
		if !ok {
			t.Fatalf("agent %q missing from CoreLocationRules", agent)
		}
		found := false
		for _, cmd := range rule.ReadOnlyExtraAllows {
			if cmd == ExecSandboxCommand {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("agent %q: ReadOnlyExtraAllows does not contain %q (Level-B grant missing)", agent, ExecSandboxCommand)
		}
	}
}

// TestExecSandbox_NoOtherReadOnlyAgentHasGrant: every read_only agent NOT in
// the granted set must have an empty ReadOnlyExtraAllows (or at least no
// exec-sandbox entry). This is the anti-leakage proof: the grant does not
// bleed to pure-deliberation agents.
func TestExecSandbox_NoOtherReadOnlyAgentHasGrant(t *testing.T) {
	grantedSet := make(map[string]bool, len(execSandboxGrantedAgents))
	for _, a := range execSandboxGrantedAgents {
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
			if cmd == ExecSandboxCommand {
				t.Errorf("agent %q (not in granted set) carries exec-sandbox — the grant must NOT leak to pure-deliberation agents", agent)
			}
		}
	}
}

// TestExecSandbox_CommitterDoesNotHaveGrant: the committer (deny-except-gate
// invariant) must NOT receive exec-sandbox.
func TestExecSandbox_CommitterDoesNotHaveGrant(t *testing.T) {
	rule, ok := CoreLocationRules["committer"]
	if !ok {
		t.Fatal("committer missing from CoreLocationRules")
	}
	for _, cmd := range rule.ReadOnlyExtraAllows {
		if cmd == ExecSandboxCommand {
			t.Fatal("committer carries exec-sandbox — the deny-except-gate invariant is violated")
		}
	}
}

// TestExecSandbox_BuildDoesNotHaveGrant: build (Level-C Ask wildcard) must NOT
// receive exec-sandbox via ReadOnlyExtraAllows (build uses its own Ask wildcard
// axis).
func TestExecSandbox_BuildDoesNotHaveGrant(t *testing.T) {
	rule, ok := CoreLocationRules["build"]
	if !ok {
		t.Fatal("build missing from CoreLocationRules")
	}
	if len(rule.ReadOnlyExtraAllows) > 0 {
		t.Errorf("build has ReadOnlyExtraAllows entries — build is Level-C (Ask wildcard), not read_only; ReadOnlyExtraAllows must be empty")
	}
}

// TestExecSandbox_Emission_GrantedAgentAllowsVerb: computeBashBlock emits
// exec-sandbox as "allow" AFTER the 4a deny for a granted agent, so findLast
// resolves it to allow.
func TestExecSandbox_Emission_GrantedAgentAllowsVerb(t *testing.T) {
	for _, agent := range execSandboxGrantedAgents {
		t.Run(agent, func(t *testing.T) {
			rule := CoreLocationRules[agent]
			if err := normalizeHarnessFields(&rule); err != nil {
				t.Fatalf("normalize: %v", err)
			}
			entries := computeBashBlock(rule, agent, Features{}).entries

			// exec-sandbox must resolve to allow under findLast.
			got := evalBashFindLast(entries, "vh-agent-harness exec-sandbox python3 -c 'print(1+1)'")
			if got != "allow" {
				t.Fatalf("agent %q: findLast(exec-sandbox) = %q, want allow", agent, got)
			}
			// Bare form too.
			got = evalBashFindLast(entries, "vh-agent-harness exec-sandbox")
			if got != "allow" {
				t.Fatalf("agent %q: findLast(exec-sandbox bare) = %q, want allow", agent, got)
			}
		})
	}
}

// TestExecSandbox_Emission_NonGrantedAgentDeniesVerb: a read_only agent NOT in
// the granted set resolves exec-sandbox to deny under findLast (the 4a catch-all
// wins because there is no 4b override for exec-sandbox).
func TestExecSandbox_Emission_NonGrantedAgentDeniesVerb(t *testing.T) {
	// Use planner (a read_only pure-deliberation agent that must NOT get
	// exec-sandbox).
	rule := CoreLocationRules["planner"]
	if err := normalizeHarnessFields(&rule); err != nil {
		t.Fatalf("normalize: %v", err)
	}
	entries := computeBashBlock(rule, "planner", Features{}).entries

	got := evalBashFindLast(entries, "vh-agent-harness exec-sandbox python3 -c 'print(1)'")
	if got != "deny" {
		t.Fatalf("planner: findLast(exec-sandbox) = %q, want deny (planner is pure-deliberation, no exec-sandbox grant)", got)
	}
}

// TestExecSandbox_Emission_PositionAfterDeny: the exec-sandbox allow is emitted
// AFTER the 4a deny entry, so findLast correctly picks allow over deny. This
// catches a regression where the allow might be accidentally placed before the
// deny (which would make it dead).
func TestExecSandbox_Emission_PositionAfterDeny(t *testing.T) {
	rule := CoreLocationRules["researcher"]
	if err := normalizeHarnessFields(&rule); err != nil {
		t.Fatalf("normalize: %v", err)
	}
	entries := computeBashBlock(rule, "researcher", Features{}).entries

	devShIdx := -1
	execSandboxIdx := -1
	for i, e := range entries {
		if e.key == DevShCommand {
			devShIdx = i
		}
		if e.key == ExecSandboxCommand {
			execSandboxIdx = i
		}
	}
	if devShIdx < 0 {
		t.Fatal("DevShCommand (4a deny) entry missing")
	}
	if execSandboxIdx < 0 {
		t.Fatal("ExecSandboxCommand (4b allow) entry missing")
	}
	if execSandboxIdx < devShIdx {
		t.Fatalf("exec-sandbox allow (idx %d) precedes 4a deny (idx %d) — findLast would deny it (dead entry)", execSandboxIdx, devShIdx)
	}
	if entries[execSandboxIdx].val != "allow" {
		t.Fatalf("exec-sandbox decision = %q, want allow", entries[execSandboxIdx].val)
	}
}

// TestExecSandbox_Emission_CanonicalVerbsUnaffected: granting exec-sandbox does
// not break any canonical read-only verb — both still resolve correctly.
func TestExecSandbox_Emission_CanonicalVerbsUnaffected(t *testing.T) {
	rule := CoreLocationRules["researcher"]
	if err := normalizeHarnessFields(&rule); err != nil {
		t.Fatalf("normalize: %v", err)
	}
	entries := computeBashBlock(rule, "researcher", Features{}).entries

	cases := []struct {
		invocation string
		expected   string
	}{
		{"vh-agent-harness doctor", "allow"},
		{"vh-agent-harness exec-ro git status", "allow"},
		{"vh-agent-harness exec-sandbox python3 tmp/x.py", "allow"},
		{"vh-agent-harness exec go test", "deny"}, // mutation verb
		{"vh-agent-harness shell", "deny"},        // mutation verb
		{"vh-agent-harness diagnostics-export", "deny"},
	}
	for _, c := range cases {
		got := evalBashFindLast(entries, c.invocation)
		if got != c.expected {
			t.Errorf("findLast(%q) = %q, want %q", c.invocation, got, c.expected)
		}
	}
}

// TestExecSandbox_Emission_NoDuplicateKeys: the granted agent's bash block
// must not contain duplicate keys (exec-sandbox is not also in the command-
// group region or canonical list).
func TestExecSandbox_Emission_NoDuplicateKeys(t *testing.T) {
	rule := CoreLocationRules["researcher"]
	if err := normalizeHarnessFields(&rule); err != nil {
		t.Fatalf("normalize: %v", err)
	}
	entries := computeBashBlock(rule, "researcher", Features{}).entries
	seen := map[string]int{}
	for _, e := range entries {
		seen[e.key]++
	}
	for key, count := range seen {
		if count > 1 {
			t.Fatalf("duplicate key %q appears %d times in researcher bash block", key, count)
		}
	}
}

// ---------------------------------------------------------------------------
// Validation tests
// ---------------------------------------------------------------------------

// TestExecSandbox_Validate_RejectsOnNonReadOnly: setting ReadOnlyExtraAllows
// on a non-read_only agent is rejected (the field is silently dropped for
// non-read_only policies, so a non-empty value is a silent authoring bug).
func TestExecSandbox_Validate_RejectsOnNonReadOnly(t *testing.T) {
	for _, policy := range []HarnessPolicy{HarnessPolicyAllow, HarnessPolicyAsk, HarnessPolicyDeny} {
		rule := LocationRule{
			Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true,
			HarnessPolicy: policy, Edit: Deny,
			ReadOnlyExtraAllows: []string{ExecSandboxCommand},
		}
		locations := map[string]LocationRule{"test-agent": rule}
		tasks := map[string][]TaskEntry{"test-agent": {{"*", Deny}}}
		gateExempt := map[string]bool{}
		err := validate(locations, tasks, gateExempt)
		if err == nil {
			t.Fatalf("policy %q: expected validation error for ReadOnlyExtraAllows on non-read_only agent, got nil", policy)
		}
		if !strings.Contains(err.Error(), "readOnlyExtraAllows is only valid for read_only agents") {
			t.Fatalf("policy %q: error %q does not mention read_only-only constraint", policy, err.Error())
		}
	}
}

// TestExecSandbox_Validate_RejectsDuplicatePattern: duplicate patterns in
// ReadOnlyExtraAllows are rejected.
func TestExecSandbox_Validate_RejectsDuplicatePattern(t *testing.T) {
	rule := LocationRule{
		Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true,
		HarnessPolicy: HarnessPolicyReadOnly, Edit: Deny,
		ReadOnlyExtraAllows: []string{ExecSandboxCommand, ExecSandboxCommand},
	}
	locations := map[string]LocationRule{"test-agent": rule}
	tasks := map[string][]TaskEntry{"test-agent": {{"*", Deny}}}
	gateExempt := map[string]bool{}
	err := validate(locations, tasks, gateExempt)
	if err == nil {
		t.Fatal("expected duplicate-pattern error, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("error %q does not mention duplicate", err.Error())
	}
}

// TestExecSandbox_Validate_RejectsProtectedKeyCollision: a ReadOnlyExtraAllows
// entry that collides with a canonical protected key (e.g. a HarnessReadOnlyCommands
// entry) is rejected.
func TestExecSandbox_Validate_RejectsProtectedKeyCollision(t *testing.T) {
	rule := LocationRule{
		Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true,
		HarnessPolicy: HarnessPolicyReadOnly, Edit: Deny,
		ReadOnlyExtraAllows: []string{"vh-agent-harness doctor"},
	}
	locations := map[string]LocationRule{"test-agent": rule}
	tasks := map[string][]TaskEntry{"test-agent": {{"*", Deny}}}
	gateExempt := map[string]bool{}
	err := validate(locations, tasks, gateExempt)
	if err == nil {
		t.Fatal("expected protected-key collision error, got nil")
	}
	if !strings.Contains(err.Error(), "protected key") {
		t.Fatalf("error %q does not mention protected key", err.Error())
	}
}

// TestExecSandbox_Validate_CoreTablesPass: the core tables (with the three
// exec-sandbox grants) pass validate without error.
func TestExecSandbox_Validate_CoreTablesPass(t *testing.T) {
	locations, tasks, gateExempt := resolveRules(nil)
	if err := validate(locations, tasks, gateExempt); err != nil {
		t.Fatalf("validate failed on core tables with exec-sandbox grants: %v", err)
	}
}

// ---------------------------------------------------------------------------
// End-to-end emission via Emit (miniConfig + full researcher block)
// ---------------------------------------------------------------------------

// TestExecSandbox_Emit_ResearcherBlockContainsGrant: emitting through the full
// Emit pipeline produces a researcher bash block that contains the exec-sandbox
// allow entry.
func TestExecSandbox_Emit_ResearcherBlockContainsGrant(t *testing.T) {
	configWithResearcher := strings.Replace(miniConfig,
		`"repo-explorer": {
      "permission": {
        "edit": "deny",
        "bash": { "__placeholder__": "deny" },
        "task": { "__placeholder__": "deny" }
      }
    }`,
		`"repo-explorer": {
      "permission": {
        "edit": "deny",
        "bash": { "__placeholder__": "deny" },
        "task": { "__placeholder__": "deny" }
      }
    },
    "researcher": {
      "permission": {
        "edit": "deny",
        "bash": { "__placeholder__": "deny" },
        "task": { "__placeholder__": "deny" }
      }
    }`, 1)
	out := mustEmit(t, configWithResearcher, nil, Features{})
	var root map[string]any
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatalf("parse: %v", err)
	}
	agents := root["agent"].(map[string]any)
	researcher := agents["researcher"].(map[string]any)
	bash := researcher["permission"].(map[string]any)["bash"].(map[string]any)
	val, ok := bash[ExecSandboxCommand]
	if !ok {
		t.Fatalf("researcher bash block missing %q entry", ExecSandboxCommand)
	}
	if val != "allow" {
		t.Fatalf("researcher %q = %v, want allow", ExecSandboxCommand, val)
	}
}

// TestExecSandbox_Emit_RepoExplorerBlockContainsGrant: same for repo-explorer.
func TestExecSandbox_Emit_RepoExplorerBlockContainsGrant(t *testing.T) {
	out := mustEmit(t, miniConfig, nil, Features{})
	var root map[string]any
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatalf("parse: %v", err)
	}
	agents := root["agent"].(map[string]any)
	explorer := agents["repo-explorer"].(map[string]any)
	bash := explorer["permission"].(map[string]any)["bash"].(map[string]any)
	val, ok := bash[ExecSandboxCommand]
	if !ok {
		t.Fatalf("repo-explorer bash block missing %q entry", ExecSandboxCommand)
	}
	if val != "allow" {
		t.Fatalf("repo-explorer %q = %v, want allow", ExecSandboxCommand, val)
	}
}

// TestExecSandbox_Emit_CommitterDoesNotContainGrant: the committer bash block
// must NOT contain exec-sandbox.
func TestExecSandbox_Emit_CommitterDoesNotContainGrant(t *testing.T) {
	out := mustEmit(t, miniConfig, nil, Features{})
	var root map[string]any
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatalf("parse: %v", err)
	}
	agents := root["agent"].(map[string]any)
	committer := agents["committer"].(map[string]any)
	bash := committer["permission"].(map[string]any)["bash"].(map[string]any)
	if _, ok := bash[ExecSandboxCommand]; ok {
		t.Fatalf("committer bash block contains %q — deny-except-gate invariant violated", ExecSandboxCommand)
	}
}

// TestExecSandbox_Emit_BuildDoesNotContainGrant: build (Level-C Ask wildcard)
// must NOT get exec-sandbox via ReadOnlyExtraAllows (build is allow policy, not
// read_only).
func TestExecSandbox_Emit_BuildDoesNotContainGrant(t *testing.T) {
	out := mustEmit(t, miniConfig, nil, Features{})
	var root map[string]any
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatalf("parse: %v", err)
	}
	agents := root["agent"].(map[string]any)
	build := agents["build"].(map[string]any)
	bash := build["permission"].(map[string]any)["bash"].(map[string]any)
	// build is allow policy: its bash block resolves "vh-agent-harness *" to
	// allow (broad), so exec-sandbox is implicitly allowed — but it should NOT
	// have a specific ReadOnlyExtraAllows entry for it (that field is
	// read_only-only). The presence of DevShCommand=allow is expected; the
	// absence of a specific exec-sandbox key is the assertion.
	if _, ok := bash[ExecSandboxCommand]; ok {
		t.Fatalf("build bash block has a specific %q entry — build is allow policy, ReadOnlyExtraAllows should be empty", ExecSandboxCommand)
	}
}

// ---------------------------------------------------------------------------
// Blast-radius anchor: the grant ships independent of any floor
// ---------------------------------------------------------------------------

// TestExecSandbox_GrantEmittedIndependentOfFloor anchors WHY the exec-sandbox
// grant's safety cannot rest on a "paired floor" for consumers: the emitter
// (this package) has NO knowledge of exec_sandbox.min_mode. The floor lives in
// the PROJECT-OWNED run-shape.yml (a runtime/binary concern resolved by
// runshape.FindMinMode at exec-sandbox invocation time), not in the permconfig
// tables that drive emission. So the grant reaches EVERY consumer's
// opencode.jsonc regardless of whether that consumer has configured a floor —
// and every adopter whose already-seeded run-shape.yml lacks an exec_sandbox
// block (the pre-Fix-2 baseline) is unfloored by default.
//
// This is the blast-radius fact that makes Fix 1 (the binary-side refuse on
// absent + explicit --sandbox=off) the LOAD-BEARING close for existing adopters:
// the seed (Fix 2) only helps NEW installs; existing adopters keep an unfloored
// run-shape, and only Fix 1's refuse stops an explicit downgrade there. This
// test pins that the emitter does not gate the grant on any floor signal, so
// the safety property MUST be enforced where the floor is resolvable (the
// binary), not here.
func TestExecSandbox_GrantEmittedIndependentOfFloor(t *testing.T) {
	// The emitter takes (input, packs, features) — NONE of which carry a floor
	// signal. Emitting the granted agent's block therefore cannot depend on a
	// floor, by construction. We assert the grant still lands in the rendered
	// researcher block (the concrete blast-radius fact: the verb reaches the
	// agent's permission surface unconditionally).
	out := mustEmit(t, miniConfig, nil, Features{})
	var root map[string]any
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatalf("parse: %v", err)
	}
	agents := root["agent"].(map[string]any)
	granted := 0
	for _, name := range execSandboxGrantedAgents {
		a, ok := agents[name].(map[string]any)
		if !ok {
			continue // miniConfig may not carry every granted agent; that's fine.
		}
		bash, ok := a["permission"].(map[string]any)["bash"].(map[string]any)
		if !ok {
			continue
		}
		if bash[ExecSandboxCommand] == "allow" {
			granted++
		}
	}
	if granted == 0 {
		t.Fatalf("no granted agent resolved in miniConfig emit — blast-radius anchor cannot be asserted (test fixture drift); agents present: %v", agentNameList(root))
	}
	// The grant is emitted to at least one granted agent AND there is no floor
	// field anywhere in the emitted permission surface (the emitter never
	// produces one). This is the structural proof that the grant ships
	// independent of any floor — the safety property lives at the binary, not
	// the emitter.
	if containsAnyFloorKey(root) {
		t.Fatalf("emitted config contains a floor-shaped key — the emitter must NOT emit any floor signal (the floor is project-owned runtime config, resolved at exec-sandbox invocation, not at permission emission); this would change where the safety property is enforced")
	}
}

// agentNameList returns the agent names in a parsed emit root (for
// diagnosable failure messages only; order follows map iteration and is not
// sorted).
func agentNameList(root map[string]any) []string {
	agents, ok := root["agent"].(map[string]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(agents))
	for k := range agents {
		out = append(out, k)
	}
	return out
}

// containsAnyFloorKey is a shallow scan for any key that looks like an
// exec_sandbox / min_mode floor signal anywhere in the emitted config's top
// level or one level under agent/permission. The emitter must never produce
// such a key — the floor is a project-owned runtime concern, not a permission
// surface. Used by TestExecSandbox_GrantEmittedIndependentOfFloor to prove the
// emitter does not (and cannot) gate the grant on a floor.
func containsAnyFloorKey(root map[string]any) bool {
	floorish := func(k string) bool {
		return strings.Contains(k, "exec_sandbox") || strings.Contains(k, "min_mode")
	}
	for k := range root {
		if floorish(k) {
			return true
		}
	}
	if agents, ok := root["agent"].(map[string]any); ok {
		for _, v := range agents {
			a, ok := v.(map[string]any)
			if !ok {
				continue
			}
			for k := range a {
				if floorish(k) {
					return true
				}
			}
			if perm, ok := a["permission"].(map[string]any); ok {
				for k := range perm {
					if floorish(k) {
						return true
					}
				}
			}
		}
	}
	return false
}
