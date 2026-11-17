package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

func echoTool() ToolDefinition {
	return ToolDefinition{
		Name:        "echo",
		Description: "echo the arguments back as canonical JSON",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`),
		Execute: func(_ context.Context, args json.RawMessage) (string, error) {
			return string(args), nil
		},
	}
}

// denylistGuard is a deny-only guard: tools on the list are vetoed; the
// guard cannot modify the call.
type denylistGuard struct{ denied map[string]bool }

func (g denylistGuard) Name() string { return "denylist" }

func (g denylistGuard) Check(c session.ToolCall) error {
	if g.denied[c.Name] {
		return errors.New("tool is on the denylist")
	}
	return nil
}

// writeBuffer is an in-memory session log sink.
type writeBuffer struct{ data []byte }

func (w *writeBuffer) Write(p []byte) (int, error) {
	w.data = append(w.data, p...)
	return len(p), nil
}

// failAfterWriter succeeds for the first n writes, then fails.
type failAfterWriter struct {
	n      int
	fail   error
	writes int
}

func (w *failAfterWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes > w.n {
		return 0, w.fail
	}
	return len(p), nil
}

func TestPipelineRegisterValidates(t *testing.T) {
	p := NewPipeline()
	if err := p.Register(ToolDefinition{Name: "", Execute: func(context.Context, json.RawMessage) (string, error) { return "", nil }}); err == nil {
		t.Fatal("expected empty name to be rejected")
	}
	if err := p.Register(ToolDefinition{Name: "noexec"}); err == nil {
		t.Fatal("expected missing Execute to be rejected")
	}
	if err := p.Register(echoTool()); err != nil {
		t.Fatalf("Register echo: %v", err)
	}
	if err := p.Register(ToolDefinition{Name: "echo", Execute: func(context.Context, json.RawMessage) (string, error) { return "", nil }}); err == nil {
		t.Fatal("expected duplicate registration to be rejected")
	}
	defs := p.Definitions()
	if len(defs) != 1 || defs[0].Name != "echo" {
		t.Fatalf("Definitions() = %+v", defs)
	}
}

func TestPipelineDefinitionsSorted(t *testing.T) {
	p := NewPipeline()
	for _, name := range []string{"zeta", "alpha", "mid"} {
		if err := p.Register(ToolDefinition{Name: name, Execute: func(context.Context, json.RawMessage) (string, error) { return "", nil }}); err != nil {
			t.Fatalf("Register %s: %v", name, err)
		}
	}
	defs := p.Definitions()
	if defs[0].Name != "alpha" || defs[1].Name != "mid" || defs[2].Name != "zeta" {
		t.Fatalf("Definitions() not sorted: %+v", defs)
	}
}

func TestPipelineExecuteEcho(t *testing.T) {
	p := NewPipeline()
	if err := p.Register(echoTool()); err != nil {
		t.Fatalf("Register: %v", err)
	}
	res := p.Execute(context.Background(), session.ToolCall{ID: "c1", Name: "echo", Args: json.RawMessage(`{"text":"hey"}`)})
	if res.IsError {
		t.Fatalf("echo must not error: %+v", res)
	}
	if res.Content != `{"text":"hey"}` || res.CallID != "c1" || res.Name != "echo" {
		t.Fatalf("result = %+v", res)
	}
}

func TestPipelineExecuteUnknownTool(t *testing.T) {
	p := NewPipeline()
	res := p.Execute(context.Background(), session.ToolCall{ID: "c1", Name: "nope"})
	if !res.IsError || !strings.Contains(res.Content, "unknown tool") {
		t.Fatalf("expected unknown-tool isError result, got %+v", res)
	}
}

func TestPipelineGuardDeniesWithoutExecute(t *testing.T) {
	executed := false
	p := NewPipeline()
	if err := p.Register(ToolDefinition{
		Name: "echo",
		Execute: func(context.Context, json.RawMessage) (string, error) {
			executed = true
			return "ran", nil
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	p.AddGuard(denylistGuard{denied: map[string]bool{"echo": true}})
	res := p.Execute(context.Background(), session.ToolCall{ID: "c1", Name: "echo"})
	if executed {
		t.Fatal("denied call must never reach Execute")
	}
	if !res.IsError {
		t.Fatalf("guard veto must produce isError result: %+v", res)
	}
	if !strings.Contains(res.Content, "denylist") {
		t.Fatalf("veto content should name the guard: %+v", res)
	}
}

func TestPipelineNormalizesExecError(t *testing.T) {
	p := NewPipeline()
	if err := p.Register(ToolDefinition{
		Name: "boom",
		Execute: func(context.Context, json.RawMessage) (string, error) {
			return "", errors.New("disk on fire")
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	res := p.Execute(context.Background(), session.ToolCall{ID: "c1", Name: "boom"})
	if !res.IsError || !strings.Contains(res.Content, "disk on fire") {
		t.Fatalf("expected normalized error result, got %+v", res)
	}
}

func TestPipelineNormalizesPanic(t *testing.T) {
	p := NewPipeline()
	if err := p.Register(ToolDefinition{
		Name: "panic",
		Execute: func(context.Context, json.RawMessage) (string, error) {
			panic("worker exploded")
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	res := p.Execute(context.Background(), session.ToolCall{ID: "c1", Name: "panic"}) // must not escape
	if !res.IsError || !strings.Contains(res.Content, "panicked") {
		t.Fatalf("expected normalized panic result, got %+v", res)
	}
}

func TestExecuteLoggedLogsPreExecutionAndResult(t *testing.T) {
	w := &writeBuffer{}
	lg, err := session.NewLog(w, "sess-tools", time.Now())
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	p := NewPipeline()
	if err := p.Register(echoTool()); err != nil {
		t.Fatalf("Register: %v", err)
	}
	p.AddGuard(denylistGuard{denied: map[string]bool{}}) // passes echo

	res, err := p.ExecuteLogged(context.Background(), lg, session.ToolCall{
		ID: "call_9", Name: "echo", Args: json.RawMessage(`{"text":"x"}`),
	})
	if err != nil {
		t.Fatalf("ExecuteLogged: %v", err)
	}
	if res.IsError || res.Content != `{"text":"x"}` {
		t.Fatalf("result = %+v", res)
	}
	events := lg.Events()
	if len(events) != 3 { // header, tool/call, tool/result
		t.Fatalf("expected 3 events (header, tool/call, tool/result), got %d", len(events))
	}
	callEv, resEv := events[1], events[2]
	if callEv.Type != session.TypeToolCall || callEv.Seq != 2 {
		t.Fatalf("pre-execution event = %+v", callEv)
	}
	var tc session.ToolCallPayload
	if err := json.Unmarshal(callEv.Payload, &tc); err != nil {
		t.Fatalf("tool/call payload: %v", err)
	}
	if tc.ID != "call_9" || tc.Name != "echo" || string(tc.Args) != `{"text":"x"}` {
		t.Fatalf("tool/call payload = %+v", tc)
	}
	if resEv.Type != session.TypeToolResult || resEv.Seq != 3 {
		t.Fatalf("result event = %+v", resEv)
	}
	var tr session.ToolResultPayload
	if err := json.Unmarshal(resEv.Payload, &tr); err != nil {
		t.Fatalf("tool/result payload: %v", err)
	}
	if tr.CallID != "call_9" || tr.IsError || tr.Content != `{"text":"x"}` {
		t.Fatalf("tool/result payload = %+v", tr)
	}
	msgs, err := lg.Surface()
	if err != nil {
		t.Fatalf("Surface: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Role != "tool" || msgs[0].ToolCallID != "call_9" {
		t.Fatalf("surface after tool round = %+v", msgs)
	}
}

func TestExecuteLoggedDenialIsLoggedAsError(t *testing.T) {
	w := &writeBuffer{}
	lg, err := session.NewLog(w, "sess-deny", time.Now())
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	p := NewPipeline()
	if err := p.Register(echoTool()); err != nil {
		t.Fatalf("Register: %v", err)
	}
	p.AddGuard(denylistGuard{denied: map[string]bool{"echo": true}})
	res, err := p.ExecuteLogged(context.Background(), lg, session.ToolCall{ID: "c1", Name: "echo"})
	if err != nil {
		t.Fatalf("ExecuteLogged: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected isError result: %+v", res)
	}
	events := lg.Events()
	var tr session.ToolResultPayload
	if err := json.Unmarshal(events[len(events)-1].Payload, &tr); err != nil {
		t.Fatalf("tool/result payload: %v", err)
	}
	if !tr.IsError {
		t.Fatalf("denial must be logged with isError: %+v", tr)
	}
}

func TestExecuteLoggedFailsClosedWhenPreLogFails(t *testing.T) {
	// Header write (write #1) succeeds; the tool/call append (write #2) fails.
	w := &failAfterWriter{n: 1, fail: errors.New("disk full")}
	lg, err := session.NewLog(w, "sess-fc", time.Now())
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	executed := false
	p := NewPipeline()
	if err := p.Register(ToolDefinition{
		Name: "echo",
		Execute: func(context.Context, json.RawMessage) (string, error) {
			executed = true
			return "ran", nil
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	_, err = p.ExecuteLogged(context.Background(), lg, session.ToolCall{ID: "c1", Name: "echo"})
	if err == nil {
		t.Fatal("expected ExecuteLogged to fail when the pre-execution log append fails")
	}
	if executed {
		t.Fatal("fail-closed violated: execution ran despite unloggable intent")
	}
}
