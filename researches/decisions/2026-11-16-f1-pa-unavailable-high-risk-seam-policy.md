# F1 P-a `unavailable` at High-Risk Seams — Refined NO-for-now

- **Date:** 2026-11-16
- **Decision-owner:** operator
- **Question:** Should high-risk release seams ALSO block on F1 P-a result=`unavailable`? (F1 design memo open-question #5, `researches/decisions/2026-07-25-f1-synthesis-family-and-s2a-topology.md` L317-321; backlog row P2-RELEASE-001.)
- **Decision:** NO — refined NO-for-now. Not a permanent no; three reconsideration triggers in §5.

## 1. Decision

- **Refined NO-for-now [Fact]:** NO categorical release refusal for unavailable-only P-a coverage at this time. Structural semantics are unchanged: `unavailable` keeps requiring an explicit limitation; the closed result enum and exact-enum rendering are unchanged.
- **NOT gap-repaired [Fact]:** this decision records an ACCEPTED, UNREPAIRED assurance gap. "unavailable cannot support proven" remains comment-only; there is NO mechanical P-a→proven-claim linkage. The decision is "decided NO-for-now" — never read as "gap repaired" or "enforced".
- **Resolution path:** the comments-only branch P2-RELEASE-001 itself prescribed for NO — deferral comments updated, no code change.

## 2. Evidence Base

Six P-a seams inventoried [Fact] (line pins verified against this tree, 2026-11-16):

| Seam | Site | Reachability |
| --- | --- | --- |
| S1 EmitF1 | `f1_emit.go:67` | library (zero non-test callers) |
| S2 ValidateR3ForkForTransition | `f1_r3_gate.go:68` (probe-COUNT-only check at :144-146) | library |
| S3 doctor #16 (F1 envelope consistency) | `doctor_f1.go:46` | LIVE via doctor→G0c |
| S4 doctor #18 (F2 pair consistency) | `doctor_f2.go:338-345` | LIVE via doctor→G0c |
| S5 IngestF1EmitForF2 | `f2_ingest.go:145` | library |
| S6 PersistF2Pair | `f2_projection.go:849` | library |

- Only S3/S4 are live, via doctor→G0c (tier application `doctor.go:589-596`; hard gate `scripts/release-tag.sh:574-583`), and BOTH PASS unavailable-only projections today [Fact].
- `paCoverageSatisfies` (`f1_validator.go:540-546`) treats `unavailable` as coverage-satisfying [Fact]. Limitation enforcement: `f1_pa.go:135-138`, `f1_validator.go:511-516`, `f1_pa_gate.go:58` [Fact]. Exact-enum rendering: `f2_projection.go:456-516` [Fact]. Proven-rule comment-only sites: `f1_envelope.go:399`, `f1_validator.go:146,513,531` (pins from the pre-edit evidence pass; :146 corroborated during this slice's pin verification) [Fact].
- Namespace hazard [Fact]: `release_gate.go` "F1-D1"/"tier1b-F1" tokens belong to a case-study taxonomy, NOT this F1 synthesis family.

## 3. Why Not B / Not C

- **B — doctor-wired policy refusal: not design-ready [Inference].** Release materiality, treatment of historical artifacts already carrying unavailable-only projections, and false-block/relabeling risk are all undefined; wiring a refusal before those are settled would risk blocking releases on projections that are structurally honest today.
- **C — enforce the proven-rule linkage only: no enforcement target [Inference].** No consumer of a proven-claim linkage was identified; an enforced rule with no consumer is dead surface.

## 4. Acknowledged Assurance Gap

"unavailable cannot support proven" is stated in comments only. Nothing mechanical prevents a future proven claim from resting solely on `unavailable`-only coverage. This gap is ACCEPTED and UNREPAIRED by this decision; the honesty burden stays on closeout/verifier authorship until a §5 trigger fires.

## 5. Reconsideration Triggers

1. Release-material reliance on unavailable-only coverage (a release actually leaning on it).
2. An observed misleading proven claim resting solely on `unavailable`.
3. BEFORE any emit/R3 production wiring lands (memo open-question #6) — wiring would convert library seams (S1/S2/S5/S6) into live ones and change this evidence base.

## 6. What Changed

- Deferral comments rewritten as DECIDED (NO-for-now) at `internal/cli/f1_pa.go`, `internal/cli/f1_pa_gate.go`, `internal/cli/f1_validator.go` — comments only, zero code deltas.
- This record; backlog row P2-RELEASE-001 closed `done`.
- Nothing mechanical changed: no gate, doctor check, test, or enum semantics touched.
