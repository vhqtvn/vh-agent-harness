package anthropic

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

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
)

// capturedRequest records what the adapter actually put on the wire.
type capturedRequest struct {
	Method string
	Path   string
	APIKey string
	Ver    string
	Body   []byte
}

// scriptedResponse is one scripted reply: status, extra headers, body.
type scriptedResponse struct {
	Status  int
	Headers map[string]string
	Body    string
}

// scriptedServer is a deterministic mock Anthropic server (the dsh
// llm-mock-server pattern): it serves scripted responses in order and
// records every request. No real network is involved.
type scriptedServer struct {
	*httptest.Server
	mu        sync.Mutex
	responses []scriptedResponse
	captured  []capturedRequest
}

func ok(body string) scriptedResponse { return scriptedResponse{Status: 200, Body: body} }

func newScriptedServer(t *testing.T, apiKey string, bodies ...string) *scriptedServer {
	resps := make([]scriptedResponse, len(bodies))
	for i, b := range bodies {
		resps[i] = ok(b)
	}
	return newScriptedResponses(t, resps...)
}

func newScriptedServerHeaders(t *testing.T, headers map[string]string, bodies ...string) *scriptedServer {
	resps := make([]scriptedResponse, len(bodies))
	for i, b := range bodies {
		resps[i] = scriptedResponse{Status: 200, Headers: headers, Body: b}
	}
	return newScriptedResponses(t, resps...)
}

func newScriptedResponses(t *testing.T, responses ...scriptedResponse) *scriptedServer {
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
			APIKey: r.Header.Get("x-api-key"),
			Ver:    r.Header.Get("anthropic-version"),
			Body:   body,
		})
		if len(s.captured) > len(s.responses) {
			t.Errorf("mock server: unexpected request #%d (only %d scripted)", len(s.captured), len(s.responses))
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		resp := s.responses[len(s.captured)-1]
		status := resp.Status
		if status == 0 {
			status = http.StatusOK
		}
		w.Header().Set("Content-Type", "application/json")
		for k, v := range resp.Headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(resp.Body))
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

// textResponseScript is a plain end_turn answer.
const textResponseScript = `{"id":"msg_1","role":"assistant","content":[{"type":"text","text":"done: hello echoed"}],"stop_reason":"end_turn","usage":{"input_tokens":30,"output_tokens":8}}`

