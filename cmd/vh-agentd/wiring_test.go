// wiring_test.go — slice B1 daemon-level wiring tests: the compiled-
// sysprompt serving rule (raw fallback → compiled after offline
// compile), the --compile-prompt CLI surface, the RETRY-LADDER CRUX
// (flaky fail-fail-succeed over the daemon's real turn path with ONE
// turn bracket and byte-identical replay), the exhaustion path
// (turn/end{kind:error} + ExhaustedError surfaced as the protocol turn
// error), the anthropic --cache-breakpoints wire round-trip, and the
// scheduler's tracker seams.
package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/prompt"
	"github.com/vhqtvn/vh-agent-harness/internal/protocol"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// --- prompt serving -----------------------------------------------------------

// TestPromptServingRawFallbackThenCompiled pins the serving rule over
// the daemon's own inputs: with no artifact the daemon serves RAW
// assembly (explicit fallback reason); after --compile-prompt's offline
// step it serves the COMPILED artifact bytes, and buildServer threads
// them into TurnOptions.System.
func TestPromptServingRawFallbackThenCompiled(t *testing.T) {
	cfg := testConfig(t, "openai", "http://127.0.0.1:1")
	// The serving MECHANICS via the offline reference fake: this test
	// has no LLM stub. (The llm-optimizer family — compile AND serve —
	// is covered end-to-end in prompt_optimizer_test.go.)
	cfg.Optimizer = optimizerDedup
	specs := toolSpecsForPrompt(cfg)

	// Fresh dir: raw assembly fallback, never silent.
	got, served, err := resolveSystemPrompt(cfg, specs)
	if err != nil {
		t.Fatalf("resolveSystemPrompt: %v", err)
	}
	if served.Source != prompt.ServeSourceRawAssembly || served.Reason != prompt.ServeReasonNotFound {
		t.Fatalf("fresh-dir serve = %+v, want raw-assembly/artifact-not-found", served)
	}
	asm, vars, _, err := buildPromptInputs(cfg, specs)
	if err != nil {
		t.Fatalf("buildPromptInputs: %v", err)
	}
	raw, err := asm.Render(vars)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if got != string(raw) {
		t.Fatalf("raw fallback bytes differ from assembly:\ngot:  %q\nwant: %q", got, raw)
	}

	// Offline compile, then re-resolve: compiled artifact bytes. (With
	// the Dedup reference fake and no duplicate sections the optimized
	// bytes may legitimately equal the raw bytes — the assertion that
	// matters is that the SERVED bytes are the ARTIFACT's bytes.)
	if err := compilePromptOffline(context.Background(), cfg, "", specs, io.Discard); err != nil {
		t.Fatalf("compilePromptOffline: %v", err)
	}
	compiled, served2, err := resolveSystemPrompt(cfg, specs)
	if err != nil {
		t.Fatalf("resolveSystemPrompt after compile: %v", err)
	}
	if served2.Source != prompt.ServeSourceCompiled || served2.Reason != "" {
		t.Fatalf("post-compile serve = %+v, want compiled/no-reason", served2)
	}
	asm2, vars2, catalog, err := buildPromptInputs(cfg, specs)
	if err != nil {
		t.Fatalf("buildPromptInputs: %v", err)
	}
	hash, err := prompt.InputHash(asm2, vars2, catalog, servingOptimizerVersion(cfg), promptContract())
	if err != nil {
		t.Fatalf("InputHash: %v", err)
	}
	art, err := prompt.LoadCompiled(promptArtifactDir(cfg), hash)
	if err != nil {
		t.Fatalf("LoadCompiled: %v", err)
	}
	if compiled != string(art.Bytes) {
		t.Fatalf("served bytes != artifact bytes (%d vs %d)", len(compiled), len(art.Bytes))
	}
	// Determinism: a second compile is a cache hit; the same bytes
	// re-serve.
	if err := compilePromptOffline(context.Background(), cfg, "", specs, io.Discard); err != nil {
		t.Fatalf("second compilePromptOffline: %v", err)
	}
	again, _, err := resolveSystemPrompt(cfg, specs)
	if err != nil || again != compiled {
		t.Fatalf("second serve differs (err %v)", err)
	}

	// buildServer threads the served prompt into the turn options.
	svc, cli := net.Pipe()
	defer cli.Close()
	_, eng, _, served3 := buildServer(cfg, "k", svc)
	if eng.TurnOptions().System != compiled || served3.Source != prompt.ServeSourceCompiled {
		t.Fatalf("buildServer system prompt = served(%+v) match=%v", served3, eng.TurnOptions().System == compiled)
	}
}

