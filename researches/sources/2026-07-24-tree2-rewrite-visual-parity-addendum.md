<!--
EVIDENCE ARTIFACT — external field-report addendum (consuming-repo: vh-solara).
Captured: 2026-07-24. Source path (consuming repo, volatile): /home/vhnvn/repo/vh-solara/tmp/2026-07-24-tree2-rewrite-visual-parity-addendum.md
Provenance discipline: Class-A (consuming-repo transcripts/commits/code anchors — CANNOT be re-derived in this repo, treated as ASSERTED) + Class-B (this repo's contracts/tooling — verified against actual files in the disposition addendum).
This is EVIDENCE, not canon. Cite as: researches/sources/2026-07-24-tree2-rewrite-visual-parity-addendum.md
-->

---

<!--
  PROVENANCE: ADDENDUM to the 2026-07-23 vh-solara harness-adoption field report,
  addressed to vh-agent-harness maintainers. Produced 2026-07-24 (read-only research;
  no code/harness/commit/backlog/checkpoint/live-doc changes — operator-review-only scratch).

  CLAIM CLASSES (same discipline as the source field report, lines 1-15):
    - Class A (consuming-repo): vh-solara commits, OpenCode transcript rows
      (message/part tables, read directly), session memory, code/test anchors
      (e.g. web/src/styles/legacy/20-session-tree.css:16). These ARE re-derivable
      in vh-solara today (unlike the 2026-07-23 report's Class A, which lived in a
      sibling repo) — but they are still treated as consuming-repo evidence: every
      Class A claim cites a commit SHA, a session_id+timestamp, or a file:line.
    - Class B (harness-structure): the 2026-07-23 field report + its disposition +
      the 2026-07-24 HYBRID addendum (the rec IDs P0-A/P0-B/P1-B/P2-A, the Class-A/
      Class-B discipline, the authority line, the merge/union rule). Verified by
      reading the two committed files in the sibling harness repo.

  RELATIONSHIP: this is an ADDENDUM, not a second report. It proposes 3 harness-layer
  recommendations with explicit MERGE/UNION verdicts under the disposition's own
  HYBRID rule (same property -> MERGE; different property -> UNION). It does NOT
  edit the harness, the disposition, or any live doc. Promoting any of this into the
  sibling harness repo's researches/ is a SEPARATE operator-approved slice and is
  out of scope here. (Carrier constraint: vh-solara carries harness-adoption tracking
  in session-state ONLY — no committed docs ledger in vh-solara; hence this file
  lives under tmp/, non-committed scratch.)

  KNOWN LIMITATION: the OpenCode transcript quotes are excerpted (truncated for
  readability) but each cites its session_id + timestamp so the full text is
  re-derivable. No metadata was trusted on its own — every behavioral claim is
  backed by a commit or a transcript row, per the field report's own method rule.
-->

---

# ADDENDUM (2026-07-24): the rewrite-parity / visual-outcome surface — a fourth symptom cluster the 2026-07-23 field report does not name

**Audience:** vh-agent-harness maintainers (same as the 2026-07-23 report).
**Scope:** the harness *orchestration* layer (done-gate, testing contract, parity
contract) — not vh-solara product code. Domain-free; coordinator non-authoritative;
synchronous — same constraints as the disposition.
**One-line thesis:** a green rewrite shipped, reported "all green / done" overnight,
and silently dropped a pile of user-facing UX behaviors (flat tree, no selected-row
highlight, child-flood, tab-switch flash, no deep-link reveal, dropped pins/density)
that were found ONLY by the operator using the live UI, one at a time, until a forced
parity audit. This is a NEW symptom surface. Under the disposition's HYBRID rule it
resolves to: **MERGE-EXTENDS P0-A** (outcome-vs-mechanism) + **UNION a new
parity-contract rec** (rewrite-preservation, distinct from P1-B motivation) +
**MERGE into P2-A** (a second band-aid-loop instance, which does NOT un-defer it).

---

## Executive summary

The `tree=2` "server-owned session tree" rewrite (the same arc the 2026-07-23 report
studied) shipped to the client and was reported all-green overnight:

- The Phase-3 "Step D" live-verify lane (`ses_06fd0e42…7zliBDTkFUNW`, 2026-07-24
  02:13) closed: *"e2e under tree=2 default: 158 passed, 0 failed, 1 skipped"* and
  *"PART 2 — a–e symptoms (all PASS)"*. That is the "all green / done" report.
- Yet a cascade of parity fixes followed, each found by the operator in the live UI:
  `24c014a` *restore pin + search parity*, `c5a006b` *pins re-expand + nested-pin
  dedup*, `e204779` *gate sidebar render on UI expand-state (stop the child flood)*,
  `cd46198` *restore depth-based tree-guides + selected-row highlight*, plus
  uncommitted `selectedPathIds`/`visiblePathIds` (deep-link ancestor reveal) and
  `selectPinnedNodes` (pins parity) still in flight at research time.
- The irony is load-bearing: `cd46198`'s commit subject itself reads
  *"…(P0-A/P0-B)"* — the fix was labeled with the very rec that should have blocked
  the green-ship.

The dropped behaviors were not exotic: row indentation (tree rendered FLAT —
children looked like roots), selected-row highlight, active-parent child-flood,
tab-switch empty-flash + lost expand-state, deep-link ancestor reveal, pins-drag /
density. None was caught by the green suite. The structural reasons:

1. **The verify scope was new-design criteria, not prior-surface parity.** Step D's
   (a)–(e) checked the NEW design's stated behaviors (first-click loads transcript;
   subagent nested; collapsed chip; reload no-flatten; archived no-ghost). It did not
   inventory what the DELETED proj=1 client used to do. (Root causes a, b.)
