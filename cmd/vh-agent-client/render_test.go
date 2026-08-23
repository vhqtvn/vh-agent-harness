// render_test.go — slice P2 step 2 (red): the two renderers. The human
// renderer draws compact lines on the injected writer (stderr in real
// wiring); the JSON renderer writes every notification's params
// VERBATIM as NDJSON (no re-marshal). Both track the last turn/end kind
// (the one-shot exit mapping depends on it).
package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

func eventParams(t *testing.T, typ string, payload any) json.RawMessage {
	t.Helper()
	ev := session.Event{Seq: 1, Type: typ}
	if payload != nil {
		switch p := payload.(type) {
		case string:
			ev.Payload = json.RawMessage(p)
		default:
			b, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("marshal payload: %v", err)
			}
			ev.Payload = b
		}
	}
	b, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	return json.RawMessage(b)
}

func renderAll(t *testing.T, r Renderer, items ...json.RawMessage) {
	t.Helper()
	for _, p := range items {
		r.RenderEvent(p)
	}
}

func TestHumanRendererPromptLine(t *testing.T) {
	var buf bytes.Buffer
	r := newHumanRenderer(&buf)
	r.RenderEvent(eventParams(t, session.TypeSessionPrompt, `{"text":"hello world"}`))
	if got := buf.String(); got != "→ prompt hello world\n" {
		t.Fatalf("prompt line = %q", got)
	}
}

func TestHumanRendererToolCallWithHint(t *testing.T) {
	var buf bytes.Buffer
	r := newHumanRenderer(&buf)
	r.RenderEvent(eventParams(t, session.TypeToolCall, `{"id":"c1","name":"echo","args":{"text":"hi there"}}`))
	r.RenderEvent(eventParams(t, session.TypeToolCall, `{"id":"c2","name":"run_shell","args":{"command":"echo child-shell-ok"}}`))
	r.RenderEvent(eventParams(t, session.TypeToolCall, `{"id":"c3","name":"read","args":{"path":"a/b.txt"}}`))
	out := buf.String()
	for _, want := range []string{
		"⚙ tool echo text=hi there\n",
		"⚙ tool run_shell command=echo child-shell-ok\n",
		"⚙ tool read path=a/b.txt\n",
	} {
		if !bytes.Contains([]byte(out), []byte(want)) {
			t.Fatalf("tool call lines missing %q in:\n%s", want, out)
		}
	}
}

func TestHumanRendererToolResults(t *testing.T) {
	var buf bytes.Buffer
	r := newHumanRenderer(&buf)
	r.RenderEvent(eventParams(t, session.TypeToolResult, `{"callId":"c1","name":"echo","content":"hello world","isError":false}`))
	r.RenderEvent(eventParams(t, session.TypeToolResult, `{"callId":"c2","name":"run_shell","content":"boom","isError":true}`))
	r.RenderEvent(eventParams(t, session.TypeToolResult, `{"callId":"c3","name":"write","content":"","isError":true,"denied":true,"deniedBy":"approval","denyReason":"operator said no"}`))
	out := buf.String()
	for _, want := range []string{
		"✔ tool result (11 bytes)",
		"✘ tool result run_shell: boom",
		"⊘ tool denied write (operator said no)",
	} {
		if !bytes.Contains([]byte(out), []byte(want)) {
			t.Fatalf("result lines missing %q in:\n%s", want, out)
		}
	}
}

func TestHumanRendererAssistantIndented(t *testing.T) {
	var buf bytes.Buffer
	r := newHumanRenderer(&buf)
	r.RenderEvent(eventParams(t, session.TypeLLMResponse, `{"model":"m","content":"line one\nline two","usage":{}}`))
	out := buf.String()
	if !bytes.Contains([]byte(out), []byte("● assistant:\n")) {
		t.Fatalf("missing assistant header:\n%s", out)
	}
	if !bytes.Contains([]byte(out), []byte("  line one\n  line two\n")) {
		t.Fatalf("assistant text must be indented 2 spaces:\n%s", out)
	}
}

