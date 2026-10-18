// driver_tailresync_test.go — review hotfix F2 (red first): the drain
// loop's cursor re-sync on the typed jobs/output errors whose
// error.data carries a re-sync hint (host-protocol §4g):
//
//   - "output-evicted" — the retention ring dropped the bytes before
//     data.evictedBase; the client must jump FORWARD to the oldest
//     retained byte and keep paging, so a SETTLED job whose early
//     output fell behind retention still completes its drain (retained
//     tail rendered, session/surface flushed) instead of warning once
//     per poll until bgWaitMax;
//   - "output-ahead" — cursor beyond written (client arithmetic bug);
//     clamp BACK to the server's written and keep paging.
//
// Seam: the REAL protocol.Client over a net.Pipe against a scripted
// fake daemon. d.client is the concrete *protocol.Client, so the
// honest fake is the SERVER side — which also exercises the real wire
// decode of *protocol.Error.Data on the re-sync path. The fake's ring
// arithmetic mirrors internal/jobs/output.go exactly: base == the
// cumulative evicted count, 16 KiB pages, exact nextOffset.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/protocol"
)

const (
	fakeRetentionBytes = 256 * 1024 // mirrors jobs.DefaultOutputRetentionBytes
	fakeChunkBytes     = 16 * 1024  // mirrors jobs.DefaultOutputChunkBytes
)

// fakeTailDaemon is a scripted jobs/status + jobs/output +
// session/surface responder over one net.Pipe, with real ring
// arithmetic over a fully pre-produced stream (the wedge scenario: the
// producer wrapped past the client's cursor BEFORE the first poll).
type fakeTailDaemon struct {
	conn    io.ReadWriteCloser
	jobID   string
	data    []byte // retained window (the most recent retention bytes)
	base    int64  // absolute offset of data[0] (== cumulative evicted)
	written int64

	mu           sync.Mutex
	outputCalls  int
	evictedErrs  int
	aheadErrs    int
	surfaceCalls int

	done chan struct{}
}

// newFakeTailDaemon materializes a settled job that produced the
// given byte count of uniform 'A' output and retained only the last
// retention window of it.
func newFakeTailDaemon(jobID string, produced int64) (io.ReadWriteCloser, *fakeTailDaemon) {
	total := int(produced)
	start := 0
	if total > fakeRetentionBytes {
		start = total - fakeRetentionBytes
	}
	c, s := net.Pipe()
	f := &fakeTailDaemon{
		conn:    s,
		jobID:   jobID,
		data:    bytes.Repeat([]byte{'A'}, total-start),
		base:    int64(start),
		written: produced,
		done:    make(chan struct{}),
	}
	go f.serve()
	return c, f
}

