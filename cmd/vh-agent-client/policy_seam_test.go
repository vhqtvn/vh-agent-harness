// policy_seam_test.go — the P3 policy engine over the LIBRARY-SERVER
// seam (driver_test.go's composition: real FileEngine + openaicompat
// adapter + fake LLM + ask-all pre-observer + net.Pipe). This is the
// honest seam for asks: the daemon's shipped tools never ask, so the
// policy decision path over the REAL approval seam is proven here.
//
// Crux assertions:
//   - policy ALLOW answers without consuming stdin (the hub records
//     ZERO approval registrations and the probe reader zero bytes);
//   - policy HARD-DENY (adversarial: a broad rule TRIES to allow the
//     class) denies the tool, the executor never runs, and the turn
//     still completes honestly.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/cmd/vh-agent-client/policy"
	"github.com/vhqtvn/vh-agent-harness/internal/adapters/openaicompat"
	"github.com/vhqtvn/vh-agent-harness/internal/protocol"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// readProbe wraps a reader and counts the BYTES actually consumed
// (zero bytes read == stdin untouched, even though the hub dispatcher
// goroutine may sit blocked in Read on an empty pipe).
type readProbe struct {
	r     io.Reader
	mu    sync.Mutex
	bytes int64
}

func (p *readProbe) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.mu.Lock()
	p.bytes += int64(n)
	p.mu.Unlock()
	return n, err
}

func (p *readProbe) consumed() int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.bytes
}

// hubApprovalRegistrations reports how many approvals the hub has EVER
// registered (waiters + order) or settled — same-package introspection
// of input.go's bookkeeping.
func hubApprovalRegistrations(h *stdinHub) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.waiters) + len(h.order) + len(h.settled)
}

// llmRequestingThen serves one response requesting the given tool call,
// then one final content response.
func llmRequestingThen(t *testing.T, callID, tool, args, final string) *httptest.Server {
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
				"id": "chatcmpl-policy", "object": "chat.completion", "model": "fake-model",
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
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-policy2", "object": "chat.completion", "model": "fake-model",
			"choices": []map[string]any{{
				"index": 0, "message": map[string]any{"role": "assistant", "content": final},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 1, "total_tokens": 4},
		})
	}))
}

// newAskSeamServer builds the library-seam server registering the GIVEN
// tool definitions behind the ask-all observer (every call asks — the
// approval bridge trigger the daemon's shipped tools lack).
func newAskSeamServer(t *testing.T, llmURL string, defs ...tools.ToolDefinition) (io.ReadWriteCloser, func()) {
	t.Helper()
	svc, cli := net.Pipe()
	engine := &protocol.FileEngine{
		Dir:      t.TempDir(),
		Executor: noopExecutor{},
		Ad:       openaicompat.New(openaicompat.Config{BaseURL: llmURL, Model: "fake-model", APIKey: "test-key"}),
		TurnOpts: tools.TurnOptions{Model: "fake-model"},
	}
	for _, d := range defs {
		engine.TurnOpts.Tools = append(engine.TurnOpts.Tools, d.Spec())
	}
	srv := protocol.NewServer(engine, protocol.NewConn(svc), protocol.ServerOptions{})
	// Composition-order contract (driver_test.go): register AFTER
	// NewServer so the pipeline freezes with the wire approval bridge.
	for _, d := range defs {
		if err := engine.Pipeline().Register(d); err != nil {
			t.Fatalf("register %s: %v", d.Name, err)
		}
	}
	engine.Pipeline().AddPreObserver(askAllObserver{})
	served := make(chan error, 1)
	go func() { served <- srv.Serve(nil) }()
	stop := func() {
		_ = cli.Close()
		select {
		case <-served:
		case <-time.After(2 * time.Second):
			t.Errorf("seam server did not stop")
		}
	}
	return cli, stop
}

