# auto-classifier-pilot

An **opt-in** overlay pack that ships a three-hook plugin — the pilot for an
auto-classifier-style tool-call gate. It implements three behavior modes,
selected by a live config `mode` field (default `audit`):

- **`audit` (Phase 1, default)** — observability only. Zero behavior change.
- **`enforce` (Phase 2)** — runs a deterministic, fail-closed decision path on
  the **event hook** (verdict parser + STUB evaluator), then **auto-approves or
  rejects** the tool call by replying to the `permission.asked` bus event via
  the SDK client. **Not a real classifier model** — for exercising the decision
  path only.
- **`live` (Phase 3b)** — fetches the real session transcript, serializes it
  (credential-scrubbed + truncated; tool inputs redacted via an allowlist) to a
  text-mode string, and calls a **provider-agnostic OpenAI-compatible HTTP**
  classifier endpoint. The returned verdict feeds the **same fail-closed
  decision matrix** as `enforce` and replies via the same event-hook surface.
  The model API key is read from an env var at call time — it never lives in
  either config file.

## What this is

The plugin (`auto-tool-gate`) hooks **three** OpenCode surfaces, but only one is
the **enforcement surface** — the **`event` hook**. In the default **`audit`**
mode the plugin writes one audit line to **stderr** per firing, capturing a
verdict placeholder (`verdict=AUDIT_ONLY`). It never throws, never blocks, never
mutates a tool call, and never replies to a permission event. **Enabling this
overlay with the default mode changes zero behavior** — it only adds stderr
audit lines.

The three hooks:

1. **`event`** *(PRIMARY ENFORCEMENT SURFACE)* — receives every OpenCode bus
   event, including `permission.asked`. When OpenCode's permission table routes
   a tool call to "ask", OpenCode stores a Deferred, publishes the
   `permission.asked` event, and awaits the Deferred. This hook intercepts that
   event, runs the classifier (stub in `enforce`, real HTTP model in `live`),
   and **replies** via the SDK client method
   `client.postSessionIdPermissionsPermissionId({path:{id:sessionID,
   permissionID:req.id}, body:{response:"once"|"always"|"reject"}})` for
   **allow** replies, or the v2 route (`POST /permission/:id/reply`) for
   **reject** replies that carry a reason (per-call gate — see "Per-call gate"
   below). The reply
   resolves the Deferred → OpenCode's `Permission.ask` unblocks → the tool call
   proceeds (allow) or is blocked (reject). This is the same pattern OpenCode
   ships in its ACP agent, `opencode run --dangerously-skip-permissions`, and
   the TUI. **In `audit` mode this hook logs the event (scrubbed) and does NOT
   reply** — the human still decides (interactive mode).
2. **`tool.execute.before`** *(AUDIT-ONLY OBSERVER)* — fires for **every** tool
   call (including ones the permission table auto-allows). Per firing it logs:
   - the **tool name** (`input.tool`),
   - a **short, redacted argument summary** (command / path / pattern / query /
     url / workdir, truncated; full payloads are never dumped),
   - `verdict=AUDIT_ONLY`.
   It **must NOT throw or block** — enforcement is owned entirely by the event
   hook. This hook is kept because it sees calls `permission.asked` does not
   (the table-allowed fast-path).
3. **`permission.ask`** *(DORMANT — RETAINED AS RESERVE)* — OpenCode does not
   fire `permission.ask` in any stock release as of the studied version. The
   event hook is the active enforcement surface. This hook is retained as a
   hedge in case upstream wires it; it mirrors the same decision logic so it
   could serve as the enforcement surface if the hook ever fires. **No claim of
   auto-approval rests on this hook.** It is a dormant reserve only.

## Decision architecture

### The three-hook model

OpenCode exposes **three** plugin hooks relevant to this plugin (verified
against `@opencode-ai/plugin` + `@opencode-ai/sdk` types + three shipped
reference implementations):

| Hook | Input | Powers | Role in this plugin |
|------|-------|--------|---------------------|
| `event` | `{ event }` where `event.type` is the event name and `event.properties` is the payload | Receives **every bus event**, including `permission.asked`. For `permission.asked`, `properties` is the `Request` `{id, sessionID, permission, patterns, metadata, always, tool}`. The hook replies via the SDK client to resolve the Deferred → auto-approve or auto-reject. **Allow** replies use the v1 route (`postSessionIdPermissionsPermissionId`); **reject** replies use the v2 route (`POST /permission/:id/reply`) to attach a reason (per-call gate — see "Per-call gate" below). | **PRIMARY ENFORCEMENT SURFACE.** This is the hook that makes `enforce`/`live` auto-approve against stock OpenCode. |
| `tool.execute.before` | `{tool, sessionID, callID}` | **block (throw)** or **passthrough (bare return)** only. Cannot force-allow. Sees EVERY tool call. | **AUDIT-ONLY OBSERVER.** Must NOT throw/block. Kept because it sees calls the event hook does not (table-allowed fast-path). |
| `permission.ask` | `Permission {id, type, pattern, sessionID, messageID, callID?, title, metadata, time}` + `{status}` output | Three-way `status` mutation (`allow` / `deny` / `ask`). | **DORMANT RESERVE.** Not fired by stock OpenCode. Retained as hedge. No enforcement claim rests on it. |

The `event` hook is the surface that maps onto the reference classifier's three
dispositions (allow / deny / ask). When `mode` is `enforce` or `live`, the hook
receives a `permission.asked` event, classifies the request, and replies with
`"once"` / `"always"` / `"reject"`. In `audit` mode it logs only and does NOT
reply (the human still decides in interactive mode).

> **Headless/autonomous hang warning.** If NO ONE replies to a
> `permission.asked` event, the Deferred never resolves and the tool call
> **hangs**. In autonomous mode (headless, no human present) the plugin MUST
> reply. `"no reply"` is only safe in interactive mode where a human can click.
> This is why `audit` mode is explicitly documented as **not for autonomous
> operation** — it is observe-only.

### Composition model (the three-stage permission flow)

Three layers compose into a strict **pipeline**, not a vote: an earlier stage
can short-circuit every later stage. The classifier is the **last, slowest**
stage and only ever runs on the **residue** that survives the two faster static
stages. This is what makes the layering economical — the fast static layers
absorb the volume, and the slow LLM spends only on the genuinely-ambiguous
middle.

#### The three-stage flow

Per tool call, OpenCode resolves permission through this exact ordering
(verified against `refs/opencode` at the current upstream):

```
tool.execute.before  ──►  shell-guard throws?  ──YES──►  BLOCKED (LLM never runs)
                         │
                         NO (passes)
                         ▼
                    item.execute → ctx.ask → permission table
                         │
                         ├── allow   ─────────────────► fast-path (LLM never runs)
                         ├── deny    ─────────────────► BLOCKED (LLM never runs)
                         └── ask / no-match ──────────► permission.asked → classifier decides
                                                                   (allow / reject-with-reason)
```

