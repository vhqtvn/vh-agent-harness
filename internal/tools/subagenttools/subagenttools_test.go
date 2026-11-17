// subagenttools_test.go — the model-facing subagent tool family over a
// REAL subagents.Manager (fake executor, in-memory store): one-shot
// spawn blocks and returns the report, continuable spawn returns the
// child id, send delivers, and the depth fence fires through the tool
// as a typed isError result with zero durable effects.
package subagenttools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/subagents"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// testRig wires one executing session (log + manager) into a registry
// and pipeline carrying the subagent tool family.
type testRig struct {
	reg      *subagents.Registry
	parent   *session.Log
	mgr      *subagents.Manager
	exec     *echoExecutor
	pipeline *tools.Pipeline
}

func newRig(t *testing.T, sessionID string, maxDepth int) *testRig {
	t.Helper()
	reg := subagents.NewRegistry()
	exec := &echoExecutor{}
	lg, err := session.NewLog(&strings.Builder{}, sessionID, time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatalf("parent log: %v", err)
	}
	mgr, err := subagents.NewManager(lg, exec, newMemStore(), subagents.Options{MaxDelegationDepth: maxDepth})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	reg.Put(sessionID, mgr)
	p := tools.NewPipeline()
	for _, d := range Definitions(reg) {
		if err := p.Register(d); err != nil {
			t.Fatalf("register %s: %v", d.Name, err)
		}
	}
	return &testRig{reg: reg, parent: lg, mgr: mgr, exec: exec, pipeline: p}
}

// call runs one tool call as if issued by sessionID's model turn.
func (r *testRig) call(t *testing.T, sessionID, name, args string) tools.Result {
	t.Helper()
	ctx := tools.WithExecutingSession(context.Background(), sessionID)
	return r.pipeline.Execute(ctx, session.ToolCall{ID: "call-1", Name: name, Args: json.RawMessage(args)})
}

func TestSpawnOneshotBlocksAndReturnsReport(t *testing.T) {
	rig := newRig(t, "sess-a", 3)
	res := rig.call(t, "sess-a", SpawnName, `{"prompt":"study the tree","mode":"oneshot"}`)
	if res.IsError {
		t.Fatalf("spawn result isError: %s", res.Content)
	}
	var out struct {
		ChildID string `json:"childId"`
		Result  string `json:"result"`
		Report  string `json:"report"`
	}
	if err := json.Unmarshal([]byte(res.Content), &out); err != nil {
		t.Fatalf("result not JSON: %q: %v", res.Content, err)
	}
	if out.ChildID != "sess-a.1" || out.Result != session.JobResultCompleted {
		t.Fatalf("spawn result = %+v", out)
	}
	if out.Report != "did: study the tree" {
		t.Fatalf("report = %q, want the child's final assistant output", out.Report)
	}
	// Provenance-clean duplication: the report ALSO landed as a
	// user-role subagent/report in the executing session's log.
	found := false
	for _, ev := range rig.parent.Events() {
		if ev.Type == session.TypeSubagentReport {
			found = true
		}
	}
	if !found {
		t.Fatal("subagent/report not relayed to the executing session log")
	}
}

func TestSpawnContinuableReturnsImmediately(t *testing.T) {
	rig := newRig(t, "sess-b", 3)
	res := rig.call(t, "sess-b", SpawnName, `{"prompt":"stand by","mode":"continuable"}`)
	if res.IsError {
		t.Fatalf("spawn result isError: %s", res.Content)
	}
	var out struct {
		ChildID string `json:"childId"`
	}
	if err := json.Unmarshal([]byte(res.Content), &out); err != nil || out.ChildID != "sess-b.1" {
		t.Fatalf("spawn result = %q (%v)", res.Content, err)
	}
	// Follow-up via send; the child answers the exact message.
	sres := rig.call(t, "sess-b", SendName, `{"childId":"sess-b.1","message":"task one"}`)
	if sres.IsError {
		t.Fatalf("send result isError: %s", sres.Content)
	}
	rig.mgr.Drain()
	// The child's log now carries both inbox messages and two turns.
	waitFor(t, "two child turns", func() bool {
		lg := rig.exec.logOf("sess-b.1")
		if lg == nil {
			return false
		}
		n := 0
		for _, ev := range lg.Events() {
			if ev.Type == session.TypeTurnEnd {
				n++
			}
		}
		return n == 2
	})
}

func TestSpawnDefaultsToOneshot(t *testing.T) {
	rig := newRig(t, "sess-c", 3)
	res := rig.call(t, "sess-c", SpawnName, `{"prompt":"default mode"}`)
	if res.IsError {
		t.Fatalf("spawn result isError: %s", res.Content)
	}
	if !strings.Contains(res.Content, `"report":"did: default mode"`) {
		t.Fatalf("default mode not one-shot: %s", res.Content)
	}
}

func TestSpawnArgValidation(t *testing.T) {
	rig := newRig(t, "sess-d", 3)
	for _, tc := range []struct {
		name, args, want string
	}{
		{"empty prompt", `{"prompt":""}`, "prompt is required"},
		{"no args", ``, "args are required"},
		{"bad mode", `{"prompt":"x","mode":"eternal"}`, `mode must be "oneshot" or "continuable"`},
		{"unknown field", `{"prompt":"x","depth":3}`, "unknown field"},
	} {
		res := rig.call(t, "sess-d", SpawnName, tc.args)
		if !res.IsError || !strings.Contains(res.Content, tc.want) {
			t.Fatalf("%s: result = %+v, want isError containing %q", tc.name, res, tc.want)
		}
	}
	// Zero durable effects: nothing spawned.
	if got := len(rig.mgr.Snapshot()); got != 0 {
		t.Fatalf("validation failures left %d children", got)
	}
}

