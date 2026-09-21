// Package jobs implements the native engine's async-jobs subsystem:
// durable background jobs with enqueue-receipt dispatch, first-wins
// settlement, reported-flag notice dedup, a per-owner-capped executor,
// and a deterministic crash-recovery pass.
//
// The semantics are ported from the dsh jobs subsystem (see
// researches/sources/deepseek-harness/session-cognition.md §jobs/, which
// transcribes docs/subsystems/jobs.md):
//
//   - IDs are `<kind>-N`, per-kind monotonic within the session.
//   - Dispatch is ENQUEUE-RECEIPT only: validate, log job/enqueued, return
//     {JobID} immediately — the dsh SDK pattern where a prompt is an
//     enqueue receipt, never a blocking execution.
//   - Owner fencing is authorization, NOT secrecy: the Owner field
//     records which session may act on the job ("fencing is authorization,
//     not secrecy — the id proves ownership"). It fences who may settle,
//     report, or re-dispatch; it is not a secrecy mechanism and job
//     payloads must not be treated as private to the owner.
//   - Settlement is FIRST-WINS: exactly one terminal job/settled event
//     per job; a second settle attempt is a no-op (logged-test-assertable).
//   - The reported flag suppresses duplicate completion notices to the
//     model (model-cost guard): one job/report per settled job, emitted
//     when the settled state is surfaced to the model.
//   - Per-owner concurrency is capped (dsh default 10).
//
// Storage discipline: the package consumes the session Log — job state is
// a fold over job/* events, never a second storage. In-memory maps are
// fold-seeded accelerators only; Recover rebuilds everything from the log.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// DefaultMaxInFlightPerOwner is the per-owner in-flight cap (dsh default
// is 10). Tests use 2 to make the cap observable.
const DefaultMaxInFlightPerOwner = 10

// Executor runs one job body. The injected executor is the seam where a
// real runtime (process, shell, RPC) attaches; Run returning nil settles
// the job completed, an error settles it failed with the error text.
type Executor interface {
	Run(ctx context.Context, job Job) error
}

