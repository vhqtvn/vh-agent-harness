// subagents.go — slice B2 protocol surface: the subagent/* method family
// over the host protocol. Wire contract (docs/native-engine/
// host-protocol.md §4):
//
//	subagent/spawn {role?, prompt, mode: oneshot|continuable,
//	                seedFromParent?: n}
//	  → {childId}                — job-style enqueue receipt: returns
//	  IMMEDIATELY, before the child's first turn (async contract, §7
//	  discipline). Depth is auto-derived from the session's persisted
//	  header — never client-supplied. seedFromParent copies the parent's
//	  last-n COMPLETED turns into the child log (fork seeding).
//	subagent/send {childId, message} → {queued:true}
//	  — follow-up inbox message; continuable, not-yet-settled children
//	  only (one turn per send, FIFO).
//	subagent/list {} → {children:[…]}
//	  — fold-derived snapshot (running/waiting/settled + contentSeq);
//	  honest empty list without an active session (jobs/status mirror).
//
// Events: the child's report/settle records are ordinary parent-log
// session events (subagent/report, subagent/settled) and therefore
// already reach session/event subscribers through the existing fan-out
// — no new notification kind. Versioning: NEW METHODS are additive
// under v1 (§8 forbids adding FIELDS to existing params/results without
// a bump — this slice adds none; the initialize result and its
// capabilities object are untouched, so ProtocolVersion stays 1).
//
// Concurrency shape: handlers run on per-request goroutines (server.go
// dispatch) and touch only the manager's non-blocking enqueue seams;
// child turns run on the manager's OWN dispatch goroutine, so a long
// child turn can never block the protocol loop, a concurrent
// session/prompt, or the spawn/send receipts.
package protocol

import (
	"context"
	"encoding/json"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/subagents"
)

// SubagentSpawner is the per-session subagent seam (satisfied by
// *subagents.Manager). Kept narrow like JobDispatcher: the server
// depends on the enqueue/messaging surface, never on the manager
// internals.
type SubagentSpawner interface {
	SpawnWithOptions(opts subagents.SpawnOptions) (subagents.Receipt, error)
	SendMessage(childID, text string) error
	Snapshot() []subagents.Descriptor
	Drain()
	Stop()
}

// Wire mode vocabulary (mapped onto the session kind constants — the
// wire says "oneshot", the log vocabulary says "one-shot").
const (
	SubagentModeOneshot     = "oneshot"
	SubagentModeContinuable = "continuable"
)

// subagentSpawnParams is the subagent/spawn request body.
type subagentSpawnParams struct {
	Role           string `json:"role,omitempty"`
	Prompt         string `json:"prompt"`
	Mode           string `json:"mode"`
	SeedFromParent int    `json:"seedFromParent,omitempty"`
}

// subagentSpawnResult is the enqueue receipt.
type subagentSpawnResult struct {
	ChildID string `json:"childId"`
}

// subagentSendParams is the subagent/send request body.
type subagentSendParams struct {
	ChildID string `json:"childId"`
	Message string `json:"message"`
}

// subagentSendResult acknowledges one queued follow-up turn.
type subagentSendResult struct {
	Queued bool `json:"queued"`
}

// subagentListResult is the fold-derived snapshot.
type subagentListResult struct {
	Children []subagents.Status `json:"children"`
}

// handleSubagentSpawn validates the request (fail-closed params, same
// discipline as session/dispatch: manager refusals — the depth fence,
// unknown state — are -32602 with the manager's descriptive text) and
// returns the enqueue receipt immediately.
func handleSubagentSpawn(ctx context.Context, s *Server, req *Request) (json.RawMessage, *Error) {
	es, perr := s.requireSession()
	if perr != nil {
		return nil, perr
	}
	var p subagentSpawnParams
	if perr := decodeParams(req, &p, false); perr != nil {
		return nil, perr
	}
	if p.Prompt == "" {
		return nil, &Error{Code: ErrInvalidParams, Message: "prompt is required and must be non-empty"}
	}
	var kind string
	switch p.Mode {
	case SubagentModeOneshot:
		kind = session.SubagentKindOneShot
	case SubagentModeContinuable:
		kind = session.SubagentKindContinuable
	default:
		return nil, &Error{Code: ErrInvalidParams, Message: "mode must be \"oneshot\" or \"continuable\""}
	}
	if p.SeedFromParent < 0 {
		return nil, &Error{Code: ErrInvalidParams, Message: "seedFromParent must be >= 0"}
	}
	if es.Subagents == nil {
		return nil, &Error{Code: ErrEngine, Message: "subagents not configured on this engine (no executor wired)"}
	}
	receipt, err := es.Subagents.SpawnWithOptions(subagents.SpawnOptions{
		Kind:           kind,
		Prompt:         p.Prompt,
		Role:           p.Role,
		SeedFromParent: p.SeedFromParent,
	})
	if err != nil {
		return nil, &Error{Code: ErrInvalidParams, Message: err.Error()}
	}
	result, merr := json.Marshal(subagentSpawnResult{ChildID: receipt.ChildID})
	if merr != nil {
		return nil, &Error{Code: ErrEngine, Message: merr.Error()}
	}
	return result, nil
}

// handleSubagentSend delivers one follow-up to a continuable child's
// inbox (one queued turn per send).
func handleSubagentSend(ctx context.Context, s *Server, req *Request) (json.RawMessage, *Error) {
	es, perr := s.requireSession()
	if perr != nil {
		return nil, perr
	}
	var p subagentSendParams
	if perr := decodeParams(req, &p, false); perr != nil {
		return nil, perr
	}
	if p.ChildID == "" || p.Message == "" {
		return nil, &Error{Code: ErrInvalidParams, Message: "childId and message are required and must be non-empty"}
	}
	if es.Subagents == nil {
		return nil, &Error{Code: ErrEngine, Message: "subagents not configured on this engine (no executor wired)"}
	}
	if err := es.Subagents.SendMessage(p.ChildID, p.Message); err != nil {
		return nil, &Error{Code: ErrInvalidParams, Message: err.Error()}
	}
	result, merr := json.Marshal(subagentSendResult{Queued: true})
	if merr != nil {
		return nil, &Error{Code: ErrEngine, Message: merr.Error()}
	}
	return result, nil
}

// handleSubagentList returns the fold-derived snapshot. It is a pure
// fold over the active session's log (no manager required — the fold is
// the truth), in spawn order; without an active session it is an honest
// empty list (jobs/status mirror).
func handleSubagentList(ctx context.Context, s *Server, req *Request) (json.RawMessage, *Error) {
	s.mu.Lock()
	es := s.active
	s.mu.Unlock()
	var children []subagents.Status
	if es != nil {
		var err error
		children, err = subagents.FoldStatus(es.Log.Events())
		if err != nil {
			return nil, &Error{Code: ErrEngine, Message: err.Error()}
		}
	}
	if children == nil {
		children = []subagents.Status{}
	}
	result, merr := json.Marshal(subagentListResult{Children: children})
	if merr != nil {
		return nil, &Error{Code: ErrEngine, Message: merr.Error()}
	}
	return result, nil
}
