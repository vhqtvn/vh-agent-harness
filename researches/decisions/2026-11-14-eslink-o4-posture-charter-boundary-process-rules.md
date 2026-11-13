# O4 eslink — Scope-Audit Posture: Charter Boundary, Product Freeze, Process Rules (P1/P2)

- **Date:** 2026-11-14
- **Type:** decision record (operator-ratified posture landing; the ONE primary record for this slice)
- **Status:** RATIFIED — operator "all as recommended" on 2026-11-14
- **Effect:** closes the O4 scope-audit debate (2026-11-14, evidence-bound, verdict
  MIXED) by landing ONE primary decision record carrying the ratified posture:
  the O1 product disposition (narrow enforcement kept as landed; discretionary
  grammar expansion FROZEN), the charter boundary (the P3 substance — pinned in
  the same slice as one line in `.vh-agent-harness/AGENTS.mission.md` → root
  `AGENTS.md` via `make update`), and the P1/P2 process rules going-forward.
  The existing 2026-11-07..13 eslink record history is preserved as-is — this
  record consolidates POSTURE, not history, and is not a retroactive merger.
- **Naming note:** the audit's option labels O1–O4 below are debate options and
  are distinct from the initiative name "O4" (the eslink scope number carried
  since the 2026-11-07 preflight).

## Provenance chain

```
O4 staged arc (2026-11-07..13, all landed):
  preflight feasibility → staged-restart 2cae4a2 → variant map 2f5b304
  → variant freeze 12d8195 → TLA amendment 8b1bd58 → S1′ parser d5882de
  → stage-4 option-2 hub-token normalization 4469f3c → S0 ff2f577
  → S2 9853ea8 → S3 f9a6438 → S4 b1ca98c + 62b2d56
  → vendor-unknown aggregation amendment 48b6c30
→ scope-audit debate 2026-11-14 (evidence-bound; verdict MIXED; carried by
   the coordinator dispatch — parent-session convention, same as the vendor
   amendment record's operator-intent finding)
→ operator ratification "all as recommended" 2026-11-14 — this record is the
   authoritative carrier of the ratified posture.
```

The debate surveyed the landed arc above against its own receipts (footprint
numbers, freeze/amendment records, commit SHAs) and returned a MIXED verdict:
keep the landed enforcement shape, but freeze discretionary growth and pin the
charter boundary plus process discipline. The operator ratified every
recommendation as stated.

## The ratified posture

### O1 (product) — ADOPTED: keep narrow enforcement as landed

The landed enforcement chain stands exactly as shipped, with no widening and
no rollback:

| Stage | Commit | What landed |
|---|---|---|
| S0 | `ff2f577` | standalone U+2028/U+2029 identifier-absorption lexer fix |
| S2 | `9853ea8` | pre-write refusal of cohort-breaking updates + `--allow-broken-cohort` escape |
| S3 | `f9a6438` | doctor check #28 (cohort-links) WARN on proven platform-edge breakage |
| S4 | `b1ca98c` + `62b2d56` | cohort honesty contract pinned + render/ledger sync for the honesty docs |
| vendor | `48b6c30` | cohort-report aggregation of vendor unknowns (a+b+c amendment) |

Three binding sub-rules:

1. **Discretionary grammar expansion is FROZEN.** The frozen-grammar variant
   set stands as amended through `8b1bd58`; grammar extensions happen ONLY via
   a genuine-authority amendment with fresh operator approval —    the mechanism
   the freeze record itself pins ("Any spec change after this point requires
   an amendment to this record plus renewed operator approval", freeze record
   :146-148, restated at :11-12; executed once by the TLA amendment
   `8b1bd58`). No
   coverage-driven, convenience, or aesthetic admissions.
2. **Parser-maintenance ownership is named.** `internal/eslink/` is an owned
   subsystem maintained on the eslink lane; its coverage debt is tracked via
   cards (e.g. `defer-eslink-binaryin-forhead-fixture`,
   `defer-eslink-dynimport-escaped-literal`, and the eslink fixture/task
   cards), NEVER retired by default grammar expansion.
3. **The accepted-UNKNOWN policy stands.** Unknowns stay WARN-side and never
   refuse (clause U6); vendor unknowns aggregate per the `48b6c30` record
   (a+b+c: WARN baseline, vendor-dir unknowns collapsed to one stated-count
   aggregate line, proven edges always itemized).

### Charter boundary (the P3 substance)

**eslink exists for ONE purpose: compatibility of the manager's own output
across managed/preserved boundaries.** It is never a general JavaScript
analyzer and never a semantic gate verifier — the honesty rider
(`link integrity only; behavior of preserved files is not verified`) carried by
every cohort message per the `48b6c30` record is the standing acknowledgment
of that boundary, not a disclaimer bolted on after the fact.

