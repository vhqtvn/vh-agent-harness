# Decision: Recurrence signature + dedup enforcement (resolves S3 / defer-002)

**Date:** 2026-07-29
**Status:** Accepted (record-of-decision; no code this slice). Resolves the S3 Open
Question #1 (`defer-002-symptom-signature-stability`); adopts the recurrence-signature
identity model and the dedup-enforcement placement. Implementation is staged (see
backlog intake). Engages, does not supersede, the 2026-07-23 disposition §6 and the
2026-07-24b addendum.
**Supersedes:** none. Narrows the 2026-07-22 claim-verifier-closure-kernel memo
§4.3/S3 Open Question #1.
**See also:**
[`./2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md`](./2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md) §4.1/§4.3/S3 (origin).
[`./2026-07-23-vh-solara-orchestration-field-report-disposition.md`](./2026-07-23-vh-solara-orchestration-field-report-disposition.md) §6 (defer-002 + recurrence-detector filed).
The 2026-07-24b addendum on the 2026-07-23 disposition (2nd Pattern-6 instance; "two instances ≠ stable signature" caution).
[`./2026-07-28-file-scope-lease-trigger-evaluation.md`](./2026-07-28-file-scope-lease-trigger-evaluation.md) (sibling coordination-lane defer; trigger NOT met).

## Framing

`defer-002` asked the S3 question: *how is the recurrence signature computed so N
reports of the same underlying defect collapse to ONE recurring defer entry rather
than N new cards?* Its trigger effectively fired (a 2nd Pattern-6 instance was
observed — 2026-07-24b addendum), making it eligible to resolve; resolving it
unblocks the coordination-lane `defer-within-session-recurrence-detector`.

