# O4 eslink — Grammar-Variant Freeze Record (S1′ spec closure)

- **Date:** 2026-11-08
- **Type:** decision record (freeze ratification)
- **Status:** FROZEN — operator ratified all three dispositions "all as
  recommended" on 2026-11-08
- **Effect:** executes the variant-freeze step of the ratified sequence
  (`D pass → A variant freeze → S1′ under B → measured C decision`); from this
  record forward the grammar variant set is closed and S1′ may proceed to
  parser code under the staged-restart record's Section B gates and budget.
  Any spec change after this point requires an amendment to this record plus
  renewed operator approval (per `2cae4a2`).

## Provenance chain

```
G32 review block (2026-11-07) → F5 deferral checkpoint 72631d9
→ corpus-conformance packet d4ae991 (measured fact base)
→ staged-restart spec 2cae4a2 (ratified 2026-11-08)
   └─ Section A freeze rule: "Exact variant EBNF for the admitted families is
      frozen AFTER the D pass and BEFORE any parser code… material ambiguity
      found by D BLOCKS freeze — D is… the last gate that can stop the
      sequence without writing a parser."
→ D pass (variant-to-production map + export-star fixture designs; commissioned
   by 2cae4a2 Section D; landed byte-verbatim at 2f5b304 as
   researches/sources/2026-11-08-eslink-o4-grammar-variant-map.md — this
   landing's commit 1)
→ operator dispositions 2026-11-08 — "all as recommended" (this record).
```

The D pass raised exactly two material ambiguities (AMB-1, AMB-2) and two
named non-blocking decisions (D-2, D-3); under the freeze rule the two AMB
flags blocked this record until the operator ratified their resolution. The
operator ratified all three disposition items as recommended on 2026-11-08
(D-3 was resolved by the AMB-2 decision rather than separately).

## The three dispositions (operator intent recorded verbatim)

### 1. AMB-1 RESOLVED → restricted arrow-parameter production

Arrow parameters are frozen to **patterns/identifiers/defaults/rest only**:
`ArrowParams := BindingIdent | ObjectPattern | ArrayPattern (incl. elision) |
'(' ParamList? ')'`, with `Param := BindingPattern ('=' AssignmentExpr)? |
'...' BindingIdent`, defaults being side-effect-free initializer expressions
without a top-level comma, rest in final position (the map's Family 1
production clause, map:89-96). Evidence basis: **all 1,222 corpus arrows fit**
the restricted production (map:136-147 — every corpus arrow's parameter list
is exactly `BindingIdent | ObjectPattern | ArrayPattern | '...' BindingIdent`).
The JS cover grammar (reparse-after-cover for dual-role `(`) is
**corpus-absent and REJECTED as unforced parser surface** — no corpus arrow
needs it, so S1′ must not build it. Consequence pinned by the map's F1-FB
fixture: an arrow param default whose initializer is a top-level comma
expression (e.g. `(a = (b, c)) => a`) is outside the frozen subset → module
`UNKNOWN(reason: arrow-param default: top-level comma expression)` — never
silently `FIT`.

### 2. AMB-2 (and D-3 by the same decision) RESOLVED → regex at expression-start positions only

