# Source Packet: Origin-Hash Cohort Skew (stale-preserved state-lib.js vs advanced sibling docs)

> **STAGING NOTE:** intended durable path is
> `researches/sources/2026-11-05-origin-hash-stale-preserve-skew.md`.
> The researcher session is read-only (writes allowed only under `./tmp/` —
> same precedent as `researches/decisions/origin-hash-update-sync.md`
> §"Recommended durable artifact path", which was staged under tmp/ for the
> same reason). Landing this file at the durable path is a one-step move for
> the coordinator/build slice.

- **Date:** 2026-11-05
- **Type:** source packet (fact base for a later design slice; NOT a decision memo)
- **Reporter class:** adopter-reported defect (their repo, commits `064334c` / `f01e71f` — not visible from here)
- **Time-sensitivity:** LOW for the code-path facts (verified against working tree at HEAD 2026-11-05); MEDIUM for the tracker verdicts (backlog/defer cards mutate)
- **Source policy:** repo canon first — code is ground truth over doc prose; every load-bearing claim carries file:line or command evidence; inferences labeled. No external sources needed (the defect class is entirely local to this codebase's update seam).
- **Recency requirement:** current working tree; the retention trilogy (`b788b43` → `bb60068` → `cccf1b0`, all 2026-10-26/27, shipped v0.29.0) is the relevant corpus cohort.

## Research question

When `vh-agent-harness update` preserves a locally-modified `platform_managed`
file (`.opencode/scripts/state-lib.js`) via the origin-hash three-way check
while its sibling docs/schema advance to the v0.29.0 corpus, the adopter repo
ends up documenting a hard gate (the decision-record retention gate) that its
runtime does not enforce. What exactly happens today at the update/doctor
seams, where is the gate actually enforced, can the adopter's local fixes be
inferred, and is this skew class already tracked?

## Scope

- Q1: current preservation + surfacing behavior of `update` and `doctor`
- Q2: retention-gate enforcement seams (JS vs Go) and what remains enforced under a stale preserved state-lib.js
- Q3: inferability of the adopter's two defect-fix commits; what to request
- Q4: prior art across trackers
- Out of scope: code changes, the design decision, adopter communication, backlog edits.

---

## Q1 — Current preservation behavior (code-path trace)

### What happens on `update`

1. **(finding)**: source=internal/substrate/apply.go:399-461 + internal/managedfile/managedfile.go:153-183, confidence=high, type=fact — For a `platform_managed` path whose live bytes differ from BOTH the recorded origin hash AND the staged (incoming corpus) bytes, `planOutcome` routes to `ActionManagedDiverged` with typed `PreservedReason: consumer-edit` (decision tree line: `hadOrigin + otherwise → ConsumerEdit` at managedfile.go:182; the self-heal branch `live.Hash == stagedHash → ""` at :179-180 does NOT fire in this scenario). The file is preserved, never clobbered; the prior origin entry is carried forward unchanged (apply.go:325-330).
2. **(finding)**: source=internal/cli/update.go:261-265 → internal/cli/install.go:437-467, confidence=high, type=fact — The preservation is NOT silent in the live-update output: `summarizePreservedPaths` prints `preserved/stalled managed file(s) — N path(s) NOT overwritten; run \`vh-agent-harness accept-platform <path>\` to adopt the platform version…` followed by `[consumer-edit] <paths>` grouped by typed reason. `--dry-run` adds a per-path section `Would PRESERVE (consumer-edited managed)…` with re-baseline guidance (internal/cli/guide.go:375-386).
3. **(finding)**: source=rg over internal/ (`versions behind|cohort|sibling` — zero output-string hits), confidence=high, type=fact — **No output anywhere names the functional consequence.** No message states that the incoming corpus version differs from the preserved file's base (platform moved on), nor that sibling docs/schema DID advance in the same update and may reference behavior the preserved file lacks. The messages name only the recovery verb (`accept-platform`) and the promotion alternative (overlay pack). guide.go:379's "accept the platform's new version for one path" is the closest hint and appears only in dry-run prose.
4. **(finding)**: source=internal/cli/doctor.go:1073-1105 (re-render into staging), :1260-1291 (byte-compare live vs staged + shared classifier), :1332-1360 (report), confidence=high, type=fact — **`doctor` DOES compare the preserved file against the CURRENT embedded corpus**: `checkManagedDrift` re-renders the corpus into staging and byte-compares every managed path. For the consumer-edit case (`live != staged`, `live != origin`, hadOrigin) it routes through the SAME shared `managedfile.ClassifyPreserved` (F1 agreement) and reports a **non-failing `tierInfo`**: `N consumer-preserved edit(s) (origin-hash: update preserves your edit)` + path list. The fact that the preserved file also differs from the CURRENT corpus (the skew signal) is the branch's entry condition but is not stated in the message, does not escalate to WARN, and never fails doctor.
5. **(finding)**: source=internal/cli/seam.go:1156-1159, confidence=high, type=fact — `state-lib.js` is NOT in `regeneratedPlatformPaths` (only `.opencode/repo-configs/allowed-commands.js` and `opencode.jsonc`), so it is fully preservation-eligible. Its origin is tracked in this repo at `.vh-agent-harness/origin-hashes.json:109`.
6. **(finding)**: source=researches/decisions/origin-hash-update-sync.md:219-222 (risks) and :245-275 (implemented-policy addendum incl. clone-retention at ~:273), confidence=high, type=fact — The design memo named this exact risk class at design time: *"Consumer awareness of preserved files. A silent skip can hide a consumer from the fact that a platform update did not reach their edited file. The report-surface … must be prominent, not just a debug log."* The implemented surface (typed listing + doctor INFO) exists but stops short of consequence-naming; no follow-up tracker was ever filed for the gap.
7. **(contradiction — doc vs code)**: source=templates/migrations/v0.29.0.md:56-59 and :69-73 vs internal/substrate/apply.go:445-447, confidence=high, type=fact — The migration note's "What `update` handles for you — Re-renders `state-lib.js`, the task-card schema (v3 anyOf branch), the `task-closeout`/`task-delete` command docs, `CLOSEOUT_TEMPLATE.md`, and `RECORD_LIFECYCLE.md` with the decision-record preservation contract" and Watch-out 1 ("The first `/task-closeout` after update refuses a closeout that lacks the fenced block") implicitly assume the whole cohort advances together. For an adopter with a preserved state-lib.js, `update` does NOT re-render state-lib.js while it DOES render the docs/schema — both migration claims are falsified at runtime in that state. The note carries no preserved-file caveat. (The rollback section caveat (a) at :106-110 shows the doc set KNOWS gate presence is render-bound — "the corpus reverts with it — the preservation gate disappears" — but never applies that knowledge to the preserved-file update path.)

### Doc-vs-code delta (itself a finding)

- **(finding)**: source=templates/migrations/v0.29.0.md:44-59 vs apply.go:445-447, confidence=high, type=inference — The migration-doc cohort assumption + the origin-hash preservation feature together CREATE the skew: preservation is per-file, docs promise per-cohort. Nothing in the update/doctor surface reconciles the two.

## Q2 — Gate enforcement seams

1. **(finding)**: source=templates/core/.opencode/scripts/state-lib.js:9006-9044 (closeout gate), :6188+ (`Verify every declared slot of a v3 decision_record against committed, branch-reachable git state`), :7054+ (task-delete retirement verification incl. v1/v2 legacy + force), confidence=high, type=fact — **The retention gate's runtime enforcement is JS-only**, living in the rendered `state-lib.js`. The closeout gate fires inside `saveCoordinationTaskCloseout` (state-lib.js:8932, exported at :10029/:10120), reached via opencode's plan-state tool (`.opencode/tools/plan-state.js`) and the `/task-closeout`/`/task-delete` command flow.
2. **(finding)**: source=internal/taskcard/validator.go:1-60, :315-358 + internal/cli/taskcard.go:21-22, :49-98, confidence=high, type=fact — The Go `taskcard` package is a **structural schema validator** (bounded draft-07 subset + ack-pair guard). It validates the `decision_record` SUBSCHEMA shape on a card, but enforces NEITHER the closeout requirement NOR retirement reachability. Its only production caller is `vh-agent-harness task-card validate [<file>|stdin]` — an **opt-in, INFORMS-only diagnostic** ("No filesystem writes, no side effects"). It is not wired into update, doctor, or preflight: rg for `decision_record|retention|state-lib` over internal/cli/doctor*.go returns nothing.
3. **(conclusion)**: source=synthesis of Q2.1+Q2.2, confidence=high, type=inference — **For an adopter whose state-lib.js is preserved at a pre-v0.29.0 base, the documented retention gate has ZERO runtime enforcement.** The Go binary never enforces closeout/retirement; the only enforcement lives in the stale JS. The updated schema will structurally accept v3 records, the updated docs demand the block, but nothing refuses a closeout lacking it and nothing verifies retirement carriers. The documented gate is fully absent at runtime in that state — exactly what the adopter's commit NOTE honestly discloses ("retirement is not runtime-gated yet").
4. **(finding — compounding hazard)**: source=rg import scan + git log -S, confidence=high, type=fact (mechanism) — **15+ rendered files import from `./state-lib.js`** (verify-task-registry.js, plan-save.js, plan-resolve.js, plan-list.js, approve-plan.js, draft-plan.js, session-bind.js, verify-session-state.js, verify-recurrence-dedup.js, verify-skill-proposals.js, verify-f3-task-ready.js, verify-state-validation.js, verify-coordinator-adoption-marker.js, tools/plan-state.js, plugins/session-state.js). The trilogy changed state-lib.js and verify-task-registry.js in lockstep (`cccf1b0` stat: state-lib.js ±2432 lines, verify-task-registry.js ±48); `collectDecisionItems` was added by `cccf1b0` to state-lib's export surface AND verify-task-registry's import list (`git log -S 'collectDecisionItems' -- templates/core/.opencode/scripts/` → `cccf1b0` only).
5. **(inference)**: source=Q2.4 mechanism + adopter state, confidence=medium, type=inference — Under cohort skew (stale state-lib.js + advanced sibling importers), an ES-module named import of a post-stale symbol is a **link-time error** — the entire importing script fails to load. If the adopter's update advanced verify-task-registry.js while preserving state-lib.js, `verify-task-registry.js` (and any other importer referencing post-base symbols) breaks outright. This is stronger than a doc-vs-runtime gap: it is a runtime break of the verification tooling itself. Confirm against the adopter's actual file states.

## Q3 — The adopter's local fixes

1. **(finding)**: source=mission brief (coordinator-verified), confidence=high, type=fact — The two commits `064334c`, `f01e71f` are defect fixes in the ADOPTER's repo history (not visible from this repo); their commit message carries an honest NOTE that retirement is not runtime-gated yet.
2. **(inference)**: source=NOTE wording + git history, confidence=medium, type=inference — Their state-lib.js base predates `b788b43` (gate v1, 2026-10-26): "not runtime-gated yet" is only true of a pre-v0.29.0 corpus, and the skew scenario (docs at v0.29.0, script preserved) puts their base at ≤ v0.28.x with local patches on top.
3. **(finding)**: source=evidence audit, confidence=high, type=fact — **The defect class their fixes addressed is NOT inferable from available evidence.** Our history offers candidate classes (registry resilience: `186bff1`/`19d36d3`/`e709756`; F3 envelope validation: `d9d5b26`; v0.24 adopter findings F1/F3 classes) but nothing discriminates between "backport of an upstream fix" and "novel local fix". Per the evidence tier rule, no speculation beyond this line.
4. **(must-request list)** — from the adopter (coordinator handles comms):
   - `git show 064334c f01e71f` (full diffs + commit messages) — pins the fix class;
   - `git log --oneline --follow -- .opencode/scripts/state-lib.js` in their repo — pins their base;
   - their `.vh-agent-harness/lineage.yml` — harness version + render identity at last clean update;
   - (optional) their `.vh-agent-harness/origin-hashes.json` entry for state-lib.js — confirms the recorded origin their divergence is measured against.
5. **(inference)**: source=synthesis, confidence=medium, type=inference — **Upstreaming removes the root cause of THIS skew instance either way**: if their fixes duplicate upstream fixes already in the corpus, `accept-platform .opencode/scripts/state-lib.js` + `update` carries them (local patch becomes unnecessary); if novel, porting into `templates/core/` (domain-free) or an overlay pack makes them canonical so the file stops diverging. The generic hazard (any future local edit re-creates skew invisibly) is independent and needs the Q1/Q4 surfacing fix regardless.

## Q4 — Prior art / tracking (verdict per tracker)

| Tracker | Verdict | Evidence |
|---|---|---|
| `researches/decisions/origin-hash-update-sync.md` | **adjacent-but-different** | Design intent + implemented-policy addendum. §Risks :219-222 names "Consumer awareness of preserved files" as a design-time risk; the implemented report surface exists but consequence-naming was never filed as a follow-up. Closest prior art. |
| `.vh-agent-harness/release-defer-dispositions.json` `defer-degraded-repair-savepath-backstop-test` (~:210-220) | **adjacent-but-different** | Its v0.20.0-arc trigger fired on `path_touched(templates/core/.opencode/scripts/state-lib.js)` because state-lib was re-rendered — but the card is a test-coverage gap for the degraded-card repair backstop, not skew. |
| `.local/coordinator/tasks/agents-md-compose-dryrun-blind-spot.json` (S2) | **adjacent-but-different** | Same preserve/blind-spot family, different mechanism: composeAgentsMd overwrites live-only AGENTS.md hunks; root AGENTS.md gets no origin-hash protection. Compose path, not update-preserve path. |
| `docs/checkpoints/2026-08-12-v024-adopter-findings-confirmation.md` (+ P1-CORE-032, done) | **adjacent-but-different** | Prior adopter-findings triage precedent (vh-solara, vh-video-maker, TrueAI); F2-narrow covered origin-hash tracking of root AGENTS.md (fixed). No skew finding. |
| `docs/planning/backlog.md` + `docs/planning/archive/` | **absent** | rg for skew/behind/sibling/cohort/preserv over backlog + archive: no row tracks preserved-file vs current-corpus skew. |
| `docs/ai/postmortem-convention.md` registry | **absent** | No incident class row for this class. |
| `docs/checkpoints/` (all) | **absent** | No checkpoint names the skew class. |

**Net:** the skew class is UNTRACKED; the design memo's risk note is the only place the concern was ever written down, and it was considered addressed by the preserved-path listing.

---

## Verdict

**Mixed, per surface:**

1. **Preservation behavior itself: documented-intended** (origin-hash three-way skip; decision memo OPT-A, implemented; not adopter misuse — the adopter did exactly what the mechanism is designed to permit).
2. **Skew surfacing: DEFECT (b)** — silent-skew in the WARNING-QUALITY sense: the preservation is listed and doctor reports it, but (i) no surface names the functional consequence (preserved file is behind the incoming corpus while sibling docs/schema advanced in the SAME update), (ii) doctor renders the state as non-failing `tierInfo` indistinguishable-at-a-glance from healthy, and (iii) the v0.29.0 migration note actively promises the opposite of what happens ("update re-renders state-lib.js with the gate"; Watch-out 1's refusal never fires). The documented gate is therefore fully absent at runtime for this adopter (Q2.3), with an additional link-time break hazard for sibling importers (Q2.4/2.5).
3. **Not already-tracked (not (c))** — absent from backlog/checkpoints/postmortem registry; adjacent-but-different items only (Q4 table).

## Contradictions

- **migration-note cohort promise vs per-file preservation** (templates/migrations/v0.29.0.md:56-59, :69-73 vs internal/substrate/apply.go:445-447): the note promises cohort advance on `update`; the apply path preserves per-file divergence. Both cannot hold for a preserved state-lib.js. HIGH confidence, code is ground truth.
- **doctor's tiering vs its own knowledge**: doctor computes `live != staged` (knows the file is behind the current corpus) yet reports "update preserves your edit" at INFO tier with no stale-vs-corpus distinction (doctor.go:1260-1291 vs :1344-1360). Not a logic error — a reporting-scope choice that hides the skew signal.
- **rollback caveat self-awareness**: the same migration note's rollback section (:106-110) correctly states gate presence follows the corpus render ("the preservation gate disappears") — the doc set had the knowledge needed to caveat the update path and did not apply it there.
- No contradictions found within the shared-classifier contract (F1: update and doctor agree on preserved-vs-genuine) — that contract holds; the gap is in message content and tiering, not classification.

## Findings (structured register)

- **(finding)**: source=internal/substrate/apply.go:445-447 + internal/managedfile/managedfile.go:153-183, confidence=high, type=fact — diverged-from-both platform_managed file → `ActionManagedDiverged`/`consumer-edit`, preserved, prior origin carried forward.
- **(finding)**: source=internal/cli/install.go:437-467 + internal/cli/update.go:261-265, confidence=high, type=fact — live update prints a typed preserved/stalled listing naming paths and `accept-platform`; not silent.
- **(finding)**: source=internal/cli/guide.go:375-386, confidence=high, type=fact — dry-run prints per-path PRESERVE section with re-baseline guidance.
- **(finding)**: source=rg internal/ (no hits for versions-behind/cohort/sibling in output strings), confidence=high, type=fact — no surface names the functional consequence of the skew.
- **(finding)**: source=internal/cli/doctor.go:1073-1105, 1260-1291, 1332-1360, confidence=high, type=fact — doctor compares preserved files against the CURRENT embedded corpus but reports consumer-edit as non-failing INFO without the stale-vs-corpus distinction.
- **(finding)**: source=internal/cli/seam.go:1156-1159, confidence=high, type=fact — state-lib.js is not platform-regenerated; fully preservation-eligible.
- **(finding)**: source=researches/decisions/origin-hash-update-sync.md:219-222, confidence=high, type=fact — design memo flagged preserved-file awareness as a required prominent surface; implemented partially.
- **(finding)**: source=templates/core/.opencode/scripts/state-lib.js:9006-9044, 6188+, 7054+, confidence=high, type=fact — retention gate enforcement is JS-only (closeout + retirement paths).
- **(finding)**: source=internal/taskcard/validator.go + internal/cli/taskcard.go:21-98, confidence=high, type=fact — Go validator is structural, opt-in, INFORMS-only; never wired into update/doctor/preflight.
- **(inference)**: source=synthesis, confidence=high, type=inference — with a pre-v0.29.0 preserved state-lib.js, the documented retention gate has zero runtime enforcement anywhere.
- **(finding)**: source=import scan + git log -S 'collectDecisionItems', confidence=high, type=fact — 15+ files import state-lib.js; the trilogy changed export surface and importers in lockstep; partial preservation creates ES-module link-time break potential.
- **(inference)**: source=import mechanism + adopter state, confidence=medium, type=inference — sibling importers may fail to load entirely under skew (needs adopter file-state confirmation).
- **(finding)**: source=evidence audit, confidence=high, type=fact — adopter fix class NOT inferable without their diffs; must-request list defined (Q3.4).
- **(inference)**: source=synthesis, confidence=medium, type=inference — upstreaming their fixes (or accept-platform when duplicative) removes this instance's root cause; generic hazard needs the surfacing fix.
- **(finding)**: source=Q4 tracker sweep, confidence=high, type=fact — skew class untracked; nearest items adjacent-but-different.

## Verification

| Claim | Verifying command/output | Verified |
|-------|--------------------------|----------|
| Diverged-from-both → preserved, typed reason | `internal/substrate/apply.go:445-447` returns `ActionManagedDiverged` + `PreservedReason`; tree at `internal/managedfile/managedfile.go:176-182` (`live.Hash != origin` + not self-heal → `ConsumerEdit`) | yes |
| Update output lists preserved paths | `internal/cli/update.go:263-265` calls `summarizePreservedPaths`; body at `internal/cli/install.go:437-467` | yes |
| No consequence-naming output exists | `rg -n 'versions behind|cohort|sibling' internal/ -g '*.go'` → only code comments (atomic-write "sibling temp file", overlay prose), zero user-facing strings | yes |
| Doctor compares preserved vs current corpus, reports INFO | `internal/cli/doctor.go:1260-1291` (live vs staged + ClassifyPreserved → consumerPreservedEdit), `:1359-1360` (`tier: tierInfo`) | yes |
| state-lib.js not regenerated | `internal/cli/seam.go:1156-1159` (`regeneratedPlatformPaths` = allowed-commands.js + opencode.jsonc only) | yes |
| state-lib.js origin tracked | `rg -n 'state-lib' .vh-agent-harness/origin-hashes.json` → line 109 | yes |
| Retention gate JS-only | `templates/core/.opencode/scripts/state-lib.js:9006-9044` (closeout requires block), `:6188+`, `:7054+` (retirement verify); `saveCoordinationTaskCloseout` at `:8932` | yes |
| Go validator opt-in only | `internal/cli/taskcard.go:49-98` (`task-card validate`, "INFORMS only"); rg `decision_record|retention|state-lib` over `internal/cli/doctor*.go` → no output | yes |
| Trilogy lockstep + import-contract change | `git show cccf1b0 --stat` (state-lib ±2432, verify-task-registry ±48); `git log -S 'collectDecisionItems' -- templates/core/.opencode/scripts/` → `cccf1b0` only | yes |
| 15+ state-lib importers | `rg -l 'from "./state-lib.js"' templates/core/.opencode/` → 15 files listed | yes |
| Migration note promises cohort advance | `templates/migrations/v0.29.0.md:56-59` ("Re-renders state-lib.js … with the decision-record preservation contract"), `:69-73` (Watch-out 1) | yes |
| Skew class untracked | `rg -i 'skew|behind|sibling|cohort' docs/ -l` → unrelated hits only; backlog/archive/defer sweep per Q4 table | yes |
| Defer card state-lib trigger is unrelated | `.vh-agent-harness/release-defer-dispositions.json:210-220` (test-coverage card, trigger fired on re-render) | yes |

## Ranked fix-shape options (input for the design slice — NOT a decision)

Ranked by leak-prevention value / implementation risk / adopter friction:

1. **Update-time stale-preserve warning naming the functional consequence.** In `summarizePreservedPaths` (and the dry-run PRESERVE section), split the `consumer-edit` set: paths where the STAGED hash also differs from the recorded origin (three-way "both moved" — platform advanced AND consumer edited) get a distinct warning: platform version of `<path>` changed since your base; sibling docs/schema in this update document behavior it lacks; diff → `vh-agent-harness diff`; re-baseline → `accept-platform`; promote edit → overlay pack. Value: high (fires at the exact moment skew is created). Risk: low (message-only; the hashes are already computed in planOutcome). Friction: low.
2. **Doctor cohort-consistency escalation.** In `checkManagedDrift`, when a consumer-preserved path ALSO differs from the current staged corpus, escalate that subset from INFO to a distinct WARN-tier signal (e.g. `preserved-stale-vs-corpus`), keeping the sanctioned-preserved non-failing contract for the still-current subset and keeping the shared-classifier F1 agreement intact (tier split only, classification unchanged). Value: high (detection at every doctor/preflight run; catches skew created before fix 1 existed — including THIS adopter). Risk: low-medium (must not regress the non-failing preserved contract). Friction: low.
3. **Upstream the adopter's fixes.** Blocked on Q3.4 must-request diffs. If duplicative → runbook is `accept-platform` + `update`; if novel → port into `templates/core/` (domain-free constraint) or an overlay pack. Value: high for this adopter (removes the local edit that causes the skew). Risk: medium (needs diffs; domain-free review). Friction: medium.
4. **Re-sync runbook for preserved files** (docs/ai/ + guide pointer): diff local vs platform, port fixes forward onto the new base or accept-then-reapply, promote canonical parts to overlays. Value: medium. Risk: very low. Friction: medium (manual).
5. **Go-side double-wiring of the retention gate.** Deepest generic fix and highest risk: two enforcement implementations (JS + Go) that can drift — the exact drift class this repo's `managedfile` shared-classifier work exists to prevent. A narrower variant worth considering instead: a doctor **link-contract lint** asserting the rendered state-lib.js exports every symbol the current corpus's sibling scripts import (catches Q2.4/2.5 breakage cheaply, no semantic duplication). Value: variant medium-high. Risk: full version high / variant low. Friction: low.

**Cross-option note:** 1+2 are complementary (creation-time + detection-time) and both cheap; 3 resolves this instance; 5-full is the only option that survives arbitrary future JS skew and is also the only one with real drift risk. The narrow link-lint variant of 5 stacks well with 1+2.

## Recommended next steps

- **Artifact type:** sources (this packet). The option comparison above is scout material for the design slice, not a decision.
- **Next specialist:** `debate`/`planner` on fix-shape choice (1+2 vs +5-variant), then `build` slice; `docs-steward` for the migration-note caveat if the design slice adopts a warning.
- **Promotion targets (if/when adopted):** `internal/cli/install.go` (`summarizePreservedPaths`), `internal/cli/guide.go` (dry-run PRESERVE section), `internal/cli/doctor.go` (`checkManagedDrift` reporting tiers), `templates/migrations/` note for the release carrying the fix; README.agent.md same-change rule applies for any new output surface.
- **Adopter request (coordinator):** the four items in Q3.4.
