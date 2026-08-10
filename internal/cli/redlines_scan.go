package cli

// redlines_scan.go implements `vh-agent-harness redlines scan` — the headless
// exact-tree scanner. This is the surface the commit gate invokes to check the
// IMMUTABLE git tree object being committed.
//
// OUTPUT DISCIPLINE (security-critical):
//   - Every line of stdout/stderr is PASTE-SAFE. Only opaque subj-* ids, generic
//     reason codes (scrub-term / relation-co-occurrence / relation-ambient-side-b),
//     and committed-tree paths appear. NEVER the configured terms, labels, sides,
//     or the why field. The Finding struct has exactly {SubjectID, Reason, Path}
//     and all three are safe: SubjectID is opaque, Reason is a generic code, Path
//     is the committed-tree path (the violation location, literally what is being
//     committed — the scanner's job is to BLOCK that path).
//   - This is distinct from `redlines guidance`, which is the ONE surface allowed
//     to emit real terms (for agent self-context-loading). scan NEVER does that.
//
// EXIT CODE CONTRACT (the machine-stable contract the gate depends on):
//   - 0 = pass (no findings) OR non-applicable (no registry / no binding subject).
//   - 1 = violation(s) found (at least one Finding).
//   - 2 = fail-closed: applicable-but-invalid/unreadable registry, missing or
//         malformed --tree, git failure, or any other error that prevents a clean
//         scan. No term leakage in any error path.

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vhqtvn/vh-agent-harness/internal/redlines"
)

// redlinesScanCmd implements `vh-agent-harness redlines scan --tree <hash>`.
var redlinesScanCmd = &cobra.Command{
	Use:           "scan",
	Short:         "Scan an exact git tree for redline violations (opaque output; headless-safe)",
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `Scan an EXACT immutable git tree object for redline violations.

This command is the headless surface the commit gate invokes. It reads ONLY the
specified tree object — never the working tree, the shared index, or HEAD. There
is NO fallback: --tree is required, and the hash must be a valid 40- or 64-hex
git object id.

EXIT CODES (the machine contract):
  0  pass or non-applicable (no binding registry, or registry absent, or no
     findings in the scanned tree).
  1  violation(s) found.
  2  fail-closed: missing or invalid --tree, applicable-but-invalid/unreadable
     registry, git failure, or any other error.

OUTPUT is opaque human diagnostics: only subj-* subject ids, generic reason
codes, and committed-tree paths. Configured terms are NEVER echoed. The commit
gate reads the exit code, not parsed stdout.

When no registry exists, or no subject binds this repo, exits 0 with a short
status line and writes no files.`,
	Args: cobra.NoArgs,
	RunE: runRedlinesScan,
}

func init() {
	redlinesScanCmd.Flags().String("tree", "", "exact git tree object hash to scan (40- or 64-hex, required)")
	redlinesScanCmd.Flags().StringP("repo", "C", "", "repository root path (default: cwd)")
	redlinesCmd.AddCommand(redlinesScanCmd)
}

// runRedlinesScan is the RunE for `vh-agent-harness redlines scan`.
func runRedlinesScan(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()

	treeHash, _ := cmd.Flags().GetString("tree")
	repoFlag, _ := cmd.Flags().GetString("repo")

	// Resolve the repo root to an absolute path.
	repoRoot, err := resolveRepoRootForScan(repoFlag)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return errRedlinesScanExit{code: 2}
	}

	// Construct the ScanTarget. NewScanTarget validates that repoRoot is
	// absolute + non-empty and treeHash is a valid 40/64-hex git oid. There
	// is NO fallback constructor (no FromWorktree/FromIndex/FromHEAD) — the
	// absence of a valid --tree is a hard error.
	target, err := redlines.NewScanTarget(repoRoot, treeHash)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return errRedlinesScanExit{code: 2}
	}

	// Load the registry. nil → inert (no registry configured). error →
	// fail-closed (present but invalid/unreadable).
	reg, err := redlines.Load(repoRoot)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return errRedlinesScanExit{code: 2}
	}
	if reg == nil {
		fmt.Fprintln(out, "redlines scan: no registry configured")
		return nil
	}

	// Resolve remotes for binding evaluation (best-effort; a non-git dir
	// yields empty remotes and path-glob-only matching).
	remotes, _ := redlines.RepoRemotes(repoRoot)

	// Filter to subjects that bind this repo.
	var binding []redlines.Subject
	for _, s := range reg.Subjects {
		if s.Binds(repoRoot, remotes) {
			binding = append(binding, s)
		}
	}
	if len(binding) == 0 {
		fmt.Fprintln(out, "redlines scan: no redlines bind this repository")
		return nil
	}

	// Enumerate the exact tree's blob entries (skipping symlinks, submodules,
	// and non-blob objects) and fetch each blob's content.
	units, err := enumerateTreeBlobs(target)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return errRedlinesScanExit{code: 2}
	}

	// Scan. The engine is pure (no I/O); it dedups + sorts findings.
	ctx := redlines.ScanContext{RepoPath: repoRoot, Remotes: remotes}
	findings := redlines.Scan(ctx, binding, units)

	// Print findings (opaque: path + subj-id + reason code). Never echo
	// configured terms — the Finding struct has no term field by design.
	for _, f := range findings {
		fmt.Fprintf(out, "%s: %s (%s)\n", f.Path, f.SubjectID, f.Reason)
	}

	// Honesty contract pointer — always printed so a pass is not over-trusted.
	fmt.Fprintln(out, "redlines scan: lexical/best-effort scan — pass ≠ safe; see `redlines guidance` honesty contract")

	if len(findings) > 0 {
		return errRedlinesScanExit{code: 1}
	}
	return nil
}

