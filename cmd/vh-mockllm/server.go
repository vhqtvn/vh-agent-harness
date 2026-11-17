// server.go — the scripted mock LLM HTTP server.
//
// Endpoints (control plane + the two LLM dialects):
//
//	GET  /healthz              readiness probe → {"ok":true}
//	GET  /count                per-path POST counters (LLM endpoints only)
//	GET|POST /reset            clear counters — NOT the script cursor, NOT the journal
//	GET  /journal?since=N      journal entries with seq > N (1-based)
//	POST /v1/chat/completions  OpenAI dialect (Authorization header required)
//	POST /v1/messages          Anthropic dialect (x-api-key header required)
//
// Script steps are consumed FIFO GLOBALLY across both dialect endpoints
// (arrival order is the ordering contract; the mutex serializes it).
// Control endpoints are never counted or journaled — /count and /journal
// describe the MODEL REQUEST PLANE only. Auth-header PRESENCE is checked
// on the LLM endpoints (401 without one) BEFORE any step is consumed:
// an auth failure is a protocol error, not a scripted response.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"sync"
)

// journalEntry is one recorded request. Auth carries header PRESENCE
// ONLY — the key value is never recorded anywhere (redaction
// discipline; the disk journal mirrors the same shape).
type journalEntry struct {
	Seq    int             `json:"seq"`
	Method string          `json:"method"`
	Path   string          `json:"path"`
	Auth   bool            `json:"auth"`
	Body   json.RawMessage `json:"body,omitempty"`
}

// mockServer serves the scripted steps.
type mockServer struct {
	mu         sync.Mutex
	scriptPath string
	steps      []Step
	cursor     int
	counters   map[string]int
	journal    []journalEntry
	chatN      int      // OpenAI-dialect response counter (mock-chatcmpl-N)
	msgN       int      // Anthropic-dialect response counter (mock-msg-N)
	disk       *os.File // optional append-only JSONL journal mirror
}

func newMockServer(scriptPath string, steps []Step) *mockServer {
	return &mockServer{
		scriptPath: scriptPath,
		steps:      steps,
		counters:   map[string]int{},
	}
}

// setDiskJournal attaches the on-disk journal mirror (already open for
// append). Writes are best-effort: a failing disk journal must never
// break the mock's responses.
func (s *mockServer) setDiskJournal(f *os.File) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.disk = f
}

func (s *mockServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/healthz":
		writeJSON(w, 200, map[string]any{"ok": true})
	case r.URL.Path == "/count":
		s.mu.Lock()
		snapshot := make(map[string]int, len(s.counters))
		for k, v := range s.counters {
			snapshot[k] = v
		}
		s.mu.Unlock()
		writeJSON(w, 200, snapshot)
	case r.URL.Path == "/reset":
		s.mu.Lock()
		s.counters = map[string]int{}
		s.mu.Unlock()
		writeJSON(w, 200, map[string]any{"ok": true})
	case r.Method == http.MethodGet && r.URL.Path == "/journal":
		since := int64(0)
		if v := r.URL.Query().Get("since"); v != "" {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil || n < 0 {
				writeJSON(w, 400, map[string]any{"error": "since must be a non-negative integer"})
				return
			}
			since = n
		}
		s.mu.Lock()
		out := make([]journalEntry, 0, len(s.journal))
		for _, e := range s.journal {
			if int64(e.Seq) > since {
				out = append(out, e)
			}
		}
		s.mu.Unlock()
		writeJSON(w, 200, out)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions":
		s.serveLLM(w, r, dialectOpenAI)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/messages":
		s.serveLLM(w, r, dialectAnthropic)
	default:
		writeJSON(w, 404, map[string]any{"error": fmt.Sprintf("unknown path %s %s", r.Method, r.URL.Path)})
	}
}

// dialect selects the request/response projection.
type dialect int

const (
	dialectOpenAI dialect = iota
	dialectAnthropic
)

func (d dialect) path() string {
	if d == dialectAnthropic {
		return "/v1/messages"
	}
	return "/v1/chat/completions"
}

