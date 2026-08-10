---
type: decision
date: 2026-08-08
scope: shell-guard — codify the evaluator ORDERING invariant + defer test verb; RETAIN pattern backstop but REJECT pattern-accretion
status: research-complete, decision-recorded (operator-pre-classified P1 codify guard-order + defer test verb; REJECT pattern-accretion)
source-basis: refs/hermes-agent @ 005421d888a40865cc61d143ff77efd87a037a1e (gitignored transport), cross-checked
---

# Guard-Order Invariant & Read-Only Test Verb — Decision Memo

## Decision statement

Two coupled decisions:

1. **CODIFY** the harness's actual 7-phase guard-**ORDERING** invariant explicitly, **pinned by property tests**: empty-command deny → structural shell-file-authoring denial → raw forbidden-pattern match → inert rg/grep AST proof → wrapper/gate/mutation checks → broad wrapper allow → parse/allowlist logic. The unbypassable floors MUST evaluate before any bypass path.
2. **DEFER** hermes's read-only dry-run **test verb** (`hermes approvals test`). The existing live bridge tests and `eval.js` subprocess contract already provide adequate testing surfaces, and there are no evidenced operator incidents requiring a CLI verb today.
3. **RETAIN the bounded regex backstop but REJECT hermes's pattern-accretion approach** (`_normalize_command_for_detection` regex substitution + `_command_detection_variants` bounded enumeration + the `hermes-verify-*` exemption carve-out). The harness uses structural proofs where possible; regressing to a regex/normalization arms race reintroduces model-guessable holes.
4. **Record** hermes's fail-open anti-patterns as things this harness MUST NOT replicate — they contradict the harness's RF-B fail-closed contract.
5. **Record NO GAP TODAY** for a "deny before YOLO" middle tier, as the harness has zero YOLO/auto-approve modes.

This is a recommendation the operator will decide on; the memo body does not speak as live repo policy.

## Refinement history

- This memo was corrected after (a) a `debate` pass (inline specialist pass) and (b) a read-only reachability audit (durable copy: `tmp/agent-runs/audit/shell-guard-reachability-audit.md`).
- The original direction held; the original SPECIFICS were wrong in these ways:
  - Wrong citation: `drift.go` was cited for the evaluator structure, but the evaluator spans the Go hook + JS phases.
  - Wrong ordering shape: proposed copying hermes's 4-step ladder instead of pinning the harness's actual 7-phase evaluator.
  - Over-broad "REJECT pattern-accretion" mistakenly rejected the harness's own legitimate regex backstop.
  - Premature test-verb recommendation when the existing `eval.js` contract and live tests suffice.
  - Premature operator-deny tier concern when the harness has no YOLO modes to guard against.
  - Over-claimed reachability breadth, corrected to accurately describe the intentionally heterogeneous layered floors.

## Why this is P1 (decision context)

Shell-guard is a safety gate: it decides whether a candidate command may run. Two properties make it trustworthy: (a) the **order** in which its evaluators fire — an unbypassable floor must run before any bypass, or a "never allow this" rule is silently void; and (b) the ability to **inspect** what the gate would decide without running the command. The harness has the structural gate but has not codified the ordering invariant. The ordering is currently implicit in call-site sequencing, which is exactly the shape that silently breaks when a new evaluator is inserted. P1 because a mis-ordered gate is a silent safety hole, not a loud one.

## Hermes finding (verified mechanism)

### The ordering invariant
`source=refs/hermes-agent/tools/approval.py` — `check_all_command_guards` ordering (`:3752-3790`):
1. `_should_skip_container_guards` (`:3754`) — isolated container fast-path.
2. `detect_hardline_command` (`:3761`, floor) — "Applies BEFORE yolo / mode=off / cron approve-mode so no session-level setting can bypass it" (`:3757-3760`).
3. `_check_sudo_stdin_guard` (`:3771`) — unconditional, before yolo.
4. `_match_user_deny_rule` (`:3780`) — "fire BEFORE the yolo / mode=off bypass" (`:3777-3779`).
5. yolo / `approvals.mode: off` bypass (`:3789`).
6. permanent `command_allowlist` (`:3792`).
7. `detect_dangerous_command` (`:3867+`) → ask-approval (the bypassable layer).

[cross-check CONFIRMED for the ordering]

### The read-only dry-run test verb
`source=refs/hermes-agent/hermes_cli/approvals_test.py:1-178`. Answers what the approval system would do without running it. It composes the real evaluators in the same order. Prints the deciding rule. Exit codes: 0=allow, 1=usage, 2=ask, 3=deny.
[cross-check CONFIRMED]

### REJECT — pattern accretion (do NOT replicate)
`source=refs/hermes-agent/tools/approval.py:1003-1061` (`_normalize_command_for_detection`) — regex substitution: strip ANSI, collapse `${IFS}`, etc. Plus `_command_detection_variants` bounded enumeration. Plus the `hermes-verify-*` exemption carve-out — a whitelist-shaped escape hatch. [cross-check CONFIRMED]
This is an accreting arms race. The hermes escalation path to avoid: broad textual detector → add normalization → add de-obfuscated spelling → add bounded variants → add workflow/name exemption → create another bypass seam to defend.

