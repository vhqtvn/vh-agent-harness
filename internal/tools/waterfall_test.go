package tools

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// --- slice-3 test doubles -------------------------------------------------

// scriptedObserver returns one fixed verdict and records the (detached)
// call it observed. mutate lets a test try to modify its LOCAL copy — the
// pipeline must hand every observer its own detached args.
type scriptedObserver struct {
	name    string
	verdict Verdict
	mutate  func(c *session.ToolCall)
	sawArgs atomic.Value // json.RawMessage
	calls   atomic.Int32
}

func (o *scriptedObserver) Name() string { return o.name }

func (o *scriptedObserver) ObservePreExecute(c session.ToolCall) Verdict {
	o.calls.Add(1)
	o.sawArgs.Store(append(json.RawMessage(nil), c.Args...))
	if o.mutate != nil {
		o.mutate(&c)
	}
	return o.verdict
}

// countingGuard records how many times it was consulted.
type countingGuard struct {
	name   string
	denied map[string]bool
	checks atomic.Int32
}

func (g *countingGuard) Name() string { return g.name }

func (g *countingGuard) Check(c session.ToolCall) error {
	g.checks.Add(1)
	if g.denied[c.Name] {
		return errString(g.name + " veto")
	}
	return nil
}

type errString string

func (e errString) Error() string { return string(e) }

// fakeApprover resolves ask verdicts from a script (last decision
// repeats) and records what it was asked.
type fakeApprover struct {
	decisions []ApprovalDecision
	asked     atomic.Int32
	mu        chan struct{} // closed once first ask seen
	sawReason string
	sawCall   session.ToolCall
}

func newFakeApprover(d ...ApprovalDecision) *fakeApprover {
	ch := make(chan struct{})
	return &fakeApprover{decisions: d, mu: ch}
}

func (a *fakeApprover) Approve(_ context.Context, call session.ToolCall, reason string) ApprovalDecision {
	n := int(a.asked.Add(1))
	a.sawReason = reason
	a.sawCall = call
	if n == 1 {
		close(a.mu)
	}
	i := n - 1
	if i >= len(a.decisions) {
		i = len(a.decisions) - 1
	}
	return a.decisions[i]
}

// recordingPostObserver records the results it observed and optionally
// replaces content.
type recordingPostObserver struct {
	name      string
	replace   string // non-empty ⇒ call ReplaceContent
	observed  atomic.Int32
	sawIsErr  atomic.Bool
	sawDenied atomic.Bool
}

func (o *recordingPostObserver) Name() string { return o.name }

func (o *recordingPostObserver) ObservePostExecute(_ session.ToolCall, res *PostResult) {
	o.observed.Add(1)
	v := res.View()
	o.sawIsErr.Store(v.IsError)
	o.sawDenied.Store(v.Denied)
	if o.replace != "" {
		res.ReplaceContent(o.replace)
	}
}

// fakeClock hands out a channel the TEST controls, so timeout
// classification is deterministic (no real sleeps racing the select).
type fakeClock struct {
	ch      chan time.Time
	request atomic.Int64 // ns of the last After request
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.request.Store(int64(d))
	return c.ch
}

func okBody(content string) func(context.Context, json.RawMessage) (string, error) {
	return func(context.Context, json.RawMessage) (string, error) { return content, nil }
}

// --- verdict lattice ------------------------------------------------------

func TestWaterfallDenyOutranksAllow(t *testing.T) {
	for _, tc := range []struct {
		name      string
		verdicts  []Verdict
		denierIdx int
	}{
		{"allow then deny", []Verdict{Allow(), Deny("policy: read-only session")}, 1},
		{"deny then allow", []Verdict{Deny("policy: read-only session"), Allow()}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			executed := false
			p := NewPipeline()
			if err := p.Register(ToolDefinition{Name: "echo", Execute: func(context.Context, json.RawMessage) (string, error) {
				executed = true
				return "ran", nil
			}}); err != nil {
				t.Fatalf("Register: %v", err)
			}
			for i, v := range tc.verdicts {
				p.AddPreObserver(&scriptedObserver{name: "obs-" + string(rune('a'+i)), verdict: v})
			}
			res := p.Execute(context.Background(), session.ToolCall{ID: "c1", Name: "echo", Args: json.RawMessage(`{}`)})
			if executed {
				t.Fatal("denied call must never execute")
			}
			if !res.Denied || !res.IsError {
				t.Fatalf("expected denied isError result: %+v", res)
			}
			if res.DeniedBy != "obs-"+string(rune('a'+tc.denierIdx)) {
				t.Fatalf("deniedBy = %q, want the deny observer's identity", res.DeniedBy)
			}
			if !strings.Contains(res.Content, "read-only session") {
				t.Fatalf("denial content must carry the reason: %+v", res)
			}
		})
	}
}

