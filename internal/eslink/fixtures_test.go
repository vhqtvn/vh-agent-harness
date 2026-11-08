package eslink

// Gate-1 discriminating fixtures (staged-restart spec §B gate 1) and gate-7
// export-star fixtures (XS-1…XS-8), both designed by the D-pass variant map
// (researches/sources/2026-11-08-eslink-o4-grammar-variant-map.md @ 2f5b304).
//
// Discriminating convention (map "Verdict vocabulary"): every fixture pins
// the CORRECT verdict; a run reporting a MORE favorable verdict exhibits
// false-ok, a LESS favorable one on admitted input exhibits false-broken.
// The map's admitted-variant tables are the FO checklists — each PRESENT
// variant row appears in exactly one FO fixture.
//
// Family-5 amendment (operator-ratified 2026-11-08): module-top [+Await].
// F5-FB(a) is pinned to its AMENDED verdict (module-top await → FIT); the
// false-broken guard survives via F5-FB(c) (await inside a non-async
// function) and F5-FB(b) (for-await).

import (
	"strings"
	"testing"
)

func fitModule(t *testing.T, name, src string) *Module {
	t.Helper()
	m := Parse(name, []byte(src))
	if m.Unparseable {
		t.Fatalf("%s: expected FIT, got UNKNOWN(%s)", name, m.UnparseableWhy)
	}
	return m
}

func unknownModule(t *testing.T, name, src, reasonSubstr string) *Module {
	t.Helper()
	m := Parse(name, []byte(src))
	if !m.Unparseable {
		t.Fatalf("%s: expected UNKNOWN(%s…), got FIT", name, reasonSubstr)
	}
	if !strings.Contains(m.UnparseableWhy, reasonSubstr) {
		t.Fatalf("%s: expected reason containing %q, got %q", name, reasonSubstr, m.UnparseableWhy)
	}
	return m
}

func requireCleanScan(t *testing.T, r *Report) {
	t.Helper()
	if r.HasBroken() {
		t.Fatalf("expected no broken edges: %+v", r.Broken)
	}
	if len(r.Unknown) != 0 {
		t.Fatalf("expected no unknown findings: %+v", r.Unknown)
	}
}

// --- Family 1: arrows (map table = 13 PRESENT variants) -------------------------

func TestF1_FO_AllArrowVariants(t *testing.T) {
	src := "import { dep } from \"./dep.js\";\n" +
		"const single = e => e && dep;\n" + // single-identifier, no parens
		"const zero = () => 1;\n" + // zero params
		"const multi = (a, b) => a + b;\n" + // multi-param parens
		"const defaulted = (taskID, extra = []) => [taskID, extra];\n" + // default-valued param
		"const rest = (...p) => p.join(\"-\");\n" + // rest param
		"export const server = async ({ client, directory }) => {\n" + // object-destructured + async + block body
		"    const withDefault = ({ mode = \"ro\" } = {}) => mode;\n" + // pattern default
		"    return withDefault({});\n" +
		"};\n" +
		"const pair = ([key, value]) => `${key}: ${value}`;\n" + // array-destructured param
		"const elision = ([, second]) => second;\n" + // array pattern with elision
		"const expr = (a) => a * 2;\n" + // expression body
		"const curried = (a) => (b) => ({ ok: true, value: a + b });\n" + // curried return-arrow
		"const iife = (() => { return 7; })();\n" + // paren-wrapped immediately invoked
		"const mapped = [1, 2].map((x) => x + 1).filter((x) => x > 1);\n" + // arrow as call argument
		"export const total = mapped.length + iife + expr(2) + pair([\"k\", 1]).length;\n"
	m := fitModule(t, ".opencode/plugins/arrows.js", src)
	if !m.NamedExports["server"] || !m.NamedExports["total"] {
		t.Fatalf("expected server+total exported, got %v", m.NamedExports)
	}
	r := Scan(treeOf(map[string]string{
		".opencode/plugins/arrows.js": src,
		".opencode/plugins/dep.js":    "export const dep = 1;\n",
	}))
	requireCleanScan(t, r)
	if r.Edges != 1 {
		t.Fatalf("Edges = %d, want 1", r.Edges)
	}
}

