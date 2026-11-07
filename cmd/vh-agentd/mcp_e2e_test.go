// mcp_e2e_test.go — the P8 crux tests at the daemon level: a REAL
// vh-mockmcp subprocess (stdio) + a REAL vh-mockmcp HTTP server (both
// content shapes) discovered through the daemon's own flag surface,
// driven through the REAL pipeline (buildServer + RunTurn), the
// degraded fail-closed twin with a bounded-duration assertion, the
// slow-tool timeout proof, and the flag/validation surface.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
	"github.com/vhqtvn/vh-agent-harness/internal/mcp"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// mockBinaries builds vh-mockmcp ONCE per test run. The binary lands
// OUTSIDE any single test's TempDir (those are reaped per-test; the
// cached path must outlive the suite — the internal/mcp pattern).
var mockBinaries struct {
	once sync.Once
	dir  string
	err  error
}

func mockBinDir(t *testing.T) string {
	t.Helper()
	mockBinaries.once.Do(func() {
		dir, err := os.MkdirTemp("", "vh-agentd-mockmcp-")
		if err != nil {
			mockBinaries.err = err
			return
		}
		build := exec.Command("go", "build", "-o", filepath.Join(dir, "vh-mockmcp"), "./cmd/vh-mockmcp")
		build.Dir = "../.."
		if out, err := build.CombinedOutput(); err != nil {
			mockBinaries.err = fmt.Errorf("go build vh-mockmcp: %v\n%s", err, out)
			return
		}
		mockBinaries.dir = dir
	})
	if mockBinaries.err != nil {
		t.Fatalf("%v", mockBinaries.err)
	}
	return mockBinaries.dir
}

// startMockMCPHTTP runs the REAL vh-mockmcp over HTTP on an ephemeral
// port and returns its base URL (it prints "listening <addr>" on
// stderr before serving).
func startMockMCPHTTP(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command(filepath.Join(mockBinDir(t), "vh-mockmcp"), append([]string{"--http", "127.0.0.1:0"}, args...)...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start mockmcp: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	line, err := bufio.NewReader(stderr).ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "listening ") {
		t.Fatalf("mockmcp did not report its address: %q %v", line, err)
	}
	return "http://" + strings.TrimSpace(strings.TrimPrefix(line, "listening "))
}

// mcpTestConfig writes an MCP config file with BOTH transport shapes
// (local stdio via the real binary; remote with a fake token in the
// URL path — the redaction proof) and returns its path + the token.
func mcpTestConfig(t *testing.T, remote string, extra map[string]map[string]any) (path, token string) {
	t.Helper()
	mockBin := filepath.Join(mockBinDir(t), "vh-mockmcp")
	servers := map[string]map[string]any{
		"localmock": {"type": "local", "command": []string{mockBin, "--stdio"}},
	}
	if remote != "" {
		token = "fake-path-token-4f5e6d7c8b9a"
		servers["remotemock"] = map[string]any{
			"type": "remote",
			"url":  remote + "/" + token + "/mcp",
		}
	}
	for name, sc := range extra {
		servers[name] = sc
	}
	dir := t.TempDir()
	raw, err := json.MarshalIndent(servers, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "mcp-config.json")
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return p, token
}

// connectRegistry runs the daemon's own setup path (setupMCP semantics
// with an EXPLICIT path — no HOME dependence) and closes at cleanup.
func connectRegistry(t *testing.T, path string, timeoutMs int) *mcp.Registry {
	t.Helper()
	reg, err := setupMCP(path, timeoutMs, newLogger(io.Discard))
	if err != nil {
		t.Fatalf("setupMCP: %v", err)
	}
	if reg == nil {
		t.Fatal("setupMCP returned a nil registry for a non-empty explicit config")
	}
	t.Cleanup(func() { _ = reg.Close() })
	return reg
}

// mcpCfg builds a daemon Config with the registry attached.
func mcpCfg(t *testing.T, reg *mcp.Registry) *Config {
	t.Helper()
	cfg, err := validate("openai", "fake-model", "http://x.test", "VH_AGENTD_TEST_KEY", t.TempDir(), "", 0, defaultApprovalTimeoutMs, 0, "off", 65536, "")
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	cfg.MCP = reg
	return cfg
}

