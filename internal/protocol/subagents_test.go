// subagents_test.go — slice B2 wire tests: the subagent/* methods over
// the REAL transport (net.Pipe + Client + per-request handler
// goroutines). Covers the async receipt contract (spawn returns before
// the child turn completes), the fold-derived list (== fold of the
// durable parent log bytes), the live depth fence, continuable sends,
// and the kill-criterion: a FAILING child executor settles cleanly.
package protocol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/subagents"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// gatedExecutor blocks each Run on its gate; on release it appends one
// assistant response to the child log (so a report exists) and returns
// ret.
type gatedExecutor struct {
	gate   chan struct{}
	runs   atomic.Int32
	ret    error
	report string
}

func newGatedExecutor() *gatedExecutor {
	return &gatedExecutor{gate: make(chan struct{})}
}

func (e *gatedExecutor) Run(ctx context.Context, child subagents.Child) error {
	e.runs.Add(1)
	<-e.gate
	if e.report != "" {
		if _, err := child.Log.AppendLLMResponse("test-model", e.report, nil, session.Usage{TotalTokens: 1}); err != nil {
			return err
		}
	}
	return e.ret
}

func (e *gatedExecutor) release() { close(e.gate) }

// failingExecutor settles every child failed, immediately (no gate —
// the kill-criterion: no hang, a settle notice must land).
type failingExecutor struct {
	runs atomic.Int32
}

func (e *failingExecutor) Run(ctx context.Context, child subagents.Child) error {
	e.runs.Add(1)
	return errors.New("child executor exploded")
}

// countingExecutor runs instantly, appending one assistant response
// per run (continuable follow-up turns each have fresh output to
// report).
type countingExecutor struct {
	runs atomic.Int32
}

func (e *countingExecutor) Run(ctx context.Context, child subagents.Child) error {
	n := e.runs.Add(1)
	if _, err := child.Log.AppendLLMResponse("test-model", "turn-"+string(rune('0'+n)), nil, session.Usage{TotalTokens: 1}); err != nil {
		return err
	}
	return nil
}

func waitRuns(t *testing.T, e any, want int32) bool {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if runsOf(e) == want {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return runsOf(e) == want
}

// runsOf extracts the run counter through the three concrete executor
// shapes used above (kept tiny; a full interface is overkill for three
// test types).
func runsOf(e any) int32 {
	switch v := e.(type) {
	case *gatedExecutor:
		return v.runs.Load()
	case *failingExecutor:
		return v.runs.Load()
	case *countingExecutor:
		return v.runs.Load()
	}
	return -1
}

// wireHarness is one server+client over net.Pipe with a FileEngine.
type wireHarness struct {
	t       *testing.T
	eng     *FileEngine
	srv     *Server
	client  *Client
	rec     *eventRecorder
	served  chan error
	logPath string
	dir     string
}

func newSubagentHarness(t *testing.T, exec subagents.Executor, opts subagents.Options) *wireHarness {
	t.Helper()
	return newSubagentHarnessOpts(t, exec, opts, nil)
}

// newSubagentHarnessOpts is newSubagentHarness with an optional
// session→manager registry (the model-facing tool seam; nil keeps the
// engine registry-free).
func newSubagentHarnessOpts(t *testing.T, exec subagents.Executor, opts subagents.Options, reg *subagents.Registry) *wireHarness {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "sess-sub.jsonl")
	eng := &FileEngine{
		Dir:              dir,
		Executor:         noopExecutor{},
		Ad:               &scriptedAdapter{},
		TurnOpts:         tools.TurnOptions{Model: "test-model"},
		SubagentExecutor: exec,
		SubagentStore:    subagents.NewFileStore(filepath.Join(dir, "children")),
		SubagentOpts:     opts,
		SubagentRegistry: reg,
	}
	svc, cli := net.Pipe()
	srv := NewServer(eng, NewConn(svc), ServerOptions{})
	served := make(chan error, 1)
	go func() { served <- srv.Serve(nil) }()
	client := NewClient(cli)

	rec := newEventRecorder()
	client.OnNotification("session/event", func(params json.RawMessage) {
		ev := &sessionEvent{}
		if err := json.Unmarshal(params, ev); err != nil {
			t.Errorf("event params: %v", err)
			return
		}
		rec.add(ev)
	})

	h := &wireHarness{t: t, eng: eng, srv: srv, client: client, rec: rec, served: served, logPath: logPath, dir: dir}
	if err := client.Call("initialize", map[string]any{"protocolVersion": 1}, nil); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if err := h.client.Call("session/create", map[string]any{"path": logPath, "sessionId": "sess-sub"}, nil); err != nil {
		t.Fatalf("session/create: %v", err)
	}
	if err := client.Call("session/subscribe", nil, nil); err != nil {
		t.Fatalf("session/subscribe: %v", err)
	}
	return h
}

