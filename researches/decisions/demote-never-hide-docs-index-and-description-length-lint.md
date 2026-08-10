---
type: decision
date: 2026-08-10
scope: agent rules — docs index demotion and skill description length lint
status: candidate-recorded (operator cross-check P2)
source-basis: refs/hermes-agent @ skills_tool.py (MAX_DESCRIPTION_LENGTH)
---

# Docs Index Demotion and Description Lint — Decision Memo

## Decision statement

**QUESTION (a):** Should the harness ship a docs index that DEMOTES stale entries but NEVER hides them (discoverable but de-prioritized)?
**QUESTION (b):** Should the harness ship a description-length lint (like hermes's ≤60-char skill description rule) applied to harness skills/commands?

This is a recommendation the operator will decide on; the memo body does not speak as live repo policy.

## Decision context

Hermes enforces a strict ≤60-character limit on skill descriptions (in `refs/hermes-agent/tools/skills_tool.py`, enforced via `MAX_DESCRIPTION_LENGTH`). This constraint ensures that the agent's context window isn't bloated by overly verbose tool or skill descriptions, keeping prompt injection crisp. 

The harness currently lacks an equivalent lint for its own skills (`.opencode/skills/`) or commands (`.opencode/commands/`, `templates/core/`).

Separately, the docs index accumulates entries over time. As project docs age, stale entries clutter the retrieval context. A "demote but never hide" paradigm would push stale docs down the index priority without removing their searchability, preserving historical access.

## Consumer field evidence (operator cross-check, studied 2026-08-08)

`source=operator-cross-check, studied:2026-08-08` — operator-provided; flagged as a P2 candidate. Both the docs index demotion and the skill description length lint were identified as potential hygiene features that consumers might need to keep context cleanly prioritized.

## This-harness posture (read by this researcher)

The harness surfaces commands and skills dynamically through its `skill list` and command registry. Because there is no length lint, a verbose skill author can consume excess tokens trivially. The existing `SKILL.md` structures do not constrain the `description:` field length. Similarly, the docs indices do not possess a concept of "staleness" or "demotion weight."

## Options considered

- **OPT-A — Adopt both features:** Ship the demote-never-hide docs index logic and enforce the description-length lint.
- **OPT-B — Adopt lint, defer demotion (RECOMMENDED):** Adopt the description-length lint now as a cheap, high-value mechanism. Defer the heavier docs index mechanism.
- **OPT-C — Defer both features:** Maintain the current posture and rely on human authors to keep descriptions brief and docs indices clean.

## Recommendation

**OPT-B.** Adopt the description-length lint (≤60-chars) for skills and commands. It is cheap, high-value, and prevents immediate token bloat. The demote-never-hide index is much heavier—defer it pending a concrete stale-entry problem that actually degrades agent retrieval.

## Findings

- **(finding)**: source=refs/hermes-agent/tools/skills_tool.py, confidence=high, type=fact — Hermes explicitly uses a ≤60-char limit for tool/skill descriptions to control context budget.
- **(inference)**: source=synthesis, confidence=high, type=inference — Implementing a description lint is structurally trivial, whereas a demote-never-hide index demands tracking mechanisms for document access, age, or relevance ("staleness").

## Contradictions

- None detected in the covered scope.

## Risks / open-questions

- Will a strict 60-character limit force current harness skills (e.g., `formal-verification`, `contract-invariant-audit`) to rewrite their currently verbose descriptions? A grandfathering exception might be required.