// compaction_race_test.go — TA-R1 regression tests: Compact's
// fold→summarize→append sequence and ReplaceGeneration's fold must
// observe the Log's concurrency contract (writeEvent appends under the
// lock; readers snapshot). Run with -race — the data race on the live
// event list IS the red signal — but the assertions (seq contiguity,
// bracket atomicity, citation consistency via Unfold, byte-deterministic
// replay of the raced log) hold without it too.
package session

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// raceSummarizer is a deterministic stub: 1 char, always admissible
// against spans of ordinary prompt messages.
func raceSummarizer(shadowed []Message) (string, error) { return "s", nil }

// tolerableRefusal reports whether a Compact error is a LEGAL refusal
// under concurrency (nothing big enough to compact, or a degenerate
// shadowed span no larger than the stub summary) rather than a contract
// violation.
func tolerableRefusal(err error) bool {
	var stl *SummaryTooLargeError
	if errors.As(err, &stl) {
		return true
	}
	return strings.Contains(err.Error(), "nothing to compact")
}

// TestCompactConcurrentWithAppends is the TA-R1 admission test: one
// goroutine running Compact in a loop while writer goroutines append
// turns. After quiescence the log must satisfy: contiguous 1-based seq,
// non-interleaved compaction brackets (start→summary→end triplets),
// citations that Unfold can verify against the pre-summary fold, and a
// byte-identical surface when the raced log is replayed.
func TestCompactConcurrentWithAppends(t *testing.T) {
	var sink jobSink
	lg := jobTestLog(t, "sess-race-1", &sink)
	defer lg.Close()

	// Seed enough surface that compaction always has a shadowable span.
	for i := 0; i < 6; i++ {
		if _, err := lg.AppendPrompt(fmt.Sprintf("seed-%02d", i)); err != nil {
			t.Fatalf("seed AppendPrompt: %v", err)
		}
	}

	const writers = 4
	const each = 40
	done := make(chan struct{})
	var sumMu sync.Mutex
	var compactResults []CompactionResult

	var appendWG sync.WaitGroup
	for w := 0; w < writers; w++ {
		appendWG.Add(1)
		go func(w int) {
			defer appendWG.Done()
			for i := 0; i < each; i++ {
				if _, err := lg.AppendPrompt(fmt.Sprintf("body-%02d-%02d", w, i)); err != nil {
					t.Errorf("AppendPrompt: %v", err)
					return
				}
			}
		}(w)
	}

	var compactWG sync.WaitGroup
	compactWG.Add(1)
	go func() {
		defer compactWG.Done()
		for {
			select {
			case <-done:
				return
			default:
			}
			res, err := lg.Compact(raceSummarizer, CompactOptions{Reason: "race-test"})
			if err == nil {
				sumMu.Lock()
				compactResults = append(compactResults, res)
				sumMu.Unlock()
				continue
			}
			if !tolerableRefusal(err) {
				t.Errorf("Compact: %v", err)
				return
			}
		}
	}()

	appendWG.Wait()
	close(done)
	compactWG.Wait()

	if len(compactResults) == 0 {
		t.Fatal("no successful compaction ran — the raced path was not exercised")
	}

	events := lg.Events()

	// Invariant 1: seq is contiguous and monotone from 1.
	for i, ev := range events {
		if ev.Seq != int64(i+1) {
			t.Fatalf("seq contiguity broken at index %d: seq %d", i, ev.Seq)
		}
	}

	// Invariant 2: each SUCCESSFUL compaction's bracket triplet
	// (start → summary → end) landed CONTIGUOUSLY in seq order — no
	// concurrent append or other compaction slipped between the three
	// events of one run (the write-locked fold+append sequence
	// guarantees it; the pre-fix per-append locking did not). Unmatched
	// starts from REFUSED compactions are legal (the durable crash-lock)
	// and are not asserted against. Successful compactions serialize:
	// their seq ranges are disjoint and ordered.
	sumMu.Lock()
	results := append([]CompactionResult(nil), compactResults...)
	sumMu.Unlock()
	prevEnd := int64(0)
	for _, r := range results {
		if r.StartSeq <= prevEnd {
			t.Fatalf("compaction at seq %d overlaps the previous bracket (ends %d)", r.StartSeq, prevEnd)
		}
		if r.SummarySeq != r.StartSeq+1 || r.EndSeq != r.SummarySeq+1 {
			t.Fatalf("compaction bracket not contiguous: start=%d summary=%d end=%d", r.StartSeq, r.SummarySeq, r.EndSeq)
		}
		if events[r.StartSeq-1].Type != TypeCompactionStart ||
			events[r.SummarySeq-1].Type != TypeCompactionSummary ||
			events[r.EndSeq-1].Type != TypeCompactionEnd {
			t.Fatalf("compaction bracket types wrong at start=%d", r.StartSeq)
		}
		prevEnd = r.EndSeq
	}

	// Invariant 3: every summary's citations verify against the
	// pre-summary fold (Unfold re-derives and checks them).
	for _, r := range results {
		if _, err := Unfold(events, r.SummarySeq); err != nil {
			t.Fatalf("Unfold(summary seq %d): %v", r.SummarySeq, err)
		}
	}

	// Invariant 4: replaying the raced log reproduces the live surface
	// byte-for-byte (replay determinism survived the race).
	replayed, err := Replay(bytes.NewReader(sink.Bytes()))
	if err != nil {
		t.Fatalf("Replay of raced log: %v", err)
	}
	if len(replayed) != len(events) {
		t.Fatalf("replayed count = %d, want %d", len(replayed), len(events))
	}
	liveMsgs, err := lg.Surface()
	if err != nil {
		t.Fatalf("live Surface: %v", err)
	}
	replayMsgs, err := DeriveMessages(replayed)
	if err != nil {
		t.Fatalf("DeriveMessages(replayed): %v", err)
	}
	liveJSON, _ := json.Marshal(liveMsgs)
	replayJSON, _ := json.Marshal(replayMsgs)
	if !bytes.Equal(liveJSON, replayJSON) {
		t.Fatalf("replay determinism broken after raced log:\nlive:   %s\nreplay: %s", liveJSON, replayJSON)
	}
}

// TestReplaceGenerationConcurrentWithAppends exercises the read side:
// ReplaceGeneration and Surface fold the live log while writers append —
// the RLock-snapshot discipline must hold (no unlocked l.events reads).
func TestReplaceGenerationConcurrentWithAppends(t *testing.T) {
	var sink jobSink
	lg := jobTestLog(t, "sess-race-2", &sink)
	defer lg.Close()

	done := make(chan struct{})
	var readers sync.WaitGroup
	for r := 0; r < 3; r++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				if _, err := lg.ReplaceGeneration(); err != nil {
					t.Errorf("ReplaceGeneration: %v", err)
					return
				}
				if _, err := lg.Surface(); err != nil {
					t.Errorf("Surface: %v", err)
					return
				}
			}
		}()
	}

	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 30; i++ {
				if _, err := lg.AppendPrompt(fmt.Sprintf("gen-%02d-%02d", w, i)); err != nil {
					t.Errorf("AppendPrompt: %v", err)
					return
				}
			}
		}(w)
	}
	wg.Wait()
	close(done)
	readers.Wait()

	if got, err := lg.ReplaceGeneration(); err != nil || got != 0 {
		t.Fatalf("final ReplaceGeneration = %d, %v; want 0, nil", got, err)
	}
}