func (h *wireHarness) close() {
	_ = h.client.Close()
	select {
	case <-h.served:
	case <-time.After(2 * time.Second):
		h.t.Errorf("server did not exit")
	}
	// Stop the session's manager dispatcher (goroutine hygiene).
	h.eng.subMu.Lock()
	if h.eng.curSub != nil {
		h.eng.curSub.Stop()
	}
	h.eng.subMu.Unlock()
}

// listChildren calls subagent/list over the wire.
func (h *wireHarness) listChildren() []subagents.Status {
	h.t.Helper()
	var res subagentListResult
	if err := h.client.Call("subagent/list", nil, &res); err != nil {
		h.t.Fatalf("subagent/list: %v", err)
	}
	return res.Children
}

// TestSubagentSpawnReceiptImmediateOverWire is the async contract: the
// spawn response arrives while the child turn is STILL RUNNING (the
// receipt never blocks on the child), the state reads running over the
// wire, and after release the report+settle stream to the subscriber
// with the list flipping to settled.
func TestSubagentSpawnReceiptImmediateOverWire(t *testing.T) {
	exec := newGatedExecutor()
	exec.report = "child finished the study"
	h := newSubagentHarness(t, exec, subagents.Options{})
	defer h.close()

	var spawn subagentSpawnResult
	if err := h.client.Call("subagent/spawn", map[string]any{
		"role": "researcher", "prompt": "study the repo", "mode": "oneshot",
	}, &spawn); err != nil {
		t.Fatalf("subagent/spawn: %v", err)
	}
	if spawn.ChildID == "" {
		t.Fatal("spawn receipt has empty childId")
	}
	// The receipt arrived while the child turn is necessarily
	// unfinished (the gate is still closed — Run cannot have returned),
	// and no settle has streamed. (Non-blocking check over the
	// recorder's current snapshot.)
	for _, ev := range h.rec.snapshot() {
		if ev.Type == "subagent/settled" {
			t.Fatalf("settled streamed before the child turn could finish: %+v", ev)
		}
	}
	kids := h.listChildren()
	if len(kids) != 1 || kids[0].State != "running" || kids[0].ChildID != spawn.ChildID {
		t.Fatalf("list while running = %+v", kids)
	}
	if kids[0].Kind != "one-shot" || kids[0].Prompt != "study the repo" || kids[0].Depth != 1 {
		t.Fatalf("list descriptor = %+v", kids[0])
	}

	exec.release()
	// The report and settle stream as session/event notifications.
	if ev := h.rec.waitType(t, "subagent/report"); ev == nil {
		t.Fatalf("no subagent/report in stream: %s", h.rec.types())
	}
	if ev := h.rec.waitType(t, "subagent/settled"); ev == nil {
		t.Fatalf("no subagent/settled in stream: %s", h.rec.types())
	}
	kids = h.listChildren()
	if len(kids) != 1 || kids[0].State != "settled" || kids[0].SettledResult != "completed" {
		t.Fatalf("list after settle = %+v", kids)
	}
	if kids[0].ContentSeq == 0 {
		t.Fatalf("settled child missing contentSeq: %+v", kids[0])
	}

	// Fold truth: the wire list equals the fold of the DURABLE parent
	// log bytes on disk.
	disk, err := session.ReplayFile(h.logPath)
	if err != nil {
		t.Fatalf("replay parent log: %v", err)
	}
	folded, err := subagents.FoldStatus(disk)
	if err != nil {
		t.Fatalf("fold: %v", err)
	}
	if len(folded) != 1 || folded[0] != kids[0] {
		t.Fatalf("wire list %+v != log fold %+v", kids, folded)
	}
}

