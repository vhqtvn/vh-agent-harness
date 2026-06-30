package cli

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	corpus "github.com/vhqtvn/vh-agent-harness"
	"github.com/vhqtvn/vh-agent-harness/internal/overlay"
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
// selection, projects the resolved set onto flat render answers, AND computes
// the deduplicated pack list that must RENDER this pass. It completes the
// Phase-3 capability-installer's overlay integration: any discoverable overlay
// pack (embedded under templates/overlays or project-local under
// <target>/.vh-agent-harness/overlays) that ships a capability-manifest.yml
// participates in the resolver AUTOMATICALLY.
//
// The two selection paths CONVERGE here so either way of opting into a pack's
// capability renders the same cluster:
//
//   - overlays: [release]           -> release pack renders AND core/release
//     resolves (pulling core/gated-commit in via the hard-dep closure).
//   - capabilities: [core/release]  -> same: release pack renders AND
//     core/gated-commit resolves.
//
// Catalog construction:
//
//	CoreCatalog() merged with EVERY discoverable overlay pack's
//	capability-manifest.yml (embedded + project-local, project-wins shadowing).
//
// Selection:
//
//	preset(profile) ∪ explicit(capabilities:) ∪
//	{capabilities provided by overlays:-listed packs that declare a manifest}
//
// The third operand is the convergence: a pack listed in overlays: whose
// manifest declares a capability id also feeds the resolver, so its hard_deps
// close and its cluster gates open — without it, overlays:[release] would
// render the pack's units but leave core/gated-commit unselected and the
// releaser's own capability gate closed.
//
// Returns:
//   - answers: "capabilities.<key>" -> "true"/"false" (one key per catalog id;
//     a closed projection so every {{ if .capabilities.<key> }} gate evaluates
//     a real boolean, never an absent-key zero value).
//   - renderPacks: the pack names that must render this pass — the explicit
//     overlays: list (profile order) UNION packs owning resolved capabilities
//     (sorted, deduplicated). Either selection path therefore reaches the
//     render loop.
func resolveCapabilityAnswers(target string) (answers map[string]string, renderPacks []string, err error) {
	contribs, derr := discoverPackContributions(target)
	if derr != nil {
		return nil, nil, fmt.Errorf("seam: %w", derr)
	}
	survivors := resolver.ResolveContributions(contribs)

	// Build the capability-id <-> pack-name maps from the surviving (post
	// project-wins shadowing) contributions. Both directions are needed:
	// packToID turns an overlays:-listed pack into a capability id for the
	// selection; idToPack turns a resolved capability id into a pack to render.
	idToPack := make(map[string]string, len(survivors))
	packToID := make(map[string]string, len(survivors))
	for _, c := range survivors {
		idToPack[c.Manifest.ID] = c.Pack
		packToID[c.Pack] = c.Manifest.ID
	}

	catalog, merr := resolver.MergeCatalogs(resolver.CoreCatalog(), contribs)
	if merr != nil {
		return nil, nil, fmt.Errorf("seam: %w", merr)
	}

	// Selection = preset(profile) ∪ explicit(capabilities:) ∪ capabilities
	// implied by overlays:-listed packs that declare a manifest.
	explicit := readProfileSelection(target) // preset(profile) ∪ capabilities:
	overlayImplied := make([]resolver.CapabilityID, 0)
	for _, name := range activeOverlays(target) {
		if id, ok := packToID[name]; ok {
			overlayImplied = append(overlayImplied, id)
		}
	}
	selected := unionCapabilities(explicit, overlayImplied)

	set, rerr := resolver.Resolve(selected, catalog)
	if rerr != nil {
		return nil, nil, fmt.Errorf("resolve capabilities %v: %w", selected, rerr)
	}

	// Render answers: one key per catalog id (closed projection).
	out := make(map[string]string, len(catalog.IDs()))
	for _, id := range catalog.IDs() {
		out["capabilities."+capabilityTemplateKey(id)] = boolStr(set.Has(id))
	}

	// Render packs: explicit overlays: list (profile order, preserved) UNION
	// packs owning resolved capabilities (sorted, appended after the explicit
	// list). Preserving explicit-list order keeps existing pack-rendering
	// semantics (shadowing guard sees packs in profile order); the implied
	// additions are sorted for determinism.
	explicitList := activeOverlays(target)
	seen := make(map[string]bool, len(explicitList)+len(survivors))
	packs := make([]string, 0, len(explicitList)+len(survivors))
	for _, name := range explicitList {
		if !seen[name] {
			seen[name] = true
			packs = append(packs, name)
		}
	}
	var implied []string
	for _, id := range set.All() {
		if name, ok := idToPack[id]; ok && !seen[name] {
			seen[name] = true
			implied = append(implied, name)
		}
	}
	sort.Strings(implied)
	packs = append(packs, implied...)

	return out, packs, nil
}

