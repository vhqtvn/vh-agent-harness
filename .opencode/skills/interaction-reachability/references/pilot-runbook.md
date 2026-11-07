# Pilot runbook — interaction-reachability (overlay-only S2)

This is a procedural runbook for running a bounded pilot of the
`interaction-reachability` advisory skill and recording the **two-sided** health
measurement (VALUE: interaction-touching changes that would have landed with a
fake/shallow reachability claim now file an honest `not-demonstrable`/`skipped`
or a genuine outcome `proven`; COST: real-runtime fixture provisioning + tracer
latency). It produces **advisory evidence only** — no gate, no
commit/release/doctor/update effect. See `SKILL.md` for the full procedure,
authority line, and the five-condition table; see core `AGENTS.md` → "Behavioral
closure" → "Interaction-reachability receipt" for the authoritative receipt the
tracer/falsifier operates on.

This pilot is **instruction-only**. There is no bundled helper and no bundled
runtime. The tracer product (the six-field receipt) is producible with whatever
real-runtime fixture the adopter repo already operates (a real browser via
Playwright/Cypress, a real device lab, etc.); the skill does NOT provision one.

## S2 hold status

This skill is under the **S2 overlay-pilot-then-promote** hold
(`templates/core/.opencode/skills/skill-creator/references/skill-lifecycle.md`
S2). It is NOT a core skill. Promotion to `templates/core/` requires:

1. at least one pilot against a **real frontend repo** with genuine
   interaction-touching changes (this dogfood repo is a Go CLI with no frontend
   paths and is NOT a valid pilot target);
2. recorded evidence that the tracer/falsifier changed an outcome (a fake
   `proven` downgraded to honest `not-demonstrable`/`skipped`, or a genuine
   outcome `proven` produced);
3. human-approved core promotion.

Until then the skill stays overlay-only and opt-in (`overlays: [frontend-ui-pilot]`
in `vh-harness-profile.yml`). The `-pilot` pack suffix is retained as the
maturity signal.

## 0. Confirm a real-runtime seam is reachable

Before running anything, confirm a real-runtime fixture that dispatches the real
event model is reachable in the pilot repo (a real browser, real focus/click
dispatch through the live DOM — NOT jsdom/headless stand-ins that elide
cross-origin focus or event retargeting). If none is reachable, the honest pilot
result for that slice is `not-demonstrable` (condition 5) — record that, do not
manufacture a `proven`.

## 1. Declare a bounded slice

Record, before running anything (state every field explicitly — use `none` where
it does not apply, so the run ledger has no silent gaps):

- **interaction path** — the real user gesture + handler the change depends on
  (e.g. "click in pane-0 region → host focus handler").
- **runtime-blindspot risk** — why the diff alone cannot certify reachability
  (e.g. "host page embeds a cross-origin iframe that can swallow the focus
  event").
- **real-runtime seam** — the actual environment that dispatches the gesture
  (real browser + driver), NOT a mocked stand-in.
- **expected outcome** — the user-visible result a human would see.
- **time / cost budget** — a wall-clock ceiling for tracer + falsifier work.

## 2. Run Procedure A (tracer) — construct the honest receipt

Walk the builder through SKILL.md → Procedure A (A0–A5). The load-bearing step is
A2: dispatch the REAL gesture and observe the USER-VISIBLE outcome (not "the
handler returned"). Record the six receipt fields + an honest `result`.

## 3. Run Procedure B (falsifier) — challenge the filed receipt

Independently, run SKILL.md → Procedure B against the filed receipt. Record each
advisory finding (B1–B5) and whether the reviewer converted it to a DEFER.

## 4. Record the two-sided measurement

- **VALUE side:** did the skill change the filed `result`? (fake `proven` →
  honest `not-demonstrable`/`skipped`; or a genuine outcome `proven` produced;
  or no change because the receipt was already honest). One concrete delta per
  slice.
- **COST side:** real-runtime fixture provisioning time + tracer/falsifier
  latency. One number per slice.

## 5. Aggregate and decide promotion posture

Across the pilot slices, aggregate the VALUE/COST ledger. Promotion to core
requires net-positive VALUE (the skill observably downgrades fake `proven`
claims or enables genuine outcome observation) at acceptable COST, recorded in
an evidence packet under `researches/sources/`, plus human approval. Absent
that, the skill remains overlay-only.
