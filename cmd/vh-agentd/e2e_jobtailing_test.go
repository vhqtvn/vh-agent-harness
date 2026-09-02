// e2e_jobtailing_test.go — the P6 CRUX battery at the binary level:
// build the real vh-agentd, drive it over the real stdio wire, and
// prove (a) the background round-trip — model calls
// run_shell{background:true}, the receipt is non-blocking (the job is
// still in flight when the turn returns), the output tail pages
// mid-flight with exact cursor arithmetic, settlement lands, the
// job/report notice reaches the surface, and the durable log replays
// byte-identically; (b) the sandbox holds for background jobs (a
// denied write inside a backgrounded call leaves no file and the
// kernel diagnostic is in the tailed output).
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
)

// bgShellLLM serves: call 1 requests a run_shell background dispatch
// with the given args JSON; call 2 plain content.
func bgShellLLM(t *testing.T, args string) *httptest.Server {
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
				"id": "chatcmpl-bg", "object": "chat.completion", "model": "fake-model",
				"choices": []map[string]any{{
					"index": 0,
					"message": map[string]any{
						"role": "assistant", "content": "",
						"tool_calls": []map[string]any{{
							"id": "call-bg", "type": "function",
							"function": map[string]any{"name": "run_shell", "arguments": args},
						}},
					},
					"finish_reason": "tool_calls",
				}},
				"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-bg2", "object": "chat.completion", "model": "fake-model",
			"choices": []map[string]any{{
				"index": 0, "message": map[string]any{"role": "assistant", "content": "background dispatched"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 1, "total_tokens": 4},
		})
	}))
}

// bgHarness is one spawned daemon over the real wire.
type bgHarness struct {
	t           *testing.T
	bin         string
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	client      *protocol.Client
	childStderr *syncBuf
	waitExit    chan error
}

func startBGDaemon(t *testing.T, llmURL string, sessDir, logPath string, extra ...string) *bgHarness {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "vh-agentd")
	build := exec.Command("go", "build", "-o", bin, "./cmd/vh-agentd")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	args := append([]string{
		"--adapter", "openai",
		"--model", "fake-model",
		"--base-url", llmURL,
		"--api-key-env", "VH_AGENTD_TEST_KEY",
		"--session-dir", sessDir,
	}, extra...)
	cmd := exec.Command(bin, args...)
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
	h := &bgHarness{t: t, bin: bin, cmd: cmd, stdin: stdin, client: client, childStderr: &childStderr, waitExit: waitExit}
	t.Cleanup(func() {
		_ = client.Close()
		select {
		case <-waitExit:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
		}
	})
	return h
}

func (h *bgHarness) must(method string, params any, into any) {
	h.t.Helper()
	if err := h.client.Call(method, params, into); err != nil {
		h.t.Fatalf("%s: %v (child stderr:\n%s)", method, err, h.childStderr.String())
	}
}

// jobsStatus fetches the fold snapshot.
func (h *bgHarness) jobsStatus() map[string]string {
	var st struct {
		Jobs []struct {
			JobID string `json:"jobId"`
			State string `json:"state"`
		} `json:"jobs"`
	}
	h.must("jobs/status", nil, &st)
	out := make(map[string]string, len(st.Jobs))
	for _, j := range st.Jobs {
		out[j.JobID] = j.State
	}
	return out
}

// jobChunk is the test-side decode of one jobs/output response.
type jobChunk struct {
	State      string `json:"state"`
	Chunk      string `json:"chunk"`
	Offset     int64  `json:"offset"`
	NextOffset int64  `json:"nextOffset"`
	HasMore    bool   `json:"hasMore"`
	Written    int64  `json:"written"`
}

// jobOutput reads one chunk at offset.
func (h *bgHarness) jobOutput(jobID string, offset int64) jobChunk {
	var ch jobChunk
	h.must("jobs/output", map[string]any{"jobId": jobID, "offset": offset}, &ch)
	return ch
}

// waitJobState polls jobs/status until the job reaches want.
func (h *bgHarness) waitJobState(jobID, want string) {
	h.t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if st := h.jobsStatus(); st[jobID] == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	h.t.Fatalf("job %s never reached %s (states: %v; stderr:\n%s)", jobID, want, h.jobsStatus(), h.childStderr.String())
}

