package jobs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// crashLog writes a fresh log file and returns its path.
func crashLog(t *testing.T, id string) (string, *session.Log) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "crashed.jsonl")
	lg, err := session.OpenFile(path, id)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	return path, lg
}

// foldFinal replays the log at path and asserts the two R9 recovery
// invariants over the resulting fold:
//   - exactly-once settlement: every job has exactly one job/settled;
//   - every settled job is reported exactly once (after EmitReports).
func foldFinal(t *testing.T, path string, afterReports bool) map[string]*jobRecord {
	t.Helper()
	events, err := session.ReplayFile(path)
	if err != nil {
		t.Fatalf("ReplayFile: %v", err)
	}
	recs, order, _, _, err := foldJobs(events)
	if err != nil {
		t.Fatalf("foldJobs on final log: %v", err)
	}
	for _, id := range order {
		rec := recs[id]
		if rec.settledSeq == 0 {
			t.Fatalf("job %s never settled (exactly-once settlement violated)", id)
		}
		if afterReports && rec.reportedSeq == 0 {
			t.Fatalf("settled job %s never reported", id)
		}
		if rec.reportedSeq != 0 && rec.reportedSeq <= rec.settledSeq {
			t.Fatalf("job %s reported at seq %d before its settle at seq %d", id, rec.reportedSeq, rec.settledSeq)
		}
	}
	// Count settle/report events directly — the fold dedups by
	// construction, so also prove the EVENT count is exactly one per job.
	settles, reports := map[string]int{}, map[string]int{}
	for _, ev := range events {
		var p session.JobPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			continue
		}
		switch ev.Type {
		case session.TypeJobSettled:
			settles[p.JobID]++
		case session.TypeJobReport:
			reports[p.JobID]++
		}
	}
	for _, id := range order {
		if settles[id] != 1 {
			t.Fatalf("job %s has %d job/settled events, want exactly 1", id, settles[id])
		}
		if afterReports && reports[id] != 1 {
			t.Fatalf("job %s has %d job/report events, want exactly 1", id, reports[id])
		}
	}
	return recs
}

// Scenario 1 — enqueued-but-unsettled (never started): Recover
// re-dispatches idempotently (at-least-once): the job runs to a terminal
// state on the recovered manager and the final log fold holds the
// exactly-once invariants.
func TestRecoverEnqueuedUnsettledRedispatches(t *testing.T) {
	path, lg := crashLog(t, "sess-rec-1")
	m0, err := NewManager(lg, &quickExec{}, Options{})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if _, err := m0.Dispatch("background", json.RawMessage(`{"cmd":"rebuild"}`)); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	// Enqueue-only crash: do NOT drain — the job never starts, the log
	// ends at job/enqueued.
	m0.Stop()
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	exec := &countingExec{}
	m, sum, err := Recover(path, exec, Options{})
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	t.Cleanup(m.Stop)
	if len(sum.Redispatched) != 1 || sum.Redispatched[0] != "background-1" {
		t.Fatalf("summary redispatched = %v, want [background-1]", sum.Redispatched)
	}
	m.Drain()
	if exec.runsOf("background-1") != 1 {
		t.Fatalf("re-dispatched job ran %d times, want 1", exec.runsOf("background-1"))
	}
	n, err := m.EmitReports()
	if err != nil {
		t.Fatalf("EmitReports: %v", err)
	}
	if n != 1 {
		t.Fatalf("EmitReports after recovery emitted %d, want 1", n)
	}
	if err := m.Log().Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	recs := foldFinal(t, path, true)
	if recs["background-1"].settleResult != session.JobResultCompleted {
		t.Fatalf("re-dispatched job settled %q, want completed", recs["background-1"].settleResult)
	}
}

