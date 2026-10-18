# session-progress-pilot

A **default-out**, **off-by-default** overlay pack shipping a single-hook
plugin (`session-progress.js`) that detects **looping tool calls** — the same
exact tool invocation repeated with unchanged adverse results — and
selectively **denies that one exact invocation** with a bounded reason.

Until the operator opts in via config, the plugin is **completely inert**:
no observation, no records, no diagnostics files, no judge calls, no LLM
spend.

This is a **pilot**, not a safety fence. It intentionally trades recall for a
narrow, low-false-positive deny surface. The uniform `steps: 10000` backstop
remains the only hard bound on unattended sessions; this plugin does NOT
change that and does NOT make unattended sessions "safely bounded".

## Hard design boundaries (pinned by the v2 brief)

- **One hook only**: `tool.execute.before`. No other hook, no engine or
  permission-rule changes, no event subscriptions, no monitor timers.
- **No cancellation, ever.** The plugin never aborts the SDK/tool request.
  A deadline hit means *return allow*, never abort. (The plugin MAY abort its
  own judge HTTP fetch — that is a plugin-owned request, not the tool call.)
- **Teeth = allow / deny-with-reason only.** Deny = `throw` with a bounded
  reason (the shell-guard deny contract), and **only in enforce mode**.
  Audit mode NEVER throws.
- **Fail-open everywhere.** Any error, malformed judge output, stale verdict,
  busy slot, missing config, unknown attribution, oversized args, or deadline
  hit (≤20000 ms total including retry — the 2026-10-05 operator-set ceiling,
  default and clamp max, down-configurable; ≤5 ms target on the local path)
  → the call is ALLOWED.
- **Cadence/time/rate gating throttles judge spend only** — it is never an
  escalation ladder and never a deny reason by itself.
- **No `steps` changes.** The uniform `steps: 10000` backstay stays untouched.
- **No enforcement state persistence across sessions.** Leases/counters live
  in process memory only; diagnostics files are never read back to rearm
  enforcement.

## How to enable (default-out)

The pack ships embedded in the binary and is selected explicitly:

1. Add `session-progress-pilot` to `overlays:` in
   `.vh-agent-harness/vh-harness-profile.yml`.
2. Re-render (`make update` in this repo; `vh-agent-harness update` for
   consumers) and restart the opencode server — the plugin is auto-discovered
   from `.opencode/plugins/session-progress.js`.
3. Behavior is **off by default**: with no config file every agent is `off`
   and the hook is a pure no-op (no observation, no records, no diagnostics,
   no judge calls, no LLM spend). Monitoring starts ONLY when the config
   explicitly names an agent (or `"*"`) as `audit` or `enforce`, e.g.:

   ```json
   {"agents": {"*": "audit"}}
   ```

   or, to watch one agent only:

   ```json
   {"agents": {"*": "off", "build": "audit"}}
   ```

## Operator-owned configuration

Config lives in ONE optional file (absent → safe defaults: every agent `off`,
the plugin fully inert):

`.opencode/repo-configs/session-progress.local.json`

The file is read on every hook invocation (mtime-cached; edits apply on the
next tool call, no restart). A present-but-invalid file falls back to defaults
with one deduplicated stderr notice — it NEVER throws.

