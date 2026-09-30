// verdict.go — the slice-3 decision lattice around tool execution:
// pre-execute observers returning typed verdicts, the fail-closed Approver
// seam for ask verdicts, the mutation-limited PostResult handed to
// post-execute observers, and the injected Clock for the dispatch timeout.
//
// Reference semantics: researches/sources/deepseek-harness/
// llm-protocols-tools.md (F-PIPE-1/2, F-EXT-1) and kernel-architecture.md
// (scheduler + waterfall sections).

package tools

import (
	"context"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// VerdictKind is the closed decision vocabulary of one pre-execute
// observer. The lattice aggregates as deny > ask > allow: ANY deny denies;
// an ask stands unless a DOWNSTREAM allow resolves it; a lone unresolved
// ask goes to the Approver and fails closed when none is configured.
type VerdictKind string

const (
	VerdictAllow VerdictKind = "allow"
	VerdictDeny  VerdictKind = "deny"
	VerdictAsk   VerdictKind = "ask"
)

// Verdict is the typed decision one pre-execute observer returns. It is a
// verdict-only value: there is no field through which the call, its args,
// or the eventual result could be modified.
type Verdict struct {
	Kind   VerdictKind
	Reason string // justification recorded in denial provenance
}

// Allow returns the allowing verdict.
func Allow() Verdict { return Verdict{Kind: VerdictAllow} }

// Deny returns the denying verdict carrying reason.
func Deny(reason string) Verdict { return Verdict{Kind: VerdictDeny, Reason: reason} }

// Ask returns the approval-seeking verdict carrying reason.
func Ask(reason string) Verdict { return Verdict{Kind: VerdictAsk, Reason: reason} }

// PreExecuteObserver is one policy observer in the ordered pre-execute
// waterfall. It receives the call BY VALUE with detached args and returns
// a verdict — the interface exposes no channel through which to modify
// the call (compile-level cannot-modify contract).
type PreExecuteObserver interface {
	Name() string
	ObservePreExecute(call session.ToolCall) Verdict
}

// ApprovalDecision is the Approver's one-shot answer to an ask verdict.
type ApprovalDecision struct {
	Allow  bool
	Reason string
}

// Approver resolves ask verdicts the waterfall could not resolve itself.
// Slice 3 ships the interface (tests inject doubles); the real approval
// transport arrives with the host-protocol slice. An absent (nil)
// Approver makes every unresolved ask DENY — fail-closed, per dsh
// F-PIPE-2 ("absent or unanswerable approval = deny").
type Approver interface {
	Approve(ctx context.Context, call session.ToolCall, askReason string) ApprovalDecision
}

// PostResult is the mutation-limited view of a Result handed to
// post-execute observers. Observers may read the whole outcome (View) and
// may REPLACE the content (ReplaceContent — derived content, provenance
// tagged with the observer's identity), but there is NO exported field or
// method that reaches IsError, Denied, TimedOut, CallID or Name: the
// isError fact is unflippable at the type level, and the canonical logged
// call is untouchable from post-observation.
type PostResult struct {
	res      *Result
	observer string
}

// View returns a copy of the observed result.
func (p *PostResult) View() Result { return *p.res }

// ReplaceContent replaces the canonical content with derived content and
// stamps the observer's identity as replace provenance (ReplacedBy).
func (p *PostResult) ReplaceContent(content string) {
	p.res.Content = content
	p.res.ReplacedBy = p.observer
}

// PostExecuteObserver observes the outcome of an EXECUTED tool call and
// may accept it (no call) or replace its content. Post-observe runs even
// on error results; it does not run for denials or unknown tools (those
// never dispatch, so there is no executed outcome to observe).
type PostExecuteObserver interface {
	Name() string
	ObservePostExecute(call session.ToolCall, res *PostResult)
}

// Clock is the injected timing seam for the execute dispatch: After
// returns a channel that fires once when d elapses. Tests inject a
// controllable clock so timeout classification is asserted
// deterministically without racing real sleeps against the select.
type Clock interface {
	After(d time.Duration) <-chan time.Time
}

// RealClock is the production clock backed by time.After.
type RealClock struct{}

// After returns a channel firing after d.
func (RealClock) After(d time.Duration) <-chan time.Time { return time.After(d) }
