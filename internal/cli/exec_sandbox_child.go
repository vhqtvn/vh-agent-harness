package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/vhqtvn/vh-agent-harness/internal/execsandbox"
)

// execSandboxChildCmd is the hidden internal trampoline subcommand invoked by
// the parent exec-sandbox via fork/exec. It is never user-facing.
//
// argv: vh-agent-harness __exec_sandbox_child -- <target> <args...>
// The profile is passed via VH_EXEC_SANDBOX_* env vars.
var execSandboxChildCmd = &cobra.Command{
	Use:    "__exec_sandbox_child",
	Short:  "Internal exec-sandbox trampoline (do not call directly)",
	Hidden: true,
	Args:   cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := execsandbox.RunChild(args); err != nil {
			// If RunChild returns, syscall.Exec failed. This is fatal.
			fmt.Fprintf(os.Stderr, "exec-sandbox child: %v\n", err)
			os.Exit(1)
		}
		return nil
	},
}
