# Run-packet schema — deep-research

The run packet is the single resumable state of a deep-research run. It lives
at `tmp/agent-runs/<alias>/deep-research/<run-id>/run-packet.md` and is
updated after every query batch, evidence batch, and classification. On
resume it is read FIRST and treated as claim-of-record, not truth.

Use the section order below. Markdown tables are the transport; keep one row
per item, and never drop a section — an empty section stays with a one-line
"none yet" so silence is never ambiguous with "not recorded".

## Skeleton

`````markdown
# Deep-research run packet — <run-id>

## Frame
- question:
- decision_supported:
- freshness_requirements:
- exclusions:
- budget: <total> (consumed: <estimate at last update>)
- material_questions: Q1..Qn (3–5)

## Surface observations   (recorded FIRST, revalidated on resume)
- runtime: agent=<build>, alias=<session alias>, started=<timestamp>
- search_surface: vhmcp=<present|absent>, tools=<which proved callable>
- webfetch_fallback: <usable|not>
- surface_incidents: <tool failures, labeled capability_failure>

## Completed queries      (the dedup index — never repeat these)
| # | query | tool | results | date | notes |
|---|---|---|---|---|---|

## Attempted reads        (every locator ever tried, success or not)
| # | locator | outcome | duplicate_of | source_family | notes |
|---|---|---|---|---|---|

## Evidence records       (only locators actually read; read_depth REQUIRED)
| id | locator | excerpt | read_depth | retrieved_at | source_version | source_family |
|---|---|---|---|---|---|---|

## Corroboration sightings (labeled; never family-counting evidence)
| locator | restates | corroborates |
|---|---|---|

## Claim classifications
| claim | classification | evidence_ids | notes |
|---|---|---|---|

## Gaps
- [ ] GAP-<n>: <gap> — ranked #<k> because <flip potential>
      status: <open|closed>  next_action: <exact next action>

## Stop
- stop_reason: <sufficient_evidence|budget_exhausted|capability_blocked>
- recommendation:
- what_would_change_it:
- remaining_uncertainty:
- pilot_usage: queries=<n>/6 reads=<n>/12 gap_passes=<n>/1 delegations=<n>/2

## Final report           (verbatim — the exact text returned to the caller)
````
<the full final report text lives IN the packet inside this fenced code
block: recommendation, what would change it, remaining uncertainty, stop
reason, and the exact locator of EVERY Evidence record cited by ≥1 claim.
The outer fence is a BARE 4+-backtick line (only backticks, nothing
after them): it opens the report block, and only another bare line with
at least as many backticks closes it. Inside the report, 3-backtick and
language-tagged lines (```json) are LITERAL report content — inner code
blocks never toggle the fence — and Markdown headings inside the fence
are LITERAL report content and never end the report. After the closing
fence the report is complete: later lines in the section (like the
finalization_check below) are packet metadata, never report content>
````
finalization_check: PASS <validator invocation> @ <date>
`````

## Field notes

### Frame

- `decision_supported` — the decision this research serves. A topic without
  a decision cannot be ranked or stopped honestly; reframe or push back on
  the caller before gathering.
- `freshness_requirements` — the maximum acceptable evidence age, per
  material question when they differ. Drives the staleness check in the
  claim audit.
- `exclusions` — sources, domains, or option families the caller ruled out.
  Excluded material is never gathered, never cited.
- `budget` — total granted and the running consumption estimate. Recording
  consumption at each update is how `budget_exhausted` stays honest.

### Surface observations

Recorded BEFORE any retrieval (step 1 of the procedure) and revalidated on
every resume. `surface_incidents` entries are labeled `capability_failure`
and carry the exact error class (missing tool, unregistered, HTTP failure,
rate limit, paywall). A capability failure on a REQUIRED discovery or read
path forces the `capability_blocked` stop reason — it is never reported as
"no evidence found".

### Completed queries

One row per discovery query actually issued, through any surface. This table
is the dedup index for resume: a completed query is never re-issued on
resume without a recorded reason (e.g. the surface changed).

### Attempted reads

One row per locator ever tried, INCLUDING failures and duplicates:

- `outcome` — `read` | `failed` | `skipped` (with the reason in notes)
- `duplicate_of` — the read number of the canonical copy when this locator
  serves the same underlying content (mirror, syndication, reprint)
- `source_family` — the independence family (e.g. `vendor-docs`,
  `community-blog-X`, `spec-repo`). Derivative sources belong to the SAME
  family as their origin; independence is counted over families, never over
  URLs.

### Evidence records

Only locators that were actually read, at a stated depth:

- `id` — stable identifier matching `E<digits>` EXACTLY (`E1`, `E2`, …),
  referenced by claim rows. An ID is never extended or decorated:
  `E6-derivative-stale` is NOT an Evidence-record ID and may never appear
  as claim evidence. A parenthetical annotation may FOLLOW an ID in a
  claim's evidence column (`E4 (live docs)`) — it is stripped, not parsed.
- `locator` — URL or local path precise enough to refetch
- `excerpt` — the actually-read passage supporting the claim binding; short
  but verbatim
- `read_depth` — REQUIRED on every evidence record: `full` (whole document)
  | `skim` (sections). A row without a `read_depth` label is an invalid
  evidence record. Snippets and search-result fragments are query
  observations only and may never become evidence records — multi-site
  snippet corroboration goes to Corroboration sightings below.
- `retrieved_at` — retrieval date
- `source_version` — the source's own version/date when it exposes one
  (release tag, "last updated", published date); `unknown` otherwise
- `source_family` — inherited from the corresponding Attempted-reads row
  (stable locator join), so per-evidence independence is auditable without
  re-derivation

### Corroboration sightings

Multi-site corroboration (field logs, Q&A threads, vendor docs restating one
origin) is recorded here as clearly-labeled rows — NEVER as Evidence records
and never as family-counting support. A corroboration row (`restates` = the
underlying content it echoes; `corroborates` = the claim/record it lines up
with) cannot lift a claim past `partial` (noted as derivative-corroborated);
only non-derivative Evidence records count toward the ≥2-family
`supported` threshold.

### Claim classifications

Exactly one classification per consequential claim:
`supported` | `partial` | `contradicted` | `unsupported` | `inference`
(definitions in `SKILL.md` → "Claim audit"). `evidence_ids` must reference
records whose excerpts were read at a depth sufficient for the
classification, and may contain ONLY Evidence-record IDs matching
`E<digits>` (comma- or `+`-separated; a parenthetical annotation directly
after an ID is allowed and ignored). Attempted reads, snippets, and
corroboration sightings NEVER appear in `evidence_ids` — prose notes may
mention them, but never as support. `contradicted` rows carry both sides in
notes. A `supported` claim citing exactly ONE evidence record (a normative
claim resting on an authoritative spec read at `full` depth — see
`SKILL.md` → "Direct support vs corroboration") MUST carry the basis note
`direct support (authoritative spec, single family)` so direct support
stays visibly distinct from ≥2-family corroboration. A `supported` claim
citing TWO OR MORE records MUST span ≥2 distinct `source_family` values
among them — duplicate or empty families do not count, and the remedy for
a single-family multi-evidence claim is reclassifying to `partial`, never
a broader note. (Distinct recorded family identifiers are a structural
proxy the validator can count — they are not proof of semantic
independence.)

### Gaps

Checklist ranked by potential to change the recommendation. `next_action` is
the EXACT next action (query text, locator to try) — it is what a resume
executes. A run that stops `budget_exhausted` always leaves ≥1 open gap with
a concrete `next_action`.

### Stop

- `stop_reason` — exactly one of `sufficient_evidence`, `budget_exhausted`,
  `capability_blocked` (definitions in `SKILL.md` → "Decide & stop")
- `recommendation` / `what_would_change_it` / `remaining_uncertainty` — the
  decision-oriented closeout
- `pilot_usage` — the limit counters at stop, recorded so the stop is
  auditable against the pilot limits

### Final report

The verbatim final-report text lives IN the packet under `## Final report`
(not only in the chat return) so every citation is checkable later. The
report MUST sit inside ONE fenced code block in that section, delimited by
a BARE 4+-backtick outer fence (a line of only backticks — nothing after
them — with at least 4; the opening fence's backtick count is the minimum
a closing fence must meet): fenced lines are the report; 3-backtick lines
and language-tagged fences (```json) inside the block are LITERAL report
content and never toggle it, and Markdown headings (`#`/`##`/`###`)
inside the fence are LITERAL report content and never end it — the report
ends at the closing fence (or the next `##` heading outside an open
fence). After the closing fence the report is complete: anything later in
the section — including a second fenced block — is packet metadata and is
never locator-checked. The `finalization_check:` line stays OUTSIDE the
fence (it is packet metadata appended after validation, not text returned
to the caller).
The report MUST contain the exact locator string of every Evidence record
cited by ≥1 claim — citing an E-ID alone is not a citation. The
finalization check
(`vh-agent-harness exec bash .opencode/skills/deep-research/scripts/validate-packet.sh <packet>`,
run as the mandatory final step — see `SKILL.md` → "Finalization check")
verifies this mechanically — a missing or never-closed fence is itself a
violation (a 3-backtick or language-tagged line never closes the report;
an unterminated report fails rather than passing as closed); the
`finalization_check:` line records the PASS and date. The check is
structural only: it never changes stop-reason semantics.

## Promotion (explicit-only)

A promoted durable artifact (`researches/sources/YYYY-MM-DD-<slug>-sources.md`
or `researches/decisions/YYYY-MM-DD-<slug>-decision.md`) is a distillation
written ONLY when the caller set `promotion=` or asked. The run packet itself
stays under `tmp/` (or travels with the promoted artifact as an appendix when
the caller asks for it) and remains the operational state for resume.
