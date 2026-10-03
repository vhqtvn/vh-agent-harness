# session-progress-pilot

A **default-out**, audit-default overlay pack shipping a single-hook plugin
(`session-progress.js`) that detects **looping tool calls** — the same exact
tool invocation repeated with unchanged adverse results — and selectively
**denies that one exact invocation** with a bounded reason.

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
  hit (≤2000 ms total including retry; ≤5 ms target on the local path)
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
3. Behavior is **audit by default**: zero denials, diagnostic records only.

## Operator-owned configuration

Config lives in ONE optional file (absent → safe defaults, audit mode):

`.opencode/repo-configs/session-progress.local.json`

The file is read on every hook invocation (mtime-cached; edits apply on the
next tool call, no restart). A present-but-invalid file falls back to defaults
with one deduplicated stderr notice — it NEVER throws. No secret values in the
file or in diagnostics; the judge endpoint/model/key are supplied via
environment variables referenced BY NAME (see `judge.*_env` below).

| Field | Type | Default | Meaning |
|---|---|---|---|
| `enabled` | boolean | `true` | Master toggle. `false` = plugin no-ops (allow everything, no records). |
| `agents` | object mapping agent name → `off`\|`audit`\|`enforce`, or the single string shorthand | `{"*": "audit"}` | Per-agent mode. `*` is the wildcard. Default is **audit for everyone** — the shipped state changes zero behavior. `enforce` may deny; `off` disables observation for that agent. With ONLY the `*` key, no agent attribution is needed (the mode is unambiguous). When any specific agent key is present, a call with UNKNOWN attribution can never be denied (audit ceiling). |
| `judge.model_env` | string (env var NAME) | `"SESSION_PROGRESS_JUDGE_MODEL"` | Env var holding the judge model ID. Unset → semantic judging unavailable → allow (recorded). |
| `judge.endpoint_env` | string (env var NAME) | `"SESSION_PROGRESS_JUDGE_ENDPOINT"` | Env var holding an OpenAI-compatible chat-completions URL. Unset → judging unavailable. |
| `judge.api_key_env` | string (env var NAME) | `"SESSION_PROGRESS_JUDGE_API_KEY"` | Env var holding the API key VALUE — the key never lives in the config file or diagnostics. |
| `judge.timeout_ms` | number | `2000` | TOTAL slow-path deadline (history read + judge fetch + retries all share it). At deadline → allow. Clamped to [250, 2000] — the 2000 ms ceiling is the pinned deadline invariant, configurable down only, never up. |
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
   total 2000 ms deadline. Only `looping` verdicts with confidence ≥0.90,
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
