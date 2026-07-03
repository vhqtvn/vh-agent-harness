package permconfig

import (
	"encoding/json"
	"strings"
	"testing"
)

// miniConfig is a minimal opencode.jsonc skeleton that exercises the emitter's
// two rewrite targets: the top-level permission.bash block and each agent's
// permission.bash/task blocks. It carries comments and a $schema URL with // to
// verify the string-aware JSONC stripper survives round-trip.
const miniConfig = `{
  // MANAGED — canonicalized by vh-agent-harness permconfig emitter
  "$schema": "https://opencode.ai/config.json",
  "permission": {
    "edit": "ask",
    "bash": {
      "__placeholder__": "deny"
    }
  },
  "agent": {
    "build": {
      "permission": {
        "edit": "allow",
        "bash": { "__placeholder__": "deny" },
        "task": { "__placeholder__": "deny" }
      }
    },
    "committer": {
      "permission": {
        "edit": "deny",
        "bash": { "__placeholder__": "deny" },
        "task": { "__placeholder__": "deny" }
      }
    },
    "coordination": {
      "permission": {
        "edit": "deny",
        "bash": { "__placeholder__": "deny" },
        "task": { "__placeholder__": "deny" }
      }
    },
    "repo-explorer": {
      "permission": {
        "edit": "deny",
        "bash": { "__placeholder__": "deny" },
        "task": { "__placeholder__": "deny" }
      }
    }
  }
}`

func mustEmit(t *testing.T, input string, packs []Pack, features Features) []byte {
	t.Helper()
	out, err := Emit([]byte(input), packs, features)
	if err != nil {
		t.Fatalf("Emit failed: %v", err)
	}
	return out
}

// Test 1: deterministic emission — byte-stable across repeated renders.
// The same input + packs + features MUST produce byte-identical output every
// time (Go map iteration is randomized, so any map-dependent ordering bug
// would surface here).
func TestEmit_DeterministicByteStable(t *testing.T) {
	out1 := mustEmit(t, miniConfig, nil, Features{Backlog: true})
	out2 := mustEmit(t, miniConfig, nil, Features{Backlog: true})
	if string(out1) != string(out2) {
		t.Fatalf("non-deterministic emission:\n--- run 1 ---\n%s\n--- run 2 ---\n%s", out1, out2)
	}
	// Run many times to catch intermittent map-iteration nondeterminism.
	for i := 0; i < 50; i++ {
		out := mustEmit(t, miniConfig, nil, Features{Backlog: true})
		if string(out) != string(out1) {
			t.Fatalf("non-deterministic at iteration %d", i)
		}
	}
}

// Test 2: features.backlog=true includes the normalize-backlog command in the
// top-level permission.bash block.
func TestEmit_BacklogEnabled(t *testing.T) {
	out := mustEmit(t, miniConfig, nil, Features{Backlog: true})
	s := string(out)
	if !strings.Contains(s, BacklogCommand) {
		t.Fatalf("backlog enabled: normalize-backlog command missing from output\n%s", out)
	}
	// Verify it's in the top-level permission.bash (not an agent block).
	var root map[string]any
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatalf("parse output: %v", err)
	}
	perm := root["permission"].(map[string]any)
	bash := perm["bash"].(map[string]any)
	if bash[BacklogCommand] != "allow" {
		t.Fatalf("backlog command decision = %v, want allow", bash[BacklogCommand])
	}
}

// Test 3: features.backlog=false EXCLUDES the normalize-backlog command.
// This is the regression guard for the legacy resolver's wipe bug (Q3a).
func TestEmit_BacklogDisabled(t *testing.T) {
	out := mustEmit(t, miniConfig, nil, Features{Backlog: false})
	s := string(out)
	if strings.Contains(s, "normalize-backlog") {
		t.Fatalf("backlog disabled: normalize-backlog should be absent\n%s", out)
	}
}

