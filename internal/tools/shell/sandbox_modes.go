// sandbox_modes.go — the model-facing confinement surface behind the
// SandboxFunc seam: a closed three-mode vocabulary (off | read-only |
// workspace-write), the SandboxOptions DTO, a typed fail-closed
// sandbox-unavailable error, and the constructor that adapts the
// options onto the repo's REAL kernel confinement backend
// (internal/execsandbox: Landlock filesystem integrity + pure-Go
// seccomp-BPF network/syscall hardening, via the two-stage re-exec
// trampoline).
//
// Vocabulary is borrowed from the dsh execution-safety source packet
// (read-only / workspace-write / writableRoots; "danger-full-access"
// is REFUSED as redundant with off — the loud nil default IS the
// no-confinement posture). Semantics are the execsandbox honesty
// contract: this is an INTEGRITY + NETWORK boundary (writes and
// networking outside the contract are impossible), NOT a
// confidentiality boundary (denied paths remain readable-visible).
package shell

import (
	"errors"
	"fmt"
	"os/exec"

	"github.com/vhqtvn/vh-agent-harness/internal/execsandbox"
)

// SandboxMode is the closed confinement vocabulary for run_shell.
type SandboxMode string

const (
	// SandboxOff is the ABSENCE of confinement and is expressed by
	// leaving Config.Sandbox nil — never by a SandboxFunc. It exists as
	// a value only so the daemon flag surface and ParseSandboxMode can
	// name the default posture.
	SandboxOff SandboxMode = "off"
	// SandboxReadOnly confines the command to read-only filesystem
	// access: the whole filesystem is readable (Landlock RO on /), no
	// path is writable (except /dev/null, which discards writes), and
	// network syscalls are denied.
	SandboxReadOnly SandboxMode = "read-only"
	// SandboxWorkspaceWrite is read-only plus an explicit set of
	// writable roots (SandboxOptions.WritableRoots): reads everywhere,
	// writes ONLY under the configured roots, network denied.
	SandboxWorkspaceWrite SandboxMode = "workspace-write"
)

// ParseSandboxMode decodes the model/operator-facing mode string. It
// refuses the dsh escalation-ladder top ("danger-full-access") as
// REDUNDANT with off, and refuses the CLI exec-sandbox modes
// (best-effort/strict) as verb-shaped strictness knobs that have no
// per-tool meaning: the tool surface is fail-closed — a requested
// confinement either applies (kernel-enforced) or the call fails with
// a typed error; there is no degraded "best-effort" run_shell.
func ParseSandboxMode(s string) (SandboxMode, error) {
	switch s {
	case "off":
		return SandboxOff, nil
	case "read-only":
		return SandboxReadOnly, nil
	case "workspace-write":
		return SandboxWorkspaceWrite, nil
	case "danger-full-access":
		return "", fmt.Errorf("invalid sandbox mode %q: danger-full-access is redundant with off — off IS the no-confinement posture (leave the sandbox unconfigured)", s)
	case "best-effort", "strict":
		return "", fmt.Errorf("invalid sandbox mode %q: best-effort/strict are the CLI exec-sandbox strictness modes, not run_shell modes (run_shell confinement is fail-closed: it applies or the call errors)", s)
	default:
		return "", fmt.Errorf("invalid sandbox mode %q (use off|read-only|workspace-write)", s)
	}
}

// SandboxOptions is the DTO the daemon (or any host) resolves into a
// confining SandboxFunc.
type SandboxOptions struct {
	// Mode is the confinement level; off is invalid here (off callers
	// leave Config.Sandbox nil).
	Mode SandboxMode
	// WritableRoots are the directories writable under
	// SandboxWorkspaceWrite (ignored for read-only). The daemon default
	// is the session dir plus the OS temp dir.
	WritableRoots []string
}

// SandboxUnavailableError is the TYPED fail-closed error returned when
// a sandboxed run_shell cannot obtain the kernel confinement backend
// (non-Linux platform, or a kernel without landlock+seccomp). The
// command NEVER executes unconfined: the error surfaces as a tool
// isError result and the operator must either fix the platform or
// explicitly choose mode off.
type SandboxUnavailableError struct {
	Mode   SandboxMode
	Reason string
}

func (e *SandboxUnavailableError) Error() string {
	return fmt.Sprintf("run_shell: sandbox mode %q requested but the confinement backend is unavailable (%s); refusing to run unconfined — start the daemon with --sandbox off to explicitly accept no confinement", e.Mode, e.Reason)
}

// IsSandboxUnavailable reports whether err is (or wraps) a
// *SandboxUnavailableError.
func IsSandboxUnavailable(err error) bool {
	var unavail *SandboxUnavailableError
	return errors.As(err, &unavail)
}

// NewSandboxFunc adapts opts onto the real kernel confinement backend
// and returns the SandboxFunc for Config.Sandbox. The mode must be a
// CONFINING mode (read-only or workspace-write): off is refused — off
// is expressed by leaving Config.Sandbox nil so the Outcome vocabulary
// ("none") and the pre-slice behavior stay byte-identical.
//
// The returned func fail-closes per call: if the OS sandbox primitives
// are unavailable it returns *SandboxUnavailableError (classified
// CauseError; the command never executes), never a silently unconfined
// run. Denials that happen at RUN time (a confined write hitting
// EACCES) surface as ordinary non-zero exits with the kernel
// diagnostic on stderr — the orthogonal outcome facts stay intact.
func NewSandboxFunc(opts SandboxOptions) (SandboxFunc, error) {
	return newSandboxFuncDetect(opts, execsandbox.Detect)
}

// newSandboxFuncDetect is the injectable constructor (tests simulate an
// unavailable backend without depending on the host kernel).
func newSandboxFuncDetect(opts SandboxOptions, detect func() execsandbox.Features) (SandboxFunc, error) {
	switch opts.Mode {
	case SandboxReadOnly, SandboxWorkspaceWrite:
	case SandboxOff:
		return nil, fmt.Errorf("NewSandboxFunc: mode off is the absence of confinement — leave Config.Sandbox nil instead (the loud documented default)")
	default:
		return nil, fmt.Errorf("NewSandboxFunc: invalid sandbox mode %q (use read-only or workspace-write)", opts.Mode)
	}
	return func(cmd *exec.Cmd) error {
		features := detect()
		if !features.Available() {
			return &SandboxUnavailableError{
				Mode:   opts.Mode,
				Reason: fmt.Sprintf("landlock=%v seccomp=%v", features.Landlock, features.Seccomp),
			}
		}
		profile := execsandbox.Profile{
			// Whole-filesystem read: confinement is an integrity boundary
			// ("cannot WRITE or NETWORK outside the contract"), not a
			// path-hiding boundary. Matches the dsh landlock profile
			// shape (ro:[/], rw:[/dev/null, writableRoots]).
			RODirs: []string{"/"},
			RWDirs: append([]string(nil), opts.WritableRoots...),
			// /dev/null is needed by virtually every shell redirection
			// (the kernel discards the writes).
			RWFiles: []string{"/dev/null"},
			// Network is DENIED under confinement (seccomp). A confined
			// agent shell must not phone home; operators who need
			// network run with mode off (explicit accepted risk).
			Net: execsandbox.NetDeny,
		}
		return execsandbox.WrapCommand(cmd, profile)
	}, nil
}