func TestWaterfallDownstreamAllowResolvesAsk(t *testing.T) {
	// An ask followed by a downstream allow resolves WITHOUT consulting
	// the approver — the poison approver proves non-consultation.
	p := NewPipelineWithOptions(PipelineOptions{Approver: newFakeApprover(ApprovalDecision{})})
	if err := p.Register(ToolDefinition{Name: "echo", Execute: okBody("ran")}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	p.AddPreObserver(&scriptedObserver{name: "asker", verdict: Ask("writes outside workspace")})
	p.AddPreObserver(&scriptedObserver{name: "checker", verdict: Allow()})
	res := p.Execute(context.Background(), session.ToolCall{ID: "c1", Name: "echo"})
	if res.IsError || res.Denied || res.Content != "ran" {
		t.Fatalf("downstream allow must resolve the ask and execute: %+v", res)
	}
}

func TestWaterfallAskWithoutDownstreamAllowNeedsApproval(t *testing.T) {
	// allow THEN ask: nothing downstream resolves the ask, so the
	// approver IS consulted.
	ap := newFakeApprover(ApprovalDecision{Allow: true})
	p := NewPipelineWithOptions(PipelineOptions{Approver: ap})
	if err := p.Register(ToolDefinition{Name: "echo", Execute: okBody("ran")}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	p.AddPreObserver(&scriptedObserver{name: "first", verdict: Allow()})
	p.AddPreObserver(&scriptedObserver{name: "asker", verdict: Ask(" destructive op")})
	res := p.Execute(context.Background(), session.ToolCall{ID: "c1", Name: "echo", Args: json.RawMessage(`{"a":1}`)})
	if res.IsError || res.Content != "ran" {
		t.Fatalf("approved ask must execute: %+v", res)
	}
	if ap.asked.Load() != 1 {
		t.Fatalf("approver consulted %d times, want 1", ap.asked.Load())
	}
	if ap.sawReason != " destructive op" || string(ap.sawCall.Args) != `{"a":1}` {
		t.Fatalf("approver context = %q / %s", ap.sawReason, ap.sawCall.Args)
	}
}

func TestWaterfallAskWithoutApproverFailsClosed(t *testing.T) {
	executed := false
	p := NewPipeline() // no approver
	if err := p.Register(ToolDefinition{Name: "echo", Execute: func(context.Context, json.RawMessage) (string, error) {
		executed = true
		return "ran", nil
	}}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	p.AddPreObserver(&scriptedObserver{name: "asker", verdict: Ask("needs human eyes")})
	res := p.Execute(context.Background(), session.ToolCall{ID: "c1", Name: "echo"})
	if executed {
		t.Fatal("fail-closed violated: ask without approver executed")
	}
	if !res.Denied || !res.IsError {
		t.Fatalf("expected denied result: %+v", res)
	}
	if !strings.Contains(res.Content, "no approver") {
		t.Fatalf("denial must name the fail-closed cause: %+v", res)
	}
	if res.DenyReason == "" {
		t.Fatalf("denial must carry a reason: %+v", res)
	}
}

func TestWaterfallAskWithApproverDenyDenies(t *testing.T) {
	executed := false
	p := NewPipelineWithOptions(PipelineOptions{Approver: newFakeApprover(ApprovalDecision{Allow: false, Reason: "operator said no"})})
	if err := p.Register(ToolDefinition{Name: "echo", Execute: func(context.Context, json.RawMessage) (string, error) {
		executed = true
		return "ran", nil
	}}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	p.AddPreObserver(&scriptedObserver{name: "asker", verdict: Ask("destructive op")})
	res := p.Execute(context.Background(), session.ToolCall{ID: "c1", Name: "echo"})
	if executed {
		t.Fatal("operator denial must stop execution")
	}
	if !res.Denied || res.DeniedBy != "asker" || !strings.Contains(res.DenyReason, "operator said no") {
		t.Fatalf("denied result must carry observer identity + approver reason: %+v", res)
	}
}

func TestWaterfallInvalidVerdictFailsClosed(t *testing.T) {
	executed := false
	p := NewPipeline()
	if err := p.Register(ToolDefinition{Name: "echo", Execute: func(context.Context, json.RawMessage) (string, error) {
		executed = true
		return "ran", nil
	}}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	p.AddPreObserver(&scriptedObserver{name: "broken", verdict: Verdict{}}) // zero kind
	res := p.Execute(context.Background(), session.ToolCall{ID: "c1", Name: "echo"})
	if executed || !res.Denied {
		t.Fatalf("unknown verdict kind must fail closed: executed=%v res=%+v", executed, res)
	}
}

func TestGuardEvaluatedAfterWaterfall(t *testing.T) {
	// The waterfall runs FIRST and short-circuits: a denial upstream means
	// the monotonic guards are never consulted; a waterfall allow still
	// leaves the guard free to tighten and deny.
	t.Run("waterfall deny short-circuits guards", func(t *testing.T) {
		g := &countingGuard{name: "counter", denied: map[string]bool{}}
		p := NewPipeline()
		if err := p.Register(ToolDefinition{Name: "echo", Execute: okBody("ran")}); err != nil {
			t.Fatalf("Register: %v", err)
		}
		p.AddGuard(g)
		p.AddPreObserver(&scriptedObserver{name: "asker", verdict: Ask("x")}) // no approver ⇒ deny
		res := p.Execute(context.Background(), session.ToolCall{ID: "c1", Name: "echo"})
		if !res.Denied {
			t.Fatalf("expected denial: %+v", res)
		}
		if g.checks.Load() != 0 {
			t.Fatalf("guards consulted %d times after a waterfall denial; must be 0", g.checks.Load())
		}
	})
	t.Run("guard tightens after waterfall allow", func(t *testing.T) {
		g := &countingGuard{name: "fs-policy", denied: map[string]bool{"echo": true}}
		p := NewPipeline()
		if err := p.Register(ToolDefinition{Name: "echo", Execute: okBody("ran")}); err != nil {
			t.Fatalf("Register: %v", err)
		}
		p.AddGuard(g)
		p.AddPreObserver(&scriptedObserver{name: "ok", verdict: Allow()})
		res := p.Execute(context.Background(), session.ToolCall{ID: "c1", Name: "echo"})
		if !res.Denied || res.DeniedBy != "fs-policy" || g.checks.Load() != 1 {
			t.Fatalf("guard must deny after waterfall allow: %+v checks=%d", res, g.checks.Load())
		}
		if !strings.Contains(res.Content, "fs-policy") {
			t.Fatalf("guard denial content must name the guard: %+v", res)
		}
	})
}

func TestObserversAndGuardsCannotModifyArgs(t *testing.T) {
	// Behavioral half of the cannot-modify contract: an observer and a
	// guard that both scribble over the bytes of their LOCAL args copy
	// cannot affect what the executor receives.
	orig := `{"text":"pristine"}`
	scribble := func(c *session.ToolCall) {
		if len(c.Args) > 0 {
			c.Args[0] = 'X'
		}
		c.Args = json.RawMessage(`{"text":"forged"}`)
	}
	p := NewPipeline()
	if err := p.Register(ToolDefinition{Name: "echo", Execute: func(_ context.Context, args json.RawMessage) (string, error) {
		return string(args), nil
	}}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	obs := &scriptedObserver{name: "scribbler", verdict: Allow(), mutate: scribble}
	p.AddPreObserver(obs)
	g := &mutatingGuard{name: "scribble-guard"}
	p.AddGuard(g)
	res := p.Execute(context.Background(), session.ToolCall{ID: "c1", Name: "echo", Args: json.RawMessage(orig)})
	if res.IsError || res.Content != orig {
		t.Fatalf("executor must see the pristine args: %+v", res)
	}
	if string(obs.sawArgs.Load().(json.RawMessage)) != orig {
		t.Fatalf("observer saw %s, want %s", obs.sawArgs.Load(), orig)
	}
	if string(g.saw) != orig {
		t.Fatalf("guard saw %s, want %s", g.saw, orig)
	}
}

// mutatingGuard scribbles its args copy and records what it saw.
type mutatingGuard struct {
	name string
	saw  json.RawMessage
}

func (g *mutatingGuard) Name() string { return g.name }

func (g *mutatingGuard) Check(c session.ToolCall) error {
	g.saw = append(json.RawMessage(nil), c.Args...)
	if len(c.Args) > 0 {
		c.Args[0] = 'X'
	}
	c.Args = json.RawMessage(`{"text":"forged"}`)
	return nil
}

func TestWaterfallDenialLoggedWithMarker(t *testing.T) {
	var wb writeBuffer
	lg, err := session.NewLog(&wb, "sess-deny-mark", time.Date(2026, 8, 20, 9, 30, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	p := NewPipeline()
	if err := p.Register(ToolDefinition{Name: "echo", Execute: okBody("ran")}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	p.AddPreObserver(&scriptedObserver{name: "asker", verdict: Ask("needs eyes")})
	res, err := p.ExecuteLogged(context.Background(), lg, session.ToolCall{ID: "c1", Name: "echo"})
	if err != nil {
		t.Fatalf("ExecuteLogged: %v", err)
	}
	if !res.Denied {
		t.Fatalf("expected denial: %+v", res)
	}
	events := lg.Events()
	var tr session.ToolResultPayload
	if err := json.Unmarshal(events[len(events)-1].Payload, &tr); err != nil {
		t.Fatalf("tool/result payload: %v", err)
	}
	if !tr.Denied || !tr.IsError || tr.DeniedBy != "asker" || tr.DenyReason == "" {
		t.Fatalf("denial marker missing from logged result: %+v", tr)
	}
}
