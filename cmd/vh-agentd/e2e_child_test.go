// e2e_child_test.go — the binary-level end-to-end crux: build the real
// vh-agentd binary, spawn it as a child process, and drive it over its
// REAL stdio with the reference protocol.Client against an httptest
// fake LLM: initialize → session/create → subscribe → prompt (echo tool
// round-trip) → dispatch (job settles over the stream) → stdin close →
// clean exit. Also asserts credential hygiene: the API key VALUE never
// appears in the child's stderr or the durable session log.
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/protocol"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// childPipes joins the child's stdout (read) and stdin (write) into the
// Client's ReadWriteCloser seam; Close closes only the write side (the
// close-write → EOF ladder).
type childPipes struct {
	io.ReadCloser
	io.WriteCloser
}

func (c childPipes) Close() error { return c.WriteCloser.Close() }

type eventRecorder struct {
	mu    sync.Mutex
	types []string
}

func (r *eventRecorder) add(t string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.types = append(r.types, t)
}

func (r *eventRecorder) has(t string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, x := range r.types {
		if x == t {
			return true
		}
	}
	return false
}

func (r *eventRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.types...)
}

// syncBuf is a mutex-guarded bytes.Buffer: exec's stderr copier
// goroutine and the test body both touch it, so reads must be
// synchronized even before cmd.Wait returns.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// TestChildProcessEndToEnd drives the actual binary end-to-end.
func TestChildProcessEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and spawns the daemon binary")
	}

	const testKey = "vh-agentd-test-key-value-DO-NOT-LOG"

	llm := fakeLLM(t)
	defer llm.Close()

	bin := filepath.Join(t.TempDir(), "vh-agentd")
	build := exec.Command("go", "build", "-o", bin, "./cmd/vh-agentd")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	sessDir := filepath.Join(t.TempDir(), "sessions")
	logPath := filepath.Join(sessDir, "sess-child.jsonl")

	cmd := exec.Command(bin,
		"--adapter", "openai",
		"--model", "fake-model",
		"--base-url", llm.URL,
		"--api-key-env", "VH_AGENTD_TEST_KEY",
		"--session-dir", sessDir,
	)
	cmd.Env = append(os.Environ(), "VH_AGENTD_TEST_KEY="+testKey)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	var childStderr syncBuf
	cmd.Stderr = &childStderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}

	client := protocol.NewClient(childPipes{stdout.(io.ReadCloser), stdin.(io.WriteCloser)})

	// Record the live event stream.
	rec := &eventRecorder{}
	client.OnNotification("session/event", func(params json.RawMessage) {
		var ev struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(params, &ev); err == nil {
			rec.add(ev.Type)
		}
	})

	waitExit := make(chan error, 1)
	go func() { waitExit <- cmd.Wait() }()
	defer func() {
		_ = client.Close()
		if cmd.ProcessState != nil {
			return // already reaped by the body's close-ladder check
		}
		select {
		case <-waitExit:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			t.Errorf("child did not exit; stderr:\n%s", childStderr.String())
		}
	}()

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("call: %v (child stderr:\n%s)", err, childStderr.String())
		}
	}

	must(client.Call("initialize", map[string]any{"protocolVersion": 1}, nil))
	must(client.Call("session/create", map[string]any{"path": logPath, "sessionId": "sess-child"}, nil))
	must(client.Call("session/subscribe", nil, nil))

	// ONE tool turn: the fake LLM requests echo; the result must come
	// back over the wire.
	var turn struct {
		Content   string            `json:"content"`
		ToolCalls []json.RawMessage `json:"toolCalls"`
		Results   []tools.Result    `json:"results"`
	}
	must(client.Call("session/prompt", map[string]any{"text": "child: echo something"}, &turn))
	if len(turn.Results) != 1 || turn.Results[0].IsError || turn.Results[0].Content != "hello-inprocess" {
		t.Fatalf("tool round-trip results = %+v (stderr:\n%s)", turn.Results, childStderr.String())
	}

	// The full turn choreography streamed as session/event.
	deadline := time.Now().Add(3 * time.Second)
	for _, want := range []string{"turn/begin", "session/prompt", "llm/request", "llm/response", "tool/call", "tool/result", "turn/end"} {
		for !rec.has(want) {
			if time.Now().After(deadline) {
				t.Fatalf("event %q missing from stream: %v", want, rec.snapshot())
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Async jobs over the stream: dispatch receipt → settled completed.
	var receipt struct {
		JobID string `json:"jobId"`
	}
	must(client.Call("session/dispatch", map[string]any{"kind": "echo"}, &receipt))
	if receipt.JobID != "echo-1" {
		t.Fatalf("jobId = %q", receipt.JobID)
	}
	settled := time.Now().Add(3 * time.Second)
	for {
		var st struct {
			Jobs []struct {
				JobID  string `json:"jobId"`
				State  string `json:"state"`
				Result string `json:"result"`
			} `json:"jobs"`
		}
		must(client.Call("jobs/status", nil, &st))
		if len(st.Jobs) == 1 && st.Jobs[0].State == "settled" && st.Jobs[0].Result == "completed" {
			break
		}
		if time.Now().After(settled) {
			t.Fatalf("job never settled completed: %+v", st.Jobs)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !rec.has("job/enqueued") || !rec.has("job/settled") {
		t.Fatalf("job lifecycle events missing from stream: %v", rec.snapshot())
	}

	// Credential hygiene: the key VALUE must appear nowhere the daemon
	// emits — not on stderr, not in the durable session log.
	if strings.Contains(childStderr.String(), testKey) {
		t.Fatalf("API key value leaked to child stderr:\n%s", childStderr.String())
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("session log not durable at %s: %v", logPath, err)
	}
	if strings.Contains(string(raw), testKey) {
		t.Fatal("API key value leaked into the durable session log")
	}
	if !strings.Contains(string(raw), "tool/call") {
		t.Fatalf("session log missing tool/call: %s", raw)
	}

	// Close ladder: close the write side → child sees EOF → exit 0.
	if err := stdin.Close(); err != nil {
		t.Fatalf("close stdin: %v", err)
	}
	select {
	case werr := <-waitExit:
		if werr != nil {
			t.Fatalf("child exit error: %v (stderr:\n%s)", werr, childStderr.String())
		}
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("child did not exit after EOF (stderr:\n%s)", childStderr.String())
	}
	if !strings.Contains(childStderr.String(), "starting: adapter=openaicompat") {
		t.Fatalf("child stderr missing startup line:\n%s", childStderr.String())
	}
	// Scheduler lifecycle smoke (B1): constructed with the real tracker
	// seams, started before Serve, drained at shutdown.
	if !strings.Contains(childStderr.String(), "scheduler: started") || !strings.Contains(childStderr.String(), "scheduler: drained") {
		t.Fatalf("child stderr missing scheduler lifecycle lines:\n%s", childStderr.String())
	}
}

// shellLLM serves one OpenAI-chat-completions response requesting a
// run_shell tool call (a real `echo` command), then plain content.
func shellLLM(t *testing.T) *httptest.Server {
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
				"id": "chatcmpl-sh", "object": "chat.completion", "model": "fake-model",
				"choices": []map[string]any{{
					"index": 0,
					"message": map[string]any{
						"role": "assistant", "content": "",
						"tool_calls": []map[string]any{{
							"id": "call-sh", "type": "function",
							"function": map[string]any{"name": "run_shell", "arguments": `{"command":"echo child-shell-ok"}`},
						}},
					},
					"finish_reason": "tool_calls",
				}},
				"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-sh2", "object": "chat.completion", "model": "fake-model",
			"choices": []map[string]any{{
				"index": 0, "message": map[string]any{"role": "assistant", "content": "shell done"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 1, "total_tokens": 4},
		})
	}))
}

// TestChildProcessRunShellRoundTrip drives run_shell — the REAL shell
// tool registered in B1 — through the actual binary: the fake LLM
// requests `echo child-shell-ok`, bash executes it for real, and the
// structured outcome (cause=exit, exitCode=0, stdout) round-trips over
// the wire and lands in the durable log.
func TestChildProcessRunShellRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and spawns the daemon binary")
	}
	llm := shellLLM(t)
	defer llm.Close()

	bin := filepath.Join(t.TempDir(), "vh-agentd")
	build := exec.Command("go", "build", "-o", bin, "./cmd/vh-agentd")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	sessDir := filepath.Join(t.TempDir(), "sessions")
	logPath := filepath.Join(sessDir, "sess-shell.jsonl")
	cmd := exec.Command(bin,
		"--adapter", "openai",
		"--model", "fake-model",
		"--base-url", llm.URL,
		"--api-key-env", "VH_AGENTD_TEST_KEY",
		"--session-dir", sessDir,
	)
	cmd.Env = append(os.Environ(), "VH_AGENTD_TEST_KEY=k")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	var childStderr syncBuf
	cmd.Stderr = &childStderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	client := protocol.NewClient(childPipes{stdout.(io.ReadCloser), stdin.(io.WriteCloser)})
	waitExit := make(chan error, 1)
	go func() { waitExit <- cmd.Wait() }()
	defer func() {
		_ = client.Close()
		select {
		case <-waitExit:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
		}
	}()

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("call: %v (child stderr:\n%s)", err, childStderr.String())
		}
	}
	must(client.Call("initialize", map[string]any{"protocolVersion": 1}, nil))
	must(client.Call("session/create", map[string]any{"path": logPath, "sessionId": "sess-shell"}, nil))

	var turn struct {
		Results []tools.Result `json:"results"`
	}
	must(client.Call("session/prompt", map[string]any{"text": "run the echo"}, &turn))
	if len(turn.Results) != 1 || turn.Results[0].IsError {
		t.Fatalf("run_shell result = %+v (stderr:\n%s)", turn.Results, childStderr.String())
	}
	if turn.Results[0].Name != "run_shell" {
		t.Fatalf("tool name = %q, want run_shell", turn.Results[0].Name)
	}
	var outcome struct {
		Cause    string `json:"cause"`
		ExitCode int    `json:"exitCode"`
		Stdout   string `json:"stdout"`
		Sandbox  string `json:"sandbox"`
	}
	if err := json.Unmarshal([]byte(turn.Results[0].Content), &outcome); err != nil {
		t.Fatalf("run_shell outcome not structured JSON: %v (%s)", err, turn.Results[0].Content)
	}
	if outcome.Cause != "exit" || outcome.ExitCode != 0 || outcome.Stdout != "child-shell-ok\n" {
		t.Fatalf("outcome = %+v, want cause=exit code=0 stdout=child-shell-ok\\n", outcome)
	}
	if outcome.Sandbox != "none" {
		t.Fatalf("sandbox = %q, want the documented default none", outcome.Sandbox)
	}

	// The execution intent + frozen outcome are durable.
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(raw), `"name":"run_shell"`) || !strings.Contains(string(raw), "child-shell-ok") {
		t.Fatalf("run_shell round-trip missing from the durable log:\n%s", raw)
	}

	if err := stdin.Close(); err != nil {
		t.Fatalf("close stdin: %v", err)
	}
	select {
	case werr := <-waitExit:
		if werr != nil {
			t.Fatalf("child exit error: %v (stderr:\n%s)", werr, childStderr.String())
		}
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("child did not exit after EOF (stderr:\n%s)", childStderr.String())
	}
}

// TestChildProcessFailClosedRuns the fail-closed contract at the REAL
// binary level: missing --session-dir exits 2 with the message.
func TestChildProcessFailClosed(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and spawns the daemon binary")
	}
	bin := filepath.Join(t.TempDir(), "vh-agentd")
	build := exec.Command("go", "build", "-o", bin, "./cmd/vh-agentd")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	cmd := exec.Command(bin,
		"--adapter", "openai", "--model", "m",
		"--base-url", "http://127.0.0.1:1", "--api-key-env", "VH_AGENTD_TEST_KEY",
	)
	var stderr syncBuf
	cmd.Stderr = &stderr
	code := 0
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("run: %v", err)
		}
	}
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--session-dir is required") {
		t.Fatalf("stderr = %s", stderr.String())
	}
}
