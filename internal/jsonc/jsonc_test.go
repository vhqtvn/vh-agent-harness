package jsonc

import (
	"encoding/json"
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
