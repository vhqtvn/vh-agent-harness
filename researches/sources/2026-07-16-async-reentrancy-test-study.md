# Async Re-entrancy Test — Feasibility Study

**Subject:** DEFER finding from C10 command-repetition toast (commit `73cdd89`) —
"add async re-entrancy test for coordination-hints Anti-spam publish-before-await"
**Card:** `.local/coordinator/tasks/task-2026-07-15t18-51-55-add-async-re-entrancy-test-...json`
**Provenance:** `source:review-defer; studied:2026-07-16` · trigger `path_touched(templates/core/.opencode/plugins/coordination-hints.js)` (FIRED by `73cdd89`)
**Mode:** read-only feasibility study (research). No code changed.

---

## Verdict

**GO.** The async re-entrancy test is mechanically straightforward (~50 lines,
no framework), low-fragility (deterministic via a manually-controlled deferred),
and closes a real regression gap: the publish-before-await invariant is a
2-line temporal discipline that the current pure-predicate verifier is
*structurally incapable* of seeing, so a future refactor that moves
`seen.add()` back after the `await` (exactly the F1 latent bug C10 just fixed)
would pass `verify` today.

## Feasibility

**Yes — the handler is unit-testable in isolation in plain Node.js.**

The plugin's contract is minimal and self-contained
(`coordination-hints.js:32-34`):

```js
export const server = async ({ client, directory }) => ({
    event: async ({ event }) => { /* ... */ },
});
```

- The only client surface the handler touches is `client.tui.showToast(payload)`
  (`coordination-hints.js:18-30`). A mock client is one line:
  `{ tui: { showToast: async (p) => { calls.push(p); } } }`.
- The plugin imports only `../scripts/coordination-hints-lib.js` (pure functions,
  `fs`+`path` only). No OpenCode runtime, no SDK, no framework coupling.
- ESM is already wired: both `templates/core/.opencode/package.json:2` and
  `.opencode/package.json:2` declare `"type": "module"`, so the verifier can
  `import { server } from "../plugins/coordination-hints.js"` exactly as it
  already imports the lib. No bundler, no `--experimental` flags.

**Race simulation is deterministic, not flaky.** Because the handler is `async`
and suspends at `await showHintToast(...)` (`:67` / `:95`), giving the mock a
*manually-resolved* deferred promise lets you pin event #2 to arrive while
event #1 is suspended at the await — with no `setTimeout`, no sleeps, no timing.
This is precisely the ordering-regression shape that OpenCode's own test
AGENTS.md blesses ("Simulating network latency in race-regression tests that
intentionally exercise ordering" is the sanctioned exception to its
no-fixed-sleep rule).

**One isolation caveat.** `shownHintsBySession` and `commandHistoryBySession`
are module-global Maps (`coordination-hints.js:11,16`). A single import gives
one shared map across all cases in the process. Mitigation: use a distinct
`sessionID` per case (or emit `session.deleted` to reset — the handler already
clears both maps on that event, `:36-37`). The pure-predicate tests never
instantiate the plugin, so there is no cross-contamination with existing
coverage.

## Options

