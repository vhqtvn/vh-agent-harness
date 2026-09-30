// e2e_sandbox_test.go — binary-level proof of the --sandbox wiring:
// build the real vh-agentd, start it with --sandbox read-only, and
// drive a run_shell WRITE ATTEMPT through the wire (fake LLM requests
// the call). The kernel must deny the write (EACCES → non-zero exit,
// Permission denied on stderr), the outcome must carry
// sandbox:"read-only", the denial must be durable in the session log,
// and the file must never exist. This exercises the daemon's OWN
// trampoline dispatch (the daemon re-execs ITSELF as the sandbox
// child), not the test binary's.
package main

import (
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

	"github.com/vhqtvn/vh-agent-harness/internal/execsandbox"
	"github.com/vhqtvn/vh-agent-harness/internal/protocol"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// sandboxLLM serves one OpenAI-chat response requesting a run_shell
// call with the given command, then plain content afterwards.
func sandboxLLM(t *testing.T, command string) *httptest.Server {
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
			args, _ := json.Marshal(map[string]string{"command": command})
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "chatcmpl-sb", "object": "chat.completion", "model": "fake-model",
				"choices": []map[string]any{{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "",
						"tool_calls": []map[string]any{{
							"id": "call-sb", "type": "function",
							"function": map[string]any{"name": "run_shell", "arguments": string(args)},
						}},
					},
					"finish_reason": "tool_calls",
				}},
				"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-sb2", "object": "chat.completion", "model": "fake-model",
			"choices": []map[string]any{{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": "done"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 1, "total_tokens": 4},
		})
	}))
}

// TestChildProcessSandboxedRunShellDenial is the binary-level sandbox
// crux: the daemon itself (not the test binary) hosts the trampoline,
// so this proves the full chain flag → SandboxFunc → self re-exec →
// landlock/seccomp → denial → structured outcome → durable log.
func TestChildProcessSandboxedRunShellDenial(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and spawns the daemon binary")
	}
	if !execsandbox.Detect().Available() {
		t.Skip("landlock+seccomp unavailable on this host; kernel denial not demonstrable")
	}

	deniedFile := filepath.Join(t.TempDir(), "e2e-denied")
	llm := sandboxLLM(t, "echo e2e-nope > "+deniedFile)
	defer llm.Close()

	bin := filepath.Join(t.TempDir(), "vh-agentd")
	build := exec.Command("go", "build", "-o", bin, "./cmd/vh-agentd")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	sessDir := filepath.Join(t.TempDir(), "sessions")
	logPath := filepath.Join(sessDir, "sess-sandbox.jsonl")
	cmd := exec.Command(bin,
		"--adapter", "openai",
		"--model", "fake-model",
		"--base-url", llm.URL,
		"--api-key-env", "VH_AGENTD_TEST_KEY",
		"--session-dir", sessDir,
		"--sandbox", "read-only",
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
		if cmd.ProcessState != nil {
			return
		}
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
	must(client.Call("session/create", map[string]any{"path": logPath, "sessionId": "sess-sandbox"}, nil))

	var turn struct {
		Results []tools.Result `json:"results"`
	}
	must(client.Call("session/prompt", map[string]any{"text": "try a write"}, &turn))
	if len(turn.Results) != 1 || turn.Results[0].IsError {
		t.Fatalf("run_shell result = %+v, want the one structured (non-error) denial outcome (stderr:\n%s)", turn.Results, childStderr.String())
	}
	var outcome struct {
		Cause    string `json:"cause"`
		ExitCode int    `json:"exitCode"`
		Stderr   string `json:"stderr"`
		Sandbox  string `json:"sandbox"`
	}
	if err := json.Unmarshal([]byte(turn.Results[0].Content), &outcome); err != nil {
		t.Fatalf("outcome not structured JSON: %v (%s)", err, turn.Results[0].Content)
	}
	if outcome.Cause != "exit" || outcome.ExitCode == 0 {
		t.Fatalf("outcome = %+v, want a NORMAL non-zero exit (honest runtime denial, not a tool error)", outcome)
	}
	if !strings.Contains(outcome.Stderr, "Permission denied") {
		t.Fatalf("stderr lacks the kernel EACCES diagnostic: %+v", outcome)
	}
	if outcome.Sandbox != "read-only" {
		t.Fatalf("sandbox = %q, want read-only", outcome.Sandbox)
	}
	if _, err := os.Stat(deniedFile); !os.IsNotExist(err) {
		t.Fatalf("denied write left a file behind: %s", deniedFile)
	}

	// The denial is durable in the session log. The outcome JSON is
	// embedded (escaped) inside the tool/result content string, so
	// decode the envelope instead of raw-byte matching.
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	foundDenial := false
	for _, line := range strings.Split(string(raw), "\n") {
		if !strings.Contains(line, `"type":"tool/result"`) {
			continue
		}
		var ev struct {
			Payload struct {
				Content string `json:"content"`
			} `json:"payload"`
		}
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		var logged struct {
			Sandbox string `json:"sandbox"`
			Stderr  string `json:"stderr"`
		}
		if err := json.Unmarshal([]byte(ev.Payload.Content), &logged); err != nil {
			continue
		}
		if logged.Sandbox == "read-only" && strings.Contains(logged.Stderr, "Permission denied") {
			foundDenial = true
		}
	}
	if !foundDenial {
		t.Fatalf("sandbox denial missing from the durable log:\n%s", raw)
	}

	// Clean close ladder: EOF → exit 0.
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
