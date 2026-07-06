package permconfig

import (
	"encoding/json"
	"fmt"
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
// shell-guard-core.js imports (GIT_MUTATION_VERBS + COMMANDS.readonly /
// .git_readonly / .gate).
func TestGenerateAllowedCommandsJS_Shape(t *testing.T) {
	js := GenerateAllowedCommandsJS()
	s := string(js)
	if !strings.HasPrefix(s, "// GENERATED by") {
		t.Fatalf("missing GENERATED header: %s", s[:60])
	}
	// GIT_MUTATION_VERBS must be emitted from the Go-canonical GitMutationVerbs
	// slice so shell-guard and exec-ro share one verb set. Single-source guard.
	if !strings.Contains(s, "export const GIT_MUTATION_VERBS = [\n") {
		t.Fatalf("missing GIT_MUTATION_VERBS export: %s", s[:120])
	}
	for _, verb := range GitMutationVerbs {
		quoted := `"` + verb + `",`
		if !strings.Contains(s, quoted) {
			t.Fatalf("git mutation verb %q missing from JS output", verb)
		}
	}
	if !strings.Contains(s, "export const COMMANDS = {\n") {
		t.Fatalf("missing export header: %s", s[:80])
	}
	// COMMANDS is emitted LAST (after GIT_MUTATION_VERBS) so the file still ends
	// with the `};\n` that consumers and this test rely on.
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

// Test 19: object-form edit for the committer. The committer's edit is emitted
// as an OBJECT map {"*":"deny","<commit-gate-glob>":"allow","tmp/**":"allow"}
// (findLast semantics — deny-* first, narrow allows last) so it can Write its
// scoped message-file path that acquire --message-file consumes, plus the
// universal tmp/** disposable-scratch carve-out. The committer is the SOLE
// agent carrying EditOverrides now (build/docs-steward reverted to broad flat
// Edit=Allow when the W1 single-writer edit-blocking was unwound). This test
// pins the committer's broad-deny+narrow-allows pattern and verifies that the
// read-only (Deny) agents (coordination, repo-explorer) emit the object form
// {"*":"deny","tmp/**":"allow"}, and that build is flat allow (NOT object-form
// — the tmp carve-out is skipped for Edit==Allow agents).
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
	// Isolate ONLY the edit object (exclude the task block that follows — it
	// would pollute the allow-count assertion with task edges).
	editObj := extractEditObjectValue(committerBlock, permIdx)
	// findLast semantics: "*" deny must appear BEFORE the narrow allows.
	starIdx := strings.Index(editObj, `"*": "deny"`)
	allowIdx := strings.Index(editObj, `"`+CommitGateMessageGlob+`": "allow"`)
	tmpIdx := strings.Index(editObj, `"`+TmpWriteGlob+`": "allow"`)
	if starIdx < 0 || allowIdx < 0 || tmpIdx < 0 {
		t.Fatalf("committer edit object missing deny-* / scoped allow / tmp allow:\n%s", editObj)
	}
	if starIdx > allowIdx {
		t.Fatalf("deny-* (pos %d) must precede the scoped allow (pos %d) — findLast semantics", starIdx, allowIdx)
	}
	if allowIdx > tmpIdx {
		t.Fatalf("scoped allow (pos %d) must precede the universal tmp allow (pos %d) — findLast + tmp-last", allowIdx, tmpIdx)
	}
	// The committer allows exactly TWO paths: its scoped commit-gate message
	// glob plus the universal tmp/** carve-out. Nothing wider, nothing extra.
	if strings.Count(editObj, `": "allow"`) != 2 {
		t.Fatalf("committer edit must allow exactly TWO paths (commit-gate glob + tmp/**), got:\n%s", editObj)
	}

	// (b) read-only (Deny) agents are now OBJECT-FORM edit: deny-* first, then
	// the universal tmp/** allow last. They were flat "deny" before the tmp
	// carve-out; the carve-out flips them to object form. The broad deny still
	// gates every non-tmp path.
	for _, agent := range []string{"coordination", "repo-explorer"} {
		blk := extractAgentBlock(t, s, agent)
		i := strings.Index(blk, `"edit"`)
		if i < 0 {
			t.Fatalf("%s block has no edit key", agent)
		}
		editVal := blk[i:]
		if !strings.HasPrefix(editVal, `"edit": {`) {
			t.Fatalf("%s edit must be object-form {\"*\":\"deny\",\"tmp/**\":\"allow\"}, got: %s", agent, firstLine(editVal))
		}
		editObj := extractEditObjectValue(blk, i)
		roStar := strings.Index(editObj, `"*": "deny"`)
		roTmp := strings.Index(editObj, `"`+TmpWriteGlob+`": "allow"`)
		if roStar < 0 || roTmp < 0 {
			t.Fatalf("%s edit object missing deny-* or tmp allow:\n%s", agent, editObj)
		}
		if roStar > roTmp {
			t.Fatalf("%s deny-* (pos %d) must precede tmp allow (pos %d) — findLast", agent, roStar, roTmp)
		}
		if strings.Count(editObj, `": "allow"`) != 1 {
			t.Fatalf("%s edit must allow exactly ONE path (tmp/**), got:\n%s", agent, editObj)
		}
	}

	// (c) build is FLAT allow (NOT object-form) — the W1 backlog deny
	// EditOverrides has been removed. This is the regression guard for the
	// unwind: build must NOT carry an object-form edit and must NOT carry the
	// backlog-path deny. If this fails, the W1 edit-blocking was re-added.
	buildBlock := extractAgentBlock(t, s, "build")
	bi := strings.Index(buildBlock, `"edit"`)
	if bi < 0 {
		t.Fatalf("build block has no edit key")
	}
	bEdit := buildBlock[bi:]
	if !strings.HasPrefix(bEdit, `"edit": "allow"`) {
		t.Fatalf("build edit must be FLAT allow (W1 backlog deny removed), got: %s", firstLine(bEdit))
	}
	if strings.Contains(bEdit, `"edit": {`) {
		t.Fatalf("build edit must NOT be object-form (W1 EditOverrides removed), got: %s", firstLine(bEdit))
	}
	if strings.Contains(bEdit, BacklogLedgerPath) {
		t.Fatalf("build edit must NOT reference the backlog path (W1 deny removed), got:\n%s", bEdit)
	}
}

// Test 20: the UNIVERSAL tmp/** disposable-scratch carve-out. Every agent in
// CoreLocationRules whose Edit decision is NOT broad Allow (i.e. every Deny or
// Ask agent) PLUS the top-level default MUST emit a tmp/**: allow entry in its
// object-form edit block; build and docs-steward (broad flat Edit=Allow) MUST
// NOT — they stay flat allow because their broad allow already covers tmp. This
// is the comprehensive pin for the single-chokepoint tmp carve-out in
// computeEditBlock, which lets every agent (including overlay-pack agents that
// lack an edit key, via the top-level default) Write the gitignored, watcher-
// ignored tmp/** scratch surface without a prompt while leaving every other
// edit decision unchanged.
func TestEmit_TmpWriteCarveOut(t *testing.T) {
	// Build a config carrying the top-level permission.edit AND one agent block
	// per CoreLocationRules entry (so the emitter processes every agent).
	const agentBlockTpl = `"%[1]s": { "permission": { "edit": "deny", "bash": {"__placeholder__":"deny"}, "task": {"__placeholder__":"deny"} } }`
	var agentBlocks []string
	agentBlocks = append(agentBlocks, `"build": { "permission": { "edit": "allow", "bash": {"__placeholder__":"deny"}, "task": {"__placeholder__":"deny"} } }`,
		`"docs-steward": { "permission": { "edit": "allow", "bash": {"__placeholder__":"deny"}, "task": {"__placeholder__":"deny"} } }`)
	for name := range CoreLocationRules {
		if name == "default" || name == "build" || name == "docs-steward" {
			continue
		}
		agentBlocks = append(agentBlocks, fmt.Sprintf(agentBlockTpl, name))
	}
	cfg := `{
  "$schema": "https://opencode.ai/config.json",
  "permission": { "edit": "ask", "bash": { "__placeholder__": "deny" } },
  "agent": {
` + strings.Join(agentBlocks, ",\n") + `
  }
}`
	out := mustEmit(t, cfg, nil, Features{})
	s := string(out)

	// (a) Top-level default: {"*":"ask","tmp/**":"allow"}. Use json.Unmarshal
	// to robustly reach root["permission"]["edit"] — the emitted output is
	// key-sorted ($schema, agent, permission), so the top-level permission
	// block comes AFTER all agent blocks and a naive string scan would land on
	// an agent's permission block instead.
	var root map[string]any
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatalf("parse output: %v", err)
	}
	topEdit, ok := root["permission"].(map[string]any)["edit"].(map[string]any)
	if !ok {
		t.Fatalf("top-level edit must be object-form, got: %T", root["permission"].(map[string]any)["edit"])
	}
	if topEdit["*"] != "ask" {
		t.Fatalf(`top-level edit["*"] = %v, want "ask"`, topEdit["*"])
	}
	if topEdit[TmpWriteGlob] != "allow" {
		t.Fatalf(`top-level edit[%q] = %v, want "allow"`, TmpWriteGlob, topEdit[TmpWriteGlob])
	}
	if len(topEdit) != 2 {
		t.Fatalf("top-level edit must have exactly TWO keys (* + tmp/**), got: %v", topEdit)
	}

	// (b) Iterate every CoreLocationRules agent. Isolate ONLY the edit object
	// so order/count assertions are not polluted by the task block that follows
	// (permission is key-sorted: bash, edit, task).
	for name, rule := range CoreLocationRules {
		if name == "default" {
			continue
		}
		blk := extractAgentBlock(t, s, name)
		i := strings.Index(blk, `"edit"`)
		if i < 0 {
			t.Fatalf("%s block has no edit key", name)
		}
		editVal := blk[i:]
		if rule.Edit == Allow && len(rule.EditOverrides) == 0 {
			// build / docs-steward: flat "allow" (tmp covered by broad allow;
			// no object form).
			if !strings.HasPrefix(editVal, `"edit": "allow"`) {
				t.Fatalf("%s edit must be FLAT allow (broad Edit=Allow skips tmp carve-out), got: %s", name, firstLine(editVal))
			}
			if strings.Contains(editVal, TmpWriteGlob) {
				t.Fatalf("%s flat-allow edit must NOT carry an explicit tmp/** entry, got: %s", name, firstLine(editVal))
			}
			continue
		}
		// Every other agent: object-form {\"*\":<Edit>,...,\"tmp/**\":\"allow\"}.
		if !strings.HasPrefix(editVal, `"edit": {`) {
			t.Fatalf("%s edit must be object-form, got: %s", name, firstLine(editVal))
		}
		editObj := extractEditObjectValue(blk, i)
		wantStar := `"*": "` + string(rule.Edit) + `"`
		if !strings.Contains(editObj, wantStar) {
			t.Fatalf("%s edit object missing %s:\n%s", name, wantStar, editObj)
		}
		if !strings.Contains(editObj, `"`+TmpWriteGlob+`": "allow"`) {
			t.Fatalf("%s edit object missing the universal tmp/** allow:\n%s", name, editObj)
		}
		// tmp/** must be the LAST allow (findLast + tmp-last invariant): no
		// `": "allow"` may appear after the tmp line. (Comparing lastIndexOf
		// positions directly is wrong because the `": "allow"` suffix pattern
		// is itself a substring of the `"tmp/**": "allow"` line.)
		tmpLine := `"` + TmpWriteGlob + `": "allow"`
		tmpPos := strings.LastIndex(editObj, tmpLine)
		if tmpPos < 0 {
			t.Fatalf("%s tmp/** allow missing:\n%s", name, editObj)
		}
		if strings.Contains(editObj[tmpPos+len(tmpLine):], `": "allow"`) {
			t.Fatalf("%s tmp/** must be the LAST allow; found allows after it:\n%s", name, editObj)
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

// extractEditObjectValue returns the substring of s covering ONLY the object
// value immediately following the "edit" key at index editIdx. It scans from
// the first '{' after editIdx to its matching '}' (bracket-depth), so the
// returned span does NOT include subsequent keys (bash/task) in the same
// permission block — the emitted permission object is key-sorted
// (bash, edit, task), so a naive blk[i:] from "edit" would leak into task and
// pollute allow-count assertions with task edges. Returns "" if the edit value
// is flat (not an object); callers handle that case via HasPrefix on the raw.
func extractEditObjectValue(s string, editIdx int) string {
	relBrace := strings.Index(s[editIdx:], "{")
	if relBrace < 0 {
		return ""
	}
	start := editIdx + relBrace
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
	return s[start:]
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
