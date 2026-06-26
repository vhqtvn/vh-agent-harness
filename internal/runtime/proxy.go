package runtime

import (
	"context"
	"fmt"
	"os"
)

// proxyCapabilityNote is the one-line capability boundary the proxy backend
// prints on lifecycle verbs. The proxy backend is a deliberate MIGRATION /
// delegation backend: exec and shell are forwarded to a project-owned wrapper
// command (run-shape runtime.proxy_command, e.g. ["./dev.sh","exec"]) that
// already carries the project's domain knowledge (which container, env, GPU
// flags, …). The shell-guard permission gate STILL runs in the cli layer before
// the delegate is invoked, so the harness remains the single gated front door.
const proxyCapabilityNote = "proxy backend: exec/shell delegate to the project wrapper command (run-shape runtime.proxy_command); the permission gate runs first."

// ProxyConfig is the run-shape-resolved configuration for the proxy backend.
type ProxyConfig struct {
	// Dir is the working directory the wrapper command runs in (project root).
	Dir string
	// Command is the argv prefix exec/shell delegate to. The user command is
	// appended to it, e.g. Command=["./dev.sh","exec"] + ["echo","hi"] runs
	// `./dev.sh exec echo hi`. Must be non-empty (selectBackend enforces this).
	Command []string
}

// Proxy implements Backend by forwarding exec/shell to a project-owned wrapper
// command instead of running them itself. It is the answer to "my wrapper
// script (./dev.sh) holds too much domain knowledge to re-encode declaratively":
// the project keeps its wrapper, and the harness becomes the gated entrypoint
// that delegates to it.
//
// Capability surface (v0):
//   - Exec / shell: delegate to Command (+ the user's argv). The real value.
//   - Up / Down: no-op success with guidance (the wrapper owns service lifecycle;
//     run it directly, e.g. `./dev.sh up`).
//   - Logs / Ps: typed-unsupported (the wrapper, not the harness, observes
//     services). A future richer proxy could forward these too.
//   - Healthcheck: success (the wrapper is the runtime authority).
type Proxy struct {
	Cfg    ProxyConfig
	Runner Runner
}

// NewProxy builds a proxy backend with the production OS runner.
func NewProxy(cfg ProxyConfig) *Proxy {
	return &Proxy{Cfg: cfg, Runner: NewOSRunner()}
}

// Name returns the stable backend identifier.
func (p *Proxy) Name() string { return "proxy" }

// Capabilities returns the proxy capability matrix (exec/shell/hook Supported;
// up/down Noop; logs/ps Unsupported).
func (p *Proxy) Capabilities() CapabilityMatrix { return proxyMatrix() }

func (p *Proxy) note() { fmt.Fprintln(os.Stderr, proxyCapabilityNote) }

// Up is a no-op success: the wrapper command owns service lifecycle.
func (p *Proxy) Up(context.Context) error {
	p.note()
	fmt.Fprintln(os.Stdout, "proxy backend: service lifecycle is owned by the project wrapper; run it directly (e.g. `./dev.sh up`)")
	return nil
}

// Down is a no-op success (symmetric with Up).
func (p *Proxy) Down(context.Context) error {
	p.note()
	fmt.Fprintln(os.Stdout, "proxy backend: service lifecycle is owned by the project wrapper; run it directly (e.g. `./dev.sh down`)")
	return nil
}

// Exec forwards cmd to the configured wrapper command: it runs
// `Command... cmd...` on the host. With an empty cmd and Interactive set (the
// `vh-agent-harness shell` path), it appends $SHELL so the wrapper opens an interactive
// shell (e.g. `./dev.sh exec /bin/bash`). opts.Service is not used by proxy;
// service selection, if any, lives inside the wrapper.
func (p *Proxy) Exec(ctx context.Context, cmd []string, opts ExecOpts) error {
	if len(p.Cfg.Command) == 0 {
		return fmt.Errorf("proxy backend: runtime.proxy_command is empty; set it in .vh-agent-harness/run-shape.yml (e.g. [\"./dev.sh\", \"exec\"])")
	}
	argv := append([]string(nil), p.Cfg.Command...)
	if len(cmd) == 0 {
		if !opts.Interactive {
			return fmt.Errorf("proxy exec requires a command to run (use `vh-agent-harness shell` for an interactive shell)")
		}
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/sh"
		}
		argv = append(argv, shell)
	} else {
		argv = append(argv, cmd...)
	}
	ro := RunOpts{Interactive: opts.Interactive, Dir: opts.Workdir}
	if ro.Dir == "" {
		ro.Dir = p.Cfg.Dir
	}
	return p.Runner.Run(ctx, argv[0], argv[1:], ro)
}

// Healthcheck succeeds: the project wrapper is the runtime authority and the
// proxy does not second-guess it.
func (p *Proxy) Healthcheck(context.Context) error {
	fmt.Fprintf(os.Stdout, "proxy backend: delegating to %v\n", p.Cfg.Command)
	return nil
}

// Logs is typed-unsupported: the wrapper, not the harness, observes services.
func (p *Proxy) Logs(context.Context, string, bool) error {
	return proxyMatrix().UnsupportedError(VerbLogs)
}

// Ps is typed-unsupported (symmetric with Logs).
func (p *Proxy) Ps(context.Context) ([]ServiceStatus, error) {
	return nil, proxyMatrix().UnsupportedError(VerbPs)
}
