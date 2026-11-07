// mcp_ask_e2e_test.go — the P8.2 crux tests: the mcp_ namespace is
// ASK-BY-DEFAULT. The default-posture test is the D1 red-first proof:
// with NO --ask-tools, NO client policy, and an approval nobody
// answers, an mcp tool call must (a) surface approval/request on the
// REAL wire, (b) settle as a typed DENIAL (fail-closed), and (c) never
// reach the server (proven by the mock's execution counter staying at
// zero). The promotion twin proves the same call round-trips once the
// ask is GRANTED over the real approval bridge — the wire-level shape
// of a client --policy exact-name allow.
package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
	"github.com/vhqtvn/vh-agent-harness/internal/mcp"
	"github.com/vhqtvn/vh-agent-harness/internal/protocol"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// mcpAskHarness boots the daemon's REAL composition (buildServer +
// Serve) on a net.Pipe with the reference protocol.Client attached, and
// a configurable approval responder: grant (mirror of a --policy allow
// answering the ask) or record-only (the unanswered-approval
// fail-closed direction). bootMCPServe below is the constructor; the
// pieces the tests assert on are returned directly.

// mcpAskCfg builds the daemon Config for the ask tests: MCP registry
// attached, NO AskTools, and a SHORT approval timeout so the unanswered
// direction denies in milliseconds (the same fail-closed class as the
// 30s production default — the battery drives the real client's
// EOF-deny direction over the real binaries).
func mcpAskCfg(t *testing.T, reg *mcp.Registry, approvalTimeoutMs int) *Config {
	t.Helper()
	cfg := mcpCfg(t, reg)
	cfg.ApprovalTimeoutMs = approvalTimeoutMs
	if len(cfg.AskTools) != 0 {
		t.Fatalf("test premise broken: AskTools = %v, want none", cfg.AskTools)
	}
	return cfg
}

