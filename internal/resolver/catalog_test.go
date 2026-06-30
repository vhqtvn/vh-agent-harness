package resolver

import (
	"testing"
)

func TestCoreCatalog_ValidatesClean(t *testing.T) {
	c := CoreCatalog()
	if errs := c.Validate(); len(errs) != 0 {
		t.Fatalf("core catalog seed should validate clean, got: %+v", errs)
	}
}

func TestCoreCatalog_SeedCapabilities(t *testing.T) {
	c := CoreCatalog()
	got := c.IDs()
	want := []string{"core/debate", "core/gated-commit"} // sorted
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("IDs: expected %v, got %v", want, got)
	}
	for _, id := range want {
		if _, ok := c.Get(id); !ok {
			t.Fatalf("Get(%q): expected ok", id)
		}
	}
	if _, ok := c.Get("core/nonexistent"); ok {
		t.Fatalf("Get(unknown): expected not ok")
	}
}

func TestCoreCatalog_GatedCommitProvides(t *testing.T) {
	c := CoreCatalog()
	m, ok := c.Get("core/gated-commit")
	if !ok {
		t.Fatalf("missing core/gated-commit")
	}
	// Every agent the audit assigns to gated-commit is owned by it.
	for _, agent := range []string{
		"commit-message", "commit-reviewer",
		"commit-reviewer-a", "commit-reviewer-b", "commit-reviewer-c", "commit-reviewer-d",
		"committer",
	} {
		owner, owned := c.CapabilityForAgent(agent)
		if !owned {
			t.Fatalf("agent %q should be owned by core/gated-commit, not owned at all", agent)
		}
		if owner != "core/gated-commit" {
			t.Fatalf("agent %q owner: expected core/gated-commit, got %q", agent, owner)
		}
	}
	// Gated-commit is self-contained: no capability-level hard deps.
	if len(m.HardDeps) != 0 {
		t.Fatalf("core/gated-commit should have no capability-level hard_deps, got %v", m.HardDeps)
	}
}

func TestCoreCatalog_DebateProvides(t *testing.T) {
	c := CoreCatalog()
	for _, agent := range []string{"debate", "debate-proposer", "debate-critic", "debate-synth", "solution-brief"} {
		owner, owned := c.CapabilityForAgent(agent)
		if !owned || owner != "core/debate" {
			t.Fatalf("agent %q should be owned by core/debate (got owner=%q owned=%v)", agent, owner, owned)
		}
	}
	m, ok := c.Get("core/debate")
	if !ok {
		t.Fatalf("missing core/debate")
	}
	if len(m.HardDeps) != 0 {
		t.Fatalf("core/debate should have no capability-level hard_deps, got %v", m.HardDeps)
	}
}

func TestCoreCatalog_BaselineEightAgents(t *testing.T) {
	c := CoreCatalog()
	baseline := c.BaselineAgents()
	if len(baseline) != 8 {
		t.Fatalf("baseline: expected 8 universal agents, got %d (%v)", len(baseline), baseline)
	}
	for _, agent := range []string{
		"coordination", "build", "project-coordinator", "researcher",
		"repo-explorer", "planner", "docs-steward", "ship-review",
	} {
		if !c.HasAgent(agent) {
			t.Fatalf("baseline agent %q should be known via HasAgent", agent)
		}
		// Baseline agents are NOT owned by any capability.
		if _, owned := c.CapabilityForAgent(agent); owned {
			t.Fatalf("baseline agent %q must not be a capability-provided agent", agent)
		}
	}
	// BaselineAgents returns a copy: mutating it must not affect the catalog.
	baseline[0] = "MUTATED"
	again := c.BaselineAgents()
	if again[0] == "MUTATED" {
		t.Fatalf("BaselineAgents must return a defensive copy")
	}
}

func TestCoreCatalog_ProvidedAndBaselineDisjoint(t *testing.T) {
	c := CoreCatalog()
	for _, agent := range c.BaselineAgents() {
		if owner, ok := c.CapabilityForAgent(agent); ok {
			t.Fatalf("baseline agent %q is also provided by %q (seed violates disjointness)", agent, owner)
		}
	}
}

