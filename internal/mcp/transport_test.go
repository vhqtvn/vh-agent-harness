// transport_test.go — both transports against the REAL vh-mockmcp
// subprocess (stdio) and hand-built httptest servers (HTTP/SSE edge
// shapes): handshake, correlation, fail-closed garbage, per-call
// deadlines, env scrub, and credential redaction.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// mockmcpBin builds the real mock ONCE per test run (the e2e_child
// pattern, cached — transport tests spawn it repeatedly).
var mockmcpBin struct {
	once sync.Once
	path string
	err  error
}

func mockMCP(t *testing.T) string {
	t.Helper()
	mockmcpBin.once.Do(func() {
		bin := filepath.Join(os.TempDir(), fmt.Sprintf("vh-mockmcp-test-%d", os.Getpid()))
		build := exec.Command("go", "build", "-o", bin, "./cmd/vh-mockmcp")
		build.Dir = "../.."
		if out, err := build.CombinedOutput(); err != nil {
			mockmcpBin.err = fmt.Errorf("go build vh-mockmcp: %v\n%s", err, out)
			return
		}
		mockmcpBin.path = bin
	})
	if mockmcpBin.err != nil {
		t.Fatalf("%v", mockmcpBin.err)
	}
	return mockmcpBin.path
}

func localCfg(args ...string) *ServerConfig {
	return &ServerConfig{Type: TransportLocal, Command: append([]string(nil), args...)}
}

