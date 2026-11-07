// compact.go — the daemon-level compaction wiring (P5): the seam that
// makes the slice-2 session compaction vocabulary (internal/session
// compaction.go) LIVE in the running engine.
//
// SEAM CHOICE + RATIONALE (documented per the slice contract): the
// trigger is a TurnRunner DECORATOR at the daemon layer, not a change
// inside internal/session, internal/tools, or the protocol handler:
//
//   - internal/session cannot host the adapter-backed Summarizer: the
//     import would cycle (internal/adapters aliases session types, so
//     session must not import adapters). The daemon layer already
//     imports both — it is the natural composition point (same
//     reasoning as the prompt optimizer, prompt.go).
//
//   - decorating the TurnRunner (vs. touching RunTurn or the handler)
//     keeps the turn choreography, the retry ladder, and the wire
//     shape byte-identical: the decorator runs AFTER the inner turn
//     has fully committed (turn/end appended) and INSIDE the handler's
//     per-session turn gate (handleSessionPrompt's beginTurn/endTurn
//     bracket), so compaction is serialized with turns by construction
//     — no new locking surface, and the compaction/* events land on
//     the wire before the session/prompt response (the client observes
//     them as ordinary session/event notifications).
//
//   - checking AFTER a turn (never mid-turn) honors the slice-2 lock
//     discipline: Compact holds the log write lock across its
//     fold→summarize→append sequence, so a mid-turn trigger would hold
//     appends hostage inside a turn; at the boundary the contention
//     window is the summarize call alone.
//
// FAILURE POSTURE (load-bearing): a compaction failure NEVER fails the
// turn or the wire call. The surface stays un-compacted (slice-2's
// unmatched-start lock semantics), the daemon logs ONE stderr line,
// and the NEXT turn boundary retries — the boundary loop IS the retry
// ladder for compaction; there is deliberately no retry inside the
// summarizer.
//
// CHILD/COVERAGE POSTURE (v1): compaction runs on PARENT-session turns
// only. Subagent child turns run through this same decorated runner
// (the child executor calls engine.TurnRunner()) but carry
// opts.InboxDriven — the decorator skips them (children are short-lived
// by design; compacting their surfaces is a documented non-goal).
//
// AUDIT GAP (disclosed): the summarize call is a real LLM call through
// the same adapter, but it is NOT a turn — the slice-2 vocabulary has
// no event for a non-turn LLM call, so no llm/request record is written
// for it. The compaction/start … compaction/end bracket IS the audit
// trail (pressure snapshot + shadowed range on start, citations +
// generation on summary); the request-plane audit lives in the
// provider journal (and the docker battery asserts its shape).
package main

