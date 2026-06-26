# vh-agent-harness — agent operating manual

You are an AI agent operating `vh-agent-harness`, a single static binary that
installs and manages a repo-resident agent harness. This file tells you how to
drive it. The binary is self-guiding: **when in doubt, run `vh-agent-harness
guide` (or `guide --json`)** — it reports the current state and the exact next
steps. This document is the static reference behind that.

## The loop

```
guide            → detect state + next steps   (run this first, any repo)
install/update   → apply changes               (always preview with --dry-run first)
doctor           → verify health
```

Everything is idempotent and fail-closed: a re-run does not double-apply, and an
ambiguous/unsafe plan aborts before writing.

## Phases `guide` reports

- **greenfield** — no harness here. → `install --name <Name> --slug <slug>`.
- **adoptable** — an existing `.opencode` not yet managed by vh-agent-harness.
  → `install` adopts it; managed files are refreshed, your project-owned files
  preserved. Preview with `--dry-run` first.
- **installed** — configure: write the mission, add overlays, set the runtime,
  add deny-rules, then `update`.

## Golden rules

1. **Preview before applying.** `install --dry-run` / `update --dry-run` print
   the full per-file plan (overwrite / seed / preserve / reconcile / conflict)
   and write nothing. Read it before the real run.
2. **Never hand-edit a `platform_managed` file** — `update` overwrites it. To
   change managed behavior, use a seam: overlay, profile, run-shape, mission,
   project deny-rules, or a raise-only ownership override.
3. **Edit config under `.vh-agent-harness/`, not the rendered `.opencode/` tree.**
   The `.opencode/` tree is generated; your inputs live in `.vh-agent-harness/`.
4. **Prefer `--json`** where offered (`guide --json`) for reliable parsing.

## Files you edit (all under `.vh-agent-harness/` unless noted)

| File | Purpose |
| --- | --- |
| `vh-harness-profile.yml` | select features + `overlays: [<pack>]` (S3, armed) |
| `run-shape.yml` | runtime `backend:` (`host-shell`/`docker_compose`/`proxy`) + `proxy_command` (S4) |
| `AGENTS.mission.md` | your project's domain mission; composed into root `AGENTS.md` |
| `overlays/<pack>/` | project overlay: `agents/`, `commands/`, `skills/`, `opencode-append.jsonc`, `permission-pack.jsonc`, `callable-graph-snippet.md` |
| `harness-ownership.yml` | raise-only ownership overrides (take a managed file to `project_owned`) |
| `.opencode/repo-configs/forbidden-patterns.project.js` | project deny-rules (import builders from `forbidden-patterns.core.js`; each rule needs a `why`) |

Do **not** edit: `lineage.yml` (binary-owned), `AGENTS.core.md` (managed
compose source), or anything under `.opencode/` that is platform_managed.

## Common tasks

- **Install / adopt:** `vh-agent-harness install --name <Name> --slug <slug>`
  (run with `--dry-run` first). Then `vh-agent-harness guide` for config steps.
- **Add domain agents/commands/skills:** create `.vh-agent-harness/overlays/<pack>/`,
  list `<pack>` under `overlays:` in `vh-harness-profile.yml`, then `update`.
- **Describe the project:** copy `AGENTS.mission.md.example` →
  `AGENTS.mission.md`, fill it in, `update` (composes `AGENTS.md`).
- **Use an existing wrapper for execution:** in `run-shape.yml` set
  `backend: proxy` and `proxy_command: ["./dev.sh", "exec"]`.
- **Refresh after a new binary or config change:** `vh-agent-harness update`
  (preview with `--dry-run`). Armed-file conflicts are recorded — list them with
  `vh-agent-harness proposals`.
- **Verify:** `vh-agent-harness doctor` (lineage, armed-schema, managed-drift,
  environment). `vh-agent-harness diff` shows drift vs. the corpus.

## What is safe

- `install`/`update` never overwrite `project_owned` files that already exist,
  and never write at all under `--dry-run`.
- Adopting an existing repo preserves `.gitignore`, `README.md`, `CLAUDE.md`,
  `Makefile`, and any `AGENTS.md`; it refreshes only generic managed files.
- `exec`/`shell` always run the shell-guard permission gate before touching the
  runtime, including the `proxy` backend.

When a command prints a "Next steps" footer, follow it. When unsure, re-run
`vh-agent-harness guide`.
