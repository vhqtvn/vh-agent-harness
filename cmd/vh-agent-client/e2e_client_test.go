// e2e_client_test.go — the BINARY-level end-to-end battery: build the
// REAL vh-agent-client and vh-agentd binaries, spawn the client as a
// child process, and let IT spawn the daemon (--exec). Seam honesty:
//
//   - one-shot / --json / REPL-EOF / usage / resume tests use the REAL
//     daemon binary with its shipped tool set (echo — never asks);
//   - the approval GRANT/DENY pair over a REAL connection lives at the
//     LIBRARY-SERVER seam (driver_test.go: the daemon's shipped tools
//     never ask, so an ask-verdict engine composition is the honest
//     seam for responder proofs);
//   - the real-binary --json approval path is exercised by the same
//     library seam via jsonApprover in approval_test.go + the json
//     renderer contract in render_test.go.
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// buildBinaries builds the real client and daemon binaries into a
// per-test temp dir (never the worktree root).
func buildBinaries(t *testing.T) (clientBin, daemonBin string) {
	t.Helper()
	dir := t.TempDir()
	clientBin = filepath.Join(dir, "vh-agent-client")
	daemonBin = filepath.Join(dir, "vh-agentd")
	for _, target := range []struct{ path, pkg string }{
		{clientBin, "./cmd/vh-agent-client"},
		{daemonBin, "./cmd/vh-agentd"},
	} {
		build := exec.Command("go", "build", "-o", target.path, target.pkg)
		build.Dir = "../.."
		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("go build %s: %v\n%s", target.pkg, err, out)
		}
	}
	return clientBin, daemonBin
}

// runClient spawns the real client binary with the given args and
// stdin, returning (exit code, stdout, stderr).
func runClient(t *testing.T, clientBin string, args []string, stdin string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(clientBin, args...)
	cmd.Env = append(os.Environ(), "VH_AGENTD_TEST_KEY=test-key-e2e")
	cmd.Stdin = strings.NewReader(stdin)
	var out, errbuf syncBuf
	cmd.Stdout = &out
	cmd.Stderr = &errbuf
	if err := cmd.Start(); err != nil {
		t.Fatalf("start client: %v", err)
	}
	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()
	var werr error
	select {
	case werr = <-waitCh:
	case <-time.After(60 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("client did not exit within 60s (stdout:\n%s\nstderr:\n%s)", out.String(), errbuf.String())
	}
	code := 0
	if werr != nil {
		if ee, ok := werr.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("client wait: %v", werr)
		}
	}
	return code, out.String(), errbuf.String()
}

// TestClientBinaryOneShotEndToEnd is the R3-gate CRUX: a REAL spawned
// daemon + scripted LLM + one-shot prompt through the REAL client
// binary → the echo tool round-trip is visible in the rendered events
// (stderr) → the final assistant text lands on stdout → exit 0.
func TestClientBinaryOneShotEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and spawns real binaries")
	}
	clientBin, daemonBin := buildBinaries(t)

	// Scripted LLM: call 1 requests the echo tool; call 2 returns the
	// final content.
	var mu sync.Mutex
	calls := 0
	llm := startHTTPStub(t, func(w *jsonEncoder) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n == 1 {
			w.toolCall("call-e2e", "echo", `{"text":"e2e-round-trip"}`)
			return
		}
		w.content("final answer: e2e")
	})
	defer llm.Close()

	sessDir := filepath.Join(t.TempDir(), "sessions")
	code, out, errbuf := runClient(t, clientBin, []string{
		"--session-dir", sessDir,
		"--prompt", "child: echo something",
		"--exec", daemonBin,
		"--adapter", "openai", "--model", "fake-model",
		"--base-url", llm.URL,
		"--api-key-env", "VH_AGENTD_TEST_KEY",
	}, "")

	if code != 0 {
		t.Fatalf("client exit = %d, want 0 (stdout:\n%s\nstderr:\n%s)", code, out, errbuf)
	}
	if out != "final answer: e2e\n" {
		t.Fatalf("stdout = %q, want the final assistant text only (stderr:\n%s)", out, errbuf)
	}
	for _, want := range []string{
		"→ prompt child: echo something",
		"⚙ tool echo text=e2e-round-trip",
		"✔ tool result (14 bytes)", // len("e2e-round-trip") == 14
		"vh-agent-client: session sess-",
		".jsonl",
	} {
		if !strings.Contains(errbuf, want) {
			t.Fatalf("rendered stream missing %q:\n%s", want, errbuf)
		}
	}
	// The pointer file exists under the session dir.
	if _, err := os.Stat(filepath.Join(sessDir, lastSessionFile)); err != nil {
		t.Fatalf("last-session pointer missing: %v", err)
	}
}