import (
	"context"
	"log"
	"os"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
	"github.com/vhqtvn/vh-agent-harness/internal/protocol"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// compactionInstruction is the ONE new message the summarize request
// appends after the shadowed region. Everything before it — system
// prompt, tool definitions, and the shadowed messages — is byte-for-byte
// the running conversation's request prefix, so the provider's KV cache
// absorbs the whole prefix and only the instruction (plus generation)
// is new (the dsh region.ts buildSummarizationInput pattern cited in
// the slice-2 Summarizer doc note).
const compactionInstruction = "Summarize the conversation above for a compacted context: distill the established facts, decisions, tool activity, and any open items into one dense passage a continuing assistant can work from. Do not restate every message — preserve what matters, drop what does not. Your reply is used verbatim as the summary, so reply with the summary text only."

// compactingTurnRunner decorates the engine's TurnRunner with the
// post-turn compaction trigger. It is a PURE wrapper (stateless across
// calls; per-turn state lives in closures), so TurnRunner() re-applying
// it per call is harmless.
type compactingTurnRunner struct {
	inner  protocol.TurnRunner
	cfg    session.SessionConfig
	printf func(format string, args ...any)
}

// newCompactingTurnRunner wraps inner with the pressure trigger bound
// to cfg. printf receives the daemon's diagnostics (one line per
// compaction attempt outcome); nil selects the stderr default.
func newCompactingTurnRunner(inner protocol.TurnRunner, cfg session.SessionConfig, printf func(format string, args ...any)) *compactingTurnRunner {
	if printf == nil {
		printf = stderrCompactionLogger()
	}
	return &compactingTurnRunner{inner: inner, cfg: cfg, printf: printf}
}

// stderrCompactionLogger mirrors run()'s stderr diagnostic logger
// (stdout is protocol; compaction notes are diagnostics, not protocol).
func stderrCompactionLogger() func(format string, args ...any) {
	l := log.New(os.Stderr, "vh-agentd ", log.LstdFlags|log.LUTC|log.Lmsgprefix)
	return func(format string, args ...any) { l.Printf(format, args...) }
}

// RunTurn runs the inner turn, then — only after a SUCCESSFUL turn, on
// a parent-session (non-InboxDriven) log, with compaction armed —
// checks surface pressure and compacts. The compaction leg can never
// fail the turn: see maybeCompact.
func (r *compactingTurnRunner) RunTurn(ctx context.Context, lg *session.Log, ad adapters.Adapter, opts tools.TurnOptions, prompt string) (*tools.TurnReport, error) {
	report, err := r.inner.RunTurn(ctx, lg, ad, opts, prompt)
	if err != nil {
		// A failed turn leaves the surface as the turn left it; the
		// next successful boundary re-checks pressure.
		return report, err
	}
	if opts.InboxDriven {
		// Child (subagent) turns: v1 compacts parent sessions only —
		// children are short-lived by design (documented posture).
		return report, nil
	}
	r.maybeCompact(ctx, lg, ad, opts)
	return report, nil
}

// maybeCompact is the turn-boundary trigger: ShouldCompact over the
// committed log, then Compact with the adapter-backed summarizer.
// Every failure path is one stderr line and return — the turn has
// already succeeded and must stay succeeded.
func (r *compactingTurnRunner) maybeCompact(ctx context.Context, lg *session.Log, ad adapters.Adapter, opts tools.TurnOptions) {
	if r.cfg.ContextBudgetTokens <= 0 {
		return // compaction disabled (--context-tokens 0)
	}
	should, pressure, err := session.ShouldCompact(lg.Events(), r.cfg)
	if err != nil {
		r.printf("compaction: pressure check failed: %v (surface un-compacted; next turn boundary retries)", err)
		return
	}
	if !should {
		return
	}
	res, err := lg.Compact(adapterSummarizer(ctx, ad, opts), session.CompactOptions{
		Reason:   "turn-boundary pressure",
		Pressure: pressure,
	})
	if err != nil {
		// Includes session.SummaryTooLargeError (summary refused — the
		// provider returned something not strictly smaller than the
		// shadowed span) and every typed adapter failure: surface stays
		// un-compacted (the unmatched compaction/start is the durable
		// lock), one line, retried at the next boundary.
		r.printf("compaction: deferred at pressure %.3f: %v (surface un-compacted; next turn boundary retries)", pressure, err)
		return
	}
	r.printf("compaction: shadowed surface messages [%d,%d) citing %d events (pressure %.3f, generation %d, summary seq %d)",
		res.ShadowedRange[0], res.ShadowedRange[1], len(res.SourceEventSeqs), pressure, res.ReplaceGeneration, res.SummarySeq)
}

// adapterSummarizer builds the KV-PREFIX-PRESERVING session.Summarizer
// over the turn's own adapter and options: the summarize request is
//
//	system prompt (opts.System) + tool definitions (opts.Tools)
//	+ the shadowed-region messages + ONE appended instruction
//
// so everything before the instruction is a genuine, byte-identical
// prefix of the running conversation's request (same fold, same
// encoder) and hits the provider's KV cache. A summarize-specific
// system prompt would break the prefix at token zero — the instruction
// rides the message tail instead. ONE call, NO retry ladder: provider
// errors ride the existing typed AdapterError classification unchanged
// and surface as a deferred compaction (next-boundary retry).
func adapterSummarizer(ctx context.Context, ad adapters.Adapter, opts tools.TurnOptions) session.Summarizer {
	return func(shadowed []session.Message) (string, error) {
		msgs := make([]adapters.Message, 0, len(shadowed)+2)
		if opts.System != "" {
			msgs = append(msgs, adapters.Message{Role: "system", Content: opts.System})
		}
		msgs = append(msgs, shadowed...)
		msgs = append(msgs, adapters.Message{Role: "user", Content: compactionInstruction})
		resp, err := ad.Call(ctx, &adapters.Request{
			Model:       opts.Model,
			Messages:    msgs,
			Tools:       opts.Tools,
			Temperature: opts.Temperature,
			MaxTokens:   opts.MaxTokens,
		})
		if err != nil {
			return "", err
		}
		if resp == nil || resp.Content == "" {
			// Empty content (with or without tool calls): there is no
			// execution path here, so a tool-call answer or an empty
			// body is a failed summarize — classified empty-response,
			// deferred to the next boundary.
			return "", adapters.EmptyResponseError(ad.Name(), opts.Model)
		}
		// A response carrying BOTH content and tool calls is taken by
		// its content; the calls are never executed on this path.
		return resp.Content, nil
	}
}
