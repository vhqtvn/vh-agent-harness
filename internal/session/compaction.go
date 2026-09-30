// Package session — compaction (slice 2).
//
// Compaction replaces SURFACE, never the log: the full event log is
// retained; compaction only changes what DeriveMessages/FoldSurface yield
// (dsh cross-cutting mechanism C2). The event vocabulary is
//
//	compaction/start (log-only, the durable lock)
//	compaction/summary (message-bearing: user-role summary text carrying
//	                   surfaceOp {op: replace, start, end} + citations)
//	compaction/end (log-only, closes the bracket)
//
// An unmatched compaction/start (a crash or a refused summary between
// start and end) is deliberately left in the log: it costs no surface and
// marks the interrupted attempt for inspection — the dsh crash-lock
// semantic, simplified (no end-seed invalidation in slice 2).
package session

import (
	"fmt"
)

// DefaultPressureThreshold is the surface-pressure ratio at which the
// caller should invoke compaction (dsh DEFAULT_THRESHOLD_RATIO = 0.8).
const DefaultPressureThreshold = 0.8

// DefaultRetainTailMessages is how many trailing surface messages compaction
// always retains (the recent-context tail).
const DefaultRetainTailMessages = 2

// SessionConfig carries the context-budget knobs the pressure policy uses.
type SessionConfig struct {
	// ContextBudgetTokens is the session's context budget in tokens. The
	// heuristic estimate is chars/4; a real provider usage envelope anchors
	// it when available.
	ContextBudgetTokens int
	// PressureThreshold is the compaction trigger ratio; 0 means
	// DefaultPressureThreshold (0.8).
	PressureThreshold float64
}

func (c SessionConfig) threshold() float64 {
	if c.PressureThreshold <= 0 {
		return DefaultPressureThreshold
	}
	return c.PressureThreshold
}

func (c SessionConfig) validate() error {
	if c.ContextBudgetTokens <= 0 {
		return fmt.Errorf("session: SessionConfig.ContextBudgetTokens must be positive, got %d", c.ContextBudgetTokens)
	}
	if c.PressureThreshold < 0 || c.PressureThreshold > 1 {
		return fmt.Errorf("session: SessionConfig.PressureThreshold must be within [0,1], got %f", c.PressureThreshold)
	}
	return nil
}

// SurfacePressure estimates current surface pressure as a ratio of the
// context budget. The heuristic is total message chars / 4; when the last
// llm/response carries a non-zero usage envelope, the real prompt-tokens
// figure anchors the estimate (max of the two, so a surface that grew since
// the last provider report is not under-counted).
func SurfacePressure(events []Event, cfg SessionConfig) (float64, error) {
	if err := cfg.validate(); err != nil {
		return 0, err
	}
	s, err := FoldSurface(events)
	if err != nil {
		return 0, err
	}
	estimate := float64(messageChars(s.Messages)) / 4.0
	// Anchor: scan backwards for the most recent llm/response usage.
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type != TypeLLMResponse {
			continue
		}
		var p LLMResponsePayload
		if err := unmarshalPayload(events[i], &p); err != nil {
			return 0, err
		}
		if p.Usage.PromptTokens > 0 && float64(p.Usage.PromptTokens) > estimate {
			estimate = float64(p.Usage.PromptTokens)
		}
		break
	}
	return estimate / float64(cfg.ContextBudgetTokens), nil
}

// ShouldCompact is the pressure trigger policy: compaction is invoked when
// surface pressure reaches the configured threshold. Slice 2 provides the
// policy function; the engine wiring that calls it per turn comes later.
func ShouldCompact(events []Event, cfg SessionConfig) (bool, float64, error) {
	p, err := SurfacePressure(events, cfg)
	if err != nil {
		return false, 0, err
	}
	return p >= cfg.threshold(), p, nil
}

// messageChars is the chars-based size heuristic over a message span
// (content only; names and tool-call shapes are second-order for slice 2).
func messageChars(msgs []Message) int {
	n := 0
	for i := range msgs {
		n += len(msgs[i].Content)
	}
	return n
}

