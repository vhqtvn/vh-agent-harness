// Package record defines the pure, I/O-free data model for the typed
// memory-record layer of vh-agent-harness.
//
// A Record is the smallest addressable atom of agent memory. Records are
// persisted as append-only JSON Lines by the sibling store package
// (internal/memory/store) beside — but NEVER replacing — the existing flat
// memory files (brief / resolved_context / open_questions and the workstream
// memory files). This package only describes the shape of a record and
// validates it; it performs no filesystem or network I/O and imports no
// framework, keeping the repo's "domain/core pure" rule.
//
// The 11-field shape, the three Type enums (persona / episodic / instruction),
// the four Priority tiers, and the two Scopes are the locked contract of the
// cognitive-layer work inspired by TencentDB-Agent-Memory. The implementation
// is Go-native and does not vendor or import that project's TypeScript.
//
// Forward compatibility: JSON decoding ignores unknown fields so that
// additive schema evolution (new optional fields) does not break older
// readers. Unknown enum values are rejected on decode and on explicit
// Validate calls.
package record

import (
	"encoding/json"
	"fmt"
	"time"
)

// Type classifies the cognitive role of a memory record.
//
//   - TypePersona: durable agent/operator preference or identity-like guidance.
//     Example: "the operator prefers concise diffs".
//   - TypeEpisodic: an event, decision, checkpoint, or remembered occurrence.
//     Example: "slice 2 reader shipped on 2026-07-07".
//   - TypeInstruction: an actionable standing rule or workflow guidance.
//     Example: "always run gofmt before commit".
type Type string

const (
	TypePersona     Type = "persona"
	TypeEpisodic    Type = "episodic"
	TypeInstruction Type = "instruction"
)

// Valid reports whether t is one of the known Type enum values.
func (t Type) Valid() bool {
	switch t {
	case TypePersona, TypeEpisodic, TypeInstruction:
		return true
	}
	return false
}

// Priority is a manually- or agent-assigned retrieval ordering hint. It is
// NOT model-derived: it reflects explicit operator/agent intent about how
// prominently a record should surface during retrieval.
//
// Rank orders tiers from least to most prominent so that sort callers can
// sort descending by Rank to surface critical records first.
type Priority string

const (
	PriorityLow      Priority = "low"
	PriorityNormal   Priority = "normal"
	PriorityHigh     Priority = "high"
	PriorityCritical Priority = "critical"
)

// Valid reports whether p is one of the known Priority enum values.
func (p Priority) Valid() bool {
	switch p {
	case PriorityLow, PriorityNormal, PriorityHigh, PriorityCritical:
		return true
	}
	return false
}

// Rank returns a deterministic ordering integer for p, where higher means
// more prominent: low=0, normal=1, high=2, critical=3. Unknown priorities
// rank below low (-1) so they sort last rather than masquerading as normal.
func (p Priority) Rank() int {
	switch p {
	case PriorityLow:
		return 0
	case PriorityNormal:
		return 1
	case PriorityHigh:
		return 2
	case PriorityCritical:
		return 3
	}
	return -1
}

// Scope bounds the lifetime and addressability of a record.
//
//   - ScopeSession: a record scoped to a single session alias under
//     .opencode/state/sessions/<alias>/memory/records.jsonl.
//   - ScopeWorkstream: a record scoped to a cross-session workstream slug under
//     .opencode/state/workstreams/<slug>/records.jsonl.
type Scope string

const (
	ScopeSession    Scope = "session"
	ScopeWorkstream Scope = "workstream"
)

// Valid reports whether s is one of the known Scope enum values.
func (s Scope) Valid() bool {
	switch s {
	case ScopeSession, ScopeWorkstream:
		return true
	}
	return false
}

