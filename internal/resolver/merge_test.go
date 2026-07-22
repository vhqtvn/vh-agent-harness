package resolver

// Phase 2 pack-manifest merge tests (resolver.MergeCatalogs). Cover:
//   - two packs each contributing a capability -> combined catalog valid
//   - the merged catalog includes core seed capabilities + baseline alongside
//     pack contributions
//   - duplicate capability id across packs -> blocker (fail-closed)
//   - duplicate provides agent-name across packs -> blocker
//   - duplicate id between a pack and a core seed -> blocker (core is immutable)
//   - unknown hard_dep in a pack manifest -> blocker
//   - unknown optional_dep in a pack manifest -> tolerated
//   - project-pack same-name shadows embedded (project wins, embedded dropped)
//   - the merged catalog is consumable by Resolve (closure + baseline correct)
//   - nil core fails closed

import (
	"strings"
	"testing"
)

// validPackManifest is a small helper building a structurally-clean manifest so
// the tests focus on the MERGE rule under test, not per-manifest validation.
func validPackManifest(id, provides string, hardDeps ...string) CapabilityManifest {
	return CapabilityManifest{ID: id, Provides: []string{provides}, HardDeps: append([]string{}, hardDeps...)}
}

func TestMergeCatalogs_TwoPacksMergeClean(t *testing.T) {
	core := CoreCatalog()
	merged, err := MergeCatalogs(core, []PackContribution{
		{Pack: "acme", Source: SourceProject, Manifest: validPackManifest("acme/deploy", "deploy-agent")},
		{Pack: "globex", Source: SourceProject, Manifest: validPackManifest("globex/audit", "audit-agent")},
	})
	if err != nil {
		t.Fatalf("MergeCatalogs: unexpected error: %v", err)
	}
	// Core seed capabilities survive.
	for _, id := range []string{"core/gated-commit", "core/debate"} {
		if _, ok := merged.Get(id); !ok {
			t.Errorf("merged catalog lost core capability %q", id)
		}
	}
	// Pack-contributed capabilities are present.
	for _, id := range []string{"acme/deploy", "globex/audit"} {
		if _, ok := merged.Get(id); !ok {
			t.Errorf("merged catalog missing pack capability %q", id)
		}
	}
	// Baseline is carried through from core.
	if got := len(merged.BaselineAgents()); got != 8 {
		t.Errorf("baseline size: got %d, want 8 (carried from core)", got)
	}
	// Validate clean (MergeCatalogs already validated, but assert directly).
	if errs := merged.Validate(); len(errs) != 0 {
		t.Errorf("merged catalog should validate clean, got: %+v", errs)
	}
}

func TestMergeCatalogs_DuplicateIDAcrossPacksBlocked(t *testing.T) {
	core := CoreCatalog()
	_, err := MergeCatalogs(core, []PackContribution{
		{Pack: "acme", Source: SourceProject, Manifest: validPackManifest("shared/cap", "agent-a")},
		{Pack: "globex", Source: SourceProject, Manifest: validPackManifest("shared/cap", "agent-b")},
	})
	if err == nil {
		t.Fatalf("MergeCatalogs must block duplicate capability id across packs (fail-closed, not last-writer-wins)")
	}
	if !strings.Contains(err.Error(), `duplicate capability id "shared/cap"`) {
		t.Errorf("duplicate-id error should name the id; got: %v", err)
	}
}

func TestMergeCatalogs_DuplicateProvidesAcrossPacksBlocked(t *testing.T) {
	core := CoreCatalog()
	// Two DIFFERENT capability ids but both Provide the SAME agent -> an agent
	// belongs to at most one capability -> blocker.
	_, err := MergeCatalogs(core, []PackContribution{
		{Pack: "acme", Source: SourceProject, Manifest: validPackManifest("acme/one", "contended-agent")},
		{Pack: "globex", Source: SourceProject, Manifest: validPackManifest("globex/two", "contended-agent")},
	})
	if err == nil {
		t.Fatalf("MergeCatalogs must block duplicate provides agent-name across capabilities")
	}
	if !strings.Contains(err.Error(), `"contended-agent" provided by both`) {
		t.Errorf("duplicate-provides error should name the agent; got: %v", err)
	}
}

