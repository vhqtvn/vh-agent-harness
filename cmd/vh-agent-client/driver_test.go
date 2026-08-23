// driver_test.go — slice P2 step 4 (red): the client driver at the
// LIBRARY-SERVER seam. An in-process protocol.NewServer over a
// net.Pipe, backed by a real FileEngine + real openaicompat adapter
// against an httptest fake LLM — the same composition shape as
// cmd/vh-agentd's in-process E2E (compose_test.go), plus an
// ask-verdict pre-observer so the approval bridge actually fires (the
// daemon's shipped tools never ask; this is the honest seam for the
// grant/deny pair — see e2e_client_test.go for which test uses which
// seam).
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
	"github.com/vhqtvn/vh-agent-harness/internal/adapters/openaicompat"
	"github.com/vhqtvn/vh-agent-harness/internal/jobs"
	"github.com/vhqtvn/vh-agent-harness/internal/protocol"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// askLLM serves one OpenAI-chat-completions response requesting the
// ask_tool tool call, then a final content response.
func askLLM(t *testing.T) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	calls := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "chatcmpl-ask", "object": "chat.completion", "model": "fake-model",
				"choices": []map[string]any{{
					"index": 0,
					"message": map[string]any{
						"role": "assistant", "content": "",
						"tool_calls": []map[string]any{{
							"id": "call-ask", "type": "function",
							"function": map[string]any{"name": "ask_tool", "arguments": `{"text":"needs a human"}`},
						}},
					},
					"finish_reason": "tool_calls",
				}},
				"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-ask2", "object": "chat.completion", "model": "fake-model",
			"choices": []map[string]any{{
				"index": 0, "message": map[string]any{"role": "assistant", "content": "final answer after approval"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 1, "total_tokens": 4},
		})
	}))
}

// noopExecutor settles every job kind completed (the driver tests never
// dispatch; the engine just needs a non-nil Executor).
type noopExecutor struct{}

func (noopExecutor) Run(ctx context.Context, job jobs.Job) error { return nil }

// askAllObserver returns an ask verdict for every call — the approval
// bridge trigger the shipped daemon tools do not have.
type askAllObserver struct{}

func (askAllObserver) Name() string { return "ask-all" }
func (askAllObserver) ObservePreExecute(call session.ToolCall) tools.Verdict {
	return tools.Ask("approval required (test seam)")
}