// Summarizer condenses the shadowed message span into summary text. In
// tests this is a deterministic stub — no real LLM.
//
// Production design note (NOT implemented in slice 2): the summarization
// call must be built as a genuine PREFIX of the main conversation — the
// same system prompt + tool advertisements + ONLY the shadowed-region
// messages — so the provider's KV cache makes the extra call cheap (dsh
// region.ts buildSummarizationInput, cross-cutting mechanism C4).
type Summarizer func(shadowed []Message) (string, error)

// SummaryTooLargeError is returned when the produced summary is not
// strictly smaller than the span it shadows: compaction must never make
// the context bigger (dsh region.ts summary-must-be-smaller check).
type SummaryTooLargeError struct {
	SummaryChars  int
	ShadowedChars int
}

func (e *SummaryTooLargeError) Error() string {
	return fmt.Sprintf("session: compaction refused: summary is %d chars, not smaller than the %d-char shadowed span", e.SummaryChars, e.ShadowedChars)
}

// CompactOptions parameterizes one compaction run.
type CompactOptions struct {
	// Reason is recorded on the compaction/start bracket (audit).
	Reason string
	// Pressure is the optional pressure snapshot recorded on start.
	Pressure float64
	// RetainTailMessages is how many trailing messages to retain; 0 means
	// DefaultRetainTailMessages (2). At least one message is ALWAYS
	// retained — shadowing the entire surface is refused.
	RetainTailMessages int
}

// CompactionResult reports what one successful compaction did.
type CompactionResult struct {
	StartSeq          int64
	SummarySeq        int64
	EndSeq            int64
	ShadowedRange     [2]int // positional span [start,end) that was replaced
	SourceEventSeqs   []int64
	ReplaceGeneration int
}

