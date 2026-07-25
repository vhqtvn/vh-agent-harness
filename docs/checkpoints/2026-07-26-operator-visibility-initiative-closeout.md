# Operator-Visibility Initiative — Final Closeout (2026-07-26)

**Status: COMPLETE.** The S2-a three-family operator-visibility gate-set is fully
live and proven. F1 (synthesis-producing), F2 (rendering/persistence), and F3
(design-gate) are all implemented, proven at their crux paths, and dogfooded
end-to-end on real content. The `15ddd54` governance ruling is recorded. This
lane closes; residual work lives in DEFER cards with triggers.

This is the committed reference for the arc's outcome, verdict tokens, open
operator-owned seams, and pointers to every artifact produced. It is a dated
progress snapshot under `docs/checkpoints/`, not canon.

## Origin → arc summary (brief)

The initiative began from the operator's problem: the harness automates AI coding
well but does not resolve **OPERATOR VISIBILITY** — operators don't understand
the underlying codebase or decisions at the lossy boundaries. The arc:

1. a Class-A evidence study of the vh-solara O1→server-owned-tree failure (the
   26–28h pivot latency);
2. derivation of requirements R1–R7 + presentation properties P-a/P-b/P-c;
3. a property map anchoring all controls to shipped seams;
4. a debate settling the S2-a topology (three UNION families by data-role:
   synthesis-producing, rendering, design-gate);
5. three mechanism briefs (F1/F2/F3);
6. the builds;
7. dogfood;
8. governance.

The demonstrated bottleneck was **synthesis + option-generation + persistence**,
NOT display — so F1 (the join/generate/persist) is load-bearing; F2 (render) is
necessary-but-insufficient; F3 (design-gate) is the sole **BLOCKS** family,
preventing the day-0 premature-BUILD-READY failure class (Pattern 5).

## Verdict tokens per family

### F1 — synthesis-producing (R1 cross-lane join, R3 redesign fork, P-a counter-evidence)

- **Crux: `proven/proven`** at `42f6276` (Slice 5) —
  `TestEmitF1_Crux_ProducerToValidatorToEmitEndToEnd` exercises the real producers
  end-to-end (`JoinR1CrossLane` → `GenerateR3Fork` → `GeneratePAProbes` →
  `AssignF1Validation` → `EmitF1` → `ConsumeF1Emit`), lossless digest round-trip.
- Slices 1–4: `inconclusive/not-demonstrable` (producer→validator substrings;
  full crux needed Slice 5).
- **F1 emit is LIVE:** `ValidatedF1Emit` is consumable by F2 (`ConsumeF1Emit` →
  `F2EnvelopeView`, read-only canonical access, 4 derived-field categories
  attachable without entering the digest).
- Commits: `80b8430` (S1), `499385b` (S2), `f9dacd3` (S3), `158cac1` (S4),
  `42f6276` (S5).

### F2 — rendering/persistence (R5 operator-synthesis binding, P-b evidence-grade media, P-c salience)

- **All 10 slices: `proven/proven`.**
- **Pipeline LIVE:** `IngestF1EmitForF2` → `PersistF2Pair` →
  `RenderF2MarkdownProjection` → `ScanF2R1Streak` → `checkF2PairConsistency`
  (doctor f2-pairs check).
- **21 files, +8714 lines** (verified: `git diff --stat 6b47a0a^..7d192525` →
  `21 files changed, 8714 insertions(+), 3 deletions(-)`). All `go test ./...`
  pass, `gofmt`/`vet` clean.
- Commits: `6b47a0a`→`7d192525` (S1–S10).

### F3 — design-gate (DAY-0 adversarial BUILD-READY refusal)

- **Both BUILD-READY crossings: `proven/proven`** blocking the
  named-but-unresolved ownership fixture:
  - Task-card route (`draft→ready`, `074001c`): 19 assertions — named hazard +
    HIGH/BUILD-READY self-assertion + no complete current-digest F3 envelope =
    **refused**; complete envelope = **permitted**.
  - Approved-plan route (`draft-plan→approved`, `e169a98`): 10 assertions — same
    fixture refused; blocked approval leaves no partial artifact.
- Dispatch backstops (`aa4bff5`, ready→working + plan dispatch): `proven/proven`
  (14 assertions — stale-readiness bypass refused).
- Authoring/reviewer surfaces (`cd6f6d6`): `proven/proven` (36 assertions,
  text-contract — disclaim coordinator/reviewer blocking authority).
- Slice 1 validator (`f98136e`): `inconclusive/not-demonstrable` (pure, 189
  assertions; lifecycle integration deferred to S2/S3).
- **Slice 6 (doctor audit): DEFERRED** (plan-sanctioned; retrospective-only;
  primary gate is the lifecycle validator).
- Dogfood/final verify (`3611910`): `proven/proven` (both routes).

### Dogfood — F1→F2 end-to-end on real content

