# auto-gate-opencode — Real-Runtime E2E

This suite proves the auto-classifier plugin's enforcement path works against a
**real OpenCode runtime** — the one integration the other two suites
(`tests/e2e/auto-gate-classifier/` with a synthetic driver, and
`tests/integration/auto-gate-live-http/` with module-import) do not exercise.

It exercises **both** execution modes:
- **`opencode run`** (one-shot CLI) — the race-proof two-case matrix.
- **`opencode serve`** (long-lived HTTP listener) — the serve-mode fix proof.

## What it proves

1. **Plugin loads as a real external plugin** via OpenCode's
   `{plugin,plugins}/*.{ts,js}` auto-discovery.
2. **Plugin receives a real `permission.asked` bus event** when the agent
   tries to read a file — not a synthetic event from a fake client.
3. **Plugin evaluates the request** (ENFORCE mode: stubEvaluate → parseVerdict
   → decision matrix) and **replies through the real SDK**
   (`postSessionIdPermissionsPermissionId`).
4. **Plugin's reply resolves the permission** so the tool proceeds (allow) or
   is blocked (reject) — proven under BOTH `opencode run` and `opencode serve`.

## Run mode + ENFORCE mode (race-proof two-case matrix)

This suite uses `opencode run` (one-shot CLI) for its race-proof two-case
matrix. `opencode run` uses **only** `Server.Default()` (the singleton Hono app)
as its SDK fetch function — every code path (session, prompt, permission ask,
reply) goes through the same app with the same middleware chain and the same
ScopedCache.

`opencode run` always auto-replies to `permission.asked`:
- **WITH** `--dangerously-skip-permissions` → replies `once` (allow)
- **WITHOUT** → replies `reject`

It does NOT short-circuit before the bus publish, so the plugin also sees the
event. **First reply wins.** The plugin has a structural head-start: it
receives the event via a direct bus-stream callback (synchronous dispatch),
while run's auto-reply goes through SSE transport (more hops).

To win the race reliably, the plugin runs in **ENFORCE mode** (not LIVE mode).
ENFORCE mode uses `stubEvaluate` — a pure synchronous evaluator with no HTTP
round-trip. The plugin's path is: `readConfig` (sync file read) →
`decidePermission` (sync) → `reply` (in-process SDK fetch). This is
substantially faster than LIVE mode (which adds a classifier HTTP call that
lost the race in earlier testing).

### Two-case matrix (airtight — no false pass possible)

| Case | `--dangerously-skip` | `stubVerdict` | Run default | PASS = tool outcome |
|------|:---:|---|---|---|
| **A (ALLOW proof)** | absent | `"allow"` | reject | **read PROCEEDS** (run alone would reject) |
| **B (BLOCK proof)** | present | `"block"` | once/allow | **read BLOCKED** (run alone would allow) |

A pass PROVES the plugin won the race: the only way the outcome flips from the
run-default is if the plugin's reply landed first. If the plugin loses, the
outcome matches the run-default and the case FAILS loudly.

## Serve mode (two fixes for plugin reply resolution)

`opencode serve` runs without ambient instance context (`effectCmd` uses
`instance: false`), so the plugin's permission reply needs explicit instance
context to find the right pending-map entry. Two issues must be fixed:

### Fix 1: InstanceMiddleware on line-139 InstanceRoutes mount

In `createHono()` Branch 2 (no `OPENCODE_WORKSPACE_ID` — the default
multi-workspace path), `InstanceRoutes` is mounted at **line 139** of
`packages/opencode/src/server/server.ts` **WITHOUT** `InstanceMiddleware`:

```typescript
// line 139 (BROKEN — no InstanceMiddleware):
.route("/", InstanceRoutes(runtime.upgradeWebSocket, opts))
```

This is the inconsistent outlier — Branch 1 (line 123, `OPENCODE_WORKSPACE_ID`
set) and `workspaceLegacyApp` (line 130) BOTH wrap their `InstanceRoutes`
mounts with `InstanceMiddleware`.

Without `InstanceMiddleware`, the permission reply route handler
(`POST /session/{id}/permissions/{permissionID}`) has no instance context:
`InstanceMiddleware` reads the `x-opencode-directory` header and provides
`WithInstance` (the AsyncLocalStorage the permission pending `ScopedCache` is
keyed by). Without it, the handler resolves the wrong directory → empty pending
map → `if (!existing) return` → **silent no-op**.

```diff
-      .route("/", InstanceRoutes(runtime.upgradeWebSocket, opts))
+      .route("/", new Hono().use(InstanceMiddleware()).route("/", InstanceRoutes(runtime.upgradeWebSocket, opts)))
```

