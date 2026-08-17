# DeepSeek Harness (dsh) — Session-Cognition Slice Source Packet

Durable home: researches/sources/deepseek-harness/session-cognition.md (landed 2026-08-17).

## Provenance

- **Source repo**: local checkout at `refs/deepseek-harness` → deepseek-ai/deepseek-harness
- **Commit**: `47f943859bef60e4160492346772ded9b24f765a` (2026-08-13)
- **What it is**: "DeepSeek Harness" (dsh) — a Cordis-based, everything-is-a-plugin
  agent harness (pnpm monorepo, TypeScript). Sessions, compaction, spill,
  subagents, skills, goals, jobs, schedules are all plugin-declared seams over a
  small event-sourced core.
- **Slice scope**: session cognition — `packages/{session, session-query,
  compaction, spill, context, todo, plan, goal, workflow, subagent, skill, jobs,
  schedule}` + `docs/agent-lifecycle.md` + `docs/capability-seams.md`.
- **Audience**: maintainers of vh-agent-harness (Go, repo-resident agent harness
  with sessions, compaction, checkpoints, workstreams, subagent delegation,
  skills, task cards) mining dsh for harness-design ideas.
- **Method**: static reading only. Prior research pass gathered the evidence
  base (~55-60% of in-scope code, ~90% of docs); this pass transcribed it into
  this packet, spot-verified load-bearing citations (see Verification table),
  then performed targeted top-up reads (folded in below, marked ⭐).
- **Stability**: stable as of commit 47f9438 (upstream evolves; re-verify
  specifics before depending on exact line numbers).

## Session lifecycle end-to-end

The end-to-end shape (cited per step):

1. **Session = append-only SessionEvent log.** Core event types are
   merge-extensible: a plugin declares extra event types via declaration
   merging (e.g. the compaction seam adds `compaction/start|summary|end`;
   the hook protocol adds log-only `hook/invoked|result`). Plugin events are
   **log-only** — not `SurfaceEventType`s, no `surfaceOp`, contribute nothing
   to derived history (`docs/subsystems/session.md:11,595`).
2. **LLM history is derived, not stored.** Message-producing events carry
   `surfaceOp: 'append' | {op:'replace', start, end}` markers;
   `foldSurface(events)` returns current sequences plus the sequences shadowed
   by each replacement; `SessionSurface.replaceGeneration` increments per
   committed replacement so incremental consumers distinguish tail growth from
   rewrite. `deriveMessages()` is cached per node
   (`docs/subsystems/session.md:205-333,494-503`).
3. **Turns bracket work.** A turn is `turn/begin … turn/end` in the log;
   agents step through pre-step hooks (context pressure checks, checkpoint
   flushes happen here) → LLM request → tool executions → quiescence
   (`docs/agent-lifecycle.md`, consistent with the verified flush sites in
   `packages/session/session-checkpoint-policy/src/index.ts`).
4. **Durability is fail-closed at boundaries.** Write-behind batching with a
   fixed window (`packages/session/session-persistence/src/write-behind.ts`);
   `session/flush` parallel-checkpoints; the checkpoint policy flushes
   **before LLM dispatch, before top-level tool exec, and at each pre-step**
   (verified: three `ctx.sessions.flush(...)` sites in
   `packages/session/session-checkpoint-policy/src/index.ts:35,72,80`).
5. **Crash recovery closes, never truncates.** Orphaned turns (log ends
   mid-turn) are closed with a synthetic `turn/end {interrupted}`; the log is
   never rewritten (`docs/subsystems/persistence.md`,
   `docs/subsystems/session.md`).
6. **Compaction replaces surface, not log.** See cross-cutting mechanism C2.
7. **Session end seeds invalidate old locks.** `session/end-seed` invalidates
   pre-seed orphan compaction locks (`packages/compaction/compaction-basic/src/region.ts`
   `inspectCompactionEntryState`, verified at :516-523 — scans backwards for
   open turn, unmatched `compaction/start`, latest end-seed).

## Per-package deep-dive

### session/ (14 subpackages)

- **Core model** (`packages/core/session`, documented in
  `docs/subsystems/session.md`): event-sourced surface model as above.
  `replaceGeneration` is the monotone rewrite counter. Verified.
- **session-persistence** (`src/write-behind.ts`, read complete): write-behind
  batching with fixed window; `session/flush` issues a parallel checkpoint
  (does not block the window); crash recovery synthesizes `turn/end
  {interrupted}`. Fact, high confidence.
- **session-checkpoint-policy** (`src/index.ts`, read complete): fail-closed
  flush points — before LLM dispatch, before top-level tool exec, at each
  pre-step (three `ctx.sessions.flush` sites, verified this pass at :35,:72,:80).
- **session-token-meter** (`docs/subsystems/token-meter.md`, read): usage-anchored
  accounting — when the last successful call's usage envelope matches the
  estimate, anchor to it; else heuristic. Snapshots are immutable and stamped
  with `logRevision`; pricing is O(surface) per node. Fact, high confidence.
- **session-projection / projection-cache**: projection-cache examined via ⭐
  top-up (T2) — durable per-session fold checkpoints, fail-soft, seq-stamped;
  projection src itself still unread.

### session-query/

- `docs/subsystems/session-query.md` (first 120/495 lines read): a query
  surface over persisted sessions. **src unread**. Stub — see Coverage.

### compaction/ (compaction-basic, compaction-tool-result-pruner, command-compact)

- **compaction-basic** `src/config.ts`, `src/index.ts`, `src/region.ts` (read
  complete) + pruner `config.ts`:
  - Defaults: `DEFAULT_THRESHOLD_RATIO = 0.8` (trigger at 80% of context),
    `DEFAULT_RETAIN_RATIO = 0.16` (retain tail 16%) — verified
    `compaction-basic/src/config.ts:20,23`. Config validates known keys
    (`thresholdRatio`, `retainRatio`, `retainTokens`, …) and ratio ordering
    (`validateRatioRetention`) — the "fail loud" config hygiene pattern.
  - `region.ts`: region selection picks the shadowed span;
    `buildSummarizationInput` (verified :498-514) builds summarizer input as
    **the conversation's own system prompt + tools + only the shadowed-region
    messages** — so the summarization call is a genuine prefix of the main
    conversation and hits provider KV cache.
  - Summary rides a **user/message with `surfaceOp: replace`** citing
    `sourceEventSeqs` (every shadowed node); `shadowedRange` is a *position
    span*, not a seq interval (post-replace seqs are non-monotonic).
  - **Summary-must-be-smaller check**: if framed summary tokens ≥ shadowed
    tokens, throw (verified `region.ts:374-378`).
  - **Durable lock = unmatched `compaction/start`**: `compaction/start` is
    appended before summarization; `compaction/end` closes it. A crash between
    leaves a lock visible to `inspectCompactionEntryState` (verified :516+).
    `session/end-seed` invalidates pre-seed orphan locks.
  - `compaction/*` events are log-only (no `surfaceOp` of their own; the
    replace marker lives on the summary message).
