# 2026-08-07 DEFER-resolution campaign — closeout

## Mission
Close out the DEFER-resolution campaign initiated 2026-08-07 (post-`v0.23.0` tag) following the fresh `solution-brief` recommendation (**OPT-B → OPT-A → selective OPT-C; park OPT-D**, superseded to *triage-first*). Records the campaign's final state. This does **not** supersede `2026-08-02-defer-closeout.md` (which closed the audit/replan effort at 46 cards); it documents the subsequent re-growth and this campaign's reduction.

## Headline
Campaign **fully executed and committed** (7 slices, ~16 cards retired, 4 new DEFERs captured). All actionable resolve-now items landed. Working tree clean, full suite green throughout. **However — the transport re-grew from 46 (2026-08-02 closeout) to 85 (this campaign's start) over 5 days; the campaign reduced it to 73 but did not change the intake mechanism.** The deferred-debt recurrence is structural (see Findings).

## Population delta (this campaign)
| Phase | Action | Delta | Running |
|---|---|---|---|
| Start (2026-08-07) | re-grown from 46 on 2026-08-02 (+39 intake in 5 days) | — | 85 |
| Slice 2 | retire 10 (satisfied / aspirational / transient / already-covered) | −10 | 75 |
| resolve-now | retire 6 (cards resolved by the implementation slices) | −6 | 69 |
| Campaign DEFER captures | 4 new (032–035) from review findings | +4 | 73 |

Net: 85 → 73 (−12: −16 retired + 4 captured). The 2026-08-02 baseline of 46 grew to 73 net (+27) over the 5-day window.

## Commits this campaign (post-`v0.23.0`, on `main`)
| Commit | Slice | Card retired |
|---|---|---|
| `8f6c9d3` | trigger-checker 6-state repair + refuse false-READY on malformed compounds | `defer-trigger-checker-distinguish-nonfiring-from-hold` |
| `ef93e38` | release-tag tests gate on shared NodeMinMajor (=18, unchanged) | `release-tag-tests-gate-on-node-minmajor` |
| `95ac104` | product-prefixes backslash-normalization regression test (pins existing behavior) | `defer-product-prefixes-backslash-normalization-test` |
| `aa9d082` | commit-gate half-born-lock mutual-exclusion fix (two monotonic refusal guards) | `commit-gate-half-born-lock-stale-break` |
| `f71ba19` | solution-brief prompt softening (card premise refuted — `tmp/**` grant existed since `78f9541`) | `defer-solution-brief-stranded-deliverable` |
| `43f511e` | perm-allowlist: allow `git merge-base`/`rev-list` across BOTH Go exec-ro AND JS shell-guard + harden mutation deny surface (GitMutationVerbs 21→35) | `perm-allowlist-readonly-git-reachability` |
| `361e1fb` | doctor check #24 `closeout-reach` (reconcile closeout ledger both ways via reachability) | `doctor-ledger-reach-reconcile` |

## Retirements (16)
**Slice 2 (10):** `defer-022` (satisfied in v0.21.0), `study-defer-context-token-budget`, `study-defer-tool-capability-registry`, `study-defer-drift-entropy-audit`, `recall-observational-validation-protocol`, `p2-ops-001`, `p2-docs-001`, `defer-cf2-worker-seam-render-test` (covered), `defer-sf1-sf3-skill-sentinel-dedup` (covered), `defer-trigger-checker-distinguish-nonfiring-from-hold`.
**resolve-now (6):** `release-tag-tests-gate-on-node-minmajor`, `defer-product-prefixes-backslash-normalization-test`, `commit-gate-half-born-lock-stale-break`, `defer-solution-brief-stranded-deliverable`, `perm-allowlist-readonly-git-reachability`, `doctor-ledger-reach-reconcile`.

## Standing watch set (~57 draft cards)
The intentional residual — each carries a concrete reactivation condition (path_touched / event / measurement trigger). Includes the release-ceremony, commit-gate, permissions, test-coverage, and process-design watches. Plus 2 `ready` (`task-…-audit-queue-retry-coordination-flow`, `defer-release-auto-recover-agent-runtime-coverage`) and 1 `cancelled`. No pure-aspiration fog remained after the Slice 4 re-triage (all 13 fog/research cards adjudicated KEEP with concrete conditions).

## DEFERs captured this campaign (`.local/`, draft)
- `defer-028` — #13 live-repo canary asymmetry (pre-existing).
- `defer-032` — checker cold-glob + after_tag classification boundary.
- `defer-033` — commit-gate stuck-lock availability tradeoff (fail-safe leaves meta-less locks permanently contended).
- `defer-034` — solution-brief persist-brief behavior precondition-proven but NOT outcome-proven.
- `defer-035` — closeout-reach uppercase-SHA case-domain mismatch (`isValidFullSHA` accepts A-F; `git rev-list` emits lowercase).

## Open follow-ups (post-close)
- **`defer-034`** — the solution-brief persist-brief behavior is **precondition-proven but not outcome-proven**. `f71ba19`'s behavioral-closure was `inconclusive`. Needs one live `/solution-brief` dogfood demonstrating the model actually Writes `tmp/agent-runs/<alias>/brief.md`. The card was retired on the preconditions; the verification is still owed.
- **`defer-livebridge-matrix-short-skip-coverage`** — genuine gap (the LiveBridge matrix still silent-skips with bare `t.Skip()`, no count sentinel). Kept, not resolved.
- **`v0.23.0` not pushed** — tagged locally (`d395381` → `c9bb13c`); `git push origin v0.23.0` to publish.

## Findings
- **Deferred-debt re-growth is structural:** source=transport counts (46 on 2026-08-02 → 85 on 2026-08-07), confidence=high, type=fact. The campaign retired 16 but did not change the intake mechanism (review DEFERs from every slice + ad-hoc intake). The plan's parked **OPT-C** (selective trigger normalization) + a tighter intake bar are worth revisiting if re-growth continues.
- **The commit-gate's tree-bound re-review is the authoritative safety layer for security/permission slices:** it caught TWO real JS-shell-guard defects the coordinator-level `/commit-review` (4/4 approve) missed — both on the `perm-allowlist` slice (the `\b`→`merge-base` boundary, then the hyphenated-plumbing hole). source=the two blocks on `43f511e`, confidence=high, type=fact.
- **Coordinator can self-discharge B1** when read-only review seats deny node/go: running `vh-agent-harness exec node --test`/`go test` directly binds the result to the tree (converted the perm-allowlist reliability-block from "deferred" to "satisfied"). source=campaign execution, confidence=high, type=fact.
- **perm-allowlist premise correction:** the card's original framing ("allowlist denies merge-base/rev-list") was too narrow — the Go-layer widening is necessary but not sufficient; the JS shell-guard `GIT_MUTATION_RE` boundary also needed fixing, which in turn required adding 14 mutating-plumbing verbs to `GitMutationVerbs`. The cross-layer coupling (`tables.go` is the single source for both Go exec-ro AND the JS regex) is the root of this complexity. source=`43f511e` + the 2 blocks, confidence=high, type=fact.

## Coordinator error (transparency)
The `perm-allowlist` card was **prematurely retired in parallel with its first commit, before seeing the commit-gate's block.** The work didn't land (2 security blocks followed). The card was recreated for honest tracking and properly retired only after `43f511e` actually landed. **Lesson:** for security slices, retire the card only after the commit-gate confirms landing, not after the coordinator-level review.

## Contradictions
None new. One operational correction: the 2026-08-02 closeout's "fully closed, 46 cards" framing was true for that effort but the transport re-grew to 85 by this campaign — the "closed" status was a point-in-time snapshot, not a stable state. This checkpoint records both honestly.

## Verification
| Claim | Verifying command/output | Verified |
|---|---|---|
| Working tree clean (campaign done) | `git status --short` → empty | yes |
| All 7 campaign commits on `main` | `git log --oneline -9` (8f6c9d3..361e1fb atop c9bb13c v0.23.0) | yes |
| Transport count = 73 | `ls .local/coordinator/tasks/ \| wc -l` → 73 | yes |
| perm-allowlist feature functional end-to-end | `node tmp/plumbing-proof.mjs` (merge-base ALLOW, all 16 plumbing DENY, separators DENY) + live shell-guard block on `git checkout-index` | yes |
| commit-gate mutual-exclusion fix | `go test ./internal/cli/ -run TestCommitGate_HalfBornLockStaleBreak -v` → 4/4 PASS | yes |
| doctor #24 finds real orphans | `./bin/vh-agent-harness doctor` → closeout-reach WARN 4 orphaned SHAs (exit 0) | yes |
| Recurrence count (46→85) | transport counts across the two checkpoints | yes |

## Artifacts
- Disposable triage worksheet: `tmp/defer-triage-worksheet.md` (candidate-only, the full 82-card classification that fed the keep/retire decisions; gitignored transport).
- The 7 commits above carry their own test evidence + behavioral-closure tokens.