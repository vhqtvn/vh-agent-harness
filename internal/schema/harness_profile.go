package schema

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// HarnessProfile is the S3 feature-surface selection schema
// (vh-harness-profile.yml). Ownership class: platform_armed.
//
// The platform owns the schema and the default; the project edits within the
// envelope. Reconcile policy (v1):
//
//   - profile (project-selectable scalar, enum-constrained):
//   - project empty/absent -> platform default
//   - project value in enum -> project value (project-wins)
//   - project value NOT in enum (platform removed it) -> PROPOSAL enum_removed
//   - modules / overlays / policy_packs (append-only arrays): union-dedup-sorted,
//     always clean (apply). Project additions retained; platform additions merged.
//   - features (keyed map of bool): platform defaults form the base; project
//     overrides applied per key (project-wins); new platform keys added with their
//     default. Always clean (apply).
//
// Unknown top-level keys are a Validate error (envelope violation), NOT a
// reconcile proposal: the schema is the contract, and stray keys are malformed.
type HarnessProfile struct{}

// profileEnum is the exhaustive set of valid profile values for v1.
var profileEnum = map[string]bool{
	"minimal":      true,
	"coordination": true,
	"supervised":   true,
	"web":          true,
}

// profileEnumList returns the enum members sorted, joined by " | ", for error
// and proposal envelope text. Computed once.
var profileEnumList = func() string {
	out := make([]string, 0, len(profileEnum))
	for k := range profileEnum {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, " | ")
}()

// harnessProfileData is the parsed, typed projection of a vh-harness-profile.yml.
type harnessProfileData struct {
	Profile     string          `yaml:"profile"`
	Modules     []string        `yaml:"modules"`
	Features    map[string]bool `yaml:"features"`
	Overlays    []string        `yaml:"overlays"`
	PolicyPacks []string        `yaml:"policy_packs"`
	// Capabilities is the profile's selected capability IDs (grammar
	// namespace/name, e.g. core/gated-commit). Phase 1 accepts and validates the
	// field (non-empty, de-duplicated entries, like the other string arrays);
	// the resolver (Phase 2) will resolve these IDs against the central core
	// Catalog and expand preset semantics. The legacy modules: key is NOT
	// deprecated by this field yet. ID grammar and existence are resolver
	// concerns, not schema concerns: the schema only carries the selection list.
	Capabilities []string `yaml:"capabilities"`
}

// allowedTopLevel is the exhaustive set of top-level keys a vh-harness-profile.yml
// may carry. Anything else is an envelope violation.
var harnessProfileAllowedTopLevel = map[string]bool{
	"profile":      true,
	"modules":      true,
	"features":     true,
	"overlays":     true,
	"policy_packs": true,
	"capabilities": true,
}

// Validate reports every structural problem in a vh-harness-profile.yml instance.
// It does NOT perform I/O and does NOT reconcile. An empty result is conformant.
func (HarnessProfile) Validate(raw []byte) []FieldError {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return []FieldError{{Field: "<root>", Message: "file is empty"}}
	}
	// Generic pass first to catch stray keys (envelope).
	var root map[string]any
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return []FieldError{{Field: "<root>", Message: fmt.Sprintf("not valid YAML: %v", err)}}
	}
	var errs []FieldError
	keys := make([]string, 0, len(root))
	for k := range root {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !harnessProfileAllowedTopLevel[k] {
			errs = append(errs, FieldError{
				Field:   k,
				Message: fmt.Sprintf("unknown top-level key %q; allowed: profile, modules, features, overlays, policy_packs, capabilities", k),
			})
		}
	}

	var d harnessProfileData
	if err := yaml.Unmarshal(raw, &d); err != nil {
		errs = append(errs, FieldError{Field: "<root>", Message: fmt.Sprintf("shape mismatch: %v", err)})
		return errs
	}

	// profile: optional, but if present must be a valid enum member.
	if d.Profile != "" && !profileEnum[d.Profile] {
		errs = append(errs, FieldError{
			Field:   "profile",
			Message: fmt.Sprintf("invalid profile %q; enum: %s", d.Profile, profileEnumList),
		})
	}

	// modules / overlays / policy_packs / capabilities: must be string arrays
	// (yaml already enforced the element type via the struct; a non-array would
	// have failed Unmarshal above). No further v1 constraints. capabilities IDs
	// (namespace/name grammar, existence) are validated by the resolver (Phase
	// 2), not the schema — the schema only carries the selection list.
	errs = appendStringArrayErrors(errs, "modules", d.Modules)
	errs = appendStringArrayErrors(errs, "overlays", d.Overlays)
	errs = appendStringArrayErrors(errs, "policy_packs", d.PolicyPacks)
	errs = appendStringArrayErrors(errs, "capabilities", d.Capabilities)

	// features: map[string]bool already enforced. No further v1 constraints.
	return errs
}

