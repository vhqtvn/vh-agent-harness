// asktools_test.go — the P3.5 --ask-tools surface: the operator names
// tools whose calls must ride the approval waterfall (ask → wire
// approval/request → the client's interactive/--json/policy approver).
// The daemon previously emitted NO asks (the battery's re-scope note);
// this flag is the daemon-side ask source. Fail-closed posture: an
// unknown tool name is a usage error (exit 2) at startup, and the
// default (no flag) changes nothing.
package main

import (
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/protocol"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// TestSessionTrackerForwardsApprovalBridge: compile-time pin that the
// daemon's engine decorator satisfies ApprovalAwareEngine. Before the
// P3.5 fix the tracker promoted only protocol.Engine's methods (which
// do NOT include SetApprover), so NewServer's injection assertion
// failed SILENTLY and the daemon's pipeline was built without the wire
// bridge — latent until --ask-tools exposed it (every ask denied "no
// approver is configured").
var _ protocol.ApprovalAwareEngine = (*sessionTracker)(nil)

// TestParseAskTools: empty flag = nil (no observer, unchanged
// behavior); comma lists split, trim, and dedupe order-preserving;
// malformed entries (empty name, whitespace-only entry) are usage
// errors, never silently dropped.
func TestParseAskTools(t *testing.T) {
	if got, err := parseAskTools(""); err != nil || got != nil {
		t.Fatalf("empty flag = (%v, %v), want (nil, nil)", got, err)
	}
	got, err := parseAskTools("run_shell")
	if err != nil || len(got) != 1 || got[0] != "run_shell" {
		t.Fatalf("single = (%v, %v)", got, err)
	}
	got, err = parseAskTools(" run_shell , write ,read ")
	if err != nil || len(got) != 3 || got[0] != "run_shell" || got[1] != "write" || got[2] != "read" {
		t.Fatalf("split/trim = (%v, %v)", got, err)
	}
	// Dedupe keeps first occurrence, order-preserving.
	got, err = parseAskTools("write,run_shell,write")
	if err != nil || len(got) != 2 || got[0] != "write" || got[1] != "run_shell" {
		t.Fatalf("dedupe = (%v, %v)", got, err)
	}
	for _, bad := range []string{",", "run_shell,,write", " , "} {
		if got, err := parseAskTools(bad); err == nil {
			t.Fatalf("malformed %q silently accepted: %v", bad, got)
		}
	}
}

// TestValidateAskToolsAgainstRegisteredSet: every name must exist in
// the REGISTERED tool set (the daemon's own tool catalog); an unknown
// name is an error naming it (fail-closed — a typo must not silently
// route nothing).
func TestValidateAskToolsAgainstRegisteredSet(t *testing.T) {
	cfg := testConfig(t, "openai", "http://127.0.0.1:1")
	defs := daemonTools(realNow, cfg, nil)
	if err := validateAskTools([]string{"run_shell", "write", "echo"}, defs); err != nil {
		t.Fatalf("known names rejected: %v", err)
	}
	err := validateAskTools([]string{"run_shell", "bogus_tool", "write"}, defs)
	if err == nil || !strings.Contains(err.Error(), "bogus_tool") {
		t.Fatalf("unknown-name error = %v, want one naming bogus_tool", err)
	}
}

// TestAskToolsObserverVerdicts: matching tool calls return VerdictAsk
// (they ride the REAL waterfall — the ProtocolApprover bridge emits
// approval/request; guards still run after resolution; absence,
// timeout, and disconnect deny fail-closed, covered at the bridge seam
// in internal/protocol). Non-matching tools return the lattice's
// no-objection Allow verdict (the observer is currently the daemon's
// only pre-observer; it resolves nothing upstream because nothing is
// upstream — documented in asktools.go).
func TestAskToolsObserverVerdicts(t *testing.T) {
	o := newAskToolsObserver([]string{"run_shell", "write"})
	if v := o.ObservePreExecute(session.ToolCall{Name: "run_shell"}); v.Kind != tools.VerdictAsk {
		t.Fatalf("run_shell verdict = %+v, want ask", v)
	}
	if v := o.ObservePreExecute(session.ToolCall{Name: "write"}); v.Kind != tools.VerdictAsk {
		t.Fatalf("write verdict = %+v, want ask", v)
	}
	if v := o.ObservePreExecute(session.ToolCall{Name: "echo"}); v.Kind != tools.VerdictAllow {
		t.Fatalf("echo verdict = %+v, want allow (not routed)", v)
	}
	if o.Name() != "ask-tools" {
		t.Fatalf("observer name = %q", o.Name())
	}
}

// TestRunAskToolsUnknownNameExitsTwo: --ask-tools with a name outside
// the registered tool set is a usage error (exit 2) BEFORE serving —
// the fail-closed startup validation.
func TestRunAskToolsUnknownNameExitsTwo(t *testing.T) {
	var out, errb safeBuffer
	code := run([]string{
		"--adapter", "openai", "--model", "m", "--base-url", "http://127.0.0.1:1",
		"--api-key-env", "VH_AGENTD_TEST_KEY", "--session-dir", t.TempDir(),
		"--ask-tools", "no-such-tool",
	}, func(string) string { return "k" }, eofConn{}, &out, &errb)
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (stderr: %s)", code, errb.String())
	}
	if !strings.Contains(errb.String(), "no-such-tool") {
		t.Fatalf("the error must name the unknown tool:\n%s", errb.String())
	}
	if out.String() != "" {
		t.Fatalf("stdout = %q, want empty (stdout is protocol)", out.String())
	}
}

// TestRunAskToolsStartupLogLine: a valid --ask-tools value arms the
// observer and logs ONE startup line naming the routed tools; the
// default (no flag) logs none. Driven over the real run() with an
// immediately-EOF conn (clean exit 0).
func TestRunAskToolsStartupLogLine(t *testing.T) {
	for _, tc := range []struct {
		name      string
		flag      []string
		wantLine  string
		wantNoLog bool
	}{
		{"armed", []string{"--ask-tools", "run_shell,write"}, "ask-tools: run_shell,write", false},
		{"default", nil, "ask-tools:", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errb safeBuffer
			args := []string{
				"--adapter", "openai", "--model", "m", "--base-url", "http://127.0.0.1:1",
				"--api-key-env", "VH_AGENTD_TEST_KEY", "--session-dir", t.TempDir(),
			}
			args = append(args, tc.flag...)
			code := run(args, func(string) string { return "k" }, eofConn{}, &out, &errb)
			if code != 0 {
				t.Fatalf("exit = %d, want 0 (stderr: %s)", code, errb.String())
			}
			if tc.wantNoLog {
				if strings.Contains(errb.String(), "ask-tools:") {
					t.Fatalf("default run must not log an ask-tools line:\n%s", errb.String())
				}
				return
			}
			if !strings.Contains(errb.String(), tc.wantLine) {
				t.Fatalf("startup log lacks %q:\n%s", tc.wantLine, errb.String())
			}
		})
	}
}
