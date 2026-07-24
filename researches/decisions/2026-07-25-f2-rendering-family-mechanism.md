# Decision: F2 Rendering Family Mechanism

**Date:** 2026-07-25
**Status:** Accepted (record-of-decision). No code lands in this slice; this is
a record-of-decision that fixes the F2 rendering-family mechanism architecture
at decision granularity. The full sliced build plan is a separate build input;
this memo captures the citable decisions that gate it.
**Basis:** commit `9dbab50` (the F1 synthesis-family + S2-a topology memo, incl.
Decision 2's F1→F2 emit-boundary contract + the F2-rendering-surface authority
classifications) and commit `68a8fc4` (canon promotion: the F1/F2/F3 union-list
sub-section in the 2026-07-23 disposition memo + the Block-BUILD-READY
authority-line row in the 2026-07-22 memo).
**Supersedes:** none (narrows the HYBRID producer-vs-renderer split + the F1
emit contract into a concrete F2 mechanism; does not replace either).
**See also:**
[`./2026-07-25-f1-synthesis-family-and-s2a-topology.md`](./2026-07-25-f1-synthesis-family-and-s2a-topology.md)
(the F1 memo — `9dbab50`; the producer-axis family whose `ValidatedF1Emit` F2
consumes, and the F1→F2 emit-boundary contract this memo re-states at decision
level).
[`./2026-07-23-vh-solara-orchestration-field-report-disposition.md`](./2026-07-23-vh-solara-orchestration-field-report-disposition.md)
(the HYBRID closure-vocabulary section + the F1/F2/F3 union-list sub-section
promoted at `68a8fc4`; F2 is the renderer/consumer axis of that union).
[`./2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md`](./2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md)
(the authority line this memo's authority-line audit respects; the
Block-BUILD-READY row there is the F3 disposition, not F2).

## Framing

The F1 memo (`9dbab50`) settled the **producer** axis: an `F1SynthesisEnvelope`
that joins / generates / counter-evidences, emitting `ValidatedF1Emit` as a
candidate, informational artifact with no transition authority. The HYBRID
reframe (`68a8fc4`) names F2 as the **renderer/consumer** axis — property-distinct
from F1, fail-closed under the union rule, but never recompute the producer act.

This memo records the **F2 mechanism**: the persistence model, the intra-family
composition (named property slots + derived views), the per-control contracts
(R5/P-b/P-c), the derived-surface contracts (R1-derived streak, P-a-derived
table), the authority-line audit, and the gate-shaped build decisions that the
full sliced build plan inherits. It is composed at **decision granularity**,
mirroring the F1 memo's granularity — not the per-slice build plan. It serves
as the citable durable basis for (a) the F2 `build` slices (which depend on F1's
actually-implemented schema), and (b) the separately-queued F3 mechanism brief.

The fixed input this memo cannot reopen: **F2 is a renderer/consumer.** It
persists, projects, and renders what F1 produced. It never joins evidence,
merges properties, generates alternatives, generates counter-evidence, adds
conclusions, infers gaps, reinterprets bounded absence as global absence,
reconstructs a missing F1 entry from prose, treats unverified media as
evidence, or emits a partial envelope as complete. That fence is load-bearing;
the rest of the memo is its mechanical articulation.

## Decision 1 — Architecture: immutable canonical sidecar + deterministic Markdown projection

One immutable artifact pair per synthesis cycle:

```
docs/checkpoints/f2/<synthesis_cycle_id>.canonical.json   # lossless persisted ValidatedF1Emit
docs/checkpoints/f2/<synthesis_cycle_id>.md               # deterministic, operator-readable projection
```

- **JSON = "what F1 said"** — immutable, digest-bound, the canonical F1 evidence
  carried losslessly into durability.
- **MD = "how F2 displays it"** — regenerable, derived ONLY from the JSON, never
  an independent source of semantic content.

