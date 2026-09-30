// wrap.go — the narrow PROGRAMMATIC sandbox surface: adapts the
// two-stage re-exec trampoline (runTrampoline/RunChild) to callers that
// bring their own fully-constructed *exec.Cmd (the run_shell SandboxFunc
// seam), instead of the CLI verb shape (host stdio, os.Environ, target
// argv) that execsandbox.Run carries.
package execsandbox

import (
	"fmt"
	"os"
	"os/exec"
)

// TrampolineVerb is the hidden first-argv argument that marks a process
// as a sandbox trampoline child (the parent re-execs its own executable
// with [self, TrampolineVerb, "--", target, args...]). The CLI registers
// it as a hidden cobra command; programmatic hosts (the vh-agentd
// daemon, test binaries) dispatch on it BEFORE their own argument
// parsing. It is the single definition of the verb string.
const TrampolineVerb = "__exec_sandbox_child"

// ProfileEnv serializes the profile into the VH_EXEC_SANDBOX_* env
// entries the child trampoline decodes (profileFromEnv). Exported as
// the single source of truth for parent-side serialization so
// programmatic callers share the exact CLI format and cannot drift
// from the child decoder.
func ProfileEnv(p Profile) []string {
	return profileToEnv(p)
}

// WrapCommand rewrites an ALREADY-CONFIGURED *exec.Cmd in place so the
// executed leader is this process re-invoked as the sandbox trampoline
// child: after the rewrite the command runs
//
//	[self, TrampolineVerb, "--", <cmd.Path>, <cmd.Args[1:]...>]
//
// with the profile appended to cmd.Env. The trampoline child installs
// NoNewPrivs + seccomp + landlock and syscall.Execs into the original
// target, so the confinement applies to exactly the command the caller
// built — with three caller-owned properties PRESERVED:
//
//   - streams: Stdout/Stderr/Stdin wiring is untouched (captured pipes,
//     not host stdio, keep working);
//   - environment: cmd.Env is extended in place (the profile vars are
//     appended AFTER the caller's env; the child strips them again via
//     envForTarget before execing the target, so the target sees the
//     caller's env exactly);
//   - process-group semantics: SysProcAttr is NOT modified. The caller
//     owns group leadership (e.g. run_shell's Setpgid + negative-pid
//     group teardown); the trampoline execs in-place (same pid, same
//     group), so the executed leader stays the group leader. Hosts must
//     NOT set Setsid on the wrapped command (setpgid-then-setsid fails
//     with EPERM — see runTrampoline).
//
// cmd.Path must already be resolved (absolute or PATH-found); it is
// passed through as the trampoline target verbatim. The profile is
// validated by the CHILD (profileFromEnv fail-closed), not here.
//
// FOOTGUN — nil cmd.Env: WrapCommand appends the profile vars with
// cmd.Env = append(cmd.Env, ...). If cmd.Env is nil (exec.Cmd's normal
// "inherit os.Environ()" convention), that append produces a NON-nil
// slice holding ONLY the VH_EXEC_SANDBOX_* profile vars: the
// nil-means-inherit semantics are silently lost, and the wrapped child
// runs with a stripped environment (no PATH/HOME/TERM — os.Environ()
// is NOT inherited through the wrap) rather than the caller's
// environment plus the profile. The trampoline child then strips the
// profile vars via envForTarget, so the exec'd target inherits an
// almost-empty environment. Safe pattern: ALWAYS materialize cmd.Env
// BEFORE calling WrapCommand — cmd.Env = os.Environ() for pass-through
// hosts, or an explicit scrubbed env (as internal/tools/shell's
// buildEnv does: TERM/PATH/HOME/LANG + allowlist).
func WrapCommand(cmd *exec.Cmd, profile Profile) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locating self executable for sandbox trampoline: %w", err)
	}
	newArgs := make([]string, 0, len(cmd.Args)+3)
	newArgs = append(newArgs, self, TrampolineVerb, "--", cmd.Path)
	newArgs = append(newArgs, cmd.Args[1:]...)
	cmd.Args = newArgs
	cmd.Path = self
	cmd.Env = append(cmd.Env, ProfileEnv(profile)...)
	return nil
}
