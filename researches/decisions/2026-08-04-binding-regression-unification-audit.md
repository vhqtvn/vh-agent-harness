# Solution-Brief: Binding-Regression Unification Audit

**Status:** read-only pass complete; operator sign-off granted 2026-08-04 with corrections (leg #1 retracted outright; 36/29 trigger split made authoritative; producer's extraction-method error added as its own finding); promoted to canon `researches/decisions/`. Follow-up `/research` re-scoped to the refusal-site fix list (not a hypothesis test).
**Scope:** vh-agent-harness (this repo). Read-only cross-reference to consumer repos only where a finding concerns consumer reach. No adopter repository is named anywhere in this artifact.
**STOP condition honored:** research → debate → planner chain ran; brief written; no source/doc/backlog/ledger edits beyond this promotion; git mutation routed to committer.

## Organizing hypothesis (tested, not assumed — and the universal form DID NOT survive)

Every recurring failure in this project was hypothesized to be one shape: **an artifact not bound to the surface where it takes effect.** Candidate verified instances (from canon): capability ↛ friction-point (exec-sandbox); decision ↛ durable card; claim ↛ observation (3/9 traced vs 6/9 observed); doc ↛ code (`permission/doc.go`, `ownership/doc.go`, `ownership/class.go`); source ↛ render (stale-embed); config ↛ process state (per-process caches); number ↛ scope (inverted ranking). Corroborated independently by `researches/decisions/2026-08-04-capability-discovery-audit.md` E24.

The job was to **falsify** the unification, not confirm it. A forced unification would itself be the unmeasured-claim failure this project keeps paying for — and producing this very brief supplied one more instance of that class (see §Self-referential instance below).

## Synthesis verdict: PARTIALLY HOLDS — the bounded model is the durable output

A **universal** claim ("every unguarded fix regresses; the guard IS the binding") is **not established**. A set of **bounded** binding principles IS supported:

1. Effects should be explicitly bound to the authoritative surface where they are observed.
2. Safety and recovery guards should be bound to the transition they govern.
3. Durable records should remain bound to recoverable meaning.
4. Optimization claims should be bound to operational measurements.

**The durable output is the four-principle bounded binding model. The resisting cases below are the load-bearing part — they are what stops a future reader from re-deriving a universal claim from a bounded one.**

**Failures that RESIST the unification** (named, per the falsification directive):

- promotion vs. digest/re-derivation vs. behavioral tests — these are different binding KINDS, not one;
- intentionally selective retention of review rationale (a deliberate non-binding, not a defect);
- domain-specific refusal semantics (refusals differ by domain, not one shape);
- render transformations that preclude universal byte identity (verbatim-1:1 is a real constraint, so byte-identity cannot be the universal binding);
- watch-card safety vs. disposable-transport policy (retiring a watch is a policy act, not a binding defect);
- concurrency identity and ownership constraints;
- context reductions that may degrade discovery, routing, or safety (a tradeoff, not an unguarded fix).

**Debate outcome:** `need_evidence` → resolved by operator adjudication to a **bounded** verdict (not a universal one).
**Current leader:** an evidence-gated **bounded** binding model — explicitly NOT a one-, two-, or three-root causal model, and not pursued further as one.

## Self-referential instance — a number not bound to its extraction method (NEW, found producing this brief)

The original "sharpest test" asserted a skill-description regression of 4,283 → 4,829 chars, drawn from a note that the formal-verification description had CHANGED after `edd5ea5`, hedged as "delta unverified," yet used as the sharpest of three tests. Measured properly at HEAD `5e871ac`, the three trimmed pilots are contract-invariant-audit 225 + resolve-first 220 + formal-verification 214 = **659 chars**, against **680** recorded post-`edd5ea5` — a **decrease of 21**. No regression in magnitude OR direction.

This is its own instance of bounded principle #4 (optimization claims bound to measurements): a number detached from its extraction method, softened with "unverified," then promoted into a load-bearing role anyway. **Same class as traced-3/9-vs-observed-6/9 and the unscoped ranking that inverted.** Recorded here so the next pass does not repeat it.

## Sharpest test (TWO fresh regressions at HEAD `5e871ac` — leg #1 RETRACTED)

1. ~~Skill descriptions regressed 4,283 → 4,829.~~ **RETRACTED** (see §Self-referential instance). Corrected: 659 vs 680, **down 21**. No regression in magnitude or direction. The corrected numbers stand.
2. `.local` defer pile: 45 → 65 cards; under the **executable-checker grammar that actually governs firing**, **29 cards** lack a legal trigger (not 22). No gate on card creation. The 36/29 split is the authoritative figure (see baseline).
3. Commit-refusal rate: 16.0% (32/200) → 20.0% (40/200). (Canon decision `2026-08-04-s2-canonical-tree-integrity-relaxation.md` owns the dispatch-side resolution; not reopened here.)

**Contrast:** the reviewer-seat byte-identity property has a TEST (`f8de2a3`) and therefore cannot drift. This is the strongest single example of a guard tied to a specified invariant — but it does **not** generalize to the universal regression claim.

## Baseline (Phase 0) — verified with corrections

| Metric | Prior figure | Corrected/verified figure | Note |
|---|---|---|---|
| Agents | 23 | 23 | confirmed |
| Per-agent max LOC | 1,024 (`harness-release-readiness`) | 1,024 | confirmed |
| `releaser` LOC | 993 | 993 | confirmed |
| Reviewer-seat LOC (each) | 838 | 838 | confirmed; per-invocation load is per-file, NOT the 3,352 four-seat sum |
| Corpus sum | 7,537 | 7,537 | confirmed |
| Skill descriptions | 4,829 chars (claimed) | **659 chars** (3 trimmed pilots: 225 + 220 + 214) vs **680** post-`edd5ea5` | leg #1 RETRACTED — down 21, no regression |
| Skills count | 14 | 14 | confirmed |
| Commands | 39 | 39 | confirmed |
| Doctor check groups | 20 | **23** | corrected upward |
| `.local` cards | 65 (64 draft / 1 ready) | 65 (64/1) | confirmed |
| Trigger split | 43 / 22 | **36 legal / 29 not** (executable-checker grammar — AUTHORITATIVE; this grammar governs firing) | 36/29 makes the card-pile finding WORSE than the 43/22 figure |
| Canon decision files | 32 / 9,369 LOC | 32 / 9,369 | confirmed |
| Canon source files | 33 / 11,917 LOC | 33 / 11,917 | confirmed |
| Ledger closeouts | 200 (160 committed / 40 rebased_refused) | 200 (160/40) | confirmed in retained window |

**Unmeasured (stated plainly):** full `go test ./...` wall-clock was NOT measured under the read-only/no-write execution constraint. Ordinary-turn request, usage, and token receipts were NOT collected; no context-optimization claim rests on them.

## Disposition candidates per lens (Phase 4 cap = 5)

All lenses within cap.

| Lens | Candidates |
|---|---:|
| Discovery / surface-at-friction | 5 |
| Durability | 4 |
| Verification integrity | 5 |
| Render binding | 3 |
| Rediscovery | 3 |
| Context cost | 5 |
| Tangle / concurrency | 5 |

**Fence-worked evidence:** the cap held across all seven lenses (≤5 candidates each, **zero** escalations to a design-decision). This is evidence the fence worked — directly comparable to the abstraction-debt pass's five rejected-for-no-cost candidates. A cap that FIRES (forces a design decision instead of N cards) would be the failure signal; here it did not fire, which is the success signal. Worth stating in canon because it is the same class of evidence as the abstraction-debt pass's negative findings.

## Dogfood verdict (honest — this is S2 pilot evidence)

- **`contract-invariant-audit`:** Helped with explicit inventory and completeness accounting, but the packets did not consistently record whether the skill itself produced that benefit. Operational attribution incomplete; some manual manifests were needed when its helper could not run.
- **`resolve-first`:** Likely helped consolidate findings and prevent card proliferation, but attribution and effort were not measured. Verdict: useful structure, ceremony cost unmeasured.
- **`formal-verification`:** Helped narrowly. Its fidelity binding + deliberate-divergence observation (6/9 cells red) is the strongest example of a guard tied to a specified invariant. It does **not** support the universal regression hypothesis. Ceremony cost unmeasured.

## Remaining work (re-scoped — fix list, NOT a hypothesis test)

The unification question is **ANSWERED**; it does not need further settling, and chasing its universal form is exactly the over-generalization this pass just corrected. What remains is the actionable generalization of the one signed-off fix (surface-at-friction):

1. **Semantic dispositions of the 54 `state-lib.js` refusal/error sites** — each a concrete yes/no: does the refusal surface name its sanctioned alternative? This is the generalization of fix entry #1 (surface-at-friction) from `2026-08-04-capability-discovery-audit.md`. Output is a **fix list**, not a verdict on the binding model.
2. (Dropped unless free) a cross-lens transition matrix — pursued only if it falls out of the refusal-site work at no marginal cost.

Still unmeasured (stated plainly, not blocking the fix list): full `go test ./...` wall-clock; ordinary-turn request/usage/token receipts. No optimization claim rests on these.

## Recommended next step (for operator decision — NOT taken here)

A narrow researcher pass scoped to the actionable half ONLY. This is NOT a hypothesis test — the bounded-vs-universal question is settled:

```
/research For each of the 54 state-lib.js refusal/error sites, determine the semantic disposition: does the refusal surface name its sanctioned alternative (yes/no)? Produce a fix list generalizing the signed-off surface-at-friction fix (entry #1 of 2026-08-04-capability-discovery-audit.md). Classify any site that already names its alternative as satisfied. Do NOT re-test the binding/unification question. Drop the cross-lens transition matrix unless it falls out for free.
```

## Consumed canon (non-goals honored — not re-derived)

- `researches/decisions/2026-08-04-capability-discovery-audit.md` (E01–E27; only fix entry 1 signed off; entries 2–9 held)
- `researches/decisions/2026-08-04-abstraction-debt-audit.md` (ShellGuard fail-closed; verbatim-1:1; ~11 unresolved claims)
- `researches/decisions/2026-08-04-opt-out-pilot-distribution.md` (+ S3 default-on overlay pilot tier)
- `researches/decisions/2026-08-02-formal-verification-agent-proof-skill.md` (+ pilot sources; `ApplyFloor`/`ParseMinMode` kept pure)
- `researches/decisions/2026-08-02-tiered-defer-promotion-model.md` (+ O2: defer-invalid = 11%)
- `researches/decisions/2026-08-04-s2-canonical-tree-integrity-relaxation.md` (20% serialization cost; dispatch-side resolution; NOT reopened)

## In-flight work excluded (NOT findings)

`v0.21.0` release ceremony running (content-complete, untagged); `.vh-agent-harness/release-defer-dispositions.json` intentionally dirty (releaser-owned); queued corrections (defer-015 adjective in `doctor_behavioral_closure.go`; `local_adapted`→`local_adopted` migration erratum); possible skill-description trim + length-guard slice. All routed/owned.

```behavioral-closure
verdict: proven
result: proven
crux: the unification question is answered — a bounded binding model (four principles) is supported; the universal "every unguarded fix regresses; the guard IS the binding" claim is NOT established; resisting cases are named in §Failures that resist
verifier: falsification test against the full finding set + consumed canon; operator sign-off accepted the bounded outcome and explicitly retracted the universal claim
command: (analytical — no single command; grounded in consumed canon + this audit)
reason: the analytical question is settled, not measurement-gated
deferred-not-demonstrable: the 54 refusal-site fix list — produced by the re-scoped follow-up /research above, not by this pass
```
