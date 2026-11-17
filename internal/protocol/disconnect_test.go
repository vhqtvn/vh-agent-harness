// disconnect_test.go — T1B-F1 regression (round 3): a client EOF is a
// DISCONNECT, and a disconnect denies every pending approval immediately
// (host-protocol.md §6 "connection closed while pending ⇒ deny all
// pending", §10 close semantics) — never a hang and never a timeout
// wait. Before the fix, Conn.ReadLine returned io.EOF without closing
// the conn (Done never fired) and Server.Serve returned without
// s.Close(), so Serve parked forever in wg.Wait with approval-timeout 0
// and drained only after the full timeout otherwise.
package protocol

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// --- conn layer: terminal read error fires Done -------------------------------

// TestReadLineTerminalErrorFiresDone pins the transport half of the fix:
// ReadLine's terminal error (EOF here; closed transport and oversized
// lines land on the same path) must Close the conn so Done fires —
// Close is the only Done-closer, and the approval bridge selects on it.
func TestReadLineTerminalErrorFiresDone(t *testing.T) {
	svc, cli := net.Pipe()
	conn := NewConn(svc)
	if err := cli.Close(); err != nil { // the disconnect
		t.Fatalf("peer close: %v", err)
	}
	if _, err := conn.ReadLine(); err != io.EOF {
		t.Fatalf("ReadLine error = %v, want io.EOF", err)
	}
	select {
	case <-conn.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Done did not fire after terminal read error")
	}
	// Idempotence: the explicit Close after the terminal close is a
	// no-op (single Done close, no panic).
	if err := conn.Close(); err != nil && err != io.ErrClosedPipe {
		t.Logf("second Close returned %v ( tolerated)", err)
	}
	select {
	case <-conn.Done():
	default:
		t.Fatal("Done must remain closed after repeated Close")
	}
}

// --- server layer: EOF mid-approval denies and returns promptly ---------------

// disconnectHarness boots the full real-seam stack (FileEngine +
// scripted adapter + guarded tool) against a reference Client, drives
// ONE prompt whose tool call parks in approval, and returns the pieces
// the disconnect tests need. approved fires when approval/request lands
// (the server handler is then parked in Approve).
type disconnectHarness struct {
	client   *Client
	served   chan error
	logPath  string
	approved chan string
}

