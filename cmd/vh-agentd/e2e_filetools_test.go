// e2e_filetools_test.go — the binary-level e2e for the file tool
// family: build the real vh-agentd binary, start it with an explicit
// --workdir-roots, and drive ONE read tool call through the real
// stdio wire (fake LLM requests the call; the numbered-lines result
// must round-trip and land in the durable log).
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

	"github.com/vhqtvn/vh-agent-harness/internal/protocol"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// fileReadLLM serves one OpenAI-chat-completions response requesting
// a read tool call, then plain content.
func fileReadLLM(t *testing.T, args string) *httptest.Server {
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
				"id": "chatcmpl-fr", "object": "chat.completion", "model": "fake-model",
				"choices": []map[string]any{{
					"index": 0,
					"message": map[string]any{
						"role": "assistant", "content": "",
						"tool_calls": []map[string]any{{
							"id": "call-fr", "type": "function",
							"function": map[string]any{"name": "read", "arguments": args},
						}},
					},
					"finish_reason": "tool_calls",
				}},
				"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-fr2", "object": "chat.completion", "model": "fake-model",
			"choices": []map[string]any{{
				"index": 0, "message": map[string]any{"role": "assistant", "content": "file read done"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 1, "total_tokens": 4},
		})
	}))
}

// TestChildProcessFileToolReadRoundTrip drives the read tool — a REAL
// file tool registered from internal/tools/filetools — through the
// actual binary: the daemon runs with --workdir-roots over a fixture
// tree, the fake LLM requests read, and the numbered-lines result
// round-trips over the wire and lands in the durable log.
func TestChildProcessFileToolReadRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and spawns the daemon binary")
	}

	// Fixture workspace: the --workdir-roots root with a file to read.
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "note.txt"), []byte("first line\nsecond line\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	llm := fileReadLLM(t, `{"path":"docs/note.txt"}`)
	defer llm.Close()

	bin := filepath.Join(t.TempDir(), "vh-agentd")
	build := exec.Command("go", "build", "-o", bin, "./cmd/vh-agentd")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	sessDir := filepath.Join(t.TempDir(), "sessions")
	logPath := filepath.Join(sessDir, "sess-file.jsonl")
	cmd := exec.Command(bin,
		"--adapter", "openai",
		"--model", "fake-model",
		"--base-url", llm.URL,
		"--api-key-env", "VH_AGENTD_TEST_KEY",
		"--session-dir", sessDir,
		"--workdir-roots", root,
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
	must(client.Call("session/create", map[string]any{"path": logPath, "sessionId": "sess-file"}, nil))

	var turn struct {
		Results []tools.Result `json:"results"`
	}
	must(client.Call("session/prompt", map[string]any{"text": "read the note"}, &turn))
	if len(turn.Results) != 1 || turn.Results[0].IsError {
		t.Fatalf("read result = %+v (stderr:\n%s)", turn.Results, childStderr.String())
	}
	if turn.Results[0].Name != "read" {
		t.Fatalf("tool name = %q, want read", turn.Results[0].Name)
	}
	if turn.Results[0].Content != "1: first line\n2: second line\n" {
		t.Fatalf("numbered-lines content = %q", turn.Results[0].Content)
	}

	// The execution intent + frozen numbered-lines outcome are durable.
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(raw), `"name":"read"`) || !strings.Contains(string(raw), "1: first line") {
		t.Fatalf("read round-trip missing from the durable log:\n%s", raw)
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
	// The startup line records the resolved roots.
	if !strings.Contains(childStderr.String(), "workdir-roots="+root) {
		t.Fatalf("startup line missing workdir-roots=%s:\n%s", root, childStderr.String())
	}
}
