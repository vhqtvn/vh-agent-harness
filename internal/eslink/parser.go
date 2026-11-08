package eslink

import (
	"fmt"
	"strings"
)

// ImportSpec is one named binding required from a provider module.
type ImportSpec struct {
	ImportName string // provider-side name ("A" of `import { A as B }`)
	LocalName  string // local alias; equal to ImportName without rename
}

// ImportDecl is one parsed static `import` declaration. Line is the
// 1-based line of the `import` keyword; SpecLine is the line of the
// module-specifier string (the importer-side source span of the edge —
// they differ on multi-line import lists).
type ImportDecl struct {
	Specifier  string
	Default    bool
	Namespace  bool
	SideEffect bool
	Named      []ImportSpec
	Line       int
	SpecLine   int
}

// ReexportEdge is one `export { a as b } from "m"` entry: ExportedName joins
// this module's surface and ImportName becomes a required edge against m.
// Line/SpecLine are the `export` keyword and specifier-string lines.
type ReexportEdge struct {
	Specifier    string
	ImportName   string
	ExportedName string
	Line         int
	SpecLine     int
}

// StarReexport is one `export * from "m"` declaration with its source span.
type StarReexport struct {
	Specifier string
	Line      int
	SpecLine  int
}

// NamespaceReexport is one `export * as ns from "m"` declaration with its
// source span.
type NamespaceReexport struct {
	Specifier string
	Line      int
	SpecLine  int
}

// DynamicImport records one `import(...)` expression. Dynamic imports are
// NEVER required edges; they surface as reason-coded unknowns (U5/U6).
type DynamicImport struct {
	Specifier string
	Computed  bool
	Line      int
}

// Module is the parsed import/export surface of one ES module under the
// frozen grammar. Unparseable modules (any parse failure — U1
// complete-consumption) are UNKNOWN: their surface is open and every edge
// they participate in is unknown, never broken.
type Module struct {
	Path string

	Imports []ImportDecl

	HasDefault         bool
	NamedExports       map[string]bool
	ReexportEdges      []ReexportEdge
	StarReexports      []StarReexport
	NamespaceReexports []NamespaceReexport

	DynamicImports []DynamicImport

	Unparseable    bool
	UnparseableWhy string
}

// Parse extracts the import/export surface of one module under the frozen
// grammar. It never fails hard: any doubt is recorded as Unparseable with a
// reason code (the tri-state unknown direction, U1/U5).
func Parse(path string, src []byte) (m *Module) {
	m = &Module{Path: path, NamedExports: map[string]bool{}}
	// Module top is an async context: module-top await is ADMITTED (the
	// family-5 grammar-freeze AMENDMENT, operator-ratified 2026-11-08 —
	// module-top [+Await]). await inside NON-async function/arrow bodies
	// still degrades (the retained false-broken guard); for-await degrades.
	p := &parser{lx: newLexer(src), mod: m, asyncStack: []bool{true}}
	defer func() {
		if r := recover(); r != nil {
			f, ok := r.(*parseFailure)
			if !ok {
				panic(r)
			}
			m.Unparseable = true
			m.UnparseableWhy = f.reason
		}
	}()
	p.advance()
	for p.tok.Kind != tkEOF {
		switch {
		case p.isIdent("import") && !p.peekIs("(") && !p.peekIs("."):
			p.parseImport()
		case p.isIdent("export"):
			p.parseExport()
		default:
			p.parseStmt()
		}
	}
	return m
}

type parseFailure struct{ reason string }

// exprNode is the minimal expression descriptor the parser needs beyond
// consumption: the restricted arrow-default check (AMB-1) must know whether
// a default's root, through parentheses, is a comma/sequence expression.
type exprNode struct {
	kind  exprKind
	inner *exprNode // for ekParen
}

type exprKind int

const (
	ekOther exprKind = iota
	ekSeq            // comma/sequence expression
	ekParen          // parenthesized expression
)

type parser struct {
	lx           *lexer
	tok          token // current (upcoming) token
	peeked       bool
	peek         token
	mod          *Module
	asyncStack   []bool
	lastSpec     string
	lastSpecLine int
	wasAsyncHead bool
	// justClosedBlock is true exactly when the last consumed token was the
	// '}' closing a BLOCK BODY (function/arrow/method/constructor body,
	// class body, or a bare block statement) — family 8: "declarations
	// with block bodies need no terminator". Object-literal '}' never sets
	// it (an object literal is not a block body; a declarator initialized
	// with one still requires its ';').
	justClosedBlock bool
}

func (p *parser) async() bool { return p.asyncStack[len(p.asyncStack)-1] }

// --- token cursor -----------------------------------------------------------

func (p *parser) advance() {
	p.justClosedBlock = false
	if p.peeked {
		p.tok = p.peek
		p.peeked = false
		if p.tok.err != "" {
			p.failLexer(p.tok)
		}
		return
	}
	t := p.lx.next()
	p.tok = t
	if t.err != "" {
		p.failLexer(t)
	}
}

func (p *parser) failLexer(t token) {
	panic(&parseFailure{reason: fmt.Sprintf("lexer: %s (line %d)", t.err, t.Line)})
}

// peekTok returns the token after the current one (one-token lookahead). It
// never panics on lexer errors; the error surfaces on advance.
func (p *parser) peekTok() token {
	if p.peeked {
		return p.peek
	}
	t := p.lx.next()
	p.peek = t
	p.peeked = true
	return t
}

// peekIs reports whether the token after the current one is the punct x.
func (p *parser) peekIs(x string) bool {
	t := p.peekTok()
	return t.Kind == tkPunct && t.Text == x
}

func (p *parser) peekIsIdent(x string) bool {
	t := p.peekTok()
	return t.Kind == tkIdent && t.Text == x
}

// cursor is the parser's backtrack state (arrow cover-attempts).
type cursor struct {
	pos, line int
	tok       token
	peeked    bool
	peek      token
	nDyn      int
	nAsync    int
	closedBlk bool
}

func (p *parser) save() cursor {
	return cursor{
		pos: p.lx.pos, line: p.lx.line,
		tok: p.tok, peeked: p.peeked, peek: p.peek,
		nDyn: len(p.mod.DynamicImports), nAsync: len(p.asyncStack),
		closedBlk: p.justClosedBlock,
	}
}

func (p *parser) restore(c cursor) {
	p.lx.pos, p.lx.line = c.pos, c.line
	p.tok, p.peeked, p.peek = c.tok, c.peeked, c.peek
	p.mod.DynamicImports = p.mod.DynamicImports[:c.nDyn]
	p.asyncStack = p.asyncStack[:c.nAsync]
	p.justClosedBlock = c.closedBlk
}

