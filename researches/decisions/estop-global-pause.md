---
type: decision
date: 2026-08-08
scope: coordination/runtime layer — bounded repo-scoped pause on new work across enumerated dispatch entrypoints
status: research-complete, decision-recorded, implemented-as-ad5f2e16
source-basis: refs/hermes-agent @ 005421d888a40865cc61d143ff77efd87a037a1e (gitignored transport), cross-checked
---

# Bounded Repo-Scoped Pause on New Work — Decision Memo

## Decision statement

Implement a **bounded repo-scoped pause on new work across enumerated harness and OpenCode dispatch entrypoints** — explicitly NOT "global ESTOP," NOT "pause every agent," NOT an agent-loop interlock, and NOT an abort/kill switch. Engaging the pause holds back new dispatch (subagent spawn, background-job launch, coordinator/cron tick) without killing in-flight work. It is a resumable circuit breaker, honestly named `pause-new-work`. This gives the operator a one-action lever for the runaway-turn / stranded-work / token-loop failure class that saturates the dogfood opencode history.

(Hermes's ESTOP mechanism inspired this, but the harness adoption is scoped strictly to what it can honestly own.)

## Refinement history

The original version of this memo proposed borrowing hermes's "global ESTOP" broadly. Three subsequent passes refined its specifics into the committed reality:

1. **Solution-brief pass:** Reframed "global ESTOP" into a bounded repo-scoped pause on new work (`pause-new-work`). Corrected the fail-safe semantics to match the harness's reality (absent = disengaged, which is the opposite of hermes).
2. **Dispatch-path inventory (PROCEED):** A read-only pass classified all 9 new-work admission paths. The load-bearing question — does `@subagent` cross the `TaskTool`/`tool.execute.before` seam? — resolved YES (high confidence). OpenCode's `resolvePart` (`refs/opencode/packages/opencode/src/session/prompt.ts:974-990`) appends synthetic instructions forcing the model to use `TaskTool`, so `@subagent` is model-mediated and covered by the plugin hook.
3. **Build slice (`ad5f2e16`):** Shipped the `pause-new-work` command and sentinel contract. Discovered a contradiction: the solution brief blocked `/resume-task`, but `/resume-task` serves BOTH new dispatch (`ready→working`) and in-flight continuation (`working→working`). Blanket-blocking it would violate the invariant that in-flight work is untouched. Resolved in favor of the invariant: `/resume-task` is permitted, the precise JS gate sits at `activateCoordinationTask @ state-lib.js:6136-6255` (ready→working seam), and the OpenCode plugin blocks `tool==="task"` (new-child dispatch) ONLY. (The solution-brief incorrectly cited `state-lib.js:5961-6044` for this function; the correct address is `6136-6255`.)

## Why this is P1 (decision context)

The dogfood harness runs many subagent sessions per root (~67 per the operator
cross-check). When one goes runaway — an infinite tool-call loop, a stranded
archive recovery, a token-burn spiral — the blast radius is large because
dispatch surfaces keep spawning NEW work while the runaway consumes budget.
Today the operator's only levers are per-job (kill the specific bgshell job)
or per-session (interrupt/steer one session). There is no single action that
says "stop starting new work everywhere, let what's running finish or be
interrupted deliberately, while I diagnose." That gap is the difference
between containing a runaway in seconds versus chasing it across N concurrent
sessions. Hermes solved exactly this with a sentinel-file ESTOP that the
dispatch surfaces check on every tick. P1 because the failure class it
addresses is recurring and costly, and the mechanism is verified, cheap, and
fail-safe.

## Hermes finding (verified mechanism)

`source=refs/hermes-agent/agent/estop.py` (167 lines). A resumable global
pause for NEW work.

- **Sentinel file at `$HERMES_HOME/ESTOP`.** `sentinel_path()` (`:54-56`).
- **Cheap check.** `is_engaged()` (`:59-64`) = a single `exists()` (one
  `os.stat`); "callers may run it every tick; no caching beyond the OS"
  (`:12-15`), so engage/disengage takes effect on the very next check.
- **Fail-safe.** "A corrupt or empty file still counts as engaged (fail
  safe): the pause must hold even if the file was created by
  `touch ~/.hermes/ESTOP`" (`:17-19`). `get_state` (`:97-115`) returns
  engaged-with-null-metadata on an unreadable body — the pause is
  authoritative, the metadata is not.
