# Source Packet — Release-Ceremony N→R→M Invocation Topology (O3 / O5 / G6 evidence-closing)

**Kind:** source packet (facts only — no implementation design)
**HEAD at research time:** `798f4c4ad61b767b349c339d66c7cc3998decdc5` (re-derived via `git rev-parse HEAD`).
**Scope:** map the exact N→R→M release-ceremony invocation topology so O3 (pre-M gate) and G6 (machine-consumed skill-pilot gate) can be re-debated evidence-bound.
**Time-sensitivity:** STABLE (code surfaces, not recency-sensitive).
**Source policy:** repo canon only.

## Repo sources checked
`templates/overlays/release/agents/releaser.md` · `.vh-agent-harness/overlays/harness-dogfood/agents/harness-release-readiness.md` · `templates/overlays/release/callable-graph-snippet.md` · `.vh-agent-harness/overlays/harness-dogfood/callable-graph-snippet.md` · `templates/overlays/release/permission-pack.jsonc` · `.vh-agent-harness/overlays/harness-dogfood/permission-pack.jsonc` · `scripts/release-tag.sh` · `.opencode/scripts/commit-gate.sh` · `AGENTS.md` · `researches/decisions/2026-07-23-release-defer-dual-mechanism-reconciliation.md` · test seams in `internal/cli/`.

## Fact 1 — Invocation topology (post-N readiness invocation) — CLOSED
- Releaser callable edges: `templates/overlays/release/permission-pack.jsonc:62` — `"task": { "*": "deny", "committer": "allow" }`. Releaser may delegate ONLY to `committer`. Restated `releaser.md:819-821` and `:515` ("The releaser CANNOT invoke the readiness agent directly").
- Readiness reporter requires caller holds `harness-release-readiness: allow` in its `permission.task` map. Reporter's own pack `.vh-agent-harness/overlays/harness-dogfood/permission-pack.jsonc:44` is `"task": { "*": "deny" }` (leaf).
- The SAME three orchestrators (`build`/`coordination`/`project-coordinator`) that delegate to the releaser ALSO hold the readiness edge — `.vh-agent-harness/overlays/harness-dogfood/callable-graph-snippet.md:11-13` + `permission-pack.jsonc:45` (`"delegateFrom": ["build", "coordination", "project-coordinator"]`). The Go emitters inject `harness-release-readiness: allow` into each orchestrator's task map.

**Verdict:** NO new callable-graph edge is required. The edge ALREADY EXISTS from each parent orchestrator. The gap is an **execution-ownership gap**, not a topology gap: the releaser declares end-to-end N→R→M ownership (`releaser.md:12-16`) but cannot self-trigger the reporter between N and R (`releaser.md:504-518`, explicit and designed). Control must leave the releaser at the N→R boundary.

## Fact 2 — R commit authority — CLOSED
- Reporter is never-commit by invariant AND permission: `harness-release-readiness.md:28-43` (Invariant 1: write NO file EXCEPT `.vh-agent-harness/release-readiness-pass.json`); `:88-91` (Invariant 6: NOT part of gated-commit; `gate: deny`). Permission route `permission-pack.jsonc:40` (`gate:deny`), `:41` (`harness:deny`), `:44` (`task:*:deny`). ONLY write is the scoped editOverride `:42` — a worktree file edit, never a commit. Exclusive-path contract `harness-release-readiness.md:878-893`.
- Authorized commit actor for R = the **committer**, via releaser's single-path delegation: `releaser.md:533-543` (Step 3.2 DELEGATE) instructs the committer to run `commit-gate.sh acquire --message-file ... --paths '[".vh-agent-harness/release-readiness-pass.json"]'`. Committer (not releaser) holds the gate; releaser is `gateExempt` (`permission-pack.jsonc:63`) and OMITS the `gate` decision. Single-path scope guarantees the artifact-only diff.

**Verdict:** R authored (worktree-only) by `harness-release-readiness`; R committed by the committer via releaser's `task.committer: allow` + gated-commit message-as-file protocol. If the designed releaser→committer delegation for R is skipped, R stays dirty and cannot satisfy the tag-time handshake (wrapper reads `HEAD:` not worktree, `release-tag.sh:698`).

