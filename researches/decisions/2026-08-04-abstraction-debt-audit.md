# Abstraction-Debt Audit — Summary-Grade Brief

**Status:** SUMMARY-GRADE — not the per-subsystem 4-step measured brief the mission specified. Produced by researcher→debate→planner; planner returned `need_evidence`. Subsystem-level verdicts are `cannot-determine` and are NOT pursued further: both the critique and the rebuttal survived unfalsified, which is a legitimate answer. Forcing a verdict there would be the unmeasured-claim failure the fence was built to prevent.

## Headline verdicts

| Subsystem | Verdict |
|---|---|
| 1. Ownership model | cannot-determine |
| 2. Permission gate | fail-closed (probe-verified 7/8 modes end-to-end; mode 3 via seam) |
| 3. Exec-sandbox trampoline | cannot-determine |
| 4. Behavioral-closure token | cannot-determine |
| 5. Agent/command count | cannot-determine |
| 6. Substrate seam / internal packages | cannot-determine |

### Supported component findings (under the cannot-determine headlines)
- Ownership reconciliation + fail-closed classification: justified. Exact taxonomy + proposal ledger: unsettled.
- Behavioral-closure consistency checking: justified. Wider scanning/doctrine cost: lacks effectiveness data.
- High-level authority separation: justified. Four-seat review shape: not justified by available evidence.
- Substrate's five properties (dry-run preview, ownership classify, managed-drift detect, lineage upgrade path, stale-embed guard): test-addressed. `internal/jsonc`: justified.
- No safe deletion or refactor recommendation settled at subsystem level.

---

## FINDINGS (prominent — these are answers, not non-answers)

### Benchmark question — VERDICT: YES
There ARE places where the harness pays autonomous-grade cost for surfaces that are advisory or ultimately human-reviewed:
- append-only armed-conflict proposal history
- behavioral-closure scanning across Markdown artifacts
- duplicated reviewer-seat prompt representations
- parts of release-readiness doctrine

These are **scrutiny candidates, not safe deletions.** Autonomous controls directly preceding execution/mutation (ShellGuard, sandbox containment, staged ownership application, lineage/drift, stale-corpus refusal) remain materially different and justified.

### Dogfood re-test — the advisory was NOT disabled prematurely
The disabled complexity advisory yielded **1/40 definite findings** (≤3/40 including ambiguous). Its real blind spot is the **agent-context axis** — the dominant cost this audit surfaced.

**Rendered prompt corpus: ~7,537 LOC total. The 4 reviewer seats contribute ~3,352 LOC — but this is a CORPUS SUM (4 × 838), NOT a per-invocation load. Each reviewer leaf loads only its own 838-line prompt.** The original audit framing cited "~44% of agent context" as if deduplication could yield a per-invocation saving; that was an error (a corpus sum misread as a per-invocation load). There is no per-invocation context saving to be had from deduplicating the four seats — each agent loads one body regardless. Dedup's only real benefits are source-duplication reduction and drift prevention. (Operator error acknowledged and corrected in this canonical version — itself an instance of the doc/claim-not-bound-to-reality defect class this audit targets.)

A precise advisory would combine: files+LOC an agent must load to change a boundary safely, function length/branching, dependency fan-in/fan-out, role mixing, churn/defect concentration.

---

## Simplification candidates
**7 passed the "cost-stated" fence; 5 were rejected for no-cost** (the fence held — rejected-for-no-cost is itself an integrity signal). None approved for implementation (read-only pass).

Surviving candidates (named; cost detail lived in sub-sessions and is not in this summary):
1. **Shared reviewer-prompt source** — the four reviewer seats are byte-identical except `description:`; dedup's real value is source-duplication reduction and drift prevention, NOT a per-invocation agent-context saving (the corpus-sum error corrected above). (Dedup, NOT seat removal — a different decision needing different evidence.)

   **POST-AUDIT TEST (2026-08-04): PREMISE HELD, MEASURE REFUTED.** Byte-identity verified (4 × 838 lines, differ only in `description:`). But the renderer is a verbatim 1:1 token-pass (`internal/substrate/renderer.go`; `docs/ai/template-authoring.md`) with no partials/includes/fan-out BY DESIGN. Each rendered agent file must carry the full 838-line body, so the ~44% agent-context reduction is **architecturally unsatisfiable** at the source level. Only drift-elimination (edit one source → regenerate four) is achievable, and only via a committed generator — which does NOT reduce what agents load, and adds a generator+lint surface. **Reclassified: not actionable as framed.** Operator decision pending: accept status quo, OR commit a generator for drift-elimination-only value, OR pursue a renderer partials/includes change (larger architectural move). The agent-context cost itself stays unless the review cascade is restructured (the seat-removal decision, which is out of scope here).

   **DISPOSITION (2026-08-04): ACCEPT STATUS QUO.** Method applied — load-bearing property: the four seats must not silently DRIFT apart. Minimal mechanism preserving it: a test asserting the four seat prompts are byte-identical except the `description:` line (~10 lines, no generator, no lint rule, no renderer change). A committed generator (drift-elimination) was rejected: it adds machinery to maintain for four byte-identical files that rarely change, and buys no agent-context gain. A renderer partials/includes change was rejected: a large architectural move whose only payoff was the per-invocation context saving that does not exist.

   **ARCHITECTURAL FACT (record for recurrence):** the harness renderer is a verbatim 1:1 token-pass (`internal/substrate/renderer.go`; `docs/ai/template-authoring.md`) with no partials, includes, or fan-out — BY DESIGN. Consequence: source-level deduplication CANNOT reduce rendered output size. Any reduction in what agents load requires a renderer change, not a source-structure change. This constraint will recur whenever "deduplicate rendered artifacts" is proposed.
