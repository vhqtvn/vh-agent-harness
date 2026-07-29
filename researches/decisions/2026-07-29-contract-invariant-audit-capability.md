# Decision Brief: Domain-Free Contract/Invariant Audit Capability

**Date:** 2026-07-29  
**Status:** Decision brief complete; awaiting operator sign-off  
**Decision scope:** Capability form and pilot design only; no implementation authorized

## 1. Position — New Capability vs. Existing Overlap

### Position

**Confirm the hypothesis, with a narrow boundary:** a proactive contract/invariant audit is materially distinct from the existing commit-review panel, coverage-enforcement property, and complexity-control signal.

The proposed capability is not simply another way to detect “contract drift.” Commit-review already does that well. Its distinct purpose is to inspect a **declared existing surface across repository history**, at an explicit milestone, and produce a disposition-complete audit ledger whose surviving findings have passed an adversarial verify/refute step.

The capability remains distinct only while all of the following are true:

1. It examines an existing bounded surface, not merely the current diff.
2. It uses a reproducible manifest and accounts for every manifested unit.
3. It investigates semantic contract and invariant risks, rather than merely proving accounting completeness.
4. It subjects candidates to an independent verify/refute pass.
5. It produces advisory evidence and standing checks without acquiring transition authority.

### Differentiation

| Surface | Primary scope | Cadence | Principal question | Output | Authority |
|---|---|---|---|---|---|
| **Commit-review panel** | Exact changed-file list and acquire-anchored diff | Per commit, reactive | Is this proposed change safe and reviewable? | Tiered findings and `PASS`, `BLOCK`, or `SPLIT` guidance | May block or split the commit path |
| **Coverage-enforcement property** | Declared scope compared with disposition records | Success/report boundary | Was every declared item accounted for exactly once? | Structural completeness result | May support a separately justified structural gate; does not prove review quality |
| **Complexity-control** | Files nominated by deterministic complexity signals | Advisory scan | Which areas may warrant simplification or investigation? | Complexity nominees and advisory findings | **INFORMS**; low-precision signals should be silenced rather than over-raised |
| **Proposed contract/invariant audit** | Reproducibly manifested existing surface, potentially spanning cross-file and historical boundaries | Explicit milestone or investigation, proactive | Do declared units contain latent contract or invariant defects, and do those claims survive refutation? | Ranked ledger, standing checks, remediation brief, and rigor-check record | **INFORMS only** |

### Relationship to commit-review

Commit-review is already strong at recognizing contract drift introduced or exposed by a proposed change. The proposed audit must therefore **not** claim “contract detection” as its unique value.

Its distinct value proposition is:

- inspection of dormant or unchanged surfaces;
- examination of inconsistencies accumulated across multiple changes;
- explicit accounting across a declared surface rather than a current diff;
- investigation of implicit, cross-unit, or inconsistently enforced contracts;
- durable advisory outputs that can be rerun at later milestones.

If a pilot merely reproduces findings that ordinary commit-review would naturally have found while reviewing the relevant changes, the differentiation hypothesis fails.

### Relationship to coverage-enforcement

The audit should **reuse the coverage-enforcement property internally**:

> Every unit in the declared manifest must receive exactly one terminal disposition before the audit may report its declared scope as disposition-complete.

This is meta-integrity, not semantic validation. As Finding A states, structural coverage enforcement can expose omitted, duplicate, outside-scope, or non-terminal records, but it cannot prove review quality, attention, or semantic understanding.

The proposed audit therefore does not replace or merge with coverage-enforcement:

- coverage-enforcement proves that the audit accounted for its declared manifest;
- the audit performs the semantic examination;
- neither manifest accounting nor unit count proves that the examination was correct.

### Relationship to complexity-control

Complexity-control establishes two relevant precedents:

1. complexity signals **INFORM rather than gate**; and
2. high nomination volume with low actionable precision is a reason to silence or reshape a signal, not to create more disposition machinery.

The contract/invariant audit must adopt the same precision-first policy. A run with no verified findings is valid. Candidate volume is not a success metric.

**Position verdict:** The capability is genuinely distinct, but only as a proactive, declared-surface, adversarially verified, advisory audit. Broader claims would overlap existing surfaces.

---

## 2. Recommended Form — Skill, Agent, Command, or Guidance

