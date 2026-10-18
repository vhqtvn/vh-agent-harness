// output.go — the per-job progressive output channel (P6 job tailing):
// a bounded in-memory retention buffer per job plus offset-cursor read
// semantics over it, in the spill_read family (read(jobId, offset) →
// {chunk, nextOffset, state}).
//
// dsh pattern (researches/sources/deepseek-harness): background work
// returns a jobId immediately; output is read progressively through an
// offset cursor (SubprocessOutputReader.readFrom). This file realizes
// that pattern over the jobs.Manager's own lifecycle.
//
// DESIGN POSTURES (deliberate, disclosed — see also
// docs/native-engine/host-protocol.md §4g):
//
//   - IN-MEMORY, NON-DURABLE: buffers live in the Manager (a per-session
//     in-memory accelerator), NOT in the session log. Across a restart
//     the recovered job's settlement facts survive (they are log
//     events) but its captured output does NOT: a read at offset 0
//     reports written=0, and a read at a stale pre-restart offset
//     (offset > 0 ⇒ beyond written=0) is the typed *OutputAheadError —
//     §4g's nextOffset-never-exceeds-written holds on the nil-buffer
//     branch too. Rationale: durability would need a spill
//     store per job plus recovery wiring; v1 keeps the channel cheap
//     and documents the loss. The log stays byte-stable (no output
//     bytes are appended to it).
//
//   - RETENTION = TAIL-KEEPING RING, per job: DefaultOutputRetentionBytes
//     (256 KiB) of the MOST RECENT bytes are retained; older bytes are
//     dropped (evicted) as the producer wraps. A read whose offset
//     falls behind the window gets a typed *OutputEvictedError naming
//     the current base — the client re-syncs forward, never guesses.
//     Post-settle the buffer is frozen (no further writes), so reads
//     keep serving the tail within retention for the Manager's
//     lifetime.
//
//   - SINGLE COMBINED STREAM: one cursor addresses one byte stream
//     (stdout and stderr of a background shell interleave in write
//     order). Per-stream cursors would double the read surface; the
//     sync run_shell tool already captures per-stream for its frozen
//     result — this channel is the progressive tail, not that result.
//
//   - HONEST TRUNCATION IS STRUCTURAL, NEVER IN-BAND: a bounded read
//     (DefaultOutputChunkBytes) that cut the available window short
//     reports hasMore=true, and cumulative eviction reports
//     evictedBytes>0. No marker bytes are injected into the stream —
//     in-band markers would corrupt byte-offset reassembly, which is
//     the whole point of the cursor contract.
//
// Concurrency: one writer goroutine (the job body via writerFor) and
// any number of concurrent readers (jobs/output handlers). Each buffer
// carries its own mutex; the Manager lock is never held across a
// buffer operation (lock order: m.mu → pick buffer, release, then
// buf.mu).
package jobs

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// DefaultOutputRetentionBytes caps retained output PER JOB (the ring
// size). 256 KiB = 2× the sync tool's per-stream capture cap, bounded
// further by the per-owner in-flight cap (default 10) for running jobs.
const DefaultOutputRetentionBytes = 256 * 1024

// DefaultOutputChunkBytes bounds ONE read's served chunk. Larger
// available windows are paged with hasMore=true.
const DefaultOutputChunkBytes = 16 * 1024

// OutputExecutor is the optional Executor extension for jobs with a
// progressive output channel and a terminal detail: RunWithOutput
// receives the job's output writer (the capture seam above) and
// returns a compact terminal detail string that rides job/settled and
// job/report (JobPayload.Detail — e.g. the background shell's exit
// facts). Executors that do not implement it run unchanged through
// Executor.Run.
type OutputExecutor interface {
	RunWithOutput(ctx context.Context, job Job, out io.Writer) (detail string, err error)
}

// Output chunk states (the observed job lifecycle, same vocabulary as
// Status.State).
const (
	OutputStateQueued  = "queued"
	OutputStateRunning = "running"
	OutputStateSettled = "settled"
)

