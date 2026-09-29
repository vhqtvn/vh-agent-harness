package jsonc

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestStripComments_LineComment(t *testing.T) {
	in := []byte(`{"a": 1 // trailing
, "b": 2}`)
	out := StripComments(in)
	// The // comment and everything after it on that line is gone; the newline
	// survives so the comma lands correctly.
	got := strings.TrimSpace(string(Normalize(out)))
	if !strings.Contains(got, `"a": 1`) || !strings.Contains(got, `"b": 2`) {
		t.Fatalf("expected both keys to survive, got %q", got)
	}
	var m map[string]any
	if err := json.Unmarshal(Normalize(out), &m); err != nil {
		t.Fatalf("parse stripped output: %v\n%s", err, out)
	}
	if m["a"].(float64) != 1 || m["b"].(float64) != 2 {
		t.Fatalf("unexpected values: %v", m)
	}
}

func TestStripComments_BlockComment(t *testing.T) {
	in := []byte(`{"a": /* inline block */ 1, "b": 2}`)
	out := Normalize(in)
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if m["a"].(float64) != 1 {
		t.Fatalf("a = %v, want 1", m["a"])
	}
}

// TestStripComments_StringWithDoubleSlash is the critical case: a $schema URL
// like "https://opencode.ai/config.json" contains // inside a string. A naive
// regex stripper would treat it as a line comment and truncate the URL. The
// string-aware stripper must preserve it.
func TestStripComments_StringWithDoubleSlash(t *testing.T) {
	in := []byte(`{
  "$schema": "https://opencode.ai/config.json",
  "agent": {}
}`)
	out := Normalize(in)
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	schema, ok := m["$schema"].(string)
	if !ok {
		t.Fatalf("$schema not a string: %v", m["$schema"])
	}
	if schema != "https://opencode.ai/config.json" {
		t.Fatalf("$schema = %q, want the full URL", schema)
	}
}