### Recommendation

**Pursue a separate overlay skill, initially paired with a small deterministic manifest/completeness helper.**

Do not create a dedicated agent or command during the pilot, and do not promote the capability into the core corpus yet.

### Form comparison

| Form | Assessment | Rationale |
|---|---|---|
| **Overlay skill plus deterministic helper** | **Recommended pilot** | Captures a repeated procedure, is usable by multiple agents, keeps semantic reasoning separate from mechanical accounting, and directly tests the uncertain manifest contract |
| **Instruction-only overlay skill** | Fallback | Simpler, but manual manifest generation is more susceptible to omissions, inconsistent unit definitions, and coverage theater |
| **Doc-only guidance** | Supporting material only | Useful as reference documentation but too weak as the primary triggerable, reusable workflow |
| **Command plus skill** | Defer | May improve structured invocation later, but creates two public surfaces before repeated invocation or discoverability failures are observed |
| **Dedicated agent** | Do not pursue initially | Adds role, maintenance, routing, and authority ambiguity without evidence that the workflow requires a new specialist identity |

### Why a skill fits

The skill lifecycle guidance characterizes a good skill as one that reproduces a useful process rather than a particular output, captures a repeated engineering or review pattern, reduces rediscovery, and defines explicit triggers and completion conditions.

This capability meets that conceptual threshold:

- the reusable asset is the audit method;
- multiple existing roles may need to invoke it;
- the process has explicit inputs, phases, completion conditions, and failure states;
- the procedure is too structured to leave as informal guidance;
- the semantic work does not require a new authority-bearing agent.

The skill description must be tightly triggered because skill descriptions impose recurring context cost. Appropriate triggers include:

- auditing an existing bounded surface for latent contract or invariant defects;
- producing a disposition-complete invariant ledger;
- conducting a milestone audit across unchanged or cross-history code.

Explicit “when not to use” cases should include:

- reviewing a current commit or changed-file slice;
- ordinary bug diagnosis;
- ordinary complexity triage;
- proving test or semantic coverage;
- establishing a new hard gate.

### Pilot overlay surface

Use a project-specific experimental overlay pack with a domain-neutral name such as:

`contract-invariant-audit-pilot`

The overlay should own the pilot skill and any small manifest helper. Nothing should enter `templates/core/` during the pilot.

The overlay may localize:

- stack adapters;
- project roots and exclusions;
- supported unit granularities;
- project-specific discovery commands;
- pilot fixtures and evaluation criteria.

Those localizations are evidence for the generic methodology; they are not content to copy into the core skill.

### S1 → S2 path

The capability is currently an **S1 candidate suitable for an overlay pilot**, not an approved core skill.

The S2 rule requires a new core skill to pilot in at least one overlay against a real repository before promotion. The pilot must provide positive evidence for the uncertain design—not merely prove that a pilot was run.

Evidence sufficient to discharge S2 should include:

1. **Real repeated trigger**
   - At least one real repository demonstrates a recurring milestone use case rather than a one-off exercise.
   - Operators can reliably identify when to invoke, stop, and avoid the workflow.

2. **Manifest reproducibility**
   - Identical scope and snapshot inputs reproduce the same unit IDs and anchors.
   - Adapter version, exclusions, and discovery basis are recorded.
   - Snapshot drift is detected and either invalidates or explicitly reconciles the manifest.

3. **Disposition completeness**
   - Every manifested unit receives exactly one terminal disposition.
   - Duplicate, unknown, omitted, and outside-manifest records are detected.
   - Shards reconcile without silent omissions.
   - Exclusions are explicit and justified.

4. **Semantic precision**
   - Initial candidates undergo independent verify/refute.
   - The final verified-survivor rate, refutation rate, and indeterminate rate are measured.
   - Findings are actionable and not predominantly duplicates of ordinary commit-review findings.

5. **Net-new value**
   - The audit identifies or confidently refutes risks arising from an existing or cross-history surface that a current changed-file review would not naturally cover.

6. **Cost evidence**
   - Wall time, manifested units, examined units, shard count, candidate count, survivor count, and operator effort are recorded.
   - The cost is proportionate to the findings or confidence obtained.

7. **Failure-mode evidence**
   - The pilot demonstrates behavior for unsupported stacks, incomplete discovery, budget exhaustion, snapshot changes, indeterminate findings, and missing evidence.

