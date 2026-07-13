# auto-gate live HTTP integration tests

Integration tests that exercise the **real HTTP path** of the auto-gate live
classifier adapter (`classifyLive` / `decideLive` in `auto-gate-live.js`)
against a Dockerized OpenAI-compatible mock LLM server.

## Architecture: full Docker isolation

Both the mock LLM server **and** the test runner run as containers on a private
Docker bridge network. The host runs only `docker compose` and reads the exit
code. **Zero host port publishing** (`-p` / `ports:`) — the mock's unauthenticated
scenario + control endpoints are reachable only from within the private network,
never from the host or the default bridge.

```
┌─────────────────────────────────────────────────────┐
│  HOST                                                │
│                                                      │
│  docker compose ... run --rm tester  ──► exit code   │
│                                                      │
│  ┌────────────────────────────────────────────────┐ │
│  │  private bridge network: auto-gate-net          │ │
│  │                                                 │ │
│  │  ┌──────────────┐        ┌──────────────┐      │ │
│  │  │  mock-llm    │◄──────│  tester       │      │ │
│  │  │  :8080       │  HTTP  │  node --test  │      │ │
│  │  │  (no -p)     │        │  (no -p)      │      │ │
│  │  └──────────────┘        └──────────────┘      │ │
│  └────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────┘
```

The `tester` container imports the real classifier source and uses the real
`globalThis.fetch` to talk to the mock over the private network. It does NOT
inject a fake `fetchFn` — the whole point is real network against the mock
container.

The system-prompt binary (`vh-agent-harness sys-prompt`) is avoided: the
classifier prompt is COPY'd into the tester image and loaded via
`config.promptFile`, so `resolveSystemPrompt` reads the file directly instead
of shelling out to the Go binary (absent in a `node:alpine` image).

## Prerequisites

- Docker Engine and Docker Compose must be installed and running.

## How to run

```bash
# Via Make target (recommended):
make test-auto-gate-live

# Or directly:
docker compose -f tests/integration/auto-gate-live-http/docker-compose.yml run --rm tester
```

The exit code is the test result (0 = pass, non-zero = fail). The `--rm` flag
removes the tester container after it exits; the mock-llm service stays running
until you bring the stack down:

```bash
docker compose -f tests/integration/auto-gate-live-http/docker-compose.yml down
```

This suite is **not** wired into the default `go test ./...` / `make test`
target because it depends on Docker.

## Scenario matrix

The mock server selects its behavior from the **last URL path segment**. The
tester sets `config.modelEndpoint` to `http://mock-llm:8080/<scenario>` for each
scenario. The request/response body follows the OpenAI chat-completions
contract in every case.

| Scenario              | Mock behavior                  | Adapter outcome                          | HTTP count | Asserts            |
|-----------------------|--------------------------------|------------------------------------------|------------|--------------------|
| `allow`               | 200 + `<block>no</block>`      | `status=allow`, `retries=0`              | 1          | happy path         |
| `block`               | 200 + block verdict + reason   | `status=deny`, `retries=0`, reason set   | 1          | block path         |
| `stall`               | never responds (holds socket)  | `status=deny`, `retries>0`               | **> 1**    | **retry fires**    |
| `recover-after-stall` | 1st stalls, 2nd succeeds       | `status=allow`, `retries=1`              | **2**      | **retry recovers** |
| `error-5xx`           | 503 every time                 | `status=deny`, `retries>0`               | **> 1**    | 5xx retryable      |
| `error-4xx`           | 404 every time                 | `status=deny`, `retries=0`               | **1**      | 4xx NOT retried    |
| `malformed`           | 200 + invalid JSON body        | `status=deny`, `retries=0`               | **1**      | parse NOT retried  |
| `empty`               | 200 + missing content field    | `status=deny`, `retries>0`               | **> 1**    | empty retryable    |

### Which scenarios prove retry vs. no-retry

Retry behavior is cross-checked at **two independent levels**:

1. **The SUT's own `r.retries` field** — `_classifyLiveCore` stamps `.retries`
   onto thrown errors (committed in eaa13a5d), so `decideLive` reports accurate
   retry counts on **all** paths, including the throw / fail-closed paths where
   the loop exhausts its attempts. `r.retries` is now a trustworthy witness.

2. **The mock's per-scenario request counter** (`GET /count/<scenario>`) — an
   independent socket-level witness that counts real HTTP POSTs: `count > 1`
   proves the adapter retried; `count === 1` proves it did not.

Two witnesses agreeing is stronger than either alone. The HTTP count is retained
as defense-in-depth: even if a future SUT regression broke the `r.retries`
stamp on some path, the socket-level count would still reveal the true retry
behavior.

- **Retry proven** (r.retries>0, HTTP count > 1): `stall`, `recover-after-stall`, `error-5xx`, `empty`
- **No-retry proven** (r.retries=0, HTTP count === 1): `error-4xx`, `malformed`
- **No retry needed** (succeeds first try): `allow`, `block`

### Mock control endpoints

The mock server (besides the scenario POST paths) serves these control routes:

| Method + path             | Returns                              | Purpose                            |
|----------------------------|--------------------------------------|------------------------------------|
| `GET /healthz`            | `{ ok: true }`                       | readiness probe (client startup)   |
| `GET /count/<scenario>`   | `{ scenario, count }`                | per-scenario POST request count    |
| `GET /reset-counts`       | `{ ok: true }`                       | clears all counters                |

## Example request/response

Request (POST to `http://mock-llm:8080/allow` — private network only):
```json
{
  "model": "mock-model",
  "messages": [
    { "role": "system", "content": "..." },
    { "role": "user", "content": "..." }
  ],
  "temperature": 1,
  "max_tokens": 64,
  "stream": false
}
```
Headers: `Content-Type: application/json`, `Authorization: Bearer dummy-mock-key-value`

Response (allow scenario):
```json
{
  "id": "mock-chatcmpl-xxxx",
  "object": "chat.completion",
  "model": "mock-model",
  "choices": [
    {
      "index": 0,
      "message": { "role": "assistant", "content": "<block>no</block>" },
      "finish_reason": "stop"
    }
  ]
}
```

## Placement rationale

These files live under `tests/integration/auto-gate-live-http/`, not under the
overlay pack (`templates/overlays/auto-classifier-pilot/`). Reasons:

1. **No render pollution**: the overlay `RenderUnits` walk copies non-excluded
   files from `.vh-agent-harness/overlays/<pack>/` into `.opencode/`. A `test/`
   dir placed in the overlay would render into `.opencode/` when the pack is
   selected. `tests/` is never part of that walk, so it cannot pollute.
2. **Convention**: AGENTS.md designates `tests/integration/` for "infrastructure
   integration or layer handoffs" — a Dockerized mock + real HTTP path is
   exactly this category.
3. **The SUT is COPY'd** into the tester image from the overlay source path
   (`templates/overlays/auto-classifier-pilot/plugins/auto-gate-live.js`),
   which is stable regardless of whether the pack is selected or rendered.

## Files

| File                          | Purpose                                                  |
|-------------------------------|----------------------------------------------------------|
| `mock-llm-server.js`          | Zero-dependency Node http mock server                    |
| `Dockerfile.mock-llm`         | Builds the mock image (`node:22-alpine`)                |
| `Dockerfile.tester`           | Builds the tester image (SUT + prompt + test, no ports)  |
| `docker-compose.yml`          | Private network + two services (mock-llm, tester)        |
| `auto-gate-live-http.test.mjs`| Integration test suite (`node:test`, pure client)       |
| `README.md`                   | This document                                            |