- **compaction-tool-result-pruner** (`src/config.ts`, `types.ts`, verified this
  pass): deterministic first-tier relief. Defaults `thresholdChars: 8192`,
  `headChars: 4096`, `tailChars: 1024` — **Unicode code points**
  (`codePointLength`), middle-pruned with `PRUNE_MARKER = '\n\n[... tool result
  middle pruned ...]\n\n'` (`config.ts:7-13`). Pruning is itself a durable
  surface replace (this matters for overflow retry, see C3).
- **command-compact**: UNREAD (stub).

### spill/ (spill, spill-local, spill-policy)

- Oversized tool-result text → session-scoped file. Verified in
  `spill-local/src/store.ts`: dir = first 12 hex of `sha256(sessionId)`
  (:74); `mkdir(dir, {recursive:true, mode:0o700})` (:109); file created with
  `open(path, 'wx', 0o600)` (:113) — exclusive-create fails on ANY existing
  path, symlink or not (anti-symlink rationale documented at :100-101).
- `spill-policy/src/index.ts`: when a tool result's UTF-8 size exceeds
  `maxInlineBytes`, save FULL text to spill, inline a branded notice with
  opaque `SpillLocator` + `retrievalHint` (:107). **Preview budget reserves
  the notice's bytes INSIDE `maxInlineBytes`** (:163 comment + logic), so the
  replacement never exceeds the cap. If the notice alone would exceed the cap,
  keep the content inline (warn + best-effort, :177-184). Omitted
  `maxInlineBytes` ⇒ plugin registers nothing (true no-op, :20). Save failure
  keeps inline (best-effort).

### context/ (agent-instructions, time-context, tmux-context, session-reference)

- Group READMEs read; subpackage sources UNREAD except via docs. Injected
  context providers (agent instructions, wall-clock time, tmux state,
  session-reference pointers). Stub — see Coverage.

### todo/

- README + docs coverage read (todo list is a session-scoped durable
  artifact). src UNREAD. Stub.

### plan/

- `docs/subsystems/plan.md` read fully: **plan mode is a log-only whole-value
  event** — `plan/mode` records entering/exiting plan mode as a single
  whole-value record (not incremental edits); pending plan selections are
  appended at the next accepted in-turn pre-step; `exit_plan_mode` goes
  through the user-questions seam (the model proposes, the user confirms).
  src UNREAD beyond docs. Fact from docs, high confidence.

### goal/ (goal, goal-round-driver)

- `docs/subsystems/goal.md` read fully; `goal-round-driver/src/index.ts`
  lines 1-260/445 read:
  - **Revision-CAS GoalRef**: goals carry a revision; updates are
    compare-and-swap on `(goalId, revision)`.
  - Goal and change records are **durable full-snapshot events in the session
    log** (not diffs).
  - `maxGoalRounds` budget → exhaustion blocks with `block(code:'round-limit')`.
  - Round-driver **drives at quiescence**: after the agent settles, flush
    checkpoint + re-check the goal state before admitting the next round
    (prevents a stale read racing the flush).
  - Round attribution: `GoalMessageSource {goalId, revision, round}` stamps
    each round's messages so provenance survives compaction.

### workflow/ (incl. workflow-worker-thread)

- `docs/subsystems/workflow.md` read fully: workflows are durable multi-step
  state machines over sessions; steps resume from the log.
  ⭐ Top-up T4 (`workflow-worker-thread/src/host.ts`): settlement is a
  three-way race (first result / unexpected death / cancellation-grace
  expiry) claimed atomically before teardown; cancel = tell worker (hooks
  throw at next await) + abort child starts + arm `disposeGraceMs` grace →
  force-settle `cancelled` + TERMINATE; settled-guard prevents grace-timer
  leaks on the ordinary await-then-dispose path; force path drives child
  disposal immediately, overlapping the grace — "the thread never outlives
  its run."

### subagent/ (11 subpackages: subagent, spawn-in-process, fork, acp, codex, claude-code, dsh-sdk, tool-subagent, tool-subagent-control, tool-subagent-report, …)

- `docs/subsystems/subagent.md` read fully (600+ lines). Key content (parts
  re-verified this pass):
  - **Two modes**: one-shot `SubagentRun` vs continuable **Activations**.
    Activation state (`running/waiting/settled`) is *derived* from agent
    quiescence + owned-children set; the **Agent inbox is the only FIFO**
    (delivery order authority).
  - **Durable descriptor** (`subagent/descriptor`, `descriptor.ts`): a
    mode-discriminated durable identity appended into the child's own log
    (one-shot: by the provider before first request; continuable: by the
    continuation manager before the initial prompt is admitted). Log-only,
    never in model history, survives compaction via append-only log. The
    identity fold is **last-wins**; the child's own descriptor overrides a
    fork-seeded ancestor's (`subagent.md:283-285`). The descriptor deliberately
    omits `subagentDepth` (cold resume trusts the persisted header
    `delegationDepth` as the monotone floor) and never snapshots the
    merge-extensible `AgentOptions` (`subagent.md:283`).
  - **Delegation depth is durable**: `SessionHeader.delegationDepth` +
    runtime `AgentOptions.subagentDepth`; greater present value wins; cold
    resume cannot lower it; starts reject out-of-domain depth or >
    `request.maxDepth` (`subagent.md:467`, verified this pass).
  - **Fork seeding**: a fork child seeds from a *balanced completed-turn
    prefix* of the parent log (`header.seedLength` is the lineage boundary).
  - **Child→parent reporting**: `reportFrom` channel + manager-authored
    `subagent/settled` notice — distinct provenance kinds so the parent
    transcript never credits the child with runtime words it didn't say.
  - **Enumeration = three-rung ladder** (`subagent.md:289`, verified): (1)
    registry watermark cache for a live child (zero log reads); (2) projection
    checkpoint cache for a cold child — valid only if the cached snapshot
    passes the own-suffix seq gate (an own descriptor is immutable once
    appended, so a passing own-suffix identity is final); (3)
    `persistence.inspect()` refold, bounded concurrency, recomputed per
    listing. The cache is a pure accelerator — any miss/fault falls silently
    to the authoritative refold. Corrupt/malformed/unknown-version descriptors
    fold to a serializable `null` sentinel; a settled candidate with no served
    identity yields one `corrupt` diagnostic; a failed cold inspection yields
    one `unavailable` diagnostic retried next listing (one damaged sibling
    cannot hide healthy children).
  - Providers are sibling plugins (`spawn-in-process`, `fork`, `acp`,
    `codex`, `claude-code`, `dsh-sdk`); model-facing tools are separate
    consumer plugins (`tool-subagent`, `tool-subagent-control`,
    `tool-subagent-report`).
