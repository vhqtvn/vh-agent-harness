# Disposition: OpenCode subagent stream-stall → CONFIG-FIRST

> Coordinator-adjudicated. The adjudication is DONE; this memo transcribes the
> confirmed verdict. Do not re-verify the core claims against ground truth here —
> they were re-derived by the coordinator at adjudication time (see Provenance and
> the Verification table).

## Verdict

**CONFIG-FIRST — CONFIRMED.** opencode already ships the fix for the observed
failure class. The single 10 h hang reported (pure half-open TCP, zero bytes, no
error, no close) is fully covered by `provider.<id>.options.chunkTimeout` +
opencode's native automatic retry. **No harness code change required.**

## Observed failure class (covered)

The provider completion stream goes half-open: TCP alive, zero tokens, no error,
no close. The engine's blocked `reader.read()` never returns; the parent
coordinator (blocked in a timeout-less `task` tool await) waited ~10 h.

Root cause restated with citations (paths relative to `refs/opencode/`,
checkout @ `2b2b69d`, version 1.18.5):

- `chunkTimeout` OFF by default (`packages/core/src/v1/config/provider.ts:111-114`)
  → `reader.read()` blocks forever inside the AI-SDK stream
  (`packages/opencode/src/provider/provider.ts:37-76`, the `wrapSSE` per-read
  timer is absent when the option is unset)
- no error is raised → retry policy never enters
  (`packages/opencode/src/session/retry.ts:184-185`, `if (!retry) return Cause.done`)
- parent `task` tool blocked in timeout-less `Deferred.await`
  (`packages/core/src/background-job.ts:292-301` — `wait` accepts a `timeout`
  argument but `packages/opencode/src/tool/task.ts:317-347` never passes it)

**Scope (covered vs not).** CONFIG-FIRST covers the **transient** stall class:
chunkTimeout trips → native retry → retry succeeds → turn completes → parent
unblocks (mechanism below). The **persistent/looping** stall class (too-low
chunkTimeout + a prompt huge enough to keep failing the time-to-first-chunk
window) is NOT covered by chunkTimeout alone — the child never completes and the
parent still hangs via the same timeout-less `job.done` await. That looping case
is why the recommended value is minutes-scale and why `defer-009` (task-await
timeout) exists.

## The fix (operator-side, today)

Set `provider.<id>.options.chunkTimeout` in opencode config (per-provider under
`provider.<id>.options`, or user-global in `~/.config/opencode/opencode.json`).

**Recommended value: 300000 ms (5 min).** The operator has set this locally;
verified working on the live 1.18.4 install.

Mechanism (all upstream-native code paths — detection AND retry):

1. `chunkTimeout` trips on zero-byte stall → `wrapSSE` raises
   `ProviderError.ResponseStreamError("SSE read timed out")`
   (`packages/opencode/src/provider/provider.ts:37-76`, wired at `:1733-1762`;
   duplicate implementation at `packages/core/src/aisdk.ts:83-118`)
2. `ResponseStreamError` maps to a retryable `APIError{isRetryable: true}`
   (`packages/opencode/src/session/message-v2.ts:665-675`)
3. retry accepts any retryable `APIError`
   (`packages/opencode/src/session/retry.ts:71-75`)
4. the stream drain is wrapped in `Effect.retry(SessionRetry.policy(...))`
   (`packages/opencode/src/session/processor.ts:660-674`)
5. backoff is exponential, 2 s × 2^n capped at 30 s with no retry-after headers
   (`packages/opencode/src/session/retry.ts:26-29,65`)
6. during backoff, session status becomes `{type:"retry", next}` on the bus
   (`packages/opencode/src/session/processor.ts:664-672`) — bus-visible for
   observability; it does NOT itself drive the unblock (see below).

Detection AND retry are upstream-native. No harness code is involved.

**How the parent unblocks (transient case).** The parent task-await resolves on
child JOB COMPLETION (`Deferred.await(job.done)`, `background-job.ts:296`), NOT
on retry-status bus publication. So chunkTimeout + native retry unblocks the
parent in the observed transient case: stall → trip → retry → retry succeeds →
turn completes → child job completes → `job.done` resolves → parent unblocks.
The `{type:"retry", next}` status is bus-visible (useful for observability) but
does not itself drive the unblock.

