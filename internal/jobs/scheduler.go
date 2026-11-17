// scheduler.go — the schedule dimension's engine: an idle-gated
// dispatcher that turns due ScheduleSpecs into ordinary job/enqueued
// events through the jobs.Manager dispatch seam (queued-not-delivered),
// persisting its next-run cursor to a package-owned state file after
// every dispatch decision.
//
// dsh schedule semantics (see researches/sources/deepseek-harness/
// session-cognition.md §schedule/):
//
//   - dispatch waits for EXECUTOR IDLE: while the owner has any job
//     queued or running, due schedules defer (checked per pass, never
//     blocking settlement — the gate is a read-only poll of the
//     manager's exported Snapshot, so runners settle unaffected);
//   - at most ONE dispatch per pass: a dispatched job immediately makes
//     the owner non-idle, so the next schedule waits for full idle —
//     scheduled work never piles onto a busy owner;
//   - fixed-rate catch-up collapses to the latest due occurrence
//     (advanceEvery): one dispatch per due gap, no storm replay;
//   - at-least-once: the cursor is persisted AFTER the dispatch
//     decision; a crash in between re-dispatches on restart. Duplicate
//     suppression is NOT the scheduler's — it is the job layer's
//     first-wins settlement (one terminal event per job id) plus the
//     reported-flag notice dedup (one report per settled job). A
//     re-dispatched occurrence is a NEW job id by design: idempotency
//     of the underlying work is the job body's concern;
//   - Start/Stop lifecycle with a DRAINED stop: Stop never interrupts a
//     pass mid-flight (a running Tick — dispatch + persist — completes
//     first; the loop only observes the stop between passes), and Stop
//     returns only after the loop goroutine has exited (Done closes).
//
// Tick is also exported as the maintenance-phase hook: an engine that
// already knows it is idle+maintenance may drive passes explicitly
// instead of relying on the internal poll loop.
package jobs

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// DefaultSchedulerPollInterval is the cadence of the standalone run
// loop's due-check passes (real time; tests inject a manual clock).
const DefaultSchedulerPollInterval = 500 * time.Millisecond

// Dispatcher is the dispatch seam the scheduler drives — satisfied by
// *jobs.Manager (Dispatch validates, logs job/enqueued, and returns the
// enqueue receipt immediately; it never blocks on execution).
type Dispatcher interface {
	Dispatch(kind string, payload json.RawMessage) (Receipt, error)
}

// IdleGate reports how many of the owner's jobs are in flight (queued
// or running). The scheduler defers all dispatch while this is > 0.
type IdleGate interface {
	InFlight() int
}

// ManagerIdleGate adapts a *Manager into an IdleGate over its EXPORTED
// Snapshot seam: queued|running states are exactly the in-flight jobs
// of the manager's (single) owner. Read-only and lock-free on return —
// polling it can never block settlement.
func ManagerIdleGate(m *Manager) IdleGate {
	return managerIdleGate{m: m}
}

type managerIdleGate struct{ m *Manager }

// InFlight counts queued+running jobs in the manager's snapshot.
func (g managerIdleGate) InFlight() int {
	n := 0
	for _, st := range g.m.Snapshot() {
		if st.State == StateQueued || st.State == StateRunning {
			n++
		}
	}
	return n
}

// SchedulerOptions configures one Scheduler. Dispatch, Gate, and
// StatePath are required; Clock defaults to RealClock and
// PollInterval to DefaultSchedulerPollInterval.
type SchedulerOptions struct {
	Dispatch     Dispatcher
	Gate         IdleGate
	Clock        Clock
	StatePath    string
	PollInterval time.Duration
}

// ScheduleRecord is the observable state of one registered schedule:
// the canonical spec plus its next-run cursor (UTC).
type ScheduleRecord struct {
	Spec    ScheduleSpec
	NextDue time.Time
}

// Scheduler owns the schedule dimension for one owner: registration
// (Add), the due-check pass (Tick / the Start-ed poll loop), durable
// cursor persistence, and the drained Stop.
type Scheduler struct {
	mu        sync.Mutex
	entries   []schedEntry // sorted by (NextDue, Name)
	dispatch  Dispatcher
	gate      IdleGate
	clock     Clock
	statePath string
	poll      time.Duration

	// persistFn is how every state-file write goes through: the real
	// atomic writer (writeSchedulerState) in production, an injectable
	// failure seam in tests (the ONLY reason it is a field). Injected
	// failures exercise the durability contracts — a failed Add or
	// Remove must leave live and durable state in agreement (F1).
	persistFn func(path string, entries []schedEntry) error

	startOnce sync.Once
	stopOnce  sync.Once
	started   bool
	stopped   bool
	stopCh    chan struct{}
	done      chan struct{}

	lastErr error
}

