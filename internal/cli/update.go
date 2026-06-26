package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/vhqtvn/vh-agent-harness/internal/lineage"
)

// updateCmd is the SEAM update path (Slice 2). It re-renders the embedded core
// corpus into the existing target tree and lets the seam reconcile per-class:
//
//   - platform_managed files are refreshed to the current platform bytes;
//   - project_owned files present on disk are preserved byte-for-byte (never
//     clobbered); absent owned files are seeded once;
//   - platform_armed files are schema-reconciled (clean merge applied) or, when
//     the platform and project disagree in a way the schema cannot auto-resolve,
//     left untouched and reported as a structured proposal.
//
// This replaces the legacy read-only "version mismatch" status check. There is
// no "out of date" concept any more: update always reconciles the tree to the
// current platform template the binary carries. The legacy manifest path
// (loadManifest / drift.Compute) is unchanged for render/upgrade/uninstall/
// preflight; manifest convergence is a later slice.
var updateCmd = &cobra.Command{
	Use:           "update",
	Short:         "Re-render and reconcile the harness in the current project (seam apply)",
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `Re-render the embedded core corpus and reconcile the installed harness.

Runs the seam apply against the existing target tree:
  - managed files refreshed to current platform bytes
  - owned files preserved byte-for-byte when present
  - armed files (vh-harness-profile.yml) schema-reconciled

Answers are taken from the existing lineage record when present (so a re-render
is faithful to the original install), or default to the cwd basename otherwise.
When an armed file has a needs-decision conflict, the project instance is left
untouched and a structured proposal is printed.

This is the seam update path. It does NOT touch the legacy manifest model
(.opencode/harness-manifest.json).`,
	Args: cobra.NoArgs,
	RunE: runUpdate,
}

// updateTargetFlag lets tests/CI point update at a target other than cwd.
var updateTargetFlag string
var updateDryRun bool

func init() {
	updateCmd.Flags().StringVarP(&updateTargetFlag, "target", "o", "",
		"target directory (default: current directory)")
	updateCmd.Flags().BoolVar(&updateDryRun, "dry-run", false,
		"preview the per-file plan without writing anything")
}

func runUpdate(cmd *cobra.Command, _ []string) (err error) {
	// A rejected seam apply (e.g. an ownership downgrade) is a genuine runtime
	// error; update runs with SilenceErrors:true so Cobra prints nothing, which
	// would otherwise produce a bare non-zero exit. Surface the reason to stderr
	// exactly once before returning. errSilent (never returned here today) is
	// skipped by reportRunErrToStderr.
	defer func() { reportRunErrToStderr(cmd, err) }()

	out := cmd.OutOrStdout()

	target := updateTargetFlag
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

	// Recover the install-identity render answers from lineage so the re-render
	// is faithful to the original install (project_name/slug the operator chose).
	// A missing lineage means the project was never seam-installed; fall back to
	// defaults rather than refusing, so `update` can also adopt a pre-seam tree.
	// An unreadable lineage is warned but not fatal (proceed with defaults).
	answers := installRenderAnswers(abs)
	if _, lerr := lineage.Read(abs); lerr != nil {
		fmt.Fprintf(out, "update: warning: lineage unreadable (%v); proceeding with default answers\n", lerr)
	}

	report, err := seamApply(abs, answers, updateDryRun)
	if err != nil {
		return err
	}

	if updateDryRun {
		printDryRunPlan(out, "update", abs, report)
		return nil
	}

	fmt.Fprintf(out, "update: seam reconciled %d file(s) in %s\n", len(report.Outcomes), abs)
	fmt.Fprintln(out, summarizeOutcomes(report.Outcomes))
	if sp := summarizeProposals(report.Proposals); sp != "" {
		fmt.Fprintln(out, sp)
	}
	if report.LineagePath != "" {
		fmt.Fprintf(out, "lineage: %s\n", report.LineagePath)
	}
	printNextStepsFooter(out, abs)
	return nil
}

// defaultAnswers returns the project_name/project_slug derived from the target
// directory basename (mirrors install's default when no --name/--slug given).
func defaultAnswers(target string) map[string]string {
	base := filepath.Base(target)
	if base == "" || base == "." || base == string(os.PathSeparator) {
		base = "my-project"
	}
	return map[string]string{
		"project_name": base,
		"project_slug": base,
	}
}

// installRenderAnswers returns the install-identity render answers recovered
// from the S1 lineage record (project_name/project_slug/coordinator_dir the
// operator chose at install), falling back to defaultAnswers(target) when there
// is no lineage or it carries no raw values (a pre-seam tree, or a lineage
// written before AnswersRef.Values existed). doctor and update use this as the
// base so a re-render is faithful to the original install instead of the target
// dir basename: without it, the harness-token-bearing managed files
// ({PROJECT_NAME}/{PROJECT_SLUG}/{COORDINATOR_DIR}) would false-flag managed
// drift on doctor and get silently rewritten on update whenever the install
// name/slug differ from the dir basename. The returned map is a defensive copy
// (the lineage's internal map is never aliased).
func installRenderAnswers(target string) map[string]string {
	if lin, err := lineage.Read(target); err == nil && lin != nil && len(lin.Answers.Values) > 0 {
		out := make(map[string]string, len(lin.Answers.Values))
		for k, v := range lin.Answers.Values {
			out[k] = v
		}
		return out
	}
	return defaultAnswers(target)
}
