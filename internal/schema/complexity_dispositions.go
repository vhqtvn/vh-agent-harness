package schema

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ComplexityDispositions is the complexity disposition manifest schema
// (.opencode/repo-configs/complexity-dispositions.yml). Ownership class:
// project_owned (platform seeds a blank scaffold once, then preserves project
// edits forever).
//
// The manifest records, per logical artifact that breached a complexity
// threshold, whether the artifact is ACCEPTED as cohesive (with rationale) or
// DEFERRED for a split (with a link to the follow-up work). The platform never
// writes dispositions itself; it only VALIDATEs that a recorded manifest is
// structurally complete and internally consistent.
//
// Validation is structural completeness + internal consistency ONLY (proves
// PRESENCE of required fields and their mutual agreement, never the TRUTH of an
// accept rationale). This is the same honesty ceiling as the behavioral-closure
// token: a structurally complete disposition is not a proven judgment. The
// vocabulary here (accept-as-cohesive | split-defer) is SEPARATE from the
// review BLOCK/DEFER/DROP vocabulary (precedent: release-defer-dispositions).
//
// WARN-only invariant: nothing in this schema or its consumers may turn a
// complexity threshold breach into a FAIL or authorize a transition. The
// manifest is advisory evidence, not authority.
type ComplexityDispositions struct{}

// complexityDispositionsData is the typed projection of a
// complexity-dispositions.yml. version must equal 1; dispositions is the
// (possibly empty) list of recorded judgments.
type complexityDispositionsData struct {
	Version      int                     `yaml:"version"`
	Dispositions []complexityDisposition `yaml:"dispositions"`
}

type complexityDisposition struct {
	DispositionID   string            `yaml:"disposition_id"`
	LogicalArtifact string            `yaml:"logical_artifact"`
	Paths           []string          `yaml:"paths"`
	Decision        string            `yaml:"decision"`
	Signal          complexityDSignal `yaml:"signal"`
	AcceptRationale *acceptRationale  `yaml:"accept_rationale,omitempty"`
	DeferLink       *deferLink        `yaml:"defer_link,omitempty"`
}

type complexityDSignal struct {
	MetricKind   string           `yaml:"metric_kind"`
	Observations []complexityDObs `yaml:"observations"`
}

type complexityDObs struct {
	Path string `yaml:"path"`
}

// acceptRationale is the seven-field cohesion record for accept-as-cohesive.
type acceptRationale struct {
	Responsibility     string `yaml:"responsibility"`
	OrganizingSpine    string `yaml:"organizing_spine"`
	CohesionEvidence   string `yaml:"cohesion_evidence"`
	InternalBoundaries string `yaml:"internal_boundaries"`
	ExtractionTradeoff string `yaml:"extraction_tradeoff"`
	VerificationSeams  string `yaml:"verification_seams"`
	ReconsiderTrigger  string `yaml:"reconsider_trigger"`
}

// deferLink points at the follow-up work for a split-defer.
type deferLink struct {
	TaskID         string `yaml:"task_id"`
	TargetBoundary string `yaml:"target_boundary"`
	Trigger        string `yaml:"trigger"`
	ValidationPlan string `yaml:"validation_plan"`
}

// acceptRationaleFields is the exhaustive set of accept-rationale fields (each
// must be non-empty for an accept-as-cohesive disposition).
var acceptRationaleFields = []string{
	"responsibility", "organizing_spine", "cohesion_evidence",
	"internal_boundaries", "extraction_tradeoff", "verification_seams",
	"reconsider_trigger",
}

// validDecision is the closed vocabulary for the decision field.
var validDecision = map[string]bool{
	"accept-as-cohesive": true,
	"split-defer":        true,
}

// validMetricKind is the closed vocabulary for signal.metric_kind.
var validMetricKind = map[string]bool{
	"file_loc":     true,
	"function_loc": true,
}

