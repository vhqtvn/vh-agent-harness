// turn.go — the RunTurn orchestration over one session log and one
// adapter: one bounded tool turn in which the model's N requested tool
// calls flow through the scheduler (waterfall → guards → approval →
// dispatch → post-observe) with model-ordered durable intents and
// results, bracketed by exactly one turn/begin … turn/end.
//
// Slice B1 — retry-ladder integration decision (documented tradeoff).
// The durable retry ladder (typed AdapterError.Retryable classification,
// llm/retry BEFORE the backoff wait → llm/retry-started after it,
// fresh-surface-per-attempt, exhaustion → turn/end{kind:error}) is
// implemented here as an OPTION ON THE PIPELINE (TurnOptions.Retry),
// reusing internal/loop's frozen RetryPolicy, payload shapes, and
// classification — not by nesting loop.Driver inside RunTurn and not by
// a wrapping TurnRunner. Why:
//
//   - Driver-inside-RunTurn: Driver.Run owns its OWN turn/begin+prompt
//     bracket; nesting it inside RunTurn's bracket would double the
//     choreography (two turn/begin per protocol turn) unless Driver were
//     refactored down to a bracket-free core — moving the ladder out of
//     the component that owns turn choreography just to re-wrap it.
//   - Wrapping TurnRunner: RunTurn exposes no bracket-free body, so a
//     wrapper would have to reimplement the WHOLE turn (bracket, prompt,
//     request, response, tool execution) to interpose retries around the
//     single adapter call — strictly worse duplication.
//   - Retry option on Pipeline (chosen): the ladder interleaves with
//     turn-scoped events only RunTurn owns (llm/request per attempt, the
//     fresh surface derivation, the single closing turn/end). Cost:
//     tools grows a retry concern and imports loop (acceptable — loop is
//     a sibling policy package over the same adapters/session types; no
//     cycle: loop imports neither tools nor protocol). Driver remains
//     the standalone no-tools driver for loop-internal consumers.
//
// Choreography invariants (host-protocol §4/§5): EXACTLY one
// turn/begin+turn/end bracket per protocol turn regardless of retry
// count; every attempt re-derives the request surface from the committed
// log (never in-memory state); llm/retry records are log-only, so replay
// determinism holds — the fold never recomputes backoff.
package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
	"github.com/vhqtvn/vh-agent-harness/internal/loop"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// TurnOptions configures one RunTurn.
type TurnOptions struct {
	Model       string
	Tools       []adapters.ToolSpec
	Temperature *float64
	MaxTokens   int
	// MaxParallel overrides the pipeline's rolling-pool cap for this
	// turn's batch when > 0.
	MaxParallel int
	// System is the system prompt for this turn. When non-empty it is
	// prepended to the log-derived surface as a role="system" message
	// (the anthropic adapter extracts it to the top-level system field;
	// openaicompat endpoints accept the role directly). It is turn
	// CONFIG, not conversation: it never enters the session log, so the
	// derived surface stays a pure projection of the log. Empty (the
	// zero value) sends no system message — byte-stable with the
	// pre-B1 request shape.
	System string
	// Retry, when non-nil, arms the durable retry ladder around the
	// adapter call. Nil (the zero value) keeps the single-attempt
	// contract: any adapter failure closes the turn immediately —
	// byte-stable with the pre-B1 failure logs.
	Retry *RetryLadder
	// InboxDriven, when true, runs the turn WITHOUT appending a new
	// session/prompt record: the turn is driven by message events
	// already committed to the log (a subagent child's inbox — the
	// initial prompt and follow-ups arrive as subagent/message records;
	// see internal/subagents). The prompt argument is ignored in this
	// mode. False (the zero value) keeps the B1 choreography unchanged:
	// every turn appends its own session/prompt.
	InboxDriven bool
}

// RetryLadder arms RunTurn's durable retry ladder. It binds loop's
// frozen RetryPolicy plus the injected time seams so the ladder is
// deterministic under test.
type RetryLadder struct {
	// Config is the retry configuration; zero values take loop's
	// defaults (2 retries, 500ms initial backoff doubling to a 10s cap,
	// ±10% jitter). Validated at turn time — an invalid config fails the
	// turn before any adapter call.
	Config loop.RetryConfig
	// Sleeper waits out the backoff; nil ⇒ loop.RealSleeper. Tests
	// inject a recorder.
	Sleeper loop.Sleeper
	// Rand is the jitter source r ∈ [0,1) fed to the policy's Backoff.
	// nil ⇒ a FIXED 0.5 (mid-band): the daemon's default backoffs are
	// deterministic, so identical failure scripts produce byte-identical
	// llm/retry records. Herd-sensitive deployments inject entropy.
	Rand func() float64
}

