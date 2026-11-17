// llmoptimizer_test.go — table tests for the adapter-backed LLM optimizer
// over an injected call func (deterministic fakes; no network), plus ONE
// crux test through a REAL adapter: openaicompat over an httptest server
// scripted to return the JSON section map → LLMOptimizer → Compile →
// full invariant pass → artifact written. openaicompat is imported here
// (_test only) so the optimizer itself stays injectable at the
// adapters.Adapter.Call seam.
package prompt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
	"github.com/vhqtvn/vh-agent-harness/internal/adapters/openaicompat"
)

// capReq captures every adapters.Request handed to a fake call func and
// replies with the scripted content.
type capReq struct {
	mu       sync.Mutex
	reqs     []*adapters.Request
	content  string
	err      error
	rawBytes [][]byte // deterministic re-marshal of each request, for byte comparisons
}

func (c *capReq) call(ctx context.Context, req *adapters.Request) (*adapters.Response, error) {
	blob, _ := json.Marshal(req) // deterministic: struct/slice fields only
	c.mu.Lock()
	c.reqs = append(c.reqs, req)
	c.rawBytes = append(c.rawBytes, blob)
	c.mu.Unlock()
	if c.err != nil {
		return nil, c.err
	}
	return &adapters.Response{Model: req.Model, Content: c.content, FinishReason: "stop"}, nil
}

func (c *capReq) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.reqs)
}

// newTestLLMOptimizer wires the optimizer over the capture fake.
func newTestLLMOptimizer(t *testing.T, model, content string, err error) (*LLMOptimizer, *capReq) {
	t.Helper()
	cap := &capReq{content: content, err: err}
	o, err2 := NewLLMOptimizer(model, cap.call)
	if err2 != nil {
		t.Fatalf("NewLLMOptimizer: %v", err2)
	}
	return o, cap
}

// llmInput is the standard table-test input: base assembler (identity,
// persona [required], tools.guidance) + read/write catalog.
func llmInput(t *testing.T) OptimizeInput {
	t.Helper()
	sections := mustAssemble(t, baseAssembler(t), nil)
	return OptimizeInput{
		Sections:   sections,
		Catalog:    baseCatalog(t),
		Invariants: InvariantsContract{},
	}
}

func TestLLMOptimizerVersionIsStableDerivedString(t *testing.T) {
	o, _ := newTestLLMOptimizer(t, "test-model", "{}", nil)
	if got := o.Version(); got != "llmopt/v1/test-model" {
		t.Fatalf("Version = %q, want llmopt/v1/test-model", got)
	}
	vo := o.Versioned()
	if vo.Version != "llmopt/v1/test-model" || vo.Fn == nil {
		t.Fatalf("Versioned = %+v", vo)
	}
	// The version is a pure function of the model, not self-reported
	// state: the package-level derivation must agree.
	if LLMOptimizerVersion("test-model") != o.Version() {
		t.Fatal("LLMOptimizerVersion(model) must equal o.Version()")
	}
	if LLMOptimizerVersion("other") == o.Version() {
		t.Fatal("different models must yield different versions (version participates in the artifact hash)")
	}
}

func TestLLMOptimizerConstructorRejectsInvalidInput(t *testing.T) {
	if _, err := NewLLMOptimizer("", func(context.Context, *adapters.Request) (*adapters.Response, error) { return nil, nil }); err == nil {
		t.Fatal("empty model accepted")
	}
	if _, err := NewLLMOptimizer("m", nil); err == nil {
		t.Fatal("nil call func accepted")
	}
}

