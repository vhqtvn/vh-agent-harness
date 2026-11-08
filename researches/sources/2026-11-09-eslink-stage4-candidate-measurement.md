# Stage-4 §C measurement — successor-run normalization numbers (MEASURED, not projected)

Task: convert the staged-restart spec §C census projections to measured successor-run
numbers for candidates 2 and 3, per the debate slate's measurement precondition.

- **Bound revision**: `d5882de` (successor parser landed), clean working tree.
- **Parser**: the landed `internal/eslink` library (grammar.go, lexer.go, parser.go,
  graph.go) — byte-exact copies compiled in a scratch module under
  `tmp/eslink-stage4-measure/` (deleted after this report was written; recipe below).
- **Baseline binding**: the scratch disk tree (templates/core/.opencode + all
  templates/overlays packs, assembled exactly like `internal/eslink/corpus_test.go`)
  hashed to the gate-4 pinned digest — scratch == embedded corpus, byte-bound.
- **Toolchain honesty note**: the repo module requires go ≥ 1.25.0 but the reachable
  toolchain surface for this read-only session was go1.22.3 (system shim; sandbox
  blocked the module-cache 1.25 toolchain). eslink is stdlib-only (fmt/strings/path/
  sort) and compiles unchanged under 1.22 in the scratch module. Behavioral binding:
  the baseline run reproduced EVERY gate-4 pin (61/45/16 · 49 · 17/22/10 · 5 · 0) AND
  every per-module first-failure reason/line (e.g. state-lib.js:5554 update-op,
  compaction-primitives.js line-18 `expected ",", found "vh"`) — identical verdicts on
  the identical digest-bound tree.

## Digest receipts (each run bound to its scratch tree)

| Stage | Tree digest (sha256, corpusDigest format) |
|---|---|
| 0 — baseline | `fea47acd172c81a28d42112135033d567f4bd0e47d30f45f13f4bc667834f095` (== gate-4 pin) |
| 2 — cand-2 (hub token) | `70ceca3c646734bcb3b64b4000316e1ee29f4256b42d66f60997f183c020a4e9` |
| 3 — cand-3 (all sites) | `2701d638ff7d0242af219091952b27d26655f4a11999b8134219b652eb4302bc` |

## Measured table

| Metric | Baseline (pin) | Candidate-2 (hub token only) | Candidate-3 (all 70 sites) |
|---|---|---|---|
| modules | 61 | 61 | 61 |
| FIT / UNKNOWN | 45 / 16 | **46 / 15** | **60 / 1** |
| ledger edges | 49 | 49 | 49 |
| L0 / L1 / L3 | 17 / 22 / 10 | **14 / 3 / 32** | **0 / 0 / 49** |
| enforceable L3 | 10/49 (20.4%) | **32/49 (65.3%)** | **49/49 (100%)** |
| hub state-lib.js | UNKNOWN(update-op L5554) | FIT, closed, 73 named exports | FIT, closed, 73 named exports |
| broken | 0 | 0 | 0 |
| dynamic unknowns | 5 | 5 | 5 |
| unknown-by-class | dyn=5 open=22 unsup=16 | dyn=5 open=3 unsup=15 | dyn=5 open=0 unsup=1 |

- Candidate-2 surgery: state-lib.js:5554 `for (…; i++) {` → `i += 1` (gate5-canonical
  line-anchored rewrite; anchor line asserted before surgery).
- Candidate-3 adds: 69 mechanically flipped sites (61 postfix `++`, 6 postfix `--`,
  2 prefix `++`) + the hub token from cand-2 = **70 total** — matching the census §B
  count exactly (68 postfix + 2 value-used prefix, 0 prefix `--`).

## Projection verdicts

- **Candidate-2 projection L3 32/49: CONFIRMED.** Delta = 19 L1→L3 consumer edges +
  3 L0→L3 outgoing hub edges; 10 existing L3 untouched (none touch state-lib).
