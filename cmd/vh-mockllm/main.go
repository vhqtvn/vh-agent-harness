// vh-mockllm is a standalone, scriptable mock LLM server for testing
// the native engine END-TO-END without a real provider: it serves both
// wire dialects the engine's adapters speak (OpenAI chat completions
// and Anthropic messages) from one deterministic, file-scripted
// scenario, with counters, a redacted request journal, and loud
// failures. It inherits the proven patterns of the JS mock at
// tests/integration/auto-gate-live-http/mock-llm-server.js (healthz
// readiness, per-path counters, reset) reimplemented engine-grade in
// Go.
//
// # Script format (--script, REQUIRED)
//
// The script is a JSON ARRAY of step objects consumed FIFO GLOBALLY
// across both dialect endpoints — deterministic ordering is the test
// author's contract (steps are handed out in request arrival order;
// the mutex serializes concurrent requests). Each step carries EXACTLY
// ONE response class:
//
//	{"text": "..."}
//	    200 success with assistant text.
//	{"tool_calls": [{"id":"call-1","name":"echo","args":{"text":"hi"}}]}
//	    200 with tool calls. Args is a JSON OBJECT in the script; the
//	    wire projection is per dialect (OpenAI: string-encoded
//	    "arguments"; Anthropic: tool_use blocks with an "input"
//	    object) — BOTH dialects are served correctly from one script.
//	{"fault": {"status": 500, "body": "...", "retry_after_ms": 2000}}
//	    Error status with the body written verbatim and, when
//	    retry_after_ms > 0, a Retry-After header in the seconds form
//	    (ceil). Status must be 400..599.
//	{"empty": true}
//	    200 with choices present but no content — the empty-response
//	    class (retryable in the engine's adapter classification).
//
// Anything else fails closed AT STARTUP (exit 2): a malformed script is
// a test bug and the mock refuses to start rather than guess.
//
// When the script is EXHAUSTED the next request gets a 500 whose body
// names the scenario file and its step count — fail loud, never
// silently loop or reuse steps.
//
// # Endpoints
//
//	POST /v1/chat/completions   OpenAI dialect (Authorization header
//	                            presence required; 401 without one —
//	                            before any script step is consumed)
//	POST /v1/messages           Anthropic dialect (x-api-key header
//	                            presence required)
//	GET  /healthz               {"ok":true}
//	GET  /count                 JSON map LLM-path → POST count
//	GET|POST /reset             clear counters — NOT the script cursor
//	                            and NOT the journal
//	GET  /journal?since=N       journal entries with seq > N (1-based):
//	                            method, path, auth-header PRESENCE ONLY,
//	                            and the full request body JSON
//
// REDACTION DISCIPLINE: the auth-header VALUE is never recorded
// anywhere — not in the in-memory journal, not in the --journal file.
// Only its presence is journaled.
//
// Response ids are deterministic and monotonic per dialect:
// mock-chatcmpl-1, mock-chatcmpl-2, ... and mock-msg-1, mock-msg-2, ...
//
// Usage:
//
//	vh-mockllm [--addr 127.0.0.1:8099] --script scenarios/x.json
//	           [--journal requests.jsonl]
//
// Exit codes: 0 clean shutdown · 1 listen failure · 2 usage (missing,
// unreadable, or malformed --script).
package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

const usageDoc = `vh-mockllm — scriptable mock LLM server (OpenAI + Anthropic dialects)

usage:
  vh-mockllm [--addr HOST:PORT] --script FILE.json [--journal FILE.jsonl]

flags:
  --addr HOST:PORT    listen address (default 127.0.0.1:8099)
  --script FILE       REQUIRED scenario script (JSON array of steps;
                      fail-closed exit 2 when missing, unreadable, or
                      malformed — see the package doc for the format)
  --journal FILE      optional append-only JSONL mirror of the request
                      journal (same redacted shape as GET /journal)
`

// run parses args, validates fail-closed, and serves. It returns the
// process exit code; listen errors map to 1, usage/validation to 2.
func run(args []string, stderrw io.Writer) int {
	var addr, scriptPath, journalPath string
	fs := flag.NewFlagSet("vh-mockllm", flag.ContinueOnError)
	fs.SetOutput(stderrw)
	fs.Usage = func() { fmt.Fprint(stderrw, usageDoc) }
	fs.StringVar(&addr, "addr", "127.0.0.1:8099", "listen address")
	fs.StringVar(&scriptPath, "script", "", "scenario script (REQUIRED, JSON array of steps)")
	fs.StringVar(&journalPath, "journal", "", "optional append-only JSONL request journal")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if scriptPath == "" {
		fmt.Fprintf(stderrw, "vh-mockllm: --script is REQUIRED (a mock without a scenario script has no behavior; pass a JSON array of steps — see the package doc)\n\n%s", usageDoc)
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderrw, "vh-mockllm: unexpected argument %q\n\n%s", fs.Arg(0), usageDoc)
		return 2
	}
	steps, err := LoadScript(scriptPath)
	if err != nil {
		fmt.Fprintf(stderrw, "vh-mockllm: %v\n\n%s", err, usageDoc)
		return 2
	}
	ms := newMockServer(scriptPath, steps)
	if journalPath != "" {
		f, jerr := os.OpenFile(journalPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if jerr != nil {
			fmt.Fprintf(stderrw, "vh-mockllm: --journal %s: %v\n", journalPath, jerr)
			return 2
		}
		defer f.Close() // best-effort; entries are flushed per line
		ms.setDiskJournal(f)
	}
	fmt.Fprintf(stderrw, "vh-mockllm: listening on %s (script %s, %d steps)\n", addr, scriptPath, len(steps))
	if err := http.ListenAndServe(addr, ms); err != nil {
		fmt.Fprintf(stderrw, "vh-mockllm: listen %s: %v\n", addr, err)
		return 1
	}
	return 0
}