The two short-circuit points — the **shell-guard throw** at Stage 1 and the
**table routing** at Stage 2 — are what gate the classifier. The
`permission.asked` event (Stage 2's `ask` branch) fires **only** for the
ask-routed subset; everything else is resolved statically before the LLM is
ever consulted.

#### Circuit-breaker: how Stage 1 short-circuits Stage 2

The pipeline ordering is not a convention — it is enforced by OpenCode's plugin
trigger machinery:

- **`Plugin.trigger` is a sequential `for`-loop** over registered hooks
  (`refs/opencode/packages/opencode/src/plugin/index.ts`, the `trigger`
  `Effect.fn`): each hook runs via
  `yield* Effect.promise(async () => fn(input, output))`. **If any hook throws,
  the loop short-circuits** (later hooks do not run) and the whole trigger
  Effect fails.
- **`tool.execute.before` fires BEFORE `item.execute`.** In
  `refs/opencode/packages/opencode/src/session/prompt.ts`, the tool-execution
  path yields `plugin.trigger("tool.execute.before", ...)` first, and only then
  runs `item.execute`, whose `ask` callback calls `permission.ask` (i.e.
  `ctx.ask`) — the entry point that consults the permission table and, on an
  `ask` routing, publishes the `permission.asked` event.

Concretely:

- **Stage 1 — `tool.execute.before`:** shell-guard lives here. If shell-guard
  throws on a forbidden pattern → the trigger Effect fails → `item.execute`
  never runs → `ctx.ask` is never called → **no `permission.asked` event** →
  the classifier's enforcement (Stage 2) never fires. Circuit broken.
- **Stage 2 — `permission.asked`:** the classifier's `event` hook replies here.
  It is reached only if Stage 1 passed **and** the permission table routed to
  `ask` (not the `allow` fast-path, not `deny`).

#### The three layers

| Layer | Where | Toggleable? | Speed | Role |
|---|---|---|---|---|
| **shell-guard** (blocklist) | `tool.execute.before` hook | **No — always on, hard floor** for every adopter | instant (static pattern match) | absorbs the obvious-bad volume; circuit-breaks before any LLM cost |
| **permission-pack** (table: readonly / git_read_only / gate / harness) | OpenCode permission config | yes — per-overlay | instant (table lookup) | routes calls to allow (fast-path) / deny / ask |
| **auto-classifier** (LLM) | `permission.asked` event hook | **opt-in** (overlay selected + `enabled` / `mode` set) | slow (HTTP to the classifier model) | residual judge for the grey middle that survives Stage 1 + the table's `ask` |

#### Key invariants

1. **The classifier NEVER overrides a static allow.** Table-`allow` calls
   fast-path past Stage 2 entirely — no `permission.asked` event is published,
   so the classifier's enforcement hook never sees the call.
2. **The classifier NEVER overrides a static deny.** A shell-guard throw at
   Stage 1 circuit-breaks before Stage 2; a table-`deny` routes away from the
   `ask` branch. In both cases the classifier's enforcement hook is never
   reached — a statically-denied call cannot become an allow.
3. **LLM cost is incurred ONLY on the ask-residue** — never on shell-guard-denied
   calls, never on statically-allowed fast-paths, never on static denies. This
   is the speed advantage of the layering: the fast static layer absorbs the
   volume; the slow LLM spends only on the genuinely-ambiguous middle.
4. **Under a readonly / read-only-execution posture, the classifier is
   effectively inert for the readonly tools** — the table pre-allows them
   (fast-path), so they never reach `ask`. For the rest, only the shell-guard
   floor plus the classifier's observe-only audit hook (the
   `tool.execute.before` observer) run. The framing: *LLM evaluation optional,
   readonly guard always.*
5. **shell-guard is unreachable-by-design for the classifier.** The classifier
   cannot see, override, or appeal a shell-guard deny; it is a true hard floor
   upstream of Stage 2.

#### Ordering nuance — audit hook vs shell-guard at Stage 1 (not a correctness issue)

The classifier's *audit* hook also lives at `tool.execute.before`. Whether it
logs before shell-guard throws depends on plugin-discovery (glob) order. If
shell-guard is discovered first, it throws and the audit hook is skipped — but
shell-guard logs its own deny, so there is **no audit-coverage gap**. The
*enforcement* (Stage 2) is what matters for correctness, and it is firmly gated
by Stage 1 regardless of audit-hook order.

#### Tight vs relaxed allowlist posture

An adopter running a **tight** posture (e.g. a `git_read_only` permission-pack
that denies most mutating shell) will see the classifier consulted **rarely** —
little traffic reaches `ask`. A **relaxed** posture (a permissive allow table)
will see the classifier consulted **more often** — more calls reach `ask`. The
classifier's value scales with the size of the ask-residue; a strict static
layer is **complementary**, not redundant. Layering a slow LLM judge on top of a
fast, strict static layer is what keeps both cost and coverage sane.

### Reconciliation rule (Phase 2/3 must preserve)

When the classifier owns the event hook, the layered precedence is:

1. **Static deny always wins.** A table / shell-guard deny blocks regardless of
   the classifier — and it does so *before* the `permission.asked` event is
   even published.
2. **Static failure denies.** If a static rule errors, the call is denied, not
   allowed.
3. **Classifier allow is valid only when no lower layer denied.** Since the
   table runs first, a classifier `allow` can only ever lift an `ask` (the
   default) — it cannot override a deny, because deny never reaches the event
   hook.
4. **Classifier failure / timeout / malformed verdict blocks.** Fail-closed:
    the gate replies `"reject"`, **never** silently allows.

### Which hook each phase uses

| Phase | `tool.execute.before` | `permission.ask` | `event` | Behavior |
|-------|-----------------------|------------------|---------|----------|
| **1 (this pack)** | audit (no block) | audit (dormant, no status mutation) | audit (logs only, NO reply) | Observability only. Default mode `audit`. |
| **2 (this pack)** | unchanged (audit, permanent) | dormant reserve | **verdict parser + fail-closed stub evaluator → reply** | `enforce` mode: parses a verdict via a DETERMINISTIC STUB and replies `"once"`/`"always"`/`"reject"`. Fail-closed → reject. Not a real model. |
| **3b (this pack)** | unchanged (audit) | dormant reserve | **live classifier model (OpenAI-compatible HTTP) → reply** | `live` mode: real security-monitor LLM replaces the stub in the event hook, fed by a serialized transcript. Same fail-closed matrix as `enforce`. |
| **4** | promotion review | promotion review | promotion review | Decide whether to promote into core templates / `README.agent.md`. |

### Settled: does the enforcement surface fire for every tool call?

**No** — the `permission.asked` event fires **only for ask-routed calls** (the
table routes them to "ask"). Table-`allow` fast-paths past it; table-`deny` /
shell-guard blocks before it. The `event` hook receives every bus event but
early-returns on anything that is not `permission.asked`. The
`tool.execute.before` hook is kept specifically because it sees the
table-allowed calls that never reach the event hook — it is the observability
surface for the full tool-call stream.

## What this is NOT (out of scope — later phases)

- No fast-path allowlist / two-stage logic (later phase). The permission table
  already provides table-`allow` fast-paths; this pilot does not add a second
  model-gated allowlist.
- No provider-native (non-OpenAI) request format. `live` mode speaks
  OpenAI-compatible chat-completions only; a provider-native format is a later
  phase if needed.
- No permission grants (no `permission-pack.jsonc`).
- `enforce` and `live` are NOT the default mode (default stays `audit` — zero
  behavior change for existing operators).

## Fail-closed intent (implemented in `enforce` and `live` modes)

The design source studies a security-monitor classifier that gates each tool
call. The reconciliation rule above is the local statement of its fail-closed
invariant: when the classifier is unavailable, returns an unparseable verdict,
or the evaluator throws, the gate fails *closed* (deny), **never** silently
allows. Phase 2 `enforce` mode and Phase 3b `live` mode both implement this:
`decidePermission()` maps evaluator-thrown errors, unparseable verdicts, and
`block` verdicts all to `deny`; only an explicit `<block>no</block>` (allow)
verdict yields `allow`. `live` mode additionally fail-closes on any transport
error, timeout, non-2xx, malformed response, missing API key, or missing
endpoint/model.

## How to enable

This pack is **opt-in** — it is not selected by default. It **ships embedded in
the binary**, so it is name-selectable out of the box: there is nothing to
vendor, copy, or recreate. (A project that keeps a pack of the same name under
`.vh-agent-harness/overlays/auto-classifier-pilot/` still shadows the embed
wholly — that override seam is unchanged.)

1. Add `auto-classifier-pilot` to the `overlays:` list in
   `.vh-agent-harness/vh-harness-profile.yml`:

   ```yaml
   overlays:
       - harness-dogfood
       - auto-classifier-pilot
   ```

2. Re-render the corpus:

   ```sh
   vh-agent-harness update
   ```

3. Restart the opencode server (plugins load at server start). The audit lines
   appear on stderr; in OpenCode they surface in the server log.

To disable, remove the pack from the `overlays:` list and re-run
`vh-agent-harness update`.

## Live configuration (no restart)

Auto-mode behavior is configurable **without restarting OpenCode**. The plugin
reads operator-owned JSON config files from disk on **every hook invocation** —
editing a file takes effect on the **next tool call**, with no server restart
and no corpus re-render. Config is loaded from a **four-level layered model**
(user-level base, committed-project override, optional project-local `.local.json`
override; see below).

