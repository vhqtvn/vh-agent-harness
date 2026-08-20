// Package anthropic implements the adapters.Adapter boundary for the
// Anthropic Messages API (non-streaming POST /v1/messages). It maps the
// SAME internal/adapters Request/Response contract the openaicompat
// adapter uses, so internal/loop and internal/tools work unchanged.
//
// Mapping contract (the value of this adapter):
//
//   - System: our role="system" messages are EXTRACTED to the top-level
//     `system` field (the Messages API has no system role). Multiple
//     system messages are joined with a blank line.
//   - Assistant tool calls: our string-encoded ToolCall.Args are decoded
//     and re-encoded as the tool_use `input` JSON OBJECT (asymmetry: our
//     contract carries args as a JSON string, Anthropic as an object).
//   - Tool results: our role="tool" messages become tool_result blocks
//     riding USER-role messages. ADJACENT tool results are merged into
//     ONE user message (the Anthropic parallel tool-result convention);
//     this placement is tested and is the documented choice.
//   - Plain user/assistant text is sent as a plain string content; block
//     arrays are used exactly when blocks exist (tool_use / tool_result).
//     An empty assistant content is the string "", never null — the same
//     ""-never-null convention as openaicompat.
//   - max_tokens is REQUIRED by the Messages API: resolution order is
//     Request.MaxTokens, then Config.MaxTokens, then DefaultMaxTokens.
//   - stop_reason mapping: tool_use→tool_calls, end_turn→stop,
//     max_tokens→length, stop_sequence→stop (same target vocabulary as
//     the openaicompat finish reasons).
//
// Prompt-cache design (IMPLEMENTED, opt-in via Config.Cache): OpenAI-
// compatible endpoints cache implicitly via prefix matching, while
// Anthropic caching is EXPLICIT — `cache_control: {type:"ephemeral"}`
// breakpoints must be attached to the tools array (last definition) and/
// or the system prompt (final block); 5m default TTL, 1h opt-in, minimum
// token thresholds apply server-side.
//
// Placement is derivable INSIDE the adapter because, within one session,
// system+tools are the stable prefix of every call (the engine rebuilds
// only messages between turns) and both ride the Request the adapter
// already receives read-only. Policy (deterministic, breakpoint order):
//
//   - breakpoint 1: the FINAL tool definition (cache_control on the last
//     entry of the tools array);
//   - breakpoint 2: the system prompt, rendered as a single-element
//     block array with cache_control on that (sole, hence final) block.
//     The block text is the SAME "\n\n"-joined string the disabled path
//     sends, so enabling caching never re-chunks system content;
//   - NO message-level breakpoints: messages are volatile turn surface.
//     Future seam: the engine/session layer could later pass
//     stable-prefix hints (which trailing messages are frozen across
//     calls) to refine placement — that hint type belongs to the shared
//     adapters package and is deliberately out of this slice's fence.
//
// Budget: Config.Cache.MaxBreakpoints, 0 = unset → 2 (the system+tools
// sweet spot), validated 1..4 when enabled. Values 3..4 are accepted for
// forward-compat but this slice defines only the two positions above.
// Unavailable positions are skipped in order (no tools → the budget
// falls through to system).
//
// Invariants (tested): disabled ⇒ wire bytes IDENTICAL to the pre-cache
// adapter (golden-compared, and cache misconfiguration is not even
// validated while disabled — zero behavior change when off); enabled ⇒
// breakpoints at exactly the policy positions, never on message content.
// Whether the stable-prefix ASSUMPTION holds across calls remains an
// engine-level concern: the adapter only shapes each single call's wire.
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
)

// DefaultProviderName is used when Config.Provider is empty.
const DefaultProviderName = "anthropic"

// DefaultMaxTokens is the max_tokens sent when neither Request.MaxTokens
// nor Config.MaxTokens is set. The Messages API REQUIRES max_tokens;
// 4096 is the sane injectable default (overridable via Config).
const DefaultMaxTokens = 4096

// defaultCacheBreakpoints is the breakpoint budget when Cache.Enabled is
// true but MaxBreakpoints is unset (0): 2 covers the tools+system prefix,
// Anthropic's documented sweet spot for tool-heavy agents.
const defaultCacheBreakpoints = 2

// cacheBreakpointsCap is the hard upper bound on MaxBreakpoints
// (Anthropic allows at most 4 cache_control breakpoints per request).
const cacheBreakpointsCap = 4

// cacheControlEphemeral is the only cache_control type this slice emits
// (the default 5-minute TTL; the 1h TTL is a deferred config knob).
const cacheControlEphemeral = "ephemeral"

