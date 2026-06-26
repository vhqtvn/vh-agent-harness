package runtime

import (
	"context"
	"fmt"
	"io"
	"os"
)

// bareNoIsolationWarning is the loud stderr warning the bare backend prints on
// every lifecycle/exec verb. It makes the safety gap explicit rather than
// silent: bare provides NO isolation, and the shell-guard permission gate still
// applies (slice 4b wires the real bridge).
const bareNoIsolationWarning = "WARNING: bare backend provides NO isolation; shell-guard permissions still apply (slice 4b)."

// bareStderr is the sink bare writes its warnings to. It defaults to os.Stderr;
// tests swap it to a buffer to assert the warning text. Swapping is test-only
// and does not change the Backend API.
var bareStderr io.Writer = os.Stderr

// BareConfig is the plain, manifest-resolved configuration for the bare
// backend. It carries only a working directory (the project root).
type BareConfig struct {
	// Dir is the working directory for host exec. Empty inherits the process cwd.
	Dir string
}

// Bare implements Backend by running commands directly on the host via os/exec
// with no isolation. It is a real fallback path (manifest.runtime.backend=bare)
// but must NEVER be silently substituted for docker_compose.
type Bare struct {
	Cfg    BareConfig
	Runner Runner
}

// NewBare builds a bare backend with the production OS runner.
func NewBare(cfg BareConfig) *Bare {
	return &Bare{Cfg: cfg, Runner: NewOSRunner()}
}

// Name returns the stable backend identifier.
func (b *Bare) Name() string { return "bare" }

// Capabilities returns bare's honest matrix (exec/shell/hook Supported with a
// no-isolation note; up/down Noop; logs/ps Unsupported). Unsupported methods
// (Logs/Ps below) derive their typed error from this same matrix.
func (b *Bare) Capabilities() CapabilityMatrix { return bareMatrix() }

// warn prints the no-isolation warning to stderr. Centralized so every relevant
// verb emits the exact same loud message.
func (b *Bare) warn() {
	fmt.Fprintln(bareStderr, bareNoIsolationWarning)
}

// Up is a no-op success on bare (there is nothing to bring up). It still prints
// the no-isolation warning so operators know which backend ran.
func (b *Bare) Up(context.Context) error {
	b.warn()
	fmt.Fprintln(os.Stdout, "bare backend: nothing to start (commands run directly on the host)")
	return nil
}

// Down is a no-op success on bare.
func (b *Bare) Down(context.Context) error {
	b.warn()
	fmt.Fprintln(os.Stdout, "bare backend: nothing to stop (commands run directly on the host)")
	return nil
}

// Exec runs cmd directly on the host. opts.Service is ignored (no isolation).
// opts.Interactive attaches host stdin; opts.Workdir sets the child cwd.
//
// When cmd is empty and opts.Interactive is true, the host shell ($SHELL,
// defaulting to /bin/sh) is opened — this is the `vh-agent-harness shell` path on bare.
func (b *Bare) Exec(ctx context.Context, cmd []string, opts ExecOpts) error {
	b.warn()
	if len(cmd) == 0 {
		if !opts.Interactive {
			return fmt.Errorf("bare exec requires a command to run (use `vh-agent-harness shell` for an interactive shell)")
		}
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/sh"
		}
		cmd = []string{shell}
	}
	ro := RunOpts{Interactive: opts.Interactive, Dir: opts.Workdir}
	if ro.Dir == "" {
		ro.Dir = b.Cfg.Dir
	}
	return b.Runner.Run(ctx, cmd[0], cmd[1:], ro)
}

// Healthcheck always succeeds on bare (the host is the runtime). It prints the
// no-isolation warning so the operator is reminded that "healthy" here means
// "no isolation is configured", not "a sandbox is up".
func (b *Bare) Healthcheck(context.Context) error {
	b.warn()
	fmt.Fprintln(os.Stdout, "bare backend: host exec available (no isolation)")
	return nil
}

// Logs is typed-unsupported on bare (there are no managed services). It returns
// a typed *UnsupportedVerbError DERIVED from the capability matrix (backend +
// verb + guidance) — never an opaque error string, never a silent docker
// fallback. Guidance points the operator at docker_compose for service logs.
func (b *Bare) Logs(context.Context, string, bool) error {
	return bareMatrix().UnsupportedError(VerbLogs)
}

// Ps is typed-unsupported on bare (there are no managed services). Returns the
// typed *UnsupportedVerbError derived from the capability matrix.
func (b *Bare) Ps(context.Context) ([]ServiceStatus, error) {
	return nil, bareMatrix().UnsupportedError(VerbPs)
}