8. **Authority containment**
   - No finding, standing check, helper result, or completeness result blocks a commit, release, update, or doctor run.
   - No advisory output is silently promoted into a hard invariant.

### Reversal conditions

Reverse from the helper-backed skill to an instruction-only overlay skill if the helper requires semantic or project-specific reasoning to enumerate units.

Prefer doc-only guidance or abandon promotion if:

- the workflow is not recurrent;
- manifests cannot be reproduced;
- findings mostly duplicate commit-review;
- survivor precision remains low after adversarial verification;
- cost materially exceeds the confidence or remediation value obtained;
- the workflow encourages unqualified “full coverage” claims.

**Form verdict:** Pursue the overlay skill pilot. Core promotion is not currently supported.

---

## 3. Domain-Free Procedure Specification

### Abstract audit target

A **unit** is:

> A stable, inspectable element whose state, behavior, interface, configuration, or relationship to other elements participates in a contract or invariant.

A unit is not necessarily a source file or language declaration. Depending on the selected discovery adapter, a unit may be:

- a file;
- a declaration;
- a public interface or callable;
- a configuration entry;
- a schema element;
- a generated boundary;
- a state transition;
- a resource definition;
- an explicitly declared cross-unit contract.

Core canon must not prescribe project layer names, language constructs, framework vocabulary, or domain-specific unit types.

### Five violation classes

The audit uses five language- and stack-agnostic classes.

#### C1. Redundant or derived state

State is independently stored, copied, or mutated even though it can be derived from another authoritative value.

Typical risk:

- divergence between competing representations;
- stale caches or mirrors without an explicit consistency contract;
- multiple sources of truth;
- update ordering that can leave inconsistent state.

The class does not condemn caching or denormalization by itself. A candidate exists only when the derivation, authority, invalidation, or reconciliation contract is missing or unreliable.

#### C2. Hidden side effects

An operation performs externally observable work that is not apparent from its name, interface, documented contract, or expected abstraction.

Typical risk:

- callers cannot reason locally about consequences;
- read-looking operations mutate state;
- lifecycle, persistence, network, process, or global-state effects are obscured;
- testing and rollback assumptions become unreliable.

#### C3. Unenforced invariants

A required relationship or state constraint is assumed by code or documentation but has no reliable enforcing mechanism at the relevant transition.

Typical risk:

- invalid states remain representable or reachable;
- enforcement occurs only in some paths;
- checks happen after effects have already occurred;
- convention is mistaken for enforcement.

#### C4. Unstated preconditions

Correctness depends on caller state, ordering, environment, initialization, prior validation, or input properties that are not made explicit or mechanically discoverable.

Typical risk:

- valid-looking calls fail only under particular sequences;
- reusable components carry hidden environmental assumptions;
- error handling occurs too late to identify the violated precondition.

#### C5. Leaky or misnamed contracts

An interface, name, type, boundary, or documented promise implies a narrower, stronger, or different behavior than the implementation provides.

Typical risk:

- callers build incorrect mental models;
- internal details leak across boundaries;
- ownership or lifecycle is ambiguous;
- nominally equivalent operations carry materially different semantics.

### Five-phase pipeline

#### Phase 1 — Generate the declared manifest

Produce a stable, reproducible inventory of every unit included in the audit’s declared scope.

Required inputs:

- snapshot or revision anchor;
- declared roots or other unit sources;
- discovery adapter IDs and versions;
- selected unit granularity;
- explicit exclusions with reasons;
- optional declared non-file or cross-file units;
- shard strategy;
- cost and time budget;
- intended coverage tier.

Minimum manifest record:

| Field | Meaning |
|---|---|
| `unit_id` | Stable identifier within the anchored manifest |
| `locator` | Reproducible location or lookup key |
| `unit_type` | Adapter-defined structural type |
| `source_anchor` | Snapshot, revision, digest, or equivalent anchor |
| `discovery_adapter` | Adapter and version that emitted the unit |
| `inclusion_basis` | Why the unit belongs in scope |
| `enumeration_evidence` | Command, index, parser output, or rule that discovered it |
| `exclusion_reason` | Reason if represented as an explicit exclusion |
| `shard_id` | Assigned shard, if sharded |
| `coverage_tier` | `adapter-complete`, `declared-inventory`, or `sample` |

