# Context-Pruning Behavioral Spec (continuity oracle)

> **What this is.** A harness-owned **behavioral oracle** that defines what
> "P3 context-continuity is sufficient" actually means. It is the baseline
> against which a P3 sufficiency *or* insufficiency claim is measured. Without
> it, neither can be asserted or tested.
>
> **Why it exists.** Named as a required-before-triggers artifact in
> `researches/decisions/2026-07-23-dcp-ownership-layer.md` (§5, §6-SQ5). It is a
> hard precondition for every P2-revisit trigger, for any P1 experiment, and for
> any "P3 failed" claim. Authored **source-independent**: it contains no
> reference to any external context-pruning engine's implementation, code,
> prompts, or architecture.
>
> **Scope.** This document specifies *behavior* — observable outcomes after
> compaction and recovery. It does not redesign the §4.2 premise-recheck
> protocol (owned by
> `researches/decisions/2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md`
> and cited here only as an unstable upstream interface). It does not implement
> or modify the current architecture.

## How sufficiency is read off this spec

A continuity mechanism is **sufficient for a profile** iff, on the workload
fixtures defined for that profile (§3), **no insufficiency signal in §6
fires**. Sufficiency is per-profile, not global: a mechanism may be sufficient
for ordinary task continuity and insufficient for correctness-sensitive
continuity, and that split is a legitimate outcome.

Conversely, **a single reproducible §6 signal on a defined fixture is a
measured correctness failure** for that profile and satisfies the "Measured P3
correctness failure" trigger condition in SQ5 of the DCP ownership-layer
record (it is *what* "failure" is measured against).

All invariants below are stated as **falsifiable observable outcomes**. There
is no "works correctly" or "maintains continuity" without a defined signal.

---

## 1. Continuity profiles

Two profiles are defined. A session's profile is determined by whether a stale
premise post-compaction could cause an *incorrect action against mutable
external state*.

### 1.1 Ordinary task continuity

Routine multi-turn work where compaction occurs mid-session. The load-bearing
risk is **losing the thread**, not **acting wrongly on a stale fact**: if
critical state is dropped, the resumed session should stall, ask, or re-derive
— not silently do the wrong thing against the world.

- **Applicability.** Implementation, research, debate, planning, docs, and
  coordination sessions whose next action does not depend on a mutable external
  fact (file path, git ref, version string, repo structure, resolved command
  output).
- **Critical-invariant class.** *Obligation retention*: the mission, hard
  constraints, non-goals, required outputs, the exact final-response format,
  the current next step, and the active blocker survive in an actionable form.

### 1.2 Correctness-sensitive continuity

Work involving mutable external state where a stale premise post-compaction
could cause an *incorrect action* rather than a stall — e.g. resuming work
that touches specific file paths, git refs, version claims, or repo structure
captured before compaction.

- **Applicability.** Any session whose first resumed action depends on a
  mutable premise (a fact that could change between capture and resume).
- **Critical-invariant class.** Everything in 1.1, **plus premise-freshness**:
  every mutable premise load-bearing for the first resumed action must carry an
  *observable re-derivation attempt* before the action is taken.

A session may migrate between profiles mid-run. When it does, the
correctness-sensitive invariants apply from the first action that depends on a
mutable premise.

---

## 2. Critical-fact set per profile

These are the facts that **MUST survive compaction** for each profile. "Survive"
means: recoverable from persisted state and/or carried into the post-compaction
context in actionable form (see §8 for which mechanism currently addresses each
and the honest component-vs-composition test status).

### 2.1 Ordinary task continuity — critical-fact set

| Fact | Where it must be recoverable from | Observable survival signal |
|---|---|---|
| **Mission** | task contract `mission` field | resumed session restates a mission that matches the persisted `mission` (same intent; phrasing may differ) |
| **Top hard constraints** | task contract `must_not_do` / `constraints` | resumed session honors each constraint in its first action |
| **Non-goals** | task contract `non_goals` / `must_not_do` | resumed session does not act on a declared non-goal |
| **Required outputs** | task contract `required_outputs` | resumed session's deliverables match the declared output paths/kinds |
| **Exact final-response format** | task contract `final_response_format` | final closeout output conforms to the declared schema/structure |
| **Current next step** | latest checkpoint `next_step` / task contract `must_do` | first resumed action corresponds to the saved next step (or deviates only on new evidence) |
| **Active blocker** | latest checkpoint body / `open_questions` | resumed session either resumes around the blocker or re-derives that it is resolved |

