package resolver

import (
	"strings"
	"testing"
)

// errContains reports whether any error in errs mentions substr (case-sensitive).
// Used to assert that a Validate call flagged a specific problem class without
// being sensitive to ordering or the full message text.
func errContains(t *testing.T, errs []error, substr string) bool {
	t.Helper()
	for _, e := range errs {
		if strings.Contains(e.Error(), substr) {
			return true
		}
	}
	return false
}

func TestManifestValidate_CoreSeedClean(t *testing.T) {
	for _, raw := range []string{
		// core/gated-commit as a pack would ship it.
		`id: core/gated-commit
provides:
  - commit-message
  - commit-reviewer
  - commit-reviewer-a
  - commit-reviewer-b
  - commit-reviewer-c
  - commit-reviewer-d
  - committer
hard_deps: []
optional_deps: []
`,
		// core/debate as a pack would ship it.
		`id: core/debate
provides:
  - debate
  - debate-proposer
  - debate-critic
  - debate-synth
  - solution-brief
`,
	} {
		m, err := ParseManifest([]byte(raw))
		if err != nil {
			t.Fatalf("ParseManifest errored on seed doc: %v\n%s", err, raw)
		}
		if errs := m.Validate(); len(errs) != 0 {
			t.Fatalf("seed manifest should validate clean, got: %+v\n%s", errs, raw)
		}
	}
}

func TestManifestParseAndMarshal_RoundTrip(t *testing.T) {
	raw := []byte(`id: core/gated-commit
provides:
    - commit-message
    - committer
hard_deps: []
optional_deps: []
`)
	m, err := ParseManifest(raw)
	if err != nil {
		t.Fatalf("ParseManifest errored: %v", err)
	}
	if m.ID != "core/gated-commit" {
		t.Fatalf("ID: expected core/gated-commit, got %q", m.ID)
	}
	if len(m.Provides) != 2 || m.Provides[0] != "commit-message" || m.Provides[1] != "committer" {
		t.Fatalf("Provides: expected [commit-message committer], got %v", m.Provides)
	}
	out, err := MarshalManifest(m)
	if err != nil {
		t.Fatalf("MarshalManifest errored: %v", err)
	}
	// Re-parse the marshaled output and confirm it round-trips.
	m2, err := ParseManifest(out)
	if err != nil {
		t.Fatalf("re-parse of marshaled output errored: %v", err)
	}
	if m2.ID != m.ID {
		t.Fatalf("round-trip ID mismatch: %q vs %q", m2.ID, m.ID)
	}
	if len(m2.Provides) != len(m.Provides) {
		t.Fatalf("round-trip Provides length mismatch: %d vs %d", len(m2.Provides), len(m.Provides))
	}
	if errs := m2.Validate(); len(errs) != 0 {
		t.Fatalf("round-tripped manifest should validate clean, got: %+v", errs)
	}
}

func TestManifestValidate_EmptyID(t *testing.T) {
	m := CapabilityManifest{ID: "", Provides: []string{"a"}}
	errs := m.Validate()
	if !errContains(t, errs, "id is empty") {
		t.Fatalf("expected empty-ID error, got: %+v", errs)
	}
}

func TestManifestValidate_BadIDGrammar(t *testing.T) {
	for _, bad := range []string{
		"gated-commit",      // missing namespace/slash
		"Core/gated-commit", // uppercase
		"core//gated-commit",
		"core/gated_commit",  // underscore not allowed
		"core/gated-commit/", // trailing slash
		"core /gated-commit",
	} {
		m := CapabilityManifest{ID: bad, Provides: []string{"a"}}
		errs := m.Validate()
		if !errContains(t, errs, "is not namespace/name") {
			t.Fatalf("expected grammar error for ID %q, got: %+v", bad, errs)
		}
	}
}

func TestManifestValidate_GoodIDGrammar(t *testing.T) {
	for _, good := range []string{"core/gated-commit", "core/debate", "vendor-x/cap1"} {
		m := CapabilityManifest{ID: good, Provides: []string{"a"}}
		if errs := m.Validate(); len(errs) != 0 {
			t.Fatalf("ID %q should be valid, got: %+v", good, errs)
		}
	}
}

func TestManifestValidate_DuplicateProvides(t *testing.T) {
	m := CapabilityManifest{
		ID:       "core/x",
		Provides: []string{"agent-a", "agent-a"},
	}
	errs := m.Validate()
	if !errContains(t, errs, "duplicate agent") {
		t.Fatalf("expected duplicate-provides error, got: %+v", errs)
	}
}

func TestManifestValidate_EmptyProvidesEntry(t *testing.T) {
	m := CapabilityManifest{
		ID:       "core/x",
		Provides: []string{"agent-a", ""},
	}
	errs := m.Validate()
	if !errContains(t, errs, "provides[1] is empty") {
		t.Fatalf("expected empty-provides error, got: %+v", errs)
	}
}

