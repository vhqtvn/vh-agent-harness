// resume_tee_test.go — P4: ResumeFileTee, the engine's resume seam.
// ResumeFile (crash recovery) reopens the log for append through the
// FILE only; the engine's session/resume must ALSO tee every appended
// record to the wire fan-out (the same MultiWriter shape NewLog gets
// at create). ResumeFileTee(path, sink) is that seam, and ResumeFile
// remains ResumeFileTee(path, nil) — byte-stable behavior for the
// existing crash-recovery caller.
package session

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestResumeFileTeeAppendsReachFileAndSink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sess-tee.jsonl")

	lg1, err := OpenFile(path, "sess-tee")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lg1.AppendPrompt("first prompt"); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := lg1.Close(); err != nil {
		t.Fatal(err)
	}

	var sink bytes.Buffer
	lg2, err := ResumeFileTee(path, &sink)
	if err != nil {
		t.Fatal(err)
	}
	defer lg2.Close()

	// The resumed log preserves every prior event and continues the seq.
	evs := lg2.Events()
	if len(evs) != 2 { // header + prompt
		t.Fatalf("resumed log has %d events, want 2", len(evs))
	}
	if _, err := lg2.AppendPrompt("after restart"); err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(after, before) {
		t.Fatalf("resume must never rewrite the prior stream — file prefix changed")
	}
	if got := sink.String(); !bytes.Contains([]byte(got), []byte("after restart")) {
		t.Fatalf("tee sink did not observe the post-resume append: %q", got)
	}
	if bytes.Contains(sink.Bytes(), []byte("first prompt")) {
		t.Fatalf("tee sink must be LIVE-ONLY (replay is not re-fanned-out): %q", sink.String())
	}
}

func TestResumeFileTeeAbsentFileFailsClosed(t *testing.T) {
	var sink bytes.Buffer
	_, err := ResumeFileTee(filepath.Join(t.TempDir(), "no-such.jsonl"), &sink)
	if err == nil {
		t.Fatal("resuming an absent log must fail closed (never create)")
	}
}

func TestResumeFileTeeTornTailDropped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sess-torn.jsonl")
	lg, err := OpenFile(path, "sess-torn")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lg.AppendPrompt("committed"); err != nil {
		t.Fatal(err)
	}
	if err := lg.Close(); err != nil {
		t.Fatal(err)
	}
	// Simulate a torn final write: a partial record with NO newline.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"seq":3,"type":"llm/re`); err != nil { // torn
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	lg2, err := ResumeFileTee(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer lg2.Close()
	if n := len(lg2.Events()); n != 2 {
		t.Fatalf("torn tail must be dropped: %d events", n)
	}
	// The resumed writer continues from the valid prefix.
	ev, err := lg2.AppendPrompt("post-crash")
	if err != nil {
		t.Fatal(err)
	}
	if ev.Seq != 3 {
		t.Fatalf("post-recovery seq = %d, want 3", ev.Seq)
	}
}

func TestResumeFileRemainsFileOnly(t *testing.T) {
	// Regression guard: ResumeFile keeps its exact pre-P4 behavior
	// (no tee), delegating through ResumeFileTee with a nil sink.
	dir := t.TempDir()
	path := filepath.Join(dir, "sess-rf.jsonl")
	lg1, err := OpenFile(path, "sess-rf")
	if err != nil {
		t.Fatal(err)
	}
	if err := lg1.Close(); err != nil {
		t.Fatal(err)
	}
	lg2, err := ResumeFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lg2.Close()
	if lg2.SessionID() != "sess-rf" {
		t.Fatalf("SessionID = %q", lg2.SessionID())
	}
	if _, err := lg2.AppendTurnBegin(); err != nil {
		t.Fatal(err)
	}
}