func TestDepthFenceFiresThroughTool(t *testing.T) {
	// maxDepth 1: the session at depth 0 can spawn (child depth 1),
	// but a manager whose OWN depth is already 1 (simulate by building
	// the manager over a depth-1 child log) cannot — the fence must
	// surface as a typed isError tool result with ZERO durable effects.
	reg := subagents.NewRegistry()
	exec := &echoExecutor{}
	lg, err := session.NewChildLog(&strings.Builder{}, "root.1", session.ChildHeader{
		ParentSessionID: "root", DelegationDepth: 1,
	}, time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatalf("child log: %v", err)
	}
	mgr, err := subagents.NewManager(lg, exec, newMemStore(), subagents.Options{MaxDelegationDepth: 1})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	reg.Put("root.1", mgr)
	p := tools.NewPipeline()
	for _, d := range Definitions(reg) {
		if err := p.Register(d); err != nil {
			t.Fatalf("register: %v", err)
		}
	}
	before := len(lg.Events())
	res := p.Execute(tools.WithExecutingSession(context.Background(), "root.1"),
		session.ToolCall{ID: "c1", Name: SpawnName, Args: json.RawMessage(`{"prompt":"one too deep"}`)})
	if !res.IsError {
		t.Fatalf("fence result = %+v, want isError", res)
	}
	if !strings.Contains(res.Content, "depth fence") {
		t.Fatalf("fence text = %q, want the depth-fence refusal", res.Content)
	}
	if after := len(lg.Events()); after != before {
		t.Fatalf("fence refusal had durable effects: %d -> %d events", before, after)
	}
	if got := len(mgr.Snapshot()); got != 0 {
		t.Fatalf("fence refusal spawned %d children", got)
	}
}

func TestNoSessionBindingAndNoManager(t *testing.T) {
	rig := newRig(t, "sess-e", 3)
	// No ctx binding (a call outside any turn): typed error, no panic.
	res := rig.pipeline.Execute(context.Background(),
		session.ToolCall{ID: "c1", Name: SpawnName, Args: json.RawMessage(`{"prompt":"x"}`)})
	if !res.IsError || !strings.Contains(res.Content, "no executing session") {
		t.Fatalf("unbound result = %+v", res)
	}
	// Bound to a session with no manager registered.
	res = rig.call(t, "ghost", SpawnName, `{"prompt":"x"}`)
	if !res.IsError || !strings.Contains(res.Content, "no subagent manager") {
		t.Fatalf("managerless result = %+v", res)
	}
}

func TestSendValidation(t *testing.T) {
	rig := newRig(t, "sess-f", 3)
	for _, tc := range []struct {
		name, args, want string
	}{
		{"missing childId", `{"message":"x"}`, "childId is required"},
		{"missing message", `{"childId":"sess-f.1"}`, "message is required"},
		{"unknown child", `{"childId":"nope","message":"x"}`, "unknown child"},
	} {
		res := rig.call(t, "sess-f", SendName, tc.args)
		if !res.IsError || !strings.Contains(res.Content, tc.want) {
			t.Fatalf("%s: result = %+v, want isError containing %q", tc.name, res, tc.want)
		}
	}
}

func TestSpecsForDepthStripsAtFence(t *testing.T) {
	base := []adapters.ToolSpec{
		{Name: "echo"}, {Name: SpawnName}, {Name: SendName}, {Name: "clock"},
	}
	// Below the fence: the family is advertised verbatim (order kept).
	got := SpecsForDepth(base, 1, 3)
	if len(got) != 4 || got[1].Name != SpawnName || got[2].Name != SendName {
		t.Fatalf("depth 1 specs = %+v", got)
	}
	// At the fence: only the non-subagent tools remain.
	got = SpecsForDepth(base, 3, 3)
	if len(got) != 2 || got[0].Name != "echo" || got[1].Name != "clock" {
		t.Fatalf("depth-max specs = %+v", got)
	}
}

// --- helpers -----------------------------------------------------------------

// echoExecutor mirrors the subagents package's scripted fake but records
// the child logs so tests can inspect them.
type echoExecutor struct {
	mu   sync.Mutex
	logs map[string]*session.Log
}

func (e *echoExecutor) Run(ctx context.Context, child subagents.Child) error {
	e.mu.Lock()
	if e.logs == nil {
		e.logs = map[string]*session.Log{}
	}
	e.logs[child.ID] = child.Log
	e.mu.Unlock()
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

func (e *echoExecutor) logOf(childID string) *session.Log {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.logs[childID]
}

// memStore is the in-memory Store (same as the subagents tests).
type memStore struct {
	mu   sync.Mutex
	logs map[string]*session.Log
}

func newMemStore() *memStore { return &memStore{logs: map[string]*session.Log{}} }

func (s *memStore) CreateChild(parentSessionID, childID string, header session.ChildHeader) (*session.Log, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lg, err := session.NewChildLog(&strings.Builder{}, childID, header, time.Unix(0, 0).UTC())
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

func waitFor(t *testing.T, label string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", label)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
