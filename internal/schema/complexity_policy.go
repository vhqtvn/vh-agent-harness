package schema

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ComplexityPolicy is the complexity-signal configuration schema
// (.vh-agent-harness/complexity-policy.yml). Ownership class: platform_armed.
//
// The platform owns the schema (the threshold model, the per-language map
// envelope, the exclusion grammar) and the v1 default. The project edits within
// the envelope: it may override per-language thresholds, toggle enabled, add
// exclusion paths, and raise/lower doctor.max_candidates. Reconcile is a
// field-level structural merge (project-wins for scalars; union for the
// per-language map and the append-only exclusion arrays; version is platform
// authoritative). doctor VALIDATEs the instance; the complexity scanner LOADs it
// to resolve per-file thresholds.
//
// Design contract (staged advisory hybrid): complexity signals INFORM; they
// never gate. A threshold breach is a WARN/advisory, never a FAIL. Nothing in
// this schema or its consumers may authorize a transition or increment a
// failure/problem count.
type ComplexityPolicy struct{}

// complexityPolicyAllowedTopLevel is the exhaustive set of top-level keys a
// complexity-policy.yml may carry. Anything else is an envelope violation.
var complexityPolicyAllowedTopLevel = map[string]bool{
	"version":      true,
	"enabled":      true,
	"defaults":     true,
	"per_language": true,
	"exclude":      true,
	"doctor":       true,
	"recurrence":   true,
}

// complexityPolicyData is the typed projection of a complexity-policy.yml.
type complexityPolicyData struct {
	Version     int                               `yaml:"version"`
	Enabled     bool                              `yaml:"enabled"`
	Defaults    complexityPolicyDefaults          `yaml:"defaults"`
	PerLanguage map[string]complexityLangOverride `yaml:"per_language"`
	Exclude     complexityPolicyExclude           `yaml:"exclude"`
	Doctor      complexityPolicyDoctor            `yaml:"doctor"`
	Recurrence  complexityPolicyRecurrence        `yaml:"recurrence"`
}

type complexityPolicyDefaults struct {
	EventFileLines    int `yaml:"event_file_lines"`
	SnapshotFileLines int `yaml:"snapshot_file_lines"`
}

// complexityLangOverride is a per-language threshold override. Pointers
// distinguish "unset (inherit default)" from "set". A nil field inherits the
// matching defaults value.
type complexityLangOverride struct {
	EventFileLines    *int `yaml:"event_file_lines,omitempty"`
	SnapshotFileLines *int `yaml:"snapshot_file_lines,omitempty"`
}

type complexityPolicyExclude struct {
	EventPaths       []string `yaml:"event_paths"`
	SnapshotPaths    []string `yaml:"snapshot_paths"`
	SnapshotSuffixes []string `yaml:"snapshot_suffixes"`
}

type complexityPolicyDoctor struct {
	MaxCandidates int `yaml:"max_candidates"`
}

type complexityPolicyRecurrence struct {
	Enabled bool `yaml:"enabled"`
}

