# Case study: why a known bug shipped three releases in a row

**Subject:** the `core/media-perception` unconditional-rendering defect (v0.12.0 → v0.14.0)
and the cluster of process failures around it.
**Audience:** harness maintainers and agent sessions studying failure modes.
**Scope note:** this is a process case study, not a bug report. The concrete bug has its
own solution brief (CoreOutputs / capability-owned outputs); this document explains why
the *system* let it recur, and what generic mechanisms prevent the class.

---

## 1. What happened (condensed timeline)

The events span one long-running design/build session (~1 week, 30+ delegated
subsessions) plus three releases:

1. **Design + build.** A new opt-in capability `core/media-perception` was designed
   (skill + a single generalist specialist agent + caller routing) and implemented — an
   orchestrator-plus-hidden-leaves cluster was considered and explicitly rejected during
   design. Template-level gating
   (`{{ if .capabilities.media_perception }}`) was applied inside `opencode.jsonc.tmpl`.
2. **A rendering gap was found and deferred.** Plain (non-`.tmpl`) files —
   `.opencode/agents/media-perception.md` and `.opencode/skills/media-perception/SKILL.md`
   — render unconditionally, because the renderer walks the entire embedded
   `templates/core/` corpus via `fs.WalkDir` with no file-level capability filter.
   The gap was captured as a defer, judged **low severity** ("agent is inert without the
   capability — no caller edges, deny-all task map; this is docs drift").
3. **v0.12.0 shipped a false claim.** Its migration note stated: "When NOT selected,
   none of these render." That was true for `opencode.jsonc` edges, false for the two
   leaf files. The claim was documented but never executed as a test.
4. **An attempted docs fix was itself a violation.** A build slice edited the released
   v0.12.0 migration note to correct the claim; the operator caught it ("released docs
   are immutable"). The revert of *other* previously-drifted released notes was caught
   as a second violation of the same rule. The rule then became a real gate
   (`TestMigrationNotes_ReleasedImmutable`, commit `e929149`), and the violation class
   stopped.
5. **The capability passed component tests but failed in production.** Live behavioral
   tests (3/3 — all image sub-classes: chart, screenshot, photo; video/PDF/audio were
   never tested) passed — but they invoked the specialist by manual delegation.
   In a consuming repo, the actual *caller* agent self-refused ("I can't read the image
   attachment") instead of delegating. The discovery path had never been tested.
   Caller-side routing was then added (v0.14.0, commits `48f12be` + `7767cf6`).
6. **A stale premise resurfaced.** During a later commit review, the commit-review
   orchestrator's checklist asserted "the dogfood profile does NOT select
   media-perception" — a fact
   that had been true earlier but was invalidated by an intervening commit (`abc8a91`).
   The stale fact had survived context compression and was re-asserted as current truth.
7. **The rendering bug was re-reported after each release.** An adopter filed the same
   symptom on v0.12.0, v0.13.1, and v0.14.0. Each time, the system recognized it as
   "known, deferred, low severity" and shipped around it. The third report arrived with
   understandable frustration, and only then was the defer re-adjudicated as must-fix.
8. **A sibling policy bug was found in the same period.** Read-only specialist agents
   ship `"vh-agent-harness *": "allow"` in their bash permission leaf (mutation verbs
   reachable from designated-RO agents), and the `debate`/`solution-brief` family
   carries a dead `exec-ro` allow rule shadowed by a later broad deny — because the
   rules were authored against a "most-specific-wins" mental model while the actual
   evaluator is last-match-wins (`findLast`).

None of these failures involved a malfunctioning tool. Every gate that existed worked.
The failures happened where **no gate existed**.

## 2. The failure classes

| # | Failure | Class |
|---|---------|-------|
| F1 | Known bug shipped 3 releases | **Deferred-debt decay** — severity verdict frozen at capture time; recurrence never escalated it |
| F2 | Stale premise re-asserted post-compression | **Fact staleness** — observation stripped of its validity condition |
| F3 | Released doc claimed unexecuted behavior | **Claim/test divergence** — documentation asserted what no test verified |
| F4 | Caller self-refused in production | **Node-tested, path-untested** — component acceptance mistaken for system acceptance |
| F5 | Released docs edited (twice) | **Prose rule without a gate** — rule known, mechanically unenforced |
| F6 | Two notions of "managed set" | **Dual derivation** — renderer, doctor, and manifests each computed "managed" independently |
| F7 | Dead permission rule; RO leaves reach mutation verbs | **Authoring against the wrong evaluator** — policy written to a mental model, not the real matcher |

## 3. Root cause, stated once

Every failure above is the same event in different clothing:

> **A claim crossed a lossy boundary without being re-checked, and the receiving side
> treated it as truth.**

The lossy boundaries in an agent harness:

- **Compression** — long conversations are summarized; observations become assertions,
  losing their validity conditions (F2).
- **Delegation** — a subsession receives a prompt, not the history; premises arrive
  pre-digested and are not re-derived (F2, F4).
- **Release** — working knowledge is frozen into documents; the documents outlive the
  verification that produced them (F3, F5).
- **Time** — a defer survives while the context and evidence around it evaporate;
  its original severity verdict silently goes stale (F1).
- **Authoring** — an agent's mental model of an evaluator is not the evaluator (F7).
- **Design** — two subsystems independently compute "the same" set and drift (F6).

Agent systems make **claim production nearly free** (docs, summaries, premises,
permission tables are all generated fluently) while **claim verification stays
expensive** and is therefore skipped. Unverified claims accumulate at every boundary
until one detonates. A larger context window shrinks the *frequency* of compression
losses but leaves the structure intact — delegation, release, and time remain lossy at
any window size.

The generic rule that dissolves the class:

> **At every lossy boundary, a load-bearing claim must be (a) re-derived from its
> source, (b) mechanically verified, or (c) explicitly marked unverified.**

Everything in §4 is this rule specialized per boundary.

## 4. Proposed mechanisms

### 4.1 Invariant registry with a closure rule — *prose is not enforcement*

The immutability arc proved the pattern empirically: a rule stated in prose was violated
twice; the moment it became a test, the violation class ended. Agents are probabilistic —
a prose rule has a per-invocation violation probability, and across dozens of
subsessions that probability integrates to ~1.

Mechanism:

- Every MUST/NEVER the harness asserts (AGENTS docs, skills, review checklists,
  migration notes) gets an ID and a declared **enforcement point**: a test, a `doctor`
  check, a shell-guard rule, or a commit-gate validator.
- Meta-invariant: **an invariant without an enforcement point is itself a `doctor`
  warning.** A lint walks normative language in core docs and demands each maps to an
  enforcement artifact — or an explicit `enforcement: declined (reason)` record.

This converts rule discovery (violation → operator catch → rule stated) into rule
compilation (rule stated → gate must exist before the thread closes).

### 4.2 Facts as cache entries — *summaries carry pointers, not truths*

Compression converts "at time T, the profile did not select X" into "the profile does
not select X". Any mutable fact carried across a boundary is a cache entry without an
invalidation key.

Mechanism:

- Load-bearing facts in summaries and dispatch prompts are stored as
  *(value, source, re-derivation command)* — not "profile doesn't select X" but
  "profile selection: check `grep capabilities <profile file>`".
- **Premise-check step in the dispatch protocol:** before a subsession acts, it
  enumerates the premises its task depends on and re-derives the cheap ones (a grep, a
  `doctor` run) instead of trusting the parent's summary. One grep would have caught F2.
- Compressor guidance: prefer preserving **decisions and their reasons** (stable) over
  **world-state** (mutable). Where world-state must be carried, timestamp it.

### 4.3 Debt liveness — *recurrence must escalate*

A defer is a contract with a trigger, but triggers based on prediction fail. The defect
here was not the initial low-severity call (defensible at the time); it was that three
recurrences of the same symptom never forced the call to be recomputed.

Mechanism:

- **Symptom → defer mapping.** When a report resolves to an existing defer, that is a
  *recurrence event*, not new noise. Priority strictly increases per recurrence; the
  second recurrence forces re-adjudication with the operator.
- **Release-boundary re-adjudication.** Release readiness enumerates open defers
  touching shipped surfaces and demands a fresh verdict on each. Specifically: **no
  release ships while an open defer contradicts a claim in released or about-to-release
  docs.** This alone would have blocked v0.13.1 and v0.14.0 from shipping around a
  known-false claim in the v0.12.0 note.
- **The gate must read state directly, not trust trigger promotion.** Post-publication
  evidence sharpened this mechanism: the erratum card for the v0.12.0 false claim had a
  documented trigger ("fires when any new migration-note file is created") that fired
  on three consecutive releases — and the card stayed `draft` through all three. So it
  is not only severity *verdicts* that decay; the trigger *machinery itself* fails
  mechanically. A release-readiness gate must therefore be **mandatory-primary**
  (enumerate open defers/errata and FAIL on contradiction at the boundary), with
  trigger-based promotion demoted to a curation aid it never depends on.
- **Severity verdicts carry expiry:** valid for K releases or until recurrence,
  whichever comes first; then stale and recomputed.

### 4.4 Declare once, derive everywhere — *dual derivation is a lint-able smell*

The managed-set inconsistency existed because three subsystems each computed "what the
harness manages": the renderer (unconditional corpus walk), `doctor` (corpus ownership
defaults), and the manifests (overlay skills only). No component was individually wrong;
they answered different questions with the same word.

Mechanism:

- Every set/spec consumed by multiple subsystems must be **declared once** and derived
  everywhere (the CoreOutputs / capability-owned-outputs design is exactly this).
- **Parity tests as standing infrastructure:** any set with multiple views (renderer's,
  doctor's, manifest's, docs') ships with a test asserting the views agree on every
  element. The token-survival and flag-parity tests from the caller-routing slice are
  instances of this pattern; the upgrade is making it a habit — *every new set ships
  with its parity test.*
- Review heuristic: finding two independent computations of "the same" set is a latent
  bug even while they agree.

### 4.5 Behavioral parity matrix — *test the path, not the node*

The 3/3 modality pass was real and still missed the production failure, because the
tests invoked the specialist from a position no production caller occupies. The unit of
behavior in an agent system is not the agent — it is the **route**: entry point →
recognition → delegation → report → synthesis.

Mechanism:

- For each capability, enumerate **{caller} × {capability on/off} × {expected observable
  behavior}**, declared next to the capability manifest so a capability without
  behavioral rows is visibly untested.
- Probes start at the **production entry point** (the caller agent receiving a
  user-shaped task), not at the specialist.
- **The OFF row is mandatory.** Capability-off behavior (caller informs the user
  gracefully, does not probe, does not hallucinate the capability) is the row nobody
  tests by default — and it is where both the false migration-note claim and the
  production self-refusal lived.
- Acceptance signals attach to routes, not nodes: the original "must reach for a tool,
  not refuse" signal was well-designed but attached to the specialist, while the actual
  failure was the caller's refusal.

### 4.6 Policy simulation — *author against the real evaluator*

Permission tables were authored under a most-specific-wins model; the evaluator is
last-match-wins. No amount of authoring care fixes a semantics mismatch — the authoring
context cannot reliably carry evaluator semantics.

Mechanism:

- The permission emitter gains a **semantic lint pass** that runs the *actual* evaluator
  over the authored rules:
  - **Dead-rule detection:** a rule that can never be the winning match for any input in
    its own pattern space is a build error (catches the shadowed `exec-ro` immediately).
  - **Reachability invariants:** declared-RO leaves are evaluated against a corpus of
    known mutation verbs; any allow-path to a mutation verb fails the build.
- Structural default for constrained leaves: **allow-list + deny catch-all**
  (fails closed on future verbs), never broad-allow (fails open on every verb the
  harness adds later).

### 4.7 Correction closure — *operator corrections are specification*

Every durable improvement in this arc traces to an operator catch ("released docs are
immutable", "we're a generic platform", "solution brief first"). Each correction is a
specification fragment delivered at the highest possible cost — after the violation —
and the system required the operator to push twice before a correction crystallized
into a gate.

Mechanism:

- **Correction closure protocol:** when the operator overrides or corrects, the thread
  may not close until the correction lands as exactly one of:
  1. a gate (per §4.1),
  2. a durable doc/skill edit, or
  3. an explicit *declined-to-enforce* record with a reason.
- This is the learning loop of the whole system. Leaving it implicit means paying for
  each lesson repeatedly — which is precisely what the repeated bug reports were
  describing from the adopter's chair.

## 5. Unified picture

```
                       claims flow →
   ┌────────────┬──────────────┬───────────────┬────────────┐
   │ authoring  │ compression  │  delegation   │  release   │  ← lossy boundaries
   ├────────────┼──────────────┼───────────────┼────────────┤
   │ policy     │ facts as     │ premise-check │ defer      │
   │ simulation │ cache entries│ + route tests │ liveness   │  ← gate per boundary
   │ (§4.6)     │ (§4.2)       │ (§4.5)        │ (§4.3)     │
   └────────────┴──────────────┴───────────────┴────────────┘
        all invariants registered & closure-checked (§4.1)
        all shared sets declared-once + parity-tested (§4.4)
        all operator corrections compiled to gates (§4.7)
```

## 6. Priority guidance

If adopting incrementally, order by (prevented pain ÷ implementation cost):

1. **§4.3 defer liveness** — directly prevents the headline failure (recurring known
   bug across releases); mostly registry/process logic.
2. **§4.6 policy simulation** — cheapest to build (the emitter already owns the
   tables); converts an entire bug class into build-time failures.
3. **§4.1 invariant registry** — the multiplier: makes every future lesson stick.
4. **§4.5 behavioral parity matrix** — highest test-authoring cost, but the only
   mechanism that catches integration failures before adopters do.
5. **§4.2 / §4.4 / §4.7** — protocol habits; adopt as review checklist items first,
   automate opportunistically.

## 7. What this is not

- Not an argument for larger context windows. The window was never the binding
  constraint: the session's compression layer never hit a context-limit emergency, and
  the recurring bug was a *prioritization* failure, not amnesia. Bigger windows reduce
  compression losses but leave delegation, release, and time lossy.
- Not a criticism of deferring. The initial low-severity call was defensible with the
  evidence available. The defect was the absence of a mechanism to *revisit* the call
  as evidence accumulated.
- Not specific to media-perception. The capability is incidental; the boundaries are
  structural. Any future capability, policy table, or release note crosses the same
  boundaries and is exposed to the same classes until the gates in §4 exist.

---

## Revisions

- **2026-07-23** — corrections from an independent post-mortem review of the source
  session (both errors were, fittingly, unverified claims that crossed a lossy boundary
  into this document): §1.1 the shipped agent is a single generalist leaf, not an
  orchestrator (the cluster design was rejected); §1.5 the "3/3" behavioral pass covered
  three *image sub-classes*, not three modalities; §1.6 clarified which orchestrator
  (commit-review) held the stale premise. §4.3 gained the erratum-trigger evidence
  (trigger machinery fails mechanically; the gate must be mandatory-primary). The
  §4.3/§4.1/§4.2 design response to this document is recorded in
  `researches/decisions/2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md`.
