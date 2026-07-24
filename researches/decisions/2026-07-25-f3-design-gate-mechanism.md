# Decision: F3 Design-Gate Mechanism (BUILD-READY refusal on named ownership hazard)

**Date:** 2026-07-25
**Status:** Accepted (record-of-decision). No code lands in this slice; this is a
record-of-decision that fixes the F3 safety-layer mechanism architecture — the
sole BLOCKS family in the F1/F2/F3 operator-visibility set — at decision
granularity. The full sliced build plan is a separate build input; this memo
captures the citable decisions that gate it.
**Basis:** commit `9dbab50` (the F1 synthesis-family + S2-a topology memo,
Decision 1's placement of DAY-0 in F3 + Decision 2's authority-line audit naming
F3 as the safety-layer gate), commit `68a8fc4` (canon promotion: the F1/F2/F3
union-list sub-section in the 2026-07-23 disposition memo + the
Block-BUILD-READY authority-line row in the 2026-07-22 memo), and commit
`8600fc8` (promotion of the Class-A operator-visibility evidence study to durable
sources — the citation root for the demonstrated failure this gate refuses).
**Supersedes:** none (narrows DAY-0 — placed in F3 by `9dbab50` Decision 1 — into
a concrete F3 mechanism; does not reopen the topology).
**See also:**
[`./2026-07-25-f1-synthesis-family-and-s2a-topology.md`](./2026-07-25-f1-synthesis-family-and-s2a-topology.md)
(the F1 memo — `9dbab50`; Decision 1 places DAY-0 in F3 as the sole BLOCKS family,
Decision 2's authority-line audit names the F3 gate as the safety-layer refusal).
[`./2026-07-25-f2-rendering-family-mechanism.md`](./2026-07-25-f2-rendering-family-mechanism.md)
(the F2 memo — the renderer/consumer family; F1/F2 are both INFORMS; F3 is the
only BLOCKS family in the three-family set).
[`./2026-07-23-vh-solara-orchestration-field-report-disposition.md`](./2026-07-23-vh-solara-orchestration-field-report-disposition.md)
(the HYBRID closure vocabulary + the F1/F2/F3 union-list sub-section promoted at
`68a8fc4`; F3 is the design-gate family in that union).
[`./2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md`](./2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md)
(the authority-line transition table carrying the Block-BUILD-READY row promoted
at `68a8fc4` — F3 is the disposition named there).
[`../sources/2026-07-24-opencode-history-visibility-study.md`](../sources/2026-07-24-opencode-history-visibility-study.md)
(the Class-A evidence study promoted at `8600fc8`; the demonstrated failure E1
and `ses_07bcf07cc` timing this gate refuses are recorded there).
[`../sources/2026-07-25-seven-controls-property-map.md`](../sources/2026-07-25-seven-controls-property-map.md)
(the seven-controls property map — DAY-0 = row 7, NEW SLOT, at BUILD-READY
crossing, BLOCKS authority).

## Framing

`9dbab50` Decision 1 placed DAY-0 in **F3** — the only one of the seven
operator-visibility controls whose authority is **BLOCKS** and whose cadence is
**pre-code** (the BUILD-READY / design-gate crossing, before code is written).
The HYBRID union-list (`68a8fc4`) and the Block-BUILD-READY authority row
(promoted at `68a8fc4` in the 2026-07-22 memo) recorded that placement in canon.
This memo records the **F3 mechanism**: the "resolved" predicate, the
authoritative call sites + dispatch backstops, the named-hazard input schema, the
adversarial counter-case discipline, the safety-layer residence, the honesty
ceiling, and the authority-line audit. It is composed at **decision granularity**,
mirroring the F1 and F2 memos — not the per-slice build plan. It serves as the
citable durable basis for the F3 `build` slices (JS-side, in
`templates/core/.opencode/scripts/`), which run parallel to the F1/F2 Go-side
build (different layer, no file conflict).

The demonstrated failure this gate refuses is recorded as **E1** in the
operator-visibility evidence study (`8600fc8`, Class-A vh-solara, `ses_07bcf07cc`,
2026-07-21 17:20:29): the build-readiness-gate prompt *named* the ownership seam
as **"the core blocker"** before code was written, yet the lane answered
**BUILD-READY / HIGH** at 17:34:15 and proceeded. The gap was hazard-**RESOLUTION
enforcement**, not hazard-naming — the hazard was visible, the resolution was not
adjudicated. F3 enforces RESOLUTION. Naming a hazard is an author act (INFORMS);
deriving whether it is resolved for BUILD-READY is a gate act (BLOCKS).

The fixed input this memo cannot reopen: **F3 is the sole BLOCKS family in the
F1/F2/F3 set.** Any design where the coordinator, a prompt, or a reviewer directly
decides BUILD-READY may proceed is outside the authority line and must be
re-scoped. The rest of the memo is the mechanical articulation of how the gate
derives its refusal.

## Decision 1 — The "resolved" predicate: hybrid (F3-O4)

**Verdict:** a named ownership hazard counts as **resolved for BUILD-READY** only
when **all** of:

1. an evidence-bearing resolution bound to the **current** design (digest-bound);
2. a structured **minimum counter-case** (the falsifying probe the author cannot
   skip);
3. a **distinct adversarial record** bound to the **same** design digest (not the
   author's own review re-used);
4. a closed reviewer verdict of `resolution_supported`; and
5. **no blocking limitation** remaining.

Merged into **one deterministic safety-layer predicate** (`F3_PASS`, below). No
single clause is sufficient on its own; the conjunction is load-bearing.

### Falsifying test (the crux argument)

The candidate senses of "resolved" were falsified against the demonstrated
`ses_07bcf07cc` failure (named hazard, declared solved, BUILD-READY/HIGH):

| Candidate sense of "resolved" | Would it have blocked? | Verdict |
|---|---|---|
| (a) authority / resolved mark (author declares solved) | No — the lane did exactly this (declared solved + BUILD-READY/HIGH at 17:34) | insufficient |
| (b) evidence-bearing resolution alone | unreliable — author selects easy falsifier / supportive evidence | necessary, insufficient |
| (c) adversarial review alone | No — the case already had a dedicated research lane | insufficient |
| (d) evidence + counter-case + distinct adversarial record + safety predicate | Yes (process counterfactual) | **adopted (O4)** |

**Honesty caveat on result (d):** it is a **process counterfactual** — the
conjunction would have blocked the demonstrated failure by construction. It is
**not proof that an elaborate-but-wrong package could never pass.** A structurally
complete package can still contain weak reasoning or a coordinated-but-wrong
adversarial record; the honesty ceiling (below) states explicitly what F3 can and
cannot verify. This mirrors the F1 memo's structural-validation-≠-truth caveat and
the behavioral-closure honesty precedent.

### Rejected alternatives (named, one-line reasons)

- **(a) authority/resolved mark as the predicate.** Reproduces the demonstrated
  failure: an author flag controls a transition. `status: resolved` is a derived
  F3 verdict, never an author-controlled input.
- **(b) evidence-bearing resolution alone.** The author selects the evidence;
  supportive evidence is the cheapest to produce. Necessary but insufficient.
- **(c) adversarial review alone.** The `ses_07bcf07cc` case already had a
  dedicated research lane that returned BUILD-READY/HIGH — adversarial framing
  without distinct identity, structural binding, and an explicit verdict adds no
  new property over what already failed.

## Decision 2 — BUILD-READY crossing: one shared gate, two call sites + dispatch backstops

F3 is **one shared pure predicate** evaluated at each authoritative BUILD-READY
mutation. There are exactly **two authoritative mutation points**:

- **Task-card route:** `templates/core/.opencode/scripts/state-lib.js::readyCoordinationTask`
  — the `draft → ready` transition driven by `/task-ready`.
- **Approved-plan route:** the `approveDraft` helper — the `draft → approved`
  transition driven by `/approve-plan`.

These are the **primary gates.** The F3 predicate runs inside each authoritative
transition **immediately before the state mutation** — fail-closed refusal leaves
the lifecycle state unchanged.

**Dispatch backstops (freshness re-check, NOT primary gates):**
`activateCoordinationTask` (`ready → working`) and plan execution dispatch re-check
that the F3 package is still current against the live design at dispatch time. A
package that passed the primary gate may be invalidated by a design-digest change
between crossing and dispatch; the backstop refuses dispatch in that case and
returns the work to the primary gate. The backstops do **not** re-adjudicate
resolution — they only re-derive freshness.

**`doctor` is a retrospective diagnostic, not an F3 implementation.** An
after-the-fact audit cannot undo a false BUILD-READY: by the time `doctor` runs,
the transition has already crossed. F3 must execute at the mutation; `doctor` may
surface drift for operator attention but carries no transition authority over a
crossing that already happened. (This is the same "doctor INFORMS, gates act"
split the 2026-07-22 memo fixes for every transition.)

## Named-hazard input schema

An `f3_design_readiness` envelope per applicable readiness crossing. The envelope
contains an explicit `ownership_hazards[]` inventory. Each hazard carries three
records:

### Hazard declaration

| Field | Notes |
|---|---|
| `hazard_id` | stable per hazard |
| `hazard_class` | closed: `ownership` (extensible by future families, never by author fiat) |
| `hazard_statement` | what the hazard is |
| `affected_boundary` | the boundary the hazard threatens |
| `competing_authorities[]` | the authorities in conflict |
| `failure_mode` | how it fails if unresolved |
| `source_records[]` | each with provenance (where the hazard was identified) |

### Resolution record

| Field | Notes |
|---|---|
| `authoritative_owner` | exactly one — the authority the resolution binds to |
| `secondary_authority_disposition` | closed: `removed` \| `prohibited` \| `delegated_to_authoritative_owner` |
| `mechanism_mapping` | how the resolution is realized in the design |
| `evidence_records[]` | each provenance-valid |
| `design_digest` | binds the resolution to the **current** design |
| `declared_by` / `declared_at` | provenance of the resolution |
| `blocking_limitations[]` | any limitation that blocks the verdict |

**A free-form author `status: resolved` MUST NOT control the transition.**
Resolution is a **derived F3 verdict**, not an author flag. This is Decision 1
result (a) rejected as a control input — the field may be carried for human
readability, but the predicate derives its verdict from the structural clauses,
never from this field.

### Adversarial record

| Field | Notes |
|---|---|
| `review_id` | stable per review |
| `hazard_id` | binds to the hazard under review |
| `design_digest` | binds to the **same** current design digest as the resolution |
| `reviewer_identity` | structurally distinct from the resolution producer |
| `reviewer_provenance` | how the reviewer was selected / what lane produced it |
| `counter_cases[]` | ≥1, each schema-valid (shape below) |
| `evidence_checked[]` | what the reviewer examined |
| `verdict` | closed: `resolution_supported` \| `refuted` \| `inconclusive` |
| `weakest_supported_claim` | the claim the reviewer judges weakest-yet-supported |
| `limitations[]` | reviewer-recorded limitations |

**Only `resolution_supported` can contribute to a pass.** `refuted` and
`inconclusive` both fail closed.

### Inventory discipline

- **Empty-but-explicit inventory passes.** `ownership_hazards: []` with the
  envelope present means "no named hazard to adjudicate at this crossing." This is
  a valid pass — the design author has surveyed and found nothing to name.
- **Omission of the inventory fails closed** at an applicable crossing. No
  envelope, or an envelope without the `ownership_hazards[]` field, is a
  structural failure distinct from an empty inventory.
- **Empty inventory does NOT prove no unnamed hazard exists.** It records that the
  author named none; it does not establish completeness of hazard discovery. The
  honesty ceiling carries this explicitly.

## Who declares what (the authority split, made concrete)

| Participant | Role | Authority |
|---|---|---|
| Design author | names hazards; proposes resolution; produces the minimum counter-case | **INFORMS** |
| Adversarial reviewer | challenges; strengthens/adds counter-cases; returns a verdict bound to the same digest | **INFORMS** |
| Coordinator | surfaces; routes; does not adjudicate resolution | **INFORMS** |
| F1 synthesis / F2 rendering | producer + renderer of design context F3 consumes | **INFORMS** (cannot satisfy or override F3) |
| **F3 lifecycle validator** | derives the resolution verdict; applies or refuses the transition | **BLOCKS** |

No participant other than the F3 lifecycle validator carries transition authority
over a BUILD-READY crossing. This is the load-bearing split: every producer of the
package INFORMS; only the gate at the mutation BLOCKS.

## Adversarial counter-case generation: hybrid

The adversarial record is produced in two distinct acts:

1. **Design author** provides a **deterministic minimum counter-case** per named
   hazard (the falsifying probe the author cannot skip — forces the author to
   state how the resolution fails).
2. A **distinct adversarial reviewer** validates / strengthens / adds **≥1**
   counter-case and returns a verdict bound to the **same** design digest.
3. The **safety-layer validator** checks the combined package **structurally**
   (does not re-reason over the counter-case content).

**Minimum counter-case shape:**

```
counter_case {
  counter_case_id
  preconditions
  competing_or_missing_event
  expected_authoritative_owner
  expected_state_or_outcome
  forbidden_state_or_outcome
  resolution_mapping
  evidence_refs[]
}
```

**Cadence:** generated + reviewed **before every BUILD-READY attempt** when
hazards exist. **Invalidated on design-digest change** — a new digest requires a
new minimum counter-case + a new adversarial verdict. **Freshness rechecked at
dispatch** by the backstops (Decision 2).

## The F3_PASS predicate

```
F3_PASS := envelope present + schema-valid
  AND design_digest matches current design
  AND every named ownership hazard satisfies F3_HAZARD_RESOLVED

F3_HAZARD_RESOLVED := declaration complete
  AND hazard sources have valid provenance
  AND resolution bound to current design digest
  AND exactly one authoritative owner declared
  AND every competing secondary authority has explicit permitted disposition
  AND mechanism mapping present + complete
  AND resolution evidence present + provenance-valid
  AND deterministic minimum counter-case complete
  AND adversarial record present + bound to same current design digest
  AND reviewer identity structurally distinct from resolution producer
  AND adversarial verdict == resolution_supported
  AND no counter-case refuted
  AND no blocking limitation remains
```

**Fail-closed on:** missing, malformed, unknown value, stale (different digest),
internally contradictory, unprovenanced, identity-colliding (reviewer ==
producer), `refuted`, or `inconclusive`. Every failure produces a structured
BLOCK with a reason code; no failure is silent.

## Safety-layer residence

F3 lives as a **pure validator in the authoritative template script layer**: a
domain-neutral helper in
`templates/core/.opencode/scripts/state-lib.js`, or (preferably, for separation
of concerns) a sibling `templates/core/.opencode/scripts/f3-design-readiness.js`
called by `state-lib.js`. The choice between inline and sibling is a build
localization question (Open question 4), not a mechanism decision.

**Transition flow at each authoritative crossing:**

1. load the current design + the F3 package;
2. derive the current design digest;
3. run the pure F3 validation;
4. **fail:** refuse without mutation (lifecycle state unchanged, structured
   reason returned); **pass:** apply the existing transition.

The gate executes **before any readiness mutation.** No mutation occurs until
`F3_PASS` holds.

## Reconciliation table

F3 borrows discipline from four precedents; it must NOT copy their cadence,
semantics, or acceptance policy.

| Precedent | F3 borrows | F3 must NOT copy |
|---|---|---|
| Motivation-check P1-B | explicit design questions + recorded reasoning | advisory/aspirational acceptance (P1-B doesn't block; F3 does) |
| Behavioral-closure #14 | pure validation, closed enums, unknown-value rejection, deterministic diagnostics, honesty caveat | closeout cadence + absent-token-pass (F3 is pre-code; the envelope must be present at the applicable crossing — absence is failure, not silence) |
| §4.3 release re-adjudication | re-read current unresolved state at the controlled transition; do NOT trust a prior "review completed" marker | release cadence + release-property semantics (F3 fires at BUILD-READY, not at release) |
| Pattern 5 (named-but-dropped principle) | treat a named-but-dropped principle as a structural readiness failure; require re-derivation before promotion | post-commit discovery + coordinator acceptance authority (F3 fires pre-mutation; the coordinator cannot accept a hazard as resolved) |

## Honesty ceiling

**F3 CAN verify:**

- the envelope exists;
- hazards are in the known schema;
- each hazard has a resolution record;
- each resolution names exactly one authority + disposes every secondary;
- evidence/provenance fields exist + are internally consistent;
- locators/digests match where checkable;
- counter-case fields exist + are schema-valid;
- a distinct adversarial record exists + is bound to the current design;
- the verdict is a known value + is `resolution_supported`;
- no refutation / inconclusive / stale-digest / blocking-limitation remains;
- the check executed pre-mutation.

**F3 CANNOT verify:**

- the design is good or correct;
- the chosen authority model works in production;
- a cited source is truthful merely because it has provenance;
- the model reviewer reasoned competently;
- identity-distinction equals genuine independence (string-inequality is only
  nominal separation — see Open question 5);
- the counter-case is exhaustive;
- no unnamed hazard exists (an empty inventory records "none named," not "none
  exists");
- a structurally complete resolution is substantively true.

**Diagnostics say:** *"the required F3 resolution process is structurally
satisfied."* **NEVER:** *"the ownership hazard is proven solved."* This wording
is load-bearing — it preserves the structural-validation-≠-truth caveat the F1
memo and behavioral-closure #14 both carry, specialized to the design-gate
cadence.

## Authority-line audit

Every F3 participant classified. F3 is the **sole family with BLOCKS authority**
in the F1/F2/F3 set.

| Participant | Authority |
|---|---|
| Design author | INFORMS |
| Adversarial reviewer | INFORMS |
| Coordinator | INFORMS |
| F1 synthesis / F2 rendering | INFORMS (cannot satisfy or override F3) |
| **Shared F3 validator at lifecycle mutation** | **BLOCKS** |
| `doctor` audit (optional) | safety diagnostic, NOT sole transition authority |
| Commit / release gates | separate later safety properties (post-code; F3 fires pre-code) |

> **Safety valve:** any design where the coordinator, a prompt, or a reviewer
> directly decides BUILD-READY may proceed is **outside the authority line** and
> must be re-scoped. F3 owns the BUILD-READY refusal; no other participant may
> grant it.

## Intra-family composition: one gate + named-hazard substructure

F3 does **not** need separate declaration / evidence / review / freshness gates
with independent authority. Those are **producer records and clauses of one
property:** *"all named ownership hazards are structurally resolved for this
exact design."* One shared pure predicate (`F3_PASS`) at each authoritative
readiness mutation. The hazard declaration, resolution record, and adversarial
record are sub-structures of the envelope, not independent gates. This mirrors
the F1 memo's intra-family composition (one envelope, named entries) and the F2
memo's intra-family composition (one shared pipeline, named render contracts),
specialized to the BLOCKS cadence.

## Fixed inputs carried (settled, non-reopenable)

- **S2-a topology** (`9dbab50` Decision 1): F3 = design-gate family, sole BLOCKS.
- **Authority line** (`9dbab50` Decision 2 + `68a8fc4`): BLOCKS = safety-layer
  gate at lifecycle mutation; the coordinator INFORMS, never blocks.
- **Property-map row 7** (`2026-07-25-seven-controls-property-map.md`): DAY-0 =
  NEW SLOT, the largest gap — no shipped design-phase adversarial gate exists.
- **Day-0 verdict** (`8600fc8` evidence study): no visibility/display mechanism
  fixes it; the gap is **resolution enforcement**, not hazard visibility.
- **F1 authority audit** (`9dbab50`): Block-BUILD-READY = F3.
- **Block-BUILD-READY authority row** (`68a8fc4` → 2026-07-22 memo L128): F3
  design-gate, safety-layer gate, coordinator INFORMS only.
- **Evidence-priority ordering**: F3 fires **earliest** — pre-code, before F1's
  producer acts feed a BUILD-READY crossing.
- **Fabricated-chart hard rule**: a model-fabricated artifact with synthesized
  content and no provenance is more dangerous than prose; F3's evidence clauses
  inherit the captured-or-verified ceiling.
- **HYBRID** (`68a8fc4` union-list): distinct property → UNION; F3 is property-
  distinct from F1/F2 on authority (BLOCKS vs INFORMS) and cadence (pre-code vs
  producer/renderer).
- **Behavioral-closure honesty precedent** (`7f95f29` + `4d8d725`, doctor #14):
  structural/process-integrity, NOT design-truth — F3 borrows the discipline but
  fires at a different cadence with a different absent-token policy.

## Sliced implementation plan (decision level — names slices, seams, verification)

- **Slice 0 — localize authoritative seams.** `state-lib.js::readyCoordinationTask`,
  the `approveDraft` helper, the storage shape, the digest input (which design
  fields are hashed — Open question 3), and the existing test seams. Output: a
  localized map of insertion points + the digest definition.
- **Slice 1 — pure F3 schema + validator.** `validateF3DesignReadiness` →
  deterministic `PASS` / structured `BLOCK` with reason codes. Table-driven tests
  including the **vh-solara-style named-but-unresolved fixture BLOCKED** (the
  `ses_07bcf07cc` shape reproduced as a generic test fixture — domain-free, not a
  shipped literal).
- **Slice 2 — task-card BUILD-READY integration.** F3 precondition before
  `draft → ready` in `readyCoordinationTask`.
- **Slice 3 — approved-plan BUILD-READY integration.** Same precondition before
  `approveDraft`.
- **Slice 4 — dispatch freshness backstops.** `activateCoordinationTask` + plan
  execution dispatch; re-check currentness (digest unchanged since primary gate).
- **Slice 5 — authoring + reviewer surfaces.** Templates for the hazard
  inventory + the adversarial record (authoring aids, NOT enforcement — the
  predicate is the enforcement).
- **Slice 6 (optional) — `doctor` retrospective audit.** Surfaces drift for
  operator attention; carries no transition authority over a crossing that
  already happened.
- **Slice 7 — dogfood regeneration + docs + full verification.** `make update`,
  `go test ./...`, `gofmt`, `go vet`; confirm domain-free (the vh-solara case
  appears only as a generic test fixture, not a shipped literal).

## Contradictions encountered

None invalidating the frame. Two reconciled + one correction:

1. **"No shipped seam" (property-map row 7) vs existing readiness functions
   (`readyCoordinationTask`, `approveDraft`).** Reconciled: the property map
   means no shipped F3 **control** — no gate currently derives a resolution
   verdict or refuses a BUILD-READY crossing. The existing transitions are
   **insertion points** for the missing property, not the property itself. The
   gap is the absent predicate, not an absent call site.
2. **Terminology match on `ses_07bcf07cc`.** Reconciled: the session cited in
   the F3 brief IS the vh-solara build-readiness-gate researcher lane (07-21
   17:20 prompt → 17:34:15 BUILD-READY/HIGH), recorded as E1 in the evidence
   study promoted at `8600fc8`. The falsifying test is calibrated to this case.
3. **Correction — behavioral-closure #14 is a separate structural-honesty
   precedent at closeout, NOT an F1/F2 capability check.** F3 borrows its
   discipline (pure validation, closed enums, unknown-value rejection,
   deterministic diagnostics, honesty caveat) but is **distinct**: it fires
   pre-code at BUILD-READY, not post-code at closeout, and its absent-token
   policy is fail-closed (envelope must be present), not pass-on-absent.

## Open questions for build (decision-level, NOT invitations to reopen the mechanism)

1. **Canonical persistence location.** Task cards + plans are the persistence
   surface — no parallel store is introduced (consistent with HYBRID's
   no-claims-DB rule).
2. **The exact `approveDraft` helper.** Named at decision level; build localizes
   the actual function in Slice 0.
3. **Digest scope.** Which design fields are hashed to produce `design_digest`.
   Must be deterministic, reproducible from current ground truth, and sensitive
   to every field the resolution + adversarial record depend on.
4. **Applicability / migration.** Legacy crossings without an envelope: fail-
   closed on **omission** (no envelope → BLOCK); pass on **explicit-empty**
   (`ownership_hazards: []` with envelope present). No silent backfill.
5. **Reviewer identity semantics.** String-inequality between `reviewer_identity`
   and `declared_by` is only **nominal** separation — the strongest deterministic
   representation available at this layer. Genuine independence cannot be
   verified structurally (honesty ceiling); build picks the strongest available
   deterministic check and records its limitation.
6. **Evidence locator verification.** Which source-record forms can be
   existence-checked or digest-checked at validation time; which are
   provenance-only (locator present + internally consistent, but not
   fetchable/verifiable from inside the validator).

These are build-localization questions. None re-opens a mechanism decision: the
mechanism is fixed by Decisions 1–2 + the schema + the predicate above; the
questions pin the build to the repo's actual seams.

## Next steps (gated on THIS memo landing — do NOT execute in this slice)

- **F3 build (Slices 0–7)** — JS-side, in `templates/core/.opencode/scripts/`.
  Parallelizable with the F1/F2 Go-side build (different layer, no file
  conflict). Slice 1's table-driven tests reproduce the `ses_07bcf07cc`
  named-but-unresolved shape as a generic fixture (domain-free, not a shipped
  literal).
- **F1 build precedes F2 build** — F2 Slice 0 depends on F1's actually-
  implemented schema. F3 is independent of that ordering (consumes the design +
  the readiness crossings, not the F1 envelope).
- **No canon edits in this slice.** The union-list sub-section (`68a8fc4`) and
  the Block-BUILD-READY authority row (`68a8fc4`) already name F3; this memo
  records the mechanism they anticipated.

## Evidence / Provenance

| Claim | Verifying artifact / command | Verified |
|---|---|---|
| F1 memo places DAY-0 in F3 (sole BLOCKS) + authority audit names F3 gate | `git log --oneline -1 9dbab50` → "persist S2-a topology verdict + F1 mechanism design"; F1 memo Decision 1 + Decision 2 authority-line audit | yes |
| F1/F2/F3 union-list + Block-BUILD-READY authority row promoted at `68a8fc4` | `git show --stat 68a8fc4` (disposition memo + claim-verifier memo, +35) | yes |
| Evidence study (Class-A vh-solara) promoted at `8600fc8` | `git show --stat 8600fc8` (337-line study promoted to durable sources) | yes |
| E1 — `ses_07bcf07cc` named ownership seam "core blocker" at 17:20:29, BUILD-READY/HIGH at 17:34:15 | `researches/sources/2026-07-24-opencode-history-visibility-study.md` E1 + Sessions appendix (`ses_07bcf07ccffe5woS8ozf0Bc8cn`, 07-21 17:20; BUILD-READY 17:34:15) | yes |
| DAY-0 = property-map row 7, NEW SLOT, BUILD-READY crossing, BLOCKS | `researches/sources/2026-07-25-seven-controls-property-map.md` (DAY-0 row; "no shipped design-phase adversarial gate") | yes |
| Block-BUILD-READY authority row cites F3 + `9dbab50` basis | `researches/decisions/2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md` L128 (Block a BUILD-READY → safety-layer gate, F3 design-gate, basis `9dbab50`) | yes |
| Behavioral-closure pilot (parser principles F3 reuses) shipped + doctor #14 | F1 memo Evidence table → `git show --stat 7f95f29`, `git show --stat 4d8d725` | yes (via F1 memo) |
| F3 is sole BLOCKS family in F1/F2/F3 set | `9dbab50` Decision 1 + `68a8fc4` union-list sub-section (F1/F2 INFORMS, F3 BLOCKS) | yes |

House style: this memo follows the `2026-07-25` (F1/F2) / `2026-07-22` /
`2026-07-23` convention (bolded-metadata frontmatter; Framing → Decision →
Mechanism → Authority → Contradictions → Evidence), matching the F1 and F2
memos' granularity exactly — decision granularity, not the per-slice build plan.
