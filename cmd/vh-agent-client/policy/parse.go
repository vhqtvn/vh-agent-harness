// parse.go — the P3 client-side policy rule file: a deliberately small
// line-oriented TOML-ish subset, parsed FAIL-CLOSED.
//
// GRAMMAR (whole grammar — anything else is an error, never a warning):
//
//	file    = line*
//	line    = blank | comment | "[[allow]]" | keyvalue
//	blank   = whitespace only
//	comment = '#' as the first non-whitespace character (whole-line
//	          only; inline trailing comments are REJECTED — a trailing
//	          fragment is a malformed line, not an ignored one)
//	keyvalue = key '=' '"' value '"'
//	key     = "tool" (required) | "path" | "argv0"
//	value   = any runes except '"' and '\' (no escape grammar — a
//	          backslash inside a value is malformed)
//
// SEMANTIC VALIDATION (also fail-closed, at parse time):
//
//	tool   = exact tool name, or a single-segment prefix glob in one of
//	         two namespace shapes: "prefix:*" (matches every tool name
//	         starting "prefix:") or "prefix_*" (matches every tool name
//	         starting "prefix_", anchored at the underscore — the
//	         per-server allow shape for MCP names, P8.2). A bare "*",
//	         a "*" anywhere else, or an empty value is rejected —
//	         wildcards must not pretend to cover semantics they do not.
//	path   = rooted-path prefix for FILE tools only (tool must be
//	         exactly "read", "write", or "edit" — the tools whose args
//	         carry a "path" field). Clean relative or absolute prefix;
//	         "..", ".", and empty segments are rejected (a rule may not
//	         encode a traversal).
//	argv0  = exact argv[0] word for run_shell only (tool must be
//	         exactly "run_shell"). A single plain word.
//
// A rule with neither path nor argv0 is a broad allow for its tool
// pattern — still behind the hard-deny classes (decide.go).
//
// An EMPTY file is valid and means zero rules: everything falls through
// to the default ASK (the interactive/json responder), which is exactly
// the no---policy behavior. Fail-closed means "no silent ignores", not
// "no empty policies".
package policy

import (
	"fmt"
	"os"
	"strings"
)

// Rule is one parsed [[allow]] entry.
type Rule struct {
	// Tool is the tool-name pattern: an exact name or "prefix:*".
	Tool string
	// Path is the optional rooted-path prefix (read/write/edit only).
	Path string
	// Argv0 is the optional exact argv[0] constraint (run_shell only).
	Argv0 string
}

// Policy is a loaded rule set. The zero rules are evaluated in file
// order; the first matching rule wins.
type Policy struct {
	rules []Rule
}

// Rules returns the parsed rules in file order.
func (p *Policy) Rules() []Rule {
	if p == nil {
		return nil
	}
	return append([]Rule(nil), p.rules...)
}

// ParseError is the typed fail-closed parse/load failure. Line/Byte/Text
// are line-bound (Line >= 1); a Line of 0 marks a whole-file failure
// (unreadable file) with Byte = -1 and no Text.
type ParseError struct {
	Path string
	Line int
	Byte int
	Text string
	Msg  string
}

func (e *ParseError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("policy %s:%d (byte %d): %s: %q", e.Path, e.Line, e.Byte, e.Msg, e.Text)
	}
	return fmt.Sprintf("policy %s: %s", e.Path, e.Msg)
}

// LoadFile reads and parses the policy file at path. Any read failure
// is a typed ParseError (Line 0) — an unreadable policy is a usage
// error, never a silently-absent policy.
func LoadFile(path string) (*Policy, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, &ParseError{Path: path, Line: 0, Byte: -1, Msg: fmt.Sprintf("cannot read policy file: %v", err)}
	}
	return Parse(path, src)
}

