# 2026-08-02 — DEFER transport audit & curation

## Mission
Comprehensive audit of the DEFER holding area (`.local/coordinator/tasks/`) to disposition every transport card: inventory, trigger-liveness, staleness/contradiction, clustering, per-card disposition (retire/promote/sharpen/keep/fog). Followed by operator-authorized execution of the safe dispositions.

## Headline
**79 → 51 cards (−28).** Dominant theme: closed/shipped records polluting transport — a single commit (`59cbfbc`) silently retired 7 cards without transport reconciliation, and several post-slice closures left their originating cards un-reconciled.

## Population delta
| action | count | commit |
|---|---|---|
| Retired (round 1: completed/cancelled/shipped) | 18 | gitignored rm (no commit) |
| Promoted → backlog, source retired (quarantine cluster + defer-011) | 4 source → 2 rows | `9a35efa` |
| Retired (round 2: orphan fixture + 5 debate-confirmed stubs) | 6 | gitignored rm |
| Drafted (new leak follow-up candidate) | +1 | gitignored |
| Promoted (f1-pa source retired) | 1 source → 1 row | `513c214` |
| Sharpened in-place (7 stubs, no count change) | 0 | gitignored |
| **Net transport delta** | **−28** | |

## Backlog promotions (3 new rows)
- **`P1-SUBSTRATE-002`** (Next/todo) — consolidated quarantine substrate slice: syntax-invalid-JSON quarantine in `state-lib.js`/`verify-task-registry.js` + negative-path coverage + quarantine regression tests. 3 cards → 1 row. [`9a35efa`]
- **`P2-DOCS-003`** (Next/todo) — defer-011 cas_conflict recovery doc-drift across 3 recovery docs. Own row (different file scope from P2-DOCS-002). [`9a35efa`]
- **`P1-CLI-002`** (Next/todo) — `internal/cli/f1_pa.go` EvidenceRefs + CheckedScope dedup (DROP polish, producer-purity-preserving). Promoted via operator override (f1 trigger unfired). [`513c214`]

## Key findings
1. **Closed/shipped records polluting transport** — the dominant theme. Post-slice transport reconciliation is missing as a habit; shipped/cancelled work leaves orphan cards.
2. **Latent test-harness leak** — `verify-task-registry.js` saves fixture cards with `{cwd:"/verification"}` while `repoRoot()` resolves to the real repo; the `finally` cleanup only removes the current run's IDs → fixture orphans accumulate indefinitely. Root cause of the `P0-REPO-060` orphan (a hardcoded fixture literal at `verify-task-registry.js:143`). Captured as draft follow-up `verify-task-registry-fixture-leak-cleanup`. Mechanism confidence: medium (inference from trace, not live reproduction).
3. **`study-defer-context-token-budget` kept (not retired)** — debate verdict. The card carries the operator's 2026-07-13 decline AND an unfired reopen trigger (operator-request OR measured context-pressure). A considered-declined marker with a live trigger, not dead weight.
4. **Sharpen cohort refined** — 7 stubs sharpened in transport (6 clean, 1 honest cannot-concretize). 3 now promote-ready (triggers fired); 4 operator-request gated.

## Contradictions
- `P0-REPO-060` orphan link: RESOLVED — the backlog row never existed (hardcoded fixture literal that escaped cleanup).
- Audit's `notes_n=0`→stub proxy: contradicted by 4 rich-content keep cards; over-counts true stubs by ~4.
- "12 Done rows wanting archival": REFUTED — `normalize-backlog.js` is count-based (`--main-done-limit=12`); 12 rows = at limit, 0 overflow. The "cleanup required" signal was a stray blank line, normalized in `b9cbea1`.

## Verification
| Claim | Verifying command/output | Verified |
|---|---|---|
| 79→51 transport delta | per-round `ls .local/coordinator/tasks/ \| wc -l` counts | yes |
| 3 backlog rows committed | `git show 9a35efa`, `git show 513c214` | yes |
| P0-REPO-060 never existed | `git log --all -S "P0-REPO-060" -- docs/planning/` (empty) + grep at `verify-task-registry.js:143` | yes |
| #6 keep with unfired reopen trigger | card body read during debate | yes |
| 7 sharpen cards refined, no promotion | mtime audit + `status: draft` preserved | yes |
| No archive drift (count-based normalizer) | `normalize-backlog.js --check` + source inspection | yes |

## Findings
- **(closed/shipped pollution)**: source=disposition ledger + per-card evidence, confidence=high, type=fact
- **(test-harness leak mechanism)**: source=orphan trace inference (cwd/repoRoot mismatch), confidence=medium, type=inference
- **(normalizer count-based semantics)**: source=`normalize-backlog.js` source + debate, confidence=high, type=fact

## Artifacts (disposable, under tmp/)
- `tmp/agent-runs/defer-audit-2026-08-02/defer-audit-report.md`
- `tmp/agent-runs/defer-audit-2026-08-02/disposition-ledger.json` (79-row record of decisions)
- `tmp/agent-runs/defer-audit-2026-08-02/stub-triage.md`

## Open follow-ups
- **3 promote-ready candidates** (operator decision, triggers fired): `defer-consumer-render-verifier-regression-fixture`, `defer-f5-dead-rule-lint-positive-fire`, `defer-f6-guard-crux-automated-test`.
- **verify-task-registry.js leak fix** — draft card `verify-task-registry-fixture-leak-cleanup`, trigger unfired.
- **4 operator-request-gated sharpen cards** — #10–#13, awaiting operator reopen.
