# Agent Harness — Backlog

Source of truth for task status — the harness coordination model reads this file.

**Agents edit this file freely.** Two disciplines keep a backlog edit from
blocking a code commit (enforced at the commit/workflow layer, not by blocking
edits):

- **Commit backlog SEPARATELY from code.** A code commit carries code (+ tests +
  the docs that justify it); a backlog commit carries backlog rows. One backlog
  commit per work cycle is the target. Never bundle a backlog-status change into
  a code commit.
- **On `cas_conflict`, re-read + re-apply + retry — do NOT revert this file.**
  Reverting discards other agents' promoted state. Re-read from the new HEAD,
  re-apply only your rows (by stable ID), and retry.

**DEFER / p2 follow-up items NEVER become rows here directly.** Capture them in
`.local/coordinator/tasks/` as conditional candidates (transport, not
truth) with Notes provenance (`source:review-defer`/`source:p2-followup`,
`trigger:...`, `studied:YYYY-MM-DD`). They reach this file only after a trigger
fires AND the promotion Definition of Ready is met. See
`docs/coordination/PROMOTER_RUNBOOK.md`.

Load the `backlog` skill before substantial backlog work. After a batch edit,
run `/backlog-cleanup` (or `vh-agent-harness exec node
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
