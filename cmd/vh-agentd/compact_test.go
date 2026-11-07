// compact_test.go — P5 compaction wiring crux tests: the KV-prefix
// summarize request shape, the turn-boundary trigger (at/below
// pressure), the failure postures (SummaryTooLarge, provider failure —
// turn never affected, next boundary retries), the child-turn skip,
// the disable path, flag validation, replay determinism + Unfold over
// a REAL compacted log, and the full wired path through buildServer +
// the protocol server (flags → decorator → handler turn gate).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
	"github.com/vhqtvn/vh-agent-harness/internal/protocol"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// --- test fakes --------------------------------------------------------------

// fakeTurnRunner simulates ONE successful tool turn against lg by
// appending the real event vocabulary (turn/begin … turn/end with a
// prompt, one assistant response requesting two echo calls, and their
// results) — the same choreography shape RunTurn commits, without the
// pipeline. It makes no adapter call; the adapter argument reaches it
// only so the decorator can build the summarizer (the production path:
// the decorator reuses the turn's own adapter + options).
type fakeTurnRunner struct{}

func (f *fakeTurnRunner) RunTurn(ctx context.Context, lg *session.Log, ad adapters.Adapter, opts tools.TurnOptions, prompt string) (*tools.TurnReport, error) {
	if _, err := lg.AppendTurnBegin(); err != nil {
		return nil, err
	}
	if !opts.InboxDriven {
		if _, err := lg.AppendPrompt(prompt); err != nil {
			return nil, err
		}
	}
	if _, err := lg.AppendLLMRequest(opts.Model, nil, nil, 0); err != nil {
		return nil, err
	}
	calls := []session.ToolCall{
		{ID: "c1", Name: "echo", Args: json.RawMessage(`{"text":"result-one"}`)},
		{ID: "c2", Name: "echo", Args: json.RawMessage(`{"text":"result-two"}`)},
	}
	if _, err := lg.AppendLLMResponse(opts.Model, "", calls, session.Usage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2}); err != nil {
		return nil, err
	}
	var results []tools.Result
	for _, c := range calls {
		if _, err := lg.AppendToolResult(c.ID, c.Name, "tool output for "+c.ID, false); err != nil {
			return nil, err
		}
		results = append(results, tools.Result{CallID: c.ID, Name: c.Name, Content: "tool output for " + c.ID})
	}
	if _, err := lg.AppendTurnEnd(""); err != nil {
		return nil, err
	}
	return &tools.TurnReport{
		Response: &adapters.Response{Model: opts.Model, Content: "", ToolCalls: calls},
		Results:  results,
	}, nil
}

// captureAdapter records every request it serves (the summarize calls)
// and answers from a scripted FIFO of response/error constructors; an
// exhausted script serves a fixed short summary.
type captureAdapter struct {
	mu       sync.Mutex
	requests []*adapters.Request
	script   []func() (*adapters.Response, error)
}

func (a *captureAdapter) Name() string { return "capture" }

func (a *captureAdapter) Call(ctx context.Context, req *adapters.Request) (*adapters.Response, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	cp := *req
	a.requests = append(a.requests, &cp)
	if len(a.script) > 0 {
		next := a.script[0]
		a.script = a.script[1:]
		return next()
	}
	return &adapters.Response{Model: req.Model, Content: "fallback summary of the shadowed span."}, nil
}

func (a *captureAdapter) callCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.requests)
}

func (a *captureAdapter) lastRequest() *adapters.Request {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.requests) == 0 {
		return nil
	}
	return a.requests[len(a.requests)-1]
}

// lines collects the decorator's one-line diagnostics.
type lines struct {
	mu sync.Mutex
	b  []string
}

func (l *lines) printf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.b = append(l.b, fmt.Sprintf(format, args...))
}

func (l *lines) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.b)
}

func (l *lines) joined() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.b, "\n")
}

