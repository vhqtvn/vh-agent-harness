---
type: decision
date: 2026-08-10
scope: skills — external-daemon-authority skills
status: candidate-recorded (operator cross-check P2)
source-basis: vh-solara skill install pattern
---

# External-Daemon-Authority Skills — Decision Memo

## Decision statement

**QUESTION:** Should the harness support/govern a class of "external-daemon-authority" skills — skills that install and manage an external daemon/process, version-pinned with drift detection?

This is a recommendation the operator will decide on; the memo body does not speak as live repo policy.

## Decision context

The harness currently provides `bgshell-job` to manage long-running local shell jobs. However, `bgshell-job` does not govern persistent daemon installations, version pinning, or binary drift detection. 

Two consumers (including vh-solara) have independently invented a `skill install` pattern for external daemons (version-pinned with drift checks) to fill this gap. The operator cross-check flagged this pattern as a missing primitive that consumers are independently building.

## Consumer field evidence (operator cross-check, studied 2026-08-08)

`source=operator-cross-check, studied:2026-08-08` — operator-provided; flagged as a P2 candidate. The vh-solara `skill install` pattern is utilized in 2 of the 3 consumers, functioning as an external-daemon authority. The third consumer (Consumer B) has not yet fully adopted this explicit pinning pattern for daemons.

## This-harness posture (read by this researcher)

The harness currently treats skills as workflow guidance and execution primitives (`.opencode/skills/`, `templates/core/.opencode/skills/`). It includes a skill-authoring proposal intake (as documented in `AGENTS.md`). While `bgshell-job` is the closest existing primitive to process management, it is explicitly bounded to running shell jobs and lacks the semantics of daemon installation or version drift tracking.

## Options considered

- **OPT-A — Formalize external-daemon-authority skills now:** Create a dedicated skill class and primitive for daemon installation, version-pinning, and drift detection.
- **OPT-B — Defer until broader convergence (RECOMMENDED):** Rely on the existing skill-authoring proposal intake to capture consumer workflows, and wait for all consumers to converge on a unified pattern before formalizing.

## Recommendation

**OPT-B.** The harness already has a mature skill-authoring proposal intake and `bgshell-job`. A dedicated external-daemon-authority skill class is a valid candidate feature, but we should wait until ≥2 consumers completely converge on a common install/pin/drift pattern before absorbing it into the core (applying the advisory-tier inventory's convergence lesson).

## Findings

- **(finding)**: source=operator-cross-check, confidence=high, type=fact — The `skill install` pattern is utilized in 2 consumers for managing pinned external daemons.
- **(finding)**: source=.opencode/skills/bgshell-job/, confidence=high, type=fact — The existing `bgshell-job` skill is strictly scoped to managing running shell tasks, not persistent installation or drift-checking of daemon binaries.

## Contradictions

- None detected in the covered scope.

## Risks / open-questions

- If we defer, will the two consumers diverge further in their `skill install` implementations, making a future harness-level unified primitive harder to adopt and migrate?