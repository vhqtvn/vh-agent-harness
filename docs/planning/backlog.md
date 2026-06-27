# vh-agent-harness — Backlog

Source of truth for task status — the harness coordination model reads this file.

**Agents:** before substantial work, move the matching row to `in_progress` with
owner + date; on finish move it to `done` recording changed files + verification;
add follow-ups as NEW rows (new ID), never overload an existing task; if blocked,
set `blocked` with the exact blocker and the next decision needed. After any
status change run `/backlog-cleanup` (or `vh-agent-harness exec node
.opencode/scripts/normalize-backlog.js`) to keep the active sections tidy and
archive `done`/`cancelled` rows under `docs/planning/archive/`.

- **IDs:** stable `<phase>-<AREA>-<NNN>`, e.g. `P1-CORE-001`, `P2-API-003`.
- **Statuses:** `todo`, `in_progress`, `blocked` (active) · `done`, `cancelled` (history).
- **Sections:** `Now` (active focus) · `Next` (queued) · `Later` (deferred). Active
  sections hold active statuses only; the normalizer enforces and archives the rest.
- **Columns:** `ID | Status | Area | Task | Owner | Notes | Links` (Notes may carry a `YYYY-MM-DD`).

## Archive Index

Older `done` and `cancelled` history lives under [docs/planning/archive/index.md](archive/index.md) and is meant for on-demand reading instead of auto-loading into the active backlog context.

- No archive files yet.

## Now

| ID | Status | Area | Task | Owner | Notes | Links |
| --- | --- | --- | --- | --- | --- | --- |

## Next

| ID | Status | Area | Task | Owner | Notes | Links |
| --- | --- | --- | --- | --- | --- | --- |

## Later

| ID | Status | Area | Task | Owner | Notes | Links |
| --- | --- | --- | --- | --- | --- | --- |

## Done

| ID | Status | Area | Task | Owner | Notes | Links |
| --- | --- | --- | --- | --- | --- | --- |

## Cancelled

| ID | Status | Area | Task | Owner | Notes | Links |
| --- | --- | --- | --- | --- | --- | --- |