// Test 4: overlay permissions coexist with backlog — a pack-contributed agent
// gets its bash block canonicalized from Go tables while the top-level backlog
// entry survives. No wipe, no duplication.
func TestEmit_OverlayCoexistsWithBacklog(t *testing.T) {
	packs := []Pack{{
		Name: "test-pack",
		Agents: map[string]PackAgent{
			"custom-agent": {
				Location:     LocationRule{Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow, Edit: Deny},
				Task:         []TaskEntry{{"*", Deny}},
				DelegateFrom: []string{"build", "coordination"},
			},
		},
	}}
	// Inject the agent block into the config so the emitter finds it.
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

	out := mustEmit(t, configWithOverlay, packs, Features{Backlog: true})
	var root map[string]any
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatalf("parse: %v", err)
	}
	// Backlog survives in top-level bash.
	perm := root["permission"].(map[string]any)
	bash := perm["bash"].(map[string]any)
	if bash[BacklogCommand] != "allow" {
		t.Fatalf("backlog entry missing while overlay active: %v", bash[BacklogCommand])
	}
	// Custom agent got canonicalized (has the readonly entries).
	agents := root["agent"].(map[string]any)
	custom := agents["custom-agent"].(map[string]any)
	cperm := custom["permission"].(map[string]any)
	cbash := cperm["bash"].(map[string]any)
	if cbash["ls *"] != "allow" {
		t.Fatalf("custom agent bash not canonicalized: %v", cbash)
	}
}

