package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/vhqtvn/vh-agent-harness/internal/taskcard"
)

// taskcard.go — the CLI bridge for the task-card contract validator
// (defer-018). It is the Go replacement for the retired Python
// templates/core/.opencode/scripts/verify-task-card-schema.py: validate a
// task-card JSON document (file or stdin) against the task-card schema + the
// cross-field acknowledgement-pair guard, and print a verdict.
//
// The pure validation logic lives in internal/taskcard (a bounded, dependency-
// free draft-07 subset engine + the ack guard). This command is a thin bridge:
// it reads bytes, calls taskcard.ValidateCard, and formats the result. No
// filesystem writes, no side effects — INFORMS only.

// taskCardCmd is the parent for task-card diagnostics. Currently exposes one
// subcommand (validate); future contract checks can hang here.
var taskCardCmd = &cobra.Command{
	Use:          "task-card",
	Short:        "Task-card contract validation (schema + acknowledgement-pair guard)",
	SilenceUsage: true,
	Long: `Task-card contract validation for coordination task cards.

Subcommands validate a task-card JSON document against the task-card schema
(docs/coordination/schemas/task-card.schema.json) plus the cross-field
acknowledgement-pair invariant (recurrence_count >= last_acknowledged_count)
that JSON Schema draft-07 cannot express.

The validator ships a bounded, dependency-free draft-07 subset engine (no
Python, no pip). This is the Go port of the retired
.opencode/scripts/verify-task-card-schema.py.`,
	Args: cobra.NoArgs,
}

// taskCardValidateCmd validates a task-card JSON file (path arg) or stdin. It
// prints the verdict to stdout and exits 0 when the card is valid, 1 when it is
// rejected (defect list on stdout) or when input reading/parse fails (message
// on stderr). SilenceErrors keeps cobra from echoing the silent sentinel so a
// normal rejection reads cleanly (stdout verdict + exit 1, no stderr noise);
// genuine I/O/parse errors are printed to stderr explicitly below.
var taskCardValidateCmd = &cobra.Command{
	Use:           "validate [<file>]",
	Short:         "Validate a task-card JSON file (or stdin) against the schema",
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `Validate a task-card JSON document against the task-card schema + the
acknowledgement-pair guard.

With a file path argument the card is read from that file. With no argument the
card is read from stdin. The verdict and any defects are printed to stdout; the
process exits 0 when the card is valid and 1 when it is rejected. A read/parse
error is reported on stderr with a non-zero exit.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runTaskCardValidate,
}

func init() {
	taskCardCmd.AddCommand(taskCardValidateCmd)
	rootCmd.AddCommand(taskCardCmd)
	assignGroup(groupHealth, taskCardCmd)
}

// errTaskCardReject is the non-zero exit sentinel returned for BOTH a rejected card and
// a genuine read/parse error. SilenceErrors (above) stops cobra echoing it as
// "Error:"; the caller's verdict goes to stdout (rejection) or an explicit
// stderr line (read/parse error). It carries no message because the human-
// readable detail is already printed by the caller.
var errTaskCardReject = fmt.Errorf("task-card: non-zero exit")

// runTaskCardValidate is the in-process entry point for the validate subcommand.
// Exported via closure so tests exercise it directly without spawning a process.
func runTaskCardValidate(cmd *cobra.Command, args []string) error {
	body, err := readCardInput(cmd, args)
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "task-card:", err)
		return errTaskCardReject
	}
	res, err := taskcard.ValidateCard(body)
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "task-card:", err)
		return errTaskCardReject
	}
	out := cmd.OutOrStdout()
	if res.Valid {
		fmt.Fprintln(out, "task-card: valid")
		return nil
	}
	printTaskCardDefects(out, res)
	return errTaskCardReject
}

// readCardInput resolves and reads the input: the file path arg when given
// (os.ReadFile closes the descriptor), otherwise the command's stdin
// (cmd.InOrStdin, so tests can inject a reader).
func readCardInput(cmd *cobra.Command, args []string) ([]byte, error) {
	if len(args) == 0 {
		body, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		return body, nil
	}
	body, err := os.ReadFile(args[0])
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", args[0], err)
	}
	return body, nil
}

// printTaskCardDefects writes the schema-level defects and/or the ack-pair
// violation as human-readable lines on stdout. A rejected card's verdict lives
// entirely on stdout (exit 1 carries the failure); stderr is reserved for
// read/parse errors.
func printTaskCardDefects(out io.Writer, res *taskcard.Result) {
	if len(res.SchemaErrors) > 0 {
		fmt.Fprintf(out, "task-card: schema rejected (%d defect(s))\n", len(res.SchemaErrors))
		for _, e := range res.SchemaErrors {
			fmt.Fprintf(out, "  - %s\n", e.String())
		}
	}
	if res.AckPairError != "" {
		if len(res.SchemaErrors) == 0 {
			fmt.Fprintln(out, "task-card: ack-pair guard rejected")
		}
		fmt.Fprintf(out, "  - recurrence: %s\n", res.AckPairError)
	}
}
