# auto-gate-classifier plugin e2e

A fully-managed, Docker-isolated **plugin e2e** test for the
`auto-classifier-pilot` overlay. This is NOT a full-OpenCode e2e — it does not
run an OpenCode server. Instead it:

1. Builds the `vh-agent-harness` Go binary from source.
2. Renders the overlay into a fresh temp project (`/tmpproj`) inside the
   container, producing `/tmpproj/.opencode/plugins/auto-tool-gate.js` (and its
   3 siblings).
3. Imports the **rendered** plugin as ESM in a real node process.
4. Drives the plugin's `event` hook with a faithful OpenCode stand-in (fake
   `client` delivering `permission.asked` bus events; replies recorded via the
   SDK method the real runtime uses).
5. Exercises the real `vh-agent-harness sys-prompt auto-gate-classifier` binary
   path (no `promptFile` short-circuit).

## Gaps closed (vs. the integration test at `tests/integration/auto-gate-live-http/`)

The integration test imports `classifyLive`/`decideLive` directly as functions.
This e2e additionally covers:

| # | Gap | How |
|---|-----|-----|
| 1 | Plugin loads as ESM in a real node process | `import()` of the rendered plugin file |
| 2 | Overlay renders into a real project | `vh-agent-harness update` at image build time |
| 3 | Config read from rendered file paths | driver writes to `/tmpproj/.opencode/repo-configs/*.json` |
| 4 | sys-prompt binary resolves the prompt | no `promptFile` → `spawnSync("vh-agent-harness",...)` fires |
| 5 | Hook contract (`event` replies to `permission.asked`) | hook invoked the way OpenCode invokes it |
| 6 | Transcript fetch path (`r.data`/`r.error`) | fake `client.session.messages` returns RequestResult shape |

## Run

```sh
make test-e2e-auto-gate
```

Or directly:

```sh
docker compose -f tests/e2e/auto-gate-classifier/docker-compose.yml run --rm e2e-runner
```

Requires Docker Compose. Zero host port publishing — all inter-service traffic
is on a private bridge network (`auto-gate-e2e-net`).

## Scenarios

All scenarios drive the `event` hook (the enforcement surface the real runtime
fires) with `permission.asked` bus events; dispositions are asserted from the
recorded SDK replies.

| Mode | Mock scenario | Expected reply | Proves |
|------|--------------|----------------|--------|
| `audit` | (none) | no reply | audit never replies, never calls model |
| `enforce` / `stubVerdict:allow` | (none) | `"once"` | enforce stub allow, no model call |
| `enforce` / `stubVerdict:allow` + `replyMode:always` | (none) | `"always"` | reply disposition plumbing |
| `enforce` / `stubVerdict:block` | (none) | `"reject"` | enforce stub block, no model call |
| `live` / `/allow` | `/allow` | `"once"` | real HTTP + binary + transcript + verdict parse |
| `live` / `/block` | `/block` | `"reject"` | live block verdict parse |
| `live` / `/recover-after-stall` | `/recover-after-stall` | `"once"` after retry (2 model calls) | retry-on-idle fires through real plugin |

The mock server is reused from `tests/integration/auto-gate-live-http/` (no
duplication).
