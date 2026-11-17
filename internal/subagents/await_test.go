// await_test.go — AwaitChild (the blocking settlement wait that backs
// the model-facing one-shot subagent_spawn tool) and the session→Manager
// Registry the tool layer resolves managers through.
package subagents

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// TestAwaitChildOneShotReport is the AwaitChild contract over a one-shot
// child: the call blocks until settlement, then reports the terminal
// result plus the child's final assistant output (the SAME content the
// subagent/report relay carried — provenance-clean by construction).
func TestAwaitChildOneShotReport(t *testing.T) {
	store := newMemStore()
	m, parent := newTestManager(t, "await-1", store, &echoExecutor{}, Options{})

	rc, err := m.Spawn(session.SubagentKindOneShot, "count widgets", "")
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	res, err := m.AwaitChild(context.Background(), rc.ChildID)
	if err != nil {
		t.Fatalf("AwaitChild: %v", err)
	}
	if res.ChildID != rc.ChildID || res.Result != session.JobResultCompleted || res.Reason != "" {
		t.Fatalf("await result = %+v", res)
	}
	if res.Report != "did: count widgets" {
		t.Fatalf("report = %q, want the child's final assistant output", res.Report)
	}
	// The relay report is durable in the parent log BEFORE settle —
	// AwaitChild must not change the durable shape.
	var sawReport, sawSettled bool
	for _, ev := range parent.Events() {
		switch ev.Type {
		case session.TypeSubagentReport:
			sawReport = true
		case session.TypeSubagentSettled:
			sawSettled = true
		}
	}
	if !sawReport || !sawSettled {
		t.Fatalf("await changed durability: report=%v settled=%v", sawReport, sawSettled)
	}
}

// TestAwaitChildBlocksUntilSettle proves the BLOCKING half: with a
// gated executor, AwaitChild is still pending while the child turn is
// executing and returns within a bounded window after the gate opens.
func TestAwaitChildBlocksUntilSettle(t *testing.T) {
	store := newMemStore()
	gate := make(chan struct{})
	exec := &gatedExecutor{gate: gate}
	m, _ := newTestManager(t, "await-2", store, exec, Options{})

	rc, err := m.Spawn(session.SubagentKindOneShot, "slow work", "")
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	awaited := make(chan AwaitResult, 1)
	errCh := make(chan error, 1)
	go func() {
		res, err := m.AwaitChild(context.Background(), rc.ChildID)
		awaited <- res
		errCh <- err
	}()

	// While the child turn is gated, AwaitChild must not return.
	select {
	case res := <-awaited:
		t.Fatalf("AwaitChild returned before settle: %+v", res)
	case <-time.After(50 * time.Millisecond):
	}
	close(gate)
	select {
	case res := <-awaited:
		if err := <-errCh; err != nil {
			t.Fatalf("AwaitChild error: %v", err)
		}
		if res.Result != session.JobResultCompleted || res.Report != "did: slow work" {
			t.Fatalf("await result = %+v", res)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("AwaitChild did not return after settle")
	}
}

// TestAwaitChildAlreadySettled: awaiting a child that already settled
// returns immediately (waiter registration after the fact).
func TestAwaitChildAlreadySettled(t *testing.T) {
	store := newMemStore()
	m, _ := newTestManager(t, "await-3", store, &echoExecutor{}, Options{})
	rc, err := m.Spawn(session.SubagentKindOneShot, "quick", "")
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	m.Drain()
	res, err := m.AwaitChild(context.Background(), rc.ChildID)
	if err != nil {
		t.Fatalf("AwaitChild: %v", err)
	}
	if res.Result != session.JobResultCompleted || res.Report != "did: quick" {
		t.Fatalf("await result = %+v", res)
	}
}

// TestAwaitChildFailedRun: a failing one-shot run settles failed and
// AwaitChild surfaces the failure text (data, not an await error).
func TestAwaitChildFailedRun(t *testing.T) {
	store := newMemStore()
	exec := &echoExecutor{fail: map[string]error{"await-4.1": errors.New("boom")}}
	m, _ := newTestManager(t, "await-4", store, exec, Options{})
	rc, err := m.Spawn(session.SubagentKindOneShot, "will fail", "")
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	res, err := m.AwaitChild(context.Background(), rc.ChildID)
	if err != nil {
		t.Fatalf("AwaitChild: %v (settlement failure is data, not an error)", err)
	}
	if res.Result != session.JobResultFailed || res.Reason != "boom" {
		t.Fatalf("await result = %+v, want failed/boom", res)
	}
}

// TestAwaitChildCancel: a canceled context abandons the WAIT (never the
// child — settlement still lands).
func TestAwaitChildCancel(t *testing.T) {
	store := newMemStore()
	gate := make(chan struct{})
	defer close(gate)
	exec := &gatedExecutor{gate: gate}
	m, parent := newTestManager(t, "await-5", store, exec, Options{})
	rc, err := m.Spawn(session.SubagentKindOneShot, "stuck", "")
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := m.AwaitChild(ctx, rc.ChildID)
		errCh <- err
	}()
	cancel()
	select {
	case err := <-errCh:
		if err == nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("AwaitChild did not observe cancel")
	}
	// The spawned child remains durable (the spawn was real); only the
	// wait was abandoned.
	found := false
	for _, ev := range parent.Events() {
		if ev.Type == session.TypeSubagentSpawned {
			found = true
		}
	}
	if !found {
		t.Fatal("cancel erased the spawn — the wait is the only thing abandoned")
	}
}

