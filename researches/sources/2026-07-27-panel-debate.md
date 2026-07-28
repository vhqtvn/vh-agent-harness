# Sources: Commit-Review Panel Model-Study Series — Panel Restructure Debate Resolution (O1/O2/O3)

**Date:** 2026-07-27 (study date; promoted to durable sources 2026-07-28)
**Topic:** debate over the five-leaf R4 blind-spot lens (O1) vs uniform four-leaf + blind-spot prompt amendment (O2) vs redirecting redundant capacity to a GPT-robust non-GPT R2 contract/data backstop (O3). Establishes the option set the rest of the series adjudicates.
**Series:** [`./2026-07-27-commit-review-panel-model-study-series.md`](./2026-07-27-commit-review-panel-model-study-series.md) (commit-review panel model-study
series, 2026-07-27 → 2026-07-28).

## Provenance

- **Source type:** debate (option-set resolution).
- **Evidence class:** Class-A — reasons over the same DB-cited corpus; read-only reasoning, no edits.
- **Original location:** `tmp/agent-runs/review-model-comparison/2026-07-27-panel-debate.md`
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
  `git log -1 --format=%H -- researches/sources/2026-07-27-panel-debate.md`.
- **Status:** durable source packet. Internal `tmp/` cross-references to peer memos
  resolve to the promoted canon paths listed in the series index above.
- **Faithfulness:** the body below the `---` delimiter is a byte-for-byte verbatim copy of the original `tmp/` study; only this provenance header is new — the study's own title, internal provenance, and cross-references to peer memos are unchanged.

---

# Panel Restructure — Debate Resolution

**Scope:** read-only evidence review and targeted re-querying. This memo uses
aggregate statistics only. `EMPTY` remains a transport failure and is excluded
from every quality inference.

## Debate packet

### Problem frame

- **Claim (assumption):** Choose the use of the redundant review capacity while
  preserving GPT-independent *fail-closed operation*. This means a non-GPT
  review attempt and gate hold remain available after GPT loss; it does **not**
  mean that present non-GPT models must equal GPT's contract/data catch rate.
- **Claim (fact):** The proposed five-leaf R4 design adds a leaf and requires
  class ownership plus per-class quorum/escalation rather than the current
  uniform-lens aggregation.

### Criteria

| Criterion | Importance | Claim type |
|---|---|---|
| Demonstrated net-new upheld coverage, especially true blind spots | critical | preference |
| GPT-less fail-closed operation, including the weak contract/data path | critical | preference |
| Aggregation authority and safety: lenses propose; coordinator/gate dispose | important | constraint |
| Runtime and reshape cost | important | preference |
| Prompt simplicity | nice_to_have | preference |

### Options

| ID | Option | Claim type |
|---|---|---|
| O1 | Five leaves: retain the proposed dedicated R4 blind-spot lens. | proposal |
| O2 | Retain four uniform leaves and add the blind-spot classes to every prompt. | proposal |
| O3 | Redirect the redundant capacity to a GPT-robust, non-GPT R2 contract/data backstop; keep any blind-spot prompt work separately attributable. | proposal |

### Evidence register

| ID | Evidence | Claim type |
|---|---|---|
| E1 | Coordinator corpus: blast radius has 0 upheld findings; performance 3; concurrency/race 2; complexity 22. The present prompt does not meaningfully cover the first three classes. | fact |
| E2 | Contract drift (208/293) and data integrity (64/82) are high-yield upheld classes and have a GPT-heavy model fingerprint. | fact |
| E3 | In v2, non-empty rounds co-presenting at least one GPT and one non-GPT leaf: 1,326 rounds, of which 394 had an upheld finding; GPT was the only upholder in 130/394 = **33.0%**. Aggregate is 150/435 = **34.5%**. | fact |
| E4 | In that controlled subset, GPT leaves supplied 381 upheld leaf-findings and non-GPT leaves 520; `gpt-latest:free` supplied 269 upheld findings, versus 277 in the whole scoreable corpus. | fact |
| E5 | Aggregate matched-finding upheld precision is 277/646 = **42.9%** for `gpt-latest:free` and 125/435 = **28.7%** for `gpt-5.5:free`. Persisted round data does not retain co-present matched-finding denominators, so controlled precision cannot be calculated from it. | fact |
| E6 | O1 adds about 25% leaf runtime and requires the class-owner/per-class-quorum redesign. Its claimed R4 net-new yield is prospective and has not been A/B tested. | fact |

## Q1 — Dedicated R4 versus O2/O3

### Evidence

- **Claim (fact, E1):** There is a genuine prompt-coverage gap, not evidence
  that one current model is uniquely bad at blast-radius, performance, or
  concurrency review. The counts are too low to estimate the yield of a new
  R4 prompt.
- **Claim (fact, E2):** The loss most clearly evidenced under GPT absence is
  contract/data-integrity coverage, where the current non-GPT path is weak.
- **Claim (fact, E6):** O1 has a certain structural cost—one more leaf and a
  non-uniform aggregation redesign—but only predicted incremental yield.

