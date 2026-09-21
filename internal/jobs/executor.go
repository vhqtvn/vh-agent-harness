// executor.go — the default executor drain: one serial dispatcher
// goroutine pulls jobs FIFO from the queue, respects the per-owner
// in-flight cap, appends job/started before handing the job to a runner
// goroutine, and settles first-wins when the runner returns.
//
// dsh pattern: a per-owner concurrency cap (default 10) bounds how many
// of one owner's jobs run simultaneously; dispatch is queued-not-delivered
// with FIFO ordering, and job/started is appended by the serial
// dispatcher, so the started order in the log is deterministic.
//
// Locking discipline: the dispatcher blocks ONLY on the owner semaphore
// (never while holding mu); Dispatch enqueues under mu and signals. A
// channel-based queue would deadlock here (send-under-lock vs
// lock-to-receive), hence the slice FIFO + condvar.
package jobs

import (
	"context"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// dispatchLoop is the serial pump. Dequeuing, acquiring an owner slot,
// and appending job/started happen sequentially in this ONE goroutine, so
// the started order always equals the enqueue order.
func (m *Manager) dispatchLoop() {
	m.mu.Lock()
	for {
		for len(m.queue) == 0 && !m.stopped {
			m.queueCond.Wait()
		}
		if len(m.queue) == 0 { // stopped and drained
			m.mu.Unlock()
			return
		}
		job := m.queue[0]
		m.queue = m.queue[1:]
		m.mu.Unlock()

		m.slots <- struct{}{} // acquire the per-owner in-flight slot (blocks at cap, no locks held)
		m.appendStarted(job)
		go m.runJob(job)

		m.mu.Lock()
	}
}

// appendStarted logs job/started for job (under the manager lock, keeping
// seq assignment race-free against concurrent Settle/EmitReports). If the
// durable start record cannot be written, execution still proceeds — the
// terminal job/settled event is what the recovery fold keys on.
func (m *Manager) appendStarted(job Job) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.jobs[job.ID]
	if !ok || rec.startedSeq != 0 {
		return // unknown or already started (recovery re-dispatch guard)
	}
	ev, err := m.lg.Append(session.TypeJobStarted, nil, session.JobPayload{
		JobID: job.ID, Kind: job.Kind, Owner: job.Owner,
	})
	if err != nil {
		return
	}
	rec.startedSeq = ev.Seq
}

// runJob executes one job body and settles it. Executors implementing
// OutputExecutor (P6) receive the job's output writer and their
// terminal detail rides job/settled + job/report. Cancellation is a
// documented non-goal of this slice: runners use a background context.
func (m *Manager) runJob(job Job) {
	defer m.finishJob()
	var detail string
	var err error
	if ox, ok := m.executor.(OutputExecutor); ok {
		detail, err = ox.RunWithOutput(context.Background(), job, m.writerFor(job.ID))
	} else {
		err = m.executor.Run(context.Background(), job)
	}
	_ = m.SettleWithDetail(job.ID, err, detail) // first-wins; settle errors surface in the log
}

// finishJob releases the owner slot and marks the job no longer pending.
func (m *Manager) finishJob() {
	<-m.slots
	m.mu.Lock()
	m.pending--
	if m.pending == 0 {
		m.drained.Broadcast()
	}
	m.mu.Unlock()
}

// Drain blocks until every dispatched job has settled (queue empty, no
// runner in flight). It is the test/verification seam for "all background
// work reached a terminal state".
func (m *Manager) Drain() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for m.pending > 0 {
		m.drained.Wait()
	}
}

// Stop halts the dispatcher once the queue has drained into runners; it
// does not cancel running jobs (cancellation propagation is a non-goal of
// this slice). Stop is idempotent.
func (m *Manager) Stop() {
	m.mu.Lock()
	m.stopped = true
	m.queueCond.Broadcast()
	m.mu.Unlock()
}
