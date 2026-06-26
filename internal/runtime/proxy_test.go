package runtime

import (
	"context"
	"strings"
	"testing"
)

// TestProxy_Name verifies the backend identifier.
func TestProxy_Name(t *testing.T) {
	p := NewProxy(ProxyConfig{Command: []string{"./dev.sh", "exec"}})
	if p.Name() != "proxy" {
		t.Errorf("Name = %q, want \"proxy\"", p.Name())
	}
}

// TestProxy_ExecDelegatesToWrapper is the core mechanism: exec runs
// proxy_command + the user's argv, e.g. ["./dev.sh","exec"] + ["echo","ok"]
// => `./dev.sh exec echo ok`. This is how acme's domain knowledge in
// ./dev.sh is preserved while vh-agent-harness stays the gated front door.
func TestProxy_ExecDelegatesToWrapper(t *testing.T) {
	fr := newFakeRunner()
	p := &Proxy{Cfg: ProxyConfig{Dir: "/proj", Command: []string{"./dev.sh", "exec"}}, Runner: fr}

	if err := p.Exec(context.Background(), []string{"echo", "ok"}, ExecOpts{}); err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if len(fr.calls) != 1 {
		t.Fatalf("got %d runner calls, want 1", len(fr.calls))
	}
	c := fr.calls[0]
	if c.name != "./dev.sh" {
		t.Errorf("proxy exec name = %q, want ./dev.sh", c.name)
	}
	if got := strings.Join(c.args, " "); got != "exec echo ok" {
		t.Errorf("proxy exec args = %q, want \"exec echo ok\"", got)
	}
	if c.dir != "/proj" {
		t.Errorf("proxy exec dir = %q, want /proj (cfg default)", c.dir)
	}
}

// TestProxy_ShellAppendsShell verifies the `vh-agent-harness shell` path (empty cmd +
// interactive) appends $SHELL so the wrapper opens an interactive shell.
func TestProxy_ShellAppendsShell(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")
	fr := newFakeRunner()
	p := &Proxy{Cfg: ProxyConfig{Dir: "/proj", Command: []string{"./dev.sh", "exec"}}, Runner: fr}

	if err := p.Exec(context.Background(), nil, ExecOpts{Interactive: true}); err != nil {
		t.Fatalf("Exec(shell): %v", err)
	}
	if got := strings.Join(fr.lastArgs(), " "); got != "exec /bin/zsh" {
		t.Errorf("proxy shell args = %q, want \"exec /bin/zsh\"", got)
	}
}

// TestProxy_ExecEmptyNonInteractiveErrors verifies a bare exec with no command
// is rejected (mirrors host-shell).
func TestProxy_ExecEmptyNonInteractiveErrors(t *testing.T) {
	p := &Proxy{Cfg: ProxyConfig{Command: []string{"./dev.sh", "exec"}}, Runner: newFakeRunner()}
	if err := p.Exec(context.Background(), nil, ExecOpts{}); err == nil {
		t.Fatal("expected error for empty non-interactive exec, got nil")
	}
}

// TestProxy_ExecEmptyCommandErrors verifies a misconfigured proxy (no
// proxy_command) fails with guidance rather than running nothing.
func TestProxy_ExecEmptyCommandErrors(t *testing.T) {
	p := &Proxy{Cfg: ProxyConfig{Command: nil}, Runner: newFakeRunner()}
	err := p.Exec(context.Background(), []string{"echo", "ok"}, ExecOpts{})
	if err == nil || !strings.Contains(err.Error(), "proxy_command") {
		t.Fatalf("want proxy_command guidance error, got %v", err)
	}
}

// TestProxy_LogsPsUnsupported verifies service-observability verbs are typed
// unsupported (the wrapper owns services, not the harness).
func TestProxy_LogsPsUnsupported(t *testing.T) {
	p := NewProxy(ProxyConfig{Command: []string{"./dev.sh", "exec"}})
	if err := p.Logs(context.Background(), "", false); err == nil {
		t.Error("Logs: want unsupported error, got nil")
	}
	if _, err := p.Ps(context.Background()); err == nil {
		t.Error("Ps: want unsupported error, got nil")
	}
}

// TestProxy_LifecycleNoOps verifies up/down/healthcheck are no-op success.
func TestProxy_LifecycleNoOps(t *testing.T) {
	p := NewProxy(ProxyConfig{Command: []string{"./dev.sh", "exec"}})
	for _, fn := range []func(context.Context) error{p.Up, p.Down, p.Healthcheck} {
		if err := fn(context.Background()); err != nil {
			t.Errorf("lifecycle no-op errored: %v", err)
		}
	}
}
