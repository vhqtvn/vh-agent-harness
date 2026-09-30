package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// turnAdapter is the fake adapter: one scripted response, every request
// recorded.
type turnAdapter struct {
	resp *adapters.Response
	saw  []*adapters.Request
}

func (a *turnAdapter) Name() string { return "turn-fake" }

func (a *turnAdapter) Call(_ context.Context, req *adapters.Request) (*adapters.Response, error) {
	cp := *req
	a.saw = append(a.saw, &cp)
	rc := *a.resp
	return &rc, nil
}

type errAdapter struct{ err error }

func (a *errAdapter) Name() string { return "turn-err" }

func (a *errAdapter) Call(context.Context, *adapters.Request) (*adapters.Response, error) {
	return nil, a.err
}

func fixedLog(t *testing.T, id string, sink *writeBuffer) *session.Log {
	t.Helper()
	lg, err := session.NewLog(sink, id, time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	return lg
}

func assertEventSequence(t *testing.T, lg *session.Log, want []string) {
	t.Helper()
	got := eventTypesOf(lg)
	if len(got) != len(want) {
		t.Fatalf("event sequence = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("events[%d] = %s, want %s (all = %v)", i, got[i], want[i], got)
		}
	}
}

// replayDeterminism is the keystone assertion: replaying the persisted
// bytes must reproduce the live event list byte-for-byte AND the derived
// surface byte-for-byte.
func replayDeterminism(t *testing.T, sink *writeBuffer, lg *session.Log) []session.Message {
	t.Helper()
	liveMsgs, err := lg.Surface()
	if err != nil {
		t.Fatalf("live Surface: %v", err)
	}
	liveEvents := lg.Events()
	liveEventsJSON, err := json.Marshal(liveEvents)
	if err != nil {
		t.Fatalf("marshal live events: %v", err)
	}
	liveJSON, err := json.Marshal(liveMsgs)
	if err != nil {
		t.Fatalf("marshal live surface: %v", err)
	}

	replayed, err := session.Replay(bytes.NewReader(sink.data))
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	replayEventsJSON, err := json.Marshal(replayed)
	if err != nil {
		t.Fatalf("marshal replayed events: %v", err)
	}
	if !bytes.Equal(liveEventsJSON, replayEventsJSON) {
		t.Fatalf("event log not byte-identical after replay:\nlive:   %s\nreplay: %s", liveEventsJSON, replayEventsJSON)
	}
	replayMsgs, err := session.DeriveMessages(replayed)
	if err != nil {
		t.Fatalf("DeriveMessages: %v", err)
	}
	replayJSON, err := json.Marshal(replayMsgs)
	if err != nil {
		t.Fatalf("marshal replayed surface: %v", err)
	}
	if !bytes.Equal(liveJSON, replayJSON) {
		t.Fatalf("surface not byte-identical after replay:\nlive:   %s\nreplay: %s", liveJSON, replayJSON)
	}
	return replayMsgs
}

// --- the mission's two-call shape ------------------------------------------

func TestRunTurnTwoCallsOneDeniedReplayDeterminism(t *testing.T) {
	var sink writeBuffer
	lg := fixedLog(t, "turn-2call", &sink)

	executed := 0
	p := NewPipeline()
	if err := p.Register(ToolDefinition{
		Name: "grep", IsConcurrencySafe: true,
		Execute: func(context.Context, json.RawMessage) (string, error) {
			executed++
			return "2 matches", nil
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := p.Register(ToolDefinition{
		Name: "write_file", // NOT concurrency-safe
		Execute: func(context.Context, json.RawMessage) (string, error) {
			executed += 100
			return "wrote", nil
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	p.AddGuard(denylistGuard{denied: map[string]bool{"write_file": true}})

	ad := &turnAdapter{resp: &adapters.Response{
		Model: "mock-1", FinishReason: "tool_calls",
		ToolCalls: []session.ToolCall{
			{ID: "call_a", Name: "grep", Args: json.RawMessage(`{"q":"waterfall"}`)},
			{ID: "call_b", Name: "write_file", Args: json.RawMessage(`{"path":"/etc/hosts"}`)},
		},
	}}
	report, err := p.RunTurn(context.Background(), lg, ad, TurnOptions{Model: "mock-1", Tools: specsOf(p)}, "search and overwrite")
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}

	if executed != 1 {
		t.Fatalf("executed = %d (grep once; write_file must be denied)", executed)
	}
	assertEventSequence(t, lg, []string{
		session.TypeSessionHeader,
		session.TypeTurnBegin, session.TypeSessionPrompt,
		session.TypeLLMRequest, session.TypeLLMResponse,
		session.TypeToolCall, session.TypeToolCall,
		session.TypeToolResult, session.TypeToolResult,
		session.TypeTurnEnd,
	})
	if len(report.Results) != 2 {
		t.Fatalf("results = %+v", report.Results)
	}
	if report.Results[0].IsError || report.Results[0].Content != "2 matches" {
		t.Fatalf("grep result = %+v", report.Results[0])
	}
	rb := report.Results[1]
	if !rb.Denied || !rb.IsError || rb.DeniedBy != "denylist" {
		t.Fatalf("write_file result = %+v", rb)
	}

	replayMsgs := replayDeterminism(t, &sink, lg)
	if len(replayMsgs) != 4 {
		t.Fatalf("replayed surface = %d messages, want 4", len(replayMsgs))
	}
	wantRoles := []string{"user", "assistant", "tool", "tool"}
	for i, m := range replayMsgs {
		if m.Role != wantRoles[i] {
			t.Fatalf("replayed[%d].Role = %s, want %s", i, m.Role, wantRoles[i])
		}
	}
	if replayMsgs[2].ToolCallID != "call_a" || replayMsgs[3].ToolCallID != "call_b" {
		t.Fatalf("replayed tool messages = %+v %+v", replayMsgs[2], replayMsgs[3])
	}
	if !strings.Contains(replayMsgs[3].Content, "denylist") {
		t.Fatalf("denied replayed content = %q", replayMsgs[3].Content)
	}

	// The one wire request carried both tool advertisements.
	if len(ad.saw) != 1 {
		t.Fatalf("adapter calls = %d, want 1", len(ad.saw))
	}
	if len(ad.saw[0].Tools) != 2 {
		t.Fatalf("request tool specs = %d, want 2", len(ad.saw[0].Tools))
	}
	if len(ad.saw[0].Messages) != 1 || ad.saw[0].Messages[0].Role != "user" {
		t.Fatalf("request messages = %+v", ad.saw[0].Messages)
	}
}

// --- the crux: interleaved parallel execution + denial, replay-identical ----

func TestRunTurnInterleavedParallelDenialReplayDeterminism(t *testing.T) {
	var sink writeBuffer
	lg := fixedLog(t, "turn-crux", &sink)

	slowH := newGateHarness()
	fastDone := make(chan struct{})
	p := NewPipelineWithOptions(PipelineOptions{MaxParallel: 2})
	if err := p.Register(ToolDefinition{
		Name: "slow_scan", IsConcurrencySafe: true,
		Execute: slowH.body,
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := p.Register(ToolDefinition{
		Name: "fast_stat", IsConcurrencySafe: true,
		Execute: func(context.Context, json.RawMessage) (string, error) {
			defer close(fastDone)
			return "stat-ok", nil
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := p.Register(ToolDefinition{
		Name: "destructive", // non-safe AND denied by the guard
		Execute: func(context.Context, json.RawMessage) (string, error) {
			return "should never run", nil
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	p.AddGuard(denylistGuard{denied: map[string]bool{"destructive": true}})

	ad := &turnAdapter{resp: &adapters.Response{
		Model: "mock-1", FinishReason: "tool_calls",
		ToolCalls: []session.ToolCall{
			{ID: "call_slow", Name: "slow_scan", Args: json.RawMessage(`{"q":"a"}`)},
			{ID: "call_fast", Name: "fast_stat", Args: json.RawMessage(`{"q":"b"}`)},
			{ID: "call_boom", Name: "destructive", Args: json.RawMessage(`{"q":"c"}`)},
		},
	}}
	type turnOutcome struct {
		report *TurnReport
		err    error
	}
	outCh := make(chan turnOutcome, 1)
	go func() {
		r, err := p.RunTurn(context.Background(), lg, ad, TurnOptions{Model: "mock-1"}, "run the batch")
		outCh <- turnOutcome{r, err}
	}()

	// Interleave proof: both parallel bodies dispatched; fast COMPLETES
	// while slow is still gated; the denied call never executes.
	<-slowH.entered
	<-fastDone
	slowH.release()
	out := <-outCh
	if out.err != nil {
		t.Fatalf("RunTurn: %v", out.err)
	}

	// Model-ordered results: slow, fast, denied — regardless of the fact
	// that fast finished first.
	assertEventSequence(t, lg, []string{
		session.TypeSessionHeader,
		session.TypeTurnBegin, session.TypeSessionPrompt,
		session.TypeLLMRequest, session.TypeLLMResponse,
		session.TypeToolCall, session.TypeToolCall, session.TypeToolCall,
		session.TypeToolResult, session.TypeToolResult, session.TypeToolResult,
		session.TypeTurnEnd,
	})
	var trs []session.ToolResultPayload
	for _, ev := range lg.Events() {
		if ev.Type != session.TypeToolResult {
			continue
		}
		var tr session.ToolResultPayload
		if err := json.Unmarshal(ev.Payload, &tr); err != nil {
			t.Fatalf("payload: %v", err)
		}
		trs = append(trs, tr)
	}
	if len(trs) != 3 {
		t.Fatalf("results = %+v", trs)
	}
	if trs[0].CallID != "call_slow" || trs[0].Content != "gated-ok" || trs[0].IsError {
		t.Fatalf("trs[0] = %+v", trs[0])
	}
	if trs[1].CallID != "call_fast" || trs[1].Content != "stat-ok" || trs[1].IsError {
		t.Fatalf("trs[1] = %+v", trs[1])
	}
	if !trs[2].Denied || !trs[2].IsError || trs[2].DeniedBy != "denylist" {
		t.Fatalf("trs[2] = %+v", trs[2])
	}

	replayMsgs := replayDeterminism(t, &sink, lg)
	if len(replayMsgs) != 5 { // user, assistant(3 calls), tool ×3
		t.Fatalf("replayed surface = %d messages, want 5", len(replayMsgs))
	}
	if len(replayMsgs[1].ToolCalls) != 3 {
		t.Fatalf("replayed assistant tool calls = %d", len(replayMsgs[1].ToolCalls))
	}
	if out.report == nil || len(out.report.Results) != 3 {
		t.Fatalf("report results = %+v", out.report)
	}
}

// --- turn closure shapes -----------------------------------------------------

func TestRunTurnNoToolCallsEndsTurn(t *testing.T) {
	var sink writeBuffer
	lg := fixedLog(t, "turn-plain", &sink)
	p := NewPipeline()
	ad := &turnAdapter{resp: &adapters.Response{Model: "mock-1", Content: "all done", FinishReason: "stop"}}
	report, err := p.RunTurn(context.Background(), lg, ad, TurnOptions{Model: "mock-1"}, "just talk")
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if len(report.Results) != 0 || report.Response.Content != "all done" {
		t.Fatalf("report = %+v", report)
	}
	assertEventSequence(t, lg, []string{
		session.TypeSessionHeader,
		session.TypeTurnBegin, session.TypeSessionPrompt,
		session.TypeLLMRequest, session.TypeLLMResponse,
		session.TypeTurnEnd,
	})
	replayDeterminism(t, &sink, lg)
}

func TestRunTurnAdapterErrorClosesTurnWithError(t *testing.T) {
	var sink writeBuffer
	lg := fixedLog(t, "turn-err", &sink)
	p := NewPipeline()
	_, err := p.RunTurn(context.Background(), lg, &errAdapter{err: errString("provider down")}, TurnOptions{Model: "mock-1"}, "try")
	if err == nil {
		t.Fatal("expected adapter error to surface")
	}
	events := lg.Events()
	last := events[len(events)-1]
	if last.Type != session.TypeTurnEnd {
		t.Fatalf("last event = %s", last.Type)
	}
	var te session.TurnEndPayload
	if err := json.Unmarshal(last.Payload, &te); err != nil {
		t.Fatalf("turn/end payload: %v", err)
	}
	if te.Kind != "error" || !jsonContains(last.Payload, "provider down") {
		t.Fatalf("turn/end = %+v", te)
	}
}

func jsonContains(raw json.RawMessage, sub string) bool {
	return bytes.Contains(raw, []byte(sub))
}

// TestRunTurnInboxDrivenSkipsPromptAppend (B2): with InboxDriven the
// turn appends NO session/prompt — it answers messages already on the
// log (the subagent child inbox shape). The zero value keeps the B1
// choreography (covered by every other test here).
func TestRunTurnInboxDrivenSkipsPromptAppend(t *testing.T) {
	var sink writeBuffer
	lg := fixedLog(t, "turn-inbox", &sink)
	// Pre-seed the inbox the way internal/subagents does: one surface-
	// appended subagent/message (a user-role message on the surface).
	if _, err := lg.Append(session.TypeSubagentMessage, &session.SurfaceOp{Op: session.SurfaceOpAppend}, session.SubagentPayload{
		ChildID: "turn-inbox.1", From: "turn-inbox", Text: "study the inbox",
	}); err != nil {
		t.Fatalf("seed inbox: %v", err)
	}
	p := NewPipeline()
	ad := &turnAdapter{resp: &adapters.Response{Model: "mock-1", Content: "answered from inbox", FinishReason: "stop"}}
	opts := TurnOptions{Model: "mock-1"}
	opts.InboxDriven = true
	report, err := p.RunTurn(context.Background(), lg, ad, opts, "ignored prompt argument")
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if report.Response.Content != "answered from inbox" {
		t.Fatalf("report = %+v", report)
	}
	// The adapter answered the inbox message, and the turn/end closes a
	// COMPLETED turn (kind "").
	assertEventSequence(t, lg, []string{
		session.TypeSessionHeader,
		session.TypeSubagentMessage, // the inbox (pre-seeded)
		session.TypeTurnBegin,
		session.TypeLLMRequest, session.TypeLLMResponse,
		session.TypeTurnEnd,
	})
	if len(ad.saw) != 1 {
		t.Fatalf("adapter calls = %d", len(ad.saw))
	}
	replayDeterminism(t, &sink, lg)
}

func specsOf(p *Pipeline) []adapters.ToolSpec {
	defs := p.Definitions()
	out := make([]adapters.ToolSpec, 0, len(defs))
	for _, d := range defs {
		out = append(out, d.Spec())
	}
	return out
}
