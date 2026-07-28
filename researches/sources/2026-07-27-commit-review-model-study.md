# Sources: Commit-Review Panel Model-Study Series — Model Differentiation Study (rate-based; SUPERSEDED)

**Date:** 2026-07-27 (study date; promoted to durable sources 2026-07-28)
**Topic:** first-pass rate-based model-differentiation study over the commit-review panel history (5174 tier-1 leaves, 1232 full review rounds). SUPERSEDED by the quality-axis study (2026-07-27-model-audit-quality-study.md) which added the missing upheld-validity layer and re-keyed the paid transient. Retained as canon because its data lineage is the starting point the rest of the series corrects.
**Series:** [`./2026-07-27-commit-review-panel-model-study-series.md`](./2026-07-27-commit-review-panel-model-study-series.md) (commit-review panel model-study
series, 2026-07-27 → 2026-07-28).

## Provenance

- **Source type:** study (SUPERSEDED — see quality-axis study).
- **Evidence class:** Class-A — this-machine opencode DB-cited (read-only `mode=ro`); not re-derivable outside this host.
- **Original location:** `tmp/agent-runs/review-model-comparison/2026-07-27-commit-review-model-study.md`
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
  `git log -1 --format=%H -- researches/sources/2026-07-27-commit-review-model-study.md`.
- **Status:** durable source packet. Internal `tmp/` cross-references to peer memos
  resolve to the promoted canon paths listed in the series index above.
- **Faithfulness:** the body below the `---` delimiter is a byte-for-byte verbatim copy of the original `tmp/` study; only this provenance header is new — the study's own title, internal provenance, and cross-references to peer memos are unchanged.

---

# Commit-Review Panel — Model Differentiation Study

**Question this informs:** the commit-review panel fans out to 4 tier-1 leaves
(`commit-reviewer-a/b/c/d`), each pinned to a *different* model but running the
*same* 753-line review prompt. Should the maintainer keep pure model-diversity,
or restructure toward lens-diversity / stakes-allocated redundancy? This is the
empirical foundation: over the full history, **did the models behave
differently, or is the diversity redundant?**

---

## Provenance

- **DB:** `~/.local/share/opencode/opencode.db` (opened read-only, `mode=ro`).
- **Selection:** `session.agent IN (commit-reviewer-a..d)` AND
  `directory IN (vh-agent-harness, vh-solara)`. Model keyed on the **actual**
  `json_extract(model,'$.id')` at run time, NOT the leaf slot (slot→model
  changed repeatedly: kiro-opus-4-5→4-6, gpt-latest→gpt-5.5, grok→4.3→4.5, a
  gemini repoint, glm-5.2-high).
- **Leaf sessions:** 5174 total (harness 2274, solara 2900).
- **Date span:** 2026-06-26 → 2026-07-27 (~31 days).
- **Verdict parse rate:** 5047 / 5174 = 97.5% (final assistant JSON block).
- **Full review rounds** (parent with ≥2 parsed leaves): 1232.
- Verdicts extracted from each leaf's final assistant `text` part (schema_v2
  JSON: `verdict`, `confidence`, `findings[]` with `category`/`severity`/
  `disposition`, `blocking_issues`). Severity/disposition weight used for
  "weighted": block=9, defer=3, drop=1.

### Per-model sample size (n)

| model id | n | in ≥2-leaf rounds |
|---|---:|---:|
| glm-5.2-high | 1241 | 1220 |
| kiro-claude-opus-4-6:free | 1210 | 1039 |
| gpt-latest:free | 848 | 822 |
| grok | 846 | 834 |
| gpt-5.5:free | 402 | 391 |
| grok-4.3 | 390 | 379 |
| kiro-claude-opus-4-5:free | 171 | 143 |
| gemini-3.1-pro-preview:free | 30 | 30 |
| kiro-claude-opus-4-5 (paid) | 21 | 0 |
| grok-4.5 | 15 | 15 |

---

## Per-model scorecard

`empty%` = final message emitted 0 output tokens (reliability failure).
`B/D/Dr` = block / defer / drop *disposition* finding counts (per-leaf, raw).
`blockRate` = block-disposition findings ÷ parsed leaves (**the load-bearing
signal**). `uniqW` = severity-weighted unique catches (see method §4).

