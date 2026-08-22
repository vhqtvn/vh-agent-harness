// handlers.go — the engine-backed method table (session lifecycle,
// async dispatch, jobs status, surface snapshot) and the eventFanout:
// the io.Writer the engine threads between the session log file and the
// wire, turning every appended session event into a session/event
// notification for subscribed connections (dsh `session.event`
// unfiltered-stream pattern; filtering is a per-subscription option).
//
// Because session.Log performs exactly one Write per appended record
// (one complete JSONL line, under the log's own lock), the fanout
// observes whole events only — never fragments.
package protocol

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/vhqtvn/vh-agent-harness/internal/jobs"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// --- event fan-out -----------------------------------------------------------

// fanoutSub is one connection's subscription: an optional event-type
// filter and the delivery callback (returns false to self-remove on a
// broken connection).
type fanoutSub struct {
	types   map[string]bool // nil ⇒ every event type
	deliver func(ev session.Event) bool
}

// eventFanout fans appended session events out to subscribers. It never
// errors as a Writer: a session-log append must never fail because a
// subscriber is broken (durability outranks observability — the log is
// the source of truth, the stream is a projection).
type eventFanout struct {
	mu   sync.Mutex
	subs map[int]*fanoutSub
	next int
}

func newEventFanout() *eventFanout {
	return &eventFanout{subs: make(map[int]*fanoutSub)}
}

// Write implements io.Writer: p is one complete JSONL event record.
func (f *eventFanout) Write(p []byte) (int, error) {
	var ev session.Event
	if err := json.Unmarshal(p, &ev); err != nil {
		// Unreachable from session.Log (it marshals Event itself); a
		// foreign writer's garbage is dropped, never propagated.
		return len(p), nil
	}
	f.mu.Lock()
	var broken []int
	for id, sub := range f.subs {
		if sub.types != nil && !sub.types[ev.Type] {
			continue
		}
		if !sub.deliver(ev) {
			broken = append(broken, id)
		}
	}
	for _, id := range broken {
		delete(f.subs, id)
	}
	f.mu.Unlock()
	return len(p), nil
}

// subscribe registers one subscription; deliver returns false to
// self-remove (broken connection).
func (f *eventFanout) subscribe(types map[string]bool, deliver func(ev session.Event) bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next++
	f.subs[f.next] = &fanoutSub{types: types, deliver: deliver}
}

// --- method handlers ---------------------------------------------------------

