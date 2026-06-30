package overlay

// Phase-3 capability-installer overlay-integration tests at the overlay layer.
// These exercise the REAL embedded `release` pack (the first shipped embedded
// overlay pack) via OpenPack, NOT a fstest fixture. This is the layer that proves
// the shipped pack carries the manifest shape the resolver-layer tests mirror
// inline (internal/resolver/release_overlay_test.go) and the cli render-level
// tests rely on (internal/cli/release_render_test.go).
//
// Covered:
//   - OpenPack("release") succeeds (the pack is discoverable + openable).
//   - ReadCapabilityManifest on the real pack returns id=core/release,
//     provides=[releaser], hard_deps=[core/gated-commit], optional_deps=[].
//     This is the exact shape internal/resolver.TestResolve_ReleasePulls*
//     asserts drives the gated-commit closure.
//   - RenderUnits renders the releaser agent unit and EXCLUDES the four merge/
//     catalog files (capability-manifest.yml, opencode-append.jsonc,
//     permission-pack.jsonc, callable-graph-snippet.md) — they are merge-content,
//     not rendered units.
//   - UnitPaths (the shadow-guard collision input) excludes the merge files too.

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// TestOpenPack_ReleasePackShips confirms the release pack is shipped and
// openable from the embedded tree.
func TestOpenPack_ReleasePackShips(t *testing.T) {
	pack, err := OpenPack("release")
	if err != nil {
		t.Fatalf("OpenPack(release): %v", err)
	}
	if pack == nil {
		t.Fatal("OpenPack(release): nil pack")
	}
	if pack.Name != "release" {
		t.Errorf("pack.Name: got %q, want release", pack.Name)
	}
}

// TestReadCapabilityManifest_RealReleasePack pins the shipped release pack's
// manifest shape. Any drift (id rename, dep change) breaks this test before it
// silently changes resolver closure behavior in the field.
func TestReadCapabilityManifest_RealReleasePack(t *testing.T) {
	pack, err := OpenPack("release")
	if err != nil {
		t.Fatalf("OpenPack(release): %v", err)
	}
	m, ok, err := pack.ReadCapabilityManifest()
	if err != nil {
		t.Fatalf("ReadCapabilityManifest: %v", err)
	}
	if !ok {
		t.Fatal("release pack must carry a capability-manifest.yml")
	}
	if m.ID != "core/release" {
		t.Errorf("manifest ID: got %q, want core/release", m.ID)
	}
	if len(m.Provides) != 1 || m.Provides[0] != "releaser" {
		t.Errorf("Provides: got %v, want [releaser]", m.Provides)
	}
	if len(m.HardDeps) != 1 || m.HardDeps[0] != "core/gated-commit" {
		t.Errorf("HardDeps: got %v, want [core/gated-commit]", m.HardDeps)
	}
	if len(m.OptionalDeps) != 0 {
		t.Errorf("OptionalDeps: got %v, want []", m.OptionalDeps)
	}
}

// TestRenderUnits_RealReleasePack confirms RenderUnits renders the releaser
// agent unit and excludes the four merge-content files (manifest, append,
// permission-pack, callable-graph-snippet).
func TestRenderUnits_RealReleasePack(t *testing.T) {
	pack, err := OpenPack("release")
	if err != nil {
		t.Fatalf("OpenPack(release): %v", err)
	}
	staging := t.TempDir()
	rendered, err := pack.RenderUnits(staging, map[string]string{
		"project_name":    "vh-agent-harness",
		"project_slug":    "vh-agent-harness",
		"coordinator_dir": ".local/coordinator",
	})
	if err != nil {
		t.Fatalf("RenderUnits: %v", err)
	}
	// The releaser agent unit MUST render.
	wantUnit := ".opencode/agents/releaser.md"
	if !contains(rendered, wantUnit) {
		t.Errorf("RenderUnits must render %q; got %v", wantUnit, rendered)
	}
	if _, err := os.Stat(filepath.Join(staging, filepath.FromSlash(wantUnit))); err != nil {
		t.Errorf("releaser.md must land on disk under staging: %v", err)
	}
	// Merge-content / catalog files MUST NOT render as units.
	for _, bad := range []string{
		".opencode/" + capabilityManifestFileName,
		".opencode/" + appendFileName,
		".opencode/" + permissionPackFileName,
		".opencode/" + snippetFileName,
	} {
		if contains(rendered, bad) {
			t.Errorf("RenderUnits must NOT render merge-content %q; got %v", bad, rendered)
		}
		if _, err := os.Stat(filepath.Join(staging, filepath.FromSlash(bad))); err == nil {
			t.Errorf("merge-content %q must not land on disk as a rendered unit", bad)
		}
	}
}

// TestUnitPaths_RealReleasePack confirms UnitPaths excludes the merge-content
// files (the shadow-guard collision input must not include them).
func TestUnitPaths_RealReleasePack(t *testing.T) {
	pack, err := OpenPack("release")
	if err != nil {
		t.Fatalf("OpenPack(release): %v", err)
	}
	paths, err := pack.UnitPaths()
	if err != nil {
		t.Fatalf("UnitPaths: %v", err)
	}
	// Sorted for a stable assertion; the release pack ships exactly one unit
	// (agents/releaser.md).
	sort.Strings(paths)
	for _, bad := range []string{
		opencodePrefix + capabilityManifestFileName,
		opencodePrefix + appendFileName,
		opencodePrefix + permissionPackFileName,
		opencodePrefix + snippetFileName,
	} {
		if contains(paths, bad) {
			t.Errorf("UnitPaths must not list merge-content %q; got %v", bad, paths)
		}
	}
	want := []string{".opencode/agents/releaser.md"}
	if len(paths) != len(want) || paths[0] != want[0] {
		t.Errorf("UnitPaths: got %v, want %v", paths, want)
	}
}
