# auto-gate-opencode — Real-Runtime E2E

This suite proves the auto-classifier plugin's enforcement path works against a
**real OpenCode runtime** — the one integration the other two suites
(`tests/e2e/auto-gate-classifier/` with a synthetic driver, and
`tests/integration/auto-gate-live-http/` with module-import) do not exercise.

## What it proves

1. **Plugin loads as a real external plugin** via OpenCode's
   `{plugin,plugins}/*.{ts,js}` auto-discovery.
2. **Plugin receives a real `permission.asked` bus event** when the agent
   tries to read a file — not a synthetic event from a fake client.
3. **Plugin evaluates the request** (ENFORCE mode: stubEvaluate → parseVerdict
   → decision matrix) and **replies through the real SDK**
   (`postSessionIdPermissionsPermissionId`).
4. **Plugin's reply resolves the permission** so the tool proceeds (allow) or
   is blocked (reject).

## Run mode + ENFORCE mode (race-proof two-case matrix)

This suite uses `opencode run` (one-shot CLI) rather than `opencode serve`.
`opencode run` uses **only** `Server.Default()` (the singleton Hono app) as its
SDK fetch function — every code path (session, prompt, permission ask, reply)
goes through the same app with the same middleware chain and the same
ScopedCache. This guarantees the permission reply resolves the Deferred.

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

## Architecture (single container)

One Dockerfile, one container:

1. **Stage 1** (`oven/bun:1-debian`): clones the OpenCode repo at a pinned
   commit, runs `bun install` (including native modules).
2. **Stage 2** (`oven/bun:1-debian`): copies the OpenCode tree, applies the
   InstanceMiddleware patch (see below), installs ripgrep + ca-certificates,
   bundles the 4 plugin modules into a single file via `bun build`, copies test
   files, and sets the entrypoint.

The driver (`run-e2e.mjs`) orchestrates everything on localhost inside the
container:

1. Writes `opencode.json` (mock provider + `permission.read:"ask"`), the plugin
   config files, and the target fixture.
2. Starts the dual-port mock LLM server (agent `:8080`, classifier `:8081`).
3. **Case A**: sets `stubVerdict:"allow"`, resets the agent counter, runs
   `opencode run --format json "Read /workspace/target.txt"` (no
   `--dangerously-skip-permissions`), captures stdout + stderr.
4. **Case B**: sets `stubVerdict:"block"`, resets, runs
   `opencode run --dangerously-skip-permissions --format json "..."`,
   captures stdout + stderr.
5. Asserts: (a) plugin audit lines in stderr show it saw both
   `permission.asked` events (`[auto-gate] permission.asked type=read
   mode=enforce`); (b) Case A stdout has file content; (c) Case B stdout has
   no file content + a rejection indicator.

### Mock LLM server

The agent endpoint (`:8080`) is **stateful**: it differentiates title-generation
calls (no `tools` in request body → short text) from agent calls (has `tools` →
1st call emits a tool_call for the `read` tool, 2nd call emits short text so the
session reaches idle). Both streaming SSE and non-streaming JSON are supported.

The classifier endpoint (`:8081`) reads a control file and returns a verdict.
It is **not used in ENFORCE mode** (the stub evaluator is pure sync), but is
included for completeness — if the suite is later switched to LIVE mode the
infrastructure is already in place.

### InstanceMiddleware patch

The Dockerfile applies a one-line `sed` patch to `server.ts` that adds
`.use(InstanceMiddleware())` to the main Hono app chain in `createHono()`.
Without this patch, `InstanceMiddleware` is only applied to
`workspaceLegacyApp` in Branch 2 of `createHono()` (the non-workspace-ID path).
Session routes and permission reply routes run without instance context (no
AsyncLocalStorage), so the permission reply's `pending.get(requestID)` looks up
a different (empty) ScopedCache entry → the Deferred never resolves → the tool
hangs.

The patch ensures `InstanceMiddleware` runs for ALL routes regardless of which
branch is taken. `InstanceMiddleware` is already imported in `server.ts`.

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
| `run-e2e.mjs` | Test driver: orchestrate mock + run cases, assert |
| `target.txt` | Fixture file the agent tries to read |
| `classifier-prompt.md` | Minimal classifier system prompt (promptFile override) |
| `README.md` | This file |
