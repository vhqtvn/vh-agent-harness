// server_test.go — the scripted mock server over httptest: FIFO
// consumption, both dialects' wire shapes, fault/empty classes,
// exhausted-script 500, counters/reset, journal recording + redaction,
// auth-presence handling, and deterministic response ids.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// startMock launches the mock handler over httptest with the given
// script source (written to a temp file so the exhausted-error names a
// real path) and a Bearer key for OpenAI-dialect requests.
func startMock(t *testing.T, script string) (*httptest.Server, *mockServer) {
	t.Helper()
	p := writeScript(t, script)
	steps, err := LoadScript(p)
	if err != nil {
		t.Fatalf("LoadScript: %v", err)
	}
	ms := newMockServer(p, steps)
	srv := httptest.NewServer(ms)
	t.Cleanup(srv.Close)
	return srv, ms
}

// postChat sends an OpenAI-dialect chat-completions request (with the
// Bearer auth header the dialect requires).
func postChat(t *testing.T, srv *httptest.Server, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sk-mock-test-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/chat/completions: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// postMsg sends an Anthropic-dialect messages request.
func postMsg(t *testing.T, srv *httptest.Server, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", "sk-mock-test-key")
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/messages: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func decodeResponseBody(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("body is not JSON: %v\n%s", err, raw)
	}
	return m
}

func TestHealthz(t *testing.T) {
	srv, _ := startMock(t, `[{"text":"x"}]`)
	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("healthz status = %d", resp.StatusCode)
	}
}

