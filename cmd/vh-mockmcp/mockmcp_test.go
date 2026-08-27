// mockmcp_test.go — unit coverage for the mock's one protocol brain
// and both framings. These tests run BEFORE the host exists (the
// vh-agentd e2e crux exercises the same binary end-to-end later).
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDispatchInitializeAndHandshake(t *testing.T) {
	resp, garbage := dispatch(mockConfig{}, &jsonrpcRequest{
		JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize",
		Params: json.RawMessage(`{"protocolVersion":"2025-06-18"}`),
	})
	if garbage {
		t.Fatal("clean config produced garbage")
	}
	var res struct {
		ProtocolVersion string `json:"protocolVersion"`
		ServerInfo      struct {
			Name string `json:"name"`
		} `json:"serverInfo"`
	}
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		t.Fatalf("initialize result: %v", err)
	}
	if res.ProtocolVersion != protocolVersion || res.ServerInfo.Name != "vh-mockmcp" {
		t.Fatalf("initialize result = %+v", res)
	}
	// Notification: no response.
	resp, garbage = dispatch(mockConfig{}, &jsonrpcRequest{JSONRPC: "2.0", Method: "notifications/initialized"})
	if garbage || resp != nil {
		t.Fatalf("notification produced resp=%v garbage=%v", resp, garbage)
	}
}

func TestDispatchToolsListCannedSetAndFlags(t *testing.T) {
	list := func(cfg mockConfig) []string {
		resp, _ := dispatch(cfg, &jsonrpcRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "tools/list"})
		var res struct {
			Tools []toolDesc `json:"tools"`
		}
		if err := json.Unmarshal(resp.Result, &res); err != nil {
			t.Fatalf("tools/list result: %v", err)
		}
		names := make([]string, 0, len(res.Tools))
		for _, tl := range res.Tools {
			names = append(names, tl.Name)
		}
		return names
	}
	base := list(mockConfig{})
	if strings.Join(base, ",") != "echo,slow,fail" {
		t.Fatalf("base tool set = %v", base)
	}
	withFlags := list(mockConfig{badSchema: true, envTool: true})
	if strings.Join(withFlags, ",") != "echo,slow,fail,weird,env" {
		t.Fatalf("flagged tool set = %v", withFlags)
	}
}

