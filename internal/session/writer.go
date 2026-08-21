package session

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Log is an append-only session event-log writer.
//
// Slice 4 lifts the slice-1 single-threaded restriction: a Log is safe
// for concurrent use. Background job events append from executor
// goroutines while the turn engine drives the same log (enqueue from a
// tool body mid-turn, job/started and job/settled racing turn/end), so
// every append takes the internal lock and each Write is one complete
// JSONL record — concurrent appends interleave only at record boundaries
// and the seq stays contiguous.
type Log struct {
	mu     sync.RWMutex
	w      io.Writer
	closer io.Closer
	seq    int64
	events []Event
	// spill is the optional armed oversize tool-result policy (engine
	// wiring; nil ⇒ inline behavior identical to pre-spill logs).
	spill *SpillPolicy
}

// NewLog starts a new session log on w, writing the session/header record
// (seq 1) carrying sessionID, the format version, and the creation time.
func NewLog(w io.Writer, sessionID string, now time.Time) (*Log, error) {
	return NewChildLog(w, sessionID, ChildHeader{}, now)
}

// ChildHeader carries the parent-child topology fields of a subagent
// CHILD session header (dsh SessionHeader.parentSessionId/delegationDepth,
// plus the optional role/persona hint). Persisting them in the child's own
// header is what makes the delegation-depth recursion budget survive
// resume: the child header is the AUTHORITATIVE depth record.
type ChildHeader struct {
	ParentSessionID string
	DelegationDepth int
	Role            string
}

// NewChildLog starts a new subagent child session log on w: the
// session/header record additionally carries the parent session id, the
// child's delegation depth, and the optional role hint. A zero ChildHeader
// writes byte-identical header bytes to NewLog.
func NewChildLog(w io.Writer, sessionID string, ch ChildHeader, now time.Time) (*Log, error) {
	l := &Log{w: w}
	payload, err := json.Marshal(HeaderPayload{
		SessionID:       sessionID,
		FormatVersion:   SESSION_FORMAT_VERSION,
		CreatedAt:       now,
		ParentSessionID: ch.ParentSessionID,
		DelegationDepth: ch.DelegationDepth,
		Role:            ch.Role,
	})
	if err != nil {
		return nil, fmt.Errorf("session: marshal header: %w", err)
	}
	ev := Event{Type: TypeSessionHeader, Payload: payload}
	if _, err := l.writeEvent(ev); err != nil {
		return nil, err
	}
	return l, nil
}

// OpenFile creates (truncating) a JSONL session log at path.
func OpenFile(path string, sessionID string) (*Log, error) {
	return OpenChildFile(path, sessionID, ChildHeader{})
}

// OpenChildFile creates (truncating) a JSONL subagent child session log
// at path with the given parent-child header fields.
func OpenChildFile(path string, sessionID string, ch ChildHeader) (*Log, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("session: create %s: %w", path, err)
	}
	lg, err := NewChildLog(f, sessionID, ch, time.Now().UTC())
	if err != nil {
		_ = f.Close()
		// A failed header write produced no durable log: remove the
		// orphaned 0-byte file instead of leaving it on disk (it would
		// never be referenced by a parent; reopening it fails
		// fail-closed — this is hygiene, not a correctness change).
		// Best-effort: if the removal itself fails, the ORIGINAL error
		// is returned with a note — never masked.
		if rmErr := os.Remove(path); rmErr != nil {
			return nil, fmt.Errorf("%w (session: best-effort removal of orphaned %s after failed header write: %v)", err, path, rmErr)
		}
		return nil, err
	}
	lg.closer = f
	return lg, nil
}

// ResumeLog continues an existing log on w from an already-replayed
// event list: no header is written and seq continues from the last
// replayed event. The events must satisfy the structural invariants
// (header first, contiguous 1-based seq) — Replay/RecoverTail enforce
// them upstream. This is the crash-recovery append seam: the recovered
// process keeps writing the SAME durable stream, never a second storage.
func ResumeLog(w io.Writer, events []Event) (*Log, error) {
	if len(events) == 0 {
		return nil, fmt.Errorf("session: cannot resume an empty event list")
	}
	if err := validateStructure(events); err != nil {
		return nil, err
	}
	l := &Log{w: w}
	l.events = make([]Event, len(events))
	copy(l.events, events)
	l.seq = events[len(events)-1].Seq
	return l, nil
}

// ResumeFile recovers a log FILE for continued append: it replays with
// torn-tail tolerance (RecoverTail), truncates any uncommitted torn
// fragment, reopens the file in append mode, and returns the resumed
// log. The recovered process continues the same session stream.
func ResumeFile(path string) (*Log, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("session: open %s: %w", path, err)
	}
	events, validBytes, torn, err := RecoverTail(f)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, err
	}
	if torn {
		if err := os.Truncate(path, validBytes); err != nil {
			return nil, fmt.Errorf("session: truncate torn tail of %s: %w", path, err)
		}
	}
	af, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("session: reopen %s for append: %w", path, err)
	}
	lg, err := ResumeLog(af, events)
	if err != nil {
		_ = af.Close()
		return nil, err
	}
	lg.closer = af
	return lg, nil
}