// TestChildProcessBackgroundShellRoundTrip is CRUX (a): the full
// background round-trip over the real binary and wire.
func TestChildProcessBackgroundShellRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and spawns the daemon binary")
	}
	// A deterministic multi-second producer: 5 ticks, 300ms apart
	// (~1.5s total) — long enough to tail mid-flight.
	command := `for i in 1 2 3 4 5; do echo "tick $i"; sleep 0.3; done`
	args, _ := json.Marshal(map[string]any{"command": command, "background": true, "timeout_ms": 30000})
	llm := bgShellLLM(t, string(args))
	defer llm.Close()

	sessDir := filepath.Join(t.TempDir(), "sessions")
	logPath := filepath.Join(sessDir, "sess-bg.jsonl")
	h := startBGDaemon(t, llm.URL, sessDir, logPath)

	h.must("initialize", map[string]any{"protocolVersion": 1}, nil)
	h.must("session/create", map[string]any{"path": logPath, "sessionId": "sess-bg"}, nil)

	promptStart := time.Now()
	var turn struct {
		Results []struct {
			CallID  string `json:"callId"`
			Name    string `json:"name"`
			Content string `json:"content"`
			IsError bool   `json:"isError"`
		} `json:"results"`
	}
	h.must("session/prompt", map[string]any{"text": "start the ticker in background"}, &turn)
	promptElapsed := time.Since(promptStart)

	if len(turn.Results) != 1 || turn.Results[0].IsError {
		t.Fatalf("tool results = %+v (stderr:\n%s)", turn.Results, h.childStderr.String())
	}
	var receipt struct {
		Background bool   `json:"background"`
		JobID      string `json:"jobId"`
		Command    string `json:"command"`
	}
	if err := json.Unmarshal([]byte(turn.Results[0].Content), &receipt); err != nil || !receipt.Background {
		t.Fatalf("tool content is not a background receipt: %s", turn.Results[0].Content)
	}
	if receipt.JobID != "shell-1" {
		t.Fatalf("jobId = %q, want shell-1 (per-kind monotonic)", receipt.JobID)
	}
	// NON-BLOCKING PROOF: the turn returned in well under the
	// producer's ~1.5s runtime, and the job is still in flight.
	if promptElapsed > 1200*time.Millisecond {
		t.Fatalf("session/prompt took %v — the background dispatch blocked the turn", promptElapsed)
	}
	if st := h.jobsStatus(); st[receipt.JobID] == "" || st[receipt.JobID] == "settled" {
		rawLog, _ := os.ReadFile(logPath)
		t.Fatalf("job state right after the turn = %q, want queued|running (in flight)\nlog:\n%s\nstderr:\n%s",
			st[receipt.JobID], rawLog, h.childStderr.String())
	}

	// Mid-flight partial tail: the first ticks are readable while the
	// job still runs.
	deadline := time.Now().Add(10 * time.Second)
	var first jobChunk
	for time.Now().Before(deadline) {
		first = h.jobOutput(receipt.JobID, 0)
		if strings.Contains(first.Chunk, "tick 1") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(first.Chunk, "tick 1") {
		t.Fatalf("no mid-flight output: %+v (stderr:\n%s)", first, h.childStderr.String())
	}
	if first.State != "running" {
		t.Fatalf("mid-flight state = %q, want running", first.State)
	}
	if st := h.jobsStatus(); st[receipt.JobID] == "settled" {
		t.Fatal("job settled before the producer finished — mid-flight proof invalid")
	}

	// Settlement, then the full tail from the mid-flight cursor.
	h.waitJobState(receipt.JobID, "settled")
	var reassembled strings.Builder
	offset := int64(0)
	for {
		ch := h.jobOutput(receipt.JobID, offset)
		if ch.Offset != offset {
			t.Fatalf("chunk offset %d != requested %d", ch.Offset, offset)
		}
		if ch.NextOffset != offset+int64(len(ch.Chunk)) {
			t.Fatalf("nextOffset %d != offset+len %d", ch.NextOffset, offset+int64(len(ch.Chunk)))
		}
		reassembled.WriteString(ch.Chunk)
		if !ch.HasMore {
			if ch.NextOffset != ch.Written {
				t.Fatalf("terminal cursor %d != written %d", ch.NextOffset, ch.Written)
			}
			break
		}
		offset = ch.NextOffset
	}
	want := "tick 1\ntick 2\ntick 3\ntick 4\ntick 5\n"
	if reassembled.String() != want {
		t.Fatalf("reassembled tail = %q, want %q", reassembled.String(), want)
	}

	// job/report in the surface: session/surface flushes the pending
	// report; the notice carries the exit facts Detail.
	var surf struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	h.must("session/surface", nil, &surf)
	found := false
	for _, m := range surf.Messages {
		if strings.Contains(m.Content, "background job shell-1 completed") &&
			strings.Contains(m.Content, "cause=exit exitCode=0") && m.Role == "user" {
			found = true
		}
	}
	if !found {
		t.Fatalf("job/report notice missing from the surface: %+v", surf.Messages)
	}

	// The durable log carries the job lifecycle + report, and replays
	// byte-identically across two --verify-log runs.
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, frag := range []string{`"type":"job/enqueued"`, `"type":"job/started"`, `"type":"job/settled"`, `"type":"job/report"`, `"detail":"cause=exit exitCode=0`} {
		if !strings.Contains(string(raw), frag) {
			t.Fatalf("log missing %s:\n%s", frag, raw)
		}
	}
	v1 := exec.Command(h.bin, "--verify-log", logPath)
	v1.Dir = "../.."
	out1, err1 := v1.CombinedOutput()
	v2 := exec.Command(h.bin, "--verify-log", logPath)
	v2.Dir = "../.."
	out2, err2 := v2.CombinedOutput()
	if err1 != nil || err2 != nil {
		t.Fatalf("verify-log runs: %v / %v\n%s\n%s", err1, err2, out1, out2)
	}
	if string(out1) != string(out2) || len(out1) == 0 {
		t.Fatalf("verify-log not byte-identical across runs:\n%s\n%s", out1, out2)
	}

	// Clean shutdown: EOF ladder.
	if err := h.stdin.Close(); err != nil {
		t.Fatalf("close stdin: %v", err)
	}
	select {
	case werr := <-h.waitExit:
		if werr != nil {
			t.Fatalf("child exit error: %v (stderr:\n%s)", werr, h.childStderr.String())
		}
	case <-time.After(5 * time.Second):
		h.t.Fatal("child did not exit after EOF")
	}
}

