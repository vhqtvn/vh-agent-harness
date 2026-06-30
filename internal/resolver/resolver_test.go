package resolver

// Phase 2 resolver engine tests. Cover the Resolve contract:
//   - transitive hard-dep closure reaches all transitively-required caps
//   - optional deps excluded unless explicitly selected (graceful absence)
//   - an explicitly-selected optional dep IS included (it is a root)
//   - hard-dep cycle rejected with a clear, path-naming error
//   - unknown hard dep rejected (defense-in-depth on an unvalidated catalog)
//   - unknown optional dep tolerated (forward/overlay reference)
//   - baseline agents always present in every CapabilitySet
//   - empty selection -> baseline-only set
//   - unknown selected id and nil catalog fail closed
//   - Provides agents of the closure are projected into Agents()
//
// Test catalogs are built with the unexported newCatalog (same package) so the
// tests can construct deliberately-broken catalogs (cycles, unknown hard deps)
// that CoreCatalog/Validate would never produce, exercising Resolve's own
// guards rather than re-testing Catalog.Validate.

import (
	"strings"
	"testing"
)

// threeChainCatalog builds A -> B -> C over hard deps, each providing one
// distinct agent. Used by closure / projection tests.
func threeChainCatalog() *Catalog {
	return newCatalog([]string{"base-one", "base-two"},
		CapabilityManifest{ID: "core/a", Provides: []string{"agent-a"}, HardDeps: []string{"core/b"}},
		CapabilityManifest{ID: "core/b", Provides: []string{"agent-b"}, HardDeps: []string{"core/c"}},
		CapabilityManifest{ID: "core/c", Provides: []string{"agent-c"}},
	)
}

func TestResolve_ClosureReachesTransitiveHardDeps(t *testing.T) {
	c := threeChainCatalog()
	set, err := Resolve([]string{"core/a"}, c)
	if err != nil {
		t.Fatalf("Resolve: unexpected error: %v", err)
	}
	// Selecting core/a must pull in core/b and core/c transitively (full
	// closure, not minimal cover).
	for _, id := range []string{"core/a", "core/b", "core/c"} {
		if !set.Has(id) {
			t.Errorf("closure missing transitive hard dep %q; got All()=%v", id, set.All())
		}
	}
	if len(set.All()) != 3 {
		t.Errorf("closure size: got %d (%v), want 3", len(set.All()), set.All())
	}
}

func TestResolve_FullClosureNotMinimalCover(t *testing.T) {
	// Diamond: D depends on B and C; both B and C depend on A. Minimal cover
	// would be {B or C, A}; full closure must include BOTH B and C (every
	// package manager resolves the full transitive set).
	c := newCatalog(nil,
		CapabilityManifest{ID: "core/a", Provides: []string{"agent-a"}},
		CapabilityManifest{ID: "core/b", Provides: []string{"agent-b"}, HardDeps: []string{"core/a"}},
		CapabilityManifest{ID: "core/c", Provides: []string{"agent-c"}, HardDeps: []string{"core/a"}},
		CapabilityManifest{ID: "core/d", Provides: []string{"agent-d"}, HardDeps: []string{"core/b", "core/c"}},
	)
	set, err := Resolve([]string{"core/d"}, c)
	if err != nil {
		t.Fatalf("Resolve: unexpected error: %v", err)
	}
	for _, id := range []string{"core/a", "core/b", "core/c", "core/d"} {
		if !set.Has(id) {
			t.Errorf("full closure must include %q (no minimal-cover pruning); got %v", id, set.All())
		}
	}
}

func TestResolve_OptionalDepsExcludedUnlessSelected(t *testing.T) {
	// core/a optionally depends on core/b. Selecting only core/a must NOT pull
	// core/b into the closure (graceful absence, npm optionalDependencies).
	c := newCatalog(nil,
		CapabilityManifest{ID: "core/a", Provides: []string{"agent-a"}, OptionalDeps: []string{"core/b"}},
		CapabilityManifest{ID: "core/b", Provides: []string{"agent-b"}},
	)
	set, err := Resolve([]string{"core/a"}, c)
	if err != nil {
		t.Fatalf("Resolve: unexpected error: %v", err)
	}
	if set.Has("core/b") {
		t.Errorf("optional dep core/b must NOT be in the closure unless selected; got %v", set.All())
	}
	if !set.Has("core/a") {
		t.Errorf("selected core/a must be in the closure; got %v", set.All())
	}
	// agent-b (owned only by the absent optional core/b) must not render.
	if containsString(set.Agents(), "agent-b") {
		t.Errorf("agent-b must not render when its capability is an unselected optional dep; got %v", set.Agents())
	}
}