### 2.2 Correctness-sensitive continuity — additional critical-fact set

Every fact in 2.1, **plus**:

| Fact | Where it must be recoverable from | Observable survival signal |
|---|---|---|
| **Each mutable premise load-bearing for the next action** | checkpoint/handoff `next_step` premises stored as the 4-tuple `(value, source, re_derivation_command, observed_at)` | the resumed transcript shows the `re_derivation_command` being run (or an equivalent side-effect-free re-derivation) before the first action that depends on the premise |
| **Resolved command output / structure the action targets** | checkpoint body / resolved-context memory | resumed action targets the path/ref/structure confirmed by re-derivation, not the captured value |

A premise is **mutable** if a cheap, side-effect-free command can disagree with
its captured value (e.g. a `grep`, an `ls`, a `test -f`, a read-only
`doctor`-class check, a `git rev-parse`). A premise that cannot change (e.g.
"the task is to author one file") is not mutable and is covered by 2.1.

---

## 3. Workload fixture shapes

These are the *categories* of session shape that define the testing surface.
They are not exhaustive test cases; each category is a representative shape a
concrete corpus is built from. A claim of sufficiency for a profile must hold
across all fixture shapes that profile subsumes.

### 3.1 Single-issue short session (ordinary)

- One compaction, one recovery.
- One focused task; one deliverable.
- Tests: the critical-fact set survives a single compaction and is recoverable
  within the §5 recovery bound.

### 3.2 Multi-step medium session (ordinary)

- 2–3 compactions, progressive obligation retention.
- A task with ordered sub-steps where obligations accumulate (a constraint
  added at step 2 must still bind at step 4 after a compaction between).
- Tests: obligations declared *before* a compaction still bind *after* it;
  repeated compaction does not shed the oldest obligations preferentially.

### 3.3 Long exploratory session (ordinary, workstream-spanning)

- 3+ compactions; the session may be bound to a workstream.
- Mission is stable, but findings/decisions accumulate across compactions.
- Tests: the mission and required output format survive even as the
  findings-context is summarized and re-summarized; the workstream brief and
  next-slice remain actionable.

### 3.4 Correctness-sensitive session (correctness-sensitive)

- Mutable repo state in play (file paths, git refs, version claims, repo
  structure); at least one premise-recheck boundary (compaction followed by a
  resumed action that depends on a captured mutable premise).
- Tests: §6 premise-freshness signals — every mutable premise load-bearing for
  the first resumed action shows an observable re-derivation attempt.

---

## 4. Accepted nondeterminism boundaries

The native compaction summary is a probabilistic LLM reduction. Some
variability is inherent and acceptable; some is a correctness failure. The
boundary is drawn by *meaning*, not by surface form.

### 4.1 Acceptable variability

- **Summary phrasing may differ.** The post-compaction summary may restate the
  mission, constraints, or next step in different words.
- **Token counts may vary.** Summary length is not pinned.
- **Ordering of injected sections may shift.** The summary may reorder the
  blocks pushed into the compaction context.
- **Summary truncation budgets differ from full records.** The injected context
  is a *summarized* projection of the persisted state (see §8); it is expected
  to be shorter than the full records. Acceptable **only** because the full
  records survive in persisted state and are recoverable via §5.

### 4.2 NOT acceptable (each is a falsifiable insufficiency signal — see §6)

- A **declared critical fact disappears or changes meaning** after compaction.
  (Restatement in different words is fine; omission or semantic drift is not.)
- The **exact required response format is lost** — the final closeout output
  does not conform to the declared `final_response_format`.
- The **first resumed action violates a saved hard constraint** (`must_not_do`)
  without new evidence justifying the deviation.

