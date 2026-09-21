// fold.go — the deterministic descriptor fold over a PARENT event list
// (dsh subagent identity fold). Last-wins by child id: our writer mints
// one spawned descriptor per id, but the fold tolerates and overrides
// duplicates the way dsh's descriptor fold does (the child's own
// descriptor overrides a fork-seeded ancestor's).
//
// Derived states (dsh: activation state is DERIVED, never stored):
//   - settled: a subagent/settled notice landed;
//   - waiting: continuable and not settled (the log-static projection of
//     "quiescent": durable evidence shows no settlement; live run-in-flight
//     detection is runtime state the Manager's Snapshot refines);
//   - running: one-shot and not settled (a one-shot runs until it settles).
package subagents

import (
	"fmt"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// Derived activation states.
const (
	StateRunning = "running"
	StateWaiting = "waiting"
	StateSettled = "settled"
)

// Descriptor is one child's derived identity + state from the parent-log
// fold. The unexported seq fields are fold bookkeeping used by Manager
// reconstruction.
type Descriptor struct {
	ChildID       string
	Kind          string
	Prompt        string
	Depth         int
	State         string
	SettledResult string
	SettledReason string

	spawnedSeq int64
	settledSeq int64
}

// SettledState reports whether the fold saw a settlement notice.
func (d *Descriptor) SettledState() bool { return d.settledSeq != 0 }

// FoldSubagents rebuilds every child descriptor from a parent event
// list, in spawn order, last-wins by child id. It enforces the writer
// invariants fail-loud (a report or settled event for an unknown child,
// or a malformed payload, is a writer bug, not a fold input).
func FoldSubagents(events []session.Event) ([]Descriptor, error) {
	index := map[string]int{}
	var out []Descriptor
	for i := range events {
		ev := events[i]
		switch ev.Type {
		case session.TypeSubagentSpawned:
			var p session.SubagentPayload
			if err := unmarshal(ev.Payload, &p); err != nil {
				return nil, fmt.Errorf("subagents: malformed subagent/spawned payload at seq %d: %w", ev.Seq, err)
			}
			if p.ChildID == "" {
				return nil, fmt.Errorf("subagents: subagent/spawned at seq %d has no childId", ev.Seq)
			}
			d := Descriptor{
				ChildID: p.ChildID, Kind: p.Kind, Prompt: p.Prompt, Depth: p.Depth,
				spawnedSeq: ev.Seq,
			}
			if j, ok := index[p.ChildID]; ok {
				out[j] = d // last-wins (dsh descriptor fold)
			} else {
				index[p.ChildID] = len(out)
				out = append(out, d)
			}
		case session.TypeSubagentReport:
			var p session.SubagentPayload
			if err := unmarshal(ev.Payload, &p); err != nil {
				return nil, fmt.Errorf("subagents: malformed subagent/report payload at seq %d: %w", ev.Seq, err)
			}
			j, ok := index[p.ChildID]
			if !ok {
				return nil, fmt.Errorf("subagents: subagent/report at seq %d for unknown child %q", ev.Seq, p.ChildID)
			}
			_ = j // report carries no state beyond provenance (contentSeq is foldReportedContentSeqs' concern)
		case session.TypeSubagentSettled:
			var p session.SubagentPayload
			if err := unmarshal(ev.Payload, &p); err != nil {
				return nil, fmt.Errorf("subagents: malformed subagent/settled payload at seq %d: %w", ev.Seq, err)
			}
			j, ok := index[p.ChildID]
			if !ok {
				return nil, fmt.Errorf("subagents: subagent/settled at seq %d for unknown child %q", ev.Seq, p.ChildID)
			}
			if out[j].settledSeq != 0 {
				return nil, fmt.Errorf("subagents: duplicate subagent/settled for %q at seq %d (first-wins violated by the writer)", p.ChildID, ev.Seq)
			}
			out[j].settledSeq = ev.Seq
			out[j].SettledResult = p.Result
			out[j].SettledReason = p.Reason
		}
	}
	for i := range out {
		switch {
		case out[i].settledSeq != 0:
			out[i].State = StateSettled
		case out[i].Kind == session.SubagentKindContinuable:
			out[i].State = StateWaiting
		default:
			out[i].State = StateRunning
		}
	}
	return out, nil
}

// foldReportedContentSeqs rebuilds, per child, the highest child-side
// origin seq already relayed by a subagent/report — the at-most-once
// report key persisted as contentSeq so a reconstructed manager cannot
// double-report pre-crash output.
func foldReportedContentSeqs(events []session.Event) map[string]int64 {
	reported := make(map[string]int64)
	for i := range events {
		ev := events[i]
		if ev.Type != session.TypeSubagentReport {
			continue
		}
		var p session.SubagentPayload
		if err := unmarshal(ev.Payload, &p); err != nil {
			continue // FoldSubagents reports malformed payloads fail-loud
		}
		if p.ContentSeq > reported[p.ChildID] {
			reported[p.ChildID] = p.ContentSeq
		}
	}
	return reported
}