func TestResolve_OptionalDepExplicitlySelectedIncluded(t *testing.T) {
	// An optional dep that is ITSELF selected is included: it is a root like
	// any other selected id, so its own closure is computed normally.
	c := newCatalog(nil,
		CapabilityManifest{ID: "core/a", Provides: []string{"agent-a"}, OptionalDeps: []string{"core/b"}},
		CapabilityManifest{ID: "core/b", Provides: []string{"agent-b"}},
	)
	set, err := Resolve([]string{"core/a", "core/b"}, c)
	if err != nil {
		t.Fatalf("Resolve: unexpected error: %v", err)
	}
	if !set.Has("core/a") || !set.Has("core/b") {
		t.Errorf("explicitly-selected optional dep must be included; got %v", set.All())
	}
}

func TestResolve_CycleRejected(t *testing.T) {
	// A -> B -> C -> A hard-dep cycle. Note this catalog Validates CLEAN:
	// Catalog.Validate checks unknown hard deps but NOT cycles, so cycle
	// detection is Resolve's own responsibility.
	c := newCatalog(nil,
		CapabilityManifest{ID: "core/a", Provides: []string{"agent-a"}, HardDeps: []string{"core/b"}},
		CapabilityManifest{ID: "core/b", Provides: []string{"agent-b"}, HardDeps: []string{"core/c"}},
		CapabilityManifest{ID: "core/c", Provides: []string{"agent-c"}, HardDeps: []string{"core/a"}},
	)
	if errs := c.Validate(); len(errs) != 0 {
		t.Fatalf("setup invariant: cyclic catalog should Validate clean (cycle detection is Resolve's job), got: %+v", errs)
	}
	_, err := Resolve([]string{"core/a"}, c)
	if err == nil {
		t.Fatalf("Resolve must reject a hard-dep cycle, got nil error")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("cycle error must mention 'cycle'; got: %v", err)
	}
	// The error must name the cycle members so a human can repair it.
	for _, id := range []string{"core/a", "core/b", "core/c"} {
		if !strings.Contains(err.Error(), id) {
			t.Errorf("cycle error should name %q; got: %v", id, err)
		}
	}
}

func TestResolve_SelfCycleRejected(t *testing.T) {
	// A manifest declaring a hard-dep on itself is rejected by manifest
	// Validate already; here we exercise a 2-node cycle A<->B to confirm the
	// back-edge is caught regardless of entry point.
	c := newCatalog(nil,
		CapabilityManifest{ID: "core/a", Provides: []string{"agent-a"}, HardDeps: []string{"core/b"}},
		CapabilityManifest{ID: "core/b", Provides: []string{"agent-b"}, HardDeps: []string{"core/a"}},
	)
	if _, err := Resolve([]string{"core/b"}, c); err == nil {
		t.Fatalf("Resolve must reject the A<->B cycle when entered from core/b")
	}
}

func TestResolve_UnknownHardDepRejected(t *testing.T) {
	// Catalog built via newCatalog (bypassing Validate) with a hard dep on a
	// capability that is not in the catalog. Resolve must fail closed even
	// though the catalog was not pre-validated.
	c := newCatalog(nil,
		CapabilityManifest{ID: "core/a", Provides: []string{"agent-a"}, HardDeps: []string{"core/ghost"}},
	)
	_, err := Resolve([]string{"core/a"}, c)
	if err == nil {
		t.Fatalf("Resolve must reject an unknown hard dep, got nil")
	}
	for _, want := range []string{`core/a`, `core/ghost`, "unknown hard_dep"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("unknown-hard-dep error should mention %q; got: %v", want, err)
		}
	}
}