// policySeamDriver wires a driver with the policy engine composed in
// front of the interactive responder, with the hub's stdin behind a
// readProbe over a pipe that is NEVER written (any interactive ask
// would block the turn; any consumed byte is recorded).
func policySeamDriver(t *testing.T, rwc io.ReadWriteCloser, pol *policy.Policy, cfg *Config) (*driver, *bytes.Buffer, *bytes.Buffer, *readProbe, func()) {
	t.Helper()
	client := protocol.NewClient(rwc)
	var errbuf, outbuf bytes.Buffer
	pr, pw := io.Pipe()
	probe := &readProbe{r: pr}
	hub := newStdinHub(bufio.NewReader(probe), &errbuf, false)
	hub.start()
	var base ApproverFunc = interactiveApprover(hub, &errbuf)
	if pol != nil {
		base = policyApprover(pol, base, &errbuf)
	}
	d := &driver{
		cfg:      cfg,
		client:   client,
		renderer: newHumanRenderer(&errbuf),
		approver: base,
		answers:  hub,
		out:      &outbuf,
		errw:     &errbuf,
	}
	cleanup := func() { _ = pw.Close() } // unblock the dispatcher goroutine
	return d, &outbuf, &errbuf, probe, cleanup
}

// TestPolicyAllowOverLibrarySeamStdinUntouched: the scripted LLM
// requests a read the policy allows → the policy approver answers
// WITHOUT touching stdin (zero hub registrations, zero bytes
// consumed), the tool executes, the turn completes.
func TestPolicyAllowOverLibrarySeamStdinUntouched(t *testing.T) {
	llm := llmRequestingThen(t, "call-read", "read", `{"path":"docs/x.md"}`, "done after auto-approve")
	defer llm.Close()

	stubRead := tools.ToolDefinition{
		Name:        "read",
		Description: "Stub file read for the policy seam.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}`),
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			return "stub-read-ok", nil
		},
	}
	rwc, stop := newAskSeamServer(t, llm.URL, stubRead)
	defer stop()

	pol, err := policy.Parse("seam.policy", []byte("[[allow]]\ntool = \"read\"\npath = \"docs/\"\n"))
	if err != nil {
		t.Fatalf("policy.Parse: %v", err)
	}
	cfg := &Config{SessionDir: t.TempDir(), Prompt: "read the doc", Mode: ModeOneShot}
	d, out, errbuf, probe, cleanup := policySeamDriver(t, rwc, pol, cfg)
	defer cleanup()

	if err := d.run(context.Background()); err != nil {
		t.Fatalf("driver run: %v (stderr:\n%s)", err, errbuf.String())
	}
	if got := out.String(); got != "done after auto-approve\n" {
		t.Fatalf("stdout = %q (stderr:\n%s)", got, errbuf.String())
	}
	if !strings.Contains(errbuf.String(), "policy: allow read(path=docs/x.md)") {
		t.Fatalf("the policy allow line is missing:\n%s", errbuf.String())
	}
	if strings.Contains(errbuf.String(), "[y/N]") {
		t.Fatalf("no interactive prompt may appear when the policy allows:\n%s", errbuf.String())
	}
	if !strings.Contains(errbuf.String(), "✔ tool result") {
		t.Fatalf("the allowed tool must have executed:\n%s", errbuf.String())
	}
	// THE stdin-untouched proof: zero hub registrations ever, zero
	// bytes consumed.
	if n := hubApprovalRegistrations(d.answers); n != 0 {
		t.Fatalf("the hub must see ZERO interactive registrations, got %d (stderr:\n%s)", n, errbuf.String())
	}
	if b := probe.consumed(); b != 0 {
		t.Fatalf("stdin must stay untouched, %d bytes consumed", b)
	}
}