// TestMCPRoundTripBothTransports is the slice CRUX: the daemon's REAL
// pipeline (buildServer → RunTurn) executes one MCP tool via EACH
// transport — P8.2 posture: every mcp call ASKS first, and the wire
// client GRANTS (the wire-level mirror of a --policy exact-name allow;
// the granting engine itself is proven over the real binaries by the
// docker battery's mcp scenario). The namespaced names are advertised
// in the turn's tool specs, tool/call + tool/result land on the
// durable log, and replay of the persisted log reproduces the surface
// byte-identically (MCP results are logged content, like every tool).
func TestMCPRoundTripBothTransports(t *testing.T) {
	remote := startMockMCPHTTP(t, "--sse")
	cfgPath, token := mcpTestConfig(t, remote, nil)
	reg := connectRegistry(t, cfgPath, 10000)
	if degraded := reg.Degraded(); len(degraded) != 0 {
		t.Fatalf("degraded servers: %v", degraded)
	}
	cfg := mcpCfg(t, reg)
	_, tracker, approvals, _, _ := bootMCPServe(t, cfg, "grant")

	// Advertised specs carry BOTH namespaced names (this is what the
	// provider request's tools array carries).
	specs := tracker.TurnOptions().Tools
	var haveLocal, haveRemote bool
	for _, s := range specs {
		if s.Name == "mcp_localmock_echo" {
			haveLocal = true
		}
		if s.Name == "mcp_remotemock_echo" {
			haveRemote = true
		}
	}
	if !haveLocal || !haveRemote {
		t.Fatalf("namespaced tools not advertised: local=%v remote=%v (%d specs)", haveLocal, haveRemote, len(specs))
	}

	es, err := tracker.NewSession("", "sess-mcp-crux", io.Discard)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	ad := &scriptedAdapter{next: func(calls int) *adapters.Response {
		switch calls {
		case 0:
			return &adapters.Response{
				Model: "fake-model", FinishReason: "tool_calls",
				ToolCalls: []session.ToolCall{
					{ID: "call_mcp1", Name: "mcp_localmock_echo", Args: json.RawMessage(`{"text":"stdio-payload"}`)},
					{ID: "call_mcp2", Name: "mcp_remotemock_echo", Args: json.RawMessage(`{"text":"http-payload"}`)},
				},
			}
		default:
			return &adapters.Response{Model: "fake-model", FinishReason: "stop", Content: "mcp round trip complete"}
		}
	}}
	ctx := context.Background()
	r1, err := tracker.TurnRunner().RunTurn(ctx, es.Log, ad, tracker.TurnOptions(), "use both mcp transports")
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	// P8.2: BOTH calls asked first (ask-by-default), then executed on
	// the grant.
	if got := waitForApproval(t, approvals); got != "mcp_localmock_echo" {
		t.Fatalf("first approval carried %q, want mcp_localmock_echo", got)
	}
	if got := waitForApproval(t, approvals); got != "mcp_remotemock_echo" {
		t.Fatalf("second approval carried %q, want mcp_remotemock_echo", got)
	}
	select {
	case extra := <-approvals:
		t.Fatalf("unexpected extra approval: %q", extra)
	default:
	}
	if len(r1.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(r1.Results))
	}
	for _, res := range r1.Results {
		if res.IsError {
			t.Fatalf("result errored: %+v", res)
		}
	}
	if !strings.Contains(r1.Results[0].Content, "echo: stdio-payload") {
		t.Fatalf("stdio result = %q", r1.Results[0].Content)
	}
	if !strings.Contains(r1.Results[1].Content, "echo: http-payload") {
		t.Fatalf("http result = %q", r1.Results[1].Content)
	}

	// Durable log: tool/call (pre-execution) + tool/result for each.
	var calls, results int
	for _, ev := range es.Log.Events() {
		switch ev.Type {
		case session.TypeToolCall:
			calls++
		case session.TypeToolResult:
			results++
		}
	}
	if calls != 2 || results != 2 {
		t.Fatalf("log tool/call=%d tool/result=%d, want 2/2", calls, results)
	}

	// Replay determinism: persisted log → same surface bytes.
	liveMsgs, err := es.Log.Surface()
	if err != nil {
		t.Fatalf("live surface: %v", err)
	}
	raw, err := os.ReadFile(es.Path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	replayed, err := session.Replay(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	replayMsgs, err := session.DeriveMessages(replayed)
	if err != nil {
		t.Fatalf("DeriveMessages: %v", err)
	}
	lj, _ := json.Marshal(liveMsgs)
	rj, _ := json.Marshal(replayMsgs)
	if string(lj) != string(rj) {
		t.Fatal("replayed surface differs from the live surface")
	}

	// REDACTION crux twin: the fake URL token never reaches the session
	// log (the URL lives only in the config file).
	if strings.Contains(string(raw), token) {
		t.Fatal("URL token leaked into the session log")
	}
}

// TestMCPDegradedServerFailsClosedNoHang: one config entry points at a
// dead port; the sentinel tool call returns the typed degraded error,
// the turn COMPLETES, and the whole proof is bounded in wall-clock.
// P8.2: the sentinel (mcp_deadmock) carries the namespace prefix and
// therefore ASKS like every mcp tool — granted here so the typed
// degraded error is reached.
func TestMCPDegradedServerFailsClosedNoHang(t *testing.T) {
	deadPort := "http://127.0.0.1:1/mcp" // nothing listens; connection refused fast
	cfgPath, _ := mcpTestConfig(t, "", map[string]map[string]any{
		"deadmock": {"type": "remote", "url": deadPort},
	})
	deadline := time.Now().Add(20 * time.Second)
	reg := connectRegistry(t, cfgPath, 3000)
	if got := reg.Degraded(); len(got) != 1 || got[0] != "deadmock" {
		t.Fatalf("degraded = %v", got)
	}
	cfg := mcpCfg(t, reg)
	_, tracker, _, _, _ := bootMCPServe(t, cfg, "grant")
	es, err := tracker.NewSession("", "sess-mcp-dead", io.Discard)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	ad := &scriptedAdapter{next: func(calls int) *adapters.Response {
		switch calls {
		case 0:
			return &adapters.Response{
				Model: "fake-model", FinishReason: "tool_calls",
				ToolCalls: []session.ToolCall{
					{ID: "call_dead", Name: "mcp_deadmock", Args: json.RawMessage(`{}`)},
					{ID: "call_live", Name: "mcp_localmock_echo", Args: json.RawMessage(`{"text":"still-alive"}`)},
				},
			}
		default:
			return &adapters.Response{Model: "fake-model", FinishReason: "stop", Content: "degraded twin complete"}
		}
	}}
	ctx := context.Background()
	r1, err := tracker.TurnRunner().RunTurn(ctx, es.Log, ad, tracker.TurnOptions(), "call the dead server")
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if len(r1.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(r1.Results))
	}
	dead := r1.Results[0]
	if !dead.IsError || !strings.Contains(dead.Content, "degraded") {
		t.Fatalf("dead-server result not the typed degraded error: %+v", dead)
	}
	if r1.Results[1].IsError || !strings.Contains(r1.Results[1].Content, "echo: still-alive") {
		t.Fatalf("healthy server harmed by the degraded twin: %+v", r1.Results[1])
	}
	if time.Now().After(deadline) {
		t.Fatal("degraded twin exceeded its bounded-duration budget (hang?)")
	}
	// The final turn completed — the model sees the typed errors and
	// finishes normally.
	r2, err := tracker.TurnRunner().RunTurn(ctx, es.Log, ad, tracker.TurnOptions(), "done")
	if err != nil || r2.Response == nil || !strings.Contains(r2.Response.Content, "degraded twin complete") {
		t.Fatalf("final turn: %v %+v", err, r2.Response)
	}
}

