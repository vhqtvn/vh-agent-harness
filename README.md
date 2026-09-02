# Agent Harness

<!-- Starter README seeded by vh-agent-harness (project_owned: edit freely / replace
     with your project's own README). Keep the term-contract block below intact. -->

> **Term contract (sacred).** "Agent harness" is a **HANDLE ONLY**. Whenever the term is used it MUST carry this definition:
>
> An **agent harness** is a repo-resident system of rules, memory, coordination, safety gates, and reusable workflows that makes AI coding agents — and the humans operating them — behave predictably and keep working across context resets and session boundaries.
>
> It has **six layers**: (1) **Prescriptive** — codified must/must-not rules; (2) **Cognitive** — state surviving context resets; (3) **Coordination** — routing/tracking/handoff of work; (4) **Safety** — hard guarantees enforced regardless of agent intent; (5) **Capability** — reusable roles & workflows; (6) **Environment** — the runtime they execute in.
>
> This definition travels with the handle **forever**. Do not let it drift to mean something narrower.

This repository uses **vh-agent-harness** — a repo-resident AI agent harness
installed and managed by the `vh-agent-harness` binary.

## Operating the harness

- Run `vh-agent-harness guide` for the current state and the exact next steps
  (`--json` for machine-readable output).
- Preview any change with `vh-agent-harness install --dry-run` /
  `vh-agent-harness update --dry-run`; verify with `vh-agent-harness doctor`.

## Layout

- Harness config + sources live under `.vh-agent-harness/` (profile, run-shape,
  overlays, and the `AGENTS.core.md` + `AGENTS.mission.md` compose sources).
- The agent-facing files at the repo root are the composed `AGENTS.md` and
  `CLAUDE.md`.
- `.opencode/` holds the rendered agent corpus (agents, commands, skills,
  plugins). It is generated — configure via `.vh-agent-harness/`, not by editing
  managed files in place.

Describe **Agent Harness** itself in `.vh-agent-harness/AGENTS.mission.md`
(it is composed into `AGENTS.md`), and replace the rest of this README with your
project's own overview.
