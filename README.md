# vh-agent-harness

> **Term contract (sacred — this definition travels with the handle everywhere).**
>
> **"Agent harness" is a HANDLE ONLY.** Whenever the term is used, it MUST carry this definition:
>
> > An **agent harness** is a repo-resident system of rules, memory, coordination, safety gates, and reusable workflows that makes AI coding agents — and the humans operating them — behave predictably and keep working across context resets and session boundaries.
>
> It has **six layers**: (1) **Prescriptive** — codified must/must-not rules; (2) **Cognitive** — state surviving context resets; (3) **Coordination** — routing/tracking/handoff of work; (4) **Safety** — hard guarantees enforced regardless of agent intent; (5) **Capability** — reusable roles & workflows; (6) **Environment** — the runtime they execute in.

`vh-agent-harness` is a single static Go binary that installs, manages, and runs
a repo-resident AI agent harness. Module `github.com/vhqtvn/vh-agent-harness`.

**Operating it from an agent?** See [`README.agent.md`](./README.agent.md) — a
concise operating manual written for an AI agent driving the binary.

## Quick start

```sh
# install the binary (see Install below), then, in any repo:
vh-agent-harness guide          # detects state, prints the exact next steps
vh-agent-harness install --name "My Project" --slug my-project --dry-run   # preview
vh-agent-harness install --name "My Project" --slug my-project             # apply
vh-agent-harness doctor         # verify health
```

`guide` is the entry point: point a fresh agent (or yourself) at a repo and it
reports the harness phase — **greenfield**, **adoptable** (an existing
`.opencode` not yet managed), or **installed** — and the concrete next steps.
Add `--json` for machine-readable output.

## Architecture

- **One static Go binary** (`vh-agent-harness`): installer + manager + executor.
- **Config-driven render** of an embedded, domain-free core corpus
  (`templates/core/`, `go:embed`) through the **substrate seam**:
  render-into-staging → classify by ownership → plan per-class (fail-closed
  before any write) → apply → write lineage.
- **Ownership classes** decide what a re-render may touch: `platform_managed`
  (force-overwritten), `platform_armed` (schema-reconciled), `project_owned`
  (seeded once, then preserved forever), `overlay_extension`, `external_generated`.
- **Runtime backend abstraction** (`host-shell`, `docker_compose`, `bare`,
  `proxy`) selected by `.vh-agent-harness/run-shape.yml`; `exec`/`shell` run the
  shell-guard permission gate first.
- **Lineage-governed, repo-relative state**: `.vh-agent-harness/lineage.yml` is
  the S1 install authority.

## Install

Grab the latest release binary with the install script:

```sh
curl -fsSL https://raw.githubusercontent.com/vhqtvn/vh-agent-harness/main/install.sh | bash
```

It downloads the release archive for your OS/arch, verifies it against
`checksums.txt`, and installs the `vh-agent-harness` binary to `/usr/local/bin`
when writable; otherwise it asks whether to install to the system path (via
`sudo`) or your user path (`$XDG_BIN_HOME` or `~/.local/bin`, no sudo). Override
the system target with `INSTALL_DIR=...`.

`vh-agent-harness self-update` upgrades the binary in place using the same
verified-download flow. (That is distinct from `vh-agent-harness update`, which
re-renders the harness *inside a project*.)

## Where harness state lives

All harness configuration and sources live under `.vh-agent-harness/`, keeping
the repo root clean. The agent-facing files at the root are the composed
`AGENTS.md`, `CLAUDE.md`, and the OpenCode entrypoint `opencode.jsonc`, plus the
project's own identity files.

```
.vh-agent-harness/
  lineage.yml                 S1 — install/render authority (binary-owned)
  vh-harness-profile.yml      S3 — feature + overlay selection (platform_armed)
  run-shape.yml               S4 — runtime backend + verbs (seeded host-shell; edit freely)
  harness-ownership.yml       S2 — raise-only ownership overrides (operator-authored; not seeded)
  overlays/<pack>/            project-supplied overlay packs (agents/commands/skills)
  AGENTS.core.md              compose source — generic rules (managed)
  AGENTS.mission.md           compose source — project domain (you create it; see `example` below)
.opencode/                    rendered agent corpus (agents, commands, skills, plugins)
opencode.jsonc                OpenCode entrypoint config (root)
AGENTS.md                     composed = AGENTS.core.md + AGENTS.mission.md (written by `update` once a mission exists)
```

Project-identity files (`.gitignore`, `README.md`, `CLAUDE.md`, `Makefile`) are
`project_owned`: the harness seeds a scaffold once on a greenfield install and
**never clobbers** them on update, so installing into an existing repo is safe.

The harness does **not** scatter `*.example` scaffolds into your repo. To create
or configure a file, print its embedded doc/template on demand and redirect it:

```
vh-agent-harness example                                          # list configurable files
vh-agent-harness example .vh-agent-harness/AGENTS.mission.md      # print one
vh-agent-harness example .vh-agent-harness/project.config.json > .vh-agent-harness/project.config.json
```

## Command surface

Running `vh-agent-harness` with **no arguments** prints the root help (exit 0):
a static command-surface map, the agent orientation block, the upgrade loop, and
a pointer to `guide`. Run `vh-agent-harness guide` for dynamic, repo-aware next
steps.

The command table mirrors the four `--help` groups (Lifecycle, Orientation,
Health & diagnostics, Runtime). Every command listed below is a registered
top-level command; run `vh-agent-harness <command> --help` for per-command
detail.

