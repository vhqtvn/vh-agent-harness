package cli

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	corpus "github.com/vhqtvn/vh-agent-harness"
	"github.com/vhqtvn/vh-agent-harness/internal/resolver"
	"github.com/vhqtvn/vh-agent-harness/internal/schema"
)

// harnessProfileName is the S3 feature-surface authority file, platform_armed.
// It lives at the target root and carries the project's profile/modules/features/
// overlays/policy_packs selections.
const harnessProfileName = ".vh-agent-harness/vh-harness-profile.yml"

// readProfileAnswers reads the LIVE S3 vh-harness-profile.yml at
// <target>/vh-harness-profile.yml (the platform_armed feature-surface authority)
// and projects it onto flat render answers the GoTemplateRenderer consumes via
// buildTemplateData:
//
//   - "features.<flag>" -> "true"/"false"  (one key per bool feature; v1 = backlog)
//   - "overlays"        -> comma-joined overlay pack names (e.g. "alpha-pack")
//
// On first install the live profile is absent (it is seeded FROM the platform
// default by the apply step), so render decisions fall back to defaults
// (backlog=false, no overlays). On update the live profile drives the re-render,
// so Slice-3 backlog conditionals and Slice-4 overlay selection resolve from the
// operator's actual decisions rather than the install-time answers.
//
// A missing OR invalid profile yields an empty map (defaults). Doctor reports the
// real validation error separately; render never aborts on a malformed profile.
func readProfileAnswers(target string) map[string]string {
	raw, err := os.ReadFile(filepath.Join(target, harnessProfileName))
	if err != nil {
		// Greenfield: the live profile is being SEEDED this same run and isn't on
		// disk yet, so fall back to the EMBEDDED platform-default profile. This
		// keeps the render (e.g. opencode.jsonc's `features.backlog` block)
		// consistent with the profile install is about to seed — otherwise install
		// renders with hardcoded defaults while a later doctor/update re-renders
		// from the seeded profile and reports spurious drift on opencode.jsonc.
		return corpusDefaultProfileAnswers()
	}
	// Lint via the schema registry before trusting the projection. A malformed
	// profile cannot drive a render; fall back to defaults and let doctor flag it.
	if errs := (schema.HarnessProfile{}).Validate(raw); len(errs) > 0 {
		return map[string]string{}
	}
	return projectProfileAnswers(raw)
}

// corpusDefaultProfileAnswers projects the embedded platform-default
// vh-harness-profile.yml (templates/core), used on a greenfield install when the
// target has no live profile yet. Empty map on any read/parse failure.
func corpusDefaultProfileAnswers() map[string]string {
	sub, err := fs.Sub(corpus.CoreFS, corpus.CoreDir)
	if err != nil {
		return map[string]string{}
	}
	raw, err := fs.ReadFile(sub, harnessProfileName)
	if err != nil {
		return map[string]string{}
	}
	return projectProfileAnswers(raw)
}

// projectProfileAnswers is the pure projection of a validated vh-harness-profile.yml
// blob onto render answers. It is split from readProfileAnswers so Slice-4 overlay
// selection and tests can reuse the projection without re-reading the file.
func projectProfileAnswers(raw []byte) map[string]string {
	out := map[string]string{}
	// Reuse the schema's own struct shape (projection only; validation already ran).
	var d struct {
		Profile  string          `yaml:"profile"`
		Modules  []string        `yaml:"modules"`
		Features map[string]bool `yaml:"features"`
		Overlays []string        `yaml:"overlays"`
	}
	if err := yaml.Unmarshal(raw, &d); err != nil {
		return out
	}
	for k, v := range d.Features {
		out["features."+k] = boolStr(v)
	}
	if len(d.Overlays) > 0 {
		out["overlays"] = strings.Join(d.Overlays, ",")
	}
	return out
}

// activeOverlays returns the overlay pack names selected in the live profile, in
// declared order. It is the Slice-4 overlay-selection read site (each pack is
// rendered + merged when present here). Missing/invalid profile -> no overlays.
func activeOverlays(target string) []string {
	raw, err := os.ReadFile(filepath.Join(target, harnessProfileName))
	if err != nil {
		return nil
	}
	if errs := (schema.HarnessProfile{}).Validate(raw); len(errs) > 0 {
		return nil
	}
	var d struct {
		Overlays []string `yaml:"overlays"`
	}
	if err := yaml.Unmarshal(raw, &d); err != nil {
		return nil
	}
	return d.Overlays
}