// TestChildProcessBackgroundSandboxHolds is CRUX (b): a backgrounded
// write under --sandbox read-only is denied by the kernel — the
// executor child never wrote, the denial is visible in the tailed
// output, and the job settles with the honest exit facts.
func TestChildProcessBackgroundSandboxHolds(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and spawns the daemon binary")
	}
	if !execsandbox.Detect().Available() {
		t.Skip("landlock+seccomp unavailable; confined run skipped (fail-closed path covered in internal/tools/shell)")
	}
	deniedFile := filepath.Join(t.TempDir(), "sbx-bg-marker")
	args, _ := json.Marshal(map[string]any{
		"command":    "echo pwned > " + deniedFile + "; echo after-attempt",
		"background": true,
		"timeout_ms": 20000,
	})
	llm := bgShellLLM(t, string(args))
	defer llm.Close()

	sessDir := filepath.Join(t.TempDir(), "sessions")
	logPath := filepath.Join(sessDir, "sess-bgsbx.jsonl")
	h := startBGDaemon(t, llm.URL, sessDir, logPath, "--sandbox", "read-only")
	h.must("initialize", map[string]any{"protocolVersion": 1}, nil)
	h.must("session/create", map[string]any{"path": logPath, "sessionId": "sess-bgsbx"}, nil)

	var turn struct {
		Results []struct {
			IsError bool   `json:"isError"`
			Content string `json:"content"`
		} `json:"results"`
	}
	h.must("session/prompt", map[string]any{"text": "try the background write"}, &turn)
	if len(turn.Results) != 1 || turn.Results[0].IsError {
		t.Fatalf("background dispatch failed: %+v (stderr:\n%s)", turn.Results, h.childStderr.String())
	}
	var receipt struct {
		JobID string `json:"jobId"`
	}
	_ = json.Unmarshal([]byte(turn.Results[0].Content), &receipt)

	h.waitJobState(receipt.JobID, "settled")

	// The executor child NEVER wrote the marker (kernel denial held).
	if _, err := os.Stat(deniedFile); !os.IsNotExist(err) {
		t.Fatalf("denied write left a file behind (err=%v)", err)
	}
	// The denial is OUTCOME-OBSERVABLE in the tailed output: bash's
	// EACCES diagnostic streamed into the job's channel.
	var tail strings.Builder
	offset := int64(0)
	for {
		ch := h.jobOutput(receipt.JobID, offset)
		tail.WriteString(ch.Chunk)
		if !ch.HasMore {
			break
		}
		offset = ch.NextOffset
	}
	if !strings.Contains(tail.String(), "Permission denied") || !strings.Contains(tail.String(), "after-attempt") {
		t.Fatalf("tailed output missing the kernel denial / continuation: %q", tail.String())
	}
	// The job settles completed with the honest facts: the DENIED
	// WRITE is a normal command outcome (exit 0 here — `;` sequencing
	// runs the trailing echo; the kernel denial itself is the outcome
	// proof above, not the exit code).
	var surf struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	h.must("session/surface", nil, &surf)
	var notice string
	for _, m := range surf.Messages {
		if strings.Contains(m.Content, "background job "+receipt.JobID) {
			notice = m.Content
		}
	}
	if !strings.Contains(notice, "completed") || !strings.Contains(notice, "sandbox=read-only") {
		t.Fatalf("settlement notice = %q, want completed under sandbox=read-only", notice)
	}
}
