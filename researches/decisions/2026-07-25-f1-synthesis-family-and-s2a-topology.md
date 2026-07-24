# Decision: F1 Synthesis Family + S2-a Topology (three UNION families)

**Date:** 2026-07-25
**Status:** Accepted (record-of-decision). No code lands in this slice; this is
a record-of-decision that fixes the control topology (Decision 1) and the F1
mechanism architecture (Decision 2) at decision granularity. The full sliced
build plan is a separate build input; this memo captures the citable decisions
that gate it.
**Supersedes:** none (narrows the HYBRID reframe and the operator-visibility
study into a concrete topology + F1 mechanism; does not replace either).
**See also:**
[`../sources/2026-07-25-seven-controls-property-map.md`](../sources/2026-07-25-seven-controls-property-map.md)
(the seven-controls property map the S2-a debate adjudicated).
`tmp/operator-visibility/2026-07-24-opencode-history-visibility-study.md`
(the Class-A evidence study — session-local / transient, NOT committed canon).
[`./2026-07-23-vh-solara-orchestration-field-report-disposition.md`](./2026-07-23-vh-solara-orchestration-field-report-disposition.md)
(the HYBRID closure vocabulary + the union list the Decision-1 UNION
candidates extend — §2026-07-24 addendum, union list ~L378-380).
[`./2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md`](./2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md)
(the authority line Decision 2's gate-shaped conversions respect — ~L125-135).
[`./2026-07-24-behavioral-closure-pilot.md`](./2026-07-24-behavioral-closure-pilot.md)
+ commits `7f95f29`, `4d8d725` (the shipped behavioral-closure pilot whose
parser principles F1 reuses, doctor check #14).

## Framing

The 2026-07-24 operator-visibility evidence study (Class-A vh-solara,
session-local at `tmp/operator-visibility/2026-07-24-...`) found the
demonstrated **26–28h pivot-delay bottleneck** was synthesis +
option-generation + persistence failure, NOT display. A researcher property
map (cited above) then mapped seven proposed controls across
owner/cadence/authority/failure-mechanism/data-role. A phase-1 debate
adjudicated one-gate-set (S1) vs two-UNION-families (S2) and converged on
**S2-a with HIGH confidence and no contradictions**. This memo records the
two decisions that came out of that convergence:

1. the **topology** — how the seven controls group into UNION families; and
2. the **F1 mechanism architecture** — the canonical/derived boundary, the
   NEW SLOTs, the gate-shaped conversions, and the F1→F2 emit contract.

It is composed at **decision granularity**, not full build-plan granularity,
so it can serve as the citable durable basis for (a) the F1 `build` slices,
(b) the F2 and F3 mechanism briefs, and (c) the separately-approved canon
promotion edits to the HYBRID union list and the authority-line table.

## Decision 1 — Topology: S2-a, THREE UNION families

**Verdict (confidence HIGH, no contradictions):** the family-defining axis is
**data-role** (producer vs renderer). **Authority** is an orthogonal
constraint that isolates DAY-0; it is not the primary axis.

- **S1 (one gate-set / MERGE) — REJECTED.** No two controls share a property
  identity that forces MERGE. The falsifying test: every candidate pair
  differs on at least one of owner/cadence/failure/data-role.
- **S2-b (authority as the primary family axis) — REJECTED.** Authority
  explains DAY-0's separation cleanly, but it *collapses* the
  producer-vs-renderer distinction among the other six (R1, R3, P-a, R5, P-b,
  P-c all INFORM). Treating "all INFORMS" as one family hides the load-bearing
  producer-vs-renderer split that the study identified as the bottleneck.

### The three families

- **F1 — synthesis-producing (producer axis, load-bearing, the demonstrated
  bottleneck):** **R1, R3, P-a.** All three INFORM. Their producer axis —
  cross-lane join (R1), redesign-option generation (R3), counter-evidence +
  weakest-claim generation (P-a) — is the synthesis act the study found
  absent.
- **F2 — rendering / persistence (renderer/consumer axis, necessary-but-
  insufficient):** **R5, P-b, P-c.** All three INFORM. They are pure
  renderers/consumers of synthesis produced upstream (R5 persists what the
  operator authored; P-b renders a captured-evidence slot; P-c renders
  already-produced verdict/counter-evidence salient).
- **F3 — design-gate (producer + BLOCKS, pre-code cadence, separate family):**
  **DAY-0.** The only control whose authority is BLOCKS and whose cadence is
  pre-code/BUILD-READY.

### MIXED resolution (load-bearing)

R1 and P-a are **declared ONCE in F1** — their canonical identity is the
producer act (R1 linkage-join; P-a counter-evidence-generation). Their
rendering/persistence surfaces are **DERIVED into F2** under HYBRID
declare-once-derive-everywhere — they are NOT duplicated as second F1
entries. This is what makes producer axis canonical: the evidence-priority
ordering (generate precedes render) makes the producer act the declared
source and the rendering a derivation. Mis-declaring them in F2 would
silently demote the synthesis act the study found missing.

### DAY-0 placed explicitly (no silence)

DAY-0 is placed in **F3**, distinct on authority (BLOCKS) **AND** cadence
(pre-code) *simultaneously*. It is recorded here explicitly, not parked,
because silence on DAY-0 is the one outcome the operator rejected as a fixed
input: DAY-0 was the cheaper of the two interventions to have gotten right
the first time, and a silent or deferred placement would reproduce the
§4.3-errata-card failure (a trigger that fired on three consecutive releases
and stayed `draft` through all three).

### Falsifying-test summary

No pair of controls has a full shared property identity that forces MERGE.
The six non-DAY-0 controls share INFORMS but differ on owner / cadence /
failure-mechanism / data-role. DAY-0 is unique on BLOCKS authority + pre-code
cadence. The full per-control falsifying matrix is in the source packet's
Day-0 disposition and Seam-coverage sections; this memo records the verdict
shape.

## Decision 2 — F1 mechanism architecture

### Intra-family composition

F1 is **ONE versioned, domain-free `F1SynthesisEnvelope`** containing exactly
one entry per F1 family: `r1_cross_lane_join`, `r3_redesign_fork`,
`pa_counter_evidence`. An entry may carry a `not_triggered` /
`not_applicable` disposition, but **omission is NOT acceptable for
applicability** — an applicable seam with no entry is incomplete.

The envelope is reconciled against the shipped behavioral-closure pilot
(`7f95f29` + `4d8d725`, doctor check #14). F1 **reuses its principles**
(closed vocabulary, pure parser/validator, unknown-value rejection,
canonical-template/parser agreement, structural-consistency-≠-truth) but
**does NOT reuse its "absent token passes" behavior** — at an applicable F1
seam, a missing declaration is incomplete, not silently satisfied.

### Canonical-vs-derived boundary (the MIXED resolution made concrete)

| Control | F1 canonical (producer acts) | F2-derived surface | Prohibited duplication |
|---|---|---|---|
| **R1** | property identity; source/lane refs; ancestry; cross-lane join; agreements/contradictions/gaps; hazard↔symptom links; merge/union disposition; bounded conclusion; linkage to R3/P-a | committed doc path+timestamp; chronological streak; headings; filters; status badges; summaries; renderer version | F2 must not recompute joins, infer relationships, replace ancestry, or author a second conclusion |
| **P-a** | probe target; falsification question; checked scope+sources; result enum; real counter-evidence refs; limitations; weakest claim; confidence | options×costs×evidence-against×weakest-claim×reversal-cost table; ordering; formatting; display summaries | F2 must not invent counter-evidence, convert bounded absence into global absence, change weakest claims, or fill missing probes |
| **shared envelope** | version, cycle ID, applicability, canonical entry IDs, source refs, validation result, semantic digest | storage locator, write timestamp, view-model/renderer version, verified-media attachment metadata | F2 cannot replace/recalculate semantic content under the same digest |

### R1 accumulator — NEW SLOT `r1_cross_lane_join`

A deterministic join over **explicit property IDs**, lane/producer-act IDs,
source ancestry, agreements/contradictions/unresolved-gaps, hazard-to-symptom
links, and MERGE/UNION disposition. Snapshots are **immutable** — a changed
conclusion creates a new `synthesis_cycle_id` and never mutates a prior
envelope. This is **NOT a claims registry or query service** (HYBRID).

**Hazard-survival rule:** a hazard link survives only when its canonical R1
record retains `hazard_ref → symptom_refs → original source_refs → ancestry →
contradiction/gap → consuming R3 option IDs → consuming P-a probe IDs`. The
chain is explicit; survival is not inferred.

F2 writes a committed deterministic Markdown projection of the canonical
record (the projection path is an F2 decision, not an F1 one). F1's contract
on that projection: it must be operator-readable, doctor-readable,
digest-bound, NOT a claims DB, and it must never infer new joins.

### R3 fork-trigger — NEW SLOT `r3_redesign_fork`

**Trigger:** `repair_intent == present AND structural_review_outcome == non_pass`.
The `structural_review_outcome` value must map to an **explicit closed set of
existing verdict/source types** — build must not infer the mapping from
narrative.

**Required record:** a continue-repair option + a materially-distinct
redesign option (invalid if it merely renames, delays, or subdivides the same
repair) + P-a links + operator disposition (`pending`/`selected`/`rejected`/
`deferred`).

**Persistence across compaction:** the canonical entry + the committed
projection. The `.local/coordinator/tasks/` card is **transport, not durable
truth**.

**Gate-shaped conversion REQUIRED.** Merely instructing the coordinator to
surface the fork is optional prose that reproduces the failure (the study
found zero pre-pivot "redesign" mentions system-wide). The conversion is a
**repair-routing / task-lifecycle validator** that checks:
trigger-recognized + both-options-exist + both-have-P-a +
disposition-recorded-before-transition + valid digest/refs. **The GATE (not
the coordinator) applies the refusal; `doctor` audits.** The gate validates
**completeness**, not redesign quality (quality is an operator judgment).

### P-a counter-evidence — NEW SLOT `pa_counter_evidence`

**Mandatory probes** for: every material R1 conclusion + the continue-repair
option + every redesign option + any recommended option / weakest-claim.

**Result enum:**
- `found` — requires real evidence refs.
- `not_found_in_checked_scope` — requires method+scope; this is **NOT global
  absence**.
- `unavailable` — requires an explicit limitation.
- `not_run` — cannot silently satisfy a coverage requirement.

**Fabricated evidence** (including synthesized-number charts without
provenance) is **invalid under any result**.

**Binding via three layers:** canonical schema (mandatory entry + coverage) +
pure validator (every applicable target resolves to a probe) + safety-layer
gate (rejects missing/dishonest probes). The prompt template stays an
**authoring aid, not enforcement**.

### F1 → F2 emit-boundary contract

F1 emits `ValidatedF1Emit { canonical_envelope, semantic_digest,
validation_disposition: complete }`. This is a **candidate / informational
artifact with NO transition authority** — consistent with the
model-output-is-candidate invariant.

**F1 preconditions before emit:** one entry per family; applicability
explicit; each entry validates independently; cross-refs resolve; R1 joins
complete; every R1 conclusion + R3 option has P-a coverage; a triggered R3
has materially-distinct options; conflicts/gaps explicit; digest covers the
canonical fields.

**F2 MAY:** persist canonical bytes / a lossless rep; write the committed
Markdown projection; render streak + decision-table; add storage/timestamp/
renderer/view metadata; attach captured-or-verified media metadata; verify
source digest matches.

**F2 must NOT:** join evidence; merge/split properties; generate
alternatives; generate counter-evidence; add conclusions; infer gaps;
reinterpret bounded absence as global absence; reconstruct missing F1 from
prose; treat unverified media as evidence; or emit partial as complete.

**Digest-binding:** every persisted/rendered F2 artifact retains
`synthesis_cycle_id` + `entry_ids` + `semantic_digest` + projection/renderer
version. A changed canonical field requires a **new F1 emit**, not an
in-place F2 correction.

## Fixed inputs carried (settled, non-reopenable)

- **Evidence-priority ordering:** join → generate-alternatives → persist →
  render → attach-verified-media.
- **Fabricated-chart hard rule:** a model-fabricated chart with synthesized
  numbers / no provenance is **MORE dangerous than prose** (visual
  credibility amplifies capture). Evidence-grade = captured-or-verified,
  always.
- **Authority line:** F1 controls INFORM; any blocking = gate-shaped
  conversion to the safety layer (doctor / commit-gate / release / tests).
  The coordinator has NO transition authority.
- **HYBRID:** a thin domain-free closure VOCABULARY; verification/enforcement
  federated per property; coordinator non-authoritative; synchronous;
  explicitly NOT a persistent claims database. Same property → MERGE;
  different property → UNION.

## Authority-line audit

Every F1 mechanism classified. No F1 mechanism gives the coordinator
transition authority.

| Mechanism | Classification |
|---|---|
| R1 join | INFORMS (F1 producer) |
| R1 streak rendering | derived persistence (F2 projection writer) |
| R3 redesign-option generation | INFORMS (F1 producer) |
| R3 presentation | INFORMS (coordinator/operator surface) |
| **require-fork-before-repair-route** | **GATE-SHAPED CONVERSION** (task-lifecycle / repair-routing validator) |
| **record-disposition-before-transition** | **GATE-SHAPED CONVERSION** (same validator) |
| P-a probe generation | INFORMS (F1 producer) |
| **require-P-a-target-coverage** | **GATE-SHAPED CONVERSION** (envelope / lifecycle completeness validator) |
| audit committed F1/F2 consistency | GATE-SHAPED CHECK (`doctor`) |
| determine-whether-evidence-actually-true | federated verifier responsibility (NOT inferred by a structural gate) |

## Contradictions resolved

1. **Study's "persistent queryable arc-ledger" vs HYBRID no-claims-DB** →
   resolved as immutable snapshots + committed deterministic projections,
   NOT a query service.
2. **R1 "owner = committed markdown" wording vs the MIXED resolution** → F1
   owns the semantic join; F2 writes the projection; the projection is not a
   second source of truth.
3. **Behavioral-closure absent-token semantics differ** → F1 reuses the
   parser pattern, NOT the absent-token-passes policy, at applicable F1
   seams.
4. **Structural validation ≠ truth** (a digest-valid complete envelope can
   still contain weak reasoning) → the behavioral-closure honesty caveat,
   stated explicitly, stays explicit.
5. **Planner-suggested generic `internal/memory/claims` registry** →
   REJECTED as ungrounded + a HYBRID violation. Build must localize existing
   seams first.

## Open questions for build (named decision-points, NOT silent assumptions)

1. The exact `templates/core/` source paths for the rendered
   closeout / repair-routing / state-library seams. Generated `.opencode/`
   files are NEVER edited directly — the source is under `templates/core/`.
2. The committed F2 projection path. (F2 brief decides; F1 only requires
   operator-readable + doctor-readable + digest-bound + not-a-claims-DB.)
3. Stable ancestry identity. v1 should **disclose overlap**, not claim
   universal source independence.
4. Mapping `structural_review_outcome == non_pass` to the repo's actual
   closed verdict enums. No narrative heuristics.
5. The P-a incomplete-result policy baseline: `not_run` fails the applicable
   completeness gate; `unavailable` is structurally valid only with an
   explicit limitation and cannot support a strong/proven claim. Whether
   high-risk release seams also block on `unavailable` needs an explicit
   policy owner.
6. Operator-disposition timing: identify the exact lifecycle transition
   where `pending → selected/rejected/deferred`. The gate operates there,
   not via coordinator authority.

## Next steps (gated on THIS memo landing — do NOT execute in this slice)

- **Canon promotion edits** (operator approved in principle; separate slice,
  gated on this memo): add the F1/F2/F3 boundaries to the HYBRID union list
  in the 2026-07-23 disposition memo's addendum; add a Block-BUILD-READY row
  to the authority-line table in the 2026-07-22 memo (anticipating F3).
- **F2 mechanism brief** (consumes `ValidatedF1Emit`).
- **F3 mechanism brief** (queued after F2; the ordering is an operator
  review-bandwidth constraint, not a mechanism-independence constraint).

## Evidence / Provenance

| Claim | Verifying artifact / command | Verified |
|---|---|---|
| Behavioral-closure pilot shipped + doctor #14 | `git show --stat 7f95f29`, `git show --stat 4d8d725` (verdict/crux gate + doctor check #14) | yes |
| HYBRID union list line range cited | `researches/decisions/2026-07-23-vh-solara-orchestration-field-report-disposition.md` ~L378-380 (Addendum 2026-07-24) | yes |
| Authority-line wording line range cited | `researches/decisions/2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md` ~L125-135 (Authority and transition routing) | yes |
| Evidence study (Class-A, session-local) | `tmp/operator-visibility/2026-07-24-opencode-history-visibility-study.md` exists (transient; NOT committed canon) | yes |
| DAY-0 has no shipped seam | grep `.local/coordinator/tasks/` for design-gate/build-ready/adversarial/ownership-hazard → zero matches | yes (source packet) |
| CoreOutputs (`ae5b30d`) is P-b's closest seam | `git show --stat ae5b30d` (capability-owned CoreOutputs filtering) | yes (via 2026-07-22 memo reuse-wins) |
| Property map (input) | `researches/sources/2026-07-25-seven-controls-property-map.md` | yes (this slice) |
| Numeric pivot-delay claim (26–28h) | Class-A, asserted-by-operator, NOT re-derivable here | n/a (by design) |

House style: this memo follows the `2026-07-22` / `2026-07-23` convention
(bolded-metadata frontmatter; Framing → Decision → Mechanism → Authority →
Contradictions → Evidence), not the YAML-frontmatter convention, per the
2026-07-22 memo's house-style note.
