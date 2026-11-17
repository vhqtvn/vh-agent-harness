// server_test.go — slice 5 step 4 (red): the full server over an
// in-memory pipe against REAL engine seams (file-backed session.Log,
// jobs.Manager, tools.Pipeline). The dispatch receipt must return
// BEFORE the job settles (async contract: dispatch → receipt{jobId} →
// events → settlement), and step 5: the surface snapshot reflects the
// settled job after report emission.
package protocol

import (
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/jobs"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// blockExec is a jobs.Executor that blocks until released.
type blockExec struct {
	started chan<- string
	release <-chan struct{}
}

func (e *blockExec) Run(ctx context.Context, job jobs.Job) error {
	e.started <- job.ID
	<-e.release
	return nil
}

// newServerPair boots a real FileEngine server plus reference Client.
// dir is the engine's session root (TB-F1: explicit create paths must
// resolve inside it).
func newServerPair(t *testing.T, dir string, exec jobs.Executor) (*Client, *FileEngine, func()) {
	t.Helper()
	eng := &FileEngine{Dir: dir, Executor: exec}
	svc, cli := net.Pipe()
	srv := NewServer(eng, NewConn(svc), ServerOptions{})
	served := make(chan error, 1)
	go func() { served <- srv.Serve(nil) }()
	client := NewClient(cli)
	cleanup := func() {
		_ = client.Close()
		select {
		case <-served:
		case <-time.After(2 * time.Second):
			t.Errorf("server did not exit")
		}
	}
	return client, eng, cleanup
}

// sessionEvent is one session/event notification payload (= one
// session.Event record).
type sessionEvent struct {
	Seq     int64           `json:"seq"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// eventRecorder collects every session/event notification in arrival
// order (nothing is discarded — later assertions can wait on any type).
type eventRecorder struct {
	mu     sync.Mutex
	events []*sessionEvent
	notify chan struct{} // signaled on every append
}

func newEventRecorder() *eventRecorder {
	return &eventRecorder{notify: make(chan struct{}, 64)}
}

func (r *eventRecorder) add(ev *sessionEvent) {
	r.mu.Lock()
	r.events = append(r.events, ev)
	r.mu.Unlock()
	r.notify <- struct{}{}
}

func (r *eventRecorder) snapshot() []*sessionEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*sessionEvent(nil), r.events...)
}

// waitType blocks until at least one recorded event has typ, returning it.
func (r *eventRecorder) waitType(t *testing.T, typ string) *sessionEvent {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		for _, ev := range r.snapshot() {
			if ev.Type == typ {
				return ev
			}
		}
		select {
		case <-r.notify:
		case <-deadline:
			t.Fatalf("no %s event arrived (recorded: %s)", typ, r.types())
			return nil
		}
	}
}

func (r *eventRecorder) types() string {
	var out string
	for _, ev := range r.snapshot() {
		out += ev.Type + " "
	}
	return out
}

func TestFullFlowCreateDispatchEventsStatus(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "sess.jsonl")
	started := make(chan string)
	release := make(chan struct{})
	client, _, stop := newServerPair(t, dir, &blockExec{started: started, release: release})
	defer stop()

	// initialize
	var initRes struct {
		ProtocolVersion int `json:"protocolVersion"`
	}
	if err := client.Call("initialize", map[string]any{"protocolVersion": 1}, &initRes); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if initRes.ProtocolVersion != 1 {
		t.Fatalf("protocolVersion = %d", initRes.ProtocolVersion)
	}

	// subscribe BEFORE create: the event stream is live-only (history
	// belongs to the log and session/surface).
	rec := newEventRecorder()
	client.OnNotification("session/event", func(params json.RawMessage) {
		ev := &sessionEvent{}
		if err := json.Unmarshal(params, ev); err != nil {
			t.Errorf("bad session/event payload: %v", err)
			return
		}
		rec.add(ev)
	})
	if err := client.Call("session/subscribe", map[string]any{
		"types": []string{"job/enqueued", "job/started", "job/settled"},
	}, nil); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	// create
	var createRes struct {
		SessionID string `json:"sessionId"`
		Path      string `json:"path"`
	}
	if err := client.Call("session/create", map[string]any{
		"path": logPath, "sessionId": "sess-flow",
	}, &createRes); err != nil {
		t.Fatalf("create: %v", err)
	}
	if createRes.SessionID != "sess-flow" || createRes.Path != logPath {
		t.Fatalf("create result = %+v", createRes)
	}

	// dispatch: the receipt returns while the job is still executing.
	var receipt struct {
		JobID string `json:"jobId"`
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- client.Call("session/dispatch", map[string]any{
			"kind": "background", "payload": map[string]any{"n": 1},
		}, &receipt)
	}()
	// The executor HAS started (job began) but is parked on release...
	if id := <-started; id != "background-1" {
		t.Fatalf("started %s", id)
	}
	// ...and STILL the receipt must have landed (non-blocking assert:
	// dispatch never waits for execution).
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("dispatch: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("dispatch receipt blocked on job execution")
	}
	if receipt.JobID != "background-1" {
		t.Fatalf("receipt = %+v", receipt)
	}

	// status while running: fold-derived state is "running".
	var st struct {
		Jobs []jobs.Status `json:"jobs"`
	}
	if err := client.Call("jobs/status", nil, &st); err != nil {
		t.Fatalf("status: %v", err)
	}
	if len(st.Jobs) != 1 || st.Jobs[0].State != jobs.StateRunning || st.Jobs[0].JobID != "background-1" {
		t.Fatalf("running status = %+v", st.Jobs)
	}

	// release: settlement flows as session/event, in order.
	close(release)
	ev := rec.waitType(t, "job/settled")
	var jp session.JobPayload
	if err := json.Unmarshal(ev.Payload, &jp); err != nil {
		t.Fatalf("settled payload: %v", err)
	}
	if jp.JobID != "background-1" || jp.Result != "completed" {
		t.Fatalf("settled payload = %+v", jp)
	}
	// enqueued and started arrived before settled (filtered stream).
	enq := rec.waitType(t, "job/enqueued")
	str := rec.waitType(t, "job/started")
	if !(enq.Seq < str.Seq && str.Seq < ev.Seq) {
		t.Fatalf("event order: enq=%d started=%d settled=%d", enq.Seq, str.Seq, ev.Seq)
	}

	// status after settlement.
	if err := client.Call("jobs/status", nil, &st); err != nil {
		t.Fatalf("status: %v", err)
	}
	if len(st.Jobs) != 1 || st.Jobs[0].State != jobs.StateSettled || st.Jobs[0].Result != "completed" {
		t.Fatalf("settled status = %+v", st.Jobs)
	}
}

func TestSurfaceSnapshotPostSettlement(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "sess.jsonl")
	started := make(chan string)
	release := make(chan struct{})
	client, _, stop := newServerPair(t, dir, &blockExec{started: started, release: release})
	defer stop()

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("call: %v", err)
		}
	}
	must(client.Call("initialize", map[string]any{"protocolVersion": 1}, nil))
	must(client.Call("session/create", map[string]any{"path": logPath, "sessionId": "sess-surface"}, nil))
	var rec struct {
		JobID string `json:"jobId"`
	}
	must(client.Call("session/dispatch", map[string]any{"kind": "ingest"}, &rec))
	<-started
	close(release)

	// Poll until settled, then surface.
	deadline := time.Now().Add(3 * time.Second)
	for {
		var st struct {
			Jobs []jobs.Status `json:"jobs"`
		}
		must(client.Call("jobs/status", nil, &st))
		if len(st.Jobs) == 1 && st.Jobs[0].State == jobs.StateSettled {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("job never settled")
		}
		time.Sleep(5 * time.Millisecond)
	}

	var surf struct {
		Messages []session.Message `json:"messages"`
	}
	must(client.Call("session/surface", nil, &surf))
	// The settled-job notice entered the surface exactly once, as the
	// environment→model user message (dsh reported-flag semantics).
	found := 0
	for _, m := range surf.Messages {
		if m.Role == "user" && m.Content == "background job ingest-1 completed" {
			found++
		}
	}
	if found != 1 {
		t.Fatalf("surface = %+v (want exactly one settled-job notice)", surf.Messages)
	}
}

func TestClientDispatchPromptReturnsReceipt(t *testing.T) {
	dir := t.TempDir()
	started := make(chan string)
	release := make(chan struct{})
	client, _, stop := newServerPair(t, dir, &blockExec{started: started, release: release})
	defer stop()

	if err := client.Call("initialize", map[string]any{"protocolVersion": 1}, nil); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	var createRes createResult
	if err := client.Call("session/create", map[string]any{
		"path": filepath.Join(dir, "sess.jsonl"), "sessionId": "sess-dp",
	}, &createRes); err != nil {
		t.Fatalf("create: %v", err)
	}

	rec, err := client.DispatchPrompt("do the thing")
	if err != nil {
		t.Fatalf("DispatchPrompt: %v", err)
	}
	if rec == nil || rec.JobID != "prompt-1" {
		t.Fatalf("receipt = %+v", rec)
	}
	close(release)
}

func TestSessionRequiredErrors(t *testing.T) {
	client, _, stop := newServerPair(t, t.TempDir(), &blockExec{started: make(chan string, 1), release: make(chan struct{})})
	defer stop()
	if err := client.Call("initialize", map[string]any{"protocolVersion": 1}, nil); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	for _, method := range []string{"session/prompt", "session/dispatch", "session/surface"} {
		var params any
		if method == "session/prompt" {
			params = map[string]any{"text": "hi"}
		} else if method == "session/dispatch" {
			params = map[string]any{"kind": "background"}
		}
		err := client.Call(method, params, nil)
		pe, ok := err.(*Error)
		if !ok || pe.Code != ErrNoSession {
			t.Fatalf("%s error = %v (%T), want ErrNoSession", method, err, err)
		}
	}
}
