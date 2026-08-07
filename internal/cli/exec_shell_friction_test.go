package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/permission"
	"github.com/vhqtvn/vh-agent-harness/internal/runtime"
)

// This file pins Fix 1 — surface-at-friction on the exec/shell deny/refuse
// families (researches/decisions/2026-08-04-capability-discovery-audit.md §1/§9).
// The signed-off message shape is: {preserve the denial} → {explain why} →
// {name the sanctioned alternative} → {point to the authority} → {never
// auto-retry}. exec-ro's denyFooter is the positive control (tested elsewhere);
// these tests cover the exec/shell gate sites in internal/cli/exec_shell.go that
// previously emitted bare "denied: <reason>" / "permission hook error: <err>".
//
// The deny message is rendered to cmd.ErrOrStderr() (captured here via
// newOutCmd). In production that resolves to os.Stderr, so these tests assert
// the real operator/agent-facing surface.

// errorHook is a Hook double whose Evaluate faults, to exercise the
// deny-by-default hook-error path and its surface-at-friction footer.
type errorHook struct{ err error }

func (h errorHook) Evaluate(context.Context, []string) (permission.Action, string, error) {
	return permission.Deny, "", h.err
}

// askHook is a Hook double that returns Ask (undecided), to exercise the
// no-operator-loop deny-by-default path and its footer.
type askHook struct{}

func (askHook) Evaluate(context.Context, []string) (permission.Action, string, error) {
	return permission.Ask, "test-ask: gate undecided", nil
}

// wantFrictionShape asserts the captured stderr carries the load-bearing
// surface-at-friction elements: the preserved denial, the gate reason, the
// never-auto-retry directive, and an authority/sanctioned-alternative pointer.
func wantFrictionShape(t *testing.T, out string, denialPrefix, reason string) {
	t.Helper()
	if !strings.Contains(out, denialPrefix) {
		t.Errorf("denial prefix %q missing from message:\n%s", denialPrefix, out)
	}
	if !strings.Contains(out, reason) {
		t.Errorf("gate reason %q must be preserved (preserve the denial):\n%s", reason, out)
	}
	// Element 5: never auto-retry — the directive not to retry/route around.
	if !strings.Contains(out, "do not retry") {
		t.Errorf("never-auto-retry directive missing from message:\n%s", out)
	}
}

// TestExec_DenyMessageSurfaceAtFriction is the crux test for Fix 1: a shell-guard
// Deny must render the full surface-at-friction shape — denial preserved, reason
// preserved, never-auto-retry directive, and a sanctioned-alternative/authority
// pointer — not the legacy bare "denied: <reason>".
func TestExec_DenyMessageSurfaceAtFriction(t *testing.T) {
	root := t.TempDir()
	writeFixtureManifest(t, root, "docker_compose", "dev")

	rec := &recordingBackend{name: "docker_compose"}
	runtimeCmdDeps.backendFor = func(*loadedManifest) (runtime.Backend, error) { return rec, nil }
	runtimeCmdDeps.hook = denyHook{}
	defer resetRuntimeDeps(t)

	execFl.service, execFl.workdir, execFl.tty = "", "", false
	runWithCwd(t, root, func() {
		cmd, buf := newOutCmd()
		err := runExec(cmd, []string{"echo", "hello"})
		if err == nil {
			t.Fatalf("runExec with deny hook: expected error, got nil")
		}
		out := buf.String()
		wantFrictionShape(t, out, "denied:", "test-deny")
		// Element 3+4: sanctioned alternative / authority pointer (shell-guard
		// rule `why` + AGENTS.md shell hygiene).
		if !strings.Contains(out, "forbidden-patterns") && !strings.Contains(out, "AGENTS.md") {
			t.Errorf("sanctioned-alternative/authority pointer missing from message:\n%s", out)
		}
		// The returned error stays a concise typed signal (cobra's "Error:" line
		// is secondary); it must still name the gate and carry the reason.
		if !strings.Contains(err.Error(), "denied by permission hook") {
			t.Errorf("error must name the permission hook; got %q", err.Error())
		}
		if !strings.Contains(err.Error(), "test-deny") {
			t.Errorf("error must carry the gate reason; got %q", err.Error())
		}
		// Backend must never be reached on a deny.
		if len(rec.log) != 0 {
			t.Errorf("backend reached despite deny: %v", rec.log)
		}
	})
}

// TestExec_HookErrorMessageSurfaceAtFriction pins the fail-closed hook-error
// path: when the permission gate itself faults, the message must explain that it
// denied by default for safety, name never-retry, and point at diagnosis.
func TestExec_HookErrorMessageSurfaceAtFriction(t *testing.T) {
	root := t.TempDir()
	writeFixtureManifest(t, root, "docker_compose", "dev")

	rec := &recordingBackend{name: "docker_compose"}
	runtimeCmdDeps.backendFor = func(*loadedManifest) (runtime.Backend, error) { return rec, nil }
	runtimeCmdDeps.hook = errorHook{err: errors.New("bridge exploded")}
	defer resetRuntimeDeps(t)

	execFl.service, execFl.workdir, execFl.tty = "", "", false
	runWithCwd(t, root, func() {
		cmd, buf := newOutCmd()
		err := runExec(cmd, []string{"echo", "hello"})
		if err == nil {
			t.Fatalf("runExec with faulting hook: expected error, got nil")
		}
		out := buf.String()
		if !strings.Contains(out, "permission hook error") {
			t.Errorf("hook-error prefix missing:\n%s", out)
		}
		if !strings.Contains(out, "bridge exploded") {
			t.Errorf("underlying hook error must be preserved:\n%s", out)
		}
		// Element 5: never-retry + element 3/4: diagnosis pointer.
		if !strings.Contains(out, "do not retry") {
			t.Errorf("never-retry directive missing:\n%s", out)
		}
		if !strings.Contains(out, "doctor") {
			t.Errorf("diagnosis pointer (doctor) missing:\n%s", out)
		}
	})
}

