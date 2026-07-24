# Sources: Seven-Controls Property Map (operator-visibility → S2-a topology)

**Date:** 2026-07-25
**Topic:** the seven proposed controls mapped across owner / cadence / authority /
failure-mechanism / data-role, derived from the 2026-07-24 operator-visibility
evidence study. This packet is the source-anchored material the S2-a topology
debate was decided on.
**Decision memo:**
[`../decisions/2026-07-25-f1-synthesis-family-and-s2a-topology.md`](../decisions/2026-07-25-f1-synthesis-family-and-s2a-topology.md).

## Provenance

- **Source type:** researcher property map.
- **Derived from:** the 2026-07-24 operator-visibility evidence study
  (`tmp/operator-visibility/2026-07-24-opencode-history-visibility-study.md`).
  That study is **Class-A vh-solara evidence** — readable only on this machine,
  NOT re-derivable inside vh-agent-harness, and **session-local / transient**
  (it lives under `tmp/`, not committed canon). Treat the study's numeric claims
  (26–28h pivot delay, etc.) as asserted-by-operator, unverified-by-us; the
  property map below rests on them only in the DAY-0 row (Class-A-anchored need)
  and the framing that the bottleneck is synthesis + option-generation +
  persistence failure rather than display.
- **Status:** adjudicated. The map's researcher-leaning hypothesis (S2, two
  UNION families) was the input to a phase-1 debate; the debate converged
  **S2-a (THREE families)** and **SUPERSEDED** the two-family hypothesis here.
  See the decision memo for the verdict. This packet preserves the raw map; it
  does not carry the verdict.

## Confidence legend

- **HIGH** — anchored on a shipped seam in THIS repo (verified structure).
- **PARTIAL** — anchored on a shipped seam for some cells; others are inference
  or a NEW SLOT.
- **INFERENCE** — derived from the study / map reasoning, not a shipped seam.
- **GAP** — no shipped seam covers the cell; NEW SLOT required.

---

## Property map

Seven rows × (owner / cadence / authority / failure-mechanism / data-role /
P-C-R-label). `NEW SLOT` marks a cell with no shipped seam that covers it.

