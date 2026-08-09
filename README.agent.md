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

`vh-agent-harness <cmd>` is a binary subcommand (runs anywhere, in any shell);
`/<cmd>` is an agent slash-command (only inside an OpenCode session).

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
   the full per-file plan (overwrite / unchanged / seed / preserve /
   managed-diverged / reconcile / conflict) and write nothing. Read it before
   the real run.
2. **Do not hand-edit a `platform_managed` file unless you mean to keep the
   edit.** `update` overwrites a `platform_managed` file **only when it is still
   the bytes the platform last wrote** (the recorded origin hash). If you have
   edited it, `update` **preserves your edit** (`managed-diverged` — ownership
   has effectively transferred to you) and will not clobber it. To re-baseline
   (accept the platform's version again): remove the file's entry from
   `.vh-agent-harness/origin-hashes.json` (or delete that store to re-bootstrap
   all) THEN run `update` — deleting the file alone will NOT work (a deleted
   managed file is also preserved as consumer-deleted). To make an edit
   canonical across updates, promote it into an overlay pack source. To change
   managed behavior without editing the file, use a seam: overlay, profile,
   run-shape, mission, project deny-rules, or a raise-only ownership override.
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
| `.vh-agent-harness/project.config.json` | Fill `project.mission_summary` + `architecture_summary` (and `db_user`/`db_name` if used). Resolved into the seeded `CLAUDE.md`/`Makefile` at install — **create + fill it BEFORE `install`** (those seeds are written once). A field that does NOT apply (e.g. `db_user`/`db_name` when there is no database) may be set to a blessed N/A sentinel — `none` / `n/a` / `null` / `na` (case-insensitive; string form only — write `"null"`, not bare JSON `null`): it renders empty and silences the `token(s) UNRESOLVED` warning for that field. |
| `.vh-agent-harness/AGENTS.mission.md` | Write the project's domain mission/architecture/rules; composed into root `AGENTS.md` on `update`. |
| `.vh-agent-harness/vh-harness-profile.yml` | (armed, seeded) Select features + `overlays: [<pack>]` (S3). |
| `.vh-agent-harness/run-shape.yml` | (seeded host-shell) Set runtime `backend:` (`host-shell`/`docker_compose`/`proxy`) + `compose_file`/`default_service` or `proxy_command`; lifecycle hooks/verbs (S4). |
| `.vh-agent-harness/harness-ownership.yml` | (optional; not seeded) Raise-only ownership overrides — create only to take a managed file to `project_owned`. |
| `.vh-agent-harness/product-prefixes.json` | (optional; not seeded) Declare the product-code surface (directory prefixes) for the coordination cross-boundary-slice hint. Absent or malformed → monorepo default `apps/` + `packages/`; create only for a non-monorepo layout (e.g. `src/`). Read at runtime by `coordination-hints-lib.js` (not the render seam). |
| `.vh-agent-harness/overlays/<pack>/` | Project overlay: `agents/`, `commands/`, `skills/`, `opencode-append.jsonc`, `permission-pack.jsonc`, `callable-graph-snippet.md`. |
| `.opencode/repo-configs/forbidden-patterns.project.js` | (seeded blank) Project deny-rules (import builders from `forbidden-patterns.core.js`; each rule needs a `why`). |
| `.vh-agent-harness/config-transform.mjs` | (seeded blank) Project permission transform (F-intent). Returns typed `permissionPatches` merged into every render. NO raw config mutation. See "Permission transform (F-intent)" below. |
| `.vh-agent-harness/config-transform.core.mjs` | (regenerated) Harness-owned types + Decision constants + builder helpers for the transform. Import, do not edit. |
| `.opencode/plugins/compaction-primitives.project.md` | Project compaction-recovery block (operational primitives an agent needs after context loss). |
| `docs/coordination/LANES.yaml`, `docs/coordination/ROLES.md` | Coordination lanes/roles — define project-specific ones or keep the generic set. |
| `.local/cleared-assumptions.yaml` | Operator-state ledger of cleared assumptions (usually operator-maintained). |

Do **not** edit: `lineage.yml` (binary-owned), `origin-hashes.json` (binary-
owned: records per-file origin hashes for the three-way update sync), `AGENTS.core.md`
(managed compose source), or anything under `.opencode/` that is `platform_managed`
— unless you intend to keep the edit (see golden rule #2: a hand-edited managed
file is preserved, not overwritten, and `origin-hashes.json` is how the platform
remembers your edit is yours).

### `vh-harness-profile.yml` field contract

The four selection fields and what they mean (implemented in
`internal/cli/profile.go`):

| Field | What it does |
| --- | --- |
| `profile:` | Enum preset. `minimal` / `coordination` / `web` → baseline-only (the 8 universal agents); `supervised` → baseline + `core/gated-commit` + `core/debate` (the gated-commit and debate clusters). Unknown enum value → baseline-only (safe default). |
| `capabilities:` | Explicit opt-in. A list of capability IDs (e.g. `core/release`) **unions onto** the `profile:` preset — it adds, never replaces. So `profile: minimal` + `capabilities: [core/debate]` = baseline + debate. |
| `overlays:` | Expert-override pack selection (e.g. `overlays: [release]`). Renders the named pack(s) directly; capability IDs implied by a listed pack's manifest are also folded into the resolver selection, so the two paths converge. |
| `modules:` | **Deprecated.** A non-empty `modules:` list emits a one-line warning on every render (update/doctor/inventory) nudging migration to `profile:` + `capabilities:`. Still parsed for backward compat (existing profiles keep working); the values carry no effect. |

`profile:` is the normal knob. Reach for `capabilities:` when you want one extra
cluster without switching preset; reach for `overlays:` only as an expert
override to force-render a pack regardless of capability resolution.

### Opt-in core capabilities

The catalog ships four capabilities today. Two are pulled in by the
`supervised` preset (`core/gated-commit`, `core/debate`); the other two are
opt-in only:

| Capability ID | Provides | In preset? | What it does |
| --- | --- | --- | --- |
| `core/gated-commit` | `commit-message`, `commit-reviewer`, `commit-reviewer-a..d`, `committer` | `supervised` | The gated-commit protocol (commit-message drafting, tiered cascade review, committer-exclusive git mutations). |
| `core/debate` | `debate`, `debate-proposer`, `debate-critic`, `debate-synth`, `solution-brief` | `supervised` | The multi-model debate pipeline plus the solution-brief wrapper. |
| `core/media-perception` | `media-perception` (agent + caller-facing skill) | none (opt-in) | A single read-only perception specialist that inspects media (image, diagram, chart, video, document/PDF, audio). For local media, callers pass BOTH `@file <path>` (bytes) and `path:` (locator); for remote media, `url:`. Parent-session attachments do NOT auto-propagate to a task child. |
| `core/worker-read-only` | `worker-read-only` (dynamic worker leaf) | none (opt-in) | A prompt-scoped read-only worker leaf for bounded repository inspection, path tracing, evidence extraction, and state observation when the dispatch itself completely defines the scope and no durable specialist process is required. `delegateFrom` is `build`, `coordination`, `project-coordinator` (researcher deliberately excluded — it is not a perception specialist, and a researcher already holds the read-only inspection surface itself). Read-only boundary: deny `edit`/`task`/`webfetch`/`skill`, deny-all task (no outbound delegation), candidate-only return; carries NO `exec-sandbox` grant, so it is deliberately narrower than `researcher`/`repo-explorer`/`media-perception` (cannot run arbitrary code). It DOES carry `defer-triggers` — the no-arg contained wrapper that runs ONLY the canonical DEFER-trigger predicate checker (no caller-controlled exe/script/mode), which fits its trigger-currency inspection charter without promoting it to Level B. |

Select `core/media-perception` by adding it to `capabilities:`:

```yaml
profile: minimal         # or supervised
capabilities:
  - core/media-perception
```

Select `core/worker-read-only` by adding it to `capabilities:`:

```yaml
profile: minimal         # or supervised
capabilities:
  - core/worker-read-only
```

When unselected, the worker-read-only agent block is absent from
`opencode.jsonc` and the three inbound caller edges (`build`, `coordination`,
`project-coordinator` → `worker-read-only`) are dropped by the permission
emitter's present-agent filter — so an unselected capability leaves zero
dangling edges. (Unlike `media-perception`, there is no caller-prompt
conditional block: the routing decision between a named specialist and the
worker is a process-vs-scope judgment the caller already carries, not a
gated skill load.)

When unselected, the agent block is absent from `opencode.jsonc` and the four
inbound caller edges (`build`, `coordination`, `project-coordinator`,
`researcher` → `media-perception`) are dropped by the permission emitter's
present-agent filter — so an unselected capability leaves zero dangling
edges. Additionally, each caller's prompt carries a conditional block (the
source `*.md.tmpl` resolves `{{ if .capabilities.media_perception }}` at
render time): when the capability is NOT selected, the callers' DISABLED
branch instructs them to NOT load the skill, NOT delegate or probe through
trial task calls, and to state honestly that media understanding is
unavailable in the current configuration (asking the operator to enable the
capability or provide an accessible `path:`/`url:` they can perceive
directly). This prevents the failure mode where a caller holds a screenshot,
self-refuses "I can't read the image," and does not know the specialist
exists.

#### Media-perception model seed

Like every agent, `media-perception` resolves its model via
`{file:./.local/config/agent-model/media-perception}`. The seed mechanism
(`seedAgentModelDefaults` in `internal/cli/seam.go`) creates that file EMPTY
on the first `vh-agent-harness update` after the capability is selected, so
OpenCode's config load never breaks on a missing ref. The operator then
writes the chosen model id into the file:

```
.local/config/agent-model/media-perception
```

While empty, `doctor` reports a WARN (`config-refs` check) — agents fall back
to OpenCode's default model until a real id is set.

#### Media-perception: missing-tool vs broken-model-reference

Two distinct absence modes have different surfaces:

- **No compatible perception capability in the session** — the agent runs and
  returns `capability_status: unavailable` in its consolidated report. This
  is the agent-level path; the harness does not intercept it. Callers should
  expect `unavailable` is possible and surface it honestly (do not fabricate
  observations).
- **Broken or missing `.local/config/agent-model/media-perception` reference**
  — this is a pre-agent config-layer failure. `doctor` FAILs the
  `config-refs` check (`N {file:} ref(s) point to missing files … run
  \`vh-agent-harness update\``) when the file is missing, and WARNs when it
  exists but is empty. Running `vh-agent-harness update` re-seeds the file.

The agent CANNOT catch a pre-invocation config failure (OpenCode may reject
the config before the agent runs), so the second mode is owned by the harness
seed + doctor diagnostics, not by the agent prompt.

#### Media-perception: attachment propagation and dual-channel handoff

