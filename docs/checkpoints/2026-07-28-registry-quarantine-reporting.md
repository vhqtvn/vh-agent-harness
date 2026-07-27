# Coordination-task registry: report-and-continue quarantine

**Date:** 2026-07-28
**Scope:** `templates/core/.opencode/scripts/state-lib.js`,
`templates/core/.opencode/scripts/verify-task-registry.js`,
`templates/core/.opencode/commands/task-list.md`
(rendered mirrors under `.opencode/` regenerated via `make update`).

This is criterion 3 of the operator-filed design defer
`defer-coordination-registry-quarantine-malformed`. It builds directly on
the 2026-07-27 resilience work (`186bff1`, see
`docs/checkpoints/2026-07-27-task-registry-resilience-enum-widen.md`),
which hardened the read path against bad stored enums / missing core
fields but **discarded** the collected errors.

## The hazard that `186bff1` left open

`186bff1` switched the read normalizer to the non-throwing
`normalizeCoordinationEnumCollected` and `loadCoordinationTask` to the
non-throwing `collectCoordinationTaskCoreFieldErrors` (collect + discard).
That stopped the registry from bricking, but it introduced a silent-coerce
gap that created a **live promotion hazard**:

1. `normalizeCoordinationTaskRecord()` coerces an invalid stored `status`
   → `""` → fallback `"draft"`.
2. `coordinationTaskRecommendation()` recommends `/task-ready` for every
   `draft` card (except incomplete research → `/task-repair`).
3. `readyCoordinationTask()` accepts `draft` and writes `ready`.
4. `updateCoordinationTask()` validates the **normalized** record — the
   original invalid value is already gone before validation runs.

So the save-path strictness (which `186bff1` preserved and which
`verify-state-validation.js` depends on) does NOT close this hazard: it is
strict about post-normalization state, and the invalid value is coerced
away before any validation sees it. A degraded card sitting on disk could
be promoted to `ready` purely through read-path coercion, with no operator
ever being told the card is malformed.

## Fix shape landed (read-path only)

### Slice 1 — preserve validation findings instead of discarding them

The read path used to route the nine enum sites through
`normalizeCoordinationEnumCollected` into a throwaway accumulator, and
`loadCoordinationTask` called `collectCoordinationTaskCoreFieldErrors`
and discarded the result. Both findings are now **retained** and associated
with their source card:

- `normalizeCoordinationTaskRecord(payload, taskID='', accumulators={})`
  accepts an optional third param `{enumErrors, enumInvalidFields}`. The
  write path keeps calling it with no accumulators (fresh locals —
  byte-identical behavior). The read path threads accumulators in.
- `normalizeStoredCoordinationReview(sourceLastReview, reviewPaths, accumulators={})`
  mirrors the same optional third param for the nested `last_review.status`
  enum site.
- `collectCoordinationTaskCoreFieldErrors(task, options={})` accepts an
  optional `options.offendingFields` Set, populated at every error site
  via a `markOffending(...fieldNames)` helper.
- New `normalizeCoordinationTaskRecordWithDiagnostics(payload, taskID='')`
  returns a discriminated internal result
  `{task, diagnostics:{problems, offendingFields, degraded}}`. It uses
  `allowLegacyIncompleteResearch: true` (read-path tolerant). The
  persistent task DTO is NOT mutated; existing fallback values are
  preserved so read resilience is unchanged.

Deterministic ordering helpers for stable quarantine entries:
`COORDINATION_QUARANTINE_FIELD_ORDER` (canonical field-name order) and
`stableSortQuarantineFields(fieldNames)` (dedup + stable sort).

### Slice 2 — scan boundary + public additive response

- New internal `scanCoordinationTaskCards()` returns
  `[{taskID, path:repo-relative, task, diagnostics, degraded}]` sorted
  desc by `updated_at`. This is the single boundary that retains
  diagnostics.
- `listCoordinationTaskCards()` is refactored into a **trusted projection**:
  `scanCoordinationTaskCards().filter(!degraded).map(.task)`. It had
  exactly two consumers (`detectCoordinationTaskOverlaps` and
  `listCoordinationTasks`), both of which want healthy-only semantics, so
  refactoring it directly (rather than adding a separate helper) keeps the
  public API surface minimal. **Build-side decision: direct change, not a
  second helper.**
