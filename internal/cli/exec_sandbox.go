package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/vhqtvn/vh-agent-harness/internal/execsandbox"
	"github.com/vhqtvn/vh-agent-harness/internal/runshape"
)

var (
	execSandboxMode    string
	execSandboxNet     string
	execSandboxCWD     string
	execSandboxProfile string
)

// execSandboxCmd implements `vh-agent-harness exec-sandbox <cmd>`.
//
// exec-sandbox is a HOST-LOCAL Linux sandbox front door. It does NOT resolve or
// dispatch through the configured runtime backend (host-shell / proxy /
// docker_compose) — it always runs on the host. It composes Landlock
// (filesystem integrity) with pure-Go seccomp-BPF (network + syscall
// hardening) in a two-stage re-exec trampoline. The Landlock/seccomp
// restrictions apply only to the host process tree directly launched by the
// trampoline; they do NOT become Docker, proxy, or remote-backend security
// policy (a daemon-created container process does not inherit the caller's
// Landlock/seccomp profile). It is layered WITH exec-ro — it does NOT replace
// it — but it is NOT a universal OS backstop for exec-ro across all backends,
// because exec-ro dispatches through the runtime backend (in-container under
// proxy/docker_compose) while exec-sandbox is host-local-only.
var execSandboxCmd = &cobra.Command{
	Use:   "exec-sandbox <command> [args...]",
	Short: "Run a command under a kernel-enforced Linux sandbox (Landlock + seccomp)",
	Long: `exec-sandbox is a HOST-LOCAL Linux sandbox front door. It does NOT resolve
or dispatch through the configured runtime backend (host-shell / proxy /
docker_compose) — it always runs on the host. The Landlock (filesystem
integrity) + pure-Go seccomp-BPF (network + high-risk syscall hardening)
restrictions apply to the host process tree directly launched by the
sandbox trampoline. They do NOT become Docker, proxy, or remote-backend
security policy: Docker is client/server, so a daemon-created container
process is governed by the container's own security policy, NOT by the
caller's Landlock/seccomp profile. Treat this as an integrity + coarse
network boundary for HOST-LOCAL execution — NOT a confidentiality boundary,
NOT a selective egress-control system, and NOT a sandbox that follows the
payload into a container runtime.

It composes two pure-Go, unprivileged, kernel-enforcing primitives in a
two-stage re-exec trampoline. It is layered WITH exec-ro — it does NOT
replace it — but it is NOT the authoritative OS layer behind exec-ro across
all backends: exec-ro classifies the command host-side and then dispatches
through the runtime backend (under proxy/docker_compose the classified
command runs in-container), while exec-sandbox is host-local-only and never
reaches the backend. Use exec-sandbox when you want kernel-enforced
host-local write/network containment.

HONESTY: exec-sandbox is an INTEGRITY + NETWORK boundary, NOT a
confidentiality boundary. Denied paths remain stat-able (metadata visible
via stat/lstat) but are unwritable (EACCES on open-for-write). opendir is
also gated, so listing a denied directory (e.g. "ls ~/.ssh") fails with
EACCES. The guarantee is "the command cannot WRITE or NETWORK outside the
contract," NOT "the command cannot SEE anything."

Default profile (Profile B):
  Read:    repo root, /usr, /bin, /sbin, /lib, /lib64, /lib32, /etc
  Write:   ./tmp/ only
  Network: denied (seccomp blocks socket/connect/bind/listen/accept/sendto/recvfrom)
  .git:    read-only (inherited from repo root — Landlock is additive)

Modes (--sandbox):
  off          No sandbox; run directly.
  best-effort  Use OS sandbox if available; otherwise warn + exec-ro fallback.
  strict       Require OS sandbox; fail-closed if unavailable.

Network (--net):
  deny   Block network syscalls via seccomp (default).
  allow  Permit network syscalls.
  ask    Interactive [Y/n] prompt (TTY only). Non-TTY hard-denies + exits non-zero.

Example:
  vh-agent-harness exec-sandbox --sandbox=best-effort --net=deny -- go test ./...
  vh-agent-harness exec-sandbox -- ls -la`,
	Args: cobra.MinimumNArgs(1),
	RunE: runExecSandbox,
}

