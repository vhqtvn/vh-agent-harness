// Package resolver owns the capability model: the declaration type a pack ships
// (CapabilityManifest), the central core catalog of known capabilities and
// baseline agents (Catalog), and — in later phases — the resolver that computes
// the active agent set from a profile's capability selection.
//
// Phase 1 scope (this file + catalog.go): define the types, the catalog seed,
// and structural Validate. There is NO resolver algorithm yet (closure /
// cycle / shadowing detection lands in Phase 2), NO rendering gate (Phase 3),
// and NO overlay recognition of capability-manifest.yml (Phase 2/4).
//
// A "capability" is a coherent, optionally-selectable bundle of agents a profile
// may turn on (e.g. core/gated-commit brings the commit pipeline agents). It is
// a DISTINCT concept from the runtime backend capability matrix in
// internal/runtime/capability.go (which models which verbs a runtime backend
// supports). The two do not interact.
package resolver

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// capabilityIDRe is the grammar a capability ID must satisfy: exactly one '/'
// separating a namespace and a name, each lowercase alphanumeric (with dashes),
// starting with a letter. Examples: "core/gated-commit", "core/debate".
//
// Single-slash (one namespace level) is intentional for v1; nested namespaces
// ("vendor/group/name") can be relaxed later without breaking existing IDs
// because the seed IDs all satisfy the single-slash form.
var capabilityIDRe = regexp.MustCompile(`^[a-z][a-z0-9-]*/[a-z][a-z0-9-]*$`)

// isValidCapabilityID reports whether id matches the namespace/name grammar.
func isValidCapabilityID(id string) bool {
	return capabilityIDRe.MatchString(id)
}

// CapabilityManifest is a single capability declaration as a pack ships it in a
// capability-manifest.yml. It captures what the capability owns (Provides) and
// what it depends on at the capability level (HardDeps / OptionalDeps).
//
// YAML shape (one document per file):
//
//	id: core/gated-commit
//	provides:
//	  - commit-message
//	  - committer
//	hard_deps: []
//	optional_deps: []
//
// Field semantics:
//
//   - ID: the capability identifier, grammar namespace/name. Unique within a
//     Catalog (enforced by Catalog.Validate).
//   - Provides: the agent names this capability OWNS. Each agent name is owned
//     by at most one capability (enforced by Catalog.Validate). The resolver
//     (Phase 2) renders the Provides agents of every active capability.
//   - HardDeps: capability IDs this capability cannot function without. An
//     unknown HardDep is a validation ERROR (Catalog.Validate) because a hard
//     dependency on a non-existent capability is a broken declaration.
//   - OptionalDeps: capability IDs this capability can use if present but does
//     not require. An unknown OptionalDep is NOT a validation error: optional
//     edges may reference capabilities contributed by overlays or future packs
//     not present in the core catalog, and the resolver treats an unresolvable
//     optional dep as simply "not satisfied" rather than a failure.
//
// Internal agent-to-agent edges (e.g. committer -> commit-reviewer, both inside
// core/gated-commit) are NOT capability-level deps: a capability is atomic from
// the catalog's perspective, and intra-capability agent edges are resolved
// within the capability's own Provides set. Only CROSS-capability edges are
// modeled here.
type CapabilityManifest struct {
	ID           string   `yaml:"id"`
	Provides     []string `yaml:"provides"`
	HardDeps     []string `yaml:"hard_deps"`
	OptionalDeps []string `yaml:"optional_deps"`
}

// Validate reports every self-contained structural problem with a manifest. It
// does NOT perform I/O and does NOT check cross-capability concerns (duplicate
// IDs across manifests, hard-dep resolvability, provides uniqueness) — those
// belong to Catalog.Validate, which needs the full catalog context. An empty
// result means the manifest is structurally sound in isolation.
//
// Checks: non-empty ID matching the namespace/name grammar; each Provides entry
// non-empty and unique within the manifest; each HardDeps / OptionalDeps entry
// non-empty, well-formed, not a self-dependency, and unique within its list.
func (m CapabilityManifest) Validate() []error {
	var errs []error

	if m.ID == "" {
		errs = append(errs, fmt.Errorf("capability manifest: id is empty"))
	} else if !isValidCapabilityID(m.ID) {
		errs = append(errs, fmt.Errorf("capability manifest: id %q is not namespace/name (lowercase alphanumerics and dashes, single slash, e.g. core/gated-commit)", m.ID))
	}

	// Provides: non-empty, no duplicates within this manifest. Cross-capability
	// uniqueness is a Catalog.Validate concern.
	seenProvides := make(map[string]bool, len(m.Provides))
	for i, a := range m.Provides {
		if strings.TrimSpace(a) == "" {
			errs = append(errs, fmt.Errorf("capability %q: provides[%d] is empty", m.ID, i))
			continue
		}
		if seenProvides[a] {
			errs = append(errs, fmt.Errorf("capability %q: provides[%d] duplicate agent %q", m.ID, i, a))
		}
		seenProvides[a] = true
	}

	errs = append(errs, validateDepList(m.ID, "hard_deps", m.HardDeps)...)
	errs = append(errs, validateDepList(m.ID, "optional_deps", m.OptionalDeps)...)
	return errs
}

// validateDepList checks one dependency list (hard_deps or optional_deps) for
// well-formed IDs, no self-dependency, and no intra-list duplicates. It is
// shared by HardDeps and OptionalDeps; the resolvability difference (unknown
// hard = error, unknown optional = tolerated) is a Catalog.Validate concern.
func validateDepList(capID, field string, deps []string) []error {
	var errs []error
	seen := make(map[string]bool, len(deps))
	for i, d := range deps {
		if strings.TrimSpace(d) == "" {
			errs = append(errs, fmt.Errorf("capability %q: %s[%d] is empty", capID, field, i))
			continue
		}
		if !isValidCapabilityID(d) {
			errs = append(errs, fmt.Errorf("capability %q: %s[%d] %q is not a well-formed namespace/name", capID, field, i, d))
		}
		if d == capID {
			errs = append(errs, fmt.Errorf("capability %q: %s[%d] self-dependency %q", capID, field, i, d))
		}
		if seen[d] {
			errs = append(errs, fmt.Errorf("capability %q: %s[%d] duplicate dep %q", capID, field, i, d))
		}
		seen[d] = true
	}
	return errs
}

// ParseManifest decodes a capability-manifest.yml document into a
// CapabilityManifest. It does NOT Validate; call Validate to check structural
// integrity. A malformed YAML document is returned as an error.
func ParseManifest(raw []byte) (CapabilityManifest, error) {
	var m CapabilityManifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return CapabilityManifest{}, fmt.Errorf("parse capability-manifest: %w", err)
	}
	return m, nil
}

// MarshalManifest serializes a manifest to its capability-manifest.yml form.
// The output is deterministic for a given manifest (yaml.v3 sorts map keys; the
// slice fields serialize in source order). Round-trip ParseManifest ->
// MarshalManifest is stable.
func MarshalManifest(m CapabilityManifest) ([]byte, error) {
	out, err := yaml.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal capability-manifest: %w", err)
	}
	return out, nil
}