func dialStdio(t *testing.T, args ...string) *StdioClient {
	t.Helper()
	sc := localCfg(append([]string{mockMCP(t)}, args...)...)
	c, err := DialStdio(sc, newRedactor(sc), func(string, ...any) {})
	if err != nil {
		t.Fatalf("DialStdio: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestStdioHandshakeListAndCall(t *testing.T) {
	c := dialStdio(t)
	ctx := context.Background()
	if err := c.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	tools, err := c.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 3 || tools[0].Name != "echo" {
		t.Fatalf("tools = %+v", tools)
	}
	res, err := c.CallTool(ctx, "echo", jsonRaw(`{"text":"transport-proof"}`))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if got := JoinContent(res); got != "echo: transport-proof" {
		t.Fatalf("content = %q", got)
	}
}

func TestStdioCallToolIsErrorIsDataNotError(t *testing.T) {
	c := dialStdio(t)
	ctx := context.Background()
	if err := c.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	res, err := c.CallTool(ctx, "fail", jsonRaw(`{"message":"tool-level failure"}`))
	if err != nil {
		t.Fatalf("isError must be data, got transport error: %v", err)
	}
	if !res.IsError || JoinContent(res) != "tool-level failure" {
		t.Fatalf("res = %+v", res)
	}
}

func TestStdioCorrelationSequentialIds(t *testing.T) {
	c := dialStdio(t)
	ctx := context.Background()
	if err := c.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	// Several sequential calls — responses must route to the right
	// call even when the server interleaves (ids are monotonic).
	for i := 0; i < 5; i++ {
		res, err := c.CallTool(ctx, "echo", jsonRaw(fmt.Sprintf(`{"text":"n%d"}`, i)))
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if got := JoinContent(res); got != fmt.Sprintf("echo: n%d", i) {
			t.Fatalf("call %d content = %q", i, got)
		}
	}
}

func TestStdioGarbageModeDegradesInitialize(t *testing.T) {
	c := dialStdio(t, "--garbage")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := c.Initialize(ctx)
	if err == nil {
		t.Fatal("garbage server initialized cleanly")
	}
	if !strings.Contains(err.Error(), "mcp:") {
		t.Fatalf("untyped error: %v", err)
	}
}

func TestStdioCallGarbageFailsCallClosedNotList(t *testing.T) {
	c := dialStdio(t, "--call-garbage")
	ctx := context.Background()
	if err := c.Initialize(ctx); err != nil {
		t.Fatalf("Initialize (must stay healthy): %v", err)
	}
	if _, err := c.ListTools(ctx); err != nil {
		t.Fatalf("ListTools (must stay healthy): %v", err)
	}
	if _, err := c.CallTool(ctx, "echo", jsonRaw(`{"text":"x"}`)); err == nil {
		t.Fatal("garbage tools/call did not fail")
	} else if !strings.Contains(err.Error(), "invalid JSON-RPC") && !strings.Contains(err.Error(), "garbage") {
		t.Fatalf("error not garbage-typed: %v", err)
	}
}

func TestStdioPerCallDeadline(t *testing.T) {
	c := dialStdio(t)
	ctx := context.Background()
	if err := c.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	bounded, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := c.CallTool(bounded, "slow", jsonRaw(`{"ms":2000}`))
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("slow tool call returned within its deadline")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("deadline not enforced: %v", elapsed)
	}
	// The transport stays usable after a timed-out call.
	res, err := c.CallTool(ctx, "echo", jsonRaw(`{"text":"alive"}`))
	if err != nil || JoinContent(res) != "echo: alive" {
		t.Fatalf("post-timeout call: %v %+v", err, res)
	}
}

func TestStdioEnvMergeAfterScrub(t *testing.T) {
	// Parent-secret: present in the TEST process env; the child's clean
	// env must not carry it.
	t.Setenv("PARENT_LEAKED_SECRET", "parent-secret-value-xyz")
	sc := &ServerConfig{
		Type:    TransportLocal,
		Command: []string{mockMCP(t), "--stdio", "--env-tool"},
		Env: map[string]string{
			"MCP_PLAIN_VALUE": "plain-visible",
			"MCP_API_TOKEN":   "configured-secret-value",
		},
	}
	c, err := DialStdio(sc, newRedactor(sc), func(string, ...any) {})
	if err != nil {
		t.Fatalf("DialStdio: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	ctx := context.Background()
	if err := c.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	env := func(name string) string {
		t.Helper()
		res, err := c.CallTool(ctx, "env", jsonRaw(fmt.Sprintf(`{"name":%q}`, name)))
		if err != nil {
			t.Fatalf("env(%s): %v", name, err)
		}
		return JoinContent(res)
	}
	if got := env("MCP_PLAIN_VALUE"); got != "MCP_PLAIN_VALUE=plain-visible" {
		t.Fatalf("configured non-secret env missing: %q", got)
	}
	if got := env("MCP_API_TOKEN"); got != "MCP_API_TOKEN=(unset)" {
		t.Fatalf("secret-named configured env NOT scrubbed: %q", got)
	}
	if got := env("PARENT_LEAKED_SECRET"); got != "PARENT_LEAKED_SECRET=(unset)" {
		t.Fatalf("parent secret leaked through clean env: %q", got)
	}
}

func TestStdioCloseIsIdempotentAndReaps(t *testing.T) {
	sc := localCfg(mockMCP(t), "--stdio")
	c, err := DialStdio(sc, newRedactor(sc), nil)
	if err != nil {
		t.Fatalf("DialStdio: %v", err)
	}
	if err := c.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	select {
	case <-c.readerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("reader goroutine did not exit after Close")
	}
	if err := c.cmd.Wait(); err == nil {
		// Wait was already consumed by Close; a second Wait errors —
		// either way the process is REAPED (no zombie). We only assert
		// Wait does not BLOCK: reaching here proves it.
	}
	// Calls after Close fail closed, fast.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := c.CallTool(ctx, "echo", jsonRaw(`{"text":"x"}`)); err == nil {
		t.Fatal("post-close call succeeded")
	}
}

func TestStdioDyingProcessTypedError(t *testing.T) {
	// A command that exits immediately: initialize must fail typed
	// (with the captured stderr tail), never hang.
	sc := localCfg("/bin/sh", "-c", "echo 'mock server fatal' >&2; exit 3")
	c, err := DialStdio(sc, newRedactor(sc), nil)
	if err != nil {
		t.Fatalf("DialStdio: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err = c.Initialize(ctx)
	if err == nil {
		t.Fatal("dead process initialized")
	}
	if !strings.Contains(err.Error(), "mock server fatal") {
		t.Fatalf("stderr tail missing from typed error: %v", err)
	}
}

// --- HTTP transport ---------------------------------------------------------

func httpCfg(url string, headers map[string]string) *ServerConfig {
	return &ServerConfig{Type: TransportRemote, URL: url, Headers: headers}
}

// mcpHTTP is a minimal hand-built Streamable-HTTP MCP endpoint for the
// edge-shape tests (the REAL vh-mockmcp --http covers the happy paths
// end-to-end in cmd/vh-agentd's crux).
type mcpHTTP struct {
	sse        bool
	heartbeats int
	preResult  int // data frames carrying NON-JSON payloads before the result
}

func (m mcpHTTP) handler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad body", 400)
			return
		}
		switch req.Method {
		case "initialize":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":{"protocolVersion":"%s","capabilities":{},"serverInfo":{"name":"hand","version":"0"}}}`, *req.ID, ClientProtocolVersion)
		case "notifications/initialized":
			w.WriteHeader(202)
		case "tools/list":
			m.writeResult(w, req.ID, `{"tools":[{"name":"echo","description":"d","inputSchema":{"type":"object"}}]}`)
		case "tools/call":
			m.writeResult(w, req.ID, `{"content":[{"type":"text","text":"http-proof"}]}`)
		default:
			m.writeResult(w, req.ID, "")
		}
	})
}

