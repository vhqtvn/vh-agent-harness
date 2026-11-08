# Measured Packet: Corpus Conformance Closure-Coverage (O3 / F2 gate for O4 restart)

> **STAGING NOTE:** durable landing path is
> `researches/sources/2026-11-08-eslink-o4-corpus-conformance-closure-coverage.md`.
> This packet is staged under `tmp/` by the read-only researcher; landing it is a
> later build slice. Commissioned by the operator per
> `tmp/agent-runs/eslink-o4-f2-disposition/brief.md` (F2 disposition, O3
> measurement contract at brief.md:59-81). This packet MEASURES; it does not
> decide the restart.

- **Date:** 2026-11-08
- **Type:** source packet (measured fact base; NOT a decision memo)
- **Reporter class:** read-only researcher, exec-sandbox-contained measurement
- **Time-sensitivity:** LOW for corpus-byte facts (bound to working tree at HEAD
  2026-11-08; per-file sha256 in `ledger.json` modules[].sha256); MEDIUM for the
  edge ledger (corpus edits move 49-edge counts — pins exist precisely to catch
  that)
- **Source policy:** repo-first. Reference implementation =
  `internal/eslink/` (working tree, still present; the archive fallback
  `.local/coordinator/archives/eslink-g32-blocked/` was not needed). No external
  sources — the question is entirely about this repo's corpus bytes.

## Research question

If the corpus were constrained to the candidate conformance grammar
(brief.md:63-69), what fraction of the current link-check problem becomes
sound-by-construction — measured at the three declared levels:

- **L1 — recognized imports**: imports the grammar can parse at all
- **L2 — assessable edges**: import edges where BOTH endpoints parse and the
  provider's export surface is assessable
- **L3 — enforceable obligations**: edges where the provider surface is CLOSED
  (complete — no open re-export/completeness coupling) so a missing named export
  is PROVEN, not implied

## Scope and method (what was actually measured)

Three surfaces, per the commissioning dispatch:

1. **Core corpus** — `templates/core/.opencode/**` (50 JS modules)
2. **Overlay packs** — `templates/overlays/**` (11 JS modules across
   auto-classifier-pilot ×5, session-progress-pilot ×4, repo-mail ×1,
   contract-invariant-audit-pilot ×1; four more packs ship no JS)
3. **Mixed preserved trees** — bounded to what is derivable in-repo: the
   historical corpus at `07b05c7` (initial substrate, 29 modules) and at
   `b788b43^` (pre-retention-trilogy, 48 modules = the skew packet's adopter
   base era), plus one constructed mixed tree (current corpus with state-lib.js
   preserved at `b788b43^` bytes).

### Measurement instruments (all under `tmp/eslink-o4-conformance-packet/`)

- `harness/` — a self-contained Go module embedding a **verbatim copy** of
  `internal/eslink/{lexer,parser,graph}.go` (reference snapshot; byte-identical)
  plus measurement-only additions. `harness/main.go` reproduces
  `corpus_test.go`'s tree assembly (core `.opencode` walk + every overlay pack
  at `.opencode/`-mirrored keys, corpus_test.go:190-254) and dumps the full
  module/edge ledger (`ledger.json`), including a re-implementation of surface
  closure from `graph.go:376-460` semantics. `harness/eslink/census.go` adds a
  token-level construct census that reuses the **reference lexer**
  (`tokenize`) and the **reference ASI predicates** (`canEndStatement` /
  `startsStatement`, parser.go:805-846 / 742-784) — so "ASI-dependent" has
  exactly the reference implementation's semantics.
- `harness/census/` — census runner over the same tree + historical blobs.
- `runcensus.sh`, `runmixed.sh` — reproduction drivers (exec-sandbox-contained;
  see "Reproduction" below).

### What each number IS and IS NOT (method honesty)

- **MEASURED EXECUTION:** the ledger reproduction (61/49/0/5), the per-module
  parse surfaces, the mixed-tree scan, and the construct census are program
  outputs from the reference lexer/parser/scanner bound to the tree bytes.
