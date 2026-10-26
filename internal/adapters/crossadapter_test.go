// Cross-adapter contract equivalence: the SAME logical adapters.Request,
// rendered by the openaicompat and anthropic adapters, must produce wire
// bodies that are semantically equivalent (same conversation, same tool
// advertisement, same argument JSON). This is the proof the shared
// Request/Response contract is provider-neutral — internal/loop and
// internal/tools can sit on either adapter unchanged.
package adapters_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
	"github.com/vhqtvn/vh-agent-harness/internal/adapters/anthropic"
	"github.com/vhqtvn/vh-agent-harness/internal/adapters/openaicompat"
)

// captureServer records the single request body it receives and replies
// with a fixed dialect-appropriate success body.
type captureServer struct {
	*httptest.Server
	once sync.Once
	body []byte
}

func newCaptureServer(t *testing.T, reply string) *captureServer {
	cs := &captureServer{}
	cs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("capture server: read body: %v", err)
		}
		cs.once.Do(func() { cs.body = body })
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(cs.Close)
	return cs
}

const openAIReply = `{"id":"c","model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`
const anthropicReply = `{"id":"m","role":"assistant","model":"m","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`

func TestOpenAIAndAnthropicWireEquivalence(t *testing.T) {
	temp := 0.2
	req := &adapters.Request{
		Model:       "shared-model",
		Temperature: &temp,
		MaxTokens:   128,
		Messages: []adapters.Message{
			{Role: "system", Content: "You are terse."},
			{Role: "user", Content: "echo hello"},
			{Role: "assistant", Content: "", ToolCalls: []adapters.ToolCall{
				{ID: "call_1", Name: "echo", Args: json.RawMessage(`{"text":"hello"}`)},
			}},
			{Role: "tool", ToolCallID: "call_1", Name: "echo", Content: `{"text":"hello"}`},
		},
		Tools: []adapters.ToolSpec{{Name: "echo", Description: "echo it", Parameters: json.RawMessage(`{"type":"object"}`)}},
	}

	oaSrv := newCaptureServer(t, openAIReply)
	oa := openaicompat.New(openaicompat.Config{Provider: "mock-openai", BaseURL: oaSrv.URL, Model: "shared-model", APIKey: "k"})
	if _, err := oa.Call(context.Background(), req); err != nil {
		t.Fatalf("openaicompat Call: %v", err)
	}
	anSrv := newCaptureServer(t, anthropicReply)
	an := anthropic.New(anthropic.Config{Provider: "mock-anthropic", BaseURL: anSrv.URL, Model: "shared-model", APIKey: "k"})
	if _, err := an.Call(context.Background(), req); err != nil {
		t.Fatalf("anthropic Call: %v", err)
	}

	var oaBody, anBody map[string]any
	if err := json.Unmarshal(oaSrv.body, &oaBody); err != nil {
		t.Fatalf("openai body not JSON: %v", err)
	}
	if err := json.Unmarshal(anSrv.body, &anBody); err != nil {
		t.Fatalf("anthropic body not JSON: %v", err)
	}

	// Invariant 1: the system prompt survives on BOTH wires, each in its
	// dialect-correct place (openai: system-role message; anthropic:
	// top-level system, no system message).
	oaMsgs := oaBody["messages"].([]any)
	oaFirst := oaMsgs[0].(map[string]any)
	if oaFirst["role"] != "system" || oaFirst["content"] != "You are terse." {
		t.Fatalf("openai system message = %v", oaFirst)
	}
	if anBody["system"] != "You are terse." {
		t.Fatalf("anthropic top-level system = %v", anBody["system"])
	}
	for _, m := range anBody["messages"].([]any) {
		if m.(map[string]any)["role"] == "system" {
			t.Fatal("anthropic messages must not contain a system role")
		}
	}

	// Invariant 2: both advertise the same tool name with the same schema.
	oaTool := oaBody["tools"].([]any)[0].(map[string]any)["function"].(map[string]any)
	anTool := anBody["tools"].([]any)[0].(map[string]any)
	if oaTool["name"] != "echo" || anTool["name"] != "echo" {
		t.Fatalf("tool names = openai %v / anthropic %v", oaTool["name"], anTool["name"])
	}
	if !jsonEqual(t, oaTool["parameters"], anTool["input_schema"]) {
		t.Fatalf("tool schemas differ: openai %v / anthropic %v", oaTool["parameters"], anTool["input_schema"])
	}

	// Invariant 3: the assistant tool call's arguments are the same JSON
	// after unmarshal (string-encoded on the openai wire, object on the
	// anthropic wire — the documented asymmetry).
	oaAssistant := oaMsgs[2].(map[string]any)
	oaCall := oaAssistant["tool_calls"].([]any)[0].(map[string]any)["function"].(map[string]any)
	anMsgs := anBody["messages"].([]any)
	anAssistant := anMsgs[1].(map[string]any)
	anBlocks := anAssistant["content"].([]any)
	var anInput any
	for _, b := range anBlocks {
		blk := b.(map[string]any)
		if blk["type"] == "tool_use" {
			anInput = blk["input"]
		}
	}
	if anInput == nil {
		t.Fatalf("no tool_use block on anthropic wire: %v", anAssistant)
	}
	var oaArgs any
	if err := json.Unmarshal([]byte(oaCall["arguments"].(string)), &oaArgs); err != nil {
		t.Fatalf("openai arguments not JSON: %v", err)
	}
	if !jsonEqual(t, oaArgs, anInput) {
		t.Fatalf("tool arguments differ: openai %v / anthropic %v", oaArgs, anInput)
	}

	// Invariant 4: the tool result is carried with the same correlation id
	// (openai: role=tool + tool_call_id; anthropic: user tool_result block
	// with tool_use_id).
	oaToolMsg := oaMsgs[3].(map[string]any)
	if oaToolMsg["role"] != "tool" || oaToolMsg["tool_call_id"] != "call_1" {
		t.Fatalf("openai tool message = %v", oaToolMsg)
	}
	anToolTurn := anMsgs[2].(map[string]any)
	if anToolTurn["role"] != "user" {
		t.Fatalf("anthropic tool_result carrier role = %v", anToolTurn["role"])
	}
	anTr := anToolTurn["content"].([]any)[0].(map[string]any)
	if anTr["type"] != "tool_result" || anTr["tool_use_id"] != "call_1" {
		t.Fatalf("anthropic tool_result block = %v", anTr)
	}

	// Invariant 5: both wires pass the shared call parameters through.
	if oaBody["model"] != "shared-model" || anBody["model"] != "shared-model" {
		t.Fatalf("model passthrough differs: %v / %v", oaBody["model"], anBody["model"])
	}
	if oaBody["max_tokens"] != anBody["max_tokens"] {
		t.Fatalf("max_tokens differ: %v / %v", oaBody["max_tokens"], anBody["max_tokens"])
	}
}

// jsonEqual compares two decoded JSON values structurally.
func jsonEqual(t *testing.T, a, b any) bool {
	t.Helper()
	ab, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal a: %v", err)
	}
	bb, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal b: %v", err)
	}
	return string(ab) == string(bb)
}