// maxResponseBytes caps response-body reads (matching the openaicompat
// slice-1 posture; fails closed beyond the cap).
const maxResponseBytes = 32 << 20

// CacheConfig configures Anthropic prompt caching (explicit
// cache_control breakpoints). The zero value is DISABLED: the adapter is
// wire-inert — output bytes are identical to the pre-cache adapter, and
// even an invalid MaxBreakpoints is ignored (no validation, no error)
// while Enabled is false.
type CacheConfig struct {
	Enabled bool
	// MaxBreakpoints is the breakpoint budget when Enabled. 0 means
	// unset and defaults to 2. Explicit values are validated to
	// 1 <= n <= 4 when Enabled; violation rejects the call on OUR side
	// before any network I/O (non-retryable).
	MaxBreakpoints int
}

// Config is the complete adapter configuration: the injected base URL,
// model, API key, and the injectable max_tokens default. No environment
// reading happens inside the adapter.
type Config struct {
	Provider string
	BaseURL  string
	Model    string
	APIKey   string
	// MaxTokens is the default sent as max_tokens when Request.MaxTokens
	// is unset; 0 falls back to DefaultMaxTokens.
	MaxTokens int
	// Cache is the prompt-caching configuration; the zero value keeps
	// the adapter cache-inert (see CacheConfig).
	Cache CacheConfig
}

// Adapter speaks the Anthropic Messages wire format (non-streaming)
// against an injected base URL.
type Adapter struct {
	cfg  Config
	http *http.Client
}

// New builds an adapter from config.
func New(cfg Config) *Adapter {
	return &Adapter{cfg: cfg, http: &http.Client{}}
}

// Name returns the provider name (default "anthropic").
func (a *Adapter) Name() string {
	if a.cfg.Provider != "" {
		return a.cfg.Provider
	}
	return DefaultProviderName
}

// --- wire types (Messages API dialect) ---

// wireCacheControl is the explicit cache breakpoint marker. The pointer
// form keeps absent breakpoints off the wire entirely (byte-identity with
// the pre-cache adapter when caching is disabled).
type wireCacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

// wireSystemBlock is one block of the system-as-blocks array. When
// caching marks the system prompt, the WHOLE joined system string rides
// ONE text block (no re-chunking); that sole block is the final block.
type wireSystemBlock struct {
	Type         string            `json:"type"` // "text"
	Text         string            `json:"text"`
	CacheControl *wireCacheControl `json:"cache_control,omitempty"`
}

type wireBlock struct {
	Type string `json:"type"` // text | tool_use | tool_result
	// text block
	Text string `json:"text,omitempty"`
	// tool_use block
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"` // JSON object
	// tool_result block
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"` // string form (our contract is string content)
}

type wireMessage struct {
	Role    string      `json:"role"` // user | assistant (never system: extracted)
	Content interface{} `json:"content"`
}

type wireTool struct {
	Name         string            `json:"name"`
	Description  string            `json:"description,omitempty"`
	InputSchema  json.RawMessage   `json:"input_schema"`
	CacheControl *wireCacheControl `json:"cache_control,omitempty"` // breakpoint 1 target (LAST tool only)
}

type wireRequest struct {
	Model     string `json:"model"`
	MaxTokens int    `json:"max_tokens"`
	// System is either a plain string (the pre-cache shape, used whenever
	// caching does not mark the system prompt) or []wireSystemBlock (used
	// when it does). nil (not "") keeps the key off the wire for an empty
	// system, matching the string+omitempty behavior it replaced.
	System      interface{}   `json:"system,omitempty"`
	Messages    []wireMessage `json:"messages"`
	Tools       []wireTool    `json:"tools,omitempty"`
	Temperature *float64      `json:"temperature,omitempty"`
}

type wireUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type wireResponse struct {
	ID         string      `json:"id"`
	Role       string      `json:"role"`
	Model      string      `json:"model"`
	Content    []wireBlock `json:"content"`
	StopReason string      `json:"stop_reason"`
	Usage      wireUsage   `json:"usage"`
}