This works because the plugin does runtime file I/O inside each hook (mirroring
how `shell-guard.js` imports `node:fs` + `node:path`). The OpenCode plugin SDK
has **no native hot-reload config API** — the `config` hook and `PluginOptions`
are load-time (set at server start) and env vars are frozen at process start —
so a per-call disk read gated by an **mtime cache** is the reload-free
mechanism: in steady state an unchanged file costs only a single `statSync` per
call, and a changed file is re-read + re-parsed only when its mtime changes.

> **`.local.json` override convention:** each committed base file may have an
> OPTIONAL gitignored `.local.json` shallow field-override companion (the first
> auto-gate consumer of the repo-wide `.opencode/repo-configs/*.local.json`
> convention). The local file is the FINAL project layer and is absent by
> default — its absence is the legacy three-level behavior with zero change. See
> "Four-level layered loading" below.

### Two-file model (config split)

Config is split across **two sibling files** under `.opencode/repo-configs/`
(resolved relative to the repo root, the same way `shell-guard-core.js` derives
`repoRoot()` — from the plugin file's location, never a hardcoded absolute
path). The split exists so that **LLM settings can NEVER be committed** while
**plugin behavior MAY be committed** (or not) at the adopter's choice:

| File | Fields | Committability |
|------|--------|----------------|
| `auto-gate-config.json` | `enabled`, `mode`, `stubVerdict`, `promptFile`, `replyMode`, `onUncertain`, `harnessContext`, `guides` | **Adopter's choice** — a team may commit a shared default (e.g. `{"mode":"enforce"}`). NOT gitignored. |
| `auto-gate-llm.json` | `modelEndpoint`, `modelEndpointEnv`, `model`, `apiKey`, `apiKeyEnv`, `timeoutMs`, `maxRetries`, `retryDelayMs` | **NEVER** — gitignored in this dogfood repo. Adopters using live mode create it locally and add the pattern to their own `.gitignore`. |

Neither file is rendered or seeded by the overlay. Leaving a file absent is the
documented fail-safe default — the plugin works out of the box with built-in
defaults.

> **Why the split?** The single-file predecessor mixed LLM endpoint settings
> with plugin behavior in one committable file. The split guarantees LLM config
> (which points at an external endpoint) can never leak into a shared repo,
> while still letting a team share a plugin-behavior default like
> `{"mode":"enforce"}`.

### Four-level layered loading (user-level base, committed-project override, project-local `.local.json` override)

Each config type (plugin-behavior and LLM) is loaded from **up to three
locations** and merged **field-by-field** with this precedence:

```
defaults  ←  user-level  ←  committed-project  ←  project-local    (project-local wins per field)
```

| Level | Plugin config path | LLM config path | Committed? |
|-------|-------------------|-----------------|------------|
| **User** (shared across all projects) | `<XDG_CONFIG_HOME>/vh-agent-harness/auto-gate-config.json` | `<XDG_CONFIG_HOME>/vh-agent-harness/auto-gate-llm.json` | operator's choice |
| **Committed project** (per-repo, shared) | `.opencode/repo-configs/auto-gate-config.json` | `.opencode/repo-configs/auto-gate-llm.json` | adopter's choice (plugin) / NEVER (llm) |
| **Project-local** (per-repo, per-developer override) | `.opencode/repo-configs/auto-gate-config.local.json` | `.opencode/repo-configs/auto-gate-llm.local.json` | **NEVER** — gitignored by `.opencode/repo-configs/*.local.json` |

`XDG_CONFIG_HOME` resolves as `process.env.XDG_CONFIG_HOME || path.join(os.homedir(), ".config")`
(typically `~/.config`). The user-level directory is `vh-agent-harness/` — **not**
`opencode/` — because vh-agent-harness ships the plugin and defines the schema.
The filenames mirror the committed-project names so the override relationship is
obvious: "same file, user-level base, committed-project override, project-local
final override."

The **project-local** `.local.json` layer realizes the repo-wide override
convention: it is OPTIONAL and absent by default (absent = legacy three-level
behavior, zero change), and when present it shallowly overrides matching base
fields (a present key with a falsy value like `false` or `""` is a real
override, not absence). A minimal local override — e.g. to flip just the mode
for one developer without touching the committed base:

```json
{"mode": "live-tiered"}
```

**Merge rules:**

- A **missing** file at any level is silently skipped (its absence is normal).
  All-missing → fail-safe defaults (exactly the pre-layering behavior). The
  project-local file is OPTIONAL and absent by default.
- A **present-but-invalid** file (bad JSON, non-object shape) is also skipped —
  the lower layers apply, and a deduped audit line is emitted with the level in
  the label (e.g. `plugin/user`, `plugin/project`, `plugin/local`, `llm/user`,
  `llm/project`, `llm/local`).
- When a higher-level file is **present and valid**, it overrides the lower
  levels **field-by-field** — only the fields it explicitly sets win; fields it
  omits fall through to the lower levels, then to defaults. So a project-local
  `{"mode": "live-tiered"}` keeps the committed-project/user endpoint+key and
  changes only the mode; a committed-project `{"model": "x"}` keeps the
  user-level endpoint/key and changes only the model. (This is the common case
  for specializing one field per repo or per developer.)
- The mtime cache tracks **all** files in play (up to three per type); changing
  **any** one invalidates the cache and triggers a re-read + re-merge on the
  next hook call. Creating or deleting a `.local.json` transitions its mtime
  (number↔null) and likewise invalidates the cache.

**Typical use:** put a shared endpoint/model/key-env in the user-level
`auto-gate-llm.json` once, and it applies to every project. A project that needs
its own override drops a committed-project file. A single developer who needs a
temporary local override (e.g. flip `mode` for an experiment) drops a
`.local.json` — it never reaches git and never affects teammates.

Example — user-level LLM config (applies to all projects without one).
Both the literal and env-var forms are shown; in practice you pick ONE per
field (literal wins if both are non-empty):

```json
{
  "modelEndpoint": "https://api.provider.example/v1/chat/completions",
  "model": "gpt-4o-mini",
  "apiKeyEnv": "MY_GATE_KEY"
}
```

Or using the env-var-name form for the endpoint (operator sets the URL via the
`AUTO_GATE_MODEL_ENDPOINT` environment variable instead of hardcoding it):

```json
{
  "modelEndpointEnv": "AUTO_GATE_MODEL_ENDPOINT",
  "model": "gpt-4o-mini",
  "apiKey": "sk-literal-key-here"
}
```

### Plugin config fields (`auto-gate-config.json`)

| Field | Type | Default | Meaning |
|-------|------|---------|---------|
| `enabled` | boolean | `true` | Master live toggle. `false` live-disables the plugin: both hooks no-op immediately on the next tool call (no audit, no behavior change, no restart). `true` is the normal on state. |
| `mode` | `"audit"` \| `"enforce"` \| `"live"` \| `"live-tiered"` | `"audit"` | Behavior selector. `audit` = Phase 1 observability only (default, zero behavior change). `enforce` = Phase 2 decision path on `permission.ask` (verdict parser + STUB evaluator; fail-closed to deny). `live` = Phase 3b decision path using a REAL OpenAI-compatible model call (fail-closed to deny on any error/timeout/misconfiguration). `live-tiered` = Phase 2 multi-leaf consensus: dispatches the `live` classifier for EACH configured leaf IN PARALLEL and grants only on **unanimous-allow** (see "Phase 2 — `live-tiered` consensus mode"). `tool.execute.before` stays an observer in all modes. |
| `stubVerdict` | `"allow"` \| `"block"` \| `"fail"` | `"block"` | Drives the Phase 2 STUB evaluator in `enforce` mode. `"allow"` → allow verdict; `"block"` → block verdict; `"fail"` → unparseable output (exercises fail-closed). **Test/placeholder only** — not a real classifier; ignored unless `mode: "enforce"`. |
| `promptFile` | string | `""` | **Full-override escape hatch** for the `live` classifier system prompt. If set and readable, its contents are used VERBATIM and **composition is skipped entirely** (no harness-context fragment, no adopter guides are appended). If unset/missing/unreadable, the plugin COMPOSES the prompt from fragments at load time (see "Classifier system-prompt composition" below). Lives in the plugin-config file (not the LLM file) so it MAY be committed as a shared default. |
| `replyMode` | `"once"` \| `"always"` | `"once"` | Controls the reply disposition when the classifier verdict is **allow**. `"once"` = approve this call only (the tool call proceeds; future matching calls still prompt). `"always"` = approve AND **persist the pattern into OpenCode's in-memory allowlist** — future matching calls never prompt again (self-tightening). **`"always"` is powerful**: it auto-allowlists patterns, so a wrong allow verdict permanently silences that pattern for the session. Use `"once"` (the default) unless you are confident in the classifier's precision. Ignored in `audit` mode (no reply is sent). Invalid values → default `"once"`. |
| `onUncertain` | `"reject"` \| `"passthrough"` | `"reject"` | Controls the reply disposition when the classifier **fails or is uncertain** (exception thrown, misconfiguration, undegradable transcript-fetch error, or unrecognized mode). `"reject"` (default, fail-closed) = reply `"reject"`, blocking the call. `"passthrough"` = **no reply** — the Deferred is left unresolved. **⚠ HANG RISK:** `"passthrough"` causes the tool call to HANG in headless/autonomous mode (no human to click). Only use `"passthrough"` in interactive mode where a human is present. Invalid values → default `"reject"`. |
| `harnessContext` | boolean | `true` | When `true` (default), the `auto-gate-harness-context` fragment is COMPOSED after the base classifier prompt — it tells the live model how the `vh-agent-harness exec` wrapper works (see "Classifier system-prompt composition" below). When `false`, the fragment is omitted, yielding the base prompt alone (unless `promptFile` overrides). Invalid/non-boolean → default `true`. |
| `guides` | boolean | `true` | When `true` (default), adopter-supplied guide `*.md` files (see "Classifier system-prompt composition" below) are COMPOSED after the harness-context fragment. When `false`, guides are omitted entirely. Invalid/non-boolean → default `true`. |

