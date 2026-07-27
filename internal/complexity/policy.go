package complexity

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Policy is the parsed complexity-policy.yml, projected to the fields the
// signal computation needs. It mirrors the schema in internal/schema but lives
// in this runtime package so the scanner does not depend on the schema
// registry.
type Policy struct {
	Version   int
	Enabled   bool
	Defaults  Defaults
	Languages map[string]LangOverride
	Exclude   ExcludeRules
	Doctor    DoctorConfig
}

type Defaults struct {
	EventFileLines    int `yaml:"event_file_lines"`
	SnapshotFileLines int `yaml:"snapshot_file_lines"`
}

// LangOverride carries a per-language threshold override. A nil pointer means
// "inherit the matching default".
type LangOverride struct {
	EventFileLines    *int `yaml:"event_file_lines,omitempty"`
	SnapshotFileLines *int `yaml:"snapshot_file_lines,omitempty"`
}

type ExcludeRules struct {
	EventPaths       []string `yaml:"event_paths"`
	SnapshotPaths    []string `yaml:"snapshot_paths"`
	SnapshotSuffixes []string `yaml:"snapshot_suffixes"`
}

type DoctorConfig struct {
	MaxCandidates int `yaml:"max_candidates"`
}

// LoadPolicy parses a complexity-policy.yml blob into a Policy. An empty blob
// yields a zero Policy (callers should treat a zero Policy as "complexity
// disabled" rather than crashing).
func LoadPolicy(raw []byte) (Policy, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return Policy{}, nil
	}
	var d struct {
		Version     int                     `yaml:"version"`
		Enabled     bool                    `yaml:"enabled"`
		Defaults    Defaults                `yaml:"defaults"`
		PerLanguage map[string]LangOverride `yaml:"per_language"`
		Exclude     ExcludeRules            `yaml:"exclude"`
		Doctor      DoctorConfig            `yaml:"doctor"`
		Recurrence  struct {
			Enabled bool `yaml:"enabled"`
		} `yaml:"recurrence"`
	}
	if err := yaml.Unmarshal(raw, &d); err != nil {
		return Policy{}, fmt.Errorf("complexity policy: %w", err)
	}
	p := Policy{
		Version:   d.Version,
		Enabled:   d.Enabled,
		Defaults:  d.Defaults,
		Languages: d.PerLanguage,
		Exclude:   d.Exclude,
		Doctor:    d.Doctor,
	}
	if p.Languages == nil {
		p.Languages = map[string]LangOverride{}
	}
	return p, nil
}

// supportsExtension reports whether a language (file extension) is supported by
// the policy. A language is supported when it appears in the policy's
// per_language map OR falls in the v1 default set (the policy seeds all 10, so
// the two agree; this fallback covers a policy that dropped a key).
func (p Policy) supportsExtension(ext string) bool {
	if _, ok := p.Languages[ext]; ok {
		return true
	}
	return defaultSupportedExtensions[ext]
}

// resolveThreshold returns the configured threshold for a language + projection,
// applying per-language overrides then falling back to the matching default.
func (p Policy) resolveThreshold(ext string, proj Projection) int {
	if ov, ok := p.Languages[ext]; ok {
		switch proj {
		case ProjectionPostEdit:
			if ov.EventFileLines != nil {
				return *ov.EventFileLines
			}
		case ProjectionRepoSnapshot:
			if ov.SnapshotFileLines != nil {
				return *ov.SnapshotFileLines
			}
		}
	}
	switch proj {
	case ProjectionPostEdit:
		return p.Defaults.EventFileLines
	case ProjectionRepoSnapshot:
		return p.Defaults.SnapshotFileLines
	}
	return 0
}

// isExcluded reports whether a repo-relative path is excluded for the given
// projection. The snapshot projection additionally excludes files whose base
// name ends with a configured snapshot_suffix (e.g. _test.go).
func (p Policy) isExcluded(relPath string, proj Projection) bool {
	var patterns []string
	switch proj {
	case ProjectionPostEdit:
		patterns = p.Exclude.EventPaths
	case ProjectionRepoSnapshot:
		patterns = p.Exclude.SnapshotPaths
	}
	if matchAnyGlob(relPath, patterns) {
		return true
	}
	if proj == ProjectionRepoSnapshot {
		base := filepath.Base(relPath)
		for _, suf := range p.Exclude.SnapshotSuffixes {
			if strings.HasSuffix(base, suf) {
				return true
			}
		}
	}
	return false
}

// CountLines counts the line number of a file's content under the shared
// semantics: empty=0; a single line with no terminal newline=1; a terminal
// newline does NOT create a phantom extra line; CRLF and LF produce equal
// counts. This is the canonical line-count rule both projections must share.
func CountLines(content []byte) int {
	if len(content) == 0 {
		return 0
	}
	text := string(content)
	// Normalize CRLF to LF so both count identically.
	text = strings.ReplaceAll(text, "\r\n", "\n")
	// A lone CR (old Mac) is also a line terminator; normalize it too for
	// parity robustness.
	text = strings.ReplaceAll(text, "\r", "\n")
	parts := strings.Split(text, "\n")
	// A trailing newline produces a trailing "" element that does NOT represent
	// a real line: drop it.
	if n := len(parts); n > 0 && parts[n-1] == "" {
		parts = parts[:n-1]
	}
	return len(parts)
}

// ComputeSignal produces the shared Signal for one file given its repo-relative
// path, content, the loaded policy, and the projection. It performs NO I/O. A
// file that is unsupported or excluded yields a Signal with Nominated=false and
// the appropriate threshold; the caller decides whether to keep or drop it.
//
// Nomination rule: observed > threshold (strict). Equality does NOT nominate.
func ComputeSignal(relPath string, content []byte, policy Policy, proj Projection) Signal {
	relPath = filepath.ToSlash(relPath)
	ext := filepath.Ext(relPath)
	threshold := policy.resolveThreshold(ext, proj)
	observed := CountLines(content)
	nominated := observed > threshold
	return Signal{
		Path:       relPath,
		Projection: proj,
		Language:   ext,
		Metric: Metric{
			Kind:      MetricFileLoc,
			Observed:  observed,
			Threshold: threshold,
			Nominated: nominated,
		},
	}
}

// SortSignals orders signals by descending observed LOC, then by ascending
// normalized path (the deterministic tiebreak). This is the canonical
// presentation order both projections share.
func SortSignals(signals []Signal) {
	sort.SliceStable(signals, func(i, j int) bool {
		if signals[i].Metric.Observed != signals[j].Metric.Observed {
			return signals[i].Metric.Observed > signals[j].Metric.Observed
		}
		return signals[i].Path < signals[j].Path
	})
}

// Eligible reports whether a path is eligible for signal computation in the
// given projection: supported extension AND not excluded.
func (p Policy) Eligible(relPath string, proj Projection) bool {
	ext := filepath.Ext(relPath)
	if !p.supportsExtension(ext) {
		return false
	}
	if p.isExcluded(relPath, proj) {
		return false
	}
	return true
}

// BoundaryIndicatorNotCollected is the v1 placeholder for the
// top_level_symbol_count indicator: the snapshot scanner counts lines only and
// runs no parser, so the indicator is reported as not_collected.
func BoundaryIndicatorNotCollected() BoundaryIndicator {
	return BoundaryIndicator{
		Kind:     "top_level_symbol_count",
		Value:    0,
		Evidence: "not_collected",
	}
}