- **`proven/proven`** at `6fec822` — the morning report as the first real F2
  artifact. P-c salience 9/9 (first disclosure layer carries decision frame +
  disposition + counter-evidence + weakest-claim + unresolved gaps + canonical
  binding). Doctor f2-pairs PASS. Artifact:
  `docs/checkpoints/f2/2026-07-25-overnight-f1f2-dogfood.{canonical.json,md}`
  (both files verified present).

## Governance

- **`15ddd54` house-rule standing-policy ruling** (4 conditions: cite house rule +
  predicate clause / own commit / flagged in closeout for post-review /
  outside-class needs pre-approval) — recorded in the F1 memo's Governance
  section (`71efe4e`). Rationale: makes the schema-omission-vs-predicate class
  mechanical, avoids the operator-bottleneck the visibility study named, post-hoc
  review keeps the authority line intact. Operator post-review: APPROVED.
- **b-F1 precedent** (`074001c`): validated as the right class + approach;
  honestly noted it met conditions 1/3/4 but not 2 (bundled, not own commit) —
  formalized going forward.
- **Render-location guard** (`186ba26`): `state-lib.js` refuses to load if its
  bytes contain the unrendered coordinator token (delimiter built at runtime via
  `String.fromCharCode` to avoid self-rendering); `verify-no-unrendered-paths.js`
  regression guard (no `{{` path; no marker file under `templates/`). Proven both
  directions (clean → pass; recreated violation → fail). Fixes the Slice-7
  dogfood defect (ran source scripts with unresolved tokens instead of rendered
  copies).

## Open seams (operator-owned — residual work in DEFER cards with triggers)

The four F1-memo open questions below are the memo's **#3–#6** (the memo's #1–2
were build-detail/source-path items resolved during the build). They are listed
here with the memo's own numbering so a reader can find them in
`researches/decisions/2026-07-25-f1-synthesis-family-and-s2a-topology.md` L296–314.

1. **F1 memo open-Q #3 (stable ancestry identity)** — stub-with-contract: R1
   producer discloses shared ancestry via union-find collapse; does NOT claim
   universal independence. Upgrade if ancestry-independence is later claimed.
2. **F1 memo open-Q #4 (verdict taxonomy mapping)** — stub-with-contract: R3
   producer takes closed-enum inputs (`RepairIntent`/`StructuralReviewOutcome`);
   the repo-verdict→enum mapping is a documented deferred seam.
