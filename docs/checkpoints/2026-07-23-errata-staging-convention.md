# Errata staging convention — location MUST live outside the embedded migration tree

**Date:** 2026-07-23
**Kind:** durable convention (progress snapshot)
**Scope:** `docs/migration-errata/`, `templates/migrations/`, erratum cards

## Convention (the rule)

Errata staging text — the correction queued to ride the NEXT release's
migration note — lives at:

```
docs/migration-errata/vX.Y.Z.md
```

It MUST **NOT** live under `templates/migrations/` (including a
`templates/migrations/errata/` subdir). `templates/migrations/` is the
**embedded canonical-release-note namespace** and must contain only
canonical `vX.Y.Z.md` release notes.

## Why this rule exists (the conflict this resolves)

`templates/migrations/` is embedded into the binary via
`//go:embed templates/migrations/*` (`corpus.go:161`). Go's embed glob
matches subdirectories and embeds their entire subtree, so a file at
`templates/migrations/errata/v0.12.0.md` is embedded and then walked by
`migrationIndex()` (`internal/cli/help_migrate.go:156`, unconditional
`fs.WalkDir`).

`TestMigrationNotes_Canonical` (`internal/cli/help_migrate_test.go:256`)
treats **every** embedded `.md` as a canonical release note and requires:

- filename matching `^v\d+\.\d+\.\d+\.md$`;
- the full canonical heading set (`# Migration: `, `## Summary`,
  `## What changed (consumer-visible only)`, `## How to migrate (automated)`,
  `## What \`update\` handles for you`, `## Watch-outs`,
  `## Verification commands`, `## Rollback`, `## Non-consumer changes`);
- the required migrate command sequence (`self-update`, `version`,
  `update --dry-run`, `update`, `doctor`).

An erratum staging doc is none of those, so placing it under
`templates/migrations/` fails `TestMigrationNotes_Canonical` and breaks
`go test ./...`.

Note the codebase was **internally inconsistent** about this:
`migrationClaimNotes` (`internal/cli/release_gate.go:270`) reads
`templates/migrations/` **top-level only** and skips directories
(`if ent.IsDir() { continue }`), so the release-gate scan deliberately
tolerated a staged `errata/` subdir — but the embed-based canonical-notes
test did not. Relocating errata staging OUT of the embedded tree resolves
both surfaces at once.

## Current state

- `docs/migration-errata/v0.12.0.md` — staged erratum text for v0.12.0
  (media-perception rendering + YAML-syntax false claims). To be consumed
  by the next release cut's migration note, then deleted.
- `.local/coordinator/tasks/errata-v0120-media-perception-rendering.json` —
  `status: "staged"`, `staged_path: "docs/migration-errata/v0.12.0.md"`,
  `staged_at: "2026-07-23"`.
- The `internal/cli/erratum_gate_test.go` gate (status-only) and the
  generalized `TestDeferLivenessGate_*` suite both pass: an open errata card
  still fails the gate; a `staged` card still passes. The path is not pinned
  by any test (the liveness tests use synthetic fixtures).

## Follow-ups (flagged, NOT done in this slice — out of scope / Go source)

1. **Stale comments in `internal/cli/release_gate.go`** — line 73
   (`...erratum written to templates/migrations/errata/)`) and lines 264–265
   (`...the errata/ subdir holds staged erratum text, not released claims`)
   now reference a location that MUST NOT be used. They should be updated to
   `docs/migration-errata/`. Deferred: editing Go source was out of scope for
   this closeout slice ("do not touch Go source").
2. **Releaser-facing docs pointer** — the release overlay / releaser agent
   docs and `README.agent.md` may benefit from a one-line pointer to this
   convention so the releaser consumes `docs/migration-errata/` at cut time.
   Flagged for the docs/releaser lane rather than edited here.
3. **Possible promotion to `docs/ai/`** — this is a durable rule; if a
   maintainer prefers it as standing workflow guidance rather than a dated
   checkpoint, promote the rule body to `docs/ai/` and leave this checkpoint
   as the dated decision record.

## Verification

| Claim | Verifying command/output | Verified |
|-------|--------------------------|----------|
| erratum text lives outside the embedded tree | `ls docs/migration-errata/v0.12.0.md`; `templates/migrations/errata/` removed | yes |
| card is staged at the new path | card JSON: `status:"staged"`, `staged_path:"docs/migration-errata/v0.12.0.md"` | yes |
| erratum gate passes | `go test ./internal/cli -run TestErrataGate -v` → PASS | yes |
| canonical-notes test no longer sees the erratum | `go test ./internal/cli -run TestMigrationNotes_Canonical -v` → PASS | yes |