// Validate reports every structural problem in a complexity-policy.yml instance.
// It is total (reports all problems it can) and performs no I/O. An empty result
// is conformant.
func (ComplexityPolicy) Validate(raw []byte) []FieldError {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return []FieldError{{Field: "<root>", Message: "file is empty"}}
	}
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
		if !complexityPolicyAllowedTopLevel[k] {
			errs = append(errs, FieldError{
				Field:   k,
				Message: fmt.Sprintf("unknown top-level key %q; allowed: version, enabled, defaults, per_language, exclude, doctor, recurrence", k),
			})
		}
	}

	var d complexityPolicyData
	if err := yaml.Unmarshal(raw, &d); err != nil {
		errs = append(errs, FieldError{Field: "<root>", Message: fmt.Sprintf("shape mismatch: %v", err)})
		return errs
	}

	// version: must equal 1 (the v1 contract).
	if d.Version != 1 {
		errs = append(errs, FieldError{
			Field:   "version",
			Message: fmt.Sprintf("must equal 1 (got %d)", d.Version),
		})
	}

	// defaults: both thresholds must be positive integers.
	if d.Defaults.EventFileLines <= 0 {
		errs = append(errs, FieldError{
			Field:   "defaults.event_file_lines",
			Message: fmt.Sprintf("must be a positive integer (got %d)", d.Defaults.EventFileLines),
		})
	}
	if d.Defaults.SnapshotFileLines <= 0 {
		errs = append(errs, FieldError{
			Field:   "defaults.snapshot_file_lines",
			Message: fmt.Sprintf("must be a positive integer (got %d)", d.Defaults.SnapshotFileLines),
		})
	}

	// per_language: each value, when its override fields are set, must be positive.
	langKeys := make([]string, 0, len(d.PerLanguage))
	for k := range d.PerLanguage {
		langKeys = append(langKeys, k)
	}
	sort.Strings(langKeys)
	for _, k := range langKeys {
		o := d.PerLanguage[k]
		if o.EventFileLines != nil && *o.EventFileLines <= 0 {
			errs = append(errs, FieldError{
				Field:   fmt.Sprintf("per_language[%q].event_file_lines", k),
				Message: fmt.Sprintf("must be a positive integer (got %d)", *o.EventFileLines),
			})
		}
		if o.SnapshotFileLines != nil && *o.SnapshotFileLines <= 0 {
			errs = append(errs, FieldError{
				Field:   fmt.Sprintf("per_language[%q].snapshot_file_lines", k),
				Message: fmt.Sprintf("must be a positive integer (got %d)", *o.SnapshotFileLines),
			})
		}
	}

	// exclude: arrays must be string arrays (yaml enforced); report empty entries.
	errs = appendStringArrayErrors(errs, "exclude.event_paths", d.Exclude.EventPaths)
	errs = appendStringArrayErrors(errs, "exclude.snapshot_paths", d.Exclude.SnapshotPaths)
	errs = appendStringArrayErrors(errs, "exclude.snapshot_suffixes", d.Exclude.SnapshotSuffixes)

	// doctor.max_candidates must be positive.
	if d.Doctor.MaxCandidates <= 0 {
		errs = append(errs, FieldError{
			Field:   "doctor.max_candidates",
			Message: fmt.Sprintf("must be a positive integer (got %d)", d.Doctor.MaxCandidates),
		})
	}

	return errs
}

