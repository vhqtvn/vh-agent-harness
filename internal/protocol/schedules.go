// schedules.go — the schedule/* method family over the host protocol:
// the wire surface of internal/jobs' Scheduler. The scheduler itself is
// ENGINE WIRING — constructed, started, and drained by the composition
// root (vh-agentd: state file under the session dir, tracker-routed to
// the active session's jobs.Manager, started before Serve, drained Stop
// at shutdown) — never owned by the protocol package. Wire contract
// (docs/native-engine/host-protocol.md §4c):
//
//	schedule/add {name, kind?, after?|at?, every?, payload?}
//	  → the canonical record + nextRun (UTC) — validation is jobs-side
//	  (slug names/kinds, exactly-one-start cadence, UTC
//	  canonicalization, valid-JSON payload); a duplicate name is -32602
//	  with the descriptive text. A persist failure is the ENGINE's
//	  fault, not the caller's: -32000 carrying the underlying text
//	  (scheduleWireErr splits the two classes). The idle gate is
//	  engine-side wiring, never a wire param.
//	schedule/list {} → {schedules:[…]} — the scheduler snapshot in
//	  dispatch-priority order; {schedules:[]} without an active session
//	  or an unwired seam (jobs/status mirror).
//	schedule/remove {name} → {removed:true} — unregisters and persists
//	  atomically; an unknown name is -32602 carrying the typed
//	  ErrScheduleNotFound text, a persist failure is -32000 with the
//	  underlying text. Remove+re-add is the v1 pause path.
//
// Events: a due schedule dispatches as an ORDINARY job/enqueued
// (kind "sched-<name>" by default) through the active session's
// jobs.Manager, so settlement and reporting ride the existing job/*
// event stream and jobs/status fold — no new notification kind, no
// scheduler-specific event vocabulary. Versioning (the B2 decision,
// restated for B3): NEW method names are additive under v1 (§8 forbids
// adding FIELDS to existing params/results without a bump — this slice
// adds none; the initialize capabilities object is untouched, so
// ProtocolVersion stays 1).
//
// Concurrency shape: handlers run on per-request goroutines and touch
// only the scheduler's non-blocking registration seams (Add/Remove/
// Snapshot — the scheduler's own mutex). Tick/dispatch stay on the
// scheduler's OWN goroutine (the Start-ed poll loop the engine wiring
// owns), so a wire handler never runs a dispatch and a due pass is at
// worst serialized with one registration (the contract Add already
// carried in B1).
package protocol

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/jobs"
)

// ScheduleManager is the per-session scheduler seam (satisfied by
// *jobs.Scheduler). Kept narrow like SubagentSpawner: the server
// depends on the registration/snapshot surface, never on the
// scheduler's loop, clock, or state file.
type ScheduleManager interface {
	Add(spec jobs.ScheduleSpec) (jobs.ScheduleRecord, error)
	Remove(name string) error
	Snapshot() []jobs.ScheduleRecord
}

// scheduleAddParams is the schedule/add request body — the wire form of
// jobs.ScheduleSpec: durations are integer nanoseconds (the persisted
// state form), `at` is an RFC3339 instant (strict JSON decoding
// enforces the grammar; the scheduler canonicalizes to UTC and rejects
// machine-dependent local times).
type scheduleAddParams struct {
	Name    string          `json:"name"`
	Kind    string          `json:"kind,omitempty"`
	After   int64           `json:"after,omitempty"`
	At      *time.Time      `json:"at,omitempty"`
	Every   int64           `json:"every,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// scheduleDTO is the stable wire DTO for one registered schedule — a
// PLAIN COPY of a jobs.ScheduleRecord (never a marshal of an internal
// struct): the canonical spec (after resolved into at, at in UTC) plus
// its next-run cursor (nextRun, always UTC).
type scheduleDTO struct {
	Name    string          `json:"name"`
	Kind    string          `json:"kind,omitempty"`
	At      *time.Time      `json:"at,omitempty"`
	Every   int64           `json:"every,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	NextRun time.Time       `json:"nextRun"`
}

// toScheduleDTO copies one record into its wire form (UTC-canonical).
func toScheduleDTO(r jobs.ScheduleRecord) scheduleDTO {
	dto := scheduleDTO{
		Name:    r.Spec.Name,
		Kind:    r.Spec.Kind,
		Every:   int64(r.Spec.Every),
		Payload: r.Spec.Payload,
		NextRun: r.NextDue.UTC(),
	}
	if r.Spec.At != nil {
		at := r.Spec.At.UTC()
		dto.At = &at
	}
	return dto
}