// createParams is the session/create request body.
type createParams struct {
	Path      string `json:"path,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
}

type createResult struct {
	SessionID string `json:"sessionId"`
	Path      string `json:"path"`
}

// handleSessionCreate opens a new durable session and makes it active.
// Creating a new session supersedes the previous one (v1 is
// single-active-session; multi-session muxing is a stated non-goal).
//
// The create critical section runs under s.createMu END-TO-END (engine
// construction + every synchronous active-tracking update inside
// engine.NewSession + this server's active swap), so concurrent creates
// cannot interleave their stages and leave engine/tracker/server
// disagreeing about the active session (ship-review finding 1). It does
// NOT wait behind in-flight prompts: a prompt admitted before the
// supersession completes on its own session; the new session becomes
// active as soon as this critical section ends.
func handleSessionCreate(ctx context.Context, s *Server, req *Request) (json.RawMessage, *Error) {
	var p createParams
	if perr := decodeParams(req, &p, true); perr != nil {
		return nil, perr
	}
	if p.SessionID == "" {
		minted, err := mintSessionID()
		if err != nil {
			// Fail-closed (F6): a crypto/rand failure refuses session
			// creation rather than degrading to a guessable id.
			return nil, &Error{Code: ErrEngine, Message: err.Error()}
		}
		p.SessionID = minted
	}
	s.createMu.Lock()
	es, err := s.engine.NewSession(p.Path, p.SessionID, s.fanout)
	if err == nil {
		s.mu.Lock()
		s.active = es
		s.mu.Unlock()
	}
	s.createMu.Unlock()
	if err != nil {
		var spe *SessionPathError
		if errors.As(err, &spe) {
			// Client-controlled path rejected by confinement (TB-F1):
			// a params error, not an engine fault — clean -32602, no
			// partial state (nothing was created or superseded).
			return nil, &Error{Code: ErrInvalidParams, Message: err.Error()}
		}
		return nil, &Error{Code: ErrEngine, Message: err.Error()}
	}
	result, merr := json.Marshal(createResult{SessionID: p.SessionID, Path: es.Path})
	if merr != nil {
		return nil, &Error{Code: ErrEngine, Message: merr.Error()}
	}
	return result, nil
}

// subscribeParams is the session/subscribe request body.
type subscribeParams struct {
	Types []string `json:"types,omitempty"`
}

// handleSessionSubscribe registers this connection's event
// subscription. The stream is LIVE-ONLY: events appended before
// subscribing are not replayed (history belongs to the log; see
// session/surface and the replay tooling). A type filter narrows the
// stream to the listed session event types.
func handleSessionSubscribe(ctx context.Context, s *Server, req *Request) (json.RawMessage, *Error) {
	var p subscribeParams
	if perr := decodeParams(req, &p, true); perr != nil {
		return nil, perr
	}
	var types map[string]bool
	if len(p.Types) > 0 {
		types = make(map[string]bool, len(p.Types))
		for _, t := range p.Types {
			types[t] = true
		}
	}
	s.fanout.subscribe(types, func(ev session.Event) bool {
		params, err := json.Marshal(ev)
		if err != nil {
			return false
		}
		return s.conn.Notify("session/event", params) == nil
	})
	result, merr := json.Marshal(struct {
		Subscribed bool `json:"subscribed"`
	}{true})
	if merr != nil {
		return nil, &Error{Code: ErrEngine, Message: merr.Error()}
	}
	return result, nil
}

// dispatchParams is the session/dispatch request body.
type dispatchParams struct {
	Kind    string          `json:"kind"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// handleSessionDispatch enqueues one background job and returns the
// receipt IMMEDIATELY (the async contract: dispatch → receipt{jobId} →
// events → settlement; operator decision 5, 2026-08-19b — "enqueue
// receipt only, never blocks on completion").
func handleSessionDispatch(ctx context.Context, s *Server, req *Request) (json.RawMessage, *Error) {
	es, perr := s.requireSession()
	if perr != nil {
		return nil, perr
	}
	var p dispatchParams
	if perr := decodeParams(req, &p, false); perr != nil {
		return nil, perr
	}
	receipt, err := es.Jobs.Dispatch(p.Kind, p.Payload)
	if err != nil {
		return nil, &Error{Code: ErrInvalidParams, Message: err.Error()}
	}
	result, merr := json.Marshal(receipt)
	if merr != nil {
		return nil, &Error{Code: ErrEngine, Message: merr.Error()}
	}
	return result, nil
}

// handleJobsStatus returns the fold-derived job snapshot in enqueue
// order. Without an active session it is an honest empty list (jobs
// live per session).
func handleJobsStatus(ctx context.Context, s *Server, req *Request) (json.RawMessage, *Error) {
	s.mu.Lock()
	es := s.active
	s.mu.Unlock()
	var snapshot []jobs.Status
	if es != nil {
		snapshot = es.Jobs.Snapshot()
	}
	if snapshot == nil {
		snapshot = []jobs.Status{}
	}
	result, merr := json.Marshal(struct {
		Jobs []jobs.Status `json:"jobs"`
	}{snapshot})
	if merr != nil {
		return nil, &Error{Code: ErrEngine, Message: merr.Error()}
	}
	return result, nil
}

// promptParams is the session/prompt request body.
type promptParams struct {
	Text string `json:"text"`
}

type promptResult struct {
	Content   string             `json:"content"`
	ToolCalls []promptToolCall   `json:"toolCalls"`
	Results   []promptToolResult `json:"results"`
}

type promptToolCall struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type promptToolResult struct {
	CallID  string `json:"callId"`
	Name    string `json:"name"`
	Content string `json:"content"`
	IsError bool   `json:"isError"`
	Denied  bool   `json:"denied,omitempty"`
}

// handleSessionPrompt runs ONE synchronous tool turn (the RunTurn seam;
// retries and multi-turn loops are engine-internal and stay off the
// wire). Settled-but-unreported jobs are surfaced first so the model
// sees each settlement exactly once before the surface is derived
// (dsh reported-flag discipline).
//
// Per-session serialization (ship-review finding 1): admission resolves
// the active session in ONE atomic decision (requireSession's single
// mu acquisition), then the session's turn gate is held for the WHOLE
// turn — concurrent prompts against the same session queue behind the
// gate, so brackets never interleave in one log. A create that
// supersedes mid-wait does not invalidate the admission: the prompt
// completes on the session it was admitted against (documented
// replacement semantics).
func handleSessionPrompt(ctx context.Context, s *Server, req *Request) (json.RawMessage, *Error) {
	es, perr := s.requireSession()
	if perr != nil {
		return nil, perr
	}
	var p promptParams
	if perr := decodeParams(req, &p, false); perr != nil {
		return nil, perr
	}
	es.beginTurn()
	defer es.endTurn()
	if _, err := es.Jobs.EmitReports(); err != nil {
		return nil, &Error{Code: ErrEngine, Message: err.Error()}
	}
	report, err := s.engine.TurnRunner().RunTurn(ctx, es.Log, s.engine.Adapter(), s.engine.TurnOptions(), p.Text)
	if err != nil {
		return nil, &Error{Code: ErrEngine, Message: err.Error()}
	}
	out := promptResult{Content: ""}
	if report != nil && report.Response != nil {
		out.Content = report.Response.Content
		for _, tc := range report.Response.ToolCalls {
			out.ToolCalls = append(out.ToolCalls, promptToolCall{ID: tc.ID, Name: tc.Name})
		}
	}
	for _, r := range report.Results {
		out.Results = append(out.Results, promptToolResult{
			CallID: r.CallID, Name: r.Name, Content: r.Content, IsError: r.IsError, Denied: r.Denied,
		})
	}
	result, merr := json.Marshal(out)
	if merr != nil {
		return nil, &Error{Code: ErrEngine, Message: merr.Error()}
	}
	return result, nil
}

// handleSessionSurface returns the current DeriveMessages snapshot,
// emitting pending settled-job notices first (same discipline as
// session/prompt: the surface is only derived after the model-cost
// guard has run).
func handleSessionSurface(ctx context.Context, s *Server, req *Request) (json.RawMessage, *Error) {
	es, perr := s.requireSession()
	if perr != nil {
		return nil, perr
	}
	if _, err := es.Jobs.EmitReports(); err != nil {
		return nil, &Error{Code: ErrEngine, Message: err.Error()}
	}
	msgs, err := es.Log.Surface()
	if err != nil {
		return nil, &Error{Code: ErrEngine, Message: err.Error()}
	}
	result, merr := json.Marshal(struct {
		Messages []session.Message `json:"messages"`
	}{msgs})
	if merr != nil {
		return nil, &Error{Code: ErrEngine, Message: merr.Error()}
	}
	return result, nil
}

// requireSession returns the active session or the ErrNoSession error.
func (s *Server) requireSession() (*EngineSession, *Error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active == nil {
		return nil, &Error{Code: ErrNoSession, Message: "no active session (session/create first)"}
	}
	return s.active, nil
}

// randRead is the crypto/rand seam (swappable only in tests).
var randRead = rand.Read

// mintSessionID mints a fresh session id (sess-<16 hex>). It is
// fail-closed (F6): a crypto/rand failure returns the error instead of
// degrading to a time-derived fallback — a predictable id weakens the
// session namespace (collisions supersede live sessions), so refusal is
// the honest outcome. Callers surface the error as an engine error.
func mintSessionID() (string, error) {
	var b [8]byte
	if _, err := randRead(b[:]); err != nil {
		return "", fmt.Errorf("protocol: mint session id: crypto/rand failed: %w", err)
	}
	return "sess-" + hex.EncodeToString(b[:]), nil
}