- `listCoordinationTasks()` is rewired to `scanCoordinationTaskCards()`.
  Degraded cards STAY in `tasks[]` for consumer compatibility (no data
  disappearance), each carrying a `degraded: true` flag. Additive fields
  are added WITHOUT redefining existing counts:
  - `quarantine[]` — each entry: `card_id` (from parsed card if
    trustworthy, else filename), `path` (repo-relative), `error_type:
    "semantic"`, `offending_fields` (deduplicated + stable-sorted),
    `problems[]` (deterministic messages). No raw card contents or
    content preview is included (local cards can carry sensitive operator
    notes).
  - `degraded_count` (= `quarantine.length`)
  - `healthy_total`, `healthy_status_counts` (non-degraded cards only)
  - Existing `total`, `status_counts`, `tasks` are UNCHANGED (compat).

### Slice 3 — consumers

- `plan-state.js`: no code change required. `render()` =
  `JSON.stringify(result, null, 2)` auto-forwards every additive field.
  Degraded cards carry `degraded: true` and `next_recommended_command:
  "/task-open <id>"` from the recommendation wiring (Slice 4), so they
  are not presented as normal actionable tasks.
- `task-list.md`: a prominent "Degraded coordination-task cards" rendering
  section, rendered BEFORE the healthy inbox. It lists card_id, path, and
  offending_fields; states the cards are REFUSED at the action boundary;
  gives the repair/inspection next action. No lifecycle-transition
  recommendation is emitted for a degraded card.

### Slice 4 — trusted-projection exclusion + ACTION-BOUNDARY REFUSAL (the safety core)

This is the load-bearing safety slice. Without it the quarantine field is
decoration and the hazard stays open.

- `coordinationTaskRecommendation(task, actorSessionName=null, options={})`
  accepts a 3rd param. When `options.degraded` is true it returns
  `{command: "/task-open <id>", note: "...degraded...Inspect or repair..."}`
  BEFORE any status-based recommendation. This is the **soft guidance**
  layer — the coordinator is kept from proposing `/task-ready`.
- `recommendedCoordinationTaskFields(...)` threads `degraded` through.
- `readCoordinationTask(...)` returns additive `degraded` (bool) and a
  public-facing `diagnostics` object (`{degraded, offending_fields,
  problems}`) consistent with the quarantine entry field names.
- **`readyCoordinationTask(...)` REFUSES a degraded card.** A pre-lock
  refusal fires BEFORE the status guard: if `loaded.diagnostics.degraded`
  is true, it throws a structured `StateError("Task X is degraded
  (offending fields: ...) and cannot be prepared for execution...")`.
  This fires regardless of the coerced status — a degraded card whose
  stored status was coerced to `"draft"` is refused before it can reach
  the status guard. The soft guidance layer keeps the coordinator from
  proposing; this hard gate keeps every direct API caller honest.
  Suppressing guidance alone would leave the action path open.
- `detectCoordinationTaskOverlaps(...)` auto-excludes degraded cards via
  the refactored `listCoordinationTaskCards()` trusted projection. A
  defaulted/empty status or core field can no longer produce false
  overlap findings.

### Slice 5 — regression tests

`verify-task-registry.js` gains a quarantine regression block covering:

1. Invalid stored `status` + otherwise-valid card → appears in
   `quarantine[]` with correct `card_id`, `path`, `offending_fields`,
   deterministic `problems`; also still in `tasks[]` with `degraded: true`
   (compat).
2. Missing required core field → reported in `quarantine[]` with the
   offending field.
3. Multiple bad fields on one card → exactly ONE combined card-level
   quarantine entry (not duplicates), carrying both offending fields.
4. Valid cards still appear in `tasks[]` without the `degraded` flag and
   are NOT in `quarantine[]`; `degraded_count === quarantine.length`;
   `healthy_total >= 1`; `healthy_status_counts` is an object.
5. Reading / operating on a valid card still succeeds when a sibling is
   degraded.
6. A degraded invalid-status card gets NO `/task-ready` recommendation,
   and does NOT participate in overlap detection (valid sibling sharing
   the same file scope does not list the degraded card as an overlap).
7. **CRUX:** `readyCoordinationTask()` REFUSES a degraded card — throws
   `StateError` matching `/degraded/i`.
8. Strict saves still throw for invalid INPUT — verified separately by
   `verify-state-validation.js` (`task_type: "bogus-type"` still throws).
9. Empty/all-healthy registry → `quarantine: []`, `degraded_count: 0`,
   normal task output unchanged.

## Verification

