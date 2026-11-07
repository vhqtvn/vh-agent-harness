---
name: deep-research
description: "Bounded deep-research procedure for decision-support questions: frame, reuse local artifacts, gather breadth-first through the existing researcher subagent, rank gaps, one selective depth pass, claim-level audit, and an honest stop. Use when `/deep-research` is invoked, when resuming a deep-research run packet, or when a bounded, resumable evidence pass with claim-level records is wanted."
compatibility: opencode
---

# Deep-research (bounded pilot procedure)

This skill is a **procedure layered on the existing runtime** — nothing new is
provisioned. You (build) orchestrate; the existing `researcher` subagent
gathers sources; the vhmcp MCP tools are the search surface. No new agent, no
database, no vault, no provider adapter, no permission changes.

Everything below is **instructions to you, the orchestrator**. The pilot
limits are guidance you enforce on yourself by recording usage in the run
packet — they are NOT hard controls enforced by any gate.

## Known surface facts (record, do not re-derive)

- **Primary search surface: the `vhmcp` MCP tools** — `web_search`,
  `fetch_url`, `read_document`, `web_answer`, `code_docs` — callable from a
  `researcher` subagent (live-proven: search → primary-source fetch chains
  have run through them).
- **Native opencode `websearch` is NOT registered in this installation**
  (provider-gated at registration; env flags default off). It may become
  available in other installations — probe, don't assume either way.
- **`webfetch` is permitted for `researcher`** as a fallback when the exact
  URL is already known.

**Surface failure is never evidence absence.** If a discovery query or a
source read could not run (tool missing, unregistered, erroring, rate-limited,
paywalled), record it in the run packet as a **capability failure** and let it
drive a `capability_blocked` stop reason. Reporting "no evidence found" from a
surface that could not run is the cardinal dishonesty of this procedure.

## The run packet

From the first step you maintain a run packet at
`tmp/agent-runs/<alias>/deep-research/<run-id>/run-packet.md`
(`<run-id>` = `YYYYMMDD-HHMMSS-<short-slug>`). The schema of record is
[`references/run-packet.md`](references/run-packet.md). Update it after every
query batch, evidence batch, and classification — it is the resumable state.
An excerpt recorded in the packet must come from a source you actually read;
a locator you never fetched is not evidence.

## Procedure

### 1. Frame

Record in the packet, before any retrieval:

- the **decision this research supports** (not just the topic)
- **freshness requirements** (how old may evidence be?)
- **exclusions** (sources, domains, or whole option families that are out)
- the **budget** and how it will be split across gathering vs the depth pass

Then **probe the search surface FIRST**: issue one trivial discovery query
through the normal path (a `researcher` delegation or, when you are probing
tool presence directly, the cheapest available call) and record what answered.
Note vhmcp availability and whether the `webfetch` fallback is usable. If the
surface cannot answer even a trivial probe, you are likely heading for
`capability_blocked` — establish that now, not after burning the budget.

### 2. Local-first

Before any new retrieval:

- search existing `researches/` artifacts (`researches/sources/`,
  `researches/decisions/`) for prior work on this question
- read any caller-supplied prior packets or pinned sources
- record what was reused and how it was fresh enough (or not) against the
  frame's freshness requirements

Reused local evidence enters the packet with the same evidence-record shape
as fresh retrieval (locator = the local artifact path plus the original
source locator it cites).

### 3. Breadth-first

Decompose the question into **3–5 material research questions** — the
subsets whose answers could independently move the recommendation. Gather
across ALL of them before going deep on any one (parallel researcher
delegations where natural; each delegation names its questions, the surface
facts, and the exclusions).

### 4. Rank gaps

After the breadth pass, list what is still unknown and **rank each gap by its
potential to change the recommendation** — not by how interesting it is, nor
by how easy it is to close. Record the ranking in the packet.

### 5. Selective depth

Take **at most 2 consequential gaps** through **one** depth pass each
(primary sources, specs, release notes, measured numbers — whatever the gap
demands). Do not spiral: one pass means one focused round of reads per chosen
gap, then re-assess.

### 6. Claim audit

For every claim that would materially affect the recommendation, classify it
as exactly one of:

- `supported` — EITHER (corroboration) ≥2 independent NON-derivative source
  families, excerpt read, fresh enough, OR (direct support) a normative claim
  restating an authoritative specification/source-of-record, backed by a
  `full`-depth read of that source — see the direct-support rule below
- `partial` — evidence exists but is thin, dated, single-family, or imprecise
- `contradicted` — credible sources disagree; record both sides
- `unsupported` — asserted somewhere, but nothing you actually read backs it
- `inference` — reasoned by you or a source; no direct evidence

Audit rules:

- **Class precedence (conservative):** `supported` requires ≥2 independent
  NON-derivative source families. A single-family empirical claim — even one
  backed by a strong primary source — is `partial`, with the single-family
  basis noted: ambiguity could silently inflate confidence, and the tie goes
  to the lower class. Derivative/corroboration-only sources and aggregations
  of snippets (however many sites the aggregation spans) NEVER count toward
  the ≥2-family threshold; derivative corroboration lifts a claim to
  `partial` at best (noted as derivative-corroborated), never to
  `supported`.
