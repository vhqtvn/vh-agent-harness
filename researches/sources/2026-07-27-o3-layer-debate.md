# Sources: Commit-Review Panel Model-Study Series — O3 Layer Debate — Lens Volume vs Disposition/Gating

**Date:** 2026-07-27 (carries 2026-07-28 framing) (study date; promoted to durable sources 2026-07-28)
**Topic:** the O3 lens-volume-vs-disposition-gating debate. SPECIFIES the S3 sample design (stratified random, 48 dropped non-empty v2 matched findings: 20 performance / 20 concurrency / up to 4 each blast-radius and complexity) and the auditable real-wrongly-dropped / noise-correctly-filtered / indeterminate rubric that the S3 probe then executed.
**Series:** [`./2026-07-27-commit-review-panel-model-study-series.md`](./2026-07-27-commit-review-panel-model-study-series.md) (commit-review panel model-study
series, 2026-07-27 → 2026-07-28).

## Provenance

- **Source type:** debate (specifies the S3 sample design + rubric).
- **Evidence class:** Class-A — reasons over the same DB-cited corpus; read-only reasoning, no edits.
- **Original location:** `tmp/agent-runs/review-model-comparison/2026-07-27-o3-layer-debate.md`
  (volatile; lives under the gitignored `tmp/` tree, not committed canon; promoted to
  durable sources on 2026-07-28 so the series is discoverable and citable).
