package adapters

import (
	"errors"
	"fmt"
	"testing"
)

func TestAdapterErrorRetryableClassification(t *testing.T) {
	cases := []struct {
		kind      ErrorKind
		retryable bool
	}{
		{KindTransport, true},
		{KindHTTP5xx, true},
		{KindRateLimit, true},
		{KindTimeout, true},
		{KindEmptyResponse, true},
		{KindOther, false},
	}
	for _, tc := range cases {
		err := NewAdapterError("prov", tc.kind, 0, 0, errors.New("boom"))
		if got := err.Retryable(); got != tc.retryable {
			t.Fatalf("kind %q: Retryable() = %v, want %v", tc.kind, got, tc.retryable)
		}
	}
}

func TestClassifyHTTPStatus(t *testing.T) {
	cases := []struct {
		status int
		want   ErrorKind
	}{
		{429, KindRateLimit},
		{500, KindHTTP5xx},
		{502, KindHTTP5xx},
		{503, KindHTTP5xx},
		{400, KindOther},
		{401, KindOther},
		{404, KindOther},
		{418, KindOther},
		{200, KindOther},
	}
	for _, tc := range cases {
		if got := ClassifyHTTPStatus(tc.status); got != tc.want {
			t.Fatalf("ClassifyHTTPStatus(%d) = %q, want %q", tc.status, got, tc.want)
		}
	}
}

func TestAdapterErrorWrappingAndAs(t *testing.T) {
	root := errors.New("connection reset")
	aerr := TransportError("prov-x", root)
	if !errors.Is(aerr, root) {
		t.Fatal("errors.Is must see through AdapterError to the wrapped cause")
	}
	wrapped := fmt.Errorf("call failed: %w", aerr)
	var out *AdapterError
	if !errors.As(wrapped, &out) {
		t.Fatal("errors.As must recover the AdapterError through wrapping")
	}
	if out.Adapter != "prov-x" || out.Kind != KindTransport || !out.Retryable() {
		t.Fatalf("recovered AdapterError = %+v", out)
	}
	if aerr.Error() == "" {
		t.Fatal("Error() must be non-empty")
	}
}

func TestHTTPStatusErrorCarriesStatusAndRetryAfter(t *testing.T) {
	aerr := HTTPStatusError("prov-y", 503, 1200, "slow down")
	if aerr.Kind != KindHTTP5xx || aerr.Status != 503 || aerr.RetryAfterMs != 1200 || !aerr.Retryable() {
		t.Fatalf("HTTPStatusError fields = %+v", aerr)
	}
	rl := HTTPStatusError("prov-y", 429, 0, "too many")
	if rl.Kind != KindRateLimit || !rl.Retryable() {
		t.Fatalf("429 classification = %+v", rl)
	}
}

func TestEmptyResponseErrorIsRetryable(t *testing.T) {
	aerr := EmptyResponseError("prov-z", "mock-1")
	if aerr.Kind != KindEmptyResponse || !aerr.Retryable() {
		t.Fatalf("EmptyResponseError = %+v", aerr)
	}
}

func TestRedactSecret(t *testing.T) {
	const key = "sk-live-0123456789abcdef"
	cases := []struct {
		name   string
		s      string
		secret string
		want   string
	}{
		{
			name:   "every occurrence redacted",
			s:      `unauthorized: key sk-live-0123456789abcdef echoed twice: sk-live-0123456789abcdef`,
			secret: key,
			want:   "unauthorized: key [REDACTED] echoed twice: [REDACTED]",
		},
		{
			name:   "absent secret unchanged",
			s:      "plain provider error, no credential here",
			secret: key,
			want:   "plain provider error, no credential here",
		},
		{
			name:   "short secret left alone (threshold guard)",
			s:      "bad token abc1234 end",
			secret: "abc1234",
			want:   "bad token abc1234 end",
		},
		{
			name:   "exactly-at-threshold secret redacted",
			s:      "bad token abc12345 end",
			secret: "abc12345",
			want:   "bad token [REDACTED] end",
		},
		{
			name:   "empty secret unchanged",
			s:      "nothing to redact",
			secret: "",
			want:   "nothing to redact",
		},
		{
			name:   "substring matches redacted; near-miss prefix alone is not",
			s:      "sk-live-0123456789abcdefPLUS and sk-live-0123456789abcde",
			secret: key,
			want:   "[REDACTED]PLUS and sk-live-0123456789abcde",
		},
	}
	for _, tc := range cases {
		if got := RedactSecret(tc.s, tc.secret); got != tc.want {
			t.Fatalf("%s: RedactSecret = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// compile-time: the fake adapter from adapter_test.go still satisfies the
// boundary after the errors file lands.
var _ Adapter = fakeAdapter{name: "fake"}