// newSeamServer builds the in-process library-seam server: a real
// FileEngine over a temp dir, a real openaicompat adapter against the
// fake LLM, and (after NewServer injects the wire approval bridge) an
// ask_tool whose every call asks. Returns the CLIENT side of the pipe
// and a stop function.
func newSeamServer(t *testing.T, llmURL string) (io.ReadWriteCloser, func()) {
	t.Helper()
	svc, cli := net.Pipe()

	askTool := tools.ToolDefinition{
		Name:        "ask_tool",
		Description: "Test tool whose every call requires an approval answer.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"],"additionalProperties":false}`),
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			return "tool-ran-after-grant", nil
		},
	}
	engine := &protocol.FileEngine{
		Dir:      t.TempDir(),
		Executor: noopExecutor{},
		Ad:       openaicompat.New(openaicompat.Config{BaseURL: llmURL, Model: "fake-model", APIKey: "test-key"}),
		TurnOpts: tools.TurnOptions{Model: "fake-model", Tools: []adapters.ToolSpec{askTool.Spec()}},
	}
	srv := protocol.NewServer(engine, protocol.NewConn(svc), protocol.ServerOptions{})
	// Composition-order contract: register AFTER NewServer so the
	// pipeline freezes with the wire approval bridge in place.
	if err := engine.Pipeline().Register(askTool); err != nil {
		t.Fatalf("register ask_tool: %v", err)
	}
	engine.Pipeline().AddPreObserver(askAllObserver{})

	served := make(chan error, 1)
	go func() { served <- srv.Serve(nil) }()
	stop := func() {
		_ = cli.Close()
		select {
		case <-served:
		case <-time.After(2 * time.Second):
			t.Errorf("seam server did not stop")
		}
	}
	return cli, stop
}

// seamDriver wires a driver against the in-process server: human
// renderer to a buffer, scripted stdin through the single-owner hub
// (input.go — the same arbitration the real wiring uses), stdout to a
// buffer.
func seamDriver(t *testing.T, rwc io.ReadWriteCloser, cfg *Config, stdin string) (*driver, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	client := protocol.NewClient(rwc)
	var errbuf, outbuf bytes.Buffer
	renderer := newHumanRenderer(&errbuf)
	hub := newStdinHub(bufio.NewReader(strings.NewReader(stdin)), &errbuf, false)
	hub.start()
	var approver ApproverFunc = interactiveApprover(hub, &errbuf)
	d := &driver{
		cfg:      cfg,
		client:   client,
		renderer: renderer,
		approver: approver,
		answers:  hub,
		out:      &outbuf,
		errw:     &errbuf,
	}
	return d, &outbuf, &errbuf
}

func seamConfig(t *testing.T) *Config {
	t.Helper()
	return &Config{SessionDir: t.TempDir(), Prompt: "please use the tool", Mode: ModeOneShot}
}

func TestDriverOneShotFinalTextOnStdout(t *testing.T) {
	llm := askLLM(t)
	defer llm.Close()
	rwc, stop := newSeamServer(t, llm.URL)
	defer stop()

	d, out, errbuf := seamDriver(t, rwc, seamConfig(t), "")
	if err := d.run(context.Background()); err != nil {
		t.Fatalf("driver run: %v (stderr:\n%s)", err, errbuf.String())
	}
	if got := out.String(); got != "final answer after approval\n" {
		t.Fatalf("stdout = %q, want final assistant text (stderr:\n%s)", got, errbuf.String())
	}
	// Session id + log path echoed at start (stderr).
	if !strings.Contains(errbuf.String(), "session sess-") || !strings.Contains(errbuf.String(), ".jsonl") {
		t.Fatalf("stderr must echo session id + log path:\n%s", errbuf.String())
	}
}

func TestDriverWritesLastSessionPointer(t *testing.T) {
	llm := askLLM(t)
	defer llm.Close()
	rwc, stop := newSeamServer(t, llm.URL)
	defer stop()

	cfg := seamConfig(t)
	d, _, errbuf := seamDriver(t, rwc, cfg, "")
	if err := d.run(context.Background()); err != nil {
		t.Fatalf("driver run: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(cfg.SessionDir, lastSessionFile))
	if err != nil {
		t.Fatalf("pointer file missing: %v", err)
	}
	if id := strings.TrimSpace(string(raw)); !strings.HasPrefix(id, "sess-") {
		t.Fatalf("pointer file = %q, want a session id", id)
	}
	// The pointer is also echoed in the driver's stderr session line.
	if !strings.Contains(errbuf.String(), strings.TrimSpace(string(raw))) {
		t.Fatalf("stderr session line missing the pointer id:\n%s", errbuf.String())
	}
}

func TestDriverResumeRefusedWithP4Pointer(t *testing.T) {
	llm := askLLM(t)
	defer llm.Close()
	rwc, stop := newSeamServer(t, llm.URL)
	defer stop()

	// Establish a prior session first.
	cfg := seamConfig(t)
	d, _, _ := seamDriver(t, rwc, cfg, "")
	if err := d.run(context.Background()); err != nil {
		t.Fatalf("first run: %v", err)
	}

	// A second run asking to resume must fail with the P4 message,
	// pointing at the missing session/resume wire method.
	cfg2 := seamConfig(t)
	cfg2.Resume = true
	rwc2, stop2 := newSeamServer(t, llm.URL)
	defer stop2()
	d2, _, errbuf2 := seamDriver(t, rwc2, cfg2, "")
	err := d2.run(context.Background())
	if err == nil {
		t.Fatal("resume must be refused")
	}
	if code := exitCodeFor(err); code != 2 {
		t.Fatalf("resume refusal exit = %d, want 2 (stderr:\n%s)", code, errbuf2.String())
	}
	if !strings.Contains(err.Error(), "session/resume") || !strings.Contains(err.Error(), "P4") {
		t.Fatalf("refusal must name session/resume and P4: %v", err)
	}
}

// The approval grant/deny pair at the LIBRARY-SERVER seam: the in-process
// server (FileEngine + NewServer-injected wire approval bridge + ask-all
// observer) fires a REAL approval/request notification over the pipe;
// the client's interactive responder answers from scripted stdin and
// the approval/respond Call resolves it. The daemon's shipped tools
// never ask, so this seam is the honest place to prove the y/n
// responder grants/denies correctly over a REAL connection.

func TestApprovalGrantOverLibrarySeam(t *testing.T) {
	llm := askLLM(t)
	defer llm.Close()
	rwc, stop := newSeamServer(t, llm.URL)
	defer stop()

	d, out, errbuf := seamDriver(t, rwc, seamConfig(t), "y\n")
	if err := d.run(context.Background()); err != nil {
		t.Fatalf("driver run: %v (stderr:\n%s)", err, errbuf.String())
	}
	// The y/N prompt named the tool and reason; the answer granted.
	if !strings.Contains(errbuf.String(), "[y/N] approve tool ask_tool") || !strings.Contains(errbuf.String(), "approval required (test seam)") {
		t.Fatalf("approval prompt missing/malformed:\n%s", errbuf.String())
	}
	if !strings.Contains(errbuf.String(), "→ granted") {
		t.Fatalf("grant not reported:\n%s", errbuf.String())
	}
	// The tool EXECUTED after the grant (its result rendered, not a denial).
	if !strings.Contains(errbuf.String(), "✔ tool result") {
		t.Fatalf("granted tool must have executed:\n%s", errbuf.String())
	}
	if strings.Contains(errbuf.String(), "⊘ tool denied") {
		t.Fatalf("no denial expected on grant:\n%s", errbuf.String())
	}
	// And the conversation continued to the final assistant text.
	if got := out.String(); got != "final answer after approval\n" {
		t.Fatalf("stdout = %q (stderr:\n%s)", got, errbuf.String())
	}
}

func TestApprovalDenyOverLibrarySeam(t *testing.T) {
	llm := askLLM(t)
	defer llm.Close()
	rwc, stop := newSeamServer(t, llm.URL)
	defer stop()

	d, _, errbuf := seamDriver(t, rwc, seamConfig(t), "n\n")
	if err := d.run(context.Background()); err != nil {
		t.Fatalf("driver run: %v (stderr:\n%s)", err, errbuf.String())
	}
	if !strings.Contains(errbuf.String(), "→ denied (default n — fail-closed)") {
		t.Fatalf("deny decision not reported:\n%s", errbuf.String())
	}
	// The denial is visible as a denied tool result (never executed).
	if !strings.Contains(errbuf.String(), "⊘ tool denied ask_tool") {
		t.Fatalf("denied tool result missing:\n%s", errbuf.String())
	}
	if strings.Contains(errbuf.String(), "✔ tool result") {
		t.Fatalf("tool must NOT have executed after a deny:\n%s", errbuf.String())
	}
}

// TestApprovalEOFDeniesHonestly is the (d) path at the same seam: stdin
// closes while an approval is pending → the responder answers a
// fail-closed deny (ENTER/EOF = n) → the daemon settles the turn with
// the denied result → the client exits CLEAN (exit 0) with the honest
// no-answer message.
func TestApprovalEOFDeniesHonestly(t *testing.T) {
	llm := askLLM(t)
	defer llm.Close()
	rwc, stop := newSeamServer(t, llm.URL)
	defer stop()

	d, _, errbuf := seamDriver(t, rwc, seamConfig(t), "")
	if err := d.run(context.Background()); err != nil {
		t.Fatalf("EOF-deny path must complete cleanly, got %v (stderr:\n%s)", err, errbuf.String())
	}
	if !strings.Contains(errbuf.String(), "denied (no answer") {
		t.Fatalf("honest no-answer message missing:\n%s", errbuf.String())
	}
	if !strings.Contains(errbuf.String(), "⊘ tool denied ask_tool") {
		t.Fatalf("fail-closed denial not rendered:\n%s", errbuf.String())
	}
}

func TestDriverFreshRunNotesPriorSession(t *testing.T) {
	llmA := askLLM(t)
	defer llmA.Close()
	rwc, stop := newSeamServer(t, llmA.URL)
	defer stop()

	cfg := seamConfig(t)
	d, _, _ := seamDriver(t, rwc, cfg, "")
	if err := d.run(context.Background()); err != nil {
		t.Fatalf("first run: %v", err)
	}
	raw, _ := os.ReadFile(filepath.Join(cfg.SessionDir, lastSessionFile))
	prior := strings.TrimSpace(string(raw))

	// Fresh stub (per-server call counter) + the SAME session dir, so
	// the second run actually finds the pointer file.
	llmB := askLLM(t)
	defer llmB.Close()
	rwc2, stop2 := newSeamServer(t, llmB.URL)
	defer stop2()
	cfg2 := seamConfig(t)
	cfg2.SessionDir = cfg.SessionDir
	d2, _, errbuf2 := seamDriver(t, rwc2, cfg2, "")
	if err := d2.run(context.Background()); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if !strings.Contains(errbuf2.String(), prior) || !strings.Contains(strings.ToLower(errbuf2.String()), "fresh") {
		t.Fatalf("fresh run must note the prior session honestly:\n%s", errbuf2.String())
	}
}

// --- hotfix c-F2/d-F2: per-prompt turnEndTracker reset ----------------------

// errorThenRecoverLLM serves: call 1 → HTTP 500 (terminal adapter
// failure — the retry ladder is DISARMED in the seam engine, so the
// turn closes kind=error immediately); call 2 → final content.
func errorThenRecoverLLM(t *testing.T) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	calls := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n == 1 {
			http.Error(w, "upstream exploded", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-recover", "object": "chat.completion", "model": "fake-model",
			"choices": []map[string]any{{
				"index": 0, "message": map[string]any{"role": "assistant", "content": "recovered after error"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 4, "total_tokens": 7},
		})
	}))
}

// laggingEndRenderer wraps a Renderer and observes the SECOND turn/end
// asynchronously, after a delay longer than waitTurnEnd's drain cap:
// it deterministically reproduces the documented
// response-beats-notification window (an async dispatch path — the
// client already dispatches some notifications on their own goroutines,
// e.g. approvals) in which a PRIOR turn's kind=error is still the last
// observed end when the next turn's Call has already returned.
type laggingEndRenderer struct {
	inner Renderer
	mu    sync.Mutex
	ends  int
	held  chan struct{} // closed when the delayed observe completed
	lag   time.Duration
}

func (l *laggingEndRenderer) RenderEvent(params json.RawMessage) {
	var ev session.Event
	if err := json.Unmarshal(params, &ev); err == nil && ev.Type == session.TypeTurnEnd {
		l.mu.Lock()
		l.ends++
		n := l.ends
		l.mu.Unlock()
		if n == 2 {
			cp := append([]byte(nil), params...)
			go func() {
				time.Sleep(l.lag)
				l.inner.RenderEvent(cp)
				close(l.held)
			}()
			return
		}
	}
	l.inner.RenderEvent(params)
}

func (l *laggingEndRenderer) RenderApproval(params json.RawMessage) { l.inner.RenderApproval(params) }
func (l *laggingEndRenderer) RenderProtocolError(params json.RawMessage) {
	l.inner.RenderProtocolError(params)
}
func (l *laggingEndRenderer) LastTurnEnd() (string, bool) { return l.inner.LastTurnEnd() }
func (l *laggingEndRenderer) ResetTurnEnd() {
	// Optional forward (compiles against pre-hotfix renderers too).
	if r, ok := l.inner.(interface{ ResetTurnEnd() }); ok {
		r.ResetTurnEnd()
	}
}

// TestREPLErrorTurnDoesNotPoisonNextTurn (hotfix c-F2/d-F2): turn N
// ends kind=error (adapter 500), turn N+1 succeeds — with turn N+1's
// turn/end lagging past the drain window, the client must NOT report a
// spurious turnError for the recovered turn. The pre-fix shape read
// the STALE kind=error from turn N (the tracker was never reset per
// turn and waitTurnEnd's drain was a no-op from the 2nd turn on) and
// misclassified turn N+1.
//
// The REPL loop is the observable surface (a one-shot PAIR uses fresh
// drivers per run — no shared tracker — and converse's own loop stops
// at the first error, so the REPL is where the named hazard lives).
func TestREPLErrorTurnDoesNotPoisonNextTurn(t *testing.T) {
	llm := errorThenRecoverLLM(t)
	defer llm.Close()
	rwc, stop := newSeamServer(t, llm.URL)
	defer stop()

	var errbuf bytes.Buffer
	client := protocol.NewClient(rwc)
	hub := newStdinHub(bufio.NewReader(strings.NewReader("first\nsecond\n")), &errbuf, false)
	hub.start()
	wrapped := &laggingEndRenderer{
		inner: newHumanRenderer(&errbuf),
		held:  make(chan struct{}),
		lag:   2500 * time.Millisecond, // > waitTurnEnd's 2s drain cap
	}
	cfg := &Config{SessionDir: t.TempDir(), Mode: ModeRepl}
	d := &driver{
		cfg:      cfg,
		client:   client,
		renderer: wrapped,
		approver: interactiveApprover(hub, &errbuf),
		answers:  hub,
		out:      io.Discard,
		errw:     &errbuf,
	}

	runDone := make(chan error, 1)
	go func() { runDone <- d.run(context.Background()) }()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("REPL run must end cleanly at EOF, got %v (stderr:\n%s)", err, errbuf.String())
		}
	case <-time.After(15 * time.Second):
		t.Fatalf("REPL run did not return (stderr:\n%s)", errbuf.String())
	}
	// Wait for the lagging observe to land BEFORE reading errbuf (it
	// writes through the wrapped renderer).
	select {
	case <-wrapped.held:
	case <-time.After(5 * time.Second):
		t.Fatalf("lagging turn/end never observed (stderr:\n%s)", errbuf.String())
	}

	got := errbuf.String()
	// Turn 1's failure is reported honestly — exactly once, as the
	// session/prompt protocol error it is.
	if !strings.Contains(got, "session/prompt:") {
		t.Fatalf("turn 1's engine failure must be reported honestly:\n%s", got)
	}
	// THE regression assertion: NO spurious turnError for turn 2 —
	// "turn ended with error" is exactly the misclassification shape.
	if strings.Contains(got, "turn ended with error") {
		t.Fatalf("spurious turnError for the RECOVERED turn (stale kind=error):\n%s", got)
	}
	// Turn 2 genuinely completed: its assistant text rendered.
	if !strings.Contains(got, "recovered after error") {
		t.Fatalf("turn 2's final assistant text missing:\n%s", got)
	}
}
