# O4 eslink — Stage-4 §C Ratification: Option-2 Hub-Token Normalization

- **Date:** 2026-11-09
- **Type:** decision record (ratification)
- **Status:** RATIFIED — operator ratification "go" on 2026-11-09, this slate verbatim
- **Effect:** selects option 2 — the single hub-token normalization
  `state-lib.js:5554` `i++` → `i += 1` — as the staged-restart §C stage-4
  disposition, and fixes its sequencing, delivery gate, rewrite-slice
  obligations, and flip conditions. This record does NOT open the S2–S4
  scopes (ratified separately), does not land the production rewrite itself,
  and does not authorize enforcement wiring — the rewrite lands as its own
  gated slice under the obligations in slate item 4.

## Provenance chain

`staged-restart spec 2cae4a2 §C` (option 1 now — zero rewrites; normalization
revisited at stage 4 with measured S1′ results, no conditional commitment to
option 3)
→ `D pass landed 2f5b304` (variant-to-production map + export-star fixture
designs) `→ A variant freeze 12d8195` `+ TLA amendment 8b1bd58`
→ `S1′ landed d5882ded` (frozen-grammar successor parser + corpus lint
harness; the seven §B gates landed green; zero production rewrites — the
option-1 baseline held through S1′)
→ `stage-4 debate` (slate: option-2 lean, subject to a measured-first
precondition — no ratification on census projections alone)
→ `candidate measurement landed f2c3700`
(`researches/sources/2026-11-09-eslink-stage4-candidate-measurement.md` —
successor-run numbers for candidates 2 and 3, every stage digest-bound)
→ `operator ratification "go", 2026-11-09` (this slate, verbatim).

The debate slate and the operator "go" are parent-session/operator records
not landed as separate repo files; this decision record is their authoritative
carrier — same convention as the staged-restart spec's handling of its
parent-session debate IDs. The measured inputs are NOT parent-session
material: the receipt is the committed file at `f2c3700`, byte-identical at
HEAD (sha256 `85de5b43…3700c6`, verified this slice).

## The ratified slate (verbatim)

1. **Option 2 chosen — hub token normalization.** `state-lib.js:5554`
   `i++` → `i += 1`; one production file
   (`templates/core/.opencode/scripts/state-lib.js`, the file's sole `i++`
   site, line-anchored).
2. **Sequencing.** Normalization lands before S2–S4 delivery; S2–S4 scopes
   are defined and ratified separately — NOT opened by this record.
3. **Delivery gate.** The docs-key card (07-26-54) and the warning-reword
   card (07-26-46) land before/with consumer delivery of hub churn; a
   migration notice + erratum are mandatory, with content enumerated:
   - grammar-only intent (no behavior change claimed or verified);
   - the exact one-token diff (`i++` → `i += 1` at state-lib.js:5554);
   - the hub origin-hash advance at the next render;
   - the fresh stale-preserve consequence for preserved-hub adopters;
   - safeguard-first reconciliation (preserved adopter bytes are reconciled
     safely, never steamrolled);
   - the correct customization routes — additive overlay vs same-path
     `project_owned`;
   - the link-integrity-not-behavior-verification rider (the notice verifies
     link integrity; it does not claim behavior verification).
4. **Rewrite-slice obligations.** Semantic-preservation verification; the
   mixed-tree fixture's `i++` anchor adapted honestly with its
   closed-provider/missing-symbol assertions retained; pins re-justified
   independently (never repinned to absorb movement); gates retained.
5. **Flip conditions.** Hold at zero (option 1, no rewrite) if semantic
   preservation fails or the delivery gate is unavailable; prefer option 3
   only on explicit operator valuation of the extra 17 non-hub edges;
   wire-first only on integration evidence.

## Measured basis

All numbers cite the landed receipt
`researches/sources/2026-11-09-eslink-stage4-candidate-measurement.md` @
`f2c3700` (successor parser runs on digest-bound scratch trees; baseline
binding: scratch corpus == embedded corpus, digest == gate-4 pin). Nothing
below is re-derived here.