---

## 5. Recovery expectations

Recovery is the deterministic backstop that makes probabilistic compaction
acceptable: even if a critical fact is degraded in the summary, it must be
reconstructable from persisted state without rebuilding the task from discarded
chat history.

### 5.1 Bounded recovery

- The task contract **and** the latest checkpoint must be reopenable within
  **no more than 2 explicit recovery commands**.
- In the current architecture, `/checkpoint-open` reopens *both* the task
  contract and the latest checkpoint (plus the memory overview) in a single
  call; `/task-contract-open` reopens the contract alone. The bound is therefore
  satisfiable in one command in the common case.

### 5.2 Recovery must NOT depend on discarded chat history

- Recovery must be possible from persisted state alone (task contract,
  checkpoint, memory files). A recovery that requires the operator or agent to
  reconstruct the task from the pre-compaction chat transcript is a §6
  insufficiency signal.

### 5.3 Missing or corrupt state must signal, not silently continue

- Missing, unbound, or corrupt session state must produce a **visible degraded
  signal** (e.g. the current "Session alias: (unbound)" / `StateError` path),
  not a plausible-but-incomplete continuation that acts as if it had full
  context.
- Silent plausible-but-incomplete continuation after missing/corrupt state is a
  §6 insufficiency signal.

---

## 6. Failure thresholds / insufficiency signals

These are the **observable outcomes** that indicate the mechanism is failing for
a profile. Each is independently checkable on a defined fixture. **Any one** is
a measured correctness failure for the profile on which it is observed.

1. **Critical-fact loss or drift.** A declared critical fact (§2) disappears or
   changes meaning after compaction, judged against the persisted ground-truth
   record (task contract / checkpoint), AND the resumed agent did not re-derive
   and update it.
2. **Unjustified next-step deviation.** The first resumed action differs from
   the saved `next_step` without new evidence (a command result, an operator
   instruction, a resolved blocker) justifying the deviation in the resumed
   transcript.
3. **Premise acted on without re-derivation** *(correctness-sensitive only)*.
   The resumed transcript shows an action depending on a mutable premise (a
   file path, git ref, version claim, repo structure) with no preceding
   observable re-derivation attempt (a read/grep/test/`doctor`-class check) of
   that premise.
4. **Progressive obligation drop under repeated compaction.** Across 3
   successive compactions, an obligation declared before the first is no longer
   honored after the third, with no intervening decision to retire it.
5. **Injection-induced or injection-worsened overflow.** The compaction-context
   injection pushes the post-compaction context over the model window, or the
   injected block itself is truncated/dropped such that a critical fact the
   injection was meant to carry is absent.
6. **Critical-fact exceeds deterministic truncation budget and is silently
   dropped.** A critical fact (e.g. a long `final_response_format` or a
   multi-item `must_do`) exceeds the injection's deterministic truncation
   budget (§8) and the surplus is neither injected nor flagged, leaving the
   resumed agent with a truncated obligation it treats as complete.
7. **Silent continuation on missing/corrupt state.** State required for §2 is
   missing or corrupt, but the resumed agent continues as if it had complete
   context (no degraded signal).

Signals 1, 2, 4, 5, 6, 7 apply to both profiles. Signal 3 is
correctness-sensitive-only (ordinary-task continuity does not require
re-derivation of mutable premises because ordinary tasks do not act on them).

---

## 7. Explicit non-requirements

This spec deliberately does **not** require the following. Stating them prevents
over-claiming and pins the seam at which the current architecture would become
insufficient by definition.

### 7.1 Deterministic / replayable compaction is NOT a current requirement

- This spec does **not** require that the same transcript compacts to the same
  summary across runs, nor that compaction be auditable/replayable.
- **If deterministic/replayable compaction becomes a requirement, P3 native
  probabilistic compaction is insufficient by definition** (the native summary
  is a probabilistic LLM reduction made with `tools: []`, i.e. it cannot call
  verification tools mid-summary and is not a deterministic reducer). Meeting
  that requirement would require a separate implementation track (a controlled
  P2 revival or a deterministic upstream seam) — see SQ4/SQ5 of the DCP
  ownership-layer record. That is the precise seam at which engine semantics
  become material; as of this spec no such requirement exists.