- **subagent/src**: `continuation.ts` + `descriptor.ts` examined via ⭐ top-up
  (T1 — activation/residency model, coldResume, provenance kinds, descriptor
  versioning); remaining ~148 files unread.

### skill/ (incl. tool-skill)

- `docs/subsystems/skills.md` read fully; `skill/tool-skill/src/index.ts`
  lines 1-300/431 read; root gate `scripts/verify-skill-invocation-metadata.ts`
  read complete (re-verified this pass):
  - **SkillInvocationPolicy** `{modelInvocable, userInvocable}` normalized
    from frontmatter keys: Claude `disable-model-invocation: true` ⟺ Codex
    `policy.allow_implicit_invocation: false`. The root script
    (`scripts/verify-skill-invocation-metadata.ts:76-90`, verified) enforces
    the cross-product alignment for repo skills under `.agents/skills/*/` —
    a skill must not be model-invocable in one product and model-disabled in
    the other; manual-only skills must remain user-invocable.
  - **`/name` user gesture is the ONLY path** for model-disabled skills.
  - **Catalog injection**: skill catalog injected as a `<system-reminder>`
    with **digest-based full replacement** (recompute digest; if changed,
    replace the whole injected catalog block — no incremental merges).

### jobs/

- `docs/subsystems/jobs.md` read fully: `<kind>-N` ids (per-kind monotone
  counters); **owner-fenced by sessionId** (fencing is authorization, not
  secrecy — the id proves ownership); **first-wins settlement** (only the
  first settle per job lands); `reported` flag suppresses duplicate
  completion notices; `attachController` admission gate (a UI/controller
  attaches to a job only through it); per-owner concurrency cap (default 10).
  src UNREAD. Fact from docs, high confidence.

### schedule/

- `docs/subsystems/schedule.md` read fully: **session-local only** (no global
  daemon); after/at/every records canonicalized to UTC; fixed-rate catch-up
  **collapses to the latest due occurrence** (no storm replay); dispatch is
  **queued-not-delivered** (enqueue into the session inbox); the scheduler
  waits for full idle + maintenance phase, never `steer()`s the agent
  mid-turn; at-least-once delivery. ⭐ Top-up T3: the live timer is a
  *disposable projection for one exact root agent* over the durable fold —
  restart-safe by reconstruction; `dueDecision` is deterministic
  (earliest-target-then-create; one one-shot or one complete fixed-rate batch
  per wake).

### docs/agent-lifecycle.md + docs/capability-seams.md (fully read)

- **agent-lifecycle.md**: turn/step/quiescence model; agent steps are
  pre-step hooks → request → tool exec → settle; interrupted turns close
  synthetically. The lifecycle is the spine the checkpoint-policy,
  round-driver, and scheduler all hook into.
- **capability-seams.md**: the everything-is-a-plugin doctrine — services are
  declared seams (`ctx.sessions`, `ctx.subagents`, …); providers and consumers
  are separate plugins over the same seam; the core event map is
  merge-extensible with log-only plugin events.

## Cross-cutting mechanisms

### C1. Three-tier context relief (verified)

1. **Tier 1 — deterministic tool-result pruning** (zero LLM cost):
   `compaction-tool-result-pruner` middle-prunes tool results over 8192 code
   points (head 4096 / tail 1024 retained). Pruning is a durable surface
   replace. (`packages/compaction/compaction-tool-result-pruner/src/config.ts:7-13`)
2. **Tier 2 — pressure compaction** (LLM cost, last resort): fires at agent
   pre-step when estimated usage ≥ 0.8× context; retains tail 0.16;
   retries region selection; enforces summary-smaller-than-shadowed.
   (`packages/compaction/compaction-basic/src/config.ts:20-23`, `region.ts`)
3. **Tier 3 — request-error overflow recovery**: on context-overflow errors
   from the provider, retry ONLY if `replaceGeneration` advanced since the
   failed request — i.e. durable progress (even a prune-only replace) is
   sufficient proof the retry has room; otherwise surface the error.
   (`docs/subsystems/compaction.md`)

### C2. Compaction = surface replace, never log deletion

The full log is retained; compaction only changes what `deriveMessages()`
yields. The summary message cites `sourceEventSeqs` (the evidence trail
survives — you can always unfold what was summarized). `shadowedRange` is
positional. Durable lock via unmatched `compaction/start`; end-seed
invalidates pre-seed orphans. This makes compaction crash-safe, auditable,
and reversible-by-replay — at the cost of unbounded log growth (see Open
questions).

### C3. Durable-progress-as-retry-proof

The overflow-recovery rule (retry iff `replaceGeneration` advanced) is a
general pattern: a retry is only justified by *committed, durable* change to
the input, not by in-memory state. Applies equally to prune-only relief.

### C4. KV-cache-preserving summarization

`buildSummarizationInput` = system prompt + tools + shadowed-region messages
only (`region.ts:498-514`, verified). Because it is a genuine prefix of the
main conversation, the provider's KV cache makes the extra summarization call
cheap. A harness-level design choice that respects provider economics.

### C5. Subagent spawning & handoff

One-shot runs vs continuable Activations; inbox-FIFO ordering; derived (not
stored) activation state; last-wins descriptor fold for identity; reportFrom +
manager-authored settled notices for provenance-clean transcripts; durable
delegationDepth so recursion budgets survive resume; three-rung enumeration
ladder (watermark → projection cache w/ seq gate → persistence inspect).

### C6. Persistence & query

Write-behind batching + parallel flush + fail-closed boundary flushes +
synthetic turn closure. Query (`session-query`) is a separate read surface
over the persisted log — src unread, docs partially read.

### C7. Scheduling & background work

Jobs (owner-fenced, first-wins, attach-gated) vs schedule (session-local,
UTC-canonicalized, queued-not-delivered, idle+maintenance-phase dispatch,
at-least-once). Neither may steer the agent mid-turn.

### C8. Config hygiene

Every config surface validates unknown keys, mutual exclusions, and ratio
ordering at load ("fail loud") — e.g. `validateKeys`-style key lists /
`validateRatioRetention` in compaction-basic, `maxInlineBytes` integer
validation in spill-policy (:117-118).

## Verification (spot-checks performed this pass)