Signal-quality debt is accepted under this charter and kept visible:
the dogfood baseline of **3 proven `node_modules/yaml` edges + 1,104 unknown
findings** (20 itemized non-vendor + 1,084 vendor-aggregated, 954 modules
scanned) is accepted, visible, and non-suppressed. Nothing is hidden: WARN
fires, totals count every finding, proven edges itemize wherever they sit.
Reducing that debt by grammar growth is exactly what the O1 freeze forbids;
the debt is the price of the charter and is paid in the open.

The boundary is pinned durably in the same slice: one line under
Non-negotiable rules in `.vh-agent-harness/AGENTS.mission.md` (composed into
root `AGENTS.md` via `make update`), so future sessions inherit the charter
as mission rules rather than rediscovering it in this archive.

### P1 (process, going-forward): one primary decision record per slice

- Each slice lands **ONE primary decision record**. Ancillary records for the
  same slice are not created; the primary record carries provenance,
  disposition, flip conditions, and verification.
- **Amendments are reserved for genuine authority changes** (a ratified
  disposition actually changing, as in the TLA amendment `8b1bd58`) — not for
  supplementing, clarifying, or extending a landed record. Landing-time
  corrections happen BEFORE the record commits, in the working tree, while it
  is still the draft of the one primary record.
- **Existing history is preserved as-is.** The 2026-11-07..13 eslink records
  (feasibility, staged restart, variant map, freeze, TLA amendment, stage-4
  option-2, S2–S4 wiring scope, vendor amendment) are NOT consolidated
  retroactively; they remain the authoritative per-slice records of their own
  ratifications. This record is the posture layer on top, not a replacement.

### P2 (process, going-forward): bounded review cascades

- Reviews are bounded by the ratified cascade budgets (per-slice budgets as
  ratified — e.g. the S2–S4 packet's 1+2+2+2 and the staged-restart §B hard
  maximum of two cascades).
- **Exceptions must stop, split, or be explicitly reauthorized by the
  operator.** A review that exhausts its budget does not roll into an
  open-ended convergence attempt; the slice either lands within budget,
  splits into separately reviewable slices, or halts for operator
  reauthorization.
- No open-ended convergence attempts: "one more pass" beyond budget is the
  failure mode this rule exists to kill.

### Audit corrections recorded

The audit corrected two of its own load-bearing framings; both corrections
are part of the ratified posture:

1. **"corpus must conform forever" was too strong.** The designed escape is
   reason-coded UNKNOWN with WARN-side reporting — corpus constructs outside
   the frozen subset land `UNKNOWN`, never refusal, never FAIL (freeze record
   :75, :100, :191; clauses U4/U5/U6). Conformance is enforced at the
   platform-endpoint boundary, not demanded of the whole corpus.
2. **Normalization was ONE hub token, not wholesale.** The production-rewrite
   disposition that actually landed is the single hub-token normalization
   `state-lib.js:5554` `i++` → `i += 1` (`4469f3c`, ratified as stage-4
   option 2). No wholesale corpus rewrites occurred or are implied.

## Flip conditions (verbatim intent from the audit)

- **Toward WARN-default** (loosening refusal): on reproducible false-positive
  refusals, persistent override needs, or adopter policy demands.
- **Toward retention** (keeping refusal posture): on real adopter structural
  catches.
- **Against further grammar investment**: on amendment overruns without
  catches (amendments that outpace their budget or multiply without landing
  real defect catches).
- **Against process trim**: if consolidated records lose authority
  distinctions (single records blurring which authority ratified what).

## Held / rejected options

- **O2 — WARN-only (drop refusal entirely): REJECTED.** Not justified: the
  landed refusal has caught real skew (the S1′ gate-5 state-lib case; S3's
  proven platform edges) and no false-positive refusal has been reproduced.
- **O3 — WARN-default + strict-profile (refusal only under an opt-in strict
  profile): SHELVED.** Revisit under the WARN-default flip conditions above;
  not carried now.
- **O4 — analyzer extraction (split eslink into a standalone analyzer
  project): DEFERRED.** No consumer demand; extraction would widen the
  maintenance surface the charter boundary exists to cap.

## Contradictions

- **None detected** between the ratified posture and the landed records: the
  O1 freeze restates the freeze record's own amendment rule (freeze
  :146-148); the accepted-UNKNOWN policy restates U6 and the `48b6c30`
  a+b+c posture; the audit corrections bring the audit's framing INTO line
  with what the records actually say (that is their purpose).
- One tension is accepted, not contradictions-class: P1's single-record rule
  vs. the existing multi-record 2026-11-07..13 history. Resolved by the
  going-forward scope: P1 binds new slices; history stays as-is.

## Findings

