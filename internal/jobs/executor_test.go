package jobs

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// capWatchingExec is the semaphore-watching fixture: it records the
// concurrent in-flight high-water mark, records run-entry order, verifies
// the durable job/started record exists BEFORE Run is entered, and holds
// every job until released.
type capWatchingExec struct {
	mu       sync.Mutex
	inflight int
	maxSeen  int
	runOrder []string
	lg       *session.Log
	t        *testing.T
	release  chan struct{}
}

func (e *capWatchingExec) Run(_ context.Context, job Job) error {
	// job/started must already be durable when the body is entered.
	if !hasStartedEvent(e.lg, job.ID) {
		e.t.Errorf("executor entered Run(%s) before job/started was logged", job.ID)
	}
	e.mu.Lock()
	e.inflight++
	if e.inflight > e.maxSeen {
		e.maxSeen = e.inflight
	}
	e.runOrder = append(e.runOrder, job.ID)
	e.mu.Unlock()
	<-e.release
	e.mu.Lock()
	e.inflight--
	e.mu.Unlock()
	return nil
}

func (e *capWatchingExec) snapshot() (maxSeen, inflight int, runOrder []string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.maxSeen, e.inflight, append([]string(nil), e.runOrder...)
}

func hasStartedEvent(lg *session.Log, jobID string) bool {
	for _, ev := range lg.Events() {
		if ev.Type != session.TypeJobStarted {
			continue
		}
		var p session.JobPayload
		if err := json.Unmarshal(ev.Payload, &p); err == nil && p.JobID == jobID {
			return true
		}
	}
	return false
}

// startedOrderInLog returns the JobIDs of job/started events in log order.
func startedOrderInLog(t *testing.T, lg *session.Log) []string {
	t.Helper()
	var out []string
	for _, ev := range lg.Events() {
		if ev.Type != session.TypeJobStarted {
			continue
		}
		var p session.JobPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			t.Fatalf("unmarshal job/started: %v", err)
		}
		out = append(out, p.JobID)
	}
	return out
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// TestExecutorCapLimitsInflight: with MaxInFlightPerOwner=2 and five
// dispatched jobs, the concurrent in-flight count never exceeds the cap
// (the semaphore-watching assertion), yet all five eventually run and
// settle.
func TestExecutorCapLimitsInflight(t *testing.T) {
	sink := &testSink{}
	lg, err := session.NewLog(sink, "sess-exec-cap", time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	exec := &capWatchingExec{lg: lg, t: t, release: make(chan struct{})}
	m, err := NewManager(lg, exec, Options{MaxInFlightPerOwner: 2})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(m.Stop)

	for i := 0; i < 5; i++ {
		if _, err := m.Dispatch("background", json.RawMessage(`{}`)); err != nil {
			t.Fatalf("Dispatch %d: %v", i, err)
		}
	}

	// The cap fills (2 running) and the other three wait.
	waitFor(t, 2*time.Second, func() bool {
		_, inflight, _ := exec.snapshot()
		return inflight == 2
	}, "inflight to reach the cap of 2")

	// Give a would-be violator time to oversubscribe; the cap must hold.
	time.Sleep(30 * time.Millisecond)
	maxSeen, inflight, _ := exec.snapshot()
	if inflight != 2 || maxSeen > 2 {
		t.Fatalf("inflight=%d maxSeen=%d: per-owner cap of 2 not observed", inflight, maxSeen)
	}
	if n := len(startedOrderInLog(t, lg)); n != 2 {
		t.Fatalf("job/started count while capped = %d, want 2", n)
	}

	close(exec.release)
	m.Drain()

	maxSeen, _, runOrder := exec.snapshot()
	if maxSeen > 2 {
		t.Fatalf("max in-flight %d exceeded the per-owner cap of 2", maxSeen)
	}
	if len(runOrder) != 5 {
		t.Fatalf("run order has %d entries, want 5 (all jobs eventually ran)", len(runOrder))
	}
	if n := len(startedOrderInLog(t, lg)); n != 5 {
		t.Fatalf("job/started count after drain = %d, want 5", n)
	}
}

// TestExecutorSerialStartedOrder: job/started lands in enqueue order
// (the serial dispatcher is the single writer of started records).
func TestExecutorSerialStartedOrder(t *testing.T) {
	sink := &testSink{}
	lg, err := session.NewLog(sink, "sess-exec-order", time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	exec := &capWatchingExec{lg: lg, t: t, release: make(chan struct{})}
	m, err := NewManager(lg, exec, Options{MaxInFlightPerOwner: 3})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(m.Stop)

	for i := 0; i < 4; i++ {
		if _, err := m.Dispatch("background", json.RawMessage(`{}`)); err != nil {
			t.Fatalf("Dispatch %d: %v", i, err)
		}
	}
	waitFor(t, 2*time.Second, func() bool {
		_, _, order := exec.snapshot()
		return len(order) == 3 // cap 3: three run concurrently, the fourth gates
	}, "three jobs to start under the cap")
	close(exec.release)
	m.Drain()
	_, _, runOrder := exec.snapshot()
	if len(runOrder) != 4 {
		t.Fatalf("run order has %d entries, want 4", len(runOrder))
	}

	want := []string{"background-1", "background-2", "background-3", "background-4"}
	got := startedOrderInLog(t, lg)
	if len(got) != len(want) {
		t.Fatalf("started order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("started order = %v, want %v (serial FIFO drain)", got, want)
		}
	}
}

// TestExecutorCapGatesUntilSettle: with cap 1, the second job must not
// start (no job/started, no Run entry) until the first settles and
// releases its slot.
func TestExecutorCapGatesUntilSettle(t *testing.T) {
	sink := &testSink{}
	lg, err := session.NewLog(sink, "sess-exec-gate", time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	exec := &capWatchingExec{lg: lg, t: t, release: make(chan struct{})}
	m, err := NewManager(lg, exec, Options{MaxInFlightPerOwner: 1})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(m.Stop)

	if _, err := m.Dispatch("background", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("Dispatch 1: %v", err)
	}
	if _, err := m.Dispatch("background", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("Dispatch 2: %v", err)
	}
	waitFor(t, 2*time.Second, func() bool {
		_, inflight, _ := exec.snapshot()
		return inflight == 1
	}, "first job to be running alone")

	time.Sleep(30 * time.Millisecond)
	if _, inflight, order := exec.snapshot(); inflight != 1 || len(order) != 1 {
		t.Fatalf("second job started under cap 1: inflight=%d runOrder=%v", inflight, order)
	}
	if got := startedOrderInLog(t, lg); len(got) != 1 || got[0] != "background-1" {
		t.Fatalf("started records under cap 1 = %v, want [background-1]", got)
	}

	close(exec.release)
	m.Drain()
	got := startedOrderInLog(t, lg)
	if len(got) != 2 || got[0] != "background-1" || got[1] != "background-2" {
		t.Fatalf("started records after drain = %v, want [background-1 background-2]", got)
	}
}
