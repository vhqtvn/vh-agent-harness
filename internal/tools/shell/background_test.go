// background_test.go — the P6 background dispatch path of run_shell:
// receipt shape, timeout posture (omitted ⇒ the cap), fail-closed
// postures (no dispatcher, sandbox unavailable at dispatch), and the
// streamed exec core.
package shell

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBackgroundSchemaIsValidJSONAndCarriesArg(t *testing.T) {
	var sch map[string]any
	if err := json.Unmarshal([]byte(parametersSchema), &sch); err != nil {
		t.Fatalf("parametersSchema is not valid JSON: %v", err)
	}
	props, _ := sch["properties"].(map[string]any)
	if _, ok := props["background"]; !ok {
		t.Fatal("schema properties missing background")
	}
}

// dispatchRecorder is a BackgroundDispatch fake.
type dispatchRecorder struct {
	mu    sync.Mutex
	calls []BackgroundArgs
}

func (d *dispatchRecorder) dispatch(_ context.Context, a BackgroundArgs) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = append(d.calls, a)
	return "shell-7", nil
}

func TestBackgroundDispatchReceipt(t *testing.T) {
	rec := &dispatchRecorder{}
	cfg := Config{Background: rec.dispatch}
	cfg.normalize()
	content, err := execute(context.Background(), &cfg, json.RawMessage(
		`{"command":"for i in 1 2 3; do echo tick $i; sleep 0.1; done","background":true}`))
	if err != nil {
		t.Fatalf("background execute: %v", err)
	}
	var r backgroundReceipt
	if err := json.Unmarshal([]byte(content), &r); err != nil {
		t.Fatalf("receipt not JSON: %v\n%s", err, content)
	}
	if !r.Background || r.JobID != "shell-7" || !strings.HasPrefix(r.Command, "for i in") {
		t.Fatalf("receipt = %+v", r)
	}
	// Timeout posture: omitted timeout_ms ⇒ the CAP (600s default),
	// not the sync 30s default.
	if r.EffectiveTimeoutMs != DefaultMaxTimeoutMs {
		t.Fatalf("receipt effectiveTimeoutMs = %d, want the cap %d", r.EffectiveTimeoutMs, DefaultMaxTimeoutMs)
	}
	if len(rec.calls) != 1 || rec.calls[0].TimeoutMs != DefaultMaxTimeoutMs {
		t.Fatalf("dispatch calls = %+v", rec.calls)
	}
	// The receipt is deterministic — no wall-clock fields.
	if strings.Contains(content, "durationMs") {
		t.Fatalf("receipt carries non-deterministic facts: %s", content)
	}
}

func TestBackgroundExplicitTimeoutClamps(t *testing.T) {
	rec := &dispatchRecorder{}
	cfg := Config{Background: rec.dispatch}
	cfg.normalize()
	if _, err := execute(context.Background(), &cfg, json.RawMessage(
		`{"command":"sleep 5","background":true,"timeout_ms":9000000}`)); err != nil {
		t.Fatal(err)
	}
	if rec.calls[0].TimeoutMs != DefaultMaxTimeoutMs {
		t.Fatalf("explicit timeout did not clamp to the cap: %d", rec.calls[0].TimeoutMs)
	}
}

func TestBackgroundFailsClosedWithoutDispatcher(t *testing.T) {
	cfg := Config{}
	cfg.normalize()
	_, err := execute(context.Background(), &cfg, json.RawMessage(`{"command":"echo hi","background":true}`))
	if err == nil || !strings.Contains(err.Error(), "no background dispatcher") {
		t.Fatalf("err = %v, want typed fail-closed no-dispatcher", err)
	}
}

func TestBackgroundSandboxUnavailableRefusesDispatch(t *testing.T) {
	rec := &dispatchRecorder{}
	cfg := Config{
		Background: rec.dispatch,
		Sandbox: func(_ *exec.Cmd) error {
			return &SandboxUnavailableError{Mode: "read-only", Reason: "probe"}
		},
		SandboxName: "read-only",
	}
	cfg.normalize()
	_, err := execute(context.Background(), &cfg, json.RawMessage(`{"command":"echo hi","background":true}`))
	var unavail *SandboxUnavailableError
	if !errors.As(err, &unavail) {
		t.Fatalf("err = %v, want typed *SandboxUnavailableError (refuse dispatch)", err)
	}
	if len(rec.calls) != 0 {
		t.Fatal("dispatch must not happen when the sandbox preflight refuses")
	}
}

func TestBackgroundValidationSharedWithSync(t *testing.T) {
	rec := &dispatchRecorder{}
	cfg := Config{Background: rec.dispatch}
	cfg.normalize()
	// A denied command never dispatches (same policy layer as sync).
	cfg.DeniedCommands = []string{"rm -rf"}
	if _, err := execute(context.Background(), &cfg, json.RawMessage(
		`{"command":"rm -rf /tmp/x","background":true}`)); err == nil {
		t.Fatal("denied command must fail before dispatch")
	}
	if len(rec.calls) != 0 {
		t.Fatal("denied command dispatched a job")
	}
	// An escaping workdir never dispatches either.
	if _, err := execute(context.Background(), &cfg, json.RawMessage(
		`{"command":"echo hi","background":true,"workdir":"../escape"}`)); err == nil {
		t.Fatal("escaping workdir must fail before dispatch")
	}
	if len(rec.calls) != 0 {
		t.Fatal("escaping workdir dispatched a job")
	}
}

func TestRunStreamedStreamsCombinedOutputAndFacts(t *testing.T) {
	cfg := Config{}
	cfg.normalize()
	var mu sync.Mutex
	var sb strings.Builder
	out := writerFunc(func(p []byte) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		return sb.Write(p)
	})
	o := RunStreamed(context.Background(), &cfg, "echo streamed-out; echo streamed-err 1>&2; exit 0", 10_000, "", out)
	if o.Cause != CauseExit || o.ExitCode != 0 {
		t.Fatalf("outcome = %+v", o)
	}
	if o.Stdout != "" || o.Stderr != "" {
		t.Fatalf("streamed outcome must not capture streams inline: %+v", o)
	}
	got := sb.String()
	if !strings.Contains(got, "streamed-out") || !strings.Contains(got, "streamed-err") {
		t.Fatalf("combined stream = %q", got)
	}
	if o.DurationMs < 0 {
		t.Fatalf("negative duration: %d", o.DurationMs)
	}
}

func TestRunStreamedTimeoutKillsAndClassifies(t *testing.T) {
	cfg := Config{}
	cfg.normalize()
	start := time.Now()
	o := RunStreamed(context.Background(), &cfg, "sleep 5", 150, "", writerFunc(func(p []byte) (int, error) { return len(p), nil }))
	if o.Cause != CauseTimeout || !o.TimedOut {
		t.Fatalf("outcome = %+v, want timeout classification", o)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("timeout teardown did not bound the run")
	}
}

type writerFunc func(p []byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }
