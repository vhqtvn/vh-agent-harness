package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vhqtvn/vh-agent-harness/internal/schema"
	"github.com/vhqtvn/vh-agent-harness/internal/substrate"
)

// installFlags holds the project answers + target for `vh-agent-harness install`.
//
// Slice 2 replaces the Copier-era sentinel-token model ({{PROJECT_NAME}} etc.
// replaced via targeted string substitution by the old installer) with the
// Go-native renderer: answers are exposed as a nested data context consumed by
// text/template conditionals ({{ if .features.backlog }}) in *.tmpl files, and
// as a flat map for the lineage answer digest. Only project_name / project_slug
// remain as first-class answers; every other render-time decision is owned by
// the platform template (managed) or the schema (armed).
type installFlags struct {
	name   string
	slug   string
	target string
	dryRun bool
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

Config docs/templates are NOT scattered into the tree as *.example files; run
` + "`vh-agent-harness example <path>`" + ` to print one on demand.

Re-running install over the same target is idempotent for managed files and a
no-op reconcile for armed files that already match. It also seeds a default
S4 run-shape (<target>/.vh-agent-harness/run-shape.yml with
runtime.backend: host-shell) when none exists (S4 is project_owned, so an
existing file is never clobbered). This is what makes the runtime verbs
(exec/shell/up/down/logs/ps/status) resolve a backend post-install.`,
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
}

func runInstall(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()

	target, err := filepath.Abs(installFl.target)
	if err != nil {
		return fmt.Errorf("resolve target: %w", err)
	}

	answers := map[string]string{
		"project_name": installFl.name,
		"project_slug": installFl.slug,
	}

	report, err := seamApply(target, answers, installFl.dryRun)
	if err != nil {
		return err
	}

	if installFl.dryRun {
		printDryRunPlan(out, "install", target, report)
		return nil
	}

	fmt.Fprintf(out, "install: seam applied %d file(s) into %s\n", len(report.Outcomes), target)
	fmt.Fprintln(out, summarizeOutcomes(report.Outcomes))
	if sp := summarizeProposals(report.Proposals); sp != "" {
		fmt.Fprintln(out, sp)
	}
	fmt.Fprintf(out, "lineage: %s\n", report.LineagePath)
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
