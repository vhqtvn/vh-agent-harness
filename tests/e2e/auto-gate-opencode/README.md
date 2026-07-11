# auto-gate-opencode — Real-Runtime E2E

This suite proves the auto-classifier plugin's enforcement path works against a
**real OpenCode runtime** — the one integration the other two suites
(`tests/e2e/auto-gate-classifier/` with a synthetic driver, and
`tests/integration/auto-gate-live-http/` with module-import) do not exercise.

It exercises **both** execution modes and **both** evaluation modes:
- **`opencode run`** (one-shot CLI) — the race-proof two-case matrix.
- **`opencode serve`** (long-lived HTTP listener) — the serve-mode reply proof.
- **ENFORCE mode** (stub evaluator, sync) — Cases A/B.
- **LIVE mode** (real classifier HTTP egress) — Cases C/D.

## What it proves

1. **Plugin loads as a real external plugin** via OpenCode's
   `{plugin,plugins}/*.{ts,js}` auto-discovery.
2. **Plugin receives a real `permission.asked` bus event** when the agent
   tries to read a file — not a synthetic event from a fake client.
3. **Plugin evaluates the request** (ENFORCE mode: stubEvaluate → parseVerdict
   → decision matrix; LIVE mode: transcript fetch → classifier HTTP → verdict
   parse) and **replies through the real SDK**
   (`postSessionIdPermissionsPermissionId`).
4. **Plugin's reply resolves the permission** so the tool proceeds (allow) or
   is blocked (reject) — proven under BOTH `opencode run` and `opencode serve`,
   against CURRENT upstream opencode with **no source patches**.
5. **The full live chain works end-to-end** (LIVE cases C/D): the plugin fetches
   the transcript via `client.session.messages`, serializes it, POSTs to the
   classifier endpoint, parses the verdict, and replies — all against the real
   runtime. The classifier HTTP egress is **proven** via the mock's call counter
   (`/count/classifier > 0`), so a pass cannot come from the stub path.

## Acquisition: latest upstream, no patches

The Docker image acquires the **LATEST HEAD** of the canonical upstream
(`https://github.com/sst/opencode.git`) as a shallow clone at build time — no
fork, no pinned commit, no host clone dependency. The image self-installs
opencode; the build context never references an out-of-tree checkout.

**No source patches are applied.** An earlier version of this suite patched two
bugs in a stale fork of opencode (an `InstanceMiddleware` outlier mount and an
in-process plugin fetch that broke serve-mode permission replies). Both bugs
were **resolved upstream**: the routing layer was rewritten from hand-rolled
Hono mounts to Effect HttpApi (eliminating the outlier mount), and the plugin
SDK client now threads `directory` via the `x-opencode-directory` header and
routes replies over HTTP when a serve listener is active
(`plugin/index.ts`: `serverUrl?.toString()` baseUrl + conditional in-process
fetch override). The plugin's permission reply now resolves correctly under both
`run` and `serve` out of the box.

This suite therefore proves the plugin works against **what operators actually
run** — current upstream, unmodified.

### Intentional non-reproducibility (the tradeoff)

Building against latest HEAD is a deliberate choice: it means this suite
**catches upstream drift**. A change to the event payload, the reply route, the
provider config shape, or the plugin auto-discovery contract surfaces here
first. The cost is that a build is not byte-for-byte reproducible across time
(today's HEAD differs from next week's). That is acceptable for a drift-detecting
integration suite; reproducibility is the job of the pinned unit/integration
suites.

## Run mode (race-proof two-case matrix)

This suite uses `opencode run` (one-shot CLI) for its race-proof two-case
matrix. `opencode run` uses **only** `Server.Default()` (the singleton app) as
its SDK fetch function — every code path (session, prompt, permission ask,
reply) goes through the same app with the same middleware chain and the same
ScopedCache.

`opencode run` always auto-replies to `permission.asked`:
- **WITH** `--dangerously-skip-permissions` → replies `once` (allow)
- **WITHOUT** → replies `reject`

It does NOT short-circuit before the bus publish, so the plugin also sees the
event. **First reply wins.** The plugin has a structural head-start: it
receives the event via a direct bus-stream callback (synchronous dispatch),
while run's auto-reply goes through SSE transport (more hops).

