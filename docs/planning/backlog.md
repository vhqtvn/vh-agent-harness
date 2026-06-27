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
| P1-OVERLAY-001 | in_progress | overlay | Research + design how to make vh-agent-harness reliably operable by coding agents in consumer repos (overlay discoverability, the customize-opencode conflict, guide/example). Deliverable: ranked fix set — A AGENTS.core.md "extending" section, B /harness command, C `overlay new` scaffolder. |  | 2026-06-28 research prompt ready; fire research next | README.agent.md |

## Next

| ID | Status | Area | Task | Owner | Notes | Links |
| --- | --- | --- | --- | --- | --- | --- |
| P1-OVERLAY-002 | todo | overlay | Implement the chosen discoverability fixes (A/B/C) from P1-OVERLAY-001; keep templates/core domain-free. |  | depends on P1-OVERLAY-001 |  |
| P1-CONFIG-001 | todo | config | Wire coordination-hints-lib.js to read product_prefixes from .vh-agent-harness/project.config.json (remove the phase-later TODO). |  | deferred from project.config consumption |  |
| P1-RELEASE-001 | todo | release | Cut v0.1.5 once the discoverability fixes land. |  |  |  |

## Later

| ID | Status | Area | Task | Owner | Notes | Links |
| --- | --- | --- | --- | --- | --- | --- |
| P1-DOCS-001 | todo | docs | Author real docs/ai/* operational primitives for this repo (dogfood) so its own AGENTS.md refs resolve — or formally document them as project-authored. |  | dogfood self-consistency |  |
| P1-CLI-001 | todo | cli | Evaluate a release subagent/overlay (auto semver from changes, tag, push) — the consumer ask that surfaced the discoverability gap; possibly ship as an example overlay. |  |  |  |

## Done

| ID | Status | Area | Task | Owner | Notes | Links |
| --- | --- | --- | --- | --- | --- | --- |
| P1-CORPUS-010 | done | corpus | Fix validate-opencode-config.py JSONC parsing (trailing comma + interleaved comments) — unblocked all commits in consumer repos. |  | 2026-06-28 v0.1.3 |  |
| P1-CONFIG-010 | done | config | Stop shipping *.example scaffolds; add `vh-agent-harness example`; consume project.config.json at render (mission/architecture/db). |  | 2026-06-28 v0.1.4 |  |
| P1-DOCTOR-010 | done | doctor | gitignore check warns when runtime-state + __pycache__ dirs aren't ignored. |  | 2026-06-28 v0.1.2/v0.1.4 |  |
| P1-BACKLOG-010 | done | backlog | Seed docs/planning + default the backlog feature on; greenfield profile fallback fixes opencode.jsonc drift. |  | 2026-06-28 |  |
| P1-CLI-011 | done | cli | exec: run mutating commands (fix shell-guard double-gate); make `--` optional. |  | 2026-06-27 v0.1.2 |  |
| P1-CLI-010 | done | cli | Seam-aware diff/uninstall/preflight (work on seam installs, not just legacy manifest). |  | 2026-06-27 v0.1.1 |  |

## Cancelled

| ID | Status | Area | Task | Owner | Notes | Links |
| --- | --- | --- | --- | --- | --- | --- |
