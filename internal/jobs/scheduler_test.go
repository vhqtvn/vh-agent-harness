package jobs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// --- fixtures -----------------------------------------------------------------

// manualClock is the deterministic injected Clock: Now is a test-held
// instant; After registers a waiter that fires when Advance moves now
// past its deadline. Single-test-goroutine use (Advance) keeps firing
// order deterministic.
type manualClock struct {
	mu      sync.Mutex
	now     time.Time
	waiters []clockWaiter
}

type clockWaiter struct {
	deadline time.Time
	ch       chan time.Time
}

func newManualClock(start time.Time) *manualClock {
	return &manualClock{now: start.UTC()}
}

func (c *manualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *manualClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan time.Time, 1)
	c.waiters = append(c.waiters, clockWaiter{deadline: c.now.Add(d), ch: ch})
	return ch
}

// Advance moves now to to (monotonic: backwards moves are ignored) and
// fires every waiter whose deadline is ≤ to, sending its deadline.
func (c *manualClock) Advance(to time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if to.After(c.now) {
		c.now = to.UTC()
	}
	kept := c.waiters[:0]
	for _, w := range c.waiters {
		if !w.deadline.After(c.now) {
			w.ch <- w.deadline // buffered cap 1: never blocks
		} else {
			kept = append(kept, w)
		}
	}
	c.waiters = kept
}

// waiterCount reports how many After waiters are currently registered —
// the test-side probe for "the loop has armed its ticker", so an
// Advance cannot race ahead of registration (lost wakeup).
func (c *manualClock) waiterCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.waiters)
}

// schedEnv bundles the real-manager fixture: a file-backed session log,
// a real Manager over the given Executor, a manual clock, and a state
// path in the test's TempDir.
type schedEnv struct {
	m         *Manager
	lg        *session.Log
	logPath   string
	statePath string
	clk       *manualClock
}

func newSchedEnv(t *testing.T, exec Executor, start time.Time) *schedEnv {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "sched.jsonl")
	lg, err := session.OpenFile(logPath, "sess-sched-1")
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	m, err := NewManager(lg, exec, Options{})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(m.Stop)
	return &schedEnv{
		m:         m,
		lg:        lg,
		logPath:   logPath,
		statePath: filepath.Join(dir, "sched-state.json"),
		clk:       newManualClock(start),
	}
}

func (e *schedEnv) scheduler(t *testing.T) *Scheduler {
	t.Helper()
	s, err := NewScheduler(SchedulerOptions{
		Dispatch:  e.m,
		Gate:      ManagerIdleGate(e.m),
		Clock:     e.clk,
		StatePath: e.statePath,
	})
	if err != nil {
		t.Fatalf("NewScheduler: %v", err)
	}
	return s
}

func enqueuedKinds(events []session.Event) []string {
	var out []string
	for _, ev := range events {
		if ev.Type != session.TypeJobEnqueued {
			continue
		}
		var p session.JobPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			continue
		}
		out = append(out, p.Kind)
	}
	return out
}

var schedBase = time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

// --- idle-gated dispatch ------------------------------------------------------

