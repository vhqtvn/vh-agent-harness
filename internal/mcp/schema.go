// schema.go — MCP inputSchema → the pipeline's arg-validation spec.
//
// The engine's ToolDefinition.Parameters IS a JSON Schema object blob
// (type/properties/required), so the mapping is a VALIDATED PASS-
// THROUGH: the MCP schema must parse as a JSON object whose type is
// "object" (explicitly, or by the lax JSON-Schema default when only
// properties/required are present). Anything else — a bare string
// schema, an array schema, structurally invalid values — is
// UNMAPPABLE: the caller skips THAT tool with a warning and the server
// stays up (fail-closed per-tool, never per-daemon).
package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// MapInputSchema validates and returns the tool's argument schema for
// ToolDefinition.Parameters. An empty schema maps to the minimal
// `{"type":"object"}` shape (no constraints — the honest translation of
// "the server described nothing").
func MapInputSchema(raw json.RawMessage) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" || string(trimmed) == "{}" {
		return json.RawMessage(`{"type":"object"}`), nil
	}
	var s struct {
		Type       string          `json:"type"`
		Properties json.RawMessage `json:"properties"`
		Required   json.RawMessage `json:"required"`
	}
	if err := json.Unmarshal(trimmed, &s); err != nil {
		return nil, fmt.Errorf("unmappable inputSchema (not valid JSON): %v", err)
	}
	if s.Type == "" {
		s.Type = "object" // lax JSON-Schema default (explicit or fully absent)
	}
	if s.Type != "object" {
		return nil, fmt.Errorf("unmappable inputSchema (type %q — only object schemas map to the engine's argument validation)", s.Type)
	}
	if len(s.Properties) > 0 && !json.Valid(s.Properties) {
		return nil, fmt.Errorf("unmappable inputSchema (properties is not valid JSON)")
	}
	var props map[string]json.RawMessage
	if len(s.Properties) > 0 {
		if err := json.Unmarshal(s.Properties, &props); err != nil || props == nil {
			return nil, fmt.Errorf("unmappable inputSchema (properties must be an object)")
		}
	}
	if len(s.Required) > 0 {
		var req []any
		if err := json.Unmarshal(s.Required, &req); err != nil {
			return nil, fmt.Errorf("unmappable inputSchema (required must be an array of strings)")
		}
		for _, r := range req {
			if _, ok := r.(string); !ok {
				return nil, fmt.Errorf("unmappable inputSchema (required entries must be strings)")
			}
		}
	}
	return json.RawMessage(trimmed), nil
}

// toolNameSanitizer is the [a-z0-9_] character policy for namespaced
// tool names (lowercase; every other rune collapses to a single _).
var toolNameSanitizer = regexp.MustCompile(`[^a-z0-9_]+`)

// sanitizeName lowercases and collapses every non-[a-z0-9_] run in s to
// a single underscore, trimming leading/trailing underscores. Used for
// both server and tool name segments (mcp_<server>_<tool>).
func sanitizeName(s string) string {
	s = strings.ToLower(s)
	s = toolNameSanitizer.ReplaceAllString(s, "_")
	return strings.Trim(s, "_")
}
