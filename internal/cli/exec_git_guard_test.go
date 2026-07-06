package cli

import (
	"strings"
	"testing"
)

// TestDenyExecGitMutationPayload is the focused unit test for the A1 Go
// backstop (denyExecGitMutationPayload). It pins the F1 fix: a git mutation
// routed past a global flag in the bare `exec` payload is denied, while
// read-only git verbs, non-git mutations, and empty payloads pass through to
// the normal exec path (A1 must NOT over-deny — regular exec is supposed to
// allow arbitrary non-git mutations like mkdir/pytest).
//
// Input is the BARE payload (what cobra hands runExec after stripping wrapper
// flags via SetInterspersed(false)), NOT prefixed with `vh-agent-harness exec`.
func TestDenyExecGitMutationPayload(t *testing.T) {
	denyCases := []struct {
		name string
		args []string
		verb string // must appear in the denial reason
	}{
		{name: "no-pager commit", args: []string{"git", "--no-pager", "commit"}, verb: "commit"},
		{name: "-C /x push", args: []string{"git", "-C", "/x", "push"}, verb: "push"},
		{name: "--git-dir=/x commit", args: []string{"git", "--git-dir=/x", "commit"}, verb: "commit"},
		{name: "--git-dir /x commit (space-separated, vfNext)", args: []string{"git", "--git-dir", "/x", "commit"}, verb: "commit"},
		{name: "bare commit regression", args: []string{"git", "commit"}, verb: "commit"},
		{name: "no-pager push", args: []string{"git", "--no-pager", "push"}, verb: "push"},
		{name: "-c kv commit (config-flag consume-and-continue)", args: []string{"git", "-c", "x=y", "commit"}, verb: "commit"},
		{name: "--config-env commit (config-flag consume-and-continue)", args: []string{"git", "--config-env=ENV", "commit"}, verb: "commit"},
		{name: "--exec-path commit (exec-flag consume-and-continue)", args: []string{"git", "--exec-path=/usr/lib/git", "commit"}, verb: "commit"},
		{name: "unknown flag commit (mutation hidden behind unknown flag)", args: []string{"git", "--fictional", "commit"}, verb: "commit"},
		{name: "checkout verb", args: []string{"git", "--no-pager", "checkout"}, verb: "checkout"},
		{name: "reset verb", args: []string{"git", "-C", "/x", "reset"}, verb: "reset"},
	}
	for _, c := range denyCases {
		c := c
		t.Run("deny/"+c.name, func(t *testing.T) {
			deny, reason := denyExecGitMutationPayload(c.args)
			if !deny {
				t.Fatalf("denyExecGitMutationPayload(%v) = (false,...); want (true,...)", c.args)
			}
			if !strings.Contains(reason, c.verb) {
				t.Errorf("reason %q must name the routed verb %q", reason, c.verb)
			}
			if !strings.Contains(reason, "commit-gate") {
				t.Errorf("reason %q must mention commit-gate", reason)
			}
			if !strings.Contains(reason, "committer agent") {
				t.Errorf("reason %q must mention the committer agent", reason)
			}
		})
	}

	allowCases := []struct {
		name string
		args []string
	}{
		{name: "no-pager status (read-only git + global flag)", args: []string{"git", "--no-pager", "status"}},
		{name: "-C /repo status (read-only git + path-bearing flag)", args: []string{"git", "-C", "/repo", "status"}},
		{name: "no-pager log (read-only git)", args: []string{"git", "--no-pager", "log"}},
		{name: "bare status (read-only git, no globals)", args: []string{"git", "status"}},
		{name: "mkdir non-git mutation (must stay allowed)", args: []string{"mkdir", "tmp/x"}},
		{name: "pytest non-git mutation (must stay allowed)", args: []string{"pytest"}},
		{name: "npm test non-git mutation (must stay allowed)", args: []string{"npm", "test"}},
		{name: "bash -c non-git (nested shell OUT OF SCOPE)", args: []string{"bash", "-c", "echo hi"}},
		{name: "empty payload", args: []string{}},
		{name: "git --help (info terminal, no verb)", args: []string{"git", "--help"}},
		{name: "git --version (info terminal, no verb)", args: []string{"git", "--version"}},
		{name: "git with only globals no verb", args: []string{"git", "--no-pager", "-C", "/x"}},
	}
	for _, c := range allowCases {
		c := c
		t.Run("allow/"+c.name, func(t *testing.T) {
			deny, reason := denyExecGitMutationPayload(c.args)
			if deny {
				t.Fatalf("denyExecGitMutationPayload(%v) = (true,%q); want (false,\"\") — A1 must NOT over-deny", c.args, reason)
			}
			if reason != "" {
				t.Errorf("denyExecGitMutationPayload(%v) reason = %q; want \"\" for allow cases", c.args, reason)
			}
		})
	}
}