// TestMCPSlowToolTimesOut: the slow tool exceeds the per-call bound and
// the pipeline records the ORTHOGONAL timedOut fact (bounded turn).
// P8.2: granted through the ask gate before the slow call runs.
func TestMCPSlowToolTimesOut(t *testing.T) {
	cfgPath, _ := mcpTestConfig(t, "", nil)
	reg := connectRegistry(t, cfgPath, 200) // 200ms per-exchange bound
	cfg := mcpCfg(t, reg)
	_, tracker, _, _, _ := bootMCPServe(t, cfg, "grant")
	es, err := tracker.NewSession("", "sess-mcp-slow", io.Discard)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	ad := &scriptedAdapter{next: func(calls int) *adapters.Response {
		switch calls {
		case 0:
			return &adapters.Response{
				Model: "fake-model", FinishReason: "tool_calls",
				ToolCalls: []session.ToolCall{
					{ID: "call_slow", Name: "mcp_localmock_slow", Args: json.RawMessage(`{"ms":30000}`)},
				},
			}
		default:
			return &adapters.Response{Model: "fake-model", FinishReason: "stop", Content: "slow twin complete"}
		}
	}}
	start := time.Now()
	r1, err := tracker.TurnRunner().RunTurn(context.Background(), es.Log, ad, tracker.TurnOptions(), "call the slow tool")
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	elapsed := time.Since(start)
	if len(r1.Results) != 1 {
		t.Fatalf("results = %d", len(r1.Results))
	}
	res := r1.Results[0]
	if !res.IsError || !res.TimedOut {
		t.Fatalf("slow-tool result = %+v, want isError+timedOut", res)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("timeout not enforced: %v", elapsed)
	}
	r2, err := tracker.TurnRunner().RunTurn(context.Background(), es.Log, ad, tracker.TurnOptions(), "done")
	if err != nil || r2.Response == nil || !strings.Contains(r2.Response.Content, "slow twin complete") {
		t.Fatalf("post-timeout turn: %v %+v", err, r2.Response)
	}
}

