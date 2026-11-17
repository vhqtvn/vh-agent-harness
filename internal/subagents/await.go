// await.go — AwaitChild: the blocking settlement wait behind the
// model-facing one-shot subagent_spawn tool. The tool enqueues the
// spawn (job-style receipt) and then WAITS for that child's terminal
// state; this is the wait primitive. It observes the same events the
// wire surface sees (report precedes settlement, first-wins settle) —
// it never appends anything itself, so the durable shape of the parent
// log is unchanged by awaiting.
package subagents

import (
	"context"
	"fmt"
)

// AwaitResult is the terminal state of one awaited child.
type AwaitResult struct {
	ChildID string
	// Result is the settlement result (completed|failed).
	Result string
	// Reason carries the failure text on a failed settlement.
	Reason string
	// Report is the child's final assistant output — the SAME content
	// the subagent/report relay carried into the parent log (the
	// at-most-once report key is the child-side origin seq; this is a
	// re-derivation of that content, not a second relay). Empty when
	// the child produced no assistant output.
	Report string
}

// AwaitChild blocks until childID settles (or ctx is canceled) and
// returns its terminal state plus final report content. Cancelling the
// context abandons the WAIT ONLY — the spawn already happened and the
// child still runs and settles durably; only this caller stops
// observing. Unknown children and stopped managers without a settling
// disposition fail closed.
func (m *Manager) AwaitChild(ctx context.Context, childID string) (AwaitResult, error) {
	m.mu.Lock()
	h, ok := m.children[childID]
	if !ok {
		m.mu.Unlock()
		return AwaitResult{}, fmt.Errorf("subagents: await unknown child %q", childID)
	}
	if h.rec.settledSeq == 0 {
		ch := make(chan struct{})
		h.settleWaiters = append(h.settleWaiters, ch)
		m.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			return AwaitResult{}, fmt.Errorf("subagents: await %s: %w", childID, ctx.Err())
		}
		m.mu.Lock()
		h = m.children[childID] // re-lookup: cheap and stable under mu
	}
	res := AwaitResult{
		ChildID: h.rec.childID,
		Result:  h.rec.settleResult,
		Reason:  h.rec.settleReason,
	}
	lg := h.lg
	m.mu.Unlock()

	// The report content is re-derived from the child log WITHOUT the
	// manager lock (the child log's own lock is innermost everywhere
	// else; keeping it innermost here too preserves the one-direction
	// lock order).
	if content, _, ok := finalAssistant(lg); ok {
		res.Report = content
	}
	return res, nil
}
