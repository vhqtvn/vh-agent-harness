package cli

// This file closes the Part-1 overlay-selection test gap. activeOverlays (the
// vh-harness-profile.yml -> selected pack-names read site) and the profile->render-
// answer projection (projectProfileAnswers) live in this package (profile.go),
// not in internal/overlay, so they are tested here. They are the bridge between
// the operator's profile selection and the overlay package's OpenPack: until now
// exercised only via the seam install/update tests.

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/overlay"
)

// writeProfile writes a vh-harness-profile.yml body into target root.
func writeProfile(t *testing.T, target string, body string) {
	t.Helper()
	p := filepath.Join(target, harnessProfileName)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir profile dir: %v", err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
}

// --- activeOverlays: with / without overlays --------------------------------

// TestActiveOverlays_AbsentProfileReturnsNil confirms a missing profile yields
// no overlay selection (first-install default: core only).
func TestActiveOverlays_AbsentProfileReturnsNil(t *testing.T) {
	target := t.TempDir()
	if got := activeOverlays(target); got != nil {
		t.Errorf("absent profile: got %v, want nil", got)
	}
}

// TestActiveOverlays_InvalidProfileReturnsNil confirms a schema-invalid profile
// yields no selection (render never trusts a malformed profile; doctor reports
// the validation error separately).
func TestActiveOverlays_InvalidProfileReturnsNil(t *testing.T) {
	target := t.TempDir()
	writeProfile(t, target, "profile: experimental_not_in_enum\nmodules: [core]\noverlays: [web-overlay]\npolicy_packs: []\n")
	if got := activeOverlays(target); got != nil {
		t.Errorf("invalid profile: got %v, want nil (must not trust malformed profile)", got)
	}
}

// TestActiveOverlays_NoOverlaysKeyReturnsEmpty confirms a valid profile with no
// overlays (or an empty overlays list) selects nothing.
func TestActiveOverlays_NoOverlaysKeyReturnsEmpty(t *testing.T) {
	target := t.TempDir()
	// Valid profile, no overlays key at all.
	writeProfile(t, target, "profile: minimal\nmodules: [core]\nfeatures:\n  backlog: false\noverlays: []\npolicy_packs: []\n")
	got := activeOverlays(target)
	if len(got) != 0 {
		t.Errorf("no-overlays profile: got %v, want empty", got)
	}
}

// TestActiveOverlays_ReturnsSelectionInOrder confirms the overlays list is
// returned in declared order (render order is pack-order-sensitive: a later pack
// can deep-merge keys an earlier pack contributed). The names are arbitrary
// declared strings — activeOverlays reads the profile verbatim and does NOT
// intersect with overlay.KnownPacks (unknown names are skipped at apply time),
// so the round-trip holds for any syntactically valid list.
func TestActiveOverlays_ReturnsSelectionInOrder(t *testing.T) {
	target := t.TempDir()
	// Two arbitrary declared pack names to prove declared-order round-trip
	// without implying any particular pack is shipped.
	writeProfile(t, target, "profile: supervised\nmodules: [core]\nfeatures:\n  backlog: true\noverlays: [alpha-pack, beta-pack]\npolicy_packs: []\n")
	got := activeOverlays(target)
	want := []string{"alpha-pack", "beta-pack"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("overlays order: got %v, want %v", got, want)
	}
}

// TestActiveOverlays_AllShippedPacksSelectable confirms each name reported by
// overlay.KnownPacks (the real shipped set) round-trips through a valid profile
// selection. The harness ships six embedded packs — `auto-classifier-pilot`,
// `contract-invariant-audit-pilot`, `formal-verification-pilot`, `release`,
// `repo-mail`, and `resolve-first-pilot` — so this loop exercises all six.
// web-overlay remains relocated to a non-shipped adoption reference under
// docs/adoption-examples/web/ and is NOT a shipped pack.
func TestActiveOverlays_AllShippedPacksSelectable(t *testing.T) {
	shipped, err := overlay.KnownPacks()
	if err != nil {
		t.Fatalf("overlay.KnownPacks: %v", err)
	}
	for _, name := range shipped {
		t.Run(name, func(t *testing.T) {
			target := t.TempDir()
			writeProfile(t, target, "profile: minimal\nmodules: [core]\noverlays: ["+name+"]\npolicy_packs: []\n")
			got := activeOverlays(target)
			if len(got) != 1 || got[0] != name {
				t.Errorf("pack %q did not round-trip; got %v", name, got)
			}
		})
	}
}

// --- projectProfileAnswers: render-answer projection ------------------------

