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

## Configurable files

The harness does **not** scatter `*.example` scaffolds into the repo. Discover
and print any file's doc/template on demand:

```
vh-agent-harness example                 # list every configurable file
vh-agent-harness example <path>          # print one file's doc/template
vh-agent-harness example <path> > <path> # create it, then edit
```

This list is authoritative (it mirrors `vh-agent-harness example`). Paths are
real target locations.

| File | What to do |
| --- | --- |
| `.vh-agent-harness/project.config.json` | Fill `project.mission_summary` + `architecture_summary` (and `db_user`/`db_name` if used). Resolved into the seeded `CLAUDE.md`/`Makefile` at install — **create + fill it BEFORE `install`** (those seeds are written once). |
| `.vh-agent-harness/AGENTS.mission.md` | Write the project's domain mission/architecture/rules; composed into root `AGENTS.md` on `update`. |
| `.vh-agent-harness/vh-harness-profile.yml` | (armed, seeded) Select features + `overlays: [<pack>]` (S3). |
| `.vh-agent-harness/run-shape.yml` | (seeded host-shell) Set runtime `backend:` (`host-shell`/`docker_compose`/`proxy`) + `compose_file`/`default_service` or `proxy_command`; lifecycle hooks/verbs (S4). |
| `.vh-agent-harness/harness-ownership.yml` | (optional; not seeded) Raise-only ownership overrides — create only to take a managed file to `project_owned`. |
| `.vh-agent-harness/overlays/<pack>/` | Project overlay: `agents/`, `commands/`, `skills/`, `opencode-append.jsonc`, `permission-pack.jsonc`, `callable-graph-snippet.md`. |
| `.opencode/repo-configs/forbidden-patterns.project.js` | (seeded blank) Project deny-rules (import builders from `forbidden-patterns.core.js`; each rule needs a `why`). |
| `.opencode/plugins/compaction-primitives.project.md` | Project compaction-recovery block (operational primitives an agent needs after context loss). |
| `docs/coordination/LANES.yaml`, `docs/coordination/ROLES.md` | Coordination lanes/roles — define project-specific ones or keep the generic set. |
| `.local/cleared-assumptions.yaml` | Operator-state ledger of cleared assumptions (usually operator-maintained). |

Do **not** edit: `lineage.yml` (binary-owned), `AGENTS.core.md` (managed
compose source), or anything under `.opencode/` that is platform_managed.

## Common tasks

- **Install / adopt:** `vh-agent-harness install --name <Name> --slug <slug>`
  (run with `--dry-run` first). Then `vh-agent-harness guide` for config steps.
- **Add domain agents/commands/skills:** create `.vh-agent-harness/overlays/<pack>/`,
  list `<pack>` under `overlays:` in `vh-harness-profile.yml`, then `update`.
- **Describe the project:** `vh-agent-harness example .vh-agent-harness/AGENTS.mission.md
  > .vh-agent-harness/AGENTS.mission.md`, fill it in, `update` (composes `AGENTS.md`).
- **Configure any file:** `vh-agent-harness example <path>` prints its doc/template
  (no `*.example` files are shipped into the repo). `vh-agent-harness example` lists all.
- **Run a command in the runtime:** `vh-agent-harness exec -- <cmd>` (the `--` is
  optional — the command's own flags pass through, e.g. `exec bash -c '…'`,
  `exec pytest -k x`; put any harness flags BEFORE the command). Mutating
  commands are allowed; only forbidden-patterns and the commit-gate are blocked.
  Put env vars / `timeout` INSIDE the command (`exec bash -c 'FOO=1 cmd'`), never
  as a host prefix.
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
