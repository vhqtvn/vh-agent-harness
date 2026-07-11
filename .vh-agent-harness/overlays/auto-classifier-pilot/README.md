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

### Composition model (when each layer fires)

Per tool call, OpenCode resolves permission in this order:

```
permission config table
├─ table "allow"    → tool runs; no permission.asked event fires (fast-path)
├─ table "deny"     → blocked; no event fires (shell-guard / hard floor)
└─ table "ask" / no-match
   └─ permission.asked event published → event hook classifies → replies
      via SDK client → Deferred resolves → tool proceeds or is blocked
```

So the `permission.asked` event fires **only** for the ask-routed subset; the
static table is the first gate. `tool.execute.before` is orthogonal — it
observes the full tool-call stream regardless of how the table resolved it.

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

This pack is **opt-in** — it is not selected by default.

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
and no corpus re-render. Config is loaded from a **three-level layered model**
(user-level base, project-level override; see below).

This works because the plugin does runtime file I/O inside each hook (mirroring
how `shell-guard.js` imports `node:fs` + `node:path`). The OpenCode plugin SDK
has **no native hot-reload config API** — the `config` hook and `PluginOptions`
are load-time (set at server start) and env vars are frozen at process start —
so a per-call disk read gated by an **mtime cache** is the reload-free
mechanism: in steady state an unchanged file costs only a single `statSync` per
call, and a changed file is re-read + re-parsed only when its mtime changes.

### Two-file model (config split)

Config is split across **two sibling files** under `.opencode/repo-configs/`
(resolved relative to the repo root, the same way `shell-guard-core.js` derives
`repoRoot()` — from the plugin file's location, never a hardcoded absolute
path). The split exists so that **LLM settings can NEVER be committed** while
**plugin behavior MAY be committed** (or not) at the adopter's choice:

| File | Fields | Committability |
|------|--------|----------------|
| `auto-gate-config.json` | `enabled`, `mode`, `stubVerdict`, `promptFile` | **Adopter's choice** — a team may commit a shared default (e.g. `{"mode":"enforce"}`). NOT gitignored. |
| `auto-gate-llm.json` | `modelEndpoint`, `model`, `apiKeyEnv`, `timeoutMs`, `maxRetries`, `retryDelayMs` | **NEVER** — gitignored in this dogfood repo. Adopters using live mode create it locally and add the pattern to their own `.gitignore`. |

Neither file is rendered or seeded by the overlay. Leaving a file absent is the
documented fail-safe default — the plugin works out of the box with built-in
defaults.

> **Why the split?** The single-file predecessor mixed LLM endpoint settings
> with plugin behavior in one committable file. The split guarantees LLM config
> (which points at an external endpoint) can never leak into a shared repo,
> while still letting a team share a plugin-behavior default like
> `{"mode":"enforce"}`.

### Three-level layered loading (user-level base, project-level override)

Each config type (plugin-behavior and LLM) is loaded from **up to two locations**
and merged **field-by-field** with this precedence:

```
defaults  ←  user-level  ←  project-level    (project wins per field)
```

| Level | Plugin config path | LLM config path |
|-------|-------------------|-----------------|
| **User** (shared across all projects) | `<XDG_CONFIG_HOME>/vh-agent-harness/auto-gate-config.json` | `<XDG_CONFIG_HOME>/vh-agent-harness/auto-gate-llm.json` |
| **Project** (per-repo, unchanged) | `.opencode/repo-configs/auto-gate-config.json` | `.opencode/repo-configs/auto-gate-llm.json` |

`XDG_CONFIG_HOME` resolves as `process.env.XDG_CONFIG_HOME || path.join(os.homedir(), ".config")`
(typically `~/.config`). The user-level directory is `vh-agent-harness/` — **not**
`opencode/` — because vh-agent-harness ships the plugin and defines the schema.
The filenames mirror the project-level names so the override relationship is
obvious: "same file, user-level base, project-level override."

**Merge rules:**

- A **missing** file at either level is silently skipped (its absence is normal).
  Both-missing → fail-safe defaults (exactly the pre-layering behavior).
- A **present-but-invalid** file (bad JSON, non-object shape) is also skipped —
  the other layer applies, and a deduped audit line is emitted with the level
  in the label (e.g. `plugin/user`, `plugin/project`, `llm/user`, `llm/project`).
- When a project-level file is **present and valid**, it overrides the
  user-level **field-by-field** — only the fields it explicitly sets win;
  fields it omits fall through to the user-level, then to defaults. So a
  project `{"model": "x"}` keeps the user-level endpoint/key and changes only
  the model. (This is the common case for specializing one field per repo.)
