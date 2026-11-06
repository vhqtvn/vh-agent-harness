# Checkpoint: deep-research pilot — implementation + comparative validation (2026-11-06)

**Date:** 2026-11-06
**Status:** pilot implemented + validated (single-run comparative evidence); pack files rendered and in the working tree awaiting commit routing; NOT a core-promotion claim
**Provenance:** validation evidence currently lives in the transient run packet
`tmp/agent-runs/deep-research-pilot/run-broken-links/run-packet.md` (retained
until tmp cleanup) and in coordinator chat. This checkpoint is the durable
landing of that evidence; the numbers below are self-contained.

## What landed

- Local overlay pack `native-deep-research-pilot` at
  `.vh-agent-harness/overlays/native-deep-research-pilot/`: a `/deep-research`
  command, a `deep-research` skill (with the `references/run-packet.md`
  schema), and a pack README.
- Selected in `.vh-agent-harness/vh-harness-profile.yml` (under `overlays:`)
  and rendered via `make update`.
- Zero permission/config changes: the command routes `agent: build` through
  the existing task map; the vhmcp tools it drives were already permitted.
- No bootstrap-inventory entry, by design: project-local packs are not shipped
  inventory (`internal/cli/bootstrap_inventory_test.go` governs shipped packs
  only). The inventory tests pass unchanged with the pack selected.

## Runtime facts settled (this installation)

