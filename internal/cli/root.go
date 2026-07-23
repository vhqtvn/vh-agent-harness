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

// Command groups. These drive the grouped "Available Commands:" layout printed
// by `--help`/no-args. AddGroup order is the display order; within a group cobra
// sorts commands alphabetically. Every registered command should carry one of
// these GroupIDs so the surface renders under a titled group rather than the
// catch-all "Additional Commands:" bucket.
const (
	groupLifecycle   = "lifecycle"
	groupOrientation = "orientation"
	groupHealth      = "health"
	groupRuntime     = "runtime"
)

var rootCmd = &cobra.Command{
	Use:   "vh-agent-harness",
	Short: rootShort,
	Long: `vh-agent-harness is a single static Go binary that installs, manages, and
runs a repo-resident AI agent harness.

It is the installer (rendering config-driven files from embedded templates
through a substrate seam), the manager (diff, doctor, proposals), and the
executor (driving a runtime backend abstraction). State is lineage-governed
and repo-relative.

Lifecycle
  install              install/adopt the harness (preview with --dry-run)
  update               re-render the corpus after a config or binary change (--dry-run)
  uninstall            remove the harness from the current project
  overlay              scaffold/manage overlay packs (overlay new, overlay docs)
  self-update          download + install the latest binary (verified by checksums.txt)

Orientation
  guide                detect harness state + the exact next steps (run this first in any repo)
  example              print a config file's doc/template (no *.example scaffolds shipped)
  docs                 print a generic agent-workflow doc (memory model, session workflow, …)
  sys-prompt           print a named system prompt (binary default, overridable via overlay)
  help [command]       help for a command; also help migrate [vX.Y.Z] for release notes

Health & diagnostics
  preflight            verify environment + install integrity before install/upgrade
  doctor               verify install health
  proposals            list recorded armed-file conflicts (the proposal ledger)
  skill                list and validate OpenCode skills (frontmatter health)
  diff                 inspect drift vs. the embedded corpus
  diagnostics-export   bundle harness state into a redacted, shareable archive (--dry-run)
  status               show install + runtime info
  version              print the vh-agent-harness version and build label

Runtime
  exec                 run a command inside the harness runtime (mutations allowed; gated)
  exec-ro              run a command as read-only intent (host-side classifier, no prompt)
  exec-sandbox         run a command under a HOST-LOCAL sandbox (Landlock + seccomp when active; --sandbox=off|best-effort|strict)
  shell                open an interactive shell inside the harness runtime
  up / down            start / stop the harness runtime backend
  logs / ps            show runtime logs / runtime service status

The exec family (exec, exec-ro, exec-sandbox, shell) is intentionally kept as
DISTINCT verbs — do not unify them. exec and shell dispatch through the runtime
backend; exec-ro is a host-side read-only classifier; exec-sandbox is a
host-local Landlock+seccomp trampoline that never reaches the backend. See
README.agent.md (the exec-family / "two execution planes" section) for the full
model and when to reach for each.

Upgrade loop (after a new binary or config change):
  vh-agent-harness self-update
  vh-agent-harness update --dry-run
  vh-agent-harness update
  vh-agent-harness doctor

Inspect migration notes for a release:
  vh-agent-harness help migrate            # the note for the locally adopted version
  vh-agent-harness help migrate vX.Y.Z     # a specific release's note

Runtime verbs (exec, exec-ro, exec-sandbox, shell, up, down, logs, ps) resolve
the backend by reading the S4 run-shape (.vh-agent-harness/run-shape.yml) first,
falling back to the legacy manifest (.opencode/harness-manifest.json), EXCEPT
exec-sandbox which is always host-local. See the config-authority model.

Glossary: in this help and the docs, "seam" means the internal render/apply
pipeline (classify → plan → per-class apply → lineage) — it is NOT a command.

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
	// Command groups render the command surface under titled sections in
	// `--help`. AddGroup order is the display order; within a group cobra
	// sorts commands alphabetically.
	rootCmd.AddGroup(
		&cobra.Group{ID: groupLifecycle, Title: "Lifecycle:"},
		&cobra.Group{ID: groupOrientation, Title: "Orientation:"},
		&cobra.Group{ID: groupHealth, Title: "Health & Diagnostics:"},
		&cobra.Group{ID: groupRuntime, Title: "Runtime:"},
	)

	// Registration order is the single source of WHICH commands exist; grouping
	// (assigned below) controls the `--help` listing LAYOUT.
	rootCmd.AddCommand(
		// installation lifecycle
		installCmd,
		updateCmd,
		uninstallCmd,
		// orientation (agent-facing entry point)
		guideCmd,
		exampleCmd,
		docsCmd,
		sysPromptCmd,
		// overlay pack management (scaffolding)
		overlayCmd,
		// binary self-management
		selfUpdateCmd,
		versionCmd,
		// release-ceremony machine steps (errata injection)
		releaseCmd,
		// health checks
		preflightCmd,
		doctorCmd,
		proposalsCmd,
		// skill visibility + validity (Slice 1)
		skillCmd,
		// rendering inspection
		diffCmd,
		// diagnostics (operator debug-bundle tooling)
		diagnosticsExportCmd,
		// runtime
		execCmd,
		execRoCmd,
		execSandboxCmd,
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

	// Hidden internal trampoline subcommand (not user-facing).
	rootCmd.AddCommand(execSandboxChildCmd)

	// Group membership. Each non-hidden command is assigned to exactly one
	// titled group so the help surface reads as Lifecycle / Orientation /
	// Health & Diagnostics / Runtime instead of one alphabetical flat list.
	assignGroup(groupLifecycle,
		installCmd, updateCmd, uninstallCmd, selfUpdateCmd, overlayCmd, releaseCmd)
	assignGroup(groupOrientation,
		guideCmd, exampleCmd, docsCmd, sysPromptCmd, helpCmd)
	assignGroup(groupHealth,
		preflightCmd, doctorCmd, proposalsCmd, diffCmd,
		diagnosticsExportCmd, statusCmd, versionCmd, skillCmd)
	assignGroup(groupRuntime,
		execCmd, execRoCmd, execSandboxCmd, shellCmd,
		upCmd, downCmd, logsCmd, psCmd)
}

// assignGroup sets the same GroupID on every supplied command. It is a
// readability helper only — it performs no validation beyond assignment.
func assignGroup(id string, cmds ...*cobra.Command) {
	for _, c := range cmds {
		c.GroupID = id
	}
}

// Execute runs the root command and exits the process on error.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
