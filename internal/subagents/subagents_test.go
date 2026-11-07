package subagents

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// echoExecutor is the scripted fake Executor: each run reads the child's
// latest inbox message (the last user-role message in the child surface —
// the message that triggered this turn) and appends one assistant
// response "did: <text>" inside a turn bracket. Deterministic and
// order-proving: report content always cites the exact follow-up that
// produced it.
type echoExecutor struct {
	mu    sync.Mutex
	calls []string // childID per executed run, for order assertions
	fail  map[string]error
}

func (e *echoExecutor) Run(ctx context.Context, child Child) error {
	e.mu.Lock()
	e.calls = append(e.calls, child.ID)
	fail := e.fail[child.ID]
	e.mu.Unlock()
	if fail != nil {
		return fail
	}
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
	_, err = child.Log.AppendTurnEnd("")
	return err
}

// memStore is an in-memory Store for non-filesystem tests: ReopenChild
// returns the SAME live log handle (no double-open).
type memStore struct {
	mu   sync.Mutex
	logs map[string]*session.Log
}

func newMemStore() *memStore { return &memStore{logs: map[string]*session.Log{}} }

func (s *memStore) CreateChild(parentSessionID, childID string, header session.ChildHeader) (*session.Log, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lg, err := session.NewChildLog(&bytes.Buffer{}, childID, header, time.Unix(0, 0).UTC())
	if err != nil {
		return nil, err
	}
	s.logs[parentSessionID+"/"+childID] = lg
	return lg, nil
}

func (s *memStore) ReopenChild(parentSessionID, childID string) (*session.Log, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lg, ok := s.logs[parentSessionID+"/"+childID]
	if !ok {
		return nil, fmt.Errorf("memStore: no child %s/%s", parentSessionID, childID)
	}
	return lg, nil
}

// newTestManager opens a root parent log + manager over the given store.
func newTestManager(t *testing.T, sessionID string, store Store, exec Executor, opts Options) (*Manager, *session.Log) {
	t.Helper()
	lg, err := session.NewLog(&bytes.Buffer{}, sessionID, time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatalf("parent log: %v", err)
	}
	m, err := NewManager(lg, exec, store, opts)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return m, lg
}

func TestOneShotLifecycle(t *testing.T) {
	store := newMemStore()
	m, parent := newTestManager(t, "root-1", store, &echoExecutor{}, Options{})

	rc, err := m.Spawn(session.SubagentKindOneShot, "count widgets", "")
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if rc.ChildID != "root-1.1" {
		t.Fatalf("receipt id = %q, want root-1.1", rc.ChildID)
	}
	m.Drain()

	// Parent log: spawned -> report -> settled (report precedes settle).
	var kinds []string
	for _, ev := range parent.Events() {
		switch ev.Type {
		case session.TypeSubagentSpawned, session.TypeSubagentReport, session.TypeSubagentSettled:
			kinds = append(kinds, ev.Type)
		}
	}
	want := []string{session.TypeSubagentSpawned, session.TypeSubagentReport, session.TypeSubagentSettled}
	if len(kinds) != 3 || kinds[0] != want[0] || kinds[1] != want[1] || kinds[2] != want[2] {
		t.Fatalf("parent event kinds = %v, want %v", kinds, want)
	}

	// Derived states (fold + snapshot agree): settled, completed.
	folded, err := FoldSubagents(parent.Events())
	if err != nil {
		t.Fatalf("FoldSubagents: %v", err)
	}
	if len(folded) != 1 || folded[0].State != StateSettled || folded[0].SettledResult != session.JobResultCompleted {
		t.Fatalf("fold = %+v, want one settled/completed child", folded)
	}
	snap := m.Snapshot()
	if len(snap) != 1 || snap[0].State != StateSettled {
		t.Fatalf("snapshot = %+v, want settled", snap)
	}

	// Child log: header topology + inbox prompt + one assistant turn.
	clg, err := store.ReopenChild("root-1", "root-1.1")
	if err != nil {
		t.Fatalf("ReopenChild: %v", err)
	}
	var hp session.HeaderPayload
	if err := unmarshal(clg.Events()[0].Payload, &hp); err != nil {
		t.Fatalf("child header: %v", err)
	}
	if hp.ParentSessionID != "root-1" || hp.DelegationDepth != 1 || hp.SessionID != "root-1.1" {
		t.Fatalf("child header topology = %+v", hp)
	}
	cmsgs, err := clg.Surface()
	if err != nil {
		t.Fatalf("child surface: %v", err)
	}
	if len(cmsgs) != 2 || cmsgs[0].Role != "user" || cmsgs[0].Content != "count widgets" ||
		cmsgs[1].Role != "assistant" || cmsgs[1].Content != "did: count widgets" {
		t.Fatalf("child surface = %+v", cmsgs)
	}

	// Parent surface: exactly one report, user-role, never assistant.
	pmsgs, err := parent.Surface()
	if err != nil {
		t.Fatalf("parent surface: %v", err)
	}
	if len(pmsgs) != 1 || pmsgs[0].Role != "user" || pmsgs[0].Content != "subagent root-1.1 report: did: count widgets" {
		t.Fatalf("parent surface = %+v", pmsgs)
	}
}

