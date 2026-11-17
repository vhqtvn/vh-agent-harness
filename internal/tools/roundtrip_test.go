package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
	"github.com/vhqtvn/vh-agent-harness/internal/adapters/openaicompat"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// TestEndToEndToolRoundTripReplay is the slice-1 keystone: one bounded tool
// round-trip driven end-to-end — prompt -> adapter (mock returns
// tool_call) -> guard pass -> execute -> results logged -> second adapter
// call on the updated surface -> replay reproduces the projection
// byte-for-byte. No real network: the "provider" is a scripted httptest
// mock server.
func TestEndToEndToolRoundTripReplay(t *testing.T) {
	ctx := context.Background()

	var mu sync.Mutex
	var bodies [][]byte
	responses := []string{
		`{"id":"chatcmpl-1","model":"mock-1","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"echo","arguments":"{\"text\":\"hello\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`,
		`{"id":"chatcmpl-2","model":"mock-1","choices":[{"index":0,"message":{"role":"assistant","content":"done: hello echoed"},"finish_reason":"stop"}],"usage":{"prompt_tokens":30,"completion_tokens":8,"total_tokens":38}}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("mock server: read body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		bodies = append(bodies, body)
		if len(bodies) > len(responses) {
			t.Errorf("unexpected request #%d", len(bodies))
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(responses[len(bodies)-1]))
	}))
	defer srv.Close()

	ad := openaicompat.New(openaicompat.Config{
		Provider: "mock",
		BaseURL:  srv.URL,
		Model:    "mock-1",
		APIKey:   "test-key",
	})

	path := filepath.Join(t.TempDir(), "roundtrip.jsonl")
	lg, err := session.OpenFile(path, "sess-roundtrip-1")
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer lg.Close()

	pipe := NewPipeline()
	if err := pipe.Register(echoTool()); err != nil {
		t.Fatalf("Register echo: %v", err)
	}
	pipe.AddGuard(denylistGuard{denied: map[string]bool{}}) // guard present, passes echo

	specs := func() []adapters.ToolSpec {
		defs := pipe.Definitions()
		out := make([]adapters.ToolSpec, 0, len(defs))
		for _, d := range defs {
			out = append(out, d.Spec())
		}
		return out
	}

	// Turn 1: user prompt.
	if _, err := lg.AppendTurnBegin(); err != nil {
		t.Fatalf("AppendTurnBegin: %v", err)
	}
	if _, err := lg.AppendPrompt("echo hello"); err != nil {
		t.Fatalf("AppendPrompt: %v", err)
	}

	// First adapter call on the derived surface.
	surface, err := lg.Surface()
	if err != nil {
		t.Fatalf("Surface: %v", err)
	}
	if _, err := lg.AppendLLMRequest("mock-1", []string{"echo"}, nil, 0); err != nil {
		t.Fatalf("AppendLLMRequest: %v", err)
	}
	resp1, err := ad.Call(ctx, &adapters.Request{Model: "mock-1", Messages: surface, Tools: specs()})
	if err != nil {
		t.Fatalf("adapter call 1: %v", err)
	}
	if len(resp1.ToolCalls) != 1 || resp1.ToolCalls[0].Name != "echo" {
		t.Fatalf("adapter call 1 must return one echo tool call: %+v", resp1.ToolCalls)
	}
	if _, err := lg.AppendLLMResponse("mock-1", resp1.Content, resp1.ToolCalls, resp1.Usage); err != nil {
		t.Fatalf("AppendLLMResponse: %v", err)
	}

	// Bounded tool round-trip: tool/call logged pre-execution, guarded,
	// executed, tool/result logged frozen.
	res, err := pipe.ExecuteLogged(ctx, lg, resp1.ToolCalls[0])
	if err != nil {
		t.Fatalf("ExecuteLogged: %v", err)
	}
	if res.IsError || res.Content != `{"text":"hello"}` {
		t.Fatalf("tool result = %+v", res)
	}

	// Second adapter call on the updated surface (assistant + tool result).
	surface2, err := lg.Surface()
	if err != nil {
		t.Fatalf("Surface after tool: %v", err)
	}
	resp2, err := ad.Call(ctx, &adapters.Request{Model: "mock-1", Messages: surface2, Tools: specs()})
	if err != nil {
		t.Fatalf("adapter call 2: %v", err)
	}
	if resp2.Content != "done: hello echoed" {
		t.Fatalf("adapter call 2 content = %q", resp2.Content)
	}
	if _, err := lg.AppendLLMResponse("mock-1", resp2.Content, resp2.ToolCalls, resp2.Usage); err != nil {
		t.Fatalf("AppendLLMResponse: %v", err)
	}
	if _, err := lg.AppendTurnEnd(""); err != nil {
		t.Fatalf("AppendTurnEnd: %v", err)
	}

	liveMsgs, err := lg.Surface()
	if err != nil {
		t.Fatalf("final Surface: %v", err)
	}
	wantRoles := []string{"user", "assistant", "tool", "assistant"}
	if len(liveMsgs) != len(wantRoles) {
		t.Fatalf("live surface = %d messages, want %d", len(liveMsgs), len(wantRoles))
	}
	liveJSON, err := json.Marshal(liveMsgs)
	if err != nil {
		t.Fatalf("marshal live surface: %v", err)
	}
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Replay IS state: reopen, replay, project — byte-for-byte identical.
	replayed, err := session.ReplayFile(path)
	if err != nil {
		t.Fatalf("ReplayFile: %v", err)
	}
	replayMsgs, err := session.DeriveMessages(replayed)
	if err != nil {
		t.Fatalf("DeriveMessages(replayed): %v", err)
	}
	replayJSON, err := json.Marshal(replayMsgs)
	if err != nil {
		t.Fatalf("marshal replayed surface: %v", err)
	}
	if !bytes.Equal(liveJSON, replayJSON) {
		t.Fatalf("end-to-end replay determinism failed:\nlive:   %s\nreplay: %s", liveJSON, replayJSON)
	}
	for i, m := range replayMsgs {
		if m.Role != wantRoles[i] {
			t.Fatalf("replayed msgs[%d].Role = %q, want %q", i, m.Role, wantRoles[i])
		}
	}

	// The second wire request must carry the assistant tool_calls (with
	// string arguments) and the tool result message.
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 {
		t.Fatalf("expected 2 wire requests, got %d", len(bodies))
	}
	var req2 map[string]any
	if err := json.Unmarshal(bodies[1], &req2); err != nil {
		t.Fatalf("wire request 2 not JSON: %v", err)
	}
	msgs := req2["messages"].([]any)
	if len(msgs) != 3 {
		t.Fatalf("wire request 2 messages = %v", msgs)
	}
	assistant := msgs[1].(map[string]any)
	calls := assistant["tool_calls"].([]any)
	fn := calls[0].(map[string]any)["function"].(map[string]any)
	if fn["name"] != "echo" || fn["arguments"] != `{"text":"hello"}` {
		t.Fatalf("wire assistant tool call = %v", fn)
	}
	toolMsg := msgs[2].(map[string]any)
	if toolMsg["role"] != "tool" || toolMsg["tool_call_id"] != "call_1" {
		t.Fatalf("wire tool message = %v", toolMsg)
	}
	if toolMsg["content"] != `{"text":"hello"}` {
		t.Fatalf("wire tool content = %v", toolMsg["content"])
	}
}
