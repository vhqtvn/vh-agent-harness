# O4 eslink — Staged Restart Specification (Ratified)

- **Date:** 2026-11-08
- **Type:** decision record (ratification)
- **Status:** RATIFIED — operator ratified the four-line slate verbatim on 2026-11-08
- **Effect:** unblocks the staged O4 restart path whose mandatory first artifact
  and ordering were fixed by the F5 deferral; nothing in this record authorizes
  production corpus rewrites or enforcement wiring (see §B scope, §C decision)

## Provenance chain

`G32 review block (2026-11-07: 4 BLOCKs / 3 defects, one root family —
misclassified slash → phantom regex → wrong surface; 37 rounds nonconverging)`
→ `F5 deferral checkpoint, landed 72631d9` ("docs: record O4 eslink F5
deferral decision (G32 blocked, asset preserved)" — restart path: conformance
packet, then debate → planner → operator, in that order)
→ `corpus-conformance closure-coverage packet, landed d4ae991` (the measured
fact base; every measured number below cites it, none is re-derived here)
→ `debate STAGED verdict` (measured-fraction deliberation over packet §F)
→ `planner approval packet`
→ `slate debate` (four-line slate refined)
→ `operator ratification, 2026-11-08` (this slate, verbatim).

The three right-most artifacts are parent-session records not landed as repo
files; this decision record is their authoritative carrier — same convention
as the cohort-consistency memo's handling of its parent-session debate IDs
(`researches/decisions/2026-11-07-cohort-consistency-preflight-feasibility.md`
§Scope). The measured inputs are NOT parent-session material: they are the
committed packet at `d4ae991`, byte-identical at HEAD.

## The ratified slate (verbatim)

- **A:** Ratify all eight production families and the six UNKNOWN-discipline clauses; freeze exact admitted variants after D and before parser code, retaining recursive template-substitution parsing.
- **B:** Ratify S1′'s parser-plus-standalone-lint scope and seven gates, with zero production rewrites or enforcement wiring, a hard maximum of two cascades, new-false-verdict halt, and exhaustion-to-deferral.
- **C:** Select option 1 now (zero rewrites); revisit normalization options after measured S1′ results, without conditionally committing to option 3 at this approval.
- **D:** Commission a bounded pre-freeze pass covering material sub-construct variants and synthetic export-star fixture design, with source mappings and expected verdicts, but no parser implementation.

---

## Section A record — grammar admissions (eight families) + UNKNOWN discipline

Ratifies the grammar-spec gap the packet identified as the remaining
pre-freeze item (packet §B: the grammar's exclusions are nearly free for this
corpus — asiDefinite=0, regexes all operand-position, zero lexer errors — but
its *admissions* were unratified). The eight families, each with its measured
corpus-usage evidence (all numbers from the packet, bound to working tree at
HEAD 2026-11-08):

| # | Production family | Ratified status | Measured corpus usage |
|---|---|---|---|
| 1 | Arrow functions | ADMITTED | 1,222 sites in 48/61 modules |
| 2 | Control flow (`if`/`for`/`while`, with `switch`) | ADMITTED | 55/61 modules (`if`/`for`/`while`); `switch` 7/61 |
| 3 | `try`/`catch` family | ADMITTED | 49/61 modules |
| 4 | Class declarations | ADMITTED | class keyword in 3/61 modules |
| 5 | `await` expressions | ADMITTED | 15/61 modules |
| 6 | Template literals | ADMITTED via **recursive substitution parsing** (retained per slate A) | 1,404 sites in 55/61 modules |
| 7 | Regex literals | ADMITTED in **operand positions only** | 314/314 in operand positions (prev-token histogram: `(` 113, `,` 110, `=` 45, `:` 12, `>` 10, `&` 9, `!` 7, `return` 4, `\|` 4) |
| 8 | Statement termination (ASI) | **EXCLUDED** — explicit `;` required | asiDefinite = 0 in all three measured eras (29/48/61-module corpora) |

Update operators (`++`/`--`) remain EXCLUDED by the grammar text — the
corpus's only measured violation class (70 sites / 15 files) — and its
disposition is a Section C decision, not a grammar admission.

### The six UNKNOWN-discipline clauses (fixed contract text)

1. **Complete consumption or module UNKNOWN** — no convenient-line resumption.
2. **Separate import-completeness and export/transitive-closure states.**
3. **Qualification by actual adopter bytes, never ancestry.**
4. **Unsupported syntax / `export *` / namespace re-export → open surface →
   UNKNOWN.**