func TestManifestValidate_SelfDependency(t *testing.T) {
	m := CapabilityManifest{
		ID:       "core/x",
		Provides: []string{"a"},
		HardDeps: []string{"core/x"},
	}
	errs := m.Validate()
	if !errContains(t, errs, "self-dependency") {
		t.Fatalf("expected self-dependency error, got: %+v", errs)
	}
}

func TestManifestValidate_MalformedDepID(t *testing.T) {
	m := CapabilityManifest{
		ID:       "core/x",
		Provides: []string{"a"},
		// "nope" lacks the namespace/name slash form.
		OptionalDeps: []string{"nope"},
	}
	errs := m.Validate()
	if !errContains(t, errs, "is not a well-formed namespace/name") {
		t.Fatalf("expected malformed-dep error, got: %+v", errs)
	}
}

func TestManifestValidate_DuplicateDep(t *testing.T) {
	m := CapabilityManifest{
		ID:       "core/x",
		Provides: []string{"a"},
		HardDeps: []string{"core/y", "core/y"},
	}
	errs := m.Validate()
	if !errContains(t, errs, "duplicate dep") {
		t.Fatalf("expected duplicate-dep error, got: %+v", errs)
	}
}

func TestManifestValidate_CoreOutputsClean(t *testing.T) {
	// A well-formed CoreOutputs list (forward-slash relative, no traversal)
	// validates clean. This mirrors core/media-perception's declaration.
	m := CapabilityManifest{
		ID:       "core/media-perception",
		Provides: []string{"media-perception"},
		CoreOutputs: []string{
			".opencode/agents/media-perception.md",
			".opencode/skills/media-perception/SKILL.md",
		},
	}
	if errs := m.Validate(); len(errs) != 0 {
		t.Fatalf("clean CoreOutputs should validate, got: %+v", errs)
	}
}

func TestManifestValidate_CoreOutputsEmptyOK(t *testing.T) {
	// An empty (or nil) CoreOutputs list is the default and must validate —
	// it means the capability does not gate core outputs (unconditional render).
	m := CapabilityManifest{ID: "core/debate", Provides: []string{"debate"}}
	if errs := m.Validate(); len(errs) != 0 {
		t.Fatalf("empty CoreOutputs should validate, got: %+v", errs)
	}
}

func TestManifestValidate_CoreOutputsAbsoluteRejected(t *testing.T) {
	for _, bad := range []string{
		"/abs/path.md", // leading slash
		"C:/foo.md",    // Windows drive letter (forward-slash form; backslash form is caught by the backslash check first)
	} {
		m := CapabilityManifest{
			ID:          "core/x",
			Provides:    []string{"a"},
			CoreOutputs: []string{bad},
		}
		errs := m.Validate()
		if !errContains(t, errs, "absolute") {
			t.Fatalf("expected absolute error for %q, got: %+v", bad, errs)
		}
	}
}

func TestManifestValidate_CoreOutputsBackslashRejected(t *testing.T) {
	m := CapabilityManifest{
		ID:          "core/x",
		Provides:    []string{"a"},
		CoreOutputs: []string{".opencode\\agents\\foo.md"},
	}
	errs := m.Validate()
	if !errContains(t, errs, "backslash") {
		t.Fatalf("expected backslash error, got: %+v", errs)
	}
}

func TestManifestValidate_CoreOutputsTraversalRejected(t *testing.T) {
	for _, bad := range []string{
		"../escape.md",
		".opencode/../escape.md",
		"foo/../../bar.md",
	} {
		m := CapabilityManifest{
			ID:          "core/x",
			Provides:    []string{"a"},
			CoreOutputs: []string{bad},
		}
		errs := m.Validate()
		if !errContains(t, errs, "traversal") {
			t.Fatalf("expected traversal error for %q, got: %+v", bad, errs)
		}
	}
}

func TestManifestValidate_CoreOutputsDuplicateRejected(t *testing.T) {
	m := CapabilityManifest{
		ID:       "core/x",
		Provides: []string{"a"},
		CoreOutputs: []string{
			".opencode/agents/foo.md",
			".opencode/agents/foo.md",
		},
	}
	errs := m.Validate()
	if !errContains(t, errs, "duplicate output") {
		t.Fatalf("expected duplicate-output error, got: %+v", errs)
	}
}

func TestManifestValidate_CoreOutputsEmptyEntryRejected(t *testing.T) {
	m := CapabilityManifest{
		ID:          "core/x",
		Provides:    []string{"a"},
		CoreOutputs: []string{""},
	}
	errs := m.Validate()
	if !errContains(t, errs, "core_outputs[0] is empty") {
		t.Fatalf("expected empty-entry error, got: %+v", errs)
	}
}