func TestF1_FB_ArrowParamCommaDefault(t *testing.T) {
	// The AMB-1 cover-grammar rejection: a param default whose ROOT,
	// through parens, is a comma/sequence expression.
	unknownModule(t, ".opencode/plugins/cover.js",
		"const f = (a = (b, c)) => a;\n", ReasonArrowDefault)
	// Parenthesized non-sequence defaults stay admitted (only the
	// sequence ROOT is rejected).
	fitModule(t, ".opencode/plugins/cover-ok.js",
		"const g = (a = (b + c)) => a;\nexport const ok = g;\n")
	// The root test COMPOSES: a sequence expression as an OPERAND of a
	// binary/conditional/assignment root is admitted — the root is the
	// operation, not the sequence (a descriptor that propagated the left
	// operand's ekSeq through composition would falsely degrade these —
	// the false-broken direction).
	fitModule(t, ".opencode/plugins/cover-bin.js",
		"const h = (a = (b, c) + d) => a;\nexport const okBin = h;\n")
	fitModule(t, ".opencode/plugins/cover-cond.js",
		"const i = (a = (b, c) ? d : e) => a;\nexport const okCond = i;\n")
	// Unary-root composition (the F1 discriminating case): a sequence
	// expression as the OPERAND of a unary operator is admitted — the
	// root is the unary operation, not the sequence.
	fitModule(t, ".opencode/plugins/cover-unary.js",
		"const u = (a = -(b, c)) => a;\nexport const okUnary = u;\n")
	// Postfix member/call composition: the same principle for '.',
	// '[…]' and '(…)' continuations over a parenthesized-sequence
	// primary — the root is the member/call operation.
	fitModule(t, ".opencode/plugins/cover-member.js",
		"const m = (a = (b, c).x) => a;\nexport const okMember = m;\n")
	fitModule(t, ".opencode/plugins/cover-computed.js",
		"const k = (a = (b, c)[0]) => a;\nexport const okComputed = k;\n")
	fitModule(t, ".opencode/plugins/cover-call.js",
		"const f = (a = (b, c)(x)) => a;\nexport const okCall = f;\n")
	// False-direction counterpart: the sequence AT the root — through
	// two paren layers, with no composing operation — still degrades
	// (cover-grammar rejection; the nested `((…))` group's own
	// binding failure is the recorded cover reason). The unary/postfix
	// compositions admit a sequence OPERAND, never a sequence ROOT
	// (paren transparency is unchanged).
	unknownModule(t, ".opencode/plugins/cover-seqroot2.js",
		"const s = (a = ((b, c))) => a;\n", ReasonArrowParams)
}

// --- Family 2: control flow (map table: 11 PRESENT variants) --------------------

func TestF2_FO_AllControlFlowVariants(t *testing.T) {
	src := "import { seed } from \"./seed.js\";\n" +
		"let acc = seed;\n" +
		"if (acc > 1) {\n" + // if, next-line body (family 8 boundary class)
		"    acc += 1;\n" +
		"} else if (acc > 2) {\n" + // else-if chain
		"    acc += 2;\n" +
		"} else {\n" +
		"    acc += 3;\n" +
		"}\n" +
		"for (let i = 0; i < 3; i += 1) { acc += i; }\n" + // classic for, conformant update
		"for (const item of [1, 2]) { acc += item; }\n" + // for-of plain binding
		"for (const [pattern, action] of [[1, 2]]) { acc += pattern + action; }\n" + // for-of array-destructured
		"for (const [, second] of [[1, 2]]) { acc += second; }\n" + // for-of elision head
		"for (;;) { acc += 1; break; }\n" + // for(;;) + break
		"while (acc > 1000) { acc -= 1; }\n" + // while
		"switch (acc % 3) {\n" +
		"    case 0:\n" +
		"        acc += 1;\n" +
		"        break;\n" + // string-literal case + break
		"    case 1: {\n" +
		"        const [key, val] = [\"k\", 1];\n" + // braced case body with lexical decls
		"        acc += val;\n" +
		"        break;\n" +
		"    }\n" +
		"    case 2:\n" +
		"        acc += 2;\n" + // fallthrough (admitted by the CaseClause Stmt* clause)
		"    default:\n" +
		"        acc += 4;\n" + // default clause
		"}\n" +
		"const matched = /^\"?(\\.[a-z0-9]+)\"?\\s*:(.*)$/.exec(\"x\");\n" + // regex inside case-adjacent expr (operand position)
		"export const out = acc + (matched === null ? 0 : 1);\n"
	fitModule(t, ".opencode/scripts/control.js", src)
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/control.js": src,
		".opencode/scripts/seed.js":    "export const seed = 1;\n",
	}))
	requireCleanScan(t, r)
}

func TestF2_FB_ExcludedControlForms(t *testing.T) {
	// (a) update operator in a for-head — the corpus-real debt class.
	unknownModule(t, ".opencode/scripts/debt.js",
		"for (let i = 0; i < 3; i++) { use(i); }\n", ReasonUpdateOp)
	// (b) labeled statement.
	unknownModule(t, ".opencode/scripts/lbl.js",
		"outer: for (;;) { break outer; }\n", ReasonLabel)
	// (c) for-in.
	unknownModule(t, ".opencode/scripts/forin.js",
		"for (const k in { a: 1 }) { use(k); }\n", ReasonForIn)
	// (d) do-while (absent construct, D-2).
	unknownModule(t, ".opencode/scripts/dowhile.js",
		"do { use(1); } while (false);\n", ReasonDoWhile)
	// (e) object-destructured for-of head — named outside the frozen
	// subset by the freeze record's completeness rule (map:172 "not
	// needed"); array-destructured heads stay admitted (F2-FO).
	unknownModule(t, ".opencode/scripts/forofobj.js",
		"for (const { x } of [{ x: 1 }]) { use(x); }\n", "object-destructured for-head")
}

