// status_test.go — fold-derived job status snapshot (the jobs/status
// seam the host protocol serves).
package jobs

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

func TestSnapshotLifecycleStates(t *testing.T) {
	started := make(chan string)
	release := make(chan struct{})
	ex := &blockingExec{started: started, release: release}
	m, _ := testManager(t, "sess-status", ex, Options{})

	// queued → running → settled transitions must be visible in order.
	if _, err := m.Dispatch("background", json.RawMessage(`{"n":1}`)); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	s := m.Snapshot()
	if len(s) != 1 {
		t.Fatalf("len = %d", len(s))
	}
	if s[0].JobID != "background-1" || s[0].Kind != "background" {
		t.Fatalf("snapshot fields = %+v", s[0])
	}
	if s[0].State != StateQueued && s[0].State != StateRunning {
		// The dispatcher goroutine may have started the job already;
		// both pre-settle states are acceptable, settled is not.
		t.Fatalf("pre-settle snapshot = %+v", s)
	}

	if id := <-started; id != "background-1" {
		t.Fatalf("started %s", id)
	}
	s = m.Snapshot()
	if len(s) != 1 || s[0].State != StateRunning {
		t.Fatalf("running snapshot = %+v", s)
	}

	close(release)
	m.Drain()
	s = m.Snapshot()
	if len(s) != 1 || s[0].State != StateSettled || s[0].Result != session.JobResultCompleted {
		t.Fatalf("settled snapshot = %+v", s)
	}
}

func TestSnapshotFailedCarriesReason(t *testing.T) {
	ex := &quickExec{err: errors.New("boom")}
	m, _ := testManager(t, "sess-status-fail", ex, Options{})
	if _, err := m.Dispatch("ingest", nil); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	m.Drain()
	s := m.Snapshot()
	if len(s) != 1 || s[0].Result != session.JobResultFailed || s[0].Reason != "boom" {
		t.Fatalf("failed snapshot = %+v", s)
	}
}

func TestSnapshotEnqueueOrder(t *testing.T) {
	release := make(chan struct{})
	ex := &blockingExec{started: make(chan string, 8), release: release}
	m, _ := testManager(t, "sess-status-order", ex, Options{})
	for i := 0; i < 3; i++ {
		if _, err := m.Dispatch("work", nil); err != nil {
			t.Fatalf("dispatch %d: %v", i, err)
		}
	}
	close(release)
	m.Drain()
	s := m.Snapshot()
	if len(s) != 3 {
		t.Fatalf("len = %d", len(s))
	}
	for i, want := range []string{"work-1", "work-2", "work-3"} {
		if s[i].JobID != want {
			t.Fatalf("s[%d].JobID = %s, want %s", i, s[i].JobID, want)
		}
	}
}
