package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
)

// Prompt-cache slice tests. The crux is the PAIR:
//   - enabled wire shape: cache_control appears at EXACTLY the policy
//     positions (final tool definition; final system block) and never
//     anywhere else;
//   - disabled byte identity: with CacheConfig zero (the default), the
//     marshaled request is byte-identical to the PRE-cache-slice output
//     (golden literals below were captured from the adapter before the
//     cache code existed).

// goldenRich is the pre-slice wire body for richRequest with caching
// disabled (captured pre-implementation; see slice notes).
const goldenRich = `{"model":"shared-model","max_tokens":128,"system":"You are terse.","messages":[{"role":"user","content":"echo hello"},{"role":"assistant","content":[{"type":"tool_use","id":"call_1","name":"echo","input":{"text":"hello"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":"{\"text\":\"hello\"}"}]}],"tools":[{"name":"echo","description":"echo it","input_schema":{"type":"object"}}],"temperature":0.2}`

// goldenNoSys is the pre-slice wire body for a minimal request (no system,
// no tools) with caching disabled.
const goldenNoSys = `{"model":"shared-model","max_tokens":128,"messages":[{"role":"user","content":"hi"}]}`

// richRequest exercises every wire surface: system, plain user text,
// assistant tool_use blocks, merged tool_result, tools, temperature.
func richRequest() *adapters.Request {
	temp := 0.2
	return &adapters.Request{
		Model:     "shared-model",
		MaxTokens: 128,
		Messages: []adapters.Message{
			{Role: "system", Content: "You are terse."},
			{Role: "user", Content: "echo hello"},
			{Role: "assistant", Content: "", ToolCalls: []adapters.ToolCall{
				{ID: "call_1", Name: "echo", Args: json.RawMessage(`{"text":"hello"}`)},
			}},
			{Role: "tool", ToolCallID: "call_1", Name: "echo", Content: `{"text":"hello"}`},
		},
		Tools:       []adapters.ToolSpec{{Name: "echo", Description: "echo it", Parameters: json.RawMessage(`{"type":"object"}`)}},
		Temperature: &temp,
	}
}

func TestCacheDisabledWireBytesIdenticalToPreSlice(t *testing.T) {
	// Zero-value CacheConfig (the default) must produce byte-identical
	// output to the pre-cache adapter.
	srv := newScriptedServer(t, "k", textResponseScript)
	ad := New(Config{BaseURL: srv.URL, Model: "m", APIKey: "k"})
	if _, err := ad.Call(context.Background(), richRequest()); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := string(srv.requests()[0].Body); got != goldenRich {
		t.Fatalf("disabled wire differs from pre-slice golden:\n got: %s\nwant: %s", got, goldenRich)
	}

	// Even a nonsense MaxBreakpoints stays inert while disabled: zero
	// behavior change when off means NO validation, NO wire delta.
	srv2 := newScriptedServer(t, "k", textResponseScript)
	ad2 := New(Config{BaseURL: srv2.URL, Model: "m", APIKey: "k", Cache: CacheConfig{Enabled: false, MaxBreakpoints: 99}})
	if _, err := ad2.Call(context.Background(), richRequest()); err != nil {
		t.Fatalf("Call with disabled-but-invalid cache config must not error: %v", err)
	}
	if got := string(srv2.requests()[0].Body); got != goldenRich {
		t.Fatalf("disabled+invalid MaxBreakpoints changed the wire:\n got: %s\nwant: %s", got, goldenRich)
	}
}

func TestCacheEnabledWireShapeToolsAndSystem(t *testing.T) {
	// Enabled with unset MaxBreakpoints (0 -> default 2): breakpoint 1 on
	// the LAST tool definition, breakpoint 2 on the system prompt rendered
	// as a block array with cache_control on the (sole, hence final)
	// block. Messages untouched.
	srv := newScriptedServer(t, "k", textResponseScript)
	ad := New(Config{BaseURL: srv.URL, Model: "m", APIKey: "k", Cache: CacheConfig{Enabled: true}})
	if _, err := ad.Call(context.Background(), richRequest()); err != nil {
		t.Fatalf("Call: %v", err)
	}
	want := `{"model":"shared-model","max_tokens":128,"system":[{"type":"text","text":"You are terse.","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"user","content":"echo hello"},{"role":"assistant","content":[{"type":"tool_use","id":"call_1","name":"echo","input":{"text":"hello"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":"{\"text\":\"hello\"}"}]}],"tools":[{"name":"echo","description":"echo it","input_schema":{"type":"object"},"cache_control":{"type":"ephemeral"}}],"temperature":0.2}`
	if got := string(srv.requests()[0].Body); got != want {
		t.Fatalf("enabled wire shape differs:\n got: %s\nwant: %s", got, want)
	}
}

