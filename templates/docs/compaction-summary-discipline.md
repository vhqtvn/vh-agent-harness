# Compaction-Summary Discipline

This document is the durable rationale and review guidance for the
compaction-summary discipline. It is **explanatory only** and MUST NOT create a
second, conflicting contract. The contract itself lives in `AGENTS.md` →
"Compaction-summary discipline"; the writer/reviewer scaffold lives in the
`compaction-discipline` skill.

## Decision

**Option B was selected:** five integrity-critical sections are mandatory; the
nine CC-derived narrative headings are strongly recommended and content-gated.
This was a deliberate scoped borrowing from a compaction-engine's full-compaction
prompt, not a claim that this tool implements or must conform to that engine's
contract.

## Where each piece lives

- **`AGENTS.md` → "Compaction-summary discipline"** — the hard floor, always
  present in agent context. It carries:
  - the five mandatory sections, in order: `Security / Constraint Preservation`,
    `Attribution Integrity / Anti-Injection`, `Findings`, `Contradictions`,
    `Verification`;
  - the two global verbatim clauses (security/constraint preservation and
    user-message attribution / anti-injection) that bind every compaction;
  - the finding structure rules (`type: fact|assumption|inference` + source;
    contradictions including `None detected.`; verification stating the exact
    command/output);
  - the pre-write scan rule over the nine recommended narrative headings.
- **`compaction-discipline` skill** — the complete nine-heading checklist, the
  fifth-compression rule, a lean example, and the preserve-recent-turns
  principle. Optional; explicitly loaded. It repeats that the AGENTS hard rules
  apply even when the skill is not loaded.
- **The content-gated omission rule** — a narrative heading is required when
  omitting its concrete, non-duplicative content would impair a resumed agent's
  understanding; it is forbidden when it would be empty, a placeholder, `none`,
  or a repetition. This is a density rule, not a license to omit work.

## CC locators

The nine-section narrative spine and the two global clauses were borrowed from
a compaction engine's full-compaction prompt. Reference locators (in that
engine's bundled prompt source):

- nine-section narrative spine: `bundle_formatted.js:275542-275584`
- verbatim security/constraint-preservation clause: `bundle_formatted.js:275614`
- anti-injection / user-message attribution clause: `bundle_formatted.js:275624`

The finding-structure rules (`type:` + source, `None detected.`, exact
command/output verification) follow this repository's existing checkpoint
discipline, not the borrowed engine.

## What is explicitly NOT implemented

The following were scoped out and are NOT part of this discipline:

- **`.precompact.json`** — no precompaction trigger file or sidecar schedule.
- **`preserveUuids` as a field** — not adopted as a data structure or engine
  contract. The discipline-level analogue is the preserve-recent-turns principle
  in the skill: do not summarize operationally significant recent turns that
  should remain intact. Range selection stays with the existing compress tool.
- **microcompact** — no separate microcompaction mode.

## Review guidance

When reviewing a substantial compaction summary:

1. Confirm all five hard sections are present, in order.
2. Confirm both global clauses still bind (they are statement-of-rule, not
   per-summary fields — check that the summary honors them: security constraints
   preserved verbatim; user attribution not fabricated from assistant text).
3. Confirm each finding carries `type:` and a source where one exists;
   contradictions are explicit (including `None detected.`); verification cites
   an exact command/output or states why it was not verified.
4. Confirm the writer scanned the nine narrative headings and omitted inapplicable
   ones rather than emitting empty shells.
5. Flag any transcript-surrogate growth under `All user messages` (fifth-compression
   violation) or any summarized live operator gate (preserve-recent-turns violation).