// newTestLog opens a file-backed session log under t.TempDir().
func newTestLog(t *testing.T) (*session.Log, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sess-compact.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create log: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	lg, err := session.NewLog(f, "sess-compact", time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("new log: %v", err)
	}
	return lg, path
}

// compactionCounts counts the compaction/* events in ev.
func compactionCounts(evs []session.Event) (starts, summaries, ends int) {
	for _, e := range evs {
		switch e.Type {
		case session.TypeCompactionStart:
			starts++
		case session.TypeCompactionSummary:
			summaries++
		case session.TypeCompactionEnd:
			ends++
		}
	}
	return
}

var testOpts = tools.TurnOptions{
	Model:  "fake-model",
	System: "You are the daemon under test.",
	Tools: []adapters.ToolSpec{
		{Name: "echo", Description: "echo probe"},
		{Name: "clock", Description: "clock probe"},
	},
}

// bigPrompt puts enough chars in the surface head that the shadowed
// span dominates the pressure estimate and a modest summary is
// strictly smaller than the span (1984 chars).
var bigPrompt = strings.Repeat("context that must be condensed. ", 64)

// runOneTurn drives the decorated runner through one fake turn.
func runOneTurn(t *testing.T, dec protocol.TurnRunner, lg *session.Log, ad adapters.Adapter, opts tools.TurnOptions, prompt string) *tools.TurnReport {
	t.Helper()
	report, err := dec.RunTurn(context.Background(), lg, ad, opts, prompt)
	if err != nil {
		t.Fatalf("decorated RunTurn failed (compaction must never fail the turn): %v", err)
	}
	if report == nil {
		t.Fatal("decorated RunTurn returned a nil report")
	}
	return report
}

// --- KV-prefix request shape (crux) ------------------------------------------