The manifest generator enumerates units. It does not decide whether a unit is correct or assign a violation class.

#### Phase 2 — Assign a per-unit disposition

Every manifested unit must receive exactly one terminal disposition:

- `clean`
- `candidate_violation`
- `not_applicable`
- `blocked_by_missing_evidence`
- `excluded_by_contract`

Rules:

- `candidate_violation` must name at least one of C1–C5 and provide a precise claim and locator.
- `clean` means no supported violation survived the primary examination; it does not prove absolute correctness.
- `not_applicable` requires a reason.
- `blocked_by_missing_evidence` is terminal for accounting but must never be counted as clean.
- `excluded_by_contract` requires a declared exclusion rule and reason.
- Bulk defaulting units to `clean`, `not_applicable`, or excluded without unit-level evidence is prohibited.

#### Phase 3 — Apply the internal completeness check

Before reporting disposition completeness:

- every manifest unit must have exactly one disposition;
- every disposition must reference a known manifest unit;
- duplicate unit IDs must be rejected;
- no shard may remain unreconciled;
- all exclusions must be explicit;
- missing or unexpected units must be surfaced;
- the snapshot anchor must still match or be explicitly reconciled.

This is an internal workflow completion condition. It is not a repository gate and does not prove semantic quality.

#### Phase 4 — Adversarially verify or refute candidates

Every candidate intended for the final ledger must undergo a distinct challenge pass.

The challenge should:

1. reconstruct the claimed contract or invariant;
2. locate the evidence supporting that interpretation;
3. attempt to produce a valid counterexample;
4. search for an existing enforcer, normalization path, documented exception, or ownership rule;
5. trace relevant reads, writes, transitions, and preconditions;
6. assess whether the observed behavior contradicts the actual contract rather than an assumed ideal;
7. assign one outcome:
   - `verified`
   - `refuted`
   - `indeterminate`
8. state what new evidence would reverse or resolve the result.

A salient initial observation cannot enter the verified ledger merely because it sounds plausible.

#### Phase 5 — Publish the audit outputs

Publish:

- the final ranked ledger;
- the complete manifest and disposition accounting;
- advisory standing checks;
- a remediation brief;
- the independent rigor-check record;
- scope, cost, coverage-tier, and uncertainty declarations.

“No verified violations” is an acceptable outcome.

---

### Manifest-generator contract

The core method must use a **pluggable unit-discovery interface** rather than embedding language knowledge.

#### Discovery adapter responsibilities

A discovery adapter must:

1. accept an anchored scope and explicit configuration;
2. enumerate units deterministically;
3. assign stable IDs and locators;
4. record how each unit was discovered;
5. declare its supported granularity;
6. expose exclusions and unsupported constructs;
7. report its claimed coverage tier;
8. avoid semantic violation judgments.

Conceptually:

```text
discover(
  snapshot_anchor,
  roots_or_sources,
  granularity,
  exclusions,
  adapter_config
) -> {
  adapter_id,
  adapter_version,
  coverage_tier,
  units[],
  unsupported_surfaces[],
  enumeration_evidence[]
}
```

#### Per-stack adapters

A localized adapter may use whichever deterministic facilities a stack provides, including:

- parser or abstract-syntax-tree indexes;
- compiler or language-service indexes;
- symbol or declaration documentation tools;
- schema and interface extractors;
- build-system inventories;
- infrastructure or resource graph enumerators.

Adapters live in the pilot overlay or consuming project until their generality and stability are established. Core methodology defines the interface, not the stack implementation.

#### Domain-free fallback

When no semantic adapter exists, the fallback may use:

- tracked-file inventory;
- explicit roots and include/exclude patterns;
- recognized configuration or schema files;
- explicit operator-declared contract units;
- conservative structural heuristics.

The fallback must record its limitations and normally claim `declared-inventory`, not `adapter-complete`.

A file manifest may be complete as a file inventory while remaining incomplete as a declaration, transition, or cross-unit contract inventory. That distinction must remain explicit.

#### Coverage tiers

| Tier | Allowed claim |
|---|---|
| `adapter-complete` | The adapter has deterministically enumerated every supported unit of its declared unit model within the anchored scope, with unsupported surfaces and exclusions disclosed |
| `declared-inventory` | Every unit emitted by the declared sources and rules has been accounted for, but the rules are not known to enumerate every semantic unit |
| `sample` | Only an explicitly selected subset was examined |