// enumerateTreeBlobs reads the exact tree object via `git ls-tree -r`, skips
// non-blob entries (symlinks mode 120000, submodules mode 160000, and any
// non-blob objecttype), and fetches each remaining blob via `git cat-file`.
// It returns ScanUnit slices ready for the pure Scan engine.
func enumerateTreeBlobs(target redlines.ScanTarget) ([]redlines.ScanUnit, error) {
	// Default ls-tree format: "<mode> SP <type> SP <object> TAB <path>"
	// Using the default format (not --format) for maximum git-version
	// portability.
	lsCmd := exec.Command("git", "-C", target.RepoRoot, "ls-tree", "-r", target.TreeHash)
	var lsErr bytes.Buffer
	lsCmd.Stderr = &lsErr
	lsOut, err := lsCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("redlines scan: git ls-tree %s in %q: %w%s",
			target.TreeHash, target.RepoRoot, err, trimErrSuffix(lsErr.String()))
	}

	var units []redlines.ScanUnit
	for _, raw := range strings.Split(string(lsOut), "\n") {
		line := strings.TrimRight(raw, "\r")
		if line == "" {
			continue
		}
		// Split on the single tab separating meta from path.
		tabIdx := strings.IndexByte(line, '\t')
		if tabIdx < 0 {
			continue
		}
		meta := line[:tabIdx]
		path := line[tabIdx+1:]

		// Meta is "<mode> <type> <object>" — three space-separated fields.
		fields := strings.Fields(meta)
		if len(fields) != 3 {
			continue
		}
		mode, objType, objName := fields[0], fields[1], fields[2]

		// Skip non-blob entries:
		//   - objecttype != "blob" catches submodules (type "commit") and
		//     tree entries that ls-tree -r does not recurse into.
		//   - mode == "120000" catches symlinks (they have type "blob" but
		//     the symlink target text is not meaningful content to scan).
		//   - mode == "160000" catches gitlinks/submodules (redundant with
		//     the type check, but explicit per the spec).
		if objType != "blob" {
			continue
		}
		if mode == "120000" || mode == "160000" {
			continue
		}

		// Fetch the blob content via cat-file.
		blob, err := gitCatFileBlob(target.RepoRoot, objName)
		if err != nil {
			return nil, err
		}
		units = append(units, redlines.ScanUnit{Path: path, Content: blob})
	}
	return units, nil
}

// gitCatFileBlob fetches a single blob object's content.
func gitCatFileBlob(repoRoot, objectName string) ([]byte, error) {
	catCmd := exec.Command("git", "-C", repoRoot, "cat-file", "blob", objectName)
	var catErr bytes.Buffer
	catCmd.Stderr = &catErr
	out, err := catCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("redlines scan: git cat-file blob %s in %q: %w%s",
			objectName, repoRoot, err, trimErrSuffix(catErr.String()))
	}
	return out, nil
}

// resolveRepoRootForScan resolves the --repo flag (or cwd when empty) to an
// absolute path.
func resolveRepoRootForScan(repoFlag string) (string, error) {
	if repoFlag == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("redlines scan: resolve cwd: %w", err)
		}
		return cwd, nil
	}
	abs, err := filepath.Abs(repoFlag)
	if err != nil {
		return "", fmt.Errorf("redlines scan: resolve repo root %q: %w", repoFlag, err)
	}
	return abs, nil
}

// trimErrSuffix cleans up git's stderr for inclusion in an opaque error line.
func trimErrSuffix(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return ": " + s
}

// errRedlinesScanExit is a codedError carrying the scan exit code (1 for
// violations, 2 for fail-closed errors). Its message is empty so cobra does
// not print a redundant "Error:" line — the command prints its own opaque
// diagnostics to stderr before returning this.
type errRedlinesScanExit struct {
	code int
}

func (e errRedlinesScanExit) Error() string {
	return ""
}

func (e errRedlinesScanExit) ExitCode() int {
	return e.code
}