// TestOpenAITextResponse pins the OpenAI wire shape for a text step:
// choices[0].message.content, finish_reason stop, deterministic id.
func TestOpenAITextResponse(t *testing.T) {
	srv, _ := startMock(t, `[{"text":"hello there"}]`)
	resp := postChat(t, srv, `{"model":"mock-model","messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	m := decodeResponseBody(t, resp)
	if m["id"] != "mock-chatcmpl-1" {
		t.Fatalf("id = %v, want deterministic mock-chatcmpl-1", m["id"])
	}
	choices := m["choices"].([]any)
	ch := choices[0].(map[string]any)
	msg := ch["message"].(map[string]any)
	if msg["role"] != "assistant" || msg["content"] != "hello there" {
		t.Fatalf("message = %v", msg)
	}
	if ch["finish_reason"] != "stop" {
		t.Fatalf("finish_reason = %v", ch["finish_reason"])
	}
}

// TestOpenAIToolCallsResponse pins the OpenAI tool-call wire shape:
// string-encoded arguments (the OpenAI asymmetry), finish_reason
// tool_calls, assistant content is the EMPTY STRING (never null).
func TestOpenAIToolCallsResponse(t *testing.T) {
	srv, _ := startMock(t, `[{"tool_calls":[{"id":"call-1","name":"echo","args":{"text":"hi"}}]}]`)
	resp := postChat(t, srv, `{"model":"mock-model","messages":[{"role":"user","content":"go"}]}`)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	m := decodeResponseBody(t, resp)
	choices := m["choices"].([]any)
	ch := choices[0].(map[string]any)
	if ch["finish_reason"] != "tool_calls" {
		t.Fatalf("finish_reason = %v", ch["finish_reason"])
	}
	msg := ch["message"].(map[string]any)
	if content, ok := msg["content"]; !ok || content != "" {
		t.Fatalf("assistant content must be present and \"\", got %v (present=%v)", content, ok)
	}
	calls := msg["tool_calls"].([]any)
	c0 := calls[0].(map[string]any)
	if c0["id"] != "call-1" || c0["type"] != "function" {
		t.Fatalf("tool call = %v", c0)
	}
	fn := c0["function"].(map[string]any)
	if fn["name"] != "echo" {
		t.Fatalf("function.name = %v", fn["name"])
	}
	// OpenAI wire shape: arguments is a JSON STRING.
	args, isString := fn["arguments"].(string)
	if !isString {
		t.Fatalf("arguments must be a string on the OpenAI wire, got %T", fn["arguments"])
	}
	if args != `{"text":"hi"}` {
		t.Fatalf("arguments = %s, want compact {\"text\":\"hi\"}", args)
	}
}

// TestAnthropicTextResponse pins the Anthropic wire shape: content
// blocks with type text, stop_reason end_turn, deterministic mock-msg-N.
func TestAnthropicTextResponse(t *testing.T) {
	srv, _ := startMock(t, `[{"text":"anthropic says hi"}]`)
	resp := postMsg(t, srv, `{"model":"mock-model","max_tokens":64,"messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	m := decodeResponseBody(t, resp)
	if m["id"] != "mock-msg-1" {
		t.Fatalf("id = %v, want mock-msg-1", m["id"])
	}
	if m["role"] != "assistant" {
		t.Fatalf("role = %v", m["role"])
	}
	blocks := m["content"].([]any)
	b0 := blocks[0].(map[string]any)
	if b0["type"] != "text" || b0["text"] != "anthropic says hi" {
		t.Fatalf("block = %v", b0)
	}
	if m["stop_reason"] != "end_turn" {
		t.Fatalf("stop_reason = %v", m["stop_reason"])
	}
	usage := m["usage"].(map[string]any)
	if _, ok := usage["input_tokens"]; !ok {
		t.Fatalf("usage must carry input_tokens: %v", usage)
	}
}

// TestAnthropicToolUseResponse pins the Anthropic tool-call wire shape:
// tool_use blocks with the input OBJECT (the Anthropic asymmetry).
func TestAnthropicToolUseResponse(t *testing.T) {
	srv, _ := startMock(t, `[{"tool_calls":[{"id":"toolu-1","name":"echo","args":{"text":"hi"}}]}]`)
	resp := postMsg(t, srv, `{"model":"mock-model","max_tokens":64,"messages":[{"role":"user","content":"go"}]}`)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	m := decodeResponseBody(t, resp)
	blocks := m["content"].([]any)
	b0 := blocks[0].(map[string]any)
	if b0["type"] != "tool_use" || b0["id"] != "toolu-1" || b0["name"] != "echo" {
		t.Fatalf("tool_use block = %v", b0)
	}
	input, isObj := b0["input"].(map[string]any)
	if !isObj {
		t.Fatalf("input must be an OBJECT on the Anthropic wire, got %T", b0["input"])
	}
	if input["text"] != "hi" {
		t.Fatalf("input = %v", input)
	}
	if m["stop_reason"] != "tool_use" {
		t.Fatalf("stop_reason = %v", m["stop_reason"])
	}
}

// TestFaultClasses: status + body verbatim + Retry-After seconds form.
func TestFaultClasses(t *testing.T) {
	srv, _ := startMock(t, `[
		{"fault":{"status":500,"body":"{\"error\":\"boom\"}"}},
		{"fault":{"status":429,"body":"{\"error\":\"slow\"}","retry_after_ms":2000}}
	]`)
	// Plain 500.
	resp := postChat(t, srv, `{}`)
	if resp.StatusCode != 500 {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
	raw, _ := io.ReadAll(resp.Body)
	if strings.TrimSpace(string(raw)) != `{"error":"boom"}` {
		t.Fatalf("fault body must be verbatim, got %s", raw)
	}
	if resp.Header.Get("Retry-After") != "" {
		t.Fatalf("Retry-After must be absent when retry_after_ms unset, got %q", resp.Header.Get("Retry-After"))
	}
	// 429 with retry_after_ms 2000 → Retry-After: 2 (seconds form).
	resp2 := postChat(t, srv, `{}`)
	if resp2.StatusCode != 429 {
		t.Fatalf("status = %d, want 429", resp2.StatusCode)
	}
	if got := resp2.Header.Get("Retry-After"); got != "2" {
		t.Fatalf("Retry-After = %q, want \"2\" (2000ms in seconds form)", got)
	}
	// Out-of-range status in the script must have failed at LOAD time —
	// the third step above is unreachable; prove the loader caught it.
	p := writeScript(t, `[{"fault":{"status":1500}}]`)
	if _, err := LoadScript(p); err == nil {
		t.Fatal("status 1500 must fail closed at load")
	}
}

// TestEmptyClasses: 200 with choices present but no content (OpenAI) /
// empty content array (Anthropic).
func TestEmptyClasses(t *testing.T) {
	srv, _ := startMock(t, `[{"empty":true},{"empty":true}]`)
	resp := postChat(t, srv, `{}`)
	if resp.StatusCode != 200 {
		t.Fatalf("openai empty status = %d", resp.StatusCode)
	}
	m := decodeResponseBody(t, resp)
	choices := m["choices"].([]any)
	if len(choices) != 1 {
		t.Fatalf("choices must be PRESENT for the empty class, got %v", m["choices"])
	}
	ch := choices[0].(map[string]any)
	msg := ch["message"].(map[string]any)
	if _, has := msg["content"]; has {
		t.Fatalf("empty class must carry NO content, got %v", msg["content"])
	}
	resp2 := postMsg(t, srv, `{}`)
	if resp2.StatusCode != 200 {
		t.Fatalf("anthropic empty status = %d", resp2.StatusCode)
	}
	m2 := decodeResponseBody(t, resp2)
	if blocks, ok := m2["content"].([]any); !ok || len(blocks) != 0 {
		t.Fatalf("anthropic empty class must be content:[], got %v", m2["content"])
	}
}

// TestFIFOGlobalConsumption: steps are consumed globally in arrival
// order across BOTH dialect endpoints.
func TestFIFOGlobalConsumption(t *testing.T) {
	srv, _ := startMock(t, `[{"text":"one"},{"text":"two"},{"text":"three"}]`)
	resp := postMsg(t, srv, `{}`) // anthropic consumes step 1
	m := decodeResponseBody(t, resp)
	blocks := m["content"].([]any)
	if blocks[0].(map[string]any)["text"] != "one" {
		t.Fatalf("FIFO violated: anthropic got %v", blocks)
	}
	resp2 := postChat(t, srv, `{}`) // openai consumes step 2
	m2 := decodeResponseBody(t, resp2)
	if m2["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)["content"] != "two" {
		t.Fatalf("FIFO violated: openai got %v", m2)
	}
	// Deterministic ids increment PER DIALECT: first anthropic id was
	// mock-msg-1; the second anthropic response must be mock-msg-2.
	resp3 := postMsg(t, srv, `{}`)
	m3 := decodeResponseBody(t, resp3)
	if m3["id"] != "mock-msg-2" {
		t.Fatalf("id = %v, want mock-msg-2 (per-dialect monotonic)", m3["id"])
	}
}

// TestExhaustedScriptFailsLoud: a request past the last step gets a 500
// naming the scenario file and its step count — never a silent loop.
func TestExhaustedScriptFailsLoud(t *testing.T) {
	p := writeScript(t, `[{"text":"only"}]`)
	steps, err := LoadScript(p)
	if err != nil {
		t.Fatalf("LoadScript: %v", err)
	}
	ms := newMockServer(p, steps)
	srv := httptest.NewServer(ms)
	defer srv.Close()
	doPost := func() *http.Response {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/chat/completions", strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer k")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		return resp
	}
	// Request 1 consumes the only step and succeeds.
	first := doPost()
	first.Body.Close()
	if first.StatusCode != 200 {
		t.Fatalf("first request status = %d, want 200", first.StatusCode)
	}
	// Request 2 is past the end: 500, naming the scenario file and the
	// step count.
	resp := doPost()
	defer resp.Body.Close()
	if resp.StatusCode != 500 {
		t.Fatalf("exhausted status = %d, want 500", resp.StatusCode)
	}
	raw, _ := io.ReadAll(resp.Body)
	msg := string(raw)
	if !strings.Contains(msg, filepath.Base(p)) {
		t.Fatalf("exhausted error must name the scenario file: %s", msg)
	}
	if !strings.Contains(msg, "1 step") && !strings.Contains(msg, "step count: 1") {
		t.Fatalf("exhausted error must name the step count: %s", msg)
	}
}

// TestCountersAndReset: /count reports per-LLM-path POST counts; /reset
// clears counters but NOT the script cursor and NOT the journal.
func TestCountersAndReset(t *testing.T) {
	srv, _ := startMock(t, `[{"text":"a"},{"text":"b"}]`)
	postChat(t, srv, `{}`)
	postChat(t, srv, `{}`)
	postMsg(t, srv, `{}`)
	count := func() map[string]int {
		resp, err := http.Get(srv.URL + "/count")
		if err != nil {
			t.Fatalf("GET /count: %v", err)
		}
		defer resp.Body.Close()
		var m map[string]int
		if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
			t.Fatalf("count decode: %v", err)
		}
		return m
	}
	c := count()
	if c["/v1/chat/completions"] != 2 || c["/v1/messages"] != 1 {
		t.Fatalf("counters = %v", c)
	}
	// /count itself must not be counted.
	if _, polluted := count()["/count"]; polluted {
		t.Fatal("control endpoints must not appear in /count")
	}
	// Reset via POST (and GET must work too).
	if resp, err := http.Post(srv.URL+"/reset", "", nil); err != nil || resp.StatusCode != 200 {
		t.Fatalf("POST /reset: %v", err)
	} else {
		resp.Body.Close()
	}
	c2 := count()
	if len(c2) != 0 {
		t.Fatalf("counters must be cleared, got %v", c2)
	}
	// The journal is NOT reset either: 3 entries remain.
	j := getJournal(t, srv, "")
	if len(j) != 3 {
		t.Fatalf("journal must survive /reset, got %d entries", len(j))
	}
	// The cursor is NOT reset: the next response is step 4 territory —
	// the script had 2 steps, so the next request must exhaust.
	resp := postChat(t, srv, `{}`)
	if resp.StatusCode != 500 {
		t.Fatalf("reset must NOT rewind the script cursor; status = %d, want 500 exhausted", resp.StatusCode)
	}
}

// getJournal fetches /journal[?since=N] and returns the entries array.
func getJournal(t *testing.T, srv *httptest.Server, query string) []map[string]any {
	t.Helper()
	resp, err := http.Get(srv.URL + "/journal" + query)
	if err != nil {
		t.Fatalf("GET /journal: %v", err)
	}
	defer resp.Body.Close()
	var entries []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		t.Fatalf("journal decode: %v", err)
	}
	return entries
}

