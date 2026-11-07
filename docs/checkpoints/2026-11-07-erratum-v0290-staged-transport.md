# Checkpoint — v0.29.0 erratum staged into release-transport (2026-11-07)

Task card:
`task-2026-11-07t04-39-27-release-ceremony-inject-docs-migration-errata-v0.29.0.md-into-next-release-note`
(session `erratum-stage`, primary lane release, closeout 2026-11-07)

Retirement card-id anchor (exact line the carrier gate searches for):

Task-Card: task-2026-11-07t04-39-27-release-ceremony-inject-docs-migration-errata-v0.29.0.md-into-next-release-note

## What happened

The committed v0.29.0 erratum (`docs/migration-errata/v0.29.0.md`, landed at
c968b21) was staged into the release ceremony's OWN discovery transport so the
next ceremony's `release inject-errata` machine step picks it up automatically:

- Staged card: `.local/coordinator/tasks/errata-v0290-preserved-file-skew.json`
  (`status: "staged"`, `staged_path: "docs/migration-errata/v0.29.0.md"` — the
  machinery-canonical shape per `internal/memory/claims` `DeferCard.staged_path`
  and the `writeStagedErrataCard` test helper; gitignored transport).
- The committed erratum file itself serves as the staged text (read-only
  input; untouched). No committed card location is demanded by the machinery.
- Verified by dry-run discovery receipts (real repo before/after contrast +
  fixture rehearsal with listing, live flip, idempotent re-run) and doctor #13
  (in-repo SKIP — no imminent release; fixture FAIL names the staged card).
  Full receipts: closeout report under
  `.local/coordinator/reports/task-2026-11-07t04-39-27-release-ceremony-inject-docs-migration-errata-v0.29.0.md-into-next-release-note/`.

## Decision record carriers (retirement preservation)

The closeout's decision-record declares this checkpoint artifact as the
carrier for both decision items. For the carriers to verify at card
retirement, THIS FILE must be committed (the slice itself is transport-only —
no landing commit carries the task-card trailer, so a `commit`-kind carrier
can never verify; this artifact is the only verifiable carrier surface).

- decision-item: task-2026-11-07t04-39-27-release-ceremony-inject-docs-migration-errata-v0.29.0.md-into-next-release-note#open_questions[fnv1a:45688b74012cf57b] (curated)

  Open question: "Does the standing ceremony auto-discover this card at the N
  step, or must the releaser be handed it explicitly?" — **Answer:
  auto-discovered.** The ceremony prompt's inject step
  (`.opencode/agents/releaser.md`, "Inject staged errata (machine step —
  REQUIRED before the note commit)") runs
  `./bin/vh-agent-harness release inject-errata --note
  templates/migrations/v<next>.md` unconditionally after note authoring, and
  the subcommand itself scans `.local/coordinator/tasks/errata-*.json` for
  `status: staged` cards (`stagedErrataCards` in `internal/cli/release_gate.go`).
  The releaser hands over nothing explicitly.

- decision-item: task-2026-11-07t04-39-27-release-ceremony-inject-docs-migration-errata-v0.29.0.md-into-next-release-note#measured_outcome[fnv1a:b941983cf3b7dec1] (verbatim)

  Verbatim handle (the single line drawn from the item's own content, as
  declared in the closeout decision-record):

  real-repo dry-run before/after contrast (no-op → held-erratum/no-note error, reachable only when the staged set is non-empty)

  Verbatim measured-outcome line it labels (from the card's
  `measured_outcome`):

  > Proven: real-repo dry-run before/after contrast (no-op → held-erratum/no-note error, reachable only when the staged set is non-empty); fixture rehearsal (real binary, byte-identical erratum, untagged v0.30.0 note) listed "would inject section for v0.29.0 into v0.30.0.md; would flip card staged→completed", live flip + idempotent re-run; doctor #13 SKIPs in-repo (no imminent release — by design) and FAIL-names the staged card in the imminent-release fixture.

  Gates green: TestMigrationNotes ok, `go test ./...` all ok,
  `internal/cli -count=1` ok. No commit (transport-only slice by design).

## Ceremony-time follow-up (not this slice)

At the next ceremony's N-step, `release inject-errata` auto-discovers the
staged card, injects the erratum body into the new note as an
`## Errata for v0.29.0` section, and flips the card `staged → completed`.
Per the erratum's own status blockquote, the ceremony then deletes
`docs/migration-errata/v0.29.0.md` and dismisses the erratum card.
