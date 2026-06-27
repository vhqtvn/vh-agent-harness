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
| P1-OVERLAY-004 | todo | overlay | Slice 2 (fix C) — `vh-agent-harness overlay new <name>` scaffolder with optional `--agent` / `--command` / `--skill` selector and `--dry-run`; register in internal/cli/root.go; embedded domain-free skeleton. Appends `<name>` to `overlays:` in vh-harness-profile.yml (ownership `platform_armed`) — MUST use the schema/reconcile path, NOT a naive text edit. Strict `--dry-run` (prints file manifest + profile diff, writes nothing); validate name collisions and partial packs; unit tests required. New command surface → README.agent.md update. | build | 2026-06-28 highest-risk slice, queued behind P1-OVERLAY-003. Risk: `platform_armed` profile append via schema/reconcile; strict `--dry-run` writes nothing. See docs/checkpoints/2026-06-28-harness-agent-operability.md |  |
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
| P1-OVERLAY-001 | done | overlay | Research + design how to make vh-agent-harness reliably operable by coding agents in consumer repos (overlay discoverability, the customize-opencode conflict, guide/example). Deliverable: ranked fix set — A AGENTS.core.md "extending" section, B /harness command, C `overlay new` scaffolder. | build | 2026-06-28 research resolved → ranked fix set (A–E) + 3 operator-confirmed decisions in docs/checkpoints/2026-06-28-harness-agent-operability.md. Implementation decomposed into P1-OVERLAY-003 (Slice 1) + P1-OVERLAY-004 (Slice 2) | README.agent.md |
| P1-CORPUS-010 | done | corpus | Fix validate-opencode-config.py JSONC parsing (trailing comma + interleaved comments) — unblocked all commits in consumer repos. |  | 2026-06-28 v0.1.3 |  |
| P1-CONFIG-010 | done | config | Stop shipping *.example scaffolds; add `vh-agent-harness example`; consume project.config.json at render (mission/architecture/db). |  | 2026-06-28 v0.1.4 |  |
| P1-DOCTOR-010 | done | doctor | gitignore check warns when runtime-state + __pycache__ dirs aren't ignored. |  | 2026-06-28 v0.1.2/v0.1.4 |  |
| P1-BACKLOG-010 | done | backlog | Seed docs/planning + default the backlog feature on; greenfield profile fallback fixes opencode.jsonc drift. |  | 2026-06-28 |  |
| P1-OVERLAY-003 | done | overlay | Slice 1 (A+B+D+E) — make vh-agent-harness agent-operable: AGENTS.core.md "Extending the harness" section + reason-gated customize-opencode warning (+ add `harness` to command enum ~line 218); new `/harness` command (templates/core/.opencode/commands/harness.md); skill-creator "Where skills live" + init_skill.py overlay target; guide.go overlay step + always-on footer + `_pack-skeleton` under example. Keep templates/core domain-free. | build | 2026-06-28 DONE. What changed (A+B+D+E): always-on `## Extending the harness` section in AGENTS.core.md (anti-patterns + reason-gated customize-opencode warning + overlay one-liner + `guide`/`/harness` pointers); new shipped `/harness` command (templates/core/.opencode/commands/harness.md); skill-creator overlay-first guidance + init_skill.py overlay-target/generated-tree warning; guide.go installed-step file names + always-on `/harness` footer; embed-only overlay-pack skeleton under templates/examples/.vh-agent-harness/overlays/_pack-skeleton/; README.agent.md documents `/harness`. Touch-up: shipped `/harness` now points at `vh-agent-harness example` (ships) instead of non-shipped docs/adoption-examples/web/ (AG-F4); token-name clarity (AG-F2); checkpoint wording (AG-F5); init_skill.py matcher tightened (AG-F3). Key files: templates/core/.vh-agent-harness/AGENTS.core.md, templates/core/.opencode/commands/harness.md, templates/core/.opencode/skills/skill-creator/{SKILL.md,scripts/init_skill.py}, internal/cli/guide.go, templates/examples/.vh-agent-harness/overlays/_pack-skeleton/*, README.agent.md + rendered dogfood outputs (AGENTS.md, .opencode/commands/harness.md, .opencode/skills/skill-creator/*). Verify: go build/gofmt/go vet/go test ./... pass; dogfood `vh-agent-harness update` clean (0 conflicts); `example` lists `_pack-skeleton`; `guide` prints named files + `/harness` footer; domain-free audit clean; commit-reviewer verdict CLEAN/approve. Deferred: fix C (`overlay new`) -> Slice 2, tracked as P1-OVERLAY-004 (Next/todo). See docs/checkpoints/2026-06-28-harness-agent-operability.md | README.agent.md |
| P1-CLI-011 | done | cli | exec: run mutating commands (fix shell-guard double-gate); make `--` optional. |  | 2026-06-27 v0.1.2 |  |
| P1-CLI-010 | done | cli | Seam-aware diff/uninstall/preflight (work on seam installs, not just legacy manifest). |  | 2026-06-27 v0.1.1 |  |

## Cancelled

| ID | Status | Area | Task | Owner | Notes | Links |
| --- | --- | --- | --- | --- | --- | --- |
| P1-OVERLAY-002 | cancelled | overlay | Implement the chosen discoverability fixes (A/B/C) from P1-OVERLAY-001; keep templates/core domain-free. |  | 2026-06-28 superseded — decomposed into P1-OVERLAY-003 (Slice 1: A+B+D+E) and P1-OVERLAY-004 (Slice 2: fix C). See docs/checkpoints/2026-06-28-harness-agent-operability.md |  |