| Claim | Verifying command/output | Verified |
|---|---|---|
| KV-preserving summarizer input | `grep buildSummarizationInput region.ts` → :498-514 uses `requestHeader()` system+tools + region messages | yes |
| Summary-smaller check | region.ts:374-378 throws when framed tokens ≥ shadowed | yes |
| Compaction thresholds 0.8/0.16 | config.ts:20 `DEFAULT_THRESHOLD_RATIO=0.8`, :23 `DEFAULT_RETAIN_RATIO=0.16` | yes |
| Pruner 8192/4096/1024 code points | compaction-tool-result-pruner/src/config.ts:11-13, `codePointLength` :27 | yes |
| Spill dir perms + exclusive create | spill-local/src/store.ts:74 (sha256 slice 12), :109 (0o700), :113 (`open 'wx' 0o600`) | yes |
| Preview budget inside maxInlineBytes | spill-policy/src/index.ts:163 comment + logic; :184 keep-inline fallback | yes |
| Checkpoint flush fail-closed sites | session-checkpoint-policy/src/index.ts :35,:72,:80 `ctx.sessions.flush` | yes |
| Skill metadata cross-product gate | scripts/verify-skill-invocation-metadata.ts:76-90 | yes |
| delegationDepth durability | docs/subsystems/subagent.md:467 | yes |
| Three-rung enumeration ladder | docs/subsystems/subagent.md:289 | yes |
| surfaceOp/replaceGeneration model | docs/subsystems/session.md:205-333 | yes |

Claims transcribed from the prior pass without re-verification are marked
medium confidence in Findings where exactness matters; all carry source paths.

## Findings

- **(fact)** Session is an append-only event log; LLM history derived via `surfaceOp` markers; `replaceGeneration` distinguishes tail growth from rewrites. source=docs/subsystems/session.md:205-333, confidence=high, type=fact
- **(fact)** Compaction is a surface replace with log-only `compaction/*` events; summary cites `sourceEventSeqs`; durable lock = unmatched `compaction/start`; end-seed invalidates pre-seed locks. source=compaction-basic/src/region.ts + docs/subsystems/compaction.md, confidence=high, type=fact
- **(fact)** Three-tier relief: deterministic prune (8192/4096/1024 cp) → pressure compaction (0.8/0.16, retry, smaller-check) → overflow retry gated on `replaceGeneration` advance. source=compaction-* src (verified), docs/subsystems/compaction.md, confidence=high (tiers 1-2), medium (tier-3 exact retry locus read in prior pass only), type=fact
- **(fact)** Summarization input is a genuine conversation prefix (system+tools+shadowed region) → KV-cache hit. source=region.ts:498-514, confidence=high, type=fact
- **(fact)** Token meter anchors to provider usage envelope when it matches, else heuristic; logRevision-stamped immutable snapshots. source=docs/subsystems/token-meter.md, confidence=high, type=fact
- **(fact)** Durability: write-behind + parallel flush + fail-closed boundary flushes + synthetic `turn/end {interrupted}` closure; never truncates. source=write-behind.ts, session-checkpoint-policy/src/index.ts (verified), docs/subsystems/persistence.md, confidence=high, type=fact
- **(fact)** Spill: sha256(sessionId) dir, 0700/0600 exclusive-create anti-symlink; notice bytes reserved inside maxInlineBytes; save failure keeps inline. source=spill-local/src/store.ts, spill-policy/src/index.ts (verified), confidence=high, type=fact
- **(fact)** Subagents: one-shot vs continuable Activations; inbox-only FIFO; last-wins descriptor fold; durable delegationDepth; fork seeds balanced completed-turn prefix; provenance-clean reporting. source=docs/subsystems/subagent.md (fully read), confidence=high, type=fact
- **(fact)** Enumeration three-rung ladder: watermark cache → projection cache (own-suffix seq gate) → persistence inspect; caches are pure accelerators with silent fallback. source=subagent.md:289, confidence=high, type=fact
- **(fact)** Skill invocation policy: cross-product gate (Claude `disable-model-invocation` ⟺ Codex `policy.allow_implicit_invocation`); `/name` gesture only path for model-disabled; catalog via `<system-reminder>` digest-replace. source=scripts/verify-skill-invocation-metadata.ts (verified), tool-skill/src/index.ts, docs/subsystems/skills.md, confidence=high, type=fact
- **(fact)** Goal: revision-CAS GoalRef, full-snapshot durable events, maxGoalRounds → `block(code:'round-limit')`, quiescence-driven rounds with flush+re-check, GoalMessageSource attribution. source=goal-round-driver/src/index.ts:1-260, docs/subsystems/goal.md, confidence=high (mechanism), medium (driver tail 260-445 unread), type=fact
- **(fact)** Jobs: `<kind>-N`, sessionId owner-fencing (authorization not secrecy), first-wins settlement, `reported` dedup, attachController gate, per-owner cap 10. source=docs/subsystems/jobs.md, confidence=high (docs), type=fact
- **(fact)** Schedule: session-local, UTC canonical, catch-up collapses to latest due, queued-not-delivered, idle+maintenance dispatch, at-least-once. source=docs/subsystems/schedule.md, confidence=high (docs), type=fact
- **(fact)** Plan mode: log-only whole-value `plan/mode` event; pending selections admitted at next accepted pre-step; exit via user-questions seam. source=docs/subsystems/plan.md, confidence=high (docs), type=fact
- **(fact)** Config hygiene: unknown-key/mutual-exclusion/ratio-ordering validation at load across config surfaces. source=compaction-basic/src/config.ts, spill-policy/src/index.ts, confidence=high, type=fact
- **(fact)** Cold-resume of a continuable child authorizes the exact live direct parent, folds the descriptor from the child's OWN suffix only (fork seeds may carry an ancestor's descriptor), and reconstructs options solely from the durable descriptor — never re-dispatching a provider, never re-capturing parent policy. source=subagent/src/continuation.ts:248-250,877-920 ⭐, confidence=high, type=fact
- **(fact)** Settlement notification is owned by the continuation manager (not an external `subagent/end` listener) because only it controls residency end; provenance kinds `subagent-report` (relay) vs `subagent-settled` (notice) are deliberately distinct. source=continuation.ts:3-16,60-90 ⭐, confidence=high, type=fact
- **(fact)** Projection cache rows are "possibly stale but never wrong": seq-stamped per-session fold checkpoints, fail-soft writes, `ver` mismatch discards; mandatory writes at `turn/end` and session disposal. source=session-projection-cache/src/index.ts:1-70 ⭐, confidence=high, type=fact
- **(fact)** The schedule's live timer is a disposable per-root-agent projection over the durable fold; due selection is deterministic (one one-shot or one complete fixed-rate batch per wake). source=schedule/src/runtime.ts:1-55 ⭐, confidence=high, type=fact
- **(fact)** Workflow worker-thread settlement is a three-way atomic race (result/death/grace-expiry); cancel arms a `disposeGraceMs` timer that force-settles `cancelled` and terminates the worker; child disposal overlaps the grace; grace timers are unref'd. source=workflow-worker-thread/src/host.ts:1-40,160-215 ⭐, confidence=high, type=fact
- **(inference)** The log-only plugin event discipline (no surfaceOp ⇒ never in model history) is the load-bearing trick that lets compaction locks, descriptors, and goal events ride the SAME durability machinery as messages without polluting the context window. source=synthesis of session.md:11,595 + subagent.md:285, confidence=high, type=inference

