// turn_retry_test.go — slice B1 retry-ladder unit tests at the RunTurn
// seam: flaky fail-fail-succeed, exhaustion, non-retryable immediacy,
// system-prompt prepend, and the disarmed-ladder byte-stability of the
// pre-B1 failure shapes. Backoff waits are recorded, never slept.
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
	"github.com/vhqtvn/vh-agent-harness/internal/loop"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// scriptStep is one scripted adapter-call outcome.
type scriptStep struct {
	resp *adapters.Response
	err  error
}

// scriptAdapter serves a fixed failure/success script, one step per call.
type scriptAdapter struct {
	name  string
	steps []scriptStep
	calls int
}

func (a *scriptAdapter) Name() string { return a.name }

func (a *scriptAdapter) Call(_ context.Context, req *adapters.Request) (*adapters.Response, error) {
	i := a.calls
	if i >= len(a.steps) {
		i = len(a.steps) - 1
	}
	a.calls++
	return a.steps[i].resp, a.steps[i].err
}

// recSleeper records backoff waits instead of sleeping.
type recSleeper struct {
	waits []time.Duration
}

func (s *recSleeper) Sleep(d time.Duration) { s.waits = append(s.waits, d) }

func okResp(content string) *adapters.Response {
	return &adapters.Response{Model: "retry-model", Content: content, FinishReason: "stop"}
}

// ladder arms the retry ladder with a recording sleeper and the fixed
// default jitter (0.5 → mid-band, deterministic backoffs).
func ladder(maxRetries int) *RetryLadder {
	return &RetryLadder{
		Config:  loop.RetryConfig{MaxRetries: maxRetries, BackoffInitial: 500 * time.Millisecond, BackoffMax: 10 * time.Second, JitterFraction: 0.10},
		Sleeper: &recSleeper{},
	}
}

// TestRunTurnRetryLadderFailFailSucceed is the B1 crux shape at the unit
// seam: a flaky adapter failing twice then succeeding must produce
// llm/retry BEFORE each wait and llm/retry-started after it, a FRESH
// llm/request per attempt, and exactly ONE turn bracket — with the log
// replaying byte-identically.
func TestRunTurnRetryLadderFailFailSucceed(t *testing.T) {
	var sink writeBuffer
	lg := fixedLog(t, "turn-retry-ok", &sink)
	p := NewPipeline()
	ad := &scriptAdapter{name: "flaky", steps: []scriptStep{
		{err: adapters.TransportError("flaky", errors.New("wire cut"))},
		{err: adapters.TransportError("flaky", errors.New("wire cut again"))},
		{resp: okResp("recovered")},
	}}
	l := ladder(2)
	report, err := p.RunTurn(context.Background(), lg, ad, TurnOptions{Model: "retry-model", Retry: l}, "flaky please")
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if report.Attempts != 3 || report.Response.Content != "recovered" {
		t.Fatalf("report = %+v, want 3 attempts + recovered", report)
	}
	assertEventSequence(t, lg, []string{
		session.TypeSessionHeader,
		session.TypeTurnBegin, session.TypeSessionPrompt,
		session.TypeLLMRequest,
		session.TypeLLMRetry, session.TypeLLMRetryStarted,
		session.TypeLLMRequest,
		session.TypeLLMRetry, session.TypeLLMRetryStarted,
		session.TypeLLMRequest,
		session.TypeLLMResponse,
		session.TypeTurnEnd,
	})
	// Exactly one bracket no matter the retry count.
	if countTypes(lg, session.TypeTurnBegin) != 1 || countTypes(lg, session.TypeTurnEnd) != 1 {
		t.Fatalf("turn bracket count = %d/%d, want 1/1", countTypes(lg, session.TypeTurnBegin), countTypes(lg, session.TypeTurnEnd))
	}
	// Numbered durable retry records with the policy snapshot and
	// deterministic mid-band backoffs (500ms, 1000ms).
	s := l.Sleeper.(*recSleeper)
	if len(s.waits) != 2 || s.waits[0] != 500*time.Millisecond || s.waits[1] != 1000*time.Millisecond {
		t.Fatalf("backoff waits = %v, want [500ms 1s]", s.waits)
	}
	var rp loop.RetryPayload
	if err := json.Unmarshal(lg.Events()[4].Payload, &rp); err != nil {
		t.Fatalf("llm/retry payload: %v", err)
	}
	if rp.Attempt != 1 || rp.ErrorClass != "transport" || rp.BackoffMs != 500 || rp.Policy.MaxRetries != 2 {
		t.Fatalf("llm/retry #1 = %+v", rp)
	}
	replayDeterminism(t, &sink, lg)
}

