# vh-agent-harness — agent operating manual

You are an AI agent operating `vh-agent-harness`, a single static binary that
installs and manages a repo-resident agent harness. This file tells you how to
drive it. The binary is self-guiding: **when in doubt, run `vh-agent-harness
guide` (or `guide --json`)** — it reports the current state and the exact next
steps. This document is the static reference behind that.

Running `vh-agent-harness` with **no arguments** prints the root help (exit 0):
a static command-surface map, the agent-orientation block, the upgrade loop, and
a pointer to `guide`. It is the quick orientation view; `guide` is the dynamic,
repo-aware advisor.

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

## Upgrade loop (after a binary self-update)

When the binary itself changes (`self-update` pulled a new release), re-render the
corpus and verify health in this order:

```
vh-agent-harness self-update      # pull the new binary
vh-agent-harness update --dry-run # ownership-safe preview of the re-render
vh-agent-harness update           # applies platform_managed + active overlay_extension
vh-agent-harness doctor           # lint the result
```

To see what changed in a release and how to migrate, inspect its migration note
(see "Common tasks" → "Inspect migration notes for a release").

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
- **Add domain agents/commands/skills:** `vh-agent-harness overlay new <pack>
  --agent <n> [--command <n>] [--skill <n>]` scaffolds the pack and wires it into
  `vh-harness-profile.yml` in one command (see "Scaffolding an overlay pack"
  below). Then `update --dry-run` and `update`. Or run `/harness` for the full
  manual recipe; a commented pack skeleton is under `vh-agent-harness example`.
  Do NOT edit the generated `.opencode/` tree or `opencode.jsonc` — those
  regenerate on `update`.
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
- **Track work:** the backlog at `docs/planning/backlog.md` is seeded on install
  (source of task truth). Before/after work, update the matching row's status;
  run `/backlog-cleanup` (or `vh-agent-harness exec node
  .opencode/scripts/normalize-backlog.js`) to tidy/archive. Roadmap intent lives
  in `docs/planning/roadmap.md`. Both are `project_owned` (never clobbered).
- **Refresh after a new binary or config change:** `vh-agent-harness update`
  (preview with `--dry-run`). Armed-file conflicts are recorded — list them with
  `vh-agent-harness proposals`.
- **Inspect migration notes for a release:** `vh-agent-harness help migrate`
  (the note for the locally adopted harness version, detected from lineage) or
  `vh-agent-harness help migrate vX.Y.Z` (a specific release). With no version
  and no local install, it prints the latest bundled note. It is **documentation
  only** — it never modifies files.
- **Verify:** `vh-agent-harness doctor` (lineage, armed-schema, managed-drift,
  environment). `vh-agent-harness diff` shows drift vs. the corpus.

## Extending the harness (`/harness`)

`/harness` is the OpenCode slash command that carries the full
add-an-agent / add-command / add-skill recipe and the overlay anatomy. Use it
**whenever you are asked to extend the harness** — add a subagent, a `/command`,
or a skill — instead of editing the generated tree.

The `.opencode/` tree and `opencode.jsonc` are **generated**; they regenerate on
every `update`, so edits there vanish. The extension unit is an **overlay pack**
at `.vh-agent-harness/overlays/<pack>/` (agents/commands/skills + merge-content
files), selected under `overlays:` in `vh-harness-profile.yml`. Do NOT use
OpenCode's built-in `customize-opencode` skill to change the harness — use an
overlay pack (only `customize-opencode` for a reason unrelated to the generated
tree).

What `/harness` gives you:
- the **golden path** (numbered): `guide` → create pack → `agents/<name>.md` →
  `opencode-append.jsonc` (agent block + task allow-injections into
  `build`/`coordination`/`project-coordinator`) → optional
  `permission-pack.jsonc` / `callable-graph-snippet.md` / `commands/<name>.md` →
  list under `overlays:` → `update --dry-run` then `update` → `doctor` + restart.
- the **overlay anatomy** (unit files vs merge-content files vs extend snippets).
- the **shadowing rule**: to REPLACE a core builtin, don't shadow from a pack
  (it fails closed) — raise the path to `project_owned` in
  `harness-ownership.yml` and edit the live file.