// Validate reports every structural problem in a complexity-dispositions.yml
// instance. It is total, performs no I/O, and proves structural completeness +
// internal consistency ONLY (not truth). An empty dispositions list is valid
// (the seeded-blank state).
func (ComplexityDispositions) Validate(raw []byte) []FieldError {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return []FieldError{{Field: "<root>", Message: "file is empty"}}
	}
	// Envelope: only version + dispositions at top level.
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
		if k != "version" && k != "dispositions" {
			errs = append(errs, FieldError{
				Field:   k,
				Message: fmt.Sprintf("unknown top-level key %q; allowed: version, dispositions", k),
			})
		}
	}

	var d complexityDispositionsData
	if err := yaml.Unmarshal(raw, &d); err != nil {
		errs = append(errs, FieldError{Field: "<root>", Message: fmt.Sprintf("shape mismatch: %v", err)})
		return errs
	}

	if d.Version != 1 {
		errs = append(errs, FieldError{
			Field:   "version",
			Message: fmt.Sprintf("must equal 1 (got %d)", d.Version),
		})
	}

	// disposition_id must be unique across the manifest.
	seenIDs := make(map[string]bool)
	for i, disp := range d.Dispositions {
		prefix := fmt.Sprintf("dispositions[%d]", i)
		if strings.TrimSpace(disp.DispositionID) == "" {
			errs = append(errs, FieldError{
				Field:   prefix + ".disposition_id",
				Message: "is required (non-empty)",
			})
		} else if seenIDs[disp.DispositionID] {
			errs = append(errs, FieldError{
				Field:   prefix + ".disposition_id",
				Message: fmt.Sprintf("duplicate disposition_id %q", disp.DispositionID),
			})
		}
		seenIDs[disp.DispositionID] = true

		if strings.TrimSpace(disp.LogicalArtifact) == "" {
			errs = append(errs, FieldError{
				Field:   prefix + ".logical_artifact",
				Message: "is required (non-empty)",
			})
		}

		// paths: required non-empty, normalized (repo-relative, forward slash),
		// unique.
		pathErrs := validateDispositionPaths(prefix+".paths", disp.Paths)
		errs = append(errs, pathErrs...)

		// decision: closed vocabulary.
		if !validDecision[disp.Decision] {
			errs = append(errs, FieldError{
				Field:   prefix + ".decision",
				Message: fmt.Sprintf("invalid decision %q; enum: accept-as-cohesive | split-defer", disp.Decision),
			})
		}

		// signal.metric_kind: closed vocabulary.
		if !validMetricKind[disp.Signal.MetricKind] {
			errs = append(errs, FieldError{
				Field:   prefix + ".signal.metric_kind",
				Message: fmt.Sprintf("invalid metric_kind %q; enum: file_loc | function_loc", disp.Signal.MetricKind),
			})
		}

		// signal.observations[].path must occur in paths; observation paths
		// unique.
		obsErrs := validateDispositionObservations(prefix+".signal.observations", disp.Signal.Observations, disp.Paths)
		errs = append(errs, obsErrs...)

		// decision-specific structural requirements.
		switch disp.Decision {
		case "accept-as-cohesive":
			if disp.DeferLink != nil {
				errs = append(errs, FieldError{
					Field:   prefix + ".defer_link",
					Message: "must be null/absent for accept-as-cohesive",
				})
			}
			if disp.AcceptRationale == nil {
				errs = append(errs, FieldError{
					Field:   prefix + ".accept_rationale",
					Message: "is required for accept-as-cohesive",
				})
			} else {
				errs = append(errs, validateAcceptRationale(prefix+".accept_rationale", *disp.AcceptRationale)...)
			}
		case "split-defer":
			if disp.AcceptRationale != nil {
				errs = append(errs, FieldError{
					Field:   prefix + ".accept_rationale",
					Message: "must be null/absent for split-defer",
				})
			}
			if disp.DeferLink == nil {
				errs = append(errs, FieldError{
					Field:   prefix + ".defer_link",
					Message: "is required for split-defer",
				})
			} else {
				errs = append(errs, validateDeferLink(prefix+".defer_link", *disp.DeferLink)...)
			}
		}
	}
	return errs
}

