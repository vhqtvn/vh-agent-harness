# Sources: Commit-Review Panel Model-Study Series — O3 A/B Baseline — Non-GPT Contract/Data Finding-Survival

**Date:** 2026-07-28 (study date; promoted to durable sources 2026-07-28)
**Topic:** corpus-only baseline leg for the O3 A/B (no live model invocation). Isolates the contract/data-specific (contract_drift + data_integrity + spec_alignment) finding-survival slice that O3's tuned R2 lens prompt must beat in the live A/B. Establishes the precise non-GPT contract/data baseline against which O3 is measured.
**Series:** [`./2026-07-27-commit-review-panel-model-study-series.md`](./2026-07-27-commit-review-panel-model-study-series.md) (commit-review panel model-study
series, 2026-07-27 → 2026-07-28).

## Provenance

- **Source type:** study (A/B baseline, corpus-only).
- **Evidence class:** Class-A — this-machine opencode DB-cited (read-only `mode=ro`); not re-derivable outside this host.
- **Original location:** `tmp/agent-runs/review-model-comparison/2026-07-28-o3-ab-baseline-contract-data.md`
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
  `git log -1 --format=%H -- researches/sources/2026-07-28-o3-ab-baseline-contract-data.md`.
- **Status:** durable source packet. Internal `tmp/` cross-references to peer memos
  resolve to the promoted canon paths listed in the series index above.
- **Faithfulness:** the body below the `---` delimiter is a byte-for-byte verbatim copy of the original `tmp/` study; only this provenance header is new — the study's own title, internal provenance, and cross-references to peer memos are unchanged.

---

# O3 A/B Baseline — Non-GPT Contract/Data Finding-Survival

**Date:** 2026-07-28
**Scope:** READ-ONLY empirical re-derivation. No source/config/panel edits, no commit.
**Mission:** establish the precise non-GPT **contract/data** finding-survival number that O3's
tuned R2 lens prompt must beat in the live A/B. This is the A/B's **baseline leg** (corpus-only,
no live model invocation). S6 established the *overall* non-GPT finding-survival (9.5% vs GPT
38.3%) and showed the gap is volume/recall, not calibration; this memo isolates the
**contract/data-specific** slice (`contract_drift` + `data_integrity` + `spec_alignment`).

---

## Provenance & method

- **DB:** `~/.local/share/opencode/opencode.db`, opened read-only (`mode=ro`) via Python `sqlite3`.
- **Schema/keying:** exact `json_extract(model,'$.id')`; `:free`/paid + version distinct; non-empty
  basis only (`tokens_output>0`); the `kiro-claude-opus-4-5` paid transient (21/1-day/100%-empty)
  is quarantined by the non-empty filter. `n≥20` model floor.
- **Matching:** the proven score-based finding-match (≥0.45: category 0.25 + location 0.45/0.55 +
  token-overlap up to 0.35), reused **byte-identically** from `q30_copresent_precision.py` / S6
  (`2026-07-27-gpt-precision-rederivation.md`). Leaf finding → best coordinator finding within the
  same parent round; unmatched raised findings count in the volume denominator but not the survival
  denominator (same basis as S6).
- **Scope definition (load-bearing):** contract/data = leaf findings whose **raw category label**
  canonicalizes (3-bucket map) into `contract_drift`, `data_integrity`, or `spec_alignment`.
  - `spec_alignment` is kept **DISTINCT** from `contract_drift` for scope attribution (the data
    shows it is a heavily-used distinct label: 163 leaf / 183 coord findings), per the task's
    three-class scope and the "category-LABEL counts" rule (the §8 inflation bug — issue-text
    keyword matching — is NOT repeated here).
  - The **matcher** still uses the proven `canon_cat` alias (which folds `spec_alignment`→
    `contract_drift`) so leaf↔coordinator matching is identical to S6; only the *scope filter*
    uses the 3-bucket split. This means a leaf `spec_alignment` finding can match a coordinator
    `contract_drift` finding (semantically correct) while being attributed to `spec_alignment` by
    its own label.
- **Bench (non-GPT):** `grok`, `grok-4.3`, `glm-5.2-high`, `glm-5.1`, `kiro-claude-opus-4-6:free`
  (per task spec). `glm-5.2`, `grok-4.5`, `gemini`, `kiro-claude-opus-4-5:free` excluded (not in
  bench spec / thin).
- **GPT-class (the gap):** `gpt-latest:free`, `gpt-5.5:free` (same as S6/q30; gpt-5.4 is v1-era,
  unscoreable — no `disposition` field).