// scheduleWireErr classifies a scheduler-seam error onto the wire: jobs-side
// validation refusals (the fail-closed table) and unknown names are CALLER
// faults (-32602, §4c — the params were invalid); persist/infrastructure
// failures (the typed jobs.ErrSchedulePersist) are ENGINE faults (-32000 —
// the request was valid, the state layer failed). In both classes the
// underlying message travels verbatim.
func scheduleWireErr(err error) *Error {
	if errors.Is(err, jobs.ErrSchedulePersist) {
		return &Error{Code: ErrEngine, Message: err.Error()}
	}
	return &Error{Code: ErrInvalidParams, Message: err.Error()}
}

// handleScheduleAdd registers one schedule through the session's seam.
// Fail-closed params (strict decode + the jobs-side validation table)
// and duplicate names are -32602 carrying the validator's stable text;
// a persist failure is the typed engine fault -32000; a missing seam is
// -32000 (engine built without scheduler wiring).
func handleScheduleAdd(ctx context.Context, s *Server, req *Request) (json.RawMessage, *Error) {
	es, perr := s.requireSession()
	if perr != nil {
		return nil, perr
	}
	var p scheduleAddParams
	if perr := decodeParams(req, &p, false); perr != nil {
		return nil, perr
	}
	if p.Name == "" {
		return nil, &Error{Code: ErrInvalidParams, Message: "name is required and must be non-empty"}
	}
	if es.Schedules == nil {
		return nil, &Error{Code: ErrEngine, Message: "scheduler not configured on this engine (no schedule seam wired)"}
	}
	rec, err := es.Schedules.Add(jobs.ScheduleSpec{
		Name:    p.Name,
		Kind:    p.Kind,
		After:   time.Duration(p.After),
		At:      p.At,
		Every:   time.Duration(p.Every),
		Payload: p.Payload,
	})
	if err != nil {
		return nil, scheduleWireErr(err)
	}
	result, merr := json.Marshal(toScheduleDTO(rec))
	if merr != nil {
		return nil, &Error{Code: ErrEngine, Message: merr.Error()}
	}
	return result, nil
}

// handleScheduleList returns the registered schedules in
// dispatch-priority order with their UTC next-run cursors. Without an
// active session — or on an engine without the seam — it is an honest
// empty list (the jobs/status mirror: absent wiring means absent
// schedules).
func handleScheduleList(ctx context.Context, s *Server, req *Request) (json.RawMessage, *Error) {
	s.mu.Lock()
	es := s.active
	s.mu.Unlock()
	var schedules []scheduleDTO
	if es != nil && es.Schedules != nil {
		for _, rec := range es.Schedules.Snapshot() {
			schedules = append(schedules, toScheduleDTO(rec))
		}
	}
	if schedules == nil {
		schedules = []scheduleDTO{}
	}
	result, merr := json.Marshal(struct {
		Schedules []scheduleDTO `json:"schedules"`
	}{schedules})
	if merr != nil {
		return nil, &Error{Code: ErrEngine, Message: merr.Error()}
	}
	return result, nil
}

// scheduleRemoveParams is the schedule/remove request body.
type scheduleRemoveParams struct {
	Name string `json:"name"`
}

// handleScheduleRemove unregisters one schedule by name (atomically
// persisted). An unknown name is -32602 carrying the typed
// ErrScheduleNotFound text; a persist failure is the typed engine fault
// -32000 with the underlying text.
func handleScheduleRemove(ctx context.Context, s *Server, req *Request) (json.RawMessage, *Error) {
	es, perr := s.requireSession()
	if perr != nil {
		return nil, perr
	}
	var p scheduleRemoveParams
	if perr := decodeParams(req, &p, false); perr != nil {
		return nil, perr
	}
	if p.Name == "" {
		return nil, &Error{Code: ErrInvalidParams, Message: "name is required and must be non-empty"}
	}
	if es.Schedules == nil {
		return nil, &Error{Code: ErrEngine, Message: "scheduler not configured on this engine (no schedule seam wired)"}
	}
	if err := es.Schedules.Remove(p.Name); err != nil {
		return nil, scheduleWireErr(err)
	}
	result, merr := json.Marshal(struct {
		Removed bool `json:"removed"`
	}{true})
	if merr != nil {
		return nil, &Error{Code: ErrEngine, Message: merr.Error()}
	}
	return result, nil
}
