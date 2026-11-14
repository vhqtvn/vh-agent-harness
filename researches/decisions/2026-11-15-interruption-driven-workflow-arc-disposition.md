# Interruption-Driven Workflow Arc — Disposition & Provenance Record (P1)

- **Date:** 2026-11-15
- **Type:** decision record (operator-ratified disposition landing; the ONE primary record for this slice)
- **Status:** RATIFIED — operator "ok" on 2026-11-15
- **Effect:** closes the interruption-driven-workflow arc (2026-10-25 owner
  handoff → 2026-10-26 retention trilogy): records the ratified disposition
  of its five surviving task cards, and carries as committed durable text the
  verbatim owner-decision exhibits whose only other carrier is gitignored
  card state. Coordinator-side card retirement follows separately (outside
  this slice). This commit is archival — a preservation landing, not fresh
  implementation — and carries NO trailer lines, deliberately: all five cards
  sit at status `ready` (never resumed), ready-card deletion carries no
  landing check today, and trailers here would be unbuilt optionality reading
  as fresh implementation provenance (flip 1 governs adding them later).
- **Naming note (satisfied-by-rename):** the research card's declared target
  `researches/decisions/2026-10-25-interruption-driven-workflow-disposition.md`
  is satisfied by THIS record under an arc-level name and date; likewise the
  other cards' declared memo targets (`…-retention-preservation-gate.md`,
  `…-retention-gate-v2-disposition.md`, `…-retention-v3-disposition.md`,
  `…-retention-v3-design-disposition.md`). Those `target_artifact_path`
  fields predate the P1 one-record rule (e4dd6d7); under P1 this single
  record carries the arc disposition, and the landing commit bodies cited
  below carry the per-change detail.

## Provenance chain

```
owner handoff 2026-10-25 → arch-research card (created ready, owner authority)
→ phases 1–5 research (solution-brief chain; workstream interruption-driven-workflow)
→ owner reframe + authority grant, Q1/Q2 answered in-chat (m0021)
→ calibration corrections (m0025) + missing-reports verdict + design mandate (m0029; m0031)
→ owner approval m0033 → LANDED b788b438 (retention-preservation gate, 2026-10-26)
→ owner review m0051   → LANDED bb60068f (v2 corrections)
→ owner round-3 m0060  → probe (concern-1 repro + machinery inventory; uncommitted by design)
→ owner approval m0066 → LANDED cccf1b06 (v3 replacement)
→ disposition research pass (ses_e2500d218, findings) — 2026-11-15
→ disposition debate (ses_e24fa96a5; verdict: Option 1 plain) — 2026-11-15
→ operator ratification "ok" 2026-11-15 — this record is the authoritative
  carrier of the ratified disposition.
```

The debate's Option 1 plain (one disposition record: plain table, verbatim
exhibits, no trailers) and the operator's "ok" are carried by the coordinator
dispatch — parent-session convention, the same treatment the O4 posture
record (e4dd6d7) gives its operator-intent finding. The debate's receipts:
the card JSONs, the workstream notes, and the SHAs re-derived below.

## Disposition of the five cards

| Card (`task-…`) | Type | Disposition | Landing evidence |
|---|---|---|---|
| `2026-10-25t11-30-00-retention-preservation-gate` | implementation | **retire-as-landed** | `b788b438`; exactly 1 reachable `Task-Card:` trailer |
| `2026-10-26t09-15-00-retention-gate-v2-corrections` | implementation | **retire-as-landed** | `bb60068f`; exactly 1 reachable trailer |
| `2026-10-26t16-00-00-retention-v3-replacement` | implementation | **retire-as-landed** | `cccf1b06`; exactly 1 reachable trailer |
| `2026-10-25t08-54-27-interruption-driven-workflow-arch-research` | research | **decisions resolved de-facto; this record = the remaining durable disposition** | no trailer (0 grep hits); facts below |
| `2026-10-26t14-20-00-retention-v3-design-probe` | study | **substance subsumed by `cccf1b06`** | trailer-less (0 hits); no trailer claimed |

Explicit distinctions between the disposition classes:

- **Retire-as-landed (the trilogy)** — the strong form: each landing commit is
  on main with EXACTLY ONE reachable `Task-Card:` trailer line for its card,
  and each card's guarantee lives in production code (v1 gate → v2
  content-bound coverage → v3 item-enumerated records; v1/v2 machinery was
  itself replaced by v3 — `cccf1b06` is the surviving contract). All three
  cards remain at status `ready`, so the sanctioned retirement path is direct
  coordinator-side removal — the v3 closeout gate binds completed-card
  closeouts, not ready-card deletion.
- **De-facto resolution (the research card)** — no implementation landing, and
  none required. (a) The card's Part 10 owner questions (Q1 pre-delegated
  architectural decisions; Q2 throughput vs retained-understanding) were
  answered by the owner in-chat, with an operative authority grant (m0021;
  workstream next-slice.md:46-48). (b) Landing history supplies the awaited
  remedy: the card's `next_action` waited on ONE owner decision — approving
  the bounded retention lifecycle correction — and the trilogy IS that
  correction, approved (m0033, m0066) and landed. (c) The posture record
  (e4dd6d7) later set the P1 one-record shape this disposition follows. This
  record is the remaining durable written disposition; with it the card is
  fully dispositioned.
- **Subsumption (the probe card)** — its substance (concern-1 reproduction on
  the real retirement path, curated-direction receipts, the v2 machinery
  inventory, and the two deferred pins) folded into v3 by the probe's own
  constraints ("NO changes to … any production gate this round") and the v3
  card's provenance note ("PRESERVE/adapt the two pins"). It is trailer-less:
  branch-wide grep for its card id returns 0 hits; no trailer is claimed.

## Verbatim provenance exhibits

These texts survive ONLY in gitignored card state (`.gitignore:78`
`.opencode/state/`, `.gitignore:84` `.local/coordinator/`) — card retirement
destroys them, so this record is their durable carrier. Each block is the
cited card's `owner_notes[i]` byte-exactly (jq -r extraction; one unwrapped
line each — byte-fidelity over column discipline); the chat-message id labels
the owner message whose terms the note records.

**m0033 — owner approval of the retention-preservation gate** (card `…retention-preservation-gate`, `owner_notes[0]`, 1527 codepoints):

```text
OWNER APPROVAL 2026-10-25 (chat m0033, this session): 'I approve a narrowly scoped retention-preservation fix, but not the proposed A+B+gate design exactly as written.' Five correctness conditions: (1) inline counts only when verified in surviving git commit or designated durable artifact; (2) validate committed content against reachable revision, distinguish local reachability from cross-machine availability; (3) post-commit material needs existing durable destination + verified preservation before ordinary retirement; (4) routine attestation = semantic judgment, detect contradictions with explicit fields, no AI validator; (5) define compatibility for existing completed and in-flight cards. Validation: ≥1 E2E retirement regression (fresh session recovers decision + rejected alternative + owner authorization after deletion); negatives = inline-only closeout, uncommitted durable-file change, routine conflicting with explicit decision-class metadata; preserve operator force path with destructive warning. Keep solution small; prefer existing commit messages + durable docs; no ledger/archiving/no mandatory docs on ordinary tasks/no semantic engine/no new approval flow. Execute without per-step approval within constraints + existing safety/release procedures. If conditions require substantially different lifecycle architecture → STOP and bring specific recommendation. Report after implementation. Success = important engineering knowledge remains retrievable after retirement, not merely that the gate passes.
```

**m0051 — owner post-landing review; v2 correction mandate** (card `…retention-gate-v2-corrections`, `owner_notes[0]`, 2050 codepoints):

