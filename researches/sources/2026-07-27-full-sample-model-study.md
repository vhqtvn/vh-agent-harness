# Sources: Commit-Review Panel Model-Study Series — Full-Sample Model Quality Study (coordinator-adjudicated upheld-precision)

**Date:** 2026-07-27 (study date; promoted to durable sources 2026-07-28)
**Topic:** full-sample coordinator-adjudicated upheld-precision across the entire review history (9637 review-leaf rows, 2935 coordinator rounds, 2026-05-25 → 2026-07-27). Source of the §8 blind-spot raise counts (performance=244, concurrency=65) that the S3 probe §1.1 later corrected as issue-TEXT keyword matches, not category labels (~10× inflation).
**Series:** [`./2026-07-27-commit-review-panel-model-study-series.md`](./2026-07-27-commit-review-panel-model-study-series.md) (commit-review panel model-study
series, 2026-07-27 → 2026-07-28).

## Provenance

- **Source type:** study (full-sample).
- **Evidence class:** Class-A — this-machine opencode DB-cited (read-only `mode=ro`); not re-derivable outside this host.
- **Original location:** `tmp/agent-runs/review-model-comparison/2026-07-27-full-sample-model-study.md`
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
  `git log -1 --format=%H -- researches/sources/2026-07-27-full-sample-model-study.md`.
- **Status:** durable source packet. Internal `tmp/` cross-references to peer memos
  resolve to the promoted canon paths listed in the series index above.
- **Faithfulness:** the body below the `---` delimiter is a byte-for-byte verbatim copy of the original `tmp/` study; only this provenance header is new — the study's own title, internal provenance, and cross-references to peer memos are unchanged.

---

# Commit-Review Panel — FULL-SAMPLE Model QUALITY Study
### Coordinator-adjudicated upheld-precision across the entire review history

**Purpose:** ground the review-panel restructure in aggregate model behavior,
keyed to the coordinator's FINAL disposition (not raw finding rate, not the
leaf's self-reported disposition).

---

## §0 Provenance, keying, and PERSIST-SCRUB note

**Data source:** the local opencode session store (read-only). Review leaves =
sessions with `agent LIKE 'commit-reviewer-%'` plus `ship-review`. Each leaf
emits a `commit-review-result.v2` JSON block (`verdict`, `findings[]` with
per-finding `category`/`severity`/`disposition ∈ {block,defer,drop}`). The
parent `commit-reviewer` (coordinator, mostly `glm-5.2-high`) emits the merged
final disposition (`verdict`, `blocking_issues[]`). The coordinator's blocking
set is treated as the authoritative final disposition against which leaf BLOCK
findings are adjudicated.

**Full sample:**
- **9,637 review-leaf rows** (all repos, all leaf slots a/b/c/d + ship-review).
- **2,935 coordinator rounds** (2,902 parsed, **98.9%**).
- **Leaf parse coverage: 9,495 / 9,637 = 98.5%.**
- Date span **2026-05-25 → 2026-07-27**.
- **776** coordinator rounds carried ≥1 blocking issue; **315** rounds produced
  ≥1 leaf-BLOCK that the coordinator upheld (the rounds where the panel actually
  caught something adjudicable).

**Keying protocol (a prior study mis-keyed this — corrected here):** group by
EXACT `json_extract(model,'$.id')`. `:free`/paid and version variants
(4-5 vs 4-6, gpt-5.4 vs 5.5 vs latest, grok vs 4.3 vs 4.5) are kept DISTINCT.
Min-sample floor **n ≥ 20** for any verdict. Single-day / broken model-ids are
FLAGGED TRANSIENT and quarantined — never allowed to define a headline.

Quarantined / excluded (do not drive any verdict):
- `kiro-claude-opus-4-5` **(no `:free`)** — 21 runs, **1 day**, **100% empty**.
  This is the known keying trap; the LIVE Claude leaves are
  `kiro-claude-opus-4-6:free` (2,305 runs, ~17% empty) and
  `kiro-claude-opus-4-5:free` (188 runs, healthy). QUARANTINED.
- `gpt-latest-hard:free` — 24 runs, **41.7% empty, 0% parseable** (broken).
- `gpt-5.5` (n=7), `claude-opus-4-6-thinking:free` (n=5) — below floor.
- `gemini-3.1-pro-preview:free` — 30 runs but **single day (07-25)**; FLAGGED
  TRANSIENT, reported for context only, not a headline driver.