- **Validity oracle:** coordinator's per-finding `disposition` — `block`/`defer` = **UPHELD
  (survived)**, `drop` = OVERTURNED.
- **Two bases reported:** **controlled co-present** (rounds with ≥1 GPT-class AND ≥1 non-GPT-bench
  leaf — the S6-comparable basis, controls participation) is the **primary**; full-sample is
  corroboration.
- **Co-present rounds:** 1,337 (S6 overall had 1,346; the 0.7% diff is because S6 counted
  *all* non-GPT models as the non-GPT arm, while this study restricts to the 5-model bench).
- **Drift note:** the DB is live; a coordinator row appended between the two script runs moved
  counts by ≤1 (1,742→1,743 rounds). Negligible; aggregates unchanged at 1 decimal place.

---

## §1 Contract/data finding-survival baseline (the primary O3 target)

`finding-survival = survived (coord block+defer) / matched contract/data findings raised`.

### Controlled co-present (the bar O3 must beat)

| model | leaves | matched | survived | **survival %** | Wilson 95% CI |
|---|---:|---:|---:|---:|---|
| `gpt-latest:free` (GPT) | 821 | 192 | 167 | 87.0 | 81.5 – 91.0 |
| `gpt-5.5:free` (GPT) | 519 | 62 | 53 | 85.5 | 74.7 – 92.2 |
| **GPT-class aggregate** | 1,340 | 254 | 220 | **86.6** | 81.9 – 90.3 |
| `glm-5.2-high` | 873 | 60 | 20 | 33.3 | 22.7 – 45.9 |
| `grok` | 721 | 103 | 27 | 26.2 | 18.7 – 35.5 |
| `kiro-claude-opus-4-6:free` | 687 | 57 | 10 | 17.5 | 9.8 – 29.4 |
| `grok-4.3` | 321 | 2 | 0 | 0.0 | 0.0 – 65.8 (n=2) |
| `glm-5.1` ⚠thin | 26 | 1 | 0 | 0.0 | 0.0 – 79.3 (n=1) |
| **non-GPT-bench aggregate** | 2,628 | 223 | 57 | **25.6** | 20.3 – 31.7 |

### **The non-GPT contract/data baseline = 25.6% (57/223) [CI 20.3–31.7].**

This is the bar O3's tuned R2 prompt must beat. The GPT-class bar is 86.6% — a **3.4× gap** on the
contract/data slice (wider than S6's overall 4.0× gap in *ratio* terms, but operating at a far
higher survival base: contract/data is a higher-survival slice for everyone because findings are
concrete and actionable).

### Full-sample corroboration (all v2 non-empty rounds, 1,743)

| aggregate | matched | survived | survival % | CI | fnd/leaf raised |
|---|---:|---:|---:|---|---:|
| GPT-class | 272 | 234 | 86.0 | 81.4–89.6 | 0.188 |
| **non-GPT-bench** | 241 | 59 | **24.5** | 19.5–30.3 | 0.086 |

Full-sample (24.5%) and co-present (25.6%) agree within 1.1pp → **the controlled estimate is not
participation-inflated** (same stability finding as S6). The co-present 25.6% is the honest
baseline because it is the apples-to-apples comparison the live A/B will reproduce.

**Cross-check vs S6 overall:** S6's *overall* (all-classes) non-GPT finding-survival was 9.5%.
The **contract/data slice is a higher-survival slice** for non-GPT models (25.6% vs 9.5%) —
contract/data findings are more concrete and the prompt already focuses there, so a larger fraction
survive. But non-GPT still trails GPT by 61pp on this slice. **O3's job is to close the 25.6%→86.6%
contract/data gap from the non-GPT side via prompt tuning, not model swap.**

---

## §2 Volume (contract/data findings raised per leaf) — the recall lever

`fnd/leaf = contract/data findings raised ÷ v2-non-empty leaves` (co-present).

| model | leaves | raised | matched | **fnd/leaf (raised)** | fnd/leaf (matched) |
|---|---:|---:|---:|---:|---:|
| `gpt-latest:free` | 821 | 194 | 192 | **0.236** | 0.234 |
| `grok` | 721 | 110 | 103 | **0.153** | 0.143 |
| `gpt-5.5:free` | 519 | 63 | 62 | **0.121** | 0.119 |
| `kiro-claude-opus-4-6:free` | 687 | 57 | 57 | **0.083** | 0.083 |
| `glm-5.2-high` | 873 | 63 | 60 | **0.072** | 0.069 |
| `glm-5.1` ⚠thin | 26 | 1 | 1 | 0.038 | 0.038 |
| `grok-4.3` | 321 | 2 | 2 | 0.006 | 0.006 |
| **GPT-class agg** | 1,340 | 257 | 254 | **0.192** | — |
| **non-GPT-bench agg** | 2,628 | 233 | 223 | **0.089** | — |