// Reconcile performs the structural reconcile of a project complexity-policy.yml
// (project) against the platform's new default (platformDefault). Either may be
// empty/nil. It never performs I/O.
//
// Merge policy (v1):
//   - version: platform authoritative (the contract version is not
//     project-selectable; validator rejects anything != 1).
//   - enabled: project-wins.
//   - defaults: per-field project-wins (each of event_file_lines,
//     snapshot_file_lines).
//   - per_language: union by key; within a key, the project's override fields
//     win where set, the platform's are inherited otherwise; new platform keys
//     are added.
//   - exclude: append-only arrays, union-dedup-sorted (project additions
//     retained; platform additions merged).
//   - doctor.max_candidates: project-wins.
//   - recurrence.enabled: project-wins.
func (ComplexityPolicy) Reconcile(project, platformDefault []byte) (ReconcileResult, error) {
	pData, pErr := loadComplexityPolicy(project)
	dData, dErr := loadComplexityPolicy(platformDefault)
	if pErr != nil {
		return ReconcileResult{}, fmt.Errorf("reconcile complexity-policy: project instance invalid: %w", pErr)
	}
	if dErr != nil {
		return ReconcileResult{}, fmt.Errorf("reconcile complexity-policy: platform default invalid: %w", dErr)
	}
	if bytesEqualish(project, platformDefault) {
		return ReconcileResult{Outcome: OutcomeNoop, Skipped: []string{"project instance == platform default; nothing to merge"}}, nil
	}

	var applied []string

	// version: platform authoritative.
	mergedVersion := dData.Version
	if pData.Version != 0 && pData.Version != dData.Version {
		applied = append(applied, fmt.Sprintf("version: retained platform authoritative %d (project %d ignored as non-contract)", dData.Version, pData.Version))
	}

	// enabled: project-wins.
	mergedEnabled, enNote := reconcileBoolScalar("enabled", pData.Enabled, dData.Enabled)
	if enNote != "" {
		applied = append(applied, enNote)
	}

	// defaults: per-field project-wins.
	mergedDefaults, defNote := reconcileComplexityDefaults(pData.Defaults, dData.Defaults)
	applied = append(applied, defNote)

	// per_language: union by key, project-wins per override field.
	mergedPerLang, plNote := reconcileComplexityPerLanguage(pData.PerLanguage, dData.PerLanguage)
	applied = append(applied, plNote)

	// exclude: append-only arrays.
	mergedEventPaths, evNote := reconcileAppendOnly("exclude.event_paths", pData.Exclude.EventPaths, dData.Exclude.EventPaths)
	applied = append(applied, evNote)
	mergedSnapPaths, spNote := reconcileAppendOnly("exclude.snapshot_paths", pData.Exclude.SnapshotPaths, dData.Exclude.SnapshotPaths)
	applied = append(applied, spNote)
	mergedSnapSuffix, ssNote := reconcileAppendOnly("exclude.snapshot_suffixes", pData.Exclude.SnapshotSuffixes, dData.Exclude.SnapshotSuffixes)
	applied = append(applied, ssNote)

	// doctor.max_candidates: project-wins.
	mergedDoctor, docNote := reconcileComplexityDoctor(pData.Doctor, dData.Doctor)
	applied = append(applied, docNote)

	// recurrence.enabled: project-wins.
	mergedRecurrence, recNote := reconcileBoolScalar("recurrence.enabled", pData.Recurrence.Enabled, dData.Recurrence.Enabled)
	if recNote != "" {
		applied = append(applied, recNote)
	}

	merged := complexityPolicyData{
		Version:     mergedVersion,
		Enabled:     mergedEnabled,
		Defaults:    mergedDefaults,
		PerLanguage: mergedPerLang,
		Exclude: complexityPolicyExclude{
			EventPaths:       mergedEventPaths,
			SnapshotPaths:    mergedSnapPaths,
			SnapshotSuffixes: mergedSnapSuffix,
		},
		Doctor:     mergedDoctor,
		Recurrence: complexityPolicyRecurrence{Enabled: mergedRecurrence},
	}
	out, err := marshalComplexityPolicy(merged)
	if err != nil {
		return ReconcileResult{}, err
	}
	return ReconcileResult{Outcome: OutcomeApply, Merged: out, Applied: applied}, nil
}

// reconcileBoolScalar merges a bool scalar, project-wins, returning the merged
// value and an applied note (empty when unchanged).
func reconcileBoolScalar(field string, projectVal, defaultVal bool) (bool, string) {
	if projectVal == defaultVal {
		return projectVal, ""
	}
	return projectVal, fmt.Sprintf("%s: applied project value %v (platform default %v)", field, projectVal, defaultVal)
}

func reconcileComplexityDefaults(p, d complexityPolicyDefaults) (complexityPolicyDefaults, string) {
	// per-field project-wins; but we cannot distinguish "project unset" from
	// "project set to same" with a plain int after unmarshal. Treat the project
	// values as authoritative whenever the project block parsed (it did); if the
	// project left a field at zero (absent), inherit the default.
	merged := complexityPolicyDefaults{
		EventFileLines:    d.EventFileLines,
		SnapshotFileLines: d.SnapshotFileLines,
	}
	var notes []string
	if p.EventFileLines > 0 {
		merged.EventFileLines = p.EventFileLines
		if p.EventFileLines != d.EventFileLines {
			notes = append(notes, fmt.Sprintf("event_file_lines=%d", p.EventFileLines))
		}
	}
	if p.SnapshotFileLines > 0 {
		merged.SnapshotFileLines = p.SnapshotFileLines
		if p.SnapshotFileLines != d.SnapshotFileLines {
			notes = append(notes, fmt.Sprintf("snapshot_file_lines=%d", p.SnapshotFileLines))
		}
	}
	if len(notes) == 0 {
		return merged, "defaults: unchanged"
	}
	return merged, "defaults: applied project " + strings.Join(notes, ", ")
}

