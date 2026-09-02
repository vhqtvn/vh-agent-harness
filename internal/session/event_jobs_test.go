package session

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

// jobSink is an in-memory log sink for the job-event tests.
type jobSink struct{ bytes.Buffer }

func (s *jobSink) Write(p []byte) (int, error) { return s.Buffer.Write(p) }

// jobTestLog opens a log with a fixed clock on an in-memory sink.
func jobTestLog(t *testing.T, id string, sink *jobSink) *Log {
	t.Helper()
	lg, err := NewLog(sink, id, time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	return lg
}

// TestJobEventsKnownAndAppendable is the slice-4 admission test: the four
// job/* event types join the closed set, append through the typed Append
// API, and round-trip through replay byte-identically.
func TestJobEventsKnownAndAppendable(t *testing.T) {
	for _, typ := range []string{TypeJobEnqueued, TypeJobStarted, TypeJobSettled, TypeJobReport} {
		if typ == "" {
			t.Fatalf("job event type constant is empty")
		}
		if !knownTypes[typ] {
			t.Fatalf("event type %q missing from the closed knownTypes set", typ)
		}
	}

	var sink jobSink
	lg := jobTestLog(t, "sess-jobs-1", &sink)

	if _, err := lg.Append(TypeJobEnqueued, nil, JobPayload{
		JobID: "background-1", Kind: "background", Owner: "sess-jobs-1",
		Payload: json.RawMessage(`{"cmd":"sleep 1"}`),
	}); err != nil {
		t.Fatalf("Append job/enqueued: %v", err)
	}
	if _, err := lg.Append(TypeJobStarted, nil, JobPayload{
		JobID: "background-1", Kind: "background", Owner: "sess-jobs-1",
	}); err != nil {
		t.Fatalf("Append job/started: %v", err)
	}
	if _, err := lg.Append(TypeJobSettled, nil, JobPayload{
		JobID: "background-1", Kind: "background", Owner: "sess-jobs-1",
		Result: JobResultCompleted,
	}); err != nil {
		t.Fatalf("Append job/settled: %v", err)
	}
	if _, err := lg.Append(TypeJobReport, &SurfaceOp{Op: SurfaceOpAppend}, JobPayload{
		JobID: "background-1", Kind: "background", Owner: "sess-jobs-1",
		Result: JobResultCompleted,
	}); err != nil {
		t.Fatalf("Append job/report: %v", err)
	}

	live := lg.Events()
	if len(live) != 5 {
		t.Fatalf("live event count = %d, want 5", len(live))
	}

	// Replay must reproduce the event list byte-identically.
	replayed, err := Replay(bytes.NewReader(sink.Bytes()))
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	liveJSON, _ := json.Marshal(live)
	replayJSON, _ := json.Marshal(replayed)
	if !bytes.Equal(liveJSON, replayJSON) {
		t.Fatalf("job events not byte-identical after replay:\nlive:   %s\nreplay: %s", liveJSON, replayJSON)
	}
}

// TestJobLogOnlyEventsLeaveSurfaceUntouched: enqueued/started/settled are
// log-only (dsh: job state rides the same durable stream without
// polluting the model surface); only job/report is message-bearing.
func TestJobLogOnlyEventsLeaveSurfaceUntouched(t *testing.T) {
	var sink jobSink
	lg := jobTestLog(t, "sess-jobs-2", &sink)

	if _, err := lg.AppendPrompt("run a background thing"); err != nil {
		t.Fatalf("AppendPrompt: %v", err)
	}
	before, err := lg.Surface()
	if err != nil {
		t.Fatalf("Surface before: %v", err)
	}

	for _, p := range []JobPayload{
		{JobID: "background-1", Kind: "background", Owner: "sess-jobs-2", Payload: json.RawMessage(`{}`)},
		{JobID: "background-1", Kind: "background", Owner: "sess-jobs-2"},
		{JobID: "background-1", Kind: "background", Owner: "sess-jobs-2", Result: JobResultFailed, Reason: "boom"},
	} {
		if _, err := lg.Append(TypeJobEnqueued, nil, p); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	after, err := lg.Surface()
	if err != nil {
		t.Fatalf("Surface after: %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("log-only job events changed the surface: %d -> %d messages", len(before), len(after))
	}
}

// TestJobReportSurfaceMessage: job/report is the ONE message-bearing job
// event (dsh reported-flag delivery: the settled-job notice reaches the
// model exactly once, as a user-role environment notice — same family as
// the compaction summary, never assistant self-talk).
func TestJobReportSurfaceMessage(t *testing.T) {
	var sink jobSink
	lg := jobTestLog(t, "sess-jobs-3", &sink)

	if _, err := lg.Append(TypeJobReport, &SurfaceOp{Op: SurfaceOpAppend}, JobPayload{
		JobID: "background-2", Kind: "background", Owner: "sess-jobs-3",
		Result: JobResultCompleted,
	}); err != nil {
		t.Fatalf("Append job/report: %v", err)
	}
	if _, err := lg.Append(TypeJobReport, &SurfaceOp{Op: SurfaceOpAppend}, JobPayload{
		JobID: "background-3", Kind: "background", Owner: "sess-jobs-3",
		Result: JobResultFailed, Reason: "exit 1",
	}); err != nil {
		t.Fatalf("Append job/report (failed): %v", err)
	}

	msgs, err := lg.Surface()
	if err != nil {
		t.Fatalf("Surface: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("surface length = %d, want 2 report notices", len(msgs))
	}
	for i, m := range msgs {
		if m.Role != "user" {
			t.Fatalf("msgs[%d].Role = %q, want user (environment->model notice)", i, m.Role)
		}
	}
	if !containsMsg(msgs[0].Content, "background-2") || !containsMsg(msgs[0].Content, "completed") {
		t.Fatalf("completed report content does not identify job+result: %q", msgs[0].Content)
	}
	if !containsMsg(msgs[1].Content, "background-3") || !containsMsg(msgs[1].Content, "failed") || !containsMsg(msgs[1].Content, "exit 1") {
		t.Fatalf("failed report content does not identify job+result+reason: %q", msgs[1].Content)
	}
}

// TestJobReportRequiresSurfaceOp: the message-bearing contract applies to
// job/report like every other surface event.
func TestJobReportRequiresSurfaceOp(t *testing.T) {
	var sink jobSink
	lg := jobTestLog(t, "sess-jobs-4", &sink)
	if _, err := lg.Append(TypeJobReport, nil, JobPayload{JobID: "background-1"}); err == nil {
		t.Fatalf("job/report without surfaceOp appended; want fail-loud error")
	}
}

// TestOldLogsReplayStableAfterJobEventAdditions is the omitempty-discipline
// guard the mission demands: a pre-slice-4 log (prompt/response/tool/turn
// plus slice-2 retry records) must replay byte-identically and fold the
// same surface after the job/* types join the closed set.
func TestOldLogsReplayStableAfterJobEventAdditions(t *testing.T) {
	var sink jobSink
	lg := jobTestLog(t, "sess-old-1", &sink)

	if _, err := lg.AppendTurnBegin(); err != nil {
		t.Fatalf("AppendTurnBegin: %v", err)
	}
	if _, err := lg.AppendPrompt("old log"); err != nil {
		t.Fatalf("AppendPrompt: %v", err)
	}
	if _, err := lg.AppendLLMRequest("mock-1", []string{"echo"}, nil, 64); err != nil {
		t.Fatalf("AppendLLMRequest: %v", err)
	}
	if _, err := lg.AppendLLMResponse("mock-1", "", []ToolCall{{
		ID: "call_1", Name: "echo", Args: json.RawMessage(`{"text":"x"}`),
	}}, Usage{PromptTokens: 4, CompletionTokens: 2, TotalTokens: 6}); err != nil {
		t.Fatalf("AppendLLMResponse: %v", err)
	}
	if _, err := lg.AppendToolCall("call_1", "echo", json.RawMessage(`{"text":"x"}`)); err != nil {
		t.Fatalf("AppendToolCall: %v", err)
	}
	if _, err := lg.AppendToolResult("call_1", "echo", "x", false); err != nil {
		t.Fatalf("AppendToolResult: %v", err)
	}
	if _, err := lg.AppendTurnEnd(""); err != nil {
		t.Fatalf("AppendTurnEnd: %v", err)
	}

	liveEvents := lg.Events()
	liveMsgs, err := lg.Surface()
	if err != nil {
		t.Fatalf("Surface: %v", err)
	}
	liveEventsJSON, _ := json.Marshal(liveEvents)
	liveJSON, _ := json.Marshal(liveMsgs)

	replayed, err := Replay(bytes.NewReader(sink.Bytes()))
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	replayEventsJSON, _ := json.Marshal(replayed)
	replayMsgs, err := DeriveMessages(replayed)
	if err != nil {
		t.Fatalf("DeriveMessages: %v", err)
	}
	replayJSON, _ := json.Marshal(replayMsgs)

	if !bytes.Equal(liveEventsJSON, replayEventsJSON) {
		t.Fatalf("old log events changed by replay:\nlive:   %s\nreplay: %s", liveEventsJSON, replayEventsJSON)
	}
	if !bytes.Equal(liveJSON, replayJSON) {
		t.Fatalf("old log surface changed by replay:\nlive:   %s\nreplay: %s", liveJSON, replayJSON)
	}
}

// TestUnknownJobLikeEventStillFailsClosed: adding job/* to the known set
// must not open the door for arbitrary unknown types.
func TestUnknownJobLikeEventStillFailsClosed(t *testing.T) {
	line := `{"seq":1,"type":"session/header","payload":{"sessionId":"s","formatVersion":0,"createdAt":"2026-08-20T00:00:00Z"}}` + "\n" +
		`{"seq":2,"type":"job/bogus","payload":{"jobId":"background-1"}}` + "\n"
	_, err := Replay(bytes.NewReader([]byte(line)))
	if err == nil {
		t.Fatalf("unknown job/bogus replayed without error; want fail-closed refusal")
	}
	if _, ok := err.(*UnknownEventError); !ok {
		t.Fatalf("error type = %T, want *UnknownEventError", err)
	}
}

func containsMsg(s, sub string) bool {
	return bytes.Contains([]byte(s), []byte(sub))
}
