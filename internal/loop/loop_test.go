package loop

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// --- fakes ---------------------------------------------------------------

type fakeSleeper struct{ sleeps []time.Duration }

func (f *fakeSleeper) Sleep(d time.Duration) { f.sleeps = append(f.sleeps, d) }

type callResult struct {
	resp *adapters.Response
	err  error
}

// scriptedAdapter returns its script entries in order (last one repeats) and
// records every request it saw, so tests can assert each retry re-derived a
// fresh surface from the durable log.
type scriptedAdapter struct {
	calls     int
	script    []callResult
	saw       []*adapters.Request
	probeFail func(n int) error // optional extra failure injection per call
}

func (a *scriptedAdapter) Name() string { return "scripted" }

func (a *scriptedAdapter) Call(_ context.Context, req *adapters.Request) (*adapters.Response, error) {
	a.calls++
	cp := *req
	a.saw = append(a.saw, &cp)
	if a.probeFail != nil {
		if err := a.probeFail(a.calls); err != nil {
			return nil, err
		}
	}
	i := a.calls - 1
	if i >= len(a.script) {
		i = len(a.script) - 1
	}
	r := a.script[i]
	if r.err != nil {
		return nil, r.err
	}
	rc := *r.resp
	return &rc, nil
}

type memBuffer struct{ data []byte }

func (w *memBuffer) Write(p []byte) (int, error) {
	w.data = append(w.data, p...)
	return len(p), nil
}

