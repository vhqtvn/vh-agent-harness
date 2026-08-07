# Source: Advisory Verb Consumer Audit

**Date:** 2026-08-07

This packet records the evidence that closed the "should we build a stateless advisory verb (`vh-agent-harness step`/`advise`)?" question. It was produced by a read-only `researcher` consumer audit.

## Verdict
**NO CONSUMER.** No STRONG or MEDIUM candidate. Gate 1 ("Distinct value") of the prior O1′ debate cannot flip. The proposed stateless advisory verb has no named current consumer in the dogfood corpus.

## Decisive finding
The proposal re-litigated an already-decided question. The signed-off `researches/decisions/2026-08-04-capability-discovery-audit.md` adjudicated the adjacent, more accurate question — "why do agents fail to *discover* capabilities?" — and its §9 explicitly **rejected** side-channel query/toast/advisory mechanisms in favor of binding information to the friction surface. None of its 9 ranked fixes proposes a query verb.

## Candidate consumers examined (all WEAK)
| Candidate | Named consumer | Why weak |
|---|---|---|
| 1. premise-recheck protocol | `.opencode/commands/handoff-save.md`, `resume-task.md`; protocol in `.vh-agent-harness/docs/opencode-session-workflow.md` | Actual re-derivation commands in real handoffs (`.opencode/state/sessions/*`) re-derive **upstream/git facts** (opencode version, `git merge-base --is-ancestor`), NOT binary-authoritative facts. When binary-proximate facts are re-derived, commands are already cheap (`grep`, `doctor`, `ls`, `test -f`). |
| 2. coordination/specialist discovering grants | `.opencode/agents/coordination.md`, `project-coordinator.md` | Grants are baked into agent prompts at render (projection); a runtime query re-states the prompt and goes stale in lockstep. Real failure is *discovery*, signed-off remedy is grant-to-prompt parity (render layer), not a verb. |
| 3. worker-read-only / resolve-first | `.opencode/agents/worker-read-only.md`, `.opencode/skills/resolve-first/SKILL.md` | Both operate on repo/git facts the binary does not own authoritatively; re-derive nothing binary-authoritative. |
| 4. build F3 ownership-hazards envelope | `.opencode/agents/build.md` | Authors *design* ownership (intent), deliberately distinct from binary runtime classification; gate re-derives class independently. `diff --dry-run` already projects per-path action. |
| 5. agents hitting permission denial | exec-ro/exec-sandbox DENY | Exactly the surface Fix 1 (surface-at-friction) targets; a pre-probe query verb would undermine the deny-and-explain / never-auto-retry discipline. Counter-indicated. |

## Authority-gap inventory (every binary-authoritative fact is already served)
| Gap | Already served by |
|---|---|
| Capability grants | baked into agent prompts at render (projection) |
| Profile/pack selection | `overlay list` (source∪selected table) + `guide` unselected-pack hint |
| Ownership classification | `diff --dry-run` per-path action (seed/preserve/reconcile/overwrite) |
| Permission edges | emitted to `opencode.jsonc` at render; hit at friction via exec-ro DENY ladder |
| Compatibility/floor | `doctor exec-sandbox-floor` advisory check (landed) |

Pattern: every binary-authoritative fact is (a) render-projected, (b) already queryable via an existing command, or (c) intentionally surfaced only at friction by design. No category is *computed, not projected, not queryable, and needed at runtime*.

## Cheaper no-binary-change path
For every candidate, the correct mechanism is a template/render/message change, not a Go verb: grant discovery → Fix 2 grant-to-prompt parity (render); denial disambiguation → Fix 1 surface-at-friction (template/message); restart-required drift → Fix 3 `doctor` diagnostic; post-compaction capability re-derivation → Fix 4 re-state grants in compaction summaries.

## Contradictions
- Hypothesis ("agents infer binary facts from docs and get them wrong") vs evidence (prior audit E04–E06: real failure is agents *not knowing* a grant exists = discovery/parity problem; signed-off remedy is render-layer, explicitly not a runtime query). No documented instance of an agent querying for and getting wrong a binary-authoritative fact.
- `doctor` "advisory" terminology already in use for whole-repo *health* checks (`exec-sandbox-floor`, `complexity-advisory`), not a per-request advisory API — a per-invocation advisory verb would be a distinct, unconsumed shape.
- `overlay list` already answers the profile-selection question the hypothesis listed as open.

## Provenance (files the researcher read)
`.opencode/agents/{worker-read-only,coordination,project-coordinator,build}.md`; `.opencode/skills/resolve-first/SKILL.md`; `.opencode/commands/{handoff-save,resume-task}.md`; `.opencode/state/sessions/closure-kernel-owned-scope/.../2026-07-23...final.md`; `internal/cli/{root,exec_ro,status,guide,overlay_list,doctor}.go`; `internal/permconfig/model.go`; `internal/ownership/resolve.go`; `internal/runtime/capability.go`; `researches/decisions/2026-08-04-capability-discovery-audit.md`.

## Disposition of the persistent-worker (O6) hypothesis
O6 (a persistent Go step-worker/advisor) stays evidence-gated. **If reconstruction-cost evidence never materializes, the honest end state is to kill O6, not keep it rhetorically parked.** This audit produced no such evidence; the prime-agent borrow that survives is the *pattern* (model output is a candidate, never transition authority), already enforced by existing gates — not a port of the persistent kernel.

See `researches/decisions/2026-08-04-capability-discovery-audit.md` and the prior prime-agent study / solution-brief chain for context.

## Disposition / fallback
> **Audited fallback:** any future advisory need starts as an extension to an existing command (e.g. a proposed `exec-ro --explain` or `doctor --why <path>` flag — neither exists today), never a new protocol-versioned verb.
