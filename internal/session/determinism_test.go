package session

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
)

// TestReplayDeterminismToolRoundTrip is the slice-1 acceptance test (the S1
// spike kill-criterion): a session written through the typed API for a full
// bounded tool round-trip (prompt -> llm/response -> tool/call ->
// tool/result -> llm/response -> turn/end) must, after close + re-open +
// replay, reproduce the identical derived transcript byte-for-byte.
func TestReplayDeterminismToolRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	lg, err := OpenFile(path, "sess-replay-1")
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer lg.Close()

	mustAppend := func(ev Event, err error) Event {
		t.Helper()
		if err != nil {
			t.Fatalf("append %s: %v", ev.Type, err)
		}
		return ev
	}

	mustAppend(lg.AppendTurnBegin())
	mustAppend(lg.AppendPrompt("echo hello"))
	mustAppend(lg.AppendLLMRequest("mock-1", []string{"echo"}, nil, 128))
	mustAppend(lg.AppendLLMResponse("mock-1", "", []ToolCall{{
		ID:   "call_1",
		Name: "echo",
		Args: json.RawMessage(`{"text":"hello"}`),
	}}, Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}))
	mustAppend(lg.AppendToolCall("call_1", "echo", json.RawMessage(`{"text":"hello"}`)))
	mustAppend(lg.AppendToolResult("call_1", "echo", `{"text":"hello"}`, false))
	mustAppend(lg.AppendLLMResponse("mock-1", "done: hello echoed", nil, Usage{PromptTokens: 30, CompletionTokens: 8, TotalTokens: 38}))
	mustAppend(lg.AppendTurnEnd(""))

	live := lg.Events()
	if len(live) != 9 { // header + 8
		t.Fatalf("live event count = %d, want 9", len(live))
	}
	liveMsgs, err := lg.Surface()
	if err != nil {
		t.Fatalf("live Surface: %v", err)
	}
	if len(liveMsgs) != 4 {
		t.Fatalf("live surface length = %d, want 4 (user, assistant, tool, assistant)", len(liveMsgs))
	}
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	replayed, err := ReplayFile(path)
	if err != nil {
		t.Fatalf("ReplayFile: %v", err)
	}
	if len(replayed) != len(live) {
		t.Fatalf("replayed event count = %d, want %d", len(replayed), len(live))
	}
	for i, ev := range replayed {
		if ev.Seq != int64(i+1) {
			t.Fatalf("replayed[%d].Seq = %d, want %d", i, ev.Seq, i+1)
		}
	}

	replayMsgs, err := DeriveMessages(replayed)
	if err != nil {
		t.Fatalf("DeriveMessages(replayed): %v", err)
	}
	liveJSON, err := json.Marshal(liveMsgs)
	if err != nil {
		t.Fatalf("marshal live surface: %v", err)
	}
	replayJSON, err := json.Marshal(replayMsgs)
	if err != nil {
		t.Fatalf("marshal replayed surface: %v", err)
	}
	if !bytes.Equal(liveJSON, replayJSON) {
		t.Fatalf("deterministic replay failed:\nlive:   %s\nreplay: %s", liveJSON, replayJSON)
	}

	wantRoles := []string{"user", "assistant", "tool", "assistant"}
	for i, m := range replayMsgs {
		if m.Role != wantRoles[i] {
			t.Fatalf("replayed msgs[%d].Role = %q, want %q", i, m.Role, wantRoles[i])
		}
	}
	if replayMsgs[1].ToolCalls[0].ID != "call_1" || replayMsgs[2].ToolCallID != "call_1" {
		t.Fatalf("tool round-trip ids not reproduced: %+v", replayMsgs)
	}
}

// TestReplaySurfaceReplace covers the projection determinism of a surface
// replace event (the compaction mechanic, exercised even though compaction
// itself is slice 2): replay must reproduce the post-replace surface
// byte-for-byte.
func TestReplaySurfaceReplace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session-replace.jsonl")
	lg, err := OpenFile(path, "sess-replace-1")
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer lg.Close()

	if _, err := lg.AppendPrompt("one"); err != nil {
		t.Fatalf("AppendPrompt: %v", err)
	}
	if _, err := lg.AppendPrompt("two"); err != nil {
		t.Fatalf("AppendPrompt: %v", err)
	}
	if _, err := lg.AppendLLMResponse("mock-1", "answer one", nil, Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5}); err != nil {
		t.Fatalf("AppendLLMResponse: %v", err)
	}
	if _, err := lg.Append(TypeSessionPrompt, &SurfaceOp{Op: SurfaceOpReplace, Start: 0, End: 2}, PromptPayload{Text: "[summary of one and two]"}); err != nil {
		t.Fatalf("Append replace: %v", err)
	}
	if _, err := lg.AppendLLMResponse("mock-1", "answer two", nil, Usage{PromptTokens: 9, CompletionTokens: 2, TotalTokens: 11}); err != nil {
		t.Fatalf("AppendLLMResponse: %v", err)
	}

	liveMsgs, err := lg.Surface()
	if err != nil {
		t.Fatalf("live Surface: %v", err)
	}
	if len(liveMsgs) != 3 {
		t.Fatalf("live surface after replace = %d messages, want 3", len(liveMsgs))
	}
	if liveMsgs[0].Content != "[summary of one and two]" {
		t.Fatalf("replace did not shadow span: %+v", liveMsgs)
	}
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	replayed, err := ReplayFile(path)
	if err != nil {
		t.Fatalf("ReplayFile: %v", err)
	}
	replayMsgs, err := DeriveMessages(replayed)
	if err != nil {
		t.Fatalf("DeriveMessages(replayed): %v", err)
	}
	liveJSON, _ := json.Marshal(liveMsgs)
	replayJSON, _ := json.Marshal(replayMsgs)
	if !bytes.Equal(liveJSON, replayJSON) {
		t.Fatalf("replace replay determinism failed:\nlive:   %s\nreplay: %s", liveJSON, replayJSON)
	}
}
