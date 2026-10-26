// Cross-package parity: the typed errors openaicompat returns must fold
// through loop.Classify — the exact classification the Driver's retry
// decision and the tools.Pipeline ladder consult — without ever falling
// into the untyped→transport fold. This is the B1 hotfix seam: before the
// typing, a 401 classified as transport (retryable) and burned the full
// retry ladder with backoff delays against a deterministic auth failure.
package openaicompat_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
	"github.com/vhqtvn/vh-agent-harness/internal/adapters/openaicompat"
	"github.com/vhqtvn/vh-agent-harness/internal/loop"
)

// statusServer is a one-shot server replying with a fixed status,
// headers, and body.
func statusServer(t *testing.T, status int, headers map[string]string, body string) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// CRUX half 1 — 401 is NOT retryable through the loop's own Classify: the
// typed HTTPStatusError passes through unwrapped as non-retryable
// KindOther instead of being folded into retryable transport.
func TestClassify401NonRetryableThroughLoop(t *testing.T) {
	srv := statusServer(t, http.StatusUnauthorized, nil, `{"error":{"message":"invalid api key"}}`)
	ad := openaicompat.New(openaicompat.Config{Provider: "mock", BaseURL: srv.URL, Model: "m", APIKey: "bad"})
	_, callErr := ad.Call(context.Background(), &adapters.Request{Model: "m"})
	if callErr == nil {
		t.Fatal("expected an error on 401")
	}
	aerr := loop.Classify(callErr, ad.Name(), "m", nil)
	if aerr == nil {
		t.Fatal("Classify returned nil for a failed call")
	}
	if aerr.Kind == adapters.KindTransport {
		t.Fatalf("401 folded into transport (the B1 defect): %+v", aerr)
	}
	if aerr.Kind != adapters.KindOther || aerr.Status != 401 || aerr.Retryable() {
		t.Fatalf("401 through loop.Classify = %+v, want non-retryable KindOther/401", aerr)
	}
}

// CRUX half 2 — 429 + retry-after is retryable through loop.Classify AND
// the hint is honored by the loop's own backoff policy (RetryAfterMs
// survives Classify and lifts Backoff above the 500ms base).
func TestClassify429RetryAfterHonoredThroughLoop(t *testing.T) {
	srv := statusServer(t, http.StatusTooManyRequests,
		map[string]string{"retry-after": "2"},
		`{"error":{"message":"rate limit reached"}}`)
	ad := openaicompat.New(openaicompat.Config{Provider: "mock", BaseURL: srv.URL, Model: "m", APIKey: "k"})
	_, callErr := ad.Call(context.Background(), &adapters.Request{Model: "m"})
	if callErr == nil {
		t.Fatal("expected an error on 429")
	}
	aerr := loop.Classify(callErr, ad.Name(), "m", nil)
	if aerr == nil {
		t.Fatal("Classify returned nil for a failed call")
	}
	if aerr.Kind != adapters.KindRateLimit || !aerr.Retryable() {
		t.Fatalf("429 through loop.Classify = %+v, want retryable rate limit", aerr)
	}
	if aerr.RetryAfterMs != 2000 {
		t.Fatalf("RetryAfterMs = %d, want 2000 (retry-after: 2 seconds)", aerr.RetryAfterMs)
	}
	policy, err := loop.NewRetryPolicy(loop.RetryConfig{})
	if err != nil {
		t.Fatalf("NewRetryPolicy: %v", err)
	}
	// r=0.5 is the jitter-neutral point ((2r−1)=0); the 2000ms hint
	// (≤ 10s cap, > 500ms base) must become the wait itself — the
	// honored-hint proof.
	if got := policy.Backoff(1, aerr.RetryAfterMs, 0.5); got != 2000*time.Millisecond {
		t.Fatalf("Backoff with honored hint = %v, want 2s", got)
	}
	// Without the hint the same attempt waits the 500ms base — proving
	// the hint (not the policy) lifted the wait.
	if got := policy.Backoff(1, 0, 0.5); got != 500*time.Millisecond {
		t.Fatalf("Backoff without hint = %v, want 500ms base", got)
	}
}
