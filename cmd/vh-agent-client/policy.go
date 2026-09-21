// policy.go — the P3 policy engine attached at the ApproverFunc seam
// (approval.go). policyApprover COMPOSES in front of an existing
// responder — delegation, not replacement: an unmatched call falls
// through to the interactive/--json approver exactly as today, so
// `--json --policy` works and humans still see real asks.
//
// One stderr line per policy decision:
//
//	policy: allow run_shell(command=git status)
//	policy: HARD-DENY edit(path=docs/x) (path traversal …)
//	policy: ask → human
//
// The policy approver NEVER reads stdin: allow/deny are answered
// synchronously from the rule file + hard-deny classes; only the ask
// path can block, and that block belongs to the delegate.
package main

import (
	"fmt"
	"io"
	"sync"

	"github.com/vhqtvn/vh-agent-harness/cmd/vh-agent-client/policy"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// policyApprover wraps next with the policy engine.
func policyApprover(pol *policy.Policy, next ApproverFunc, errw io.Writer) ApproverFunc {
	// Approvals answer on their own goroutines (driver.go); serialize
	// the diagnostic lines so concurrent decisions never interleave
	// mid-line. The lock guards ONLY the line render (round-4
	// C-F2/D-F2): the delegate below can block on a human answer for
	// as long as the human takes, and holding the mutex across that
	// call would convoy every concurrent policy decision behind the
	// human's latency — the delegate therefore runs OUTSIDE the lock.
	var mu sync.Mutex
	printLine := func(format string, a ...any) {
		mu.Lock()
		fmt.Fprintf(errw, format+"\n", a...)
		mu.Unlock()
	}
	return func(approvalID string, call session.ToolCall, reason string) ApprovalAnswer {
		d := pol.Decide(call.Name, call.Args)
		switch d.Kind {
		case policy.DecisionAllow:
			printLine("policy: allow %s", policyNote(call))
			return ApprovalAnswer{Allow: true, Reason: ""}
		case policy.DecisionDeny:
			printLine("policy: HARD-DENY %s (%s)", policyNote(call), compactOneLine(d.Reason, 80))
			return ApprovalAnswer{Allow: false, Reason: "policy hard-deny: " + d.Reason}
		default:
			printLine("policy: ask → human")
			return next(approvalID, call, reason)
		}
	}
}

// policyNote renders `tool(arg-hint)` for the decision lines (the
// argHint shape is the client's established steering-key hint).
func policyNote(call session.ToolCall) string {
	if hint := argHint(call.Args); hint != "" {
		return call.Name + "(" + hint + ")"
	}
	return call.Name
}
