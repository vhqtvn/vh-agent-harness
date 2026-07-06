package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/vhqtvn/vh-agent-harness/internal/execsandbox"
)

var (
	execSandboxMode    string
	execSandboxNet     string
	execSandboxCWD     string
	execSandboxProfile string
)

// execSandboxCmd implements `vh-agent-harness exec-sandbox <cmd>`.
//
// exec-sandbox is the authoritative OS-level defense-in-depth layer behind the
// script-level exec-ro. It composes Landlock (filesystem integrity) with
// pure-Go seccomp-BPF (network + syscall hardening) in a two-stage re-exec
// trampoline. Layered WITH exec-ro — it does NOT replace it.
var execSandboxCmd = &cobra.Command{
	Use:   "exec-sandbox <command> [args...]",
	Short: "Run a command under a kernel-enforced Linux sandbox (Landlock + seccomp)",
	Long: `exec-sandbox runs a command inside a pure-Go, unprivileged, kernel-enforcing
Linux sandbox composed of Landlock (filesystem integrity) and pure-Go seccomp-BPF
(network + high-risk syscall hardening).

It is the authoritative defense-in-depth layer behind exec-ro. exec-ro is a
script-level heuristic pre-filter; exec-sandbox provides kernel-enforced
guarantees that survive even if the target command tries to bypass shell-level
checks.

HONESTY: exec-sandbox is an INTEGRITY + NETWORK boundary, NOT a confidentiality
boundary. Denied paths remain VISIBLE (ls-able) but are unwritable (EACCES).
The guarantee is "the command cannot WRITE or NETWORK outside the contract,"
NOT "the command cannot SEE anything."

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

	repoRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("determining working directory: %w", err)
	}
	if execSandboxCWD != "" {
		repoRoot = execSandboxCWD
	}
	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return fmt.Errorf("absolutizing repo root: %w", err)
	}
	repoRoot = filepath.Clean(absRoot)

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
