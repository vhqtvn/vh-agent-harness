// Package cli wires the harness Cobra command tree.
//
// The binary ships the install/management + render surface: install/update/
// uninstall render the embedded template corpus through the substrate seam
// (classify → plan → per-class apply → lineage), preflight/doctor lint the
// install, proposals surfaces needs-decision armed conflicts, and diff
// inspects the staged tree without touching the live project.
//
// The runtime verbs (exec, shell, up, down, logs, ps) and `status` resolve the
// backend by reading the S4 run-shape (`.vh-agent-harness/run-shape.yml`
// `runtime:` block) FIRST, falling back to the legacy manifest
// (`.opencode/harness-manifest.json`) when S4 is absent. The seam `install`
// path seeds a default S4 (`runtime.backend: host-shell`) when none exists, so
// the runtime verbs resolve a backend post-install (see
// the config-authority model).
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const rootShort = "vh-agent-harness: install, manage, and run a repo-resident AI agent harness"

var rootCmd = &cobra.Command{
	Use:   "vh-agent-harness",
	Short: rootShort,
	Long: `vh-agent-harness is a single static Go binary that installs, manages, and
runs a repo-resident AI agent harness.

It is the installer (rendering config-driven files from embedded templates
through a substrate seam), the manager (diff, doctor, proposals), and the
executor (driving a runtime backend abstraction). State is lineage-governed
and repo-relative.

Agent orientation
  guide          detect harness state + the exact next steps (run this first in any repo)
  install        install/adopt the harness (preview with --dry-run)
  update         re-render the corpus after a config or binary change (--dry-run)
  doctor         verify install health
  diff           inspect drift vs. the embedded corpus
  status         show install + runtime info
  example        print a config file's doc/template (no *.example scaffolds shipped)

Upgrade loop (after a new binary or config change):
  vh-agent-harness self-update
  vh-agent-harness update --dry-run
  vh-agent-harness update
  vh-agent-harness doctor

Inspect migration notes for a release:
  vh-agent-harness help migrate            # the note for the locally adopted version
  vh-agent-harness help migrate vX.Y.Z     # a specific release's note

Runtime verbs (exec, shell, up, down, logs, ps) resolve the backend by reading
the S4 run-shape (.vh-agent-harness/run-shape.yml) first, falling back to the
legacy manifest (.opencode/harness-manifest.json). See the config-authority model.

Run 'vh-agent-harness guide' for dynamic, repo-aware next steps.`,
	// No-args prints the root help and exits 0. An unexpected token (a typo'd
	// subcommand) is surfaced as an unknown-command error rather than silently
	// showing help, so typos don't look like success.
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
	},
}

func init() {
	// Registration order controls `--help` listing order.
	rootCmd.AddCommand(
		// installation lifecycle
		installCmd,
		updateCmd,
		uninstallCmd,
		// orientation (agent-facing entry point)
		guideCmd,
		exampleCmd,
		// overlay pack management (scaffolding)
		overlayCmd,
		// binary self-management
		selfUpdateCmd,
		versionCmd,
		// health checks
		preflightCmd,
		doctorCmd,
		proposalsCmd,
		// rendering inspection
		diffCmd,
		// runtime
		execCmd,
		execRoCmd,
		shellCmd,
		upCmd,
		downCmd,
		logsCmd,
		// status (installation info)
		statusCmd,
		// runtime service status
		psCmd,
		// help command (replaces cobra's auto-generated one so it can also route
		// the special `help migrate [version]` topic; migrate is NOT a top-level
		// command — it is intercepted inside help only).
		helpCmd,
	)
	rootCmd.SetHelpCommand(helpCmd)
}

// Execute runs the root command and exits the process on error.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
