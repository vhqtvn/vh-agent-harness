// concurrency_gate_test.go — ship-review finding 1 crux tests: the
// per-session turn gate and create serialization contract.
//
//	(a) N concurrent session/prompt calls on ONE session must produce N
//	    NON-interleaved turn/begin…turn/end brackets in the replayed log
//	    (each bracket contiguous, exactly one session/prompt inside), and
//	    the adapter must never see a second concurrent call for that
//	    session;
//	(b) a concurrent session/create storm must leave every created log
//	    complete on disk and the final active session CONSISTENT across
//	    the engine (curSubID, the subagent-manager supersede seam) and
//	    the server (active pointer);
//	(c) a create while a prompt is in flight supersedes cleanly: the
//	    prompt's turn completes atomically on ITS session, the new
//	    session becomes active without waiting for the turn.
//
// Determinism: no sleeps. Overlap opportunity pre-fix comes from
// runtime.Gosched() yields inside the adapter (test a) and genuine
// goroutine contention (test b); post-fix the assertions hold by
// construction (serialization, not timing). Test (c) is fully
// channel-synchronized; its one time.After is deadlock DETECTION (the
// parked turn never completes on its own), not a timing dependency —
// the green path resolves in microseconds.
package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/subagents"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// --- fixtures ---------------------------------------------------------------

// yieldAdapter answers immediately but yields inside Call, widening the
// scheduler window in which concurrent turns interleave — WITHOUT
// sleeping. Under the turn gate the yields are harmless (turns are
// serialized by construction); pre-gate they make interleaving
// near-certain. It also tracks the maximum number of concurrently
// in-flight calls (the serialization invariant at the adapter seam).
type yieldAdapter struct {
	inflight    atomic.Int32
	maxInflight atomic.Int32
}

func (a *yieldAdapter) Name() string { return "yield" }

func (a *yieldAdapter) Call(_ context.Context, req *adapters.Request) (*adapters.Response, error) {
	n := a.inflight.Add(1)
	for {
		old := a.maxInflight.Load()
		if n <= old || a.maxInflight.CompareAndSwap(old, n) {
			break
		}
	}
	content := "ok"
	if len(req.Messages) > 0 {
		content = "ok:" + req.Messages[len(req.Messages)-1].Content
	}
	for i := 0; i < 100; i++ {
		runtime.Gosched()
	}
	a.inflight.Add(-1)
	return &adapters.Response{Model: "test-model", Content: content, FinishReason: "stop"}, nil
}

// blockingAdapter parks every Call until the test releases it, and
// signals arrival first — the deterministic in-flight seam for tests
// (c) and the read-only-concurrency sanity check.
type blockingAdapter struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func newBlockingAdapter() *blockingAdapter {
	return &blockingAdapter{entered: make(chan struct{}, 1), release: make(chan struct{})}
}

func (a *blockingAdapter) Name() string { return "blocking" }

func (a *blockingAdapter) Call(_ context.Context, _ *adapters.Request) (*adapters.Response, error) {
	a.once.Do(func() { a.entered <- struct{}{} })
	<-a.release
	return &adapters.Response{Model: "test-model", Content: "released", FinishReason: "stop"}, nil
}

// idleSubagentExecutor arms the engine's subagent surface (so curSub /
// curSubID — the supersede seam — is maintained) without running any
// child turns; no spawn is issued in these tests.
type idleSubagentExecutor struct{}

func (idleSubagentExecutor) Run(_ context.Context, _ subagents.Child) error {
	return fmt.Errorf("idleSubagentExecutor: no child turns in this test")
}

// trackingYieldEngine decorates *FileEngine exactly the way the daemon's
// sessionTracker does — it records ITS OWN active-session pointer AFTER
// the wrapped engine.NewSession returns (a second assignment stage, in
// a second lock) — and yields between the two stages. That is the
// ship-review topology: engine supersede (curSub), tracker active, and
// server active updated in separate stages under separate locks; the
// yields widen the pre-fix interleaving window so the disagreement is
// observable. Under the fix the server serializes the whole create
// handler, so the yields only reorder work inside one serialized
// critical section.
type trackingYieldEngine struct {
	*FileEngine
	mu     sync.Mutex
	active *EngineSession
}

func (t *trackingYieldEngine) NewSession(path, sessionID string, sink io.Writer) (*EngineSession, error) {
	es, err := t.FileEngine.NewSession(path, sessionID, sink)
	if err != nil {
		return nil, err
	}
	for i := 0; i < 100; i++ {
		runtime.Gosched()
	}
	t.mu.Lock()
	t.active = es
	t.mu.Unlock()
	return es, nil
}

func (t *trackingYieldEngine) current() *EngineSession {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.active
}

// wirePair is one server+client over a net.Pipe on the given engine.
type wirePair struct {
	srv    *Server
	client *Client
	served chan error
}

