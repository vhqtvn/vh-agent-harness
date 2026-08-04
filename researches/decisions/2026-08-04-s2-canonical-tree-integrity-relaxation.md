# Decision — S2 canonical-tree integrity: do not relax under measured serialization cost

**Date:** 2026-08-04
**Status:** decided
**Origin:** canon-bound decisions absent from this repo; re-derived 2026-08-04 from
harness-side evidence (binding-regression unification audit surfaced the absence)
**Run artifacts:** none (this is the re-derivation itself; no session run artifacts)

## Context

The commit gate's commit-time concurrency guard — labeled **S2** in the origin
memo `researches/decisions/2026-07-27-commit-scope-integrity.md` — refuses any
commit whose tree would differ from the tree the reviewer approved. S2 is a
harness-owned surface: it lives in `templates/core/.opencode/scripts/commit-gate.sh`
(re-verified 82622 bytes, 1999 lines at HEAD). Its serialization cost (refusal
rate) is now measured from the rolling closeout ledger. The question this memo
closes is whether to **relax S2** now that the cost is known.

This file was cited as canon by
`researches/decisions/2026-08-04-binding-regression-unification-audit.md`
(line 47 and line 118 of that memo) but was absent from this repo until created
here. The binding-regression memo is left byte-identical; creating this file at
the cited path makes those citations valid.

### What S2 does (bound to the code)

S2 is "approved-tree integrity under concurrency," defined in a comment block at
`commit-gate.sh:1130-1147`. The mechanism:

- During the commit phase, the gate reads the current HEAD and compares it to the
  head recorded at acquire. If they differ (`current_head != expected_head`,
  `commit-gate.sh:1074`), a concurrent commit landed between acquire and commit,
  so the gate performs a 3-way merge using git objects only:
  `git read-tree -m -i "$base_tree" "$new_head_tree" "$tree_hash"`
  (`commit-gate.sh:1100`), where base = original HEAD at acquire, theirs = new
  HEAD (the concurrent winner), and ours = the reviewed tree. The merged result
  is written with `new_tree=$(git write-tree)` (`commit-gate.sh:1117`).
- `original_tree` is the reviewed tree, captured before the merge at
  `commit-gate.sh:1062` (`local original_tree="$tree_hash"`).
- **The refuse condition is whole-tree identity, not file overlap**
  (`commit-gate.sh:1140`): `if [[ "$new_tree" != "$original_tree" ]]; then` emits
  `status: rebased_refused`, `reason: reviewed_tree_diverged`, and returns 1.
- **Load-bearing consequence of whole-tree comparison:** a file-disjoint
  concurrent commit produces a clean *union* tree that still differs from the
  reviewed tree → it is still refused. There is no file-disjoint shortcut in S2.
  This follows directly from the comparison being two full tree hashes
  (`new_tree != original_tree`), not a per-path overlap check.