func TestCacheEnabledMultipleToolsMarksOnlyLast(t *testing.T) {
	srv := newScriptedServer(t, "k", textResponseScript)
	ad := New(Config{BaseURL: srv.URL, Model: "m", APIKey: "k", Cache: CacheConfig{Enabled: true}})
	req := &adapters.Request{
		Model:     "m",
		MaxTokens: 64,
		Messages:  []adapters.Message{{Role: "system", Content: "S"}, {Role: "user", Content: "hi"}},
		Tools: []adapters.ToolSpec{
			{Name: "a", Parameters: json.RawMessage(`{"type":"object"}`)},
			{Name: "b", Parameters: json.RawMessage(`{"type":"object"}`)},
			{Name: "c", Parameters: json.RawMessage(`{"type":"object"}`)},
		},
	}
	if _, err := ad.Call(context.Background(), req); err != nil {
		t.Fatalf("Call: %v", err)
	}
	want := `{"model":"m","max_tokens":64,"system":[{"type":"text","text":"S","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"user","content":"hi"}],"tools":[{"name":"a","input_schema":{"type":"object"}},{"name":"b","input_schema":{"type":"object"}},{"name":"c","input_schema":{"type":"object"},"cache_control":{"type":"ephemeral"}}]}`
	if got := string(srv.requests()[0].Body); got != want {
		t.Fatalf(">1 tool: breakpoint must be on the LAST tool only:\n got: %s\nwant: %s", got, want)
	}
}

func TestCacheEnabledEmptySystemOmitsSystemMarksTools(t *testing.T) {
	srv := newScriptedServer(t, "k", textResponseScript)
	ad := New(Config{BaseURL: srv.URL, Model: "m", APIKey: "k", Cache: CacheConfig{Enabled: true}})
	req := &adapters.Request{
		Model:     "m",
		MaxTokens: 64,
		Messages:  []adapters.Message{{Role: "user", Content: "hi"}},
		Tools:     []adapters.ToolSpec{{Name: "echo", Parameters: json.RawMessage(`{"type":"object"}`)}},
	}
	if _, err := ad.Call(context.Background(), req); err != nil {
		t.Fatalf("Call: %v", err)
	}
	want := `{"model":"m","max_tokens":64,"messages":[{"role":"user","content":"hi"}],"tools":[{"name":"echo","input_schema":{"type":"object"},"cache_control":{"type":"ephemeral"}}]}`
	if got := string(srv.requests()[0].Body); got != want {
		t.Fatalf("empty system: system key must stay omitted, tool marked:\n got: %s\nwant: %s", got, want)
	}
}

func TestCacheEnabledSystemOnlyNoTools(t *testing.T) {
	srv := newScriptedServer(t, "k", textResponseScript)
	ad := New(Config{BaseURL: srv.URL, Model: "m", APIKey: "k", Cache: CacheConfig{Enabled: true}})
	req := &adapters.Request{
		Model:     "m",
		MaxTokens: 64,
		Messages:  []adapters.Message{{Role: "system", Content: "You are terse."}, {Role: "user", Content: "hi"}},
	}
	if _, err := ad.Call(context.Background(), req); err != nil {
		t.Fatalf("Call: %v", err)
	}
	want := `{"model":"m","max_tokens":64,"system":[{"type":"text","text":"You are terse.","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"user","content":"hi"}]}`
	if got := string(srv.requests()[0].Body); got != want {
		t.Fatalf("system-only: breakpoint falls to the system block:\n got: %s\nwant: %s", got, want)
	}
}

