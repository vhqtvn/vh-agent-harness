# Sources: S1 localization-split evidence (VH-Solara, TrueAI)

**Date:** 2026-07-14
**Topic:** First real S1 localization artifacts from two consuming repos, plus the
load-bearing additive-merge overlay-placement finding.
**Decision memo:** none separate — this packet enriches the S1 rule in
`templates/core/.opencode/skills/skill-creator/references/skill-lifecycle.md`
(the placement note added in the same slice).

This packet records the first real consumer-produced S1 localization artifacts
(core discipline skeleton + contracted overlay localization file) for the two
skills shipped this cycle (`tdd-loop` `f56b964`, `debugging-loop` `c52268a`),
and a mechanism finding about where those localization files naturally live. It
**summarizes**; it does not embed the consumer files verbatim (they are
authority-honest to their own repos — embedding them here would duplicate
authority and drift).

## Confidence legend

- **HIGH** — verified against repo source, committed artifact, or reproducible
  behavior.
- **MED** — single-source claim or pilot-reported behavior; not independently
  re-verified in this slice.
- **LOW** — anecdotal; directionally useful only.

## Provenance

- Two consuming repos produced real S1 localization artifacts: **VH-Solara**
  (on-disk, read directly in this slice) and **TrueAI** (relayed inline; their
  repo commit `938ccb0` superseded their earlier overlay-pilot
  `tdd/seam-map.md` + `diagnosing-bugs/recipes.md`).
- Both consumers' localization files target the same two core skills
  (`tdd-loop`, `debugging-loop`) and carry the S1 absence-contract pointer back
  to the core skill.
  - **Confidence: HIGH** — VH-Solara files read directly; TrueAI content relayed
    with provenance (commit `938ccb0`).

## Finding 1 — VH-Solara localization artifacts (first consumer; 6-lane Go/Vitest/Playwright repo)

- **On-disk reference:** `/home/vhnvn/repo/vh-solara/tmp/agent-runs/validate-debug-tdd-skills/{vh-solara-debugging-loops.md (128L), vh-solara-tdd-seams.md (119L)}`.
  - **Confidence: HIGH** — both files read and field-verified in this slice.

### vh-solara-debugging-loops.md (128L)

- **Structure:** a 6 bug-class → seam map, a decision flowchart, and two full
  worked-example traces.
- **Bug-class → seam map fields:** `Example` / `Observable at` / `Red signal
  shape` / `Agent-runnable seam?` / `Loop decision` / `bgshell-job?` / `Recipe`.
  Six classes span GPU/thermal render (WebRender), serial-aggregate cross-test
  contamination (Playwright serial flake), reconnect-storm concurrency (Go
  aggregator), deterministic UI assertion failure, Go domain logic, and
  cross-package Go integration.
- **Worked trace (a) — heat saga DOWNGRADE.** The GPU/thermal WebRender defect
  routes to the `human-observed` label
  (`downgrade-protocol.md:44-45`); all **FIVE** guardrails fire (classify
  correctly, never promote to an agent gate, stop theorizing, route to
  `diagnostics-export`, bgshell-job boundary holds); the loop ends after the
  labeled handoff; bgshell-job is OUT for GPU thermal profiling
  (`downgrade-protocol.md:88-90`).
- **Worked trace (b) — scroll-follow test 11 IN-LOOP.** The
  predeclared-aggregate exception fires with all four sub-conditions:
  predeclared threshold, reproducible count, no cheaper seam (isolation green),
  bgshell-hosted. NOT a downgrade (`downgrade-protocol.md:46-49`).
- **Negative boundary.** Bug-class 4 (test 6, isolation-red) — the
  predeclared-aggregate exception does NOT apply because the cheaper seam
  (isolation) is already red. This is the key negative-space validation that the
  exception does not over-fire on a legitimate cheaper red.
- **Guardrail-count consistency.** The file uses **five** as the guardrail count
  (the `human-observed | non-deterministic | not agent-runnable` label is the
  taxonomy those guardrails operate on, not a sixth guardrail) — consistent with
  the committed `c52268a` skill and the prior pilot-evidence packet.

### vh-solara-tdd-seams.md (119L)

- **Structure:** 6 seams with the full field set (`implements` / `tested-at` /
  `runner` / `accepts` / `promises` / `TDD-fit`), a seam-selection guide, an
  authority-honesty note, and an AGENTS.md co-localization note.
