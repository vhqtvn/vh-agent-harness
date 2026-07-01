package cli

// Phase 5 capability-preset + modules-deprecation unit tests. These pin the
// pure resolution helpers in internal/cli/profile.go (the bridge-free preset
// model) without going through the full seam install/update pipeline:
//
//   - profileCapabilityPresets / presetCapabilities: the profile enum -> base
//     capability mapping (minimal/coordination/web = baseline-only; supervised
//     = both core clusters; unknown = baseline-only safe default).
//   - unionCapabilities: explicit `capabilities:` are UNIONED onto the preset
//     (not replace), so `profile: minimal` + `capabilities: [core/debate]`
//     yields {core/debate}. This is the most flexible interpretation of the
//     plan's "opt-in" wording and the one operator-facing semantic the Phase-3
//     bridge obscured.
//   - parseProfileSelection: the pure (profile, capabilities) projection.
//   - modulesDeprecationWarning / emitModulesDeprecationWarning: a non-empty
//     `modules:` list surfaces a deprecation warning on update/doctor (the
//     field is parsed but no longer meaningful under the preset model).

import (
	"bytes"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/resolver"
)

// asCapIDs is a tiny local helper so the table rows read as plain strings.
func asCapIDs(ss []string) []resolver.CapabilityID {
	out := make([]resolver.CapabilityID, 0, len(ss))
	for _, s := range ss {
		out = append(out, s)
	}
	return out
}

func TestPresetCapabilities(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []resolver.CapabilityID
	}{
		{name: "minimal baseline-only", in: "minimal", want: nil},
		{name: "supervised both clusters", in: "supervised", want: asCapIDs([]string{"core/gated-commit", "core/debate"})},
		{name: "coordination aliased to minimal", in: "coordination", want: nil},
		{name: "web aliased to minimal", in: "web", want: nil},
		{name: "unknown safe-default baseline-only", in: "bogus", want: nil},
		{name: "empty safe-default baseline-only", in: "", want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := presetCapabilities(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("presetCapabilities(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestPresetCapabilitiesMutationSafety confirms presetCapabilities returns a
// COPY (mutating it must not corrupt the package-level preset map). This guards
// a future caller that appends to the returned slice.
func TestPresetCapabilitiesMutationSafety(t *testing.T) {
	a := presetCapabilities("supervised")
	a[0] = "mutated"
	b := presetCapabilities("supervised")
	if b[0] == "mutated" {
		t.Fatalf("presetCapabilities must return a copy; mutating the result corrupted the preset map (got %v)", b)
	}
}

func TestUnionCapabilities(t *testing.T) {
	cases := []struct {
		name     string
		preset   []resolver.CapabilityID
		explicit []resolver.CapabilityID
		want     []resolver.CapabilityID
	}{
		{
			name:   "empty preset empty explicit -> empty (baseline-only)",
			preset: nil, explicit: nil,
			want: []resolver.CapabilityID{},
		},
		{
			name:   "supervised preset no explicit -> preset only",
			preset: asCapIDs([]string{"core/gated-commit", "core/debate"}), explicit: nil,
			want: asCapIDs([]string{"core/gated-commit", "core/debate"}),
		},
		{
			name:   "minimal preset union debate -> debate only",
			preset: nil, explicit: asCapIDs([]string{"core/debate"}),
			want: asCapIDs([]string{"core/debate"}),
		},
		{
			name:   "supervised preset union debate -> both, dedup, preset order first",
			preset: asCapIDs([]string{"core/gated-commit", "core/debate"}), explicit: asCapIDs([]string{"core/debate"}),
			want: asCapIDs([]string{"core/gated-commit", "core/debate"}),
		},
		{
			name:   "supervised preset union new cluster -> preset first then explicit",
			preset: asCapIDs([]string{"core/gated-commit"}), explicit: asCapIDs([]string{"core/debate"}),
			want: asCapIDs([]string{"core/gated-commit", "core/debate"}),
		},
		{
			name:   "explicit duplicate within itself deduped",
			preset: nil, explicit: asCapIDs([]string{"core/debate", "core/debate", "core/gated-commit"}),
			want: asCapIDs([]string{"core/debate", "core/gated-commit"}),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := unionCapabilities(tc.preset, tc.explicit)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("unionCapabilities(%v, %v) = %v, want %v", tc.preset, tc.explicit, got, tc.want)
			}
		})
	}
}

func TestParseProfileSelection(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		prof string
		caps []resolver.CapabilityID
	}{
		{name: "minimal no capabilities", raw: "profile: minimal\n", prof: "minimal", caps: nil},
		{name: "supervised no capabilities", raw: "profile: supervised\n", prof: "supervised", caps: nil},
		{name: "minimal plus explicit debate", raw: "profile: minimal\ncapabilities:\n  - core/debate\n", prof: "minimal", caps: asCapIDs([]string{"core/debate"})},
		{name: "no profile key", raw: "features:\n  backlog: true\n", prof: "", caps: nil},
		{name: "empty capabilities list", raw: "profile: minimal\ncapabilities: []\n", prof: "minimal", caps: nil},
		{name: "malformed yaml", raw: "profile: [unterminated\n", prof: "", caps: nil},
		{name: "profile with surrounding whitespace", raw: "profile:   supervised  \n", prof: "supervised", caps: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prof, caps := parseProfileSelection([]byte(tc.raw))
			if prof != tc.prof {
				t.Errorf("profile: got %q, want %q", prof, tc.prof)
			}
			if !reflect.DeepEqual(caps, tc.caps) {
				t.Errorf("capabilities: got %v, want %v", caps, tc.caps)
			}
		})
	}
}