// Record is a single typed memory atom.
//
// All pointer fields are optional and use omitempty on the wire: a nil value
// means "unset". The non-pointer fields (ID, Type, Priority, Scope, CreatedAt,
// UpdatedAt, Body) are required and validated by Validate.
//
// Time fields serialize via time.Time's default JSON marshaling, which emits
// RFC3339 with optional fractional seconds (RFC3339Nano when present). That is
// strictly compatible with RFC3339 readers.
//
// Provenance: SourceRef is a free-form pointer to where this record came from
// (a file path, a command, a checkpoint/handoff id, a message ref). It is
// informational and not dereferenced by this layer.
type Record struct {
	// ID is the stable record identifier (e.g. ULID/UUID). Appending a Record
	// whose ID already exists in a file is the supported update path: readers
	// deduplicate by ID keeping the latest UpdatedAt (last-write-wins).
	ID string `json:"id"`
	// Type is the cognitive role of the record. See the Type enum.
	Type Type `json:"type"`
	// Priority is the retrieval ordering hint. See the Priority enum.
	Priority Priority `json:"priority"`
	// Scope bounds the record to a session or workstream. See the Scope enum.
	Scope Scope `json:"scope"`
	// Scene is an optional local context label; nil if unset.
	Scene *string `json:"scene,omitempty"`
	// Workstream is the workstream slug; nil if unset. Required-ish for
	// ScopeWorkstream records but not enforced here (the store path already
	// pins the workstream by directory).
	Workstream *string `json:"workstream,omitempty"`
	// SourceRef is provenance: file path / command / checkpoint / handoff /
	// message ref; nil if unset.
	SourceRef *string `json:"source_ref,omitempty"`
	// SessionKey is the session alias/key; nil if unset.
	SessionKey *string `json:"session_key,omitempty"`
	// CreatedAt is the record creation time (RFC3339).
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is the record last-update time (RFC3339). Used as the
	// last-write-wins tiebreaker when an ID appears more than once.
	UpdatedAt time.Time `json:"updated_at"`
	// Body is the memory atom text.
	Body string `json:"body"`
}

// Validate checks that all required fields are present and that the enum
// fields carry known values. It returns nil for a record that is safe to
// append to a records.jsonl file.
func (r Record) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("record: id is required")
	}
	if !r.Type.Valid() {
		return fmt.Errorf("record: unknown type %q", r.Type)
	}
	if !r.Priority.Valid() {
		return fmt.Errorf("record: unknown priority %q", r.Priority)
	}
	if !r.Scope.Valid() {
		return fmt.Errorf("record: unknown scope %q", r.Scope)
	}
	if r.CreatedAt.IsZero() {
		return fmt.Errorf("record: created_at is required")
	}
	if r.UpdatedAt.IsZero() {
		return fmt.Errorf("record: updated_at is required")
	}
	if r.Body == "" {
		return fmt.Errorf("record: body is required")
	}
	return nil
}

// UnmarshalJSON decodes a Record from JSON while (a) ignoring unknown fields
// so the schema is forward-compatible with additive optional fields, and
// (b) rejecting unknown Type / Priority / Scope enum values. Unknown enum
// values are treated as data corruption rather than silently coerced.
//
// To customize decoding without infinite recursion, we decode into a
// plainRecord — a distinct named type with the same fields as Record but
// WITHOUT Record's methods. encoding/json therefore applies its default
// field-based decoding to plainRecord (which ignores unknown fields) and
// never re-enters this method. We then convert back to Record and validate.
func (r *Record) UnmarshalJSON(data []byte) error {
	var pr plainRecord
	if err := json.Unmarshal(data, &pr); err != nil {
		return err
	}
	if !pr.Type.Valid() {
		return fmt.Errorf("record: unknown type %q", pr.Type)
	}
	if !pr.Priority.Valid() {
		return fmt.Errorf("record: unknown priority %q", pr.Priority)
	}
	if !pr.Scope.Valid() {
		return fmt.Errorf("record: unknown scope %q", pr.Scope)
	}
	*r = Record(pr)
	return nil
}

// plainRecord is a distinct named type with the same underlying struct as
// Record but without Record's methods. It exists solely to let
// Record.UnmarshalJSON delegate to encoding/json's default decoder (which
// ignores unknown fields) without recursing. A type *definition*
// (`type plainRecord Record`), as opposed to a type *alias* (`type X = Record`),
// does not inherit Record's method set — that is the whole point.
type plainRecord Record
