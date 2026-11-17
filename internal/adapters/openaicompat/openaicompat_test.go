package openaicompat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
)

// capturedRequest records what the adapter actually put on the wire.
type capturedRequest struct {
	Method string
	Path   string
	Auth   string
	Body   []byte
}

// scriptedServer is a deterministic mock OpenAI-compatible server (the
// dsh llm-mock-server pattern): it serves scripted JSON responses in
// order and records every request. No real network is involved.
type scriptedServer struct {
	*httptest.Server
	mu        sync.Mutex
	responses []string
	captured  []capturedRequest
}

func newScriptedServer(t *testing.T, apiKey string, responses ...string) *scriptedServer {
	s := &scriptedServer{responses: responses}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("mock server: read body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		s.captured = append(s.captured, capturedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Auth:   r.Header.Get("Authorization"),
			Body:   body,
		})
		if len(s.captured) > len(s.responses) {
			t.Errorf("mock server: unexpected request #%d (only %d scripted)", len(s.captured), len(s.responses))
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(s.responses[len(s.captured)-1]))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *scriptedServer) requests() []capturedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]capturedRequest, len(s.captured))
	copy(out, s.captured)
	return out
}

func decodeBody(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("request body is not JSON: %v\n%s", err, raw)
	}
	return m
}

const textResponseScript = `{"id":"chatcmpl-2","model":"mock-1","choices":[{"index":0,"message":{"role":"assistant","content":"done: hello echoed"},"finish_reason":"stop"}],"usage":{"prompt_tokens":30,"completion_tokens":8,"total_tokens":38}}`

const toolCallResponseScript = `{"id":"chatcmpl-1","model":"mock-1","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"echo","arguments":"{\"text\":\"hello\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`

