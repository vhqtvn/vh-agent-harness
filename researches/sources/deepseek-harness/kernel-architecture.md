Durable home: researches/sources/deepseek-harness/kernel-architecture.md (landed 2026-08-17).

# DeepSeek Harness (dsh) — Kernel Architecture (Source Packet)

## Provenance

| field | value |
|---|---|
| Upstream | `deepseek-ai/deepseek-harness` ("dsh") |
| Local checkout | `refs/deepseek-harness/` (read-only; never modified) |
| Commit | `47f943859bef60e4160492346772ded9b24f765a` |
| Checkout date | 2026-08-13 |
| Slice | kernel-architecture — `packages/{core,boot,host,api,context,extensions,hooks,bundle,preset,util}` + the architecture docs set |
| Method | static reading of the local checkout only; no upstream web re-verification (offline evidence pass) |
| Audience | vh-agent-harness maintainers mining dsh for harness-design ideas |
| Packet type | `sources` — evidence transcription, not a decision memo |

Citation convention: all paths below are relative to `refs/deepseek-harness/`
(e.g. `docs/architecture.md` means `refs/deepseek-harness/docs/architecture.md`).

This packet transcribes a completed prior read pass (~35–40% slice coverage,
weighted toward `packages/core` and `docs/`), then folds in a bounded top-up
pass (README-level reads of `boot/app-boot`, `bundle/base`, all three listed
`extensions`/`hooks`/`host` targets, and `core/agent-loop`), lifting coverage
to roughly 50% of the slice by directory. Claims are confined to the evidence
base plus verified top-up reads; anything beyond is marked `inference`.

## Orientation — what dsh is

dsh is an agent harness built as a **pnpm monorepo** on the **Cordis plugin
framework**, with a strict "everything is a plugin" doctrine:

- **No privileged core.** The kernel makes no feature exceptions for itself;
  every capability (agent loop, tools, prompts, host services) is a plugin
  mounted into the runtime. The standing extension rule is **mount beside,
  never patch** — you extend behavior by registering a new plugin next to the
  existing one (shadowing/intercepting it through kernel primitives), not by
  forking or editing it. Source: `docs/architecture.md` → "Cordis"; root
  `AGENTS.md` ("DeepSeek Harness is a plugin-based agent harness on vendored
  Cordis: everything is a plugin"; "Plugins, not loop changes: new behavior
  goes on documented extension points; changing `agent-loop` requires
  updating docs/architecture.md").
- **Cordis is the kernel.** The vendored Cordis (see below) supplies `Context`
  — a **service-repository proxy**. Plugins call `ctx.extend()`,
  `ctx.isolate()`, `ctx.intercept()` to create scoped child contexts, and the
  low-level service surface is `get/set/provide/accessor/mixin`
  (`docs/cordis-api/context.md`; `vendor/cordis/src/context.ts`).
- **Two compiler faces.** The repo typechecks and builds a *host face* and a
  *client face* as separate aggregates, because both faces declaration-merge
  the Cordis `Context` type under the same keys. `tsconfig.host.json` /
  `tsconfig.client.json` define the two aggregates over shared bases
  (`tsconfig.base.json`, `tsconfig.base.client.json`); `tsdown.config.ts`
  selects the face at build time via the `DSH_BUILD_FACE` env var.
- **Vendor-as-fork.** 9 pinned Cordis packages are vendored under `vendor/`,
  rescoped to `@deepseek-ai/*`, with an 18-entry local-modification log in
  `vendor/README.md` (read in full, 60 lines incl. the log). This is the
  mineable "hardening checklist" — see Cross-cutting mechanisms.
- Root orientation fully read: `AGENTS.md`, `pnpm-workspace.yaml`, all four
  face/base tsconfigs, `tsdown.config.ts`, `vendor/README.md`. The root
  `AGENTS.md` package-group map (surfaced again during top-ups) also names the
  slice's unexamined groups: `api/` = Remote BFF assembly + Typert RPC
  gateway, `context/` = request-context plugins, `preset/` = per-session
  agent composition from preset cordis.yml files, `util/` = zero-dependency
  utilities.
- **Pre-release stance** (root `AGENTS.md`): "foundation over blast radius" —
  no external consumers yet, rename/repackage freely; on-disk formats carry no
  compatibility promise (`SESSION_FORMAT_VERSION` pinned at `0`).

The docs set read in full: `docs/architecture.md`, `docs/cordis-primer.md`,
`docs/glossary.md`, `docs/event-producer-consumer.md` (complete 56-event
producer/consumer matrix), `docs/rescope.md`, `docs/graph-atlas.md`, all 6
`docs/cordis-api/*.md`, all 8 `docs/cordis-tutorial/*.md`.
`docs/module-graph.md` and `docs/config-catalog.md` were read structurally
only (headings/shape, not content-verified).

## Per-package deep dive

### `packages/core` (~75% touched; 8 subpackage `package.json` files read)

#### `core/scope` — examined (complete read of `src/index.ts`)

The scope primitive underpinning per-agent isolation:

- **ScopeKey is opaque and identity-compared.** Consumers never inspect scope
  internals; a live agent *is its own key* (the runtime registers effects
  bound to the agent instance itself).
- Two binding functions anchor the model: `bindScopeParent` / `scopeTarget`
  (`packages/core/scope/src/index.ts`).
- **Registrations inherit DOWN the parent chain** — a registration on a parent
  scope is visible to child scopes.
- **Event admission extends UP the chain** — an event emitted in a child scope
  can be admitted/observed by registrations up the parent chain.
- **Shadowing is most-specific-wins**: the registration closest to the
  emitting scope takes precedence.
- Terminology cross-ref: `docs/glossary.md` → "agent-scope".

#### `core/session` — partial (README complete; `src/index.ts` partial: event declarations, header validation)

Event-sourced session model:

- The session is an **append-only `SessionEvent` log**; nothing mutates
  history. `deriveMessages()` is the *projection* that derives the model-visible
  message history from the event log (`packages/core/session/README.md`,
  `src/types.ts`).
- `SessionEventMap` is **merge-extensible** across packages: new plugins can
  declare new event types without touching the core map. Root `AGENTS.md`
  sharpens the envelope: a `SessionEventMap` member is **required-on-read by
  default** — builds that do not know its type refuse the log unless the event
  carries `ignorable: true`; only structural format changes bump
  `SESSION_FORMAT_VERSION`.
- **Runtime invariant: model-visible ⟺ logged.** Root `AGENTS.md`: "anything
  that reaches a model request must be reconstructable from the session log;
  a new model-visible input requires a session event." Also stated in
  `packages/core/session/README.md` and `docs/architecture.md`.
- **Split, ordered teardown lifecycle** (from the partial `src/index.ts` read):
  teardown proceeds through `prepare`/`enter`/`announce` primitives, and
  `fork()` only cuts at **stable turn boundaries** — never mid-turn.
- Concrete persistence (top-up, `bundle/base`): JSONL under
  `dshHomePath('sessions')` via `dsh-session-persistence-jsonl`; durable image
  bytes live **outside** the append-only log — messages keep content-addressed
  references resolved by `dsh-attachment-local`.

#### `core/agent` — partial (`src/runtime-types.ts` complete; `src/index.ts` partial: AgentRegistry)

- **AgentRegistry is the agent-lifecycle seam.** It is built on an
  **`AgentFactory`** interface — the *loop* package is what actually
  implements `create`/`resume`; core/agent only owns registration and lookup
  (`packages/core/agent/src/index.ts`). Top-up confirmation:
  `core/agent-loop` README — "AgentLoop also implements the `AgentFactory`
  contract and registers itself via `ctx.agents.setFactory(this)`".
- **Initiator scope via `AsyncLocalStorage`**: `withInitiator` /
  `withoutInitiator` establish which agent initiated the current async
  operation, so nested calls can be attributed without threading a parameter.
  `packages/AGENTS.md` codifies the derived rule: "Initiator-owned private
  chains derive, then capture" — recover the Agent at each orchestration
  entry, derive `agent.session`, keep `Agent`/`Session` explicit at lifecycle
  boundaries.
- **`ctx.agent` is a DX accessor defaulting to `undefined`** — dereferencing
  `.agent` outside an agent context is a typed absence, not an exception.
  (Related convention from `packages/AGENTS.md`: optional services are read
  with `ctx.get(name)`; the `ctx.<name>` property proxy is
  topology-sensitive.)
- **Typert wire-lookup registration** — agent types are registered so they can
  be looked up across the host/client wire by name.
- `src/runtime-types.ts` (complete) defines the runtime-facing agent types.

#### `core/tools` — partial (`src/index.ts`: events, `ToolDefinition`, `Config`, `ToolLayer`)

The tool execution pipeline is a fixed sequence of observable events:

```
tools/pre-execute  (allow/deny/ask waterfall — policy gate)
      ↓
  ToolGuards       (tools.guard() — monotonic owner policy)
      ↓
tools/execute      (around-waterfall with signal re-fusing)
      ↓
tools/post-execute → finalizeContent → tools/result
```

- Pipeline order and roles corroborated by a second source (top-up,
  `core/agent-loop` README): "the guarded `tools/pre-execute` →
  `tools/execute` → `tools/post-execute` → definition-owned
  `finalizeContent` → `tools/result` pipeline"; "Sandbox, permission, plan
  mode: `tools/pre-execute` for extensible deny/ask, `tools.guard()` for
  monotonic owner policy, `tools/post-execute` for result decisions, and
  `tools/result` for final observation."
- `ToolDefinition` carries more than input schema: `output.schema`, a **pure
  `render`** function, `finalizeContent`, `isConcurrencySafe`, `timeoutMs`,
  and `presentCall` — presentation is part of the tool contract
  (`packages/core/tools/src/index.ts`; root `AGENTS.md`: "A tool's UI render
  intent is part of its design, decided up front; presentation methods are
  pure functions of `args`").
- **Code mode** collapses the whole tool surface to a single `run_code` tool
  plus generated TS/Python SDKs — tools become library calls inside generated
  SDK code rather than individually exposed JSON-schema tools.

#### `core/system-prompt` — partial (`src/index.ts`: events, sections, config)

Ordered, layered prompt assembly:

- **Numbered sections**: `-100` = harness identity, `0` = persona, `100–199` =
  tool guidance. Section numbers create a stable total order across plugins.
  (Top-up example: `boot/app-boot` registers a `harness:source` section
  "ordered just after the harness identity, before the persona".)
- **Dynamic `PromptContext` snapshots are durable user-role messages** —
  dynamic context is materialized into the log, honoring the session
  model-visible ⟺ logged invariant.
- **Strict `{{variable}}` interpolation** — interpolation failure is an error,
  not silent passthrough. (Top-up: the loop "supplies `provider`, `model`,
  and `cwd` variable values but no additional fixed prose" — variables come
  from the loop plugin, identity/persona from `dsh-system-prompt`.)
- **`toolOrder` with a mandatory `<unlisted-tools>` marker** — the prompt must
  account for every tool, including the ones deliberately not listed.
- **Complete-section override** and **`PERSONA_SECTION` shadowing** — presets
  override the persona section by shadowing the same section key rather than
  by editing the base prompt.

#### `core/agent-loop` — partial (README complete, 134 lines; `src/` unread)

**The only package in the harness that contains concrete loop logic.**
Everything else is an abstract service or a plugin against extension points
(`core/agent-loop/README.md`).

- **Creation/resume is one rollback-covered transaction**: construct a private
  session, concrete agent, and scoped context → await optional setup → enter
  both registries → announce `session/created` then `agent/created` → emit
  `agent/session-start` → only then start the driver. An `AbortSignal`
  cancels only load/setup/publication and is detached before the returned
  handle becomes visible.
- **Co-ownership + quiescence**: the caller fiber and the AgentLoop provider
  co-own the agent; caller unload, handle disposal, and provider unload
  converge on one memoized quiescence boundary — "no continuation can publish
  after dependencies disappear". Same-id arbitration: concurrent creates may
  both prepare, the final `enter()` calls arbitrate, losers roll back private
  resources; each detach is bound to the exact entered object so a stale
  disposer cannot remove a later same-id replacement. Teardown order: stop
  and drain → unwind scope → detach agent → detach session.
- **Implements `AgentFactory`, self-registers via `ctx.agents.setFactory(this)`**
  — plugins create/resume agents through `ctx.agents.create({sessionId,
  meta?, seed?, agentOptions?, setup?, signal?})` / `ctx.agents.resume({…})`.
  Injects all five interface services: `agents`, `sessions`, `llm`, `tools`,
  `systemPrompt`.
- Driver internals (`ReactLoopAgent`, inbox, run controls) are package-private
  — no `./src/*` export escape hatch. The unified `send()` primitive routes
  by (`target` × `wakeup`) with fixed presets: `followup` (next-turn FIFO,
  wakes), `steer` (next-step inbox, wakes), `inject` (same inbox, no wake).
  At a turn boundary the driver opens the durable turn, then atomically
  claims pending next-step input plus one queued prompt; `agent/pre-step`
  returns either rejection or the complete messages entering the step.
- Every inbox mutation publishes a normalized `agent/inbox/spliced` event
  **before** changing the live projection (log-then-derive); every successful
  provider call appends exactly one `assistant/message` completion anchor
  (assembled content + exact chunk seqs in `sourceEventSeqs` + usage; empty
  content stays out of derived history).
- `ctx.llm.prepareCall()` validates adapter-owned fields and materializes
  defaults; the prepared call retains the exact adapter registration across
  async resolution "so HMR cannot mix one adapter's capability result with
  another adapter's request"; adapter-marked defaults are stripped before the
  next waterfall so each route re-materializes its own.
- **Plugin failure ends the current turn, not the loop.** Recovery runs on
  `agent/request-error`: a handling listener returns `{kind: 'retry'}` (e.g.
  `dsh-llm-retry`), an unhandled failure is terminal. One cancellation signal
  per admission/turn; wakes landing after abort are latched and replayed at
  the driver's convergence boundary; undispatched tool calls receive
  synthetic `tool/call` + `ABORTED_BEFORE_DISPATCH` result pairs.
- **Parallel tool calls**: exclusive calls form barriers; parallel-safe calls
  use a bounded rolling pool (`maxParallelToolCalls`, default 10) and are
  reclassified before start; only dispatch/body overlaps — policy, durable
  results, and result context remain model-ordered. Classification is unary
  (no sibling comparison), so safety-dependent-on-siblings calls stay
  exclusive.
- Plugin-owned concerns: compaction (`agent/pre-step` pressure +
  `agent/request-error` repair), recovery, sandbox/permission/plan mode (the
  tools pipeline above), sub-agents (`ctx.subagents` providers; in-process
  ones use `ctx.agents.create()` + owned `AgentHandle` teardown),
  persistence (eager write-behind from `session/event`; `session/flush` is an
  explicit observation barrier), UI (`session/event` + `agent/*`).
- No built-in turn budget — runaway turns must be bounded by a plugin
  cancelling from an existing extension point such as `agent/turn-stopping`.

#### `core/agent-default-model` — skipped (partially informed: `bundle/base`
patches it with default `provider: deepseek-official`, `model:
deepseek-v4-flash`; "Settings may supply a saved selection; consumers read it
at creation time").

#### `core/agent-tool-presentation` — skipped

### `packages/boot` — partial: `app-boot` README complete (60 lines); `cmdline` skipped

Shared boot glue for the app bins — each bin is a thin self-executing
composition over these helpers, "so loader-failure behavior has one owner
instead of drifting between published artifacts":

- **Fail-loud boot**: `installFailLoud` turns an unhandled boot or Loader
  rejection into one labelled stderr line + `exit(1)`, awaiting an optional
  terminal-restore `release` hook bounded by `FAIL_LOUD_RELEASE_TIMEOUT_MS`;
  the handler stays installed and latched during teardown (first rejection is
  reported, later ones — teardown's included — are swallowed). `boot()`
  itself disposes the partial context and rejects with a labelled error.
- **Startup audits**: `assertEntriesLoaded` rejects when a settled tree holds
  an enabled entry with no fiber, naming every unresolved plugin;
  `assertEntriesActivated` additionally awaits every enabled entry and
  reports each failure's original stack or each pending entry's unresolved
  services. Because the Loader mounts entries concurrently, a
  terminal-owning surface must get teardown before exit — otherwise raw mode,
  bracketed paste, and the keyboard protocol stay set on the user's shell.
- **Profile machinery**: a profile is `$DSH_HOME/profiles/<name>` holding a
  `package.json` (out-of-tree plugin `dependencies` + the `dsh.profile`
  manifest with its ordered `bundles` list) and the user's
  `cordis.patch.yml`. A bundle is an npm package declaring `"dsh": { "bundle":
  { "patch": "./cordis.patch.yml" } }`; `loadProfile` resolves each bundle
  name two-anchored (the dsh installation first, then the profile directory)
  and fails loud on a listed package without a bundle declaration.
  `PROFILE_TEMPLATES` (`web`, `headless`) auto-initialize on first use.
- **Composition cannot drift from boot**: `composeEntries` applies patch
  layers over an empty entry list through the include's own
  `applyEntryPatches`; `renderConfigDump` renders `--dump-config` offline
  with the include's own parser and patch algorithm, "so the result equals
  what `boot()` mounts" (rows grouped under `# ==` source-file comments; a
  patch matching no row warns and is skipped).
- **Live patch layer**: `watchUserPatches` registers the user patch file with
  the Cordis HMR service; every add/change/removal **transactionally
  recomposes the full patch list** and returns an async disposer; a failed
  read/parse leaves the last good tree running and broadcasts
  `hmr/config-update-failed(filename, Error)`.
- **Env + patch semantics**: `.env` layering is inherited env > invoking-dir
  `.env` > Harness-home `.env` (frozen snapshot; bootstrap-only file
  variables rejected). A user id-targeted patch **replaces the matched row's
  whole `config`** (no deep-merge); per-profile patch applies before the
  home-level patch (home therefore outranks it); an empty or comments-only
  patch file **throws** (disable the layer with `[]`).
- `addHarnessSourceSection`: a `harness:source` global prompt section (just
  after harness identity) telling the agent the on-disk path of the DSH
  implementation checkout while warning it to use `pwd` instead of inferring
  cwd from that path; registered against the systemPrompt fiber, so a dev HMR
  reload of the system prompt drops it until the next boot.
- `cordis:group` is registered beside `cordis:include` so a composition can
  give **one `isolate` realm to a provider and its consumers together**;
  bare npm specifiers resolve two ways (config dir by default;
  `bareModuleBaseUrl` pins them to the installed host).

### `packages/host` — partial: `plugin-inventory` + `webserver` READMEs complete; `apiproxy`, `directory-picker{,-auto,-browse,-native}`, `frontend-static` skipped

- `host/plugin-inventory`: **read-only Host projection of the current Cordis
  Loader tree**. Registers the `pluginInventory` service and one generated
  direct Remote, `pluginInventory/list`; every call reads
  `ctx.loader.entries()` directly, skips structural group rows, and returns
  entries in Loader order with id, module specifier, effective enablement,
  and current root fiber phase (`pending|loading|active|failed|unloading`,
  `null` when no live root fiber). Deliberately point-in-time: no cache,
  history, provenance, or mutation path; Remote-only, no same-process
  `Context` merge.
- `host/webserver`: `node:http` plugin (`ctx.webServer`, config
  `{host, port}`). `register(route)` / `registerUpgrade(route)` each return a
  disposer; **a duplicate path within either table throws** ("route patterns
  are a composition-level contract and a collision is a misconfiguration").
  Exactly one fallback handler (the SPA dist server is the shipped owner; 404
  until registered); fixed HTTP match order: exact over the whole table →
  longest prefix → fallback; upgrades match exactly, unmatched connections
  closed. Bind host accepts only `127.0.0.1` (default) or `0.0.0.0`
  (deliberate exposure); **no TLS, auth, or origin policy** (dev-facing v1).
  `EADDRINUSE` throws out of activation and rejects Loader composition (the
  failed candidate fiber is disposed); disposal runs `close()` +
  `closeAllConnections()`, destroys every tracked upgraded socket, and
  returns only after all have closed (dispose-reaches-quiescence).

### `packages/api` — skipped (root `AGENTS.md` one-liner only: "Remote BFF
assembly and Typert RPC gateway")

### `packages/context` — skipped (root `AGENTS.md` one-liner only: "request-context plugins")

### `packages/extensions` — partial: `cordis-host-runner` README complete; `cordis-client-runner`, `tool-cordis`, `ui-cordis` skipped

`cordis-host-runner` is the host half of **model-mounted dynamic packages**
(`ctx.dynamicCordisRunner`) — the affirmative answer to "can the harness
mount plugins at runtime, and under what authority":

- **Two phases: `define` only records; everything with an effect hangs off a
  `run`.** `define` prechecks each half's syntax by compiling it (running
  nothing), mints `dyn-<n>`, and records the definition against the asking
  session — unparseable code is refused before an id exists.
- A **host-only** package evaluates in a `node:vm` sandbox under the
  `cordis-dynamic` group fiber. A package **with a browser half** is an
  answerable round trip: `run` emits `cordis/request-run`, suspends, and is
  settled **by a person allowing or declining it**. No timer — the asking
  turn's `AbortSignal` is the only other exit. First answer wins; an answer
  naming a superseded revision is refused (`accepted: false`, request stays
  suspended); a failing verdict unwinds the host half only when this same
  request evaluated it.
- `stop` unwinds one live dispatch (handlers dropped, host-half fiber disposed
  to quiescence, `dynamicCordisRunner/retract` broadcast) and leaves the
  definition runnable.
- **The registry is process memory and the only source of truth.** The
  session log carries a define call's metadata **never its code**; a
  restarted process legitimately has no definitions, and a card whose id no
  longer resolves says exactly that. Four forwarded events
  (`cordis/request-run`, `cordis/request-run-resolved`,
  `dynamicCordisRunner/package`, `dynamicCordisRunner/retract`) carry
  **metadata, never code** — "code never rides an announcement".
- Ownership: acting verbs check session ownership; a definition another
  session defined **reads as absent rather than forbidden** — nothing leaks
  across sessions.
- Trust stance: the vm sandbox isolates globals but is **not a security
  boundary** — Node globals are absent or redirected to Cordis services
  (`ctx.fs`, `ctx.web`, `ctx.bash`); "treat a dynamic package like bash
  access".

### `packages/hooks` — partial: `hook-protocol` README complete; `hooks-claude-code`, `hooks-codex` skipped

- `hook-protocol` is a **library, not a Cordis plugin** (registers nothing,
  injects nothing): the shared core of the Claude Code / Codex external hook
  wire protocol. Codex deliberately reimplements a **subset** of the Claude
  Code hook protocol — the same `hooks.json` matcher-group shape, the same
  exit-code/stdout output contract, the same command-hook execution model;
  the two bridge plugins own only the dialect differences.
- **External hooks are shell commands**: `runHook(bash, hook, opts, now)`
  forwards the stdin JSON payload + env through `ctx.shell` (after the
  executor's credential scrub), honors the hook's `timeoutSec` (default
  10-minute reference), and **never throws** — an executor rejection
  (infra fault) becomes `exitCode: undefined`, a non-blocking error.
- `parseHookOutput`: **exit 2 blocks with stderr; other failures are
  non-blocking**; a matching hook-specific permission decision overrides the
  legacy top-level decision.
- **`mergeHookOutputs` = most-restrictive-wins**: permission precedence
  **deny > ask > allow**, halt sticky on the first `continue:false`, block
  reasons joined with `\n\n`, `additionalContext`/`systemMessages`
  accumulated in order. (Same lattice shape as the tools pipeline's
  allow/deny/ask waterfall.)
- **`createDetachedRuns()` quiescence**: fire-and-forget hook points track
  their run chains; `drain()` aborts (killing a still-running hook process
  via the signal) then awaits, and is registered as the effect disposer —
  "`fiber.dispose()` resolving therefore means no detached hook work is left
  to fire into a disposed context".
- `hook/invoked` / `hook/result` session events (paired by `handlerId`) are
  declaration-merged into `SessionEventMap` (log-only); records must sit
  inside an open turn (`SessionStart` runs before turn 1 and gets no record).
  Known gap: `HookOutput.updatedInput` is parsed but **not honored** (deferred
  consistency design).

### `packages/bundle` — partial: `base` README complete + `cordis.patch.yml` (first 120/451 lines); `headless`, `web-app` skipped

- `base` is the **first layer of every profile**: one `insert` over the empty
  profile root adding every base plugin row (model adapters, shared
  `agent-default-model` selection, tools, persistence, policy,
  settings/credentials, telemetry, host-level subagent providers). It has
  **no runtime API** — the profile composer resolves the patch through the
  `dsh.bundle.patch` manifest field, never through code.
- **Row order carries no load semantics** — "activation is
  service-availability driven" (code-level confirmation of inject-driven
  loading). Patch semantics: a patch replaces the targeted row's whole
  `config` (no deep-merge); mode-specific rows live in mode bundles;
  **last write wins per row id**.
- **Platform gating inside the patch**: `bash-sandbox`/`tool-bash` carry
  `disabled: !!js process.platform === 'win32'` and the `pwsh-sandbox`/
  `tool-pwsh` twins the inverted expression — one shared patch file, exactly
  one shell stack per host. Mounting `dsh-fs-local` alongside `dsh-fs-sandbox`
  would double-register `ctx.fs` and fail the load (misconfiguration fails
  loud at load).
- Observed rows (first 120 of 451 lines): timer, hmr, llm, session, typert
  registry/loader/gateway, session-title (+llm title generator),
  user-questions, agent, `agent-default-model`, jobs-local, llm-retry,
  settings-file (`$DSH_HOME/settings.yaml`, hot-reloaded — a settings section
  overrides adapter entries "without a restart"), credentials-local (inherited
  env > managed `$DSH_HOME/.credentials.yaml` > project/user `.env`),
  **`llm-pi-ai` mounted dormant** — zero routes until a settings section
  supplies provider profiles ("which adapters exist is composition; which
  providers run is the user's settings document"),
  `session-persistence-jsonl` (root `!!js dshHomePath('sessions')`),
  `attachment-local` (durable image bytes outside the append-only log),
  `session-query-sqlite` (opt-in full-text search; `openAt: never` keeps
  exact reads/titles/lineage available while search fails with
  `SESSION_QUERY_SEARCH_DISABLED` and SQLite is never opened).
- Codex and Claude Code providers load dormant in base; Agent Presets
  independently decide whether their agent contributes either model-facing
  delegation tool.

### `packages/preset` — skipped (root `AGENTS.md` one-liner only: "per-session agent composition from preset cordis.yml files")

### `packages/util` — skipped (root `AGENTS.md` one-liner only: "zero-dependency utilities")

## Cross-cutting kernel mechanisms

### 1. Plugin lifecycle — fibers with reversible registrations

- Fiber state machine: `PENDING → LOADING → ACTIVE → UNLOADING → DISPOSED`
  (plus `FAILED`). Source: `docs/cordis-api/fiber.md`, `docs/cordis-tutorial/02*`.
  (`host/plugin-inventory` exposes exactly these phases as `pending|loading|
  active|failed|unloading`.)
- **Every registration is a reversible effect.** Root `AGENTS.md`: "every
  contribution goes through `ctx.effect()` / `ctx.on()`; a registry's
  `register()` returns the disposer." Plugin unload runs **disposers in
  reverse registration order**, and **async disposers run concurrently**
  (with the vendor hardening applied). `packages/AGENTS.md` adds the
  verification rule: "Registry contributions prove disposal through the
  HMR-safety test: dispose the fiber and observe removal."
- Quiescence is a hard disposal contract across the repo: detached hook work
  (`createDetachedRuns().drain()`), the webserver's socket teardown, and the
  agent-loop's "disposal waits for signal-ignoring work before registry
  removal" all converge on dispose-must-reach-quiescence.

### 2. Dependency graph — `inject`-driven loading

- Plugins declare required services via `inject`; a fiber stays `PENDING`
  until its injected services exist, then loads.
- **Provider swap is clean**: replacing a provider unloads and reloads the
  dependent fibers rather than leaving them live against a vanished service
  (`docs/cordis-tutorial/03*`).
- Code-level confirmation (top-up): `bundle/base` — "row order carries no
  load semantics (activation is service-availability driven)"; dormant
  provider twins mount with zero routes until settings supply profiles.

### 3. Event bus — five dispatch modes as public contract

- Modes: `emit` (fire-and-forget), `parallel`, `serial`, `bail` (first
  non-undefined wins, stops), `waterfall` (value passed through all handlers).
- **The mode is part of each event's public contract.** It is pinned by an
  `@mode` JSDoc tag on the event declaration (root `AGENTS.md`: "Event JSDoc
  needs `@mode` and payload `@param`") and **verified by generated catalogs**
  — the 56-event matrix in `docs/event-producer-consumer.md` is the generated
  cross-check of producers, consumers, and modes (`docs/cordis-primer.md`).

### 4. Waterfall veto — single-decision policy pattern

- `waterfall` handlers are around-middleware with an explicit `next()`.
- **Omitting `next()` is a deliberate veto**, not a bug: it is the sanctioned
  way to write a single-decision policy (e.g. deny a tool call) without a
  separate gate abstraction. Root `AGENTS.md` codifies it: "Waterfall
  listeners MUST call `next()` to delegate; returning without it
  short-circuits the chain."
- Standing repo rule: **observers must delegate** — a handler that only
  observes must call `next()` and pass the value through; veto rights are
  reserved for policy handlers (`docs/cordis-primer.md`,
  `docs/cordis-tutorial/04*`).

### 5. Vendor-as-fork — the hardening checklist

`vendor/README.md` (read in full) documents 9 pinned Cordis packages rescoped
to `@deepseek-ai/*` and an 18-entry modification log, including:

- **fiber disposal hardening** (safe async disposal semantics),
- **transactional Loader/Include reconciliation** (partial-load repair),
- **lazy `!!js` config resolution**.

Each entry is a concrete upstream-framework weakness found in production and
the local fix for it — directly mineable as a checklist for evaluating any
plugin-kernel dependency. (Full 18 entries not transcribed here; the file is
short and worth a direct read when used.) Vendoring policy (root `AGENTS.md`):
pinned source copies with upstream SHAs; updates go through the sync
procedure, re-applying or retiring logged modifications.

### 6. Host/client compiler faces

- Both faces declaration-merge Cordis `Context` under the same keys, so they
  cannot be typechecked in one aggregate; `tsconfig.host.json` and
  `tsconfig.client.json` are two project-reference roots over
  `tsconfig.base.json` / `tsconfig.base.client.json`.
- `tsdown.config.ts` switches the build between faces via `DSH_BUILD_FACE`.
- Related convention (root `AGENTS.md`): "Keep compiler faces explicit. Each
  package uses one aggregate except `api/remotes`."

### 7. Composition model — profiles stack bundles, patches layer

Composition order (later layers override earlier):

1. **bundles** — each bundle is a patch-layer declared in its `package.json`
   `dsh` field (`dsh.bundle.patch` → its `cordis.patch.yml`),
2. → **profile `cordis.patch.yml`** (per-profile user patch),
3. → **home-level `cordis.patch.yml`** (outranks the per-profile file),
4. → **`--patch` CLI overlay**.

`dsh --dump-config` prints the fully resolved config tree, rendered by the
include's own parser and patch algorithm — the dump provably equals what
boots (`docs/architecture.md`; `boot/app-boot` README → `renderConfigDump`,
`composeEntries`). The user patch layer stays **live** through
`watchUserPatches` HMR: every change transactionally recomposes the full
patch list; failures keep the last good tree running.

### 8. Event-sourced session + derived projection

See `core/session` above; the load-bearing idea is that **the log is the only
truth** and everything the model sees is a projection (`deriveMessages()`),
keeping replay, audit, and forward-compatibility one mechanism. Required-on-
read event typing + `ignorable: true` gives strict readers forward-compat
without weakening known events.

### 9. Repo-wide convention net (root `AGENTS.md` + `packages/AGENTS.md`, both surfaced during top-ups)

Highly transferable rule set, quoted/paraphrased with sources:

- "Registrations are effects"; "Misconfiguration fails loud at load";
- "Enforce a decision in the operation that makes it" — schema omission,
  prompt filtering, facades, wrappers, listener order are not enforcement
  when direct or alternate callers can bypass them; test denial through the
  executor;
- "Publish state only at its commit point"; "Apply bounds to the complete
  result" (byte/token/item/time limits where the complete value is known);
- Opaque cross-boundary ids are **branded** (`Branded<B>`), never bare
  `string`; "Trust TypeScript at typed same-process boundaries" — runtime
  validation only at parser/config, queued, model/tool JSON, durable/file,
  worker, process, and wire boundaries;
- Capability seam = Service Definition / Service Provider / Consumer roles,
  complete, never one role;
- Every package owns `./invariant` — runtime invariants assert owned
  event/data relations, and unexplained empty invariant companions fail a
  gate;
- Product-visible plugins require a **REAL-composition test** (boot a
  test-only `cordis.yml` through the Loader; mock-only suites are
  insufficient);
- Every package README documents **Model Experience** (what the model sees,
  token effect, KV-cache effect) and **Known Limitations and Deferred Work**
  (gated by `verify-package-readme-limitations`).

## Findings

Kernel/lifecycle:

- **Everything-is-a-plugin; no privileged core; extend by mounting beside, never patching; "plugins, not loop changes"**: source=docs/architecture.md + AGENTS.md, confidence=high, type=fact
- **Context is a service-repository proxy; extend/isolate/intercept create scoped children; low-level get/set/provide/accessor/mixin**: source=docs/cordis-api/context.md + vendor/cordis/src/context.ts, confidence=high, type=fact
- **Fiber state machine PENDING→LOADING→ACTIVE→UNLOADING→DISPOSED (+FAILED)**: source=docs/cordis-api/fiber.md + cordis-tutorial/02 + host/plugin-inventory README (phase enum), confidence=high, type=fact
- **Registrations are reversible effects; disposers run reverse-order; async disposers run concurrently; registry contributions prove disposal via HMR-safety test**: source=docs/cordis-api/fiber.md + cordis-tutorial/02 + AGENTS.md + packages/AGENTS.md, confidence=high, type=fact
- **Five dispatch modes (emit/parallel/serial/bail/waterfall); mode is public contract pinned by @mode JSDoc and verified by generated catalogs**: source=docs/cordis-primer.md + docs/event-producer-consumer.md + AGENTS.md, confidence=high, type=fact
- **Waterfall next()-omission is a deliberate veto; observers must delegate**: source=docs/cordis-primer.md + cordis-tutorial/04 + AGENTS.md, confidence=high, type=fact
- **inject-driven dependency loading; PENDING until services exist; provider swap unloads/reloads dependents; row order carries no load semantics ("activation is service-availability driven")**: source=docs/cordis-tutorial/03 + bundle/base README, confidence=high, type=fact
- **Vendor fork: 9 pinned Cordis packages rescoped @deepseek-ai/*; 18-entry modification log incl. fiber disposal hardening, transactional Loader/Include reconciliation, lazy `!!js` resolution**: source=vendor/README.md (full read), confidence=high, type=fact
- **Dual compiler faces (host/client) from same-key Context declaration merging; DSH_BUILD_FACE splits tsdown builds**: source=tsconfig.{host,client,base,base.client}.json + tsdown.config.ts, confidence=high, type=fact

Session/agent:

- **Session is an append-only SessionEvent log; deriveMessages() projects model history; merge-extensible SessionEventMap; required-on-read default with `ignorable: true` forward-compat; model-visible ⟺ logged invariant**: source=packages/core/session/README.md + src/types.ts + docs/architecture.md + AGENTS.md, confidence=high, type=fact
- **ScopeKey opaque/identity-compared; a live agent is its own key; registrations inherit DOWN, event admission extends UP; most-specific-wins shadowing**: source=packages/core/scope/src/index.ts (complete) + docs/glossary.md, confidence=high, type=fact
- **Split ordered-teardown lifecycle (prepare/enter/announce); fork() cuts only at stable turn boundaries**: source=packages/core/session/src/index.ts (partial read), confidence=medium, type=fact
- **AgentRegistry + AgentFactory seam; AsyncLocalStorage initiator scope; ctx.agent defaults undefined; Typert wire-lookup**: source=packages/core/agent/src/index.ts (partial) + src/runtime-types.ts (complete) + packages/AGENTS.md, confidence=high, type=fact
- **agent-loop is the ONLY concrete loop; creation/resume is one rollback-covered transaction with co-owned quiescence; same-id creates arbitrate at final enter()**: source=packages/core/agent-loop/README.md, confidence=high, type=fact
- **Plugin failure ends the current turn, not the loop; agent/request-error handlers return {kind:'retry'} or fail terminal**: source=packages/core/agent-loop/README.md, confidence=high, type=fact
- **Exclusive tool calls form barriers; parallel-safe calls run in a bounded rolling pool (default 10), reclassified before start; only dispatch/body overlaps**: source=packages/core/agent-loop/README.md, confidence=high, type=fact
- **Every successful provider call appends exactly one assistant/message completion anchor with sourceEventSeqs; empty content stays out of derived history**: source=packages/core/agent-loop/README.md, confidence=high, type=fact

Tools/prompts:

- **Tool pipeline pre-execute (allow/deny/ask) → tools.guard() (monotonic owner policy) → execute (around waterfall, signal re-fusing) → post-execute → finalizeContent → result; ToolDefinition carries output.schema, pure render, finalizeContent, isConcurrencySafe, timeoutMs, presentCall**: source=packages/core/tools/src/index.ts (partial) + packages/core/agent-loop/README.md (pipeline restated), confidence=high, type=fact — *whether "monotonic" is mechanically enforced or convention remains unverified in src*
- **Code mode collapses the tool surface to run_code + generated TS/Python SDKs**: source=packages/core/tools/src/index.ts (partial), confidence=medium, type=fact
- **System-prompt assembly: numbered sections (-100/0/100–199), PromptContext snapshots as durable user-role messages, strict {{variable}} interpolation, toolOrder with mandatory <unlisted-tools> marker, complete-section override, PERSONA_SECTION shadowing**: source=packages/core/system-prompt/src/index.ts (partial), confidence=medium, type=fact

Boot/composition (top-up):

- **Fail-loud boot: installFailLoud (latched, bounded release teardown), boot() disposes partial context, assertEntriesLoaded/Activated name every unresolved plugin and await every enabled entry**: source=packages/boot/app-boot/README.md, confidence=high, type=fact
- **Dump/boot parity: composeEntries + renderConfigDump use the include's own parser/patch algorithm, so the dump equals what boots**: source=packages/boot/app-boot/README.md, confidence=high, type=fact
- **Profile = $DSH_HOME/profiles/<name> with dsh.profile.bundles; bundle manifest field dsh.bundle.patch; two-anchored bundle resolution; patch replaces whole row config (no deep-merge); empty patch file throws, [] disables**: source=packages/boot/app-boot/README.md + packages/bundle/base/README.md, confidence=high, type=fact
- **User patch layer is live: watchUserPatches HMR transactionally recomposes the full patch list; failures keep the last good tree running**: source=packages/boot/app-boot/README.md, confidence=high, type=fact
- **Platform gating via `!!js process.platform` inside patch rows; dormant provider twins ("which adapters exist is composition; which providers run is the user's settings document")**: source=packages/bundle/base/README.md + cordis.patch.yml, confidence=high, type=fact
- **Base bundle row set observed: persistence JSONL at dshHomePath('sessions'); durable attachment bytes outside the log with content-addressed references; opt-in SQLite search that never opens without an override**: source=packages/bundle/base/cordis.patch.yml (lines 1–120), confidence=high, type=fact (rows 121–451 unread)

Extensions/hooks/host (top-up):

- **Model-mounted dynamic packages exist: define (records only) / run (effects); browser-half runs are human-approved round trips with no timer, AbortSignal-only exit; registry is in-memory truth, log carries metadata never code; cross-session definitions read absent, not forbidden; vm sandbox is explicitly NOT a security boundary**: source=packages/extensions/cordis-host-runner/README.md, confidence=high, type=fact
- **External hook wire protocol: shell-command hooks with stdin JSON payload after credential scrub; exit 2 blocks, other failures non-blocking; mergeHookOutputs = deny > ask > allow most-restrictive with sticky halt; detached runs drain to quiescence as the effect disposer**: source=packages/hooks/hook-protocol/README.md, confidence=high, type=fact
- **pluginInventory is a read-only, point-in-time, Remote-only projection of the Loader tree (id/specifier/enablement/fiber phase; no provenance, no mutation)**: source=packages/host/plugin-inventory/README.md, confidence=high, type=fact
- **Webserver: duplicate route path throws (composition-level contract); fixed match order exact → longest-prefix → fallback; bind only 127.0.0.1 or 0.0.0.0, no TLS/auth in v1; disposal reaches socket quiescence**: source=packages/host/webserver/README.md, confidence=high, type=fact
- **Repo convention net: branded opaque ids; enforcement at the deciding operation; publish at commit point; bounds on complete results; every package owns ./invariant; REAL-composition test policy; per-package Model Experience (model-visible/token/KV-cache) documentation**: source=AGENTS.md + packages/AGENTS.md, confidence=high, type=fact

Inferences:

- **AgentFactory split makes the loop a swappable strategy**: source=inference from the registry seam (partially corroborated: the loop self-registers as the factory; no second implementation observed), confidence=medium, type=inference
- **dsh's pre-release "no compatibility promise" stance means mined mechanisms may still churn**: source=inference from root AGENTS.md pre-release section, confidence=medium, type=inference

## Transferable ideas

Vh-agent-harness Go analogs are suggestions (marked `inference`) unless they
map onto an existing mechanism.

| idea | evidence path | application to vh-agent-harness |
|---|---|---|
| Reversible effect registrations with reverse-order, concurrent-async disposers; dispose-must-reach-quiescence as a repo-wide contract | docs/cordis-api/fiber.md; cordis-tutorial/02; hooks/hook-protocol README; host/webserver README | Go analog: plugin/effect registry where every registration returns a `func()` disposer; lifecycle runner executes disposers LIFO, fans out async disposers, and never resolves before background work drains. Maps onto render/apply pipeline rollback semantics. (inference) |
| Typed event bus with dispatch mode as public contract (5 fixed modes, @mode-pinned, catalog-verified) | docs/cordis-primer.md; docs/event-producer-consumer.md; AGENTS.md | A fixed dispatch-mode vocabulary instead of ad-hoc per-event semantics; a generated producer/consumer catalog would make the event surface auditable the way doctor audits declarations. (inference) |
| Append-only session event log + derived projection; required-on-read typing with `ignorable: true`; model-visible ⟺ logged invariant | packages/core/session/README.md; AGENTS.md | Session memory / checkpoints could adopt log-as-truth + projection, making replay and audit one mechanism; the ignorable envelope is a forward-compat pattern for evolving durable formats. (inference) |
| Scope-keyed registration shadowing (identity keys; inherit down, admit up; most-specific-wins) | packages/core/scope/src/index.ts | Ownership classification and permission scoping could use opaque scope keys instead of path prefixes for per-agent/per-session rule shadowing. (inference) |
| Waterfall veto (`next()` omission) as the single-decision policy pattern + observers-must-delegate rule | docs/cordis-primer.md; cordis-tutorial/04; AGENTS.md | Permission/hook design: one chain shape covers both observers and decision-makers; the delegate obligation keeps advisory checks honest — closely mirrors the existing advisory-vs-gate authority split. (inference) |
| Most-restrictive-wins merge lattice (deny > ask > allow, sticky halt) shared by the tools pipeline and external hook merging | packages/core/tools/src/index.ts; packages/hooks/hook-protocol/README.md | One reusable permission-lattice merge wherever decisions from multiple sources combine (permission map, commit-review cascade, shell-guard). (inference) |
| Fail-loud boot + startup audits: partial-context disposal, every unresolved entry named, every enabled entry awaited | packages/boot/app-boot/README.md | Render/apply analog: any partial render failure disposes cleanly and the error names every entry that failed to resolve; a doctor-adjacent startup audit rather than silent partial state. (inference) |
| Dump/boot parity: `--dump-config` and `boot()` share one parser + patch algorithm | packages/boot/app-boot/README.md (renderConfigDump/composeEntries) | Directly applicable to `--dry-run`/`update` parity: preview and apply must be one code path, else preview drifts (the same class of footgun as the stale-embed corpus revert this repo already guards). (inference) |
| Layered patch composition (bundles → profile patch → home patch → overlay) with whole-config replacement and fail-loud on empty/mismatched patches | packages/boot/app-boot/README.md; packages/bundle/base/README.md | Parallels profile/overlay layering; the discipline worth copying: no deep-merge (a layer restates), last-write-wins per id, colliding/misaddressed patches fail loud. (inference) |
| Live config layer via HMR with transactional recompose and last-good-tree-on-failure | packages/boot/app-boot/README.md (watchUserPatches) | A watcher-driven harness profile could hot-recompose and keep the last good tree on parse failure — bounded, disposable, quiescent. (inference) |
| Read-only inventory projection of the mounted tree (Remote-only, point-in-time, no authority) | packages/host/plugin-inventory/README.md | An inspectable, authority-free answer to "what is currently mounted and in what phase" — the service-surface analog of `doctor`'s managed-drift check. (inference) |
| Model-mounted dynamic packages: define/run split, human-approved effectful runs, in-memory-only registry, metadata-never-code events | packages/extensions/cordis-host-runner/README.md | A safety template for any future self-modification surface: proposals are records, effects need an approved run, transport never carries code/authority — rhymes strongly with the existing skill-proposal intake (model proposes, human gate). (inference) |
| Per-package Model Experience docs (model-visible text / token effect / KV-cache effect) as a gated README section | packages/AGENTS.md; multiple READMEs | A mandatory "what does this change do to the model's context and cache" section for harness changes — forces cache-invalidation reasoning into every change slice. (inference) |
| Vendor-fork hardening checklist (18 logged local modifications) | vendor/README.md | When adopting any upstream framework, maintain a modification log as first-class docs; each entry is a production-found defect class. (inference) |
| Presentation as tool contract (render, presentCall, isConcurrencySafe, timeoutMs on ToolDefinition) | packages/core/tools/src/index.ts | Subagent/command contracts could carry presentation + concurrency-safety metadata instead of leaving rendering to callers. (inference) |
| Strict interpolation + mandatory `<unlisted-tools>` accounting in prompt assembly | packages/core/system-prompt/src/index.ts | Template token discipline: fail on unresolved tokens; force prompts to account for deliberately-omitted items. (inference) |
| Parallel-safe vs exclusive tool classification with a bounded rolling pool; only dispatch overlaps, results stay model-ordered | packages/core/agent-loop/README.md | Directly relevant to concurrent subagent/tool dispatch: classify calls before start, barrier the exclusive ones, keep an ordered result merge. (inference) |

## Contradictions

None detected within the evidence base. Two apparent tensions were checked
and resolve as consistent:

- `agent-loop` publishes `agent/inbox/spliced` **before** changing the live
  projection, while `packages/AGENTS.md` says "publish state only at its
  commit point … after the operation succeeds". Consistent reading: the
  durable log write IS the commit; the projection is derived state updated
  after (log-then-derive). Not a contradiction.
- `boot/app-boot` says an empty/comments-only `cordis.patch.yml` **throws**,
  while "misconfiguration fails loud" is the general rule — these agree; the
  deliberate disable is the explicit `[]`, not an empty file.

Caveat retained: `docs/module-graph.md` and `docs/config-catalog.md` were read
structurally only, so cross-document consistency between those two and
`docs/architecture.md` was not audited.

## Coverage

| in-scope path | status | reason |
|---|---|---|
| root configs (`AGENTS.md`, `pnpm-workspace.yaml`, tsconfigs, `tsdown.config.ts`) | examined | full orientation read; root + packages `AGENTS.md` re-surfaced during top-ups |
| `vendor/` (README + 18-entry mod log) | examined | full 60-line read |
| `docs/architecture.md` | examined | core architecture doc |
| `docs/cordis-primer.md`, `glossary.md`, `event-producer-consumer.md`, `rescope.md`, `graph-atlas.md` | examined | full reads; event matrix complete |
| `docs/cordis-api/*` (6 files) | examined | full reads |
| `docs/cordis-tutorial/*` (8 files) | examined | full reads |
| `docs/module-graph.md`, `config-catalog.md` | partial | structural read only |
| `packages/core/scope` | examined | `src/index.ts` complete |
| `packages/core/session` | partial | README + types complete; `src/index.ts` partial |
| `packages/core/agent` | partial | `runtime-types.ts` complete; `src/index.ts` partial |
| `packages/core/tools` | partial | events/ToolDefinition/Config/ToolLayer only; pipeline corroborated via agent-loop README |
| `packages/core/system-prompt` | partial | events/sections/config only |
| `packages/core/agent-loop` | partial | README complete (134 lines); `src/` (ReactLoopAgent) unread |
| `packages/core/agent-default-model` | partial | only its bundle/base row (defaults) observed |
| `packages/core/agent-tool-presentation` | skipped | budget |
| `packages/boot` | partial | `app-boot` README complete (src unread); `cmdline` skipped |
| `packages/host` | partial | `plugin-inventory` + `webserver` READMEs complete; `apiproxy`, `directory-picker*` (4), `frontend-static` skipped |
| `packages/api` | skipped | root one-liner only |
| `packages/context` | skipped | root one-liner only |
| `packages/extensions` | partial | `cordis-host-runner` README complete; `cordis-client-runner`, `tool-cordis`, `ui-cordis` skipped |
| `packages/hooks` | partial | `hook-protocol` README complete; `hooks-claude-code`, `hooks-codex` skipped |
| `packages/bundle` | partial | `base` README + patch lines 1–120/451; `headless`, `web-app` skipped |
| `packages/preset` | skipped | root one-liner only |
| `packages/util` | skipped | root one-liner only |

## Top-up pass summary

Post-packet top-up reads (all README-level, folded into the sections above):
`boot/app-boot` (full 60-line README), `bundle/base` (README + patch head),
`extensions/cordis-host-runner` (full), `hooks/hook-protocol` (full),
`host/plugin-inventory` + `host/webserver` (full), `core/agent-loop` (full
134-line README). Bonus: root `AGENTS.md` and `packages/AGENTS.md` surfaced
via read attachments and are used as corroborating sources throughout.
Net effect: the six priority open questions from the prior pass are answered
at README depth; "monotonic ToolGuards" upgraded from inference to documented
fact (`tools.guard()` for monotonic owner policy); the composition model now
has code-level confirmation (dump/boot parity, live patch HMR).

## Open questions

1. **`core/agent-loop` internals** — README is unusually detailed, but
   `ReactLoopAgent`, the inbox splice implementation, and the scheduler are
   unread; the concurrency claims (barriers, rolling pool, wake latch) rest
   on the README, not code. Residual from top-up.
2. **Mechanical enforcement vs convention of `tools.guard()` monotonicity**
   — documented characterization confirmed; the guard-chain implementation in
   `core/tools/src` was only partially read.
3. **`bundle/base/cordis.patch.yml` lines 121–451** — the remaining ~330
   lines of base rows (policy, tools, host subagent providers) are unread.
4. **Mode bundles: `bundle/headless`, `bundle/web-app`** — what a
   specialization layer restates; the template-vs-user-owned bundle
   normalization rule begs an example.
5. **`extensions/{cordis-client-runner,tool-cordis,ui-cordis}`** — the
   model-facing `cordis_*` tools and the browser half of dynamic packages.
6. **`hooks/{hooks-claude-code,hooks-codex}`** — how neutral `HookOutput`
   maps onto typed extension-point Decisions per dialect.
7. **`boot/cmdline`**, **`host/{apiproxy,directory-picker*,frontend-static}`**
   — unread.
8. **`api`, `context`, `preset`, `util`** — entirely unread beyond root
   one-liners; `preset` (per-session agent composition from preset
   cordis.yml files) is the most kernel-adjacent of these.
9. **`docs/module-graph.md` / `docs/config-catalog.md`** — content-level read
   pending; both likely encode package-level dependency truth.
10. **`core/agent-default-model` / `core/agent-tool-presentation`** — unread
    beyond their bundle rows.

---

## Gap-fill pass (2026-08-17)

Follow-up gap-fill pass at the same pinned commit `47f9438`
(47f943859bef60e4160492346772ded9b24f765a, v0.1.0-rc.5). The prose above
is the frozen original record and stays unmodified; this section folds in
the gap-fill addendum (7 clusters, all 10 packet OQs addressed). Method
unchanged: static read-only reading of `refs/deepseek-harness/`, every
claim cites a repo-relative path.

### Open-question resolutions

- **OQ 1 (agent-loop internals)** — all three read at code level; README
  concurrency claims code-confirmed. ReactLoopAgent
  (`packages/core/agent-loop/src/agent.ts`, 496 ln): single `phase` state
  machine; sticky max-tokens turn outcome; `send()` classifies
  `wakingAfterAbort` BEFORE inbox insertion; wake latch replayed at driver
  convergence; driver containment flattens unknown errors to `errorChain`
  under `UNKNOWN`; canonical `headerEquals` headers. Scheduler
  (`tool-calls.ts`, 289 ln): exclusive calls = barriers; bounded rolling
  pool (`DEFAULT_MAX_PARALLEL_TOOL_CALLS = 10`, `constants.ts`); model-order
  commits; abort records synthetic `TOOL_ABORTED_BEFORE_DISPATCH`; failure
  drains without fabricating results. Inbox splice lives in
  `packages/core/agent/src/inbox.ts` (220 ln), NOT agent-loop (agent.ts:18
  imports it): durable event committed BEFORE the projection mutates;
  `claim()` = all next-step + exactly one next-turn. Wiring
  (`agent-loop/src/index.ts`, 713 ln): memoized REVERSE teardown registered
  before resources exist (three fused abort owners); launcher strips
  config-supplied session ids; `agents` deliberately not user-settings.
- **OQ 2 (guard monotonicity)** — mechanically enforced: deny-only
  signature (`ToolGuard = (execution) => string | undefined`,
  `packages/core/tools/src/index.ts:703-711`; force-allow unrepresentable)
  plus guards evaluated LAST after the pre-execute waterfall and approval
  ask (index.ts:1463-1507; layer chain 1118-1128); the separate
  `tools/src/invariant.ts` (128 ln) enforces stage order, frozen snapshots,
  code-dispatch enclosure; `admits()` filters inherited surface only
  (index.ts:1130-1148).
- **OQ 3 (patch rows 121-451)** — fully read
  (`packages/bundle/base/cordis.patch.yml:115-451`): telemetry default-off;
  permission stack + `permission-presets` matrix; inline plan-mode prompt;
  subagent suite (fork-stays-one-shot note); ralph `maxRounds: 64`;
  `tool-web` with `fetch: false` (SSRF rationale); budgets (spill 50000,
  result-pruner 8192/4096/1024, repeat-reminder [3,5,8]); value-slot
  section (rows every mode mounts).
- **OQ 4 (mode bundles)** — headless (patch + `src/startup.ts` full):
  one-shot task mode; startup an ORDINARY provider (nothing provided on
  `--help`). web-app (patch 424 ln + README full): ~25 agent-plane rows
  `disabled: true` ("Disabling rather than deleting is deliberate",
  patch.yml:283-285); four named plane-ownership criteria (276-409);
  preset trust ("a preset IS a composition", 410-424); `--host 0.0.0.0`
  rejected pre-publish; HMR disabled in BOTH modes.
- **OQ 5 (extensions)** — trio READMEs full, src/ unread (assumption):
  dynamic packages process-memory-only and non-promotable; reflection
  curated by reachability + freshness-gated; dynamic = same loader
  lifecycle (`loader.create`); approval panel GLOBAL; human approval
  actions deliberately NOT session-log events.
- **OQ 6 (hooks bridges)** — both bridges full (`packages/hooks/
  hooks-claude-code/README.md` 97 ln, `hooks-codex/README.md` 100 ln):
  per-point Decision tables onto the SAME canonical points; serial +
  most-restrictive fold (`deny > ask > allow`); injected context carries
  plugin source; misconfig contained (warn + register nothing); CC bridge
  7-of-30 events with 23 unsupported named; bridges deliberately
  second-class vs native plugins.
- **OQ 7 (host/boot stragglers)** — `packages/boot/cmdline/README.md`
  full (injection-ordered interpolation — "a launch flag cannot be
  silently reset", :53); `packages/host/apiproxy/README.md` read (4 lines
  display-truncated, unread-in-part): 415 media-type CSRF gate, queue =
  projection of inbox events, preset lock at first turn, id-not-path
  document opens, 3-tier model default; `host/frontend-static` +
  `host/directory-picker-auto` full.
- **OQ 8 (api/context/preset/util)** — four group READMEs full;
  `packages/preset/README.md:14`: a preset naming a process-global row is
  REJECTED at mount; roster = the shipped directory listing itself.
- **OQ 9 (docs catalogs)** — heads + generation contract read; both
  GENERATED + freshness-gated (`scripts/gen-module-graph.ts`,
  `scripts/gen-config-catalog.ts`; schema cross-check "cannot hide a
  loader-accepted field", `docs/config-catalog.md:8`); bulk read judged
  low-value (assumption).
- **OQ 10 (core stragglers)** — `core/agent-default-model` (one
  process-wide default; per-field settings merge) and
  `core/agent-tool-presentation` (mount-time capability refusal; executor
  `UNKNOWN_TOOL` collapse under code mode) READMEs full.

### New findings

Cluster 1 (`packages/core/agent-loop/src/` + `core/agent/src/inbox.ts`):
`invariant.ts` mechanically enforces model-visible ⟺ logged (prepended
`llm/stream` listener; log-reconstruction desync check; prepend prevents
silencing); `runtime-context.ts` projects context as plugin-owned messages
(`CLEARED` marker, text dedup); `tool-calls.ts` results carry
`sourceEventSeqs: [callSeq]` and persist presentation `meta` for replay;
`agent.ts:271-277` a rewritten enter decision "still owns the initial turn
boundary, but it spends no model call"; `index.ts` persistence load is
"the per-id serialization barrier", duplicate identities rejected at load,
`agent-loop/config-start-failed` transient event; `inbox.ts` durable-first
mutation, invalid persisted splices throw with the session seq.

Cluster 2: `ctx.invariants.register` is a concrete enforcement engine
(`tools/src/invariant.ts`); plan mode keeps the full tool catalog listed
"for request-cache stability" (patch.yml:265-279); web fetch disabled as
SSRF-motivated omission (patch.yml:396-418);
`session-checkpoint-policy` mounts durability checkpoints before each
model request and top-level dispatch (patch.yml:354-356).

Cluster 3: disable-not-delete normalization (web-app patch.yml:283-285);
four plane-ownership criteria with concrete failure modes (287-409);
shell-equivalent user-preset trust (410-419); headless publishes its task
as an ordinary service (startup.ts:44-56); HMR off in both shipped modes;
boot-stable prompt sections "do not invalidate the [KV] cache across
turns" (web-app README:13-21).

Cluster 4: self-modification non-promotable, regular-plugin escape hatch
(tool-cordis README:19) — mirrors our candidate/authority split;
reachability-curated reflection carried as data + freshness gate
(README:41-46); dynamic rides the same loader lifecycle
(cordis-client-runner README:10-12); human approvals intentionally
unlogged (ui-cordis README:21); (assumption) README depth only.

Cluster 5: per-dialect tables onto the SAME points; serial + fold = log
adjacency AND order-independent decisions (hooks-claude-code:49);
containment for OPTIONAL integrations vs loud composed config (:31);
exhaustive partiality matrices with named TODOs (:89-96); flags beat file
config via injection-ordered interpolation (boot/cmdline README:53).

Cluster 6: media-type CSRF gate — status = carrier only, business errors
ride RpcResult (apiproxy README:21); queue view projects durable inbox
events, cancel preserves pending work (:41); preset lock at first turn +
copy-only authoring (:55-57); skill invocation = plain `session.prompt`
text at the pre-step boundary (:59); 3-tier model default with "no
separate gesture" (:9-17); capability sampled ONCE per boot, ambiguous →
safest (directory-picker-auto README:7); (caveat) 4 apiproxy lines
truncated at the display limit — unread-in-part.

Cluster 7: preset mounts GUARDED (preset README:14); capability
preconditions at MOUNT, not first request (agent-tool-presentation
README:15); executor enforcement — announced == callable (:23); single
shared default-model service (agent-default-model README:5-7);
schema-cross-checked generated catalog (docs/config-catalog.md:8);
(assumption) catalog bulk generator-enforced, line read low-value.

### Transferable ideas added

- C1-i1 Log-reconstruction invariant as a runtime check (`invariant.ts`) —
  prepend-positioned check at the LLM-call seam; maps onto doctor checks.
- C1-i2 Durable-first mutation with normalized coordinates (`inbox.ts`) —
  commit-then-project ordering.
- C1-i3 Wake-latch classification captured BEFORE the mutation it
  precedes (`agent.ts` send()).
- C1-i4 Memoized reverse teardown registered before resources exist
  (`index.ts` prepare()) — fused-three-owners abort.
- C1-i5 Sticky worst-outcome turn semantics — passing sub-checks never
  downgrade an aggregate verdict.
- C2-i1 Deny-only guard signature — force-allow structurally
  unrepresentable.
- C2-i2 Package-owned invariant companions in a shared registry
  (`ctx.invariants`).
- C2-i3 Inline-mode prompts with tool-catalog stability (request-cache).
- C2-i4 SSRF-motivated tool omission — search before fetch.
- C3-i1 Disable-not-delete in shared overlays.
- C3-i2 Named plane-ownership criteria with failure modes.
- C3-i3 Service-absence as clean degradation (args as a service).
- C3-i4 Cache-stable prompt sections (stable head, volatile isolated).
- C4-i1 Ephemeral self-modification with a promotion wall.
- C4-i2 Reachability-curated reflection (filter, data, gate).
- C4-i3 Same-lifecycle rule for dynamic and static extensions.
- C4-i4 Global seat for blocking approvals.
- C5-i1 Compatibility bridges second-class by design.
- C5-i2 Serial + most-restrictive fold (adjacent logs, order-independent).
- C5-i3 Exhaustive public partiality matrix.
- C6-i1 Media-type gate as CSRF defense.
- C6-i2 Id-based, never path-based, document opening.
- C6-i3 One deterministic invocation path for all surfaces.
- C6-i4 Composition-locks-at-first-use.
- C7-i1 Mount-time capability precondition refusal.
- C7-i2 Announced surface == callable surface, at the executor.
- C7-i3 Roster IS the directory listing.
- C7-i4 Schema-cross-checked generated catalogs.

### Coverage after gap-fill

| area | before | after |
|---|---|---|
| `packages/core/agent-loop` | ~35 (README only) | 100 (all src full-read) |
| `packages/core/agent` | ~40 | ~70 (inbox.ts full) |
| `packages/core/tools` | ~40 | ~65 (targeted index.ts + invariant) |
| `core/{agent-default-model,agent-tool-presentation}` | ~10 | 90 (READMEs) |
| `core/{session,system-prompt}` | partial | unchanged |
| `packages/bundle` | base ~30 | base 100 / headless 100 / web-app ~85 |
| `packages/extensions` | ~25 | ~70 (READMEs; src unread) |
| `packages/hooks` | ~35 | ~90 |
| `packages/boot` | ~55 | ~85 (cmdline full) |
| `packages/host` | ~45 | ~75 (apiproxy 4 lines truncated) |
| `packages/{api,context,preset,util}` | ~5 | ~60 (group READMEs) |
| `docs/{module-graph,config-catalog}` | 10 | ~30 (heads + contract) |

### Contradictions and corrections from this pass

- None detected across the pass (addendum final).
- Two merge notes against the frozen body, applied HERE only: (1) "live
  patch HMR" holds for the base layer, but the shared HMR row is disabled
  in BOTH shipped mode bundles; (2) the frozen OQ-1 phrasing places the
  inbox under agent-loop — ownership is `core/agent` (agent-loop imports
  it).