// bootMCPServe runs buildServer + Serve and attaches the reference
// client with the given approval policy: "grant" answers every
// approval/request with allow (the wire shape of a --policy rule
// match), "record" never answers (fail-closed deny on timeout), and
// "grant-exact:<name>" grants only the named tool.
func bootMCPServe(t *testing.T, cfg *Config, mode string) (*protocol.Client, *sessionTracker, chan string, chan error, func()) {
	t.Helper()
	svc, cli := net.Pipe()
	srv, _, tracker, _ := buildServer(cfg, "test-key", svc)
	srvDone := make(chan error, 1)
	go func() { srvDone <- srv.Serve(context.Background()) }()
	client := protocol.NewClient(cli)
	if err := client.Call("initialize", map[string]any{"protocolVersion": 1}, nil); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	approvals := make(chan string, 8)
	client.OnNotification("approval/request", func(params json.RawMessage) {
		var p struct {
			ApprovalID string `json:"approvalId"`
			Call       struct {
				Name string `json:"name"`
			} `json:"call"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			t.Errorf("approval params: %v", err)
			return
		}
		approvals <- p.Call.Name
		switch {
		case mode == "grant":
			go func() {
				if err := client.Call("approval/respond", map[string]any{
					"approvalId": p.ApprovalID, "allow": true,
				}, nil); err != nil {
					t.Errorf("approval/respond: %v", err)
				}
			}()
		case strings.HasPrefix(mode, "grant-exact:"):
			if p.Call.Name == strings.TrimPrefix(mode, "grant-exact:") {
				go func() {
					if err := client.Call("approval/respond", map[string]any{
						"approvalId": p.ApprovalID, "allow": true,
					}, nil); err != nil {
						t.Errorf("approval/respond: %v", err)
					}
				}()
			}
		}
	})
	stop := func() {
		_ = client.Close()
		select {
		case <-srvDone:
		case <-time.After(5 * time.Second):
			t.Errorf("server did not exit after client close")
		}
	}
	t.Cleanup(stop)
	return client, tracker, approvals, srvDone, stop
}

// mockCallTotal reads the mock HTTP server's execution counter.
func mockCallTotal(t *testing.T, baseURL string) int {
	t.Helper()
	resp, err := http.Get(baseURL + "/calls")
	if err != nil {
		t.Fatalf("GET /calls: %v", err)
	}
	defer resp.Body.Close()
	var body struct {
		Total int `json:"total"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode /calls: %v", err)
	}
	return body.Total
}

// waitForApproval drains the approvals channel with a deadline.
func waitForApproval(t *testing.T, ch chan string) string {
	t.Helper()
	select {
	case name := <-ch:
		return name
	case <-time.After(10 * time.Second):
		t.Fatal("no approval/request arrived on the wire within 10s")
		return ""
	}
}

// TestMCPDefaultPostureAsksAndFailsClosed is the P8.2 D1 CRUX: NO
// --ask-tools, NO policy, an approval nobody answers. The mcp tool
// call must ASK (approval/request on the real wire), settle as a typed
// denial attributed to the mcp ask source, and NEVER execute at the
// server (execution counter stays zero). The turn still completes and
// the log replays deterministically.
func TestMCPDefaultPostureAsksAndFailsClosed(t *testing.T) {
	remote := startMockMCPHTTP(t)
	cfgPath, token := mcpTestConfig(t, remote, nil)
	reg := connectRegistry(t, cfgPath, 10000)
	cfg := mcpAskCfg(t, reg, 500) // fast fail-closed timeout
	_, tracker, approvals, _, _ := bootMCPServe(t, cfg, "record")

	es, err := tracker.NewSession("", "sess-mcp-ask-default", io.Discard)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	ad := &scriptedAdapter{next: func(calls int) *adapters.Response {
		switch calls {
		case 0:
			return &adapters.Response{
				Model: "fake-model", FinishReason: "tool_calls",
				ToolCalls: []session.ToolCall{
					{ID: "call_mcpask1", Name: "mcp_remotemock_echo", Args: json.RawMessage(`{"text":"must-not-run"}`)},
				},
			}
		default:
			return &adapters.Response{Model: "fake-model", FinishReason: "stop", Content: "mcp ask default complete"}
		}
	}}
	ctx := context.Background()
	r1, err := tracker.TurnRunner().RunTurn(ctx, es.Log, ad, tracker.TurnOptions(), "call the mcp tool with no approval configured")
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if len(r1.Results) != 1 {
		t.Fatalf("results = %d, want 1", len(r1.Results))
	}
	res := r1.Results[0]

	// (a) The ask surfaced on the REAL wire.
	if got := waitForApproval(t, approvals); got != "mcp_remotemock_echo" {
		t.Fatalf("approval/request carried tool %q, want mcp_remotemock_echo", got)
	}

	// (b) Fail-closed denial: isError + typed denial markers.
	if !res.IsError || !res.Denied {
		t.Fatalf("result = %+v, want isError+denied (fail-closed)", res)
	}
	if !strings.Contains(res.DenyReason, "approval") {
		t.Fatalf("deny reason = %q, want the approval denial class", res.DenyReason)
	}

	// (c) The tool NEVER executed at the server: the counter the mock
	// exposes must be ZERO (initialize/tools/list are not tool calls).
	if n := mockCallTotal(t, remote); n != 0 {
		t.Fatalf("mock server executed %d tool call(s), want 0 (the denied call must never reach the server)", n)
	}

	// The turn completed — the model sees the denial and finishes.
	r2, err := tracker.TurnRunner().RunTurn(ctx, es.Log, ad, tracker.TurnOptions(), "done")
	if err != nil || r2.Response == nil || !strings.Contains(r2.Response.Content, "mcp ask default complete") {
		t.Fatalf("final turn: %v %+v", err, r2.Response)
	}

	// Durable log: the denial landed as a tool/result with the typed
	// markers; replay determinism holds.
	logRaw := readLogOrFatal(t, es.Path)
	var deniedSeen bool
	for _, ev := range es.Log.Events() {
		if ev.Type == session.TypeToolResult {
			var p session.ToolResultPayload
			if err := json.Unmarshal(ev.Payload, &p); err != nil {
				t.Fatalf("tool/result payload: %v", err)
			}
			if p.Denied && p.CallID == "call_mcpask1" && p.DeniedBy != "" {
				deniedSeen = true
			}
		}
	}
	if !deniedSeen {
		t.Fatal("no typed denial (denied+deniedBy) landed on the durable log")
	}
	replayed, err := session.Replay(strings.NewReader(logRaw))
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	replayMsgs, err := session.DeriveMessages(replayed)
	if err != nil {
		t.Fatalf("DeriveMessages: %v", err)
	}
	liveMsgs, err := es.Log.Surface()
	if err != nil {
		t.Fatalf("live surface: %v", err)
	}
	lj, _ := json.Marshal(liveMsgs)
	rj, _ := json.Marshal(replayMsgs)
	if string(lj) != string(rj) {
		t.Fatal("replayed surface differs from the live surface")
	}

	// REDACTION twin still holds: the fake URL token never reached the
	// session log.
	if strings.Contains(logRaw, token) {
		t.Fatal("URL token leaked into the session log")
	}
}

// readLogOrFatal reads the durable session log bytes.
func readLogOrFatal(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	return string(raw)
}

// TestMCPPolicyPromotionExactName is the D4/D5b wire-level promotion
// proof: the client grants EXACTLY ONE tool name (the wire shape of a
// --policy [[allow]] tool = "mcp_localmock_echo" rule). The granted
// call round-trips real content through the REAL pipeline; a sibling
// mcp call NOT covered by the grant is denied fail-closed and never
// executes.
func TestMCPPolicyPromotionExactName(t *testing.T) {
	remote := startMockMCPHTTP(t)
	cfgPath, _ := mcpTestConfig(t, remote, nil)
	reg := connectRegistry(t, cfgPath, 10000)
	cfg := mcpAskCfg(t, reg, 500)
	_, tracker, approvals, _, _ := bootMCPServe(t, cfg, "grant-exact:mcp_localmock_echo")

	es, err := tracker.NewSession("", "sess-mcp-ask-promote", io.Discard)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	ad := &scriptedAdapter{next: func(calls int) *adapters.Response {
		switch calls {
		case 0:
			return &adapters.Response{
				Model: "fake-model", FinishReason: "tool_calls",
				ToolCalls: []session.ToolCall{
					{ID: "call_promote1", Name: "mcp_localmock_echo", Args: json.RawMessage(`{"text":"promoted-by-exact-allow"}`)},
					{ID: "call_promote2", Name: "mcp_remotemock_echo", Args: json.RawMessage(`{"text":"not-covered"}`)},
				},
			}
		default:
			return &adapters.Response{Model: "fake-model", FinishReason: "stop", Content: "promotion twin complete"}
		}
	}}
	r1, err := tracker.TurnRunner().RunTurn(context.Background(), es.Log, ad, tracker.TurnOptions(), "one granted, one denied")
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if len(r1.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(r1.Results))
	}
	// Both asked; only the exact-named one was granted.
	if got := waitForApproval(t, approvals); got != "mcp_localmock_echo" {
		t.Fatalf("first approval carried %q", got)
	}
	if got := waitForApproval(t, approvals); got != "mcp_remotemock_echo" {
		t.Fatalf("second approval carried %q", got)
	}
	granted, denied := r1.Results[0], r1.Results[1]
	if granted.IsError || !strings.Contains(granted.Content, "echo: promoted-by-exact-allow") {
		t.Fatalf("granted call did not round-trip: %+v", granted)
	}
	if !denied.IsError || !denied.Denied {
		t.Fatalf("uncovered call = %+v, want isError+denied", denied)
	}
	if n := mockCallTotal(t, remote); n != 0 {
		t.Fatalf("remote server executed %d call(s), want 0 (only the LOCAL server's tool was granted)", n)
	}
	// The turn completed with the mixed grant/deny surface.
	r2, err := tracker.TurnRunner().RunTurn(context.Background(), es.Log, ad, tracker.TurnOptions(), "done")
	if err != nil || r2.Response == nil || !strings.Contains(r2.Response.Content, "promotion twin complete") {
		t.Fatalf("final turn: %v %+v", err, r2.Response)
	}
}

// TestMCPAutoAllowRoundTrips is the D5c twin: with --mcp-auto-allow
// (cfg.MCPAutoAllow) the mcp ask observer is NOT registered — the
// pre-P8.2 posture, now an explicit operator opt-in. No approval is
// requested; the tool executes directly.
func TestMCPAutoAllowRoundTrips(t *testing.T) {
	remote := startMockMCPHTTP(t)
	cfgPath, _ := mcpTestConfig(t, remote, nil)
	reg := connectRegistry(t, cfgPath, 10000)
	cfg := mcpAskCfg(t, reg, 500)
	cfg.MCPAutoAllow = true
	_, tracker, approvals, _, _ := bootMCPServe(t, cfg, "record")

	es, err := tracker.NewSession("", "sess-mcp-auto-allow", io.Discard)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	ad := &scriptedAdapter{next: func(calls int) *adapters.Response {
		switch calls {
		case 0:
			return &adapters.Response{
				Model: "fake-model", FinishReason: "tool_calls",
				ToolCalls: []session.ToolCall{
					{ID: "call_auto1", Name: "mcp_remotemock_echo", Args: json.RawMessage(`{"text":"auto-allowed"}`)},
				},
			}
		default:
			return &adapters.Response{Model: "fake-model", FinishReason: "stop", Content: "auto-allow twin complete"}
		}
	}}
	r1, err := tracker.TurnRunner().RunTurn(context.Background(), es.Log, ad, tracker.TurnOptions(), "no ask on the opt-in path")
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if len(r1.Results) != 1 {
		t.Fatalf("results = %d, want 1", len(r1.Results))
	}
	res := r1.Results[0]
	if res.IsError || res.Denied || !strings.Contains(res.Content, "echo: auto-allowed") {
		t.Fatalf("auto-allow result = %+v, want a clean round-trip", res)
	}
	select {
	case name := <-approvals:
		t.Fatalf("no approval may fire under --mcp-auto-allow, got one for %q", name)
	case <-time.After(300 * time.Millisecond):
	}
	if n := mockCallTotal(t, remote); n != 1 {
		t.Fatalf("remote server executed %d call(s), want 1", n)
	}
}
