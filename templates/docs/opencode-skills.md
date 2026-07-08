# OpenCode Skills Guide

Repo-local skills under `.opencode/skills/` are **advisory helpers**, not guaranteed behavior.

If a task depends on a specific skill for correctness, cost control, or operational safety, name that skill explicitly in the prompt, task contract, or plan.

Examples:

- "Use `repo-recon` to map the repo before editing."
- "Use `bgshell-job` for this long-running build."
- "Use `gated-commit` before staging anything."

## Rules

- Treat skills as reusable workflow shortcuts, not as hard policy boundaries.
- Repo rules still come first: `AGENTS.md`, the boundary docs, and any project
  operational docs under `docs/ai/` remain authoritative.
- Keep this guide read-when-relevant. Do not promote the whole skill catalog into always-loaded baseline instructions unless there is a deliberate reason.
- Keep detailed workflow logic in `.opencode/skills/<name>/SKILL.md`; keep only durable operator guidance here.

## Core skills

These ship with the harness. A consuming project may add its own domain skills
alongside them.

| Skill | Name explicitly when | Do not use it for |
| --- | --- | --- |
| `repo-recon` | mapping the repo, locating entrypoints/hotspots/packages/tests, or refreshing `.opencode/repo-configs/repo-recon-data.yml` after a structural change | implementation work |
| `backlog` | editing `docs/planning/backlog.md`, resolving a backlog `cas_conflict`, or routing where a DEFER/follow-up finding should land | general planning or non-backlog docs |
| `bgshell-job` | long local non-GPU shell tasks (build/release/maintenance), detached lifecycle management, and post-compaction resume of a job that may outlive one shell call | quick foreground commands |
| `gated-commit` | any git write — `git add`, `git commit`, `git push`, `git reset` — or understanding the commit-gate enforcement model | read-only git inspection |
| `harness-operator` | installing, self-updating, running the upgrade loop, reading release migration notes, or using `guide` / `update` / `doctor` | ordinary repo edits unrelated to the harness itself |
| `skill-creator` | creating, updating, validating, or troubleshooting a repo-local skill under `.opencode/skills/`, or distilling archived sessions into reusable workflows | unrelated repo work |
| `think-mode` | choosing between `researcher` / `debate` / one-shot `solution-brief` / phased `solution-brief` before starting a read-only thinking session, or after a stakeholder reframe | implementation work, or workflows the operator already named explicitly |

## When to add `docs/ai/` guidance for a skill

Add durable docs only when at least one of these is true:

- missing the skill materially increases risk or cost
- the skill corrects a false assumption about OpenCode behavior
- humans need to know when to invoke the skill manually
- multiple sessions already depend on the same skill workflow

Do not mirror every `SKILL.md` into `docs/ai/`.

## Skills most worth naming explicitly

- `repo-recon` before edits that depend on knowing the repo's structure
- `bgshell-job` for long local non-GPU detached shell work
- `gated-commit` before any git write, so the commit gate is respected
- `harness-operator` for install/update/doctor and release-note reads
- `think-mode` when the read-only workflow shape (research vs debate vs one-shot brief vs phased brief) is not obvious or a stakeholder just reframed the question
