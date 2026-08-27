// client.go — the transport-independent MCP host vocabulary: the
// Client interface both transports implement, the JSON-RPC 2.0 wire
// structs, the protocol handshake constants, and the credential
// redactor every diagnostic surface scrubs through.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
)

// ClientProtocolVersion is the protocol version this host sends in
// initialize. Negotiation is honest-tolerant: the server's version is
// accepted as-is; a mismatch is LOGGED and the session proceeds
// (tools/list and tools/call are stable across versions).
const ClientProtocolVersion = "2025-06-18"

// Tool is one MCP tool as returned by tools/list.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// CallResult is the tools/call result envelope.
type CallResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

// ContentBlock is one content block; the text class carries Text,
// every other class (image, audio, resource...) is mapped to an honest
// placeholder by JoinContent.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// JoinContent renders a call result for the engine's canonical string
// tool-result content: text blocks concatenate (newline-joined);
// non-text blocks become an honest placeholder naming their type; an
// empty content list is the empty string (never fabricated).
func JoinContent(res *CallResult) string {
	if res == nil || len(res.Content) == 0 {
		return ""
	}
	parts := make([]string, 0, len(res.Content))
	for _, b := range res.Content {
		if b.Type == "text" || b.Type == "" {
			parts = append(parts, b.Text)
			continue
		}
		parts = append(parts, fmt.Sprintf("[non-text content block (type %q) omitted — v1 surfaces text blocks only]", b.Type))
	}
	return strings.Join(parts, "\n")
}

// Client is one connected MCP server. Implementations are safe for
// concurrent use (the engine may interleave calls across a session;
// every method is bounded by its context deadline — NO unbounded
// waits).
type Client interface {
	// Initialize performs the initialize handshake and sends
	// notifications/initialized. A non-nil error DEGRADES the server
	// at the registry layer.
	Initialize(ctx context.Context) error
	// ListTools fetches the tool advertisement.
	ListTools(ctx context.Context) ([]Tool, error)
	// CallTool invokes one tool. A protocol-level failure returns an
	// error; a tool-level failure (isError) returns (result, nil) —
	// the registry maps it to the typed tool error.
	CallTool(ctx context.Context, name string, arguments json.RawMessage) (*CallResult, error)
	// Close releases the transport (subprocess teardown / no-op for
	// HTTP) and is idempotent.
	Close() error
}

// --- JSON-RPC 2.0 wire structs (hand-rolled; no SDK) ---

// rpcRequest is one outgoing call or notification (ID nil ⇒
// notification).
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// rpcResponse is one incoming result or error.
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *rpcError) Error() string {
	if len(e.Data) > 0 {
		return fmt.Sprintf("mcp: json-rpc error %d: %s (%s)", e.Code, e.Message, string(e.Data))
	}
	return fmt.Sprintf("mcp: json-rpc error %d: %s", e.Code, e.Message)
}

// idSource hands out monotonic request ids (per client; starts at 1 so
// the zero id is never ambiguously omitted by omitempty encoders).
type idSource struct{ n atomic.Int64 }

func (s *idSource) next() int64 { return s.n.Add(1) }

// initializeParams/initializeResult are the handshake envelope.
type initializeParams struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ClientInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"clientInfo"`
}

type initializeResult struct {
	ProtocolVersion string `json:"protocolVersion"`
	ServerInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"serverInfo"`
}

// toolsListResult is the tools/list envelope.
type toolsListResult struct {
	Tools []Tool `json:"tools"`
}

// toolsCallParams is the tools/call request envelope.
type toolsCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// redactor scrubs this server's CREDENTIALS (url, header values, env
// values) out of any text that may reach a log or error surface — the
// providers-key discipline (adapters.RedactSecret) applied to MCP
// server config. Values under the documented minimum secret length
// pass through untouched (RedactSecret's contract).
type redactor []string

// newRedactor collects the credential strings from a server config.
func newRedactor(sc *ServerConfig) redactor {
	var creds []string
	if sc.URL != "" {
		creds = append(creds, sc.URL)
	}
	for _, v := range sc.Headers {
		if v != "" {
			creds = append(creds, v)
		}
	}
	for _, v := range sc.Env {
		if v != "" {
			creds = append(creds, v)
		}
	}
	return redactor(creds)
}

// Clean redacts every credential occurrence from s.
func (r redactor) Clean(s string) string {
	for _, c := range r {
		s = adapters.RedactSecret(s, c)
	}
	return s
}
