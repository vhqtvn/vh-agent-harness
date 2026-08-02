# Decision Brief: Formal-Verification Capability — Agent-Authored Proofs, Engine-Checked

**Date:** 2026-08-02
**Status:** Proposed S1 Overlay Pilot (Design-Only, operator-signed-off 2026-08-02). No skill code
lands in this memo; this is the design spec a future slice authors from.
**Paradigm:** Agent-authored proofs, engine-checked (Lean4 + TLAPS safety-invariants).
**Supersedes (paradigm axis):** the earlier TLC/model-checking-centric draft that lived at
`tmp/2026-08-02-tiered-formal-verification-skill-design-brief.md` (gitignored scratch, now deleted).
The TLC-first conclusion is **SUPERSEDED** — TLC/model-checking puts the *reasoning* in the engine
(wrong paradigm: the engine brute-forces the state space); this design keeps the *reasoning* in the
agent (the agent authors the proof; the engine only provides a rigorous language and mechanically
checks it). **Do NOT inherit the model-check-first framing**, the TLC-as-canonical-Tier-2-tool
position, the fast-check/`testing/quick`-as-Tier-1-default position, or the four-tool ladder
(TLC + Alloy + Lean + property-test). The deleted draft's tooling-feasibility/exec-sandbox analysis
is also not carried forward; this design is operator/offline-provisioned only.
**Precedents:**
- `contract-invariant-audit` — `.opencode/skills/contract-invariant-audit/SKILL.md` +
  `researches/decisions/2026-07-29-contract-invariant-audit-capability.md` for the
  S1-overlay, INFORMS-only, domain-free skill form (skills-only overlay pack, no
  agent/command/permission/gate surface, `opencode-append.jsonc` a no-op `{}` shell).
- `behavioral-closure-pilot` — `researches/decisions/2026-07-24-behavioral-closure-pilot.md` +
  `internal/cli/doctor_behavioral_closure.go` for the verdict/crux token the engine check *feeds*
  (never independently justifies a code `proven`).
**Provenance trail:** produced via phased solution-brief (researcher → debate → planner); working
artifacts at `tmp/agent-runs/formal-verification/{research,debate,brief}.md` (gitignored; the
synthesis is captured in this memo).
**See also:** `researches/decisions/2026-07-23-vh-solara-orchestration-field-report-disposition.md`
(the HYBRID merge-vs-union rule + the docker-gold "the token does NOT prove reality" load-bearing
caveat this design extends).

---

## Framing

This design brief synthesizes the research and debate findings into a formal specification for an
agent-authored formal-verification skill. Operating strictly as an S1 overlay pilot, the capability
allows an agent to author proofs checked mechanically by a rigorous engine. This brief establishes
the precise boundaries of authority, configuration, degradation, and the critical model-fidelity
binding required to prevent laundering a model proof into a code-behavior proof.

## Authority & S1 Containment

**INFORMS ONLY.** Every output informs; the skill never gates, blocks, approves, promotes, releases,
or transitions state on its own.
- **No Auto-Invocation:** The capability must never auto-invoke on edits, commits, doctor, release,
  or update paths.
- **Gate Promotion Separation:** Gate promotion (if any) is a SEPARATE decision that must name the
  protected transition, predicate, validator ownership, false-positive recovery, and explicit
  authority. The pilot grants NONE.
- **Provisioning Constraint:** Operator/offline provisioning ONLY. The skill consumes pinned direct
  binaries or pre-built images. It NEVER triggers networked docker builds.

**Pilot Scope:** This is strictly an S1 overlay pilot. Core promotion is NOT authorized. The S2
evidence classes remain unmet until a real pilot runs and records positive evidence. No skill, no
overlay pack, no helper, no agent, no command, no permission surface, and no gate is authorized by
this memo — it is the design spec a future operator-approved slice authors from.

## The Paradigm

