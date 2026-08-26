// recover.go — the R9 crash-recovery pass (the slice crux).
//
// On startup, Recover(path, executor, opts) replays the crashed session
// log (tolerating a torn final write), resumes the SAME durable stream,
// folds the job state, and repairs it deterministically:
//
//   - enqueued-but-unsettled AND never started → re-dispatch through the
//     recovered manager's executor (at-least-once: the job may have
//     side effects from a first attempt the log cannot prove; idempotent
//     re-dispatch means the recovery PASS itself never duplicates a job
//     or a terminal event);
//   - started-but-unsettled (the torn tail of a crash mid-run: the dead
//     process's runner is gone and cannot be resumed) → a synthetic
//     job/settled {result: failed, reason: "recovered-after-crash"} so
//     the fold terminates and the outcome is reported like any failure;
//   - settled-but-unreported → the pending job/report notice is emitted
//     (dsh reported-flag semantics hold across the crash: the settled
//     state reaches the model exactly once).
//
// Recovery is deterministic and replay-testable: every decision and
// appended event is a pure function of the recovered event list, in log
// order, and a second Recover over the recovered log makes no decisions
// and appends nothing.
package jobs

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// RecoveredAfterCrashReason is the synthetic settle-fail marker carried
// by torn-tail settlements.
const RecoveredAfterCrashReason = "recovered-after-crash"

// RecoverySummary records the deterministic decisions one Recover pass
// made — the test-assertable audit of the repair.
type RecoverySummary struct {
	// TornBytes is the size of the uncommitted tail fragment dropped from
	// the log file (0 when the file ended cleanly).
	TornBytes int64
	// Redispatched lists the job ids queued for at-least-once re-execution.
	Redispatched []string
	// SyntheticSettles lists the job ids terminated with the
	// recovered-after-crash settle-fail marker.
	SyntheticSettles []string
	// ReportsEmitted counts the job/report notices emitted during the pass.
	ReportsEmitted int
}

// Recover performs the crash-recovery pass over the session log at path
// and returns a live Manager continuing that log. The recovered manager
// seeds every counter and record from the fold (the log is the only
// source of truth), queues re-dispatches, and starts its executor drain.
func Recover(path string, executor Executor, opts Options) (*Manager, RecoverySummary, error) {
	var sum RecoverySummary

	f, err := os.Open(path)
	if err != nil {
		return nil, sum, fmt.Errorf("jobs: recover: open %s: %w", path, err)
	}
	events, validBytes, torn, err := session.RecoverTail(f)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, sum, fmt.Errorf("jobs: recover %s: %w", path, err)
	}
	if torn {
		st, statErr := os.Stat(path)
		if statErr != nil {
			return nil, sum, fmt.Errorf("jobs: recover: stat %s: %w", path, statErr)
		}
		sum.TornBytes = st.Size() - validBytes
		if err := os.Truncate(path, validBytes); err != nil {
			return nil, sum, fmt.Errorf("jobs: recover: truncate torn tail of %s: %w", path, err)
		}
	}

	af, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, sum, fmt.Errorf("jobs: recover: reopen %s for append: %w", path, err)
	}
	lg, err := session.ResumeLog(af, events)
	if err != nil {
		_ = af.Close()
		return nil, sum, fmt.Errorf("jobs: recover %s: %w", path, err)
	}

	maxInFly := opts.MaxInFlightPerOwner
	if maxInFly <= 0 {
		maxInFly = DefaultMaxInFlightPerOwner
	}
	retention := opts.OutputRetentionBytes
	if retention <= 0 {
		retention = DefaultOutputRetentionBytes
	}
	m := &Manager{
		lg:              lg,
		executor:        executor,
		maxInFly:        maxInFly,
		slots:           make(chan struct{}, maxInFly),
		counters:        make(map[string]int64),
		jobs:            make(map[string]*jobRecord),
		outputRetention: retention,
	}
	m.drained = sync.NewCond(&m.mu)
	m.queueCond = sync.NewCond(&m.mu)
	if err := m.seed(lg.Events()); err != nil {
		_ = lg.Close()
		return nil, sum, fmt.Errorf("jobs: recover %s: %w", path, err)
	}

	// Deterministic repair, in log (enqueue) order.
	m.mu.Lock()
	for _, id := range m.order {
		rec := m.jobs[id]
		switch {
		case rec.settledSeq == 0 && rec.startedSeq != 0:
			// Torn tail: the runner died mid-flight. Synthesize the
			// terminal event; never re-run.
			m.mu.Unlock()
			if err := m.Settle(id, errors.New(RecoveredAfterCrashReason)); err != nil {
				_ = lg.Close()
				return nil, sum, fmt.Errorf("jobs: recover: synthetic settle for %s: %w", id, err)
			}
			m.mu.Lock()
			sum.SyntheticSettles = append(sum.SyntheticSettles, id)
		case rec.settledSeq == 0 && rec.startedSeq == 0:
			// Never started: re-dispatch at-least-once.
			m.pending++
			m.queue = append(m.queue, rec.job)
			sum.Redispatched = append(sum.Redispatched, id)
		}
	}
	m.mu.Unlock()

	// Pending report notices for jobs settled at recovery time (including
	// the synthetic settles just written).
	n, err := m.EmitReports()
	if err != nil {
		_ = lg.Close()
		return nil, sum, fmt.Errorf("jobs: recover %s: %w", path, err)
	}
	sum.ReportsEmitted = n

	go m.dispatchLoop()
	return m, sum, nil
}