// operandize implements the AMB-2 ratified scoping: at an EXPRESSION-START
// position, a '/' (lexed as division by the context-free lexer) is re-lexed
// as a regex literal. This is the only place regex admission is decided, and
// it is decided by parse structure — an expression must begin here — never
// by a flat previous-token rule. This function is the structural replacement
// for the G32 lexer's entire slash-context state machine.
func (p *parser) operandize() {
	if p.tok.Kind == tkPunct && (p.tok.Text == "/" || p.tok.Text == "/=") {
		start, line := p.tok.start, p.tok.Line
		t, err := p.lx.lexRegexAt(start, line)
		if err != nil {
			panic(&parseFailure{reason: fmt.Sprintf("lexer: %s (line %d)", err, line)})
		}
		p.tok = t
		p.peeked = false // the cached peek belongs to the discarded reading
	}
}

// --- failure helpers --------------------------------------------------------

func (p *parser) fail(format string, args ...any) {
	panic(&parseFailure{reason: fmt.Sprintf(format, args...)})
}

func (p *parser) failAt(line int, format string, args ...any) {
	p.fail("%s (line %d)", fmt.Sprintf(format, args...), line)
}

func (p *parser) isPunct(x string) bool { return p.tok.Kind == tkPunct && p.tok.Text == x }
func (p *parser) isIdent(x string) bool { return p.tok.Kind == tkIdent && p.tok.Text == x }
func (p *parser) atEOF() bool           { return p.tok.Kind == tkEOF }

// expect consumes the punct x or fails.
func (p *parser) expect(x string) {
	if !p.isPunct(x) {
		p.failAt(p.tok.Line, "expected %q, found %q", x, p.describe())
	}
	p.advance()
}

// expectIdent consumes an identifier (any IdentifierName, including
// keywords used as names) and returns it.
func (p *parser) expectIdent() string {
	if p.tok.Kind != tkIdent {
		p.failAt(p.tok.Line, "expected identifier, found %q", p.describe())
	}
	s := p.tok.Text
	p.advance()
	return s
}

func (p *parser) describe() string {
	switch p.tok.Kind {
	case tkEOF:
		return "end of module"
	case tkString:
		return "string literal"
	case tkNumber:
		return "number literal"
	case tkRegex:
		return "regex literal"
	case tkTick:
		return "template literal"
	}
	return p.tok.Text
}

// expectSemi enforces family 8: every simple statement is explicitly
// terminated. A missing ';' is an ASI-dependent boundary — the module
// degrades to UNKNOWN (never a fabricated partial surface) — UNLESS the
// statement's last token closed a block body (family 8 clause: declarations
// and statements ending in a block '}' need no terminator).
func (p *parser) expectSemi() {
	if p.isPunct(";") {
		p.advance()
		return
	}
	if p.justClosedBlock {
		return
	}
	p.failAt(p.tok.Line, "%s: statement not terminated by ';'", ReasonAsiBoundary)
}

// --- module-level declarations ----------------------------------------------

func (p *parser) parseImport() {
	declLine := p.tok.Line
	p.advance() // past 'import'
	d := ImportDecl{Line: declLine}
	if p.tok.Kind == tkString {
		if hasEscape(p.tok.Text) {
			p.failAt(p.tok.Line, "module specifier with escape sequences (not decodable)")
		}
		d.Specifier = p.tok.Text
		d.SpecLine = p.tok.Line
		d.SideEffect = true
		p.advance()
		p.expectSemi()
		p.mod.Imports = append(p.mod.Imports, d)
		return
	}
	if p.tok.Kind == tkIdent {
		p.advance()
		d.Default = true
		if p.isPunct(",") {
			p.advance()
		}
	}
	switch {
	case d.Default && !p.isPunct("*") && !p.isPunct("{"):
		// `import x from "m"` — the clause is complete.
	case p.isPunct("*"):
		p.advance()
		if !p.isIdent("as") {
			p.failAt(p.tok.Line, "expected 'as' in namespace import")
		}
		p.advance()
		p.expectIdent()
		d.Namespace = true
	case p.isPunct("{"):
		p.advance()
		p.parseImportList(&d)
	default:
		if d.Default {
			p.failAt(p.tok.Line, "expected ',', '{' or '*' after default import binding")
		}
		p.failAt(p.tok.Line, "unsupported import form")
	}
	p.expectFrom()
	d.Specifier = p.lastSpec
	d.SpecLine = p.lastSpecLine
	p.expectSemi()
	if !d.Default && !d.Namespace && !d.SideEffect && len(d.Named) == 0 {
		// `import {} from "m"` is valid ES: presence-only edge.
		d.SideEffect = true
	}
	p.mod.Imports = append(p.mod.Imports, d)
}

func (p *parser) parseImportList(d *ImportDecl) {
	for {
		if p.isPunct("}") {
			p.advance()
			return
		}
		if p.tok.Kind != tkIdent && p.tok.Kind != tkString {
			p.failAt(p.tok.Line, "unsupported import-list entry %q", p.describe())
		}
		name := p.tok.Text
		if p.tok.Kind == tkString && hasEscape(name) {
			p.failAt(p.tok.Line, "escaped string-literal import name (not decodable)")
		}
		p.advance()
		local := name
		if p.isIdent("as") {
			p.advance()
			if p.tok.Kind != tkIdent && p.tok.Kind != tkString {
				p.failAt(p.tok.Line, "missing binding after 'as'")
			}
			if p.tok.Kind == tkString && hasEscape(p.tok.Text) {
				p.failAt(p.tok.Line, "escaped string-literal import name (not decodable)")
			}
			local = p.tok.Text
			p.advance()
		}
		d.Named = append(d.Named, ImportSpec{ImportName: name, LocalName: local})
		if p.isPunct(",") {
			p.advance()
		} else if !p.isPunct("}") {
			p.failAt(p.tok.Line, "expected ',' or '}' in import list")
		}
	}
}

// expectFrom consumes `from "spec"`, recording the specifier and its line
// in lastSpec/lastSpecLine.
func (p *parser) expectFrom() {
	if !p.isIdent("from") {
		p.failAt(p.tok.Line, "expected 'from'")
	}
	p.advance()
	if p.tok.Kind != tkString {
		p.failAt(p.tok.Line, "expected module specifier string")
	}
	if hasEscape(p.tok.Text) {
		p.failAt(p.tok.Line, "module specifier with escape sequences (not decodable)")
	}
	p.lastSpec = p.tok.Text
	p.lastSpecLine = p.tok.Line
	p.advance()
}