// AppendOverlay returns a vh-harness-profile.yml instance with name added to the
// overlays list, serialized through the SAME load/marshal path the Reconciler
// uses (loadHarnessProfile + marshalHarnessProfile). It is the safe, schema-
// shaped mutation site for tooling that must edit the live overlays selection
// (e.g. the `vh-agent-harness overlay new` scaffolder) WITHOUT a naive
// text/regex edit on the platform_armed file.
//
// raw may be empty/nil — a fresh conformant instance carrying only
// `overlays: [name]` is built. When raw is non-empty it is Validated FIRST; a
// schema-invalid instance is returned as an error (never silently re-serialized
// into a different shape). name must be non-empty.
//
// The marshaled output is normalized exactly the way an applied reconcile would
// normalize it (arrays sorted + deduped), so a subsequent `update` reconcile
// treats the file as clean (no spurious drift / proposal).
//
// added is false when name was already present in overlays — merged is then the
// re-serialized current state and the caller may skip the write (no change).
// added is true when name was appended.
func (HarnessProfile) AppendOverlay(raw []byte, name string) (merged []byte, added bool, err error) {
	if strings.TrimSpace(name) == "" {
		return nil, false, fmt.Errorf("append-overlay: name must be non-empty")
	}
	if len(strings.TrimSpace(string(raw))) > 0 {
		if errs := (HarnessProfile{}).Validate(raw); len(errs) > 0 {
			return nil, false, fmt.Errorf("append-overlay: refusing to mutate a schema-invalid vh-harness-profile.yml: %v", errs)
		}
	}
	d, lerr := loadHarnessProfile(raw)
	if lerr != nil {
		return nil, false, fmt.Errorf("append-overlay: parse vh-harness-profile.yml: %w", lerr)
	}
	for _, existing := range d.Overlays {
		if existing == name {
			out, merr := marshalHarnessProfile(d)
			if merr != nil {
				return nil, false, fmt.Errorf("append-overlay: marshal: %w", merr)
			}
			return out, false, nil
		}
	}
	d.Overlays = append(d.Overlays, name)
	out, merr := marshalHarnessProfile(d)
	if merr != nil {
		return nil, false, fmt.Errorf("append-overlay: marshal: %w", merr)
	}
	return out, true, nil
}

// appendStringArrayErrors adds per-element errors for empty/duplicate entries.
func appendStringArrayErrors(errs []FieldError, field string, vals []string) []FieldError {
	seen := make(map[string]bool, len(vals))
	for i, v := range vals {
		if strings.TrimSpace(v) == "" {
			errs = append(errs, FieldError{
				Field:   fmt.Sprintf("%s[%d]", field, i),
				Message: "empty entry",
			})
			continue
		}
		if seen[v] {
			errs = append(errs, FieldError{
				Field:   fmt.Sprintf("%s[%d]", field, i),
				Message: fmt.Sprintf("duplicate entry %q", v),
			})
		}
		seen[v] = true
	}
	return errs
}

