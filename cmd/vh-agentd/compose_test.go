// compose_test.go — engine assembly tests (httptest-level, no real
// network): both adapter selections yield a live server+engine with the
// dogfood tools registered AFTER the approval bridge; the echo/clock
// tool bodies execute through the real pipeline; and one in-process
// composition E2E (real openaicompat adapter against an httptest fake
// LLM) drives a full tool turn plus a job settlement through the wire.
package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/prompt"
	"github.com/vhqtvn/vh-agent-harness/internal/protocol"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/subagents"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

func testConfig(t *testing.T, adapter, baseURL string) *Config {
	t.Helper()
	cfg, err := validate(adapter, "fake-model", baseURL, "VH_AGENTD_TEST_KEY", t.TempDir(), "", 0, defaultApprovalTimeoutMs, 0, "off", 65536, "")
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	return cfg
}

// TestBuildServerBothAdapters proves the assembly returns a non-nil
// server with both adapter selections and the daemon tool set (echo,
// clock, spill_read, run_shell, the file family, the subagent family)
// registered on the engine's pipeline (post-bridge).
func TestBuildServerBothAdapters(t *testing.T) {
	for _, adapter := range []string{"openai", "anthropic"} {
		t.Run(adapter, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTeapot) // assembly must not call the LLM
			}))
			defer ts.Close()
			cfg := testConfig(t, adapter, ts.URL)

			svc, cli := net.Pipe()
			defer cli.Close()
			srv, eng, tracker, served := buildServer(cfg, "test-key", svc)
			if srv == nil || eng == nil || tracker == nil {
				t.Fatalf("srv=%v eng=%v tracker=%v", srv, eng, tracker)
			}
			if eng.Adapter() == nil || eng.Adapter().Name() == "" {
				t.Fatalf("engine adapter not wired: %v", eng.Adapter())
			}
			got := map[string]bool{}
			for _, d := range eng.Pipeline().Definitions() {
				got[d.Name] = true
			}
			if !got["echo"] || !got["clock"] || !got["run_shell"] || !got["spill_read"] {
				t.Fatalf("daemon tools not registered: %v", got)
			}
			// The model-facing file family rides the same set
			// (confined to the configured workdir roots).
			if !got["read"] || !got["write"] || !got["edit"] || !got["glob"] || !got["search"] {
				t.Fatalf("file tools not registered: %v", got)
			}
			// The model-facing subagent family rides the same set (the
			// root session, depth 0, is always below the fence).
			if !got["subagent_spawn"] || !got["subagent_send"] {
				t.Fatalf("subagent tools not registered: %v", got)
			}
			if len(eng.TurnOptions().Tools) != 11 {
				t.Fatalf("turn options do not advertise all eleven tools: %+v", eng.TurnOptions().Tools)
			}
			if eng.TurnOptions().Retry == nil {
				t.Fatal("retry ladder not armed on the daemon turn path")
			}
			if eng.TurnOptions().System == "" || served.Source == "" {
				t.Fatalf("system prompt not served: %q %+v", eng.TurnOptions().System, served)
			}
			// No artifact in a fresh session dir: the serving rule must
			// have fallen back to raw assembly, explicitly.
			if served.Source != prompt.ServeSourceRawAssembly || served.Reason != prompt.ServeReasonNotFound {
				t.Fatalf("fresh-dir serve result = %+v, want raw-assembly/artifact-not-found", served)
			}
		})
	}
}

