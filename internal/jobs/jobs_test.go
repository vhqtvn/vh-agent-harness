package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// --- test fixtures -----------------------------------------------------------

type testSink struct{ bytes.Buffer }

type quickExec struct {
	mu   sync.Mutex
	runs []Job
	err  error
}

func (e *quickExec) Run(_ context.Context, job Job) error {
	e.mu.Lock()
	e.runs = append(e.runs, job)
	e.mu.Unlock()
	return e.err
}

// blockingExec never returns from Run until the test releases it — the
// fixture that proves enqueue-receipt dispatch never blocks on execution.
type blockingExec struct {
	started chan<- string
	release <-chan struct{}
}

func (e *blockingExec) Run(_ context.Context, job Job) error {
	e.started <- job.ID
	<-e.release
	return nil
}

func testManager(t *testing.T, id string, exec Executor, opts Options) (*Manager, *testSink) {
	t.Helper()
	sink := &testSink{}
	lg, err := session.NewLog(sink, id, time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	m, err := NewManager(lg, exec, opts)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(m.Stop)
	return m, sink
}

func countEvents(lg *session.Log, typ string) int {
	n := 0
	for _, ev := range lg.Events() {
		if ev.Type == typ {
			n++
		}
	}
	return n
}

func lastPayload(t *testing.T, lg *session.Log, typ string) session.JobPayload {
	t.Helper()
	var last session.JobPayload
	found := false
	for _, ev := range lg.Events() {
		if ev.Type == typ {
			if err := json.Unmarshal(ev.Payload, &last); err != nil {
				t.Fatalf("unmarshal %s payload: %v", typ, err)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("no %s event in log", typ)
	}
	return last
}

// --- enqueue-receipt dispatch ------------------------------------------------

// TestDispatchReturnsImmediatelyWithBlockingExecutor proves the dsh
// enqueue-receipt contract: Dispatch validates, logs job/enqueued, and
// returns the receipt IMMEDIATELY — the prompt is an enqueue receipt only,
// never a blocking execution (dsh SDK pattern).
func TestDispatchReturnsImmediatelyWithBlockingExecutor(t *testing.T) {
	started := make(chan string, 4)
	release := make(chan struct{})
	m, _ := testManager(t, "sess-jobs-a", &blockingExec{started: started, release: release}, Options{})

	type receipt struct {
		r   Receipt
		err error
	}
	done := make(chan receipt, 1)
	go func() {
		r, err := m.Dispatch("background", json.RawMessage(`{"cmd":"sleep 30"}`))
		done <- receipt{r, err}
	}()

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("Dispatch: %v", got.err)
		}
		if got.r.JobID != "background-1" {
			t.Fatalf("receipt JobID = %q, want background-1", got.r.JobID)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Dispatch blocked on execution; enqueue-receipt contract violated")
	}

	// The enqueue is durable before the receipt returns.
	if n := countEvents(m.Log(), session.TypeJobEnqueued); n != 1 {
		t.Fatalf("job/enqueued count after Dispatch = %d, want 1", n)
	}
	p := lastPayload(t, m.Log(), session.TypeJobEnqueued)
	if p.Owner != "sess-jobs-a" {
		t.Fatalf("enqueued Owner = %q, want the session id (owner fencing record)", p.Owner)
	}

	// The job may start, but nothing settles until released.
	close(release)
	m.Drain()
	if n := countEvents(m.Log(), session.TypeJobSettled); n != 1 {
		t.Fatalf("job/settled count after drain = %d, want 1", n)
	}
}

// TestDispatchAssignsPerKindMonotonicIDs: `<kind>-N` ids are per-kind
// monotonic within the session (dsh jobs id scheme).
func TestDispatchAssignsPerKindMonotonicIDs(t *testing.T) {
	m, _ := testManager(t, "sess-jobs-b", &quickExec{}, Options{})
	for _, want := range []struct{ kind, id string }{
		{"background", "background-1"},
		{"background", "background-2"},
		{"digest", "digest-1"},
		{"background", "background-3"},
	} {
		r, err := m.Dispatch(want.kind, json.RawMessage(`{}`))
		if err != nil {
			t.Fatalf("Dispatch(%s): %v", want.kind, err)
		}
		if r.JobID != want.id {
			t.Fatalf("Dispatch(%s) JobID = %q, want %q", want.kind, r.JobID, want.id)
		}
	}
	m.Drain()
}

// TestDispatchValidatesKindAndPayload: dispatch validates before any event
// is logged — bad input leaves no trace in the log.
func TestDispatchValidatesKindAndPayload(t *testing.T) {
	m, _ := testManager(t, "sess-jobs-c", &quickExec{}, Options{})
	bad := []struct {
		kind    string
		payload json.RawMessage
	}{
		{"", json.RawMessage(`{}`)},
		{"Background", json.RawMessage(`{}`)},           // uppercase
		{"back ground", json.RawMessage(`{}`)},          // space
		{"background;", json.RawMessage(`{}`)},          // punctuation
		{"background", json.RawMessage(`{broken json`)}, // invalid payload JSON
	}
	for i, b := range bad {
		if _, err := m.Dispatch(b.kind, b.payload); err == nil {
			t.Fatalf("bad case %d (%q) accepted; want validation error", i, b.kind)
		}
	}
	if n := countEvents(m.Log(), session.TypeJobEnqueued); n != 0 {
		t.Fatalf("job/enqueued count after rejected dispatches = %d, want 0", n)
	}
}

// --- first-wins settlement -----------------------------------------------------

// TestFirstWinsSettlement: exactly one terminal job/settled event lands;
// a second settle attempt for the same job is a no-op (dsh first-wins
// settlement). Here the "manual" settle wins and the runner's later
// settle is suppressed.
func TestFirstWinsSettlement(t *testing.T) {
	started := make(chan string, 4)
	release := make(chan struct{})
	m, _ := testManager(t, "sess-jobs-d", &blockingExec{started: started, release: release}, Options{})

	r, err := m.Dispatch("background", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	select {
	case id := <-started:
		if id != r.JobID {
			t.Fatalf("started %q, want %q", id, r.JobID)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("executor never started the job")
	}

	manual := errors.New("manual settle first")
	if err := m.Settle(r.JobID, manual); err != nil {
		t.Fatalf("Settle: %v", err)
	}
	// Second settle attempt (completed) must be a logged-test-assertable no-op.
	if err := m.Settle(r.JobID, nil); err != nil {
		t.Fatalf("second Settle errored: %v", err)
	}

	close(release)
	m.Drain()

	if n := countEvents(m.Log(), session.TypeJobSettled); n != 1 {
		t.Fatalf("job/settled count = %d, want exactly 1 (first-wins)", n)
	}
	p := lastPayload(t, m.Log(), session.TypeJobSettled)
	if p.Result != session.JobResultFailed || p.Reason != "manual settle first" {
		t.Fatalf("settled payload = %+v, want failed/manual settle first (the FIRST settle wins)", p)
	}
}

// TestSettleCompletedAndFailedResults: executor error maps to
// {result: failed, reason: err}; nil maps to {result: completed}.
func TestSettleCompletedAndFailedResults(t *testing.T) {
	boom := errors.New("exit 1")
	m, _ := testManager(t, "sess-jobs-e", &quickExec{err: boom}, Options{})
	r1, err := m.Dispatch("background", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	m.Drain()
	p := lastPayload(t, m.Log(), session.TypeJobSettled)
	if p.JobID != r1.JobID || p.Result != session.JobResultFailed || p.Reason != "exit 1" {
		t.Fatalf("failed settle payload = %+v", p)
	}

	m2, _ := testManager(t, "sess-jobs-e2", &quickExec{}, Options{})
	if _, err := m2.Dispatch("background", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	m2.Drain()
	p2 := lastPayload(t, m2.Log(), session.TypeJobSettled)
	if p2.Result != session.JobResultCompleted || p2.Reason != "" {
		t.Fatalf("completed settle payload = %+v", p2)
	}
}

// --- reported-flag dedup -------------------------------------------------------

// TestReportEmittedOncePerSettledJob: job/report is emitted once per
// settled job when surfaced to the model; duplicate reports are
// suppressed (dsh `reported` flag as model-cost guard).
func TestReportEmittedOncePerSettledJob(t *testing.T) {
	m, _ := testManager(t, "sess-jobs-f", &quickExec{}, Options{})
	r, err := m.Dispatch("background", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	m.Drain()

	n, err := m.EmitReports()
	if err != nil {
		t.Fatalf("EmitReports: %v", err)
	}
	if n != 1 {
		t.Fatalf("first EmitReports emitted %d, want 1", n)
	}
	n2, err := m.EmitReports()
	if err != nil {
		t.Fatalf("second EmitReports: %v", err)
	}
	if n2 != 0 {
		t.Fatalf("second EmitReports emitted %d, want 0 (reported dedup)", n2)
	}
	if got := countEvents(m.Log(), session.TypeJobReport); got != 1 {
		t.Fatalf("job/report count = %d, want 1", got)
	}

	// The report is the notice the surface folds: one user-role message
	// identifying the job and its result.
	msgs, err := m.Log().Surface()
	if err != nil {
		t.Fatalf("Surface: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("surface length = %d, want 1 report notice", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Fatalf("report role = %q, want user", msgs[0].Role)
	}
	if !bytes.Contains([]byte(msgs[0].Content), []byte(r.JobID)) {
		t.Fatalf("report content %q does not name the job", msgs[0].Content)
	}
}

// TestEmitReportsSkipsUnsettledJobs: only settled jobs report.
func TestEmitReportsSkipsUnsettledJobs(t *testing.T) {
	started := make(chan string, 4)
	release := make(chan struct{})
	m, _ := testManager(t, "sess-jobs-g", &blockingExec{started: started, release: release}, Options{})

	if _, err := m.Dispatch("background", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	<-started // running, not settled
	n, err := m.EmitReports()
	if err != nil {
		t.Fatalf("EmitReports: %v", err)
	}
	if n != 0 {
		t.Fatalf("EmitReports emitted %d for an unsettled job, want 0", n)
	}
	close(release)
	m.Drain()
	n, err = m.EmitReports()
	if err != nil {
		t.Fatalf("EmitReports after settle: %v", err)
	}
	if n != 1 {
		t.Fatalf("EmitReports after settle emitted %d, want 1", n)
	}
}