// NewScheduler builds a Scheduler around the injected seams, adopting
// any persisted state at StatePath (restart: due-again specs re-dispatch
// on the next pass — at-least-once).
func NewScheduler(opts SchedulerOptions) (*Scheduler, error) {
	if opts.Dispatch == nil {
		return nil, errors.New("scheduler: Dispatch seam is required")
	}
	if opts.Gate == nil {
		return nil, errors.New("scheduler: Gate seam is required")
	}
	if opts.StatePath == "" {
		return nil, errors.New("scheduler: StatePath is required (schedule state is a persisted file, not an event type)")
	}
	clock := opts.Clock
	if clock == nil {
		clock = RealClock{}
	}
	poll := opts.PollInterval
	if poll <= 0 {
		poll = DefaultSchedulerPollInterval
	}
	entries, err := loadSchedulerState(opts.StatePath)
	if err != nil {
		return nil, err
	}
	return &Scheduler{
		entries:   entries,
		dispatch:  opts.Dispatch,
		gate:      opts.Gate,
		clock:     clock,
		statePath: opts.StatePath,
		poll:      poll,
		persistFn: writeSchedulerState,
		stopCh:    make(chan struct{}),
		done:      make(chan struct{}),
	}, nil
}

// Add registers a spec: it is validated, canonicalized to UTC against
// the clock's now (After resolved into At), inserted in dispatch-priority
// order, and persisted immediately — a registered schedule survives a
// crash right after Add. Duplicate names are rejected (the name is the
// schedule's identity in the state file). Like Remove, the registration
// is PERSIST-FIRST: a candidate entry set (with the new schedule) is
// written to the state file, and only a SUCCESSFUL write is swapped
// into the live entries — a failed persist leaves both surfaces
// unchanged (no phantom schedule in the snapshot that a restart would
// drop; the schedule lands in both or neither) and returns the typed
// ErrSchedulePersist.
func (s *Scheduler) Add(spec ScheduleSpec) (ScheduleRecord, error) {
	stored, cursor, err := canonicalizeSpec(spec, s.clock.Now())
	if err != nil {
		return ScheduleRecord{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.entries {
		if s.entries[i].Spec.Name == stored.Name {
			return ScheduleRecord{}, fmt.Errorf("scheduler: schedule %q already exists", stored.Name)
		}
	}
	// Candidate copy WITH the new schedule: persisted FIRST, swapped
	// into live state only after the write lands. Appending to a copy
	// of a (NextDue, Name)-sorted slice and re-sorting keeps it sorted.
	candidate := make([]schedEntry, 0, len(s.entries)+1)
	candidate = append(candidate, s.entries...)
	candidate = append(candidate, schedEntry{Spec: stored, NextDue: cursor})
	sortSchedEntries(candidate)
	if err := s.persistFn(s.statePath, candidate); err != nil {
		err = fmt.Errorf("%w after add: %w", ErrSchedulePersist, err)
		s.lastErr = err
		return ScheduleRecord{}, err
	}
	s.entries = candidate
	return ScheduleRecord{Spec: stored, NextDue: cursor}, nil
}

// ErrScheduleNotFound is the typed error Remove returns for a name that
// is not registered (the wire maps it to a clean -32602 carrying the
// descriptive text).
var ErrScheduleNotFound = errors.New("scheduler: schedule not found")

// ErrSchedulePersist is the typed error class wrapped into every
// state-persist failure (Add/Remove/after-dispatch): the caller's
// request was VALID — the infrastructure failed. The wire maps it to
// -32000 (engine error) instead of -32602; check with errors.Is.
var ErrSchedulePersist = errors.New("scheduler: persist failed")

// Remove unregisters the named schedule and persists the state file
// atomically — a removed schedule stays removed across restarts. The
// removal is PERSIST-FIRST: a candidate entry set (without the removed
// schedule) is written to the state file, and only a SUCCESSFUL write is
// swapped into the live entries — a failed persist leaves both surfaces
// unchanged (the schedule stays in the snapshot AND the state file:
// durable/live agreement; the schedule lands in both or neither) and
// returns the typed ErrSchedulePersist. An unknown name is the typed
// ErrScheduleNotFound (check with errors.Is); a removed name may be
// re-registered afterwards (remove+re-add is the v1 pause path). Like
// Add, Remove takes the scheduler's own lock: it serializes with — but
// never runs — a Tick; the dispatch loop stays on its own goroutine.
func (s *Scheduler) Remove(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.entries {
		if s.entries[i].Spec.Name != name {
			continue
		}
		// Candidate copy WITHOUT the removed schedule: persisted FIRST,
		// swapped into live state only after the write lands. Removing
		// one entry of a (NextDue, Name)-sorted slice keeps it sorted.
		candidate := make([]schedEntry, 0, len(s.entries)-1)
		candidate = append(candidate, s.entries[:i]...)
		candidate = append(candidate, s.entries[i+1:]...)
		if err := s.persistFn(s.statePath, candidate); err != nil {
			err = fmt.Errorf("%w after remove: %w", ErrSchedulePersist, err)
			s.lastErr = err
			return err
		}
		s.entries = candidate
		return nil
	}
	return fmt.Errorf("%w: %q", ErrScheduleNotFound, name)
}

// Snapshot returns the registered schedules in dispatch-priority order.
func (s *Scheduler) Snapshot() []ScheduleRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ScheduleRecord, 0, len(s.entries))
	for i := range s.entries {
		out = append(out, ScheduleRecord{Spec: s.entries[i].Spec, NextDue: s.entries[i].NextDue})
	}
	return out
}