Parent-session attachments do NOT automatically propagate into a task child's
context. OpenCode's `task` tool creates the child from `params.prompt` only —
any image, screenshot, or file attached to the parent session stays in the
parent. This is the root cause of the vh-solara failure: a build agent
received a screenshot as a parent attachment, self-refused ("I can't read the
image"), and did not know the specialist existed or how to hand the bytes
forward.

The fix has two parts:

1. **Caller prompt gating** — each of the four callers (`build`,
   `coordination`, `project-coordinator`, `researcher`) now carries a
   conditional block in its `*.md.tmpl` source. When the capability is
   selected, the ENABLED branch tells the caller to load the skill and make
   ONE bounded delegation. When unselected, the DISABLED branch tells the
   caller to honestly state media understanding is unavailable. Either way,
   the caller KNOWS whether the specialist exists.

2. **Dual-channel handoff** — for local media, the caller MUST pass BOTH:
   - `@file <path>` — so the specialist receives the bytes (opencode attaches
     the file content to the child's prompt)
   - `path: <repo-relative path>` — so the specialist has an explicit locator
     it can hand to its perception capability

   For remote media, pass `url: <accessible URL>` (no `@file` needed — the
   capability fetches the URL itself). If only a parent attachment is
   available without a locator, the caller must request an accessible path or
   URL rather than inventing one. The specialist itself classifies failures
   into structured classes (`missing_locator`, `inaccessible_local`,
   `inaccessible_remote`, `unavailable_capability`, `timeout`, etc.) and
   maps them into its report's `limitations` and `next_action` fields.

#### Media-perception integration recipe (overlay / operator)

To wire a perception capability into a consuming project:

1. **Select the capability** in `.vh-agent-harness/vh-harness-profile.yml`
   (`capabilities: [core/media-perception]`).
2. **Populate the model file** `.local/config/agent-model/media-perception`
   with the chosen opencode-managed provider model id (operator step; the
   file is gitignored).
3. **Expose a compatible perception capability** via overlay or operator
   config (e.g. an MCP server, a project-local tool) that the runtime makes
   discoverable to agent sessions. The core capability is provider-neutral;
   the actual tooling lives in the project, not in core.
4. **Run validation**: `vh-agent-harness update` (renders the agent + skill +
   4 caller edges); `vh-agent-harness doctor` (confirms no FAIL, only the
   empty-model WARN until a model id is set).

#### Media-perception no-refusal acceptance procedure

The behavioral signal that the prompt works: given a perception task in a
session with a compatible capability exposed, `media-perception` MUST
inspect/invoke the capability rather than refuse with “I have no vision.”
Refusal = prompt failure = revise.

To verify live:

1. Ensure a perception-capable tool is exposed to the session (overlay or
   operator config).
2. Delegate a perception task to `media-perception` with `@file <an
   accessible image/diagram/chart path>` AND `path: <the same path>` (the
   dual-channel handoff — bytes + locator), plus a real question.
3. Confirm the agent returns `capability_status: available` with grounded
   observations, NOT a refusal and NOT `unavailable` (when a capability was
   in fact available).

If no perception backend is available in the build environment, this
acceptance item is **UNVERIFIED (pending operator-backed validation)** — do
not claim it passed from prompt inspection alone.

### Shipped overlay packs

Besides project packs you author under `.vh-agent-harness/overlays/`, six
overlay packs ship **embedded in the binary**, selectable by name with no
vendoring. Run `vh-agent-harness overlay list` to see every pack (embedded +
project-local) with its source and selected/available status — the discovery
surface that prevents a shipped-but-unselected pack from being treated as
nonexistent:

- `release` — the tag-driven `releaser` workflow (the first embedded pack). It
  is selected either way and the two paths converge:
  - `capabilities: [core/release]` — the explicit capability opt-in, OR
  - `overlays: [release]` — the expert-override pack selection.

  Selecting `core/release` also pulls the `core/gated-commit` cluster in via
  hard-dep closure (the releaser delegates to the gated-commit agents), so both
  selection paths render the same cluster.

- `auto-classifier-pilot` — the opt-in auto-classifier safety pilot (a
  three-hook tool-call gate with `audit`/`enforce`/`live`/`live-tiered` modes).
  It is **overlay-only** (no capability-manifest), so it is selected solely via
  `overlays: [auto-classifier-pilot]`. See "Auto-classifier configuration"
  below, or run `vh-agent-harness overlay docs auto-classifier-pilot` for the
  full reference.

- `repo-mail` — the opt-in repo-mail inter-repo communication egress gate. The
  pack provides ONLY the integration wiring (`repo-mail-egress-wiring.js`),
  which binds the generic domain-free fail-closed egress gate
  (`repo-mail-egress-gate.js`, shipped in **core** as the repo-mail protocol's
  Contract C) to the shared `scrubCredentials` helper from `auto-classifier-pilot`
  via dependency injection. It is **overlay-only** (no capability-manifest), so
  it is selected solely via `overlays: [repo-mail]`, and it depends on
  `auto-classifier-pilot` for the shared `scrubCredentials` helper.

- `formal-verification-pilot` — a skills-only pilot for agent-authored formal
  proofs (Lean4-core/TLAPS). It ships a SKILL plus references and is strictly
  **INFORMS-only**: it wires no agent, no command, no permission surface, and no
  gate. It ships **default-on** for every consumer; disable it with
  `features: formal-verification-pilot: false`. An explicit
  `overlays: [formal-verification-pilot]` entry re-adds it even when opted out
  (the feature flag disables only the default, not a forced re-selection).

- `resolve-first-pilot` — a skills-only pilot for DEFER/follow-up triage at
  card-creation. It ships an instruction-only SKILL plus a runbook and is
  strictly **INFORMS-only**. It ships **default-on** for every consumer;
  disable it with `features: resolve-first-pilot: false`. An explicit
  `overlays: [resolve-first-pilot]` entry re-adds it even when opted out.

- `contract-invariant-audit-pilot` — a skills-only pilot for proactive
  contract/invariant auditing across repository history. It ships a SKILL, a
  runbook, and a deterministic discovery/accounting helper that runs under the
  existing broad `vh-agent-harness *` allow. It is strictly **INFORMS-only**.
  It ships **default-on** for every consumer; disable it with
  `features: contract-invariant-audit-pilot: false`. An explicit
  `overlays: [contract-invariant-audit-pilot]` entry re-adds it even when
  opted out.

Each renders into `.opencode/` on `update` exactly like a project-local pack.
The three pilots (`formal-verification-pilot`, `resolve-first-pilot`,
`contract-invariant-audit-pilot`) are **default-on**: a `minimal` profile that
never names them still renders them because their platform-default feature
keys are `true`. Disable any pilot by setting its feature key to `false`. The
other three (`release`, `auto-classifier-pilot`, `repo-mail`) remain opt-in
(a `minimal` profile that never names them renders nothing of them). A
project-local pack of the same name still shadows the embed wholly.

### Auto-classifier configuration

The `auto-classifier-pilot` overlay renders 5 plugins (the `auto-tool-gate`
hook set). Its config lives in TWO files under `.opencode/repo-configs/`.

| File | Purpose | Committed? |
|------|---------|------------|
| `auto-gate-config.json` | Behavior: mode, reply disposition, prompt composition | Adopter's choice |
| `auto-gate-llm.json` | LLM endpoint, model, API key | NEVER (secrets-adjacent) |

Behavior config (`auto-gate-config.json`) — all 8 fields:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `true` | Master switch. |
| `mode` | enum | `"audit"` | `audit`=observe; `enforce`=stub; `live`=real single-model; `live-tiered`=multi-leaf consensus. |
| `stubVerdict` | enum | `"block"` | Drives enforce-mode stub. Test only. |
| `promptFile` | string | `""` | Full-override escape hatch for classifier system prompt. |
| `replyMode` | enum | `"once"` | `once`=approve this call; `always`=persist to in-memory allowlist (powerful). |
| `onUncertain` | enum | `"reject"` | `reject`=fail-closed; `passthrough`=hang risk in headless. |
| `harnessContext` | bool | `true` | Include harness-context fragment in composed prompt. |
| `guides` | bool | `true` | Include adopter guide files in composed prompt. |

Mode → LLM requirements:

| Mode | LLM config | Fields needed |
|------|-----------|---------------|
| `audit` | No | — |
| `enforce` | No | — |
| `live` | Yes (top-level) | `model` + `modelEndpoint`/`modelEndpointEnv` (default `AUTO_GATE_MODEL_ENDPOINT`) + `apiKey`/`apiKeyEnv` (default `AUTO_GATE_API_KEY`) |
| `live-tiered` | Yes (`leaves[]`) | ≥1 leaf with `model` + endpoint + key (endpoint/key env-var names default to `AUTO_GATE_MODEL_ENDPOINT`/`AUTO_GATE_API_KEY` per leaf) |

The `modelEndpointEnv` / `apiKeyEnv` env-var NAME fields default to
`AUTO_GATE_MODEL_ENDPOINT` / `AUTO_GATE_API_KEY` (from `DEFAULT_LLM_CONFIG`) when
omitted, so an adopter can supply just `model` and set those env vars at runtime.
`vh-agent-harness doctor` cross-checks config SHAPE (field names non-empty after
normalization) — it cannot verify the env vars are actually SET, so a config that
passes doctor may still fail-close at runtime if an env var is unset.

Enablement steps:

1. Ensure `auto-classifier-pilot` is listed under `overlays:` in
   `.vh-agent-harness/vh-harness-profile.yml`, then `vh-agent-harness update`.
2. Create `.opencode/repo-configs/auto-gate-config.json` with your `mode`.
3. For `live`/`live-tiered`, create `.opencode/repo-configs/auto-gate-llm.json`
   with at least `model` (the `*Env` endpoint/key names default to
   `AUTO_GATE_MODEL_ENDPOINT`/`AUTO_GATE_API_KEY`; set explicit `*Env` or literal
   values to override). Set the named env vars at runtime.
4. Run `vh-agent-harness doctor` to verify config health (it cross-checks mode
   against the LLM config shape).
5. Restart OpenCode so the plugins load.

For the complete reference (all modes, fail-closed behavior, prompt
composition, per-call gate flow), run
`vh-agent-harness overlay docs auto-classifier-pilot`.

#### CI gate & credential hygiene (never-commit paths)

The auto-gate config surface has two never-commit file classes the seed
`.gitignore` ignores: the secrets-adjacent LLM file (`auto-gate-llm.json`, which
may hold a literal `apiKey`) and the per-developer local-companion convention
(`.opencode/repo-configs/*.local.json`). `.gitignore` is `project_owned` (seeded
on greenfield, preserved on update), so an adopter that installed before the seed
fix — or hand-edited `.gitignore` — can silently commit one of these.

The `auto-gate-ignore` doctor check detects that state (see Diagnostics &
verification above), but two residuals the harness CANNOT autonomously close
remain, and are stated honestly here:

- **CI gate (required for overlay users).** Run `vh-agent-harness doctor` as a
  **pre-stage / pre-package CI gate** so a tracked or un-ignored never-commit
  auto-gate file FAILs the pipeline before it lands. External CI artifact
  collectors may ignore `.gitignore` entirely (e.g. publish the whole work tree),
  so `.gitignore` alone is not a sufficient guard — the doctor gate is.
- **Disclosed credentials cannot be un-leaked.** If a literal `apiKey` was ever
  committed, tracked, or collected, adding a `.gitignore` rule does NOT revoke or
  erase prior exposure. Rotate/revoke the credential at the provider, then
  consider history rewrite if the key reached a shared ref. This is an
  owner-driven incident action, not something the harness performs.
- **No reconciliation command.** There is intentionally no command that auto-edits
  your `.gitignore`. If the `auto-gate-ignore` check reports a missing rule, the
  owner-authorized path is a manual edit (or `git rm --cached <path>` for a tracked
  file) — a silent rewrite of an operator-owned file would violate the ownership
  contract. The two seed lines are documented in the overlay README for paste.

#### `.local.json` repo-config override convention

Layered config consumers under `.opencode/repo-configs/` follow a shared
override convention: a committed `X.json` is the optionally-committed **base**
(per family policy), and a gitignored `X.local.json` is an OPTIONAL
**field-override** companion. The local file is never committed (the scoped
wildcard `.opencode/repo-configs/*.local.json` is gitignored) and is absent by
default — its absence reproduces the legacy single-file behavior with zero
change. When present, local fields **shallowly overwrite** matching base fields
(a present key with a falsy value like `false` or `""` is a real override, not
absence); fields the local file omits inherit from the lower layers. The local
file is the **final project layer** (project-local > committed-project > user >
defaults). The secrets-adjacent files (`*-llm.json`) are always gitignored by
their own bare-name rule and are unrelated to this convention wildcard.

Auto-gate is the **first consumer** of this convention: each committed behavior
base (`auto-gate-config.json`) / secrets LLM file (`auto-gate-llm.json`) may
have an optional `auto-gate-config.local.json` / `auto-gate-llm.local.json`
override — e.g. a developer can flip just `{"mode":"live-tiered"}` locally
without touching the committed base. `vh-agent-harness doctor` lints the local
file when present (absent = valid/silent; present-but-invalid = FAIL identifying
the local layer) and includes it in effective-value resolution.

## Permission transform (F-intent)

The canonical permission emitter (`permconfig.Emit`) is the SOLE writer of
`opencode.jsonc` — permission-packs are its input, never a post-render patch.
Every agent's `permission.bash` block is regenerated from a `location`
descriptor that has a fixed set of slots (`wildcard`, `readonly`,
`git_readonly`, `gate`, `harness`, `edit`). There is **no slot for arbitrary
project bash patterns** like `"./dev.sh *": "allow"`.

The **permission transform** closes that gap as **F-intent**: the project
maintains a JS function that returns *typed permission intent*; the Go
pipeline validates it and feeds it to the same canonical emitter. The
transform NEVER directly mutates `opencode.jsonc`.

| File | Ownership | Role |
| --- | --- | --- |
| `.vh-agent-harness/config-transform.mjs` | `project_owned` (seeded blank, preserved) | Your transform function. Edit this. |
| `.vh-agent-harness/config-transform.core.mjs` | `platform_managed` (regenerated) | JSDoc typedefs, `Decision` constants, `allow`/`deny`/`ask` builders. Import, do not edit. |

### Contract

```
INPUT  = { context: { packs: string[], features: {k: string}, agents: string[] } }
OUTPUT = { permissionPatches: [{ agent: string, bash: [{ pattern: string, decision: "allow"|"deny"|"ask" }] }] }
```

- `agent` must be in `context.agents` (the rendered roster: core + active-pack).
  Unknown agent → fail-closed render error.
- `pattern` must be non-empty and must NOT collide with a protected key
  (`"*"`, command-group commands, `"vh-agent-harness *"`, backlog command).
  Collision → fail-closed.
- Duplicate pattern for the same agent → fail-closed.
- Empty/absent `permissionPatches` → no-op (byte-identical to no-transform).

### Trusted-code execution

The transform is **trusted project-owned code** — the same trust model as
`forbidden-patterns.project.js`. If you can edit the transform file, you already
have commit authority on the repo, so the transform has the same authority as
any project-owned source file. It is **not sandboxed**.

The harness applies an **advisory source lint** that rejects obvious host-API
usage (`process.env`, `require()`, `fs.*`, `http(s).request`, `child_process`,
`Math.random`, `Date.now`, …) as defense-in-depth (comment-aware — documenting
these APIs in comments is fine). This lint is **NOT a security boundary** — it
is trivially evaded via string concatenation, dynamic imports, etc. The **real
security boundary** is Go validation of the typed output
(`ValidateTransformOutput`), which runs AFTER the transform returns and rejects
any malformed, invalid, or non-JSON output **LOUD** (never silent). A hard 10s
timeout kills hung transforms. No ambient env, no secrets, no file paths, no
`process` state reach the function — the context is a deterministic snapshot of
the render.

### Emission ordering

OpenCode permission matching is **last-match-wins** (`findLast` over the
flattened ruleset). Extra transform entries are emitted AFTER the leading
`"*"` wildcard and AFTER the sorted command-group region, BEFORE the
`"vh-agent-harness *"` entry — sorted length-then-locale among themselves for
determinism. This means an extra `allow` beats the leading `*:deny` (it comes
later), and does not interfere with the `"vh-agent-harness *"` boundary
(project patterns do not match `vh-agent-harness ...` commands).

### Harness policy (4-state)

Each core role carries a `HarnessPolicy` that selects how the
`"vh-agent-harness *"` wildcard entry is emitted in Region 4 of the bash
block:

| Policy | Region 4 emission | Used by |
|---|---|---|
| `allow` | `"vh-agent-harness *": "allow"` (broad) | `build`, `coordination`, `project-coordinator` |
| `ask` | `"vh-agent-harness *": "ask"` (broad) | _(none in CoreLocationRules — `plan` migrated to `read_only`)_ |
| `deny` | `"vh-agent-harness *": "deny"` (broad) | `committer` (keeps its gated command surface; uses `commit-gate.sh` directly, needs no harness exec surface) |
| `read_only` | `"vh-agent-harness *": "deny"` **FIRST**, then a canonical set of safe read-only verbs as `"allow"` **AFTER** | see below (18 core agents + 1 overlay-managed) |

The `read_only` roster is **18 core agents** (in `CoreLocationRules`) plus
**1 overlay-managed**:

- **9 original RO specialists:** `researcher`, `planner`, `media-perception`, `repo-explorer`, `debate`, `debate-proposer`, `debate-critic`, `debate-synth`, `solution-brief`.
- **8 read-only service roles (F7-residue migration):** `commit-message`, `commit-reviewer`, `commit-reviewer-a`, `commit-reviewer-b`, `commit-reviewer-c`, `commit-reviewer-d`, `ship-review`, `docs-steward`.
- **1 read-only primary-mode planner:** `plan` (migrated from `ask`).
- **1 overlay-managed service role (not in `CoreLocationRules`):** `harness-release-readiness`.

All 18 core roles are asserted in `CoreLocationRules`
(`internal/permconfig/tables.go`); `harness-release-readiness` is
overlay-managed (the `.vh-agent-harness` release pack renders its block and
emits the same read_only surface). These roles draft text, review diffs,
audit changes, or edit via the Edit tool — none needs a mutating `vh-agent-harness exec` surface, so `read_only` is the correct least-privilege policy.

The `read_only` policy is **deny-first + canonical-exceptions-after**, which is
the correct shape under `findLast`: the trailing `allow` entries win over the
leading `deny`. A non-canonical harness verb (e.g. a future `vh-agent-harness
new-command` or any mutation verb) falls through to the `deny` because nothing
after it matches.

The canonical read-only verb inventory is **Go-owned** (defined once in
`internal/permconfig/tables.go` as `HarnessReadOnlyCommands`) and is **not**
copied per agent — every `read_only` role emits the same list. It currently
allows the safe read surface: `exec-ro *`, `guide`, `doctor`, `preflight`,
`diff`, `status`, `proposals`, `version`, `docs`, `example`, `sys-prompt`,
`help`, `--help`/`-h`, `skill list`, and `overlay docs *` (each with and
without arguments).

Withheld pending audit (still denied): `skill validate`, `logs`, `ps`.
Excluded entirely from the roster-wide read-only list: all mutation verbs
(`exec`, `shell`, `up`, `down`, `install`, `update`, `uninstall`,
`self-update`, `overlay new`), artifact producers (`diagnostics-export`), and
broad wildcards (`skill *`, `overlay *`).

`exec-sandbox` is a special case: it is NOT in the roster-wide
`HarnessReadOnlyCommands` (it runs arbitrary code), but it is granted
PER-AGENT via `ReadOnlyExtraAllows` to the read-only specialists that
genuinely need to run read-code: `researcher`, `repo-explorer`, and
`media-perception`. The grant is kernel-contained by the strict mode-floor
(`exec_sandbox.min_mode: strict`): the executed code cannot write outside
`./tmp/` or make network connections, so the agent keeps its read-only
character. See the Read-Only Execution Policy
(`.opencode/docs/agents/read-only-execution-policy.md`) for the Level B
mechanism and the paired floor+grant invariant.

`defer-triggers` is a second special case: it is the no-arg contained wrapper
that runs ONLY the canonical DEFER-trigger predicate checker
(`.opencode/scripts/check-defer-triggers.mjs`) under ModeStrict + NetDeny +
DefaultProfile — NO caller-controlled exe/script/mode/net (there are no
flags). It is granted PER-AGENT via `ReadOnlyExtraAllows` to `researcher` and
`worker-read-only` (the agents with a trigger-currency inspection charter).
Unlike `exec-sandbox`, it does NOT run arbitrary code, so it does NOT promote
an agent to Level B — `worker-read-only` receives `defer-triggers` but still
carries NO `exec-sandbox` grant. `coordination` is covered implicitly by its
broad allow wildcard (it is non-read_only, so it cannot carry the literal).
`repo-explorer` / `media-perception` keep `exec-sandbox` but deliberately do
NOT get `defer-triggers` (no trigger-currency charter). The checker writes its
git-capture scratch to `<repo>/tmp` (the sole RWDir) and uses a file-backed
stdout descriptor (not a pipe) so libuv does not allocate the
`socketpair(AF_UNIX)` that `NetDeny` blocks — this is what makes the checker
functional under the strict sandbox.

**Family read-only admission rule.** Each admitted verb appears as both the
scalar (`vh-agent-harness doctor`) and the wildcard (`vh-agent-harness
doctor *`) because its entire family is currently read-only — inspection flags,
no mutating subcommands. This is distinct from the `skill *` / `overlay *`
exclusion: those families already carry mutating verbs (`overlay new`; future
skill verbs), so a family-wide wildcard would leak a mutation to read-only
specialists. **Fail-open caveat:** while the wildcard form is used, the matrix
does not deny a *future* subcommand of an admitted verb — a future mutating
`doctor <subcommand>` (e.g. a repair/write/network/secret-sensitive path) would
inherit the allow unless the family is re-audited. Admission of `verb *`
carries a standing re-audit obligation; narrowing the wildcard while no
mutator exists is hardening (parked behind a named trigger), not a defect fix.

All canonical read-only verbs are protected keys — a config transform cannot
collide with or replace them, and a non-canonical harness pattern injected by a
transform lands BEFORE the Region 4 `deny`, so it is inert under `findLast`.

**Legacy compatibility:** the deprecated `devSh` and `harness` config keys
still accept the scalar values `allow`/`ask`/`deny` and normalize to the
matching `HarnessPolicy`. The new `read_only` value may be expressed through
any of `harnessPolicy`, `harness`, or `devSh`. Conflicting declarations across
these keys fail closed.

### Dry-run / failure behavior

- `update --dry-run` runs the transform and shows the changed config; writes
  nothing.
- `doctor` runs the same render pipeline, so a broken transform surfaces as
  drift/FAIL identically to `update`.
- If the transform file is absent, the render is byte-identical to the
  no-transform path (regression guard).

### Types-import pattern

```js
// .vh-agent-harness/config-transform.mjs
import { Decision, allow, deny, ask } from "./config-transform.core.mjs";

export default function transform({ context }) {
  return {
    permissionPatches: [
      { agent: "build", bash: [allow("./dev.sh *")] },
    ],
  };
}
```

> **SECURITY NOTE:** The transform CAN alter core-agent permissions (including
> the build agent), because it is trusted project-owned code (not sandboxed).
> Review every project transform as a **security policy**: a compromised
> transform could grant arbitrary bash access to any rendered agent. The Go
> validator (`ValidateTransformOutput`) enforces the output shape and rejects
> protected-key collisions, but the intent (which patterns to allow) is the
> project's responsibility. The advisory lint catches only obvious host-API
> misuse — it is not the security boundary.

## Common tasks

### Setup & configuration

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

### Execution

- **Run a command in the runtime:** `vh-agent-harness exec -- <cmd>` (the `--` is
  optional — the command's own flags pass through, e.g. `exec bash -c '…'`,
  `exec pytest -k x`; put any harness flags BEFORE the command). Mutating
  commands are allowed; only forbidden-patterns and the commit-gate are blocked.
  Put env vars / `timeout` INSIDE the command (`exec bash -c 'FOO=1 cmd'`), never
  as a host prefix.
  - **Exit code + output honesty (exec family):** `exec`/`exec-ro`/`shell`
    propagate the CHILD's real exit code — `exec bash -c 'exit 3'` exits 3, not
    1 (the child's code is the honest signal; scripts should assert the real
    code, never `==1`-on-any-failure). `exec-sandbox` always did (it bypasses
    cobra via `os.Exit(code)`). On a child failure the exec family prints the
    `Error:` line and does NOT dump the Usage/Flags block (a child failure is
    not a usage error). Genuine non-child errors (permission denials, hook
    failures, the silent `errSilent` sentinel used by diff/doctor) still exit 1.
    (v0.23.0; see `templates/migrations/v0.23.0.md`.)
  - **Git mutations are denied at both layers.** `vh-agent-harness exec git
    <mutation>` — with OR without a leading global flag (`exec git --no-pager
    commit`, `exec git -C /x push`, `exec git --git-dir=/x commit`, `exec git
    commit`) — is denied by the Go binary backstop (before the JS gate runs) AND
    by the shell-guard JS gate, and must route through the commit-gate /
    committer agent. Read-only git through exec (`exec git --no-pager status`,
    `exec git -C /repo log`) and all non-git exec mutations (`exec mkdir`,
    `exec pytest`, `exec npm test`, `exec bash -c '...'`) are unaffected — the
    guard is git-mutation-scoped only and does not default-deny. Nested-shell
    git (`exec bash -c 'git …'`) is governed by the existing forbidden-pattern
    chain-guard scan, not this guard. (F1 fix, v0.5.0; see
    `templates/migrations/v0.5.0.md`.)
- **Use an existing wrapper for execution:** in `run-shape.yml` set
  `backend: proxy` and `proxy_command: ["./dev.sh", "exec"]`.
- **Run a command as read-only intent, classified host-side (no prompt):** `vh-agent-harness exec-ro -- <cmd>`.
  exec-ro is a HOST-SIDE INTENT CLASSIFIER that runs BEFORE backend dispatch: it
  classifies the command against the host repo path, then delegates to the
  selected runtime backend. It is NOT proof that the backend payload is
  OS-sandboxed or on read-only mounts — under proxy/docker_compose the classified
  command runs in-container against the container's filesystem view. This is a
  general read-only execution gate enforced INSIDE the Go binary (a separate,
  narrower gate than `exec`). It is allowlisted in `opencode.jsonc` as
  `vh-agent-harness exec-ro *: allow` for every agent, so opencode NEVER prompts
  for it — which means **exec-ro itself is the only gate** and hard-DENIES
  anything dangerous. Classifier = curated allowlist + default-deny:
  - **Non-git:** the binary must be a known read-only inspection tool from the
    `readonly` command group (`ls`, `cat`, `jq`, `grep`, `rg`, `wc`, `head`,
    `tail`, `find`, …). Anything else (`npm install`, `rm foo`, `curl`,
    `python script.py`) → DENY. A matching binary ALSO cannot carry a known
    write/exec-capable flag — exec-ro denies find's `-delete`/`-exec`/`-execdir`/
    `-ok`/`-okdir`/`-fls`/`-fprint`/`-fprint0`/`-fprintf` (delete files, run a
    program, or write the listing to a file), sort's `-o`/`--output[=file]`
    (writes sorted output to a file), and requires `sed` to carry `--sandbox`
    (which disables its `e`/`r`/`w` commands) AND independently denies `sed`'s
    `-i`/`--in-place` (which `--sandbox` does NOT disable) — so under exec-ro
    the only ALLOWED sed form is `sed -n --sandbox …`. **Accepted heuristic
    residual (F7):** the `-i` matcher only recognizes bare `-i`,
    `--in-place`/`--in-place=`, and `-i<suffix>` forms where the suffix starts
    with a NON-ALPHA character (`.`, digit, `_`, …). Alpha-leading suffixes
    like `-ibak` or `-iX` are NOT matched (they look like a hypothetical
    future `-iX` flag rather than `-i<backup-extension>`); a write-vector via
    `sed -n --sandbox -ibak f` is accepted as a residual because exec-ro is
    explicitly heuristic (exec-sandbox is authoritative) and the cost of
    false-tripping on a future `-iX` was judged higher than the cost of
    accepting this residual. See
    `TestClassify_SedInPlaceAlphaSuffixIsAcceptedResidual`. (Binary-level
    heuristic denylist of the known prompt-free non-git write/exec vectors; a
    per-binary safe-flag allowlist is deferred and the OS-level exec-sandbox is
    the authoritative layer for the long tail of unknown flags ON THE HOST-SHELL
    BACKEND — exec-sandbox is host-local-only and does not follow the payload
    into a proxy/docker_compose container. The `readonly`
    group entry itself is left as `find *` / `sort *` / `sed -n *` on purpose:
    it also feeds the shell-guard L2 permission.bash emission for ALL agents,
    and widening it would emit a broader prompt-free rule for every agent and
    reopen the vector at the L2 layer — the flag rules are exec-ro-internal.)
  - **Git:** the verb is isolated past global flags (a Go port of the
    shell-guard `walkGitGlobals` walker — same flag registry, same `-C`
    classification). The verb must be in `git_readonly`; mutation verbs
    (`commit`, `push`, `reset`, `rm`, …) and unknown verbs → DENY. A readonly
    verb ALSO cannot carry a known write/exec-capable SUBCOMMAND flag — exec-ro
    denies `--output`/`--output=<path>` (writes diff/show/log output to a file),
    `--ext-diff` (invokes the configured external diff driver), grep's
    `-O`/`--open-files-in-pager[=<pager>]` (runs a pager binary over the matching
    files), and the textconv/filter family `--textconv`/`--filters` (invoke
    configured diff/filter driver programs) across all readonly verbs
    (verb-level heuristic denylist of the known prompt-free write/exec vectors;
    a per-verb safe-flag allowlist is deferred and the OS-level exec-sandbox is
    the authoritative layer for the long tail of unknown flags ON THE HOST-SHELL
    BACKEND (exec-sandbox is host-local-only and does not follow the payload into
    a proxy/docker_compose container) — including diff/log textconv, which is
    default-on when configured via gitattributes and is therefore a residual the
    flag-level denylist cannot fully close).
  - **Shell metacharacters are refused** (conservative deny-on-unparseable): any
    of `|`, `;`, `&`, `$`, backtick, `>`, `<`, newline → DENY. exec-ro is a fast
    script-level heuristic (spoofable by complex shell), so it refuses pipelines
    / sequences / subshells / redirects outright. For those, run the BARE command
    via `exec` (which will prompt) — the deny notice says so.
  - **`-C` handling** matches shell-guard's contract: relative `-C` (`-C .`,
    `-C ./sub`) → DENY+notice (not auto-normalized); external absolute `-C`
    (`-C /external`, `-C ../`, `--git-dir=/external`) → DENY+notice (exec-ro is
    allowlisted and cannot prompt, so it cannot reach external repos); an
    in-repo absolute `-C` (a subdir of the repo root) → ignored for
    classification, ALLOW if the verb is read-only. **Accepted heuristic
    residual (F6):** path classification applies to the SPACED form
    `-C <path>` only; the ATTACHED short form `-C<path>` (e.g.
    `git -C/tmp/out-of-repo diff`) is NOT path-classified — the verb is still
    extracted past the unrecognized flag, so mutations (`git -C/tmp/x commit`)
    are still caught, but a readonly verb against an external repo is allowed
    through. This is accepted because exec-ro is explicitly heuristic (the
    authoritative layer is the OS-level exec-sandbox) and a fix would require
    maintaining equivalent semantics in both this Go classifier and the JS
    shell-guard walker plus parity tests — not justified by the demonstrated
    benefit (worst case is "readonly verb against an external repo", not a
    mutation bypass). See `TestClassify_GitAttachedCIsAcceptedResidual`.
  - exec-ro **executes the command exactly as given or DENIES** — it never
    rewrites the command. On DENY it prints a human-readable notice to stderr and
    exits non-zero (no prompt, since the outer invocation is allowlisted).
  Use `exec-ro` when an agent wants prompt-free read-only inspection (git or
  non-git). If `exec-ro` DENIES a command, the command was not proven read-only
  and was not executed: DENY is final, so do not rewrite or rerun it through
  another verb. For separately identified, explicitly granted host-local
  read-code work, use `vh-agent-harness exec-sandbox` when the calling role has
  the grant AND the applicable mode-floor (`exec_sandbox.min_mode`) supplies the
  required containment; it does NOT follow a command into `proxy`/`docker_compose`
  backends. Use `exec` for genuine mutation, anything with shell plumbing, or
  runtime/backend execution.
- **Run a command under a kernel-enforced HOST-LOCAL Linux sandbox:** `vh-agent-harness exec-sandbox [--sandbox=off|best-effort|strict] [--net=deny|allow|ask] -- <cmd>`. An optional **mode floor** (`exec_sandbox.min_mode` in `run-shape.yml`) can clamp the effective mode UP so a caller can never run below the configured minimum (see "Mode floor" below).

  **Two execution planes (read this).** The exec commands look like one family
  but sit on two disjoint planes. `exec` and `exec-ro` dispatch through
  `resolveBackend()` and are runtime-backend-aware: under `host-shell` they run
  on the host; under `proxy`/`docker_compose` they run INSIDE the container.
  `exec-sandbox` is a HOST-LOCAL Landlock+seccomp trampoline that NEVER calls
  `resolveBackend` and always runs on the host. The Landlock/seccomp
  restrictions apply only to the host process tree directly launched by the
  trampoline; they do NOT become Docker, proxy, or remote-backend security
  policy (Docker is client/server, so a daemon-created container process is
  governed by the container's own policy, NOT the caller's Landlock/seccomp
  profile). Wrapping a `docker compose exec`/proxy payload in `exec-sandbox`
  constrains the local client process but NOT the in-container payload.

  exec-sandbox composes two pure-Go, unprivileged, kernel-enforcing primitives:
  **Landlock** (filesystem integrity) + **pure-Go seccomp-BPF** (network +
  high-risk syscall hardening). It is layered WITH exec-ro (they compose, exec-
  sandbox does NOT replace it): exec-ro is the script-level heuristic pre-filter
  (fast, spoofable by complex shell); exec-sandbox is the kernel-enforced
  authoritative layer for HOST-LOCAL execution (survives bypass attempts because
  it is enforced in the kernel, not in Go user-space). Single static Go binary —
  no bwrap, no cgo, no libseccomp. **Build prerequisite:** source builds require Go 1.25+ (`go.mod`
  requires `go 1.25.0`, bumped from 1.22 for the new OS-primitive deps); the
  binary remains a single static build via `CGO_ENABLED=0`. Binary self-update
  (`vh-agent-harness update` from the installed binary) is unaffected.

  **HONESTY FRAMING (read this):** exec-sandbox is an **integrity + network**
  boundary, NOT a confidentiality/path-hiding boundary. Landlock is additive:
  denied paths stay **visible** (stat-able, metadata exposed) but unwritable
  (EACCES on open-for-write). The promise is "the command cannot WRITE or
  NETWORK outside the contract," NOT "the command cannot SEE anything." `stat
  ~/.ssh` succeeds (metadata visible); `ls ~/.ssh` fails (cannot open directory
  for listing); `touch ~/.ssh/foo` fails (EACCES). This is by design — v1
  accepts "inaccessible but visible" rather than adding bwrap path-hiding
  (deferred to roadmap as O3).

  **Architecture (two-stage re-exec trampoline):** the parent feature-detects
  (raw `landlock_create_ruleset` probe + `seccomp.Supported()`), serializes the
  profile to env vars, then fork/execs itself as a hidden
  `__exec_sandbox_child` in a new session/process group (`Setsid`). The child
  installs protections in order — `PR_SET_PDEATHSIG(SIGKILL)` (dies if parent
  harness dies) → `SetNoNewPrivs` → seccomp filter (`FlagTsync`) →
  `landlock.V9.BestEffort().RestrictPaths` → `syscall.Exec` — then replaces its
  process image with the target. Landlock is per-process/irreversible so it
  MUST be in the child, not the parent. The child also sets
  `GIT_OPTIONAL_LOCKS=0` in the target environment (parent sets before fork).

  **Default profile (Profile B):**
  - Read: repo root, `/usr`, `/bin`, `/sbin`, `/lib`, `/lib64`, `/lib32`,
    `/etc`, `/dev`
  - Write: `./tmp/` only (plus `/dev/null` as RW — writes discarded by kernel)
  - Network: denied (seccomp blocks socket/connect/bind/listen/accept/sendto/
    recvfrom)
  - `.git`: read-only (inherited from repo root — Landlock is additive: a
    subpath cannot be less restrictive than its parent in one layer)
  - Sibling repos / home-sensitive paths: denied (not in ruleset → EACCES on
    open; metadata may still be visible via `stat`)

  **Modes (`--sandbox`):**
  - `off` — no sandbox; run directly.
  - `best-effort` (default) — use OS sandbox if available; otherwise print a
    LOUD warning and fall back to exec-ro classification level (import
    `execro.Classify` read-only, run if allowed, deny if not).
  - `strict` — require OS primitives; fail-closed (exit non-zero, do not run)
    if unavailable.

  A **mode floor** (see below) can clamp the effective mode UP from the
  caller's `--sandbox` choice, so e.g. `--sandbox=off` under a strict floor
  silently runs at `strict` instead.

  **Network (`--net`):** At the syscall layer this is a binary filter: seccomp
  blocks network syscalls when denied, permits when allowed. Default = deny.
  - `deny` — block socket/connect/bind/listen/accept/sendto/recvfrom via
    seccomp (ActionErrno; the command gets EPERM/ENOSYS on the syscall).
  - `allow` — permit network (no network blocklist; high-risk syscalls like
    ptrace/bpf/mount/unshare remain blocked).
  - `ask` — interactive `[Y/n]` prompt to stderr (TTY only). **Non-TTY →
    hard-deny + stderr notice + exit non-zero** (agents CANNOT auto-accept).
    The ask decision is resolved in the PARENT before forking the child, so the
    child trampoline only ever sees deny or allow.

  **Mode floor (`exec_sandbox.min_mode`):** a project may set a binary-enforced
  minimum sandbox mode so a caller can NEVER downgrade below it — regardless of
  the `--sandbox`/`--net` flags passed on the command line. Configured in
  `run-shape.yml` (project-owned):
  ```yaml
  exec_sandbox:
    min_mode: strict   # off | best-effort | strict
  ```
  - **strict floor** — clamps BOTH axes: `--sandbox` is forced to `strict`
    (writes outside `./tmp` become Landlock-impossible) AND `--net` is forced to
    `deny` (network becomes seccomp-impossible). `--sandbox=off` and
    `--net=allow` are silently upgraded; the caller cannot escape.
  - **best-effort floor** — clamps `--sandbox` up to at least `best-effort`.
  - **explicit `off`** — no clamp; the flags behave as documented above. This is
    a DELIBERATE opt-out (the project explicitly accepts uncontained execution).
  - **ABSENT (no `exec_sandbox` block / no `run-shape.yml` anywhere in the
    chain)** — the contained no-flag default (`best-effort`) and any explicit
    stricter mode are honored exactly (standalone contained behavior). An
    EXPLICIT `--sandbox=off` downgrade is **REFUSED** — silence is not consent
    to disable containment. Because `run-shape.yml` is project-owned, the
    harness cannot ship a floor with the exec-sandbox grant, so every adopter is
    unfloored by default until they add the key; the refuse is what keeps an
    explicit downgrade from becoming fully uncontained in an unfloored repo. To
    disable containment deliberately, set an explicit `min_mode: off`.
  - **Discovery** — the floor root walks the ENTIRE ancestor chain from BOTH
    the real (physical, symlink-resolved) cwd AND the `--cwd` target, taking
    the MOST RESTRICTIVE floor found across the whole chain (not
    nearest-wins). A child `run-shape.yml` without an `exec_sandbox` block is
    skipped (walk continues); a weakening child floor (`min_mode: off` planted
    under `./tmp/`) is overridden by a stricter enclosing parent. A
    subdirectory invocation, an out-of-project `--cwd` targeting a
    strict-floored project, or a weakening child floor cannot escape an
    enclosing parent's strict floor. (An explicit `off` at any level counts as
    a configured floor — the absent-vs-explicit-off distinction is what the
    refuse hinges on: absent refuses the downgrade, explicit off honors it.)
  - **Fail-closed** — a present-but-BROKEN floor (wrong type, value/key typo,
    YAML syntax error, directory at the path, or unreadable file) makes
    exec-sandbox REFUSE to run uncontained rather than silently dropping the
    floor. A genuinely ABSENT floor is distinct from an explicit `off`: absent
    refuses an explicit `--sandbox=off` (see above), while still honoring the
    contained default and stricter modes.
  - **Seed default** — new installs seed `min_mode: strict` via
    `defaultRunShapeSeed`. On `update` from a pre-floor version, an existing
    consumer's `run-shape.yml` is preserved (project-owned), so the floor is
    NOT retroactively applied until the consumer adds the key (the absent-floor
    refuse + the `exec-sandbox-floor` doctor WARN are what protect existing
    adopters in the meantime). Fail-closed if OS primitives are unavailable
    under a strict floor.

  **Seccomp policy = focused BLOCKLIST, not broad allowlist.** Default action
  is ALLOW; the blocklist covers (a) network syscalls when `--net=deny`, and
  (b) always-blocked high-risk syscalls: ptrace, process_vm_readv/writev, bpf,
  perf_event_open, open_by_handle_at, mount/umount2/pivot_root/move_mount/
  fsopen/fsmount/fsconfig/open_tree, unshare/setns, swapon/swapoff, reboot,
  settimeofday/clock_settime, kexec_load/kexec_file_load, init_module/
  finit_module/delete_module, vmsplice. clone/clone3 are intentionally LEFT
  ALLOWED (blocking them breaks normal fork/thread creation); namespace defense
  relies on blocking unshare/setns/mount/pivot_root/move_mount/fs* instead.

  **macOS = Linux-first.** No Seatbelt profile in v1. On macOS, strict fails
  closed (no primitives); best-effort prints a loud warning and falls back to
  exec-ro classification. macOS must NEVER pretend protected.

  Use exec-sandbox when you want kernel-enforced HOST-LOCAL guarantees that the
  command cannot write outside the repo/tmp contract or make unauthorized network
  connections. Compose with exec-ro on the host-shell backend: exec-ro is the
  fast pre-filter; exec-sandbox is the authoritative backstop — but ONLY for
  host-local execution. Under proxy/docker_compose, exec-ro's classified command
  runs in-container and exec-sandbox cannot follow it; use backend-native
  container security for in-container containment.

### Work tracking

- **Track work:** `docs/planning/backlog.md` is the canonical task-status source
  of truth (seeded on install, `project_owned`). Agents edit it **freely** under
  the hybrid split-commit discipline: re-read from disk before editing, edit
  only your own task rows (stable IDs), and **commit backlog separately from
  code** so a concurrent backlog edit can't collide with your code commit. On
  a concurrent backlog race, re-read from the new HEAD, re-apply your row, and
  retry — **do
  NOT revert `backlog.md`** (that discards a collaborator's update); in
  particular, `commit-gate.sh revert docs/planning/backlog.md` is the
  blind-revert anti-pattern on the ledger. Load the `backlog` skill before
  editing. DEFER / p2 / follow-up findings follow a **drain-first** lifecycle
  (see `docs/coordination/RECORD_LIFECYCLE.md`): they pass an **admission bar**
  (resolve-first / admitted-value — precise question, concrete area + file
  scope, validation approach, an ADMITTED BLOCKER, a ground-truth-derivable
  trigger when deferral is trigger-dependent, provenance, dedup) before they are
  filed as a `draft` card in `.local/coordinator/tasks/` (via `/write-task`),
  and are **drained directly** by default — no backlog row is created before or
  after. A card is promoted to a backlog row ONLY when it crosses a boundary
  (blocked / owner-change / must-survive-session); only then does the promoter
  apply the Definition of Ready (and the predicate checker,
  `.opencode/scripts/check-defer-triggers.mjs`, promoter-use-only, never
  blocking, confirms a trigger for trigger-gated candidates). A `completed`
  card is **retired only on landing** — never on review approval alone — when a
  commit carrying `Task-Card: <card-id>` is reachable from the integration
  branch (`git log <branch> --fixed-strings --grep="Task-Card: <card-id>"` ≥ 1;
  the `commit-message` agent authors the trailer, the committer is pass-through).
  The landing commit (carrying the card id) is the durable record: no
  tombstones, no done-rows, checkpoints only for campaign closeouts / reopenable
  decisions. Run `/backlog-cleanup` (or `vh-agent-harness exec node
  .opencode/scripts/normalize-backlog.js`) to tidy/archive after a batch edit.
  Roadmap intent lives in `docs/planning/roadmap.md`. The backlog is an
  **eventually-consistent ledger**: the safety model is (a) the **commit-gate
  preflight** — `acquire` refuses any path list that mixes
  `docs/planning/backlog.md` with code/docs changes, so split-commit is
  ENFORCED, not advisory (no real-time per-edit nudge is achievable in opencode
  v1.14.x, so agents learn the discipline at the commit boundary from the
  rejection message); (b) the **promoter-cycle reconciliation** — each cycle the
  promoter runs `normalize-backlog.js --check`, reconciles holding-area cards
  against backlog rows, and lands backlog changes as a backlog-only commit; and
  (c) the **`backlog` skill** as the agent's procedure reference. Code commits
  never wait on a backlog blob.
- **Retire unpromoted transport cards with `/task-delete <id>`** (validated
  destructive single-card wrapper). A DEFER/fog candidate in
  `.local/coordinator/tasks/` that should never reach execution is removed
  (card + local report directory) via `/task-delete <id> [force]`. It is
  irreversible hard removal, NOT a lifecycle status — it does not mark the task
  cancelled, create a tombstone/archive, edit `backlog.md`, or bypass
  `/task-review` / `/task-closeout`. A `draft`, `ready`, or `cancelled` card is
  freely disposable (direct `rm` of the gitignored file remains an equivalent
  operator path for those statuses); a `completed` card is **landing-gated** —
  retirement is allowed only when a commit carrying `Task-Card: <card-id>` is
  reachable from the integration branch, so direct `rm` of a `completed` card is
  NOT equivalent (it bypasses the landing check — see
  `docs/coordination/RECORD_LIFECYCLE.md`). The Slice-2 retirement CODE will
  enforce the reachability check in the sanctioned op; until then a `working`,
  `reported`, `blocked`, or `completed` card is refused without an explicit
  `force` (a `completed` card is additionally landing-gated per the contract
  above) because its report directory may carry evidence a coordinator gate
  still needs. It
  rejects wildcard/path-like/multiple-id/whitespace-separated input before any
  filesystem mutation, confines removal to the transport roots, surfaces the
  card (id, title, status, owner, report dir) to the operator *before* the
  destructive call, and prints the post-removal `removed` summary after. No
  `commit` follows — the transport is gitignored.
- **Model-originated skill proposals use a parallel transport,
  `/skill-propose`.** A model that identifies a reusable workflow worth becoming
  a repo-local skill captures it as a structured `draft` card under
  `.local/coordinator/skill-proposals/` via `/skill-propose create` (backed by
  the `save_skill_proposal` plan-state operation) — gitignored LOCAL CANDIDATE
  TRANSPORT, never truth, never authority. This is the sanctioned intake
  surface; the `/init` direct-write path (a model writing `SKILL.md` directly
  into `.opencode/skills/`) is REJECTED. Provenance is enforced at the write
  layer: nested `metadata.proposal-origin: model-session` stamped from the real
  session; a top-level `created_by` is REFUSED. The human gate is load-bearing
  and **documentation-enforced** (same pattern as task-card promotion — the
  plan-state tool surface is one tool with an operation enum and cannot
  distinguish human from model callers, so `set_skill_proposal_status` cannot
  be scoped out of agent-reachable plan-state):
  `/skill-propose accept|reject <id>` moves a `draft` to `accepted`/`rejected`
  (terminal); a model cannot move its own proposal out of `draft`, and the
  downstream mechanical gates (overlay authoring → S2 → pilot → evidence →
  core promotion) hold regardless of who calls it. Acceptance
  creates NOTHING — it only records human approval for SEPARATELY AUTHORIZED
  overlay authoring under `.vh-agent-harness/overlays/<pack>/skills/`, followed
  by the unchanged S2 → overlay pilot → external evidence → core promotion path.
  `/skill-propose delete <id>` retires an unpromoted card (like `/task-delete`,
  destructive hard removal of gitignored transport, NOT a lifecycle status).

### Refresh & migration

- **Refresh after a new binary or config change:** `vh-agent-harness update`
  (preview with `--dry-run`). Armed-file conflicts are recorded — list them with
  `vh-agent-harness proposals`.
- **In a dev/dogfood repo that edits `templates/core/` (this one), prefer
  `make update` over a bare `vh-agent-harness update`.** `update` renders only
  from the corpus embedded in the running binary, never from the working tree; the
  `Makefile` target rebuilds first so it picks up current source. A bare
  `vh-agent-harness update` uses the PATH binary's embedded corpus, which may be
  older than your in-tree source and will silently overwrite in-progress
  `.opencode/` edits with older bytes. `doctor` does not detect this revert (it
  re-renders from the same stale embedded corpus and byte-compares). Consumers
  whose binary embeds the same corpus they run with are unaffected.
- **This dev-repo footgun is now ENFORCED, not just documented.** A target that
  positively looks like a source checkout — it carries BOTH `corpus.go` AND
  `templates/core/` at its root — is guarded. When the binary's embedded corpus
  DIFFERS from the checkout's `templates/core/`:
  - a LIVE `vh-agent-harness update` REFUSES (non-zero exit, before any write);
    recovery is `make update` (the `Makefile` target rebuilds first), and
    `--allow-stale-corpus` is the explicit override (it is NOT `--force`, which
    already means bypassing the uninitialized-target confirmation);
  - `vh-agent-harness update --dry-run` still previews, but warns that the
    output reflects the BINARY's embedded corpus, not what a rebuilt binary would
    render;
  - `vh-agent-harness doctor` surfaces a non-failing `dev-stale-embed` WARN and
    qualifies its `managed-drift` line to say "in sync with the embedded corpus
    in this binary" (never a bare "in sync"). The WARN alone never makes doctor
    UNHEALTHY.

  A target missing either marker (every consumer) is completely unaffected — the
  guard never fires there. The guard never asserts which side is "ahead": a byte
  comparison proves difference, not chronology, and the recovery is `make update`
  either way.
- **Inspect migration notes for a release:** `vh-agent-harness help migrate`
  (the **bounded forward path** — every bundled note whose target falls in the
  half-open interval `(adopted, binary]`, oldest first, where `adopted` is the
  locally installed version detected from lineage and `binary` is the running
  binary; releases in the interval with no consumer-visible note are skipped
  silently) or `vh-agent-harness help migrate vX.Y.Z` (a specific release's
  note). When no adopted version is detected, or either endpoint is not a clean
  released version (e.g. a `-dev`/`+meta` build), it honestly reports that a
  range cannot be inferred rather than guessing. It is **documentation only** —
  it never modifies files.

### Docs & prompts

- **Print a generic agent-workflow doc:** `vh-agent-harness docs [key]`. With no
  argument, lists every available doc key; with a KEY, prints that doc to stdout.
  These docs describe harness machinery identical for every adopter (the session,
  memory, prompt, and skill model) and ship inside the binary rather than into
  your repo — current keys include `opencode-memory-model`,
  `opencode-session-workflow`, `opencode-prompt-guide`, `opencode-skills`, and
  `temporary-files` (run `vh-agent-harness docs` to see the live set). It is
  **read-only**: it only writes to stdout and never modifies files. This repo
  dogfoods live source by mapping keys to repo-relative files in
  `.vh-agent-harness/docs-overrides.yml`; adopters with no overrides file always
  get the embedded copy.
- **Print a named system prompt:** `vh-agent-harness sys-prompt [name]`. With no
  argument, lists every available prompt key; with a NAME, prints that prompt's
  raw bytes to stdout. Prompts ship inside the binary (embedded defaults); an
  overlay pack or operator can override one by rendering
  `.opencode/sys-prompts/<name>.md` (the live tree takes precedence over the
  embed). It is **read-only**: it only writes to stdout and never modifies files.

### Diagnostics & verification

- **Commit-gate closeout status vocabulary.** Each commit-gate closeout records
  its outcome to a durable, append-only JSON-lines ledger at
  `.git/commit-gate/closeouts.log` (gitignored runtime state; never committed)
  that survives the transient per-session metadata cleanup. The ledger is the
  substrate for the `head-progress` doctor check (the HEAD-staleness WARN).
  Recorded `status` values (every record also carries the **`head_at_acquire`**
  field — the acquire-time HEAD the committer pinned as the review diff base;
  S1: the reviewer diffs against this anchor, not bare HEAD, so a concurrent
  committer moving HEAD can't slip an unrequested file into the reviewed scope):
  - `committed` — the closeout landed and advanced HEAD (the normal success).
  - `no_head_progress` — the closeout landed but HEAD did **not** advance past
    the reference it was built on (the pre/post-HEAD-equal canary from the
    vh-solara field report). Structurally unreachable via a normal
    acquire→commit (a `commit-tree` always produces a new object, and the
    `no_changes` guard blocks no-op trees); it emits on the genuine post==pre
    edge / fault condition. Distinct from the terminal failures below, which
    did **not** land at all.
  - `could_not_land` — terminal: the closeout could not land due to a content
    tangle (the same-file conflict the CAS 3-way merge cannot reconcile; reason
    `merge_failed`/`write_tree_failed`). `post_commit_head` is `current_head`
    (the branch did not move on this session's behalf).
  - `rebased_refused` — terminal (S2, fail-closed): the CAS 3-way merge
    reconciled the reviewed tree against a concurrent winner's HEAD, but the
    merged tree **differs** from the tree the reviewer approved (reason
    `reviewed_tree_diverged`). The gate REFUSES to land a tree the reviewer
    never saw and requires re-acquire + re-review; `post_commit_head` is
    `current_head` (the branch moved on the winner's behalf, not this
    session's). A terminal sibling of `could_not_land`.
  Concurrent-HEAD movement the CAS retry loop can still reconcile never reaches
  the ledger as a failure — it retries until the merge reproduces the reviewed
  tree (`committed`) or exhausts into a transient `error`/`cas_retry_exhausted`
  stdout status (not a ledger entry). The pre-S1 `cas_conflict` status is no
  longer emitted; its content-tangle path is now `could_not_land`.
  The durable ledger is the cheap near-term surfacing mitigation for the ~6h
  commit-freeze symptom (Pattern 4 same-file tangle); it does not address the
  root cause (worktree/lease isolation — long-term).

- **Verify:** `vh-agent-harness doctor` (lineage, armed-schema, managed-drift,
  overlay-perm, environment, config-refs, gitignore, auto-classifier,
  auto-gate-ignore, skills, subagent-depth, defer-liveness,
  staged-errata-content, behavioral-closure, dev-stale-embed, f1-envelope,
  f1-f2-consistency, f2-pairs, head-progress, complexity-advisory,
  recurrence-state, exec-sandbox-floor, shipped-pilots, closeout-reach,
  rewrite-parity). The `head-progress` check reads the commit-gate's
  durable closeout ledger (`.git/commit-gate/closeouts.log`) and WARNs when the
  last N (default 3, env `COMMIT_GATE_STALE_HEAD_THRESHOLD`) successful closeouts
  all recorded the same `post_commit_head` — a flatline signalling commits may
  be recording as successful without advancing HEAD (Pattern 4 same-file tangle);
  it SKIPs cleanly when there is no ledger or fewer than N closeouts (consumers
  and fresh checkouts see nothing), and never changes doctor's exit code (WARN
  only — doctor INFORMs; the coordinator reads, never acts). The
  `closeout-reach` check reconciles the same closeout ledger against branch
  REACHABILITY in both directions: (dir-1, WARN) a committed entry whose
  `post_commit_head` is NOT reachable from any branch (orphaned — the operator
  `git reset` escape hatch moved the branch away after the gate recorded the
  commit; the object may still exist but no branch tip reaches it); (dir-2,
  INFO/advisory) a commit reachable on the current branch with NO ledger entry
  (work that landed unrecorded — e.g. a cherry-pick through the escape hatch;
  indistinguishable from a review bypass). Reachability — NOT object existence —
  is the property that means "landed" (`git show` passes for a reverted/orphaned
  commit); the check asserts it via `git rev-list --branches` (dir-1) and
  `git rev-list --max-count=500 HEAD` (dir-2). dir-1 is WARN (never FAIL); dir-2 is
  INFO (advisory; never FAIL/WARN); greenfield (no ledger / no committed entries)
  is SKIP — fail-open-safe, exactly like head-progress. The `auto-classifier` check lints the shape (field
  set + types + enums) of the auto-classifier-pilot overlay's config files when
  present — a present-but-invalid `auto-gate-config.json` / `auto-gate-llm.json`
  FAILs; absent configs are never failures (defaults apply). The `auto-gate-ignore`
  check FAILs when a never-commit auto-gate file (`auto-gate-llm.json`,
  `*.local.json`) is tracked by git (an ignore rule does NOT untrack) or present
  but not gitignored (would be staged on the next `git add`); FAILs with rotate
  guidance when a tracked `auto-gate-llm.json` carries a non-empty literal
  `apiKey` (a credential incident — the key value is never emitted, only the
  finding); WARNs when protection is missing (overlay in use, no rule yet) or
  only via a non-portable global `core.excludesFile`; and SKIPs when the overlay
  is unselected and no config files exist. The `skills` check validates every
  rendered skill's SKILL.md frontmatter (Go-native; no python). The
  `subagent-depth` check resolves the EFFECTIVE merged OpenCode `subagent_depth`
  across project + user/global configs (OpenCode precedence: project overrides
  global) and WARNs when it is unset or below the minimum this harness's
  multi-level delegation needs (`subagent_depth` defaults to 1, which breaks
  coordination→build→committer and the solution-brief chain); a user-level
  override is honored and never false-flagged missing. The `defer-liveness`
  check FAILs when an open coordinator defer/errata card contradicts a present
  released (or about-to-release) migration-note claim, and is fail-closed on
  malformed cards. The `staged-errata-content` check FAILs when an
  about-to-release (untagged) migration note exists AND any errata card with
  `status: staged` has correction content NOT present in that note — this turns
  the "staged erratum never actually injected" failure mode into a hard stop.
  The `behavioral-closure` check FAILs when a durable closeout artifact
  (`.local/coordinator/reports` or `docs/checkpoints`) carries a
  `behavioral-closure` declaration that is internally inconsistent —
  specifically a `verdict: proven` claimed WITHOUT a proven crux `result:` —
  and is fail-closed on unknown/malformed declarations; an absent token PASSES
  (the pilot does not force adoption, so pre-pilot closeouts stay HEALTHY). The
  token declares consistency; it does NOT prove the cited crux path executed
  (proving that needs the repo-specific live verification). The `rewrite-parity`
  check scans the same closeout surfaces for fenced `rewrite-parity` contract
  blocks (versioned JSON governing explicitly-declared deletion/rewrite slices —
  `mode: deletion_replacement` or `mode: modification_only_rewrite`) and FAILs
  when one is structurally malformed or internally inconsistent (unknown
  version/mode/result enum, missing verifier per behavior, duplicate behavior
  IDs, prior-surface shape errors). It SKIPs cleanly when no artifact carries a
  block — the contract is OPT-IN, so a plain delete/refactor/rename with no
  block contributes no finding (zero burden on ordinary work). Like
  behavioral-closure, it is a STRUCTURAL consistency audit, NOT a truth prover;
  the commit-gate (`acquire --rewrite-parity-contract <file>` Stage-1 precheck)
  and the closeout transition (Stage-2 completion refusal unless every behavior
  is `proven` with a non-empty receipt) are the primary enforcement surfaces,
  and doctor is independent defense-in-depth. The `f1-envelope`
  check scans the same closeout surfaces for fenced `f1-synthesis-envelope`
  JSON projections and FAILs when a COMMITTED projection is structurally
  inconsistent — unknown closed-vocabulary enum, missing/duplicate family when
  applicable, a foreign summary on the wrong family entry, a dangling
  cross-reference, or a `semantic_digest` that no longer matches the canonical
  content; it SKIPs cleanly when no artifact carries a projection (nothing to
  audit — this is not "passing an applicable seam"; an applicable envelope with
  a missing entry is INCOMPLETE and FAILs). Like behavioral-closure, it is a
  STRUCTURAL consistency audit, NOT a truth prover: a passing projection is
  internally consistent, not thereby proven true.
  The `f1-f2-consistency` check scans the same closeout surfaces for
  `f1-synthesis-envelope` projections that carry F2-DERIVED view metadata
  (`f2_view`) and FAILs when the F1→F2 emit-boundary digest-binding contract is
  violated: a `semantic_digest` that no longer binds the canonical content (F2
  drifted it in place instead of requesting a new F1 emit), a missing binding
  reference (`synthesis_cycle_id` / `entry_ids` / digest), or a foreign field on
  the `f2_view` object (content smuggled past the closed derived-field allow-
  list: `storage_locator`, `write_timestamp`, `view_model_version`,
  `renderer_version`, `attachment_meta_ref`). It SKIPs cleanly when no
  projection carries an `f2_view` (nothing to audit — F2 rendering is a separate
  track and may not have attached view metadata yet). Like `f1-envelope`, it is
  a STRUCTURAL consistency audit, NOT a truth prover.
  The `complexity-advisory` check loads the platform_armed
  `complexity-policy.yml` and runs the repo-snapshot projection (file LOC over
  tracked source, excluding rendered `.opencode/**`, test files, and
  dependency/cache/tmp paths). It WARNs when any file exceeds the configured
  threshold (default 500) and surfaces the top N candidates (default 10). It
  NEVER returns FAIL — complexity signals INFORM; they do not gate. The check
  SKIPs when the policy is absent or disabled, PASSES when no file is
  nominated, and WARNs when candidates exist. A malformed policy WARNs too
  (the armed-schema owns the FAIL). The blank
  `complexity-dispositions.yml` is validated structurally without taking update
  ownership. This is the staged advisory hybrid: complexity is advisory
  evidence, never transition authority.
  `vh-agent-harness diff` shows drift vs. the corpus.
- **Inspect / validate skills:** `vh-agent-harness skill list` prints every skill
  (core, overlay-pack, and rendered) with its source, whether it is rendered to
  `.opencode/skills/`, and whether its SKILL.md frontmatter is valid.
  `vh-agent-harness skill validate [dir...]` validates one or more skill
  directories' frontmatter (with no args, validates every rendered skill). Both
  read the trees directly, so their view is always fresh.
  **Restart opencode after `update` touches skills:** opencode caches the
  discovered skill list per-process (a module-closure Map cleared only on
  process death), so a running session will NOT see newly added/changed skills
  under `.opencode/skills/` until you restart it. `update` prints a one-line hint
  when it writes skill files.
- **Package a bug bundle:** `vh-agent-harness diagnostics-export [--dry-run]
  [--output <path>]` bundles selected harness state (`.opencode/state/`,
  `.local/coordinator/`, `.local/config/`, `docs/checkpoints/`) into a
  field-aware-redacted `tar.gz` under repo-scoped `tmp/`. It is **never
  auto-uploaded** — the operator decides if/when to share. Run `--dry-run`
  first to review the manifest and redaction counts. See the `diagnostics-export`
  skill for the operator review checklist.
- **Recurrence dedup bridge:** `vh-agent-harness recurrence dedup` is a stateless
  stdin/stdout JSON bridge between the JS task-card producer
  (`saveCoordinationTask` in `.opencode/scripts/state-lib.js`) and the pure Go
  recurrence derivation (`internal/memory/recurrence`). The producer scans
  existing cards, sends `{incoming:{task_id,recurrence:{...}}, existing:[...]}` on
  stdin, and receives `{action:"merge"|"new_card", effective_id, canonical_task_id,
  merged:{...}}` on stdout. On `merge` the producer updates the canonical card
  (count bumped, observation appended, ack held → unacknowledged) instead of
  spawning a new card; on `new_card` it writes fresh. This INFORMs only — the
  producer APPLIES the convenience; doctor check #12 (`defer-liveness`) is the
  transition authority that FAILS-CLOSED at release time when a recurrence is
  unacknowledged (see "Single release-authority model" below). Not normally run
  by humans.
- **Task-card validator:** `vh-agent-harness task-card validate [<file>]`
  validates a task-card JSON document against the task-card schema
  (`docs/coordination/schemas/task-card.schema.json`) plus the cross-field
  acknowledgement-pair invariant (`recurrence_count >= last_acknowledged_count`)
  that JSON Schema draft-07 cannot express. With a file path it reads that file;
  with no argument it reads stdin. It prints `task-card: valid` on stdout and
  exits 0 on success, or a defect list on stdout and exits 1 on rejection
  (read/parse errors go to stderr). The validator ships a bounded, dependency-
  free draft-07 subset engine (pure logic in `internal/taskcard` — no Python, no
  pip; the Go port of the retired `.opencode/scripts/verify-task-card-schema.py`
  under defer-018). The contract is also gated by a Go test that ports the
  validator's fixtures, so it runs in `go test ./...`. This INFORMs only — it is
  a diagnostic, not a transition authority.

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

## Discovering overlay packs (`overlay list`, `overlay docs`)

`vh-agent-harness overlay list` enumerates **every** pack visible to this
project — embedded in the binary PLUS project-local under
`.vh-agent-harness/overlays/` — one line per pack, with its source
(`embedded` / `project-local`) and status (`selected` / `available`).

`selected` = renders on the next `vh-agent-harness update`. A pack is selected
when ANY of:
- its name is listed under `overlays:` in `vh-harness-profile.yml`, OR
- its capability-manifest id is listed under `capabilities:` (the `release`
  pack is dual-selectable via `core/release` — both paths converge), OR
- it is a shipped default-on pilot whose feature key resolves `true`.

`available` = shipped/visible but not selected.

This is the discovery surface that prevents a shipped-but-unselected pack from
being wrongly concluded to **not exist**. Use it before authorizing a
rebuild-from-scratch: a pack that ships embedded (e.g. `auto-classifier-pilot`,
`release`, `repo-mail`) is always visible here, whether or not it is selected.

`vh-agent-harness overlay docs <name>` prints any pack's README (configuration
reference, enablement steps) — embedded, project-local, selected or not — so
you can evaluate a pack before enabling it. Enable a pack by listing it under
`overlays:` in `.vh-agent-harness/vh-harness-profile.yml`, then run
`vh-agent-harness update`.

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
  runtime, including the `proxy` backend. `exec-ro` is a SEPARATE, narrower gate
  (read-only only, enforced inside the Go binary, never prompts) — see its
  command-surface entry above. The two share the same Go source of truth for git
  mutation verbs (`internal/permconfig/tables.go` → emitted `allowed-commands.js`
  → consumed by both the exec-ro classifier and the shell-guard plugin).
  `exec-sandbox` is a HOST-LOCAL Linux sandbox front door (NOT a backend peer to
  exec/exec-ro): pure-Go Landlock (filesystem integrity) + seccomp-BPF (network +
  syscall hardening), kernel-enforced, single static binary. It never dispatches
  through the runtime backend and never follows a payload into a container; the
  Landlock/seccomp restrictions apply only to the host process tree directly
  launched by the trampoline. It is an INTEGRITY + NETWORK boundary, NOT a
  confidentiality boundary (denied paths stay visible but unwritable). See its
  command-surface entry above for the two execution planes, profiles, modes, and
  the honesty framing.
- `doctor`/`preflight` surface an ownership-raised divergent path (a managed
  file taken to `project_owned` via `harness-ownership.yml`) as a non-failing
  `preserved` (INFO) signal — never a FAIL, never blocks install/update.

## Preserved orphan overlay skills (report-only; `--prune-orphans` to auto-delete)

When you remove an overlay skill source
(`.vh-agent-harness/overlays/<pack>/skills/<name>/`) and re-run
`vh-agent-harness update`, the previously-rendered `.opencode/skills/<name>/`
stays on disk. The renderer **never deletes** files a render no longer
contributes — that is the deliberate conservative posture that protects
operator-authored content. Previously this left removed-source skills
**invisible**: live, unreported, and not surfaced by `update --dry-run`.

`update` and `update --dry-run` now surface those as **preserved orphans** — a
report-only notice naming the orphaned **file** (its full destination path, e.g.
`.opencode/skills/<name>/SKILL.md`), the producing pack, the destination state
(`unchanged`/`modified`), and the removed source. The notice is **file-accurate**:
it lists each removed-source file individually, so a skill directory that still
contains an actively-rendered file is never suggested for whole-directory removal.
The notice is informational; **by default nothing is deleted.** To actually remove a
preserved orphan, delete the **file** named in the notice (e.g.
`rm .opencode/skills/<name>/SKILL.md`). Remove the **whole** skill directory only
after verifying that **every** file inside it is orphaned — a directory may mix
orphaned files with files still being actively rendered. Or restore the overlay
source to clear the notice on the next update. Or pass `--prune-orphans` (below)
to auto-delete the byte-identical ones.

### Pruning orphans automatically (`--prune-orphans`)

`update --prune-orphans` deletes the preserved-orphan files for you, but only
the ones that are safe to delete:

- **`unchanged`** (byte-identical to the last recorded render) → **deleted
  automatically.** The file is genuinely dead (it carries no operator content),
  so `--prune-orphans` removes it.
- **`modified`** (you hand-edited the rendered file after it was rendered) →
  **refused and reported for manual `rm`.** The harness never auto-deletes a
  file whose bytes differ from what it rendered, because that diff is operator
  content. Remove it by hand (`rm .opencode/skills/<name>/SKILL.md`) if you no
  longer want it.
- **`missing`** (already gone) → nothing to do.

A closing summary prints how many were deleted, how many were refused for manual
removal, and how many were skipped.

**Safety floor.** `--prune-orphans` only ever touches files in the preserved-
orphan set — it never deletes a non-orphan or a project-owned file. The
rendered-outputs manifest records only harness-rendered overlay skill files
(never project-owned paths), and a project ownership override
(`harness-ownership.yml`) that targets a now-sourceless path is rejected before
the apply runs, so a project-owned orphan cannot reach the prune. As
defense-in-depth the prune path also re-checks ownership before each delete and
refuses a project-owned path (mirroring `uninstall --force`'s never-touch-
project-owned guarantee). `--force` does **not** weaken this floor.

**Compose with `--dry-run`.** `update --prune-orphans --dry-run` previews which
orphans *would* be deleted vs refused and deletes nothing. Run it first to
preview before a real prune.

The pruned file's record is retired on the **next** `update` (the prune happens
after the apply, so the manifest written during this run still lists it); a
subsequent plain `update` reports no orphan for a pruned file.

How a **definite orphan** is told apart from benign cases:

- A skill is flagged **only** when its producing overlay SOURCE is now MISSING
  (the source file is gone from its pack, or the whole pack is gone). Removing
  a skill from its pack and re-running `update` flags it.
- **Pack deselection** (removing the pack from `overlays:` while its source dir
  is still on disk) is NOT an orphan — the source still exists, it is just not
  selected. No notice.
- A **project-added** skill dir you created directly under `.opencode/skills/`
  (never produced by an overlay render) is never recorded, so it can never be
  flagged.

This provenance is tracked in a generated manifest at
**`.vh-agent-harness/rendered-outputs.json`** — a JSON file with a
`manifest_version` field, written atomically after a non-dry-run
`update`/`install` only when the generation FULLY applied (P1-SUBSTRATE-001
generation-wide gate). Any live-write failure gates the persist — a failed
managed write, whether a tracked overlay-skill destination OR a non-skill
managed file, leaves the prior manifest byte-for-byte intact and emits a stderr
warning (`substrate.Apply` reports `GenerationFullyApplied=false` while still
returning nil on a live-write failure). This keeps the manifest correlated with
lineage's `last_successful_update_id`, which gates on the same any-failure
condition; the prior v1.1 scope gated only on tracked overlay-skill
destinations. It records each overlay-rendered skill file's destination,
producing pack, source-relative path, and content digest. If the manifest is
absent (a project first updated by a pre-v0.10.0 binary), the first `update`
establishes a baseline from the current render — it does NOT retroactively
adopt pre-existing `.opencode/skills/` dirs as managed (there is no historical
proof they were overlay-rendered).

## Capability residue (deselected capability; NOT an orphan)

A **capability residue** is different from the orphan case above. When a
capability-owned file (for example the `media-perception` agent prompt + skill)
was written by a prior `update` while its capability was selected, and the
capability is later deselected, the already-written on-disk file is **left in
place as inactive residue** — it is a *known* path, so it is exempt from
managed-drift and unexpected-drift, and `doctor` does **not** fail on it. It is
deliberately not auto-deleted.

`--prune-orphans` does **not** remove capability residue. It only consumes
renderstate *orphans* (files whose producing **source** is now missing); a
deselected capability's source still exists, so its residue is never a prune
target.

To remove residue once an operator confirms it is unwanted, delete the **whole
skill directory** (`.opencode/skills/<name>/`, not `SKILL.md` alone) and, for
capability-owned agent residue, the agent file too (e.g.
`.opencode/agents/media-perception.md`). Once the whole skill directory is gone,
the `doctor` skills check SKIPs (no directory to inspect) and `doctor` passes.
Note that a **half-deleted** skill — directory present but `SKILL.md` deleted —
is a deliberate `doctor` **FAIL** (a broken skill definition breaks opencode's
skill discovery), so never remove `SKILL.md` in isolation.

## Git global-flag detection (shell-guard)

Agents routinely run read-only git commands prefixed with global flags such as
`git --no-pager log -1` or `git -C <repo-root> diff`. shell-guard's
`tool.execute.before` hook **parses** these flags to reach the correct
allow/deny/ask **decision**, but it **never mutates the command**. A detector
has a safe fallback ("I don't know → ask"); a rewriter does not, and real agent
commands (pipelines `git … | head`, sequences `cd x; git …`, subshells
`$(git rev-parse HEAD)`, redirects) make a safe whole-command rewrite unprovable
without a fallible parse that would itself mutate execution. So shell-guard
decides; it does not rewrite.

The decision is driven by a **flag registry** (`walkGitGlobals` in
`shell-guard-core.js`) that consumes leading git global flags from the
`git --help` usage line, value-form aware (`-C <path>`, `-c <name>=<value>`,
`--git-dir=<path>`, `--exec-path[=<path>]`, boolean `-p`, etc.), and extracts
the verb past them. Four security verdicts depend on this parse:

1. **Mutation-slip guard.** A mutation verb routed past ANY leading global flag
   is denied before the allowlist is consulted — `git --no-pager commit`,
   `git -C <external> commit`, and `git -C <repo-root> commit` all DENY. The
   walker extracts the verb regardless of how many flags sit between `git` and
   it, so a global flag can never hide a mutation from the gate.
2. **Relative `-C` is denied with a notice.** Any relative `-C` argument (`.`,
   `..`, a subdir) is rejected: `"relative -C paths are not auto-normalized;
   use an absolute path equal to the working directory or drop -C"`.
   Normalizing relative paths would invite symlink / `..` / normalization bugs,
   so the gate refuses to guess.
3. **External `-C` readonly is `ask`, not silent allow.** `git -C /var/x diff`
   is classified `ask` (the operator sees a prompt); the mutation-slip guard
   still turns `git -C /var/x commit` into a hard `deny`.
4. **Info flags** (`--help`, `--version`, `--html-path`, …) with no verb are
   read-only terminal requests → allowed directly.

The always-strip set used for the INTERNAL classification (paging flags only:
`-p`, `--paginate`, `-P`, `--no-pager`) affects the decision but is never
written back. (`--paging=no` is NOT a real git flag and is absent from the set;
the walker treats it as an unknown never-strip boolean.)

**Tradeoff (accepted).** Because the command is never rewritten, opencode's
path-blind L2 matcher still sees the original text. Forms WITHOUT a matching
L2 rule will prompt:

- `git -C <cwd> <readonly>` prompts (there is no `git -C *` L2 rule). In
  practice agents elide `git -C <cwd>` ~always, so this is rarely hit.
- `git --paginate <ro>` / `git -p <ro>` / `git -P <ro>` prompt (only
  `--no-pager` has L2 coverage — the `git --no-pager <sub> *` config-table
  rules are the load-bearing prompt-free path for that form, NOT a rewrite).

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
  A field set to a blessed N/A sentinel (`none` / `n/a` / `null` / `na`; string form only — write `"null"`, not bare JSON `null`) renders
  empty in the re-seed and does not trip the unresolved-token warning — use that
  for `db_user`/`db_name` on a project with no database.
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

> **`--allow-stale-corpus` is a separate override, NOT `--force`.** `--force`
> bypasses this uninitialized-target prompt. The stale-corpus guard (a source
> checkout whose `templates/core/` differs from the binary's embedded corpus)
> is a different hazard and gets its own explicit opt-in. See "Refresh &
> migration" above for when it applies.

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

## Release ceremony (release-tag wrapper + DEFER manifest)

This section documents the project's release-time DEFER safety gate. The
**sanctioned tag mutation surface** is `scripts/release-tag.sh`; raw `git tag`
and `git push` of release tags are forbidden to every agent (shell-guard's
`git-mutation-bypass` rule). The wrapper invokes the DEFER evaluator before any
tag mutation and refuses on any blocker / evaluator-error.

### Operator-interaction contract (release ceremony)

Operator initiation of a release IS the authorization for the standard
end-to-end ceremony. Once the operator instructs a release, the ceremony runs
end-to-end WITHOUT a confirmation round: a green gate means PROCEED. The
anti-pattern this contract kills is a "shall I cut now?" go/no-go round run
AFTER the readiness verdict was already `ready: yes` and every gate was green —
the operator's initiating instruction was itself the authorization.

**STOP AND ASK the operator only when:**
1. any gate is RED or the release-tag wrapper refuses (never work around it);
2. the DEFER evaluator returns `refuse` / `override-required` (`disclose` =
   PROCEED — do not ask);
3. version derivation is ambiguous (two defensible versions with different
   consumer impact);
4. the migration note requires a content decision the arc doesn't answer;
5. an action is irreversible AND non-standard (force-push, history rewrite, tag
   deletion — the standard tag+push of a green ceremony is NOT in this class);
6. anything pending/blocking that the ceremony cannot resolve itself.

**NEVER ask to confirm:**
- the standard next ceremony step;
- post-release cleanup already named in the initiating instruction;
- anything a green gate already authorized;
- bundled secondary tasks in the initiating instruction (e.g. "release then
  delete X") — these are pre-authorized in order; no per-step re-confirmation.

**AUTO-RECOVER (self-recoverable red gates — the complement to STOP-AND-ASK
#1):** a red gate whose recovery is (a) **mechanical** (no decision), (b)
**known-safe** (the recipe cannot weaken a gate or mutate state outside its
named scope), and (c) **named in the closed recipe list below** is
AUTO-RECOVERED, not escalated to the operator: apply the recipe, retry the gate
ONCE, and log the recovery in the closeout. STOP-AND-ASK fires only on a
**recipe-miss** (the red is not in the list) or a **retry failure** (the recipe
was applied but the gate is still red on retry). This is a whitelist of boring
recoveries, NOT a general workaround license — the agent CANNOT extend the list
at runtime.

**Closed recipe list (exhaustive — a red not named here is a STOP-AND-ASK):**
1. **`stale/missing ceremony binary` doctor red (G0c).** Symptom: the
   release-tag wrapper's G0c gate refuses with "ceremony binary missing —
   ./bin/vh-agent-harness not found" or "G0c staleness guard — the ceremony
   binary ... predates the staged-errata-content enforcement". Recipe: run
   `make build` (produces `./bin/vh-agent-harness` from current source), then
   retry the ceremony via the wrapper — G0c resolves `./bin/vh-agent-harness`
   deterministically and never PATH, so the retry exercises the freshly-built
   binary. Known-safe: `make build` writes only `bin/vh-agent-harness` (a build
   artifact); it touches no gate, no manifest, no tag, no ref. Log:
   "AUTO-RECOVER recipe #1 applied (make build + retry via
   ./bin/vh-agent-harness); G0c green on retry."

**Core rule:** operator initiation of a release IS the authorization for the
standard end-to-end ceremony. Green gate = proceed. A recipe-listed red gate
auto-recovers; every other red is a STOP-AND-ASK.

### Single release-authority model (committed manifest)

The release DEFER gate has exactly ONE input mode: the evaluator reads the
**committed disposition manifest** at
`.vh-agent-harness/release-defer-dispositions.json` ONLY and performs NO
`.local/` access in release mode. A missing committed manifest is a hard
`evaluator-error` (NOT `clear`), so the gate protects fresh checkouts. There is
no env switch and no legacy fallback — manifest authority is always active.
`.local/coordinator/tasks/` is **provenance transport only**, read by the
commit-time promoter mechanism (which stays non-blocking); release mode never
reads it.

The `releaser` agent owns the release ceremony end-to-end. Across three
single-path `committer` delegations it lands N (the migration note), R (the
readiness artifact, written by the parent-orchestrator-invoked
`harness-release-readiness` agent), then M (the manifest, whose handshake SHAs
bind to R = HEAD^ at tag time). It re-verifies the handshake and invokes the
wrapper. A release-agent-only operator gets manifest authority by default — no
out-of-band operator release-prep.

The gate surfaces TWO distinct failure classes, with different remedies — both
are reported before any `git tag` invocation:

- **A release-relevant finding requires disposition** (evaluator
  `classification: blocker`, exit 1). Remedy: resolve the finding in the
  manifest (update `release_relevance` / `disposition` / `metadata_state`) OR
  use the override ceremony. The override ceremony CAN cure this class.
- **The manifest itself is missing/malformed/stale** (evaluator
  `classification: evaluator-error`, exit 2). Remedy: REPAIR the committed
  manifest (re-run the manifest ceremony end-to-end). The override ceremony
  CANNOT cure this class — it never authorizes a schema-invalid manifest or a
  stale handshake.

**Two independent DEFER gates; every firing card is dispositioned.** The JS
manifest gate above and the Go all-live defer-liveness gate (`checkDeferLiveness`,
G0c, `internal/cli/release_gate.go`) are TWO INDEPENDENT surfaces over the same
card pool. The manifest provenance policy widens to the Go gate's breadth: EVERY
firing card the release-prep enumerator surfaces gets an explicit, committed
disposition regardless of provenance (`source:review-defer`,
`source:external-study`, `source:p2-followup`, any source; frequently a
non-blocking `disclose`, NOT an automatic blocker). When the GO gate refuses
mid-ceremony, the NON-DESTRUCTIVE escape hatch is the
`VH_HARNESS_DEFER_OVERRIDE_IDS` env var (comma-separated task_ids; surfaced in
the gate's FAIL detail). **Do NOT `rm` transport cards to clear the Go gate** —
use `VH_HARNESS_DEFER_OVERRIDE_IDS`, or add a manifest disposition record. Full
recovery guidance lives in the release-readiness G7 ceremony doc
(`.vh-agent-harness/overlays/harness-dogfood/agents/harness-release-readiness.md`)
and the releaser prompt (`templates/overlays/release/agents/releaser.md`); the
two-gate topology and the v0.19.0 incident it prevents are recorded in decision
memo `researches/decisions/2026-08-02-defer-liveness-provenance-scope-divergence.md`.

### Verification split: `go test ./...` (hermetic) vs `doctor` #12 at G0c (live F4-C)

The defer-liveness contract is enforced at TWO deliberately separated layers.
Confusing them re-introduces the defect this split prevents: a `go test` canary
over the real repo duplicates the authoritative G0c check at the wrong layer
and turns the universal dev test red during release-prep windows whenever an
open defer card's `path_touched` target is touched by a legitimate commit.

- **`go test ./...` — hermetic code + predicate contracts; no mutable
  current-checkout state.** The defer-liveness / F4-C tests in
  `internal/cli/release_gate_test.go` build SCRATCH-REPO fixtures
  (`t.TempDir()` + controlled git history + controlled cards) and assert the
  predicate's contract (active refusal, dormant/clearance paths, silencer
  immunity, fail-closed, fog/cold advisory). They NEVER read the real working
  tree, so editing a file in an open defer card's `path_touched` cannot turn
  `go test` red. (`TestDeferLivenessGate_LiveRepoIsClean` /
  `TestDeferRecurrence_LiveRepoIsClean` were removed for exactly this reason:
  they were live-repo canaries that duplicated G0c at the go-test layer.)
- **`doctor` check #12 (`checkDeferLiveness`) at G0c — the live F4-C /
  defer-liveness authority, the release gate.** `scripts/release-tag.sh` runs
  `VH_HARNESS_DEFER_DIFF_SINCE="$PRIOR_TAG" ./bin/vh-agent-harness doctor` AFTER
  the about-to-release migration note is committed and BEFORE any `git tag -a`
  mutation; an undisposed fired card makes doctor UNHEALTHY and refuses the
  tag. This is where the live-repo assertion belongs.
- **doctor may legitimately be RED between N and M (release prep).** During the
  release-prep window — after an about-to-release note exists but before a
  disposition lands in the committed manifest — `doctor` #12 can be UNHEALTHY
  on an undisposed fired card. That is operational feedback to the releaser
  (dispose the card: closed status / manifest record /
  `VH_HARNESS_DEFER_OVERRIDE_IDS`), NOT a unit-test failure. `go test ./...`
  stays green throughout because it is hermetic; only the release-tag wrapper
  (G0c) treats the live state as authoritative.

### The manifest (project-owned, committed, fresh-checkout-visible)

`.vh-agent-harness/release-defer-dispositions.json` is a schema-v1 JSON file
attesting that the promoter/operator confirmed release relevance and disposition
for the declared release arc. The full schema, disposition matrix, freshness
handshake, and override semantics live in
`.vh-agent-harness/overlays/harness-dogfood/agents/harness-release-readiness.md`
(G7 section) and in the evaluator source at
`templates/core/.opencode/scripts/check-defer-triggers.mjs`. Highlights:

- `release_base: {kind:"tag"|"root", value:<tag>|null}` — the start of the
  release arc. `kind:"root"` (whole history) is used for the very first release;
  there is NO `HEAD~32` fallback in release mode. For `kind:"tag"`, `value` is
  DERIVED authoritatively by the evaluator from the last reachable tag at
  evaluation time; the attested manifest value is advisory (a stale value
  self-heals with no manifest write and cannot block the release), so it can
  never go stale in a load-bearing way.
- `evaluated_commit`, `evaluated_tree`, `manifest_parent_commit` — a freshness
  handshake. All three must equal `HEAD^` and `tree(HEAD^)` at evaluation time.
  The only changed path in `HEAD^..HEAD` must be the manifest itself. This
  prevents weakening the manifest after its claimed evaluation.
- `records[]` — one entry per DEFER finding, with `release_relevance ∈
  {yes,no,unknown}`, `disposition ∈ {block,disclose,override_required}`,
  `metadata_state ∈ {valid,stale,invalid}`, summary/reason/source_ref, and an
  optional `override` object. A record MAY also carry the OPTIONAL recurrence
  acknowledgement pair `recurrence_count` + `last_acknowledged_count`; schema-v1
  entries omit them (bounded backward-compat — the ack check is dormant for
  records without the pair). When the pair is present, doctor check #12 reads the
  committed `last_acknowledged_count` from the manifest and compares it against
  the DERIVED recurrence count (computed from the live card the producer wrote,
  NOT the manifest's attested `recurrence_count`), FAILS-CLOSED at release time
  when the derived count exceeds the ack (an unacknowledged recurrence — a new
  observation must not slip through under a stale acknowledgement). The remedy is
  operator re-adjudication (bump the manifest's `last_acknowledged_count` to the
  current count), NOT a code change. An uncollapsed duplicate (≥2 cards sharing an
  effective recurrence id, a producer-dedup bypass) is also a release-blocking
  failure. Both are dormant when no release is imminent.

### Coordinator-adoption marker (defer-liveness opt-in, unmanaged)

`.vh-agent-harness/coordinator-adoption.json` is a schema-v1 marker
(`{"version":1,"adopted":true}`) attesting that this repo has adopted the local
coordinator transport — i.e. that at least one canonical coordinator task has
been written via any coordinator-task op. It cures the fail-open in doctor check
#12 (`checkDeferLiveness`, the §4.3 release gate): without it, the gate could
not tell "genuine greenfield, never adopted" (correct SKIP) apart from "adopted
then whole transport deleted" (the silent-pass-on-gitignored-`.local/` vector
that `38c5c477` cured for the disposition manifest). The marker is unmanaged —
template-less, never renderer-seeded, runtime-created — following the same shape
as the disposition manifest: committed channel meta-state, visible on a fresh
checkout, produced at runtime rather than seeded on install.

- **Producer (sole).** `updateCoordinationTask` in
  `.opencode/scripts/state-lib.js` — the single write chokepoint through which
  all seven public coordinator-task ops route (save / activate / ready /
  updateMetadata / repair / closeout / review) — creates the marker idempotently
  (create-if-absent, NEVER overwrite) after every successful canonical
  coordinator-task write. A WRITE is the act of adoption (matrix row-2
  semantics); mere file presence is not. It is NOT written by `/coordination`
  (read-only) or any other surface, and it is NOT renderer-seeded on install — a
  greenfield install stays unadopted until its first coordinator-task write fires.
- **Channel-liveness state matrix** (the gate's sole transition authority). The
  claims kernel (`internal/memory/claims`) only DERIVES the marker state
  (absent / valid / corrupt) and projects it into the liveness result; it does
  not choose the tier. `checkDeferLiveness` is the only place that maps state to
  a check tier:

  | `.local/coordinator/tasks/` | adoption marker | doctor #12 result |
  |---|---|---|
  | absent | absent | **SKIP** — greenfield, never adopted |
  | present | absent | **SKIP** — do not retroactively infer adoption from losable transport |
  | present | valid | run the existing gate (cards + CardErrors behave normally) |
  | **absent** | **valid** | **FAIL** — adopted transport lost (the fix) |
  | either | corrupt / unreadable | **FAIL** — fail-closed (mirrors CardError discipline) |

- **`.local/coordinator/tasks/` is NOT committed truth.** Card contents stay
  losable transport; the adoption marker is channel meta-state (legitimately
  committable; it does not promote transport identity to canon). This mirrors how
  the disposition manifest is the committed authority while `.local/` is
  provenance transport.
- **Accepted tradeoff.** Option A closes WHOLE-DIR loss but NOT selective
  single-card deletion (e.g. one errata card deleted while the directory
  remains). This is accepted because whole-dir loss is the structurally-forced
  gitignore vector, and an actor who can delete a single card can also delete
  the committed marker. The full rationale is recorded in
  `templates/migrations/v0.16.0.md`.
- **`/task-delete` is the sanctioned single-card removal wrapper.** It destroys
  one transport card and its local report directory (irreversible hard-rm). It
  does NOT route through `updateCoordinationTask` (the write chokepoint that
  creates the adoption marker) — deletion is an rm, not a write — so it neither
  creates nor removes the marker, and the "seven public coordinator-task ops"
  count above (the write-producers) is unchanged. The accepted-tradeoff row
  above (whole-dir loss detected, selective single-card loss not) is exactly why
  `/task-delete` ships as a validated, observable wrapper rather than a silent
  rm: it rejects wildcard/path-like/multiple-id/whitespace-separated input
  before any filesystem mutation, confines removal to the transport roots
  (physical, not lexical — symlink escapes are refused), surfaces the card to
  the operator before the destructive call, handles a degraded or malformed
  card without strict-validation blocking and reports its condition honestly,
  and refuses a `working`, `reported`, `blocked`, or `completed` card without
  an explicit `force` so it cannot bypass a pending review, closeout, or
  decision gate.

### Release ceremony: note → readiness artifact → manifest (releaser-owned)

The releaser agent owns this ceremony end-to-end. There is NO out-of-band
operator release-prep. The ceremony produces THREE sequential single-path
`committer` delegations — N (migration note), R (readiness artifact), and M
(manifest) — so that at tag time `HEAD = M`, `HEAD^ = R`, and `HEAD^^ = N`.
The release-tag wrapper's deterministic gates refuse the tag unless each
commit binds to its predecessor exactly. The wrapper also runs **G0c**
  (`vh-agent-harness doctor` — all 25 checks, including #12 defer-liveness,
  #13 staged-errata-content, #14 behavioral-closure, #15 dev-stale-embed,
  #16 f1-envelope, #17 f1-f2-consistency, #18 f2-pairs, #19 head-progress,
  #20 complexity-advisory, #21 recurrence-state, #22 exec-sandbox-floor,
  #23 shipped-pilots, #24 closeout-reach, and #25 rewrite-parity)
  as a hard machine gate
  AFTER the clean-worktree
gate (G0b) and BEFORE the readiness-pass artifact gate (G1-G5). A
non-HEALTHY doctor refuses the tag. This makes doctor a HARD ceremony stop,
not a human-remembered pre-flight. Push-only mode exits before G0c (the tag
already passed it at creation).

When invoked, the releaser:

1. **Step 3.1 — migration note commit N.** Authors
   `templates/migrations/v<next>.md`, then runs
   `vh-agent-harness release inject-errata --note templates/migrations/v<next>.md`
   (a clean no-op when no staged errata cards exist) to inject each `status:
   staged` errata card's correction body into the note as a
   `## Errata for v<version>` section and flip each card `staged → completed`
   (zero human copy-paste), and delegates a single-path commit to the
   committer (scope = that one path). N is HEAD after this step.
2. **Step 3.2 — readiness artifact commit R.** The parent orchestrator
   (`build` / `coordination` / `project-coordinator`) invokes
   `harness-release-readiness` against N; the readiness agent writes
   `.vh-agent-harness/release-readiness-pass.json` (its exclusive edit — the
   releaser cannot write it, and the releaser cannot invoke the readiness
   agent directly under `task: {"committer":"allow","*":"deny"}`). The
   releaser DISCOVERS the artifact read-only, VALIDATES that its `commit_sha`
   binds to N and all five model gates (G1-G5) report `ready`, then delegates
   a single-path commit R to the committer (scope = the artifact path), so
   `R^ == N` and `git diff --name-only N..R` is exactly the artifact path. R
   is HEAD after this step.
3. **Step 3.3 — manifest commit M.** Recomputes the release arc against R and
   rewrites `.vh-agent-harness/release-defer-dispositions.json`'s three
   handshake SHA fields — `evaluated_commit` AND `manifest_parent_commit`
   (both = full SHA of R), and `evaluated_tree` = `tree(R)`. The releaser does
   NOT touch `release_base`, `records[]` dispositions, or any other
   operator-attested field. Delegates a single-path immediate-child commit M
   of R to the committer (scope = the manifest file), so `M^ == R` and
   `git diff --name-only R..M` is exactly the manifest path.
4. Re-runs the evaluator against M to confirm the handshake passes.
5. Invokes the wrapper to tag M as the release.

The operator's release-time role is reduced to: confirming an override when an
`override_required` record exists (see Wrapper flags below). The first release
after the seed manifest lands is the canonical case — the seed manifest's SHAs
are placeholders that the releaser recomputes against the actual post-artifact
commit R on its first run.

### Recovery: ceremony refusals (operator-only git recovery)

The release-tag wrapper refuses the tag BEFORE any `git tag` mutation, emitting
a structured JSON refusal and a non-zero exit. Two refusal classes need
operator git recovery that the agent CANNOT perform itself — shell-guard's
`git-mutation-bypass` rule denies `git reset`, `git branch`, and
`git cherry-pick` to every agent (the gated-commit `committer` only stages and
commits an authorized slice; it does not reset or cherry-pick). On either class
the agent's correct role is **STOP-AND-ASK** the operator with the verified
chain below — never author a recovery that leaves untracked files in the
worktree or relies on the reflog in place of a branch ref.

- **Failure mode A — mis-authored M.** M's handshake SHAs are wrong, or the
  `records[]` block is malformed/unsorted, so the evaluator returns
  `evaluator-error` (exit 2). The override ceremony CANNOT cure this class;
  M must be re-authored.
- **Failure mode B — a concurrent commit landed above M.** A commit outside
  the N→R→M chain now sits at HEAD above M, breaking the wrapper's readiness
  binding (at tag time M must be HEAD, R = HEAD^, N = HEAD^^). The handshake
  no longer holds and the wrapper refuses.

**The G0b untracked-file trap (applies to both).** G0b runs
`git status --short` (release-tag.sh:558) and refuses on ANY non-empty output.
`git status --short` COUNTS UNTRACKED files — they appear with a `??` prefix —
not just modified/staged ones, so the worktree can be clean of *modifications*
yet still fail G0b on untracked files. **Distinguish untracked from modified
before acting:**

- `git status --short` lists untracked files with a leading `??`; modified
  files show ` M` / `M ` / `MM`. Only the latter are real edits.
- `git status --short --untracked-files=no` isolates the *modified* check
  (it shows both staged and unstaged tracked changes; do NOT substitute
  `git diff --quiet`, which exits 0 on a purely staged change while G0b still
  refuses). If `--untracked-files=no` output is empty but plain
  `git status --short` is not, the entire G0b refusal is untracked-file noise,
  not an uncommitted edit.

**Operator-verified recovery sequence (v0.21.0 reference; operator-only):**

1. **Pin any concurrent commit with a real branch ref BEFORE reset** —
   `git branch keep-<id> <sha>`. A branch ref is GC-proof; the reflog is NOT
   (it expires and is not a durable anchor), so never substitute reflog
   inspection for a real branch ref. This preserves the concurrent commit for
   re-application after the tag. NOTE: a branch ref preserves ONLY the named
   commit — it does not capture uncommitted index/worktree state, which step 2
   handles separately.
2. **Preserve any uncommitted tracked WIP before the hard reset.** Because
   `keep-<id>` holds only the concurrent commit, run
   `git status --short --untracked-files=no` and stash or branch any tracked
   modifications that are NOT part of that commit (e.g.
   `git stash push -m "ceremony-recovery-wip"`, which captures both staged and
   unstaged tracked changes). The `git reset --hard` in the next step would
   otherwise discard them irreversibly. (Untracked scratch is backed up in
   step 4.)
3. **Reset to R (hard)** — `git reset --hard <R-sha>`. HEAD, index, AND worktree
   all move to R; the concurrent commit's tracked changes are discarded from the
   worktree, preserved in the pinned `keep-<id>` ref (which is why steps 1 and 2
   are mandatory). Use `--hard`, NOT `--mixed` — `--mixed` resets only
   HEAD+index and leaves the concurrent commit's tracked changes in the worktree,
   so `git status --short` stays non-empty and G0b still refuses at tag time.
4. **Remove untracked files from the worktree** so G0b sees a clean tree
   (`git reset --hard` resets tracked files but does NOT delete untracked
   files). Back up genuinely-untracked scratch first; tracked-concurrent content
   is already safe in the pinned branch. This step resolves the G0b
   untracked-file trap from above.
5. **Commit M(good) as an immediate child of R** — manifest path only — via
   the gated-commit `committer`, so `M^ == R` and `git diff --name-only R..M`
   is exactly the manifest path. (For failure mode A, this is also where the
   mis-authored M is corrected before re-commit.)
6. **Re-verify the handshake** — re-run the evaluator against M; confirm
   `evaluated_commit` / `manifest_parent_commit` == SHA(R) and
   `evaluated_tree` == tree(R).
7. **Tag** — invoke the wrapper; with a clean worktree G0b passes and the
   ceremony completes.
8. **AFTER the tag, cherry-pick the pinned branch back onto the integration
   branch and delete the temp branch** — `git cherry-pick keep-<id>` then
   `git branch -D keep-<id>` (single-commit concurrent work; a multi-commit
   branch would need a range). If step 2 stashed WIP, restore it
   (`git stash pop`). The release tag sits immutable on M, with the concurrent
   work reapplied above it.

**Agent role.** Every git step above (reset, branch, cherry-pick) is
operator-only. On a ceremony refusal in either failure mode the agent
STOP-AND-ASKs the operator with this verified chain — it does not run the
recovery, does not author a variant that leaves untracked files in the
worktree, and does not rely on the reflog in place of a branch ref. The G0b
global-clean requirement at release transitions is correct and load-bearing
(see the AGENTS.md B2 release-cleanliness rule); the gap this runbook closes is
the UNDOCUMENTED recovery path, not the gate.

### Published-tag immutability (never move a published version)

A successfully-published `vX.Y.Z` tag is **immutable**: the annotated tag object
AND the commit it dereferences retain the same SHAs for the life of the version.
The release-tag wrapper enforces this by REFUSING the create flow when
`refs/tags/<version>` already exists (release-tag.sh:232-238) — it never moves,
replaces, or force-updates a published tag. There is **no**
`--force` / `--move` / `--move-published-tag` mode; every such verb is rejected
as an unknown argument (release-tag.sh:287-290). This is the "never move a
published version" policy.

**Correction is forward-only.** A defect in a published release is fixed by
shipping a **successor version** (`vX.Y.(Z+1)`, or `vX.(Y+1).0`) through the
full N→R→M + G0-G7 ceremony — never by rewriting the existing tag. The
published tag stays as the historical record; the successor is the correction.

Policy contract (7 points):

1. **Published-tag immutability.** The annotated tag object and its
   dereferenced commit are immutable once published.
2. **Forward-only correction.** A defect ships as a successor version through
   the full release ceremony (note → readiness artifact → manifest + G0-G7).
3. **No wrapper move mode.** The wrapper ships no `--force` / `--move` /
   `--move-published-tag` branch and never will.
4. **`--push-only` is publication-only.** It pushes an already-cut annotated
   tag to origin; it is NOT a replacement, deletion, or force authority.
5. **Unpublished-local cleanup is the operator's.** If a tag was cut locally
   but never published, the operator may remove it (`git tag -d <version>`) and
   re-run the wrapper. This does NOT authorize agents to use raw `git` —
   shell-guard's `git-mutation-bypass` rule still denies tag mutation to every
   agent.
6. **Published-reference surgery is a separately-authorized incident.** Moving
   or deleting an ALREADY-PUBLISHED tag (e.g. a hostile or compromised tag) is
   an operator/platform incident with its own authorization, NOT routine
   wrapper functionality. The wrapper provides no path for it.
7. **No misleading manual escape hatch.** The `git tag -d <version>` + re-run
   recipe is valid ONLY for an unpublished LOCAL tag. It must never be
   documented or implied as a way to "correct" a published version — that is
   published-reference surgery (point 6), not a cleanup.

The regression tests in `internal/cli/release_tag_move_policy_test.go` pin both
refusal paths (existing-tag create-flow refusal + no-move-mode), so a future
edit that adds a move branch or weakens the existing-tag check fails before it
lands.

### Wrapper flags

```sh
vh-agent-harness exec bash -c 'RELEASE_TAG_MESSAGE_FILE=tmp/release-tag-msg-<version>.txt \
  scripts/release-tag.sh <version>'
```

- **`RELEASE_TAG_MESSAGE_FILE=<path>`** (env) — annotated-tag message file
  (required for the create flow; not required for `--push-only`).
- **`RELEASE_TAG_PUSH=1`** (env, optional) — push the tag after creating it.
- **`--push-only`** (positional flag, optional) — push an already-cut local
  tag to origin through the sanctioned wrapper, removing the need for agents
  to fall back to raw `git push` (forbidden by the `git-mutation-bypass`
  rule). Invocation: `scripts/release-tag.sh <version> --push-only`. The tag
  MUST already exist locally (cut by a prior create-only invocation) AND must
  be an annotated tag object; if missing, the wrapper refuses with `"tag <v>
  does not exist; cannot push-only"` (prefix of the full stderr line, which
  also names the remedy — operators grepping logs should prefix-match). If the
  tag exists but is a lightweight tag (`git tag <v>` with no `-a`), the
  wrapper refuses with `"... is not an annotated tag object; push-only
  requires an annotated tag ..."` — a lightweight tag never passed the full
  ceremony and would defeat the annotated-tag invariant. In push-only mode
  the wrapper inverts the tag-existence check (requires the tag to already
  exist), skips `RELEASE_TAG_MESSAGE_FILE` validation, the override ceremony,
  the DEFER gate, and the `git tag -a` mutation — it goes straight to
  `git push origin <version>` and emits the same JSON contract with
  `disclosures:null` and `accepted_overrides:null` (the DEFER gate already
  passed at tag-creation time; push-only trusts the existing annotated tag
  object). The JSON `commit` field carries the tag's dereferenced target
  commit, which may differ from the caller's current HEAD (the create flow
  emits HEAD; push-only emits what the tag points at). Cannot be combined
  with `--override-*` flags (the DEFER gate is skipped, so there is nothing
  to override). Use this for a push-only slice after a tag was cut by an
  earlier create-only run.
- **`--override-release-version <vX.Y.Z>` + `--override-manifest-sha <blob-sha>`**
  (both required together) — explicit noninteractive override confirmation.
  The operator is the ONLY transition authority for an override; the releaser
  forwards operator-confirmed values only and never invents them. Exact 3-way
  agreement is required: `--override-release-version` == the version being
  tagged == the `override.release_version` recorded in the manifest, AND
  `--override-manifest-sha` == the actual git blob SHA of the committed
  manifest. Overridden findings DO appear in release notes, wrapper output, and
  CI (default adopted). An override CANNOT cure schema/staleness/ancestry/
  malformed failures.

On any refusal (blocker, evaluator-error, override-ceremony mismatch,
handshake mismatch, `--push-only` against a missing tag, `--push-only` against
a lightweight (non-annotated) tag, or `--push-only` combined with
`--override-*`) the wrapper emits a structured JSON refusal and exits NON-zero
BEFORE any `git tag` or `git push` invocation. There is no fallback to raw git.

When a command prints a "Next steps" footer, follow it. When unsure, re-run
`vh-agent-harness guide`.