## Transferable ideas

(each: idea → evidence path → application to vh-agent-harness)

1. **surfaceOp / replaceGeneration surface model** → `docs/subsystems/session.md`,
   `packages/core/session` → vh-agent-harness's session/compaction story is
   file-based (JSONL sessions + compaction summaries). A derived-surface
   model would let compaction become "replace marker + cited source events"
   instead of a new file generation: auditability (unfold shadowed content),
   overflow retry proof (generation counter), and incremental consumers all
   fall out. Big architectural lift; the *replaceGeneration-as-retry-proof*
   half is transferable standalone.
2. **Durable-progress-as-retry-proof (C3)** → compaction.md tier 3 → vh
   context-overflow recovery could adopt exactly this gate: retry a failed
   LLM call after context relief ONLY when the relief is committed (a flushed
   checkpoint/compaction), not in-memory. Cheap, high-value.
3. **Three-tier relief ordering** → compaction-* → vh compaction currently
   jumps to LLM summarization; a deterministic tier-0 prune (head/tail
   middle-prune of oversized tool outputs, code-point-safe) would remove most
   pressure without model cost. Directly transferable to vh session files.
4. **KV-prefix summarization** → region.ts:498-514 → when vh delegates
   summarization, build the input as a genuine prefix (same system prompt +
   tools + the shadowed region only). Cost lever, easy adopt.
5. **Summary-must-be-smaller assertion** → region.ts:374-378 → cheap guard
   against compaction that makes context *bigger*; vh compaction could
   assert this before accepting a summary checkpoint.
6. **Fail-closed flush boundaries** → session-checkpoint-policy/src/index.ts →
   vh checkpoint discipline maps 1:1 (flush before LLM dispatch / before
   top-level tool exec / at pre-step); "fail-closed BEFORE dispatch" is the
   transferable invariant for vh checkpoint-save cadence rules.
7. **Synthetic turn closure on crash** → persistence.md/write-behind.ts →
   vh session recovery could close interrupted turns with an explicit
   `interrupted` marker rather than leaving dangling state; "never truncate"
   is the invariant worth copying.
8. **Spill with reserved notice budget** → spill-policy/src/index.ts:163 →
   vh tool-output truncation could reserve the truncation-notice bytes inside
   the cap (never exceed the budget with your own bookkeeping), and use
   exclusive-create (wx) + hashed-session dirs for anti-symlink scratch
   files. Relevant to vh `tmp/` and tool-result hygiene.
9. **Descriptor last-wins fold + three-rung enumeration** → subagent.md:283,289 →
   vh subagent delegation could give each delegated run a durable,
   mode-discriminated descriptor in the session/task card, with a
   cache-as-accelerator-only read ladder (any doubt → refold from truth).
   Maps to vh task-card reads and `.local/coordinator/tasks/` listing.
10. **Durable delegationDepth in the session header** → subagent.md:467 →
    recursion budgets for vh subagent delegation should live in the persisted
    session header (monotone floor on resume), not in runtime memory —
    survives vh session resume/compaction.
11. **Provenance-clean child reporting** → subagent.md (reportFrom +
    manager-authored settled notice) → vh specialist reports could separate
    "words the child produced" from "words the manager asserts about the
    child" as distinct provenance kinds — transcripts/closeout reports never
    launder manager claims as child output.
12. **Cross-product skill invocation gate** →
    scripts/verify-skill-invocation-metadata.ts → vh skills render into
    multiple surfaces (opencode + harness docs); a root-level verify script
    keeping invocation-policy metadata aligned across surfaces (and a
    normalized `SkillInvocationPolicy {modelInvocable, userInvocable}`) is a
    cheap governance win. Digest-based full-replacement catalog injection
    also beats incremental merges for vh skill catalogs.
13. **`<kind>-N` job ids + owner-fencing + first-wins settlement** → jobs.md →
    vh background jobs (bgshell) and task cards could adopt per-kind monotone
    ids, sessionId fencing (the authorization-not-secrecy framing is a useful
    principle), and first-wins settlement to make concurrent reports
    idempotent.
14. **Schedule: queued-not-delivered + idle-phase dispatch** → schedule.md →
    any vh future scheduler should enqueue into the session inbox and wait
    for idle+maintenance, never steer mid-turn; catch-up collapse (latest due
    occurrence) avoids storm replay after downtime.
15. **Config fail-loud validation** → compaction-basic/src/config.ts → vh
    profile/run-shape parsing could adopt unknown-key rejection +
    ratio-ordering validation at load across all config surfaces.
16. **Log-only events for non-message state** → session.md:595 → vh session
    records could carry policy/coordination events (locks, descriptors,
    round markers) in the SAME durable stream as messages but excluded from
    the model surface — one durability mechanism, two audiences. The deepest
    architectural idea in the slice.
17. **"Possibly stale but never wrong" cache discipline + mandatory write
    points** → session-projection-cache/src/index.ts ⭐ → vh task-card /
    session-memory caches could adopt the same contract: caches are
    seq-stamped accelerators, fail-soft on write, discarded on version
    mismatch, with NON-tunable mandatory flushes at turn boundaries and at
    live→cold transitions (maps to vh checkpoint-save cadence and
    `.local/coordinator/tasks/` reads).
18. **Three-way settlement race with bounded grace** →
    workflow-worker-thread/src/host.ts ⭐ → any vh bgshell-style background
    job with a worker could adopt: settlement claimed atomically by
    result/death/grace-expiry before teardown callbacks; force path drives
    child disposal immediately (overlapping the grace) rather than waiting on
    a wedged worker; timers unref'd; "the thread never outlives its run."
19. **Timer-as-projection** → schedule/src/runtime.ts ⭐ → if vh ever grows
    scheduling, timers should be disposable projections reconstructed from
    durable records (restart-safe), with deterministic due selection and
    re-arming past platform timer clamps.