func TestDispatchToolsCallPaths(t *testing.T) {
	call := func(args string) (*jsonrpcResponse, bool) {
		return dispatch(mockConfig{}, &jsonrpcRequest{
			JSONRPC: "2.0", ID: json.RawMessage("9"), Method: "tools/call",
			Params: json.RawMessage(args),
		})
	}
	// echo happy path.
	resp, _ := dispatch(mockConfig{}, &jsonrpcRequest{
		JSONRPC: "2.0", ID: json.RawMessage("9"), Method: "tools/call",
		Params: json.RawMessage(`{"name":"echo","arguments":{"text":"hi"}}`),
	})
	var cr callResult
	if err := json.Unmarshal(resp.Result, &cr); err != nil {
		t.Fatalf("echo result: %v", err)
	}
	if len(cr.Content) != 1 || cr.Content[0].Text != "echo: hi" || cr.IsError {
		t.Fatalf("echo = %+v", cr)
	}
	// fail tool: isError true with the message.
	resp, _ = dispatch(mockConfig{}, &jsonrpcRequest{
		JSONRPC: "2.0", ID: json.RawMessage("9"), Method: "tools/call",
		Params: json.RawMessage(`{"name":"fail","arguments":{"message":"boom"}}`),
	})
	if err := json.Unmarshal(resp.Result, &cr); err != nil {
		t.Fatalf("fail result: %v", err)
	}
	if !cr.IsError || cr.Content[0].Text != "boom" {
		t.Fatalf("fail = %+v", cr)
	}
	// unknown tool: JSON-RPC error.
	resp, _ = call(`{"name":"nope","arguments":{}}`)
	if resp.Error == nil || resp.Error.Code != -32602 {
		t.Fatalf("unknown tool = %+v", resp)
	}
	// garbage modes.
	_, garbage := dispatch(mockConfig{garbage: true}, &jsonrpcRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize"})
	if !garbage {
		t.Fatal("--garbage did not fire on initialize")
	}
	_, garbage = dispatch(mockConfig{callGarb: true}, &jsonrpcRequest{
		JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "tools/call",
		Params: json.RawMessage(`{"name":"echo","arguments":{"text":"x"}}`),
	})
	if !garbage {
		t.Fatal("--call-garbage did not fire on tools/call")
	}
	// call-garbage leaves initialize healthy.
	_, garbage = dispatch(mockConfig{callGarb: true}, &jsonrpcRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize"})
	if garbage {
		t.Fatal("--call-garbage broke initialize")
	}
}

func TestServeStdioRoundTrip(t *testing.T) {
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{"text":"round-trip"}}}`,
	}, "\n") + "\n"
	var out bytes.Buffer
	if err := serveStdio(mockConfig{}, strings.NewReader(in), &out, io_discard()); err != nil {
		t.Fatalf("serveStdio: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 { // initialize, tools/list, tools/call — no notification response
		t.Fatalf("response lines = %d (%q)", len(lines), out.String())
	}
	if !strings.Contains(lines[2], "echo: round-trip") {
		t.Fatalf("echo response line = %q", lines[2])
	}
}

func TestServeStdioGarbageMode(t *testing.T) {
	in := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n"
	var out bytes.Buffer
	if err := serveStdio(mockConfig{garbage: true}, strings.NewReader(in), &out, io_discard()); err != nil {
		t.Fatalf("serveStdio: %v", err)
	}
	if !strings.Contains(out.String(), garbageLine) {
		t.Fatalf("garbage line missing: %q", out.String())
	}
	if json.Valid([]byte(strings.TrimSpace(out.String()))) {
		t.Fatal("garbage line unexpectedly parsed as JSON")
	}
}

func TestHTTPHandlerJSONAndSSE(t *testing.T) {
	post := func(cfg mockConfig, body string) (*http.Response, string) {
		srv := httptest.NewServer(newHTTPHandler(cfg))
		defer srv.Close()
		resp, err := http.Post(srv.URL+"/mcp", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		defer resp.Body.Close()
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(resp.Body)
		return resp, buf.String()
	}
	// JSON framing: single response object.
	resp, body := post(mockConfig{}, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Type"), "application/json") {
		t.Fatalf("json framing: status=%d ct=%s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if !strings.Contains(body, `"echo"`) {
		t.Fatalf("tools/list body = %q", body)
	}
	// SSE framing: heartbeat comment + data frame.
	resp, body = post(mockConfig{sse: true}, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("sse framing: status=%d ct=%s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if !strings.HasPrefix(body, ": mock heartbeat") || !strings.Contains(body, "data: ") {
		t.Fatalf("sse body = %q", body)
	}
	// Notification: 202 no body.
	resp, _ = post(mockConfig{}, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("notification status = %d", resp.StatusCode)
	}
	// Garbage: 500 junk.
	resp, body = post(mockConfig{garbage: true}, `{"jsonrpc":"2.0","id":1,"method":"initialize"}`)
	if resp.StatusCode != 500 || !strings.Contains(body, garbageLine) {
		t.Fatalf("garbage status=%d body=%q", resp.StatusCode, body)
	}
	// Healthz.
	srv := httptest.NewServer(newHTTPHandler(mockConfig{}))
	defer srv.Close()
	hr, err := http.Get(srv.URL + "/healthz")
	if err != nil || hr.StatusCode != 200 {
		t.Fatalf("healthz: %v %v", err, hr)
	}
	_ = hr.Body.Close()
}

func TestHTTPRequireSessionDiscipline(t *testing.T) {
	srv := httptest.NewServer(newHTTPHandler(mockConfig{reqSess: true}))
	defer srv.Close()
	// initialize assigns the session id...
	resp, err := http.Post(srv.URL+"/mcp", "application/json",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	_ = resp.Body.Close()
	if resp.Header.Get("Mcp-Session-Id") == "" {
		t.Fatal("initialize did not assign Mcp-Session-Id")
	}
	// ...and a follow-up without it 400s (the real-server behavior).
	resp2, err := http.Post(srv.URL+"/mcp", "application/json",
		strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`))
	if err != nil {
		t.Fatalf("POST 2: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadRequest {
		t.Fatalf("sessionless tools/list status = %d", resp2.StatusCode)
	}
	// With the id echoed, it succeeds.
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":3,"method":"tools/list"}`))
	req.Header.Set("Mcp-Session-Id", "mock-session-id-123456")
	resp3, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST 3: %v", err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != 200 {
		t.Fatalf("sessionful tools/list status = %d", resp3.StatusCode)
	}
}

func io_discard() *bufio.Writer { return bufio.NewWriter(bytes.NewBuffer(nil)) }