// boolStr renders a Go bool as the canonical string form buildTemplateData's
// coerce() recognizes ("true"/"false"), so {{ if .features.backlog }} evaluates
// the boolean rather than a non-empty string.
func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// --- Capability-installer: profile-preset resolution (Phase 5 behavior flip) -
//
// The functions below run the capability resolver (Phase 2) against the live
// S3 profile's capability selection and project the resolved set onto the flat
// "capabilities.<key>" render answers the GoTemplateRenderer consumes via
// buildTemplateData's dotted-key nesting. Templates then gate agent blocks and
// task-allowlist edges with {{ if .capabilities.<key> }}.
//
// The wiring lives in renderSeamStaging (see seam.go), not seamApply, so EVERY
// render path — install/update AND doctor's managed-drift re-render — resolves
// capabilities like-for-like (a gate true at install must also be true at
// doctor, else doctor false-flags drift on the gated agent blocks).
//
// SELECTION MODEL (Phase 5). A profile's resolved capability selection is
//
//	preset(profile) ∪ explicit(capabilities:)
//
// where preset(profile) is a small map keyed by the `profile:` enum and the
// explicit `capabilities:` list is unioned on top. An empty selection is a
// valid, meaningful state — it resolves to the catalog's 8 universal baseline
// agents only (no clusters). This replaces the Phase-3 `phase3BackwardCompatDefault`
// bridge, which forced [core/gated-commit, core/debate] whenever a profile
// declared no explicit capabilities (so every profile rendered the full 20-agent
// roster regardless of its `profile:` enum). Phase 5 makes the enum mean
// something.

// profileCapabilityPresets maps the `profile:` enum value to the capability
// selection it carries as a BASE preset (before any explicit `capabilities:`
// are unioned in). Unknown values resolve to the empty preset (baseline-only),
// which is the safe default — a typo in `profile:` cannot silently grant
// clusters.
//
// Semantics:
//
//   - "minimal"      -> baseline-only (the 8 universal agents). This is the
//     genuine "lean harness" shape: coordination, build, project-coordinator,
//     researcher, repo-explorer, planner, docs-steward, ship-review.
//   - "supervised"   -> baseline + both core clusters (gated-commit, debate).
//     This is the full-featured shape this repo and vh-solara-style repos want:
//     it keeps the gated-commit protocol AND the multi-model debate workflow.
//   - "coordination" -> aliased to minimal (baseline-only). No capability cluster
//     is intrinsically scoped to "coordination-only" today; treating it as
//     baseline-only keeps the enum honest rather than inventing a fake default.
//     A profile that wants a cluster under `coordination` declares it via
//     `capabilities:`.
//   - "web"          -> aliased to minimal (baseline-only) for the same reason
//     as `coordination`: there is not yet a web-scoped capability cluster.
//
// When a new capability cluster is added whose scope aligns with one of the
// under-used enum values, extend this map rather than letting the enum drift.
var profileCapabilityPresets = map[string][]resolver.CapabilityID{
	"minimal":      nil,
	"supervised":   {"core/gated-commit", "core/debate"},
	"coordination": nil,
	"web":          nil,
}

// capabilityTemplateKey maps a capability ID (namespace/name) to the template
// predicate key used in {{ if .capabilities.<key> }} gates. The mapping strips
// the namespace (everything up to and including the last "/") and replaces every
// "-" with "_" so the key reads naturally and is a clean map lookup. Examples:
// "core/gated-commit" -> "gated_commit", "core/debate" -> "debate". Applied
// consistently for every catalog capability id.
//
// Rationale: capability IDs are namespaced ("core/gated-commit") but template
// predicates want a short, dash-free key ("/" and "-" are awkward inside
// .capabilities.<x> lookups and would require bracket syntax). Stripping the
// namespace is safe because capability Provides (agent names) are globally
// unique across the catalog (Catalog.Validate enforces this), and the namespace
// is redundant once the key is scoped under .capabilities.
func capabilityTemplateKey(id string) string {
	if i := strings.LastIndex(id, "/"); i >= 0 {
		id = id[i+1:]
	}
	return strings.ReplaceAll(id, "-", "_")
}

