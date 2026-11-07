// output_test.go — the P6 job-output capture seam: ring retention,
// offset-cursor reads (never re-serve, exact nextOffset arithmetic),
// typed read errors, the OutputExecutor wiring, SettleWithDetail, and
// concurrent reader/writer safety (run under -race by the suite).
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- outputBuffer unit semantics ----------------------------------------------

func TestOutputBuffer_CursorNeverReserves(t *testing.T) {
	b := newOutputBuffer(1024)
	if _, err := b.Write([]byte("hello world")); err != nil {
		t.Fatal(err)
	}
	// Two disjoint tails: read at 0, then at the returned cursor; the
	// second chunk must start exactly where the first ended.
	first, next1, more, err := b.read("j", 0, 4)
	if err != nil || first != "hell" || next1 != 4 || !more {
		t.Fatalf("first read = %q next=%d more=%v err=%v", first, next1, more, err)
	}
	second, next2, more2, err := b.read("j", next1, 4)
	if err != nil || second != "o wo" || next2 != 8 || !more2 {
		t.Fatalf("second read = %q next=%d more=%v err=%v", second, next2, more2, err)
	}
	// Re-reading the SAME offset returns the SAME bytes (idempotent
	// re-read is fine — the contract forbids the CURSOR going stale or
	// the stream re-serving via a moving window, not deterministic
	// re-reads).
	again, _, _, err := b.read("j", 0, 4)
	if err != nil || again != "hell" {
		t.Fatalf("re-read = %q err=%v", again, err)
	}
	// A read at EOF is the honest empty poll; a read BEYOND it is the
	// typed ahead error (a client arithmetic bug — nextOffset never
	// exceeds written).
	if eof, _, _, err := b.read("j", 11, 4); err != nil || eof != "" {
		t.Fatalf("eof read = %q err=%v", eof, err)
	}
	if _, _, _, err := b.read("j", 12, 4); err == nil {
		t.Fatal("read past written must fail")
	} else {
		var ahead *OutputAheadError
		if !errors.As(err, &ahead) || ahead.Written != 11 {
			t.Fatalf("err = %v, want OutputAheadError{Written:11}", err)
		}
	}
}

func TestOutputBuffer_RetentionEvictsOldest(t *testing.T) {
	b := newOutputBuffer(8)
	_, _ = b.Write([]byte("0123456789")) // 10 bytes into an 8-byte window
	written, evicted := b.stats()
	if written != 10 || evicted != 2 {
		t.Fatalf("stats = %d/%d, want 10/2", written, evicted)
	}
	// Offset 0 is behind the window: typed evicted error naming base 2.
	var ev *OutputEvictedError
	if _, _, _, err := b.read("j", 0, 4); err == nil {
		t.Fatal("stale offset must fail")
	} else if !errors.As(err, &ev) || ev.Base != 2 || ev.Evicted != 2 {
		t.Fatalf("err = %v, want OutputEvictedError{Base:2,Evicted:2}", err)
	}
	// The tail is still served: base..written.
	chunk, next, _, err := b.read("j", 2, 100)
	if err != nil || chunk != "23456789" || next != 10 {
		t.Fatalf("tail read = %q next=%d err=%v", chunk, next, err)
	}
}

func TestOutputBuffer_ChunkBoundHasMore(t *testing.T) {
	b := newOutputBuffer(1024)
	_, _ = b.Write([]byte(strings.Repeat("x", 100)))
	chunk, next, more, err := b.read("j", 0, 16)
	if err != nil || len(chunk) != 16 || next != 16 || !more {
		t.Fatalf("bounded read len=%d next=%d more=%v err=%v", len(chunk), next, more, err)
	}
	// Final short page lands exactly at EOF, never past it.
	last, next2, more2, err := b.read("j", 96, 16)
	if err != nil || len(last) != 4 || next2 != 100 || more2 {
		t.Fatalf("final page len=%d next=%d more=%v err=%v", len(last), next2, more2, err)
	}
	// At EOF: empty window, not an error, not hasMore.
	empty, next3, more3, err := b.read("j", 100, 16)
	if err != nil || empty != "" || next3 != 100 || more3 {
		t.Fatalf("eof read = %q next=%d more=%v err=%v", empty, next3, more3, err)
	}
}

// --- Manager.ReadOutput --------------------------------------------------------

func TestReadOutput_UnknownJob(t *testing.T) {
	m, _ := testManager(t, "s-out-1", &quickExec{}, Options{})
	var uj *UnknownJobError
	_, err := m.ReadOutput("nope-1", 0)
	if !errors.As(err, &uj) {
		t.Fatalf("err = %v, want UnknownJobError", err)
	}
}