// TestPolicyHardDenyOverLibrarySeam: the scripted LLM requests a
// run_shell the policy's broad allow-rule TRIES to cover — the
// hard-deny class wins, the executor never runs, the turn completes
// honestly with a denied tool result and final assistant text.
func TestPolicyHardDenyOverLibrarySeam(t *testing.T) {
	llm := llmRequestingThen(t, "call-git", "run_shell", `{"command":"git push origin main"}`, "honest completion after deny")
	defer llm.Close()

	var executed atomicBool
	stubShell := tools.ToolDefinition{
		Name:        "run_shell",
		Description: "Stub shell tool for the policy seam.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}},"required":["command"],"additionalProperties":false}`),
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			executed.set(true)
			return "MUST-NOT-RUN", nil
		},
	}
	rwc, stop := newAskSeamServer(t, llm.URL, stubShell)
	defer stop()

	// Adversarial rule: broadest possible allow for run_shell.
	pol, err := policy.Parse("seam.policy", []byte("[[allow]]\ntool = \"run_shell\"\n"))
	if err != nil {
		t.Fatalf("policy.Parse: %v", err)
	}
	cfg := &Config{SessionDir: t.TempDir(), Prompt: "push it", Mode: ModeOneShot}
	d, out, errbuf, probe, cleanup := policySeamDriver(t, rwc, pol, cfg)
	defer cleanup()

	if err := d.run(context.Background()); err != nil {
		t.Fatalf("a hard-denied turn must still complete cleanly, got %v (stderr:\n%s)", err, errbuf.String())
	}
	if !strings.Contains(errbuf.String(), "policy: HARD-DENY run_shell(") {
		t.Fatalf("the policy hard-deny line is missing:\n%s", errbuf.String())
	}
	if !strings.Contains(errbuf.String(), "⊘ tool denied run_shell") {
		t.Fatalf("the denied tool result must render:\n%s", errbuf.String())
	}
	if strings.Contains(errbuf.String(), "✔ tool result") {
		t.Fatalf("a hard-denied tool must NOT execute:\n%s", errbuf.String())
	}
	if executed.get() {
		t.Fatal("the stub executor ran despite the hard-deny")
	}
	if got := out.String(); got != "honest completion after deny\n" {
		t.Fatalf("stdout = %q (stderr:\n%s)", got, errbuf.String())
	}
	if n := hubApprovalRegistrations(d.answers); n != 0 {
		t.Fatalf("a hard-deny must not touch the hub either, got %d registrations", n)
	}
	if b := probe.consumed(); b != 0 {
		t.Fatalf("stdin must stay untouched, %d bytes consumed", b)
	}
}

// TestPolicyHardDenySingleAmpersandEvasionOverLibrarySeam (review-F2
// regression): the scripted LLM requests the single-'&' evasion —
// `git status & git push …` — which the broadest possible run_shell
// allow-rule would cover if the '&' did not split segments. Through
// the REAL client loop the hard-deny class wins, the executor never
// runs, and the turn completes honestly. (Red proven at the engine
// level in policy/reviewfix_test.go — pre-fix this Decide returned
// allow — which is the sole input this seam rendering depends on.)
func TestPolicyHardDenySingleAmpersandEvasionOverLibrarySeam(t *testing.T) {
	llm := llmRequestingThen(t, "call-git-amp", "run_shell", `{"command":"git status & git push origin HEAD:main"}`, "honest completion after deny")
	defer llm.Close()

	var executed atomicBool
	stubShell := tools.ToolDefinition{
		Name:        "run_shell",
		Description: "Stub shell tool for the policy seam.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}},"required":["command"],"additionalProperties":false}`),
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			executed.set(true)
			return "MUST-NOT-RUN", nil
		},
	}
	rwc, stop := newAskSeamServer(t, llm.URL, stubShell)
	defer stop()

	pol, err := policy.Parse("seam.policy", []byte("[[allow]]\ntool = \"run_shell\"\n"))
	if err != nil {
		t.Fatalf("policy.Parse: %v", err)
	}
	cfg := &Config{SessionDir: t.TempDir(), Prompt: "status and push", Mode: ModeOneShot}
	d, out, errbuf, probe, cleanup := policySeamDriver(t, rwc, pol, cfg)
	defer cleanup()

	if err := d.run(context.Background()); err != nil {
		t.Fatalf("a hard-denied turn must still complete cleanly, got %v (stderr:\n%s)", err, errbuf.String())
	}
	if !strings.Contains(errbuf.String(), "policy: HARD-DENY run_shell(") {
		t.Fatalf("the policy hard-deny line is missing:\n%s", errbuf.String())
	}
	if !strings.Contains(errbuf.String(), "⊘ tool denied run_shell") {
		t.Fatalf("the denied tool result must render:\n%s", errbuf.String())
	}
	if strings.Contains(errbuf.String(), "✔ tool result") {
		t.Fatalf("a hard-denied tool must NOT execute:\n%s", errbuf.String())
	}
	if executed.get() {
		t.Fatal("the stub executor ran despite the hard-deny")
	}
	if got := out.String(); got != "honest completion after deny\n" {
		t.Fatalf("stdout = %q (stderr:\n%s)", got, errbuf.String())
	}
	if n := hubApprovalRegistrations(d.answers); n != 0 {
		t.Fatalf("a hard-deny must not touch the hub either, got %d registrations", n)
	}
	if b := probe.consumed(); b != 0 {
		t.Fatalf("stdin must stay untouched, %d bytes consumed", b)
	}
}