// discoverPackContributions reads capability-manifest.yml from EVERY
// discoverable overlay pack — embedded under templates/overlays (via
// overlay.KnownPacks) and project-local under
// <target>/.vh-agent-harness/overlays — and returns a PackContribution record
// for each pack that declares a manifest.
//
// SHADOWING MODEL (Model X). A project-local pack shadows an embedded pack by
// name WHOLLY — rendered files (OpenPackFor is project-first at the render
// layer) AND capability identity (this catalog). So:
//
//   - If the project pack HAS a capability-manifest.yml, its manifest becomes
//     the capability (resolver.MergeCatalogs applies project-wins at the merge
//     layer; resolver.ResolveContributions preserves it as SourceProject).
//   - If the project pack has NO manifest, it is overlay-only: it renders its
//     files but provides NO capability, and the embedded capability it shadows
//     by name is DROPPED here (NOT inherited). To REPLACE (rather than drop) a
//     shadowed capability, ship a capability-manifest.yml in the project pack.
//
// This is enforced by building the project pack-name set FIRST and skipping any
// embedded pack whose name it contains BEFORE emitting a contribution, so the
// render layer (OpenPackFor project-first) and the resolver layer (this
// catalog) agree on what "the pack" is. A present-but-malformed manifest is a
// hard error (fail-closed): silently dropping a broken pack from the resolver
// would hide a real configuration problem and could let a hard_dep quietly go
// unsatisfied.
//
// A missing project overlays directory is benign (greenfield or a project that
// ships no packs of its own) and yields contributions from the embedded tree
// only.
func discoverPackContributions(target string) ([]resolver.PackContribution, error) {
	var contribs []resolver.PackContribution

	// Build the project pack-name set FIRST. A project-local pack that shadows
	// an embedded pack by name replaces it WHOLLY (files + capability), so any
	// embedded pack whose name collides MUST be skipped below regardless of
	// whether the project pack has a manifest. A no-manifest project pack is
	// overlay-only (renders its files, provides no capability, drops the
	// embedded capability it shadows); a with-manifest project pack supplies its
	// own capability via the project loop below (MergeCatalogs project-wins).
	projectDir := filepath.Join(target, filepath.FromSlash(overlay.ProjectOverlaysSubdir))
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		// Missing project overlays dir is benign; any other read error fails
		// closed so a permissions problem surfaces rather than silently masking
		// a project pack from the resolver.
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("discover overlay contributions: read project overlays dir %q: %w", projectDir, err)
		}
		entries = nil
	}
	projectPackNames := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			projectPackNames[e.Name()] = struct{}{}
		}
	}

	// Embedded packs (shipped inside the binary). Skip any whose name is
	// shadowed WHOLLY by a project-local pack of the same name (Model X).
	embedded, err := overlay.KnownPacks()
	if err != nil {
		return nil, fmt.Errorf("discover overlay contributions: list embedded packs: %w", err)
	}
	for _, name := range embedded {
		if _, shadowed := projectPackNames[name]; shadowed {
			continue // project pack shadows this embedded pack wholly (files + capability)
		}
		pack, err := overlay.OpenPack(name)
		if err != nil {
			return nil, fmt.Errorf("discover overlay contributions: open embedded pack %q: %w", name, err)
		}
		m, ok, err := pack.ReadCapabilityManifest()
		if err != nil {
			return nil, fmt.Errorf("discover overlay contributions: read manifest %q: %w", name, err)
		}
		if !ok {
			continue // skills-only / no-manifest EMBEDDED pack: renders via overlays: but resolves nothing
		}
		contribs = append(contribs, resolver.PackContribution{
			Pack: name, Source: resolver.SourceEmbedded, Manifest: m,
		})
	}

	// Project-local packs (the consuming project's .vh-agent-harness/overlays).
	// OpenPackFor resolves project-first, so it opens THIS project dir's
	// manifest — the same project pack FS the render loop loads — keeping the
	// resolver and render layers in agreement. A project pack WITHOUT a
	// manifest contributes nothing here (overlay-only); its files still render
	// via the render loop's OpenPackFor.
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		pack, err := overlay.OpenPackFor(target, name)
		if err != nil {
			return nil, fmt.Errorf("discover overlay contributions: open project pack %q: %w", name, err)
		}
		m, ok, err := pack.ReadCapabilityManifest()
		if err != nil {
			return nil, fmt.Errorf("discover overlay contributions: read manifest %q: %w", name, err)
		}
		if !ok {
			continue
		}
		contribs = append(contribs, resolver.PackContribution{
			Pack: name, Source: resolver.SourceProject, Manifest: m,
		})
	}
	return contribs, nil
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
