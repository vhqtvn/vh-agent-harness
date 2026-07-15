# derive/infer vocabulary and the model-output-is-candidate invariant

This note names a boundary the harness already enforces in code but does not
call by name in durable docs: the split between **deterministic (`derive`)**
operations and **LLM-mediated (`infer`)** proposals, and how an `infer` result
becomes durable state only through a gate.

It is teaching and corroboration material, not a new mechanism — the
enforcement surfaces below already implement the contract. Naming the boundary
makes it findable for new contributors and records where two external papers
agree (and where one is weaker) with the repo's
**"model output is a candidate, never transition authority"** invariant
(`AGENTS.md` → Safety invariant).

## derive vs infer

- **`derive`** — deterministic and replayable: the same inputs always produce
  the same result, with no judgment involved. In this repo every durable state
  mutation is deterministic git plumbing. The canonical example is
  `templates/core/.opencode/scripts/commit-gate.sh` performing a
  compare-and-swap `git update-ref refs/heads/<branch> <new> <old>` (the single
  declared transition that moves a branch tip). The backlog-split preflight
  (lexical normalization that refuses mixing `docs/planning/backlog.md` with
  any other path into one commit) is likewise a `derive` check.
- **`infer`** — LLM-mediated judgment whose output is a *candidate*, not
  authority. The commit-reviewer leaves, the commit-message drafter, and the
  researcher / debate / planner agents all emit `infer` output. None of them
  can call `update-ref`; they only produce proposals.

## How an infer result reaches state (the 1:1 enforcement mapping)

1. **Declared transition (infer → state).** The only binding path is the
   commit-gate's `git update-ref` CAS — a `derive` operation. An `infer`
   verdict does not itself move a ref; the gate does.
2. **Executor validation of the infer result.** The committer
   (`templates/core/.opencode/agents/committer.md` → "Fail-closed review
   handling") mediates the commit-reviewer's output before it can reach the
   gate: it commits only on an exact `approve` verdict with all leaves
   approving and the JSON parsed; otherwise it blocks ("When in doubt, BLOCK").
3. **Capability gating (which agent may even attempt the op).** Ownership
   classification (the "plain render may overwrite vs. must preserve" contract)
   plus the committer-exclusive gate
   (`templates/core/.opencode/skills/gated-commit/SKILL.md` → Enforcement
   layers): shell-guard's `git-mutation-bypass` blocks raw git mutations for
   every agent, while the generated `opencode.jsonc` grants only the committer
   `gate: "allow"`.

## The escape hatch is operator-only

The single bypass is `SKIP_COMMIT_GATE=1`. It is operator-only, run from a host
terminal, and is **inert inside OpenCode** — shell-guard does not suppress its
forbidden patterns for any agent. No agent-reachable `infer` output can become
state without the gate, so the validation criterion — "a reviewer should be
unable to find a place where an `infer` output becomes state ungated" — holds.

## Cross-paper corroboration (with a strength caveat)

- **Paper 1 — "Workflow as Knowledge" (arXiv:2607.08740)** agrees in both
  principle *and* mechanism: model output is not transition authority, and
  capability gating is an executor requirement. This is the
  mechanism-matching corroboration and is the source of the `derive`/`infer`
  terms used above.
- **Paper 2 — "Remember When It Matters: A Proactive Memory Agent"
  (arXiv:2607.08716)** agrees only at the principle level: its §3.3 reminder is
  "advisory" and the action agent is "free to ignore" it. That is
  **structurally weaker** than this repo's invariant — Paper 2 forces context
  *into* the action agent (ignorable), whereas this repo *mediates* a state
  transition through a gate and keeps surfacing explicit-invocation.

Do not cite the two papers as equal-strength corroboration. The repo's
gate-mediated invariant is **stricter than Paper 2's advisory-injection model**
and must not be relaxed toward it; Paper 2's fixed-interval forced-injection
variant is already rejected here as conflicting with the Anti-spam rule (see
`vh-agent-harness docs opencode-memory-model`).

## Provenance

- Source memos: `researches/sources/2026-07-15-candidates-deep-study-safety-vocab.md`
  (candidates C1, C11) and the synthesis
  `researches/sources/2026-07-13-agent-harness-papers-synthesis.md`.
- Underlying papers: arXiv:2607.08740 ("Workflow as Knowledge") and
  arXiv:2607.08716 ("Proactive Memory Agent").