The review phase that precedes commit is **lock-free**: the lock is released
immediately after acquire so review runs unlocked
(`commit-gate.sh:874` "Release lock immediately — review (Phase 2) is lock-free";
`commit-gate.sh:1013` "The lock is NOT held during commit — acquire releases it
for lock-free review"). Liveness during that lock-free window is carried by the
existing heartbeat (`cmd_heartbeat` at `commit-gate.sh:1407`; the `heartbeat_at`
field at `:868` and `:973`; age via `_heartbeat_age_seconds` at `:170`).

S2's origin as a fail-closed decision is `researches/decisions/2026-07-27-commit-scope-integrity.md`,
which established the invariant *"committed_tree == reviewed_tree, OR refusal
triggered"* and explicitly REJECTED the "Disjoint-auto-merge hybrid (no safe
semantic-conflict classifier)" alternative. RE-VIEW-with-bounded-retry was parked
there as deferred.

### Measured serialization cost (windowed — name the caveat)

The closeout ledger `.git/commit-gate/closeouts.log` is a **200-entry rolling
window at its cap** (verified: `wc -l` = 200). All rates derived from it are
**windowed**, not cumulative.

Current tally (extraction method:
`grep -o '"status":"[a-z_]*"' .git/commit-gate/closeouts.log | sort | uniq -c`):

- `162` entries with `"status":"committed"`
- `38` entries with `"status":"rebased_refused"`
- → current refusal rate **38/200 = 19.0%**, windowed.

The binding-regression unification audit recorded a prior windowed figure of
**40/200 = 20.0%** (`2026-08-04-binding-regression-unification-audit.md` line 47
and line 68). Both figures are windowed; the 2-entry difference between them is
consistent with ordinary window roll, not a contradiction.

### Orphaned ledger entries (a release-recovery artifact, stated for completeness)

Four orphaned entries exist — closeout records whose `post_commit_head` is no
longer reachable from the branch (extraction: `grep -c 060ab60
.git/commit-gate/closeouts.log` = 3; `grep -c d843ef9
.git/commit-gate/closeouts.log` = 1). Per the binding-regression memo's Addendum
(lines 136-149), these are a release-recovery artifact: the operator `git reset`
escape hatch reverted `d843ef9` (the audit memo) and cherry-picked it back as
`ccc95f8`; `060ab60` is likewise a release-recovery artifact. This is **not a
data error**, and the orphaned entries self-erase on the rolling window. These
entries are not relevant to this decision and are recorded only so a reader does
not mistake them for a new finding.

## Decision

**Do not relax S2.** Keep `commit-gate.sh:1130-1147` byte-identical, including
the whole-tree refuse condition at `:1140`. The serialization cost that S2
imposes (the ~19-20% windowed refusal rate) is addressed by moving contention
serialization to the **dispatch side at the review boundary** — see the sibling
memo `researches/decisions/2026-08-04-review-commit-ticket-design.md` — not by
weakening the commit-time integrity guard.

## Rationale (the asymmetry is load-bearing)

The argument is an **asymmetry of failure modes**, not a cost-benefit
optimization:

- A **refusal fails loudly.** The lane observes `rebased_refused`, re-acquires,
  re-reviews, and self-recovers through the existing `could_not_land`-style
  recovery path. The cost is latency and retry work, both observable and bounded
  by the operator's willingness to retry.
- A **wrongly relaxed integrity check fails silently and permanently.** If S2
  were softened to auto-merge a file-disjoint concurrent commit into the reviewed
  tree, the committed tree would no longer equal the reviewed tree — a tree the
  reviewer never saw would be committed, with no signal that it happened. There
  is no after-the-fact recovery from a silently substituted tree.

Because the expensive-but-loud failure is recoverable and the cheap-but-silent
failure is not, the correct trade is to **keep the loud failure and move the
contention that causes it elsewhere** (the dispatch side), rather than to silence
it at the integrity guard. This is the same reasoning that led the origin memo to
reject the disjoint-auto-merge hybrid: there is no safe semantic-conflict
classifier that could distinguish "a disjoint concurrent commit that is safe to
merge" from "a disjoint concurrent commit that changes the meaning of the
reviewed scope." Whole-tree identity is the only check that cannot be fooled by a
missing classifier.

## Integration evidence (consumer-side, not reproduced here)

**[UNVERIFIED in this repo]** There is consumer-side integration evidence for the
dispatch-side resolution that lowers S2's refusal rate: a two-arm test in which
the queue was bypassed produced refusals and a double-review, while the queue
engaged produced none, with contention structurally guaranteed rather than
timing-dependent. This evidence is consumer-side integration evidence (not
reproduced or independently verified in this repo's tree); it is cited here as
corroboration only. The consumer is not named, and the test mechanism is not
described beyond "a two-arm test with contention structurally guaranteed."

## Alternatives considered (rejected)

- **Relax S2 on file-disjoint concurrent commits.** Rejected per the asymmetry
  argument above and per the origin memo's rejection of the disjoint-auto-merge
  hybrid: no safe semantic-conflict classifier exists, so a file-disjoint
  shortcut would reintroduce the silent integrity hole S2 exists to close.
- **Auto RE-VIEW the merged tree at commit time.** Rejected, livelock risk; this
  is the path parked as deferred in `2026-07-27-commit-scope-integrity.md`. Not
  reopened here.
- **Accept a relaxed guard and measure.** Rejected: this is exactly the
  silent-permanent failure mode, and "measure later" does not recover a tree
  already silently substituted.

## Consequences

- **S2 is unchanged.** `commit-gate.sh:1130-1147` stays byte-identical; the
  refuse condition at `:1140` continues to compare whole-tree identity.
- **The throughput answer is the dispatch-side resolution**, owned by the sibling
  memo `2026-08-04-review-commit-ticket-design.md`, which serializes contention
  at the review boundary so concurrent reviews queue instead of concurrent
  commits refusing.
- **RE-VIEW / auto-retry remains deferred** per `2026-07-27-commit-scope-integrity.md`;
  this memo does not revive it.
- The orphaned-ledger entries (four, per the tally above) are a release-recovery
  artifact and are not affected by this decision; they self-erase on the rolling
  window.
- No code change. This memo binds a decision that was already canon in a
  downstream consumer but absent from this repo.

## References

- S2 mechanism: `templates/core/.opencode/scripts/commit-gate.sh:1062` (reviewed
  tree captured), `:1074-1100` (CAS 3-way merge), `:1117` (write-tree),
  `:1130-1147` (S2 comment + refuse), `:1140` (whole-tree refuse condition).
- Lock-free review: `commit-gate.sh:874`, `:1013`. Heartbeat liveness:
  `:1407`, `:868`, `:973`, `:170`.
- Ledger: `.git/commit-gate/closeouts.log` (200-entry rolling window).
- Origin memo: `researches/decisions/2026-07-27-commit-scope-integrity.md`
  (S2 = fail-closed; rejected disjoint-auto-merge hybrid; RE-VIEW deferred).
- Citing memo: `researches/decisions/2026-08-04-binding-regression-unification-audit.md`
  (line 47, line 118).
- Sibling memo (dispatch-side resolution):
  `researches/decisions/2026-08-04-review-commit-ticket-design.md`.
