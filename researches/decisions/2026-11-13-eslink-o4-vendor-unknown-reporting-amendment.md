# O4 eslink — Vendor-Noise Reporting Amendment: cohort-links Unknown Aggregation

- **Date:** 2026-11-13
- **Type:** decision record (reporting amendment, operator-ratified posture "a+b+c")
- **Status:** RATIFIED — operator "grant + a + b + c" on 2026-11-13
- **Effect:** amends the REPORTING posture of doctor check #28 / the cohort
  preflight WARN surfaces (the wiring-scope record
  `researches/decisions/2026-11-12-eslink-o4-s2-s4-wiring-scope.md` §C "Dogfood
  Noise Handling", ratified 2026-11-13). REPORTING ONLY: no tiering change, no
  scan-scope change, no suppression, no eslink semantic change, no corpus-pin
  change (`61/46/15 · 49 · 14/3/32 · 5 · 0` frozen). Own-file amendment per the
  TLA freeze-amendment precedent
  (`2026-11-08-eslink-o4-grammar-freeze-amendment-tla.md`).

## Provenance chain

```
wiring-scope record 2026-11-12 (ratified 2026-11-13, §C Dogfood Noise:
  "documented recurring WARN. No suppression, no hard-coded exception lists")
→ S3 landing f9a6438 + S4 landing 62b2d56 — LIVE dogfood receipt: the
   cohort-links check WARNs on this repo's own installed tree with
   3 proven adopter-internal broken edge(s) + 1104 unknown finding(s), the
   bulk under the vendored .opencode/node_modules/ tree (frozen-grammar
   unknowns across effect/, @ai-sdk/, yaml/, …; the 3 proven edges sit inside
   node_modules/yaml where dist/cli.mjs requires symbols sibling builds do
   not export) — itemized, the unknown list alone is four figures long
→ operator ratification "grant + a + b + c" 2026-11-13 — this record is the
   authoritative carrier of the ratified posture (parent-session records
   convention, same as the TLA amendment record).
```

The problem the amendment solves is NOISE, not findings: the wiring-scope
posture ("no suppression, a short note explains the limitation") produced an
itemized unknown report whose vendor bulk (thousands of vendored third-party
unknowns) buries the ~15 known platform unknowns (14 update-op `++/--` debt +
`compaction-primitives.js`) an operator of THIS repo actually needs to read.

## The ratified posture (operator "grant + a + b + c", 2026-11-13)

Verbatim-synthesized from the operator disposition; (a)/(b)/(c) are the
operator's lines, the riders below are the coordinator's synthesis carried by
the same grant:

- **(a) WARN stays the non-blocking baseline — no suppression, nothing
  hidden.** Unknown findings remain WARN-side only (U6: unknown never FAILs,
  never refuses); the check still fires, the totals still count every finding.
- **(b) vendor-dir unknown findings are EXCLUDED from the itemized unknown
  report.** The per-finding list (path + specifier + reason) carries
  non-vendor unknowns only.
- **(c) they collapse to ONE aggregate line (count + rider) — visibility
  without noise.** One line of the shape
  `+ N unknown finding(s) under vendor directories (node_modules/) not itemized`
  carrying the literal honesty rider
  (`link integrity only; behavior of preserved files is not verified`).

### SYNTHESIS riders (carried by the same grant)

1. **Proven broken edges stay itemized regardless of location** — a proven
   edge is a FACT, not noise; an edge inside `node_modules/yaml` itemizes
   exactly like an edge in `.opencode/scratch/`. Only UNKNOWN findings
   aggregate. (This is also what keeps the adopter-internal WARN side
   meaningful in vendored adopter trees.)
2. **Platform-endpoint enforcement remains whole-tree** — FAIL/refuse
   semantics, the O3 boundary predicate, the scan scope (the whole
   `.opencode/` JS tree, `node_modules/` included), and the G0c release gate
   are UNCHANGED. This is a reporting amendment only: it changes what the
   WARN-side report PRINTS, never what the check finds or how it tiers.
3. **Honesty preserved** — the aggregate count is STATED, never pretended
   zero: the unknown-total in the report header counts vendor unknowns, and
   the aggregate line states N. Nothing is dropped from the counts.
4. **Vendor definition = any path containing a `node_modules/` segment**
   (slash-clean path-segment equality; `.opencode/node_modules/…` matches,
   `foo/node_modules.js` does not). Deterministic path-shape rule — no
   content sniffing, no package-name lists, no hard-coded exception list
   (the §C "no hard-coded exception lists" clause is honored: the predicate
   is a shape, not an enumeration).

### Flip condition

Revisit (and revert to full itemization) **if aggregation ever masks a
FAIL-relevant fact** — e.g. an unknown finding under a vendor path that is
later shown to participate in a proven platform-bound breakage chain, or any
case where the aggregate line hides a finding the FAIL/refuse side needed to
see. Until such a case is demonstrated, the posture stands.

## Effect

- `internal/cli/cohort.go` (WARN block + full-diagnostic section 1b — the
  surfaces `update`/`install`/`accept-platform` preflights print) and
  `internal/cli/doctor.go` (check #28 WARN detail) split unknown findings
  into itemized (non-vendor) vs aggregated (vendor) at REPORT time only.
- Counts stay whole-tree and totals-first: the unknown-count header/summary
  numbers remain the TOTAL (itemized + vendor); the aggregate line states the
  vendor count.
- Proven broken edges (`Refuse` and `Warn` partitions alike) are never
  aggregated, wherever they sit.
- No inventory surface; no `internal/eslink` change; corpus pins frozen;
  the honesty contract (`internal/cli/cohort_honesty_test.go` — rider and
  banned-phrase discipline over every cohort message) extended, not weakened.
- README.agent.md doctor-row dogfood note updated to describe the aggregate
  line (accuracy requirement).

## Contradictions

- **None against the wiring-scope record.** §C said "no suppression, no
  hard-coded exception lists; a short doctor/docs note explains the
  known-unknown limitation" — all four clauses hold under this amendment
  (the WARN still fires with every finding counted; the vendor predicate is a
  path-shape rule, not an enumerated list; the docs note now explains the
  aggregation too).
- **None against U6.** Unknown findings never FAIL/refuse before AND after;
  aggregation is presentation of the WARN side only.
- **None against the honesty riders.** The aggregate line itself carries the
  literal rider; the totals never drop vendor findings.

## Findings

- **(finding)**: source=live dogfood doctor at the S3/S4 landings (carried by the operator dispatch, 2026-11-13), confidence=high, type=fact — this repo's installed tree yields a cohort-links WARN with 3 proven adopter-internal broken edge(s) and 1104 unknown finding(s), the bulk vendored under `.opencode/node_modules/`.
- **(finding)**: source=README.agent.md doctor-row dogfood note (landed 62b2d56), confidence=high, type=fact — the 3 proven edges sit inside `node_modules/yaml` (`dist/cli.mjs` requires symbols sibling builds do not export); the vendor unknown bulk spans `effect/`, `@ai-sdk/`, `yaml/`, and peers.
- **(inference)**: source=this record's ratification provenance, confidence=high, type=inference — the operator "grant + a + b + c" is carried by the coordinator dispatch to the landing session; this record is its authoritative carrier per the parent-session convention (same as the TLA amendment record).

## Verification

Pre-commit working-tree verification (assessed tree: HEAD `62b2d56` + this
change, single landing commit; commands run in that exact tree — receipts
below, reproducible at the landed commit):

| Claim | Verifying evidence (command/output, run where stated) | Verified |
|---|---|---|
| Wiring-scope record carries the no-suppression dogfood posture | record §C "Dogfood Noise Handling" @ `b773c1c`/"2026-11-12-eslink-o4-s2-s4-wiring-scope.md" (read this slice) | yes (cited) |
| S3/S4 dogfood receipt: WARN 3 proven + 1104 unknowns, bulk vendored | README.agent.md dogfood note (landed `62b2d56`, read this slice) + pre-commit live receipt: `bin/vh-agent-harness doctor` (rebuilt via `make build`) on this change's tree → cohort-links WARN "3 adopter-internal proven broken link(s), 1104 unknown finding(s) … 954 module(s) scanned)" with the 3 `node_modules/yaml` proven edges ITEMIZED, 20 non-vendor unknowns itemized, and the aggregate line "+ 1084 unknown finding(s) under vendor directories (node_modules/) not itemized — link integrity only; behavior of preserved files is not verified"; result HEALTHY (0 problems) | yes (re-observed pre-commit) |
| Operator ratified posture a+b+c on 2026-11-13 | operator disposition "grant + a + b + c" carried by the coordinator dispatch; this record is the carrier (parent-session convention) | yes (cited) |
| Vendor predicate is a path SEGMENT rule | implementation `isVendorCohortPath` (segment equality, no substring, no sniffing) + `TestIsVendorCohortPath_SegmentSemantics` (incl. `node_modules.js`/`node_modulesish` negatives) — `go test ./internal/cli/ -run Cohort -count=1` ok | yes (pre-commit, this tree) |
| Proven edges under node_modules still itemized | `TestCheckCohortLinks_VendorProvenEdgeStillItemized` + `TestCohortWarnSurfaces_VendorUnknownsAggregated` + `TestCohortHonesty_VendorAggregateLine` (formatter surfaces) — same test run ok; live receipt shows the 3 vendor proven edges itemized | yes (pre-commit, this tree) |
| Totals never drop vendor findings | header/summary counts stay `len(c.Unknown)`; aggregate line states N (`TestCohortWarnSurfaces_VendorUnknownsAggregated` pins total 4 = 2 itemized + 2 vendor; live receipt 1104 = 20 + 1084) | yes (pre-commit, this tree) |
| Corpus pins untouched | `go test ./... -count=1` → 33/33 packages ok at this tree (61/46/15 · 49 · 14/3/32 · 5 · 0); `git status` shows no `internal/eslink/` modification | yes (pre-commit, this tree) |
| No tiering / scan-scope / suppression change | `go test ./internal/cli/ -run Cohort -count=1` ok with all pre-existing doctor/cohort assertions unweakened (reviewer-confirmed extend-not-weaken); whole-tree sweep code unchanged; live receipt tier WARN + HEALTHY exactly as the amendment ratifies | yes (pre-commit, this tree) |
| Honesty discipline on the aggregate line | `TestCohortHonesty_VendorAggregateLine`: rider ON the aggregate line, no banned phrase, vendor paths not itemized; all `cohort_honesty` sweeps green over the vendor-enriched fixture | yes (pre-commit, this tree) |