**Read:** GPT-class raises **~2.2× more** contract/data findings per leaf (0.192 vs 0.089). The
single biggest volume lever is `gpt-latest:free` at 0.236 fnd/leaf; the best non-GPT volume is
`grok` (0.153), which actually *matches* `gpt-5.5:free` (0.121) on raw volume but converts far
worse (26.2% vs 85.5% survival) — grok's contract/data findings mostly drop. `glm-5.2-high` raises
few contract/data findings (0.072) but converts the best of the non-GPT bench (33.3%). **O3's
volume goal is to lift non-GPT contract/data fnd/leaf above ~0.089 without collapsing survival.**

---

## §3 Block-calibration (per-class confirmation: gap is volume, not calibration)

Metric A scoped to contract/data: `P(coord-block | leaf-block, matched)`, per class (co-present).

| class | non-GPT-bench (coord-block / leaf-block) | GPT-class | read |
|---|---|---|---|
| `contract_drift` | **89.5%** (17/19) | 94.2% (163/173) | tied within ~5pp |
| `data_integrity` | 100% (5/5) ⚠n=5 | 94.9% (37/39) | tied (non-GPT n tiny) |
| `spec_alignment` | **undefined — non-GPT never block** (0 leaf-blocks) | 92.9% (13/14) | see below |
| **all contract/data** | **91.7%** (22/24) | 94.2% (213/226) | **tied** |

**Re-confirms S6 on the contract/data slice:** when non-GPT models DO call block on contract/data,
they are trustworthy (~92% vs GPT ~94% — CIs overlap). **The gap is not calibration.** The gap is
volume/recall, concentrated in one specific failure mode:

> **Non-GPT models raise `spec_alignment` findings but NEVER escalate them to block.** Across the
> whole non-GPT bench, `spec_alignment` produced **0 leaf-blocks** (grok raised 71 spec_alignment
> findings, kiro 43, glm-5.2-high 26 — all at `defer`/`drop`). GPT raised 14 spec_alignment blocks
> (13 coord-upheld). The `spec_alignment` class is where the non-GPT block-silence is total; it is
> the sharpest single lever for O3's prompt.

`contract_drift` is the class where non-GPT *do* block (19 leaf-blocks, 89.5% upheld) — they just
block far less often than GPT (19 vs 173 leaf-blocks).

---

## §4 Coverage (contract/data slice of the coverage curve)

Of coordinator-**upheld** contract/data findings (coord label in scope + disp block/defer), the
share each side catches with a contract/data-labeled leaf finding (co-present):

| | caught / total upheld contract/data coord findings | share |
|---|---|---:|
| GPT-class | 184 / 225 | **81.8%** |
| non-GPT-bench | 40 / 225 | **17.8%** |

GPT-class catches **~4.6× more** of the upheld contract/data findings. (Full-sample: GPT 75.4%,
bench 16.4% — same shape.) On coverage, the contract/data slice shows the same "one seat (GPT)
carries the load" structure S6 found overall — but more extreme.

---

## §5 Controlled co-present GPT-only survival share (does the ~33% GPT-loss concentrate in contract/data?)

On co-present rounds, contract/data-upheld findings/rounds caught **only by GPT-class** (no non-GPT
bench leaf caught any upheld contract/data finding):

| level | GPT-only / total | GPT-only share |
|---|---|---:|
| finding-level | 157 / 198 | **79.3%** |
| round-level | 133 / 172 | **77.3%** |

**Yes — the GPT-loss concentrates sharply in contract/data.** S6's *overall* GPT-only round
coverage was **32.3%** (131/405). The contract/data slice is **77.3%** — **~2.4× the overall
average.** If GPT quota vanishes, ~77% of contract/data catches are lost (vs ~32% of all catches).
**Contract/data is precisely where the non-GPT backstop matters most**, which is why O3 targets
this slice.

---

## §6 The target statement

