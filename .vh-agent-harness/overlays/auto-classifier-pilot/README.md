# auto-classifier-pilot

An **opt-in** overlay pack that ships a dual-surface plugin — the pilot for an
auto-classifier-style tool-call gate. It implements three behavior modes,
selected by a live config `mode` field (default `audit`):

- **`audit` (Phase 1, default)** — observability only. Zero behavior change.
- **`enforce` (Phase 2)** — runs a deterministic, fail-closed decision path on
  `permission.ask` (verdict parser + STUB evaluator). **Not a real classifier
  model** — for exercising the decision path only.
- **`live` (Phase 3b)** — fetches the real session transcript, serializes it
  (credential-scrubbed + truncated; tool inputs redacted via an allowlist) to a
  text-mode string, and calls a **provider-agnostic OpenAI-compatible HTTP**
  classifier endpoint. The returned verdict feeds the **same fail-closed
  decision matrix** as `enforce`. The model API key is read from an env var at
  call time — it never lives in the (commitable) config file.

## What this is

The plugin (`auto-tool-gate`) hooks **both** OpenCode permission surfaces. In
the default **`audit`** mode it writes one audit line to **stderr** per firing,
capturing a verdict placeholder (`verdict=AUDIT_ONLY`). It never throws, never
blocks, never mutates a tool call, and never mutates a permission decision.
**Enabling this overlay with the default mode changes zero behavior** — it only
adds stderr audit lines.

The two surfaces it observes:

1. **`tool.execute.before`** — fires for **every** tool call (including ones the
   permission table auto-allows). Per firing it logs:
   - the **tool name** (`input.tool`),
   - a **short, redacted argument summary** (command / path / pattern / query /
     url / workdir, truncated; full payloads are never dumped),
   - `verdict=AUDIT_ONLY`.
2. **`permission.ask`** — fires only for calls the permission table routes to an
   interactive ask (table-`allow` calls fast-path past it; table-`deny` /
   shell-guard blocks before it). Per firing it logs:
   - the permission **type** (`input.type` — `bash` / `edit` / …),
   - the **pattern** (`input.pattern` — the command/path string, truncated),
   - the **incoming** `output.status` (the default decision, expected `"ask"`),
   - `verdict=AUDIT_ONLY`.

It then **unconditionally `return`s** on both hooks, leaving `output.status`
untouched so the normal interactive ask still fires.

The goal of Phase 1 is to **prove both seams and the context-capture**, and to
answer the dual-hook question (see "Open caveat" below): which tools route to
`permission.ask` versus get fast-pathed by the permission table? That answer is
a precondition for Phase 2, which must own the authoritative decision hook.

## Decision architecture

### The two-hook model

OpenCode exposes **two** plugin hooks with different powers (verified against
`@opencode-ai/plugin` + `@opencode-ai/sdk` types):

| Hook | Input | Output | Powers |
|------|-------|--------|--------|
| `tool.execute.before` | `{tool, sessionID, callID}` | `{args}` | **block (throw)** or **passthrough (bare return)** only. Cannot force-allow, cannot force-ask. Sees EVERY tool call. |
| `permission.ask` | `Permission {id, type, pattern, sessionID, messageID, callID?, title, metadata, time}` | `{status}` | The **authoritative** three-way decision. `status:"allow"` grants AND skips the user prompt; `status:"deny"` blocks; `status:"ask"` (default) triggers the interactive prompt. Fires only for ask-routed calls. |

`permission.ask` maps **exactly** onto the reference classifier's three
dispositions (allow / deny / ask). That is why it — not `tool.execute.before` —
is the hook the classifier must own in Phase 2/3. `tool.execute.before` is kept
in Phase 1 because it sees the calls `permission.ask` does not (the
table-allowed fast-path).

### Composition model (when each layer fires)

Per tool call, OpenCode resolves permission in this order:

```
permission config table
├─ table "allow"    → tool runs; NEITHER hook fires  (fast-path)
├─ table "deny"     → blocked; NEITHER hook fires    (shell-guard / hard floor)
└─ table "ask" / no-match
   └─ permission.ask hook fires → classifier decides allow / deny / ask
```

