package permconfig

import (
	"encoding/json"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// HarnessPolicy read_only — test matrix for the deny-first + canonical
// read-only allow-list emission. This verifies the findLast (last-match-wins)
// correctness of the emitted permission.bash blocks.
//
// These tests use a TEST-ONLY findLast evaluator (globMatch +
// evalBashFindLast) that mirrors opencode's .findLast() permission resolution
// (refs/opencode/packages/core/src/permission.ts). The evaluator is NOT used
// in production — it exists to verify actual permission OUTCOMES, not just key
// positions.
// ---------------------------------------------------------------------------

// globMatch tests whether an invocation matches a permission.bash pattern. The
// matching rules mirror opencode's glob semantics:
//   - "*" matches everything.
//   - "prefix *" (trailing space-star) matches the bare "prefix" OR "prefix
//     <args>" (the star absorbs the arguments).
//   - anything else is an exact match.
func globMatch(pattern, invocation string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, " *") {
		prefix := pattern[:len(pattern)-2] // strip trailing " *"
		return invocation == prefix || strings.HasPrefix(invocation, prefix+" ")
	}
	return invocation == pattern
}

// evalBashFindLast mirrors opencode's findLast permission resolution: it
// iterates entries in emission order and returns the value of the LAST entry
// whose pattern matches the invocation. When no pattern matches, it returns
// "deny" (the safe default). This is the CRITICAL test helper — it proves the
// emitted entries produce the correct permission OUTCOMES under findLast, not
// just the correct key POSITIONS.
func evalBashFindLast(entries []orderedEntry, invocation string) string {
	result := "deny"
	for _, e := range entries {
		if globMatch(e.key, invocation) {
			result = e.val
		}
	}
	return result
}

// readOnlyRule is a canonical read_only LocationRule used across tests.
var readOnlyRule = LocationRule{
	Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true,
	HarnessPolicy: HarnessPolicyReadOnly, Edit: Deny,
}

// ---------------------------------------------------------------------------
// Test A: Policy model and normalization
// ---------------------------------------------------------------------------

// TestReadOnly_PolicyModel_FourStates: each of the 4 policies has the correct
// wildcardDecision and parses via validHarnessPolicy.
func TestReadOnly_PolicyModel_FourStates(t *testing.T) {
	cases := []struct {
		policy HarnessPolicy
		scalar Decision
	}{
		{HarnessPolicyAllow, Allow},
		{HarnessPolicyAsk, Ask},
		{HarnessPolicyDeny, Deny},
		{HarnessPolicyReadOnly, Deny},
	}
	for _, c := range cases {
		if !validHarnessPolicy(c.policy) {
			t.Errorf("validHarnessPolicy(%q) = false, want true", c.policy)
		}
		if got := c.policy.wildcardDecision(); got != c.scalar {
			t.Errorf("%q.wildcardDecision() = %q, want %q", c.policy, got, c.scalar)
		}
	}
}

// TestReadOnly_PolicyModel_UnknownInvalid: an unknown policy value is invalid.
func TestReadOnly_PolicyModel_UnknownInvalid(t *testing.T) {
	if validHarnessPolicy("strict") {
		t.Error(`validHarnessPolicy("strict") = true, want false`)
	}
	if validHarnessPolicy("") {
		t.Error(`validHarnessPolicy("") = true, want false`)
	}
}

// TestReadOnly_PolicyModel_UnknownScalarFailClosed: unknown scalar via
// wildcardDecision returns Deny (fail-closed).
func TestReadOnly_PolicyModel_UnknownScalarFailClosed(t *testing.T) {
	var unknown HarnessPolicy = "bogus"
	if got := unknown.wildcardDecision(); got != Deny {
		t.Errorf("unknown policy wildcardDecision = %q, want %q (fail-closed)", got, Deny)
	}
}

