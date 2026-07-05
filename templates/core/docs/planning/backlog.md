# {{PROJECT_NAME}} — Backlog

Source of truth for task status — the harness coordination model reads this file.

**Workers (`build`, `docs-steward`):** direct edits to this file are DENIED via
the per-agent permission map (W1 single-writer-promotion). Route status intents
through the local task-card lifecycle instead:

- before work — `/task-update` (or `/write-task` if no card yet) to record
  `in_progress` + owner + date
- on finish — `/task-closeout <id>` with changed files + verification
- if blocked — `/task-update` with the exact blocker + next decision
- new follow-ups — new task card with a new ID, never overload an existing task

This file **lags live state** between promoter runs; `.local/{{COORDINATOR_DIR}}/tasks/`
is the live view during a work cycle. The **promoter** (operator/coordination
initially) is the sole writer of this file and batch-promotes consolidated
results per cycle, then runs `/backlog-cleanup` (or `vh-agent-harness exec node
.opencode/scripts/normalize-backlog.js`) to keep the active sections tidy and
archive `done`/`cancelled` rows under `docs/planning/archive/`.

- **IDs:** stable `<phase>-<AREA>-<NNN>`, e.g. `P1-CORE-001`, `P2-API-003`.
- **Statuses:** `todo`, `in_progress`, `blocked` (active) · `done`, `cancelled` (history).
- **Sections:** `Now` (active focus) · `Next` (queued) · `Later` (deferred). Active
  sections hold active statuses only; the normalizer enforces and archives the rest.
- **Columns:** `ID | Status | Area | Task | Owner | Notes | Links` (Notes may carry a `YYYY-MM-DD`).

## Now

| ID | Status | Area | Task | Owner | Notes | Links |
| --- | --- | --- | --- | --- | --- | --- |

## Next

| ID | Status | Area | Task | Owner | Notes | Links |
| --- | --- | --- | --- | --- | --- | --- |

## Later

| ID | Status | Area | Task | Owner | Notes | Links |
| --- | --- | --- | --- | --- | --- | --- |