// TestF2_Repertoire_BinaryIn pins the corpus-forced binary-'in' admission
// (verify-recurrence-dedup.js:544 `!("recurrence" in onDisk)` — a live FIT
// module in the ratified pin set; the operator is on no enumerated exclusion
// list). The for-in STATEMENT rejection stays pinned in F2-FB(c). Known
// corpus-absent permissiveness (documented, unpinned): ES forbids a bare
// 'in' in a classic for-head's first slot; this parser does not model that.
func TestF2_Repertoire_BinaryIn(t *testing.T) {
	src := "const onDisk = { recurrence: 1 };\n" +
		"const ok = !(\"recurrence\" in onDisk);\n" +
		"const two = (\"a\" in onDisk) && (\"b\" in onDisk);\n" +
		"export const out = ok || two;\n"
	fitModule(t, ".opencode/scripts/binin.js", src)
	requireCleanScan(t, Scan(treeOf(map[string]string{
		".opencode/scripts/binin.js": src,
	})))
}

// --- Family 3: try/catch/finally (5 PRESENT variants) ---------------------------

func TestF3_FO_AllTryVariants(t *testing.T) {
	src := "import { probe } from \"./probe.js\";\n" +
		"export const results = [];\n" +
		"try {\n" +
		"    results.push(probe());\n" +
		"} catch (err) {\n" + // try/catch(binding)
		"    results.push(err.message);\n" +
		"}\n" +
		"try {\n" +
		"    results.push(\"bare\");\n" +
		"} catch {\n" + // optional catch binding (the 6-site shell-guard-core form)
		"    results.push(\"swallowed\");\n" +
		"}\n" +
		"try {\n" +
		"    results.push(\"fin\");\n" +
		"} finally {\n" + // try/finally only
		"    results.push(\"cleanup\");\n" +
		"}\n" +
		"try {\n" +
		"    results.push(\"full\");\n" +
		"} catch (err) {\n" + // try/catch/finally
		"    try {\n" + // nested try inside catch (cleanup-retry pattern)
		"        results.push(err);\n" +
		"    } catch (_cleanupError) {\n" +
		"        results.push(\"nested\");\n" +
		"    }\n" +
		"} finally {\n" +
		"    results.push(\"done\");\n" +
		"}\n"
	fitModule(t, ".opencode/plugins/tryforms.js", src)
	r := Scan(treeOf(map[string]string{
		".opencode/plugins/tryforms.js": src,
		".opencode/plugins/probe.js":    "export const probe = () => 1;\n",
	}))
	requireCleanScan(t, r)
}

func TestF3_FB_CatchDestructuring(t *testing.T) {
	unknownModule(t, ".opencode/plugins/catchdestr.js",
		"try { work(); } catch ({ message }) { use(message); }\n", ReasonCatchDestr)
}

// --- Family 4: classes (4 PRESENT variants) --------------------------------------

func TestF4_FO_ClassForms(t *testing.T) {
	// The exact StateError shape (state-lib.js:223-236) + empty derived.
	src := "import { tag } from \"./tag.js\";\n" +
		"export class StateError extends Error {\n" +
		"    constructor(message, cause) {\n" +
		"        super(message);\n" +
		"        if (cause) {\n" +
		"            this.cause = cause;\n" +
		"        }\n" +
		"    }\n" +
		"}\n" +
		"class BacklogError extends Error {}\n" +
		"export function raise(message) {\n" +
		"    throw new StateError(`${tag}: ${message}`);\n" + // new on the declared class
		"}\n"
	m := fitModule(t, ".opencode/scripts/classes.js", src)
	if !m.NamedExports["StateError"] || !m.NamedExports["raise"] {
		t.Fatalf("expected StateError+raise exported, got %v", m.NamedExports)
	}
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/classes.js": src,
		".opencode/scripts/tag.js":     "export const tag = \"t\";\n",
	}))
	requireCleanScan(t, r)
}

func TestF4_FB_ExcludedClassMembers(t *testing.T) {
	// The map's F4-FB shape: #x first, so the private-field reason wins.
	unknownModule(t, ".opencode/scripts/classx.js",
		"class A extends Error { #x = 1; static y = 2; static {} get g() { return 1; } }\n",
		ReasonClassPrivate)
	unknownModule(t, ".opencode/scripts/classs.js",
		"class B { static parse() { return 1; } }\n", ReasonClassStatic)
	unknownModule(t, ".opencode/scripts/classa.js",
		"class C { get g() { return 1; } }\n", ReasonClassAccess)
}

// --- Family 5: await (AMENDED: module-top [+Await]) -------------------------------