func newDisconnectHarness(t *testing.T, approvalTimeoutMs int) *disconnectHarness {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "sess-eof.jsonl")

	adapter := &scriptedAdapter{responses: []*adapters.Response{{
		Model:        "test-model",
		ToolCalls:    []session.ToolCall{{ID: "call-1", Name: "guarded", Args: json.RawMessage(`{}`)}},
		FinishReason: "tool_calls",
	}}}
	eng := &FileEngine{
		Dir:      dir,
		Executor: noopExecutor{},
		Ad:       adapter,
		TurnOpts: tools.TurnOptions{Model: "test-model"},
	}
	svc, cli := net.Pipe()
	srv := NewServer(eng, NewConn(svc), ServerOptions{ApprovalTimeoutMs: approvalTimeoutMs})
	h := &disconnectHarness{
		client:   NewClient(cli),
		served:   make(chan error, 1),
		logPath:  logPath,
		approved: make(chan string, 4),
	}
	go func() { h.served <- srv.Serve(nil) }()

	// Composition contract: tools register AFTER NewServer (the
	// approval bridge is injected there; see engine.go).
	if err := eng.Pipeline().Register(tools.ToolDefinition{
		Name: "guarded",
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			return "RAN-DESPITE-DISCONNECT", nil
		},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	eng.Pipeline().AddPreObserver(&askObserver{tool: "guarded"})
	h.client.OnNotification("approval/request", func(params json.RawMessage) {
		var p struct {
			ApprovalID string `json:"approvalId"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			t.Errorf("approval params: %v", err)
			return
		}
		h.approved <- p.ApprovalID
	})

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("call: %v", err)
		}
	}
	must(h.client.Call("initialize", map[string]any{"protocolVersion": 1}, nil))
	must(h.client.Call("session/create", map[string]any{"path": logPath, "sessionId": "sess-eof"}, nil))

	// Park the server: the prompt turn blocks inside Approve until the
	// disconnect under test.
	go func() {
		var turn promptResult
		_ = h.client.Call("session/prompt", map[string]any{"text": "run the guarded thing"}, &turn)
	}()
	select {
	case id := <-h.approved:
		if id != "approval-1" {
			t.Fatalf("approvalId = %s", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no approval/request arrived (turn never parked)")
	}
	return h
}

// logHasDeny replays the durable session log and reports whether the
// guarded call settled as a DENIED tool result carrying the
// disconnect-deny reason. The fanout never fails appends (durability
// outranks observability), so the deny must be on disk even though the
// notification stream was dead when it landed.
func logHasDeny(t *testing.T, logPath string) bool {
	t.Helper()
	events, err := session.ReplayFile(logPath)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	for _, ev := range events {
		if ev.Type != session.TypeToolResult {
			continue
		}
		var p session.ToolResultPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			t.Fatalf("tool/result payload: %v", err)
		}
		if p.CallID == "call-1" {
			if !p.Denied || !p.IsError {
				t.Fatalf("guarded tool/result = %+v, want denied error", p)
			}
			if p.Content == "RAN-DESPITE-DISCONNECT" {
				t.Fatal("denied tool body executed")
			}
			return true
		}
	}
	return false
}

// TestEOFMidApprovalTimeoutZeroReturnsAndDenies is the daemon-hang case:
// with --approval-timeout-ms 0 (documented "wait while connected"
// mode) a pending approval must still deny on disconnect, Serve must
// drain and return promptly (bounded by the test deadline, not a
// hang), and the durable log must show the deny.
func TestEOFMidApprovalTimeoutZeroReturnsAndDenies(t *testing.T) {
	h := newDisconnectHarness(t, 0)

	start := time.Now()
	if err := h.client.Close(); err != nil { // the disconnect (client EOF)
		t.Fatalf("client close: %v", err)
	}
	select {
	case err := <-h.served:
		if err != io.EOF {
			t.Fatalf("Serve error = %v, want io.EOF", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve did not return after client EOF with approval-timeout 0 (daemon-hang regression)")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Serve drained in %s; want prompt deny, not a timeout wait", elapsed)
	}
	if !logHasDeny(t, h.logPath) {
		t.Fatal("session log shows no denied tool/result after disconnect")
	}
}

// TestEOFMidApprovalLongTimeoutDeniesImmediately is the spec-violation
// case (§6/§10): with a LONG nonzero approval timeout the denial must
// fire on DISCONNECT, immediately — not wait the timeout out. A 30s
// timeout with a multi-second ceiling proves the disconnect path fired.
func TestEOFMidApprovalLongTimeoutDeniesImmediately(t *testing.T) {
	const timeoutMs = 30_000
	h := newDisconnectHarness(t, timeoutMs)

	start := time.Now()
	if err := h.client.Close(); err != nil {
		t.Fatalf("client close: %v", err)
	}
	select {
	case err := <-h.served:
		if err != io.EOF {
			t.Fatalf("Serve error = %v, want io.EOF", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve did not return after client EOF: denial waited out the approval timeout instead of firing on disconnect (spec §6/§10)")
	}
	elapsed := time.Since(start)
	if elapsed >= time.Duration(timeoutMs)*time.Millisecond/10 {
		t.Fatalf("deny took %s; want immediate disconnect-deny (<< %dms timeout)", elapsed, timeoutMs)
	}
	if !logHasDeny(t, h.logPath) {
		t.Fatal("session log shows no denied tool/result after disconnect")
	}
}

// TestEOFCleanSessionReturns pins the benign path: EOF with no pending
// approval returns io.EOF promptly (unchanged behavior, now with Done
// fired and the ctx watcher stepped aside).
func TestEOFCleanSessionReturns(t *testing.T) {
	dir := t.TempDir()
	eng := &FileEngine{Dir: dir, Executor: noopExecutor{}}
	svc, cli := net.Pipe()
	srv := NewServer(eng, NewConn(svc), ServerOptions{})
	served := make(chan error, 1)
	go func() { served <- srv.Serve(nil) }()
	client := NewClient(cli)
	if err := client.Call("initialize", map[string]any{"protocolVersion": 1}, nil); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("client close: %v", err)
	}
	select {
	case err := <-served:
		if err != io.EOF {
			t.Fatalf("Serve error = %v, want io.EOF", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return on clean EOF")
	}
}
