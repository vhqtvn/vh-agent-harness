// decide_test.go — the decision-order battery (TDD slice 3). The order
// is FIXED: hard-deny → allow-rules → ask. The adversarial cases prove
// a rule file that TRIES to allow a hard-deny class still denies.
package policy

import (
	"encoding/json"
	"testing"
)

func mustPolicy(t *testing.T, src string) *Policy {
	t.Helper()
	p, err := Parse("test.policy", []byte(src))
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	return p
}

func decideOf(t *testing.T, p *Policy, tool, argsJSON string) Decision {
	t.Helper()
	return p.Decide(tool, json.RawMessage(argsJSON))
}

// THE adversarial order proof: rules that try to cover hard-deny
// shapes must still deny.
func TestDecideHardDenyBeatsAllowRules(t *testing.T) {
	p := mustPolicy(t, "[[allow]]\ntool = \"run_shell\"\n") // broadest possible
	for _, c := range []struct{ name, args string }{
		{"git push", `{"command":"git push"}`},
		{"secret env", `{"command":"GITHUB_TOKEN=x ls"}`},
		{"traversal", `{"command":"cat ../../etc/passwd"}`},
		{"escalation", `{"command":"tool --sandbox danger-full-access"}`},
		{"unrecognized", `{"command":""}`},
	} {
		if d := decideOf(t, p, "run_shell", c.args); d.Kind != DecisionDeny {
			t.Fatalf("%s: a broad run_shell allow must NOT override the hard-deny class, got %+v", c.name, d)
		}
	}

	// An argv0=git rule (legal — read-only git is rule-eligible) still
	// denies the mutation subcommands.
	gitRule := mustPolicy(t, "[[allow]]\ntool = \"run_shell\"\nargv0 = \"git\"\n")
	if d := decideOf(t, gitRule, "run_shell", `{"command":"git push"}`); d.Kind != DecisionDeny {
		t.Fatalf("argv0=git rule must not allow git push, got %+v", d)
	}
	if d := decideOf(t, gitRule, "run_shell", `{"command":"git status"}`); d.Kind != DecisionAllow {
		t.Fatalf("argv0=git rule must allow git status, got %+v", d)
	}

	// A path-prefixed rule whose prefix LEXICALLY matches a traversal
	// arg: docs/../../x starts with docs/ — the rule would allow it if
	// the order were wrong.
	docsRule := mustPolicy(t, "[[allow]]\ntool = \"read\"\npath = \"docs/\"\n")
	if d := decideOf(t, docsRule, "read", `{"path":"docs/../../etc/passwd"}`); d.Kind != DecisionDeny {
		t.Fatalf("traversal under a matching prefix must still deny, got %+v", d)
	}
}

func TestDecideUnmatchedFallsThroughToAsk(t *testing.T) {
	p := mustPolicy(t, "[[allow]]\ntool = \"clock\"\n")
	for _, c := range []struct{ tool, args string }{
		{"echo", `{"text":"x"}`},
		{"read", `{"path":"docs/x"}`},     // no rule for read
		{"run_shell", `{"command":"ls"}`}, // no rule for run_shell
		{"mystery", `{"x":1}`},            // unknown tool
	} {
		if d := p.Decide(c.tool, json.RawMessage(c.args)); d.Kind != DecisionAsk {
			t.Fatalf("%s %s: unmatched must ASK, got %+v", c.tool, c.args, d)
		}
	}

	// The empty policy asks for everything (the no---policy posture).
	empty := mustPolicy(t, "")
	if d := empty.Decide("clock", json.RawMessage(`{}`)); d.Kind != DecisionAsk {
		t.Fatalf("empty policy must ask, got %+v", d)
	}
}

func TestDecideExactToolMatch(t *testing.T) {
	p := mustPolicy(t, "[[allow]]\ntool = \"echo\"\n")
	if d := decideOf(t, p, "echo", `{"text":"hi"}`); d.Kind != DecisionAllow {
		t.Fatalf("exact echo allow, got %+v", d)
	}
	if d := decideOf(t, p, "echo2", `{"text":"hi"}`); d.Kind != DecisionAsk {
		t.Fatalf("echo2 is not echo, got %+v", d)
	}
}