// TestDogfoodToolsExecute drives the echo and clock tool bodies through
// the REAL pipeline (waterfall → guards → dispatch → post-observe) with
// an injected clock for determinism.
func TestDogfoodToolsExecute(t *testing.T) {
	fixed := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	offCfg, err := validate("openai", "fake-model", "http://x.test", "VH_AGENTD_TEST_KEY", t.TempDir(), "", 0, defaultApprovalTimeoutMs, 0, "off", 65536, "")
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	p := tools.NewPipeline()
	for _, d := range daemonTools(func() time.Time { return fixed }, offCfg, subagents.NewRegistry(), shellConfigFor(offCfg)) {
		if err := p.Register(d); err != nil {
			t.Fatalf("register %s: %v", d.Name, err)
		}
	}

	r := p.Execute(context.Background(), session.ToolCall{ID: "c1", Name: "echo", Args: json.RawMessage(`{"text":"hello dogfood"}`)})
	if r.IsError || r.Content != "hello dogfood" {
		t.Fatalf("echo result = %+v", r)
	}

	r = p.Execute(context.Background(), session.ToolCall{ID: "c2", Name: "clock", Args: nil})
	if r.IsError || r.Content != fixed.Format(time.RFC3339Nano) {
		t.Fatalf("clock result = %+v", r)
	}

	// Fail-closed arg contracts.
	r = p.Execute(context.Background(), session.ToolCall{ID: "c3", Name: "echo", Args: json.RawMessage(`{}`)})
	if !r.IsError || r.Content == "" {
		t.Fatalf("echo without text = %+v, want error", r)
	}
	r = p.Execute(context.Background(), session.ToolCall{ID: "c4", Name: "clock", Args: json.RawMessage(`{"nope":1}`)})
	if !r.IsError {
		t.Fatalf("clock with stray args = %+v, want error", r)
	}
}

// fakeLLM serves one OpenAI-chat-completions response requesting an echo
// tool call, then plain content afterwards.
func fakeLLM(t *testing.T) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	calls := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "chatcmpl-1", "object": "chat.completion", "model": "fake-model",
				"choices": []map[string]any{{
					"index": 0,
					"message": map[string]any{
						"role": "assistant", "content": "",
						"tool_calls": []map[string]any{{
							"id": "call-1", "type": "function",
							"function": map[string]any{"name": "echo", "arguments": `{"text":"hello-inprocess"}`},
						}},
					},
					"finish_reason": "tool_calls",
				}},
				"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-2", "object": "chat.completion", "model": "fake-model",
			"choices": []map[string]any{{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": "done"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 1, "total_tokens": 4},
		})
	}))
}

// TestInProcessCompositionEndToEnd is the in-process E2E (slice-5
// integration pattern): the daemon's own buildServer composition driven
// by the reference client over a net.Pipe, with the REAL openaicompat
// adapter against an httptest fake LLM — one full tool turn plus one
// settled job.
func TestInProcessCompositionEndToEnd(t *testing.T) {
	llm := fakeLLM(t)
	defer llm.Close()
	cfg := testConfig(t, "openai", llm.URL)
	logPath := filepath.Join(cfg.SessionDir, "sess-inproc.jsonl")

	svc, cli := net.Pipe()
	srv, _, _, _ := buildServer(cfg, "test-key", svc)
	served := make(chan error, 1)
	go func() { served <- srv.Serve(nil) }()
	client := protocol.NewClient(cli)
	defer func() {
		_ = client.Close()
		select {
		case <-served:
		case <-time.After(2 * time.Second):
			t.Errorf("server did not exit")
		}
	}()

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("call: %v", err)
		}
	}
	must(client.Call("initialize", map[string]any{"protocolVersion": 1}, nil))
	must(client.Call("session/create", map[string]any{"path": logPath, "sessionId": "sess-inproc"}, nil))
	must(client.Call("session/subscribe", nil, nil))

	var turn struct {
		Results []tools.Result `json:"results"`
	}
	must(client.Call("session/prompt", map[string]any{"text": "echo something"}, &turn))
	if len(turn.Results) != 1 || turn.Results[0].IsError || turn.Results[0].Content != "hello-inprocess" {
		t.Fatalf("tool round-trip results = %+v", turn.Results)
	}

	var receipt struct {
		JobID string `json:"jobId"`
	}
	must(client.Call("session/dispatch", map[string]any{"kind": "echo"}, &receipt))
	if receipt.JobID != "echo-1" {
		t.Fatalf("jobId = %q", receipt.JobID)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		var st struct {
			Jobs []struct {
				JobID  string `json:"jobId"`
				State  string `json:"state"`
				Result string `json:"result"`
			} `json:"jobs"`
		}
		must(client.Call("jobs/status", nil, &st))
		if len(st.Jobs) == 1 && st.Jobs[0].State == "settled" && st.Jobs[0].Result == "completed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job never settled completed: %+v", st.Jobs)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