2. Persistent or in-process ShellGuard evaluator.
3. Current-state-only proposal records.
4. Folding `local_only` out of the taxonomy.
5. Reconciling `overlay_extension` semantics.
6. Moving closure consistency to a typed transition boundary.
7. Splitting the large seam orchestration surface by phase.

---

## ShellGuardHook fail-direction — VERIFIED fail-closed at probe grade
All 8 fault modes observed **Deny (fail-closed)** via live probe driving the real `Evaluate` code path: modes 1–2 (node missing, eval.js missing) through real `validate()`/`bridgeErr`; modes 4–8 (timeout, non-zero-exit, empty-output, malformed-JSON, unknown-action) through the production `osExecRunner` + real node v24 with only `eval.js` stubbed; mode 3 (spawn error) via the package's exported `WithRunner`/`Runner` injection seam. Independently corroborated by the package's `fakeRunner` test suite (NodeMissing/EvalMissing/ExitNonZero/RunnerError/MalformedJSON/UnknownAction/EmptyStdout all PASS = Deny).

**Honest caveat:** mode 3 was driven through the designed injection seam rather than a true post-`validate` spawn fault, which is non-deterministic without source edits. All other modes were probed end-to-end against production plumbing.

| mode | observed |
|---|---|
| node missing | Deny |
| eval.js missing | Deny |
| spawn error | Deny (via Runner injection seam) |
| timeout | Deny |
| non-zero exit | Deny |
| empty output | Deny |
| malformed JSON | Deny |
| unknown action | Deny |

**Safety grade upgraded from `cannot-determine` to fail-closed, probe-verified for 7/8 modes end-to-end (mode 3 via seam with the non-determinism caveat above).** The probe was required, not belt-and-suspenders: exec-sandbox's absent→most-permissive defect was VISIBLE IN CODE (`case "", "off"`) with a comment asserting it was intentional, and shipped for weeks — code-grade and probe-grade are different, proven by direct project history.

---

## Stale-doc instances (8+) — a doc not bound to the code it describes IS the project's recurring defect class
1. `internal/permission/doc.go` — claims `NoOpHook` is the wired default; `ShellGuardHook` is. **Directly caused the external critic's factual error about a safety gate.**
2. `internal/ownership/doc.go` — describes the live armed-update-path wiring as future work.
3. `internal/ownership/class.go` — `overlay_extension` comment conflicts with the live wholesale-overwrite behavior in apply code.
4. `researches/sources/2026-07-07-codex-bubblewrap-sandbox-study.md` — Go-native/no-reexec reasoning incomplete relative to the shipped parent-supervision architecture.
5. `researches/sources/2026-07-28-complexity-advisory-signal-triage.md` — body recommendation no longer matches current policy.
6. `templates/migrations/v0.1.8.md` — uses `local_adapted`.
7. `templates/migrations/v0.2.1.md` — uses `local_adapted`.
8. **Premise corrected during harvest (direction was inverted):** `local_adopted` is the CURRENT canonical term (~100× across `*.go`, e.g. adopted-version, the coordinator adoption marker `{"version":1,"adopted":true}`). The obsolete/typo form is `local_adapted`, present only in `templates/migrations/v0.1.8.md` and `v0.2.1.md`. Any erratum must correct `local_adapted` → `local_adopted`, NOT the reverse. (This inversion — caught during the harvest — is a second instance of the doc-not-bound-to-code defect class the harvest targets. The erratum is releaser-owned: released migration notes are immutable in-tree (`TestMigrationNotes_ReleasedImmutable`) and ship as an erratum in the next release note.)
9. Behavioral-closure "unbypassable" wording — two occurrences located. (1) `researches/decisions/2026-07-24-behavioral-closure-pilot.md` — corrected to "consistency-enforcing" in commit `6fb9493`. (2) `internal/cli/doctor_behavioral_closure.go` — deferred: correcting it re-fires open DEFER card `defer-015` (releaser-owned release-defer machinery); under bounded adjudication this pass (see post-audit dispositions).

