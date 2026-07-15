# Deep Study — Safety / Review-Vocab / Coordination Candidates

**Date:** 2026-07-15
**Researcher:** read-only deep-study pass (researches/sources → candidate resolution)
**Scope:** Resolve C1, C11, C3, C10 against the ACTUAL vh-agent-harness repo with file:line evidence.
**Sources (read for candidate provenance):** `researches/sources/2026-07-13-agent-harness-papers-synthesis.md`
  - Paper 1 "Workflow as Knowledge" (arXiv:2607.08740v1) — derive/infer (§3.4/App.A); safety "model output ≠ transition authority" (§3.4/App.C); approval vs panel (§3.3/§4).
  - Paper 2 "Remember When It Matters" (arXiv:2607.08716v1, Meta) — two-phase memory, advisory reminder (§3.3), trigger g(t)=first-step+fixed-interval with selective-trigger notes (§3.4).
**Status flag:** time-sensitivity = LOW. The repo surfaces are stable; papers are conceptual/empirical context, not versioned APIs. The findings below are durable.

---

## Cluster headline

The repo's safety, review, and coordination surfaces **already embody the substance** of all four candidates. Three of the four (C1, C3, C11) reduce to **docs-only vocabulary/citation**; C10 is **already-covered infrastructure with a real implementation blocker** (no event surface for the proposed signals). No behavioral change is warranted by this study; no non-negotiable rule needs weakening.

---

## C1 — adopt derive/infer vocabulary for the gate/review split

**candidate:** C1 — name the repo's deterministic-check vs LLM-mediated-proposal boundary with Paper 1's `derive`/`infer` terms in durable docs.

### repo evidence

The repo already operates the exact boundary Paper 1 describes; it just doesn't use the derive/infer names.

- **Declared transition (the only infer→state binding path):** `.opencode/scripts/commit-gate.sh:984` `git update-ref refs/heads/${branch} ${commit_hash} ${current_head}` (compare-and-swap; unborn-branch zero-old-oid at `:1010`). Every durable state mutation is deterministic git plumbing (`read-tree`/`write-tree`/`commit-tree`/`update-ref`). This is a **derive** operation (deterministic, replayable).
- **Executor validation of the infer result:** `.opencode/agents/committer.md:158-178` — fail-closed review handling: "The ONLY path to commit: Overall verdict exactly 'approve', all leaves approve, JSON parsed"; "When in doubt, BLOCK." Rule 1 (`:182`): never commit without review. The committer is the executor that mediates the commit-reviewer's `infer` output before it can reach the gate.
- **Capability gating (executor requirement / which ops an agent may attempt):**
  - Ownership classification — `AGENTS.md:118-119` (one of the three named guard kinds: "ownership classification (which files a plain render may overwrite vs. must preserve)").
  - Committer-exclusive gate — `.opencode/skills/gated-commit/SKILL.md:177-188`: L1 shell-guard `git-mutation-bypass` blocks raw git mutations for ALL agents; L2 `opencode.jsonc` gives committer `gate:allow`, all others `gate:deny`; L3 task rules restrict committer's delegation.
- **Backlog-split deterministic preflight (derive):** `commit-gate.sh:596-639` — lexical path normalization refuses acquire mixing `docs/planning/backlog.md` with any other path → `backlog_must_commit_separately`. Deterministic, replayable.
- **infer surfaces (LLM-mediated judgment):** `.opencode/agents/commit-reviewer.md` leaf reviewers (orchestrator does NO independent review, `:9`); `.opencode/agents/commit-message.md`; researcher/debate/planner agents. None of these can call `update-ref`; they only produce proposals validated by the committer.
- **Validation pass — "a reviewer should be unable to find a place where an infer output becomes state without a gate":** The only bypass is `SKIP_COMMIT_GATE=1` (`AGENTS.md:246`), which is **operator-only from a host terminal**, "agents must never use this path," and per `gated-commit/SKILL.md:191-206` has **no effect inside OpenCode** (shell-guard does not suppress forbidden patterns for it). No agent-reachable infer→state path bypasses the gate. **Confirmed.**