// TestReadOnly_NormalizeHarnessFields_PolicyDerivesScalar: setting HarnessPolicy
// derives Harness/DevSh.
func TestReadOnly_NormalizeHarnessFields_PolicyDerivesScalar(t *testing.T) {
	rule := LocationRule{HarnessPolicy: HarnessPolicyReadOnly}
	if err := normalizeHarnessFields(&rule); err != nil {
		t.Fatalf("normalizeHarnessFields ReadOnly: %v", err)
	}
	if rule.Harness != Deny {
		t.Errorf("Harness = %q, want %q", rule.Harness, Deny)
	}
	if rule.DevSh != Deny {
		t.Errorf("DevSh = %q, want %q", rule.DevSh, Deny)
	}
}

// TestReadOnly_NormalizeHarnessFields_ScalarDerivesPolicy: setting only Harness
// (scalar) derives HarnessPolicy. read_only is NOT derivable.
func TestReadOnly_NormalizeHarnessFields_ScalarDerivesPolicy(t *testing.T) {
	for _, c := range []struct {
		scalar Decision
		policy HarnessPolicy
	}{
		{Allow, HarnessPolicyAllow},
		{Ask, HarnessPolicyAsk},
		{Deny, HarnessPolicyDeny},
	} {
		rule := LocationRule{Harness: c.scalar}
		if err := normalizeHarnessFields(&rule); err != nil {
			t.Fatalf("normalizeHarnessFields Harness=%q: %v", c.scalar, err)
		}
		if rule.HarnessPolicy != c.policy {
			t.Errorf("Harness=%q: HarnessPolicy = %q, want %q", c.scalar, rule.HarnessPolicy, c.policy)
		}
	}
}

// TestReadOnly_NormalizeHarnessFields_ConflictingPolicyAndScalar: HarnessPolicy
// + inconsistent Harness fails closed.
func TestReadOnly_NormalizeHarnessFields_ConflictingPolicyAndScalar(t *testing.T) {
	rule := LocationRule{HarnessPolicy: HarnessPolicyReadOnly, Harness: Allow}
	err := normalizeHarnessFields(&rule)
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}
}

// TestReadOnly_NormalizeHarnessFields_Idempotent: normalizing an already-
// normalized rule is a no-op.
func TestReadOnly_NormalizeHarnessFields_Idempotent(t *testing.T) {
	rule := LocationRule{HarnessPolicy: HarnessPolicyReadOnly}
	if err := normalizeHarnessFields(&rule); err != nil {
		t.Fatalf("first normalize: %v", err)
	}
	policy1, harness1, devSh1 := rule.HarnessPolicy, rule.Harness, rule.DevSh
	if err := normalizeHarnessFields(&rule); err != nil {
		t.Fatalf("second normalize: %v", err)
	}
	if rule.HarnessPolicy != policy1 || rule.Harness != harness1 || rule.DevSh != devSh1 {
		t.Fatalf("not idempotent: (%q,%q,%q) vs (%q,%q,%q)",
			policy1, harness1, devSh1, rule.HarnessPolicy, rule.Harness, rule.DevSh)
	}
}

// TestReadOnly_ParseHarnessValue_FourValues: parseHarnessValue maps all 4
// strings to (policy, scalar) pairs.
func TestReadOnly_ParseHarnessValue_FourValues(t *testing.T) {
	cases := []struct {
		raw    string
		policy HarnessPolicy
		scalar Decision
	}{
		{"allow", HarnessPolicyAllow, Allow},
		{"ask", HarnessPolicyAsk, Ask},
		{"deny", HarnessPolicyDeny, Deny},
		{"read_only", HarnessPolicyReadOnly, Deny},
	}
	for _, c := range cases {
		policy, scalar, ok := parseHarnessValue(c.raw)
		if !ok {
			t.Errorf("parseHarnessValue(%q) ok=false, want true", c.raw)
		}
		if policy != c.policy || scalar != c.scalar {
			t.Errorf("parseHarnessValue(%q) = (%q,%q), want (%q,%q)", c.raw, policy, scalar, c.policy, c.scalar)
		}
	}
}