| # | Control | Owner | Cadence | Authority | Failure mechanism | Data role | P/C/R |
|---|---|---|---|---|---|---|---|
| **R1** | Cross-lane accumulation + operator-surface rendering + hazard↔symptom linkage survival | committed doc read by `doctor`; **NEW SLOT** for cross-lane accumulator | at every gate/verdict emission + at repair-routing (cross-lane) | **INFORMS** (doc renders); no shipped gate accumulates cross-lane | absent synthesis (E1+E2+E3 never joined) + provenance decay | CONSUMES per-lane synthesis; PRODUCES the cross-lane join (linkage); renders the streak | **MIXED**: PRODUCER (linkage axis) / RENDERER (persistence axis) |
| **R3** | "step back to design" surfaced/recorded as a fork at repair-routing | coordinator surfaces (**INFORMS**); `researches/decisions/` memo + `.local/coordinator/tasks/` card store (both partial); **NEW SLOT** for fork-trigger | at repair-routing following a structural-class NEEDS-CHANGES | **INFORMS** (raw "block routing" would cross authority line → gate-shaped conversion) | absent option-generation (zero pre-pivot "redesign" mentions system-wide) | CONSUMES review verdict; PRODUCES the redesign alternative | **PRODUCER** |
| **R5** | operator-authored syntheses → durable artifacts when written | operator authors; harness persists; closest seam P2-B findings-delta (binds AGENT closeouts not OPERATOR messages); **NEW SLOT** for operator-message binding | at operator-authoring (when written in chat) | **INFORMS** (persistence into committed doc); no transition blocked | absent persistence (syntheses lived only as chat) | CONSUMES operator's synthesis; renders it durable | **CONSUMER + RENDERER** (operator is producer) |
| **P-a** | decision-surface contract: STRUCTURED artifact (options × costs × evidence-against × weakest-claim × reversal cost) | agent authors; template/discipline; closest seam disposition house-style table + prompt-guide grilling rule 3; **NEW SLOT** for binding contract | at every operator-facing decision request | **INFORMS** (template discipline; "prose discipline, not enforcement") | malformed rendering (free narrative instead of structured options) | renders structured + PRODUCES counter-evidence & weakest-claim to populate mandatory slots | **MIXED**: RENDERER (slot contract) + PRODUCER (mandatory counter-evidence generation) |
| **P-b** | evidence-grade visual attachment: CAPTURED evidence (real render/screenshot/diff, or chart from verified data WITH provenance); harness defines the contract SLOT (capability-class, media-perception precedent), never vendor/transport | harness defines SLOT; closest seam CoreOutputs (`ae5b30d`) + `core/media-perception` capability-class; **NEW SLOT** for provenance-carriage | at attach-time (visual attached to decision/closeout) | **INFORMS** at minimum (slot contract); gate-shaped version mirrors behavioral-closure #14 | provenance decay / fabricated evidence (fabricated-chart hard rule) | CONSUMES captured evidence upstream; renders via slot; does NOT create evidence | **RENDERER** (slot contract) |
| **P-c** | salience discipline: progressive disclosure; headline carries decision frame + counter-evidence + weakest-claim STRUCTURALLY | agent authors headline; template/discipline; closest seam CLOSEOUT_TEMPLATE section structure + prompt-guide Structured Findings + `2026-07-16-adoption-progressive-disclosure` (partial) | at render-time (decision/closeout/headline rendering) | **INFORMS** (pure discipline); gate is on underlying token (behavioral-closure #14), NOT the salience layer | malformed/incomplete rendering (clean headline vs buried INCONCLUSIVE) | CONSUMES already-produced verdict/counter-evidence; renders it salient | **RENDERER** (pure) |
| **DAY-0** | adversarial design-gate failure (premature BUILD-READY despite named ownership hazard) | **NEW SLOT** — no shipped design-phase adversarial gate in THIS repo; closest precedents miss (motivation-check P1-B advisory not adversarial; behavioral-closure #14 wrong cadence; §4.3 release-boundary wrong boundary) | at BUILD-READY / design-gate crossing (before code written) | **BLOCKS** (must refuse BUILD-READY when named hazard unresolved) — per authority line, blocking REQUIRES safety-layer gate | premature confidence (BUILD-READY/HIGH despite named "core blocker" hazard) | CONSUMES design + named hazards; PRODUCES the adversarial counter-case; BLOCKS on it | **PRODUCER** (adversarial counter-case) + **GATE** (block) — NOT a renderer |

---

## Per-control anchored notes (load-bearing cells)

- **R1 — the crux is the linkage-vs-persistence split.** The persistence half is
  a **RENDERER** (a `docs/checkpoints/` scan already ships). The linkage half is
  a **JOIN act = synthesis = PRODUCER**. These are two different data-roles
  inside one control. The cross-lane accumulator itself is a **NEW SLOT**: a
  per-lane verdict token (the behavioral-closure `7f95f29` token) scaled
  cross-lane. It is a UNION candidate vs per-lane behavioral-completion (different
  property identity: cross-lane join ≠ single-lane completion).
- **R3 — generating the redesign alternative IS synthesis** (step 2 of
  evidence-priority ordering: join → generate-alternatives). A raw "block
  routing" crosses the authority line (coordinator would gain transition
  authority), so the block must be **gate-shaped** (a repair-routing / task-
  lifecycle validator), not coordinator prose.
- **R5 — the operator is the producer; R5 is the persistence half.** The closest
  seam is P2-B findings-delta, but P2-B binds **agent** closeouts, not **operator**
  messages → R5 is a UNION candidate OR an extension of P2-B's subject scope.
  Either way the producer/renderer split is load-bearing: the synthesis is
  authored by the operator (outside the harness); the harness only persists.
- **P-a — dual nature is load-bearing for the S1/S2 debate.** The slot contract
  (options×costs×...×reversal-cost) is rendering. Populating the mandatory
  evidence-against + weakest-claim slots is synthesis (production). Treating
  P-a as renderer-only would hide the producer axis; treating it as
  producer-only would hide the template discipline.
- **P-b — the slot is rendering; the evidence is upstream.** The
  fabricated-chart hard rule governs (a model-fabricated chart with synthesized
  numbers and no provenance is MORE dangerous than prose). Honest ceiling for the
  slot is a **structural / provenance-presence check, not content-truth**, per
  the behavioral-closure honesty caveat (a digest-valid envelope can still carry
  weak reasoning).
- **P-c — pure rendering of already-produced synthesis.** No production axis.
- **DAY-0 — the only BLOCKS-authority control, the only one with no shipped
  seam.** Verified by grep across `.local/coordinator/tasks/` for
  `design-gate`, `build-ready`, `adversarial`, `ownership-hazard` → zero
  matches. The study itself places DAY-0 **OUTSIDE the R1–R7 visibility lane**
  ("No visibility mechanism fixes that") — it is a design-phase adversarial
  gate, not a synthesis/rendering control.

## Day-0 disposition — mapped as the 7th row, NOT parked

Parking-with-trigger-only would reproduce the case-study §4.3 errata-card
failure: a trigger that **fired on three consecutive releases** and stayed
`draft` through all three. The lesson on record is *"it is not only severity
verdicts that decay; the trigger machinery itself fails mechanically."* So
DAY-0 is mapped as a full row carrying its own property identity, not deferred
behind a trigger.

**Property-identity test (HYBRID):** DAY-0 ≠ behavioral-completion ≠
defer-not-drop ≠ HEAD-progress ≠ motivation-satisfaction ≠ findings-retention
≠ rewrite-parity → a **distinct fail-closed UNION property**.

**Family-ownership claim:** the design-gate / adversarial-review family is
**DISTINCT** from the synthesis-accumulation / rendering family. The split
turns on TWO dimensions simultaneously:

1. **Authority** — DAY-0 is the only control whose authority is BLOCKS.
2. **Cadence** — DAY-0 fires **pre-code** at BUILD-READY; the others fire at
   repair-routing / closeout / render-time.

Either dimension alone is insufficient; together they force a third family.

## Seam-coverage gaps

All seven controls need a NEW SLOT for at least one cell. **No control is fully
covered by a shipped seam.** DAY-0 has the largest gap (no seam covers ANY
cell). The other six have partial seams covering the persistence / rendering
half but NOT the synthesis-production or cross-lane-accumulation half.

| Control | NEW SLOT | Closest precedent (why it misses) |
|---|---|---|
| R1 | cross-lane accumulator (join) | per-lane behavioral-closure token (`7f95f29`) + `docs/checkpoints/` scan — but neither scales cross-lane |
| R3 | fork-trigger + redesign-option record | defer-card trigger machinery + decision-memo house style — both partial; neither binds a redesign alternative |
| R5 | operator-message → durable-artifact binding | P2-B findings-delta (binds agent closeouts) + operator-authored memo convention — subject scope mismatch |
| P-a | binding decision-surface contract | disposition house-style table — exists as convention, not a binding contract |
| P-b | provenance-carriage for the visual slot | CoreOutputs (`ae5b30d`) + `core/media-perception` capability-class — slot precedent, no provenance contract |
| P-c | salience/progressive-disclosure discipline | CLOSEOUT_TEMPLATE section structure + progressive-disclosure (`2026-07-16-adoption`) — partial, advisory |
| DAY-0 | adversarial design-gate at BUILD-READY | behavioral-closure #14 (right authority shape, **wrong cadence** — post-code); motivation-check P1-B (right cadence, **wrong authority** — advisory); §4.3 release-boundary (right pattern, **wrong boundary** — release, not design) |

## Evidence strength per row

| Row | Evidence strength |
|---|---|
| R1 | existing-seam-anchored (persistence) + INFERENCE (linkage) |
| R3 | PARTIAL + INFERENCE (fork-generation) |
| R5 | existing-seam-anchored |
| P-a | slot-anchored + INFERENCE (counter-evidence production) |
| P-b | existing-seam-anchored |
| P-c | existing-seam-anchored |
| DAY-0 | GAP (no shipped seam) + Class-A-anchored need |

## Researcher non-binding hypothesis (SUPERSEDED by the debate verdict)

The map as authored leans **S2 (two UNION families)**; the split turns on
data-role (synthesis-producing vs rendering-only), with DAY-0 forcing the
two-vs-three question. **S1** (one gate-set / MERGE) was rejected because no
two controls share a property identity that forces MERGE.

The live question for the phase-1 debate was **S2-a vs S2-b** and whether
DAY-0's authority or data-role is family-defining. The debate's verdict is
recorded in the decision memo and **SUPERSEDES this hypothesis** — the map is
preserved here verbatim as the input the debate adjudicated, not as the
decided position.

---

**House-style note:** this packet follows the
`2026-07-14-skill-craft-pilot-evidence.md` convention (bolded-metadata header
+ Topic/Decision-memo link, Provenance, Confidence legend, then source-
anchored tables and notes). It is evidence (source-anchored material), NOT
canon; verdicts route to `researches/decisions/` only after adjudication, per
the `researches/sources/` contract.