### LLM config fields (`auto-gate-llm.json`)

> **Dual-form resolution (literal-preferred).** The endpoint URL and API key
> each support **two forms** — a literal value and an env-var **name**. When
> both forms are specified and non-empty, the **literal wins** and the env var
> is NOT consulted. An empty literal (the default) is treated as "unspecified"
> and falls through to the env form. If neither yields a value at call time,
> the call fail-closes to deny.
>
> | Field | Literal form (wins) | Env-var-name form (fallback) |
> |-------|---------------------|------------------------------|
> | Endpoint | `modelEndpoint` (URL) | `modelEndpointEnv` (env-var name; default `AUTO_GATE_MODEL_ENDPOINT`) |
> | API key | `apiKey` (key value) | `apiKeyEnv` (env-var name; default `AUTO_GATE_API_KEY`) |
>
> **Security note:** a literal `apiKey` puts the key VALUE in the LLM config
> file. At the **project** level the file is gitignored (never committed); at
> the **user** level it lives under `~/.config` (also outside the repo). For
> CI/containers, `apiKeyEnv` remains recommended — the key stays in the
> environment only and never touches disk config.

### Never-commit paths & CI gate

Two auto-gate config file classes are **never committed** and the seed
`.gitignore` ignores them, but `.gitignore` is `project_owned` (seeded on
greenfield, preserved on update), so a repo that installed before the seed fix —
or hand-edited `.gitignore` — can silently commit one:

- `auto-gate-llm.json` — the secrets-adjacent LLM file (may hold a literal
  `apiKey`);
- `.opencode/repo-configs/*.local.json` — the per-developer local-companion
  override convention (auto-gate is the first consumer).

The `auto-gate-ignore` `vh-agent-harness doctor` check detects a tracked or
un-ignored never-commit file (FAIL), flags a tracked `auto-gate-llm.json` with a
non-empty literal `apiKey` as a credential incident (FAIL + rotate guidance — the
key value is never emitted), and WARNs when protection is missing or only via a
non-portable global `core.excludesFile`. It SKIPs when the overlay is unselected
and no config files exist.

**For existing repos — add these two lines to your root `.gitignore` if missing**
(the seed carries them; this is the manual reconciliation path — there is
intentionally no command that auto-edits your `.gitignore`, since that would
violate the ownership contract):

```gitignore
.opencode/repo-configs/*.local.json
.opencode/repo-configs/auto-gate-llm.json
```

If a never-commit file is already **tracked** by git, a `.gitignore` rule does NOT
untrack it — remove it from the index first:

```sh
git rm --cached .opencode/repo-configs/auto-gate-llm.json
git commit -m "chore: stop tracking secrets-adjacent auto-gate-llm.json"
```

**CI gate (required for overlay users).** Run `vh-agent-harness doctor` as a
**pre-stage / pre-package CI gate** so a tracked or un-ignored never-commit file
FAILs the pipeline before it lands. `.gitignore` alone is not a sufficient guard:

- **External CI artifact collectors may ignore `.gitignore`** (e.g. publish the
  whole work tree or copy ignored files into a release tarball). The doctor gate
  is the harness-side guard; review your collector/export steps separately.
- **A disclosed credential cannot be un-leaked by any ignore rule.** If a literal
  `apiKey` was ever committed, tracked, or collected, rotate/revoke it at the
  provider and consider history rewrite if it reached a shared ref. This is an
  owner-driven incident action, not something the harness performs.

| Field | Type | Default | Meaning |
|-------|------|---------|---------|
| `modelEndpoint` | string | `""` | **Literal endpoint URL** for `live` mode (literal-preferred form). The FULL OpenAI-compatible chat-completions URL (e.g. `https://api.provider.example/v1/chat/completions`). Empty (the default) → falls through to `modelEndpointEnv`. If neither yields a value when `mode: "live"` → fail-closed deny. Ignored in other modes. |
| `modelEndpointEnv` | string | `"AUTO_GATE_MODEL_ENDPOINT"` | The **name** of the environment variable holding the endpoint URL (env-var-name fallback form). The URL VALUE is read from `process.env[modelEndpointEnv]` at call time — it is **never** stored in the config file itself. Only consulted when `modelEndpoint` is empty/unspecified. Ignored in other modes. |
| `model` | string | `""` | Required for `live` mode. The model identifier sent in the request body (e.g. `gpt-4o-mini`, a provider alias, etc.). Empty/missing when `mode: "live"` → fail-closed deny. Ignored in other modes. |
| `apiKey` | string | `""` | **Literal API key value** for `live` mode (literal-preferred form). When non-empty, used directly and `apiKeyEnv` is NOT consulted. Empty (the default) → falls through to `apiKeyEnv`. If neither yields a value at call time → fail-closed deny. Ignored in other modes. |
| `apiKeyEnv` | string | `"AUTO_GATE_API_KEY"` | The **name** of the environment variable holding the API key (env-var-name fallback form). The key VALUE is read from `process.env[apiKeyEnv]` at call time. Only consulted when `apiKey` is empty/unspecified. Missing/unset env var at call time → fail-closed deny. Ignored in other modes. |
| `timeoutMs` | number | `8000` | Hard timeout for the `live` model HTTP call, via `AbortController`. On timeout the call fails-closed to deny (after exhausting retries — see `maxRetries`). Ignored in other modes. |
| `maxRetries` | number | `1` | Number of **additional** attempts the `live` call makes after a **transient** failure (timeout / network error / `5xx` / `2xx`-with-empty-content). `0` = single attempt (the pre-retry behavior); `1` (default) = one retry. A `4xx` or malformed-JSON response is **not** retried (retrying won't help). After the final allowed attempt still fails, the call fail-closes to deny. Coerced to a non-negative integer; invalid/missing → `1`. Ignored in other modes. |
| `retryDelayMs` | number | `500` | Base delay between retries, with **linear** backoff: the delay before attempt *N* (N ≥ 2) = `retryDelayMs * (N - 1)`, so attempt 2 waits 1×, attempt 3 waits 2×, etc. Coerced to a non-negative integer; invalid/missing → `500`. Ignored in other modes. |
| `leaves` | array of leaf-config objects | `[]` | **Phase 2 (`live-tiered` only).** An array of per-leaf configs, each a full leaf object `{modelEndpoint, modelEndpointEnv, model, apiKey, apiKeyEnv, timeoutMs, maxRetries, retryDelayMs}`. Each leaf may point at a DIFFERENT endpoint/model — that is the whole point (independent classifiers for consensus). A leaf missing both `modelEndpoint` and `modelEndpointEnv`, or missing `model`, is dropped; if ALL leaves are malformed or the array is empty/missing → fail-closed deny with audit line `live-tiered misconfigured: no leaves`. Ignored in `audit`/`enforce`/`live` modes. |

> **Token-cost note (retries).** Retries only fire on transient failures; each
> retry is a fresh API call that **may consume tokens on the provider side even
> when the prior attempt stalled** (e.g. a request that hangs idle is aborted
> client-side, but the provider may still have processed it). `maxRetries` has
> **no hard upper bound** — it is operator-controlled (the normalizer only floors
> and rejects negatives; any non-negative integer passes), so the cost ceiling
> is the operator's responsibility. The default (`maxRetries: 1`) is conservative
> (the common case costs at most one extra call), but high values multiply token
> cost on **every retryable failure** (e.g. `maxRetries: 999` can issue up to
> ~1000 calls per gated request during a transient outage). Pick the lowest
> value that meets your resilience needs.

Unknown fields are ignored. A field present but of the wrong type or with an
invalid value falls back to that field's default (partial configs are merged
over the defaults field-by-field, so `{"enabled": false}` is valid and leaves
`mode` at its default).

