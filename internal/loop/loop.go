package loop

import (
	"context"
	"errors"
	"fmt"
	"math/rand"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// TurnEndKindError is the turn/end closure kind for failed turns (retry
// exhaustion or a non-retryable adapter error).
const TurnEndKindError = "error"

// Options configures one Driver.
type Options struct {
	// Model is the model name logged on responses.
	Model string
	// Tools are the tool advertisements sent with each request.
	Tools []adapters.ToolSpec
	// Temperature and MaxTokens pass through to the request.
	Temperature *float64
	MaxTokens   int
	// Retry is the retry configuration; zero takes the defaults.
	Retry RetryConfig
}

// Driver is the minimal durable turn driver over one session log and one
// adapter. It is not safe for concurrent use; the engine drives it
// single-threaded per session.
type Driver struct {
	lg      *session.Log
	ad      adapters.Adapter
	policy  *RetryPolicy
	sleeper Sleeper
	opts    Options
	rand    func() float64
}

// NewDriver builds a Driver, freezing the retry policy at construction
// (immutable thereafter). randFn is the jitter source r ∈ [0,1) fed to
// the policy's Backoff; nil uses the global math/rand source. Tests
// inject a fixed value for deterministic backoff.
func NewDriver(lg *session.Log, ad adapters.Adapter, opts Options, sleeper Sleeper, randFn func() float64) (*Driver, error) {
	if lg == nil {
		return nil, fmt.Errorf("loop: nil session log")
	}
	if ad == nil {
		return nil, fmt.Errorf("loop: nil adapter")
	}
	if sleeper == nil {
		return nil, fmt.Errorf("loop: nil sleeper")
	}
	policy, err := NewRetryPolicy(opts.Retry)
	if err != nil {
		return nil, err
	}
	if randFn == nil {
		randFn = rand.Float64
	}
	d := &Driver{lg: lg, ad: ad, policy: policy, sleeper: sleeper, opts: opts, rand: randFn}
	return d, nil
}

// RunResult reports one completed Run.
type RunResult struct {
	Response *adapters.Response
	// Attempts is the number of adapter calls made (1 = first try).
	Attempts int
}

// ExhaustedError is the typed terminal failure after the retry budget is
// spent. It wraps the last adapter error.
type ExhaustedError struct {
	Attempts int
	Last     error
}

func (e *ExhaustedError) Error() string {
	return fmt.Sprintf("loop: retries exhausted after %d attempts: %v", e.Attempts, e.Last)
}

func (e *ExhaustedError) Unwrap() error { return e.Last }

// RetryPayload is the log-only llm/retry record: the numbered attempt plus
// the immutable policy snapshot under which it was made, written BEFORE
// the backoff wait so the intent is durable even if the process dies
// mid-wait.
type RetryPayload struct {
	Attempt      int            `json:"attempt"`
	Policy       PolicySnapshot `json:"policy"`
	ErrorClass   string         `json:"errorClass"`
	ErrorMessage string         `json:"errorMessage,omitempty"`
	BackoffMs    int64          `json:"backoffMs"`
	RetryAfterMs int64          `json:"retryAfterMs,omitempty"`
}

// PolicySnapshot is the frozen policy stamped onto each llm/retry record.
type PolicySnapshot struct {
	MaxRetries       int     `json:"maxRetries"`
	BackoffInitialMs int64   `json:"backoffInitialMs"`
	BackoffMaxMs     int64   `json:"backoffMaxMs"`
	JitterFraction   float64 `json:"jitterFraction"`
}

// RetryStartedPayload is the log-only llm/retry-started record, appended
// after the backoff wait, right before the retry is actually dispatched.
type RetryStartedPayload struct {
	Attempt int `json:"attempt"`
}

// Run drives one durable turn: turn/begin, the user prompt, the adapter
// call (with numbered durable retries on retryable failures), and on
// success llm/response + turn/end. On terminal failure it appends
// turn/end {kind: error} and returns a typed error.
func (d *Driver) Run(ctx context.Context, prompt string) (*RunResult, error) {
	if _, err := d.lg.AppendTurnBegin(); err != nil {
		return nil, fmt.Errorf("loop: append turn/begin: %w", err)
	}
	if _, err := d.lg.AppendPrompt(prompt); err != nil {
		return nil, fmt.Errorf("loop: append prompt: %w", err)
	}

	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			_ = d.appendTurnEndError(fmt.Sprintf("context: %v", err))
			return nil, fmt.Errorf("loop: run canceled: %w", err)
		}

		// Fresh durable surface per attempt: the request is always derived
		// from the committed log, never from in-memory state.
		msgs, err := d.lg.Surface()
		if err != nil {
			return nil, fmt.Errorf("loop: derive surface: %w", err)
		}
		resp, callErr := d.ad.Call(ctx, &adapters.Request{
			Model:       d.opts.Model,
			Messages:    msgs,
			Tools:       d.opts.Tools,
			Temperature: d.opts.Temperature,
			MaxTokens:   d.opts.MaxTokens,
		})

		aerr := classify(callErr, d.ad.Name(), d.opts.Model, resp)
		if aerr == nil {
			model := resp.Model
			if model == "" {
				model = d.opts.Model
			}
			if _, err := d.lg.AppendLLMResponse(model, resp.Content, resp.ToolCalls, resp.Usage); err != nil {
				return nil, fmt.Errorf("loop: append llm/response: %w", err)
			}
			if _, err := d.lg.AppendTurnEndKind("", ""); err != nil {
				return nil, fmt.Errorf("loop: append turn/end: %w", err)
			}
			return &RunResult{Response: resp, Attempts: attempt + 1}, nil
		}

		if !aerr.Retryable() {
			// Non-retryable: immediate turn/end {kind: error}, no retry events.
			_ = d.appendTurnEndError(aerr.Error())
			return nil, aerr
		}
		if attempt >= d.policy.MaxRetries() {
			_ = d.appendTurnEndError(aerr.Error())
			return nil, &ExhaustedError{Attempts: attempt + 1, Last: aerr}
		}

		// Durable retry record BEFORE waiting (crash mid-wait still shows
		// the numbered intent), then the wait, then retry-started.
		backoff := d.policy.Backoff(attempt+1, aerr.RetryAfterMs, d.rand())
		if _, err := d.lg.Append(session.TypeLLMRetry, nil, RetryPayload{
			Attempt:      attempt + 1,
			Policy:       d.snapshot(),
			ErrorClass:   string(aerr.Kind),
			ErrorMessage: aerr.Error(),
			BackoffMs:    backoff.Milliseconds(),
			RetryAfterMs: aerr.RetryAfterMs,
		}); err != nil {
			return nil, fmt.Errorf("loop: append llm/retry: %w", err)
		}
		d.sleeper.Sleep(backoff)
		if _, err := d.lg.Append(session.TypeLLMRetryStarted, nil, RetryStartedPayload{Attempt: attempt + 1}); err != nil {
			return nil, fmt.Errorf("loop: append llm/retry-started: %w", err)
		}
	}
}