func reconcileComplexityPerLanguage(p, d map[string]complexityLangOverride) (map[string]complexityLangOverride, string) {
	merged := make(map[string]complexityLangOverride)
	// Start from platform defaults.
	for k, v := range d {
		merged[k] = cloneComplexityLangOverride(v)
	}
	var projectOverrides, platformNew []string
	for k, pv := range p {
		base, hadDefault := merged[k]
		if !hadDefault {
			// New project key not in platform defaults: still record it (the
			// envelope is open), and note it.
			merged[k] = cloneComplexityLangOverride(pv)
			projectOverrides = append(projectOverrides, k)
			continue
		}
		// Project override fields win where set.
		ov := cloneComplexityLangOverride(base)
		if pv.EventFileLines != nil {
			ov.EventFileLines = pv.EventFileLines
		}
		if pv.SnapshotFileLines != nil {
			ov.SnapshotFileLines = pv.SnapshotFileLines
		}
		merged[k] = ov
		if !langOverrideEqual(ov, base) {
			projectOverrides = append(projectOverrides, k)
		}
	}
	for k := range d {
		if _, hadProject := p[k]; !hadProject {
			platformNew = append(platformNew, k)
		}
	}
	sort.Strings(projectOverrides)
	sort.Strings(platformNew)
	switch {
	case len(projectOverrides) == 0 && len(platformNew) == 0:
		return merged, "per_language: unchanged"
	case len(projectOverrides) > 0 && len(platformNew) > 0:
		return merged, fmt.Sprintf("per_language: merged platform +%s and project overrides %s", quoteList(platformNew), quoteList(projectOverrides))
	case len(projectOverrides) > 0:
		return merged, fmt.Sprintf("per_language: applied project overrides %s", quoteList(projectOverrides))
	default:
		return merged, fmt.Sprintf("per_language: added platform defaults %s", quoteList(platformNew))
	}
}

func reconcileComplexityDoctor(p, d complexityPolicyDoctor) (complexityPolicyDoctor, string) {
	merged := d
	if p.MaxCandidates > 0 {
		merged.MaxCandidates = p.MaxCandidates
		if p.MaxCandidates != d.MaxCandidates {
			return merged, fmt.Sprintf("doctor.max_candidates: applied project value %d (platform default %d)", p.MaxCandidates, d.MaxCandidates)
		}
	}
	return merged, "doctor.max_candidates: unchanged"
}

func cloneComplexityLangOverride(o complexityLangOverride) complexityLangOverride {
	var ev, sp *int
	if o.EventFileLines != nil {
		v := *o.EventFileLines
		ev = &v
	}
	if o.SnapshotFileLines != nil {
		v := *o.SnapshotFileLines
		sp = &v
	}
	return complexityLangOverride{EventFileLines: ev, SnapshotFileLines: sp}
}

// langOverrideEqual reports whether two overrides resolve identically (nil
// pointers treated as equal to each other; nil != a set value).
func langOverrideEqual(a, b complexityLangOverride) bool {
	return intPtrEqual(a.EventFileLines, b.EventFileLines) && intPtrEqual(a.SnapshotFileLines, b.SnapshotFileLines)
}

func intPtrEqual(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func loadComplexityPolicy(raw []byte) (complexityPolicyData, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return complexityPolicyData{}, nil
	}
	var d complexityPolicyData
	if err := yaml.Unmarshal(raw, &d); err != nil {
		return complexityPolicyData{}, err
	}
	return d, nil
}

func marshalComplexityPolicy(d complexityPolicyData) ([]byte, error) {
	// Normalize exclusion arrays so identical merges produce identical bytes.
	d.Exclude.EventPaths = sortedDedupStrings(d.Exclude.EventPaths)
	d.Exclude.SnapshotPaths = sortedDedupStrings(d.Exclude.SnapshotPaths)
	d.Exclude.SnapshotSuffixes = sortedDedupStrings(d.Exclude.SnapshotSuffixes)
	out, err := yaml.Marshal(d)
	if err != nil {
		return nil, err
	}
	return out, nil
}