// Append appends one event, assigning the next monotonic seq. It validates
// eventType against the closed slice-1 set (session/header is reserved for
// NewLog) and requires message-bearing types to carry a well-formed
// surfaceOp.
func (l *Log) Append(eventType string, surface *SurfaceOp, payload any) (Event, error) {
	ev, err := l.buildEvent(eventType, surface, payload)
	if err != nil {
		return Event{}, err
	}
	return l.writeEvent(ev)
}

// appendLocked is Append for a caller that ALREADY holds l.mu (write):
// Compact's atomic fold+append sequence. Everything Append does happens
// here minus the lock acquisition — taking l.mu again would deadlock
// (sync.RWMutex is not reentrant).
func (l *Log) appendLocked(eventType string, surface *SurfaceOp, payload any) (Event, error) {
	ev, err := l.buildEvent(eventType, surface, payload)
	if err != nil {
		return Event{}, err
	}
	return l.writeEventLocked(ev)
}

// buildEvent validates and marshals one event WITHOUT touching the lock
// (validation and payload marshaling are pure; only the seq assignment
// and the write are stateful).
func (l *Log) buildEvent(eventType string, surface *SurfaceOp, payload any) (Event, error) {
	if eventType == TypeSessionHeader {
		return Event{}, fmt.Errorf("session: event type %q is reserved for the session header", TypeSessionHeader)
	}
	if !knownTypes[eventType] {
		return Event{}, fmt.Errorf("session: unknown event type %q", eventType)
	}
	if err := validateSurfaceOp(eventType, surface); err != nil {
		return Event{}, err
	}
	var raw json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return Event{}, fmt.Errorf("session: marshal %s payload: %w", eventType, err)
		}
		raw = b
	}
	return Event{Type: eventType, SurfaceOp: surface, Payload: raw}, nil
}

// AppendPrompt records user input ingress (surface append, role user).
func (l *Log) AppendPrompt(text string) (Event, error) {
	return l.Append(TypeSessionPrompt, &SurfaceOp{Op: SurfaceOpAppend}, PromptPayload{Text: text})
}

// AppendLLMRequest records the audit shape of an outgoing LLM request.
func (l *Log) AppendLLMRequest(model string, toolNames []string, temperature *float64, maxTokens int) (Event, error) {
	return l.Append(TypeLLMRequest, nil, LLMRequestPayload{Model: model, Tools: toolNames, Temperature: temperature, MaxTokens: maxTokens})
}

// AppendLLMResponse records an assistant message (content + tool calls)
// and its usage envelope (surface append, role assistant).
func (l *Log) AppendLLMResponse(model, content string, toolCalls []ToolCall, usage Usage) (Event, error) {
	return l.Append(TypeLLMResponse, &SurfaceOp{Op: SurfaceOpAppend}, LLMResponsePayload{Model: model, Content: content, ToolCalls: toolCalls, Usage: usage})
}

// AppendToolCall records the pre-execution intent of one tool invocation.
func (l *Log) AppendToolCall(id, name string, args json.RawMessage) (Event, error) {
	return l.Append(TypeToolCall, nil, ToolCallPayload{ID: id, Name: name, Args: args})
}

// AppendToolResult records the frozen canonical result of one tool
// execution (surface append, role tool; isError on failure).
func (l *Log) AppendToolResult(callID, name, content string, isError bool) (Event, error) {
	return l.Append(TypeToolResult, &SurfaceOp{Op: SurfaceOpAppend}, ToolResultPayload{CallID: callID, Name: name, Content: content, IsError: isError})
}

// AppendToolResultMeta records a tool result with the full typed outcome
// metadata (denial marker + identity, timeout classification, replace
// provenance). The payload remains frozen once appended.
func (l *Log) AppendToolResultMeta(p ToolResultPayload) (Event, error) {
	return l.Append(TypeToolResult, &SurfaceOp{Op: SurfaceOpAppend}, p)
}

// AppendTurnBegin opens a turn bracket.
func (l *Log) AppendTurnBegin() (Event, error) {
	return l.Append(TypeTurnBegin, nil, nil)
}

// AppendTurnEnd closes a turn bracket, optionally carrying a reason.
func (l *Log) AppendTurnEnd(reason string) (Event, error) {
	return l.AppendTurnEndKind("", reason)
}

// AppendTurnEndKind closes a turn bracket with an explicit closure kind
// (e.g. TurnEndKindError for failed turns).
func (l *Log) AppendTurnEndKind(kind, reason string) (Event, error) {
	return l.Append(TypeTurnEnd, nil, TurnEndPayload{Kind: kind, Reason: reason})
}

// AppendSubagentSpawned records the durable spawn descriptor in the PARENT
// log (log-only: identity rides the durable stream, never the surface).
func (l *Log) AppendSubagentSpawned(childID, kind, prompt string, depth int) (Event, error) {
	return l.Append(TypeSubagentSpawned, nil, SubagentPayload{
		ChildID: childID, Kind: kind, Prompt: prompt, Depth: depth,
	})
}

