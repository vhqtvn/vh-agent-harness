package runtime

import (
	"context"
	"fmt"
	"os"
)

// hostShellCapabilityNote is the concise one-line capability boundary the
// host-shell backend prints on lifecycle verbs. Unlike bare's loud
// "no isolation, I'm a fallback" warning, host-shell is a FIRST-CLASS backend
// chosen deliberately by the project (run-shape.backend: host-shell). Its
// capability surface is bounded: exec/shell run on the host, up/down are
// no-ops (no services to manage), logs/ps are typed-unsupported. The shell-guard
// permission gate STILL runs before any host exec (host-shell is MORE
// gate-critical, not less).
const hostShellCapabilityNote = "host-shell backend: commands run directly on the host (capability-scoped: exec/shell only; no service lifecycle)."

// HostShellConfig is the manifest-resolved configuration for the host-shell
// backend. It carries only a working directory (the project root).
type HostShellConfig struct {
	// Dir is the working directory for host exec. Empty inherits the process cwd.
	Dir string
}

// HostShell implements Backend by running commands directly on the host via the
// Runner. It is a FIRST-CLASS, capability-scoped backend (the D1-C verdict): a
// typed peer of docker_compose and proxy in the backend enum, deliberately
// chosen for web-less / docker-less repos.
//
// Capability surface (v0):
//   - Exec / shell: WORK on the host (the real value of this backend).
//   - Up / Down: no-op success (host-shell manages no services). They print
//     guidance so `vh-agent-harness up` is harmless instead of erroring.
//   - Logs / Ps: typed-unsupported. They return a guidance error rather than
//     pretending to tail/list anything. A future hook or proxy backend can
//     supply service observability; host-shell does not invent it.
//   - Healthcheck: success (the host is the runtime).
//
// Host-shell is NOT a fallback. It never silently substitutes for
// docker_compose, and docker_compose never silently degrades to host-shell.
type HostShell struct {
	Cfg    HostShellConfig
	Runner Runner
}

// NewHostShell builds a host-shell backend with the production OS runner.
func NewHostShell(cfg HostShellConfig) *HostShell {
	return &HostShell{Cfg: cfg, Runner: NewOSRunner()}
}

// Name returns the stable backend identifier (matches manifest.Runtime.Backend).
func (h *HostShell) Name() string { return "host-shell" }

// Capabilities returns host-shell's first-class D1-C capability matrix
// (exec/shell/hook Supported; up/down Noop; logs/ps Unsupported). Unsupported
// methods (Logs/Ps below) derive their typed error from this same matrix, so
// the declared surface and runtime behavior share one source.
func (h *HostShell) Capabilities() CapabilityMatrix { return hostShellMatrix() }

// note prints the concise capability boundary. Centralized so every relevant
// verb emits the same single-line message.
func (h *HostShell) note() {
	fmt.Fprintln(os.Stderr, hostShellCapabilityNote)
}

// Up is a no-op success on host-shell (there are no services to bring up). It
// prints guidance so the operator knows the capability boundary rather than
// seeing silent success.
func (h *HostShell) Up(context.Context) error {
	h.note()
	fmt.Fprintln(os.Stdout, "host-shell backend: nothing to start (no services; commands run directly on the host)")
	return nil
}

// Down is a no-op success on host-shell (there are no services to tear down).
func (h *HostShell) Down(context.Context) error {
	h.note()
	fmt.Fprintln(os.Stdout, "host-shell backend: nothing to stop (no services; commands run directly on the host)")
	return nil
}

// Exec runs cmd directly on the host. opts.Service is ignored (no isolation).
// opts.Interactive attaches host stdin; opts.Workdir sets the child cwd.
//
// When cmd is empty and opts.Interactive is true, the host shell ($SHELL,
// defaulting to /bin/sh) is opened — this is the `vh-agent-harness shell` path on
// host-shell.
func (h *HostShell) Exec(ctx context.Context, cmd []string, opts ExecOpts) error {
	if len(cmd) == 0 {
		if !opts.Interactive {
			return fmt.Errorf("host-shell exec requires a command to run (use `vh-agent-harness shell` for an interactive shell)")
		}
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/sh"
		}
		cmd = []string{shell}
	}
	ro := RunOpts{Interactive: opts.Interactive, Dir: opts.Workdir}
	if ro.Dir == "" {
		ro.Dir = h.Cfg.Dir
	}
	return h.Runner.Run(ctx, cmd[0], cmd[1:], ro)
}

// Healthcheck always succeeds on host-shell (the host is the runtime).
func (h *HostShell) Healthcheck(context.Context) error {
	fmt.Fprintln(os.Stdout, "host-shell backend: host exec available")
	return nil
}

// Logs is typed-unsupported on host-shell. host-shell manages no services, so
// there is nothing to tail. It returns a typed *UnsupportedVerbError DERIVED
// from the capability matrix (backend + verb + guidance) — never a silent
// fallback to docker_compose, never an opaque error string. Guidance points the
// operator at declaring a hook or using a service-capable backend.
func (h *HostShell) Logs(context.Context, string, bool) error {
	return hostShellMatrix().UnsupportedError(VerbLogs)
}

// Ps is typed-unsupported on host-shell. host-shell manages no services. Returns
// the typed *UnsupportedVerbError derived from the capability matrix.
func (h *HostShell) Ps(context.Context) ([]ServiceStatus, error) {
	return nil, hostShellMatrix().UnsupportedError(VerbPs)
}