3. **F1 memo open-Q #5 (P-a high-risk-seam policy)** — baseline shipped
   (`not_run` fails; `unavailable` needs limitation, can't support `proven`);
   the high-risk-seam `unavailable` extension is deferred →
   `review-defer-f1-unavailable-highrisk-policy.json`.
4. **F1 memo open-Q #6 (disposition timing)** — stub-with-contract: R3 gate takes
   a `transitionRequested` flag; exact repo lifecycle transition deferred.
5. **F1 Slice 5 judgment call** — two doctor allow-list BLOCKs read as distinct
   (different mechanism/fix: pure-foreign f2_view fail-open; null f2_view
   silent-pass), both fixed with regression tests. Flagged for operator overrule
   if "two blocks in the same doctor check's allow-list area" should be read as
   "same finding twice."
6. **F3 Slice 6 (doctor audit)** — DEFERRED, plan-sanctioned, retrospective-only.
   Parked at `tmp/agent-runs/f3-build/SLICE-6-DEFERRED.md`.
7. **Dev-stale-binary** — PATH binary (0.15.1) predates f2-pairs (#18); `doctor`
   on PATH doesn't run #18; build-from-source is HEALTHY. Operator:
   rebuild/reinstall when convenient.
8. **Cosmetic F1 memo direction-words** — L129/L181 "above"/"below" swapped at
   two cross-reference qualifiers (non-blocking; section-name references remain
   accurate).
9. **3 absent must-read docs** — `docs/ai/shell-execution.md`,
   `docs/ai/codebase-operational-primitives.md`, `docs/ai/deployment-workflow.md`
   referenced by build-agent instructions but absent (verified: `docs/ai/`
   contains only 6 files, none of these three); shell-guard rules still enforced
   by the plugin + mirrored in `AGENTS.md`.
10. **Releaser.md doctor-check count lag** — `templates/overlays/release/agents/releaser.md`
    says "all 15 checks"; `README.agent.md` is current. Sync separately by the
    release track.

## Pointers — every artifact this arc produced

### Decision memos (`researches/decisions/`)

- `2026-07-25-f1-synthesis-family-and-s2a-topology.md` — F1 mechanism + S2-a
  topology (`9dbab50`, amended `15ddd54` C1, governance `71efe4e`).
- `2026-07-25-f2-rendering-family-mechanism.md` — F2 mechanism (`605f406`,
  amended `4029e42` C4).
- `2026-07-25-f3-design-gate-mechanism.md` — F3 mechanism (`135aa16`, amended
  `074001c` b-F1).
- `2026-07-23-vh-solara-orchestration-field-report-disposition.md` — HYBRID +
  field-report dispositions (canon-promoted `68a8fc4`: F1/F2/F3 union-list
  sub-section).
- `2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md` —
  authority line (canon-promoted `68a8fc4`: Block-BUILD-READY row).

### Source packets (`researches/sources/`)

- `2026-07-25-seven-controls-property-map.md` — the 7-row property map
  (`9dbab50`).
- `2026-07-24-opencode-history-visibility-study.md` — the Class-A evidence study
  (`8600fc8`).

### Dogfood artifacts (`docs/checkpoints/f2/`)

- `2026-07-25-overnight-f1f2-dogfood.{canonical.json,md}` — first real F2 artifact
  (`6fec822`).

### DEFER cards (`.local/coordinator/tasks/` — gitignored transport, NOT committed)

Verified present via `ls .local/coordinator/tasks/` at closeout time:

- `defer-review-leaf-health-preflight.json` — review-tier leaf empty-output
  preflight.
- `review-defer-f1-unavailable-highrisk-policy.json` — open-Q #5 (was #3 in the
  draft; renumbered to the memo's numbering).
- `review-defer-f1-pa-producer-dedup.json` — producer dedup polish.
- `review-defer-f1-f2-persist-attach-helper.json` — F2 persistence helper.
- `defer-006-transient-locator-in-f2-canonical-sidecar.json` — transient tmp/
  locator.
- `defer-010-f3-task-ready-lifecycle-toctou-concurrent-test.json` — TOCTOU
  concurrent test (non-load-bearing).
- `defer-011-f3-task-ready-command-template-envelope-step.json` — **RESOLVED** by
  F3 S5.
- `defer-f6-guard-crux-automated-test.json` — persistent test for render-location
  guard crux. *(Correction: the draft named this `DEFER-F6-...`; the actual
  filename is lowercase `defer-f6-...` — corrected here.)*
- `defer-f7-reporoot-portability.json` — `findRepoRoot` go.mod-only portability.
  *(Correction: the draft named this `DEFER-F7-...`; the actual filename is
  lowercase `defer-f7-...` — corrected here.)*

### Citation chain (durable commits)

study `8600fc8` ← map `9dbab50` ← F1 memo `9dbab50` ← canon `68a8fc4` ← F2 memo
`605f406` ← F3 memo `135aa16` ← F1 governance `71efe4e`. F1 build
`80b8430`→`42f6276`. F2 build `6b47a0a`→`7d192525`. F3 build
`f98136e`→`3611910`. Fix `186ba26`. Dogfood `6fec822`.

## Honesty note

Structural validators prove consistency + coverage, NOT truth. Diagnostics say
"structurally satisfied," NEVER "proven solved." The F3 gate verifies the
resolution PROCESS is structurally satisfied; it CANNOT verify the design is
good, that a cited source is truthful, that reviewer identity-distinction =
genuine independence, or that no unnamed hazard exists. The dogfood morning
report honestly bounds every claim (F1 crux proven at unit/integration,
real-refusal deferred; F2 first real-content cycle; F3 design committed +
implemented, S6 deferred; C1 bounded to this run). Every `proven` token is
backed by an outcome-observed crux exercise, never a mechanism assertion alone.

## Verification

| Claim | Verifying command/output | Verified |
|-------|--------------------------|----------|
| All 9 listed DEFER cards exist | `ls .local/coordinator/tasks/` — 9 of 9 present (2 with corrected lowercase names) | yes |
| F2 = 21 files, +8714 lines | `git diff --stat 6b47a0a^..7d192525` → `21 files changed, 8714 insertions(+), 3 deletions(-)` | yes |
| F1 crux commit `42f6276` = Slice 5 emit | `git show --stat 42f6276` → "F1 ValidatedF1Emit + F2 consumer interface + F1/F2 doctor audit (slice 5/5)" | yes |
| Dogfood commit `6fec822` = first real F2 artifact | `git show --stat 6fec822` → "first real F2 artifact — overnight synthesis morning report (dogfood)" | yes |
| `15ddd54` governance ruling recorded at `71efe4e` | `git show --stat 71efe4e` → "record 15ddd54 house-rule standing-policy ruling" | yes |
| Render-location guard at `186ba26` | `git show --stat 186ba26` → "guard against running unrendered templates/core scripts" | yes |
| Canon promotion `68a8fc4` | `git show --stat 68a8fc4` → "promote S2-a family boundaries + Block-BUILD-READY authority row to canon" | yes |
| F3 final `3611910` = dogfood regeneration (both routes) | `git show --stat 3611910` → "Slice 7 — dogfood regeneration of the F3 design-gate rendered corpus" | yes |
| Dogfood pair artifacts exist | `ls docs/checkpoints/f2/` → both `.canonical.json` and `.md` present | yes |
| F1 memo open-Qs #3–#6 map to closeout seams #1–#4 | `grep` F1 memo L296–314: ancestry(verdict#3)/enum-mapping(#4)/P-a-policy(#5)/disposition-timing(#6) | yes |
| 3 must-read docs absent from `docs/ai/` | `ls docs/ai/` → 6 files, none of shell-execution/codebase-operational-primitives/deployment-workflow | yes |
| F1 build S1 `80b8430` resolves | `git show --stat 80b8430` → "F1 synthesis-envelope vocabulary + pure validator (slice 1/5)" | yes |