20. **Own-suffix-only identity folds** → continuation.ts:897-899 ⭐ → when vh
    resumes a delegated specialist session seeded from a parent (fork/branch
    patterns), identity/authority should fold from the child's OWN suffix
    only — inherited seed content may carry an ancestor's identity and must
    never win over the child's own records.

## Contradictions

- **None detected** between dsh docs and dsh code on the verified claims.
- Note (a correction, not a contradiction): the prior pass's evidence base
  placed the deterministic tool-result pruner under "context relief" without
  pinning its package; it actually lives in
  `packages/compaction/compaction-tool-result-pruner` (verified this pass).
  `packages/context/*` is only injected context providers (instructions,
  time, tmux, session-reference).
- Log growth is unbounded by design (compaction never deletes log events);
  the read material states no retention/trim story. Whether upstream has an
  archival/trim mechanism elsewhere is unverified — flagged in Open
  questions, not silently assumed.

## Coverage

| Path | Status | Reason |
|---|---|---|
| docs/agent-lifecycle.md | examined | full read (prior pass) |
| docs/capability-seams.md | examined | full read (prior pass) |
| docs/subsystems/{compaction,spill,session,persistence,subagent,skills,goal,workflow,jobs,schedule,plan,token-meter}.md | examined | full reads (prior pass) |
| docs/subsystems/session-query.md | partial | first 120/495 lines |
| packages/session — persistence, checkpoint-policy | examined | src complete |
| packages/session — projection, projection-cache | projection-cache: partial ⭐ (header + config + ladder); projection/title/telemetry/stats/backends: skipped | step budget |
| packages/session-query src | skipped | step budget |
| packages/compaction — compaction-basic src (config/index/region), pruner config | examined | complete (pruner re-verified this pass) |
| packages/compaction — command-compact, summarizer.ts tail | partial | summarizer head read, tail unread |
| packages/spill — spill-policy, spill-local (key logic) | examined | complete on cited sites (re-verified this pass) |
| packages/context — 4 subpackages | partial | READMEs + docs only |
| packages/todo src | skipped | README/docs only |
| packages/plan src | skipped | docs fully read |
| packages/goal — goal-round-driver (1-260/445) | partial | mechanism clear, driver tail unread |
| packages/workflow src — worker-thread host | partial ⭐ | header + cancel/settle path read; worker side unread |
| packages/workflow src — rest | skipped | docs fully read |
| packages/subagent src — continuation.ts, descriptor.ts | partial ⭐ | headers + coldResume + provenance kinds read; bulk of 150 files unread |
| packages/skill — tool-skill (1-300/431) | partial | policy + catalog verified; tail unread |
| scripts/verify-skill-invocation-metadata.ts | examined | complete, re-verified this pass |
| packages/jobs src | skipped | docs fully read |
| packages/schedule src — runtime.ts | partial ⭐ | header + dueDecision read; transaction/persistence/tools unread |

Overall: ~60-65% of in-scope code (post-top-ups), ~90% of in-scope docs,
100% of the 13 package groups mapped with READMEs.

## Open questions

1. ~~continuation.ts cold-resume mechanics~~ — **resolved by ⭐ T1**
   (inspect → lineage-authorize exact live parent → own-suffix descriptor
   fold → `agents.resume` reconstruct-from-descriptor; delivery races wait
   for release then cold-resume).
2. ~~session-projection-cache format/seq-gate~~ — **largely resolved by ⭐ T2**
   (per-session `session_projcache` rows, seq-staleness, fail-soft, `ver`
   discard; exact own-suffix seq-gate arithmetic lives in
   session-projection/subagent spec code, still unread).
3. ~~schedule/runtime.ts timer ownership~~ — **resolved by ⭐ T3** (disposable
   per-root-agent timer projection over the durable fold; re-arm beyond Node
   clamp).
4. ~~workflow-worker-thread grace/force-settle~~ — **resolved by ⭐ T4**
   (three-way settlement race; cancel→grace→force-settle+terminate; child
   disposal overlaps grace; thread never outlives its run).
5. **Log retention** — is there any trim/archive story for the append-only
   session log, or is unbounded growth accepted upstream?
6. **session-query** — the query surface's actual API (remaining 375 lines of
   docs + src unread).
7. **Goal round-driver tail (260-445)** — round-close and block handling
   details beyond the mechanism read.
8. **session-projection spec internals** — the projection unit registration
   and restore contract (`session-projection/src`, `spec.ts`) that T2's cache
   serves.

## ⭐ Top-ups (this pass)

### T1. subagent/src/continuation.ts (header :1-95, coldResume :877-920, race sites :222,:486-499) + descriptor.ts (:1-60)

- **Activation = one residency epoch** for a reconstructed child Agent — not a
  request/result/cancellation/Task boundary; it may execute many FIFO turns
  and stays resident while descendants it created still run. Because residency
  is the manager's alone to end, **settlement notification to the parent is
  also the manager's job** (`notifySettlement`): an external `subagent/end`
  listener cannot do it correctly — that payload names no parent, the child
  handle is already disposed, and the release that wakes the parent's
  settlement watcher has already run (continuation.ts:3-16).