---

## Unresolved claims (12) — KEPT with evidence paths; NOT resolved by argument
1. **Minimum ownership taxonomy** — needs class/call-site/serialized-value census + compatibility fixtures.
2. **Real value of the armed-conflict ledger** — needs conflict frequency, operator-read, resolution, recurrence-use data.
3. **Correct `overlay_extension` authority** — needs a consumer-semantics decision (live overwrite vs documented extension).
4. **ShellGuard operational fail-closed proof** — CLOSED this pass: all 8 fault modes observed Deny (fail-closed) via live probe driving the real `Evaluate` path; 7/8 end-to-end against production `osExecRunner`, mode 3 (spawn error) via the `WithRunner` injection seam with a non-determinism caveat.
5. **ShellGuard overhead + parser-fallback precision** — needs warm/cold p50/p95 + parser-present/absent differential tests.
6. **Whether reexec is irreducible** — needs an equivalent alternative preserving child-only restrictions, parent supervision, single-binary constraint.
7. **Behavioral-closure effectiveness** — needs post-adoption catch-rate evidence. (The verified historical failures were found by human investigation, not by the token.)
8. **"Proven while three races survived"** — no primary source was found.
9. **Optimal reviewer-seat count** — needs fixed-input 4/2/1-seat comparison (net-new upheld findings, misses, latency, token cost, provider-loss behavior).
10. **Production value of the debate triad** — existing evidence is bounded and curated, not production-representative.
11. **Fresh sandbox + substrate behavior** — relevant tests exist but fresh execution receipts were unavailable.
12. **Exact path of stale behavioral-closure pilot wording** — CLOSED this pass: two occurrences located (stale-doc item #9 above). (1) pilot memo corrected to "consistency-enforcing" in `6fb9493`; (2) `internal/cli/doctor_behavioral_closure.go` deferred to DEFER `defer-015`.

---

## Closure
```behavioral-closure
verdict: inconclusive
result: not-demonstrable
crux:
  path: evidence re-derived across ownership, permission, sandbox, behavioral closure, agent orchestration, and substrate boundaries
  verifier: inspected code plus observed fault probes and fresh targeted integration tests
  command: N/A
  notes: Code, tests, repository history, and limited timing probes were inspected. ShellGuard fault denial observed fail-closed across all 8 modes via live probe (7/8 end-to-end; mode 3 via injection seam with non-determinism caveat). Sandbox integration behavior and substrate tests were not freshly observed at report time. Several effectiveness and marginal-value claims remain unmeasured. Subsystem-level cannot-determine is the legitimate answer where both critique and rebuttal survived unfalsified. The benchmark-question YES finding and the dogfood NOT-disabled-prematurely finding ARE answers, not non-answers.
```

---

## Post-audit dispositions (2026-08-04)
- **Stale-doc harvest:** 6 files corrected, committed as `6fb9493` (`internal/permission/doc.go`, `internal/ownership/doc.go`, `internal/ownership/class.go`, two `researches/sources/` addenda, the behavioral-closure pilot memo). Two items correctly stopped on enforced guards (migration-note immutability → releaser erratum; `doctor_behavioral_closure.go` "unbypassable" → DEFER `defer-015`).
- **Reviewer-seat deduplication:** ACCEPT STATUS QUO. The per-invocation context saving does not exist (corpus sum, not per-load). A drift-prevention test is the chosen minimal mechanism. The verbatim-1:1 renderer constraint is recorded above as an architectural fact.
- **Migration-note `local_adapted` typo:** releaser-owned erratum in the next release note; direction is `local_adapted` → `local_adopted` (canonical).
- **DEFER `defer-015`:** bounded adjudication this pass — answer only whether the termination contract is genuinely unbypassable at the `doctor_behavioral_closure.go` path; correct the one adjective if not, else wording stands.
- **`solution-brief` cannot write its own deliverable:** curated as a DEFER candidate (harness-process concern; trigger: `solution-brief` tasked with a file deliverable; 5+ observed instances; two candidate fixes recorded).
- **Subsystem-level cannot-determine verdicts:** deliberately NOT pursued further. Both critique and rebuttal survived unfalsified; that is the legitimate answer. The ten remaining unresolved claims stay listed with their evidence paths (twelve total; #4 and #12 closed this pass).
