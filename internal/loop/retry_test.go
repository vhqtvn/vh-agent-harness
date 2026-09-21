package loop

import (
	"testing"
	"time"
)

func TestNewRetryPolicyDefaultsAndValidation(t *testing.T) {
	p, err := NewRetryPolicy(RetryConfig{})
	if err != nil {
		t.Fatalf("zero config must take defaults: %v", err)
	}
	if p.MaxRetries() != 2 {
		t.Fatalf("default MaxRetries = %d, want 2", p.MaxRetries())
	}
	snap := p.Snapshot()
	if snap.BackoffInitial != 500*time.Millisecond {
		t.Fatalf("default BackoffInitial = %v, want 500ms", snap.BackoffInitial)
	}
	if snap.BackoffMax != 10*time.Second {
		t.Fatalf("default BackoffMax = %v, want 10s", snap.BackoffMax)
	}
	if snap.JitterFraction != 0.10 {
		t.Fatalf("default JitterFraction = %v, want 0.10", snap.JitterFraction)
	}

	bad := []RetryConfig{
		{MaxRetries: -1},
		{MaxRetries: 1, BackoffInitial: -time.Second},
		{MaxRetries: 1, BackoffInitial: 20 * time.Second, BackoffMax: time.Second},
		{MaxRetries: 1, JitterFraction: -0.1},
		{MaxRetries: 1, JitterFraction: 1.5},
	}
	for i, cfg := range bad {
		if _, err := NewRetryPolicy(cfg); err == nil {
			t.Fatalf("case %d: expected validation error for %+v", i, cfg)
		}
	}
}

func TestRetryPolicyBackoffMath(t *testing.T) {
	p, err := NewRetryPolicy(RetryConfig{})
	if err != nil {
		t.Fatalf("NewRetryPolicy: %v", err)
	}
	// r = 0.5 makes the ±10% jitter factor exactly 1.0, so the base is exact.
	cases := []struct {
		attempt     int
		retryAfter  int64
		r           float64
		want        time.Duration
		description string
	}{
		{1, 0, 0.5, 500 * time.Millisecond, "first retry waits the initial backoff"},
		{2, 0, 0.5, time.Second, "second retry doubles"},
		{3, 0, 0.5, 2 * time.Second, "third retry doubles again"},
		{4, 0, 0.5, 4 * time.Second, "fourth retry"},
		{5, 0, 0.5, 8 * time.Second, "fifth retry"},
		{6, 0, 0.5, 10 * time.Second, "exponential growth caps at BackoffMax"},
		{20, 0, 0.5, 10 * time.Second, "cap holds at extreme attempt counts"},
		{1, 3000, 0.5, 3 * time.Second, "provider retryAfter honored when <= cap"},
		{2, 3000, 0.5, 3 * time.Second, "retryAfter beats the doubled base when larger"},
		{1, 30000, 0.5, 500 * time.Millisecond, "retryAfter above the cap is ignored"},
		{1, 0, 0.0, 450 * time.Millisecond, "jitter floor: r=0 shrinks 10%"},
		{1, 0, 1.0, 550 * time.Millisecond, "jitter ceiling: r=1 grows 10%"},
		{6, 0, 1.0, 10 * time.Second, "jitter can never push past the cap"},
	}
	for _, tc := range cases {
		got := p.Backoff(tc.attempt, tc.retryAfter, tc.r)
		if got != tc.want {
			t.Fatalf("%s: Backoff(%d, %d, %f) = %v, want %v", tc.description, tc.attempt, tc.retryAfter, tc.r, got, tc.want)
		}
	}
}
