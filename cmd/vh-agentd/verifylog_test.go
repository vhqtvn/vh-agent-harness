// verifylog_test.go — the --verify-log read-only mode: replay a real
// session log, print ONE deterministic JSON line, and fail closed on
// any replay error. The determinism crux: two consecutive runs on the
// same file print IDENTICAL bytes.
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// buildSampleLog writes a small but real session log via the session
// writer (header → prompt → llm/response with a tool call → tool result
// → llm/response final → turn/end) and returns its path.
func buildSampleLog(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "sess-verify.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create log: %v", err)
	}
	lg, err := session.NewLog(f, "sess-verify", time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	if _, err := lg.AppendPrompt("verify me"); err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if _, err := lg.AppendLLMResponse("mock-model", "", []session.ToolCall{
		{ID: "call-1", Name: "echo", Args: []byte(`{"text":"hi"}`)},
	}, session.Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5}); err != nil {
		t.Fatalf("llm response: %v", err)
	}
	if _, err := lg.AppendToolResult("call-1", "echo", "hi", false); err != nil {
		t.Fatalf("tool result: %v", err)
	}
	if _, err := lg.AppendLLMResponse("mock-model", "all done", nil, session.Usage{PromptTokens: 9, CompletionTokens: 2, TotalTokens: 11}); err != nil {
		t.Fatalf("final response: %v", err)
	}
	if err := lg.Close(); err != nil {
		t.Fatalf("close log: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}
	return path
}

// TestVerifyLogDeterministicOutput: two consecutive runs on the same
// log print BYTE-IDENTICAL stdout lines and exit 0.
func TestVerifyLogDeterministicOutput(t *testing.T) {
	path := buildSampleLog(t)
	var out1, err1, out2, err2 bytes.Buffer
	code1 := runVerifyLog(path, &out1, &err1)
	code2 := runVerifyLog(path, &out2, &err2)
	if code1 != 0 || code2 != 0 {
		t.Fatalf("exit codes = %d/%d, want 0/0 (stderr1: %s stderr2: %s)", code1, code2, err1.String(), err2.String())
	}
	if out1.Len() == 0 {
		t.Fatal("verify-log printed nothing")
	}
	if out1.String() != out2.String() {
		t.Fatalf("two runs differ:\nrun1: %s\nrun2: %s", out1.String(), out2.String())
	}
	// Exactly ONE line, and it is the documented JSON object.
	lines := strings.Split(strings.TrimRight(out1.String(), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("want exactly one stdout line, got %d:\n%s", len(lines), out1.String())
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(lines[0]), &obj); err != nil {
		t.Fatalf("output is not one JSON object: %v (%s)", err, lines[0])
	}
	for _, key := range []string{"events", "format_version", "surface_sha256", "messages"} {
		if _, ok := obj[key]; !ok {
			t.Fatalf("output missing %q: %s", key, lines[0])
		}
	}
	if string(obj["format_version"]) != "0" {
		t.Fatalf("format_version = %s, want 0", obj["format_version"])
	}
	// 6 events: header, prompt, llm/response, tool/result, llm/response, turn/end? —
	// the writer choreography decides; assert it is a positive number
	// and that messages counts the surface (prompt + 2 assistant + 1 tool = 4).
	var events int
	if err := json.Unmarshal(obj["events"], &events); err != nil || events < 4 {
		t.Fatalf("events = %s (%v), want >= 4", obj["events"], err)
	}
	var msgs int
	if err := json.Unmarshal(obj["messages"], &msgs); err != nil || msgs != 4 {
		t.Fatalf("messages = %s (%v), want 4 (prompt, assistant+toolcall, tool, assistant)", obj["messages"], err)
	}
	if !strings.HasPrefix(string(obj["surface_sha256"]), "\"") || len(obj["surface_sha256"]) != len(`"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"`) {
		t.Fatalf("surface_sha256 = %s, want a sha256 hex string", obj["surface_sha256"])
	}
}

// TestVerifyLogCorruptFailsClosed: a malformed record mid-log exits 1
// with the reason on stderr and NOTHING on stdout.
func TestVerifyLogCorruptFailsClosed(t *testing.T) {
	path := buildSampleLog(t)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	corrupt := append([]byte(nil), raw...)
	corrupt = append(corrupt, []byte("{ not json\n")...)
	corruptPath := filepath.Join(t.TempDir(), "corrupt.jsonl")
	if err := os.WriteFile(corruptPath, corrupt, 0o644); err != nil {
		t.Fatalf("write corrupt: %v", err)
	}
	var out, errb bytes.Buffer
	code := runVerifyLog(corruptPath, &out, &errb)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if out.Len() != 0 {
		t.Fatalf("corrupt log must print nothing on stdout, got %s", out.String())
	}
	if errb.Len() == 0 {
		t.Fatal("corrupt log must state the reason on stderr")
	}
}

// TestVerifyLogMissingFileFailsClosed.
func TestVerifyLogMissingFileFailsClosed(t *testing.T) {
	var out, errb bytes.Buffer
	code := runVerifyLog(filepath.Join(t.TempDir(), "absent.jsonl"), &out, &errb)
	if code != 1 || errb.Len() == 0 {
		t.Fatalf("exit = %d stderr = %q, want 1 + reason", code, errb.String())
	}
}

// TestVerifyLogEmptyFileFailsClosed: a zero-byte log has no
// session/header — refuse rather than verify an empty surface.
func TestVerifyLogEmptyFileFailsClosed(t *testing.T) {
	p := filepath.Join(t.TempDir(), "empty.jsonl")
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	var out, errb bytes.Buffer
	code := runVerifyLog(p, &out, &errb)
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (a headerless log is not a verifiable session)", code)
	}
}

// TestVerifyLogViaRunFlagShape: the --verify-log flag routes through
// run() BEFORE required-flag validation — no --adapter/--model needed,
// no protocol session started.
func TestVerifyLogViaRunFlagShape(t *testing.T) {
	path := buildSampleLog(t)
	var out, errb bytes.Buffer
	code := run([]string{"--verify-log", path}, nil, nil, &out, &errb)
	if code != 0 {
		t.Fatalf("run(--verify-log) = %d, want 0 (stderr: %s)", code, errb.String())
	}
	if !strings.Contains(out.String(), `"surface_sha256"`) {
		t.Fatalf("run(--verify-log) stdout missing the verify line: %s", out.String())
	}
}