func (f *fakeTailDaemon) serve() {
	defer close(f.done)
	r := bufio.NewReader(f.conn)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return // transport closed (test teardown or client exit)
		}
		var req struct {
			ID     int64           `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal([]byte(strings.TrimSpace(line)), &req) != nil || req.Method == "" {
			continue
		}
		switch req.Method {
		case "jobs/status":
			f.respond(req.ID, map[string]any{
				"jobs": []map[string]any{{"jobId": f.jobID, "state": "settled"}},
			})
		case "jobs/output":
			f.serveOutput(req.ID, req.Params)
		case "session/surface":
			f.mu.Lock()
			f.surfaceCalls++
			f.mu.Unlock()
			f.respond(req.ID, map[string]any{"ok": true})
		default:
			f.respondErr(req.ID, protocol.ErrMethodNotFound, "method not found: "+req.Method, nil)
		}
	}
}

// serveOutput answers one offset-cursor read with the same typed
// error contract as the real handler (internal/protocol/handlers.go):
// -32602 with data {kind:"output-evicted", evictedBase, evicted} or
// {kind:"output-ahead", written}.
func (f *fakeTailDaemon) serveOutput(id int64, params json.RawMessage) {
	var p struct {
		JobID  string `json:"jobId"`
		Offset int64  `json:"offset"`
	}
	_ = json.Unmarshal(params, &p)
	f.mu.Lock()
	f.outputCalls++
	f.mu.Unlock()
	switch {
	case p.Offset < f.base:
		f.mu.Lock()
		f.evictedErrs++
		f.mu.Unlock()
		data, _ := json.Marshal(map[string]any{
			"kind": "output-evicted", "evictedBase": f.base, "evicted": f.base,
		})
		f.respondErr(id, protocol.ErrInvalidParams, fmt.Sprintf(
			"jobs: offset behind the retention window of %s: %d bytes evicted, oldest readable offset is %d",
			f.jobID, f.base, f.base), data)
	case p.Offset > f.written:
		f.mu.Lock()
		f.aheadErrs++
		f.mu.Unlock()
		data, _ := json.Marshal(map[string]any{
			"kind": "output-ahead", "written": f.written,
		})
		f.respondErr(id, protocol.ErrInvalidParams, fmt.Sprintf(
			"jobs: offset %d is ahead of the produced output (%d bytes written)",
			p.Offset, f.written), data)
	default:
		end := f.written
		if p.Offset+fakeChunkBytes < end {
			end = p.Offset + fakeChunkBytes
		}
		f.respond(id, map[string]any{
			"jobId": f.jobID, "state": "settled",
			"chunk":  string(f.data[p.Offset-f.base : end-f.base]),
			"offset": p.Offset, "nextOffset": end,
			"hasMore": end < f.written, "written": f.written,
			"evictedBytes": f.base,
		})
	}
}

func (f *fakeTailDaemon) respond(id int64, result any) {
	b, err := json.Marshal(result)
	if err != nil {
		return
	}
	line, _ := protocol.MarshalResponse(id, b)
	_, _ = f.conn.Write(append(line, '\n'))
}

func (f *fakeTailDaemon) respondErr(id int64, code int, msg string, data []byte) {
	line, _ := protocol.MarshalResponseError(id, code, msg, data)
	_, _ = f.conn.Write(append(line, '\n'))
}

func (f *fakeTailDaemon) stats() (outputCalls, evictedErrs, aheadErrs, surfaceCalls int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.outputCalls, f.evictedErrs, f.aheadErrs, f.surfaceCalls
}

// recordingRenderer captures every job-output tail record the drain
// loop renders (the Renderer seam the driver hands chunks to).
type recordingRenderer struct {
	mu      sync.Mutex
	records []JobOutputRecord
}

func (r *recordingRenderer) RenderEvent(json.RawMessage)         {}
func (r *recordingRenderer) RenderApproval(json.RawMessage)      {}
func (r *recordingRenderer) RenderProtocolError(json.RawMessage) {}

func (r *recordingRenderer) RenderJobOutput(rec JobOutputRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, rec)
}

func (r *recordingRenderer) LastTurnEnd() (string, bool) { return "", false }
func (r *recordingRenderer) ResetTurnEnd()               {}

func (r *recordingRenderer) snapshot() []JobOutputRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]JobOutputRecord, len(r.records))
	copy(out, r.records)
	return out
}

// tailDrainDriver wires a driver whose only live surface is the drain
// loop: no conversation, bgJobs seeded directly with the fake's job.
func tailDrainDriver(conn io.ReadWriteCloser, rec Renderer, errw *bytes.Buffer, cursor int64) *driver {
	d := &driver{
		cfg:      &Config{},
		client:   protocol.NewClient(conn),
		renderer: rec,
		out:      io.Discard,
		errw:     errw,
	}
	d.bgMu.Lock()
	d.bgJobs = map[string]*bgTail{"shell-1": {cursor: cursor}}
	d.bgMu.Unlock()
	return d
}

// runDrain executes drainBackgroundJobs bounded FAR below bgWaitMax,
// so a cursor wedge FAILS the test instead of hanging it for 10
// minutes (the pre-fix red shape: the poll loop spins on the evicted
// error until the deadline).
func runDrain(t *testing.T, d *driver, daemonDone <-chan struct{}) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		d.drainBackgroundJobs()
		close(done)
	}()
	select {
	case <-done:
		return
	case <-time.After(8 * time.Second):
		_ = d.client.Close() // unblock the poll loop (isDone) + the fake's reads
		select {
		case <-daemonDone:
		case <-time.After(2 * time.Second):
		}
		t.Fatalf("drain did not complete within 8s (bgWaitMax is 10m) — the per-job cursor never re-synced past the typed jobs/output error")
	}
}

// TestTailJobEvictedResyncCompletesDrain is the F2 unit (red first):
// a SETTLED job whose cursor sat at 0 while the producer wrapped past
// retention (300000 produced, 37856 evicted) must, on the typed
// output-evicted -32602, re-sync the cursor to data.evictedBase, page
// the retained window, and COMPLETE the drain — retained tail
// rendered, terminal record, session/surface flushed — with the
// evicted condition served exactly once (no warning loop).
//
// B-F1 (hotfix 3): the final settled-and-fully-consumed read CARRIES
// bytes (the 16th retained page), so this test is also the pin for
// §4g's uniform terminal contract — the ONE-TIME empty terminal record
// follows that bytes-carrying read too (16 full pages, then the empty
// marker at written), not only the empty-read path.
func TestTailJobEvictedResyncCompletesDrain(t *testing.T) {
	const produced = int64(300000) // 300000 - 262144 = 37856 evicted
	const evicted = produced - fakeRetentionBytes
	conn, f := newFakeTailDaemon("shell-1", produced)
	rec := &recordingRenderer{}
	var errbuf bytes.Buffer
	d := tailDrainDriver(conn, rec, &errbuf, 0)
	runDrain(t, d, f.done)

	oc, ee, ae, sc := f.stats()
	_ = oc
	if ae != 0 {
		t.Fatalf("output-ahead errors served = %d, want 0", ae)
	}
	if ee != 1 {
		t.Fatalf("evicted errors served = %d, want exactly 1 (the re-sync must clear the condition instead of re-polling it)", ee)
	}
	if got := strings.Count(errbuf.String(), "warning: jobs/output"); got != 0 {
		t.Fatalf("%d warning:jobs/output line(s) after the re-sync — the drain must not warn-loop:\n%s", got, errbuf.String())
	}
	if !strings.Contains(errbuf.String(), "oldest retained byte") {
		t.Fatalf("the one-time re-sync note (honest absence) is missing from stderr:\n%s", errbuf.String())
	}
	if sc != 1 {
		t.Fatalf("session/surface calls = %d, want 1 (the report flush after a completed drain)", sc)
	}

	recs := rec.snapshot()
	if len(recs) != 17 {
		t.Fatalf("tail records = %d, want 16 retained-window pages (262144/16384) + the one-time empty terminal record (B-F1: it follows the bytes-carrying settled read too)", len(recs))
	}
	if recs[0].Offset != evicted {
		t.Fatalf("first rendered record offset = %d, want evictedBase %d (the re-sync)", recs[0].Offset, evicted)
	}
	var sum int64
	for i, r := range recs {
		if r.State != "settled" {
			t.Fatalf("record %d state = %q, want settled", i, r.State)
		}
		if r.EvictedBytes != evicted {
			t.Fatalf("record %d evictedBytes = %d, want %d (the structural honest-absence signal)", i, r.EvictedBytes, evicted)
		}
		if i > 0 && r.Offset != recs[i-1].NextOffset {
			t.Fatalf("cursor chain broken at %d: offset %d != previous nextOffset %d", i, r.Offset, recs[i-1].NextOffset)
		}
		if i < len(recs)-1 && len(r.Chunk) != fakeChunkBytes {
			t.Fatalf("record %d chunk = %d bytes, want a full %d-byte retained page (only the terminal record may be empty)", i, len(r.Chunk), fakeChunkBytes)
		}
		sum += int64(len(r.Chunk))
	}
	last := recs[len(recs)-1]
	if last.Chunk != "" || last.Offset != produced || last.HasMore || last.NextOffset != produced {
		t.Fatalf("terminal record = %+v, want the one-time empty marker at written (%d): chunk empty, offset == nextOffset == written, hasMore false", last, produced)
	}
	if sum != fakeRetentionBytes {
		t.Fatalf("rendered %d tail bytes, want the full retained window %d", sum, fakeRetentionBytes)
	}
}

// TestTailJobAheadClampResyncsCursor covers the sibling typed error
// sharing the re-sync branch: a cursor AHEAD of the produced output
// (client arithmetic bug) clamps back to the server's written and the
// drain still completes with the empty terminal record.
func TestTailJobAheadClampResyncsCursor(t *testing.T) {
	const produced = int64(5000)
	conn, f := newFakeTailDaemon("shell-1", produced)
	rec := &recordingRenderer{}
	var errbuf bytes.Buffer
	d := tailDrainDriver(conn, rec, &errbuf, 6000) // stale/bugged cursor
	runDrain(t, d, f.done)

	_, ee, ae, sc := f.stats()
	if ee != 0 {
		t.Fatalf("evicted errors served = %d, want 0", ee)
	}
	if ae != 1 {
		t.Fatalf("output-ahead errors served = %d, want exactly 1 (the clamp must clear the condition)", ae)
	}
	if got := strings.Count(errbuf.String(), "warning: jobs/output"); got != 0 {
		t.Fatalf("%d warning:jobs/output line(s) — the clamp must not warn-loop:\n%s", got, errbuf.String())
	}
	if sc != 1 {
		t.Fatalf("session/surface calls = %d, want 1", sc)
	}
	recs := rec.snapshot()
	if len(recs) != 1 {
		t.Fatalf("tail records = %d, want 1 (the empty terminal record at written)", len(recs))
	}
	r := recs[0]
	if r.Chunk != "" || r.Offset != produced || r.NextOffset != produced || r.State != "settled" || r.HasMore {
		t.Fatalf("terminal record = %+v, want the empty settled record at written=%d", r, produced)
	}
}

// TestTailJobRecoveredAheadClampCompletesDrain pins the B1 pairing on
// the client side: a daemon restart mid-drain recovers the job with
// written:0 (non-durable retention), so the client's stale pre-restart
// cursor draws the typed output-ahead error with the written:0 hint —
// the one-shot clamp must complete the drain (empty terminal record at
// 0, honest-absence note, session/surface flush) instead of warning.
func TestTailJobRecoveredAheadClampCompletesDrain(t *testing.T) {
	conn, f := newFakeTailDaemon("shell-1", 0) // recovered job: written=0, nothing retained
	rec := &recordingRenderer{}
	var errbuf bytes.Buffer
	d := tailDrainDriver(conn, rec, &errbuf, 7000) // stale pre-restart cursor
	runDrain(t, d, f.done)

	_, ee, ae, sc := f.stats()
	if ee != 0 {
		t.Fatalf("evicted errors served = %d, want 0", ee)
	}
	if ae != 1 {
		t.Fatalf("output-ahead errors served = %d, want exactly 1 (the clamp must clear the condition)", ae)
	}
	if got := strings.Count(errbuf.String(), "warning: jobs/output"); got != 0 {
		t.Fatalf("%d warning:jobs/output line(s) — the clamp must not warn-loop:\n%s", got, errbuf.String())
	}
	if !strings.Contains(errbuf.String(), "re-syncing back to written 0") {
		t.Fatalf("the honest-absence clamp note is missing from stderr:\n%s", errbuf.String())
	}
	if sc != 1 {
		t.Fatalf("session/surface calls = %d, want 1", sc)
	}
	recs := rec.snapshot()
	if len(recs) != 1 {
		t.Fatalf("tail records = %d, want 1 (the empty terminal record at 0)", len(recs))
	}
	r := recs[0]
	if r.Chunk != "" || r.Offset != 0 || r.NextOffset != 0 || r.Written != 0 || r.State != "settled" || r.HasMore {
		t.Fatalf("terminal record = %+v, want the empty settled record at written=0", r)
	}
}