- **CONSTRUCT CENSUS, NOT A GRAMMAR PARSER RUN:** no parser implementing the
  candidate grammar exists (the brief's measurement contract anticipated this —
  a prototype would need separate authorization, brief.md:78). "Fits the
  grammar" here means **zero enumerated excluded constructs** were found by the
  token-level census: no `++`/`--`, no definite-ASI boundaries, no lexer errors,
  regexes only in operand positions. This is a conformance **proxy** with
  high confidence for the *enumerated* exclusions and **no information** about
  constructs the grammar text does not enumerate (see "grammar-spec gap").
  [inference — method]
- **Two grammar readings, both reported.** The grammar text names exclusions
  explicitly (updates, ASI, regex/template productions) but says only
  "restricted expression statements" for everything else.
  - **G-charitable (operative measurement):** admitted = the corpus's actual
    statement repertoire minus the enumerated exclusions. Verdicts below use
    this unless stated.
  - **G-strict (literal text):** admitted = function/class declarations +
    restricted expression statements ONLY. Reported as the spec-decision bound.

---

## A. Reference-ledger audit (61 / 49 / 0 / 5 — pins reproduced)

**Result: the existing reference ledger is accurate.** Re-assembling the tree
exactly as `corpus_test.go:190-254` does and running the reference
`eslink.Scan`:

- **modules = 61** (pin: corpus_test.go:33) — 50 core + 11 overlay, **zero
  destination collisions** (no overlay key collides with a core key; measured)
- **edges = 49** (pin: corpus_test.go:34) — 48 import edges + 1 named
  re-export edge (`forbidden-patterns.js` → `forbidden-patterns.core.js`,
  symbol `GIT_MUTATION_VERBS`, corpus_test.go:92-99)
- **broken = 0** (pin: corpus_test.go:41-47)
- **unknown = 5** (pin: corpus_test.go:35) — ALL dynamic-import class
  (corpus_test.go:48-52 expectation holds): `repo-mail-egress-wiring.js:238`
  (computed), `:280`, `:283`; `forbidden-patterns.js:39`;
  `verify-task-registry.js:582` (computed)
- **unparseable = 0** — every module parses under the current S1 scanner

**Non-vacuity check (load-bearing for the 0-broken pin):** all 49 edges have
provider present + parseable + **surface CLOSED** under the reference closure
semantics (graph.go:237-305, 376-460). The corpus contains **zero `export *`
star re-exports and zero namespace re-exports** anywhere (measured) — so in
THIS corpus, "provider surface closed" degenerates to "provider parses": there
is no open re-export coupling to propagate. **Consequence:** for this corpus,
L2 (assessable) and L3 (enforceable) coincide for any parseability notion; the
L2-vs-L3 distinction only becomes live for future corpus shapes that introduce
`export *`.

**Hub structure:** `state-lib.js` is the provider hub — 19 of 49 edges point
into it (39% of the edge set), carrying up to 44 required symbols per importer
(`tools/plan-state.js`), 29 (`verify-session-state.js`), 16
(`verify-task-registry.js`). state-lib.js itself is an importer of 3 modules
(f3-design-readiness, rewrite-parity-validate, pause-new-work).

Per-surface edge split: **core importers 39 edges, overlay importers 10 edges**
(all 10 intra-overlay: auto-classifier-pilot ×6, session-progress-pilot ×4).

---

## B. Per-file grammar-conformance verdicts (G-charitable, current corpus)

### Census totals (61 modules)

| Construct class | Count | Files | Grammar status |
|---|---|---|---|
| `++`/`--` update operators | **70** | **15** | **EXCLUDED by grammar text** — the only measured corpus violation |
| Definite-ASI boundaries (missing-semicolons) | **0** | 0 | excluded; none found (see method note) |
| Control-head continuations (`if (…)` `)` + next-line body) | 11 | 4 | not ASI; no terminator required [verified by line inspection] |
| Block/declaration-close boundaries (`}` + next-line statement) | 2637 | 57 | not ASI reliance (sampled, see below) |
| Regex literals | 314 | 30 | admitted only via operand productions — **all 314 in operand positions** (prev-token histogram: `(` 113, `,` 110, `=` 45, `:` 12, `>` 10, `&` 9, `!` 7, `return` 4, `\|` 4) |
| Template literals | 1404 | 55 | admitted via recursive substitution productions |
| Arrow functions | 1222 | 48 | **grammar-unspecified** (see spec gap) |
| Lexer errors | 0 | 0 | none |

