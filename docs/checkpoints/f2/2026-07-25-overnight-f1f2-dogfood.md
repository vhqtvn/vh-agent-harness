# F2 Projection — Synthesis Cycle `2026-07-25-overnight-f1f2-dogfood`

> **Derived, informational, and non-authoritative.** Canonical meaning remains in the digest-bound F1 emit at `docs/checkpoints/f2/2026-07-25-overnight-f1f2-dogfood.canonical.json`.

## F2 View Metadata

```f2-view-metadata
{
  "synthesis_cycle_id": "2026-07-25-overnight-f1f2-dogfood",
  "entry_ids": [
    "entry-pa",
    "entry-r1",
    "entry-r3"
  ],
  "source_semantic_digest": "1880898adb32ac69e689ea8bb2bcf669b3e07780470f87ceff2d58f1d9bdb600",
  "canonical_representation_version": "1",
  "schema_version": "1",
  "projection_version": "1",
  "renderer_version": "1",
  "reciprocal_locator": "docs/checkpoints/f2/2026-07-25-overnight-f1f2-dogfood.canonical.json",
  "write_timestamp": "2026-07-25T12:56:26Z"
}
```

<!-- f2-pc-headline:begin -->
## P-c Headline — Decision Salience Layer

> First disclosure layer. Counter-evidence and weakest claim surface here, before the detailed sections. Values are deterministic projections of canonical entries — no model summarization.

### Decision Frame
- **Trigger recognized:** `false`

### Current Disposition
- **R3 disposition:** `pending`

### Counter-evidence
- Probe `PA-P1` (target `R1C-c1-distillation-hazard`): `found` — evidence: `researches/decisions/2026-07-25-f1-synthesis-family-and-s2a-topology.md@15ddd54`, `tmp/agent-runs/f1-build/SLICE-0-STOP.md`
- Probe `PA-P2` (target `R1C-synthesis-failure-is-bottleneck`): `not_found_in_checked_scope`
- Probe `PA-P3` (target `R1C-three-family-topology-data-role-axis`): `found` — evidence: `researches/decisions/2026-07-25-f3-design-gate-mechanism.md@135aa16`
- Probe `PA-P4` (target `R1C-three-family-topology-data-role-axis`): `not_found_in_checked_scope`

