package session

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testHeaderLine = `{"seq":1,"type":"session/header","payload":{"sessionId":"s1","formatVersion":0,"createdAt":"2026-08-20T00:00:00Z"}}`

func TestReplayFailClosedUnknownEvent(t *testing.T) {
	logText := testHeaderLine + "\n" + `{"seq":2,"type":"future/feature","payload":{}}` + "\n"
	_, err := Replay(strings.NewReader(logText))
	if err == nil {
		t.Fatal("expected fail-closed refusal on unknown event type")
	}
	var ue *UnknownEventError
	if !errors.As(err, &ue) {
		t.Fatalf("expected UnknownEventError, got %T: %v", err, err)
	}
	if ue.Type != "future/feature" {
		t.Fatalf("UnknownEventError.Type = %q, want %q", ue.Type, "future/feature")
	}
	if ue.Seq != 2 {
		t.Fatalf("UnknownEventError.Seq = %d, want 2", ue.Seq)
	}
}

func TestReplayIgnoresMarkedUnknownEvents(t *testing.T) {
	logText := testHeaderLine + "\n" +
		`{"seq":2,"type":"future/feature","ignorable":true,"payload":{}}` + "\n" +
		`{"seq":3,"type":"session/prompt","surfaceOp":{"op":"append"},"payload":{"text":"hi"}}` + "\n"
	events, err := Replay(strings.NewReader(logText))
	if err != nil {
		t.Fatalf("expected ignorable unknown event to replay cleanly: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	msgs, err := DeriveMessages(events)
	if err != nil {
		t.Fatalf("DeriveMessages: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Role != "user" || msgs[0].Content != "hi" {
		t.Fatalf("ignorable event must contribute nothing to the surface; got %+v", msgs)
	}
}

func TestReplayHeaderMustBeFirst(t *testing.T) {
	logText := `{"seq":1,"type":"session/prompt","surfaceOp":{"op":"append"},"payload":{"text":"hi"}}` + "\n"
	if _, err := Replay(strings.NewReader(logText)); err == nil {
		t.Fatal("expected error when the first record is not session/header")
	}
}

func TestReplayHeaderOnlyOnce(t *testing.T) {
	logText := testHeaderLine + "\n" + testHeaderLine + "\n"
	if _, err := Replay(strings.NewReader(logText)); err == nil {
		t.Fatal("expected error on a second session/header record")
	}
}

func TestReplayRejectsVersionMismatch(t *testing.T) {
	bad := `{"seq":1,"type":"session/header","payload":{"sessionId":"s1","formatVersion":99,"createdAt":"2026-08-20T00:00:00Z"}}` + "\n"
	if _, err := Replay(strings.NewReader(bad)); err == nil {
		t.Fatal("expected error on unsupported format version")
	}
}

func TestReplayRejectsSeqGap(t *testing.T) {
	logText := testHeaderLine + "\n" +
		`{"seq":3,"type":"session/prompt","surfaceOp":{"op":"append"},"payload":{"text":"hi"}}` + "\n"
	if _, err := Replay(strings.NewReader(logText)); err == nil {
		t.Fatal("expected error on non-contiguous seq")
	}
}

func TestReplayRejectsMalformedRecord(t *testing.T) {
	logText := testHeaderLine + "\n" + `{"seq":2,"type":` + "\n"
	if _, err := Replay(strings.NewReader(logText)); err == nil {
		t.Fatal("expected error on malformed JSON record")
	}
}

func TestReplayFileRoundTripsWrittenLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	lg, err := NewLog(f, "sess-file", time.Now())
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	if _, err := lg.AppendPrompt("round trip"); err != nil {
		t.Fatalf("AppendPrompt: %v", err)
	}
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	events, err := ReplayFile(path)
	if err != nil {
		t.Fatalf("ReplayFile: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Type != TypeSessionHeader || events[1].Type != TypeSessionPrompt {
		t.Fatalf("unexpected replayed types: %q, %q", events[0].Type, events[1].Type)
	}
}