// TestPolicyHardDenySubstitutionWrappedMutationOverLibrarySeam
// (round-2 review regression): the scripted LLM requests the
// substitution-wrapped mutation `echo $(git push origin main)` —
// argv[0] plain and non-git, the mutation hidden inside $() — under
// the broadest possible run_shell allow-rule. Pre-fix the engine
// ALLOWED this shape (proven at the engine level in
// policy/reviewfix_test.go round 2, RED receipts captured); the
// plain-word provability gate now denies it. Through the REAL client
// loop: hard-deny wins, the executor never runs, the turn completes
// honestly.
func TestPolicyHardDenySubstitutionWrappedMutationOverLibrarySeam(t *testing.T) {
	llm := llmRequestingThen(t, "call-git-subst", "run_shell", `{"command":"echo $(git push origin main)"}`, "honest completion after deny")
	defer llm.Close()

	var executed atomicBool
	stubShell := tools.ToolDefinition{
		Name:        "run_shell",
		Description: "Stub shell tool for the policy seam.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}},"required":["command"],"additionalProperties":false}`),
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			executed.set(true)
			return "MUST-NOT-RUN", nil
		},
	}
	rwc, stop := newAskSeamServer(t, llm.URL, stubShell)
	defer stop()

	// Adversarial rule: broadest possible allow for run_shell.
	pol, err := policy.Parse("seam.policy", []byte("[[allow]]\ntool = \"run_shell\"\n"))
	if err != nil {
		t.Fatalf("policy.Parse: %v", err)
	}
	cfg := &Config{SessionDir: t.TempDir(), Prompt: "echo the push", Mode: ModeOneShot}
	d, out, errbuf, probe, cleanup := policySeamDriver(t, rwc, pol, cfg)
	defer cleanup()

	if err := d.run(context.Background()); err != nil {
		t.Fatalf("a hard-denied turn must still complete cleanly, got %v (stderr:\n%s)", err, errbuf.String())
	}
	if !strings.Contains(errbuf.String(), "policy: HARD-DENY run_shell(") {
		t.Fatalf("the policy hard-deny line is missing:\n%s", errbuf.String())
	}
	if !strings.Contains(errbuf.String(), "⊘ tool denied run_shell") {
		t.Fatalf("the denied tool result must render:\n%s", errbuf.String())
	}
	if strings.Contains(errbuf.String(), "✔ tool result") {
		t.Fatalf("a hard-denied tool must NOT execute:\n%s", errbuf.String())
	}
	if executed.get() {
		t.Fatal("the stub executor ran despite the hard-deny")
	}
	if got := out.String(); got != "honest completion after deny\n" {
		t.Fatalf("stdout = %q (stderr:\n%s)", got, errbuf.String())
	}
	if n := hubApprovalRegistrations(d.answers); n != 0 {
		t.Fatalf("a hard-deny must not touch the hub either, got %d registrations", n)
	}
	if b := probe.consumed(); b != 0 {
		t.Fatalf("stdin must stay untouched, %d bytes consumed", b)
	}
}