Reference: `docs/adoption-examples/web/` is a worked (non-shipped) overlay.
Skeleton files: `vh-agent-harness example` lists `_pack-skeleton`.

## Scaffolding an overlay pack (`overlay new`)

`vh-agent-harness overlay new <pack> [--agent <n>] [--command <n>] [--skill <n>]
[--dry-run] [-o/--target <dir>]` is the one-command path from "I need a new agent /
command / skill" to a renderable overlay pack. It writes the pack and wires it
into the profile for you.

What it creates (under `.vh-agent-harness/overlays/<pack>/`):
- `agents/<n>.md` when `--agent <n>` is given (subagent skeleton; frontmatter
  `description` + `mode: subagent`).
- `commands/<n>.md` when `--command <n>` is given (slash-command skeleton;
  frontmatter `description` + `agent` + `subtask`).
- `skills/<n>/SKILL.md` when `--skill <n>` is given (skill skeleton; frontmatter
  `name` + `description` + `compatibility`).
- `opencode-append.jsonc` (always) — when `--agent` is given this is ACTIVE: the
  agent block + `task: { <n>: "allow" }` injections into
  `build`/`coordination`/`project-coordinator` (so the pack is immediately
  functional after `update`). With no `--agent` it is a commented no-op shell.
- `permission-pack.jsonc` (always) — a LIVE self-descriptor: effective on the
  next `update` once the pack is listed under `overlays:`. The scaffolded agent
  is a committer-delegator (`gateExempt: true`), so its `location` block omits
  `gate` by contract (see the file header).
- `callable-graph-snippet.md` (always) — fully HTML-commented, inert until you
  uncomment it.

If you omit ALL of `--agent`/`--command`/`--skill`, the command still creates
the pack (the three always-on files above) and prints a stderr warning: it is a
minimal pack with no `.md` unit skeletons. Add a unit later and re-run with a
new pack name (existing packs are never overwritten).

Profile wiring (the high-risk part — done safely): `<pack>` is appended to the
`overlays:` list in `.vh-agent-harness/vh-harness-profile.yml` through the
schema's own load/marshal path (the same one `update` reconciles with) — **not**
a text/regex edit. The file is `platform_armed`; the append is structural, so a
subsequent `update` raises no conflict/proposal on it. ("Clean" here means no
armed-file conflict — not that the whole `update` is a no-op; a first `update`
still emits normal platform-seed/managed applies for the new pack.) If `<pack>`
is already selected, it is not duplicated.

Fail-closed behavior:
- `--dry-run` prints the full file-creation manifest AND the exact `overlays:`
  diff (before → after) and writes **nothing**.
- If the pack dir already exists, or any target file already exists, the command
  errors with a clear message and writes nothing (never overwrites).
- Pack + unit names must be filesystem-safe (lowercase alphanumerics, internal
  `.`/`-`/`_`, starting/ending alphanumeric).
- Requires `.vh-agent-harness/` to exist at `--target` (default: cwd) — run
  `vh-agent-harness install` first.

Golden path: `overlay new <pack> --agent <n> --dry-run` (preview) →
`overlay new <pack> --agent <n>` (apply) → `update --dry-run` (expect 0
conflicts) → `update` (renders the pack into `.opencode/`, materializes the
permission-pack, AND resolves the new agent's `permission.bash`/`permission.task`
blocks + `delegateFrom` edges via the Go-native emitter — no separate step) →
`doctor` (healthy).

## What is safe

- `install`/`update` never overwrite `project_owned` files that already exist,
  and never write at all under `--dry-run`.
- Adopting an existing repo preserves `.gitignore`, `README.md`, `CLAUDE.md`,
  `Makefile`, and any `AGENTS.md`; it refreshes only generic managed files.
- `exec`/`shell` always run the shell-guard permission gate before touching the
  runtime, including the `proxy` backend.

## Re-seeding a project_owned file (CLAUDE.md / Makefile)

`CLAUDE.md` and `Makefile` are `project_owned`: they are **seeded once** at
install (resolved from `.vh-agent-harness/project.config.json`) and then
**preserved byte-for-byte** on every `update`. That preserve-once rule is the
safety contract — but it has a consequence: a plain `vh-agent-harness update`
**cannot** push a template fix into a file that already exists on disk. If a
seeded file is stale (an old template body, blank where tokens should be, or you
changed `project.config.json` and want the new values baked in), `update` will
leave your existing copy untouched.