So `permission.ask` is reached **only** for the ask-routed subset; the static
table is the first gate. `tool.execute.before` is orthogonal — it observes the
full tool-call stream regardless of how the table resolved it.

### Reconciliation rule (Phase 2/3 must preserve)

When the classifier owns `permission.ask`, the layered precedence is:

1. **Static deny always wins.** A table / shell-guard deny blocks regardless of
   the classifier — and it does so *before* `permission.ask` even fires.
2. **Static failure denies.** If a static rule errors, the call is denied, not
   allowed.
3. **Classifier allow is valid only when no lower layer denied.** Since the
   table runs first, a classifier `allow` can only ever lift an `ask` (the
   default) — it cannot override a deny, because deny never reaches the hook.
4. **Classifier failure / timeout / malformed verdict blocks.** Fail-closed:
   the gate degrades to deny or escalate-to-ask, **never** silently allows.

### Which hook each phase uses

| Phase | `tool.execute.before` | `permission.ask` | Behavior |
|-------|-----------------------|------------------|----------|
| **1 (this pack)** | audit (no block) | audit (no status mutation) | Observability only — logs both surfaces, changes zero behavior. Default mode `audit`. |
| **2 (this pack)** | unchanged (audit, permanent) | verdict parser + **fail-closed stub evaluator** | `enforce` mode: parses a verdict via a DETERMINISTIC STUB and sets `output.status`; fail-closed on parse error / evaluator error / thrown exception. Not a real model. |
| **3b (this pack)** | unchanged (audit) | **live classifier model** (OpenAI-compatible HTTP) | `live` mode: real security-monitor LLM replaces the stub in `permission.ask`, fed by a serialized transcript. Same fail-closed matrix as `enforce`. |
| **4** | promotion review | promotion review | Decide whether to promote into core templates / `README.agent.md`. |

### Open caveat (to verify at Phase 2)

Does `permission.ask` fire for **every** tool call, or only for ask-routed ones
(i.e. does the classifier see calls that opencode would auto-allow)? The
composition model above implies **only ask-routed calls** reach it, but this has
not been confirmed against a live server. The Phase 1 **dual-hook audit is
specifically designed to answer this**: by logging both surfaces we can diff
which tools appeared in `tool.execute.before` but never in `permission.ask`
(the fast-pathed set). Phase 2's verdict parser must not assume it sees
auto-allowed calls until this is confirmed.

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
reads a small operator-owned JSON config file from disk on **every hook
invocation** — editing the file takes effect on the **next tool call**, with no
server restart and no corpus re-render.

This works because the plugin does runtime file I/O inside each hook (mirroring
how `shell-guard.js` imports `node:fs` + `node:path`). The OpenCode plugin SDK
has **no native hot-reload config API** — the `config` hook and `PluginOptions`
are load-time (set at server start) and env vars are frozen at process start —
so a per-call disk read gated by an **mtime cache** is the reload-free
mechanism: in steady state an unchanged file costs only a single `statSync` per
call, and a changed file is re-read + re-parsed only when its mtime changes.

### Config file

