package cli

// pause_new_work.go implements the `vh-agent-harness pause-new-work` verb, the
// operator UX for the repo-scoped pause on NEW work across enumerated dispatch
// entrypoints (memo-4).
//
// NAMING HONESTY (load-bearing): this is NOT 'global ESTOP', NOT 'pause every
// agent', NOT an agent-loop interlock, NOT an abort/kill switch. It engages a
// sentinel file under .opencode/state/ that the JS contract
// (.opencode/scripts/pause-new-work.js), the OpenCode plugin
// (.opencode/plugins/pause-new-work.js), and the Python bgshell port all read
// to REFUSE covered NEW-work admissions. In-flight work is NEVER touched.
//
// The Go binary owns the sentinel WRITE/DELETE operations so engage/status/
// disengage work from a plain terminal with no node dependency, and so status
// is reachable even under a degraded read. The READ contract is mirrored
// verbatim from the JS module (one source of truth — keep in lockstep):
//
//	sentinel absent                 -> disengaged (permit)
//	sentinel present + valid        -> engaged
//	sentinel present + malformed    -> engaged (fail-safe)
//	sentinel present + unreadable   -> engaged (fail-safe)
//	indeterminate FS check failure  -> engaged + DEGRADED
//
// The sentinel lives under stateRoot = OPENCODE_STATE_ROOT (override) or
// <repoRoot>/.opencode/state. That subtree is drift-exempt
// (internal/drift/drift.go managedSubtreesToSkip includes .opencode/state), so
// creating/removing the sentinel never trips managed-drift.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/vhqtvn/vh-agent-harness/internal/runshape"
)

const pauseSentinelFilename = "pause-new-work.json"

var pauseNewWorkCmd = &cobra.Command{
	Use:   "pause-new-work",
	Short: "Manage the repo-scoped pause on NEW work (engage / status / disengage)",
	Long: `Manage the repo-scoped pause on NEW work across enumerated dispatch entrypoints.

This is NOT a global pause, NOT an abort/kill switch, and NOT an agent-loop
interlock. When engaged, a sentinel file is written under .opencode/state/ that
covered dispatch seams read to REFUSE new-work admissions with a clear message.
IN-FLIGHT WORK IS NEVER TOUCHED: engaging sends no signal and cancels no
running process or OpenCode response. Existing child work may complete and
report. Closeout, status, diagnosis, and disengage remain reachable while
engaged.

Covered new-work seams:
  - coordination task activation (ready->working dispatch only; in-flight
    working->working resume/reclaim/takeover is continuation and stays available)
  - bgshell launch + resume (NEW spawn; the stop path is untouched)
  - OpenCode TaskTool dispatch (@subagent / new child task)
  - the dispatch commands /implement /implement-goal /research /solution-brief
    (the "begin new delegated work" class)

Deliberately NOT blocked: ordinary chat, diagnostic tools, ordinary non-dispatch
tool calls by an in-flight root turn, /resume-task (it is BOTH a new-dispatch AND
a continuation entry point — the precise ready->working gate in
activateCoordinationTask is the seam, so blanket-blocking the command would
forbid in-flight continuation), and all state/utility/diagnosis/review/planning
commands (including /write-task, which creates candidate transport and does not
begin execution).

Subcommands:
  engage [reason]    write the sentinel; refuse covered new work
  status             report engaged / disengaged / degraded
  disengage          remove the sentinel; resume ordinary operation

Run with no subcommand to print status.`,
	Args: cobra.NoArgs,
	RunE: runPauseStatusFromRoot,
}

var pauseEngageCmd = &cobra.Command{
	Use:   "engage [reason]",
	Short: "Engage the pause on new work (write the sentinel)",
	Long: `Engage the repo-scoped pause on new work.

Writes the sentinel file under .opencode/state/. Covered dispatch seams will
refuse new-work admissions with a clear message. In-flight work is NOT affected.
An optional reason string is stored as advisory metadata in the sentinel.`,
	Args: cobra.ArbitraryArgs,
	RunE: runPauseEngage,
}

var pauseStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Report the pause-on-new-work state",
	Long: `Report whether the pause on new work is engaged, disengaged, or degraded.

Exits 0 in all cases (status is always reachable, including under degraded).`,
	Args: cobra.NoArgs,
	RunE: runPauseStatus,
}

var pauseDisengageCmd = &cobra.Command{
	Use:   "disengage",
	Short: "Disengage the pause on new work (remove the sentinel)",
	Long: `Disengage the repo-scoped pause on new work.

Removes the sentinel file so covered dispatch seams resume permitting new work.
A no-op (exit 0) when the sentinel is already absent.`,
	Args: cobra.NoArgs,
	RunE: runPauseDisengage,
}

func init() {
	pauseNewWorkCmd.AddCommand(pauseEngageCmd, pauseStatusCmd, pauseDisengageCmd)
}

// pauseProjectRoot resolves the harness project root from the current working
// directory by walking up for the run-shape marker. Returns "" when no harness
// install is found in any parent.
func pauseProjectRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	root, _, err := runshape.FindForRoot(cwd)
	if err != nil {
		return "", err
	}
	return root, nil
}

// pauseStateRoot mirrors the JS contract: OPENCODE_STATE_ROOT override, else
// <repoRoot>/.opencode/state.
func pauseStateRoot(repoRoot string) string {
	if override := strings.TrimSpace(os.Getenv("OPENCODE_STATE_ROOT")); override != "" {
		return override
	}
	return filepath.Join(repoRoot, ".opencode", "state")
}

func pauseSentinelPath(repoRoot string) string {
	return filepath.Join(pauseStateRoot(repoRoot), pauseSentinelFilename)
}

