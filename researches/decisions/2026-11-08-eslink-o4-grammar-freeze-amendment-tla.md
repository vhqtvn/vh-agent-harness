# O4 eslink — Grammar-Freeze Amendment: Module-Top Await (TLA) Admission + FIT Pin Correction

- **Date:** 2026-11-08
- **Type:** decision record (freeze amendment + renewed operator approval)
- **Status:** RATIFIED — operator "go" (Option 1) on 2026-11-08
- **Effect:** amends the freeze record
  (`researches/decisions/2026-11-08-eslink-o4-grammar-variant-freeze.md` @
  `12d8195`) exactly as that record requires ("Any spec change after this
  point requires an amendment to this record plus renewed operator approval");
  S1′ resumes under the corrected spec with both review cascades intact
  (0 of 2 consumed at escalation). This is a single-family amendment — every
  other freeze disposition (AMB-1, AMB-2/D-3, the D-2 twelve exclusions)
  stands unchanged.

## Provenance chain

```
freeze record 12d8195 (Family 5 clause: "no top-level await")
→ S1′ stage-3 build (session ses_e4449ac16ffe, 2026-11-08) — frozen-spec
   gate-4/gate-5 pins unreachable; STOP per the frozen spec's
   "any mismatch = STOP and report, do not re-pin to absorb" clause
→ STOP AND ESCALATE report (full report:
   tmp/agent-runs/eslink-s1p/escalation-report.md — the three-way
   contradiction: corpus bytes vs packet pins vs frozen variant map)
→ coordinator recommendation (Option 1 of the report's decision space)
→ operator "go" (Option 1) 2026-11-08 — this record is the authoritative
   carrier of the ratification (parent-session records convention, same as
   the staged-restart record `2cae4a2`).
```

The escalation preserved the worktree (nothing committed; untracked
`internal/eslink/` only) and consumed **0 of 2** review cascades — the S1′
Section B budget is intact.

## The amendment (operator-ratified Option 1)

### 1. Family 5 (await) amended — module-top top-level await ADMITTED

The Family 5 clause is amended from "`await` admitted only as `AwaitExpr`
inside async function/arrow/method bodies; top-level await NOT admitted"
(map:268-272 @ `2f5b304`) to:

> `AwaitExpr := 'await' UnaryExpr` is admitted inside async
> function/arrow/method bodies **and at module top level** (module top is an
> [Await] context).

Parser-level realization (as measured in the escalation's one-line Option-B
flip): the module-top async frame carries [+Await]
(`internal/eslink/parser.go:65` in the S1′ worktree — currently
`asyncStack: []bool{false}`; the amendment makes the module-top frame true).

**Unchanged by this amendment:**

- `for await … of` stays EXCLUDED (Family 5 + D-2; fixture F5-FB(b) verdict
  unchanged: `UNKNOWN(reason: for-await not admitted)`).
- `await` in a NON-async function stays rejected — the false-broken guard
  survives, weakened only for the module-top case.
- The D-2 twelve-exclusion table stands untouched: top-level await was never
  a D-2 row — it was excluded by Family 5's clause text (the freeze record's
  completeness rule cited it as the worked example), so this correction is a
  Family 5 clause amendment, not a D-2 revision.

**Fixture verdict flip:** F5-FB(a) (module with top-level `await fetch(x);`
at module scope) flips `UNKNOWN(reason: top-level await not admitted)` →
`FIT`. The variant map carries this as an appended dated correction
(item 3).