Method notes:
- **Definite-ASI = 0** uses the reference predicates plus a control-paren
  refinement: the 11 initial hits were all `if (cond)` heads whose body starts
  on the next line (e.g. `state-lib.js:4541-4544`,
  `verify-coordination-hints.js:294-297`) — structurally not ASI. Verified by
  reading each site class.
- **Block-close boundaries (2637)** were classified by next-token histogram +
  line inspection of the dominant classes (`}` → `if`/`const`/`return`/calls:
  `auto-gate-live.js:104-117`, `verify-f3-dispatch-backstop.js:350-356`,
  `verify-skill-proposals.js:230-236`): all sampled sites are block/declaration
  closes where the grammar requires no terminator. [inference — sampled across
  dominant classes, not all 2637 individually]
- **All 70 update operators are counter arithmetic** (`i++` loop counters,
  `attempt++`, `retries++`, `passed += 1` patterns; prefix forms at
  `session-progress.js:492` `seq: ++st.seq`, `check-defer-triggers.mjs:344`
  `argv[++i]`) — mechanically rewriteable to `+= 1` / `-= 1` with no semantic
  change. [inference from cite context, high confidence]

### Verdict vocabulary (per the dispatch's fits/partial/unknown)

Under the grammar's complete-consumption rule (brief.md:68), a module is
binary: fully consumed or UNKNOWN — intra-module "partial" cannot exist. The
actionable tri-state therefore maps to:

- **FIT (46/61)** — zero enumerated excluded constructs.
- **UNKNOWN as-is, update-op-debt only (15/61)** — the ONLY violations are
  `++`/`--`; each file would FIT after single-token mechanical edits. This is
  the dispatch's "partial" class: no grammar-redesign constructs anywhere in
  the current corpus.
- **UNKNOWN, grammar-redesign required (0/61)** — none measured.

### Per-file offending-construct table (the corpus-side lint payload)

All 15 failing files, update-op cites (line:op):

| Surface | File | Offending constructs |
|---|---|---|
| core | `.opencode/plugins/shell-guard-core.js` | L165,177,208,213,496,500,588,606,839,1039,1066,1080,1185,1209 `++` (14) |
| core | `.opencode/plugins/shell-guard.js` | L133,147,154,164,169,170,185,192 `++`, L165 `--` (10) |
| core | `.opencode/scripts/check-defer-triggers.mjs` | L241,338,344,501,1004,1084,1636 `++`, L503 `--` (8) |
| core | `.opencode/scripts/complexity-signal-lib.js` | L215,231,233,234,237,243 `++` (6) |
| core | `.opencode/scripts/state-lib.js` | L5554 `++` (1) |
| core | `.opencode/scripts/check-s2-holds.mjs` | L104 `++` (1) |
| core | `.opencode/scripts/verify-f3-authoring-surfaces.js` | L122 `++` (1) |
| core | `.opencode/scripts/verify-no-source-tree-only-paths.js` | L459 `++` (1) |
| core | `.opencode/scripts/verify-release-interaction-contract.js` | L72,174 `++` (2) |
| auto-classifier-pilot | `.opencode/plugins/auto-gate-live.js` | L507,855,874,1538,1706,2160,2253 `++` (7) |
| auto-classifier-pilot | `.opencode/plugins/auto-gate-tiered.js` | L257,258,259,712,715,718,740 `++` (7) |
| auto-classifier-pilot | `.opencode/plugins/auto-tool-gate.js` | L427,4229 `++` (2) |
| session-progress-pilot | `.opencode/plugins/session-progress.js` | L332 `--`, L492 `++` (2) |
| session-progress-pilot | `.opencode/scripts/session-progress-judge.js` | L70,73,78,240 `++`, L80 `--` (5) |
| session-progress-pilot | `.opencode/scripts/session-progress-policy.js` | L32,334 `--`, L73 `++` (3) |

**Per-surface FIT rates (as-is):** core 41/50 (82%), overlays 5/11 (45.5%),
corpus 46/61 (75.4%). (Surface = render source: the session-progress scripts
render from the session-progress-pilot pack even though their tree keys sit
under `.opencode/scripts/`.) Update-op totals by surface: core 44, overlays 26.

### Grammar-spec gap (what the grammar text does NOT decide — measured usage)