func TestDecidePrefixGlob(t *testing.T) {
	p := mustPolicy(t, "[[allow]]\ntool = \"edit:*\"\n")
	if d := decideOf(t, p, "edit:big", `{}`); d.Kind != DecisionAllow {
		t.Fatalf("edit:* must match edit:big, got %+v", d)
	}
	// Glob specificity: no colon, no match (bare `edit` with `{}`
	// would separately hit the fail-closed args-shape deny, so use a
	// non-risky name to isolate the glob behavior).
	if d := decideOf(t, p, "editx", `{}`); d.Kind != DecisionAsk {
		t.Fatalf("edit:* must NOT match editx, got %+v", d)
	}
	if d := decideOf(t, p, "editor", `{}`); d.Kind != DecisionAsk {
		t.Fatalf("edit:* must NOT match editor, got %+v", d)
	}
}

func TestDecidePathPrefixConstraint(t *testing.T) {
	p := mustPolicy(t, "[[allow]]\ntool = \"read\"\npath = \"docs/\"\n")
	allow := []string{"docs/x.md", "docs/deep/nested.yml", "docs/"}
	for _, path := range allow {
		if d := decideOf(t, p, "read", `{"path":"`+path+`"}`); d.Kind != DecisionAllow {
			t.Fatalf("path %q must allow, got %+v", path, d)
		}
	}
	ask := []string{"docsx", "src/x", "/etc/passwd", "./docs/x"}
	for _, path := range ask {
		if d := decideOf(t, p, "read", `{"path":"`+path+`"}`); d.Kind != DecisionAsk {
			t.Fatalf("path %q must ask, got %+v", path, d)
		}
	}

	// A slashless prefix matches itself and its children only.
	bare := mustPolicy(t, "[[allow]]\ntool = \"read\"\npath = \"docs\"\n")
	if d := decideOf(t, bare, "read", `{"path":"docs"}`); d.Kind != DecisionAllow {
		t.Fatalf("bare prefix must match itself, got %+v", d)
	}
	if d := decideOf(t, bare, "read", `{"path":"docs/x"}`); d.Kind != DecisionAllow {
		t.Fatalf("bare prefix must match children, got %+v", d)
	}
	if d := decideOf(t, bare, "read", `{"path":"docsx"}`); d.Kind != DecisionAsk {
		t.Fatalf("bare prefix must not match docsx, got %+v", d)
	}

	// An absolute prefix matches absolute paths under it.
	abs := mustPolicy(t, "[[allow]]\ntool = \"write\"\npath = \"/tmp/build/\"\n")
	if d := decideOf(t, abs, "write", `{"path":"/tmp/build/out.bin","content":"x"}`); d.Kind != DecisionAllow {
		t.Fatalf("absolute prefix must match, got %+v", d)
	}
	if d := decideOf(t, abs, "write", `{"path":"build/out.bin","content":"x"}`); d.Kind != DecisionAsk {
		t.Fatalf("absolute prefix must not match relative, got %+v", d)
	}
}

func TestDecideArgv0Constraint(t *testing.T) {
	p := mustPolicy(t, "[[allow]]\ntool = \"run_shell\"\nargv0 = \"git\"\n")
	allow := []string{"git status", "git log -3 --oneline", "git diff --stat HEAD"}
	// (round-2 widened boundary: `git diff HEAD~1` now HARD-DENIES —
	// `~` is outside the plain-word grammar, so the ref word is
	// unidentifiable; pinned in reviewfix_test.go round 2.)
	for _, cmd := range allow {
		if d := decideOf(t, p, "run_shell", `{"command":"`+cmd+`"}`); d.Kind != DecisionAllow {
			t.Fatalf("command %q must allow, got %+v", cmd, d)
		}
	}
	ask := []string{"ls", "go test ./...", "git status && ls"} // last: one segment is not git
	for _, cmd := range ask {
		if d := decideOf(t, p, "run_shell", `{"command":"`+cmd+`"}`); d.Kind != DecisionAsk {
			t.Fatalf("command %q must ask, got %+v", cmd, d)
		}
	}
	// Path-form git also satisfies an argv0=git constraint.
	if d := decideOf(t, p, "run_shell", `{"command":"/usr/bin/git status"}`); d.Kind != DecisionAllow {
		t.Fatalf("path-form git status must allow, got %+v", d)
	}
}

