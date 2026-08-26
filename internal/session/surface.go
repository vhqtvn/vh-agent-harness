package session

import (
	"encoding/json"
	"fmt"
)

// Message is one LLM-visible message in the derived surface. The surface
// is a projection of the event log — it is never stored.
type Message struct {
	Role       string     `json:"role"` // user | assistant | tool
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"toolCalls,omitempty"` // assistant messages
	ToolCallID string     `json:"toolCallId,omitempty"`
	Name       string     `json:"name,omitempty"` // tool name on role=tool
}

// Surface is the full fold result over an event log: the derived message
// surface, the monotone replaceGeneration (incremented by each committed
// surface replace), and per-position origin event seqs (the citation basis
// compaction unfolds from).
type Surface struct {
	Messages          []Message
	ReplaceGeneration int
	// OriginSeq[i] is the seq of the event whose surfaceOp produced
	// Messages[i]. It is exactly the citation list a compaction of the span
	// [i, j) must record in SourceEventSeqs.
	OriginSeq []int64
}

// DeriveMessages projects the LLM-visible message surface from the event
// log. Message-bearing events (session/prompt, llm/response, tool/result,
// compaction/summary) MUST carry a surfaceOp: append grows the surface;
// replace shadows the cited positional span with the event's own message
// (compaction shape). Log-only events (header, llm/request, tool/call, turn
// brackets, compaction brackets, retry records) and ignorable unknown
// events contribute nothing. An unknown event type without ignorable fails
// closed.
func DeriveMessages(events []Event) ([]Message, error) {
	s, err := FoldSurface(events)
	if err != nil {
		return nil, err
	}
	return s.Messages, nil
}

// FoldSurface is the complete deterministic fold: messages plus the
// replaceGeneration counter and origin seqs. Replay of a persisted log
// reproduces the live fold exactly (same events in, same surface out).
func FoldSurface(events []Event) (*Surface, error) {
	s := &Surface{Messages: []Message{}, OriginSeq: []int64{}}
	for i := range events {
		ev := events[i]
		if !messageBearing(ev.Type) {
			switch ev.Type {
			case TypeSessionHeader, TypeLLMRequest, TypeToolCall, TypeTurnBegin, TypeTurnEnd,
				TypeCompactionStart, TypeCompactionEnd, TypeLLMRetry, TypeLLMRetryStarted,
				TypeJobEnqueued, TypeJobStarted, TypeJobSettled,
				TypeSubagentSpawned, TypeSubagentSettled:
				// log-only: contributes nothing to the surface
			default:
				if ev.Ignorable {
					continue
				}
				return nil, &UnknownEventError{Seq: ev.Seq, Type: ev.Type}
			}
			continue
		}
		if ev.SurfaceOp == nil {
			return nil, fmt.Errorf("session: %s at seq %d is message-bearing but carries no surfaceOp", ev.Type, ev.Seq)
		}
		msg, err := surfaceMessage(ev)
		if err != nil {
			return nil, err
		}
		next, nextOrigins, err := applySurfaceOp(s.Messages, s.OriginSeq, ev.SurfaceOp, msg, ev)
		if err != nil {
			return nil, err
		}
		s.Messages = next
		s.OriginSeq = nextOrigins
		if ev.SurfaceOp.Op == SurfaceOpReplace {
			s.ReplaceGeneration++
		}
	}
	return s, nil
}

// surfaceMessage decodes one message-bearing event into its surface
// message. The role is fixed by the event TYPE, never by payload data:
// user input only enters via session/prompt, assistant output only via
// llm/response, tool output only via tool/result — and a compaction
// summary enters as a user message (the conversation's own memory of
// itself, never assistant self-talk).
func surfaceMessage(ev Event) (Message, error) {
	switch ev.Type {
	case TypeSessionPrompt:
		var p PromptPayload
		if err := unmarshalPayload(ev, &p); err != nil {
			return Message{}, err
		}
		return Message{Role: "user", Content: p.Text}, nil
	case TypeCompactionSummary:
		var p CompactionSummaryPayload
		if err := unmarshalPayload(ev, &p); err != nil {
			return Message{}, err
		}
		return Message{Role: "user", Content: p.Text}, nil
	case TypeLLMResponse:
		var p LLMResponsePayload
		if err := unmarshalPayload(ev, &p); err != nil {
			return Message{}, err
		}
		return Message{Role: "assistant", Content: p.Content, ToolCalls: p.ToolCalls}, nil
	case TypeToolResult:
		var p ToolResultPayload
		if err := unmarshalPayload(ev, &p); err != nil {
			return Message{}, err
		}
		return Message{Role: "tool", ToolCallID: p.CallID, Name: p.Name, Content: p.Content}, nil
	case TypeJobReport:
		var p JobPayload
		if err := unmarshalPayload(ev, &p); err != nil {
			return Message{}, err
		}
		// The settled-job notice is an environment->model injection in the
		// same family as the compaction summary: it enters the surface as
		// a user message (the environment addressing the model, never
		// assistant self-talk). Content is derived deterministically from
		// the payload so replay reproduces it byte-for-byte. The P6
		// Detail (compact terminal facts, e.g. the background shell's
		// exit facts) appends in parentheses — absent on old logs, so
		// pre-P6 content is byte-identical.
		content := fmt.Sprintf("background job %s %s", p.JobID, p.Result)
		if p.Reason != "" {
			content += ": " + p.Reason
		}
		if p.Detail != "" {
			content += " (" + p.Detail + ")"
		}
		return Message{Role: "user", Content: content}, nil
	case TypeSubagentReport:
		var p SubagentPayload
		if err := unmarshalPayload(ev, &p); err != nil {
			return Message{}, err
		}
		// The child→parent report relay is provenance-tagged
		// subagent-report and enters the parent surface as a USER-role
		// context event — NEVER assistant: the parent transcript must not
		// credit the child with runtime words it did not say (dsh
		// provenance-clean reporting). Content is derived
		// deterministically from the payload so replay reproduces it
		// byte-for-byte.
		return Message{Role: "user", Content: fmt.Sprintf("subagent %s report: %s", p.ChildID, p.Content)}, nil
	case TypeSubagentMessage:
		var p SubagentPayload
		if err := unmarshalPayload(ev, &p); err != nil {
			return Message{}, err
		}
		// A parent→child follow-up lands in the child log as an
		// addressed user-role inbox message (the Agent inbox is the only
		// FIFO). The From sender rides the payload for audit; the surface
		// content is the plain text, like a user typing.
		return Message{Role: "user", Content: p.Text}, nil
	default:
		return Message{}, fmt.Errorf("session: %s at seq %d is not message-bearing", ev.Type, ev.Seq)
	}
}

// applySurfaceOp folds one message into the current surface (and its
// parallel origin-seq vector). append grows the surface; replace validates
// the positional span [Start, End) against the current length and shadows
// it with the new message.
func applySurfaceOp(cur []Message, origins []int64, op *SurfaceOp, msg Message, ev Event) ([]Message, []int64, error) {
	switch op.Op {
	case SurfaceOpAppend:
		out := make([]Message, len(cur), len(cur)+1)
		copy(out, cur)
		outOrigins := make([]int64, len(origins), len(origins)+1)
		copy(outOrigins, origins)
		return append(out, msg), append(outOrigins, ev.Seq), nil
	case SurfaceOpReplace:
		if op.Start < 0 || op.End < op.Start || op.End > len(cur) {
			return nil, nil, fmt.Errorf("session: %s at seq %d replaces invalid span [%d,%d) over %d messages",
				ev.Type, ev.Seq, op.Start, op.End, len(cur))
		}
		out := make([]Message, 0, len(cur)-(op.End-op.Start)+1)
		outOrigins := make([]int64, 0, len(origins)-(op.End-op.Start)+1)
		out = append(out, cur[:op.Start]...)
		outOrigins = append(outOrigins, origins[:op.Start]...)
		out = append(out, msg)
		outOrigins = append(outOrigins, ev.Seq)
		out = append(out, cur[op.End:]...)
		outOrigins = append(outOrigins, origins[op.End:]...)
		return out, outOrigins, nil
	default:
		return nil, nil, fmt.Errorf("session: %s at seq %d carries unknown surfaceOp %q", ev.Type, ev.Seq, op.Op)
	}
}

func unmarshalPayload(ev Event, v any) error {
	if len(ev.Payload) == 0 {
		return fmt.Errorf("session: %s at seq %d has no payload", ev.Type, ev.Seq)
	}
	if err := json.Unmarshal(ev.Payload, v); err != nil {
		return fmt.Errorf("session: %s at seq %d has a malformed payload: %w", ev.Type, ev.Seq, err)
	}
	return nil
}