No tier establishes semantic correctness.

---

## 4. Outputs Contract

### A. Ranked ledger

Each verified or indeterminate finding should include:

| Column | Requirement |
|---|---|
| `finding_id` | Stable identifier |
| `rank` | Priority ordering with stated rationale |
| `violation_class` | One or more of C1–C5 |
| `affected_units` | Manifest unit IDs |
| `contract_claim` | The contract or invariant believed to apply |
| `holds` | `yes`, `no`, or `indeterminate` |
| `primary_evidence` | Precise evidence and locators |
| `adversarial_result` | `verified`, `refuted`, or `indeterminate` |
| `counterevidence` | Evidence considered against the finding |
| `confidence` | `high`, `medium`, or `low` |
| `impact` | Consequence if the finding holds |
| `likelihood` | Likelihood of the consequence |
| `risk_rank` | Combined, explainable priority |
| `cheapest_recheck` | Smallest reproducible verification |
| `standing_check_id` | Associated advisory check, if any |
| `recommended_disposition` | Repair, investigate, document, accept, or defer |

Refuted candidates remain available in the rigor record but must not be presented as surviving violations.

### B. Standing-check record

Each standing check should include:

| Column | Requirement |
|---|---|
| `check_id` | Stable identifier |
| `finding_id` | Finding that motivated the check |
| `purpose` | Condition the check observes |
| `scope` | Explicit files, units, or boundaries |
| `reproduction` | Deterministic command or procedure where feasible |
| `expected_signal` | Observable result |
| `limitations` | What the check cannot prove |
| `cadence` | Suggested manual or advisory cadence |
| `authority` | Always `advisory` |
| `presentation` | Report or `doctor`-WARN-shaped diagnostic only |

A standing check is a reusable observation recipe, not a gate.

### C. Remediation brief

The remediation brief should contain:

- finding IDs addressed;
- desired contract or invariant;
- affected units and boundaries;
- smallest coherent remediation slice;
- alternatives considered;
- compatibility and migration concerns;
- suggested verification;
- residual risk;
- explicit non-goals;
- whether a separate implementation plan is required.

It must not claim that remediation has been implemented.

### D. Rigor-check record

The rigor-check is a separate second-opinion pass over audit quality. It should include:

1. a sample of downgraded, refuted, or killed candidates;
2. the sampling method and size;
3. attempted reconstruction of each rejected claim;
4. agreement or disagreement with the original disposition;
5. any wrongly killed candidate restored for verification;
6. an explicit probe for a latent violation class that may have been under-sampled;
7. unexamined classes or surfaces;
8. conclusions about audit precision and blind spots.

Minimum columns:

| Column | Requirement |
|---|---|
| `sample_id` | Stable sample identifier |
| `candidate_id` | Candidate being checked |
| `original_disposition` | Original result |
| `selection_basis` | Random, risk-weighted, boundary-weighted, or class probe |
| `second_opinion` | Confirm, reverse, or indeterminate |
| `evidence` | Supporting locator and reasoning |
| `latent_class_probe` | Class or surface intentionally tested |
| `effect_on_ledger` | None, restore, downgrade, or escalate |

The rigor-check does not guarantee the absence of unsampled defects. It measures whether the primary pass is disposing of candidates responsibly and whether a latent class appears systematically neglected.

---

## 5. Authority Line

### Rule

> The contract/invariant audit **INFORMS**. It never gates, blocks, approves, promotes, releases, or transitions state on its own.

This applies to:

- findings;
- completeness reports;
- manifest-helper results;
- final ledgers;
- standing checks;
- remediation briefs;
- rigor-check conclusions.

The capability must not:

- block a commit or release;
- alter a commit-review verdict;
- return a doctor failure;
- silently create canonical backlog work;
- convert a standing check into enforcement;
- claim that manifest completeness proves semantic correctness;
- treat audit output as transition authority.

Standing checks may be presented as ordinary reports or as **doctor-WARN-shaped advisory diagnostics**. They must not produce a doctor failure or protected-transition denial.

If a later decision concludes that a particular invariant deserves enforcement, that requires a separate decision identifying:

1. the protected transition;
2. the deterministic predicate;
3. ownership of the validator;
4. false-positive and recovery behavior;
5. explicit authority to gate.

The audit itself does not grant that authority.

This follows the complexity-control and authority-line precedent: signals begin as advisory evidence, and only a separately justified deterministic validator attached to a protected transition may acquire gate authority.

---

## 6. Scope, Cost, and Anti-Coverage-Theater Guardrails

### Scope declaration

Every run must declare:

- anchored snapshot;
- roots or other unit sources;
- unit model and granularity;
- discovery adapters and versions;
- exclusions and reasons;
- non-file units;
- coverage tier;
- shard strategy;
- time or cost budget;
- completion and stop conditions.

### Execution modes

The capability should support three honestly named modes:

1. **Bounded slice**
   - A complete declared inventory of a deliberately narrow boundary.
   - Preferred for initial runs and pilot calibration.

2. **Tiered or sharded audit**
   - A larger declared inventory split deterministically into reconciled shards.
   - Each shard retains the same manifest and disposition contract.

3. **Sample**
   - A subset selected by an explicit sampling method.
   - Must remain labeled `sample`.
   - Cannot support a full-surface conclusion.

A bounded slice is not a sample if every unit in the deliberately bounded declared scope is enumerated. It still must not be described as repository-wide coverage.

### Cost controls

A run should:

- start with the narrowest surface capable of testing the hypothesis;
- estimate unit and shard counts before semantic examination;
- allow deterministic pause and resume;
- preserve manifest and snapshot identity across resume;
- report actual units manifested and examined;
- stop or reshape when the budget is exhausted;
- accept “no verified findings” as a valid result;
- avoid automatic invocation on edits, commits, doctor, or releases.

Recommended run metrics:

- units manifested;
- units examined;
- units blocked by missing evidence;
- explicit exclusions;
- candidate violations;
- verified, refuted, and indeterminate candidates;
- findings duplicated by commit-review;
- findings materially outside current-diff review;
- wall time and effort;
- rigor-check reversal rate.

### Stop and reshape conditions

Stop, narrow, or redesign the audit if:

- candidate volume is high but verified-survivor yield is low;
- the rigor-check repeatedly restores wrongly rejected candidates;
- most survivors duplicate commit-review findings;
- the fallback requires undisclosed semantic assumptions;
- unit IDs or anchors are unstable;
- exclusions grow without clear reasons;
- the cost of disposition dominates the resulting evidence;
- completion requires silently treating unknown units as clean.

### Anti-coverage-theater rule

> Manifest completeness is mandatory and must be demonstrated, never asserted. It proves accounting only for the declared unit model and anchored scope.

A run may use the label **full-surface** only when all of the following hold:

1. the exact surface is declared;
2. the unit model is stated;
3. the discovery adapter or fallback supplies reproducible enumeration evidence;
4. unsupported surfaces are disclosed;
5. exclusions are explicit and justified;
6. all shards reconcile;
7. the snapshot remains valid or is explicitly reconciled;
8. every manifested unit has exactly one terminal disposition;
9. the coverage tier supports the claim.

Even then, “full-surface” means complete enumeration under the declared unit model. It does not mean that all semantic contracts were discovered or that every verdict is correct.

If any condition fails, the run must be labeled:

- `bounded`;
- `partial`;
- `declared-inventory`; or
- `sample`,

as appropriate.

Silent sampling, silent budget truncation, undisclosed exclusions, and extrapolating semantic certainty from unit count are prohibited.

The large exemplar establishes only that the procedure can scale to hundreds of manifested units and that a full audit is expensive. No language, architecture, path, layer, or domain detail from that exemplar belongs in the reusable capability.

---

## 7. Decision Verdict and Closure

### Verdict

**`pursue-as-skill-overlay`**

Proceed, after operator sign-off, with an S1 pilot in a domain-neutral overlay pack:

- primary form: reusable skill;
- supporting mechanism: small deterministic manifest/completeness helper;
- authority: advisory only;
- core status: not approved for promotion;
- agent or command surface: deferred pending observed need.

### Confidence

**MEDIUM**

Structural confidence is strong because repository evidence clearly separates the proposed audit from commit-review, coverage-enforcement, and complexity-control by scope, cadence, data role, and authority.

