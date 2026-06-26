package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/vhqtvn/vh-agent-harness/internal/proposals"
)

// proposalsCmd is the Slice-5.3 surface for the proposal ledger. It lists every
// armed-file conflict the seam apply path recorded but did not auto-resolve
// (.vh-agent-harness/proposals.jsonl). Each entry names the path, its ownership
// class, the harness ref that produced it, the timestamp, and the structured
// proposals (e.g. enum_removed) the reconciler emitted.
//
// The ledger is append-only across updates, so this command shows the full
// history of needs-decision surfaces. Slice 6 (HELD beyond v0) adds the D2-C
// path that ACTS on a proposal (downgrade protection to resolve it); until then
// this command is read-only — it lists, never resolves.
var proposalsCmd = &cobra.Command{
	Use:          "proposals",
	Short:        "List recorded armed-file conflicts (proposal ledger)",
	SilenceUsage: true,
	Long: `List every armed-file conflict the seam apply path surfaced but did not
auto-resolve, recorded in .vh-agent-harness/proposals.jsonl (append-only).

When an armed file (e.g. vh-harness-profile.yml) reconciles to a needs-decision
conflict — the platform withdrew an enum value the project still uses, surfaced
as an enum_removed proposal — the apply leaves the live instance untouched and
records the structured proposal here. Review the entries, then either accept the
platform's new contract (edit the project instance to match) or keep the project
value (the harness will keep proposing on each update until resolved).

Read-only: this command lists proposals; it never resolves them.`,
	Args: cobra.NoArgs,
	RunE: runProposals,
}

// proposalsTargetFlag lets tests/CI point proposals at a target other than cwd.
var proposalsTargetFlag string

func init() {
	proposalsCmd.Flags().StringVarP(&proposalsTargetFlag, "target", "o", "",
		"target directory (default: current directory)")
}

func runProposals(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()

	target := proposalsTargetFlag
	if target == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getcwd: %w", err)
		}
		target = cwd
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolve target: %w", err)
	}

	records, err := proposals.Read(abs)
	if err != nil {
		return fmt.Errorf("read proposal ledger: %w", err)
	}
	if len(records) == 0 {
		fmt.Fprintln(out, "proposals: none (no armed-file conflicts recorded)")
		fmt.Fprintln(out, "  ledger:", proposals.FilePath(abs))
		return nil
	}
	fmt.Fprintf(out, "proposals: %d record(s)\n", len(records))
	fmt.Fprintln(out, "  ledger:", proposals.FilePath(abs))
	for i, rec := range records {
		fmt.Fprintf(out, "\n[%d] %s\n", i+1, rec.Path)
		fmt.Fprintf(out, "    class:     %s\n", rec.Class)
		if rec.Ref != "" {
			fmt.Fprintf(out, "    ref:       %s\n", rec.Ref)
		}
		fmt.Fprintf(out, "    recorded:  %s\n", rec.Timestamp)
		for _, p := range rec.Proposals {
			label := p.Field
			if p.Kind != "" {
				label = fmt.Sprintf("%s (%s)", p.Field, p.Kind)
			}
			fmt.Fprintf(out, "    proposal:  %s\n", label)
			if p.PlatformValue != "" {
				fmt.Fprintf(out, "      platform: %s\n", p.PlatformValue)
			}
			if p.ProjectValue != "" {
				fmt.Fprintf(out, "      project:  %s\n", p.ProjectValue)
			}
			if p.Envelope != "" {
				fmt.Fprintf(out, "      envelope: %s\n", p.Envelope)
			}
			if p.Hint != "" {
				fmt.Fprintf(out, "      hint:     %s\n", p.Hint)
			}
		}
	}
	return nil
}
