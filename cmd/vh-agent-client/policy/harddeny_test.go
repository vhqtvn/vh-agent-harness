// harddeny_test.go — the HARD-DENY invariant battery (TDD slice 2). No
// rule file can override these classes: they run BEFORE allow-rules
// are even consulted (order proven in decide_test.go). Uncertainty
// denies: an unrecognized args shape on a known-risky tool is a deny,
// never an allow.
package policy

import (
	"encoding/json"
	"strings"
	"testing"
)

func wantDeny(t *testing.T, name, tool, argsJSON, reasonKey string) {
	t.Helper()
	reason, denied := HardDeny(tool, json.RawMessage(argsJSON))
	if !denied {
		t.Fatalf("%s: expected a hard-deny, got none (tool=%s args=%s)", name, tool, argsJSON)
	}
	if !strings.Contains(reason, reasonKey) {
		t.Fatalf("%s: reason %q must contain %q", name, reason, reasonKey)
	}
}

func wantNoDeny(t *testing.T, name, tool, argsJSON string) {
	t.Helper()
	reason, denied := HardDeny(tool, json.RawMessage(argsJSON))
	if denied {
		t.Fatalf("%s: must NOT hard-deny (tool=%s args=%s), got %q", name, tool, argsJSON, reason)
	}
}

// --- class 0: unrecognized args on known-risky tools (uncertainty = deny)

func TestHardDenyUnrecognizedArgsFailClosed(t *testing.T) {
	wantDeny(t, "empty command", "run_shell", `{"command":""}`, "unrecognized")
	wantDeny(t, "missing command", "run_shell", `{}`, "unrecognized")
	wantDeny(t, "command not a string", "run_shell", `{"command":42}`, "unrecognized")
	wantDeny(t, "args not an object", "run_shell", `["ls"]`, "unrecognized")
	wantDeny(t, "args invalid json", "run_shell", `{nope`, "unrecognized")
	wantDeny(t, "unknown run_shell key", "run_shell", `{"command":"ls","cwd":"/tmp"}`, "unrecognized")
	wantDeny(t, "write missing path", "write", `{"content":"x"}`, "unrecognized")
	wantDeny(t, "read path wrong type", "read", `{"path":7}`, "unrecognized")
	wantDeny(t, "edit empty path", "edit", `{"path":"","old":"a","new":"b"}`, "unrecognized")
	wantDeny(t, "read unknown key", "read", `{"path":"a","root":"/"}`, "unrecognized")

	// Non-risky tools carry no shape requirement: garbage args fall
	// through (the daemon rejects them; the policy just asks).
	wantNoDeny(t, "echo garbage args", "echo", `{nope`)
	wantNoDeny(t, "echo no args", "echo", ``)
	wantNoDeny(t, "unknown tool garbage", "mystery", `42`)
}

// --- class 1: secret env-var writes

func TestHardDenySecretEnvWrites(t *testing.T) {
	wantDeny(t, "prefix assignment", "run_shell", `{"command":"GITHUB_TOKEN=x git push"}`, "secret")
	wantDeny(t, "export", "run_shell", `{"command":"export DEPLOY_SECRET=nope"}`, "secret")
	wantDeny(t, "engine prefix", "run_shell", `{"command":"VH_AGENT_HARNESS_JWT_SECRET=k ./run"}`, "VH_AGENT_HARNESS_")
	wantDeny(t, "KEY substring", "run_shell", `{"command":"MY_API_KEY=1 ls"}`, "secret")
	wantDeny(t, "case-insensitive", "run_shell", `{"command":"my_password=1 ls"}`, "secret")
	wantDeny(t, "env arg array", "spawn", `{"env":["AWS_SECRET_ACCESS_KEY=x"]}`, "secret")
	wantDeny(t, "env arg object", "spawn", `{"env":{"SESSION_TOKEN":"t"}}`, "secret")
	wantDeny(t, "mid-command assignment", "run_shell", `{"command":"env DEPLOY_KEY=v tool"}`, "secret")

	wantNoDeny(t, "benign env prefix", "run_shell", `{"command":"FOO=bar ls"}`)
	wantNoDeny(t, "benign env arg", "spawn", `{"env":{"FOO":"1"}}`)
	// A word that merely CONTAINS "TOKEN" without assignment shape is
	// not an env write (the pattern binds to NAME= shapes only).
	wantNoDeny(t, "token in prose", "run_shell", `{"command":"echo token list"}`)
}

// --- class 2: git-mutation argv[0]