// Test 5: overlay delegateFrom edges appear in orchestrator task allowlists.
// A pack agent with delegateFrom:["build"] injects "custom-agent":"allow" into
// build's task block.
func TestEmit_DelegateFromEdges(t *testing.T) {
	packs := []Pack{{
		Name: "test-pack",
		Agents: map[string]PackAgent{
			"custom-agent": {
				Location:     LocationRule{Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow, Edit: Deny},
				Task:         []TaskEntry{{"*", Deny}},
				DelegateFrom: []string{"build"},
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
	json.Unmarshal(out, &root)
	agents := root["agent"].(map[string]any)
	build := agents["build"].(map[string]any)
	bperm := build["permission"].(map[string]any)
	btask := bperm["task"].(map[string]any)
	if btask["custom-agent"] != "allow" {
		t.Fatalf("delegateFrom edge missing: build.task should have custom-agent:allow, got %v", btask["custom-agent"])
	}
}

// Test 6: unresolved placeholder detection — the emitter overwrites
// __placeholder__ blocks; if a block is MISSING entirely (agent declared in
// tables but absent from config), the emitter skips it gracefully (no panic).
// This is the Slice 1 detector's concern; here we verify the emitter doesn't
// choke on a sparse config and DOES overwrite placeholders.
func TestEmit_OverwritesPlaceholder(t *testing.T) {
	out := mustEmit(t, miniConfig, nil, Features{})
	s := string(out)
	if strings.Contains(s, "__placeholder__") {
		t.Fatalf("placeholder survived emission:\n%s", out)
	}
	// build should have its canonical bash block (e.g. "ls *": "allow").
	var root map[string]any
	json.Unmarshal(out, &root)
	agents := root["agent"].(map[string]any)
	build := agents["build"].(map[string]any)
	bperm := build["permission"].(map[string]any)
	bash := bperm["bash"].(map[string]any)
	if bash["ls *"] != "allow" {
		t.Fatalf("build bash not canonicalized: %v", bash)
	}
	// build is gate-exempt → must NOT carry any gate MUTATION command.
	// (.opencode/scripts/commit-gate.sh status is a pure-read metadata probe
	// and lives in the readonly group, so build legitimately gets it with
	// "allow" — see Q2 split. Only mutation verbs stay gate-grouped.)
	for k := range bash {
		if !strings.HasPrefix(k, ".opencode/scripts/commit-gate.sh") {
			continue
		}
		if strings.HasSuffix(k, " status") {
			continue // read-only status check; allowed for all agents
		}
		t.Fatalf("build (gate-exempt) has gate mutation command %q in bash block", k)
	}
	// committer is NOT gate-exempt → gate commands present with "allow".
	committer := agents["committer"].(map[string]any)
	cperm := committer["permission"].(map[string]any)
	cbash := cperm["bash"].(map[string]any)
	if cbash[".opencode/scripts/commit-gate.sh status"] != "allow" {
		t.Fatalf("committer readonly status command should be allow, got %v", cbash[".opencode/scripts/commit-gate.sh status"])
	}
}

// Test 7: bash block key order — "*" first, then sorted commands, then
// "vh-agent-harness *" last. Verifies orderedMap preserves insertion order in
// the RAW emitted bytes (not a re-parsed version, which would lose order).
func TestEmit_BashBlockOrder(t *testing.T) {
	out := mustEmit(t, miniConfig, nil, Features{})
	s := string(out)
	// The top-level bash block appears first in the output. Find key positions.
	starIdx := strings.Index(s, `"*": "`)
	lsIdx := strings.Index(s, `"ls *": "`)
	devShIdx := strings.Index(s, `"`+DevShCommand+`": "`)
	if starIdx < 0 {
		t.Fatalf("wildcard * missing from bash block")
	}
	if lsIdx < 0 {
		t.Fatalf("ls * missing from bash block")
	}
	if devShIdx < 0 {
		t.Fatalf("devSh %s missing from bash block", DevShCommand)
	}
	if starIdx > lsIdx {
		t.Fatalf("* (pos %d) should appear before ls * (pos %d)", starIdx, lsIdx)
	}
	if lsIdx > devShIdx {
		t.Fatalf("ls * (pos %d) should appear before %s (pos %d)", lsIdx, DevShCommand, devShIdx)
	}
}

// Test 8: GenerateAllowedCommandsJS produces the exact export shape that
// shell-guard-core.js imports (COMMANDS.readonly / .git_readonly / .gate).
func TestGenerateAllowedCommandsJS_Shape(t *testing.T) {
	js := GenerateAllowedCommandsJS()
	s := string(js)
	if !strings.HasPrefix(s, "// GENERATED by") {
		t.Fatalf("missing GENERATED header: %s", s[:60])
	}
	if !strings.Contains(s, "export const COMMANDS = {\n") {
		t.Fatalf("missing export header: %s", s[:80])
	}
	if !strings.HasSuffix(s, "};\n") {
		t.Fatalf("missing closing };\\n: ...%q", s[len(s)-10:])
	}
	// Each group must appear with its commands.
	for _, group := range CommandGroups {
		header := group.Name + ": ["
		if !strings.Contains(s, header) {
			t.Fatalf("group %q missing from JS output", group.Name)
		}
		for _, cmd := range group.Commands {
			quoted := `"` + cmd + `",`
			if !strings.Contains(s, quoted) {
				t.Fatalf("command %q missing from group %q in JS output", cmd, group.Name)
			}
		}
	}
}

// Test 9: GenerateAllowedCommandsJS is deterministic (byte-stable).
func TestGenerateAllowedCommandsJS_Deterministic(t *testing.T) {
	a := GenerateAllowedCommandsJS()
	for i := 0; i < 20; i++ {
		b := GenerateAllowedCommandsJS()
		if string(a) != string(b) {
			t.Fatalf("GenerateAllowedCommandsJS non-deterministic at iteration %d", i)
		}
	}
}

// Test 9b: the canonical command roster reflects the Q1a/Q1b/Q2 changes —
// git_readonly carries the 12 `git --no-pager <sub> *` readonly forms (config-
// table match that kills the prompt for `git --no-pager log` etc.), and
// commit-gate.sh status lives in readonly (pure-read metadata probe), NOT in
// gate (which is committer-only and carries only mutation verbs).
func TestCommandGroups_NoPagerReadonlyAndStatusSplit(t *testing.T) {
	findGroup := func(name string) CommandGroup {
		t.Helper()
		for _, g := range CommandGroups {
			if g.Name == name {
				return g
			}
		}
		t.Fatalf("group %q not found", name)
		return CommandGroup{}
	}
	has := func(group CommandGroup, cmd string) bool {
		for _, c := range group.Commands {
			if c == cmd {
				return true
			}
		}
		return false
	}

	// Q1b: every bare `git <sub> *` readonly subcommand has a paired
	// `git --no-pager <sub> *` form (so `--no-pager` between `git` and the
	// sub does not fall through to the permission prompt).
	bareSubs := []string{
		"diff", "log", "show", "grep", "blame", "ls-tree",
		"status", "ls-files", "check-ignore", "cat-file", "show-ref", "rev-parse",
	}
	gitReadonly := findGroup("git_readonly")
	for _, sub := range bareSubs {
		bare := "git " + sub + " *"
		noPager := "git --no-pager " + sub + " *"
		if !has(gitReadonly, bare) {
			t.Errorf("git_readonly missing bare form %q", bare)
		}
		if !has(gitReadonly, noPager) {
			t.Errorf("git_readonly missing --no-pager form %q (Q1b prompt fix)", noPager)
		}
	}

	// Q2: commit-gate.sh status is a pure-read metadata probe → readonly,
	// available to ALL agents (including gate-exempt ones) without a prompt.
	readonly := findGroup("readonly")
	if !has(readonly, ".opencode/scripts/commit-gate.sh status") {
		t.Errorf("readonly group must contain commit-gate.sh status (Q2 split: pure-read metadata probe)")
	}
	// The gate group (committer-only) keeps ONLY mutation verbs; status must
	// NOT also remain there (it would be a redundant entry and would defeat
	// the prompt-free read for gate-exempt agents).
	gate := findGroup("gate")
	if has(gate, ".opencode/scripts/commit-gate.sh status") {
		t.Errorf("gate group must NOT contain commit-gate.sh status (moved to readonly in Q2 split)")
	}
	for _, mutation := range []string{
		".opencode/scripts/commit-gate.sh acquire *",
		".opencode/scripts/commit-gate.sh commit *",
		".opencode/scripts/commit-gate.sh release *",
		".opencode/scripts/commit-gate.sh heartbeat *",
		".opencode/scripts/commit-gate.sh revert *",
		".opencode/scripts/commit-gate.sh stage-message *",
	} {
		if !has(gate, mutation) {
			t.Errorf("gate group must still contain mutation verb %q", mutation)
		}
	}
}

// Test 10: validate catches gate-exempt agent that wrongly carries a gate key.
func TestValidate_GateExemptWithGate(t *testing.T) {
	// "evil" agent is gate-exempt but has HasGate=true.
	locs := map[string]LocationRule{
		"default": {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow, Edit: Deny},
		"evil":    {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow, Edit: Deny},
	}
	tasks := map[string][]TaskEntry{
		"evil": {{"*", Deny}},
	}
	gateExempt := map[string]bool{"evil": true}
	err := validate(locs, tasks, gateExempt)
	if err == nil {
		t.Fatal("expected error for gate-exempt agent with gate key")
	}
	if !strings.Contains(err.Error(), "gate-exempt") {
		t.Fatalf("wrong error: %v", err)
	}
}

// Test 11: validate catches task rule referencing an unknown agent.
func TestValidate_TaskTargetUnknown(t *testing.T) {
	locs := map[string]LocationRule{
		"default": {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow, Edit: Deny},
		"build":   {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow, Edit: Deny},
	}
	tasks := map[string][]TaskEntry{
		"build": {{"*", Deny}, {"ghost", Allow}}, // "ghost" is not a known location
	}
	gateExempt := map[string]bool{}
	err := validate(locs, tasks, gateExempt)
	if err == nil {
		t.Fatal("expected error for unknown task target")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("wrong error: %v", err)
	}
}

// Test 12: schema URL with // survives emission (string-aware stripper).
func TestEmit_SchemaURLPreserved(t *testing.T) {
	out := mustEmit(t, miniConfig, nil, Features{})
	var root map[string]any
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if root["$schema"] != "https://opencode.ai/config.json" {
		t.Fatalf("$schema corrupted: %v", root["$schema"])
	}
}

// Test 13: comments are dropped in normalized output (Q1b).
func TestEmit_CommentsDropped(t *testing.T) {
	out := mustEmit(t, miniConfig, nil, Features{})
	s := string(out)
	if strings.Contains(s, "// MANAGED") {
		t.Fatalf("comment survived in normalized output")
	}
}

// Test 14: core canonical tables validate cleanly (the shipped default set
// must always pass its own contract).
func TestValidate_CoreTablesPass(t *testing.T) {
	locs := make(map[string]LocationRule, len(CoreLocationRules))
	for k, v := range CoreLocationRules {
		locs[k] = v
	}
	tasks := make(map[string][]TaskEntry, len(CoreTaskRules))
	for k, v := range CoreTaskRules {
		cp := make([]TaskEntry, len(v))
		copy(cp, v)
		tasks[k] = cp
	}
	ge := make(map[string]bool, len(GateExemptBase))
	for k, v := range GateExemptBase {
		ge[k] = v
	}
	if err := validate(locs, tasks, ge); err != nil {
		t.Fatalf("core tables fail validation: %v", err)
	}
}

// Test 15: multi-agent delegateFrom determinism — two pack agents sharing one
// delegateFrom target MUST produce byte-stable output regardless of Go map
// iteration order. Runs 50 iterations to catch intermittent nondeterminism.
func TestEmit_MultiAgentDelegateFromDeterministic(t *testing.T) {
	packs := []Pack{{
		Name: "multi-pack",
		Agents: map[string]PackAgent{
			"agent-zebra": {
				Location:     LocationRule{Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow, Edit: Deny},
				Task:         []TaskEntry{{"*", Deny}},
				DelegateFrom: []string{"build"},
			},
			"agent-alpha": {
				Location:     LocationRule{Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow, Edit: Deny},
				Task:         []TaskEntry{{"*", Deny}},
				DelegateFrom: []string{"build"},
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
    "agent-zebra": {
      "permission": {
        "edit": "deny",
        "bash": { "__placeholder__": "deny" },
        "task": { "__placeholder__": "deny" }
      }
    },
    "agent-alpha": {
      "permission": {
        "edit": "deny",
        "bash": { "__placeholder__": "deny" },
        "task": { "__placeholder__": "deny" }
      }
    }`, 1)

	out1 := mustEmit(t, configWithOverlay, packs, Features{})
	for i := 0; i < 50; i++ {
		out := mustEmit(t, configWithOverlay, packs, Features{})
		if string(out) != string(out1) {
			t.Fatalf("non-deterministic at iteration %d:\n--- first ---\n%s\n--- iter ---\n%s", i, out1, out)
		}
	}
	// Verify BOTH delegateFrom edges are present in build's task block.
	var root map[string]any
	json.Unmarshal(out1, &root)
	agents := root["agent"].(map[string]any)
	build := agents["build"].(map[string]any)
	btask := build["permission"].(map[string]any)["task"].(map[string]any)
	if btask["agent-alpha"] != "allow" || btask["agent-zebra"] != "allow" {
		t.Fatalf("both delegateFrom edges should be present: %v", btask)
	}
}

// Test 16: validate catches an invalid wildcard decision (F5 regression guard).
// Previously "*" was skipped before decision validation, so {"*":"bogus"} passed.
func TestValidate_WildcardInvalidDecision(t *testing.T) {
	locs := map[string]LocationRule{
		"default": {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow, Edit: Deny},
		"build":   {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow, Edit: Deny},
	}
	tasks := map[string][]TaskEntry{
		"build": {{"*", Decision("bogus")}},
	}
	gateExempt := map[string]bool{}
	err := validate(locs, tasks, gateExempt)
	if err == nil {
		t.Fatal("expected error for invalid wildcard decision")
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("wrong error: %v", err)
	}
}

// (extractKeys helper removed — key-order is verified via raw byte position
// inspection in TestEmit_BashBlockOrder, which doesn't lose order to re-parse.)

// Test 19: object-form edit for the committer ONLY. The committer is the sole
// agent whose edit is emitted as an OBJECT map {"*":"deny","<glob>":"allow"}
// (findLast semantics — deny-* first, narrow allow last) so it can Write ONE
// scoped message-file path that acquire --message-file consumes. Every other
// agent MUST stay flat edit unchanged. This is the load-bearing assertion for
// the Q3 message-staging rework.
func TestEmit_CommitterObjectFormEdit(t *testing.T) {
	out := mustEmit(t, miniConfig, nil, Features{})
	s := string(out)

	// (a) committer: object-form edit, deny-* FIRST then the scoped allow LAST.
	committerBlock := extractAgentBlock(t, s, "committer")
	permIdx := strings.Index(committerBlock, `"edit"`)
	if permIdx < 0 {
		t.Fatalf("committer block has no edit key:\n%s", committerBlock)
	}
	// The edit value must be an object, not a flat string.
	editSlice := committerBlock[permIdx:]
	if !strings.HasPrefix(editSlice, `"edit": {`) {
		t.Fatalf("committer edit must be object-form, got: %s", firstLine(editSlice))
	}
	// findLast semantics: "*" deny must appear BEFORE the narrow allow.
	starIdx := strings.Index(editSlice, `"*": "deny"`)
	allowIdx := strings.Index(editSlice, `"`+CommitGateMessageGlob+`": "allow"`)
	if starIdx < 0 || allowIdx < 0 {
		t.Fatalf("committer edit object missing deny-* or scoped allow:\n%s", editSlice)
	}
	if starIdx > allowIdx {
		t.Fatalf("deny-* (pos %d) must precede the scoped allow (pos %d) — findLast semantics", starIdx, allowIdx)
	}
	// The scoped glob is exactly the ONE allowlisted path; nothing wider.
	if strings.Contains(editSlice, `"allow"`) && strings.Count(editSlice, `": "allow"`) != 1 {
		t.Fatalf("committer edit must allow exactly ONE path, got:\n%s", editSlice)
	}

	// (b) build + coordination stay FLAT edit unchanged (allow / deny).
	for _, agent := range []struct{ name, want string }{
		{"build", "allow"},
		{"coordination", "deny"},
		{"repo-explorer", "deny"},
	} {
		blk := extractAgentBlock(t, s, agent.name)
		i := strings.Index(blk, `"edit"`)
		if i < 0 {
			t.Fatalf("%s block has no edit key", agent.name)
		}
		line := firstLine(blk[i:])
		want := `"edit": "` + agent.want + `"`
		if !strings.HasPrefix(blk[i:], want) {
			t.Fatalf("%s edit must be flat %q, got: %s", agent.name, agent.want, line)
		}
	}
}

// extractAgentBlock returns the substring of the emitted config covering one
// agent block (from `"name": {` to the matching closing brace at the agent
// level). It is a naive bracket-depth scanner sufficient for the test fixture.
func extractAgentBlock(t *testing.T, s, name string) string {
	t.Helper()
	start := strings.Index(s, `"`+name+`": {`)
	if start < 0 {
		t.Fatalf("agent %q block not found in emitted config", name)
	}
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	t.Fatalf("agent %q block unterminated", name)
	return ""
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// Test 17: present-agent filter drops task edges to absent agents (Phase 3
// capability-gating seam). When the rendered config omits an agent block
// (because a template capability gate hid it), every task edge pointing at
// that agent must be dropped so no orchestrator carries a dangling allow
// entry. The "*" wildcard is always preserved. This is the unit-level pin for
// the graceful-degradation contract; the end-to-end proof lives in
// internal/cli/capability_render_test.go.
func TestEmit_PresentAgentFilterDropsAbsentTaskEdges(t *testing.T) {
	// build is present and is an orchestrator; repo-explorer is present and is
	// a valid task target; committer / commit-message / commit-reviewer are
	// ABSENT (gated out). Per CoreTaskRules, build's task allowlist references
	// all of them — the absent ones must be dropped, the present one kept.
	const cfg = `{
  "$schema": "https://opencode.ai/config.json",
  "permission": { "bash": { "__placeholder__": "deny" } },
  "agent": {
    "build":         { "permission": { "bash": {"__placeholder__":"deny"}, "task": {"__placeholder__":"deny"} } },
    "repo-explorer": { "permission": { "bash": {"__placeholder__":"deny"}, "task": {"__placeholder__":"deny"} } }
  }
}`
	out := mustEmit(t, cfg, nil, Features{})
	var root map[string]any
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatalf("parse: %v", err)
	}
	build := root["agent"].(map[string]any)["build"].(map[string]any)
	task := build["permission"].(map[string]any)["task"].(map[string]any)

	// "*" wildcard always survives.
	if task["*"] != "deny" {
		t.Fatalf(`build.task["*"] = %v, want "deny"`, task["*"])
	}
	// repo-explorer is present → its edge is kept.
	if task["repo-explorer"] != "allow" {
		t.Fatalf(`build.task["repo-explorer"] = %v, want "allow" (agent is present)`, task["repo-explorer"])
	}
	// Absent gated-commit agents → their edges are dropped (no dangling allow).
	for _, absent := range []string{"committer", "commit-message", "commit-reviewer", "debate", "solution-brief"} {
		if v, ok := task[absent]; ok {
			t.Fatalf(`build.task[%q] = %v, want ABSENT (agent gated out → graceful degradation)`, absent, v)
		}
	}
}

// Test 18: present-agent filter keeps ALL task edges when every referenced
// agent is present (backward-compat invariant). A full roster must render
// build's complete task allowlist byte-identically to pre-Phase-3 — the filter
// is a strict no-op when nothing is gated out.
func TestEmit_PresentAgentFilterNoopWhenAllPresent(t *testing.T) {
	// Every agent CoreTaskRules["build"] references gets a block.
	const cfg = `{
  "$schema": "https://opencode.ai/config.json",
  "permission": { "bash": { "__placeholder__": "deny" } },
  "agent": {
    "build": { "permission": { "bash": {"__placeholder__":"deny"}, "task": {"__placeholder__":"deny"} } }
  }
}`
	// Augment with every build task target so all are present.
	full := strings.Replace(cfg, `"build": { "permission": { "bash": {"__placeholder__":"deny"}, "task": {"__placeholder__":"deny"} } }`,
		strings.Join([]string{
			`"build": { "permission": { "bash": {"__placeholder__":"deny"}, "task": {"__placeholder__":"deny"} } }`,
			`"commit-message": { "permission": { "bash": {"__placeholder__":"deny"}, "task": {"__placeholder__":"deny"} } }`,
			`"project-coordinator": { "permission": { "bash": {"__placeholder__":"deny"}, "task": {"__placeholder__":"deny"} } }`,
			`"planner": { "permission": { "bash": {"__placeholder__":"deny"}, "task": {"__placeholder__":"deny"} } }`,
			`"researcher": { "permission": { "bash": {"__placeholder__":"deny"}, "task": {"__placeholder__":"deny"} } }`,
			`"repo-explorer": { "permission": { "bash": {"__placeholder__":"deny"}, "task": {"__placeholder__":"deny"} } }`,
			`"commit-reviewer": { "permission": { "bash": {"__placeholder__":"deny"}, "task": {"__placeholder__":"deny"} } }`,
			`"ship-review": { "permission": { "bash": {"__placeholder__":"deny"}, "task": {"__placeholder__":"deny"} } }`,
			`"committer": { "permission": { "bash": {"__placeholder__":"deny"}, "task": {"__placeholder__":"deny"} } }`,
			`"docs-steward": { "permission": { "bash": {"__placeholder__":"deny"}, "task": {"__placeholder__":"deny"} } }`,
			`"debate": { "permission": { "bash": {"__placeholder__":"deny"}, "task": {"__placeholder__":"deny"} } }`,
			`"solution-brief": { "permission": { "bash": {"__placeholder__":"deny"}, "task": {"__placeholder__":"deny"} } }`,
		}, ",\n    "), 1)
	out := mustEmit(t, full, nil, Features{})
	var root map[string]any
	json.Unmarshal(out, &root)
	task := root["agent"].(map[string]any)["build"].(map[string]any)["permission"].(map[string]any)["task"].(map[string]any)
	// Every CoreTaskRules["build"] entry must survive.
	for _, e := range CoreTaskRules["build"] {
		if task[e.Target] != string(e.Decision) {
			t.Fatalf(`build.task[%q] = %v, want %q (full roster → no filtering)`, e.Target, task[e.Target], e.Decision)
		}
	}
}