| model | n | empty% | parse% | B / D / Dr findings | **blockRate** | uniqW | uniqW/round |
|---|---:|---:|---:|---:|---:|---:|---:|
| **gpt-latest:free** | 848 | 1.9% | 97.1% | **171** / 59 / 383 | **0.208** | 2089 | 2.54 |
| **gemini-3.1-pro:free** | 30 | 0.0% | 100% | 4 / 4 / 17 | **0.133** | 65 | 2.17 |
| **gpt-5.5:free** | 402 | 1.0% | 97.3% | **51** / 10 / 170 | **0.130** | 659 | 1.69 |
| glm-5.2-high | 1241 | 0.0% | 98.3% | 30 / 160 / 2584 | 0.025 | 3326 | 2.73 |
| grok | 846 | 0.4% | 98.6% | 18 / 121 / 1813 | 0.022 | 2330 | 2.79 |
| grok-4.3 | 390 | 2.1% | 97.2% | 7 / 10 / 247 | 0.018 | 340 | 0.90 |
| kiro-opus-4-5:free | 171 | 4.7% | 97.1% | 2 / 9 / 196 | 0.012 | 220 | 1.54 |
| **kiro-opus-4-6:free** | 1210 | **17.2%** | 98.3% | 5 / 49 / 2129 | **0.004** | 2206 | 2.12 |
| grok-4.5 | 15 | 0.0% | 100% | 1 / 1 / 38 | 0.067 | 50 | 3.33 |
| **kiro-opus-4-5 (paid)** | 21 | **100%** | **0%** | 0 / 0 / 0 | — | 0 | 0.00 |