func TestMergeCatalogs_DuplicateIDWithCoreSeedBlocked(t *testing.T) {
	// A pack that re-declares a core seed id (core/gated-commit) is NOT a
	// shadow — core capabilities are immutable. It is a duplicate-id blocker.
	core := CoreCatalog()
	_, err := MergeCatalogs(core, []PackContribution{
		{Pack: "rogue", Source: SourceProject, Manifest: validPackManifest("core/gated-commit", "usurper-agent")},
	})
	if err == nil {
		t.Fatalf("MergeCatalogs must block a pack re-declaring a core seed id")
	}
	if !strings.Contains(err.Error(), `duplicate capability id "core/gated-commit"`) {
		t.Errorf("core-collision error should name the core id; got: %v", err)
	}
}

func TestMergeCatalogs_UnknownHardDepInPackBlocked(t *testing.T) {
	core := CoreCatalog()
	_, err := MergeCatalogs(core, []PackContribution{
		{Pack: "acme", Source: SourceProject, Manifest: validPackManifest("acme/deploy", "deploy-agent", "acme/missing-dep")},
	})
	if err == nil {
		t.Fatalf("MergeCatalogs must block an unknown hard_dep in a pack manifest")
	}
	if !strings.Contains(err.Error(), `unknown hard_dep "acme/missing-dep"`) {
		t.Errorf("unknown-hard-dep error should name the dep; got: %v", err)
	}
}

func TestMergeCatalogs_PackHardDepOnCoreResolves(t *testing.T) {
	// A pack capability may hard-depend on a core seed capability (e.g. a
	// release capability depending on core/gated-commit). The merged catalog
	// must resolve it (core seeds are present in the merged set).
	core := CoreCatalog()
	merged, err := MergeCatalogs(core, []PackContribution{
		{Pack: "acme", Source: SourceProject, Manifest: validPackManifest("acme/release", "release-agent", "core/gated-commit")},
	})
	if err != nil {
		t.Fatalf("pack hard-dep on a core seed must resolve in the merged catalog; got: %v", err)
	}
	if _, ok := merged.Get("acme/release"); !ok {
		t.Errorf("acme/release missing from merged catalog")
	}
}

func TestMergeCatalogs_UnknownOptionalDepInPackTolerated(t *testing.T) {
	core := CoreCatalog()
	merged, err := MergeCatalogs(core, []PackContribution{
		{Pack: "acme", Source: SourceProject, Manifest: CapabilityManifest{
			ID:           "acme/deploy",
			Provides:     []string{"deploy-agent"},
			OptionalDeps: []string{"acme/future-overlay-cap"}, // not present anywhere
		}},
	})
	if err != nil {
		t.Fatalf("unknown optional_dep in a pack manifest must be tolerated, got: %v", err)
	}
	if _, ok := merged.Get("acme/deploy"); !ok {
		t.Errorf("acme/deploy must be present despite its unknown optional dep")
	}
}

func TestMergeCatalogs_ProjectShadowsEmbedded(t *testing.T) {
	core := CoreCatalog()
	// Embedded pack "acme" provides agent-from-embedded; the project pack of
	// the SAME name provides agent-from-project. Project wins: the merged
	// catalog must carry the PROJECT capability and NOT the embedded one.
	embedded := CapabilityManifest{ID: "acme/deploy", Provides: []string{"agent-from-embedded"}}
	project := CapabilityManifest{ID: "acme/deploy", Provides: []string{"agent-from-project"}}
	merged, err := MergeCatalogs(core, []PackContribution{
		{Pack: "acme", Source: SourceEmbedded, Manifest: embedded},
		{Pack: "acme", Source: SourceProject, Manifest: project},
	})
	if err != nil {
		t.Fatalf("project shadowing merge: unexpected error: %v", err)
	}
	m, ok := merged.Get("acme/deploy")
	if !ok {
		t.Fatalf("shadowed pack capability acme/deploy must be present (from the project manifest)")
	}
	// The project manifest's Provides won; the embedded provides is NOT a
	// union (no inheritance) — the shadowing pack declares its own manifest.
	if len(m.Provides) != 1 || m.Provides[0] != "agent-from-project" {
		t.Errorf("project manifest should win; got Provides=%v, want [agent-from-project]", m.Provides)
	}
	if _, owned := merged.CapabilityForAgent("agent-from-embedded"); owned {
		t.Errorf("embedded manifest must NOT contribute after being shadowed; agent-from-embedded is still owned")
	}
	// Validate clean: the shadowed (dropped) embedded manifest does not leave a
	// duplicate-id residue behind.
	if errs := merged.Validate(); len(errs) != 0 {
		t.Errorf("shadowed merge should validate clean, got: %+v", errs)
	}
}

