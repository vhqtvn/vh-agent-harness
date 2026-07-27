package complexity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureVector is the flexible JSON shape of a parity vector (a superset of
// the optional fields; absent fields are nil/zero).
type fixtureVector struct {
	ID                      string                 `json:"id"`
	Description             string                 `json:"description"`
	Projection              string                 `json:"projection"`
	Policy                  json.RawMessage        `json:"policy"`
	Files                   []fixtureFile          `json:"files"`
	ExpectedSignals         []fixtureExpectedSig   `json:"expected_signals"`
	ExpectedExcluded        []string               `json:"expected_excluded"`
	LineCounts              []fixtureLineCount     `json:"line_counts"`
	ExpectedOrderedPaths    []string               `json:"expected_ordered_paths"`
	MaxCandidates           int                    `json:"max_candidates"`
	ExpectedShownCount      int                    `json:"expected_shown_count"`
	ExpectedTotal           int                    `json:"expected_total"`
	MessageAssertions       *fixtureMsgAssert      `json:"message_assertions"`
	BoundaryIndicatorAssert *fixtureBoundaryAssert `json:"boundary_indicator_assertions"`
	PostEditOverride        *fixtureVector         `json:"post_edit_override"`
}

type fixtureFile struct {
	Path         string `json:"path"`
	Content      string `json:"content"`
	ContentLines int    `json:"content_lines"`
}

type fixtureExpectedSig struct {
	Path      string `json:"path"`
	Language  string `json:"language"`
	Observed  int    `json:"observed"`
	Threshold int    `json:"threshold"`
	Nominated bool   `json:"nominated"`
}

type fixtureLineCount struct {
	Path     string `json:"path"`
	Expected int    `json:"expected"`
}

type fixtureMsgAssert struct {
	MustContain    []string `json:"must_contain"`
	MustNotContain []string `json:"must_not_contain"`
}

type fixtureBoundaryAssert struct {
	Kind               string `json:"kind"`
	Evidence           string `json:"evidence"`
	SeparateFromMetric bool   `json:"separate_from_metric"`
}

type fixtureDoc struct {
	DefaultPolicy map[string]any  `json:"default_policy"`
	Vectors       []fixtureVector `json:"vectors"`
}

func loadFixture(t *testing.T) fixtureDoc {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "complexity-vectors.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var doc fixtureDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return doc
}

// policyFromVector resolves the policy for a vector: if policy is null, use the
// default policy; otherwise unmarshal the given minimal policy and merge with
// defaults for any missing structural fields.
func policyFromVector(t *testing.T, defaultRaw []byte, vec fixtureVector) Policy {
	t.Helper()
	if len(vec.Policy) == 0 || strings.TrimSpace(string(vec.Policy)) == "null" {
		p, err := LoadPolicy(defaultRaw)
		if err != nil {
			t.Fatalf("[%s] load default policy: %v", vec.ID, err)
		}
		return p
	}
	// The vector policy is a minimal overlay; merge it over the default so
	// structural fields (version, enabled, etc.) are present.
	merged := mergePolicyJSON(t, defaultRaw, vec.Policy)
	p, err := LoadPolicy(merged)
	if err != nil {
		t.Fatalf("[%s] load vector policy: %v", vec.ID, err)
	}
	return p
}

// mergePolicyJSON deep-merges an overlay over a base at the JSON level. Array
// values from the overlay REPLACE the base (no array concatenation here — the
// vectors specify complete arrays).
func mergePolicyJSON(t *testing.T, base, overlay []byte) []byte {
	t.Helper()
	var b, o any
	if err := json.Unmarshal(base, &b); err != nil {
		t.Fatalf("merge base: %v", err)
	}
	if err := json.Unmarshal(overlay, &o); err != nil {
		t.Fatalf("merge overlay: %v", err)
	}
	merged := deepMerge(b, o)
	out, err := json.Marshal(merged)
	if err != nil {
		t.Fatalf("marshal merged: %v", err)
	}
	return out
}

func deepMerge(base, overlay any) any {
	ob, ok1 := base.(map[string]any)
	oo, ok2 := overlay.(map[string]any)
	if !ok1 || !ok2 {
		// Non-map overlay replaces base entirely.
		if overlay != nil {
			return overlay
		}
		return base
	}
	out := make(map[string]any, len(ob))
	for k, v := range ob {
		out[k] = v
	}
	for k, v := range oo {
		if existing, had := out[k]; had {
			out[k] = deepMerge(existing, v)
		} else {
			out[k] = v
		}
	}
	return out
}

// materializeContent produces the file content for a fixture file, either from
// the literal `content` string or from `content_lines` (N non-empty lines each
// terminated by a newline).
func materializeContent(f fixtureFile) []byte {
	if f.ContentLines > 0 {
		var sb strings.Builder
		for i := 0; i < f.ContentLines; i++ {
			sb.WriteString("line\n")
		}
		return []byte(sb.String())
	}
	return []byte(f.Content)
}

func TestParityVectors(t *testing.T) {
	doc := loadFixture(t)
	defaultRaw, err := json.Marshal(doc.DefaultPolicy)
	if err != nil {
		t.Fatalf("marshal default policy: %v", err)
	}
	for _, vec := range doc.Vectors {
		vec := vec
		t.Run(vec.ID, func(t *testing.T) {
			runParityVector(t, defaultRaw, vec)
			if vec.PostEditOverride != nil {
				ov := *vec.PostEditOverride
				ov.ID = vec.ID + "/post-edit-override"
				ov.Policy = vec.Policy
				ov.Files = vec.Files
				t.Run("post-edit-override", func(t *testing.T) {
					runParityVector(t, defaultRaw, ov)
				})
			}
		})
	}
}

func runParityVector(t *testing.T, defaultRaw []byte, vec fixtureVector) {
	t.Helper()
	policy := policyFromVector(t, defaultRaw, vec)
	proj := Projection(vec.Projection)

	// Line-count assertions (case 7).
	for _, lc := range vec.LineCounts {
		content := materializeContent(findFile(vec, lc.Path))
		got := CountLines(content)
		if got != lc.Expected {
			t.Errorf("[%s] line count %s: got %d want %d", vec.ID, lc.Path, got, lc.Expected)
		}
	}

	// Compute signals for all files; partition eligible vs excluded.
	var eligible []Signal
	excluded := map[string]bool{}
	var excludedNames []string
	for _, f := range vec.Files {
		content := materializeContent(f)
		if !policy.Eligible(f.Path, proj) {
			excluded[f.Path] = true
			excludedNames = append(excludedNames, f.Path)
			continue
		}
		eligible = append(eligible, ComputeSignal(f.Path, content, policy, proj))
	}

	// Expected excluded set.
	if len(vec.ExpectedExcluded) > 0 {
		for _, p := range vec.ExpectedExcluded {
			if !excluded[p] {
				t.Errorf("[%s] expected %s to be excluded, but it was eligible", vec.ID, p)
			}
		}
		// And no unexpected exclusions among files that should be signals.
		for _, es := range vec.ExpectedSignals {
			if excluded[es.Path] {
				t.Errorf("[%s] expected %s to be eligible, but it was excluded", vec.ID, es.Path)
			}
		}
	}

	// Expected signals: match by path.
	if len(vec.ExpectedSignals) > 0 {
		sigByPath := make(map[string]Signal, len(eligible))
		for _, s := range eligible {
			sigByPath[s.Path] = s
		}
		for _, es := range vec.ExpectedSignals {
			s, ok := sigByPath[es.Path]
			if !ok {
				t.Errorf("[%s] expected signal for %s not found among eligible", vec.ID, es.Path)
				continue
			}
			if s.Language != es.Language {
				t.Errorf("[%s] %s language: got %q want %q", vec.ID, es.Path, s.Language, es.Language)
			}
			if s.Metric.Observed != es.Observed {
				t.Errorf("[%s] %s observed: got %d want %d", vec.ID, es.Path, s.Metric.Observed, es.Observed)
			}
			if s.Metric.Threshold != es.Threshold {
				t.Errorf("[%s] %s threshold: got %d want %d", vec.ID, es.Path, s.Metric.Threshold, es.Threshold)
			}
			if s.Metric.Nominated != es.Nominated {
				t.Errorf("[%s] %s nominated: got %v want %v", vec.ID, es.Path, s.Metric.Nominated, es.Nominated)
			}
		}
	}

	// Ordering assertion (case 11).
	if len(vec.ExpectedOrderedPaths) > 0 {
		SortSignals(eligible)
		got := make([]string, len(eligible))
		for i, s := range eligible {
			got[i] = s.Path
		}
		if !equalStringSlices(got, vec.ExpectedOrderedPaths) {
			t.Errorf("[%s] ordering: got %v want %v", vec.ID, got, vec.ExpectedOrderedPaths)
		}
	}

	// Presentation truncation (case 12).
	if vec.ExpectedTotal > 0 || vec.ExpectedShownCount > 0 {
		SortSignals(eligible)
		shown, total := TruncatePresentation(eligible, vec.MaxCandidates)
		if total != vec.ExpectedTotal {
			t.Errorf("[%s] total: got %d want %d", vec.ID, total, vec.ExpectedTotal)
		}
		if len(shown) != vec.ExpectedShownCount {
			t.Errorf("[%s] shown count: got %d want %d", vec.ID, len(shown), vec.ExpectedShownCount)
		}
	}

	// Message assertions (case 13).
	if vec.MessageAssertions != nil {
		SortSignals(eligible)
		nom := Nominated(eligible)
		if len(nom) == 0 {
			t.Fatalf("[%s] message assertions need a nominated signal, got none", vec.ID)
		}
		msg := SnapshotAdvisoryMessage(nom[0], 1, len(nom))
		for _, sub := range vec.MessageAssertions.MustContain {
			if !strings.Contains(msg, sub) {
				t.Errorf("[%s] message must contain %q; got: %s", vec.ID, sub, msg)
			}
		}
		for _, forbidden := range vec.MessageAssertions.MustNotContain {
			if strings.Contains(msg, forbidden) {
				t.Errorf("[%s] message must NOT contain %q; got: %s", vec.ID, forbidden, msg)
			}
		}
	}

	// Boundary indicator assertions (case 14).
	if vec.BoundaryIndicatorAssert != nil {
		bi := BoundaryIndicatorNotCollected()
		if bi.Kind != vec.BoundaryIndicatorAssert.Kind {
			t.Errorf("[%s] boundary kind: got %q want %q", vec.ID, bi.Kind, vec.BoundaryIndicatorAssert.Kind)
		}
		if bi.Evidence != vec.BoundaryIndicatorAssert.Evidence {
			t.Errorf("[%s] boundary evidence: got %q want %q", vec.ID, bi.Evidence, vec.BoundaryIndicatorAssert.Evidence)
		}
	}
}

func findFile(vec fixtureVector, p string) fixtureFile {
	for _, f := range vec.Files {
		if f.Path == p {
			return f
		}
	}
	return fixtureFile{Path: p}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