- **Pauses NEW work only, NEVER kills in-flight.** "In-flight work is NEVER
  killed — this is pause-new-work, not panic/exit" (`:12`). Call sites skip
  dispatching due work but do not terminate running work (`:6-10`): the cron
  scheduler tick, the kanban dispatcher, and gateway new-turn handling.
- **Per-component log-once.** `check_paused(component, logger)` (`:135-161`)
  returns True when engaged, logging once per engagement per component (re-arms
  after resume) so a long pause does not spam one line per tick.
- **Engage/disengage.** `engage(reason)` (`:67-83`, idempotent, writes JSON
  `{reason, engaged_at}`; best-effort `touch` on write failure so a partial
  sentinel still pauses); `disengage()` (`:86-94`, removes the sentinel).
- **Provenance.** "Ported from: gastownhall/gastown estop.go (MIT)" (`:21`),
  with explicit contrast to `/panic` (kill/exit semantics, deliberately
  different) and to interrupting in-flight cron (deliberately out of scope).
  [from pass-2 internals, confidence high — read directly by this researcher]

## Consumer / dogfood field evidence (operator cross-check, studied 2026-08-08)

`source=operator-cross-check, studied:2026-08-08` — operator-provided; this
researcher cannot re-verify the opencode history aggregates from here.

- The dogfood opencode history is saturated with the failure class an
  ESTOP-shaped pause would address: "Recovering stranded archive work",
  "Subagent stall chunkTimeout", "Runaway-turn watchdog design brief",
  "token-loop watchdog".
- Dogfood runs ~67 subagent sessions per root — the blast radius of a single
  runaway, left uncontained, is large because dispatch keeps spawning.

## This-harness posture (read by this researcher)

The harness has **no global pause-new-work control.** The pause surfaces that
exist are per-job or per-session, and the per-job one KILLS in-flight — the
opposite of ESTOP's semantics.

- **No ESTOP/global-pause sentinel exists.** A grep for
  `ESTOP|estop|global.pause|GlobalPause|pause_new|pauseNew|sentinel` across
  `internal/` returns only token sentinels (`{{PROJECT_SLUG}}`), blessed
  N/A sentinels (`internal/cli/projectconfig.go`), and test fixtures —
  nothing that functions as a pause-control sentinel. There is no single-bit
  global "stop starting new work" switch.
- **The only job-level control kills in-flight.**
  `source=.opencode/skills/bgshell-job/SKILL.md` and
  `source=.opencode/skills/bgshell-job/scripts/bgshell_job.py` — the surface
  is per-job `launch`/`status`/`stop`/`resume`/`list`. `command_stop`
  (`bgshell_job.py:371-399`) sends `SIGTERM` then `SIGKILL` to the child
  process group (`:383-394`) — it TERMINATES the job. This is the right tool
  for "kill this one job" and the wrong tool for "hold back all new work
  globally." States include `stopped` (killed) and `interrupted`
  (`:191-206`) — both are terminal/kill semantics, not pause-new-work.
- **Coordinator dispatch has no global gate.** The local coordinator runtime
  under `.local/coordinator/` (task cards, `/resume-task` execution
  sessions, subagent dispatch) has no global pause consulted before spawning
  a new subagent or resuming a task. Each dispatch surface decides
  independently.
- **The dogfood loop's blast radius is the motivation.** With ~67 subagent
  sessions per root and no global hold, a runaway in one lane proceeds
  alongside normal dispatch in every other lane. An ESTOP lets the operator
  freeze new dispatch across all lanes in one action while diagnosing.

So the gap is precisely a missing single-bit, fail-safe, pause-new-work
sentinel consulted by the dispatch surfaces.

## Options considered

- **OPT-A — Implement pause-new-work (sentinel file, fail-safe, pause-new-work-only) (RECOMMENDED).**
  A sentinel file under repo-local state; `is_engaged()` = one `os.stat`;
  fail-safe (corrupt/empty still engaged); per-component log-once; pause-NEW-
  work only (never kill in-flight). The dispatch surfaces (subagent dispatch,
  bgshell-job launch, coordinator/cron tick) call `check_paused` before
  starting new work. Recovery = disengage sentinel → resume.
- **OPT-B — Per-component pauses instead of one global.** Give each dispatch
  surface its own pause flag. Rejected as the primary mechanism: the failure
  class is cross-lane (a runaway + normal dispatch in other lanes), and the
  operator's containment action must be ONE action, not N. A per-component
  dimension can layer ON TOP of the global (open question), but the global is
  the load-bearing primitive.
