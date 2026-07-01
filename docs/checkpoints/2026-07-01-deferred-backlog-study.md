# 2026-07-01 — Deferred Backlog Candidates — Study Decision

> **Decision checkpoint.** A read-only study pass assessed each deferred
> backlog candidate against the shipped machinery before any promotion to
> `docs/planning/backlog.md`. This file records the outcome so the deferrals
> are durable knowledge rather than implicit neglect. No `backlog.md` rows are
> added by this decision; revisit triggers below name the conditions that
> justify a fresh study before promotion.

## Title

Deferred Backlog Candidates — Study Decision (2026-07-01).

## Context

After completing the capability-installer (5 phases) + release pack + cleanup
arc (commits `15c0887`→`c79622cf`), each deferred backlog candidate was studied
before any promotion to `docs/planning/backlog.md`. A read-only researcher pass
assessed each against the shipped machinery.

## Decision

**None of the Group-1 design items are promoted to the active backlog.** All
are obviated or premature at current scale (1 embedded overlay pack, 3
capabilities, 0 custom-adapter consumers). The render-time guards
(`DetectShadowing`, `validateRenderedRefs`, present-agent filter) + the
preset/union selection model cover every realistic case.

## Group 1 — larger items (all DEFERRED or REJECTED)

1. **Overlay-unit `text/template` + `.tmpl` convention** — DEFER. Zero
   consumers need in-unit capability gating today (pack-granular render
   suffices). ~30 LOC, opt-in, low risk. Revisit trigger: a pack needs a unit
   that adapts on co-selected capabilities. (Also the unblocker for item 5.)
2. **Auto-derive lint (callable-graph↔manifest)** — DEFER. Render already
   prunes broken edges. Residual = doc-vs-permission drift (cosmetic at 1
   pack). Revisit trigger: a 2nd embedded pack ships; put the lint in `doctor`.
3. **Mutex/path-collision detection** — REJECT (redundant). `DetectShadowing`
   (render-time) + Provides-uniqueness (catalog) already fail-closed.
   `validateOutputPaths` no-op in `internal/resolver/merge.go` remains the
   forward hook.
4. **Tag-toggle** — REJECT (premature). 3 capabilities + working preset/union
   model = no grouping pain. Revisit at ~10+ capabilities.
5. **Spine/adapter file separation (release pack)** — DEFER. Blocked by item 1
   (no template includes in overlays). Zero custom-adapter consumers;
   single-file + Model X shadow good enough for v1. Revisit trigger: item 1
   lands OR a real custom-adapter consumer appears.

## Group 2 — advisory DROPs

- Fixed in the housekeeping commit landing alongside this checkpoint: IG-1
  (`merge.go:124` dangling comment), A-F1 (`profile_preset_test.go:197` sort
  hand-roll → `sort.Strings`).
- Rejected as noise/unconfirmed: IG-2, A-F3, D-F3.
- Already resolved by `c79622cf`: A-F2, B-F1, D-F1, D-F2.

## Rationale

The deferrals were well-chosen — promoting any Group-1 item now would
re-introduce correctly-cut scope. This checkpoint records the decision so the
deferrals are durable knowledge. When a revisit trigger fires, re-study the
item against the then-current machinery before promotion.

## References

Capability-installer capstone checkpoint `2026-06-30T17-37-35`; release-pack
checkpoint `2026-06-30T19-50-46`; cleanup commit `c79622cf`.
