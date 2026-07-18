package resolver

// Phase-3 capability-installer overlay-integration tests at the resolver layer.
// Cover:
//   - ResolveContributions (the exported project-over-embedded shadowing helper)
//     preserves pack names + sources and applies project-wins by pack name.
//   - the embedded `release` pack's manifest SHAPE (id core/release, hard_dep
//     core/gated-commit) parses, merges into the catalog, and selecting it
//     closes core/gated-commit via the hard-dep resolver closure.
//   - project-pack shadowing of a release pack: a project-local release pack
//     shadows the embedded one (project-wins), contributing its OWN manifest.
//
// internal/resolver cannot import internal/overlay (overlay imports resolver),
// so the manifest bodies are constructed inline mirroring the real embedded
// release/capability-manifest.yml. The end-to-end pack load is covered at the
// overlay layer (ReadCapabilityManifest on the real pack) and at the cli layer
// (seam install/update selecting release).

import (
	"sort"
	"testing"
)

// releaseManifest is the inline mirror of the embedded
// templates/overlays/release/capability-manifest.yml so the resolver-layer tests
// exercise the real shape without importing internal/overlay.
func releaseManifest() CapabilityManifest {
	return CapabilityManifest{
		ID:       "core/release",
		Provides: []string{"releaser"},
		HardDeps: []string{"core/gated-commit"},
	}
}

func TestResolveContributions_EmptyInput(t *testing.T) {
	got := ResolveContributions(nil)
	if len(got) != 0 {
		t.Fatalf("ResolveContributions(nil): want empty, got %v", got)
	}
}

func TestResolveContributions_PreservesPackAndSource(t *testing.T) {
	got := ResolveContributions([]PackContribution{
		{Pack: "alpha", Source: SourceEmbedded, Manifest: validPackManifest("alpha/a", "a-agent")},
		{Pack: "beta", Source: SourceProject, Manifest: validPackManifest("beta/b", "b-agent")},
	})
	if len(got) != 2 {
		t.Fatalf("want 2 survivors, got %d (%v)", len(got), got)
	}
	// Survivors are sorted by pack name (alpha, beta).
	if got[0].Pack != "alpha" || got[0].Source != SourceEmbedded {
		t.Errorf("survivor[0]: got %+v, want pack=alpha source=embedded", got[0])
	}
	if got[1].Pack != "beta" || got[1].Source != SourceProject {
		t.Errorf("survivor[1]: got %+v, want pack=beta source=project", got[1])
	}
}

func TestResolveContributions_ProjectShadowsEmbeddedByPackName(t *testing.T) {
	emb := validPackManifest("core/release", "releaser-embedded")
	emb.HardDeps = []string{"core/gated-commit"}
	proj := validPackManifest("core/release", "releaser-project") // SAME pack name, project wins
	proj.HardDeps = []string{"core/gated-commit"}
	got := ResolveContributions([]PackContribution{
		{Pack: "release", Source: SourceEmbedded, Manifest: emb},
		{Pack: "release", Source: SourceProject, Manifest: proj},
	})
	if len(got) != 1 {
		t.Fatalf("project-over-embedded shadowing: want 1 survivor, got %d (%v)", len(got), got)
	}
	if got[0].Source != SourceProject {
		t.Errorf("survivor source: got %v, want project", got[0].Source)
	}
	if got[0].Manifest.Provides[0] != "releaser-project" {
		t.Errorf("survivor manifest must be the PROJECT one (releaser-project), got %v", got[0].Manifest.Provides)
	}
}

func TestResolveContributions_SortedByPackName(t *testing.T) {
	got := ResolveContributions([]PackContribution{
		{Pack: "zeta", Source: SourceEmbedded, Manifest: validPackManifest("zeta/z", "z-agent")},
		{Pack: "alpha", Source: SourceEmbedded, Manifest: validPackManifest("alpha/a", "a-agent")},
		{Pack: "mid", Source: SourceProject, Manifest: validPackManifest("mid/m", "m-agent")},
	})
	names := make([]string, 0, len(got))
	for _, c := range got {
		names = append(names, c.Pack)
	}
	if !sort.StringsAreSorted(names) {
		t.Errorf("survivors must be sorted by pack name; got %v", names)
	}
}

// TestMergeCatalogs_ReleasePackShapeMirrorsEmbedded confirms the inline
// releaseManifest merges cleanly into the core catalog (the exact shape the
// embedded release/capability-manifest.yml carries).
func TestMergeCatalogs_ReleasePackShapeMirrorsEmbedded(t *testing.T) {
	merged, err := MergeCatalogs(CoreCatalog(), []PackContribution{
		{Pack: "release", Source: SourceEmbedded, Manifest: releaseManifest()},
	})
	if err != nil {
		t.Fatalf("MergeCatalogs with release: %v", err)
	}
	if _, ok := merged.Get("core/release"); !ok {
		t.Errorf("merged catalog missing core/release")
	}
	// Baseline (8) + core seed capabilities (gated-commit, debate,
	// media-perception) + release.
	if got := len(merged.IDs()); got != 4 {
		t.Errorf("merged catalog id count: got %d, want 4 (gated-commit, debate, media-perception, release)", got)
	}
}

// TestResolve_ReleasePullsGatedCommitClosure is the headline hard-dep test:
// selecting core/release (the embedded release pack's shape) must pull
// core/gated-commit into the resolved closure via the hard-dep edge. This is the
// invariant that makes the release pack's hard_dep declaration meaningful.
func TestResolve_ReleasePullsGatedCommitClosure(t *testing.T) {
	merged, err := MergeCatalogs(CoreCatalog(), []PackContribution{
		{Pack: "release", Source: SourceEmbedded, Manifest: releaseManifest()},
	})
	if err != nil {
		t.Fatalf("MergeCatalogs: %v", err)
	}
	set, err := Resolve([]CapabilityID{"core/release"}, merged)
	if err != nil {
		t.Fatalf("Resolve([core/release]): %v", err)
	}
	if !set.Has("core/release") {
		t.Errorf("closure must contain the selected core/release")
	}
	if !set.Has("core/gated-commit") {
		t.Errorf("closure must contain core/gated-commit (hard_dep of core/release); got %v", set.All())
	}
	// The releaser agent must be in the rendered roster (release provides it),
	// AND the gated-commit agents (committer et al.) must render too.
	agents := set.Agents()
	for _, want := range []string{"releaser", "committer", "commit-message", "commit-reviewer"} {
		found := false
		for _, a := range agents {
			if a == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("agent roster missing %q (release + gated-commit cluster must render); got %v", want, agents)
		}
	}
}
