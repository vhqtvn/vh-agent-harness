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
   the full per-file plan (overwrite / unchanged / seed / preserve / reconcile / conflict)
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
| `.vh-agent-harness/project.config.json` | Fill `project.mission_summary` + `architecture_summary` (and `db_user`/`db_name` if used). Resolved into the seeded `CLAUDE.md`/`Makefile` at install — **create + fill it BEFORE `install`** (those seeds are written once). A field that does NOT apply (e.g. `db_user`/`db_name` when there is no database) may be set to a blessed N/A sentinel — `none` / `n/a` / `null` / `na` (case-insensitive; string form only — write `"null"`, not bare JSON `null`): it renders empty and silences the `token(s) UNRESOLVED` warning for that field. |
| `.vh-agent-harness/AGENTS.mission.md` | Write the project's domain mission/architecture/rules; composed into root `AGENTS.md` on `update`. |
| `.vh-agent-harness/vh-harness-profile.yml` | (armed, seeded) Select features + `overlays: [<pack>]` (S3). |
| `.vh-agent-harness/run-shape.yml` | (seeded host-shell) Set runtime `backend:` (`host-shell`/`docker_compose`/`proxy`) + `compose_file`/`default_service` or `proxy_command`; lifecycle hooks/verbs (S4). |
| `.vh-agent-harness/harness-ownership.yml` | (optional; not seeded) Raise-only ownership overrides — create only to take a managed file to `project_owned`. |
| `.vh-agent-harness/overlays/<pack>/` | Project overlay: `agents/`, `commands/`, `skills/`, `opencode-append.jsonc`, `permission-pack.jsonc`, `callable-graph-snippet.md`. |
| `.opencode/repo-configs/forbidden-patterns.project.js` | (seeded blank) Project deny-rules (import builders from `forbidden-patterns.core.js`; each rule needs a `why`). |
| `.vh-agent-harness/config-transform.mjs` | (seeded blank) Project permission transform (F-intent). Returns typed `permissionPatches` merged into every render. NO raw config mutation. See "Permission transform (F-intent)" below. |
| `.vh-agent-harness/config-transform.core.mjs` | (regenerated) Harness-owned types + Decision constants + builder helpers for the transform. Import, do not edit. |
| `.opencode/plugins/compaction-primitives.project.md` | Project compaction-recovery block (operational primitives an agent needs after context loss). |
| `docs/coordination/LANES.yaml`, `docs/coordination/ROLES.md` | Coordination lanes/roles — define project-specific ones or keep the generic set. |
| `.local/cleared-assumptions.yaml` | Operator-state ledger of cleared assumptions (usually operator-maintained). |

Do **not** edit: `lineage.yml` (binary-owned), `AGENTS.core.md` (managed
compose source), or anything under `.opencode/` that is platform_managed.

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

### Shipped overlay packs

Besides project packs you author under `.vh-agent-harness/overlays/`, two
overlay packs ship **embedded in the binary**, selectable by name with no
vendoring:

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

Each renders into `.opencode/` on `update` exactly like a project-local pack,
and each is opt-in (a `minimal` profile that never names it renders nothing of
it). A project-local pack of the same name still shadows the embed wholly.

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
    the only ALLOWED sed form is `sed -n --sandbox …`. (Binary-level heuristic
    denylist of the known prompt-free non-git write/exec vectors; a per-binary
    safe-flag allowlist is deferred and the OS-level exec-sandbox is the
    authoritative layer for the long tail of unknown flags ON THE HOST-SHELL
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
    classification, ALLOW if the verb is read-only.
  - exec-ro **executes the command exactly as given or DENIES** — it never
    rewrites the command. On DENY it prints a human-readable notice to stderr and
    exits non-zero (no prompt, since the outer invocation is allowlisted).
  Use exec-ro when an agent wants prompt-free read-only inspection (git or
  non-git). Use `exec` for anything mutating, anything with shell plumbing, or
  anything exec-ro's allowlist does not cover.
- **Run a command under a kernel-enforced HOST-LOCAL Linux sandbox:** `vh-agent-harness exec-sandbox [--sandbox=off|best-effort|strict] [--net=deny|allow|ask] -- <cmd>`.

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
- **Track work:** `docs/planning/backlog.md` is the canonical task-status source
  of truth (seeded on install, `project_owned`). Agents edit it **freely** under
  the hybrid split-commit discipline: re-read from disk before editing, edit
  only your own task rows (stable IDs), and **commit backlog separately from
  code** so a concurrent backlog edit can't `cas_conflict` your code commit. On
  `cas_conflict`, re-read from the new HEAD, re-apply your row, and retry — **do
  NOT revert `backlog.md`** (that discards a collaborator's update); in
  particular, `commit-gate.sh revert docs/planning/backlog.md` is the
  blind-revert anti-pattern on the ledger. Load the `backlog` skill before
  editing. DEFER / p2 / follow-up findings route to the holding area
  (`.local/coordinator/tasks/` via `/write-task` with Notes provenance),
  never directly to a backlog row; the promoter promotes them only after the
  predicate checker (`.opencode/scripts/check-defer-triggers.js`,
  promoter-use-only, never blocking) confirms the trigger and the Definition of
  Ready is met. Run `/backlog-cleanup` (or `vh-agent-harness exec node
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
- **Refresh after a new binary or config change:** `vh-agent-harness update`
  (preview with `--dry-run`). Armed-file conflicts are recorded — list them with
  `vh-agent-harness proposals`.
- **Inspect migration notes for a release:** `vh-agent-harness help migrate`
  (the note for the locally adopted harness version, detected from lineage) or
  `vh-agent-harness help migrate vX.Y.Z` (a specific release). With no version
  and no local install, it prints the latest bundled note. It is **documentation
  only** — it never modifies files.
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
- **Verify:** `vh-agent-harness doctor` (lineage, armed-schema, managed-drift,
  overlay-perm, environment, config-refs, gitignore, auto-classifier). The
  `auto-classifier` check lints the shape (field set + types + enums) of the
  auto-classifier-pilot overlay's config files when present — a present-but-invalid
  `auto-gate-config.json` / `auto-gate-llm.json` FAILs; absent configs are never
  failures (defaults apply). `vh-agent-harness diff` shows drift vs. the corpus.
- **Package a bug bundle:** `vh-agent-harness diagnostics-export [--dry-run]
  [--output <path>]` bundles selected harness state (`.opencode/state/`,
  `.local/coordinator/`, `.local/config/`, `docs/checkpoints/`) into a
  field-aware-redacted `tar.gz` under repo-scoped `tmp/`. It is **never
  auto-uploaded** — the operator decides if/when to share. Run `--dry-run`
  first to review the manifest and redaction counts. See the `diagnostics-export`
  skill for the operator review checklist.

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

## Preserved orphan overlay skills (report-only)

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
The notice is informational; **nothing is deleted.** To actually remove a
preserved orphan, delete the **file** named in the notice (e.g.
`rm .opencode/skills/<name>/SKILL.md`). Remove the **whole** skill directory only
after verifying that **every** file inside it is orphaned — a directory may mix
orphaned files with files still being actively rendered. Or restore the overlay
source to clear the notice on the next update.

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
`manifest_version` field, written atomically only after a successful non-dry-run
`update`/`install`. It records each overlay-rendered skill file's destination,
producing pack, source-relative path, and content digest. If the manifest is
absent (a project first updated by a pre-v0.10.0 binary), the first `update`
establishes a baseline from the current render — it does NOT retroactively
adopt pre-existing `.opencode/skills/` dirs as managed (there is no historical
proof they were overlay-rendered).

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