// TestCompilePromptFlagRunsOffline drives the --compile-prompt CLI
// surface end-to-end through run() in its OFFLINE mode (--optimizer
// dedup): NO API key environment value is set, the artifact lands under
// <session-dir>/compiled-prompts/, and the report goes to stderr. (The
// default llm optimizer without a key is the exit-2 fail-closed case in
// prompt_optimizer_test.go.)
func TestCompilePromptFlagRunsOffline(t *testing.T) {
	dir := t.TempDir()
	var out, errBuf safeBuffer
	code := run([]string{
		"--adapter", "openai", "--model", "m", "--base-url", "http://127.0.0.1:1",
		"--api-key-env", "VH_AGENTD_NO_SUCH_KEY", "--session-dir", dir,
		"--compile-prompt", "--optimizer", "dedup",
	}, func(string) string { return "" }, nil, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, errBuf.String())
	}
	if out.String() != "" {
		t.Fatalf("stdout = %q, want empty (stdout is protocol)", out.String())
	}
	if !strings.Contains(errBuf.String(), "compiled system prompt") {
		t.Fatalf("stderr missing compile report: %s", errBuf.String())
	}
	entries, err := os.ReadDir(filepath.Join(dir, "compiled-prompts"))
	if err != nil || len(entries) != 1 || !strings.HasSuffix(entries[0].Name(), ".json") {
		t.Fatalf("artifact dir entries = %+v (err %v), want exactly one prompt-<hash>.json", entries, err)
	}
}

// --- retry ladder (crux) ------------------------------------------------------