// TestReadProfileSelection_PresetUnion pins the headline Phase-5 semantic —
// explicit capabilities are UNIONED onto the preset, not replace — through the
// live-profile reader (I/O path), so the union contract holds end-to-end at the
// profile layer, not just in the pure helpers. This is the test the plan asked
// for to pin the union behavior.
func TestReadProfileSelection_PresetUnion(t *testing.T) {
	root := t.TempDir()
	// minimal preset (empty) ∪ {core/debate} = {core/debate} — explicit ADDS to
	// the preset, it does not replace anything (minimal has nothing to replace).
	writeProfile(t, root, "profile: minimal\ncapabilities:\n  - core/debate\n")
	got := readProfileSelection(root)
	want := asCapIDs([]string{"core/debate"})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("minimal ∪ {core/debate}: got %v, want %v", got, want)
	}

	// supervised preset ∪ {core/debate} = preset (dedup) — explicit does not
	// DROP the preset's other entries; it can only ADD.
	writeProfile(t, root, "profile: supervised\ncapabilities:\n  - core/debate\n")
	got = readProfileSelection(root)
	want = asCapIDs([]string{"core/gated-commit", "core/debate"})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("supervised ∪ {core/debate}: got %v, want %v (explicit must not replace preset)", got, want)
	}

	// supervised preset ∪ {} = preset unchanged (empty explicit is a no-op).
	writeProfile(t, root, "profile: supervised\n")
	got = readProfileSelection(root)
	want = asCapIDs([]string{"core/gated-commit", "core/debate"})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("supervised ∪ {}: got %v, want %v", got, want)
	}
}

// TestReadProfileSelection_MalformedBaselineOnly confirms a malformed profile
// resolves to baseline-only (nil selection) rather than aborting — render never
// aborts on a malformed profile; doctor reports the schema error separately.
func TestReadProfileSelection_MalformedBaselineOnly(t *testing.T) {
	root := t.TempDir()
	// Write a syntactically valid but schema-invalid profile (bogus profile enum
	// value fails profileEnum validation, so Validate returns errors).
	writeProfile(t, root, "profile: not-a-real-enum\n")
	got := readProfileSelection(root)
	if got != nil {
		// Unknown enum would normally fall back to baseline-only via
		// presetCapabilities, but a SCHEMA-invalid profile short-circuits to nil
		// before the preset lookup. Either way the result is baseline-only.
		sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
		t.Errorf("malformed profile: got %v, want nil (baseline-only)", got)
	}
}

