// prompt_optimizer_test.go — the --optimizer wiring matrix over the real
// run() surface: default llm without a key fails closed with exit 2
// (naming the missing piece, never silently deduping); --optimizer dedup
// reproduces the reference-fake behavior byte-identically offline; and
// --optimizer llm with a key drives the REAL adapter (openaicompat over
// an httptest stub that echoes the sections back) → adapter called →
// artifact written with the llmopt version → the serving rule finds it.
package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/prompt"
)

// TestCompilePromptOptimizerLLMDefaultWithoutKeyExits2: the DEFAULT
// optimizer is llm; without the key environment variable the compile
// refuses with exit 2, naming the missing piece and the dedup escape —
// fail-closed, no silent dedup fallback.
func TestCompilePromptOptimizerLLMDefaultWithoutKeyExits2(t *testing.T) {
	dir := t.TempDir()
	var out, errBuf safeBuffer
	code := run([]string{
		"--adapter", "openai", "--model", "m", "--base-url", "http://127.0.0.1:1",
		"--api-key-env", "VH_AGENTD_LLMOPT_KEY", "--session-dir", dir,
		"--compile-prompt",
	}, func(string) string { return "" }, nil, &out, &errBuf)
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (stderr: %s)", code, errBuf.String())
	}
	stderr := errBuf.String()
	for _, want := range []string{
		"VH_AGENTD_LLMOPT_KEY",
		"fail-closed",
		"--optimizer dedup",
	} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("stderr missing %q: %s", want, stderr)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "compiled-prompts")); !os.IsNotExist(err) {
		t.Fatal("no artifact directory may be created on the refused run")
	}
}