- **The six seams:**
  1. Go unit — co-located `pkg/<pkg>/*_test.go`.
  2. Go integration — `tests/integration/`.
  3. Go e2e (in-process cluster) — `tests/e2e/` via `StartCluster()` at
     `tests/e2e/harness.go:47`.
  4. Go e2e (docker gold) — **explicitly NOT a TDD seam** (regression only; too
     slow for red-green iteration).
  5. Web unit (Vitest) — `node`-default environment, `jsdom` opt-in per file
     via docblock (36 of 52 files).
  6. Web e2e (Playwright) — SERIAL execution (`workers:1`,
     `fullyParallel:false`).
- **Authority-honesty note:** every path/line verified against the live tree on
  2026-07-14; the seam *shapes* are structural/stable, the *paths* are pointers
  that may move.
- **AGENTS.md co-localization note:** ties the seam map to the refined AGENTS.md
  testing section and keeps the two in sync — this is the `P2-SKILLS-004`
  dogfood finding (inherited harness testing boilerplate) made concrete.

## Finding 2 — TrueAI localization artifacts (second consumer; Python/TS web-platform repo)

- **Provenance:** relayed inline (no on-disk path in this repo). Both files carry
  HTML header comments documenting placement + provenance. Their repo commit
  `938ccb0` superseded the earlier overlay-pilot `tdd/seam-map.md` +
  `diagnosing-bugs/recipes.md`.
  - **Confidence: MED** — relayed (single-source); not independently re-verified
    against the TrueAI repo in this slice.

### trueai-tdd-seams.md

- **Structure:** 3 vertical-slice tiers (unit / integration / e2e) with fields
  `implements` / `tested-at` / `runner` / `accepts` / `promises`.
- **Maintenance rule:** references the consumer's `AGENTS.md` testing rules
  rather than redeclaring them (one source of truth).
- **Authority-honest packages list:** 18 packages verified via `ls packages/`
  on 2026-07-14 — `application`, `authz`, `contracts`, `controlplane`,
  `controlplane_policy`, `datasets`, `detectors`, `domain`, `eval_btl`,
  `fusion`, `llm`, `media`, `models`, `observability`, `queueing`, `reports`,
  `review`, `storage`. This is the D3 authority-honesty rule propagated from
  the prior pilot's `billing` self-inconsistency catch (see Contradiction audit).
- **Boundary notes:** distinguishes e2e-verification (a checking lane) from
  debugging-loop (a reproduction lane) from test-led-construction (a TDD lane).
- **Absence-contract pointer:** back to the core `tdd-loop` skill
  (seam absence = step 0/1).

### trueai-debugging-loops.md

- **Structure:** 8 recipes covering: failing unit, failing integration, failing
  e2e, demo API chain (creds sourced from `/workspace/.env.local`, never the
  command line), contracts→producer→renderer, manifest→registry→adapter,
  API→worker→detector→fusion, and CLI+snapshot.
- **Recipe fields:** `Reproduces` / `Command shape` / `Red signal` / `Minimize`
  — each satisfying the 4-property checklist from the core
  `red-signal-recipes.md`.
- **Boundary note + absence-contract pointer:** the e2e-verification boundary is
  noted, and recipe absence points back to the core `debugging-loop` skill as
  step 0/1.

## Finding 3 — Additive-merge overlay placement (load-bearing S1 mechanism finding)

- **The finding.** An overlay directory named **identically to a core skill**
  (e.g. `.vh-agent-harness/overlays/<consumer>/skills/tdd-loop/`) but containing
  **only the localization file** (no `SKILL.md`) **merges additively** into the
  core skill dir on `vh-agent-harness update`: the core's `SKILL.md`
  (byte-identical) and `references/` survive, and the localization file lands as
  a sibling — no shadow, no drop.
- **Why this is load-bearing for S1.** It resolves the open S1 placement
  question: **the consumer's localization file's natural home is an overlay dir
  mirroring the core skill name.** The core keeps the discipline; the overlay
  carries only the repo-specific localization; neither clobbers the other.
- **Validation.** Probed by TrueAI via `--dry-run` + a real `update` on
  2026-07-14: their localization files ship this way and merge cleanly.