// arm freezes the ladder's policy and time seams.
func (r *RetryLadder) arm() (*loop.RetryPolicy, loop.Sleeper, func() float64, error) {
	policy, err := loop.NewRetryPolicy(r.Config)
	if err != nil {
		return nil, nil, nil, err
	}
	sleeper := r.Sleeper
	if sleeper == nil {
		sleeper = loop.RealSleeper{}
	}
	rand := r.Rand
	if rand == nil {
		rand = func() float64 { return 0.5 }
	}
	return policy, sleeper, rand, nil
}

// TurnReport reports one completed RunTurn: the assistant response whose
// tool calls were executed, and the model-ordered results.
type TurnReport struct {
	Response *adapters.Response
	Results  []Result
	// Attempts is the number of adapter calls made (1 = first try; >1
	// only when the retry ladder fired).
	Attempts int
}

// RunTurn drives one bounded tool turn:
//
//	turn/begin → session/prompt → [per attempt: llm/request → adapter
//	call; on a retryable failure: llm/retry → backoff wait →
//	llm/retry-started → FRESH surface re-derivation] → llm/response →
//	[tool/call ×N in model order → batch execution per concurrency
//	policy → tool/result ×N in model order] → turn/end.
//
// With opts.InboxDriven the session/prompt record is omitted: the turn
// answers messages already committed to the log (subagent child inbox).
//
// On a terminal adapter failure (non-retryable class, retry exhaustion
// when the ladder is armed, or any failure with the ladder disarmed) the
// turn closes with turn/end {kind: error} and a typed error; no tool
// runs. Exhaustion surfaces *loop.ExhaustedError. The prompt surface
// handed to the adapter is always the log-derived projection (plus the
// configured system message), never in-memory state.
func (p *Pipeline) RunTurn(ctx context.Context, lg *session.Log, ad adapters.Adapter, opts TurnOptions, prompt string) (*TurnReport, error) {
	if lg == nil {
		return nil, errors.New("tools: nil session log")
	}
	if ad == nil {
		return nil, errors.New("tools: nil adapter")
	}
	var policy *loop.RetryPolicy
	var sleeper loop.Sleeper
	var rand func() float64
	if opts.Retry != nil {
		var err error
		policy, sleeper, rand, err = opts.Retry.arm()
		if err != nil {
			return nil, fmt.Errorf("tools: retry ladder: %w", err)
		}
	}
	if _, err := lg.AppendTurnBegin(); err != nil {
		return nil, fmt.Errorf("tools: append turn/begin: %w", err)
	}
	// Bind the executing session for tool bodies: the model-facing
	// subagent family resolves the executing session's manager through
	// this (a shared pipeline serves every session; the binding is what
	// makes a tool session-aware). Applied after turn/begin so the log
	// is confirmed writable first; tools never see the pre-turn ctx.
	ctx = WithExecutingSession(ctx, lg.SessionID())
	if !opts.InboxDriven {
		if _, err := lg.AppendPrompt(prompt); err != nil {
			return nil, fmt.Errorf("tools: append prompt: %w", err)
		}
	}

	var resp *adapters.Response
	attempt := 0 // retries spent so far (0-based, Driver numbering)
	for {
		if err := ctx.Err(); err != nil {
			_ = p.closeTurnError(lg, fmt.Sprintf("context: %v", err))
			return nil, fmt.Errorf("tools: run canceled: %w", err)
		}

		// Fresh durable surface per attempt: the request is always
		// derived from the committed log, never from in-memory state.
		msgs, err := lg.Surface()
		if err != nil {
			return nil, fmt.Errorf("tools: derive surface: %w", err)
		}
		if opts.System != "" {
			msgs = append([]adapters.Message{{Role: "system", Content: opts.System}}, msgs...)
		}
		if _, err := lg.AppendLLMRequest(opts.Model, toolNames(opts.Tools), opts.Temperature, opts.MaxTokens); err != nil {
			return nil, fmt.Errorf("tools: append llm/request: %w", err)
		}
		var callErr error
		resp, callErr = ad.Call(ctx, &adapters.Request{
			Model:       opts.Model,
			Messages:    msgs,
			Tools:       opts.Tools,
			Temperature: opts.Temperature,
			MaxTokens:   opts.MaxTokens,
		})
		attempt++

		aerr := loop.Classify(callErr, ad.Name(), opts.Model, resp)
		if aerr == nil {
			break // clean response — fall through to success path
		}

		// Terminal when the ladder is disarmed (single-attempt
		// contract), the class is non-retryable, or the budget is spent.
		// Non-retryability is checked FIRST (Driver order): a
		// non-retryable class never reports exhaustion even when the
		// budget happens to be spent on the same call.
		exhausted := false
		if policy == nil || !aerr.Retryable() {
			exhausted = false
		} else if attempt-1 >= policy.MaxRetries() {
			exhausted = true
		}
		if policy == nil || !aerr.Retryable() || exhausted {
			reason, retErr := p.terminalFailure(policy, callErr, aerr, attempt, exhausted)
			_ = p.closeTurnError(lg, reason)
			return nil, retErr
		}

		// Durable retry record BEFORE waiting (crash mid-wait still
		// shows the numbered intent), then the wait, then
		// retry-started — the Driver ordering, verbatim.
		backoff := policy.Backoff(attempt, aerr.RetryAfterMs, rand())
		if _, err := lg.Append(session.TypeLLMRetry, nil, loop.RetryPayload{
			Attempt:      attempt,
			Policy:       loop.SnapshotOf(policy),
			ErrorClass:   string(aerr.Kind),
			ErrorMessage: aerr.Error(),
			BackoffMs:    backoff.Milliseconds(),
			RetryAfterMs: aerr.RetryAfterMs,
		}); err != nil {
			return nil, fmt.Errorf("tools: append llm/retry: %w", err)
		}
		sleeper.Sleep(backoff)
		if _, err := lg.Append(session.TypeLLMRetryStarted, nil, loop.RetryStartedPayload{Attempt: attempt}); err != nil {
			return nil, fmt.Errorf("tools: append llm/retry-started: %w", err)
		}
	}

	model := resp.Model
	if model == "" {
		model = opts.Model
	}
	if _, err := lg.AppendLLMResponse(model, resp.Content, resp.ToolCalls, resp.Usage); err != nil {
		return nil, fmt.Errorf("tools: append llm/response: %w", err)
	}
	report := &TurnReport{Response: resp, Results: nil, Attempts: attempt}
	if len(resp.ToolCalls) == 0 {
		if _, err := lg.AppendTurnEnd(""); err != nil {
			return nil, fmt.Errorf("tools: append turn/end: %w", err)
		}
		return report, nil
	}

	maxParallel := p.maxParallel
	if opts.MaxParallel > 0 {
		maxParallel = opts.MaxParallel
	}
	results, err := p.executeBatch(ctx, lg, resp.ToolCalls, maxParallel)
	if err != nil {
		return nil, err
	}
	report.Results = results
	if _, err := lg.AppendTurnEnd(""); err != nil {
		return nil, fmt.Errorf("tools: append turn/end: %w", err)
	}
	return report, nil
}