// TestReadOnly_ParseHarnessValue_Unknown: unrecognized value returns ok=false.
func TestReadOnly_ParseHarnessValue_Unknown(t *testing.T) {
	_, _, ok := parseHarnessValue("strict")
	if ok {
		t.Error(`parseHarnessValue("strict") ok=true, want false`)
	}
}

// TestReadOnly_ParseLocation_HarnessPolicyKey: the new "harnessPolicy" key
// selects read_only and sets Harness/DevSh to Deny.
func TestReadOnly_ParseLocation_HarnessPolicyKey(t *testing.T) {
	rule, err := parseLocation(map[string]any{
		"wildcard": "deny", "readonly": "allow", "git_readonly": "allow",
		"gate": "deny", "harnessPolicy": "read_only",
	})
	if err != nil {
		t.Fatalf("parseLocation harnessPolicy=read_only: %v", err)
	}
	if rule.HarnessPolicy != HarnessPolicyReadOnly {
		t.Errorf("HarnessPolicy = %q, want read_only", rule.HarnessPolicy)
	}
	if rule.Harness != Deny || rule.DevSh != Deny {
		t.Errorf("Harness=%q DevSh=%q, want both deny", rule.Harness, rule.DevSh)
	}
}

// TestReadOnly_ParseLocation_LegacyHarnessReadOnly: legacy "harness" key also
// accepts "read_only" value.
func TestReadOnly_ParseLocation_LegacyHarnessReadOnly(t *testing.T) {
	rule, err := parseLocation(map[string]any{
		"wildcard": "deny", "readonly": "allow", "git_readonly": "allow",
		"gate": "deny", "harness": "read_only",
	})
	if err != nil {
		t.Fatalf("parseLocation harness=read_only: %v", err)
	}
	if rule.HarnessPolicy != HarnessPolicyReadOnly {
		t.Errorf("HarnessPolicy = %q, want read_only", rule.HarnessPolicy)
	}
}

// TestReadOnly_ParseLocation_ConflictingHarnessPolicyAndHarness: harnessPolicy +
// harness with different values fails closed.
func TestReadOnly_ParseLocation_ConflictingHarnessPolicyAndHarness(t *testing.T) {
	_, err := parseLocation(map[string]any{
		"wildcard": "deny", "readonly": "allow", "git_readonly": "allow",
		"gate": "deny", "harnessPolicy": "read_only", "harness": "allow",
	})
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}
}

// TestReadOnly_ParseLocation_InvalidHarnessValue: an unrecognized harness value
// is rejected.
func TestReadOnly_ParseLocation_InvalidHarnessValue(t *testing.T) {
	_, err := parseLocation(map[string]any{
		"wildcard": "deny", "readonly": "allow", "git_readonly": "allow",
		"gate": "deny", "harness": "bogus",
	})
	if err == nil {
		t.Fatal("expected error for bogus harness value, got nil")
	}
}

// ---------------------------------------------------------------------------
// Test B: Raw emission ordering (for read_only)
// ---------------------------------------------------------------------------

// TestReadOnly_EmissionOrder_DenyFirstThenAllows: for read_only, exactly one
// "vh-agent-harness *" entry exists with decision "deny", it precedes every
// canonical safe harness entry, and output order is stable.
func TestReadOnly_EmissionOrder_DenyFirstThenAllows(t *testing.T) {
	om := computeBashBlock(readOnlyRule, "test-agent", Features{})
	entries := om.entries

	// Find the DevShCommand entry — must be exactly one, decision deny.
	devShCount := 0
	devShIdx := -1
	for i, e := range entries {
		if e.key == DevShCommand {
			devShCount++
			devShIdx = i
		}
	}
	if devShCount != 1 {
		t.Fatalf("found %d %q entries, want exactly 1", devShCount, DevShCommand)
	}
	if entries[devShIdx].val != "deny" {
		t.Fatalf("%q decision = %q, want deny", DevShCommand, entries[devShIdx].val)
	}

	// Every canonical read-only allow must come AFTER the devSh deny.
	for i, e := range entries {
		if HarnessReadOnlyCommandsSet[e.key] {
			if i < devShIdx {
				t.Fatalf("canonical allow %q (idx %d) precedes %q deny (idx %d) — findLast would deny it",
					e.key, i, DevShCommand, devShIdx)
			}
			if e.val != "allow" {
				t.Fatalf("canonical allow %q decision = %q, want allow", e.key, e.val)
			}
		}
	}
}