### Competing positions

**O1 — dedicated R4 lens.**

- **Claim (prediction):** A single explicitly scoped R4 prompt can concentrate
  attention on the classes in E1 better than a model swap or a diluted
  all-prompt amendment.
- **Claim (fact, E1):** A prompt intervention is necessary to solicit these
  classes; model diversity alone does not demonstrate coverage of a class the
  prompts rarely request.
- **Counterargument (fact, E6):** The incremental upheld yield is unmeasured,
  while the latency and aggregation cost are unavoidable. A single owner is
  safe only if class ownership, cross-lens escalation, and quorum tuning are
  proven in fixtures; otherwise it can produce either a unilateral practical
  blocker or an under-reviewed finding.

**O2 — four uniform leaves plus all-prompt amendment.**

- **Claim (fact, E1):** It directly addresses the observed prompt gap with no
  fifth leaf or aggregation reshape.
- **Claim (prediction):** Its lower concentration may bury blind-spot signal in
  broad prompts, especially in high-volume/noisy leaves. The corpus cannot
  quantify this attention-dilution effect.
- **Counterargument (fact, E2):** It does not directly shore up the most
  consequential demonstrated GPT-less degradation: contract/data coverage.

**O3 — non-GPT R2 backstop.**

- **Claim (fact, E2):** It targets the only high-yield, GPT-heavy weak point
  identified by the corpus, and strengthens the required non-GPT fallback
  rather than relying on a prospective blind-spot yield.
- **Claim (prediction):** A sharper non-GPT R2 prompt/routing or redundant
  non-GPT R2 review will improve GPT-less upheld contract/data coverage. This
  is not established by the existing corpus.
- **Counterargument (fact, E1):** O3 does not itself close the R4 blind-spot
  classes. Those remain a prompt-work obligation, not a reason to declare the
  panel complete.

### Resolved position

**Call: lean O3 as the redundant-slot decision; use a narrowly measured O2
prompt amendment as a separate follow-on; do not approve O1/R4 as the default
S3 shape yet. Confidence: medium.**

The decisive distinction is evidence readiness. O1 is the most plausible
answer to the R4 gap, but it trades a measured architectural cost for an
unmeasured benefit. O3 points capacity at the demonstrated GPT-less weakness
(E2), while O2 can cheaply test whether prompting alone produces blind-spot
findings without entangling that test with a fifth leaf and new quorum rules.

This is **not** a claim that blind spots are unimportant. It is a claim that
their observed scarcity is consistent with both a valuable unsought class and
a low underlying true-defect rate; the corpus cannot distinguish those causal
explanations.

**Flip conditions:**

1. **Flip to O1** if a fixed-diff, same-model A/B shows a dedicated R4 lens
   creates a material number of *net-new upheld* blast-radius/concurrency/
   performance/complexity findings over O2, with acceptable false-positive and
   review-latency cost; and class-owner/escalation fixtures demonstrate that
   an R4 proposal is never transition authority.
2. **Prefer O2-first** if its all-prompt A/B captures substantially the same
   upheld R4 findings as a dedicated lens without materially lowering precision
   or swamping coordinator load.
3. **Abandon/limit O3** if a GPT-removed fixture evaluation shows no meaningful
   non-GPT R2 uplift. That result does not prove O1; it triggers the R4 probe.

### Implication for the slice plan

**S3 does not proceed as briefed.** Keep S1/S2 only as optional additive,
configuration/fixture preparation; do not reshape live fan-out to five leaves
until a pre-S3 probe resolves O1 versus O2 and observes O3's GPT-less effect.

The probe must run the same fixed review inputs under: (a) four-leaf baseline,
(b) O2, (c) O3 with GPT unavailable, and (d) dedicated R4. Primary measures:
net-new coordinator-upheld findings by class, GPT-removed coverage, and review
latency. It must separately attribute prompt changes from slot changes. O1
only earns the S3/S4 aggregation redesign if it wins that comparison.

## Q2 — GPT unique-upheld coverage: capability or participation artifact?

### Evidence

- **Claim (fact, E3):** Controlling for the key participation variable—both
  GPT and non-GPT are present in the same parent round—changes GPT-only
  upheld-round share from **34.5%** (150/435 aggregate upheld rounds) to
  **33.0%** (130/394 co-present upheld rounds). The reduction is **1.5
  percentage points**, or about 4% relative to the aggregate estimate.
- **Claim (fact, E4):** GPT participation is not confined to a tiny residual
  slice: `gpt-latest:free` has 269 of its 277 upheld findings in the co-present
  subset. This supports the relevance of the controlled subset for its actual
  output population.
- **Claim (fact, E5):** The exact controlled *matched-finding* precision cannot
  be reported honestly from persisted round data. It retains upheld counts but
  not the matched denominators. Aggregate precision remains 42.9% for
  `gpt-latest:free`; it is not a controlled precision estimate.

### Competing positions

**Capability-dominance position.**