### Anti-patterns to record (do NOT replicate — contradict RF-B fail-closed)
- **Container-skip evaluated ABOVE the hardline floor.** `source=refs/hermes-agent/tools/approval.py:3752-3755` — `_should_skip_container_guards` returns `{"approved": True}` BEFORE the hardline floor. [cross-check CONFIRMED]
- **Fail-open return for non-interactive sessions.** `source=refs/hermes-agent/tools/approval.py:3865` — the non-interactive branch ends in `return {"approved": True}`. [cross-check QUALIFIED] — the hardline floor, sudo guard, and user-deny DO scan first. It is a fail-open of the ask-approval layer. Still contradicts RF-B fail-closed.

## This-harness posture (read by this researcher)

The harness's gate spans Go (hook/bridge, fail-closed) + JS (evaluator phases). It uses a **structural** (bash-grammar AST) parser alongside a regex backstop.

- **The gate spans Go + JS.** `source=templates/core/.opencode/plugins/shell-guard-core.js:1204-1427`, confidence=high, type=fact — this is the live evaluator. `source=internal/permission/shellguard_hook.go:241-277`, confidence=high, type=fact — this is the fail-closed Go hook/bridge. The decision is made on a parsed bash AST via `web-tree-sitter` (when available), structurally stronger than hermes's regex normalization.
- **The evaluator phases.** `source=templates/core/.opencode/plugins/shell-guard-core.js:1204-1427`, confidence=high, type=fact. The actual 7-phase evaluator is:
  1. empty-command deny
  2. structural shell-file-authoring denial
  3. raw forbidden-pattern match
  4. tightly-constrained AST proof that an otherwise-forbidden match is an inert `rg`/`grep` search operand
  5. special wrapper/gate/mutation checks
  6. a broad `vh-agent-harness ...` wrapper allow path after its narrow checks
  7. parse + read-only allowlist logic for non-wrapper commands.
- **The bridge is fail-closed (RF-B).** `source=internal/permission/shellguard_hook.go:241-277`, confidence=high, type=fact — every fault path returns `Deny`: bridge error (`:242`), runner err (`:251`), non-zero exit (`:254`), empty stdout (`:258`), malformed JSON (`:265`), unknown action (`:275`).
- **No YOLO gap today.** `source=internal/`, confidence=medium, type=fact — grep found zero `YOLO`/`autonomous`/`auto-approve`/`always_allow` mode in `internal/`. Hermes's "deny before YOLO" middle tier solves a problem this harness doesn't have.
- **Reachability (layered floors).** `source=tmp/agent-runs/audit/shell-guard-reachability-audit.md`, confidence=high, type=fact — the harness relies on heterogeneous layered floors, NOT unavoidable reachability on every path. Shell-guard (JS) gates `exec` and `shell`. `exec-ro` carries its own `execro.Classify` default-deny floor. `exec-sandbox` uses Landlock+seccomp or `execro.ClassifyArgs`. The SOLE floor-less path is `exec-sandbox ModeOff` → `runDirect`, which is operator-gated (FIX-1 guard `internal/cli/exec_sandbox.go:239-241` refuses absent `min_mode: off`).
- **Anti-pattern contrast.** There is no current analog to hermes's non-interactive-fail-open and container-skip-above-hardline-floor anti-patterns.

## Options considered

- **OPT-A — Codify ordering via property tests, defer test verb, retain bounded pattern backstop (RECOMMENDED).** Pin the harness's actual 7-phase evaluator using property tests to assert ordering invariants. Defer the test verb. Reject hermes-style pattern accretion but keep the core regex backstop.
- **OPT-B — Borrow hermes wholesale.** Rejected: hermes's 4-step ordering doesn't match our actual 7-phase implementation, its pattern accretion is an arms race, and we lack the YOLO modes that justify its middle tier.
- **OPT-C — Status quo.** Rejected: implicit ordering silently breaks.

## Recommendation

**OPT-A.**

1. **Pin the ordering invariant by property test.** Codify the harness's actual 7-phase order (empty-command → structural file-authoring deny → raw forbidden pattern → inert rg/grep AST proof → wrapper checks → broad wrapper allow → parse/allowlist). Concrete gains: a new permissive wrapper path cannot silently move ahead of a floor; a future suppression/carveout cannot be added before the raw deny without an explicit test; parser-unavailable behavior is pinned (regex backstop stays active when AST precision is unavailable).
   - Property tests can assert: structural file-authoring denial wins over later allow; genuine forbidden-pattern match never overridden by allowlisting; only closed rg/grep grammar suppresses a matched forbidden pattern; wrapper handling can't bypass mutation floors.
   - *Flip condition*: skipping formalization is only acceptable if the evaluator becomes a single declarative decision table with mechanically-inherent precedence.