| # | Approach | Feasibility | Effort | Fragility | What it proves |
|---|----------|-------------|--------|-----------|----------------|
| **a** | **Mock client + fire-events-without-awaiting** (mock's `showToast` returns a manually-resolved deferred; fire N events as un-awaited promises; release deferred; `await Promise.all`; assert call count) | **High** | **Low** (~50 LoC) | **Low** (deterministic) | Mechanically proves the once/session/key invariant under concurrent re-entrancy. FAILS if `seen.add()` is moved after the `await`. This is exactly the regression the DEFER targets. |
| b | Macro/micro-task scheduling (`queueMicrotask` / `setTimeout(0))` to force interleaving | High | Low | Low-Med | Sub-variant of (a) — just changes *how* the mock's promise resolves. `queueMicrotask` models "second event within same tick" most faithfully. Not a separate option; fold into (a). |
| c | Instrument plugin to export internal state for inspection | Medium | Low-Med | Low (but tests wrong thing) | Only proves *final* Set shape, NOT the *temporal* publish-before-await ordering. The pure-predicate tests already cover final state indirectly. Pollutes the production plugin with test scaffolding. **Reject as primary.** |
| d | Static/lint check (AST rule: "`seen.add()` must not follow an `await` on the same path") | Low-Med | **High** (no lint infra in repo for plugin JS; bespoke AST walker) | **High** (ordering rules are brittle, false positives on legit awaits) | A syntactic approximation only. Catches naive regressions, misses semantic equivalents. **Reject.** |

**Winner: (a)**, with the mock resolving on a manual deferred (the (a)+(b)
hybrid). Extend the existing `verify-coordination-hints.js` rather than spawning
a companion script — it already follows the one-script/many-`verify*()`-fns
pattern (`verify-coordination-hints.js:128-129`), and the new function reuses
the existing `assert()` helper (`:29-33`).

## Recommended shape

Concrete `verifyAsyncReentrancy()` to add to `verify-coordination-hints.js`
(after `verifyRepetitionHints()`, called from `main()`):

```js
import { server } from "../plugins/coordination-hints.js";

function makeCountingClient() {
    const calls = [];
    let releaseToast;                                  // manual deferred
    let pending = new Promise((r) => { releaseToast = r; });
    return {
        client: { tui: { showToast: async (payload) => {
            calls.push(payload); await pending;        // suspends here
        } } },
        calls,
        release: () => releaseToast(),
    };
}

async function verifyAsyncReentrancy() {
    // --- Case 1: command.executed re-entrancy (the C10 race) ---
    // Prime counts to 2 with two distinct files that normalize to ONE identity,
    // so the 3rd event crosses threshold and the 4th is the re-entrant rival.
    const { client, calls, release } = makeCountingClient();
    const handler = (await server({ client, directory: "/sandbox" })).event;
    const SID = "race-cmd";
    const ev = (args) => ({ event: { type: "command.executed",
        properties: { sessionID: SID, name: "pytest", arguments: args } } });

    await handler(ev("tests/unit/a.py"));              // count 1 -> no hint (sync return)
    await handler(ev("tests/unit/b.py"));              // count 2 -> no hint (sync return)
    const p3 = handler(ev("tests/unit/c.py"));         // count 3 -> hint -> SUSPENDS at await
    const p4 = handler(ev("tests/unit/d.py"));         // count 4 -> hint -> must dedup (key already reserved by p3)
    release();                                          // let the suspended showToast resolve
    await Promise.all([p3, p4]);
    assert(calls.length === 1,
        `command.executed re-entrancy must fire exactly one toast; got ${calls.length}`);

    // --- Case 2 (optional): session.diff re-entrancy across two concurrent
    // diff events whose hint sets overlap -> assert shared keys fire once. ---
    // (Use a distinct sessionID; build a sandbox dir as main() already does.)

    console.log("async re-entrancy verification: ok");
}
```

**Why this is deterministic:** after `const p3 = handler(ev(...))`, the handler
has run synchronously up to `await showHintToast`, the mock's `showToast` has
pushed one payload, and p3 is suspended. `p4` then runs synchronously; with the
fix it hits `seen.has(hint.key)` → `true` (`coordination-hints.js:57`) and
returns without calling `showToast`; *without* the fix (the F1 regression) the
key is absent and `showToast` is called a second time. So `calls.length` is
`1` (fixed) vs `>=2` (regressed) — a clean pass/fail that needs no timers.

**Note on the regression-detection claim:** to *prove* the test actually catches
the F1 regression (not just passes vacuously), the implementer should temporarily
revert `seen.add(hint.key)`/`shownHintsBySession.set(...)` to *after* the await
locally, confirm the assertion fails, then restore. One-time validation, not
committed.

## Coverage note (what was read)

- `templates/core/.opencode/plugins/coordination-hints.js` (full, 107 lines) —
  both event branches, the `shownHintsBySession`/`commandHistoryBySession` maps,
  the publish-before-await discipline (`:60-68` cmd branch, `:83-96` diff
  branch), the `session.deleted` reset (`:35-39`).
- `templates/core/.opencode/scripts/verify-coordination-hints.js` (full, 224
  lines) — established pattern: ESM, manual `assert()`, sandbox in `tmp/`,
  `verifyRepetitionHints()` + `main()` orchestration. Confirms it tests lib
  predicates only — never instantiates the plugin.
- `templates/core/.opencode/scripts/coordination-hints-lib.js` (full, 327
  lines) — pure predicates already covered; `buildRepetitionHint` key stability
  across counts 3..5 (`:289-303`) is what makes the per-session Set dedup sound.
- Commit `73cdd89` full plugin diff — confirms the F1 fix: old code did
  `await showHintToast` THEN `seen.add`, and published the Set only AFTER the
  loop; C10 inverted both (reserve-before-await, publish-before-loop).
- `refs/opencode/` plugin test fixtures — `test/fixture/tui-plugin.ts`
  (`createTuiPluginApi`, TS/Bun/Effect, SDK+`@opentui` coupled) and
  `test/fixture/plugin.ts` (`markPluginDependenciesReady`, just writes
  package-lock). **No lightweight plain-JS plugin harness exists** in OpenCode;
  bespoke verifier scripts are the repo norm (`verify-session-state.js`,
  `verify-state-validation.js`, `verify-task-registry.js` all follow the same
  plain-script shape).
- `refs/opencode/packages/opencode/test/AGENTS.md` — confirms "race-regression
  tests that intentionally exercise ordering" are a sanctioned pattern (and that
  published-signal beats fixed sleeps; here we need neither — manual deferred).
- ESM context: `templates/core/.opencode/package.json:2` + `.opencode/package.json:2`
  both `"type": "module"` — plain `node` runs the verifier + plugin import as-is.
- DEFER card (full) — `success_criteria: ["[object Object]"]` is a serialization
  bug (stringified `[Object: null prototype]`); the real criterion lives in
  `validation_plan` ("verify-coordination-hints.js passes with new async
  re-entrancy cases"). Worth fixing when the card is promoted.

## Contradictions / stale guidance
- None detected. The DEFER card's stated mechanism (fake client + overlapping
  events + assert one toast) matches option (a) exactly — the card author and
  this study agree on the shape.

## Confidence
**High.** The plugin's async handler is a plain async function with a single
mockable client method, running in an ESM context that already executes under
`node`. The race window is controllable with a manual deferred — no timing
assumptions. The only non-trivial detail (module-global map isolation) has a
clean mitigation (distinct sessionID per case / `session.deleted` reset).

Key evidence: `coordination-hints.js:32-34` (contract), `:18-30` (mock surface),
`:60-68` & `:83-96` (the invariant under test), `verify-coordination-hints.js:29-33,128-129`
(test pattern), `73cdd89` diff (the regression the test must catch).

## Recommendation: promote now, not hold

The DEFER meets **all** promotion Definition-of-Ready criteria from
`AGENTS.md` (DEFER/follow-up curation):

| DoR criterion | Status |
|---|---|
| Trigger fired | ✅ `path_touched(templates/core/.opencode/plugins/coordination-hints.js)` fired in `73cdd89` |
| Concrete area | ✅ coordination-hints async re-entrancy |
| File scope | ✅ `verify-coordination-hints.js` (extend only; plugin is read-only) |
| Validation plan | ✅ `node .opencode/scripts/verify-coordination-hints.js` passes with new cases |
| Clear slice | ✅ add `verifyAsyncReentrancy()` (~50 LoC, option (a)) |
| Provenance Notes | ✅ `source:review-defer; studied:2026-07-16` |

**Suggested next step (coordinator's call, not the researcher's):**
`/task-ready` the card → `/resume-task` an execution session → implement option
(a) → run the one-time local revert-and-confirm-fail validation → commit
(code commit for the test; the card's `success_criteria` serialization bug can
be repaired via `/task-repair` either before or during the slice).

If the operator prefers to keep the bar higher, the only thing that would change
the verdict to HOLD is evidence that the publish-before-await discipline is
*already* mechanically protected another way — and this study found no such
protection (the verifier is pure-predicate only by design).