// Scenario 2 — started-but-unsettled (the torn tail of a crash mid-run):
// the dead runner cannot be resumed, so Recover terminates the fold with
// a synthetic settle-fail carrying the recovered-after-crash marker, and
// the executor never re-runs the job.
func TestRecoverStartedUnsettledSyntheticSettle(t *testing.T) {
	path, lg := crashLog(t, "sess-rec-2")
	started := make(chan string, 4)
	release := make(chan struct{})
	exec := &blockingExec{started: started, release: release}
	m, err := NewManager(lg, exec, Options{})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if _, err := m.Dispatch("background", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	<-started // running; the "crash" freezes it there forever
	// Do NOT release; close the log from under the frozen runner.
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	t.Cleanup(func() { close(release); m.Stop() })

	exec2 := &countingExec{}
	m2, sum, err := Recover(path, exec2, Options{})
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	t.Cleanup(m2.Stop)
	m2.Drain()
	if len(sum.SyntheticSettles) != 1 || sum.SyntheticSettles[0] != "background-1" {
		t.Fatalf("summary synthetic settles = %v, want [background-1]", sum.SyntheticSettles)
	}
	if exec2.total() != 0 {
		t.Fatalf("recovered manager re-ran a torn-tail job %d times, want 0 (synthetic settle, not re-run)", exec2.total())
	}
	if _, err := m2.EmitReports(); err != nil {
		t.Fatalf("EmitReports: %v", err)
	}
	if err := m2.Log().Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	recs := foldFinal(t, path, true)
	rec := recs["background-1"]
	if rec.settleResult != session.JobResultFailed {
		t.Fatalf("torn-tail job settled %q, want failed", rec.settleResult)
	}
	if rec.settleReason != RecoveredAfterCrashReason {
		t.Fatalf("torn-tail settle reason = %q, want %q", rec.settleReason, RecoveredAfterCrashReason)
	}
}

// Scenario 3 — settled-but-unreported: Recover emits the pending report
// notice (dsh reported-flag semantics: the settled state reaches the
// model exactly once, across the crash).
func TestRecoverSettledUnreportedEmitsReport(t *testing.T) {
	path, lg := crashLog(t, "sess-rec-3")
	m, err := NewManager(lg, &quickExec{}, Options{})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if _, err := m.Dispatch("background", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	m.Drain()
	m.Stop()
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	m2, sum, err := Recover(path, &quickExec{}, Options{})
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	t.Cleanup(m2.Stop)
	if sum.ReportsEmitted != 1 {
		t.Fatalf("summary reports emitted = %d, want 1", sum.ReportsEmitted)
	}
	n, err := m2.EmitReports()
	if err != nil {
		t.Fatalf("EmitReports after recovery: %v", err)
	}
	if n != 0 {
		t.Fatalf("EmitReports after recovery emitted %d, want 0 (dedup holds across recovery)", n)
	}
	if err := m2.Log().Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	foldFinal(t, path, true)
}

// Combined scenario — mixed states recover in one deterministic pass and
// a SECOND Recover over the recovered log is a no-op (idempotent
// re-dispatch: at-least-once does not mean duplicate terminal events).
// States built by typed appends for precision:
//
//	digest-1   enqueued + started + settled + reported  (complete)
//	digest-2   enqueued + started + settled             (unreported)
//	background-1  enqueued only                         (re-dispatch)
//	background-2  enqueued + started                    (torn tail)
func TestRecoverMixedStatesAndSecondPassIsNoop(t *testing.T) {
	path, lg := crashLog(t, "sess-rec-4")

	appendJob := func(evType string, p session.JobPayload) {
		t.Helper()
		var surface *session.SurfaceOp
		if evType == session.TypeJobReport {
			surface = &session.SurfaceOp{Op: session.SurfaceOpAppend}
		}
		if _, err := lg.Append(evType, surface, p); err != nil {
			t.Fatalf("Append %s: %v", evType, err)
		}
	}
	owner := "sess-rec-4"
	appendJob(session.TypeJobEnqueued, session.JobPayload{JobID: "digest-1", Kind: "digest", Owner: owner})
	appendJob(session.TypeJobStarted, session.JobPayload{JobID: "digest-1", Kind: "digest", Owner: owner})
	appendJob(session.TypeJobSettled, session.JobPayload{JobID: "digest-1", Kind: "digest", Owner: owner, Result: session.JobResultCompleted})
	appendJob(session.TypeJobReport, session.JobPayload{JobID: "digest-1", Kind: "digest", Owner: owner, Result: session.JobResultCompleted})

	appendJob(session.TypeJobEnqueued, session.JobPayload{JobID: "digest-2", Kind: "digest", Owner: owner})
	appendJob(session.TypeJobStarted, session.JobPayload{JobID: "digest-2", Kind: "digest", Owner: owner})
	appendJob(session.TypeJobSettled, session.JobPayload{JobID: "digest-2", Kind: "digest", Owner: owner, Result: session.JobResultCompleted})

	appendJob(session.TypeJobEnqueued, session.JobPayload{JobID: "background-1", Kind: "background", Owner: owner})

	appendJob(session.TypeJobEnqueued, session.JobPayload{JobID: "background-2", Kind: "background", Owner: owner})
	appendJob(session.TypeJobStarted, session.JobPayload{JobID: "background-2", Kind: "background", Owner: owner})

	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	exec := &countingExec{}
	m, sum, err := Recover(path, exec, Options{})
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	t.Cleanup(m.Stop)

	if len(sum.SyntheticSettles) != 1 || sum.SyntheticSettles[0] != "background-2" {
		t.Fatalf("synthetic settles = %v, want [background-2]", sum.SyntheticSettles)
	}
	if len(sum.Redispatched) != 1 || sum.Redispatched[0] != "background-1" {
		t.Fatalf("redispatched = %v, want [background-1]", sum.Redispatched)
	}
	// Recover's report pass covers the settled-unreported jobs present at
	// recovery time: digest-2 and the just-synthesized background-2.
	if sum.ReportsEmitted != 2 {
		t.Fatalf("reports emitted during Recover = %d, want 2 (digest-2 + synthetic background-2)", sum.ReportsEmitted)
	}

	m.Drain() // background-1 runs to completion on the recovered manager
	if exec.runsOf("background-1") != 1 {
		t.Fatalf("re-dispatched job ran %d times, want 1", exec.runsOf("background-1"))
	}
	n, err := m.EmitReports()
	if err != nil {
		t.Fatalf("EmitReports: %v", err)
	}
	if n != 1 {
		t.Fatalf("EmitReports after drain emitted %d, want 1 (background-1)", n)
	}
	if err := m.Log().Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	foldFinal(t, path, true)

	// Second recovery pass: everything terminal + reported ⇒ no new
	// events, empty summary decisions.
	after, err := session.ReplayFile(path)
	if err != nil {
		t.Fatalf("ReplayFile: %v", err)
	}
	m3, sum3, err := Recover(path, &quickExec{}, Options{})
	if err != nil {
		t.Fatalf("second Recover: %v", err)
	}
	t.Cleanup(m3.Stop)
	m3.Drain()
	if len(sum3.Redispatched) != 0 || len(sum3.SyntheticSettles) != 0 || sum3.ReportsEmitted != 0 {
		t.Fatalf("second Recover decisions not empty: %+v", sum3)
	}
	if err := m3.Log().Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	final, err := session.ReplayFile(path)
	if err != nil {
		t.Fatalf("ReplayFile: %v", err)
	}
	if len(final) != len(after) {
		t.Fatalf("second Recover appended %d events, want 0 (idempotent pass)", len(final)-len(after))
	}
}

// Torn FILE tail integration: a crash that tore the final bytes mid-write
// still recovers — the torn fragment is dropped, the started-but-unsettled
// job gets its synthetic settle, and the resumed file replays cleanly.
func TestRecoverTornFileTail(t *testing.T) {
	path, lg := crashLog(t, "sess-rec-5")
	if _, err := lg.Append(session.TypeJobEnqueued, nil, session.JobPayload{
		JobID: "background-1", Kind: "background", Owner: "sess-rec-5",
	}); err != nil {
		t.Fatalf("Append job/enqueued: %v", err)
	}
	if _, err := lg.Append(session.TypeJobStarted, nil, session.JobPayload{
		JobID: "background-1", Kind: "background", Owner: "sess-rec-5",
	}); err != nil {
		t.Fatalf("Append job/started: %v", err)
	}
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Tear the file: append a partial settle record.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("open torn: %v", err)
	}
	if _, err := f.WriteString(`{"seq":4,"type":"job/sett`); err != nil {
		t.Fatalf("write torn: %v", err)
	}
	f.Close()

	m, sum, err := Recover(path, &countingExec{}, Options{})
	if err != nil {
		t.Fatalf("Recover over torn file: %v", err)
	}
	t.Cleanup(m.Stop)
	m.Drain()
	if sum.TornBytes == 0 {
		t.Fatalf("summary torn bytes = 0, want the dropped fragment size")
	}
	if _, err := m.EmitReports(); err != nil {
		t.Fatalf("EmitReports: %v", err)
	}
	if err := m.Log().Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	recs := foldFinal(t, path, true)
	if recs["background-1"].settleReason != RecoveredAfterCrashReason {
		t.Fatalf("torn settle reason = %q, want %q", recs["background-1"].settleReason, RecoveredAfterCrashReason)
	}
}

// countingExec is a recovery fixture that counts runs per job.
type countingExec struct {
	runs map[string]int
}

func (e *countingExec) Run(_ context.Context, job Job) error {
	if e.runs == nil {
		e.runs = make(map[string]int)
	}
	e.runs[job.ID]++
	return nil
}

func (e *countingExec) runsOf(id string) int { return e.runs[id] }
func (e *countingExec) total() int {
	n := 0
	for _, v := range e.runs {
		n += v
	}
	return n
}
