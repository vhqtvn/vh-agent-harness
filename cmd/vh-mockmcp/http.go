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
			})
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
			writeHTTPResponse(cfg, w, resp)
		}
	})
	return mux
}

// writeHTTPResponse frames one response per cfg: plain JSON, or an SSE
// stream with a heartbeat comment + one data frame.
func writeHTTPResponse(cfg mockConfig, w http.ResponseWriter, resp *jsonrpcResponse) {
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
	// Heartbeat comment first (a host must ignore it), then the result
	// data frame, then stream end — the first result frame wins.
	_, _ = w.Write([]byte(": mock heartbeat\n\n"))
	_, _ = fmt.Fprintf(w, "data: %s\n\n", out)
	if ok {
		fl.Flush()
	}
}