// Parse parses src, attributing errors to path.
func Parse(path string, src []byte) (*Policy, error) {
	p := &Policy{}
	cur := &Rule{}                  // rule under construction
	open := false                   // an [[allow]] table is active
	openLine := 0                   // line of the active [[allow]] opener
	openByte := 0                   // byte offset of the active [[allow]] opener
	keyLines := map[string]int{}    // line each key of the active rule was set on
	keyTexts := map[string]string{} // verbatim text of each key line
	keyBytes := map[string]int{}    // byte offset of each key line

	// finish validates and commits the rule under construction.
	finish := func() error {
		if !open {
			return nil
		}
		if cur.Tool == "" {
			return &ParseError{Path: path, Line: openLine, Byte: openByte,
				Msg: "incomplete [[allow]] rule: a \"tool\" key is required", Text: "[[allow]]"}
		}
		// Cross-key compatibility, attributed to the key line that
		// completed the incompatible pair (the later key) — line,
		// byte, AND text all from that same key's recorded line.
		if cur.Path != "" && cur.Tool != "read" && cur.Tool != "write" && cur.Tool != "edit" {
			ln, by, tx := laterKeyLine(keyLines, keyBytes, keyTexts, "path", "tool")
			return lineErr(path, ln, by, tx,
				fmt.Sprintf("path constraint is valid only for the file tools read/write/edit (tool %q does not take a rooted path)", cur.Tool))
		}
		if cur.Argv0 != "" && cur.Tool != "run_shell" {
			ln, by, tx := laterKeyLine(keyLines, keyBytes, keyTexts, "argv0", "tool")
			return lineErr(path, ln, by, tx,
				fmt.Sprintf("argv0 constraint is valid only for run_shell (tool %q)", cur.Tool))
		}
		p.rules = append(p.rules, *cur)
		cur, open, keyLines = &Rule{}, false, map[string]int{}
		return nil
	}

	byteOff := 0
	for idx, raw := range strings.Split(string(src), "\n") {
		lineNo := idx + 1 // 1-based, matching editor/cursor conventions
		line := strings.TrimSuffix(raw, "\r")
		trimmed := strings.TrimSpace(line)

		switch {
		case trimmed == "":
			// blank — ignored (documented shape)
		case strings.HasPrefix(trimmed, "#"):
			// whole-line comment — ignored (documented shape)
		case trimmed == "[[allow]]":
			if err := finish(); err != nil {
				return nil, err
			}
			open, openLine, openByte = true, lineNo, byteOff
			keyTexts, keyBytes = map[string]string{}, map[string]int{}
		case strings.HasPrefix(trimmed, "[[") || strings.HasPrefix(trimmed, "["):
			return nil, lineErr(path, lineNo, byteOff, line,
				fmt.Sprintf("unknown section %q (the only section is [[allow]])", trimmed))
		default:
			key, val, ok := splitKeyValue(trimmed)
			if !ok {
				if !open {
					return nil, lineErr(path, lineNo, byteOff, line, "malformed line (expected [[allow]] or key = \"value\")")
				}
				return nil, lineErr(path, lineNo, byteOff, line, "malformed line (expected key = \"value\"; inline comments and escapes are not supported)")
			}
			if !open {
				return nil, lineErr(path, lineNo, byteOff, line, fmt.Sprintf("malformed line: %q appears outside any [[allow]] table", key))
			}
			if _, dup := keyLines[key]; dup {
				return nil, lineErr(path, lineNo, byteOff, line, fmt.Sprintf("duplicate key %q in one [[allow]] rule", key))
			}
			if err := applyKey(cur, key, val); err != nil {
				return nil, lineErr(path, lineNo, byteOff, line, err.Error())
			}
			keyLines[key] = lineNo
			keyTexts[key] = line
			keyBytes[key] = byteOff
		}
		byteOff += len(raw) + 1 // +1 for the consumed '\n'
	}
	if err := finish(); err != nil {
		return nil, err
	}
	return p, nil
}

// laterKeyLine returns the line number, byte offset, and verbatim
// text of whichever of the two named keys appeared later in the file
// — all three from the SAME key's recorded line, so a cross-key error
// never names one key's line number while quoting the other key's
// byte offset and text. Two keys cannot share a line (one key=value
// per line), so the tie branch is unreachable in practice.
func laterKeyLine(keyLines, keyBytes map[string]int, keyTexts map[string]string, a, b string) (line, byteOff int, text string) {
	if keyLines[b] > keyLines[a] {
		a = b
	}
	return keyLines[a], keyBytes[a], keyTexts[a]
}

// lineErr builds a line-bound ParseError.
func lineErr(path string, lineNo, byteOff int, text, msg string) error {
	return &ParseError{Path: path, Line: lineNo, Byte: byteOff, Text: text, Msg: msg}
}

