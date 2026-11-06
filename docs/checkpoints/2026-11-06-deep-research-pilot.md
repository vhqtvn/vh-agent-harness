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
| Fresh-process discoverability of command+skill | `tmp/agent-runs/deep-research-pilot/smoke-stdout.log`: `→ Skill "deep-research"` + "`/deep-research` loaded"; frame parsed `budget=20m` | yes |
| command→researcher→vhmcp path from a fresh process | smoke log: researcher task fired + completed; `vhmcp_web_search`/`fetch_url`/`code_docs`/`web_answer` receipts in auto-gate-audit lines | yes |
| Unbound-alias fallback packet dir worked | pre-run `ls tmp/agent-runs/deep-research/` → absent; post-run `20261106-pg16-wal-limits/run-packet.md` exists | yes |
| Crux claim-audited with conservative classes | smoke packet claim table: C4 restart-vs-reload `supported` (families A+B); C3/C7/C8/C10 `partial` with single-family bases named | yes |
| Evidence records free of snippet-only rows | smoke packet E10 = multi-site snippet-aggregation record (no excerpt, no read depth) bound by C5 | no (PARTIAL — recorded in addendum) |
| Final-message citations are literal URLs | `grep -c "https\?://"` over `smoke-stdout.log` → 0 | no (PARTIAL — recorded in addendum) |
| Smoke usage/stop recorded | smoke packet `Usage` + report: 6/6 queries, 6/12 reads, 1/1 depth, 1/2 delegations, `sufficient_evidence` | yes |
| Smoke exit 0 within budget | `SMOKE_EXIT=0` in run output; epoch-file delta 745 s < 1200 s (20m budget) | yes |

## Findings

- **Recommendation convergence**: source=run packet stop section + same-day baseline report, confidence=high, type=fact — both runs recommend the split; the procedure did not change the outcome at this evidence tier.
- **Source-level resume works**: source=run packet "Phase-2 resume-test record", confidence=high, type=fact — deliberate interruption + resume repeated 0 completed work; the planted duplicate was detected rather than double-counted.
- **Planted tests exercise the honesty rules**: source=run packet planted-test rows, confidence=high, type=fact — capability-failure labeling, duplicate/source-family detection, and primary-source contradiction all behaved as the skill text requires.
- **Tokens unobservable**: source=both run reports, confidence=high, type=fact — neither surface exposes token usage; recorded "unknown" rather than estimated.

## Contradictions

None detected in this slice. (The fragment-checking tension recorded in the
claim audit is an intra-corpus research finding, not a repo/process
contradiction; it is preserved in the run packet and summarized above.)

## Closeout addendum (operator closeout, later 2026-11-06)

### Acceptance fixes applied after the pilot review

Six fixes from the pilot review landed in the pack source + rendered files
(uncommitted at smoke-test time; see tree binding below):

1. **C1 — conservative class precedence**: `supported` requires ≥2
   independent source families; any single-family claim is `partial` with
   the single-family basis noted (SKILL.md → "Claim audit" → class
   precedence).
2. **C2 — snippets are not evidence**: snippet/search-fragment rows removed
   from the Evidence-records contract — snippets may never become evidence
   records, and claims may not bind to snippet-only locators (SKILL.md
   binding rule + run-packet schema `read_depth` field note).
3. **C3 — `source_family` column**: added to Attempted-reads and
   Evidence-records tables, inherited by stable locator join so per-evidence
   independence is auditable without re-derivation.
4. **Delimiter rows**: schema skeleton tables now carry explicit `|---|`
   delimiter rows.
5. **B4 — session rules + unbound fallback**: the command no longer implies
   a session prerequisite (only "repo session rules") and names the headless
   fallback packet dir `tmp/agent-runs/deep-research/<run-id>/` when no
   session alias is bound.
6. **C4 — README overclaim reworded**: the "exactly-once" resume claim was
   replaced by "logical resume, not an exactly-once guarantee" (pack README
   "What it adds" table + skill resume section).

Nine pilot ambiguities were deliberately NOT fixed (procedure refinements,
documented in the pack README → "Known limitations (from pilot validation)",
items 1–9): researcher-child query counting, failed-read counting,
phase-boundary stop vocabulary, caller-overridable packet path, retry-once
letter-vs-intent, depth-pass unit, `web_answer` budget counting, hardcoded
`pilot_usage` denominators, and formalizing planted/duplicate rows.

