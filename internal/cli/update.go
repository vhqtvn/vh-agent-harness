package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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
(.opencode/harness-manifest.json).

Interactive guard: when the target is not an initialized harness project (no
.vh-agent-harness/vh-harness-profile.yml) and stdin is a TTY, update asks for a
yes/no confirmation before scaffolding managed files there — so a hand-run
update in the wrong directory cannot install dust by accident. The prompt is
bypassed automatically for non-interactive callers (piped stdin, agents, CI,
'make update', '/harness'), and can be skipped with --force (-f) or
RUN_FROM_AGENT=1. --dry-run never prompts (it writes nothing).`,
	Args: cobra.NoArgs,
	RunE: runUpdate,
}

// updateTargetFlag lets tests/CI point update at a target other than cwd.
var updateTargetFlag string
var updateDryRun bool
var updateForce bool

// The uninitialized-target confirmation guard (interactive-only pre-flight gate)
// is wired through two injectable seams so unit tests can drive both the
// prompt-fires path and every bypass path without a real TTY or os.Stdin:
//
//   - updateStdinIsTTY reports whether stdin looks interactive. The default uses
//     a stdlib-only os.Stdin.Stat() check (no isatty dependency); tests override
//     it to force the interactive or non-interactive branch.
//   - updateConfirm prints the warning + reads a yes/no answer. The default
//     reads from os.Stdin; tests override it to simulate accept/decline and to
//     assert the prompt was (or was not) reached.
//
// Both mirror the existing updateTargetFlag/updateDryRun/updateForce package
// vars. Non-interactive callers (agents, CI, `make update`, `/harness`, piped
// input) bypass the prompt automatically via the non-TTY check; --force,
// RUN_FROM_AGENT=1, and --dry-run also bypass.
var updateStdinIsTTY = func() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

var updateConfirm = defaultUpdateConfirm

func init() {
	updateCmd.Flags().StringVarP(&updateTargetFlag, "target", "o", "",
		"target directory (default: current directory)")
	updateCmd.Flags().BoolVar(&updateDryRun, "dry-run", false,
		"preview the per-file plan without writing anything")
	updateCmd.Flags().BoolVarP(&updateForce, "force", "f", false,
		"bypass the uninitialized-target confirmation prompt")
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

	// Uninitialized-target confirmation guard (interactive-only pre-flight gate).
	// `update` deliberately adopts any tree it is pointed at, so a hand-run
	// `update` in the wrong directory would scaffold managed files ("dust") that
	// then have to be removed by hand. When the target carries no harness profile
	// AND the call is interactive AND nothing bypasses the prompt, require an
	// explicit yes/no before adopting the tree. The profile is the authoritative
	// "this is a harness project" signal; a lineage file without a profile is
	// still treated as uninitialized here (the existing unreadable-lineage warn
	// below is unchanged). --dry-run writes nothing, so it is safe to run
	// anywhere and never prompts. On decline, nothing is written and update
	// returns successfully (exit 0).
	if !updateDryRun && !updateForce && !envTruthy("RUN_FROM_AGENT") &&
		updateStdinIsTTY() && !harnessProfileExists(abs) {
		if !updateConfirm(out, abs) {
			fmt.Fprintln(out, "No changes made.")
			return nil
		}
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

	// Warn loudly when project.config.json is absent or a consumed token resolves
	// empty (W3): non-fatal, emitted to stderr so it appears under --dry-run too.
	// (On update the CLAUDE.md/Makefile are project_owned and preserved when
	// already present, but an absent config still affects any seed/re-seed and is
	// the signal a consumer needs to know the tokens are unresolved.)
	warnUnresolvedProjectConfigTokens(os.Stderr, abs)

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

// harnessProfileExists reports whether target carries the harness profile
// (.vh-agent-harness/vh-harness-profile.yml) — the authoritative "this is a
// harness project" signal used by the interactive confirmation guard. A target
// without it is treated as uninitialized. A lineage file present without a
// profile is still treated as uninitialized: the profile is the project-intent
// signal, not the lineage record (which may survive a partial teardown).
func harnessProfileExists(target string) bool {
	_, err := os.Stat(filepath.Join(target, harnessProfileName))
	return err == nil
}

// envTruthy reports whether the environment variable named by key is set to a
// canonical truthy value, via the single truthiness check reused across the
// CLI's env-driven bypass switches (e.g. RUN_FROM_AGENT).
func envTruthy(key string) bool {
	return truthyString(os.Getenv(key))
}

// truthyString is the canonical env-value truthiness test. Truthy = one of
// "1", "true", "yes", "on" (case-insensitive, after trimming whitespace);
// everything else — including the empty string and "0" — is false.
func truthyString(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// defaultUpdateConfirm is the default uninitialized-target confirmation prompt.
// It prints a warning naming the absolute target dir, states that managed files
// (.opencode/, etc.) will be scaffolded/adopted there, suggests --dry-run first,
// then reads a yes/no answer from os.Stdin. Returns true on accept (proceed),
// false on decline (no / empty / EOF). The prompt style mirrors selfupdate's
// "[y/N]" confirmation.
func defaultUpdateConfirm(out io.Writer, target string) bool {
	fmt.Fprintf(out, "Target %s is not an initialized harness project ", target)
	fmt.Fprintln(out, "(no .vh-agent-harness/vh-harness-profile.yml found).")
	fmt.Fprintln(out, "Running update here will scaffold/adopt managed files (.opencode/, etc.) into that directory.")
	fmt.Fprintln(out, "Preview first with `vh-agent-harness update --dry-run`.")
	fmt.Fprint(out, "Proceed with update? [y/N]: ")
	var ans string
	_, _ = fmt.Scanln(&ans)
	return strings.EqualFold(strings.TrimSpace(ans), "y")
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
