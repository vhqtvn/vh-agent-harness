package schema

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ForbiddenPatternsProject is the project deny-rule payload schema
// (forbidden-patterns.project.js). Ownership class: project_owned (the project
// authors its own deny rules within the platform's deny-rule array contract).
//
// The engine (shell-guard) lives in a core platform_managed file; this file holds
// ONLY the per-project deny-rule payloads. Reconcile is seed-only (project_owned,
// like run-shape): doctor validates the array shape.
type ForbiddenPatternsProject struct{}

// forbiddenPatternRule is the v1 contract for one deny rule. Each rule names the
// pattern, why it is denied, and the canonical alternative. The schema is the
// stable contract the shell-guard engine consumes.
type forbiddenPatternRule struct {
	Pattern     string `json:"pattern"`
	Why         string `json:"why"`
	Alternative string `json:"alternative,omitempty"`
}

// Validate reports structural problems in a forbidden-patterns.project.js file.
// The file is expected to be a JS module exporting a JSON array of deny rules:
//
//	export const FORBIDDEN_PATTERNS = [ { pattern, why, alternative }, ... ]
//
// Parsing is lenient but comment-aware (Slice 5 hardening): we strip JS line
// comments (// ...), block comments (/* ... */), and trailing commas in a
// string-aware pass, then locate the deny-rule array (the whole file if it is
// pure JSON, otherwise the first top-level array literal). This lets project
// authors keep inline documentation in the file without breaking the schema
// check. String literals are tracked so comment markers or brackets inside them
// are never mistaken for syntax.
func (ForbiddenPatternsProject) Validate(raw []byte) []FieldError {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return []FieldError{{Field: "<root>", Message: "file is empty"}}
	}
	s := strings.TrimSpace(string(raw))
	rules, err := extractJSONArray(s)
	if err != nil {
		// The array is not pure JSON. The real shell-guard deny-rule format is a
		// JS module whose rules carry regex literals and unquoted keys
		// (`{ id, re: /.../, allowIf: /.../, why: "..." }`) — see
		// forbidden-patterns.core.js — which json.Unmarshal cannot read and whose
		// regex char-classes ([...]) defeat a JSON bracket scan. Fall back to a
		// regex/JS-aware structural lint rather than failing a well-formed file.
		return validateJSDenyRuleModule(s)
	}
	var errs []FieldError
	for i, r := range rules {
		base := fmt.Sprintf("rules[%d]", i)
		if strings.TrimSpace(r.Pattern) == "" {
			errs = append(errs, FieldError{Field: base + ".pattern", Message: "empty pattern"})
		}
		if strings.TrimSpace(r.Why) == "" {
			errs = append(errs, FieldError{Field: base + ".why", Message: "empty why (every deny rule must explain itself)"})
		}
	}
	return errs
}

// validateJSDenyRuleModule lints a JS-module forbidden-patterns file (regex
// literals + unquoted keys) that the JSON path cannot parse. It locates the
// FORBIDDEN_PATTERNS array with a string- AND regex-literal-aware scan (so
// brackets/braces inside regexes or strings never miscount), then enforces the
// one load-bearing invariant the JSON path also enforces: every top-level rule
// object must carry a non-empty `why`. It does NOT evaluate the regexes (that is
// the runtime engine's job); it guarantees the array is well-formed and every
// rule explains itself.
func validateJSDenyRuleModule(s string) []FieldError {
	cleaned := stripJSNoise(s)
	idx := strings.Index(cleaned, "FORBIDDEN_PATTERNS")
	if idx < 0 {
		return []FieldError{{Field: "<root>", Message: "no FORBIDDEN_PATTERNS export found"}}
	}
	start := strings.Index(cleaned[idx:], "[")
	if start < 0 {
		return []FieldError{{Field: "<root>", Message: "FORBIDDEN_PATTERNS is not an array literal"}}
	}
	start += idx

	objects, err := scanTopLevelObjects(cleaned[start:])
	if err != nil {
		return []FieldError{{Field: "<root>", Message: fmt.Sprintf("could not parse deny-rule array: %v", err)}}
	}
	var errs []FieldError
	for i, obj := range objects {
		if !hasTopLevelKey(obj, "why") {
			errs = append(errs, FieldError{
				Field:   fmt.Sprintf("rules[%d].why", i),
				Message: "empty why (every deny rule must explain itself)",
			})
		}
	}
	return errs
}