Notes:
- **No leaf ever emits a top-level `block` *verdict`.** Leaf verdicts are
  overwhelmingly `approve` (a few `split`); escalation to a block lives in the
  `findings[].disposition=block` / `blocking_issues`. So the verdict field is a
  near-constant and is NOT the differentiation axis — finding disposition is.
- Cost columns were 0.00 for every leaf (free/plan-billed providers); no
  cost/latency signal recoverable from the DB for this panel.

---

## Differentiation verdict: **DIFFERENTIATED — but along one axis only**

The panel is **not** redundant, but the differentiation is concentrated in a
single, decision-relevant dimension: **who finds the load-bearing BLOCK.**

- **Block-finding rate spans ~50×:** gpt-latest **0.208** and gpt-5.5 **0.130**
  vs grok **0.022**, glm **0.025**, and kiro-opus-4-6 **0.004**. The GPT slot
  raises a blocking finding in ~1 of 5 reviews; the kiro slot in ~1 of 250.
  This spread holds across BOTH repos (harness and solara), so it is a model
  property, not a repo artifact.
- **The GPT slot owns the block category `contract_drift`** — the single
  highest-stakes class (147 of 202 contract_drift findings are blocks). gpt-
  latest alone raised 100 of them; gpt-5.5 raised 24. kiro-opus-4-6 raised 3.
  (e.g. gpt-latest block sessions `ses_0b3b8f1acffeWyfHMMM8C1q9hS`,
  `ses_0b3a25225ffeYjnXdYkw9thNat`.)
- **The kiro/grok/glm slots are advisory-noise generators.** Their unique-catch
  weight is high in raw terms (glm uniqW 3326, grok 2330) but it is almost
  entirely DROP advisories: glm 2576 unique DROPs vs 30 unique BLOCKs; grok
  1805 DROPs vs 18 BLOCKs; kiro-opus-4-6 2023 DROPs vs 5 BLOCKs. Severity-
  weighted, a unique DROP is worth 1 and a unique BLOCK 9 — and the drop volume
  does not convert into catches that change a merge decision.
- **Restated as the number that matters:** across all models, unique BLOCK
  catches = gpt-latest 170, gpt-5.5 51, glm 30, grok 18, grok-4.3 7, kiro-4-6 5.
  **The two GPT variants account for ~70% of all unique block catches** while
  being ~24% of leaf runs. That is real, load-bearing differentiation.

So: keeping *some* model-diversity earns its keep **because the GPT slot catches
blocks the others do not**. But three of the four slots (kiro, grok, glm) are
near-interchangeable on the only axis that stops a bad merge.

---

## Shared blind spots (classes NO model catches — a prompt gap, not a model gap)

Across all 5047 parsed leaves, entire lens-classes are absent or block-free.
These argue for **lens-diversity**, because no model choice will fix them — the
753-line prompt never asks:

- **complexity / cyclomatic complexity:** 0 findings, ever.
- **blast-radius / change-impact:** 0 findings, ever.
- **maintainability:** 174 findings but **0 blocks** — raised as soft advisory,
  never load-bearing.
- **concurrency / race / race-safety:** ~22 concurrency + ~6 race findings
  total, **0 blocks** — near-silent for a Go codebase.
- **performance:** 22 findings, 2 blocks — barely on the radar.
- **rollback / migration-safety / backward-compat:** 1–2 findings each, mostly
  from a single model — effectively unmonitored.

By contrast the prompt *does* drive: correctness (2192 findings), doc_drift
(1837), test_coverage (1252), contract_drift (202). The panel is a
**correctness+contract+docs** reviewer with a systemic/architectural blind spot.
No amount of model-swapping addresses this; a dedicated complexity/blast-radius
*lens* would.

---

## Reliability signal (do not skip — this is a decision input)

- **kiro-claude-opus-4-5 (paid slot): 21/21 = 100% empty, 0% parse.** Pure dead
  weight during its window — every run returned 0 output tokens
  (`ses_066432811ffe951NTczYc9d86D`). If this slot is still live, it contributes
  nothing.
- **kiro-claude-opus-4-6:free: 17.2% empty (208/1210).** Of these, 196 burned
  reasoning tokens then emitted no final text (truncation/timeout failure mode)
  and 12 were truly empty. Combined with its 0.004 block rate, this is the
  weakest *productive* slot: it is both the least reliable and the least likely
  to catch a block.
- All other models: empty rate ≤ 4.7%, parse ≥ 97%.

---

## Honest limitations

1. **Cross-leaf finding-matching is strict.** Unique-catch is computed by
   canonicalizing `(category, location, issue-prefix)`. Different models phrase
   the same real issue differently, so this **under-counts agreement**: the
   headline "99.8% of 8158 distinct findings are raised by only one leaf, 0.2%
   by ≥2" (see below) overstates disagreement and should NOT be read as "the
   models never agree." The parse-robust, matching-independent signals — the
   **per-model block *rate*** and the **empty rate** — are the ones to trust.
   The unique-catch weights (uniqW) inherit the matching noise; treat them as
   directional, not exact.
2. **Model-change confound.** Slot→model changed several times mid-history, so
   per-model windows are non-overlapping in time; a model's numbers reflect the
   commit stream during its window, not a controlled A/B on identical diffs.
   (The 4 leaves in a *round* DO see the same diff, so round-level comparisons
   are controlled; per-model aggregates are not.)
3. **Small-n models:** gemini (n=30), grok-4.5 (n=15), kiro-opus-4-5-paid
   (n=21) are too small to conclude from — gemini's 0.133 block rate is
   suggestive but n=30. Do not over-weight these rows.
4. **Verdict-field is near-constant** (approve dominates); differentiation had
   to be read from finding disposition, which depends on parse success (2.5%
   unparsed dropped silently — likely skewed toward malformed/empty outputs,
   i.e. this slightly *flatters* reliability).
5. Cost/latency not recoverable (all rows cost 0).

Supporting metric (with caveat 1): of 8158 distinct findings across 1232 rounds,
8145 (99.8%) appeared in exactly one leaf and 13 (0.2%) in ≥2. Read this as
"finding *text* rarely matches across models," not "models rarely agree."

---

## Bottom-line read (input to the panel decision)

**Does model-diversity earn its cost?** Partially. It earns its keep through
**one slot** — the GPT variants, which find ~70% of unique blocks and own the
highest-stakes class (contract_drift) at a block rate 6–50× the others. Kill
model-diversity entirely and you lose the panel's primary block-catcher.

**Is there dead weight?** Yes, clearly:
- **kiro-opus-4-5 (paid): 100% empty — remove it now.**
- **kiro-opus-4-6:free: 17% empty + 0.004 block rate** — the weakest productive
  model; a candidate to repoint. grok and glm are reliable but block-thin
  (~0.02) — they mostly add DROP-advisory noise, not merge-stopping catches.

**Keep model-redundancy, add lens-diversity, or both? → BOTH, re-weighted.**
- The evidence does **not** support *pure* model-diversity across 4 near-equal
  models: 3 of 4 slots are interchangeable on the block axis. Running 4 models
  on the identical prompt buys one strong block-catcher plus three advisory
  generators.
- The evidence **does** support: (a) **guarantee a GPT-class slot** for the
  block/contract lens (it is the demonstrated load-bearing reviewer); (b)
  **retire the dead/near-dead kiro slots** or repoint them; (c) reinvest the
  freed slot(s) into **lens-diversity** — a dedicated complexity / blast-radius
  / concurrency / rollback reviewer — because those classes are prompt blind
  spots that NO model currently catches and NO model swap will fix.

Recommended framing for the restructure: **stakes-allocated, not uniform.** One
slot pinned to the proven block-finder (GPT), one slot on a distinct
architectural/systemic *lens prompt* (to close the blind spots), and at most one
model-diverse corroborator — rather than four models reading the same prompt.