## Fact 3 — Pre-M enforcement seam — CLOSED (NEGATIVE: NO such seam exists today)
- **`scripts/release-tag.sh` (tag-time handshake):** validates N→R→M at TAG TIME only — `release-tag.sh:676-813` (HEAD^ exists `:682-687`, HEAD^^ exists `:689-694`, artifact at `HEAD:` `:698-703`, `HEAD^^..HEAD^` single-path diff `:708-714`, schema `:723-776`, `commit_sha==HEAD^^` `:799-803`, all-ready `:808-813`). Tag-time validator; refuses the TAG not the M commit; M already HEAD when it runs.
- **`.opencode/scripts/commit-gate.sh` (where M lands):** FULLY GENERIC. `acquire` handler (`:1825`, `:1901`): atomic lock + private-index staging (`GIT_INDEX_FILE`, `:795-922`) + `git read-tree`/`add`/`write-tree` + tree-hash + metadata write. `paths` stored in metadata (`:811-812`) used ONLY to scope staging — NEVER validated against ceremony constraints. Grep `pre-commit|hook|manifest|readiness|release-defer|single-path|N.*R.*M|HEAD^^` → ZERO ceremony-aware matches. NO pre-commit hook; NO `core.hooksPath`. Commits ANY single-path it is told to commit.
- **No dedicated manifest-writer helper.** Releaser IS the writer via Write tool (`releaser.md:593-602`). Only manifest validator = DEFER evaluator `check-defer-triggers.js --mode=release`, invoked post-M (`releaser.md:613-628`) and at tag time.

**Verdict (NEGATIVE):** NO pre-M enforcement seam exists today. Only ceremony-aware validators run at tag time (release-tag.sh) or post-M (evaluator re-verification). **O3 must CREATE a new seam.** The prior N→R→M decision's G0c precedent (wrapper added tag-time gates — `researches/decisions/2026-07-23-release-defer-dual-mechanism-reconciliation.md:146-157`: "the wrapper is the only machine layer effective this session") does NOT transfer: O3's pre-M requirement is structurally different because M must be HEAD for the wrapper to run, so the wrapper cannot serve as a pre-M gate without becoming an orchestrator.

## Fact 4 — Attestation author-vs-transition-authority invariants — CLOSED
Three invariants any topology must preserve:
- **A — "model output is a candidate, never transition authority":** `AGENTS.md:110-122` (Safety invariant). Enforced by capability policy + ownership classification + gate-controlled side effects.
- **B — author/authorize separation:** `release-tag.sh:636-643` — reporter authors the verdict, wrapper authorizes at tag. Exclusive-path: `templates/overlays/release/permission-pack.jsonc:46-56` (releaser OMITS readiness-path override); reporter alone holds it.
- **C — `handoff_to_releaser` / `authorized_by` binding:** `harness-release-readiness.md:57-83` (Invariant 4: populate only when `ready:yes`; operator-initiation IS authorization; null on six STOP-AND-ASK conditions; reporter `task:*:deny` refuses all delegations). Handoff is a REPORT FIELD + human relay, NOT a task delegation.

## G6 topology-share verdict — UNDECIDED (feasible to share; choice is a separate policy decision)
- G6 today: model-driven in report but deterministic-in-structure; wrapper does NOT re-compute or consume it (`harness-release-readiness.md:855-864`). G6 has NO slot in the readiness-pass artifact (only G1–G5). G6 evidence is a two-surface cross-check (backlog `s2-hold` token + `researches/sources/` packet, both committed) — DETERMINISTICALLY RECOMPUTABLE (`harness-release-readiness.md:401-449`). G6 blocker → `ready:no` + null handoff.
- Three feasible topologies (mutually exclusive):
  1. **Deterministic recompute by wrapper** (parallel to G0/G0b/G7) — wrapper already recomputes G0/G0b/G7 from primary state; G6's committed inputs are similarly primary. Shares G7's topology, NOT R's. Only mechanically-testable topology today.
  2. **Separated attestation** (parallel to R) — G6 gets its own artifact slot; reporter authors, committer commits, wrapper consumes. Shares R's topology but contradicts the "deterministic in structure" framing.
  3. **Retain human/orchestrator boundary** (status quo) — `ready:no` + null handoff, machine-unenforced at tag.
