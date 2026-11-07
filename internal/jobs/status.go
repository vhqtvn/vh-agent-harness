// status.go — the fold-derived job status snapshot served by the host
// protocol's jobs/status method. State is read from the manager's
// fold-seeded records (the log is the source of truth; memory is the
// accelerator), in enqueue order.
package jobs

// Job lifecycle states as observed on the wire.
const (
	StateQueued  = "queued"  // enqueued, not yet started
	StateRunning = "running" // job/started appended, not yet settled
	StateSettled = "settled" // terminal job/settled landed
)

// Status is one job's fold-derived state (the jobs/status wire shape).
type Status struct {
	JobID  string `json:"jobId"`
	Kind   string `json:"kind"`
	State  string `json:"state"`
	Result string `json:"result,omitempty"` // completed|failed (settled only)
	Reason string `json:"reason,omitempty"` // failure text (failed only)
}

// Snapshot returns the status of every known job in enqueue order,
// derived from the fold-seeded records. It is a read-only projection:
// no locks are held on return and no events are appended.
func (m *Manager) Snapshot() []Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Status, 0, len(m.order))
	for _, id := range m.order {
		rec := m.jobs[id]
		if rec == nil {
			continue
		}
		s := Status{JobID: id, Kind: rec.job.Kind}
		switch {
		case rec.settledSeq != 0:
			s.State = StateSettled
			s.Result = rec.settleResult
			s.Reason = rec.settleReason
		case rec.startedSeq != 0:
			s.State = StateRunning
		default:
			s.State = StateQueued
		}
		out = append(out, s)
	}
	return out
}