- **Provenance kinds in code** (continuation.ts:60-90): `CoordinatorMessageSource`
  (kind `coordinator`, form `relay`), `SubagentReportMessageSource`
  (kind `subagent-report`, form `relay` — "content the child chose"),
  `SubagentSettledMessageSource` (kind `subagent-settled`, form `notice` —
  "the manager stating what became of the child… a transcript that merged
  them would credit the child with words it never wrote"). Confirms the
  packet's provenance-clean reporting finding at source level.
- **coldResume** (:883-920): `persistence.inspect(childId)` → abort-aware →
  `assertAdmitting(parent)` → `authorizeLineage(parent, childId,
  loaded.meta.parentSession)` — *only the durable child's exact live direct
  parent may continue it* → fold the descriptor from ONLY the child's own
  suffix (`events.slice(seedLength ?? 0)`) — a fork seed replays the parent's
  log, which may carry an ANCESTOR's descriptor when the parent is itself a
  continuable child → materialize via `ctx.agents.resume({resumeSessionId,
  provider/model from descriptor, composition {persona, toolFilter}})` →
  submit. Never dispatches through a subagent provider — "the persisted
  Session already holds the initial prefix and the descriptor is the whole
  reconstruction input."
- **Delivery races**: a delivery racing handle teardown waits for release,
  then cold-resumes (:222,:486-499); a parked queue is resumed by the waking
  send (:515). Delegated policy overrides are captured at creation only —
  "a resume never re-captures the parent's policy" (:248-250).
- **descriptor.ts**: `SUBAGENT_DESCRIPTOR_VERSION = 2`, stamped verbatim and
  required by the fold — supporting another composition input is a deliberate
  version change, never an implicit extra field. Declared via `declare
  module '@deepseek-ai/dsh-session/types'` merging `subagent/descriptor` into
  `SessionEventMap` (the merge-extensible event map in action). Omits
  `outputSchema` and per-activation knobs like `maxTokens` ("they budget one
  activation"); cold resume takes the resumed route's defaults rather than
  restoring the prior budget or inheriting the parent's current one.

### T2. session/session-projection-cache/src/index.ts (:1-70)

- Durable checkpoints of every registered projection unit, **one record per
  session** on the `session_projcache` domain (json backend lands it beside
  `workspace.json`).
- **"A fold shortcut, never an authority: a row is possibly stale (its `seq`
  says how stale) but never wrong."** Every durable write is fail-soft — a
  lost write costs a longer tail replay on the next cold read; a `ver`
  mismatch discards the row instead of migrating it. Self-heals on the next
  write or cold read.
- **Throttled write-behind with two MANDATORY write points**: `writeEveryEvents`
  (count) + `writeIntervalMs` (time) are explicit deployment tunables, but
  writes at `turn/end` and at session disposal (the live→cold moment) are
  policy, not tunables, and always fire.
- **Cold-read ladder in code**: cached row → persistence `readFrom` tail →
  registry `restore` → durable write-back. (This is the middle rung the
  subagent enumeration ladder sits on.)
- Design authority: session-projection RFC at
  `.agents/notes/proposed/architecture/2026-07-27-session-projection-and-command-log.md`.

### T3. schedule/src/runtime.ts (:1-55)

- The live timer is a **"disposable live timer projection for one exact root
  agent"** — the durable schedule fold (`foldScheduleEvents`) is the truth;
  the timer is reconstructed state, so restarts lose nothing.
- `MAX_TIMER_DELAY_MS = 2_147_483_647` — Node clamps timers beyond this, so
  long waits need re-arming rather than one giant setTimeout.
- `dueDecision` is deterministic: earliest `scheduledAt` then creation order;
  picks ONE due one-shot, or ONE complete fixed-rate batch (with `acceptedAt`),
  or `{kind:'wait', target}` for the next wake. Dispatch runs through
  `runScheduleTransaction` + `flushSchedulePersistence`.

### T4. workflow/workflow-worker-thread/src/host.ts (:1-40, cancel/settle :160-215)

- **Settlement is a race with exactly three winners**: first worker result,
  unexpected death, or cancellation-grace expiry — claimed atomically
  (`terminalClaimed`) before teardown callbacks, then message admission closes.
- **Cancel path**: worker told (its hooks start throwing; the script dies at
  its next await) → shared child-start signal aborted → grace timer arms: a
  run still unsettled `disposeGraceMs` later **force-settles `cancelled` and
  the worker is TERMINATED**. Idempotent; first reason wins.
- **Settled guard against a bounded-leak footgun**: without it, the ordinary
  "await result, then dispose → cancel" path would arm a grace timer nothing
  ever clears, pinning the run + Worker until grace expiry.
- **Grace timer is `.unref()`d** — an armed grace timer must never hold the
  process open.
- **Force path drives child disposal IMMEDIATELY, overlapping the grace**: a
  wedged worker can relay no dispose RPC, and deferring child teardown to the
  post-terminate reap would spend the whole grace waiting for a quiescence
  that cannot start. Then unconditional terminate — "the thread never
  outlives its run" — and stranded agent starts are paired with synthesized
  ends so child `end`s precede `workflow/end`.
- Worker env is scrubbed (no ambient credentials, no loader flags), with a
  documented Windows `TMP/TEMP`-empty edge case.

---

## Gap-fill pass (2026-08-17)

Follow-up gap-fill pass at the same pinned commit `47f9438`
(47f943859bef60e4160492346772ded9b24f765a, v0.1.0-rc.5). The prose above
is the frozen original record and stays unmodified; this section folds in
the session-cognition gap-fill addendum. Method unchanged: static reading
only, `refs/` never modified, every claim cites a repo-relative path.

### Open-question resolutions

- **OQ 5 (log retention)** — absent upstream BY DESIGN; unbounded growth
  accepted, retention delegated to out-of-band maintenance. Five seams
  state it in contracts: `packages/session/session-persistence/README.md:83`,
  `session-projection-cache/README.md:60`,
  `docs/subsystems/feedback.md:215` (even `session/disposed` does not
  cascade), `attachment.md:72`, `spill.md:41`. The only "truncate"
  machinery is torn-tail crash repair
  (`session-persistence-jsonl/src/index.ts:328,408`); `archiveSession`
  (`docs/subsystems/workspace.md:188-213`) is registry accounting, not
  trimming. Rationale (inference): compaction audit, fork seeding, and
  cold-resume refolds all require the full log.
- **OQ 8 (session-projection spec internals)** — fully read
  (`packages/session/session-projection/src/{index 428, types, invariant}.ts`;
  `session-projection-cache/src/{spec 70, invariant 35}.ts`). Unit
  contract `{key, schema, init, apply, view, stateVersion}`: three PURE
  SYNCHRONOUS functions; `apply` returns the SAME reference when
  uninterested (`!Object.is(next, cell.state)` is the change gate);
  `stateVersion` invalidates rows. Registry: eager drive, refcounted keys,
  differing `stateVersion` refused; `checkpoint()` detaches via
  `structuredClone`; `restoreFloor` anchors one-below the lowest watermark
  (detects a shrunken log); `restore()` seq-gate throws on unusable rows
  with `baseSeq > 0`. Cache spec: whole-value rows, `checkpointIdentity
  {createdAt, cwd}` — "a session id names a slot, not a lifecycle". Both
  invariant companions explained-empty with reasons.
- **OQ 6 (session-query API)** — docs complete (495/495) + core src
  (`session-query/src/{index 359, corpus 309, extraction, documents,
  config, cursor, sources, tracing 248, filters head}.ts`,
  `tool-session-query/src/{service-boundary 179, workspace-access 255}.ts`,
  `session-query-sqlite/src/schema.ts`). ONLY `searchSessions`/
  `searchEvents` abstract; live-preferred logical corpus (a live target
  never consults persistence; optional persistence via child fiber;
  inspect pool capped at 4); `foldSurface` classifies current/shadowed/
  log-only as a first-class filter; closed-switch extraction ("Unknown
  events remain non-searchable"); regex-safe text filters; `traceEvent`
  exposes the compaction audit trail with cycle detection; SQLite read
  model is a disposable derived index (FTS5 + TEMP live tables); model
  tool = cwd-containment authorization with `null`-placeholder lineage,
  fixed safe-error table collapsing internal codes to
  `SESSION_QUERY_TOOL_FAILED`.
- **OQ 7 (goal-round-driver tail)** — file complete (445/445). Attempt
  state driven by inbox/session events; `validReservation` fails closed
  (exact live revision + `source.round === goal.roundsStarted + 1`);
  `agent/pre-step` rejects invalid reservations, restores displaced
  claims, re-validates after `next()`; downstream `reject` blocks the goal
  (`prompt-rejected`); fresh-load hygiene disarms pre-existing states
  ("never inherits hidden automatic authority"); shutdown disarms, marks
  stale, cancels, drains via `Promise.allSettled`.

### New findings

- **NF1** (subagent projection/list-children/index, complete): own-suffix
  seq gate VERIFIED (`list-children.ts:375` — `cached.seq >=
  header.seedLength`; a creation-window checkpoint may carry an ANCESTOR
  descriptor that "must not outrank the re-fold"); last-wins null-sentinel
  identity fold ("a projection fold must never throw"); three-rung ladder
  verified (live watermark → cache row iff gate passes → full refold;
  creation-window children OMITTED, not a diagnostic); `sameLifecycle`
  re-asserts 7 immutable header fields; `COLD_READ_CONCURRENCY = 4`;
  enumeration reads `ctx.get('sessions')` (never the caller-scope proxy);
  iterative cycle-guarded `listDescendants`; named-PROVIDER registry;
  continuable children never become a `SubagentRun`; per-capability
  validation before delegation.
- **NF1a**: every descriptor RESETS timing state (fork-seed ancestor
  turns); negative intervals clamped; `stateVersion: 2`.
- **NF2** (plan-mode index.ts 477/477): log-only last-wins `plan/mode`
  fold ("resume and fork restore it without a live mirror"); pre-step
  gate delegates FIRST ("policy cannot block the step"); `set()` appends
  between turns, queues during; delete-only-after-append ("retryable, not
  dropped"); double-event plan projection (pending is "a pure replay
  quantity"); `exit_plan_mode` registered while inactive; dismissed
  review gets a distinct model message; approval sets a SILENT pending;
  narration gated on `planModeAtLastHeader`.
- **NF3** (jobs, complete): `JobRegistry` ABSTRACT (instantiation throws
  — "Fail loud at load"); registrations outlive fibers; "authorization —
  not secrecy — is the boundary"; settlement FIRST-WINS; completion
  announced LAST (model-cost guard); owner-RELATIVE listener delivery;
  `<kind>-N` ids; `run()` synchronous ("a throw leaves nothing
  registered"); `reported` flag guards teardown model spend.
- **NF4** (tool-todo, complete): `todo_write` = whole-list replacement as
  LOGGED STATE; `allowParallelInProgress` required, no default; duplicates
  rejected by content; `additionalProperties: false` ("the logged snapshot
  must equal what the model believes it wrote"); `todos` projection is a
  per-TURN standing plan (`turn/start` clears to null).
- **NF5** (command-compact, complete — at `packages/compaction/
  command-compact/`, see corrections): human `/compact` adapter over the
  backend-independent seam; one global command ("without a model turn");
  idle-only ("the command itself is not queued"); `command/run`+`done`
  log-only with `sourceEventSeq` naming the `compaction/summary`; `null`
  → "No compactable history yet." (no marker written); closed error union
  → stable outcomes; LIFO drain disposer; explained-empty invariant;
  "The slash input and direct result never enter a model request".
- **NF6** (compaction-basic summarizer, complete): route `configured ??
  latest ?? agentTarget` (conversation's own model); the aux call
  reproduces system/tools/prefix so it is "a genuine prefix of the last
  routed request" (KV reuse); EXACTLY eight Markdown sections, "(none)"
  for empty, never dropped; prior summaries MERGE, never nest; `max-tokens`
  → `MAX_TOKENS` — truncated checkpoints REJECTED; image output refused;
  `frameSummary` = `CHECKPOINT_PREAMBLE` + `<compacted-summary>`
  (inference: eight-section schema independently converges with Claude
  Code's template and this harness's compaction-discipline scaffold).
- **NF7** (session backend READMEs, complete): jsonl — first-append
  materialization ("leaves nothing on disk"), hard-link no-overwrite
  publication, torn-tail recovery with synthetic closers, nothing deletes
  session files; sqlite — contiguous-seq per transaction,
  pristine-or-reject, `locate()` honestly `undefined`, CASCADE for
  out-of-band cleanup only; stats — whole-log unit (steps count `step/end`
  in a `finally`; "Every field is 0 until its first contributing event");
  telemetry — coordinator "stops after it calls `emit()`", WeakMap cursor
  as "a deliberate, narrow exception", first-chunk-only elision ("a gap is
  never a loss signal"); title family — log-only latest-wins, rename PINS,
  ONE provider, shared `session-title-llm` LIBRARY so behavior "cannot
  drift".
- **Surprise checks (all CITED)**: S1 telemetry ships NO default redact
  rules — "records leave the process exactly as captured"
  (`session-telemetry/README.md:23,48`); S2 zstd frames + packed chunk
  rows "~60% smaller" (`session-persistence-jsonl/README.md:18,27,36`);
  S3 stats log-scoped not surface-scoped (`session-stats/README.md:5,38`).

### Transferable ideas added

- Own-suffix seq gate for fork-seeded child caches
  (`subagent/src/list-children.ts:367-377`).
- Serializable `null` sentinel, never `undefined`, for no-value folds
  (`subagent/src/projection.ts`).
- "An id names a slot, not a lifecycle" witness re-check on every re-read
  (`list-children.ts:390-395` + `session-projection-cache/src/spec.ts:30-45`).
- Cache-damage renders no verdict — derived layers fall through to the
  authoritative fold (`list-children.ts:361-366`).

### Coverage after gap-fill

| area | before | after |
|---|---|---|
| docs/subsystems + package READMEs | ~90% | ~97% |
| session-projection src | 0 | 100% |
| session-projection-cache | spec 0 | 100% (package effectively 100% with prior ⭐T2) |
| session-query src | 0 | ~85% |
| goal-round-driver | 58 | 100 (445/445) |
| command-compact | 0 | 100 (at `packages/compaction/command-compact/`) |
| compaction-basic summarizer | partial | 100 (224/224) |
| subagent trio / plan-mode index / jobs / tool-todo | partial | 100 each |

### Contradictions and corrections from this pass

- **command-compact path correction** (cross-round ledger): a prior
  handoff placed it under `packages/session/`; it actually lives at
  `packages/compaction/command-compact/` — grouped with the compaction
  capability, not the session-data group.
- All three surprise checks (S1-S3) verified against source — prior
  statements stood; none dropped. No other discrepancy found.