// TestSubagentFailingExecutorSettlesCleanly is the kill-criterion: a
// child executor that fails settles the child (settle event present,
// result failed) without hanging the protocol loop.
func TestSubagentFailingExecutorSettlesCleanly(t *testing.T) {
	exec := &failingExecutor{}
	h := newSubagentHarness(t, exec, subagents.Options{})
	defer h.close()

	var spawn subagentSpawnResult
	if err := h.client.Call("subagent/spawn", map[string]any{
		"prompt": "doomed task", "mode": "oneshot",
	}, &spawn); err != nil {
		t.Fatalf("subagent/spawn: %v", err)
	}
	// Bounded wait: settle MUST land.
	if ev := h.rec.waitType(t, "subagent/settled"); ev == nil {
		t.Fatalf("no subagent/settled after failing run: %s", h.rec.types())
	}
	kids := h.listChildren()
	if len(kids) != 1 || kids[0].State != "settled" || kids[0].SettledResult != "failed" {
		t.Fatalf("list after failed run = %+v", kids)
	}
	if kids[0].SettledReason != "child executor exploded" {
		t.Fatalf("settle reason = %q", kids[0].SettledReason)
	}
	// The protocol loop is still alive: an ordinary call answers.
	if err := h.client.Call("session/surface", nil, new(json.RawMessage)); err != nil {
		t.Fatalf("protocol loop dead after child failure: %v", err)
	}
}

// TestSubagentContinuableSendsOverWire: a continuable child runs its
// initial turn, then one batched turn PER send — two sends ⇒ three
// turns total — and stays waiting (settlement is manager-owned).
func TestSubagentContinuableSendsOverWire(t *testing.T) {
	exec := &countingExecutor{}
	h := newSubagentHarness(t, exec, subagents.Options{})
	defer h.close()

	var spawn subagentSpawnResult
	if err := h.client.Call("subagent/spawn", map[string]any{
		"prompt": "keep working", "mode": "continuable",
	}, &spawn); err != nil {
		t.Fatalf("subagent/spawn: %v", err)
	}
	if !waitRuns(t, exec, 1) {
		t.Fatalf("initial turn did not run: %d", exec.runs.Load())
	}
	for i, msg := range []string{"go deeper", "wrap up"} {
		var send subagentSendResult
		if err := h.client.Call("subagent/send", map[string]any{
			"childId": spawn.ChildID, "message": msg,
		}, &send); err != nil {
			t.Fatalf("subagent/send[%d]: %v", i, err)
		}
		if !send.Queued {
			t.Fatalf("send[%d] not queued: %+v", i, send)
		}
		if !waitRuns(t, exec, int32(i+2)) {
			t.Fatalf("send[%d] did not run a turn: runs=%d", i, exec.runs.Load())
		}
	}
	kids := h.listChildren()
	if len(kids) != 1 || kids[0].State != "waiting" {
		t.Fatalf("continuable child state = %+v, want waiting", kids)
	}
	// Sending to an UNKNOWN child is a clean wire error.
	err := h.client.Call("subagent/send", map[string]any{
		"childId": "sess-sub.999", "message": "ghost",
	}, nil)
	var perr *Error
	if !errors.As(err, &perr) || perr.Code != ErrInvalidParams {
		t.Fatalf("send to unknown child error = %v", err)
	}
	// Sending to a ONE-SHOT child is refused too (it is not continuable).
	var oneshot subagentSpawnResult
	if err := h.client.Call("subagent/spawn", map[string]any{
		"prompt": "single", "mode": "oneshot",
	}, &oneshot); err != nil {
		t.Fatalf("spawn oneshot: %v", err)
	}
	if !waitRuns(t, exec, 4) {
		t.Fatalf("oneshot turn did not run: %d", exec.runs.Load())
	}
	err = h.client.Call("subagent/send", map[string]any{
		"childId": oneshot.ChildID, "message": "second wind",
	}, nil)
	if !errors.As(err, &perr) || perr.Code != ErrInvalidParams {
		t.Fatalf("send to one-shot error = %v", err)
	}
}