// TestClientBinaryJSONMode drives the real pair in --json mode: every
// stdout line is valid NDJSON, the tool/call event is visible on the
// stream, and the final prompt-result object is the last line.
func TestClientBinaryJSONMode(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and spawns real binaries")
	}
	clientBin, daemonBin := buildBinaries(t)

	var mu sync.Mutex
	calls := 0
	llm := startHTTPStub(t, func(w *jsonEncoder) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n == 1 {
			w.toolCall("call-j", "echo", `{"text":"json-mode"}`)
			return
		}
		w.content("json final")
	})
	defer llm.Close()

	sessDir := filepath.Join(t.TempDir(), "sessions")
	code, out, errbuf := runClient(t, clientBin, []string{
		"--session-dir", sessDir,
		"--json",
		"--prompt", "json: echo something",
		"--exec", daemonBin,
		"--adapter", "openai", "--model", "fake-model",
		"--base-url", llm.URL,
		"--api-key-env", "VH_AGENTD_TEST_KEY",
	}, "")

	if code != 0 {
		t.Fatalf("client exit = %d, want 0 (stdout:\n%s\nstderr:\n%s)", code, out, errbuf)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected several NDJSON lines, got %q", out)
	}
	sawToolCall := false
	for _, ln := range lines {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal([]byte(ln), &obj); err != nil {
			t.Fatalf("stdout line is not valid JSON: %q (%v)", ln, err)
		}
		if typ, ok := obj["type"]; ok && strings.Contains(string(typ), "tool/call") {
			sawToolCall = true
		}
	}
	if !sawToolCall {
		t.Fatalf("no tool/call event on the NDJSON stream:\n%s", out)
	}
	var last map[string]json.RawMessage
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &last); err != nil {
		t.Fatalf("last line not JSON: %q (%v)", lines[len(lines)-1], err)
	}
	if string(last["kind"]) != `"prompt-result"` || string(last["content"]) != `"json final"` {
		t.Fatalf("last line = %q, want the prompt-result object", lines[len(lines)-1])
	}
}

// TestClientBinaryUsageExitTwo: an unknown flag exits 2 without
// spawning anything.
func TestClientBinaryUsageExitTwo(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and spawns real binaries")
	}
	clientBin, _ := buildBinaries(t)
	code, _, errbuf := runClient(t, clientBin, []string{"--nope"}, "")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (stderr: %s)", code, errbuf)
	}
}

// TestClientBinaryResumeRefused: --resume exits 2 with the P4 message.
func TestClientBinaryResumeRefused(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and spawns real binaries")
	}
	clientBin, daemonBin := buildBinaries(t)
	code, _, errbuf := runClient(t, clientBin, []string{
		"--session-dir", filepath.Join(t.TempDir(), "s"),
		"--prompt", "x",
		"--resume",
		"--exec", daemonBin,
		"--adapter", "openai", "--model", "m",
		"--base-url", "http://127.0.0.1:1",
		"--api-key-env", "VH_AGENTD_TEST_KEY",
	}, "")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (stderr:\n%s)", code, errbuf)
	}
	if !strings.Contains(errbuf, "session/resume") || !strings.Contains(errbuf, "P4") {
		t.Fatalf("refusal must name session/resume and P4:\n%s", errbuf)
	}
}

// TestClientBinaryREPLImmediateEOF: the REPL on an immediately-closed
// stdin exits cleanly (exit 0) and the daemon follows its EOF ladder
// (settleDaemon maps a non-zero daemon exit to 1, so exit 0 proves the
// whole ladder worked).
func TestClientBinaryREPLImmediateEOF(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and spawns real binaries")
	}
	clientBin, daemonBin := buildBinaries(t)

	llm := startHTTPStub(t, func(w *jsonEncoder) { w.content("unused") })
	defer llm.Close()

	code, out, errbuf := runClient(t, clientBin, []string{
		"--session-dir", filepath.Join(t.TempDir(), "s"),
		"--repl",
		"--exec", daemonBin,
		"--adapter", "openai", "--model", "fake-model",
		"--base-url", llm.URL,
		"--api-key-env", "VH_AGENTD_TEST_KEY",
	}, "")
	if code != 0 {
		t.Fatalf("client exit = %d, want 0 (stdout:\n%s\nstderr:\n%s)", code, out, errbuf)
	}
	if !strings.Contains(errbuf, "input closed — exiting cleanly") {
		t.Fatalf("honest EOF message missing:\n%s", errbuf)
	}
	if out != "" {
		t.Fatalf("REPL mode must leave stdout empty (machine content only), got %q", out)
	}
}