- **vhmcp MCP is the live-proven discovery surface from both researcher seats
  AND build-dispatched researcher.** Receipt: the
  build→researcher→vhmcp_web_search→vhmcp_fetch_url chain answering against
  the lychee README (run packet, surface observations + queries/reads #1–#5).
- **Native opencode `websearch` is registration-gated in this installation**
  (provider/env-flag gate at registration — not a permissions issue). Any
  other installation must probe rather than assume.
- **Command routing needs no permission changes**: `/deep-research` targets
  `agent: build`; the existing task map already covers everything the run used.

## Comparative validation (single-run pilot, NOT a benchmark)

Same day (2026-11-06), same public question:

> For a small public documentation site hosted on GitHub Pages, should all
> broken-link checks run on every pull request, or should internal-link checks
> run on every pull request while external-link checks run on a nightly
> schedule?

### Baseline — plain researcher run

- 5 queries; 4 full reads + ~8 snippet-only reads.
- ~17–18 min wall.
- Honest truncation/provenance caveats in the report; contradictions recorded
  as "none detected" (scoped to what it actually read).
- Recommendation: split — internal-link checks on every PR, external-link
  checks on a nightly schedule.

### Procedure — deep-research run (2 phases, deliberate interruption + resume)

- 8 queries (6 in phase 1 + 2 in phase 2 under a caller-raised budget);
  14 attempted locators → 8 successful-distinct reads (planted duplicate and
  bogus URL excluded); ~10 min wall; 1 of 2 researcher delegations used.
- Resume after a deliberate phase-boundary interruption: **0 repeated
  completed work** (0 queries re-issued from the packet index; 0 sources
  re-read except the caller-mandated planted duplicate, which is the test
  itself and was detected + excluded).
- Stop reason `sufficient_evidence` with all 5 gaps dispositioned (2
  flip-capable gaps closed by the depth pass; 3 ranked non-flipping or
  excluded-by-scale).
- Recommendation: split — **CONVERGENT with baseline**; the procedure did not
  distort the outcome.

### Planted tests (3/3 passed)

1. Bogus URL → labeled capability failure and excluded from coverage (the
   surface fails loudly rather than fabricating content).
2. Planted duplicate re-fetch → detected as a duplicate/source-family member,
   not counted as new independent coverage.
3. Planted false claim ("GitHub Pages requires a paid plan for every public
   repository") → contradicted against a primary docs.github.com citation
   (Pages is available with GitHub Free for public repos). Verdict: FALSE.

### Claim audit

- 5 supported / 1 partial / 2 inference / 1 contradicted (the planted claim) /
  0 unsupported.
- One genuine intra-corpus tension surfaced and recorded: fragment-checking
  flags differ across practitioners (enable vs. disable around SPA/hash-route
  sites) — a config-level tension that does not affect the A/B recommendation.
- Scoped-none statement recorded: no contradiction detected on the core
  question across the evidence set (the fragment tension is the only
  recorded one).

### Cost

- Procedure run: fewer successful reads and less wall time than baseline;
  more total queries. Tokens were unobservable in both runs (recorded
  "unknown").

## Verdict (scoped)

At pilot evidence tier (a single-run comparison), the procedure's added value
is **inspectability** — claim→excerpt bindings, read-depth labels,
source-family independence, resume machinery, an honest stop taxonomy, and a
falsifier list — not a different recommendation. No superiority or
equivalence claims beyond this single-run comparison.

Nine procedure-text ambiguities were documented as inputs for future skill
iteration (recorded, deliberately not fixed): see the pack README →
"Known limitations (from pilot validation)".

## Run packet

Transient, retained until tmp cleanup:
`tmp/agent-runs/deep-research-pilot/run-broken-links/run-packet.md`.

## Follow-ups (NOT in this slice)

- Stale docs-version mentions (`v1.14.x` → `v1.18.x` range) in
  `docs/coordination/PROMOTER_RUNBOOK.md` (around line 201) and
  `docs/coordination/README.md` (around line 35).
- A second baseline+procedure pair if stronger comparative evidence is ever
  needed.

## Verification

| Claim | Verifying command/output | Verified |
|-------|--------------------------|----------|
| gofmt clean on Go source | `gofmt -l internal/ cmd/` → empty output | yes |
| go vet clean | `go vet ./...` → no output | yes |
| Full test suite green on this tree | `go test ./... -count=1` → 32 ok / 0 FAIL | yes |
| Rendered skill frontmatter valid | `vh-agent-harness skill validate .opencode/skills/deep-research` → `OK deep-research — frontmatter valid` | yes |
| Rendered files exist | `ls` → `.opencode/commands/deep-research.md`, `.opencode/skills/deep-research/SKILL.md`, `.opencode/skills/deep-research/references/run-packet.md` | yes |
| Bootstrap inventory tests pass with pack selected | `go test ./internal/cli/ -run TestBootstrapInventory -count=1 -v` → 5/5 PASS | yes |
| Pack selected in profile | `grep -n native-deep-research-pilot .vh-agent-harness/vh-harness-profile.yml` → line 10 under `overlays:` | yes |
| Run packet exists at tmp path | Read of `tmp/agent-runs/deep-research-pilot/run-broken-links/run-packet.md` (120 lines) | yes |
| Procedure-run numbers above | Run packet: queries table (8), attempted-reads table (14 rows), `pilot_usage`, stop + resume-record sections | yes |
| Baseline-run numbers above | Same-day baseline run report (coordinator-held); numbers as recorded there | yes (as reported; packet-independent) |

## Findings

- **Recommendation convergence**: source=run packet stop section + same-day baseline report, confidence=high, type=fact — both runs recommend the split; the procedure did not change the outcome at this evidence tier.
- **Source-level resume works**: source=run packet "Phase-2 resume-test record", confidence=high, type=fact — deliberate interruption + resume repeated 0 completed work; the planted duplicate was detected rather than double-counted.
- **Planted tests exercise the honesty rules**: source=run packet planted-test rows, confidence=high, type=fact — capability-failure labeling, duplicate/source-family detection, and primary-source contradiction all behaved as the skill text requires.
- **Tokens unobservable**: source=both run reports, confidence=high, type=fact — neither surface exposes token usage; recorded "unknown" rather than estimated.

## Contradictions

None detected in this slice. (The fragment-checking tension recorded in the
claim audit is an intra-corpus research finding, not a repo/process
contradiction; it is preserved in the run packet and summarized above.)