5. **Reason-coded UNKNOWNs.**
6. **UNKNOWN never authorizes refusal or FAIL.**

These clauses restate, as parser contract, the discipline the F5 checkpoint's
restart path demanded (complete consumption or UNKNOWN; import completeness
separated from export completeness; closure never inheriting from ancestry or
a corpus-origin label) plus the tri-state honesty rule the corpus's 5
dynamic-import unknowns already embody (unknown-by-contract,
grammar-independent — packet §F item vi).

### Freeze rule

Exact variant EBNF for the admitted families is frozen **AFTER** the D pass
and **BEFORE** any parser code. The slate debate's interaction rule: material
ambiguity found by D **blocks freeze** — D is not a formality between
ratification and code; it is the last gate that can stop the sequence without
writing a parser.

## Section B record — S1′ scope, gates, budget

**Scope (ratified):** a successor parser for the ratified grammar, plus a
standalone corpus lint harness. Explicitly OUT of scope: **no refusal wiring,
no doctor check, no CI hooks, no production corpus rewrites.** S1′ is a
correctness demonstration under gates, not a shipped enforcement surface;
nothing lands in a consumer repo by building it.

**The seven gates:**

1. **Discriminating red-checked fixtures per admitted production** — including
   slash/ASI boundaries exercised in BOTH false-ok and false-broken directions
   (the G32 defect family produced both; single-direction tests cannot catch
   it).
2. **Retained G32 regression suite + negatives** — removed-export, late-import,
   decoy, unsupported-suffix, slash-update mutations (the negative set named
   by the F5 restart path).
3. **Independent source-span/symbol audit of the 49-edge ledger** — S1 cannot
   be its own correctness oracle (F5 restart-path requirement; the packet's
   §A audit is the current baseline this gate must re-establish under the
   successor parser).
4. **Revision/hash-bound golden pins** covering core, overlays, rendered
   variants, and destination collisions.
5. **The mixed preserved-tree `collectDecisionItems` skew catch as a REQUIRED
   test** — the packet §E mixed-tree run (current corpus + pre-trilogy
   state-lib.js → broken=1, missing `collectDecisionItems`, 68 vs 73 export
   names) is the one demonstrated case where L3-capable enforcement fires on a
   real, adopter-representable tree; the successor must catch it.
6. **Disaggregated reporting** — dynamic UNKNOWNs / unsupported-syntax
   UNKNOWNs / closed obligations reported separately, never blended (extends
   the F5 checkpoint's never-a-single-blended-number rule to the parser's own
   output).
7. **Semantic-preservation protocol for any future approved rewrites** — no
   rewrite proceeds on orthographic equivalence alone without this protocol.

**Budget (ratified):** HARD maximum of **two** review cascades; **any new
false verdict halts** the stage immediately; budget exhaustion reverts to the
F5 deferral — **no automatic extensions**. Finding-class budgets were
explicitly rejected as the primary limit: the G32 history (declining-discovery
premise refuted — ~2 findings/round earlier vs 4 BLOCKs in G32) shows
counting findings is not evidence of convergence; false verdicts are.

## Section C record — normalization baseline (option 1 selected)

The three options, with measured numbers (packet §C join; §B per-file debt):

| Option | Corpus normalization | Enforceable obligations (L3) | Files edited |
|---|---|---|---|
| **1 — zero rewrites (SELECTED NOW)** | none | 10/49 (20.4%) | 0 |
| 2 — one hub token | `state-lib.js:5554` `++` only | 32/49 (65.3%) | 1 |
| 3 — all update-op sites | 70 `++`/`--` sites → `+= 1`-class | 49/49 (100%) | 15 — incl. 2 value-used prefix operators (`session-progress.js:492` `seq: ++st.seq`; `check-defer-triggers.mjs:344` `argv[++i]`) requiring manual semantic review |

Option 2's 32/49 is derived from the packet's measured join: as-is L3 = 10,
plus the hub token's 19 consumer edges (L1 → L3: every state-lib importer is
otherwise conformant), plus state-lib's own 3 outgoing edges (L0 → L3: its
three providers are conformant) — 10 + 19 + 3 = 32. This is the packet's
concentration fact (22 of the 39 degraded edges trace to one token in one
file) read as a normalization yield.

### Hub-churn analysis (why options 2/3 are not free)

