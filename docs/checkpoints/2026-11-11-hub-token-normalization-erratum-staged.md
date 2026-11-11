# Hub-token normalization migration notice committed + erratum staged — decision-record carrier

- **Date:** 2026-11-11
- **Task:** task-2026-11-11t18-10-20-stage-migration-notice-erratum-for-state-lib.js-hub-token-normalization-4469f3c-ahead-of-next-release
- **Kind:** decision-record carrier (retirement-verifiable committed state for the
  card's decision items; this file is the anchor + verbatim-representation
  carrier the closeout decision-record points at)

## Outcome snapshot

The last delivery-gate element of
`researches/decisions/2026-11-09-eslink-o4-stage4-normalization-option2.md`
slate item 3 landed: the migration notice
`docs/migration-errata/v0.29.0-hub-token-normalization.md` is committed
(`05a16d7`, one commit, zero tags) and its errata transport card
`.local/coordinator/tasks/errata-v0290-hub-token-normalization.json` is staged
(`status: staged`, `staged_path` → the committed notice). The next release
ceremony's `release inject-errata` step auto-discovers BOTH staged cards
(new + `errata-v0290-preserved-file-skew`) — proven by a fixture rehearsal
with the real make-built binary and byte-identical notice bytes: dry-run
listed both as would-inject into an untagged next-version note; the live run
injected both sections and flipped both cards staged→completed; re-run was an
idempotent no-op; fixture doctor #13 FAIL-named both pre-inject → PASS
post-inject; real-repo doctor #13 SKIPs (no untagged note — designed steady
state). Gates at HEAD: `go test ./...` 33/33 packages ok, `TestMigrationNotes`
ok, `make test-js` 234/234 pass. Full receipts in the closeout report under
`.local/coordinator/reports/task-2026-11-11t18-10-20-*/`.

## Decision items (verbatim carrier lines)

Anchor: task-2026-11-11t18-10-20-stage-migration-notice-erratum-for-state-lib.js-hub-token-normalization-4469f3c-ahead-of-next-release

owner_notes[fnv1a:d6f36b810698e246]:

```
Provenance: stage-4 decision record researches/decisions/2026-11-09-eslink-o4-stage4-normalization-option2.md item 3 (delivery gate: migration notice/erratum MANDATORY, 7-element content enumerated); hub churn landed 4469f3c + 0b56ab6.
```

owner_notes[fnv1a:462ecde4b7a4df51]:

```
Trigger: fired — docs legs of the delivery gate complete (af97e05/89ea161/9b74cdf+134798a); this is the last gate element.
```

owner_notes[fnv1a:8f9a4508b1dc4099]:

```
Dedup: distinct from errata-v0290-preserved-file-skew (that corrects the v0.29.0 note's preservation promises; this notices the upcoming hub churn).
```

measured_outcome[fnv1a:0fa186571c8f013e]:

```
Committed docs/migration-errata/v0.29.0-hub-token-normalization.md at 05a16d7 (one commit, parent 134798a, zero tags, no push, tree clean) carrying all 7 mandated elements verbatim-grep-checkable (22/22 strings). Staged card .local/coordinator/tasks/errata-v0290-hub-token-normalization.json (status=staged, staged_path → the committed notice, mirrors errata-v0290-preserved-file-skew.json exactly; v0290 card untouched — both still staged). Discovery proven without a ceremony: real-repo inject-errata --dry-run reaches the held-erratum/no-note error (exit 1, reachable only with non-empty staged set); fixture rehearsal with the make-built binary (0.29.0+dev) and byte-identical FINAL notice bytes listed BOTH staged cards as would-inject into an untagged v0.30.0 note, live-injected both + flipped both staged→completed, re-run idempotent no-op; fixture doctor #13 FAIL-names both staged cards pre-inject → PASS post-inject; real-repo doctor #13 SKIP (designed steady state, no untagged note). Gates: TestMigrationNotes ok, go test ./... 33/33 packages ok, make test-js 234/234 pass. Delivery gate of researches/decisions/2026-11-09-eslink-o4-stage4-normalization-option2.md item 3 now complete.
```