// Reconcile performs the structural reconcile of a project vh-harness-profile.yml
// (project) against the platform's new default (platformDefault). Either may be
// empty/nil (treated as absent). It never performs I/O.
//
// It returns:
//   - OutcomeApply with Merged when the merge is clean.
//   - OutcomePropose with Proposals when a needs-decision conflict exists (v1:
//     the project selected a profile value the platform's enum no longer offers).
//   - OutcomeNoop when the project instance is byte-identical to the platform
//     default (nothing changed) — a fast-path that avoids a pointless rewrite.
func (HarnessProfile) Reconcile(project, platformDefault []byte) (ReconcileResult, error) {
	pData, pErr := loadHarnessProfile(project)
	dData, dErr := loadHarnessProfile(platformDefault)

	// Malformed inputs are validator territory, not reconcile territory. Surface
	// them as an error rather than silently inventing a merge.
	if pErr != nil {
		return ReconcileResult{}, fmt.Errorf("reconcile harness-profile: project instance invalid: %w", pErr)
	}
	if dErr != nil {
		return ReconcileResult{}, fmt.Errorf("reconcile harness-profile: platform default invalid: %w", dErr)
	}

	// Fast path: byte-identical (or both absent) -> nothing to do.
	if bytesEqualish(project, platformDefault) {
		return ReconcileResult{Outcome: OutcomeNoop, Skipped: []string{"project instance == platform default; nothing to merge"}}, nil
	}

	var applied []string
	var proposals []Proposal

	// --- profile (project-selectable scalar) ---
	mergedProfile, profApplied, profProposal := reconcileProfileScalar(pData.Profile, dData.Profile)
	if profApplied != "" {
		applied = append(applied, profApplied)
	}
	if profProposal != nil {
		proposals = append(proposals, *profProposal)
	}

	// --- append-only arrays ---
	mergedModules, modNote := reconcileAppendOnly("modules", pData.Modules, dData.Modules)
	applied = append(applied, modNote)
	mergedOverlays, ovNote := reconcileAppendOnly("overlays", pData.Overlays, dData.Overlays)
	applied = append(applied, ovNote)
	mergedPacks, pkNote := reconcileAppendOnly("policy_packs", pData.PolicyPacks, dData.PolicyPacks)
	applied = append(applied, pkNote)
	// capabilities is append-only like the other string arrays: the project's
	// selections are unioned with the platform default so a reconcile can never
	// silently drop a declared capability. This is NOT preset expansion (Phase
	// 2+); it only preserves the selection list through the merge.
	mergedCapabilities, capNote := reconcileAppendOnly("capabilities", pData.Capabilities, dData.Capabilities)
	applied = append(applied, capNote)

	// --- features (keyed map, project-wins per key) ---
	mergedFeatures, featNote := reconcileFeaturesMap(pData.Features, dData.Features)
	applied = append(applied, featNote)

	// A needs-decision conflict blocks the apply: do not write a partial file.
	if len(proposals) > 0 {
		return ReconcileResult{
			Outcome:   OutcomePropose,
			Proposals: proposals,
			Skipped:   []string{"apply blocked by needs-decision proposal(s); project instance left untouched"},
		}, nil
	}

	merged := harnessProfileData{
		Profile:      mergedProfile,
		Modules:      mergedModules,
		Features:     mergedFeatures,
		Overlays:     mergedOverlays,
		PolicyPacks:  mergedPacks,
		Capabilities: mergedCapabilities,
	}
	out, err := marshalHarnessProfile(merged)
	if err != nil {
		return ReconcileResult{}, err
	}
	return ReconcileResult{Outcome: OutcomeApply, Merged: out, Applied: applied}, nil
}

// reconcileProfileScalar merges the profile selection. The project's selection
// wins when it is a valid enum member; the platform default is used when the
// project left it blank. If the project selected a value the enum no longer
// offers, a Proposal (enum_removed) is returned and the apply is blocked.
func reconcileProfileScalar(projectVal, defaultVal string) (merged string, appliedNote string, proposal *Proposal) {
	switch {
	case projectVal == "" && defaultVal == "":
		return "", "", nil
	case projectVal == "":
		return defaultVal, fmt.Sprintf("profile: seeded platform default %q", defaultVal), nil
	case profileEnum[projectVal]:
		// Project selection wins (within envelope).
		if projectVal == defaultVal {
			return projectVal, "", nil
		}
		return projectVal, fmt.Sprintf("profile: retained project selection %q (platform default %q)", projectVal, defaultVal), nil
	default:
		// Project value is outside the current enum: platform removed it (or it
		// was never valid). This is a genuine needs-decision.
		return defaultVal, "", &Proposal{
			Field:         "profile",
			Kind:          "enum_removed",
			PlatformValue: defaultVal,
			ProjectValue:  projectVal,
			Envelope:      "enum: " + profileEnumList,
			Hint:          fmt.Sprintf("Profile %q is no longer offered. Choose a current profile or confirm the platform default %q.", projectVal, defaultVal),
		}
	}
}