- **Promotion basis:** part of the commit-review panel model-study series. The series
  lived only under `tmp/` and was undiscoverable (a researcher searching for "the
  panel probe" could not find it); promoted wholesale so the S3 probe's over-raise
  corroboration and its upstream/downstream memos are citable canon, not tmp-only.
- **Consuming decision:** complexity-control over-raise policy. The S3 probe
  (`2026-07-28-s3-probe-dropped-blindspot.md`) is the corroborating evidence on whose
  basis the complexity-over-raise check was silenced. The consuming policy is owned by
  a concurrent session and is not in `researches/decisions/` at promotion time.
- **Promotion commit:** the commit that introduces this file. A committed file cannot
  embed its own introducing commit's hash without an amend (the hash depends on the
  file's bytes); resolve via
  `git log -1 --format=%H -- researches/sources/2026-07-27-o3-layer-debate.md`.
- **Status:** durable source packet. Internal `tmp/` cross-references to peer memos
  resolve to the promoted canon paths listed in the series index above.
- **Faithfulness:** the body below the `---` delimiter is a byte-for-byte verbatim copy of the original `tmp/` study; only this provenance header is new — the study's own title, internal provenance, and cross-references to peer memos are unchanged.

---

# O3 Layer Debate — Lens Volume vs Disposition/Gating

**Scope:** Read-only reasoning. No source, config, panel, or commit changes.

## 1. Branch A likelihood (empirical)

### Status: not yet estimated — the required hand classification is the decision gate

The corpus establishes the *pattern*, but not the truth-status of the dropped
findings. S6 shows that non-GPT BLOCK calls are well calibrated when made
(88.9%, 80/90) relative to GPT (92.2%, 285/309), while their finding-survival is
only 9.5% (544/5,733) versus GPT's 38.3% (393/1,027). The full-sample study also
records the suspicious raised-then-dropped populations: concurrency about 65
raised / effectively 0 upheld and performance about 244 / 2; it describes those
classes as prompt/gating gaps. Those observations make a disposition failure
plausible, but they do **not** establish that any dropped item was a real defect.

The requested live DB extraction could not be executed in this coordinator's
read-only command capability: `exec-ro` refuses Python by design and the normal
`exec` route is denied by the current command policy. Therefore no sample may be
represented as hand-classified. This is an evidence gap, not a negative result.

**Required sample before a layer decision:** stratified random sample of 48
dropped, non-empty, v2 matched findings: 20 performance, 20 concurrency, and up
to 4 each blast-radius and complexity (substitute proportionally if a stratum is
smaller). Key by exact `model.id`; retain `:free`/paid and versions separately;
exclude the paid `kiro-claude-opus-4-5` transient; do not use empty output as a
quality signal.

**Auditable classification rubric.** Classify each item against the reviewed
diff and its stated evidence, blinded to model where practical:

| Label | Rule |
|---|---|
| `real-wrongly-dropped` | The finding names a specific, reproducible changed-path defect; the defect could cause incorrect behavior, corruption, race, material latency/resource regression, or unsafe change impact; and the evidence supports at least `defer` or `block` under the stated review contract. |
| `noise-correctly-filtered` | The alleged failure is absent, speculative without a changed-path mechanism, pre-existing/out of scope, merely stylistic, or has no material consequence; dropping is justified. |
| `indeterminate` | The stored diff/evidence cannot establish either result. Do not force it into either denominator; report it separately. |

Have a second reviewer adjudicate disagreements. Report the stratum counts,
real/noise/indeterminate split, and Wilson intervals for `real/(real+noise)`;
the sample is directional rather than a population-causal proof.

**Current Branch A likelihood:** *plausible but unquantified; low confidence.*
The evidence warrants moving the probe forward, not declaring Branch A likely
enough to replace O3 today.

## 2. Debate packet

### Problem frame

- **PF1 (preference):** Preserve GPT-independence by construction: a non-GPT
  review path remains available after GPT loss; parity with GPT is not assumed.
- **PF2 (constraint):** The authority line remains `lens finds; coordinator and
  gate act`. A lens cannot independently transition a finding.
- **PF3 (fact, E1–E4):** Choose the next primary investment to improve
  GPT-loss resilience without treating a prospective lens prompt as proven.

### Criteria

| Criterion | Importance | Claim type |
|---|---|---|
| Increases *surviving*, actionable GPT-free contract/data coverage | critical | preference |
| Avoids scaling a demonstrably bad disposition decision | critical | preference |
| Preserves GPT-independence and the authority line | critical | constraint |
| Is evidence-ready and isolates causal effects | important | preference |
| Limits schema/merge and operational complexity | important | preference |

### Evidence register

| ID | Evidence | Claim type |
|---|---|---|
| **E1** | Controlled co-present S6: GPT/non-GPT block-calibration is 92.2%/88.9%, but finding-survival is 38.3%/9.5%. Non-GPT has a volume/survival problem, not demonstrated BLOCK-calibration failure. | fact |
| **E2** | Controlled co-present GPT-only upheld-round loss is 131/405 = 32.3%. | fact |
| **E3** | Contract drift and data integrity are high-yield and GPT-heavy; a non-GPT backstop is the identified GPT-loss weakness. | fact |
| **E4** | Blind-spot classes were raised then mostly dropped: concurrency about 65/~0 upheld and performance about 244/~2; blast-radius/complexity have very small bases. | fact |
| **E5** | O3's staged prompt targets only contract/data/spec alignment, retains a concrete-evidence bar, and predicts—not proves—higher surviving non-GPT volume. Its proposed owner-or-redundant treatment requires future merge support. | fact |
| **E6** | The approved probe already defines Branch A (real wrongly dropped → stop lens experiments and redesign disposition), B (noise → no gap), and C (true detection gap → lens comparison). | fact |

### Options considered

| ID | Option | Core claim |
|---|---|---|
| **O1 / P1** | **O3 now as approved** | A focused non-GPT contract/data prompt is the right immediate resilience lever; treat blind-spot disposition as separate. |
| **O2 / P2** | **Disposition/gating first** | The raised-then-dropped pattern means a broken filter is the likely root cause; do not invest in more detection first. |
| **O3 / P3** | **Probe-gated, survival-coupled O3** | Run the cheap disposition probe first, then run O3 only as a controlled experiment whose primary outcome is final survival, not leaf volume. |
| **O4** | **Parallel but independent tracks** | Start O3 implementation and the probe concurrently, accepting that the result may later invalidate O3. |

## 3. Per-position debate

### O1 / P1 — O3 now as approved

**Steelman.**

- **Claim (fact, E1):** The controlled gap is fourfold in survival, while
  calibration differs only 3.3 percentage points. A prompt designed to elicit
  more specific owned-class evidence targets the measured non-GPT failure mode
  better than model replacement.
- **Claim (fact, E2–E3):** The loss is material (32.3% GPT-only upheld rounds)
  and contract/data is the demonstrated high-yield, GPT-heavy exposure. O3 is
  targeted at the resilience problem actually measured.
- **Claim (fact, E5):** The real prompt is narrow: it suppresses unrelated
  categories, requires concrete evidence, and does not propose broad blind-spot
  volume. Therefore the performance/concurrency chatter is not automatically
  evidence that O3 itself will flood the coordinator.

**Weakness.**

- **Claim (prediction, E5):** A more aggressive non-GPT contract/data prompt
  will create *surviving* actionable findings. The corpus proves neither the
  uplift nor that the current low survival is prompt-recoverable rather than a
  capability or disposition limitation.
- **Claim (assumption, E1):** High BLOCK calibration transfers unchanged to the
  larger set O3 induces. That is reasonable but untested because O3 deliberately
  changes the selection threshold.
- **Claim (fact, E4/E6):** If Branch A is real, a volume-only success metric is
  misleading: the system may detect more real concerns but still dispose of
  them. O3 cannot be called the primary investment on leaf volume alone.

### O2 / P2 — disposition/gating first

**Steelman.**

- **Claim (fact, E4):** The leaf layer does raise many performance/concurrency
  findings, yet nearly none survive. This directly identifies disposition as a
  plausible high-leverage bottleneck for those classes.
- **Claim (fact, E6):** The existing plan itself specifies that Branch A stops
  lens experiments and routes the fix to disposition/gating (and, for
  complexity, its separate brief). This is not an invented coupling.
- **Claim (prediction, E1/E4):** If a material share of those dropped findings
  is real, fixing survival can recover value without demanding more leaf volume.

**Weakness.**

- **Claim (assumption, E4):** Raised-and-dropped means wrongly dropped. It could
  instead be correct filtering of noisy or non-actionable concerns. Aggregate
  calibration/survival statistics do not adjudicate the semantic correctness of
  a particular drop.
- **Claim (fact, E3/E5):** The observed Branch-A candidates are predominantly
  blind-spot categories, whereas O3 targets contract/data. The evidence does
  not show that a disposition defect, if present for performance/concurrency,
  also explains the GPT-free contract/data deficit.
- **Claim (prediction):** A policy change improves net quality. Without the
  hand sample, this could simply raise false blocking and violate the gate's
  safety role.

### O3 / P3 — probe-gated, survival-coupled O3

**Steelman.**

- **Claim (fact, E1–E3):** It retains O3's strongest rationale—measured,
  high-yield GPT-free contract/data weakness—without assuming its prompt-level
  mechanism works.
- **Claim (fact, E4–E6):** It respects the existing causal gate: settle whether
  drops hide real blind-spot defects before investing in a lens whose success
  would otherwise be read only at leaf volume.
- **Claim (preference):** It makes final disposition survival the primary O3
  outcome and leaf emissions a diagnostic. That preserves the authority line
  and makes a broken filter observable rather than silently optimized around.

**Weakness.**

- **Claim (prediction):** The probe's blind-spot result generalizes sufficiently
  to O3's contract/data scope. It may not; the appropriate disposition rule
  could be class-specific.
- **Claim (preference):** It delays an urgent resilience intervention by one
  cheap classification pass. If GPT-loss mitigation has an immediate deadline,
  that delay has opportunity cost.

### O4 — parallel independent tracks

**Steelman.**

- **Claim (preference):** Parallel work reduces calendar time when staffing
  permits and treats O3 and the disposition probe as distinct hypotheses.

**Weakness.**

- **Claim (fact, E5–E6):** O3 requires prospective measurement and may require
  a different gating treatment after Branch A. Starting it as an approved
  primary investment spends complexity before its causal dependency is settled.
- **Claim (prediction):** Parallelism is worthwhile. That cannot outweigh the
  risk of producing an uninterpretable O3 result unless the protocol freezes
  gate behavior and labels the work as shadow-only.

## 4. Objections, contradiction audit, and settled points

### Objections

| ID | Level | Target | Status | Claim |
|---|---|---|---|---|
| OBJ-1 | option | O1/P1 | confirmed | O3 efficacy is prospective; low current non-GPT survival does not identify prompt recoverability (E1, E5). |
| OBJ-2 | option | O1/P1 | confirmed | O3 cannot be evaluated from emitted BLOCK volume: its user value is final survival under coordinator/gate disposition (E5). |
| OBJ-3 | option | O2/P2 | confirmed | Raised-then-dropped counts do not prove that drops are wrong; the required semantic sample is missing (E4, E6). |
| OBJ-4 | option | O4 | confirmed | Uncontrolled parallel implementation risks wasted, causally uninterpretable work if Branch A changes the applicable gate (E5, E6). |

**Frame-level audit.** No valid frame-level trigger passed. The critic offered a
purported trigger using the label `S6`, but it was rejected: it did not cite an
evidence-register ID and E1 measures calibration/survival, not a demonstrated
contradiction of the authority-line or GPT-independence frame. The open question
is an ordinary layer/causal uncertainty, resolvable by the existing probe—not a
reason to replace the frame.

### Settled points

- **Claim (fact, E1):** Non-GPT's observed deficit is survival/volume, not
  confidence calibration among existing BLOCK calls.
- **Claim (fact, E2–E3):** A GPT-free contract/data backstop remains necessary;
  the 32.3% controlled GPT-only coverage loss is not erased.
- **Claim (fact, E4–E6):** Disposition is a credible alternative cause for
  blind-spot loss, but Branch A is unverified until dropped findings are judged
  against the actual diff/evidence.
- **Claim (constraint):** No outcome permits a lens to become transition
  authority. Any disposition adjustment remains coordinator/gate policy,
  domain-free core behavior plus per-repo class configuration as applicable.

## 5. Resolved position

### Recommendation: **lean O3 / P3 — probe first, then survival-coupled O3; demote O3 from approved primary implementation to a gated primary experiment.**

**Layer call:** The evidence supports neither a pure lens-first conclusion nor
a disposition-first conclusion. It supports a **coupled measurement sequence**:
the immediate next decision is disposition diagnosis; O3 remains the most
evidence-aligned contract/data resilience candidate *after* that diagnosis, but
must be assessed at the disposition/survival layer rather than leaf volume.

**Confidence: medium-low.** E1–E3 strongly support the need to improve
non-GPT contract/data resilience. The decisive layer choice is only low
confidence because E4 is counts of coordinator outcomes, not an independent
ground-truth audit of the dropped findings.

### Flip conditions

1. **Branch A—real wrongly dropped dominates:** pause O3 as currently shaped.
   Prioritize class-scoped disposition/gating redesign (and route complexity
   threshold policy to the complexity-control brief). Resume a shadow O3 only
   after the gate policy is fixed or held constant; evaluate survival under the
   new explicit rule. O3 may be unnecessary for the affected classes and must
   not be justified by volume alone.
2. **Branch B—noise correctly filtered dominates:** retain the gate; proceed
   with a narrow O3 A/B for contract/data. Do not broaden it to blind-spot
   classes merely because their raw raise counts are large.
3. **Branch C—detection gap:** keep O3 as the contract/data experiment and
   separately run the already-planned blind-spot lens comparison. Neither
   result proves the other because their class scopes differ.
4. **O3 A/B fails:** if it does not materially increase final contract/data
   survival with a stable evidence-supported precision floor, stop treating
   prompt volume as the remedy; investigate model capability/routing rather
   than reclassifying emissions as success.
5. **Urgency override:** if a real GPT-outage deadline makes waiting
   unacceptable, permit only a shadow, reversible O3 experiment with frozen
   disposition and no production authority change—not O3 as an unqualified
   primary deployment.

## 6. Sequencing recommendation and slice-plan implication

1. **Move S3 (probe Step 1) ahead of S2/O3.** Perform the stratified
   classification above. It is cheap, directly tests the disputed causal link,
   and is already the plan's strict decision gate for blind-spot classes.
2. **Branch immediately.**
   - **A:** disposition/gating policy and the complexity boundary become the
     primary investment; halt blind-spot lens expansion. Do not silently place
     threshold logic in a panel leaf.
   - **B:** retain current disposition; start O3 A/B.
   - **C:** retain O3 A/B for contract/data and start the separate blind-spot
     O2/R4 comparison.
3. **Run O3 only as a controlled, survival-coupled experiment.** Same non-GPT
   model, same labeled contract/data input set, baseline uniform prompt versus
   R2 prompt, with coordinator/gate policy frozen for the comparison. Primary
   metric: incremental *final block/defer survival* on owned classes with
   concrete evidence. Secondary: emitted volume and calibration. Do not use an
   owner rule to automatically convert O3 emissions into proof; that would
   confound the lens and disposition hypotheses.
4. **Only after a passing O3 experiment, decide whether S1/S2 structural work
   is warranted.** The required core must remain domain-free; class ownership,
   tuning, and any disposition policy belong in per-repo configuration. The
   coordinator/gate remains the authority.

**Result for the slice plan:** S2/O3 does **not** proceed as an implemented
primary investment. It proceeds as the first contract/data experiment after the
now-prioritized S3 Step 1 diagnostic. S3 moves up. The slice plan should be
restructured to make O3's behavioral closure final survival—not prompt output
or leaf BLOCK rate—and to preserve a class-specific causal record.

## 7. Refined open-research list

1. **Branch A sample (highest priority):** What share of dropped concurrency,
   performance, blast-radius, and complexity findings is genuinely actionable?
   Record stratum-specific uncertainty and reviewer agreement.
2. **Disposition scope:** If Branch A passes, which classes, severities,
   evidence requirements, and escalation path should change without weakening
   the authority line? Complexity thresholds remain owned by its separate brief.
3. **O3 causal A/B:** Does the real R2 prompt improve non-GPT final survival on
   contract/data over the same model's uniform baseline, while preserving a
   stated precision floor?
4. **Failure-mode split:** For non-surviving contract/data non-GPT findings,
   what fraction is prompt-recoverable versus capability-bound versus
   correctly-filtered? This determines whether a further lens iteration is
   justified.
5. **Blind-spot detection comparison (only Branch C):** On fixed inputs, does
   O2 or dedicated R4 produce net-new final surviving findings at acceptable
   noise/latency?
6. **Recency/transport:** Recheck model/prompt-version drift on the eventual
   prospective sample. Empty outputs remain transport failures and are excluded
   from quality ranking.

## Persist-guard compliance

Aggregate model statistics only; neutral `harness`/`consumer-A` convention;
no sensitive repository paths, code, verbatim finding text, or absolute
home-directory paths. No source/config/panel edits or commit were made.