func TestResolve_TransitiveUnknownHardDepRejected(t *testing.T) {
	// The unknown hard dep is two hops deep: selecting A reaches B, whose hard
	// dep C is unknown. Resolve must surface the error naming B and C.
	c := newCatalog(nil,
		CapabilityManifest{ID: "core/a", Provides: []string{"agent-a"}, HardDeps: []string{"core/b"}},
		CapabilityManifest{ID: "core/b", Provides: []string{"agent-b"}, HardDeps: []string{"core/c-missing"}},
	)
	_, err := Resolve([]string{"core/a"}, c)
	if err == nil {
		t.Fatalf("Resolve must reject a transitive unknown hard dep")
	}
	if !strings.Contains(err.Error(), "core/c-missing") {
		t.Errorf("transitive unknown-hard-dep error must name the missing dep; got: %v", err)
	}
}

func TestResolve_UnknownOptionalDepTolerated(t *testing.T) {
	// An optional dep referencing a capability NOT in the catalog must NOT
	// error: optional edges may point at overlay/future capabilities, and the
	// resolver treats an unresolvable optional dep as "not satisfied".
	c := newCatalog(nil,
		CapabilityManifest{ID: "core/a", Provides: []string{"agent-a"}, OptionalDeps: []string{"core/overlay-only"}},
	)
	set, err := Resolve([]string{"core/a"}, c)
	if err != nil {
		t.Fatalf("unknown optional dep must be tolerated, got: %v", err)
	}
	if !set.Has("core/a") {
		t.Errorf("core/a must still resolve; got %v", set.All())
	}
}

func TestResolve_BaselineAlwaysPresent(t *testing.T) {
	c := threeChainCatalog()
	set, err := Resolve([]string{"core/c"}, c)
	if err != nil {
		t.Fatalf("Resolve: unexpected error: %v", err)
	}
	// The baseline agents are present regardless of which capability is
	// selected (here only core/c, which owns agent-c).
	for _, b := range []string{"base-one", "base-two"} {
		if !containsString(set.Baseline(), b) {
			t.Errorf("baseline agent %q must be carried by the CapabilitySet", b)
		}
		if !containsString(set.Agents(), b) {
			t.Errorf("baseline agent %q must render via Agents()", b)
		}
	}
	// Selecting core/c brings agent-c; baseline ∪ {agent-c}.
	if !containsString(set.Agents(), "agent-c") {
		t.Errorf("agent-c (Provides of core/c) must render; got %v", set.Agents())
	}
}

// TestResolve_BaselineAgentsNeverSatisfyCapabilityGate is the Phase 3
// carry-forward of the F1 negative-pin assertion: a baseline agent name is
// NEVER a capability ID in the resolved closure, even when every seed capability
// is selected. This pins the exact invariant the Phase 3 template gates depend
// on — CapabilitySet.Has only consults the capability closure, so a baseline
// agent can never accidentally satisfy a {{- if .capabilities.<id> }} gate and
// force its cluster to render. The template gates are predicated on capability
// IDs (core/gated-commit, core/debate), never on baseline agent names, so this
// test guards against a future regression that conflated the two.
func TestResolve_BaselineAgentsNeverSatisfyCapabilityGate(t *testing.T) {
	c := CoreCatalog()
	// Select the full backward-compat default so the closure is non-empty and
	// Has has real capability IDs to report true for (positive control below).
	set, err := Resolve([]string{"core/gated-commit", "core/debate"}, c)
	if err != nil {
		t.Fatalf("Resolve: unexpected error: %v", err)
	}
	// Positive control: the selected capabilities DO satisfy Has, proving the
	// assertions below are exercising a populated closure, not a vacuous one.
	if !set.Has("core/gated-commit") || !set.Has("core/debate") {
		t.Fatalf("positive control failed: selected capabilities must satisfy Has(); got All()=%v", set.All())
	}
	// Negative pin: every baseline agent name returns Has()==false. Baseline
	// agents are infrastructural (never selected), so they never appear in the
	// capability closure that Has consults.
	for _, b := range c.BaselineAgents() {
		if set.Has(CapabilityID(b)) {
			t.Errorf("baseline agent %q must NOT satisfy Has() (baseline is not a capability id); All()=%v", b, set.All())
		}
	}
}