func TestHardDenyGitMutation(t *testing.T) {
	for _, sub := range []string{
		"push", "reset", "clean", "checkout", "branch", "merge", "rebase",
		"cherry-pick", "revert", "restore", "worktree", "commit", "tag",
		"stash", "apply", "am", "clone", "fetch", "pull", "add", "rm",
		"mv", "switch", "config", "update-ref", "gc", "prune",
	} {
		wantDeny(t, "git "+sub, "run_shell", `{"command":"git `+sub+`"}`, "git")
	}
	wantDeny(t, "git with args", "run_shell", `{"command":"git push origin main --force"}`, "git")
	wantDeny(t, "flags before subcommand", "run_shell", `{"command":"git -C /repo push"}`, "git")
	wantDeny(t, "bare git", "run_shell", `{"command":"git"}`, "git")
	wantDeny(t, "compound hides mutation", "run_shell", `{"command":"git status && git push"}`, "git")
	wantDeny(t, "env-wrapped git", "run_shell", `{"command":"env NAME=v git push"}`, "git")
	wantDeny(t, "path argv0 basename", "run_shell", `{"command":"/usr/bin/git push"}`, "git")
	wantDeny(t, "quoted subcommand evasion", "run_shell", `{"command":"git \"pu\"sh"}`, "git")
	wantDeny(t, "quoted argv0 evasion", "run_shell", `{"command":"gi\"t\" push"}`, "unidentifiable")
	wantDeny(t, "command substitution", "run_shell", `{"command":"$(git push)"}`, "git")
	wantDeny(t, "subshell git", "run_shell", `{"command":"( git push )"}`, "git")

	// Read-only git stays rule-eligible (NOT hard-denied).
	for _, sub := range []string{"status", "log", "diff", "show", "grep", "describe"} {
		wantNoDeny(t, "read-only git "+sub, "run_shell", `{"command":"git `+sub+`"}`)
	}
	wantNoDeny(t, "plain command", "run_shell", `{"command":"ls -la"}`)
	wantNoDeny(t, "compound benign", "run_shell", `{"command":"go test ./... && go vet ./..."}`)
	// Documented false-positive direction: a quoted "&&" lexically
	// splits the command, and the quoted tail segment's first word is
	// not a plain argv[0] → deny (uncertainty denies; the deny is the
	// fail-closed direction, documented in decide.go).
	wantDeny(t, "quoted compound text", "run_shell", `{"command":"echo \"a && b\""}`, "unidentifiable")
}

// --- class 3: confinement-escape path shapes

func TestHardDenyConfinementEscape(t *testing.T) {
	wantDeny(t, "read traversal", "read", `{"path":"../etc/passwd"}`, "traversal")
	wantDeny(t, "write mid traversal", "write", `{"path":"docs/../../x","content":"x"}`, "traversal")
	wantDeny(t, "edit inner traversal", "edit", `{"path":"a/b/../c","old":"x","new":"y"}`, "traversal")
	wantDeny(t, "workdir traversal", "run_shell", `{"command":"ls","workdir":".."}`, "traversal")
	wantDeny(t, "command traversal word", "run_shell", `{"command":"cat ../../etc/passwd"}`, "traversal")
	wantDeny(t, "command embedded traversal", "run_shell", `{"command":"cp /x/../y z"}`, "traversal")

	// Boundary (documented): absolute paths are NOT hard-denied — the
	// client cannot know the daemon's configured roots, so absolute
	// paths fall through to rule matching (a relative-rooted rule
	// never matches them → ask).
	wantNoDeny(t, "absolute path not provable", "read", `{"path":"/etc/passwd"}`)
	// A bare ".." word lexically indistinguishable from `cat ..`
	// denies (fail-closed); ellipsis prose does not.
	wantDeny(t, "bare dotdot word", "run_shell", `{"command":"echo .."}`, "traversal")
	wantNoDeny(t, "ellipsis prose", "run_shell", `{"command":"echo \"...\""}`)
	wantNoDeny(t, "regex dots are not traversal", "search", `{"pattern":"a..b","glob":"*.go"}`)
	wantNoDeny(t, "clean path", "read", `{"path":"docs/x.md"}`)
}

// --- class 4: sandbox-mode escalation vocabulary

func TestHardDenySandboxEscalation(t *testing.T) {
	wantDeny(t, "danger-full-access flag", "run_shell", `{"command":"tool --sandbox danger-full-access"}`, "sandbox")
	wantDeny(t, "sandbox flag with value", "run_shell", `{"command":"vh-agent-harness exec --sandbox=off build"}`, "sandbox")
	wantDeny(t, "sandbox flag long form", "run_shell", `{"command":"x --sandbox-mode read-only"}`, "sandbox")
	wantDeny(t, "override key snake", "ask_tool", `{"text":"x","sandbox_mode":"off"}`, "sandbox")
	wantDeny(t, "override key camel", "ask_tool", `{"sandboxPermissions":"all"}`, "sandbox")
	wantDeny(t, "bare key", "ask_tool", `{"sandbox":"off"}`, "sandbox")
	wantDeny(t, "nested override key", "spawn", `{"spec":{"sandbox_mode":"off"}}`, "sandbox")
	wantDeny(t, "literal in any arg value", "echo", `{"text":"run with danger-full-access please"}`, "sandbox")

	wantNoDeny(t, "exec without sandbox flag", "run_shell", `{"command":"vh-agent-harness exec go build ./..."}`)
	wantNoDeny(t, "sandbox word in prose", "echo", `{"text":"the sandbox is fine"}`)
}

// --- non-run_shell tools never hit the command classes

func TestHardDenyNonShellToolsSkipCommandClasses(t *testing.T) {
	// The git/env/traversal command logic is run_shell-scoped: an echo
	// of the TEXT "git push" is not a git mutation.
	wantNoDeny(t, "git in echo text", "echo", `{"text":"git push"}`)
	wantNoDeny(t, "secret name in echo text", "echo", `{"text":"MY_API_KEY=1"}`)
}