| Group | Commands |
| --- | --- |
| Lifecycle | `install`, `update`, `uninstall` (`install`/`update` take `--dry-run`; `update` also takes `-f`/`--force`), `overlay` (`overlay new` scaffolds a pack; `overlay docs` prints a pack doc), `self-update` (upgrade the binary in place, distinct from `update`) |
| Orientation | `guide` (state + next steps; `--json`), `example` (print a config file's doc/template), `docs` (print a generic agent-workflow doc), `sys-prompt` (print a named system prompt), `help [command]` / `help migrate [version]` (read-only migration notes) |
| Health & diagnostics | `preflight`, `doctor`, `proposals` (armed-conflict ledger), `diff` (drift vs. the embedded corpus), `diagnostics-export` (`--dry-run`; redacted, shareable bundle), `status`, `version` |
| Runtime | `exec`, `exec-ro` (read-only intent; host-side classifier; no prompt), `exec-sandbox` (host-local Landlock + seccomp; never reaches the backend), `shell`, `up`, `down`, `logs`, `ps` |

The exec family — `exec`, `exec-ro`, `exec-sandbox`, `shell` — is intentionally
kept as **distinct verbs** (do not unify them): `exec`/`shell` dispatch through
the runtime backend, `exec-ro` is a host-side read-only classifier, and
`exec-sandbox` is a host-local trampoline. See `README.agent.md` (the exec-family
/ "two execution planes" section) for when to reach for each.

`help migrate [version]` prints the per-release migration note. With no version
it shows the note for the locally adopted harness version (detected from
lineage), or the latest bundled note when none matches; an explicit `vX.Y.Z`
(or `X.Y.Z`) prints that release's note. It is **documentation only** — it never
modifies files. The notes are embedded in the binary (under
`templates/migrations/`) and are not rendered into consumer repos.

`--dry-run` on `install`/`update` prints the full per-file plan
(would-overwrite / seed / preserve / reconcile / conflict) **without writing
anything** — a safe preview before applying.

`update` confirms before scaffolding into an **uninitialized** directory when
run interactively (no `.vh-agent-harness/vh-harness-profile.yml`). The prompt is
bypassed for non-interactive callers (piped stdin, agents, CI), by
`RUN_FROM_AGENT=1`, or with `-f`/`--force`. `make update` only bypasses when
its stdin is not a TTY (an interactive `make update` in a terminal still
prompts). `--dry-run` also bypasses it (it writes nothing). See
`README.agent.md` for the full guard rules.

## Adoption & extension

A consuming project extends the managed core without editing managed files:

- **Overlays** — drop a pack at `.vh-agent-harness/overlays/<name>/` (its
  `agents/`, `commands/`, `skills/`, plus `opencode-append.jsonc` /
  `permission-pack.jsonc` / `callable-graph-snippet.md`) and select it under
  `overlays:` in `vh-harness-profile.yml`. Packs load from the **project** first,
  then the embedded FS, so the binary stays domain-free.
- **Runtime** — set the backend in `run-shape.yml`. `backend: proxy` +
  `proxy_command: ["./dev.sh", "exec"]` delegates `exec`/`shell` to an existing
  wrapper script (the shell-guard gate still runs first), so a project keeps its
  domain runtime knowledge instead of re-encoding it.
- **Mission** — write `.vh-agent-harness/AGENTS.mission.md` (copy the `.example`);
  the seam composes `AGENTS.md = AGENTS.core.md + AGENTS.mission.md`.
- **Deny-rules** — add project rules to
  `.opencode/repo-configs/forbidden-patterns.project.js` (import the shared
  inspector builders from `forbidden-patterns.core.js`).
- **Take over a managed file** — raise it to `project_owned` in
  `harness-ownership.yml` (raise-only).
- **Operational docs (`docs/ai/`)** — several managed agent prompts and commands
  tell agents to consult project operational primitives under `docs/ai/` (e.g.
  `docs/ai/codebase-operational-primitives.md`, `shell-execution.md`,
  `dev-environment.md`): canonical paths, helper functions, container/service
  names, env conventions, API shapes. These are **domain knowledge the harness
  cannot ship** (the core stays domain-free), so the harness does **not** seed
  them — authoring them is the adopting project's job. Until you create them the
  references are simply forward pointers; create the ones your agents need so
  they stop rediscovering project facts from scratch.

Adopting an existing hand-maintained harness is the **adoptable** path: run
`install` (preview with `--dry-run`); managed files are refreshed and your
project-owned files preserved. Move domain agents/commands/skills into an
overlay pack so they survive future updates.

## Build from source

```sh
go build -o bin/vh-agent-harness ./cmd/vh-agent-harness
./bin/vh-agent-harness version
go test ./...
```

Version/build metadata are injected via `-ldflags` into `internal/cli` at
release time (see `.goreleaser.yml`); the default dev label is `0.1.0-dev`.

## Repository layout

```
cmd/vh-agent-harness/    main entrypoint
core_manifest.go         core-corpus ownership classification (embed walk)
corpus.go                go:embed roots: templates/{core,overlays}, plus embedded
                         helpers (examples, overlay-skeleton, migrations)
internal/                substrate seam, ownership, schema, lineage, runshape,
                         runtime, hooks, overlay, proposals, drift, permission, cli
templates/core/          canonical domain-free corpus (rendered into projects)
templates/overlays/      shipped embedded overlay packs (opt-in capability packs;
                          e.g. `release` — the tag-driven release subagent). Projects
                          ship their own under <project>/.vh-agent-harness/overlays/
templates/migrations/    per-release migration notes (binary/help-surface only;
                         embedded, NOT rendered into consumer repos — one per release)
docs/coordination/       coordination templates + report schemas (rendered into projects)
docs/adoption-examples/  non-shipped adoption reference (web/)
```
