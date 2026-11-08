# O4 D-pass: Variant-to-Production Map + Synthetic Export-Star Fixture Designs

> **STAGING NOTE (header-required):** this artifact is staged at
> `tmp/agent-runs/o4-d-pass/variant-map.md`. Landing it durably under
> `researches/sources/` is a LATER BUILD SLICE — this file is not yet durable
> repo guidance. It is EVIDENCE DESIGN for the pre-freeze gate: mappings,
> fixture cases, and expected verdicts only.
>
> **Ratified boundaries honored:** NO parser implementation, NO new census,
> NO corpus edits. The construct counts below are quoted from the ratified O3
> measurement packet (its census instruments), not re-derived; the per-variant
> SOURCE SPANS are fresh corpus reads (rg probes + file reads) done in this
> pass, which is evidence-gathering, not a census.

- **Date:** 2026-11-08
- **Type:** source packet (evidence design; NOT a decision memo — the only
  recommendations herein are *fixture verdict expectations* and *named
  ratification items*, which the freeze owner must ratify or reject)
- **Reporter class:** read-only researcher (D-pass, ratified commission
  2026-11-08: "Commission a bounded pre-freeze pass covering material
  sub-construct variants and synthetic export-star fixture design, with source
  mappings and expected verdicts, but no parser implementation.")
- **Time-sensitivity:** LOW for the corpus-byte facts (bound to working tree
  2026-11-08; inventory cross-checked against the O3 packet's `ledger.json`
  module list — exact match, 61 modules); MEDIUM for the grammar-candidate
  framing (brief.md:59-81 is the living candidate text)
- **Source policy:** repo-first. Primary sources: (1) the corpus bytes under
  `templates/core/.opencode/` + `templates/overlays/` (the 61 modules), (2)
  the ratified measurement packet
  `researches/sources/2026-11-08-eslink-o4-corpus-conformance-closure-coverage.md`
  (§A ledger, §B census), (3) the grammar candidate
  `tmp/agent-runs/eslink-o4-f2-disposition/brief.md:59-81`, (4) the reference
  closure semantics as preserved verbatim in the packet's measurement harness
  at `tmp/eslink-o4-conformance-packet/harness/eslink/{graph,parser}.go`
  (byte-identical copies of the S1 reference implementation; used read-only
  for semantics grounding of PART 2 expected verdicts). No external sources —
  the question is entirely about this repo's corpus and ratified contracts.

## The six UNKNOWN clauses (binding; referenced as U1–U6 throughout)

| # | Clause (as ratified in the dispatch context) | Short tag |
|---|---|---|
| U1 | Complete consumption or UNKNOWN — unsupported/trailing input cannot be skipped; the whole module is UNKNOWN | complete-consumption |
| U2 | Import completeness and export closure are SEPARATE states (an importer can be import-complete while its own export surface is open, and vice versa) | split-states |
| U3 | Adopter bytes, not ancestry — old/out-of-subset modules are UNKNOWN by their actual bytes; never gain closure via version/corpus-origin labels | bytes-not-ancestry |
| U4 | `export *` / namespace re-export → open surface → UNKNOWN propagates to dependent proofs | open-surface-propagation |
| U5 | Every UNKNOWN is reason-coded | reason-coded |
| U6 | UNKNOWN never refuses (warning surface, never FAIL/refusal) | never-refuses |

**Verdict vocabulary used in fixture expectations.** Module level: `FIT`
(fully consumed; import surface assessable) or `UNKNOWN(reason)` (U1). Edge
level: `ok` (link proven present), `broken(proven)` (surface closed, absence
proven), `unknown(reason)`. **Discriminating-fixture convention:** each
fixture pins the CORRECT verdict; a checker run that reports a *more favorable*
verdict than expected exhibits the **false-ok** direction (missed
degradation/defect), and a run reporting a *less favorable* verdict on
admitted input exhibits the **false-broken** direction (over-strict
refusal/degradation). Any deviation from the pinned verdict fails the fixture.

## Corpus inventory binding (this pass)

61 modules = 50 core (`templates/core/.opencode/**`) + 11 overlay
(`templates/overlays/**`, keyed by rendered destination). Verified this pass:
`jq -r '.modules[].path' tmp/eslink-o4-conformance-packet/ledger.json` lists
exactly the 50 core files found by `find templates/core/.opencode` (9 plugins
incl. `shell-guard/eval.js`, 5 repo-configs, 34 scripts incl. 2 `.mjs`,
1 sys-scripts, 1 tools) plus the 11 overlay modules at rendered keys
(auto-classifier-pilot ×5, session-progress-pilot ×4 incl. the three
`session-progress-*.js` that render from the pack, repo-mail ×1,
contract-invariant-audit-pilot ×1 via
`skills/contract-invariant-audit/scripts/manifest-helper.js`). **Zero
destination collisions, zero `export *`, zero `import * as`, zero namespace
re-exports** (packet §A; re-verified by rg this pass: `export\s*\*` → no
matches).

---

# PART 1 — Variant-to-production map (8 families)

Method note: family footprint numbers are the packet's census (§B), quoted.
Variant spans are fresh rg-probe/read evidence from this pass. Every ABSENT
determination is probe-based (regex over the 61 files) and carries the probe
pattern; absence is asserted only at that strength. Where a family's sites are
ubiquitous, variants are evidenced by representative spans, not exhaustive
lists (bounded pass, per the ratified timebox).

## Family 1 — Arrow functions (packet: 48/61 modules, 1222 arrows)

**Proposed production clause (freeze candidate):**
`Arrow := [async] ArrowParams '=>' ArrowBody`; `ArrowParams := BindingIdent |
'(' ParamList? ')'`; `ParamList := Param (',' Param)* ','?`; `Param :=
BindingPattern ('=' AssignmentExpr)? | '...' BindingIdent`; `ArrowBody :=
AssignmentExpr | '{' BlockStmts '}'`. **Restriction (see AMB-1):** param
default initializers are AssignmentExpr WITHOUT a top-level comma expression,
and ParamList entries are binding patterns only (no arbitrary expressions) —
the corpus-constrained arrow-param sub-grammar.

| Variant | Status | Source span(s) (corpus evidence) | Clause that admits it |
|---|---|---|---|
| single-identifier, no parens | PRESENT | `scripts/state-lib.js:833` (`entries.filter(e => e.scope && …)`) | `ArrowParams := BindingIdent` |
| zero params `() =>` | PRESENT | `overlays/auto-classifier-pilot/plugins/auto-gate-live.js` (135 sites), `auto-tool-gate.js` (133), `auto-gate-scrub.js` (53) — counts by `rg -c '\(\s*\)\s*=>'` | `'(' ParamList? ')'` with empty list |
| multi-param parens | PRESENT | `plugins/session-state.js:283` (`"shell.env": async (input, output) => {`) | `'(' ParamList ')'` |
| default-valued param | PRESENT | `scripts/verify-task-registry.js:824,1178` (`(taskID, extra = []) => [`) | `Param := BindingPattern ('=' AssignmentExpr)?` |
| rest param | PRESENT | `scripts/verify-release-interaction-contract.js:62-63` (`(...p) => join(…)`), `scripts/state-lib.js:1893`, `overlays/…/auto-gate-live.js:2253` (`async (...a) => {`) | `Param := '...' BindingIdent` |
| object-destructured param | PRESENT | `plugins/session-state.js:225`, `plugins/compaction-primitives.js:7`, `plugins/coordination-hints.js:32` (all `async ({ client, directory }) => {` — the plugin export pattern), `overlays/…/session-progress.js:448` (`= {}` default on the pattern) | BindingPattern = ObjectBindingPattern |
| array-destructured param | PRESENT | `scripts/state-lib.js:3364,4258` (`([key, value]) => \`${key}: …\``) | BindingPattern = ArrayBindingPattern |
| array pattern with elision | PRESENT | `scripts/state-lib.js:4257` (`.filter(([, value]) => value !== undefined …)`) | ArrayBindingPattern with Elision (`,` as first element) |
| async arrow | PRESENT | `plugins/session-state.js:283,289`; `overlays/…/auto-gate-live.js:2253` | `[async]` prefix |
| expression body | PRESENT | `scripts/state-lib.js:833` | `ArrowBody := AssignmentExpr` |
| block body | PRESENT | `plugins/session-state.js:225-…` | `ArrowBody := Block` |
| arrow returned from function/arrow (curried) | PRESENT | `overlays/…/auto-gate-live.js:1471,1487,1761` (`return () => ({ ok: true, … })`, `return async () => ({`) | expression grammar nests arrows |
| arrow as call argument (callback) | PRESENT | `scripts/state-lib.js:3364` (`.map(([key, value]) => …)`); every `.map/.filter/.join` chain | expression grammar nests arrows |
| paren-wrapped, immediately invoked (IIFE arrow) | PRESENT | `scripts/verify-f3-design-readiness.js:631,696` (`resolution: (() => { … })()`) | CallExpr with parenthesized Arrow arg |

**Discriminating fixtures (both false directions).**

- **F1-FO (false-ok guard — must be FIT/ok, never degraded):** a module
  containing one of EACH present variant above (no-paren, `() =>`, defaults,
  rest, both pattern kinds incl. elision, async, both bodies, curried) plus a
  closed export surface. Expected: module `FIT`; its export edge `ok`. A run
  returning `UNKNOWN` on any variant = **false-broken** (over-strict grammar).
- **F1-FB (false-broken guard — must be UNKNOWN, never silently FIT):** a
  module with `const f = (a = (b, c)) => a;` — an arrow param default whose
  initializer is a top-level COMMA EXPRESSION. Under the restricted param
  production (AMB-1) this is outside the sub-grammar → module `UNKNOWN`,
  reason `arrow-param default: top-level comma expression` (U1+U5; U6: warn
  only). A run returning `FIT` = **false-ok** (the parser silently accepted a
  cover-grammar form the freeze did not ratify — the exact G32-class
  misclassification surface).

### ⛔ AMB-1 (FREEZE BLOCKER until ratified): arrow cover grammar

`(` after an expression-prefix token is dual-role in JS: parenthesized
expression OR arrow parameter list (the spec solves this with a cover grammar
+ reparse). A "simple production clause" cannot decide `f(a, b)` (call) vs
  `(a, b) => …` (params) on local tokens alone. **This pass's finding:** every
  one of the corpus's 1222 arrows has a parameter list that is exactly
  `BindingIdent | ObjectPattern | ArrayPattern | '...' BindingIdent`, with
  defaults that are side-effect-free initializer expressions, rest in final
  position — i.e. a RESTRICTED arrow-param production cleanly admits the whole
  corpus corpus and never needs the cover grammar (no corpus arrow param is
  ambiguous with a parenthesized expression given the `=>` lookahead).
  **Disposition per the ratified material-ambiguity rule:** this is flagged
  AMBIGUOUS — the freeze is BLOCKED until the operator ratifies the restricted
  arrow-param production (or explicitly ratifies a cover-grammar approach,
  which is strictly more complex). The restriction text above is the proposed
  resolution; it is NOT silently resolved here.

## Family 2 — Control flow: if / for / while / switch (packet: 55/61; switch 7/61)

Switch files (7, verified): `tools/plan-state.js`,
`overlays/…/auto-gate-live.js`, `scripts/complexity-signal-lib.js`,
`scripts/state-lib.js`, `scripts/check-defer-triggers.mjs`,
`scripts/normalize-backlog.js`, `scripts/verify-task-registry.js`.

**Proposed production clause:** `IfStmt := 'if' '(' Expr ')' Stmt ('else'
Stmt)?`; `ForStmt := 'for' '(' (ForClassic | ForOf | <empty in/cond/update>)
')' Stmt` with `ForClassic := VarDecl ';' Expr? ';' Expr?` (update expression
EXCLUDING `++`/`--` per the grammar text), `ForOf := VarDecl 'of' Expr`;
`WhileStmt := 'while' '(' Expr ')' Stmt`; `SwitchStmt := 'switch' '(' Expr ')'
'{' CaseClause* '}'`, `CaseClause := ('case' Expr | 'default') ':' Stmt*`.
NOTE: `ForIn`, `DoWhile`, labels, and `for await` are NOT admitted (absent
from corpus — see below); ratifying their exclusion is DECISION D-2.

| Variant | Status | Source span(s) | Clause |
|---|---|---|---|
| if / else-if / else | PRESENT | `scripts/state-lib.js:5708-5710` (else-if chain); if-head with next-line body: `state-lib.js:4541-4544`, `verify-coordination-hints.js:294-297` (packet §B cites, re-read this pass) | `IfStmt` |
| classic for, conformant update `i += 1` | PRESENT | `overlays/…/manifest-helper.js:530,587,946`; `scripts/normalize-backlog.js:31` (`index += 1`) | `ForClassic` |
| classic for, `++` update | PRESENT but **EXCLUDED** (corpus debt) | 70 sites / 15 files, packet §B table (e.g. `complexity-signal-lib.js:215-243`, `plugins/shell-guard-core.js:165…`) | excluded by grammar text; single-token `+= 1` rewrite is the packet's sanctioned lift |
| for-of, plain binding | PRESENT | `plugins/shell-guard.js:249,258` (`for (const [pattern, action] of …)` is the destructured cite; plain: `overlays/…/session-progress.js` throughout) | `ForOf` |
| for-of, array-destructured head | PRESENT | `overlays/…/auto-tool-gate.js:4019`, `overlays/…/auto-gate-tiered.js:489` (`[l1, l2, exp, expDis, expInc]` — 5-way), `plugins/session-state.js:64`, `scripts/verify-release-interaction-contract.js:172` | `ForOf` + ArrayBindingPattern |
| for-of, object-destructured head | ABSENT (probe `for\s*\(\s*(const\|let\|var)\s+\{` → none) | — | not needed; DECISION D-2 may add |
| for-in | ABSENT (probe `for\s*\(…\s+in\s` → none) | — | NOT admitted |
| `for (;;)` infinite | PRESENT | `scripts/verify-no-source-tree-only-paths.js:137`, `verify-f3-authoring-surfaces.js:85`, `verify-no-unrendered-paths.js:71` (all with `break`) | `ForStmt` empty in/cond/update |
| while | PRESENT | `scripts/state-lib.js:575` (`while (fd === null) {`), `:1071` | `WhileStmt` |
| do-while | ABSENT (probe `\bdo\s*\{` → none) | — | NOT admitted (also removes the `while(…);` terminator edge case from Family 8) |
| labeled statement / labeled break/continue | ABSENT (probes `^\s*\w+:\s*(for\|while\|do\|switch\|{)` → only object-literal keys; `\b(break\|continue)\s+\w+;` → none) | — | NOT admitted |
| switch: string-literal cases + break | PRESENT | `scripts/normalize-backlog.js:33-62` | `CaseClause` |
| switch: braced case bodies with lexical decls | PRESENT | `tools/plan-state.js:444,523,593` (`case "resolve_plan": {`); `scripts/complexity-signal-lib.js:108-…` (`case "defaults": { const [key, val] = … }`) | `CaseClause` + Block |
| switch: default clause | PRESENT | `scripts/normalize-backlog.js:62` | `'default' ':'` |
| switch: fallthrough (case body without break) | ABSENT in sampled files (normalize-backlog, complexity-signal-lib all break-terminated) — **probe-limited, not corpus-exhaustive** | — | if freeze admits Stmt* without break it falls out; else DECISION D-2 |
| regex inside case body (expression-start) | PRESENT | `scripts/complexity-signal-lib.js:~115` (`trimmed.match(/^"?(\.[a-z0-9]+)"?\s*:(.*)$/)`) | Family 7 regex production |

**Fixtures.**
- **F2-FO:** module combining: else-if chain, classic for with `i += 1`,
  for-of with 2-way array destructure, `for (;;)` + break, while, switch with
  braced case + default + break, regex in a case body. Expected `FIT`, export
  edge `ok`. Deviation = false-broken.
- **F2-FB:** three modules: (a) one `i++` in a for-head → `UNKNOWN(reason:
  update operator)` (U1/U5; the corpus-real debt pattern); (b) `outer: for
  (;;) { break outer; }` → `UNKNOWN(reason: labeled statement)`; (c)
  `for (const k in o) {}` → `UNKNOWN(reason: for-in not admitted)` — all
  under DECISION D-2 exclusion scope; if the freeze admits any of these the
  pinned verdict flips to FIT, which is exactly what ratification must record.
  A run returning `FIT` on (a) is false-ok (the packet's 15-file debt class
  silently passing); a run returning `broken`/refusal instead of `UNKNOWN` is
  false-broken (U6 violation).

## Family 3 — try / catch / finally (packet: 49/61)

**Proposed production clause:** `TryStmt := 'try' Block ('catch' '('
BindingIdent ')' Block | 'catch' Block)? ('finally' Block)?`.

| Variant | Status | Source span(s) | Clause |
|---|---|---|---|
| try/catch(binding) | PRESENT | ubiquitous in the 49-file family (e.g. `scripts/state-lib.js` throughout) | `'catch' '(' BindingIdent ')'` |
| optional catch binding `catch {` | PRESENT | `plugins/shell-guard-core.js:149,300,418,1092,1388`; `plugins/shell-guard.js:231` | `'catch' Block` |
| try/finally only (no catch) | PRESENT | `scripts/state-lib.js:625-628` (`try { return fn(); } finally {`) | `('finally' Block)?` alone |
| try/catch/finally | PRESENT | `scripts/verify-task-registry.js` (16 code `finally` sites; comments at :37,:93 describe them) | full production |
| nested try (try inside catch) | PRESENT | `scripts/state-lib.js:613-614` (`} catch (_cleanupError) { try {` — cleanup-retry pattern) | recursive Stmt |
| catch with destructuring `catch ({msg})` | ABSENT (probe `catch\s*\(\s*[{[]` → none) | — | NOT admitted; DECISION D-2 |

**Fixtures.**
- **F3-FO:** module with all four present forms incl. the nesting pattern.
  Expected `FIT`/`ok`. Deviation = false-broken.
- **F3-FB:** module with `try {} catch ({ message }) {}` → `UNKNOWN(reason:
  catch destructuring not admitted)` under D-2 exclusion. `FIT` = false-ok.
  Second guard: a checker that mis-scans `catch {` (optional binding) as an
  object-literal start would degrade all 6 shell-guard-core sites — the F3-FO
  fixture with that exact form is the false-broken discriminator.

## Family 4 — Classes (packet census: "class keyword in 3/61"; real class-declaration surface: **2 files** — see Contradiction note C-1)

**Proposed production clause:** `ClassDecl := 'export'? 'class' BindingIdent
'extends' LeftHandExpr? ClassBody`; `ClassBody := '{' MethodDef* '}'`;
`MethodDef := 'constructor' '(' ParamList ')' Block`. Only constructor-form
methods are admitted (the entire corpus surface); fields, statics, accessors,
generators, computed keys are NOT admitted.

| Variant | Status | Source span(s) | Clause |
|---|---|---|---|
| `export class X extends Error` with constructor + super + this-assignment | PRESENT | `scripts/state-lib.js:223-236` (`StateError`: `constructor(message, cause)`, `super(message)`, conditional `this.cause = cause`) | full production |
| derived class, EMPTY body | PRESENT | `scripts/normalize-backlog.js:20` (`class BacklogError extends Error {}`) | `ClassBody := '{' '}'` |
| `extends Error` heritage (built-in) | PRESENT | both files | `'extends' LeftHandExpr` |
| `new` on the declared class | PRESENT | `scripts/state-lib.js:284` (`throw new StateError(message)`) | expression grammar `new` |
| **private `#fields`** | **ABSENT** (probe `[#$]\w+\s*[;=(]` on the 2 class files → none) | — | NOT admitted |
| **static blocks / static methods / static fields** | **ABSENT** (probe `static\s+[A-Za-z#\[]` → none) | — | NOT admitted |
| getter/setter methods | **ABSENT** (probe `\b(get\|set)\s+\w+\s*\(` corpus-wide → none) | — | NOT admitted |
| generator methods | **ABSENT** (probe `function\s*\*` → none corpus-wide, Family-2 note) | — | NOT admitted |
| computed class keys `[expr]()` | **ABSENT** in classes (computed OBJECT keys exist — Family 1/expr note: `overlays/…/auto-gate-live.js:1582-1583` `[CLASSIFIER_PROMPT_KEY]:` is object-literal, not class) | — | NOT admitted in class body |
| class expressions (`= class …`), `new.target`, decorators | ABSENT (probes) | — | NOT admitted |

**Fixtures.**
- **F4-FO:** a module reproducing the exact StateError shape (export, extends,
  constructor, super, conditional this-assign) + the empty-derived shape.
  Expected `FIT`/`ok`. Deviation = false-broken — note this would degrade
  `state-lib.js`, the 19-edge hub; a class bug here costs the whole L1 surface
  of 22 edges (packet §C concentration fact).
- **F4-FB:** one module containing `class A extends Error { #x = 1; static y
  = 2; static {} get g() { return 1; } }` → `UNKNOWN(reason: class private
  field / static member / accessor not admitted)` (U1/U5). `FIT` = false-ok.
  This is the dispatch's named audit item: the three class files contain NONE
  of these forms — the corpus risk is zero today, and the fixture pins that
  the freeze keeps it that way.

## Family 5 — await (packet: 15/61)

await files (15, verified by code-position probe `^[^/*]*\bawait\b`,
comment-filtered): `plugins/session-state.js`, `plugins/shell-guard-core.js`,
`plugins/coordination-hints.js`, `plugins/shell-guard/eval.js`,
`plugins/shell-guard.js`, `overlays/…/repo-mail-egress-wiring.js`,
`repo-configs/forbidden-patterns.js`, `scripts/verify-chunk-timeout-backstop.js`,
`scripts/verify-task-registry.js`, `overlays/…/auto-gate-live.js`,
`scripts/verify-coordination-hints.js`, `scripts/verify-skill-sentinel.js`,
`overlays/…/auto-tool-gate.js`, `overlays/…/session-progress.js`,
`overlays/…/session-progress-judge.js`.

**Proposed production clause:** `await` admitted only as
`AwaitExpr := 'await' UnaryExpr` inside async function/arrow/method bodies
(Family 1 `[async]` prefix, Family 4 constructor bodies excluded by absence of
async methods). Top-level await, `for await` NOT admitted (both absent;
DECISION D-2).

| Variant | Status | Source span(s) | Clause |
|---|---|---|---|
| await call in async fn | PRESENT | `overlays/…/repo-mail-egress-wiring.js:238` (`const mod = await import(resolved);` — the ledger's computed-specifier dynamic-import unknown) | `AwaitExpr` |
| await + object destructure | PRESENT | `overlays/…/repo-mail-egress-wiring.js:280` (`const { gateEgressMessage } = await import(…)`); `plugins/shell-guard-core.js:475` (`const { Parser, Language } = await import("web-tree-sitter")`) | `AwaitExpr` in VarDecl initializer |
| await dynamic import, module specifier | PRESENT | `overlays/…/repo-mail-egress-wiring.js:600,662,693,713,739` (`await import("node:fs")`) | `AwaitExpr` (edge stays dynamic-import UNKNOWN-by-contract — the 5 ledger pins; U5/U6) |
| await in async arrow handler | PRESENT | `plugins/session-state.js:283` context (`"shell.env": async (input, output) => {` bodies) | async Arrow body |
| top-level await | ABSENT (probe `^await\s` → none; both `.mjs` entry scripts keep await inside async functions) | — | NOT admitted |
| `for await … of` | ABSENT (probe `for\s+await` → none) | — | NOT admitted |
| await destructuring assignment (no decl) | ABSENT (probe `await\s*([{[])` beyond the decl forms → none standalone) | — | — |

**Fixtures.**
- **F5-FO:** module with the plugin pattern: `export const server = async
  ({ client, directory }) => { const mod = await import("./x.js"); const { a }
  = await helper(); };` Expected `FIT`; the static import edge (if any) `ok`;
  the dynamic-import edge `unknown(reason: dynamic import)` — pinned as
  UNKNOWN-BY-CONTRACT (the existing 5-pin policy; changing it needs explicit
  contract approval per brief.md:71). Deviation = false-broken.
- **F5-FB:** (a) module with top-level `await fetch(x);` at module scope →
  `UNKNOWN(reason: top-level await not admitted)`; (b) `for await (const c of
  stream) {}` inside async fn → `UNKNOWN(reason: for-await not admitted)`.
  Both under D-2 exclusion. `FIT` = false-ok.

## Family 6 — Template literals with recursively parsed ${…} (packet: 55/61, 1404 sites)

**Proposed production clause:** `Template := '``' (TemplateChar |
'${' Expr '}' )* '``'` — `Expr` is the full expression grammar INCLUDING
`Template` (this is the recursion the brief mandates: "template substitutions
parsed recursively", consolidation-corrections paragraph). No depth cap.

| Variant | Status | Source span(s) | Clause |
|---|---|---|---|
| substitution with call chains + arrows | PRESENT | `scripts/check-s2-holds.mjs:678,721` (`${ev.map((r) => \`${r.file}:${r.line}\`).join(", ")}`) | recursive Expr |
| nested template depth 2 (template inside substitution) | PRESENT | same spans; `overlays/…/auto-gate-live.js:121` (`${stderr ? \`: ${stderr}\` : ""}`); `scripts/verify-state-validation.js:80` (two nested-template substitutions on one line); `scripts/state-lib.js:3749,4645,5699,7406` | recursion, depth 2 |
| nested template inside conditional inside substitution | PRESENT | `scripts/state-lib.js:7406` (`${p.locator ? \` ${p.locator}\` : ""}`) | recursion + conditional Expr |
| multi-line template with embedded substitutions | PRESENT | `scripts/verify-state-validation.js:80` (`\n` continuations); `state-lib.js:5844` | TemplateChar includes newline |
| nested template depth ≥3 | ABSENT (probe `\$\{[^}]*\$\{[^}]*\$\{` → none) — corpus max depth = 2 | — | admitted anyway (no depth cap); see fixture F6-FB(b) |
| tagged template `` tag`…` `` | ABSENT (probe: identifier-adjacent backticks all resolved to prose/markdown inside STRINGS/COMMENTS; no code-position tag found) | — | NOT admitted; DECISION D-2 |

**Fixtures.**
- **F6-FO:** module with the check-s2-holds.mjs:678 pattern verbatim
  (map-arrow returning a nested template, joined). Expected `FIT`/`ok`.
  Deviation = false-broken.
- **F6-FB:** (a) `` const t = tag`x${y}`; `` → `UNKNOWN(reason: tagged
  template not admitted)` under D-2; `FIT` = false-ok. (b) **Depth-3
  recursion guard:** `` const d = `a${`b${`c${x}`}d`}e`; `` → expected `FIT`
  (the production is RECURSIVE, not depth-capped — depth 3 never occurs in
  the corpus but the grammar text admits unbounded recursion; a checker
  degrading this to UNKNOWN exhibits a false depth cap = false-broken).

## Family 7 — Regex literals in operand positions only (packet: 314/314 conform; prev-token histogram §B: `(` 113, `,` 110, `=` 45, `:` 12, `>` 10, `&` 9, `!` 7, `return` 4, `|` 4)

*Dispatch-note: the dispatch calls this "§G registry"; the packet has no §G —
the operand-prefix registry IS the §B regex histogram. Recorded as a citation
mismatch, not an evidence gap (see Contradictions C-3).*

**Proposed production clause:** `PrimaryExpr := … | RegexLiteral`, admitted
exactly at **expression-start (operand) positions** — the position set whose
prev-token histogram the packet measured. The freeze must scope the admission
as "expression-start", with the 9-prefix histogram as the measured corpus
subset (see AMB-2 for the `>` scoping and DECISION D-3 for whitelist-vs-rule).

| Prev-token context | Count (packet §B) | Representative span(s) (this pass) |
|---|---|---|
| `(` — call arg / parenthesized | 113 | `plugins/session-state.js:137` (`.replace(/\\/g, "/")`), `plugins/shell-guard-core.js:424,455`, `scripts/complexity-signal-lib.js:~115` (in case body) |
| `,` — second+ call arg / array elt | 110 | `overlays/…/auto-gate-verdict.js:432,444,458` (`assert.match(r.audit, /blocked:/)`), `overlays/…/auto-gate-scrub.js:420` |
| `=` — var-init / assignment RHS | 45 | `plugins/shell-guard-core.js:64` (`export const SKIP_COMMIT_GATE_RE = /^\s*SKIP_COMMIT_GATE\s*=\s*1(\s\|$/` — note regex CONTAINING `=`), `:601` |
| `:` — object property value | 12 | `plugins/maxoutputtokens.js:49-50` (`{ pattern: /zai-coding-plan/i, … }`) |
| `>` — arrow body (`=>` then regex) | 10 | `repo-configs/repo-mail-egress-gate.js:876` (`test: (t) => /secretrepo/i.test(t)`) |
| `&` — `&&` RHS | 9 | `scripts/verify-recurrence-dedup.js:790`, `scripts/verify-f3-authoring-surfaces.js:221,239` (`/structural/i.test(…) && /truth/i.test(…)`) |
| `!` — unary-negation operand | 7 | `scripts/verify-f3-design-readiness.js:161,165`, `scripts/verify-skill-sentinel.js:263` (`!/reload(ed)? automatically\|auto-reload/i.test(msg)`) |
| `return` | 4 | `scripts/state-lib.js:392` (`return /^[A-Za-z0-9._/:+-]+$/.test(text) ? …`) |
| `\|` — `\|\|` RHS | 4 | `overlays/…/repo-mail-egress-wiring.js:748` (`err instanceof SyntaxError \|\| /SyntaxError\|Unexpected/.test(…)`) |
| regex as STATEMENT-initial expression | 0 in corpus (histogram sums to 314 with 9 prefixes) | — (DECISION D-3) |
| `new RegExp(…)` constructor form | PRESENT (not a literal; no production needed) | `repo-configs/forbidden-patterns.core.js:147,249,278,315` |

**Fixtures.**
- **F7-FO:** a module with one regex in EACH of the 9 prefix contexts,
  including regexes containing `/`, `=`, and `|` INSIDE the pattern (per
  shell-guard-core.js:64). Expected `FIT`/`ok`. Deviation = false-broken.
- **F7-FB (the slash-goal double guard):** module A: `const q = (t) =>
  /x/.test(t); const d = a / b / c;` — expected `FIT` with the FIRST slash
  pair lexed as regex and the SECOND as division; misclassifying either
  direction is exactly the G32 defect (both false-ok and false-broken
  outcomes are possible here, which is why this fixture is the freeze's
  sharpest discriminator). Module B: regex NOT in an operand position, e.g.
  as the operand of an unadmitted operator form — any module the lexer
  cannot decide must come out `UNKNOWN(reason: unclassifiable '/')`,
  never `broken` (U6).

### ⛔ AMB-2 (FREEZE BLOCKER until ratified): the `>` prefix scoping (slash-goal boundary)

The histogram token `>` covers two productions: `=>` (arrow head — regex
FORCED, an expression must start) and bare `>` (binary operator — an operand
must start, so regex is also forced, but a division AFTER an expression like
`a > /x/.test(s) / 2` interleaves both roles). The corpus's 10 `>`-prefixed
regexes are all arrow bodies [inference from cite inspection — all sampled
spans are `=>`]. The production cannot be "regex after `>`" as a flat rule
without choosing the scoping; the sound clause is "regex at expression-start
positions", where `=>` is an expression-start context by construction. **Per
the material-ambiguity rule this is flagged AMBIGUOUS** — the freeze is
BLOCKED until the operator ratifies the expression-start formulation (or a
`=>`-only scoping with bare-`>` explicitly excluded, which would be more
restrictive than needed but corpus-sufficient). This is the exact boundary
where G32's four BLOCKs lived (brief.md:22); it must be named in the freeze
document, not inherited implicitly.

## Family 8 — Explicit statement termination (`;` or block-close; ASI excluded; packet asiDefinite = 0)

**Proposed production clause:** every simple statement ends with `;`;
statements ending in `}` (blocks, declarations with block bodies, control
heads with block bodies) need no terminator; a control head whose body is a
non-block statement requires the body's own terminator. Boundary classes per
packet §B: block/declaration-close boundaries (2637, 57 files) and
control-head continuations (11, 4 files) are NOT ASI reliance.

| Variant | Status | Source span(s) | Clause |
|---|---|---|---|
| `;`-terminated statements | PRESENT (universal) | corpus-wide | terminator |
| `}`-terminated blocks/declarations | PRESENT (2637 boundary sites; packet-sampled cites `overlays/…/auto-gate-live.js:104-117`, `scripts/verify-f3-dispatch-backstop.js:350-356`, `scripts/verify-skill-proposals.js:230-236`) | block-close |
| if-head, body on next line | PRESENT (11 sites; packet cites `scripts/state-lib.js:4541-4544`, `scripts/verify-coordination-hints.js:294-297`) | head-paren close is not a terminator point | control-head continuation |
| `for (;;)` + `break;` loop completion | PRESENT | `scripts/verify-no-source-tree-only-paths.js:137` | break carries its own `;` |
| do-while `while(…);` terminator edge | N/A — do-while absent (Family 2) | — | — |
| ASI-reliant boundary (missing `;` where required) | ABSENT — asiDefinite=0 (packet §B, reference predicates + control-paren refinement; NOT re-measured this pass per the no-new-census boundary) | — | excluded by grammar text |

**Fixtures.**
- **F8-FO:** module with the two boundary classes: an if-head next-line body
  and a `}` → next-statement sequence. Expected `FIT`/`ok`. Deviation =
  false-broken (an over-strict terminator demand after `}` would degrade
  essentially the whole corpus — this is the highest-blast-radius false
  direction in the freeze).
- **F8-FB:** module with `const a = 1\nconst b = 2;` → `UNKNOWN(reason:
  ASI-dependent boundary)` (U1/U5). `FIT` = false-ok (silent ASI admission
  contradicts the grammar text and the asiDefinite=0 pin).

---

# PART 2 — Synthetic export-star fixture designs (S1′ gate-1, adversarial, NOT implemented)

Design grounding: the reference closure semantics preserved verbatim at
`tmp/eslink-o4-conformance-packet/harness/eslink/graph.go` (surface
computation `resolveSurface` :376-460; phase-1 re-export checks :154-235;
phase-2 import checks :237-302; cycle rule :140-144) and `parser.go`
(`StarReexports`/`NamespaceReexports`/`ReexportEdges` data model :33-91).
Every expected verdict below cites the clause (U1-U6) and the reference line
that a successor S1′ implementation must reproduce or consciously deviate
from. Corpus reality being frozen at zero `export*`, ALL fixtures are
synthetic — that is the point (the closure logic is untested by real files;
packet §A non-vacuity note).

Fixture modules use relative specifiers and `.js` keys as the scanner tree
would (rendered-destination convention). "Chain" = star-re-export chain.

## XS-1 — Transitive closure through a star chain: closed vs open terminal

```
// a.js  (terminal provider)
export const x = 1;
export const helper = () => 2;

// b.js  (star relay)
export * from "./a.js";

// c.js  (star relay, relay of relay)
export * from "./b.js";

// importer-closed.js
import { x } from "./c.js";        // (i)
import { helper } from "./c.js";   // (ii)
import { missing } from "./c.js";  // (iii) — absence probe
```

- **(a) all terminal closed:** every module `FIT`; surfaces closed
  (`resolveSurface` merges a.js names through both hops, graph.go:406-458).
  Verdicts: (i) `ok` (proven present through the chain); (ii) `ok`;
  (iii) `broken(proven)` — "fully resolved export surface does not contain it
  (surface closed, so the absence is proven)" (graph.go:365-370 shape).
  **Pins that stars PRESERVE provability when every link is closed — the
  anti-degrade guard (false-broken direction: an implementation that degrades
  all star-linked edges to unknown fails (iii)).**
- **(b) terminal OPEN variant** (a.js given an update-op token `i++;` → a.js
  `UNKNOWN(reason: update operator)` U1; surface open, graph.go:380-383):
  b.js surface open (427-430), c.js open; (i),(ii),(iii) all
  `unknown(reason: "provider c.js has an open export surface (unparseable or
  unresolvable export-* chain)")` (graph.go:263-270). **(iii) flips
  broken→unknown: an open chain can never ground a broken verdict** — the
  false-ok guard (an implementation that still reports broken on an open
  chain fabricates proven absence).

## XS-2 — Re-export cycle A↔B

```
// cyc-a.js
export * from "./cyc-b.js";
export const a1 = 1;

// cyc-b.js
export * from "./cyc-a.js";
export const b1 = 2;

// importer-cyc.js
import { a1 } from "./cyc-a.js";
```

Expected: resolve(cyc-a) re-enters cyc-b → visiting-guard returns
`&surface{open: true}` (graph.go:140-144); both surfaces open; importer edge
`unknown(reason: "provider cyc-a.js has an open export surface (…
cyclic)")`. **Adversarial point:** even `a1` — a LOCALLY declared name — is
unprovable through the cycle under the reference semantics (the whole surface
is open; names are not consulted once `open`, graph.go:263-270). This is
deliberately stricter than ESM runtime resolution (which resolves local
exports fine); the fixture PINS the honest direction so S1′ cannot quietly
ship the laxer runtime behavior. An implementation reporting `ok` on `a1`
= false-ok; one reporting `broken` = false-broken/U6 violation.

## XS-3 — Ambiguity: two stars exporting the same symbol

```
// amb-x.js
export const v = 1;
export const onlyX = 1;

// amb-y.js
export const v = 2;

// amb-m.js
export * from "./amb-x.js";
export * from "./amb-y.js";

// importer-amb.js
import { v } from "./amb-m.js";      // (i)
import { onlyX } from "./amb-m.js"; // (ii)

// amb-m2.js — local shadow variant
export const v = 3;
export * from "./amb-x.js";
export * from "./amb-y.js";
// importer-amb2.js
import { v } from "./amb-m2.js";    // (iii)
```

Expected: (i) `v` provided by two DISTINCT star targets → ambiguous
(graph.go:439-445) → edge `unknown(reason: "re-exported name … is provided
ambiguously through multiple export-* targets")` (graph.go:358-364; also
reachable at the import check via the same surface flag). (ii) `onlyX`
single-sourced → `ok`. (iii) local declaration SHADOWS every star source
(graph.go:436-438) → `ok` (not ambiguous). **Discriminates:** reporting
`ok` on (i) = false-ok (fabricated unambiguous provenance); reporting
`unknown` on (iii) = false-broken (shadow rule missed). Duplicate stars of
the SAME target stay ok (providedBy keyed by target, graph.go:401-406) —
worth a sub-case (iv) `export * from "./amb-x.js"; export * from
"./amb-x.js";`.

## XS-4 — Star from a missing provider

```
// star-missing.js
export * from "./gone.js";

// importer-missing.js
import { anything } from "./star-missing.js";
```

Expected (tree lacks `gone.js`): phase-1 finding on star-missing.js itself:
`unknown(reason: "star re-export target .opencode/gone.js is not present in
the scanned tree (cannot assess merged surface)")` (graph.go:221-226);
resolveSurface marks the surface open (graph.go:423-425); importer edge
`unknown(open-surface reason)` (graph.go:263-270). **Guards the asymmetry
against XS-6:** a MISSING star target OPENS the surface (names could flow
from anywhere), while a missing NAMESPACE-re-export target does not (XS-6).
An implementation that treats missing-star as `broken` = false-broken; one
that keeps the surface closed = false-ok (would fabricate absence proofs
downstream).

## XS-5 — Star + named re-export mix

```
// mix-a.js
export const s1 = 1;

// mix-b.js
export const s2 = 2;

// mix-m.js
export * from "./mix-a.js";
export { s2 } from "./mix-b.js";
export { s2 as s2renamed } from "./mix-b.js";

// importer-mix.js
import { s1, s2, s2renamed } from "./mix-m.js";  // (i)
// variant mix-b2.js WITHOUT s2:
// importer-mix2.js: import { s2 } from "./mix-m2.js"; // (ii) — absence probe
```

Expected: the named re-exports are their own REQUIRED edges (ReexportEdges,
parser.go:33-34; checked in phase 1, graph.go:163-176): (i) all `ok` (s1 via
merged star; s2/s2renamed via re-export edges, each proven in mix-b.js).
(ii) with mix-b2 lacking `s2`: the re-export edge itself
`broken(proven)` — "module re-exports … whose fully resolved export surface
does not contain it (surface closed, so the absence is proven)"
(graph.go:365-370) — while the star-provided `s1` path stays `ok` (a local
defect in one edge does not open the surface; only star mechanics open it,
graph.go:380-458). **Discriminates state separation (U2):** a missing
re-export name is PROVEN broken with the surface still closed; an
implementation that opens the whole surface (degrading s1 to unknown too)
exhibits false degradation; one that reports `ok` on (ii) = false-ok.

## XS-6 — Namespace re-export `export * as ns from …`

```
// ns-a.js
export const inner = 1;

// ns-m.js
export * as ns from "./ns-a.js";

// importer-ns.js
import { ns } from "./ns-m.js";   // (i)
import { inner } from "./ns-m.js"; // (ii) — NOT re-exported by ns form

// variant ns-m2.js with target missing from tree:
// importer-ns2.js: import { ns } from "./ns-m2.js"; // (iii)
```

Expected: `ns` itself IS an export name of ns-m.js (NamedExports carries
namespace re-export bindings, parser.go:66-68) and the target imposes a
PRESENCE-ONLY edge — like a namespace import, no name check inside ns-a
(graph.go:177-199 comment). (i) `ok` (name `ns` present; surface closed —
resolveSurface does NOT open on NamespaceReexports, contrast graph.go:406-425
which opens only on StarReexports). (ii) `broken(proven)` — `inner` is not
in ns-m's surface (closed, absence proven). (iii) target missing: one
`unknown(reason: "namespace re-export target … is not present in the scanned
tree (cannot assess module presence)")` (graph.go:194-199) — **but ns-m2's
surface stays CLOSED with `ns` present**, so the importer's name edge (i')
is `ok` while the presence edge is unknown. **This is the sharpest
adversarial case in the set:** it pins TWO findings on one module (a
presence-unknown AND a name-ok) and pins the star/ns asymmetry. Reporting
`broken` on (iii)'s importer = false-broken; collapsing (i) to unknown =
false degradation; reporting (ii) as `ok` = false-ok.

## XS-7 — Star chain ending in unsupported syntax (whole-chain degradation)

```
// bad.js  (out-of-subset: contains i++;)
export const x = 1;
let i = 0; i++;

// chain1.js
export * from "./bad.js";

// chain2.js
export * from "./chain1.js";

// importer-chain.js
import { x } from "./chain2.js";
```

Expected (grammar-level, the six clauses firing together):
- bad.js: `UNKNOWN(reason: update operator)` — U1 complete-consumption, U5
  reason-coded. Its export surface is OPEN (out-of-subset ⇒ surface not
  enumerable with confidence — same honest direction as unparseable,
  graph.go:380-383).
- chain1.js: phase-1 `unknown(reason: "star re-export target bad.js has an
  open surface (unparseable, unassessable, or cyclic); merged surface cannot
  be assessed")` (graph.go:228-233) — surface open.
- chain2.js: same reason one hop up (its own star target chain1 is open,
  graph.go:427-430 → 228-233 fires at its level too) — **the WHOLE chain's
  every consumer degrades, reason-coded AT EACH HOP** (U4 propagation + U5
  per-hop reasons).
- importer-chain.js: `unknown(reason: "provider chain2.js has an open export
  surface …")` (graph.go:263-270). NEVER `broken` (U6; open can never ground
  broken). The scan completes and reports — no refusal.
- **Contrast control (the discriminator):** the byte-identical tree with
  bad.js's `i++` replaced by `i += 1` (the packet's sanctioned mechanical
  rewrite) flips the entire chain to closed: all modules `FIT`, importer edge
  `ok`. **A one-token delta must flip four modules' verdicts** — an
  implementation that does not propagate to the far consumer (false-ok:
  importer reported ok) or that refuses/degrades the scan itself
  (false-broken: U6 violation) fails. This case also exercises U3 in spirit:
  bad.js is out-of-subset BY ITS BYTES; nothing about its being
  corpus-authored or version-labelled may rescue it.

## XS-8 — Contrast case: full closure WITHOUT any star (the corpus norm)

```
// plain-a.js
export const x = 1;

// plain-m.js
import { x } from "./plain-a.js";
export const y = x + 1;
export { y as y2 };

// importer-plain.js
import { y, y2 } from "./plain-m.js";
import { missing } from "./plain-a.js";  // absence probe
```

Expected: all modules `FIT`; all surfaces closed with zero star machinery
(resolveSurface's own-names path, graph.go:384-400); `y`/`y2` edges `ok`;
`missing` edge `broken(proven)`. **Real-corpus grounding:** this is the
shape of all 49 live edges — nearest real analogues: the named re-export
edge `repo-configs/forbidden-patterns.js → forbidden-patterns.core.js`
(symbol `GIT_MUTATION_VERBS`, packet §A) and the state-lib.js hub (19 edges,
closed surface, 73 names). **Purpose:** proves the closed path still works
and that the star machinery (XS-1..XS-7) added zero degradation to star-free
trees — the regression guard for the 49-edge baseline (a run of XS-8
reporting anything other than the above, or perturbing the existing 61/49/0/5
pins' closed-edge subset, fails).

---

# Verification (claim → evidence)

| Claim | Verifying evidence | Verified |
|---|---|---|
| Corpus inventory = packet ledger (61 = 50 core + 11 overlay) | `jq -r '.modules[].path' ledger.json` (60 rendered keys + count check) vs `find templates/core/.opencode -name '*.js' -o -name '*.mjs'` (50 files) + overlays glob (11) | yes |
| Zero `export *` / `import * as` corpus-wide | `rg -n 'export\s*\*' …` → no matches (this pass; packet §A concurs) | yes |
| Switch files = the 7 listed | `rg -l '\bswitch\s*\('` over 61 modules → exactly those 7 | yes |
| await files = the 15 listed | `rg -l '^[^/*]*\bawait\b'` (comment-filtered) → exactly 15; matches packet 15/61 | yes |
| Classes: real declarations in exactly 2 files | `rg '\bclass\s+[A-Za-z_$]'` (filtered) → `state-lib.js:223`, `normalize-backlog.js:20` only; StateError span read :220-237 | yes |
| `for await` / do-while / labels / labeled break / for-in / catch-destructure / depth-3 templates / tagged templates / logical-assignment ops / exponent / top-level await: ABSENT | rg probes (patterns recorded per family above) → no matches. Probe-limited (regex, not parser) | yes (probe-strength) |
| `??` and `?.` present | `rg -cF '??'` (manifest-helper ×2, auto-tool-gate ×2, auto-gate-live ×2), `rg -cF '?.'` (state-lib ×6, shell-guard ×5, chunk-timeout ×2) | yes |
| All 9 regex prefix contexts have code exemplars | spans in Family 7 table (each from an rg probe this pass) | yes |
| Reference closure semantics as cited | read of `tmp/eslink-o4-conformance-packet/harness/eslink/graph.go:110-302,335-460`, `parser.go` model lines (verbatim packet harness copy) | yes |
| Packet census numbers (48/61 arrows, 1222 sites, 1404 templates, 314 regexes, asiDefinite 0, 70 update ops / 15 files, 49-edge L-table) | quoted from packet §A/§B/§C — NOT re-measured (no-new-census boundary) | quoted |
| F2/F3/F8 boundary-class spans (state-lib.js:4541-44, verify-coordination-hints.js:294-97, auto-gate-live.js:104-117 etc.) | packet §B cites (authoritative; sampled there by line inspection) | quoted |
| Ambiguity/cycle/missing/star semantics fixture reasons | graph.go line-cited in each XS case | yes |

# Contradictions

- **C-1 (resolved by evidence): census "class keyword in 3/61" vs 2 real
  class-declaration files.** The census counts `class` KEYWORD tokens; the
  third file is almost certainly a property-key artifact —
  `overlays/…/session-progress-judge.js` carries 5 `class:` property keys
  (lines 232-261) and `session-progress.js:619` has `r.class` property
  access; a keyword-table lexer counts those. [inference, high confidence]
  The variant map uses the REAL surface (2 files); no corpus risk either way.
- **C-2 (resolved): "51 core files" initial count.** Arithmetic slip in this
  pass's first enumeration; the ledger join (Verification row 1) shows exactly
  50. No discrepancy with the packet exists.
- **C-3 (citation mismatch, flagged): the dispatch's "§G registry"** — the
  packet has sections §A-§F only; the operand-prefix registry the dispatch
  intends is the §B regex prev-token histogram. Mapped accordingly (Family 7).
- **C-4 (stale-context note): `internal/eslink/` no longer exists in the
  working tree** (the packet, same day, called it "still present"). The
  reference implementation now lives in the worktree at `worktrees/native/`
  and the archive `.local/coordinator/archives/eslink-g32-blocked/…tar.gz`;
  this pass used the packet's verbatim harness copies (byte-identical per
  packet §"Measurement instruments"). No semantic divergence found or
  asserted — but any S1′ implementation pass must re-bind to a named,
  hash-pinned source.
- **C-5 (intra-pass, resolved): early operator probes (`||=`, `**`) used
  regex metacharacters unsafely** and produced garbage counts; re-run with
  `-F` fixed strings before any absence claim was recorded. Only the fixed-
  string results are asserted.

# Closeout report

## Per-family variant counts (PRESENT variants only; absences listed per family)

| Family | Variants PRESENT | Variants ABSENT (probe) | AMBIGUOUS flags |
|---|---|---|---|
| 1 Arrows | 13 | 0 within the restricted sub-grammar (cover-grammar form absent from corpus) | **AMB-1** |
| 2 Control flow | 11 | 6 (for-in, do-while, labels, object-destructure for-head, for-await*, fallthrough†) | — (DECISION D-2) |
| 3 try/catch/finally | 5 | 1 (catch destructure) | — |
| 4 Classes | 4 | 7 (#fields, static blocks/methods/fields, get/set, generators, computed keys, class expr) | — |
| 5 await | 4 | 2 (top-level await, for-await) | — (D-2) |
| 6 Templates | 5 | 2 (depth ≥3†, tagged) | — |
| 7 Regex | 9 prefix contexts (314/314) + constructor form | statement-initial regex (0 in corpus) | **AMB-2** (D-3 adjacent) |
| 8 Termination | 4 | ASI reliance (asiDefinite=0) | — |

\* for-await counted under both families 2 and 5 for cross-reference. †
fallthrough and depth-3 are probe-limited/sampled absences (noted in-family).

## ⛔ FREEZE BLOCKERS (prominent, per the ratified material-ambiguity rule)

1. **AMB-1 — arrow cover grammar.** `(…)` is dual-role in general JS; the
   corpus (1222 arrows) fits a RESTRICTED arrow-param production with no
   cover grammar, but that restriction must be RATIFIED, not inherited. Blocks
   the freeze until the operator ratifies the restricted production text
   (Family 1) or explicitly commissions the cover-grammar alternative.
2. **AMB-2 — regex-after-`>` scoping (slash-goal boundary).** The production
   must be scoped as "regex at expression-start positions" (with `=>` an
   expression-start context); a flat "after `>`" rule is unsound. This is the
   exact G32 BLOCK site. Blocks the freeze until the scoping is ratified.

Named NON-blocking decisions the freeze should still record (DECISIONS D-2,
D-3): D-2 = exclusion-scope list for the absent constructs (recommended:
exclude all — corpus-zero today; each exclusion's fixture verdict is UNKNOWN,
flipping to FIT only if the freeze admits one); D-3 = regex
whitelist-vs-expression-start-rule (recommended: expression-start rule;
statement-initial regex then lands as admitted-but-corpus-absent rather than
UNKNOWN — the freeze must say which).

## Export-star fixture count

8 case designs (XS-1…XS-8), 14 sub-cases with pinned verdicts ((i)/(ii)/(iii)
variants), every verdict clause-tagged (U1-U6) and grounded in reference
graph.go/parser.go lines. NOT implemented — designs only, per the ratified
boundary.

## Honest gaps (could not be verified from the corpus)

- **Probe-based absences** are regex-strength, not parser-strength: a tagged
  template, fallthrough case, or `**` in an unusual formatting could evade
  the probe patterns (patterns recorded per family). The census's own
  token-level data (packet §B) covers the enumerated exclusions but not
  these constructs.
- **Census third-class-file identity** (C-1) is inferred, not reproduced —
  reproducing it would be a new census (out of bounds).
- **`>`-prefix regexes all being arrow bodies** is inference from cite
  inspection; the packet histogram does not distinguish `=>` from bare `>`.
- **Case fallthrough**: sampled only in normalize-backlog.js and
  complexity-signal-lib.js (both fully break-terminated); not swept across
  all 7 switch files.
- **`?.` form split** (member vs call `?.(`) not separated — probe counted
  the token generally. `??=`/`||=`/`&&=` absences are fixed-string-solid.
- **switch files' internals beyond the two sampled** (plan-state.js,
  auto-gate-live.js, state-lib.js, check-defer-triggers.mjs,
  verify-task-registry.js switch bodies not read line-by-line; braced-case
  evidence comes from plan-state.js + complexity-signal-lib.js).
- The 49-edge join facts and 70-update-op cites are packet-quoted, not
  re-measured (no-new-census boundary).

## Confidence

**HIGH** for: corpus inventory binding; per-variant PRESENT determinations
(each backed by a read span); the 9 regex prefix exemplars; reference closure
semantics cites; absent-construct determinations with fixed-string probes.
**MEDIUM** for: probe-based absences with context-sensitive patterns (tagged
templates, fallthrough); the census-artifact explanation (C-1); the
arrow-body-only reading of `>`-prefix regexes. **Overall packet confidence:
high**, with the two AMBIGUOUS flags as the only items that gate the freeze
decision itself.

## Recommended artifact type / next steps

- **Artifact type:** sources (this file). Durable landing path for the later
  build slice: `researches/sources/2026-11-08-eslink-o4-d-pass-variant-map.md`
  (one-step move + header de-staging).
- **Next specialist:** back to the **freeze owner (operator)** with the two
  AMBIGUOUS flags + D-2/D-3 decisions; then **planner** for the S1′ gate-1
  implementation slice that turns PART 2 designs into executable adversarial
  fixtures (separately authorized — this pass implements nothing). The
  existing O3 packet's sequence (packet §"Recommended artifact type": packet →
  debate → planner) continues to hold; this D-pass feeds the freeze gate that
  precedes all of it.
- **Promotion targets (if/when adopted):** the ratified variant map becomes
  the grammar-freeze document's admission annex; the XS-1…XS-8 designs become
  the S1′ gate-1 fixture corpus under the eslink regression surface (location
  to be chosen by the implementing slice, not this pass).
- No repo, git, backlog, or coordination state was modified by this pass. All
  outputs live under `tmp/agent-runs/o4-d-pass/`. Read-only throughout; the
  only writes were this artifact.

---

## Amendment 2026-11-08 — Family 5 correction (module-top await; probe miss)

> Appended by the grammar-freeze amendment record
> (`researches/decisions/2026-11-08-eslink-o4-grammar-freeze-amendment-tla.md`,
> operator-ratified Option 1, 2026-11-08). The D-pass body above is the
> record AS MEASURED and is preserved unedited (byte-verbatim landing pin at
> `2f5b304` unaffected); this section is the dated correction the amendment
> ratifies.

- **What was wrong:** the Family 5 row "top-level await | ABSENT (probe
  `^await\s` → none)" is a **probe miss**, not a corpus fact. The probe is
  line-start anchored and structurally cannot see mid-statement or indented
  module-top await — exactly the probe-strength risk this packet's
  "Honest gaps" section pre-flagged. The S1′ build escalation of 2026-11-08
  surfaced the contradiction; the miss was in the census inputs, not in the
  S1′ build.
