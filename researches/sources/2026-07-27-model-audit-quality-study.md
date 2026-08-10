# Sources: Commit-Review Panel Model-Study Series — Model Quality Study (upheld-validity axis; supersedes rate-based study)

**Date:** 2026-07-27 (study date; promoted to durable sources 2026-07-28)
**Topic:** adds the missing QUALITY axis (did a model's findings survive into the coordinator's final disposition) and re-keys the kiro-claude-opus-4-5 paid transient so it can never define a verdict. Explicitly supersedes the 2026-07-27-commit-review-model-study.md rate-based study.
**Series:** [`./2026-07-27-commit-review-panel-model-study-series.md`](./2026-07-27-commit-review-panel-model-study-series.md) (commit-review panel model-study
series, 2026-07-27 → 2026-07-28).

## Provenance

- **Source type:** study (quality-axis; SUPERSEDES rate-based study).
- **Evidence class:** Class-A — this-machine opencode DB-cited (read-only `mode=ro`); not re-derivable outside this host.
- **Original location:** `tmp/agent-runs/review-model-comparison/2026-07-27-model-audit-quality-study.md`
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
  `git log -1 --format=%H -- researches/sources/2026-07-27-model-audit-quality-study.md`.
- **Status:** durable source packet. Internal `tmp/` cross-references to peer memos
  resolve to the promoted canon paths listed in the series index above.
- **Faithfulness:** the body below the `---` delimiter is a byte-for-byte verbatim copy of the original `tmp/` study; only this provenance header is new — the study's own title, internal provenance, and cross-references to peer memos are unchanged.

---

# Commit-Review Panel — Model QUALITY Study (upheld-validity, not raw rate)

**Supersedes** `2026-07-27-commit-review-model-study.md` (same directory). That
study keyed correctly on runtime model-id and *did* separate the paid transient
in its table — but its **headline signals were rate-based** (`blockRate`,
severity-weighted **text-unique** `uniqW`) and it had **no upheld-validity
layer**. Its "retire the kiro slots / dead weight" framing then blurred a
one-day **paid transient** (`kiro-claude-opus-4-5`, no `:free`) together with
the **live-but-weak** `kiro-claude-opus-4-6:free` into a single "kiro is dead"
recommendation. This study adds the missing quality axis (did a model's findings
**survive into the final disposition**), and re-keys the transient explicitly so
it can never define a verdict.

**Decision this informs:** the panel fans out to 4 tier-1 leaves
(`commit-reviewer-a/b/c/d`), each pinned to a *different* model, all running the
*same* review prompt. Keep pure 4-way model-diversity, or restructure toward
lens-diversity / stakes-allocation? The load-bearing question here is **QUALITY**
(upheld-precision + unique-upheld catches), not who raises the most findings.

---

## Provenance & keying protocol

- **DB:** `~/.local/share/opencode/opencode.db`, opened read-only (`mode=ro`).
- **Scope:** `agent IN (commit-reviewer-a..d)` under `directory IN`
  {`vh-agent-harness`, `vh-solara`}, parented by a `commit-reviewer`
  coordinator. `ship-review` and `debate-critic` are **fixed-role single-model
  specialists** (see §11), not part of the rotating panel, and are excluded from
  the panel scorecard.
- **Sample:** **1407 coordinator rounds**, **5174 tier-1 leaves**. Rounds split
  harness 632 / solara 775.