func TestCatalog_DuplicateIDRejected(t *testing.T) {
	c := newCatalog(nil,
		CapabilityManifest{ID: "core/x", Provides: []string{"a"}},
		CapabilityManifest{ID: "core/x", Provides: []string{"b"}},
	)
	errs := c.Validate()
	if !errContains(t, errs, `duplicate capability id "core/x"`) {
		t.Fatalf("expected duplicate-id error, got: %+v", errs)
	}
}

func TestCatalog_DuplicateProvidesAcrossCapabilitiesRejected(t *testing.T) {
	c := newCatalog(nil,
		CapabilityManifest{ID: "core/x", Provides: []string{"shared-agent"}},
		CapabilityManifest{ID: "core/y", Provides: []string{"shared-agent"}},
	)
	errs := c.Validate()
	if !errContains(t, errs, `agent "shared-agent" provided by both`) {
		t.Fatalf("expected duplicate-provides error, got: %+v", errs)
	}
}

func TestCatalog_UnknownHardDepRejected(t *testing.T) {
	c := newCatalog(nil,
		CapabilityManifest{
			ID:       "core/x",
			Provides: []string{"a"},
			// References a capability that does not exist in this catalog.
			HardDeps: []string{"core/ghost"},
		},
	)
	errs := c.Validate()
	if !errContains(t, errs, `unknown hard_dep "core/ghost"`) {
		t.Fatalf("expected unknown-hard-dep error, got: %+v", errs)
	}
}

func TestCatalog_KnownHardDepAccepted(t *testing.T) {
	c := newCatalog(nil,
		CapabilityManifest{ID: "core/base", Provides: []string{"base-agent"}},
		CapabilityManifest{
			ID:       "core/dependent",
			Provides: []string{"dep-agent"},
			HardDeps: []string{"core/base"},
		},
	)
	if errs := c.Validate(); len(errs) != 0 {
		t.Fatalf("known hard dep should validate clean, got: %+v", errs)
	}
}

func TestCatalog_UnknownOptionalDepTolerated(t *testing.T) {
	c := newCatalog(nil,
		CapabilityManifest{
			ID:       "core/x",
			Provides: []string{"a"},
			// Optional dep referencing a capability not in the catalog must NOT
			// be a validation error (forward/overlay reference).
			OptionalDeps: []string{"core/overlay-only-cap"},
		},
	)
	if errs := c.Validate(); len(errs) != 0 {
		t.Fatalf("unknown optional dep must be tolerated, got: %+v", errs)
	}
}

func TestCatalog_BaselineProvidesCollisionRejected(t *testing.T) {
	c := newCatalog(
		[]string{"colliding-agent"},
		CapabilityManifest{ID: "core/x", Provides: []string{"colliding-agent"}},
	)
	errs := c.Validate()
	if !errContains(t, errs, `"colliding-agent" is also provided by capability`) {
		t.Fatalf("expected baseline/provides collision error, got: %+v", errs)
	}
}

func TestCatalog_BaselineDedup(t *testing.T) {
	c := newCatalog([]string{"coordination", "coordination"})
	if errs := c.Validate(); !errContains(t, errs, `duplicate agent "coordination"`) {
		t.Fatalf("expected baseline duplicate error, got: %+v", errs)
	}
}

func TestCatalog_LookupAccessors(t *testing.T) {
	c := CoreCatalog()
	// by id
	if m, ok := c.Get("core/gated-commit"); !ok || m.ID != "core/gated-commit" {
		t.Fatalf("Get(core/gated-commit) unexpected: %+v ok=%v", m, ok)
	}
	// by provided agent name
	if id, ok := c.CapabilityForAgent("committer"); !ok || id != "core/gated-commit" {
		t.Fatalf("CapabilityForAgent(committer): expected core/gated-commit, got %q ok=%v", id, ok)
	}
	// baseline agent is known but not "provided"
	if c.HasAgent("build") != true {
		t.Fatalf("HasAgent(build): expected true")
	}
	if _, ok := c.CapabilityForAgent("build"); ok {
		t.Fatalf("CapabilityForAgent(build): baseline agent must not be 'provided'")
	}
	// truly unknown agent
	if c.HasAgent("no-such-agent") {
		t.Fatalf("HasAgent(unknown): expected false")
	}
}
