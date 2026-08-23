// render.go — the client's two event renderers.
//
// Output discipline (mirrors the daemon's stdout-is-protocol purity):
// RENDERED OUTPUT AND PROMPTS GO TO STDERR; stdout is reserved for
// machine-readable final content (the one-shot final assistant text,
// or NDJSON events in --json mode). The renderers therefore write to
// whatever stream the wiring hands them — stderr for the human
// renderer, stdout for the JSON renderer.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// Renderer renders protocol notifications and tracks the last observed
// turn/end (the one-shot exit mapping keys on it).
type Renderer interface {
	// RenderEvent renders one session/event notification (params = the
	// full session Event record).
	RenderEvent(params json.RawMessage)
	// RenderApproval renders one approval/request notification (params =
	// {approvalId, call, reason}). The human renderer is a no-op here —
	// the interactive y/N prompt IS the notice (approval.go); the JSON
	// renderer writes the params verbatim so a machine consumer sees
	// the request before answering on stdin.
	RenderApproval(params json.RawMessage)
	// RenderProtocolError renders one protocol/error notification.
	RenderProtocolError(params json.RawMessage)
	// LastTurnEnd reports the kind of the most recent turn/end event
	// ("" and "ok" are clean; "error" maps to exit 1).
	LastTurnEnd() (kind string, seen bool)
	// ResetTurnEnd opens a new per-prompt epoch (hotfix c-F2/d-F2):
	// the driver resets BEFORE each session/prompt so the quiescence
	// drain actually waits for THIS turn's turn/end, and a prior
	// turn's kind=error cannot misclassify the next turn in the
	// response-beats-notification window.
	ResetTurnEnd()
}

// turnEndTracker is the shared last-turn/end state (both renderers).
type turnEndTracker struct {
	mu   sync.Mutex
	kind string
	seen bool
}

func (tt *turnEndTracker) observe(ev *session.Event) {
	if ev.Type != session.TypeTurnEnd {
		return
	}
	tt.mu.Lock()
	defer tt.mu.Unlock()
	var p session.TurnEndPayload
	if len(ev.Payload) > 0 {
		_ = json.Unmarshal(ev.Payload, &p) // payload shape is engine-owned; default kind ""
	}
	tt.kind, tt.seen = p.Kind, true
}

func (tt *turnEndTracker) last() (string, bool) {
	tt.mu.Lock()
	defer tt.mu.Unlock()
	return tt.kind, tt.seen
}

// reset opens a new per-prompt epoch: the previous turn/end is
// forgotten so a drain waits for the CURRENT turn's end and a stale
// kind=error cannot bleed into the next turn's classification.
func (tt *turnEndTracker) reset() {
	tt.mu.Lock()
	tt.kind, tt.seen = "", false
	tt.mu.Unlock()
}

// --- human renderer ----------------------------------------------------------

// humanRenderer draws compact lines on the injected writer (stderr in
// real wiring). Concurrency-safe: notifications arrive on the client's
// read loop while REPL/approval prompts may interleave — writes are
// whole-line and mutex-serialized.
type humanRenderer struct {
	w  io.Writer
	mu sync.Mutex
	turnEndTracker
}

func newHumanRenderer(w io.Writer) *humanRenderer { return &humanRenderer{w: w} }

func (h *humanRenderer) line(format string, args ...any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	fmt.Fprintf(h.w, format+"\n", args...)
}

// argHint extracts a compact `key=value` hint from tool args: the first
// recognized steering key, else a compact JSON snippet.
func argHint(args json.RawMessage) string {
	if len(args) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(args, &m); err != nil {
		return compactOneLine(string(args), 40)
	}
	for _, k := range []string{"command", "path", "file", "text", "pattern", "query"} {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok {
				return k + "=" + compactOneLine(s, 40)
			}
		}
	}
	return compactOneLine(string(args), 40)
}

