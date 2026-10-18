package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vhqtvn/vh-agent-harness/internal/managedfile"
	"github.com/vhqtvn/vh-agent-harness/internal/schema"
	"github.com/vhqtvn/vh-agent-harness/internal/substrate"
)

// installFlags holds the project answers + target + bootstrap selector for
// `vh-agent-harness install`.
//
// Slice 2 replaces the Copier-era sentinel-token model ({{PROJECT_NAME}} etc.
// replaced via targeted string substitution by the old installer) with the
// Go-native renderer: answers are exposed as a nested data context consumed by
// text/template conditionals ({{ if .features.backlog }}) in *.tmpl files, and
// as a flat map for the lineage answer digest. Only project_name / project_slug
// remain as first-class answers; every other render-time decision is owned by
// the platform template (managed) or the schema (armed).
//
// Bootstrap selectors (greenfield-only, mutually exclusive):
//
//	(no selector) → frozen FULL recipe (templates/install/full-harness-profile.yml):
//	                supervised preset + every shipped core capability + every
//	                shipped overlay pack. This is the greenfield default.
//	--full        → the same frozen full recipe, explicitly.
//	--minimal     → the historical minimal seed (embedded core default).
//	--profile P   → the embedded default seeded with preset P
//	                (minimal|supervised|coordination|web).
//
// Any selector against a target with an EXISTING live profile is rejected
// before any write; a no-selector install over an existing install keeps the
// historical reconcile semantics (live selection preserved).
type installFlags struct {
	name    string
	slug    string
	target  string
	dryRun  bool
	full    bool
	minimal bool
	profile string
}

// newInstallFlags returns the flag set with defaults resolved against cwd
// (slug defaults to the current directory's basename).
func newInstallFlags() *installFlags {
	f := &installFlags{name: "My Project"}
	if cwd, err := defaultCwdBasename(); err == nil && cwd != "" {
		f.slug = cwd
	} else {
		f.slug = "my-project"
	}
	f.target = "."
	return f
}

var installFl *installFlags

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the agent harness into the current project (seam render + apply)",
	Long: `Render the embedded core corpus into a target directory through the
substrate seam and write the S1 lineage record at
<target>/.vh-agent-harness/lineage.yml.

The seam is the validated render/apply pipeline: it renders the corpus into an
out-of-tree staging dir, classifies every file via the S2 ownership map,
plans all per-class outcomes fail-closed BEFORE any write, then applies:
  - platform_managed files are written (free-overwrite on update);
  - platform_armed files (vh-harness-profile.yml) are seeded from the validated
    platform default, then schema-reconciled on subsequent runs;
  - project_owned files (.gitignore, README.md, CLAUDE.md, Makefile,
    forbidden-patterns.project.js) are seeded once and preserved thereafter.

Bootstrap selection (GREENFIELD targets only — a target with no live
.vh-agent-harness/vh-harness-profile.yml):
  - no selector  → the frozen FULL recipe: supervised preset + every shipped
    core capability + every shipped overlay pack (the greenfield default);
  - --full       → the same frozen full recipe, pinned explicitly;
  - --minimal    → the historical minimal seed (the 8-agent baseline plus the
    default-on shipped pilots);
  - --profile P  → the embedded default seeded with preset P
    (minimal|supervised|coordination|web).
Selectors are mutually exclusive and are REJECTED on a target that already has
a live profile — edit .vh-agent-harness/vh-harness-profile.yml and run
` + "`vh-agent-harness update`" + ` instead. A no-selector install over an existing
install keeps the live selection untouched (reconcile semantics).

Config docs/templates are NOT scattered into the tree as *.example files; run
` + "`vh-agent-harness example <path>`" + ` to print one on demand.

Re-running install over the same target is idempotent for managed files and a
no-op reconcile for armed files that already match. It also seeds a default
S4 run-shape (<target>/.vh-agent-harness/run-shape.yml with
runtime.backend: host-shell) when none exists (S4 is project_owned, so an
existing file is never clobbered). This is what makes the runtime verbs
(exec/shell/up/down/logs/ps/status) resolve a backend post-install.

An install that applies incompletely (live write failures) reports the partial
state and exits non-zero; lineage is not advanced in that case.`,
	Args: cobra.NoArgs,
	RunE: runInstall,
}

