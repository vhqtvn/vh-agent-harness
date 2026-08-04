# Dispatch discipline — serialize contention at the review boundary

> When concurrent work contends for the commit lane, serialize it at the
> **review boundary**, never by relaxing the commit-time integrity guard (S2).
> Prefer the loud refusal that self-recovers over the silent substitution that
> does not. This is the dispatch-side rule that keeps S2's measured refusal rate
> low without weakening S2.

This is durable dispatch guidance for anyone designing or changing the path that
hands reviewed work to the commit gate. It is bound to harness-owned surfaces
that are verifiable in this repo's tree; anything that depends on consumer-side
implementation is out of scope here and is marked as such.

## The principle

Serialize contention **at the review boundary**, not at the commit-time integrity
guard. The review phase is already the natural contention point because it is
**lock-free**: the gate releases the lock immediately after acquire so review
runs unlocked (`templates/core/.opencode/scripts/commit-gate.sh:874` — "Release
lock immediately — review (Phase 2) is lock-free"; `commit-gate.sh:1013` — "The
lock is NOT held during commit — acquire releases it for lock-free review").
Building the dispatch path so concurrent reviews *queue* at that boundary is what
keeps concurrent commits from piling up at commit time and tripping the guard.

The liveness signal for a queued review is the **existing heartbeat** — do not
introduce a new lease or lock extension. The heartbeat is the single liveness
primitive the gate already ships: `cmd_heartbeat` at `commit-gate.sh:1407`; the
`heartbeat_at` field at `:868` and `:973`; age via `_heartbeat_age_seconds` at
`:170`. A second liveness primitive would duplicate the ownership/TTL surface the
heartbeat already owns and would itself become an integrity surface needing a
guard.

## Keep S2 byte-identical (the integrity guard)

S2 — "approved-tree integrity under concurrency" — is the commit-time guard at
`commit-gate.sh:1130-1147`. Its refuse condition is at `:1140`:
`if [[ "$new_tree" != "$original_tree" ]]; then` emits `status: rebased_refused`,
`reason: reviewed_tree_diverged`, and returns 1. **Do not relax S2 to reduce the
refusal rate.** The refusal rate goes down by serializing at the dispatch side,
not by weakening the comparison at `:1140`.

The refuse condition is **whole-tree identity**, not file overlap (the comparison
is two full tree hashes: `new_tree != original_tree`). A file-disjoint concurrent
commit produces a clean *union* tree that still differs from the reviewed tree
and is still refused. **Never add a file-disjoint shortcut to S2.** There is no
safe semantic-conflict classifier that could tell a safe disjoint merge from an
unsafe one; whole-tree identity is the only check that cannot be fooled by a
missing classifier.

## The asymmetry rule (imperative)

Prefer the **loud refusal** over the **silent relaxation**:

- A `rebased_refused` at `:1140` fails loudly. The lane observes it, re-acquires,
  re-reviews, and self-recovers. The cost is latency and retry work — observable,
  bounded, recoverable.
- A relaxed guard would fail **silently and permanently**. If S2 auto-merged a
  concurrent commit into the reviewed tree, the committed tree would no longer
  equal the reviewed tree, with no signal that it happened. There is no
  after-the-fact recovery from a silently substituted tree.

So: **never auto-merge a concurrent commit into the reviewed tree.** Keep the
expensive-but-loud failure; move the contention that causes it to the dispatch
side.

## What this repo can and cannot claim

Bound to harness-owned surfaces only:

- **Verifiable here:** the serialization site (the lock-free review boundary,
  `commit-gate.sh:874`, `:1013`), the reused liveness primitive (the heartbeat,
  `:1407`), and the integrity guard that must stay byte-identical (S2,
  `:1130-1147`, `:1140`).
- **Not verifiable here:** the review-commit ticket/queue implementation. A grep
  of `templates/core` for `review-commit|review_commit|reviewqueue|review-ticket|reviewticket`
  returns no matches. The queue is a resolved direction whose reference
  implementation is consumer-side, not a harness-shipped mechanism.

Any dispatch-queue behavior that depends on consumer-side implementation is **out
of scope for this repo's canon**. Do not assert queue internals — ordering,
admission, fairness policies, or the voided-mid-review recovery path — as if
verifiable here. Mark such claims **[UNVERIFIED]** or omit them. The
voided-mid-review recovery path in particular is unit-covered only and
integration-deferred; do not describe it as validated under live concurrency.

## Decision basis

- `researches/decisions/2026-08-04-s2-canonical-tree-integrity-relaxation.md` —
  the decision to keep S2 byte-identical and move serialization to the dispatch
  side; states the measured windowed refusal rate and the asymmetry argument.
- `researches/decisions/2026-08-04-review-commit-ticket-design.md` — the
  dispatch-side mechanism (serialize at the review boundary, reuse the
  heartbeat), with the explicit limitation that the queue/ticket implementation
  is consumer-side and the voided-mid-review recovery is integration-deferred.
- `researches/decisions/2026-07-27-commit-scope-integrity.md` — S2's origin as a
  fail-closed decision; the rejected disjoint-auto-merge hybrid; RE-VIEW-with-
  bounded-retry parked as deferred.

## Cross-references

- S2 mechanism in full: `commit-gate.sh:1062` (reviewed tree captured),
  `:1074-1100` (CAS 3-way merge), `:1117` (write-tree), `:1130-1147` (S2 comment
  + refuse), `:1140` (whole-tree refuse condition).
- The closeout ledger that measures S2's refusal rate is a 200-entry rolling
  window at `.git/commit-gate/closeouts.log`; see the S2 memo for the current
  windowed figure and its extraction method.