// RenderEvent renders one session event as a compact line.
func (h *humanRenderer) RenderEvent(params json.RawMessage) {
	var ev session.Event
	if err := json.Unmarshal(params, &ev); err != nil {
		h.line("· <undecodable event> %s", compactOneLine(string(params), 60))
		return
	}
	h.observe(&ev)
	switch ev.Type {
	case session.TypeSessionPrompt:
		var p session.PromptPayload
		_ = json.Unmarshal(ev.Payload, &p)
		h.line("→ prompt %s", compactOneLine(p.Text, 60))
	case session.TypeLLMRequest:
		var p session.LLMRequestPayload
		_ = json.Unmarshal(ev.Payload, &p)
		h.line("↗ llm %s", p.Model)
	case session.TypeLLMResponse:
		var p session.LLMResponsePayload
		_ = json.Unmarshal(ev.Payload, &p)
		if p.Content != "" {
			h.renderAssistant(p.Content)
		}
	case session.TypeToolCall:
		var p session.ToolCallPayload
		_ = json.Unmarshal(ev.Payload, &p)
		if hint := argHint(p.Args); hint != "" {
			h.line("⚙ tool %s %s", p.Name, hint)
		} else {
			h.line("⚙ tool %s", p.Name)
		}
	case session.TypeToolResult:
		var p session.ToolResultPayload
		_ = json.Unmarshal(ev.Payload, &p)
		switch {
		case p.Denied:
			h.line("⊘ tool denied %s (%s)", p.Name, p.DenyReason)
		case p.IsError:
			h.line("✘ tool result %s: %s", p.Name, compactOneLine(p.Content, 60))
		default:
			h.line("✔ tool result (%d bytes)", len(p.Content))
		}
	case session.TypeJobEnqueued:
		var p session.JobPayload
		_ = json.Unmarshal(ev.Payload, &p)
		h.line("… job %s enqueued", p.JobID)
	case session.TypeJobStarted:
		var p session.JobPayload
		_ = json.Unmarshal(ev.Payload, &p)
		h.line("… job %s started", p.JobID)
	case session.TypeJobSettled:
		var p session.JobPayload
		_ = json.Unmarshal(ev.Payload, &p)
		if p.Reason != "" {
			h.line("■ job %s settled %s — %s", p.JobID, p.Result, compactOneLine(p.Reason, 60))
		} else {
			h.line("■ job %s settled %s", p.JobID, p.Result)
		}
	case session.TypeTurnEnd:
		var p session.TurnEndPayload
		_ = json.Unmarshal(ev.Payload, &p)
		if p.Kind == "error" {
			h.line("✗ turn error: %s", p.Reason)
		}
		// clean turn/end renders nothing (the final text / next prompt
		// is the signal); tracking happened in observe.
	default:
		h.line("· %s", ev.Type)
	}
}

// renderAssistant prints the assistant block: header + 2-space
// indented text lines.
func (h *humanRenderer) renderAssistant(content string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	fmt.Fprintln(h.w, "● assistant:")
	for _, ln := range bytes.Split([]byte(content), []byte("\n")) {
		fmt.Fprintf(h.w, "  %s\n", ln)
	}
}

// RenderApproval is a no-op for the human renderer: the interactive
// responder's y/N prompt is the approval notice.
func (h *humanRenderer) RenderApproval(params json.RawMessage) {}

// RenderProtocolError renders a protocol/error notification.
func (h *humanRenderer) RenderProtocolError(params json.RawMessage) {
	var p struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(params, &p)
	h.line("⚠ protocol error %d: %s", p.Code, p.Message)
}

// LastTurnEnd reports the most recent turn/end kind.
func (h *humanRenderer) LastTurnEnd() (string, bool) { return h.last() }

// ResetTurnEnd opens a new per-prompt epoch.
func (h *humanRenderer) ResetTurnEnd() { h.reset() }

// --- JSON renderer -----------------------------------------------------------

// jsonRenderer writes every notification's params VERBATIM as NDJSON
// (machine mode: no rendering, no re-marshal — the bytes the daemon
// sent are the bytes the consumer gets).
type jsonRenderer struct {
	w  io.Writer
	mu sync.Mutex
	turnEndTracker
}

func newJSONRenderer(w io.Writer) *jsonRenderer { return &jsonRenderer{w: w} }

func (j *jsonRenderer) verbatim(params json.RawMessage) {
	j.mu.Lock()
	defer j.mu.Unlock()
	_, _ = j.w.Write(append(append([]byte(nil), params...), '\n'))
}

// RenderEvent writes the session/event params verbatim.
func (j *jsonRenderer) RenderEvent(params json.RawMessage) {
	var ev session.Event
	if err := json.Unmarshal(params, &ev); err == nil {
		j.observe(&ev)
	}
	j.verbatim(params)
}

// RenderApproval writes the approval/request params verbatim (the
// machine consumer answers with a JSON line on stdin; see jsonApprover).
func (j *jsonRenderer) RenderApproval(params json.RawMessage) { j.verbatim(params) }

// RenderProtocolError writes the protocol/error params verbatim.
func (j *jsonRenderer) RenderProtocolError(params json.RawMessage) { j.verbatim(params) }

// LastTurnEnd reports the most recent turn/end kind.
func (j *jsonRenderer) LastTurnEnd() (string, bool) { return j.last() }

// ResetTurnEnd opens a new per-prompt epoch.
func (j *jsonRenderer) ResetTurnEnd() { j.reset() }
