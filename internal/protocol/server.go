// server.go — the host-protocol server: a read-dispatch loop over one
// Conn with per-request handler goroutines (a blocking session/prompt
// must never stop the loop from serving approval/respond), serialized
// responses, and fail-closed inbound discipline.
//
// Engine seams (telegraf-style composition — narrow consumer-side
// interfaces, no global state; tmp/solution-brief/02-solution-brief.md
// v3 Architecture Map "Modularity" row):
//
//   - SessionSource.NewSession → session/create (real session.Log file,
//     with appends observable through the injected sink);
//   - TurnRunner.RunTurn       → session/prompt (the tools.Pipeline seam);
//   - JobDispatcher            → session/dispatch + jobs/status (the
//     jobs.Manager seam).
//
// Method handlers land with their slices; the dispatch table grows.
package protocol

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"runtime/debug"
	"sync"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
	"github.com/vhqtvn/vh-agent-harness/internal/jobs"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// Engine is the narrow composition seam every handler drives: the server
// depends only on these methods, never on global state.
type Engine interface {
	// NewSession opens a new durable session (log + bound jobs manager)
	// at path; every appended event is written through sink (the
	// server's notification fan-out) as it lands. sessionID is minted by
	// the server (or supplied by the client).
	NewSession(path, sessionID string, sink io.Writer) (*EngineSession, error)
	// Adapter returns the LLM adapter used for prompt turns.
	Adapter() adapters.Adapter
	// TurnRunner returns the tool-turn seam (satisfied by *tools.Pipeline).
	TurnRunner() TurnRunner
	// TurnOptions are the adapter parameters for prompt turns.
	TurnOptions() tools.TurnOptions
}

// EngineSession is one live session bound to the engine.
type EngineSession struct {
	Log  *session.Log
	Jobs JobDispatcher
	// Subagents is the per-session subagent seam (spawn/send); nil on
	// engines built without a subagent executor (spawn/send then fail
	// closed -32000; subagent/list still works — it is a pure log fold).
	Subagents SubagentSpawner
	// Schedules is the per-session scheduler seam (schedule/add,
	// schedule/remove, schedule/list); nil on engines built without
	// scheduler wiring (add/remove then fail closed -32000; list is an
	// honest empty — the jobs/status mirror). The ENGINE WIRING owns
	// the scheduler (construction, Start, drained Stop) — the protocol
	// package only registers/reads through this seam.
	Schedules ScheduleManager
	// Path is the engine-resolved durable location of the session log.
	Path string

	// turnMu is the PER-SESSION TURN GATE (ship-review finding 1): it
	// serializes turn-executing requests (session/prompt) so at most
	// ONE turn/begin…turn/end bracket is ever in flight on this
	// session's log — concurrent prompts QUEUE instead of interleaving
	// (the log's own mutex guarantees valid JSONL and contiguous seq,
	// not atomic turn semantics; without the gate, two concurrent turns
	// interleave brackets and each surface derivation can observe the
	// other turn's half-committed events). Read-only seams — surface,
	// jobs status, event subscription, job/subagent dispatch — never
	// take it and stay fully concurrent. Zero-value ready; never copied
	// after construction.
	turnMu sync.Mutex
}

// beginTurn acquires the per-session turn gate: admission to execute a
// RunTurn-driven turn against this session's log. Callers MUST pair it
// with endTurn (usually via defer) on the same goroutine.
func (es *EngineSession) beginTurn() { es.turnMu.Lock() }

// endTurn releases the per-session turn gate.
func (es *EngineSession) endTurn() { es.turnMu.Unlock() }

// TurnRunner is the RunTurn seam (satisfied by *tools.Pipeline).
type TurnRunner interface {
	RunTurn(ctx context.Context, lg *session.Log, ad adapters.Adapter, opts tools.TurnOptions, prompt string) (*tools.TurnReport, error)
}

// JobDispatcher is the async-jobs seam (satisfied by *jobs.Manager).
type JobDispatcher interface {
	Dispatch(kind string, payload json.RawMessage) (jobs.Receipt, error)
	Snapshot() []jobs.Status
	EmitReports() (int, error)
}