// TestLLMOptimizerHappyPath is the table-driven core: a well-formed JSON
// object of section bodies optimizes the section set, classifies each
// section honestly (preserved only when the rendered body is unchanged),
// and declares the tool references actually present in the output.
func TestLLMOptimizerHappyPath(t *testing.T) {
	in := llmInput(t)
	resp := `{"identity":"You are the kernel.","persona":"Operate predictably.","tools.guidance":"read reads files; write edits them."}`
	o, _ := newTestLLMOptimizer(t, "test-model", resp, nil)

	out, err := o.Optimize(context.Background(), in)
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}
	byKey := map[string]SectionOutcome{}
	for _, oc := range out.SectionOutcomes {
		byKey[oc.Key] = oc
	}
	if len(out.SectionOutcomes) != len(in.Sections) {
		t.Fatalf("outcomes = %d, want one per input section (%d)", len(out.SectionOutcomes), len(in.Sections))
	}
	if byKey["identity"].Action != ActionRewritten {
		t.Fatalf("rewritten identity = %+v", byKey["identity"])
	}
	if byKey["persona"].Action != ActionPreserved {
		t.Fatalf("verbatim persona = %+v (required sections come back byte-identical → preserved)", byKey["persona"])
	}
	if byKey["tools.guidance"].Action != ActionRewritten {
		t.Fatalf("rewritten tools.guidance = %+v", byKey["tools.guidance"])
	}
	if !strings.Contains(string(out.Bytes), "You are the kernel.") || !strings.Contains(string(out.Bytes), "read reads files") {
		t.Fatalf("output bytes missing optimized bodies:\n%s", out.Bytes)
	}
	if strings.Contains(string(out.Bytes), "harness kernel") {
		t.Fatalf("output bytes still carry the old identity body:\n%s", out.Bytes)
	}
	// Honest tool references: exactly the catalog names present as words.
	if len(out.ReferencedTools) != 2 || out.ReferencedTools[0] != "read" || out.ReferencedTools[1] != "write" {
		t.Fatalf("ReferencedTools = %v, want [read write]", out.ReferencedTools)
	}
}

// TestLLMOptimizerWhitespaceOnlyChangeIsPreserved: a body differing only
// in trailing whitespace renders identically, so it must be classified
// preserved — not rewritten (which would fail the required-section
// invariant without any semantic change).
func TestLLMOptimizerWhitespaceOnlyChangeIsPreserved(t *testing.T) {
	in := llmInput(t)
	resp := `{"persona":"Operate predictably.\n\n\t"}`
	o, _ := newTestLLMOptimizer(t, "m", resp, nil)
	out, err := o.Optimize(context.Background(), in)
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}
	for _, oc := range out.SectionOutcomes {
		if oc.Key == "persona" && oc.Action != ActionPreserved {
			t.Fatalf("whitespace-only persona delta = %+v, want preserved", oc)
		}
	}
}

// TestLLMOptimizerDroppedSections: keys absent from the map are dropped
// with an EMPTY rationale — dropping a required section then fails the
// compile-time invariant (fail-closed), it is never laundered here.
func TestLLMOptimizerDroppedSections(t *testing.T) {
	in := llmInput(t)
	resp := `{"persona":"Operate predictably.","identity":"","tools.guidance":"Use read and write."}`
	o, _ := newTestLLMOptimizer(t, "m", resp, nil)
	out, err := o.Optimize(context.Background(), in)
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}
	byKey := map[string]SectionOutcome{}
	for _, oc := range out.SectionOutcomes {
		byKey[oc.Key] = oc
	}
	if byKey["identity"].Action != ActionDropped {
		t.Fatalf("empty-bodied identity = %+v, want dropped", byKey["identity"])
	}
	if strings.TrimSpace(byKey["identity"].Rationale) != "" {
		t.Fatalf("drop rationale must stay empty so the compile invariant stays authoritative: %+v", byKey["identity"])
	}
	if strings.Contains(string(out.Bytes), "harness kernel") {
		t.Fatalf("dropped section leaked into bytes:\n%s", out.Bytes)
	}
}

// TestLLMOptimizerFailClosedTable drives every fail-closed parse/call
// failure class to a TYPED error — the optimizer never falls back.
func TestLLMOptimizerFailClosedTable(t *testing.T) {
	callErr := errors.New("connection refused")
	cases := []struct {
		name       string
		content    string
		callErr    error
		wantSent   error
		wantSubstr string
	}{
		{"malformed json", `{"persona": "unclosed`, nil, ErrLLMOptimizerUnparseable, ""},
		{"non-object array", `["persona"]`, nil, ErrLLMOptimizerUnparseable, ""},
		{"non-object string", `"persona"`, nil, ErrLLMOptimizerUnparseable, ""},
		{"non-string value", `{"persona": 42}`, nil, ErrLLMOptimizerUnparseable, ""},
		{"ghost key", `{"persona":"x","ghost.section":"y"}`, nil, ErrLLMOptimizerGhostSection, "ghost.section"},
		{"empty content", "", nil, ErrLLMOptimizerEmptyContent, ""},
		{"whitespace content", "   \n\t", nil, ErrLLMOptimizerEmptyContent, ""},
		{"call error", "", callErr, ErrLLMOptimizerCallFailed, "connection refused"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := llmInput(t)
			o, _ := newTestLLMOptimizer(t, "m", tc.content, tc.callErr)
			_, err := o.Optimize(context.Background(), in)
			if err == nil {
				t.Fatal("Optimize succeeded, want typed fail-closed error")
			}
			if !errors.Is(err, tc.wantSent) {
				t.Fatalf("err = %v, want sentinel %v", err, tc.wantSent)
			}
			if tc.wantSubstr != "" && !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("err = %v, want it to name %q", err, tc.wantSubstr)
			}
		})
	}
}