Both artifacts retain, as **F2 view metadata** (not canonical F1 evidence):
`synthesis_cycle_id`, the relevant `entry_id` values, source `semantic_digest`,
canonical representation/schema version, projection/renderer version, the
reciprocal locator back to the canonical pair, and the write timestamp.

The MD projection must identify itself with a standing notice (rendered into the
projection, not authored ad hoc): *"Derived, informational, and
non-authoritative. Canonical meaning remains in the digest-bound F1 emit."*

### Rejected alternatives (named, one-line reasons)

- **Single Markdown artifact (persistence + presentation coupled).** Couples
  durability to presentation and complicates digest canonicalization — a
  digest-valid projection could not be regenerated from a stable canonical base.
- **Shared generated `INDEX.md`.** Unnecessary mutable shared state; the streak
  derives by scanning immutable cycle artifacts (see Decision 2). A shared index
  reintroduces a second source of truth under the same cycle ID.
- **`.local/coordinator/reports/` then promotion.** `.local/` is transport, not
  committed durability (per the DEFER/follow-up curation contract). F2 emits a
  committed projection; it does not depend on promotion to reach durability.
- **Runtime claims/query service.** Prohibited by HYBRID — a persistent claims
  database with a runtime query surface is the exact mechanism the HYBRID
  reframe rejected. F2's projection is a deterministic committed doc, read
  synchronously at boundaries the harness already controls.

## Decision 2 — Intra-family composition: one shared pipeline with named property slots + derived views

### Named F2 entries (distinct properties)

- **R5** — operator-synthesis durable binding.
- **P-b** — evidence-grade media provenance attachment.
- **P-c** — decision-headline salience.

These are distinct named properties with distinct owner / cadence / failure
mechanism / data-role identities. They are NOT collapsed into one F2 entry.

### Derived F2 views (NOT re-declared as new canonical F2 properties)

- **R1-derived operator synthesis streak.**
- **P-a-derived decision-request table.**

Their meaning stays canonical in F1; F2 only formats. Re-declaring them as new
F2 properties would duplicate the producer act the MIXED resolution (F1 memo,
`9dbab50`) forbids — R1 and P-a are declared ONCE in F1 and DERIVED into F2.

### Reconciliation with the F1 envelope

F1 uses the envelope to contain distinct **synthesis-producing** acts
(`r1_cross_lane_join`, `r3_redesign_fork`, `pa_counter_evidence`). F2 uses one
**shared ingest/binding core** — all outputs share the same source emit + digest
(the pair from Decision 1) — BUT keeps separate **named render contracts**
because R5/P-b/P-c have different property identities and different failure
modes. One pipeline, one digest, three render contracts.

## The F1→F2 consumption contract (fixed input from the F1 memo — restated at decision level)

- **Input:** F2 accepts exactly `ValidatedF1Emit { canonical_envelope,
  semantic_digest, validation_disposition: complete }`. No second narrative
  input, no chat excerpt, no inferred history, no reconstructed entry, no
  model-generated summary, and no manually edited rendering field.
- **Deterministic ingest sequence (6 steps):**
  1. **Parse** without semantic normalization — only the committed F1 digest
     algorithm's normalization applies.
  2. **Structurally validate** the envelope: complete disposition, valid cycle
     ID, entry refs resolve, known enums, required fields present, and **no
     entry reconstructed from prose.**
  3. **Recompute and compare the semantic digest** (`recomputed == supplied`).
     No silent normalization, no digest update, no repair — a changed canonical
     field requires a new F1 emit + digest.
  4. **Construct the immutable pair** from the same validated in-memory
     representation (Decision 1).
  5. **Collision handling:** neither exists → write; both exist byte-identical →
     idempotent no-op; either exists different → refuse and report a new cycle
     required; only one exists → report an incomplete pair.
  6. **Structural audit** by `doctor`.

