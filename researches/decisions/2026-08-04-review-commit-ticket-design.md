# Decision — review-commit ticket: serialize contention at the review boundary

**Date:** 2026-08-04
**Status:** decided (design bound to harness surfaces; reference implementation
consumer-side, not in this repo)
**Origin:** canon-bound decisions absent from this repo; re-derived 2026-08-04
from harness-side evidence (binding-regression unification audit surfaced the
absence)
**Run artifacts:** none (this is the re-derivation itself; no session run artifacts)

## Context

The sibling memo `researches/decisions/2026-08-04-s2-canonical-tree-integrity-relaxation.md`
establishes that S2 (the commit-time integrity guard at
`commit-gate.sh:1130-1147`) must stay byte-identical: its measured serialization
cost (~19-20% windowed refusal rate) is the price of not silently substituting a
reviewer's tree. That leaves a throughput question: how does the refusal rate go
*down* without weakening S2? This memo owns the answer — serialize contention at
the **review boundary** instead of at the commit-time guard.

This file was cited as canon by
`researches/decisions/2026-08-04-binding-regression-unification-audit.md`
(line 118 names the dispatch-side resolution owned by the S2 memo, which in turn
defers the mechanism to this sibling). It was absent from this repo until created
here.

### Scope of what this repo can verify (read this before the mechanism)

The **serialization site** and the **reused liveness primitive** are
harness-owned and verifiable in this repo's tree:

- The review phase is lock-free, so it is the natural contention point:
  `commit-gate.sh:874` ("Release lock immediately — review (Phase 2) is
  lock-free") and `commit-gate.sh:1013` ("The lock is NOT held during commit —
  acquire releases it for lock-free review").
- The heartbeat is the existing liveness primitive that survives the lock-free
  window: `cmd_heartbeat` at `commit-gate.sh:1407`; the `heartbeat_at` field at
  `:868` and `:973`; age via `_heartbeat_age_seconds` at `:170`.
- S2 stays byte-identical: `commit-gate.sh:1130-1147`, refuse condition at `:1140`.

The **review-commit ticket/queue itself is NOT present in this repo's shipped
corpus.** A grep of `templates/core` for
`review-commit|review_commit|reviewqueue|review-ticket|reviewticket` returns no
matches (verified by grep scoped to `templates/core`). From this repo's vantage,
the queue is a **resolved direction** whose reference implementation is
consumer-side, not a harness-shipped mechanism. Accordingly, the queue's
internals are **not described here as if verifiable** — any claim that depends on
the queue's internal behavior is marked **[UNVERIFIED]** below.

## Decision

Adopt the dispatch-side resolution: **serialize concurrent reviews at the
lock-free review boundary** so that reviews queue rather than concurrent commits
refusing at S2. Build the dispatch path on the surfaces this repo *does* ship:

- **Serialization site:** the review phase, which is already lock-free
  (`commit-gate.sh:874`, `:1013`). Move contention serialization *out* of the
  commit-time integrity guard (S2) and *into* the review phase.
- **Liveness primitive:** reuse the **existing heartbeat** (`cmd_heartbeat` at
  `:1407`; `heartbeat_at` field at `:868`, `:973`) as the "still alive / still
  reviewing" signal for a queued review, rather than introducing a new lease or
  lock extension.
- **Integrity guard:** S2 stays byte-identical (`commit-gate.sh:1130-1147`,
  `:1140`). The dispatch path reduces how often S2 fires by reducing concurrent
  commits; it does not change what S2 does when it fires.

### What is NOT proven (explicit limitation — mandatory)

**[UNVERIFIED]** The queue/ticket implementation is consumer-side; its internals
are not present in `templates/core` and are not reproduced or verified in this
memo. Specifically:

- The **queue's ordering, admission, and fairness policies** are not described
  here because they cannot be substantiated from this repo's tree. Any such
  description would be a claim about consumer-side code this memo does not and
  must not read.
- **[UNVERIFIED]** The **voided-mid-review recovery path** is unit-covered only
  and integration-deferred. It MUST NOT be described as validated under live
  concurrency. The mid-review case (a review whose lane is voided while it holds
  a queued slot) has unit coverage but no integration evidence in this repo;
  treating it as concurrency-validated would be exactly the
  claim-outrunning-evidence failure this pair of memos exists to discipline.

### Integration evidence (consumer-side, not reproduced here)

**[UNVERIFIED in this repo]** There is consumer-side integration evidence for
the dispatch-side resolution: a two-arm test in which the queue was bypassed
produced refusals and a double-review, while the queue engaged produced none,
with contention structurally guaranteed rather than timing-dependent. This is
consumer-side integration evidence (not reproduced or independently verified in
this repo's tree); it is cited as corroboration only. The consumer is not named,
and the test mechanism is not described beyond "a two-arm test with contention
structurally guaranteed."

## Rationale

The dispatch-side resolution is the throughput complement to S2, not a relaxation
of it. S2 refuses concurrent commits because it cannot tell a safe disjoint
merge from an unsafe one (see the S2 memo's asymmetry argument). But the *cause*
of concurrent commits is concurrent reviews landing into the same commit window.
If reviews serialize at the review boundary, the commit phase sees far fewer
concurrent contenders, and S2 fires far less often — without S2 ever having to
make a safety judgment it is structurally unable to make. Reusing the heartbeat
(rather than a new lease) keeps the liveness model single-primitive and avoids
introducing a second clock/ownership surface that would itself need an integrity
guard.

## Alternatives considered (rejected)

- **Relax S2 on file-disjoint concurrent commits.** Rejected per the S2 memo's
  asymmetry argument and the origin memo's rejection of the disjoint-auto-merge
  hybrid: no safe semantic-conflict classifier exists. (See
  `2026-08-04-s2-canonical-tree-integrity-relaxation.md`.)
- **Add a new lease / lock extension for the review phase.** Rejected in favor of
  reusing the existing heartbeat (`commit-gate.sh:1407`, `:868`, `:973`). A
  second liveness primitive would duplicate the ownership/TTL surface the
  heartbeat already covers and would itself become an integrity surface needing a
  guard.
- **Auto RE-VIEW the merged tree at commit time.** Rejected, livelock risk; this
  is the path parked as deferred in `2026-07-27-commit-scope-integrity.md`. Not
  reopened here.

## Consequences

- **S2 is unchanged** (`commit-gate.sh:1130-1147`). This memo does not touch the
  integrity guard; it owns the dispatch-side serialization that reduces how often
  the guard fires.
- **The reference implementation is consumer-side.** From this repo's vantage the
  queue/ticket is a resolved direction, not a shipped mechanism; a reader looking
  for the queue's internals will not find them in `templates/core` and must look
  to the consumer repo.
- **The voided-mid-review recovery path remains integration-deferred.** It must
  not be claimed as concurrency-validated until integration evidence is produced
  in this repo (which this memo does not do).
- No code change. This memo binds a decision that was already canon in a
  downstream consumer but absent from this repo.

## References

- Lock-free review boundary (serialization site): `commit-gate.sh:874`, `:1013`.
- Heartbeat (reused liveness primitive): `:1407`, `:868`, `:973`, `:170`.
- S2 (unchanged integrity guard): `commit-gate.sh:1130-1147`, `:1140`,
  `:1062` (reviewed tree captured), `:1074-1100` (CAS merge), `:1117`
  (write-tree).
- Sibling memo (do not relax S2):
  `researches/decisions/2026-08-04-s2-canonical-tree-integrity-relaxation.md`.
- Origin memo (S2 = fail-closed; rejected disjoint-auto-merge; RE-VIEW deferred):
  `researches/decisions/2026-07-27-commit-scope-integrity.md`.
- Citing memo: `researches/decisions/2026-08-04-binding-regression-unification-audit.md`.
