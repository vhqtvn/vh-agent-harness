package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vhqtvn/vh-agent-harness/internal/runshape"
	"github.com/vhqtvn/vh-agent-harness/internal/substrate"
)

// guide is the agent-facing orientation command. An agent (or human) pointed at
// a repo runs `vh-agent-harness guide` to learn, from the binary itself, what
// state the harness is in and exactly what to do next — install, configure,
// adopt an existing harness, or update. Every step is a concrete command or
// file edit, so an automated operator can act without external docs.
//
// --json emits the same state + steps as a machine-readable object.

// harnessState is the detected situation of the cwd's project.
type harnessState struct {
	ProjectRoot string `json:"project_root,omitempty"`
	// Phase is the top-level situation: "greenfield" (no harness), "adoptable"
	// (an existing .opencode harness not yet managed by vh-agent-harness), or
	// "installed".
	Phase          string   `json:"phase"`
	Overlays       []string `json:"overlays,omitempty"`
	RuntimeBackend string   `json:"runtime_backend,omitempty"`
	HasMission     bool     `json:"has_mission"`
}

const (
	phaseGreenfield = "greenfield"
	phaseAdoptable  = "adoptable"
	phaseInstalled  = "installed"
)

var guideJSON bool

var guideCmd = &cobra.Command{
	Use:           "guide",
	Short:         "Show what the harness needs next here (agent-facing: install/configure/update)",
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `Orient yourself (or an automated agent) in the current repo: detect the
harness state and print the exact next steps — install, configure, adopt an
existing harness, or update. Use --json for machine-readable output.

This is the recommended first command when pointing a fresh agent session at a
repo: it teaches how to operate the harness from the binary itself.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		st := detectHarnessState(cwd)
		// Warn loudly when project.config.json tokens resolve empty (W3). Emitted
		// to stderr so it is out-of-band from --json stdout and surfaces the
		// incomplete-render risk before the operator installs/updates. Non-fatal.
		warnUnresolvedProjectConfigTokens(os.Stderr, cwd)
		steps := nextSteps(st)
		if guideJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
				harnessState
				NextSteps []string `json:"next_steps"`
			}{st, steps})
		}
		writeGuide(cmd.OutOrStdout(), st, steps)
		return nil
	},
}

func init() {
	guideCmd.Flags().BoolVar(&guideJSON, "json", false, "emit state + next steps as JSON")
}

// detectHarnessState inspects the cwd (walking up for an install root) and
// classifies the project. An install is authoritative when a run-shape resolves
// (the seam always seeds one). Absent that, an existing .opencode tree means the
// repo is adoptable (the migrate path); otherwise it is greenfield.
func detectHarnessState(cwd string) harnessState {
	if root, rs, err := runshape.FindForRoot(cwd); err == nil && root != "" {
		st := harnessState{Phase: phaseInstalled, ProjectRoot: root}
		if rs != nil && rs.Runtime != nil {
			st.RuntimeBackend = rs.Runtime.Backend
		}
		st.Overlays = activeOverlays(root)
		st.HasMission = isRegularFile(filepath.Join(root, runshape.DirName, "AGENTS.mission.md"))
		return st
	}
	if isExistingDir(filepath.Join(cwd, ".opencode")) {
		return harnessState{Phase: phaseAdoptable, ProjectRoot: cwd}
	}
	return harnessState{Phase: phaseGreenfield, ProjectRoot: cwd}
}

// nextSteps returns the ordered, concrete actions for the detected state. Each
// entry is a self-contained instruction (command or file edit) an agent can run.
func nextSteps(st harnessState) []string {
	switch st.Phase {
	case phaseGreenfield:
		return []string{
			"Optional but recommended BEFORE install: create `.vh-agent-harness/project.config.json` " +
				"so the seeded CLAUDE.md/Makefile pick up your project values. Print the template with " +
				"`vh-agent-harness example .vh-agent-harness/project.config.json > .vh-agent-harness/project.config.json`, " +
				"then fill `mission_summary`/`architecture_summary` (and `db_user`/`db_name` if used). " +
				"Those seeds are written ONCE, so filling the config first avoids blank sections.",
			"Preview first (optional): `vh-agent-harness install --name <ProjectName> --slug <project-slug> --dry-run` " +
				"shows the per-file plan without writing anything.",
			"Install the harness here: `vh-agent-harness install --name <ProjectName> --slug <project-slug>` " +
				"(seeds .vh-agent-harness/ config, the .opencode/ agent corpus, and a default host-shell run-shape).",
			"Then run `vh-agent-harness guide` again for configuration steps.",
		}
	case phaseAdoptable:
		return []string{
			"This repo has an existing .opencode harness but is NOT yet managed by vh-agent-harness. " +
				"Preview the adoption safely first: `vh-agent-harness install --name <ProjectName> " +
				"--slug <project-slug> --dry-run` — it lists which files would be overwritten, seeded, or " +
				"preserved WITHOUT writing anything.",
			"Adopt it: `vh-agent-harness install --name <ProjectName> --slug <project-slug>`. " +
				"Generic managed files are refreshed; your project-owned files (.gitignore, README.md, " +
				"CLAUDE.md, Makefile, plus any AGENTS.md) are preserved.",
			"Review what the install changed: `vh-agent-harness diff` and `vh-agent-harness doctor`.",
			"Move domain agents/commands/skills into an overlay pack at .vh-agent-harness/overlays/<name>/ " +
				"and select it under `overlays:` in .vh-agent-harness/vh-harness-profile.yml, then re-run " +
				"`vh-agent-harness update`.",
		}
	default: // installed
		var steps []string
		if !st.HasMission {
			steps = append(steps, "Describe this project: "+
				"`vh-agent-harness example .vh-agent-harness/AGENTS.mission.md > .vh-agent-harness/AGENTS.mission.md`, "+
				"fill in mission/architecture/product rules, then `vh-agent-harness update` "+
				"(composes the agent-facing AGENTS.md = AGENTS.core.md + your mission).")
		}
		if len(st.Overlays) == 0 {
			steps = append(steps, "Add project-specific agents/commands/skills via an overlay pack: "+
				"create .vh-agent-harness/overlays/<name>/ with agents/<name>.md + opencode-append.jsonc "+
				"(and optional permission-pack.jsonc), list `<name>` under `overlays:` in "+
				".vh-agent-harness/vh-harness-profile.yml, then `vh-agent-harness update` "+
				"(see `/harness` for the full recipe; `vh-agent-harness example` for a pack skeleton).")
		} else {
			steps = append(steps, "Active overlays: "+strings.Join(st.Overlays, ", ")+
				" (pack sources under .vh-agent-harness/overlays/). Edit the pack "+
				"(agents/<name>.md, opencode-append.jsonc, optional permission-pack.jsonc) and run "+
				"`vh-agent-harness update` to apply. See `/harness` for the full recipe.")
		}
		if st.RuntimeBackend == "host-shell" || st.RuntimeBackend == "" {
			steps = append(steps, "Runtime is host-shell (commands run on the host). To run in a container or via "+
				"your own wrapper, edit .vh-agent-harness/run-shape.yml: `backend: docker_compose` (set compose_file/"+
				"default_service) OR `backend: proxy` + `proxy_command: [\"./dev.sh\", \"exec\"]` to delegate to an "+
				"existing script.")
		}
		steps = append(steps,
			"Project deny-rules go in .opencode/repo-configs/forbidden-patterns.project.js "+
				"(import builders from ./forbidden-patterns.core.js; every rule needs a `why`).",
			"After editing config or installing a new binary: `vh-agent-harness update` (preview with "+
				"`vh-agent-harness update --dry-run`; managed files refreshed, project-owned preserved; "+
				"armed-file conflicts are recorded — see `vh-agent-harness proposals`).",
			"Stale project_owned seed (CLAUDE.md/Makefile)? They are seeded ONCE then preserved, so `update` "+
				"will NOT push a template fix into an existing copy. Re-seed manually: `rm <file>` then "+
				"`vh-agent-harness update` (warning: this loses local edits — back the file up first).",
			"Verify health anytime: `vh-agent-harness doctor`.",
		)
		return steps
	}
}

