// concurrent_approval_test.go — hotfix b-F1 regression: ≥2 CONCURRENT
// ask-verdict tools at the LIBRARY-SERVER seam (real FileEngine + real
// openaicompat adapter over an httptest LLM + NewServer's wire approval
// bridge + an ask-all pre-observer — the same composition as
// driver_test.go's seam). The engine's parallel tool pool fires both
// approval/request notifications while BOTH are pending; the client's
// answers arrive interleaved and ADVERSARIALLY out of order. Each
// grant/deny must land on exactly its own approvalId.
//
// Run with -race: the pre-fix shape (two approval goroutines sharing
// one unsynchronized *bufio.Reader) is a data race here, and the
// attribution assertions below catch the misrouted-answer shape even
// when the race detector is off (the old responder loop consumed and
// DROPPED lines addressed to sibling ids, so one grant starved its
// tool into an EOF deny).
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
	"github.com/vhqtvn/vh-agent-harness/internal/adapters/openaicompat"
	"github.com/vhqtvn/vh-agent-harness/internal/protocol"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// lockWriter is a mutex-guarded io.Writer (the driver's stderr notes
// come from several goroutines under concurrent approvals).
type lockWriter struct {
	mu sync.Mutex
	b  strings.Builder
}

func (w *lockWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

func (w *lockWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.String()
}

// concurrentAskLLM serves: call 1 → BOTH tool calls in one batch (the
// scheduler pools them in parallel); call 2 → final content.
func concurrentAskLLM(t *testing.T) *httptest.Server {
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
				"id": "chatcmpl-conc", "object": "chat.completion", "model": "fake-model",
				"choices": []map[string]any{{
					"index": 0,
					"message": map[string]any{
						"role": "assistant", "content": "",
						"tool_calls": []map[string]any{
							{
								"id": "call-ask-a", "type": "function",
								"function": map[string]any{"name": "ask_a", "arguments": `{"text":"A needs a human"}`},
							},
							{
								"id": "call-ask-b", "type": "function",
								"function": map[string]any{"name": "ask_b", "arguments": `{"text":"B needs a human"}`},
							},
						},
					},
					"finish_reason": "tool_calls",
				}},
				"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 4, "total_tokens": 7},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-conc2", "object": "chat.completion", "model": "fake-model",
			"choices": []map[string]any{{
				"index": 0, "message": map[string]any{"role": "assistant", "content": "both grants routed correctly"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
}

// newConcurrentSeamServer builds the library-seam server whose single
// batch fires TWO concurrency-safe ask-verdict tools in parallel.
func newConcurrentSeamServer(t *testing.T, llmURL string) (io.ReadWriteCloser, func()) {
	t.Helper()
	svc, cli := net.Pipe()

	mk := func(name string) tools.ToolDefinition {
		return tools.ToolDefinition{
			Name:        name,
			Description: "Test tool whose every call requires an approval answer (concurrency-safe).",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"],"additionalProperties":false}`),
			// Concurrency-safe is what puts both calls in ONE parallel
			// pool segment — the defect's enabling condition.
			IsConcurrencySafe: true,
			Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
				return name + "-ran", nil
			},
		}
	}
	askA, askB := mk("ask_a"), mk("ask_b")
	engine := &protocol.FileEngine{
		Dir:      t.TempDir(),
		Executor: noopExecutor{},
		Ad:       openaicompat.New(openaicompat.Config{BaseURL: llmURL, Model: "fake-model", APIKey: "test-key"}),
		TurnOpts: tools.TurnOptions{Model: "fake-model", Tools: []adapters.ToolSpec{askA.Spec(), askB.Spec()}},
	}
	srv := protocol.NewServer(engine, protocol.NewConn(svc), protocol.ServerOptions{})
	if err := engine.Pipeline().Register(askA); err != nil {
		t.Fatalf("register ask_a: %v", err)
	}
	if err := engine.Pipeline().Register(askB); err != nil {
		t.Fatalf("register ask_b: %v", err)
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

// raceDriver wires a --json one-shot driver over PIPES: NDJSON out (the
// test reads approval requests + tool results as they stream) and
// stdin from a pipe the test writes answers into, out of order, after
// proving both approvals are pending.
func raceDriver(t *testing.T, rwc io.ReadWriteCloser, stdin io.Reader, out, errw io.Writer) *driver {
	t.Helper()
	client := protocol.NewClient(rwc)
	hub := newStdinHub(bufio.NewReader(stdin), errw, true)
	hub.start()
	cfg := &Config{SessionDir: t.TempDir(), Prompt: "please use both tools", Mode: ModeOneShot, JSON: true}
	return &driver{
		cfg:      cfg,
		client:   client,
		renderer: newJSONRenderer(out),
		approver: jsonApprover(hub),
		answers:  hub,
		out:      out,
		errw:     errw,
	}
}

// TestConcurrentApprovalsRouteByApprovalID is the b-F1 crux: two
// CONCURRENTLY-pending approvals (non-vacuous — the test blocks until
// BOTH approval/request notifications are on the wire before answering
// EITHER), answers interleaved and out of order, each grant lands on
// exactly its own approvalId.
func TestConcurrentApprovalsRouteByApprovalID(t *testing.T) {
	llm := concurrentAskLLM(t)
	defer llm.Close()
	rwc, stop := newConcurrentSeamServer(t, llm.URL)
	defer stop()

	stdinR, stdinW := io.Pipe()
	defer stdinR.Close()
	outR, outW := io.Pipe()
	errw := &lockWriter{}
	d := raceDriver(t, rwc, stdinR, outW, errw)

	runDone := make(chan error, 1)
	go func() {
		runErr := d.run(context.Background())
		_ = outW.Close() // EOF the test's NDJSON reader
		runDone <- runErr
	}()

	// NON-VACUOUSNESS BARRIER: read NDJSON lines until BOTH distinct
	// approval/request notifications have streamed. Until this point
	// the test has written NO answer — both approvals are pending
	// simultaneously on the same connection.
	ids := map[string]string{} // approvalId → tool name
	results := map[string]map[string]any{}
	reader := bufio.NewReader(outR)
	barrier := time.After(10 * time.Second)
	for len(ids) < 2 {
		select {
		case <-barrier:
			t.Fatalf("did not see both approval/request notifications (ids=%v, stderr:\n%s)", ids, errw.String())
		default:
		}
		line, rerr := reader.ReadString('\n')
		if rerr != nil && line == "" {
			t.Fatalf("NDJSON stream ended before both approvals arrived (ids=%v, stderr:\n%s)", ids, errw.String())
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			continue // not a JSON line — skip
		}
		var id, name string
		if raw, ok := obj["approvalId"]; ok {
			_ = json.Unmarshal(raw, &id)
			if raw, ok := obj["call"]; ok {
				var call struct {
					Name string `json:"name"`
				}
				_ = json.Unmarshal(raw, &call)
				name = call.Name
			}
			if id != "" {
				ids[id] = name
			}
			continue
		}
		var typ string
		if raw, ok := obj["type"]; ok {
			_ = json.Unmarshal(raw, &typ)
		}
		if typ == "tool/result" {
			if raw, ok := obj["payload"]; ok {
				var p map[string]any
				if err := json.Unmarshal(raw, &p); err == nil {
					results[p["name"].(string)] = p
				}
			}
		}
	}
	if len(ids) != 2 {
		t.Fatalf("expected exactly 2 approvals, got %v", ids)
	}
	idOf := map[string]string{} // tool name → approvalId
	for id, name := range ids {
		if name != "ask_a" && name != "ask_b" {
			t.Fatalf("unexpected tool in approval request: %q (%s)", name, id)
		}
		idOf[name] = id
	}

	// ADVERSARIAL INTERLEAVE: write the grant for the SECOND-rendered
	// approval FIRST (out of order), then the first's. Both grant.
	// Deterministic adversarialness: write B before A regardless of
	// render order.
	writeLine := func(s string) {
		if _, err := stdinW.Write([]byte(s + "\n")); err != nil {
			t.Fatalf("write answer %q: %v (stderr:\n%s)", s, err, errw.String())
		}
	}
	writeLine(`{"id":"` + idOf["ask_b"] + `","approve":true}`)
	writeLine(`{"id":"` + idOf["ask_a"] + `","approve":true}`)
	_ = stdinW.Close() // EOF after both answers — no EOF-deny may fire

	// Drain the rest of the NDJSON stream (tool results + final
	// prompt-result) until the driver's final line lands.
	sawFinal := false
	deadline := time.After(10 * time.Second)
	for !sawFinal {
		select {
		case <-deadline:
			t.Fatalf("driver did not finish (stderr:\n%s)", errw.String())
		default:
		}
		line, rerr := reader.ReadString('\n')
		if rerr != nil && line == "" {
			break // outW closed — driver run finished
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			continue
		}
		if k, ok := obj["kind"]; ok && string(k) == `"prompt-result"` {
			sawFinal = true
		}
		var typ string
		if raw, ok := obj["type"]; ok {
			_ = json.Unmarshal(raw, &typ)
		}
		if typ == "tool/result" {
			if raw, ok := obj["payload"]; ok {
				var p map[string]any
				if err := json.Unmarshal(raw, &p); err == nil {
					results[p["name"].(string)] = p
				}
			}
		}
	}

	// THE ATTRIBUTION ASSERTIONS: each grant landed on its own tool.
	for _, name := range []string{"ask_a", "ask_b"} {
		p := results[name]
		if p == nil {
			t.Fatalf("no tool/result for %s (results=%v, stderr:\n%s)", name, results, errw.String())
		}
		if denied, _ := p["denied"].(bool); denied {
			t.Fatalf("%s was DENIED despite its grant (result=%v, stderr:\n%s) — the answer landed on the wrong approval", name, p, errw.String())
		}
		if content, _ := p["content"].(string); content != name+"-ran" {
			t.Fatalf("%s result content = %v, want %q (stderr:\n%s)", name, p["content"], name+"-ran", errw.String())
		}
	}

	select {
	case runErr := <-runDone:
		if runErr != nil {
			t.Fatalf("driver run: %v (stderr:\n%s)", runErr, errw.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("driver run did not return (stderr:\n%s)", errw.String())
	}
	if !sawFinal {
		t.Fatalf("final prompt-result line missing (stderr:\n%s)", errw.String())
	}
}