func init() {
	execSandboxCmd.Flags().SetInterspersed(false)
	execSandboxCmd.Flags().StringVar(&execSandboxMode, "sandbox", "best-effort",
		"sandbox mode: off|best-effort|strict")
	execSandboxCmd.Flags().StringVar(&execSandboxNet, "net", "deny",
		"network policy: deny|allow|ask")
	execSandboxCmd.Flags().StringVar(&execSandboxCWD, "cwd", "",
		"working directory (defaults to repo root)")
	execSandboxCmd.Flags().StringVar(&execSandboxProfile, "profile", "default",
		"profile name (currently only 'default' = Profile B)")
}

func runExecSandbox(cmd *cobra.Command, args []string) error {
	mode, err := parseSandboxMode(execSandboxMode)
	if err != nil {
		return err
	}
	net, err := parseNetPolicy(execSandboxNet)
	if err != nil {
		return err
	}

	realCWD, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("determining working directory: %w", err)
	}

	// repoRoot is the SANDBOX PROFILE working directory (read repoRoot, write
	// repoRoot/tmp) — it honors --cwd. The floor root is resolved SEPARATELY
	// from realCWD via a walk-up (applyFloorToRequest -> FindMinMode), so a
	// caller cannot escape the project's strict floor by cd-ing into a
	// subdirectory or pointing --cwd at ./tmp (B3 closed).
	repoRoot := realCWD
	if execSandboxCWD != "" {
		repoRoot = execSandboxCWD
	}
	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return fmt.Errorf("absolutizing repo root: %w", err)
	}
	repoRoot = filepath.Clean(absRoot)

	// MODE-FLOOR + NET-FLOOR (binary-side containment). The exec-sandbox grant
	// to read-only agents is safe only because the binary clamps BOTH the
	// caller-supplied --sandbox mode AND --net policy UP to exec_sandbox.min_mode,
	// so an agent cannot escape strict by passing --sandbox=off (P5 bypass) or
	// --net=allow, by duplicating either flag, or by interspersing it (cobra
	// resolves all occurrences to one value before this runs). A strict floor
	// forces ModeStrict (writes-outside-tmp impossible) AND NetDeny (network
	// impossible) — the Level-B containment contract. cobra resolves duplicate/
	// interspersed flags to one value before applyFloorToRequest runs; ApplyFloor
	// then clamps that resolved value up to the floor, so no flag arrangement can
	// downgrade below the floor.
	//
	// The floor is resolved from BOTH realCWD AND repoRoot (--cwd), taking the
	// MAX of both. This closes a bypass where a caller outside the project uses
	// --cwd to target a strict-floored project from /tmp: the floor from realCWD
	// (/tmp) is ModeOff, but the floor from repoRoot discovers the project's
	// strict floor. Taking the max ensures neither axis escapes.
	// Fail-closed if the floor is present-but-undecodable (a typo must not
	// silently disable a floor the operator asked for) or if strict is required
	// but the OS sandbox primitives are unavailable (handled in execsandbox.Run).
	effectiveMode, effectiveNet, err := applyFloorToRequest(mode, net, realCWD, repoRoot)
	if err != nil {
		return err
	}
	if effectiveMode != mode {
		fmt.Fprintf(os.Stderr,
			"exec-sandbox: --sandbox=%s upgraded to %s (exec_sandbox.min_mode floor; caller cannot run below the floor)\n",
			mode, effectiveMode)
	}
	if effectiveNet != net {
		fmt.Fprintf(os.Stderr,
			"exec-sandbox: --net=%s upgraded to deny (exec_sandbox.min_mode=strict floor; Level-B containment denies network)\n",
			net)
	}
	mode, net = effectiveMode, effectiveNet

	profile := execsandbox.DefaultProfile(repoRoot)
	profile.Net = net

	ctx := context.Background()
	exitCode, runErr := execsandbox.Run(ctx, mode, profile, repoRoot, args[0], args[1:])
	if runErr != nil {
		fmt.Fprintf(os.Stderr, "exec-sandbox: %v\n", runErr)
	}
	if exitCode != 0 {
		os.Exit(exitCode)
	}
	return nil
}