- The mtime cache tracks **both** files; changing **either** one invalidates
  the cache and triggers a re-read + re-merge on the next hook call.

**Typical use:** put a shared endpoint/model/key-env in the user-level
`auto-gate-llm.json` once, and it applies to every project. A project that needs
its own override drops a project-level file — that file is then authoritative.

Example — user-level LLM config (applies to all projects without one):

```json
{
  "modelEndpoint": "https://api.provider.example/v1/chat/completions",
  "model": "gpt-4o-mini",
  "apiKeyEnv": "MY_GATE_KEY"
}
```

### Plugin config fields (`auto-gate-config.json`)

| Field | Type | Default | Meaning |
|-------|------|---------|---------|
| `enabled` | boolean | `true` | Master live toggle. `false` live-disables the plugin: both hooks no-op immediately on the next tool call (no audit, no behavior change, no restart). `true` is the normal on state. |
| `mode` | `"audit"` \| `"enforce"` \| `"live"` \| `"live-tiered"` | `"audit"` | Behavior selector. `audit` = Phase 1 observability only (default, zero behavior change). `enforce` = Phase 2 decision path on `permission.ask` (verdict parser + STUB evaluator; fail-closed to deny). `live` = Phase 3b decision path using a REAL OpenAI-compatible model call (fail-closed to deny on any error/timeout/misconfiguration). `live-tiered` = Phase 2 multi-leaf consensus: dispatches the `live` classifier for EACH configured leaf IN PARALLEL and grants only on **unanimous-allow** (see "Phase 2 — `live-tiered` consensus mode"). `tool.execute.before` stays an observer in all modes. |
| `stubVerdict` | `"allow"` \| `"block"` \| `"fail"` | `"block"` | Drives the Phase 2 STUB evaluator in `enforce` mode. `"allow"` → allow verdict; `"block"` → block verdict; `"fail"` → unparseable output (exercises fail-closed). **Test/placeholder only** — not a real classifier; ignored unless `mode: "enforce"`. |
| `promptFile` | string | `""` | Optional override path for the `live` classifier system prompt. If set and readable, its contents replace the binary-served default prompt. If unset/missing/unreadable, the plugin loads the prompt from `vh-agent-harness sys-prompt auto-gate-classifier`. Lives in the plugin-config file (not the LLM file) so it MAY be committed as a shared default. |
| `replyMode` | `"once"` \| `"always"` | `"once"` | Controls the reply disposition when the classifier verdict is **allow**. `"once"` = approve this call only (the tool call proceeds; future matching calls still prompt). `"always"` = approve AND **persist the pattern into OpenCode's in-memory allowlist** — future matching calls never prompt again (self-tightening). **`"always"` is powerful**: it auto-allowlists patterns, so a wrong allow verdict permanently silences that pattern for the session. Use `"once"` (the default) unless you are confident in the classifier's precision. Ignored in `audit` mode (no reply is sent). Invalid values → default `"once"`. |
| `onUncertain` | `"reject"` \| `"passthrough"` | `"reject"` | Controls the reply disposition when the classifier **fails or is uncertain** (exception thrown, misconfiguration, undegradable transcript-fetch error, or unrecognized mode). `"reject"` (default, fail-closed) = reply `"reject"`, blocking the call. `"passthrough"` = **no reply** — the Deferred is left unresolved. **⚠ HANG RISK:** `"passthrough"` causes the tool call to HANG in headless/autonomous mode (no human to click). Only use `"passthrough"` in interactive mode where a human is present. Invalid values → default `"reject"`. |

### LLM config fields (`auto-gate-llm.json`)

