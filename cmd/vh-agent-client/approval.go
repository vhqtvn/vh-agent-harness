// approval.go — the client's approval answering: the ApproverFunc seam
// and the two shipped responders (interactive y/N and --json line
// reader), both backed by the stdinHub (input.go — the single stdin
// owner; hotfix b-F1: the responders used to read the shared
// bufio.Reader directly, racing under concurrent approvals).
//
// Fail-closed in every unanswerable direction, mirroring the daemon's
// approval bridge: EOF, ENTER alone, malformed input, or anything that
// is not an explicit y/yes DENIES. The P3 policy engine (policy.go)
// COMPOSES at this seam — in front of whichever responder the mode
// chose — delegating unmatched asks to it; the driver is untouched.
package main

import (
	"fmt"
	"io"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// ApprovalAnswer is the responder's one-shot answer.
type ApprovalAnswer struct {
	Allow  bool
	Reason string
}

// ApproverFunc answers one approval/request. It is the policy seam:
// the shipped implementations are interactiveApprover (human mode) and
// jsonApprover (--json); the P3 policy engine (policy.go) wraps the
// chosen one via policyApprover when --policy is given, without
// changing the driver.
type ApproverFunc func(approvalID string, call session.ToolCall, reason string) ApprovalAnswer

// denyAll is the fail-closed responder used when no responder applies
// (defensive wiring default; never reached in the shipped wiring).
func denyAll(approvalID string, call session.ToolCall, reason string) ApprovalAnswer {
	return ApprovalAnswer{Allow: false, Reason: "no approver configured (fail-closed deny)"}
}

// interactiveApprover returns the human responder: a blocking [y/N]
// prompt on errw, one line routed back by the hub (FIFO — prompt order
// is answer order). Default (ENTER alone) and EOF deny — fail-closed.
func interactiveApprover(hub *stdinHub, errw io.Writer) ApproverFunc {
	return func(approvalID string, call session.ToolCall, reason string) ApprovalAnswer {
		hint := argHint(call.Args)
		notice := func() {
			if hint != "" {
				fmt.Fprintf(errw, "[y/N] approve tool %s (%s)? %s", call.Name, hint, approvalPromptSuffix(reason))
			} else {
				fmt.Fprintf(errw, "[y/N] approve tool %s? %s", call.Name, approvalPromptSuffix(reason))
			}
		}
		// askFIFO prints the notice under the registration lock: two
		// concurrently-pending approvals prompt in a strict order, and
		// each typed line answers the approval that prompted it.
		return hub.askFIFO(approvalID, notice)
	}
}

// approvalPromptSuffix renders the ask reason as a prompt suffix.
func approvalPromptSuffix(reason string) string {
	if reason == "" {
		return ""
	}
	return "[" + compactOneLine(reason, 60) + "] "
}

// jsonApprover returns the --json responder: the request itself was
// already emitted verbatim by the JSON renderer; the answer arrives as
// one JSON line on stdin, {"id":"<approvalId>","approve":bool}, routed
// STRICTLY by approvalId by the hub. An answer for a different,
// already-settled, or unknown id never applies to this approval; EOF
// denies.
func jsonApprover(hub *stdinHub) ApproverFunc {
	return func(approvalID string, call session.ToolCall, reason string) ApprovalAnswer {
		return hub.askByID(approvalID)
	}
}