The paradigm is fixed: the agent WRITES the proof; the engine only provides a rigorous language and
mechanically checks it.
- **Lean4 (dependent type theory)** is the primary engine for pure-logic, algebraic, and simple
  state-machine invariants.
- **TLAPS (TLA+ Proof System)** is the engine for the **SAFETY invariants** of concurrent-system
  models. It is expressly for safety-only, NOT temporal/liveness reasoning.
- **TLC/Model-Checking** is NOT permitted, as an engine brute-forcing the state space means the
  engine reasons, violating the agent-authored paradigm.
- **Property-Test Prefilter:** Property testing is exposed as an advisory
  `prefilter: property-test | none` setting ONLY. It serves as a cheap pre-filter to falsify
  candidates but NEVER becomes proof evidence. Outputs are separately labelled and carry no doctor
  hook.

**LIVENESS is explicitly OUT of the pilot.** TLAPS is safety-only; liveness/eventual-settlement and
temporal properties are excluded from the pilot's provable surface. (Liveness reasoning has no
engine in this pilot; an honest exit to property-test-prefilter-or-no-proof applies — see
Invocation/Classification Rule (d).) This is the operator-confirmed scope; do not re-open it.

## THE CRUX — Model-Fidelity Binding

A proof proves the MODEL, not the CODE. To prevent the vh-solara laundering trap (where a diverged
model goes undetected or fake fidelity claims are asserted), the design enforces strict fidelity
binding.

### Canonical crux example (operator-confirmed, re-grounded from source)