// scanTopLevelObjects parses a JS array literal (starting at the leading '[') and
// returns the source text of each depth-1 `{...}` object. It is aware of string
// literals (", ', `) and regex literals (/.../), so brackets and braces inside
// them are never counted. Returns an error if the array bracket never closes.
func scanTopLevelObjects(s string) ([]string, error) {
	var objects []string
	bracket := 0 // [] depth
	brace := 0   // {} depth
	objStart := -1
	inStr := false
	var quote byte
	inRegex := false
	prevSig := byte(0) // previous significant (non-space) char, for regex detection

	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			if c == '\\' {
				i++
				continue
			}
			if c == quote {
				inStr = false
			}
			continue
		}
		if inRegex {
			if c == '\\' {
				i++
				continue
			}
			if c == '/' {
				inRegex = false
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			inStr = true
			quote = c
		case '/':
			// A '/' starts a regex literal when it appears in value position —
			// i.e. after a key colon, an opening bracket/brace/paren, a comma, or
			// nothing. In these deny-rule files that is always the case (re:/.../).
			if prevSig == ':' || prevSig == '[' || prevSig == '{' || prevSig == '(' || prevSig == ',' || prevSig == 0 {
				inRegex = true
			}
		case '[':
			bracket++
		case ']':
			bracket--
		case '{':
			if bracket == 1 && brace == 0 {
				objStart = i
			}
			brace++
		case '}':
			brace--
			if bracket == 1 && brace == 0 && objStart >= 0 {
				objects = append(objects, s[objStart:i+1])
				objStart = -1
			}
		}
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			prevSig = c
		}
		if bracket == 0 {
			return objects, nil // array closed
		}
	}
	return nil, fmt.Errorf("unbalanced '[' (no matching ']')")
}

// hasTopLevelKey reports whether a JS object-literal source contains a key with
// the given name at brace depth 1 (string/regex-aware), with a non-empty value
// region after the colon. Used to assert each deny rule carries a `why`.
func hasTopLevelKey(obj, key string) bool {
	// Look for `<key>` as a bare identifier or quoted, followed by ':' at depth 1.
	depthBrace := 0
	inStr := false
	var quote byte
	inRegex := false
	prevSig := byte(0)
	for i := 0; i < len(obj); i++ {
		c := obj[i]
		if inStr {
			if c == '\\' {
				i++
				continue
			}
			if c == quote {
				inStr = false
			}
			continue
		}
		if inRegex {
			if c == '\\' {
				i++
				continue
			}
			if c == '/' {
				inRegex = false
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			inStr = true
			quote = c
		case '/':
			if prevSig == ':' || prevSig == '[' || prevSig == '{' || prevSig == '(' || prevSig == ',' || prevSig == 0 {
				inRegex = true
			}
		case '{':
			depthBrace++
		case '}':
			depthBrace--
		default:
			if depthBrace == 1 && (c == key[0]) && matchesKeyAt(obj, i, key) {
				return true
			}
		}
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			prevSig = c
		}
	}
	return false
}

// matchesKeyAt reports whether `key` begins at obj[i] as an object key — i.e. the
// run obj[i:i+len(key)] equals key, the preceding char is not an identifier char
// (so `why` does not match inside `notwhy`), and the next significant char is ':'.
func matchesKeyAt(obj string, i int, key string) bool {
	if i+len(key) > len(obj) || obj[i:i+len(key)] != key {
		return false
	}
	if i > 0 {
		p := obj[i-1]
		if p == '_' || p == '$' || (p >= 'a' && p <= 'z') || (p >= 'A' && p <= 'Z') || (p >= '0' && p <= '9') {
			return false
		}
	}
	j := i + len(key)
	for j < len(obj) && (obj[j] == ' ' || obj[j] == '\t' || obj[j] == '\n' || obj[j] == '\r') {
		j++
	}
	if j >= len(obj) || obj[j] != ':' {
		return false
	}
	// Require a non-empty value after the colon (not immediately a , } or EOL).
	k := j + 1
	for k < len(obj) && (obj[k] == ' ' || obj[k] == '\t' || obj[k] == '\n' || obj[k] == '\r') {
		k++
	}
	return k < len(obj) && obj[k] != ',' && obj[k] != '}'
}