func (p *parser) parseExport() {
	declLine := p.tok.Line
	p.advance() // past 'export'
	switch {
	case p.isIdent("default"):
		p.advance()
		switch {
		case p.isIdent("function"):
			p.wasAsyncHead = false
			p.parseFunctionRest(true)
		case p.isIdent("async") && p.peekIsIdent("function"):
			p.advance()
			p.wasAsyncHead = true
			p.parseFunctionRest(true)
		case p.isIdent("class"):
			p.parseClassRest(true)
		default:
			p.operandize()
			p.parseAssign()
			p.expectSemi()
		}
		p.mod.HasDefault = true
	case p.isIdent("class"):
		name := p.parseClassRest(false)
		p.mod.NamedExports[name] = true
	case p.isIdent("function"):
		p.wasAsyncHead = false
		name := p.parseFunctionRest(false)
		p.mod.NamedExports[name] = true
	case p.isIdent("async") && p.peekIsIdent("function"):
		p.advance()
		p.wasAsyncHead = true
		name := p.parseFunctionRest(false)
		p.mod.NamedExports[name] = true
	case p.isIdent("const") || p.isIdent("let") || p.isIdent("var"):
		p.advance()
		p.parseDeclarators(declExport)
	case p.isPunct("{"):
		p.advance()
		entries := p.parseNameList()
		if p.isIdent("from") {
			p.expectFrom()
			spec := p.lastSpec
			specLine := p.lastSpecLine
			for _, e := range entries {
				exported := e.exported()
				if exported == "default" {
					p.mod.HasDefault = true
				} else {
					p.mod.NamedExports[exported] = true
				}
				p.mod.ReexportEdges = append(p.mod.ReexportEdges, ReexportEdge{
					Specifier: spec, ImportName: e.first, ExportedName: exported,
					Line: declLine, SpecLine: specLine,
				})
			}
			if len(entries) == 0 {
				// `export {} from "m"`: presence-only edge (reference parity).
				p.mod.Imports = append(p.mod.Imports, ImportDecl{Specifier: spec, SideEffect: true, Line: declLine, SpecLine: specLine})
			}
			p.expectSemi()
		} else {
			for _, e := range entries {
				exported := e.exported()
				if exported == "default" {
					p.mod.HasDefault = true
				} else {
					p.mod.NamedExports[exported] = true
				}
			}
			p.expectSemi()
		}
	case p.isPunct("*"):
		p.advance()
		if p.isIdent("as") {
			p.advance()
			if p.tok.Kind != tkIdent && p.tok.Kind != tkString {
				p.failAt(p.tok.Line, "missing namespace binding after 'as'")
			}
			if p.tok.Kind == tkString && hasEscape(p.tok.Text) {
				p.failAt(p.tok.Line, "escaped string-literal namespace name (not decodable)")
			}
			ns := p.tok.Text
			p.advance()
			p.expectFrom()
			spec := p.lastSpec
			if ns == "default" {
				p.mod.HasDefault = true
			} else {
				p.mod.NamedExports[ns] = true
			}
			p.mod.NamespaceReexports = append(p.mod.NamespaceReexports, NamespaceReexport{
				Specifier: spec, Line: declLine, SpecLine: p.lastSpecLine,
			})
			p.expectSemi()
		} else {
			p.expectFrom()
			p.mod.StarReexports = append(p.mod.StarReexports, StarReexport{
				Specifier: p.lastSpec, Line: declLine, SpecLine: p.lastSpecLine,
			})
			p.expectSemi()
		}
	default:
		p.failAt(p.tok.Line, "unsupported export form starting with %q", p.describe())
	}
}

type nameEntry struct {
	first, second string
	hasAlias      bool
}

func (e nameEntry) exported() string {
	if e.hasAlias {
		return e.second
	}
	return e.first
}

func (p *parser) parseNameList() []nameEntry {
	var out []nameEntry
	for {
		if p.isPunct("}") {
			p.advance()
			return out
		}
		if p.tok.Kind != tkIdent && p.tok.Kind != tkString {
			p.failAt(p.tok.Line, "unsupported name-list entry %q", p.describe())
		}
		var e nameEntry
		e.first = p.tok.Text
		if p.tok.Kind == tkString && hasEscape(e.first) {
			p.failAt(p.tok.Line, "escaped string-literal name (not decodable)")
		}
		p.advance()
		if p.isIdent("as") {
			p.advance()
			if p.tok.Kind != tkIdent && p.tok.Kind != tkString {
				p.failAt(p.tok.Line, "missing binding after 'as'")
			}
			if p.tok.Kind == tkString && hasEscape(p.tok.Text) {
				p.failAt(p.tok.Line, "escaped string-literal name (not decodable)")
			}
			e.second = p.tok.Text
			e.hasAlias = true
			p.advance()
		}
		out = append(out, e)
		if p.isPunct(",") {
			p.advance()
		} else if !p.isPunct("}") {
			p.failAt(p.tok.Line, "expected ',' or '}' in name list")
		}
	}
}

func hasEscape(s string) bool { return strings.Contains(s, "\\") }

// --- declarations -----------------------------------------------------------

type declMode int

const (
	declStmt    declMode = iota // statement position: ';' required, patterns OK
	declExport                  // export position: identifiers only, ';' required
	declForHead                 // for-head slot: no ';' here (caller dispatches)
)

// parseDeclarators parses a const/let/var declarator list. The current token
// is the first binding. In export position every binding identifier joins
// the module's named export surface.
func (p *parser) parseDeclarators(mode declMode) {
	for {
		if mode == declExport && (p.isPunct("{") || p.isPunct("[")) {
			p.failAt(p.tok.Line, "%s", ReasonDestrExport)
		}
		if mode == declExport {
			if p.tok.Kind != tkIdent {
				p.failAt(p.tok.Line, "expected exported binding name, found %q", p.describe())
			}
			p.mod.NamedExports[p.tok.Text] = true
		}
		p.parseBindingPattern(false)
		if p.isPunct("=") {
			p.advance()
			p.operandize()
			p.parseAssign()
		}
		if p.isPunct(",") {
			p.advance()
			continue
		}
		if mode == declForHead {
			return
		}
		p.expectSemi()
		return
	}
}

// parseBindingPattern parses Ident | ObjectPattern | ArrayPattern (binding
// position: var/let/const declarators, for-of heads, parameters). In
// PARAMETER position (paramCtx) the AMB-1 restricted-default rule binds
// through the whole pattern: every element default is a Param-level
// Default, so the sequence-root rejection applies to it exactly as to a
// top-level param default (the tier1_d F1 bypass fix).
func (p *parser) parseBindingPattern(paramCtx bool) {
	switch {
	case p.tok.Kind == tkIdent:
		p.advance()
	case p.isPunct("{"):
		p.parseObjectPattern(paramCtx)
	case p.isPunct("["):
		p.parseArrayPattern(paramCtx)
	default:
		p.failAt(p.tok.Line, "expected binding, found %q", p.describe())
	}
}

// parsePatternDefault parses one binding-pattern element default and
// enforces the AMB-1 sequence-root rejection in parameter position (shared
// by object shorthand `{ a = … }` and array element `[a = …]` defaults).
func (p *parser) parsePatternDefault(paramCtx bool) {
	p.advance() // past '='
	p.operandize()
	d := p.parseAssign()
	if paramCtx && isSeqRoot(d) {
		p.failAt(p.tok.Line, "%s", ReasonArrowDefault)
	}
}