func newLoopLog(t *testing.T, id string) *session.Log {
	t.Helper()
	lg, err := session.NewLog(&memBuffer{}, id, time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	t.Cleanup(func() { _ = lg.Close() })
	return lg
}

// fixedRand pins the jitter factor to exactly 1.0 so recorded sleeps equal
// the policy's base backoff.
func fixedRand() float64 { return 0.5 }

func okResponse(content string) *adapters.Response {
	return &adapters.Response{Model: "mock-1", Content: content, FinishReason: "stop"}
}

func eventTypes(lg *session.Log) []string {
	evs := lg.Events()
	out := make([]string, len(evs))
	for i, ev := range evs {
		out[i] = ev.Type
	}
	return out
}

func countType(lg *session.Log, typ string) int {
	n := 0
	for _, ev := range lg.Events() {
		if ev.Type == typ {
			n++
		}
	}
	return n
}

func decodeRetryPayload(t *testing.T, lg *session.Log) []RetryPayload {
	t.Helper()
	var out []RetryPayload
	for _, ev := range lg.Events() {
		if ev.Type != session.TypeLLMRetry {
			continue
		}
		var rp RetryPayload
		if err := json.Unmarshal(ev.Payload, &rp); err != nil {
			t.Fatalf("llm/retry payload: %v", err)
		}
		out = append(out, rp)
	}
	return out
}

func lastTurnEnd(t *testing.T, lg *session.Log) session.TurnEndPayload {
	t.Helper()
	var te session.TurnEndPayload
	found := false
	for _, ev := range lg.Events() {
		if ev.Type == session.TypeTurnEnd {
			if err := json.Unmarshal(ev.Payload, &te); err != nil {
				t.Fatalf("turn/end payload: %v", err)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("no turn/end event found")
	}
	return te
}

func seqOfFirst(t *testing.T, lg *session.Log, typ string) int64 {
	t.Helper()
	for _, ev := range lg.Events() {
		if ev.Type == typ {
			return ev.Seq
		}
	}
	t.Fatalf("no %s event found", typ)
	return 0
}

// --- driver scenarios ------------------------------------------------------

func TestRunHappyPath(t *testing.T) {
	lg := newLoopLog(t, "sess-happy")
	ad := &scriptedAdapter{script: []callResult{{resp: okResponse("hi there")}}}
	sl := &fakeSleeper{}
	d, err := NewDriver(lg, ad, Options{Model: "mock-1"}, sl, fixedRand)
	if err != nil {
		t.Fatalf("NewDriver: %v", err)
	}
	res, err := d.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Response.Content != "hi there" || res.Attempts != 1 {
		t.Fatalf("res = %+v, want content 'hi there' after 1 attempt", res)
	}
	want := []string{session.TypeSessionHeader, session.TypeTurnBegin, session.TypeSessionPrompt, session.TypeLLMResponse, session.TypeTurnEnd}
	got := eventTypes(lg)
	if len(got) != len(want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("events = %v, want %v", got, want)
		}
	}
	if te := lastTurnEnd(t, lg); te.Kind != "" {
		t.Fatalf("happy turn/end kind = %q, want empty", te.Kind)
	}
	msgs, err := lg.Surface()
	if err != nil {
		t.Fatalf("Surface: %v", err)
	}
	if len(msgs) != 2 || msgs[0].Content != "hello" || msgs[1].Role != "assistant" || msgs[1].Content != "hi there" {
		t.Fatalf("surface = %+v", msgs)
	}
	if len(sl.sleeps) != 0 {
		t.Fatalf("happy path must not sleep, got %v", sl.sleeps)
	}
	if len(ad.saw) != 1 || len(ad.saw[0].Messages) != 1 || ad.saw[0].Messages[0].Content != "hello" {
		t.Fatalf("adapter requests = %+v", ad.saw)
	}
}

func TestRunRetryThenSuccessNumberedTurnsAndReplay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "retry.jsonl")
	lg, err := session.OpenFile(path, "sess-retry")
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer lg.Close()

	ad := &scriptedAdapter{script: []callResult{
		{err: adapters.HTTPStatusError("scripted", 503, 0, "upstream sad")},
		{resp: okResponse("recovered")},
	}}
	sl := &fakeSleeper{}
	d, err := NewDriver(lg, ad, Options{Model: "mock-1"}, sl, fixedRand)
	if err != nil {
		t.Fatalf("NewDriver: %v", err)
	}
	res, err := d.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Attempts != 2 || res.Response.Content != "recovered" {
		t.Fatalf("res = %+v, want success on attempt 2", res)
	}

	// Durable numbering: llm/retry BEFORE llm/retry-started, both before the
	// response; the backoff wait sits between them.
	if got := countType(lg, session.TypeLLMRetry); got != 1 {
		t.Fatalf("llm/retry count = %d, want 1", got)
	}
	if got := countType(lg, session.TypeLLMRetryStarted); got != 1 {
		t.Fatalf("llm/retry-started count = %d, want 1", got)
	}
	rp := decodeRetryPayload(t, lg)[0]
	if rp.Attempt != 1 {
		t.Fatalf("retry attempt = %d, want 1", rp.Attempt)
	}
	if rp.ErrorClass != string(adapters.KindHTTP5xx) {
		t.Fatalf("retry errorClass = %q, want %q", rp.ErrorClass, adapters.KindHTTP5xx)
	}
	if rp.BackoffMs != 500 {
		t.Fatalf("retry backoffMs = %d, want 500", rp.BackoffMs)
	}
	if rp.Policy.MaxRetries != 2 || rp.Policy.BackoffInitialMs != 500 || rp.Policy.BackoffMaxMs != 10000 || rp.Policy.JitterFraction != 0.10 {
		t.Fatalf("retry policy snapshot = %+v", rp.Policy)
	}
	retrySeq := seqOfFirst(t, lg, session.TypeLLMRetry)
	startedSeq := seqOfFirst(t, lg, session.TypeLLMRetryStarted)
	if retrySeq >= startedSeq {
		t.Fatalf("llm/retry (seq %d) must precede llm/retry-started (seq %d)", retrySeq, startedSeq)
	}
	if startedSeq >= seqOfFirst(t, lg, session.TypeLLMResponse) {
		t.Fatal("llm/retry-started must precede the successful llm/response")
	}

	// The sleeper waited the policy backoff between the durable retry record
	// and the retry-started record.
	if len(sl.sleeps) != 1 || sl.sleeps[0] != 500*time.Millisecond {
		t.Fatalf("sleeps = %v, want [500ms]", sl.sleeps)
	}

	// Each retry re-derived the surface from the durable log.
	if len(ad.saw) != 2 {
		t.Fatalf("adapter calls = %d, want 2", len(ad.saw))
	}
	if len(ad.saw[1].Messages) != 1 || ad.saw[1].Messages[0].Content != "hello" {
		t.Fatalf("second request surface = %+v", ad.saw[1].Messages)
	}
	if te := lastTurnEnd(t, lg); te.Kind != "" {
		t.Fatalf("recovered turn/end kind = %q, want empty", te.Kind)
	}

	// Replay shows the same numbered attempts and the same surface.
	liveMsgs, err := lg.Surface()
	if err != nil {
		t.Fatalf("Surface: %v", err)
	}
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	replayed, err := session.ReplayFile(path)
	if err != nil {
		t.Fatalf("ReplayFile: %v", err)
	}
	nRetry, nStarted := 0, 0
	var rSeq, sSeq int64
	for _, ev := range replayed {
		switch ev.Type {
		case session.TypeLLMRetry:
			nRetry++
			rSeq = ev.Seq
		case session.TypeLLMRetryStarted:
			nStarted++
			sSeq = ev.Seq
		}
	}
	if nRetry != 1 || nStarted != 1 || rSeq >= sSeq {
		t.Fatalf("replayed retry records: retry=%d started=%d rSeq=%d sSeq=%d", nRetry, nStarted, rSeq, sSeq)
	}
	replayMsgs, err := session.DeriveMessages(replayed)
	if err != nil {
		t.Fatalf("DeriveMessages(replayed): %v", err)
	}
	lj, _ := json.Marshal(liveMsgs)
	rj, _ := json.Marshal(replayMsgs)
	if !bytes.Equal(lj, rj) {
		t.Fatalf("replay drift:\nlive:   %s\nreplay: %s", lj, rj)
	}
}