// authPresent reports whether the dialect's auth header is present.
func (d dialect) authPresent(r *http.Request) bool {
	if d == dialectAnthropic {
		return r.Header.Get("x-api-key") != ""
	}
	return r.Header.Get("Authorization") != ""
}

// serveLLM handles one LLM-dialect request: count, journal, auth check,
// body validation, script step consumption, dialect rendering.
func (s *mockServer) serveLLM(w http.ResponseWriter, r *http.Request, d dialect) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
	_ = r.Body.Close()
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": fmt.Sprintf("read body: %v", err)})
		return
	}
	auth := d.authPresent(r)

	s.mu.Lock()
	s.counters[d.path()]++
	var raw json.RawMessage
	if json.Valid(body) {
		raw = json.RawMessage(body)
	}
	s.journal = append(s.journal, journalEntry{
		Seq:    len(s.journal) + 1,
		Method: r.Method,
		Path:   r.URL.Path,
		Auth:   auth,
		Body:   raw,
	})
	if s.disk != nil {
		line, merr := json.Marshal(journalEntry{
			Seq: len(s.journal), Method: r.Method, Path: r.URL.Path, Auth: auth, Body: raw,
		})
		if merr == nil {
			_, _ = s.disk.Write(append(line, '\n'))
		}
	}
	if !auth {
		s.mu.Unlock()
		writeJSON(w, 401, map[string]any{"error": map[string]any{
			"message": "missing auth header (" + map[bool]string{true: "x-api-key", false: "Authorization"}[d == dialectAnthropic] + ")",
			"type":    "authentication_error",
		}})
		return
	}
	if !json.Valid(body) {
		s.mu.Unlock()
		writeJSON(w, 400, map[string]any{"error": "request body is not valid JSON"})
		return
	}
	if s.cursor >= len(s.steps) {
		path := s.scriptPath
		n := len(s.steps)
		s.mu.Unlock()
		writeJSON(w, 500, map[string]any{"error": map[string]any{
			"message": fmt.Sprintf("mock script exhausted: %s had %d steps and all were consumed — the test scripted too few responses (a test bug, never a provider condition)", path, n),
			"type":    "mock_script_exhausted",
		}})
		return
	}
	step := s.steps[s.cursor]
	s.cursor++
	if d == dialectOpenAI {
		s.chatN++
	} else {
		s.msgN++
	}
	chatN, msgN := s.chatN, s.msgN
	s.mu.Unlock()

	var model string
	_ = json.Unmarshal(body, &struct {
		Model *string `json:"model"`
	}{Model: &model})
	if model == "" {
		model = "mock-model"
	}

	// Faults are dialect-independent: status + verbatim body + optional
	// Retry-After (seconds form, ceil).
	if step.Fault != nil {
		if step.Fault.RetryAfterMs > 0 {
			w.Header().Set("Retry-After", strconv.FormatInt(int64(math.Ceil(float64(step.Fault.RetryAfterMs)/1000.0)), 10))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(step.Fault.Status)
		if step.Fault.Body != "" {
			_, _ = io.WriteString(w, step.Fault.Body)
		} else {
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
				"message": fmt.Sprintf("mock fault %d (no body scripted)", step.Fault.Status),
				"type":    "mock_fault",
			}})
		}
		return
	}

	if d == dialectOpenAI {
		s.writeOpenAI(w, step, model, chatN)
		return
	}
	s.writeAnthropic(w, step, model, msgN)
}

// ---- OpenAI dialect -------------------------------------------------------
//
// Wire shapes mirror internal/adapters/openaicompat's contract tests:
// tool arguments are STRING-encoded; assistant content on a tool-call
// message is "" (never null); the empty class carries choices with a
// message that has NO content field.

type oaFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type oaToolCall struct {
	ID       string     `json:"id"`
	Type     string     `json:"type"`
	Function oaFunction `json:"function"`
}