- **Candidate-3 projection 49/49: CONFIRMED (edges).** cand2→cand3 delta = 14 L0→L3 +
  3 L1→L3 (complexity-signal-lib ×2 consumers + shell-guard-core's eval.js consumer).
- **Module-count footnote**: the packet census's "61/61 FIT after rewrites" does NOT
  reproduce — 60/61. The single holdout is `plugins/compaction-primitives.js`
  (objectively invalid JS, unbalanced template literal — the mis-lexed-FIT census
  artifact already documented in corpus_test.go's header; carded separately). It
  participates in **0 ledger edges**, which is why the 49/49 edge projection holds
  despite it. Broken stays 0 at every stage — closure introduced **zero** false
  positives, including on the closed hub surface (all 19 importers' required names,
  up to 44 symbols, are present).

## Candidate-2 recovery set (22 edges, enumerated)

L1→L3 (19 consumers of state-lib.js; symbols in the run output, summarized here):
1. plugins/session-state.js → state-lib (3)
2. scripts/approve-plan.js (4)
3. scripts/draft-plan.js (4)
4. scripts/plan-adopt.js (4)
5. scripts/plan-list.js (6)
6. scripts/plan-resolve.js (4)
7. scripts/plan-save.js (4)
8. scripts/session-bind.js (4)
9. scripts/session-current.js (4)
10. scripts/verify-coordinator-adoption-marker.js (3)
11. scripts/verify-f3-dispatch-backstop.js (13)
12. scripts/verify-f3-plan-approve.js (9)
13. scripts/verify-f3-task-ready.js (8)
14. scripts/verify-recurrence-dedup.js (3)
15. scripts/verify-session-state.js (29)
16. scripts/verify-skill-proposals.js (8)
17. scripts/verify-state-validation.js (6)
18. scripts/verify-task-registry.js (16 — incl. collectDecisionItems)
19. tools/plan-state.js (44)

L0→L3 (3 outgoing): state-lib → scripts/f3-design-readiness.js (2),
scripts/pause-new-work.js (2), scripts/rewrite-parity-validate.js (2).

Remaining degraded after cand-2 (L1=3): plugins/shell-guard/eval.js →
shell-guard-core (complexity hub), coordination-hints-lib → complexity-signal-lib,
verify-coordination-hints → complexity-signal-lib. All three recover in cand-3.

## Candidate-3 flip enumeration (69 sites; all before→after pairs in run output)

Per-file measured counts (census §B in parens): shell-guard-core 14 (14) ·
shell-guard 10 (10) · check-defer-triggers.mjs 8 (8) · complexity-signal-lib 6 (6) ·
check-s2-holds 1 (1) · verify-f3-authoring-surfaces 1 (1) ·
verify-no-source-tree-only-paths 1 (1) · verify-release-interaction-contract 2 (2) ·
auto-gate-live 7 (7) · auto-gate-tiered 7 (7) · auto-tool-gate 2 (2) ·
session-progress 2 (2) · session-progress-judge 5 (5) · session-progress-policy 3 (3).
(state-lib: its 1 site is the cand-2 hub token.)

Line-for-line match with the census cites in 14 of 15 files. The one enumeration
discrepancy: **the census §B table's shell-guard.js cite list names 9 lines but its
count (10) is right — the missing cite is L204 (`" \t\n\r".includes(s[j])) j++;`),
measured and flipped.** (The harness's on-screen "DIVERGES" markers were its own
bookkeeping artifacts: a map-key typo — `verify-no-source-tree-only-paths` missing
its `.js` suffix — and the state-lib 0-remaining being the expected cand-2
precondition. Neither is a corpus divergence.)

## Honest caveats

1. **The 2 value-used prefix sites were measured with mechanically-flipped,
   semantically UNREVIEWED forms** — `session-progress.js:492` `seq: ++st.seq` →
   `seq: st.seq += 1` and `check-defer-triggers.mjs:344` `argv[++i]` →
   `argv[i += 1]`. Both parse FIT under the frozen grammar (corpus run + standalone
   micro-probes) — so 49/49 does not hinge on any further grammar admission — but
   this measurement validates **parseability/closure only**. A production rewrite of
   these two sites requires the stage-4 §B(7) semantic-preservation protocol (manual
   semantic review); no semantic equivalence is claimed here.
2. **compaction-primitives.js stays UNKNOWN** (invalid JS, unrelated to update ops)
   and holds module FIT at 60/61; the 49/49 edge number is unaffected (0 edges).
3. The census "61/61 FIT" projection line was the packet's mis-lexed-FIT artifact
   (already documented at corpus_test.go:14-23); the successor's honest number is
   60/61, edges 49/49.
4. Flipper is string/comment-aware (verified: `"--"` string literals on flipped
   lines, e.g. shell-guard-core.js:1039/1066/1080/1185/1209, were preserved; comment
   mentions of `i++` on shell-guard.js:165/170 untouched). No string/comment bytes
   were modified; the changed-line list (69 entries) is exactly the code-site list.
5. Measurement ran the copied library under go1.22.3 (see toolchain note above);
   baseline pin + full per-module verdict identity is the binding evidence the copy
   behaves as the landed parser on this tree.

## Contradictions

- None material. The two §C projections reproduce exactly. Minor: census §B
  shell-guard.js cite-list omission (L204) — count correct, enumeration corrected
  above.

## Findings

- **(finding)**: source=this run (digest-bound scratch tree, eslink @ d5882de), confidence=high, type=fact — baseline pins reproduced exactly: 61/45/16 · 49 (17/22/10) · 5 · 0; digest == gate-4 pin.
- **(finding)**: source=this run cand-2, confidence=high, type=fact — hub-token flip yields 46/15 FIT, L-join 14/3/32 (32/49 L3), broken 0, hub FIT+closed with 73 exports; recovery set = 19 L1→L3 + 3 L0→L3, enumerated.
- **(finding)**: source=this run cand-3, confidence=high, type=fact — all-70 flip yields 60/1 FIT, L-join 0/0/49 (49/49 L3), broken 0; total sites 70 = 68 postfix + 2 prefix++, 0 prefix--; census per-file counts all match.
- **(finding)**: source=this run, confidence=high, type=fact — compaction-primitives.js remains UNKNOWN and touches 0 ledger edges.
- **(caveat/assumption)**: source=task framing + census §B, confidence=high, type=assumption — the 2 prefix-site mechanical forms are semantically unreviewed; parse-FIT ≠ semantic equivalence; production rewrite needs §B(7) review.
- **(finding)**: source=census §B vs this run, confidence=high, type=fact — census cite list omits shell-guard.js:204 (its own count includes it).

## Reproducibility (scratch deleted; recipe)

Scratch module `tmp/eslink-stage4-measure/` (go.mod `module stage4measure`, `go 1.22`),
`eslink/` = byte-copies of internal/eslink {grammar,lexer,parser,graph}.go @ d5882de,
`main.go` = assembler mirroring corpus_test.go's walk (templates/core/.opencode +
each templates/overlays/<pack>/ → `.opencode/…` keys, .js/.mjs, 61-module assert,
corpusDigest-format digest + pin check), gate5-canonical hub surgery, string/comment-
aware mechanical flipper, Summarize/EdgeLedger per stage, edge-key diffs. Run via
`vh-agent-harness exec-sandbox` with GOCACHE/GOTMPDIR under tmp (go1.22.3,
GOTOOLCHAIN=local, GOPROXY=off). Deleted after numbers were recorded here; digests
above re-bind any re-run.
