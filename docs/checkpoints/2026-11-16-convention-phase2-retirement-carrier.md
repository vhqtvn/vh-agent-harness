# Retirement carrier — convention-phase2 + sparseread disposition (2026-11-16)

Durable carrier for the decision items of two completed local task cards:
`sparseread-borrow-concepts-disposition` and `coverage-convention-phase2-episode`.
This file is the committed carrier the landing-gated retirement check resolves
curated handles against (pattern: 2026-11-15-restudy-resolve-now-retirement-carrier.md).

Underlying evidence: commit `0869f15` (disposition file),
`researches/sources/2026-11-15-sparseread-2608.22237.md` (study + audit),
commit `004a6e22` (landed convention). Episode report (transient, gitignored):
`tmp/agent-runs/coverage-convention-phase2/REPORT.md` — its decision-class
content is curated below; the full report is not promoted.

---

## Card: sparseread-borrow-concepts-disposition

decision-item: sparseread-borrow-concepts-disposition#measured_outcome[fnv1a:e756ec24cecfdf89] (curated)

Curated content: the one-page disposition
`researches/decisions/2026-11-15-sparseread-borrow-concepts-disposition.md`
was written and landed as commit `0869f15` on main (Task-Card trailer present,
exactly 1 file, tree-hash equal, CAS 1). All card success criteria verified;
commit-review approved (high/low, lightweight; 2 non-gating dropped advisories).

---

## Card: coverage-convention-phase2-episode

decision-item: coverage-convention-phase2-episode#measured_outcome[fnv1a:d052a0d98e14c894] (curated)

Curated content — episode verdict (evidence: two-session episode, packet-only
transport, carrier premises re-derived at anomalyco/opencode@e3ed63c, dev HEAD,
2026-11-16): **CONVENTION-ONLY stands** — no skill/command/pack (C/D/E)
proposal, no rollback. Recording overhead ~5% (resume session) / 15–20%
(gathering session) / ~10% episode-amortized. Freshness-aware reuse worked as
designed: all 7 premise 4-tuples reused at the pinned SHA, zero upstream
re-reads, one-fetch freshness check. Known blind spot: nothing prompts a
resuming session to append its read ledger (voluntary in this episode).
Revisit triggers for skill-shape: recurring tuple-authoring defects or
resume-side ledger omissions — then propose the thinnest shape first (a skill).

decision-item: coverage-convention-phase2-episode#open_questions[fnv1a:6273c4315d9b8268] (curated)

Curated content — skill-shape verdict (answered BY the episode):
convention-only. The question "convention-only vs C-through-E vs rollback" is
resolved as convention-only per the measured_outcome item above; n=2 is below
any promotion threshold and observed failures are not tooling-addressable.

decision-item: coverage-convention-phase2-episode#open_questions[fnv1a:79b6b257c335af2a] (curated)

Curated content — durable promotion: effected by THIS carrier file (committed,
branch-reachable) for the decision-class verdicts. The full episode report
stays transient under tmp/ by design; promoting it to researches/ is not
recommended (the carrier + disposition file carry the decision content).

---

## Carrier verdicts for backlog rows (operator decision pending, recorded here as evidence)

Both P2-UPSTREAM premises re-derived 2026-11-16 at anomalyco/opencode@e3ed63c:

- **P2-UPSTREAM-001** (chunkTimeout default PR): PR OBSOLETE — upstream now
  ships `DEFAULT_CHUNK_TIMEOUT_MS = 900_000` (15 min) applied per-route via
  `Provider.timeouts()` in `v1/config/provider.ts`; `false` opts out;
  rationale documented in-code. The row's "only default missing" premise is
  stale.
- **P2-UPSTREAM-002** (task-await timeout PR): PR HAS NO TARGET — the v2
  always-background subagent restructure removed the blocking await;
  `tool/plugin/subagent.ts` returns `backgroundResult(sessionID)` immediately;
  no `job.wait` call site remains to pass a timeout into. (The `Job.wait`
  timeout param itself survives in `packages/core/src/job.ts` via
  `Effect.timeoutOption`.)

Recommendation to operator: cancel both rows with the above as reason
(operator-promoted rows — coordinator does not cancel unilaterally).