func TestHumanRendererJobSettlement(t *testing.T) {
	var buf bytes.Buffer
	r := newHumanRenderer(&buf)
	r.RenderEvent(eventParams(t, session.TypeJobEnqueued, `{"jobId":"echo-1","kind":"echo"}`))
	r.RenderEvent(eventParams(t, session.TypeJobSettled, `{"jobId":"echo-1","kind":"echo","result":"completed"}`))
	r.RenderEvent(eventParams(t, session.TypeJobSettled, `{"jobId":"fail-1","kind":"fail","result":"failed","reason":"requested failure"}`))
	out := buf.String()
	for _, want := range []string{
		"… job echo-1 enqueued",
		"■ job echo-1 settled completed",
		"■ job fail-1 settled failed — requested failure",
	} {
		if !bytes.Contains([]byte(out), []byte(want)) {
			t.Fatalf("job lines missing %q in:\n%s", want, out)
		}
	}
}

func TestHumanRendererTurnError(t *testing.T) {
	var buf bytes.Buffer
	r := newHumanRenderer(&buf)
	r.RenderEvent(eventParams(t, session.TypeTurnEnd, `{"kind":"error","reason":"adapter boom"}`))
	if got := buf.String(); got != "✗ turn error: adapter boom\n" {
		t.Fatalf("turn error line = %q", got)
	}
	if kind, seen := r.LastTurnEnd(); !seen || kind != "error" {
		t.Fatalf("LastTurnEnd = %q,%v want error,true", kind, seen)
	}
}

func TestHumanRendererCleanTurnEndIsSilentButTracked(t *testing.T) {
	var buf bytes.Buffer
	r := newHumanRenderer(&buf)
	r.RenderEvent(eventParams(t, session.TypeTurnEnd, `{"kind":""}`))
	if buf.Len() != 0 {
		t.Fatalf("clean turn/end must render nothing, got %q", buf.String())
	}
	if kind, seen := r.LastTurnEnd(); !seen || kind != "" {
		t.Fatalf("LastTurnEnd = %q,%v want \"\",true", kind, seen)
	}
}

func TestHumanRendererUnknownTypeFallback(t *testing.T) {
	var buf bytes.Buffer
	r := newHumanRenderer(&buf)
	r.RenderEvent(eventParams(t, "future/thing", `{"x":1}`))
	if !bytes.Contains([]byte(buf.String()), []byte("· future/thing")) {
		t.Fatalf("fallback line missing:\n%s", buf.String())
	}
}

func TestHumanRendererProtocolError(t *testing.T) {
	var buf bytes.Buffer
	r := newHumanRenderer(&buf)
	r.RenderProtocolError(json.RawMessage(`{"code":-32700,"message":"bad line"}`))
	if !bytes.Contains([]byte(buf.String()), []byte("⚠ protocol error -32700: bad line")) {
		t.Fatalf("protocol error line missing:\n%s", buf.String())
	}
}

func TestJSONRendererEventsVerbatim(t *testing.T) {
	var buf bytes.Buffer
	r := newJSONRenderer(&buf)
	p := eventParams(t, session.TypeToolCall, `{"id":"c1","name":"echo","args":{"text":"hi"}}`)
	r.RenderEvent(p)
	if buf.String() != string(p)+"\n" {
		t.Fatalf("json renderer must write params verbatim + newline, got %q", buf.String())
	}
	// turn/end kind tracking still works in json mode.
	p2 := eventParams(t, session.TypeTurnEnd, `{"kind":"error","reason":"x"}`)
	r.RenderEvent(p2)
	if kind, seen := r.LastTurnEnd(); !seen || kind != "error" {
		t.Fatalf("LastTurnEnd = %q,%v want error,true", kind, seen)
	}
}

func TestJSONRendererApprovalAndProtocolErrorVerbatim(t *testing.T) {
	var buf bytes.Buffer
	r := newJSONRenderer(&buf)
	r.RenderApproval(json.RawMessage(`{"approvalId":"approval-1","call":{"id":"c","name":"write"},"reason":"r"}`))
	r.RenderProtocolError(json.RawMessage(`{"code":-32700,"message":"m"}`))
	out := buf.String()
	if out != "{\"approvalId\":\"approval-1\",\"call\":{\"id\":\"c\",\"name\":\"write\"},\"reason\":\"r\"}\n{\"code\":-32700,\"message\":\"m\"}\n" {
		t.Fatalf("json approval/protocol-error output = %q", out)
	}
}