- **Mechanism basis (documented).** Overlay unit files render 1:1 into
  `.opencode/` as `overlay_extension`, and — per the consumer-shipped
  `.opencode/commands/harness.md` → "Shadowing rule" — "Overlays ADD new units;
  they do not shadow-and-replace." A pack therefore cannot displace a core
  builtin by naming a dir identically; it contributes a sibling file under a
  **per-file** ownership model (`platform_managed` core files coexist with
  `overlay_extension` overlay files in the same dir). The deeper mechanism lives
  in `internal/overlay/merge.go` (package comment: overlay packs "layer
  additively on top of the curated core corpus," contributing unit files
  "mirroring the `.opencode/` subtree").
  - **Confidence: HIGH** for the mechanism (consumer-shipped doc + TrueAI
    dry-run/real-update probe).
- **Asymmetry (noted for honesty).** VH-Solara's localization files currently
  live under `tmp/agent-runs/` (the validation-artifact location), not yet in an
  overlay. So VH-Solara validates the localization *content* (Finding 1) but has
  not yet exercised the overlay *placement*; the placement model is
  TrueAI-validated and mechanism-documented.

## Cross-cutting observation

Both consumers independently arrived at **three shared disciplines** in their
localization files:

1. **Maintenance rule** — reference the consumer's `AGENTS.md` conventions
   (testing rules, env/cred sourcing) rather than redeclaring them, so there is
   one source of truth.
2. **Authority-honesty** — cite only what `ls` / glob verifies in the current
   repo state (TrueAI's 18-packages list; VH-Solara's "verified 2026-07-14"
   note). This discipline was motivated by TrueAI's own F1 `billing`
   self-inconsistency finding during the prior pilot (a seam-map cited a
   package that did not exist).
3. **Absence-contract pointer** — point back to the core skill so that seam /
   recipe *absence* is step 0/1 of the workflow ("no localization file found →
   construct one before proceeding").

This is the S1 contract working as designed across two diverse repo shapes
(Go/Vitest/Playwright vs Python/TS web-platform), with no central coordination
beyond the core skill skeleton.

- **Confidence: HIGH** — synthesized from Findings 1 and 2 (VH-Solara verified
  directly, TrueAI relayed with provenance commit `938ccb0`); both consumers
  carry the three disciplines explicitly in their localization files.

## Contradiction audit

1. **F1 `billing` self-inconsistency — resolved (propagated).** The prior
   pilot-evidence packet recorded that TrueAI's earlier overlay-pilot seam-map
   cited a `billing` package that did not exist in their repo state — the
   incident that produced the D3 authority-honesty rule. The new
   `trueai-tdd-seams.md` carries an authority-honest 18-packages list verified
   via `ls packages/`, so the D3 rule propagated into the real artifact. No
   active contradiction.
2. **Placement asymmetry — noted, not a contradiction.** VH-Solara's
   localization files are in `tmp/` while TrueAI's ship in an overlay. This is a
   difference in placement maturity, not a factual conflict: both are valid S1
   localization artifacts; the overlay placement (Finding 3) is the recommended
   durable home, validated by TrueAI and documented at the mechanism level.

## Files referenced (verified paths)

- `/home/vhnvn/repo/vh-solara/tmp/agent-runs/validate-debug-tdd-skills/vh-solara-debugging-loops.md` — 128L, read directly this slice (cross-repo).
- `/home/vhnvn/repo/vh-solara/tmp/agent-runs/validate-debug-tdd-skills/vh-solara-tdd-seams.md` — 119L, read directly this slice (cross-repo).
- TrueAI localization files (`trueai-tdd-seams.md`, `trueai-debugging-loops.md`) — relayed inline; no on-disk path in this repo. Their repo commit `938ccb0`.
- `templates/core/.opencode/skills/skill-creator/references/skill-lifecycle.md` — the S1 pattern definition; enriched with the placement note in the same slice.
- `templates/core/.opencode/commands/harness.md` → "Overlay anatomy" / "Shadowing rule" — the consumer-shipped documentation of the additive overlay-merge model.
- `internal/overlay/merge.go` — the deeper additive-layering mechanism (package comment).
- `researches/sources/2026-07-14-skill-craft-pilot-evidence.md` — the prior pilot-evidence packet (F1 `billing` context, guardrail-count basis).
