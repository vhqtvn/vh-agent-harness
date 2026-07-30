# Decision: Coordination-task registry — lifecycle-mutability audit (4 gaps → 1 genuine defect)

**Date:** 2026-07-30
**Status:** Accepted (record-of-decision). A read-only audit of the coordination-task
registry's lifecycle-mutability boundaries against `templates/core/.opencode/scripts/state-lib.js`.
One genuine defect (#3) is captured as a DEFER; the rest are by-design or refuted. No code change
in this slice — this memo rescues a chat-only analysis into a durable home and closes the provenance
gap cited by `7eeb858`.
**Supersedes:** none.
**See also:**
- Captured DEFER card: `.local/coordinator/tasks/defer-registry-lifecycle-exit-degraded-nonresearch.json`
- Sibling (DONE): `coord-registry-resilience` (`186bff1`) — read-path coercion + enum-widen
- C4 canon clarifications: commit `7eeb858` (rm-as-dispose is the intended transport-card retire path; create DEFER/advisory candidates as `draft`)
- `researches/decisions/2026-04-29-coordination-control-plane-options.md` (stub) + `researches/decisions/2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md` (partial refill) — the control-plane design that contains **no** explicit lifecycle-mutability decision
- `templates/core/.opencode/scripts/state-lib.js` (the audited surface)

## Framing

During a `.local/coordinator/tasks/` cleanup pass, the coordinator hit four
lifecycle-mutability behaviors on the coordination-task registry. This audit
(read-only, against `templates/core/.opencode/scripts/state-lib.js`) determined
which are genuine defects versus by-design.

Two scoping facts frame every verdict below:

1. **The boundaries are mechanically enforced in `state-lib.js`.** There is no
   configuration toggle or policy layer that relaxes them; the only way past a
   refusal is a sanctioned mutator that the registry exposes (or `rm` of the
   transport file).
2. **The only documented intent is the "transport, not truth" posture.**
   `.local/coordinator/tasks/` is transport, not canonical truth; unpromoted
   candidates may be lost — this is intentionally fine. Beyond that, the
   coordination-control-plane design (the 2026-04-29 stub and its 2026-07-22
   partial refill) contains **no explicit lifecycle-mutability design decision**.
   These boundaries are **emergent, not designed-for**: they fall out of a status
   enum, a create-allowlist, an update-allowlist, and a research-only repair path
   that were each built for a narrower purpose than "a complete lifecycle API."

## The load-bearing gate

`coordinationTaskStatusTransitionErrors` (`state-lib.js:3853-3868`):

- **create allows `draft`/`ready` only** (`3857`) — a new card may not start in
  any other status.
- **ANY status change on an existing card is rejected** (`3862-3866`) — every
  non-create save where `current !== next` is refused with "Use dedicated
  lifecycle commands to move coordination tasks from …".

This one function is why `save` can neither demote (`ready`→`draft`) nor cancel
(`*`→`cancelled`). The "dedicated lifecycle commands" it points to are
`/task-ready` (promote), `/task-closeout`+`/task-review` (advance to
reported/blocked/completed) — and nothing else.

## The four gaps + verdicts

### 1. No cancel/drop for a draft/ready card — **Verdict: by-design**

No operation moves a `draft`/`ready` card to `cancelled`. Closeout
(`saveCoordinationTaskCloseout`, `5098-5133`) requires the card to be `working`
**and** have an active owner (`5102-5116`), and accepts only
`task_status ∈ {reported, blocked, completed}` (`5126-5133`) — so it cannot serve
as a cancel path for an unstarted card. The transition gate (`3853-3868`) blocks
any `*`→`cancelled` move, and `update` cannot set `status` (see gap #2).

**Why by-design:** `rm`-as-dispose is the sanctioned transport-retire path under
the "transport, not truth" posture. The registry is not a canonical ledger for
these candidates; deleting the gitignored file loses nothing durable. This is now
also documented as canon by C4 commit `7eeb858`.

### 2. `update` refuses `status` and `task_type` — **Verdict: split (status by-design; task_type folds into #3)**

Both `status` and `task_type` are absent from
`TASK_METADATA_UPDATE_PRE_EXECUTION_FIELD_NAMES` (`2036-2060`). The applicable
allowlist is selected by `allowedTaskMetadataUpdateFieldNamesForStatus` (`2120-2133`),
applied at the update validation site (`4820-4845`); an unsupported field yields
`"Unsupported fields for …"` (`2100`).

- **`status`-refusal is by-design.** Status transitions route to the dedicated
  lifecycle commands; `update`'s contract is "adjust metadata **without changing
  lifecycle state**." (AGENTS.md: "without changing lifecycle state.")
- **`task_type`-refusal folds into gap #3.** It only bites a *degraded* card that
  has lost its `task_type`: a healthy card always carries one, because `task_type`
  is required on save (`1732-1735`, enforced in the save path at `4240-4244`).
  So `task_type`-refusal is not an independent defect; it is a facet of the
  degraded-card dead-end (#3).

### 3. `repair` is research-only → degraded non-research cards have NO API lifecycle exit — **Verdict: GENUINE DEFECT**

`repairCoordinationTask` throws `"… does not use the research repair flow"` for
any `task_type !== research` (`5007-5011`). `RESEARCH_REPAIRABLE_FIELD_NAMES`
(`2028-2034`) is research-only. `update` cannot set the enum fields a degraded
card needs (#2). There is no cancel (#1). The transition gate blocks any status
change (`3853-3868`).

**Dead-end:** a degraded non-research card (e.g. one whose stored `task_type` was
corrupted/blanked) has **no API lifecycle exit** — only `rm` of the file or a
manual JSON edit recovers it. This is exactly what bricked `list_coordination_tasks`
on the vh-solara docs/verification cards that had been hand-authored without a
valid `task_type`.

**Decision: CAPTURE as a DEFER, sibling to `coord-registry-resilience`, NOT a
fold-in.** Captured as
`.local/coordinator/tasks/defer-registry-lifecycle-exit-degraded-nonresearch.json`.

- **Fix axis = write-path lifecycle completeness**, distinct from
  `coord-registry-resilience`'s read-path tolerance (the DONE sibling at `186bff1`
  added read-path enum coercion + a widened `task_type`). This DEFER preserves
  `coord-registry-resilience`'s deliberate "keep the save-path throwing validator"
  constraint: the fix touches only write-path lifecycle-*exit* mutators, not the
  save-path input check (`collectCoordinationTaskCoreFieldErrors`).
- **Highest-value fix:** generalize `repairCoordinationTask` beyond research
  (diagnostics-gated, atomic) **OR** widen `update`'s enum-field allowlist for
  draft/degraded cards. This design fork is resolved provisionally as **Hybrid/A**
  via a `/solution-brief` and is currently `need_evidence` — see the DEFER card's
  `next_action` and `open_questions`.

### 4. `save` defaults to `ready`; "no create-as-draft"; no `ready`→`draft` demote — **Verdict: split (one refuted, one defensible, one low-value-absent)**

- **(a) "no create-as-draft" is FALSE — refuted.** `write-task.md` exposes
  `optional status: draft | ready` (the command's argument block, `:18`); `save`
  accepts an explicit status (`4089-4097`); the transition gate allows create→`draft`
  (`3857`). Creating-as-draft is a fully supported, first-class path. (This is also
  why C4 `7eeb858` records that DEFER/advisory candidates should be created as
  `draft` so they do not inherit the `ready` default.)
- **(b) default → `ready` is defensible.** When `status` is omitted on create,
  `save` defaults to `ready` (`4098-4100`). This matches the common "I have a
  well-formed card, route it" case; the advisory case has the explicit `draft`
  escape hatch above.
- **(c) no `ready`→`draft` demote is genuinely absent but low-value.** The
  transition gate (`3853-3868`) blocks it, `update` cannot set `status` (#2), and
  there is no demote operation. The residual is real but avoidable: create
  advisory cards as `draft` in the first place; if an advisory already landed as
  `ready`, `rm` + recreate corrects it. Not worth a dedicated mutator.

## Decision (one genuine residual)

Only **#3** is a genuine defect. **#2**'s `task_type`-refusal and **#1**'s
no-cancel are facets of the same dead-end as #3 (a degraded card with no API
exit), not independent gaps; #2's `status`-refusal and #4 are by-design or
refuted.

The recommended fix for gap #3 touches **only write-path lifecycle-exit
mutators** (generalize `repair` beyond research, or widen `update`'s enum
allowlist for draft/degraded cards) and is fully compatible with
`coord-registry-resilience`'s "keep the save-path throw" constraint. It does not
weaken the create-allowlist, the transition gate, or the closeout whitelist.

## Authority line

- This audit **INFORMS**. It is read-only against `state-lib.js`; no coordinator
  transition authority is invoked, and no registry mutation was performed.
- The registry mutators **ACT** (refuse or accept); the coordinator reads and
  routes. Consistent with the 2026-07-22 control-plane authority line
  (coordinator state *informs*; the safety layer / mutators *act*).
- The DEFER disposition (capture, not promote) follows the DEFER-curation
  discipline: the trigger (`path_touched(templates/core/.opencode/scripts/state-lib.js)`)
  has not fired on a fixing slice, so the candidate stays parked in
  `.local/coordinator/tasks/` as transport, not a `backlog.md` row.

## Evidence / Provenance

| Claim | Verifying artifact / command | Verified |
|-------|------------------------------|----------|
| Create allows `draft`/`ready` only; any existing-card status change rejected | `state-lib.js:3853-3868` (`coordinationTaskStatusTransitionErrors`) | yes |
| Closeout requires `working` + active owner | `state-lib.js:5102-5116` (`saveCoordinationTaskCloseout`) | yes |
| Closeout status whitelist = {reported, blocked, completed} | `state-lib.js:5126-5133` | yes |
| `update` allowlist excludes `status` and `task_type` | `state-lib.js:2036-2060` (`TASK_METADATA_UPDATE_PRE_EXECUTION_FIELD_NAMES`); applied via `2120-2133`/`4820-4845`; error template `2100` | yes |
| `repair` throws for `task_type !== research` | `state-lib.js:5007-5011` (`repairCoordinationTask`) | yes |
| Repairable-field set is research-only | `state-lib.js:2028-2034` (`RESEARCH_REPAIRABLE_FIELD_NAMES`) | yes |
| `task_type` required on save | `state-lib.js:1732-1735`, enforced in save path at `4240-4244` | yes |
| `save` accepts explicit status; defaults to `ready` on create | `state-lib.js:4089-4100` | yes |
| `write-task.md` exposes `status: draft | ready` | `templates/core/.opencode/commands/write-task.md:18` | yes (spot-checked; narrowed from `:18-19` to `:18`) |
| DEFER card captured | `.local/coordinator/tasks/defer-registry-lifecycle-exit-degraded-nonresearch.json` | yes |
| Sibling DONE (read-path coercion + enum-widen) | commit `186bff1` | yes |
| rm-as-dispose + create-as-draft canon clarifications | commit `7eeb858` | yes |
| Control-plane design has no explicit lifecycle-mutability decision | `2026-04-29-coordination-control-plane-options.md` (stub) + `2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md` (partial refill) | yes |

## Status

One genuine defect (#3) captured as DEFER
`defer-registry-lifecycle-exit-degraded-nonresearch` (status `draft`, parked;
trigger `path_touched(templates/core/.opencode/scripts/state-lib.js)` not yet
fired). Gaps #1 (no-cancel), #2 `status`-refusal, and #4 are by-design or
refuted; #2 `task_type`-refusal folds into #3. No code change in this slice.