func (p *parser) parseObjectPattern(paramCtx bool) {
	p.expect("{")
	for {
		if p.isPunct("}") {
			p.advance()
			return
		}
		if p.isPunct("...") {
			p.advance()
			p.expectIdent()
			if p.isPunct(",") {
				p.failAt(p.tok.Line, "rest element must be the final element")
			}
			continue
		}
		if p.isPunct("[") {
			p.failAt(p.tok.Line, "computed key in binding pattern not admitted")
		}
		if p.tok.Kind != tkIdent && p.tok.Kind != tkString && p.tok.Kind != tkNumber {
			p.failAt(p.tok.Line, "unsupported binding property key %q", p.describe())
		}
		p.advance()
		if p.isPunct(":") {
			p.advance()
			p.parseBindingPattern(paramCtx)
		} else if p.isPunct("=") {
			// shorthand with default: `{ a = 1 }`
			p.parsePatternDefault(paramCtx)
		} else if p.tok.Kind != tkIdent && !p.isPunct(",") && !p.isPunct("}") {
			p.failAt(p.tok.Line, "shorthand binding requires identifier property")
		}
		if p.isPunct(",") {
			p.advance()
		} else if !p.isPunct("}") {
			p.failAt(p.tok.Line, "expected ',' or '}' in object pattern")
		}
	}
}

func (p *parser) parseArrayPattern(paramCtx bool) {
	p.expect("[")
	for {
		if p.isPunct("]") {
			p.advance()
			return
		}
		if p.isPunct(",") { // elision
			p.advance()
			continue
		}
		if p.isPunct("...") {
			p.advance()
			p.parseBindingPattern(paramCtx)
			if p.isPunct(",") {
				p.failAt(p.tok.Line, "rest element must be the final element")
			}
			continue
		}
		p.parseBindingPattern(paramCtx)
		if p.isPunct("=") {
			p.parsePatternDefault(paramCtx)
		}
		if p.isPunct(",") {
			p.advance()
		} else if !p.isPunct("]") {
			p.failAt(p.tok.Line, "expected ',' or ']' in array pattern")
		}
	}
}

// parseFunctionRest parses everything after the 'function' keyword (cursor
// ON 'function'); anonymous allowed only for export default. Returns the
// name. The caller sets wasAsyncHead before calling.
func (p *parser) parseFunctionRest(anonOK bool) string {
	p.advance() // past 'function'
	if p.isPunct("*") {
		p.failAt(p.tok.Line, "%s", ReasonGenFunc)
	}
	name := ""
	if p.tok.Kind == tkIdent {
		name = p.tok.Text
		p.advance()
	} else if !anonOK {
		p.failAt(p.tok.Line, "function declaration requires a name")
	}
	wasAsync := p.wasAsyncHead
	p.parseParams()
	p.parseBlockBody(wasAsync)
	return name
}

// parseClassRest parses everything after the 'class' keyword (cursor ON
// 'class'). Returns the class name ("" for anonymous export-default).
func (p *parser) parseClassRest(anonOK bool) string {
	p.advance() // past 'class'
	name := ""
	if p.tok.Kind == tkIdent {
		name = p.tok.Text
		p.advance()
	} else if !anonOK {
		p.failAt(p.tok.Line, "class declaration requires a name")
	}
	if p.isIdent("extends") {
		p.advance()
		p.operandize()
		p.parsePostfix() // heritage: LHS expression (member/call chain)
	}
	p.expect("{")
	p.parseClassBody()
	return name
}

// parseClassBody parses class members: constructor-form ONLY (family 4).
func (p *parser) parseClassBody() {
	for {
		if p.isPunct(";") { // empty class element
			p.advance()
			continue
		}
		if p.isPunct("}") {
			p.advance()
			p.justClosedBlock = true
			return
		}
		line := p.tok.Line
		switch {
		case p.isPunct("*"):
			p.failAt(line, "%s", ReasonClassGen)
		case p.isPunct("#"):
			p.failAt(line, "%s", ReasonClassPrivate)
		case p.isPunct("["):
			p.failAt(line, "%s", ReasonClassCompute)
		case p.isIdent("static"):
			p.failAt(line, "%s", ReasonClassStatic)
		case (p.isIdent("get") || p.isIdent("set")) && p.peekTok().Kind == tkIdent:
			p.failAt(line, "%s", ReasonClassAccess)
		case p.isIdent("constructor") && p.peekIs("("):
			p.advance() // constructor
			p.parseParams()
			p.parseBlockBody(false)
		default:
			p.failAt(line, "%s: %q", ReasonClassMethod, p.describe())
		}
	}
}

// parseParams parses a parenthesized parameter list (cursor ON '('):
// functions, arrows, constructor, methods. Restricted production (AMB-1):
// binding patterns; defaults whose root (through parens) is not a sequence
// expression; rest last.
func (p *parser) parseParams() {
	p.expect("(")
	for {
		if p.isPunct(")") {
			p.advance()
			return
		}
		if p.isPunct("...") {
			p.advance()
			p.expectIdent()
			if p.isPunct(",") {
				p.failAt(p.tok.Line, "rest parameter must be the final parameter")
			}
			p.expect(")")
			return
		}
		p.parseBindingPattern(true)
		if p.isPunct("=") {
			p.advance()
			p.operandize()
			d := p.parseAssign()
			if isSeqRoot(d) {
				p.failAt(p.tok.Line, "%s", ReasonArrowDefault)
			}
		}
		if p.isPunct(",") {
			p.advance()
			continue
		}
		p.expect(")")
		return
	}
}

// isSeqRoot reports whether the expression's root, through parentheses, is a
// comma/sequence expression (the cover-grammar rejection, AMB-1).
func isSeqRoot(n *exprNode) bool {
	for n != nil {
		if n.kind == ekSeq {
			return true
		}
		if n.kind != ekParen {
			return false
		}
		n = n.inner
	}
	return false
}

// parseBlockBody parses a '{...}' function body with an explicit async scope.
func (p *parser) parseBlockBody(isAsync bool) {
	p.expect("{")
	p.asyncStack = append(p.asyncStack, isAsync)
	p.stmtLoop("}")
	p.asyncStack = p.asyncStack[:len(p.asyncStack)-1]
	p.expect("}")
	p.justClosedBlock = true
}

// stmtLoop parses statements until the closer punct (not consumed).
func (p *parser) stmtLoop(closer string) {
	for {
		if p.isPunct(closer) || p.atEOF() {
			return
		}
		p.parseStmt()
	}
}

// --- statements -------------------------------------------------------------