// terminalFailure shapes the closing reason and the returned error for a
// terminal adapter failure. With the ladder DISARMED the pre-B1 shapes
// are preserved byte-for-byte (raw call-error text; "empty response" for
// the empty class). With the ladder ARMED the typed classification text
// closes the turn; exhausted=true (a retryable class that spent the
// budget) surfaces *loop.ExhaustedError.
func (p *Pipeline) terminalFailure(policy *loop.RetryPolicy, callErr error, aerr *adapters.AdapterError, attempts int, exhausted bool) (reason string, retErr error) {
	if policy == nil {
		if callErr != nil {
			return callErr.Error(), fmt.Errorf("tools: adapter call: %w", callErr)
		}
		return "empty response", errors.New("tools: adapter call: empty response")
	}
	if exhausted {
		return aerr.Error(), &loop.ExhaustedError{Attempts: attempts, Last: aerr}
	}
	// Non-retryable class: immediate typed failure, no retry events.
	return aerr.Error(), fmt.Errorf("tools: adapter call: %w", aerr)
}

func (p *Pipeline) closeTurnError(lg *session.Log, reason string) error {
	_, err := lg.AppendTurnEndKind("error", reason)
	return err
}

func toolNames(specs []adapters.ToolSpec) []string {
	if len(specs) == 0 {
		return nil
	}
	names := make([]string, len(specs))
	for i, s := range specs {
		names[i] = s.Name
	}
	return names
}