**Secret placement rule (hard):** tracked files — including the pack sources
and every shipped default — carry env var NAMES only, never literal secret
values. Literal judge endpoint/model/key values are permitted ONLY in (a) the
gitignored repo-local `session-progress.local.json` and (b) the user-level
file `~/.config/vh-agent-harness/session-progress-llm.json` (see
[Judge target resolution](#judge-target-resolution-dual-form) below).
Diagnostics never contain credentials in any form.

| Field | Type | Default | Meaning |
|---|---|---|---|
| `enabled` | boolean | `true` | Master toggle. `false` = plugin no-ops (allow everything, no records). |
| `agents` | object mapping agent name → `off`\|`audit`\|`enforce`, or the single string shorthand | `{"*": "off"}` | Per-agent mode. `*` is the wildcard. Default is **off for everyone** — the plugin is fully inert (nothing observed, recorded, or spent) until an agent is explicitly opted in as `audit`/`enforce`. `enforce` may deny; `audit` records would-deny and never denies; `off` disables observation for that agent. With ONLY the `*` key, no agent attribution is needed (the mode is unambiguous). When any specific agent key is present, a call with UNKNOWN attribution can never be denied (audit ceiling). |
| `judge.endpoint` | string (literal URL) | `""` | Literal OpenAI-compatible chat-completions URL. Empty = unspecified → next source (user-level file, then env) applies. Gitignored repo-local config only — never commit a literal. |
| `judge.model` | string (literal model ID) | `""` | Literal judge model ID. Same dual-form/empty-falls-through rule as `judge.endpoint`. |
| `judge.api_key` | string (literal key VALUE) | `""` | Literal API key value. Gitignored repo-local config only — NEVER in a tracked file or diagnostics. |
| `judge.model_env` | string (env var NAME) | `"SESSION_PROGRESS_JUDGE_MODEL"` | Fallback: env var holding the judge model ID, used when no literal resolves. Unset → semantic judging unavailable → allow (recorded). |
| `judge.endpoint_env` | string (env var NAME) | `"SESSION_PROGRESS_JUDGE_ENDPOINT"` | Fallback: env var holding an OpenAI-compatible chat-completions URL. |
| `judge.api_key_env` | string (env var NAME) | `"SESSION_PROGRESS_JUDGE_API_KEY"` | Fallback: env var holding the API key VALUE. |
| `judge.user_config_path` | string (path) | `""` | Overrides the user-level judge-file location (empty = `<XDG_CONFIG_HOME or ~/.config>/vh-agent-harness/session-progress-llm.json`). Primarily a test-injection/hermeticity seam. |
| `judge.timeout_ms` | number | `20000` | TOTAL slow-path deadline (history read + judge fetch + retries all share it). At deadline → allow. Clamped to [250, 20000] — the 20000 ms ceiling is the pinned deadline invariant (operator decision 2026-10-05, set from measured real-gateway latency: no sampled model answers <6 s, so a 2000 ms pin guaranteed fail-open for every real judge), configurable down only, never up. Judged calls are cadence-gated: at most one assessment per 60 seconds per session when the new-observation condition is also met. |
| `judge.retries` | number 0..1 | `0` | Extra judge attempts INSIDE the same deadline. Max 1. |
| `judge.min_looping_confidence` | number 0..1 | `0.90` | Minimum confidence for a `looping` verdict to qualify for denial. |
| `cadence.min_interval_seconds` | number | `60` | Minimum seconds between judge assessments (spend control only). |
| `cadence.min_new_signatures` | number | `8` | Minimum NEW call observations (new callIDs, NOT distinct signatures — the name is historical and deliberately misleading; see brief) accrued since the last assessment before the next one may run. |
| `mechanical.enabled` | boolean | `true` | Enables the no-LLM mechanical fast path — only for tools with a verified safe adapter (`bash` only in Phase 1). |
| `mechanical.repeat_threshold` | number | `4` | Nth identical invocation within the window that triggers the mechanical lease (4 = the 4th). |
| `mechanical.window_seconds` | number | `30` | Trailing window for the identical-invocation run. |
| `mechanical.lease_seconds` | number | `20` | Mechanical lease duration. |
| `mechanical.max_denials` | number | `1` | Max denied hits per signature per lease window. |
| `semantic.prior_matches` | number | `3` | Matching completed prior calls of the CURRENT signature within the semantic window required before a `looping` verdict may deny. |
| `semantic.window_seconds` | number | `90` | Semantic evidence window. |
| `semantic.lease_seconds` | number | `45` | Semantic lease duration. |
| `semantic.max_denials` | number | `2` | Max denied hits per signature per 90 s. |
| `polling_exemptions.bgshell_status` | boolean | `true` | Exempts the EXACT documented bgshell-job status grammar (see below) from both denial paths. |
| `state.max_sessions` | number | `128` | Global session-state cap (lazy eviction beyond it). |
| `state.idle_ttl_seconds` | number | `1800` | Sessions idle longer than this are evicted lazily at hook entry. |
| `state.ring_entries` | number | `32` | Max observations retained per session. |
| `state.ring_bytes` | number | `16384` | Max serialized bytes of evidence retained per session. |
| `concurrency.per_session` | number | `1` | Max concurrent judge assessments per session (busy → allow). Pinned to 1 in Phase 1 (key kept for shape stability). |
| `concurrency.global` | number | `4` | Max concurrent judge assessments process-wide. |
| `diagnostics.enabled` | boolean | `true` | Enables bounded diagnostic writing (below). |

Unknown fields are ignored. Wrong-typed values fall back to defaults.

### Judge target resolution (dual form)

Operator decision 2026-10-05 (mirrors auto-gate's documented literal-preferred
dual form). Each of the three judge fields — endpoint, model, api key —
resolves PER FIELD, first-non-empty-wins, across three sources:

1. **repo-config literal** — `judge.endpoint` / `judge.model` /
   `judge.api_key` in `session-progress.local.json` (gitignored);
2. **user-level file** — `<XDG_CONFIG_HOME or ~/.config>/vh-agent-harness/
   session-progress-llm.json`, schema:

   ```json
   { "endpoint": "<url>", "model": "<id>", "apiKey": "<key>" }
   ```

   The auto-gate field spellings are accepted as aliases so a leaf of
   `auto-gate-llm.json` can be copied shape-for-shape: `modelEndpoint` ~
   `endpoint`, `api_key` ~ `apiKey`. `model` is identical in both.
3. **env var** — the value of the env var NAMED by `judge.endpoint_env` /
   `judge.model_env` / `judge.api_key_env` (the original Phase-1 mechanism,
   unchanged, still the fallback).

Rules:

- **Per-field layering** (auto-gate's shallow per-field merge): a partial mix
  is legitimate — e.g. endpoint+model from the user-level file and the key
  from env. Each field resolves independently; an EMPTY value at a level
  means "unspecified" and falls through to the next level. An empty literal
  never suppresses a lower source.
- **All three required**: if ANY of endpoint/model/key resolves empty, the
  judge is `unavailable` — semantic judging disabled, the call allows
  (recorded). There is no partial-judge state.
- **User-level file lifecycle**: absent = silent (the normal
  no-user-config state); present-but-invalid (bad JSON / non-object) = empty
  values + ONE deduplicated stderr notice, then re-warns only on a state
  transition — exactly the repo config's dedup contract. NEVER throws.
- **mtime-cached** like the repo config: one `statSync` per unchanged tool
  call; edits apply on the next call, no restart.
- **No secrets in tracked files** (hard rule): the pack sources and every
  shipped default carry env var NAMES only. Literal values belong ONLY in
  the gitignored repo-local config and the user-level file. `judge.
   user_config_path` may point anywhere (it is a test/hermeticity seam), but
  pointing it at a TRACKED file with secrets violates this rule.

## What is detected

Two deny paths, both exact-signature-scoped (a deny NEVER covers other calls):

1. **Mechanical fast path (no LLM).** The Nth (default 4th) identical
   invocation within 30 s — an unbroken trailing run, no other signature in
   between — where ≥2 of the prior calls have RECORDED, UNCHANGED adverse
   outcomes (bash non-zero exit or tool-error state, observed via bounded
   history enrichment), and the tool has a verified safe adapter (`bash`
   only). Successful reads, unknown outcomes, and unadaptable tools never
   mechanically deny.
2. **Semantic judge path.** When the spend gate opens (≥60 s AND ≥8 new
   observations) and a judge model is configured, a bounded packet (≤32
   observations, scrubbed, ≤512-char assistant excerpt) is assessed under one
   total deadline (default 20000 ms). Only `looping` verdicts with confidence ≥0.90,
   ≥1 valid evidence reference, ≥3 matching completed prior calls of the
   current signature within 90 s with adverse unchanged outcomes, and
   trustworthy attribution may deny. `productive`, `stuck`, and `drifting`
   NEVER deny.

Both paths arm a **lease** (short expiry, small deny-hit cap, no rearming
from the plugin's own denials — only fresh executed evidence rearms).

## Polling exemption (exact grammar)

Legitimate status polling is exempt. The EXACT token grammar (nothing looser):

```
vh-agent-harness exec python  .opencode/skills/bgshell-job/scripts/bgshell_job.py status --job <name> [--lines <1..500>]
vh-agent-harness exec python3 .opencode/skills/bgshell-job/scripts/bgshell_job.py status --job <name> [--lines <1..500>]
vh-agent-harness exec python  .opencode/skills/bgshell-job/scripts/bgshell_job.py status --job-dir <path> [--lines <1..500>]
```

`--lines` may also precede the `--job`/`--job-dir` pair. ANY other token —
command chaining (`&&`, `;`, `|`), other subcommands (`launch`, `stop`,
`resume`, `logs`), extra flags — makes the command non-exempt. Arbitrary bash
commands merely containing the word `status` are never exempt.

## Diagnostics (bounded, scrubbed, best-effort)

When enabled, the plugin writes under `tmp/agent-runs/session-progress-pilot/`:

- `verdicts.jsonl` (+ one rotation `verdicts.jsonl.1`, ≤256 KiB each) — one
  line per notable event (assessment, deny/would-deny, skip/busy/timeout).
- `status.json` (≤64 KiB) — a coalesced snapshot.

Records carry time, pseudonymous session/agent ids, mode, rule, verdict,
confidence, allow/deny/would-deny, bounded scrubbed reason, evidence ids,
signature short-id, outcome summary, latency, error class, and lease state.
NO raw args, full outputs, credentials, or transcripts. Writing is async,
coalesced, single-flight, and NEVER blocks or denies — a failed write is
counted and dropped. Nothing is ever read back for enforcement.

## Fail-open inventory (every one of these → allow)

- config missing/invalid, `enabled:false`, mode `off`
- unknown attribution when any specific agent key is configured
- oversized or non-JSON-ish args (never truncated into a colliding signature)
- spend gate closed, judge slot busy (per-session or global), judge
  unavailable (no endpoint/model/key)
- history read failure or budget exhaustion inside the deadline
- judge timeout / non-2xx / malformed / schema-invalid / adversarial output
- verdict resolving AFTER the deadline (stale assessments cannot deny)
- lease expired or deny-hit cap reached
- any thrown error anywhere in the hook (top-level try/catch → allow)

## Explicit non-goals

- No cancellation, no abort of SDK/tool requests, no mid-stream interrupts.
- No claim that unattended sessions are safely bounded (the `steps: 10000`
  backstop remains the only fence).
- No detection of text-only loops, argument-varying loops, long cycles beyond
  the ring, cross-session loops, or productive-looking useless work — recall
  is intentionally sacrificed for precision.
- No enforcement-state persistence, no config/schema changes to core, no
  permission packs.
