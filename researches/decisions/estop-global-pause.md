---
type: decision
date: 2026-08-08
scope: coordination/runtime layer — single-bit fail-safe global pause (pause NEW work only)
status: research-complete, decision-recorded (operator-pre-classified P1 BORROW)
source-basis: refs/hermes-agent @ 005421d888a40865cc61d143ff77efd87a037a1e (gitignored transport), cross-checked
---

# ESTOP Global Pause — Decision Memo

## Decision statement

**BORROW** hermes's ESTOP-shaped global pause for the harness's
coordination/runtime layer: a **single-bit, fail-safe, pause-NEW-work-only**
global control. Engaging the pause holds back new dispatch (subagent spawn,
background-job launch, coordinator/cron tick) without killing in-flight work.
It is a resumable circuit breaker, not a panic/exit. This gives the operator
a one-action kill-switch for the runaway-turn / stranded-work / token-loop
failure class that saturates the dogfood opencode history.

This is a recommendation the operator will decide on; the memo body does not
speak as live repo policy.

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

- **OPT-A — Borrow hermes ESTOP (sentinel file, fail-safe, pause-new-work-only) (RECOMMENDED).**
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
  pattern. ESTOP is deliberately resumable; panic is not. The harness already
  has kill per-job (bgshell `stop`); it lacks pause.

## Recommendation

**OPT-A.** Borrow hermes's ESTOP (framing for a separately-authorized slice):

1. **Sentinel.** A sentinel file under repo-local state
   (`.opencode/state/ESTOP` — repo-scoped, not a `HERMES_HOME`-equivalent;
   see open question). `is_engaged()` = a single `os.Stat`; run on every
   tick, no caching beyond the OS, so engage/disengage takes effect on the
   next check (mirrors `estop.py:59-64`).
2. **Fail-safe.** A corrupt/empty/partial sentinel still counts engaged
   (mirrors `estop.py:17-19`). The pause is authoritative; the optional
   `{reason, engaged_at}` JSON body is metadata only.
3. **Pause NEW work only.** The surfaces that check it (and ONLY these):
   subagent dispatch (before spawning a child), bgshell-job launch (before
   starting a new job — NOT `stop`, which keeps its kill semantics for a
   specific job), and the coordinator/cron tick (before claiming/resuming a
   task). In-flight work is NEVER killed by the pause — it finishes or is
   interrupted separately (mirrors `estop.py:12`).
4. **Per-component log-once.** Each dispatch surface logs once per engagement
   (re-arms after resume) so a long pause does not spam (mirrors
   `estop.py:135-161`).
5. **Recovery.** Disengage the sentinel → resume. Open: auto-resume on
   disengage (surfaces pick up on next tick naturally) vs an explicit
   operator-triggered resume verb.

The interaction with the existing interrupt/steer surfaces is additive, not
overlapping: interrupt/steer acts on a running session; ESTOP acts on the
dispatch boundary. They compose.

## Findings

- **(finding)**: source=refs/hermes-agent/agent/estop.py:1-167, confidence=high, type=fact — hermes ships a verified single-bit fail-safe global pause: sentinel file, one-stat check, fail-safe on corrupt/empty, pause-new-work-only (never kill in-flight), per-component log-once. [confidence high — read directly]
- **(finding)**: source=internal grep (ESTOP|global.pause|sentinel), confidence=high, type=fact — the harness has NO global pause-new-work sentinel; all "sentinel" hits are token/N-A/test-fixture sentinels, none function as a pause control.
- **(finding)**: source=.opencode/skills/bgshell-job/scripts/bgshell_job.py:371-399, confidence=high, type=fact — the only job-level control (`stop`) KILLS in-flight (SIGTERM/SIGKILL); it is the right tool for one job and the wrong tool for global pause-new-work. bgshell has no global pause.
- **(inference)**: source=synthesis, confidence=medium, type=inference — a repo-local ESTOP sentinel consulted by the three dispatch surfaces (subagent dispatch, bgshell launch, coordinator/cron tick) is the minimal mechanism that contains the runaway blast radius in one operator action.
- **(assumption)**: source=operator-cross-check, confidence=medium, type=assumption — the dogfood opencode history is saturated with the runaway/stranded/token-loop failure class; this is operator-provided and not independently re-verified here.

## Contradictions

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

## Promotion targets (if the operator accepts)

When this becomes active guidance, the live targets a follow-up slice would
touch — **not** this memo's job to edit: a new `internal/runtime/` (or
`internal/coordination/`) ESTOP helper (sentinel path, `is_engaged`,
`check_paused`, `engage`/`disengage`), wired into the three dispatch surfaces
(subagent dispatch, bgshell-job launch in `.opencode/skills/bgshell-job/`,
coordinator/cron tick under `.local/coordinator/`), plus engage/disengage
verbs (e.g. a `/pause` + `/resume` slash command or `vh-agent-harness pause`
CLI). Tests: `check_paused` fail-safe-on-corrupt, per-component log-once,
and a "paused → new dispatch held, in-flight NOT killed" behavioral test.