- **"F2 may"** (carried from the F1 memo): persist canonical bytes / a lossless
  representation; write the committed MD projection; render the R1 streak and
  the P-a decision table; add storage/timestamp/renderer/view metadata; attach
  captured-or-verified media metadata; verify the source digest matches.
- **"F2 must NOT"** (carried from the F1 memo — the load-bearing fence): join
  evidence; merge/split properties; generate alternatives; generate
  counter-evidence; add conclusions or rationales; infer missing gaps;
  reinterpret `not_found_in_checked_scope` as global absence; reconstruct a
  missing F1 entry from narrative prose; treat unverified media as evidence;
  emit a partial envelope as complete.

## Per-control contracts (decision level)

### R5 — operator-message durable binding

**Property-identity verdict (HYBRID): NEW UNION property — NOT a P2-B extension.**

P2-B's `findings_delta` binds **AGENT closeouts** (subject = an agent's
closeout; property = what an agent's work found/changed). R5 binds
**OPERATOR-authored synthesis** (subject = operator-authored synthesis; property
= durable, addressable binding when an operator deliberately authors
synthesis). Different subject, different cadence, different failure mechanism.
R5 is therefore **more than a provenance field appended to P2-B** — adding the
field to the same property would collapse a distinct union member.

**Binding mechanism:** an operator explicitly marks or promotes an authored
synthesis → that synthesis is admitted into an F1 synthesis cycle through the
settled F1 mechanism → F1 emits a complete `ValidatedF1Emit` retaining the
operator source locator and canonical bytes → F2 persists the emit pair and
renders an addressable R5 section. **F2 does NOT ingest raw chat independently
of F1.** "At operator-authoring" is the capture / promotion trigger; durable F2
emission occurs only AFTER F1 validation.

**Prohibited:**
- accept an arbitrary chat excerpt as a second F2 input;
- summarize the operator's message anew;
- infer which chat passages constitute synthesis;
- merge several operator messages;
- reconstruct missing canonical text from transcript prose;
- treat the memo-only `findings_delta` syntax as an already-shipped runtime seam
  (see the C4 gate below).

### P-b — evidence-grade media provenance slot

**Domain-free slot contract:**

```
media_attachment {
  attachment_id
  entry_id
  locator { kind: path|url, value }
  capability_class
  modality_hint
  evidence_grade: captured|verified
  provenance {
    source_locator
    capture_or_verification_method
    observed_or_verified_at
    producer_or_verifier_class
    content_digest if available
  }
  capability_status if perception attempted
  limitations
}
```

**Capability-class rule:** the harness defines the locator shape + the
capability class + the provenance fields + the closed structural value set +
rendering/validation. The harness must NOT define vendor / transport /
proprietary-service / model-name-as-contract. This follows the
`core/media-perception` precedent and adds the new provenance-carriage slot.

> **[VERBATIM — operator requirement: capability_status ≠ evidence-grade]**
> Existing media-perception values such as `available | unavailable | uncertain` describe perception capability status. They do not mean `true | false | authenticated`. They must not be reused as evidence-grade or content-truth values.

**Fabricated-chart refusal:** a media attachment is eligible for evidence-grade
display ONLY when (all of) the locator is structurally valid + `evidence_grade`
is captured/verified + the required provenance fields are present + the
attachment is associated with a canonical F1 entry + source digest/cycle binding
is retained. If absent: do NOT display as evidence-grade media; render a bounded
"attachment omitted: provenance not structurally established" notice **only if**
the canonical envelope requires the omission to remain visible; do NOT infer
evidence from the media; do NOT reject unrelated F1 entries or the entire F2
projection.

**Honesty ceiling:** the P-b validator may establish *"required provenance
metadata is structurally present and declares the source as captured or
verified."* It may NOT establish *"the chart's numbers are real, accurate,
unbiased, or authentically produced."* Plausible fabricated metadata remains
beyond this structural gate. This limitation must appear in the contract AND in
the `doctor` documentation. This is the F2 specialization of the F1 memo's
fabricated-chart hard rule.