func TestMergeCatalogs_ShadowingOrderIndependent(t *testing.T) {
	// Shadowing must hold regardless of the order contributions are passed in
	// (project listed BEFORE embedded). Project wins either way.
	core := CoreCatalog()
	embedded := CapabilityManifest{ID: "acme/deploy", Provides: []string{"embedded-agent"}}
	project := CapabilityManifest{ID: "acme/deploy", Provides: []string{"project-agent"}}
	for _, order := range [][]PackContribution{
		{{Pack: "acme", Source: SourceEmbedded, Manifest: embedded},
			{Pack: "acme", Source: SourceProject, Manifest: project}},
		{{Pack: "acme", Source: SourceProject, Manifest: project},
			{Pack: "acme", Source: SourceEmbedded, Manifest: embedded}},
	} {
		merged, err := MergeCatalogs(core, order)
		if err != nil {
			t.Fatalf("order-independent shadowing: unexpected error: %v", err)
		}
		if _, owned := merged.CapabilityForAgent("embedded-agent"); owned {
			t.Errorf("embedded-agent must not survive shadowing regardless of input order")
		}
		if _, owned := merged.CapabilityForAgent("project-agent"); !owned {
			t.Errorf("project-agent must win regardless of input order")
		}
	}
}

func TestMergeCatalogs_ShadowingDoesNotInheritDeps(t *testing.T) {
	// The shadowing pack declares its OWN manifest; it does NOT inherit the
	// shadowed pack's deps. Here the embedded pack hard-depends on a pack cap
	// that the project manifest does NOT declare; after shadowing the merged
	// catalog must validate clean (the embedded hard dep is gone with the
	// embedded manifest).
	core := CoreCatalog()
	// Embedded declares a hard dep on core/gated-commit.
	embedded := CapabilityManifest{
		ID: "acme/deploy", Provides: []string{"embedded-agent"}, HardDeps: []string{"core/gated-commit"},
	}
	// Project replaces it with a self-contained manifest (no hard deps).
	project := CapabilityManifest{ID: "acme/deploy", Provides: []string{"project-agent"}}
	merged, err := MergeCatalogs(core, []PackContribution{
		{Pack: "acme", Source: SourceEmbedded, Manifest: embedded},
		{Pack: "acme", Source: SourceProject, Manifest: project},
	})
	if err != nil {
		t.Fatalf("shadowing must not inherit the embedded deps; got: %v", err)
	}
	m, _ := merged.Get("acme/deploy")
	if len(m.HardDeps) != 0 {
		t.Errorf("shadowed project manifest should carry no inherited hard deps; got %v", m.HardDeps)
	}
}

func TestMergeCatalogs_NilCoreErrors(t *testing.T) {
	if _, err := MergeCatalogs(nil, []PackContribution{
		{Pack: "acme", Source: SourceProject, Manifest: validPackManifest("acme/deploy", "deploy-agent")},
	}); err == nil {
		t.Fatalf("MergeCatalogs must fail closed on a nil core catalog")
	}
}