// TestAwaitChildUnknownChild: unknown ids fail closed without blocking.
func TestAwaitChildUnknownChild(t *testing.T) {
	store := newMemStore()
	m, _ := newTestManager(t, "await-6", store, &echoExecutor{}, Options{})
	if _, err := m.AwaitChild(context.Background(), "nope"); err == nil || !strings.Contains(err.Error(), "unknown child") {
		t.Fatalf("unknown child error = %v", err)
	}
}

// gatedExecutor holds every run until its gate closes, then echoes the
// last inbox message like echoExecutor.
type gatedExecutor struct {
	gate  chan struct{}
	calls int64
}

func (e *gatedExecutor) Run(ctx context.Context, child Child) error {
	atomic.AddInt64(&e.calls, 1)
	<-e.gate
	s, err := session.FoldSurface(child.Log.Events())
	if err != nil {
		return err
	}
	last := ""
	for i := len(s.Messages) - 1; i >= 0; i-- {
		if s.Messages[i].Role == "user" {
			last = s.Messages[i].Content
			break
		}
	}
	if _, err := child.Log.AppendTurnBegin(); err != nil {
		return err
	}
	if _, err := child.Log.AppendLLMResponse("mock-child", "did: "+last, nil, session.Usage{TotalTokens: 1}); err != nil {
		return err
	}
	_, aerr := child.Log.AppendTurnEnd("")
	return aerr
}

// TestRegistryPutGetRemove covers the session→Manager registry the
// model-facing tools resolve through.
func TestRegistryPutGetRemove(t *testing.T) {
	r := NewRegistry()
	if _, ok := r.Get("sess-x"); ok {
		t.Fatal("empty registry returned a manager")
	}
	store := newMemStore()
	m, _ := newTestManager(t, "sess-x", store, &echoExecutor{}, Options{})
	r.Put("sess-x", m)
	got, ok := r.Get("sess-x")
	if !ok || got != m {
		t.Fatalf("Get = %p,%v want %p", got, ok, m)
	}
	r.Put("sess-x", m) // idempotent overwrite
	r.Remove("sess-x")
	if _, ok := r.Get("sess-x"); ok {
		t.Fatal("Remove left the manager resolvable")
	}
	r.Remove("sess-x") // removing an absent id is a no-op
}

// TestEffectiveMaxDepth pins the fence-default resolution shared by the
// manager and the depth-conditional tool advertising.
func TestEffectiveMaxDepth(t *testing.T) {
	if got := EffectiveMaxDepth(0); got != DefaultMaxDelegationDepth {
		t.Fatalf("0 = %d, want default %d", got, DefaultMaxDelegationDepth)
	}
	if got := EffectiveMaxDepth(-2); got != DefaultMaxDelegationDepth {
		t.Fatalf("-2 = %d, want default %d", got, DefaultMaxDelegationDepth)
	}
	if got := EffectiveMaxDepth(2); got != 2 {
		t.Fatalf("2 = %d, want 2", got)
	}
}
