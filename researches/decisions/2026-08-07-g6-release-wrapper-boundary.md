# Decision: G6 release-wrapper boundary — deterministic re-computation at the tag mutation

**Date:** 2026-08-07
**Status:** Accepted (record-of-decision). No code lands in this slice; this is a
record-of-decision that fixes the enforcement architecture for the G6 (skill-pilot
S2-hold) release-readiness gate at the tag-mutation boundary. The implementation
is a separate follow-up (filed as `g6-s2-hold-deterministic-wrapper-gate`); this
memo captures the citable decision that gates it.
**Basis:** the settled `debate` disposition on card
`g6-release-wrapper-boundary` (A-family recommendation: deterministic
re-computation in the wrapper, not expansion of the model-authored readiness
artifact), re-derived against current HEAD (`bb2db205`). Every cited source line
range below was re-verified against HEAD in this slice; the debate's ranges held
exactly.
**Supersedes / corrects:** the card's original premise — *"release-tag.sh consults
NO readiness verdict"* — which is **stale** against current source. The correction
is load-bearing and documented as Decision 0 below; it narrows the real gap from
"the wrapper consumes no readiness signal" to "the wrapper consumes G1–G5 but not
G6."
**See also:**
[`./2026-07-23-release-defer-dual-mechanism-reconciliation.md`](./2026-07-23-release-defer-dual-mechanism-reconciliation.md)
(the G7/DEFER dual-mechanism reconciliation — the structural precedent this memo
generalizes to G6: a model-judgment gate that gains wrapper-side deterministic
enforcement WITHOUT the model's conclusion becoming transition authority).
[`./2026-07-30-defer-liveness-release-gate.md`](./2026-07-30-defer-liveness-release-gate.md)
(defer-liveness at the release boundary — the same "re-derive at the controlled
transition, do not trust a prior marker" discipline this memo applies to G6).

## Non-goal (load-bearing)

**This decision does NOT block the next release.** No S2 hold is active today, so
G6 passes and the next release is unblocked. This memo records the enforcement
architecture for a future release in which a skill is held under S2 (`s2-hold:`
backlog token with a `PENDING` verdict). It changes no behavior; the
implementation follow-up fires when (a) the S2 record/evidence parsing contract is
pinned AND (b) `scripts/release-tag.sh` is next touched.

## Framing

The release ceremony has **two mutation-capable surfaces** between a
readiness-blocked release and a tag:

1. the **releaser agent** (`templates/overlays/release/agents/releaser.md`), which
   runs the N→R→M ceremony and delegates the tag mutation; and
2. the **release-tag wrapper** (`scripts/release-tag.sh`), the shell script that
   actually runs `git tag -a`.

The harness-release-readiness agent is **read-only / chat-only** by invariant and
permission: it authors the model-gate verdicts into an exclusive-signing artifact
(`.vh-agent-harness/release-readiness-pass.json`) and populates a `handoff_to_releaser`
report field, but it never tags, never commits, and its model output is **never**
transition authority (the sacred invariant). The question this decision answers is:
**at which surface, and by what mechanism, is a readiness-blocked release
mechanically prevented from being tagged?**

The card was created with the premise that the wrapper consults *no* readiness
verdict. Re-derivation against current HEAD refutes that premise for G1–G5 and
isolates the real gap to G6. The decision follows the correction.

## Decision 0 — Premise correction (the wrapper DOES consume G1–G5; the gap is G6-only)

The card's original premise — *"release-tag.sh consults NO readiness verdict"* —
is **stale**. Re-verified against HEAD `bb2db205`:

`scripts/release-tag.sh:647–833` is a **"release readiness-pass artifact gate
(G1–G5, model-driven)"** that independently consumes the committed readiness
artifact `.vh-agent-harness/release-readiness-pass.json`:

- **Reads from `HEAD:`, not the worktree.** Existence via `git cat-file -e
  HEAD:.vh-agent-harness/release-readiness-pass.json` (line 718); bytes via
  `git show HEAD:.vh-agent-harness/release-readiness-pass.json` (line 742). G0b
  has already refused a dirty tree (lines 558–564), so worktree-vs-HEAD cannot
  bypass this gate.
- **Requires the N→R→M commit sequence.** The release ceremony sequences
  note → artifact → manifest. At tag time HEAD is the manifest commit (DEFER
  handshake), the artifact commit is at `HEAD^` (line 702), and the release-prep
  commit is at `HEAD^^` (line 709). Shallow history → fail-closed (lines 702–714).
- **Requires the artifact commit to change only the artifact path.**
  `git diff --name-only HEAD^^ HEAD^` must equal exactly
  `.vh-agent-harness/release-readiness-pass.json` (lines 728–734) — a single-path
  child commit mirroring the DEFER manifest ceremony one level deeper.
- **Binds `artifact.commit_sha` to `HEAD^^`** (the release-prep commit the
  readiness agent evaluated), NOT HEAD (lines 819–824). This refuses stale
  artifacts evaluated against an older commit.
- **Fails closed on missing / malformed / stale artifacts.** Missing artifact
  (line 718); invalid JSON (lines 753–756); wrong `schema_version` (761–763);
  non-40-hex `commit_sha` (765–767); missing `model_gates` object (769–771);
  missing gate (773–776); unknown verdict value (778–782); **unknown gate keys
  rejected** (787–792 — a new gate emitting `blocked` cannot pass by being
  silently dropped); validator crash (lines 798–800).
- **Refuses any `blocked` / `skipped` G1–G5 gate** before `git tag -a`
  (lines 828–833). The tag mutation (`git tag -a -F …`) runs at line 902, **after**
  every gate above.

The validated gate set is hard-coded at line 747:
`["G1_coverage","G2_significance","G3_docs","G4_visibility","G5_curated_note"]`.
**G6 is absent from this array.** A direct invocation of `release-tag.sh` that
satisfies G0/G0b/G0c/G7 and carries an all-ready G1–G5 artifact **cannot** be
refused by this gate for a G6 reason.

**Conclusion of the correction:** the original premise ("consults NO readiness
verdict") is false. The wrapper independently consumes the committed readiness
artifact for **G1–G5** and a direct wrapper invocation **cannot** bypass a blocked
G1–G5 readiness artifact. The real gap is narrower: **G6 only.**

## Decision 1 — The real gap: G6 / S2-hold readiness is handoff-only

G6 (skill-pilot S2-hold evidence) is enforced at exactly **one** surface today:
the readiness agent's own `handoff_to_releaser` report field. It is **neither
consumed nor recomputed by the wrapper**:

- The readiness agent emits `ready: no` + `handoff_to_releaser: null` on a G6
  blocker (`harness-release-readiness.md:449`, `:856`). This blocks the **handoff**.
- G6 is **not** in the artifact's `model_gates` object. The readiness agent's own
  schema doc states this explicitly (`harness-release-readiness.md:915–927`):
  *"the wrapper does NOT re-compute [G6]"* and *"the wrapper does not gate on G6;
  G6 gates the HANDOFF, not the tag."* The artifact's `model_gates` carries only
  G1–G5 (`harness-release-readiness.md:892–898`).
- The wrapper itself **explicitly defers** G6. `release-tag.sh:512–516`:
  *"G6-structure (S2 cross-check) is intentionally DEFERRED to a follow-up phase:
  the backlog prose parsing for `s2-hold:` tokens is heuristic and the readiness
  agent's G6 model-driven judgment already covers this gate at the model layer. A
  wrapper-side deterministic re-computation will land when the parsing contract is
  firmly established."*

Therefore: **a direct invocation of `scripts/release-tag.sh` CAN still bypass a
G6 / S2-hold block.** The wrapper is the sole tag-cutting surface and it currently
has no G6 input. The releaser agent's own ceremony (which does read the readiness
report) would stop on a `handoff_to_releaser: null`, but a direct wrapper call —
or any release path that does not route through the full readiness→releaser
handoff — reaches `git tag -a` blind to G6.

This is the gap. It is acknowledged-by-design in the source comments above, and
the wrapper's own deferral note already names the resolution shape ("a wrapper-side
deterministic re-computation will land when the parsing contract is firmly
established"). This memo ratifies that shape as the decision.

## Decision 2 — Recommendation: A-family (deterministic re-computation in the wrapper), applied to G6

**Verdict:** add a deterministic G6 gate to `scripts/release-tag.sh` that
**RECOMPUTES G6 from committed primary state** — the existing committed `s2-hold:`
backlog record plus the matching committed evidence packet under
`researches/sources/`, joined by the readiness agent's existing G6 join/check
contract — **NOT** by expanding the model-authored readiness artifact to carry a
G6 verdict.

**Why this preserves the sacred invariant ("model output is never transition
authority"):** the wrapper **derives** the G6 result itself from committed
source-of-truth state. No model-emitted conclusion (the readiness agent's `ready`
verdict, its `handoff_to_releaser` field, or a hypothetical new `model_gates.G6`
key) becomes transition authority. The readiness agent remains read-only; its G6
judgment stays advisory at the handoff layer. This is the same split already in
force for G0/G0b/G0c/G7: those gates are re-computed by the wrapper from primary
state precisely so a model surface cannot bypass them.

### Structural precedent: G7 already proves the pattern

G7 (release-time DEFER enforcement) is the proof-by-existence that a
model-judgment gate can gain wrapper-side deterministic enforcement without
becoming model-authority:

- The readiness agent treats G7 as **advisory** (`harness-release-readiness.md:467–477`):
  G7 surfaces DEFERs at the release boundary for the operator, but *"does NOT
  physically prevent a tag."*
- The **authoritative** enforcement lives in the wrapper.
  `release-tag.sh:365–468` is the **"release DEFER gate (authoritative hard
  enforcement)"**: it independently re-invokes
  `.opencode/scripts/check-defer-triggers.mjs --mode=release` — the **single
  source of DEFER classification truth** — and refuses to tag before any
  `git tag` mutation (line 371). The readiness agent confirms the split
  (`harness-release-readiness.md:928`): *"G7 (release DEFER) — enforced by the
  wrapper via the DEFER disposition manifest, INDEPENDENT of this artifact."*

G6 is the analogous gate that does **not** yet have wrapper-side enforcement. The
readiness agent already classifies G6 as *"model-driven in this report but
deterministic in structure"* (`harness-release-readiness.md:923`) — the structural
basis for the deterministic re-compute this decision adopts. G7 is the precedent;
G6 follows it.

### The G6 join/check contract the re-compute reuses

The readiness agent's existing G6 contract (`harness-release-readiness.md:430–447`)
is the join/check logic the wrapper would re-implement deterministically against
committed state:

1. **Enumerate** backlog rows carrying the `s2-hold:` token (each with a
   `PENDING` / `SATISFIED` verdict).
2. **Follow** each row's stable hold ID + evidence-packet reference.
3. **Require EXACTLY ONE** matching evidence record (joined by the same hold ID).
4. **Confirm** the evidence record identifies the held skill AND a real pilot.

**BLOCKER** when any of: a tagged S2 hold is still `PENDING`; the referenced
evidence record is missing or malformed; the evidence does not identify the held
skill + a real pilot; the packet says `SATISFIED` but the backlog row is
unresolved (or vice versa); records are duplicated, contradictory, or ambiguous.
**PASS** when unambiguously `SATISFIED` + backlog agrees + no caveat.

Both inputs are **committed** primary state: the backlog record in
`docs/planning/backlog.md` and the evidence packet under `researches/sources/`.
The wrapper would read them from `HEAD:` (same authority contract as the
readiness artifact and the DEFER manifest) — never the worktree.

## Decision 3 — Enforcement site

A **new top-level G6 deterministic-gate block** in `scripts/release-tag.sh`,
positioned **immediately before the tag mutation** (`git tag -a` at line 902),
alongside the existing G0 / G0b / G0c / G7 deterministic checks.

**Behavior:** refuse before `git tag -a` when committed S2 inputs show a
`pending` / `missing` / `ambiguous` / `duplicate` / `disagreed` hold/evidence
state — i.e. exactly the BLOCKER conditions of the G6 join/check contract above.
Fail-closed, no `ready` default, no env-var bypass (mirrors the readiness agent's
own G6 scope fence, `harness-release-readiness.md:463–465`: *"no env var, no
operator-directive override clears a G6 block"*).

The block must read its inputs from `HEAD:` (committed), not the worktree, so it
is robust to the same worktree-vs-HEAD bypass class G0b already closes.

## Rejected alternatives (named, with reasons)

- **B — releaser-spine only (enforce G6 inside the `releaser` agent's ceremony).**
  **Rejected as the SOLE enforcement at the tag mutation boundary.** The releaser
  agent reads the readiness report and would stop on `handoff_to_releaser: null`,
  but this is bypassable by a **direct** `scripts/release-tag.sh` invocation — the
  sole tag-cutting surface. A gate that lives only in the agent prompt is not
  effective against a wrapper-direct path. opencode also caches subagent prompts
  per-process (the same reason G0c lives in the wrapper, not the releaser prompt —
  `release-tag.sh:577–581`), so a prompt-only G6 step would be stale-cached for
  the ceremony run. B may exist as a *secondary* defense (the ceremony stopping
  early is good UX), but it cannot be the sole enforcement.
- **C — accept-as-human-gate (current posture; do nothing mechanical).**
  **Rejected as knowingly leaving the sole mutation surface blind to a
  release-blocking condition.** Low-cost, but it concedes that a G6-blocked
  release can still be tagged by a direct wrapper call, relying entirely on the
  human to never bypass the readiness→releaser handoff. C is acceptable **only**
  where the blocking condition cannot be mechanically defined from primary state.
  G6 *can* be (it is "deterministic in structure"), so C is not justified for G6.

## Confidence

- **High** on the **premise correction** (G1–G5 ARE consumed; the source is
  unambiguous and was re-derived line-by-line against HEAD).
- **High** on the **A-family choice** (G7 is a shipped precedent for exactly this
  pattern; the readiness agent already classifies G6 as structurally deterministic;
  the wrapper's own deferral note names the deterministic-recompute shape).
- **Medium-high** on **G6 deterministic recomputation** itself. The join/check
  contract exists and is committed-primary-state-based, but the backlog
  `s2-hold:` token parsing is currently described as *heuristic*
  (`release-tag.sh:513`). The implementation must first **precisely pin the S2
  record/evidence parsing contract** — the stable fields, the verdict vocabulary,
  and the evidence-packet locator — before the deterministic re-compute can be
  trusted. This is the open precondition captured in the follow-up card.

## What would change this decision

Evidence that the G6 backlog / evidence inputs are **inherently semantic or not
stably machine-interpretable** — i.e. that no deterministic parser can reliably
extract the `s2-hold:` verdict and join it to exactly one evidence packet — would
weaken the deterministic-re-compute recommendation. In that case the gap reopens
to **B-vs-C for G6**: B (releaser-spine) becomes the strongest *partial*
enforcement and C (human-gate) becomes the honest residual, with the decision
recording that G6 is not mechanically enforceable at the wrapper. This memo's
recommendation is contingent on the S2 parsing contract being pinnable; the
follow-up card carries that as its firing precondition.

## Authority-line note

| Surface | Role | Authority |
|---|---|---|
| harness-release-readiness agent | emits G6 verdict into report + `handoff_to_releaser`; writes the G1–G5 artifact | **INFORMS** (model output is never transition authority) |
| releaser agent ceremony | reads readiness report; stops on null handoff | secondary defense (UX), NOT the sole tag-boundary enforcement |
| **`scripts/release-tag.sh` G6 deterministic gate** (future) | re-derives G6 from committed primary state; refuses before `git tag -a` | **BLOCKS** (the sole tag-mutation enforcement) |

The wrapper is the sole tag-cutting surface; the G6 verdict's authority over a tag
must therefore live there, derived — not in the model-authored artifact.

## Evidence / Provenance

| Claim | Verifying artifact / command | Verified |
|---|---|---|
| Wrapper consumes the committed readiness artifact for G1–G5 | `release-tag.sh:647–833` (gate reads `HEAD:.vh-agent-harness/release-readiness-pass.json`, lines 718, 742); GATES array = G1–G5 only, line 747 | yes |
| Wrapper reads from `HEAD:`, not worktree | `release-tag.sh:718` (`git cat-file -e HEAD:<path>`), `:742` (`git show HEAD:<path>`); G0b dirty-tree refusal `:558–564` | yes |
| N→R→M handshake + artifact-commit single-path + commit_sha bound to HEAD^^ | `release-tag.sh:702–714` (HEAD^/HEAD^^), `:728–734` (single-path child), `:819–824` (commit_sha == HEAD^^) | yes |
| Fail-closed on missing/malformed/stale + unknown gate keys rejected | `release-tag.sh:718, 753–792, 798–800, 819–824, 828–833` | yes |
| `git tag -a` runs AFTER every readiness gate | `release-tag.sh:902` (`git tag -a -F …`), after the G1–G5 gate (`:647–833`) | yes |
| G6 absent from the artifact `model_gates` | `harness-release-readiness.md:892–898` (schema: G1–G5 only); `:915–927` ("the wrapper does NOT re-compute it"; "G6 gates the HANDOFF, not the tag") | yes |
| Wrapper explicitly defers G6 | `release-tag.sh:512–516` ("intentionally DEFERRED … a wrapper-side deterministic re-computation will land when the parsing contract is firmly established") | yes |
| G7 is the structural precedent (deterministic re-compute in wrapper, independent of the artifact) | `release-tag.sh:365–468` (DEFER gate re-invokes `check-defer-triggers.mjs --mode=release`); `harness-release-readiness.md:928` ("G7 … enforced by the wrapper … INDEPENDENT of this artifact") | yes |
| G6 join/check contract (the re-compute basis) | `harness-release-readiness.md:430–447` (enumerate s2-hold rows → hold ID + evidence ref → exactly-one match → skill+pilot; BLOCKER on pending/missing/ambiguous/duplicate/disagreed) | yes |
| Re-derived against current HEAD | `git rev-parse HEAD` → `bb2db205d41965342c8af964d530a0252afbda4c` | yes |

House style: this memo follows the `2026-07-25` / `2026-07-22` / `2026-07-23`
convention (bolded-metadata frontmatter; Framing → Decision → Mechanism →
Authority → Contradictions → Evidence), at decision granularity — not the
per-slice build plan. The implementation is tracked separately.
