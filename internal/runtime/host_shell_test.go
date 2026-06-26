package runtime

import (
	"context"
	"strings"
	"testing"
)

// TestHostShell_Name verifies the backend identifier matches the manifest enum.
func TestHostShell_Name(t *testing.T) {
	h := NewHostShell(HostShellConfig{Dir: "/proj"})
	if h.Name() != "host-shell" {
		t.Errorf("Name = %q, want \"host-shell\"", h.Name())
	}
}

// TestHostShell_LifecycleNoOps verifies Up/Down/Healthcheck are no-op success
// (capability-scoped: host-shell manages no services). They must NOT error.
func TestHostShell_LifecycleNoOps(t *testing.T) {
	h := NewHostShell(HostShellConfig{Dir: "/proj"})
	if err := h.Up(context.Background()); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if err := h.Down(context.Background()); err != nil {
		t.Fatalf("Down: %v", err)
	}
	if err := h.Healthcheck(context.Background()); err != nil {
		t.Fatalf("Healthcheck: %v", err)
	}
}

// TestHostShell_Exec runs a command via a fake runner and confirms the runner
// saw the RAW host command (no docker/compose wrapping) — this is the core
// mechanism: host-shell exec is plain host exec.
func TestHostShell_Exec(t *testing.T) {
	fr := newFakeRunner()
	h := &HostShell{Cfg: HostShellConfig{Dir: "/proj"}, Runner: fr}

	if err := h.Exec(context.Background(), []string{"echo", "ok"}, ExecOpts{Workdir: "/srv"}); err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if len(fr.calls) != 1 {
		t.Fatalf("got %d runner calls, want 1", len(fr.calls))
	}
	c := fr.calls[0]
	// host-shell runs the command DIRECTLY (name=echo, args=[ok]); no docker.
	if c.name != "echo" || len(c.args) != 1 || c.args[0] != "ok" {
		t.Errorf("host-shell exec call = name=%q args=%v, want echo [ok]", c.name, c.args)
	}
	if c.dir != "/srv" {
		t.Errorf("host-shell exec dir = %q, want /srv", c.dir)
	}
}

// TestHostShell_ExecUsesCfgDirWhenNoWorkdir verifies an unset Workdir falls back
// to the backend's configured project dir.
func TestHostShell_ExecUsesCfgDirWhenNoWorkdir(t *testing.T) {
	fr := newFakeRunner()
	h := &HostShell{Cfg: HostShellConfig{Dir: "/proj"}, Runner: fr}
	if err := h.Exec(context.Background(), []string{"pwd"}, ExecOpts{}); err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if fr.calls[0].dir != "/proj" {
		t.Errorf("exec dir = %q, want /proj (cfg default)", fr.calls[0].dir)
	}
}

// TestHostShell_ExecInteractiveShell verifies empty-cmd interactive exec opens
// the host shell via the runner.
func TestHostShell_ExecInteractiveShell(t *testing.T) {
	// Force SHELL unset so the test is hermetic: HostShell.Exec falls back to
	// /bin/sh when SHELL is empty, regardless of the ambient environment (the
	// dev container sets SHELL=/bin/zsh, which would otherwise flake this).
	t.Setenv("SHELL", "")
	fr := newFakeRunner()
	h := &HostShell{Cfg: HostShellConfig{Dir: "/proj"}, Runner: fr}
	if err := h.Exec(context.Background(), nil, ExecOpts{Interactive: true}); err != nil {
		t.Fatalf("Exec shell: %v", err)
	}
	if len(fr.calls) != 1 {
		t.Fatalf("got %d runner calls, want 1", len(fr.calls))
	}
	c := fr.calls[0]
	// SHELL env may be unset in the test runner -> expect /bin/sh fallback.
	want := "/bin/sh"
	if c.name != want || len(c.args) != 0 {
		t.Errorf("host-shell shell call = name=%q args=%v, want %s []", c.name, c.args, want)
	}
	if !c.interactive {
		t.Errorf("host-shell shell should pass Interactive=true")
	}
}

// TestHostShell_ExecEmptyNonInteractive verifies an empty non-interactive exec
// errors with guidance (use `vh-agent-harness shell`).
func TestHostShell_ExecEmptyNonInteractive(t *testing.T) {
	h := NewHostShell(HostShellConfig{})
	if err := h.Exec(context.Background(), nil, ExecOpts{}); err == nil {
		t.Errorf("empty non-interactive exec: expected error, got nil")
	}
}

// TestHostShell_LogsPsTypedUnsupported verifies Logs/Ps return a typed-unsupported
// guidance error (capability-scoped: host-shell has no service observability).
// This must be a real error, never silent success or accidental docker fallback.
func TestHostShell_LogsPsTypedUnsupported(t *testing.T) {
	h := NewHostShell(HostShellConfig{})
	if _, err := h.Ps(context.Background()); err == nil || !strings.Contains(err.Error(), "does not manage services") {
		t.Errorf("Ps error = %v, want 'does not manage services'", err)
	}
	if err := h.Logs(context.Background(), "", false); err == nil || !strings.Contains(err.Error(), "does not manage services") {
		t.Errorf("Logs error = %v, want 'does not manage services'", err)
	}
}
