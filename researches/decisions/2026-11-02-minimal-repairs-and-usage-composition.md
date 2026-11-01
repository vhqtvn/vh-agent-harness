# Decision — Minimal Repairs, Lean Doctrine, and Usage Composition

**Date:** 2026-11-02  
**Status:** decided (repairs landed, lean doctrine and usage-model reframes adopted, no-action register settled)  
**Authority:** operator authorized  

---

## 1. Provenance

This decision memo records the conclusions of a completed learning-and-repair cycle based on a session-mining study of one external project's agent operations over approximately six weeks. The mined project operated a parallel-lane supervisor model characterized by frontier-model improvisation without an agent harness. By operator rule for this repository, the external project's real identity is strictly withheld; it is designated exclusively as "the mined project."

The study progressed through two distinct research-and-brief passes:
1. **Failure-mode lens:** Catalogued recurring execution defects, verification slips, and protocol drift, mapping them against existing harness protections and identifying tensions.
2. **Usage-model lens:** Re-evaluated the mined project's topology, roles, artifacts, and cadences following an operator reframe to determine whether any multi-lane usage abstractions warranted introduction into the harness.

All intermediate scratch artifacts and working notes generated during these passes are disposable and intentionally deleted post-decision. This record is completely self-contained.

---

## 2. Lean Doctrine (Adopted)

The harness adopts an explicit **graduation rule** governing when prose guidance should transition to mechanical enforcement:

- **Recurrence threshold:** A prose rule graduates to an active mechanization investigation only upon **at least two independent, receipt-backed violations of the same rule across distinct dogfood tasks while that rule was in force**. Deduplication applies to retries, and failed or blocked tasks count toward eligible observations. Missing records must not be treated as zero failures.
- **Immediate escalation bypass:** Any single reproducible defect involving **scope integrity, unauthorized state mutation, destructive writes, or false-transition claims** bypasses the recurrence count and triggers an immediate, bounded investigation.
- **Investigation, not auto-authorization:** Neither trigger automatically authorizes a gate. Every new mechanical control must independently establish:
  1. A deterministic, side-effect-free predicate.
  2. A clearly identified owning seam.
  3. Bounded runtime and maintenance costs.
  4. An observable pilot demonstrating outcome value.
  5. An explicit authority decision defining whether the check is advisory (INFORMS) or a hard transition gate.
- **Keep-as-prose stances:**
  - **Scope and product fit** remain the exclusive domain of operator and reviewer judgment. Heuristics and proxy metrics cannot evaluate product intent.
  - **Complexity remains INFORMS:** Complexity signals direct attention to growth and drift, but never unilaterally block legitimate engineering work or approve expansion.
- **Structural honesty vs. evidence truth:** Mechanical consistency checks verify declaration structure and internal consistency, not empirical truth. Rejecting an inconsistent declaration does not prove that a cited command ran, that a receipt is genuine, or that a user-visible outcome occurred.

---

## 3. Usage-Model Verdicts and Adoptable Reframes

### Five Product Verdicts

1. **Is a wave structurally more than task cards + workstream + synthesizer?**  
   *Semantically yes; a new first-class runtime does not follow.* A wave conceptually introduces descope ordering, lane disjointness guarantees, sealed dispatch orders, quota-aware resumption, and aggregate exit conditions. While current harness contracts and checkpoints can express these relationships descriptively, they do not mechanically enforce them. However, dogfooding does not demonstrate demand for absent-owner multi-lane enforcement machinery. Current practice does not justify building a wave runtime.

2. **Native layer, templates/lints, composition patterns, or decline?**  
   *Decline new usage surface now.* A named composition pattern over existing primitives (cards, contracts, workstreams, checkpoints, and detached jobs) is the smallest credible future candidate if a concrete handoff omission appears during multi-session execution. Templates and advisory lints require demonstrated recurring omissions that deterministic checks can reliably detect. A native runtime layer is rejected. No new scheduler or orchestration engine is recommended.

3. **Which single element carries the most adopter value?**  
   *Owner communication is the strongest candidate for dogfood, though not a proven adopter-wide winner.* The relative value ranking is:
   1. *Owner communication:* Concise, decision-oriented summaries (conclusions, evidence, residuals, explicit decisions needed, safe no-response behavior).
   2. *Sealed brief + advisory lint:* Protects against revision confusion, though a hash proves text identity rather than correctness.
   3. *Decision authority as data:* Provenance tracking and explicit scope references (already expressible in cards and contracts).
   4. *External-review handoff/reconciliation:* Sanitized handoff and revision-bound finding reconciliation when independent review is sought.
   5. *Lane contract/confinement:* Worktree isolation and diff confinement (high mechanism cost; insufficient local demand).
   6. *Overnight/quota cadence:* Highly specialized to an absent-owner, high-concurrency operating model.  
   None of these elements currently clears the threshold for adding new surface to the harness core.

4. **Which decision carrier should remain?**  
   *Retain git-anchored durable records and retirement-gate preservation.* Live append-only decision ledgers provide immediate in-flight visibility, but git-anchored carriers tied to branch-reachable landing commits guarantee canonical durability without maintaining a secondary out-of-band database. In-flight notes and checkpoints suffice for live context; receivers must re-derive mutable premises at consumption boundaries.

5. **Is the low-attention-owner surface in scope?**  
   *Yes as communication practice; no new core surface now.* Generic report and decision-request principles belong in optional operator guidance or profile overlays, never hard-coded into the domain-free core. Crucially, keyword parsing is not approval, silence is never consent, and brevity must never suppress a protected decision or obscure a material evidence limitation.

