// http.go — the REMOTE (Streamable-HTTP) transport, hand-rolled on
// net/http (stdlib only, no SDK): POST each JSON-RPC request with
// `Accept: application/json, text/event-stream`; the response is EITHER
// a single `application/json` body OR a `text/event-stream` stream —
// parsed by hand (data: frames, heartbeat comments ignored, FIRST
// JSON-RPC result wins, stream end without a result is a typed error).
//
// Bounds: the shared http.Client carries dial/TLS-response-header
// timeouts; every call is additionally bounded by its context deadline
// (the registry arms the per-call deadline — NO unbounded waits, a hung
// remote fails closed within the configured --mcp-timeout-ms).
//
// Headers: the configured map rides every request; the transport's own
// Content-Type/Accept are protocol-mandated and always win over a
// configured key of the same name (a configured override would break
// the transport itself). Header VALUES are credentials — redacted out
// of every error surface via the server's redactor, and the URL never
// reaches a log line (http.Client errors embed it; every error string
// is scrubbed with the full URL).
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// HTTPClient is one remote MCP server endpoint.
type HTTPClient struct {
	url     string
	headers map[string]string
	red     redactor
	logf    func(string, ...any)
	hc      *http.Client
	ids     idSource

	sessionMu sync.Mutex
	sessionID string // the server-assigned Mcp-Session-Id, when it issues one
}

// NewHTTPClient builds the client for one remote server config. The
// shared http.Client carries connection-level timeouts; per-call
// bounds come from the caller's context (the registry arms the
// configured per-call deadline).
func NewHTTPClient(sc *ServerConfig, red redactor, logf func(string, ...any)) *HTTPClient {
	return &HTTPClient{
		url:     sc.URL,
		headers: sc.Headers,
		red:     red,
		logf:    logf,
		hc: &http.Client{
			Transport: &http.Transport{
				DialContext:         (&net.Dialer{Timeout: dialTimeout}).DialContext,
				TLSHandshakeTimeout: dialTimeout,
				MaxIdleConnsPerHost: 4,
			},
		},
	}
}

// dialTimeout bounds connection establishment (dial + TLS handshake).
const dialTimeout = 10 * time.Second

// maxResponseBytes bounds one response body read (a runaway remote
// cannot exhaust daemon memory; SSE streams stop at the first result
// frame anyway).
const maxResponseBytes = 16 << 20

// doRequest performs one request/response exchange bounded by ctx.
func (h *HTTPClient) doRequest(ctx context.Context, method string, params json.RawMessage, notification bool) (*rpcResponse, error) {
	var id *int64
	if !notification {
		n := h.ids.next()
		id = &n
	}
	req := rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("mcp: marshal %s: %w", method, err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, h.url, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New(h.red.Clean(fmt.Sprintf("mcp: build %s request: %v", method, err)))
	}
	for k, v := range h.headers {
		lk := strings.ToLower(k)
		if lk == "content-type" || lk == "accept" || lk == "mcp-session-id" {
			continue // protocol-mandated; a configured override would break the transport
		}
		httpReq.Header.Set(k, v)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	// Streamable-HTTP session discipline: a server that assigns an
	// Mcp-Session-Id on initialize REQUIRES it on every subsequent
	// request (real-world servers 400 without it — discovered against
	// a live endpoint). Echo it when present; never log it.
	h.sessionMu.Lock()
	if h.sessionID != "" {
		httpReq.Header.Set("Mcp-Session-Id", h.sessionID)
	}
	h.sessionMu.Unlock()

	resp, err := h.hc.Do(httpReq)
	if err != nil {
		return nil, errors.New(h.red.Clean(fmt.Sprintf("mcp: %s request failed: %v", method, err)))
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
	}()
	// Capture a session assignment from ANY 2xx response (the spec
	// allows the header on initialize; tolerate it later too).
	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" && resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		h.sessionMu.Lock()
		h.sessionID = sid
		h.sessionMu.Unlock()
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, errors.New(h.red.Clean(fmt.Sprintf("mcp: %s: HTTP %d: %s", method, resp.StatusCode, truncateForError(string(snippet)))))
	}
	if notification {
		return nil, nil // 2xx accepted; no body contract
	}

	ct := resp.Header.Get("Content-Type")
	switch {
	case strings.Contains(ct, "text/event-stream"):
		return readSSEResponse(resp.Body, id)
	default: // application/json (and anything else JSON-shaped)
		raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		if err != nil {
			return nil, errors.New(h.red.Clean(fmt.Sprintf("mcp: %s: read body: %v", method, err)))
		}
		var out rpcResponse
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, errors.New(h.red.Clean(fmt.Sprintf("mcp: %s: response is not JSON-RPC (content-type %s): %v", method, ct, truncateForError(string(raw)))))
		}
		if out.Error != nil {
			return nil, errors.New(h.red.Clean(out.Error.Error()))
		}
		return &out, nil
	}
}