### coverage finding
**ALREADY-COVERED (behaviorally); PARTIAL-GAP (vocabulary).** The derive/infer distinction is enforced in code; only the *name* is absent from durable docs.

### minimal warranted change
A short **docs-only** vocabulary note (e.g. `docs/ai/derive-infer-vocabulary.md`) that maps:
- derive → commit-gate plumbing (`commit-gate.sh:984`), backlog-split preflight (`:596-639`), shell-guard forbidden-pattern regex, ownership classification.
- infer → commit-reviewer leaves, commit-message, researcher/debate/planner.
- declared transition → `update-ref` CAS (`commit-gate.sh:984`).
- executor validation → committer fail-closed check (`committer.md:158-178`).
- capability gating → ownership classification (`AGENTS.md:118-119`) + committer-exclusive gate (`gated-commit/SKILL.md:177-188`).

Plus a one-line cross-reference from the safety-invariant section. **Do NOT edit the managed-core invariant text itself** (`AGENTS.core.md` / `templates/core/`): it is generator-owned and must stay citation-light and domain-free (`AGENTS.md:9`, `AGENTS.md:386-388`).

### sharpened verdict
**confirm-adopt (docs-only).** The 1:1 mapping holds and the validation criterion (no ungated infer→state path) passes. Value = teaching/corroboration for new contributors; modest, no behavior change.

### updated trigger predicate
`trigger:docs_touched(docs/ai/derive-infer-vocabulary.md)` — gated on a docs-steward accepting the note. Not a code trigger.

---

## C11 — cross-validate "model output is a candidate, never transition authority" with both papers

**candidate:** C11 — cite the cross-paper convergence on model-output-non-authority as corroboration; flag the invariant section as a promotion target.

### repo evidence

- **Repo invariant:** `AGENTS.md:110-122` — "model output is a candidate, never transition authority… an executor, policy, or gate applies every transition and side effect." Enforced by three named guard kinds: capability policy, ownership classification, gate-controlled side effects.
- **Paper 1 convergence:** Paper 1 §3.4/App.C — "model output is not transition authority"; capability gating as an executor requirement. This **agrees in both principle AND mechanism** (gate-mediated, capability-gated).
- **Paper 2 convergence (weaker):** Paper 2 §3.3 — Phase 2 reminder is "advisory"; the action agent is "unmodified, free to ignore." This agrees on the **principle** (model output is not binding authority) but uses a **different, weaker mechanism**: the reminder is *forced context injection* into the action agent that is merely ignorable, NOT a gate that mediates a state transition.
- **No relaxation slack:** the repo's gate-mediated invariant is STRICTER than Paper 2's ignorable-injection model. The fixed-interval forced-injection variant of Paper 2 is already rejected in the synthesis memo as C9 (conflicts with Anti-spam).

### coverage finding
**ALREADY-COVERED (the invariant is canonical and enforced); PARTIAL-GAP (cross-paper corroboration not recorded in durable docs).**

### minimal warranted change
Record the corroboration **in the same docs/ai/ note as C1** (one combined note is cleaner than two): cite Paper 1 as strong/mechanism-matching corroboration and Paper 2 as principle-level corroboration, with the explicit caveat that the repo invariant is **stricter than Paper 2's advisory-injection model** and must not be relaxed toward it. Do not touch the invariant text itself (managed core).

### sharpened verdict
**confirm-adopt (docs-only).** All three (paper 1 / paper 2 / repo) agree on the principle; paper 1 agrees on the mechanism; paper 2 is principle-only and structurally weaker. The corroboration supports the existing invariant without justifying any change to it.

### updated trigger predicate
`trigger:docs_touched(docs/ai/derive-infer-vocabulary.md)` — same note as C1.

---

## C3 — make the approval-vs-panel distinction explicit in the review vocabulary