## Classifier system-prompt composition

When `mode` is `live` or `live-tiered`, the classifier reads its system prompt
at load time and **composes it from fragments** rather than reading a single
baked prompt. The composed prompt is:

```
final_prompt = base_prompt                    # auto-gate-classifier (embedded)
             + harness_context_fragment       # auto-gate-harness-context (embedded), unless disabled
             + adopter_guides                 # concatenated *.md from per-level dirs, unless disabled
```

- **base** — the embedded `auto-gate-classifier` fragment served by
  `vh-agent-harness sys-prompt auto-gate-classifier`. Always present (unless
  `promptFile` overrides the whole prompt — see below).
- **harness context** — the embedded `auto-gate-harness-context` fragment served
  by `vh-agent-harness sys-prompt auto-gate-harness-context`. Describes the
  `vh-agent-harness exec` wrapper contract, the shell-guard deny-list floor, and
  git→committer routing so the classifier understands the environment. Omitted
  when `harnessContext: false`.
- **adopter guides** — every `*.md` file found in (a) the user-level guide dir
  and (b) the project-level guide dir (see locations below), concatenated.
  Omitted when `guides: false` or no guide files exist.

Guide directory locations (mirroring the config layering):

- **user-level:** `<userConfigDir>/vh-agent-harness/auto-gate-classifier-guides/*.md`
  where `<userConfigDir>` is `XDG_CONFIG_HOME` if set, else `~/.config`.
- **project-level:** `<repoRoot>/.opencode/sys-prompts/auto-gate-classifier-guides/*.md`