// ServerOptions configures one Server.
type ServerOptions struct {
	// ApprovalTimeoutMs bounds each pending approval; 0 waits
	// indefinitely while the connection stays open (disconnect still
	// denies — fail-closed either way).
	ApprovalTimeoutMs int
}

// Server serves the host protocol on one Conn.
//
// Concurrency contract (ship-review finding 1; mirrored in
// docs/native-engine/host-protocol.md §4): every request is handled in
// its own goroutine and responses may arrive in any order. On top of
// that fan-out:
//
//   - session/prompt SERIALIZES PER SESSION (the EngineSession turn
//     gate): one in-flight turn bracket per session log; concurrent
//     prompts queue. Prompt ADMISSION — resolving which session a
//     prompt runs against — is a single atomic decision under mu.
//   - session/create SERIALIZES against every other create END-TO-END
//     (createMu): engine construction (including any engine-decorator
//     active tracking, which happens synchronously inside NewSession),
//     and this server's active-pointer swap land as one non-interleaved
//     stage sequence, so engine/tracker/server can never durably
//     disagree about the active session. A create does NOT wait behind
//     an in-flight prompt: a prompt admitted before supersession
//     completes on its own session, and the new session becomes active
//     immediately.
//   - read-only seams (session/subscribe, session/surface, jobs/status,
//     subagent/list, schedule/list) and the async receipts
//     (session/dispatch, subagent/spawn, subagent/send,
//     approval/respond) stay fully concurrent.
type Server struct {
	engine   Engine
	conn     *Conn
	opts     ServerOptions
	approver *ProtocolApprover
	fanout   *eventFanout

	mu          sync.Mutex
	initialized bool
	closing     bool
	active      *EngineSession

	// createMu serializes session/create (supersede) end-to-end — see
	// the concurrency contract above. It is deliberately SEPARATE from
	// mu: a create holds it across engine/file work while active-session
	// reads stay O(1) pointer loads under mu.
	createMu sync.Mutex

	wg sync.WaitGroup // in-flight request handlers
}

// ApprovalAwareEngine is the optional injection point for engines whose
// tool pipeline consults the wire approval bridge: NewServer calls
// SetApprover before serving, so the engine's TurnRunner can build on
// the protocol approver without global state.
type ApprovalAwareEngine interface {
	SetApprover(a tools.Approver)
}

// NewServer binds engine to conn.
func NewServer(engine Engine, conn *Conn, opts ServerOptions) *Server {
	s := &Server{engine: engine, conn: conn, opts: opts}
	s.approver = NewProtocolApprover(conn, time.Duration(opts.ApprovalTimeoutMs)*time.Millisecond)
	s.fanout = newEventFanout()
	if engine != nil {
		if aa, ok := engine.(ApprovalAwareEngine); ok {
			aa.SetApprover(s.approver)
		}
	}
	return s
}

// handlerFunc is one method handler: it returns a result or a protocol
// error, never both.
type handlerFunc func(ctx context.Context, s *Server, req *Request) (json.RawMessage, *Error)

// handlers is the closed method table (docs/native-engine/host-protocol.md).
var handlers = map[string]handlerFunc{
	"initialize": func(ctx context.Context, s *Server, req *Request) (json.RawMessage, *Error) {
		return handleInitialize(s, req)
	},
	"approval/respond":  handleApprovalRespond,
	"session/create":    handleSessionCreate,
	"session/resume":    handleSessionResume,
	"session/list":      handleSessionList,
	"session/subscribe": handleSessionSubscribe,
	"session/dispatch":  handleSessionDispatch,
	"session/prompt":    handleSessionPrompt,
	"session/surface":   handleSessionSurface,
	"jobs/status":       handleJobsStatus,
	"subagent/spawn":    handleSubagentSpawn,
	"subagent/send":     handleSubagentSend,
	"subagent/list":     handleSubagentList,
	"schedule/add":      handleScheduleAdd,
	"schedule/list":     handleScheduleList,
	"schedule/remove":   handleScheduleRemove,
}

