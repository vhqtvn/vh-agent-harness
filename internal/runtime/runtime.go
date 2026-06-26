package runtime

import (
	"context"
	"io"
	"os"
	"os/exec"
)

// Backend is the runtime-agnostic lifecycle + execution surface. The CLI verbs
// (up/down/exec/shell/logs/ps) operate against this interface; the concrete
// docker_compose, host-shell, and bare backends implement it.
//
// Implementations must be safe to construct but need NOT be safe for concurrent
// use — the harness CLI drives one verb per process.
//
// Slice 2 (D1-C): every backend also exposes a first-class Capabilities()
// matrix declaring, per verb, whether it is Supported / Noop / Unsupported.
// Methods whose verb is Unsupported DERIVE their typed guidance error from that
// matrix (see capability.go), so the declared surface and the runtime behavior
// cannot drift. No backend silently substitutes for another.
type Backend interface {
	// Name is the stable backend identifier (e.g. "docker_compose", "host-shell",
	// "bare"). It matches manifest.Runtime.Backend.
	Name() string

	// Capabilities returns this backend's first-class, introspectable
	// capability matrix (verb → Supported/Noop/Unsupported + guidance). It is
	// the single source of truth for what the backend can do; Unsupported
	// methods derive their typed error from it. See capability.go.
	Capabilities() CapabilityMatrix

	// Up brings the runtime into a ready state. For docker_compose this is
	// `docker compose up -d`; for bare/host-shell it is a no-op with guidance.
	Up(ctx context.Context) error

	// Down tears the runtime down. For docker_compose this is
	// `docker compose down`; for bare it is a no-op with a warning.
	Down(ctx context.Context) error

	// Exec runs cmd inside the runtime. For docker_compose this is
	// `docker compose exec ...`; for bare it is a direct host exec. opts
	// carries TTY/interactive intent, the target service, and an optional
	// working directory.
	Exec(ctx context.Context, cmd []string, opts ExecOpts) error

	// Logs tails runtime logs. When service is empty, all services are tailed.
	// follow controls streaming vs. snapshot behavior.
	Logs(ctx context.Context, service string, follow bool) error

	// Ps lists runtime services and their status.
	Ps(ctx context.Context) ([]ServiceStatus, error)

	// Healthcheck verifies the backend can actually be driven. For
	// docker_compose this probes `docker compose version` + `docker info`
	// (daemon reachability). It is used as the fail_with_guidance preflight for
	// lifecycle verbs so an unreachable daemon never silently degrades to bare.
	Healthcheck(ctx context.Context) error
}

// ExecOpts carries per-exec invocation intent. It is backend-agnostic; backends
// translate the relevant fields into their own argv.
type ExecOpts struct {
	// Interactive signals TTY intent. For docker_compose, true leaves the
	// default TTY allocation ON (host stdio passed through); false forces -T
	// (no TTY, clean piped output). For bare, it attaches host stdin when true.
	Interactive bool

	// Service names the container/service to exec into (docker_compose). When
	// empty, the backend's configured DefaultService is used; if that is also
	// empty, docker_compose errors with guidance.
	Service string

	// Workdir is an optional working directory inside the runtime
	// (docker_compose -w / bare cmd.Dir). Empty leaves the backend default.
	Workdir string
}

// ServiceStatus is one row of `Ps` output, normalized across backends.
type ServiceStatus struct {
	// Name is the service name (docker_compose service, or "host" for bare).
	Name string
	// State is a normalized lifecycle state ("running", "exited", "unknown").
	State string
	// Status is the backend-native status string (e.g. docker's "Up 2 minutes").
	Status string
}

// Runner abstracts process execution so command-construction (argv building) is
// unit-testable without a real docker daemon. The default runner
// (NewOSRunner()) wraps exec.CommandContext.
//
// Run executes `name args...` with the given options. It must wire the child's
// stdout/stderr/stdin exactly as opts specifies, run the process to completion,
// and return any non-zero exit as an error (typically a *exec.ExitError).
type Runner interface {
	Run(ctx context.Context, name string, args []string, opts RunOpts) error
}

// RunOpts controls stdio + working directory for a Runner.Run call.
type RunOpts struct {
	// Interactive, when true, attaches the host's stdin to the child (so an
	// interactive shell or TTY-bound docker exec receives keyboard input).
	// Stdout/Stderr are still honored when set; when nil they default to the
	// host's os.Stdout/os.Stderr so streaming output is visible by default.
	Interactive bool

	// Stdout/Stderr/Stdin override the child streams. When nil, the runner uses
	// the host os.Stdout/os.Stderr/os.Stdin (or os.Stdin only when Interactive).
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader

	// Dir is the child working directory. Empty inherits the caller's cwd.
	Dir string
}

// osRunner is the default Runner backed by exec.CommandContext.
type osRunner struct{}

// NewOSRunner returns the production Runner backed by exec.CommandContext.
func NewOSRunner() Runner { return osRunner{} }

// Run builds and runs the command, wiring stdio per opts. A non-zero exit code
// is returned as a *exec.ExitError from the os/exec package.
func (osRunner) Run(ctx context.Context, name string, args []string, opts RunOpts) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}
	cmd.Stdout = opts.Stdout
	if cmd.Stdout == nil {
		cmd.Stdout = os.Stdout
	}
	cmd.Stderr = opts.Stderr
	if cmd.Stderr == nil {
		cmd.Stderr = os.Stderr
	}
	if opts.Interactive {
		cmd.Stdin = opts.Stdin
		if cmd.Stdin == nil {
			cmd.Stdin = os.Stdin
		}
	}
	return cmd.Run()
}
