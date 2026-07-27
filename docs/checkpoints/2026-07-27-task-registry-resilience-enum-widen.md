# Task-registry resilience + task_type enum widen

**Date:** 2026-07-27
**Scope:** `templates/core/.opencode/scripts/state-lib.js`,
`templates/core/docs/coordination/schemas/task-card.schema.json`,
`templates/core/.opencode/commands/write-task.md`,
`templates/core/.opencode/scripts/verify-task-registry.js`
(rendered mirrors under `.opencode/` and `docs/coordination/` regenerated
via `make update`).

## The defect

A single coordination-task card with an out-of-enum stored value on any of
the six enum fields (`status`, `coordination_mode`, `report_envelope`,
`task_type`, `source_policy`, `desired_artifact_type`) bricked the ENTIRE
registry: `listCoordinationTasks()` threw `task_type must be one of:
implementation, study, research` (or the field-specific equivalent) and
aborted, taking down every load-based op (`read`, `activate`, `ready`,
`update`, `repair`, `review`, `saveCloseout`) with it.

**Root cause — a read/write asymmetry.** The WRITE path
(`saveCoordinationTask` and friends) already used the non-throwing
`normalizeCoordinationEnumCollected` twin (bad input → `""`, message
collected, then one collected throw at the end). The READ path
(`normalizeCoordinationTaskRecord`) routed every enum field through the
THROWING `normalizeCoordinationEnum`. So bad STORED data (a prior schema
version, a manual edit, a code regression) killed the registry where bad
INPUT was merely rejected.

## Fix shape landed

### Slice A — harden the read/normalize path (defect fix)

`normalizeCoordinationTaskRecord` now routes **all nine** enum fields
reachable during normalization through the non-throwing
`normalizeCoordinationEnumCollected`, with a single throwaway
`readEnumErrors` accumulator whose messages are discarded (the save path
remains the strict authority for INPUT; the read path only tolerates what
is already on disk):

- 6 top-level: `status`, `coordination_mode`, `report_envelope`,
  `task_type`, `source_policy`, `desired_artifact_type`
- 2 nested `latest_report`: `latest_report.status`,
  `latest_report.report_envelope`
- 1 in `normalizeStoredCoordinationReview` (called from the record
  normalizer): `last_review.status` — same defect class, allocated its own
  local `reviewEnumErrors` accumulator (no signature change; the function
  has a single caller).

A bad value coerces to `""` and the existing per-field `|| <default>`
fallback applies where one exists (`status → "draft"`,
`report_envelope → defaultReportEnvelopeForMode(mode)`). The throwing
behavior of `normalizeCoordinationEnum` itself is UNCHANGED — the save
path and `verify-state-validation.js` still depend on it throwing on bad
INPUT.

