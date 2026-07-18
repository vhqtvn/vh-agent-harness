package resolver

import (
	"fmt"
	"sort"
	"strings"
)

// Catalog is the central registry of known capabilities and the baseline agent
// set. It is the resolver's ground truth: the capabilities the harness ships,
// the agent names each one owns (Provides), the capability-level dependency
// edges, and the always-on baseline agents. Phase 1 ships a hardcoded
// CoreCatalog(); Phase 2 adds the resolver that computes the active agent set
// from a profile's capability selection, and overlay packs may contribute
// additional CapabilityManifests.
//
// # Baseline representation decision
//
// The 8 universal agents (coordination, build, project-coordinator, researcher,
// repo-explorer, planner, docs-steward, ship-review) are modeled as a SEPARATE
// baseline list on the Catalog, NOT as an AlwaysOn-flagged capability. Rationale:
//
//  1. Honesty: a capability is something a profile SELECTS (opts into); the
//     baseline agents are infrastructural and never selected — they are always
//     rendered. Modeling them as a dedicated list keeps the capability set
//     purely about selectable bundles.
//  2. No synthetic ID: the alternative (an AlwaysOn bool on a fake
//     core/baseline capability) invents a capability ID that is not part of the
//     real capability taxonomy and adds a per-capability axis that only ever
//     applies to one entry.
//  3. Resolver integration (Phase 2): the active agent set is computed as
//     baseline ∪ Provides(activeCapabilities ∪ hardDepClosure). The baseline is
//     a single union operand; no special "always-active capability" branch.
//  4. Ownership invariant preserved: Catalog.Validate enforces that no baseline
//     agent is also a Provides agent of any capability, so every agent has
//     exactly one owner (baseline OR a capability) — the same invariant the
//     AlwaysOn-capability alternative would enforce implicitly via Provides
//     uniqueness.
type Catalog struct {
	// caps is the manifest list in registration (stable) order. Kept as a slice
	// (not just a map) so Validate can detect duplicate IDs and report
	// cross-capability provides collisions deterministically.
	caps []CapabilityManifest
	// byID indexes caps by ID for Get(). Last-wins on duplicate ID; Validate
	// flags the duplicate. Built once at construction.
	byID map[string]CapabilityManifest
	// provides indexes agent name -> owning capability ID for
	// CapabilityForAgent(). Last-wins on duplicate; Validate flags collisions.
	provides map[string]string
	// baseline is the always-on agent list (the 8 universal agents), in
	// registration order. Never selected; rendered unconditionally.
	baseline []string
}

// newCatalog builds a Catalog from the given baseline agent list and capability
// manifests, indexing them for lookup. It does NOT Validate; call Validate to
// check structural integrity (unique IDs, unique Provides, disjoint baseline,
// resolvable hard-deps). The inputs are copied; later mutation by the caller
// does not affect the catalog.
func newCatalog(baseline []string, caps ...CapabilityManifest) *Catalog {
	c := &Catalog{
		caps:     append([]CapabilityManifest(nil), caps...),
		byID:     make(map[string]CapabilityManifest, len(caps)),
		provides: make(map[string]string),
		baseline: append([]string(nil), baseline...),
	}
	for _, m := range caps {
		c.byID[m.ID] = m // last-wins; Validate flags duplicate IDs
		for _, a := range m.Provides {
			c.provides[a] = m.ID // last-wins; Validate flags collisions
		}
	}
	return c
}

// Get returns the capability manifest for id and ok=true if id is a known
// capability in this catalog.
func (c *Catalog) Get(id string) (CapabilityManifest, bool) {
	m, ok := c.byID[id]
	return m, ok
}

// CapabilityForAgent returns the ID of the capability that owns agent, and
// ok=true if agent is a Provides agent of some capability. Baseline agents are
// NOT owned by any capability (use HasAgent / BaselineAgents for those), so
// CapabilityForAgent returns ("", false) for a baseline agent.
func (c *Catalog) CapabilityForAgent(agent string) (string, bool) {
	id, ok := c.provides[agent]
	return id, ok
}

// BaselineAgents returns the always-on agent names (the 8 universal agents), in
// registration order. The returned slice is a copy; callers may mutate it.
func (c *Catalog) BaselineAgents() []string {
	out := make([]string, len(c.baseline))
	copy(out, c.baseline)
	return out
}

