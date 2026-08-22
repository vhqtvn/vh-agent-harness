// sched_test.go — ship-review finding 1 daemon-layer coverage: the
// sessionTracker's NewSession must serialize the wrapped engine
// construction and its own active assignment as ONE stage, so a
// concurrent create storm leaves the tracker's active session healthy
// and every created log complete on disk. (The full three-layer
// engine/tracker/server disagreement RED is proven at the protocol
// layer — internal/protocol/concurrency_gate_test.go drives the same
// topology through a tracker-shaped decorator; here the REAL tracker
// type is pinned under the same storm.)
package main

import (
	"fmt"
	"io"
	"path/filepath"
	"sync"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/protocol"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

func TestTrackerConcurrentCreateStormConsistency(t *testing.T) {
	const m = 8
	cfg := testConfig(t, "openai", "http://127.0.0.1:1")
	engine := &protocol.FileEngine{Dir: cfg.SessionDir, Executor: daemonExecutor{}}
	tracker := &sessionTracker{Engine: engine}

	paths := make([]string, m)
	var wg sync.WaitGroup
	errs := make([]error, m)
	sessions := make([]*protocol.EngineSession, m)
	for i := 0; i < m; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sessions[i], errs[i] = tracker.NewSession(
				filepath.Join(cfg.SessionDir, fmt.Sprintf("storm-%d.jsonl", i)),
				fmt.Sprintf("sess-storm-%d", i), io.Discard)
			if sessions[i] != nil {
				paths[i] = sessions[i].Path
			}
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("tracker create %d: %v", i, err)
		}
	}

	// Every created log is complete on disk (valid replay: header
	// first, contiguous seq).
	for i, path := range paths {
		if _, err := session.ReplayFile(path); err != nil {
			t.Fatalf("created session %d (%s) incomplete on disk: %v", i, path, err)
		}
	}

	// The tracker's active session is exactly one of the created
	// sessions, and its dispatch seam routes to it (engine stage and
	// assignment ran atomically under the tracker lock).
	cur := tracker.current()
	if cur == nil {
		t.Fatal("tracker recorded no active session")
	}
	found := false
	for i := 0; i < m; i++ {
		if cur == sessions[i] {
			found = true
		}
	}
	if !found {
		t.Fatalf("tracker active session %q is not one of the created sessions", cur.Log.SessionID())
	}
	if _, err := (trackerDispatcher{t: tracker}).Dispatch("echo", nil); err != nil {
		t.Fatalf("tracker dispatch on active session after storm: %v", err)
	}
}