// OutputChunk is one offset-cursor read of a job's captured output
// (the jobs/output wire shape).
type OutputChunk struct {
	JobID string `json:"jobId"`
	// State is the job's fold state at read time (queued|running|settled).
	State string `json:"state"`
	// Chunk is the served byte window ("" when nothing new is
	// available — an honest poll answer, not an error).
	Chunk string `json:"chunk"`
	// Offset is the requested start (== the served window's start).
	Offset int64 `json:"offset"`
	// NextOffset is where the NEXT read must start: offset+len(chunk).
	// It never moves backwards and never re-serves bytes.
	NextOffset int64 `json:"nextOffset"`
	// HasMore is true when unread bytes were available beyond the
	// chunk bound at read time (page again immediately).
	HasMore bool `json:"hasMore"`
	// Written is the total bytes the job has ever produced (the EOF
	// cursor once state=settled: a read at offset==written with
	// hasMore=false has consumed the whole output).
	Written int64 `json:"written"`
	// EvictedBytes is the cumulative count of bytes dropped behind
	// the retention window (0 while the ring never wrapped).
	EvictedBytes int64 `json:"evictedBytes"`
}

// UnknownJobError is the typed read error for a job id the fold does
// not know (never dispatched, or a different session's job).
type UnknownJobError struct{ JobID string }

func (e *UnknownJobError) Error() string {
	return fmt.Sprintf("jobs: unknown job %q", e.JobID)
}

// OutputEvictedError is the typed read error for an offset that fell
// behind the retention window: the requested bytes were dropped by the
// ring. Base is the oldest offset still readable; re-sync forward.
type OutputEvictedError struct {
	JobID   string
	Base    int64
	Evicted int64
}

func (e *OutputEvictedError) Error() string {
	return fmt.Sprintf("jobs: offset behind the retention window of %s: %d bytes evicted, oldest readable offset is %d", e.JobID, e.Evicted, e.Base)
}

// OutputAheadError is the typed read error for an offset beyond the
// produced output (a client arithmetic bug — nextOffset never exceeds
// written).
type OutputAheadError struct {
	JobID   string
	Written int64
}

func (e *OutputAheadError) Error() string {
	return fmt.Sprintf("jobs: offset ahead of the produced output of %s (%d bytes written)", e.JobID, e.Written)
}

// outputBuffer is the bounded retention window of one job's output:
// the most recent cap bytes ever written, addressed by absolute offset
// (base = absolute offset of data[0]; written = total bytes produced).
type outputBuffer struct {
	mu      sync.Mutex
	data    []byte
	base    int64
	written int64
	dropped int64
	limit   int64
}

func newOutputBuffer(cap int64) *outputBuffer {
	if cap <= 0 {
		cap = DefaultOutputRetentionBytes
	}
	return &outputBuffer{limit: cap}
}

// Write appends p to the stream, dropping the OLDEST retained bytes
// when the window would exceed cap. It always accepts everything (the
// child's pipe copier can never block on retention) and returns len(p).
func (b *outputBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, p...)
	if overflow := int64(len(b.data)) - b.limit; overflow > 0 {
		b.data = b.data[overflow:]
		b.base += overflow
		b.dropped += overflow
	}
	b.written += int64(len(p))
	return len(p), nil
}

// read serves up to max bytes at absolute offset. nextOffset arithmetic
// is exact: offset+len(served) lands at most at written (EOF).
func (b *outputBuffer) read(jobID string, offset, max int64) (string, int64, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if offset < b.base {
		return "", 0, false, &OutputEvictedError{JobID: jobID, Base: b.base, Evicted: b.dropped}
	}
	if offset > b.written {
		return "", 0, false, &OutputAheadError{JobID: jobID, Written: b.written}
	}
	end := b.written
	if max > 0 && offset+max < end {
		end = offset + max
	}
	served := string(b.data[offset-b.base : end-b.base])
	return served, offset + int64(len(served)), end < b.written, nil
}

// stats reports the cumulative written/evicted counters (for the
// empty-buffer early return in ReadOutput).
func (b *outputBuffer) stats() (written, evicted int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.written, b.dropped
}

