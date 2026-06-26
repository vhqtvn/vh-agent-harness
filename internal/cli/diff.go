package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/vhqtvn/vh-agent-harness/internal/drift"
)

// diffCmd reports drift between the manifest and disk. It replaces the Slice 1/2
// stub of the same name.
//
// SilenceErrors + SilenceUsage are set so that:
//   - on drift, the only output is the report we print (then a silent non-zero
//     exit via errSilent), with no Cobra "Error:" line or usage dump;
//   - on a genuine runtime error, we print it ourselves to stderr exactly once
//     before returning the error (main() then exits non-zero).
var diffCmd = &cobra.Command{
	Use:           "diff",
	Short:         "Show drift between the corpus and disk",
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `Compare the harness against the files on disk and report drift.

On a seam-installed tree (the default since the substrate seam: an
.vh-agent-harness/lineage.yml is present) diff re-renders the embedded corpus +
active overlays and byte-compares every platform-controlled path. On an older
legacy-manifest install it falls back to comparing against the manifest hashes.

Categories reported (one line per non-ok file):
  ok          on disk, identical to the re-rendered corpus
  drifted     on disk, differs from the corpus (file was edited)
  missing     the corpus produces this path, but it is not on disk
  unexpected  on disk under .opencode/ but not produced by the corpus

platform_armed (schema-merged), project_owned, and external_generated files are
intentionally NOT flagged — they are preserved/merged by design (doctor lints
armed files; project content is yours). Runtime-state subtrees (.opencode/state,
.opencode/plans, .opencode/runs, .opencode/sessions, .opencode/node_modules) are
not flagged as unexpected.

Exit code is non-zero when any non-ok category is present, so this command can
gate CI. For seam installs this mirrors doctor's managed-drift verdict.`,
	Args: cobra.NoArgs,
	RunE: runDiff,
}

func runDiff(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(errOut, "error:", err)
		return err
	}

	// Seam-first: an .vh-agent-harness/lineage.yml means this tree is seam-
	// managed, so re-render the corpus and compare. Legacy manifest installs
	// (no lineage) fall back to the manifest-hash comparison below.
	if isSeamInstalled(cwd) {
		return runSeamDiff(cmd, cwd)
	}

	lm, err := loadManifest()
	if err != nil {
		fmt.Fprintln(errOut, "error:", err)
		return err
	}

	report, err := drift.Compute(lm.dir, lm.m)
	if err != nil {
		fmt.Fprintln(errOut, "error:", err)
		return err
	}

	for _, e := range report.Entries {
		if e.Category == drift.OK {
			continue
		}
		fmt.Fprintf(out, "%-11s %s\n", e.Category, e.Path)
	}
	fmt.Fprintf(out, "summary: %d ok, %d drifted, %d missing, %d unexpected\n",
		report.Counts[drift.OK], report.Counts[drift.Drifted],
		report.Counts[drift.Missing], report.Counts[drift.Unexpected])

	if report.HasProblems() {
		// errSilent carries no message (Cobra prints nothing thanks to
		// SilenceErrors) but is non-nil so main() exits non-zero.
		return errSilent{}
	}
	return nil
}

// runSeamDiff re-renders the corpus + overlays and reports per-file drift for a
// seam-installed tree, with the same exit-code contract as the legacy path.
func runSeamDiff(cmd *cobra.Command, target string) error {
	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()

	rep, err := computeSeamDrift(target)
	if err != nil {
		fmt.Fprintln(errOut, "error:", err)
		return err
	}
	for _, p := range rep.drifted {
		fmt.Fprintf(out, "%-11s %s\n", "drifted", p)
	}
	for _, p := range rep.missing {
		fmt.Fprintf(out, "%-11s %s\n", "missing", p)
	}
	for _, p := range rep.unexpected {
		fmt.Fprintf(out, "%-11s %s\n", "unexpected", p)
	}
	fmt.Fprintf(out, "summary: %d ok, %d drifted, %d missing, %d unexpected\n",
		len(rep.ok), len(rep.drifted), len(rep.missing), len(rep.unexpected))

	if rep.hasProblems() {
		return errSilent{}
	}
	return nil
}

// errSilent is a no-message error used to force a non-zero process exit without
// Cobra appending any output of its own.
type errSilent struct{}

func (errSilent) Error() string { return "" }

// reportRunErrToStderr prints a command's returned error to its stderr exactly
// once when it is a genuine (non-silent) error, so a rejected seam apply — most
// importantly an ownership downgrade rejected by the raise-only rule
// (*ownership.DowngradeError naming the path, the from/to classes, and the
// future reviewed-downgrade guidance) — surfaces its human-readable reason
// instead of producing a bare non-zero exit with no output.
//
// update/doctor set SilenceErrors:true so Cobra does not append its own
// "Error:" line (the verb already printed its own report to stdout). The
// downside was that a genuine runtime error was silenced too. errSilent is
// skipped here on purpose: the verb has already printed everything it needs to
// (e.g. doctor's full UNHEALTHY report), and errSilent only forces the non-zero
// exit. This mirrors diff.go's own "print genuine runtime errors ourselves to
// stderr exactly once" contract. No-op for nil.
func reportRunErrToStderr(cmd *cobra.Command, err error) {
	if err == nil {
		return
	}
	var silent errSilent
	if errors.As(err, &silent) {
		return
	}
	fmt.Fprintln(cmd.ErrOrStderr(), "error:", err)
}