// staticEngine serves one prepared EngineSession (depth-fence fixture:
// a parent log whose header already sits at the cap).
type staticEngine struct {
	es *EngineSession
	ad adapters.Adapter
}

func (e *staticEngine) NewSession(path, sessionID string, sink io.Writer) (*EngineSession, error) {
	return e.es, nil
}
func (e *staticEngine) Adapter() adapters.Adapter      { return e.ad }
func (e *staticEngine) TurnRunner() TurnRunner         { return nil }
func (e *staticEngine) TurnOptions() tools.TurnOptions { return tools.TurnOptions{} }

// TestSubagentDepthFenceOverWire: a parent at max depth gets a CLEAN
// -32602 over the wire carrying the fence text, and no durable spawn
// effects land.
func TestSubagentDepthFenceOverWire(t *testing.T) {
	dir := t.TempDir()
	buf := &bytes.Buffer{}
	parent, err := session.NewChildLog(buf, "sess-deep", session.ChildHeader{
		ParentSessionID: "sess-root", DelegationDepth: 3,
	}, time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatalf("deep parent log: %v", err)
	}
	sm, err := subagents.NewManager(parent, &countingExecutor{}, subagents.NewFileStore(filepath.Join(dir, "children")), subagents.Options{MaxDelegationDepth: 3})
	if err != nil {
		t.Fatalf("manager: %v", err)
	}
	defer sm.Stop()

	eng := &staticEngine{es: &EngineSession{Log: parent, Subagents: sm}, ad: &scriptedAdapter{}}
	svc, cli := net.Pipe()
	srv := NewServer(eng, NewConn(svc), ServerOptions{})
	go func() { _ = srv.Serve(nil) }()
	client := NewClient(cli)
	defer func() {
		_ = client.Close()
		sm.Stop()
	}()

	if err := client.Call("initialize", map[string]any{"protocolVersion": 1}, nil); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if err := client.Call("session/create", map[string]any{"sessionId": "sess-deep"}, nil); err != nil {
		t.Fatalf("session/create: %v", err)
	}
	err = client.Call("subagent/spawn", map[string]any{
		"prompt": "one too deep", "mode": "oneshot",
	}, nil)
	var perr *Error
	if !errors.As(err, &perr) || perr.Code != ErrInvalidParams {
		t.Fatalf("depth-fence error = %v, want -32602", err)
	}
	if perr.Message == "" || !bytes.Contains([]byte(perr.Message), []byte("depth fence")) {
		t.Fatalf("fence error text = %q", perr.Message)
	}
	// Fail-closed: no child was created and the fold stays empty.
	kids := h_list(t, client)
	if len(kids) != 0 {
		t.Fatalf("children after fenced spawn = %+v", kids)
	}
}

func h_list(t *testing.T, client *Client) []subagents.Status {
	t.Helper()
	var res subagentListResult
	if err := client.Call("subagent/list", nil, &res); err != nil {
		t.Fatalf("subagent/list: %v", err)
	}
	return res.Children
}