// TestCompilePromptOptimizerDedupByteIdentical: --optimizer dedup keeps
// the reference-fake path — offline (no key), and the artifact file is
// byte-identical to a direct prompt.Compile with prompt.Dedup over the
// same inputs (deterministic artifacts).
func TestCompilePromptOptimizerDedupByteIdentical(t *testing.T) {
	dir := t.TempDir()
	var out, errBuf safeBuffer
	code := run([]string{
		"--adapter", "openai", "--model", "m", "--base-url", "http://127.0.0.1:1",
		"--api-key-env", "VH_AGENTD_LLMOPT_KEY", "--session-dir", dir,
		"--compile-prompt", "--optimizer", "dedup",
	}, func(string) string { return "" }, nil, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "optimizer=dedup-fake/1") {
		t.Fatalf("stderr report missing dedup optimizer version: %s", errBuf.String())
	}

	// Ground truth: the same compile through the library seam.
	cfg, err := validate("openai", "m", "http://127.0.0.1:1", "VH_AGENTD_LLMOPT_KEY", dir, "dedup", 0, defaultApprovalTimeoutMs, 0, "off")
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	specs := toolSpecsForPrompt(cfg)
	asm, vars, catalog, err := buildPromptInputs(cfg, specs)
	if err != nil {
		t.Fatalf("buildPromptInputs: %v", err)
	}
	refDir := t.TempDir()
	if _, err := prompt.Compile(context.Background(), asm, vars, catalog, prompt.Dedup, promptContract(), refDir); err != nil {
		t.Fatalf("reference compile: %v", err)
	}
	hash, err := prompt.InputHash(asm, vars, catalog, prompt.DedupOptimizerVersion, promptContract())
	if err != nil {
		t.Fatalf("InputHash: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "compiled-prompts", "prompt-"+hash+".json"))
	if err != nil {
		t.Fatalf("daemon artifact: %v", err)
	}
	want, err := os.ReadFile(filepath.Join(refDir, "prompt-"+hash+".json"))
	if err != nil {
		t.Fatalf("reference artifact: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("dedup artifact differs from the reference-fake compile (%d vs %d bytes)", len(got), len(want))
	}
}

// echoOptimizerLLM is an httptest LLM that PARSES the optimizer's user
// payload and echoes every section body back verbatim inside a
// ```json fence — the identity LLM. The daemon's section set is
// all-required, so a compliant model returns bodies unchanged; the
// fence exercises fence-stripping on the full daemon path.
func echoOptimizerLLM(t *testing.T) (*httptest.Server, *int) {
	t.Helper()
	var mu sync.Mutex
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("echoOptimizerLLM: decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var user string
		for _, m := range req.Messages {
			if m.Role == "user" {
				user = m.Content
			}
		}
		idx := strings.Index(user, `{"sections"`)
		if idx < 0 {
			t.Errorf("echoOptimizerLLM: no sections payload in user message: %q", user)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var payload struct {
			Sections []struct {
				Key  string `json:"key"`
				Body string `json:"body"`
			} `json:"sections"`
		}
		if err := json.Unmarshal([]byte(user[idx:]), &payload); err != nil {
			t.Errorf("echoOptimizerLLM: decode sections: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		out := make(map[string]string, len(payload.Sections))
		for _, s := range payload.Sections {
			out[s.Key] = s.Body
		}
		mapJSON, _ := json.Marshal(out)
		mu.Lock()
		calls++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-echo", "object": "chat.completion", "model": "stub-model",
			"choices": []map[string]any{{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": "```json\n" + string(mapJSON) + "\n```",
				},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20},
		})
	}))
	return ts, &calls
}

// TestCompilePromptOptimizerLLMEndToEndStubAdapter is the daemon-side
// crux: --optimizer llm with the key present drives the REAL adapter
// against the stub (exactly one call), the artifact lands with the
// llmopt/v1/<model> version and passes every invariant, and the SERVING
// rule — using the deterministic version derivation, no key, no network
// — finds and serves the compiled bytes.
func TestCompilePromptOptimizerLLMEndToEndStubAdapter(t *testing.T) {
	stub, calls := echoOptimizerLLM(t)
	defer stub.Close()

	dir := t.TempDir()
	var out, errBuf safeBuffer
	code := run([]string{
		"--adapter", "openai", "--model", "stub-model", "--base-url", stub.URL,
		"--api-key-env", "VH_AGENTD_LLMOPT_KEY", "--session-dir", dir,
		"--compile-prompt",
	}, func(k string) string {
		if k == "VH_AGENTD_LLMOPT_KEY" {
			return "stub-key"
		}
		return ""
	}, nil, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, errBuf.String())
	}
	if out.String() != "" {
		t.Fatalf("stdout = %q, want empty (stdout is protocol)", out.String())
	}
	stderr := errBuf.String()
	if !strings.Contains(stderr, "optimizer=llmopt/v1/stub-model") {
		t.Fatalf("stderr report missing llm optimizer version: %s", stderr)
	}
	if *calls != 1 {
		t.Fatalf("adapter calls = %d, want exactly 1 (single compile-time call, no retries)", *calls)
	}

	// Artifact: llmopt version, all invariants passed.
	cfg, err := validate("openai", "stub-model", stub.URL, "VH_AGENTD_LLMOPT_KEY", dir, "", 0, defaultApprovalTimeoutMs, 0, "off")
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	specs := toolSpecsForPrompt(cfg)
	asm, vars, catalog, err := buildPromptInputs(cfg, specs)
	if err != nil {
		t.Fatalf("buildPromptInputs: %v", err)
	}
	hash, err := prompt.InputHash(asm, vars, catalog, servingOptimizerVersion(cfg), promptContract())
	if err != nil {
		t.Fatalf("InputHash: %v", err)
	}
	art, err := prompt.LoadCompiled(promptArtifactDir(cfg), hash)
	if err != nil {
		t.Fatalf("LoadCompiled llm artifact: %v", err)
	}
	if art.OptimizerVersion != "llmopt/v1/stub-model" {
		t.Fatalf("artifact optimizer_version = %q", art.OptimizerVersion)
	}
	for _, iv := range art.Invariants {
		if !iv.Passed {
			t.Fatalf("invariant %s failed: %s", iv.Name, iv.Detail)
		}
	}

	// Serving rule: the daemon finds the llm-compiled artifact with the
	// deterministic version derivation — offline, no key, no network.
	served, res, err := resolveSystemPrompt(cfg, specs)
	if err != nil {
		t.Fatalf("resolveSystemPrompt: %v", err)
	}
	if res.Source != prompt.ServeSourceCompiled || res.Reason != "" {
		t.Fatalf("serve = %+v, want compiled/no-reason", res)
	}
	if served != string(art.Bytes) {
		t.Fatalf("served bytes != artifact bytes (%d vs %d)", len(served), len(art.Bytes))
	}
}