Under the **G-strict literal reading** (declarations + restricted expression
statements only), the only files with zero measured use of constructs the text
does not enumerate are the 3 pure-config modules (`allowed-commands.js`,
`forbidden-patterns.project.js`, `update-opencode-config.js`) — an **upper
bound of ≤3/61 (4.9%)**. Corpus-wide usage of unspecified constructs:
arrows in 48/61, if/for/while in 55/61, try/catch-family in 49/61, function
declarations in 55/61, await in 15/61, switch in 7/61, class keyword in 3/61.

**[inference — the load-bearing spec fact]** the grammar's exclusions are
nearly free for this corpus (asiDefinite=0, regexes all operand-position, zero
lexer errors), but its *admissions* are unratified: reaching 100% conformance
requires the grammar spec to explicitly admit the corpus's real statement
repertoire (arrows, control flow, try/catch, template recursion, regex operand
productions). That is specification work, and it is exactly where a successor
parser's soundness burden would live — not in the corpus, which is already
disciplined.

---

## C. Per-edge L-classification (49-edge ledger audit)

Join: census fit verdicts × ledger edges. Level definitions (disjoint chain):
L0 = importer unrecognized (below L1); L1 = importer recognized, provider not
assessable; L2 = both parse + provider assessable; L3 = provider CLOSED. In
this corpus L2 ≡ L3 (no `export *`, §A).

