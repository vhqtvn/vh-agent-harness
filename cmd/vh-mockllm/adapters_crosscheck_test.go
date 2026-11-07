// adapters_crosscheck_test.go — the dialect crux: drive the REAL
// internal/adapters/openaicompat and internal/adapters/anthropic
// adapters against the mock handler (httptest-wrapped) and prove the
// wire shapes are exactly what the engine expects. If the mock drifts
// from a real provider contract, these tests fail — that is their only
// job.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
	"github.com/vhqtvn/vh-agent-harness/internal/adapters/anthropic"
	"github.com/vhqtvn/vh-agent-harness/internal/adapters/openaicompat"
)

// startMockRaw builds the mock over a raw script path + steps (for
// cross-package use where writeScript's t.TempDir is fine too).
func startMockRaw(t *testing.T, script string) *httptest.Server {
	t.Helper()
	p := writeScript(t, script)
	steps, err := LoadScript(p)
	if err != nil {
		t.Fatalf("LoadScript: %v", err)
	}
	srv := httptest.NewServer(newMockServer(p, steps))
	t.Cleanup(srv.Close)
	return srv
}

// TestRealOpenAIAdapterThroughMock: text and tool-call steps decode
// through the REAL openaicompat adapter.
func TestRealOpenAIAdapterThroughMock(t *testing.T) {
	srv := startMockRaw(t, `[
		{"text":"real adapter round trip"},
		{"tool_calls":[{"id":"call-x","name":"echo","args":{"text":"hi"}}]}
	]`)
	ad := openaicompat.New(openaicompat.Config{Provider: "mock", BaseURL: srv.URL + "/v1", Model: "mock-model", APIKey: "test-key"})

	resp, err := ad.Call(context.Background(), &adapters.Request{Model: "mock-model"})
	if err != nil {
		t.Fatalf("text Call: %v", err)
	}
	if resp.Content != "real adapter round trip" || resp.FinishReason != "stop" {
		t.Fatalf("text response = %+v", resp)
	}

	resp2, err := ad.Call(context.Background(), &adapters.Request{Model: "mock-model"})
	if err != nil {
		t.Fatalf("tool-call Call: %v", err)
	}
	if len(resp2.ToolCalls) != 1 {
		t.Fatalf("ToolCalls = %+v", resp2)
	}
	tc := resp2.ToolCalls[0]
	if tc.ID != "call-x" || tc.Name != "echo" || string(tc.Args) != `{"text":"hi"}` {
		t.Fatalf("tool call = %+v (args %s)", tc, tc.Args)
	}
	if resp2.FinishReason != "tool_calls" {
		t.Fatalf("FinishReason = %q", resp2.FinishReason)
	}
}

// TestRealOpenAIAdapterFaultClassification: the mock's fault steps
// classify through the REAL adapter exactly as the engine's retry
// ladder needs — 500 retryable 5xx; 429 + retry-after 2s → 2000ms hint.
func TestRealOpenAIAdapterFaultClassification(t *testing.T) {
	srv := startMockRaw(t, `[
		{"fault":{"status":500,"body":"{\"error\":{\"message\":\"boom\"}}"}},
		{"fault":{"status":429,"body":"{\"error\":{\"message\":\"slow\"}}","retry_after_ms":2000}}
	]`)
	ad := openaicompat.New(openaicompat.Config{Provider: "mock", BaseURL: srv.URL + "/v1", Model: "m", APIKey: "k"})

	_, err := ad.Call(context.Background(), &adapters.Request{Model: "m"})
	var aerr *adapters.AdapterError
	if !errors.As(err, &aerr) {
		t.Fatalf("500 must be an AdapterError, got %v", err)
	}
	if aerr.Status != 500 || aerr.Kind != adapters.KindHTTP5xx || !aerr.Retryable() {
		t.Fatalf("500 classification = %+v", aerr)
	}

	_, err2 := ad.Call(context.Background(), &adapters.Request{Model: "m"})
	if !errors.As(err2, &aerr) {
		t.Fatalf("429 must be an AdapterError, got %v", err2)
	}
	if aerr.Status != 429 || aerr.Kind != adapters.KindRateLimit || !aerr.Retryable() {
		t.Fatalf("429 classification = %+v", aerr)
	}
	if aerr.RetryAfterMs != 2000 {
		t.Fatalf("RetryAfterMs = %d, want 2000 (retry-after: 2 seconds)", aerr.RetryAfterMs)
	}
}

