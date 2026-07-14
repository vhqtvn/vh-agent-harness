package cli

// overlay_docs.go implements `vh-agent-harness overlay docs <name>` — the
// discovery command that prints a pack's README.md so adopters can read a
// pack's configuration reference and enablement steps WITHOUT rendering the
// pack or reading plugin source code.
//
// It resolves the pack the SAME way a render does: project-local first
// (target/.vh-agent-harness/overlays/<name>/) then the embedded overlays FS,
// via overlay.OpenPackFor. Pack READMEs are deliberately excluded from the
// rendered .opencode/ tree (overlay.IsPackDocFile), so this command is the
// canonical way to surface that documentation to a consumer who already has
// the binary but has not authored a project-local pack of the same name.

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vhqtvn/vh-agent-harness/internal/overlay"
	"github.com/vhqtvn/vh-agent-harness/internal/runshape"
)

// overlayDocsFl holds the inputs to `vh-agent-harness overlay docs`.
type overlayDocsFlags struct {
	target string
}

var overlayDocsFl *overlayDocsFlags

var overlayDocsCmd = &cobra.Command{
	Use:           "docs <name>",
	Short:         "Print a pack's README documentation (embedded or project-local)",
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `Print an overlay pack's README.md to stdout.

The pack is resolved the same way a render resolves it: a project-local pack
at .vh-agent-harness/overlays/<name>/ is preferred, then the embedded overlays
filesystem shipped inside the binary. This works for ANY pack name (shipped
packs like ` + "`auto-classifier-pilot`" + ` or ` + "`release`" + `, or your own
project-local packs).

Pack READMEs describe the pack itself — configuration reference, mode tables,
enablement steps — and are deliberately excluded from the rendered .opencode/
tree (so a pack README never pollutes .opencode/README.md). This command is the
way to read that documentation: it is the discovery mechanism for adopters who
need to configure a pack without reading its plugin source code.

Run after selecting a pack under ` + "`overlays:`" + ` to learn how to configure it,
or run on any pack name to evaluate it before adoption.`,
	Args: cobra.ExactArgs(1),
	RunE: runOverlayDocs,
}

func init() {
	overlayDocsFl = &overlayDocsFlags{target: "."}
	overlayDocsCmd.Flags().StringVarP(&overlayDocsFl.target, "target", "o", overlayDocsFl.target,
		"project root containing .vh-agent-harness/ (default: current directory)")

	overlayCmd.AddCommand(overlayDocsCmd)
}

// runOverlayDocs is the RunE for `vh-agent-harness overlay docs <name>`.
func runOverlayDocs(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()

	name := args[0]

	// Validate the pack name BEFORE resolving it: overlay.OpenPackFor joins
	// target/.vh-agent-harness/overlays/<name> with filepath.Join, which cleans
	// (not rejects) ".." segments, so an unvalidated name could escape the
	// overlays directory and surface an arbitrary readable directory's README.
	// nameRe is the canonical pack-name contract (see overlay_new.go) and also
	// rejects absolute paths, slashes, and leading separators.
	if !nameRe.MatchString(name) {
		return fmt.Errorf("overlay: invalid pack name %q (must match %s)", name, nameRe.String())
	}

	// Resolve the project root the same way guide.go does: runshape.FindForRoot
	// walks up from cwd for an install root; if that fails, fall back to the
	// --target flag resolved absolute. An empty target forces embedded-only
	// resolution (overlay.OpenPackFor documents target="" as embedded-only).
	projectRoot := ""
	target := strings.TrimSpace(overlayDocsFl.target)
	if abs, absErr := filepath.Abs(target); absErr == nil && abs != "" {
		if root, _, err := runshape.FindForRoot(abs); err == nil && root != "" {
			projectRoot = root
		} else if isExistingDir(abs) {
			projectRoot = abs
		}
	}

	pack, err := overlay.OpenPackFor(projectRoot, name)
	if err != nil {
		fmt.Fprintf(errOut, "error: overlay: pack %q not found (neither project-local nor embedded)\n", name)
		return errSilent{}
	}

	body, rErr := fs.ReadFile(pack.FS, "README.md")
	if rErr != nil {
		// Pack exists but has no README.md. List the documentation-shaped files
		// that DO exist (readme/license/changelog/contributing base names,
		// case-insensitive), and point at `doctor` for the unit inventory.
		docs := listPackDocFiles(pack.FS)
		if len(docs) == 0 {
			fmt.Fprintf(errOut, "error: pack %q has no README.md (and no other doc files: README/LICENSE/CHANGELOG/CONTRIBUTING)\n", name)
			fmt.Fprintf(errOut, "       list the pack's renderable units with `vh-agent-harness doctor`.\n")
		} else {
			fmt.Fprintf(errOut, "error: pack %q has no README.md. Documentation files present: %s\n", name, strings.Join(docs, ", "))
			fmt.Fprintf(errOut, "       list the pack's renderable units with `vh-agent-harness doctor`.\n")
		}
		return errSilent{}
	}

	fmt.Fprint(out, string(body))
	return nil
}

// listPackDocFiles walks the pack FS and returns the sorted list of files whose
// base name is a documentation file (README/LICENSE/CHANGELOG/CONTRIBUTING,
// case-insensitive) — i.e. the same set overlay.IsPackDocFile recognizes, but
// enumerated here so the no-README error can tell the operator what IS present.
func listPackDocFiles(fsys fs.FS) []string {
	var found []string
	_ = fs.WalkDir(fsys, ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if overlay.IsPackDocFile(rel) {
			found = append(found, rel)
		}
		return nil
	})
	sort.Strings(found)
	return found
}
