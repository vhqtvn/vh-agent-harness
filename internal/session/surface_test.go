package session

import (
	"encoding/json"
	"testing"
	"time"
)

// newLogOrFail builds an in-memory log; failures abort the test.
func newLogOrFail(t *testing.T) *Log {
	t.Helper()
	var sb writeBuffer
	lg, err := NewLog(&sb, "sess-surface", time.Now())
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	t.Cleanup(func() { _ = lg.Close() })
	return lg
}

type writeBuffer struct{ data []byte }

func (w *writeBuffer) Write(p []byte) (int, error) {
	w.data = append(w.data, p...)
	return len(p), nil
}

func TestDeriveMessagesAppend(t *testing.T) {
	lg := newLogOrFail(t)
	if _, err := lg.AppendPrompt("hello"); err != nil {
		t.Fatalf("AppendPrompt: %v", err)
	}
	if _, err := lg.AppendLLMResponse("mock-1", "thinking", []ToolCall{{
		ID:   "call_1",
		Name: "echo",
		Args: json.RawMessage(`{"text":"hello"}`),
	}}, Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}); err != nil {
		t.Fatalf("AppendLLMResponse: %v", err)
	}
	if _, err := lg.AppendToolResult("call_1", "echo", `{"text":"hello"}`, false); err != nil {
		t.Fatalf("AppendToolResult: %v", err)
	}
	msgs, err := DeriveMessages(lg.Events())
	if err != nil {
		t.Fatalf("DeriveMessages: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "hello" {
		t.Fatalf("msgs[0] = %+v, want user/hello", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "thinking" {
		t.Fatalf("msgs[1] = %+v, want assistant/thinking", msgs[1])
	}
	if len(msgs[1].ToolCalls) != 1 || msgs[1].ToolCalls[0].ID != "call_1" || msgs[1].ToolCalls[0].Name != "echo" {
		t.Fatalf("msgs[1].ToolCalls = %+v, want call_1/echo", msgs[1].ToolCalls)
	}
	if string(msgs[1].ToolCalls[0].Args) != `{"text":"hello"}` {
		t.Fatalf("tool call args = %s, want %s", msgs[1].ToolCalls[0].Args, `{"text":"hello"}`)
	}
	if msgs[2].Role != "tool" || msgs[2].ToolCallID != "call_1" || msgs[2].Name != "echo" {
		t.Fatalf("msgs[2] = %+v, want tool/call_1/echo", msgs[2])
	}
	if msgs[2].Content != `{"text":"hello"}` {
		t.Fatalf("tool content = %q", msgs[2].Content)
	}
}

func TestDeriveMessagesExcludesLogOnlyEvents(t *testing.T) {
	lg := newLogOrFail(t)
	if _, err := lg.AppendTurnBegin(); err != nil {
		t.Fatalf("AppendTurnBegin: %v", err)
	}
	if _, err := lg.AppendPrompt("hi"); err != nil {
		t.Fatalf("AppendPrompt: %v", err)
	}
	if _, err := lg.AppendLLMRequest("m", []string{"echo"}, nil, 0); err != nil {
		t.Fatalf("AppendLLMRequest: %v", err)
	}
	if _, err := lg.AppendToolCall("call_1", "echo", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("AppendToolCall: %v", err)
	}
	if _, err := lg.AppendTurnEnd(""); err != nil {
		t.Fatalf("AppendTurnEnd: %v", err)
	}
	msgs, err := DeriveMessages(lg.Events())
	if err != nil {
		t.Fatalf("DeriveMessages: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Content != "hi" {
		t.Fatalf("log-only events must not surface; got %+v", msgs)
	}
}

func TestDeriveMessagesRequiresSurfaceOp(t *testing.T) {
	events := []Event{
		{Seq: 1, Type: TypeSessionHeader},
		{Seq: 2, Type: TypeLLMResponse, Payload: json.RawMessage(`{"model":"m","content":"x"}`)},
	}
	if _, err := DeriveMessages(events); err == nil {
		t.Fatal("expected error when a message-bearing event carries no surfaceOp")
	}
}

func TestDeriveMessagesReplaceShadowsSpan(t *testing.T) {
	lg := newLogOrFail(t)
	for _, text := range []string{"one", "two", "three"} {
		if _, err := lg.AppendPrompt(text); err != nil {
			t.Fatalf("AppendPrompt: %v", err)
		}
	}
	if _, err := lg.Append(TypeSessionPrompt, &SurfaceOp{Op: SurfaceOpReplace, Start: 0, End: 2}, PromptPayload{Text: "[summary]"}); err != nil {
		t.Fatalf("Append replace: %v", err)
	}
	msgs, err := DeriveMessages(lg.Events())
	if err != nil {
		t.Fatalf("DeriveMessages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages after replace, got %d", len(msgs))
	}
	if msgs[0].Content != "[summary]" || msgs[1].Content != "three" {
		t.Fatalf("replace must shadow [0,2): got %+v", msgs)
	}

	// Tail replace: [1,3) over {one,two,three} -> {one,[summary]}
	lg2 := newLogOrFail(t)
	for _, text := range []string{"one", "two", "three"} {
		if _, err := lg2.AppendPrompt(text); err != nil {
			t.Fatalf("AppendPrompt: %v", err)
		}
	}
	if _, err := lg2.Append(TypeSessionPrompt, &SurfaceOp{Op: SurfaceOpReplace, Start: 1, End: 3}, PromptPayload{Text: "[tail summary]"}); err != nil {
		t.Fatalf("Append tail replace: %v", err)
	}
	msgs2, err := DeriveMessages(lg2.Events())
	if err != nil {
		t.Fatalf("DeriveMessages: %v", err)
	}
	if len(msgs2) != 2 || msgs2[0].Content != "one" || msgs2[1].Content != "[tail summary]" {
		t.Fatalf("tail replace must shadow [1,3): got %+v", msgs2)
	}
}

func TestDeriveMessagesReplaceBounds(t *testing.T) {
	base := []Event{
		{Seq: 1, Type: TypeSessionHeader},
		{Seq: 2, Type: TypeSessionPrompt, SurfaceOp: &SurfaceOp{Op: SurfaceOpAppend}, Payload: json.RawMessage(`{"text":"one"}`)},
	}
	cases := []struct {
		name string
		op   SurfaceOp
	}{
		{"start beyond end", SurfaceOp{Op: SurfaceOpReplace, Start: 1, End: 0}},
		{"end beyond surface", SurfaceOp{Op: SurfaceOpReplace, Start: 0, End: 5}},
	}
	for _, tc := range cases {
		op := tc.op
		events := append(append([]Event{}, base...), Event{
			Seq:       3,
			Type:      TypeSessionPrompt,
			SurfaceOp: &op,
			Payload:   json.RawMessage(`{"text":"s"}`),
		})
		if _, err := DeriveMessages(events); err == nil {
			t.Fatalf("%s: expected bounds error", tc.name)
		}
	}
}

func TestDeriveMessagesFailClosedOnUnknown(t *testing.T) {
	events := []Event{
		{Seq: 1, Type: TypeSessionHeader},
		{Seq: 2, Type: "future/thing"},
	}
	if _, err := DeriveMessages(events); err == nil {
		t.Fatal("expected fail-closed error on unknown event in projection")
	}
}
