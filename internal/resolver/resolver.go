package resolver

// Phase 2 scope (this file): the resolver engine. Resolve computes the active
// CapabilitySet from a profile's explicit capability selection and an
// already-merged Catalog: the full transitive closure over HardDeps, with cycle
// detection, optional-dep graceful absence, and the baseline agent roster.
//
// This file does NOT wire Resolve into the render seam (Phase 3), does NOT
// expand profile presets to a selection list (Phase 3), and does NOT do
// post-render dangling-ref validation (Phase 4). Resolve is PURE: a Catalog and
// a selection list in, a CapabilitySet out.

import (
	"fmt"
	"sort"
	"strings"
)

// CapabilityID is the capability identifier type (namespace/name, e.g.
// "core/gated-commit"). It is a TYPE ALIAS for string so it interchanges freely
// with the Phase-1 string-typed CapabilityManifest.ID and selected-list entries;
// the alias exists for documentation clarity, not to introduce a distinct type
// (no conversion is ever required).
type CapabilityID = string

// CapabilitySet is the resolver's output: the closed set of active capabilities
// (the selection plus its full transitive HardDep closure) together with the
// always-on baseline agent roster, projected to the agent set that should
// render.
//
// # Baseline representation decision
//
// The set carries a COPY of the catalog's baseline agent list (the 8 universal
// agents), exposed via Baseline() and folded into Agents(). Rationale:
//
//  1. Phase 1 models baseline as a SEPARATE list on Catalog (not an AlwaysOn
//     capability), because baseline agents are infrastructural and never
//     selected. CapabilitySet preserves that distinction: baseline lives in a
//     parallel field, NOT in the caps map, so Has("core/gated-commit") answers
//     the capability-gate question and a baseline agent name never accidentally
//     satisfies a capability gate.
//  2. A CapabilitySet represents "what renders". Baseline always renders, so
//     carrying it makes the set SELF-CONTAINED: the Phase-3 render seam calls
//     set.Agents() and gets the full roster with no separate baseline-merge
//     step. This is the "single union operand" Phase 1's Catalog doc describes:
//     active agents = baseline ∪ Provides(closure).
//  3. The closure (caps) stays purely about SELECTABLE capabilities, preserving
//     the Phase-1 invariant that a capability is something a profile opts into.
//
// Optional deps are intentionally absent from every CapabilitySet unless they
// were themselves explicitly selected (graceful absence, mirroring npm
// optionalDependencies and semantic-release "skip if none" semantics).
type CapabilitySet struct {
	// caps is the resolved capability-ID set: the selected IDs plus the full
	// transitive closure over HardDeps (fixpoint). OptionalDeps are NOT
	// traversed; an optional dep is included only if it was itself selected.
	caps map[CapabilityID]bool
	// baseline is a copy of the catalog's always-on agent list, carried so the
	// set is self-contained for rendering. Never selected; always rendered.
	baseline []string
	// agents is the cached projection baseline ∪ Provides(caps), sorted and
	// de-duplicated, computed once at Resolve time.
	agents []string
}

// Has reports whether capability id is in the resolved closure (it was selected
// or pulled in as a transitive hard dep). Baseline agents are NOT capabilities;
// test those via Baseline() / Agents().
func (s *CapabilitySet) Has(id CapabilityID) bool {
	return s.caps[id]
}

