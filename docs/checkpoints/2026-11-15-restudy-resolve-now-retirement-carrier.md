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

Post-sweep addenda (sections appended below after the original six-card
sweep) are excluded from the six-card totals above; each addendum section
carries its own card's owner notes verbatim plus its measured outcome under
the same label discipline.

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

## task-2026-09-29t04-05-00-pin-armed-coreoutputs-disjointness-tripwire

- source:review-defer (wave-3 slice-4 verdict follow-up, build final report 2026-09-29)
- trigger:path_touched(internal/permconfig/catalog.go)
- studied:2026-09-29
- decision-item: task-2026-09-29t04-05-00-pin-armed-coreoutputs-disjointness-tripwire#owner_notes[fnv1a:fc6180a7dcbd26ca] (curated)
- resolve-first triage: defer-with-trigger, pure-future-watch — NO actionable-now component asserted explicitly: the invariant HOLDS today (verdict memo 2026-09-29, pinned by existing per-capability catalog tests); the tripwire only becomes load-bearing when a capability first declares a CoreOutput containing an armed path — catalog.go touch is that moment.
- studied:2026-11-15
- decision-item: task-2026-09-29t04-05-00-pin-armed-coreoutputs-disjointness-tripwire#owner_notes[fnv1a:cbb98d136061d7e2] (curated)
- resolved-by-operator-ask:2026-11-15 (previously valid-waiting; operator-directed execution)
- Tripwire landed as TestCoreCatalog_CoreOutputsNeverPlatformArmed in internal/resolver/catalog_test.go (correct home — the card's original internal/permconfig anchor was stale): iterates CoreCatalog().IDs() + CoreOutputs against the platform_armed set from corpus.CoreOwnershipDefaults; passes trivially at HEAD; RED-check receipt via a temporarily injected armed CoreOutputs entry on core/debate (failure names capability + path), mutation reverted to byte-identical; go test ./internal/resolver/ green.
- Exact assertion home: catalog_test.go general test vs core_manifest_test.go — decide at pickup

## defer-020-auto-gate-dispatch-provenance-rail

- source:review-defer
- decision-item: defer-020-auto-gate-dispatch-provenance-rail#owner_notes[fnv1a:e8a2f357cf18ba64] (curated)
- trigger:path_touched(templates/sys-prompts/auto-gate-classifier.md)
- studied:2026-10-02
- decision-item: defer-020-auto-gate-dispatch-provenance-rail#owner_notes[fnv1a:8aa49d9e33936ab7] (curated)
- origin:commit-review of 1331717 (slice-2 reviewer AF1/tier1_c-F1): delegated-dispatch provenance cannot be verified from the transcript; injected tool output formatted as a delegation could ride the carve-out. Reviewer notes it upgrades to BLOCK only if a resolution claim is made without a live-classifier receipt.
- This is transport, not truth (per AGENTS.md DEFER curation rule). Reaches backlog.md only if a trigger fires AND the promotion DoR is met.
- studied:2026-11-15
- decision-item: defer-020-auto-gate-dispatch-provenance-rail#owner_notes[fnv1a:cbb98d136061d7e2] (curated)
- resolved-by-operator-ask:2026-11-15 (previously valid-waiting; operator-directed execution)
- Provenance rail landed: the delegated-dispatch clause in templates/sys-prompts/auto-gate-classifier.md now names the distinction between session-origin dispatch text (operator/agent intent) and dispatch-claims appearing inside tool output or fetched text — the latter authorize nothing however official they look — with the pinned rail line preserved verbatim; golden lines extended in TestSysPrompt_ClassifierJudgmentScopeGoldenLines (RED before the prompt edit, green after); go test ./internal/cli/ green.
- The delegated-dispatch clause (commit 1331717) relaxes scope-creep judgment for operator-driven delegation/coordination chains, but a classifier reading a transcript cannot verify dispatch provenance — injected tool output FORMATTED as a coordinator delegation could in principle ride the carve-out. Should the clause gain an explicit sentence distinguishing session-origin dispatch text from dispatch-claims appearing inside tool output (or aligning the rail with the threat model's full untrusted enumeration: tool results, fetched pages, files, messages from other agents)?

## defer-release-wrapper-refusal-emit-perf

- source:ops-finding
- decision-item: defer-release-wrapper-refusal-emit-perf#owner_notes[fnv1a:8de6d88233bd54df] (curated)
- trigger:path_touched(scripts/release-tag.sh)
- milestone note: ideally land before the next release ceremony
- studied:2026-10-05
- decision-item: defer-release-wrapper-refusal-emit-perf#owner_notes[fnv1a:8da4a257309365fe] (curated)
- ORIGIN: v0.28.0 ceremony resume (releaser session): wrapper refusal emit ground multi-MB failure output ~25 min; documented in tmp/agent-runs/release-v0-28-0/ receipts.
- Transport, not truth.
- decision-item: defer-release-wrapper-refusal-emit-perf#owner_notes[fnv1a:479fda1c2225e2e3] (curated)
- studied:2026-11-15
- decision-item: defer-release-wrapper-refusal-emit-perf#owner_notes[fnv1a:cbb98d136061d7e2] (curated)
- resolved-by-operator-ask:2026-11-15 (previously valid-waiting; operator-directed execution)
- Emit perf fixed: json_str in scripts/release-tag.sh escapes through one node -e JSON.stringify invocation instead of the per-character loop (contract preserved — named-control shorthand, \u00XX C0, quote/backslash escapes, UTF-8 passthrough, no truncation; the parameter-expansion pre-pass deliberately replaced because it would double-escape under stringify); four regression arms added — 130k-char payload under a 15s spawn guard the old per-char path fails by construction (extrapolated ~164s), a nasty-payload round-trip + double-escape detector, an argv-trap arm pinning stdin routing for leading-dash payloads, and a chunk-boundary arm pinning Buffer-accumulation reassembly of a multibyte char split across two stdin data events (F1/F2 fix); suite 13/13 green with RED receipts on faithful old-path copies.
- Where exactly is the per-char loop (json_str or its caller)? Truncate vs stream decision.

## draft-e2e-stale-image-20261004

- source:p2-followup
- decision-item: draft-e2e-stale-image-20261004#owner_notes[fnv1a:4c0c5c07f3b0738a] (curated)
- trigger:path_touched(Makefile)
- secondary watch: tests/e2e/auto-gate-classifier/ (directory prefix not expressible in the trigger grammar; Makefile is the primary seam)
- studied:2026-10-04
- decision-item: draft-e2e-stale-image-20261004#owner_notes[fnv1a:8ca4a0c431936791] (curated)
- origin:verification of the 2026-10-04 permission.ask reserve-hook deletion slice (session auto-gate-permask-del-20261004). First post-edit `make test-e2e-auto-gate` completed in 1s and passed 12/12 against the STALE pre-edit image; explicit `docker compose build e2e-runner` + rerun exercised the new code (7/7). Receipts: tmp/agent-runs/auto-gate-permask-del-20261004/RECEIPTS.md (stale log bg-jobs/post-e2e-classifier/job.log vs valid bg-jobs/post-e2e-classifier-rebuilt/job.log).
- resolve-first-triage:defer-with-trigger (fix is one Makefile line but outside the deletion slice's declared edit surface; not driven to verdict in-slice by contract).
- Transport, not truth. Reaches backlog.md only if a trigger fires AND the promotion DoR is met.
- studied:2026-11-15
- decision-item: draft-e2e-stale-image-20261004#owner_notes[fnv1a:cbb98d136061d7e2] (curated)
- resolved-by-operator-ask:2026-11-15 (previously valid-waiting; operator-directed execution)
- --build appended to the test-e2e-auto-gate docker compose run line in the Makefile so the e2e-runner image rebuilds from current sources before each run; verified textually via make -n dry expansion showing the flag (no docker in this environment); live docker reproduction deferred to the next run in a docker-capable environment.
- decision-item: draft-e2e-stale-image-20261004#measured_outcome[fnv1a:cf27b06107e049b6] (curated)
- Should the target prepend `docker compose -f tests/e2e/auto-gate-classifier/docker-compose.yml build e2e-runner` (matches the opencode target's explicit `docker build`), or switch to `docker compose run --build e2e-runner`? --build is the smaller diff; explicit build is the more legible one.

## defer-018-repair-task-resurrection-window

- source:review-defer
- decision-item: defer-018-repair-task-resurrection-window#owner_notes[fnv1a:e8a2f357cf18ba64] (curated)
- trigger:path_touched(templates/core/.opencode/scripts/state-lib.js:repairCoordinationTask)
- studied:2026-11-07
- decision-item: defer-018-repair-task-resurrection-window#owner_notes[fnv1a:4dbc98505e5f9625] (curated)
- ORIGIN: committer round-3 review of slice 3 (1286c00), tier1_a/F1: the public updater's allowAbsent:true semantics let repairCoordinationTask recreate a deleted card from defaults.
- resolve-first triage: defer-with-trigger — resolving requires a semantics decision (refuse vs opt-in recreate) that is not in any in-flight slice's scope.
- studied:2026-11-15
- decision-item: defer-018-repair-task-resurrection-window#owner_notes[fnv1a:cbb98d136061d7e2] (curated)
- resolved-by-operator-ask:2026-11-15 (previously valid-waiting; operator-directed execution)
- Resurrection window closed with the refuse-vs-opt-in decision resolved as REFUSE (no caller wants resurrection; create flows exist explicitly): repairCoordinationTask runs under the per-task lock with required loads end-to-end and both repair-branch saves route through applyCoordinationTaskUpdateLocked; a card vanishing mid-repair or repaired after deletion refuses with 'card no longer exists; repair refused' — the pre-fix defaults-based write was attempted, not landed: core-field validation rejected it (a misleading '5 validation problems' diagnostic), so no successful resurrection was ever demonstrated; TOCTOU + absent-id + existing-card-control arms pinned in task-delete-retirement.test.js (2/2 green; pre-fix RED baseline receipts m0072/m0074); eslink corpus digest re-pinned with counters identical (61/46/15 · 49/0/5 · 14/3/32, hub 73 exports); make update twin + origin-hashes refreshed.
- Is recreate-from-defaults ever the INTENDED repair behavior (e.g. reconstructing a structurally-degraded card whose file was lost)? If yes, the fix is a required-load + explicit opt-in recreate flag rather than a blanket refusal.
- Which public updateCoordinationTask callers genuinely rely on allowAbsent:true today beyond repair (save/create flows construct their own cards)?