### P-c — headline-layer salience

**Purpose:** prevent a clean headline from contradicting, burying, or
omitting an inconclusive, failed, or materially-qualified result.

**Fixed first-layer structure:** every decision-request MD projection must
BEGIN with an operator-visible section containing, **in order**:

1. Decision frame;
2. Current disposition/verdict;
3. Counter-evidence;
4. Weakest claim;
5. Unresolved gaps or limitations;
6. Canonical binding metadata.

Counter-evidence and weakest claim MUST be in the **first disclosure layer**,
not merely linked or appendixed.

**Binding discipline:** deterministic rendering + structural `doctor`
diagnostics. The headline section must exist; all required fields populated from
canonical entries; displayed disposition == canonical disposition; counter-
evidence/weakest-claim refs resolve to canonical entry IDs; no renderer-authored
conclusion may replace those values. A missing or malformed P-c section makes
the projection structurally invalid — it STILL does not authorize blocking
another system transition (see the safety valve below).

**No semantic summarization:** use exact values / deterministic bounded
excerpts / formatting rules defined by the canonical schema. Do NOT ask a model
to "summarize for the headline" — that would make F2 a synthesis producer and
cross the F1→F2 fence.

## Derived-surface contracts (decision level)

### R1-derived operator synthesis streak

Derived by scanning immutable `docs/checkpoints/f2/*.canonical.json`.

**Shows:** chronological cycles, source cycle/entry IDs, source locators,
hazard↔symptom links (as declared by F1 — F2 never infers a link), agreements,
contradictions, unresolved gaps, status badges mapping to canonical enums, and
the semantic digest + renderer version.

**Must NOT:** infer entries form a streak; create new hazard links; collapse
contradictions into agreement; derive global absence from bounded absence;
replace canonical ancestry; or author a second conclusion.

**Ordering** uses canonical chronology + a stable tie-breaker
(`synthesis_cycle_id`), NEVER filesystem mtime. No shared mutable index.

**Initial scope:** render all valid committed F2 cycles in the checked repo
scope. No arbitrary "last N" policy. If scale later requires a bound, it MUST be
explicit in renderer config AND printed AND deterministic AND described as a
view filter — never a silent truncation.

### P-a-derived decision-request table

**Required columns:** `Option | Costs | Evidence against | Weakest claim |
Reversal cost`.

Each value comes from P-a canonical probes or explicitly identified canonical F1
fields. Probe-result semantics are preserved EXACTLY:

- `found` / `not_found_in_checked_scope` / `unavailable` / `not_run`.
- `not_found_in_checked_scope` must NEVER render as "none exists."
- `unavailable` stays distinct from a negative result.
- `not_run` stays visibly unperformed.
- Blank cells must not silently collapse these states.

## Contradictions resolved

1. **C1 — P-a columns versus described canonical fields.** The required table
   includes `costs` and `reversal cost`, but the inspected F1 description did not
   explicitly locate both fields. F2 cannot invent them. **Resolved as a NAMED
   BUILD GATE** (see below) — the renderer is gated on mapping each column to an
   existing `ValidatedF1Emit` field with a defined absence representation.
2. **C2 — single emit vs cross-cycle streak.** The streak is derived by
   deterministically scanning prior immutable canonical F2 artifacts; no
   cross-cycle mutable state is introduced. Resolved by Decision 1's immutable
   pair + Decision 2's derivation model.
3. **C3 — R5 "when written" vs complete-emit-only F2 input.** Boundary
   sequencing: operator authoring triggers promotion, but F2 durability occurs
   ONLY after F1 produces a complete validated emit. "At operator-authoring" is
   a capture trigger, not an F2 ingest path. Resolved by the R5 binding
   mechanism.