func (p *parser) parseStmt() {
	switch {
	case p.isPunct(";"):
		p.advance()
	case p.isPunct("{"):
		p.advance()
		p.stmtLoop("}")
		p.expect("}")
		p.justClosedBlock = true
	case p.isIdent("if"):
		p.parseIf()
	case p.isIdent("for"):
		p.parseFor()
	case p.isIdent("while"):
		p.advance()
		p.expect("(")
		p.operandize()
		p.parseExpression()
		p.expect(")")
		p.parseStmt()
	case p.isIdent("switch"):
		p.parseSwitch()
	case p.isIdent("try"):
		p.parseTry()
	case p.isIdent("break"), p.isIdent("continue"):
		if p.peekTok().Kind == tkIdent {
			p.failAt(p.tok.Line, "%s (labeled break/continue)", ReasonLabel)
		}
		p.advance()
		p.expectSemi()
	case p.isIdent("return"), p.isIdent("throw"):
		p.parseReturnThrow()
	case p.isIdent("const"), p.isIdent("let"), p.isIdent("var"):
		p.advance()
		p.parseDeclarators(declStmt)
	case p.isIdent("function"):
		p.wasAsyncHead = false
		p.parseFunctionRest(false)
	case p.isIdent("class"):
		p.parseClassRest(false)
	case p.isIdent("async") && p.peekIsIdent("function"):
		p.advance()
		p.wasAsyncHead = true
		p.parseFunctionRest(false)
	case p.isIdent("import"):
		// Expression-position import (dynamic / import.meta); a nested
		// declaration is invalid ES and degrades honestly via the parse.
		p.operandize()
		p.parseExpression()
		p.expectSemi()
	case p.isIdent("export"):
		p.failAt(p.tok.Line, "export declaration in non-top-level position")
	case p.isIdent("do"):
		p.failAt(p.tok.Line, "%s", ReasonDoWhile)
	case p.isIdent("with"):
		p.failAt(p.tok.Line, "%s: 'with' statement", ReasonUnsupported)
	case p.isIdent("debugger"):
		p.failAt(p.tok.Line, "%s: 'debugger' statement", ReasonUnsupported)
	case p.tok.Kind == tkIdent && p.peekIs(":"):
		p.failAt(p.tok.Line, "%s", ReasonLabel)
	default:
		p.operandize()
		p.parseExpression()
		p.expectSemi()
	}
}

// return/throw are restricted productions: a line terminator after the
// keyword is ASI reliance (family 8 excludes it), and family 8's explicit
// terminator rule binds here too: a bare 'return' must carry its ';' (the
// ENCLOSING block's '}' is not the statement's own terminator — the block
// exemption covers statements that END in '}', which a bare return does
// not). Bare 'throw' additionally requires an expression (invalid ES
// otherwise) and degrades reason-coded.
func (p *parser) parseReturnThrow() {
	line := p.tok.Line
	kw := p.tok.Text
	p.advance()
	if kw == "throw" && (p.isPunct(";") || p.isPunct("}") || p.atEOF() || p.tok.Line > line) {
		p.failAt(line, "throw requires an expression")
	}
	if p.isPunct(";") {
		p.advance()
		return
	}
	if p.isPunct("}") || p.atEOF() {
		p.failAt(line, "%s: bare '%s' without ';'", ReasonAsiBoundary, kw)
	}
	if p.tok.Line > line {
		p.failAt(line, "%s: newline after %q", ReasonAsiReturn, kw)
	}
	p.operandize()
	p.parseExpression()
	p.expectSemi()
}

func (p *parser) parseIf() {
	p.advance()
	p.expect("(")
	p.operandize()
	p.parseExpression()
	p.expect(")")
	p.parseStmt()
	if p.isIdent("else") {
		p.advance()
		p.parseStmt()
	}
}

func (p *parser) parseFor() {
	p.advance()
	if p.isIdent("await") {
		p.failAt(p.tok.Line, "%s", ReasonForAwait)
	}
	p.expect("(")
	if p.isPunct(";") {
		p.parseForClassicTail()
	} else if p.isIdent("const") || p.isIdent("let") || p.isIdent("var") {
		p.advance()
		if p.isPunct("{") {
			// Object-destructured for heads are outside the frozen subset
			// (freeze record completeness rule, naming map:172's "not
			// needed" row explicitly); array-destructured heads stay
			// admitted (Family 2 variant row).
			p.failAt(p.tok.Line, "%s: object-destructured for-head", ReasonUnsupported)
		}
		p.parseDeclarators(declForHead)
		p.parseForOfIn()
	} else {
		p.operandize()
		p.parseExpression()
		p.parseForOfIn()
	}
	p.parseStmt()
}

// parseForOfIn dispatches after the for-head first slot: 'of' continues a
// for-of (family 2), 'in' is the excluded for-in, ';' continues classic.
func (p *parser) parseForOfIn() {
	switch {
	case p.isIdent("of"):
		p.advance()
		p.operandize()
		p.parseAssign()
		p.expect(")")
	case p.isIdent("in"):
		p.failAt(p.tok.Line, "%s", ReasonForIn)
	case p.isPunct(";"):
		p.parseForClassicTail()
	default:
		p.failAt(p.tok.Line, "expected ';', 'of' or 'in' in for-head, found %q", p.describe())
	}
}

func (p *parser) parseForClassicTail() {
	p.expect(";")
	if !p.isPunct(";") {
		p.operandize()
		p.parseExpression()
	}
	p.expect(";")
	if !p.isPunct(")") {
		p.operandize()
		p.parseExpression() // update slot: sequence expressions admitted
	}
	p.expect(")")
}

func (p *parser) parseSwitch() {
	p.advance()
	p.expect("(")
	p.operandize()
	p.parseExpression()
	p.expect(")")
	p.expect("{")
	for {
		switch {
		case p.isPunct("}"):
			p.advance()
			return
		case p.isIdent("case"):
			p.advance()
			p.operandize()
			p.parseExpression()
			p.expect(":")
			p.stmtLoopUntilNextClause()
		case p.isIdent("default"):
			p.advance()
			p.expect(":")
			p.stmtLoopUntilNextClause()
		default:
			p.failAt(p.tok.Line, "expected 'case', 'default' or '}' in switch body")
		}
	}
}

func (p *parser) stmtLoopUntilNextClause() {
	for {
		if p.isPunct("}") || p.isIdent("case") || p.isIdent("default") || p.atEOF() {
			return
		}
		p.parseStmt()
	}
}

func (p *parser) parseTry() {
	p.advance()
	p.expect("{")
	p.stmtLoop("}")
	p.expect("}")
	if p.isIdent("catch") {
		p.advance()
		if p.isPunct("(") {
			p.advance()
			if p.isPunct("{") || p.isPunct("[") {
				p.failAt(p.tok.Line, "%s", ReasonCatchDestr)
			}
			p.expectIdent()
			p.expect(")")
		}
		p.expect("{")
		p.stmtLoop("}")
		p.expect("}")
	}
	if p.isIdent("finally") {
		p.advance()
		p.expect("{")
		p.stmtLoop("}")
		p.expect("}")
	}
}