2. **DEFER the read-only verdict verb.** `templates/core/.opencode/plugins/shell-guard/eval.js` already exposes the runtime evaluator via a read-only subprocess contract returning `{action, reason}`; live bridge tests already exercise real evaluation paths.
   - *BUILD TRIGGER*: build a narrow verb only if a short audit finds operators cannot currently answer "what verdict + which phase/rule decided it + was AST parsing available" against a rendered target. If built: compose the existing evaluator exactly once, report `action`/`reason`/`phase`/`rule_id`/`parser_mode`, with no execution/persistence/prompt/gateway action.
   - *Flip condition*: build now if existing testing/operational practice shows denied commands regularly require source-diving, or rendered project overlays make direct evaluator invocation impractical.
3. **RETAIN the bounded regex backstop but REJECT pattern accretion.** Reject hermes-style normalization + variant enumeration + name-based exemption accretion.
   - "A pattern layer is a good complement only when it blocks a stable, semantically identifiable dangerous invocation while every exception is structurally proven or narrowly bounded; it becomes an arms race when new text normalizations, guessed variants, or name-based exemptions are needed to chase spellings of the same behavior."
   - Each regex rule/carveout should have: a stable behavioral rationale + an explicit false-positive analysis + a closed grammar/structural proof for any suppression + a regression test for both the true positive and an attempted bypass + no name-only exemption.
4. **Conditional operator-deny middle tier:** IF an autonomous/mode-off/blanket-auto-approval/role-wide bypass is ever introduced, its evaluation must occur only AFTER the unbypassable core + project deny floors.
   - *Flip condition*: becomes a real gap when a mode can allow commands without normal permission prompting.

## Findings

- **(finding)**: source=templates/core/.opencode/plugins/shell-guard-core.js:1204-1427, confidence=high, type=fact — The live evaluator runs in JS with 7 actual phases, not pure Go.
- **(finding)**: source=internal/permission/shellguard_hook.go:241-277, confidence=high, type=fact — the shell-guard Go bridge is fail-closed (every fault → Deny), satisfying RF-B; JS evaluator Allow only on harness-branch + allowlist-match.
- **(finding)**: source=tmp/agent-runs/audit/shell-guard-reachability-audit.md, confidence=high, type=fact — P2 (no fail-open conversion) CONFIRMED. P3 (no classification bypass) CONFIRMED (`selectBackend` returns Backend-or-error, never Allow). P4 (parser-unavailable backstop retained) CONFIRMED (forbidden scan runs BEFORE parse).
- **(finding)**: source=templates/core/.opencode/repo-configs/forbidden-patterns.core.js:22-41,124-280,325-409, confidence=high, type=fact — The harness ships its regex deny layer intentionally as a necessary backstop for wrapped command strings.
- **(finding)**: source=refs/hermes-agent/tools/approval.py:3752-3790, confidence=high, type=fact — hermes codifies the evaluator order. [cross-check CONFIRMED]
- **(finding)**: source=refs/hermes-agent/hermes_cli/approvals_test.py:1-178, confidence=high, type=fact — hermes ships a read-only verdict verb. [cross-check CONFIRMED]
- **(finding)**: source=refs/hermes-agent/tools/approval.py:1003-1061,2098-2176, confidence=high, type=fact — hermes's normalization + variant enumeration + verify-* carve-out is an accreting regex arms race. [cross-check CONFIRMED]
- **(finding)**: source=internal/, confidence=medium, type=fact — Grep found zero `YOLO`/`autonomous`/`auto-approve`/`always_allow` mode in `internal/`.
- **(assumption)**: source=inference, confidence=medium, type=assumption — the harness's shell-guard evaluator order is currently implicit in call-site sequencing (not pinned by a golden test); this assumption should be confirmed in the implementation slice by locating the compose site.

## Contradictions

- The cross-check QUALIFIED the `:3865` fail-open: the hardline floor, sudo guard, and user-deny DO scan before the non-interactive fail-open, so it is a fail-open of the *ask-approval* layer, not of the floor.
- No contradiction between the harness posture and the BORROW of ordering invariants: the harness's AST gate is a stronger base than hermes's regex gate, so pinning the actual 7-phase ordering layers the proven disciplines on top of the better mechanism.

## Risks / open-questions

- **Residual unknown on reachability:** One trust boundary the audit could NOT verify from the Go substrate: how opencode's JS host routes an agent `bash` call to the `shell-guard.js` `tool.execute.before` handler before the Go binary (separate trust boundary, outside `internal/`).
- **Exit-code scheme (deferred).** If the test verb is ever built, must not collide with existing `vh-agent-harness` CLI exit codes.
- **AST vs normalization coverage gap.** If the AST gate misses a de-obfuscation case hermes's normalization catches, the fix is grammar/node-classification, not a regex.

## Recommended durable artifact path

`researches/decisions/guard-order-invariant-and-test-verb.md` (intended target; staged under `tmp/decisions-staging/` — read-only execution policy denied the direct write; see session handoff).

## Promotion targets (if the operator accepts)

When this becomes active guidance, the live targets a follow-up slice would touch: property tests pinning floor-over-bypass for the 7-phase order. The `GIT_MUTATION_VERBS` single-source discipline (`internal/permconfig/tables.go`) is the model for how to keep the Go/JS evaluator sets from drifting.