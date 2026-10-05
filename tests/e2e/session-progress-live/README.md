# session-progress-live — live-runtime receipt e2e

Drives the **session-progress-pilot** plugin inside a REAL opencode process
(the installed host binary — currently 1.18.34) and captures the
live-runtime receipts required by card `defer-session-progress-live-receipts`
before any Phase-2 enforce activation, default-in promotion, or "live-proven"
claim.

Unlike `tests/e2e/auto-gate-opencode/` (Docker-isolated, source-built
opencode), this suite runs the **installed binary directly on the host** —
that is its point: receipts against the exact runtime the operator uses.
It is fully self-contained: scratch projects under
`tmp/agent-runs/session-progress-live/fixture/` with their own
`.opencode/` trees and a `git init` fence (the config up-walk stops at the
scratch root; no repo profile change, no pack selection, no restart of any
interactive session).

## Run

```
make test-e2e-session-progress-live        # all legs (~4 min)
vh-agent-harness exec node tests/e2e/session-progress-live/run-e2e.mjs
# single leg: LEG=A|B|C|D   kill timer: KILL_MS=90000
# child forensic logs: OPENCODE_DEBUG=1    keep fixture dirs: KEEP=1
```

Prerequisite: `opencode` on PATH. The plugin/script bytes are copied from
`templates/overlays/session-progress-pilot/` (the pack carries no render
tokens, so source bytes == rendered bytes; the committed pack test governs
embed equivalence).

Hermeticity (dual-form judge wiring): every leg injects its judge via ENV —
the FALLBACK form — and `buildFixture` pins `judge.user_config_path` to a
nonexistent in-fixture path, so the legs stay hermetic even when the operator
has a real `~/.config/vh-agent-harness/session-progress-llm.json` (whose
per-field literal precedence would otherwise override the mock endpoints).
The legs therefore double as the standing proof that the env fallback path
still works.

## Legs

| Leg | Scenario | Receipt (what PROVES it) |
|---|---|---|
| A | **live enforce deny-throw** — identical failing `bash` loop, mechanical rule | deny text in run output; **deny text in the NEXT MODEL REQUEST BODY** (captured by the mock agent server — the model-visible surface); `verdicts.jsonl` `action:"deny"`; side-effect count < turns; exit 0 |
| B | **live judge-timeout** — stall-headers judge (accepts the TCP connection, never responds) | `verdicts.jsonl` `why:"judge-timeout"` (the plugin's own AbortController deadline — NOT the dead-port `judge-network` class); fail-open; all turns execute |
| C | **live slow-path audit** — audit + scripted valid-verdict judge | `verdicts.jsonl` `kind:"assessment"` rows with `verdict:"looping"`, `judge:true` + `would-deny` rows; never throws. Label: **scripted-judge, real-seam** — the judge is deterministic/local, the hook seam is real |
| D | **dead-endpoint fail-open** — enforce, mechanical off, dead judge port | `why:"judge-network"` rows; NO deny under enforce; all turns execute |

### Pinned cardinalities + completion gate (F4 hardening)

Counts are asserted from **parsed** verdict rows (never substrings), pinned
to the retained receipts (rev `19e6d97`):

- **A**: exactly **1** `action:"deny"` row — mechanical rule, lease-bounded
  reason (`lease_expires`/`lease_hits`/`lease_max` + "temporarily denied
  until" text). Structural: the lease is maxHits=1 and cannot re-arm from
  its own denial.
- **B**: exactly **6** `why:"judge-timeout"` rows and **zero**
  `judge-network` rows. Structural: 8 scripted turns − 2 spend-gate skips;
  every post-gate call is assessed before it executes and each stalled
  judge burns the full 800 ms deadline, keeping later cadence intervals
  above the 1 s floor.
- **D**: exactly **5** `why:"judge-network"` rows and zero deny rows. The
  count rests on call 4 landing ~0.87 s after the first assessment (inside
  the 1 s cadence floor → third skip); that margin is ~130 ms, so sustained
  host jitter could legitimately produce 6 — if that ever flakes, relax to
  `>=5` and keep the zero-deny/all-executed class guards.
- **C**: **no exact census** — the mechanical/semantic would-deny split
  races 1 s lease windows against call gaps that grow with conversation
  length. Asserted instead: every parsed assessment is `judge:true` +
  `verdict:"looping"`, ≥1 `would-deny` row, zero hard `action:"deny"` rows,
  and zero judge failure-class rows.
- **All legs**: the opencode child must **exit 0**. A SIGKILLed/timed-out
  child (`status: null`) FAILS the leg — its receipt stays on disk as
  forensics, never as a pass.

The agent "model" is a deterministic local OpenAI-compatible mock that
scripts identical failing bash calls (`printf … | tee -a side-effects.txt;
false`) — no real LLM, no egress beyond 127.0.0.1 (plus whatever the opencode
binary itself phones home for).

## Receipts

Per-leg receipts land in `tmp/agent-runs/session-progress-live/receipts/`
(`<leg>-receipt.json` + `<leg>-verdicts.jsonl` copy), bound to the repo git
rev: command + outcome summaries, never raw dumps. Each receipt also carries
the **D-F1 byte binding** (card requirement): `git rev-parse HEAD` PLUS a
sha256 per fixture-copied plugin/judge/config file (`d_f1_binding` block) —
HEAD alone cannot bind uncommitted pack edits; the hashes bind the exact
working-tree bytes that ran. Receipts are tmp-only and
MUST NOT be committed. Child stdout/stderr are retained alongside as
`<leg>.child-*.log` for forensics.

## Gotchas baked into this driver (bit-rot prevention)

- **stdin must be `"ignore"` when spawning `opencode run`.** Non-TTY stdin
  makes the run command `await Bun.stdin.text()` — reading piped stdin to
  EOF — BEFORE it creates the session or sends the prompt. A default spawn
  pipe never EOFs, so the child stalls forever right after `init` with two
  idle TLS sockets and an empty stdout. (This was misdiagnosed in the
  2026-10-03 session as a "post-first-stream mock-SSE vs ai-sdk" stall; the
  bisect + procfs probe on 2026-10-04 refuted that — root cause was stdin.)
- **`git init` inside the scratch** fences the config up-walk at the scratch
  root (`~/.opencode` is still read — it is loaded literally from `$HOME`).
- **The mock server must be in-process (async spawn, not spawnSync)** — a
  synchronous child would starve the mock's event loop.
- **Judge timeout_ms floors at 250 ms and ceilings at 20000 ms** (config
  clamp; the 20000 default/ceiling is the 2026-10-05 operator decision from
  measured real-gateway latency). Every leg PINS a short explicit
  `timeout_ms` (500–1500 ms) in its fixture config so legs stay fast and
  deterministic instead of inheriting the 20 s production default; the
  stall-headers leg uses 800 ms.
- A mechanical deny with `max_denials: 1` denies exactly ONE call (7/8
  side effects): the lease then caps and later identical calls are allowed
  again until it expires.