// TestJournalRecordsAndRedacts: the journal records method, path, auth
// PRESENCE, and the full request body JSON for every LLM request — and
// the key VALUE never appears anywhere in the journal bytes.
func TestJournalRecordsAndRedacts(t *testing.T) {
	srv, _ := startMock(t, `[{"text":"a"},{"tool_calls":[{"id":"c1","name":"echo","args":{"text":"hi"}}]}]`)
	// OpenAI request WITH auth header.
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"mock-model","tools":[{"type":"function","function":{"name":"echo"}}],"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer sk-super-secret-value-123")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	resp.Body.Close()
	// Anthropic request WITHOUT auth (journaled, then 401 — see auth test).
	req2, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/messages", strings.NewReader(`{"model":"m","messages":[]}`))
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("post2: %v", err)
	}
	resp2.Body.Close()

	j := getJournal(t, srv, "")
	if len(j) != 2 {
		t.Fatalf("journal entries = %d, want 2", len(j))
	}
	e0 := j[0]
	if e0["method"] != "POST" || e0["path"] != "/v1/chat/completions" {
		t.Fatalf("entry 0 = %v", e0)
	}
	if e0["auth"] != true {
		t.Fatalf("auth PRESENCE must be recorded true, got %v", e0["auth"])
	}
	body := e0["body"].(map[string]any)
	if body["model"] != "mock-model" {
		t.Fatalf("journal body must carry the full request JSON, got %v", body)
	}
	tools := body["tools"].([]any)
	if tools[0].(map[string]any)["function"].(map[string]any)["name"] != "echo" {
		t.Fatalf("journal body must carry the advertised tools array, got %v", tools)
	}
	e1 := j[1]
	if e1["auth"] != false {
		t.Fatalf("missing auth must record false, got %v", e1["auth"])
	}
	// ?since=1 returns only entries with seq > 1.
	j2 := getJournal(t, srv, "?since=1")
	if len(j2) != 1 {
		t.Fatalf("?since=1 entries = %d, want 1", len(j2))
	}
	// THE redaction crux: the key VALUE never appears in the journal.
	raw, err := json.Marshal(j)
	if err != nil {
		t.Fatalf("marshal journal: %v", err)
	}
	if strings.Contains(string(raw), "sk-super-secret-value-123") {
		t.Fatalf("API-key VALUE leaked into the journal: %s", raw)
	}
}

