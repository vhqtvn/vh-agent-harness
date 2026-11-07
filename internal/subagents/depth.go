// depth.go — the delegation-depth fence and the header cross-check.
//
// dsh ground truth (session-cognition.md §subagent/): delegation depth
// is DURABLE — it lives in the persisted SessionHeader.delegationDepth,
// cold resume trusts it as the monotone floor, and starts reject
// out-of-domain depth or depth > maxDepth. Our port: the depth fence is
// enforced fail-closed at spawn (an error, never a silent truncate),
// and the CHILD's persisted header is authoritative — at manager
// (re)construction each child header is cross-checked against the
// parent's spawned record; any mismatch refuses reconstruction.
package subagents

import (
	"fmt"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// EffectiveMaxDepth resolves a configured depth cap the way the Manager
// does (<= 0 ⇒ DefaultMaxDelegationDepth). Shared by the manager and
// the depth-conditional tool advertising so both sides fence at the
// same line.
func EffectiveMaxDepth(configured int) int {
	if configured <= 0 {
		return DefaultMaxDelegationDepth
	}
	return configured
}

// crossCheckChildHeader validates the authoritative child header (first
// event of the child log) against the parent's spawned record: the child
// log must carry its own session id, the exact direct parent session id,
// and the exact recorded delegation depth. A mismatch is fail-closed —
// it refuses the fold rather than trusting either side silently.
func crossCheckChildHeader(lg *session.Log, childID, parentSessionID string, wantDepth int) error {
	events := lg.Events()
	if len(events) == 0 || events[0].Type != session.TypeSessionHeader {
		return fmt.Errorf("subagents: child %q log has no session header", childID)
	}
	var hp session.HeaderPayload
	if err := unmarshal(events[0].Payload, &hp); err != nil {
		return fmt.Errorf("subagents: malformed child %q header: %w", childID, err)
	}
	if hp.SessionID != childID {
		return fmt.Errorf("subagents: child header id mismatch: log claims %q, parent record says %q", hp.SessionID, childID)
	}
	if hp.ParentSessionID != parentSessionID {
		return fmt.Errorf("subagents: child %q header parent mismatch: claims %q, actual parent %q", childID, hp.ParentSessionID, parentSessionID)
	}
	if hp.DelegationDepth != wantDepth {
		return fmt.Errorf("subagents: child %q delegation depth mismatch: header (authoritative) says %d, parent spawned record says %d — refusing reconstruction",
			childID, hp.DelegationDepth, wantDepth)
	}
	return nil
}