// splitKeyValue parses `key = "value"` after trimming. ok=false marks
// anything that is not exactly that shape (including trailing junk).
func splitKeyValue(s string) (key, val string, ok bool) {
	eq := strings.Index(s, "=")
	if eq <= 0 {
		return "", "", false
	}
	key = strings.TrimSpace(s[:eq])
	if key == "" || strings.ContainsAny(key, " \t\"") {
		return "", "", false
	}
	rest := strings.TrimSpace(s[eq+1:])
	if len(rest) < 2 || rest[0] != '"' || rest[len(rest)-1] != '"' {
		return "", "", false
	}
	inner := rest[1 : len(rest)-1]
	if strings.Contains(inner, "\"") {
		return "", "", false
	}
	return key, inner, true
}

// applyKey validates one key/value against the rule under construction.
func applyKey(r *Rule, key, val string) error {
	switch key {
	case "tool":
		if err := validToolPattern(val); err != nil {
			return err
		}
		r.Tool = val
		return nil
	case "path":
		if val == "" {
			return fmt.Errorf("path must not be empty")
		}
		if err := validRulePath(val); err != nil {
			return err
		}
		r.Path = val
		return nil
	case "argv0":
		if val == "" {
			return fmt.Errorf("argv0 must not be empty")
		}
		if !isPlainWord(val) {
			return fmt.Errorf("argv0 %q must be a single plain word", val)
		}
		r.Argv0 = val
		return nil
	default:
		return fmt.Errorf("unknown key %q (keys: tool, path, argv0)", key)
	}
}

// validToolPattern accepts an exact tool name, "prefix:*", or
// "prefix_*" (the underscore-namespace twin added in P8.2 for
// per-server MCP allows: mcp_mock_* covers mcp_mock_echo but not
// mcp_mockery_echo — the anchor is the separator itself). Exactly one
// trailing "*" after a separator; a bare "*" and any other placement
// stay rejected.
func validToolPattern(v string) error {
	if v == "" {
		return fmt.Errorf("tool must not be empty")
	}
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_' || r == '-' || r == ':' || r == '*':
		default:
			return fmt.Errorf("tool %q: unsupported character %q in tool name", v, r)
		}
	}
	if !strings.Contains(v, "*") {
		return nil // exact name
	}
	sep := byte(0)
	switch {
	case strings.HasSuffix(v, ":*"):
		sep = ':'
	case strings.HasSuffix(v, "_*"):
		sep = '_'
	default:
		return fmt.Errorf("tool %q: the only wildcard shapes are the prefix globs \"prefix:*\" and \"prefix_*\"", v)
	}
	if strings.Count(v, "*") != 1 {
		return fmt.Errorf("tool %q: the only wildcard shapes are the prefix globs \"prefix:*\" and \"prefix_*\" (exactly one star)", v)
	}
	if len(v) < 3 || v[len(v)-2] != sep {
		return fmt.Errorf("tool %q: the only wildcard shapes are the prefix globs \"prefix:*\" and \"prefix_*\"", v)
	}
	prefix := v[:len(v)-2]
	if prefix == "" {
		return fmt.Errorf("tool %q: a bare \"*\" cannot be expressed (name the tools you allow)", v)
	}
	return nil
}

// validRulePath accepts a clean rooted-path prefix: relative or
// absolute, no ".."/"."/empty segments, no backslash. A trailing slash
// is legal (and makes the prefix explicitly directory-shaped).
func validRulePath(v string) error {
	if strings.Contains(v, "\\") {
		return fmt.Errorf("path %q: backslashes are not path separators here", v)
	}
	body := strings.TrimPrefix(v, "/")
	body = strings.TrimSuffix(body, "/")
	if body == "" {
		return fmt.Errorf("path %q: at least one path segment is required", v)
	}
	for _, seg := range strings.Split(body, "/") {
		if seg == "" {
			return fmt.Errorf("path %q: empty path segment", v)
		}
		if seg == "." || seg == ".." {
			return fmt.Errorf("path %q: a rule may not encode %q traversal", v, seg)
		}
	}
	return nil
}

// isPlainWord reports whether s is a single shell-word-safe token:
// letters, digits, underscore, dot, slash, colon, at, percent, plus,
// minus. Anything else (quotes, dollars, globs, metacharacters) makes
// an argv0 unusable as an exact-match constraint.
func isPlainWord(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_' || r == '.' || r == '/' || r == ':' || r == '@' || r == '%' || r == '+' || r == '-':
		default:
			return false
		}
	}
	return true
}