Both ENFORCE and LIVE modes are exercised under run:

- **ENFORCE mode** (Cases A/B) uses `stubEvaluate` — a pure synchronous
  evaluator with no HTTP round-trip. The plugin's path is: `readConfig` (sync
  file read) → `decidePermission` (sync) → `reply` (in-process SDK fetch).
- **LIVE mode** (Cases C/D) exercises the full live chain: transcript fetch via
  `client.session.messages` → `serializeTranscript` → classifier HTTP egress
  (`decideLive` POSTs to `modelEndpoint`) → verdict parse → reply. This is
  slower (HTTP round-trip), so the run-mode race is tighter; serve-live is the
  deterministic proof. The live cases ALSO assert the classifier HTTP was
  actually called (mock `/count/classifier > 0`), proving the live egress
  happened rather than the stub path.

### Two-case matrix × two evaluation modes (airtight — no false pass possible)

| Case | Mode | `--dangerously` | Verdict source | Run default | PASS = tool outcome |
|------|------|:---:|---|---|---|
| **A (ALLOW proof)** | enforce | absent | stub `"allow"` | reject | **read PROCEEDS** |
| **B (BLOCK proof)** | enforce | present | stub `"block"` | once/allow | **read BLOCKED** |
| **C (LIVE ALLOW proof)** | live | absent | classifier `<block>no` | reject | **read PROCEEDS** |
| **D (LIVE BLOCK proof)** | live | present | classifier `<block>yes` | once/allow | **read BLOCKED** |

A pass PROVES the plugin won the race: the only way the outcome flips from the
run-default is if the plugin's reply landed first. If the plugin loses, the
outcome matches the run-default and the case FAILS loudly. For LIVE cases, the
classifier-call-count assertion adds a second guard: a pass requires the live
HTTP egress to have actually happened.

> **Run-live race note:** LIVE mode adds an HTTP round-trip that narrows the
> plugin's head-start over run's auto-reply. In practice the live path
> (transcript fetch + classifier POST) is consistently a few milliseconds
> slower than run's in-process auto-reply, so the plugin typically LOSES the
> reply race under run. This is classified as **RACE_LOSS** — not a failure:
> the stderr proves the live chain ran correctly (event seen + classifier
> called + correct decision parsed + `permission reply failed: Permission
> request not found`). The **serve-live cases** (no auto-replier, sole
> replier) are the **deterministic proof** that the full live chain resolves
> end-to-end. RACE_LOSS does not fail the suite; only a genuine live-chain
> failure (event not seen, classifier not called, wrong decision) does.

## Serve mode (plugin reply resolves under the HTTP listener)

`opencode serve` runs the headless HTTP listener with no ambient instance
context (`effectCmd` uses `instance: false`), so the plugin's permission reply
must resolve through the serve routing path. Current upstream handles this
correctly with **no patches**:

- The Effect HttpApi routing rewrite eliminated the inconsistent
  `InstanceMiddleware` outlier mount that previously left the reply handler
  without instance context.
- The plugin SDK client threads `directory` via `x-opencode-directory` and, when
  a serve listener is active (`Server.url` set), routes replies over HTTP to the
  live listener instead of an in-process fetch — so the reply resolves in the
  same request context as the ask.

Serve mode has **no** `--dangerously-skip-permissions` auto-reply, so the plugin
is the **sole replier** — no race. This makes serve-mode assertions simpler and
more direct, and makes serve-LIVE the **deterministic proof** of the full live
chain (no race to lose):

