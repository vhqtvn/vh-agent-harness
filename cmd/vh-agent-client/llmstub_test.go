// llmstub_test.go — the scripted OpenAI-chat-completions stub for the
// binary-level e2e battery (mirrors cmd/vh-agentd's fakeLLM/shellLLM
// pattern, expressed as one reusable encoder).
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// jsonEncoder encodes scripted chat-completions responses.
type jsonEncoder struct {
	w http.ResponseWriter
}

func (e *jsonEncoder) toolCall(callID, tool, args string) {
	_ = json.NewEncoder(e.w).Encode(map[string]any{
		"id": "chatcmpl-stub", "object": "chat.completion", "model": "fake-model",
		"choices": []map[string]any{{
			"index": 0,
			"message": map[string]any{
				"role": "assistant", "content": "",
				"tool_calls": []map[string]any{{
					"id": callID, "type": "function",
					"function": map[string]any{"name": tool, "arguments": args},
				}},
			},
			"finish_reason": "tool_calls",
		}},
		"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5},
	})
}

func (e *jsonEncoder) content(text string) {
	_ = json.NewEncoder(e.w).Encode(map[string]any{
		"id": "chatcmpl-stub2", "object": "chat.completion", "model": "fake-model",
		"choices": []map[string]any{{
			"index": 0, "message": map[string]any{"role": "assistant", "content": text},
			"finish_reason": "stop",
		}},
		"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 1, "total_tokens": 4},
	})
}

// startHTTPStub serves the scripted handler; the callback receives a
// fresh encoder per request.
func startHTTPStub(t *testing.T, script func(*jsonEncoder)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		script(&jsonEncoder{w: w})
	}))
}