func TestReadOutput_QueuedJobIsEmptyNotError(t *testing.T) {
	// The "not-yet" posture: a dispatched-but-queued job answers an
	// honest empty chunk carrying its state — polling is the intended
	// use, so this must not be an error.
	m, _ := testManager(t, "s-out-2", &blockingExec{}, Options{MaxInFlightPerOwner: 1})
	if _, err := m.Dispatch("slow", nil); err != nil {
		t.Fatal(err)
	}
	// Occupy the single slot so the second job stays queued.
	occ, err := m.Dispatch("slow", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Let the first job start (block in Run) so the second is truly
	// queued behind the semaphore. Deterministic wait on the snapshot.
	waitFor(t, 5*time.Second, func() bool {
		for _, st := range m.Snapshot() {
			if st.JobID == occ.JobID && st.State == StateQueued {
				return true
			}
		}
		return false
	}, "the second job to sit queued")
	ch, err := m.ReadOutput(occ.JobID, 0)
	if err != nil {
		t.Fatalf("queued read must not error: %v", err)
	}
	if ch.State != OutputStateQueued || ch.Chunk != "" || ch.NextOffset != 0 || ch.HasMore {
		t.Fatalf("queued chunk = %+v", ch)
	}
}

// streamingExec is an OutputExecutor fixture: writes lines to the job's
// output writer, optionally pausing between them.
type streamingExec struct {
	lines   []string
	pause   time.Duration
	release chan struct{} // optional external gate before the final line
	detail  string
}

func (e *streamingExec) Run(_ context.Context, _ Job) error { return nil }

func (e *streamingExec) RunWithOutput(_ context.Context, _ Job, out io.Writer) (string, error) {
	for i, ln := range e.lines {
		if i == len(e.lines)-1 && e.release != nil {
			<-e.release
		}
		if _, err := io.WriteString(out, ln); err != nil {
			return "", err
		}
		if e.pause > 0 {
			time.Sleep(e.pause)
		}
	}
	return e.detail, nil
}

func TestReadOutput_ProgressiveTailAndSettlement(t *testing.T) {
	release := make(chan struct{})
	exec := &streamingExec{
		lines:   []string{"alpha\n", "beta\n", "gamma\n"},
		release: release,
		detail:  "cause=exit exitCode=0",
	}
	m, _ := testManager(t, "s-out-3", exec, Options{})
	r, err := m.Dispatch("stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Mid-flight: wait until the first two lines landed while the job
	// is still gated before the final line.
	waitFor(t, 5*time.Second, func() bool {
		ch, err := m.ReadOutput(r.JobID, 0)
		return err == nil && strings.Contains(ch.Chunk, "beta")
	}, "the first two output lines mid-flight")
	ch, err := m.ReadOutput(r.JobID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if ch.State != OutputStateRunning || !strings.HasPrefix(ch.Chunk, "alpha\nbeta\n") {
		t.Fatalf("mid-flight chunk = %+v", ch)
	}
	// The cursor advance is exact: continue at nextOffset.
	rest, err := m.ReadOutput(r.JobID, ch.NextOffset)
	if err != nil {
		t.Fatal(err)
	}
	if rest.Chunk != "" || rest.NextOffset != ch.NextOffset || rest.State != OutputStateRunning {
		t.Fatalf("empty continuation = %+v", rest)
	}
	close(release)
	waitFor(t, 5*time.Second, func() bool {
		for _, st := range m.Snapshot() {
			if st.JobID == r.JobID && st.State == StateSettled {
				return true
			}
		}
		return false
	}, "the gated job to settle")
	// Post-settle reads keep serving the tail within retention.
	final, err := m.ReadOutput(r.JobID, ch.NextOffset)
	if err != nil {
		t.Fatal(err)
	}
	if final.Chunk != "gamma\n" || final.State != OutputStateSettled || final.HasMore {
		t.Fatalf("post-settle chunk = %+v", final)
	}
	if final.NextOffset != final.Written {
		t.Fatalf("cursor %d must equal written %d at EOF", final.NextOffset, final.Written)
	}
}

func TestSettleWithDetail_RidesSettledAndReport(t *testing.T) {
	exec := &streamingExec{lines: []string{"x"}, detail: "cause=exit exitCode=0 durationMs=7"}
	m, sink := testManager(t, "s-out-4", exec, Options{})
	r, err := m.Dispatch("stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	m.Drain()
	if _, err := m.EmitReports(); err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(sink)
	if !strings.Contains(string(raw), `"detail":"cause=exit exitCode=0 durationMs=7"`) {
		t.Fatalf("detail missing from the durable events:\n%s", raw)
	}
	// The fold reproduces settleDetail (report-after-resume parity).
	recs, _, _, _, err := foldJobs(m.lg.Events())
	if err != nil {
		t.Fatal(err)
	}
	if recs[r.JobID].settleDetail != "cause=exit exitCode=0 durationMs=7" {
		t.Fatalf("fold settleDetail = %q", recs[r.JobID].settleDetail)
	}
}

// TestOutputCursor_NeverReservesTwoTailsDisjoint is the crux-(c) seam
// proof at the Manager level: consecutive tails return disjoint chunks
// whose concatenation is exactly the produced stream, and the
// nextOffset arithmetic is byte-exact end to end across chunk paging
// (the producer emits more than one chunk bound of output).
func TestOutputCursor_NeverReservesTwoTailsDisjoint(t *testing.T) {
	const line = "0123456789abcdef\n" // 17 bytes
	const n = 2000                    // 34_000 bytes ≈ 3 chunks at 16 KiB
	lines := make([]string, n)
	for i := range lines {
		lines[i] = line
	}
	exec := &streamingExec{lines: lines}
	m, _ := testManager(t, "s-out-5", exec, Options{})
	r, err := m.Dispatch("stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	m.Drain()

	var reassembled strings.Builder
	offset := int64(0)
	reads := 0
	for {
		ch, err := m.ReadOutput(r.JobID, offset)
		if err != nil {
			t.Fatalf("read at %d: %v", offset, err)
		}
		if ch.Offset != offset {
			t.Fatalf("chunk offset %d != requested %d", ch.Offset, offset)
		}
		if ch.NextOffset != offset+int64(len(ch.Chunk)) {
			t.Fatalf("nextOffset %d != offset+len(chunk) %d", ch.NextOffset, offset+int64(len(ch.Chunk)))
		}
		if len(ch.Chunk) > DefaultOutputChunkBytes {
			t.Fatalf("chunk len %d exceeds the chunk bound %d", len(ch.Chunk), DefaultOutputChunkBytes)
		}
		reassembled.WriteString(ch.Chunk)
		reads++
		if !ch.HasMore {
			if ch.NextOffset != ch.Written {
				t.Fatalf("terminal cursor %d != written %d", ch.NextOffset, ch.Written)
			}
			break
		}
		offset = ch.NextOffset
	}
	want := strings.Repeat(line, n)
	if reassembled.String() != want {
		t.Fatalf("reassembled %d bytes != produced %d bytes", reassembled.Len(), len(want))
	}
	if reads < 2 {
		t.Fatalf("expected paging across multiple reads, got %d", reads)
	}
}

// TestOutput_ConcurrentReaderWriter is the -race proof: a producer
// writing while N readers page with fresh cursors must never race, never
// serve inconsistent arithmetic, and end byte-exact.
func TestOutput_ConcurrentReaderWriter(t *testing.T) {
	const produced = "line-0000\n"
	lines := make([]string, 200)
	for i := range lines {
		lines[i] = produced
	}
	exec := &streamingExec{lines: lines, pause: 150 * time.Microsecond}
	m, _ := testManager(t, "s-out-6", exec, Options{})
	r, err := m.Dispatch("stream", nil)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	readAll := func() {
		defer wg.Done()
		var got strings.Builder
		offset := int64(0)
		for {
			ch, err := m.ReadOutput(r.JobID, offset)
			if err != nil {
				t.Errorf("read at %d: %v", offset, err)
				return
			}
			got.WriteString(ch.Chunk)
			offset = ch.NextOffset
			if ch.State == OutputStateSettled && !ch.HasMore && ch.NextOffset >= ch.Written {
				if got.String() != strings.Repeat(produced, 200) {
					t.Errorf("reader reassembly mismatch (%d bytes)", got.Len())
				}
				return
			}
			time.Sleep(300 * time.Microsecond)
		}
	}
	wg.Add(3)
	go readAll()
	go readAll()
	go readAll()
	m.Drain()
	wg.Wait()
}

// TestOutput_RecoveredJobHasNoBuffer pins the non-durability posture:
// a job recovered from a pre-restart log reports written=0 at offset 0
// — its captured output did not survive (settlement facts did) — while
// a read at a stale pre-restart offset (offset > 0 ⇒ beyond written=0)
// is the typed ahead error, exactly as the buffered branch types it
// (§4g: nextOffset never exceeds written). Review hotfix 2 (B1).
func TestOutput_RecoveredJobHasNoBuffer(t *testing.T) {
	m, sink := testManager(t, "s-out-7", &quickExec{}, Options{})
	r, err := m.Dispatch("quick", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	m.Drain()
	m.Stop()

	path := filepath.Join(t.TempDir(), "recovered.jsonl")
	if err := os.WriteFile(path, sink.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	m2, _, err := Recover(path, &quickExec{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer m2.Stop()
	ch, err := m2.ReadOutput(r.JobID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if ch.Written != 0 || ch.Chunk != "" || ch.State != OutputStateSettled {
		t.Fatalf("recovered job read = %+v, want empty settled read", ch)
	}
	// B1: a stale pre-restart cursor is typed as ahead-of-output with
	// the written:0 clamp hint — NOT a success at any offset. The
	// client's ahead-clamp then completes the drain (driver-side test).
	var ahead *OutputAheadError
	_, err = m2.ReadOutput(r.JobID, 1)
	if !errors.As(err, &ahead) || ahead.Written != 0 {
		t.Fatalf("recovered read at offset 1: err = %v, want OutputAheadError{Written:0}", err)
	}
}