### 7.2 This spec does NOT require premise-recheck compliance guarantees

- The correctness-sensitive invariants (§2.2, §6-signal-3) specify the
  *observable outcome* required (a re-derivation attempt is observable before
  the action). They do **not** specify *how* that compliance is achieved or
  enforced.
- The premise-recheck protocol itself is an **unstable upstream interface**
  owned by
  `researches/decisions/2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md`
  (§Mechanism §4.2), which records it as **softened — a protocol discipline,
  not a mechanically-enforced gate**. The spec's correctness-sensitive
  invariants may depend on that protocol's eventual shape; this spec does not
  redesign it and does not assert it is closed.

### 7.3 This spec does NOT require zero information loss

- Some summarization loss is expected and acceptable (§4.1) precisely because
  persisted state is recoverable (§5). The spec requires *actionable
  recoverability* of the critical-fact set, not *perfect preservation in the
  summary*.

---

## 8. Relationship to the current architecture

This section maps each invariant to the mechanism that currently addresses it,
grounded in the actual file contents of the shipped P3 stack. It is **honest
about which mappings are component-tested vs composition-untested**, because
that distinction is exactly the gap this spec exists to measure.

The P3 stack is a five-layer composition: (a) static operating-rules injection,
(b) per-session state injection at compaction time, (c) persisted durable state,
(d) explicit recovery commands, and (e) native probabilistic compaction that
consumes (a)+(b).

### 8.1 Operating-rules injection — static, component-tested

- **Mechanism.**
  `templates/core/.opencode/plugins/compaction-primitives.js` hooks the
  `experimental.session.compacting` event and pushes a **static** "Operational
  Primitives" block (git-mutation routing, shell-guard, tmp hygiene,
  exec-through-`vh-agent-harness`-exec). Same text every compaction; no
  per-session content.
- **Invariant addressed.** Hard operational constraints (must route git through
  `committer`; must not evade shell-guard; must use `./tmp/`).
- **Test status.** Component-tested (the hook fires and pushes the block).
  Composition-untested: whether the native summary preserves these constraints
  in an actionable form.

### 8.2 Per-session state injection — deterministic projection, component-tested

- **Mechanism.**
  `templates/core/.opencode/plugins/session-state.js` hooks
  `experimental.session.compacting`, fetches todos, and calls
  `buildCompactionContext(sessionID, todos)` in
  `templates/core/.opencode/scripts/state-lib.js`. The result is pushed into the
  compaction context. On unbound/corrupt state it degrades visibly to
  "Session alias: (unbound)" + the error message (the §5.3 degraded-signal
  behavior is *already implemented* at this seam).
- **What it injects** (deterministic JS, grounded in `buildCompactionContext`):
  session alias, active workstream, task contract (version + path + summary +
  `final_response_format`), active-plan excerpt, session/workstream memory
  summaries, latest-checkpoint summary, resolved context, recent decisions, open
  questions, artifacts, top todos, and operator-cleared assumptions.
- **Deterministic truncation budgets** (these are the §6-signal-6 budget):
  mission → 4 lines, user requirements → 5 lines, `final_response_format` → 12
  lines (inside the contract summary) and 20 lines (dedicated injection), list
  items (`must_read`/`must_do`/`required_outputs`/etc.) → first 3 items, plan
  excerpt → 14 lines, todos → 5. A critical fact longer than its budget is
  truncated, not flagged.
- **Invariant addressed.** Obligation retention (§2.1) — *the injection
  carries a summarized projection of the critical facts into the compaction
  context*.
- **Test status.** Component-tested (the JS functions and their truncation
  budgets are deterministic and unit-tested). **Composition-untested**: whether
  the native summary, fed this injection, preserves the critical facts in a form
  the resumed agent acts on. This is the probabilistic seam the spec measures.

### 8.3 Persisted durable state — survives compaction, component-tested