- **Verdict: UNDECIDED.** Sharing is FEASIBLE under two distinct topologies, but they are mutually exclusive and the choice is a POLICY decision (is G6 deterministic like G7, or model-attested like G1–G5?). The facts 1–4 evidence does NOT pick the option. No runtime harness exists, so any G6 machine-enforcement claim depending on agent-runtime behavior is not-demonstrable end-to-end; only a deterministic-recompute gate is testable today.

## Recurrence mechanism (confirmed + sharpened)
The "separate release-prep path commits M against P while R remains dirty" is a DEVIATION from designed Step 3.2, not a located code path. The designed flow DOES commit R before M via committer. The recurrence (routed by the coordinator as G7-clearing to `build`) = build commits M against prep-HEAD P directly, skipping designed N→R→M; releaser then reconstructs N→R→M on top. (Medium-confidence inference — the "separate path" is inferred as a workaround, not cited code.)

## Exact repo paths for the eventual build slice
| Artifact | Path | Why |
|---|---|---|
| Releaser contract | `templates/overlays/release/agents/releaser.md` | N→R→M ceremony, Step 3.2 (504-518), Step 3.3 (566-645) |
| Readiness reporter | `.vh-agent-harness/overlays/harness-dogfood/agents/harness-release-readiness.md` | Never-commit (28-43, 88-91), G6 (401-458), schema (825-837), fence (855-864, 451-458) |
| Releaser perm pack | `templates/overlays/release/permission-pack.jsonc` | task map (62), gateExempt (63), delegateFrom (64), readiness omission (46-56) |
| Readiness perm pack | `.vh-agent-harness/overlays/harness-dogfood/permission-pack.jsonc` | gate:deny (40), task:*:deny (44), editOverride (42), delegateFrom (45) |
| Tag-time validator | `scripts/release-tag.sh:676-813` | ONLY ceremony-aware validator (tag time) |
| Generic commit gate | `.opencode/scripts/commit-gate.sh` | Where M lands; GENERIC, no ceremony seam (acquire :1825/:1901) |
| DEFER evaluator | `.opencode/scripts/check-defer-triggers.js` (`--mode=release`) | Post-M / tag-time manifest validator |
| Safety invariant | `AGENTS.md:110-122` | Model output ≠ transition authority |
| Prior N→R→M decision | `researches/decisions/2026-07-23-release-defer-dual-mechanism-reconciliation.md` | Wrapper-authority + G0c precedent (146-157) |
| Manifest / Artifact | `.vh-agent-harness/release-defer-dispositions.json` / `.vh-agent-harness/release-readiness-pass.json` | M target / R target |

## Existing test seams
| Test | Path | Exercises |
|---|---|---|
| **Manifest + readiness (PRIMARY for O3)** | `internal/cli/release_tag_manifest_test.go` | `setupReleaseTagManifestRepo` (line 46) + `insertReadinessArtifactCommit` (line 1037) build EXACT N→R→M in scratch repo. Fail-closed: missing artifact (1135), invalid schema (1156), wrong commit_sha (1290), non-single-path (1348). |
| Commit gate (black-box) | `internal/cli/commit_gate_closeout_ledger_test.go` | Black-boxes RENDERED commit-gate.sh in scratch repo (`setupScratchRepo` line 52). Seam if O3 adds ceremony logic to commit-gate.sh. |
| Push-only / G0c / gates | `release_tag_push_only_test.go`, `release_tag_g0c_path_test.go`, `release_gate_test.go` | Wrapper contract surfaces |
| DEFER release | `check_defer_release_manifest_test.go`, `check_defer_release_test.go` | Manifest evaluator |
| Interaction contract (structural) | `.opencode/scripts/verify-release-interaction-contract.js` | Text-only; HONEST FRAMING: NO runtime harness exists |

`scripts/release-tag.sh` has NO `--dry-run`; fresh N/R/M flows are exercised ONLY via Go test harnesses.

## Contradictions
None vs the settled assumptions. HEAD drift (`798f4c4`) is predicted (concurrent docs session); release surfaces stable. Root cause CONFIRMED. Wrapper ranges CONFIRMED. Two inferences flagged: (1) the "separate path" as deviation (medium conf); (2) G6 share-feasibility as topology-only, not policy-resolving.