Any platform-side edit to `state-lib.js` — option 2's single token, or
option 3's single token among its 70 — **advances the hub's origin hash** at
the next render. The origin-hash store records the bytes the platform last
wrote (`internal/originhash/originhash.go:96-103`); every adopter preserving
the hub therefore receives a **fresh stale-preserve event** on the following
update — exactly the cohort-skew interaction class the
`researches/sources/2026-11-05-origin-hash-stale-preserve-skew.md` packet and
`researches/decisions/origin-hash-update-sync.md` memo document. A
normalization touching the hub is a migration-visible event: a **migration
notice is mandatory if option 2 or 3 is ever chosen.**

### Decision (ratified)

**Option 1 now — zero rewrites.** The normalization decision is revisited at
stage 4 with **measured S1′ results**, because census projections are not
parser results: packet §C's L-join is a token-level construct-census proxy
(high confidence for enumerated exclusions, no information about constructs
the grammar text does not enumerate — packet §B method honesty), whereas §F's
fraction answer is explicit that successor-parser soundness is unmeasured.
This approval does **not** conditionally commit to option 3; the stage-4
revisit starts from measured parser output, not from the census projection.

## Section D record — the pre-freeze commission (runs in parallel with this landing)

**Commissioned:** a bounded evidence-design pass producing:

- a **variant-to-production map with source spans** — every material
  sub-construct variant located in the corpus and mapped to the production
  that must admit it;
- **proposed fixture cases with expected outcomes** for the 8 ratified
  families, including the sub-construct variants: **private `#fields`,
  static blocks, `for await`, destructuring in arrow params, labeled
  statements, comma expressions in statement position**;
- **synthetic export-star fixture designs** (the corpus contains zero
  `export *` — packet §A measured — so these must be constructed):
  transitive closure, cycles, ambiguity, missing providers, open
  dependencies — each with closed-vs-UNKNOWN expected outcomes exercising
  UNKNOWN-discipline clauses 2 and 4.

**Boundaries (ratified):** evidence design only — **no parser implementation,
no new census, no corpus edits.** The pass runs in parallel with this
landing; its artifact lands separately under `researches/sources/`.

## Sequence and flip conditions

**Sequence:** D pass → A variant freeze → S1′ under B → measured C decision
(stage 4).

**Stay-deferred** (the restart reverts to the F5 deferral state, no automatic
extensions) if any of:

1. admissions cannot be ratified at freeze (a family's exact variant EBNF
   cannot be written soundly);
2. D exposes material ambiguity (blocks freeze per §A freeze rule);
3. the successor parser emits false verdicts (halt per §B budget);
4. the two-cascade budget exhausts.

---

## Contradictions

- **None detected** in the measured inputs: packet pins reproduced exactly
  (61/49/0/5); the L-join is internally consistent (17+22+10 = 49;
  per-surface 39+10 = 49); era tallies sum (4+34+70 = 108).
- Carried from the packet (context, not measurement): the F2 brief's prior
  2/49 mid-tier estimate and the 49/49 guess were both wrong for the as-is
  corpus — the measured as-is number is 10/49 L3. This record's option table
  uses only measured numbers.
- No conflict with the F5 deferral: this record executes its restart path in
  its stated order (packet → debate → planner → operator) and leaves the
  deferral's no-fifth-cascade-by-default stance intact (§B's two-cascade
  budget is the fresh authorization with structural explanation the F5
  checkpoint required, not an extension of the old rounds).

## Findings

- **(finding)**: source=`docs/checkpoints/2026-11-07-o4-eslink-f5-deferral.md` @ `72631d9`, confidence=high, type=fact — G32 BLOCKED (4 BLOCKs / 3 defects, one root family); restart path fixed: packet → debate → planner → operator; no fifth cascade by default.
- **(finding)**: source=`researches/sources/2026-11-08-eslink-o4-corpus-conformance-closure-coverage.md` @ `d4ae991`, confidence=high, type=fact — ledger 61/49/0/5 reproduced; update ops are the only enumerated violation (70/15 files); asiDefinite=0 all eras; 314/314 regexes operand-position; L-join 17/22/10; mixed-tree `collectDecisionItems` broken=1.
- **(finding)**: source=packet §A hub structure, confidence=high, type=fact — 19/49 edges point into state-lib.js (up to 44 symbols per importer); state-lib imports 3 modules; 22 of 39 degraded edges trace to the single `state-lib.js:5554` token.
- **(inference)**: source=packet §A+§C join arithmetic, confidence=high, type=inference — option 2 yields 32/49 L3 (10 + 19 consumer + 3 outgoing); derivation shown in §C; the rewrite itself was not performed (read-only measurement).
- **(finding)**: source=packet §B, confidence=high, type=fact — 2 of the 70 update-op sites are value-used prefix operators (`session-progress.js:492`, `check-defer-triggers.mjs:344`) not mechanically rewriteable without semantic review.
- **(inference)**: source=packet §E + `originhash.go:96-103` + skew packet, confidence=high, type=inference — any state-lib.js edit advances its origin hash → fresh stale-preserve event for preserved-hub adopters → migration notice mandatory if options 2/3 are ever chosen.
- **(finding)**: source=this session's git receipts (below), confidence=high, type=fact — provenance commits verified: `72631d9` adds the checkpoint (125 lines), `d4ae991` adds the packet (513 lines) and is HEAD; the packet is byte-identical at HEAD.