4. **C4 — P2-B adopted versus apparently unshipped.** The disposition memo
   adopts `findings_delta`, while current authoritative closeout/state material
   reportedly does not implement it. **Resolved as a NAMED BUILD GATE** (see
   below) — R5's new-UNION verdict is validated against the authoritative
   shipped P2-B schema during Slice 0.

> **[VERBATIM — operator requirement: Slice-0 stop-conditions as NAMED BUILD GATES]**
> **C1 — P-a columns versus described canonical fields (NAMED BUILD GATE).** The required table includes costs and reversal cost, but the inspected F1 description did not explicitly locate both fields. F2 cannot invent them. **Before implementing the P-a renderer, build must map each column to an existing `ValidatedF1Emit` field and document its defined absence representation.** If no such canonical fields exist: stop the P-a rendering slice; report an F1/F2 contract incompatibility; do not add fields to F1 within this task; do not fill the columns with renderer-authored prose; do not emit a table that appears complete. This is a schema-evidence gap, not permission to reopen F1.
>
> **C4 — P2-B adopted versus apparently unshipped (NAMED BUILD GATE).** The disposition memo adopts `findings_delta`, while current authoritative closeout/state material reportedly does not implement it. **Build must not treat memo-only syntax as an existing runtime seam.** R5's new-UNION verdict is validated against the authoritative shipped P2-B schema during Slice 0; if the shipped schema is unexpectedly subject-neutral, extending it may be appropriate.

## Fixed inputs carried (settled, non-reopenable)

1. **F1→F2 emit contract** — `ValidatedF1Emit`, candidate/informational, no
   transition authority.
2. **"F2 may" list** — persist, project, render, attach verified media, verify
   digest.
3. **"F2 must NOT" list** — the load-bearing fence: no join / merge / generate /
   infer / reconstruct.
4. **Digest-binding rule** — every persisted/rendered F2 artifact retains
   `synthesis_cycle_id` + `entry_ids` + `semantic_digest` + projection version;
   a changed canonical field requires a new F1 emit, NOT an in-place F2
   correction.
5. **Fabricated-chart hard rule** — captured-or-verified, always; structural
   provenance-presence ceiling, NOT content-truth.
6. **Evidence-priority ordering** — join → generate → persist → render →
   attach-verified-media; F2 owns steps 4–5, downstream of and dependent on F1.
7. **Authority line** — all F2 mechanisms INFORM; no gate conversions. Any
   non-INFORM surface is a re-scope signal to F1/F3.
8. **HYBRID** — thin domain-free closure vocabulary; NOT a persistent claims
   database; the committed projection is a deterministic doc, NOT a runtime
   query service.

## Authority-line audit

Every F2 mechanism classified INFORM or INFORM/artifact-integrity. No F2
mechanism carries transition authority.

> **[VERBATIM — operator requirement: authority-line audit + safety valve]**

| Mechanism | Authority | Audit result |
|---|---|---|
| F2 input validation | INFORM / artifact integrity | May refuse production of an invalid F2 artifact; cannot block another transition |
| Semantic-digest comparison | INFORM / artifact integrity | Confirms source binding only; does not validate evidence truth |
| Canonical JSON persistence | INFORM | Lossless storage of F1 output; no independent conclusion |
| Markdown projection | INFORM | Deterministic display only |
| R5 binding | INFORM | Makes validated operator synthesis durable; does not decide or transition |
| P-b provenance slot | INFORM | Carries declared provenance; does not prove media truth |
| P-b attachment refusal | INFORM / artifact integrity | Omits structurally unverified attachment; does not reject unrelated synthesis or block workflow state |
| P-c salience | INFORM | Makes existing disposition and counter-evidence visible |
| R1 streak | INFORM | Projects canonical chronology; no inferred recurrence or joins |
| P-a table | INFORM | Formats canonical probes; no generated alternatives or evidence |
| F2 doctor check | INFORM / diagnostic | Detects structural inconsistency; does not adjudicate claims or apply transitions |