**Evidence (the amendment's factual basis):** module-top top-level await
exists in **6 of the 61 corpus modules**, each verified by direct line
reading AND `node --check` PASS (TLA is legal ES2022 module syntax):

| Module (corpus path) | Line(s) | Form |
|---|---|---|
| `templates/core/.opencode/repo-configs/forbidden-patterns.js` | 39 | `const mod = await import("./forbidden-patterns.project.js")` in top-level `try` |
| `templates/overlays/repo-mail/plugins/repo-mail-egress-wiring.js` | 280, 283 | `const { gateEgressMessage } = await import(…)`, `const { scrubCredentials } = await import(…)` in `if (__isMain)` at module scope |
| `templates/core/.opencode/scripts/verify-chunk-timeout-backstop.js` | 225 | `try { await main(); } catch` epilogue |
| `templates/core/.opencode/scripts/verify-coordination-hints.js` | 701 | same epilogue |
| `templates/core/.opencode/scripts/verify-skill-sentinel.js` | 575 | same epilogue |
| `templates/core/.opencode/scripts/verify-task-registry.js` | 5349 | `try { await runWorkerMode(…) }` in `if (__entryPath === __modulePath)` |

**Root cause — census-input probe miss, not an S1′ build defect:** the
D-pass probe `^await\s` is line-start anchored and structurally misses
mid-statement/indented TLA (all six sites sit mid-line or indented). The
D-pass packet's own honest-gaps section pre-flagged exactly this risk class
("Probe-based absences are regex-strength, not parser-strength … could evade
the probe patterns"). The miss was in the census inputs; the S1′ build's
STOP was the frozen spec working as ratified.

### 2. Pin corrections (evidence-backed)

- **FIT 46 → 45 (of 61).** The packet-era census FIT pin (current era
  46/61) is off by one against ground truth. The delta module is
  `templates/core/.opencode/plugins/compaction-primitives.js` — **invalid
  JavaScript** (see item 5): the G32 reference lexer mis-lexed its unescaped
  backticks as balanced, so the census counted it FIT. `node --check` is the
  objective arbiter and rejects exactly this one module of 61 (full sweep
  re-run for this record).
- **All other pins reconcile EXACTLY under the TLA admission.** The
  escalation report's Option-B measurement (one-line flip, measured then
  reverted, tests green at both ends) matches every packet pin: edges 49,
  L-join L0/L1/L3 = 17/22/10, dynamic unknowns 5 (the exact pin set:
  `forbidden-patterns.js:39`, `repo-mail-egress-wiring.js:238` computed +
  `:280`, `:283`, `verify-task-registry.js:582` computed), collisions 0,
  and mixed-tree `broken=1` (`verify-task-registry.js` → `state-lib.js`
  missing `collectDecisionItems`) — the gate-5 required verdict, which was
  UNSATISFIABLE under the unamended grammar.

**Reconciliation evidence — the escalation report's measured divergence
table (reproduced verbatim from
`tmp/agent-runs/eslink-s1p/escalation-report.md` §3):**

| Pin (packet §C / spec §B) | Ratified grammar (pre-amendment) | TLA admitted (post-amendment) |
|---|---|---|
| modules 61 | 61 ✓ | 61 ✓ |
| FIT 46/61 | **39 ✗** | **45** (delta = node-invalid module) |
| edges 49 | 24 scan-edges ✗ | 49 with UNKNOWN-importer L0 rows ✓ |
| dynamic unknowns 5 | 0 ✗ (swallowed) | 5 ✓ exact set |
| collisions 0 | 0 ✓ | 0 ✓ |
| L-join 17/22/10 | shifted ✗ | 17/22/10 ✓ exact |
| mixed broken=1 (vtr→state-lib collectDecisionItems) | **0 ✗ unsatisfiable** | **1 ✓ exact** |
| map F5-FB(a): TLA → UNKNOWN | consistent ✓ | **flips to FIT ✗ (map revision needed)** |

The last row's map revision is item 3; with it, every row is reconciled.

These corrections are operator-ratified (this record), NOT a build-side
re-pin — the frozen spec's "STOP and report, do not re-pin to absorb" clause
is honored: the build stopped, reported, and the operator corrected the pins
through this amendment.

### 3. Variant-map F5 revision (appended correction, not a silent edit)

The variant map
(`researches/sources/2026-11-08-eslink-o4-grammar-variant-map.md`, landed
byte-verbatim at `2f5b304`) Family 5 row recorded "top-level await | ABSENT
(probe `^await\s` → none)". This landing appends a dated
"Amendment 2026-11-08" correction section to the map file naming the six
modules and the probe miss, per the repo's appended-correction convention:
the measured D-pass body above the appendix is preserved unedited, and the
byte-verbatim pin at `2f5b304` is unaffected (git history holds the D-pass
bytes). The map's Family 5 clause and the F5-FB(a) fixture verdict are
corrected by that appendix; F5-FB retains its discriminating role for
`for-await` and await-in-non-async, minus the module-top case.

### 4. Reference-implementation binding (folded from the freeze-landing review defer F2)

The freeze record's carried note (C-4: the S1′ slice must re-bind the
reference implementation to named, hash-pinned sources) is discharged HERE,
binding S1′'s retained-G32-regression and closure-semantics references to
two named pinned sources:

