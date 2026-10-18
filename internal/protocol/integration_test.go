// integration_test.go — slice 5 step 7: the end-to-end crux. An external
// reference Client drives ONE full tool turn over the wire against the
// real engine seams; a tool requiring approval is granted THROUGH the
// protocol (approval/request → approval/respond); the durable session
// log replays byte-stably afterwards.
package protocol

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
	"github.com/vhqtvn/vh-agent-harness/internal/jobs"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// scriptedAdapter serves a fixed response per call.
type scriptedAdapter struct {
	calls     int
	responses []*adapters.Response
}

func (a *scriptedAdapter) Name() string { return "scripted" }

func (a *scriptedAdapter) Call(ctx context.Context, req *adapters.Request) (*adapters.Response, error) {
	if a.calls < len(a.responses) {
		r := a.responses[a.calls]
		a.calls++
		return r, nil
	}
	return &adapters.Response{Model: "test-model", Content: "done"}, nil
}

// askObserver asks approval for one tool name.
type askObserver struct{ tool string }

func (o *askObserver) Name() string { return "ask-" + o.tool }
func (o *askObserver) ObservePreExecute(call session.ToolCall) tools.Verdict {
	if call.Name == o.tool {
		return tools.Ask("mutates workspace")
	}
	return tools.Allow()
}

// noopExecutor settles every job completed.
type noopExecutor struct{}

func (noopExecutor) Run(ctx context.Context, job jobs.Job) error { return nil }

