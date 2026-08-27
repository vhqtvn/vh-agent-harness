// dispatch.go — the mock's ONE protocol brain: the canned tool set and
// the JSON-RPC request dispatch shared by both transport framings
// (stdio lines and HTTP POSTs). A request arrives as parsed JSON, the
// response leaves as marshalled JSON — the framings only add wire
// shapes around this.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// protocolVersion is the version this mock negotiates in initialize.
const protocolVersion = "2025-06-18"

// mockConfig carries the fault-injection and tool-set flags.
type mockConfig struct {
	sse              bool // http framing: text/event-stream responses
	garbage          bool // EVERY response is garbage (server-level degrade proof)
	callGarb         bool // tools/call responses only are garbage
	badSchema        bool // advertise the unmappable-schema tool
	envTool          bool // advertise the env readback tool
	reqSess          bool // http framing: assign + REQUIRE Mcp-Session-Id (the real-server discipline)
	notifyDuringCall bool // emit a server-initiated notification BEFORE each tools/call response
}

// notificationLine is the server-initiated notification frame the mock
// interleaves MID-CALL before each tools/call response when
// --notify-during-call is set: a real-MCP-shape progress notification
// (method set, NO id — never a response). Real servers emit exactly
// this class of frame while a host's call is pending; a host that
// mistakes it for garbage and fails pending calls breaks against every
// chatty server. This flag is that proof.
const notificationLine = `{"jsonrpc":"2.0","method":"notifications/progress","params":{"progressToken":"mock-notify-token","progress":50,"total":100}}`

// jsonrpcRequest is one incoming call or notification.
type jsonrpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// jsonrpcResponse is one outgoing result or error.
type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// contentBlock is one MCP tools/call content block (text class only in
// this mock; a non-text block class is a host-side mapping concern).
type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// callResult is the MCP tools/call result envelope.
type callResult struct {
	Content []contentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

// toolDesc is one advertised tool. inputSchema is a JSON Schema object
// (or, for the bad-schema tool, deliberately NOT an object — the
// unmappable case).
type toolDesc struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// cannedTools returns the advertised tool list per cfg. Deterministic
// order: echo, slow, fail, then the optional extras.
func cannedTools(cfg mockConfig) []toolDesc {
	tools := []toolDesc{
		{
			Name:        "echo",
			Description: "Echoes the text argument back as a text content block.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string","description":"text to echo"}},"required":["text"]}`),
		},
		{
			Name:        "slow",
			Description: "Sleeps ms milliseconds (capped at 120000) and then confirms the sleep — the timeout-proof tool.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"ms":{"type":"number","description":"milliseconds to sleep"}},"required":["ms"]}`),
		},
		{
			Name:        "fail",
			Description: "Always returns an isError result carrying the given message — the typed tool-error proof.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"message":{"type":"string"}},"required":["message"]}`),
		},
	}
	if cfg.badSchema {
		tools = append(tools, toolDesc{
			Name:        "weird",
			Description: "Deliberately unmappable inputSchema (a bare string, not an object) — a host must skip this tool with a warning.",
			InputSchema: json.RawMessage(`{"type":"string"}`),
		})
	}
	if cfg.envTool {
		tools = append(tools, toolDesc{
			Name:        "env",
			Description: "Returns the value of the named environment variable in this subprocess, or (unset) — the env-merge-after-scrub proof.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`),
		})
	}
	return tools
}

// garbageLine is the invalid-JSON payload the garbage modes write.
const garbageLine = `{{{{ definitely not json !!!`

// dispatch handles one parsed JSON-RPC request and returns the response
// to write (nil for notifications — they carry no response), or the
// garbage marker when a garbage mode fires. Framings turn the marker
// into their wire-specific junk (invalid JSON line / HTTP 500).
func dispatch(cfg mockConfig, req *jsonrpcRequest) (*jsonrpcResponse, bool) {
	if cfg.garbage {
		return nil, true
	}
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p) // params optional-tolerant; version logged only
		result, _ := json.Marshal(map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{},
			"serverInfo":      map[string]string{"name": "vh-mockmcp", "version": "0.1.0"},
		})
		return &jsonrpcResponse{JSONRPC: "2.0", ID: req.ID, Result: result}, false

	case "notifications/initialized":
		// A notification: no id, no response.
		return nil, false

	case "tools/list":
		tools, _ := json.Marshal(map[string]any{"tools": cannedTools(cfg)})
		return &jsonrpcResponse{JSONRPC: "2.0", ID: req.ID, Result: tools}, false

	case "tools/call":
		if cfg.callGarb {
			return nil, true
		}
		return callTool(req)

	default:
		return &jsonrpcResponse{
			JSONRPC: "2.0", ID: req.ID,
			Error: &jsonrpcError{Code: -32601, Message: fmt.Sprintf("mock: unknown method %q", req.Method)},
		}, false
	}
}

// callTool executes one tools/call against the canned set.
func callTool(req *jsonrpcRequest) (*jsonrpcResponse, bool) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil || p.Name == "" {
		return &jsonrpcResponse{
			JSONRPC: "2.0", ID: req.ID,
			Error: &jsonrpcError{Code: -32602, Message: "mock: tools/call requires {name, arguments}"},
		}, false
	}
	var res callResult
	switch p.Name {
	case "echo":
		var a struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(p.Arguments, &a); err != nil || a.Text == "" {
			return argError(req, "echo", "text (non-empty string)"), false
		}
		res = callResult{Content: []contentBlock{{Type: "text", Text: "echo: " + a.Text}}}

	case "slow":
		var a struct {
			MS float64 `json:"ms"`
		}
		if err := json.Unmarshal(p.Arguments, &a); err != nil {
			return argError(req, "slow", "ms (number)"), false
		}
		ms := time.Duration(a.MS) * time.Millisecond
		if ms > 120*time.Second {
			ms = 120 * time.Second
		}
		if ms > 0 {
			time.Sleep(ms)
		}
		res = callResult{Content: []contentBlock{{Type: "text", Text: fmt.Sprintf("slept %.0fms", a.MS)}}}

	case "fail":
		var a struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(p.Arguments, &a); err != nil || a.Message == "" {
			return argError(req, "fail", "message (non-empty string)"), false
		}
		res = callResult{
			Content: []contentBlock{{Type: "text", Text: a.Message}},
			IsError: true,
		}

	case "env":
		var a struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(p.Arguments, &a); err != nil || a.Name == "" {
			return argError(req, "env", "name (non-empty string)"), false
		}
		v := os.Getenv(a.Name)
		if v == "" {
			v = "(unset)"
		}
		res = callResult{Content: []contentBlock{{Type: "text", Text: a.Name + "=" + v}}}

	default:
		return &jsonrpcResponse{
			JSONRPC: "2.0", ID: req.ID,
			Error: &jsonrpcError{Code: -32602, Message: fmt.Sprintf("mock: unknown tool %q", p.Name)},
		}, false
	}
	out, _ := json.Marshal(res)
	return &jsonrpcResponse{JSONRPC: "2.0", ID: req.ID, Result: out}, false
}

// argError builds the -32602 for a bad tool argument.
func argError(req *jsonrpcRequest, tool, want string) *jsonrpcResponse {
	return &jsonrpcResponse{
		JSONRPC: "2.0", ID: req.ID,
		Error: &jsonrpcError{Code: -32602, Message: fmt.Sprintf("mock: tool %q requires %s", tool, want)},
	}
}