func TestF5_FO_PluginAwaitPattern(t *testing.T) {
	src := "import { helper } from \"./helper.js\";\n" +
		"export const server = async ({ client, directory }) => {\n" +
		"    const mod = await import(\"./x.js\");\n" + // dynamic import: UNKNOWN-by-contract
		"    const { a } = await helper();\n" + // await + destructure in async arrow
		"    return { mod, a };\n" +
		"};\n" +
		"export const epilogue = async () => {\n" +
		"    const fs = await import(\"node:fs\");\n" + // module-specifier dynamic import
		"    return fs;\n" +
		"};\n"
	fitModule(t, ".opencode/plugins/awaitplugin.js", src)
	r := Scan(treeOf(map[string]string{
		".opencode/plugins/awaitplugin.js": src,
		".opencode/plugins/helper.js":      "export const helper = async () => ({ a: 1 });\n",
	}))
	if r.HasBroken() {
		t.Fatalf("static edge must be ok: %+v", r.Broken)
	}
	// Exactly ONE dynamic-import unknown: the RELATIVE ./x.js. The bare
	// node:fs dynamic import is out of graph entirely (no finding) — the
	// G32 bare-specifier contract.
	if len(r.Unknown) != 1 {
		t.Fatalf("expected exactly 1 dynamic-import unknown (relative only), got %+v", r.Unknown)
	}
	if u := r.Unknown[0]; u.Path != ".opencode/plugins/awaitplugin.js" || u.Specifier != "./x.js" || !strings.Contains(u.Reason, "dynamic import") {
		t.Fatalf("unexpected dynamic unknown shape: %+v", u)
	}
}

func TestF5_FB_AwaitBoundaries(t *testing.T) {
	// (a) AMENDED verdict: module-top await is ADMITTED (module-top
	// [+Await], operator-ratified 2026-11-08). Pre-amendment this was
	// UNKNOWN(top-level await); the amendment flipped it — pinned FIT.
	fitModule(t, ".opencode/plugins/tla.js",
		"const mod = await import(\"./optional-panel.js\");\nexport const ready = mod;\n")
	fitModule(t, ".opencode/plugins/tla-try.js",
		"try { await main(); } catch (err) { use(err); }\nasync function main() { return 1; }\n")
	// (b) for-await still excluded (D-2).
	unknownModule(t, ".opencode/plugins/forawait.js",
		"async function f(stream) { for await (const c of stream) { use(c); } }\n", ReasonForAwait)
	// (c) await inside a NON-async function — the retained false-broken
	// guard (weakened only for the module-top case).
	unknownModule(t, ".opencode/plugins/nonasync.js",
		"function f(x) { return await x; }\n", ReasonTopAwait)
}

// --- Family 6: templates (recursive substitution; no depth cap) ------------------

func TestF6_FO_TemplatePatterns(t *testing.T) {
	// The check-s2-holds.mjs:678 shape verbatim-in-form: map-arrow
	// returning a nested template, joined; multi-line template.
	src := "import { ev } from \"./ev.js\";\n" +
		"const t = ev.map((r) => `${r.file}:${r.line}`).join(\", \");\n" + // depth 2
		"const c = `${ev.length ? `: ${t}` : \"\"}`;\n" + // nested-in-conditional-in-substitution
		"const m = `line one\nline two ${t}\nline three`;\n" + // multi-line
		"export const out = `${c} ${m}`;\n" // depth 2 at top
	fitModule(t, ".opencode/scripts/tmpl.js", src)
	r := Scan(treeOf(map[string]string{
		".opencode/scripts/tmpl.js": src,
		".opencode/scripts/ev.js":   "export const ev = [{ file: \"a\", line: 1 }];\n",
	}))
	requireCleanScan(t, r)
}

func TestF6_FB_TemplateBoundaries(t *testing.T) {
	// (a) tagged template (D-2 exclusion).
	unknownModule(t, ".opencode/scripts/tagged.js",
		"const y = 1;\nconst t = tag`x${y}`;\n", ReasonTaggedTmpl)
	// (b) depth-3 recursion guard: the production is RECURSIVE, never
	// depth-capped (map F6-FB(b)) — degrading this is a false depth cap.
	fitModule(t, ".opencode/scripts/depth3.js",
		"const x = 1;\nconst d = `a${`b${`c${x}`}d`}e`;\nexport const out = d;\n")
}

// --- Family 7: regex at expression-start positions (9 prefix contexts) ----------