**Scope extension beyond the contract.** The task card named six
top-level fields. The same defect class existed in three more nested enum
sites (two in `latest_report`, one in `normalizeStoredCoordinationReview`).
Leaving them throwing would have left a residual brick vector that
contradicts the observable contract ("list MUST NOT throw when a card has a
bad stored enum"). The extension is the same mechanical swap applied
uniformly; it is not a quarantine subsystem.

### Slice A.1 — loadCoordinationTask fault tolerance (necessary for the observable contract)

**Discovered during implementation.** Coercing a bad enum to `""` alone is
INSUFFICIENT: `loadCoordinationTask` called the THROWING
`ensureCoordinationTaskCoreFields` immediately after normalize. A bad
`task_type` coerced to `""` would then throw `task_type is required.` —
moving the brick, not fixing it. (No valid default exists for `task_type`;
`defaultCoordinationTaskPayload` sets it to `""`.)

Fix: `loadCoordinationTask` now calls the non-throwing
`collectCoordinationTaskCoreFieldErrors` (collect + discard) instead of the
throwing `ensureCoordinationTaskCoreFields`. This is the second half of the
read/write asymmetry fix: **writes reject bad INPUT, reads tolerate bad
STORED data.** The save path (`updateCoordinationTask` and friends) keeps
the strict throwing `ensureCoordinationTaskCoreFields` call.

### Slice B — widen the task_type enum (additive)

Two legitimately-useful task types that real DEFER cards wanted:
`docs` (documentation work) and `verification` (verification/validation
work). Neither carries type-specific required fields (same as
`implementation`/`study`); only `research` has the source_policy /
research_question contract, so no new `allOf` conditional was added to the
JSON schema.

- `state-lib.js` `COORDINATION_TASK_TYPES`: `+ "docs", "verification"`.
- `task-card.schema.json` `task_type.enum`: `+ "docs", "verification"`.
- `write-task.md` union type: `implementation | study | research | docs | verification`.

### Slice C — tests

`verify-task-registry.js` gains:

1. **Resilience (the crux).** Saves a good sentinel card and a sibling
   card, then corrupts the sibling ON DISK (bypassing the validating save
   path) with bad values on all six top-level enum fields. Asserts
   `listCoordinationTasks()` does NOT throw, the good sentinel is still
   returned (blast radius contained), the bad card itself is still returned
   (coerced), and the bad values do not propagate verbatim (`task_type →
   ""`, `status → "draft"`).
2. **Positive widen coverage.** `saveCoordinationTask` now accepts
   `task_type: "docs"` and `task_type: "verification"` and they round-trip.

`verify-state-validation.js` is UNCHANGED — its `"bogus-type"` throw
assertion stays green because `"bogus-type"` is still invalid after
widening.

## Verification

| Claim | Verifying command/output | Verified |
|-------|--------------------------|----------|
| `listCoordinationTasks()` does not throw on a bad stored enum (crux) | `node .opencode/scripts/verify-task-registry.js` → `verification: ok` (Resilience block: bad-enum card on disk, list returns sentinel + coerced bad card) | yes |
| Bad enum coerces (`task_type → ""`, `status → "draft"`) | same run — assertions `coercedBadEnum.task_type === ""` and `coercedBadEnum.status === "draft"` | yes |
| `docs`/`verification` save + round-trip | same run — `docsTask.task.task_type === "docs"`, `verificationTask.task.task_type === "verification"` | yes |
| Save-path throw-on-bad-INPUT intact | `node .opencode/scripts/verify-state-validation.js` → `verification: ok`, `aggregated_errors_confirmed: 3`, `single_enum_regression_confirmed: 3` (bogus-type still throws) | yes |
| `normalizeCoordinationEnum` still throws (input validator untouched) | all remaining throwing call sites (6: L4070 input-statuses filter, L4579/L4591/L4635 update payload, L4741/L4753 task-repair payload) are on the input/write path; the read-normalize path has zero | yes |
| Go gates green | `go test ./...` → all `ok`; `gofmt -l .` → none unformatted; `go vet ./...` → ok | yes |
| `.opencode/` regenerated from edited corpus (no stale-embed revert) | `make update` → `5 managed-overwrite`; `doctor dev-stale-embed PASS (embedded corpus matches checkout's templates/core)` | yes |
| Rendered mirrors carry the changes | `.opencode/scripts/state-lib.js` L105-111 enum widened; `.opencode/commands/write-task.md` L20 union widened; `docs/coordination/schemas/task-card.schema.json` L37 enum widened; `.opencode/scripts/verify-task-registry.js` carries Resilience block | yes |

## Findings

- **Second brick vector (load ensure)**: source=implementation analysis,
  confidence=high, type=fact. `loadCoordinationTask` (L3490 at handoff)
  called the throwing `ensureCoordinationTaskCoreFields` after normalize.
  Coercing bad→`""` in normalize alone moves the brick to a "required"
  throw. Fixed by switching to `collectCoordinationTaskCoreFieldErrors`.
  This was not named in the task card's Slice A description but is required
  by its observable closeout (#3: "proving a bad stored card does not brick
  list").
- **Nested enum brick vectors**: source=code reading, confidence=high,
  type=fact. Two in `latest_report` (status, report_envelope) and one in
  `normalizeStoredCoordinationReview` (`last_review.status`). Same defect
  class; hardened uniformly.
- **Concurrent-session dirt**: source=git status, confidence=high,
  type=fact. The working tree carries unrelated modified files
  (commit-reviewer*, committer.md, commit-review.md, review-tiers.json,
  commit-gate.sh, README.agent.md, two `internal/cli/` test files) from a
  concurrent session. `doctor managed-drift FAIL (7 drifted)` is caused by
  that session's in-progress work, NOT this slice. This slice stages only
  its own 8 files (4 source + 4 rendered mirrors); the committer's
  private-index gate excludes the rest.

## Contradictions

- **Task card Slice A scope vs. observable contract.** The card named six
  top-level enum fields and described the fix as "coerce to `""` so the
  default kicks in." That description is incomplete in two ways: (a) it
  omits the three nested enum sites, and (b) coercing to `""` alone re-bricks
  at `loadCoordinationTask`'s ensure call (no valid `task_type` default
  exists). Resolved by following the card's OBSERVABLE closeout (#3 and the
  behavioral contract "list MUST NOT throw") as the authority and extending
  the fix to cover all nine enum sites + the load-ensure call. The card's
  non-goal "do not touch the save path's throwing-on-bad-input" is
  respected — only the READ path's ensure call changed.

```behavioral-closure
verdict: proven
result: proven
crux: load-bearing path is listCoordinationTaskCards() (exercised via the
  public listCoordinationTasks()) reading a card whose stored enum fields
  were corrupted on disk (task_type="bogus-type", status="bogus-status",
  coordination_mode="bogus-mode", report_envelope="bogus-envelope",
  source_policy="bogus-policy", desired_artifact_type="bogus-artifact").
  Verifier: verify-task-registry.js "Resilience bad-enum card" block.
  Command: `node .opencode/scripts/verify-task-registry.js` → printed
  `verification: ok`. The OUTCOME was observed (list returned the good
  sentinel card AND the coerced bad card without throwing; bad values did
  not propagate — task_type coerced to "", status to "draft"), not merely
  the mechanism asserted.
```
