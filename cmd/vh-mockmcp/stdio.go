// stdio.go — stdio framing: newline-delimited JSON-RPC on stdin/stdout.
// Each request line is parsed and dispatched; each response (garbage
// marker included) is one line on stdout. Diagnostics go to stderr.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// serveStdio runs the stdio loop until EOF on r. Responses are written
// one JSON object per line to w; the garbage marker writes the invalid
// JSON junk line instead (a host must fail THAT call closed, not hang).
func serveStdio(cfg mockConfig, r io.Reader, w io.Writer, stderrw io.Writer) error {
	br := bufio.NewReader(r)
	bw := bufio.NewWriter(w)
	for {
		line, err := br.ReadString('\n')
		if line == "" && err != nil {
			if err == io.EOF {
				return nil // clean shutdown on stdin close
			}
			return fmt.Errorf("stdin read: %w", err)
		}
		if ln := len(line); ln > 0 && line[ln-1] == '\n' {
			line = line[:ln-1]
		}
		if line == "" {
			if err == io.EOF {
				return nil
			}
			continue
		}
		var req jsonrpcRequest
		if jerr := json.Unmarshal([]byte(line), &req); jerr != nil || req.JSONRPC != "2.0" || req.Method == "" {
			// A malformed request line is a protocol error RESPONSE
			// (id null), never a crash.
			resp := jsonrpcResponse{
				JSONRPC: "2.0", ID: json.RawMessage("null"),
				Error: &jsonrpcError{Code: -32600, Message: "mock: invalid request line"},
			}
			out, _ := json.Marshal(resp)
			if _, werr := fmt.Fprintf(bw, "%s\n", out); werr != nil {
				return werr
			}
			_ = bw.Flush()
			if err == io.EOF {
				return nil
			}
			continue
		}
		resp, garbage := dispatch(cfg, &req)
		if garbage {
			if _, werr := fmt.Fprintf(bw, "%s\n", garbageLine); werr != nil {
				return werr
			}
		} else if resp != nil {
			// --notify-during-call: interleave the server-initiated
			// notification line BEFORE the tools/call response — the
			// mid-call window where the host necessarily has the call
			// pending (garbage modes stay pure: no notification).
			if cfg.notifyDuringCall && req.Method == "tools/call" {
				if _, werr := fmt.Fprintf(bw, "%s\n", notificationLine); werr != nil {
					return werr
				}
			}
			out, merr := json.Marshal(resp)
			if merr != nil {
				return merr
			}
			if _, werr := fmt.Fprintf(bw, "%s\n", out); werr != nil {
				return werr
			}
		}
		if ferr := bw.Flush(); ferr != nil {
			return ferr
		}
		if err == io.EOF {
			return nil
		}
	}
}