// TestAdapterSummarizerKVPrefixShape asserts the summarize request is a
// genuine prefix of the running conversation: SAME system message,
// SAME tool advertisements, the shadowed-region messages verbatim, and
// exactly ONE appended instruction — nothing else new.
func TestAdapterSummarizerKVPrefixShape(t *testing.T) {
	lg, _ := newTestLog(t)
	inner := &fakeTurnRunner{}
	ad := &captureAdapter{}
	notes := &lines{}
	// Budget 200: surface after one big turn ≈ 2022 chars ≈ 505
	// estimated tokens → pressure ≈ 2.5 ≥ 0.8 → compaction fires.
	dec := newCompactingTurnRunner(inner, session.SessionConfig{ContextBudgetTokens: 200}, notes.printf)

	runOneTurn(t, dec, lg, ad, testOpts, bigPrompt)

	if got := ad.callCount(); got != 1 {
		t.Fatalf("summarize calls = %d, want 1", got)
	}
	evs := lg.Events()
	// The pre-compaction surface: the fold of everything before the
	// compaction/summary event (the fold Compact itself observed).
	idx := -1
	for i, e := range evs {
		if e.Type == session.TypeCompactionSummary {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("no compaction/summary event after the trigger")
	}
	pre, err := session.FoldSurface(evs[:idx])
	if err != nil {
		t.Fatalf("pre-compaction fold: %v", err)
	}
	// [prompt, assistant(toolcalls), result c1, result c2], shadow =
	// [0, len-retain) = [0,2).
	if len(pre.Messages) != 4 {
		t.Fatalf("pre-compaction surface = %d messages, want 4", len(pre.Messages))
	}
	var sp session.CompactionSummaryPayload
	if err := json.Unmarshal(evs[idx].Payload, &sp); err != nil {
		t.Fatalf("summary payload: %v", err)
	}
	if sp.ShadowedRange != [2]int{0, 2} {
		t.Fatalf("shadowedRange = %v, want [0 2] (surface of 4 minus retain-tail 2)", sp.ShadowedRange)
	}

	req := ad.lastRequest()
	if req.Model != testOpts.Model {
		t.Fatalf("summarize request model = %q, want %q (the turn's own model)", req.Model, testOpts.Model)
	}
	if !reflect.DeepEqual(req.Tools, testOpts.Tools) {
		t.Fatalf("summarize request tools = %+v, want the turn's own tool advertisements %+v", req.Tools, testOpts.Tools)
	}
	// Message shape: [system] + shadowed[0:2] + [instruction].
	wantMsgs := make([]adapters.Message, 0, 4)
	wantMsgs = append(wantMsgs, adapters.Message{Role: "system", Content: testOpts.System})
	wantMsgs = append(wantMsgs, pre.Messages[0], pre.Messages[1])
	wantMsgs = append(wantMsgs, adapters.Message{Role: "user", Content: compactionInstruction})
	if !reflect.DeepEqual(req.Messages, wantMsgs) {
		t.Fatalf("summarize request messages not the KV-prefix shape:\n got %+v\nwant %+v", req.Messages, wantMsgs)
	}
	if len(req.Messages) != 4 {
		t.Fatalf("summarize messages = %d, want 4 (system + 2 shadowed + instruction)", len(req.Messages))
	}
	// The shadowed segment must be the PRE-compaction surface head.
	if req.Messages[1].Content != bigPrompt {
		t.Fatalf("shadowed message 0 = %q…, want the big prompt", req.Messages[1].Content[:40])
	}
}

// --- trigger ------------------------------------------------------------------

// TestCompactionTriggerAtPressure proves the wired trigger: after a
// successful turn at/above threshold, compaction lands (start+summary+
// end, generation 1) and the surface head is replaced (message count
// drops by shadow-1).
func TestCompactionTriggerAtPressure(t *testing.T) {
	lg, _ := newTestLog(t)
	ad := &captureAdapter{}
	notes := &lines{}
	dec := newCompactingTurnRunner(&fakeTurnRunner{}, session.SessionConfig{ContextBudgetTokens: 200}, notes.printf)

	runOneTurn(t, dec, lg, ad, testOpts, bigPrompt)

	starts, summaries, ends := compactionCounts(lg.Events())
	if starts != 1 || summaries != 1 || ends != 1 {
		t.Fatalf("compaction events start/summary/end = %d/%d/%d, want 1/1/1", starts, summaries, ends)
	}
	gen, err := lg.ReplaceGeneration()
	if err != nil || gen != 1 {
		t.Fatalf("replaceGeneration = %d err=%v, want 1", gen, err)
	}
	post, err := lg.Surface()
	if err != nil {
		t.Fatalf("post surface: %v", err)
	}
	// Pre: [prompt, asst, r1, r2] (4). Post: [summary, r1, r2] (3).
	if len(post) != 3 {
		t.Fatalf("post-compaction surface = %d messages, want 3 (summary + retained tail)", len(post))
	}
	if post[0].Role != "user" || post[0].Content != "fallback summary of the shadowed span." {
		t.Fatalf("surface head = %+v, want the user-role summary", post[0])
	}
	if notes.count() != 1 {
		t.Fatalf("diagnostic lines = %d, want exactly one success line", notes.count())
	}
}

// TestCompactionBelowThresholdNoop: pressure under the threshold ⇒ no
// compaction events, no summarize call, surface untouched.
func TestCompactionBelowThresholdNoop(t *testing.T) {
	lg, _ := newTestLog(t)
	ad := &captureAdapter{}
	notes := &lines{}
	dec := newCompactingTurnRunner(&fakeTurnRunner{}, session.SessionConfig{ContextBudgetTokens: 100000}, notes.printf)

	runOneTurn(t, dec, lg, ad, testOpts, bigPrompt)

	starts, summaries, ends := compactionCounts(lg.Events())
	if starts+summaries+ends != 0 {
		t.Fatalf("compaction events = %d/%d/%d, want none below threshold", starts, summaries, ends)
	}
	if got := ad.callCount(); got != 0 {
		t.Fatalf("summarize calls = %d, want 0 below threshold", got)
	}
	if notes.count() != 0 {
		t.Fatalf("diagnostic lines = %d, want 0 (no attempt below threshold)", notes.count())
	}
}

// --- failure postures ---------------------------------------------------------

// TestCompactionSummaryTooLargeDefers: a summary not strictly smaller
// than the shadowed span is refused — the turn still succeeds, the
// unmatched compaction/start stays as the lock, the surface is
// unchanged, one diagnostic line lands, and the NEXT boundary retries
// to success.
func TestCompactionSummaryTooLargeDefers(t *testing.T) {
	lg, _ := newTestLog(t)
	ad := &captureAdapter{}
	ad.script = append(ad.script, func() (*adapters.Response, error) {
		return &adapters.Response{Model: testOpts.Model, Content: strings.Repeat("x", 4000)}, nil
	})
	notes := &lines{}
	dec := newCompactingTurnRunner(&fakeTurnRunner{}, session.SessionConfig{ContextBudgetTokens: 200}, notes.printf)

	runOneTurn(t, dec, lg, ad, testOpts, bigPrompt)

	starts, summaries, ends := compactionCounts(lg.Events())
	if starts != 1 || summaries != 0 || ends != 0 {
		t.Fatalf("after refusal start/summary/end = %d/%d/%d, want 1/0/0 (unmatched-start lock)", starts, summaries, ends)
	}
	surface, err := lg.Surface()
	if err != nil {
		t.Fatalf("surface: %v", err)
	}
	if len(surface) != 4 || surface[0].Content != bigPrompt {
		t.Fatalf("surface after refused compaction = %d messages, head changed — want the un-compacted 4", len(surface))
	}
	if notes.count() != 1 || !strings.Contains(notes.joined(), "not smaller") {
		t.Fatalf("diagnostic = %q, want one deferred line naming the refusal", notes.joined())
	}

	// Next boundary retries (script exhausted ⇒ short summary) and
	// succeeds despite the earlier unmatched start.
	runOneTurn(t, dec, lg, ad, testOpts, "small second prompt")
	starts, summaries, ends = compactionCounts(lg.Events())
	if summaries != 1 || ends != 1 {
		t.Fatalf("after retry start/summary/end = %d/%d/%d, want the second attempt committed", starts, summaries, ends)
	}
	gen, err := lg.ReplaceGeneration()
	if err != nil || gen != 1 {
		t.Fatalf("replaceGeneration after retry = %d err=%v, want 1", gen, err)
	}
}

// TestCompactionProviderFailureDefers: a typed provider failure on the
// summarize call defers exactly like a refusal — turn unaffected, lock
// left, surface un-compacted, log still replayable, one line naming
// the class, next boundary succeeds.
func TestCompactionProviderFailureDefers(t *testing.T) {
	lg, path := newTestLog(t)
	ad := &captureAdapter{}
	ad.script = append(ad.script, func() (*adapters.Response, error) {
		return nil, adapters.NewAdapterError("capture", adapters.KindHTTP5xx, 500, 0, errors.New("summarize endpoint exploded"))
	})
	notes := &lines{}
	dec := newCompactingTurnRunner(&fakeTurnRunner{}, session.SessionConfig{ContextBudgetTokens: 200}, notes.printf)

	runOneTurn(t, dec, lg, ad, testOpts, bigPrompt)

	starts, summaries, ends := compactionCounts(lg.Events())
	if starts != 1 || summaries != 0 || ends != 0 {
		t.Fatalf("after provider failure start/summary/end = %d/%d/%d, want 1/0/0", starts, summaries, ends)
	}
	if notes.count() != 1 || !strings.Contains(notes.joined(), "http5xx") {
		t.Fatalf("diagnostic = %q, want one deferred line naming the typed class", notes.joined())
	}
	// The log with the unmatched start replays cleanly and derives the
	// un-compacted surface (replay determinism holds through a refusal).
	events, err := session.ReplayFile(path)
	if err != nil {
		t.Fatalf("replay log with unmatched compaction/start: %v", err)
	}
	msgs, err := session.DeriveMessages(events)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if len(msgs) != 4 {
		t.Fatalf("replayed surface = %d messages, want the un-compacted 4", len(msgs))
	}

	// Next boundary: script exhausted ⇒ success.
	runOneTurn(t, dec, lg, ad, testOpts, "second prompt")
	_, summaries, ends = compactionCounts(lg.Events())
	if summaries != 1 || ends != 1 {
		t.Fatalf("after retry summary/end = %d/%d, want 1/1", summaries, ends)
	}
}

// --- coverage postures --------------------------------------------------------

// TestCompactionSkipsChildTurns: InboxDriven (subagent child) turns
// never trigger compaction — v1 compacts parent sessions only.
func TestCompactionSkipsChildTurns(t *testing.T) {
	lg, _ := newTestLog(t)
	ad := &captureAdapter{}
	notes := &lines{}
	dec := newCompactingTurnRunner(&fakeTurnRunner{}, session.SessionConfig{ContextBudgetTokens: 200}, notes.printf)

	childOpts := testOpts
	childOpts.InboxDriven = true
	runOneTurn(t, dec, lg, ad, childOpts, bigPrompt)

	starts, summaries, ends := compactionCounts(lg.Events())
	if starts+summaries+ends != 0 {
		t.Fatalf("child-turn compaction events = %d/%d/%d, want none (parent-only v1)", starts, summaries, ends)
	}
	if got := ad.callCount(); got != 0 {
		t.Fatalf("child-turn summarize calls = %d, want 0", got)
	}
}

// TestCompactionDisabledNoop: a zero budget (the --context-tokens 0
// disable path) never checks pressure and never calls the adapter.
func TestCompactionDisabledNoop(t *testing.T) {
	lg, _ := newTestLog(t)
	ad := &captureAdapter{}
	notes := &lines{}
	dec := newCompactingTurnRunner(&fakeTurnRunner{}, session.SessionConfig{}, notes.printf)

	runOneTurn(t, dec, lg, ad, testOpts, bigPrompt)

	starts, summaries, ends := compactionCounts(lg.Events())
	if starts+summaries+ends != 0 {
		t.Fatalf("disabled compaction events = %d/%d/%d, want none", starts, summaries, ends)
	}
	if got := ad.callCount(); got != 0 {
		t.Fatalf("disabled summarize calls = %d, want 0", got)
	}
}

// --- flag validation ----------------------------------------------------------

// TestValidateCompaction covers the fail-closed flag pair: the disable
// path, defaults, the inclusive [0,1] boundaries, and the refused
// nonsense values (including the non-finite ones a `<0 || >1` range
// check cannot see).
func TestValidateCompaction(t *testing.T) {
	if c, err := validateCompaction(0, 0.8); err != nil || c.ContextBudgetTokens != 0 {
		t.Fatalf("disable path = %+v err=%v, want zero config", c, err)
	}
	if c, err := validateCompaction(defaultContextTokens, 0); err != nil || c.ContextBudgetTokens != defaultContextTokens || c.PressureThreshold != 0 {
		t.Fatalf("default threshold = %+v err=%v", c, err)
	}
	if c, err := validateCompaction(1000, 0.9); err != nil || c.PressureThreshold != 0.9 {
		t.Fatalf("explicit threshold = %+v err=%v", c, err)
	}
	// Boundaries stay VALID: [0,1] is inclusive on both ends (0 means
	// "session default", 1 = compact only at the full budget).
	for _, boundary := range []float64{0, 1} {
		if c, err := validateCompaction(1000, boundary); err != nil || c.PressureThreshold != boundary {
			t.Fatalf("boundary threshold %f = %+v err=%v, want it admitted verbatim", boundary, c, err)
		}
	}
	for _, tc := range []struct {
		tokens    int
		threshold float64
		wantIn    string
	}{
		{-1, 0.8, "--context-tokens"},
		{1000, 1.5, "--compact-threshold"},
		{1000, -0.1, "--compact-threshold"},
		// Non-finite values need an EXPLICIT guard: NaN compares false
		// on both sides of `<0 || >1` and would sail through to become
		// PressureThreshold (±Inf only trip a side by accident). The
		// refusal must also fire on the documented DISABLE path — a
		// nonsense threshold never passes silent, budget or not.
		{1000, math.NaN(), "--compact-threshold"},
		{1000, math.Inf(1), "--compact-threshold"},
		{1000, math.Inf(-1), "--compact-threshold"},
		{0, math.NaN(), "--compact-threshold"},
	} {
		_, err := validateCompaction(tc.tokens, tc.threshold)
		if err == nil || !strings.Contains(err.Error(), tc.wantIn) {
			t.Fatalf("validateCompaction(%d, %f) err = %v, want it naming %s", tc.tokens, tc.threshold, err, tc.wantIn)
		}
	}
}

// --- replay + unfold over a real compacted log --------------------------------

// TestCompactedReplayDeterministicAndUnfold: the compacted log replays
// to the same surface the live log reports, and Unfold recovers the
// PRE-compaction surface from the summary's citations — the wired
// end-to-end proof of reversibility (slice-2 proved it at the library
// seam; this proves it over a log the decorator actually produced).
func TestCompactedReplayDeterministicAndUnfold(t *testing.T) {
	lg, path := newTestLog(t)
	ad := &captureAdapter{}
	dec := newCompactingTurnRunner(&fakeTurnRunner{}, session.SessionConfig{ContextBudgetTokens: 200}, nil)

	runOneTurn(t, dec, lg, ad, testOpts, bigPrompt)

	live, err := lg.Surface()
	if err != nil {
		t.Fatalf("live surface: %v", err)
	}
	events, err := session.ReplayFile(path)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	replayed, err := session.DeriveMessages(events)
	if err != nil {
		t.Fatalf("derive replayed: %v", err)
	}
	liveJSON, _ := json.Marshal(live)
	replayedJSON, _ := json.Marshal(replayed)
	if string(liveJSON) != string(replayedJSON) {
		t.Fatalf("replayed surface differs from live surface:\nlive:     %s\nreplayed: %s", liveJSON, replayedJSON)
	}

	// Unfold from the summary's citations == the pre-compaction surface.
	var summarySeq int64 = -1
	for _, e := range events {
		if e.Type == session.TypeCompactionSummary {
			summarySeq = e.Seq
		}
	}
	if summarySeq < 0 {
		t.Fatal("no compaction/summary in the compacted log")
	}
	unfolded, err := session.Unfold(events, summarySeq)
	if err != nil {
		t.Fatalf("unfold: %v", err)
	}
	if len(unfolded) != 4 || unfolded[0].Content != bigPrompt {
		t.Fatalf("unfolded = %d messages, head %q…, want the 4 pre-compaction messages headed by the big prompt", len(unfolded), first40(unfolded))
	}
}

func first40(msgs []session.Message) string {
	if len(msgs) == 0 || len(msgs[0].Content) < 40 {
		return ""
	}
	return msgs[0].Content[:40]
}

// --- the wired path: buildServer + protocol server ----------------------------

// compactionLLM serves the wired scenario: call 1 = the turn (two echo
// tool calls in one batch), call 2 = the summarize (short summary).
func compactionLLM(t *testing.T) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	calls := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "chatcmpl-c1", "object": "chat.completion", "model": "fake-model",
				"choices": []map[string]any{{
					"index": 0,
					"message": map[string]any{
						"role": "assistant", "content": "",
						"tool_calls": []map[string]any{
							{"id": "call-cm-1", "type": "function", "function": map[string]any{"name": "echo", "arguments": `{"text":"wired-one"}`}},
							{"id": "call-cm-2", "type": "function", "function": map[string]any{"name": "echo", "arguments": `{"text":"wired-two"}`}},
						},
					},
					"finish_reason": "tool_calls",
				}},
				"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 2, "total_tokens": 12},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-c2", "object": "chat.completion", "model": "fake-model",
			"choices": []map[string]any{{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": "wired summary: the echo probes ran and returned; the preamble is condensed."},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
		})
	}))
}

