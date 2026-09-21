package jobs

import (
	"encoding/json"
	"errors"
	"fmt"
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

// --- removal (the wire's schedule/remove seam) ----------------------------------

// TestSchedulerRemovePersistsAndUnknownTyped: Remove unregisters a
// schedule from BOTH the in-memory snapshot and the durable state file
// (atomically), removing an UNKNOWN name is the typed
// ErrScheduleNotFound, and a removed name may be re-registered
// (remove+re-add is the v1 pause path).
func TestSchedulerRemovePersistsAndUnknownTyped(t *testing.T) {
	env := newSchedEnv(t, &quickExec{}, schedBase)
	s := env.scheduler(t)

	t0 := schedBase
	if _, err := s.Add(ScheduleSpec{Name: "tick", Every: time.Minute, At: &t0}); err != nil {
		t.Fatalf("Add tick: %v", err)
	}
	past := t0.Add(-time.Second)
	if _, err := s.Add(ScheduleSpec{Name: "once", At: &past}); err != nil {
		t.Fatalf("Add once: %v", err)
	}

	if err := s.Remove("tick"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if snap := s.Snapshot(); len(snap) != 1 || snap[0].Spec.Name != "once" {
		t.Fatalf("snapshot after remove = %+v, want only once", snap)
	}
	st, err := loadSchedulerState(env.statePath)
	if err != nil {
		t.Fatalf("loadSchedulerState: %v", err)
	}
	if len(st) != 1 || st[0].Spec.Name != "once" {
		t.Fatalf("state file after remove = %+v, want only once (removal persisted)", st)
	}

	// Unknown name: the typed error (wire: a clean -32602 with text).
	err = s.Remove("ghost")
	if !errors.Is(err, ErrScheduleNotFound) {
		t.Fatalf("Remove unknown error = %v, want ErrScheduleNotFound", err)
	}

	// A removed name is re-registerable (the v1 pause path).
	if _, err := s.Add(ScheduleSpec{Name: "tick", Every: time.Minute, At: &t0}); err != nil {
		t.Fatalf("re-Add after remove: %v", err)
	}
	if len(s.Snapshot()) != 2 {
		t.Fatalf("snapshot after re-add = %+v, want 2 entries", s.Snapshot())
	}
}

// --- removal durability (F1: persist-first-then-swap) ----------------------------

// TestSchedulerRemovePersistFailureIsAtomic (reviewer F1, blocking): a
// Remove whose persist FAILS must leave BOTH surfaces unchanged — the
// LIVE snapshot still lists the schedule AND a restarted scheduler over
// the same state file still adopts it (durable/live agreement: the
// remove lands in both or neither). The defect it guards: Remove used
// to mutate the in-memory entries before persisting, so a failed
// remove made the live snapshot omit a schedule the state file still
// carried — the schedule returned after restart while schedule/list
// denied it existed.
func TestSchedulerRemovePersistFailureIsAtomic(t *testing.T) {
	env := newSchedEnv(t, &quickExec{}, schedBase)
	s := env.scheduler(t)

	t0 := schedBase
	if _, err := s.Add(ScheduleSpec{Name: "tick", Every: time.Minute, At: &t0}); err != nil {
		t.Fatalf("Add tick: %v", err)
	}
	past := t0.Add(-time.Second)
	if _, err := s.Add(ScheduleSpec{Name: "once", At: &past}); err != nil {
		t.Fatalf("Add once: %v", err)
	}

	// Inject the persist failure through the test-only seam (production
	// default is the real atomic writer). The error mimics the real
	// writer's shape: an underlying infrastructure failure.
	injected := fmt.Errorf("scheduler: create temp state %s: %w", env.statePath+".tmp", os.ErrPermission)
	s.persistFn = func(path string, entries []schedEntry) error { return injected }

	err := s.Remove("tick")
	if err == nil {
		t.Fatal("Remove with a failing persist returned nil")
	}

	// (a) LIVE agreement: the snapshot still lists BOTH schedules — the
	// failed remove changed nothing in memory.
	snap := s.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("snapshot after failed remove = %+v, want BOTH schedules (in-memory state must be unchanged by a failed persist)", snap)
	}

	// (b) DURABLE agreement: a restarted scheduler over the same state
	// file still adopts both — the remove did not half-land on disk.
	s2 := env.scheduler(t)
	if got := len(s2.Snapshot()); got != 2 {
		t.Fatalf("restarted scheduler adopted %d schedules, want 2 (the state file must be unchanged by a failed persist)", got)
	}

	// The failure is the TYPED engine-fault class (the wire maps it to
	// -32000, not -32602 — the caller's params were valid).
	if !errors.Is(err, ErrSchedulePersist) {
		t.Fatalf("Remove error = %v, want it to wrap ErrSchedulePersist (the typed engine-fault class)", err)
	}

	// Recovery: with the real writer restored, the SAME remove succeeds,
	// persists, and the scheduler was not poisoned by the failure.
	s.persistFn = writeSchedulerState
	if err := s.Remove("tick"); err != nil {
		t.Fatalf("Remove after persist recovery: %v", err)
	}
	if snap := s.Snapshot(); len(snap) != 1 || snap[0].Spec.Name != "once" {
		t.Fatalf("snapshot after successful remove = %+v, want only once", snap)
	}
	st, err := loadSchedulerState(env.statePath)
	if err != nil {
		t.Fatalf("loadSchedulerState: %v", err)
	}
	if len(st) != 1 || st[0].Spec.Name != "once" {
		t.Fatalf("state file after successful remove = %+v, want only once", st)
	}
}

// TestSchedulerAddPersistFailureIsAtomic (mirror of the reviewer-F1
// Remove defect, same class): an Add whose persist FAILS must leave
// BOTH surfaces unchanged — the LIVE snapshot does NOT list the new
// schedule AND a restarted scheduler over the same state file does NOT
// adopt it (durable/live agreement: the schedule lands in both or
// neither). The defect it guards: Add used to mutate the in-memory
// entries before persisting, so a failed Add left a phantom schedule in
// the snapshot that a restart silently dropped — schedule/list advertised
// a schedule the durable state never carried.
func TestSchedulerAddPersistFailureIsAtomic(t *testing.T) {
	env := newSchedEnv(t, &quickExec{}, schedBase)
	s := env.scheduler(t)

	t0 := schedBase
	if _, err := s.Add(ScheduleSpec{Name: "tick", Every: time.Minute, At: &t0}); err != nil {
		t.Fatalf("Add tick: %v", err)
	}

	// Inject the persist failure through the test-only seam (production
	// default is the real atomic writer). The error mimics the real
	// writer's shape: an underlying infrastructure failure.
	injected := fmt.Errorf("scheduler: create temp state %s: %w", env.statePath+".tmp", os.ErrPermission)
	s.persistFn = func(path string, entries []schedEntry) error { return injected }

	_, err := s.Add(ScheduleSpec{Name: "once", At: &t0})
	if err == nil {
		t.Fatal("Add with a failing persist returned nil")
	}

	// (a) LIVE agreement: the snapshot does NOT list the new schedule —
	// the failed Add changed nothing in memory (no phantom).
	snap := s.Snapshot()
	if len(snap) != 1 || snap[0].Spec.Name != "tick" {
		t.Fatalf("snapshot after failed add = %+v, want ONLY tick (in-memory state must be unchanged by a failed persist)", snap)
	}

	// (b) DURABLE agreement: a restarted scheduler over the same state
	// file does NOT adopt the new schedule — the add did not half-land
	// on disk, so live and restarted views agree.
	s2 := env.scheduler(t)
	if got := len(s2.Snapshot()); got != 1 {
		t.Fatalf("restarted scheduler adopted %d schedules, want 1 (the state file must be unchanged by a failed persist)", got)
	}

	// The failure is the TYPED engine-fault class (the wire maps it to
	// -32000, not -32602 — the caller's params were valid).
	if !errors.Is(err, ErrSchedulePersist) {
		t.Fatalf("Add error = %v, want it to wrap ErrSchedulePersist (the typed engine-fault class)", err)
	}

	// Recovery: with the real writer restored, the SAME add succeeds,
	// persists, and the scheduler was not poisoned by the failure (the
	// failed attempt left no duplicate-name residue).
	s.persistFn = writeSchedulerState
	if _, err := s.Add(ScheduleSpec{Name: "once", At: &t0}); err != nil {
		t.Fatalf("Add after persist recovery: %v", err)
	}
	if snap := s.Snapshot(); len(snap) != 2 {
		t.Fatalf("snapshot after successful add = %+v, want 2 entries", snap)
	}
	st, err := loadSchedulerState(env.statePath)
	if err != nil {
		t.Fatalf("loadSchedulerState: %v", err)
	}
	if len(st) != 2 {
		t.Fatalf("state file after successful add = %+v, want 2 entries", st)
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
