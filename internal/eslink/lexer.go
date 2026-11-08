package eslink

import (
	"fmt"
	"strings"
)

// The lexer is deliberately context-free: it is a pure function of
// (src, pos, line). It NEVER tracks slash context — a '/' (not starting a
// comment) always lexes as the division punctuator '/' or '/='. The PARSER
// re-lexes a '/' as a regex literal exactly at expression-start positions
// (see parser.operandize) — the AMB-2 ratified scoping, decided by parse
// structure, which is structurally incapable of the G32 flat-prev-token
// misclassification family.

type tokenKind int

const (
	tkEOF tokenKind = iota
	tkIdent
	tkPunct
	tkNumber
	tkString // '...' or "..." — Text holds the raw inner bytes (escapes NOT decoded)
	tkRegex  // /.../flags — opaque
	tkTick   // '`' — template opener; the parser drives template-part scanning
)

// token is one lexical token. Line is the 1-based source line of the first
// byte. start is the byte offset of the first byte (for speculative re-lex).
type token struct {
	Kind  tokenKind
	Text  string
	Line  int
	start int
	err   string // lexer error surfaced when the token is consumed
}

type lexer struct {
	src  []byte
	pos  int
	line int
}

func newLexer(src []byte) *lexer {
	l := &lexer{src: src, line: 1}
	// Strip a UTF-8 BOM (Node accepts BOM-prefixed modules).
	if len(l.src) >= 3 && l.src[0] == 0xEF && l.src[1] == 0xBB && l.src[2] == 0xBF {
		l.pos = 3
	}
	// A leading '#!' line is a HashbangComment (Module goal) — never code.
	if l.pos+1 < len(l.src) && l.src[l.pos] == '#' && l.src[l.pos+1] == '!' {
		i := l.pos + 2
		for i < len(l.src) && !isLineTerm(l.src, i) {
			i++
		}
		if i < len(l.src) {
			i += lineTermLen(l.src, i)
			l.pos = i
			l.line = 2
		} else {
			l.pos = len(l.src)
		}
	}
	return l
}

func isLineTerm(src []byte, i int) bool {
	c := src[i]
	if c == '\n' || c == '\r' {
		return true
	}
	// U+2028 (E2 80 A8) / U+2029 (E2 80 A9)
	return c == 0xE2 && i+2 < len(src) && src[i+1] == 0x80 && (src[i+2] == 0xA8 || src[i+2] == 0xA9)
}

func lineTermLen(src []byte, i int) int {
	if src[i] == '\r' && i+1 < len(src) && src[i+1] == '\n' {
		return 2 // CRLF is one terminator
	}
	if src[i] == 0xE2 {
		return 3
	}
	return 1
}

// punctuators, longest first. Unadmitted compound operators are still lexed
// as tokens so the parser can degrade with a precise reason.
var puncts = []string{
	">>>=", "...",
	"===", "!==", ">>>", "<<=", ">>=", "**=", "&&=", "||=", "??=",
	"=>", "==", "!=", "<=", ">=", "&&", "||", "??", "?.", "++", "--",
	"+=", "-=", "*=", "/=", "%=", "<<", ">>", "&=", "|=", "^=",
	"+", "-", "*", "/", "%", "<", ">", "=", "!", "&", "|", "^", "~",
	"?", ":", ";", ",", ".",
	"(", ")", "[", "]", "{", "}", "#", "@",
}