func newWirePair(t *testing.T, eng Engine) *wirePair {
	t.Helper()
	svc, cli := net.Pipe()
	srv := NewServer(eng, NewConn(svc), ServerOptions{})
	p := &wirePair{srv: srv, served: make(chan error, 1)}
	go func() { p.served <- srv.Serve(nil) }()
	p.client = NewClient(cli)
	t.Cleanup(func() {
		_ = p.client.Close()
		<-p.served
	})
	return p
}

// scanBrackets replays a session log and asserts the load-bearing
// invariant: every turn/begin…turn/end bracket is CONTIGUOUS — no
// turn/begin may open while another bracket is open, each bracket holds
// exactly one session/prompt, and no prompt lands outside a bracket. It
// returns the per-bracket prompt texts in log order.
func scanBrackets(t *testing.T, logPath string) []string {
	t.Helper()
	events, err := session.ReplayFile(logPath)
	if err != nil {
		t.Fatalf("replay %s: %v", logPath, err)
	}
	var prompts []string
	open := false
	for i, ev := range events {
		switch ev.Type {
		case session.TypeSessionHeader:
			if i != 0 {
				t.Fatalf("session/header at event %d (not first)", i)
			}
		case session.TypeTurnBegin:
			if open {
				t.Fatalf("interleaved brackets: turn/begin at seq %d while a bracket is open (prompts so far: %v)", ev.Seq, prompts)
			}
			open = true
		case session.TypeTurnEnd:
			if !open {
				t.Fatalf("turn/end at seq %d with no open bracket", ev.Seq)
			}
			open = false
		case session.TypeSessionPrompt:
			if !open {
				t.Fatalf("session/prompt at seq %d outside any turn bracket", ev.Seq)
			}
			var pp session.PromptPayload
			if err := json.Unmarshal(ev.Payload, &pp); err != nil {
				t.Fatalf("prompt payload seq %d: %v", ev.Seq, err)
			}
			prompts = append(prompts, pp.Text)
		}
	}
	if open {
		t.Fatal("log ends inside an open bracket (unterminated turn)")
	}
	return prompts
}

// --- (a) concurrent prompts → non-interleaved brackets ----------------------

// TestConcurrentPromptsNonInterleavedTurnBrackets is the finding-1 crux:
// N concurrent session/prompt calls against ONE session must serialize —
// the replayed log shows N contiguous turn brackets (no bracket ever
// opens inside another), each carrying exactly its own prompt, and the
// adapter observes at most ONE in-flight call for the session at any
// instant.
func TestConcurrentPromptsNonInterleavedTurnBrackets(t *testing.T) {
	const n = 8
	dir := t.TempDir()
	ad := &yieldAdapter{}
	eng := &FileEngine{
		Dir:      dir,
		Executor: noopExecutor{},
		Ad:       ad,
		TurnOpts: tools.TurnOptions{Model: "test-model"},
	}
	p := newWirePair(t, eng)

	if err := p.client.Call("initialize", map[string]any{"protocolVersion": 1}, nil); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	var created createResult
	if err := p.client.Call("session/create", map[string]any{"sessionId": "sess-gate"}, &created); err != nil {
		t.Fatalf("session/create: %v", err)
	}

	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = p.client.Call("session/prompt", map[string]any{"text": fmt.Sprintf("p%d", i)}, nil)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("prompt %d: %v", i, err)
		}
	}

	prompts := scanBrackets(t, created.Path)
	if len(prompts) != n {
		t.Fatalf("bracketed prompts = %v (%d brackets), want %d", prompts, len(prompts), n)
	}
	seen := map[string]bool{}
	for _, txt := range prompts {
		if seen[txt] {
			t.Fatalf("duplicate prompt inside brackets: %q (all: %v)", txt, prompts)
		}
		seen[txt] = true
	}
	for i := 0; i < n; i++ {
		want := fmt.Sprintf("p%d", i)
		if !seen[want] {
			t.Fatalf("prompt %q missing from brackets: %v", want, prompts)
		}
	}
	if got := ad.maxInflight.Load(); got != 1 {
		t.Fatalf("max concurrent adapter calls = %d, want 1 (per-session turn serialization)", got)
	}
}

// --- (b) concurrent create storm → durable consistency ----------------------

