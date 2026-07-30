**Date:** 2026-07-30
**Status:** Accepted (record-of-decision) — recording what shipped in `09fe9eb`; design SETTLED, implementation committed. Not a re-derivation.
**Supersedes:** None. Extends, but does not reopen, the F4 assurance/stewardship family defined by `2026-07-28-success-report-integrity-and-working-tree-stewardship.md` and the §4.3 defer-liveness gate first shipped in `69e0104`.
**See also:**
- `researches/decisions/2026-07-28-success-report-integrity-and-working-tree-stewardship.md` (F4 family — extended here, not reopened)
- `researches/decisions/2026-07-23-release-defer-dual-mechanism-reconciliation.md`
- `docs/case-study/2026-07-22-unverified-claims-at-lossy-boundaries.md` §4.3
- `internal/cli/release_gate.go` (code comments: recurrence predicate + dormant-vs-imminent activation + federated authority)
- `git show 09fe9eb` (commit body: silencer-immune `LivenessCard` pool, exact-match v1 resolution, fog/COLD advisories, disposition surfaces)
- `git show 69e0104` (original doctor #12 released-claim gate)

# F4-C defer-liveness release gate — crosswalk of the shipped 69e0104 → 09fe9eb composition

## Framing

This is a cross-artifact decision record, not a full adjudication. The mechanics and rationale already live in the `09fe9eb` commit body and the `internal/cli/release_gate.go` code comments; this memo assembles the sequential relationship and the topology placement into one durable home. It records what was decided and shipped — **no new policy, and it does not reopen** the 2026-07-28 F4 decision, the 2026-07-25 topology, or the case study.

F1-D1 is the population-scale instance of case-study §4.3 (deferred-debt decay): the erratum-v0120 lesson generalized. Concretely, 16 `draft` DEFER cards had fired `path_touched` targets in `v0.18.0..HEAD` (the self-derived PRIOR_TAG) and would otherwise ship without a forced verdict. The §4.3 lesson is direct and load-bearing here: **the gate must read state directly, not trust trigger promotion** — the trigger machinery itself fails mechanically (the erratum card's trigger fired on three consecutive releases and stayed `draft` through all of them, so it is not only severity verdicts that decay).

## Decision (settled)

Extend doctor check #12 — the §4.3 generic release-readiness defer-liveness gate first shipped in `69e0104` as the released-claim class — with a SECOND contradiction class, **release-diff-recurrence**. Place the combined control in **F4** (defer-liveness / debt stewardship), MERGE'd into a single check that emits two contradiction classes. Architecture **(iii) BOTH**:

- **doctor-live** = load-bearing tag refusal. Releases are locally tagged; CI is post-tag publication protection and cannot un-create a tag, so the refusal must land at tag time (the G0c gate).
- **manifest** = committed CI-verifiable disposition record, now mechanically populated via the `--mode=release-prep` enumerator added to `check-defer-triggers.js` in the same slice.

The new class reads `path_touched` targets from a liveness card's body regardless of filename prefix / `.json`-vs-`.md` format / trigger grammar, resolves each against `PRIOR_TAG..HEAD`, and FAILs (via G0c) when an OPEN card has a fired target AND lacks a fresh disposition. Federated authority: gate + G0c refuse; releaser / operator dispose; the manifest M is the releaser's (the committed disposition record rechecked by the release-mode evaluator). No new role.

## Crosswalk (the sequential composition)

One check (#12, `checkDeferLiveness`), two contradiction classes — MERGE'd because they share owner, cadence, and authority:

| Class | Shipped | Predicate | Contradiction |
|---|---|---|---|
| released-claim | `69e0104` (original) | `findDeferLivenessContradictions` | OPEN card referencing a present released/about-to-release migration note |
| release-diff-recurrence | `09fe9eb` (F4-C) | `evaluateDeferRecurrence` | OPEN card whose `path_touched` target exact-matches a path in `PRIOR_TAG..HEAD`, lacking a fresh disposition |

The two classes are independent: a card can trigger either, both, or neither. `69e0104` established the gate's read-state-directly discipline and its SKIP / FAIL / PASS tiering (incl. fail-closed on malformed cards); `09fe9eb` extends that same discipline to the recurrence surface without re-deriving the released-claim logic.

## Mechanism pointers (cross-reference, not restatement)

For the mechanics — the silencer-immune `LivenessCard` pool, exact-match v1 resolution, fog/COLD advisory treatment, and the three disposition surfaces — **point to `git show 09fe9eb` (commit body)** and **`internal/cli/release_gate.go` (code comments on `checkDeferLiveness` activation and `evaluateDeferRecurrence`)**. They are not restated here; that would be redundant duplication of a settled, committed design.

The loader the predicate consults is `loadLivenessCards` in `internal/memory/claims/claim.go` — it reads EVERY file under `.local/coordinator/tasks/` with no prefix and no extension filter (the silencer-immunity contract: a card cannot escape the gate by being named without the `defer-` prefix or by being `.md` instead of `.json`).

## Why dormant by design

The recurrence refusal's BLOCK escalates to FAIL only when `releaseImminent` (an untagged, about-to-release migration note exists — `unrelCnt > 0`). This mirrors check #13's proven pattern. Ordinary `doctor` runs are advisory: the predicate still runs and reports the recurrence surface (fog/COLD advisories) but does not escalate. At tag time (G0c) the release ceremony has created the about-to-release note, so the predicate is active and refuses on any undisposed fired card. The gate therefore protects the **release boundary**, not every doctor invocation.

Open v1 boundaries are honest, not hidden: fog cards (no target) bypass resolution; glob/dir triggers are COLD (advisory only until v2 exact-match is widened). Both are captured by spawned DEFER cards (e.g. `defer-glob-trigger-v2`).

## Authority-line summary

| Mechanism | Classification | May apply a transition? |
|---|---|---|
| doctor #12 via G0c (both contradiction classes) | **GATE-SHAPED CONVERSION** (reads primary state directly — same basis as G0 / G0b) | Yes — refuses the tag |
| manifest evaluator (`check-defer-triggers.js --mode=release-prep`) | **GATE-SHAPED CONVERSION** over the committed record | Yes — independent second surface |
| `check-defer-triggers.js` promoter mode | **INFORM** | No (silencer blind spots; §4.3 demotes it to a curation aid) |

This is consistent with the F4 authority rule from the 2026-07-28 decision: the family label creates no new transition authority; each mechanism receives authority only through its actual enforcement host.

## Evidence / Provenance

| Evidence | Source | Verifying command |
|---|---|---|
| F4-C extension shipped (recurrence class + release-prep enumerator) | `09fe9eb` | `git show --stat 09fe9eb` |
| Original §4.3 released-claim gate (#12) | `69e0104` | `git show --stat 69e0104` |
| The §4.3 deferred-debt-decay failure this seals | `docs/case-study/2026-07-22-unverified-claims-at-lossy-boundaries.md` §4.3 | `git show HEAD:docs/case-study/2026-07-22-unverified-claims-at-lossy-boundaries.md` |
| F4 family definition this extends (not reopens) | `researches/decisions/2026-07-28-success-report-integrity-and-working-tree-stewardship.md` | `git show HEAD:researches/decisions/2026-07-28-success-report-integrity-and-working-tree-stewardship.md` |
| Mechanism (code comments: activation + federated authority) | `internal/cli/release_gate.go` | `git show HEAD:internal/cli/release_gate.go` |
| Silencer-immune `LivenessCard` pool | `internal/memory/claims/claim.go` (`loadLivenessCards`) | `git show HEAD:internal/memory/claims/claim.go` |

The behavioral-closure crux was PROVEN in the implementation slice — `go test ./internal/cli/ -run TestDeferRecurrence` (9 tests: refuse→clear across 3 disposition surfaces [closed status / manifest entry / operator override] + silencer immunity [`.md` card blocks] + fog/COLD/dormant advisories + live-repo clean). This memo records that proven control; it does not re-prove it.