// TestLLMOptimizerStripsFences: both ```json and bare ``` fences around
// the object parse identically to the raw object.
func TestLLMOptimizerStripsFences(t *testing.T) {
	for _, fence := range []string{"```json", "```"} {
		content := fence + "\n" + `{"persona":"Operate predictably.","identity":"You are the kernel.","tools.guidance":"Use read and write."}` + "\n```"
		in := llmInput(t)
		o, _ := newTestLLMOptimizer(t, "m", content, nil)
		out, err := o.Optimize(context.Background(), in)
		if err != nil {
			t.Fatalf("fence %q: Optimize: %v", fence, err)
		}
		if !strings.Contains(string(out.Bytes), "You are the kernel.") {
			t.Fatalf("fence %q: bodies missing from %s", fence, out.Bytes)
		}
	}
}

// TestLLMOptimizerRequestDeterminism: the same optimize input must
// produce byte-identical request payloads across runs (deterministic
// ordering, no maps, temperature pinned to 0), and the payload must
// carry the invariants contract, the catalog, and the sections with
// provenance.
func TestLLMOptimizerRequestDeterminism(t *testing.T) {
	in := llmInput(t)
	withDelegated := in
	withDelegated.Invariants = InvariantsContract{DelegatedTools: []string{"write"}, MaxGrowthRatio: 1.25}

	o, cap := newTestLLMOptimizer(t, "m", `{"persona":"Operate predictably.","identity":"i","tools.guidance":"t"}`, nil)
	if _, err := o.Optimize(context.Background(), withDelegated); err != nil {
		t.Fatalf("Optimize #1: %v", err)
	}
	if _, err := o.Optimize(context.Background(), withDelegated); err != nil {
		t.Fatalf("Optimize #2: %v", err)
	}
	cap.mu.Lock()
	defer cap.mu.Unlock()
	if len(cap.rawBytes) != 2 {
		t.Fatalf("captured %d requests, want 2", len(cap.rawBytes))
	}
	if string(cap.rawBytes[0]) != string(cap.rawBytes[1]) {
		t.Fatalf("request payloads differ across identical inputs:\n%s\n%s", cap.rawBytes[0], cap.rawBytes[1])
	}

	req := cap.reqs[0]
	if req.Model != "m" {
		t.Fatalf("request model = %q, want m", req.Model)
	}
	if req.Temperature == nil || *req.Temperature != 0 {
		t.Fatalf("temperature = %v, want pinned 0 (determinism)", req.Temperature)
	}
	if len(req.Messages) != 2 || req.Messages[0].Role != "system" || req.Messages[1].Role != "user" {
		t.Fatalf("messages = %+v, want [system, user]", req.Messages)
	}
	sys := req.Messages[0].Content
	for _, want := range []string{"JSON object", "required", "write", "delegated", "1.25"} {
		if !strings.Contains(sys, want) {
			t.Fatalf("system instructions missing %q:\n%s", want, sys)
		}
	}
	user := req.Messages[1].Content
	for _, want := range []string{`"key":"persona"`, `"owner":"core"`, `"required":true`, `"name":"read"`, `"name":"write"`} {
		if !strings.Contains(user, want) {
			t.Fatalf("user payload missing %q:\n%s", want, user)
		}
	}
}

