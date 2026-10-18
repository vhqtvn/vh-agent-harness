package session

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// decodeLines independently decodes a JSONL buffer into events, without
// going through Replay (keeps writer tests decoupled from the reader).
func decodeLines(t *testing.T, buf *bytes.Buffer) []Event {
	t.Helper()
	var events []Event
	for i, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		if line == "" {
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("line %d is not valid JSON: %v", i+1, err)
		}
		events = append(events, ev)
	}
	return events
}

func TestNewLogWritesHeaderFirst(t *testing.T) {
	var buf bytes.Buffer
	createdAt := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	lg, err := NewLog(&buf, "sess-1", createdAt)
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	if _, err := lg.AppendPrompt("hello"); err != nil {
		t.Fatalf("AppendPrompt: %v", err)
	}
	events := decodeLines(t, &buf)
	if len(events) != 2 {
		t.Fatalf("expected 2 records, got %d", len(events))
	}
	first := events[0]
	if first.Type != TypeSessionHeader {
		t.Fatalf("first record type = %q, want %q", first.Type, TypeSessionHeader)
	}
	if first.Seq != 1 {
		t.Fatalf("header seq = %d, want 1", first.Seq)
	}
	var hp HeaderPayload
	if err := json.Unmarshal(first.Payload, &hp); err != nil {
		t.Fatalf("header payload: %v", err)
	}
	if hp.SessionID != "sess-1" {
		t.Fatalf("header sessionId = %q, want %q", hp.SessionID, "sess-1")
	}
	if hp.FormatVersion != SESSION_FORMAT_VERSION {
		t.Fatalf("header formatVersion = %d, want %d", hp.FormatVersion, SESSION_FORMAT_VERSION)
	}
	if !hp.CreatedAt.Equal(createdAt) {
		t.Fatalf("header createdAt = %v, want %v", hp.CreatedAt, createdAt)
	}
	second := events[1]
	if second.Type != TypeSessionPrompt {
		t.Fatalf("second record type = %q, want %q", second.Type, TypeSessionPrompt)
	}
	if second.Seq != 2 {
		t.Fatalf("prompt seq = %d, want 2", second.Seq)
	}
	if second.SurfaceOp == nil || second.SurfaceOp.Op != SurfaceOpAppend {
		t.Fatalf("prompt surfaceOp = %+v, want append", second.SurfaceOp)
	}
	var pp PromptPayload
	if err := json.Unmarshal(second.Payload, &pp); err != nil {
		t.Fatalf("prompt payload: %v", err)
	}
	if pp.Text != "hello" {
		t.Fatalf("prompt text = %q, want %q", pp.Text, "hello")
	}
}

func TestAppendSeqIsMonotonic(t *testing.T) {
	var buf bytes.Buffer
	lg, err := NewLog(&buf, "sess-seq", time.Now())
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	evs := []Event{}
	for range 4 {
		ev, err := lg.AppendPrompt("x")
		if err != nil {
			t.Fatalf("AppendPrompt: %v", err)
		}
		evs = append(evs, ev)
	}
	want := int64(2)
	for _, ev := range evs {
		if ev.Seq != want {
			t.Fatalf("event seq = %d, want %d", ev.Seq, want)
		}
		want++
	}
	if got := len(lg.Events()); got != 5 { // header + 4
		t.Fatalf("live event count = %d, want 5", got)
	}
}

func TestAppendValidatesTypeAndSurfaceOp(t *testing.T) {
	var buf bytes.Buffer
	lg, err := NewLog(&buf, "sess-val", time.Now())
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	if _, err := lg.Append("future/thing", nil, nil); err == nil {
		t.Fatal("expected unknown event type to be rejected at append time")
	}
	if _, err := lg.Append(TypeSessionHeader, nil, nil); err == nil {
		t.Fatal("expected session/header to be reserved for NewLog")
	}
	if _, err := lg.Append(TypeLLMResponse, nil, LLMResponsePayload{}); err == nil {
		t.Fatal("expected message-bearing event without surfaceOp to be rejected")
	}
	if _, err := lg.Append(TypeSessionPrompt, &SurfaceOp{Op: "teleport"}, PromptPayload{}); err == nil {
		t.Fatal("expected unknown surfaceOp kind to be rejected")
	}
	if _, err := lg.Append(TypeSessionPrompt, &SurfaceOp{Op: SurfaceOpReplace, Start: 2, End: 1}, PromptPayload{}); err == nil {
		t.Fatal("expected replace with Start > End to be rejected")
	}
}
