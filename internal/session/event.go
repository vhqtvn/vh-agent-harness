// Package session implements the native engine's append-only event-log
// kernel: a JSONL session log with monotonic sequence numbers, typed
// events, and a derived (never stored) LLM-visible message surface.
//
// Slice-1 invariants:
//   - every log starts with a session/header record (format version 0);
//   - replay is fail-closed: an unknown event type without ignorable=true
//     refuses reconstruction instead of being silently skipped;
//   - the LLM message surface is a pure projection of the log
//     (DeriveMessages); message-bearing events carry a surfaceOp
//     (append, or replace over a positional span for future compaction).
package session

import (
	"encoding/json"
	"time"
)

// SESSION_FORMAT_VERSION is the session log format version written in the
// session/header record. Version 0 is the slice-1 kernel format.
const SESSION_FORMAT_VERSION = 0

// Slice-1 event types (closed set). The fail-closed replay reader rejects
// unknown types unless the record carries ignorable:true (forward
// compatibility: ignorable unknown events are log-only and contribute
// nothing to the derived surface).
const (
	TypeSessionHeader = "session/header"
	TypeSessionPrompt = "session/prompt"
	TypeLLMRequest    = "llm/request"
	TypeLLMResponse   = "llm/response"
	TypeToolCall      = "tool/call"
	TypeToolResult    = "tool/result"
	TypeTurnBegin     = "turn/begin"
	TypeTurnEnd       = "turn/end"
)

// Slice-2 event types. compaction/start and compaction/end are log-only
// brackets around a compaction (an unmatched start is the durable lock a
// crash or refused summary leaves behind); compaction/summary is the ONE
// message-bearing compaction event — it rides the user-role summary message
// carrying the replace surfaceOp plus the unfold citations. llm/retry and
// llm/retry-started are log-only records the retry loop writes around each
// backoff wait, so replay shows numbered attempts.
const (
	TypeCompactionStart   = "compaction/start"
	TypeCompactionSummary = "compaction/summary"
	TypeCompactionEnd     = "compaction/end"
	TypeLLMRetry          = "llm/retry"
	TypeLLMRetryStarted   = "llm/retry-started"
)

// Slice-4 event types — the async-jobs family (dsh jobs subsystem
// semantics, see researches/sources/deepseek-harness/session-cognition.md
// §jobs/). job/enqueued, job/started, and job/settled are LOG-ONLY: job
// state rides the same durable stream as messages without polluting the
// model surface (the log-only plugin-event discipline). job/report is the
// ONE message-bearing job event — the settled-job notice delivered to the
// model exactly once (dsh `reported` flag as model-cost guard).
const (
	TypeJobEnqueued = "job/enqueued"
	TypeJobStarted  = "job/started"
	TypeJobSettled  = "job/settled"
	TypeJobReport   = "job/report"
)

// Job settlement results (job/settled Result field).
const (
	JobResultCompleted = "completed"
	JobResultFailed    = "failed"
)

// Slice-8 event types — the subagent family (dsh subagent subsystem
// semantics, see researches/sources/deepseek-harness/session-cognition.md
// §subagent/, which transcribes docs/subsystems/subagent.md). This slice
// implements the PARENT-CHILD TOPOLOGY ONLY (risk R10: any-to-any
// session-bus generalization is explicitly deferred):
//
//   - subagent/spawned (PARENT log, log-only): the durable spawn
//     descriptor — child id, kind (one-shot|continuable), prompt, and the
//     child's delegation depth. Like the dsh descriptor it is log-only:
//     identity rides the durable stream without polluting the model
//     surface.
//   - subagent/report (PARENT log, message-bearing): the child→parent
//     report relay. It enters the parent surface as a USER-role context
//     event, provenance-tagged subagent-report — NEVER assistant, so the
//     parent transcript never credits the child with runtime words it
//     did not say (dsh provenance-clean reporting).
//   - subagent/settled (PARENT log, log-only): the MANAGER-authored
//     settlement notice, provenance-tagged subagent-settled — a distinct
//     kind from the report (dsh separates them deliberately: only the
//     manager controls settlement, and the notice costs no surface).
//   - subagent/message (CHILD log, message-bearing): a parent→child
//     follow-up that lands in the child log as an addressed user-role
//     inbox message — the Agent inbox is the only FIFO (delivery-order
//     authority).
//
// SESSION_FORMAT_VERSION stays 0: the additions are strictly additive
// within version 0. Payload fields are omitempty (slice-4 discipline), so
// pre-slice-8 logs replay byte-stably, and forward compatibility is
// already governed by the fail-closed unknown-event reader — an old
// binary refuses reconstruction of these types rather than silently
// skipping them, while this code knows them.
const (
	TypeSubagentSpawned = "subagent/spawned"
	TypeSubagentReport  = "subagent/report"
	TypeSubagentSettled = "subagent/settled"
	TypeSubagentMessage = "subagent/message"
)