// TestAuthPresenceEnforced: LLM endpoints require the dialect's auth
// header (Authorization for OpenAI, x-api-key for Anthropic); absence
// is a 401 BEFORE any script step is consumed.
func TestAuthPresenceEnforced(t *testing.T) {
	srv, _ := startMock(t, `[{"text":"a"}]`)
	// OpenAI without Authorization.
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("openai no-auth status = %d, want 401", resp.StatusCode)
	}
	// Anthropic without x-api-key.
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/messages", strings.NewReader(`{}`))
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post2: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != 401 {
		t.Fatalf("anthropic no-auth status = %d, want 401", resp2.StatusCode)
	}
	// The 401s consumed NO steps: the next authorized request still
	// gets step 1.
	resp3 := postChat(t, srv, `{}`)
	if resp3.StatusCode != 200 {
		t.Fatalf("authorized request after 401s: status = %d (steps must not be consumed by auth failures)", resp3.StatusCode)
	}
}

// TestUnknownPath404.
func TestUnknownPath404(t *testing.T) {
	srv, _ := startMock(t, `[{"text":"a"}]`)
	resp, err := http.Post(srv.URL+"/v1/embeddings", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("unknown path status = %d, want 404", resp.StatusCode)
	}
}

// TestDiskJournalAppend: --journal mirrors every entry to an append-only
// JSONL file on disk, with the same redaction discipline.
func TestDiskJournalAppend(t *testing.T) {
	dir := t.TempDir()
	p := writeScript(t, `[{"text":"a"}]`)
	steps, err := LoadScript(p)
	if err != nil {
		t.Fatalf("LoadScript: %v", err)
	}
	ms := newMockServer(p, steps)
	jpath := filepath.Join(dir, "journal.jsonl")
	f, err := os.OpenFile(jpath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("open journal: %v", err)
	}
	ms.setDiskJournal(f)
	srv := httptest.NewServer(ms)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/chat/completions", strings.NewReader(`{"model":"m","messages":[]}`))
	req.Header.Set("Authorization", "Bearer sk-disk-secret-999")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	resp.Body.Close()
	if err := f.Sync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	raw, err := os.ReadFile(jpath)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 1 {
		t.Fatalf("disk journal lines = %d, want 1:\n%s", len(lines), raw)
	}
	if strings.Contains(string(raw), "sk-disk-secret-999") {
		t.Fatalf("key value leaked to the disk journal: %s", raw)
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatalf("journal line not JSON: %v", err)
	}
	if entry["path"] != "/v1/chat/completions" || entry["auth"] != true {
		t.Fatalf("disk entry = %v", entry)
	}
}

