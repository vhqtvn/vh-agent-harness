# Retirement carrier — 2026-11-15 resolve-now sweep (six cards)

This document is a **retirement carrier** under the retention-preservation
contract (v3 decision-record, see `docs/coordination/RECORD_LIFECYCLE.md` and
`docs/coordination/CLOSEOUT_TEMPLATE.md`). It durably preserves the decision-class content (owner notes, open questions,
measured outcomes) of the six task cards retired in the 2026-11-15
restudy/resolve-now sweep closeout — twenty-four items verbatim, eight short
provenance/timestamp notes in curated label form (each original line retained
directly above its `decision-item:` label) — so that deleting those cards and
their report directories destroys no last-copy decision knowledge. The six
outcome summaries are the closeout measured_outcome texts, declared verbatim
at `/task-closeout`.

Landing commits carrying the `Task-Card:` trailers for the six cards:
`5db2741` (carrier-mismatch), `c2b2f78` (eslink fixtures), `80edc5c`
(twin-parity guard), `097b133` (pause blocklist), `e15e94b` (Makefile re-spec),
`58abbb0` (copieranswers deletion).

Each section below is anchored by its task id; each listed line is a verbatim
decision item from that card.

## task-2026-11-13t19-58-34-review-flow-delete-gate-carrier-mismatch-auto-populated-decision-records-reference-commits-that-lack-decision-item-labels-handles-forcing-retirement-refusals

- source:review-defer equivalent — actually retirement-sweep finding 2026-11-14; operator-directed filing (m0221, option 1 aftermath).
- trigger: FIRED — 7 reproductions in this sweep alone; every reviewed completion under the current flow hits it unless its commit coincidentally carries labels.
- studied:2026-11-14
- decision-item: task-2026-11-13t19-58-34-review-flow-delete-gate-carrier-mismatch-auto-populated-decision-records-reference-commits-that-lack-decision-item-labels-handles-forcing-retirement-refusals#owner_notes[fnv1a:cab98b806161d975] (curated)
- Fix B landed (gate-side carrier-doc fallback, naming-convention constrained): committed carrier documents under durable homes (docs/checkpoints/, researches/decisions/, researches/sources/, docs/planning/) with a retirement-carrier basename now satisfy the retirement gate's carrier verification when pinned+search commit carriers deterministically miss (anchor_missing/handle_not_in_carrier); fail-closed preserved; artifact carriers excluded. RED-first reproduction of the Class A handle_not_in_carrier shape, then green through the fixed path. Landed in commit 5db2741 (Task-Card trailer present). Verification: node --test tests/scripts/task-retire-carrier-doc-fallback.test.js (674-line suite, reproduction retire + naming/home boundaries + negatives + search-mode rescues + artifact-scope pin + honesty note) 1 pass; full tests/scripts/ 235 pass; go test ./... green; make update idempotent; make doctor HEALTHY.

## task-2026-11-12t18-40-43-eslink-strictexport-fitmodule-fit-enforcement-coupling-missing-number-regex-flags-ls-ps-adjacency-fixtures

- source:review-defer
- decision-item: task-2026-11-12t18-40-43-eslink-strictexport-fitmodule-fit-enforcement-coupling-missing-number-regex-flags-ls-ps-adjacency-fixtures#owner_notes[fnv1a:e8a2f357cf18ba64] (curated)
- trigger:path_touched(internal/eslink/lexer.go) — FIRED 2026-11-13 by ff2f577 (Slice 0)
- studied:2026-11-13
- decision-item: task-2026-11-12t18-40-43-eslink-strictexport-fitmodule-fit-enforcement-coupling-missing-number-regex-flags-ls-ps-adjacency-fixtures#owner_notes[fnv1a:d1b996855a61ce70] (curated)
- Both adjacency fixture classes landed in commit c2b2f78 (Task-Card trailer present): number-adjacency and regex-flags-adjacency U+2028/U+2029 fixtures pin the ff2f577 isLineTerm-refusal boundary with no lexer change (pre-fix behaviors documented: false-broken number literal vs silent regex absorption); strictExport-fitModule coupling adjudicated sound and load-bearing, pinned by TestStrictExportFitModuleCoupling with the verdict comment-recorded; corpus pins unchanged (61/46/15 - 49/0/5 - 14/3/32); go test ./internal/eslink/ green; cascade review APPROVE.

## task-2026-11-13t13-03-35-twin-parity-guard-test-for-the-reconciliation-runbook-twins-docs-ai-templates-docs-body-from-first-h2-byte-identity

