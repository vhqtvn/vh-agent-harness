# Postmortem → Guardrail Convention

How incident learnings leave durable protection behind. Mined from the
deepseek-harness study (execution-safety idea #9, adopted 2026-08-18 — see
`docs/checkpoints/2026-08-18-assurance-convergence-adoption.md`).

## Rule

When a postmortem lands (in a checkpoint under `docs/checkpoints/`, a local
closeout report, a task card, or a decision memo under `researches/decisions/`),
it must leave behind **exactly one** row-shaped entry:

> incident class → root mechanism → ONE concrete guardrail → its verifier

Constraints:

1. **Existing records only.** The row lives in whatever durable record the
   incident already produced. No new archive, ledger, registry, or transition
   authority — this convention adds zero systems.
2. **One guardrail per class.** If a class already has a guardrail, a new
   incident in that class STRENGTHENS the existing guardrail (or its
   fixtures) instead of adding a second one.
3. **The verifier must exist and run.** A guardrail without a runnable
   verifier (test, gate, script, or documented check command) is prose, not
   a guardrail. Name the command.
4. **Prefer widening an existing contract test** over new test files; prefer
   a mechanical gate (preflight, doctor check) over prompt guidance.
5. A shipped-bug class that recurs DESPITE its guardrail is a signal the
   guardrail is structural-only — escalate to the coordinator, do not
   silently stack mitigations.

## Registry of classes (existing guardrails — do not duplicate)

| Incident class | Root mechanism | Guardrail | Verifier |
| --- | --- | --- | --- |
| CI leg failure/skip reads as green | skip paths collapse into success | `VH_REQUIRE_LIVE_BRIDGE=1` skip-to-red + blocking full-suite matrix | `go test ./internal/ -run TestCIContract_` (go-test.yml job) |
| Best-effort step masks gate outcome | `continue-on-error` swallows the deciding failure | dry-run + Fail-on-drift step pair wired by `steps.<id>.outcome` | `TestCIContract_RenderCheckDriftFailsWorkflow` |
| Required checks go stale on lane churn | branch protection pins per-leg job names | `ci-aggregate` stable sentinel depending on all blocking jobs | `TestCIContract_StableAggregateJob` + operator pins it in branch protection |
| Publication outruns release-readiness | publisher step ordered before evaluation | DEFER evaluator step ordered before GoReleaser | `TestCIContract_ReleaseReadinessRunsBeforePublication` |
| Cleanup machinery reaps live state | duplicated protection sets drift apart | shared `_protected_uuids` construction + scratch age gate in `commit-gate.sh` | `make test-js` (tests/scripts) + P3 audit record (2026-08-18) |
| Stat failure inverts freshness | `|| echo 0` makes a file look ancient | `_file_age_seconds` stat-fallback → age 0 → retention | P1-CORE-031 repro + `tests/scripts` suite |
| Unidentifiable lock destroyed | half-born lock indistinguishable from stale | identity-bound stale-break: empty uuid → never stale, never deleted | `_is_stale`/`_stale_break` in `.opencode/scripts/commit-gate.sh` + P3 audit record |
| Review approval treated as landing | card retired before the commit landed | Task-Card trailer landing-proof before retirement | `git log --branches --fixed-strings --grep=Task-Card: <id>` (RECORD_LIFECYCLE) |
| Ambiguous "green" | one word covers full/targeted/clean states | B1 / B2 / A vocabulary kept separate | `docs/coordination/CLOSEOUT_TEMPLATE.md` + doctor behavioral-closure audit |

## Authoring checklist

- [ ] Class named at class level (not "the bug in file X").
- [ ] Root mechanism stated (why the old guardrail shape could not see it).
- [ ] Exactly one guardrail; verifier command named and runnable today.
- [ ] Row added to the table above ONLY if it is a new class; otherwise the
      existing row's verifier/fixtures were strengthened.
