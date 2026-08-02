# 2026-08-02 DEFER audit + replan — closeout

## Mission
Close out the full DEFER transport audit → restudy → replan → execution arc initiated 2026-08-02. This checkpoint records the final state after the solution-brief replan was accepted ("ok all defaults") and executed. It supersedes the open-follow-ups in the earlier cap-time checkpoint `docs/checkpoints/2026-08-02-defer-audit.md` (commit `21559a5`), whose "3 promote-ready candidates" are now resolved/refuted.

## Headline
The DEFER audit/replan effort is **fully closed**. Transport cleaned from 79 → 46 cards. Backlog gained 8 actionable rows. One candidate was honestly **refuted** (its gap already closed by an existing gate) rather than rubber-stamped. The live substantive frontier — the **lever-E seat-routing study** — is now a schedulable backlog row (P2-EVAL-001) with a two-layer acceptance design.

## Population delta (full arc)
| Phase | Action | Delta | Running |
|---|---|---|---|
| Start | — | — | 79 |
| Audit r1 | retire 18 (closed/shipped polluting transport) | −18 | 61 |
| Promote r1 | quarantine cluster + defer-011 (source cards retired) | −4 | 57 |
| Retire r2 | orphan fixture + 5 stubs | −6 | 51 |
| Promote D | f1-pa-producer-dedup (source retired) | −1 | 50 |
| Capture B | verify-task-registry leak-fix draft added | +1 | 51 |
| Promote r3 | 3 candidates (source retired) | −3 | 48 |
| Replan exec | lever-E study + normalizer runbook (source retired); archive-id refuted (not promoted) | −2 | 46 |

Net: 79 → 46 (−33: −34 retired/promoted-source + 1 added).

## Backlog promotions (full effort, +8 rows)
| Commit | Rows |
|---|---|
| `9a35efa` | P1-SUBSTRATE-002, P2-DOCS-003 |
| `513c214` | P1-CLI-002 |
| `372a9ad` | P1-TESTS-001, P2-PERMCFG-005, P1-TESTS-002 |
| `9c8fc0b` | P2-EVAL-001, P2-DOCS-004 |

Hygiene commit `b9cbea1` (stray blank-line normalize, no archive).

## Refutation (honest outcome — safety behavior working as designed)
`defer-backlog-archive-id-collision-precheck` proposed an archive-ID collision precheck. Verification showed `normalize-backlog.js` `buildState()` (lines 378-381) already validates active∪archive and `main()` calls it before the `--check` branch (line 727). The promoter gate already catches collisions; the card's own success_criteria ("caught at promotion time") is satisfied by the existing `--check`. **Refuted, not promoted, not silently retired** — parked with reopen condition ("only if `--check` leaves the promoter workflow"). Exercised refuted-premise authority rather than rubber-stamping a redundant row.

## Key findings
- The lever-E frontier is the live center of substantive uncertainty. Commit `5c9f370` made the seat-routing live; whether it improves recall is unproven.
- `defer-017`'s trigger was refired by lever-E but its quorum-load-bearing question is non-discriminating under the current panel (A specialized; B/C/D redundant) — parked behind an explicit semantic reopening condition (2nd specialist leaf / quorum-rule change / tier_cascade.py consuming panel metadata).
- The original audit's headline theme — "closed/shipped records polluting transport" — is **fully CLEARED**: zero closed/shipped/cancelled records remain in transport.

## Contradictions
None new this phase. Prior resolved: defer-017 liveness (trigger fired by `5c9f370`, not 06-27 as first audit recorded). All 46 remaining cards carry `backlog_id: null` — zero card↔backlog contradictions.

## Verification
| Claim | Verifying command/output | Verified |
|---|---|---|
| Transport count = 46 | `ls .local/coordinator/tasks/*.json \| wc -l` | yes |
| +8 backlog rows this effort | grep backlog.md for the 8 row IDs | yes |
| `9c8fc0b` = only backlog.md (+2) | `git show --stat 9c8fc0b` | yes |
| archive-id refuted (normalizer handles collisions) | `normalize-backlog.js` lines 378-381 + 719/727 ordering | yes |
| lever-E DoR has both layers + mechanism-not-sufficient encoded | P2-EVAL-001 row body | yes |
| recurrence-detector untouched (4-part predicate honored) | `defer-within-session-recurrence-detector.json` updated_at=2026-07-31 | yes |
| context-token-budget marker kept (debate verdict) | `study-defer-context-token-budget.json` present | yes |

## Findings
- lever-E routing is live but unproven: source=`5c9f370`, confidence=high, type=fact
- lever-E outcome (recall improvement) not-demonstrable until L2 runs: source=replan design, confidence=medium, type=inference
- archive-ID collision gap already closed by `--check` gate: source=`normalize-backlog.js` source read, confidence=high, type=fact
- leak-fix mechanism (verify-task-registry cwd/repoRoot mismatch) still present: source=orphan trace inference, confidence=medium, type=inference

## Artifacts
- Prior cap-time checkpoint: `docs/checkpoints/2026-08-02-defer-audit.md` (`21559a5`) — snapshot through the first promotion round; its open-follow-ups are superseded by this closeout.
- Disposable audit artifacts: `tmp/agent-runs/defer-audit-2026-08-02/` (report, ledger, stub-triage), `tmp/agent-runs/defer-restudy-2026-08-02/` (restudy-report, restudy-ledger).

## Open follow-ups (post-close)
- **P2-EVAL-001** (lever-E live study) — schedulable; L2 outcome layer preconditioned on (a) a fixed reference-finding corpus (tracked by the kept `recall-observational-validation-protocol` card) + (b) an operator-gated config swap for the fixed-seat baseline.
- **P2-DOCS-004** (normalizer two-commit archive-sweep runbook) — schedulable.
- **Slice 4 shaping queue** (4 independent sharpen cards: `defer-degraded-repair-savepath-backstop-test`, `shellguard-doc-prose-fp`, `defer-complexity-overlay-embedded-corpus-exclusion`, `defer-f6-failure-taxonomy-duplication`) — deferred until lever-E study progresses.
- **Parked population** (~35 cards): `defer-within-session-recurrence-detector` (4-part predicate UNMET/honored/not-reactivated), upstream/stall trio, operator-owned f1-unavailable, ready-but-parked autogate pair, refined-draft #10–#13, `verify-task-registry-fixture-leak-cleanup` (trigger unfired), 3 fog, `study-defer-context-token-budget` (considered-declined marker), `defer-017` (semantic-gated), `defer-015` (fold-in), `defer-backlog-archive-id-collision-precheck` (refuted).