| Claim | Verifying command/output | Verified |
|-------|--------------------------|----------|
| Read path preserves enum/core findings instead of discarding | `node .opencode/scripts/verify-task-registry.js` → `verification: ok` (quarantine block exercises preserved findings) | yes |
| `listCoordinationTasks()` exposes additive `quarantine[]`, `degraded_count`, `healthy_total`, `healthy_status_counts` | same run — `quarantine_degraded_count: 6`, `quarantine_healthy_total: 109`; existing `total`/`status_counts`/`tasks` unchanged | yes |
| Degraded card appears in `quarantine[]` AND stays in `tasks[]` with `degraded: true` (compat) | same run — assertions on degraded sentinel + quarantine entry | yes |
| Multiple bad fields → ONE combined quarantine entry (not duplicates) | same run — combined-bad card assertion | yes |
| Degraded card gets NO `/task-ready` recommendation | same run — degraded `next_recommended_command !== "/task-ready"` | yes |
| Degraded card does NOT participate in overlap detection | same run — valid sibling overlap list excludes degraded sibling sharing the same file scope | yes |
| CRUX: `readyCoordinationTask()` REFUSES a degraded card (action boundary closed) | same run — `readyCoordinationTask` throws `StateError` matching `/degraded/i` | yes |
| Save-path throwing intact (`task_type:"bogus-type"` still throws) | `node .opencode/scripts/verify-state-validation.js` → `verification: ok` | yes |
| Go gates green | `go test ./...` → all `ok`; `gofmt -l .` → none unformatted; `go vet ./...` → ok | yes |
| `.opencode/` regenerated from edited corpus (no stale-embed revert) | `make update` → regenerated mirrors; `vh-agent-harness doctor` → HEALTHY, `managed-drift PASS`, `dev-stale-embed PASS`, `behavioral-closure PASS` | yes |

## Findings

- **Two-layer safety (guidance + gate).** source=design debate,
  confidence=high, type=decision. Suppressing the `/task-ready`
  recommendation alone is insufficient: a direct API caller
  (`plan-state ready_coordination_task`) would bypass coordinator prose
  and still promote. The fix lands BOTH layers — the soft guidance keeps
  the coordinator honest, the pre-lock refusal in `readyCoordinationTask`
  keeps every caller honest.
- **Scan-boundary shape decision.** source=implementation analysis,
  confidence=high, type=decision. `listCoordinationTaskCards` had exactly
  two consumers, both wanting healthy-only semantics. Refactoring it
  directly into a trusted projection (rather than adding a separate
  helper) keeps the public API surface minimal. The scan boundary that
  retains diagnostics (`scanCoordinationTaskCards`) is the new internal
  helper.
- **Public diagnostics field names.** source=testing,
  confidence=high, type=fact. `readCoordinationTask` initially returned
  internal camelCase (`offendingFields`); tests and quarantine entries
  use snake_case. Fixed by mapping internal diagnostics to public-facing
  snake_case (`offending_fields`, `problems`) in `readCoordinationTask`
  for consistency with quarantine entries.

## Contradictions

- **"Just coerce and carry on" vs. report-and-continue.** The 2026-07-27
  slice's silent collect-and-discard was correct for the brick fix (the
  contract was "list MUST NOT throw"), but it opened the promotion hazard.
  Resolved by adding the report-and-continue layer on top WITHOUT
  weakening the save path or redefining existing count semantics. The
  additive fields (`quarantine[]`, `degraded_count`, `healthy_total`,
  `healthy_status_counts`) preserve consumer compatibility.

## DEFER follow-ups surfaced

These are recorded for capture via `/write-task` into
`.local/coordinator/tasks/` (transport, not truth). They are NOT promoted
to `backlog.md` here — promotion requires the trigger to fire and the DoR
to be met.

- **Syntax-invalid JSON scan quarantine.** `readJson()` throws on
  malformed JSON, which is a different scanner contract (filename-level
  quarantine, no normalized task object). This slice covers
  successfully-parsed cards whose normalization/core validation yields
  degradation only.
  - `source: review-defer`
  - `trigger: path_touched(templates/core/.opencode/scripts/state-lib.js)`
  - `studied: 2026-07-28`

```behavioral-closure
verdict: proven
result: proven
crux: load-bearing path is readyCoordinationTask() (the action boundary)
  refusing a degraded card whose stored status was corrupted on disk to
  an out-of-enum value (silently coerced to "draft" by the read
  normalizer, exactly as in the 186bff1 hazard). Verifier:
  verify-task-registry.js quarantine regression block, case 7. Command:
  `node .opencode/scripts/verify-task-registry.js` → printed
  `verification: ok` with `quarantine_degraded_count: 6` and
  `quarantine_healthy_total: 109`. The OUTCOME was observed — the
  degraded card is REFUSED at readyCoordinationTask (throws StateError
  matching /degraded/i), AND it appears in quarantine[], AND valid
  sibling cards still list/read — not merely the mechanism asserted
  (a flag is set).
```
