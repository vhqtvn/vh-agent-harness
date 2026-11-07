// http.go — Streamable-HTTP framing: every POST (any path except
// /healthz) carries one JSON-RPC request; the response is
// application/json with the single result, or — with --sse — a
// text/event-stream body whose first data frame carries the result
// behind a leading heartbeat comment (the shape a real Streamable
// server produces). The garbage marker becomes HTTP 500 with a junk
// body.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
)

// serveHTTP listens on addr until process death (test harnesses kill
// the process). With port 0 the bound address is printed to stderr so
// callers can take an ephemeral port.
func serveHTTP(cfg mockConfig, addr string, stderrw io.Writer) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}
	fmt.Fprintf(stderrw, "listening %s\n", ln.Addr().String())
	srv := &http.Server{Handler: newHTTPHandler(cfg)}
	return srv.Serve(ln)
}

// newHTTPHandler builds the mock's HTTP surface (test seam: httptest
// wraps this exact handler).
func newHTTPHandler(cfg mockConfig) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	// GET /calls — the EXECUTION counter (P8.2): {"total":N,"by_tool":
	// {name:count}} over every tools/call that dispatched a known tool.
	// The ask-by-default proofs read this to show a DENIED call never
	// reached the server (total stays 0) where a granted one did.
	mux.HandleFunc("/calls", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "mock: GET only", http.StatusMethodNotAllowed)
			return
		}
		total, by := callCountSnapshot()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"total": total, "by_tool": by})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "mock: POST only", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "mock: read body", http.StatusBadRequest)
			return
		}
		var req jsonrpcRequest
		if jerr := json.Unmarshal(body, &req); jerr != nil || req.JSONRPC != "2.0" || req.Method == "" {
			writeHTTPResponse(cfg, w, &jsonrpcResponse{
				JSONRPC: "2.0", ID: json.RawMessage("null"),
				Error: &jsonrpcError{Code: -32600, Message: "mock: invalid request body"},
			}, false)
			return
		}
		// The real-server session discipline (--require-session):
		// initialize ASSIGNS an Mcp-Session-Id; every later request
		// MUST carry it or the server 400s (the exact behavior a live
		// endpoint showed — this flag proves the host echoes it).
		if cfg.reqSess {
			sid := r.Header.Get("Mcp-Session-Id")
			if req.Method == "initialize" {
				w.Header().Set("Mcp-Session-Id", "mock-session-id-123456")
			} else if sid != "mock-session-id-123456" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"server-error","error":{"code":-32600,"message":"Bad Request: Missing session ID"}}`))
				return
			}
		}
		resp, garbage := dispatch(cfg, &req)
		switch {
		case garbage:
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(garbageLine))
		case resp == nil:
			// Notification: accepted, no body.
			w.WriteHeader(http.StatusAccepted)
		default:
			writeHTTPResponse(cfg, w, resp, cfg.notifyDuringCall && req.Method == "tools/call")
		}
	})
	return mux
}

// writeHTTPResponse frames one response per cfg: plain JSON, or an SSE
// stream with a heartbeat comment + one data frame. notifyFirst adds a
// server-initiated notification data frame BEFORE the result frame (the
// mid-call notification proof) — representable only in the SSE framing;
// a single application/json body cannot carry two messages, so the flag
// is a no-op there.
func writeHTTPResponse(cfg mockConfig, w http.ResponseWriter, resp *jsonrpcResponse, notifyFirst bool) {
	out, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, "mock: marshal", http.StatusInternalServerError)
		return
	}
	if !cfg.sse {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(out)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	fl, ok := w.(http.Flusher)
	// Heartbeat comment first (a host must ignore it), then — with
	// notifyFirst — the notification data frame (method set, no id — a
	// host must skip it mid-stream), then the result data frame, then
	// stream end — the first RESULT frame wins.
	_, _ = w.Write([]byte(": mock heartbeat\n\n"))
	if notifyFirst {
		_, _ = fmt.Fprintf(w, "data: %s\n\n", notificationLine)
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", out)
	if ok {
		fl.Flush()
	}
}