**PERSIST-SCRUB note (confidentiality boundary is at OUTPUT):** the store
includes a sensitive consumer repo (the largest presence). This file contains
**aggregate model statistics only**. No sensitive-repo name/path, no file paths,
no code snippets, no verbatim finding text from any consumer repo, and no
home-directory absolute paths appear anywhere below. Cross-repo results use NEUTRAL labels
(`harness`, `consumer-A`, `consumer-B (unnamed)`). Any illustrative example is
from the harness repo only. Model-ids are safe to cite and are cited.

**Panel architecture (load-bearing for the read):** the 4-leaf panel is a
FIXED-SEAT design. Each round assigns one model per family-seat — the dominant
combo is `{glm-seat, gpt-seat, grok-seat, kiro-seat}`. Leaves therefore rarely
raise the SAME finding (they cover different lenses), so "unique-upheld" ≈
"upheld" by construction, and marginal-value is best read per SEAT (family).

---

## §1 Per-model scorecard — reliability & verdict rates (all leaf rows)

| model-id | n | days | empty% | parse% | block% | split% | approve% |
|---|---:|---:|---:|---:|---:|---:|---:|
| kiro-claude-opus-4-6:free | 2305 | 48 | **17.0** | 99.4 | 1.0 | 0.1 | 76.4 |
| glm-5.2-high | 1753 | 37 | 0.0 | 99.5 | 1.9 | 0.3 | 73.5 |
| gpt-latest:free | 1119 | 18 | 2.1 | 98.4 | **17.6** | 1.1 | 70.8 |
| grok | 1118 | 18 | 1.2 | 98.9 | 2.2 | 0.6 | 89.9 |
| grok-4.3 | 975 | 35 | 0.9 | 99.3 | 1.5 | 0.5 | 66.5 |
| gpt-5.5:free | 962 | 33 | 0.9 | 98.0 | **17.7** | 0.4 | 62.2 |
| glm-5.1 (retired) | 318 | 8 | 0.0 | 98.1 | 7.7 | 0.3 | 89.7 |
| gpt-5.4 (retired) | 257 | 11 | 1.6 | 98.8 | **62.6** | 11.0 | 25.2 |
| kiro-claude-opus-4-5:free | 188 | 3 | 4.3 | 97.9 | 1.1 | 0.0 | 97.3 |
| claude-opus-4-8 (retired) | 141 | 4 | 0.0 | 100.0 | 19.9 | 2.1 | 78.0 |
| gpt-5.4:free (retired) | 140 | 3 | 0.0 | 99.3 | **77.0** | 2.9 | 20.1 |
| claude-opus-4-7 (retired) | 124 | 9 | 2.4 | 92.7 | 21.7 | 3.5 | 73.9 |
| glm-5.2 | 120 | 9 | 0.0 | 94.2 | 7.1 | 0.0 | 71.7 |
| grok-4.5 | 30 | 2 | 13.3 | 93.3 | 10.7 | 0.0 | 82.1 |
| gemini-3.1-pro-preview:free ⚠TRANSIENT | 30 | 1 | 0.0 | 100.0 | 10.0 | 0.0 | 73.3 |

**Reliability read:** GPT-class and current GLM/grok/kiro-4-5 leaves are all
clean on empty-rate except `kiro-claude-opus-4-6:free` at a persistent **17%
empty** (structural, stable across weeks — the single biggest live reliability
liability). Block-rate cleanly separates the panel into **block-finders
(GPT-class, ~18-77%)** and **advisory/approve models (glm/grok/kiro,
~1-8%)** — a rate difference of roughly **~20-40× GPT vs kiro-4-6**. Rate alone
is a trap, so the headline is validity, below.

---

## §2 VALIDITY — coordinator-adjudicated upheld-precision (the headline)

For each model, of its leaf findings raised at `disposition=block`, how many were
UPHELD in the coordinator's final blocking set (matched by finding category or by
location file-basename). This is the true "rate ≠ quality" separator.

| model-id | block raised | UPHELD | upheld-precision | unique-upheld |
|---|---:|---:|---:|---:|
| **gpt-latest:free** | 251 | **236** | **94.0%** | 233 |
| **gpt-5.5:free** | 127 | **104** | 81.9% | 99 |
| **glm-5.2-high** | 41 | 37 | **90.2%** | 33 |
| **grok** | 33 | 28 | 84.8% | 23 |
| kiro-claude-opus-4-6:free | 16 | 14 | 87.5% | 8 |
| grok-4.3 | 12 | 9 | 75.0% | 6 |
| glm-5.1 (retired) | 8 | 8 | 100.0% | 5 |
| glm-5.2 | 8 | 6 | 75.0% | 5 |
| grok-4.5 | 5 | 4 | 80.0% | 4 |
| gemini:free ⚠TRANSIENT | 3 | 3 | 100.0% | 3 |
| kiro-claude-opus-4-5:free | 2 | 1 | 50.0% | 1 |