func TestCallPlainTextResponse(t *testing.T) {
	srv := newScriptedServer(t, "test-key", textResponseScript)
	ad := New(Config{Provider: "mock", BaseURL: srv.URL, Model: "claude-mock", APIKey: "test-key"})
	temp := 0.2
	resp, err := ad.Call(context.Background(), &adapters.Request{
		// Model intentionally empty: the adapter must fall back to Config.Model.
		Messages: []adapters.Message{
			{Role: "system", Content: "You are terse."},
			{Role: "user", Content: "hi"},
		},
		Tools:       []adapters.ToolSpec{{Name: "echo", Description: "echo it", Parameters: json.RawMessage(`{"type":"object"}`)}},
		Temperature: &temp,
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Content != "done: hello echoed" {
		t.Fatalf("Content = %q", resp.Content)
	}
	if resp.FinishReason != "stop" {
		t.Fatalf("FinishReason = %q, want stop (end_turn)", resp.FinishReason)
	}
	if resp.Usage != (adapters.Usage{PromptTokens: 30, CompletionTokens: 8, TotalTokens: 38}) {
		t.Fatalf("Usage = %+v, want input=30 output=8 total=38 (summed: Anthropic sends no total)", resp.Usage)
	}

	reqs := srv.requests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}
	r0 := reqs[0]
	if r0.Method != http.MethodPost || r0.Path != "/v1/messages" {
		t.Fatalf("request = %s %s, want POST /v1/messages", r0.Method, r0.Path)
	}
	if r0.APIKey != "test-key" {
		t.Fatalf("x-api-key = %q", r0.APIKey)
	}
	if r0.Ver != "2023-06-01" {
		t.Fatalf("anthropic-version = %q", r0.Ver)
	}

	body := decodeBody(t, r0.Body)
	if body["model"] != "claude-mock" {
		t.Fatalf("wire model = %v, want Config.Model fallback", body["model"])
	}
	if body["temperature"] != 0.2 {
		t.Fatalf("wire temperature = %v", body["temperature"])
	}
	// max_tokens is REQUIRED by the Messages API: the default must appear.
	if body["max_tokens"] != float64(DefaultMaxTokens) {
		t.Fatalf("wire max_tokens = %v, want default %d", body["max_tokens"], DefaultMaxTokens)
	}
	// System prompt is a TOP-LEVEL field, never a message.
	if body["system"] != "You are terse." {
		t.Fatalf("wire system = %v, want extracted top-level system", body["system"])
	}
	msgs := body["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("wire messages = %v, want only the user turn (system extracted)", msgs)
	}
	m0 := msgs[0].(map[string]any)
	if m0["role"] != "user" || m0["content"] != "hi" {
		t.Fatalf("wire message[0] = %v", m0)
	}
	tools := body["tools"].([]any)
	t0 := tools[0].(map[string]any)
	if t0["name"] != "echo" || t0["description"] != "echo it" {
		t.Fatalf("wire tool = %v", t0)
	}
	schema := t0["input_schema"].(map[string]any)
	if schema["type"] != "object" {
		t.Fatalf("wire input_schema = %v", schema)
	}
	if _, has := body["tool_choice"]; has {
		t.Fatal("tool_choice must be omitted (auto) unless a later design needs forcing")
	}
}

func TestCallMaxTokensResolution(t *testing.T) {
	cases := []struct {
		name    string
		cfgMax  int
		reqMax  int
		wantOnW int
	}{
		{"request overrides config", 999, 128, 128},
		{"config default when request unset", 999, 0, 999},
		{"built-in default when both unset", 0, 0, DefaultMaxTokens},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newScriptedServer(t, "k", textResponseScript)
			ad := New(Config{BaseURL: srv.URL, Model: "m", APIKey: "k", MaxTokens: tc.cfgMax})
			if _, err := ad.Call(context.Background(), &adapters.Request{Model: "m", MaxTokens: tc.reqMax, Messages: []adapters.Message{{Role: "user", Content: "hi"}}}); err != nil {
				t.Fatalf("Call: %v", err)
			}
			body := decodeBody(t, srv.requests()[0].Body)
			if body["max_tokens"] != float64(tc.wantOnW) {
				t.Fatalf("wire max_tokens = %v, want %d", body["max_tokens"], tc.wantOnW)
			}
		})
	}
}