func TestCacheEnabledJoinsSystemIntoSingleBlock(t *testing.T) {
	// Multiple system messages join into ONE text block (the same
	// "\n\n"-joined string as the disabled path) — enabling caching never
	// re-chunks system content; the sole block is the final block.
	srv := newScriptedServer(t, "k", textResponseScript)
	ad := New(Config{BaseURL: srv.URL, Model: "m", APIKey: "k", Cache: CacheConfig{Enabled: true}})
	req := &adapters.Request{
		Model:     "m",
		MaxTokens: 64,
		Messages:  []adapters.Message{{Role: "system", Content: "Part one."}, {Role: "system", Content: "Part two."}, {Role: "user", Content: "hi"}},
	}
	if _, err := ad.Call(context.Background(), req); err != nil {
		t.Fatalf("Call: %v", err)
	}
	want := `{"model":"m","max_tokens":64,"system":[{"type":"text","text":"Part one.\n\nPart two.","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"user","content":"hi"}]}`
	if got := string(srv.requests()[0].Body); got != want {
		t.Fatalf("joined system must render as one block:\n got: %s\nwant: %s", got, want)
	}
}

func TestCacheMaxBreakpointsOneToolsBreakpointOnly(t *testing.T) {
	// Budget 1 + tools present: ONLY the tools breakpoint; the system
	// prompt stays a PLAIN STRING (not blocks, not marked).
	srv := newScriptedServer(t, "k", textResponseScript)
	ad := New(Config{BaseURL: srv.URL, Model: "m", APIKey: "k", Cache: CacheConfig{Enabled: true, MaxBreakpoints: 1}})
	req := &adapters.Request{
		Model:     "m",
		MaxTokens: 64,
		Messages:  []adapters.Message{{Role: "system", Content: "S"}, {Role: "user", Content: "hi"}},
		Tools:     []adapters.ToolSpec{{Name: "echo", Parameters: json.RawMessage(`{"type":"object"}`)}},
	}
	if _, err := ad.Call(context.Background(), req); err != nil {
		t.Fatalf("Call: %v", err)
	}
	want := `{"model":"m","max_tokens":64,"system":"S","messages":[{"role":"user","content":"hi"}],"tools":[{"name":"echo","input_schema":{"type":"object"},"cache_control":{"type":"ephemeral"}}]}`
	if got := string(srv.requests()[0].Body); got != want {
		t.Fatalf("MaxBreakpoints=1 must mark tools only, system stays plain:\n got: %s\nwant: %s", got, want)
	}
}

func TestCacheMaxBreakpointsOneNoToolsFallsToSystem(t *testing.T) {
	// Budget 1 + NO tools: the tools position is unavailable, so the
	// breakpoint falls to the system block.
	srv := newScriptedServer(t, "k", textResponseScript)
	ad := New(Config{BaseURL: srv.URL, Model: "m", APIKey: "k", Cache: CacheConfig{Enabled: true, MaxBreakpoints: 1}})
	req := &adapters.Request{
		Model:     "m",
		MaxTokens: 64,
		Messages:  []adapters.Message{{Role: "system", Content: "S"}, {Role: "user", Content: "hi"}},
	}
	if _, err := ad.Call(context.Background(), req); err != nil {
		t.Fatalf("Call: %v", err)
	}
	want := `{"model":"m","max_tokens":64,"system":[{"type":"text","text":"S","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"user","content":"hi"}]}`
	if got := string(srv.requests()[0].Body); got != want {
		t.Fatalf("MaxBreakpoints=1 without tools must mark system:\n got: %s\nwant: %s", got, want)
	}
}