- **Mechanism.** Durable files under `.opencode/state/sessions/<alias>/`:
  task contract (`memory/task-contract.{md,json}`), checkpoints
  (`memory/checkpoints/`), handoffs (`memory/handoffs/`), memory files
  (`brief.md`, `resolved-context.md`, `open-questions.md`, `decision-log.md`,
  `artifacts.json`), and workstream state under
  `.opencode/state/workstreams/<slug>/`.
- **Invariant addressed.** Full recoverability of the §2 critical-fact set
  independent of what the summary preserved (the §5.2 backstop).
- **Test status.** Component-tested (atomic write, flock, fault-tolerant read;
  the I/O layer is unit-tested). Composition-untested: whether a resumed agent
  that did *not* receive a fact in the summary actually re-opens the persisted
  state rather than proceeding from the degraded summary.

### 8.4 Explicit recovery commands — bounded, component-tested

- **Mechanism.** `/checkpoint-open` (reopens task contract + latest checkpoint +
  memory overview in one call), `/task-contract-open` (contract alone),
  `/resume-task <id>` (bootstraps a local coordination task, and — notably —
  carries the §4.2 premise-recheck 4-tuple discipline inline in its workflow).
- **Invariant addressed.** §5 bounded recovery (≤2 commands) and §5.2
  independence from discarded chat history.
- **Test status.** Component-tested (each command resolves to deterministic
  `plan_state` reads). Composition-untested: whether an agent *post-compaction*
  actually invokes the recovery commands rather than continuing from a
  degraded summary.

### 8.5 Native probabilistic compaction — the composition seam, UNTESTED end-to-end

- **Mechanism.** The host runtime's native compaction: a probabilistic LLM
  summary made with `tools: []` (it cannot call verification tools mid-summary),
  consuming the context produced by 8.1 + 8.2.
- **Invariant addressed.** None directly — it is the mechanism whose
  sufficiency this spec *measures*.
- **Test status.** **Composition-untested and structurally untestable on P3 for
  determinism.** This is the precise reason §7.1 holds: determinism cannot be
  required of this layer, and if it ever is, P3 is insufficient by definition.

### 8.6 Summary of the honest test-status map

| Invariant class | Current mechanism | Component-tested? | Composition-tested? |
|---|---|---|--- |
| Operating rules (8.1) | static injection hook | yes | **no** |
| Obligation retention (8.2) | deterministic injection projection | yes (the projection) | **no** (summary preservation) |
| Full recoverability (8.3) | persisted durable state | yes (I/O) | **no** (agent re-opens it) |
| Bounded recovery (8.4) | recovery commands | yes (reads) | **no** (agent invokes them) |
| Summary correctness (8.5) | native probabilistic compaction | n/a | **no** (structural) |

The entire sufficiency question reduces to: **on the §3 fixtures, does the
composition (8.1–8.5) avoid every §6 signal?** The component tests prove the
deterministic layers do their part; they do not prove the composition does.
That composition gap is what this oracle makes measurable.

---

## Provenance and constraints honored

- **Source-independent.** Authored from the harness's own architecture and the
  two decision records cited inline. No reference to any external
  context-pruning engine's implementation, code, prompts, or architecture.
- **Does not redesign §4.2.** The premise-recheck protocol is cited as an
  unstable upstream interface (§7.2); only its *observable outcome* is
  specified here (§2.2, §6-signal-3).
- **Architecture claims grounded in disk.** §8 mappings cite the actual shipped
  files (`compaction-primitives.js`, `session-state.js`, `state-lib.js`
  `buildCompactionContext`, the recovery commands, the persisted-state layout)
  and their observed truncation budgets and degradation behavior.
- **See also.**
  `researches/decisions/2026-07-23-dcp-ownership-layer.md` (§5, §3-SQ1, §6-SQ4,
  §6-SQ5 — the record that names this spec as a required-before-triggers
  artifact),
  `researches/decisions/2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md`
  (§Mechanism §4.2 — owner of the premise-recheck protocol cited, not
  redesigned).