// applyFloorToRequest is the testable core of the binary-side containment: it
// resolves the floor for BOTH realCWD and repoRoot (walking UP from each to find
// the enclosing project's run-shape.yml via runshape.FindMinMode), takes the
// MAX (most restrictive) of both, and clamps the caller's requested mode and net
// policy up to it. runExecSandbox calls this AFTER cobra flag resolution and
// feeds the result to execsandbox.Run. Extracting it makes the full clamp
// pipeline (flag-resolved values -> floor -> effective mode+net) unit-testable
// without the kernel Run path.
//
// Dual-root resolution closes a bypass where a caller outside the project uses
// --cwd to target a strict-floored project: the floor from realCWD (/tmp) is
// ModeOff, but the floor from repoRoot discovers the project's strict floor.
// Taking the max of both ensures neither axis escapes.
//
// Floor contract:
//   - floor == off (absent from both roots): no clamp — requested mode and net
//     honored exactly (standalone behavior).
//   - floor == best-effort (from either root): mode clamped up to best-effort.
//   - floor == strict (from either root): mode clamped up to strict AND net
//     forced to deny (the Level-B containment contract).
//
// Any error from the floor loader is fail-closed (refuse to run uncontained) —
// a floor the operator deliberately set must not be silently dropped.
func applyFloorToRequest(reqMode execsandbox.SandboxMode, reqNet execsandbox.NetPolicy, realCWD string, repoRoot string) (execsandbox.SandboxMode, execsandbox.NetPolicy, error) {
	floor := execsandbox.ModeOff
	for _, root := range []string{realCWD, repoRoot} {
		f, err := loadExecSandboxFloor(root)
		if err != nil {
			return reqMode, reqNet, err
		}
		// Take the MAX of both floors — a strict floor from EITHER root applies.
		floor = execsandbox.ApplyFloor(floor, f)
	}
	effMode := execsandbox.ApplyFloor(reqMode, floor)
	effNet := reqNet
	// A strict floor forces Level-B containment: not just write-containment
	// (ModeStrict via ApplyFloor) but ALSO network denial. --net=allow or
	// --net=ask under a strict floor is upgraded to deny so the shipped claim
	// "strict = writes-outside-tmp and network are impossible" holds.
	if floor == execsandbox.ModeStrict && effNet != execsandbox.NetDeny {
		effNet = execsandbox.NetDeny
	}
	return effMode, effNet, nil
}

// loadExecSandboxFloor resolves the exec_sandbox.min_mode floor for floorRoot
// (walking up to find the enclosing run-shape). Absent file / absent key /
// empty value => ModeOff (no floor). ANY load error (present-but-wrong-type
// min_mode, document syntax error, unreadable file) is FAIL-CLOSED — the
// operator asked for a floor we cannot honor, so refuse rather than silently
// running uncontained. The schema validator (doctor) catches structural typos
// at health-check time; this is the runtime defense-in-depth.
func loadExecSandboxFloor(floorRoot string) (execsandbox.SandboxMode, error) {
	_, raw, err := runshape.FindMinMode(floorRoot)
	if err != nil {
		return execsandbox.ModeOff, fmt.Errorf("exec-sandbox: cannot read exec_sandbox.min_mode floor; refusing to run uncontained: %w", err)
	}
	floor, perr := execsandbox.ParseMinMode(raw)
	if perr != nil {
		// Explicit-but-invalid min_mode value (e.g. a string typo like "strcit"):
		// fail closed.
		return execsandbox.ModeOff, fmt.Errorf("exec-sandbox: %w", perr)
	}
	return floor, nil
}

func parseSandboxMode(s string) (execsandbox.SandboxMode, error) {
	switch s {
	case "off":
		return execsandbox.ModeOff, nil
	case "best-effort":
		return execsandbox.ModeBestEffort, nil
	case "strict":
		return execsandbox.ModeStrict, nil
	default:
		return "", fmt.Errorf("invalid --sandbox=%q (use off|best-effort|strict)", s)
	}
}

func parseNetPolicy(s string) (execsandbox.NetPolicy, error) {
	switch s {
	case "deny":
		return execsandbox.NetDeny, nil
	case "allow":
		return execsandbox.NetAllow, nil
	case "ask":
		return execsandbox.NetAsk, nil
	default:
		return "", fmt.Errorf("invalid --net=%q (use deny|allow|ask)", s)
	}
}