- **(finding)**: source=git log (read this slice), confidence=high, type=fact — the landed enforcement chain is exactly S0 `ff2f577`, S2 `9853ea8`, S3 `f9a6438`, S4 `b1ca98c`+`62b2d56`, vendor `48b6c30`, hub-token normalization `4469f3c`; the TLA amendment is `8b1bd58`; the freeze is `12d8195`.
- **(finding)**: source=vendor amendment record (`2026-11-13-eslink-o4-vendor-unknown-reporting-amendment.md`, :118-131), confidence=high, type=fact — the dogfood footprint baseline is 3 proven adopter-internal broken edges (all inside `node_modules/yaml`: `dist/cli.mjs` requires symbols sibling builds do not export) + 1,104 unknown findings (20 itemized non-vendor + 1,084 vendor-aggregated; 954 modules scanned), WARN-tier, result HEALTHY.
- **(finding)**: source=freeze record (`2026-11-08-eslink-o4-grammar-variant-freeze.md`, :75, :100, :191), confidence=high, type=fact — reason-coded UNKNOWN is the designed non-conformance escape: never `broken`, never refusal, never FAIL (U4/U5/U6).
- **(finding)**: source=stage-4 record (`2026-11-09-eslink-o4-stage4-normalization-option2.md`, Effect + slate item 1), confidence=high, type=fact — the ratified normalization is the single hub token `state-lib.js:5554` `i++` → `i += 1`, landed `4469f3c`; no other production rewrites.
- **(inference)**: source=coordinator dispatch (this slice), confidence=high, type=inference — the 2026-11-14 scope-audit debate (verdict MIXED) and the operator's "all as recommended" ratification are carried by the dispatch; this record is their authoritative carrier per the parent-session convention (same as the vendor record's operator-intent finding). The debate's own receipts are the repo records, SHAs, and footprint numbers re-derived in the Verification table below.
- **(finding)**: source=corpus-pin lines in the vendor record (:11) and wiring record (§A), confidence=high, type=fact — corpus pins remain frozen at `61/46/15 · 49 · 14/3/32 · 5 · 0`; this posture record changes no runtime code and no corpus bytes.

## Verification

| Claim | Verifying evidence (all re-derived from repo ground truth this slice) | Verified |
|---|---|---|
| Landed chain SHAs (S0/S2/S3/S4/vendor/normalization/TLA/freeze) | `git log --oneline` (read this slice): `ff2f577` fix(eslink) U+2028/U+2029; `9853ea8` refuse cohort-breaking updates; `f9a6438` doctor cohort-links; `b1ca98c` cohort honesty contract; `62b2d56` render sync for S4 docs; `48b6c30` vendor unknown aggregation (a+b+c); `4469f3c` state-lib hub-token normalization; `8b1bd58` TLA freeze amendment; `12d8195` variant freeze | yes (cited) |
| Footprint baseline: 3 proven node_modules edges + 1,104 unknowns, visible and non-suppressed | vendor record :118-131 (live doctor receipt: "3 adopter-internal proven broken link(s), 1104 unknown finding(s) … 954 module(s) scanned"; 20 itemized + 1,084 aggregated; proven `node_modules/yaml` edges itemized) | yes (cited) |
| UNKNOWN/WARN is the designed escape — "conform forever" corrected | freeze record :75 ("never `broken`, never refusal (U5/U6…)"), :100 ("reason-coded UNKNOWN … never refusal, never FAIL (U6)"), :191 (verification row pinning clauses U4/U5/U6) | yes (cited) |
| Normalization was ONE hub token, not wholesale | stage-4 record Effect + option table ("the single hub-token normalization `state-lib.js:5554` `i++` → `i += 1`"); commit `4469f3c` (corpus: normalize state-lib update operator — one file, one token) | yes (cited) |
| Grammar-extension path requires amendment + fresh operator approval | freeze record :146-148 + :11-12 ("Any spec change after this point requires an amendment to this record plus renewed operator approval"); precedent executed exactly once (`8b1bd58`) | yes (cited) |
| Honesty rider is the standing charter acknowledgment | vendor record :52-54 (aggregate line carries the literal rider "link integrity only; behavior of preserved files is not verified"); honesty contract tests per `b1ca98c`/`48b6c30` records | yes (cited) |
| Corpus pins frozen; no runtime code in this slice | `git status` on this slice's tree: only `researches/decisions/2026-11-14-…md` (this record) + the companion mission-line/render chore; no `internal/`, no `templates/` bytes changed | yes (observed) |
| Charter boundary pinned in mission source (companion chore) | `.vh-agent-harness/AGENTS.mission.md` Non-negotiable rules gains the one-line boundary; root `AGENTS.md` recomposed via `make update` (compose = `AGENTS.core.md` + `AGENTS.mission.md`, seam.go `composeAgentsMd`; doctor byte-compares the composition) | attested (companion chore — separate slice, separately reviewed) |