// runClientInDir spawns the real client binary with a controlled
// working directory (the default session dir resolves against the
// CLIENT's cwd — that resolution is under test), returning (exit
// code, stdout, stderr).
func runClientInDir(t *testing.T, clientBin, dir string, args []string, stdin string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(clientBin, args...)
	cmd.Env = append(os.Environ(), "VH_AGENTD_TEST_KEY=test-key-e2e")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)
	var out, errbuf syncBuf
	cmd.Stdout = &out
	cmd.Stderr = &errbuf
	if err := cmd.Start(); err != nil {
		t.Fatalf("start client: %v", err)
	}
	werr := cmd.Wait()
	code := 0
	if werr != nil {
		ee, ok := werr.(*exec.ExitError)
		if !ok {
			t.Fatalf("client wait: %v", werr)
		}
		code = ee.ExitCode()
	}
	return code, out.String(), errbuf.String()
}

// TestClientBinaryDefaultSessionDirBoots (hotfix b-F2): the DOCUMENTED
// default invocation — no --session-dir flag at all — must boot the
// daemon and complete the turn (exit 0). The default
// `.vh-agent-sessions` is relative; the daemon hard-rejects relative
// session dirs, so before the fix this invocation exited 2 before
// protocol init. The client now resolves the dir to an absolute path
// (from its own cwd) before daemon argv assembly.
func TestClientBinaryDefaultSessionDirBoots(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and spawns real binaries")
	}
	clientBin, daemonBin := buildBinaries(t)

	llm := startHTTPStub(t, func(w *jsonEncoder) { w.content("default-dir ok") })
	defer llm.Close()

	cwd := t.TempDir()
	code, out, errbuf := runClientInDir(t, clientBin, cwd, []string{
		"--prompt", "hello",
		"--exec", daemonBin,
		"--adapter", "openai", "--model", "fake-model",
		"--base-url", llm.URL,
		"--api-key-env", "VH_AGENTD_TEST_KEY",
	}, "")
	if code != 0 {
		t.Fatalf("default-invocation exit = %d, want 0 (stdout:\n%s\nstderr:\n%s)", code, out, errbuf)
	}
	if out != "default-dir ok\n" {
		t.Fatalf("stdout = %q, want the final assistant text (stderr:\n%s)", out, errbuf)
	}
	// The session landed under the RESOLVED default dir at the cwd,
	// and the client's last-session pointer lives there too.
	sessDir := filepath.Join(cwd, defaultSessionDir)
	if _, err := os.Stat(filepath.Join(sessDir, lastSessionFile)); err != nil {
		t.Fatalf("last-session pointer missing under the resolved default dir %s: %v (stderr:\n%s)", sessDir, err, errbuf)
	}
}

// TestClientBinaryRelativeSessionDirFlag (hotfix b-F2): a
// user-supplied RELATIVE --session-dir is resolved by the client
// against its cwd (not rejected, not forwarded verbatim) — the daemon
// contract stays "absolute only".
func TestClientBinaryRelativeSessionDirFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and spawns real binaries")
	}
	clientBin, daemonBin := buildBinaries(t)

	llm := startHTTPStub(t, func(w *jsonEncoder) { w.content("relative-dir ok") })
	defer llm.Close()

	cwd := t.TempDir()
	code, out, errbuf := runClientInDir(t, clientBin, cwd, []string{
		"--session-dir", "rel-sessions",
		"--prompt", "hello",
		"--exec", daemonBin,
		"--adapter", "openai", "--model", "fake-model",
		"--base-url", llm.URL,
		"--api-key-env", "VH_AGENTD_TEST_KEY",
	}, "")
	if code != 0 {
		t.Fatalf("relative-flag exit = %d, want 0 (stdout:\n%s\nstderr:\n%s)", code, out, errbuf)
	}
	if out != "relative-dir ok\n" {
		t.Fatalf("stdout = %q, want the final assistant text (stderr:\n%s)", out, errbuf)
	}
	if _, err := os.Stat(filepath.Join(cwd, "rel-sessions", lastSessionFile)); err != nil {
		t.Fatalf("last-session pointer missing under the resolved relative dir: %v (stderr:\n%s)", err, errbuf)
	}
}

// --- test doubles ------------------------------------------------------------
//
// (Kept local to this file: syncBuf mirrors cmd/vh-agentd's, and the
// LLM stub is a small scripted encoder over httptest.)

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
