package resolver

// Phase 2 scope (this file): pack-manifest merge. MergeCatalogs combines the
// core Catalog (baseline agents + seed capabilities) with pack-contributed
// capability manifests into one merged Catalog, applying project-over-embedded
// shadowing and strict (fail-closed) cross-source validation. The merged
// Catalog is what Resolve consumes.
//
// This file does NOT load manifests from disk (that lives in internal/overlay,
// which reads a pack's capability-manifest.yml bytes and parses them via
// ParseManifest); MergeCatalogs takes already-parsed contributions so the merge
// rule is pure and unit-testable without a shipped pack.

import (
	"errors"
	"fmt"
	"sort"
)

// ContributionSource identifies whether a pack-manifest contribution came from
// the embedded overlays tree (shipped inside the binary) or from the consuming
// project's local .vh-agent-harness/overlays/ tree. It drives project-wins
// shadowing in MergeCatalogs.
type ContributionSource int

const (
	// SourceEmbedded marks a manifest contributed by a pack shipped inside the
	// binary (templates/overlays/<pack>/capability-manifest.yml).
	SourceEmbedded ContributionSource = iota
	// SourceProject marks a manifest contributed by a pack the consuming project
	// ships locally (.vh-agent-harness/overlays/<pack>/capability-manifest.yml).
	// A project contribution SHADOWS an embedded contribution of the same pack
	// name (project-wins); the project pack declares its OWN manifest with no
	// implicit dependency inheritance from the embedded pack it replaces.
	SourceProject
)

// String returns a human-readable source label for error messages and debug.
func (s ContributionSource) String() string {
	switch s {
	case SourceEmbedded:
		return "embedded"
	case SourceProject:
		return "project"
	default:
		return "unknown"
	}
}

// PackContribution is one pack's capability-manifest contribution toward the
// merged catalog, tagged with its pack name and source so MergeCatalogs can
// apply project-over-embedded shadowing before merging. Pack is the shadowing
// key: two contributions with the same Pack name but different Source resolve
// to the project one (the embedded contribution is dropped).
type PackContribution struct {
	// Pack is the contributing pack's name (the overlays/<name>/ directory).
	// Same name across sources => project wins.
	Pack string
	// Source is embedded vs project-local; drives shadowing.
	Source ContributionSource
	// Manifest is the parsed capability-manifest.yml body (parse-only; MergeCatalogs
	// validates the combined catalog rather than trusting each contribution).
	Manifest CapabilityManifest
}

// MergeCatalogs combines the core Catalog (its baseline agents and seed
// capabilities) with pack-contributed capability manifests into one merged
// Catalog, applying the two merge invariants the capability model requires:
//
//  1. Project-over-embedded shadowing: when a project-local pack and an embedded
//     pack share a pack name, the PROJECT pack's manifest REPLACES the embedded
//     pack's manifest (project-wins). The shadowing pack declares its OWN
//     manifest — there is NO implicit dependency inheritance from the shadowed
//     pack. This mirrors the existing same-name pack shadowing at FS discovery
//     (overlay.OpenPackFor: a project dir at .vh-agent-harness/overlays/<name>/
//     wins over the embedded tree). Encoding it here too keeps the merge
//     self-contained and unit-testable without a shipped pack.
//  2. Strict (fail-closed) cross-source validation, NOT last-writer-wins:
//     duplicate capability id across surviving sources = blocker; duplicate
//     provides agent-name across capabilities = blocker (an agent belongs to at
//     most one capability); unknown hard_dep = blocker; unknown optional_dep is
//     tolerated (forward/overlay references). These are exactly Catalog.
//     Validate's checks; MergeCatalogs builds the combined catalog and returns
//     its Validate errors joined. A duplicate OUTPUT PATH check is an extension
//     point (Phase-1 manifests do not yet carry output paths) — see
//     validateOutputPaths, currently a no-op.
//
// Core seed capabilities (core/gated-commit, core/debate) are NOT shadowable:
// a pack manifest reusing a core id is a duplicate-id blocker, not a shadow.
// The returned Catalog is a fresh copy; core is not mutated.
func MergeCatalogs(core *Catalog, contribs []PackContribution) (*Catalog, error) {
	if core == nil {
		return nil, fmt.Errorf("resolver: merge: core catalog is nil")
	}

	survivors := ResolveContributions(contribs)

	// Combined manifest list: core seed capabilities first (in their stable
	// registration order), then pack contributions in sorted pack-name order
	// (deterministic). Baseline always comes from core.
	combined := make([]CapabilityManifest, 0, len(core.caps)+len(survivors))
	combined = append(combined, core.caps...)
	for _, c := range survivors {
		combined = append(combined, c.Manifest)
	}

	merged := newCatalog(core.baseline, combined...)

	// Strict fail-closed validation: per-manifest structure, duplicate ids,
	// duplicate provides, disjoint baseline, unknown hard deps (unknown optional
	// deps tolerated), plus the extension-point output-path check.
	var errs []error
	errs = append(errs, merged.Validate()...)
	errs = append(errs, validateOutputPaths(merged)...)

	if len(errs) > 0 {
		return nil, fmt.Errorf("resolver: merge catalog: %w", errors.Join(errs...))
	}
	return merged, nil
}

// ResolveContributions applies project-over-embedded shadowing and returns the
// surviving pack contributions in sorted pack-name order. It is the exported
// shadowing entry point: it preserves each survivor's Pack name and Source, so
// a render caller can build the capability-id -> pack-name map it needs to
// render the packs owning the resolved capabilities (the Phase-3
// capability-installer overlay integration).
//
// Shadowing rule: for each pack name, a project-local contribution (if any)
// wins and the embedded contribution for that name is dropped. The shadowing
// pack REPLACES the shadowed pack's manifest entirely — it contributes its own
// id/provides/deps and inherits NOTHING from the embedded pack it shadows.
// Among contributions sharing the SAME (pack, source) — a discovery
// double-call — the first wins; that is a discovery concern, not a merge
// concern.
func ResolveContributions(contribs []PackContribution) []PackContribution {
	type pick struct {
		contrib PackContribution
	}
	chosen := make(map[string]pick)
	// First pass: seed with embedded contributions; second pass: project
	// contributions overwrite. This makes project-wins explicit and keeps the
	// logic readable regardless of input order.
	for _, c := range contribs {
		if c.Source == SourceEmbedded {
			if _, exists := chosen[c.Pack]; !exists {
				chosen[c.Pack] = pick{contrib: c}
			}
		}
	}
	for _, c := range contribs {
		if c.Source == SourceProject {
			// Project wins: overwrite any embedded (or earlier project) entry.
			chosen[c.Pack] = pick{contrib: c}
		}
	}
	names := make([]string, 0, len(chosen))
	for n := range chosen {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]PackContribution, 0, len(names))
	for _, n := range names {
		out = append(out, chosen[n].contrib)
	}
	return out
}

// validateOutputPaths is the extension point for a duplicate-output-path check
// across merged capabilities. Phase-1 capability manifests do NOT yet carry
// output paths (a capability contributes agents via Provides, not file paths),
// so this is currently a no-op returning nil. When a future phase adds a path
// field to CapabilityManifest, the duplicate-path blocker check lands here —
// keeping the merge's fail-closed contract in one place rather than scattering
// path-conflict detection across callers.
func validateOutputPaths(_ *Catalog) []error {
	return nil
}