// All returns the sorted capability IDs in the resolved closure (selected plus
// transitive hard deps). The returned slice is a copy; callers may mutate it.
func (s *CapabilitySet) All() []CapabilityID {
	out := make([]CapabilityID, 0, len(s.caps))
	for id := range s.caps {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Baseline returns a copy of the always-on baseline agent list carried by this
// set (the 8 universal agents). Always rendered, never selected.
func (s *CapabilitySet) Baseline() []string {
	out := make([]string, len(s.baseline))
	copy(out, s.baseline)
	return out
}

// Agents returns the full agent roster this set renders: the baseline agents
// UNION the Provides agents of every capability in the closure, sorted and
// de-duplicated. This is the projection a render seam iterates to decide which
// agent blocks to emit. The returned slice is a copy; callers may mutate it.
func (s *CapabilitySet) Agents() []string {
	out := make([]string, len(s.agents))
	copy(out, s.agents)
	return out
}

// Resolve computes the active CapabilitySet for a selection: the selected
// capability IDs plus the full transitive closure over their HardDeps, with the
// catalog's baseline agents carried alongside. It is PURE — no I/O, no profile
// parsing; the caller supplies an already-merged Catalog and the explicit
// selection list (Phase 3 wires profile preset-expansion -> this function).
//
// Algorithm (single DFS pass, white/gray/black coloring):
//
//  1. Each selected ID must be a known capability in the catalog; an unknown
//     selected ID is a hard error (the profile names something that does not
//     exist in the merged catalog).
//  2. DFS from each selected ID over HardDeps edges. White = unvisited, gray =
//     on the current recursion stack, black = fully processed. A gray->gray
//     edge is a back-edge, i.e. a cycle: hard error naming the cycle path.
//  3. Every capability reached (selected + transitive hard deps) joins the
//     closure. OptionalDeps are NOT traversed (graceful absence); an optional
//     dep is included only if it was itself explicitly selected (it is then a
//     DFS root like any other selected id).
//  4. Defense-in-depth: every HardDeps entry reached must resolve to a known
//     capability; an unknown hard dep is a hard error with a message naming the
//     parent and the missing dep. (A Catalog that passed Validate already
//     guarantees this; the check makes Resolve robust when called on an
//     unvalidated catalog and yields a clearer error than a silent skip.)
//  5. The CapabilitySet's agent roster is baseline ∪ Provides(closure).
//
// # Cycle detection: DFS coloring vs Tarjan SCC
//
// DFS coloring is chosen over Tarjan strongly-connected-components because
// Resolve needs BOTH the transitive closure AND cycle detection in a single
// pass. Coloring detects a back-edge (cycle) during the SAME traversal that
// accumulates the closure, and the recursion stack reconstructs the exact cycle
// path for the error message. Tarjan SCC computes ALL strongly-connected
// components — more than required, since we only care whether ANY cycle is
// reachable from the selection — and would need a separate closure pass
// afterward. Coloring is simpler, single-pass, and sufficient. (Tarjan would be
// the right pick if we needed to enumerate every independent cycle for a
// repair tool; Phase 2 only needs to fail-closed on the first cycle.)
//
// # Full closure, not minimal cover
//
// The FULL transitive closure is computed (every reachable capability, not a
// minimal subset whose removal still satisfies all hard deps). This matches
// every real package manager (npm/pip/cargo resolve the full transitive set)
// and is the only correct choice given capabilities are not versioned within a
// single resolve; minimal cover is NP-hard and offers no benefit here.
//
// Resolve takes a *Catalog (pointer) to match Phase 1's pointer-receiver
// convention (CoreCatalog returns *Catalog; all Catalog methods are on *Catalog)
// and returns a *CapabilitySet (the set holds a map; a pointer avoids copying
// it and lets callers use the same value through one Resolve call site).
func Resolve(selected []CapabilityID, catalog *Catalog) (*CapabilitySet, error) {
	if catalog == nil {
		return nil, fmt.Errorf("resolver: catalog is nil")
	}

	// 1. Every selected ID must be known. (Unknown selected = hard error, same
	//    class as an unknown hard dep: the selection references a capability
	//    that does not exist in the merged catalog.)
	for _, id := range selected {
		if _, ok := catalog.Get(id); !ok {
			return nil, fmt.Errorf("resolver: selected capability %q is not in the catalog", id)
		}
	}

	// 2-4. DFS coloring over hard-dep edges, accumulating the closure.
	const (
		white = 0 // unvisited
		gray  = 1 // on the current recursion stack
		black = 2 // fully processed
	)
	color := make(map[CapabilityID]int)
	closure := make(map[CapabilityID]bool, len(selected))

	// stack tracks the current DFS path (the gray nodes) so a cycle error can
	// name the exact cycle (back-edge target ... current).
	var stack []CapabilityID

	var visit func(id CapabilityID) error
	visit = func(id CapabilityID) error {
		switch color[id] {
		case black:
			return nil // already fully processed; closure already holds it
		case gray:
			// Back-edge: id is on the current stack -> cycle. Reconstruct the
			// cycle slice from the first occurrence of id on the stack.
			for i, s := range stack {
				if s == id {
					cycle := append([]CapabilityID{}, stack[i:]...)
					cycle = append(cycle, id) // close the loop: target ... -> target
					return fmt.Errorf("resolver: hard-dep cycle detected: %s", joinCycle(cycle))
				}
			}
			// Unreachable (gray implies membership on stack) but fail-closed.
			return fmt.Errorf("resolver: hard-dep cycle detected involving %q", id)
		}

		m, ok := catalog.Get(id)
		if !ok {
			// Defense-in-depth: roots are checked above and deps are checked
			// before recursion, so visit is only called on known ids. This guard
			// makes Resolve robust on an unvalidated catalog regardless.
			return fmt.Errorf("resolver: capability %q is not in the catalog", id)
		}
		color[id] = gray
		stack = append(stack, id)
		closure[id] = true

		// Traverse hard deps in sorted order for deterministic cycle reporting
		// (the same cycle is reported with the same path regardless of map
		// iteration order). Each dep is checked for catalog membership BEFORE
		// recursion so the error names the parent and the missing dep.
		deps := append([]CapabilityID{}, m.HardDeps...)
		sort.Strings(deps)
		for _, dep := range deps {
			if _, ok := catalog.Get(dep); !ok {
				return fmt.Errorf("resolver: capability %q has unknown hard_dep %q", id, dep)
			}
			if err := visit(dep); err != nil {
				return err
			}
		}

		stack = stack[:len(stack)-1]
		color[id] = black
		return nil
	}

	// Visit each selected id. Iterate a sorted copy of the selection so that,
	// when multiple roots independently error (e.g. two unknown selections),
	// the reported error is deterministic.
	roots := append([]CapabilityID{}, selected...)
	sort.Strings(roots)
	for _, id := range roots {
		if err := visit(id); err != nil {
			return nil, err
		}
	}

	// 5. Project the agent roster: baseline ∪ Provides(closure). Iterate the
	//    closure in sorted order so Provides accumulation is deterministic.
	set := &CapabilitySet{
		caps:     closure,
		baseline: catalog.BaselineAgents(),
	}
	agentSet := make(map[string]bool, len(set.baseline))
	for _, a := range set.baseline {
		agentSet[a] = true
	}
	closedIDs := make([]CapabilityID, 0, len(closure))
	for id := range closure {
		closedIDs = append(closedIDs, id)
	}
	sort.Strings(closedIDs)
	for _, id := range closedIDs {
		m, _ := catalog.Get(id)
		for _, a := range m.Provides {
			agentSet[a] = true
		}
	}
	set.agents = make([]string, 0, len(agentSet))
	for a := range agentSet {
		set.agents = append(set.agents, a)
	}
	sort.Strings(set.agents)
	return set, nil
}

// joinCycle renders a cycle path (a -> b -> c -> a) for an error message.
func joinCycle(cycle []CapabilityID) string {
	return strings.Join(cycle, " -> ")
}
