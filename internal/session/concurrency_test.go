package session

import (
	"bytes"
	"sync"
	"testing"
	"time"
)

// TestLogConcurrentAppendIsSafe is the slice-4 concurrency admission test:
// background job events append from executor goroutines WHILE the turn
// engine drives the same log (enqueue from a tool body mid-turn, settlement
// racing turn/end). The log must be safe for concurrent use — every append
// is one complete JSONL line under the internal lock, so concurrent appends
// interleave only at record boundaries and seq stays contiguous.
//
// Run with -race (the slice gate does): the data race IS the red signal.
func TestLogConcurrentAppendIsSafe(t *testing.T) {
	var sink jobSink
	lg := jobTestLog(t, "sess-conc-1", &sink)
	defer lg.Close()

	const writers = 8
	const each = 25
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < each; i++ {
				if _, err := lg.AppendPrompt("w"); err != nil {
					t.Errorf("AppendPrompt: %v", err)
					return
				}
			}
		}(w)
	}
	wg.Wait()

	events := lg.Events()
	if len(events) != writers*each+1 {
		t.Fatalf("event count = %d, want %d (header + %d appends)", len(events), writers*each+1, writers*each)
	}
	for i, ev := range events {
		if ev.Seq != int64(i+1) {
			t.Fatalf("seq contiguity broken at %d: %d", i, ev.Seq)
		}
	}

	// The persisted bytes must be line-consistent: replay must succeed
	// and reproduce the same event count.
	replayed, err := Replay(bytes.NewReader(sink.Bytes()))
	if err != nil {
		t.Fatalf("Replay after concurrent appends: %v", err)
	}
	if len(replayed) != len(events) {
		t.Fatalf("replayed count = %d, want %d", len(replayed), len(events))
	}
}

// TestLogConcurrentAppendWithReaders extends the admission test with
// concurrent surface folds (the reader side: Events/Surface while writers
// append).
func TestLogConcurrentAppendWithReaders(t *testing.T) {
	var sink jobSink
	lg := jobTestLog(t, "sess-conc-2", &sink)
	defer lg.Close()

	stop := make(chan struct{})
	var readers sync.WaitGroup
	for r := 0; r < 3; r++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if _, err := lg.Surface(); err != nil {
					t.Errorf("Surface: %v", err)
					return
				}
				_ = lg.Events()
			}
		}()
	}
	for i := 0; i < 40; i++ {
		if _, err := lg.AppendPrompt("x"); err != nil {
			t.Fatalf("AppendPrompt: %v", err)
		}
		time.Sleep(time.Millisecond)
	}
	close(stop)
	readers.Wait()

	if got := len(lg.Events()); got != 41 {
		t.Fatalf("event count = %d, want 41", got)
	}
}
