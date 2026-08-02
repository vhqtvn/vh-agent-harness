# 2026-08-02 — Three-tier DEFER/follow-up routing model (definition memo)

- **Status:** DECIDED-as-DEFINITION (research + design memo). Read-only pass; no
  machinery, skill, card, or code is created by this memo. The single tier whose
  value could not be falsified-grounded is declared `not-demonstrable` below; it
  is recorded as a definition + an open operator decision, NOT as a shipped tier.
- **Supersedes / extends:** None. Composes with (does NOT reopen) the defer-liveness
  gate (`2026-07-30-defer-liveness-release-gate.md`), the promotion Definition of
  Ready (AGENTS.md §"DEFER / follow-up curation"), and the resolve-first front gate
  (`2026-08-02-resolve-first-defer-processing.md`, skill at
  `.opencode/skills/resolve-first/SKILL.md`).
- **Scope:** a deterministic routing rule + tier definitions an agent applies per
  candidate WITHOUT `/debate`. Read-only: the only mutation is this file.
- **Method:** cite-or-die. Every claim resolves to one of the sources listed in
  §"Source policy" below. The falsification methodology (§4) is the load-bearing
  crux; classes that could not be grounded are dropped or declared
  `not-demonstrable`, never shipped as invented.

## Source policy (every claim below resolves to one of these)

