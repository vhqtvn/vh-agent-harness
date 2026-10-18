package overlay

// Layer-integrity smoke test for the embedded `session-progress-pilot`
// overlay pack (Phase 1 of the session-progress-v2 detector). Like
// auto_classifier_pilot_pack_test.go, these exercise the REAL shipped
// embedded pack via OpenPack (NOT a fstest fixture, and NOT OpenPackFor —
// which would resolve a project-local copy first and never touch the embed).
//
// The pack is overlay-only and default-out:
//   - plugins/session-progress.js       — the single-hook plugin unit
//     (tool.execute.before ONLY; throw-to-deny / return-to-allow).
//   - scripts/session-progress-config.js  — operator config loader module.
//   - scripts/session-progress-judge.js   — bounded LLM judge module.
//   - scripts/session-progress-policy.js  — pure policy/lease/exemption
//     module.
//   - opencode-append.jsonc (merge-content, intentionally an empty object —
//     plugin auto-discovery needs no registration) and README.md (pack doc).
//   - NO capability-manifest.yml (declares no core capability) and NO
//     permission-pack.jsonc (adds no permission rules).
//
// The unit list is pinned so a misplaced merge-content/doc file becoming a
// renderable unit, a dropped plugin file, or a renamed script fails here
// rather than surfacing as a broken consumer render.
//
// Rendering shape: RenderUnits mirrors pack-relative paths under .opencode/,
// so plugins/session-progress.js renders to .opencode/plugins/ (auto-discovered
// by OpenCode) and scripts/*.js render to .opencode/scripts/ (imported by the
// plugin via ../scripts/<name>.js — the same relative layout exists in the
// pack source tree, which is what tests/scripts/session-progress.test.js
// imports).

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// sessionProgressPilotUnits are the renderable units the FULL Phase-1 pack
// ships, as live .opencode-relative paths (sorted). Both RenderUnits and
// UnitPaths must surface exactly this set. Sub-slice note: the list grows as
// sub-slices land (config/plugin at sub-slice 2, judge/policy at sub-slice 4);
// the final committed state asserts the complete set.
var sessionProgressPilotUnits = []string{
	".opencode/plugins/session-progress.js",
	".opencode/scripts/session-progress-config.js",
	".opencode/scripts/session-progress-judge.js",
	".opencode/scripts/session-progress-policy.js",
}

// TestOpenPack_SessionProgressPilotShips confirms the session-progress-pilot
// pack is shipped and openable from the embedded tree.
func TestOpenPack_SessionProgressPilotShips(t *testing.T) {
	pack, err := OpenPack("session-progress-pilot")
	if err != nil {
		t.Fatalf("OpenPack(session-progress-pilot): %v", err)
	}
	if pack == nil {
		t.Fatal("OpenPack(session-progress-pilot): nil pack")
	}
	if pack.Name != "session-progress-pilot" {
		t.Errorf("pack.Name: got %q, want session-progress-pilot", pack.Name)
	}
}

// TestRenderUnits_RealSessionProgressPilotPack confirms RenderUnits renders
// exactly the expected unit set under .opencode/ and excludes the
// merge-content file (opencode-append.jsonc) and the pack doc (README.md).
func TestRenderUnits_RealSessionProgressPilotPack(t *testing.T) {
	pack, err := OpenPack("session-progress-pilot")
	if err != nil {
		t.Fatalf("OpenPack(session-progress-pilot): %v", err)
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
	for _, wantUnit := range sessionProgressPilotUnits {
		if !contains(rendered, wantUnit) {
			t.Errorf("RenderUnits must render %q; got %v", wantUnit, rendered)
			continue
		}
		if _, err := os.Stat(filepath.Join(staging, filepath.FromSlash(wantUnit))); err != nil {
			t.Errorf("unit %q must land on disk under staging: %v", wantUnit, err)
		}
	}
	if len(rendered) != len(sessionProgressPilotUnits) {
		t.Errorf("RenderUnits count: got %d, want %d; got %v", len(rendered), len(sessionProgressPilotUnits), rendered)
	}
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

// TestUnitPaths_RealSessionProgressPilotPack confirms UnitPaths returns
// exactly the unit paths (the shadow-guard collision input lists only units,
// never merge-content or pack docs).
func TestUnitPaths_RealSessionProgressPilotPack(t *testing.T) {
	pack, err := OpenPack("session-progress-pilot")
	if err != nil {
		t.Fatalf("OpenPack(session-progress-pilot): %v", err)
	}
	paths, err := pack.UnitPaths()
	if err != nil {
		t.Fatalf("UnitPaths: %v", err)
	}
	sort.Strings(paths)
	want := append([]string{}, sessionProgressPilotUnits...)
	sort.Strings(want)
	if len(paths) != len(want) {
		t.Errorf("UnitPaths count: got %d, want %d; got %v", len(paths), len(want), paths)
	}
	for _, w := range want {
		if !contains(paths, w) {
			t.Errorf("UnitPaths must list %q; got %v", w, paths)
		}
	}
	for _, bad := range []string{
		opencodePrefix + appendFileName,
		".opencode/README.md",
	} {
		if contains(paths, bad) {
			t.Errorf("UnitPaths must not list %q; got %v", bad, paths)
		}
	}
}

// TestSessionProgressPilot_PackDocsShipped confirms the pack doc and the
// (intentionally empty) merge-content file are present in the embedded pack —
// the README documents the operator config schema and fail-open contract, and
// opencode-append.jsonc documents WHY no registration is needed.
func TestSessionProgressPilot_PackDocsShipped(t *testing.T) {
	pack, err := OpenPack("session-progress-pilot")
	if err != nil {
		t.Fatalf("OpenPack(session-progress-pilot): %v", err)
	}
	for _, f := range []string{"README.md", appendFileName} {
		raw, err := fs.ReadFile(pack.FS, f)
		if err != nil {
			t.Fatalf("pack must ship %s: %v", f, err)
		}
		if strings.TrimSpace(string(raw)) == "" {
			t.Errorf("pack file %s must be non-empty", f)
		}
	}
}

// TestReadCapabilityManifest_RealSessionProgressPilotPack pins the
// overlay-only structural property: the pack ships NO capability-manifest.yml
// (it declares no core capability), so ReadCapabilityManifest reports ok=false
// with no error.
func TestReadCapabilityManifest_RealSessionProgressPilotPack(t *testing.T) {
	pack, err := OpenPack("session-progress-pilot")
	if err != nil {
		t.Fatalf("OpenPack(session-progress-pilot): %v", err)
	}
	m, ok, err := pack.ReadCapabilityManifest()
	if err != nil {
		t.Fatalf("ReadCapabilityManifest: %v", err)
	}
	if ok {
		t.Errorf("session-progress-pilot is overlay-only (no manifest); got ok=true, manifest=%+v", m)
	}
}