// TestSubagentListWithoutSessionAndExecutor: list is honest without an
// active session ({children:[]}) and spawn fails closed -32000 on an
// engine without a subagent executor.
func TestSubagentListWithoutSessionAndExecutor(t *testing.T) {
	dir := t.TempDir()
	eng := &FileEngine{
		Dir:      dir,
		Executor: noopExecutor{},
		Ad:       &scriptedAdapter{},
		TurnOpts: tools.TurnOptions{Model: "test-model"},
	}
	svc, cli := net.Pipe()
	srv := NewServer(eng, NewConn(svc), ServerOptions{})
	go func() { _ = srv.Serve(nil) }()
	client := NewClient(cli)
	defer func() { _ = client.Close() }()

	if err := client.Call("initialize", map[string]any{"protocolVersion": 1}, nil); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	// No session: honest empty list (jobs/status mirror).
	kids := h_list(t, client)
	if kids == nil || len(kids) != 0 {
		t.Fatalf("list without session = %+v, want []", kids)
	}
	// With a session but NO executor wired: spawn fails closed; list
	// still folds (empty).
	if err := client.Call("session/create", map[string]any{
		"path": filepath.Join(dir, "sess-bare.jsonl"), "sessionId": "sess-bare",
	}, nil); err != nil {
		t.Fatalf("session/create: %v", err)
	}
	err := client.Call("subagent/spawn", map[string]any{"prompt": "x", "mode": "oneshot"}, nil)
	var perr *Error
	if !errors.As(err, &perr) || perr.Code != ErrEngine {
		t.Fatalf("spawn without executor error = %v, want -32000", err)
	}
	kids = h_list(t, client)
	if len(kids) != 0 {
		t.Fatalf("list without executor = %+v", kids)
	}
	// Param discipline: bad mode and missing prompt are -32602.
	for _, params := range []map[string]any{
		{"prompt": "x", "mode": "sometimes"},
		{"mode": "oneshot"},
		{"prompt": "x", "mode": "oneshot", "seedFromParent": -1},
	} {
		if err := client.Call("subagent/spawn", params, nil); err == nil {
			t.Fatalf("spawn %+v unexpectedly succeeded", params)
		} else if !errors.As(err, &perr) || perr.Code != ErrInvalidParams {
			t.Fatalf("spawn %+v error = %v, want -32602", params, err)
		}
	}
}

// TestSubagentSeedOverWire: seedFromParent flows through the wire into
// the child log (the parent has one completed turn; the seeded child
// log contains exactly its surface messages before the inbox message).
func TestSubagentSeedOverWire(t *testing.T) {
	exec := newGatedExecutor()
	exec.report = "seeded child done"
	h := newSubagentHarness(t, exec, subagents.Options{})
	defer h.close()

	// Give the parent one completed turn via a real session/prompt.
	var turn promptResult
	if err := h.client.Call("session/prompt", map[string]any{"text": "parent context"}, &turn); err != nil {
		t.Fatalf("session/prompt: %v", err)
	}

	var spawn subagentSpawnResult
	if err := h.client.Call("subagent/spawn", map[string]any{
		"prompt": "use my context", "mode": "oneshot", "seedFromParent": 1,
	}, &spawn); err != nil {
		t.Fatalf("subagent/spawn: %v", err)
	}
	exec.release()
	if ev := h.rec.waitType(t, "subagent/settled"); ev == nil {
		t.Fatalf("no settle: %s", h.rec.types())
	}

	childPath := filepath.Join(h.dir, "children", "sess-sub", spawn.ChildID+".jsonl")
	if _, err := os.Stat(childPath); err != nil {
		t.Fatalf("child log missing: %v", err)
	}
	evs, err := session.ResumeFile(childPath)
	if err != nil {
		t.Fatalf("resume child: %v", err)
	}
	// header, seed prompt, seed response, inbox message, then the
	// child's own turn events.
	var kinds []string
	for _, ev := range evs.Events() {
		kinds = append(kinds, ev.Type)
	}
	want := []string{
		session.TypeSessionHeader,
		session.TypeSessionPrompt, session.TypeLLMResponse, // seeds
		session.TypeSubagentMessage, // initial prompt inbox
	}
	for i, w := range want {
		if kinds[i] != w {
			t.Fatalf("child event[%d] = %s, want %s (all: %v)", i, kinds[i], w, kinds)
		}
	}
	// The seed prompt payload is the parent's prompt, verbatim.
	var sp session.PromptPayload
	if err := json.Unmarshal(evs.Events()[1].Payload, &sp); err != nil {
		t.Fatalf("seed prompt payload: %v", err)
	}
	if sp.Text != "parent context" {
		t.Fatalf("seed prompt text = %q", sp.Text)
	}
	// The spawned descriptor records seedTurns=1.
	disk, err := session.ReplayFile(h.logPath)
	if err != nil {
		t.Fatalf("replay parent: %v", err)
	}
	var seeded int
	for _, ev := range disk {
		if ev.Type != session.TypeSubagentSpawned {
			continue
		}
		var p session.SubagentPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			t.Fatalf("spawned payload: %v", err)
		}
		seeded = p.SeedTurns
	}
	if seeded != 1 {
		t.Fatalf("seedTurns = %d, want 1", seeded)
	}
}