## Verification

| Claim | Verifying command/output | Verified |
|---|---|---|
| Provenance: F5 checkpoint landed at `72631d9` | `git show --stat --oneline 72631d9` → adds `docs/checkpoints/2026-11-07-o4-eslink-f5-deferral.md`, 125 insertions (run this slice) | yes |
| Provenance: packet landed at `d4ae991` | `git show --stat --oneline d4ae991` → adds `researches/sources/2026-11-08-…-closure-coverage.md`, 513 insertions (run this slice) | yes |
| Packet cited at HEAD is the committed bytes | `git diff d4ae991 HEAD -- researches/sources/2026-11-08-eslink-o4-corpus-conformance-closure-coverage.md` → empty (run this slice) | yes |
| Ledger 61 modules / 49 edges / 0 broken / 5 unknown | packet §A + Verification row 1 (harness run: `modules=61 edges=49 broken=0 unknown=5 collisions=0`) @ `d4ae991` | yes (cited) |
| Arrows 1,222 sites in 48/61 | packet §B census totals @ `d4ae991` | yes (cited) |
| Control flow 55/61; switch 7/61 | packet §B gap paragraph @ `d4ae991` | yes (cited) |
| try/catch 49/61; classes 3/61; await 15/61 | packet §B gap paragraph @ `d4ae991` | yes (cited) |
| Templates 1,404 sites in 55/61 (recursive substitution) | packet §B census totals @ `d4ae991` | yes (cited) |
| Regex 314/314 operand-position | packet §B census totals + Verification row 6 (prev-token histogram) @ `d4ae991` | yes (cited) |
| asiDefinite = 0, all eras (29/48/61-module corpora) | packet §B + §E + Verification rows 5, 8 @ `d4ae991` | yes (cited) |
| Update ops 70 sites / 15 files (only enumerated violation) | packet §B per-file table + Verification row 4 @ `d4ae991` | yes (cited) |
| As-is L3 = 10/49 (20.4%); recognized 32/49 (65.3%) | packet §C + Verification row 7 (L-join `jq -f leveljoin.jq` → 17/22/10) @ `d4ae991` | yes (cited) |
| Option 2 = 32/49 from one token | derived: 10 + 19 + 3 (packet §A hub structure + §C concentration: 22 of 39 degraded edges → one token); derivation in §C above | yes (derived from measured join) |
| Option 3 = 49/49, 15 files, 2 value-used prefix ops | packet §C ("after the 70 mechanical rewrites… 49/49") + §B cites `session-progress.js:492`, `check-defer-triggers.mjs:344` @ `d4ae991` | yes (cited) |
| Mixed-tree `collectDecisionItems` broken=1; 68 vs 73 export names | packet §E + Verification rows 9–10 (`runmixed.sh` BROKEN-EDGE output; `jq` exportNames length) @ `d4ae991` | yes (cited) |
| Corpus has zero `export *` (fixtures must be synthetic) | packet §A + Verification row 3 (`jq … starReexports` → 0) @ `d4ae991` | yes (cited) |
| G32: 4 BLOCKs / 3 defects; no fifth cascade by default | `docs/checkpoints/2026-11-07-o4-eslink-f5-deferral.md` §Decision, §G32 outcome @ `72631d9` | yes (cited) |
| Origin hash = bytes the platform last wrote (hub-churn basis) | `internal/originhash/originhash.go:96-103`, cited via packet §F chain + `researches/decisions/origin-hash-update-sync.md` | yes (cited) |

Measured numbers are cited, not re-derived, per the landing contract; the
packet's own Verification table (rows referenced above) carries the commands
and machine artifacts (`ledger.json`, `census.json`, `ledger-mixed.json`)
that reproduce them, bound to the working tree at HEAD 2026-11-08.