func TestStripTrailingCommas(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"object trailing", `{"a": 1, "b": 2,}`},
		{"array trailing", `["a", "b",]`},
		{"nested trailing", `{"a": [1, 2,], "b": {"c": 3,},}`},
		{"comma inside string preserved", `{"a": "hello, world", "b": 2,}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := Normalize([]byte(tc.in))
			var v any
			if err := json.Unmarshal(out, &v); err != nil {
				t.Fatalf("parse: %v\n%s", err, out)
			}
		})
	}
}

func TestParse_NullYieldsEmptyMap(t *testing.T) {
	m, err := Parse([]byte(`null`))
	if err != nil {
		t.Fatalf("parse null: %v", err)
	}
	if len(m) != 0 {
		t.Fatalf("expected empty map, got %v", m)
	}
}

func TestParse_EmptyBytesRejected(t *testing.T) {
	for _, in := range [][]byte{nil, []byte(``)} {
		if _, err := Parse(in); err == nil {
			t.Errorf("Parse(%q): want error (fail-closed), got nil", string(in))
		}
	}
}

func TestParse_FullJSONCDocument(t *testing.T) {
	// A miniature opencode.jsonc: comments, trailing commas, $schema URL.
	in := []byte(`{
  // top-level comment
  "$schema": "https://opencode.ai/config.json",
  "permission": {
    "bash": {
      "*": "deny", // wildcard
      "ls *": "allow",
    },
  },
}`)
	m, err := Parse(in)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if m["$schema"] != "https://opencode.ai/config.json" {
		t.Fatalf("$schema lost: %v", m["$schema"])
	}
	perm := m["permission"].(map[string]any)
	bash := perm["bash"].(map[string]any)
	if bash["*"] != "deny" || bash["ls *"] != "allow" {
		t.Fatalf("bash block wrong: %v", bash)
	}
}

// ---------------------------------------------------------------------------
// Shared stripper-parity fixture corpus (testdata/stripper-parity-fixtures.
// json). This corpus is the single source of truth for the Go↔JS stripper
// twin-parity obligation: internal/permission's scratch-install parity test
// (TestWrapperGrantNaming_JSONCStripperTwinParity) drives the corpus plugin's
// JS twins (templates/core/.opencode/plugins/shell-guard.js
// stripJSONCComments/stripJSONCTrailingCommas, feeding parseJSONCTolerant)
// over the SAME fixtures and asserts byte-identical normalization against
// Normalize — so a semantic change on either side trips a test instead of
// silently re-diverging the twin.
// ---------------------------------------------------------------------------

// stripperParityFixture is one shared-corpus row. Parses records whether the
// fixture is expected to normalize to valid JSON (the malformed-input rows
// pin the fail-closed direction: the stripper passes them through verbatim
// and encoding/json still rejects them — never silently repaired).
type stripperParityFixture struct {
	Name   string `json:"name"`
	Input  string `json:"input"`
	Parses bool   `json:"parses"`
}

func loadStripperParityFixtures(t *testing.T) []stripperParityFixture {
	t.Helper()
	data, err := os.ReadFile("testdata/stripper-parity-fixtures.json")
	if err != nil {
		t.Fatalf("read shared fixture corpus: %v", err)
	}
	var fixtures []stripperParityFixture
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatalf("parse shared fixture corpus: %v", err)
	}
	if len(fixtures) == 0 {
		t.Fatalf("shared fixture corpus must not be empty")
	}
	return fixtures
}

// TestNormalize_SharedFixtureCorpus: every shared-corpus fixture normalizes
// exactly as its Parses flag predicts — valid-JSONC rows become parseable
// JSON; malformed rows stay unparseable after normalization (fail-closed).
func TestNormalize_SharedFixtureCorpus(t *testing.T) {
	for _, f := range loadStripperParityFixtures(t) {
		f := f
		t.Run(f.Name, func(t *testing.T) {
			out := Normalize([]byte(f.Input))
			err := json.Unmarshal(out, new(any))
			if f.Parses && err != nil {
				t.Fatalf("fixture must normalize to valid JSON: %v\ninput:  %q\noutput: %q", err, f.Input, string(out))
			}
			if !f.Parses && err == nil {
				t.Fatalf("fixture marked parses=false must stay unparseable after normalization (fail-closed); got valid JSON: %q", string(out))
			}
		})
	}
}

// TestNormalize_CRLFAndEOFEdges pins EXACT normalized output for the two
// stripper edge fixtures left unpinned by the ad36dfa review advisory (folded
// here, and mirrored into the shared corpus by name so the JS twin is held
// to the same bytes by the parity test):
//   - crlf-line-comment: a // line comment ending in CRLF — the \r is INSIDE
//     the comment and is stripped with it; only the \n survives (line
//     structure stays stable for diagnostics).
//   - eof-line-comment-no-newline: a // line comment terminated by EOF with
//     no trailing newline — the comment simply ends with the input; the
//     preceding newline survives.
func TestNormalize_CRLFAndEOFEdges(t *testing.T) {
	want := map[string]string{
		"crlf-line-comment":           "{\"a\": 1, \n \"b\": 2}",
		"eof-line-comment-no-newline": "{\"a\": 1}\n",
	}
	seen := map[string]bool{}
	for _, f := range loadStripperParityFixtures(t) {
		wantOut, ok := want[f.Name]
		if !ok {
			continue
		}
		f, wantOut := f, wantOut
		seen[f.Name] = true
		t.Run(f.Name, func(t *testing.T) {
			if got := string(Normalize([]byte(f.Input))); got != wantOut {
				t.Fatalf("Normalize = %q; want %q", got, wantOut)
			}
		})
	}
	for name := range want {
		if !seen[name] {
			t.Errorf("advisory edge fixture %q is missing from the shared corpus — the exact pin no longer tests anything", name)
		}
	}
}