// Reconcile is seed-only (project_owned). See RunShape.Reconcile for the same
// rationale.
func (ForbiddenPatternsProject) Reconcile(project, platformDefault []byte) (ReconcileResult, error) {
	if len(strings.TrimSpace(string(project))) == 0 {
		return ReconcileResult{
			Outcome: OutcomeApply,
			Merged:  platformDefault,
			Applied: []string{"forbidden-patterns.project: seed-only (project_owned); substrate seeds when absent"},
		}, nil
	}
	return ReconcileResult{
		Outcome: OutcomeNoop,
		Skipped: []string{"forbidden-patterns.project: project_owned; project instance preserved"},
	}, nil
}

// extractJSONArray pulls the first JSON array of rule objects out of the JS
// module text. It first strips JS comments (// and /* */) and trailing commas
// in a string-aware pass (stripJSNoise), so project-authored files with inline
// documentation parse correctly. It then tries whole-file JSON, falling back to
// the substring between the first '[' and its matching ']' (string-aware
// brace-depth tracking so a bracket inside a string literal never confuses the
// scan).
func extractJSONArray(s string) ([]forbiddenPatternRule, error) {
	cleaned := stripJSNoise(s)

	// Whole-file JSON?
	var rules []forbiddenPatternRule
	if err := json.Unmarshal([]byte(cleaned), &rules); err == nil {
		return rules, nil
	}

	// Substring between first '[' and matching ']' (string-aware).
	start := strings.Index(cleaned, "[")
	if start < 0 {
		return nil, fmt.Errorf("no '[' array literal found")
	}
	depth := 0
	inStr := false
	var quote byte
	for i := start; i < len(cleaned); i++ {
		c := cleaned[i]
		if inStr {
			if c == '\\' {
				i++ // skip escaped char
				continue
			}
			if c == quote {
				inStr = false
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			inStr = true
			quote = c
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				chunk := cleaned[start : i+1]
				if err := json.Unmarshal([]byte(chunk), &rules); err != nil {
					return nil, fmt.Errorf("array literal not valid JSON: %w", err)
				}
				return rules, nil
			}
		}
	}
	return nil, fmt.Errorf("unbalanced '[' (no matching ']')")
}

// stripJSNoise removes JS line comments (// ...), block comments (/* ... */),
// and trailing commas that precede a ']' or '}'. It is string-aware: comment
// markers and commas inside string literals (", ', `) are preserved and escape
// sequences are respected. This lets the deny-rule validator parse
// project-authored files that use inline comments without requiring a full JS
// parser.
func stripJSNoise(s string) string {
	stripped := stripJSComments(s)
	return stripTrailingCommasJS(stripped)
}

// stripJSComments removes // line comments and /* */ block comments. It is
// string-aware so comment markers inside string literals are preserved.
func stripJSComments(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	n := len(s)
	inStr := false
	var quote byte
	for i < n {
		c := s[i]
		if inStr {
			b.WriteByte(c)
			if c == '\\' && i+1 < n {
				b.WriteByte(s[i+1])
				i += 2
				continue
			}
			if c == quote {
				inStr = false
			}
			i++
			continue
		}
		switch {
		case c == '"' || c == '\'' || c == '`':
			inStr = true
			quote = c
			b.WriteByte(c)
			i++
		case c == '/' && i+1 < n && s[i+1] == '/':
			// line comment: skip to end of line (preserve the newline)
			for i < n && s[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < n && s[i+1] == '*':
			// block comment: skip to closing */ (stop at EOF if unbalanced)
			i += 2
			for i+1 < n && !(s[i] == '*' && s[i+1] == '/') {
				i++
			}
			i += 2
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}

// stripTrailingCommasJS drops a ',' whose next significant (non-whitespace)
// character is ']' or '}'. String-aware: commas inside literals are kept.
func stripTrailingCommasJS(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	n := len(s)
	inStr := false
	var quote byte
	for i < n {
		c := s[i]
		if inStr {
			b.WriteByte(c)
			if c == '\\' && i+1 < n {
				b.WriteByte(s[i+1])
				i += 2
				continue
			}
			if c == quote {
				inStr = false
			}
			i++
			continue
		}
		if c == '"' || c == '\'' || c == '`' {
			inStr = true
			quote = c
			b.WriteByte(c)
			i++
			continue
		}
		if c == ',' {
			j := i + 1
			for j < n && (s[j] == ' ' || s[j] == '\t' || s[j] == '\n' || s[j] == '\r') {
				j++
			}
			if j < n && (s[j] == ']' || s[j] == '}') {
				i++ // drop the trailing comma
				continue
			}
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}