func TestF7_FO_AllNinePrefixContexts(t *testing.T) {
	src := "import { s } from \"./s.js\";\n" +
		"const a = wrap(/^[a-z/]+$/.test(s));\n" + // '(' call arg
		"const b = pair(1, /a|b\\/c/.test(s));\n" + // ',' second arg + '|' inside pattern
		"const c = /SKIP_X\\s*=\\s*1(\\s|$/.test(s);\n" + // '=' init + '=' inside pattern + '/' inside class
		"const obj = { pattern: /zai-coding-plan/i, other: 1 };\n" + // ':' property value
		"const arrow = (t) => /secretrepo/i.test(t);\n" + // '=>' arrow body
		"const and = /structural/i.test(s) && /truth/i.test(s);\n" + // '&' && RHS (both operands)
		"const not = !/reload(ed)?|auto-reload/i.test(s);\n" + // '!' unary operand
		"function ret() {\n" +
		"    return /^[A-Za-z0-9._/:+-]+$/.test(s) ? 1 : 0;\n" + // 'return' + '/' inside class
		"}\n" +
		"const or = s instanceof Error || /SyntaxError|Unexpected/.test(s);\n" + // '|' || RHS
		"export const all = [a, b, c, arrow(s), and, not, ret(), or].length + Object.keys(obj).length;\n"
	fitModule(t, ".opencode/plugins/regexes.js", src)
	r := Scan(treeOf(map[string]string{
		".opencode/plugins/regexes.js": src,
		".opencode/plugins/s.js":       "export const s = \"x\";\n",
	}))
	requireCleanScan(t, r)
}

func TestF7_FB_SlashDoubleGuard(t *testing.T) {
	// Module A (the freeze's sharpest discriminator): the FIRST slash pair
	// must lex as regex (expression start after '=>'), the SECOND as
	// division. Misclassifying either direction degrades the module.
	srcA := "const q = (t) => /x/.test(t);\n" +
		"const a = 8;\n" +
		"const b = 2;\n" +
		"const c = 4;\n" +
		"const d = a / b / c;\n" +
		"export const out = q(\"x\") ? d : 0;\n"
	fitModule(t, ".opencode/plugins/slashA.js", srcA)
	requireCleanScan(t, Scan(treeOf(map[string]string{
		".opencode/plugins/slashA.js": srcA,
	})))
	// Statement-initial regex (admitted-but-corpus-absent, D-3): an
	// expression start is an expression start.
	srcS := "const s = \"x\";\n/[{]/.test(s);\nexport const ok = 1;\n"
	fitModule(t, ".opencode/plugins/slashS.js", srcS)

	// Module B (U5/U6): a '/' the lexer cannot complete is a reason-coded
	// UNKNOWN, never broken, never refusal.
	srcB := "import { dep } from \"./dep.js\";\nconst r = /x\nexport const out = dep;\n"
	m := unknownModule(t, ".opencode/plugins/slashB.js", srcB, "unterminated regex")
	_ = m
	r := Scan(treeOf(map[string]string{
		".opencode/plugins/slashB.js": srcB,
		".opencode/plugins/dep.js":    "export const dep = 1;\n",
	}))
	if r.HasBroken() {
		t.Fatalf("undecidable slash must never ground broken (U6): %+v", r.Broken)
	}
	if findUnknownContaining(r, ".opencode/plugins/slashB.js", "module UNKNOWN") == nil {
		t.Fatalf("expected module-UNKNOWN finding: %+v", r.Unknown)
	}
}

// --- Family 8: explicit termination (both boundary classes) ----------------------

func TestF8_FO_BoundaryClasses(t *testing.T) {
	src := "import { v } from \"./v.js\";\n" +
		"let acc = v;\n" +
		"if (acc > 0)\n" + // if-head, body on next line (control-head continuation)
		"    acc += 1;\n" +
		"const block = () => {\n" +
		"    return acc;\n" +
		"}\n" + // '}' → next statement (block-close boundary)
		"export const out = block();\n"
	fitModule(t, ".opencode/plugins/term.js", src)
	r := Scan(treeOf(map[string]string{
		".opencode/plugins/term.js": src,
		".opencode/plugins/v.js":    "export const v = 1;\n",
	}))
	requireCleanScan(t, r)
}

func TestF8_FB_AsiBoundary(t *testing.T) {
	unknownModule(t, ".opencode/plugins/asi.js",
		"const a = 1\nconst b = 2;\n", ReasonAsiBoundary)
	// Restricted production: newline after return.
	unknownModule(t, ".opencode/plugins/asi2.js",
		"function f() {\n    return\n    1;\n}\n", ReasonAsiReturn)
	// The strict side of the F8-FO exemption: an OBJECT-LITERAL declarator
	// is not a block body — the explicit ';' is still required.
	unknownModule(t, ".opencode/plugins/asi3.js",
		"const o = { a: 1 }\nconst p = 2;\n", ReasonAsiBoundary)
	// A bare 'return' before the ENCLOSING block's '}' is not a statement
	// "ending in '}' — the terminator rule binds (the block exemption
	// covers statements that end in their own block close).
	unknownModule(t, ".opencode/plugins/asi4.js",
		"function f() { return }\nexport const x = 1;\n", ReasonAsiBoundary)
	// Bare 'throw' requires an expression (invalid ES otherwise).
	unknownModule(t, ".opencode/plugins/asi5.js",
		"function f() { throw; }\nexport const x = 1;\n", "throw requires an expression")
	unknownModule(t, ".opencode/plugins/asi6.js",
		"function f() { throw }\nexport const x = 1;\n", "throw requires an expression")
}