```text
OWNER 2026-10-26 (chat m0051, post-review of b788b438): direction good, review process valuable, but NOT YET SATISFIED the implementation provides the intended preservation guarantee. Primary concern: trivial satisfaction — bare {kind:commit} with default card-id marker satisfied by the landing trailer itself, EVEN with non-empty owner_notes (contradiction check only guards routine). 'The presence of explicit decision-class metadata does not necessarily imply that its content survives retirement.' Do not fix with a more elaborate marker that still proves only presence of arbitrary text. C2: routine task after NORMAL /task-review must currently promote its verdict — distinguish ordinary process approval from consequential review decisions; not every successful review is important engineering knowledge. C3: historical presence ≠ practical retrievability — assess fresh-session discoverability from card-id; demonstrate sufficiency, no automatic index/archive. C4: ~1,261 added lines per state-lib copy — reassess guarantee vs complexity; challenge current design and previous recommendation; do not keep adding validators around a weak contract; do not rewrite just for size; establish actual failure modes first. Scope: bounded correction within approved retention-preservation scope; minimal failing regressions first; actual sanctioned lifecycle; preserve safeguards + compatibility; no new approvals for routine; no committing sensitive owner notes merely to satisfy gate; no ledger/archive/engine/framework; keep template sync + test discipline; stop + recommend if materially different architecture needed. Report: which reproduced / what lost or blocked / correction + why over strongest simpler alternative / what regressions establish / what stays judgment-dependent / actual overhead routine vs decision-bearing. No round-by-round narration. Success = important decision knowledge meaningfully retrievable after retirement AND routine work stays lightweight. No additional approval needed for bounded investigation and correction.
```

**m0066 — owner approval of the v3 replacement** (card `…retention-v3-replacement`, `owner_notes[0]`, 1522 codepoints):

```text
OWNER APPROVAL 2026-10-26 (chat m0066, VERBATIM there — six conditions): 1 enumeration alone insufficient (per-item identity/disposition/carrier-from-card-id/per-item representation/provenance; card-id-only commit must not satisfy multiple items); 2 protect item inventory across lifecycle transitions (listed cases; later clean review must not erase earlier consequential decisions; sanctioned update path without ceremony; guarantee limited to recognized task state); 3 curation/sanitization useful not ceremonial (record answers what/why/reasoning/authority/omissions; proportional; no confidential copying; no invented authorization; refuse ordinary retirement when no safe representation; force = emergency only); 4 demonstrate recovery not gate acceptance (two-notes reproducer primary; per-item locatable; curated vs verbatim distinguished; refusals for missing carriers/stale inventory; routine lightweight; include two uncommitted pins; fresh-process git-only recovery); 5 simplify aggressively but preserve load-bearing safeguards (landing proof, reachability, committed-state, arg/path safety, fail-closed, legacy compat, force, gates); 6 challenge the model during implementation; counterexample → reassess not auto-v4; authorized to implement/test/review/land without routine approval; escalate only for materially different architecture/authority/privacy. Final report: guarantee change / demonstrated failures+counterexamples / semantic limits / ACTUAL complexity / operational costs / stability assessment.
```

**m0029 — protective instruction + pilot limitation + design mandate** (arch-research card, `owner_notes[0..2]`, 685 + 279 + 454 codepoints). The instruction lapsed by its own terms: it bound "until the retention-preservation correction is decided and landed", which the trilogy satisfied (workstream next-slice.md:88 records it SATISFIED by `b788b438`); preserved here as provenance, not as a live obligation.

```text
2026-10-25 OWNER PROTECTIVE INSTRUCTION (m0029 §6, OPERATIVE): until the retirement-preservation correction is decided and landed, no task retirement may destroy the last surviving copy of decision-class material (significant decisions, materially-rejected alternatives, owner authorizations with provenance, load-bearing assumptions, unresolved objections, measured outcomes). Promote via existing durable mechanism (researches/decisions or docs/checkpoints committed file) before retiring; if the correct destination is unresolved, report to owner BEFORE destructive retirement. Bounded: NOT authorization to rewrite the generic lifecycle, bypass gates, or retain every card forever.
```

```text
2026-10-25 PILOT LIMITATION (m0029 §7): phase-1 observer only sees tasks whose evidence survives to a review touch; evidence deleted pre-review is invisible. Missing evidence is NEVER interpretable as good decision quality, successful autonomy, or absence of owner interruptions.
```

```text
2026-10-25 RETENTION DESIGN MANDATE (m0029 §1/§8): bounded design investigation of retire-on-landing destruction authorized — alternatives comparison, regression scenario, decision-oriented recommendation. NOT a major lifecycle refactor; implementation requires separate owner approval. Remedies must not create: duplicate permanent ledger, committed raw reports, transcript archive, mandatory essays for routine changes, new completion approval process.
```