1. **The conformance packet's verbatim harness snapshot** —
   `tmp/eslink-o4-conformance-packet/harness/eslink/` (the landed packet @
   `d4ae991` records the verbatim-copy binding in its Verification table;
   corpus-module digests live in its `ledger.json`). Per-file SHA-256,
   computed fresh for this record:
   - `lexer.go` — `f5eff7f1083564c69f18d048d7f0bb0d187a4c20f957b6a14687c47450ae277d`
   - `parser.go` — `5204557259283ca59728436e1035fac19743790a2851ce0c520684f999388268`
   - `graph.go` — `ceb565b8e1f4ad8ff14a4d9098d04225d9c17568b4f29f97144ee34f4b1327be`
   - `census.go` (measurement-only additive file, not reference bytes) —
     `988b9fa3b9fcc64d203ecd0f9d8d45b945864876b3f40b1119e144a16df64e1a`
2. **The G32 archive** —
   `.local/coordinator/archives/eslink-g32-blocked/eslink-g32-blocked-2026-11-07.tar.gz`,
   SHA-256
   `f45d2fc104c04f29317e48ccc7988eaea5cd6a2186a9a4982c2115f6c629e750`.

Both digests verified this slice (see Verification). Any S1′ reference cite
(G32 regression ports, closure-semantics grounds) must resolve to one of
these two pinned sources.

### 5. compaction-primitives.js — known corpus defect, tracked separately

`templates/core/.opencode/plugins/compaction-primitives.js` is recorded as a
**known corpus defect**: invalid JavaScript — a template literal opened at
line 11 is terminated by bare, unescaped backticks at line 18
(`node --check` → `SyntaxError: missing ) after argument list`); it is the
only module of 61 that `node --check` rejects. It is tracked separately by
card
`task-2026-11-08t16-27-14-corpus-defect-compaction-primitives.js-is-invalid-js-unescaped-backticks-line-18-establish-usage-and-repair`
(status `draft` at this record). Until that card lands, the S1′ successor
pins the module as reason-coded `UNKNOWN`/unparseable (U1/U5) — this
amendment authorizes NO rewrite (Section C option 1, zero rewrites, stands).

## Effect

- The freeze is AMENDED (Family 5 clause; FIT pin 46→45; map appendix;
  reference binding recorded). Everything else in the freeze record stands.
- S1′ RESUMES under the corrected spec with the Section B budget intact:
  0 of 2 review cascades consumed at escalation; any new false verdict still
  halts; exhaustion still reverts to the F5 deferral.
- Gates 4 (golden pins) and 5 (mixed-tree skew) — BLOCKED at escalation —
  become satisfiable under the corrected pins: modules 61, FIT 45/61, edges
  49, dynamic unknowns 5, collisions 0, L-join 17/22/10, mixed-tree
  broken=1 (`verify-task-registry.js` → `state-lib.js`,
  `collectDecisionItems`).
- No parser code, corpus rewrite, refusal wiring, doctor check, or CI hook
  is authorized by this record beyond what the freeze + staged-restart
  records already authorize.

## Contradictions

- **The escalation's three-way contradiction (corpus bytes vs packet pins
  vs frozen variant map) is RESOLVED by this amendment** — root cause named
  (line-start-anchored probe blind to mid-statement TLA; a pre-flagged
  probe-strength gap), disposition operator-ratified, evidence recorded.
- **Census FIT 46 vs ground-truth 45** (compaction-primitives.js mis-lexed
  FIT by the reference lexer) — resolved with objective `node --check`
  evidence; pin corrected by ratification, not silent re-pin.
- **None detected against D-2**: top-level await was excluded by Family 5
  clause text, not by a D-2 exclusion row; the twelve-row table is
  untouched.
- The map's Family 5 file list (15 await modules) remains consistent: all
  six TLA modules are members of that 15-file list (TLA is a subset of
  await usage).

## Findings