- `researches/decisions/2026-07-30-defer-liveness-release-gate.md` — §F1-D1 decay
  evidence (the population-scale instance: 16 `draft` cards had fired `path_touched`
  targets in `v0.18.0..HEAD` and would have shipped without a forced verdict; one
  erratum card's trigger fired on three consecutive releases and stayed `draft`).
  **Ground-truth for which parked items actually mattered vs decayed.**
- `AGENTS.md` §"DEFER / follow-up curation" (lines 342–385) + §"Picking contract
  (R1)" (lines 387–392) — promotion Definition of Ready (DoR) + "fog vs ticket"
  triage test + "holding area is transport, not truth."
- `.opencode/skills/resolve-first/SKILL.md` — front gate: exactly three legal
  outputs (`landed-this-session` / `decided-to-verdict` / `defer-with-trigger`,
  lines 94–98) + the four-tag whitelist (lines 183–205).
- `.local/coordinator/tasks/` — the live cards (empirical sample of the "global
  pile"). Runtime-derived count in §1.
- `researches/decisions/2026-08-02-resolve-first-defer-processing.md` lines 200–235
  — the **Test-A anchor**: the settled verdict that the resolve-first pilot's
  unrun-pilot obligation is NOT a defer (it is an S2-promotion-gate-owned
  milestone-gate record).
- `.opencode/skills/contract-invariant-audit/SKILL.md` (§"S2 record", lines
  303–310) and `.opencode/skills/formal-verification/SKILL.md` (§"S2 record",
  lines 321–336) — the S2-hold **form** the anchor cites as parity.
- `.opencode/scripts/check-defer-triggers.js` — the back-end predicate grammar
  (exactly `path_touched(<path>)` and `after_tag(<tag>)`; lines 74–88, 301–359;
  anything else = `unknown-predicate` in promoter mode, `evaluator-error` in
  release mode).
- External two-tier practice, used ONLY as the promotion-criterion TEST (never to
  invent classes): code-local TODO/FIXME vs tracked issue; GTD "someday-maybe" vs
  "next-action"; engineering debt registers. Recurring test criterion: *"does
  someone other than the author need to know, AND will it survive the author's
  memory?"*

## Path-resolution map (exact / replaced / missing)

All cited paths are **exact** (verified at runtime 2026-08-02):
`researches/decisions/2026-07-30-defer-liveness-release-gate.md`;
`researches/decisions/2026-08-02-resolve-first-defer-processing.md`;
`AGENTS.md`; `.opencode/skills/resolve-first/SKILL.md`;
`.opencode/skills/contract-invariant-audit/SKILL.md`;
`.opencode/skills/formal-verification/SKILL.md`;
`.opencode/scripts/check-defer-triggers.js`; `.opencode/scripts/state-lib.js`;
`.local/coordinator/tasks/` (49 cards). None replaced, none missing. (Minor
observation, not a settled-assumption contradiction: the task brief said "there
are already 4 `2026-08-02-*` entries"; the runtime `find` shows **3**
(`defer-liveness-provenance-scope-divergence`, `formal-verification-agent-proof-skill`,
`resolve-first-defer-processing`). The proposed slug is clear either way.)

---

## §1 — Current-state map (what each surface IS today)

| Surface | Lifetime | Owner | Truth / transport | Existing gate(s) |
|---|---|---|---|---|
| **Session chat / checkpoints** | ephemeral; pruned by compaction | the session | neither (invisible to the back end) | none — resolve-first rule-#1 targets this as the #1 loss failure mode (`.opencode/skills/resolve-first/SKILL.md` lines 232–236) |
| **`.local/coordinator/tasks/`** ("global" pile, operator's term) | persists until manually retired or promoted | coordinator (created via `/write-task`) | **transport, not truth** — "unpromoted candidates may be lost" (AGENTS.md lines 357–370); retire = `rm` the gitignored file or `/task-delete` | none into it; the DoR governs OUT of it (§"Promotion Definition of Ready", AGENTS.md lines 377–382); the release gate reads it directly (`loadLivenessCards`, `2026-07-30` memo lines 44, 71) |
| **`docs/planning/backlog.md`** | durable; archived, never silently deleted | the promoter (agent edits its own rows) | **committed truth** — stable-ID rows | IN: the DoR (trigger fired + concrete area + file scope + validation plan + clear slice + provenance); OUT: `done` / `cancelled` with notes |

### Runtime-derived card count (re-derived, NOT cited from memory)

Query (read-only, jq over the live pile):

```
$ for f in .local/coordinator/tasks/*.json; do jq -r '.status // "NO_STATUS_FIELD"' "$f"; done | sort | uniq -c
     46 draft
      3 ready
$ find .local/coordinator/tasks -name '*.json' -type f | wc -l
49
```

**Authoritative count: 49 cards = 46 `draft` + 3 `ready` + 0 `working`.** (This
refutes the two earlier eyeballs: the coordinator's "28" undercounted; the
sibling session's "48 (45 draft + 3 ready)" was off-by-one on the draft line —
the live count is 46 draft, 49 total.)

### Empirical characterization of the pile (runtime-derived)

- **Trigger grammar in practice** — of the cards carrying a `trigger:` owner_note,
  only **~15 use a legal predicate** (`path_touched(<path>)` ×14, `after_tag(<tag>)`
  ×1). The rest use `always_before(...)`, `evidence_obtained(...)`,
  `behavioral-completion-claimed(...)`, free-form prose, ORed chains, or
  `path_touched(<glob>)` — **all `unknown-predicate` / malformed in the back end**
  (`check-defer-triggers.js` lines 301–359), so they can **never mechanically fire**
  and can **never mechanically promote**. They are parked under fake triggers: the
  exact "prose-only defer" failure mode resolve-first rule-#1 forbids
  (`.opencode/skills/resolve-first/SKILL.md` lines 232–236).
- **Whitelist-tag coverage: ZERO.** No card in the pile carries any of the four
  resolve-first whitelist tags. The skill shipped 2026-08-02 (`2026-08-02-resolve-first-defer-processing.md`
  line 3); the pile predates it. The model defined here must therefore be
  self-consistent with a back end whose live population has not yet been
  re-tagged.
- **`studied:` dates span 2026-07-06 → 2026-08-02** — roughly one month of
  accumulation with no automated GC. One representative card
  (`defer-release-ceremony-rm-rebind.json`) explicitly records a **retired**
  `path_touched(scripts/release-tag.sh)` trigger on the grounds that it "fires
  every release and carries no information" — a card-level acknowledgment that an
  always-firing trigger is information-free, the §F1-D1 decay mode in miniature.

---

## §2 — Confusion named precisely (where routing is non-deterministic today)

1. **Gate 2 (.local → backlog) is the most confused.** Three distinct things get
   parked in `.local/` under the same `draft` status and are not distinguished by
   any existing field:
   - **(a) genuinely-deferred work** — whitelist-blocked, waiting on a real
     trigger;
   - **(b) in-progress multi-session work** — NOT deferred at all; it is active
     work that belongs in a `backlog.md` `in_progress` row or a checkpoint, but
     gets parked in `.local/` as if it were a defer because the promoter has not
     applied the DoR;
   - **(c) information-free-trigger cards** — parked under `always_before` /
     `evidence_obtained` / free-form triggers that can never mechanically fire, so
     they sit forever (the ~34/49 majority). They are neither deferred work nor
     in-progress work; they are malformed transport.

   The DoR (AGENTS.md lines 377–382) is the only existing rule for promoting OUT
   of `.local/`, and it gates on a fired trigger + specifiability — so (a) can
   promote, (b) is in the wrong pile entirely, and (c) can never promote. An
   agent looking at the pile cannot tell which is which from the status field
   alone. **This is the non-determinism: the operator must hand-triage because
   the pile conflates three kinds under one status.**

2. **No boundary between DEFERRED WORK and MILESTONE-GATE RECORDS.** Without it,
   every S2-hold (an unrun pilot recorded as committed canon) *looks* like a
   prose-only defer, tempting an agent to card it. The settled call
   (`2026-08-02-resolve-first-defer-processing.md` lines 200–235) had to be
   argued case-by-case; the tier model must make it fall out of the definition
   (Test A/B).

3. **No session binding on parked cards.** The sample card shows
   `session_aliases: []`, `active_session_alias: null` — a parked card carries no
   record of which session surfaced it, so there is no "this outlived its
   session" signal and no deterministic session-end cleanup. Cleanup is purely
   manual (`rm` / `/task-delete`), which is why the pile only grows.

---

## §3 — Tier definitions (including the first-class boundary)

> **First-class boundary (Test B — load-bearing).** Two domains are DISTINCT and
> must never be conflated:
>
> - **DEFERRED WORK** — work-to-be-done-later, gated by a path/tag trigger, owned
>   by the **defer-card mechanism**. Its lifecycle is capture → trigger-fire → DoR
>   → backlog. It lives in the defer tiers (Tiers 0/1 below) and is the ONLY thing
>   the routing rule in §5 admits.
> - **MILESTONE-GATE RECORD** — a committed canon record (e.g. an S2-hold's "S2
>   record" + `behavioral-closure` token) that **withholds a PROMOTION pending
>   evidence**, owned by the **promotion-gate mechanism** (not the defer-card
>   mechanism). It is neither deferred work nor work lost in prose. The work it
>   withholds (running a pilot, gathering evidence) is on the *promotion path of
>   its own gate*, not a defer to be routed. It is recorded in canon in the same
>   form as `contract-invariant-audit` (`.opencode/skills/contract-invariant-audit/SKILL.md`
>   lines 303–310) and `formal-verification` (`.opencode/skills/formal-verification/SKILL.md`
>   lines 321–336) record their S2-holds.
>
> **Distinguishing test:** *"Does it withhold a promotion pending evidence owned
> by a named gate mechanism (→ milestone-gate record, no card), or does it defer
> concrete work gated by a path/tag trigger (→ deferred work, route in §5)?"*
> This single test is what makes the §5 anchor reproduce cleanly.

### The three tiers (deferred-work domain only)

| Tier | Name | Purpose | Lifetime | Owner | What lives-and-dies here |
|---|---|---|---|---|---|
| **0** | **session-owned defers** | the birthplace of a `defer-with-trigger` card; the session where resolve-first runs its three-way | bound to a session identity; **default = GC'd at session end** unless promoted | the session that surfaced the candidate | resolve-first outputs 1 & 2 (resolve-now) leave **no residue** here — they shrink the pile before it forms; only output 3 creates a Tier-0 card. **⚠ value `not-demonstrable` — see §4/§6; do NOT build until the prerequisite is decided.** |
| **1** | **`.local/coordinator/tasks/`** ("global" pile) | cross-session transport; the coordinator's intake inbox | persists until retired or promoted; **transport, not truth; may be lost** | coordinator | `draft`/`ready` cards that outlived their surfacing session (Tier 0 → Gate 1) OR were created directly (today's behavior, when Tier 0 is absent) |
| **2** | **`docs/planning/backlog.md`** | committed truth; the work queue | durable; archived | the promoter | stable-ID rows; reached ONLY via the DoR (Tier 1 → Gate 2) |

**In-progress multi-session work is NOT in any defer tier.** It belongs in a
`backlog.md` `in_progress` row directly (AGENTS.md §"Agent update requirements",
lines 394–411), reached by the normal row-creation path — NOT by defer promotion.
Naming this explicitly dissolves the §2.1(b) confusion: the promoter who parked a
multi-session slice in `.local/` put it in the wrong domain; the fix is to move
it to a backlog row, not to build a promotion class for it.

---

## §4 — Evidence-grounded promotion criteria for BOTH gates (falsification methodology)

Per the HARD CONSTRAINT: each proposed class was (1) grounded in §F1-D1, (2)
cross-checked against DoR/whitelist/fog-vs-ticket (KEEP/MERGE/REFUTE), (3) tested
against the external criterion, (4) given a machine-checkable trigger where one
exists or an honest "no clean trigger fits" flag, (5) DROPPED or declared
`not-demonstrable` if ungrounded. An independent adversarial pass (debate
subagent) falsified the three structural claims; its verdicts are folded in
below.

### Gate 2 — `.local/` (Tier 1) → `backlog.md` (Tier 2)

| Class | §F1-D1 ground? | Cross-check | External criterion | Trigger | Verdict |
|---|---|---|---|---|---|
| **2.A trigger-fired + DoR-met** | **YES** — the 16 fired-`path_touched` cards ARE this class; they "would have shipped without a forced verdict" (`2026-07-30` memo line 18). This is the population-scale instance of "a parked item that actually mattered later." | KEEP — this IS the DoR verbatim (AGENTS.md lines 377–382) | PASS — a fired trigger means the release diff touched the relevant path; the release process needs to know, and it survives the author's memory | **machine-checkable**: `check-defer-triggers.js` promoter mode (`path_touched(<path>)` / `after_tag(<tag>)`) | **GROUNDED — ship.** The one class with both evidence and a clean trigger. |
| **2.B fog that sharpened to a precise ticket** | NO direct instance (decay evidence is about fired triggers, not fog sharpening) | MERGE into the DoR's "concrete area + clear slice" — grounded in the **"fog vs ticket"** triage test (AGENTS.md lines 371–376): ticket-ready = "you can state the question precisely now" | PASS — once precise, others need to know | **no clean mechanical trigger** (sharpening is a cognitive act); re-derivation is operator/promoter-driven | **GROUNDED in AGENTS.md policy (not §F1-D1), triggerless — ship with the honest "no clean trigger fits" flag.** |
| **2.C cross-session continuity (work too big for one session)** | NO | REFUTE as a defer-class — a multi-session slice is **in-progress work, not deferred work**. It belongs in a `backlog.md` `in_progress` row via the normal path (AGENTS.md lines 394–411), NOT via defer promotion. | (moot — wrong domain) | n/a | **DROP from defer criteria.** This conflation IS the §2.1(b) confusion. |
| **2.D capacity / batching** | NO | REFUTE — resolve-first NON-reason: "bare capacity/batching NOT a standalone reason; rider only on `blocked-on-sibling-slice`/`pure-future-watch`" (`.opencode/skills/resolve-first/SKILL.md` lines 216–217) | (fails) | n/a | **DROP.** |

**Gate 2 net result: ONE grounded class (2.A) with a clean trigger; ONE
policy-grounded triggerless class (2.B); TWO refuted/dropped.** This is the crux
fix: Gate 2 admits only trigger-fired-DoR-met (machine) or fog-sharpened-to-ticket
(operator judgment). Everything else is either in-progress work (wrong domain,
→ backlog row directly) or a non-reason (drop).

### Gate 1 — Tier 0 (session-owned) → Tier 1 (`.local/`)

Gate 1 is **conditional on Tier 0 existing.** Because Tier 0's value is
`not-demonstrable` (see Claim-1 falsification below), Gate 1 is recorded as a
definition, not a shipped gate.

| Class | §F1-D1 ground? | Cross-check | Trigger | Verdict |
|---|---|---|---|---|
| **1.A genuine whitelist-block that outlives the session** | NO — §F1-D1 decay is release-bound (fired triggers not getting verdicts), NOT session-bound | grounded in the resolve-first **whitelist** (the 4 tags; `.opencode/skills/resolve-first/SKILL.md` lines 183–205), not in decay evidence | the card's own carried `path_touched`/`after_tag`, OR honest "no mechanical trigger" for `operator-reserved-signoff` | **CONDITIONAL on Tier 0.** If Tier 0 is built, this is its sole promotion class. Absent Tier 0, it does not exist (output 3 → Tier 1 directly, today's behavior). |

**Claim-1 falsification (Tier 0 session-GC value) — REFUTED by the adversarial pass:**
the §F1-D1 decay mode is "OPEN cards with fired triggers not getting forced
verdicts," NOT "cards persisting after their session ended." Session-end GC does
not touch that failure — the 16 decayed cards would still decay under GC (they
would either persist into Tier 1 and decay there, or be GC'd *before* the release
gate ever sees them, which is **worse**: silent debt deletion). The "deterministic
GC inverts the default" framing is therefore **not grounded in the demonstrated
failure**. The claimed value (a) does not hold against §F1-D1; claimed value (b)
"cleaner two-gate promotion" has no demonstrated failure of the current single-gate
model to ground it. → **Tier 0 value = `not-demonstrable`.** It is recorded as a
definition + §6 open decision, NOT built. If ever built, GC must be hygiene-only
AFTER a resolve-first trace exists (commit / decision artifact / valid triggered
card), never a substitute for disposition.

### Claim-2 falsification (DEFERRED WORK vs MILESTONE-GATE RECORD boundary) — SURVIVES
The adversarial pass confirmed: without the boundary every S2-hold looks like a
prose-only defer; WITH it the anchor falls out at the first routing step (§5).
This is a genuine load-bearing control-surface distinction, not a cosmetic label.
**Ship as the first-class boundary in §3.**

### Claim-3 falsification (3 outputs ↔ 3 tiers) — INCONCLUSIVE → resolves to TODAY-form
The mapping does NOT create a forbidden fourth "resolve later" branch: outputs 1
& 2 are *resolutions* that leave nothing parked (their "homes" are the commit /
the decision artifact, not a tier); output 3 is the sole card-producing path.
**In TODAY's form (no Tier 0)** the mapping is clean and self-consistent with the
shipped skill: output 3 → Tier 1 card directly, exactly as
`.opencode/skills/resolve-first/SKILL.md` lines 227–230 specify. **The Tier-0-first
form is conditional and unevidenced** — the live pile has zero whitelist tags, so
real-world self-consistency of conditional routing is unproven, and introducing a
session-owned PRE-card state without a mandatory persisted trace would recreate
the prohibited invisible defer. → The model's deterministic routing (§5) is
written in the **TODAY form**; the Tier-0 extension is recorded as the §6
conditional.

---

## §5 — Deterministic routing rule (an agent applies this per candidate, NO `/debate`)

Pre-condition: the candidate has passed resolve-first **STEP 0** (premise
verified; ~1/3 evaporate here — `.opencode/skills/resolve-first/SKILL.md`
lines 128–135).

```
ROUTING(candidate):
  # ---- STEP R0: the first-class boundary (Test B) ----
  IF candidate withholds a PROMOTION pending evidence owned by a named
     gate mechanism (e.g. an unrun pilot blocking S2 promotion,
     recorded in canon as an S2-record + behavioral-closure token):
       => MILESTONE-GATE RECORD. NOT a defer. DO NOT CARD. EXIT.
         (owned by the promotion-gate mechanism; the work is on its
          promotion path, not a defer to route.)

  # ---- STEP R1: resolve-first three-way (the front gate) ----
  IF executable-now (small fix OR clear scope+approach):
       => LAND THIS SESSION (resolve-first output 1). Nothing parked.
  ELIF needs-a-decision / unclear-approach / reframe:
       => DRIVE TO VERDICT NOW via /debate or /solution-brief (output 2).
          Nothing parked.
  ELIF genuinely-blocked by EXACTLY ONE whitelist tag
        (blocked-on-absent-evidence | blocked-on-sibling-slice |
         pure-future-watch | operator-reserved-signoff):
       => DEFER-WITH-TRIGGER (output 3). GO TO STEP R2.
  ELSE:
       => NOT a defer (a NON-reason: marginal value / review cost / size /
          bare capacity / "unclear approach"). Either DO IT or DROP it.
          Do NOT card.

  # ---- STEP R2: where does the output-3 card live? ----
  IF Tier 0 exists (PREREQUISITE MET — see §6):
       => create the draft card in TIER 0 (session-owned), carrying the
          whitelist tag + a legal trigger (path_touched/after_tag) OR an
          explicit "no mechanical trigger, operator-driven" note for
          operator-reserved-signoff.
       => at session end: Gate 1 promotes IFF class 1.A (the block
          outlives the session); else GC (hygiene-only, after a
          resolve-first trace exists — never a disposition substitute).
  ELSE (TODAY form — Tier 0 absent / not-demonstrable):
       => create the draft card directly in TIER 1 (.local/), exactly as
          .opencode/skills/resolve-first/SKILL.md lines 227–230 specify.

  # ---- STEP R3: Gate 2 (.local -> backlog) — run by the promoter ----
  IF class 2.A (trigger FIRED + full DoR: concrete area + file scope +
        validation plan + clear slice + provenance):
       => PROMOTE to backlog.md as a stable-ID row.
  ELIF class 2.B (fog has sharpened to a precise ticket; no mechanical
        trigger but the question is now statable precisely):
       => PROMOTE on operator/promoter judgment (no clean trigger;
          record that explicitly).
  ELSE:
       => stays in .local/ (transport) or is retired (rm /task-delete).
```

### Test A — anchor reproduction trace (the load-bearing worked example)

**Candidate:** the resolve-first pilot's *unrun-pilot obligation* (S2/core
promotion withheld pending pilot evidence; recorded as memo prose in
`2026-08-02-resolve-first-defer-processing.md` §"S2 record" lines 192–235, paired
with a `behavioral-closure` token).

Run through ROUTING:

- **STEP R0:** Does it withhold a promotion pending evidence owned by a named gate
  mechanism? **YES** — the S2-promotion-gate mechanism owns the eight evidence
  classes; the pilot-run is the evidence path to `proven`; the withholding is
  recorded in committed canon (the S2 record + the `behavioral-closure` token with
  `verdict: inconclusive / result: not-demonstrable`, lines 184–190).
  → **MILESTONE-GATE RECORD. NOT a defer. DO NOT CARD. EXIT.**

**Reproduced verdict:** "no `.local/coordinator/tasks/` card; no rule-#1
violation" — matching the settled call (`2026-08-02-resolve-first-defer-processing.md`
lines 213–225) **with no debate, from the definitions alone.** The
`operator-reserved-signoff` temptation never even reaches evaluation, because R0
exits before R1. (For completeness: even if R0 were absent, R1's whitelist check
would reject `operator-reserved-signoff` here, because that tag requires the
enabling brief DONE with only the sign-off deferred — but the entire pilot RUN is
undone, so carding would be an "invalid-tag manufactured card," exactly what
resolve-first forbids. The boundary makes the call trivial; the whitelist makes
it robust. **Test A PASSES.**)

