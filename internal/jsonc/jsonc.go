// Package jsonc provides string-aware JSONC (JSON with comments) parsing
// utilities shared by the overlay deep-merger and the permission-config emitter.
//
// JSONC permits // line comments, /* block comments */, and trailing commas —
// none of which strict encoding/json accepts. The strippers here normalize
// JSONC to valid JSON while preserving string contents (a // or /* inside a
// JSON string value, such as a URL in $schema, is NOT a comment and must
// survive). This is the single canonical implementation; both internal/overlay
// (MergeJSONC deep-merge) and internal/permconfig (Emit canonicalization) call
// into it so there is exactly one string-aware comment stripper in the binary.
package jsonc

import (
	"encoding/json"
)

// StripComments removes // line comments and /* block */ comments from a JSONC
// byte stream while preserving string contents (a // or /* inside a JSON string
// is NOT a comment). Newlines outside comments are preserved so line numbers
// stay roughly stable for diagnostics.
func StripComments(b []byte) []byte {
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

// StripTrailingCommas removes trailing commas (a `,` immediately followed by
// optional whitespace and then `]` or `}`) outside of strings. JSONC permits
// trailing commas; strict encoding/json does not, so this normalizes JSONC to
// JSON. String-aware: a comma inside a string is never dropped.
func StripTrailingCommas(b []byte) []byte {
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

// Normalize applies StripComments + StripTrailingCommas, returning valid JSON
// bytes suitable for encoding/json.Unmarshal. It does NOT parse; callers that
// need a map should use Parse.
func Normalize(b []byte) []byte {
	return StripTrailingCommas(StripComments(b))
}

// Parse strips JSONC comments and trailing commas, then unmarshals into a
// map[string]any. A JSON null document yields an empty map (never nil) so
// callers can merge/iterate safely. Empty/nil bytes are rejected (fail-closed):
// a missing or truncated document is a real error, not a silent empty document.
// This matches the original overlay.parseJSONC semantics.
func Parse(b []byte) (map[string]any, error) {
	stripped := Normalize(b)
	var m map[string]any
	if err := json.Unmarshal(stripped, &m); err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}