**Looping case is NOT covered by chunkTimeout alone.** If retry never succeeds
(too-low chunkTimeout + a prompt huge enough to keep failing the time-to-first-
chunk window), the child never completes and the parent STILL HANGS — via the
same timeout-less `job.done` await that produced the original 10 h hang. That is
the parked `defer-009` (task-await timeout) gap, NOT covered by chunkTimeout.
This is why the recommended value is minutes-scale (see the caveat) and why
`defer-009` exists.

## Caveat that MUST travel with this recommendation (TTFT + unbounded retry)

This is the load-bearing caveat for the disposition.

- `chunkTimeout` is **byte-level** and bounds **time-to-first-body-chunk** (≈
  time-to-first-token for SSE providers) and all inter-chunk gaps thereafter.
  The timer starts at the first body-read AFTER response headers (created
  inside `ReadableStream.pull()` at `provider.ts:45-53`); `fetchFn(...)` is
  awaited (headers received) before `wrapSSE` is called
  (`provider.ts:1755-1762`). The request-dispatch-to-headers gap is bounded by
  the SEPARATE `headerTimeout` — an optional, provider-configurable pre-header
  bound that the OpenAI integration defaults to 300000ms
  (`OPENAI_HEADER_TIMEOUT_DEFAULT` at `provider.ts:35`, wired at `:208` and
  `:1742-1743`); other integrations (e.g. `meta`, `xai`) do not default it
  (`config/provider.ts:102-110`).
- `retry.ts` `policy()` (`packages/opencode/src/session/retry.ts:176-199`) has
  **NO max-attempts cap**. The only exit is
  `if (!retry) return Cause.done(meta.attempt)` (line 185). Backoff is
  exponential 2 s × 2^n capped at 30 s (`RETRY_MAX_DELAY_NO_HEADERS`, line 28).

Therefore: a `chunkTimeout` value set **TOO LOW** combined with a prompt huge
enough to consistently fail to emit a first body chunk within the window will
**loop indefinitely** (each trip is retryable, retry is unbounded). This is why
the recommended value is **minutes-scale, not seconds-scale**. Reasoning models
legitimately go 60 s+ between visible deltas, and the timer is byte-level
(provider SSE keepalives reset it), so 300 s has near-zero false-positive risk
while cutting worst-case stall from hours to ~5 min + backoff. **Do NOT set this
to seconds-scale** (e.g. 30 s would loop a large-prompt turn forever).

## Latent gap (NOT observed, parked — do NOT over-engineer)

`chunkTimeout` is byte-level: SSE keepalive comments/pings from the provider
RESET the timer, so a "server pings but never emits tokens" stall evades it.
Non-SSE / WebSocket transport paths and a wedged opencode process itself are
also uncovered.

These classes are covered by the parked watchdog plugin (see the defer-007 card
under `.local/coordinator/tasks/`), promoted only on a named trigger. The
**OBSERVED** field failure was pure half-open TCP (zero bytes), which IS covered
by `chunkTimeout`. The keepalive-fed variant is unobserved.

## What is parked behind what trigger

All three parked items share the same trigger set: ANY of
(a) a keepalive-alive-but-dead stall observed anywhere in the fleet,
(b) a retry-loop incident, or
(c) a second consumer stall report of any class.

- **Harness watchdog plugin** (Leg A config-hook injection of `chunkTimeout` as
  a backstop + Leg B event-hook abort for keepalive-fed stalls; full design in
  the study): parked behind the trigger set. See
  `.local/coordinator/tasks/defer-007-stall-watchdog-plugin.json`.
- **Upstream PR — default `chunkTimeout`**: parked behind the trigger set. The
  schema slot already exists at `packages/core/src/v1/config/provider.ts:111-114`;
  only a default value is missing. See
  `.local/coordinator/tasks/defer-008-upstream-pr-default-chunktimeout.json`.
