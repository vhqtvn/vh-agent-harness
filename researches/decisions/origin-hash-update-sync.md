---
type: decision
date: 2026-08-08
scope: managed-file update seam — three-way origin-hash sync + deletion suppression
status: research-complete, decision-recorded (operator-pre-classified P1 BORROW)
source-basis: refs/hermes-agent @ 005421d888a40865cc61d143ff77efd87a037a1e (gitignored transport), cross-checked
---

# Origin-Hash Update Sync — Decision Memo

## Decision statement

**BORROW** hermes's three-way origin-hash sync + deletion-suppression into the
harness's managed-file update seam. Record an origin hash at render time; on
update, treat a managed file whose recorded origin hash no longer matches its
on-disk content as **ownership-transferred** → skip (never clobber). Respect
user-deleted files. Drop the manifest entry for upstream-removed files but do
**not** delete the on-disk copy. This turns the per-release "adoption
migration" (today a silent clobber of consumer edits) into a non-destructive
three-way merge.

This is a recommendation the operator will decide on; the memo body does not
speak as live repo policy.

## Why this is P1 (decision context)

The harness's `platform_managed` ownership class is a **wholesale-overwrite**
class: a plain re-render overwrites it unconditionally (modulo a byte-identical
no-op). There is no "the consumer edited this since we last rendered it →
skip" branch. The drift detector *observes* that divergence but the update
seam *ignores* it. The result is that every hand-edit a consumer makes to a
`platform_managed` file (the composed `AGENTS.md`, an agent file, a doc) is
silently destroyed on the next `update`, and the consumer must re-apply it by
hand — every release, in every consumer repo. That is the recurring
"adoption migration" tax. Hermes solved exactly this with an origin-hash
manifest and a three-way comparison; the mechanism is verified and the
operator has pre-classified it a BORROW.

## Hermes finding (verified mechanism)

Hermes's `skills_sync.py` seeds bundled skills into `~/.hermes/skills/` and
tracks each one by an **origin hash** recorded at sync time.

- Manifest = `{skill_name: origin_hash}`, v2 format `name:hash`, atomically
  written. `source=refs/hermes-agent/tools/skills_sync.py:113-137` (read) and
  `:166-197` (atomic write). [cross-check CONFIRMED]
- Update decision (`sync_skills`, `source=refs/hermes-agent/tools/skills_sync.py:836-921`):
  - **Bundled unchanged AND still matches origin** (`:845-847`): skip without
    even hashing the user copy.
  - **User copy matches origin, bundled changed** (`user_hash == origin_hash`,
    `bundled_hash != origin_hash`, `:869-870`): safe to update — backup old,
    copy new, record new origin hash.
  - **User copy diverged from origin** (`_is_tracked_user_modification` →
    `user_hash != origin_hash`, `:862-867`): append to `user_modified`,
    **skip, NEVER clobber**. Hermes's own message is "user-modified,
    skipping". [cross-check CONFIRMED — the "ownership transfer" framing in
    this memo is a gloss; the mechanism is exact and the hermes term is
    "user-modified, skipping"]
  - **User-deleted** (in manifest, absent on disk, `:914-916`): respected,
    not re-added.
  - **Upstream-removed** (in manifest, gone from bundled): only
    `del manifest[name]` (`:918-921`); the on-disk copy is **not** deleted.
- Shared classifier `_is_tracked_user_modification(origin_hash, user_hash)`
  (`source=refs/hermes-agent/tools/skills_sync.py:1099-1108`) is the single
  test the sync loop and the `list_user_modified` discovery both use, so the
  two can never drift. [cross-check CONFIRMED]

## Consumer field evidence (operator cross-check, studied 2026-08-08)

`source=operator-cross-check, studied:2026-08-08` — these are operator-provided;
this researcher cannot re-verify consumer repos from here.

- **vh-video-maker** has a hand-edited composed `AGENTS.md`. Rule 6 "Angular
  Dark Field" exists ONLY in the render, NOT in
  `.vh-agent-harness/AGENTS.mission.md`. The composed `AGENTS.md` is
  `platform_managed` and wholesale-overwritten on update, so the next update
  silently drops the rule. Captured as defect card
  `defect-vhvm-rule6-port-and-relocate`.
- **TrueAI** render is stale at harness/0.1.0 (2026-06-26) with real
  managed-file divergence — a consumer that has not migrated precisely
  because migration is a destructive clobber.
- Every consumer pays a recurring per-release "adoption migration" session to
  re-apply local edits after a clobbering update.

## This-harness posture (read by this researcher)

The harness already records a hash per managed file and already classifies
ownership. The gap is that the hash is used for *diagnosis*, not for
*merge arbitration*, and the `platform_managed` update branch has no
"diverged → skip" path.