- **Path**: `.opencode/repo-configs/auto-gate-config.json` (resolved relative
  to the repo root, the same way `shell-guard-core.js` derives `repoRoot()` —
  from the plugin file's location, never a hardcoded absolute path).
- **Ownership**: operator-owned. The overlay does **not** render or seed this
  file. Leaving it absent is the documented fail-safe default — the plugin
  works out of the box with built-in defaults.

### Config fields

| Field | Type | Default | Meaning |
|-------|------|---------|---------|
| `enabled` | boolean | `true` | Master live toggle. `false` live-disables the plugin: both hooks no-op immediately on the next tool call (no audit, no behavior change, no restart). `true` is the normal on state. |
| `mode` | `"audit"` \| `"enforce"` \| `"live"` | `"audit"` | Behavior selector. `audit` = Phase 1 observability only (default, zero behavior change). `enforce` = Phase 2 decision path on `permission.ask` (verdict parser + STUB evaluator; fail-closed to deny). `live` = Phase 3b decision path using a REAL OpenAI-compatible model call (fail-closed to deny on any error/timeout/misconfiguration). `tool.execute.before` stays an observer in all modes. |
| `stubVerdict` | `"allow"` \| `"block"` \| `"fail"` | `"block"` | Drives the Phase 2 STUB evaluator in `enforce` mode. `"allow"` → allow verdict; `"block"` → block verdict; `"fail"` → unparseable output (exercises fail-closed). **Test/placeholder only** — not a real classifier; ignored unless `mode: "enforce"`. |
| `modelEndpoint` | string | `""` | Required for `live` mode. The FULL OpenAI-compatible chat-completions URL (e.g. `https://api.provider.example/v1/chat/completions`). Empty/missing when `mode: "live"` → fail-closed deny with audit line `live mode misconfigured: no modelEndpoint`. Ignored in other modes. |
| `model` | string | `""` | Required for `live` mode. The model identifier sent in the request body (e.g. `gpt-4o-mini`, a provider alias, etc.). Empty/missing when `mode: "live"` → fail-closed deny. Ignored in other modes. |
| `apiKeyEnv` | string | `"AUTO_GATE_API_KEY"` | The **name** of the environment variable holding the API key for `live` mode. The key VALUE is read from `process.env[apiKeyEnv]` at call time — it is **never** stored in this config file (which may be committed). Missing/unset env var at call time → fail-closed deny. Ignored in other modes. |
| `timeoutMs` | number | `8000` | Hard timeout for the `live` model HTTP call, via `AbortController`. On timeout the call fails-closed to deny. Ignored in other modes. |
| `promptFile` | string | `""` | Optional override path for the `live` classifier system prompt. If set and readable, its contents replace the binary-served default prompt. If unset/missing/unreadable, the plugin loads the prompt from `vh-agent-harness sys-prompt auto-gate-classifier` (the embedded binary default, overridable via an overlay rendering `.opencode/sys-prompts/auto-gate-classifier.md`). Ignored in other modes. |

Unknown fields are ignored. A field present but of the wrong type or with an
invalid value falls back to that field's default (partial configs are merged
over the defaults field-by-field, so `{"enabled": false}` is valid and leaves
`mode` at its default).

> **Security note on `apiKeyEnv`:** the config file may be committed to version
> control, so it carries only the env-var **name** (default
> `AUTO_GATE_API_KEY`). The actual secret is supplied at runtime via that env
> var and is read fresh on every `live` call. Never paste the key value into
> the config file.

### Example

Minimal (audit, the default):

```json
{
  "enabled": true,
  "mode": "audit"
}
```

To live-disable the plugin (kill switch — takes effect on the next tool call,
no restart):

```json
{
  "enabled": false
}
```

`enforce` mode (Phase 2 stub decision path):

```json
{
  "enabled": true,
  "mode": "enforce",
  "stubVerdict": "block"
}
```

`live` mode (Phase 3b real model call):

```json
{
  "enabled": true,
  "mode": "live",
  "modelEndpoint": "https://api.provider.example/v1/chat/completions",
  "model": "your-model-id",
  "apiKeyEnv": "AUTO_GATE_API_KEY",
  "timeoutMs": 8000
}
```

(with the key exported in the environment: `export AUTO_GATE_API_KEY=sk-...`)

### Fail-safe behavior

If the config file is **missing, unreadable, or invalid JSON**, the plugin
falls back to the hardcoded defaults (`{enabled: true, mode: "audit"}`) and
emits **one** `console.error` audit line noting the fallback — so the operator
learns their config isn't loading without the log being spammed every call.
The plugin **never** throws on a config error; it keeps working with defaults.
The fallback warning is de-duplicated per failure **state**: a transition
(missing → present → unreadable → invalid) re-warns once, but a persistent
failure logs only on the first occurrence.

### Reserved for later phases (not yet implemented)

These fields are **planned** but not read by the plugin today. They are listed
here so the operator knows the forward shape; do not depend on them yet —
including them in the config file is harmless (they are ignored):

- `thresholds` (object) — verdict confidence / length thresholds, for a later
  phase.
- `fastPathAllowlist` (array) — tool/pattern allowlist to skip the classifier,
  for a later phase.

