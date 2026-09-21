package shell

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// TestSandboxedDenialLoggedTurnReplaysByteIdentical is this slice's
// CRUX (acceptance spine, the established determinism pattern): a
// sandboxed run_shell WRITE DENIAL executed through the REAL
// tools.Pipeline (ExecuteLogged: tool/call PRE-execution, tool/result
// post-execution) must, after close + Replay + DeriveMessages,
// reproduce the identical derived transcript byte-for-byte. The denial
// is the honest runtime classification — a NORMAL non-zero exit whose
// stderr carries the kernel EACCES — proving confinement outcomes flow
// through the logging vocabulary with no nondeterministic event shape.
func TestSandboxedDenialLoggedTurnReplaysByteIdentical(t *testing.T) {
	skipWithoutBackend(t)

	fn, err := NewSandboxFunc(SandboxOptions{Mode: SandboxReadOnly})
	if err != nil {
		t.Fatalf("constructor: %v", err)
	}

	path := filepath.Join(t.TempDir(), "session.jsonl")
	lg, err := session.OpenFile(path, "sess-sandbox-1")
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}

	def := Definition(Config{Sandbox: fn, SandboxName: "read-only"})
	p := tools.NewPipeline()
	if err := p.Register(def); err != nil {
		t.Fatalf("register run_shell: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := lg.AppendTurnBegin(); err != nil {
		t.Fatalf("turn/begin: %v", err)
	}
	if _, err := lg.AppendPrompt("probe sandbox confinement"); err != nil {
		t.Fatalf("prompt: %v", err)
	}

	deniedPath := filepath.Join(t.TempDir(), "replay-denied")
	calls := []session.ToolCall{
		{ID: "call_ok", Name: Name, Args: json.RawMessage(`{"command":"printf sandbox-replay-hello"}`)},
		{ID: "call_denied", Name: Name, Args: json.RawMessage(`{"command":"echo nope > ` + deniedPath + `"}`)},
	}
	results := make([]tools.Result, len(calls))
	for i, c := range calls {
		res, err := p.ExecuteLogged(ctx, lg, c)
		if err != nil {
			t.Fatalf("ExecuteLogged %s: %v", c.ID, err)
		}
		results[i] = res
	}

	// Sanity: the clean run succeeded confined; the write was DENIED by
	// the kernel and classified honestly (normal non-zero exit, not a
	// tool error, not a hang).
	if results[0].IsError || !bytes.Contains([]byte(results[0].Content), []byte(`"stdout":"sandbox-replay-hello"`)) {
		t.Fatalf("call_ok result wrong: %+v", results[0])
	}
	if !bytes.Contains([]byte(results[1].Content), []byte(`"sandbox":"read-only"`)) {
		t.Fatalf("call_denied lost the sandbox label: %s", results[1].Content)
	}
	if !bytes.Contains([]byte(results[1].Content), []byte("Permission denied")) {
		t.Fatalf("call_denied content lacks the kernel denial diagnostic: %s", results[1].Content)
	}
	if _, err := os.Stat(deniedPath); !os.IsNotExist(err) {
		t.Fatalf("denied write left a file behind: %s", deniedPath)
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

	// The denial facts are durable and id-bound in the model order.
	wantIDs := []string{"call_ok", "call_denied"}
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
	if !bytes.Contains(replayJSON, []byte("Permission denied")) || !bytes.Contains(replayJSON, []byte("sandbox-replay-hello")) {
		t.Fatalf("replayed surface lost the sandbox outcome facts: %s", replayJSON)
	}
}