// Subagent kinds (subagent/spawned Kind field).
const (
	SubagentKindOneShot     = "one-shot"
	SubagentKindContinuable = "continuable"
)

// Subagent provenance kinds (subagent/report and subagent/settled
// Provenance field). They are deliberately distinct (dsh continuation
// manager): subagent-report is the child-words relay, subagent-settled is
// the manager's settlement notice.
const (
	SubagentProvenanceReport  = "subagent-report"
	SubagentProvenanceSettled = "subagent-settled"
)

// Surface operation kinds. Positions are indices into the derived message
// surface (NOT seq intervals): a replace shadows the span [Start, End).
const (
	SurfaceOpAppend  = "append"
	SurfaceOpReplace = "replace"
)

// SurfaceOp is the surface-model marker carried by message-bearing events.
// {op: append} grows the surface; {op: replace, start, end} replaces the
// cited positional span with the event's own message (compaction shape).
type SurfaceOp struct {
	Op    string `json:"op"`
	Start int    `json:"start,omitempty"`
	End   int    `json:"end,omitempty"`
}

// Event is one JSONL record in the session log. Payload holds the typed
// payload for the event's type, marshaled deterministically.
type Event struct {
	Seq       int64           `json:"seq"`
	Type      string          `json:"type"`
	Ignorable bool            `json:"ignorable,omitempty"`
	SurfaceOp *SurfaceOp      `json:"surfaceOp,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

// knownTypes is the closed event-type set consulted by both the writer and
// the fail-closed replay reader.
var knownTypes = map[string]bool{
	TypeSessionHeader:     true,
	TypeSessionPrompt:     true,
	TypeLLMRequest:        true,
	TypeLLMResponse:       true,
	TypeToolCall:          true,
	TypeToolResult:        true,
	TypeTurnBegin:         true,
	TypeTurnEnd:           true,
	TypeCompactionStart:   true,
	TypeCompactionSummary: true,
	TypeCompactionEnd:     true,
	TypeLLMRetry:          true,
	TypeLLMRetryStarted:   true,
	TypeJobEnqueued:       true,
	TypeJobStarted:        true,
	TypeJobSettled:        true,
	TypeJobReport:         true,
	TypeSubagentSpawned:   true,
	TypeSubagentReport:    true,
	TypeSubagentSettled:   true,
	TypeSubagentMessage:   true,
}

// messageBearing reports whether an event type contributes a message to
// the derived surface (and therefore must carry a surfaceOp).
func messageBearing(eventType string) bool {
	switch eventType {
	case TypeSessionPrompt, TypeLLMResponse, TypeToolResult, TypeCompactionSummary, TypeJobReport,
		TypeSubagentReport, TypeSubagentMessage:
		return true
	}
	return false
}

// HeaderPayload is the payload of the mandatory first session/header record.
// The slice-8 fields are the parent-child topology of a subagent CHILD
// session (dsh SessionHeader.parentSessionId/delegationDepth — persisted
// so recursion budgets survive resume; the role/persona hint is optional).
// All omitempty: a root header (no parent) marshals byte-identically to
// the pre-slice-8 shape, so old logs stay byte-stable.
type HeaderPayload struct {
	SessionID     string    `json:"sessionId"`
	FormatVersion int       `json:"formatVersion"`
	CreatedAt     time.Time `json:"createdAt"`
	// ParentSessionID is the session id of the direct parent for a
	// subagent child session; empty for a root session.
	ParentSessionID string `json:"parentSessionId,omitempty"`
	// DelegationDepth is this session's depth in the delegation tree
	// (root = 0, each child = parent+1). Persisted in the header (the
	// authoritative record — dsh: cold resume trusts the persisted
	// delegationDepth as the monotone floor; a descriptor deliberately
	// omits it).
	DelegationDepth int `json:"delegationDepth,omitempty"`
	// Role is an optional role/persona hint for the child.
	Role string `json:"role,omitempty"`
}

// PromptPayload is the payload of session/prompt — the typed user-input
// ingress. Because user input enters only through this typed record,
// model output can never enter the surface with the user role.
type PromptPayload struct {
	Text string `json:"text"`
}

// LLMRequestPayload is the audit record of one outgoing chat-completions
// request (log-only; the message surface itself is derived, not stored).
type LLMRequestPayload struct {
	Model       string   `json:"model"`
	Tools       []string `json:"tools,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
	MaxTokens   int      `json:"maxTokens,omitempty"`
}