// reconcileAppendOnly merges two append-only string arrays into a sorted, deduped
// union. It returns the merged slice and a human-readable note describing the
// merge (additions only; never a removal).
func reconcileAppendOnly(field string, projectVals, defaultVals []string) ([]string, string) {
	seen := make(map[string]bool)
	merged := make([]string, 0)
	add := func(vals []string) {
		for _, v := range vals {
			if v == "" || seen[v] {
				continue
			}
			seen[v] = true
			merged = append(merged, v)
		}
	}
	add(defaultVals)
	add(projectVals)
	sort.Strings(merged)

	var added []string
	for _, v := range merged {
		if !contains(defaultVals, v) {
			added = append(added, v)
		}
	}
	var platformAdded []string
	for _, v := range merged {
		if !contains(projectVals, v) {
			platformAdded = append(platformAdded, v)
		}
	}
	switch {
	case len(added) == 0 && len(platformAdded) == 0:
		return merged, fmt.Sprintf("%s: unchanged (%d entries)", field, len(merged))
	case len(added) > 0 && len(platformAdded) > 0:
		return merged, fmt.Sprintf("%s: merged platform +%s and project +%s (%d entries)", field, quoteList(platformAdded), quoteList(added), len(merged))
	case len(added) > 0:
		return merged, fmt.Sprintf("%s: added project +%s (%d entries)", field, quoteList(added), len(merged))
	default:
		return merged, fmt.Sprintf("%s: merged platform +%s (%d entries)", field, quoteList(platformAdded), len(merged))
	}
}

// reconcileFeaturesMap merges the features map. Platform defaults form the base;
// project overrides apply per key (project-wins). New platform keys are added.
func reconcileFeaturesMap(projectFeat, defaultFeat map[string]bool) (map[string]bool, string) {
	merged := make(map[string]bool)
	for k, v := range defaultFeat {
		merged[k] = v
	}
	var projectOverrides, platformNew []string
	for k, v := range projectFeat {
		dVal, hadDefault := defaultFeat[k]
		switch {
		case !hadDefault:
			merged[k] = v
			projectOverrides = append(projectOverrides, k)
		case v != dVal:
			merged[k] = v // project-wins within envelope (bool)
			projectOverrides = append(projectOverrides, k)
		}
	}
	for k := range defaultFeat {
		if _, hadProject := projectFeat[k]; !hadProject {
			platformNew = append(platformNew, k)
		}
	}
	sort.Strings(projectOverrides)
	sort.Strings(platformNew)
	switch {
	case len(projectOverrides) == 0 && len(platformNew) == 0:
		return merged, "features: unchanged"
	case len(projectOverrides) > 0 && len(platformNew) > 0:
		return merged, fmt.Sprintf("features: merged platform +%s and project overrides %s", quoteList(platformNew), quoteList(projectOverrides))
	case len(projectOverrides) > 0:
		return merged, fmt.Sprintf("features: applied project overrides %s", quoteList(projectOverrides))
	default:
		return merged, fmt.Sprintf("features: added platform defaults %s", quoteList(platformNew))
	}
}

// loadHarnessProfile parses and structurally sanity-checks a vh-harness-profile.yml
// blob. It does NOT run full Validate (that's doctor's job); it only guarantees
// the shape is loadable for reconcile. An empty/nil blob yields a zero value.
func loadHarnessProfile(raw []byte) (harnessProfileData, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return harnessProfileData{}, nil
	}
	var d harnessProfileData
	if err := yaml.Unmarshal(raw, &d); err != nil {
		return harnessProfileData{}, err
	}
	return d, nil
}

// marshalHarnessProfile serializes the merged data deterministically.
func marshalHarnessProfile(d harnessProfileData) ([]byte, error) {
	// Normalize arrays so identical merges produce identical bytes.
	d.Modules = sortedDedupStrings(d.Modules)
	d.Overlays = sortedDedupStrings(d.Overlays)
	d.PolicyPacks = sortedDedupStrings(d.PolicyPacks)
	d.Capabilities = sortedDedupStrings(d.Capabilities)
	out, err := yaml.Marshal(d)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// bytesEqualish reports whether two byte slices are equal after trimming
// surrounding whitespace. Used for the noop fast-path.
func bytesEqualish(a, b []byte) bool {
	return strings.TrimSpace(string(a)) == strings.TrimSpace(string(b))
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func quoteList(vals []string) string {
	if len(vals) == 0 {
		return "[]"
	}
	out := make([]string, len(vals))
	for i, v := range vals {
		out[i] = fmt.Sprintf("%q", v)
	}
	return "[" + strings.Join(out, ",") + "]"
}

func sortedDedupStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
