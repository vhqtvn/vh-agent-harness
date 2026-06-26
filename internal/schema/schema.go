// Package schema owns the schema registry for the platform_armed (and other
// schema'd) file types introduced by the rebuild spec (STEP 2).
//
// Authority split (see the rebuild spec and the config-authority model):
// the platform owns each armed file's SCHEMA + DEFAULT; the project may edit an
// instance WITHIN the schema envelope. doctor/preflight VALIDATE an instance
// against its schema. update does a schema-aware STRUCTURAL reconcile for
// platform_armed files (not a textual 3-way merge).
//
// This package is deliberately independent of the ownership/lineage/substrate
// packages: it knows file shapes, not update safety or render lineage. The
// substrate consults it to (a) validate and (b) reconcile platform_armed files
// during a render/apply.
//
// Slice 1 scope:
//   - vh-harness-profile.yml (S3): full Validate + structural Reconcile.
//   - run-shape.yml (S4), forbidden-patterns.project.js, repo-recon data:
//     Validate + a seed-only / regenerate reconcile skeleton. Full corpus wiring
//     is Slice 5.
package schema

// Type names the schema'd file family. It is the registry key.
type Type string

const (
	// TypeHarnessProfile is the S3 feature-surface selection file
	// (vh-harness-profile.yml). Ownership class: platform_armed -> full reconcile.
	TypeHarnessProfile Type = "harness-profile"

	// TypeRunShape is the S4 runtime execution shape file
	// (.vh-agent-harness/run-shape.yml). Ownership class: project_owned
	// (seed once, never clobber) -> reconcile is seed-only / noop on update;
	// doctor still validates the shape.
	TypeRunShape Type = "run-shape"

	// TypeForbiddenPatternsProject is the project deny-rule payload
	// (forbidden-patterns.project.js). Ownership class: project_owned ->
	// seed-only reconcile; doctor validates the deny-rule array shape.
	TypeForbiddenPatternsProject Type = "forbidden-patterns.project"

	// TypeRepoRecon is the repo-recon structural data file (entrypoints,
	// hotspots, packages, tests). Ownership class: external_generated ->
	// regenerate; doctor validates the shape against the contract.
	TypeRepoRecon Type = "repo-recon"
)

// FieldError is one structural validation problem at a specific schema path. It
// is the unit doctor/preflight report. Field is a dotted path
// (e.g. "profile", "features", "modules[2]"); Message is human-readable.
type FieldError struct {
	Field   string
	Message string
}

// Validator validates a file instance against its schema. It returns the list of
// structural field errors; an empty (non-nil) slice means the instance is
// schema-conformant. A validator MUST be total: it reports every problem it can
// rather than aborting on the first, so doctor can surface the full picture in
// one pass. It MUST NOT perform I/O.
type Validator interface {
	Validate(raw []byte) []FieldError
}

// ReconcileOutcome classifies the result of a structural reconcile.
type ReconcileOutcome string

const (
	// OutcomeApply means the reconcile produced a clean merged instance ready to
	// be written atomically. Merged is populated.
	OutcomeApply ReconcileOutcome = "apply"

	// OutcomePropose means the reconcile hit a needs-decision conflict: the
	// platform and project disagree on a field in a way the schema cannot
	// auto-resolve. Proposals is populated; Merged is nil. The substrate MUST
	// emit the proposals (a structured proposal document naming each schema
	// field) and NOT write the file or drop conflict markers.
	OutcomePropose ReconcileOutcome = "propose"

	// OutcomeNoop means there is nothing to do: the project instance is already
	// up to date with the platform default (or the file is seed-only/project_owned
	// and must not be touched on update). Merged and Proposals are empty.
	OutcomeNoop ReconcileOutcome = "noop"
)

// Proposal names one needs-decision conflict precisely. It is the structured
// replacement for a textual conflict marker: instead of "<<<<<<<" the operator
// gets the schema field (dotted path), what the platform now requires, what the
// project currently has, the envelope that defines validity, and a hint.
type Proposal struct {
	// Field is the dotted schema path at fault (e.g. "profile",
	// "features.backlog").
	Field string `yaml:"field"`
	// Kind classifies the conflict (e.g. "enum_removed", "forced_scalar_conflict",
	// "type_changed"). Stable machine label.
	Kind string `yaml:"kind"`
	// PlatformValue is what the platform's current schema/default requires at
	// Field. May be nil when the platform removed the value entirely.
	PlatformValue any `yaml:"platform_value,omitempty"`
	// ProjectValue is what the project's current instance has at Field.
	ProjectValue any `yaml:"project_value"`
	// Envelope describes the valid space at Field (e.g. "enum: minimal |
	// coordination | supervised" or "bool"). Human-readable.
	Envelope string `yaml:"envelope,omitempty"`
	// Hint is short operator guidance on how to resolve.
	Hint string `yaml:"hint,omitempty"`
}

// ReconcileResult is the outcome of a structural reconcile. Exactly one of
// (Merged on Apply) / (Proposals on Propose) is meaningful; on Noop both are
// empty. Applied/Skipped are human-readable change notes for the apply path so
// the operator (and tests) can see what the reconcile did without diffing bytes.
type ReconcileResult struct {
	Outcome   ReconcileOutcome `yaml:"outcome"`
	Merged    []byte           `yaml:"-"`                   // valid when Outcome==Apply
	Proposals []Proposal       `yaml:"proposals,omitempty"` // valid when Outcome==Propose
	Applied   []string         `yaml:"applied,omitempty"`   // what the reconcile changed/merged
	Skipped   []string         `yaml:"skipped,omitempty"`   // what was left untouched and why
}

// Reconciler performs a schema-aware structural reconcile of a project instance
// against the platform's new default. It MUST be deterministic, total (it either
// produces a clean merged instance or a complete proposal list, never a partial
// write), and side-effect free (no I/O). The substrate owns the atomic apply.
type Reconciler interface {
	Reconcile(project, platformDefault []byte) (ReconcileResult, error)
}

// Schema binds a file family's Type to its Validator and Reconciler.
type Schema struct {
	Type       Type
	Validator  Validator
	Reconciler Reconciler
}
