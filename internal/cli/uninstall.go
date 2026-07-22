package cli

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vhqtvn/vh-agent-harness/internal/lineage"
	"github.com/vhqtvn/vh-agent-harness/internal/manifest"
	"github.com/vhqtvn/vh-agent-harness/internal/ownership"
)

// uninstallCmd removes the harness from the current project. It deletes every
// manifest-tracked file whose ownership class is renderable (managed /
// generated-from-config), preserves project-owned files, all runtime state
// (.opencode/state, .opencode/plans), the entire .local/ tree, and any overlay,
// cleans up the empty parent directories it leaves behind (never the .opencode/
// root or the project root), and removes the manifest file LAST. It then prints a
// leftover report of what was intentionally retained.
//
// --force additionally deletes project-owned files, but STILL never touches
// state dirs or .local/ (those require manual removal by the operator).
var uninstallCmd = &cobra.Command{
	Use:           "uninstall",
	Short:         "Remove the harness from the current project",
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `Remove the harness installation from the current project.

Deleted:
  - every manifest-tracked file whose class is managed or generated-from-config
    (the rendered corpus), EXCEPT anything under .local/ or the runtime-state
    subtrees (.opencode/state, .opencode/plans), which are always preserved.

Preserved (intentionally, see the leftover report):
  - project-owned files
  - .local/ entirely (the installed scaffold examples AND any operator overlay)
  - runtime state: .opencode/state/, .opencode/plans/
  - the .opencode/ root and the project root

On a seam install the harness footprint is enumerated by re-rendering the
embedded corpus + active overlays (no legacy manifest needed); the seam record
(.vh-agent-harness/lineage.yml) is removed LAST. On a legacy install the
manifest file (.opencode/harness-manifest.json) is removed LAST instead.

--force: ALSO delete project-owned files. State dirs (.opencode/state,
.opencode/plans) and .local/ are NEVER removed, even with --force.`,
	Args: cobra.NoArgs,
	RunE: runUninstall,
}

// uninstallForce is wired to the --force flag.
var uninstallForce bool

func init() {
	uninstallCmd.Flags().BoolVar(&uninstallForce, "force", false,
		"also delete project-owned files (state dirs and .local/ are still preserved)")
}

func runUninstall(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	// Seam-first: a tree carrying .vh-agent-harness/lineage.yml is seam-managed,
	// so enumerate the harness footprint by re-rendering the corpus rather than
	// reading a legacy manifest (which a seam install never writes). Legacy
	// installs (no lineage) fall back to the manifest-driven path below.
	if isSeamInstalled(cwd) {
		return runSeamUninstall(cmd, cwd, uninstallForce)
	}

	lm, err := loadManifest()
	if err != nil {
		return err
	}

	res, err := uninstallHarness(lm, uninstallForce)
	if err != nil {
		fmt.Fprintln(errOut, "error:", err)
		return err
	}

	fmt.Fprintf(out, "uninstall: removed %d managed files (%d project-owned retained)\n",
		res.removed, res.projectOwnedRetained)
	if res.projectOwnedRemoved > 0 {
		fmt.Fprintf(out, "  --force also removed %d project-owned files\n", res.projectOwnedRemoved)
	}
	printLeftoverReport(out, lm, res)
	return nil
}

// uninstallResult summarizes a completed uninstall for the leftover report.
type uninstallResult struct {
	removed              int      // managed/generated files deleted
	projectOwnedRetained int      // project-owned files kept (always, unless --force)
	projectOwnedRemoved  int      // project-owned files deleted (--force only)
	preservedLocal       []string // .local/ entries left on disk
	preservedState       []string // state subtree dirs left on disk
	retainedProjectOwned []string // project-owned rel paths kept (non-force)
}