### Review-finding dispositions

- C1 — fixed (conservative class precedence, ≥2-family-or-partial).
- C2 — fixed (snippet-as-evidence removed + no-claim-binding rule).
- C3 — fixed (`source_family` column with inheritance).
- B4 — fixed (session-rules wording + unbound-alias fallback dir).
- C4 — fixed (README exactly-once overclaim reworded).
- delimiter-rows — fixed (schema skeleton delimiter rows).
- B1 — DISCHARGED by the fresh-session receipt below (discoverability and
  function demonstrated from a genuinely fresh OpenCode process; per-process
  skill caching can no longer mask it).

### Fresh-session smoke receipt (2026-11-06, later)

- **Command** (nested headless opencode — the fresh process):
  `vh-agent-harness exec bash -c 'timeout 1500 opencode run "/deep-research In PostgreSQL 16, what are the per-instance limits on WAL senders (max_wal_senders) and replication slots (max_replication_slots), what operator-visible failures occur when each is exhausted, and can either be changed without a restart?; budget=20m"'`
  — stdout+stderr captured to
  `tmp/agent-runs/deep-research-pilot/smoke-stdout.log` (81 lines).
- **Exit status**: 0. **Wall time**: 745 s external (epoch files; the run's
  own internal estimate ≈14 min; budget 20 m). **Stop reason**:
  `sufficient_evidence`.
- **Packet**: `tmp/agent-runs/deep-research/20261106-pg16-wal-limits/run-packet.md`.
- **Usage**: 6/6 queries (2 vhmcp MCP timeouts honestly recorded as
  no-evidence rows), 6/12 reads, 1/1 depth pass, 1/2 researcher delegations.
- **Verdict: PARTIAL** — the boundary is functional end-to-end from a fresh
  process; two integrity deltas recorded below.

Six verification outcomes:

| # | Check | Outcome | One-line evidence |
|---|---|---|---|
| 1 | Discoverability | PASS | `→ Skill "deep-research"` loaded + "`/deep-research` loaded" (log lines 5/9); `budget=20m` parsed into the frame; report carries the command Return contract (packet path, recommendation, stop reason, usage); zero "unknown command" strings. |
| 2 | Path exercised | PASS | researcher task "PG16 WAL limits deep-research" fired and completed (log lines 20/41); vhmcp receipts: `web_search` ×3, `fetch_url` ×3, `code_docs` ×2, `web_answer` ×1; packet Queries table with 6 rows incl. outcomes. |
| 3 | Packet fallback | PASS | packet at `tmp/agent-runs/deep-research/20261106-pg16-wal-limits/` — no alias component; the directory was absent pre-run (checked before launch). |
| 4 | Evidence integrity | PARTIAL | claims bind to E-IDs ✓; families labeled (A/B/derivative) with a source-policy section ✓; crux C4 restart-vs-reload `supported` over families A+B ✓ with C1 precedence visibly applied (C3/C7/C8/C10 → `partial`, single-family bases named) — BUT no per-record `read_depth` labels, and E10 is a multi-site snippet-aggregation record (no excerpt, no read depth) bound by C5, violating the C2 letter. |
| 5 | Report citations | PARTIAL | final message contains ZERO URLs (`grep -c "https\?://"` over the whole log → 0); citations resolve one hop away through the packet (E1/E8 full URLs, E10 partial locators) and the report names the packet path. |
| 6 | Usage | PASS | external wall 745 s + packet `Usage` (6/6, 6/12, 1/1, ≈14 min) + report "1 of 2 allowed researcher delegations used". |

Observations carried forward (no source edits in this slice, per contract):

- **Agent seat**: headless `opencode run` executed under the repo default
  primary (`coordination` — `default_agent` in `opencode.jsonc`); the
  command frontmatter `agent: build` did not reroute in run mode. The
  command → researcher → vhmcp chain was still exercised (coordination
  delegated the gather per the skill's runtime model, and its permission map
  covers `researcher` + `tmp/**` writes). Whether run mode should honor
  command agent frontmatter is an upstream opencode behavior question, not a
  pack defect.
