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
    "bash": {
      "__placeholder__": "deny"
    }
  },
  "agent": {
    "build": {
      "permission": {
        "bash": { "__placeholder__": "deny" },
        "task": { "__placeholder__": "deny" }
      }
    },
    "committer": {
      "permission": {
        "bash": { "__placeholder__": "deny" },
        "task": { "__placeholder__": "deny" }
      }
    },
    "coordination": {
      "permission": {
        "bash": { "__placeholder__": "deny" },
        "task": { "__placeholder__": "deny" }
      }
    },
    "repo-explorer": {
      "permission": {
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
				Location:     LocationRule{Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow},
				Task:         []TaskEntry{{"*", Deny}},
				DelegateFrom: []string{"build", "coordination"},
			},
		},
	}}
	// Inject the agent block into the config so the emitter finds it.
	configWithOverlay := strings.Replace(miniConfig,
		`"repo-explorer": {
      "permission": {
        "bash": { "__placeholder__": "deny" },
        "task": { "__placeholder__": "deny" }
      }
    }`,
		`"repo-explorer": {
      "permission": {
        "bash": { "__placeholder__": "deny" },
        "task": { "__placeholder__": "deny" }
      }
    },
    "custom-agent": {
      "permission": {
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
				Location:     LocationRule{Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow},
				Task:         []TaskEntry{{"*", Deny}},
				DelegateFrom: []string{"build"},
			},
		},
	}}
	configWithOverlay := strings.Replace(miniConfig,
		`"repo-explorer": {
      "permission": {
        "bash": { "__placeholder__": "deny" },
        "task": { "__placeholder__": "deny" }
      }
    }`,
		`"repo-explorer": {
      "permission": {
        "bash": { "__placeholder__": "deny" },
        "task": { "__placeholder__": "deny" }
      }
    },
    "custom-agent": {
      "permission": {
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
	// build is gate-exempt → must NOT have a gate command entry.
	for k := range bash {
		if strings.HasPrefix(k, ".opencode/scripts/commit-gate.sh") {
			t.Fatalf("build (gate-exempt) has gate command %q in bash block", k)
		}
	}
	// committer is NOT gate-exempt → gate commands present with "allow".
	committer := agents["committer"].(map[string]any)
	cperm := committer["permission"].(map[string]any)
	cbash := cperm["bash"].(map[string]any)
	if cbash[".opencode/scripts/commit-gate.sh status"] != "allow" {
		t.Fatalf("committer gate command should be allow, got %v", cbash[".opencode/scripts/commit-gate.sh status"])
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
	if !strings.HasPrefix(s, "export const COMMANDS = {\n") {
		t.Fatalf("missing export header: %s", s[:60])
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

// Test 10: validate catches gate-exempt agent that wrongly carries a gate key.
func TestValidate_GateExemptWithGate(t *testing.T) {
	// "evil" agent is gate-exempt but has HasGate=true.
	locs := map[string]LocationRule{
		"default": {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow},
		"evil":    {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow},
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
		"default": {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow},
		"build":   {Wildcard: Deny, Readonly: Allow, GitReadonly: Allow, Gate: Deny, HasGate: true, DevSh: Allow},
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

// (extractKeys helper removed — key-order is verified via raw byte position
// inspection in TestEmit_BashBlockOrder, which doesn't lose order to re-parse.)