// TestConcurrentCreateStormActiveConsistency fires M concurrent
// session/create calls through a tracker-shaped engine decorator (the
// daemon's sessionTracker topology: engine supersede, decorator active,
// server active — three stages, three locks) and asserts the post-storm
// invariants: every created session log is complete on disk (valid
// replay), and the final active session agrees across ALL THREE layers.
func TestConcurrentCreateStormActiveConsistency(t *testing.T) {
	const m = 8
	dir := t.TempDir()
	eng := &FileEngine{
		Dir:              dir,
		Executor:         noopExecutor{},
		Ad:               &scriptedAdapter{},
		TurnOpts:         tools.TurnOptions{Model: "test-model"},
		SubagentExecutor: idleSubagentExecutor{},
	}
	dec := &trackingYieldEngine{FileEngine: eng}
	p := newWirePair(t, dec)

	if err := p.client.Call("initialize", map[string]any{"protocolVersion": 1}, nil); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	paths := make([]string, m)
	var wg sync.WaitGroup
	errs := make([]error, m)
	for i := 0; i < m; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var res createResult
			errs[i] = p.client.Call("session/create", map[string]any{"sessionId": fmt.Sprintf("sess-storm-%d", i)}, &res)
			paths[i] = res.Path
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}

	// Every created log is complete on disk: replay enforces the
	// structural invariants (header first, contiguous 1-based seq) — a
	// partial or interleaved log fails here.
	for i, path := range paths {
		if path == "" {
			t.Fatalf("create %d returned no path", i)
		}
		if _, err := session.ReplayFile(path); err != nil {
			t.Fatalf("created session %d (%s) incomplete on disk: %v", i, path, err)
		}
	}

	// Active-session consistency: the server's active pointer, the
	// tracker-shaped decorator's active pointer, and the engine's
	// supersede seam must all name the SAME session.
	p.srv.mu.Lock()
	activeES := p.srv.active
	p.srv.mu.Unlock()
	if activeES == nil {
		t.Fatal("no active session after create storm")
	}
	serverID := activeES.Log.SessionID()
	trackerES := dec.current()
	if trackerES == nil {
		t.Fatal("decorator recorded no active session")
	}
	trackerID := trackerES.Log.SessionID()
	eng.subMu.Lock()
	curID := eng.curSubID
	eng.subMu.Unlock()
	if serverID != curID || trackerID != curID {
		t.Fatalf("durable active-session disagreement: server.active=%q tracker.active=%q engine.curSubID=%q", serverID, trackerID, curID)
	}
	found := false
	for i := 0; i < m; i++ {
		if curID == fmt.Sprintf("sess-storm-%d", i) {
			found = true
		}
	}
	if !found {
		t.Fatalf("active session %q is not one of the created sessions", curID)
	}
}

// --- (c) prompt in flight + create supersedes -------------------------------

// TestPromptInFlightThenCreateSupersedes pins the documented replacement
// semantics: while a prompt's turn is executing on session A, a
// session/create for session B completes WITHOUT waiting for the turn;
// A's turn then finishes atomically (one contiguous bracket on A's log);
// B is and stays the active session.
func TestPromptInFlightThenCreateSupersedes(t *testing.T) {
	dir := t.TempDir()
	ad := newBlockingAdapter()
	eng := &FileEngine{
		Dir:      dir,
		Executor: noopExecutor{},
		Ad:       ad,
		TurnOpts: tools.TurnOptions{Model: "test-model"},
	}
	p := newWirePair(t, eng)

	if err := p.client.Call("initialize", map[string]any{"protocolVersion": 1}, nil); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	var first createResult
	if err := p.client.Call("session/create", map[string]any{"sessionId": "sess-a"}, &first); err != nil {
		t.Fatalf("create A: %v", err)
	}

	promptDone := make(chan error, 1)
	go func() {
		promptDone <- p.client.Call("session/prompt", map[string]any{"text": "slow question"}, nil)
	}()

	// Deterministic in-flight point: the turn on sess-a has reached the
	// adapter (turn/begin, session/prompt, llm/request already appended).
	<-ad.entered

	// The create supersedes while the turn is in flight — it must not
	// block on the in-flight prompt.
	var second createResult
	if err := p.client.Call("session/create", map[string]any{"sessionId": "sess-b"}, &second); err != nil {
		t.Fatalf("create B while prompt in flight: %v", err)
	}
	p.srv.mu.Lock()
	activeES := p.srv.active
	p.srv.mu.Unlock()
	if activeES == nil || activeES.Log.SessionID() != "sess-b" {
		got := "<nil>"
		if activeES != nil {
			got = activeES.Log.SessionID()
		}
		t.Fatalf("active session after supersede = %q, want sess-b", got)
	}

	// Release the turn; it completes atomically on ITS session.
	close(ad.release)
	if err := <-promptDone; err != nil {
		t.Fatalf("in-flight prompt: %v", err)
	}
	prompts := scanBrackets(t, first.Path)
	if len(prompts) != 1 || prompts[0] != "slow question" {
		t.Fatalf("sess-a bracketed prompts = %v, want exactly [slow question]", prompts)
	}
	events, err := session.ReplayFile(first.Path)
	if err != nil {
		t.Fatalf("replay sess-a: %v", err)
	}
	if events[len(events)-1].Type != session.TypeTurnEnd {
		t.Fatalf("sess-a last event = %s, want turn/end", events[len(events)-1].Type)
	}

	// B remains active and complete on disk.
	p.srv.mu.Lock()
	activeES = p.srv.active
	p.srv.mu.Unlock()
	if activeES == nil || activeES.Log.SessionID() != "sess-b" {
		t.Fatal("active session changed back after turn completion")
	}
	if _, err := session.ReplayFile(second.Path); err != nil {
		t.Fatalf("sess-b incomplete on disk: %v", err)
	}
}