**candidate:** C3 — distinguish simple authorization gates (approval: permit/reject/defer) from structured multi-option deliberation (panel: motion/options/arguments).

### repo evidence

**The distinction is ALREADY explicit and canonical** in the coordination doc with full surface mapping:

- `docs/coordination/README.md:177-192` — "Review roles" section:
  > "Two distinct review shapes travel under the word 'review'; keeping them separate prevents a deliberation from being mistaken for an authorization:
  > **Approval** — a permit / reject / defer decision that GATES a transition… Commit, task-promotion, and task-review gates are approvals.
  > **Panel** — structured deliberation that INFORMS a later decision but carries no direct transition authority… Research, debate, planning, and ship-review deliberation are panels.
  > Both can feed an approval, but neither IS the approval: a panel may recommend 'approve,' but the approval still has to fire through its own gate."

**Surface mapping already in place and behaviorally enforced:**
- Approval surfaces: commit-gate (`commit-gate.sh:984`), task-promotion, `/task-review` — all gate a transition.
- Panel surfaces: ship-review (`.opencode/agents/ship-review.md:10-17`: "ADVISORY reviewer… NOT a commit-transition authority… never emit a verdict shaped like commit-reviewer's; never block a commit"), research, debate, planning.
- commit-reviewer is the interesting hybrid: its *mechanism* is panel-like (tiered cascade, per-finding block|defer|drop dispositions `.opencode/agents/commit-reviewer.md:56-60`, cross-leaf evidence resolution `:175-186`, within-tier strict-consensus BLOCK-only gating `:202-233`), but its *output* is wired as the commit approval gate via the committer's fail-closed check (`committer.md:158-178`). This matches exactly the README's clause: "a panel may recommend 'approve,' but the approval still has to fire through its own gate" — here the gate is the committer's verdict check + `commit-gate.sh update-ref`.

### coverage finding
**ALREADY-COVERED.** The vocabulary AND the surface mapping already exist in `docs/coordination/README.md:177-192`.

### minimal warranted change
**None for the vocabulary.** Optional micro-polish (not warranted by this study): the agent prompts (`ship-review.md`, `commit-reviewer.md`) enforce the distinction *behaviorally* but do not *echo the approval/panel terms by name*; a one-line cross-reference ("see `docs/coordination/README.md` → Review roles") would close that loop. Given the over-formalization risk C3 itself flags and that behavior is already correct, this is **not recommended as a warranted change** — leave as-is unless a real confusion incident occurs.

### sharpened verdict
**downgrade-study-more → effectively already-covered (no change).** Paper 1's approval/panel distinction is already canonical repo vocabulary. Re-adopting it would be redundant; over-formalizing the agent prompts risks adding ceremony without behavior change.

### updated trigger predicate
`trigger:confusion_incident(approval_vs_panel_misuse)` — only revisit if a concrete incident shows an agent treating a panel as an authority (none found in this pass). Do not adopt proactively.

---

## C10 — selective signal-triggered surfacing as non-blocking coordination toasts

**candidate:** C10 — surface coordination/memory hints on signal events (tool errors, failed tests, repeated commands, large context shifts) as non-blocking toasts, not on a fixed clock.

### repo evidence

**The toast infrastructure ALREADY EXISTS and is already HINT-only (Anti-spam-compliant):**