- **Parse coverage:** coordinator disposition JSON **1394/1407 = 99.1%**; leaf
  result JSON **5050/5174 = 97.6%**. Extraction pulls the ` ```json ` block from
  *any* assistant text part (leaves often emit prose after the JSON, so
  last-part-only under-counts).
- **Keying rule (non-negotiable):** every metric is keyed on the **exact runtime
  `model.id`** — `:free` vs paid kept distinct, version kept distinct (4-5 vs
  4-6, gpt-5.5 vs gpt-latest, grok vs grok-4.3 vs grok-4.5). Slot→model rotates
  freely round-to-round; the slot letter is never used as an identity.
- **Validity oracle:** each coordinator emits a merged `findings[]` array where
  every finding carries a `disposition` ∈ {`block`, `defer`, `drop`} — this is
  the authoritative final disposition. `block`/`defer` = **UPHELD**; `drop` =
  **OVERTURNED**. A leaf's raised finding is credited to the coordinator finding
  it matches by location+issue token similarity (≥0.5, path line-numbers
  stripped). This is fuzzy: see §12 for match coverage and its limits.

### §0 — Exact-model enumeration + TRANSIENT flags

| model.id | n (leaves) | first | last | days | empty% | flag |
|---|---:|---|---|---:|---:|---|
| glm-5.2-high | 3834 | 06-21 | 07-27 | 37 | 0.0% | well-sampled |
| kiro-claude-opus-4-6:free | 2308 | 06-08 | 07-25 | 48 | **16.2%** | well-sampled, high-empty |
| gpt-latest:free | 1110 | 07-10 | 07-27 | 18 | 2.1% | well-sampled |
| grok | 1104 | 07-10 | 07-27 | 18 | 0.3% | well-sampled |
| grok-4.3 | 975 | 06-06 | 07-10 | 35 | 0.9% | well-sampled (retired 07-10) |
| gpt-5.5:free | 962 | 06-08 | 07-10 | 33 | 0.9% | well-sampled (retired 07-10) |
| glm-5.1 | 958 | 05-25 | 06-13 | 19 | 0.0% | well-sampled (retired) |
| glm-5.2 | 322 | 06-13 | 06-21 | 9 | 0.0% | moderate |
| gemini-3.1-pro-preview:free | 290 | 06-11 | 07-27 | 39 | 0.0% | mostly debate-critic (§11) |
| gpt-5.4 | 261 | 05-26 | 06-06 | 11 | 1.5% | retired |
| kiro-claude-opus-4-5:free | 178 | 07-25 | 07-27 | 3 | 4.5% | **NEW, thin (3 days)** |
| kiro-claude-opus-4-5 **(paid)** | 21 | 07-25 | 07-25 | **1** | **100%** | ⚠️ **TRANSIENT — excluded from all verdicts** |
| grok-4.5 | 30 | 07-09 | 07-10 | 2 | 13.3% | ⚠️ thin transient |
| gpt-latest-hard:free | 27 | 07-10 | 07-23 | 9 | 37.0% | ship-review only (§11) |

**The transient, handled correctly:** `kiro-claude-opus-4-5` **without** `:free`
= 21 runs, **single day (07-25), 21/21 empty** — a provider glitch on the paid
id. It is reported here **separately and never folded into any headline**. The
actual pinned Claude leaf is `kiro-claude-opus-4-6:free` (live, 2308 runs); a
newer `kiro-claude-opus-4-5:free` (paid-tier-free) appeared 07-25 (178 runs, 3
days — treated as thin/provisional, not a headline driver). Prior-study check:
this study **does not** let the 1-day paid glitch stand in for a Claude-family
verdict.

---

## §1 Reliability (keyed, over time)

Cost is `0` for every model (self-hosted `vhllm` / plan-based `zai-coding-plan`);
no per-run cost signal exists. Reliability = **empty (0-token) rate**, which has
a real downstream cost (see §3, infra blocks).

- **glm-5.2-high, grok, grok-4.3, gpt-5.5:free, gemini:** empty ≤ 2.1% — solid.
- **gpt-latest:free:** 2.1% — solid.
- **kiro-claude-opus-4-6:free:** **16.2% empty, and stable over time** — weekly
  empty-rate is a flat 10.6–19.8% band across 2026-W23→W29, **not** a glitch and
  **not** degrading. This is a persistent behavior (burns reasoning tokens, then
  emits no final text). It is a real reliability tax, but a *baseline* one — do
  not treat it as a transient to average away.
- **kiro-4-5 (paid):** 100% empty — **transient, 1 day** (see §0).

---

## §2 Block-rate — prior "gpt blocks far more" claim RE-VERIFIED (CONFIRMED)

Leaf top-level verdict distribution (rounds ≥20, clean keying):

| model | n | approve | block | split | block+split% |
|---|---:|---:|---:|---:|---:|
| **gpt-latest:free** | 848 | 672 | 144 | 8 | **17.9%** |
| **gpt-5.5:free** | 402 | 342 | 49 | 0 | **12.2%** |
| gemini:free | 30 | 26 | 4 | 0 | 13.3% (thin) |
| glm-5.2-high | 1241 | 1191 | 26 | 7 | 2.7% |
| grok | 846 | 816 | 13 | 5 | 2.1% |
| grok-4.3 | 390 | 372 | 3 | 2 | 1.3% |
| kiro-4-5:free | 171 | 164 | 2 | 0 | 1.2% |
| **kiro-4-6:free** | 1210 | 1183 | 5 | 1 | **0.5%** |

**CONFIRMED:** GPT-class models block **~36×** more than kiro-4-6 (17.9% vs
0.5%) and ~7× more than grok/glm. The prior "~50×" was right in magnitude. But
rate alone is exactly the signal the validity layer reframes (§3): gpt's blocks
also **survive** the most, so this is not trigger-happiness.

---

## §3 VALIDITY — upheld-precision (the load-bearing quality signal)

For each model: of the findings it raised that reached the coordinator's merged
disposition, what fraction were **UPHELD** (block/defer) vs **OVERTURNED**
(drop). Match coverage and caveats in §12.

| model | matched findings | block | defer | drop | **upheld-precision** |
|---|---:|---:|---:|---:|---:|
| **gpt-latest:free** | 1824 | 171 | 164 | 1489 | **18.4%** |
| gemini:free | 103 | 8 | 21 | 74 | 28.2% (thin, n<110) |
| kiro-4-5:free | 520 | 15 | 59 | 446 | 14.2% (thin, 3-day) |
| gpt-5.5:free | 588 | 44 | 26 | 518 | 11.9% |
| **grok** | 3505 | 155 | 237 | 3113 | **11.2%** |
| **glm-5.2-high** | 4828 | 168 | 261 | 4399 | **8.9%** |
| **kiro-4-6:free** | 4195 | 138 | 146 | 3911 | **6.8%** |
| grok-4.3 | 510 | 12 | 17 | 481 | 5.7% |

**Read:** `gpt-latest:free` is the quality leader — it both **blocks the most
AND its raised findings survive at the highest rate** (18.4%). This *corrects*
any "gpt is just trigger-happy noise" reading: high rate + high precision = a
genuine block-finder. At the other end, `kiro-4-6:free` (6.8%) and `grok-4.3`
(5.7%) are the trigger-*shy*-yet-still-imprecise tail — they raise a lot of
advisory volume that mostly drops.

**Infra-noise correction (important):** **15.4% of all UPHELD blocks (41/267)
are orchestrator/infra blocks** — the coordinator blocking because a *leaf
failed to produce output* ("leaf tier1_c returned non-parseable output", "model
could not execute"), not a code catch. These trace to the empty-rate of
kiro-4-6 and the kiro-4-5-paid transient — a concrete downstream cost of §1's
16% empty rate. Real capability-block yield is ~85% of the headline count.

---

## §4 Unique upheld catches (does a model find what others miss)

> **Correction flag (added 2026-07-29):** the unique-catch ranking in this §4 was
> computed **UNSCOPED** (contaminated by the co-mingled sensitive repo) and **INVERTS
> when scoped** to `vh-agent-harness`+`vh-solara`. See the §14 erratum at the end of
> this file (cites `./2026-07-28-m1-natural-miss-mechanism-study.md` §1.8). The original
> unscoped numbers below are preserved in place as the historical record.

A coord upheld finding whose *only* matched leaf-origin is model *m* = a **true
unique catch**. Normalized by participation (rounds the model was present with
≥1 upheld finding in the round):

| model | participated | rounds-with-upheld | **solo-upheld** | solo / part-upheld |
|---|---:|---:|---:|---:|
| **gpt-5.5:free** | 397 | 81 | 6 | **7.4%** |
| **grok** | 837 | 275 | 18 | **6.5%** |
| **glm-5.2-high** | 1234 | 356 | 22 | **6.2%** |
| gpt-latest:free | 837 | 275 | 8 | 2.9% |
| **kiro-4-6:free** | 1196 | 273 | 5 | **1.8%** |
| grok-4.3 / kiro-4-5:free / gemini | — | — | 0 | 0.0% |

Sharply distinct from unique-**noise**: these are drop-normalized (only upheld
solos counted). **grok and glm are the strongest *unique* contributors** —
they surface upheld findings the panel would otherwise miss more often than gpt
does per-round, even though gpt raises more total. **kiro-4-6 is the weakest
unique contributor** (1.8%) despite the highest participation — it mostly
echoes what others already catch. Concrete unique-BLOCK catches (session-cited):
- grok — `ses_05e2c9540ffeYbGVeywSj71tqa` (harness/contract_drift: undeclared
  release-ceremony tree rewrite); `ses_05fffd132ffe6Y08fAz34LD3vK`
  (solara/data_integrity: reintroduced over-archive hazard).
- gpt-latest — `ses_0603fc8ceffeisqeJp2EWmE0sz` (harness/contract_drift:
  throwing-accessor bypasses claimed malformed-rule defense).
- glm-5.2-high — `ses_0a7710847ffecMYh7JQrM4iQP8` (harness/scope: two unrelated
  files in one commit).
- kiro-4-6 — `ses_0aad59651ffePQ9oAJmkXNOH4p` (solara/data_integrity: archive
  resurrects queue items via retained OLD store pointer).

---

## §5 Capability fingerprint (which classes survive, per model)

Upheld findings by class across the whole panel (absolute upheld counts;
per-model class share is convergence-attributed, so read as fingerprint not
precision):

| class | upheld | raised | note |
|---|---:|---:|---|
| test_coverage | 446 | 1871 | highest-volume upheld class |
| contract_drift | 336 | — | high-yield, harness-heavy |
| correctness | 225 | 2134 | |
| security | 153 | ~144 | high-yield |
| data_integrity | 94 | — | high-yield (solara-heavy) |
| complexity | 42 | 166 | |
| dependency | 34 | 114 | |
| **doc_drift** | 61 | **1890** | **3.2% upheld — dominant noise class** |

**Per-model specialization** (share of that model's upheld findings):
- `gpt-latest:free` — test_coverage 30%, contract_drift 26%, correctness 13%,
  security 8%. **The contract/test lens.**
- `grok` — test_coverage 29%, contract_drift 20%, correctness 14%.
- `glm-5.2-high` — test_coverage 28%, contract_drift 18%, correctness 14%,
  security 9%. Broadest, most balanced.
- `kiro-4-6:free` — **security 16%** (highest security-share of any model),
  contract_drift 19%, correctness 16%. Its one distinctive edge is a
  **security-weighted fingerprint** — the argument against fully retiring it.
- `gpt-5.5:free` — security 20%, contract_drift 24% (security-leaning too).

No upheld class is caught by exactly one model — the top classes
(test_coverage, contract_drift, correctness, security) are covered by all four
well-sampled models. Specialization is in *emphasis*, not exclusivity.

---

## §6 Marginal-value / coverage curve

Greedy coverage of the **343 rounds that produced ≥1 upheld finding** (how many
distinct models until you cover the rounds where the panel actually caught
something):

| panel | +model | cumulative upheld-round coverage |
|---|---|---:|
| top-1 | glm-5.2-high | **83.1%** |
| top-2 | + grok | **93.0%** |
| top-3 | + kiro-4-6:free | **96.5%** |
| top-4 | + gpt-latest:free | **98.3%** |
| top-5 | + gpt-5.5:free | 100% |

**Caveat (stated honestly):** greedy pick-order is confounded by
**participation** — glm/grok/kiro-4-6 lead partly because they ran in the most
rounds, not purely on capability. So treat *order* as participation-weighted,
but the **shape is robust and participation-independent**: **diminishing returns
hit hard after 3 models** — top-2 already covers 93%, top-3 covers 96.5%, and
the 4th model adds only ~1.8pp. **A 3-model panel captures ~96–98% of upheld
blocks; the 4th slot buys < 2pp of coverage.** This is the direct answer to "is
4-model redundancy worth its cost": on coverage, no.

---

## §7 Severity calibration (leaf severity vs final disposition)

All four well-sampled models grade severity **monotonically and sanely** — high
severity survives, low severity drops:

| leaf severity | gpt-latest | grok | glm-5.2 | kiro-4-6 |
|---|---:|---:|---:|---:|
| critical | 37% | 55% | 45% | 45% |
| major | 30% | 36% | 34% | 43% |
| minor | 10% | 16% | 11% | 14% |
| info | 5% | 6% | 5% | 5% |

No model is "everything-BLOCK" or "everything-DROP". The distinguishing skew is
**volume at the bottom**: `grok` emits **2006 `info`-severity findings** (6%
upheld) and `glm` **2595** (5% upheld) — enormous low-severity chatter that
mostly drops. gpt-latest is far more concentrated in `major` (605 findings, 30%
upheld). So gpt is the *tightest* grader; grok/glm are *loudest* at the floor.

---

## §8 Override / trust signal

The coordinator's `drop` disposition = an override of a leaf's raised finding.
By upheld-precision (§3), **kiro-4-6 and grok-4.3 are overturned most** (93–94%
of their matched findings drop); **gpt-latest is overturned least** (81.6%).
By verdict agreement, gpt-latest's leaf verdict most often aligns with the final
`blocked`/`split` outcome (it drives the block); kiro-4-6 almost never does
(0.5% block rate). **Most-trusted verdict: gpt-latest. Least: kiro-4-6.**

---

## §9 Cross-repo generalization (harness Go/config vs solara TS/Python)

The patterns **hold in both repos**, and the model *ranking is stable across
both* — a strong generalization result:

| model | harness upheldP | solara upheldP |
|---|---:|---:|
| gpt-latest:free | **25.6%** (n=971) | 10.1% (n=853) |
| grok | 17.0% | 6.4% |
| gpt-5.5:free | 15.7% | 8.3% |
| glm-5.2-high | 13.0% | 5.6% |
| kiro-4-6:free | 9.3% | 4.8% |

Every model is **~2× more precise on harness than on solara**, and gpt-latest
leads both. This is a **repo/prompt effect, not a model effect**: the review
prompt and harness's own conventions are tuned to the harness's Go/config
domain; on solara's TS/Python the same models over-raise and get dropped more.
No model is strong on one language but weak on the other — the ordering is
identical.

---

## §10 Blind spots (prompt gaps, not model gaps)

Classes essentially **absent from the entire corpus of 6680 coord findings** —
no model raises them because the prompt never asks:

| class | raised | % of findings | upheld |
|---|---:|---:|---:|
| **blast_radius** | **2** | **0.03%** | 1 |
| performance | 49 | 0.73% | 3 |
| concurrency/race | 214 | 3.20% | 31 |
| complexity/maintainability | 209 | 3.13% | 18 |

And classes **raised but never upheld** (pure prompt-induced noise): `style`
(348 raised / 0 upheld), `scope_violation` (98/0), `overclaim` (59/0),
`testability` (31/0), and the dominant `doc_drift` (1890 raised / 3.2% upheld).

**Blast-radius is the single biggest blind spot** — 2 findings in 6680. This is
a **prompt gap that no model swap will fix**: adding a 5th GPT-class model does
nothing for blast-radius, but a dedicated lens prompt would. Symmetrically, the
`doc_drift`/`style`/`overclaim` noise argues the prompt should *stop* soliciting
those low-yield classes.

---

## §11 Fixed-role specialists (context, not panel)

- `debate-critic`: `gemini-3.1-pro-preview(:free)` (331 runs) + `o3-pro` (21,
  05-15..05-17 transient). Single-model critique role.
- `ship-review`: `gpt-latest-hard:free` (23) + `gpt-5.5:free` (7) — small-n,
  high empty-rate (37%) for the hard variant. Reported for completeness; too
  thin (n<25 each) for a quality verdict.

---

## §12 Honest limitations

- **Validity matching is fuzzy.** Leaf→coord-finding linkage is location+issue
  token similarity (≥0.5), because explicit leaf-attribution keys
  (`source_leaf`/`leaf`) appear on a *minority* of coord findings and
  leaf-prefixed IDs on only 1828/6680. Where multiple leaves converge on one
  coord finding, the credit is shared (all matched leaves counted) — this
  **inflates per-model class-share denominators** (§5 shows >100% artifacts) and
  makes upheld-precision a *converged-attribution* estimate, not an exact
  per-leaf ledger. The **direction and ordering are robust**; the exact
  percentages carry ±a few points.
- **Cost is unmeasurable** (all `0`); no latency column — reliability stands in.
- **Thin models flagged, never headlined:** kiro-4-5:free (3 days), gemini (n<110
  in-panel), grok-4.5 (n=30) — excluded from load-bearing claims.
- **The paid `kiro-4-5` transient is quarantined** (§0) — the specific error the
  prior study's framing risked.
- **No conclusion drawn from n<20.**

---

## §13 Bottom-line read (input to the panel-restructure decision)

> **Correction flag (added 2026-07-29):** the "keep one grok/glm diversity slot for
> unique catches" rationale below is **superseded on-repo** — it rests on the unscoped §4
> ranking that inverts when scoped (GPT dominates unique-catch scoped). See the §14
> erratum at the end of this file.

**What earns a slot on QUALITY (not rate):**

1. **`gpt-latest:free` — keep, pin it.** Highest upheld-precision (18.4%,
   25.6% on harness), highest block-rate, least overturned, most trusted
   verdict, tightest severity grading. It is the demonstrated **block/contract
   finder**. Rate + validity agree: this is the load-bearing reviewer.
2. **One diversity slot from {grok, glm-5.2-high}.** These are the top *unique*
   upheld contributors (6.5% / 6.2% solo-upheld) — they catch what gpt misses.
   grok edges glm on precision (11.2% vs 8.9%); glm has zero empty-rate and the
   broadest fingerprint. Either earns the second slot; running *both* is where
   diminishing returns begin (top-3 = 96.5%).
3. **`kiro-4-6:free` — the marginal slot.** Weakest on every quality axis
   (6.8% precision, 1.8% solo-upheld, 16% empty, most overturned) **except one**:
   the most **security-weighted** fingerprint (16% of its upheld). It is not
   dead weight — but it earns its slot only as a *security lens*, not as a
   general 4th reviewer, and its 16% empty-rate directly manufactures the §3
   infra-block noise. **Repoint or de-weight, don't blindly retire** — and
   *never* on the basis of the kiro-4-5-paid transient (§0).

**How many models?** On coverage, **3 models capture 96–98% of upheld blocks;
the 4th adds < 2pp.** Pure 4-way model-diversity is **not** justified by
quality — 3 of 4 slots are near-interchangeable on upheld catches.

**Model-diversity vs lens-diversity vs stakes-allocation — one-line read:**
> The panel is over-invested in redundant model-diversity and under-invested in
> lens-diversity — spend the 4th slot on a **blast-radius / concurrency /
> complexity lens prompt** (the true blind spots, which no model swap fixes),
> pin **one GPT-class block-finder** on stakes, keep **one grok/glm diversity
> slot** for unique catches, and repoint the weak kiro slot to a **security
> lens** rather than retiring it on a one-day transient.

---

## §14 Erratum (2026-07-29) — §4 unique-catch ranking was UNSCOPED and INVERTS when scoped

**Erratum type:** ADDITIVE correction. The original §4 and §13 numbers and prose are
**preserved in place** as the historical record (marked with correction flags at §4 and
§13 above); this section records the scoped recompute and supersedes the diversity-slot
rationale on-repo. Nothing above is deleted or silently rewritten.

**Source:** [`./2026-07-28-m1-natural-miss-mechanism-study.md`](./2026-07-28-m1-natural-miss-mechanism-study.md) §1.8 (new series member #11).

### What was wrong

The §4 "Unique upheld catches" table and the §13 bottom-line that consumes it ("**grok
and glm are the strongest unique contributors → keep one diversity slot from {grok,
glm}**") were computed **UNSCOPED** — over `directory IN {vh-agent-harness, vh-solara,
<a sensitive third-party repo>}` with the sensitive third-party repo included. That repo dominates
the unscoped volume and its leaf/findings mix skewed the unique-catch ranking.

### The scoped recompute (the inversion)

**Scoped correctly (`vh-agent-harness` + `vh-solara` only), the unique-catch ranking
INVERTS.** Scoped numbers from the validated M1 quantitative leg (operator-anchor-verified):

| model | §4 solo-upheld (UNSCOPED, canon) | **M1 scoped solo-upheld (co-present)** | **M1 scoped solo / matched-upheld** | **M1 scoped miss-rate** |
|---|---:|---:|---:|---:|
| `gpt-latest:free` | 8 (2.9%, near-bottom) | **114** | **57.3%** | **33.1%** |
| `gpt-5.5:free` | 6 (7.4%, top) | 24 | 58.5% | **29.9% (lowest)** |
| `grok` | 18 (6.5%, top) | 65 | 43.6% | 41.0% |
| `glm-5.2-high` | 22 (6.2%, top) | 46 | 32.9% | 44.6% |
| `kiro-4-6:free` | 5 (1.8%, bottom) | 10 | 21.7% | 56.5% |

**The inversion is on TWO independent scoped cuts:**

1. **Unique-catch ranking flips.** Unscoped canon ranked grok/glm on top and gpt-latest
   near the bottom (2.9%). Scoped, **gpt-latest is the TOP unique catcher** (114 solo
   findings — more than grok and glm COMBINED) and grok/glm rank below it. The canon's
   "grok and glm are the strongest unique contributors" is an artifact of the sensitive
   repo's leaf mix, not a property of the in-scope panel.
2. **Miss-rate confirms the flip from the other side.** Scoped miss-rate: GPT-class misses
   LEAST (gpt-5.5 29.9% / gpt-latest 33.1%); grok 41.0%; glm 44.6%; kiro-4-6 56.5%. GPT is
   simultaneously the **strongest unique catcher AND the lowest misser** scoped — the
   mirror image of the canon's "keep grok/glm for unique catches" framing.

### What this changes in §13

- **STRENGTHENED:** "pin one GPT-class block-finder on stakes." Scoped evidence is
  stronger than the canon's — GPT leads not only on block-rate + upheld-precision (§2/§3)
  but ALSO on unique catches and miss-rate. GPT is the load-bearing reviewer on every
  scoped quality axis.
- **SUPERSEDED on-repo:** "keep one grok/glm diversity slot for unique catches." Scoped,
  grok/glm are NOT the top unique contributors (GPT is). They still contribute real unique
  catches (grok 65, glm 46 solo), so a diversity slot is not worthless — but its
  justification shifts from "they catch what GPT misses MOST" to "redundant coverage +
  GPT-independent agreement." The diminishing-returns coverage curve (§6: 3 models ≈
  96.5%) is unaffected (it was already participation-weighted and holds scoped).

### Comparability caveat (from M1 §1.8, stated honestly)

The two solo-upheld tables use DIFFERENT denominators (canon: solo / rounds-with-upheld;
M1: solo / matched-upheld), so the percentages are not directly comparable across tables.
The inversion rests on (a) the solo-upheld COUNT ranking within the scoped co-present
population (apples-to-apples: gpt-latest 114 > grok 65 > glm 46) and (b) the miss-rate,
which is definitionally comparable and independently confirms the direction. The scoped
population is co-present-only (a subset) and uses the refined ≥0.45 match threshold (vs
canon's ≥0.5); these are methodological refinements, not contamination, and the direction
is robust because it appears on BOTH cuts.

### Recommendation

Until an official recompute of §4/§13 lands (scoped to `vh-agent-harness`+`vh-solara`,
excluding the sensitive repo, co-present subset, §0 quarantine applied), **any citation of
§4/§13's "grok/glm are the strongest unique contributors" should carry this erratum
flag.** The scoped numbers already exist in the M1 memo §1.8 and its validated
`m1_output.json` (`cross_check_section4_solo_upheld` + `per_model_miss_rate`); the
official recompute is a canon-side transcription + matching-threshold sensitivity check,
not new measurement.
