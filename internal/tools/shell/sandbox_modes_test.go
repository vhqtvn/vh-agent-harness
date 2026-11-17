package shell

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/execsandbox"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// unavailableDetect is the injected probe for fail-closed tests: it
// simulates a platform/config without the OS sandbox primitives.
func unavailableDetect() execsandbox.Features {
	return execsandbox.Features{Landlock: false, Seccomp: false}
}

// TestParseSandboxMode pins the closed mode vocabulary: off |
// read-only | workspace-write. "danger-full-access" is REFUSED as
// redundant with off (the loud nil-default is the no-confinement
// posture); the CLI exec-sandbox modes (best-effort/strict) are
// refused as verb-shaped, not tool modes.
func TestParseSandboxMode(t *testing.T) {
	for s, want := range map[string]SandboxMode{
		"off":             SandboxOff,
		"read-only":       SandboxReadOnly,
		"workspace-write": SandboxWorkspaceWrite,
	} {
		got, err := ParseSandboxMode(s)
		if err != nil || got != want {
			t.Fatalf("ParseSandboxMode(%q) = %q, %v; want %q", s, got, err, want)
		}
	}
	for _, s := range []string{"", "danger-full-access", "best-effort", "strict", "readonly", "workspace_write", "FULL"} {
		if _, err := ParseSandboxMode(s); err == nil {
			t.Fatalf("ParseSandboxMode(%q) must fail", s)
		}
	}

	// The refusal messages carry their reason.
	_, err := ParseSandboxMode("danger-full-access")
	if err == nil || !strings.Contains(err.Error(), "redundant with off") {
		t.Fatalf("danger-full-access refusal must name the redundancy with off: %v", err)
	}
	_, err = ParseSandboxMode("strict")
	if err == nil || !strings.Contains(err.Error(), "CLI") {
		t.Fatalf("strict refusal must name the CLI verb confusion: %v", err)
	}
}

// TestNewSandboxFuncRejectsOff: the constructor is for CONFINEMENT
// only; off is expressed by leaving Config.Sandbox nil (the loud
// pre-slice default), never by a no-op sandbox func.
func TestNewSandboxFuncRejectsOff(t *testing.T) {
	if _, err := NewSandboxFunc(SandboxOptions{Mode: SandboxOff}); err == nil {
		t.Fatalf("NewSandboxFunc(off) must fail: off means leave Config.Sandbox nil")
	}
	if _, err := NewSandboxFunc(SandboxOptions{}); err == nil {
		t.Fatalf("NewSandboxFunc(zero mode) must fail")
	}
}

// TestSandboxUnavailableFailClosed is the typed fail-closed contract:
// when the backend is unavailable, a sandboxed run NEVER executes the
// command and the failure is a *SandboxUnavailableError — through the
// SandboxFunc, through run() (CauseError with sandbox provenance), and
// through execute() (typed error for the Pipeline to normalize into an
// isError result).
func TestSandboxUnavailableFailClosed(t *testing.T) {
	fn, err := newSandboxFuncDetect(SandboxOptions{Mode: SandboxReadOnly}, unavailableDetect)
	if err != nil {
		t.Fatalf("constructor: %v", err)
	}

	// Direct seam: the func itself returns the typed error and never
	// rewraps the command (no backend ⇒ no transformation, no run).
	dummy := &exec.Cmd{Path: "/bin/true", Args: []string{"true"}}
	if err := fn(dummy); !IsSandboxUnavailable(err) {
		t.Fatalf("fn error = %v, want a typed sandbox-unavailable error", err)
	}
	if dummy.Path != "/bin/true" || len(dummy.Env) != 0 {
		t.Fatalf("unavailable sandbox must refuse BEFORE transforming the command (path=%q env=%v)", dummy.Path, dummy.Env)
	}

	sentinel := filepath.Join(t.TempDir(), "must-not-exist")
	cfg := Config{Sandbox: fn, SandboxName: "read-only"}
	out := runQuick(t, cfg, "touch "+sentinel, 5000)
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("command executed despite unavailable sandbox (sentinel %s exists)", sentinel)
	}
	if out.Cause != CauseError {
		t.Fatalf("cause = %q, want error", out.Cause)
	}
	if !strings.Contains(out.SpawnError, "sandbox") || !strings.Contains(out.SpawnError, "unavailable") {
		t.Fatalf("spawnError = %q, want sandbox-unavailable provenance", out.SpawnError)
	}

	// Typed propagation through execute(): errors.As must recover the
	// type (the Pipeline-normalized isError result carries this text).
	cfg2 := Config{Sandbox: fn, SandboxName: "read-only"}
	cfg2.normalize()
	_, execErr := execute(context.Background(), &cfg2, json.RawMessage(`{"command":"true"}`))
	if execErr == nil {
		t.Fatalf("execute must fail when the sandbox backend is unavailable")
	}
	var unavail *SandboxUnavailableError
	if !errors.As(execErr, &unavail) {
		t.Fatalf("execute error is %T, want *SandboxUnavailableError (typed fail-closed)", execErr)
	}
	if unavail.Mode != SandboxReadOnly {
		t.Fatalf("typed error mode = %q, want read-only", unavail.Mode)
	}
	if !IsSandboxUnavailable(execErr) {
		t.Fatalf("IsSandboxUnavailable must detect the wrapped typed error")
	}
}

// TestSandboxUnavailablePipelineIsError: through the REAL Pipeline, a
// sandbox-unavailable failure lands as an isError tool result (never a
// silently unconfined success).
func TestSandboxUnavailablePipelineIsError(t *testing.T) {
	fn, err := newSandboxFuncDetect(SandboxOptions{Mode: SandboxReadOnly}, unavailableDetect)
	if err != nil {
		t.Fatalf("constructor: %v", err)
	}
	def := Definition(Config{Sandbox: fn, SandboxName: "read-only"})
	p := tools.NewPipeline()
	if err := p.Register(def); err != nil {
		t.Fatalf("register: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res := p.Execute(ctx, session.ToolCall{ID: "call-unavail", Name: Name, Args: json.RawMessage(`{"command":"true"}`)})
	if !res.IsError {
		t.Fatalf("unavailable sandbox must be an isError result: %+v", res)
	}
	if !strings.Contains(res.Content, "unavailable") {
		t.Fatalf("result content lacks the unavailable fact: %q", res.Content)
	}
}