// TestRunTurnRetryLadderExhaustion drives the all-failures script: the
// budget spends (initial + 2 retries), the turn closes
// turn/end{kind:error}, and *loop.ExhaustedError surfaces — replay-stable.
func TestRunTurnRetryLadderExhaustion(t *testing.T) {
	var sink writeBuffer
	lg := fixedLog(t, "turn-retry-dead", &sink)
	p := NewPipeline()
	ad := &scriptAdapter{name: "dead", steps: []scriptStep{
		{err: adapters.TransportError("dead", errors.New("refused"))},
	}}
	l := ladder(2)
	_, err := p.RunTurn(context.Background(), lg, ad, TurnOptions{Model: "retry-model", Retry: l}, "try anyway")
	var ex *loop.ExhaustedError
	if !errors.As(err, &ex) {
		t.Fatalf("error = %v (%T), want *loop.ExhaustedError", err, err)
	}
	if ex.Attempts != 3 || ad.calls != 3 {
		t.Fatalf("exhaustion after %d attempts (adapter calls %d), want 3/3", ex.Attempts, ad.calls)
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
	if te.Kind != "error" || !bytes.Contains(last.Payload, []byte("refused")) {
		t.Fatalf("turn/end = %+v (%s)", te, last.Payload)
	}
	if countTypes(lg, session.TypeLLMRetry) != 2 || countTypes(lg, session.TypeLLMRetryStarted) != 2 || countTypes(lg, session.TypeLLMRequest) != 3 {
		t.Fatalf("retry ladder counts wrong: retries=%d started=%d requests=%d",
			countTypes(lg, session.TypeLLMRetry), countTypes(lg, session.TypeLLMRetryStarted), countTypes(lg, session.TypeLLMRequest))
	}
	replayDeterminism(t, &sink, lg)
}

// TestRunTurnRetryLadderNonRetryableImmediate proves a non-retryable
// class (auth) fails the turn IMMEDIATELY — no retry records, no
// exhaustion label even with budget unspent.
func TestRunTurnRetryLadderNonRetryableImmediate(t *testing.T) {
	var sink writeBuffer
	lg := fixedLog(t, "turn-retry-auth", &sink)
	p := NewPipeline()
	authErr := adapters.NewAdapterError("authy", adapters.KindOther, 401, 0, errors.New("invalid api key"))
	ad := &scriptAdapter{name: "authy", steps: []scriptStep{{err: authErr}}}
	_, err := p.RunTurn(context.Background(), lg, ad, TurnOptions{Model: "retry-model", Retry: ladder(2)}, "try")
	var ex *loop.ExhaustedError
	if errors.As(err, &ex) {
		t.Fatalf("non-retryable failure mislabeled as exhaustion: %v", err)
	}
	var aerr *adapters.AdapterError
	if !errors.As(err, &aerr) || aerr.Kind != adapters.KindOther {
		t.Fatalf("error = %v, want the typed non-retryable AdapterError", err)
	}
	if ad.calls != 1 {
		t.Fatalf("adapter calls = %d, want exactly 1 (no retries)", ad.calls)
	}
	assertEventSequence(t, lg, []string{
		session.TypeSessionHeader,
		session.TypeTurnBegin, session.TypeSessionPrompt,
		session.TypeLLMRequest,
		session.TypeTurnEnd,
	})
	replayDeterminism(t, &sink, lg)
}

// TestRunTurnSystemPromptPrepended proves the configured system prompt
// reaches the adapter as a leading role="system" message on EVERY
// attempt while never entering the session log.
func TestRunTurnSystemPromptPrepended(t *testing.T) {
	var sink writeBuffer
	lg := fixedLog(t, "turn-sys", &sink)
	p := NewPipeline()
	rec := &turnAdapter{resp: okResp("done")}
	if _, err := p.RunTurn(context.Background(), lg, rec, TurnOptions{Model: "m", System: "You are the daemon."}, "hi"); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if len(rec.saw) != 1 {
		t.Fatalf("adapter calls = %d", len(rec.saw))
	}
	msgs := rec.saw[0].Messages
	if len(msgs) != 2 || msgs[0].Role != "system" || msgs[0].Content != "You are the daemon." || msgs[1].Role != "user" {
		t.Fatalf("messages = %+v, want [system, user]", msgs)
	}
	if bytes.Contains(sink.data, []byte("You are the daemon.")) {
		t.Fatalf("system prompt leaked into the session log:\n%s", sink.data)
	}
	replayDeterminism(t, &sink, lg)
}

// TestRunTurnDisarmedLadderByteStable pins the pre-B1 failure shapes:
// with Retry nil, an untyped call error closes the turn with the RAW
// error text (no classification wrapping) and an empty response closes
// with "empty response" — exactly the pre-bis slice logs.
func TestRunTurnDisarmedLadderByteStable(t *testing.T) {
	var sink writeBuffer
	lg := fixedLog(t, "turn-legacy-err", &sink)
	p := NewPipeline()
	_, err := p.RunTurn(context.Background(), lg, &errAdapter{err: errString("provider down")}, TurnOptions{Model: "m"}, "try")
	if err == nil {
		t.Fatal("expected error")
	}
	events := lg.Events()
	var te session.TurnEndPayload
	if err := json.Unmarshal(events[len(events)-1].Payload, &te); err != nil {
		t.Fatalf("turn/end payload: %v", err)
	}
	if te.Reason != "provider down" {
		t.Fatalf("disarmed reason = %q, want the raw pre-B1 text", te.Reason)
	}

	var sink2 writeBuffer
	lg2 := fixedLog(t, "turn-legacy-empty", &sink2)
	_, _ = p.RunTurn(context.Background(), lg2, &scriptAdapter{name: "empty", steps: []scriptStep{{resp: &adapters.Response{Model: "m"}}}}, TurnOptions{Model: "m"}, "try")
	var te2 session.TurnEndPayload
	if err := json.Unmarshal(lg2.Events()[len(lg2.Events())-1].Payload, &te2); err != nil {
		t.Fatalf("turn/end payload: %v", err)
	}
	if te2.Reason != "empty response" {
		t.Fatalf("disarmed empty reason = %q, want %q", te2.Reason, "empty response")
	}
}

func countTypes(lg *session.Log, typ string) int {
	n := 0
	for _, ev := range lg.Events() {
		if ev.Type == typ {
			n++
		}
	}
	return n
}