// TestPolicyHardDenyExecIntermediaryOverLibrarySeam (round-3 review
// regression): the scripted LLM requests the cross-segment pipe
// assembly `echo push | xargs git` under the broadest possible
// run_shell allow-rule. Pre-fix the engine ALLOWED this shape (RED
// receipt captured at the engine level in policy/reviewfix_test.go
// round 3 — the pipe split puts the mutation word and the word git
// in different segments, so no segment shows git+mutation adjacency,
// and every word is plain so gate 2a passed); the exec-intermediary
// tripwires (class 2b) now deny it. Through the REAL client loop:
// hard-deny wins, the executor never runs, the turn completes
// honestly.
func TestPolicyHardDenyExecIntermediaryOverLibrarySeam(t *testing.T) {
	llm := llmRequestingThen(t, "call-xargs", "run_shell", `{"command":"echo push | xargs git"}`, "honest completion after deny")
	defer llm.Close()

	var executed atomicBool
	stubShell := tools.ToolDefinition{
		Name:        "run_shell",
		Description: "Stub shell tool for the policy seam.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}},"required":["command"],"additionalProperties":false}`),
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			executed.set(true)
			return "MUST-NOT-RUN", nil
		},
	}
	rwc, stop := newAskSeamServer(t, llm.URL, stubShell)
	defer stop()

	// Adversarial rule: broadest possible allow for run_shell.
	pol, err := policy.Parse("seam.policy", []byte("[[allow]]\ntool = \"run_shell\"\n"))
	if err != nil {
		t.Fatalf("policy.Parse: %v", err)
	}
	cfg := &Config{SessionDir: t.TempDir(), Prompt: "pipe the push", Mode: ModeOneShot}
	d, out, errbuf, probe, cleanup := policySeamDriver(t, rwc, pol, cfg)
	defer cleanup()

	if err := d.run(context.Background()); err != nil {
		t.Fatalf("a hard-denied turn must still complete cleanly, got %v (stderr:\n%s)", err, errbuf.String())
	}
	if !strings.Contains(errbuf.String(), "policy: HARD-DENY run_shell(") {
		t.Fatalf("the policy hard-deny line is missing:\n%s", errbuf.String())
	}
	if !strings.Contains(errbuf.String(), "exec intermediary") {
		t.Fatalf("the hard-deny reason must name exec intermediary:\n%s", errbuf.String())
	}
	if !strings.Contains(errbuf.String(), "⊘ tool denied run_shell") {
		t.Fatalf("the denied tool result must render:\n%s", errbuf.String())
	}
	if strings.Contains(errbuf.String(), "✔ tool result") {
		t.Fatalf("a hard-denied tool must NOT execute:\n%s", errbuf.String())
	}
	if executed.get() {
		t.Fatal("the stub executor ran despite the hard-deny")
	}
	if got := out.String(); got != "honest completion after deny\n" {
		t.Fatalf("stdout = %q (stderr:\n%s)", got, errbuf.String())
	}
	if n := hubApprovalRegistrations(d.answers); n != 0 {
		t.Fatalf("a hard-deny must not touch the hub either, got %d registrations", n)
	}
	if b := probe.consumed(); b != 0 {
		t.Fatalf("stdin must stay untouched, %d bytes consumed", b)
	}
}

