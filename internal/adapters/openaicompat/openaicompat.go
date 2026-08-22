// Package openaicompat implements the adapters.Adapter boundary for any
// OpenAI-compatible chat-completions endpoint. Non-streaming only (slice
// 1); the adapter talks exclusively to the injected base URL and never
// reads the environment.
//
// Failure classification mirrors the anthropic adapter exactly: every
// failure surfaces as a typed *adapters.AdapterError (HTTPStatusError for
// non-2xx with the Retry-After hint, TimeoutError/TransportError for
// deadline vs wire failures, EmptyResponseError for a 200 carrying
// neither content nor tool calls), so loop.Classify never has to fold an
// untyped error into transport — a 401/403 or 4xx is non-retryable and
// must not burn the retry ladder.
package openaicompat

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
const DefaultProviderName = "openaicompat"

// maxResponseBytes caps response-body reads (a later spill/oversize story
// may raise this; slice 1 fails closed beyond the cap).
const maxResponseBytes = 32 << 20

// Config is the complete adapter configuration: the injected base URL,
// model, and API key. No environment reading happens inside the adapter.
type Config struct {
	Provider string
	BaseURL  string
	Model    string
	APIKey   string
}

// Adapter speaks the OpenAI chat-completions wire format (non-streaming)
// against an injected base URL.
type Adapter struct {
	cfg  Config
	http *http.Client
}

// New builds an adapter from config.
func New(cfg Config) *Adapter {
	return &Adapter{cfg: cfg, http: &http.Client{}}
}

// Name returns the provider name (default openaicompat).
func (a *Adapter) Name() string {
	if a.cfg.Provider != "" {
		return a.cfg.Provider
	}
	return DefaultProviderName
}

// --- wire types (chat-completions dialect) ---

type wireMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content"` // always a string; "" never null
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type wireToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function wireFunction `json:"function"`
}

type wireFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON-encoded args as a string
}

type wireTool struct {
	Type     string       `json:"type"`
	Function wireToolFunc `json:"function"`
}

type wireToolFunc struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type wireRequest struct {
	Model       string        `json:"model"`
	Messages    []wireMessage `json:"messages"`
	Tools       []wireTool    `json:"tools,omitempty"`
	Temperature *float64      `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

type wireUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type wireResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Index        int         `json:"index"`
		Message      wireMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage wireUsage `json:"usage"`
}

func toWireMessages(msgs []adapters.Message) []wireMessage {
	out := make([]wireMessage, 0, len(msgs))
	for _, m := range msgs {
		wm := wireMessage{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID}
		for _, tc := range m.ToolCalls {
			wm.ToolCalls = append(wm.ToolCalls, wireToolCall{
				ID:       tc.ID,
				Type:     "function",
				Function: wireFunction{Name: tc.Name, Arguments: string(tc.Args)},
			})
		}
		out = append(out, wm)
	}
	return out
}

// Call performs one non-streaming chat-completions request against the
// injected base URL and normalizes the response.
func (a *Adapter) Call(ctx context.Context, req *adapters.Request) (*adapters.Response, error) {
	model := req.Model
	if model == "" {
		model = a.cfg.Model
	}
	wr := wireRequest{
		Model:       model,
		Messages:    toWireMessages(req.Messages),
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	}
	for _, ts := range req.Tools {
		wr.Tools = append(wr.Tools, wireTool{
			Type:     "function",
			Function: wireToolFunc{Name: ts.Name, Description: ts.Description, Parameters: ts.Parameters},
		})
	}
	body, err := json.Marshal(wr)
	if err != nil {
		return nil, adapters.NewAdapterError(a.Name(), adapters.KindOther, 0, 0,
			fmt.Errorf("%s: marshal request: %w", a.Name(), err))
	}

	url := strings.TrimRight(a.cfg.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, adapters.NewAdapterError(a.Name(), adapters.KindOther, 0, 0,
			fmt.Errorf("%s: build request: %w", a.Name(), err))
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if a.cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+a.cfg.APIKey)
	}

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
		// Redact BEFORE truncating: a provider can ECHO the API key in
		// an error body, and an occurrence straddling the 512-byte
		// excerpt boundary must not survive as a partial fragment.
		// Excerpt length behavior is unchanged (still capped at 512) —
		// only the key value is substituted (adapters.RedactSecret).
		return nil, adapters.HTTPStatusError(a.Name(), resp.StatusCode, retryAfterMs(resp.Header.Get("retry-after")),
			truncate([]byte(adapters.RedactSecret(string(respBody), a.cfg.APIKey)), 512))
	}

	var wire wireResponse
	if err := json.Unmarshal(respBody, &wire); err != nil {
		return nil, adapters.NewAdapterError(a.Name(), adapters.KindOther, resp.StatusCode, 0,
			fmt.Errorf("%s: decode response: %w", a.Name(), err))
	}
	if len(wire.Choices) == 0 {
		return nil, adapters.EmptyResponseError(a.Name(), model)
	}
	choice := wire.Choices[0]
	if choice.Message.Content == "" && len(choice.Message.ToolCalls) == 0 {
		return nil, adapters.EmptyResponseError(a.Name(), model)
	}

	out := &adapters.Response{
		Model:        wire.Model,
		Content:      choice.Message.Content,
		Usage:        adapters.Usage{PromptTokens: wire.Usage.PromptTokens, CompletionTokens: wire.Usage.CompletionTokens, TotalTokens: wire.Usage.TotalTokens},
		FinishReason: choice.FinishReason,
	}
	for _, tc := range choice.Message.ToolCalls {
		args := json.RawMessage(bytes.TrimSpace([]byte(tc.Function.Arguments)))
		if len(args) == 0 {
			args = nil
		} else if !json.Valid(args) {
			return nil, adapters.NewAdapterError(a.Name(), adapters.KindOther, 0, 0,
				fmt.Errorf("%s: tool %q returned malformed JSON arguments", a.Name(), tc.Function.Name))
		}
		out.ToolCalls = append(out.ToolCalls, adapters.ToolCall{ID: tc.ID, Name: tc.Function.Name, Args: args})
	}
	return out, nil
}

func truncate(b []byte, n int) string {
	s := string(b)
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// retryAfterMs parses a retry-after header into milliseconds, accepting
// both the delta-seconds form and the HTTP-date form. Absent or
// unparseable values yield 0 (no hint). Anthropic-adapter parity: the
// same dual-form parser, byte-for-byte semantics.
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
