package runtime

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestBare_Name verifies the backend identifier.
func TestBare_Name(t *testing.T) {
	b := NewBare(BareConfig{Dir: "/proj"})
	if b.Name() != "bare" {
		t.Errorf("Name = %q, want bare", b.Name())
	}
}

// TestBare_LifecycleWarns verifies Up/Down/Healthcheck print the loud
// no-isolation warning (captured via the package stderr seam) AND succeed.
func TestBare_LifecycleWarns(t *testing.T) {
	b := NewBare(BareConfig{Dir: "/proj"})
	var buf bytes.Buffer
	saved := bareStderr
	bareStderr = &buf
	defer func() { bareStderr = saved }()

	if err := b.Up(context.Background()); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if err := b.Down(context.Background()); err != nil {
		t.Fatalf("Down: %v", err)
	}
	if err := b.Healthcheck(context.Background()); err != nil {
		t.Fatalf("Healthcheck: %v", err)
	}
	out := buf.String()
	// Three warnings (Up + Down + Healthcheck), all identical.
	if got := strings.Count(out, bareNoIsolationWarning); got != 3 {
		t.Errorf("warning printed %d times, want 3 (Up+Down+Healthcheck); got:\n%s", got, out)
	}
}

// TestBare_Exec runs a command via a fake runner and confirms the runner saw
// the raw host command (no docker wrapping) and the loud warning was emitted.
func TestBare_Exec(t *testing.T) {
	fr := newFakeRunner()
	b := &Bare{Cfg: BareConfig{Dir: "/proj"}, Runner: fr}

	var buf bytes.Buffer
	saved := bareStderr
	bareStderr = &buf
	defer func() { bareStderr = saved }()

	if err := b.Exec(context.Background(), []string{"echo", "hello"}, ExecOpts{Workdir: "/srv"}); err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if len(fr.calls) != 1 {
		t.Fatalf("got %d runner calls, want 1", len(fr.calls))
	}
	c := fr.calls[0]
	// Bare runs the command DIRECTLY (name=echo, args=[hello]); no docker.
	if c.name != "echo" || len(c.args) != 1 || c.args[0] != "hello" {
		t.Errorf("bare exec call = name=%q args=%v, want echo [hello]", c.name, c.args)
	}
	if c.dir != "/srv" {
		t.Errorf("bare exec dir = %q, want /srv", c.dir)
	}
	if !strings.Contains(buf.String(), bareNoIsolationWarning) {
		t.Errorf("bare exec did not print the no-isolation warning; got:\n%s", buf.String())
	}
}

// TestBare_ExecInteractiveShell verifies empty-cmd interactive exec opens the
// host shell via the runner.
func TestBare_ExecInteractiveShell(t *testing.T) {
	// Force SHELL unset so the test is hermetic: Bare.Exec falls back to /bin/sh
	// when SHELL is empty, regardless of the ambient environment (the dev
	// container sets SHELL=/bin/zsh, which would otherwise flake this).
	t.Setenv("SHELL", "")
	fr := newFakeRunner()
	b := &Bare{Cfg: BareConfig{Dir: "/proj"}, Runner: fr}
	if err := b.Exec(context.Background(), nil, ExecOpts{Interactive: true}); err != nil {
		t.Fatalf("Exec shell: %v", err)
	}
	if len(fr.calls) != 1 {
		t.Fatalf("got %d runner calls, want 1", len(fr.calls))
	}
	c := fr.calls[0]
	// SHELL env may be unset in the test runner -> expect /bin/sh fallback.
	want := "/bin/sh"
	if c.name != want || len(c.args) != 0 {
		t.Errorf("bare shell call = name=%q args=%v, want %s []", c.name, c.args, want)
	}
	if !c.interactive {
		t.Errorf("bare shell should pass Interactive=true")
	}
}

// TestBare_ExecEmptyNonInteractive verifies an empty non-interactive exec errors.
func TestBare_ExecEmptyNonInteractive(t *testing.T) {
	b := NewBare(BareConfig{})
	if err := b.Exec(context.Background(), nil, ExecOpts{}); err == nil {
		t.Errorf("empty non-interactive exec: expected error, got nil")
	}
}

// TestBare_LogsPsUnsupported verifies Logs/Ps return a clear error (no managed
// services on bare).
func TestBare_LogsPsUnsupported(t *testing.T) {
	b := NewBare(BareConfig{})
	if _, err := b.Ps(context.Background()); err == nil || !strings.Contains(err.Error(), "no managed services") {
		t.Errorf("Ps error = %v, want 'no managed services'", err)
	}
	if err := b.Logs(context.Background(), "", false); err == nil || !strings.Contains(err.Error(), "no managed services") {
		t.Errorf("Logs error = %v, want 'no managed services'", err)
	}
}