// TestProjectProfileAnswers_ProjectsFeaturesAndOverlays confirms the profile is
// projected onto the flat render answers the GoTemplateRenderer consumes:
// "features.<flag>" -> "true"/"false" and "overlays" -> comma-joined names. The
// overlay names are arbitrary declared strings (not tied to any shipped pack).
func TestProjectProfileAnswers_ProjectsFeaturesAndOverlays(t *testing.T) {
	raw := []byte("profile: supervised\nmodules: [core]\nfeatures:\n  backlog: true\n  experiments: false\noverlays: [alpha-pack, beta-pack]\npolicy_packs: []\n")
	ans := projectProfileAnswers(raw)
	if ans["features.backlog"] != "true" {
		t.Errorf("features.backlog: got %q, want true", ans["features.backlog"])
	}
	if ans["features.experiments"] != "false" {
		t.Errorf("features.experiments: got %q, want false", ans["features.experiments"])
	}
	if ans["overlays"] != "alpha-pack,beta-pack" {
		t.Errorf("overlays: got %q, want alpha-pack,beta-pack", ans["overlays"])
	}
}

// TestProjectProfileAnswers_NoOverlaysOmitsKey confirms a profile with no
// overlays does NOT inject an empty "overlays" key (so {{ if .overlays }} stays
// false rather than truthy-on-empty-string).
func TestProjectProfileAnswers_NoOverlaysOmitsKey(t *testing.T) {
	raw := []byte("profile: minimal\nmodules: [core]\nfeatures:\n  backlog: false\noverlays: []\npolicy_packs: []\n")
	ans := projectProfileAnswers(raw)
	if v, ok := ans["overlays"]; ok {
		t.Errorf("empty overlays must omit the key; got %q", v)
	}
	if ans["features.backlog"] != "false" {
		t.Errorf("features.backlog: got %q, want false", ans["features.backlog"])
	}
}

// TestReadProfileAnswers_MissingFileFallsBackToCorpusDefault confirms that on a
// greenfield install (no live profile yet) readProfileAnswers projects the
// EMBEDDED platform-default profile, so the render matches the profile install is
// about to seed (e.g. features.backlog defaults true). This prevents spurious
// opencode.jsonc drift between install and the first doctor/update.
func TestReadProfileAnswers_MissingFileFallsBackToCorpusDefault(t *testing.T) {
	target := t.TempDir()
	ans := readProfileAnswers(target)
	if got := ans["features.backlog"]; got != "true" {
		t.Errorf("missing profile should fall back to corpus default (features.backlog=true), got %q", got)
	}
}

// --- shipped-pilot feature activation ----------------------------------------
//
// The tests below cover the default-on shipped-pilot distribution mechanism
// (see researches/decisions/2026-08-04-opt-out-pilot-distribution.md). The 3
// pilot packs are default-on for every consumer and disable-able via
// features: <key>: false. The mechanism lives in reconciledFeatures (render-time
// platform-default ∪ project-wins) + featureActivatedPacks (finite feature-key
// -> pack-name table) + the union in resolveCapabilityAnswers.

// containsPack is a small helper to check a pack name is in a slice.
func containsPack(packs []string, name string) bool {
	for _, p := range packs {
		if p == name {
			return true
		}
	}
	return false
}

// TestReconciledFeatures_GreenfieldReturnsPlatformDefault confirms a target with
// no live profile (greenfield install) gets the platform default features
// verbatim, including the 3 shipped-pilot keys (default true).
func TestReconciledFeatures_GreenfieldReturnsPlatformDefault(t *testing.T) {
	target := t.TempDir()
	feats := reconciledFeatures(target)
	for _, key := range []string{
		"formal-verification-pilot",
		"resolve-first-pilot",
		"contract-invariant-audit-pilot",
	} {
		if !feats[key] {
			t.Errorf("greenfield features[%q] = false, want true (platform default-on)", key)
		}
	}
	if !feats["backlog"] {
		t.Errorf("greenfield features[backlog] = false, want true (platform default)")
	}
}

// TestReconciledFeatures_ProjectFalseOverridesDefault confirms an explicit
// features: <key>: false in the live profile disables the default-on pilot for
// THAT key only (the other two stay on).
func TestReconciledFeatures_ProjectFalseOverridesDefault(t *testing.T) {
	target := t.TempDir()
	writeProfile(t, target, "profile: minimal\nfeatures:\n  formal-verification-pilot: false\noverlays: []\npolicy_packs: []\n")
	feats := reconciledFeatures(target)
	if feats["formal-verification-pilot"] {
		t.Errorf("explicit false must disable default; features[formal-verification-pilot] = true, want false")
	}
	if !feats["resolve-first-pilot"] {
		t.Errorf("unrelated pilot must stay on; features[resolve-first-pilot] = false, want true")
	}
	if !feats["contract-invariant-audit-pilot"] {
		t.Errorf("unrelated pilot must stay on; features[contract-invariant-audit-pilot] = false, want true")
	}
}

