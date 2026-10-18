// approval.go — the ProtocolApprover: a tools.Approver bridged over the
// host protocol. An ask becomes an approval/request notification; the
// answer arrives as an approval/respond request (or never).
//
// Fail-closed in EVERY unanswerable direction (dsh F-PIPE-2: "absent or
// unanswerable approval = deny"; ACP's one-shot permission bridge does
// the same — see
// researches/sources/deepseek-harness/llm-protocols-tools.md §1.1, §2.7):
//
//   - no answer within the configured timeout → DENY;
//   - connection closed while pending        → DENY (every pending ask);
//   - respond for an expired/unknown id      → refused (the tool call
//     has already been denied; approvals are one-shot, never re-opened).
package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// approvalRequestParams is the approval/request notification body.
type approvalRequestParams struct {
	ApprovalID string           `json:"approvalId"`
	Call       session.ToolCall `json:"call"`
	Reason     string           `json:"reason,omitempty"`
}

// approvalRespondParams is the approval/respond request body.
type approvalRespondParams struct {
	ApprovalID string `json:"approvalId"`
	Allow      bool   `json:"allow"`
	Reason     string `json:"reason,omitempty"`
}

// ProtocolApprover implements tools.Approver over one Conn.
type ProtocolApprover struct {
	conn    *Conn
	timeout time.Duration

	// after is the injectable timeout clock (tests drive it
	// deterministically); default time.After.
	after func(d time.Duration) <-chan time.Time

	mu      sync.Mutex
	pending map[string]chan tools.ApprovalDecision
	next    int64
}

// NewProtocolApprover bridges approvals onto conn. timeout of 0 waits
// indefinitely while the connection stays open (a disconnect still
// denies).
func NewProtocolApprover(conn *Conn, timeout time.Duration) *ProtocolApprover {
	a := &ProtocolApprover{
		conn:    conn,
		timeout: timeout,
		pending: make(map[string]chan tools.ApprovalDecision),
	}
	a.after = time.After
	return a
}

// Approve emits approval/request and blocks for the one-shot answer.
// See the file comment for the fail-closed semantics.
func (a *ProtocolApprover) Approve(ctx context.Context, call session.ToolCall, askReason string) tools.ApprovalDecision {
	if ctx == nil {
		ctx = context.Background()
	}
	a.mu.Lock()
	a.next++
	id := fmt.Sprintf("approval-%d", a.next)
	ch := make(chan tools.ApprovalDecision, 1)
	a.pending[id] = ch
	a.mu.Unlock()

	params, err := json.Marshal(approvalRequestParams{
		ApprovalID: id,
		Call:       call,
		Reason:     askReason,
	})
	if err != nil {
		a.unpend(id)
		return tools.ApprovalDecision{Allow: false, Reason: fmt.Sprintf("approval marshal failed: %v", err)}
	}
	if err := a.conn.Notify("approval/request", params); err != nil {
		a.unpend(id)
		return tools.ApprovalDecision{Allow: false, Reason: "approval/request could not be sent (connection lost)"}
	}

	var timeoutCh <-chan time.Time
	if a.timeout > 0 {
		timeoutCh = a.after(a.timeout)
	}
	select {
	case d := <-ch:
		return d
	case <-timeoutCh:
		a.unpend(id)
		return tools.ApprovalDecision{Allow: false, Reason: fmt.Sprintf("approval %s timed out after %s (denied)", id, a.timeout)}
	case <-a.conn.Done():
		a.unpend(id)
		return tools.ApprovalDecision{Allow: false, Reason: "connection closed while approval pending (denied)"}
	case <-ctx.Done():
		a.unpend(id)
		return tools.ApprovalDecision{Allow: false, Reason: fmt.Sprintf("approval canceled: %v (denied)", ctx.Err())}
	}
}

// Respond resolves one pending approval (the approval/respond method
// handler). Unknown, expired, or already-resolved ids error.
func (a *ProtocolApprover) Respond(approvalID string, allow bool, reason string) error {
	a.mu.Lock()
	ch, ok := a.pending[approvalID]
	if !ok {
		a.mu.Unlock()
		return fmt.Errorf("no pending approval %q (unknown, expired, or already answered)", approvalID)
	}
	delete(a.pending, approvalID)
	a.mu.Unlock()

	select {
	case ch <- tools.ApprovalDecision{Allow: allow, Reason: reason}:
		return nil
	default:
		return fmt.Errorf("approval %q is no longer waiting", approvalID)
	}
}

// unpend removes a pending entry without resolving it (timeout /
// disconnect / cancel paths — the caller already denies).
func (a *ProtocolApprover) unpend(id string) {
	a.mu.Lock()
	delete(a.pending, id)
	a.mu.Unlock()
}

// handleApprovalRespond is the approval/respond method handler.
func handleApprovalRespond(ctx context.Context, s *Server, req *Request) (json.RawMessage, *Error) {
	var p approvalRespondParams
	if perr := decodeParams(req, &p, false); perr != nil {
		return nil, perr
	}
	if p.ApprovalID == "" {
		return nil, &Error{Code: ErrInvalidParams, Message: "approvalId is required"}
	}
	if err := s.approver.Respond(p.ApprovalID, p.Allow, p.Reason); err != nil {
		return nil, &Error{Code: ErrUnknownApproval, Message: err.Error()}
	}
	result, merr := json.Marshal(struct {
		Resolved bool `json:"resolved"`
	}{true})
	if merr != nil {
		return nil, &Error{Code: ErrEngine, Message: merr.Error()}
	}
	return result, nil
}