// --- expressions ------------------------------------------------------------

// parseExpression parses a full expression including the comma operator
// (sequence expressions). Callers needing a single AssignmentExpr (argument
// lists, declarator initializers, arrow bodies) call parseAssign.
func (p *parser) parseExpression() *exprNode {
	n := p.parseAssign()
	if !p.isPunct(",") {
		return n
	}
	for p.isPunct(",") {
		p.advance()
		p.operandize()
		p.parseAssign()
	}
	return &exprNode{kind: ekSeq}
}

// parseAssign parses one AssignmentExpression (no top-level comma).
func (p *parser) parseAssign() *exprNode {
	var n *exprNode
	switch {
	case p.tok.Kind == tkIdent && p.peekIs("=>"):
		// Binding-identifier arrow. `async => …` (a parameter NAMED
		// async, no [async] prefix) lands here too: the modifier form
		// always requires an identifier after `async` (handled below).
		p.advance() // binding identifier
		p.parseArrowRest(false)
		return &exprNode{}
	case p.isIdent("async") && p.asyncSingleArrowAhead():
		p.advance()
		p.expectIdent()
		p.parseArrowRest(true)
		return &exprNode{}
	case p.isIdent("async") && p.peekIs("("):
		// `async (params) => …` OR a call `async(…)` — cover attempt, then
		// fall back to the plain expression read.
		c := p.save()
		p.advance()
		if a, _, ok := p.tryArrowParen(true); ok {
			return a
		}
		p.restore(c)
		n = p.parseConditional()
	default:
		n = p.parseConditional()
	}
	switch {
	case p.isPunct("="), p.isPunct("+="), p.isPunct("-="), p.isPunct("*="),
		p.isPunct("/="), p.isPunct("%="),
		p.isPunct("<<="), p.isPunct(">>="), p.isPunct(">>>="),
		p.isPunct("&="), p.isPunct("^="), p.isPunct("|="):
		// Compound assignments are admitted exactly over ADMITTED binary
		// operators (arithmetic + bitwise + shift). The corpus carries
		// `^=` (state-lib fnv1a32); the G32 reference parsed it clean.
		p.advance()
		p.operandize()
		p.parseAssign()
		// The completed root is the assignment operation (see
		// parseBinaryPrec's composition note).
		return &exprNode{}
	case p.isPunct("&&="), p.isPunct("||="), p.isPunct("??="), p.isPunct("**="):
		// Logical/exponent assignment operators: probe-absent (map D-2,
		// fixed-string solid) — degrade reason-coded, never silently FIT.
		p.failAt(p.tok.Line, "%s: logical/exponent assignment operator '%s'", ReasonUnsupported, p.tok.Text)
	}
	return n
}

// parseArrowRest parses from the cursor ON '=>'.
func (p *parser) parseArrowRest(isAsync bool) {
	p.expect("=>")
	if p.isPunct("{") {
		p.parseBlockBody(isAsync)
		return
	}
	p.operandize()
	p.asyncStack = append(p.asyncStack, isAsync)
	p.parseAssign()
	p.asyncStack = p.asyncStack[:len(p.asyncStack)-1]
}

func (p *parser) parseConditional() *exprNode {
	n := p.parseBinary()
	if p.isPunct("?") {
		p.advance()
		p.operandize()
		p.parseAssign()
		p.expect(":")
		p.operandize()
		p.parseAssign()
		// The completed root is the conditional — a comma/sequence
		// expression in a BRANCH no longer makes the root a sequence
		// (the AMB-1 root test composes, not penetrates).
		return &exprNode{}
	}
	return n
}

// binary operator precedence table. '**' and the logical-assignment forms
// are outside the frozen subset (exponent probe-absent, map honest-gaps;
// logical-assignment D-2/fixed-string-absent). Binary 'in' IS part of the
// corpus's operator repertoire — verify-recurrence-dedup.js:544
// `!("recurrence" in onDisk)` (live: that module is FIT in the ratified
// pin set) and auto-gate-live.js:1594-1595 — and is NOT on any enumerated
// exclusion list (freeze record; the restart record's family table frames
// admissions as the corpus's measured repertoire minus enumerated
// exclusions). for-in STATEMENTS stay rejected (parseForOfIn). Known
// corpus-absent permissiveness: ES forbids a bare 'in' operator in the
// first slot of a classic for-head; this parser does not model that
// restriction (no corpus site exercises it — recorded in the S1′ closeout).

var binPrec = map[string]int{
	"||": 1, "??": 1,
	"&&": 2,
	"|":  3, "^": 4, "&": 5,
	"==": 6, "!=": 6, "===": 6, "!==": 6,
	"<": 7, ">": 7, "<=": 7, ">=": 7, "instanceof": 7, "in": 7,
	"<<": 8, ">>": 8, ">>>": 8,
	"+": 9, "-": 9,
	"*": 10, "/": 10, "%": 10,
}

func (p *parser) parseBinary() *exprNode { return p.parseBinaryPrec(1) }

func (p *parser) parseBinaryPrec(minPrec int) *exprNode {
	left := p.parseUnary()
	for {
		t := p.tok
		var op string
		if t.Kind == tkPunct || t.Kind == tkIdent {
			op = t.Text
		}
		prec, admitted := binPrec[op]
		if !admitted || prec < minPrec {
			switch op {
			case "++", "--":
				p.failAt(t.Line, "%s", ReasonUpdateOp)
			case "**":
				p.failAt(t.Line, "%s: '**' operator", ReasonUnsupported)
			}
			return left
		}
		p.advance()
		p.operandize()
		p.parseBinaryPrec(prec + 1)
		// The completed root is the binary operation: a parenthesized
		// sequence expression as an OPERAND (e.g. `(b, c) + d`) must not
		// propagate its ekSeq/ekParen root through the composition — only
		// a sequence AT the root (through parens) is the AMB-1 rejection.
		left = &exprNode{}
	}
}

func (p *parser) parseUnary() *exprNode {
	t := p.tok
	switch {
	case t.Kind == tkPunct:
		switch t.Text {
		case "++", "--":
			p.failAt(t.Line, "%s", ReasonUpdateOp)
		case "!", "~", "-", "+":
			p.advance()
			p.operandize()
			p.parseUnary()
			// The completed root is the unary operation — the AMB-1
			// composition note in parseBinaryPrec applies here too: a
			// sequence expression as the unary OPERAND (e.g. `-(b, c)`)
			// must not propagate its ekParen/ekSeq root.
			return &exprNode{}
		}
	case t.Kind == tkIdent:
		switch t.Text {
		case "typeof", "void", "delete":
			p.advance()
			p.operandize()
			p.parseUnary()
			return &exprNode{}
		case "await":
			if !p.async() {
				p.failAt(t.Line, "%s", ReasonTopAwait)
			}
			p.advance()
			p.operandize()
			p.parseUnary()
			return &exprNode{}
		}
	}
	return p.parsePostfix()
}