- **The corpus fact (amended):** module-top top-level await EXISTS in 6 of
  the 61 modules, all `node --check`-legal (each re-verified by direct line
  read for the amendment record):

  | Module (corpus path) | Line(s) | Form |
  |---|---|---|
  | `templates/core/.opencode/repo-configs/forbidden-patterns.js` | 39 | `const mod = await import("./forbidden-patterns.project.js")` in top-level `try` |
  | `templates/overlays/repo-mail/plugins/repo-mail-egress-wiring.js` | 280, 283 | `const { gateEgressMessage } = await import(…)`, `const { scrubCredentials } = await import(…)` in `if (__isMain)` at module scope |
  | `templates/core/.opencode/scripts/verify-chunk-timeout-backstop.js` | 225 | `try { await main(); } catch` epilogue |
  | `templates/core/.opencode/scripts/verify-coordination-hints.js` | 701 | same epilogue |
  | `templates/core/.opencode/scripts/verify-skill-sentinel.js` | 575 | same epilogue |
  | `templates/core/.opencode/scripts/verify-task-registry.js` | 5349 | `try { await runWorkerMode(…) }` in `if (__entryPath === __modulePath)` |

- **Amended clause (Family 5):** `AwaitExpr := 'await' UnaryExpr` is
  admitted inside async function/arrow/method bodies **and at module top
  level** (module top is an [Await] context). Unchanged: `for await … of`
  stays excluded; `await` in a non-async function stays rejected.
- **Fixture verdict flip:** F5-FB(a) (module with top-level `await fetch(x);`
  at module scope) flips `UNKNOWN(reason: top-level await not admitted)` →
  `FIT`. F5-FB(b) (`for await`) is unchanged.
- **Carried pin correction (same amendment):** the packet-era FIT pin
  46/61 is corrected to **45/61** — the delta module
  (`templates/core/.opencode/plugins/compaction-primitives.js`) is invalid
  JavaScript (unescaped backticks, line 18; the only module of 61 that
  `node --check` rejects) that the reference lexer mis-lexed as balanced.
  See the amendment record for the full measured reconciliation and the
  hash-pinned reference-implementation binding.