// Serve runs the read-dispatch loop until the transport ends or ctx is
// canceled. On exit it closes the server BEFORE draining in-flight
// request handlers (dsh close ladder simplified for stdio: no signals —
// Close + EOF only).
func (s *Server) Serve(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	go func() {
		// ctx cancel (e.g. the daemon's signal context) closes the
		// server (spec §10). Once the transport itself has terminated,
		// the read-loop exit path below owns the close, so this watcher
		// steps aside instead of outliving Serve on a parking ctx.
		select {
		case <-ctx.Done():
			_ = s.Close()
		case <-s.conn.Done():
		}
	}()
	defer s.wg.Wait()

	for {
		line, err := s.conn.ReadLine()
		if err != nil {
			// Terminal read error — EOF is the client disconnect. Close
			// BEFORE draining handlers (wg.Wait above): Close fires
			// Done, so every pending approval denies IMMEDIATELY (spec
			// §6 "connection closed while pending ⇒ deny all pending",
			// §10) instead of waiting out its timeout — and with
			// approval-timeout 0 the drain below would hang forever.
			// Idempotent with ReadLine's own terminal close.
			_ = s.Close()
			return err // EOF, closed transport, or oversized line
		}
		msg, err := ParseLine(line)
		if err != nil {
			// Malformed line: skip, stay alive, report loudly (dsh
			// malformed-line skip discipline).
			s.protocolError(ErrParse, err.Error())
			continue
		}
		switch msg.Kind {
		case KindRequest:
			s.dispatch(ctx, msg.Request)
		case KindNotification:
			// Forward compatibility: unknown notifications are ignored.
			// (v1 defines no client→server notifications.)
		case KindResponse:
			s.protocolError(ErrInvalidRequest, "unexpected response: the server issued no request")
		case KindInvalid:
			if msg.HasID {
				s.respondError(msg.InvalidID, &Error{Code: ErrInvalidRequest, Message: msg.InvalidReason})
			} else {
				s.protocolError(ErrInvalidRequest, msg.InvalidReason)
			}
		}
	}
}

// dispatch routes one request: closing rejection, initialization gate,
// method lookup, then a handler goroutine (fan-out keeps the read loop
// live while handlers block — the approval bridge depends on this).
func (s *Server) dispatch(ctx context.Context, req *Request) {
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		s.respondError(req.ID, &Error{Code: ErrInvalidRequest, Message: "server is closing"})
		return
	}
	if !s.initialized && req.Method != "initialize" {
		s.mu.Unlock()
		s.respondError(req.ID, &Error{Code: ErrInitializeRequired, Message: "initialize required before " + req.Method})
		return
	}
	h, ok := handlers[req.Method]
	s.mu.Unlock()

	if !ok {
		s.respondError(req.ID, &Error{Code: ErrMethodNotFound, Message: fmt.Sprintf("method not found: %s", req.Method)})
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				s.respondError(req.ID, &Error{Code: ErrEngine, Message: fmt.Sprintf("handler panic: %v", r)})
				debug.PrintStack()
			}
		}()
		result, perr := h(ctx, s, req)
		if perr != nil {
			s.respondError(req.ID, perr)
			return
		}
		s.respond(req.ID, result)
	}()
}

// Close stops accepting work: new requests are rejected, the transport
// is closed (unblocking the read loop and every pending approval —
// fail-closed), and Serve drains in-flight handlers before returning.
func (s *Server) Close() error {
	s.mu.Lock()
	s.closing = true
	s.mu.Unlock()
	return s.conn.Close()
}

// respond writes a success response.
func (s *Server) respond(id int64, result json.RawMessage) {
	b, err := MarshalResponse(id, result)
	if err != nil {
		s.protocolError(ErrEngine, err.Error())
		return
	}
	_ = s.conn.WriteLine(b)
}

// respondError writes an error response (data, when present, travels).
func (s *Server) respondError(id int64, e *Error) {
	b, err := MarshalResponseError(id, e.Code, e.Message, e.Data)
	if err != nil {
		return
	}
	_ = s.conn.WriteLine(b)
}

// protocolError emits the protocol/error notification (malformed or
// unattributable inbound violations). Emission is best-effort: a dead
// connection cannot be reported on.
func (s *Server) protocolError(code int, message string) {
	params, err := json.Marshal(struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}{code, message})
	if err != nil {
		return
	}
	_ = s.conn.Notify("protocol/error", params)
}

// jsonDecoder builds a strict decoder (unknown fields rejected).
func jsonDecoder(b []byte, v any) *json.Decoder {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	return d
}