// next returns the next token, skipping whitespace and comments. Errors
// (unterminated string/comment/regex) are carried on the token's err field;
// the parser converts them into module-level UNKNOWNs when the token is
// consumed.
func (l *lexer) next() token {
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		switch {
		case c == ' ' || c == '\t' || c == '\v' || c == '\f':
			l.pos++
		case isLineTerm(l.src, l.pos):
			l.line++
			l.pos += lineTermLen(l.src, l.pos)
		case c == '/' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '/':
			l.skipToEOL()
		case c == '/' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '*':
			start := l.pos
			line := l.line
			l.pos += 2
			closed := false
			for l.pos < len(l.src) {
				if l.src[l.pos] == '*' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '/' {
					l.pos += 2
					closed = true
					break
				}
				if isLineTerm(l.src, l.pos) {
					l.line++
					l.pos += lineTermLen(l.src, l.pos)
				} else {
					l.pos++
				}
			}
			if !closed {
				return token{Kind: tkEOF, Line: line, start: start, err: "unterminated block comment"}
			}
		default:
			return l.lexToken()
		}
	}
	return token{Kind: tkEOF, Line: l.line, start: l.pos}
}

func (l *lexer) skipToEOL() {
	for l.pos < len(l.src) && !isLineTerm(l.src, l.pos) {
		l.pos++
	}
}

func (l *lexer) lexToken() token {
	start, line := l.pos, l.line
	c := l.src[l.pos]
	mk := func(k tokenKind, text string) token {
		return token{Kind: k, Text: text, Line: line, start: start}
	}
	switch {
	case isIdentStart(l.src, l.pos):
		for l.pos < len(l.src) && isIdentPart(l.src, l.pos) {
			l.pos++
		}
		if l.pos < len(l.src) && l.src[l.pos] == '\\' {
			return token{Kind: tkEOF, Line: line, start: start, err: "identifier escape sequences not supported"}
		}
		return mk(tkIdent, string(l.src[start:l.pos]))
	case c >= '0' && c <= '9':
		return l.lexNumber(start, line)
	case c == '.' && l.pos+1 < len(l.src) && l.src[l.pos+1] >= '0' && l.src[l.pos+1] <= '9':
		return l.lexNumber(start, line)
	case c == '\'' || c == '"':
		return l.lexString(start, line, c)
	case c == '`':
		l.pos++
		return mk(tkTick, "`")
	}
	for _, p := range puncts {
		if len(l.src)-l.pos >= len(p) && string(l.src[l.pos:l.pos+len(p)]) == p {
			// '?.' followed by a digit is a ternary whose branch is a decimal
			// literal, not optional chaining (ECMAScript lookahead restriction).
			if p == "?." && l.pos+2 < len(l.src) && l.src[l.pos+2] >= '0' && l.src[l.pos+2] <= '9' {
				continue
			}
			l.pos += len(p)
			return mk(tkPunct, p)
		}
	}
	l.pos++
	return token{Kind: tkEOF, Line: line, start: start, err: fmt.Sprintf("unsupported character %q", c)}
}

func (l *lexer) lexNumber(start, line int) token {
	s := l.src
	i := l.pos
	if s[i] == '0' && i+1 < len(s) && (s[i+1] == 'x' || s[i+1] == 'X' || s[i+1] == 'b' || s[i+1] == 'B' || s[i+1] == 'o' || s[i+1] == 'O') {
		i += 2
		for i < len(s) && (isHexDigit(s[i]) || s[i] == '_') {
			i++
		}
	} else {
		for i < len(s) && ((s[i] >= '0' && s[i] <= '9') || s[i] == '_') {
			i++
		}
		if i < len(s) && s[i] == '.' {
			i++
			for i < len(s) && ((s[i] >= '0' && s[i] <= '9') || s[i] == '_') {
				i++
			}
		}
		if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
			j := i + 1
			if j < len(s) && (s[j] == '+' || s[j] == '-') {
				j++
			}
			if j < len(s) && s[j] >= '0' && s[j] <= '9' {
				i = j
				for i < len(s) && ((s[i] >= '0' && s[i] <= '9') || s[i] == '_') {
					i++
				}
			}
		}
	}
	if i < len(s) && s[i] == 'n' { // BigInt
		i++
	}
	if i < len(s) && isIdentPart(s, i) {
		return token{Kind: tkEOF, Line: line, start: start, err: "unsupported number literal"}
	}
	l.pos = i
	return token{Kind: tkNumber, Text: string(l.src[start:i]), Line: line, start: start}
}