- source:review-defer (docs-key build ses closeout 2026-11-11, round-1 advisory; consumer-reachability study's twin-relationship design).
- trigger:path_touched(docs/ai/preserved-managed-reconciliation.md) OR path_touched(templates/docs/preserved-managed-reconciliation.md) — not yet fired; filed as dormant guard.
- studied:2026-11-13
- decision-item: task-2026-11-13t13-03-35-twin-parity-guard-test-for-the-reconciliation-runbook-twins-docs-ai-templates-docs-body-from-first-h2-byte-identity#owner_notes[fnv1a:d1b996855a61ce70] (curated)
- Twin-parity guard landed in commit 80edc5c (Task-Card trailer present, branch-reachable on main): TestRunbookTwinParity asserts docs/ai and templates/docs reconciliation-runbook twins stay byte-identical from the first H2 onward. Failure message names both paths + the conscious-uncoupling escape; permanent in-memory red arm; disk red-check receipt. Verification: target test PASS, full go test ./... green (33 ok), cascade commit-review APPROVE.

## defer-017-pause-new-work-coverage-reaudit

- source:review-defer
- decision-item: defer-017-pause-new-work-coverage-reaudit#owner_notes[fnv1a:e8a2f357cf18ba64] (curated)
- trigger:path_touched(templates/core/.opencode/commands/) AND new-dispatch-command-added
- studied:2026-08-10
- decision-item: defer-017-pause-new-work-coverage-reaudit#owner_notes[fnv1a:e0b1b4cabb11e1fb] (curated)
- ORIGIN: memo-4 (ESTOP) implementation slice closeout. The shipped slice documents unknown coverage (plugin header lists deliberately-not-blocked commands; README.agent.md names the covered seams). This card captures the recurring re-audit obligation as the command surface grows.
- This is transport, not truth (per AGENTS.md DEFER curation rule). Reaches backlog.md only if a new dispatch command is added AND the promotion DoR is met.
- resolve-first triage: defer-with-trigger — the current blocklist is complete for today's command surface; the trigger is a new dispatch command landing in templates/core/.opencode/commands/.
- drain:2026-08-10 park-future-watch
- Blocklist re-audit landed in commit 097b133 (Task-Card trailer present): deep-research triaged BLOCK (deep variant of blocked /research; inert where the native-deep-research-pilot pack is absent) and added to PAUSE_BLOCKED_DISPATCH_COMMANDS; new fiction guard asserts blocklist membership against the rendered command surface (core + selected packs); /write-task exclusion preserved; README covered-seams updated. Verification: node --test pause-new-work 17/17, full tests/scripts 236/236, doctor HEALTHY; cascade review APPROVE.
- The current blocklist is intentionally narrow (the 'begin new delegated work' class). New commands added in future slices need a triage: is the new command new-work dispatch (block) or state/utility/diagnosis/review/planning (do not block)? The classification criteria live in the plugin header comment.
- Should a structural test assert that the blocklist only references commands that actually exist (fiction guard), mirroring the command_surface_test README consistency check?

## makefile-verbs-dispatch-gap

- source:p2-followup (wave1 slice-2 commit-reviewer F1 DEFER, 2026-09-24)
- trigger:path_touched(internal/cli/root.go)
- trigger:path_touched(internal/runshape/runshape.go)
- secondary trigger (not expressible in checker vocabulary): a release shipping bare-verb prescriptions as functional — operator-driven re-derivation
- studied:2026-09-24
- decision-item: makefile-verbs-dispatch-gap#owner_notes[fnv1a:ef616c30d1bd1579] (curated)
- corrected:2026-09-29 — prior card version cited internal/cli/runshape.go (nonexistent path); actual file is internal/runshape/runshape.go (2026-09-29 pile sweep finding); files_in_scope + trigger normalized.
- Makefile verb gap resolved by RE-SPEC in commit e15e94b (Task-Card trailer present): test/lint/fmt/smoke targets re-specced as fail-loud placeholders behind vh-agent-harness exec bash -c with swap-in examples (wiring rejected as inventory-ledger surface expansion, tradeoff recorded in-file); grep receipt confirms every recipe invocation is a dispatched verb; go test ./internal/cli/ command-surface tests green; cascade review APPROVE.

## defer-copieranswers-memorystore-deletion-audit

- source:review-2026-08-15 (candidates only)
- studied:2026-08-16
- decision-item: defer-copieranswers-memorystore-deletion-audit#owner_notes[fnv1a:dab1ab58c111eb6d] (curated)
- Deletion audit closed in commit 58abbb0 (Task-Card trailer present): internal/copieranswers deleted after zero-consumer grep receipts (sole repo-wide Go reference was the package itself); internal/memory audited ALIVE (12+ importers) and untouched; go build ./..., go vet ./..., go test ./... all green (33 ok); cascade review APPROVE.

## defer-carrier-fallback-unavailable-threading

- source:review-defer (cascade commit-review 2026-11-15, d-F2 converged with b-F1/c-F1)
- trigger:path_touched(templates/core/.opencode/scripts/state-lib.js)
- studied:2026-11-15
- decision-item: defer-carrier-fallback-unavailable-threading#owner_notes[fnv1a:cbb98d136061d7e2] (curated)
- Unavailability threading landed: carrierDocFallbackVerify now distinguishes per-candidate search unavailability from deterministic handle absence (fail-closed refusal unchanged); regression arms added; verified via node --test tests/scripts/ + go test ./... green at HEAD 3b92525 + this working tree.