- **OPT-C — Kill-based panic/exit (hermes `/panic` shape).** Rejected: kill
  semantics destroy in-flight work (a half-written commit-gate, a partial
  render), which is exactly the "destroying the feature it secures" anti-
  pattern. A pause is deliberately resumable; panic is not. The harness already
  has kill per-job (bgshell `stop`); it lacks pause.

## Recommendation

**OPT-A.** Implement `pause-new-work` (inspired by hermes's ESTOP) for a bounded repo-scoped pause:

1. **Sentinel.** A sentinel file under repo-local state (`.opencode/state/PAUSE_NEW_WORK` or similar — repo-scoped). `is_engaged()` = a single `os.Stat`; run on every tick, no caching beyond the OS, so engage/disengage takes effect on the next check.
2. **Fail-safe (Contract opposite of hermes).** In the harness, ordinary operation has NO sentinel, so "missing=engaged" is impossible. The contract: absent=disengaged; present+valid=engaged; present+malformed/empty/unreadable=engaged (fail-safe); indeterminate filesystem failure=refuse covered work + report degraded. Disengagement, status, diagnostics, and recovery remain reachable even under degraded state.
3. **Pause NEW work only (Implemented as ad5f2e16).** The 3 gate seams that check it (and ONLY these):
   - JS: `activateCoordinationTask @ state-lib.js:6136-6255` (prevents `ready→working` claim, while allowing `working→working` continuation via `/resume-task`).
   - Python: `bgshell_job.py` pre-spawn (before starting a new job, keeping kill semantics for `stop`).
   - Plugin: OpenCode `tool.execute.before` intercepting `TaskTool` (blocks new-child dispatch only, never blanket), plus 4-command dispatch interception.
   In-flight work is NEVER killed by the pause — it finishes or is interrupted separately.
4. **Coverage exclusions.** Ordinary chat (no hook), diagnostic tools (tool-id whitelist), closeout/status/recovery commands (command-name whitelist), and non-dispatch tool calls by an in-flight turn are deliberately NOT blocked.
5. **Per-component log-once.** Each dispatch surface logs once per engagement (re-arms after resume) so a long pause does not spam.
6. **Recovery.** Disengage the sentinel → resume. Auto-resume on disengage; the sentinel's absence IS the resume signal.

The interaction with the existing interrupt/steer surfaces is additive, not overlapping: interrupt/steer acts on a running session; `pause-new-work` acts on the dispatch boundary. They compose.

## Findings

- **(finding)**: source=refs/hermes-agent/agent/estop.py:1-167, confidence=high, type=fact — hermes ships a verified single-bit fail-safe global pause: sentinel file, one-stat check, fail-safe on corrupt/empty, pause-new-work-only (never kill in-flight), per-component log-once. [confidence high — read directly]
- **(finding)**: source=internal grep (ESTOP|global.pause|sentinel), confidence=high, type=fact — the harness has NO global pause-new-work sentinel; all "sentinel" hits are token/N-A/test-fixture sentinels, none function as a pause control.
- **(finding)**: source=.opencode/skills/bgshell-job/scripts/bgshell_job.py:371-399, confidence=high, type=fact — the only job-level control (`stop`) KILLS in-flight (SIGTERM/SIGKILL); it is the right tool for one job and the wrong tool for global pause-new-work. bgshell has no global pause.
- **(finding)**: source=refs/opencode/packages/opencode/src/session/prompt.ts:974-990, confidence=high, type=fact — `@subagent` mentions in OpenCode are model-mediated via `TaskTool` (synthetic instruction appended by `resolvePart`), meaning they are safely intercepted by a `tool.execute.before` hook on `TaskTool`.
- **(finding)**: source=templates/core/.opencode/scripts/state-lib.js:1560-1572, confidence=high, type=fact — the normalizer's ownerless-working downgrade correctly reverts a `working` card with no active owner to `ready`, making it subject to the `activateCoordinationTask` new-work gate on reclaim.
- **(finding)**: source=behavioral-closure, confidence=high, type=fact — engaged sentinel refuses covered new-work admissions while in-flight work is untouched (16/16 JS tests + 10/10 Go tests + live e2e CLI). Commit reachable on main.
- **(inference)**: source=synthesis, confidence=medium, type=inference — a repo-local ESTOP sentinel consulted by the three dispatch surfaces (subagent dispatch, bgshell launch, coordinator/cron tick) is the minimal mechanism that contains the runaway blast radius in one operator action.
- **(assumption)**: source=operator-cross-check, confidence=medium, type=assumption — the dogfood opencode history is saturated with the runaway/stranded/token-loop failure class; this is operator-provided and not independently re-verified here.

## Contradictions

- **Contradiction (Brief vs Contract):** The solution-brief proposed blocking the `/resume-task` command, but `/resume-task` is used for continuing in-flight work as well as new dispatch. Resolved in favor of the invariant (in-flight work is NEVER touched): `/resume-task` is permitted, and the specific `ready→working` transition is gated instead at `activateCoordinationTask @ state-lib.js:6136-6255`.
- Hermes's ESTOP is explicitly contrasted with `/panic` (kill/exit) and with
  interrupting in-flight cron (`estop.py:21-24`). This memo honors that
  distinction: the recommendation is pause-new-work ONLY, and the existing
  bgshell `stop` (kill) and per-session interrupt are NOT subsumed — they
  remain the tools for terminating specific running work. No contradiction;
  the surfaces compose by scope.
- No contradiction with the harness's transport-not-truth model: an ESTOP
  sentinel under `.opencode/state/` is local runtime state (consistent with
  the drift exemption for `.opencode/state/`, `source=internal/drift/drift.go:137-142`),
  not committed truth. It is gitignored transport, lost on clone — which is
  correct for a pause control (a pause is operator-action-scoped, not
  durable policy).

## Risks / open-questions

- **Live OpenCode Runtime Execution:** The OpenCode plugin hooks are proven at the handler-logic level, but live OpenCode runtime invocation is the one not-demonstrable element (tracked as a DEFER, resting on the shell-guard precedent).
- **Sentinel location.** Repo-local `.opencode/state/ESTOP` (gitignored,
  drift-exempt — consistent with the existing `.opencode/state` exemption in
  `drift.go:137-142`) vs a `HERMES_HOME`-equivalent global. Recommendation:
  repo-local, because the harness is repo-resident and the pause should scope
  to the repo's dispatch surfaces, not the operator's whole environment. Open:
  does the coordinator runtime under `.local/coordinator/` need a separate
  sentinel, or does one repo-local sentinel cover both `.opencode/` and
  `.local/coordinator/` dispatch?
- **Global vs per-component.** Is one global pause enough, or does the
  operator need per-component granularity (pause subagent dispatch but not
  bgshell)? Recommendation: ship the global first (the load-bearing
  primitive); per-component can layer on top as additional sentinels checked
  by the same `check_paused` helper. Hermes is global-only and that suffices.
- **Resume ceremony.** Auto-resume on disengage (each surface picks up new
  work on its next tick — simplest, matches hermes) vs an explicit
  `/resume-everything` verb. Recommendation: auto-resume on disengage; the
  sentinel's absence IS the resume signal.
- **Interaction with the runaway-turn / token-loop watchdogs.** If a future
  watchdog detects a runaway and auto-engages ESTOP, the pause becomes
  automated containment. Open: should the watchdog be allowed to ENGAGE
  (operator-only disengage), or is engage operator-only for v1?
  Recommendation: v1 operator-only engage/disengage; automated engage by a
  watchdog is a later, separately-authorized layer (it must not create a
  self-DoS where a flappy watchdog toggles the pause).
- **Fail-safe direction.** Confirm "fail-safe = engaged": a sentinel the
  operator cannot read, or a stat that errors, must count as ENGAGED
  (mirrors `estop.py:17-19,63`), not disengaged. This is the opposite of the
  shell-guard fail-closed direction (deny on fault) but the same principle:
  the safe direction is the one that holds the protective state.

## Recommended durable artifact path

`researches/decisions/estop-global-pause.md` (intended target; staged under
`tmp/decisions-staging/` — read-only execution policy denied the direct
write; see session handoff).

## Promotion targets (Implemented)

This was implemented as `ad5f2e16` on main (reachable) "pause on new work". The slice shipped the ESTOP helper, the 3 gate seams (JS `activateCoordinationTask`, Python `bgshell_job.py` pre-spawn, OpenCode plugin `tool.execute.before` TaskTool + 4-command dispatch interception), and the `pause-new-work` CLI command.

Behavioral-closure proven: engaged sentinel refuses covered new-work admissions while in-flight work is untouched (16/16 JS tests + 10/10 Go tests + live e2e CLI).