// IDs returns the sorted capability IDs in this catalog.
func (c *Catalog) IDs() []string {
	out := make([]string, 0, len(c.byID))
	for id := range c.byID {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// HasAgent reports whether agent is known to the catalog — either a baseline
// agent or a Provides agent of some capability.
func (c *Catalog) HasAgent(agent string) bool {
	if _, ok := c.provides[agent]; ok {
		return true
	}
	for _, b := range c.baseline {
		if b == agent {
			return true
		}
	}
	return false
}

// Validate reports every structural problem with the catalog as a whole. It
// does NOT perform I/O and does NOT compute an agent closure (that is the
// resolver's job in Phase 2). An empty result means the catalog is internally
// consistent.
//
// Checks:
//
//   - Each manifest passes its own (CapabilityManifest).Validate.
//   - Capability IDs are unique across the catalog.
//   - Provides agent names are unique across the catalog (each agent owned by
//     at most one capability).
//   - Baseline agents are non-empty, de-duplicated, and DISJOINT from every
//     capability's Provides (so an agent has exactly one owner).
//   - Every HardDeps entry resolves to a known capability ID in the catalog.
//   - Unknown OptionalDeps entries are TOLERATED (not an error): optional edges
//     may reference capabilities contributed by overlays or future packs not
//     present in the core catalog.
func (c *Catalog) Validate() []error {
	var errs []error

	// Per-manifest self-validation, plus collect IDs and Provides ownership for
	// the cross-capability checks below.
	idCount := make(map[string]int, len(c.caps))
	owner := make(map[string]string) // agent -> capability ID that owns it
	for _, m := range c.caps {
		idCount[m.ID]++
		errs = append(errs, m.Validate()...)
	}
	// Duplicate IDs (across the catalog).
	for id, n := range idCount {
		if n > 1 {
			errs = append(errs, fmt.Errorf("catalog: duplicate capability id %q (%d occurrences)", id, n))
		}
	}
	// Provides uniqueness across capabilities. Iterate caps in stable order so
	// the error names a deterministic "first owner" when two capabilities claim
	// the same agent.
	for _, m := range c.caps {
		for _, a := range m.Provides {
			if prev, ok := owner[a]; ok {
				errs = append(errs, fmt.Errorf("catalog: agent %q provided by both %q and %q", a, prev, m.ID))
			} else {
				owner[a] = m.ID
			}
		}
	}
	// Baseline: non-empty, de-duplicated, disjoint from Provides.
	baselineSeen := make(map[string]bool, len(c.baseline))
	for i, a := range c.baseline {
		if strings.TrimSpace(a) == "" {
			errs = append(errs, fmt.Errorf("catalog: baseline[%d] is empty", i))
			continue
		}
		if baselineSeen[a] {
			errs = append(errs, fmt.Errorf("catalog: baseline[%d] duplicate agent %q", i, a))
		}
		if _, provided := owner[a]; provided {
			errs = append(errs, fmt.Errorf("catalog: baseline agent %q is also provided by capability %q (ambiguous ownership)", a, owner[a]))
		}
		baselineSeen[a] = true
	}
	// Hard-dep resolvability: unknown hard deps are errors. Unknown optional
	// deps are tolerated (forward-references to overlay/future capabilities).
	known := make(map[string]bool, len(idCount))
	for id := range idCount {
		known[id] = true
	}
	for _, m := range c.caps {
		for _, d := range m.HardDeps {
			if !known[d] {
				errs = append(errs, fmt.Errorf("catalog: capability %q has unknown hard_dep %q", m.ID, d))
			}
		}
	}
	return errs
}

// CoreCatalog returns the hardcoded central core catalog: the capabilities and
// baseline agents the harness ships. This is the ground truth the resolver
// (Phase 2) consults; Phase 1 only establishes it and validates its integrity.
//
// The seed encodes the agent-capability audit:
//
//   - core/gated-commit owns the gated-commit pipeline agents (commit-message,
//     the commit-reviewer cascade leaves, and committer). It is SELF-CONTAINED:
//     it has NO capability-level hard_deps. The committer -> commit-reviewer
//     edge is an INTERNAL agent-level edge (one agent depending on another
//     within the same capability), not a cross-capability dependency, so it is
//     not modeled here.
//   - core/debate owns the debate pipeline agents (debate and its roles plus
//     solution-brief). Also self-contained: no capability-level hard_deps.
//   - core/media-perception owns a single read-only perception specialist
//     (media-perception) plus its caller-facing skill. Opt-in: not in any
//     profile preset, so it renders only when a project explicitly selects it.
//     Self-contained: no capability-level hard_deps. The four inbound caller
//     edges (build, coordination, project-coordinator, researcher →
//     media-perception) live in permconfig.CoreTaskRules and are dropped at
//     emit time by the present-agent filter when the capability is not
//     selected (so an unselected capability leaves no dangling task edge).
//   - The 8 universal agents are the always-on baseline (see the Catalog
//     baseline-representation note).
//
// Future phases (NOT in this seed) will add: cross-capability OPTIONAL edges
// (e.g. baseline agents build/coordination/docs-steward -> committer, modeled
// as agent-level optional edges by the resolver), and a `release` capability
// that will hard-depend on core/gated-commit. The types here accommodate those
// without rework: OptionalDeps already tolerates unknown IDs, and HardDeps will
// resolve once the referenced capability exists.
func CoreCatalog() *Catalog {
	return newCatalog(
		// The 8 universal agents — always rendered, never selected. Modeled as
		// the Catalog baseline (not a capability); see the Catalog doc comment.
		[]string{
			"coordination",
			"build",
			"project-coordinator",
			"researcher",
			"repo-explorer",
			"planner",
			"docs-steward",
			"ship-review",
		},
		CapabilityManifest{
			ID: "core/gated-commit",
			Provides: []string{
				"commit-message",
				"commit-reviewer",
				"commit-reviewer-a",
				"commit-reviewer-b",
				"commit-reviewer-c",
				"commit-reviewer-d",
				"committer",
			},
			// No capability-level hard_deps: gated-commit is self-contained.
			// Intra-capability agent edges (committer -> commit-reviewer) are
			// resolved within the capability's own Provides set, not modeled as
			// capability deps.
		},
		CapabilityManifest{
			ID: "core/debate",
			Provides: []string{
				"debate",
				"debate-proposer",
				"debate-critic",
				"debate-synth",
				"solution-brief",
			},
			// Self-contained; no capability-level hard_deps.
		},
		CapabilityManifest{
			ID:       "core/media-perception",
			Provides: []string{"media-perception"},
			// Self-contained; no capability-level hard_deps. Opt-in: not in
			// any profile preset. Inbound caller edges from baseline agents
			// live in permconfig.CoreTaskRules and are dropped by Emit's
			// present-agent filter when this capability is unselected.
		},
	)
}
