# Cohort-Consistency Preflight — Frame Adjudication Decision Memo

- **Task card**: `task-2026-11-07t05-01-59-o4-cohort-consistency-preflight-feasibility-research-plan-seam-go-es-module-link-check-semantic-capability-check` (status ready)
- **Session**: `o4-adjudication` (execution session; parent ses_e6280d946ffeFByKS2CWfouMb5)
- **Date**: 2026-11-07
- **Artifact type**: decision memo (explicitly mandated by card — frame adjudication with recommendation)
- **Staged at**: `tmp/agent-runs/o4-adjudication/` (read-only researcher writes only `tmp/`; landing at `researches/decisions/2026-11-07-cohort-consistency-preflight-feasibility.md` is the coordinator's one-step move)

## Research question

Which cohort-consistency frame should ship for the stale-preserve skew class:

- **(detection)** pre-write refusal only on *proven incompatibility* (link-level breakage demonstrable before write), warnings otherwise; or
- **(qualification)** guarantee-scoped admission control — a tri-state qualified / proven-incompatible / unattested model that refuses *activation* of unqualified states.

Answered through Q1 (pre-write seam capability — shared gate for both frames), Q2 (Go ES-module link scanner feasibility), Q3 (semantic verifier credibility — honest "not credible" permitted), Q4 (qualification-policy feasibility, six sub-questions).

## Scope

- **In scope**: feasibility adjudication only — Q1–Q4 verdicts, frame verdict with adjudication table scored against both frames' flip conditions, the recommended frame's full contingent policy package (m0034 per card's `open_questions`; m0034/m0038 are parent-session debate IDs not present in repo files — the card is the authoritative carrier), architecture sketch (check location, warn/refuse/fail triggers, new shipped surface needing bootstrap-inventory decisions), contradiction audit vs `researches/decisions/origin-hash-update-sync.md`.
- **Out of scope (card non-goals)**: O3 Go per-gate double-wiring; O5 adopter upstreaming; automated 3-way merge; blanket hash refusal; semantic-diagnosis claims; any implementation.

## Time-sensitivity

**Low.** The question is repo-internal architecture; the decisive evidence (plan-before-write apply pipeline, overlay collision semantics, origin-hash provenance semantics, corpus module graph) is stable repo fact. Web prior art (pure-Go ESM parsing libraries, admission-control patterns) only informs feasibility ceilings and is slow-moving; findings re-checked 2026-11-07.

## Source policy

Repo canon first (task card; `internal/substrate/apply.go`; `internal/cli/seam.go`; `internal/cli/doctor.go`; `internal/overlay/shadow.go`; `internal/originhash/originhash.go`; `internal/managedfile/managedfile.go`; `internal/taskcard/validator_test.go`; `templates/core/.opencode/scripts/*`; `researches/decisions/origin-hash-update-sync.md`; `researches/sources/2026-11-05-origin-hash-stale-preserve-skew.md`; `templates/migrations/v0.29.0.md`). Web allowlist per card: static ES-module import/export analysis in Go without Node/V8 (goja, esbuild, coder/guts, MoeForge/moejs); release-qualification / admission-control patterns for platform-managed plugin ecosystems (VS Code `engines.vscode`, OpenClaw plugin acceptance model, Kubernetes admission controllers). Primary sources preferred (READMEs, pkg.go.dev, vendor docs).

## Settled ground truth (card constraints — verified, cite-through)

- **Overlay packs cannot replace same-path builtins; collisions fail closed.** `internal/overlay/shadow.go:3-13,24-55` (`ErrShadowBuiltin` + managed→owned S2 replacement guidance), enforced at `internal/cli/seam.go:952-961`. *Precision note*: the card cites `overlay.go:233-244,274-286`; those lines are `isUnitFile`/`RenderUnits` (near-miss) — the substance lives in `shadow.go`/`seam.go` as cited here.
- **Origin-hashes are provenance, not qualification.** `internal/originhash/originhash.go:96-103` (store = bytes the platform last wrote); `internal/substrate/apply.go:139-149` (`GenerationFullyApplied` semantics — a successful generation ≠ coherent advancement), `:325-350` (origin recording/carry-forward).
- **No frame claims arbitrary semantic diagnosis** (settled in both parent debates). Effective-tree model, link≠semantic distinction, and no-preservation-mechanics-change are fixed.

---

## Q1 — Pre-write seam capability: **FEASIBLE AT EXISTING SEAM (strong yes)**

**Verdict: `feasible-at-seam`.** The full effective tree (which paths advance to staged bytes, which preserve live bytes, which armed-merge) is knowable before any write, at the existing `seamApply` boundary, with zero substrate API change.

Evidence:

- `substrate.Apply` (`internal/substrate/apply.go:180-358`) plans **all** per-file outcomes (`planOutcome`, `:205-214`) **before** any write (`:262-264`); the plan phase is side-effect-free; `DryRun` returns the full plan (`:249-251`). Managed writes are verbatim staged-byte copies (`os.ReadFile(stagedPath)` → `os.WriteFile`, `:597-604,652`) ⇒ assessed bytes = applied bytes for `platform_managed`.
- `seamApply` (`internal/cli/seam.go:227-459`) already runs **pre-write staged-bytes linting**: `warnIfAllowedCommandsCustomized` (`:244-246`), `warnIfDeadGrants` (`:259`), `warnIfGatedCommitIncoherent` (`:270`) — all lint `ps.staging` before `substrate.Apply`. A cohort link check slots into this exact position.
- Staging is rendered by `prepareSeamStaging` (`:105-113`) — the shared front-half of `accept-platform` — so same-inputs rendering is deterministic, and the landed `stalePreservedPaths` (`:305`, body `:461-512`) already demonstrates post-plan, staging-alive analysis with an explicit lifecycle contract (`:472-475`).
- doctor's `checkManagedDrift` (`internal/cli/doctor.go:1073+`, byte-compare `:1260-1342`) re-renders through the same pipeline and routes through the shared `managedfile.ClassifyPreserved` (`internal/managedfile/managedfile.go:153-183`), with the landed preserved-stale-vs-corpus WARN (`:1291-1313`, tier escalation `:1366-1400`). So the detection-time WARN baseline both frames build on is **landed** surface.

Caveats (all same-class as today, none blocking):

1. **Plan/execute TOCTOU window** — a live run re-plans; a concurrent edit between dry-run-plan and live-apply can change outcomes. Same class as today's intra-`Apply` window.
2. **Armed-merge bytes re-derives at execute** (`apply.go:617-643`) — plan/execute can diverge for `platform_armed` under concurrent edit. Not the cohort class; `platform_managed` unaffected.
3. **Staging lifecycle** — a dry-run report alone (outside `seamApply`) lacks staging bytes; the check must live inside `seamApply` (recommended) or substrate (via a new `ApplyOptions` pre-execute hook — the hardening alternative, new internal API only).

## Q2 — Go ES-module link scanner: **FEASIBLE WITH NEW INTERNAL SURFACE (no rendered files)**

**Verdict: `feasible-with-new-surface`** — a new pure-Go, zero-dependency internal package (e.g. `internal/eslink`). Because it renders nothing into consumer repos, **no bootstrap-inventory decision is required for the scanner itself**.

Evidence — the corpus module graph is real and non-trivial:

- `state-lib.js` is a 10,435-line provider with `export class StateError` (`:223`), one large named-export block with **interleaved comments** (`:10256-10344`), and `export default {` (`:10346`); ~17 static multi-line alphabetical importers (e.g. `verify-task-registry.js:5-22`).
- Parser hazards **confirmed present**: import renames (`forbidden-patterns.js:25` `FORBIDDEN_PATTERNS as _CORE`; `strict as assert` for `node:assert`); re-exports (`forbidden-patterns.js:32`); dynamic imports (`shell-guard-core.js:475` external `web-tree-sitter`; `forbidden-patterns.js:39` optional project file that may legitimately not exist; `verify-task-registry.js:582` state-lib); import-text inside comments (`forbidden-patterns.js:7`); cross-directory edges (plugins → `../scripts/`, `../repo-configs/`, e.g. `shell-guard-core.js:47-48`, `pause-new-work.js:57`); varied export forms (`session-state.js:10-314`).
- ⇒ **naive regex is unreliable.** Required: a comment/string/template-literal-aware, statement-level lexer (small, pure Go, ~hundreds of lines) with **tri-state unknown-vs-missing discipline** — unparseable files classify as *unknown* (warn+skip), never *missing* (fail). Repo-native precedent for tri-state discipline: `renderstate.SourceState` (`internal/renderstate/diff.go:75`; `overlaySkillSourceChecker` `seam.go:543-573`) and state-lib's own tri-state git helpers (`state-lib.js:6179`). Dynamic imports of optional/project files are excluded from the required-edge set; bare `node:`/package specifiers stay out of the graph.

Web prior art (allowlist):

- **goja** (dop251/goja, MIT, pure Go): ECMAScript 5.1(+) with most-of-ES6 WIP (README, fetched 2026-11). ESM import/export syntax support is **not documented** as a feature; and a full runtime dependency is disproportionate for a lexer problem. Not recommended.
- **esbuild**: pure-Go production ESM parser+linker — existence proof that Go-native ESM static analysis is practical — but the parser/linker live in `internal/` packages (not importable); shelling out to the binary violates the static-binary posture; vendoring internals = maintenance fork. Cited as feasibility proof only.
- **coder/guts v1.7.1** (Jun 2026, CC0-1.0, pure Go, pkg.go.dev): exposes `ImportDeclaration`-level TS/ESM AST manipulation as a Go library — proof that full ESM/TS parsing in pure Go without Node is practical as an importable library, if an in-repo lexer ever proves insufficient.
- **MoeForge/moejs**: pure-Go JS/TS runtime (parser drops TS types) — additional existence proof.

**Judgment** (labeled): build the small in-repo lexer first (the required subset — top-level import/export statements, comment/string-aware — is hundreds of lines and testable against the shipped corpus fixtures); treat `coder/guts` as the fallback if the subset grows. No external dependency is needed for v1.

## Q3 — Semantic verifier credibility: **NOT CREDIBLE (honest verdict, per card's explicit permission)**

**Verdict: `not-credible`.** A deterministic Go-side verifier of JS *semantics* ("does the closeout path actually refuse an absent `decision_record`?") is not achievable within this repo's constraints.

Evidence:

- The retention gate is enforced **only in JS**: `saveCoordinationTaskCloseout` at `templates/core/.opencode/scripts/state-lib.js:9154` (locked variant `:9178`). *(Drift note: the prior source packet's line cites — 8932, 9006-9044 — have drifted ~+200 lines; function existence re-verified.)*
- The Go taskcard validator is structural-only (`internal/taskcard/validator_test.go:380-461` pins `decision_record` subschema fixtures; no closeout-requirement/reachability enforcement; `task-card validate` is opt-in INFORMS-only).
- The only deterministic paths to semantic verification are: (i) executing the JS — Node breaks the static-binary/zero-dep posture; goja is not credible because state-lib depends on `node:fs`/`node:path`/`node:child_process`/`import.meta`/spawn-git; or (ii) duplicating gate logic in Go — O3-rejected double-wiring. String/AST pattern-matching ("a refusal branch is present") is semantic-diagnosis-adjacent, brittle under refactor, and indistinguishable from absent behavior — excluded by the settled no-semantic-diagnosis constraint.

**Consequence (fixed by card)**: with Q3 not-credible, the detection frame reduces to link checks + warnings. The semantic frontier must be policed by *transparency* (naming what is not checked), never by pretense.

## Q4 — Qualification-policy feasibility: **STRUCTURALLY FEASIBLE, BUT BEHAVIORAL QUALIFICATION NOT CREDIBLE — the frame is coherent and reco(very)-capable, yet its "qualified" tier is unreachable for any legitimately-diverged implementation**

- **(a) Minimal guarantee-family scope — feasible.** The decision-record-retention cohort is enumerable: provider `state-lib.js`; sibling consumers `verify-task-registry.js` (+16 other importers); schema = task-card v3 `decision_record` subschema (Go-validated structurally); docs (`docs/coordination/CLOSEOUT_TEMPLATE.md`, `RECORD_LIFECYCLE.md`, `README.md`) + commands (`task-closeout`/`task-delete`). The b788b43→bb60068→cccf1b0 trilogy changed provider+consumer in lockstep (prior packet; `git log -S collectDecisionItems` → cccf1b0) — the cohort is a real, historically-coupled unit.
- **(b) Requirement authority independent of preserved JS — feasible (strong).** The binary (embedded corpus + Go tables + taskcard schema) is re-derived every run and never preservation-eligible ⇒ a Go-side guarantee-registry table is inherently independent of preserved JS. Overlay/builtin collision fail-closed semantics (`shadow.go:24-55`) mean a pack can never override the requirement table.
- **(c) Qualified release-artifact config vs independent behavioral evidence; token substitution — feasible with a digest-keying rule.** `{{COORDINATOR_DIR}}` appears in *executable* positions (`state-lib.js:748`; `verify-task-registry.js:38,87,132,140,…` — `:140` executable), so live/staged bytes are project-specific post-substitution. **Rule**: any qualification digest must key on canonical pre-substitution corpus bytes (binary-internal embed FS) or the per-project staged re-render (what doctor already computes) — never raw live bytes. (Originhash precedent: hashes post-render staged bytes — memo §Token-stability.)
- **(d) Real behavioral tests qualifying custom impls — NOT CREDIBLE (honest verdict).** Behavioral qualification requires executing preserved JS: Node breaks the static posture (and is best-effort nondeterministic); goja cannot host the node-API surface state-lib requires (Q3 evidence). ⇒ per-adopter behavioral qualification is not credible. The only qualification evidence available is the platform's own canonical corpus under upstream test + cohort digest identity; local divergence ⇒ **UNATTESTED** (not proven-incompatible) unless link-level breakage is demonstrable.
- **(e) Pre-write integration, concurrent edits, partial writes, legacy metadata, override semantics, recovery — feasible.** Integration rides the Q1 seam (same plan). Refusal is pre-write ⇒ no partial writes by construction. refuse→reconcile→rerun maps to existing verbs (update refuses → `accept-platform <path>` / port-edit → update reruns; re-plan absorbs concurrent edits — same TOCTOU class as today). Legacy metadata: origin-store absence / `UnknownBaseline` already preserved-stalls (`managedfile.go:157-165`) ⇒ the registry must treat no-record as UNATTESTED with **no grandfathering** (consistent with F6's no-auto-adopt stance). Override keeps UNATTESTED (D2' semantics — recorded, not laundered into qualified).
- **(f) Friction on benign patches to the large shared provider — material and one-sided.** Any divergence of a named-guarantee file (including whitespace/comment edits to `state-lib.js`) drops certification to UNATTESTED; certification keyed on canonical-bytes equality is exact, so benign edits cannot be distinguished from breaking ones without (d), which is not credible. Escape = re-attest via `accept-platform` (abandons the preserve) or a documented override that persists UNATTESTED. Doctor D3' assurance-FAIL is scoped to named guarantees (not blanket), but for a preserving adopter it fires on **every** update touching the cohort.

Web prior art (allowlist): VS Code `engines.vscode` (install/activation-time admission refusal on declared engine range — declaration-keyed, i.e. exactly the self-declaration weakness the card rejects for *semantic* capability, but sound as independently-derived *range* admission control); OpenClaw plugin acceptance (acceptance reused only while declared surface + recorded integrity unchanged; updates require fresh acceptance — snippet-level confidence, docs.openclaw.ai/plugins/manage-plugins); Kubernetes admission controllers (two-phase mutate/validate — canonical admission architecture). These support the D1'/D2' *shape* (acceptance recorded per (surface, integrity), invalidated on change, enforced at activation). **They do not supply the missing ingredient — behavioral qualification of diverged implementations — because in each prior art the platform can execute what it admits.**

---

## FRAME VERDICT

### Adjudication table (criteria per card)

| # | Criterion | Detection (m0034) | Qualification (m0038) | Edge |
|---|---|---|---|---|
| 1 | Catches actual adopter failure class (cohort-skew → link breakage at runtime) | Yes — Q1 seam + Q2 scanner catch exactly this class pre-write | Yes — same scanner, same seam | **Tie** |
| 1b | Semantic non-enforcement (preserved file exports fine, gate logic altered) | No — and says so (warnings name the frontier) | No — despite refusing more states (Q4d: "qualified" unreachable for diverged impls) | **Tie (both honest only via labeling)** |
| 2 | Prevention/removal vs detection | Prevents only *proven-incompatible* writes (D2) | Prevents *all unqualified activation* — but the extra margin is conservatism, not verified safety (Q4d) | **Qualification on paper; evaporates on inspection** |
| 3 | Implementation risk (parser/verifier fidelity + surface) | Q2 scanner + `--allow-broken-cohort` + doctor check; no new committed state | Same scanner **plus** registry table, digests, tri-state bookkeeping, committed sidecar (F7-class decision) | **Detection** |
| 4 | Adopter friction (benign patches to shared provider) | Benign patches pass (links intact); refusal only on proven breakage | Every benign patch to a named-guarantee file ⇒ UNATTESTED ⇒ refuse-until-override, persistently (Q4f) | **Detection (decisively)** |
| 5 | Coherence with preservation guarantee + domain-free core | Refusal blocks sibling advance, never the preserve itself; escape hatch; no rendered files; domain-free | Registry is independent (Q4b) and scoped in letter, but functionally locks preserving adopters into override-steady-state (see below) | **Detection** |

### Flip-condition audit

- **Detection retains if Q3 shows adequate actual-failure coverage with materially less friction.**
  - Materially less friction: **yes** (criterion 4).
  - Adequate coverage: **yes under the comparative reading, and the comparative reading is the correct one** — *judgment, labeled*: the card's settled constraints already exclude semantic diagnosis from *both* frames ("no frame claims arbitrary semantic diagnosis"), so "actual-failure coverage" cannot sensibly mean the semantic axis the constraints foreclosed; it means the *demonstrated* failure class (stale-preserve cohort skew → link breakage, per the 2026-11-05 source packet), which the Q2 scanner covers. Under an absolute reading (coverage must include semantics), *both* frames fail and the fallback outcome is the landed status quo (WARN baseline) plus the honesty rider below — named here so the coordinator can select it explicitly if they reject the comparative reading.
- **Qualification rejected if (i) it lacks independence from preserved JS** — no: Q4b shows structural independence. **(iii) it cannot support refuse→override→reconcile→rerun** — no: Q4e shows recovery maps to existing verbs. **(ii) it becomes blanket template lock-in** — **fires in effect**: *judgment, labeled* — the registry is scoped in letter (named guarantees only), but because behavioral qualification is not credible (Q4d), the "qualified" tier is reachable *only* by canonical-bytes identity; any legitimate preserve of a named-guarantee file — a supported, encouraged preservation action — lands permanently in UNATTESTED, and D2' refuses activation on every subsequent cohort-touching update, escapable only by override (which persists UNATTESTED) or abandoning the preserve via `accept-platform`. Refusal without evidentiary payoff, borne precisely by the adopters the frame claims to serve, is template lock-in through the back door. This is an inferential adjudication from Q4d+Q4f, not a repo fact.

### Verdict

> **SHIP THE DETECTION FRAME (m0034 contingent package: D1 hybrid scan + D2 pre-write proven-incompatibility refusal with `--allow-broken-cohort` + D3 separate doctor FAIL), with a mandated honesty rider.**
>
> **REJECT THE QUALIFICATION FRAME (m0038) on rejection-condition (ii) in effect, sustained by Q4(d)** — structurally feasible (Q4a/b/c/e all pass) but its incremental guarantee over detection is unverifiable by its own admission, so its refusals block benign work without evidentiary payoff.
>
> **Honesty rider (mandatory)**: because Q3/Q4(d) are not-credible, D1's WARN wording and D3's doctor text must name the semantic frontier explicitly ("link integrity only; behavior of preserved files is not verified") — transparency is the only legitimate policing of that frontier. Record the qualification frame as **revisitable** if a credible behavioral harness ever lands (e.g. an opt-in Node-based test runner — out of posture today; would itself need a default-in/default-out decision).

## Recommended contingent policy package (detection, m0034 — full spec)

1. **D1 — hybrid cohort link scan (WARN on unknown, evidence on breakage).** A pure-Go statement-level ES-module lexer (`internal/eslink`, new internal package, zero deps) builds the rendered `.opencode` tree's relative-specifier module graph (scripts + plugins + agents cross-dir edges included). Tri-state: `ok` / `broken` (required edge → module present in effective tree but import missing or not exported) / `unknown` (unparseable file, dynamic import, optional/project file — warn + skip, never fail). Excluded: bare `node:`/package specifiers; dynamic imports of optional files (registry: `forbidden-patterns.project.js`). 
2. **D2 — pre-write refusal on proven incompatibility.** Inside `seamApply`, after staging render and existing lints (`seam.go:244-270` position), before `substrate.Apply`: two-phase (dry-run plan → compose effective tree = staged bytes for advancing paths + live bytes for preserved paths → scan → refuse or live-apply). Refusal exits non-zero, writes nothing (Q1: pre-write is structurally guaranteed), names the broken edges and the two escape hatches: `--allow-broken-cohort` (downgrade refusal to WARN + proceed, preserving exactly as today) or reconcile first (`accept-platform <path>` / port-edit) then re-run. The preserve itself is never clobbered or bypassed — refusal blocks sibling advances only (consistent with the origin-hash memo's per-file preservation contract).
3. **D3 — separate doctor check (not entangled with managed-drift tier escalation).** New `checkCohortLinks` re-derives staging via the same `renderSeamStaging` pipeline (`doctor.go:1260-1342` precedent) + same scanner over the current effective tree: FAIL on proven-incompatible (naming the edges), WARN on unknown (naming the semantic frontier per the honesty rider). Distinct check id so the landed WARN-escalation logic (`:1366-1400`) is untouched. Doctor help text + `README.agent.md` updated same-change (agent-operability rule).

## Architecture sketch (where each check lives)

```
vh-agent-harness update / accept-platform
  └─ seamApply (internal/cli/seam.go:227-459)
       ├─ prepareSeamStaging        (existing — renders corpus+overlays to temp staging)
       ├─ existing pre-write lints  (existing — :244-270)
       ├─ NEW cohortLinkScan        (D1/D2 — internal/eslink over staging + dry-run plan's
       │                            effective tree; refuse→exit pre-write, or WARN if
       │                            --allow-broken-cohort; TOCTOU = same class as today)
       └─ substrate.Apply           (existing — unchanged preservation mechanics)

vh-agent-harness doctor
  └─ checkCohortLinks               (D3 — new separate check; renderSeamStaging + same
                                   scanner over current effective tree; FAIL broken /
                                   WARN unknown w/ semantic-frontier wording)
```

- **Warn triggers**: unknown-class findings (unparseable, dynamic, optional-file edges); broken-under-`--allow-broken-cohort`.
- **Refuse triggers** (update path, pre-write): required static edge resolves to an effective-tree module whose import is missing or not exported.
- **Fail triggers** (doctor only): proven-incompatible links on the current effective tree.
- **Never triggers**: semantic/behavioral claims of any kind (Q3/Q4d rider).

## New shipped surface → inventory decisions

- **Detection frame ships NO new rendered files.** `--allow-broken-cohort` (CLI flag), `internal/eslink` (internal package), the doctor check, and help-text changes are binary/CLI surface only ⇒ **no bootstrap-inventory ledger entry required** (the ledger governs overlay packs / core capabilities rendered into consumer repos — `internal/cli/bootstrap_inventory_test.go:19-55`). `README.agent.md` must be updated same-change (command-surface rule).
- *(For the record, the rejected qualification frame would have added a committed sidecar registry under `.vh-agent-harness/` — F7-class committed-transport decision, not bootstrap-inventory — an additional surface cost counted in criterion 3.)*

## Contradiction audit vs `researches/decisions/origin-hash-update-sync.md` designed intent

1. **Preservation contract ("preservation is per-file, never clobber") vs D2 refusal — NO REVERSAL.** Refusal blocks *sibling advances* in the apply pass; the preserved file is neither written nor bypassed, and the escape hatch preserves exactly today's behavior. The frame *extends* awareness into enforcement for the demonstrated-broken class only — consistent with, arguably a fulfillment of, the memo's "consumer awareness … must be prominent" risk posture (§:219-222).
2. **Origin-hashes as provenance-not-qualification (`originhash.go:96-103`) — NO CONFLICT, and the verdict *protects* it.** The detection link check is computed from staging bytes + plan (independent of hashes). The rejected qualification frame's digest-identity certification would have leaned origin-hashes toward a qualification role they were designed not to carry — rejecting it keeps the memo's semantics intact.
3. **F-series landed addenda (F3/F6/F7) — consistent.** Both frames build on the landed both-moved WARN (F-series); detection's D3 adds a *separate* check rather than escalating the managed-drift WARN tier, so F-agreements (shared `ClassifyPreserved`, WARN-never-fails-G0c wording discipline) are preserved.
4. **Known-stale doc, unfixed by either frame (flagged, pre-existing):** `templates/migrations/v0.29.0.md:56-59,69-73` promises cohort-consistent advancement that `apply.go`'s per-file mechanics falsify at runtime (documented in the 2026-11-05 source packet). Neither frame edits shipped migration notes; **follow-up**: docs-steward slice to correct the note's cohort promise.
5. **Citation drift (flagged):** prior packet's state-lib gate line cites drifted ~+200 lines (now `state-lib.js:9154/9178`); card's `overlay.go:233-244,274-286` cites are near-miss (substance in `shadow.go:24-55`, `seam.go:952-961`); `m0034`/`m0038` do not exist as repo files (parent-session debate IDs; card `open_questions` is the carrier). This memo uses the corrected cites.

## Findings

- **(finding)**: `substrate.Apply` plans all outcomes before any write; managed writes are verbatim staged copies — the effective tree is fully knowable pre-write. source=`internal/substrate/apply.go:205-264,249-251,597-604`, confidence=high, type=fact
- **(finding)**: `seamApply` already lints staged bytes pre-write; a cohort check slots into the existing position. source=`internal/cli/seam.go:244-270,105-113`, confidence=high, type=fact
- **(finding)**: The corpus module graph contains renames, re-exports, dynamic imports, comment-embedded import text, and cross-dir edges — naive regex scanning is unreliable; a comment/string-aware lexer with tri-state discipline is required. source=`templates/core/.opencode/scripts/forbidden-patterns.js:7,25,32,39; shell-guard-core.js:475; verify-task-registry.js:5-22,582; state-lib.js:223,10256-10346`, confidence=high, type=fact
- **(finding)**: Pure-Go ESM parsing is practical (esbuild existence proof; coder/guts importable library proof); goja's ESM support is undocumented and a full runtime is disproportionate. source=goja README (2026-11), pkg.go.dev coder/guts v1.7.1, esbuild repo, confidence=high (existence) / medium (guts suitability as fallback), type=fact
- **(finding)**: The retention gate is JS-only; no credible Go-side semantic verifier exists within posture (Node breaks static posture; goja can't host node-API surface; Go duplication is O3-rejected). source=`state-lib.js:9154,9178; internal/taskcard/validator_test.go:380-461`, confidence=high, type=fact
- **(finding)**: A Go-side guarantee registry would be independent of preserved JS (binary re-derived every run; overlay collisions fail closed). source=`internal/overlay/shadow.go:24-55; internal/cli/seam.go:952-961`, confidence=high, type=fact
- **(finding)**: Qualification digests must key on canonical pre-substitution corpus bytes or per-project staged re-render — `{{COORDINATOR_DIR}}` sits in executable positions. source=`state-lib.js:748; verify-task-registry.js:140`, confidence=high, type=fact
- **(inference)**: Behavioral qualification of diverged implementations is not credible ⇒ "qualified" reachable only by canonical identity ⇒ preserving adopters sit in UNATTESTED override-steady-state ⇒ qualification is lock-in in effect. source=Q4d+Q4f synthesis, confidence=medium-high, type=inference (the load-bearing adjudication step — labeled judgment in the memo body)
- **(assumption)**: The card's flip condition "actual-failure coverage" means the demonstrated link-skew class (not the foreclosed semantic axis). source=card settled-constraints reading, confidence=medium, type=assumption (explicitly labeled; fallback outcome named if rejected)

## Contradictions

- None detected between the recommended frame and the origin-hash memo's designed intent (audit §1-3 above all resolve consistent).
- Flagged (pre-existing, not introduced here): migration-note cohort promise vs per-file apply mechanics (§4); prior-packet/card citation drift (§5).

## Verification

| Claim | Verifying command/output | Verified |
|---|---|---|
| Apply plans before writing; dry-run returns plan | `apply.go:205-264` (`planOutcome` loop before `:262` write dispatch); `:249-251` DryRun | yes |
| Managed write = verbatim staged bytes | `apply.go:597-604,652` (`os.ReadFile(stagedPath)`→`os.WriteFile`) | yes |
| Pre-write staged linting precedent in seamApply | `seam.go:244-246,259,270` (lint `ps.staging` before `substrate.Apply`) | yes |
| Landed both-moved WARN + doctor escalation | `seam.go:461-512`; `doctor.go:1291-1313,1366-1400` | yes |
| Corpus has renames/re-exports/dynamic imports/comment-imports | `forbidden-patterns.js:7,25,32,39`; `shell-guard-core.js:475`; `verify-task-registry.js:582` | yes |
| Export block with interleaved comments | `state-lib.js:10256-10344` | yes |
| Tokens in executable positions | `state-lib.js:748`; `verify-task-registry.js:140` | yes |
| Retention gate is JS-only (drifted cites corrected) | `state-lib.js:9154,9178` (was 8932 in prior packet) | yes |
| Go validator is structural-only | `internal/taskcard/validator_test.go:380-461` | yes |
| Overlay collision fail-closed | `shadow.go:24-55`; `seam.go:952-961` | yes |
| Origin-hash = provenance not qualification | `originhash.go:96-103`; `apply.go:139-149,325-350` | yes |
| Bootstrap-inventory ledger governs packs/capabilities | `internal/cli/bootstrap_inventory_test.go:19-55` | yes |
| goja = ES5.1+ focus, ESM undocumented | goja README FAQ (fetched 2026-11) | yes |
| Pure-Go ESM parsing practical as library | pkg.go.dev coder/guts v1.7.1 (ImportDeclaration API, CC0) | yes |
| Admission-control prior art (VS Code/OpenClaw/k8s) | code.visualstudio.com v1_36 vsce checks; docs.openclaw.ai/plugins/manage-plugins (snippet-level); k8s blog 2019 | partial (OpenClaw = snippet-level confidence) |

*Undecidable-and-named*: whether goja can parse the corpus's ESM syntax (untested, undocumented — mooted by the not-recommended verdict); whether the operator reads the flip-condition "coverage" comparatively (assumption labeled above, fallback outcome named).

## Staging, promotion, next steps

- **Staged**: `tmp/agent-runs/o4-adjudication/2026-11-07-cohort-consistency-preflight-feasibility.md` (this file). Read-only researcher writes only `tmp/` — same precedent as prior packets.
- **Promotion target (coordinator's one-step move)**: `researches/decisions/2026-11-07-cohort-consistency-preflight-feasibility.md`. **No live docs are modified by this memo.** Follow-on promotion targets *if the coordinator accepts the verdict*: a `docs/ai/` slice for the cohort-link check contract when implemented; `README.agent.md` + doctor help in the same change as D2/D3 (implementation-time, not now).
- **Recommended next specialist**: coordinator fan-in first (frame verdict ratification + memo landing); then `planner` for a compact implementation brief of D1/D2/D3 (`internal/eslink` lexer + seamApply two-phase hook + doctor check), with the v0.29.0 migration-note correction routed as a separate docs-steward slice.

---

## Appended at landing (2026-11-07) — retirement carrier anchors

Landed verbatim from `tmp/agent-runs/o4-adjudication/2026-11-07-cohort-consistency-preflight-feasibility.md`
(coordinator landing batch). The lines below are the decision-record carrier
anchors required for card retirement — the card-id anchor plus each per-item
decision-item handle, verbatim from the closeout's decision-record (do not
paraphrase).

Task-Card: task-2026-11-07t05-01-59-o4-cohort-consistency-preflight-feasibility-research-plan-seam-go-es-module-link-check-semantic-capability-check

decision-item: task-2026-11-07t05-01-59-o4-cohort-consistency-preflight-feasibility-research-plan-seam-go-es-module-link-check-semantic-capability-check#measured_outcome[fnv1a:2c9cb433c9e1f7d4] (curated)
decision-item: task-2026-11-07t05-01-59-o4-cohort-consistency-preflight-feasibility-research-plan-seam-go-es-module-link-check-semantic-capability-check#open_questions[fnv1a:49c2c09ec52ea363] (curated)
decision-item: task-2026-11-07t05-01-59-o4-cohort-consistency-preflight-feasibility-research-plan-seam-go-es-module-link-check-semantic-capability-check#open_questions[fnv1a:276a410b7606f844] (curated)
decision-item: task-2026-11-07t05-01-59-o4-cohort-consistency-preflight-feasibility-research-plan-seam-go-es-module-link-check-semantic-capability-check#open_questions[fnv1a:80a105e7b578ae50] (curated)