// TestReadOnly_EmissionOrder_StableAcrossRenders: repeated computeBashBlock
// calls produce identical entry sequences.
func TestReadOnly_EmissionOrder_StableAcrossRenders(t *testing.T) {
	first := computeBashBlock(readOnlyRule, "test-agent", Features{}).entries
	for i := 0; i < 20; i++ {
		again := computeBashBlock(readOnlyRule, "test-agent", Features{}).entries
		if len(first) != len(again) {
			t.Fatalf("iteration %d: entry count drift %d vs %d", i, len(first), len(again))
		}
		for j := range first {
			if first[j] != again[j] {
				t.Fatalf("iteration %d: entry %d differs: %+v vs %+v", i, j, first[j], again[j])
			}
		}
	}
}

// TestReadOnly_EmissionOrder_NoDuplicatesIn4b: the 4b region must not contain
// duplicate keys (the exec-ro skip in region 2 prevents the one overlap).
func TestReadOnly_EmissionOrder_NoDuplicatesIn4b(t *testing.T) {
	om := computeBashBlock(readOnlyRule, "test-agent", Features{})
	seen := map[string]int{}
	for _, e := range om.entries {
		seen[e.key]++
	}
	for key, count := range seen {
		if count > 1 {
			t.Fatalf("duplicate key %q appears %d times in read_only bash block", key, count)
		}
	}
}

// TestReadOnly_EmissionOrder_ScalarPoliciesHaveNoExceptions: allow/ask/deny
// agents emit ONLY the scalar "vh-agent-harness *" entry — no 4b exceptions
// region (no canonical read-only allows AFTER the devSh entry). Note:
// "vh-agent-harness exec-ro *" legitimately appears in the command-group
// region (region 2) for ALL agents — it is dead code there (shadowed by the
// later devSh entry under findLast) but byte-stable with the legacy emission.
func TestReadOnly_EmissionOrder_ScalarPoliciesHaveNoExceptions(t *testing.T) {
	for _, policy := range []HarnessPolicy{HarnessPolicyAllow, HarnessPolicyAsk, HarnessPolicyDeny} {
		rule := LocationRule{
			Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true,
			HarnessPolicy: policy, Edit: Deny,
		}
		entries := computeBashBlock(rule, "test-agent", Features{}).entries

		// Find the devSh entry — everything after it is the 4b region.
		devShIdx := -1
		for i, e := range entries {
			if e.key == DevShCommand {
				devShIdx = i
			}
		}
		if devShIdx < 0 {
			t.Fatalf("policy %q: devSh entry missing", policy)
		}

		// No canonical read-only allow may appear AFTER devSh (that would be a
		// 4b region, which only read_only agents should have).
		for i := devShIdx + 1; i < len(entries); i++ {
			if HarnessReadOnlyCommandsSet[entries[i].key] {
				t.Fatalf("policy %q emitted canonical read-only allow %q in 4b region (idx %d > devSh idx %d)",
					policy, entries[i].key, i, devShIdx)
			}
		}

		// The devSh entry must carry the policy's wildcardDecision.
		want := string(policy.wildcardDecision())
		if entries[devShIdx].val != want {
			t.Fatalf("policy %q: devSh decision %q, want %q", policy, entries[devShIdx].val, want)
		}
	}
}