// flakyLLM scripts HTTP outcomes per call: two 500s, then success. It
// records every request body for the system-prompt assertion.
func flakyLLM(t *testing.T, outcomes []int) (*httptest.Server, *[]string) {
	t.Helper()
	var mu sync.Mutex
	calls := 0
	var bodies []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		calls++
		n := calls
		bodies = append(bodies, string(body))
		mu.Unlock()
		if n <= len(outcomes) && outcomes[n-1] != 200 {
			w.WriteHeader(outcomes[n-1])
			_, _ = w.Write([]byte(`{"error":"scripted failure"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-ok", "object": "chat.completion", "model": "fake-model",
			"choices": []map[string]any{{
				"index": 0, "message": map[string]any{"role": "assistant", "content": "recovered"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 1, "total_tokens": 4},
		})
	}))
	return ts, &bodies
}

// serveSession boots the daemon composition on a pipe and drives
// initialize + session/create, returning the client and the log path.
func serveSession(t *testing.T, cfg *Config) (*protocol.Client, string, func()) {
	t.Helper()
	logPath := filepath.Join(cfg.SessionDir, "sess-wiring.jsonl")
	svc, cli := net.Pipe()
	srv, _, _, _ := buildServer(cfg, "test-key", svc)
	served := make(chan error, 1)
	go func() { served <- srv.Serve(nil) }()
	client := protocol.NewClient(cli)
	if err := client.Call("initialize", map[string]any{"protocolVersion": 1}, nil); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if err := client.Call("session/create", map[string]any{"path": logPath, "sessionId": "sess-wiring"}, nil); err != nil {
		t.Fatalf("session/create: %v", err)
	}
	return client, logPath, func() {
		_ = client.Close()
		select {
		case <-served:
		case <-time.After(2 * time.Second):
			t.Errorf("server did not exit")
		}
	}
}

// TestRetryLadderCruxFlakyThenSuccess is the B1 CRUX at the daemon
// level: the daemon's turn path (retry ladder armed in buildServer)
// against a fail-fail-succeed mock produces llm/retry BEFORE each wait
// and llm/retry-started after it, a fresh llm/request per attempt, a
// successful response — all inside exactly ONE turn bracket — and the
// persisted log replays byte-identically. Real backoff (500ms+1000ms):
// this test takes ~1.5s.
func TestRetryLadderCruxFlakyThenSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("real backoff waits (500ms + 1000ms)")
	}
	llm, bodies := flakyLLM(t, []int{500, 500})
	defer llm.Close()
	cfg := testConfig(t, "openai", llm.URL)
	client, logPath, stop := serveSession(t, cfg)
	defer stop()

	var turn struct {
		Content string `json:"content"`
	}
	if err := client.Call("session/prompt", map[string]any{"text": "flaky turn"}, &turn); err != nil {
		t.Fatalf("session/prompt: %v", err)
	}
	if turn.Content != "recovered" {
		t.Fatalf("content = %q, want recovered", turn.Content)
	}
	if len(*bodies) != 3 {
		t.Fatalf("adapter calls = %d, want 3", len(*bodies))
	}
	// The served system prompt rode every attempt as the leading
	// system-role message.
	if !strings.Contains((*bodies)[0], `"role":"system"`) || !strings.Contains((*bodies)[2], `"role":"system"`) {
		t.Fatalf("system prompt missing from request bodies: %s", (*bodies)[0])
	}

	events := replayLog(t, logPath)
	count := func(typ string) int {
		n := 0
		for _, ev := range events {
			if ev.Type == typ {
				n++
			}
		}
		return n
	}
	if count(session.TypeTurnBegin) != 1 || count(session.TypeTurnEnd) != 1 {
		t.Fatalf("turn brackets = %d/%d, want exactly 1/1", count(session.TypeTurnBegin), count(session.TypeTurnEnd))
	}
	if count(session.TypeLLMRetry) != 2 || count(session.TypeLLMRetryStarted) != 2 || count(session.TypeLLMRequest) != 3 {
		t.Fatalf("ladder counts: retry=%d started=%d request=%d, want 2/2/3",
			count(session.TypeLLMRetry), count(session.TypeLLMRetryStarted), count(session.TypeLLMRequest))
	}
	// The llm/retry record lands with its numbered attempt + policy
	// snapshot BEFORE the wait (retry < retry-started ordering in seq).
	// Class is "http5xx": the openaicompat adapter now returns TYPED
	// errors for non-2xx (adapters.HTTPStatusError), and Classify passes
	// the typed class through — a scripted 500 is http5xx, retryable, so
	// the ladder fires exactly as before the typing (the slice B1.5 fix;
	// 4xx now surfaces non-retryable, which these 5xx fixtures never hit).
	var firstRetry loopRetryShape
	for _, ev := range events {
		if ev.Type == session.TypeLLMRetry {
			if err := json.Unmarshal(ev.Payload, &firstRetry); err != nil {
				t.Fatalf("llm/retry payload: %v", err)
			}
			break
		}
	}
	if firstRetry.Attempt != 1 || firstRetry.ErrorClass != "http5xx" || firstRetry.Policy.MaxRetries != 2 {
		t.Fatalf("llm/retry #1 = %+v", firstRetry)
	}

	// Replay determinism: the surface derived from the persisted log is
	// exactly the expected message list.
	msgs, err := session.DeriveMessages(events)
	if err != nil {
		t.Fatalf("DeriveMessages: %v", err)
	}
	want, _ := json.Marshal([]session.Message{
		{Role: "user", Content: "flaky turn"},
		{Role: "assistant", Content: "recovered"},
	})
	got, _ := json.Marshal(msgs)
	if string(got) != string(want) {
		t.Fatalf("replayed surface = %s, want %s", got, want)
	}
}

// loopRetryShape mirrors loop.RetryPayload for log assertions without
// importing loop into this test file.
type loopRetryShape struct {
	Attempt    int    `json:"attempt"`
	ErrorClass string `json:"errorClass"`
	Policy     struct {
		MaxRetries int `json:"maxRetries"`
	} `json:"policy"`
}

// TestRetryLadderExhaustionSurfacesTurnError drives the all-failures
// mock: the budget spends, the turn closes turn/end{kind:error}, and
// the *loop.ExhaustedError text surfaces over the wire as the
// session/prompt error — with the log replaying stably (surface =
// just the user prompt). Real backoff: ~1.5s.
func TestRetryLadderExhaustionSurfacesTurnError(t *testing.T) {
	if testing.Short() {
		t.Skip("real backoff waits (500ms + 1000ms)")
	}
	llm, _ := flakyLLM(t, []int{500, 500, 500, 500})
	defer llm.Close()
	cfg := testConfig(t, "openai", llm.URL)
	client, logPath, stop := serveSession(t, cfg)
	defer stop()

	err := client.Call("session/prompt", map[string]any{"text": "doomed turn"}, nil)
	if err == nil {
		t.Fatal("session/prompt succeeded against an all-failures mock")
	}
	if !strings.Contains(err.Error(), "retries exhausted") {
		t.Fatalf("wire error = %v, want the ExhaustedError text", err)
	}

	events := replayLog(t, logPath)
	last := events[len(events)-1]
	if last.Type != session.TypeTurnEnd {
		t.Fatalf("last event = %s, want turn/end", last.Type)
	}
	var te session.TurnEndPayload
	if err := json.Unmarshal(last.Payload, &te); err != nil {
		t.Fatalf("turn/end payload: %v", err)
	}
	if te.Kind != "error" || !strings.Contains(te.Reason, "HTTP 500") {
		t.Fatalf("turn/end = %+v, want kind=error carrying the failure text", te)
	}
	count := func(typ string) int {
		n := 0
		for _, ev := range events {
			if ev.Type == typ {
				n++
			}
		}
		return n
	}
	if count(session.TypeTurnBegin) != 1 || count(session.TypeTurnEnd) != 1 || count(session.TypeLLMRetry) != 2 {
		t.Fatalf("exhaustion brackets: begin=%d end=%d retry=%d, want 1/1/2",
			count(session.TypeTurnBegin), count(session.TypeTurnEnd), count(session.TypeLLMRetry))
	}
	msgs, err := session.DeriveMessages(events)
	if err != nil {
		t.Fatalf("DeriveMessages: %v", err)
	}
	want, _ := json.Marshal([]session.Message{{Role: "user", Content: "doomed turn"}})
	got, _ := json.Marshal(msgs)
	if string(got) != string(want) {
		t.Fatalf("replayed surface = %s, want %s", got, want)
	}
}

// replayLog reads + replays the durable session log (fail-closed).
func replayLog(t *testing.T, path string) []session.Event {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read session log: %v", err)
	}
	events, err := session.Replay(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	return events
}

// --- anthropic cache flag round-trip ------------------------------------------

// anthropicFakeLLM speaks just enough of the Messages API for one turn
// and captures the request body (mutex-guarded: the handler goroutine
// writes, the test reads after the turn response).
func anthropicFakeLLM(t *testing.T) (*httptest.Server, *capturedBody) {
	t.Helper()
	cell := &capturedBody{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		cell.set(string(b))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_1", "role": "assistant", "model": "fake-claude",
			"content":     []map[string]any{{"type": "text", "text": "anthropic ok"}},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 5, "output_tokens": 2},
		})
	}))
	return ts, cell
}

// capturedBody is a mutex-guarded request-body cell.
type capturedBody struct {
	mu sync.Mutex
	b  string
}

func (c *capturedBody) set(s string) {
	c.mu.Lock()
	c.b = s
	c.mu.Unlock()
}

func (c *capturedBody) get() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.b
}

// safeBuffer is a mutex-guarded output buffer for run()'s writers.
type safeBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *safeBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// TestAnthropicCacheBreakpointsWireRoundTrip proves the flag→CacheConfig
// mapping on the real wire: --cache-breakpoints 1 with --adapter
// anthropic puts a cache_control breakpoint on the FINAL tool definition
// of the outgoing Messages request, and the served system prompt rides
// the top-level system field.
func TestAnthropicCacheBreakpointsWireRoundTrip(t *testing.T) {
	llm, bodyCell := anthropicFakeLLM(t)
	defer llm.Close()
	dir := t.TempDir()
	cfg, err := validate("anthropic", "fake-claude", llm.URL, "VH_AGENTD_TEST_KEY", dir, "", 0, defaultApprovalTimeoutMs, 1, "off", 65536)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	client, _, stop := serveSession(t, cfg)
	defer stop()

	var turn struct {
		Content string `json:"content"`
	}
	if err := client.Call("session/prompt", map[string]any{"text": "cache me"}, &turn); err != nil {
		t.Fatalf("session/prompt: %v", err)
	}
	if turn.Content != "anthropic ok" {
		t.Fatalf("content = %q", turn.Content)
	}
	body := bodyCell.get()
	if !strings.Contains(body, "cache_control") {
		t.Fatalf("no cache_control breakpoint on the wire: %s", body)
	}
	if !strings.Contains(body, `"system"`) {
		t.Fatalf("no top-level system field on the wire: %s", body)
	}
}

// --- scheduler tracker seams ---------------------------------------------------

// TestSchedulerTrackerRoutesToActiveSession proves the daemon's real
// scheduler seams: dispatch through the tracker lands job/enqueued in
// the ACTIVE session's durable log, and the idle gate reads that
// manager's snapshot.
func TestSchedulerTrackerRoutesToActiveSession(t *testing.T) {
	cfg := testConfig(t, "openai", "http://127.0.0.1:1")
	engine := &protocol.FileEngine{Dir: cfg.SessionDir, Executor: daemonExecutor{}}
	tracker := &sessionTracker{Engine: engine}

	es, err := tracker.NewSession(filepath.Join(cfg.SessionDir, "sched.jsonl"), "sess-sched", io.Discard)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if tracker.current() != es {
		t.Fatal("tracker did not record the active session")
	}

	d := trackerDispatcher{t: tracker}
	receipt, err := d.Dispatch("echo", nil)
	if err != nil {
		t.Fatalf("tracker dispatch: %v", err)
	}
	if receipt.JobID != "echo-1" {
		t.Fatalf("receipt = %+v, want echo-1 routed to the real manager", receipt)
	}
	found := false
	for _, ev := range es.Log.Events() {
		if ev.Type == session.TypeJobEnqueued {
			found = true
		}
	}
	if !found {
		t.Fatal("job/enqueued missing from the active session log")
	}

	g := trackerIdleGate{t: tracker}
	deadline := time.Now().Add(2 * time.Second)
	for g.InFlight() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond) // the echo job settles quickly
	}
	if n := g.InFlight(); n != 0 {
		t.Fatalf("idle gate = %d after settlement, want 0", n)
	}

	// No-session dispatcher fails closed (cursor untouched upstream).
	empty := &sessionTracker{Engine: engine}
	if _, err := (trackerDispatcher{t: empty}).Dispatch("echo", nil); err == nil {
		t.Fatal("dispatch without an active session succeeded, want fail-closed")
	}
	if n := (trackerIdleGate{t: empty}).InFlight(); n != 0 {
		t.Fatalf("empty-tracker gate = %d, want 0", n)
	}
}

// TestSchedulerWireSurfaceDaemonWiring proves the B3 glue exactly as
// run() performs it: buildServer → buildScheduler →
// engine.Schedules = sched (before Serve) — so the daemon-composed
// server answers schedule/add|list|remove over the wire against the
// REAL daemon scheduler (state file under the session dir, tracker
// routing). The loop is not started (registration-only; dispatch lives
// in the protocol crux test).
func TestSchedulerWireSurfaceDaemonWiring(t *testing.T) {
	cfg := testConfig(t, "openai", "http://127.0.0.1:1")
	svc, cli := net.Pipe()
	srv, engine, tracker, _ := buildServer(cfg, "test-key", svc)
	sched, err := buildScheduler(cfg, tracker)
	if err != nil {
		t.Fatalf("buildScheduler: %v", err)
	}
	defer sched.Stop() // drained (no-op when never started)
	engine.Schedules = sched

	served := make(chan error, 1)
	go func() { served <- srv.Serve(nil) }()
	client := protocol.NewClient(cli)
	defer func() { _ = client.Close() }()

	if err := client.Call("initialize", map[string]any{"protocolVersion": 1}, nil); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if err := client.Call("session/create", map[string]any{
		"path": filepath.Join(cfg.SessionDir, "sess-wire-sched.jsonl"), "sessionId": "sess-wire-sched",
	}, nil); err != nil {
		t.Fatalf("session/create: %v", err)
	}

	var added struct {
		Name    string `json:"name"`
		NextRun string `json:"nextRun"`
	}
	if err := client.Call("schedule/add", map[string]any{
		"name": "tick", "every": int64(time.Minute),
	}, &added); err != nil {
		t.Fatalf("schedule/add: %v", err)
	}
	if added.Name != "tick" || added.NextRun == "" {
		t.Fatalf("add result = %+v", added)
	}

	var list struct {
		Schedules []struct {
			Name string `json:"name"`
		} `json:"schedules"`
	}
	if err := client.Call("schedule/list", nil, &list); err != nil {
		t.Fatalf("schedule/list: %v", err)
	}
	if len(list.Schedules) != 1 || list.Schedules[0].Name != "tick" {
		t.Fatalf("list = %+v, want the registered tick", list.Schedules)
	}

	var rm struct {
		Removed bool `json:"removed"`
	}
	if err := client.Call("schedule/remove", map[string]any{"name": "tick"}, &rm); err != nil || !rm.Removed {
		t.Fatalf("schedule/remove = (%v, %+v)", err, rm)
	}
	if err := client.Call("schedule/list", nil, &list); err != nil {
		t.Fatalf("schedule/list after remove: %v", err)
	}
	if len(list.Schedules) != 0 {
		t.Fatalf("list after remove = %+v, want empty", list.Schedules)
	}

	// The daemon scheduler's state file landed under the session dir.
	if _, err := os.Stat(filepath.Join(cfg.SessionDir, schedulerStateFilename)); err != nil {
		t.Fatalf("scheduler state file: %v", err)
	}
}