func (m mcpHTTP) writeResult(w http.ResponseWriter, id *int64, result string) {
	if result == "" {
		result = `{}`
	}
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":%s}`, *id, result)
	if !m.sse {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	fl := w.(http.Flusher)
	for i := 0; i < m.heartbeats; i++ {
		_, _ = w.Write([]byte(": keepalive\n\n"))
	}
	for i := 0; i < m.preResult; i++ {
		_, _ = w.Write([]byte("data: not-json-heartbeat\n\n"))
	}
	fmt.Fprintf(w, "data: %s\n\n", body)
	fl.Flush()
}

func TestHTTPJSONResponseBody(t *testing.T) {
	srv := httptest.NewServer(mcpHTTP{}.handler(t))
	defer srv.Close()
	sc := httpCfg(srv.URL+"/mcp", nil)
	c := NewHTTPClient(sc, newRedactor(sc), func(string, ...any) {})
	ctx := context.Background()
	if err := c.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	res, err := c.CallTool(ctx, "echo", jsonRaw(`{"text":"x"}`))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if JoinContent(res) != "http-proof" {
		t.Fatalf("content = %q", JoinContent(res))
	}
}

func TestHTTPSSEResponseWithHeartbeatsAndJunkFrames(t *testing.T) {
	srv := httptest.NewServer(mcpHTTP{sse: true, heartbeats: 2, preResult: 2}.handler(t))
	defer srv.Close()
	sc := httpCfg(srv.URL+"/mcp", nil)
	c := NewHTTPClient(sc, newRedactor(sc), func(string, ...any) {})
	ctx := context.Background()
	if err := c.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	res, err := c.CallTool(ctx, "echo", jsonRaw(`{"text":"x"}`))
	if err != nil {
		t.Fatalf("CallTool through SSE: %v", err)
	}
	if JoinContent(res) != "http-proof" {
		t.Fatalf("content = %q", JoinContent(res))
	}
}

func TestHTTPSSEStreamEndWithoutResultFailsClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(": only heartbeats here\n\n"))
		_, _ = w.Write([]byte("data: not-a-rpc-frame\n\n"))
	}))
	defer srv.Close()
	sc := httpCfg(srv.URL, nil)
	c := NewHTTPClient(sc, newRedactor(sc), nil)
	if err := c.Initialize(context.Background()); err == nil {
		t.Fatal("stream without result initialized cleanly")
	} else if !strings.Contains(err.Error(), "fail-closed") && !strings.Contains(err.Error(), "without a result") {
		t.Fatalf("error not stream-end-typed: %v", err)
	}
}