type oaMessage struct {
	Role string `json:"role"`
	// Content is a *string so the tool-call case can emit the literal
	// "" (present, never null — the adapter contract) while the empty
	// class omits the field entirely.
	Content   *string      `json:"content,omitempty"`
	ToolCalls []oaToolCall `json:"tool_calls,omitempty"`
}

type oaChoice struct {
	Index        int       `json:"index"`
	Message      oaMessage `json:"message"`
	FinishReason string    `json:"finish_reason"`
}

type oaUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type oaResponse struct {
	ID      string     `json:"id"`
	Object  string     `json:"object"`
	Model   string     `json:"model"`
	Choices []oaChoice `json:"choices"`
	Usage   oaUsage    `json:"usage"`
}

func (s *mockServer) writeOpenAI(w http.ResponseWriter, step Step, model string, n int) {
	resp := oaResponse{
		ID:     fmt.Sprintf("mock-chatcmpl-%d", n),
		Object: "chat.completion",
		Model:  model,
		Choices: []oaChoice{{
			Index:   0,
			Message: oaMessage{Role: "assistant"},
		}},
		Usage: oaUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}
	switch {
	case step.Text != "":
		c := step.Text
		resp.Choices[0].Message.Content = &c
		resp.Choices[0].FinishReason = "stop"
	case len(step.ToolCalls) > 0:
		calls := make([]oaToolCall, 0, len(step.ToolCalls))
		for _, tc := range step.ToolCalls {
			calls = append(calls, oaToolCall{
				ID:       tc.ID,
				Type:     "function",
				Function: oaFunction{Name: tc.Name, Arguments: compactJSON(tc.Args)},
			})
		}
		empty := "" // present, empty string — never null
		resp.Choices[0].Message.Content = &empty
		resp.Choices[0].Message.ToolCalls = calls
		resp.Choices[0].FinishReason = "tool_calls"
	case step.Empty:
		// choices present, message carries role but NO content.
		resp.Choices[0].FinishReason = "stop"
		resp.Usage = oaUsage{PromptTokens: 1, CompletionTokens: 0, TotalTokens: 1}
	}
	writeJSON(w, 200, resp)
}

// ---- Anthropic dialect ----------------------------------------------------
//
// Wire shapes mirror internal/adapters/anthropic's contract tests:
// tool_use blocks carry the input OBJECT; the empty class is content:[].

type anTextBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anToolUseBlock struct {
	Type  string          `json:"type"`
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type anResponse struct {
	ID         string         `json:"id"`
	Role       string         `json:"role"`
	Model      string         `json:"model"`
	Content    []any          `json:"content"`
	StopReason string         `json:"stop_reason"`
	Usage      map[string]int `json:"usage"`
}

func (s *mockServer) writeAnthropic(w http.ResponseWriter, step Step, model string, n int) {
	resp := anResponse{
		ID:      fmt.Sprintf("mock-msg-%d", n),
		Role:    "assistant",
		Model:   model,
		Content: []any{},
		Usage:   map[string]int{"input_tokens": 10, "output_tokens": 5},
	}
	switch {
	case step.Text != "":
		resp.Content = []any{anTextBlock{Type: "text", Text: step.Text}}
		resp.StopReason = "end_turn"
	case len(step.ToolCalls) > 0:
		for _, tc := range step.ToolCalls {
			input := tc.Args
			if len(input) == 0 {
				input = json.RawMessage("{}")
			}
			resp.Content = append(resp.Content, anToolUseBlock{Type: "tool_use", ID: tc.ID, Name: tc.Name, Input: input})
		}
		resp.StopReason = "tool_use"
	case step.Empty:
		resp.Content = []any{}
		resp.StopReason = "end_turn"
		resp.Usage = map[string]int{"input_tokens": 1, "output_tokens": 0}
	}
	writeJSON(w, 200, resp)
}

// compactJSON re-serializes raw JSON compactly (deterministic wire bytes
// regardless of script formatting). nil → "{}".
func compactJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return string(raw)
	}
	return buf.String()
}

// writeJSON marshals v and writes it with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