// TestReconciledFeatures_MalformedProfileReturnsPlatformDefault confirms a
// schema-invalid live profile falls back to the platform default verbatim (all 3
// pilots on) rather than stripping them. A broken profile must not silently
// strip shipped pilots; doctor reports the schema error separately.
func TestReconciledFeatures_MalformedProfileReturnsPlatformDefault(t *testing.T) {
	target := t.TempDir()
	writeProfile(t, target, "profile: experimental_not_in_enum\nfeatures:\n  backlog: true\noverlays: [web-overlay]\npolicy_packs: []\n")
	feats := reconciledFeatures(target)
	for _, key := range []string{
		"formal-verification-pilot",
		"resolve-first-pilot",
		"contract-invariant-audit-pilot",
	} {
		if !feats[key] {
			t.Errorf("malformed profile must fall back to platform default (3 pilots on); features[%q] = false, want true", key)
		}
	}
}

// TestProjectProfileAnswers_AbsentPilotKeyIsNotProjected confirms the RAW
// projection (projectProfileAnswers) does NOT inject pilot feature keys that are
// absent from the profile — they evaluate false at raw template lookup via Go's
// zero value. The default-on semantics come from reconciledFeatures (which unions
// in the platform default), NOT from the raw projection. This keeps the two
// layers honest: raw projection = what the profile literally says; reconciled =
// what the render actually sees.
func TestProjectProfileAnswers_AbsentPilotKeyIsNotProjected(t *testing.T) {
	raw := []byte("profile: minimal\nfeatures:\n  backlog: true\noverlays: []\npolicy_packs: []\n")
	ans := projectProfileAnswers(raw)
	for _, key := range []string{
		"features.formal-verification-pilot",
		"features.resolve-first-pilot",
		"features.contract-invariant-audit-pilot",
	} {
		if v, ok := ans[key]; ok {
			t.Errorf("absent pilot key %q must not be projected (raw lookup = zero-value false); got %q", key, v)
		}
	}
}

// TestFeatureActivatedPacks_DefaultOnNoOverlays confirms a consumer profile with
// no explicit overlays and no explicit feature keys still activates the 3 pilot
// packs via the default-on mechanism. This is the crux: a bare `profile: minimal`
// consumer GAINS the 3 pilot skills on update without any opt-in.
func TestFeatureActivatedPacks_DefaultOnNoOverlays(t *testing.T) {
	target := t.TempDir()
	writeProfile(t, target, "profile: minimal\noverlays: []\npolicy_packs: []\n")
	_, packs, _, _, err := resolveCapabilityAnswers(target)
	if err != nil {
		t.Fatalf("resolveCapabilityAnswers: %v", err)
	}
	for _, name := range []string{
		"formal-verification-pilot",
		"resolve-first-pilot",
		"contract-invariant-audit-pilot",
	} {
		if !containsPack(packs, name) {
			t.Errorf("default-on pilot %q missing from renderPacks %v", name, packs)
		}
	}
}

// TestFeatureActivatedPacks_OptOutFalseDropsOne confirms setting a single pilot
// feature to false drops ONLY that pilot from the render list (the other two
// remain on).
func TestFeatureActivatedPacks_OptOutFalseDropsOne(t *testing.T) {
	target := t.TempDir()
	writeProfile(t, target, "profile: minimal\nfeatures:\n  formal-verification-pilot: false\noverlays: []\npolicy_packs: []\n")
	_, packs, _, _, err := resolveCapabilityAnswers(target)
	if err != nil {
		t.Fatalf("resolveCapabilityAnswers: %v", err)
	}
	if containsPack(packs, "formal-verification-pilot") {
		t.Errorf("opted-out pilot must NOT render; found it in %v", packs)
	}
	if !containsPack(packs, "resolve-first-pilot") {
		t.Errorf("non-opted-out pilot must still render; resolve-first-pilot missing from %v", packs)
	}
	if !containsPack(packs, "contract-invariant-audit-pilot") {
		t.Errorf("non-opted-out pilot must still render; contract-invariant-audit-pilot missing from %v", packs)
	}
}

// TestFeatureActivatedPacks_ExplicitOverlaySurvivesOptOut confirms that an
// explicit overlays: entry for a pilot SURVIVES a features: <key>: false opt-out.
// This proves `false` disables only the DEFAULT-on path — it is NOT a global
// veto. An operator who explicitly lists the pack under overlays: gets it
// regardless of the feature flag.
func TestFeatureActivatedPacks_ExplicitOverlaySurvivesOptOut(t *testing.T) {
	target := t.TempDir()
	writeProfile(t, target, "profile: minimal\nfeatures:\n  formal-verification-pilot: false\noverlays: [formal-verification-pilot]\npolicy_packs: []\n")
	_, packs, _, _, err := resolveCapabilityAnswers(target)
	if err != nil {
		t.Fatalf("resolveCapabilityAnswers: %v", err)
	}
	if !containsPack(packs, "formal-verification-pilot") {
		t.Errorf("explicit overlays: entry must survive features:false (NOT a global veto); missing from %v", packs)
	}
}