func TestResolve_EmptySelectionBaselineOnly(t *testing.T) {
	// A profile that selects no capabilities still renders the baseline. The
	// closure is empty and Agents() == Baseline().
	c := threeChainCatalog()
	set, err := Resolve(nil, c)
	if err != nil {
		t.Fatalf("Resolve(empty): unexpected error: %v", err)
	}
	if len(set.All()) != 0 {
		t.Errorf("empty selection must yield an empty closure; got %v", set.All())
	}
	agents := set.Agents()
	if len(agents) != 2 {
		t.Errorf("Agents() should be baseline-only (2 agents); got %v", agents)
	}
	for _, b := range c.BaselineAgents() {
		if !containsString(agents, b) {
			t.Errorf("baseline agent %q missing from Agents(); got %v", b, agents)
		}
	}
}

func TestResolve_UnknownSelectedRejected(t *testing.T) {
	c := threeChainCatalog()
	_, err := Resolve([]string{"core/never-existed"}, c)
	if err == nil {
		t.Fatalf("Resolve must reject an unknown selected capability")
	}
	if !strings.Contains(err.Error(), "core/never-existed") {
		t.Errorf("unknown-selected error must name the id; got: %v", err)
	}
}

func TestResolve_NilCatalogErrors(t *testing.T) {
	if _, err := Resolve([]string{"core/a"}, nil); err == nil {
		t.Fatalf("Resolve must fail closed on a nil catalog")
	}
}

func TestResolve_ProvidesAgentsProjectedAndDeduped(t *testing.T) {
	// Agents() is baseline ∪ union(Provides over closure), de-duped + sorted.
	c := newCatalog([]string{"shared-baseline"}, // baseline disjoint from provides
		CapabilityManifest{ID: "core/a", Provides: []string{"agent-a", "agent-shared"}},
		CapabilityManifest{ID: "core/b", Provides: []string{"agent-b", "agent-shared"}, HardDeps: []string{"core/a"}},
	)
	set, err := Resolve([]string{"core/b"}, c)
	if err != nil {
		t.Fatalf("Resolve: unexpected error: %v", err)
	}
	// core/b pulls core/a (hard dep); both Provide sets union with baseline.
	for _, a := range []string{"shared-baseline", "agent-a", "agent-b", "agent-shared"} {
		if !containsString(set.Agents(), a) {
			t.Errorf("Agents() missing %q; got %v", a, set.Agents())
		}
	}
	// No duplicates (agent-shared provided by both must appear once).
	seen := map[string]int{}
	for _, a := range set.Agents() {
		seen[a]++
	}
	for a, n := range seen {
		if n > 1 {
			t.Errorf("Agents() has duplicate %q (%d times)", a, n)
		}
	}
}

func TestResolve_AccessorsReturnCopies(t *testing.T) {
	c := threeChainCatalog()
	set, err := Resolve([]string{"core/a"}, c)
	if err != nil {
		t.Fatalf("Resolve: unexpected error: %v", err)
	}
	// Mutating returned slices must not affect the set.
	all := set.All()
	all[0] = "MUTATED"
	again := set.All()
	if containsString(again, "MUTATED") {
		t.Errorf("All() must return a defensive copy")
	}
	agents := set.Agents()
	agents[0] = "MUTATED"
	if containsString(set.Agents(), "MUTATED") {
		t.Errorf("Agents() must return a defensive copy")
	}
	base := set.Baseline()
	base[0] = "MUTATED"
	if containsString(set.Baseline(), "MUTATED") {
		t.Errorf("Baseline() must return a defensive copy")
	}
}

// containsString reports whether s contains x. Local helper (the overlay
// package's contains is not exported); kept here to avoid pulling sort/search
// for a trivial linear scan over small test slices.
func containsString(s []string, x string) bool {
	for _, v := range s {
		if v == x {
			return true
		}
	}
	return false
}