// TestCompactionWiredThroughServer is the full-wiring crux: flags →
// validateCompaction → buildServer (decorator armed) → the protocol
// handler's turn gate → the decorated turn → the compaction bracket on
// the wire-visible log, with the post-compaction surface observable
// over session/surface.
func TestCompactionWiredThroughServer(t *testing.T) {
	llm := compactionLLM(t)
	defer llm.Close()
	cfg := testConfig(t, "openai", llm.URL)
	// The P5 flag pair, armed: a tiny budget so one big turn crosses.
	cfg.Compaction = session.SessionConfig{ContextBudgetTokens: 300}
	logPath := filepath.Join(cfg.SessionDir, "sess-wired.jsonl")

	svc, cli := net.Pipe()
	srv, eng, _, _ := buildServer(cfg, "test-key", svc)
	if eng.TurnRunnerDecorator == nil {
		t.Fatal("buildServer did not arm the compaction decorator for a positive --context-tokens budget")
	}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(nil) }()
	client := protocol.NewClient(cli)
	defer func() {
		_ = client.Close()
		select {
		case <-served:
		case <-time.After(2 * time.Second):
			t.Errorf("server did not exit")
		}
	}()

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("call: %v", err)
		}
	}
	must(client.Call("initialize", map[string]any{"protocolVersion": 1}, nil))
	must(client.Call("session/create", map[string]any{"path": logPath, "sessionId": "sess-wired"}, nil))

	var turn struct {
		Results []tools.Result `json:"results"`
	}
	// One protocol turn: the fake model answers with two echo calls;
	// the turn commits, pressure crosses, the decorator compacts
	// BEFORE the response is written (the gate holds through Compact).
	must(client.Call("session/prompt", map[string]any{"text": bigPrompt}, &turn))
	if len(turn.Results) != 2 || turn.Results[0].IsError {
		t.Fatalf("wired turn results = %+v", turn.Results)
	}

	// The compaction bracket landed on the durable log.
	events, err := session.ReplayFile(logPath)
	if err != nil {
		t.Fatalf("replay wired log: %v", err)
	}
	starts, summaries, ends := compactionCounts(events)
	if starts != 1 || summaries != 1 || ends != 1 {
		t.Fatalf("wired log compaction events = %d/%d/%d, want 1/1/1", starts, summaries, ends)
	}

	// Post-compaction surface over the wire: [summary, r1, r2] (3 <
	// pre 4 — the head was shadowed), headed by the user-role summary.
	var surf struct {
		Messages []session.Message `json:"messages"`
	}
	must(client.Call("session/surface", nil, &surf))
	if len(surf.Messages) != 3 {
		t.Fatalf("wired post-compaction surface = %d messages, want 3", len(surf.Messages))
	}
	if surf.Messages[0].Role != "user" || !strings.HasPrefix(surf.Messages[0].Content, "wired summary:") {
		t.Fatalf("wired surface head = %+v, want the summary message", surf.Messages[0])
	}
}

// TestBuildServerCompactionDisabled: the zero-budget disable path arms
// no decorator (the bare pipeline, byte-identical turn behavior).
func TestBuildServerCompactionDisabled(t *testing.T) {
	cfg := testConfig(t, "openai", "http://x.test")
	if c, err := validateCompaction(0, 0.8); err != nil || c.ContextBudgetTokens != 0 {
		t.Fatalf("disable validation = %+v err=%v", c, err)
	}
	svc, cli := net.Pipe()
	defer cli.Close()
	_, eng, _, _ := buildServer(cfg, "test-key", svc)
	if eng.TurnRunnerDecorator != nil {
		t.Fatal("disable path must not arm a TurnRunnerDecorator")
	}
}