- **Direct support vs corroboration (the one exception):** a **normative**
  claim — one that restates what an authoritative specification or
  source-of-record *says* (e.g. "the v2 docs specify four search
  providers") — may be classed `supported` on a single `full`-depth read of
  that authoritative source, with the basis note
  `direct support (authoritative spec, single family)` in the claim's
  notes. This keeps direct claim support visibly distinct from independent
  corroboration: derivative duplication never converts direct support into
  broader independence, and corroboration sightings stay in their labeled
  section — they add context, never confidence. Empirical/contested claims
  never qualify; for them the conservative ≥2-family rule above governs.
- **Source-family independence:** derivative sources (aggregators, mirrors,
  syndications, blog posts restating a vendor doc) are NOT independent —
  record the family per evidence record and count families, not URLs.
- **Contradictions:** name them explicitly; a packet with zero recorded
  contradictions across a contested question was not audited.
- **Staleness:** check each consequential claim against the frame's freshness
  requirement, and against the source version/date when available.
- **Binding:** every consequential claim binds to evidence IDs whose excerpts
  you actually read at the stated depth. Claims may not bind to snippet-only
  locators; a locator that was never fetched is not evidence.
- **Evidence-ID binding rule:** a claim's evidence column may contain ONLY
  Evidence-record IDs matching `E<digits>`. Attempted reads, snippet or
  corroboration sightings, and descriptive pseudo-IDs (e.g.
  `E6-derivative-stale`) are NEVER claim evidence — prose notes may mention
  them, but never as support. The finalization check (step 7) enforces this
  mechanically.

### 7. Finalization check (mandatory, before stop)

Run the packet validator on the run packet, fix every reported violation,
re-run until PASS, and record the result in the packet
(`finalization_check: PASS <validator invocation> @ <date>` in the
`## Final report` section, OUTSIDE the report fence — it is packet
metadata appended after validation, not text returned to the caller):

```sh
vh-agent-harness exec bash .opencode/skills/deep-research/scripts/validate-packet.sh \
  tmp/agent-runs/<alias>/deep-research/<run-id>/run-packet.md
```

(Always through the harness `exec` wrapper — a bare `sh <script>` has no
allowlist entry in agent sessions and stalls on a permission prompt.)

The check verifies **structural invariants only** — evidence-row shape,
`E<digits>` referential integrity of claim evidence columns, `supported`
validity (the single-record `direct support` basis note + full depth, OR
≥2 cited records spanning ≥2 distinct `source_family` values), a fenced
`## Final report` section (the verbatim report inside a bare ````
≥4-backtick fence — 3-backtick and language-tagged lines inside it are
literal content), and fenced-report locator
containment — and does NOT change stop-reason semantics:
`sufficient_evidence` never implies every secondary question was
resolved.

### 8. Decide & stop

Return a **decision-oriented packet**, not a link dump:

- the recommendation (or, honestly, "cannot recommend yet")
- what would change it (the concrete findings that would flip it)
- remaining uncertainty, tied to open gaps

and close with exactly one **stop reason**:

- `sufficient_evidence` — the recommendation is stable against the ranked
  gaps; nothing plausible flips it
- `budget_exhausted` — the budget ran out with unresolved work remaining
  (name the unfinished gaps and their exact next actions)
- `capability_blocked` — a required discovery query or source read could not
  run (name the failed surface and what it blocked)

## Pilot limits (instructions, not controls)

| Limit | Value |
|---|---|
| Wall-clock budget | 20 min (default; caller-adjustable via `budget=`) |
| Discovery queries | ≤ 6 |
| Distinct source reads | ≤ 12 |
| Depth (gap) passes | 1 |
| `researcher` delegations | ≤ 2 |

These bound the pilot's cost. They are enforced only by your own recording:
log usage in the packet as you go. If you hit a limit with the question still
open, stop honestly (`budget_exhausted`) rather than silently exceeding it —
exceeding a limit is fine ONLY when the caller explicitly raised the budget.

## Resume semantics (logical, not exactly-once)

`/deep-research resume <run-packet-path>`:

1. **Read the packet first.** Treat it as the claim of record, not as truth.
2. **Revalidate mutable premises** — re-derive from current ground truth
   anything in the packet that could have changed (the frame's decision may
   have been made elsewhere; a pinned source may have moved; a prior
   capability failure may have healed). Record revalidation outcomes.
3. **Continue unfinished gaps** using their recorded exact next actions.
4. **Never repeat** completed queries or already-read sources — the packet's
   query and read logs are the dedup index. A failed read MAY be retried once
   after a revalidated surface change; record the retry as new.

Resume is logical resume: the goal is no wasted repetition, not a guarantee
that exactly-once semantics held across the interruption.

## Output honesty

- State evidence tier honestly: how many queries, how many reads, which
  families — never dress a skim as a survey.
- `sufficient_evidence` claims are scoped to the frame's freshness and
  exclusions, not to the universe.
- Nothing in this procedure writes outside `tmp/` except an explicitly
  requested `promotion=` target under `researches/`.