The motivating failure this design exists to prevent is **vh-solara field-report Pattern-1
"GREEN-TESTS / BROKEN-PRODUCT"**
(`researches/sources/2026-07-23-vh-solara-harness-adoption-field-report.md`, Pattern-1): the
in-process e2e (`tests/e2e/`) was green but **structurally could not exercise the load-bearing
path** — the fake OpenCode fixture always emitted the delete event
(`pkg/fixtures/opencode.go:1214-1217`: *"Emit a session.deleted event so the aggregator's event
subscriber drops it"*), so it could never produce the *missed* delete that the reconcile tick
exists to cover (`pkg/aggregator/tree_reconcile.go:27-28`: *"Missed session.deleted → the store
holds a ghost node; this tick emits node.remove to evict it deterministically"*). The one lane that
could have proven the crux live/adversarially — **"Prove Phase 2 in docker-gold lane"**
(`ses_070f57e44ffeaAwXnITi4Q09fN`, the only context that can produce a genuine missed-event delete
via raw SQLite row-delete) — **was abandoned mid-recon (compaction, no closeout)**; the docker-gold
proof lane never wrote an assertion, never ran, never proved the crux. Despite that, Phase-2 was
committed as "complete" (`e5d09d4`), and the diff reviewer's advisory finding F9 ("reconcile-tick
ghost-eviction self-heal is unit-tested but not exercised end-to-end") was dispositioned `drop`,
not deferred.

This is the exact laundering this design targets: a verification artifact (the green e2e) was
treated as proof of the load-bearing behavior when it was structurally incapable of exercising that
behavior, the proof lane that could close the gap was abandoned with no consequence, and "complete"
landed anyway. **Do NOT use the loose "B-F1/C-F1/C-F2" labels as the crux anchor** — those are the
harness FIX's regression labels (a different symptom surface in a later remediation), not the
consuming-repo symptom that motivates this design. The canonical anchor is Pattern-1's
fake-short-circuits-the-crux + abandoned-proof-lane + "complete"-nonetheless.

### The fidelity binding (enforced)

- **Fidelity-Binding Artifact:** A declared mapping from the model transitions/variables to the code
  locations.
- **Red-on-Divergence Test:** A targeted code-level invariant regression test that MUST go red on
  divergence. This is the cheapest recheck that catches a diverged model. (The Pattern-1 lesson: a
  test that *cannot* go red on the load-bearing divergence — the fake always emitting the delete —
  is not a fidelity binding; it is the laundering surface.)
- **Distinct Union Evidence:** The engine check and the fidelity binding are DISTINCT UNION evidence
  (fail-closed if either fails). The engine check feeds the existing behavioral-closure crux model
  ONLY with real crux evidence; it must NEVER independently justify a code `proven` verdict.
- **Anti-Over-Claim / Anti-Laundering Rule:** A proof of a model is not a proof of the code.
  "Proven" requires the engine-ran + reviewed-fidelity-binding + red-on-divergence test present.
  Silent model drift, dropping the fidelity binding, or reporting a model proof as a code proof are
  FORBIDDEN. This is the direct extension of the vh-solara disposition's "the token does NOT prove
  reality" caveat: a green engine check, like a green verdict token, must not be laundered into
  "the code is correct" without the repo-specific live verification (the docker-gold pattern: seed
  real data, induce the real failure, observe the fix, show the assertion goes RED on a pre-fix
  build).
- **Surviving Objection & Falsifier:** The binding is falsifiable evidence, not a mechanically
  proven code↔model equivalence. If this mechanism STILL lets a diverged-model "proven" through
  (the Pattern-1 failure mode repeats under this design), the mechanism has failed.

## Degradation & The Result Token

The design-time product (the proof/spec + fidelity binding) is ALWAYS producible and NEVER blocks.
- **Verifier Infeasibility:** If no engine is reachable, record proof-written-but-unchecked as
  diagnostic metadata. This maps to the shipped `result: not-demonstrable` / `verdict: inconclusive`
  (MERGE: engine unavailability is verifier infeasibility, not a new closure property). An engine
  that did not run can never yield `proven`; it routes to `not-demonstrable`/`inconclusive` and
  never blocks — this is the disciplined response to the Pattern-1 "abandoned proof lane" failure
  (the lane vanishing did not block "complete" then either; the fix is the honest token, not a new
  gate).
- **Claiming "Proven":** "proven/green" is claimable ONLY when the engine actually ran AND the run
  is bound to a reviewed fidelity claim. The NEVER-blocks invariant must be preserved.

## Engine-Invocation Config & Docker Containment

The exact schema uses a 4-level, two-file, field-by-field merge mirroring the `auto-classifier-pilot`
(`defaults <- user <- committed-project <- project-local`). It resolves via findLast (user-level
default + project override). This is NOT a parallel config scheme — it reuses the established
two-file field-merge convention at `.opencode/repo-configs/`.
- **Configuration Files:** `.opencode/repo-configs/<engine>-config.json` (behavior-committable) and
  `.opencode/repo-configs/<engine>-config.local.json` (secrets/local override, never committed).
- **Provisioning Mode:** The skill discovers/selects the engine and invokes it via the
  `provisioning` field (`direct-binary | docker-image`). Projects can override the user default
  (e.g., "this project runs lean via docker image X; that one uses the host binary").
- **JSON Schema Sketch:**
  ```json
  {
    "enabled": true,
    "engine": "lean4", // lean4 | tlaps | none
    "provisioning": "docker-image", // direct-binary | docker-image
    "binary_path": "",
    "docker_image": "leanprovercommunity/mathlib4:v4.32.2", // illustrative; operator MUST pin to a real checked tag — :latest is forbidden by the Provisioning Constraint
    "onUnavailable": "defer"
  }
  ```
- **Docker Containment Caveat (Surviving Objection #2):** Docker checker containment is
  container-side (read-only mounts, `--network=none`), NOT inherited from exec-sandbox. The pilot
  must NOT imply a new safe Docker execution wrapper; it only documents the container config the
  operator must supply. **A contained-docker-checker `exec` verb stays OUT of the pilot** — the
  skill consumes operator/offline-provisioned pinned binaries or pre-built images and never
  triggers a networked build, so no new harness exec verb is introduced or implied.

## Invocation / Classification Rule

A seconds-fast decision tree the agent runs:
1. **(a) Pure-logic/algebraic invariant** -> **Lean4** (e.g., "For integers a <= b and b <= c,
   prove a <= c").
2. **(b) Simple state-machine invariant** -> **Lean4** (e.g., "A single-threaded bounded counter
   begins in [0,max]; inc/dec preserve bound").
3. **(c) Concurrent-protocol safety invariant** -> **TLAPS** (e.g., "For a lock protocol, no
   reachable state has two owners").
4. **(d) Honest exit to property-test-prefilter-or-no-proof** -> Liveness, underspecified
   heuristics, or disproportionate-cost claims.

*The Boundary:* A Lean encoding of a transition system becomes the wrong tool versus TLA+ when real
interleavings/concurrency must be modeled. A sequential Lean encoding erases them, necessitating
TLAPS.

## Honest Scope + Climbing Path

- **Provable NOW:** Pure-logic, algebraic, simple state-machine invariants, and concurrent-protocol
  safety invariants via TLAPS.
- **The Frontier:** Iris-grade concurrent separation logic and liveness/temporal proofs (out of
  pilot scope — see The Paradigm: LIVENESS is OUT). Do not over-promise.
- **Climbing Path:** Run the S1 pilot on a bounded set of real candidate bugs, establish the
  baseline value, collect S2 evidence, and evaluate cost before expanding to harder concurrency
  forms or new engines.

## Two-Sided Health Signal & Falsifiers

Predeclared before the pilot runs:
- **(a) VALUE Side:** Does it CATCH real bugs an agent's prose reasoning missed?
  - *Falsifier:* Green check counts rise but no net-new bug is caught vs prose reasoning ->
    STOP/reshape.
- **(b) COST Side:** Does it stay cheap / not become ceremony?
  - *Falsifier:* Proof-authoring latency dominates and lanes avoid the skill -> STOP/reshape.

## S2 Evidence Classes

The S1 overlay pilot must produce eight classes of positive evidence before any core promotion
claim:
1. Utility (net-new defects caught)
2. Cost (latency remains bounded)
3. Safety (never blocks normal lanes)
4. Maintenance (red-on-divergence tests avoid flakiness)
5. Determinism (stable checker engine results)
6. Usability (configuration is easily discoverable)
7. Adoption (voluntary selection by developers)
8. Reversibility (clean removal leaves code untouched)

## References

- `researches/decisions/2026-07-29-contract-invariant-audit-capability.md` — S1-overlay,
  INFORMS-only, domain-free skill form precedent.
- `.opencode/skills/contract-invariant-audit/SKILL.md` — the skill shape this pilot mirrors.
- `researches/decisions/2026-07-24-behavioral-closure-pilot.md` + `internal/cli/doctor_behavioral_closure.go`
  — the verdict/crux token the engine check feeds (never independently justifies `proven`).
- `researches/decisions/2026-07-23-vh-solara-orchestration-field-report-disposition.md` — the HYBRID
  merge-vs-union rule + the docker-gold "token does not prove reality" caveat this design extends.
- `researches/sources/2026-07-23-vh-solara-harness-adoption-field-report.md` — Pattern-1
  (GREEN-TESTS/BROKEN-PRODUCT), the canonical crux example.
- `templates/overlays/auto-classifier-pilot/README.md` — the 4-level two-file field-by-field
  merge convention (`defaults <- user <- committed-project <- project-local`, findLast) the
  Engine-Invocation Config reuses.
- `tmp/agent-runs/formal-verification/research.md` — source packet (gitignored).
- `tmp/agent-runs/formal-verification/debate.md` — debate verdicts (gitignored).

---

```behavioral-closure
verdict: inconclusive
result: not-demonstrable
```
*Honesty caveat: The design's internal consistency is review-verifiable, not runtime-proven. The
formal-verification paradigm itself (engines prove models, not code) is the load-bearing limitation
the fidelity-binding design addresses but does not eliminate. No load-bearing code path ran — this
is a design product only. No engine ran; per this design's own Degradation rule, that maps to
`result: not-demonstrable` / `verdict: inconclusive`, never `proven`.*