// --- modules deprecation -----------------------------------------------------

func TestModulesDeprecationWarning(t *testing.T) {
	cases := []struct {
		name    string
		modules []string
		want    string
		empty   bool
	}{
		{name: "nil modules no warning", modules: nil, empty: true},
		{name: "empty modules no warning", modules: []string{}, empty: true},
		{name: "single entry warns", modules: []string{"core"}, want: "seam: warning: vh-harness-profile.yml `modules:` (1 entry) is deprecated;"},
		{name: "multiple entries warn with count", modules: []string{"core", "extra"}, want: "seam: warning: vh-harness-profile.yml `modules:` (2 entries) is deprecated;"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := modulesDeprecationWarning(tc.modules)
			if tc.empty {
				if got != "" {
					t.Errorf("got warning %q, want empty", got)
				}
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Errorf("warning %q must contain %q", got, tc.want)
			}
			if !strings.HasSuffix(got, "\n") {
				t.Errorf("warning must end with newline; got %q", got)
			}
		})
	}
}

// TestModulesDeprecationWarningNoValueInterpolation confirms the modules VALUES
// are NOT interpolated into the warning (they are operator-controlled and could
// contain shell metacharacters). Only the count appears.
func TestModulesDeprecationWarningNoValueInterpolation(t *testing.T) {
	got := modulesDeprecationWarning([]string{"core; rm -rf /", "extra"})
	if strings.Contains(got, "rm -rf /") {
		t.Errorf("modules value must NOT be interpolated into warning; got %q", got)
	}
}

// TestEmitModulesDeprecationWarning swaps the package sink to a buffer and
// asserts the warning is written when the LIVE profile carries modules, and is
// NOT written when the live profile omits modules (or is absent).
func TestEmitModulesDeprecationWarning(t *testing.T) {
	orig := profileDeprecationSink
	profileDeprecationSink = &bytes.Buffer{}
	defer func() { profileDeprecationSink = orig }()

	root := t.TempDir()
	// Greenfield: no live profile -> no warning.
	emitModulesDeprecationWarning(root)
	if got := profileDeprecationSink.(*bytes.Buffer).String(); got != "" {
		t.Errorf("greenfield must not warn; got %q", got)
	}

	// Live profile WITH modules -> warning emitted.
	writeProfile(t, root, "profile: supervised\nmodules:\n  - core\nfeatures:\n  backlog: true\noverlays: []\npolicy_packs: []\n")
	profileDeprecationSink.(*bytes.Buffer).Reset()
	emitModulesDeprecationWarning(root)
	if got := profileDeprecationSink.(*bytes.Buffer).String(); !strings.Contains(got, "modules:") || !strings.Contains(got, "deprecated") {
		t.Errorf("profile with modules must warn; got %q", got)
	}

	// Live profile WITHOUT modules -> no warning.
	writeProfile(t, root, "profile: supervised\nfeatures:\n  backlog: true\noverlays: []\npolicy_packs: []\n")
	profileDeprecationSink.(*bytes.Buffer).Reset()
	emitModulesDeprecationWarning(root)
	if got := profileDeprecationSink.(*bytes.Buffer).String(); got != "" {
		t.Errorf("profile without modules must not warn; got %q", got)
	}
}

// TestEmitModulesDeprecationWarningMalformedQuiet confirms a malformed profile
// does not emit the warning (doctor reports the schema error separately; the
// deprecation path must stay quiet so it never races the schema error).
func TestEmitModulesDeprecationWarningMalformedQuiet(t *testing.T) {
	orig := profileDeprecationSink
	profileDeprecationSink = &bytes.Buffer{}
	defer func() { profileDeprecationSink = orig }()

	root := t.TempDir()
	// Schema-invalid (bogus enum) but with modules present — must stay quiet.
	writeProfile(t, root, "profile: bogus\nmodules:\n  - core\n")
	emitModulesDeprecationWarning(root)
	if got := profileDeprecationSink.(*bytes.Buffer).String(); got != "" {
		t.Errorf("malformed profile must not emit deprecation warning; got %q", got)
	}
}
