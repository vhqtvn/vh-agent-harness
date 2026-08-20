// Package adapters — typed error classification.
//
// Retry never lives inside an adapter; the retry layer (internal/loop)
// consults the classification an adapter reports through AdapterError.
package adapters

import (
	"fmt"
)

// ErrorKind is the closed classification of a failed adapter call. The
// retryable classes are: transport (wire-level failure), http5xx (server
// error), ratelimit (429), timeout, and empty-response (a 200 whose body
// carries neither content nor tool calls). Everything else — auth errors,
// 4xx bad requests, decode failures — is KindOther and NOT retryable:
// retrying a deterministic failure can only burn budget.
type ErrorKind string

const (
	KindTransport     ErrorKind = "transport"
	KindHTTP5xx       ErrorKind = "http5xx"
	KindRateLimit     ErrorKind = "ratelimit"
	KindTimeout       ErrorKind = "timeout"
	KindEmptyResponse ErrorKind = "empty-response"
	KindOther         ErrorKind = "other"
)

// retryableKinds is the retry classification table (dsh llm-retry
// semantics, simplified for slice 2).
var retryableKinds = map[ErrorKind]bool{
	KindTransport:     true,
	KindHTTP5xx:       true,
	KindRateLimit:     true,
	KindTimeout:       true,
	KindEmptyResponse: true,
}

// AdapterError is the typed failure a provider adapter (or the driver,
// for empty responses) reports. It carries the retry classification plus
// the provider's Retry-After hint in milliseconds when one was seen.
type AdapterError struct {
	Adapter      string
	Kind         ErrorKind
	Status       int
	RetryAfterMs int64
	Err          error
}

// NewAdapterError builds a typed adapter failure.
func NewAdapterError(adapter string, kind ErrorKind, status int, retryAfterMs int64, err error) *AdapterError {
	return &AdapterError{Adapter: adapter, Kind: kind, Status: status, RetryAfterMs: retryAfterMs, Err: err}
}

// TransportError classifies a wire-level failure (connection refused,
// reset, DNS, TLS...).
func TransportError(adapter string, err error) *AdapterError {
	return NewAdapterError(adapter, KindTransport, 0, 0, err)
}

// TimeoutError classifies a deadline/timeout failure.
func TimeoutError(adapter string, err error) *AdapterError {
	return NewAdapterError(adapter, KindTimeout, 0, 0, err)
}

// EmptyResponseError classifies a successful transport whose response
// carries neither content nor tool calls.
func EmptyResponseError(adapter, model string) *AdapterError {
	return NewAdapterError(adapter, KindEmptyResponse, 0, 0, fmt.Errorf("%s: model %q returned an empty response", adapter, model))
}

// ClassifyHTTPStatus maps an HTTP status onto the error classification:
// 429 is a rate limit, any 5xx is a server error, everything else is
// non-retryable KindOther.
func ClassifyHTTPStatus(status int) ErrorKind {
	switch {
	case status == 429:
		return KindRateLimit
	case status >= 500 && status < 600:
		return KindHTTP5xx
	default:
		return KindOther
	}
}

// HTTPStatusError builds the typed failure for a non-2xx status, carrying
// the provider's Retry-After hint when supplied (milliseconds).
func HTTPStatusError(adapter string, status int, retryAfterMs int64, body string) *AdapterError {
	return NewAdapterError(adapter, ClassifyHTTPStatus(status), status, retryAfterMs, fmt.Errorf("%s: HTTP %d: %s", adapter, status, body))
}

// Retryable reports whether the retry layer may retry this class.
func (e *AdapterError) Retryable() bool { return retryableKinds[e.Kind] }

func (e *AdapterError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("adapters: [%s/%s] %v", e.Adapter, e.Kind, e.Err)
	}
	return fmt.Sprintf("adapters: [%s/%s]", e.Adapter, e.Kind)
}

func (e *AdapterError) Unwrap() error { return e.Err }