func isHexDigit(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func (l *lexer) lexString(start, line int, quote byte) token {
	l.pos++ // opening quote
	for {
		if l.pos >= len(l.src) {
			return token{Kind: tkEOF, Line: line, start: start, err: "unterminated string"}
		}
		c := l.src[l.pos]
		if c == quote {
			l.pos++
			return token{Kind: tkString, Text: string(l.src[start+1 : l.pos-1]), Line: line, start: start}
		}
		if c == '\\' {
			l.pos++
			if l.pos >= len(l.src) {
				return token{Kind: tkEOF, Line: line, start: start, err: "unterminated string"}
			}
			if isLineTerm(l.src, l.pos) {
				l.line++
				l.pos += lineTermLen(l.src, l.pos)
			} else {
				l.pos++
			}
			continue
		}
		if isLineTerm(l.src, l.pos) {
			return token{Kind: tkEOF, Line: line, start: start, err: "unterminated string (raw line terminator)"}
		}
		l.pos++
	}
}

// lexRegexAt scans a regex literal starting at byte offset start (must point
// at '/'), returning the opaque literal. It is called by the parser's
// expression-entry re-lex, never speculatively by next().
func (l *lexer) lexRegexAt(start int, line int) (token, error) {
	i := start + 1
	inClass := false
	for i < len(l.src) {
		c := l.src[i]
		if c == '\\' {
			i += 2
			continue
		}
		if isLineTerm(l.src, i) {
			return token{}, fmt.Errorf("unterminated regex literal")
		}
		if inClass {
			if c == ']' {
				inClass = false
			}
		} else {
			if c == '[' {
				inClass = true
			} else if c == '/' {
				i++
				// flags
				fs := i
				for i < len(l.src) && isIdentPart(l.src, i) {
					i++
				}
				if strings.ContainsAny(string(l.src[fs:i]), "\\") {
					return token{}, fmt.Errorf("unsupported regex flags")
				}
				l.pos = i
				l.line = line
				return token{Kind: tkRegex, Text: string(l.src[start:i]), Line: line, start: start}, nil
			}
		}
		i++
	}
	return token{}, fmt.Errorf("unterminated regex literal")
}

// scanTemplatePart scans raw template characters from the current position
// (just after an opening '`' or a closing '}' of a substitution). It returns
// the raw chunk text and whether it ended at the closing backtick (tail),
// a '${' substitution opener (sub), or EOF (error).
func (l *lexer) scanTemplatePart() (text string, tail, sub bool, err error) {
	start := l.pos
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		if c == '\\' {
			l.pos++
			if l.pos < len(l.src) {
				if isLineTerm(l.src, l.pos) {
					l.line++
					l.pos += lineTermLen(l.src, l.pos)
				} else {
					l.pos++
				}
			}
			continue
		}
		if c == '`' {
			chunk := string(l.src[start:l.pos])
			l.pos++
			return chunk, true, false, nil
		}
		if c == '$' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '{' {
			chunk := string(l.src[start:l.pos])
			l.pos += 2
			return chunk, false, true, nil
		}
		if isLineTerm(l.src, l.pos) {
			l.line++
			l.pos += lineTermLen(l.src, l.pos)
			continue
		}
		l.pos++
	}
	return "", false, false, fmt.Errorf("unterminated template literal")
}

// isIdentStart reports whether a rune starting at i can begin an identifier.
// Non-ASCII bytes are accepted coarsely (they lex as identifier characters;
// there is no unicode class table, matching the reference lexer's stance).
func isIdentStart(src []byte, i int) bool {
	c := src[i]
	return c == '$' || c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}

func isIdentPart(src []byte, i int) bool {
	c := src[i]
	return isIdentStart(src, i) || c >= '0' && c <= '9'
}