// toWireMessages maps our message surface onto the Messages API shape:
// system extracted by the caller, assistant tool calls becoming tool_use
// blocks, and adjacent tool results merged into one user message.
func toWireMessages(msgs []adapters.Message) ([]wireMessage, string, error) {
	var systemParts []string
	var out []wireMessage
	for _, m := range msgs {
		switch m.Role {
		case "system":
			systemParts = append(systemParts, m.Content)
		case "tool":
			// Adjacent tool results merge into ONE user message.
			if n := len(out); n > 0 && out[n-1].Role == "user" {
				if blocks, ok := out[n-1].Content.([]wireBlock); ok && len(blocks) > 0 && blocks[0].Type == "tool_result" {
					out[n-1].Content = append(blocks, wireBlock{Type: "tool_result", ToolUseID: m.ToolCallID, Content: m.Content})
					continue
				}
			}
			out = append(out, wireMessage{Role: "user", Content: []wireBlock{{Type: "tool_result", ToolUseID: m.ToolCallID, Content: m.Content}}})
		case "assistant":
			if len(m.ToolCalls) == 0 {
				out = append(out, wireMessage{Role: "assistant", Content: m.Content})
				continue
			}
			var blocks []wireBlock
			if m.Content != "" {
				blocks = append(blocks, wireBlock{Type: "text", Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				input, err := canonicalObject(tc.Args)
				if err != nil {
					return nil, "", err
				}
				blocks = append(blocks, wireBlock{Type: "tool_use", ID: tc.ID, Name: tc.Name, Input: input})
			}
			out = append(out, wireMessage{Role: "assistant", Content: blocks})
		default: // user
			out = append(out, wireMessage{Role: "user", Content: m.Content})
		}
	}
	return out, strings.Join(systemParts, "\n\n"), nil
}

// canonicalObject validates that raw JSON is an object (or empty, which
// becomes {}) and re-encodes it canonically. It is the single engine of
// the args asymmetry, used in BOTH directions: outbound, our string-encoded
// ToolCall.Args become the tool_use `input` object (malformed args are
// rejected on OUR side before any network I/O); inbound, a tool_use input
// object becomes our string-encoded args. Non-object JSON is rejected.
func canonicalObject(raw json.RawMessage) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return json.RawMessage(`{}`), nil
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return nil, err
	}
	canonical, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	return canonical, nil
}

// resolve validates the cache config (only when enabled — disabled is
// wire-inert and not even validated) and returns the effective breakpoint
// budget: 0 stays 0-with-default (→ defaultCacheBreakpoints), explicit
// values must satisfy 1..cacheBreakpointsCap.
func (c CacheConfig) resolve() (int, error) {
	if !c.Enabled {
		return 0, nil
	}
	n := c.MaxBreakpoints
	if n == 0 {
		n = defaultCacheBreakpoints
	}
	if n < 1 || n > cacheBreakpointsCap {
		return 0, fmt.Errorf("CacheConfig.MaxBreakpoints=%d outside 1..%d (0 means unset, default %d)", c.MaxBreakpoints, cacheBreakpointsCap, defaultCacheBreakpoints)
	}
	return n, nil
}

// applyCacheControl places cache_control breakpoints per the documented
// policy, consuming budget in breakpoint order and skipping unavailable
// positions: breakpoint 1 on the FINAL tool definition, breakpoint 2 on
// the system prompt (as a single-block array carrying the SAME joined
// string the disabled path sends). It never touches message content.
func applyCacheControl(wr *wireRequest, system string, budget int) {
	if budget <= 0 {
		return
	}
	if len(wr.Tools) > 0 {
		wr.Tools[len(wr.Tools)-1].CacheControl = &wireCacheControl{Type: cacheControlEphemeral}
		budget--
	}
	if budget > 0 && system != "" {
		wr.System = []wireSystemBlock{{
			Type:         "text",
			Text:         system,
			CacheControl: &wireCacheControl{Type: cacheControlEphemeral},
		}}
	}
}

// mapStopReason maps a Messages API stop_reason onto the shared finish
// vocabulary (same target strings the openaicompat adapter reports):
// tool_use→tool_calls, end_turn/stop_sequence→stop, max_tokens→length.
// Unknown values pass through untouched.
func mapStopReason(stop string) string {
	switch stop {
	case "end_turn", "stop_sequence":
		return "stop"
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	default:
		return stop
	}
}