func TestDecideFirstMatchingRuleWins(t *testing.T) {
	p := mustPolicy(t,
		"[[allow]]\ntool = \"read\"\npath = \"docs/\"\n"+
			"[[allow]]\ntool = \"read\"\n")
	if d := decideOf(t, p, "read", `{"path":"src/x"}`); d.Kind != DecisionAllow {
		t.Fatalf("second (broad) rule must catch src/, got %+v", d)
	}
	// Both rules match docs/x; the first one wins — observable via the
	// recorded provenance.
	if d := decideOf(t, p, "read", `{"path":"docs/x"}`); d.Kind != DecisionAllow || d.Rule == nil || d.Rule.Path != "docs/" {
		t.Fatalf("first matching rule must win, got %+v", d)
	}
}

func TestDecideAllowCarriesRuleProvenance(t *testing.T) {
	p := mustPolicy(t, "[[allow]]\ntool = \"run_shell\"\nargv0 = \"git\"\n")
	d := decideOf(t, p, "run_shell", `{"command":"git status"}`)
	if d.Kind != DecisionAllow || d.Rule == nil || d.Rule.Argv0 != "git" {
		t.Fatalf("allow decision must record the matching rule, got %+v", d)
	}
	if d := decideOf(t, p, "clock", `{}`); d.Rule != nil {
		t.Fatalf("ask decision must not claim a rule, got %+v", d)
	}
}

// TestDecideUnderscorePrefixGlob (P8.2): "prefix_*" is the
// underscore-namespace twin of "prefix:*" — the per-server allow
// shape for MCP tool names (mcp_mock_*). Anchored at the underscore:
// mcp_mock_* matches mcp_mock_echo but NOT mcp_mockery_echo; the
// whole-namespace mcp_* shape is an explicit operator choice. Deny
// direction untouched (hard-deny runs before rules regardless).
func TestDecideUnderscorePrefixGlob(t *testing.T) {
	p := mustPolicy(t, "[[allow]]\ntool = \"mcp_mock_*\"\n")
	if d := decideOf(t, p, "mcp_mock_echo", `{"text":"hi"}`); d.Kind != DecisionAllow {
		t.Fatalf("mcp_mock_* must match mcp_mock_echo, got %+v", d)
	}
	if d := decideOf(t, p, "mcp_mockery_echo", `{"text":"hi"}`); d.Kind != DecisionAsk {
		t.Fatalf("mcp_mock_* must NOT match mcp_mockery_echo (anchored at the underscore), got %+v", d)
	}
	if d := decideOf(t, p, "mcp_other_echo", `{"text":"hi"}`); d.Kind != DecisionAsk {
		t.Fatalf("mcp_mock_* must NOT match mcp_other_echo, got %+v", d)
	}
	// The whole-namespace shape: explicit, legal, operator's choice.
	pAll := mustPolicy(t, "[[allow]]\ntool = \"mcp_*\"\n")
	if d := decideOf(t, pAll, "mcp_anything", `{}`); d.Kind != DecisionAllow {
		t.Fatalf("mcp_* must match any mcp_ name, got %+v", d)
	}
	if d := decideOf(t, pAll, "mcpancake", `{}`); d.Kind != DecisionAsk {
		t.Fatalf("mcp_* must NOT match mcpancake (no underscore), got %+v", d)
	}
}