func (p *parser) parsePostfix() *exprNode {
	n := p.parsePrimary()
	// Member/call continuations compose like binary operators (the AMB-1
	// composition note in parseBinaryPrec): once a '.'/'?.'/'['/'(' tail
	// is consumed the root is the member/call operation — a
	// parenthesized-sequence primary (e.g. `(b, c).x`) must not
	// propagate its ekParen/ekSeq root through the composition.
	for {
		switch {
		case p.isPunct("."):
			p.advance()
			p.expectIdent()
			n = &exprNode{}
		case p.isPunct("?."):
			p.advance()
			if p.isPunct("(") {
				p.failAt(p.tok.Line, "%s: optional call '?.('", ReasonUnsupported)
			}
			if p.isPunct("[") {
				p.advance()
				p.operandize()
				p.parseExpression()
				p.expect("]")
			} else {
				p.expectIdent()
			}
			n = &exprNode{}
		case p.isPunct("["):
			p.advance()
			p.operandize()
			p.parseExpression()
			p.expect("]")
			n = &exprNode{}
		case p.isPunct("("):
			p.parseArguments()
			n = &exprNode{}
		case p.tok.Kind == tkTick:
			p.failAt(p.tok.Line, "%s", ReasonTaggedTmpl)
		case p.isPunct("++"), p.isPunct("--"):
			p.failAt(p.tok.Line, "%s", ReasonUpdateOp)
		default:
			return n
		}
	}
}

func (p *parser) parseArguments() {
	p.expect("(")
	first := true
	for {
		if p.isPunct(")") {
			p.advance()
			return
		}
		if !first {
			p.expect(",")
			if p.isPunct(")") { // trailing comma
				p.advance()
				return
			}
		}
		p.operandize()
		if p.isPunct("...") {
			p.advance()
			p.operandize()
		}
		p.parseAssign()
		first = false
	}
}

func (p *parser) parsePrimary() *exprNode {
	switch p.tok.Kind {
	case tkIdent:
		return p.parseIdentPrimary()
	case tkNumber, tkString, tkRegex:
		p.advance()
		return &exprNode{}
	case tkTick:
		p.parseTemplate()
		return &exprNode{}
	case tkPunct:
		switch p.tok.Text {
		case "(":
			a, attemptReason, ok := p.tryArrowParen(false)
			if ok {
				return a
			}
			// The attempt's own failure reason is held LOCALLY: a nested
			// group's attempt during the re-parse below cannot clobber
			// it (outermost-attempt-wins, the cascade-3 fix).
			p.advance()
			p.operandize()
			inner := p.parseGroupInterior(attemptReason)
			p.expect(")")
			if p.isPunct("=>") {
				// '=>' after the close: a cover-grammar form the freeze
				// REJECTS — degrade citing THIS group's attempt reason.
				reason := attemptReason
				if reason == "" {
					reason = "not a valid restricted parameter list"
				}
				p.failAt(p.tok.Line, "%s: %s", ReasonArrowParams, reason)
			}
			return &exprNode{kind: ekParen, inner: inner}
		case "[":
			p.parseArrayLiteral()
			return &exprNode{}
		case "{":
			p.parseObjectLiteral()
			return &exprNode{}
		case "...":
			p.failAt(p.tok.Line, "%s: '...' outside list context", ReasonUnsupported)
		case "#":
			p.failAt(p.tok.Line, "%s: private name", ReasonUnsupported)
		}
		p.failAt(p.tok.Line, "unexpected token %q in expression", p.describe())
	}
	p.failAt(p.tok.Line, "unexpected %s in expression", p.describe())
	return nil
}

func (p *parser) parseIdentPrimary() *exprNode {
	switch p.tok.Text {
	case "function":
		p.wasAsyncHead = false
		p.parseFunctionRest(true) // anonymous OK in expression position
		return &exprNode{}
	case "async":
		if p.peekIsIdent("function") {
			p.advance()
			p.wasAsyncHead = true
			p.parseFunctionRest(true)
			return &exprNode{}
		}
		if p.asyncSingleArrowAhead() {
			p.advance()
			p.expectIdent()
			p.parseArrowRest(true)
			return &exprNode{}
		}
	case "class":
		p.failAt(p.tok.Line, "%s", ReasonClassExpr)
	case "new":
		p.parseNew()
		return &exprNode{}
	case "import":
		if p.peekIs("(") {
			p.parseDynamicImport()
			return &exprNode{}
		}
		if p.peekIs(".") {
			p.advance()
			p.expect(".")
			p.expectIdent() // meta
			return &exprNode{}
		}
	case "this", "super":
		p.advance()
		return &exprNode{}
	}
	p.advance()
	return &exprNode{}
}

// asyncSingleArrowAhead reports whether the cursor is at `async <ident> =>`.
func (p *parser) asyncSingleArrowAhead() bool {
	if !p.isIdent("async") {
		return false
	}
	c := p.save()
	p.advance()
	if p.tok.Kind != tkIdent {
		p.restore(c)
		return false
	}
	p.advance()
	ok := p.isPunct("=>")
	p.restore(c)
	return ok
}

// tryArrowParen attempts the RESTRICTED arrow parameter production (AMB-1)
// for a '(' group at the cursor. On success (valid restricted binding list
// followed by '=>') the arrow is parsed and (node, "", true) is returned.
// On failure the cursor is RESTORED to '(' and the attempt's OWN failure
// reason is returned alongside (nil, reason, false) — the caller holds it
// locally across the cover re-parse, so a nested group's attempt cannot
// clobber it: if '=>' follows the group close, this was a cover-grammar
// form the freeze REJECTS and the caller degrades citing THIS group's
// outermost attempt reason (the F1-FB false-ok guard: never a silent
// re-read).
func (p *parser) tryArrowParen(isAsync bool) (*exprNode, string, bool) {
	c := p.save()
	reason := ""
	failed := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				f, isF := r.(*parseFailure)
				if !isF {
					panic(r)
				}
				reason = f.reason
				failed = true
			}
		}()
		p.parseParams()
	}()
	if !failed && p.isPunct("=>") {
		p.parseArrowRest(isAsync)
		return &exprNode{}, "", true
	}
	p.restore(c)
	return nil, reason, false
}

