// seed.go — fork turn-prefix seeding (the dsh fork pattern): a spawn may
// copy the parent's last-n COMPLETED turns' surface messages into the
// child log as seed events, BEFORE the child's first turn, so children
// inherit balanced context instead of a cold start.
//
// Replay-deterministic by construction: seeding reuses the EXISTING
// message-bearing event vocabulary — exactly {session/prompt,
// llm/response, tool/result} — re-appended through the ordinary Log
// append path with verbatim payloads and append surfaceOps. No new
// event types are introduced; a seeded child log replays under the same
// fail-closed reader as any other log, and re-seeding the same parent
// state produces the same child bytes.
//
// Turn selection: a COMPLETED turn is a turn/begin…turn/end bracket
// whose turn/end carries kind "" or "ok" (writer.go's completed-turn
// vocabulary); brackets closed kind:"error" (retry exhaustion, adapter
// failure) are NOT seed material — a child must not inherit a failed
// exchange as balanced context. Only the last n completed brackets are
// seeded; fewer available ⇒ fewer seeded (never an error). Log-only
// events inside the brackets (llm/request, tool/call, the brackets
// themselves) are not copied: seeds carry surface messages only.
package subagents

import (
	"encoding/json"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// seedableTypes is the closed seed vocabulary: the message-bearing event
// kinds a completed parent turn may contribute. Documented in the
// package doc above and in docs/native-engine/host-protocol.md.
var seedableTypes = map[string]bool{
	session.TypeSessionPrompt: true,
	session.TypeLLMResponse:   true,
	session.TypeToolResult:    true,
}

// appendSeedTurns copies the surface messages of the parent's last n
// completed turns into clg (in log order, oldest seeded turn first) and
// returns the number of TURNS seeded (≤ n; 0 when the parent has no
// completed turns). Each copied event keeps its type and payload bytes
// verbatim and lands with an append surfaceOp.
func appendSeedTurns(clg *session.Log, parentEvents []session.Event, n int) (int, error) {
	spans := completedTurnSpans(parentEvents, n)
	for _, sp := range spans {
		for _, ev := range parentEvents[sp[0]:sp[1]] {
			if !seedableTypes[ev.Type] {
				continue
			}
			if _, err := clg.Append(ev.Type, &session.SurfaceOp{Op: session.SurfaceOpAppend}, ev.Payload); err != nil {
				return 0, err
			}
		}
	}
	return len(spans), nil
}

// completedTurnSpans returns the [start, end) event-index spans of the
// last n COMPLETED turn brackets (end inclusive of the turn/end). An
// unterminated bracket (crash-torn turn) is not completed and yields
// nothing.
func completedTurnSpans(events []session.Event, n int) [][2]int {
	var spans [][2]int
	start := -1
	for i := range events {
		switch events[i].Type {
		case session.TypeTurnBegin:
			start = i
		case session.TypeTurnEnd:
			if start < 0 {
				continue
			}
			if turnEndCompleted(events[i]) {
				spans = append(spans, [2]int{start, i + 1})
			}
			start = -1
		}
	}
	if len(spans) > n {
		return spans[len(spans)-n:]
	}
	return spans
}

// turnEndCompleted reports whether one turn/end record closes a
// completed turn (kind "" or "ok" — writer.go's completed vocabulary).
// A malformed payload fails closed: the bracket does not seed.
func turnEndCompleted(ev session.Event) bool {
	if len(ev.Payload) == 0 {
		return true // AppendTurnEnd("") writes {}; decode-side zero kind
	}
	var p session.TurnEndPayload
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		return false
	}
	return p.Kind == "" || p.Kind == "ok"
}