func TestCacheMaxBreakpointsAbovePolicyStillTwoBreakpoints(t *testing.T) {
	// 3..4 are VALID config (forward-compat) but this slice's policy
	// defines only two positions; the wire must equal the default-enabled
	// wire exactly.
	mk := func() *scriptedServer {
		return newScriptedServer(t, "k", textResponseScript)
	}
	s4 := mk()
	ad4 := New(Config{BaseURL: s4.URL, Model: "m", APIKey: "k", Cache: CacheConfig{Enabled: true, MaxBreakpoints: 4}})
	if _, err := ad4.Call(context.Background(), richRequest()); err != nil {
		t.Fatalf("Call(4): %v", err)
	}
	sD := mk()
	adD := New(Config{BaseURL: sD.URL, Model: "m", APIKey: "k", Cache: CacheConfig{Enabled: true}})
	if _, err := adD.Call(context.Background(), richRequest()); err != nil {
		t.Fatalf("Call(default): %v", err)
	}
	if got, want := string(s4.requests()[0].Body), string(sD.requests()[0].Body); got != want {
		t.Fatalf("MaxBreakpoints=4 must equal default-enabled wire:\n got: %s\nwant: %s", got, want)
	}
}

func TestCacheNeverPlacesBreakpointsOnMessages(t *testing.T) {
	// The invariant behind the policy: message content NEVER carries
	// cache_control (messages are volatile; stable-prefix hints from the
	// engine are a future seam).
	srv := newScriptedServer(t, "k", textResponseScript)
	ad := New(Config{BaseURL: srv.URL, Model: "m", APIKey: "k", Cache: CacheConfig{Enabled: true}})
	if _, err := ad.Call(context.Background(), richRequest()); err != nil {
		t.Fatalf("Call: %v", err)
	}
	body := decodeBody(t, srv.requests()[0].Body)
	msgs, err := json.Marshal(body["messages"])
	if err != nil {
		t.Fatalf("re-marshal messages: %v", err)
	}
	if strings.Contains(string(msgs), "cache_control") {
		t.Fatalf("messages must never carry cache_control: %s", msgs)
	}
	// And the exact count on the whole wire: 2 breakpoints (tool + system).
	if got := strings.Count(string(srv.requests()[0].Body), `"cache_control"`); got != 2 {
		t.Fatalf("whole-wire cache_control count = %d, want exactly 2 (one tool, one system block)", got)
	}
}

func TestCacheEnabledWithNothingToCacheIsByteIdentical(t *testing.T) {
	// Enabled but neither tools nor system exist: zero placeable
	// positions, wire identical to the pre-slice (and disabled) output.
	srv := newScriptedServer(t, "k", textResponseScript)
	ad := New(Config{BaseURL: srv.URL, Model: "m", APIKey: "k", Cache: CacheConfig{Enabled: true}})
	if _, err := ad.Call(context.Background(), &adapters.Request{Model: "shared-model", MaxTokens: 128, Messages: []adapters.Message{{Role: "user", Content: "hi"}}}); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := string(srv.requests()[0].Body); got != goldenNoSys {
		t.Fatalf("enabled-with-nothing-to-cache differs from golden:\n got: %s\nwant: %s", got, goldenNoSys)
	}
}

func TestCacheValidation(t *testing.T) {
	cases := []struct {
		name    string
		cache   CacheConfig
		wantErr bool
	}{
		{"unset max defaults to 2 when enabled", CacheConfig{Enabled: true}, false},
		{"1 is valid", CacheConfig{Enabled: true, MaxBreakpoints: 1}, false},
		{"4 is valid (forward-compat cap)", CacheConfig{Enabled: true, MaxBreakpoints: 4}, false},
		{"5 exceeds cap", CacheConfig{Enabled: true, MaxBreakpoints: 5}, true},
		{"negative rejected", CacheConfig{Enabled: true, MaxBreakpoints: -1}, true},
		{"disabled ignores invalid max", CacheConfig{Enabled: false, MaxBreakpoints: 99}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newScriptedServer(t, "k", textResponseScript)
			ad := New(Config{BaseURL: srv.URL, Model: "m", APIKey: "k", Cache: tc.cache})
			_, err := ad.Call(context.Background(), richRequest())
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected validation error, got nil")
				}
				var aerr *adapters.AdapterError
				if !errors.As(err, &aerr) {
					t.Fatalf("expected *adapters.AdapterError, got %T: %v", err, err)
				}
				if aerr.Kind != adapters.KindOther || aerr.Retryable() {
					t.Fatalf("cache misconfiguration must be non-retryable KindOther, got %+v", aerr)
				}
				if got := len(srv.requests()); got != 0 {
					t.Fatalf("rejection must happen BEFORE network I/O; server saw %d requests", got)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
