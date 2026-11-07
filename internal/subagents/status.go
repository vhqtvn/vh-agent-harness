// status.go — the FOLD-DERIVED wire snapshot of a parent session's
// subagent activations (the subagent/list seam). Like jobs/status it is
// a pure fold over the parent log's subagent/* events — never live
// runtime state — so the wire answer and a cold replay of the same log
// agree by construction. State vocabulary mirrors the fold (fold.go):
// running (one-shot, not settled), waiting (continuable, not settled),
// settled (a settlement notice landed).
package subagents

import (
	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// Status is one child's fold-derived, wire-shaped snapshot. ContentSeq
// is the highest child-side origin seq already relayed by a
// subagent/report (0 = nothing reported yet) — the durable at-most-once
// report key surfaced for observability.
type Status struct {
	ChildID       string `json:"childId"`
	Kind          string `json:"kind"`
	Prompt        string `json:"prompt,omitempty"`
	Depth         int    `json:"depth,omitempty"`
	State         string `json:"state"`
	SettledResult string `json:"result,omitempty"`
	SettledReason string `json:"reason,omitempty"`
	ContentSeq    int64  `json:"contentSeq,omitempty"`
}

// FoldStatus rebuilds the wire-shaped snapshot from a parent event
// list, in spawn order (FoldSubagents) enriched with the per-child
// reported contentSeq (foldReportedContentSeqs). Malformed subagent
// events fail loud, exactly like the descriptor fold.
func FoldStatus(events []session.Event) ([]Status, error) {
	ds, err := FoldSubagents(events)
	if err != nil {
		return nil, err
	}
	reported := foldReportedContentSeqs(events)
	out := make([]Status, 0, len(ds))
	for _, d := range ds {
		out = append(out, Status{
			ChildID:       d.ChildID,
			Kind:          d.Kind,
			Prompt:        d.Prompt,
			Depth:         d.Depth,
			State:         d.State,
			SettledResult: d.SettledResult,
			SettledReason: d.SettledReason,
			ContentSeq:    reported[d.ChildID],
		})
	}
	return out, nil
}