// parseGroupInterior parses a parenthesized group's interior as a plain
// expression AFTER a failed restricted-parameter attempt (attemptReason is
// that attempt's own failure reason, "" when the interior parsed as a
// valid binding list but no '=>' followed). If the expression reading
// ALSO fails and the param attempt had failed, the group cannot be read
// either way — the attempt's reason is the informative cover explanation
// and replaces the expression-side failure (U5; e.g. `({ a = (b, c) })`
// fails the attempt on the AMB-1 default rule and the object-literal
// re-read on `=` — the arrow-params form is the honest verdict). When the
// attempt succeeded as a binding list (attemptReason == "") the original
// expression failure propagates untouched.
func (p *parser) parseGroupInterior(attemptReason string) *exprNode {
	if attemptReason == "" {
		return p.parseExpression()
	}
	defer func() {
		if r := recover(); r != nil {
			_, isF := r.(*parseFailure)
			if !isF {
				panic(r)
			}
			panic(&parseFailure{reason: fmt.Sprintf("%s: %s", ReasonArrowParams, attemptReason)})
		}
	}()
	return p.parseExpression()
}

func (p *parser) parseNew() {
	p.advance() // past 'new'
	p.operandize()
	p.parsePrimary()
	for {
		switch {
		case p.isPunct("."):
			p.advance()
			p.expectIdent()
		case p.isPunct("["):
			p.advance()
			p.operandize()
			p.parseExpression()
			p.expect("]")
		default:
			if p.isPunct("(") {
				p.parseArguments()
			}
			return
		}
	}
}

func (p *parser) parseDynamicImport() {
	line := p.tok.Line
	p.advance() // past 'import'
	p.expect("(")
	// The specifier is the literal string ONLY when it is the complete
	// first argument (followed by ')' or ','): the reference contract.
	literal := ""
	isLiteral := false
	if p.tok.Kind == tkString {
		literal = p.tok.Text
		nx := p.peekTok()
		isLiteral = nx.Kind == tkPunct && (nx.Text == ")" || nx.Text == ",")
		if isLiteral && hasEscape(literal) {
			// Not-decodable guard, mirroring expectFrom's static-path
			// stance: the lexer keeps string bytes raw (escapes are NOT
			// decoded), so raw bytes must never be admitted as a literal
			// specifier edge input. An escape in the relative prefix can
			// make IsRelativeSpecifier misjudge the raw bytes (the
			// finding would vanish silently); later escapes keep the
			// prefix but stay undecodable. node --check confirms an
			// escaped-literal dynamic import is legal ES — this is a
			// reason-coded parser-limitation degradation (U5), the same
			// uniform stance the static specifier path takes, never a
			// syntax-error call.
			p.failAt(p.tok.Line, "module specifier with escape sequences (not decodable)")
		}
	}
	if p.isPunct(")") {
		p.advance()
	} else {
		p.operandize()
		p.parseAssign()
		for p.isPunct(",") {
			p.advance()
			p.operandize()
			p.parseAssign()
		}
		p.expect(")")
	}
	d := DynamicImport{Computed: true, Line: line}
	if isLiteral {
		d = DynamicImport{Specifier: literal, Line: line}
	}
	p.mod.DynamicImports = append(p.mod.DynamicImports, d)
}

func (p *parser) parseArrayLiteral() {
	p.expect("[")
	first := true
	for {
		if p.isPunct("]") {
			p.advance()
			return
		}
		if !first {
			p.expect(",")
			if p.isPunct("]") { // trailing comma
				p.advance()
				return
			}
		}
		p.operandize()
		if p.isPunct("...") {
			p.advance()
			p.operandize()
		}
		p.parseAssign()
		first = false
	}
}

func (p *parser) parseObjectLiteral() {
	p.expect("{")
	first := true
	for {
		if p.isPunct("}") {
			p.advance()
			return
		}
		if !first {
			p.expect(",")
			if p.isPunct("}") { // trailing comma
				p.advance()
				return
			}
		}
		first = false
		if p.isPunct("...") { // spread property
			p.advance()
			p.operandize()
			p.parseAssign()
			continue
		}
		if p.isPunct("*") {
			p.failAt(p.tok.Line, "%s: generator method", ReasonGenFunc)
		}
		if p.isIdent("async") && (p.peekTok().Kind == tkIdent || p.peekIs("(")) && !p.peekIs(":") {
			// async method: `async name(...) {}` or `async (computed)`
			p.advance()
			if p.isPunct("[") {
				p.failAt(p.tok.Line, "%s: computed method name", ReasonUnsupported)
			}
			p.expectIdent()
			p.parseParams()
			p.parseBlockBody(true)
			continue
		}
		if (p.isIdent("get") || p.isIdent("set")) &&
			(p.peekTok().Kind == tkIdent || p.peekTok().Kind == tkString || p.peekTok().Kind == tkNumber || p.peekIs("[")) {
			p.failAt(p.tok.Line, "%s: object accessor (get/set)", ReasonUnsupported)
		}
		switch p.tok.Kind {
		case tkIdent, tkString, tkNumber:
			p.advance() // key
			if p.isPunct("(") {
				// method shorthand (e.g. `{ get() {…} }` — a method NAMED
				// get, not an accessor)
				p.parseParams()
				p.parseBlockBody(false)
				continue
			}
			if p.isPunct(":") {
				p.advance()
				p.operandize()
				p.parseAssign()
				continue
			}
			if p.tok.Kind != tkIdent && !p.isPunct(",") && !p.isPunct("}") {
				p.failAt(p.tok.Line, "invalid object literal property")
			}
			// shorthand — separator handling below
		case tkPunct:
			if p.isPunct("[") {
				p.advance()
				p.operandize()
				p.parseAssign()
				p.expect("]")
				if p.isPunct("(") {
					p.parseParams()
					p.parseBlockBody(false)
					continue
				}
				p.expect(":")
				p.operandize()
				p.parseAssign()
				continue
			}
			p.failAt(p.tok.Line, "unsupported object literal key %q", p.describe())
		default:
			p.failAt(p.tok.Line, "unsupported object literal property %q", p.describe())
		}
	}
}

// parseTemplate parses a template literal with recursively parsed ${...}
// substitutions (family 6; no depth cap). Cursor: p.tok == tkTick, with the
// lexer positioned just after the opening backtick. Template interiors are
// scanned as raw character chunks between expression parses — substitutions
// are full expression parses in the ENCLOSING async context.
func (p *parser) parseTemplate() {
	for {
		_, tail, sub, err := p.lx.scanTemplatePart()
		if err != nil {
			p.fail("lexer: %v", err)
		}
		if tail {
			p.advance()
			return
		}
		if sub {
			p.advance() // fetch the substitution's first token
			p.operandize()
			p.parseExpression()
			// parseExpression stopped at '}' and the lexer is already
			// positioned just past it — resume raw template scanning.
			if !p.isPunct("}") {
				p.failAt(p.tok.Line, "expected '}' to close template substitution")
			}
		}
	}
}
