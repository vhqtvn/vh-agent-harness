# Decision: Consumer-Render Source-Path Defect Class

**Date:** 2026-07-26
**Status:** Accepted (record-of-decision). The defect class is named; the three
shipped instances are FIXED in commit `b69fb1f`; a render-time-check gate is
DEFERRED behind a named trigger; a cheaper CI regression fixture is captured as
a separate F1 follow-up. No code lands in this slice — this is the durable
record-of-decision that names the class so future instances are recognizable.
**Supersedes:** none (names a new defect class; does not reopen any prior
disposition).
**See also:**
[`./2026-07-23-vh-solara-orchestration-field-report-disposition.md`](./2026-07-23-vh-solara-orchestration-field-report-disposition.md)
(the F1-class failure taxonomy this defect's recurrence pattern engages; the
2026-07-26 addendum to that disposition records the P0-B instance of the same
untracked-adoption class).
[`../../docs/ai/shell-execution.md`](../../docs/ai/shell-execution.md) (the
render-time-check / render-location discipline this memo's DEFER engages).

## Framing

A supervised-overlay consumer report (v0.16.0 adoption) surfaced three shipped
verifier/doc surfaces that broke in a consumer render — a non-Go tree with no
`templates/core/` at the repo root. All three are the **same defect class**: a
shipped file references a path or applies a heuristic that is valid only in the
harness's own source checkout and silently fails (exit-1, ENOENT, wrong-root)
in every consumer render. The class is structural and recurring; this memo
names it so the next instance is recognizable rather than rediscovered.

**The class, one sentence:** *a consumer render that references a path or
heuristic valid only in the source checkout.* The source checkout dogfoods
itself (it carries `corpus.go` + `templates/core/`); a consumer render does
not. Any code path that assumes the source-checkout layout — a `go.mod`-anchored
root walk, a `.tmpl` source-file read, a `templates/core/...` citation — is
unreachable or wrong in a consumer.

## The three instances (FIXED in `b69fb1f`)

| ID | File | Defect (source-checkout-only assumption) | Fix shape |
|----|------|------------------------------------------|-----------|
| **TA-1** | `templates/core/.opencode/scripts/verify-f3-authoring-surfaces.js` (~L70, ~L140) | The `.tmpl` read path read `templates/core/.opencode/agents/build.md.tmpl` unconditionally. In a consumer render that file does not exist (the resolved `agents/build.md` is the shipped artifact); the script exited ENOENT. | Gated the `.tmpl` read behind the source-checkout identity heuristic (`isHarnessSourceCheckout`, ~L97-104), which reuses the EXACT predicate from `internal/cli/corpus_freshness.go::isSourceCheckout` (regular-file `corpus.go` AND directory `templates/core/` at the resolved root). A consumer render reads the resolved `agents/build.md`; a source checkout verifies the authoritative `.tmpl`. |
| **TA-2** | `templates/core/.opencode/scripts/verify-no-unrendered-paths.js` (~L46-72, `findRepoRoot`) | `findRepoRoot` anchored on `go.mod` — a Go-SPECIFIC manifest. A non-Go consumer (Python/Node) with no `go.mod` in the ancestor chain returned `null` and the guard exited 1; a Go consumer nested under a parent `go.mod` resolved the wrong root. | Re-anchored on `.git` (every consumer render is a git repo, language-agnostic) with `go.mod` / `pyproject.toml` / `package.json` as a defensive manifest fallback for unpacked-tarball-style trees. Polyglot: works in Go, Python, and Node consumers alike. |
| **TA-3** | `templates/core/.opencode/agents/build.md.tmpl`, `templates/core/.opencode/commands/approve-plan.md`, `templates/core/.opencode/commands/task-ready.md` (build.md.tmpl:~136, approve-plan.md:~18, task-ready.md:~34) | The F3 design-readiness citation pointed at `templates/core/.opencode/scripts/f3-design-readiness.js` — a source-tree-only path. A consumer render resolving the rendered doc has no `templates/core/`; the citation was a dangling source-tree pointer. | Rewrote the citations to the consumer-relative `.opencode/scripts/f3-design-readiness.js`, which resolves in BOTH the source checkout (via the rendered `.opencode/`) and a consumer render. |

All three fixes land in source (`templates/core/`) and the rendered `.opencode/`
mirrors are regenerated via `make update` (the `Makefile` target rebuilds the
binary first, so the embedded corpus is fresh — not a bare
`vh-agent-harness update`, per the dogfood-loop rule).

## The fix rationale

Two design decisions are load-bearing and worth recording:

1. **Polyglot anchor, not a Go assumption.** TA-2's fix anchors repo-root
   discovery on `.git` (universal for any version-controlled consumer render)
   rather than `go.mod`. The manifest files remain as a *defensive secondary*
   for the rare unpacked-tarball case, not the primary anchor. This is the
   generic correctness fix: a verifier must not bake a single language's
   manifest convention into the discovery of the repo root.

2. **Source-checkout identity reused, not reinvented.** TA-1's fix needed to
   distinguish "this is the harness's own source checkout" from "this is a
   consumer render." The predicate already existed —
   `internal/cli/corpus_freshness.go::isSourceCheckout` (the dev-stale-embed
   guard, `:96-139`) — and was reused verbatim (`isHarnessSourceCheckout` in
   the script mirrors `corpus.go` regular-file + `templates/core/` dir). This
   avoids a second, drift-prone definition of "is this the source checkout" and
   keeps the source-checkout identity a single declared concept.

## Recurrence facts

Two recurrence signals attach to this class and are recorded so the trigger
below is evaluable:

- **Recurrence fact 1 — the guard leaked the class it guards.** TA-2's
  `verify-no-unrendered-paths.js` IS the `186ba26` guard ("guard against running
  unrendered templates/core scripts"). That guard's own `findRepoRoot` baked in
  the very class it is meant to catch: a `go.mod` source-checkout assumption
  inside the script that detects unrendered/source-only paths. The guard could
  not reliably protect consumers from source-only assumptions while itself
  carrying one. This is an F1-flavored recursion (the safety mechanism
  reproduced the defect class it guards) and is recorded in-code.

- **Recurrence fact 2 — a prior stall-lane advisory predicted it.** A
  dogfood-only cross-reference dropped during an earlier stall-lane (an advisory
  that source-tree-only paths in shipped docs are a latent consumer-render
  hazard) anticipated this class. Three instances (TA-1/TA-2/TA-3) now confirm
  the advisory was not noise.

## Render-time-check DECISION: DEFER

**Verdict: DEFER — do not gate-shape now.** A render-time check that flags any
shipped file referencing a source-checkout-only path is appealing (it would have
caught all three instances pre-ship), but it carries real false-positive cost
against *legitimate* source-only references that must NOT be rewritten:

- `state-lib.js` render-location guard (a shipped script that legitimately
  distinguishes source-tree from rendered-tree locations);
- `verify-no-unrendered-paths.js`'s own recovery hint (which legitimately names
  `templates/core/` as the source of truth for re-render);
- `init_skill.py` source-tree exclusion logic (which legitimately reasons about
  the source tree);
- descriptive comments that name `templates/core/...` as documentation, not as
  a load-bearing path reference.

Distinguishing a *broken* source-only reference from a *legitimate* one is a
semantic judgment the check would have to make at every match, and a
false-positive on any of the above would block legitimate content. The
cost-benefit does not clear the gate threshold today: three instances fixed,
recurrence pattern now named in this memo, and a cheaper complement below.

**Trigger to promote to a gate:** a **4th instance** of the same class in a
shipped file, OR a **release-boundary audit** surfacing the class. Either makes
the false-positive cost worth paying.

**Cheaper complement (captured separately):** a CI regression fixture that runs
BOTH verifiers (`verify-no-unrendered-paths.js` + `verify-f3-authoring-surfaces.js`)
against a synthetic non-Go consumer tree (Python, no `go.mod`, no
`templates/core/`). This is the high-leverage, low-cost guard: it directly
reproduces the consumer-render environment and would catch any future instance
at CI time without the false-positive cost of a render-time path check. Captured
as a separate F1 follow-up card (NOT this memo's deliverable) — see the
closeout of the slice that produced this memo.

## Behavioral closure

```behavioral-closure
verdict: proven
path: the consumer-render reproduction (tmp/repro/consumer-sim) — a synthetic
  non-Go consumer tree (Python, no go.mod, no templates/core/) running BOTH
  verifiers (verify-no-unrendered-paths.js + verify-f3-authoring-surfaces.js)
verifier: both verifier scripts exit 0 against the consumer-sim tree
command: node templates/core/.opencode/scripts/verify-no-unrendered-paths.js
  (in consumer-sim) && node templates/core/.opencode/scripts/verify-f3-authoring-surfaces.js
  (in consumer-sim); both exit 0
result: proven
```

The crux is outcome-observed: before the fix, both verifiers exited non-zero
(`null` root / ENOENT on the `.tmpl` read) in the consumer-sim tree; after the
fix, both exit 0. The reproduction exercises the actual consumer-render code
path against a tree that lacks every source-checkout marker the defect class
depends on. This is the load-bearing path — the fix is proven, not
mechanism-asserted.

## Generic naming discipline

No adopter/overlay identifier appears in this memo. The reporter is referred to
generically as "a supervised-overlay consumer report" / "the rendering-bug
reporter" / "the consumer-render defect report." The consumer-render defect
class itself is named by its structural property (source-checkout-only path
assumption), not by who reported it. This discipline travels with any artifact
derived from this memo.

## Evidence / Provenance

| Claim | Verifying artifact / command | Verified |
|-------|------------------------------|----------|
| Three shipped instances of the class | `git show --stat b69fb1f` (12 files: 3 source fixes + rendered mirrors + lineage) | yes |
| TA-1 `.tmpl` read gated on source-checkout identity | `templates/core/.opencode/scripts/verify-f3-authoring-surfaces.js` `isHarnessSourceCheckout` (~L97-104) reusing `corpus_freshness.go::isSourceCheckout` (`:96-139`) | yes |
| TA-2 `findRepoRoot` polyglot anchor | `templates/core/.opencode/scripts/verify-no-unrendered-paths.js` `findRepoRoot` (~L46-72): `.git` primary, `go.mod`/`pyproject.toml`/`package.json` fallback | yes |
| TA-3 citation rewrite to consumer-relative path | `templates/core/.opencode/agents/build.md.tmpl:~136`, `approve-plan.md:~18`, `task-ready.md:~34` → `.opencode/scripts/f3-design-readiness.js` | yes |
| `186ba26` is the unrendered-paths guard | `git log --oneline -- templates/core/.opencode/scripts/verify-no-unrendered-paths.js` → `186ba26 fix(coordinator): guard against running unrendered templates/core scripts` | yes |
| Recurrence fact 1 (guard leaked the class it guards) | TA-2's pre-fix `findRepoRoot` baked `go.mod` into the very script detecting source-only paths | yes |
| Behavioral closure (consumer-sim reproduction) | both verifiers exit 0 against `tmp/repro/consumer-sim` (Python, no `go.mod`, no `templates/core/`) post-fix | yes |

House style: this memo follows the `2026-07-23` / `2026-07-25` convention
(bolded-metadata frontmatter; Framing → instances → fix rationale → recurrence
→ decision → closure → Evidence), matching the disposition and F2 memos'
granularity at decision level.
