# state-lib-source-copy fixture (P1-TESTS-002)

Frozen SOURCE-copy snapshot of the render-location guard crux #1 regression pin
(`TestStateLibRenderGuard` / `TestStateLibRenderGuard_RedControl` in
`../../state_lib_render_guard_test.go`).

## What this is

A byte-identical copy of the harness SOURCE scripts:

- `state-lib.js` — copy of `templates/core/.opencode/scripts/state-lib.js`
- `f3-design-readiness.js` — copy of `templates/core/.opencode/scripts/f3-design-readiness.js`
  (state-lib.js imports it; the sibling must be present so the module loads far
  enough for the render-location guard IIFE to read its own bytes and fire)
- `rewrite-parity-validate.js` — copy of
  `templates/core/.opencode/scripts/rewrite-parity-validate.js`
  (state-lib.js imports it for the closeout Stage 2 rewrite-parity check; same
  reason as `f3-design-readiness.js` — the sibling must be present so the module
  loads. Only required when the snapshot is refreshed from a state-lib.js that
  carries the rewrite-parity import; the frozen snapshot predates that import.)

## Why a frozen copy (not a runtime copy of the live source)

The render-location guard (`assertRenderedNotSource` IIFE in state-lib.js,
shipped in commit 186ba269) reads its OWN bytes at load time and refuses to load
if they contain the literal coordinator-directory template token
`{{COORDINATOR_DIR}}` (resolved only at render time). The guard fires on a
SOURCE copy (token present) and skips on a RENDERED copy (token absent).

The committed snapshot is the artifact under test:

- it is **stable** — decoupled from concurrent edits to the live
  `templates/core/.opencode/scripts/state-lib.js` (a separate SUBSTRATE-lane
  quarantine slice edits that file in a different region; a runtime copy could
  transiently observe a half-written source);
- it is **faithful** — it is the real guard bytes, not a re-implementation or a
  truncated stub (the task explicitly requires a copy of state-lib.js retaining
  the `{{COORDINATOR_DIR}}` token, not a synthesized minimal fixture);
- the guard IIFE itself has been untouched since 186ba269, so the snapshot does
  not drift in the property under test.

## Refreshing the snapshot

If the `assertRenderedNotSource` guard IIFE is intentionally changed (not just
other regions of state-lib.js), refresh BOTH files from source so the snapshot
matches the current guard:

```
cp templates/core/.opencode/scripts/state-lib.js            tests/integration/fixtures/state-lib-source-copy/state-lib.js
cp templates/core/.opencode/scripts/f3-design-readiness.js  tests/integration/fixtures/state-lib-source-copy/f3-design-readiness.js
cp templates/core/.opencode/scripts/rewrite-parity-validate.js  tests/integration/fixtures/state-lib-source-copy/rewrite-parity-validate.js
```

The fixture copy MUST retain the literal `{{COORDINATOR_DIR}}` token (it appears
in the path-construction line `path.join(repoRoot(), ".local", "{{COORDINATOR_DIR}}")`);
the guard builds the token-search delimiter at runtime via char codes so the
renderer never resolves the guard's own condition.