// Call performs one non-streaming Messages request against the injected
// base URL and normalizes the response.
func (a *Adapter) Call(ctx context.Context, req *adapters.Request) (*adapters.Response, error) {
	cacheBudget, err := a.cfg.Cache.resolve()
	if err != nil {
		return nil, adapters.NewAdapterError(a.Name(), adapters.KindOther, 0, 0,
			fmt.Errorf("%s: rejecting request with invalid cache config (our side): %w", a.Name(), err))
	}
	model := req.Model
	if model == "" {
		model = a.cfg.Model
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = a.cfg.MaxTokens
	}
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}
	msgs, system, err := toWireMessages(req.Messages)
	if err != nil {
		return nil, adapters.NewAdapterError(a.Name(), adapters.KindOther, 0, 0,
			fmt.Errorf("%s: rejecting request with malformed tool arguments (our side): %w", a.Name(), err))
	}
	// System representation: nil (key omitted) when empty — byte-identical
	// to the string+omitempty field this replaced; plain string otherwise.
	// applyCacheControl may re-render it as marked blocks.
	var sysVal interface{}
	if system != "" {
		sysVal = system
	}
	wr := wireRequest{
		Model:       model,
		MaxTokens:   maxTokens,
		System:      sysVal,
		Messages:    msgs,
		Temperature: req.Temperature,
	}
	for _, ts := range req.Tools {
		schema := ts.Parameters
		if len(bytes.TrimSpace(schema)) == 0 {
			schema = json.RawMessage(`{"type":"object"}`)
		}
		wr.Tools = append(wr.Tools, wireTool{Name: ts.Name, Description: ts.Description, InputSchema: schema})
	}
	applyCacheControl(&wr, system, cacheBudget)
	body, err := json.Marshal(wr)
	if err != nil {
		return nil, adapters.NewAdapterError(a.Name(), adapters.KindOther, 0, 0,
			fmt.Errorf("%s: marshal request: %w", a.Name(), err))
	}

	url := strings.TrimRight(a.cfg.BaseURL, "/") + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, adapters.NewAdapterError(a.Name(), adapters.KindOther, 0, 0,
			fmt.Errorf("%s: build request: %w", a.Name(), err))
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", a.cfg.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := a.http.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil || isTimeout(err) {
			return nil, adapters.TimeoutError(a.Name(), err)
		}
		return nil, adapters.TransportError(a.Name(), err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, adapters.TransportError(a.Name(), err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, adapters.HTTPStatusError(a.Name(), resp.StatusCode, retryAfterMs(resp.Header.Get("retry-after")), truncate(respBody, 512))
	}
	var wire wireResponse
	if err := json.Unmarshal(respBody, &wire); err != nil {
		return nil, adapters.NewAdapterError(a.Name(), adapters.KindOther, resp.StatusCode, 0,
			fmt.Errorf("%s: decode response: %w", a.Name(), err))
	}
	var content strings.Builder
	var toolCalls []adapters.ToolCall
	for _, blk := range wire.Content {
		switch blk.Type {
		case "text":
			content.WriteString(blk.Text)
		case "tool_use":
			args, err := canonicalObject(blk.Input)
			if err != nil {
				return nil, adapters.NewAdapterError(a.Name(), adapters.KindOther, 0, 0,
					fmt.Errorf("%s: tool %q returned a malformed tool_use input (not a JSON object): %w", a.Name(), blk.Name, err))
			}
			toolCalls = append(toolCalls, adapters.ToolCall{ID: blk.ID, Name: blk.Name, Args: args})
		}
	}
	if content.Len() == 0 && len(toolCalls) == 0 {
		return nil, adapters.EmptyResponseError(a.Name(), model)
	}
	return &adapters.Response{
		Model:     wire.Model,
		Content:   content.String(),
		ToolCalls: toolCalls,
		Usage: adapters.Usage{
			PromptTokens:     wire.Usage.InputTokens,
			CompletionTokens: wire.Usage.OutputTokens,
			TotalTokens:      wire.Usage.InputTokens + wire.Usage.OutputTokens,
		},
		FinishReason: mapStopReason(wire.StopReason),
	}, nil
}

// retryAfterMs parses a retry-after header into milliseconds, accepting
// both the delta-seconds form (what Anthropic sends) and the HTTP-date
// form. Absent or unparseable values yield 0 (no hint).
func retryAfterMs(header string) int64 {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0
	}
	if secs, err := strconv.Atoi(header); err == nil {
		if secs < 0 {
			secs = 0
		}
		return int64(secs) * 1000
	}
	if at, err := http.ParseTime(header); err == nil {
		if ms := time.Until(at).Milliseconds(); ms > 0 {
			return ms
		}
	}
	return 0
}

// isTimeout reports whether a transport error is a timeout class.
func isTimeout(err error) bool {
	var nerr net.Error
	if errors.As(err, &nerr) && nerr.Timeout() {
		return true
	}
	return errors.Is(err, context.DeadlineExceeded)
}

func truncate(b []byte, n int) string {
	s := string(b)
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