// --- sanity: read-only seams stay concurrent under the gate -----------------

// TestSurfaceConcurrentWithTurn proves the gate serializes ONLY
// turn-executing requests: session/surface answers while another
// prompt's turn is parked inside the adapter. The time.After is
// deadlock detection only — the parked turn never completes on its own,
// so a surface queued behind the gate would hang the test; the green
// path answers in microseconds.
//
// It also PINS the documented mid-turn contract (§4): the concurrent
// surface derivation observes the in-flight turn's already-committed
// events — the trailing user message — at whole-record granularity;
// replaying the log at that same parked moment yields well-formed
// records ending inside the open bracket, never a partial record.
func TestSurfaceConcurrentWithTurn(t *testing.T) {
	dir := t.TempDir()
	ad := newBlockingAdapter()
	eng := &FileEngine{
		Dir:      dir,
		Executor: noopExecutor{},
		Ad:       ad,
		TurnOpts: tools.TurnOptions{Model: "test-model"},
	}
	p := newWirePair(t, eng)

	if err := p.client.Call("initialize", map[string]any{"protocolVersion": 1}, nil); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	var created createResult
	if err := p.client.Call("session/create", map[string]any{"sessionId": "sess-ro"}, &created); err != nil {
		t.Fatalf("session/create: %v", err)
	}

	promptDone := make(chan error, 1)
	go func() {
		promptDone <- p.client.Call("session/prompt", map[string]any{"text": "hold"}, nil)
	}()
	<-ad.entered

	// The read-only seam answers while the turn is parked indefinitely,
	// and (the pinned v1 contract) its derivation observes the
	// in-flight turn's ALREADY-COMMITTED events at whole-record
	// granularity — never interleaved brackets, never a partial record.
	var surf struct {
		Messages []session.Message `json:"messages"`
	}
	surfDone := make(chan error, 1)
	go func() {
		surfDone <- p.client.Call("session/surface", nil, &surf)
	}()
	select {
	case err := <-surfDone:
		if err != nil {
			t.Fatalf("session/surface during in-flight turn: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("session/surface blocked behind the in-flight turn (read-only seams must stay concurrent)")
	}

	// Pin the mid-turn observation: the parked turn has already
	// committed turn/begin + session/prompt ("hold"), so the concurrent
	// surface MUST show that prompt as the trailing user message.
	if n := len(surf.Messages); n == 0 {
		t.Fatal("surface during in-flight turn is empty (want the committed prompt)")
	} else {
		last := surf.Messages[n-1]
		if last.Role != "user" || last.Content != "hold" {
			t.Fatalf("surface during in-flight turn = %+v, want trailing user message %q", surf.Messages, "hold")
		}
	}

	// Still parked at assertion time (the adapter holds until release),
	// so the replay below sees exactly the committed prefix the surface
	// just derived from.
	select {
	case err := <-promptDone:
		t.Fatalf("turn completed before the mid-turn assertions ran (park broken): %v", err)
	default:
	}

	// Whole-record granularity of that committed prefix: replaying the
	// log AT THIS MOMENT yields well-formed records (ReplayFile
	// enforces valid JSON, header first, contiguous 1-based seq) ending
	// inside the in-flight turn's OPEN bracket — turn/begin committed,
	// turn/end not yet, last record the whole llm/request.
	events, err := session.ReplayFile(created.Path)
	if err != nil {
		t.Fatalf("replay mid-turn committed prefix: %v", err)
	}
	var began, ended int
	for _, ev := range events {
		switch ev.Type {
		case session.TypeTurnBegin:
			began++
		case session.TypeTurnEnd:
			ended++
		}
	}
	if began != 1 {
		t.Fatalf("mid-turn replay: turn/begin count = %d, want 1 (the in-flight turn's committed opener)", began)
	}
	if ended != 0 {
		t.Fatalf("mid-turn replay: turn/end count = %d, want 0 (turn still parked)", ended)
	}
	if lastEv := events[len(events)-1]; lastEv.Type != session.TypeLLMRequest {
		t.Fatalf("mid-turn replay: last committed record = %s, want llm/request (whole-record boundary)", lastEv.Type)
	}

	close(ad.release)
	if err := <-promptDone; err != nil {
		t.Fatalf("prompt: %v", err)
	}
}