// uninstallHarness is the testable core of `vh-agent-harness uninstall`. It never touches
// .local/, .opencode/state/, .opencode/plans/, the .opencode/ root, or the
// project root. The manifest file is removed LAST.
func uninstallHarness(lm *loadedManifest, force bool) (*uninstallResult, error) {
	res := &uninstallResult{}

	// Deterministic iteration over manifest.files.
	rels := make([]string, 0, len(lm.m.Files))
	for rel := range lm.m.Files {
		rels = append(rels, rel)
	}
	sort.Strings(rels)

	opencodeRoot := filepath.Join(lm.dir, manifest.DirName)
	preserve := map[string]bool{lm.dir: true, opencodeRoot: true}
	var cleanedDirs []string

	for _, rel := range rels {
		f := lm.m.Files[rel]
		if isPreservedPath(rel) {
			// .local/ and runtime-state subtrees are NEVER removed, even --force.
			continue
		}
		if !force && !manifest.IsRenderable(f.Class) {
			// project-owned, non-force -> retain.
			if f.Class == manifest.ClassProjectOwned {
				res.projectOwnedRetained++
				res.retainedProjectOwned = append(res.retainedProjectOwned, rel)
			}
			continue
		}
		abs := filepath.Join(lm.dir, filepath.FromSlash(rel))
		if rerr := os.Remove(abs); rerr != nil {
			if !os.IsNotExist(rerr) {
				return nil, fmt.Errorf("delete %s: %w", rel, rerr)
			}
			// already absent; still count below as removed-from-tracking
		} else {
			cleanedDirs = append(cleanedDirs, filepath.Dir(abs))
		}
		if force && f.Class == manifest.ClassProjectOwned {
			res.projectOwnedRemoved++
		} else {
			res.removed++
		}
	}

	// Clean up now-empty parent directories (never the .opencode/ root or the
	// project root). Deepest-first so empty leaves clear before their parents.
	cleanupEmptyParentDirs(cleanedDirs, preserve)

	// Record preserved on-disk state BEFORE removing the manifest (which may be
	// the last thing under .opencode/).
	res.preservedLocal = listExistingSubPaths(lm.dir, []string{".local"})
	res.preservedState = listExistingSubPaths(lm.dir,
		[]string{filepath.Join(manifest.DirName, "state"), filepath.Join(manifest.DirName, "plans")})

	// Remove the manifest file LAST.
	if err := os.Remove(manifest.FilePath(lm.dir)); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("delete manifest: %w", err)
	}
	return res, nil
}

// runSeamUninstall removes the harness footprint from a seam-installed tree. It
// enumerates every rendered corpus/overlay path via seamInventory (the seam
// equivalent of the manifest), deletes the platform-controlled files (managed /
// armed / overlay / external_generated), preserves project_owned files (unless
// --force), never touches .local/ or runtime-state subtrees, cleans up the empty
// dirs it leaves, and removes the S1 lineage record LAST.
func runSeamUninstall(cmd *cobra.Command, target string, force bool) error {
	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()

	staging, eff, staged, _, err := seamInventory(target)
	if err != nil {
		fmt.Fprintln(errOut, "error:", err)
		return err
	}
	defer os.RemoveAll(staging)

	res := &uninstallResult{}
	opencodeRoot := filepath.Join(target, manifest.DirName)
	preserve := map[string]bool{target: true, opencodeRoot: true}
	var cleanedDirs []string

	for _, rel := range staged {
		if isPreservedPath(rel) {
			continue
		}
		// A rendered path with no classification was still produced by the
		// corpus, so it is platform-controlled: default to removing it.
		class := ownership.ClassPlatformManaged
		if c, ok := eff.ClassOf(rel); ok {
			class = c
		}
		if class == ownership.ClassProjectOwned && !force {
			res.projectOwnedRetained++
			res.retainedProjectOwned = append(res.retainedProjectOwned, rel)
			continue
		}
		abs := filepath.Join(target, filepath.FromSlash(rel))
		if rerr := os.Remove(abs); rerr != nil {
			if !os.IsNotExist(rerr) {
				fmt.Fprintln(errOut, "error:", fmt.Errorf("delete %s: %w", rel, rerr))
				return fmt.Errorf("delete %s: %w", rel, rerr)
			}
		} else {
			cleanedDirs = append(cleanedDirs, filepath.Dir(abs))
		}
		if force && class == ownership.ClassProjectOwned {
			res.projectOwnedRemoved++
		} else {
			res.removed++
		}
	}

	cleanupEmptyParentDirs(cleanedDirs, preserve)

	res.preservedLocal = listExistingSubPaths(target, []string{".local"})
	res.preservedState = listExistingSubPaths(target,
		[]string{filepath.Join(manifest.DirName, "state"), filepath.Join(manifest.DirName, "plans")})

	// Remove the S1 lineage record LAST (the seam's manifest equivalent), then
	// drop the .vh-agent-harness/ dir if it is now empty. run-shape.yml and any
	// operator-authored overrides there are preserved (operator runtime config).
	if rerr := os.Remove(lineage.FilePath(target)); rerr != nil && !os.IsNotExist(rerr) {
		fmt.Fprintln(errOut, "error:", fmt.Errorf("delete lineage: %w", rerr))
		return fmt.Errorf("delete lineage: %w", rerr)
	}
	_ = os.Remove(filepath.Dir(lineage.FilePath(target))) // no-op if non-empty

	fmt.Fprintf(out, "uninstall: removed %d managed files (%d project-owned retained)\n",
		res.removed, res.projectOwnedRetained)
	if res.projectOwnedRemoved > 0 {
		fmt.Fprintf(out, "  --force also removed %d project-owned files\n", res.projectOwnedRemoved)
	}
	printSeamLeftoverReport(out, target, res)
	return nil
}

