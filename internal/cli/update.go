package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vhqtvn/vh-agent-harness/internal/lineage"
	"github.com/vhqtvn/vh-agent-harness/internal/ownership"
	"github.com/vhqtvn/vh-agent-harness/internal/renderstate"
	"github.com/vhqtvn/vh-agent-harness/internal/substrate"
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
bypassed automatically when stdin is not a TTY (piped/redirected input, agents,
CI; 'make update' and '/harness' only when their stdin is non-interactive), and
can be skipped with --force (-f) or RUN_FROM_AGENT=1. An interactive 'make
update' in a terminal still has a TTY and still prompts. --dry-run never prompts
(it writes nothing).`,
	Args: cobra.NoArgs,
	RunE: runUpdate,
}

// updateTargetFlag lets tests/CI point update at a target other than cwd.
var updateTargetFlag string
var updateDryRun bool
var updateForce bool
var updatePruneOrphans bool

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
// vars. The prompt is bypassed automatically whenever stdin is not a TTY
// (agents, CI, piped input, and `make update` / `/harness` run non-interactively;
// an interactive `make update` in a terminal still has a TTY and still prompts).
// --force, RUN_FROM_AGENT=1, and --dry-run also bypass.
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
	updateCmd.Flags().BoolVar(&updatePruneOrphans, "prune-orphans", false,
		"delete byte-identical orphan rendered files whose overlay source was removed (refuses hand-edited or project-owned ones; composes with --dry-run)")
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
		// --prune-orphans composes with --dry-run: print what the prune WOULD do
		// without deleting anything. Emitted here (not inside the shared
		// printDryRunPlan in guide.go, which a parallel slice owns) so this
		// feature stays contained to update.go. printDryRunPlan's own
		// preserved-orphan block already lists the orphans; this block states
		// the per-file prune verdict (would-delete vs refuse) for clarity.
		if updatePruneOrphans {
			applyPruneOrphans(out, abs, report.Orphans, true)
		}
		return nil
	}

	if _, err := materializeContextDocs(abs); err != nil {
		return fmt.Errorf("materialize always-on context docs: %w", err)
	}

	fmt.Fprintf(out, "update: seam reconciled %d file(s) in %s\n", len(report.Outcomes), abs)
	fmt.Fprintln(out, summarizeOutcomes(report.Outcomes))
	if sp := summarizeProposals(report.Proposals); sp != "" {
		fmt.Fprintln(out, sp)
	}
	if report.LineagePath != "" {
		fmt.Fprintf(out, "lineage: %s\n", report.LineagePath)
	}
	// Preserved orphan overlay skills (P1-LINEAGE-002). A non-empty
	// report.Orphans means previously-rendered skill files whose overlay source
	// was removed are still sitting on disk. Two modes:
	//   - default (report-only): list them, leave them in place (nothing deleted);
	//   - --prune-orphans: delete the byte-identical (DestUnchanged) ones and
	//     refuse the hand-edited (DestModified) ones for manual `rm`. A
	//     project-owned orphan is NEVER deleted even with the flag (safety floor
	//     mirroring uninstall --force). See applyPruneOrphans for the contract.
	if updatePruneOrphans {
		applyPruneOrphans(out, abs, report.Orphans, false)
	} else if n := len(report.Orphans); n > 0 {
		fmt.Fprintf(out, "\nPreserved orphan skill file(s) — %d previously-rendered overlay skill file(s) whose source was removed; left in place (report-only, NOT deleted):\n", n)
		for _, o := range report.Orphans {
			fmt.Fprintf(out, "  %s  [%s, from pack %q, source %q]\n", o.DestinationPath, o.DestinationState, o.OverlayPack, o.SourceRelativePath)
		}
		fmt.Fprintln(out, "Remove the file listed above if you no longer want it; remove the whole skill directory only after verifying EVERY file in it is orphaned. Or restore the overlay source to clear this notice. Pass --prune-orphans to auto-delete the byte-identical ones (hand-edited or project-owned files are always refused).")
	}
	// Skill-cache staleness (D1): opencode caches the discovered skill list in a
	// module-closure Map that is cleared ONLY by process death. A running
	// opencode session will therefore NOT see skills this update added or
	// changed under .opencode/skills/ until the operator restarts it. The
	// harness cannot fix that upstream cache; the durable mitigation is to hint
	// at the exact moment the staleness was caused. Gate on an actually-written
	// skill path (WriteSucceeded) so a pure reconcile that touched no skill bytes
	// stays quiet.
	if updateTouchedSkills(report.Outcomes) {
		fmt.Fprintln(out, "\nrestart opencode to see newly added/changed skills (opencode caches the skill list per-process; only a restart picks up .opencode/skills/ changes).")
	}
	printNextStepsFooter(out, abs)
	return nil
}

// updateTouchedSkills reports whether the apply actually wrote a file under
// .opencode/skills/ (D1 restart-hint trigger). It gates on WriteSucceeded so a
// pure managed-noop reconcile (byte-identical, no write) stays quiet — the
// skill cache staleness only matters when skill bytes materially changed.
func updateTouchedSkills(outcomes []substrate.FileOutcome) bool {
	for _, o := range outcomes {
		if o.WriteState == substrate.WriteSucceeded && strings.HasPrefix(o.Path, ".opencode/skills/") {
			return true
		}
	}
	return false
}

// pruneCounts tallies a --prune-orphans pass for the closing summary.
type pruneCounts struct {
	pruned  int // deleted (live) or would-be-deleted (dry-run) DestUnchanged files
	refused int // DestModified, project-owned, classifier-unavailable, or delete-failed → manual rm
	skipped int // DestMissing (already gone)
}

// pruneClassifier builds a read-only ownership classifier for the prune safety
// floor: the core ownership defaults resolved against the project's raise-only
// overrides, with NO overlay-extension rules (the orphan's source was removed,
// so it is not part of this run's rendered overlayFiles anyway). It is used ONLY
// to refuse a project-owned orphan defensively — it never authorizes a delete
// on its own. By construction every orphan is platform-controlled (the
// rendered-outputs manifest records only harness-rendered overlay skill files,
// never project-owned paths, and ownership.Resolve rejects an override that
// targets an unknown path, so an override claiming the now-sourceless orphan
// path would have aborted the apply before prune ran). This classifier is the
// belt-and-suspenders guard that makes that guarantee explicit at the delete
// site, mirroring uninstall --force's project-owned retention. A build/resolve
// error makes the whole prune refuse-safe (see applyPruneOrphans).
func pruneClassifier(target string) (*substrate.Classifier, error) {
	overrides, err := readOwnershipOverrides(target)
	if err != nil {
		return nil, fmt.Errorf("read ownership overrides: %w", err)
	}
	cls, err := seamClassifierWithOverlays(nil, overrides)
	if err != nil {
		return nil, fmt.Errorf("ownership resolve: %w", err)
	}
	return cls, nil
}

// isProjectOwnedOrphan is the safety-floor predicate. It returns true ONLY when
// the classifier positively resolves the orphan path to project_owned. An
// unclassified path (ok=false, the normal case for a sourceless custom skill
// path) is NOT project-owned and stays eligible for prune — consistent with the
// structural guarantee that every orphan is platform-controlled.
func isProjectOwnedOrphan(cls *substrate.Classifier, dest string) bool {
	if cls == nil {
		return false
	}
	if c, ok := cls.Classify(dest); ok && c.Class == ownership.ClassProjectOwned {
		return true
	}
	return false
}

// orphanPathEscapesTarget reports whether destPath — a manifest record's
// forward-slashed DestinationPath — resolves OUTSIDE target under the SAME
// filepath.Join(target, filepath.FromSlash(destPath)) expression that
// applyPruneOrphans uses to compute the os.Remove target. This is the
// path-traversal safety guard for --prune-orphans (B1): a malicious or corrupt
// manifest record carrying a traversal destination (e.g. "../../victim") whose
// source is missing AND whose recorded digest matches an external file would
// otherwise be classified DestUnchanged and os.Remove'd OUTSIDE the project.
// The guard refuses such a path (report for manual review, delete nothing).
//
// Absolute destinations are rejected outright even though filepath.Join would
// nest them under target (Join concatenates rather than resetting on an
// absolute element): they are never legitimate repo-relative destinations, and
// refusing them here keeps the guard honest instead of depending on Join's
// concatenation quirk.
//
// The check is deliberately LEXICAL. We control the join — no symlink is
// followed to resolve destPath — so the containment decision must rest on the
// exact path string we will hand to os.Remove, not on whatever the filesystem
// currently resolves it to. os.Stat / filepath.EvalSymlinks are intentionally
// NOT used: they would introduce a TOCTOU window and could follow a planted
// symlink, defeating a lexical guard. Both target and the resolved candidate
// are filepath.Clean'd so the comparison is canonical.
func orphanPathEscapesTarget(target, destPath string) bool {
	if filepath.IsAbs(filepath.FromSlash(destPath)) {
		return true
	}
	cleanTarget := filepath.Clean(target)
	resolved := filepath.Clean(filepath.Join(target, filepath.FromSlash(destPath)))
	rel, err := filepath.Rel(cleanTarget, resolved)
	if err != nil {
		return true // cannot relate — refuse-safe (treat as escape)
	}
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// applyPruneOrphans consumes the report-only orphan findings under the
// --prune-orphans flag and either deletes them (live) or previews the action
// (dry-run). The safe-delete contract:
//
//   - DestUnchanged (rendered bytes byte-identical to the recorded render
//     digest → genuinely dead, no operator content): DELETE in live,
//     "would delete" in dry-run. Refused instead when the destination
//     resolves OUTSIDE target (path-traversal safety guard, B1), the path
//     resolves project-owned (safety floor), or the ownership classifier
//     could not be built (refuse-safe).
//   - DestModified (operator hand-edited the rendered file): ALWAYS refused and
//     reported for manual `rm` — never auto-deleted.
//   - DestMissing (already gone): skipped.
//
// Nothing outside report.Orphans is ever touched: non-orphan and project-owned
// files never enter this set (the detection layer + ownership.Resolve guarantee
// it), and this function additionally checks ownership before each delete.
func applyPruneOrphans(out io.Writer, target string, findings []renderstate.OrphanFinding, dryRun bool) pruneCounts {
	var counts pruneCounts
	if len(findings) == 0 {
		return counts
	}
	cls, clsErr := pruneClassifier(target)
	classifierOK := clsErr == nil
	if !classifierOK {
		fmt.Fprintf(out, "\nprune-orphans: ownership classifier unavailable (%v); refusing all orphans for manual removal — nothing deleted.\n", clsErr)
	}
	verb := "would prune"
	if !dryRun {
		verb = "pruning"
	}
	fmt.Fprintf(out, "\n--prune-orphans %s %d orphan file(s):\n", verb, len(findings))
	for _, o := range findings {
		switch o.DestinationState {
		case renderstate.DestMissing:
			counts.skipped++
			fmt.Fprintf(out, "  skip (already gone): %s\n", o.DestinationPath)
		case renderstate.DestModified:
			counts.refused++
			fmt.Fprintf(out, "  refuse (hand-edited; remove manually): %s  [from pack %q]\n", o.DestinationPath, o.OverlayPack)
		case renderstate.DestUnchanged:
			// B1 path-containment guard (runs before the dry-run/live split so a
			// traversal destination is refused in BOTH modes): a destination that
			// resolves outside target must NEVER reach os.Remove, even when its
			// recorded digest matches an external file. Refuse it for manual
			// review like a DestModified finding — refuse-safe, deletes nothing.
			if orphanPathEscapesTarget(target, o.DestinationPath) {
				counts.refused++
				fmt.Fprintf(out, "  refuse (destination escapes target — possible path traversal; remove manually): %s\n", o.DestinationPath)
				continue
			}
			if !classifierOK || isProjectOwnedOrphan(cls, o.DestinationPath) {
				counts.refused++
				fmt.Fprintf(out, "  refuse (project-owned or classifier unavailable; remove manually): %s\n", o.DestinationPath)
				continue
			}
			if dryRun {
				fmt.Fprintf(out, "  would delete (byte-identical to recorded render): %s\n", o.DestinationPath)
				counts.pruned++
				continue
			}
			live := filepath.Join(target, filepath.FromSlash(o.DestinationPath))
			if err := os.Remove(live); err != nil {
				counts.refused++
				fmt.Fprintf(out, "  refuse (delete failed: %v; remove manually): %s\n", err, o.DestinationPath)
				continue
			}
			fmt.Fprintf(out, "  deleted (byte-identical to recorded render): %s\n", o.DestinationPath)
			counts.pruned++
		default:
			// An unexpected/future DestinationState must never auto-delete.
			counts.refused++
			fmt.Fprintf(out, "  refuse (unknown on-disk state %q; remove manually): %s\n", o.DestinationState, o.DestinationPath)
		}
	}
	deletedWord := "deleted"
	if dryRun {
		deletedWord = "would be deleted"
	}
	fmt.Fprintf(out, "prune-orphans summary: %d file(s) %s, %d refused for manual removal, %d skipped\n",
		counts.pruned, deletedWord, counts.refused, counts.skipped)
	return counts
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