| Stage | Enforceable L3 | FIT / UNKNOWN | L-join (L0/L1/L3) | Broken | Tree digest (sha256) |
|---|---|---|---|---|---|
| Baseline (option 1, as-landed) | 10/49 (20.4%) | 45/16 | 17/22/10 | 0 | `fea47acd172c81a28d42112135033d567f4bd0e47d30f45f13f4bc667834f095` (== gate-4 pin) |
| **Candidate-2 (hub token only) — RATIFIED** | **32/49 (65.3%)** | 46/15 | 14/3/32 | 0 | `70ceca3c646734bcb3b64b4000316e1ee29f4256b42d66f60997f183c020a4e9` |
| Candidate-3 (all 70 sites) | 49/49 (100%) | 60/1 | 0/0/49 | 0 | `2701d638ff7d0242af219091952b27d26655f4a11999b8134219b652eb4302bc` |

- **Candidate-2 recovery composition (measured, enumerated in the receipt):**
  19 incoming hub consumer edges L1→L3 — including
  `verify-task-registry.js` → `state-lib` (`collectDecisionItems`) — plus 3
  hub-outgoing edges L0→L3; 10 existing L3 untouched. The hub itself flips
  UNKNOWN(update-op L5554) → FIT, closed, **73 named exports**. Broken
  stays 0 — closure introduced zero false positives across all 19 importers
  (up to 44 symbols each).
- **Candidate-3 (measured, not chosen):** 49/49 edges across 15 files; the
  2 value-used prefix sites (`session-progress.js:492` `seq: ++st.seq`,
  `check-defer-triggers.mjs:344` `argv[++i]`) were measured with
  mechanically-flipped, **semantically unreviewed** forms;
  `compaction-primitives.js` stays UNKNOWN and holds module FIT at 60/61 —
  it participates in **0 ledger edges**, which is why 49/49 edges holds
  despite it.
- **Honest caveats carried from the receipt:** the prefix-site forms are
  non-semantic (parse-FIT ≠ semantic equivalence; production rewrite of
  those sites would need §B(7) manual semantic review — moot under option 2,
  which touches neither); module FIT is 60/61, not the census-projected
  61/61 (the mis-lexed-FIT census artifact); and the census §B
  shell-guard.js cite list omitted L204 (count right, enumeration nit).

## Corrections carried

Three misconceptions surfacing in the stage-4 debate are corrected here so
they do not propagate into the rewrite slice:

1. **The baseline L3 = 10 composition touches NO state-lib edges.** All 10
   baseline-enforceable edges are non-hub; the hub's degraded edges (19
   incoming + 3 outgoing = 22 of 39) are exactly the set the option-2 token
   recovers. Option 2 does not "improve" existing L3 edges — it recovers
   hub-blocked ones.
2. **The historical skew catch works regardless of normalization.** The
   mixed-tree `collectDecisionItems` skew catch (gate 5) fires on tree-shape
   mismatch — a preserved-hub adopter still holds the OLD hub bytes, which
   remain UNKNOWN under the frozen grammar whether or not current platform
   bytes are normalized. Normalization broadens current-byte hub coverage
   only; the catch is not contingent on this decision.
3. **Option-2-then-3 does NOT churn the hub twice.** If option 3 is later
   preferred on explicit operator valuation, its incremental flip is 69
   non-hub sites — the hub token is already normalized and is a no-op for
   the second wave. Option 3's edge over option 2 is one review wave, not
   one hub event; the hub's origin hash advances once per actual hub edit.

## Contradictions

- **None material.** Both §C projections reproduced exactly under the
  successor parser: candidate-2 32/49 (10 + 19 + 3) and candidate-3 49/49
  edges. Carried nits (non-contradictions, already dispositioned in the
  receipt): the census 61/61 module-FIT line vs measured 60/61
  (compaction-primitives invalid-JS artifact, 0 edges), and the census §B
  L204 cite omission (count right).

## Findings