// pauseReadState mirrors readPauseState in the JS contract. Returns engaged,
// degraded, and advisory metadata. Existence is the authority; content is
// advisory. A single os.ReadFile is both existence check and content read (no
// stat-then-read TOCTOU).
func pauseReadState(repoRoot string) (engaged, degraded bool, meta map[string]any, readErr error) {
	sp := pauseSentinelPath(repoRoot)
	raw, err := os.ReadFile(sp)
	if err != nil {
		if os.IsNotExist(err) {
			return false, false, nil, nil
		}
		// Indeterminate filesystem failure -> fail safe + degraded.
		return true, true, nil, err
	}
	// Present -> engaged. Parse advisory metadata defensively.
	var parsed map[string]any
	if len(strings.TrimSpace(string(raw))) > 0 {
		if jerr := json.Unmarshal(raw, &parsed); jerr != nil {
			parsed = nil // malformed content: engaged, NOT degraded
		}
	}
	return true, false, parsed, nil
}

// pauseEngageSentinel writes the sentinel + advisory metadata.
func pauseEngageSentinel(repoRoot, reason string) (string, error) {
	sp := pauseSentinelPath(repoRoot)
	if err := os.MkdirAll(filepath.Dir(sp), 0o755); err != nil {
		return sp, err
	}
	trimmed := strings.TrimSpace(reason)
	meta := map[string]any{
		"engaged_at": time.Now().UTC().Format(time.RFC3339),
		"reason":     nil,
	}
	if trimmed != "" {
		meta["reason"] = trimmed
	}
	body, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return sp, err
	}
	body = append(body, '\n')
	return sp, os.WriteFile(sp, body, 0o644)
}

// pauseDisengageSentinel removes the sentinel. A missing sentinel is a clean
// no-op.
func pauseDisengageSentinel(repoRoot string) (path string, existed bool, err error) {
	sp := pauseSentinelPath(repoRoot)
	err = os.Remove(sp)
	if err != nil {
		if os.IsNotExist(err) {
			return sp, false, nil
		}
		return sp, false, err
	}
	return sp, true, nil
}

func runPauseStatusFromRoot(cmd *cobra.Command, _ []string) error {
	return runPauseStatus(cmd, nil)
}

func runPauseStatus(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	root, err := pauseProjectRoot()
	if err != nil {
		return err
	}
	if root == "" {
		// No harness install found. Report disengaged rather than erroring so
		// status is always reachable; the sentinel could not exist here anyway.
		fmt.Fprintln(out, "disengaged")
		fmt.Fprintln(out, "  (no harness installation found in this directory or any parent)")
		return nil
	}
	engaged, degraded, meta, readErr := pauseReadState(root)
	sp := pauseSentinelPath(root)
	fmt.Fprintf(out, "sentinel: %s\n", sp)
	if !engaged {
		fmt.Fprintln(out, "state:    disengaged")
		fmt.Fprintln(out, "(ordinary operation; covered new work is permitted)")
		return nil
	}
	if degraded {
		fmt.Fprintln(out, "state:    engaged (DEGRADED)")
		if readErr != nil {
			fmt.Fprintf(out, "  error: %v\n", readErr)
		}
		fmt.Fprintln(out, "  (the sentinel could not be read cleanly; covered new work is refused")
		fmt.Fprintln(out, "   as a precaution. Recover by disengaging or correcting the file.)")
		fmt.Fprintf(out, "\nTo disengage: vh-agent-harness pause-new-work disengage\n")
		return nil
	}
	fmt.Fprintln(out, "state:    engaged")
	if v, ok := meta["engaged_at"].(string); ok && v != "" {
		fmt.Fprintf(out, "  engaged_at: %s\n", v)
	}
	if v, ok := meta["reason"].(string); ok && v != "" {
		fmt.Fprintf(out, "  reason:     %s\n", v)
	}
	fmt.Fprintln(out, "(covered new work is refused; in-flight work is unaffected)")
	fmt.Fprintf(out, "\nTo disengage: vh-agent-harness pause-new-work disengage\n")
	return nil
}

func runPauseEngage(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	root, err := pauseProjectRoot()
	if err != nil {
		return err
	}
	if root == "" {
		return fmt.Errorf("no harness installation found in this directory (or any parent); " +
			"run from within a harness-installed repo")
	}
	reason := strings.Join(args, " ")
	sp, err := pauseEngageSentinel(root, reason)
	if err != nil {
		return fmt.Errorf("engage pause: %w", err)
	}
	fmt.Fprintln(out, "engaged")
	fmt.Fprintf(out, "  sentinel: %s\n", sp)
	fmt.Fprintln(out, "  Covered new-work admissions are now refused.")
	fmt.Fprintln(out, "  In-flight work is NOT affected.")
	return nil
}

func runPauseDisengage(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	root, err := pauseProjectRoot()
	if err != nil {
		return err
	}
	if root == "" {
		// Nothing to disengage; mirror status's reachability stance.
		fmt.Fprintln(out, "disengaged")
		fmt.Fprintln(out, "  (no harness installation found; nothing to remove)")
		return nil
	}
	sp, existed, err := pauseDisengageSentinel(root)
	if err != nil {
		return fmt.Errorf("disengage pause: %w", err)
	}
	fmt.Fprintln(out, "disengaged")
	fmt.Fprintf(out, "  sentinel: %s\n", sp)
	if existed {
		fmt.Fprintln(out, "  (removed; covered new work is permitted again)")
	} else {
		fmt.Fprintln(out, "  (sentinel was already absent; ordinary operation)")
	}
	return nil
}