**As-is (G-charitable): L0 = 17, L1 = 22, L3 (=L2) = 10.**
**Cumulative framing (matches the dispatch's L-definitions): recognized (L1+)
= 32/49 (65.3%); assessable (L2+) = 10/49 (20.4%); enforceable (L3) = 10/49
(20.4%).**

Per-surface: **core 39 edges = L0:7 + L1:22 + L3:10; overlays 10 edges =
L0:10.**

Concentration facts: the single `state-lib.js` update op (L5554) downgrades 19
consumer edges to L1 and its own 3 outgoing edges to L0 — **22 of the 39
degraded edges (L0+L1) trace to ONE token in ONE file** [measured join].
complexity-signal-lib.js contributes 2 L1 edges; shell-guard-core.js 1
(its `eval.js` consumer). All 10 overlay edges are L0 because every overlay
importer of substance carries update-op debt.

Full 49-edge audit table (L-class as-is → after the 70 mechanical rewrites
every row becomes L3):

| L | Type | Importer → Provider | Symbols |
|---|---|---|---|
| L0 | import | plugins/auto-gate-live.js → plugins/auto-gate-scrub.js | 3 |
| L0 | import | plugins/auto-gate-live.js → plugins/auto-gate-verdict.js | 1 |
| L0 | import | plugins/auto-tool-gate.js → plugins/auto-gate-live.js | 2 |
| L0 | import | plugins/auto-tool-gate.js → plugins/auto-gate-scrub.js | 4 |
| L0 | import | plugins/auto-tool-gate.js → plugins/auto-gate-tiered.js | 2 |
| L0 | import | plugins/auto-tool-gate.js → plugins/auto-gate-verdict.js | 1 |
| L0 | import | plugins/session-progress.js → scripts/session-progress-config.js | 5 |
| L0 | import | plugins/session-progress.js → scripts/session-progress-judge.js | 2 |
| L0 | import | plugins/session-progress.js → scripts/session-progress-policy.js | 14 |
| L0 | import | scripts/session-progress-judge.js → scripts/session-progress-policy.js | 1 |
| L0 | import | plugins/shell-guard-core.js → repo-configs/allowed-commands.js | 2 |
| L0 | import | plugins/shell-guard-core.js → repo-configs/forbidden-patterns.js | 1 |
| L0 | import | plugins/shell-guard.js → plugins/shell-guard-core.js | 12 |
| L0 | import | scripts/state-lib.js → scripts/f3-design-readiness.js | 2 |
| L0 | import | scripts/state-lib.js → scripts/pause-new-work.js | 2 |
| L0 | import | scripts/state-lib.js → scripts/rewrite-parity-validate.js | 2 |
| L0 | import | scripts/verify-f3-authoring-surfaces.js → scripts/f3-design-readiness.js | 5 |
| L1 | import | plugins/session-state.js → scripts/state-lib.js | 3 |
| L1 | import | plugins/shell-guard/eval.js → plugins/shell-guard-core.js | 1 |
| L1 | import | scripts/coordination-hints-lib.js → scripts/complexity-signal-lib.js | 3 |
| L1 | import | scripts/verify-coordination-hints.js → scripts/complexity-signal-lib.js | 5 |
| L1 | import | scripts/approve-plan.js → scripts/state-lib.js | 4 |
| L1 | import | scripts/draft-plan.js → scripts/state-lib.js | 4 |
| L1 | import | scripts/plan-adopt.js → scripts/state-lib.js | 4 |
| L1 | import | scripts/plan-list.js → scripts/state-lib.js | 6 |
| L1 | import | scripts/plan-resolve.js → scripts/state-lib.js | 4 |
| L1 | import | scripts/plan-save.js → scripts/state-lib.js | 4 |
| L1 | import | scripts/session-bind.js → scripts/state-lib.js | 4 |
| L1 | import | scripts/session-current.js → scripts/state-lib.js | 4 |
| L1 | import | scripts/verify-coordinator-adoption-marker.js → scripts/state-lib.js | 3 |
| L1 | import | scripts/verify-f3-dispatch-backstop.js → scripts/state-lib.js | 13 |
| L1 | import | scripts/verify-f3-plan-approve.js → scripts/state-lib.js | 9 |
| L1 | import | scripts/verify-f3-task-ready.js → scripts/state-lib.js | 8 |
| L1 | import | scripts/verify-recurrence-dedup.js → scripts/state-lib.js | 3 |
| L1 | import | scripts/verify-session-state.js → scripts/state-lib.js | 29 |
| L1 | import | scripts/verify-skill-proposals.js → scripts/state-lib.js | 8 |
| L1 | import | scripts/verify-state-validation.js → scripts/state-lib.js | 6 |
| L1 | import | scripts/verify-task-registry.js → scripts/state-lib.js | 16 |
| L1 | import | tools/plan-state.js → scripts/state-lib.js | 44 |
| L3 | import | plugins/coordination-hints.js → scripts/coordination-hints-lib.js | 3 |
| L3 | import | plugins/pause-new-work.js → scripts/pause-new-work.js | 3 |
| L3 | import | repo-configs/forbidden-patterns.core.js → repo-configs/allowed-commands.js | 1 |
| L3 | import | repo-configs/forbidden-patterns.js → repo-configs/forbidden-patterns.core.js | 1 |
| L3 | import | scripts/verify-chunk-timeout-backstop.js → plugins/chunk-timeout-backstop.js | 3 |
| L3 | import | scripts/verify-coordination-hints.js → plugins/coordination-hints.js | 1 |
| L3 | import | scripts/verify-coordination-hints.js → scripts/coordination-hints-lib.js | 5 |
| L3 | import | scripts/verify-f3-design-readiness.js → scripts/f3-design-readiness.js | 3 |
| L3 | import | scripts/verify-skill-sentinel.js → plugins/session-state.js | 4 |
| L3 | reexport | repo-configs/forbidden-patterns.js → repo-configs/forbidden-patterns.core.js | GIT_MUTATION_VERBS |

(paths relative to `.opencode/`)

**After the 70 mechanical update-op rewrites: 61/61 modules FIT → 49/49 edges
at L1 = L2 = L3 (100%).** The entire as-is→100% gap is corpus-side single-token
edits; the grammar's enumerated exclusions require NO grammar work to lift.
[inference from the join; the rewrite itself was not performed — read-only
measurement]

---

## D. Lint-rule feasibility verdict (corpus CI enforcement surface)

**Feasible, and already demonstrated.** The census harness IS the lint: it is a
token-level checker built on the reference lexer plus two predicates, and it
produced zero false positives on the 61-module corpus (every update-op hit
manually line-verified; every ASD-definite candidate resolved by line
inspection).

Mechanically checkable at CI on `templates/core/.opencode/**` +
`templates/overlays/**` (the same walk corpus_test.go already does):

1. **Update-operator ban** — token-level (postfix/prefix distinguished by the
   lexer's PrefixOp flag); pure-grep `\+\+|--` is a weaker but viable
   approximation (comment/string false positives exist in prose-heavy files;
   the token-level check has none).
2. **ASI discipline** — the definite-ASI detector (reference predicates +
   control-paren refinement) fails on any statement-completable boundary
   lacking `;`. Current corpus passes 61/61.
3. **Regex-position check** — regex token whose prev-token is outside the
   operand-position set fails. Current corpus: 314/314 pass.
4. **Lexer-error = parse-refusal proxy** — any module that fails the reference
   lexer fails the lint (unknown-direction).

NOT lint-able (grammar-redesign / ratification items): admission of arrows,
control flow, try/catch, template-substitution recursion, regex operand
productions — these are grammar-SPEC decisions the text leaves open (§B gap);
a lint can only enforce them after the grammar freezes the productions
(brief.md:75 "freeze productions before counting").

---

## E. Old-preserved-file compatibility (bytes-not-ancestry)

### Historical corpus censuses (three eras, same census instruments)

| Era | Commit | Modules | FIT | FAIL | Update ops | asiD | Lexer errors |
|---|---|---|---|---|---|---|---|
| initial substrate | `07b05c7` | 29 | 28 | 1 | 4 (shell-guard-core L123,127,215,233) | 0 | 0 |
| pre-trilogy | `b788b43^` | 48 | 42 | 6 | 34 (shell-guard-core 14, shell-guard.js 10, complexity-signal-lib 6, verify-f3-authoring-surfaces L122, verify-no-source-tree-only L459, verify-release-interaction L72,174) | 0 | 0 |
| current | HEAD | 61 | 46 | 15 | 70 | 0 | 0 |

**No era ever relied on ASI (asiD=0 in all three), and no era has lexer
errors.** The update-op debt is old and era-stable — several verify-* cites are
byte-identical across pretrilogy and current (L122/L459/L72/L174) — i.e. the
corpus's grammar debt predates any recent style drift; it was never style-rule
gated. [inference from cite stability]

### state-lib.js across eras (the hub, the skew packet's subject)

- initial (5413 lines): **FIT** — 0 update ops
- pre-trilogy (8191 lines): **FIT** — 0 update ops
- current (10435 lines): 1 update op (L5554)

**The hub provider is conformant across its entire history.** Under
bytes-not-ancestry qualification (brief.md:71), an adopter's preserved
state-lib.js at ANY historical base parses under the grammar — its surface
stays assessable/closed; no mixed tree composed of any historical state-lib +
current siblings loses provider-side closure to grammar.

### Mixed-tree scan (the skew packet's adopter scenario, executed)

Constructed: current 61-module corpus with `state-lib.js` bytes replaced by the
`b788b43^` version (per
`researches/sources/2026-11-05-origin-hash-stale-preserve-skew.md` — the
adopters' base era; retention trilogy `b788b43`→`bb60068`→`cccf1b0`,
v0.29.0).

**Result: modules=61, edges=49, broken=1, unknown=5.**

> BROKEN: importer `verify-task-registry.js` (current) → provider
> `state-lib.js` (pre-trilogy), symbol **`collectDecisionItems`** — "importer
> requires named export; provider fully resolved export surface does not
> contain it (surface closed, so the absence is proven)".

The pre-trilogy surface has 68 export names vs the current 73; the missing
symbol is exactly the one the skew packet identified as added by `cccf1b0`
(`git log -S 'collectDecisionItems'` → cccf1b0 only). This is the
skew packet's Q2.4/2.5 link-time-break mechanism, **executed and proven** by
the reference scanner on an adopter-representable tree.

**[inference — grammar-level]** under the conformance grammar both endpoints
of that edge FIT (pre-trilogy state-lib: 0 excluded constructs; current
verify-task-registry: 0), so an L3-capable conformance-grammar checker
**catches this defect class**: preservation skew that silently disables
documented runtime gates today (skew packet verdict 2) becomes a PROVEN broken
edge at scan time. This is the first demonstrated case where the enforcement
the O3 direction exists to deliver fires on a real, non-hypothetical tree.

---

## F. The honest fraction answer (measured numbers, not a recommendation)

**"What fraction of L3 obligations would the grammar deliver?"**

- **As-is corpus, G-charitable reading: L3 = 10/49 enforceable (20.4%)**;
  recognized 32/49 (65.3%); assessable 10/49 (20.4%, = enforceable because
  closure is trivial here — no `export *` exists).
- **After 70 single-token mechanical rewrites (`++`→`+= 1`, `--`→`-= 1`) in 15
  files: 61/61 modules conform → 49/49 (100%) at L1 = L2 = L3.**
- **Under the G-strict literal reading: ≤3/61 modules conform (4.9%, pure
  config files) — the literal grammar text is not viable for this corpus
  without ratifying the constructs the corpus demonstrably uses** (arrows
  48/61, control flow 55/61, try 49/61).

**"Is that fraction worth a restart?"** — operator judgment; the measured
inputs:

For: (i) the as-is→100% gap is 70 counter-arithmetic tokens, zero grammar
work, and a lint the packet has already demonstrated; (ii) the corpus was
never ASI-dependent in any era and regex/template usage is uniformly in
admitted positions — the corpus is already near-conformant by discipline;
(iii) the mixed-tree run proves the enforcement catches a real defect class
(preservation skew) the current WARN baseline cannot see; (iv) the hub
provider is conformant across all history, so the L3 mechanism survives
adopter preservation skew.

Against / open: (v) the grammar's ADMISSIONS are unratified — a successor
parser must soundly implement arrows/control-flow/try/template-recursion/
regex-operand productions, and the G32 history shows misclassification defects
lived precisely in production-level decisions (slash-goal, update-op prefix/
postfix — brief.md:22); this packet measures corpus CONFORMANCE, not successor
parser SOUNDNESS; (vi) nothing here changes the 5 dynamic-import unknowns
(unknown-by-contract, grammar-independent); (vii) reaching 100% L3 in THIS
corpus says nothing about future corpus shapes that introduce `export *`
(where L2≠L3 becomes live); (viii) per the brief, any prototype work to build
the actual grammar parser needs separate authorization (brief.md:78) — this
packet is the census the restart evidence required (brief.md:57: "frozen
grammar and independently audited module/edge obligation ledger"; this packet
delivers the audited ledger and the conformance census, NOT the frozen grammar
— the spec gap in §B is the remaining pre-freeze item).

---

## Contradictions

- **None detected within the measurement.** All pins reproduced exactly
  (61/49/0/5); the join is internally consistent (17+22+10=49; per-surface
  39+10=49); era tallies sum (108 census update-ops = 4+34+70).
- **Prior-estimate contradiction (context, not measurement):** the brief
  recorded the prior 2/49 mid-tier estimate as non-defensible (brief.md:61);
  the measured as-is number is 10/49 L3 — both the 2/49 and 49/49 guesses were
  wrong for the as-is corpus.
- **Doc-vs-reality note:** `state-lib.js` at HEAD is 10435 lines; the census
  bound its exclusion debt to one token — contrary to any intuition that the
  largest file carries the most grammar risk (it carries the least: 1 op in
  10.4k lines; the debt concentrates in small shell-guard/auto-gate files).

## Findings (structured register)

- **(finding)**: source=ledger.json scanCounts vs corpus_test.go:33-41, confidence=high, type=fact — reference ledger 61/49/0/5 reproduced exactly from tree assembly mirroring corpus_test.go:190-254; zero destination collisions; zero unparseables.
- **(finding)**: source=ledger.json edges[], confidence=high, type=fact — all 49 edges have provider present+parseable+CLOSED under graph.go:237-305/376-460 semantics; corpus contains zero `export *` and zero namespace re-exports; L2≡L3 in this corpus.
- **(finding)**: source=ledger.json edges[] (state-lib hub), confidence=high, type=fact — 19/49 edges point into state-lib.js (up to 44 symbols/importer); state-lib imports 3 modules.
- **(finding)**: source=census.json totals, confidence=high, type=fact — update operators are the ONLY enumerated-grammar violation in the current corpus: 70 sites / 15 files; asiDefinite=0 after control-head refinement (11 sites verified as if-heads); 314/314 regexes in operand positions; 0 lexer errors.
- **(finding)**: source=census.json keywords + arrow counts, confidence=high, type=fact — grammar-unspecified construct usage: arrows 48/61, if/for/while 55/61, try 49/61; strict-literal conformance ≤3/61.
- **(finding)**: source=leveljoin.jq join output, confidence=high, type=fact (join) — as-is edge classification L0=17 / L1=22 / L3=10; core 39 edges = 7+22+10; overlays 10 edges all L0; 22 of 39 degraded edges trace to state-lib.js L5554 alone.
- **(inference)**: source=join + rewrite analysis, confidence=high, type=inference — after the 70 mechanical `+= 1`-class rewrites, 61/61 conform → 49/49 L1=L2=L3; the rewrite was not executed (read-only).
- **(finding)**: source=census.json hist eras, confidence=high, type=fact — era tallies: initial 28/29 FIT (4 ops), pretrilogy 42/48 FIT (34 ops), current 46/61 FIT (70 ops); asiD=0 and lexer errors=0 in ALL eras; state-lib.js FIT at initial and pretrilogy, 1 op at current.
- **(finding)**: source=ledger-mixed.json + harness BROKEN-EDGE print, confidence=high, type=fact — mixed tree (current + pretrilogy state-lib): broken=1, verify-task-registry.js → state-lib.js missing collectDecisionItems (68 vs 73 export names; symbol added by cccf1b0 per skew packet).
- **(inference)**: source=mixed-tree run + census fit verdicts, confidence=high, type=inference — the skew defect class is L3-provable under the conformance grammar (both endpoints fit); grammar enforcement survives preservation skew via bytes-not-ancestry.
- **(inference)**: source=§B method, confidence=medium, type=inference (method limit) — "fits" is a construct-census proxy, not a grammar-parser run; high confidence for enumerated exclusions, none for unspecified constructs.

## Verification

| Claim | Verifying command/output | Verified |
|---|---|---|
| Ledger pins reproduced | `bash tmp/eslink-o4-conformance-packet/runcensus.sh`-style harness run: `modules=61 edges=49 broken=0 unknown=5 collisions=0`; ledger.json scanCounts | yes |
| 49 edges all closed/present/parseable | `jq '.edges[] | select(.providerClosed!=true)' ledger.json` → empty | yes |
| No `export *` corpus-wide | `jq '[.modules[] | select((.starReexports\|length)>0]] \| length' ledger.json` → 0 | yes |
| 70 update ops / 15 files / per-file cites | census.json report.updateOps; table §B | yes |
| asiDefinite=0 (post-refinement) | census.json `.report.asiDefinite` empty ∀ modules; 11 initial hits line-verified as if-heads | yes |
| 314 regexes all operand-position | census.json regexes[].prevToken histogram (§B) | yes |
| L-join L0/L1/L3 = 17/22/10 | `jq -f leveljoin.jq --slurp census.json ledger.json` | yes |
| Era tallies (29/48/61) | census.json per-prefix groups (§E) | yes |
| Mixed-tree broken=1 collectDecisionItems | runmixed.sh run: `BROKEN-EDGE importer=verify-task-registry.js provider=state-lib.js symbol=collectDecisionItems`; ledger-mixed.json scanCounts | yes |
| state-lib export surfaces 68 (mixed) vs 73 (current) | jq exportNames length on ledger-mixed.json / ledger.json | yes |
| Harness uses verbatim reference implementation | harness/eslink/{lexer,parser,graph}.go copied unmodified from internal/eslink (census.go is a separate additive file) | yes |

**Reproduction** (exec-sandbox, kernel-contained): `vh-agent-harness
exec-sandbox bash tmp/eslink-o4-conformance-packet/runcensus.sh` (census,
current + historical eras → census.json) and `... runmixed.sh` (mixed tree →
ledger-mixed.json). Ledger harness: same pattern with `go run .` in
`harness/` (env: GOTOOLCHAIN=local, caches under tmp/; see drivers).
Machine artifacts: `ledger.json`, `census.json`, `ledger-mixed.json`,
`census-stdout.txt`, `leveljoin.jq`, `hist/{initial,pretrilogy}/`.

## Recommended artifact type / next steps

- **Artifact type:** sources (this packet). Durable path:
  `researches/sources/2026-11-08-eslink-o4-corpus-conformance-closure-coverage.md`
  (one-step move for the build slice).
- **Next specialist per the brief's sequence** (brief.md:99-105): this packet
  returns to **debate** (measured-fraction deliberation), then **planner**;
  the remaining pre-freeze item is the grammar-spec ratification set (§B gap:
  arrows / control flow / try / template recursion / regex operand
  productions) — an operator decision, not a research one.
- **Promotion targets (if/when adopted):** a corpus CI lint derived from the
  census instruments (§D) on `templates/core/.opencode/**` +
  `templates/overlays/**`; the 70 update-op rewrites; pins update in
  `internal/eslink/corpus_test.go` only if a rewrite changes any parse shape
  (it should not — `+= 1` is orthographic).
- No repo, git, backlog, or coordination state was modified by this pass. All
  outputs live under `tmp/eslink-o4-conformance-packet/`.