// Job is one durable background work item as handed to an Executor.
type Job struct {
	ID      string          `json:"id"`
	Kind    string          `json:"kind"`
	Owner   string          `json:"owner"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Receipt is the immediate enqueue receipt returned by Dispatch.
type Receipt struct {
	JobID string `json:"jobId"`
}

// Options configures one Manager.
type Options struct {
	// MaxInFlightPerOwner caps concurrently RUNNING jobs per owner;
	// 0 ⇒ DefaultMaxInFlightPerOwner.
	MaxInFlightPerOwner int
	// OutputRetentionBytes caps retained output PER JOB (the P6
	// tailing ring); <=0 ⇒ DefaultOutputRetentionBytes. See
	// output.go for the full retention posture.
	OutputRetentionBytes int64
}

// Manager owns the async-jobs lifecycle over one session log: dispatch,
// executor draining, settlement, and notice emission. A Manager is bound
// to exactly one owner (the session that enqueues), so owner fencing is
// structural — it never operates on another session's log.
type Manager struct {
	mu        sync.Mutex
	drained   *sync.Cond // broadcast when pending reaches 0
	queueCond *sync.Cond // signaled when the dispatcher queue grows
	lg        *session.Log
	executor  Executor
	owner     string
	maxInFly  int

	queue   []Job // FIFO under mu (never a channel: sending under the lock deadlocks)
	stopped bool
	slots   chan struct{} // per-owner in-flight semaphore

	pending  int // queued + running (Drain waits for 0)
	counters map[string]int64
	jobs     map[string]*jobRecord
	order    []string // enqueue order

	// outputs holds the per-job in-memory output buffers (P6 tailing).
	// Lazily created by writerFor (a queued job has none yet); NOT
	// seeded by the fold — captured output is non-durable across
	// restart (see output.go).
	outputs         map[string]*outputBuffer
	outputRetention int64
}

// jobRecord is the fold-level state of one job.
type jobRecord struct {
	job          Job
	enqueuedSeq  int64
	startedSeq   int64
	settledSeq   int64
	settleResult string
	settleReason string
	settleDetail string
	reportedSeq  int64
}

// NewManager binds a Manager to lg and executor, seeding all state from
// the log fold (the log is the source of truth, memory is accelerator),
// and starts the executor drain goroutine.
func NewManager(lg *session.Log, executor Executor, opts Options) (*Manager, error) {
	if lg == nil {
		return nil, errors.New("jobs: nil session log")
	}
	if executor == nil {
		return nil, errors.New("jobs: nil executor")
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
		return nil, err
	}
	go m.dispatchLoop()
	return m, nil
}

// Log returns the session log the manager is bound to.
func (m *Manager) Log() *session.Log { return m.lg }

// seed rebuilds counters and job records from an event list (the fold).
func (m *Manager) seed(events []session.Event) error {
	recs, order, counters, owner, err := foldJobs(events)
	if err != nil {
		return err
	}
	m.jobs = recs
	m.order = order
	m.counters = counters
	m.owner = owner
	return nil
}

// Dispatch validates the request, logs job/enqueued, queues the job for
// the executor drain, and returns the enqueue receipt IMMEDIATELY. It
// never blocks on execution (dsh: the prompt is an enqueue receipt only).
func (m *Manager) Dispatch(kind string, payload json.RawMessage) (Receipt, error) {
	if err := validateKind(kind); err != nil {
		return Receipt{}, err
	}
	if len(payload) > 0 && !json.Valid(payload) {
		return Receipt{}, fmt.Errorf("jobs: payload for kind %q is not valid JSON", kind)
	}

	m.mu.Lock()
	m.counters[kind]++
	id := fmt.Sprintf("%s-%d", kind, m.counters[kind])
	job := Job{ID: id, Kind: kind, Owner: m.owner, Payload: append(json.RawMessage(nil), payload...)}
	if _, err := m.lg.Append(session.TypeJobEnqueued, nil, session.JobPayload{
		JobID: job.ID, Kind: job.Kind, Owner: job.Owner, Payload: job.Payload,
	}); err != nil {
		m.counters[kind]--
		m.mu.Unlock()
		return Receipt{}, fmt.Errorf("jobs: log job/enqueued: %w", err)
	}
	rec := &jobRecord{job: job}
	m.jobs[job.ID] = rec
	m.order = append(m.order, job.ID)
	m.pending++
	m.queue = append(m.queue, job)
	m.queueCond.Signal()
	m.mu.Unlock()
	return Receipt{JobID: job.ID}, nil
}

// Settle records the terminal state of one job: nil runErr settles
// completed, an error settles failed with its text. Settlement is
// FIRST-WINS (dsh): if the job is already settled this is a logged
// no-op — exactly one job/settled event ever lands per job.
func (m *Manager) Settle(jobID string, runErr error) error {
	return m.SettleWithDetail(jobID, runErr, "")
}

// EmitReports surfaces settled-but-unreported jobs to the model: one
// job/report notice per settled job (the ONE message-bearing job event),
// in settle order. Duplicate reports are suppressed (dsh `reported` flag
// as a model-cost guard). It returns how many notices were emitted; the
// engine calls it before deriving the surface so the model sees each
// settled job exactly once.
func (m *Manager) EmitReports() (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ids := make([]string, 0, len(m.jobs))
	for id, rec := range m.jobs {
		if rec.settledSeq != 0 && rec.reportedSeq == 0 {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return m.jobs[ids[i]].settledSeq < m.jobs[ids[j]].settledSeq })

	emitted := 0
	for _, id := range ids {
		rec := m.jobs[id]
		ev, err := m.lg.Append(session.TypeJobReport, &session.SurfaceOp{Op: session.SurfaceOpAppend}, session.JobPayload{
			JobID: rec.job.ID, Kind: rec.job.Kind, Owner: rec.job.Owner,
			Result: rec.settleResult, Reason: rec.settleReason, Detail: rec.settleDetail,
		})
		if err != nil {
			return emitted, fmt.Errorf("jobs: log job/report: %w", err)
		}
		rec.reportedSeq = ev.Seq
		emitted++
	}
	return emitted, nil
}

// foldJobs is the deterministic fold over a session event list that
// rebuilds all job state: per-job records in enqueue order, per-kind id
// counters, and the owning session id from the header. It enforces the
// writer invariants fail-loud (duplicate enqueue/settle/report for one
// job in a log is a writer bug, not a fold input).
func foldJobs(events []session.Event) (recs map[string]*jobRecord, order []string, counters map[string]int64, owner string, err error) {
	recs = make(map[string]*jobRecord)
	counters = make(map[string]int64)
	for i := range events {
		ev := events[i]
		switch ev.Type {
		case session.TypeSessionHeader:
			var hp session.HeaderPayload
			if err := json.Unmarshal(ev.Payload, &hp); err != nil {
				return nil, nil, nil, "", fmt.Errorf("jobs: malformed header payload: %w", err)
			}
			owner = hp.SessionID
		case session.TypeJobEnqueued:
			var p session.JobPayload
			if err := json.Unmarshal(ev.Payload, &p); err != nil {
				return nil, nil, nil, "", fmt.Errorf("jobs: malformed job/enqueued payload at seq %d: %w", ev.Seq, err)
			}
			if _, dup := recs[p.JobID]; dup {
				return nil, nil, nil, "", fmt.Errorf("jobs: duplicate job/enqueued for %q at seq %d", p.JobID, ev.Seq)
			}
			recs[p.JobID] = &jobRecord{
				job:         Job{ID: p.JobID, Kind: p.Kind, Owner: p.Owner, Payload: append(json.RawMessage(nil), p.Payload...)},
				enqueuedSeq: ev.Seq,
			}
			order = append(order, p.JobID)
			if n := idNumberOf(p.JobID); n > counters[p.Kind] {
				counters[p.Kind] = n
			}
		case session.TypeJobStarted:
			rec, ok := recs[jobIDOf(ev)]
			if !ok {
				return nil, nil, nil, "", fmt.Errorf("jobs: job/started at seq %d for unknown job", ev.Seq)
			}
			if rec.startedSeq != 0 {
				return nil, nil, nil, "", fmt.Errorf("jobs: duplicate job/started for %q at seq %d", rec.job.ID, ev.Seq)
			}
			rec.startedSeq = ev.Seq
		case session.TypeJobSettled:
			var p session.JobPayload
			if err := json.Unmarshal(ev.Payload, &p); err != nil {
				return nil, nil, nil, "", fmt.Errorf("jobs: malformed job/settled payload at seq %d: %w", ev.Seq, err)
			}
			rec, ok := recs[p.JobID]
			if !ok {
				return nil, nil, nil, "", fmt.Errorf("jobs: job/settled at seq %d for unknown job %q", ev.Seq, p.JobID)
			}
			if rec.settledSeq != 0 {
				return nil, nil, nil, "", fmt.Errorf("jobs: duplicate job/settled for %q at seq %d (first-wins violated by the writer)", p.JobID, ev.Seq)
			}
			rec.settledSeq = ev.Seq
			rec.settleResult = p.Result
			rec.settleReason = p.Reason
			rec.settleDetail = p.Detail
		case session.TypeJobReport:
			var p session.JobPayload
			if err := json.Unmarshal(ev.Payload, &p); err != nil {
				return nil, nil, nil, "", fmt.Errorf("jobs: malformed job/report payload at seq %d: %w", ev.Seq, err)
			}
			rec, ok := recs[p.JobID]
			if !ok {
				return nil, nil, nil, "", fmt.Errorf("jobs: job/report at seq %d for unknown job %q", ev.Seq, p.JobID)
			}
			if rec.reportedSeq != 0 {
				return nil, nil, nil, "", fmt.Errorf("jobs: duplicate job/report for %q at seq %d (reported dedup violated by the writer)", p.JobID, ev.Seq)
			}
			rec.reportedSeq = ev.Seq
		}
	}
	return recs, order, counters, owner, nil
}

// jobIDOf extracts the JobID from a job/* event payload.
func jobIDOf(ev session.Event) string {
	var p session.JobPayload
	_ = json.Unmarshal(ev.Payload, &p)
	return p.JobID
}

// idNumberOf parses the N of a `<kind>-N` job id (0 when unparseable).
func idNumberOf(id string) int64 {
	i := len(id) - 1
	for i >= 0 && id[i] >= '0' && id[i] <= '9' {
		i--
	}
	if i < 0 || i == len(id)-1 || id[i] != '-' {
		return 0
	}
	var n int64
	if _, err := fmt.Sscanf(id[i+1:], "%d", &n); err != nil {
		return 0
	}
	return n
}

// validateKind enforces the `<kind>` grammar of `<kind>-N` ids:
// a lowercase slug (first char a-z, then a-z0-9-), so ids stay
// unambiguous and filesystem/query friendly.
func validateKind(kind string) error {
	if kind == "" {
		return errors.New("jobs: kind is required")
	}
	for i := 0; i < len(kind); i++ {
		c := kind[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9' && i > 0:
		case c == '-' && i > 0:
		default:
			return fmt.Errorf("jobs: invalid kind %q: must be a lowercase slug [a-z][a-z0-9-]*", kind)
		}
	}
	return nil
}