A `/solution-brief` compare-and-plan pass (researcher → debate → planner) converged
on the design this memo adopts. The 2026-07-24b caution ("two instances ≠ a stable
signature") is engaged head-on below: it is a statement about resolution difficulty,
not a blocker, and the design answers it by making stability come from explicit
identity rather than from hashing.

## Decision

**Explicit two-level identity, not a computed hash.** The collapse key is an
authored identity, not a digest of (possibly unstable semantic) inputs.

- **`recurrence_id`** — stable, explicit identity of ONE underlying defect. This is
  the actual collapse key. Authored when a defect is first recognized as recurring.
- **`symptom_class_id`** — immutable, versioned taxonomy identifier (e.g.
  `recurrence.v1/band-aid-loop`). Aggregates the *class* of symptom.
- **evidence** (paths, claim_ids, affected capabilities, outcomes, commit
  subjects/ranges, later root-cause findings) — non-identity observations attached
  to the entry.
- **alias / supersession** — bounded reconciliation for an identity that must be
  re-pointed after later evidence.
- Hashing may be used as an internal representation or integrity check, but the
  opaque digest MUST NOT be the only authored or operator-visible contract.

**Crux distinction (load-bearing):** a shared `symptom_class_id` does **not** merge
exact defects. It lets the state/reconcile and UX/visual Pattern-6 loops collapse
into ONE symptom-class aggregation, while distinct `recurrence_id` values keep
independently-actionable defects separate. Reports collapse to one defer entry
**only** when they carry the same exact-defect `recurrence_id`.

## Placement (split, authority-line-respecting)

- **Pure derivation** below the CLI: `internal/memory/claims/` (or a sibling pure
  package if recurrence semantics outgrow claims) computes effective identity and
  canonical grouping. No persisted-memory authority.
- **Producer / synchronous merge:** at the task-writing boundary, a recognized
  repeat updates the canonical entry rather than writing a new card.
- **Doctor:** synchronous diagnostics — report canonical recurrence groups,
  malformed identity, conflicting aliases, uncollapsed duplicates.
- **Release enforcement (the acting seam):** the release gate / doctor compares
  canonical recurrence state against the committed disposition and **fails closed**
  when a recurrence is unadjudicated. Backed by the committed manifest
  (`.vh-agent-harness/release-defer-dispositions.json`) — the fresh-checkout
  authority.
- **Predicate checker** (`check-defer-triggers.js`) stays **advisory-only**. The
  §4.3 errata evidence (errata-v0120 stayed draft across 3 releases despite its
  trigger firing) proves it cannot be the enforcement seam.
- **`internal/memory/store`** (persisted typed memory) is **not** release authority:
  missing storage reads empty and malformed records are skipped — fail-open. The
  committed manifest remains the cross-checkout authority.

## Repeat semantics

When a 2nd instance of a known `recurrence_id` appears:

1. Resolve effective identity = explicit `recurrence_id`, else legacy `task_id`.
2. Locate the canonical defer.
3. **Append** a structured recurrence observation (do NOT spawn a child card).
4. Increment `recurrence_count`.
5. Mark the release disposition as requiring renewed acknowledgement/adjudication
   (see next section).
6. Surface canonical entry, count, latest observation, and evidence history to the
   operator.

Spawn-and-link is reserved for later evidence establishing a genuinely DIFFERENT
defect. A bare counter without evidence history is insufficient.

## Manifest-v2 disposition interaction (resolved sub-decision)

A new observation MUST NOT silently pass under a stale acknowledgement. The rule:

- Each recurrence entry carries `recurrence_count` (total observations) and
  `last_acknowledged_count` (count at last disposition adjudication).
- A new observation increments `recurrence_count`. If
  `recurrence_count > last_acknowledged_count`, the entry is **unadjudicated** → a
  gate-consistency failure → release is **blocked** until the operator re-adjudicates
  (re-applies the disposition at the new count).
- After re-adjudication, the explicit disposition controls exactly as today:
  `block` continues to block; `disclose` permits with the required disclosure;
  `override_required` requires the existing override ceremony.
- This is **fail-closed** (new evidence cannot slip through under a stale ack) and
  **authority-line-respecting** (the gate ACTS; the disposition is operator-attested
  in the committed manifest).

Recurrence does not itself select a disposition — those remain explicit operator
decisions. Recurrence only forces RE-adjudication.

## Backward compatibility

- `effective_recurrence_id = recurrence_id` when present; otherwise `task_id`.
- Do NOT auto-hash or auto-merge existing cards.
- Preserve `task_id` as the card/report identifier even after recurrence identity
  is added.
- Manifest schema v1 accepted during a bounded migration; a legacy entry remains
  1:1 by `defer_id`. Multi-observation recurrence requires explicit promotion to the
  v2 recurrence form before it can be acknowledged.

## Stable-signature reconciliation (engaging the 2026-07-24b caution)

Stability does **not** emerge because a tuple was hashed. A hash only freezes its
inputs; unstable semantic inputs still produce unstable identities. Stability comes
from: an explicit exact-defect ID, immutable/versioned `symptom_class_id`s, and
controlled alias/supersession rules. The taxonomy therefore evolves as instances
accrue — two examples justify `recurrence.v1/band-aid-loop` as a class, not a
universal fingerprint. The caution is satisfied by construction, not contradicted.

## What this does NOT solve (honest limits)

- It does **not** detect band-aid loops or infer that two reports concern the same
  defect. It only provides the identity, aggregation, and enforcement contract
  AFTER a producer or the downstream recurrence detector supplies that judgment.
- It does **not** establish a complete symptom-class taxonomy from two instances.
- It cannot guarantee correct human assignment of `recurrence_id`; aliasing,
  conflict diagnostics, and explicit reconciliation are still required.
- It does **not** prove the two observed Pattern-6 clusters are one exact defect —
  they are evidenced as one symptom CLASS, not necessarily one canonical defer.
- It does not reopen W4 (worktrees) or G1 (line-level merge).

## Staged build plan (backlog intake; no code this slice)

1. **Contract slice** — add optional recurrence identity, symptom class, evidence,
   observation/count, and alias/supersession shapes to the domain-free task-card
   and manifest contracts (`templates/core/...task-card.schema.json`, manifest
   schema source).
2. **Derivation slice** — effective identity (explicit `recurrence_id` else
   `task_id`) + canonical grouping, no persisted-memory authority
   (`internal/memory/claims/` or sibling pure pkg).
3. **Producer/dedup slice** — on a recognized repeat, update the canonical entry
   synchronously rather than writing a new card.
4. **Doctor slice** — report canonical recurrence groups, malformed identity,
   conflicting aliases, uncollapsed duplicates.
5. **Release slice** — compare canonical recurrence state vs committed
   disposition/acknowledgement; fail closed when unadjudicated
   (`internal/cli/release_gate.go`, `internal/cli/doctor*.go`).
6. **Migration slice** — schema-v1 + legacy-card behavior during bounded transition;
   explicit migration, no retroactive hashing.

Likely implementation surfaces: `internal/memory/claims/claim.go`(+_test),
`internal/cli/release_gate.go`, `internal/cli/doctor.go`(+helpers/tests),
`internal/cli/check_defer_release_manifest_test.go`,
`templates/core/docs/coordination/schemas/task-card.schema.json`, the task-card
writer/state library + command docs under `templates/core/.opencode/`, and
`.vh-agent-harness/release-defer-dispositions.json`. Rendered `.opencode/` and
`docs/coordination/` copies are generated; source changes go under `templates/core/`
then `make update`.

### Dedup crux test (the load-bearing proof for the build)

- Supply N reports with different `task_id` values and heterogeneous
  path/claim/outcome evidence but the SAME `recurrence_id`. Exercise the real
  producer→derivation→gate path. Observe exactly ONE canonical defer, N retained
  observations, `recurrence_count == N`, and no N extra canonical cards.
- Supply two reports with the same `symptom_class_id` but DIFFERENT
  `recurrence_id` values → observe TWO canonical defers.
- Add a release test proving a newly-increased count is NOT silently accepted under
  a stale `last_acknowledged_count`.

## Authority-line engagement

- The task writer provides synchronous merge convenience; the claims/recurrence
  package derives identity — neither holds transition authority.
- Doctor and the release gate detect and reject an uncollapsed or unadjudicated
  recurrence. The standalone predicate checker remains advisory.
- Consistent with the 2026-07-22 authority line and the 2026-07-23 disposition §5.

## Evidence / Provenance

| Claim | Verifying artifact / command | Verified |
|-------|------------------------------|----------|
| slice (a) keys recurrence by task_id only (naive) | `internal/cli/release_gate.go` (commit `69e0104`); claims projection | yes |
| predicate checker is advisory / known-broken (errata-v0120) | `.opencode/scripts/check-defer-triggers.js`; case study §4.3 | yes |
| internal/memory/store unsuitable as release authority (fail-open) | `internal/memory/claims/` (read-only derivation; avoids persisted state for release safety) | yes |
| committed manifest = fresh-checkout authority | `.vh-agent-harness/release-defer-dispositions.json` (schema v1, keyed by defer_id) | yes |
| 2nd Pattern-6 instance observed | 2026-07-24b addendum (UX/visual band-aid loop) | yes |
| task-card schema has no recurrence fields | `templates/core/docs/coordination/schemas/task-card.schema.json` (`additionalProperties:false`) | yes |
| design options + tradeoff matrix | `/solution-brief` pass (researcher→debate→planner), 2026-07-29 | yes (read-only reasoning) |