| Field | Type | Default | Meaning |
|-------|------|---------|---------|
| `modelEndpoint` | string | `""` | Required for `live` mode. The FULL OpenAI-compatible chat-completions URL (e.g. `https://api.provider.example/v1/chat/completions`). Empty/missing when `mode: "live"` → fail-closed deny with audit line `live mode misconfigured: no modelEndpoint`. Ignored in other modes. |
| `model` | string | `""` | Required for `live` mode. The model identifier sent in the request body (e.g. `gpt-4o-mini`, a provider alias, etc.). Empty/missing when `mode: "live"` → fail-closed deny. Ignored in other modes. |
| `apiKeyEnv` | string | `"AUTO_GATE_API_KEY"` | The **name** of the environment variable holding the API key for `live` mode. The key VALUE is read from `process.env[apiKeyEnv]` at call time — it is **never** stored in either config file. Missing/unset env var at call time → fail-closed deny. Ignored in other modes. |
| `timeoutMs` | number | `8000` | Hard timeout for the `live` model HTTP call, via `AbortController`. On timeout the call fails-closed to deny (after exhausting retries — see `maxRetries`). Ignored in other modes. |
| `maxRetries` | number | `1` | Number of **additional** attempts the `live` call makes after a **transient** failure (timeout / network error / `5xx` / `2xx`-with-empty-content). `0` = single attempt (the pre-retry behavior); `1` (default) = one retry. A `4xx` or malformed-JSON response is **not** retried (retrying won't help). After the final allowed attempt still fails, the call fail-closes to deny. Coerced to a non-negative integer; invalid/missing → `1`. Ignored in other modes. |
| `retryDelayMs` | number | `500` | Base delay between retries, with **linear** backoff: the delay before attempt *N* (N ≥ 2) = `retryDelayMs * (N - 1)`, so attempt 2 waits 1×, attempt 3 waits 2×, etc. Coerced to a non-negative integer; invalid/missing → `500`. Ignored in other modes. |
| `leaves` | array of leaf-config objects | `[]` | **Phase 2 (`live-tiered` only).** An array of per-leaf configs, each a full leaf object `{modelEndpoint, model, apiKeyEnv, timeoutMs, maxRetries, retryDelayMs}`. Each leaf may point at a DIFFERENT endpoint/model — that is the whole point (independent classifiers for consensus). A leaf missing `modelEndpoint` or `model` is dropped; if ALL leaves are malformed or the array is empty/missing → fail-closed deny with audit line `live-tiered misconfigured: no leaves`. Ignored in `audit`/`enforce`/`live` modes. |

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

> **The API key is env-only, never in a file.** Neither config file carries the
> secret value — both hold at most the env-var **name** (`apiKeyEnv`, default
> `AUTO_GATE_API_KEY`). The actual secret is supplied at runtime via that env
> var and is read fresh on every `live` call. Never paste the key value into
> either config file.

> **Backward-compat note (clean cut):** the single-file predecessor held all
> fields in `auto-gate-config.json`. The split ignores any LLM fields
> (`modelEndpoint`/`model`/`apiKeyEnv`/`timeoutMs`/`maxRetries`/`retryDelayMs`)
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
and keep it out of git:

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

### Fail-safe behavior

> **Layered note:** the fail-safe rules below apply at EACH level independently
> (user-level and project-level). A missing/invalid file at one level is
> silently skipped and the other level applies. Both-missing → defaults.

**Plugin config** (`auto-gate-config.json`): if the file is **missing,
unreadable, or invalid JSON**, the plugin falls back to the hardcoded defaults
(`{enabled: true, mode: "audit", stubVerdict: "block", promptFile: "",
replyMode: "once", onUncertain: "reject"}`) and
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
mirroring the plugin-config handling. Defaults: `{modelEndpoint: "", model: "",
apiKeyEnv: "AUTO_GATE_API_KEY", timeoutMs: 8000, maxRetries: 1, retryDelayMs:
500}`. In `live` mode, an empty `modelEndpoint`/`model` fail-closes to deny via
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
2. live config is validated — a missing `modelEndpoint` or `model` fails-closed
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
adapter) does the path reply `"reject"`. A missing `modelEndpoint`/`model`,
unparseable verdict (no `<block>` token), or any final adapter failure all yield
a `"reject"` reply. Only an explicit `<block>no</block>` (allow) verdict yields
an allow reply. The retry policy keeps the fail-closed contract intact: a retry
never turns a reject into an allow on its own — it only gives a stalled request
a second chance to return a verdict.

### The API key comes from the environment, never a config file

`live` mode reads the key from `process.env[apiKeyEnv]` (default
`AUTO_GATE_API_KEY`) **at call time**. Neither config file carries the secret
value — the LLM config file (`auto-gate-llm.json`) holds only the env-var
**name** via `apiKeyEnv`, and that file is never committed (gitignored). Export
the key in the server environment:

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
`modelEndpoint` / `model` / `apiKeyEnv` / `timeoutMs` / `maxRetries` /
`retryDelayMs` from its entry in the `leaves` array.

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

Each leaf is a FULL leaf-config object (all six LLM fields). The top-level
`modelEndpoint`/`model`/etc. are ignored when `leaves` is present. A leaf
missing `modelEndpoint` or `model` is dropped; if that leaves zero valid leaves,
the dispatch fail-closes.

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

## Design source

- `researches/sources/2026-07-10-auto-mode-classifier-source-packet.local.md` — the
  design packet for the reference agent system's
  auto/classifier permission mode (the security-monitor LLM tool-call gate).
  The porting-notes section (§10) frames the later phases.