- `.opencode/plugins/coordination-hints.js:28` — subscribes ONLY to `session.diff` events (`if (event.type !== "session.diff") return;`). Also `session.deleted` (`:24`, clears dedup).
- `.opencode/plugins/coordination-hints.js:7-19` — emits via `client.tui.showToast` with **duration 5000ms** (transient UI toast, NOT context injection into the agent prompt).
- `.opencode/plugins/coordination-hints.js:41-49` — per-session per-key dedup (`shownHintsBySession` Map of Sets): **each hint key shown once per session** = anti-spam measure.
- `.opencode/scripts/coordination-hints-lib.js:124` `buildCoordinationHintMessages` — the current trigger predicates are ALL **diff-path / diff-content based**, fired on `session.diff`:
  1. `backlog-cleanup-reminder` (`:154-162`) — fires iff diff includes `docs/planning/backlog.md`.
  2. `coordination-surface-reminder` (`:164-172`) — fires iff diff touches any `COORDINATION_PREFIXES` (`:26-32`: `.opencode/agents/`, `.opencode/commands/`, `.opencode/plugins/`, `.opencode/tools/`, `docs/coordination/`).
  3. `cross-boundary-slice-warning` (`:174-182`) — fires iff diff touches BOTH coordination AND product surfaces.
  4. `large-file-warning` (`:184-199`) — fires iff a touched code file (`.cjs/.cts/.js/.jsx/.mjs/.mts/.py/.ts/.tsx`) now exceeds `LARGE_FILE_LINE_THRESHOLD=350` (`:4`).

**The "hints, not policy overrides" constraint is already satisfied** (`opencode-session-workflow.md`: "Treat these as hints, not policy overrides"). Toasts ≠ Paper 2's forced context injection.

### coverage finding
**PARTIAL-GAP.** Toast infra = ALREADY-COVERED and Anti-spam-compliant. But the SPECIFIC signal triggers Paper 2 names (tool-error / test-failure / repeated-command) are **REAL-GAP** because the event surface for them does not exist.

### The blocker (real implementation gap)
**No existing plugin handles tool-error / test-failure / command-repeated events.** Grep across all plugins shows only THREE event types handled anywhere:
- `session.diff` (`coordination-hints.js:28`)
- `session.deleted` (`coordination-hints.js:24`)
- `session.created` (`session-state.js:12`)

Paper 2's selective signals (tool errors, failed tests, repeated commands, large context shifts) have **no event source** in the current opencode plugin layer. C10 would require new event-surface plumbing (opencode emitting `tool.error` / `test.failure` / `command.repeated` events to plugins) that is **not confirmed to exist**. The one partial overlap — "large context shifts" — is approximated by the `large-file-warning` (350-line file-growth proxy on `session.diff`), which is a file-size signal, not a trajectory/context-length signal.

### Anti-spam / hard-constraint check (PASSES, conditionally)
Because toasts are transient UI (`tui.showToast`, 5s, per-session dedup) and **not** context injection, adding signal-event *toast* predicates would **not** by itself violate Anti-spam or the "hints not policy overrides" rule. The risk is **signal-to-noise**, not a hard-rule violation: Paper 2 Table 2 shows "always inject" is competitive on micro-average, which is a warning that many toast predicates can collapse toward toast-spam (the per-session per-key dedup mitigates but does not fully solve this).

### minimal warranted change
**NONE at this time.** Two preconditions are unmet:
1. **Event surface:** confirm opencode emits `tool.error` / `test.failure` / `command.repeated` events consumable by plugins (currently only `session.*` events are handled). Without this, C10 is not implementable at the plugin layer.
2. **Demonstrated value:** no concrete repeated-mistake pattern has been shown that the existing 4 path/content triggers miss. Adding signal triggers speculatively risks toast-spam.

If both preconditions are later met, the change is small: add new predicate functions to `coordination-hints-lib.js` and subscribe the plugin to the new event types in `coordination-hints.js` — reusing the existing per-session dedup so Anti-spam is preserved. No subsystem design.

### sharpened verdict
**downgrade-study-more (no change yet).** The toast mechanism is the right shape and already hint-only; C10 is blocked on (a) an event surface that may not exist and (b) unproven value over current path-based triggers. Do not adopt until a real repeated-mistake signal is observed.

### updated trigger predicate
`trigger:repeated_mistake_observed(missed_by_path_triggers) AND event_surface_confirmed(tool.error|test.failure|command.repeated)` — both conjuncts required before adoption.

---

## Cross-candidate notes