The sanctioned recovery is **delete-then-update** (a manual re-seed):

```
rm CLAUDE.md            # or: rm Makefile
vh-agent-harness update  # re-seeds the deleted file from the current template
```

- **Warning: this loses local edits.** `CLAUDE.md`/`Makefile` are yours after
  seed; `rm` discards any hand-edits. Back the file up first if you need them.
  If you want a managed file to track the platform template forever instead, do
  NOT use this — raise its ownership to `platform_managed` is not allowed
  (downgrades are rejected); the intended path for always-managed content is an
  overlay or the composed `AGENTS.md` (core+mission), not `CLAUDE.md`.
- Re-seed re-reads `.vh-agent-harness/project.config.json` at render time, so
  fill that **before** the `update` if you want the new token values in the seed.
- This is the Slice-1 workaround. Automatic stale-seed detection and a `--reseed`
  flag are deferred (tracked in the backlog); today the operator notices the
  staleness and runs the two commands above.

## Update guard (running `update` in an uninitialized directory)

`update` deliberately adopts any tree it is pointed at — that is how it can
re-render a pre-seam project. The flip side is that a hand-run `update` in the
wrong (uninitialized) directory would scaffold managed files (`.opencode/`,
etc.) you then have to remove by hand. To prevent that, `update` asks for
confirmation before adopting when **all** of these hold:

- the target has **no** `.vh-agent-harness/vh-harness-profile.yml` (the
  authoritative "is this a harness project" signal), and
- stdin is a **TTY** (you are running it by hand), and
- nothing bypasses the prompt.

The prompt names the absolute target dir and suggests previewing with
`--dry-run` first. Decline (no / empty / EOF) and `update` writes nothing and
exits 0; accept and it proceeds as usual.

The prompt is **bypassed automatically** for non-interactive callers, so the
agent / dogfood / CI path stays frictionless. It is skipped when any of:

- stdin is **not** a TTY (the actual mechanism) — piped/redirected input,
  agents, CI, `/harness`, and `make update` only when its stdin is not a TTY
  (an interactive `make update` in a real terminal still inherits the TTY and
  still prompts), or
- `RUN_FROM_AGENT=1` is set (truthy: `1`/`true`/`yes`/`on`), or
- `--force` / `-f` is passed, or
- `--dry-run` is passed (it writes nothing, so it is safe to run anywhere and
  never prompts).

An **initialized** target (profile present) never prompts, regardless of TTY or
flags — the guard only guards the adopt-into-uninitialized case. `install`
remains the explicit "I mean it" path and is unaffected.

## Migration-note convention (releasing)

Every release ships a migration note so operators and agents know what changed
and how to migrate. Notes are **binary/help-surface docs**, not consumer corpus:

- They live in **`templates/migrations/<vX.Y.Z>.md`** — **outside** `templates/core/`.
- They are **embedded** in the binary (`//go:embed templates/migrations/*` in
  `corpus.go`), read by `help migrate` **only from the embedded copy** (never the
  live filesystem), and **NOT rendered into consumer repos** and get **no
  ownership class** in `core_manifest.go`.
- **One note per release**, named `vMAJOR.MINOR.PATCH.md`. A Go test
  (`TestMigrationNotes_Canonical`) enforces the filename and the canonical
  heading set, so a malformed note fails CI rather than shipping silently.

Every note must contain these headings (in order):

```
# Migration: vX.Y.Z
## Summary
## What changed (consumer-visible only)
## How to migrate (automated)        # must include the upgrade-loop command sequence
## What `update` handles for you
## Watch-outs
## Verification commands
## Rollback
## Non-consumer changes
```

The `## How to migrate (automated)` section must include the sequence
`self-update` → `version` → `update --dry-run` → `update` → `doctor`.

There is **no top-level `migrate` command** — the surface is `help migrate
[version]` only (intercepted inside the help command), keeping the command list
free of a `migrate` verb.

When a command prints a "Next steps" footer, follow it. When unsure, re-run
`vh-agent-harness guide`.
