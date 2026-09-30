package tools

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// gateBody returns an execute body that records entry, signals `entered`,
// blocks until `proceed` is closed, and tracks observed concurrency.
type gateHarness struct {
	entered chan struct{}
	proceed chan struct{}
	cur     atomic.Int64
	maxObs  atomic.Int64
}

func newGateHarness() *gateHarness {
	return &gateHarness{entered: make(chan struct{}), proceed: make(chan struct{})}
}

func (g *gateHarness) body(context.Context, json.RawMessage) (string, error) {
	c := g.cur.Add(1)
	for {
		m := g.maxObs.Load()
		if c <= m || g.maxObs.CompareAndSwap(m, c) {
			break
		}
	}
	g.entered <- struct{}{}
	<-g.proceed
	g.cur.Add(-1)
	return "gated-ok", nil
}

func (g *gateHarness) release() { close(g.proceed) }

// probeNoEntry asserts nothing arrives on ch within d (a cap/barrier
// hold proof: a violating scheduler would admit the next body almost
// immediately).
func probeNoEntry(t *testing.T, ch <-chan struct{}, d time.Duration, what string) {
	t.Helper()
	select {
	case <-ch:
		t.Fatalf("%s admitted while it must be held back", what)
	case <-time.After(d):
	}
}

func newBatchLog(t *testing.T, id string) *session.Log {
	t.Helper()
	lg, err := session.NewLog(&writeBuffer{}, id, time.Date(2026, 8, 20, 11, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	return lg
}

func eventTypesOf(lg *session.Log) []string {
	evs := lg.Events()
	out := make([]string, len(evs))
	for i, ev := range evs {
		out[i] = ev.Type
	}
	return out
}

// --- bounded rolling pool --------------------------------------------------

func TestSchedulerCapEnforced(t *testing.T) {
	// 4 concurrency-safe calls under a cap of 2: exactly two bodies may
	// be in flight; the third+fourth stay pool-queued until slots free.
	h := newGateHarness()
	p := NewPipelineWithOptions(PipelineOptions{MaxParallel: 2})
	if err := p.Register(ToolDefinition{Name: "probe", IsConcurrencySafe: true, Execute: h.body}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	lg := newBatchLog(t, "sched-cap")
	calls := []session.ToolCall{
		{ID: "c1", Name: "probe"},
		{ID: "c2", Name: "probe"},
		{ID: "c3", Name: "probe"},
		{ID: "c4", Name: "probe"},
	}
	done := make(chan struct{})
	var results []Result
	var batchErr error
	go func() {
		results, batchErr = p.ExecuteBatchLogged(context.Background(), lg, calls)
		close(done)
	}()

	<-h.entered // body 1 in flight
	<-h.entered // body 2 in flight
	probeNoEntry(t, h.entered, 100*time.Millisecond, "a third body (cap=2)")
	if h.maxObs.Load() != 2 {
		t.Fatalf("observed concurrency = %d, want exactly 2", h.maxObs.Load())
	}

	h.release()
	<-h.entered // body 3
	<-h.entered // body 4
	<-done

	if batchErr != nil {
		t.Fatalf("ExecuteBatchLogged: %v", batchErr)
	}
	if len(results) != 4 {
		t.Fatalf("results = %d, want 4", len(results))
	}
	for i, res := range results {
		if res.IsError || res.CallID != calls[i].ID {
			t.Fatalf("results[%d] = %+v", i, res)
		}
	}
	if h.maxObs.Load() > 2 {
		t.Fatalf("cap violated: observed concurrency %d > 2", h.maxObs.Load())
	}
}

// --- non-safe barriers ------------------------------------------------------

func TestSchedulerNonSafeCallWaitsForPoolDrain(t *testing.T) {
	// [safe(gated), non-safe(gated)]: the non-safe call is a barrier — it
	// must not start while the safe call is in flight, and it runs alone.
	sh := newGateHarness()
	xh := newGateHarness()
	p := NewPipelineWithOptions(PipelineOptions{MaxParallel: 3})
	if err := p.Register(ToolDefinition{Name: "safe_probe", IsConcurrencySafe: true, Execute: sh.body}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := p.Register(ToolDefinition{Name: "solo_probe", Execute: xh.body}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	lg := newBatchLog(t, "sched-barrier1")
	done := make(chan struct{})
	go func() {
		_, _ = p.ExecuteBatchLogged(context.Background(), lg, []session.ToolCall{
			{ID: "c1", Name: "safe_probe"},
			{ID: "c2", Name: "solo_probe"},
		})
		close(done)
	}()

	<-sh.entered
	probeNoEntry(t, xh.entered, 100*time.Millisecond, "the non-safe call (pool not drained)")
	sh.release()
	<-xh.entered // barrier passed: solo call starts alone
	xh.release()
	<-done
}

func TestSchedulerNonSafeCallBlocksFollowingPool(t *testing.T) {
	// [non-safe(gated), safe(gated)]: nothing after the barrier starts
	// until the barrier call completes.
	xh := newGateHarness()
	sh := newGateHarness()
	p := NewPipelineWithOptions(PipelineOptions{MaxParallel: 3})
	if err := p.Register(ToolDefinition{Name: "solo_probe", Execute: xh.body}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := p.Register(ToolDefinition{Name: "safe_probe", IsConcurrencySafe: true, Execute: sh.body}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	lg := newBatchLog(t, "sched-barrier2")
	done := make(chan struct{})
	go func() {
		_, _ = p.ExecuteBatchLogged(context.Background(), lg, []session.ToolCall{
			{ID: "c1", Name: "solo_probe"},
			{ID: "c2", Name: "safe_probe"},
		})
		close(done)
	}()

	<-xh.entered
	probeNoEntry(t, sh.entered, 100*time.Millisecond, "the trailing safe call (barrier in flight)")
	xh.release()
	<-sh.entered
	sh.release()
	<-done
}

func TestSchedulerTwoNonSafeCallsSerialize(t *testing.T) {
	x1 := newGateHarness()
	x2 := newGateHarness()
	bodies := map[string]func(context.Context, json.RawMessage) (string, error){
		"first_solo":  x1.body,
		"second_solo": x2.body,
	}
	p := NewPipeline()
	for name, body := range bodies {
		if err := p.Register(ToolDefinition{Name: name, Execute: body}); err != nil {
			t.Fatalf("Register: %v", err)
		}
	}
	lg := newBatchLog(t, "sched-serialize")
	done := make(chan struct{})
	go func() {
		_, _ = p.ExecuteBatchLogged(context.Background(), lg, []session.ToolCall{
			{ID: "c1", Name: "first_solo"},
			{ID: "c2", Name: "second_solo"},
		})
		close(done)
	}()

	<-x1.entered
	probeNoEntry(t, x2.entered, 100*time.Millisecond, "the second non-safe call")
	x1.release()
	<-x2.entered
	x2.release()
	<-done
}

// --- model-ordered commits ---------------------------------------------------

func TestBatchLogsAllIntentsBeforeAnyExecution(t *testing.T) {
	h := newGateHarness()
	p := NewPipelineWithOptions(PipelineOptions{MaxParallel: 2})
	if err := p.Register(ToolDefinition{Name: "probe", IsConcurrencySafe: true, Execute: h.body}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	lg := newBatchLog(t, "sched-intents")
	done := make(chan struct{})
	go func() {
		_, _ = p.ExecuteBatchLogged(context.Background(), lg, []session.ToolCall{
			{ID: "c1", Name: "probe"},
			{ID: "c2", Name: "probe"},
		})
		close(done)
	}()

	<-h.entered
	<-h.entered // both bodies dispatched
	types := eventTypesOf(lg)
	// header + 2 tool/call, and NO tool/result yet: intents precede all
	// execution; results come after bodies complete.
	want := []string{session.TypeSessionHeader, session.TypeToolCall, session.TypeToolCall}
	if len(types) != len(want) {
		t.Fatalf("events while executing = %v, want %v", types, want)
	}
	for i := range want {
		if types[i] != want[i] {
			t.Fatalf("events[%d] = %s, want %s (all = %v)", i, types[i], want[i], types)
		}
	}
	h.release()
	<-done
}

func TestModelOrderedResultsUnderInterleavedCompletion(t *testing.T) {
	// Model order [slow(safe, gated), fast(safe, immediate)]: fast
	// COMPLETES while slow is still gated, but tool/result for slow must
	// be appended FIRST — the log never depends on execution timing.
	slowH := newGateHarness()
	fastDone := make(chan struct{})
	p := NewPipelineWithOptions(PipelineOptions{MaxParallel: 2})
	if err := p.Register(ToolDefinition{Name: "slow", IsConcurrencySafe: true, Execute: slowH.body}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := p.Register(ToolDefinition{Name: "fast", IsConcurrencySafe: true, Execute: func(context.Context, json.RawMessage) (string, error) {
		defer close(fastDone)
		return "fast-ok", nil
	}}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	lg := newBatchLog(t, "sched-order")
	calls := []session.ToolCall{
		{ID: "c1", Name: "slow"},
		{ID: "c2", Name: "fast"},
	}
	done := make(chan struct{})
	var results []Result
	go func() {
		results, _ = p.ExecuteBatchLogged(context.Background(), lg, calls)
		close(done)
	}()

	<-slowH.entered
	<-fastDone // fast finished while slow is still gated
	if len(eventTypesOf(lg)) != 3 {
		t.Fatalf("no result may be committed out of model order: %v", eventTypesOf(lg))
	}
	slowH.release()
	<-done

	types := eventTypesOf(lg)
	want := []string{
		session.TypeSessionHeader,
		session.TypeToolCall, session.TypeToolCall,
		session.TypeToolResult, session.TypeToolResult,
	}
	if len(types) != len(want) {
		t.Fatalf("final events = %v", types)
	}
	for i := range want {
		if types[i] != want[i] {
			t.Fatalf("events[%d] = %s, want %s", i, types[i], want[i])
		}
	}
	// Result order: slow's result carries c1 and precedes fast's.
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
	if len(trs) != 2 || trs[0].CallID != "c1" || trs[0].Content != "gated-ok" || trs[1].CallID != "c2" || trs[1].Content != "fast-ok" {
		t.Fatalf("model-ordered results = %+v", trs)
	}
	if len(results) != 2 || results[0].CallID != "c1" || results[1].CallID != "c2" {
		t.Fatalf("returned results = %+v", results)
	}
}