- **(finding)**: source=direct line reads (this slice), confidence=high, type=fact — module-top TLA present at the six cited corpus sites (forbidden-patterns.js:39; repo-mail-egress-wiring.js:280,283; verify-chunk-timeout-backstop.js:225; verify-coordination-hints.js:701; verify-skill-sentinel.js:575; verify-task-registry.js:5349).
- **(finding)**: source=full-corpus node sweep (this slice), confidence=high, type=fact — of the 61 corpus modules exactly one (`compaction-primitives.js`) fails `node --check`; all six TLA modules pass.
- **(finding)**: source=escalation report §1-§3 (S1′ session ses_e4449ac16ffe), confidence=high, type=fact — probe `^await\s` is line-start anchored; all six TLA sites sit mid-line/indented; the packet's honest-gaps section pre-flagged probe-strength absences.
- **(finding)**: source=escalation report §2B/§3 (measured then reverted; tests green both ends), confidence=high, type=fact — under the one-line TLA admission the measured clean tree is fit=45/unknown=16/broken=0/dynUnknowns=5, edges 49, L-join 17/22/10, mixed-tree broken=1 (collectDecisionItems) — every packet pin reconciles except FIT 46→45.
- **(finding)**: source=sha256sum (this slice), confidence=high, type=fact — G32 archive digest `f45d2fc104c04f29317e48ccc7988eaea5cd6a2186a9a4982c2115f6c629e750` and the four harness-snapshot file digests as recorded in §4.
- **(inference)**: source=this record's ratification provenance, confidence=high, type=inference — the operator "go" (Option 1) is carried by the coordinator dispatch to the landing session; this record is its authoritative carrier per the parent-session convention (same as `2cae4a2`).

## Verification

| Claim | Verifying evidence (command/output, run where stated) | Verified |
|---|---|---|
| Freeze record requires amendment + renewed operator approval for any spec change | freeze record @ `12d8195`, header "Effect" + closing paragraph | yes (cited) |
| Escalation occurred with 0/2 cascades consumed, nothing committed, worktree preserved | `tmp/agent-runs/eslink-s1p/escalation-report.md` header + §6-§7 (read this slice) | yes (cited) |
| TLA exists at the 6 cited sites | `sed -n` line reads this slice — all six confirmed (forms in §1 table) | yes (re-verified) |
| All 6 TLA modules `node --check`-legal | full 61-module sweep this slice (`find templates/core/.opencode templates/overlays -name '*.js' -o -name '*.mjs'` + `node --check` loop): total=61, sole failure compaction-primitives.js | yes (re-verified) |
| compaction-primitives.js is invalid JS (unescaped backticks line 18, template opened line 11) | `node --check` → `SyntaxError: missing ) after argument list` (this slice); lines 11-19 read — bare backticks at line 18, other sites escaped | yes (re-verified) |
| Defect card exists in draft | `jq -r .status` on the card → `draft` (card id in §5) | yes (re-verified) |
| G32 archive SHA-256 = `f45d2fc1…e750` | `sha256sum .local/coordinator/archives/eslink-g32-blocked/eslink-g32-blocked-2026-11-07.tar.gz` (this slice) → `f45d2fc104c04f29317e48ccc7988eaea5cd6a2186a9a4982c2115f6c629e750` | yes (re-verified) |
| Harness snapshot present with the recorded digests | `sha256sum tmp/eslink-o4-conformance-packet/harness/eslink/*.go` (this slice) → values in §4 | yes (re-verified) |
| Landed packet records the verbatim-copy binding + per-module digests | packet @ `d4ae991` Verification row "Harness uses verbatim reference implementation"; "per-file sha256 in `ledger.json` modules[].sha256" (packet time-sensitivity note) | yes (cited) |
| Module-top async frame is parser.go:65 in the S1′ worktree | `sed -n '60,70p' internal/eslink/parser.go` (this slice) → `asyncStack: []bool{false}` on line 65 | yes (re-verified) |
| Under TLA admission all pins reconcile except FIT 46→45 | escalation report §2B measurement + §3 divergence table (S1′-measured, then reverted with tests green); reproduced in §2 above | yes (cited) |
| Map landed byte-verbatim at `2f5b304`; freeze record at `12d8195` (HEAD) | freeze record verification table (sha256 `74e8972a…`, commit `2f5b304`); `git log --oneline` this slice → HEAD `12d8195` | yes (cited + HEAD re-verified) |
| This landing = amendment record + map appendix only (2 files) | commit diff of this change (map edit is append-only) | yes (see closeout) |

Measured Option-B numbers are cited from the S1′ escalation report, not
re-derived by this docs slice; their re-derivation is the S1′ successor's
gate-4 run under the corrected pins.