func TestRunExhaustion(t *testing.T) {
	lg := newLoopLog(t, "sess-exhaust")
	ad := &scriptedAdapter{script: []callResult{
		{err: adapters.HTTPStatusError("scripted", 500, 0, "boom 1")},
		{err: adapters.HTTPStatusError("scripted", 500, 0, "boom 2")},
		{err: adapters.HTTPStatusError("scripted", 500, 0, "boom 3")},
	}}
	sl := &fakeSleeper{}
	d, err := NewDriver(lg, ad, Options{Model: "mock-1"}, sl, fixedRand)
	if err != nil {
		t.Fatalf("NewDriver: %v", err)
	}
	res, err := d.Run(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected exhaustion error")
	}
	if res != nil {
		t.Fatalf("res = %+v, want nil on exhaustion", res)
	}
	var ex *ExhaustedError
	if !errors.As(err, &ex) {
		t.Fatalf("expected *ExhaustedError, got %T: %v", err, err)
	}
	if ex.Attempts != 3 { // initial + 2 retries (default maxRetries)
		t.Fatalf("exhausted after %d attempts, want 3", ex.Attempts)
	}
	var aerr *adapters.AdapterError
	if !errors.As(err, &aerr) || aerr.Kind != adapters.KindHTTP5xx {
		t.Fatalf("exhaustion must wrap the last adapter error, got %T", aerr)
	}
	if ad.calls != 3 {
		t.Fatalf("adapter calls = %d, want 3", ad.calls)
	}
	if got := countType(lg, session.TypeLLMRetry); got != 2 {
		t.Fatalf("llm/retry count = %d, want 2", got)
	}
	rps := decodeRetryPayload(t, lg)
	if len(rps) != 2 || rps[0].Attempt != 1 || rps[1].Attempt != 2 {
		t.Fatalf("retry attempts recorded = %+v, want numbered 1,2", rps)
	}
	if got := countType(lg, session.TypeLLMResponse); got != 0 {
		t.Fatalf("no llm/response may land on exhaustion, got %d", got)
	}
	if te := lastTurnEnd(t, lg); te.Kind != TurnEndKindError {
		t.Fatalf("exhausted turn/end kind = %q, want %q", te.Kind, TurnEndKindError)
	}
	want := []time.Duration{500 * time.Millisecond, time.Second}
	if len(sl.sleeps) != 2 || sl.sleeps[0] != want[0] || sl.sleeps[1] != want[1] {
		t.Fatalf("sleeps = %v, want %v", sl.sleeps, want)
	}
	msgs, err := lg.Surface()
	if err != nil {
		t.Fatalf("Surface: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Content != "hello" {
		t.Fatalf("exhausted surface = %+v, want the prompt only", msgs)
	}
}

func TestRunNonRetryableImmediateFail(t *testing.T) {
	lg := newLoopLog(t, "sess-nonretry")
	ad := &scriptedAdapter{script: []callResult{
		{err: adapters.HTTPStatusError("scripted", 400, 0, "bad request")},
	}}
	sl := &fakeSleeper{}
	d, err := NewDriver(lg, ad, Options{Model: "mock-1"}, sl, fixedRand)
	if err != nil {
		t.Fatalf("NewDriver: %v", err)
	}
	res, err := d.Run(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected non-retryable error")
	}
	if res != nil {
		t.Fatalf("res = %+v, want nil", res)
	}
	var aerr *adapters.AdapterError
	if !errors.As(err, &aerr) || aerr.Retryable() {
		t.Fatalf("expected the non-retryable AdapterError, got %T: %v", err, err)
	}
	if ad.calls != 1 {
		t.Fatalf("adapter calls = %d, want exactly 1 (no retries)", ad.calls)
	}
	if got := countType(lg, session.TypeLLMRetry); got != 0 {
		t.Fatalf("non-retryable failure must write no retry events, got %d", got)
	}
	if got := countType(lg, session.TypeLLMRetryStarted); got != 0 {
		t.Fatalf("non-retryable failure must write no retry-started events, got %d", got)
	}
	if len(sl.sleeps) != 0 {
		t.Fatalf("non-retryable failure must not sleep, got %v", sl.sleeps)
	}
	if te := lastTurnEnd(t, lg); te.Kind != TurnEndKindError {
		t.Fatalf("failed turn/end kind = %q, want %q", te.Kind, TurnEndKindError)
	}
}

func TestRunEmptyResponseClassifiedRetryable(t *testing.T) {
	lg := newLoopLog(t, "sess-empty")
	ad := &scriptedAdapter{script: []callResult{
		{resp: &adapters.Response{Model: "mock-1", Content: "", FinishReason: "stop"}},
		{resp: okResponse("second try works")},
	}}
	sl := &fakeSleeper{}
	d, err := NewDriver(lg, ad, Options{Model: "mock-1"}, sl, fixedRand)
	if err != nil {
		t.Fatalf("NewDriver: %v", err)
	}
	res, err := d.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Attempts != 2 || res.Response.Content != "second try works" {
		t.Fatalf("res = %+v, want recovery on attempt 2", res)
	}
	rps := decodeRetryPayload(t, lg)
	if len(rps) != 1 || rps[0].ErrorClass != string(adapters.KindEmptyResponse) {
		t.Fatalf("retry records = %+v, want one empty-response classification", rps)
	}
	if got := countType(lg, session.TypeLLMResponse); got != 1 {
		t.Fatalf("only the final non-empty response may land, got %d", got)
	}
}

func TestRunRateLimitRetryAfterHonored(t *testing.T) {
	lg := newLoopLog(t, "sess-ratelimit")
	ad := &scriptedAdapter{script: []callResult{
		{err: adapters.HTTPStatusError("scripted", 429, 2000, "slow down")},
		{resp: okResponse("after cooldown")},
	}}
	sl := &fakeSleeper{}
	d, err := NewDriver(lg, ad, Options{Model: "mock-1"}, sl, fixedRand)
	if err != nil {
		t.Fatalf("NewDriver: %v", err)
	}
	if _, err := d.Run(context.Background(), "hello"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(sl.sleeps) != 1 || sl.sleeps[0] != 2*time.Second {
		t.Fatalf("sleeps = %v, want [2s] from providerRetryAfterMs", sl.sleeps)
	}
	rps := decodeRetryPayload(t, lg)
	if len(rps) != 1 || rps[0].ErrorClass != string(adapters.KindRateLimit) || rps[0].RetryAfterMs != 2000 {
		t.Fatalf("retry records = %+v, want a rate-limit record citing retryAfterMs=2000", rps)
	}
}

func TestRunWithoutAdapterErrorClassifiesTransport(t *testing.T) {
	lg := newLoopLog(t, "sess-plainerr")
	ad := &scriptedAdapter{script: []callResult{
		{err: errors.New("plain wire hiccup")},
		{resp: okResponse("recovered")},
	}}
	sl := &fakeSleeper{}
	d, err := NewDriver(lg, ad, Options{Model: "mock-1"}, sl, fixedRand)
	if err != nil {
		t.Fatalf("NewDriver: %v", err)
	}
	if _, err := d.Run(context.Background(), "hello"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	rps := decodeRetryPayload(t, lg)
	if len(rps) != 1 || rps[0].ErrorClass != string(adapters.KindTransport) {
		t.Fatalf("retry records = %+v, want one transport classification for an untyped error", rps)
	}
}

func TestNewDriverValidatesPolicy(t *testing.T) {
	lg := newLoopLog(t, "sess-badcfg")
	if _, err := NewDriver(lg, &scriptedAdapter{}, Options{Retry: RetryConfig{MaxRetries: -3}}, &fakeSleeper{}, fixedRand); err == nil {
		t.Fatal("expected invalid retry config to fail driver construction")
	}
	if _, err := NewDriver(nil, &scriptedAdapter{}, Options{}, &fakeSleeper{}, fixedRand); err == nil {
		t.Fatal("expected nil log to fail driver construction")
	}
	if _, err := NewDriver(lg, nil, Options{}, &fakeSleeper{}, fixedRand); err == nil {
		t.Fatal("expected nil adapter to fail driver construction")
	}
	if _, err := NewDriver(lg, &scriptedAdapter{}, Options{}, nil, fixedRand); err == nil {
		t.Fatal("expected nil sleeper to fail driver construction")
	}
}

// TestCruxCompactedAndRetriedReplayUnfold is the slice-2 acceptance crux:
// one durable session that (1) survived a retried turn, (2) was compacted
// under the pressure policy, and (3) ran another retried turn afterwards —
// replay must reproduce the post-compaction surface exactly (and the
// replaceGeneration), and unfold must re-derive the pre-compaction surface
// from the cited source events.
func TestCruxCompactedAndRetriedReplayUnfold(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crux.jsonl")
	lg, err := session.OpenFile(path, "sess-crux")
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer lg.Close()

	ad := &scriptedAdapter{script: []callResult{
		{err: adapters.HTTPStatusError("scripted", 503, 0, "first try fails")},
		{resp: okResponse("first answer")},
	}}
	sl := &fakeSleeper{}
	d, err := NewDriver(lg, ad, Options{Model: "mock-1"}, sl, fixedRand)
	if err != nil {
		t.Fatalf("NewDriver: %v", err)
	}
	if _, err := d.Run(context.Background(), "hello engine"); err != nil {
		t.Fatalf("Run 1: %v", err)
	}

	// Grow pressure past the threshold with a large prompt, let the policy
	// fire, then compact.
	big := make([]byte, 4800)
	for i := range big {
		big[i] = 'x'
	}
	if _, err := lg.AppendPrompt(string(big)); err != nil {
		t.Fatalf("AppendPrompt: %v", err)
	}
	cfg := session.SessionConfig{ContextBudgetTokens: 1000}
	fire, pressure, err := session.ShouldCompact(lg.Events(), cfg)
	if err != nil {
		t.Fatalf("ShouldCompact: %v", err)
	}
	if !fire {
		t.Fatalf("pressure policy must fire before compaction, got %f", pressure)
	}
	preCompaction, err := lg.Surface()
	if err != nil {
		t.Fatalf("Surface: %v", err)
	}
	summarizer := func(sh []session.Message) (string, error) {
		out := "[compact:"
		for i, m := range sh {
			if i > 0 {
				out += "|"
			}
			c := m.Content
			if len(c) > 3 {
				c = c[:3]
			}
			out += c
		}
		return out + "]", nil
	}
	res, err := lg.Compact(summarizer, session.CompactOptions{Reason: "crux", Pressure: pressure, RetainTailMessages: 1})
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}

	// Second retried turn runs on the compacted surface (rate-limited once).
	ad2 := &scriptedAdapter{script: []callResult{
		{err: adapters.HTTPStatusError("scripted", 429, 1500, "cool down")},
		{resp: okResponse("second answer")},
	}}
	d2, err := NewDriver(lg, ad2, Options{Model: "mock-1"}, sl, fixedRand)
	if err != nil {
		t.Fatalf("NewDriver: %v", err)
	}
	if _, err := d2.Run(context.Background(), "after compaction"); err != nil {
		t.Fatalf("Run 2: %v", err)
	}
	// The post-compaction request surface the adapter saw starts with the
	// summary message, proving the retry consumed the compacted surface.
	lastSaw := ad2.saw[len(ad2.saw)-1].Messages
	if len(lastSaw) == 0 || lastSaw[0].Content != "[compact:hel|fir]" {
		t.Fatalf("run-2 surface must start with the compaction summary, got %+v", lastSaw)
	}

	liveMsgs, err := lg.Surface()
	if err != nil {
		t.Fatalf("Surface: %v", err)
	}
	liveGen, err := lg.ReplaceGeneration()
	if err != nil {
		t.Fatalf("ReplaceGeneration: %v", err)
	}
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	replayed, err := session.ReplayFile(path)
	if err != nil {
		t.Fatalf("ReplayFile: %v", err)
	}
	replayMsgs, err := session.DeriveMessages(replayed)
	if err != nil {
		t.Fatalf("DeriveMessages(replayed): %v", err)
	}
	lj, _ := json.Marshal(liveMsgs)
	rj, _ := json.Marshal(replayMsgs)
	if !bytes.Equal(lj, rj) {
		t.Fatalf("crux: replay must reproduce the post-compaction surface:\nlive:   %s\nreplay: %s", lj, rj)
	}
	fold, err := session.FoldSurface(replayed)
	if err != nil {
		t.Fatalf("FoldSurface(replayed): %v", err)
	}
	if fold.ReplaceGeneration != liveGen || liveGen != 1 {
		t.Fatalf("crux: replay generation = %d, live = %d, want both 1", fold.ReplaceGeneration, liveGen)
	}

	unfolded, err := session.Unfold(replayed, res.SummarySeq)
	if err != nil {
		t.Fatalf("Unfold: %v", err)
	}
	pj, _ := json.Marshal(preCompaction)
	uj, _ := json.Marshal(unfolded)
	if !bytes.Equal(pj, uj) {
		t.Fatalf("crux: unfold must reproduce the pre-compaction surface:\npre:      %s\nunfolded: %s", pj, uj)
	}

	// The replayed log shows both numbered retry rounds.
	retries := 0
	for _, ev := range replayed {
		if ev.Type == session.TypeLLMRetry {
			retries++
		}
	}
	if retries != 2 {
		t.Fatalf("replayed log must carry both numbered retries, got %d", retries)
	}
}