// TestConcurrentRequestsSerializeSteps: parallel requests consume
// distinct steps (the global FIFO under the mutex — no step is served
// twice, none skipped).
func TestConcurrentRequestsSerializeSteps(t *testing.T) {
	scripts := make([]string, 8)
	for i := range scripts {
		scripts[i] = fmt.Sprintf(`{"text":"msg-%d"}`, i)
	}
	srv, _ := startMock(t, "["+strings.Join(scripts, ",")+"]")
	var wg sync.WaitGroup
	var mu sync.Mutex
	var bodies []string
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/chat/completions", strings.NewReader(`{}`))
			req.Header.Set("Authorization", "Bearer k")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Errorf("post: %v", err)
				return
			}
			raw, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			mu.Lock()
			bodies = append(bodies, string(raw))
			mu.Unlock()
		}()
	}
	wg.Wait()
	seen := map[string]bool{}
	for _, b := range bodies {
		var m map[string]any
		if err := json.Unmarshal([]byte(b), &m); err != nil {
			t.Fatalf("concurrent response not JSON: %v (%s)", err, b)
		}
		choices := m["choices"].([]any)
		content := choices[0].(map[string]any)["message"].(map[string]any)["content"].(string)
		seen[content] = true
	}
	if len(seen) != 8 {
		t.Fatalf("distinct steps served = %d, want 8 (FIFO must not duplicate or skip under concurrency): %v", len(seen), seen)
	}
}