func TestMergeCatalogs_MergedCatalogResolves(t *testing.T) {
	// End-to-end: MergeCatalogs output is consumable by Resolve. A pack cap
	// hard-depending on a core seed resolves to the full closure + baseline.
	core := CoreCatalog()
	merged, err := MergeCatalogs(core, []PackContribution{
		{Pack: "acme", Source: SourceProject,
			Manifest: validPackManifest("acme/release", "release-agent", "core/gated-commit")},
	})
	if err != nil {
		t.Fatalf("MergeCatalogs: unexpected error: %v", err)
	}
	set, err := Resolve([]string{"acme/release"}, merged)
	if err != nil {
		t.Fatalf("Resolve on merged catalog: unexpected error: %v", err)
	}
	// Closure: acme/release + its hard dep core/gated-commit.
	if !set.Has("acme/release") || !set.Has("core/gated-commit") {
		t.Errorf("merged-catalog closure missing capabilities; got %v", set.All())
	}
	// core/debate is NOT in the closure (not selected, not a hard dep).
	if set.Has("core/debate") {
		t.Errorf("core/debate should not be pulled in; got %v", set.All())
	}
	// Agents(): baseline (8) + release-agent + gated-commit's Provides.
	agents := set.Agents()
	for _, a := range []string{"build", "release-agent", "committer", "commit-reviewer"} {
		if !containsString(agents, a) {
			t.Errorf("Agents() missing %q; got %v", a, agents)
		}
	}
}

func TestContributionSource_String(t *testing.T) {
	// Smoke-test the source labels used in debug/error context.
	for s, want := range map[ContributionSource]string{
		SourceEmbedded: "embedded",
		SourceProject:  "project",
	} {
		if s.String() != want {
			t.Errorf("ContributionSource(%d).String() = %q, want %q", s, s.String(), want)
		}
	}
}

func TestMergeCatalogs_CoreOutputsNoCollision(t *testing.T) {
	// Two capabilities each declaring DISTINCT core outputs merge clean.
	core := CoreCatalog()
	merged, err := MergeCatalogs(core, []PackContribution{
		{Pack: "acme", Source: SourceProject, Manifest: CapabilityManifest{
			ID:          "acme/thing",
			Provides:    []string{"thing-agent"},
			CoreOutputs: []string{".opencode/agents/thing-agent.md"},
		}},
	})
	if err != nil {
		t.Fatalf("distinct CoreOutputs should merge clean, got: %v", err)
	}
	m, _ := merged.Get("acme/thing")
	if len(m.CoreOutputs) != 1 {
		t.Errorf("acme/thing CoreOutputs should survive merge; got %v", m.CoreOutputs)
	}
}

func TestMergeCatalogs_CoreOutputsCollisionAcrossPacksBlocked(t *testing.T) {
	// Two DIFFERENT capability ids both declaring the SAME core output path is
	// an ambiguous-ownership blocker (a core output is owned by at most one
	// capability).
	core := CoreCatalog()
	_, err := MergeCatalogs(core, []PackContribution{
		{Pack: "acme", Source: SourceProject, Manifest: CapabilityManifest{
			ID:          "acme/one",
			Provides:    []string{"agent-a"},
			CoreOutputs: []string{".opencode/agents/shared.md"},
		}},
		{Pack: "globex", Source: SourceProject, Manifest: CapabilityManifest{
			ID:          "globex/two",
			Provides:    []string{"agent-b"},
			CoreOutputs: []string{".opencode/agents/shared.md"},
		}},
	})
	if err == nil {
		t.Fatalf("MergeCatalogs must block duplicate core output across capabilities")
	}
	if !strings.Contains(err.Error(), `core output ".opencode/agents/shared.md" declared by both`) {
		t.Errorf("collision error should name the path and both owners; got: %v", err)
	}
}

func TestMergeCatalogs_CoreOutputsCollisionWithCoreSeedBlocked(t *testing.T) {
	// A pack re-declaring a core output that a core seed (core/media-perception)
	// already owns is a collision blocker — core outputs are not shadowable.
	core := CoreCatalog()
	_, err := MergeCatalogs(core, []PackContribution{
		{Pack: "rogue", Source: SourceProject, Manifest: CapabilityManifest{
			ID:          "rogue/cap",
			Provides:    []string{"rogue-agent"},
			CoreOutputs: []string{".opencode/agents/media-perception.md"},
		}},
	})
	if err == nil {
		t.Fatalf("MergeCatalogs must block a pack colliding with a core seed's CoreOutput")
	}
	if !strings.Contains(err.Error(), `.opencode/agents/media-perception.md" declared by both`) {
		t.Errorf("collision error should name the contended path; got: %v", err)
	}
}