> **Safety valve:** No proposed F2 mechanism requires transition authority. **If implementation attempts to make F2 doctor output a prerequisite for build-ready, release, or coordinator state movement, that portion must be removed or moved to F3.**

## Open questions for build (decision-level, NOT invitations to reopen the mechanism)

1. The exact existing DTO/package owning `ValidatedF1Emit` + the semantic-digest
   algorithm.
2. Which canonical F1 fields supply P-a `costs` and `reversal_cost`, and what
   absence form is defined (the **C1** gate).
3. The exact canonical chronology field ordering F2 cycles.
4. The authoritative shipped P2-B schema (the **C4** gate — validates R5's
   new-UNION verdict).
5. Whether the repo already has an atomic paired-artifact write abstraction, or
   F2 must introduce one at an existing filesystem boundary.
6. Whether the canonical sidecar preserves received JSON bytes verbatim or
   serializes an equivalent lossless typed representation (must follow the
   committed F1 digest definition).
7. Which existing `doctor` registration file is the correct authoritative seam
   for the F2 check.
8. Initial scan scope: all valid F2 cycles (recommended default) vs an explicit
   configured view bound.

These are build-localization questions. None of them re-opens a mechanism
decision: the mechanism is fixed by Decisions 1–2 + the per-control contracts
above; the questions pin the build to the repo's actual seams.

## Next steps (gated on THIS memo landing — do NOT execute in this slice)

- **F3 mechanism brief** (design-gate family) — queued, launches after this memo
  commits. F3 owns DAY-0 (adversarial BUILD-READY refusal); it is the only
  operator-visibility control whose authority is BLOCKS.
- **F1 build precedes F2 build** — F2 Slice 0 depends on F1's actually-
  implemented schema. The C1/C4 gates resolve against the shipped F1 + P2-B
  schemas, not against narrative.
- **Optional cosmetic follow-up (non-blocking):** repoint
  `researches/sources/2026-07-25-seven-controls-property-map.md`'s stale
  `tmp/` citation for the operator-visibility study to its durable sibling
  `researches/sources/2026-07-24-opencode-history-visibility-study.md` if/when
  that study is promoted to a durable path. Not blocking; not in this slice.

## Evidence / Provenance

| Claim | Verifying artifact / command | Verified |
|---|---|---|
| F1 memo + S2-a topology (Decision 1 + Decision 2) is basis `9dbab50` | `git log --oneline -1 9dbab50` → "persist S2-a topology verdict + F1 mechanism design" | yes |
| F1/F2/F3 union-list + Block-BUILD-READY authority row promoted at `68a8fc4` | `git show --stat 68a8fc4` (disposition memo + claim-verifier memo, +35) | yes |
| F1→F2 emit contract wording carried faithfully | `researches/decisions/2026-07-25-f1-synthesis-family-and-s2a-topology.md` Decision 2 (F1 → F2 emit-boundary contract) | yes |
| HYBRID prohibits a runtime claims/query service | `researches/decisions/2026-07-23-vh-solara-orchestration-field-report-disposition.md` Addendum 2026-07-24 (the resolution) | yes |
| Authority line: coordinator informs, gates act | `researches/decisions/2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md` "Authority and transition routing" | yes |
| F2 is the renderer/consumer axis (data-role primary) | `9dbab50` Decision 1 + `68a8fc4` union-list sub-section | yes |
| Three VERBATIM operator-requirement blocks carried faithfully | this memo (capability_status ≠ evidence-grade; C1/C4 named build gates; authority audit table + safety valve) | yes (verbatim) |

House style: this memo follows the `2026-07-25` / `2026-07-22` / `2026-07-23`
convention (bolded-metadata frontmatter; Framing → Decision → Contract →
Authority → Contradictions → Evidence), matching the F1 memo's granularity
exactly — decision granularity, not the per-slice build plan.
