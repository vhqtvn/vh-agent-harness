package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vhqtvn/vh-agent-harness/internal/execro"
	"github.com/vhqtvn/vh-agent-harness/internal/hooks"
	"github.com/vhqtvn/vh-agent-harness/internal/runshape"
	"github.com/vhqtvn/vh-agent-harness/internal/runtime"
)

// execRoCmd runs a command inside the configured runtime backend AFTER the
// exec-ro strictly-read-only classifier approves it. Unlike `vh-agent-harness
// exec` (which routes through the shell-guard permission gate and may prompt),
// exec-ro is allowlisted in opencode.jsonc as `vh-agent-harness exec-ro *` so
// opencode NEVER prompts for it — which means exec-ro itself is the ONLY gate
// for its payload. The internal execro.Classifier hard-denies mutations,
// out-of-repo reads, and shell metacharacters; anything it cannot prove
// read-only is denied with a notice (opencode cannot prompt for an allowlisted
// command).
//
// exec-ro NEVER rewrites the command: it either executes it exactly as given or
// exits non-zero with the notice.
var execRoCmd = &cobra.Command{
	Use:   "exec-ro [--] <cmd> [args...]",
	Short: "Run a read-only command inside the harness runtime (exec-ro classifier gates it)",
	Long: `exec-ro is a HOST-SIDE INTENT CLASSIFIER that runs BEFORE backend dispatch.
It classifies the requested command against the host repo path, then delegates
execution to the selected runtime backend. It is NOT proof that the backend
payload is OS-sandboxed or running on read-only mounts; backend filesystem and
security enforcement depend on the selected runtime. Under host-shell the
classified command runs locally; under proxy/docker_compose it runs inside the
container against the container's filesystem view.

Unlike ` + "`vh-agent-harness exec`" + ` (which routes through the shell-guard permission gate
and may prompt), exec-ro is allowlisted in opencode.jsonc as ` + "`vh-agent-harness exec-ro *`" + `
so opencode NEVER prompts for it. exec-ro itself is therefore the ONLY gate: its
internal classifier hard-denies git mutations, out-of-repo reads, shell
metacharacters, and any command it cannot prove is read-only. The deny notice
explains WHY and suggests the bare-command alternative (which DOES prompt through
the normal permission table).

Allowed: read-only git inspection (log/show/diff/status/...), read-only non-git
binaries (ls/cat/grep/rg/jq/find/wc/head/tail/...). Denied: any git mutation
(commit/push/rm/...), any path-bearing git flag pointing outside the repo
(-C/--git-dir/--work-tree), relative -C paths, shell metacharacters
(| ; & $ ` + "`" + ` > < newline), and any binary outside the readonly set.

exec-ro NEVER rewrites the command: it executes it exactly as given or denies it.

Flag parsing STOPS at the first positional (the command), so the command's own
flags pass straight through and you do NOT need ` + "`--`" + `:
   vh-agent-harness exec-ro git log --oneline      # --oneline goes to git
   vh-agent-harness exec-ro ls -la                 # -la goes to ls
Use ` + "`--`" + ` only when the command's FIRST token would otherwise look like a flag.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runExecRo,
}

func init() {
	// Stop flag parsing at the first positional so the wrapped command's own
	// flags pass through to it (mirrors execCmd).
	execRoCmd.Flags().SetInterspersed(false)
}

// runExecRo: load manifest -> resolve backend -> pre_exec hook -> exec-ro
// classify -> exec (if allowed) or deny+notice -> post_exec hook. exec-ro is
// allowlisted in opencode.jsonc, so the classifier IS the gate: there is NO
// evaluateGate call here. The outer `vh-agent-harness exec-ro` invocation was
// already allowed by opencode's permission table (that is why exec-ro must gate
// its own payload — opencode cannot prompt for it). pre_exec/post_exec WRAP the
// classify+exec, matching runExec's hook ordering.
func runExecRo(cmd *cobra.Command, args []string) error {
	be, lm, err := resolveBackend()
	if err != nil {
		return err
	}
	ctx := context.Background()
	if _, err := fireHook(ctx, hooks.Point(runshape.HookPreExec), be.Name(), lm.dir); err != nil {
		return fmt.Errorf("pre_exec hook: %w", err)
	}

	verdict := execro.Classify(strings.Join(args, " "), lm.dir)
	if !verdict.Allow {
		fmt.Fprintln(os.Stderr, verdict.Notice)
		return fmt.Errorf("denied by exec-ro classifier")
	}

	opts := runtime.ExecOpts{Interactive: false}
	if err := be.Exec(ctx, args, opts); err != nil {
		return err
	}
	if _, err := fireHook(ctx, hooks.Point(runshape.HookPostExec), be.Name(), lm.dir); err != nil {
		return fmt.Errorf("post_exec hook: %w", err)
	}
	return nil
}