// parseProfileSelection is the pure projection of a validated vh-harness-profile.yml
// blob onto the (profile, capabilities) pair that drives capability resolution.
// It returns the `profile:` enum value (lowercased; "" if absent/unparsable) and
// the explicit `capabilities:` list (nil if absent/empty). Split from the I/O
// reader so tests can pin the preset-union behavior without touching the disk.
func parseProfileSelection(raw []byte) (profile string, capabilities []resolver.CapabilityID) {
	var d struct {
		Profile      string   `yaml:"profile"`
		Capabilities []string `yaml:"capabilities"`
	}
	if err := yaml.Unmarshal(raw, &d); err != nil {
		return "", nil
	}
	profile = strings.TrimSpace(d.Profile)
	out := make([]resolver.CapabilityID, 0, len(d.Capabilities))
	for _, c := range d.Capabilities {
		out = append(out, c)
	}
	if len(out) == 0 {
		return profile, nil
	}
	return profile, out
}

// presetCapabilities returns the base capability selection for a `profile:` enum
// value, copying the preset slice so callers may mutate freely. Unknown values
// resolve to nil (baseline-only) — the safe default.
func presetCapabilities(profile string) []resolver.CapabilityID {
	if preset, ok := profileCapabilityPresets[profile]; ok {
		return append([]resolver.CapabilityID(nil), preset...)
	}
	return nil
}

