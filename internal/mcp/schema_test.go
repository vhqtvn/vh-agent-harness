// schema_test.go — the inputSchema → arg-validation-spec mapping
// contract: mappable object schemas pass through (validated shape),
// anything else is UNMAPPABLE (skip that tool with a warning, never
// degrade the server).
package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMapInputSchemaPassThrough(t *testing.T) {
	raw := json.RawMessage(`{"type":"object","properties":{"text":{"type":"string","description":"d"}},"required":["text"]}`)
	out, err := MapInputSchema(raw)
	if err != nil {
		t.Fatalf("MapInputSchema: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("mapped schema not JSON: %v", err)
	}
	if m["type"] != "object" {
		t.Fatalf("type = %v", m["type"])
	}
	if _, ok := m["properties"]; !ok {
		t.Fatal("properties dropped")
	}
	if _, ok := m["required"]; !ok {
		t.Fatal("required dropped")
	}
}

func TestMapInputSchemaMissingTypeIsObject(t *testing.T) {
	// A schema with no "type" but with properties is treated as an
	// object schema (JSON Schema's lax default); pass it through.
	out, err := MapInputSchema(json.RawMessage(`{"properties":{"a":{"type":"string"}}}`))
	if err != nil {
		t.Fatalf("MapInputSchema: %v", err)
	}
	if !strings.Contains(string(out), "properties") {
		t.Fatalf("out = %s", out)
	}
}

func TestMapInputSchemaEmptySchemaGetsObjectShape(t *testing.T) {
	// Some servers advertise an empty/absent schema; an empty object
	// schema is the honest mapped shape.
	out, err := MapInputSchema(json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("MapInputSchema: %v", err)
	}
	if strings.TrimSpace(string(out)) != `{"type":"object"}` && !strings.Contains(string(out), "object") {
		t.Fatalf("out = %s", out)
	}
}

func TestMapInputSchemaUnmappable(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"bare string schema", `{"type":"string"}`},
		{"array schema", `{"type":"array"}`},
		{"non-object schema body", `"just a string"`},
		{"type number", `{"type":"number"}`},
		{"properties not object", `{"type":"object","properties":"x"}`},
		{"required not string array", `{"type":"object","required":[1,2]}`},
		{"required non-array", `{"type":"object","required":"text"}`},
		{"invalid json", `{{`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := MapInputSchema(json.RawMessage(tc.raw))
			if err == nil {
				t.Fatalf("no error for %s", tc.raw)
			}
			if !strings.Contains(err.Error(), "unmappable") {
				t.Fatalf("error %q lacks the unmappable classification", err)
			}
		})
	}
}

func TestSanitizeServerNameForToolNamespace(t *testing.T) {
	cases := map[string]string{
		"vhmcp":       "vhmcp",
		"Zai-MCP":     "zai_mcp",
		"my.server":   "my_server",
		"a b/c":       "a_b_c",
		"--x--":       "x",
		"こんにちは":       "",
		"mcp":         "mcp",
		"UPPER_lower": "upper_lower",
	}
	for in, want := range cases {
		if got := sanitizeName(in); got != want {
			t.Fatalf("sanitizeName(%q) = %q, want %q", in, got, want)
		}
	}
}
