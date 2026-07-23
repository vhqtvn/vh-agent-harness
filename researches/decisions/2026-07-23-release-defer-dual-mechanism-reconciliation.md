# Decision: Release-DEFER Dual-Mechanism Reconciliation

**Status:** DECIDED — union fail-closed defense-in-depth
**Date:** 2026-07-23
**Commits:** `38c5c477` (manifest authority) · `5d749fd` (readiness enforcement) · `69e0104` (doctor #12 defer-liveness) · `c197171` (claims kernel)
**Ceremony:** N→R→M (note → readiness-artifact → manifest), releaser-owned

## Problem

Two independent DEFER-blocking mechanisms landed from separate tracks. Neither
references the other. A reconciliation review was requested to confirm they do
not create a dual-derivation of "open defer blocks release" that silently
contradicts.

## Mechanism A — Release wrapper (tag-time enforcement)

| Property | Value |
|---|---|
| Runs at | `scripts/release-tag.sh`, before `git tag -a` |
| Input | `.vh-agent-harness/release-defer-dispositions.json` (committed manifest) |
| Secondary input | `.vh-agent-harness/release-readiness-pass.json` (readiness-exclusive write) |
| Vocabulary | `release_relevance`(yes/no/unknown) × `disposition`(block/disclose/override_required) × `metadata_state`(valid/stale/invalid) |
| Also enforces | G0 (go test/vet/build/gofmt) + G0b (dirty tree) + G1-G5 readiness gates |
| Failure mode | "A fired DEFER with release-blocking disposition hasn't been resolved against the release arc" |
| Trust boundary | Commits at HEAD (git show, not worktree); readiness agent has exclusive write the releaser can't forge |

## Mechanism B — Doctor check #12 defer-liveness (harness health)

| Property | Value |
|---|---|
| Runs at | `vh-agent-harness doctor` |
| Input Side A | `.local/coordinator/tasks/{defer,errata}-*.json` (card `status` field) |
| Input Side B | `templates/migrations/v*.md` (migration notes, classified by git tags) |
| Vocabulary | card status closed set = `{completed, cancelled, staged}`; everything else = OPEN |
| Contradiction | OPEN card that (is errata) OR (references an existing migration note) |
| Failure mode | "An open coordinator card contradicts a shipped/about-to-ship migration-note claim" |
| Trust boundary | Reads on-disk sources via the claims kernel (non-authoritative, re-derived each call, fail-closed on malformed cards) |

## Semantic comparison

The two mechanisms have **completely different vocabularies, different inputs,
and different failure modes**:

| Dimension | Mechanism A (wrapper) | Mechanism B (doctor #12) |
|---|---|---|
| What "resolved" means | manifest record with `release_relevance: no` (or no record) | card `status ∈ {completed, cancelled, staged}` |
| What "blocking" means | manifest record `release_relevance: yes, disposition: block` | open card referencing an existing migration note |
| What it reads | committed manifest (HEAD) | `.local/` card status + migration notes on disk |
| When it acts | tag time (pre-`git tag -a`) | doctor run |

They check **different safety properties** that happen to overlap on the same
input source (DEFER cards) but look at different fields:

- Mechanism A looks at **release-arc relevance** (path_touched against the
  release arc, disposition encoded in the committed manifest).
- Mechanism B looks at **released-claim contradiction** (card status open +
  references a shipped migration note or is an erratum card).

## State matrix (cross-mechanism)

| Card state | Mechanism A | Mechanism B | Conflict? |
|---|---|---|---|
| `completed`, manifest `no+disclose` | ALLOW (disclose) | PASS (closed) | No |
| `draft`, no migration-note reference, not in manifest | No effect | PASS (no contradiction) | No |
| `draft`, references migration note, not in manifest | No effect | **FAIL** (open + contradicts) | Latent — only if doctor runs pre-release |
| `staged` (errata), references migration note | No effect | PASS (staged = closed) | No |
| `draft` (errata), references migration note | No effect (wrapper doesn't read errata) | **FAIL** (open errata) | Latent — only if doctor runs pre-release |
| In manifest as `yes+block`, `draft`, no migration-note ref | **REFUSE** (block) | PASS (no contradiction) | No (different failure modes) |

**No conflict exists today.** All current cards are either closed or do not
reference migration notes.

## Decision: union fail-closed defense-in-depth

Each mechanism owns its own failure mode. Neither should derive from the other.

**Mechanism A (wrapper) owns:** release-arc DEFER path_touched enforcement.
This is the tag-time gate that prevents shipping with a fired, unresolved,
release-relevant DEFER.

**Mechanism B (doctor #12) owns:** released-claim/erratum contradiction
detection. This is the harness-health check that catches an open card
contradicting a shipped migration note.

**Both must pass independently.** If either fails, the release should not
proceed. The mechanisms do not need to agree on vocabulary because they check
different things:

- The wrapper would MISS an open errata card (it doesn't read errata cards).
- The doctor check would MISS a release-arc DEFER that doesn't reference a
  migration note (most DEFERs reference code paths).

Making them derive from each other would weaken coverage by collapsing two
distinct failure modes into one.

## Coverage gap

**The doctor check is NOT wired into the release ceremony.** The releaser's
N→R→M ceremony (Step 3.x) does not invoke `vh-agent-harness doctor`. If the
operator only uses the release agent, an open errata card could slip through
the wrapper, which has no awareness of errata cards or migration-note
contradictions.

This is a coverage gap (doctor exists but isn't invoked at release time), not
a semantic conflict (the mechanisms don't disagree on any card's state). A
DEFER card has been filed to wire the defer-liveness check into the release
ceremony as a pre-flight step.

## Staged-errata injection hole (operator-identified)

The state-matrix row `staged (errata) → PASS on both mechanisms` has a hole:
**neither mechanism verifies the injection actually happened.**

A `staged` errata card (e.g. `errata-v0120`) means "correction queued for the
next release note." But if the next release note is cut WITHOUT injecting the
erratum text into `templates/migrations/v<next>.md`:

- **Doctor #12:** PASS — `staged` is in the closed set, so the card is not
  flagged as a contradiction. The check does not verify the erratum content
  actually landed in the target note.
- **Release wrapper:** No effect — errata cards are not in the manifest. The
  wrapper has no awareness of errata cards at all.

Both stay green. The false claim in the original released note remains
uncorrected for another release — the exact failure mode the errata card was
filed to prevent.

This is a THIRD failure mode neither mechanism covers: "staged erratum was
never actually injected into the release note it was queued for."

The DEFER card `defer-release-wire-doctor-liveness-into-ceremony` has been
extended to require:

1. The release ceremony FAILs when a staged errata card exists while an
   about-to-release (untagged) note lacks that erratum's content.
2. A mandatory inject-staged-errata step in the releaser checklist (inject
   the staged erratum text into the about-to-release migration note before
   the note commit lands).

## Non-goals

- Do NOT merge the two vocabularies into one. They serve different purposes.
- Do NOT add errata cards to the release-defer manifest. The manifest is for
  release-arc DEFERs; errata is a released-claim contradiction, not a
  release-arc finding.
- ~~Do NOT make the wrapper call doctor. The defer-liveness check should be
  invoked as a read-only pre-flight, not as a wrapper-internal dependency.~~
  **SUPERSEDED** by the wiring card `defer-release-wire-doctor-liveness-into-ceremony`
  (operator directive, 2026-07-23): read-only pre-flight / prompt prose is NOT
  enforcement. opencode caches the releaser subagent prompt per-process, so a
  prompt-only ceremony step is stale-cached and inactive for the ceremony run in
  the current session. The wrapper (`scripts/release-tag.sh`) is the sole tag
  authority and the only machine layer effective this session, so doctor (all
  checks, now including #13 staged-errata-content) MUST be a hard non-zero-exit
  gate (G0c) the wrapper enforces at the tag boundary — gated on seam-installation
  (lineage.yml presence) so it only runs where doctor is meaningful. This is the
  closing of the THIRD failure mode the memo itself identified (lines 128-138).

## Residual trust boundary

Both mechanisms share the same accepted residual: raw-shell/operator bypass
can defeat OpenCode-layer enforcement (doctor reads `.local/` directly; the
wrapper reads committed state via git). Neither provides cryptographic
producer proof. This is the same residual the DEFER handshake accepts.
