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
	"os"

	"github.com/spf13/cobra"
)

const rootShort = "vh-agent-harness: install, manage, and run a repo-resident AI agent harness"

var rootCmd = &cobra.Command{
	Use:   "vh-agent-harness",
	Short: rootShort,
	Long: `vh-agent-harness is a single static binary that installs, manages, and
runs a repo-resident AI agent harness.

New here? Run 'vh-agent-harness guide' first — it detects the harness state in
the current repo and prints the exact next steps (install, configure, adopt, or
update). Add --json for machine-readable output.

It is the installer (rendering config-driven files from embedded templates
through a substrate seam), the manager (diff, doctor, proposals), and the
executor (driving a runtime backend abstraction). State is lineage-governed
and repo-relative.

Implemented: install, update, uninstall, preflight, doctor, proposals, diff,
and the seam that renders + applies the embedded corpus with per-class
ownership, schema reconciliation, and lineage tracking.

Runtime verbs: exec, shell, up, down, logs, ps, status. These resolve the
backend by reading the S4 run-shape (.vh-agent-harness/run-shape.yml runtime:
block) FIRST, falling back to the legacy manifest
(.opencode/harness-manifest.json) when S4 is absent. The seam install path
seeds a default S4 (runtime.backend: host-shell) when none exists, so the
runtime verbs resolve a backend post-install (at minimum 'vh-agent-harness exec'
works via host-shell). See the config-authority model.`,
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
		shellCmd,
		upCmd,
		downCmd,
		logsCmd,
		// status (installation info)
		statusCmd,
		// runtime service status
		psCmd,
	)
}

// Execute runs the root command and exits the process on error.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