func init() {
	installFl = newInstallFlags()
	installCmd.Flags().StringVar(&installFl.name, "name", installFl.name,
		"project display name (rendered into *.tmpl as .project_name)")
	installCmd.Flags().StringVar(&installFl.slug, "slug", installFl.slug,
		"dir/container/service slug (rendered into *.tmpl as .project_slug)")
	installCmd.Flags().StringVarP(&installFl.target, "target", "o", installFl.target,
		"install destination directory (default: current directory)")
	installCmd.Flags().BoolVar(&installFl.dryRun, "dry-run", false,
		"preview the per-file plan without writing anything")
	installCmd.Flags().BoolVar(&installFl.full, "full", false,
		"greenfield only: seed the frozen FULL recipe (supervised + every shipped capability and overlay); rejected on an existing install")
	installCmd.Flags().BoolVar(&installFl.minimal, "minimal", false,
		"greenfield only: seed the historical MINIMAL profile (8-agent baseline); rejected on an existing install")
	installCmd.Flags().StringVar(&installFl.profile, "profile", "",
		"greenfield only: seed the embedded default with preset "+strings.Join(knownInstallPresets(), "|")+"; rejected on an existing install")
}

// knownInstallPresets returns the selectable `profile:` presets, sorted. It
// mirrors the profileCapabilityPresets key set (the enum the schema validates).
func knownInstallPresets() []string {
	out := make([]string, 0, len(profileCapabilityPresets))
	for k := range profileCapabilityPresets {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// resolveInstallBootstrap validates the selector combination and resolves the
// greenfield bootstrap bytes (nil when the run must keep historical semantics:
// an existing install with no selector). All usage failures are returned as
// errors BEFORE any write or render: conflicting selectors, unknown presets,
// and selectors against an existing (or unreadable/malformed) live profile.
//
// The returned source string names where the selection came from, for the
// effective-selection report ("frozen full recipe (greenfield default)",
// "flag --full", "flag --minimal", "flag --profile <p>", or
// "existing live profile (preserved)").
func resolveInstallBootstrap(target string, fl *installFlags) (bootstrap []byte, source string, err error) {
	// Selector presence is recognized by explicit flag value, not by resulting
	// selection (an empty --profile="" still counts as given for conflict
	// purposes via cobra's Changed, which runInstall checks; here we count the
	// value-bearing forms).
	selectors := 0
	var named []string
	if fl.full {
		selectors++
		named = append(named, "--full")
	}
	if fl.minimal {
		selectors++
		named = append(named, "--minimal")
	}
	if fl.profile != "" {
		selectors++
		named = append(named, "--profile")
	}
	if selectors > 1 {
		return nil, "", fmt.Errorf("install: bootstrap selectors are mutually exclusive; got %s (pick one of --full, --minimal, --profile <preset>, or none)",
			strings.Join(named, " + "))
	}

	livePath := filepath.Join(target, harnessProfileName)
	raw, readErr := os.ReadFile(livePath)
	if readErr != nil && !os.IsNotExist(readErr) {
		// The live profile EXISTS but cannot be read (permissions, EISDIR, …).
		// Fail closed for EVERY path below — selector and default alike — so an
		// undecidable live state can never be mistaken for greenfield and
		// seeded over (pre-write, same as every other usage failure here).
		return nil, "", fmt.Errorf("install: cannot read live profile %s: %w", livePath, readErr)
	}
	livePresent := readErr == nil

	if selectors > 0 && livePresent {
		// A selector demands to know exactly what would be seeded; a live
		// profile we cannot validate makes that undecidable — reject rather
		// than guess (fail-closed before any write).
		if errs := validateBootstrapProfile(raw); len(errs) > 0 {
			return nil, "", fmt.Errorf("install: selector %s refused: the live profile at %s is unreadable or malformed (%v); fix or remove it deliberately before selecting a bootstrap",
				named[0], livePath, errs)
		}
		return nil, "", fmt.Errorf("install: selector %s refused: a live profile already exists at %s; existing installs keep their selection — edit the profile and run `vh-agent-harness update` instead",
			named[0], livePath)
	}

	switch {
	case fl.full:
		b, err := bootstrapRecipeBytes()
		if err != nil {
			return nil, "", err
		}
		return b, "flag --full (frozen full recipe)", nil
	case fl.minimal:
		b, err := corpusDefaultProfileBytes()
		if err != nil {
			return nil, "", err
		}
		if errs := validateBootstrapProfile(b); len(errs) > 0 {
			return nil, "", fmt.Errorf("install: --minimal: embedded default is schema-invalid: %v", errs)
		}
		return b, "flag --minimal (historical minimal seed)", nil
	case fl.profile != "":
		if _, ok := profileCapabilityPresets[fl.profile]; !ok {
			return nil, "", fmt.Errorf("install: unknown --profile preset %q; known presets: %s",
				fl.profile, strings.Join(knownInstallPresets(), ", "))
		}
		b, err := presetProfileBytes(fl.profile)
		if err != nil {
			return nil, "", err
		}
		return b, fmt.Sprintf("flag --profile %s", fl.profile), nil
	default:
		if livePresent {
			// No selector + existing install: historical semantics. The live
			// selection is preserved by the armed reconcile; no bootstrap.
			return nil, "existing live profile (preserved)", nil
		}
		// Greenfield no-selector default: the frozen FULL recipe.
		b, err := bootstrapRecipeBytes()
		if err != nil {
			return nil, "", err
		}
		return b, "greenfield default (frozen full recipe; pin with --full)", nil
	}
}

// shellQuote wraps s in single quotes POSIX-safely so replay commands can be
// pasted verbatim into a shell. Values are operator-supplied (never trusted).
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// installReplayCommand renders the shell-safe replay of THIS install
// invocation, preserving target and identity flags plus the selector. A
// default-full greenfield install pins `--full` explicitly so the replay stays
// stable against any future default change.
func installReplayCommand(fl *installFlags, selectorPinnedFull bool) string {
	parts := []string{"vh-agent-harness", "install",
		"--name", shellQuote(fl.name),
		"--slug", shellQuote(fl.slug),
		"--target", shellQuote(fl.target)}
	switch {
	case fl.full:
		parts = append(parts, "--full")
	case fl.minimal:
		parts = append(parts, "--minimal")
	case fl.profile != "":
		parts = append(parts, "--profile", shellQuote(fl.profile))
	case selectorPinnedFull:
		parts = append(parts, "--full")
	}
	return strings.Join(parts, " ")
}

// printInstallSelection reports the EFFECTIVE selection for the run so an
// operating agent sees exactly what was (or will be) seeded, plus its source.
// Works identically on dry-run (bootstrap bytes, live profile absent) and apply
// (live profile seeded) paths because it reads through the same
// bootstrap-aware readers the render used.
func printInstallSelection(out io.Writer, target string, bootstrap []byte, source string) {
	profile, explicitCaps, explicitOverlays := "", []string{}, []string{}
	if len(bootstrap) > 0 {
		profile, explicitCaps, explicitOverlays = parseBootstrapSelection(bootstrap)
	} else {
		if raw, err := os.ReadFile(filepath.Join(target, harnessProfileName)); err == nil {
			profile, explicitCaps, explicitOverlays = parseBootstrapSelection(raw)
		}
	}
	// Effective capability selection (preset ∪ explicit), computed by the same
	// reader the render used.
	sel := readProfileSelectionWithBootstrap(target, bootstrap)
	caps := make([]string, 0, len(sel))
	for _, id := range sel {
		caps = append(caps, string(id))
	}
	overlays := activeOverlaysWithBootstrap(target, bootstrap)

	fmt.Fprintf(out, "selection (source: %s):\n", source)
	if profile != "" {
		fmt.Fprintf(out, "  profile:      %s\n", profile)
	}
	if len(caps) > 0 {
		fmt.Fprintf(out, "  capabilities: %s\n", strings.Join(caps, ", "))
	} else if len(explicitCaps) > 0 {
		fmt.Fprintf(out, "  capabilities: (unresolved) %s\n", strings.Join(explicitCaps, ", "))
	}
	if len(overlays) > 0 {
		fmt.Fprintf(out, "  overlays:     %s\n", strings.Join(overlays, ", "))
	} else if len(explicitOverlays) > 0 {
		fmt.Fprintf(out, "  overlays:     (not yet rendered) %s\n", strings.Join(explicitOverlays, ", "))
	} else {
		fmt.Fprintln(out, "  overlays:     (none)")
	}
}

func runInstall(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()

	target, err := filepath.Abs(installFl.target)
	if err != nil {
		return fmt.Errorf("resolve target: %w", err)
	}

	// Treat an explicitly-passed-but-empty --profile "" as a given selector
	// (cobra flag-presence, not value): it must conflict-count like the others.
	if cmd.Flags().Changed("profile") && installFl.profile == "" {
		return fmt.Errorf("install: --profile requires a preset name (known: %s)", strings.Join(knownInstallPresets(), ", "))
	}

	// Resolve + validate the bootstrap selection BEFORE any write or render:
	// usage errors (conflicting selectors, unknown presets, selector-vs-live
	// profile) fail closed here.
	bootstrap, selectionSource, err := resolveInstallBootstrap(target, installFl)
	if err != nil {
		return err
	}

	answers := map[string]string{
		"project_name": installFl.name,
		"project_slug": installFl.slug,
	}

	// Warn loudly when project.config.json is absent or a consumed token resolves
	// empty (W3): previously this was silent and a consumer shipped a CLAUDE.md
	// with blank sections. Non-fatal; emitted to stderr so it appears alongside
	// both the --dry-run plan (stdout) and a real apply.
	warnUnresolvedProjectConfigTokens(os.Stderr, target)

	printInstallSelection(out, target, bootstrap, selectionSource)

	report, err := seamApply(target, answers, installFl.dryRun, bootstrap)
	if err != nil {
		return err
	}

	if installFl.dryRun {
		printDryRunPlan(out, "install", target, report)
		return nil
	}

	if _, err := materializeContextDocs(target); err != nil {
		return fmt.Errorf("materialize always-on context docs: %w", err)
	}

	fmt.Fprintf(out, "install: seam applied %d file(s) into %s\n", len(report.Outcomes), target)
	fmt.Fprintln(out, summarizeOutcomes(report.Outcomes))
	if sp := summarizePreservedPaths(report.Outcomes); sp != "" {
		fmt.Fprintln(out, sp)
	}
	if sp := summarizeProposals(report.Proposals); sp != "" {
		fmt.Fprintln(out, sp)
	}
	fmt.Fprintf(out, "replay: %s\n", installReplayCommand(installFl, len(bootstrap) > 0 && !installFl.full && !installFl.minimal && installFl.profile == ""))
	// Lineage advance is gated on a fully-applied generation (P1-SUBSTRATE-001):
	// when any live write failed, substrate.Apply did NOT write lineage and
	// LineagePath is "". Surface that distinctly instead of printing an empty
	// path, so the operator knows the install did not record a successful render.
	if !report.GenerationFullyApplied {
		failed := failedWriteOutcomes(report.Outcomes)
		paths := make([]string, len(failed))
		for i, o := range failed {
			paths[i] = o.Path
		}
		fmt.Fprintf(out, "incomplete: %d live write(s) failed (%s); lineage was NOT advanced and the install is PARTIAL (no rollback was attempted — the writes that landed are real). Fix the failing write and re-run.\n",
			len(failed), strings.Join(paths, ", "))
		printNextStepsFooter(out, target)
		// Incomplete install is a FAILURE exit (agent UX contract): a live
		// install that applied partially must not read as success to an
		// automated operator. The partial state was reported above; lineage
		// was not advanced. (Scoped to install; update keeps its contract.)
		return fmt.Errorf("install: applied incompletely (%d failed live write(s)); lineage not advanced", len(failed))
	}
	if report.LineagePath != "" {
		fmt.Fprintf(out, "lineage: %s\n", report.LineagePath)
	}
	printNextStepsFooter(out, target)
	return nil
}

// summarizeOutcomes tallies FileOutcome actions into a one-line human summary.
// It is shared by install and update so both report the same way.
func summarizeOutcomes(outcomes []substrate.FileOutcome) string {
	counts := map[substrate.FileAction]int{}
	for _, o := range outcomes {
		counts[o.Action]++
	}
	// Stable order for readability.
	order := []substrate.FileAction{
		substrate.ActionManagedOverwrite,
		substrate.ActionManagedNoop,
		substrate.ActionManagedDiverged,
		substrate.ActionProjectSeeded,
		substrate.ActionProjectPreserved,
		substrate.ActionArmedMerged,
		substrate.ActionArmedNoop,
		substrate.ActionArmedProposal,
		substrate.ActionUnsupportedClass,
		substrate.ActionIgnoredLocal,
	}
	var parts []string
	for _, a := range order {
		if n := counts[a]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, a))
		}
	}
	return "outcomes: " + strings.Join(parts, ", ")
}

