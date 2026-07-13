package overlay

// Layer-integrity smoke test for the embedded `auto-classifier-pilot` overlay
// pack. These exercise the REAL shipped embedded pack via OpenPack (NOT a
// fstest fixture, and NOT OpenPackFor — which would resolve a project-local
// copy first and never touch the embed). This is the layer that proves the
// shipped embedded pack round-trips through OpenPack -> RenderUnits ->
// UnitPaths -> ReadCapabilityManifest end to end.
//
// The e2e test (tests/e2e/auto-gate-classifier/Dockerfile.e2e-runner) COPIES
// templates/ into a project-local path, so OpenPackFor resolves the project
// copy and the embedded FS is never exercised there. A regression breaking
// the embed (a dropped go:embed root, a renamed pack directory, a misplaced
// merge-content file becoming a renderable unit) would pass every current
// test and only surface as a broken consumer render. These tests close that
// gap by driving the embedded pack directly.
//
// The pack is overlay-only: it ships 5 plugin units under plugins/ and an
// opencode-append.jsonc merge-content file, plus a README.md pack doc. It
// carries NO capability-manifest.yml, so ReadCapabilityManifest must report
// ok=false (a pack may legitimately contribute units without declaring a
// capability).
//
// Covered:
//   - OpenPack("auto-classifier-pilot") succeeds (the pack is discoverable +
//     openable from the embedded tree).
//   - RenderUnits renders exactly the 5 plugin units under .opencode/plugins/
//     and EXCLUDES opencode-append.jsonc (merge-content) and README.md (pack
//     doc) — neither is a renderable unit.
//   - UnitPaths returns exactly the 5 plugin paths (the shadow-guard collision
//     input must list only units, never merge-content or pack docs).
//   - ReadCapabilityManifest returns ok=false (overlay-only pack, no manifest).

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// autoClassifierPilotPluginUnits are the 5 renderable plugin units the pack
// ships, as live .opencode-relative paths (sorted). Both RenderUnits and
// UnitPaths must surface exactly this set.
var autoClassifierPilotPluginUnits = []string{
	".opencode/plugins/auto-gate-live.js",
	".opencode/plugins/auto-gate-scrub.js",
	".opencode/plugins/auto-gate-tiered.js",
	".opencode/plugins/auto-gate-verdict.js",
	".opencode/plugins/auto-tool-gate.js",
}

// TestOpenPack_AutoClassifierPilotShips confirms the auto-classifier-pilot
// pack is shipped and openable from the embedded tree.
func TestOpenPack_AutoClassifierPilotShips(t *testing.T) {
	pack, err := OpenPack("auto-classifier-pilot")
	if err != nil {
		t.Fatalf("OpenPack(auto-classifier-pilot): %v", err)
	}
	if pack == nil {
		t.Fatal("OpenPack(auto-classifier-pilot): nil pack")
	}
	if pack.Name != "auto-classifier-pilot" {
		t.Errorf("pack.Name: got %q, want auto-classifier-pilot", pack.Name)
	}
}

// TestRenderUnits_RealAutoClassifierPilotPack confirms RenderUnits renders
// exactly the 5 plugin units under .opencode/plugins/ and excludes the
// merge-content file (opencode-append.jsonc) and the pack doc (README.md).
func TestRenderUnits_RealAutoClassifierPilotPack(t *testing.T) {
	pack, err := OpenPack("auto-classifier-pilot")
	if err != nil {
		t.Fatalf("OpenPack(auto-classifier-pilot): %v", err)
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
	// Every plugin unit MUST render and land on disk under staging.
	for _, wantUnit := range autoClassifierPilotPluginUnits {
		if !contains(rendered, wantUnit) {
			t.Errorf("RenderUnits must render %q; got %v", wantUnit, rendered)
		}
		if _, err := os.Stat(filepath.Join(staging, filepath.FromSlash(wantUnit))); err != nil {
			t.Errorf("plugin unit %q must land on disk under staging: %v", wantUnit, err)
		}
	}
	// Exactly the 5 plugin units — no accidental extras (a misplaced
	// merge-content or doc file becoming a unit would inflate the count).
	if len(rendered) != len(autoClassifierPilotPluginUnits) {
		t.Errorf("RenderUnits count: got %d, want %d; got %v", len(rendered), len(autoClassifierPilotPluginUnits), rendered)
	}
	// Merge-content (opencode-append.jsonc) and pack doc (README.md) MUST NOT
	// render as units.
	for _, bad := range []string{
		".opencode/" + appendFileName,
		".opencode/README.md",
	} {
		if contains(rendered, bad) {
			t.Errorf("RenderUnits must NOT render %q; got %v", bad, rendered)
		}
		if _, err := os.Stat(filepath.Join(staging, filepath.FromSlash(bad))); err == nil {
			t.Errorf("%q must not land on disk as a rendered unit", bad)
		}
	}
}

// TestUnitPaths_RealAutoClassifierPilotPack confirms UnitPaths returns exactly
// the 5 plugin paths (the shadow-guard collision input must list only units,
// never merge-content or pack docs).
func TestUnitPaths_RealAutoClassifierPilotPack(t *testing.T) {
	pack, err := OpenPack("auto-classifier-pilot")
	if err != nil {
		t.Fatalf("OpenPack(auto-classifier-pilot): %v", err)
	}
	paths, err := pack.UnitPaths()
	if err != nil {
		t.Fatalf("UnitPaths: %v", err)
	}
	sort.Strings(paths)
	// Exactly the 5 plugin units.
	want := append([]string{}, autoClassifierPilotPluginUnits...)
	sort.Strings(want)
	if len(paths) != len(want) {
		t.Errorf("UnitPaths count: got %d, want %d; got %v", len(paths), len(want), paths)
	}
	for _, w := range want {
		if !contains(paths, w) {
			t.Errorf("UnitPaths must list %q; got %v", w, paths)
		}
	}
	// Merge-content and pack doc MUST NOT appear in the unit path list.
	for _, bad := range []string{
		opencodePrefix + appendFileName,
		".opencode/README.md",
	} {
		if contains(paths, bad) {
			t.Errorf("UnitPaths must not list %q; got %v", bad, paths)
		}
	}
}

// TestReadCapabilityManifest_RealAutoClassifierPilotPack pins the overlay-only
// structural property: the pack ships NO capability-manifest.yml, so
// ReadCapabilityManifest must report ok=false with no error (a pack may
// legitimately contribute units/permissions without declaring a capability).
func TestReadCapabilityManifest_RealAutoClassifierPilotPack(t *testing.T) {
	pack, err := OpenPack("auto-classifier-pilot")
	if err != nil {
		t.Fatalf("OpenPack(auto-classifier-pilot): %v", err)
	}
	m, ok, err := pack.ReadCapabilityManifest()
	if err != nil {
		t.Fatalf("ReadCapabilityManifest: %v", err)
	}
	if ok {
		t.Errorf("auto-classifier-pilot is overlay-only (no manifest); got ok=true, manifest=%+v", m)
	}
}