func TestEndToEndApprovalTurnReplayStable(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "sess-e2e.jsonl")

	adapter := &scriptedAdapter{responses: []*adapters.Response{
		{
			Model:        "test-model",
			ToolCalls:    []session.ToolCall{{ID: "call-1", Name: "guarded", Args: json.RawMessage(`{"x":1}`)}},
			Usage:        session.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
			FinishReason: "tool_calls",
		},
	}}
	eng := &FileEngine{
		Dir:      dir,
		Executor: noopExecutor{},
		Ad:       adapter,
		TurnOpts: tools.TurnOptions{
			Model: "test-model",
			Tools: []adapters.ToolSpec{{Name: "guarded", Description: "needs approval"}},
		},
	}
	svc, cli := net.Pipe()
	srv := NewServer(eng, NewConn(svc), ServerOptions{})
	served := make(chan error, 1)
	go func() { served <- srv.Serve(nil) }()
	client := NewClient(cli)

	// Pipeline setup happens AFTER NewServer: the approval bridge is
	// injected there (ApprovalAwareEngine), and the pipeline freezes its
	// decision lattice at construction.
	if err := eng.Pipeline().Register(tools.ToolDefinition{
		Name:        "guarded",
		Description: "needs approval",
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			return "approved-ran", nil
		},
	}); err != nil {
		_ = client.Close()
		t.Fatalf("register: %v", err)
	}
	eng.Pipeline().AddPreObserver(&askObserver{tool: "guarded"})

	defer func() {
		_ = client.Close()
		select {
		case <-served:
		case <-time.After(2 * time.Second):
			t.Errorf("server did not exit")
		}
	}()

	// The approval bridge: every approval/request is granted over the
	// wire from a separate goroutine (Call blocks; the notification
	// handler runs on the client read loop).
	grants := make(chan string, 4)
	client.OnNotification("approval/request", func(params json.RawMessage) {
		var p struct {
			ApprovalID string `json:"approvalId"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			t.Errorf("approval params: %v", err)
			return
		}
		go func() {
			if err := client.Call("approval/respond", map[string]any{
				"approvalId": p.ApprovalID, "allow": true,
			}, nil); err != nil {
				t.Errorf("approval/respond: %v", err)
			}
		}()
		grants <- p.ApprovalID
	})

	// Record every session/event (unfiltered).
	rec := newEventRecorder()
	client.OnNotification("session/event", func(params json.RawMessage) {
		ev := &sessionEvent{}
		if err := json.Unmarshal(params, ev); err != nil {
			t.Errorf("event params: %v", err)
			return
		}
		rec.add(ev)
	})

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("call: %v", err)
		}
	}
	must(client.Call("initialize", map[string]any{"protocolVersion": 1}, nil))
	var createRes createResult
	must(client.Call("session/create", map[string]any{"path": logPath, "sessionId": "sess-e2e"}, &createRes))
	must(client.Call("session/subscribe", nil, nil))

	// Drive the turn.
	var turn promptResult
	must(client.Call("session/prompt", map[string]any{"text": "run the guarded thing"}, &turn))

	// The approval round-trip happened over the wire.
	select {
	case id := <-grants:
		if id != "approval-1" {
			t.Fatalf("approvalId = %s", id)
		}
	default:
		t.Fatal("no approval was requested over the wire")
	}

	// The tool RAN (grant → execution), with clean provenance.
	if len(turn.Results) != 1 {
		t.Fatalf("results = %+v", turn.Results)
	}
	r := turn.Results[0]
	if r.CallID != "call-1" || r.Name != "guarded" || r.Content != "approved-ran" || r.IsError || r.Denied {
		t.Fatalf("tool result = %+v", r)
	}
	if len(turn.ToolCalls) != 1 || turn.ToolCalls[0].Name != "guarded" {
		t.Fatalf("toolCalls = %+v", turn.ToolCalls)
	}

	// The full turn choreography streamed as session/event.
	wantOrder := []string{
		"turn/begin", "session/prompt", "llm/request", "llm/response",
		"tool/call", "tool/result", "turn/end",
	}
	got := rec.types()
	for _, want := range wantOrder {
		if e := rec.waitType(t, want); e == nil {
			t.Fatalf("missing %s in stream: %s", want, got)
		}
	}

	// Surface: user prompt, assistant tool-call message, tool result.
	var surf struct {
		Messages []session.Message `json:"messages"`
	}
	must(client.Call("session/surface", nil, &surf))
	if len(surf.Messages) != 3 {
		t.Fatalf("surface = %+v", surf.Messages)
	}
	if surf.Messages[0].Role != "user" || surf.Messages[0].Content != "run the guarded thing" {
		t.Fatalf("m0 = %+v", surf.Messages[0])
	}
	if surf.Messages[1].Role != "assistant" || len(surf.Messages[1].ToolCalls) != 1 {
		t.Fatalf("m1 = %+v", surf.Messages[1])
	}
	if surf.Messages[2].Role != "tool" || surf.Messages[2].ToolCallID != "call-1" || surf.Messages[2].Content != "approved-ran" {
		t.Fatalf("m2 = %+v", surf.Messages[2])
	}

	// Replay byte-stability: the persisted log re-marshals to exactly
	// its own bytes and re-derives the same surface.
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	events, err := session.ReplayFile(logPath)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	var rebuilt []byte
	for _, ev := range events {
		b, merr := json.Marshal(ev)
		if merr != nil {
			t.Fatalf("re-marshal seq %d: %v", ev.Seq, merr)
		}
		rebuilt = append(rebuilt, b...)
		rebuilt = append(rebuilt, '\n')
	}
	if string(raw) != string(rebuilt) {
		t.Fatalf("replay not byte-stable:\nfile:    %s\nrebuilt: %s", raw, rebuilt)
	}
	msgs, err := session.DeriveMessages(events)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if len(msgs) != len(surf.Messages) {
		t.Fatalf("replay surface length = %d, live = %d", len(msgs), len(surf.Messages))
	}
	for i := range msgs {
		if msgs[i].Role != surf.Messages[i].Role || msgs[i].Content != surf.Messages[i].Content {
			t.Fatalf("replay surface[%d] = %+v, live = %+v", i, msgs[i], surf.Messages[i])
		}
	}
}

// TestEndToEndApprovalDenied verifies the same wire path denying: the
// tool must NOT run and the denial marker must survive to the surface.
func TestEndToEndApprovalDenied(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "sess-deny.jsonl")

	adapter := &scriptedAdapter{responses: []*adapters.Response{
		{
			Model:        "test-model",
			ToolCalls:    []session.ToolCall{{ID: "call-1", Name: "guarded", Args: json.RawMessage(`{}`)}},
			FinishReason: "tool_calls",
		},
	}}
	eng := &FileEngine{
		Dir:      dir,
		Executor: noopExecutor{},
		Ad:       adapter,
		TurnOpts: tools.TurnOptions{Model: "test-model"},
	}

	svc, cli := net.Pipe()
	srv := NewServer(eng, NewConn(svc), ServerOptions{})
	go func() { _ = srv.Serve(nil) }()
	client := NewClient(cli)

	// After NewServer (approval bridge injected): register the tool.
	_ = eng.Pipeline().Register(tools.ToolDefinition{
		Name: "guarded",
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			return "SHOULD-NOT-RUN", nil
		},
	})
	eng.Pipeline().AddPreObserver(&askObserver{tool: "guarded"})
	defer client.Close()

	client.OnNotification("approval/request", func(params json.RawMessage) {
		var p struct {
			ApprovalID string `json:"approvalId"`
		}
		_ = json.Unmarshal(params, &p)
		go func() {
			_ = client.Call("approval/respond", map[string]any{
				"approvalId": p.ApprovalID, "allow": false, "reason": "operator refused",
			}, nil)
		}()
	})

	if err := client.Call("initialize", map[string]any{"protocolVersion": 1}, nil); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if err := client.Call("session/create", map[string]any{"path": logPath, "sessionId": "sess-deny"}, nil); err != nil {
		t.Fatalf("create: %v", err)
	}
	var turn promptResult
	if err := client.Call("session/prompt", map[string]any{"text": "try the guarded thing"}, &turn); err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if len(turn.Results) != 1 || !turn.Results[0].IsError || !turn.Results[0].Denied {
		t.Fatalf("results = %+v, want denied error result", turn.Results)
	}
	if turn.Results[0].Content == "SHOULD-NOT-RUN" {
		t.Fatal("denied tool body executed")
	}
}