// writerFor returns the job's output writer, creating the retention
// buffer on first use (a queued job has no buffer until its body
// starts producing; a recovered pre-restart job may never get one).
func (m *Manager) writerFor(jobID string) io.Writer {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.outputs == nil {
		m.outputs = make(map[string]*outputBuffer)
	}
	b, ok := m.outputs[jobID]
	if !ok {
		b = newOutputBuffer(m.outputRetention)
		m.outputs[jobID] = b
	}
	return b
}

// ReadOutput serves one bounded chunk of the job's captured output at
// offset (the jobs/output seam). The state is the fold-derived job
// state; a job with no buffer yet (queued, or recovered from a
// pre-restart log) is an honest empty read at offset 0, while an
// offset beyond what this process produced (offset > 0 against no
// buffer ⇒ beyond written=0) is the typed ahead error — same family,
// same wire mapping as the buffered branch (§4g).
func (m *Manager) ReadOutput(jobID string, offset int64) (OutputChunk, error) {
	if offset < 0 {
		return OutputChunk{}, fmt.Errorf("jobs: offset must be >= 0 (got %d)", offset)
	}
	m.mu.Lock()
	rec, ok := m.jobs[jobID]
	if !ok {
		m.mu.Unlock()
		return OutputChunk{}, &UnknownJobError{JobID: jobID}
	}
	state := OutputStateQueued
	switch {
	case rec.settledSeq != 0:
		state = OutputStateSettled
	case rec.startedSeq != 0:
		state = OutputStateRunning
	}
	buf := m.outputs[jobID]
	m.mu.Unlock()

	if buf == nil {
		// No in-memory ring: the job has produced no output in THIS
		// process (queued, or recovered from a pre-restart log — the
		// disclosed non-durable retention). Offset 0 is the honest
		// empty read (written:0 IS that posture); a stale pre-restart
		// offset is beyond written=0, so it gets the SAME typed ahead
		// error the buffered branch serves (review hotfix 2 / B1) —
		// the client's ahead-clamp then completes the drain with the
		// honest-absence note.
		if offset > 0 {
			return OutputChunk{}, &OutputAheadError{JobID: jobID, Written: 0}
		}
		return OutputChunk{
			JobID: jobID, State: state, Chunk: "",
			Offset: offset, NextOffset: offset, HasMore: false,
			Written: 0, EvictedBytes: 0,
		}, nil
	}
	chunk, next, more, err := buf.read(jobID, offset, DefaultOutputChunkBytes)
	if err != nil {
		return OutputChunk{}, err
	}
	written, evicted := buf.stats()
	return OutputChunk{
		JobID: jobID, State: state, Chunk: chunk,
		Offset: offset, NextOffset: next, HasMore: more,
		Written: written, EvictedBytes: evicted,
	}, nil
}

// SettleWithDetail records the terminal state like Settle, with a
// compact terminal detail string (e.g. the background shell's exit
// facts) riding job/settled and job/report payloads. Settlement is
// first-wins exactly as Settle.
func (m *Manager) SettleWithDetail(jobID string, runErr error, detail string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.jobs[jobID]
	if !ok {
		return fmt.Errorf("jobs: settle unknown job %q", jobID)
	}
	if rec.settledSeq != 0 {
		return nil // first-wins: the terminal event already landed
	}
	result, reason := session.JobResultCompleted, ""
	if runErr != nil {
		result, reason = session.JobResultFailed, runErr.Error()
	}
	ev, err := m.lg.Append(session.TypeJobSettled, nil, session.JobPayload{
		JobID: rec.job.ID, Kind: rec.job.Kind, Owner: rec.job.Owner,
		Result: result, Reason: reason, Detail: detail,
	})
	if err != nil {
		return fmt.Errorf("jobs: log job/settled: %w", err)
	}
	rec.settledSeq = ev.Seq
	rec.settleResult = result
	rec.settleReason = reason
	rec.settleDetail = detail
	return nil
}