// AppendSubagentReport relays a child→parent report into the parent log
// (surface append, role user, provenance subagent-report — the parent
// transcript never credits the child with runtime words).
func (l *Log) AppendSubagentReport(childID, content string) (Event, error) {
	return l.Append(TypeSubagentReport, &SurfaceOp{Op: SurfaceOpAppend}, SubagentPayload{
		ChildID: childID, Content: content, Provenance: SubagentProvenanceReport,
	})
}

// AppendSubagentSettled records the MANAGER-authored settlement notice in
// the parent log (log-only — the notice costs no surface; it is a
// distinct kind from the report by design).
func (l *Log) AppendSubagentSettled(childID, result, reason string) (Event, error) {
	return l.Append(TypeSubagentSettled, nil, SubagentPayload{
		ChildID: childID, Result: result, Reason: reason, Provenance: SubagentProvenanceSettled,
	})
}

// AppendSubagentMessage appends a parent→child message to the CHILD log:
// an addressed user-role inbox message (the inbox is the only FIFO). The
// initial spawn prompt arrives as the first such message.
func (l *Log) AppendSubagentMessage(childID, from, text string) (Event, error) {
	return l.Append(TypeSubagentMessage, &SurfaceOp{Op: SurfaceOpAppend}, SubagentPayload{
		ChildID: childID, From: from, Text: text,
	})
}

// Events returns a copy of the events appended so far (live fold input).
func (l *Log) Events() []Event {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]Event, len(l.events))
	copy(out, l.events)
	return out
}

// SessionID returns the session id from the log's header record ("" if
// the log has no parsable header — impossible for a well-formed log,
// which always opens with session/header). It is the identity the
// turn-context binding and the session→manager registry key on.
func (l *Log) SessionID() string {
	l.mu.RLock()
	events := l.events
	l.mu.RUnlock()
	if len(events) == 0 || events[0].Type != TypeSessionHeader {
		return ""
	}
	var hp HeaderPayload
	if err := json.Unmarshal(events[0].Payload, &hp); err != nil {
		return ""
	}
	return hp.SessionID
}

// SetSpillPolicy arms (nil disarms) the oversize tool-result spill for
// results committed to this log — the per-session commit-time decision
// (see SpillPolicy). It is engine wiring: the writer itself stays
// policy-free; the pipeline consults the log's policy when committing
// tool results. Arm before the first turn of a session; replay never
// reads it (spill files are sidecar state, not replay input).
func (l *Log) SetSpillPolicy(p *SpillPolicy) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.spill = p
}

// SpillPolicy returns the armed spill policy, or nil (today's inline
// behavior, byte-stable).
func (l *Log) SpillPolicy() *SpillPolicy {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.spill
}

// Surface derives the LLM-visible message surface from the live event
// list — the same fold replay applies to the persisted log.
func (l *Log) Surface() ([]Message, error) {
	l.mu.RLock()
	events := make([]Event, len(l.events))
	copy(events, l.events)
	l.mu.RUnlock()
	return DeriveMessages(events)
}

// Close closes the underlying writer when it is an io.Closer.
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closer != nil {
		return l.closer.Close()
	}
	return nil
}

// writeEvent assigns the next monotonic seq, marshals one complete JSONL
// record, appends the newline, and updates the log's sequence + live
// event mirror — atomically under the write lock, so concurrent Append
// calls serialize at record granularity and never observe or mint the
// same seq.
func (l *Log) writeEvent(ev Event) (Event, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.writeEventLocked(ev)
}

// writeEventLocked is writeEvent's body for a caller that ALREADY holds
// l.mu (write) — the internal seam Compact's atomic sequence appends
// through. Precondition: l.mu is held for writing.
func (l *Log) writeEventLocked(ev Event) (Event, error) {
	ev.Seq = l.seq + 1
	line, err := json.Marshal(ev)
	if err != nil {
		return Event{}, fmt.Errorf("session: marshal %s event: %w", ev.Type, err)
	}
	line = append(line, '\n')
	if _, err := l.w.Write(line); err != nil {
		return Event{}, fmt.Errorf("session: append %s: %w", ev.Type, err)
	}
	l.seq = ev.Seq
	l.events = append(l.events, ev)
	return ev, nil
}

// validateSurfaceOp enforces the fail-loud surface contract at append
// time: message-bearing events need a surfaceOp with a known op kind, and
// a replace span must be well-ordered (bounds against the live surface are
// checked by the projection, which owns positional truth).
func validateSurfaceOp(eventType string, surface *SurfaceOp) error {
	if !messageBearing(eventType) {
		return nil
	}
	if surface == nil {
		return fmt.Errorf("session: event type %q is message-bearing and requires a surfaceOp", eventType)
	}
	switch surface.Op {
	case SurfaceOpAppend:
		return nil
	case SurfaceOpReplace:
		if surface.End < surface.Start {
			return fmt.Errorf("session: %s replace span [%d,%d) has End < Start", eventType, surface.Start, surface.End)
		}
		return nil
	default:
		return fmt.Errorf("session: %s carries unknown surfaceOp %q", eventType, surface.Op)
	}
}