// printSeamLeftoverReport mirrors printLeftoverReport for the seam path (no
// loadedManifest); it lists what uninstall intentionally preserved.
func printSeamLeftoverReport(out interface{ Write([]byte) (int, error) }, target string, res *uninstallResult) {
	fmt.Fprintln(out, "leftover (intentionally preserved):")
	noted := false
	for _, p := range res.preservedLocal {
		fmt.Fprintf(out, "  %s\n", p)
		noted = true
	}
	for _, p := range res.preservedState {
		fmt.Fprintf(out, "  %s\n", p)
		noted = true
	}
	for _, p := range res.retainedProjectOwned {
		fmt.Fprintf(out, "  %s  (project-owned)\n", p)
		noted = true
	}
	if _, err := os.Stat(filepath.Join(target, manifest.DirName)); err == nil {
		fmt.Fprintf(out, "  %s/  (root preserved; never removed by uninstall)\n", manifest.DirName)
		noted = true
	}
	if _, err := os.Stat(filepath.Join(target, ".vh-agent-harness")); err == nil {
		fmt.Fprintln(out, "  .vh-agent-harness/  (operator runtime config preserved; e.g. run-shape.yml)")
		noted = true
	}
	if !noted {
		fmt.Fprintln(out, "  (nothing preserved — clean removal)")
	}
	fmt.Fprintln(out, "note: state dirs, .local/, project-owned files, and .vh-agent-harness/ runtime config are never auto-removed; delete manually if desired.")
}

// isPreservedPath reports whether rel is under a subtree that uninstall must
// NEVER delete, regardless of ownership class or --force. It matches the drift
// package's runtime-state exclusions (.opencode/state, .opencode/plans) and adds
// the entire .local/ tree (which ships scaffold examples but also holds the
// operator overlay).
func isPreservedPath(rel string) bool {
	clean := path.Clean("/" + filepath.ToSlash(rel)) // leading-slash normalize
	if clean == "/.local" || strings.HasPrefix(clean, "/.local/") {
		return true
	}
	if clean == "/.opencode/state" || strings.HasPrefix(clean, "/.opencode/state/") {
		return true
	}
	if clean == "/.opencode/plans" || strings.HasPrefix(clean, "/.opencode/plans/") {
		return true
	}
	return false
}

// cleanupEmptyParentDirs removes each directory in dirs and walks upward
// removing parents while they are empty, stopping at any directory in preserve
// (typically the project root and the .opencode/ root) or the filesystem root.
// A non-empty directory stops the walk for that branch. Errors are ignored
// (os.Remove on a non-empty dir is a benign no-op here).
func cleanupEmptyParentDirs(dirs []string, preserve map[string]bool) {
	// Deduplicate + collect the full ancestor chain (minus preserved roots) so
	// we can remove deepest-first in one deterministic pass.
	seen := map[string]bool{}
	var chain []string
	for _, start := range dirs {
		d := start
		for {
			if preserve[d] {
				break
			}
			parent := filepath.Dir(d)
			if parent == d {
				break // filesystem root
			}
			if !seen[d] {
				seen[d] = true
				chain = append(chain, d)
			}
			d = parent
		}
	}
	sort.Slice(chain, func(i, j int) bool { return len(chain[i]) > len(chain[j]) })
	for _, d := range chain {
		_ = os.Remove(d) // non-empty -> no-op; we never remove preserved roots.
	}
}

// listExistingSubPaths returns the repo-relative entries (as given) that exist on
// disk under root, used to build the leftover report. Each candidate is a
// repo-relative path; only those that stat successfully are returned.
func listExistingSubPaths(root string, candidates []string) []string {
	var out []string
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(c))); err == nil {
			out = append(out, c)
		}
	}
	return out
}

// printLeftoverReport prints the intentionally-retained paths so the operator
// understands what uninstall deliberately left behind and why.
func printLeftoverReport(out interface{ Write([]byte) (int, error) }, lm *loadedManifest, res *uninstallResult) {
	fmt.Fprintln(out, "leftover (intentionally preserved):")
	noted := false
	for _, p := range res.preservedLocal {
		fmt.Fprintf(out, "  %s\n", p)
		noted = true
	}
	for _, p := range res.preservedState {
		fmt.Fprintf(out, "  %s\n", p)
		noted = true
	}
	for _, p := range res.retainedProjectOwned {
		fmt.Fprintf(out, "  %s  (project-owned)\n", p)
		noted = true
	}
	// The .opencode/ root and project root are always preserved by design.
	opencodeRoot := filepath.Join(lm.dir, manifest.DirName)
	if _, err := os.Stat(opencodeRoot); err == nil {
		fmt.Fprintf(out, "  %s/  (root preserved; never removed by uninstall)\n", manifest.DirName)
		noted = true
	}
	if !noted {
		fmt.Fprintln(out, "  (nothing preserved — clean removal)")
	}
	fmt.Fprintln(out, "note: state dirs, .local/, and project-owned files are never auto-removed; delete manually if desired.")
}
