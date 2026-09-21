// derive.go — the P4 derived-session projections: title and usage
// totals as pure folds over an event list. Both are REPLAY-DERIVED by
// design (no new events, no new durable state): the session's identity
// surface (what session/list and session/resume report) is a projection
// of the log, exactly like the message surface.
package session

import (
	"encoding/json"
	"strings"
)

// TitleMaxRunes is the derived-title budget: titles longer than this
// are rune-truncated with a trailing ellipsis. 60 runes keeps a
// one-line listing readable at 80 columns with the session id beside
// it.
const TitleMaxRunes = 60

// DeriveTitle projects the session title from the event list: the
// FIRST user prompt (session/prompt), whitespace-collapsed to one
// line, truncated to TitleMaxRunes runes with a trailing ellipsis.
// A log with no user prompt derives the empty title (the honest
// "nothing to name it by" — never assistant words, never a synthetic
// placeholder). Non-prompt message-bearing events never contribute:
// the user's own first words are the session's name.
func DeriveTitle(events []Event) string {
	for i := range events {
		if events[i].Type != TypeSessionPrompt {
			continue
		}
		var p PromptPayload
		if err := json.Unmarshal(events[i].Payload, &p); err != nil {
			// A malformed prompt payload cannot survive replay's own
			// fail-closed checks upstream; treat as no-title rather
			// than guessing content out of a broken record.
			return ""
		}
		return truncateTitle(p.Text)
	}
	return ""
}

// truncateTitle applies the single-line + budget rule.
func truncateTitle(text string) string {
	oneLine := strings.Join(strings.Fields(text), " ")
	r := []rune(oneLine)
	if len(r) <= TitleMaxRunes {
		return oneLine
	}
	return string(r[:TitleMaxRunes]) + "…"
}

// SumUsage projects the session's cumulative token usage: the SUM of
// every llm/response usage envelope in the event list, in log order.
// Usage has been logged on every llm/response since slice 1 (the
// envelope is non-omitempty), so the sum is replay-derivable for ALL
// logs — including pre-P4 ones. Logs from providers that do not
// report usage honestly sum to zero (the envelope is present but
// zero-valued); the projection never invents numbers.
func SumUsage(events []Event) Usage {
	var total Usage
	for i := range events {
		if events[i].Type != TypeLLMResponse {
			continue
		}
		var p LLMResponsePayload
		if err := json.Unmarshal(events[i].Payload, &p); err != nil {
			// Same posture as DeriveTitle: a payload this malformed
			// fails replay upstream; skip rather than guess.
			continue
		}
		total.PromptTokens += p.Usage.PromptTokens
		total.CompletionTokens += p.Usage.CompletionTokens
		total.TotalTokens += p.Usage.TotalTokens
	}
	return total
}

// LastActivityHint is documented guidance for engines deriving a
// last-activity timestamp: session events carry NO timestamps (the
// slice-1 determinism design), so the only activity signal visible
// without reading the log is the file's mtime. Engines state their
// rule; this constant documents why no in-log field exists.
const LastActivityHint = "session events carry no timestamps; lastActivity is engine-derived (v1: log file mtime)"