// validateDispositionPaths checks the paths array: non-empty, each entry
// normalized (repo-relative forward-slash, not absolute, no backslash), and
// unique.
func validateDispositionPaths(field string, paths []string) []FieldError {
	var errs []FieldError
	if len(paths) == 0 {
		errs = append(errs, FieldError{Field: field, Message: "is required (non-empty array)"})
		return errs
	}
	seen := make(map[string]bool)
	for i, p := range paths {
		elem := fmt.Sprintf("%s[%d]", field, i)
		if strings.TrimSpace(p) == "" {
			errs = append(errs, FieldError{Field: elem, Message: "is empty"})
			continue
		}
		// Normalized: forward-slash, not absolute, no leading drive/backslash.
		slash := filepath.ToSlash(p)
		if slash != p {
			errs = append(errs, FieldError{Field: elem, Message: fmt.Sprintf("must be normalized to forward slashes (got %q)", p)})
		}
		if strings.HasPrefix(slash, "/") {
			errs = append(errs, FieldError{Field: elem, Message: fmt.Sprintf("must be repo-relative (got absolute %q)", p)})
		}
		if seen[slash] {
			errs = append(errs, FieldError{Field: elem, Message: fmt.Sprintf("duplicate path %q", slash)})
		}
		seen[slash] = true
	}
	return errs
}

func validateDispositionObservations(field string, obs []complexityDObs, paths []string) []FieldError {
	var errs []FieldError
	pathSet := make(map[string]bool, len(paths))
	for _, p := range paths {
		pathSet[filepath.ToSlash(p)] = true
	}
	seenObs := make(map[string]bool)
	for i, o := range obs {
		elem := fmt.Sprintf("%s[%d].path", field, i)
		norm := filepath.ToSlash(o.Path)
		if strings.TrimSpace(o.Path) == "" {
			errs = append(errs, FieldError{Field: elem, Message: "is empty"})
			continue
		}
		if !pathSet[norm] {
			errs = append(errs, FieldError{
				Field:   elem,
				Message: fmt.Sprintf("observation path %q does not occur in paths", o.Path),
			})
		}
		if seenObs[norm] {
			errs = append(errs, FieldError{
				Field:   elem,
				Message: fmt.Sprintf("duplicate observation path %q", norm),
			})
		}
		seenObs[norm] = true
	}
	return errs
}

func validateAcceptRationale(field string, r acceptRationale) []FieldError {
	var errs []FieldError
	vals := map[string]string{
		"responsibility":      r.Responsibility,
		"organizing_spine":    r.OrganizingSpine,
		"cohesion_evidence":   r.CohesionEvidence,
		"internal_boundaries": r.InternalBoundaries,
		"extraction_tradeoff": r.ExtractionTradeoff,
		"verification_seams":  r.VerificationSeams,
		"reconsider_trigger":  r.ReconsiderTrigger,
	}
	// Report in the canonical field order for stable output.
	for _, name := range acceptRationaleFields {
		if strings.TrimSpace(vals[name]) == "" {
			errs = append(errs, FieldError{
				Field:   field + "." + name,
				Message: "is required (non-empty)",
			})
		}
	}
	return errs
}

func validateDeferLink(field string, l deferLink) []FieldError {
	var errs []FieldError
	vals := map[string]string{
		"task_id":         l.TaskID,
		"target_boundary": l.TargetBoundary,
		"trigger":         l.Trigger,
		"validation_plan": l.ValidationPlan,
	}
	order := []string{"task_id", "target_boundary", "trigger", "validation_plan"}
	for _, name := range order {
		if strings.TrimSpace(vals[name]) == "" {
			errs = append(errs, FieldError{
				Field:   field + "." + name,
				Message: "is required (non-empty)",
			})
		}
	}
	return errs
}

// Reconcile is seed-only for complexity-dispositions (project_owned). On update
// it NEVER overwrites the project's instance. It returns OutcomeNoop. (First-
// install seeding is handled by the substrate's project_owned path, which copies
// the platform default only when the project file is absent.)
func (ComplexityDispositions) Reconcile(project, platformDefault []byte) (ReconcileResult, error) {
	if len(strings.TrimSpace(string(project))) == 0 {
		return ReconcileResult{
			Outcome: OutcomeApply,
			Merged:  platformDefault,
			Applied: []string{"complexity-dispositions: seed-only (project_owned); substrate seeds when project instance absent"},
		}, nil
	}
	return ReconcileResult{
		Outcome: OutcomeNoop,
		Skipped: []string{"complexity-dispositions: project_owned; project instance preserved (never clobbered on update)"},
	}, nil
}