| Case | Mode | Verdict source | PASS = tool outcome |
|------|------|---|---|
| **serve-A (ALLOW proof)** | enforce | stub `"allow"` | **read PROCEEDS** (plugin's allow reply resolved) |
| **serve-B (BLOCK proof)** | enforce | stub `"block"` | **read BLOCKED** (plugin's reject reply resolved) |
| **serve-C (LIVE ALLOW proof)** | live | classifier `<block>no` | **read PROCEEDS** + classifier HTTP called |
| **serve-D (LIVE BLOCK proof)** | live | classifier `<block>yes` | **read BLOCKED** + classifier HTTP called |

Both polarities are tested in both evaluation modes, proving the plugin's reply
resolves end-to-end under serve against current upstream — including the full
live chain (transcript fetch over HTTP + classifier egress + verdict parse).

## Architecture (single container)

One Dockerfile, one container:

1. **Stage 1** (`oven/bun:1-debian`): shallow-clones the LATEST upstream
   opencode (`sst/opencode`, HEAD only), runs `bun install` (including native
   modules). No fork, no pinned commit.
2. **Stage 2** (`oven/bun:1-debian`): copies the opencode tree, installs
   ripgrep + ca-certificates, bundles the 4 plugin modules into a single file
   via `bun build`, copies test files, and sets the entrypoint. **No source
   patches are applied.**

The driver (`run-e2e.mjs`) orchestrates everything on localhost inside the
container:

1. Writes `opencode.json` (mock provider + `permission.read:"ask"`), the plugin
   config files (`auto-gate-config.json` per case, `auto-gate-llm.json` once),
   and the target fixture.
2. Starts the dual-port mock LLM server (agent `:8080`, classifier `:8081`).
3. **Run Case A** (enforce allow): sets `stubVerdict:"allow"`, resets the agent
   counter, runs `opencode run --format json "Read /workspace/target.txt"` (no
   `--dangerously-skip-permissions`), captures stdout + stderr.
4. **Run Case B** (enforce block): sets `stubVerdict:"block"`, resets, runs
   `opencode run --dangerously-skip-permissions --format json "..."`.
5. **Run Case C** (live allow): sets `mode:"live"` + classifier verdict
   `<block>no</block>`, resets the classifier counter, runs `opencode run`
   (no `--dangerously-skip-permissions`), then asserts classifier count > 0.
6. **Run Case D** (live block): sets `mode:"live"` + classifier verdict
   `<block>yes</block>...<reason>`, resets, runs `opencode run
   --dangerously-skip-permissions`, asserts classifier count > 0.
7. **Starts `opencode serve`** on `:3000` (with `OPENCODE_SERVER_PASSWORD` set),
   polls `GET /global/health` until healthy.
8. **Serve Cases A–D**: same evaluation-mode matrix as run, driven over HTTP
   (create session → `prompt_async` → poll `GET /session/{id}/message`). Serve
   has no auto-replier, so the plugin is the sole replier (deterministic).
9. Tears down serve, then the mock LLM server.
10. Asserts per case: (a) plugin audit lines in stderr show it saw each
    `permission.asked` event (mode=enforce for A/B, mode=live for C/D); (b)
    allow cases have file content, block cases have no content + a rejection
    indicator; (c) live cases have classifier call count > 0.

### Mock LLM server

The agent endpoint (`:8080`) is **stateful**: it differentiates title-generation
calls (no `tools` in request body → short text) from agent calls (has `tools` →
1st call emits a tool_call for the `read` tool, 2nd call emits short text so the
session reaches idle). Both streaming SSE and non-streaming JSON are supported.

The classifier endpoint (`:8081`) reads a control file (`/tmp/classifier-verdict`)
and returns a verdict. It supports a **passthrough mode**: if the control file
contains a string starting with `<block>`, it is returned verbatim as the
classifier content, so the live cases can inject exact verdict text (including
test-specific reasons). It is **not used in ENFORCE mode** (the stub evaluator
is pure sync), but is exercised by **LIVE mode** (Cases C/D): the plugin POSTs
the serialized transcript, the mock returns the verdict, and the driver asserts
the call happened via `GET /count/classifier` (counter > 0). The counter is
reset between cases via `GET /reset-classifier-count`.

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
| `Dockerfile` | Multi-stage build: latest upstream opencode (shallow clone) + test runtime, no patches |
| `mock-llm-server.js` | Dual-port mock LLM (agent + classifier endpoints) |
| `run-e2e.mjs` | Test driver: orchestrate mock + 4 run cases (A–D) + 4 serve cases (A–D), assert |
| `target.txt` | Fixture file the agent tries to read |
| `classifier-prompt.md` | Minimal classifier system prompt (promptFile override) |
| `README.md` | This file |
