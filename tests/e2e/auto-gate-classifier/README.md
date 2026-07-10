# auto-gate-classifier plugin e2e

A fully-managed, Docker-isolated **plugin e2e** test for the
`auto-classifier-pilot` overlay. This is NOT a full-OpenCode e2e — it does not
run an OpenCode server. Instead it:

1. Builds the `vh-agent-harness` Go binary from source.
2. Renders the overlay into a fresh temp project (`/tmpproj`) inside the
   container, producing `/tmpproj/.opencode/plugins/auto-tool-gate.js` (and its
   3 siblings).
3. Imports the **rendered** plugin as ESM in a real node process.
4. Drives the plugin's `permission.ask` hook with a faithful OpenCode stand-in
   (fake `client`/`Permission`/`output`).
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
| 5 | Hook contract (`permission.ask` mutates status) | hook invoked the way OpenCode invokes it |
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

| Mode | Mock scenario | Expected `output.status` | Proves |
|------|--------------|--------------------------|--------|
| `audit` | (none) | `"ask"` (unchanged) | audit never mutates status, never calls model |
| `enforce` / `stubVerdict:block` | (none) | `"deny"` | enforce stub block, no model call |
| `enforce` / `stubVerdict:allow` | (none) | `"allow"` | enforce stub allow, no model call |
| `live` / `/allow` | `/allow` | `"allow"` | real HTTP + binary + transcript + verdict parse |
| `live` / `/block` | `/block` | `"deny"` | live block verdict parse |
| `live` / `/recover-after-stall` | `/recover-after-stall` | `"allow"` after retry | retry-on-idle fires through real plugin |

The mock server is reused from `tests/integration/auto-gate-live-http/` (no
duplication).
