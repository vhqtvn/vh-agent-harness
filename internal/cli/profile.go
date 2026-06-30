package cli

import (
	"fmt"
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

// --- Phase 3 capability-installer: resolver wiring --------------------------
//
// The functions below run the capability resolver (Phase 2) against the live
// S3 profile's `capabilities:` selection and project the resolved set onto the
// flat "capabilities.<key>" render answers the GoTemplateRenderer consumes via
// buildTemplateData's dotted-key nesting. Templates then gate agent blocks and
// task-allowlist edges with {{ if .capabilities.<key> }}.
//
// The wiring lives in renderSeamStaging (see seam.go), not seamApply, so EVERY
// render path — install/update AND doctor's managed-drift re-render — resolves
// capabilities like-for-like (a gate true at install must also be true at
// doctor, else doctor false-flags drift on the gated agent blocks).

// phase3BackwardCompatDefault is the capability selection applied when a profile
// declares NO explicit `capabilities:` list. Phase 3 changes the render
// MECHANISM (gates + resolver wiring) but MUST NOT change the render BEHAVIOR
// for the existing dogfood profile (profile: minimal, no capabilities: key),
// which today ships all 20 agents. Selecting both core seeds reproduces that
// output exactly: core/gated-commit provides the 7 gated-commit agents and
// core/debate provides the 5 debate agents, so every capability gate evaluates
// true and every gated block renders.
//
// PHASE 5 TODO: flip this default to a baseline-only (empty) selection once the
// dogfood profile is migrated to an explicit capabilities: declaration (or
// `profile: supervised`). That single change here is the genuine "minimal = 8
// universal agents only" behavior flip; everything else (the template gates, the
// resolver wiring, the F1 baseline-never-satisfies-gate invariant) is already
// in place after Phase 3, so the flip is one findable edit.
var phase3BackwardCompatDefault = []resolver.CapabilityID{
	"core/gated-commit",
	"core/debate",
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

// readProfileCapabilities reads the explicit `capabilities:` list from the live
// S3 vh-harness-profile.yml at <target>. Contract mirrors readProfileAnswers:
//
//   - greenfield/missing file -> nil (the caller applies the backward-compat
//     default).
//   - malformed/unreadable -> nil (doctor reports the schema error separately;
//     render never aborts on a malformed profile).
//   - valid file with NO capabilities: key, null, OR an empty `capabilities:
//     []` -> nil (default applies; this is the current dogfood profile). Empty
//     is folded into the default because the profile normalizer materializes an
//     absent key as `capabilities: []` on the first update, so distinguishing
//     them would make the default vanish post-update. The genuine baseline-only
//     opt-out is Phase 5's flip of phase3BackwardCompatDefault to empty.
//   - valid file with capabilities: [ids] -> that selection.
//
// The nil-vs-non-empty distinction is load-bearing: nil (no explicit selection)
// applies the backward-compat default; a non-empty slice is honored as-is.
func readProfileCapabilities(target string) []resolver.CapabilityID {
	raw, err := os.ReadFile(filepath.Join(target, harnessProfileName))
	if err != nil {
		return nil // greenfield; default applies
	}
	if errs := (schema.HarnessProfile{}).Validate(raw); len(errs) > 0 {
		return nil // malformed; default applies (doctor reports the schema error)
	}
	var d struct {
		Capabilities []string `yaml:"capabilities"`
	}
	if err := yaml.Unmarshal(raw, &d); err != nil {
		return nil
	}
	// An absent key, an explicit null, and an explicit `capabilities: []` are
	// ALL treated as "no explicit selection -> default applies". They are
	// indistinguishable in practice because the profile normalizer materializes
	// an absent/null key as `capabilities: []` on the first update, so honoring
	// a nil-vs-empty distinction here would make the backward-compat default
	// vanish after a single update (false drift on every gated block). The only
	// way to opt OUT of the default is to declare a non-empty selection; the
	// genuine "baseline-only" behavior is Phase 5's flip of
	// phase3BackwardCompatDefault to an empty slice (then absent/empty -> empty
	// selection -> 8 universal agents).
	if len(d.Capabilities) == 0 {
		return nil
	}
	out := make([]resolver.CapabilityID, 0, len(d.Capabilities))
	for _, c := range d.Capabilities {
		out = append(out, c)
	}
	return out
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
// Catalog construction: Phase 3 uses CoreCatalog() alone — no overlay pack
// contributes capabilities today. PHASE 3 HOOK: when an overlay pack contributes
// a CapabilityManifest, merge it via resolver.MergeCatalogs here (read each
// active overlay pack's ReadCapabilityManifest and build PackContribution
// records) before resolving. Left as a marked hook because wiring overlay
// contributions at render time is out of scope for Phase 3 and no current
// overlay contributes.
func resolveCapabilityAnswers(target string) (map[string]string, error) {
	catalog := resolver.CoreCatalog()
	selected := readProfileCapabilities(target)
	if selected == nil {
		// No explicit capabilities: declaration -> backward-compat default so
		// the current profile renders byte-identically to pre-Phase-3.
		selected = append([]resolver.CapabilityID(nil), phase3BackwardCompatDefault...)
	}
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