// ---------------------------------------------------------------------------
// Test C: findLast evaluator (CRITICAL)
// ---------------------------------------------------------------------------

// TestReadOnly_FindLast_TestMatrix is the CRITICAL proof that the emitted
// read_only bash block produces correct permission OUTCOMES under opencode's
// findLast resolution. Each invocation must resolve to the expected decision.
func TestReadOnly_FindLast_TestMatrix(t *testing.T) {
	om := computeBashBlock(readOnlyRule, "test-agent", Features{})
	entries := om.entries

	cases := []struct {
		invocation string
		expected   string
		category   string
	}{
		// Canonical safe verbs → allow.
		{"vh-agent-harness doctor", "allow", "safe verb (bare)"},
		{"vh-agent-harness doctor --json", "allow", "safe verb (with args)"},
		{"vh-agent-harness exec-ro git status", "allow", "exec-ro"},
		{"vh-agent-harness skill list", "allow", "skill list"},
		{"vh-agent-harness guide", "allow", "guide"},
		{"vh-agent-harness status", "allow", "status"},
		{"vh-agent-harness version", "allow", "version"},
		{"vh-agent-harness --help", "allow", "--help"},
		{"vh-agent-harness -h", "allow", "-h"},
		{"vh-agent-harness help", "allow", "help"},
		{"vh-agent-harness docs", "allow", "docs"},
		// Mutation verbs → deny (not in the canonical inventory).
		{"vh-agent-harness exec go test ./...", "deny", "exec mutation"},
		{"vh-agent-harness shell", "deny", "shell"},
		{"vh-agent-harness update", "deny", "update"},
		{"vh-agent-harness install", "deny", "install"},
		// Artifact-producing → deny.
		{"vh-agent-harness diagnostics-export", "deny", "diagnostics-export"},
		// Withheld pending audit → deny in v1.
		{"vh-agent-harness skill validate tmp/foo", "deny", "skill validate (withheld v1)"},
		{"vh-agent-harness logs", "deny", "logs (withheld v1)"},
		{"vh-agent-harness ps", "deny", "ps (withheld v1)"},
		// Broad wildcards that would be dangerous → deny (only skill list is allowed).
		{"vh-agent-harness skill validate", "deny", "skill validate bare (withheld v1)"},
		{"vh-agent-harness overlay new my-pack", "deny", "overlay new mutation"},
		// Unknown future verb → deny (the deny-first catch-all wins).
		{"vh-agent-harness future-new-command", "deny", "unknown future verb"},
		// Non-harness commands → resolved by wildcard/groups (deny for read_only).
		{"ls", "allow", "readonly group (ls)"},
		{"git status", "allow", "git_readonly group"},
	}

	for _, c := range cases {
		t.Run(c.category, func(t *testing.T) {
			got := evalBashFindLast(entries, c.invocation)
			if got != c.expected {
				t.Fatalf("findLast(%q) = %q, want %q (%s)", c.invocation, got, c.expected, c.category)
			}
		})
	}
}