- **(finding)**: source=receipt @ `f2c3700`, confidence=high, type=fact — baseline run reproduced every gate-4 pin (61/45/16 · 49 · 17/22/10 · 5 · 0) and every per-module first-failure reason; scratch digest == gate-4 pin `fea47acd…f095`.
- **(finding)**: source=receipt @ `f2c3700` cand-2 run, confidence=high, type=fact — hub-token flip yields 32/49 L3 (recovery 19 L1→L3 + 3 L0→L3, enumerated), hub FIT closed with 73 exports, broken 0; digest `70ceca3c…0a4e9`.
- **(finding)**: source=receipt @ `f2c3700` cand-3 run, confidence=high, type=fact — all-70 flip yields 49/49 L3, 60/1 FIT, broken 0; digest `2701d638…302bc`; 2 value-used prefix sites semantically unreviewed in measurement.
- **(inference)**: source=measured table join, confidence=high, type=inference — option 3's marginal value over option 2 = 17 non-hub edges (49 − 32); this is the quantity the flip conditions require the operator to value explicitly before any option-3 preference.
- **(finding)**: source=this session's git/fs receipts (Verification table), confidence=high, type=fact — provenance commits verified in lineage (`2cae4a2`, `2f5b304`, `12d8195`, `8b1bd58`, `d5882ded`, `f2c3700`); receipt byte-identical at HEAD; both delivery-gate cards exist on disk; hub anchor `state-lib.js:5554` confirmed in the working tree (the file's sole `i++`).
- **(assumption)**: source=stage-4 debate + operator directive carried in this slice, confidence=high, type=assumption — the debate slate (option-2 lean, measured-first precondition) and the operator "go" of 2026-11-09 are parent-session/operator records; this record is their authoritative carrier (provenance convention above), not a re-derivation.

## Verification

| Claim | Verifying command/output | Verified |
|---|---|---|
| Receipt landed byte-verbatim at `f2c3700` | `git show f2c3700:researches/sources/2026-11-09-…-measurement.md \| sha256sum` → `85de5b43…3700c6` == tmp-source digest; `\| cmp - tmp/…` → identical (run this slice) | yes |
| Receipt cited at HEAD is the committed bytes | `git diff f2c3700 HEAD -- researches/sources/2026-11-09-…-measurement.md` → empty (run this slice) | yes |
| Provenance: staged-restart spec at `2cae4a2` (§C option-1-now, stage-4 revisit) | `git show --stat --oneline 2cae4a2` → adds `researches/decisions/2026-11-08-eslink-o4-staged-restart-spec.md`, 260 insertions (run this slice) | yes |
| Provenance: S1′ successor parser at `d5882ded` (gates landed) | `git show --stat --oneline d5882ded` → adds `internal/eslink/{corpus,fixtures,negatives,ledger_golden}_test.go` + parser sources (run this slice) | yes |
| Variant freeze lineage (D pass → freeze → amendment → S1′) | `git log --oneline -6` → `2f5b304`, `12d8195`, `8b1bd58`, `d5882ded` in order (run this slice) | yes |
| Hub anchor: `state-lib.js:5554` is the sole `i++` in one production file | `grep -n "i++" templates/core/.opencode/scripts/state-lib.js` → only 5554 (`for (let i = 0; i < s.length; i++) {`) (run this slice) | yes |
| Delivery-gate cards exist | `ls .local/coordinator/tasks/*07-26-54* *07-26-46*` → both cards present (docs-key runbook ship; stale-preserve warning/reword + routing) (run this slice) | yes |
| Baseline digest == gate-4 pin `fea47acd…f095` | receipt §digest-receipts @ `f2c3700`; cross-checked by the landing reviewer against `pinnedCorpusDigest` in `internal/eslink/corpus_test.go` @ `d5882ded` | yes (cited) |
| Cand-2 32/49 = 10 + 19 + 3; hub closed 73 exports; broken 0 | receipt §measured-table + §recovery-set @ `f2c3700` (19 consumer edges + 3 outgoing enumerated; `verify-task-registry` → `collectDecisionItems` is #18) | yes (cited) |
| Cand-3 49/49, 15 files, 2 prefix sites unreviewed; compaction-primitives UNKNOWN at 0 edges | receipt §measured-table + §honest-caveats @ `f2c3700` | yes (cited) |
| Option-3 marginal = 17 non-hub edges | arithmetic on the measured table (49 − 32) | yes (derived) |
| S1′ gates green at landing | landed gate surface at `d5882ded` (four test files incl. ledger golden pins); not re-run this slice — the working tree carries a concurrent session's in-flight `internal/eslink/fixtures_test.go` edits, so a current-tree run would not be attributable to this record | yes (cited, not re-run) |
| Operator ratification "go" 2026-11-09 | operator directive carried in this slice's mission; parent-session/operator record per the provenance convention above, not a repo-verifiable artifact | attested (not repo-verifiable) |
