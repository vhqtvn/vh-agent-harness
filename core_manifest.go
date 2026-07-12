package corpus

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/ownership"
)

// substrateTemplateSuffix mirrors substrate.TemplateSuffix (".tmpl"). The
// renderer treats a corpus file whose name ends in this suffix as a Go
// text/template, parses it, and writes the output under the suffix-stripped name.
// substrateStaticTemplateSuffix mirrors substrate.TemplateAltSuffix (".template"):
// the renderer treats a corpus file whose name ends in this suffix as a static
// scaffold and writes the bytes verbatim under the suffix-stripped name.
// In both cases the ownership map keys by the LIVE (suffix-stripped) name so the
// classifier — which matches on the staged/live name — resolves managed template
// files correctly. Kept as local consts (not imports of internal/substrate) so
// the corpus package stays leaf-level with no risk of a future import cycle.
const (
	substrateTemplateSuffix       = ".tmpl"
	substrateStaticTemplateSuffix = ".template"
)

// CoreOwnershipDefaults walks the embedded curated corpus (CoreFS under
// CoreDir) and emits the platform-authored S2 module-default ownership map:
// every file path -> platform_managed (provenance "core"), EXCEPT the
// documented armed/owned exceptions that the platform ships with hand-protection
// (every exception below is mirrored exactly in the switch + coreExceptionsForDoc):
//
//   - .vh-agent-harness/vh-harness-profile.yml             -> platform_armed     (provenance "core.profile")
//   - .vh-agent-harness/config-transform.mjs               -> project_owned      (provenance "core.transform.project")
//   - .opencode/repo-configs/forbidden-patterns.project.js -> project_owned      (provenance "core.deny.project")
//   - .opencode/repo-configs/repo-recon-data.yml           -> external_generated (provenance "core.repo-recon.data")
//   - docs/planning/backlog.md, docs/planning/roadmap.md   -> project_owned      (provenance "core.planning")
//   - .gitignore, README.md, CLAUDE.md, Makefile           -> project_owned      (provenance "core.project-identity")
//
// This is the canonical ownership manifest for the core corpus. The seam's
// Classifier consumes the EffectiveMap produced by ownership.Resolve over these
// defaults; platform_managed paths are free-overwrite on update, while the
// exceptions exercise the armed-reconcile and owned-preserve / external-skip
// paths.
//
// The walk is robust to corpus growth: new files added to templates/core
// (including by later overlay packs that materialize into the core tree) are
// automatically classified platform_managed unless explicitly excepted here.
// Callers that need bespoke exceptions beyond these should layer an Overrides
// map via ownership.Resolve rather than editing this function.
func CoreOwnershipDefaults() (ownership.ModuleDefaults, error) {
	sub, err := fs.Sub(CoreFS, CoreDir)
	if err != nil {
		return nil, fmt.Errorf("core ownership walk: fs.Sub: %w", err)
	}
	out := ownership.ModuleDefaults{}
	err = fs.WalkDir(sub, ".", func(rel string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		// The renderer strips the TemplateSuffix (.tmpl) or the static
		// TemplateAltSuffix (.template) from staged output names, so a corpus
		// file "opencode.jsonc.tmpl" materializes in the live tree (and in
		// staging) as "opencode.jsonc", and "CLAUDE.md.template" / "Makefile.template"
		// materialize as "CLAUDE.md" / "Makefile". The ownership map is over LIVE
		// paths — the suffix is a render-time marker, not a path component — so
		// the map key MUST be the suffix-stripped name. Otherwise the classifier
		// (which matches on the staged/live name) would fail-closed on every
		// managed template file.
		liveRel := strings.TrimSuffix(rel, substrateTemplateSuffix)
		liveRel = strings.TrimSuffix(liveRel, substrateStaticTemplateSuffix)
		// liveRel is relative to the corpus root and uses forward slashes
		// (fs.FS convention); the classifier matches on the same forward-slash
		// rel paths the apply walk computes.
		rule := ownership.PathRule{
			Class:      ownership.ClassPlatformManaged,
			Provenance: "core",
		}
		switch liveRel {
		case ".vh-agent-harness/vh-harness-profile.yml":
			rule.Class = ownership.ClassPlatformArmed
			rule.Provenance = "core.profile"
		case "docs/planning/backlog.md", "docs/planning/roadmap.md":
			// Planning docs: the harness seeds a canonical starter on a greenfield
			// install, then NEVER clobbers — the backlog is the project's living
			// source of task truth that agents edit constantly. project_owned.
			rule.Class = ownership.ClassProjectOwned
			rule.Provenance = "core.planning"
		case ".gitignore", "README.md", "CLAUDE.md", "Makefile":
			// Project-identity files: the harness ships a generic scaffold, but
			// these belong to the consuming project (its ignores, its readme, its
			// make targets, its cross-agent rules). Seed the
			// scaffold once on a greenfield install, then NEVER clobber — a project
			// that already has any of these keeps its own. This is what makes the
			// harness safe to `install`/`update` into an EXISTING repo without a
			// per-project harness-ownership.yml override. The harness's own runtime
			// engine files (.opencode/package.json, AGENTS.core.md, etc.) stay
			// platform_managed; only these root project-identity files are owned.
			rule.Class = ownership.ClassProjectOwned
			rule.Provenance = "core.project-identity"
		case ".opencode/repo-configs/forbidden-patterns.project.js":
			// Project-owned deny-rule payload: harness seeds a blank scaffold on
			// first install, then preserves project edits forever. The generic
			// engine (forbidden-patterns.core.js) stays platform_managed.
			rule.Class = ownership.ClassProjectOwned
			rule.Provenance = "core.deny.project"
		case ".vh-agent-harness/config-transform.mjs":
			// Project-owned permission transform: harness seeds a blank no-op
			// scaffold on first install, then preserves project edits forever.
			// The types/helpers support file (config-transform.core.mjs) stays
			// platform_managed.
			rule.Class = ownership.ClassProjectOwned
			rule.Provenance = "core.transform.project"
		case ".opencode/repo-configs/repo-recon-data.yml":
			// Project-generated recon data: harness seeds a blank skeleton on
			// first install, then leaves it to the project's recon-generator
			// skill / project agents to maintain. external_generated.
			rule.Class = ownership.ClassExternalGenerated
			rule.Provenance = "core.repo-recon.data"
		}
		out[liveRel] = rule
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("core ownership walk: %w", err)
	}
	return out, nil
}

// CorePaths returns the sorted list of forward-slash relative file paths in the
// embedded core corpus. Useful for diagnostics, ownership audits, and tests that
// need to assert the curated set.
func CorePaths() ([]string, error) {
	sub, err := fs.Sub(CoreFS, CoreDir)
	if err != nil {
		return nil, err
	}
	var paths []string
	err = fs.WalkDir(sub, ".", func(rel string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

// coreExceptionsForDoc is the exhaustive set of non-managed exceptions, kept as a
// constant for reference and for tests that assert the exception surface.
var coreExceptionsForDoc = map[string]ownership.Class{
	".vh-agent-harness/vh-harness-profile.yml":             ownership.ClassPlatformArmed,
	".vh-agent-harness/config-transform.mjs":               ownership.ClassProjectOwned,
	".opencode/repo-configs/forbidden-patterns.project.js": ownership.ClassProjectOwned,
	".opencode/repo-configs/repo-recon-data.yml":           ownership.ClassExternalGenerated,
	// Planning docs: canonical starter seeded once, then project-owned (living backlog).
	"docs/planning/backlog.md": ownership.ClassProjectOwned,
	"docs/planning/roadmap.md": ownership.ClassProjectOwned,
	// Project-identity files: generic scaffold seeded once, then project-owned.
	".gitignore": ownership.ClassProjectOwned,
	"README.md":  ownership.ClassProjectOwned,
	"CLAUDE.md":  ownership.ClassProjectOwned,
	"Makefile":   ownership.ClassProjectOwned,
}