- **Upstream PR — task await timeout**: parked behind the trigger set.
  `background.wait` already supports a `timeout` param
  (`packages/core/src/background-job.ts:292-301`) but
  `packages/opencode/src/tool/task.ts:317-347` never passes it. This is why the
  10 h hang reached the parent at all. See
  `.local/coordinator/tasks/defer-009-upstream-pr-task-await-timeout.json`.

These are transport, not truth (per AGENTS.md DEFER curation rule). They reach
`docs/planning/backlog.md` only if a trigger fires AND the promotion Definition
of Ready is met.

## Provenance

- Completed study (full detail, citations, Leg A/B watchdog design):
  `tmp/agent-runs/stall-watchdog-study/2026-07-25-opencode-stall-study.md`
- opencode source checkout: `refs/opencode/` @ `2b2b69d668ed05836ea6d3fa7f42d416bdb61806` (1.18.5)
- Live opencode install: 1.18.4 (one-patch drift from the cited checkout; low
  residual risk, noted in the study's open questions — all cited code paths
  must be re-derived against the live version before any parked item is built)
- Field report: vh-solara operator, 10 h subagent stall (pure half-open TCP,
  zero bytes, no error, no close)

## Verification

| Claim | Verifying command/output | Verified |
|-------|--------------------------|----------|
| `chunkTimeout` exists in schema (off by default, no default value) | `refs/opencode/packages/core/src/v1/config/provider.ts:111-114` | yes |
| `chunkTimeout` enforced via `wrapSSE` (per-read timer → abort controller + cancel reader) | `refs/opencode/packages/opencode/src/provider/provider.ts:37-76`, wired at `:1733-1762`; duplicate `refs/opencode/packages/core/src/aisdk.ts:83-118` | yes |
| `chunkTimeout` documented upstream ("Timeout in milliseconds between streamed response chunks … If no chunk arrives in time, the request is aborted.") | `refs/opencode/packages/web/src/content/docs/config.mdx:371-389` | yes |
| `chunkTimeout` tested (fires on SSE body stall) | `refs/opencode/packages/opencode/test/provider/header-timeout.test.ts:49` ("chunkTimeout raises a response stream error when SSE body stalls"); `refs/opencode/packages/opencode/test/provider/provider.test.ts:295` | yes |
| Native retry chain wired end-to-end | `refs/opencode/packages/opencode/src/session/message-v2.ts:665-675` → `refs/opencode/packages/opencode/src/session/retry.ts:71-75` → `refs/opencode/packages/opencode/src/session/processor.ts:660-674` | yes |
| `retry.ts` has NO max-attempts cap (only exit: `if (!retry) return Cause.done`) | `refs/opencode/packages/opencode/src/session/retry.ts:176-199` (exit at line 185) | yes |
| Harness already references `chunkTimeout` as operator-side defense layer #1 | `templates/core/.opencode/plugins/maxoutputtokens.js:28` ("provider.options.timeout + chunkTimeout — kills stuck streams") | yes |
| `.local/` and `tmp/` paths are gitignored (defer cards + study stay out of commit) | `git check-ignore .local/coordinator/tasks/defer-007-stall-watchdog-plugin.json .local/coordinator/tasks/defer-008-upstream-pr-default-chunktimeout.json .local/coordinator/tasks/defer-009-upstream-pr-task-await-timeout.json tmp/agent-runs/stall-watchdog-study/2026-07-25-reply-to-reporter.md` → all paths echoed (ignored) | yes |

## Behavioral closure

```behavioral-closure
verdict: inconclusive
result: not-demonstrable
crux: none — this is a documentation-only change recording an already-adjudicated
  disposition. There is no load-bearing code path in this slice to exercise; the
  disposition's behavioral claim (chunkTimeout covers the observed zero-byte
  stall class on live 1.18.4) was verified against ground truth by the
  coordinator at adjudication time and is recorded here, not re-proven by this
  slice. The live verification (operator set 300000ms locally, working on 1.18.4)
  is field-observed and cited in Provenance, not reproduced by a repo-internal
  command in this slice.
```