### Adoptable Reframes

- **Represent relationships before building runtimes:** Coordination structure must follow observed work patterns rather than imported organizational topologies. Express dependencies, lane boundaries, and descope orders explicitly within existing contracts before designing runtime engines to police them.
- **Cards are transport, briefs are dispatch views:** Task cards serve as ephemeral execution transport that retires upon landing. A dispatch brief is a scoped lens on current work, not a competing permanent ledger. A workstream carries contextual memory across sessions, not a rival authority system.
- **Reduce attention via better evidence, not weaker gates:** Protect operator attention by improving the density, clarity, and verifiability of review evidence rather than bypassing safety checks. Communication preferences must never alter authorization boundaries.
- **A missing named tool is not evidence of unmet need:** The presence of an improvised tool in an external workflow does not establish a requirement in this harness. Avoid mechanizing ceremonies that local dogfooding has not demanded.

---

## 4. No-Action Register

The following proposals are explicitly rejected or declined with specific rationale:

| Proposal | Disposition | Rationale |
|---|---|---|
| **Native wave/lane runtime objects** | Declined | Unsupported demand in current dogfood; introduces complex concurrency, scheduling, and write-confinement responsibilities. |
| **Live project-level decision ledger** | Declined | Git-anchored records with landing-gated retirement provide verifiable durability without maintaining a redundant approval database. |
| **Rubric scoring kit** | Declined | Existing structured debate with explicit assumptions and sensitivity flip facts suffices absent consequential, recurring multi-criteria decisions. |
| **Quota auto-resume engine** | Declined | Requires transport-specific runtime hooks and autonomous redispatch authority unjustified by the single-process OpenCode execution model. |
| **Supervisor scheduler / cron loop** | Declined | Task-contract timeboxes, explicit stop conditions, and background shell job survival cover pauses without autonomous loop daemons. |
| **Owner-profile templates** | Declined | Personal communication preferences belong in operator configuration or overlay packs, not within the domain-free core. |
| **Pipeline exit hiding regex (`cmd \| tail`)** | Declined | Exit code propagation depends on shell dialect, option state (`pipefail`), and wrapper semantics; a regex pattern produces false positives and cannot guarantee safety. |
| **Timestamp provenance lint** | Declined | Scanning arbitrary prose for date-time patterns generates false positives against quoted or historical text; product-owned timestamps should be generated by write layers. |
| **AGENTS.md size quota / line budget** | Declined | Line count alone does not establish attention failure or instruction decay; rendered-from-template corpus design already prevents phrasing drift. |

---

## 5. Repairs

### (a) Commit-Gate Operator Bypass Staging Hazard (Approved & Implemented)

- **Defect:** The `commit-gate.sh` operator bypass (`SKIP_COMMIT_GATE=1`) historically executed `git add -A`, inadvertently sweeping all untracked and modified working-tree files into the bypass commit.
- **Disposition:** Approved and **implemented** in commit `f77205e52eaaf3e9ff0ffaee629585e865d5d7c9`.
- **Implementation:**
  - Removed broad implicit staging from the bypass acquire path. The escape hatch now commits the index exactly as explicitly prepared by the operator.
  - Added fail-closed refusal (`skip_gate_empty_index`) if the index contains no staged changes, returning clear remediation guidance (`git add <paths>`).
  - Added script-execution seam tests (`TestCommitGate_SkipPath` in `internal/cli/commit_gate_skipgate_test.go`) covering the hazard case (staged A committed while dirty B/C remain unstaged), empty-index refusal, and environment-preflight refusal.
  - Updated documentation across `AGENTS.core.md`, `committer.md`, `git-execution-routing.md`, and the `gated-commit` skill, verified via `make update`.

### (b) Root Binary Age Discrepancy (Dismissed)

- **Observation:** A date discrepancy was noted between `./vh-agent-harness` at the repository root and `bin/vh-agent-harness`.
- **Disposition:** **Dismissed** as a non-defect following operator clarification on 2026-11-02.
- **Rationale:** The presence of a root binary reflects normal release cadence rather than an unmanaged defect or broken build loop. The canonical developer build route is `make` or `go build -o bin/`, and existing dev-stale-embed guards already alert when an embedded corpus diverges from working-tree templates during update operations.

---

## 6. Open Deferred Items (Local Candidate Transport Only)

The following two follow-up items are retained as unpromoted local draft task cards in `.local/coordinator/tasks/` (ephemeral transport, not durable backlog entries):

1. **`README.agent.md` staging requirement sync:**  
   Update `README.agent.md` to document that the operator bypass (`SKIP_COMMIT_GATE=1`) requires explicit staging via `git add <paths>` prior to commit.  
   *Trigger:* `path_touched(README.agent.md)`

2. **OpenCode-context refusal integration test:**  
   Execute one non-skipped run of the OpenCode-context refusal test on a host environment with healthy preflight conditions.  
   *Trigger:* `always_before` claiming that this guard is fully verified in release auditing.

---

## 7. Limitations

- **Confidence:** Overall confidence in these conclusions is medium.
- **Absence of runtime pilots:** Multi-lane coordination and wave constructs were evaluated analytically against the mined project's logs rather than through live comparative execution pilots in this harness.
- **Dogfood preference vs. universal adopter demand:** The conclusion that owner communication represents the highest-value element reflects current dogfood priorities (single-operator workflows) rather than measured multi-user adopter telemetry.
- **Sample bounds:** The mined operational data derives from a single external project, executed on a single frontier model tier, operating without the architectural constraints of an agent harness.
