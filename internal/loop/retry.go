// Package loop implements the native engine's durable numbered-turn retry
// driver: the minimal turn loop that brackets one Run(prompt) in
// turn/begin … turn/end, and retries failed adapter calls as FRESH DURABLE
// TURNS — every retry first appends its llm/retry record (policy snapshot +
// attempt number) BEFORE waiting, then llm/retry-started once the backoff
// has elapsed, so replay shows numbered attempts.
//
// Crash-note (design contract, dsh C3 "durable-progress-as-retry-proof"):
// durable progress — any successful turn already appended to the session
// log — is the retry-proof precondition. Each retry re-derives the request
// surface from the durable log (never from in-memory state), so whatever
// committed between attempts is included; conversely a retry is only
// justified by committed change to the input. Slice 2 makes each retry
// re-derive the surface; the replaceGeneration-advance gate for
// context-overflow recovery is future wiring.
package loop

import (
	"fmt"
	"time"
)

// Retry defaults (dsh llm-retry semantics, simplified): at most 2 retries,
// exponential backoff 500ms doubling to a 10s cap, ±10% jitter, provider
// Retry-After honored when it fits under the cap.
const (
	DefaultMaxRetries     = 2
	DefaultBackoffInitial = 500 * time.Millisecond
	DefaultBackoffMax     = 10 * time.Second
	DefaultJitterFraction = 0.10
)

// RetryConfig is the caller-facing retry configuration; zero values take
// the defaults above.
type RetryConfig struct {
	MaxRetries     int           // retries after the initial attempt; default 2
	BackoffInitial time.Duration // wait before retry 1; default 500ms
	BackoffMax     time.Duration // backoff ceiling; default 10s
	JitterFraction float64       // ± fraction for jitter; default 0.10
}

// RetryPolicy is the IMMUTABLE-AT-CONSTRUCTION retry policy consulted by
// the driver. Build it once (NewRetryPolicy validates and applies
// defaults); it never mutates afterwards.
type RetryPolicy struct {
	cfg RetryConfig
}

// NewRetryPolicy validates cfg, applies defaults for zero fields, and
// returns the frozen policy.
func NewRetryPolicy(cfg RetryConfig) (*RetryPolicy, error) {
	if cfg.MaxRetries < 0 {
		return nil, fmt.Errorf("loop: RetryConfig.MaxRetries must be >= 0, got %d", cfg.MaxRetries)
	}
	if cfg.BackoffInitial < 0 {
		return nil, fmt.Errorf("loop: RetryConfig.BackoffInitial must be >= 0, got %v", cfg.BackoffInitial)
	}
	if cfg.BackoffMax < 0 {
		return nil, fmt.Errorf("loop: RetryConfig.BackoffMax must be >= 0, got %v", cfg.BackoffMax)
	}
	if cfg.JitterFraction < 0 || cfg.JitterFraction > 1 {
		return nil, fmt.Errorf("loop: RetryConfig.JitterFraction must be within [0,1], got %f", cfg.JitterFraction)
	}
	if cfg.BackoffInitial == 0 {
		cfg.BackoffInitial = DefaultBackoffInitial
	}
	if cfg.BackoffMax == 0 {
		cfg.BackoffMax = DefaultBackoffMax
	}
	if cfg.BackoffMax < cfg.BackoffInitial {
		return nil, fmt.Errorf("loop: RetryConfig.BackoffMax (%v) is below BackoffInitial (%v)", cfg.BackoffMax, cfg.BackoffInitial)
	}
	if cfg.JitterFraction == 0 {
		cfg.JitterFraction = DefaultJitterFraction
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = DefaultMaxRetries
	}
	return &RetryPolicy{cfg: cfg}, nil
}

// MaxRetries returns the configured retry count.
func (p *RetryPolicy) MaxRetries() int { return p.cfg.MaxRetries }

// Snapshot returns the frozen configuration (for logging on llm/retry).
func (p *RetryPolicy) Snapshot() RetryConfig { return p.cfg }

// Backoff computes the wait before retry `attempt` (1-based). The base
// doubles per attempt from BackoffInitial up to BackoffMax. A provider
// retry-after hint is honored only when it fits under the cap and exceeds
// the base; hints above the cap are ignored (never extend past 10s).
// r ∈ [0,1) drives the ±JitterFraction jitter; the result is clamped to
// the cap so jitter can never push past it.
func (p *RetryPolicy) Backoff(attempt int, retryAfterMs int64, r float64) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	base := p.cfg.BackoffInitial
	for i := 1; i < attempt && base < p.cfg.BackoffMax; i++ {
		base *= 2
	}
	if base > p.cfg.BackoffMax {
		base = p.cfg.BackoffMax
	}
	if retryAfterMs > 0 {
		hint := time.Duration(retryAfterMs) * time.Millisecond
		if hint <= p.cfg.BackoffMax && hint > base {
			base = hint
		}
	}
	d := time.Duration(float64(base) * (1 + (2*r-1)*p.cfg.JitterFraction))
	if d > p.cfg.BackoffMax {
		d = p.cfg.BackoffMax
	}
	if d < 0 {
		d = 0
	}
	return d
}

// Sleeper is the injected time interface: tests record sleeps instead of
// waiting, so backoff behavior is asserted deterministically and fast.
type Sleeper interface {
	Sleep(time.Duration)
}

// RealSleeper sleeps for real. It is the production default.
type RealSleeper struct{}

// Sleep blocks the calling goroutine for d.
func (RealSleeper) Sleep(d time.Duration) { time.Sleep(d) }