// Tick runs ONE due-check pass and returns how many dispatches it made
// (0 or 1 — at most one dispatch per pass by design). While the owner
// is busy (gate > 0) it defers everything; the earliest due schedule
// otherwise dispatches through the Dispatcher seam as an ordinary
// job/enqueued, and the resulting cursor decision is persisted before
// the pass returns. A dispatch error leaves the cursor untouched (the
// pass retries on the next tick) and is surfaced via LastError.
func (s *Scheduler) Tick() int {
	now := s.clock.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.entries) == 0 || s.entries[0].NextDue.After(now) {
		return 0 // nothing due (entries are priority-sorted)
	}
	if s.gate.InFlight() > 0 {
		return 0 // idle-gated: defer while the owner has in-flight jobs
	}
	e := s.entries[0]
	if _, err := s.dispatch.Dispatch(dispatchKind(e.Spec), e.Spec.Payload); err != nil {
		s.lastErr = fmt.Errorf("scheduler: dispatch %s: %w", e.Spec.Name, err)
		return 0 // cursor unchanged — retried on the next pass
	}
	if e.Spec.Every > 0 {
		e.NextDue = advanceEvery(e.NextDue, now, e.Spec.Every)
		s.entries[0] = e
		sortSchedEntries(s.entries)
	} else {
		s.entries = append(s.entries[:0], s.entries[1:]...) // one-shot done
	}
	if err := s.persistFn(s.statePath, s.entries); err != nil {
		// In-memory cursor advanced but not durable: a restart will
		// re-dispatch (the documented at-least-once window).
		s.lastErr = fmt.Errorf("%w after dispatch: %w", ErrSchedulePersist, err)
	} else {
		s.lastErr = nil
	}
	return 1
}

// Start launches the standalone poll loop (one Tick per PollInterval
// on the injected clock). Single-shot: a second Start, or a Start after
// Stop, is an error. Drive Tick manually instead when the engine's own
// maintenance phase should decide cadence.
func (s *Scheduler) Start() error {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return errors.New("scheduler: already stopped")
	}
	if s.started {
		s.mu.Unlock()
		return errors.New("scheduler: already started")
	}
	s.started = true
	s.mu.Unlock()
	go s.run()
	return nil
}

// run is the standalone loop: one due-check pass per poll interval,
// terminating between passes when Stop fires.
func (s *Scheduler) run() {
	defer close(s.done)
	for {
		select {
		case <-s.stopCh:
			return
		case <-s.clock.After(s.poll):
			s.Tick()
		}
	}
}

// Stop halts the scheduler DRAINED: an in-flight Tick (dispatch +
// persist) always completes — the loop only observes the stop between
// passes — and Stop returns only after the loop goroutine exited (Done
// closes). Idempotent; a stopped scheduler never dispatches again, but
// Add/persist still work (registration stays durable).
func (s *Scheduler) Stop() {
	s.mu.Lock()
	started := s.started
	s.stopped = true
	s.mu.Unlock()
	s.stopOnce.Do(func() { close(s.stopCh) })
	if started {
		<-s.done
	}
}

// Done closes when the run loop has exited (never, if never started).
func (s *Scheduler) Done() <-chan struct{} { return s.done }

// LastError returns the most recent dispatch/persist error, if any
// (nil after a clean dispatch+persist cycle).
func (s *Scheduler) LastError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastErr
}