// Compact runs one compaction over the head of the surface, retaining the
// tail: it shadows positions [0, len-retain) with a single user-role
// summary message that cites every shadowed event's seq. The flow is:
//  1. compaction/start (log-only durable lock) is appended BEFORE the
//     summarizer runs — a crash or refusal leaves the unmatched start
//     visible (the lock), and the surface unchanged;
//  2. the summarizer sees ONLY the shadowed messages;
//  3. the summary must be strictly smaller than the shadowed span
//     (chars heuristic), else SummaryTooLargeError and nothing more is
//     appended;
//  4. compaction/summary (the replace event) and compaction/end land,
//     and the fold's replaceGeneration advances by one.
//
// Lock discipline (TA-R1): the whole fold→summarize→append sequence runs
// under the WRITE lock, making one compaction atomic against concurrent
// appends — the fold observes a stable event list, so the cited
// SourceEventSeqs, the shadow span, and the replaceGeneration the
// summary records are exactly the fold's, and the three bracket events
// land contiguously in seq order. Appends block for the duration of the
// run, summarizer included: citation stability and replay determinism
// outrank append latency (and in the engine wiring compaction runs
// between turns, so the contention window is bounded). The appends go
// through appendLocked/writeEventLocked — taking l.mu again here would
// deadlock.
func (l *Log) Compact(sum Summarizer, opts CompactOptions) (CompactionResult, error) {
	if sum == nil {
		return CompactionResult{}, fmt.Errorf("session: Compact requires a non-nil Summarizer")
	}
	retain := opts.RetainTailMessages
	if retain == 0 {
		retain = DefaultRetainTailMessages
	}
	if retain < 0 {
		return CompactionResult{}, fmt.Errorf("session: CompactOptions.RetainTailMessages must be >= 0, got %d", opts.RetainTailMessages)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	cur, err := FoldSurface(l.events)
	if err != nil {
		return CompactionResult{}, err
	}
	shadowEnd := len(cur.Messages) - retain
	if shadowEnd <= 0 {
		return CompactionResult{}, fmt.Errorf("session: nothing to compact: surface of %d messages retains %d", len(cur.Messages), retain)
	}

	startEv, err := l.appendLocked(TypeCompactionStart, nil, CompactionStartPayload{
		Reason:        opts.Reason,
		Pressure:      opts.Pressure,
		ShadowedRange: [2]int{0, shadowEnd},
	})
	if err != nil {
		return CompactionResult{}, err
	}

	shadowed := make([]Message, shadowEnd)
	copy(shadowed, cur.Messages[:shadowEnd])
	sourceSeqs := make([]int64, shadowEnd)
	copy(sourceSeqs, cur.OriginSeq[:shadowEnd])

	summary, err := sum(shadowed)
	if err != nil {
		return CompactionResult{}, fmt.Errorf("session: summarizer failed after compaction/start seq %d (lock left): %w", startEv.Seq, err)
	}

	summaryChars, shadowedChars := len(summary), messageChars(shadowed)
	if summaryChars >= shadowedChars {
		return CompactionResult{}, &SummaryTooLargeError{SummaryChars: summaryChars, ShadowedChars: shadowedChars}
	}

	gen := cur.ReplaceGeneration + 1
	sumEv, err := l.appendLocked(TypeCompactionSummary, &SurfaceOp{Op: SurfaceOpReplace, Start: 0, End: shadowEnd}, CompactionSummaryPayload{
		Text:              summary,
		SourceEventSeqs:   sourceSeqs,
		ShadowedRange:     [2]int{0, shadowEnd},
		ReplaceGeneration: gen,
	})
	if err != nil {
		return CompactionResult{}, err
	}
	endEv, err := l.appendLocked(TypeCompactionEnd, nil, CompactionEndPayload{
		SummarySeq:        sumEv.Seq,
		ReplaceGeneration: gen,
	})
	if err != nil {
		return CompactionResult{}, err
	}
	return CompactionResult{
		StartSeq:          startEv.Seq,
		SummarySeq:        sumEv.Seq,
		EndSeq:            endEv.Seq,
		ShadowedRange:     [2]int{0, shadowEnd},
		SourceEventSeqs:   sourceSeqs,
		ReplaceGeneration: gen,
	}, nil
}

// ReplaceGeneration folds the live log and returns the monotone count of
// committed surface replaces. Replay of a persisted log reproduces it.
//
// Lock discipline (TA-R1): this is a PURE READ — no append follows the
// fold, so there is no read-modify-write to serialize (the write-side
// discipline lives in Compact, which holds the write lock across ITS
// fold+appends). The RLock-snapshot discipline of Surface() is therefore
// the correct one: copy the event list under the read lock, fold the
// stable copy.
func (l *Log) ReplaceGeneration() (int, error) {
	l.mu.RLock()
	events := make([]Event, len(l.events))
	copy(events, l.events)
	l.mu.RUnlock()
	s, err := FoldSurface(events)
	if err != nil {
		return 0, err
	}
	return s.ReplaceGeneration, nil
}

// Unfold re-derives the PRE-COMPACTION surface cited by the
// compaction/summary event with the given seq: the fold of every event
// before the summary, with the citations verified against that fold's
// origin seqs. Given SourceEventSeqs, the summarized content is always
// recoverable from the retained log — compaction is reversible-by-replay.
func Unfold(events []Event, summarySeq int64) ([]Message, error) {
	idx := -1
	for i := range events {
		if events[i].Seq == summarySeq {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("session: unfold: no event with seq %d", summarySeq)
	}
	if events[idx].Type != TypeCompactionSummary {
		return nil, fmt.Errorf("session: unfold: event %d is %q, not %q", summarySeq, events[idx].Type, TypeCompactionSummary)
	}
	var sp CompactionSummaryPayload
	if err := unmarshalPayload(events[idx], &sp); err != nil {
		return nil, err
	}
	pre, err := FoldSurface(events[:idx])
	if err != nil {
		return nil, err
	}
	start, end := sp.ShadowedRange[0], sp.ShadowedRange[1]
	if start < 0 || end < start || end > len(pre.Messages) {
		return nil, fmt.Errorf("session: unfold: summary %d cites invalid span [%d,%d) over %d pre-compaction messages", summarySeq, start, end, len(pre.Messages))
	}
	cited := pre.OriginSeq[start:end]
	if len(cited) != len(sp.SourceEventSeqs) {
		return nil, fmt.Errorf("session: unfold: summary %d cites %d events but the span holds %d", summarySeq, len(sp.SourceEventSeqs), len(cited))
	}
	for i := range cited {
		if cited[i] != sp.SourceEventSeqs[i] {
			return nil, fmt.Errorf("session: unfold: summary %d citation %d is seq %d, want %d", summarySeq, i, sp.SourceEventSeqs[i], cited[i])
		}
	}
	return pre.Messages, nil
}