Ordering is **deterministic**: user-level guides first, then project-level
guides, alphabetical by filename within each level. (Project content sits closer
to the user's message, mirroring the config precedence project > user > default.)

Each fragment is introduced by a delimiter so the LLM sees distinct sections.
The delimiter is the ONLY thing the composer injects — each fragment owns its
own heading (delimiter-only injection, mirroring how adopter guides work):

- `<!-- harness-context -->` before the harness-context block. The
  `## Harness execution context` heading comes from the fragment itself, not
  the composer (no injected heading, so no double-H2).
- `<!-- adopter-guide: <level>/<filename> -->` before each adopter guide.

### The wrapper is context, not a bypass

The harness-context fragment exists to teach the classifier a critical rule:
**`vh-agent-harness exec` is context, not a bypass.** A destructive payload
(`vh-agent-harness exec bash -c 'rm -rf …'`) is still destructive — the wrapper
roots the working directory and applies the shell-guard deny-list floor, but it
does **not** sanitize intent. The classifier must judge the **payload**, not the
wrapper. Conversely, an unwrapped mutating command (raw `bash -c '…'` with no
`vh-agent-harness exec`) bypasses the harness's hygiene and warrants **more**
scrutiny, not less. Adopter guides written against this model should treat the
wrapper as environmental context, never as an allow signal.

### `promptFile` — full-override escape hatch

Setting `promptFile` (in either config file) to a readable file path uses that
file's contents verbatim and **skips composition entirely** — neither the
harness-context fragment nor adopter guides are appended. This is the escape
hatch for adopters who want full control over the prompt (the e2e suite relies
on this path).

> **The API key is env-only by default, never in a file.** The RECOMMENDED
> form is the env-var **name** (`apiKeyEnv`, default `AUTO_GATE_API_KEY`) — the
> actual secret is supplied at runtime via that env var and is read fresh on
> every `live` call. A literal `apiKey` form IS supported (literal-preferred
> dual-form), but it places the key VALUE in the LLM config file. That file is
> gitignored at the project level and lives under `~/.config` at the user
> level (so it won't be committed), but `apiKeyEnv` remains recommended for
> CI/containers where the key should stay in the environment only.

> **Backward-compat note (clean cut):** the single-file predecessor held all
> fields in `auto-gate-config.json`. The split ignores any LLM fields
> (`modelEndpoint`/`modelEndpointEnv`/`model`/`apiKey`/`apiKeyEnv`/`timeoutMs`/`maxRetries`/`retryDelayMs`)
> left in the plugin-config file — they MUST come from `auto-gate-llm.json`. This
> is a freshly-shipped pilot with no real install base, so a clean cut (no
> deprecation fallback) keeps the two files strictly disjoint.

### Examples

Plugin config — minimal (audit, the default):

```json
{
  "enabled": true,
  "mode": "audit"
}
```

Plugin config — kill switch (takes effect on the next tool call, no restart):

```json
{
  "enabled": false
}
```

Plugin config — `enforce` mode (Phase 2 stub decision path):

```json
{
  "enabled": true,
  "mode": "enforce",
  "stubVerdict": "block"
}
```

LLM config — `live` mode (Phase 3b real model call); create this file locally
and keep it out of git. Either form works for endpoint and key — literal is
preferred when both are present:

```json
{
  "modelEndpoint": "https://api.provider.example/v1/chat/completions",
  "model": "your-model-id",
  "apiKeyEnv": "AUTO_GATE_API_KEY",
  "timeoutMs": 8000,
  "maxRetries": 1,
  "retryDelayMs": 500
}
```

(with the key exported in the environment: `export AUTO_GATE_API_KEY=sk-...`)

Or the env-var-name form for the endpoint (URL via env var) and a literal key:

```json
{
  "modelEndpointEnv": "AUTO_GATE_MODEL_ENDPOINT",
  "model": "your-model-id",
  "apiKey": "sk-literal-key-here"
}
```

(with `export AUTO_GATE_MODEL_ENDPOINT=https://...`)

### Fail-safe behavior

> **Layered note:** the fail-safe rules below apply at EACH level independently
> (user-level, committed-project-level, and project-local `.local.json`). A
> missing/invalid file at one level is silently skipped and the lower levels
> apply. The project-local `.local.json` is OPTIONAL and absent by default
> (absent = legacy three-level behavior). All-missing → defaults.

**Plugin config** (`auto-gate-config.json`): if the file is **missing,
unreadable, or invalid JSON**, the plugin falls back to the hardcoded defaults
(`{enabled: true, mode: "audit", stubVerdict: "block", promptFile: "",
replyMode: "once", onUncertain: "reject", harnessContext: true, guides: true}`)
and
emits **one** `console.error` audit line noting the fallback — so the operator
learns their config isn't loading without the log being spammed every call. The
plugin **never** throws on a config error; it keeps working with defaults. The
fallback warning is de-duplicated per failure **state**: a transition (missing
→ present → unreadable → invalid) re-warns once, but a persistent failure logs
only on the first occurrence.

**LLM config** (`auto-gate-llm.json`): a **missing** file is **silent** (no
audit spam) — it is the normal case when an operator has not set up live mode;
`audit`/`enforce` modes never fail because the LLM file is absent. Only a
**present-but-invalid** (unreadable / invalid JSON) file emits one audit line,
mirroring the plugin-config handling. Defaults: `{modelEndpoint: "",
modelEndpointEnv: "AUTO_GATE_MODEL_ENDPOINT", model: "", apiKey: "",
apiKeyEnv: "AUTO_GATE_API_KEY", timeoutMs: 8000, maxRetries: 1, retryDelayMs:
500}`. In `live` mode, an endpoint that cannot be resolved from either form
(empty literal + unset env var) or an empty `model` fail-closes to deny via
the existing decision path.

### Reserved for later phases (not yet implemented)

These fields are **planned** but not read by the plugin today. They are listed
here so the operator knows the forward shape; do not depend on them yet —
including them in the config file is harmless (they are ignored):

- `thresholds` (object) — verdict confidence / length thresholds, for a later
  phase.
- `fastPathAllowlist` (array) — tool/pattern allowlist to skip the classifier,
  for a later phase.

## Event hook enforcement flow (the mechanism that makes it real)

The `event` hook is the **enforcement surface** that makes `enforce`/`live`
modes actually auto-approve (or auto-reject) tool calls against stock OpenCode.
The flow (proven by three shipped OpenCode reference implementations: ACP agent,
`opencode run --dangerously-skip-permissions`, TUI):

1. A tool call that the permission table routes to "ask" causes OpenCode's
   `Permission.ask` to store a `Deferred` in `pending` (keyed by request `id`),
   publish the `permission.asked` bus event, and then await the Deferred.
2. The plugin's `event` hook receives the `permission.asked` event. Its
   `event.properties` is the `Request`:
   `{id, sessionID, permission, patterns, metadata, always, tool}`.
3. The hook reads `readConfig()`; if `enabled === false` it returns immediately
   (kill-switch). Otherwise it dispatches on `config.mode`:
   - **`audit`** — logs the event (scrubbed: type/patterns summary) and **does
     NOT reply**. The human still decides. This is observe-only and is **not
     for autonomous operation** (no reply → the Deferred hangs in headless mode
     until a human clicks).
   - **`enforce`** — runs `decidePermission(config)` (the stub classifier). If
     `result.status === "allow"` → replies `config.replyMode` (`"once"` or
     `"always"`); otherwise → replies `"reject"`.
   - **`live`** — merges plugin + LLM config, fetches the transcript via
     `client.session.messages(...)`, serializes it, runs `decideLive(...)` (the
     real HTTP classifier). If `result.status === "allow"` → replies
     `config.replyMode`; otherwise → replies `"reject"`.
4. The reply is sent via two routes depending on disposition:
   - **Allow** (`"once"` / `"always"`) — via the v1 SDK method:
     ```js
     client.postSessionIdPermissionsPermissionId({
         path: { id: req.sessionID, permissionID: req.id },
         body: { response: "once" | "always" },
     });
     ```
     This hits `POST /session/{id}/permissions/{permissionID}` →
     `Permission.reply` → resolves the Deferred → `Permission.ask` unblocks →
     the tool call proceeds.
   - **Reject with reason** — via the v2 route, reusing the same in-process
     transport as the v1 client (`client._client.post`):
     ```js
     client._client.post({
         url: "/permission/" + encodeURIComponent(req.id) + "/reply",
         body: { reply: "reject", message: reason },
         headers: { "Content-Type": "application/json" },
     });
     ```
     This hits `POST /permission/{requestID}/reply` (the same route the TUI's
     RejectPrompt uses with feedback) → `Permission.reply({message})` → resolves
     the Deferred with a `CorrectedError` carrying the feedback. The **reason
     is threaded from every deny path**: the stub verdict reason (`enforce`),
     the parsed `<reason>` (`live`), the aggregate audit (`live-tiered`), or a
     fail-closed message (`onUncertain`). See "Per-call gate" below for why
     this matters.
5. `"once"` approves this call only; `"always"` persists the pattern into
   OpenCode's in-memory allowlist (future matching calls never prompt —
   self-tightening); `"reject"` denies.

**Uncertainty / failure policy:** on classifier failure, misconfiguration, or
any unrecognized state, the hook consults `config.onUncertain`. The default
`"reject"` fail-closes (replies `"reject"` with a fail-closed reason). The
`"passthrough"` alternative does NOT reply (intended for interactive mode where
a human is present — **it hangs in headless mode**).

**Headless hang warning (repeated for emphasis):** if NO ONE replies to a
`permission.asked` event, the Deferred never resolves and the tool call hangs.
In autonomous mode the plugin MUST reply. `"no reply"` is only safe in
interactive mode.

## Per-call gate: reject carries a reason

A reject is a **per-call gate**, not a session-kill switch. When the classifier
denies a tool call, the plugin sends the reject **with a reason** through the
v2 route (`POST /permission/:requestID/reply` with `{reply:"reject",
message:<reason>}`). Upstream, this produces a `CorrectedError` (not a
`RejectedError`):

- **`CorrectedError`** (reject WITH message) — the tool call is blocked, but the
  agent turn **continues**. The model sees the reason as tool-error feedback on
  the next step and can adapt (retry differently, explain, or stop).
- **`RejectedError`** (reject WITHOUT message) — the tool call is blocked AND,
  under default config, the agent turn **ends** (`ctx.blocked = true` →
  `process` returns `"stop"` → the outer loop breaks). The model never gets to
  react to the denial.

The v1 route (`POST /session/:id/permissions/:permissionID`) only forwards
`response`, never `message` — so any reject through it is a bare
`RejectedError`. The plugin uses the v2 route specifically to attach a reason,
switching the error class to `CorrectedError` and keeping the turn alive. This
is what makes the gate **per-call**: block the one call, tell the model why, let
it retry differently.

### Cascade caveat (parallel tool calls)

On a reject, upstream also fails **other pending permission requests in the same
session** with `RejectedError` (not `CorrectedError`). If the model made
**parallel tool calls** whose permission requests are concurrently pending, a
reject on one can cascade-fail the others with `RejectedError` — which under
default config CAN still trip the session-kill path for those other calls.

This is an upstream design constraint, not a plugin bug. Mitigations:

- **Prefer sequential tool calls** over parallel ones when the gate is active
  (avoids concurrent permission requests).
- **Set `experimental.continue_loop_on_deny: true`** in session config as
  defense-in-depth (see below).

### Recommended: `experimental.continue_loop_on_deny: true`

As operator-side defense-in-depth, set this in the OpenCode session config:

```json
{
  "experimental": {
    "continue_loop_on_deny": true
  }
}
```

This makes `ctx.shouldBreak = false` even for `RejectedError`, so a deny never
kills the turn regardless of error class. The plugin does NOT set this itself
(it is session config, operator-side) — it is recommended alongside the
per-call-gate fix so that even the cascade edge case cannot end the turn.

### v2-route server-error → fail-safe kill-switch (no hang)

The reject-with-reason path goes through the v2 route to attach a message
(`CorrectedError`, per-call gate). If the v2 route itself returns a
**server-error response** (the pending entry is not found, or the route handler
throws), the helper does **not** leave the permission Deferred unresolved — a
hanging tool call (`ctx.ask` awaits forever) is strictly worse than a
kill-switch. Instead the helper **falls back to the v1 bare reject**, which
resolves the Deferred with a `RejectedError` (turn ends under default config).

This is an intentional **fail-safe degradation**: the per-call-gate property
holds on the v2 success path (turn continues, model sees the reason); the rare
v2-error path degrades to kill-switch (turn ends). It remains **fail-closed**
either way — the tool call is blocked, never silently allowed. The same
fallback already applies when no v2 transport is available at all.

## Enforce mode (Phase 2)

`mode: "enforce"` switches the `event` hook from observability into a
deterministic, **fail-closed** decision path that **auto-approves or
auto-rejects** tool calls by replying to the `permission.asked` bus event. The
`audit` default is unchanged (see below). The `permission.ask` hook remains
dormant.

### What it does

When a `permission.asked` event reaches the `event` hook and
`mode === "enforce"`:

1. the event is audit-logged (permission type + patterns summary +
   `mode=enforce`, all scrubbed),
2. `decidePermission(config)` runs: the stub evaluator produces raw verdict
   text, `parseVerdict` greps the first `<block>yes|no` token, and the decision
   matrix produces a `status`,
3. `allow` verdict → reply `config.replyMode` (`"once"` or `"always"`) — the
   tool call proceeds; `"always"` additionally persists the pattern,
4. `block` verdict → reply `"reject"` (the reason is audit-logged),
5. unparseable verdict / evaluator error / thrown exception → **fail-closed**
   reply `"reject"` (or no-reply if `onUncertain:"passthrough"`).

The `stubVerdict` config field drives the stub evaluator's raw output
(`"allow"` / `"block"` / `"fail"`), so all branches are exercisable without a
model.

### It is a STUB, not a real classifier

The Phase 2 evaluator (`stubEvaluate`) is a **deterministic placeholder** — it
returns canned verdict text based only on `stubVerdict`. It makes NO assessment
of the actual tool call. **Do not enable `enforce` against real traffic until
Phase 3 wires a live classifier model.** The stub is for exercising and
regression-testing the decision path only.

### Fail-closed to deny on any uncertainty

Every indeterminate path replies `"reject"` (or no-reply if
`onUncertain:"passthrough"`) — the gate degrades to reject, never silently
allows: parse failure (no `<block>` token), an invalid disposition
(`<block>maybe</block>`), an evaluator that throws, or any exception in the
decision path all yield a `"reject"` reply.

### The hard floor is never overridden

The `permission.asked` event fires **only** for calls opencode's permission
table routes to `ask`. Table-`allow` fast-paths past the event; table-`deny` /
shell-guard blocks **before** the event is published. Therefore the classifier
decision can only ever resolve an `ask` to allow/deny — it can **never**
override a static deny, because a statically-denied call never reaches the event
hook. The classifier only ever decides the ask-routed subset; the static
permission table (plus shell-guard) remains the hard floor.

### `audit` mode is unchanged

Switching back to `mode: "audit"` (the default) restores the exact Phase 1
behavior: the event hook logs only and does NOT reply, `tool.execute.before`
logs only, and the interactive ask still fires. The `enforce` and `live`
branches are separate code paths that do not touch the audit branch.

## Live mode (Phase 3b)

`mode: "live"` switches the `event` hook from the stub decision path to a REAL
classifier model call. It reuses the **same fail-closed decision matrix** as
`enforce`; only the evaluator changes (stub → live HTTP adapter). The reply
dispositions are identical: allow → `replyMode` reply; deny → `"reject"`. The
`audit` default is unchanged (see below).

### What it does

When a `permission.asked` event reaches the `event` hook and `mode === "live"`:

1. the event is audit-logged (permission type + patterns summary +
   `mode=live`, all scrubbed),
2. live config is validated — an endpoint that cannot be resolved from either
   form (empty literal + unset env var) or a missing `model` fails-closed
   to `"reject"` with a clear audit line (`live mode misconfigured:
   no modelEndpoint` / `no model`),
3. the session transcript is fetched via the SDK client
   (`client.session.messages(...)`) — **on any fetch failure the gate degrades
   gracefully** to the permission payload alone (it does NOT fail-closed on a
   transcript-fetch error; the model still gets the type+pattern to judge),
4. the transcript is serialized to a text-mode string (User: / Assistant: /
   Tool: prefixes) with the following egress surface (see "Egress surface and
   credential scrubbing" below for the precise detail):
   - **Tool inputs** — redacted via an **allowlist** (command / path / pattern /
     query / url / workdir only, truncated; every other field is omitted; tool
     **results / outputs are NEVER transmitted**),
   - **User/assistant text + reasoning + delegation descriptions** —
     **credential-scrubbed** (API keys, tokens, bearer auth, high-entropy blobs
     → `[redacted]`) and **truncated**, but otherwise transmitted so the
     classifier has the conversation context it needs to judge actions,
   - the action under evaluation is emphasized at the end,
5. the serialized text is sent to the configured OpenAI-compatible endpoint as
   a chat-completions request (system prompt = the binary-served default via
   `vh-agent-harness sys-prompt auto-gate-classifier`, or the `promptFile`
   override; user message = the serialized transcript), with `temperature: 1`,
   `max_tokens: 64`, `stream: false`. A **transient** failure (timeout /
   network error / `5xx` / `2xx`-with-empty-content) is **retried** up to
   `maxRetries` additional attempts with linear backoff (`retryDelayMs`); a
   `4xx` or malformed-JSON response is not retried,
6. the returned verdict text is parsed by the **same** `parseVerdict` → decision
   matrix as `enforce`,
7. the reply disposition is set from the decision: `allow` → reply
   `config.replyMode`; `deny` → reply `"reject"`. The decision, reason, model
   latency, and retry count (when retries occurred) are audit-logged.

### Fail-closed to deny on any uncertainty

Every indeterminate path replies `"reject"` (or no-reply if
`onUncertain:"passthrough"`) — the gate degrades to reject, never silently
allows. **Transient** failures (transport error, timeout (`timeoutMs`), `5xx`
response, `2xx`-with-empty-content) are first **retried** up to `maxRetries`
additional attempts; only after retries are exhausted (or for a non-retryable
failure: `4xx`, malformed JSON, missing API key, or a thrown exception in the
adapter) does the path reply `"reject"`. An unresolvable endpoint (neither
literal nor env form yields a value), missing `model`, unparseable verdict
(no `<block>` token), or any final adapter failure all yield a `"reject"`
reply. Only an explicit `<block>no</block>` (allow) verdict yields an allow
reply. The retry policy keeps the fail-closed contract intact: a retry never
turns a reject into an allow on its own — it only gives a stalled request a
second chance to return a verdict.

### The API key — dual-form, env-var recommended

`live` mode resolves the key at **call time** using dual-form literal-preferred
resolution: a non-empty literal `apiKey` (in the LLM config) wins; otherwise the
env var named by `apiKeyEnv` (default `AUTO_GATE_API_KEY`) is read from
`process.env`. The RECOMMENDED form is `apiKeyEnv` — the secret stays in the
environment only and never touches disk config. The literal `apiKey` form IS
supported but places the value in `auto-gate-llm.json` (gitignored at project
level, under `~/.config` at user level). Export the key in the server
environment:

```sh
export AUTO_GATE_API_KEY=sk-...
```

### Transcript fetch degrades gracefully (not fail-closed)

A transcript-fetch failure (no client, SDK error, missing data) is a **soft
degradation**, not a fail-closed condition: the gate proceeds with the
permission payload alone (`serializeTranscript([], input)`), so the model still
has the type+pattern to judge. Only the model-call / decision layer fail-closes.

### Egress surface and credential scrubbing

In `live` mode the serialized transcript is sent to the configured
`modelEndpoint` over HTTP. This is an **egress surface**: the operator MUST
point `live` mode at a trusted endpoint and treat the transcript as data
leaving the repo. The serializer reduces — but does NOT eliminate — that risk.
The exact posture:

- **Tool inputs** are redacted by an **allowlist**: only `command` / `path` /
  `pattern` / `query` / `url` / `workdir` are emitted (truncated); every other
  input field is omitted; tool **results (`state.output`) are NEVER
  transmitted**.
- **User/assistant text, reasoning, and delegation/subtask descriptions** are
  passed through a **credential scrubber** (`scrubCredentials`) that masks
  secret-shaped values — API keys (`api_key=` / `apikey:` / `token=` /
  `secret=` / `password=` …), `Bearer <token>` and `Authorization: Bearer …`
  headers, and standalone high-entropy blobs (32+ hex chars, 40+ base64-ish
  chars, `sk-`/`AKIA`-prefixed keys) → `[redacted]`. The key NAME is kept; only
  the VALUE is masked, so the surrounding sentence context (which the
  classifier needs to judge scope-creep / injection) survives. The scrubber is
  **conservative by design**: it prefers false-positive redaction over
  false-negative leakage.
- The scrubbed text is then **truncated** (scrub-before-truncate, so a secret
  split across the truncation boundary is still caught).

> **Egress warning — read before enabling `live`.** The (scrubbed, truncated)
> transcript still leaves the repo. Credential-scrubbing is heuristic: it
> catches standard secret shapes but **can miss non-standard or obfuscated
> secrets** (e.g. a credential reformatted, split, or embedded in prose). Do
> not enable `live` mode against an untrusted endpoint, and prefer an endpoint
> you control for any sensitive repo.

### Provider-agnostic (OpenAI-compatible)

The adapter speaks standard OpenAI chat-completions: `POST ${modelEndpoint}`
with `Authorization: Bearer ${key}` + `Content-Type: application/json`, body
`{model, messages:[{system},{user}], temperature:1, max_tokens:64,
stream:false}`, reading `choices[0].message.content`. Any provider that exposes
an OpenAI-compatible endpoint works. A provider-native (non-OpenAI) format is a
later phase.

### `audit` mode is unchanged

Switching back to `mode: "audit"` (the default) restores the exact Phase 1
behavior. The event hook logs only and does NOT reply. The `live` branch is a
separate code path that does not touch the audit branch.

## Phase 2 — `live-tiered` consensus mode

`mode: "live-tiered"` is an **opt-in** consensus mode that dispatches the same
`live` classifier (Phase 3b substrate) for EACH configured leaf IN PARALLEL and
grants only on **unanimous allow**. It is the multi-leaf generalization of
single-leaf `live`: each leaf may point at a different endpoint/model (independent
classifiers), so a single misbehaving or compromised classifier cannot silently
allow a call that another classifier would block.

### Unanimous-allow policy

The aggregate is computed by the behavior-frozen aggregation core
(`auto-gate-tiered.js`, 47 passing unit tests of its own). The policy is:

- **ALLOW × N (≥1 leaf, all allow)** → **allow** (the ONLY grant path).
- **Any DENY** → **deny**. If at least one leaf also ALLOWed → `disagreement: true`.
- **Any FAIL** (a leaf threw, timed out, returned an unparseable verdict, or hit
  a non-2xx) → **deny** with `incomplete: true`. The dominant reason is
  incompleteness, so `disagreement` is `false` even if both ALLOW and DENY are
  also present.
- **Empty / malformed / missing leaves** → **deny** (fail-closed via `onUncertain`).
- **Unknown leaf outcome** → treated as **FAIL** → deny.

Each leaf's `decideLive` outcome (shape `{status, audit, reason, latencyMs,
retries}`) is normalized by `normalizeLeafOutcome` — `{status:"allow"}` → ALLOW,
`{status:"deny"}` → DENY, anything else → FAIL. No adapter is needed because the
live path already returns exactly this shape.

### Shared transcript, per-leaf endpoints

The transcript is fetched **once** and shared across all leaves (the leaves
differ in LLM endpoint/model, not in what they see). The shared fields
(`promptFile`, `replyMode`, `onUncertain`, the transcript) come from the plugin
config + a single `client.session.messages` fetch. Each leaf gets its OWN
`modelEndpoint` / `modelEndpointEnv` / `model` / `apiKey` / `apiKeyEnv` /
`timeoutMs` / `maxRetries` / `retryDelayMs` from its entry in the `leaves`
array (dual-form literal-preferred resolution applies per leaf).

### Config shape

The plugin config sets `mode: "live-tiered"`. The LLM config carries the
`leaves` array. Example `auto-gate-llm.json`:

```json
{
  "leaves": [
    {
      "modelEndpoint": "https://classifier-a.example/v1/chat/completions",
      "model": "gate-model-a",
      "apiKeyEnv": "CLASSIFIER_A_KEY",
      "timeoutMs": 8000,
      "maxRetries": 1,
      "retryDelayMs": 500
    },
    {
      "modelEndpoint": "https://classifier-b.example/v1/chat/completions",
      "model": "gate-model-b",
      "apiKeyEnv": "CLASSIFIER_B_KEY",
      "timeoutMs": 8000,
      "maxRetries": 1,
      "retryDelayMs": 500
    }
  ]
}
```

Each leaf is a FULL leaf-config object (all eight LLM fields). The top-level
`modelEndpoint`/`model`/etc. are ignored when `leaves` is present. A leaf
missing both `modelEndpoint` and `modelEndpointEnv`, or missing `model`, is
dropped; if that leaves zero valid leaves, the dispatch fail-closes.

### SERVE-ONLY enforcement (run-mode race)

> **Single-leaf `live` already loses the reply race under `opencode run`** (the
> async classifier HTTP path, ~4–14 ms, is slower than `run`'s near-instant
> in-process auto-reply). Multi-leaf consensus (parallel `decideLive` = even
> more latency) loses the run race **worse**. **Therefore `live-tiered`
> consensus enforcement is effectively SERVE-ONLY**, just like single-leaf
> `live`. Under `opencode run`, the consensus reply arrives after `run`'s own
> auto-reply has already resolved the Deferred, so the tool call is NOT gated by
> the consensus verdict. Enforce/stub mode still wins run-mode (it is
> synchronous). Do NOT attempt to fix the run race — it is inherent to
> `opencode run`'s auto-reply design.

### Egress discipline (per-leaf audit)

The aggregate audit line is **constant-shaped**: `tierId` + integer leaf counts
+ normalized-outcome enums only. Leaf endpoint/model/apiKeyEnv VALUES are
**never** interpolated into any log line. The per-leaf summary logged alongside
carries only the normalized outcome enum + integer retries/latency. All
tool-call-derived content continues to pass through the existing
`scrubTruncate`/`scrubCredentials` egress discipline.

## Where the plugin renders

OpenCode **auto-discovers** plugins from `.opencode/plugins/*.js` — there is no
`plugins` key in `opencode.jsonc`, yet `shell-guard.js`, `session-state.js`,
and `maxoutputtokens.js` all load automatically. This pack renders four units
(via the overlay `RenderUnits` walk) into `.opencode/plugins/`:

- `plugins/auto-tool-gate.js` → `.opencode/plugins/auto-tool-gate.js` — the
  plugin (exports `server`); picked up on the next server start.
- `plugins/auto-gate-verdict.js` → `.opencode/plugins/auto-gate-verdict.js` —
  the pure verdict-parse + decision module (does NOT export `server`); a
  plugin dependency imported by `auto-tool-gate.js`. OpenCode tolerates it as a
  non-plugin (same precedent as `shell-guard-core.js`). It is also
  **self-testing**: run `vh-agent-harness exec node --test
  .opencode/plugins/auto-gate-verdict.js` to execute its regression suite
  (importing it as a module runs no tests).
- `plugins/auto-gate-live.js` → `.opencode/plugins/auto-gate-live.js` — the
  Phase 3b live classifier substrate: transcript serializer + binary-served
  system prompt (via `vh-agent-harness sys-prompt auto-gate-classifier`) +
  OpenAI-compatible HTTP adapter + the `decideLive` bridge (does NOT export
  `server`); a plugin dependency imported by `auto-tool-gate.js` only when
  `mode === "live"`. Also **self-testing**: run
  `vh-agent-harness exec node --test .opencode/plugins/auto-gate-live.js` to
  execute its regression suite (importing it as a module runs no tests).
- `plugins/auto-gate-scrub.js` → `.opencode/plugins/auto-gate-scrub.js` — the
  shared credential scrubber (`truncate`, `scrubCredentials`,
  `scrubTruncate`); a plugin dependency imported by both `auto-tool-gate.js`
  (stderr/audit egress) and `auto-gate-live.js` (HTTP-egress transcript). Does
  NOT export `server`. Also **self-testing**: run `vh-agent-harness exec node
  --test .opencode/plugins/auto-gate-scrub.js` to execute its regression suite
  (importing it as a module runs no tests).
- `plugins/auto-gate-tiered.js` → `.opencode/plugins/auto-gate-tiered.js` — the
  Phase 2 multi-leaf consensus aggregation core (`LEAF`, `normalizeLeafOutcome`,
  `aggregateLeafOutcomes`); a plugin dependency imported by `auto-tool-gate.js`
  only when `mode === "live-tiered"`. Does NOT export `server`. Also
  **self-testing**: run `vh-agent-harness exec node --test
  .opencode/plugins/auto-gate-tiered.js` to execute its regression suite
  (importing it as a module runs no tests).

No `opencode.jsonc` registration is needed (this pack's
`opencode-append.jsonc` is intentionally empty for that reason).

## Naming

All identifiers are **generic** — `auto-tool-gate`, `auto-gate-audit`,
`auto-classifier-pilot`. The upstream mechanism is referred to only as "the
reference agent system" / "a security-monitor classifier". No product names
appear anywhere in this pack.

## Design lineage

The verdict protocol, system-prompt anatomy, and transcript-serialization shape
were derived from the **structure** of a reference agent system's auto/classifier
permission mode (a security-monitor LLM tool-call gate). The wording throughout
this pack is original; nothing is copied or paraphrased from any proprietary
bundle. All identifiers are generic.
