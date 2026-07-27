// Package complexity owns the complexity signal contract shared by both
// projections (the Go repo-snapshot scanner and the Node event-time hint) and
// the policy/threshold resolution that drives them.
//
// Design contract (staged advisory hybrid): complexity signals INFORM; they
// never gate. A Signal carries a nominated metric (file_loc observed >
// configured threshold) plus optional boundary diagnostics, but it has NO
// transition authority. Nothing in this package may turn a threshold breach
// into a FAIL or authorize a state transition.
//
// The shared contract is the language-neutral logical result both projections
// produce BEFORE rendering projection-specific messages. The Node event-time
// projection and the Go snapshot projection materialize the SAME Signal shape
// from the SAME input vectors; a cross-language parity fixture (JSON) verifies
// they agree.
package complexity

// Projection names the context in which a signal is computed. The two contexts
// have DISTINCT thresholds and exclusion rules: the event-time (post-edit)
// projection fires on an edited file against the event threshold; the snapshot
// (repo-snapshot) projection enumerates the whole repo against the snapshot
// threshold with additional suffix exclusions.
type Projection string

const (
	// ProjectionPostEdit is the event-time projection (an editor just touched
	// the file). Threshold: policy defaults.event_file_lines (350).
	ProjectionPostEdit Projection = "post_edit"

	// ProjectionRepoSnapshot is the repo-snapshot projection (doctor enumerates
	// the whole repo). Threshold: policy defaults.snapshot_file_lines (500).
	// Excludes *_test.go and root .opencode/** (rendered output).
	ProjectionRepoSnapshot Projection = "repo_snapshot"
)

// MetricKind names the complexity metric. v1 ships file_loc only; function_loc
// is reserved for where a real parser exists (do NOT regex-approximate).
type MetricKind string

const (
	MetricFileLoc     MetricKind = "file_loc"
	MetricFunctionLoc MetricKind = "function_loc"
)

// Signal is the shared signal contract. Both projections produce this logical
// result before rendering projection-specific advisory messages. It has NO
// transition authority.
type Signal struct {
	Path               string              `yaml:"path"`
	Projection         Projection          `yaml:"projection"`
	Language           string              `yaml:"language"`
	Metric             Metric              `yaml:"metric"`
	BoundaryIndicators []BoundaryIndicator `yaml:"boundary_indicators,omitempty"`
}

// Metric carries the nomination decision for one file.
type Metric struct {
	Kind      MetricKind `yaml:"kind"`
	Observed  int        `yaml:"observed"`
	Threshold int        `yaml:"threshold"`
	// Nominated is true when observed > threshold (strict; equality does NOT
	// nominate).
	Nominated bool `yaml:"nominated"`
}

// BoundaryIndicator is an optional diagnostic reported SEPARATELY from the
// nomination metric. It MUST NOT be folded into a scalar "complexity score" and
// MUST NOT alter the nomination status. v1 reports top_level_symbol_count as
// not_collected (the snapshot scanner counts lines only; no parser).
type BoundaryIndicator struct {
	Kind     string `yaml:"kind"`
	Value    any    `yaml:"value"`
	Evidence string `yaml:"evidence"`
}

// defaultSupportedExtensions is the v1 supported-extension set, mirrored from
// the platform-seeded policy per_language keys. The policy loader validates
// consistency but this is the authoritative fallback when a key has no entry.
var defaultSupportedExtensions = map[string]bool{
	".go":  true,
	".cjs": true,
	".cts": true,
	".js":  true,
	".jsx": true,
	".mjs": true,
	".mts": true,
	".py":  true,
	".ts":  true,
	".tsx": true,
}