- **Packet shape condensed by the fresh run**: no Surface-observations
  section header (probe recorded as Queries row #1 "probe PASSED"), no
  Attempted-reads table, per-record `retrieved_at`/`source_version` mostly
  implicit, delegations counter in the report instead of packet Usage. The
  schema-required CONTENT is mostly present; the transport deviates from the
  skeleton. Candidate input for the future skill-iteration list alongside
  the nine recorded ambiguities.
- **E10/C5 class**: C5 `supported` rests partly on snippet-level multi-site
  field-log corroboration (E10). Under the C2 letter that support is not an
  evidence record, so C5 would degrade to `partial` (E9 is single-family).
  Future skill iteration should decide whether field-log corroboration rows
  belong in Evidence records at all or in a separate corroboration annex.

**Tree binding**: the smoke run executed against the post-fix working tree —
HEAD `7fa07b9` plus eight uncommitted modified files (three overlay-pack
source files, pack README, three rendered `.opencode/` files, and
`rendered-outputs.json`). The fixes were NOT committed at test time; this
addendum binds to that exact tree state.

## Closeout addendum 2 (post-smoke adjudication, 2026-11-06)

### Adjudication (demonstrated defect — fixed, not re-adjudicated)

The fresh-session smoke receipt (addendum 1, check #4) was adjudicated
material: the packet classified C5 and C6 as `supported` on the strength of
one authoritative source family each (C5: E9/family B; C6: E1/family A) plus
E10 — a multi-site snippet-aggregation record the run's own source policy
classes as derivative = corroboration only. Derivative snippet aggregation
silently counted as a second source family → confidence inflation, the exact
operator-flagged inflation mode. Root cause: the C1 precedence rule required
≥2 independent families but did not exclude derivatives from the family
count. Two smaller deltas in the same receipt: Evidence records omitted the
required `read_depth` label; the final report carried zero inline URLs
(citations one hop away in the packet).

### Fixes applied (three surgical clauses)

1. **F1 — SKILL.md, claim-audit precedence rule** (extends C1): derivative /
   corroboration-only sources and aggregations of snippets — however many
   sites the aggregation spans — NEVER count toward the ≥2-family threshold;
   `supported` requires two independent NON-derivative source families;
   derivative corroboration lifts a claim to `partial` at best (noted as
   derivative-corroborated), never to `supported`.
2. **F2 — run-packet.md schema, Evidence records**: (a) multi-site
   corroboration (field logs, Q&A, vendor docs restating one origin) is
   recorded in a new clearly-labeled **Corroboration sightings** section as
   rows, never as family-counting Evidence records; (b) `read_depth`
   (`full`|`skim`) is a REQUIRED field on every Evidence record — a row
   without it is invalid.
3. **F3 — commands/deep-research.md, Return contract**: the final report must
   inline the locator URL of every cited evidence record; an E-ID or packet
   reference alone is not a citation.

### Verification (honest: structural only)

| # | Check | Outcome | One-line evidence |
|---|---|---|---|
| 1 | F1 rule text in shipped skill | yes | precedence bullet in pack `SKILL.md` names derivative/corroboration-only sources + snippet aggregations as never threshold-counting |
| 2 | F2 schema changes | yes | `Corroboration sightings` section + REQUIRED `read_depth` wording in pack `references/run-packet.md` |
| 3 | F3 Return contract | yes | inline-locator-URL bullet in pack `commands/deep-research.md` Return list |
| 4 | Render byte-identity (3 pairs) | yes | `git diff --no-index` source-vs-rendered per pair → empty output, exit 0 (3/3 identical) |
| 5 | Gates green | yes | `go test ./... -count=1` → all ok / 0 FAIL; `gofmt -l internal/ cmd/` → empty; `go vet ./...` → clean; `vh-agent-harness skill validate .opencode/skills/deep-research` → `OK deep-research — frontmatter valid` |

The fixes are verified **STRUCTURALLY** only: rule text present in the
shipped skill/schema/command, renders byte-identical to sources, gates
green. **No second smoke run was performed**, per the operator's
single-receipt boundary — behavioral re-verification (a subsequent run
excluding derivatives from family counts, labeling `read_depth`, and inlining
cited URLs) is deferred to real subsequent use.

### Runtime-mode observation (recorded, no action this slice)

Headless `opencode run` executed the command under the repo default primary
(`coordination`) rather than rerouting per the command's `agent: build`
frontmatter; the command → researcher → vhmcp chain still functioned via the
default primary's task map (addendum 1, "Agent seat", records the receipt).
Recorded as a runtime-mode nuance; no action this slice.