**m0060 — owner round-3 message; probe mandate** (probe card, `owner_notes[0]`, 903 codepoints):

```text
OWNER ROUND-3 message 2026-10-26 (chat m0060, verbatim there): per-field coverage may destroy independent decision B while excerpt covers A — reproduce on actual retirement path; reconsider UNIT of preservation, not more validation; privacy vs verbatim-in-git incompatibility (curated/sanitized OK if honestly labeled, never falsely claim verbatim); close 3 deferred gaps with small regressions only (artifact card-id recovery, anti-laundering; rebinding pin may be moot under redesign); reassess architecture: field/excerpt-coverage-corrected vs simpler curated durable decision record + narrow mechanical existence/reachability gate, or better third approach; explain mechanical vs semantic guarantees + sensitive-info path; concise decision-oriented response; no large implementation before design choice; small clearly-justified correction may be recommended, material contract change goes to owner.
```

## Pilot digest (evidence-limited)

The arc's pilot (owner-designed, m0021) accumulated three independent
review-cost data points, recorded in the workstream at next-slice.md:86,
:102, :124:

1. **b788b438 cycle (:86)** — 5 review + 5 build rounds; the receipt protocol
   caught a RED tree (1/36 fixture contamination) all static reviews
   missed/misreported: "panel catches logic/injection/design defects;
   execution receipts catch what static review cannot."
2. **v2 cycle (:102)** — the OWNER caught what 4 review + 5 build rounds
   missed (trivial-satisfaction contract hole; routine-burden regression):
   JUDGMENT findings, not detectable by the current panel composition.
3. **v3 cycle (:124)** — specialist leaf missed scatter (generalist caught);
   coordinator cascade missed positional-ID laundering (committer gate
   caught); build owned + root-caused its own deleted-pin error. "Pattern
   holds: each layer catches a distinct defect class; no single layer
   sufficient."

That closing observation is preserved as an OBSERVATION, explicitly NOT
promoted to guidance. The three-way split: **durability** — committed now,
this record carries the data points past card retirement; **relevance** —
evidence bearing on the owner's open review-cost question (what each review
layer actually buys); **authority** — nothing: not ratified as guidance, and
no review-configuration, cascade-budget, or process change follows from this
digest (promotion requires flip condition 4). The owner's review-cost
question remains OPEN.

## Clarifying note (b7af07e anchor label)

Workstream next-slice.md:26 credits the "full Part 10 report (solution-brief
chain @ b7af07e, 2026-10-25)". `b7af07e` is the 2026-10-18
compaction-ownership disposition memo — merely HEAD when Phase 1 bound its
inventory (next-slice.md:8), not an output of this arc. The arc committed
nothing until the retention trilogy (2026-10-26); its surviving outputs are
the workstream notes, the five cards, and the trilogy.

## Open questions & reversal conditions (the four ratified flips)

Standing open questions: **owner review-cost question** — OPEN (three data
points, observation only); **trailer-less retirement custody** — ready-card
deletion carries no landing check; carrier-recognition is unbuilt optionality.

Flip conditions (condition → action), ratified with this disposition:

1. **Verified carrier-recognition contract → add trailers later.** If a
   verified contract later lets the retirement machinery recognize a
   committed record as the durable carrier for a card WITHOUT a `Task-Card:`
   trailer, trailers for the trailer-less cards (arch-research, probe) may
   be added then — never speculatively.
2. **Binding placement rule → move to checkpoint.** If a binding rule emerges
   governing where disposition records live (`researches/decisions/` vs
   `docs/checkpoints/`), this record moves (git mv) rather than forking a
   second copy.
3. **Material unresolved decisions emerge → reopen disposition.** If a
   decision this record marks resolved de-facto turns out unresolved in
   substance (e.g. a retention guarantee the trilogy does not actually
   provide), the disposition reopens via a NEW primary record — this record
   is not edited (amendments are reserved for genuine authority changes, per
   P1).
4. **Later evidence + owner review-cost resolution → promote pilot
   guidance.** If further cycles accumulate evidence AND the owner resolves
   the review-cost question, the pilot observation may be promoted to
   guidance as its own record with its own ratification.