Overall confidence remains medium because the decisive empirical evidence does not yet exist:

- real recurrence;
- heterogeneous manifest adequacy;
- survivor precision;
- duplication rate versus commit-review;
- cost-to-value ratio;
- ability to represent cross-file and non-file contracts without coverage theater.

### Contradictions

1. **Earlier versus later detection-volume evidence:** Earlier work considered additional architectural or blind-spot detection, but the later sampled probe found no wrongly dropped findings in its sample. The later empirical result controls and warns against equating more candidates with more value.
2. **Generic contract-drift uniqueness:** A claim that this capability is unique because it detects contract drift would contradict the commit-review evidence. Its distinctness instead comes from proactive existing-surface scope and disposition-complete audit structure.
3. **Blocking authority:** Any design in which the audit or its standing checks block transitions would contradict the repository’s authority-line rules and complexity-control precedent.
4. **Unqualified full-surface claims:** Calling an approximate, sampled, or weakly declared inventory “full surface” would contradict the limits of coverage-enforcement.

No contradiction was found that prevents an advisory overlay pilot.

### Remaining uncertainty

The main unresolved question is not whether the methodology is coherent. It is whether a real consuming repository can demonstrate enough recurring, net-new, adversarially surviving value to justify the cost and eventual core promotion.

The file-plus-explicit-cross-unit fallback is a synthesized design proposal, not an established repository fact. Its adequacy must be tested by the pilot.

### Load-bearing premise re-derivation

All supplied load-bearing repository premises were re-derived through the read-only research pass:

- the S2 overlay-pilot requirement;
- the skill lifecycle and authoring contract;
- coverage-enforcement Finding A;
- complexity-control’s advisory and over-raise precedent;
- commit-review’s changed-file, reactive, tiered behavior;
- the current absence of a contract/invariant-audit skill;
- the overlay-based skill extension model.

No supplied load-bearing premise remained unre-derived.

The unproven items identified above are predictions or proposed design choices, not repository premises being represented as facts.

### Sign-off gate

**Awaiting operator sign-off before authoring any skill, helper, agent, or command file.**

This decision brief authorizes no implementation. After sign-off, the next step is to prepare a narrowly scoped overlay-pilot authoring brief. Core promotion remains separately gated on positive real-repository S2 evidence.

### Sources

- `AGENTS.md` — “Extending the harness,” “Safety invariant: model output is a candidate, never transition authority,” and project rule that `templates/core/` remains domain-free.
- `.opencode/skills/skill-creator/SKILL.md` — skill placement, anatomy, repeatable-process criteria, triggers, and authoring workflow.
- `.opencode/skills/skill-creator/references/skill-lifecycle.md` — S2 overlay-pilot-then-promote lifecycle.
- `.opencode/skills/skill-creator/references/skill-design-vocabulary.md` — trigger precision and recurring skill context-load considerations.
- `researches/decisions/2026-07-14-skill-s2-promotion-gate.md` — required positive pilot evidence and high-confidence promotion threshold.
- `researches/decisions/skill-lifecycle-management.md` — skill lifecycle canon.
- `researches/decisions/2026-07-28-success-report-integrity-and-working-tree-stewardship.md` — Finding A, coverage-enforcement limits, property separation, and authority-line audit.
- `templates/core/.opencode/agents/commit-reviewer.md` — exact-file-list, anchored-diff, tiered-cascade, fail-fast, `BLOCK`, and `SPLIT` behavior.
- `researches/sources/2026-07-27-commit-review-model-study.md` — commit-review contract-drift strength and architectural-lens analysis.
- `researches/sources/2026-07-27-panel-slice-plan.md` — reconciliation of earlier detection proposals with later empirical evidence.
- `researches/sources/2026-07-28-s3-probe-dropped-blindspot.md` — sampled blind-spot result and precision implications.
- `researches/sources/2026-07-28-complexity-advisory-signal-triage.md` — complexity-control precision, over-raise, and silence-until-improved recommendation.
- `.opencode/skills/` — current rendered skill inventory; no existing contract/invariant-audit skill found.

> Note: the decision verdict, confidence, sign-off gate, core-promotion status,
> authority line, and next-action live in Section 7 above and are not repeated
> here. Implementation is authorized only after operator sign-off (see Section 7
> → Sign-off gate).