Regex literals are admitted **exactly at expression-start (operand) positions**
(the position set whose prev-token histogram the packet measured: `(` 113,
`,` 110, `=` 45, `:` 12, `>` 10, `&` 9, `!` 7, `return` 4, `|` 4 — 314/314
corpus regexes; map:323, 335-346). The arrow `=>` **qualifies by construction**
(an expression must start after `=>`). A flat **"regex after `>`" rule is
REJECTED as unsound** — the token `>` is dual-role (`=>` arrow head vs. bare
binary `>`), and `a > /x/.test(s) / 2` interleaves both roles; this is the
exact G32 BLOCK boundary (four BLOCKs, one root family: misclassified slash →
phantom regex → wrong surface; map:363-378). **D-3 is resolved by this same
decision**: the prefix-whitelist variant (admitting only the 9 measured
prev-token prefixes) is rejected in favor of the expression-start rule.
Recorded consequence (the map's D-3 "the freeze must say which", map:760-766):
**statement-initial regex is ADMITTED-but-corpus-absent** (0 occurrences,
map:346) — a statement-initial expression start is an expression start, so it
lands `FIT` under the rule, not `UNKNOWN`. Any `/` the lexer cannot decide at
an admitted position comes out reason-coded `UNKNOWN` — never `broken`, never
refusal (U5/U6; F7-FB double guard).

### 3. D-2 RESOLVED → exclude all twelve absent constructs

The following twelve corpus-absent constructs are **EXCLUDED from the frozen
subset** (each absence is probe-based, map's absent-probe table):

| # | Construct | Map cite (probe) |
|---|---|---|
| 1 | `for-in` heads | map:173 (`for\s*\(…\s+in\s` → none) |
| 2 | `do-while` | map:176 (`\bdo\s*\{` → none) |
| 3 | labeled statements / labeled `break`/`continue` | map:177 (label probes → none) |
| 4 | class private `#fields` | map:236 (`[#$]\w+\s*[;=(]` on class files → none) |
| 5 | class static members (static blocks, static methods, static fields) | map:237 (`static\s+[A-Za-z#\[]` → none) |
| 6 | class accessors (getters/setters) | map:238 (`\b(get\|set)\s+\w+\s*\(` → none) |
| 7 | generator methods | map:239 (`function\s*\*` → none corpus-wide) |
| 8 | computed class keys `[expr]()` | map:240 (computed keys exist only as object-literal forms) |
| 9 | class expressions (`= class …`), `new.target`, decorators | map:241 (probes → none) |
| 10 | `for await … of` | map:281 (`for\s+await` → none; also map:163) |
| 11 | tagged templates `` tag`…` `` | map:310 (identifier-adjacent backticks resolve to prose only) |
| 12 | catch parameter destructuring `catch ({…})` | map:211 (`catch\s*\(\s*[{[]` → none) |

**Future-corpus safety rule:** any future corpus occurrence of any excluded
construct → module-level **reason-coded UNKNOWN** (safe by clauses U4/U5 —
unsupported syntax → open surface → UNKNOWN propagates; every UNKNOWN
reason-coded) — never refusal, never FAIL (U6). The map's discriminating
fixtures (F2-FB, F3-FB, F4-FB, F5-FB, F6-FB) pin exactly these UNKNOWN
verdicts; a checker returning `FIT` on any of them exhibits the false-ok
direction.

## Freeze statement

**The grammar variant set is NOW FROZEN for S1′.** The complete spec is the
conjunction of:

1. the **eight production families** ratified at `2cae4a2` Section A
   (arrows; control flow incl. `switch`; try/catch/finally; class
   declarations; `await`; template literals with recursive substitution
   parsing; regex literals in operand positions; explicit statement
   termination with ASI excluded) **plus the six UNKNOWN-discipline clauses
   (U1–U6)** ratified in the same section;
2. **these three dispositions** (AMB-1 restricted arrows; AMB-2/D-3
   expression-start regex; D-2 twelve exclusions);
3. **the map's per-family production clauses and admitted-variant tables**
   as landed byte-verbatim at `2f5b304`
   (`researches/sources/2026-11-08-eslink-o4-grammar-variant-map.md`,
   PART 1) — the variant rows marked PRESENT are the admitted surface, with
   their production-clause bindings.

Completeness rule (uniform, no per-case adjudication left open): **any
construct form not carried by an admitted variant row or a ratified production
clause is outside the frozen subset** and lands reason-coded UNKNOWN under
U1/U4/U5 — this uniformly covers remaining corpus-absences the map records
but the twelve did not name (e.g. top-level `await`, excluded by Family 5's
clause text, map:268-272; object-destructured `for-of` heads, recorded
"not needed", map:172; logical-assignment operators `??=`/`||=`/`&&=`, absent
by fixed-string probes). Case fallthrough is ADMITTED by the ratified
`CaseClause := ('case' Expr | 'default') ':' Stmt*` clause (no `break`
requirement in the clause text; corpus-absent in sampled files, map:181).
Nested template depth is uncapped by the recursive Template production
(map:298-302; the map's F6-FB(b) fixture pins depth-3 as `FIT`). Update
operators `++`/`--` remain excluded by the grammar text — their disposition
is Section C (option 1, zero rewrites, stands), not a grammar admission
(`2cae4a2` Section A closing paragraph).

**S1′ proceeds under the staged-restart record's Section B gates and budget**
(seven gates; hard maximum of two review cascades; any new false verdict
halts; exhaustion reverts to the F5 deferral — no automatic extensions). The
map's PART 2 (XS-1…XS-8) is the S1′ gate-1 fixture design basis. **Any spec
change after this point requires an amendment to this record plus renewed
operator approval** (per `2cae4a2`; its stay-deferred conditions 1 and 2 are
now discharged — admissions were ratified and both material ambiguities
resolved — leaving conditions 3 and 4 to govern S1′ execution).

What this freeze does NOT authorize: no parser code is written by this record
(S1′ is separately commissioned); no production corpus rewrites (Section C
option 1 stands); no refusal wiring, doctor check, or CI hooks (Section B
scope exclusions).

## Contradictions

- **None detected** between the three dispositions and the measured inputs:
  the restricted arrow production covers all 1,222 measured arrows; the
  expression-start rule covers all 314 measured regexes; the twelve
  exclusions are all corpus-absent at probe strength. The map's own
  contradictions (C-1…C-5, map:704-728) are recorded there and none affects a
  disposition: C-1 (class 3/61 census vs. 2-file real surface) changes no
  admitted variant; C-4 (`internal/eslink/` relocation) binds S1′
  implementation, not this freeze.
- Carried note for S1′ (from the D-pass landing review, advisory): C-4
  requires the S1′ slice to re-bind the reference implementation to a named,
  hash-pinned source at its current location (`worktrees/native/` or the
  `.local/coordinator/archives/eslink-g32-blocked/…tar.gz` archive) — the
  map's cites run through the packet's verbatim harness copies.

## Findings

- **(finding)**: source=map Family 1 @ `2f5b304`, confidence=high, type=fact — every one of the 1,222 corpus arrows fits the restricted arrow-param production (`BindingIdent | ObjectPattern | ArrayPattern | '...' BindingIdent`, side-effect-free defaults, final rest); the cover-grammar form is corpus-absent.
- **(finding)**: source=map Family 7 @ `2f5b304`, confidence=high, type=fact — 314/314 corpus regexes sit in operand positions; the 9-prefix histogram sums to 314; statement-initial regex count is 0.
- **(inference)**: source=map AMB-2 + honest gaps @ `2f5b304`, confidence=medium, type=inference — the 10 `>`-prefixed regexes are all arrow bodies (cite inspection; the histogram does not distinguish `=>` from bare `>`). **Not load-bearing for this freeze**: the ratified rule is expression-start, under which the `=>`/bare-`>` distinction collapses (both force an operand start), so the disposition stands regardless of the inference.
- **(finding)**: source=map per-family absent rows @ `2f5b304`, confidence=high (probe-strength), type=fact — each of the twelve excluded constructs is corpus-absent by a recorded probe pattern; absences are regex-strength, not parser-strength (map honest gaps), which the future-corpus UNKNOWN rule makes safe.
- **(finding)**: source=`2cae4a2` Section A/B @ HEAD, confidence=high, type=fact — freeze rule text, eight families, six UNKNOWN clauses, seven gates, two-cascade budget as cited above.

## Verification

| Claim | Verifying evidence (map §/lines @ `2f5b304`; spec @ `2cae4a2`) | Verified |
|---|---|---|
| Freeze window and rule: variant EBNF frozen after D, before parser code; material ambiguity blocks freeze | spec §A "Freeze rule" (lines 81-87) | yes (cited) |
| D pass ran within ratified boundaries (no parser impl, no new census, no corpus edits) | map staging note (lines 3-13); commit-review validation at `2f5b304` | yes |
| Variant map landed byte-verbatim (this landing's commit 1) | sha256 `74e8972aa7bc927892732e894eb2caa9fc94e5de3fd738f191bec255ba1cdbd4` both sides; `cmp` identical; 52,789 bytes both; commit `2f5b304` | yes |
| All 1,222 corpus arrows fit the restricted production | map Family 1 (lines 87, 136-147) | yes (cited) |
| Cover-grammar arrow form corpus-absent (rejected as unforced) | map Family 1 closeout row (line 736) | yes (probe-strength) |
| 314/314 regexes operand-position; 9-prefix histogram; statement-initial = 0 | map Family 7 (lines 323, 335-346) | yes (cited) |
| Flat "after `>`" rule unsound; G32 boundary; `=>` is expression-start by construction | map AMB-2 (lines 363-378) | yes (cited) |
| D-3 resolved by AMB-2 decision (whitelist rejected) → statement-initial regex admitted-but-corpus-absent | map lines 346, 760-766 (map's recorded "the freeze must say which") | yes (derived from ratified rule) |
| Twelve constructs corpus-absent, excluded | map rows: for-in :173, do-while :176, labels :177, #fields :236, statics :237, get/set :238, generators :239, computed keys :240, class expr :241, for-await :281, tagged :310, catch-destructure :211 | yes (probe-strength) |
| Future occurrence of any excluded construct → reason-coded UNKNOWN, never refusal | clauses U4/U5/U6 (map lines 39-48; spec lines 64-79) | yes (cited) |
| Fallthrough admitted by `Stmt*` clause (no break requirement); template depth uncapped | map :181 (fallthrough row), :298-302 + F6-FB(b) :316-321 | yes (cited) |
| Top-level `await` excluded by Family 5 clause; update ops remain a Section C matter (option 1 stands) | map :268-272; spec Section A closing paragraph + Section C | yes (cited) |
| S1′ governed by Section B gates/budget (seven gates; ≤2 cascades; false-verdict halt) | spec §B (lines 89-129) | yes (cited) |
| Stay-deferred conditions 1-2 discharged; 3-4 govern S1′ | spec "Sequence and flip conditions" (lines 193-205) | yes (cited) |