---

## §6 — Open operator decisions (surfaced, not solved)

### O1 — Session-identity prerequisite for Tier 0 (FLAGGED, prerequisite)

Tier 0 needs a home = a session identity. Runtime finding
(`.opencode/scripts/state-lib.js`): opencode **does** mint a native stable
`ses_…` id per session, and the durable binding files ARE already `ses_`-keyed
(`sessionBindingPath(sessionID)` → `<sessionID>.json`, line 491; `defaultBinding`
sets `session_id: sessionID`, lines 738–744). The human `/start-session` alias is
a `session_name` *field* on that binding plus an index entry, not the key. So the
"bind to native id" half of the prerequisite is **closer to already-true than the
brief assumed.**

The genuinely-open question is **lazy materialization**: today
`ensureSessionBinding` (lines 2505–2557) creates a binding eagerly, so every
session litters a `<ses_id>.json`. To make Tier 0 cheap (identity always, durable
state only on first use — "identity always, durable state only on first use, to
avoid litter"), the binding must be created lazily, on first durable write, with
the `ses_` id always derivable in-memory.

**Recommendation (non-binding):** bind Tier 0 to the native `ses_` id by default
(already the key), make `/start-session` an optional friendly name (already a
field), and gate durable materialization on first use. **This is a prerequisite
the operator must decide before Tier 0 can be built.** Given Claim-1's
`not-demonstrable` verdict, the operator may instead choose to NOT build Tier 0
at all and keep the today-form routing (output 3 → Tier 1 directly) — that is a
defensible choice the evidence supports.

### O2 — Tier 0 itself: build, or keep the two-tier today-form?

The falsification could not ground Tier 0's value in §F1-D1 (Claim 1 REFUTED).
The today-form (§5 STEP R2 "ELSE" branch) is self-consistent with the shipped
resolve-first skill and needs no new machinery. **Recommendation: do NOT build
Tier 0 on the strength of this memo alone.** If the operator wants it, build it
only AFTER (a) O1 is decided, (b) a resolve-first pilot actually runs and records
positive pile-shrink evidence, and (c) a mandatory persisted trace + session-end
rule is specified so Tier 0 does not recreate the prohibited invisible defer.

### O3 — Re-tagging the existing pile (the live decay)

34/49 live cards carry triggers that can never mechanically fire. They are the
"prose-only defer" failure mode in substance. **Recommendation (independent of
O1/O2):** a one-time curation pass that, per card, either (a) rewrites the
trigger to a legal `path_touched`/`after_tag` if the block is real, (b) attaches
a whitelist tag, or (c) retires the card (`rm` / `/task-delete`) if the premise
has decayed. This is the concrete pile-shrink the resolve-first scoreboard
(`.opencode/skills/resolve-first/SKILL.md` lines 259–281) is waiting on. Out of
scope for this memo (no machinery); recorded as the next operator-bearing step.

---

## Behavioral closure

The load-bearing path is: *the routing rule reproduces the anchor verdict from
its definitions alone, and every shipped promotion class is grounded in cited
evidence with a real or honestly-absent trigger.* Verified by construction in §5
(anchor trace) and §4 (per-class falsification table). The verified seam here is
**internal consistency + cited evidence**, not a live pilot run.

```behavioral-closure
verdict: inconclusive
path: researches/decisions/2026-08-02-tiered-defer-promotion-model.md (§3 boundary + §4 promotion classes + §5 routing rule)
verifier: internal-consistency check (Test A reproduction trace; Test B boundary; Test C output mapping) + cite-or-die source resolution
command: none — this is a definition memo, not an executable change; no live routing pass was run against the pile
result: not-demonstrable
```

Honest scoping of the verdict:
- **Verified by construction (internal consistency only — NOT a `proven` verdict
  on the load-bearing path):** the §3 first-class boundary reproduces the Test-A
  anchor with no debate (§5 trace); the §4 Gate-2 classes 2.A and 2.B are grounded
  in cited evidence; Test C's today-form mapping is self-consistent with the
  shipped resolve-first skill. (The token's `verdict: inconclusive` governs; this
  bullet is scoping prose, not a verdict claim.)
- **`not-demonstrable`:** Tier 0's value (§4 Claim-1 falsification) and Gate-1
  class 1.A — NOT grounded in §F1-D1; conditional on operator decisions O1/O2.
- **DROPPED (refuted, not shipped):** promotion classes 2.C (in-progress work —
  wrong domain) and 2.D (capacity/batching — resolve-first non-reason).

No promotion class is shipped invented. No claim of `proven` is made for Tier 0.

## Non-goals (held)

- Do NOT implement Tier 0, the session-identity binding, or any new skill /
  machinery / card. This memo ships the DEFINITION only.
- Do NOT edit `AGENTS.md`, `backlog.md`, or any skill. The only write is this file.
- Do NOT create a `.local/coordinator/tasks/` card for this work (resolve-first
  principle: the work is starting now; don't manufacture a card).
- Do NOT run a pilot or a pile-curation pass (those are O3, operator-bearing).
