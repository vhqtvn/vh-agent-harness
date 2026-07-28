# Decision: File-scope-lease dispatcher — recurrence-trigger evaluation (NOT MET; no promotion)

**Date:** 2026-07-28
**Status:** Accepted (record-of-decision). The parked DEFER `defer-file-scope-lease-dispatcher` recurrence trigger was evaluated against durable evidence and is NOT met. No promotion; no build; no backlog card. The DEFER stays parked.
**Supersedes:** none. **Narrows** (does not supersede) the 2026-07-23 disposition §4.3/§6.
**See also:**
[`./2026-07-23-vh-solara-orchestration-field-report-disposition.md`](./2026-07-23-vh-solara-orchestration-field-report-disposition.md) §4.3 (P1-A file-scope-lease DEFER) + §6 (named trigger).
[`./2026-07-05-commit-gate-shared-file-coupling.md`](./2026-07-05-commit-gate-shared-file-coupling.md) (W1/W4/G1 decided position).
The 2026-07-26 addendum (P0-B shipped trio) + 2026-07-26b (could_not_land live+tested) on the 2026-07-23 disposition.
Commit `4bc821e` / memo `d87e6f7` (S2 refuse-on-rebase, post-disposition).

## Framing

An operator invoked the recurrence-escalation trigger for the parked
`defer-file-scope-lease-dispatcher` card, citing this session's commit-gate churn
as evidence that Pattern 4 (concurrent-lane same-file tangle) has recurred. The
task: establish from durable evidence whether the trigger is genuinely met; if
not, do not promote on a soft signal.

The trigger (defer card `.local/coordinator/tasks/defer-file-scope-lease-dispatcher.json`
+ 2026-07-23 disposition §6):

> ≥1 more documented concurrent-same-CODE-file tangle, OR G1 line-level merge validated.

## 1. Evidence-derived recurrence count (from the durable closeout ledger)

Source of truth: `.git/commit-gate/closeouts.log` (48 records, 2026-07-26 →
2026-07-28). Derived directly, not asserted:

| Status | Count | Meaning |
|--------|-------|---------|
| `committed` | 42 | successful landings |
| `rebased_refused` | 6 | S2 approved-tree-integrity refusal (see §2) |
| `could_not_land` | 0 | genuine content-tangle merge failure (merge_failed/write_tree_failed) |
| `cas_conflict` | 0 | (terminal; the retry loop's transient cas_conflict is internal) |

The 6 `rebased_refused` events cluster on 2026-07-28 (08:05–10:25), each a
concurrent-dispatch pair: two sessions acquired the same head, one committed,
the other refused. The many "Re-commit … after CAS divergence" operator
sessions are the candidate symptom; this evaluation **refutes** that they are
Pattern-4 same-file tangles (see §2).

## 2. rebased_refused ≠ same-file content tangle (decisive)

Read from `templates/core/.opencode/scripts/commit-gate.sh:1004-1097`: when HEAD
moved between acquire and commit, the gate 3-way-merges (base = acquire-tree,
theirs = current-head, ours = reviewed-tree). Three outcomes:

- merge FAILS on conflicting hunks → `could_not_land` (reason merge_failed /
  write_tree_failed). **This is the genuine same-file content-tangle signal.**
- merge SUCCEEDS but the merged tree differs from the reviewed tree →
  `rebased_refused` (S2, commit `4bc821e`, post-disposition). Committing would
  substitute a tree the reviewer never saw; the gate refuses and requires
  re-acquire + re-review.
- merge SUCCEEDS and reproduces the reviewed tree exactly → land.

Decisive: a clean merge of **disjoint** files still produces new_tree ≠
original_tree (the winner's disjoint changes are fused in), so `rebased_refused`
fires for **any** concurrent commit — including entirely disjoint-file sessions.
It does **not** discriminate same-file overlap.

`could_not_land` is the status that signals a genuine same-file content tangle,
and it is live + tested (2026-07-26b addendum;
`TestCommitGate_CloseoutLedger/could_not_land_on_merge_conflict`). Its count is
**ZERO**.

⇒ The durable ledger contains **zero** same-file content tangles. The 6
collisions are concurrent-dispatch refusals by a gate WORKING AS DESIGNED
(authority line: the gate ACTS; the coordinator reads).

## 3. Verdict: trigger NOT met

- "concurrent-same-CODE-file tangle" requires a content merge failure
  (`could_not_land`). Count = 0.
- The observed churn (`rebased_refused` → re-dispatch) is concurrent-dispatch
  rework, refused cleanly and safely by S2. It is a **different phenomenon**
  from the named trigger.
- The winning commits in the collision window were predominantly DOCS
  (researches/, docs/checkpoints/); a few touched config (review-tiers.json,
  complexity-policy.yml, repo-mail-egress-gate.js). None produced a
  content-tangle failure.
- Promoting on this evidence would rename "concurrent-dispatch rework" as
  "same-file tangle" — a soft signal. Per the authority line and the evaluation
  constraint, do not promote.

## 4. Reconciliation (no position re-opened)

- **P1-A W4 (REJECTED) stays rejected.** Not re-opened. The lease was the
  DEFERRED alternative; this evaluation only checks its trigger, not W4.
- **G1 (DEFERRED) stays deferred.** No line-level-merge validation packet landed.
- **P0-B near-term mitigation is now LIVE (post-disposition).** The 2026-07-23
  disposition §4.2/§4.3 predicted the tangle symptom would be "cheaply mitigated
  by P0-B." The P0-B trio shipped (`a4afe6e8` ledger + `6f524bd` vocab +
  `5495a9e` doctor #19 head-progress WARN). The churn the operator observes is
  exactly the cross-closeout sequence signal doctor #19 was built to surface.
- **S2 refuse-on-rebase (`4bc821e`) is a NEW post-disposition safety gate.** It
  converts what would have been a silent approved-tree-substitution into a clean
  refusal + re-acquire. The `rebased_refused` churn is S2 WORKING, not a gap.
- Net: the symptom space the lease was meant to address is now actively managed
  by S2 + P0-B. The lease (a coordination ROUTING function, never blocking
  commits) remains a valid long-term option IF a genuine same-CODE-file content
  tangle recurs (`could_not_land` > 0) or G1 lands — neither has occurred.

## 5. What WOULD constitute a genuine trigger (for future evaluation)

A future promotion is justified ONLY when one of these is documented from
durable evidence:

1. `could_not_land` (merge_failed / write_tree_failed) count > 0 on a CODE file
   — a genuine content-tangle merge failure the gate could not reconcile; OR
2. G1 line-level-merge validation packet lands (the orthogonal gate-mechanical
   alternative); OR
3. `rebased_refused` churn on CODE files reaches a cost threshold that the
   ROUTING lease (not a state gate) would specifically avert — AND S2/P0-B prove
   insufficient. (This third path requires evidence that the refusals are
   same-file-overlap-driven, which `rebased_refused` alone cannot establish; the
   closeout ledger would need to record refused-session intended paths to make
   this mechanizable.)

Note on the trigger grammar: the card's prose trigger ("≥1 documented
concurrent-same-CODE-file tangle") is NOT expressible in the current
`check-defer-triggers.js` predicate grammar (`path_touched` / `after_tag` only).
It is a human-judgment trigger, evaluated by reading the durable ledger. This is
acceptable for a parked DEFER; mechanizing it would require extending the grammar
to an event/count predicate, out of scope here.

## Authority line

- This evaluation INFORMS (the coordinator reads the ledger + gate semantics);
  no coordinator transition authority is invoked.
- The gate (`could_not_land` / `rebased_refused` / S2) ACTS. Consistent with the
  2026-07-23 disposition §5 and the 2026-07-22 authority line.

## Evidence / Provenance

| Claim | Verifying artifact / command | Verified |
|-------|------------------------------|----------|
| 6 rebased_refused, 0 could_not_land, 0 cas_conflict | `grep -o '"status":"[a-z_]*"' .git/commit-gate/closeouts.log \| sort \| uniq -c` | yes |
| rebased_refused fires on any concurrent commit (incl. disjoint) | `templates/core/.opencode/scripts/commit-gate.sh:1004-1097` | yes |
| could_not_land = content-tangle signal, live+tested | 2026-07-26b addendum; gate.sh:1042-1071; `TestCommitGate_CloseoutLedger/could_not_land_on_merge_conflict` | yes |
| P0-B trio shipped | commits `a4afe6e8`/`6f524bd`/`5495a9e`; closeout ledger exists | yes |
| S2 refuse-on-rebase, post-disposition | commit `4bc821e`; memo `d87e6f7` | yes |
| W4 REJECTED, G1 DEFERRED | 2026-07-05 memo §Options; not re-opened | yes |
| trigger grammar = path_touched/after_tag only | `.opencode/scripts/check-defer-triggers.js:58-66` | yes |
| winning commits in collision window mostly docs/config | `git show --stat 03ca41d ec34c6b 5f3fb17 08920d5 6618a11 b41cc28` | yes |

## Status

Narrows the 2026-07-23 disposition §4.3/§6. The DEFER
`defer-file-scope-lease-dispatcher` remains parked (status `draft`). No
promotion; no backlog card; no build. Reactivate only on a genuine same-CODE-file
content-tangle signal (`could_not_land` > 0) or G1 validation.