## Enforce mode (Phase 2)

`mode: "enforce"` switches `permission.ask` from observability into a
deterministic, **fail-closed** decision path. The `audit` default is unchanged
(see below).

### What it does

When an ask-routed call reaches `permission.ask` and `mode === "enforce"`:

1. the request is audit-logged (tool type + pattern + `mode=enforce`),
2. `decidePermission(config)` runs: the stub evaluator produces raw verdict
   text, `parseVerdict` greps the first `<block>yes|no` token, and the decision
   matrix sets `output.status`,
3. `allow` verdict → `output.status = "allow"` (grants, skips the prompt),
4. `block` verdict → `output.status = "deny"` (the reason is audit-logged),
5. unparseable verdict / evaluator error / thrown exception → **fail-closed**
   `output.status = "deny"` with a `fail-closed: ...` audit line.

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

Every indeterminate path denies — the gate degrades to `deny`, never silently
allows: parse failure (no `<block>` token), an invalid disposition
(`<block>maybe</block>`), an evaluator that throws, or any exception in the
decision path all yield `output.status = "deny"`.

### The hard floor is never overridden

`permission.ask` fires **only** for calls opencode's permission table routes to
`ask`. Table-`allow` fast-paths past the hook; table-`deny` / shell-guard blocks
**before** the hook. Therefore the classifier decision can only ever lift an
`ask` to `allow`/`deny` — it can **never** override a static deny, because a
statically-denied call never reaches the hook. The classifier only ever decides
the ask-routed subset; the static permission table (plus shell-guard) remains
the hard floor.

### `audit` mode is unchanged

Switching back to `mode: "audit"` (the default) restores the exact Phase 1
behavior: both hooks log only, `permission.ask` never sets `output.status`, and
the interactive ask still fires. The `enforce` and `live` branches are separate
code paths that do not touch the audit branch.

## Live mode (Phase 3b)

`mode: "live"` switches `permission.ask` from the stub decision path to a REAL
classifier model call. It reuses the **same fail-closed decision matrix** as
`enforce`; only the evaluator changes (stub → live HTTP adapter). The `audit`
default is unchanged (see below).

### What it does

When an ask-routed call reaches `permission.ask` and `mode === "live"`:

1. the request is audit-logged (tool type + pattern + `mode=live`),
2. live config is validated — a missing `modelEndpoint` or `model` fails-closed
   to deny with a clear audit line (`live mode misconfigured: no modelEndpoint`
   / `no model`),
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
   `max_tokens: 64`, `stream: false`,
6. the returned verdict text is parsed by the **same** `parseVerdict` → decision
   matrix as `enforce`,
7. `output.status` is set from the decision; the decision, reason, and model
   latency are audit-logged.

### Fail-closed to deny on any uncertainty

Every indeterminate path denies — the gate degrades to `deny`, never silently
allows: transport error, timeout (`timeoutMs`), non-2xx response, malformed
JSON, missing `choices[0].message.content`, unparseable verdict (no `<block>`
token), missing/unset API key, or a thrown exception in the adapter all yield
`output.status = "deny"`. Only an explicit `<block>no</block>` (allow) verdict
yields `allow`.

### The API key comes from the environment, never the config file

`live` mode reads the key from `process.env[apiKeyEnv]` (default
`AUTO_GATE_API_KEY`) **at call time**. The config file carries only the
env-var **name** — because the config file may be committed, it must never hold
the secret value. Export the key in the server environment:

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
behavior. The `live` branch is a separate code path that does not touch the
audit branch.

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

No `opencode.jsonc` registration is needed (this pack's
`opencode-append.jsonc` is intentionally empty for that reason).

## Naming

All identifiers are **generic** — `auto-tool-gate`, `auto-gate-audit`,
`auto-classifier-pilot`. The upstream mechanism is referred to only as "the
reference agent system" / "a security-monitor classifier". No product names
appear anywhere in this pack.

## Design source

- `researches/sources/2026-07-10-auto-mode-classifier-source-packet.md` — the
  design packet for the reference agent system's
  auto/classifier permission mode (the security-monitor LLM tool-call gate).
  The porting-notes section (§10) frames the later phases.