2. **Tests assert MECHANISMS, not OUTCOMES.** The `selected` class was applied to the
   inner `.tree-node` while the CSS rule `.tree-row.selected` targets the outer
   `.tree-row` — any "class present" assertion is green; the pixel is unstyled. jsdom
   unit tests query `.tree-node[data-session-id]` DOM presence; they cannot see
   indentation, highlight, or layout. (Root cause c.) The catching tests were written
   WITH the fix, not at ship (`cd46198`: *"4 new TreeRow tests failed on unmodified
   HEAD, GREEN after"*).
3. **Verification was synthetic/isolated and dropped parity as "infeasible."** Step D
   attempted old-client evidence and recorded *"(a) INFEASIBLE (fixture sessions all
   hydrated) … (d) INFEASIBLE (fixture has 4 sessions, not real scale)"* — i.e. the
   one check that could have shown the gap was abandoned, not escalated. (Root cause d.)
4. **Reactive whack-a-mole until a forced audit.** The fixes came one at a time until
   the operator issued a 7-item parity batch at 14:01 (the systematic inventory). (Root cause e.)

These four map to three harness-layer recs, all shaped to respect the authority line
(gates act; coordinator reads), evaluated under the disposition's HYBRID rule below.

---

## Reconciliation: is this new?

**IS-IT-NEW verdict: YES — new symptom surface; resolves to MERGE-EXTENDS P0-A +
UNION (new parity-contract rec) + MERGE P2-A. I agree with the operator's 3-verdict
hypothesis, with two sharpenings (flagged).**

Confirmed against the two committed source-of-truth files in the sibling harness repo
(`researches/sources/2026-07-23-…-field-report.md`, 491 lines; and
`researches/decisions/2026-07-23-…-disposition.md`, 403 lines incl. the 2026-07-24
HYBRID addendum):

- `rg -in "visual|outcome|rewrite-parity|prior-contract|prior-surface|parity-contract|parity.audit"`
  over BOTH files → **no matches.** The terms are absent.
- The report's only "rewrite" mentions are contextual (Pattern 6's *"server-owned-tree
  rewrite"*, Pattern 5's *"missed the entire motivation of the rewrite"*) — i.e. the
  rewrite as *background*, never as a *parity-preservation* concept.