**Match coverage / honesty caveat:** upheld matching is on
(category-name equality) OR (location file-basename equality) within a round.
This is conservative on wording variance (a real catch phrased with a different
category label and a different file than the coordinator's echo can be missed) —
so absolute upheld COUNTS are a lower bound. The near-equality of `unique-upheld`
to `upheld` is a design artifact (fixed-seat, non-overlapping lenses), not a
uniqueness discovery. Precision (ratio) is robust to the count floor.

**Read (upheld-based, not rate-based):**
- **`gpt-latest:free` is the unambiguous quality leader** — it raises the most
  blocks AND survives adjudication at **94%**. High rate + high precision =
  a genuine block-finder, not trigger-happy noise. This is the load-bearing seat.
- **`gpt-5.5:free`** is the second GPT block-finder — high volume (104 upheld)
  but noticeably noisier (82% precision).
- **`glm-5.2-high`** is the strongest NON-GPT block-finder: fewer blocks (37
  upheld) but excellent **90% precision** — a real second lens, and it doubles as
  the coordinator model.
- **`grok`** third-tier real block-finder (28 upheld, 85%).
- **`kiro-claude-opus-4-6:free`** contributes little as a block-finder (14
  upheld) and carries the 17%-empty reliability cost.

---

## §3 GPT-INDEPENDENCE — fallback bench & graceful degradation

**This is the load-bearing constraint: GPT quota can vanish. What survives?**

Share of ALL upheld block findings by FAMILY-SEAT (n=450 upheld, matched):

| seat (family) | upheld | share | cumulative |
|---|---:|---:|---:|
| **gpt** | 340 | **75.6%** | 75.6% |
| glm | 51 | 11.3% | 86.9% |
| grok | 41 | 9.1% | 96.0% |
| kiro/claude | 15 | 3.3% | 99.3% |
| gemini (transient) | 3 | 0.7% | 100.0% |

**Answer: removing ALL GPT models drops upheld catches from 450 → 110 — a
75.6% loss of the panel's adjudicated block-finding power.** The panel does NOT
gracefully survive full GPT removal at current quality; it degrades to roughly
one-quarter of its catch power.

**Best NON-GPT fallback bench (explicit, ranked):**

| non-GPT model | upheld | precision |
|---|---:|---:|
| **glm-5.2-high** | 37 | **90.2%**  ← next-best block-finder |
| grok | 28 | 84.8% |
| kiro-claude-opus-4-6:free | 14 | 87.5% |
| grok-4.3 (retired) | 9 | 75.0% |
| glm-5.1 (retired) | 8 | 100.0% |

**Degradation story:** if GPT is removed entirely, `glm-5.2-high` is the
next-best block-finder at essentially GPT-grade *precision* (90% vs 94%) — the
precision cost is small (~4pp). The unavoidable cost is **recall/volume**:
glm+grok together upheld only ~65 blocks vs GPT's 340, because the non-GPT
models are tuned advisory-shy (block-rate 2-8% vs GPT's ~18%). So a
GPT-free panel keeps quality-per-block but loses the throughput of block
*discovery*. The mitigation is not a model swap but re-tuning the surviving
seats to block more aggressively (raise their block-rate toward GPT's) —
`glm-5.2-high` is the model to promote into the primary block-finder seat.

---

## §4 Marginal-value / coverage curve — how many seats are needed?

Greedy set-cover over the **315 rounds that produced ≥1 upheld block**, by seat:

| add seat | cumulative rounds covered | share |
|---|---:|---:|
| +gpt | 273 / 315 | **86.7%** |
| +glm | 302 / 315 | **95.9%** |
| +grok | 311 / 315 | 98.7% |
| +kiro | 313 / 315 | 99.4% |
| +other | 315 / 315 | 100.0% |

**Read: 2 seats (GPT + GLM) cover 96% of rounds with a real catch. The grok
seat adds ~2.8pp; the kiro seat adds ~0.7pp.** On adjudicated quality,
**the panel needs ~2 seats, not 4.** The 3rd and 4th seats are near-interchangeable
low-marginal-value additions — justified only by lens diversity / redundancy,
not by upheld coverage.

---

## §5 Severity calibration & coordinator-override (trigger-happy check)

Overturn rate = leaf BLOCK raised but NOT upheld by the coordinator (noise):

| model-id | blocks | overturned | overturn% |
|---|---:|---:|---:|
| gpt-latest:free | 251 | 15 | **6.0%** (best at scale) |
| glm-5.2-high | 41 | 4 | 9.8% |
| kiro-claude-opus-4-6:free | 16 | 2 | 12.5% |
| grok | 33 | 5 | 15.2% |
| gpt-5.5:free | 127 | 23 | 18.1% |
| grok-4.3 | 12 | 3 | 25.0% |
| glm-5.2 | 8 | 2 | 25.0% |

**Read:** `gpt-latest:free` is both the highest-volume AND the best-calibrated
block-finder (only 6% overturned across 251 blocks) — it is NOT trigger-happy.
`gpt-5.5:free` is the noisy GPT variant (18%). Severity mix: GPT-class blocks
concentrate in `major`/`critical`/`high`; grok/kiro blocks are almost entirely
`critical` (they only escalate to block on the highest stakes). No model shows
pathological severity inflation among the surviving blocks.

---

## §6 Cross-repo stability (NEUTRAL labels)

Rounds per repo: **consumer-B (unnamed)** 1,509 · **consumer-A** 787 ·
**harness** 639. Upheld-block share by family-seat, per repo:

| repo | gpt | glm | grok | kiro |
|---|---:|---:|---:|---:|
| harness | **83.2%** | 7.0% | 7.0% | 0.7% |
| consumer-A | **65.4%** | 14.1% | 15.4% | 5.1% |
| consumer-B (unnamed) | **74.2%** | 13.1% | 8.3% | 4.4% |

**Read: the GPT-dominant ranking holds across all three repos** — GPT is the
top block-finder everywhere (65-83%), glm/grok are the consistent #2/#3 non-GPT
seats, kiro is the tail everywhere. The ranking is stable; the panel structure
generalizes across repos. (consumer-A shows the most non-GPT balance — GPT's
edge is largest on the harness's own contract-heavy diffs.)

---

## §7 Capability fingerprint & unique-class coverage

Top upheld categories per model (which lens each seat is actually good at):

- **gpt-latest:free** — `contract_drift` (132), `data_integrity` (33),
  `security` (20), `spec_alignment` (18), `correctness` (16). The
  contract/spec-conformance workhorse.
- **gpt-5.5:free** — `contract_drift` (41), `security` (22), `correctness` (21).
- **glm-5.2-high** — `correctness` (20), `contract_drift` (8); also the sole
  upholder of `style`/`tests` classes.
- **grok** — `correctness` (11), `contract_drift` (6), `data_integrity` (5);
  sole upholder of `test_coverage` and `domain-purity`.
- **kiro-claude-opus-4-6:free** — `correctness` (9), `contract_drift` (4).

**Classes upheld by exactly ONE (n≥20) model** (differentiation): the vast
majority of single-model classes belong to GPT (backward-compat, capability_policy,
immutability, safety, spec-conformance/deviation, verbatim-contract,
regression-resolution, docs). glm uniquely upholds `style`/`tests`; grok uniquely
`domain-purity`/`test_coverage`. **GPT is not just highest-volume — it uniquely
owns the contract/spec/safety class cluster**, which is precisely the harness's
highest-stakes review surface. That cluster is the concrete thing lost if GPT is
removed.

---

## §8 Blind spots — prompt-policy gaps, NOT model gaps

Distinguishing "model missed it" from "prompt/gating never lets it block":

- **doc_drift (2,741 raised, 0 upheld-as-block), missing_test (647), maintainability
  (235), dependency_risk (232)** — these are raised overwhelmingly at
  `disposition=drop`/`defer`. That is BY DESIGN: the coordinator gates on BLOCK
  only; defer/drop never gate. This is expected advisory volume, not a blind spot
  — but it is enormous low-value chatter the prompt could stop soliciting.
- **True blind spots (should block, systematically don't):**
  - **concurrency** — 65 raised, effectively 0 upheld as a block (mostly dropped).
  - **performance** — 244 raised, only 2 upheld as a block.
  - **race conditions** — 9 raised, 0 upheld.
  - **blast-radius / complexity** — essentially absent from the blocking corpus.

  These are **PROMPT/GATING gaps, not model incapability**: the models DO surface
  concurrency/performance concerns, but the gating policy downgrades them to
  advisory, so they never reach a blocking disposition. **No model swap or added
  seat fixes this** — only a dedicated lens prompt + a gating rule that lets
  concurrency/perf/blast-radius escalate to block will. This is the single most
  important structural finding for the restructure.

---

## §9 Limitations

- Upheld matching is category-name OR location-basename equality → conservative;
  absolute upheld COUNTS are a lower bound (precision RATIOS are robust).
- Uphold is defined against the coordinator's final blocking set. The coordinator
  is itself a model (mostly `glm-5.2-high`); a systematic coordinator bias toward
  or against a given leaf model would bias upheld attribution. Overturn rates
  (§5) are low and consistent, which argues against strong coordinator bias, but
  it is not fully controllable from logs.
- Retired models (glm-5.1, gpt-5.4/5.4:free, claude-opus-4-7/4-8) are reported
  for context; they are not candidates for the live panel.
- `gemini-3.1-pro-preview:free` (single day) and all n<20 ids are excluded from
  verdicts.
- The blind-spot disposition read relies on some findings echoing the schema
  enum literal in the disposition field; the direction (concurrency/perf drop,
  rarely block) is unambiguous even after discounting those.

---

## §10 Cross-check vs the prior 2-repo study

Prior study: `2026-07-27-model-audit-quality-study.md` (1,407 coord rounds /
5,174 leaves). This full-sample re-derivation covers **2,902 parsed rounds /
9,495 parsed leaves** — the prior study's extraction EXCLUDED the largest
(sensitive) repo and the no-suffix/ship-review leaves, so it was a partial sample.

**CONFIRMED (both studies agree):**
- Keying trap: `kiro-claude-opus-4-5` (no `:free`) = 1-day/100%-empty transient →
  quarantine. Live kiro is `kiro-4-6:free` at ~16-17% empty. ✓
- GPT-class blocks vastly more than others (~20-40×). ✓
- **`gpt-latest:free` is the quality leader** — high rate AND high precision, not
  trigger-happy noise. ✓ (My coordinator-adjudicated precision 94% is even
  higher than the prior study's leaf-self-disposition 18.4%, because the metrics
  differ — see below.)
- Blind spots (concurrency/performance/blast-radius) are PROMPT gaps no model
  swap fixes. ✓

**CORRECTED / refined (trust this fuller re-derivation):**
- **Metric definition:** the prior study measured "upheld" as the LEAF's own
  self-reported disposition (block+defer = upheld, drop = overturned). This study
  measures upheld against the COORDINATOR's FINAL blocking set — the disposition
  the task specifies. The two give different absolute precisions but the same
  ranking.
- **Marginal value is even more concentrated than "3 of 4 slots":** on
  coordinator-adjudicated coverage, **GPT+GLM = 96% of rounds-with-a-catch (2
  seats), not 3.** The grok seat adds ~2.8pp and the kiro seat ~0.7pp.
- **GPT-independence quantified for the first time as a full-sample number:**
  full GPT removal loses **75.6%** of upheld catches; `glm-5.2-high` is the
  named fallback at 90% precision but far lower volume.
- Cross-repo stability now confirmed on THREE repos (prior study saw two) — the
  GPT-dominant ranking holds on all three.

---

## §11 Bottom-line read (INPUT to the panel-restructure decision)

1. **`gpt-latest:free` is the irreplaceable primary block-finder** — 94%
   upheld-precision, 6% overturn at scale, sole owner of the contract/spec/safety
   class cluster. Pin it on the highest-stakes seat.
2. **`glm-5.2-high` is the designated GPT fallback and best non-GPT seat** —
   90% precision. In a GPT-outage it becomes primary, but must be re-tuned to
   block more aggressively (its 2% block-rate is the recall bottleneck, not its
   precision).
3. **On quality, the panel needs ~2 seats (GPT + GLM), not 4.** grok is a
   defensible 3rd diversity seat (85% precision, a couple unique classes);
   **the kiro seat is near-zero marginal value (+0.7pp coverage) AND carries the
   17%-empty reliability cost** — repoint it (e.g. to a dedicated concurrency/
   perf/blast-radius lens) rather than keep it as a redundant general reviewer.
   Do NOT retire kiro on the basis of the 1-day paid transient (§0).
4. **The biggest lever is NOT model choice — it is the prompt/gating:**
   concurrency, performance, race, and blast-radius are systematically dropped
   to advisory and never block. A dedicated lens prompt + a gating rule that lets
   these escalate would add coverage that no model swap or extra seat can.
5. **GPT-independence risk is real and large** (75.6% catch loss on full
   removal). If GPT quota is a live risk, the restructure should (a) keep GLM as
   a hot fallback tuned to block harder, and (b) not depend on a 4-way panel for
   resilience — resilience comes from a re-tuned GLM seat, not from seat count.

*Output confirmed sensitive-repo-scrubbed: aggregate model statistics only;
neutral repo labels; no sensitive-repo name/path, no file paths, no code, no
verbatim consumer finding-text, no home-directory absolute paths.*
