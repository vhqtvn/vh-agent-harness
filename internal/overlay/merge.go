// Package overlay implements the Slice-4 overlay pack mechanism: opt-in packs
// selected via vh-harness-profile.yml (overlays:[...]) that layer additively on top
// of the curated core corpus. Each pack contributes unit files (agents/, skills/,
// commands/ mirroring the .opencode/ subtree) plus two merge-content files:
//
//   - opencode-append.jsonc   — deep-merged into the rendered opencode.jsonc
//   - callable-graph-snippet.md — appended to callable-graph.md
//
// The deep-merge is dependency-free (stdlib encoding/json) and string-aware
// comment-stripping so it works on JSONC sources. Overlay units are ownership
// class overlay_extension (auto-overwritten while the pack stays active).
package overlay

import (
	"encoding/json"
	"fmt"
)

// MergeJSONC deep-merges one or more JSONC append documents into a base JSONC
// document. Comments are stripped before parsing (JSONC -> JSON); the merged
// result is re-serialized as indented JSON (comments are not preserved in merged
// output — acceptable since opencode reads JSONC tolerantly).
//
// Merge semantics: for each key in an append, if both base and append values are
// JSON objects, recurse; otherwise the append value wins (scalars, arrays, and
// type-mismatches overwrite the base). This lets an append BOTH introduce new
// top-level entries (e.g. a brand-new agent block) AND inject keys into existing
// nested maps (e.g. add "browser-qa": "allow" into an orchestrator agent's
// permission.task map) without disturbing sibling fields.
func MergeJSONC(base []byte, appends ...[]byte) ([]byte, error) {
	dst, err := parseJSONC(base)
	if err != nil {
		return nil, fmt.Errorf("merge base: %w", err)
	}
	for i, a := range appends {
		src, err := parseJSONC(a)
		if err != nil {
			return nil, fmt.Errorf("merge append[%d]: %w", i, err)
		}
		deepMerge(dst, src)
	}
	out, err := json.MarshalIndent(dst, "", "    ")
	if err != nil {
		return nil, fmt.Errorf("merge marshal: %w", err)
	}
	return append(out, '\n'), nil
}

// parseJSONC strips JSONC comments and unmarshals into a map. A null/empty
// document yields an empty map (never nil) so callers can merge safely.
func parseJSONC(b []byte) (map[string]any, error) {
	stripped := stripTrailingCommas(stripJSONCComments(b))
	var m map[string]any
	if err := json.Unmarshal(stripped, &m); err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

// deepMerge recursively merges src into dst in place. Nested maps recurse; all
// other value kinds are overwritten by src (append wins).
func deepMerge(dst, src map[string]any) {
	for k, sv := range src {
		dv, ok := dst[k]
		if !ok {
			dst[k] = sv
			continue
		}
		dm, dIsMap := dv.(map[string]any)
		sm, sIsMap := sv.(map[string]any)
		if dIsMap && sIsMap {
			deepMerge(dm, sm)
		} else {
			dst[k] = sv
		}
	}
}

// stripJSONCComments removes // line comments and /* block */ comments from a
// JSONC byte stream while preserving string contents (a // or /* inside a JSON
// string is NOT a comment). Newlines outside comments are preserved so line
// numbers stay roughly stable for diagnostics.
func stripJSONCComments(b []byte) []byte {
	out := make([]byte, 0, len(b))
	n := len(b)
	inStr := false
	for i := 0; i < n; i++ {
		c := b[i]
		switch {
		case inStr:
			out = append(out, c)
			if c == '\\' {
				if i+1 < n {
					out = append(out, b[i+1])
					i++
				}
			} else if c == '"' {
				inStr = false
			}
		case c == '"':
			inStr = true
			out = append(out, c)
		case c == '/' && i+1 < n && b[i+1] == '/':
			// line comment: skip to (but not past) the newline
			for i < n && b[i] != '\n' {
				i++
			}
			i-- // outer i++ will land back on the newline, which is then emitted
		case c == '/' && i+1 < n && b[i+1] == '*':
			// block comment: skip to the closing */
			i += 2
			for i+1 < n && !(b[i] == '*' && b[i+1] == '/') {
				i++
			}
			// i is at '*' (with '/' at i+1) or at end; advance past the '/' if present
			if i+1 < n {
				i++ // now at '/'; outer i++ moves past it
			}
		default:
			out = append(out, c)
		}
	}
	return out
}

// stripTrailingCommas removes trailing commas (a `,` immediately followed by
// optional whitespace and then `]` or `}`) outside of strings. JSONC permits
// trailing commas; strict encoding/json does not, so this normalizes JSONC ->
// JSON. String-aware: a comma inside a string is never dropped.
func stripTrailingCommas(b []byte) []byte {
	out := make([]byte, 0, len(b))
	n := len(b)
	inStr := false
	for i := 0; i < n; i++ {
		c := b[i]
		switch {
		case inStr:
			out = append(out, c)
			if c == '\\' {
				if i+1 < n {
					out = append(out, b[i+1])
					i++
				}
			} else if c == '"' {
				inStr = false
			}
		case c == '"':
			inStr = true
			out = append(out, c)
		case c == ',':
			// Look ahead past whitespace; if the next significant char closes an
			// array/object, this is a trailing comma -> drop it.
			j := i + 1
			for j < n && (b[j] == ' ' || b[j] == '\t' || b[j] == '\n' || b[j] == '\r') {
				j++
			}
			if j < n && (b[j] == ']' || b[j] == '}') {
				// trailing comma: do not emit
			} else {
				out = append(out, c)
			}
		default:
			out = append(out, c)
		}
	}
	return out
}
