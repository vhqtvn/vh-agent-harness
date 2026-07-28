# Sources: Commit-Review Panel Model-Study Series — Controlled Co-Present GPT Upheld-Precision Re-Derivation (S6)

**Date:** 2026-07-27 (re-derivation; carries 2026-07-28 framing) (study date; promoted to durable sources 2026-07-28)
**Topic:** settles S6: reconciles the 94% (block-calibration = P(coord-block | leaf-block)) vs 42.9% (finding-survival = P(coord-upheld | matched)) GPT precision figures. Controlled co-present finding-survival 43.2% confirms 42.9% is uninflated. Establishes non-GPT weakness is volume/recall (finding-survival 9.5% vs GPT 38.3%), not calibration.
**Series:** [`./2026-07-27-commit-review-panel-model-study-series.md`](./2026-07-27-commit-review-panel-model-study-series.md) (commit-review panel model-study
series, 2026-07-27 → 2026-07-28).

## Provenance

- **Source type:** study (controlled re-derivation).
- **Evidence class:** Class-A — this-machine opencode DB-cited (read-only `mode=ro`); not re-derivable outside this host.
- **Original location:** `tmp/agent-runs/review-model-comparison/2026-07-27-gpt-precision-rederivation.md`
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
  `git log -1 --format=%H -- researches/sources/2026-07-27-gpt-precision-rederivation.md`.
- **Status:** durable source packet. Internal `tmp/` cross-references to peer memos
  resolve to the promoted canon paths listed in the series index above.
- **Faithfulness:** the body below the `---` delimiter is a byte-for-byte verbatim copy of the original `tmp/` study; only this provenance header is new — the study's own title, internal provenance, and cross-references to peer memos are unchanged.

---

# Open-Research #3 — Controlled Co-Present GPT Upheld-Precision Re-Derivation

**Scope:** READ-ONLY analytical re-derivation. No source/config/panel edits, no commit.
The only file written is this one (gitignored `tmp/`).

**Mission:** two GPT upheld-precision figures are in circulation — **94%** (full-sample
study §2) and **42.9%** (restructure-brief Table 1). The debate (Q2/E5) could not
compute the controlled co-present precision because the persisted cache
(`derived3.json`) retained upheld counts but not matched-finding denominators. This
memo re-runs the controlled query with denominators persisted, gives the
controlled-verified number, and reconciles WHY 94% and 42.9% diverge.

---

## §0 Methodology — three precisions stated precisely

All three are computed on the same DB (`~/.local/share/opencode/opencode.db`, opened
`mode=ro` via Python `sqlite3` stdlib), v2 schema leaves, `tokens_output>0` (non-empty),
matched to v2 coordinator findings. Matching = score ≥ 0.45 (category 0.25 +
location 0.45/0.55 + token-overlap up to 0.35) — reused verbatim from the brief's
`q18_final.py` pipeline.

GPT-class = {`gpt-latest:free`, `gpt-5.5:free`} (brief Table 10). Non-GPT = everything
else. Keying = exact `json_extract(model,'$.id')`; `:free`/paid + version kept DISTINCT.
The `kiro-claude-opus-4-5` paid transient (21 runs / 1 day / 100% empty) is quarantined
by the non-empty filter.