func TestOneShotFailureSettlesFailed(t *testing.T) {
	store := newMemStore()
	exec := &echoExecutor{fail: map[string]error{"root-1.1": errors.New("adapter exploded")}}
	m, parent := newTestManager(t, "root-1", store, exec, Options{})

	if _, err := m.Spawn(session.SubagentKindOneShot, "doomed task", ""); err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	m.Drain()

	folded, err := FoldSubagents(parent.Events())
	if err != nil {
		t.Fatalf("FoldSubagents: %v", err)
	}
	if folded[0].State != StateSettled || folded[0].SettledResult != session.JobResultFailed || folded[0].SettledReason != "adapter exploded" {
		t.Fatalf("fold = %+v, want settled/failed with reason", folded)
	}
	// No assistant output was produced, so no report event exists.
	for _, ev := range parent.Events() {
		if ev.Type == session.TypeSubagentReport {
			t.Fatalf("unexpected report for failed child with no output")
		}
	}
}

func TestContinuableFollowUpInboxOrder(t *testing.T) {
	store := newMemStore()
	m, parent := newTestManager(t, "root-1", store, &echoExecutor{}, Options{})

	if _, err := m.Spawn(session.SubagentKindContinuable, "study the repo", "researcher"); err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	m.Drain()

	// Derived state: continuable + quiescent + not settled => waiting.
	snap := m.Snapshot()
	if snap[0].State != StateWaiting {
		t.Fatalf("snapshot state = %q, want waiting", snap[0].State)
	}
	folded, _ := FoldSubagents(parent.Events())
	if folded[0].State != StateWaiting {
		t.Fatalf("fold state = %q, want waiting", folded[0].State)
	}

	// Follow-up messages land in the child inbox in FIFO order. Each
	// message is drained before the next is sent, so each run's response
	// cites exactly the message that triggered it.
	if err := m.SendMessage("root-1.1", "now the docs"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	m.Drain()
	if err := m.SendMessage("root-1.1", "and the tests"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	m.Drain()

	clg, err := store.ReopenChild("root-1", "root-1.1")
	if err != nil {
		t.Fatalf("ReopenChild: %v", err)
	}
	cmsgs, err := clg.Surface()
	if err != nil {
		t.Fatalf("child surface: %v", err)
	}
	// 3 inbox messages + 3 assistant responses, strictly interleaved FIFO.
	if len(cmsgs) != 6 {
		t.Fatalf("child surface length = %d, want 6", len(cmsgs))
	}
	wantSeq := []struct{ role, content string }{
		{"user", "study the repo"}, {"assistant", "did: study the repo"},
		{"user", "now the docs"}, {"assistant", "did: now the docs"},
		{"user", "and the tests"}, {"assistant", "did: and the tests"},
	}
	for i, w := range wantSeq {
		if cmsgs[i].Role != w.role || cmsgs[i].Content != w.content {
			t.Fatalf("child msgs[%d] = {%s %q}, want {%s %q}", i, cmsgs[i].Role, cmsgs[i].Content, w.role, w.content)
		}
	}

	// One report per distinct child output (3 runs, 3 reports, no dups).
	reports := 0
	for _, ev := range parent.Events() {
		if ev.Type == session.TypeSubagentReport {
			reports++
		}
	}
	if reports != 3 {
		t.Fatalf("report count = %d, want 3", reports)
	}

	// Explicit settle by the parent: settlement is manager/parent-owned.
	if err := m.Settle("root-1.1", nil); err != nil {
		t.Fatalf("Settle: %v", err)
	}
	if got := m.Snapshot()[0].State; got != StateSettled {
		t.Fatalf("state after settle = %q, want settled", got)
	}
	// Inbox is closed once settled.
	if err := m.SendMessage("root-1.1", "too late"); err == nil {
		t.Fatalf("SendMessage on settled child must fail")
	}
	// Follow-ups on a one-shot child fail closed.
	if _, err := m.Spawn(session.SubagentKindOneShot, "quick job", ""); err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	m.Drain()
	if err := m.SendMessage("root-1.2", "no follow-ups"); err == nil {
		t.Fatalf("SendMessage on one-shot child must fail")
	}
}

func TestDoubleSettleFirstWins(t *testing.T) {
	store := newMemStore()
	m, parent := newTestManager(t, "root-1", store, &echoExecutor{}, Options{})

	if _, err := m.Spawn(session.SubagentKindContinuable, "work", ""); err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	m.Drain()
	if err := m.Settle("root-1.1", nil); err != nil {
		t.Fatalf("Settle #1: %v", err)
	}
	if err := m.Settle("root-1.1", errors.New("second opinion")); err != nil {
		t.Fatalf("Settle #2 must be a first-wins no-op, got %v", err)
	}
	settled := 0
	for _, ev := range parent.Events() {
		if ev.Type == session.TypeSubagentSettled {
			settled++
		}
	}
	if settled != 1 {
		t.Fatalf("settled events = %d, want exactly 1", settled)
	}

	// One-shot: explicit settle AFTER auto-settle is also a no-op.
	if _, err := m.Spawn(session.SubagentKindOneShot, "quick", ""); err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	m.Drain()
	if err := m.Settle("root-1.2", errors.New("late")); err != nil {
		t.Fatalf("explicit settle after auto-settle must be a no-op, got %v", err)
	}
	settled = 0
	for _, ev := range parent.Events() {
		if ev.Type == session.TypeSubagentSettled && ev.Payload != nil {
			var p session.SubagentPayload
			_ = unmarshal(ev.Payload, &p)
			if p.ChildID == "root-1.2" {
				settled++
			}
		}
	}
	if settled != 1 {
		t.Fatalf("root-1.2 settled events = %d, want exactly 1 (auto)", settled)
	}

	// Unknown child fails closed.
	if err := m.Settle("root-1.99", nil); err == nil {
		t.Fatalf("settle unknown child must fail")
	}
}

func TestSettleFlushesPendingReportOnce(t *testing.T) {
	store := newMemStore()
	m, parent := newTestManager(t, "root-1", store, &echoExecutor{}, Options{})

	if _, err := m.Spawn(session.SubagentKindContinuable, "study", ""); err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	m.Drain()
	// The initial turn's output was already flushed post-run; settling
	// flushes nothing more (no new output) — exactly one report total.
	if err := m.Settle("root-1.1", nil); err != nil {
		t.Fatalf("Settle: %v", err)
	}
	reports := 0
	for _, ev := range parent.Events() {
		if ev.Type == session.TypeSubagentReport {
			reports++
		}
	}
	if reports != 1 {
		t.Fatalf("reports = %d, want 1", reports)
	}
}

func TestDepthFenceFailClosed(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	// root(0) -> root.1(1) -> root.1.1(2) -> root.1.1.1(3): each deeper
	// manager is bound to the child's own log (its header depth is
	// authoritative).
	type level struct {
		mgr *Manager
		lg  *session.Log
	}
	var levels []level

	rootLog, err := session.OpenFile(filepath.Join(dir, "root-1.jsonl"), "root-1")
	if err != nil {
		t.Fatalf("root log: %v", err)
	}
	defer rootLog.Close()
	m0, err := NewManager(rootLog, &echoExecutor{}, store, Options{})
	if err != nil {
		t.Fatalf("NewManager root: %v", err)
	}
	levels = append(levels, level{m0, rootLog})

	for i := 0; i < 3; i++ {
		parent := levels[len(levels)-1]
		if _, err := parent.mgr.Spawn(session.SubagentKindContinuable, fmt.Sprintf("depth %d work", i+1), ""); err != nil {
			t.Fatalf("Spawn at depth %d: %v", i+1, err)
		}
		parent.mgr.Drain()
		// Reopen the freshly spawned first-born via its store key and
		// bind a manager to it (its header depth = i+1).
		var hp session.HeaderPayload
		if err := unmarshal(parent.lg.Events()[0].Payload, &hp); err != nil {
			t.Fatalf("parent header at depth %d: %v", i+1, err)
		}
		childID := hp.SessionID + ".1"
		clg, err := store.ReopenChild(hp.SessionID, childID)
		if err != nil {
			t.Fatalf("ReopenChild %s: %v", childID, err)
		}
		mgr, err := NewManager(clg, &echoExecutor{}, store, Options{})
		if err != nil {
			t.Fatalf("NewManager over %s: %v", childID, err)
		}
		levels = append(levels, level{mgr, clg})
	}

	// The depth-3 session cannot spawn: child depth 4 exceeds the cap.
	deepest := levels[3]
	before := len(deepest.lg.Events())
	_, err = deepest.mgr.Spawn(session.SubagentKindOneShot, "one too deep", "")
	if err == nil {
		t.Fatalf("spawn beyond MaxDelegationDepth must fail closed")
	}
	if want := "delegation depth fence"; !contains(err.Error(), want) {
		t.Fatalf("fence error = %q, want substring %q", err.Error(), want)
	}
	if after := len(deepest.lg.Events()); after != before {
		t.Fatalf("failed spawn left durable traces: events %d -> %d", before, after)
	}
	// No orphan child log was created for the refused spawn.
	var hp session.HeaderPayload
	if err := unmarshal(deepest.lg.Events()[0].Payload, &hp); err != nil {
		t.Fatalf("deepest header: %v", err)
	}
	if _, err := store.ReopenChild(hp.SessionID, hp.SessionID+".1"); err == nil {
		t.Fatalf("no child log may exist for the fence-refused spawn")
	}

	// The fence is configurable: an explicit larger cap admits the deeper
	// spawn over the same depth-3 log.
	deepMgr, err := NewManager(deepest.lg, &echoExecutor{}, store, Options{MaxDelegationDepth: 4})
	if err != nil {
		t.Fatalf("NewManager with cap 4: %v", err)
	}
	if _, err := deepMgr.Spawn(session.SubagentKindOneShot, "depth 4 allowed", ""); err != nil {
		t.Fatalf("Spawn at depth 4 with cap 4: %v", err)
	}
	deepMgr.Drain()
}

func TestDepthSurvivesChildLogResume(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	rootLog, err := session.OpenFile(filepath.Join(dir, "root-1.jsonl"), "root-1")
	if err != nil {
		t.Fatalf("root log: %v", err)
	}
	m, err := NewManager(rootLog, &echoExecutor{}, store, Options{})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if _, err := m.Spawn(session.SubagentKindContinuable, "survive me", ""); err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	m.Drain()

	// Simulate process death: close the child log, then resume it from
	// disk — the header (authoritative) still carries depth 1 + parent.
	childPath := filepath.Join(dir, "root-1", "root-1.1.jsonl")
	resumed, err := session.ResumeFile(childPath)
	if err != nil {
		t.Fatalf("ResumeFile child: %v", err)
	}
	var hp session.HeaderPayload
	if err := unmarshal(resumed.Events()[0].Payload, &hp); err != nil {
		t.Fatalf("resumed child header: %v", err)
	}
	if hp.DelegationDepth != 1 || hp.ParentSessionID != "root-1" {
		t.Fatalf("resumed child header = %+v, want depth 1 parent root-1", hp)
	}

	// A manager bound to the resumed child log enforces the fence from
	// the PERSISTED depth (not a runtime guess): depth 2 and 3 spawns
	// pass, a depth-4 spawn fails closed.
	m2, err := NewManager(resumed, &echoExecutor{}, store, Options{})
	if err != nil {
		t.Fatalf("NewManager over resumed child: %v", err)
	}
	if _, err := m2.Spawn(session.SubagentKindOneShot, "grandchild", ""); err != nil {
		t.Fatalf("Spawn from resumed depth-1 child: %v", err)
	}
	m2.Drain()
	gclg, err := store.ReopenChild("root-1.1", "root-1.1.1")
	if err != nil {
		t.Fatalf("ReopenChild grandchild: %v", err)
	}
	m3, err := NewManager(gclg, &echoExecutor{}, store, Options{})
	if err != nil {
		t.Fatalf("NewManager grandchild: %v", err)
	}
	if _, err := m3.Spawn(session.SubagentKindOneShot, "great-grandchild", ""); err != nil {
		t.Fatalf("Spawn at depth 3 (within cap): %v", err)
	}
	m3.Drain()
	ggclg, err := store.ReopenChild("root-1.1.1", "root-1.1.1.1")
	if err != nil {
		t.Fatalf("ReopenChild great-grandchild: %v", err)
	}
	m4, err := NewManager(ggclg, &echoExecutor{}, store, Options{})
	if err != nil {
		t.Fatalf("NewManager great-grandchild: %v", err)
	}
	if _, err := m4.Spawn(session.SubagentKindOneShot, "too deep", ""); err == nil {
		t.Fatalf("spawn at depth 4 must fail closed")
	}
}

func TestManagerReconstructionCrossChecksDepth(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	// Parent log with a spawned record claiming depth 1…
	rootPath := filepath.Join(dir, "root-1.jsonl")
	rootLog, err := session.OpenFile(rootPath, "root-1")
	if err != nil {
		t.Fatalf("root log: %v", err)
	}
	if _, err := rootLog.AppendSubagentSpawned("subagent-1", session.SubagentKindContinuable, "p", 1); err != nil {
		t.Fatalf("AppendSubagentSpawned: %v", err)
	}
	// …but a TAMPERED child log whose authoritative header says depth 5.
	if _, err := store.CreateChild("root-1", "subagent-1", session.ChildHeader{
		ParentSessionID: "root-1", DelegationDepth: 5,
	}); err != nil {
		t.Fatalf("CreateChild: %v", err)
	}
	if _, err := NewManager(rootLog, &echoExecutor{}, store, Options{}); err == nil {
		t.Fatalf("depth mismatch between child header and parent record must refuse reconstruction")
	} else if !contains(err.Error(), "delegation depth mismatch") {
		t.Fatalf("cross-check error = %q, want delegation depth mismatch", err.Error())
	}

	// Parent-id mismatch is equally fail-closed.
	otherPath := filepath.Join(dir, "root-2.jsonl")
	otherLog, err := session.OpenFile(otherPath, "root-2")
	if err != nil {
		t.Fatalf("other log: %v", err)
	}
	if _, err := otherLog.AppendSubagentSpawned("subagent-1", session.SubagentKindOneShot, "p", 1); err != nil {
		t.Fatalf("AppendSubagentSpawned: %v", err)
	}
	if _, err := NewManager(otherLog, &echoExecutor{}, store, Options{}); err == nil {
		t.Fatalf("child claimed by another parent must refuse reconstruction")
	}
}

func TestSpawnValidation(t *testing.T) {
	store := newMemStore()
	m, _ := newTestManager(t, "root-1", store, &echoExecutor{}, Options{})
	if _, err := m.Spawn("sometimes", "p", ""); err == nil {
		t.Fatalf("invalid kind must fail")
	}
	if _, err := m.Spawn(session.SubagentKindOneShot, "", ""); err == nil {
		t.Fatalf("empty prompt must fail")
	}
	if err := m.SendMessage("root-1.404", "x"); err == nil {
		t.Fatalf("unknown child send must fail")
	}
	if err := m.SendMessage("root-1.404", ""); err == nil {
		t.Fatalf("empty message must fail")
	}
}

func TestConcurrentSpawnSnapshotSettle(t *testing.T) {
	store := newMemStore()
	m, parent := newTestManager(t, "root-1", store, &echoExecutor{}, Options{})

	const n = 8
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			kind := session.SubagentKindOneShot
			if i%2 == 0 {
				kind = session.SubagentKindContinuable
			}
			if _, err := m.Spawn(kind, fmt.Sprintf("task %d", i), ""); err != nil {
				t.Errorf("Spawn %d: %v", i, err)
			}
		}(i)
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.Snapshot()
		}()
	}
	wg.Wait()
	m.Drain()

	// Settle all continuable children concurrently; one-shot children
	// already auto-settled (first-wins makes explicit settles no-ops).
	var wg2 sync.WaitGroup
	for i := 0; i < n; i++ {
		wg2.Add(1)
		go func(i int) {
			defer wg2.Done()
			_ = m.Settle(fmt.Sprintf("root-1.%d", i+1), nil)
		}(i)
	}
	wg2.Wait()

	folded, err := FoldSubagents(parent.Events())
	if err != nil {
		t.Fatalf("FoldSubagents: %v", err)
	}
	if len(folded) != n {
		t.Fatalf("children = %d, want %d", len(folded), n)
	}
	settled := 0
	for _, d := range folded {
		if d.State == StateSettled {
			settled++
		}
	}
	if settled != n {
		t.Fatalf("settled = %d, want %d", settled, n)
	}
	settledEvents := 0
	for _, ev := range parent.Events() {
		if ev.Type == session.TypeSubagentSettled {
			settledEvents++
		}
	}
	if settledEvents != n {
		t.Fatalf("settled events = %d, want exactly %d (first-wins under concurrency)", settledEvents, n)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
