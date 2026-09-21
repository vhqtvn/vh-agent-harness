// policy_test.go — the P3 wiring at the ApproverFunc seam: the
// policyApprover composition (allow / HARD-DENY / delegate-to-next),
// the --policy flag surface, and the startup posture (bad file = exit 2
// BEFORE the daemon spawns).
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/cmd/vh-agent-client/policy"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// recordingNext is a sentinel delegate: it records calls and answers
// deny (the composition under test is whether it is CALLED, not what
// it would answer).
type recordingNext struct {
	calls int
}

func (r *recordingNext) approver(approvalID string, call session.ToolCall, reason string) ApprovalAnswer {
	r.calls++
	return ApprovalAnswer{Allow: false, Reason: "next answered"}
}

func testPolicy(t *testing.T, src string) *policy.Policy {
	t.Helper()
	p, err := policy.Parse("test.policy", []byte(src))
	if err != nil {
		t.Fatalf("policy.Parse: %v", err)
	}
	return p
}

func TestPolicyApproverAllow(t *testing.T) {
	pol := testPolicy(t, "[[allow]]\ntool = \"echo\"\n")
	next := &recordingNext{}
	var errbuf bytes.Buffer
	approve := policyApprover(pol, next.approver, &errbuf)

	ans := approve("a1", session.ToolCall{ID: "c1", Name: "echo", Args: json.RawMessage(`{"text":"hi"}`)}, "needs human")
	if !ans.Allow {
		t.Fatalf("matching rule must allow, got %+v (stderr:\n%s)", ans, errbuf.String())
	}
	if next.calls != 0 {
		t.Fatalf("an allowed call must NOT reach the delegate (stdin untouched), got %d calls", next.calls)
	}
	if !strings.Contains(errbuf.String(), "policy: allow echo(") {
		t.Fatalf("allow must render one policy line naming the tool:\n%s", errbuf.String())
	}
	if !strings.Contains(errbuf.String(), "text=hi") {
		t.Fatalf("allow line should carry the arg hint:\n%s", errbuf.String())
	}
}

func TestPolicyApproverHardDeny(t *testing.T) {
	// The rule TRIES to allow run_shell broadly; git push must still
	// hard-deny through the composition (order proven at the seam).
	pol := testPolicy(t, "[[allow]]\ntool = \"run_shell\"\n")
	next := &recordingNext{}
	var errbuf bytes.Buffer
	approve := policyApprover(pol, next.approver, &errbuf)

	ans := approve("a1", session.ToolCall{ID: "c1", Name: "run_shell", Args: json.RawMessage(`{"command":"git push"}`)}, "r")
	if ans.Allow {
		t.Fatalf("git push must hard-deny even with a broad allow rule")
	}
	if next.calls != 0 {
		t.Fatalf("a hard-denied call must NOT reach the delegate, got %d calls", next.calls)
	}
	if !strings.Contains(ans.Reason, "hard-deny") {
		t.Fatalf("deny reason must be honest, got %q", ans.Reason)
	}
	if !strings.Contains(errbuf.String(), "policy: HARD-DENY run_shell(") || !strings.Contains(errbuf.String(), "git push") {
		t.Fatalf("hard-deny must render one policy line:\n%s", errbuf.String())
	}
}

func TestPolicyApproverAskDelegates(t *testing.T) {
	pol := testPolicy(t, "[[allow]]\ntool = \"clock\"\n")
	next := &recordingNext{}
	var errbuf bytes.Buffer
	approve := policyApprover(pol, next.approver, &errbuf)

	ans := approve("a1", session.ToolCall{ID: "c1", Name: "echo", Args: json.RawMessage(`{"text":"x"}`)}, "needs human")
	if next.calls != 1 {
		t.Fatalf("an unmatched call must delegate to the next approver exactly once, got %d", next.calls)
	}
	if ans.Allow {
		t.Fatalf("the delegate's answer is the composed answer (deny sentinel)")
	}
	if !strings.Contains(errbuf.String(), "policy: ask → human") {
		t.Fatalf("ask must render one policy line:\n%s", errbuf.String())
	}
}

// --- flag surface -----------------------------------------------------------

func TestParseArgsPolicyFlag(t *testing.T) {
	cfg, err := parseArgs([]string{"--policy", "/tmp/p.policy", "--prompt", "hi"}, func() bool { return false }, os.Stderr)
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if cfg.PolicyPath != "/tmp/p.policy" {
		t.Fatalf("PolicyPath = %q", cfg.PolicyPath)
	}

	cfg, err = parseArgs([]string{"--prompt", "hi"}, func() bool { return false }, os.Stderr)
	if err != nil {
		t.Fatalf("parseArgs without --policy: %v", err)
	}
	if cfg.PolicyPath != "" {
		t.Fatalf("absent --policy must keep the current behavior (PolicyPath empty), got %q", cfg.PolicyPath)
	}
}

func TestParseArgsEmptyPolicyPathIsUsageError(t *testing.T) {
	_, err := parseArgs([]string{"--policy", "", "--prompt", "hi"}, func() bool { return false }, os.Stderr)
	if err == nil || !isUsageError(err) {
		t.Fatalf("--policy \"\" must be a usage error (exit 2), got %v", err)
	}
}

// --- startup posture --------------------------------------------------------

func TestRunBadPolicyFileExitsTwoBeforeSpawn(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "broken.policy")
	if err := os.WriteFile(bad, []byte("[[allow]]\ntool = \"read\"\nmode = \"auto\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errbuf bytes.Buffer
	// --exec points at a binary that does not exist: if the policy
	// were loaded after the spawn, the spawn failure (exit 1) would
	// surface instead of the parse error.
	code := run([]string{
		"--policy", bad,
		"--prompt", "hi",
		"--exec", filepath.Join(dir, "no-such-daemon"),
	}, strings.NewReader(""), &out, &errbuf)
	if code != 2 {
		t.Fatalf("bad policy file must exit 2, got %d (stderr:\n%s)", code, errbuf.String())
	}
	if !strings.Contains(errbuf.String(), "broken.policy:3") {
		t.Fatalf("the error must name the offending file:line:\n%s", errbuf.String())
	}
}

func TestRunUnreadablePolicyFileExitsTwo(t *testing.T) {
	dir := t.TempDir()
	var out, errbuf bytes.Buffer
	code := run([]string{
		"--policy", dir, // a directory is unreadable as a file
		"--prompt", "hi",
		"--exec", filepath.Join(dir, "no-such-daemon"),
	}, strings.NewReader(""), &out, &errbuf)
	if code != 2 {
		t.Fatalf("unreadable policy file must exit 2, got %d (stderr:\n%s)", code, errbuf.String())
	}
	if !strings.Contains(errbuf.String(), "cannot read policy file") {
		t.Fatalf("the error must name the read failure:\n%s", errbuf.String())
	}
}