// readSSEResponse parses a text/event-stream body by hand: comment
// lines (`:` heartbeats) ignored, `data:` frames accumulate until a
// blank line dispatches the event; the first frame carrying a JSON-RPC
// result/error with the awaited id wins; frames with other ids or
// notifications are skipped; stream end without the result is a typed
// error (fail-closed, never a hang).
func readSSEResponse(r io.Reader, want *int64) (*rpcResponse, error) {
	br := bufio.NewReaderSize(r, 64*1024)
	var data []string
	flush := func() (*rpcResponse, bool) {
		defer func() { data = nil }()
		if len(data) == 0 {
			return nil, false
		}
		payload := strings.Join(data, "\n")
		var resp rpcResponse
		if err := json.Unmarshal([]byte(payload), &resp); err != nil {
			return nil, false // a non-JSON data frame: heartbeat-class, ignore
		}
		if resp.ID == nil {
			return nil, false // notification frame: ignore
		}
		if want != nil && *resp.ID != *want {
			return nil, false // another call's frame: ignore
		}
		if resp.Error != nil {
			return &rpcResponse{ID: resp.ID}, true // caller renders via typed path
		}
		return &resp, true
	}
	for {
		line, err := br.ReadString('\n')
		trimmed := strings.TrimRight(line, "\r\n")
		switch {
		case trimmed == "":
			if resp, ok := flush(); ok {
				if resp.Error == nil {
					return resp, nil
				}
				return nil, errors.New(fmt.Sprintf("mcp: stream carried an error frame: %v", resp.Error))
			}
		case strings.HasPrefix(trimmed, ":"):
			// comment / heartbeat — ignore
		case strings.HasPrefix(trimmed, "data:"):
			d := strings.TrimPrefix(trimmed, "data:")
			d = strings.TrimPrefix(d, " ")
			data = append(data, d)
		default:
			// event:/id:/retry: fields carry no payload we consume
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				if resp, ok := flush(); ok && resp.Error == nil {
					return resp, nil
				}
				return nil, errors.New("mcp: event stream ended without a result frame (fail-closed)")
			}
			return nil, errors.New(fmt.Sprintf("mcp: event stream read error: %v", err))
		}
	}
}

// Initialize performs the handshake (initialize → notifications/
// initialized as a bodiless 2xx POST).
func (h *HTTPClient) Initialize(ctx context.Context) error {
	p := initializeParams{
		ProtocolVersion: ClientProtocolVersion,
		Capabilities:    map[string]any{},
	}
	p.ClientInfo.Name = "vh-agentd"
	p.ClientInfo.Version = "0.1.0"
	params, _ := json.Marshal(p)
	resp, err := h.doRequest(ctx, "initialize", params, false)
	if err != nil {
		return err
	}
	var initRes initializeResult
	if err := json.Unmarshal(resp.Result, &initRes); err != nil {
		return fmt.Errorf("mcp: initialize result unparseable: %v", err)
	}
	if initRes.ProtocolVersion != ClientProtocolVersion && h.logf != nil {
		h.logf("mcp: server negotiated protocolVersion %s (host speaks %s) — proceeding (tools surface is stable)", initRes.ProtocolVersion, ClientProtocolVersion)
	}
	_, err = h.doRequest(ctx, "notifications/initialized", nil, true)
	return err
}

// ListTools fetches the advertisement.
func (h *HTTPClient) ListTools(ctx context.Context) ([]Tool, error) {
	resp, err := h.doRequest(ctx, "tools/list", json.RawMessage(`{}`), false)
	if err != nil {
		return nil, err
	}
	var res toolsListResult
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("mcp: tools/list result unparseable: %v", err)
	}
	return res.Tools, nil
}

// CallTool invokes one tool (tool-level isError is data, not error).
func (h *HTTPClient) CallTool(ctx context.Context, name string, arguments json.RawMessage) (*CallResult, error) {
	if len(arguments) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	params, _ := json.Marshal(toolsCallParams{Name: name, Arguments: arguments})
	resp, err := h.doRequest(ctx, "tools/call", params, false)
	if err != nil {
		return nil, err
	}
	var res CallResult
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("mcp: tools/call(%s) result unparseable: %v", name, err)
	}
	return &res, nil
}

// Close is a no-op for HTTP (no persistent resource to release).
func (h *HTTPClient) Close() error { return nil }
