package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	corpus "github.com/vhqtvn/vh-agent-harness"
	"github.com/vhqtvn/vh-agent-harness/internal/lineage"
	"github.com/vhqtvn/vh-agent-harness/internal/ownership"
	"github.com/vhqtvn/vh-agent-harness/internal/substrate"
)

// seamInventory renders the full core corpus + active overlays into a fresh
// out-of-tree staging dir (the EXACT pipeline seamApply and doctor use) and
// resolves the effective ownership class for every staged path. It is the seam
// equivalent of the legacy manifest: the authoritative inventory of every file
// the harness manages, derived LIVE from the embedded corpus + the project's S3
// profile / S2 overrides rather than from a tracked manifest file.
//
// The returned staging dir is the caller's to remove (defer os.RemoveAll). eff
// maps every classified path (core defaults + overlay_extension entries) to its
// resolved class; staged is the sorted list of every rel path the renderer
// actually wrote (a superset of eff for post-render composed files such as
// AGENTS.md). Callers classify a staged path with eff.ClassOf(rel).
func seamInventory(target string) (staging string, eff ownership.EffectiveMap, staged []string, err error) {
	sub, err := coreSubFSImpl()
	if err != nil {
		return "", nil, nil, err
	}
	staging, err = os.MkdirTemp("", "harness-seam-staging-*")
	if err != nil {
		return "", nil, nil, fmt.Errorf("create staging: %w", err)
	}
	// On any error past this point the staging dir is removed so callers never
	// leak it on the failure path (they only defer-clean on success).
	cleanup := func(e error) (string, ownership.EffectiveMap, []string, error) {
		os.RemoveAll(staging)
		return "", nil, nil, e
	}

	answers := mergeRenderAnswers(installRenderAnswers(target), readProfileAnswers(target))
	r := substrate.EmbedFSRenderer{Source: sub}
	overlayFiles, rerr := renderSeamStaging(staging, r, answers, target)
	if rerr != nil {
		return cleanup(rerr)
	}

	defaults, derr := corpus.CoreOwnershipDefaults()
	if derr != nil {
		return cleanup(fmt.Errorf("core ownership: %w", derr))
	}
	for _, rel := range overlayFiles {
		defaults[rel] = ownership.PathRule{
			Class:      ownership.ClassOverlayExtension,
			Provenance: "overlay",
		}
	}
	overrides, oerr := readOwnershipOverrides(target)
	if oerr != nil {
		return cleanup(fmt.Errorf("read ownership overrides: %w", oerr))
	}
	eff, rverr := ownership.Resolve(defaults, overrides)
	if rverr != nil {
		return cleanup(fmt.Errorf("ownership resolve (raise-only): %w", rverr))
	}

	for rel := range walkStagedLivePaths(staging) {
		staged = append(staged, rel)
	}
	sort.Strings(staged)
	return staging, eff, staged, nil
}

// isSeamInstalled reports whether target carries an S1 lineage record (i.e. it
// was installed/updated through the seam). A present-but-unreadable lineage is
// treated as installed (true) so callers surface the seam error rather than
// silently dropping to the legacy manifest path. Absent lineage => false.
func isSeamInstalled(target string) bool {
	lin, err := lineage.Read(target)
	if err != nil {
		return true // leaked/unparseable: belongs to the seam path, let it report
	}
	return lin != nil
}

// liveBytes reads a rel path under root, returning (bytes, exists). A read error
// other than not-exist is reported via exists=false too; callers that need to
// distinguish use os.ReadFile directly.
func liveBytes(root, rel string) ([]byte, bool) {
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return nil, false
	}
	return b, true
}