// TestSchedulerIdleGateDefersWhileOwnerBusy: dispatch is deferred while
// the owner has an in-flight job (queued OR running) and fires once the
// executor is idle again — and the scheduled job/enqueued lands AFTER
// the blocking job settled.
func TestSchedulerIdleGateDefersWhileOwnerBusy(t *testing.T) {
	started := make(chan string, 4)
	release := make(chan struct{})
	exec := &blockingExec{started: started, release: release}
	env := newSchedEnv(t, exec, schedBase)
	s := env.scheduler(t)

	due := schedBase.Add(-time.Minute) // already due
	if _, err := s.Add(ScheduleSpec{Name: "tick", Every: time.Minute, At: &due}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Occupy the owner with a manually dispatched blocking job.
	if _, err := env.m.Dispatch("manual", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("manual Dispatch: %v", err)
	}
	select {
	case id := <-started:
		if id != "manual-1" {
			t.Fatalf("executor entered %q, want manual-1", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("executor never entered the manual job")
	}

	// Due spec + busy owner → Tick defers, nothing enqueued for the schedule.
	if n := s.Tick(); n != 0 {
		t.Fatalf("Tick dispatched %d while owner busy, want 0", n)
	}
	for _, k := range enqueuedKinds(env.lg.Events()) {
		if k == "sched-tick" {
			t.Fatal("sched-tick dispatched while owner busy — idle gate failed")
		}
	}

	// Owner goes idle → the same due state now dispatches.
	close(release)
	env.m.Drain()
	if n := s.Tick(); n != 1 {
		t.Fatalf("Tick dispatched %d when idle, want 1", n)
	}
	env.m.Drain()

	// Ordering proof: the scheduled enqueue landed after the manual settle.
	events := env.lg.Events()
	schedEnqSeq, manualSettleSeq := int64(-1), int64(-1)
	for _, ev := range events {
		switch ev.Type {
		case session.TypeJobEnqueued:
			var p session.JobPayload
			_ = json.Unmarshal(ev.Payload, &p)
			if p.Kind == "sched-tick" {
				schedEnqSeq = ev.Seq
			}
		case session.TypeJobSettled:
			var p session.JobPayload
			_ = json.Unmarshal(ev.Payload, &p)
			if p.JobID == "manual-1" {
				manualSettleSeq = ev.Seq
			}
		}
	}
	if schedEnqSeq < 0 || manualSettleSeq < 0 {
		t.Fatalf("missing events: schedEnq=%d manualSettle=%d", schedEnqSeq, manualSettleSeq)
	}
	if schedEnqSeq < manualSettleSeq {
		t.Fatalf("sched-tick enqueued at seq %d BEFORE manual settle at seq %d — gate not ordered", schedEnqSeq, manualSettleSeq)
	}
}

// --- catch-up collapse ----------------------------------------------------------

// TestSchedulerCatchUpCollapseRealManager: with the clock 3 intervals
// past the first due instant, exactly ONE dispatch covers the gap and
// the next cursor is the first occurrence strictly after now.
func TestSchedulerCatchUpCollapseRealManager(t *testing.T) {
	env := newSchedEnv(t, &quickExec{}, schedBase)
	s := env.scheduler(t)

	t0 := schedBase
	if _, err := s.Add(ScheduleSpec{Name: "tick", Every: time.Minute, At: &t0}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Downtime: 3 whole intervals elapse before the first tick.
	env.clk.Advance(t0.Add(3 * time.Minute))
	if n := s.Tick(); n != 1 {
		t.Fatalf("Tick dispatched %d jobs, want exactly 1 (collapse)", n)
	}
	env.m.Drain()

	if got := countEvents(env.lg, session.TypeJobEnqueued); got != 1 {
		t.Fatalf("job/enqueued count = %d, want 1 (no storm replay)", got)
	}
	snap := s.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("snapshot len = %d, want 1", len(snap))
	}
	if want := t0.Add(4 * time.Minute); !snap[0].NextDue.Equal(want) {
		t.Fatalf("next due = %v, want %v (latest missed occurrence + 1 interval)", snap[0].NextDue, want)
	}

	// Still inside the same gap → nothing more fires.
	if n := s.Tick(); n != 0 {
		t.Fatalf("second Tick in the same gap dispatched %d, want 0", n)
	}

	// The next occurrence fires exactly once at its due instant.
	env.clk.Advance(t0.Add(4 * time.Minute))
	if n := s.Tick(); n != 1 {
		t.Fatalf("Tick at next due dispatched %d, want 1", n)
	}
	env.m.Drain()
	if got := countEvents(env.lg, session.TypeJobEnqueued); got != 2 {
		t.Fatalf("job/enqueued count = %d, want 2", got)
	}
}

// --- one-shot -------------------------------------------------------------------

// TestSchedulerOneShotRemovedAfterDispatch: After/At specs fire once and
// leave the durable state (dispatch persists the removal).
func TestSchedulerOneShotRemovedAfterDispatch(t *testing.T) {
	env := newSchedEnv(t, &quickExec{}, schedBase)
	s := env.scheduler(t)

	due := schedBase.Add(-time.Second)
	if _, err := s.Add(ScheduleSpec{Name: "once", At: &due, Payload: json.RawMessage(`{"x":1}`)}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if n := s.Tick(); n != 1 {
		t.Fatalf("Tick dispatched %d, want 1 (past At due immediately)", n)
	}
	env.m.Drain()
	if len(s.Snapshot()) != 0 {
		t.Fatalf("one-shot still present after dispatch: %+v", s.Snapshot())
	}
	// The state file no longer carries it.
	st, err := loadSchedulerState(env.statePath)
	if err != nil {
		t.Fatalf("loadSchedulerState: %v", err)
	}
	if len(st) != 0 {
		t.Fatalf("state file still carries the fired one-shot: %+v", st)
	}
	if n := s.Tick(); n != 0 {
		t.Fatalf("re-tick dispatched %d, want 0", n)
	}
}

// --- at-least-once ----------------------------------------------------------------

// TestSchedulerAtLeastOnceStateDeleted: losing the persisted cursor
// (deleted state file = the crash-between-dispatch-and-persist window)
// re-dispatches a due spec — at-least-once. Duplicate SUPPRESSION is
// not the scheduler's: the jobs layer settles each job first-wins and
// reports each settled job exactly once (asserted here as the boundary).
func TestSchedulerAtLeastOnceStateDeleted(t *testing.T) {
	env := newSchedEnv(t, &quickExec{}, schedBase)
	s := env.scheduler(t)

	t0 := schedBase
	if _, err := s.Add(ScheduleSpec{Name: "tick", Every: time.Minute, At: &t0}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if n := s.Tick(); n != 1 {
		t.Fatalf("first Tick dispatched %d, want 1", n)
	}
	env.m.Drain()

	// The crash window: dispatch happened, persistence did not.
	if err := os.Remove(env.statePath); err != nil {
		t.Fatalf("remove state: %v", err)
	}
	s2 := env.scheduler(t) // same manager, same path, state gone
	if _, err := s2.Add(ScheduleSpec{Name: "tick", Every: time.Minute, At: &t0}); err != nil {
		t.Fatalf("re-Add: %v", err)
	}
	if n := s2.Tick(); n != 1 {
		t.Fatalf("post-crash Tick dispatched %d, want 1 (at-least-once)", n)
	}
	env.m.Drain()

	if got := countEvents(env.lg, session.TypeJobEnqueued); got != 2 {
		t.Fatalf("job/enqueued count = %d, want 2 (duplicate dispatch admitted)", got)
	}
	// Boundary discipline: two distinct jobs, each settled + reported once.
	if got := countEvents(env.lg, session.TypeJobSettled); got != 2 {
		t.Fatalf("job/settled count = %d, want 2", got)
	}
	if n, err := env.m.EmitReports(); err != nil || n != 2 {
		t.Fatalf("EmitReports = (%d, %v), want (2, nil) — one report per settled job", n, err)
	}
	if n, err := env.m.EmitReports(); err != nil || n != 0 {
		t.Fatalf("second EmitReports = (%d, %v), want (0, nil) — reported-flag dedup", n, err)
	}
}

// --- restart / persistence -----------------------------------------------------

// TestSchedulerCursorSurvivesRestart: a fresh Scheduler over the same
// state file adopts the persisted next-run cursor (no re-fire of the
// already-covered occurrence, no lost cadence).
func TestSchedulerCursorSurvivesRestart(t *testing.T) {
	env := newSchedEnv(t, &quickExec{}, schedBase)
	s := env.scheduler(t)

	t0 := schedBase
	if _, err := s.Add(ScheduleSpec{Name: "tick", Every: time.Minute, At: &t0}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if n := s.Tick(); n != 1 {
		t.Fatalf("Tick dispatched %d, want 1", n)
	}
	env.m.Drain()

	s2 := env.scheduler(t)
	snap := s2.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("restarted snapshot len = %d, want 1", len(snap))
	}
	if want := t0.Add(time.Minute); !snap[0].NextDue.Equal(want) {
		t.Fatalf("restarted next due = %v, want %v", snap[0].NextDue, want)
	}
	if n := s2.Tick(); n != 0 {
		t.Fatalf("restarted Tick re-fired the covered occurrence: %d dispatched", n)
	}
	env.clk.Advance(t0.Add(time.Minute))
	if n := s2.Tick(); n != 1 {
		t.Fatalf("restarted Tick at due dispatched %d, want 1", n)
	}
	env.m.Drain()
}

// --- lifecycle -----------------------------------------------------------------

// TestSchedulerLoopStartStopDrains: the run loop consumes clock ticks,
// dispatches due specs, and Stop DRAINS — the loop goroutine exits (Done
// closes) and no dispatch or persist races the stop. No further
// dispatches fire after Stop.
func TestSchedulerLoopStartStopDrains(t *testing.T) {
	env := newSchedEnv(t, &quickExec{}, schedBase)
	s := env.scheduler(t)

	t0 := schedBase
	if _, err := s.Add(ScheduleSpec{Name: "tick", Every: time.Minute, At: &t0}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := s.Start(); err == nil {
		t.Fatal("second Start must error")
	}

	// The loop must ARM its ticker before the clock can fire it — wait
	// for registration, then advance past the due instant.
	waitFor(t, 2*time.Second, func() bool { return env.clk.waiterCount() >= 1 }, "loop ticker armed")
	env.clk.Advance(t0.Add(time.Minute))
	waitFor(t, 2*time.Second, func() bool {
		return countEvents(env.lg, session.TypeJobEnqueued) == 1
	}, "loop dispatch")
	env.m.Drain()

	s.Stop() // must return only after the loop goroutine exited
	select {
	case <-s.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler loop did not exit after Stop (goroutine leak)")
	}
	s.Stop() // idempotent

	before := countEvents(env.lg, session.TypeJobEnqueued)
	env.clk.Advance(t0.Add(10 * time.Minute))
	// The loop goroutine has exited (Done closed above) — no consumer of
	// clock ticks remains, so no dispatch can fire; the beat only rules
	// out a late persist from a straggler.
	time.Sleep(25 * time.Millisecond)
	if got := countEvents(env.lg, session.TypeJobEnqueued); got != before {
		t.Fatalf("dispatches fired after Stop: %d → %d", before, got)
	}
}

// TestSchedulerValidationOnConstruction: nil seams and missing state
// path are deterministic constructor errors.
func TestSchedulerValidationOnConstruction(t *testing.T) {
	if _, err := NewScheduler(SchedulerOptions{Gate: ManagerIdleGate(nil), StatePath: "x"}); err == nil {
		t.Fatal("nil Dispatch accepted")
	}
	env := newSchedEnv(t, &quickExec{}, schedBase)
	if _, err := NewScheduler(SchedulerOptions{Dispatch: env.m, StatePath: env.statePath}); err == nil {
		t.Fatal("nil Gate accepted")
	}
	if _, err := NewScheduler(SchedulerOptions{Dispatch: env.m, Gate: ManagerIdleGate(env.m)}); err == nil {
		t.Fatal("empty StatePath accepted")
	}
	// Invalid spec rejected at Add, before any persistence.
	s := env.scheduler(t)
	due := schedBase
	if _, err := s.Add(ScheduleSpec{Name: "bad", Every: -time.Minute, At: &due}); err == nil {
		t.Fatal("invalid spec accepted at Add")
	}
	if _, err := s.Add(ScheduleSpec{Name: "dup", Every: time.Minute, At: &due}); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	if _, err := s.Add(ScheduleSpec{Name: "dup", Every: time.Hour, At: &due}); err == nil {
		t.Fatal("duplicate schedule name accepted")
	}
}

// --- the slice crux -------------------------------------------------------------

// TestSchedulerCruxCatchUpAndIdleGateAgainstRealManager is the slice
// crux: catch-up collapse + idle gating against the REAL jobs.Manager
// and executor over a file-backed session log, with the resulting log
// replayed and folded by the EXISTING foldJobs — scheduled dispatches
// are ordinary job/enqueued events under the existing vocabulary and
// the fold rebuilds them without any scheduler-specific knowledge.
func TestSchedulerCruxCatchUpAndIdleGateAgainstRealManager(t *testing.T) {
	started := make(chan string, 4)
	release := make(chan struct{})
	exec := &blockingExec{started: started, release: release}
	env := newSchedEnv(t, exec, schedBase)
	s := env.scheduler(t)

	t0 := schedBase
	if _, err := s.Add(ScheduleSpec{Name: "nightly", Every: time.Minute, At: &t0, Payload: json.RawMessage(`{"job":"digest"}`)}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Owner busy with a blocking manual job while 3 intervals of schedule
	// time elapse.
	if _, err := env.m.Dispatch("manual", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("manual Dispatch: %v", err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("executor never entered the manual job")
	}
	env.clk.Advance(t0.Add(3 * time.Minute))

	// Busy owner: the due schedule defers (no dispatch mid-flight).
	if n := s.Tick(); n != 0 {
		t.Fatalf("Tick dispatched %d while owner busy, want 0", n)
	}

	// Idle: exactly one collapsed catch-up dispatch.
	close(release)
	env.m.Drain()
	if n := s.Tick(); n != 1 {
		t.Fatalf("Tick dispatched %d when idle, want 1 (collapse)", n)
	}
	env.m.Drain()

	snap := s.Snapshot()
	if len(snap) != 1 || !snap[0].NextDue.Equal(t0.Add(4*time.Minute)) {
		t.Fatalf("post-collapse cursor = %+v, want next due %v", snap, t0.Add(4*time.Minute))
	}
	if n, err := env.m.EmitReports(); err != nil || n != 2 {
		t.Fatalf("EmitReports = (%d, %v), want (2, nil) — manual + collapsed schedule", n, err)
	}

	// Replay the durable log and fold it with the EXISTING fold: the
	// scheduler's dispatches are ordinary job/* events.
	live := env.lg.Events()
	replayed, err := session.ReplayFile(env.logPath)
	if err != nil {
		t.Fatalf("ReplayFile: %v", err)
	}
	if len(replayed) != len(live) {
		t.Fatalf("replay has %d events, live log has %d", len(replayed), len(live))
	}
	recs, order, counters, owner, err := foldJobs(replayed)
	if err != nil {
		t.Fatalf("foldJobs: %v", err)
	}
	if owner != "sess-sched-1" {
		t.Fatalf("fold owner = %q, want sess-sched-1", owner)
	}
	schedRec, ok := recs["sched-nightly-1"]
	if !ok {
		t.Fatalf("fold missing sched-nightly-1; order=%v kinds=%v", order, enqueuedKinds(replayed))
	}
	if schedRec.settledSeq == 0 || schedRec.settleResult != session.JobResultCompleted {
		t.Fatalf("sched-nightly-1 not settled-completed in the fold: %+v", schedRec)
	}
	if counters["sched-nightly"] != 1 {
		t.Fatalf("fold counter for sched-nightly = %d, want 1", counters["sched-nightly"])
	}

	if err := env.lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
