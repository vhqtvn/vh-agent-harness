package session

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestRecoverTailDropsTornLastRecord: a crash can leave a half-written
// final record; RecoverTail must return the intact prefix, report the
// torn tail, and give the byte offset where the torn write began (the
// resume truncation point). A proper prefix of a marshaled event is never
// valid JSON, so "final record without trailing newline" is the torn
// signature.
func TestRecoverTailDropsTornLastRecord(t *testing.T) {
	var sink jobSink
	lg := jobTestLog(t, "sess-torn-1", &sink)
	defer lg.Close()
	if _, err := lg.AppendPrompt("before crash"); err != nil {
		t.Fatalf("AppendPrompt: %v", err)
	}
	clean := append([]byte(nil), sink.Bytes()...)
	tornBytes := append([]byte(nil), `{"seq":2,"type":"job/sett`...) // half-written record
	data := append(append([]byte(nil), clean...), tornBytes...)

	events, validBytes, torn, err := RecoverTail(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("RecoverTail: %v", err)
	}
	if !torn {
		t.Fatalf("torn = false, want true")
	}
	if validBytes != int64(len(clean)) {
		t.Fatalf("validBytes = %d, want %d (offset where the torn write began)", validBytes, len(clean))
	}
	if len(events) != 2 { // header + prompt
		t.Fatalf("recovered events = %d, want 2", len(events))
	}
}

// TestRecoverTailCleanLogNoTorn: a cleanly terminated log recovers with
// torn=false and the same events Replay returns.
func TestRecoverTailCleanLogNoTorn(t *testing.T) {
	var sink jobSink
	lg := jobTestLog(t, "sess-torn-2", &sink)
	defer lg.Close()
	if _, err := lg.AppendPrompt("clean"); err != nil {
		t.Fatalf("AppendPrompt: %v", err)
	}
	want, err := Replay(bytes.NewReader(sink.Bytes()))
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	events, validBytes, torn, err := RecoverTail(bytes.NewReader(sink.Bytes()))
	if err != nil {
		t.Fatalf("RecoverTail: %v", err)
	}
	if torn {
		t.Fatalf("torn = true on a clean log, want false")
	}
	if validBytes != int64(len(sink.Bytes())) {
		t.Fatalf("validBytes = %d, want full length %d", validBytes, len(sink.Bytes()))
	}
	got, _ := json.Marshal(events)
	wantJSON, _ := json.Marshal(want)
	if !bytes.Equal(got, wantJSON) {
		t.Fatalf("RecoverTail events differ from Replay:\n%s\n%s", got, wantJSON)
	}
}

// TestRecoverTailMidFileCorruptionStillFails: torn-tail tolerance applies
// ONLY to the final record; a malformed newline-terminated record in the
// middle is corruption and fails closed like Replay.
func TestRecoverTailMidFileCorruptionStillFails(t *testing.T) {
	var sink jobSink
	lg := jobTestLog(t, "sess-torn-3", &sink)
	defer lg.Close()
	if _, err := lg.AppendPrompt("one"); err != nil {
		t.Fatalf("AppendPrompt: %v", err)
	}
	if _, err := lg.AppendPrompt("two"); err != nil {
		t.Fatalf("AppendPrompt: %v", err)
	}
	data := sink.Bytes()
	// Corrupt the FIRST line's tail (it is newline-terminated, mid-file).
	data[20] = 'x'
	if _, _, _, err := RecoverTail(bytes.NewReader(data)); err == nil {
		t.Fatalf("mid-file corruption accepted by RecoverTail; want error")
	}
}

// TestResumeLogContinuesSeq: resuming a replayed event list appends with
// the next monotonic seq and no new header.
func TestResumeLogContinuesSeq(t *testing.T) {
	var sink jobSink
	lg := jobTestLog(t, "sess-resume-1", &sink)
	if _, err := lg.AppendPrompt("before"); err != nil {
		t.Fatalf("AppendPrompt: %v", err)
	}
	events, err := Replay(bytes.NewReader(sink.Bytes()))
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}

	var sink2 jobSink
	r2, err := ResumeLog(&sink2, events)
	if err != nil {
		t.Fatalf("ResumeLog: %v", err)
	}
	ev, err := r2.AppendPrompt("after")
	if err != nil {
		t.Fatalf("AppendPrompt after resume: %v", err)
	}
	if ev.Seq != 3 {
		t.Fatalf("resumed append seq = %d, want 3 (continuing the folded prefix)", ev.Seq)
	}
	merged := append(append([]byte(nil), sink.Bytes()...), sink2.Bytes()...)
	replayed, err := Replay(bytes.NewReader(merged))
	if err != nil {
		t.Fatalf("Replay of merged log: %v", err)
	}
	if len(replayed) != 3 {
		t.Fatalf("merged log events = %d, want 3", len(replayed))
	}
}

// TestResumeFileTruncatesTornTailEndToEnd: the file-level resume —
// recover the intact prefix, drop the torn bytes from the file, reopen in
// append mode, and keep appending; the final file replays cleanly.
func TestResumeFileTruncatesTornTailEndToEnd(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crashed.jsonl")
	lg, err := OpenFile(path, "sess-resume-2")
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if _, err := lg.Append(TypeJobEnqueued, nil, JobPayload{JobID: "background-1", Kind: "background", Owner: "sess-resume-2"}); err != nil {
		t.Fatalf("Append job/enqueued: %v", err)
	}
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Simulate the crash-torn settle write.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("open for torn write: %v", err)
	}
	if _, err := f.WriteString(`{"seq":3,"type":"job/sett`); err != nil {
		t.Fatalf("write torn tail: %v", err)
	}
	f.Close()

	r2, err := ResumeFile(path)
	if err != nil {
		t.Fatalf("ResumeFile: %v", err)
	}
	ev, err := r2.Append(TypeJobSettled, nil, JobPayload{JobID: "background-1", Kind: "background", Result: JobResultCompleted})
	if err != nil {
		t.Fatalf("Append job/settled after resume: %v", err)
	}
	if ev.Seq != 3 {
		t.Fatalf("resumed settle seq = %d, want 3", ev.Seq)
	}
	if err := r2.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	replayed, err := ReplayFile(path)
	if err != nil {
		t.Fatalf("ReplayFile after resume: %v", err)
	}
	if len(replayed) != 3 {
		t.Fatalf("resumed log events = %d, want 3 (torn bytes gone, settle landed)", len(replayed))
	}
	if replayed[2].Type != TypeJobSettled {
		t.Fatalf("replayed[2].Type = %s, want job/settled", replayed[2].Type)
	}
}
