// vh-mockmcp is a standalone, scriptable mock MCP server for testing
// the native engine's MCP HOST implementation without any real server:
// it speaks JSON-RPC 2.0 over BOTH transport shapes the host supports —
// stdio (newline-delimited JSON on stdin/stdout, the default) and
// Streamable-HTTP (POST; application/json or text/event-stream
// responses) — with a canned tool set and fault-injection flags for
// fail-closed proofs. It inherits the vh-mockllm patterns: fail-loud
// startup (exit 2 on usage bugs), healthz readiness, deterministic
// responses, and stderr-only diagnostics.
//
// # Tool set (canned, deterministic)
//
//	echo {text: string}  → text content "echo: <text>"
//	slow {ms: number}    → sleeps ms (capped at 120000), then text
//	                       "slept <ms>ms" — the timeout-proof tool
//	fail {message}       → result with isError:true and the message —
//	                       the typed tool-error proof
//	env  {name: string}  → (--env-tool) the subprocess value of the env
//	                       var, or "(unset)" — the env-scrub proof
//	weird {value: any}   → (--bad-schema-tool) inputSchema
//	                       {"type":"string"} — the UNMAPPABLE schema a
//	                       host must skip per-tool with a warning
//
// # Fault injection (flags)
//
//	--garbage       every response is garbage: stdio mode writes an
//	                invalid JSON line; http mode returns 500 with a junk
//	                body. A host pointed here DEGRADES at initialize —
//	                the server-level fail-closed proof.
//	--call-garbage  initialize/tools/list are healthy; every tools/call
//	                response is garbage (stdio: invalid JSON; http: 500)
//	                — the call-level fail-closed proof (error, no hang,
//	                no crash).
//	--sse           (http mode) frame every response as
//	                text/event-stream with a leading heartbeat comment
//	                line — exercises the host's SSE parsing against a
//	                real server (not just a hand-written fixture).
//
// # Protocol surface (both modes, one dispatch function)
//
//	requests   initialize → {protocolVersion:"2025-06-18",
//	          │             capabilities:{}, serverInfo}
//	          ├ notifications/initialized → accepted, no response
//	          ├ tools/list  → {tools:[...canned set...]}
//	          └ tools/call  → {content:[{type:"text",...}], isError?}
//	errors     unknown method → -32601; unknown tool name → -32602;
//	          bad params → -32700/-32600 class errors
//
// http mode also serves GET /healthz → {"ok":true}.
//
// Usage:
//
//	vh-mockmcp [--stdio]
//	vh-mockmcp --http HOST:PORT [--sse] [fault flags]
//
// Exit codes: 0 clean shutdown · 1 runtime failure · 2 usage error.
// With --http and port 0, the bound address is printed to stderr as
// "listening <addr>" so tests can take an ephemeral port.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

func main() { os.Exit(run(os.Args[1:], os.Stderr)) }

const usageDoc = `vh-mockmcp — scriptable mock MCP server (stdio + Streamable-HTTP)

usage:
  vh-mockmcp [--stdio]
  vh-mockmcp --http HOST:PORT [--sse] [--garbage] [--call-garbage]
             [--bad-schema-tool] [--env-tool]

modes:
  --stdio            newline-delimited JSON-RPC 2.0 on stdin/stdout (default)
  --http HOST:PORT   Streamable-HTTP: POST requests to any path except
                     /healthz; responses are application/json, or
                     text/event-stream with --sse. Port 0 prints the
                     bound address to stderr ("listening <addr>").

fault injection:
  --garbage          every response is garbage (invalid JSON line / HTTP 500)
  --call-garbage     tools/call responses only are garbage
  --bad-schema-tool  advertise a tool with an unmappable inputSchema
  --env-tool         advertise the env tool (subprocess env readback)
  --require-session  (http) assign + require Mcp-Session-Id — the
                     real-server session discipline a host must honor

exit codes: 0 clean · 1 runtime failure · 2 usage
`

// run parses flags and dispatches to the selected transport loop. Both
// loops share dispatchRequest — one protocol brain, two framings.
func run(args []string, stderrw io.Writer) int {
	fs := flag.NewFlagSet("vh-mockmcp", flag.ContinueOnError)
	fs.SetOutput(stderrw)
	fs.Usage = func() { fmt.Fprint(stderrw, usageDoc) }
	var (
		stdio     = fs.Bool("stdio", false, "stdio mode (default)")
		httpAddr  = fs.String("http", "", "http mode: listen address HOST:PORT")
		sse       = fs.Bool("sse", false, "http mode: frame responses as text/event-stream")
		garbage   = fs.Bool("garbage", false, "every response is garbage")
		callGarb  = fs.Bool("call-garbage", false, "tools/call responses are garbage")
		badSchema = fs.Bool("bad-schema-tool", false, "advertise the unmappable-schema tool")
		envTool   = fs.Bool("env-tool", false, "advertise the env readback tool")
		reqSess   = fs.Bool("require-session", false, "http mode: assign + require Mcp-Session-Id (the real-server session discipline)")
	)
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2 // flag package already printed the specific error
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderrw, "vh-mockmcp: unexpected positional arguments: %v\n\n%s", fs.Args(), usageDoc)
		return 2
	}
	if *stdio && *httpAddr != "" {
		fmt.Fprintf(stderrw, "vh-mockmcp: --stdio and --http are mutually exclusive\n\n%s", usageDoc)
		return 2
	}

	cfg := mockConfig{
		sse:       *sse,
		garbage:   *garbage,
		callGarb:  *callGarb,
		badSchema: *badSchema,
		envTool:   *envTool,
		reqSess:   *reqSess,
	}
	if *httpAddr != "" {
		if err := serveHTTP(cfg, *httpAddr, stderrw); err != nil {
			fmt.Fprintf(stderrw, "vh-mockmcp: %v\n", err)
			return 1
		}
		return 0
	}
	if err := serveStdio(cfg, os.Stdin, os.Stdout, stderrw); err != nil {
		fmt.Fprintf(stderrw, "vh-mockmcp: %v\n", err)
		return 1
	}
	return 0
}