This mirrors the pattern already used at line 123 (Branch 1). `Hono` and
`InstanceMiddleware` are already imported in `server.ts`.

Under `opencode run`, `effectCmd` (`instance: true`) establishes ambient ALS
instance context for the whole handler, so the reply resolves via ambient
context even without the middleware. Under `opencode serve`, `effectCmd` uses
`instance: false` (no ambient ALS) → the middleware is **required**.

### Fix 2: Route plugin reply through HTTP when serve listener is active

Fix 1 is **necessary but not sufficient**. The plugin creates its SDK client
with an **in-process** fetch override:

```typescript
// plugin/index.ts — original (BROKEN for serve):
fetch: async (...args) => Server.Default().app.fetch(...args)
```

This processes the permission reply via `Server.Default().app.fetch()` — a
direct in-process Hono call that runs in the **caller's** async context (the
bus stream-consumer fiber that dispatched the `permission.asked` event to the
plugin). Despite InstanceMiddleware correctly setting `x-opencode-directory`
ALS and `attach()` correctly providing `InstanceRef`, the `ScopedCache` that
backs the permission pending map resolves to a **different `State` object** for
the reply vs the ask (the ask ran through the serve listener's HTTP path → a
fresh fiber; the reply ran through the in-process path → the bus consumer's
fiber lineage). Result: `pending.get(requestID)` returns `undefined` →
`if (!existing) return` → **silent no-op**.

The Dockerfile patches `plugin/index.ts` to use `globalThis.fetch` (HTTP) when
`Server.url` is set (serve mode), routing the reply through Bun's HTTP server
→ a fresh async context → the same fiber/scope lineage as the ask. In run mode
(`Server.url` unset), it falls back to the original in-process fetch (which
works via ambient ALS from `effectCmd` `instance: true`).

```diff
-        baseUrl: "http://localhost:4096",
+        baseUrl: Server.url ? Server.url.origin : "http://localhost:4096",
         ...
-        fetch: async (...args) => Server.Default().app.fetch(...args),
+        fetch: Server.url ? globalThis.fetch : (async (...args) => Server.Default().app.fetch(...args)),
```

### Reconciliation with the prior run-mode patch

The previous Dockerfile applied a **global** `InstanceMiddleware` patch
(appending `.use(InstanceMiddleware())` after `CorsMiddleware` on the main app
chain). Investigation found that Hono v4 `.use()` on a parent app does NOT
propagate to `.route("/", subApp)` sub-apps (sub-apps are isolated routers), so
the global patch was a **no-op** for the InstanceRoutes mount. The surgical
line-139 wrap is what actually puts `InstanceMiddleware` inside the sub-app's
chain.

Under `opencode run`, the reply resolves via ambient ALS (`effectCmd`
`instance: true`) regardless of any middleware patch — the run-mode cases pass
with or without the global patch. The line-139 fix is necessary for serve but
not sufficient; the HTTP-fetch fix (Fix 2) is what ultimately makes serve pass.
The old global patch was redundant and has been removed.

### Serve-mode cases

Serve mode has **no** `--dangerously-skip-permissions` auto-reply, so the
plugin is the **sole replier** — no race. This makes serve-mode assertions
simpler and more direct:

| Case | `stubVerdict` | PASS = tool outcome |
|------|:---:|---|
| **serve-A (ALLOW proof)** | `"allow"` | **read PROCEEDS** (plugin's allow reply resolved) |
| **serve-B (BLOCK proof)** | `"block"` | **read BLOCKED** (plugin's reject reply resolved) |

If either fix were absent, the permission `Deferred` would never resolve and
both cases would time out (90 s) with neither content nor rejection in the
messages — a loud FAIL.

## Architecture (single container)

One Dockerfile, one container:

1. **Stage 1** (`oven/bun:1-debian`): clones the OpenCode repo at a pinned
   commit, runs `bun install` (including native modules).
2. **Stage 2** (`oven/bun:1-debian`): copies the OpenCode tree, applies two
   serve fixes (InstanceMiddleware line-139 wrap + plugin HTTP-fetch patch —
   see below), installs ripgrep + ca-certificates, bundles the 4 plugin
   modules into a single file via `bun build`, copies test files, and sets
   the entrypoint.

The driver (`run-e2e.mjs`) orchestrates everything on localhost inside the
container:

1. Writes `opencode.json` (mock provider + `permission.read:"ask"`), the plugin
   config files, and the target fixture.
2. Starts the dual-port mock LLM server (agent `:8080`, classifier `:8081`).
3. **Run Case A**: sets `stubVerdict:"allow"`, resets the agent counter, runs
   `opencode run --format json "Read /workspace/target.txt"` (no
   `--dangerously-skip-permissions`), captures stdout + stderr.
4. **Run Case B**: sets `stubVerdict:"block"`, resets, runs
   `opencode run --dangerously-skip-permissions --format json "..."`,
   captures stdout + stderr.
5. **Starts `opencode serve`** on `:3000` (with `OPENCODE_SERVER_PASSWORD` set),
   polls `GET /global/health` until healthy.
6. **Serve Case A**: creates a session over HTTP, sends `prompt_async` with the
   same prompt, polls `GET /session/{id}/message` for content (allow → read
   proceeds).
7. **Serve Case B**: same flow with `stubVerdict:"block"`, polls for rejection
   (plugin's reject reply resolved).
8. Tears down serve, then the mock LLM server.
9. Asserts: (a) plugin audit lines in stderr show it saw each
   `permission.asked` event; (b) run-A + serve-A have file content; (c) run-B +
   serve-B have no file content + a rejection indicator.

### Mock LLM server

The agent endpoint (`:8080`) is **stateful**: it differentiates title-generation
calls (no `tools` in request body → short text) from agent calls (has `tools` →
1st call emits a tool_call for the `read` tool, 2nd call emits short text so the
session reaches idle). Both streaming SSE and non-streaming JSON are supported.

The classifier endpoint (`:8081`) reads a control file and returns a verdict.
It is **not used in ENFORCE mode** (the stub evaluator is pure sync), but is
included for completeness — if the suite is later switched to LIVE mode the
infrastructure is already in place.

### Dockerfile patches (two serve fixes)

The Dockerfile applies two `sed` patches to the cloned OpenCode source:

1. **InstanceMiddleware line-139 wrap** (`server/server.ts`): wraps the
   multi-workspace `InstanceRoutes` mount with `InstanceMiddleware`, mirroring
   the pattern already used at line 123 and line 130. See [Fix 1](#fix-1-instancemiddleware-on-line-139-instanceroutes-mount)
   above for the full root-cause analysis.

2. **Plugin HTTP-fetch** (`plugin/index.ts`): when `Server.url` is set (serve
   mode), routes the plugin's SDK client through `globalThis.fetch` to the live
   serve listener instead of the in-process `Server.Default().app.fetch()`.
   This avoids a `ScopedCache` context-isolation issue where the in-process
   fetch resolves the reply in a different fiber lineage than the ask. See
   [Fix 2](#fix-2-route-plugin-reply-through-http-when-serve-listener-is-active)
   above for details.

Neither patch modifies the committed plugin source — both are build-time
patches to the cloned OpenCode source tree.

### Why `permission.read:"ask"` is mandatory

The default `build` agent pre-allows `read: {"*":"allow"}`. The permission
evaluator uses `findLast` (last-match-wins) and short-circuits on allow — so
the `permission.asked` event never fires for a pre-allowed read. Setting
`"permission": {"read":"ask"}` appends a user rule `{read, *, ask}` AFTER the
default, so `findLast` picks `ask` and the event fires. Without this, both
cases silently "pass" with zero plugin involvement (a false pass).

### Why the plugin is bundled

OpenCode's plugin auto-discovery scans `.opencode/{plugin,plugins}/*.{ts,js}`
and loads **each file as a separate plugin**. The plugin's sibling modules
(`auto-gate-live.js`, `auto-gate-verdict.js`, `auto-gate-scrub.js`) don't export
a `server` factory, so loading them individually crashes the plugin loader. The
Dockerfile uses `bun build` to inline all 4 modules into a single self-contained
ESM file — this is a build-time transform, not a change to the committed plugin
source.

## Running

```bash
# Via Makefile
make test-e2e-auto-gate-opencode

# Directly
docker build -t auto-gate-opencode-e2e -f tests/e2e/auto-gate-opencode/Dockerfile .
docker run --rm auto-gate-opencode-e2e
```

## Files

| File | Purpose |
|------|---------|
| `Dockerfile` | Multi-stage build: OpenCode from source + test runtime |
| `mock-llm-server.js` | Dual-port mock LLM (agent + classifier endpoints) |
| `run-e2e.mjs` | Test driver: orchestrate mock + 2 run cases + 2 serve cases, assert |
| `target.txt` | Fixture file the agent tries to read |
| `classifier-prompt.md` | Minimal classifier system prompt (promptFile override) |
| `README.md` | This file |