// --- Gate 7: XS-1…XS-8 export-star fixtures (map PART 2 pinned verdicts) ---------

func xsTree(files map[string]string) map[string][]byte { return treeOf(files) }

func TestXS1_TransitiveClosureClosedAndOpenTerminal(t *testing.T) {
	// (a) all terminal closed.
	base := map[string]string{
		".opencode/a.js":               "export const x = 1;\nexport const helper = () => 2;\n",
		".opencode/b.js":               "export * from \"./a.js\";\n",
		".opencode/c.js":               "export * from \"./b.js\";\n",
		".opencode/importer-closed.js": "import { x } from \"./c.js\";\nimport { helper } from \"./c.js\";\nimport { missing } from \"./c.js\";\n",
	}
	r := Scan(xsTree(base))
	if r.HasBroken() && findBroken(r, ".opencode/importer-closed.js", "x") != nil {
		t.Fatalf("x must be ok through the chain (anti-degrade): %+v", r.Broken)
	}
	if findBroken(r, ".opencode/importer-closed.js", "helper") != nil {
		t.Fatalf("helper must be ok through the chain: %+v", r.Broken)
	}
	if b := findBroken(r, ".opencode/importer-closed.js", "missing"); b == nil {
		t.Fatalf("closed chain proves absence (iii): %+v / %+v", r.Broken, r.Unknown)
	} else if !strings.Contains(b.Reason, "surface closed, so the absence is proven") {
		t.Fatalf("broken must cite proven absence: %+v", b)
	}
	if len(r.Unknown) != 0 {
		t.Fatalf("closed chain yields no unknowns: %+v", r.Unknown)
	}

	// (b) terminal OPEN variant: a.js gets an update-op token.
	open := map[string]string{
		".opencode/a.js":               "export const x = 1;\nexport const helper = () => 2;\nlet i = 0; i++;\n",
		".opencode/b.js":               "export * from \"./a.js\";\n",
		".opencode/c.js":               "export * from \"./b.js\";\n",
		".opencode/importer-closed.js": "import { x } from \"./c.js\";\nimport { helper } from \"./c.js\";\nimport { missing } from \"./c.js\";\n",
	}
	r2 := Scan(xsTree(open))
	if r2.HasBroken() {
		t.Fatalf("an open chain can NEVER ground broken (false-ok guard): %+v", r2.Broken)
	}
	// Every importer edge — including the (iii) absence probe — is unknown.
	uk := 0
	for _, u := range r2.Unknown {
		if u.Path == ".opencode/importer-closed.js" && strings.Contains(u.Reason, "open export surface") {
			uk++
		}
	}
	if uk != 3 {
		t.Fatalf("expected 3 open-surface unknowns on the importer, got %d: %+v", uk, r2.Unknown)
	}
	// The chain relays degrade per-hop (U5).
	if findUnknownContaining(r2, ".opencode/b.js", "open surface") == nil ||
		findUnknownContaining(r2, ".opencode/c.js", "open surface") == nil {
		t.Fatalf("star relays must degrade reason-coded per hop: %+v", r2.Unknown)
	}
}

func TestXS2_ReexportCycleOpens(t *testing.T) {
	r := Scan(xsTree(map[string]string{
		".opencode/cyc-a.js":        "export * from \"./cyc-b.js\";\nexport const a1 = 1;\n",
		".opencode/cyc-b.js":        "export * from \"./cyc-a.js\";\nexport const b1 = 2;\n",
		".opencode/importer-cyc.js": "import { a1 } from \"./cyc-a.js\";\n",
	}))
	if r.HasBroken() {
		t.Fatalf("cycle must never ground broken: %+v", r.Broken)
	}
	if findUnknownContaining(r, ".opencode/importer-cyc.js", "open export surface") == nil {
		t.Fatalf("even the LOCALLY declared a1 must be unprovable through the cycle: %+v", r.Unknown)
	}
	// The cyclic detail is pinned at the relay level (reference graph.go
	// star-target reason); the importer gets the open-surface shape.
	if findUnknownContaining(r, ".opencode/cyc-a.js", "cyclic") == nil {
		t.Fatalf("the relay star finding must name the cycle: %+v", r.Unknown)
	}
}