// writeGuide renders the human/agent-readable guide.
func writeGuide(out io.Writer, st harnessState, steps []string) {
	fmt.Fprintln(out, "vh-agent-harness — a repo-resident AI agent harness.")
	fmt.Fprintln(out, "All harness config lives under .vh-agent-harness/; the agent-facing files are the")
	fmt.Fprintln(out, "composed AGENTS.md and CLAUDE.md at the repo root.")
	fmt.Fprintln(out)

	fmt.Fprintln(out, "State:")
	switch st.Phase {
	case phaseGreenfield:
		fmt.Fprintln(out, "  phase:    greenfield (no harness installed here)")
	case phaseAdoptable:
		fmt.Fprintln(out, "  phase:    adoptable (existing .opencode, not yet managed by vh-agent-harness)")
	default:
		fmt.Fprintln(out, "  phase:    installed")
		fmt.Fprintf(out, "  root:     %s\n", st.ProjectRoot)
		if st.RuntimeBackend != "" {
			fmt.Fprintf(out, "  runtime:  %s\n", st.RuntimeBackend)
		}
		if len(st.Overlays) > 0 {
			fmt.Fprintf(out, "  overlays: %s\n", strings.Join(st.Overlays, ", "))
		} else {
			fmt.Fprintln(out, "  overlays: (none)")
		}
		fmt.Fprintf(out, "  mission:  %s\n", yesNo(st.HasMission, "AGENTS.mission.md present", "not yet written"))
	}
	fmt.Fprintln(out)

	fmt.Fprintln(out, "Next steps:")
	for i, s := range steps {
		fmt.Fprintf(out, "  %d. %s\n", i+1, s)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Extending the harness? `/harness` is the full add-an-agent / add-command /")
	fmt.Fprintln(out, "add-skill recipe and overlay anatomy; `vh-agent-harness example` lists")
	fmt.Fprintln(out, "configurable files plus a pack skeleton.")
}

func yesNo(b bool, yes, no string) string {
	if b {
		return yes
	}
	return no
}

func isRegularFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func isExistingDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// printDryRunPlan renders a --dry-run preview of an install/update: what WOULD
// change, with nothing written. It foregrounds the decision-relevant outcomes
// (new files seeded, your files preserved, armed-config reconciles, and any
// conflicts) and collapses the bulk generic-overwrite to a count, so an operator
// or agent can judge safety at a glance before applying.
func printDryRunPlan(out io.Writer, verb, target string, report *substrate.ApplyReport) {
	fmt.Fprintf(out, "DRY RUN — %s plan for %s\n", verb, target)
	fmt.Fprintln(out, "Nothing was written (lineage, run-shape seed, and AGENTS.md compose were skipped).")
	fmt.Fprintln(out, summarizeOutcomes(report.Outcomes))
	fmt.Fprintln(out)

	byAction := map[substrate.FileAction][]string{}
	managed := 0
	for _, o := range report.Outcomes {
		if o.Action == substrate.ActionManagedOverwrite {
			managed++
			continue
		}
		byAction[o.Action] = append(byAction[o.Action], o.Path)
	}
	section := func(title string, act substrate.FileAction) {
		paths := byAction[act]
		if len(paths) == 0 {
			return
		}
		fmt.Fprintf(out, "%s (%d):\n", title, len(paths))
		for _, p := range paths {
			fmt.Fprintf(out, "  %s\n", p)
		}
	}
	section("Would SEED — new project file, written once then yours", substrate.ActionProjectSeeded)
	section("Would PRESERVE — your file, left untouched", substrate.ActionProjectPreserved)
	section("Would RECONCILE — armed config, schema-merged", substrate.ActionArmedMerged)
	section("CONFLICT — armed config needs a decision, NOT written", substrate.ActionArmedProposal)
	if managed > 0 {
		fmt.Fprintf(out, "Would OVERWRITE — %d generic managed file(s), force-refreshed from the corpus.\n", managed)
	}
	if sp := summarizeProposals(report.Proposals); sp != "" {
		fmt.Fprintln(out, sp)
	}
	fmt.Fprintf(out, "\nRe-run `vh-agent-harness %s` without --dry-run to apply.\n", verb)
}

// printNextStepsFooter appends a short, state-aware "Next steps" block after an
// install/update so the operating agent always knows what to do next without a
// separate `guide` call.
func printNextStepsFooter(out io.Writer, target string) {
	steps := nextSteps(detectHarnessState(target))
	if len(steps) == 0 {
		return
	}
	fmt.Fprintln(out, "\nNext steps (`vh-agent-harness guide` for detail):")
	for i, s := range steps {
		fmt.Fprintf(out, "  %d. %s\n", i+1, s)
	}
}