## Contradictions

- **None detected** between this disposition and the landed state: trilogy
  SHAs, trees, and single-trailer reachability re-derived this slice match
  the workstream records; the exhibits match the card JSONs byte-exactly.
- The b7af07e labeling tension is recorded as the clarifying note above, not
  counted as a substantive contradiction: a tree-binding label in workstream
  notes, not a claim about arc output.
- The CONTRADICTION logged during the research arc itself (next-slice.md:32:
  the evidence-gated-task-card-completion memo describes a completion gate
  the code does not implement; current code governs) stands as recorded
  there, unrevisited by this disposition.

## Findings

- **(finding)**: source=git (read this slice), confidence=high, type=fact —
  the trilogy is exactly `b788b438` (tree `5d172731949b…`), `bb60068f` (tree
  `d15320c8…`), `cccf1b06` (tree `b4907a2f…`; 19 files, +6674/−5116); each
  with exactly ONE reachable trailer line for its card; arch-research and
  probe have ZERO reachable trailer hits (no trailer claimed).
- **(finding)**: source=cards + workstream next-slice.md:88, confidence=high,
  type=fact — the m0029 protective instruction lapsed by its own terms once
  the trilogy landed; the workstream records it SATISFIED by `b788b438`.
- **(finding)**: source=git (read this slice), confidence=high, type=fact —
  `b7af07e` is dated 2026-10-18 and is the compaction-ownership disposition
  memo commit; it is not an output of this arc.
- **(inference)**: source=coordinator dispatch (this slice), confidence=high,
  type=inference — the disposition research pass (ses_e2500d218), the debate
  verdict (ses_e24fa96a5, Option 1 plain), and the operator's "ok"
  ratification (2026-11-15) are carried by the dispatch; this record is
  their authoritative carrier per the parent-session convention (same as
  e4dd6d7's operator-intent finding). The research card's de-facto
  resolution rests on in-repo facts (m0021 authority grant + Q1/Q2 answers,
  workstream :46-48; landing history; P1 shape).
- **(finding)**: source=workstream next-slice.md:86,:102,:124,
  confidence=medium, type=fact (as recorded observations) — the three
  review-cost data points; their generalization ("no single layer
  sufficient") is an observation, NOT ratified guidance.

## Verification

| Claim | Verifying evidence (all re-derived from repo ground truth this slice) | Verified |
|---|---|---|
| Trilogy commits exist with stated subjects/trees/stats | `git show -s --format="%h tree=%T"` + `--stat`: `b788b43`/`5d172731949b…`, `bb60068`/`d15320c8…`, `cccf1b0`/`b4907a2f…`; stats match workstream records | yes |
| Exactly 1 reachable trailer per implementation card | `git log --branches --fixed-strings --grep="Task-Card: <id>" --oneline` → exactly `b788b43`, `bb60068`, `cccf1b0` respectively, one hit each | yes |
| Arch-research + probe are trailer-less | same grep form → 0 hits for both card ids; no trailer claimed for them | yes |
| Exhibits are byte-exact copies of card `owner_notes[i]` | per-exhibit diff of each fenced line vs `jq -r '.owner_notes[i]'` extraction → identical, 7/7 blocks (codepoints 1527 / 2050 / 1522 / 685+279+454 / 903) | yes |
| Exhibits' only other carrier is gitignored | `.gitignore:78` = `.opencode/state/`; `:84` = `.local/coordinator/` | yes |
| b7af07e = 2026-10-18 compaction-ownership memo, not arc output | `git show b7af07e --format="%ad %s" --date=short` → "2026-10-18 docs(decisions): record compaction-ownership disposition …" | yes |
| No trailer lines in this record | `grep -c '^Task-Card:'` on this file → 0; no adjacent trailer-prefix + card-id literal pair anywhere (no false future-grep signal) | yes |
| Working tree clean pre-commit | `git status --porcelain` empty at HEAD `476bfc86` | yes |
| P1 conformance (one primary record) | this slice adds exactly one record; per-card memo targets subsumed per the naming note; style mirrors e4dd6d7 | yes |
