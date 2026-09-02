package session

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

// TestAppendToolResultMetaRoundTrip covers the slice-3 typed outcome
// metadata on tool/result: the denial marker (denied + reason + identity),
// the orthogonal timeout classification, and replace provenance must all
// survive the append → payload → surface path, while the slice-1
// four-argument AppendToolResult keeps compiling and producing identical
// wire shapes for plain results.
func TestAppendToolResultMetaRoundTrip(t *testing.T) {
	var sb writeBuffer
	lg, err := NewLog(&sb, "sess-meta", time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}

	// Slice-1 path unchanged: no new fields serialized when unset.
	plain, err := lg.AppendToolResult("c1", "echo", `{"text":"x"}`, false)
	if err != nil {
		t.Fatalf("AppendToolResult: %v", err)
	}
	var plainPayload ToolResultPayload
	if err := json.Unmarshal(plain.Payload, &plainPayload); err != nil {
		t.Fatalf("plain payload: %v", err)
	}
	if plainPayload.Denied || plainPayload.TimedOut || plainPayload.DeniedBy != "" || plainPayload.ReplacedBy != "" {
		t.Fatalf("plain result must not carry slice-3 metadata: %+v", plainPayload)
	}
	// omitempty: the marshaled payload must not even name the new fields.
	if string(plain.Payload) != `{"callId":"c1","name":"echo","content":"{\"text\":\"x\"}","isError":false}` {
		t.Fatalf("plain payload wire shape = %s", plain.Payload)
	}

	// Full-fidelity path: denial with identity provenance.
	ev, err := lg.AppendToolResultMeta(ToolResultPayload{
		CallID:     "c2",
		Name:       "write_file",
		Content:    "denied by guard fs-policy: path outside workspace",
		IsError:    true,
		Denied:     true,
		DeniedBy:   "fs-policy",
		DenyReason: "path outside workspace",
	})
	if err != nil {
		t.Fatalf("AppendToolResultMeta: %v", err)
	}
	if ev.Type != TypeToolResult || ev.SurfaceOp == nil || ev.SurfaceOp.Op != SurfaceOpAppend {
		t.Fatalf("meta result event = %+v", ev)
	}
	var full ToolResultPayload
	if err := json.Unmarshal(ev.Payload, &full); err != nil {
		t.Fatalf("meta payload: %v", err)
	}
	if !full.Denied || full.DeniedBy != "fs-policy" || full.DenyReason != "path outside workspace" || !full.IsError {
		t.Fatalf("meta payload = %+v", full)
	}

	// Timeout classification and replace provenance round-trip too.
	ev2, err := lg.AppendToolResultMeta(ToolResultPayload{
		CallID:     "c3",
		Name:       "slow_query",
		Content:    "tool slow_query timed out after 5000ms",
		IsError:    true,
		TimedOut:   true,
		ReplacedBy: "redactor",
	})
	if err != nil {
		t.Fatalf("AppendToolResultMeta (timeout): %v", err)
	}
	var to ToolResultPayload
	if err := json.Unmarshal(ev2.Payload, &to); err != nil {
		t.Fatalf("timeout payload: %v", err)
	}
	if !to.TimedOut || !to.IsError || to.ReplacedBy != "redactor" {
		t.Fatalf("timeout payload = %+v", to)
	}

	msgs, err := lg.Surface()
	if err != nil {
		t.Fatalf("Surface: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("surface = %d messages, want 3", len(msgs))
	}
	if msgs[1].ToolCallID != "c2" || msgs[1].Content != "denied by guard fs-policy: path outside workspace" {
		t.Fatalf("denied surface message = %+v", msgs[1])
	}
	if msgs[2].ToolCallID != "c3" {
		t.Fatalf("timeout surface message = %+v", msgs[2])
	}

	// Replay folds the metadata events identically.
	replayed, err := Replay(bytes.NewReader(sb.data))
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	replayMsgs, err := DeriveMessages(replayed)
	if err != nil {
		t.Fatalf("DeriveMessages: %v", err)
	}
	liveJSON, _ := json.Marshal(msgs)
	replayJSON, _ := json.Marshal(replayMsgs)
	if string(liveJSON) != string(replayJSON) {
		t.Fatalf("surface drift after replay:\nlive:   %s\nreplay: %s", liveJSON, replayJSON)
	}
}