- **Claim (fact, E3):** The controlled estimate remains roughly one third of
  all upheld rounds, nearly the aggregate value. Thus participation/recency
  cannot explain most of the observed 34.5% loss.
- **Claim (fact, E5):** The high aggregate GPT precision is consistent with,
  but does not by itself prove, that this unique coverage is capability rather
  than calendar timing.

**Recency/participation-artifact position.**

- **Claim (fact, E3):** The controlled estimate is lower, so some aggregate
  inflation is present or plausible.
- **Claim (assumption):** Co-present rounds may still differ by change mix,
  repository, prompt version, or model-version mix; same parent round removes
  leaf participation confounding but does not prove random assignment.
- **Counterargument (fact, E3):** Those residual confounders cannot support the
  stronger claim that GPT's dominance is merely participation volume. The
  controlled share is still 33.0%.

### Resolved position

**Call: GPT dominance is predominantly real capability, not a
recency/participation artifact. Best estimate: GPT-only coverage is about
33% of upheld rounds when competing directly with non-GPT leaves, rather than
34.5% aggregate. Confidence: medium.**

The defensible recalibration is therefore **about 33 percentage points of
upheld-round loss**, not a claimed exact 34.5 points, if all GPT-class models
vanish. In practical terms, the original degradation estimate is slightly
overstated by available participation control, but not reversed: roughly
96% of the aggregate percentage-point estimate remains.

The controlled sample is **1,326 co-present v2/non-empty rounds and 394
upheld rounds**. The precision sub-question is unresolved, not negative:
there is no preserved controlled matched-finding denominator from which to
test whether GPT's 42.9% `gpt-latest:free` precision is inflated by absent
weaker models. Do not substitute GPT upheld-findings per participating leaf
for precision.

**Flip conditions:**

1. Re-derive controlled matched-finding denominators from the live read-only
   session data; if controlled GPT-only coverage or matched precision falls
   materially below the current 33.0% estimate after stratifying by repository,
   prompt revision, and model ID, treat more of aggregate dominance as artifact.
2. If repeat co-present cohorts consistently stay near one third, promote the
   33% figure from best estimate to a stable degradation baseline.

### Implication for the slice plan

The panel must retain GPT as a high-value primary block-finder **and** retain a
non-GPT R1′/fallback path so GPT is not a single point of operational failure.
Recalibrate brief §3 from “34.5% exact loss” to “approximately 33% controlled
upheld-round loss, medium confidence.” Do not make any controlled-precision
claim until the matching-level co-present query is re-run and persisted.

## Cross-cutting objections and settled points

### Objections

- **OBJ-1 (option-level, confirmed):** O1 has a certain +1-leaf and
  aggregation-redesign cost but untested R4 net-new upheld yield (E6).
- **OBJ-2 (option-level, confirmed):** O3's desired non-GPT R2 uplift is also
  prospective; it needs the same fixed-input evaluation before implementation.
- **OBJ-3 (frame-level, rejected):** “GPT-independent fail-closed operation
  conflicts with GPT-heavy contract capability” is not a valid frame
  contradiction. The stated constraint requires survival/fail-closed holds,
  not equivalence after GPT loss; the brief explicitly labels degradation.

### Settled points

- **Claim (fact):** The 34.5% aggregate coverage result is not erased by
  participation control; the controlled estimate is 33.0% (E3).
- **Claim (fact):** Exact co-present precision remains an evidence gap (E5).
- **Claim (preference):** No lens may independently transition a review;
  coordinator and commit-gate disposition remain authoritative.

## Refined open-research list

1. **R4 marginal yield:** Does dedicated R4 beat all-prompt amendment on
   net-new coordinator-upheld blind-spot findings on fixed inputs?
2. **Non-GPT R2 uplift:** What specific prompt/routing/redundancy change makes
   contract/data detection materially stronger with GPT unavailable?
3. **Controlled precision:** Re-run/persist matched-finding denominators in
   GPT/non-GPT co-present rounds; stratify by prompt/model version and neutral
   repository cohort.
4. **Aggregation calibration:** What owner-plus-R1/R1′ quorum and escalation
   rule prevents both noisy unilateral holds and cross-lens outvoting?
5. **R4 scope boundary:** Coordinate complexity detection with the separate
   complexity-control work without importing its thresholds into this panel.

## Net recommendation

The five-leaf restructure does **not** yet stand as an evidence-ready S3
commitment. Redirect the immediate redundant-capacity decision to **O3**, a
non-GPT R2 backstop, because GPT's approximately 33% controlled unique coverage
is real and contract/data is the demonstrated GPT-less weakness. Run **O2** as
a low-structural-cost, separately measured blind-spot prompt experiment. R4
remains a credible candidate—not rejected on theory—but its prospective yield
does not currently justify the honest +1-leaf runtime cost and class-ownership/
per-class-quorum reshape. GPT remains primary but never a single operational
point of failure.

## Persist-guard compliance

This memo contains aggregate model statistics only: no consumer-repository
name/path, consumer file path, code, or verbatim finding text.