- "parity" appears in the disposition only as **"behavioral-parity-matrix"** (§4.5,
  `defer-behavioral-parity-matrix-full.json`) — which is **route-not-node test-PATH
  coverage** (does a test *reach* the load-bearing runtime path), the SAME property
  family as the P0-A crux. It is NOT rewrite-preservation (does a replacement *keep*
  the old thing's accreted behavior). This is the decisive distinction for Verdict 2.

So the 2026-07-23 report's seven patterns are organized by symptom surface
(green-tests, metadata-reporting, commit-freeze, tangle, design-drop, band-aid,
re-discovery) and **none of them is the visual/UX outcome layer or the
rewrite-preservation surface.** The 2026-07-24 HYBRID addendum's union list
(*"behavioral completion ≠ defer-not-drop ≠ HEAD-progress ≠ motivation-satisfaction
≠ findings-retention"*) does not include rewrite-parity either.

**Sharpening vs the operator's framing (two flags):**

1. **Verdict 1 — the gap is only PARTIALLY gestured by the existing token≠crux
   caveat, so MERGE-EXTENDS (not already-closed).** The HYBRID addendum (lines
   ~382-391) already says the verdict/crux token *"does not prove the crux path was
   actually exercised… Proving the crux still requires repo-specific live
   verification — the docker-gold pattern: seed real data, induce the real failure,
   observe the fix."* That gestures at "observe the fix." But it does NOT name (i) the
   mechanism≠outcome decoupling WITHIN a green lane (class applied to the wrong
   element; DOM-present ≠ pixel-styled), (ii) that jsdom web-unit and Playwright
   DOM-assertions are structurally MECHANISM-only for the visual layer (no layout /
   paint engine), or (iii) that for a *rewrite* specifically, "observe the fix" must
   be a *visual diff against the prior surface* — which loops into Verdict 2. So this
   is a genuine EXTENSION of the same property (behavioral/outcome completion), not a
   rival signal. A reviewer could argue the caveat already covers it; the rebuttal is
   the cd46198 evidence: the crux PATH ran (the elements rendered) and the a-e verify
   PASSED — the gap was purely the outcome-vs-mechanism axis the caveat never names.

2. **Verdict 2 — the existence of the word "parity" in fix commits does NOT mean a
   parity CONTRACT existed.** The P2 lane honestly labeled its work "parity"
   (`24c014a` *restore pin + search parity*) and even DEFERRED density + path-reveal
   (`ses_06f5d5d5c…ZkCCE9QfATM2`, 03:43). But that parity was AD HOC and
   operator-named (the operator found each gap live); there was no PRIOR-SURFACE
   INVENTORY authored before the old client was deleted. The distinction is between
   "some fixes were retroactively called parity" (true) and "a contract required
   enumerating the old surface + verifying each before ship" (false). The latter is
   the gap.

---

## Relationship to existing recs

| This finding's root cause | Existing rec (disposition) | Relation | Verdict |
|---|---|---|---|
| (c) tests assert mechanisms not outcomes; (d) no real-UI outcome gate | **P0-A** behavioral done gate (crux/load-bearing-path clause + verdict token `proven\|inconclusive\|failed\|abandoned` + `crux: proven\|skipped\|not-demonstrable`) | SAME property (behavioral/outcome completion); sharpens the crux clause to require OUTCOME-verification and to name the mechanism≠outcome decoupling + the visual layer | **MERGE-EXTENDS P0-A** (the token≠crux caveat gestures but does not name it) |
| (a) "done" never included parity; (b) old surface deleted with no inventory | **P1-B** motivation-satisfaction (forward: does the NEW design meet ITS goals) + §4.5 behavioral-parity-matrix (test-PATH coverage, crux family) | DIFFERENT property (backward: does the replacement PRESERVE the old thing's accreted behavior); different input (prior-surface inventory, captured BEFORE deletion) + different verifier (parity audit / visual diff vs prior surface) | **UNION — new rec** (parity-contract); not collapsible into P1-B or §4.5 |
| (e) reactive whack-a-mole until forced audit | **P2-A** recurrence detector (Pattern 6 band-aid loops) | SAME property (recurrence → step-back); a SECOND Class A instance, at the UX/visual layer (the original was state/reconcile) | **MERGE into P2-A** (strengthens evidence; does NOT un-defer — still blocked on `defer-002`) |

---

## Root cause (a) — "done" was defined as new-design-criteria + named-bugs + suite-green, never parity

### Evidence
- **E9** (Class A): the Step D live-verify lane closeout (`ses_06fd0e42dffevg7zliBDTkFUNW`, 2026-07-24 02:13) reports *"e2e under tree=2 default: 158 passed, 0 failed, 1 skipped"* and *"PART 2 — a–e symptoms (all PASS): (a) first-click loads transcript ✓ (b) subagent stays nested ✓ (c) collapsed node shows chip + right-clickable ✓ (d) reload preserves structure ✓ (e) archived session disappears ✓."* This is the "all green / done" report. The verify scope = the NEW design's stated behaviors only.
- **E6** (Class A): the planned Phase 3d verify scope was authored in the handoff (`.opencode/state/sessions/server-owned-tree-client/memory/handoffs/2026-07-23T14-59-49-…`) as *"(a) first-click loads transcript; (b) subagent stays nested; (c) collapsed shows chip+right-click; (d) reload no flatten; (e) archived no ghost."* New-design criteria. None of: indentation, selected-highlight, expand-state persistence, deep-link reveal, pins-drag, density.
- **E10** (Class A): the systematic parity inventory came from the OPERATOR, not any lane. At 2026-07-24 14:01 the coordinator (`ses_0733c95c4ffeiYE91DLVRAJgeT`) received the user directive: *"Fix these seven… P0-A — INDENTATION / tree-guides (the tree renders FLAT → children look like roots)… P0-B … P0-C … P0-D …"* — a forced 7-item parity batch. The inventory was authored only after the operator used the live UI.

### Harness-layer root cause
The done-gate (P0-A's verdict token + crux clause) answers *"did the change do what it
set out to do"* — i.e. meet the NEW design's stated goals (motivation, P1-B territory)
and exercise its load-bearing path. It has no clause that asks *"did the replacement
PRESERVE the behavior the old thing accreted."* A rewrite can be motivation-satisfied +
suite-green + crux-proven and still drop un-named accreted behavior, because nobody
was required to enumerate the prior surface. The old `SessionTree.tsx` was DELETED in
Phase 3c (coordinator transcript 14:03: *"the old SessionTree.tsx was DELETED in Phase
3 Step C (926→73 lines)"*); once deleted, the prior surface cannot be re-derived from
current code. The inventory must be captured BEFORE deletion, and nothing in the
contract requires it.

### Fix (gate/template-shaped; coordinator reads, gate acts)
- **A "prior-surface inventory" contract clause for any change that REPLACES or
  DELETES a user-facing component.** Before the old thing is removed, the lane must
  enumerate its accreted behaviors (functional AND visual/UX) and, for each, name the
  parity verification the new thing must pass. This is a closeout/contract field, not
  a coordinator power.
- **Enforcement is gate-shaped (mirrors the disposition's P0-A/P5 conversion):** a
  closeout/commit-gate check refuses a closeout that replaces/deletes a flagged
  component (e.g. `SessionTree.tsx`, a primary view) without a populated
  parity-contract section. The gate acts; the coordinator reads. (See authority-line
  table below.)

### Verdict — feeds a NEW rec, UNION (see root cause (b))
Root cause (a) is the *trigger condition* for the parity-contract rec defined under
(b); it is not a standalone MERGE into P0-A. The *property* (rewrite-preservation)
is distinct from behavioral completion (P0-A) and from motivation-satisfaction (P1-B).

---

## Root cause (b) — no prior-surface inventory; the old behavior was deleted without a parity contract

### Evidence
- **E12** (Class A): the coordinator re-derives the gap at 14:03 (`ses_0733c95c…E91DLVRAJgeT`): *"The CSS ALREADY HAS `.tree-guides`, `.tg-cell`, `.tg-cell.rail`, `.tg-connector`, `.tg-connector.last` (lines 24-43)! These are leftover from the old SessionTree (deleted). So the guide styling infrastructure EXISTS — I just need TreeRow to render the guide cells."* The old CSS *survived* the rewrite; the old MARKUP did not — and nothing flagged the orphaned CSS as "behavior the new thing must reproduce."
- **E1** (Class A): `web/src/styles/legacy/20-session-tree.css:16` = `.tree-row.selected { background: color-mix(in srgb, var(--accent) 15%, transparent); }` and `:21` = `.tree-row.selected .tree-title { color: var(--accent); font-weight: 600; }`. The `cd46198` commit message: *"The `.tree-row` div also lacked the selected class the CSS (`.tree-row.selected`) targets, so selected rows were not highlighted."* The rewrite put `selected` on the inner `.tree-node`; the CSS targets the outer `.tree-row`. No contract required the new markup to match the surviving selector.
- **E5** (Class A): the parity-fix code itself speaks in parity language retroactively — `web/src/sync/treeSelectors.ts:3-6`: *"This module restores the parity the deleted proj=1 client had (PINS + SEARCH) but implemented against the NEW flat Map."* Parity was reconstructed AFTER the fact, component by component.

### Harness-layer root cause
The harness has **no "before you delete the old thing, inventory its behavior"
contract.** The closest existing notion — §4.5 behavioral-parity-matrix — is about
test-PATH coverage (route-not-node: does a test *reach* the load-bearing runtime
path), which is the crux/P0-A property family, NOT rewrite-preservation. P1-B
(motivation) is forward-looking. Neither covers "enumerate the old surface and prove
the new one reproduces it." Because the inventory is absent, parity becomes an
ad-hoc, operator-discovered property: each gap surfaces only when a human notices a
missing pixel.

### Fix — propose a NEW rec: **P0-C (or P1-C) rewrite-parity contract** (UNION)
- **Property:** rewrite-preservation — does a replacement/deletion preserve the prior
  surface's accreted behavior (functional + visual/UX)?
- **Input (distinct from P1-B and §4.5):** a **prior-surface inventory** authored
  BEFORE the old component is deleted — enumerate the old behaviors, including the
  visual/UX ones (indentation, highlight, expand-state, reveal, drag, density), each
  tied to the CSS/markup/interaction that produced it.
- **Verifier (distinct):** a **parity audit** — for each inventoried behavior, a
  check that the new thing reproduces it. For visual behaviors this is a visual diff
  against the prior surface (real-UI observation; NOT jsdom DOM-presence, NOT
  Playwright DOM-assert alone). Where the verifier cannot be satisfied in a synthetic
  fixture (Step D's "INFEASIBLE" rows), the gap must **defer-not-drop** (persist +
  block "complete" until a capable verifier runs or the operator explicitly accepts
  the regression) — reusing P0-A's defer-not-drop discipline, not inventing a new one.
- **Both motivation (P1-B) AND parity (new) must pass** ⇒ UNION, fail-closed. A
  rewrite that meets its new goals but drops old behavior is not "done."

### Verdict — **UNION (new rec)**, per release-defer-dual
Distinct property from behavioral-completion (P0-A), motivation-satisfaction (P1-B),
defer-not-drop, HEAD-progress, and findings-retention — the five already in the
HYBRID addendum's union list. Add **rewrite-parity ≠** to that list. Do NOT collapse
it into P1-B (forward vs backward) or §4.5 (test-path vs behavior-preservation).

---

## Root cause (c) — tests assert MECHANISMS, not OUTCOMES

### Evidence
- **E1** (Class A, repeated for this root cause): the `selected` class WAS applied (to
  the inner `.tree-node`) — a "class present on an element" assertion is green — but
  the CSS rule `.tree-row.selected` (`20-session-tree.css:16`) targets the OUTER div,
  so the pixel was unstyled. Mechanism green; outcome broken.
- **E2** (Class A): `cd46198` commit message: *"The tree=2 TreeRow rendered flat —
  children sat at the same offset as roots because depth only toggled a `.sub`
  boolean class."* The `.sub` class was present (mechanism); the visual offset was
  absent (outcome). The fix *"port[s] the old `.tree-guides` markup (rail/connector
  cells whose CSS already existed)."*
- **E3** (Class A): `cd46198`: *"Tests: RED-first (4 new TreeRow tests failed on
  unmodified HEAD), GREEN after — full web unit suite 1041 pass."* The tests that
  would have caught both gaps were authored **as part of the fix**, not at ship. At
  ship, no test asserted indentation depth or `.tree-row.selected`.
- **E4** (Class A): the green tests that DID exist assert DOM structure, not visual
  outcome. `web/tests/unit/tree2Flood.test.tsx:74-78` `renderedIds()` =
  `container.querySelectorAll(".tree-node[data-session-id]")` (presence in the DOM
  tree); `web/tests/unit/tree2VisiblePath.test.tsx:62-66` identical. jsdom has no
  layout/paint engine — it cannot see indentation, highlight, or offset. The assertion
  is "is the element rendered" (mechanism), never "does it look right" (outcome).

### Harness-layer root cause
The testing contract (AGENTS core) treats a green lane as sufficient proof of behavior
(the 2026-07-23 report's RC1, confirmed). It does not distinguish *"the test asserts
the mechanism"* (class applied / element present / token schema-valid) from *"the test
observes the OUTCOME"* (pixel styled / behavior manifest / real-data shape). The
sanctioned seams for the visual layer — jsdom web-unit (no layout), Playwright DOM-
assertions (no visual parity vs prior) — are mechanism-only by construction, and
nothing in the contract names that limitation. So a green suite can coexist with a
visually broken product, exactly as the 2026-07-23 report's Pattern 1 showed a green
suite coexisting with an un-exercised runtime crux — but on a DIFFERENT axis
(outcome, not path).

### Fix (gate/template-shaped) — sharpens P0-A's crux clause
- **Extend the P0-A crux clause to require OUTCOME-verification and to name the
  mechanism≠outcome decoupling.** A lane declaring `crux: proven` for a behavior whose
  value is user-visible must name an OUTCOME-observation (real-UI / visual / real-data
  check), not a mechanism-assertion (class present / DOM-present / token-valid). The
  clause must explicitly flag that jsdom unit + Playwright DOM-assert are
  MECHANISM-only for the visual layer, so a `proven` resting on them for a visual
  behavior is invalid.
- **Reuse, do not rival:** this feeds the SAME verdict token (P0-A). green-tests and
  diff-review still FEED the token; they do not become a parallel signal. The
  sharpening is to the *crux clause's definition of "exercised,"* adding the
  outcome/mechanism axis the token≠crux caveat gestures at but does not name.

### Verdict — **MERGE-EXTENDS P0-A**
Same property (behavioral/outcome completion). The HYBRID addendum's token≠crux
caveat (*"the token… does not prove the crux path was actually exercised… observe the
fix"*) gestures at this but does not name (i) the green-lane-internal
mechanism≠outcome decoupling, (ii) the visual layer, or (iii) that the sanctioned
visual seams are mechanism-only. This EXTENDS the caveat; it does not rival the token.

---

## Root cause (d) — verification was programmatic/synthetic/isolated; no real-UI/real-data/human-look gate; parity dropped as "infeasible"

### Evidence
- **E9** (Class A, the decisive nuance): the Step D lane did NOT silently skip parity
  — it ATTEMPTED old-client evidence and recorded, honestly: *"PART 3 — old-client
  evidence: (a) INFEASIBLE (fixture sessions all hydrated) (b) INFEASIBLE (fixture
  deterministic, no race) (c) SOURCE-CODE EVIDENCE (StubNode omits AgentChip +
  menuTriggers) (d) INFEASIBLE (fixture has 4 sessions, not real scale) (e)
  INFEASIBLE (fixture archive is clean/synchronous)."* The one check that could have
  exposed the gaps was dropped-as-infeasible in the synthetic fixture, not escalated.
- **E11** (Class A): the P2 parity lane (`ses_06f5d5d5c…ZkCCE9QfATM2`, 03:43) deferred
  the harder parity items: *"density = deferred… path-reveal = deferred — auto-
  expanding a collapsed ancestor chain on deep-link needs multiple server
  fetchChildren round-trips."* Those deferred items became the LATER reactive fixes
  (path-reveal = the uncommitted `selectedPathIds`/`visiblePathIds` work at research
  time). Defer without a blocking verifier let "done" land with known-open parity holes.

### Harness-layer root cause
The sanctioned verification lanes are synthetic/isolated by design (web-unit jsdom,
Playwright vs a 4-session fixture, in-process e2e, docker-gold with a fake LLM). For
runtime behaviors the docker-gold pattern can seed real data and induce the failure
(the 2026-07-23 report's recommended crux probe). But for VISUAL/PARITY behaviors the
synthetic fixture *structurally cannot* reproduce the prior surface (it has no old
client, too few sessions, no real scale) — so the check is recorded INFEASIBLE and
dropped. There is no rule that *"if the sanctioned verifier cannot observe a
load-bearing outcome, the lane must defer-not-drop and block 'complete'."* The
harness's honesty (the lane reported INFEASIBLE, not "passed") is correct; the gap is
that INFEASIBLE did not block.

### Fix (gate/template-shaped)
- **"Verifier-capability" sub-clause on the P0-A crux (MERGE) and on the parity
  contract (UNION, root cause b):** when the sanctioned seam cannot observe the
  outcome (synthetic fixture cannot show the old client / cannot render at scale),
  the finding MUST route to defer-not-drop (persist + block "complete") until a
  capable verifier runs (real-UI observation, a visual-diff harness, or an explicit
  operator-accepted regression recorded as such). INFEASIBLE-in-the-fixture is a
  `crux: not-demonstrable` declaration, not a green.
- This reuses P0-A's already-adopted defer-not-drop discipline (the disposition's
  highest-leverage, lowest-cost item) and the `not-demonstrable` crux state — no new
  mechanism.

### Verdict — **MERGE-EXTENDS P0-A** (and feeds the parity-contract UNION under (b))
Same property as (c): outcome-completion. The fix is a sub-clause of the P0-A crux
definition (verifier-capability) plus a parity-contract input (root cause b). No new
property; no coordinator authority (the gate blocks, the coordinator reads).

---

## Root cause (e) — reactive whack-a-mole until a forced systematic parity audit

### Evidence
- **E7** (Class A): the post-"green" commit cascade, each a separate live-found fix:
  `24c014a` *restore pin + search parity* → `c5a006b` *pins re-expand + nested-pin
  dedup* → `e204779` *gate sidebar render on UI expand-state (stop the child flood)* →
  `cd46198` *restore depth-based tree-guides + selected-row highlight* → uncommitted
  `selectedPathIds`/`visiblePathIds` (deep-link reveal) + `selectPinnedNodes` (pins
  parity) STILL in flight. Each touches the same files
  (`SessionTree.tsx`/`TreeRow.tsx`/`treeSelectors.ts`).
- **E10** (Class A): the systematic inventory was forced by the OPERATOR at 14:01 (the
  7-item parity batch), not surfaced by any lane/gate — exactly the 2026-07-23 report's
  Pattern 6 shape (*"the synthesis… appears only in the redesign brief… framed by a
  human… after the band-aids, not surfaced by the harness mid-loop"*), now at the
  UX/visual layer instead of the state/reconcile layer.
- **E8** (Class A): `cd46198`'s subject cites *"(P0-A/P0-B)"* — the team is AWARE of
  the rec IDs but applied them to the FIX, not the ship gate.

### Harness-layer root cause
Same as Pattern 6 / P2-A: no cross-session recurrence detector that notices *"N fixes
to the same component / same failure-family (parity/restore) in K days"* and
auto-raises a "step back / parity audit" task before another symptom fix is
dispatched. The 2026-07-23 report filed exactly this gap and the disposition DEFERRED
it (blocked on `defer-002` symptom-signature-stability; §4.5, §8.3) as the one pattern
with thin Class-A evidence (single instance).

### Fix
No new fix. This is a SECOND Class A instance of Pattern 6, at a new layer. It
STRENGTHENS the evidence for P2-A (addresses the disposition's *"Pattern 6 evidence
too thin — the one pattern load-bearing on un-re-derifiable Class A evidence; defer"*
concern). Note: this instance has a CLEANER recurrence signature than the original —
the fixes cluster on `SessionTree.tsx`/`TreeRow.tsx`/`treeSelectors.ts` with
"parity"/"restore" in commit subjects — which slightly de-risks `defer-002`, but not
enough to resolve it.

### Verdict — **MERGE into P2-A** (does NOT un-defer)
Same property (recurrence → step-back). The DEFER verdict stands: P2-A remains blocked
on `defer-002`. This addendum adds a second Class A instance to P2-A's evidence base;
it does not change P2-A's lifecycle. Stated explicitly so no one reads this as
un-blocking the recurrence detector.

---

## Authority-line engagement (engaged, not silently violated)

Same line as the disposition (§5): *coordinator state informs; safety-layer gates
act.* Any fix that would give the coordinator transition authority is converted to a
gate/template shape, exactly as the disposition converted P2/P3/P5.

| Proposed fix (raw form) | Why it could cross the line | Gate-shaped conversion |
|---|---|---|
| "coordinator must verify parity before accepting the rewrite" | gates a design from proceeding | a **closeout/commit-gate check** that refuses a closeout replacing/deleting a flagged component without a populated parity-contract section + verifier-capability note — the gate acts, the coordinator reads (mirrors the disposition's P5 conversion) |
| "coordinator must reject a `proven` resting on a mechanism-only seam for a visual behavior" | enforces what the lane may declare | a **crux-clause definition** (P0-A testing-contract clause): outcome-vs-mechanism is part of what "exercised" means; the verdict token carries it — the gate/token act, the coordinator reads |
| "coordinator must block 'complete' when the parity verifier is INFEASIBLE" | blocks a done state | a **defer-not-drop rule** (already adopted, RC7/P0-A): INFEASIBLE-in-the-fixture → `crux: not-demonstrable` → blocks "complete" — the gate acts, the coordinator reads |

The fixes that do NOT cross the line (the prior-surface-inventory contract field, the
verifier-capability sub-clause, the P2-A evidence strengthening) are adopted/deferred
on their merits without conversion.

---

## Evidence / Provenance

| Claim | Verifying artifact / command | Verified |
|---|---|---|
| Terms `visual\|outcome\|rewrite-parity\|prior-contract\|parity-contract\|parity.audit` absent from both 2026-07-23 canonical files | `rg -in …` over the sibling-harness field-report + disposition → "NO MATCHES" | yes |
| "parity" in disposition = behavioral-parity-matrix (route-not-node test-PATH coverage), not rewrite-preservation | disposition lines 182, 295-297; §4.5 | yes |
| CSS `.tree-row.selected` targets the outer div; rewrite put `selected` on inner `.tree-node` | `web/src/styles/legacy/20-session-tree.css:16,21`; `cd46198` commit message | yes |
| Indentation flat-render: `depth` only toggled `.sub`; guides markup dropped (CSS survived) | `cd46198` commit msg; `20-session-tree.css:24-26` (`.tree-guides`/`.tg-cell`); coordinator `ses_0733c95c…E91DLVRAJgeT` 14:03 | yes |
| Catching tests authored WITH the fix, not at ship | `cd46198`: "4 new TreeRow tests failed on unmodified HEAD, GREEN after" | yes |
| Green tests assert DOM presence, not visual outcome (jsdom, no layout) | `tree2Flood.test.tsx:74-78`, `tree2VisiblePath.test.tsx:62-66` (`querySelectorAll(".tree-node[data-session-id]")`) | yes |
| Explicit parity language in fix code | `web/src/sync/treeSelectors.ts:3-6` ("restores the parity the deleted proj=1 client had") | yes |
| Planned Phase 3d verify = new-design a-e, NOT prior-surface parity | handoff `.opencode/state/sessions/server-owned-tree-client/memory/handoffs/2026-07-23T14-59-49-…` | yes |
| "All green / done" overnight report = Step D closeout | `ses_06fd0e42dffevg7zliBDTkFUNW` 02:13 ("158 passed 0 failed; a-e all PASS") | yes |
| Parity comparison ATTEMPTED then dropped as INFEASIBLE in synthetic fixture | `ses_06fd0e42…7zliBDTkFUNW` 02:13 ("old-client evidence: (a)(b)(d)(e) INFEASIBLE") | yes |
| P2 lane deferred density + path-reveal (became later reactive fixes) | `ses_06f5d5d5c…ZkCCE9QfATM2` 03:43 ("density=deferred; path-reveal=deferred") | yes |
| Reactive cascade + forced operator parity audit | commit chain `24c014a…cd46198` + uncommitted; operator directive `ses_0733c95c…E91DLVRAJgeT` 14:01 ("Fix these seven… P0-A INDENTATION…") | yes |
| Fix labeled with the rec that should have prevented green-ship | `cd46198` subject cites "(P0-A/P0-B)" | yes |
| Session memory stubs empty; carrier = session-state only (no committed ledger in vh-solara) | `.opencode/state/sessions/{server-owned-tree-client,harness-adoption-tracker}/memory/` (stubs) | yes |

(Transcript quotes are excerpted for readability; each cites session_id + timestamp so
full text is re-derivable from `~/.local/share/opencode/opencode.db` read-only.)

---

## Findings

- **(finding)**: the `tree=2` rewrite shipped green and reported "all green / done"
  overnight while silently dropping user-facing UX behaviors — source=`ses_06fd0e42…7zliBDTkFUNW` 02:13 + commit chain `24c014a…cd46198`, confidence=high, type=fact
- **(finding)**: the dropped behaviors were not caught because the verify scope was
  new-design criteria (a-e), not prior-surface parity — source=Step D closeout + Phase 3d handoff, confidence=high, type=fact
- **(finding)**: tests asserted MECHANISMS (class applied / DOM present) not OUTCOMES
  (pixel styled); jsdom + Playwright-DOM are structurally mechanism-only for visuals — source=`cd46198` msg + `20-session-tree.css:16` + `tree2Flood.test.tsx:74-78`, confidence=high, type=fact
- **(finding)**: the parity check was ATTEMPTED and dropped as INFEASIBLE in the
  synthetic fixture rather than escalated — source=Step D closeout "old-client evidence INFEASIBLE", confidence=high, type=fact
- **(finding)**: the systematic parity inventory was forced by the OPERATOR (live UI),
  not surfaced by any lane — a second Pattern-6 instance at the UX layer — source=operator directive `ses_0733c95c…E91DLVRAJgeT` 14:01, confidence=high, type=fact
- **(finding)**: under the HYBRID rule this resolves to MERGE-EXTENDS P0-A + UNION
  (new parity-contract rec) + MERGE P2-A — source=disposition HYBRID addendum + property-identity test, confidence=medium, type=inference (the verdicts are this researcher's synthesis, not adjudicated by the maintainers)
- **(finding)**: the token≠crux caveat PARTIALLY gestures at outcome-verification but
  does not name the mechanism≠outcome decoupling, the visual layer, or that the
  sanctioned visual seams are mechanism-only — source=HYBRID addendum lines ~382-391, confidence=medium, type=inference

## Contradictions
- **None detected against the settled assumptions.** The operator's root-cause (a)-(e)
  is real and the symptom surface is genuinely absent from the 2026-07-23 report +
  disposition (grep-confirmed).
- **One nuance that SHARPENS, not contradicts:** the Step D lane did NOT silently skip
  parity — it honestly reported old-client evidence INFEASIBLE. This strengthens root
  cause (d) (the gap is "INFEASIBLE did not block," not "parity was never attempted")
  and sharpens the parity-contract fix (verifier-capability sub-clause). Flag it so
  the rec is not mis-framed as "nobody thought about parity."
- **Where a reviewer may push back on Verdict 1:** one could argue the token≠crux
  caveat's "observe the fix / repo-specific live verification" already covers
  outcome-verification. Rebuttal inlined above (the cd46198 evidence: the crux PATH
  ran, a-e PASSED; the gap was purely the outcome/mechanism axis + visual layer the
  caveat never names). This is the weakest-seamed verdict; it is offered as
  MERGE-EXTENDS, not as settled.
- **Carrier-constraint flag (not a contradiction):** this file is non-committed
  scratch under `tmp/` because vh-solara carries harness-adoption tracking in
  session-state ONLY. The canonical eventual home is the sibling harness repo's
  `researches/` as a follow-on; promoting it there is a separate operator-approved
  slice, explicitly out of scope here.

---

## Status

This is an operator-review-only ADDENDUM (read-only research; no code, harness,
commit, backlog, checkpoint, or live-doc changes). It proposes 3 harness-layer recs
with MERGE/UNION verdicts under the disposition's HYBRID rule; it does not adjudicate
them. If the maintainers adopt any of them, the target live docs to update in a
follow-on slice are: the disposition's rec table (§4) + the HYBRID addendum's
merge/union candidate lists — NOT this file.