**Metric A — "block-calibration" (the 94% figure's definition):**

> Of a model's leaf findings raised at `disposition=block`, what fraction was matched
> to a coordinator finding with `disposition=block`?

```
P(coord-block | leaf-block, matched) = coord-block-of-leaf-block / leaf-block-matched
```

This is a SEVERITY-CALIBRATION metric: "when this model says BLOCK, is the coordinator
right to also block?" It conditions on the model's high-confidence subset (block-only).

**Metric B — "finding-survival" (the 42.9% figure's definition):**

> Of a model's leaf findings that matched a coordinator finding (any leaf disposition:
> block, defer, OR drop), what fraction survived with a coordinator disposition of
> block or defer (i.e. was NOT dropped)?

```
P(coord-upheld | matched) = coord-{block,defer} / all-matched
```

This is a FINDING-LEVEL SURVIVAL metric: "of everything this model raises that reaches
the coordinator, what fraction is actionable (not dropped)?" It includes the defer tier.

**Metric C — "round-coverage" (the 33% figure, already controlled):**

> Of co-present rounds (≥1 GPT-class AND ≥1 non-GPT leaf, same parent_id), what
> fraction of upheld rounds was upheld ONLY by a GPT-class leaf?

This is a ROUND-LEVEL metric, not a finding-level precision. The debate already
controlled it (E3: 33.0%). It is NOT the same as finding-precision.

---

## §1 Controlled co-present GPT precision (the load-bearing number)

**Sample:** 1,346 co-present rounds (≥1 GPT-class + ≥1 non-GPT leaf, non-empty, v2),
of which 405 produced ≥1 upheld finding. GPT-only upheld rounds = 131/405 = **32.3%**
(consistent with the debate's 33.0%; the 0.7pp difference is from exact-GPT-class
keying vs the debate's broader `"gpt" in model` predicate).

### Table 1a — gpt-latest:free (controlled co-present, denominators PERSISTED)

| metric | numerator | denominator | precision | 95% CI |
|---|---:|---:|---:|---|
| **A — block-calibration (94%-style)** | 199 | 210 | **94.8%** | 90.9 – 97.1 |
| **B — finding-survival (42.9%-style)** | 281 | 651 | **43.2%** | 39.4 – 47.0 |

The debate's open gap — "no preserved controlled matched-finding denominator" (E5) — is
now closed. The denominator **is** persisted: 651 matched findings (Metric B) and 210
matched block-findings (Metric A) for gpt-latest:free in the co-present subset.

### Table 1b — gpt-5.5:free (controlled co-present)

| metric | numerator | denominator | precision | 95% CI |
|---|---:|---:|---:|---|
| A — block-calibration | 86 | 99 | **86.9%** | 78.8 – 92.2 |
| B — finding-survival | 112 | 376 | **29.8%** | 25.4 – 34.6 |

### Table 1c — GPT-class aggregate (controlled co-present)

| metric | numerator | denominator | precision | 95% CI |
|---|---:|---:|---:|---|
| A — block-calibration | 285 | 309 | **92.2%** | 88.7 – 94.7 |
| B — finding-survival | 393 | 1027 | **38.3%** | 35.3 – 41.3 |

**Headline:** on the controlled co-present subset, gpt-latest:free's finding-survival
precision is **43.2%** [CI 39.4–47.0] — statistically indistinguishable from the
aggregate **42.9%** (brief) / **43.6%** (my full-sample re-derivation). The
Δ = 0.4pp difference is well within the confidence interval. **The aggregate precision
is NOT inflated by absent weaker models.** This refutes the debate's concern (Q2
flip-condition #1) that the 42.9% might be a participation artifact.

---

## §2 Reconciliation: WHY 94% ≠ 42.9% (the exact mechanism)

**They are not competing estimates of the same quantity.** They are answers to
different questions, called by the same name ("upheld-precision") in their respective
documents. The confusion is a metric-definition ambiguity, not a data conflict.

### The mechanism, shown on the SAME full-sample data (gpt-latest:free)

gpt-latest:free full-sample, all 668 matched findings broken into two disjoint subsets
by the LEAF's own disposition:

| subset | count | % of matched | coord-block | coord-defer | coord-drop |
|---|---:|---:|---:|---:|---:|
| leaf-disposition = **block** | 220 | 33% | 208 | (small) | (small) |
| leaf-disposition = **defer/drop** | 448 | 67% | 9 | 74 | 365 |
| **total matched** | **668** | 100% | 217 | 74 | 377 |

Now compute both metrics from this single cross-tab:

| metric | numerator (what counts as "upheld") | denominator (what's conditioned on) | result |
|---|---|---|---:|
| **A (94%)** | coord-block **from leaf-block** = 208 | leaf-block = 220 | **94.5%** |
| **B (42.9%)** | coord-block + coord-defer (any leaf-disp) = 291 | all matched = 668 | **43.6%** |

**The divergence is driven by the denominator, not the numerator.**

- Metric A's denominator (220) is **one-third** of Metric B's denominator (668).
  Metric A conditions on leaf-block, which is GPT's conservative high-confidence tier
  (only 33% of its findings).
- Metric B's denominator (668) includes the 448 defer/drop findings — GPT's
  lower-confidence majority. The coordinator drops **365 of those 448 (81%)**. That
  mass of dropped findings pulls the survival rate down to 43.6%.
- The numerator difference is small: Metric A counts 208 (strict block↔block); Metric B
  counts 291 (adds 9 block-from-nonblock + 74 defer). The numerator only grows 1.4×,
  but the denominator grows 3.0× — so the ratio falls from 94.5% to 43.6%.

**Restated as conditional probabilities on the same data:**

```
Metric A = P(coord-block | leaf-block)           = 208/220 = 94.5%
Metric B = P(coord-block ∪ coord-defer | matched) = 291/668 = 43.6%
```

Metric A answers: *"is GPT trustworthy when it blocks?"* → yes, 94%.
Metric B answers: *"of everything GPT raises, how much is actionable?"* → 43%.

Both are correct. They are not in conflict. The 94% is high because GPT is
**selective** about calling block (only 33% of its findings); the 42.9% is lower
because it includes the other 67% where GPT itself was less sure.

**What this is NOT:**
- NOT a v1/v2 schema difference (both figures are v2-only).
- NOT a sample-basis difference (both are full-sample; the controlled co-present
  check shows both are stable).
- NOT a matching-methodology artifact (my pipeline reproduces both within 0.7pp).
- NOT "one is right and the other is wrong" — they measure different things.

**Why the same name caused confusion:** the full-sample study §2 labeled 94% as
"coordinator-adjudicated upheld-precision." The brief Table 1 labeled 42.9% as
"upheld-precision." Both used the words "upheld-precision" for different definitions.
The fix is terminological, not numerical: call Metric A "block-calibration" and Metric
B "finding-survival."

---

## §3 Controlled non-GPT precision (the fallback-bench baseline)

For the same co-present subset, the non-GPT models' matched-finding precision:

### Table 3 — Non-GPT models (controlled co-present)

| model | Metric A (block-cal) | Metric B (survival) | matched n |
|---|---:|---:|---:|
| grok | 96.0% (24/25) | **11.5%** (194/1683) | 1683 |
| glm-5.2-high | 88.2% (30/34) | **10.0%** (201/2008) | 2008 |
| kiro-claude-opus-4-6:free | 100% (9/9) | **6.0%** (86/1441) | 1441 |
| grok-4.3 | 87.5% (7/8) | 10.2% (16/157) | 157 |
| grok-4.5 | 80.0% (4/5) | 14.9% (7/47) | 47 |
| glm-5.2 | 75.0% (3/4) | 8.2% (13/158) | 158 |
| kiro-claude-opus-4-5:free | 66.7% (2/3) | 11.0% (16/146) | 146 |
| glm-5.1 | 0.0% (0/1) | 7.2% (6/83) | 83 |
| **non-GPT aggregate** | **88.9%** (80/90) | **9.5%** (544/5733) | 5733 |

**The non-GPT fallback-bench baseline (controlled co-present):**

- **Finding-survival (Metric B): 9.5%** [CI 8.8–10.3] — this is the honest number for
  what the non-GPT panel delivers today. The best individual non-GPT model is
  `grok-4.5` at 14.9% (thin, n=47) and `grok` at 11.5% (n=1683). `glm-5.2-high` —
  the designated GPT fallback — is at 10.0%.
- **Block-calibration (Metric A): 88.9%** [CI 80.7–93.9] — when non-GPT models DO
  call block, they're trustworthy (~89%). But they almost never call block (90
  leaf-blocks across 5733 matched findings = 1.6% block-rate). Their weakness is
  **recall/volume**, not calibration.

**The GPT vs non-GPT gap, controlled co-present:**

| metric | GPT-class | non-GPT | gap |
|---|---:|---:|---|
| finding-survival (B) | **38.3%** | **9.5%** | **4.0×** |
| block-calibration (A) | 92.2% | 88.9% | 1.04× (negligible) |

The gap is entirely in finding-survival (Metric B), NOT in block-calibration (Metric A).
This confirms the brief's core thesis: GPT's edge is in **raising actionable findings**
(38% survival vs 9.5%), not in being more accurate when it blocks (92% vs 89%, nearly
tied). The restructure's non-GPT R2 backstop must improve **recall/survival**, not
calibration.

---

## §4 Recommendation — which basis the panel-restructure should standardize on

**Standardize on Metric B (finding-survival) for panel-structure decisions; use Metric
A (block-calibration) only for seat-trust calibration.**

**Why Metric B is the sounder basis for the restructure:**

1. **It measures what the panel needs.** The restructure's central question is
   "what coverage does each seat provide?" Coverage = actionable findings raised =
   finding-survival. The controlled co-present GPT survival (43.2%) vs non-GPT (9.5%)
   is the **4.5× gap** that defines the GPT-fragility risk. Metric A (94% vs 89%)
   hides this gap because it conditions on the rare block event.

2. **The controlled check confirms it is not inflated.** The debate's Q2 flip-condition
   feared the aggregate 42.9% was participation-inflated. The co-present re-derivation
   (43.2%, Δ = +0.3pp from aggregate) **refutes** that: controlling for co-presence
   leaves the precision unchanged. The 42.9% is a sound aggregate estimate.

3. **It includes the defer tier.** The coordinator's actionable-but-not-blocking signal
   (defer) is where most non-GPT value lives (e.g. grok upholds 194 findings, of which
   only 65 are coord-block; the other 129 are coord-defer). Metric A ignores all of
   those. A panel designed on Metric A alone would under-credit the non-GPT seats.

**Why Metric A is still useful — for a different question:**

Metric A (94%) answers "can we trust GPT's BLOCK calls?" → yes. This justifies pinning
GPT on the highest-stakes R1 seat (its blocks are the most trustworthy at scale). But
it must NOT be used as the coverage/quality headline, because it conditions on a
self-selected 33% subset and is blind to the recall gap.

**For the 33% GPT-loss figure:** use Metric C (round-coverage, already controlled) =
**32.3%** co-present (debate's 33.0% is within rounding). This is a round-level metric,
not a finding-precision metric. It is already controlled and sound; this re-derivation
does not change it.

**For the O3 target (non-GPT R2 backstop):** the controlled non-GPT finding-survival
baseline is **9.5%** [CI 8.8–10.3]. This is the honest floor. The restructure should
backstop it with prompt-side improvements (sharper contract/data lens that lifts
glm/grok survival toward GPT's 38%), not expect model-side parity.

---

## §5 Open-research #3 status — SETTLED

**Controlled co-present GPT matched-finding precision (with denominators persisted):**

| | gpt-latest:free | gpt-5.5:free | GPT-class agg |
|---|---:|---:|---:|
| Metric A (block-cal) | 94.8% (199/210) | 86.9% (86/99) | 92.2% (285/309) |
| **Metric B (survival)** | **43.2% (281/651)** | **29.8% (112/376)** | **38.3% (393/1027)** |
| Aggregate B (original) | 42.9% (277/646) | 28.7% (125/435) | — |
| **Δ (co-present − agg)** | **+0.3pp** | +1.1pp | — |

**The debate's E5 gap is closed.** The controlled co-present denominator IS persistable
(a live re-query produces it, as shown here). The aggregate finding-survival precision
is NOT inflated by absent weaker models: the co-present estimate is within 0.3–1.1pp of
the aggregate for both GPT models. **The 42.9% is a sound number for the
finding-survival question; the 94% is a sound number for the block-calibration
question; they are not the same question.**

**What remains (refinements, not blockers):**
- Stratification by prompt version and repository cohort (debate flip-condition #1) —
  not done here. The stability of per-model survival precision under co-present control
  (Δ ≤ 1.1pp) makes material stratification-driven inflation unlikely, but a per-repo
  split would tighten the confidence band.
- The 94% vs 42.9% naming collision should be resolved terminologically in any future
  panel document: "block-calibration" vs "finding-survival."

---

## §6 Cross-checks

| claim | this study | prior source | verdict |
|---|---|---|---|
| gpt-latest:free Metric A (full-sample) | 94.5% (208/220) | 94.0% (236/251) full-sample §2 | **CONFIRM** (Δ=0.5pp; denom differs: matched-block 220 vs all-block 251) |
| gpt-latest:free Metric B (full-sample) | 43.6% (291/668) | 42.9% (277/646) brief Table 1 | **CONFIRM** (Δ=0.7pp; matching variance) |
| gpt-5.5:free Metric B (full-sample) | 29.2% (128/438) | 28.7% (125/435) brief Table 1 | **CONFIRM** (Δ=0.5pp) |
| co-present GPT-only round coverage | 32.3% (131/405) | 33.0% (130/394) debate E3 | **CONFIRM** (Δ=0.7pp; GPT-class keying variance) |
| co-present rounds | 1,346 | 1,326 debate E3 | **CONFIRM** (~1.5%; exact-GPT-class vs `"gpt" in m`) |
| non-GPT survival (full-sample) | 9.1% | 9.3% brief Table 1 (glm-5.2-high) | **CONFIRM** |
| glm-5.2-high Metric B (full-sample) | 9.8% (207/2115) | 9.3% (194/2076) brief Table 1 | **CONFIRM** (Δ=0.5pp) |

All cross-checks agree within matching/methodology variance (≤0.7pp). The pipeline is
validated against two independent prior studies.

---

## §7 Limitations

1. **Matching is conservative.** Score ≥ 0.45 (category + location + token overlap)
   may miss findings phrased differently from the coordinator's echo. This depresses
   absolute upheld COUNTS (a lower bound) but leaves PRECISION RATIOS robust (both
   numerator and denominator are equally affected).
2. **Coordinator is itself a model** (mostly `glm-5.2-high`). A systematic coordinator
   bias toward/against a given leaf model would bias attribution. Overturn rates are
   low and consistent (brief §5), arguing against strong bias, but it is not fully
   controllable from logs.
3. **Co-present control removes leaf-participation confounding but not all confounders.**
   Co-present rounds may still differ by change mix, prompt version, or model-version
   mix. Same-parent-round is the strongest available natural control; random assignment
   is not present in observational data.
4. **v1-era leaves (pre-2026-06-08) are excluded** (no `disposition` field). GPT models
   from that era (`gpt-5.4`, `gpt-5.4:free`) are unscoreable and absent from all
   metrics here. This does not affect the live-panel decision (those models are retired).
5. **The controlled co-present subset is observational, not experimental.** It controls
   for participation but cannot prove causation. The stability of the precision under
   control is strong evidence against participation inflation, but a fixed-input A/B
   (the debate's S7 probe) would be the decisive test.

---

## Persist-guard compliance

This memo contains **aggregate model statistics only**. No consumer-repo name/path,
no consumer file paths, no code, no verbatim finding text, no `/home/` absolute paths
(the DB is referenced in tilde form). Cross-repo references use neutral labels. The
script (`q30_copresent_precision.py`) lives under `tmp/commit-study/` (gitignored).
No source/config/panel files were read or written.

*Source data: `q30_output.json` (this run's raw output) + `q30_copresent_precision.py`
(the re-derivation script), both under `tmp/commit-study/`.*