- **A manifest hash IS recorded.** `source=internal/manifest/manifest.go:161-164`
  — `File{Hash, Class}`; the manifest is `path → {sha256 hash, ownership
  class}`. This hash is the rendered content hash at install/update time —
  functionally an origin hash already.
- **Drift detection classifies divergence but is diagnostic-only.**
  `source=internal/drift/drift.go:36-45` defines four categories
  (`ok`/`drifted`/`missing`/`unexpected`); `:160-189` (`Compute`) hashes each
  manifest path and compares to the recorded hash. `drifted` = on disk with a
  non-matching hash. This is the engine behind `vh-agent-harness diff` — a
  read-only report. It is **not** consulted by the update apply path.
- **The authoritative overwrite decision has no three-way branch.**
  `source=internal/substrate/apply.go:311-384` (`planOutcome`) is "the
  AUTHORITATIVE overwrite decision for a seam apply" (comment `:300-307`).
  For `platform_managed` (`:320-332`): byte-identical-to-staged →
  `ActionManagedNoop`; otherwise → `ActionManagedOverwrite`. A drifted live
  copy is **still overwritten** — confirmed by
  `source=internal/substrate/apply_test.go` ("Drifted -> managed-overwrite
  (still written)"). For `project_owned` (`:334-340`): present → preserved,
  absent → seeded once.
- **The ownership lattice is binary at the platform edge.**
  `source=internal/ownership/doc.go:21-34` — `platform_managed` = "platform
  owns; platform updates overwrite"; `project_owned` = "project owns; platform
  NEVER touches on update". There is no third state for "platform-owned file
  the consumer has diverged from origin". A consumer who wants to keep an
  edit must today reclassify the file `project_owned` (a raise on the lattice)
  — losing all future platform updates to it. That is the wrong granularity.

So the precise gap: the harness has the origin hash (manifest), has the
divergence signal (drift), and has the overwrite gate (planOutcome), but the
overwrite gate does not consume the divergence signal the way hermes's
sync loop does.

## Options considered

- **OPT-A — Borrow hermes three-way into `planOutcome` (RECOMMENDED).** Add a
  divergence check to the `platform_managed` branch: compare the on-disk hash
  to the recorded origin hash. If they differ (ownership transferred), route
  to a new `ActionManagedDiverged` outcome that skips the write and reports
  the file as consumer-modified (mirrors hermes's `user_modified`). Keep the
  byte-identical noop and the otherwise-overwrite behavior. Record origin
  hashes at render time (already present in the manifest). Respect
  user-deleted files; for upstream-removed files, drop only the manifest
  entry.
- **OPT-B — Status quo + reclassification guidance.** Tell consumers to raise
  any file they edit to `project_owned`. Rejected: it is the wrong granularity
  (the consumer loses all future platform updates to that file), it is
  manual and error-prone, and it does not solve the silent-clobber default
  that causes the adoption-migration tax.
- **OPT-C — Full three-way merge with conflict markers.** Attempt to merge
  platform changes into consumer edits. Rejected for now: text merge on
  structured rule files (AGENTS.md) is brittle, and hermes's simpler
  "skip-and-report" is sufficient and proven. Merge can be a later
  enhancement layered on the skip primitive.

## Recommendation

**OPT-A.** Borrow hermes's three-way origin-hash sync into the update seam.
Concretely (framing for a separately-authorized implementation slice, not
live policy):

1. The origin hash already lives in the manifest
   (`internal/manifest/manifest.go`). Wire `planOutcome`'s `platform_managed`
   branch (`internal/substrate/apply.go:320-332`) to consult it: when the
   live on-disk hash != recorded origin hash, emit a skip outcome (do not
   clobber). This is the hermes `_is_tracked_user_modification` test
   (`skills_sync.py:1099-1108`) ported to the harness apply path.
2. Make the divergence observable: the skip outcome should surface in the
   update report (mirrors hermes's `user_modified` list +
   `list_user_modified_bundled_skills`) so the operator can see which files
   were preserved and choose to re-baseline (`hermes skills reset` equivalent)
   or port their edits into the mission/overlay layer where they survive.
3. Respect user-deleted `platform_managed` files (do not re-seed on update —
   mirrors hermes `:914-916`).
4. For upstream-removed files, drop only the manifest entry, never the
   on-disk copy (mirrors hermes `:918-921`).
5. The origin-hash layer sits **above** ownership classification (it is a
   refinement of the `platform_managed` branch only); the
   `project_owned`/`external_generated`/`local_only` branches are unchanged.

The interaction with the `make update` dev-stale-embed guard is benign: that
guard refuses a live `update` when the binary's embedded corpus differs from
the checkout's `templates/core/`. Origin-hash sync fires inside `planOutcome`
regardless of how staging was produced.

## Findings

- **(finding)**: source=internal/substrate/apply.go:320-332, confidence=high, type=fact — the `platform_managed` update branch overwrites a drifted live copy; there is no "user diverged from origin → skip" path.
- **(finding)**: source=internal/manifest/manifest.go:161-164, confidence=high, type=fact — a per-file hash (origin) is already recorded in the manifest.
- **(finding)**: source=internal/drift/drift.go:160-189, confidence=high, type=fact — divergence is detected (`drifted` category) but only as a diagnostic for `vh-agent-harness diff`; it is not consumed by the apply path.
- **(finding)**: source=refs/hermes-agent/tools/skills_sync.py:862-867,1099-1108, confidence=high, type=fact — hermes's three-way origin-hash comparison ("user-modified, skipping") is the proven mechanism that closes this gap. [cross-check CONFIRMED]
- **(inference)**: source=synthesis, confidence=medium, type=inference — wiring the manifest hash into `planOutcome`'s `platform_managed` branch as a divergence skip is the minimal change that turns the adoption migration from a silent clobber into a non-destructive merge.

## Contradictions

- Hermes's framing in the cross-check was scrutinized for an "ownership
  transfer" gloss. The verified mechanism is "user-modified, skipping"
  (`skills_sync.py:866`); this memo uses "ownership transferred" only as a
  label for the skip outcome, not as a claim about hermes's terminology.
  No contradiction with the verified mechanism.
- No contradiction between this-harness posture and the hermes finding: the
  harness records the origin hash and detects divergence but does not act on
  it; hermes records the origin hash, detects divergence, and acts (skip).
  The borrow closes exactly that wiring gap.

## Risks / open-questions

- **Layering vs ownership classification.** Does the origin-hash divergence
  check sit inside the `platform_managed` branch (refinement) or as a new
  pseudo-class above the lattice? Recommendation: inside the
  `platform_managed` branch only — it is a per-file update-time decision, not
  a static class. `overlay_extension` (active pack) likely wants the same
  treatment but is a separate question (overlay units are staged only when
  active, so divergence there means "consumer edited an active overlay unit").
- **Re-baseline ceremony.** Hermes ships `hermes skills reset <name>` to clear
  the manifest entry and re-accept upstream changes. The harness needs an
  equivalent ("I intentionally want the platform version back") or the skip
  becomes a one-way ratchet. Open: verb shape and scope.
- **Manifest storage / format.** The hash already lives in
  `.opencode/harness-manifest.json` (committed transport in consumer repos).
  No new storage needed; the origin hash is the existing manifest hash.
  Confirm the manifest is refreshed atomically with the apply (it is —
  `manifest.File.SetFile` + `Write`), so origin hashes stay correct across
  partial failure.
- **First-render bootstrap.** A file with no prior manifest entry has no
  origin hash. Hermes treats v1/empty-hash as "set baseline from current
  copy" (`skills_sync.py:851-860`). The harness's first install seeds+records
  the hash, so this is naturally handled (origin = first-rendered content).
- **Consumer awareness of preserved files.** A silent skip can hide a
  consumer from the fact that a platform update did not reach their edited
  file. The report-surface (item 2 above) must be prominent, not just a
  debug log.
- **Token-stability.** Origin-hash comparison must be on the *rendered*
  content (post token-substitution), which is exactly what the manifest
  records. No token-drift risk.

## Recommended durable artifact path

`researches/decisions/origin-hash-update-sync.md` (intended target; staged
under `tmp/decisions-staging/` in this session because the read-only
execution policy denied the direct write — see session handoff).

## Promotion targets (if the operator accepts)

When this becomes active guidance, the live targets a follow-up slice would
touch — **not** this memo's job to edit: `internal/substrate/apply.go`
(`planOutcome` platform_managed branch), `internal/drift/drift.go` (the
origin-hash accessor the apply path consumes), and the update report surface
in `internal/cli/`. A new re-baseline verb (or an extension of an existing
one) for the `hermes skills reset` equivalent. Tests:
`internal/substrate/apply_test.go` should gain a "diverged platform_managed →
preserved, not clobbered" case mirroring hermes's
`list_user_modified_bundled_skills` behavior.

## Implementation status & policy addenda (post-decision, updated 2026-08-10)

The BORROW landed at commit `16c69f8` (origin-hash feature, unreleased at time
of this addendum). The origin-hash store lives at
`.vh-agent-harness/origin-hashes.json` (JSON, schema_version=1, path→sha256).
This section records the policies that crystallized during implementation and
the closure work that followed. It speaks as **implemented policy**, not
recommendation — the mechanism shipped.

### Sidecar convention — committed tracked state (F7)

`origin-hashes.json` is **COMMITTED binary-owned platform state**, not
gitignored. Rationale:

- **Deterministic platform provenance.** The store is the protection baseline;
  losing it on clone silently degrades every consumer edit to "bootstrap" (no
  recorded origin → update overwrites unconditionally). A committed baseline
  survives clone and restores full three-way protection immediately.
- **Comparable state is already committed.** `lineage.yml` and
  `rendered-outputs.json` (also under `.vh-agent-harness/`) are committed
  binary-owned platform state. The sidecar is the same kind of artifact.
- **No `.gitignore` entry excludes it.** The `.gitignore` lines under
  `.vh-agent-harness/` target runtime-state (memory, coordinator, ssh, logs),
  none of which match `origin-hashes.json`.

**Uninstall still removes the sidecar** (`internal/cli/uninstall.go`:
`os.Remove(originhash.FilePath(target))`). This is install-lifecycle — distinct
from the tracked-vs-gitignored question. Uninstall removes managed files AND
the sidecar together; a later clean install re-seeds both. The clone-retention
property is about the steady-state `git clone` lifecycle, not the uninstall
lifecycle.

### Regenerated policy — opencode.jsonc whole-file (F3)

`opencode.jsonc` joins the **regenerated platform paths** set
(`regeneratedPlatformPaths` in `internal/cli/seam.go`), alongside
`allowed-commands.js`. Both are canonically rewritten by the Go permission
emitter (`permconfig.EmitWithExtra`) on every apply. A consumer edit to a
regenerated path is **always overwritten** (never preserved) so the live
permission surface stays byte-in-sync with shell-guard and the generated
permission blocks.

Rationale: opencode.jsonc is the live OpenCode permission config. Preserving a
hand-edited live copy while advancing generated companion policy
(`allowed-commands.js`) leaves live OpenCode permissions **inconsistent** with
shell-guard. Supported customization stays in profile/overlay inputs +
`config-transform.mjs`, not the rendered live file. This is NOT a structural
JSONC merge — that would be a larger separate design.

**Migration precedence (F3↔F6 coordination hook):** opencode.jsonc is DECLARED
in `regeneratedPlatformPaths` but EFFECTIVELY regenerated only once a valid
origin record exists. The `effectiveRegeneratedPaths` helper
(`internal/cli/seam.go`) drops opencode.jsonc from the effective set when the
origin store is nil (pre-migration / first-origin-aware-run). This preserves
the F6 first-run stall's authority over a colliding pre-feature baseline: the
regenerated admission does NOT short-circuit migration protection. Once F6
(Slice 3) establishes a valid origin record post-migration, opencode.jsonc
becomes effectively regenerated and is whole-file overwritten on every apply.

Both Apply (`seamApply`) and doctor (`checkManagedDrift`) consult the SAME
effective set, so they cannot disagree on preserved-vs-genuine for
opencode.jsonc.

### Migration-classification-only model (F6, landed Slice 3)

F6 is the first-run migration gate. On the first origin-aware run with an
unknown pre-feature baseline, a colliding existing managed file must be
preserved/stalled + require explicit `accept-platform` rather than silently
overwritten. The origin-hash feature's three-way check provides the
classification (consumer-edit vs. genuine-drift) F6 keys on, but the full
migration algorithm (stall UX, `accept-platform` verb) is Slice 4 (F2) work.

F3's regenerated policy was designed to NOT short-circuit this: the
`effectiveRegeneratedPaths` hook keys on the per-path origin entry's existence,
which F6's migration establishes. F3 can land first; F6 can land later without
re-opening F3's classification.

**Implemented F6 model (Slice 3):** the shared `managedfile.ClassifyPreserved`
decision tree now returns a new `UnknownBaseline` preserved reason when a
platform_managed path has NO recorded origin (`hadOrigin == false`) AND an
EXISTING live regular file. This is the adoption-migration gate: record
absence MUST NEVER authorize overwriting existing bytes. The classification is:

- `!hadOrigin` + live ABSENT → `""` (safe bootstrap — no existing bytes to lose)
- `!hadOrigin` + live REGULAR FILE → `UnknownBaseline` (preserve/stall)
- `!hadOrigin` + live directory/stat-weirdness → `""` (caller reports)
- `hadOrigin` + live diverged → existing `ConsumerEdit` / `ConsumerDelete` / `Unreadable`

A stalled path (UnknownBaseline) is NEVER recorded in the origin store — live
bytes are NEVER snapshotted as prior platform bytes. The path stays stalled
until a future `accept-platform` recovery operation (Slice 4 / F2) explicitly
adopts the platform bytes. `effectiveRegeneratedPaths` keys on PER-PATH origin
entry existence (not whole-store existence) so a partial-migration state (store
exists but opencode.jsonc was stalled on the first run) stays migration-
protected. Doctor surfaces UnknownBaseline as a non-failing migration-stalled
INFO signal (agreeing with Apply's preserve/stall), not drift FAIL.