// TestCruxLLMOptimizerRealAdapterCompile is the CRUX seam test: a real
// openaicompat adapter over an httptest server scripted to return the
// fenced JSON section map → LLMOptimizer → Compile → every mechanical
// invariant passes → the content-hashed artifact lands on disk and
// reloads byte-stably.
func TestCruxLLMOptimizerRealAdapterCompile(t *testing.T) {
	scripted := "```json\n" +
		`{"identity":"You are the kernel.","persona":"Operate predictably.","tools.guidance":"Use read for files, write for edits."}` +
		"\n```"

	var mu sync.Mutex
	var bodies []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-llmopt", "object": "chat.completion", "model": "test-model",
			"choices": []map[string]any{{
				"index": 0, "message": map[string]any{"role": "assistant", "content": scripted},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 50, "completion_tokens": 20, "total_tokens": 70},
		})
	}))
	defer ts.Close()

	ad := openaicompat.New(openaicompat.Config{BaseURL: ts.URL, Model: "test-model", APIKey: "test-key"})
	o, err := NewLLMOptimizer("test-model", ad.Call)
	if err != nil {
		t.Fatalf("NewLLMOptimizer: %v", err)
	}

	a, cat := baseAssembler(t), baseCatalog(t)
	raw, err := a.Render(nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	art, err := Compile(context.Background(), a, nil, cat, o.Versioned(), InvariantsContract{}, dir)
	if err != nil {
		t.Fatalf("Compile through the real adapter: %v", err)
	}

	if art.OptimizerVersion != "llmopt/v1/test-model" {
		t.Fatalf("artifact optimizer_version = %q", art.OptimizerVersion)
	}
	for _, iv := range art.Invariants {
		if !iv.Passed {
			t.Fatalf("invariant %s failed: %s", iv.Name, iv.Detail)
		}
	}
	if string(art.Bytes) == string(raw) {
		t.Fatal("compiled bytes identical to raw assembly — no optimization happened")
	}
	if !strings.Contains(string(art.Bytes), "You are the kernel.") {
		t.Fatalf("compiled bytes missing the optimized identity:\n%s", art.Bytes)
	}
	if art.Tokens.OutputTokens > art.Tokens.InputTokens {
		t.Fatalf("token ratchet regressed: in=%d out=%d", art.Tokens.InputTokens, art.Tokens.OutputTokens)
	}

	// Artifact on disk, reloads byte-stably.
	loaded, err := LoadCompiled(dir, art.Hash)
	if err != nil {
		t.Fatalf("LoadCompiled: %v", err)
	}
	if string(loaded.Bytes) != string(art.Bytes) {
		t.Fatal("reloaded artifact bytes differ")
	}
	if _, err := os.Stat(filepath.Join(dir, "prompt-"+art.Hash+".json")); err != nil {
		t.Fatalf("artifact file: %v", err)
	}

	// The wire request the adapter carried: chat-completions shape with
	// the deterministic two-message optimizer payload inside.
	mu.Lock()
	wire := bodies[0]
	mu.Unlock()
	if len(bodies) != 1 {
		t.Fatalf("adapter calls = %d, want exactly 1 (no retries)", len(bodies))
	}
	var wireReq struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		Temperature *float64 `json:"temperature"`
	}
	if err := json.Unmarshal([]byte(wire), &wireReq); err != nil {
		t.Fatalf("decode wire request: %v", err)
	}
	if wireReq.Model != "test-model" || len(wireReq.Messages) != 2 ||
		wireReq.Messages[0].Role != "system" || wireReq.Messages[1].Role != "user" {
		t.Fatalf("wire request shape wrong: model=%q messages=%+v", wireReq.Model, wireReq.Messages)
	}
	if wireReq.Temperature == nil || *wireReq.Temperature != 0 {
		t.Fatalf("wire temperature = %v, want pinned 0", wireReq.Temperature)
	}
	if user := wireReq.Messages[1].Content; !strings.Contains(user, `"key":"persona"`) || !strings.Contains(user, `"name":"read"`) {
		t.Fatalf("wire user payload missing sections/catalog:\n%s", user)
	}
}

// TestLLMOptimizerFailClosedThroughCompile: a scripted invariant
// violation (required section dropped from the map) fails the COMPILE
// with the named invariant — the invariants stay the authority, the
// optimizer never repairs or falls back.
func TestLLMOptimizerFailClosedThroughCompile(t *testing.T) {
	scripted := `{"identity":"You are the kernel.","tools.guidance":"Use read for files, write for edits."}` // persona dropped
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "test-model",
			"choices": []map[string]any{{
				"index": 0, "message": map[string]any{"role": "assistant", "content": scripted},
				"finish_reason": "stop",
			}},
		})
	}))
	defer ts.Close()

	ad := openaicompat.New(openaicompat.Config{BaseURL: ts.URL, Model: "test-model", APIKey: "test-key"})
	o, _ := NewLLMOptimizer("test-model", ad.Call)
	a, cat := baseAssembler(t), baseCatalog(t)
	_, err := Compile(context.Background(), a, nil, cat, o.Versioned(), InvariantsContract{}, t.TempDir())
	if err == nil {
		t.Fatal("compile succeeded with a dropped required section, want invariant failure")
	}
	var inv *InvariantError
	if !errors.As(err, &inv) {
		t.Fatalf("err = %v (%T), want *InvariantError", err, err)
	}
	joined := fmt.Sprint(inv.Violations)
	if !strings.Contains(joined, "required-sections-preserved-or-rationaled") || !strings.Contains(joined, "persona") {
		t.Fatalf("violations = %v, want the required-section invariant naming persona", inv.Violations)
	}
}