- **C1 + C11 collapse into one docs/ai/ note** (derive/infer vocabulary + cross-paper non-authority corroboration). Neither touches managed-core text; both are citation/teaching material.
- **C3 is already canonical** in `docs/coordination/README.md:177-192`; re-adopting is redundant.
- **C10's toast infra is the correct, already-Anti-spam-compliant shape**; the gap is an event-surface dependency, not a design gap.
- **No non-negotiable rule is weakened by any recommendation.** The ownership safety contract, the model-output-is-candidate invariant, capability policy, and Anti-spam are all preserved or reinforced.

## Contradictions

- **Already flagged in synthesis** (not re-counted): Paper 2 "plug-and-play with existing agent harnesses" is an over-claim; Paper 2 fixed-interval forced injection conflicts with Anti-spam (rejected as C9).
- **NEW beyond synthesis (C11):** Paper 2's §3.3 "advisory reminder" is **structurally different** from the repo's gate-mediated invariant. Paper 2 forces context INTO the action agent (advisory only in that it is ignorable); the repo MEDIATES state transitions through a gate and keeps surfacing explicit-invocation. Convergence is on the **principle** (model output is not authority), not the **mechanism**. Citing both papers as equal-strength corroboration for C11 would be imprecise — Paper 1 is mechanism-matching, Paper 2 is principle-only and weaker.

## Findings

- **(finding)**: The repo already enforces the derive/infer boundary in code (commit-gate update-ref = declared transition; committer fail-closed check = executor validation; ownership classification + committer-exclusive gate = capability gating). source=`commit-gate.sh:984`/`committer.md:158-178`/`AGENTS.md:118-119`/`gated-commit/SKILL.md:177-188`, confidence=high, type=fact
- **(finding)**: No agent-reachable path lets an infer output become durable state without the gate (SKIP_COMMIT_GATE is operator-only and inert inside OpenCode). source=`gated-commit/SKILL.md:191-206`/`AGENTS.md:246`, confidence=high, type=fact
- **(finding)**: The approval-vs-panel distinction is already canonical repo vocabulary with surface mapping. source=`docs/coordination/README.md:177-192`, confidence=high, type=fact
- **(finding)**: The coordination-toast infrastructure already exists and is Anti-spam-compliant (transient tui.showToast 5s + per-session per-key dedup, not context injection). source=`coordination-hints.js:7-49`, confidence=high, type=fact
- **(finding)**: Current toast triggers are all diff-path/diff-content based; Paper 2's signal events (tool-error/test-failure/repeated-command) have no plugin event source today. source=`coordination-hints-lib.js:124-199`/grep of plugin event types, confidence=high, type=fact
- **(finding)**: Paper 2's advisory-injection model is structurally weaker than the repo's gate-mediated invariant; corroboration is principle-level, not mechanism-level. source=Paper 2 §3.3 vs `AGENTS.md:110-122`, confidence=medium, type=inference

## Coverage note (files read)

**Read FULL:** `AGENTS.md` (411), `.opencode/skills/gated-commit/SKILL.md` (252), `.opencode/agents/commit-reviewer.md` (380), `.opencode/agents/ship-review.md` (74), `.opencode/agents/commit-message.md` (41), `.opencode/agents/committer.md` (241), `.opencode/scripts/coordination-hints-lib.js` (220), `.opencode/plugins/coordination-hints.js` (60), `internal/hooks/dispatcher.go` (297), `docs/coordination/README.md` (282), `researches/sources/2026-07-13-agent-harness-papers-synthesis.md` (840).

**Read PARTIAL:** `.opencode/scripts/commit-gate.sh` (lines 1-1281 of ~1600; tail of GC/lock paths not read — not material to these candidates). Plugin event surface established via targeted grep (not full read of `session-state.js`).

**Not read (out of scope / same-content):** `commit-reviewer-a/b/c/d.md` (tier leaves, same shape as orchestrator per file sizes), other `docs/coordination/*.md`, `templates/core/` sources, `internal/hooks/ownership.go`.