### Weakest Claim
- Probe `PA-P1`: the C1 hazard<->symptom link is documented in two run artifacts; the distillation house-rule was added, but no second occurrence was checked for (bounded to this run)
- Probe `PA-P2`: F1's producer->emit crux is proven at the unit/integration level (TestEmitF1_Crux_ProducerToValidatorToEmitEndToEnd); the real-transition-refusal is deferred (operator-owned seam #6)
- Probe `PA-P3`: F3's design is committed + self-adjudicated ACCEPT; F3 is NOT implemented (JS-side, separate track, not in this run)
- Probe `PA-P4`: F2 is live + unit/integration-tested on fixtures; this dogfood is its first real-content cycle (pending this run's result)

### Unresolved Gaps
- (none declared in canonical R1 conclusions)

### Canonical Binding Metadata
- **Cycle:** `2026-07-25-overnight-f1f2-dogfood`
- **Entry IDs:** `entry-pa`, `entry-r1`, `entry-r3`
- **Source digest:** `1880898adb32ac69e689ea8bb2bcf669b3e07780470f87ceff2d58f1d9bdb600`

<!-- f2-pc-headline:end -->

<!-- f2-pa-table:begin -->
## P-a Decision-Request Table

> Deterministic per-option decision matrix from canonical R3 option records + P-a probes. Probe-result semantics preserved EXACTLY. No model summarization.

- (no R3 redesign fork with options in canonical emit — decision-request table not applicable)

<!-- f2-pa-table:end -->

<!-- f2-r5-binding:begin -->
## R5 — Operator-Synthesis Durable Binding

> Durable binding of operator-authored synthesis to this cycle + digest. F2 consumes only what F1 emitted — no raw chat, no model summarization.

- (no operator-source synthesis bound to this cycle)

<!-- f2-r5-binding:end -->

<!-- f2-pb-media:begin -->
## P-b — Evidence-Grade Media Provenance

> Media attachments eligible for evidence-grade display. Provenance is structurally present — **content truth is NOT verified** (plausible fabricated metadata passes this gate). capability_status ≠ evidence-grade.

- (no evidence-grade media attachments bound to this cycle)

<!-- f2-pb-media:end -->

## Canonical Envelope (projected)

- **Schema version:** `1`
- **Synthesis cycle ID:** `2026-07-25-overnight-f1f2-dogfood`
- **Applicability:** `required`
- **Semantic digest:** `1880898adb32ac69e689ea8bb2bcf669b3e07780470f87ceff2d58f1d9bdb600`
- **Validation disposition:** `complete`

### Entries

#### Family `r1_cross_lane_join` — Entry `entry-r1`

- **Triggered:** `triggered`

##### R1 Conclusions

###### Conclusion `R1C-c1-distillation-hazard` — Property `c1-distillation-hazard`

- **Join disposition:** `union`
- **Lanes:**
  - `lane-f1-build` (act: `slice0-stop`, position: `field-level-schema-drop-during-distillation`)
- **Sources:**
  - `researches/decisions/2026-07-25-f1-synthesis-family-and-s2a-topology.md@15ddd54`
  - `tmp/agent-runs/f1-build/SLICE-0-STOP.md`
- **Hazard links:**
  - Hazard `field-level-schema-drop-during-distillation` → symptoms `f1-track-stopped`, `f2-slice0-c1-gate-fail` → sources `researches/decisions/2026-07-25-f1-synthesis-family-and-s2a-topology.md@15ddd54`, `tmp/agent-runs/f1-build/SLICE-0-STOP.md` → consuming P-a probes `PA-P1`

###### Conclusion `R1C-synthesis-failure-is-bottleneck` — Property `synthesis-failure-is-bottleneck`

- **Join disposition:** `merge`
- **Lanes:**
  - `lane-evidence-study` (act: `history-visibility-study`, position: `synthesis-is-the-demonstrated-bottleneck`)
  - `lane-property-map` (act: `seven-controls-property-map`, position: `synthesis-is-the-demonstrated-bottleneck`)
- **Sources:**
  - `researches/sources/2026-07-24-opencode-history-visibility-study.md@8600fc8`
  - `researches/sources/2026-07-25-seven-controls-property-map.md@9dbab50`
- **Hazard links:**
  - Hazard `named-but-unresolved-synthesis-hazard` → symptoms `proceeded-as-if-moot` → sources `researches/sources/2026-07-24-opencode-history-visibility-study.md@8600fc8`, `researches/sources/2026-07-25-seven-controls-property-map.md@9dbab50` → consuming P-a probes `PA-P2`

###### Conclusion `R1C-three-family-topology-data-role-axis` — Property `three-family-topology-data-role-axis`

- **Join disposition:** `merge`
- **Lanes:**
  - `lane-canon-promotion` (act: `family-boundaries-canon`, position: `data-role-is-the-family-defining-axis`)
  - `lane-s2a-decision` (act: `s2a-topology-verdict`, position: `data-role-is-the-family-defining-axis`)
- **Sources:**
  - `researches/decisions/2026-07-23-vh-solara-orchestration-field-report-disposition.md@68a8fc4`
  - `researches/decisions/2026-07-25-f1-synthesis-family-and-s2a-topology.md@9dbab50`


#### Family `r3_redesign_fork` — Entry `entry-r3`

- **Triggered:** `triggered`

- **Trigger recognized:** `false`

- **Disposition:** `pending`

#### Family `pa_counter_evidence` — Entry `entry-pa`

- **Triggered:** `triggered`

##### P-a Probes

###### Probe `PA-P1` — Target `R1C-c1-distillation-hazard`

- **Result:** `found`
- **Falsification question:** Is the C1 field-level-schema-drop -> F2-Slice-0-gate-fail link actually evidenced in run artifacts?
- **Method:** read the F1 memo amendment (C1 incident note) + the F1-build Slice-0 stop note
- **Evidence refs:** `researches/decisions/2026-07-25-f1-synthesis-family-and-s2a-topology.md@15ddd54`, `tmp/agent-runs/f1-build/SLICE-0-STOP.md`
- **Weakest claim:** the C1 hazard<->symptom link is documented in two run artifacts; the distillation house-rule was added, but no second occurrence was checked for (bounded to this run)
- **Confidence:** high

###### Probe `PA-P2` — Target `R1C-synthesis-failure-is-bottleneck`

- **Result:** `not_found_in_checked_scope`
- **Falsification question:** Is F1's producer->emit->consume crux proven by a real repo-lifecycle transition refusal, or only the in-process producer->emit path?
- **Method:** read the F1 crux test + the F1 build operator-owned-seam notes
- **Checked scope:** `internal/cli/f1_emit_test.go`, `internal/cli/f1_f2_consumer.go`
- **Weakest claim:** F1's producer->emit crux is proven at the unit/integration level (TestEmitF1_Crux_ProducerToValidatorToEmitEndToEnd); the real-transition-refusal is deferred (operator-owned seam #6)
- **Confidence:** high-on-substring-medium-on-full-crux

###### Probe `PA-P3` — Target `R1C-three-family-topology-data-role-axis`

- **Result:** `found`
- **Falsification question:** Is F3's design settled (committed, self-adjudicated ACCEPT)?
- **Method:** read the F3 design-gate mechanism memo
- **Evidence refs:** `researches/decisions/2026-07-25-f3-design-gate-mechanism.md@135aa16`
- **Weakest claim:** F3's design is committed + self-adjudicated ACCEPT; F3 is NOT implemented (JS-side, separate track, not in this run)
- **Confidence:** high

###### Probe `PA-P4` — Target `R1C-three-family-topology-data-role-axis`

- **Result:** `not_found_in_checked_scope`
- **Falsification question:** Has F2 rendering been proven on real (non-fixture) F1 content before this dogfood?
- **Method:** grepped the F2 test files for fixture-only inputs
- **Checked scope:** `internal/cli/f2_ingest_test.go`, `internal/cli/f2_persist_test.go`, `internal/cli/f2_projection_test.go`
- **Weakest claim:** F2 is live + unit/integration-tested on fixtures; this dogfood is its first real-content cycle (pending this run's result)
- **Confidence:** medium-high