// Classify is the exported form of classify, shared by Driver.Run and
// the tools.Pipeline retry ladder (slice B1) so both fold raw call
// outcomes into the SAME typed AdapterError classification.
func Classify(callErr error, adapterName, model string, resp *adapters.Response) *adapters.AdapterError {
	return classify(callErr, adapterName, model, resp)
}

// SnapshotOf renders a frozen policy as the log-payload PolicySnapshot
// shape stamped onto each llm/retry record (Driver.snapshot, exported
// for the tools.Pipeline retry ladder).
func SnapshotOf(p *RetryPolicy) PolicySnapshot {
	s := p.Snapshot()
	return PolicySnapshot{
		MaxRetries:       s.MaxRetries,
		BackoffInitialMs: s.BackoffInitial.Milliseconds(),
		BackoffMaxMs:     s.BackoffMax.Milliseconds(),
		JitterFraction:   s.JitterFraction,
	}
}

// classify folds a raw call outcome into the typed AdapterError
// classification. A typed *AdapterError passes through (unwrapped); an
// untyped error is classified as transport (the common wire case); a
// "successful" response with neither content nor tool calls is an
// empty-response failure. A clean response yields nil.
func classify(callErr error, adapterName, model string, resp *adapters.Response) *adapters.AdapterError {
	if callErr != nil {
		var aerr *adapters.AdapterError
		if errors.As(callErr, &aerr) {
			return aerr
		}
		return adapters.TransportError(adapterName, callErr)
	}
	if resp == nil {
		return adapters.EmptyResponseError(adapterName, model)
	}
	if resp.Content == "" && len(resp.ToolCalls) == 0 {
		return adapters.EmptyResponseError(adapterName, model)
	}
	return nil
}

func (d *Driver) snapshot() PolicySnapshot {
	s := d.policy.Snapshot()
	return PolicySnapshot{
		MaxRetries:       s.MaxRetries,
		BackoffInitialMs: s.BackoffInitial.Milliseconds(),
		BackoffMaxMs:     s.BackoffMax.Milliseconds(),
		JitterFraction:   s.JitterFraction,
	}
}

func (d *Driver) appendTurnEndError(reason string) error {
	_, err := d.lg.AppendTurnEndKind(TurnEndKindError, reason)
	return err
}
