# Promoter Runbook

> **Term contract.** "Agent harness" is a **HANDLE ONLY**. This runbook carries
> the **Coordination** layer of the harness (routing/tracking/handoff of work).

This is the operational runbook for the **promoter** role in the W1
single-writer-promotion model. It is a procedure, not code.

## Why this role exists

Worker agents (`build`, `docs-steward`) are denied direct edits to
`docs/planning/backlog.md` via the per-agent permission map. This decouples
worker progress from shared-file commit contention: a worker never commits a
backlog blob, so a backlog conflict can never block a clean code commit. The
cost is that the canonical status file needs a single writer to keep it current
— the **promoter**.

## Who runs it

- **Initially:** the operator, or a `coordination` session acting for the
  operator. No dedicated agent is required to start.
- **Later (optional):** a dedicated promoter agent may be introduced. Until
  then, the promoter is a human-in-the-loop responsibility.

The promoter is the **only** role that edits `docs/planning/backlog.md`.

## Cadence

Promote on any of these triggers:

- **Per cycle:** at the end of a work cycle (a slice or fan-in batch lands).
- **On demand:** when an operator or coordinator needs the canonical backlog to
  reflect current reality (e.g. before opening a new cycle, before a release).
- **Per-N-completions:** after every N task closeouts, to bound the stale-status
  window (N is operator choice; start with 1 per cycle and relax if it holds).

The goal is to keep the stale-status window **bounded**, not zero. Live state
during a cycle lives in `.local/coordinator/tasks/`; the backlog is the
promoted, durable view.

## Procedure

1. **Read the live transport.** Open `.local/coordinator/tasks/` and
   scan for closeouts (`/task-closeout`), status updates (`/task-update`), and
   coordinator decisions (`/task-review`) that have not yet been promoted.
2. **Reconcile against canon.** Open `docs/planning/backlog.md` and identify the
   rows whose status, owner, notes, or links need to change to match the live
   closeouts. Match by stable task ID (`<phase>-<AREA>-<NNN>`).
3. **Batch-edit the backlog.** Apply all pending transitions in one edit pass:
   - `todo` → `in_progress` (work started)
   - `in_progress` → `done` (closeout filed, with changed files + verification)
   - `in_progress` → `blocked` (exact blocker + next decision)
   - new rows for newly-discovered follow-ups (new ID, never overload)
   - `cancelled` for abandoned items (with a short reason)
4. **Normalize.** Run `/backlog-cleanup` (or
   `vh-agent-harness exec node .opencode/scripts/normalize-backlog.js`) so
   `Now` / `Next` / `Later` stay active-only and history is archived under
   `docs/planning/archive/`.
5. **Commit.** Delegate a single gated commit via the committer agent, passing
   `docs/planning/backlog.md` (and any archived rows) as the explicit file list.
   The promoter does not run `git commit` directly.
6. **Confirm.** Verify the backlog reflects the promoted state and the
   stale-status window is closed for this cycle.

## Stale-status window

`docs/planning/backlog.md` **lags live state** between promoter runs. This is
intentional and bounded:

- **Workers** reading the backlog mid-cycle may see stale `in_progress` /
  assignment rows. The `.local/coordinator/tasks/` cards are the live
  view during a cycle.
- **Do not hide the window.** If a worker notices a stale row, the correct
  response is to check the task card, not to edit the backlog directly (denied).

## Recovery behavior

### Missed promotions (backlog drifted behind reality)

If the promoter has not run for several cycles and the backlog is stale:

1. Read all task cards in `.local/coordinator/tasks/` since the last
   promotion.
2. Reconcile each against the backlog row by ID.
3. Batch-edit and normalize in one pass, then commit. Do not promote piecemeal —
   a single consolidated promotion keeps the history coherent.

### Promoter is down / unavailable

Workers continue uninterrupted: they route status to transport and keep working.
The backlog simply stays stale until the promoter returns. No worker is blocked
by the promoter being down, because workers never edit the backlog.

### Conflict on promotion (rare)

Because the promoter is the sole backlog writer, a content conflict on
`docs/planning/backlog.md` should not occur under normal operation. If one does
(e.g. two coordination sessions both acted as promoter), resolve via the
committer's conflict path — do not `revert` the backlog, since that loses
promoted state. Reconcile manually from the task cards and re-promote.

## What the promoter does NOT do

- Does not edit code, tests, or non-backlog docs (that is worker territory).
- Does not synthesize technical conclusions (that is the synthesizer's job at
  fan-in; the promoter only promotes status).
- Does not run the normalizer as a substitute for editing — normalize only
  after the batch edit lands.
- Does not bypass the gated commit. Promotion commits go through the committer
  like any other.

## Reference

- [README.md](README.md) — Canonical State Map and the single-writer/promoter model.
- [TASK_MODES.md](TASK_MODES.md) — Non-Negotiable #2 (one writer promotes to canon).
- [RUNTIME_MODEL.md](RUNTIME_MODEL.md) — transport-versus-truth and promotion rules.