// Usage is the token-usage envelope reported by the provider.
type Usage struct {
	PromptTokens     int `json:"promptTokens"`
	CompletionTokens int `json:"completionTokens"`
	TotalTokens      int `json:"totalTokens"`
}

// ToolCall is one model-requested tool invocation (assistant message) and,
// with an Args-only reading, the logged pre-execution intent.
type ToolCall struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

// LLMResponsePayload is the payload of llm/response: the assistant
// message (content + tool calls) plus the provider usage envelope.
type LLMResponsePayload struct {
	Model     string     `json:"model"`
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"toolCalls,omitempty"`
	Usage     Usage      `json:"usage"`
}

// ToolCallPayload is the payload of tool/call, logged PRE-execution so
// the intent is durable even if execution never happens.
type ToolCallPayload struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

// ToolResultPayload is the payload of tool/result — the frozen canonical
// result of one tool execution, with isError set on failure or denial.
// The slice-3 fields are typed outcome metadata carried alongside the
// canonical content (all omitempty: slice-1 logs replay byte-identically):
//   - Denied/DeniedBy/DenyReason — the denial marker: a pre-execution
//     waterfall or guard veto. A denied result is always isError too.
//   - TimedOut — the ORTHOGONAL timeout cause fact (dsh defensive
//     pattern: timedOut is never conflated with the error text; a result
//     carries it only when the dispatch deadline fired).
//   - ReplacedBy — replace provenance: the post-execute observer whose
//     derived content replaced the raw body output.
//
// The spill fields (dsh spill-policy, additive + omitempty so pre-spill
// logs replay byte-identically) mark that Content is a bounded PREVIEW:
// the full output was written to the session's spill store and is
// retrievable via the spill_read tool with SpillLocator. The preview and
// notice are PART of Content (the model-visible surface), so replay and
// DeriveMessages never touch the spill files.
type ToolResultPayload struct {
	CallID     string `json:"callId"`
	Name       string `json:"name"`
	Content    string `json:"content"`
	IsError    bool   `json:"isError"`
	Denied     bool   `json:"denied,omitempty"`
	DeniedBy   string `json:"deniedBy,omitempty"`
	DenyReason string `json:"denyReason,omitempty"`
	TimedOut   bool   `json:"timedOut,omitempty"`
	ReplacedBy string `json:"replacedBy,omitempty"`
	// Spilled is true when Content is a preview of a spilled result.
	Spilled bool `json:"spilled,omitempty"`
	// SpillLocator is the opaque locator for paging the spilled bytes
	// back via spill_read windows (set iff Spilled).
	SpillLocator *SpillLocator `json:"spillLocator,omitempty"`
}

// TurnEndPayload optionally carries why the turn ended. Kind classifies the
// closure: "" (or "ok") for a completed turn, "error" when the turn failed
// (retry exhaustion or a non-retryable adapter error); a future synthetic
// "interrupted" closure is reserved for crash recovery.
type TurnEndPayload struct {
	Kind   string `json:"kind,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// CompactionStartPayload is the payload of the log-only compaction/start
// bracket — the durable lock. An unmatched start (no summary/end following)
// marks an interrupted or refused compaction; the fold skips it, so it costs
// no surface, but inspection can always see it.
type CompactionStartPayload struct {
	Reason        string  `json:"reason,omitempty"`
	Pressure      float64 `json:"pressure,omitempty"`
	ShadowedRange [2]int  `json:"shadowedRange"` // intended [start,end) positions
}

// CompactionSummaryPayload rides the user-role summary message. Start/End are
// POSITIONS in the derived surface (not seq intervals); SourceEventSeqs cites
// every shadowed event's seq so the pre-compaction surface stays unfoldable.
type CompactionSummaryPayload struct {
	Text              string  `json:"text"`
	SourceEventSeqs   []int64 `json:"sourceEventSeqs"`
	ShadowedRange     [2]int  `json:"shadowedRange"`
	ReplaceGeneration int     `json:"replaceGeneration"`
}

// CompactionEndPayload closes the compaction bracket.
type CompactionEndPayload struct {
	SummarySeq        int64 `json:"summarySeq"`
	ReplaceGeneration int   `json:"replaceGeneration"`
}

// JobPayload is the shared payload shape of the job/* family. Fields are
// omitempty (slice-4 omitempty discipline, same as the slice-3 tool-result
// metadata) so pre-slice-4 logs replay byte-stably and each event carries
// only the fields meaningful for its type:
//
//   - job/enqueued: JobID, Kind (the job kind, e.g. "background"), Owner
//     (the fencing session id), Payload (the dispatched work item);
//   - job/started: JobID, Kind, Owner;
//   - job/settled: JobID, Kind, Owner, Result (completed|failed), Reason
//     (failure text, omitempty);
//   - job/report: JobID, Kind, Owner, Result, Reason — the notice the
//     surface derives its message from.
//
// dsh pattern (session-cognition.md §jobs/, from docs/subsystems/jobs.md):
// `<kind>-N` ids are per-kind monotonic; owner fencing "is authorization,
// not secrecy — the id proves ownership" (the Owner field records who may
// act on the job; it fences authorization, it is not a secrecy mechanism);
// settlement is first-wins with exactly one terminal event; the reported
// flag suppresses duplicate completion notices to the model.
type JobPayload struct {
	JobID   string          `json:"jobId"`
	Kind    string          `json:"kind,omitempty"`
	Owner   string          `json:"owner,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Result  string          `json:"result,omitempty"`
	Reason  string          `json:"reason,omitempty"`
}

// SubagentPayload is the shared payload shape of the subagent/* family.
// Fields are omitempty (slice-4 discipline) so each event carries only
// the fields meaningful for its type and pre-slice-8 logs replay
// byte-stably:
//
//   - subagent/spawned: ChildID, Kind (one-shot|continuable), Prompt
//     (the dispatch prompt or its durable reference), Depth (the child's
//     delegation depth, cross-checkable against the child header),
//     SeedTurns (B2 fork seeding: the number of COMPLETED parent turns
//     whose surface messages were copied into the child log before its
//     first turn — omitempty, so pre-B2 spawned records stay
//     byte-identical);
//   - subagent/report: ChildID, Kind, Content (the child's report — its
//     final assistant output relayed by the manager), ContentSeq (the
//     child-side origin seq of that output — the durable at-most-once
//     report key), Provenance (subagent-report);
//   - subagent/settled: ChildID, Kind, Result (completed|failed), Reason
//     (failure text, omitempty), Provenance (subagent-settled);
//   - subagent/message: ChildID, From (the sender — the parent session
//     id), Text (the follow-up body; the initial spawn prompt arrives as
//     the first inbox message).
type SubagentPayload struct {
	ChildID    string `json:"childId"`
	Kind       string `json:"kind,omitempty"`
	Prompt     string `json:"prompt,omitempty"`
	Depth      int    `json:"depth,omitempty"`
	SeedTurns  int    `json:"seedTurns,omitempty"`
	Content    string `json:"content,omitempty"`
	ContentSeq int64  `json:"contentSeq,omitempty"`
	From       string `json:"from,omitempty"`
	Text       string `json:"text,omitempty"`
	Result     string `json:"result,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Provenance string `json:"provenance,omitempty"`
}
