package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// scriptedAdapter serves a queue of responses and records every request.
type scriptedAdapter struct {
	mu    sync.Mutex
	resps []*adapters.Response
	saw   []*adapters.Request
}

func (a *scriptedAdapter) Name() string { return "scripted" }

func (a *scriptedAdapter) Call(_ context.Context, req *adapters.Request) (*adapters.Response, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	cp := *req
	a.saw = append(a.saw, &cp)
	if len(a.saw) > len(a.resps) {
		return nil, errOutOfScript
	}
	r := *a.resps[len(a.saw)-1]
	return &r, nil
}

var errOutOfScript = &simpleError{"scripted adapter: no more scripted responses"}

type simpleError struct{ s string }

func (e *simpleError) Error() string { return e.s }

// gateExec blocks every Run until the test opens the gate — the fixture
// that proves the TURN completes without waiting on job execution.
type gateExec struct {
	entered chan string
	gate    chan struct{}
}

func (e *gateExec) Run(_ context.Context, job Job) error {
	e.entered <- job.ID
	<-e.gate
	return nil
}

// TestRunTurnDispatchesBackgroundJobEndToEnd is the slice's integration
// keystone: a tool called mid-turn dispatches a background job through
// the jobs manager; the turn completes WITHOUT waiting (the gate stays
// closed past turn/end); the job settles after the turn; the report
// notice appears on the next surface fold and the model sees it on the
// next turn; and the whole log replays byte-stable.
func TestRunTurnDispatchesBackgroundJobEndToEnd(t *testing.T) {
	path := filepath.Join(t.TempDir(), "e2e.jsonl")
	lg, err := session.OpenFile(path, "sess-e2e-1")
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}

	entered := make(chan string, 4)
	gate := make(chan struct{})
	exec := &gateExec{entered: entered, gate: gate}
	m, err := NewManager(lg, exec, Options{MaxInFlightPerOwner: 2})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(m.Stop)

	p := tools.NewPipeline()
	if err := p.Register(tools.ToolDefinition{
		Name: "spawn", IsConcurrencySafe: true,
		Execute: func(_ context.Context, args json.RawMessage) (string, error) {
			r, err := m.Dispatch("background", args)
			if err != nil {
				return "", err
			}
			return r.JobID, nil // the tool result is the enqueue receipt
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ad := &scriptedAdapter{resps: []*adapters.Response{
		{Model: "mock-1", Content: "", ToolCalls: []adapters.ToolCall{{
			ID: "call_1", Name: "spawn", Args: json.RawMessage(`{"cmd":"rebuild"}`),
		}}, Usage: adapters.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}},
		{Model: "mock-1", Content: "noticed the background result", Usage: adapters.Usage{PromptTokens: 40, CompletionTokens: 6, TotalTokens: 46}},
	}}

	// Turn 1: model asks to spawn; the tool body enqueues background-1 and
	// returns the receipt; the turn closes with the gate STILL CLOSED —
	// the job has not even finished, proving non-blocking dispatch.
	report, err := p.RunTurn(context.Background(), lg, ad, tools.TurnOptions{Model: "mock-1"}, "run the rebuild in the background")
	if err != nil {
		t.Fatalf("RunTurn 1: %v", err)
	}
	if len(report.Results) != 1 {
		t.Fatalf("turn 1 results = %d, want 1", len(report.Results))
	}
	if report.Results[0].Content != "background-1" {
		t.Fatalf("spawn tool result = %q, want the enqueue receipt background-1", report.Results[0].Content)
	}
	select {
	case id := <-entered:
		if id != "background-1" {
			t.Fatalf("executor entered %q, want background-1", id)
		}
		// The job is running with the gate closed.
	case <-time.After(2 * time.Second):
		t.Fatalf("executor never entered the job")
	}
	if hasTurnEnd(t, lg) != true {
		t.Fatalf("turn/end not logged; the turn waited on the job")
	}

	// The job settles AFTER the turn (gate opens now).
	close(gate)
	m.Drain()

	// Report on the next surface fold: exactly one notice, user role,
	// naming the job and its completed result.
	n, err := m.EmitReports()
	if err != nil {
		t.Fatalf("EmitReports: %v", err)
	}
	if n != 1 {
		t.Fatalf("EmitReports emitted %d, want 1", n)
	}
	surf, err := lg.Surface()
	if err != nil {
		t.Fatalf("Surface: %v", err)
	}
	notices := 0
	for _, msg := range surf {
		if bytes.Contains([]byte(msg.Content), []byte("background-1")) && bytes.Contains([]byte(msg.Content), []byte(session.JobResultCompleted)) {
			notices++
			if msg.Role != "user" {
				t.Fatalf("report notice role = %q, want user", msg.Role)
			}
		}
	}
	if notices != 1 {
		t.Fatalf("found %d report notices on the surface, want 1", notices)
	}

	// Turn 2: the model's request surface must carry the notice (the
	// report reached the conversation).
	if _, err := p.RunTurn(context.Background(), lg, ad, tools.TurnOptions{Model: "mock-1"}, "thanks"); err != nil {
		t.Fatalf("RunTurn 2: %v", err)
	}
	ad.mu.Lock()
	second := ad.saw[1]
	ad.mu.Unlock()
	carried := 0
	for _, msg := range second.Messages {
		if bytes.Contains([]byte(msg.Content), []byte("background-1")) && bytes.Contains([]byte(msg.Content), []byte(session.JobResultCompleted)) {
			carried++
		}
	}
	if carried != 1 {
		t.Fatalf("turn-2 request carried %d report notices, want 1", carried)
	}

	// Whole-log byte stability: replay reproduces the event list and the
	// derived surface byte-for-byte.
	liveEvents := lg.Events()
	liveMsgs, err := lg.Surface()
	if err != nil {
		t.Fatalf("Surface: %v", err)
	}
	liveEventsJSON, _ := json.Marshal(liveEvents)
	liveJSON, _ := json.Marshal(liveMsgs)
	replayed, err := session.ReplayFile(path)
	if err != nil {
		t.Fatalf("ReplayFile: %v", err)
	}
	replayEventsJSON, _ := json.Marshal(replayed)
	replayMsgs, err := session.DeriveMessages(replayed)
	if err != nil {
		t.Fatalf("DeriveMessages: %v", err)
	}
	replayJSON, _ := json.Marshal(replayMsgs)
	if !bytes.Equal(liveEventsJSON, replayEventsJSON) {
		t.Fatalf("e2e log not byte-identical after replay:\nlive:   %s\nreplay: %s", liveEventsJSON, replayEventsJSON)
	}
	if !bytes.Equal(liveJSON, replayJSON) {
		t.Fatalf("e2e surface not byte-identical after replay:\nlive:   %s\nreplay: %s", liveJSON, replayJSON)
	}
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func hasTurnEnd(t *testing.T, lg *session.Log) bool {
	t.Helper()
	for _, ev := range lg.Events() {
		if ev.Type == session.TypeTurnEnd {
			return true
		}
	}
	return false
}