func TestXS3_AmbiguityShadowAndDuplicateStars(t *testing.T) {
	r := Scan(xsTree(map[string]string{
		".opencode/amb-x.js":         "export const v = 1;\nexport const onlyX = 1;\n",
		".opencode/amb-y.js":         "export const v = 2;\n",
		".opencode/amb-m.js":         "export * from \"./amb-x.js\";\nexport * from \"./amb-y.js\";\n",
		".opencode/importer-amb.js":  "import { v } from \"./amb-m.js\";\nimport { onlyX } from \"./amb-m.js\";\n",
		".opencode/amb-m2.js":        "export const v = 3;\nexport * from \"./amb-x.js\";\nexport * from \"./amb-y.js\";\n",
		".opencode/importer-amb2.js": "import { v } from \"./amb-m2.js\";\n",
		".opencode/amb-dup.js":       "export * from \"./amb-x.js\";\nexport * from \"./amb-x.js\";\n",
		".opencode/importer-dup.js":  "import { onlyX } from \"./amb-dup.js\";\n",
	}))
	// (i) ambiguous v — unknown, never ok (fabricated provenance).
	if findUnknownContaining(r, ".opencode/importer-amb.js", "ambiguously") == nil {
		t.Fatalf("ambiguous v must be unknown: %+v", r.Unknown)
	}
	if findBroken(r, ".opencode/importer-amb.js", "v") != nil {
		t.Fatalf("ambiguous v must not be broken: %+v", r.Broken)
	}
	// (ii) onlyX single-sourced through the merge — ok.
	if findBroken(r, ".opencode/importer-amb.js", "onlyX") != nil {
		t.Fatalf("onlyX is single-sourced: %+v / %+v", r.Broken, r.Unknown)
	}
	// (iii) local declaration shadows every star source — ok, not ambiguous.
	if findBroken(r, ".opencode/importer-amb2.js", "v") != nil {
		t.Fatalf("local v shadows the stars: %+v", r.Broken)
	}
	// (iv) duplicate stars of the SAME target stay ok.
	if findBroken(r, ".opencode/importer-dup.js", "onlyX") != nil {
		t.Fatalf("duplicate same-target stars are not ambiguity: %+v / %+v", r.Broken, r.Unknown)
	}
}

func TestXS4_StarFromMissingProvider(t *testing.T) {
	r := Scan(xsTree(map[string]string{
		".opencode/star-missing.js":     "export * from \"./gone.js\";\n",
		".opencode/importer-missing.js": "import { anything } from \"./star-missing.js\";\n",
	}))
	if r.HasBroken() {
		t.Fatalf("missing star target must never be broken (false-broken guard): %+v", r.Broken)
	}
	if findUnknownContaining(r, ".opencode/star-missing.js", "star re-export target .opencode/gone.js is not present in the scanned tree") == nil {
		t.Fatalf("phase-1 star-missing reason must be pinned: %+v", r.Unknown)
	}
	if findUnknownContaining(r, ".opencode/importer-missing.js", "open export surface") == nil {
		t.Fatalf("missing star target OPENS the surface: %+v", r.Unknown)
	}
}

func TestXS5_StarAndNamedReexportMix(t *testing.T) {
	// (i) all ok.
	r := Scan(xsTree(map[string]string{
		".opencode/mix-a.js":        "export const s1 = 1;\n",
		".opencode/mix-b.js":        "export const s2 = 2;\n",
		".opencode/mix-m.js":        "export * from \"./mix-a.js\";\nexport { s2 } from \"./mix-b.js\";\nexport { s2 as s2renamed } from \"./mix-b.js\";\n",
		".opencode/importer-mix.js": "import { s1, s2, s2renamed } from \"./mix-m.js\";\n",
	}))
	if r.HasBroken() || len(r.Unknown) != 0 {
		t.Fatalf("(i) all-mix must resolve: broken=%+v unknown=%+v", r.Broken, r.Unknown)
	}
	// (ii) mix-b2 lacks s2: the re-export edge is PROVEN broken while the
	// star-provided s1 path stays ok (U2 state separation).
	r2 := Scan(xsTree(map[string]string{
		".opencode/mix-a.js":         "export const s1 = 1;\n",
		".opencode/mix-b2.js":        "export const other = 2;\n",
		".opencode/mix-m2.js":        "export * from \"./mix-a.js\";\nexport { s2 } from \"./mix-b2.js\";\n",
		".opencode/importer-mix2.js": "import { s1 } from \"./mix-m2.js\";\n",
	}))
	if b := findBroken(r2, ".opencode/mix-m2.js", "s2"); b == nil {
		t.Fatalf("missing re-export name is PROVEN broken on the re-exporter: %+v / %+v", r2.Broken, r2.Unknown)
	} else if !strings.Contains(b.Reason, "module re-exports") {
		t.Fatalf("re-export broken reason must be pinned: %+v", b)
	}
	if findBroken(r2, ".opencode/importer-mix2.js", "s1") != nil {
		t.Fatalf("a local re-export defect must NOT open the whole surface: %+v", r2.Broken)
	}
	if findUnknownContaining(r2, ".opencode/importer-mix2.js", "open export surface") != nil {
		t.Fatalf("surface stays closed (U2): %+v", r2.Unknown)
	}
}

