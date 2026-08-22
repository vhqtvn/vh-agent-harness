// sched.go — the daemon's slice-B1 scheduler wiring: internal/jobs'
// Scheduler constructed with REAL seams (the ACTIVE session's
// jobs.Manager as the dispatch/idle target through a tracking engine
// decorator), its state file under the session dir, started before
// Serve, and drained (Stop) at shutdown. B3 hands the scheduler to the
// engine seam (run() sets FileEngine.Schedules after buildScheduler,
// before Serve — every session/create stamps it), so the schedule/*
// wire methods reach it; the scheduler stays daemon-owned and the
// tracker keeps routing dispatches to the ACTIVE session.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sync"

	"github.com/vhqtvn/vh-agent-harness/internal/jobs"
	"github.com/vhqtvn/vh-agent-harness/internal/protocol"
)

// schedulerStateFilename is the scheduler's persisted cursor file,
// under the session dir (the daemon's single durable root).
const schedulerStateFilename = "scheduler-state.json"

// sessionTracker decorates the engine so the daemon-level scheduler can
// reach the ACTIVE session's real jobs.Manager: v1 is
// single-active-session (session/create supersedes), so "the owner" is
// whatever session was created last. Everything else delegates
// untouched. Thread-safe: NewSession is called from handler goroutines
// while the scheduler polls concurrently.
type sessionTracker struct {
	protocol.Engine
	mu     sync.Mutex
	active *protocol.EngineSession
}

// NewSession delegates to the wrapped engine and records the new active
// session. The wrapped construction AND the tracker's own assignment
// run as ONE serialized stage (t.mu held across both): concurrent
// creates cannot interleave this tracker's assignment between another
// create's engine supersede and its own assignment, so tracker.active
// and the engine's supersede seam never durably disagree
// (ship-review finding 1; the protocol server additionally serializes
// the whole session/create handler end-to-end — this lock keeps the
// daemon layer safe even when driven without the server).
func (t *sessionTracker) NewSession(path, sessionID string, sink io.Writer) (*protocol.EngineSession, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	es, err := t.Engine.NewSession(path, sessionID, sink)
	if err != nil {
		return nil, err
	}
	t.active = es
	return es, nil
}

// current returns the active session (nil before the first
// session/create).
func (t *sessionTracker) current() *protocol.EngineSession {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.active
}

// trackerDispatcher adapts the tracker into the scheduler's Dispatcher
// seam: dispatches route to the ACTIVE session's real jobs.Manager
// (job/enqueued lands in that session's durable log). With no active
// session it errors — the scheduler leaves the cursor untouched and
// retries on the next pass (the documented at-least-once window).
type trackerDispatcher struct{ t *sessionTracker }

func (d trackerDispatcher) Dispatch(kind string, payload json.RawMessage) (jobs.Receipt, error) {
	es := d.t.current()
	if es == nil {
		return jobs.Receipt{}, fmt.Errorf("vh-agentd: scheduler dispatch: no active session (schedule waits for session/create)")
	}
	return es.Jobs.Dispatch(kind, payload)
}

// trackerIdleGate adapts the tracker into the scheduler's IdleGate seam:
// in-flight = queued+running jobs of the ACTIVE session's manager; 0
// when there is no session (idle by vacuity — dispatch still fails
// closed on the dispatcher above).
type trackerIdleGate struct{ t *sessionTracker }

func (g trackerIdleGate) InFlight() int {
	es := g.t.current()
	if es == nil {
		return 0
	}
	n := 0
	for _, st := range es.Jobs.Snapshot() {
		if st.State == jobs.StateQueued || st.State == jobs.StateRunning {
			n++
		}
	}
	return n
}

// buildScheduler constructs the daemon's scheduler around the tracker's
// real seams with its state file under the session dir. The caller owns
// Start (before Serve) and Stop (drained, at shutdown).
func buildScheduler(cfg *Config, tracker *sessionTracker) (*jobs.Scheduler, error) {
	return jobs.NewScheduler(jobs.SchedulerOptions{
		Dispatch:  trackerDispatcher{t: tracker},
		Gate:      trackerIdleGate{t: tracker},
		StatePath: filepath.Join(cfg.SessionDir, schedulerStateFilename),
	})
}
