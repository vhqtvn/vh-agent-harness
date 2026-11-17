// e2e_askflow_test.go — the P3.5 --ask-tools approval flow at the
// REAL-binary seam: the daemon (spawned with --ask-tools run_shell)
// emits approval/request over the wire, the client's --policy engine
// answers, and the tool executes (grant) or settles denied
// (hard-deny class under the same broad allow rule — the floor proven
// over the real binaries). Before P3.5 the shipped daemon emitted no
// asks, so this flow was only coverable at the library seam
// (policy_seam_test.go); the timeout/disconnect deny directions stay
// covered at the bridge seam (internal/protocol/disconnect_test.go)
// — this file does not duplicate them: the observer feeds the SAME
// ProtocolApprover those tests drive.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestAskToolsApprovalFlowGrantOverRealBinaries: the model calls an
// ask-routed run_shell with a benign command; the client's policy
// (broad run_shell allow) auto-approves; the tool EXECUTES and the
// result reaches the model (final text).
func TestAskToolsApprovalFlowGrantOverRealBinaries(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and spawns real binaries")
	}
	clientBin, daemonBin := buildBinaries(t)

	dir := t.TempDir()
	pol := filepath.Join(dir, "broad.policy")
	if err := os.WriteFile(pol, []byte("[[allow]]\ntool = \"run_shell\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	calls := 0
	llm := startHTTPStub(t, func(w *jsonEncoder) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n == 1 {
			w.toolCall("call-ask-1", "run_shell", `{"command":"echo ask-flow-marker"}`)
			return
		}
		w.content("ask flow grant complete")
	})
	defer llm.Close()

	code, out, errbuf := runClient(t, clientBin, []string{
		"--policy", pol,
		"--session-dir", filepath.Join(dir, "s"),
		"--json",
		"--prompt", "run the tool",
		"--exec", daemonBin,
		"--adapter", "openai", "--model", "fake-model",
		"--base-url", llm.URL,
		"--api-key-env", "VH_AGENTD_TEST_KEY",
		"--ask-tools", "run_shell",
	}, "")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stdout:\n%s\nstderr:\n%s)", code, out, errbuf)
	}
	// The daemon asked over the wire: the NDJSON stream carries the
	// approval/request notification (the json renderer emits params
	// verbatim — no method name — so the approvalId + the ask-routed
	// reason are the observable marks) ...
	if !strings.Contains(out, "approvalId") || !strings.Contains(out, "ask-routed by --ask-tools") {
		t.Fatalf("no approval/request on the --json wire stream:\n%s", out)
	}
	// ... the policy engine auto-allowed (decision line) ...
	if !strings.Contains(errbuf, "policy: allow run_shell(command=echo ask-flow-marker)") {
		t.Fatalf("no policy allow decision line:\n%s", errbuf)
	}
	// ... the tool EXECUTED (its result reached the model → final text).
	if !strings.Contains(out, "ask-flow-marker") {
		t.Fatalf("the ask-routed tool result never reached the stream:\n%s", out)
	}
	if !strings.Contains(out, "ask flow grant complete") {
		t.Fatalf("final text missing:\n%s", out)
	}
	// The daemon's startup log names the routed tools.
	if !strings.Contains(errbuf, "ask-tools: run_shell") {
		t.Fatalf("daemon startup log lacks the ask-tools line:\n%s", errbuf)
	}
}

// TestAskToolsApprovalFlowHardDenyUnderBroadAllow: under the SAME
// broad run_shell allow rule, a git-push call asks, the policy engine
// HARD-DENIES (git mutation class), and the call settles denied — the
// executor never ran (no shell error output, a denied tool/result).
func TestAskToolsApprovalFlowHardDenyUnderBroadAllow(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and spawns real binaries")
	}
	clientBin, daemonBin := buildBinaries(t)

	dir := t.TempDir()
	pol := filepath.Join(dir, "broad.policy")
	if err := os.WriteFile(pol, []byte("[[allow]]\ntool = \"run_shell\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	calls := 0
	llm := startHTTPStub(t, func(w *jsonEncoder) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n == 1 {
			w.toolCall("call-ask-2", "run_shell", `{"command":"git push origin main"}`)
			return
		}
		w.content("deny flow complete")
	})
	defer llm.Close()

	code, out, errbuf := runClient(t, clientBin, []string{
		"--policy", pol,
		"--session-dir", filepath.Join(dir, "s"),
		"--json",
		"--prompt", "run the tool",
		"--exec", daemonBin,
		"--adapter", "openai", "--model", "fake-model",
		"--base-url", llm.URL,
		"--api-key-env", "VH_AGENTD_TEST_KEY",
		"--ask-tools", "run_shell",
	}, "")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (the turn completes honestly; stdout:\n%s\nstderr:\n%s)", code, out, errbuf)
	}
	if !strings.Contains(out, "approvalId") || !strings.Contains(out, "ask-routed by --ask-tools") {
		t.Fatalf("no approval/request on the wire:\n%s", out)
	}
	if !strings.Contains(errbuf, "policy: HARD-DENY run_shell(command=git push origin main)") {
		t.Fatalf("no policy HARD-DENY decision line:\n%s", errbuf)
	}
	// The denied result is visible on the stream and carries the
	// denial, not a shell execution outcome.
	if !strings.Contains(out, "denied by ask-tools") {
		t.Fatalf("no denied result on the stream:\n%s", out)
	}
	if strings.Contains(out, "fatal:") || strings.Contains(out, "git:") {
		t.Fatalf("the executor appears to have RUN git push:\n%s", out)
	}
	if !strings.Contains(out, "deny flow complete") {
		t.Fatalf("final text missing:\n%s", out)
	}

	// The durable session log settles the call as a typed denial
	// (ExecuteLogged commits denied results too).
	sess := filepath.Join(dir, "s")
	entries, err := os.ReadDir(sess)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".jsonl") || !strings.HasPrefix(e.Name(), "sess-") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(sess, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(raw), "\n") {
			var ev struct {
				Type    string          `json:"type"`
				Payload json.RawMessage `json:"payload"`
			}
			if json.Unmarshal([]byte(line), &ev) != nil || ev.Type != "tool/result" {
				continue
			}
			var p struct {
				CallID     string `json:"callId"`
				Denied     bool   `json:"denied"`
				DeniedBy   string `json:"deniedBy"`
				DenyReason string `json:"denyReason"`
			}
			if json.Unmarshal(ev.Payload, &p) != nil {
				continue
			}
			if p.CallID == "call-ask-2" {
				found = true
				if !p.Denied || p.DeniedBy != "ask-tools" {
					t.Fatalf("tool/result for the denied call = %+v, want denied by ask-tools", p)
				}
				if !strings.Contains(p.DenyReason, "approval denied") {
					t.Fatalf("deny reason = %q, want the approval-denied provenance", p.DenyReason)
				}
			}
		}
	}
	if !found {
		t.Fatalf("no tool/result for call-ask-2 in the session log under %s", sess)
	}
}