// summarizePreservedPaths lists each preserved/stalled managed path WITH its
// typed reason from the shared managedfile.PreservedReason taxonomy, in LIVE
// install/update output (not just dry-run/doctor). It makes the stall state
// VISIBLE and ACTIONABLE: the operator sees exactly which path(s) update did NOT
// overwrite and why, and the accept-platform operation that resolves each.
//
// Only ActionManagedDiverged outcomes with a non-empty PreservedReason are
// listed — the non-preserved outcomes (normal overwrite, self-healed origin,
// regenerated, managed-noop) are NOT stalls and are intentionally absent. The
// reasons are grouped and ordered by the taxonomy's stable enum order so the
// surface is deterministic across runs.
//
// This is the F2 live-UX counterpart to doctor's non-failing stall report: both
// consume the SAME shared typed-reason model (managedfile.PreservedReason) so
// they never disagree on why a path stalled.
func summarizePreservedPaths(outcomes []substrate.FileOutcome) string {
	var stalled []substrate.FileOutcome
	for _, o := range outcomes {
		if o.Action == substrate.ActionManagedDiverged && o.PreservedReason != "" {
			stalled = append(stalled, o)
		}
	}
	if len(stalled) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("preserved/stalled managed file(s) — %d path(s) NOT overwritten; run `vh-agent-harness accept-platform <path>` to adopt the platform version (writes platform bytes + advances the origin, live-first then sidecar rename):",
		len(stalled)))
	// Group by reason in the taxonomy's stable order for a deterministic surface.
	for _, reason := range []managedfile.PreservedReason{
		managedfile.ConsumerEdit, managedfile.ConsumerDelete,
		managedfile.Unreadable, managedfile.UnknownBaseline,
	} {
		var paths []string
		for _, o := range stalled {
			if o.PreservedReason == reason {
				paths = append(paths, o.Path)
			}
		}
		if len(paths) == 0 {
			continue
		}
		sb.WriteString(fmt.Sprintf("\n  [%s] %s", reason, strings.Join(paths, ", ")))
	}
	return sb.String()
}

// summarizeProposals returns a short human list of armed-proposal conflicts so
// the operator knows which armed file needs a decision (and that the project
// instance was left untouched). Kept as a helper so update can share it.
func summarizeProposals(proposals []schema.Proposal) string {
	if len(proposals) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("proposals (needs-decision; armed files left untouched):")
	for _, p := range proposals {
		sb.WriteString(fmt.Sprintf("\n  - %s: %s (platform=%v project=%v envelope=%q)",
			p.Field, p.Kind, p.PlatformValue, p.ProjectValue, p.Envelope))
	}
	return sb.String()
}