func TestCallPlainTextResponse(t *testing.T) {
	srv := newScriptedServer(t, "test-key", textResponseScript)
	ad := New(Config{Provider: "mock", BaseURL: srv.URL, Model: "mock-1", APIKey: "test-key"})
	temp := 0.2
	resp, err := ad.Call(context.Background(), &adapters.Request{
		// Model intentionally empty: the adapter must fall back to Config.Model.
		Messages:    []adapters.Message{{Role: "user", Content: "hi"}},
		Tools:       []adapters.ToolSpec{{Name: "echo", Description: "echo it", Parameters: json.RawMessage(`{"type":"object"}`)}},
		Temperature: &temp,
		MaxTokens:   128,
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Content != "done: hello echoed" {
		t.Fatalf("Content = %q", resp.Content)
	}
	if resp.FinishReason != "stop" {
		t.Fatalf("FinishReason = %q, want stop", resp.FinishReason)
	}
	if resp.Usage != (adapters.Usage{PromptTokens: 30, CompletionTokens: 8, TotalTokens: 38}) {
		t.Fatalf("Usage = %+v", resp.Usage)
	}
	reqs := srv.requests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}
	r0 := reqs[0]
	if r0.Method != http.MethodPost || r0.Path != "/chat/completions" {
		t.Fatalf("request = %s %s, want POST /chat/completions", r0.Method, r0.Path)
	}
	if r0.Auth != "Bearer test-key" {
		t.Fatalf("Authorization = %q", r0.Auth)
	}
	body := decodeBody(t, r0.Body)
	if body["model"] != "mock-1" {
		t.Fatalf("wire model = %v, want Config.Model fallback", body["model"])
	}
	if body["temperature"] != 0.2 {
		t.Fatalf("wire temperature = %v", body["temperature"])
	}
	if body["max_tokens"] != float64(128) {
		t.Fatalf("wire max_tokens = %v", body["max_tokens"])
	}
	msgs := body["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("wire messages = %v", msgs)
	}
	m0 := msgs[0].(map[string]any)
	if m0["role"] != "user" || m0["content"] != "hi" {
		t.Fatalf("wire message[0] = %v", m0)
	}
	if _, has := m0["tool_call_id"]; has {
		t.Fatal("user message must not carry tool_call_id")
	}
	tools := body["tools"].([]any)
	t0 := tools[0].(map[string]any)
	if t0["type"] != "function" {
		t.Fatalf("wire tool type = %v", t0["type"])
	}
	fn := t0["function"].(map[string]any)
	if fn["name"] != "echo" || fn["description"] != "echo it" {
		t.Fatalf("wire tool function = %v", fn)
	}
	params := fn["parameters"].(map[string]any)
	if params["type"] != "object" {
		t.Fatalf("wire tool parameters = %v", params)
	}
}

func TestCallToolCallResponse(t *testing.T) {
	srv := newScriptedServer(t, "k", toolCallResponseScript)
	ad := New(Config{Provider: "mock", BaseURL: srv.URL, Model: "mock-1", APIKey: "k"})
	resp, err := ad.Call(context.Background(), &adapters.Request{
		Model:    "mock-1",
		Messages: []adapters.Message{{Role: "user", Content: "echo hello"}},
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("ToolCalls = %+v", resp.ToolCalls)
	}
	tc := resp.ToolCalls[0]
	if tc.ID != "call_1" || tc.Name != "echo" {
		t.Fatalf("tool call = %+v", tc)
	}
	if string(tc.Args) != `{"text":"hello"}` {
		t.Fatalf("tool call args = %s", tc.Args)
	}
	if resp.FinishReason != "tool_calls" {
		t.Fatalf("FinishReason = %q", resp.FinishReason)
	}
}

func TestCallSerializesAssistantToolCallsAndToolResults(t *testing.T) {
	srv := newScriptedServer(t, "k", textResponseScript)
	ad := New(Config{Provider: "mock", BaseURL: srv.URL, Model: "mock-1", APIKey: "k"})
	_, err := ad.Call(context.Background(), &adapters.Request{
		Model: "mock-1",
		Messages: []adapters.Message{
			{Role: "user", Content: "echo hello"},
			{Role: "assistant", Content: "", ToolCalls: []adapters.ToolCall{{
				ID:   "call_1",
				Name: "echo",
				Args: json.RawMessage(`{"text":"hello"}`),
			}}},
			{Role: "tool", ToolCallID: "call_1", Name: "echo", Content: `{"text":"hello"}`},
		},
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	body := decodeBody(t, srv.requests()[0].Body)
	msgs := body["messages"].([]any)
	if len(msgs) != 3 {
		t.Fatalf("wire messages = %v", msgs)
	}
	assistant := msgs[1].(map[string]any)
	if content, ok := assistant["content"]; !ok || content != "" {
		t.Fatalf("assistant content must be the string \"\" (never null); got %v (present=%v)", content, ok)
	}
	calls := assistant["tool_calls"].([]any)
	c0 := calls[0].(map[string]any)
	fn := c0["function"].(map[string]any)
	// OpenAI wire shape: arguments is a JSON *string*.
	if fn["arguments"] != `{"text":"hello"}` {
		t.Fatalf("wire tool arguments = %v (%T), want string", fn["arguments"], fn["arguments"])
	}
	toolMsg := msgs[2].(map[string]any)
	if toolMsg["role"] != "tool" || toolMsg["tool_call_id"] != "call_1" {
		t.Fatalf("wire tool message = %v", toolMsg)
	}
	if toolMsg["content"] != `{"text":"hello"}` {
		t.Fatalf("wire tool content = %v", toolMsg["content"])
	}
}

func TestCallHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"boom"}}`))
	}))
	defer srv.Close()
	ad := New(Config{Provider: "mock", BaseURL: srv.URL, Model: "m", APIKey: "k"})
	_, err := ad.Call(context.Background(), &adapters.Request{Model: "m"})
	if err == nil {
		t.Fatal("expected an error on HTTP 500")
	}
	if !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "mock") {
		t.Fatalf("error should mention status and provider: %v", err)
	}
}

// TestCallHTTPErrorBodyRedactsAPIKey is the finding-2 adversarial crux
// at the adapter seam: a hostile/broken provider that ECHOES the API-key
// value in a non-2xx error body must never get that value into the
// AdapterError text — every occurrence is replaced by [REDACTED] at the
// source (the excerpt-capture site), while the rest of the body excerpt
// survives unchanged.
func TestCallHTTPErrorBodyRedactsAPIKey(t *testing.T) {
	const key = "sk-live-0123456789abcdef"
	body := `{"error":{"message":"unauthorized: key sk-live-0123456789abcdef rejected for tenant-7734 (decoy, not a credential); echo #2: sk-live-0123456789abcdef"}}`
	srv := newStatusServer(t, http.StatusInternalServerError, nil, body)
	ad := New(Config{Provider: "mock", BaseURL: srv.URL, Model: "m", APIKey: key})
	_, err := ad.Call(context.Background(), &adapters.Request{Model: "m"})
	if err == nil {
		t.Fatal("expected an error on HTTP 500")
	}
	msg := err.Error()
	if strings.Contains(msg, key) {
		t.Fatalf("API-key value leaked through the adapter error: %s", msg)
	}
	if !strings.Contains(msg, "[REDACTED]") {
		t.Fatalf("error text should carry the redaction marker: %s", msg)
	}
	if !strings.Contains(msg, "tenant-7734") {
		t.Fatalf("non-key body excerpt should survive redaction: %s", msg)
	}
}

// newStatusServer is a one-shot server replying with a fixed status,
// headers, and body — the fixture for non-2xx classification tests.
func newStatusServer(t *testing.T, status int, headers map[string]string, body string) *httptest.Server {
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

// asAdapterError asserts the error is a typed *adapters.AdapterError and
// returns it.
func asAdapterError(t *testing.T, err error) *adapters.AdapterError {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error")
	}
	var aerr *adapters.AdapterError
	if !errors.As(err, &aerr) {
		t.Fatalf("expected *adapters.AdapterError, got %T: %v", err, err)
	}
	return aerr
}

// The B1 crux: an auth failure (401) must classify NON-retryable — never
// folded into transport, or the retry ladder would burn its full budget
// (with backoff delays) against a deterministic credential failure.
func TestCallAuth401NonRetryable(t *testing.T) {
	srv := newStatusServer(t, http.StatusUnauthorized, nil, `{"error":{"message":"invalid api key","type":"invalid_request_error"}}`)
	ad := New(Config{Provider: "mock", BaseURL: srv.URL, Model: "m", APIKey: "bad"})
	_, err := ad.Call(context.Background(), &adapters.Request{Model: "m"})
	aerr := asAdapterError(t, err)
	if aerr.Status != 401 || aerr.Kind != adapters.KindOther {
		t.Fatalf("401 classification = %+v, want status 401 / KindOther", aerr)
	}
	if aerr.Retryable() {
		t.Fatalf("401 must NOT be retryable: %+v", aerr)
	}
	if !strings.Contains(err.Error(), "invalid api key") {
		t.Fatalf("error should carry the provider body excerpt: %v", err)
	}
}

func TestCallHTTP404NonRetryable(t *testing.T) {
	srv := newStatusServer(t, http.StatusNotFound, nil, `{"error":{"message":"model not found"}}`)
	ad := New(Config{Provider: "mock", BaseURL: srv.URL, Model: "m", APIKey: "k"})
	_, err := ad.Call(context.Background(), &adapters.Request{Model: "m"})
	aerr := asAdapterError(t, err)
	if aerr.Status != 404 || aerr.Kind != adapters.KindOther || aerr.Retryable() {
		t.Fatalf("404 classification = %+v, want non-retryable KindOther", aerr)
	}
}

func TestCallHTTP500RetryableServerError(t *testing.T) {
	srv := newStatusServer(t, http.StatusInternalServerError, nil, `{"error":{"message":"boom"}}`)
	ad := New(Config{Provider: "mock", BaseURL: srv.URL, Model: "m", APIKey: "k"})
	_, err := ad.Call(context.Background(), &adapters.Request{Model: "m"})
	aerr := asAdapterError(t, err)
	if aerr.Status != 500 || aerr.Kind != adapters.KindHTTP5xx || !aerr.Retryable() {
		t.Fatalf("500 classification = %+v, want retryable http5xx", aerr)
	}
	if aerr.RetryAfterMs != 0 {
		t.Fatalf("RetryAfterMs = %d, want 0 when header absent", aerr.RetryAfterMs)
	}
}

func TestCallRateLimited429CarriesRetryAfter(t *testing.T) {
	srv := newStatusServer(t, http.StatusTooManyRequests,
		map[string]string{"retry-after": "2"},
		`{"error":{"message":"rate limit reached","type":"rate_limit_error"}}`)
	ad := New(Config{Provider: "mock", BaseURL: srv.URL, Model: "m", APIKey: "k"})
	_, err := ad.Call(context.Background(), &adapters.Request{Model: "m"})
	aerr := asAdapterError(t, err)
	if aerr.Status != 429 || aerr.Kind != adapters.KindRateLimit || !aerr.Retryable() {
		t.Fatalf("429 classification = %+v, want retryable rate limit", aerr)
	}
	if aerr.RetryAfterMs != 2000 {
		t.Fatalf("RetryAfterMs = %d, want 2000 (retry-after: 2 seconds)", aerr.RetryAfterMs)
	}
}

// The HTTP-date form of retry-after (some OpenAI-compatible gateways send
// it) must parse to a positive millisecond hint, not 0.
func TestCallRateLimited429HTTPDateRetryAfter(t *testing.T) {
	at := time.Now().UTC().Add(5 * time.Second).Format(http.TimeFormat)
	srv := newStatusServer(t, http.StatusTooManyRequests,
		map[string]string{"retry-after": at},
		`{"error":{"message":"slow down"}}`)
	ad := New(Config{Provider: "mock", BaseURL: srv.URL, Model: "m", APIKey: "k"})
	_, err := ad.Call(context.Background(), &adapters.Request{Model: "m"})
	aerr := asAdapterError(t, err)
	if aerr.Kind != adapters.KindRateLimit || !aerr.Retryable() {
		t.Fatalf("429 classification = %+v, want retryable rate limit", aerr)
	}
	if aerr.RetryAfterMs <= 0 || aerr.RetryAfterMs > 5000 {
		t.Fatalf("RetryAfterMs = %d, want within (0, 5000] for a 5s-ahead HTTP-date (%q)", aerr.RetryAfterMs, at)
	}
}

// Transport failures classify distinctly from timeouts: connection
// refused is KindTransport; a context deadline is KindTimeout. Both are
// retryable, but the kinds must never be confused.
func TestCallTransportRefusedIsKindTransport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	srv.Close() // port now refuses connections
	ad := New(Config{Provider: "mock", BaseURL: srv.URL, Model: "m", APIKey: "k"})
	_, err := ad.Call(context.Background(), &adapters.Request{Model: "m"})
	aerr := asAdapterError(t, err)
	if aerr.Kind != adapters.KindTransport || !aerr.Retryable() {
		t.Fatalf("connection-refused classification = %+v, want retryable KindTransport", aerr)
	}
}

func TestCallContextDeadlineIsKindTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(textResponseScript))
	}))
	defer srv.Close()
	ad := New(Config{Provider: "mock", BaseURL: srv.URL, Model: "m", APIKey: "k"})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := ad.Call(ctx, &adapters.Request{Model: "m"})
	aerr := asAdapterError(t, err)
	if aerr.Kind != adapters.KindTimeout || !aerr.Retryable() {
		t.Fatalf("context-deadline classification = %+v, want retryable KindTimeout", aerr)
	}
	if aerr.Kind == adapters.KindTransport {
		t.Fatalf("timeout must not classify as transport: %+v", aerr)
	}
}

func TestCallRejectsEmptyChoices(t *testing.T) {
	srv := newScriptedServer(t, "k", `{"model":"m","choices":[]}`)
	ad := New(Config{Provider: "mock", BaseURL: srv.URL, Model: "m", APIKey: "k"})
	_, err := ad.Call(context.Background(), &adapters.Request{Model: "m"})
	aerr := asAdapterError(t, err)
	// A 200 carrying neither content nor tool calls is an empty response
	// (retryable) — anthropic parity — never a wire/transport failure.
	if aerr.Kind != adapters.KindEmptyResponse || !aerr.Retryable() {
		t.Fatalf("empty-choices classification = %+v, want retryable empty-response", aerr)
	}
}

// A choice that decodes but carries neither content nor tool calls is the
// same empty-response class (the loop folds it identically at its seam).
func TestCallEmptyContentNoToolCallsIsEmptyResponse(t *testing.T) {
	srv := newScriptedServer(t, "k", `{"model":"m","choices":[{"index":0,"message":{"role":"assistant","content":""},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	ad := New(Config{Provider: "mock", BaseURL: srv.URL, Model: "m", APIKey: "k"})
	_, err := ad.Call(context.Background(), &adapters.Request{Model: "m"})
	aerr := asAdapterError(t, err)
	if aerr.Kind != adapters.KindEmptyResponse || !aerr.Retryable() {
		t.Fatalf("empty-content classification = %+v, want retryable empty-response", aerr)
	}
}

func TestCallRejectsMalformedJSON(t *testing.T) {
	srv := newScriptedServer(t, "k", "not-json-at-all")
	ad := New(Config{Provider: "mock", BaseURL: srv.URL, Model: "m", APIKey: "k"})
	_, err := ad.Call(context.Background(), &adapters.Request{Model: "m"})
	// A 200 body we cannot decode is deterministic — non-retryable
	// (anthropic parity: decode failures are KindOther).
	aerr := asAdapterError(t, err)
	if aerr.Kind != adapters.KindOther || aerr.Retryable() {
		t.Fatalf("decode-failure classification = %+v, want non-retryable KindOther", aerr)
	}
}

func TestCallRejectsMalformedToolArguments(t *testing.T) {
	bad := `{"model":"m","choices":[{"index":0,"message":{"role":"assistant","tool_calls":[{"id":"c","type":"function","function":{"name":"echo","arguments":"not-json{"}}]},"finish_reason":"tool_calls"}]}`
	srv := newScriptedServer(t, "k", bad)
	ad := New(Config{Provider: "mock", BaseURL: srv.URL, Model: "m", APIKey: "k"})
	_, err := ad.Call(context.Background(), &adapters.Request{Model: "m"})
	if err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("expected malformed-arguments error, got %v", err)
	}
	aerr := asAdapterError(t, err)
	if aerr.Kind != adapters.KindOther || aerr.Retryable() {
		t.Fatalf("malformed-arguments classification = %+v, want non-retryable KindOther", aerr)
	}
}

func TestNameDefaults(t *testing.T) {
	if got := New(Config{}).Name(); got != DefaultProviderName {
		t.Fatalf("Name() = %q, want %q", got, DefaultProviderName)
	}
	if got := New(Config{Provider: "custom"}).Name(); got != "custom" {
		t.Fatalf("Name() = %q, want custom", got)
	}
}
