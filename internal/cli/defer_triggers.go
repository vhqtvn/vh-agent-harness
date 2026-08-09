package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vhqtvn/vh-agent-harness/internal/execsandbox"
)

// deferTriggersCmd implements `vh-agent-harness defer-triggers` — a no-arg
// command that runs the DEFER-trigger predicate checker
// (.opencode/scripts/check-defer-triggers.mjs) under a strict host-local
// sandbox (ModeStrict + NetDeny + DefaultProfile). It is the sanctioned,
// contained read-only surface that lets a read-only agent (researcher /
// worker-read-only) evaluate which DEFER/p2/follow-up candidates' triggers are
// currently met, WITHOUT exposing exec-sandbox's open --sandbox/--net flags or
// a caller-controlled exe/script/mode.
//
// Design contract (O5_TMP / DEFER card defer-checker-runnability-readonly-role):
//
//   - NO args and NO flags. The checker path, node binary, sandbox mode, and
//     net policy are ALL fixed by this command — a caller cannot override any
//     of them. This is the permission boundary that makes a `defer-triggers`
//     grant safe for a read-only agent: the only thing the agent can do is run
//     the canonical checker contained, in its default (promoter) mode.
//
//   - The checker runs in default PROMOTER mode (no --mode forwarded): it
//     prints a human-readable report and exits 0 — never blocking, never
//     release-authoritative. The release/release-prep modes are NOT exposed
//     here (they are release-ceremony surfaces with their own paths).
//
//   - The checker writes its report to stdout (forwarded by the trampoline) and
//     uses repo <repoRoot>/tmp for git-capture scratch (the sole RWDir in the
//     profile). The checker's gitCapture/gitSuccess helpers use a FILE-BACKED
//     stdout descriptor (not a pipe), so libuv does not allocate the
//     socketpair(AF_UNIX) that NetDeny blocks — git runs and its stdout is
//     captured from the file. This is the load-bearing mechanism (Phase-0
//     proven) that makes the checker functional under the strict sandbox.
//
//   - Profile-covered node: bare `node` may resolve to an nvm install under
//     $HOME (outside the Landlock read profile), which the trampoline's
//     syscall.Exec would deny with EACCES. The wrapper resolves node to a
//     candidate UNDER a profile RO dir (/usr/bin, /usr/local/bin, /bin) so the
//     trampoline succeeds.
//
//   - Exit code propagates from the checker (mirrors exec-sandbox: os.Exit on
//     nonzero; NOT routed through root.exitCodeFromError).
var deferTriggersCmd = &cobra.Command{
	Use:          "defer-triggers",
	Short:        "Run the DEFER-trigger predicate checker under a strict sandbox (read-only, contained)",
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	Long: `vh-agent-harness defer-triggers runs the DEFER-trigger predicate checker
(.opencode/scripts/check-defer-triggers.mjs) under a strict host-local sandbox
(Landlock + seccomp, ModeStrict + NetDeny, DefaultProfile).

It takes NO arguments and NO flags: the checker path, the node binary, the
sandbox mode, and the network policy are all fixed. A read-only agent
(researcher, worker-read-only) invokes this to evaluate which
DEFER/p2/follow-up candidates' triggers are currently met. The checker runs in
its default promoter mode — it prints a human-readable report and exits 0
(promoter-use-only, never blocking, never wired into a commit hook).

This is the contained equivalent of
  vh-agent-harness exec-sandbox --sandbox=strict --net=deny -- <node> <checker>
but with NO caller-controlled exe/script/mode/net — the only thing the agent can
do is run the canonical checker contained. Pre-creates <repo>/tmp (the checker's
git-capture scratch dir) if absent.`,
	RunE: runDeferTriggers,
}

func runDeferTriggers(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("defer-triggers: determining working directory: %w", err)
	}
	repoRoot := findRepoRoot(cwd)

	// Pre-create <repoRoot>/tmp — the sole writable path in the sandbox profile
	// (RWDirs=[repoRoot/tmp]). The checker's git-capture helpers write scratch
	// files here. The checker allocates a unique exclusive subdir per git call;
	// this top-level tmp only needs to exist so those mkdirSync calls succeed.
	tmpDir := filepath.Join(repoRoot, "tmp")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return fmt.Errorf("defer-triggers: creating %s: %w", tmpDir, err)
	}

	// The RENDERED checker is the runtime source of truth (rendered from
	// templates/core on `update`). Resolve its absolute path under repoRoot.
	checkerPath := filepath.Join(repoRoot, ".opencode", "scripts", "check-defer-triggers.mjs")
	if _, err := os.Stat(checkerPath); err != nil {
		return fmt.Errorf("defer-triggers: checker not found at %s: %w (run `vh-agent-harness update`)", checkerPath, err)
	}

	// Hardcode the containment contract: ModeStrict + NetDeny + DefaultProfile.
	// No caller-controlled mode/net/profile (there are no flags). This repo's
	// strict floor would force strict anyway; the hardcode makes the guarantee
	// hold even in an unfloored consumer repo.
	profile := execsandbox.DefaultProfile(repoRoot)

	// Resolve node to a profile-covered path so the trampoline's syscall.Exec
	// succeeds under Landlock (bare `node` may resolve to an nvm install under
	// $HOME, which is outside the read profile).
	nodePath, err := resolveProfileCoveredNode(profile.RODirs)
	if err != nil {
		return fmt.Errorf("defer-triggers: %w", err)
	}

	// Invoke ONLY the canonical checker, with NO forwarded args. The checker
	// runs in default promoter mode (exit 0, never blocking).
	ctx := context.Background()
	exitCode, runErr := execsandbox.Run(ctx, execsandbox.ModeStrict, profile, repoRoot, nodePath, []string{checkerPath})
	if runErr != nil {
		fmt.Fprintf(os.Stderr, "defer-triggers: %v\n", runErr)
	}
	// Mirror exec-sandbox: propagate the child exit code directly (the
	// trampoline yields a numeric exit code, not a Go error). NOT routed through
	// root.exitCodeFromError.
	if exitCode != 0 {
		os.Exit(exitCode)
	}
	return nil
}

// resolveProfileCoveredNode resolves the node binary to a path that (a) exists
// and is executable, and (b) is under one of the sandbox profile's read-only
// directories, so the trampoline's syscall.Exec succeeds under Landlock. Bare
// `node` on a developer machine frequently resolves to an nvm install under
// $HOME (outside the profile), which Landlock would deny with EACCES at
// syscall.Exec; this picks a system node under /usr or /bin instead. Candidate
// order is the common system install locations.
func resolveProfileCoveredNode(roDirs []string) (string, error) {
	candidates := []string{"/usr/bin/node", "/usr/local/bin/node", "/bin/node"}
	var tried []string
	for _, c := range candidates {
		tried = append(tried, c)
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		info, err := os.Stat(abs)
		if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
			continue
		}
		if !pathUnderRODir(abs, roDirs) {
			continue
		}
		return abs, nil
	}
	return "", fmt.Errorf("no profile-covered node binary found (tried %s); install node under /usr or /bin", strings.Join(tried, ", "))
}

// pathUnderRODir reports whether absPath is equal to or nested under one of
// roDirs. roDirs entries are absolute directory paths treated as recursive
// read-only roots by Landlock (a rule on /usr covers /usr/bin and
// /usr/local/bin recursively).
func pathUnderRODir(absPath string, roDirs []string) bool {
	sep := string(filepath.Separator)
	for _, d := range roDirs {
		if absPath == d || strings.HasPrefix(absPath, d+sep) {
			return true
		}
	}
	return false
}
