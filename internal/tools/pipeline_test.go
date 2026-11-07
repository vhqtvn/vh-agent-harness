package tools

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// --- timeout classification (deterministic via injected clock) -----------

func TestExecuteTimeoutClassified(t *testing.T) {
	fc := &fakeClock{ch: make(chan time.Time)}
	entered := make(chan struct{})
	executed := false
	p := NewPipelineWithOptions(PipelineOptions{Clock: fc})
	if err := p.Register(ToolDefinition{
		Name:      "slow",
		TimeoutMs: 5000,
		Execute: func(ctx context.Context, _ json.RawMessage) (string, error) {
			executed = true
			close(entered)
			<-ctx.Done() // a well-behaved body honors cancellation
			return "", ctx.Err()
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	resCh := make(chan Result, 1)
	go func() {
		resCh <- p.Execute(context.Background(), session.ToolCall{ID: "c1", Name: "slow"})
	}()
	<-entered    // body is in flight
	close(fc.ch) // the timeout fires
	res := <-resCh
	if !executed {
		t.Fatal("the dispatch must have reached the body")
	}
	if !res.TimedOut || !res.IsError {
		t.Fatalf("timeout must classify as isError+timedOut: %+v", res)
	}
	if !strings.Contains(res.Content, "timed out") {
		t.Fatalf("timeout content = %q", res.Content)
	}
	if got := time.Duration(fc.request.Load()); got != 5000*time.Millisecond {
		t.Fatalf("timeout requested = %v, want 5000ms (per-tool timeoutMs)", got)
	}
}

func TestExecuteWithinTimeoutSucceeds(t *testing.T) {
	// The fake clock's channel never fires, so a body that returns before
	// the timeout completes deterministically.
	fc := &fakeClock{ch: make(chan time.Time)}
	p := NewPipelineWithOptions(PipelineOptions{Clock: fc})
	if err := p.Register(ToolDefinition{
		Name:      "quick",
		TimeoutMs: 5000,
		Execute:   okBody("fast path"),
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	res := p.Execute(context.Background(), session.ToolCall{ID: "c1", Name: "quick"})
	if res.IsError || res.TimedOut || res.Content != "fast path" {
		t.Fatalf("in-timeout execution must succeed: %+v", res)
	}
}

func TestExecuteErrorNotConflatedWithTimeout(t *testing.T) {
	// Orthogonal cause facts: a plain execution error is isError WITHOUT
	// the timedOut fact.
	p := NewPipeline()
	if err := p.Register(ToolDefinition{
		Name: "boom",
		Execute: func(context.Context, json.RawMessage) (string, error) {
			return "", errors.New("disk on fire")
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	res := p.Execute(context.Background(), session.ToolCall{ID: "c1", Name: "boom"})
	if !res.IsError || res.TimedOut {
		t.Fatalf("plain error must not carry the timeout fact: %+v", res)
	}
}

// --- post-execute observers ----------------------------------------------

func TestPostExecuteReplaceProvenance(t *testing.T) {
	p := NewPipeline()
	if err := p.Register(ToolDefinition{Name: "read_file", Execute: okBody("SECRET-key=value\nother")}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	p.AddPostObserver(&recordingPostObserver{name: "redactor", replace: "SECRET-[masked]"})
	res := p.Execute(context.Background(), session.ToolCall{ID: "c1", Name: "read_file"})
	if res.IsError || res.Content != "SECRET-[masked]" {
		t.Fatalf("replaced content expected: %+v", res)
	}
	if res.ReplacedBy != "redactor" {
		t.Fatalf("replace provenance missing: %+v", res)
	}
}

func TestPostExecuteReplaceCannotFlipIsError(t *testing.T) {
	// A replacer that runs on an ERROR result may rewrite the content but
	// the isError fact is sticky — there is no API path to flip it.
	p := NewPipeline()
	if err := p.Register(ToolDefinition{Name: "boom", Execute: func(context.Context, json.RawMessage) (string, error) {
		return "", errors.New("boom")
	}}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	p.AddPostObserver(&recordingPostObserver{name: "explainer", replace: "failed for a boring reason"})
	res := p.Execute(context.Background(), session.ToolCall{ID: "c1", Name: "boom"})
	if !res.IsError {
		t.Fatalf("isError must be unflippable: %+v", res)
	}
	if res.Content != "failed for a boring reason" || res.ReplacedBy != "explainer" {
		t.Fatalf("replace on error results must still work: %+v", res)
	}
}

func TestPostObserverRunsOnErrorResults(t *testing.T) {
	p := NewPipeline()
	if err := p.Register(ToolDefinition{Name: "boom", Execute: func(context.Context, json.RawMessage) (string, error) {
		return "", errors.New("boom")
	}}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	obs := &recordingPostObserver{name: "watcher"}
	p.AddPostObserver(obs)
	res := p.Execute(context.Background(), session.ToolCall{ID: "c1", Name: "boom"})
	if obs.observed.Load() != 1 || !obs.sawIsErr.Load() {
		t.Fatalf("post-observe must run on error results: observed=%d sawErr=%v", obs.observed.Load(), obs.sawIsErr.Load())
	}
	if !res.IsError {
		t.Fatalf("still an error: %+v", res)
	}
}

func TestPostObserverSkippedOnDenial(t *testing.T) {
	// Denials exit before dispatch; there is no executed outcome to
	// observe.
	p := NewPipeline()
	if err := p.Register(ToolDefinition{Name: "echo", Execute: okBody("ran")}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	obs := &recordingPostObserver{name: "watcher"}
	p.AddPostObserver(obs)
	p.AddPreObserver(&scriptedObserver{name: "asker", verdict: Ask("x")})
	_ = p.Execute(context.Background(), session.ToolCall{ID: "c1", Name: "echo"})
	if obs.observed.Load() != 0 {
		t.Fatalf("post-observer ran on a denial (%d times); denials are unexecuted", obs.observed.Load())
	}
}

// --- type-level cannot-modify contract -----------------------------------

// TestObserverInterfacesCannotModifyAtTypeLevel pins the compile-level
// half of the guards-cannot-modify contract via reflection:
//   - Guard and PreExecuteObserver take the call BY VALUE with no pointer
//     parameters at all — there is no channel through which to mutate the
//     pipeline's call, and their returns are verdict-only;
//   - PostResult (the one pointer a post-observer legitimately holds, so
//     ReplaceContent can take effect) has NO exported fields, and every
//     method it exposes is in the sanctioned set {View, ReplaceContent} —
//     no method can reach IsError, Denied, CallID or Name.
func TestObserverInterfacesCannotModifyAtTypeLevel(t *testing.T) {
	for _, iface := range []reflect.Type{
		reflect.TypeOf((*Guard)(nil)).Elem(),
		reflect.TypeOf((*PreExecuteObserver)(nil)).Elem(),
	} {
		mName := "ObservePreExecute"
		if iface == reflect.TypeOf((*Guard)(nil)).Elem() {
			mName = "Check"
		}
		m, ok := iface.MethodByName(mName)
		if !ok {
			t.Fatalf("%s has no %s method", iface.Name(), mName)
		}
		// Interface method types carry no receiver In: the call parameter
		// is the only argument, and it must not be a pointer.
		if m.Type.NumIn() != 1 {
			t.Fatalf("%s.%s arity = %d, want exactly the call parameter", iface.Name(), mName, m.Type.NumIn())
		}
		if m.Type.In(0).Kind() == reflect.Ptr {
			t.Fatalf("%s.%s takes a pointer — mutation channel", iface.Name(), mName)
		}
		if n := m.Type.NumOut(); n != 1 {
			t.Fatalf("%s.%s must be verdict-only (1 return), got %d", iface.Name(), mName, n)
		}
	}
	pt := reflect.TypeOf(PostResult{})
	if pt.Kind() == reflect.Ptr {
		pt = pt.Elem()
	}
	for i := 0; i < pt.NumField(); i++ {
		if pt.Field(i).IsExported() {
			t.Fatalf("PostResult has exported field %q — mutation channel", pt.Field(i).Name)
		}
	}
	sanctioned := map[string]bool{"View": true, "ReplaceContent": true}
	for i := 0; i < pt.NumMethod(); i++ {
		name := pt.Method(i).Name
		if !sanctioned[name] {
			t.Fatalf("PostResult exposes unsanctioned method %q", name)
		}
	}
}

// --- frozen canonical result logging --------------------------------------

func TestExecuteLoggedCarriesFullMeta(t *testing.T) {
	var wb writeBuffer
	lg, err := session.NewLog(&wb, "sess-meta", time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	p := NewPipeline()
	if err := p.Register(ToolDefinition{Name: "read_file", Execute: okBody("raw")}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	p.AddPostObserver(&recordingPostObserver{name: "redactor", replace: "masked"})
	res, err := p.ExecuteLogged(context.Background(), lg, session.ToolCall{ID: "c1", Name: "read_file"})
	if err != nil {
		t.Fatalf("ExecuteLogged: %v", err)
	}
	events := lg.Events()
	var tr session.ToolResultPayload
	if err := json.Unmarshal(events[len(events)-1].Payload, &tr); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if tr.Content != "masked" || tr.ReplacedBy != "redactor" || tr.IsError {
		t.Fatalf("logged meta = %+v (res=%+v)", tr, res)
	}
}

func TestBatchPreLogFailsClosed(t *testing.T) {
	// Header (write #1) succeeds; the batch's FIRST tool/call intent
	// (write #2) fails — no execution in the batch may start.
	w := &failAfterWriter{n: 1, fail: errors.New("disk full")}
	lg, err := session.NewLog(w, "sess-batch-fc", time.Date(2026, 8, 20, 10, 5, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	executed := 0
	p := NewPipeline()
	if err := p.Register(ToolDefinition{Name: "echo", IsConcurrencySafe: true, Execute: func(context.Context, json.RawMessage) (string, error) {
		executed++
		return "ran", nil
	}}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	_, err = p.ExecuteBatchLogged(context.Background(), lg, []session.ToolCall{
		{ID: "c1", Name: "echo"},
		{ID: "c2", Name: "echo"},
	})
	if err == nil {
		t.Fatal("expected batch to fail when intent logging fails")
	}
	if executed != 0 {
		t.Fatalf("fail-closed violated: %d executions despite unloggable intent", executed)
	}
}