// TestExec_AskMessageSurfaceAtFriction pins the undecided/ask path: the message
// must state deny-by-default (no operator loop), never-auto-retry, and surface
// the decision to the operator.
func TestExec_AskMessageSurfaceAtFriction(t *testing.T) {
	root := t.TempDir()
	writeFixtureManifest(t, root, "docker_compose", "dev")

	rec := &recordingBackend{name: "docker_compose"}
	runtimeCmdDeps.backendFor = func(*loadedManifest) (runtime.Backend, error) { return rec, nil }
	runtimeCmdDeps.hook = askHook{}
	defer resetRuntimeDeps(t)

	execFl.service, execFl.workdir, execFl.tty = "", "", false
	runWithCwd(t, root, func() {
		cmd, buf := newOutCmd()
		err := runExec(cmd, []string{"echo", "hello"})
		if err == nil {
			t.Fatalf("runExec with ask hook: expected error, got nil")
		}
		out := buf.String()
		if !strings.Contains(out, "ask") {
			t.Errorf("ask state must be surfaced:\n%s", out)
		}
		if !strings.Contains(out, "do not auto-retry") {
			t.Errorf("never-auto-retry directive missing:\n%s", out)
		}
	})
}

// TestShell_DenyMessageSurfaceAtFriction mirrors the exec deny test for the
// interactive shell path (runShell), which shares the same deny/refuse family.
func TestShell_DenyMessageSurfaceAtFriction(t *testing.T) {
	root := t.TempDir()
	writeFixtureManifest(t, root, "docker_compose", "dev")

	rec := &recordingBackend{name: "docker_compose"}
	runtimeCmdDeps.backendFor = func(*loadedManifest) (runtime.Backend, error) { return rec, nil }
	runtimeCmdDeps.hook = denyHook{}
	defer resetRuntimeDeps(t)

	shellFl.service = ""
	runWithCwd(t, root, func() {
		cmd, buf := newOutCmd()
		err := runShell(cmd, nil)
		if err == nil {
			t.Fatalf("runShell with deny hook: expected error, got nil")
		}
		out := buf.String()
		wantFrictionShape(t, out, "denied:", "test-deny")
		if !strings.Contains(out, "forbidden-patterns") && !strings.Contains(out, "AGENTS.md") {
			t.Errorf("sanctioned-alternative/authority pointer missing from message:\n%s", out)
		}
		if len(rec.log) != 0 {
			t.Errorf("backend reached despite deny: %v", rec.log)
		}
	})
}

// TestShell_HookErrorMessageSurfaceAtFriction pins runShell's fail-closed
// hook-error path: the message must name the gate fault, forbid retry, and point
// at diagnosis (mirrors the exec variant; closes the shell-side coverage gap).
func TestShell_HookErrorMessageSurfaceAtFriction(t *testing.T) {
	root := t.TempDir()
	writeFixtureManifest(t, root, "docker_compose", "dev")

	rec := &recordingBackend{name: "docker_compose"}
	runtimeCmdDeps.backendFor = func(*loadedManifest) (runtime.Backend, error) { return rec, nil }
	runtimeCmdDeps.hook = errorHook{err: errors.New("bridge exploded")}
	defer resetRuntimeDeps(t)

	shellFl.service = ""
	runWithCwd(t, root, func() {
		cmd, buf := newOutCmd()
		err := runShell(cmd, nil)
		if err == nil {
			t.Fatalf("runShell with faulting hook: expected error, got nil")
		}
		out := buf.String()
		if !strings.Contains(out, "permission hook error") {
			t.Errorf("hook-error prefix missing:\n%s", out)
		}
		if !strings.Contains(out, "bridge exploded") {
			t.Errorf("underlying hook error must be preserved:\n%s", out)
		}
		if !strings.Contains(out, "do not retry") {
			t.Errorf("never-retry directive missing:\n%s", out)
		}
		if !strings.Contains(out, "doctor") {
			t.Errorf("diagnosis pointer (doctor) missing:\n%s", out)
		}
	})
}

// TestShell_AskMessageSurfaceAtFriction pins runShell's undecided/ask path: the
// message must surface the ask state and the never-auto-retry directive (mirrors
// the exec variant).
func TestShell_AskMessageSurfaceAtFriction(t *testing.T) {
	root := t.TempDir()
	writeFixtureManifest(t, root, "docker_compose", "dev")

	rec := &recordingBackend{name: "docker_compose"}
	runtimeCmdDeps.backendFor = func(*loadedManifest) (runtime.Backend, error) { return rec, nil }
	runtimeCmdDeps.hook = askHook{}
	defer resetRuntimeDeps(t)

	shellFl.service = ""
	runWithCwd(t, root, func() {
		cmd, buf := newOutCmd()
		err := runShell(cmd, nil)
		if err == nil {
			t.Fatalf("runShell with ask hook: expected error, got nil")
		}
		out := buf.String()
		if !strings.Contains(out, "ask") {
			t.Errorf("ask state must be surfaced:\n%s", out)
		}
		if !strings.Contains(out, "do not auto-retry") {
			t.Errorf("never-auto-retry directive missing:\n%s", out)
		}
	})
}