func TestCallSerializesAssistantToolCallsAndToolResults(t *testing.T) {
	srv := newScriptedServer(t, "k", textResponseScript)
	ad := New(Config{BaseURL: srv.URL, Model: "m", APIKey: "k"})
	_, err := ad.Call(context.Background(), &adapters.Request{
		Model: "m",
		Messages: []adapters.Message{
			{Role: "user", Content: "echo hello"},
			{Role: "assistant", Content: "", ToolCalls: []adapters.ToolCall{
				{ID: "tu_1", Name: "echo", Args: json.RawMessage(`{"text":"hello"}`)},
				{ID: "tu_2", Name: "upper", Args: json.RawMessage(`{"text":"hello"}`)},
			}},
			{Role: "tool", ToolCallID: "tu_1", Name: "echo", Content: `{"text":"hello"}`},
			{Role: "tool", ToolCallID: "tu_2", Name: "upper", Content: `{"text":"HELLO"}`},
		},
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	body := decodeBody(t, srv.requests()[0].Body)
	msgs := body["messages"].([]any)
	// user turn, assistant tool_use turn, ONE merged user tool_result turn.
	if len(msgs) != 3 {
		t.Fatalf("wire messages = %v", msgs)
	}
	assistant := msgs[1].(map[string]any)
	if assistant["role"] != "assistant" {
		t.Fatalf("wire assistant role = %v", assistant["role"])
	}
	blocks := assistant["content"].([]any)
	if len(blocks) != 2 {
		t.Fatalf("assistant content blocks = %v, want 2 tool_use blocks (no empty text block)", blocks)
	}
	b0 := blocks[0].(map[string]any)
	if b0["type"] != "tool_use" || b0["id"] != "tu_1" || b0["name"] != "echo" {
		t.Fatalf("tool_use block = %v", b0)
	}
	// Asymmetry (documented): our contract carries args as a JSON STRING;
	// the Anthropic wire wants a JSON OBJECT under `input`.
	if input, ok := b0["input"].(map[string]any); !ok || input["text"] != "hello" {
		t.Fatalf("tool_use input = %v (%T), want object", b0["input"], b0["input"])
	}
	// Adjacent tool results share ONE user-role message (Anthropic parallel
	// tool-result convention) — the documented placement choice.
	tr := msgs[2].(map[string]any)
	if tr["role"] != "user" {
		t.Fatalf("tool_result carrier role = %v, want user", tr["role"])
	}
	trBlocks := tr["content"].([]any)
	if len(trBlocks) != 2 {
		t.Fatalf("tool_result blocks = %v, want 2 merged", trBlocks)
	}
	r0 := trBlocks[0].(map[string]any)
	if r0["type"] != "tool_result" || r0["tool_use_id"] != "tu_1" || r0["content"] != `{"text":"hello"}` {
		t.Fatalf("tool_result block = %v", r0)
	}
}

// toolUseResponseScript mixes a text block and a tool_use block, the
// canonical parallel-tool shape.
const toolUseResponseScript = `{"id":"msg_2","role":"assistant","model":"claude-mock","content":[{"type":"text","text":"calling echo"},{"type":"tool_use","id":"toolu_1","name":"echo","input":{"text":"hello"}}],"stop_reason":"tool_use","usage":{"input_tokens":10,"output_tokens":5}}`

func TestCallToolUseResponse(t *testing.T) {
	srv := newScriptedServer(t, "k", toolUseResponseScript)
	ad := New(Config{BaseURL: srv.URL, Model: "claude-mock", APIKey: "k"})
	resp, err := ad.Call(context.Background(), &adapters.Request{
		Model:    "claude-mock",
		Messages: []adapters.Message{{Role: "user", Content: "echo hello"}},
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	// Text blocks concatenate.
	if resp.Content != "calling echo" {
		t.Fatalf("Content = %q, want concatenated text blocks", resp.Content)
	}
	if resp.FinishReason != "tool_calls" {
		t.Fatalf("FinishReason = %q, want tool_calls (tool_use)", resp.FinishReason)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("ToolCalls = %+v", resp.ToolCalls)
	}
	tc := resp.ToolCalls[0]
	if tc.ID != "toolu_1" || tc.Name != "echo" {
		t.Fatalf("tool call = %+v, want id preserved", tc)
	}
	// Asymmetry (documented): wire input arrives as a JSON OBJECT; our
	// contract stringifies it back to match the shared string-args shape.
	if string(tc.Args) != `{"text":"hello"}` {
		t.Fatalf("tool call args = %s, want stringified object", tc.Args)
	}
}

func TestCallStopReasonMappings(t *testing.T) {
	cases := []struct {
		stop string
		want string
	}{
		{"end_turn", "stop"},
		{"max_tokens", "length"},
		{"tool_use", "tool_calls"},
		{"stop_sequence", "stop"},
	}
	for _, tc := range cases {
		t.Run(tc.stop, func(t *testing.T) {
			script := `{"id":"m","role":"assistant","model":"m","content":[{"type":"text","text":"x"}],"stop_reason":"` + tc.stop + `","usage":{"input_tokens":1,"output_tokens":1}}`
			srv := newScriptedServer(t, "k", script)
			ad := New(Config{BaseURL: srv.URL, Model: "m", APIKey: "k"})
			resp, err := ad.Call(context.Background(), &adapters.Request{Model: "m"})
			if err != nil {
				t.Fatalf("Call: %v", err)
			}
			if resp.FinishReason != tc.want {
				t.Fatalf("stop_reason %q mapped to %q, want %q", tc.stop, resp.FinishReason, tc.want)
			}
		})
	}
}

func TestCallRejectsEmptyContent(t *testing.T) {
	srv := newScriptedServer(t, "k", `{"id":"m","role":"assistant","model":"m","content":[],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":0}}`)
	ad := New(Config{BaseURL: srv.URL, Model: "m", APIKey: "k"})
	_, err := ad.Call(context.Background(), &adapters.Request{Model: "m"})
	if err == nil {
		t.Fatal("expected error on 200 with neither content nor tool calls")
	}
	var aerr *adapters.AdapterError
	if !errors.As(err, &aerr) {
		t.Fatalf("expected *adapters.AdapterError, got %T: %v", err, err)
	}
	if aerr.Kind != adapters.KindEmptyResponse || !aerr.Retryable() {
		t.Fatalf("empty-content classification = %+v, want retryable empty-response", aerr)
	}
}

func TestCallRejectsNonObjectToolInput(t *testing.T) {
	bad := `{"id":"m","role":"assistant","model":"m","content":[{"type":"tool_use","id":"t1","name":"echo","input":"not-an-object"}],"stop_reason":"tool_use","usage":{"input_tokens":1,"output_tokens":1}}`
	srv := newScriptedServer(t, "k", bad)
	ad := New(Config{BaseURL: srv.URL, Model: "m", APIKey: "k"})
	_, err := ad.Call(context.Background(), &adapters.Request{Model: "m"})
	if err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("expected malformed-input error, got %v", err)
	}
}

func TestCallHTTP400IsNonRetryable(t *testing.T) {
	srv := newScriptedResponses(t, scriptedResponse{Status: 400, Body: `{"type":"error","error":{"type":"invalid_request_error","message":"max_tokens is required"}}`})
	ad := New(Config{BaseURL: srv.URL, Model: "m", APIKey: "k"})
	_, err := ad.Call(context.Background(), &adapters.Request{Model: "m"})
	var aerr *adapters.AdapterError
	if !errors.As(err, &aerr) {
		t.Fatalf("expected *adapters.AdapterError, got %T: %v", err, err)
	}
	if aerr.Status != 400 || aerr.Kind != adapters.KindOther || aerr.Retryable() {
		t.Fatalf("400 classification = %+v, want non-retryable KindOther", aerr)
	}
	if !strings.Contains(err.Error(), "invalid_request_error") {
		t.Fatalf("error should carry the provider message: %v", err)
	}
}

// TestCallHTTPErrorBodyRedactsAPIKey is the finding-2 adversarial crux
// at the adapter seam: a hostile/broken provider that ECHOES the API-key
// value (sent via x-api-key) in a non-2xx error body must never get that
// value into the AdapterError text — every occurrence is replaced by
// [REDACTED] at the source (the excerpt-capture site), while the rest of
// the body excerpt survives unchanged.
func TestCallHTTPErrorBodyRedactsAPIKey(t *testing.T) {
	const key = "sk-ant-live-0123456789abcdef"
	body := `{"type":"error","error":{"type":"authentication_error","message":"key sk-ant-live-0123456789abcdef rejected for tenant-8821 (decoy, not a credential); echo #2: sk-ant-live-0123456789abcdef"}}`
	srv := newScriptedResponses(t, scriptedResponse{Status: 500, Body: body})
	ad := New(Config{BaseURL: srv.URL, Model: "m", APIKey: key})
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
	if !strings.Contains(msg, "tenant-8821") {
		t.Fatalf("non-key body excerpt should survive redaction: %s", msg)
	}
}

func TestCallRateLimited429CarriesRetryAfter(t *testing.T) {
	srv := newScriptedResponses(t, scriptedResponse{Status: 429, Headers: map[string]string{"retry-after": "2"}, Body: `{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`})
	ad := New(Config{BaseURL: srv.URL, Model: "m", APIKey: "k"})
	_, err := ad.Call(context.Background(), &adapters.Request{Model: "m"})
	var aerr *adapters.AdapterError
	if !errors.As(err, &aerr) {
		t.Fatalf("expected *adapters.AdapterError, got %T: %v", err, err)
	}
	if aerr.Kind != adapters.KindRateLimit || !aerr.Retryable() {
		t.Fatalf("429 classification = %+v, want retryable rate limit", aerr)
	}
	if aerr.RetryAfterMs != 2000 {
		t.Fatalf("RetryAfterMs = %d, want 2000 (retry-after: 2 seconds)", aerr.RetryAfterMs)
	}
}

func TestCallOverloaded529IsRetryable(t *testing.T) {
	srv := newScriptedResponses(t, scriptedResponse{Status: 529, Body: `{"type":"error","error":{"type":"overloaded_error","message":"overloaded"}}`})
	ad := New(Config{BaseURL: srv.URL, Model: "m", APIKey: "k"})
	_, err := ad.Call(context.Background(), &adapters.Request{Model: "m"})
	var aerr *adapters.AdapterError
	if !errors.As(err, &aerr) {
		t.Fatalf("expected *adapters.AdapterError, got %T: %v", err, err)
	}
	if aerr.Status != 529 || !aerr.Retryable() {
		t.Fatalf("529 classification = %+v, want retryable (server-error class)", aerr)
	}
	if aerr.RetryAfterMs != 0 {
		t.Fatalf("RetryAfterMs = %d, want 0 when header absent", aerr.RetryAfterMs)
	}
}

func TestCallRejectsMalformedJSON(t *testing.T) {
	srv := newScriptedServer(t, "k", "not-json-at-all")
	ad := New(Config{BaseURL: srv.URL, Model: "m", APIKey: "k"})
	_, err := ad.Call(context.Background(), &adapters.Request{Model: "m"})
	if err == nil {
		t.Fatal("expected error on malformed JSON body")
	}
	var aerr *adapters.AdapterError
	if !errors.As(err, &aerr) || aerr.Retryable() {
		t.Fatalf("decode failure must be non-retryable, got %v", err)
	}
}

func TestCallRejectsMalformedOurSideArgsBeforeNetwork(t *testing.T) {
	srv := newScriptedServer(t, "k", textResponseScript)
	ad := New(Config{BaseURL: srv.URL, Model: "m", APIKey: "k"})
	_, err := ad.Call(context.Background(), &adapters.Request{
		Model: "m",
		Messages: []adapters.Message{
			{Role: "user", Content: "echo"},
			{Role: "assistant", ToolCalls: []adapters.ToolCall{{ID: "tu_1", Name: "echo", Args: json.RawMessage(`not-json{`)}}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("expected malformed-args rejection, got %v", err)
	}
	var aerr *adapters.AdapterError
	if !errors.As(err, &aerr) || aerr.Retryable() {
		t.Fatalf("our-side malformed args must be non-retryable, got %v", err)
	}
	if got := len(srv.requests()); got != 0 {
		t.Fatalf("rejection must happen BEFORE network I/O; server saw %d requests", got)
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

func TestRegistryRegistration(t *testing.T) {
	reg := adapters.NewRegistry()
	if err := reg.Register(New(Config{BaseURL: "http://unused", Model: "m", APIKey: "k"})); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, ok := reg.Get(DefaultProviderName)
	if !ok || got == nil {
		t.Fatalf("Get(%q) miss; names = %v", DefaultProviderName, reg.Names())
	}
	if got.Name() != DefaultProviderName {
		t.Fatalf("registered adapter Name() = %q", got.Name())
	}
	// The distinct route name must not collide with openaicompat's.
	if err := reg.Register(New(Config{Provider: "openaicompat"})); err != nil {
		t.Fatalf("cross-provider registration under a second name must work: %v", err)
	}
}
