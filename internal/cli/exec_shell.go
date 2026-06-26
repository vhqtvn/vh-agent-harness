package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/vhqtvn/vh-agent-harness/internal/hooks"
	"github.com/vhqtvn/vh-agent-harness/internal/permission"
	"github.com/vhqtvn/vh-agent-harness/internal/runshape"
	"github.com/vhqtvn/vh-agent-harness/internal/runtime"
)

// execFlags holds the flags for `vh-agent-harness exec`.
type execFlags struct {
	service string
	workdir string
	tty     bool
}

var execFl *execFlags

// execCmd runs a command inside the configured runtime backend AFTER the
// permission gate evaluates it. It replaces the slice-1/2/3 stub.
var execCmd = &cobra.Command{
	Use:   "exec [--service <name>] [--workdir <dir>] [--tty] -- <cmd> [args...]",
	Short: "Run a command inside the harness runtime",
	Long: `Run a command inside the configured runtime backend (manifest.runtime.backend).

The command is first evaluated by the permission gate (shell-guard); the gate is
wired to the real shell-guard hook (internal/permission.NewShellGuardHook), which
runs the node eval.js bridge against the active permission rules and may allow,
ask, or deny the command before the runtime backend is touched.

By default exec is NON-interactive (docker compose: -T) so output streams
cleanly to stdout — this matches the canonical ` + "`vh-agent-harness exec bash -c '...'`" + `
usage. Pass --tty/-t to allocate a TTY.

Use ` + "`--`" + ` to separate harness flags from the command when needed, e.g.
   vh-agent-harness exec --service dev -- echo hello

Requires a governing manifest (run ` + "`vh-agent-harness install`" + ` first).`,
	Args: cobra.MinimumNArgs(1),
	RunE: runExec,
}

func init() {
	execFl = &execFlags{}
	execCmd.Flags().StringVar(&execFl.service, "service", "", "target service (default: manifest runtime.default_service)")
	execCmd.Flags().StringVarP(&execFl.workdir, "workdir", "w", "", "working directory inside the runtime")
	execCmd.Flags().BoolVarP(&execFl.tty, "tty", "t", false, "allocate a TTY (interactive)")
}

// shellFlags holds the flags for `vh-agent-harness shell`.
type shellFlags struct {
	service string
}

var shellFl *shellFlags

// shellCmd opens an interactive shell in the default (or --service) container.
// Like exec it runs the permission gate first.
var shellCmd = &cobra.Command{
	Use:   "shell [--service <name>]",
	Short: "Open an interactive shell inside the harness runtime",
	Long: `Open an interactive shell inside the configured runtime backend.

For docker_compose this is ` + "`docker compose exec <service>`" + ` with the host TTY
passed through; for bare it opens the host $SHELL. The command is evaluated by
the permission gate first (the real shell-guard hook, internal/permission.NewShellGuardHook).`,
	Args: cobra.NoArgs,
	RunE: runShell,
}

func init() {
	shellFl = &shellFlags{}
	shellCmd.Flags().StringVar(&shellFl.service, "service", "", "target service (default: manifest runtime.default_service)")
}

// runExec: load manifest -> resolve backend -> pre_exec hook -> evaluate
// permission gate -> exec -> post_exec hook. The gate runs BEFORE the backend is
// touched; on Deny/Ask the backend is never invoked and the process exits non-zero
// with the denial reason. pre_exec/post_exec WRAP the gate+exec (the run-shape spec §5):
// they are gate-checked through the SAME policy pack as the user command, so hooks
// are not a side door around the gate. pre_exec is FailVerb (fires per-invocation,
// use sparingly); post_exec is WarnAndContinue.
func runExec(cmd *cobra.Command, args []string) error {
	be, lm, err := resolveBackend()
	if err != nil {
		return err
	}
	ctx := context.Background()
	if _, err := fireHook(ctx, hooks.Point(runshape.HookPreExec), be.Name(), lm.dir); err != nil {
		return fmt.Errorf("pre_exec hook: %w", err)
	}

	action, reason, err := evaluateGate(lm.dir, args)
	if err != nil {
		// Hook itself failed: deny-by-default for safety.
		fmt.Fprintf(os.Stderr, "permission hook error: %v\n", err)
		return fmt.Errorf("permission hook error: %w", err)
	}
	switch action {
	case permission.Deny:
		fmt.Fprintf(os.Stderr, "denied: %s\n", reason)
		return fmt.Errorf("denied by permission hook: %s", reason)
	case permission.Ask:
		// No operator loop in slice 4a -> deny-by-default.
		fmt.Fprintf(os.Stderr, "permission required (ask): %s — no operator loop attached; denying\n", reason)
		return fmt.Errorf("permission ask (no operator loop): %s", reason)
	case permission.Allow:
		// proceed
	}

	opts := runtime.ExecOpts{
		Interactive: execFl.tty,
		Service:     execFl.service,
		Workdir:     execFl.workdir,
	}
	if err := be.Exec(ctx, args, opts); err != nil {
		return err
	}
	if _, err := fireHook(ctx, hooks.Point(runshape.HookPostExec), be.Name(), lm.dir); err != nil {
		return fmt.Errorf("post_exec hook: %w", err)
	}
	return nil
}

// runShell: load manifest -> resolve backend -> pre_exec hook -> evaluate
// permission gate on the implicit shell invocation -> open interactive shell ->
// post_exec hook. Like exec, the gate runs before the backend; hooks wrap it.
func runShell(cmd *cobra.Command, _ []string) error {
	be, lm, err := resolveBackend()
	if err != nil {
		return err
	}
	ctx := context.Background()
	if _, err := fireHook(ctx, hooks.Point(runshape.HookPreExec), be.Name(), lm.dir); err != nil {
		return fmt.Errorf("pre_exec hook: %w", err)
	}

	// The gate sees the shell intent. cmd is empty (the backend opens its own
	// default shell); we still pass it so the hook can log/inspect the intent.
	// Fork (slice 4b): an empty command makes eval.js return {deny,"empty
	// command"}, so the non-interactive vh-agent-harness shell denies by default until
	// an operator prompt loop is wired. No test breaks (no shell-allows test).
	action, reason, err := evaluateGate(lm.dir, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "permission hook error: %v\n", err)
		return fmt.Errorf("permission hook error: %w", err)
	}
	switch action {
	case permission.Deny:
		fmt.Fprintf(os.Stderr, "denied: %s\n", reason)
		return fmt.Errorf("denied by permission hook: %s", reason)
	case permission.Ask:
		fmt.Fprintf(os.Stderr, "permission required (ask): %s — no operator loop attached; denying\n", reason)
		return fmt.Errorf("permission ask (no operator loop): %s", reason)
	case permission.Allow:
		// proceed
	}

	opts := runtime.ExecOpts{
		Interactive: true, // shell is always interactive
		Service:     shellFl.service,
	}
	if err := be.Exec(ctx, nil, opts); err != nil {
		return err
	}
	if _, err := fireHook(ctx, hooks.Point(runshape.HookPostExec), be.Name(), lm.dir); err != nil {
		return fmt.Errorf("post_exec hook: %w", err)
	}
	return nil
}