func TestXS6_NamespaceReexportAsymmetry(t *testing.T) {
	// (i)+(ii): ns present, inner NOT re-exported by the ns form.
	r := Scan(xsTree(map[string]string{
		".opencode/ns-a.js":        "export const inner = 1;\n",
		".opencode/ns-m.js":        "export * as ns from \"./ns-a.js\";\n",
		".opencode/importer-ns.js": "import { ns } from \"./ns-m.js\";\nimport { inner } from \"./ns-m.js\";\n",
	}))
	if findBroken(r, ".opencode/importer-ns.js", "ns") != nil || findUnknownContaining(r, ".opencode/importer-ns.js", "open export surface") != nil {
		t.Fatalf("(i) ns itself IS an export name, surface closed: %+v / %+v", r.Broken, r.Unknown)
	}
	if b := findBroken(r, ".opencode/importer-ns.js", "inner"); b == nil {
		t.Fatalf("(ii) inner is not in ns-m's surface — proven broken: %+v / %+v", r.Broken, r.Unknown)
	}
	// (iii) target missing: presence edge unknown, but the surface STAYS
	// CLOSED with ns present (the star/ns asymmetry — sharpest case).
	r2 := Scan(xsTree(map[string]string{
		".opencode/ns-m2.js":        "export * as ns from \"./ns-gone.js\";\n",
		".opencode/importer-ns2.js": "import { ns } from \"./ns-m2.js\";\n",
	}))
	if findUnknownContaining(r2, ".opencode/ns-m2.js", "namespace re-export target .opencode/ns-gone.js is not present in the scanned tree") == nil {
		t.Fatalf("(iii) presence-edge unknown must be pinned: %+v", r2.Unknown)
	}
	if findBroken(r2, ".opencode/importer-ns2.js", "ns") != nil {
		t.Fatalf("(iii) importer ns edge stays ok: %+v", r2.Broken)
	}
	if findUnknownContaining(r2, ".opencode/importer-ns2.js", "open export surface") != nil {
		t.Fatalf("(iii) ns-m2 surface stays CLOSED: %+v", r2.Unknown)
	}
}

func TestXS7_WholeChainDegradationAndContrast(t *testing.T) {
	bad := map[string]string{
		".opencode/bad.js":            "export const x = 1;\nlet i = 0; i++;\n",
		".opencode/chain1.js":         "export * from \"./bad.js\";\n",
		".opencode/chain2.js":         "export * from \"./chain1.js\";\n",
		".opencode/importer-chain.js": "import { x } from \"./chain2.js\";\n",
	}
	r := Scan(xsTree(bad))
	if r.HasBroken() {
		t.Fatalf("whole-chain degradation must never ground broken (U6): %+v", r.Broken)
	}
	if findUnknownContaining(r, ".opencode/bad.js", ReasonUpdateOp) == nil {
		t.Fatalf("bad.js degrades with the update-op reason (U5): %+v", r.Unknown)
	}
	if findUnknownContaining(r, ".opencode/chain1.js", "star re-export target .opencode/bad.js has an open surface") == nil {
		t.Fatalf("chain1 pins the per-hop reason (U4+U5): %+v", r.Unknown)
	}
	if findUnknownContaining(r, ".opencode/chain2.js", "star re-export target .opencode/chain1.js has an open surface") == nil {
		t.Fatalf("chain2 pins the per-hop reason one hop up: %+v", r.Unknown)
	}
	if findUnknownContaining(r, ".opencode/importer-chain.js", "provider .opencode/chain2.js has an open export surface") == nil {
		t.Fatalf("the far consumer degrades (U4 propagation): %+v", r.Unknown)
	}

	// Contrast control: the sanctioned mechanical rewrite flips FOUR
	// modules' verdicts — the whole chain closes.
	fixed := map[string]string{}
	for k, v := range bad {
		fixed[k] = v
	}
	fixed[".opencode/bad.js"] = "export const x = 1;\nlet i = 0; i += 1;\n"
	r2 := Scan(xsTree(fixed))
	if r2.HasBroken() || len(r2.Unknown) != 0 {
		t.Fatalf("one-token delta must close the whole chain: broken=%+v unknown=%+v", r2.Broken, r2.Unknown)
	}
}

func TestXS8_PlainClosureNoStars(t *testing.T) {
	r := Scan(xsTree(map[string]string{
		".opencode/plain-a.js":        "export const x = 1;\n",
		".opencode/plain-m.js":        "import { x } from \"./plain-a.js\";\nexport const y = x + 1;\nexport { y as y2 };\n",
		".opencode/importer-plain.js": "import { y, y2 } from \"./plain-m.js\";\nimport { missing } from \"./plain-a.js\";\n",
	}))
	if r.HasBroken() && findBroken(r, ".opencode/importer-plain.js", "y") != nil {
		t.Fatalf("y must be ok: %+v", r.Broken)
	}
	if findBroken(r, ".opencode/importer-plain.js", "y2") != nil {
		t.Fatalf("y2 must be ok (renamed local re-export): %+v", r.Broken)
	}
	if b := findBroken(r, ".opencode/importer-plain.js", "missing"); b == nil {
		t.Fatalf("absence probe must be proven broken: %+v / %+v", r.Broken, r.Unknown)
	}
	if len(r.Unknown) != 0 {
		t.Fatalf("star-free closed tree yields no unknowns: %+v", r.Unknown)
	}
}