// TestReadOnly_FindLast_ScalarPolicies: allow/ask/deny agents resolve every
// harness invocation to their scalar decision (no exceptions override).
func TestReadOnly_FindLast_ScalarPolicies(t *testing.T) {
	cases := []struct {
		policy   HarnessPolicy
		expected string
	}{
		{HarnessPolicyAllow, "allow"},
		{HarnessPolicyAsk, "ask"},
		{HarnessPolicyDeny, "deny"},
	}
	invocations := []string{
		"vh-agent-harness doctor",
		"vh-agent-harness exec go test",
		"vh-agent-harness shell",
		"vh-agent-harness future-verb",
	}
	for _, c := range cases {
		rule := LocationRule{
			Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true,
			HarnessPolicy: c.policy, Edit: Deny,
		}
		entries := computeBashBlock(rule, "test-agent", Features{}).entries
		for _, inv := range invocations {
			got := evalBashFindLast(entries, inv)
			if got != c.expected {
				t.Fatalf("policy %q findLast(%q) = %q, want %q", c.policy, inv, got, c.expected)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Test D: Role regression — verify each core role produces the correct policy.
// ---------------------------------------------------------------------------

// readOnlySpecialists is the canonical list of core agents migrated to
// read_only. The first 9 are the original RO specialists; the latter 8 are the
// F7-residue migration (commit-message, commit-reviewer + its a..d leaves,
// ship-review, docs-steward). harness-release-readiness is the 18th read_only
// agent overall but is overlay-managed (the .vh-agent-harness release pack),
// so it is NOT in CoreLocationRules and is not asserted here.
var readOnlySpecialists = []string{
	"planner", "researcher", "repo-explorer", "media-perception",
	"debate", "debate-proposer", "debate-critic", "debate-synth",
	"solution-brief",
	// F7-residue migration (m0120):
	"commit-message", "commit-reviewer",
	"commit-reviewer-a", "commit-reviewer-b", "commit-reviewer-c", "commit-reviewer-d",
	"ship-review", "docs-steward",
}

// TestReadOnly_RoleRegression_SpecialistsAreReadOnly: all 9 RO specialists
// carry HarnessPolicyReadOnly in CoreLocationRules.
func TestReadOnly_RoleRegression_SpecialistsAreReadOnly(t *testing.T) {
	for _, agent := range readOnlySpecialists {
		rule, ok := CoreLocationRules[agent]
		if !ok {
			t.Fatalf("agent %q missing from CoreLocationRules", agent)
		}
		if rule.HarnessPolicy != HarnessPolicyReadOnly {
			t.Errorf("agent %q: HarnessPolicy = %q, want read_only", agent, rule.HarnessPolicy)
		}
	}
}

// TestReadOnly_RoleRegression_OrchestratorsAreAllow: build, coordination,
// project-coordinator carry HarnessPolicyAllow.
func TestReadOnly_RoleRegression_OrchestratorsAreAllow(t *testing.T) {
	for _, agent := range []string{"build", "coordination", "project-coordinator"} {
		rule, ok := CoreLocationRules[agent]
		if !ok {
			t.Fatalf("agent %q missing from CoreLocationRules", agent)
		}
		if rule.HarnessPolicy != HarnessPolicyAllow {
			t.Errorf("agent %q: HarnessPolicy = %q, want allow", agent, rule.HarnessPolicy)
		}
	}
}

// TestReadOnly_RoleRegression_CommitterIsDeny: committer carries
// HarnessPolicyDeny (keeps its gated command surface).
func TestReadOnly_RoleRegression_CommitterIsDeny(t *testing.T) {
	rule, ok := CoreLocationRules["committer"]
	if !ok {
		t.Fatal("committer missing from CoreLocationRules")
	}
	if rule.HarnessPolicy != HarnessPolicyDeny {
		t.Errorf("committer: HarnessPolicy = %q, want deny", rule.HarnessPolicy)
	}
	// The committer's gate decision must still be Allow (gated command surface).
	if rule.Gate != Allow {
		t.Errorf("committer: Gate = %q, want allow (gated surface)", rule.Gate)
	}
}

// TestReadOnly_RoleRegression_EmissionMatchesPolicy: each role's emitted bash
// block has the correct findLast outcome for a representative invocation.
func TestReadOnly_RoleRegression_EmissionMatchesPolicy(t *testing.T) {
	cases := []struct {
		agent      string
		invocation string
		expected   string
	}{
		// RO specialists: safe verb → allow, mutation → deny.
		{"researcher", "vh-agent-harness doctor", "allow"},
		{"researcher", "vh-agent-harness exec go test", "deny"},
		{"planner", "vh-agent-harness status", "allow"},
		{"repo-explorer", "vh-agent-harness guide", "allow"},
		// Orchestrators: everything → allow.
		{"build", "vh-agent-harness doctor", "allow"},
		{"build", "vh-agent-harness exec go test", "allow"},
		{"coordination", "vh-agent-harness shell", "allow"},
		// Committer: everything harness → deny.
		{"committer", "vh-agent-harness doctor", "deny"},
		{"committer", "vh-agent-harness exec go test", "deny"},
	}
	for _, c := range cases {
		t.Run(c.agent+"/"+c.invocation, func(t *testing.T) {
			rule, ok := CoreLocationRules[c.agent]
			if !ok {
				t.Fatalf("agent %q missing", c.agent)
			}
			// Normalize (core tables are already normalized but the map copy
			// hasn't been through resolveRules).
			if err := normalizeHarnessFields(&rule); err != nil {
				t.Fatalf("normalize: %v", err)
			}
			entries := computeBashBlock(rule, c.agent, Features{}).entries
			got := evalBashFindLast(entries, c.invocation)
			if got != c.expected {
				t.Fatalf("agent %q findLast(%q) = %q, want %q", c.agent, c.invocation, got, c.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Test E: Transform protection
// ---------------------------------------------------------------------------

// TestReadOnly_TransformProtection_DevShCollisionRejected: injecting
// "vh-agent-harness *" via a transform is rejected.
func TestReadOnly_TransformProtection_DevShCollisionRejected(t *testing.T) {
	extra := map[string][]BashEntry{
		"build": {{Pattern: DevShCommand, Decision: Allow}},
	}
	_, err := EmitWithExtra([]byte(miniConfig), nil, Features{}, extra)
	if err == nil {
		t.Fatal("expected protected-key error for vh-agent-harness *, got nil")
	}
	if !strings.Contains(err.Error(), "protected key") {
		t.Fatalf("error %q does not mention 'protected key'", err.Error())
	}
}

// TestReadOnly_TransformProtection_CanonicalAllowKeyRejected: injecting any
// canonical read-only allow key via a transform is rejected.
func TestReadOnly_TransformProtection_CanonicalAllowKeyRejected(t *testing.T) {
	for _, pat := range []string{
		"vh-agent-harness doctor",
		"vh-agent-harness doctor *",
		"vh-agent-harness exec-ro *",
		"vh-agent-harness skill list",
		"vh-agent-harness guide",
	} {
		extra := map[string][]BashEntry{
			"build": {{Pattern: pat, Decision: Deny}},
		}
		_, err := EmitWithExtra([]byte(miniConfig), nil, Features{}, extra)
		if err == nil {
			t.Fatalf("expected protected-key error for %q, got nil", pat)
		}
		if !strings.Contains(err.Error(), "protected key") {
			t.Fatalf("error %q does not mention 'protected key' (pattern %q)", err.Error(), pat)
		}
	}
}

// TestReadOnly_TransformProtection_NonCanonicalHarnessPatternInert: a transform
// injecting a non-canonical harness pattern cannot broaden the policy —
// findLast picks the later 4a deny (for read_only). Verified at the
// computeBashBlock level: the ExtraBash entry lands in region 3 (before 4a
// deny), so findLast resolves the hostile verb to deny.
func TestReadOnly_TransformProtection_NonCanonicalHarnessPatternInert(t *testing.T) {
	rule := LocationRule{
		Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true,
		HarnessPolicy: HarnessPolicyReadOnly, Edit: Deny,
		ExtraBash: []BashEntry{
			{Pattern: "vh-agent-harness hostile-injection", Decision: Allow},
		},
	}
	entries := computeBashBlock(rule, "test-agent", Features{}).entries

	// The hostile injection IS present in the entries (region 3)...
	found := false
	for _, e := range entries {
		if e.key == "vh-agent-harness hostile-injection" {
			found = true
			if e.val != "allow" {
				t.Fatalf("hostile injection decision = %q, want allow (it's the injected value)", e.val)
			}
		}
	}
	if !found {
		t.Fatal("hostile injection missing from entries")
	}

	// ...but findLast resolves it to deny because the 4a deny comes LATER.
	got := evalBashFindLast(entries, "vh-agent-harness hostile-injection")
	if got != "deny" {
		t.Fatalf("findLast(hostile-injection) = %q, want deny (injection should be inert under findLast)", got)
	}

	// And a canonical safe verb still resolves to allow (not shadowed by the
	// hostile injection or anything else).
	got = evalBashFindLast(entries, "vh-agent-harness doctor")
	if got != "allow" {
		t.Fatalf("findLast(doctor) = %q, want allow (canonical verb must still work)", got)
	}
}

// ---------------------------------------------------------------------------
// Test F: Legacy compatibility
// ---------------------------------------------------------------------------

// TestReadOnly_LegacyCompat_OverlayScalarDevSh: a legacy overlay pack using
// the DevSh scalar (not HarnessPolicy) still normalizes correctly. The agent
// gets the scalar-derived policy, NOT read_only.
func TestReadOnly_LegacyCompat_OverlayScalarDevSh(t *testing.T) {
	packs := []Pack{{
		Name: "legacy-pack",
		Agents: map[string]PackAgent{
			"custom-agent": {
				Location: LocationRule{
					Wildcard: Deny, Readonly: Allow, GitReadonly: Allow,
					Gate: Deny, HasGate: true,
					DevSh: Allow, // legacy scalar — no HarnessPolicy set
					Edit:  Deny,
				},
				Task: []TaskEntry{{"*", Deny}},
			},
		},
	}}
	configWithOverlay := strings.Replace(miniConfig,
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
    "custom-agent": {
      "permission": {
        "edit": "deny",
        "bash": { "__placeholder__": "deny" },
        "task": { "__placeholder__": "deny" }
      }
    }`, 1)
	out := mustEmit(t, configWithOverlay, packs, Features{})
	var root map[string]any
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatalf("parse: %v", err)
	}
	agents := root["agent"].(map[string]any)
	custom := agents["custom-agent"].(map[string]any)
	bash := custom["permission"].(map[string]any)["bash"].(map[string]any)
	// Legacy DevSh=Allow normalizes to HarnessPolicyAllow → broad allow, no 4b.
	if bash[DevShCommand] != "allow" {
		t.Fatalf("legacy DevSh=Allow: devSh decision = %v, want allow", bash[DevShCommand])
	}
	// No canonical read-only exceptions should be present.
	if _, ok := bash["vh-agent-harness doctor"]; ok {
		t.Fatal("legacy scalar agent should NOT have read_only 4b exceptions")
	}
}

// TestReadOnly_LegacyCompat_OverlayHarnessReadOnlyViaLegacyKey: a pack that
// writes "harness": "read_only" (legacy key, new value) gets read_only.
func TestReadOnly_LegacyCompat_OverlayHarnessReadOnlyViaLegacyKey(t *testing.T) {
	const pack = `{
		"agents": {
			"ro-agent": {
				"location": {
					"wildcard": "deny", "readonly": "allow", "git_readonly": "allow",
					"gate": "deny", "harness": "read_only"
				},
				"task": {"*": "deny"}
			}
		}
	}`
	parsed, err := parsePack([]byte(pack), "ro-pack")
	if err != nil {
		t.Fatalf("parsePack: %v", err)
	}
	loc := parsed.Agents["ro-agent"].Location
	if loc.HarnessPolicy != HarnessPolicyReadOnly {
		t.Errorf("HarnessPolicy = %q, want read_only", loc.HarnessPolicy)
	}
}

// TestReadOnly_LegacyCompat_CoreValidatePasses: the core tables pass validate
// after the read_only migration (no normalization conflicts).
func TestReadOnly_LegacyCompat_CoreValidatePasses(t *testing.T) {
	locations, tasks, gateExempt := resolveRules(nil)
	if err := validate(locations, tasks, gateExempt); err != nil {
		t.Fatalf("validate failed on core tables: %v", err)
	}
}
