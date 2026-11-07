package shell

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// TestRunShellLoggedTurnReplaysByteIdentical is the slice-A2 CRUX
// (acceptance spine): a session log written through the REAL
// tools.Pipeline (ExecuteLogged: tool/call PRE-execution, tool/result
// post-execution) for run_shell invocations — one clean exit, one
// non-zero exit, one timeout, and one guard denial — must, after close
// + Replay + DeriveMessages, reproduce the identical derived
// transcript byte-for-byte. This proves run_shell results flow through
// the existing logging vocabulary with no nondeterministic event
// shape: replay determinism holds by construction for native subprocess
// outcomes.
func TestRunShellLoggedTurnReplaysByteIdentical(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	lg, err := session.OpenFile(path, "sess-runshell-1")
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}

	def := Definition(Config{})
	p := tools.NewPipeline()
	if err := p.Register(def); err != nil {
		t.Fatalf("register run_shell: %v", err)
	}
	p.AddGuard(denyGuard{pattern: "rm -rf"})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := lg.AppendTurnBegin(); err != nil {
		t.Fatalf("turn/begin: %v", err)
	}
	if _, err := lg.AppendPrompt("run some shell commands"); err != nil {
		t.Fatalf("prompt: %v", err)
	}

	calls := []session.ToolCall{
		{ID: "call_1", Name: Name, Args: json.RawMessage(`{"command":"printf replay-hello"}`)},
		{ID: "call_2", Name: Name, Args: json.RawMessage(`{"command":"exit 7"}`)},
		{ID: "call_3", Name: Name, Args: json.RawMessage(`{"command":"sleep 5","timeout_ms":300}`)},
		{ID: "call_4", Name: Name, Args: json.RawMessage(`{"command":"rm -rf /"}`)}, // denied by guard
	}
	results := make([]tools.Result, len(calls))
	for i, c := range calls {
		res, err := p.ExecuteLogged(ctx, lg, c)
		if err != nil {
			t.Fatalf("ExecuteLogged %s: %v", c.ID, err)
		}
		results[i] = res
	}

	// Sanity: the four orthogonal result shapes landed as designed.
	if results[0].IsError || !bytes.Contains([]byte(results[0].Content), []byte(`"stdout":"replay-hello"`)) {
		t.Fatalf("call_1 result wrong: %+v", results[0])
	}
	if results[1].IsError || !bytes.Contains([]byte(results[1].Content), []byte(`"exitCode":7`)) {
		t.Fatalf("call_2 result wrong: %+v", results[1])
	}
	if !results[2].IsError || !bytes.Contains([]byte(results[2].Content), []byte("timed out after 300ms")) {
		t.Fatalf("call_3 result wrong: %+v", results[2])
	}
	if !results[3].Denied || results[3].DeniedBy != "shell-danger-scanner" {
		t.Fatalf("call_4 result wrong: %+v", results[3])
	}

	if _, err := lg.AppendTurnEnd(""); err != nil {
		t.Fatalf("turn/end: %v", err)
	}

	liveMsgs, err := lg.Surface()
	if err != nil {
		t.Fatalf("live Surface: %v", err)
	}
	liveJSON, err := json.Marshal(liveMsgs)
	if err != nil {
		t.Fatalf("marshal live surface: %v", err)
	}
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Replay from durable bytes — twice, independently.
	replayed1, err := session.ReplayFile(path)
	if err != nil {
		t.Fatalf("ReplayFile #1: %v", err)
	}
	replayed2, err := session.ReplayFile(path)
	if err != nil {
		t.Fatalf("ReplayFile #2: %v", err)
	}
	r1, _ := json.Marshal(replayed1)
	r2, _ := json.Marshal(replayed2)
	if !bytes.Equal(r1, r2) {
		t.Fatalf("two replays of the same log differ — nondeterministic replay")
	}

	replayMsgs, err := session.DeriveMessages(replayed1)
	if err != nil {
		t.Fatalf("DeriveMessages(replayed): %v", err)
	}
	replayJSON, err := json.Marshal(replayMsgs)
	if err != nil {
		t.Fatalf("marshal replayed surface: %v", err)
	}
	if !bytes.Equal(liveJSON, replayJSON) {
		t.Fatalf("replay determinism failed:\nlive:   %s\nreplay: %s", liveJSON, replayJSON)
	}

	// The replayed transcript carries the run_shell outcomes as tool
	// messages, id-bound to their calls, in model order.
	wantIDs := []string{"call_1", "call_2", "call_3", "call_4"}
	idx := 0
	for _, m := range replayMsgs {
		if m.Role != "tool" {
			continue
		}
		if idx >= len(wantIDs) {
			t.Fatalf("more tool messages than calls: %+v", replayMsgs)
		}
		if m.ToolCallID != wantIDs[idx] {
			t.Fatalf("tool message %d has id %q, want %q (model order)", idx, m.ToolCallID, wantIDs[idx])
		}
		idx++
	}
	if idx != len(wantIDs) {
		t.Fatalf("replayed %d tool messages, want %d", idx, len(wantIDs))
	}
	if !bytes.Contains(replayJSON, []byte(`replay-hello`)) {
		t.Fatalf("replayed surface lost the captured stdout: %s", replayJSON)
	}
	// The exit-code fact survives replay (decode instead of raw-byte
	// matching — the outer JSON escapes the inner quotes).
	for _, m := range replayMsgs {
		if m.Role == "tool" && m.ToolCallID == "call_2" {
			if !bytes.Contains([]byte(m.Content), []byte(`"exitCode":7`)) {
				t.Fatalf("replayed call_2 lost the exit-code fact: %s", m.Content)
			}
		}
	}
}