// unionCapabilities merges a base preset selection with an explicit opt-in list,
// deduplicating by capability id while preserving order (preset entries first,
// then explicit entries not already present). This implements the Phase-5 union
// semantics: `capabilities:` adds to the preset rather than replacing it, so a
// profile can say `profile: minimal` + `capabilities: [core/debate]` and get
// baseline + debate only.
func unionCapabilities(preset, explicit []resolver.CapabilityID) []resolver.CapabilityID {
	seen := make(map[string]struct{}, len(preset)+len(explicit))
	out := make([]resolver.CapabilityID, 0, len(preset)+len(explicit))
	for _, id := range preset {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, id := range explicit {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// corpusDefaultProfileSelection is the capability selection implied by the
// embedded platform-default profile, used on a greenfield install when the
// target has no live profile yet. Mirrors readProfileAnswers' greenfield
// fallback so install-time and post-update renders agree on the seeded profile.
func corpusDefaultProfileSelection() []resolver.CapabilityID {
	sub, err := fs.Sub(corpus.CoreFS, corpus.CoreDir)
	if err != nil {
		return nil
	}
	raw, err := fs.ReadFile(sub, harnessProfileName)
	if err != nil {
		return nil
	}
	profile, caps := parseProfileSelection(raw)
	return unionCapabilities(presetCapabilities(profile), caps)
}

// readProfileSelection reads the capability selection driving THIS render from
// the live S3 profile at <target>, applying the Phase-5 preset-union model:
//
//	preset(profile) ∪ explicit(capabilities:)
//
// Contract mirrors readProfileAnswers:
//
//   - greenfield/missing file -> the embedded platform-default profile's
//     selection (so install renders byte-identically to the profile it is about
//     to seed, avoiding spurious drift on the first doctor/update).
//   - malformed/unreadable -> nil selection (baseline-only). Doctor reports the
//     schema error separately; render never aborts on a malformed profile.
//   - valid file -> preset(profile) ∪ capabilities:.
func readProfileSelection(target string) []resolver.CapabilityID {
	raw, err := os.ReadFile(filepath.Join(target, harnessProfileName))
	if err != nil {
		return corpusDefaultProfileSelection() // greenfield; render the seeded shape
	}
	if errs := (schema.HarnessProfile{}).Validate(raw); len(errs) > 0 {
		return nil // malformed; baseline-only (doctor reports the schema error)
	}
	profile, caps := parseProfileSelection(raw)
	return unionCapabilities(presetCapabilities(profile), caps)
}

// resolveCapabilityAnswers runs the resolver for the live profile's capability
// selection and projects the result onto flat render answers:
//
//	"capabilities.<key>" -> "true"/"false"
//
// One answer key is emitted for EVERY capability id in the catalog (selected ->
// "true", absent -> "false"), so the `.capabilities` map always exists and every
// {{ if .capabilities.<key> }} gate evaluates a real boolean (never an
// absent-key zero value). This mirrors how features.backlog is projected: a
// complete, closed projection rather than a sparse one.
//
// Catalog construction: CoreCatalog() alone — no overlay pack contributes
// capabilities today. PHASE 3 HOOK: when an overlay pack contributes a
// CapabilityManifest, merge it via resolver.MergeCatalogs here (read each active
// overlay pack's ReadCapabilityManifest and build PackContribution records)
// before resolving. Left as a marked hook because wiring overlay contributions
// at render time is out of scope for Phase 3 and no current overlay contributes.
func resolveCapabilityAnswers(target string) (map[string]string, error) {
	catalog := resolver.CoreCatalog()
	selected := readProfileSelection(target)
	set, err := resolver.Resolve(selected, catalog)
	if err != nil {
		return nil, fmt.Errorf("resolve capabilities %v: %w", selected, err)
	}
	out := make(map[string]string, len(catalog.IDs()))
	for _, id := range catalog.IDs() {
		out["capabilities."+capabilityTemplateKey(id)] = boolStr(set.Has(id))
	}
	return out, nil
}

// --- modules: deprecation (Phase 5) ------------------------------------------
//
// `modules:` is the legacy profile field that pre-dates the capability-preset
// model. Under Phase 5 it is redundant: the `profile:` enum's presets now carry
// the intent `modules:` once approximated (a curated bundle of agents). It is
// NOT removed from the schema (deprecation only, so existing profiles keep
// parsing), but a non-empty `modules:` list surfaces a one-line warning on every
// update and doctor run so operators know to migrate.

// profileDeprecationSink is the writer for profile-deprecation warnings. It
// defaults to os.Stderr (the seam.go warning channel) but is a package-level
// variable so tests can swap it for a buffer and assert the warning text. This
// mirrors the swappable-sink precedent in internal/runtime/bare.go
// (`bareStderr`) and internal/permission/permission.go (`stderr`).
var profileDeprecationSink io.Writer = os.Stderr

// modulesDeprecationWarning returns the one-line deprecation warning for a
// non-empty `modules:` list, or "" when the list is nil/empty (no warning to
// emit). Pure + testable; the caller owns the sink write. The modules values are
// not interpolated into the message (they are operator-controlled and could
// contain shell metacharacters); only the count is reported.
func modulesDeprecationWarning(modules []string) string {
	if len(modules) == 0 {
		return ""
	}
	return fmt.Sprintf(
		"seam: warning: vh-harness-profile.yml `modules:` (%d entry) is deprecated; "+
			"use the `profile:` enum (presets) and `capabilities:` (opt-in union) instead.\n",
		len(modules),
	)
}

// liveProfileModules reads the `modules:` list from the LIVE S3 profile only
// (NOT the embedded default), returning nil when the live profile is absent.
// This is deliberate: the embedded default still ships `modules: [core]`, and we
// do NOT want to warn during the greenfield seeding render (install) where the
// live profile does not yet exist — only on a real update/doctor where the
// operator's live profile still carries a now-meaningless `modules:`. A
// malformed profile yields nil (doctor reports the schema error separately).
func liveProfileModules(target string) []string {
	raw, err := os.ReadFile(filepath.Join(target, harnessProfileName))
	if err != nil {
		return nil // greenfield; no live profile to warn about
	}
	if errs := (schema.HarnessProfile{}).Validate(raw); len(errs) > 0 {
		return nil // malformed; doctor reports the schema error
	}
	var d struct {
		Modules []string `yaml:"modules"`
	}
	if err := yaml.Unmarshal(raw, &d); err != nil {
		return nil
	}
	return d.Modules
}

// emitModulesDeprecationWarning writes the modules-deprecation warning to
// profileDeprecationSink when the live profile carries a non-empty `modules:`
// list. No-op otherwise (greenfield, empty modules, or malformed profile). Used
// by renderSeamStaging so the warning fires on both update (seamApply) and
// doctor (checkManagedDrift) renders.
func emitModulesDeprecationWarning(target string) {
	if msg := modulesDeprecationWarning(liveProfileModules(target)); msg != "" {
		fmt.Fprint(profileDeprecationSink, msg)
	}
}