// TestMCPFlagsValidation: the timeout flag and the explicitly-passed
// config file fail closed with exit 2; the honest-absent default (no
// config anywhere) runs with zero MCP.
func TestMCPFlagsValidation(t *testing.T) {
	// Timeout bounds.
	if _, err := validateMCPTimeout(-1); err == nil {
		t.Fatal("negative timeout accepted")
	}
	if _, err := validateMCPTimeout(maxMCPTimeoutMs + 1); err == nil {
		t.Fatal("over-cap timeout accepted")
	}
	if v, err := validateMCPTimeout(0); err != nil || v != defaultMCPTimeoutMs {
		t.Fatalf("zero timeout = %d %v", v, err)
	}

	// run() with an explicitly-missing config exits 2 naming the file.
	dir := t.TempDir()
	var out, errBuf safeBuffer
	code := run([]string{
		"--adapter", "openai", "--model", "m", "--base-url", "http://127.0.0.1:1",
		"--api-key-env", "VH_AGENTD_TEST_KEY", "--session-dir", dir,
		"--mcp-config", filepath.Join(dir, "absent.json"),
	}, func(string) string { return "" }, nil, &out, &errBuf)
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (stderr: %s)", code, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "absent.json") {
		t.Fatalf("error does not name the file: %s", errBuf.String())
	}

	// An invalid explicitly-passed file exits 2 with a line locator.
	badPath := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(badPath, []byte("{\n  \"s\": {\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out2, errBuf2 safeBuffer
	code = run([]string{
		"--adapter", "openai", "--model", "m", "--base-url", "http://127.0.0.1:1",
		"--api-key-env", "VH_AGENTD_TEST_KEY", "--session-dir", dir,
		"--mcp-config", badPath,
	}, func(string) string { return "" }, nil, &out2, &errBuf2)
	if code != 2 || !strings.Contains(errBuf2.String(), "line") {
		t.Fatalf("exit = %d stderr = %s", code, errBuf2.String())
	}
}
