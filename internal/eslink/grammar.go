// Package eslink is the successor ES-module link parser + standalone corpus
// lint harness for the FROZEN O4 conformance grammar.
//
// This package is the S1′ successor of the G32-blocked scanner (archived at
// .local/coordinator/archives/eslink-g32-blocked/). It is deliberately
// SMALLER than its predecessor: no ASI machinery (family 8 excludes ASI —
// explicit ';' required), no lexer-context slash tracking (regex literals are
// admitted exactly at expression-start positions, decided by parse structure
// — the AMB-2 ratified resolution), no cover grammar (the AMB-1 restricted
// arrow-parameter production), and no await-scope tracking beyond the
// admitted async function/arrow form.
//
// It renders NOTHING into consumer repos, adds no CLI verb, no doctor check,
// no CI hook, and no enforcement wiring: the S1′ scope (ratified restart
// record researches/decisions/2026-11-08-eslink-o4-staged-restart-spec.md
// §B, commit 2cae4a2) is a correctness demonstration under gates only. Like
// the G32 package it requires no bootstrap-inventory decision (internal-only
// test surface).
//
// # The frozen grammar
//
// Ratified eight production families (restart record §A) with exact variants
// frozen after the D pass (tmp/agent-runs/o4-d-pass/variant-map.md, landing
// concurrently at researches/sources/2026-11-08-eslink-o4-grammar-variant-map.md):
//
//	Module      := Hashbang? (ImportDecl | ExportDecl | Stmt)* EOF
//
//	ImportDecl  := 'import' ImportClause FromClause ';'?
//	             | 'import' String ';'?                      (side-effect)
//	FromClause  := 'from' String
//
//	ExportDecl  := 'export' ('default' (HoistableDecl | AssignmentExpr ';'?)
//	             | ClassDecl | FunctionDecl | AsyncFunctionDecl
//	             | ('const'|'let'|'var') Declarators ';'?
//	             | ExportList FromClause? ';'?  | StarExport ';'?)
//
// Statements (family 2/3/8): if/for(classic|of)/while/switch per the
// admitted variants; try/catch/finally with optional catch binding (BINDING
// IDENT only — destructuring degrades); break/continue/return/throw;
// const/let/var declarations (binding patterns admitted outside export
// position); function/class declarations; blocks; expression statements.
// Every simple statement ends with an explicit ';' — no ASI. for-in,
// do-while, labeled statements and for-await are EXCLUDED (D-2): any
// occurrence degrades the module to UNKNOWN, never refusal (U6).
//
// Classes (family 4): 'class' Ident ('extends' LHSExpr)? ClassBody with
// CONSTRUCTOR-FORM members only. #fields, static members, accessors,
// generators, computed keys, class expressions: EXCLUDED (D-2).
//
// Expressions: the corpus's restricted repertoire — primaries (identifiers,
// this/super, literals, templates with recursively parsed ${...}
// substitutions (family 6, no depth cap), regex literals at EXPRESSION-START
// positions only (family 7, AMB-2), arrays/objects with spread, function
// expressions, new, dynamic import(...), import.meta), unary (! ~ - + typeof
// void delete await), the admitted binary operators, ternary, assignment
// (= and compound assignments over ADMITTED arithmetic/bitwise/shift
// operators), member/call postfix incl. '?.'.
// Update operators (++/--), tagged templates, exponent and logical
// assignment operators: NOT admitted (degrade to UNKNOWN).
//
// Arrows (family 1, AMB-1 RESTRICTED parameter production):
//
//	Arrow       := ['async'] (BindingIdent | '(' ParamList? ')') '=>' Body
//	ParamList   := Param (',' Param)* ','?
//	Param       := BindingPattern ('=' Default)? | '...' BindingIdent
//	Default     := AssignmentExpr whose root (through parentheses) is not a
//	               comma/sequence expression (the cover-grammar rejection).
//
// The parameter list is parsed STRUCTURALLY (binding patterns only, no
// cover grammar): a '(' group followed by '=>' whose interior is not a
// valid restricted binding list degrades the module to UNKNOWN.
//
// await (family 5): AwaitExpression := 'await' UnaryExpr, legal at MODULE
// TOP LEVEL and inside async function/arrow bodies (module-top [+Await] per
// the family-5 grammar-freeze AMENDMENT, operator-ratified 2026-11-08 — the
// corpus carries module-top await in 6 modules, all previously misread by a
// line-anchored probe). await inside NON-async function/arrow bodies and
// for-await still degrade.
//
// # The six UNKNOWN-discipline clauses (U1–U6, binding)
//
//	U1 complete-consumption: unsupported or trailing input can never be
//	   skipped — the whole module is UNKNOWN.
//	U2 split-states: import-completeness and export/transitive-closure are
//	   separate states.
//	U3 bytes-not-ancestry: qualification by actual adopter bytes, never by
//	   version/corpus-origin labels.
//	U4 open-surface propagation: unsupported syntax / export * / namespace
//	   re-export open the surface; UNKNOWN propagates to dependent proofs.
//	U5 reason-coded: every UNKNOWN carries a reason.
//	U6 never-refuses: UNKNOWN never authorizes refusal or FAIL.
//
// The graph layer reproduces the reference closure semantics (star-merge,
// cycles → open, ambiguity → unknown, non-cascading re-export attribution)
// that the XS-1…XS-8 fixture designs pin against the G32-era
// graph.go/parser.go.
package eslink

// Reason codes for module-level degradation (U5). Every parse failure
// produces exactly one of these; the first failure wins (U1 — no resumption).
const (
	ReasonUpdateOp     = "update operator (++/--) not admitted"
	ReasonAsiBoundary  = "ASI-dependent boundary (missing ';')"
	ReasonAsiReturn    = "ASI-dependent boundary (newline after restricted keyword)"
	ReasonForIn        = "for-in statement not admitted"
	ReasonForAwait     = "for-await not admitted"
	ReasonDoWhile      = "do-while statement not admitted"
	ReasonLabel        = "labeled statement not admitted"
	ReasonTopAwait     = "await inside non-async function/arrow body" // module top is async (amendment)
	ReasonTaggedTmpl   = "tagged template not admitted"
	ReasonCatchDestr   = "catch destructuring not admitted"
	ReasonArrowDefault = "arrow-param default: top-level comma expression"
	ReasonArrowParams  = "arrow parameters rejected"
	ReasonClassPrivate = "class private field not admitted"
	ReasonClassStatic  = "class static member not admitted"
	ReasonClassAccess  = "class accessor (get/set) not admitted"
	ReasonClassGen     = "class generator not admitted"
	ReasonClassCompute = "class computed key not admitted"
	ReasonClassMethod  = "class member not admitted (constructor-only form)"
	ReasonClassField   = "class field not admitted (constructor-only form)"
	ReasonClassExpr    = "class expression not admitted"
	ReasonDestrExport  = "destructuring export declaration (surface not enumerable)"
	ReasonGenFunc      = "generator function not admitted"
	ReasonUnsupported  = "unsupported syntax"
)