> **The tuned R2 contract/data lens prompt must raise non-GPT contract/data finding-survival above
> 25.6% (currently 25.6% aggregate, co-present; per-model bars: glm-5.2-high 33.3%, grok 26.2%,
> kiro-4-6 17.5%) and/or contract/data volume above 0.089 fnd/leaf (currently 0.089 aggregate;
> best non-GPT grok 0.153), without collapsing block-calibration below ~92% (currently 91.7% on
> contract/data), to count as proven.** The deepest single lever is `spec_alignment`: non-GPT
> models are 100% block-silent there today (0 blocks vs GPT's 14), so a prompt that makes the
> non-GPT lens escalate `spec_alignment` to block is the highest-leverage move.

---

## §7 What remains for the live A/B (NOT done here)

This memo is the **baseline leg only** — corpus re-derivation, no live model invocation. What the
live A/B still needs:

1. **The tuned-vs-baseline delta** — run the *same* non-GPT model on the *same* labeled
   change-slices with (a) the current uniform leaf prompt and (b) the tuned R2 contract/data lens
   prompt (`2026-07-28-r2-contract-data-lens-prompt.md`), then compare contract/data
   finding-survival + volume against the baselines above. This requires the **live non-GPT model
   endpoint** (litellm + API key), which is explicitly out of scope here. The runnable harness is
   `2026-07-28-r2-upheld-volume-harness.py` (same dir); its preflight currently blocks on
   sample/key absence (expected).
2. **Model choice for the A/B:** the per-model bars differ (glm-5.2-high 33.3% is the hardest to
   beat; kiro-4-6 17.5% the easiest). The aggregate 25.6% is the primary bar; pick the O3 target
   model and compare against its own per-model bar for a fair within-model test.
3. **Slice labeling:** the A/B needs a labeled sample of change-slices (`diff.patch` +
   `expected.json` ground-truth) covering contract/data-relevant diffs. Not present in this
   environment.

---

## §8 Confidence, sample sizes, thinness caveats

- **Primary baseline (non-GPT-bench, co-present):** 57/223 = 25.6% [CI 20.3–31.7]. Denominator
  223 matched contract/data findings is adequate for a ±5pp band. Dominated by grok (103), glm-5.2-high
  (60), kiro-4-6 (57) — three well-sampled models.
- **GPT-class baseline:** 220/254 = 86.6% [81.9–90.3] — tight, well-sampled.
- **`glm-1` thinness (flagged):** only **44 v2-non-empty leaves** (26 co-present). It retired
  2026-06-13, just after the v2 schema began (~06-08), so its v2 footprint is ~5 days. On
  contract/data it raised 4 findings full-sample (1 co-present) — **too thin to characterize on
  contract/data**; included per the task's bench spec but contributes ~0.4% of the matched
  denominator and does not move the aggregate.
- **`grok-4.3` near-silent on contract/data:** 2 matched findings co-present (0.006 fnd/leaf).
  Effectively does not participate in the contract/data lens.
- **`data_integrity` thin for non-GPT blocks:** only 5 non-GPT leaf-blocks co-present — the 100%
  block-calibration is real but tiny-n. `contract_drift` (19 leaf-blocks) is the only class with
  enough non-GPT block mass to calibrate.
- **Matching is conservative** (score≥0.45): absolute survived/matched COUNTS are a lower bound,
  but **ratios (survival %) are robust** — numerator and denominator are equally affected (same
  caveat as S6/q30). Wilson CIs reflect binomial sampling, not matching uncertainty.
- **Coordinator is itself a model** (mostly `glm-5.2-high`); a systematic coordinator bias
  toward/against a leaf model would bias attribution. Overturn rates are low and consistent (S6 §5),
  arguing against strong bias, but it is not fully controllable from logs.
- **Observational, not experimental:** co-present control removes participation confounding but
  not all confounders (change mix, prompt version). Same-parent-round is the strongest available
  natural control; the stability of survival under co-present control (Δ≤1.1pp vs full-sample)
  argues against material inflation. The decisive test is the live fixed-input A/B (§7).

---

## Persist-guard compliance

This document contains **aggregate model statistics only**. No sensitive-repo name/path, no file
paths, no code snippets, no verbatim finding text, and no `/home/` absolute paths (the DB is
referenced in tilde form). All cross-repo context uses neutral labels. Model-ids are safe to cite
and are cited. Raw per-finding data lives only in the gitignored `tmp/commit-study/o3ab_output.json`
(not committed, not reproduced here).

## Constraints confirmed

- **Read-only:** no source/config/panel edits, no commit.
- **Only files written:** this document + `tmp/commit-study/o3ab_baseline.py`,
  `tmp/commit-study/o3ab_probe_labels.py`, `tmp/commit-study/o3ab_output.json` (all gitignored
  `tmp/`).
- **No shared in-flight file touched:** did NOT read or write `review-tiers.json`,
  `commit-reviewer*.md`, or any commit-gate / panel config (commit-gate lane active).
- `git status -- tmp/` is **clean** (tmp/ is fully gitignored).