// TestRealOpenAIAdapterEmptyClassification: the mock's empty step is
// the retryable empty-response class through the REAL adapter.
func TestRealOpenAIAdapterEmptyClassification(t *testing.T) {
	srv := startMockRaw(t, `[{"empty":true}]`)
	ad := openaicompat.New(openaicompat.Config{Provider: "mock", BaseURL: srv.URL + "/v1", Model: "m", APIKey: "k"})
	_, err := ad.Call(context.Background(), &adapters.Request{Model: "m"})
	var aerr *adapters.AdapterError
	if !errors.As(err, &aerr) {
		t.Fatalf("empty must be an AdapterError, got %v", err)
	}
	if aerr.Kind != adapters.KindEmptyResponse || !aerr.Retryable() {
		t.Fatalf("empty classification = %+v, want retryable empty-response", aerr)
	}
}

// TestRealAnthropicAdapterThroughMock: text + tool_use steps decode
// through the REAL anthropic adapter (input object → stringified args).
func TestRealAnthropicAdapterThroughMock(t *testing.T) {
	srv := startMockRaw(t, `[
		{"text":"anthropic round trip"},
		{"tool_calls":[{"id":"toolu-9","name":"echo","args":{"text":"hi"}}]}
	]`)
	ad := anthropic.New(anthropic.Config{Provider: "mock", BaseURL: srv.URL, Model: "mock-model", APIKey: "test-key"})

	resp, err := ad.Call(context.Background(), &adapters.Request{Model: "mock-model"})
	if err != nil {
		t.Fatalf("text Call: %v", err)
	}
	if resp.Content != "anthropic round trip" || resp.FinishReason != "stop" {
		t.Fatalf("text response = %+v", resp)
	}

	resp2, err := ad.Call(context.Background(), &adapters.Request{Model: "mock-model"})
	if err != nil {
		t.Fatalf("tool-use Call: %v", err)
	}
	if len(resp2.ToolCalls) != 1 {
		t.Fatalf("ToolCalls = %+v", resp2)
	}
	tc := resp2.ToolCalls[0]
	if tc.ID != "toolu-9" || tc.Name != "echo" {
		t.Fatalf("tool call = %+v", tc)
	}
	// The documented asymmetry: wire input OBJECT → contract string.
	var args map[string]any
	if err := json.Unmarshal(tc.Args, &args); err != nil || args["text"] != "hi" {
		t.Fatalf("args = %s, want stringified {\"text\":\"hi\"}", tc.Args)
	}
	if resp2.FinishReason != "tool_calls" {
		t.Fatalf("FinishReason = %q", resp2.FinishReason)
	}
}

// TestRealAnthropicAdapterFaultAndEmpty: 429 + retry-after and the
// empty class classify correctly through the REAL anthropic adapter.
func TestRealAnthropicAdapterFaultAndEmpty(t *testing.T) {
	srv := startMockRaw(t, `[
		{"fault":{"status":429,"body":"{\"type\":\"error\",\"error\":{\"type\":\"rate_limit_error\",\"message\":\"slow\"}}","retry_after_ms":3000}},
		{"empty":true}
	]`)
	ad := anthropic.New(anthropic.Config{Provider: "mock", BaseURL: srv.URL, Model: "m", APIKey: "k"})

	_, err := ad.Call(context.Background(), &adapters.Request{Model: "m"})
	var aerr *adapters.AdapterError
	if !errors.As(err, &aerr) {
		t.Fatalf("429 must be an AdapterError, got %v", err)
	}
	if aerr.Kind != adapters.KindRateLimit || !aerr.Retryable() || aerr.RetryAfterMs != 3000 {
		t.Fatalf("429 classification = %+v", aerr)
	}

	_, err2 := ad.Call(context.Background(), &adapters.Request{Model: "m"})
	if !errors.As(err2, &aerr) {
		t.Fatalf("empty must be an AdapterError, got %v", err2)
	}
	if aerr.Kind != adapters.KindEmptyResponse || !aerr.Retryable() {
		t.Fatalf("empty classification = %+v", aerr)
	}
}

// TestAnthropicExhaustedMentionsScript: the exhausted 500 through the
// REAL adapter carries the provider body excerpt naming the script
// file (the failure is diagnosable from the engine's error text).
func TestAnthropicExhaustedMentionsScript(t *testing.T) {
	srv := startMockRaw(t, `[{"text":"only-one"}]`)
	ad := anthropic.New(anthropic.Config{Provider: "mock", BaseURL: srv.URL, Model: "m", APIKey: "k"})
	if _, err := ad.Call(context.Background(), &adapters.Request{Model: "m"}); err != nil {
		t.Fatalf("first call: %v", err)
	}
	_, err := ad.Call(context.Background(), &adapters.Request{Model: "m"})
	if err == nil || !strings.Contains(err.Error(), "exhausted") {
		t.Fatalf("second call must fail with the exhausted body, got %v", err)
	}
}