// TestPolicyHardDenyDisplacedXargsOverLibrarySeam (round-4 review
// regression): the scripted LLM requests the displaced-intermediary
// evasion `echo push | nohup xargs git` — nohup moves xargs out of
// argv[0], where the round-3 tripwire lived — under the broadest
// possible run_shell allow-rule. Pre-fix the engine ALLOWED this
// shape (RED receipt captured at the engine level in
// policy/reviewfix_test.go round 4); the position-independent
// word-level tripwire (class 2b) now denies it wherever the word
// xargs appears. Through the REAL client loop: hard-deny wins, the
// executor never runs, the turn completes honestly.
func TestPolicyHardDenyDisplacedXargsOverLibrarySeam(t *testing.T) {
	llm := llmRequestingThen(t, "call-xargs-displaced", "run_shell", `{"command":"echo push | nohup xargs git"}`, "honest completion after deny")
	defer llm.Close()

	var executed atomicBool
	stubShell := tools.ToolDefinition{
		Name:        "run_shell",
		Description: "Stub shell tool for the policy seam.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}},"required":["command"],"additionalProperties":false}`),
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			executed.set(true)
			return "MUST-NOT-RUN", nil
		},
	}
	rwc, stop := newAskSeamServer(t, llm.URL, stubShell)
	defer stop()

	// Adversarial rule: broadest possible allow for run_shell.
	pol, err := policy.Parse("seam.policy", []byte("[[allow]]\ntool = \"run_shell\"\n"))
	if err != nil {
		t.Fatalf("policy.Parse: %v", err)
	}
	cfg := &Config{SessionDir: t.TempDir(), Prompt: "pipe the push through nohup", Mode: ModeOneShot}
	d, out, errbuf, probe, cleanup := policySeamDriver(t, rwc, pol, cfg)
	defer cleanup()

	if err := d.run(context.Background()); err != nil {
		t.Fatalf("a hard-denied turn must still complete cleanly, got %v (stderr:\n%s)", err, errbuf.String())
	}
	if !strings.Contains(errbuf.String(), "policy: HARD-DENY run_shell(") {
		t.Fatalf("the policy hard-deny line is missing:\n%s", errbuf.String())
	}
	if !strings.Contains(errbuf.String(), "exec intermediary") {
		t.Fatalf("the hard-deny reason must name exec intermediary:\n%s", errbuf.String())
	}
	if !strings.Contains(errbuf.String(), "⊘ tool denied run_shell") {
		t.Fatalf("the denied tool result must render:\n%s", errbuf.String())
	}
	if strings.Contains(errbuf.String(), "✔ tool result") {
		t.Fatalf("a hard-denied tool must NOT execute:\n%s", errbuf.String())
	}
	if executed.get() {
		t.Fatal("the stub executor ran despite the hard-deny")
	}
	if got := out.String(); got != "honest completion after deny\n" {
		t.Fatalf("stdout = %q (stderr:\n%s)", got, errbuf.String())
	}
	if n := hubApprovalRegistrations(d.answers); n != 0 {
		t.Fatalf("a hard-deny must not touch the hub either, got %d registrations", n)
	}
	if b := probe.consumed(); b != 0 {
		t.Fatalf("stdin must stay untouched, %d bytes consumed", b)
	}
}

// atomicBool is a tiny test flag (the stub executor runs on an engine
// goroutine).
type atomicBool struct {
	mu sync.Mutex
	v  bool
}

func (a *atomicBool) set(b bool) {
	a.mu.Lock()
	a.v = b
	a.mu.Unlock()
}

func (a *atomicBool) get() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.v
}

// TestPolicyAskDelegatesOverLibrarySeam: an unmatched tool asks; the
// composition delegates to the interactive responder (scripted `y`
// grants) — proving --policy composes WITH the human loop rather than
// replacing it.
func TestPolicyAskDelegatesOverLibrarySeam(t *testing.T) {
	llm := llmRequestingThen(t, "call-ask", "ask_tool", `{"text":"needs a human"}`, "answered by human")
	defer llm.Close()

	askTool := tools.ToolDefinition{
		Name:        "ask_tool",
		Description: "Test tool whose every call requires an approval answer.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"],"additionalProperties":false}`),
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			return "tool-ran-after-grant", nil
		},
	}
	rwc, stop := newAskSeamServer(t, llm.URL, askTool)
	defer stop()

	pol, err := policy.Parse("seam.policy", []byte("[[allow]]\ntool = \"clock\"\n"))
	if err != nil {
		t.Fatalf("policy.Parse: %v", err)
	}

	var errbuf, outbuf bytes.Buffer
	hub := newStdinHub(bufio.NewReader(strings.NewReader("y\n")), &errbuf, false)
	hub.start()
	base := policyApprover(pol, interactiveApprover(hub, &errbuf), &errbuf)
	cfg := &Config{SessionDir: t.TempDir(), Prompt: "please use the tool", Mode: ModeOneShot}
	d := &driver{
		cfg:      cfg,
		client:   protocol.NewClient(rwc),
		renderer: newHumanRenderer(&errbuf),
		approver: base,
		answers:  hub,
		out:      &outbuf,
		errw:     &errbuf,
	}
	if err := d.run(context.Background()); err != nil {
		t.Fatalf("driver run: %v (stderr:\n%s)", err, errbuf.String())
	}
	if !strings.Contains(errbuf.String(), "policy: ask → human") {
		t.Fatalf("the policy ask line is missing:\n%s", errbuf.String())
	}
	if !strings.Contains(errbuf.String(), "→ granted") {
		t.Fatalf("the delegate must have asked and been granted:\n%s", errbuf.String())
	}
	if got := outbuf.String(); got != "answered by human\n" {
		t.Fatalf("stdout = %q (stderr:\n%s)", got, errbuf.String())
	}
}