func TestHTTP500TypedErrorAndRedaction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream exploded with token abc123def456 inside", 500)
	}))
	defer srv.Close()
	// The URL carries a fake token; the DEAD-server error must not
	// leak it (http.Client embeds the URL in transport errors).
	deadURL := strings.Replace(srv.URL, "127.0.0.1:", "127.0.0.1:1/", 1) + "fake-token-deadbeef99/mcp"
	sc := httpCfg(deadURL, nil)
	red := newRedactor(sc)
	c := NewHTTPClient(sc, red, nil)
	err := c.Initialize(context.Background())
	if err == nil {
		t.Fatal("dead endpoint initialized")
	}
	if strings.Contains(err.Error(), "fake-token-deadbeef99") {
		t.Fatalf("URL token leaked into error: %v", err)
	}
	// 500 path: body snippet surfaces but the header VALUE credential redacts.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom with header-value-secret-889900 inside", 500)
	}))
	defer srv2.Close()
	sc2 := httpCfg(srv2.URL, map[string]string{"X-Private": "header-value-secret-889900"})
	c2 := NewHTTPClient(sc2, newRedactor(sc2), nil)
	err = c2.Initialize(context.Background())
	if err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("500 error = %v", err)
	}
	if strings.Contains(err.Error(), "header-value-secret-889900") {
		t.Fatalf("header value leaked into error: %v", err)
	}
}

func TestHTTPConfiguredHeadersRideEveryRequest(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("X-Custom-Auth")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-06-18"}}`))
	}))
	defer srv.Close()
	sc := httpCfg(srv.URL, map[string]string{"X-Custom-Auth": "custom-credential-777"})
	c := NewHTTPClient(sc, newRedactor(sc), nil)
	if err := c.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if gotAuth != "custom-credential-777" {
		t.Fatalf("configured header did not ride the request: %q", gotAuth)
	}
}

func TestHTTPPerCallDeadline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	}))
	defer srv.Close()
	sc := httpCfg(srv.URL, nil)
	c := NewHTTPClient(sc, newRedactor(sc), nil)
	bounded, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := c.Initialize(bounded); err == nil {
		t.Fatal("hung endpoint initialized within deadline")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("deadline not enforced: %v", elapsed)
	}
}

func TestHTTPSessionIDEcho(t *testing.T) {
	// The real-server discipline (discovered against a live endpoint):
	// initialize assigns Mcp-Session-Id; later requests without it 400.
	// The mock's --require-session mirrors it; this handler is its
	// httptest twin proving the HOST echoes the assigned id.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sid := r.Header.Get("Mcp-Session-Id")
		var req rpcRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Method == "initialize" {
			w.Header().Set("Mcp-Session-Id", "sess-assigned-abc123")
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":{"protocolVersion":"%s"}}`, deref(req.ID), ClientProtocolVersion)
			return
		}
		if sid != "sess-assigned-abc123" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","error":{"code":-32600,"message":"Missing session ID"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if req.Method == "tools/call" {
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"error":{"code":-32602,"message":"unknown tool"}}`, deref(req.ID))
			return
		}
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":{"tools":[]}}`, deref(req.ID))
	}))
	defer srv.Close()
	sc := httpCfg(srv.URL, nil)
	c := NewHTTPClient(sc, newRedactor(sc), nil)
	ctx := context.Background()
	if err := c.Initialize(ctx); err != nil {
		t.Fatalf("Initialize through a session-assigning server: %v", err)
	}
	if _, err := c.ListTools(ctx); err != nil {
		t.Fatalf("ListTools without session echo would 400: %v", err)
	}
	if _, err := c.CallTool(ctx, "echo", jsonRaw(`{}`)); err == nil {
		t.Fatal("expected typed error for unknown tool through session (proves the exchange happened)")
	}
}

func deref(i *int64) int64 {
	if i == nil {
		return 0
	}
	return *i
}

func TestHTTPJSONRPCErrorSurfaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"unknown tool"}}`))
	}))
	defer srv.Close()
	sc := httpCfg(srv.URL, nil)
	c := NewHTTPClient(sc, newRedactor(sc), nil)
	_, err := c.CallTool(context.Background(), "nope", jsonRaw(`{}`))
	if err == nil || !strings.Contains(err.Error(), "-32602") || !strings.Contains(err.Error(), "unknown tool") {
		t.Fatalf("json-rpc error = %v", err)
	}
}

// --- shared helpers ----------------------------------------------------------

func jsonRaw(s string) (out json.RawMessage) { return json.RawMessage(s) }

// Ensure the package compiles against the imports the tests lean on.
var _ = bufio.NewReader
