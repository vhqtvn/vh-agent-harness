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

## defer-carrier-fallback-hardening-batch

- source:review-defer (cascade commit-review 2026-11-15 of 2fc595f, round-2 + committer-round advisories)
- studied:2026-11-15
- decision-item: defer-carrier-fallback-hardening-batch#owner_notes[fnv1a:cbb98d136061d7e2] (curated)
- re-derivation: operator-driven opportunistic batch — land alongside the next carrier-verification or test-file touch after v0.30.0 cuts (no mechanical trigger; path predicates on these files self-fire while they sit in the post-v0.29.0 diff window)
- F1 fixture hardening: blob-corruption arm assumes loose-object storage — add !fs.existsSync(objPath) assert or set gc.auto=0 on the scratch repo
- F3 comment accuracy: state-lib ~L6913 pinned-carrier comment groups discovery-level and per-candidate unavailability; split into the two actual cases (discovery-level appends nothing; per-candidate appends the qualified note)
- F5 coverage: search-mode call site lacks an end-to-end unreadable-candidate arm — add the search-mode analog (refusal + unexamined path + qualified absence language + recovery); claim guard before any both-modes behavioral-completeness claim
- cap-truncation honesty: DECISION_CARRIER_DOC_PATH_CAP=32 / DECISION_RECORD_SEARCH_CAP=200 truncation can yield an unqualified deterministic miss — same diagnostic-certainty class as the unavailability threading
- carrier-doc intro totals: the 2026-11-15 retirement-carrier intro still describes the original six-card sweep counts; scope the totals or update when the doc is next touched
- studied:2026-11-16
- decision-item: defer-carrier-fallback-hardening-batch#owner_notes[fnv1a:ccb98ea65f61d64f] (curated)
- resolved-by-operator-ask:2026-11-16 (batch 1-3 go)
- Carrier-fallback hardening batch implemented: gc-invariant blob-corruption fixtures (gc.auto=0 plus post-rm existence assert), discovery-level vs per-candidate unavailability comments split at both commit-carrier call sites, qualified absence notes whenever DECISION_CARRIER_DOC_PATH_CAP=32 or DECISION_RECORD_SEARCH_CAP=200 actually truncates, and a search-mode unreadable-candidate end-to-end arm plus a mixed unreadable+readable continue-walking pin; fail-closed refusals unchanged throughout and the carrier-doc intro totals verified accurate post-addenda. RED receipts: both cap-truncation arms fail against the pre-fix twin; the search-mode and mixed arms are green-from-birth characterization pins. Verified: node --test tests/scripts/task-retire-carrier-doc-fallback.test.js 6 pass, full tests/scripts/ 0 fail, go test ./... green, gofmt and go vet clean, make doctor HEALTHY.

## defer-taskcard-repair-op-deadlock

- source:ops-finding (2026-11-15 errata-pair repair attempt, session restudy-resolve-now-fanin)
- studied:2026-11-15
- decision-item: defer-taskcard-repair-op-deadlock#owner_notes[fnv1a:cbb98d136061d7e2] (curated)
- re-derivation: operator-driven — fires when the plan-state task-repair path is next touched or the next degraded card needs repair (no mechanical trigger)
- defect: repairing a degraded card (errata-v0290-*) deadlocks — update_coordination_task rejects task_type/coordination_mode/primary_lane under draft-guards, repair_coordination_task rejects success_criteria/validation_plan as unsupported payload fields, but repair validation demands all of them at once; no single sanctioned op can complete the repair
- workaround used 2026-11-15: sanctioned-equivalent hand-edit of the gitignored transport JSONs via build (all fields at once, LCS line-diff invariant, jq-verified); errata cards now valid ready
- fix direction: either repair accepts the DoR fields, or update lifts the enum-field guard for degraded cards, or a dedicated degraded-recovery fill path exists
- second instance 2026-11-15: ready→staged unreachable via plan-state (update rejects status field on ready; review whitelist is reported|blocked|completed only) — errata transport cards that must return to staged holding require a hand-edit; candidate fix: allow review/closeout transitions into staged for errata-class cards, or a dedicated restage op.
- studied:2026-11-16
- decision-item: defer-taskcard-repair-op-deadlock#owner_notes[fnv1a:ccb98ea65f61d64f] (curated)
- resolved-by-operator-ask:2026-11-16 (batch 1-3 go)
Repair-op deadlock resolved via a dedicated restage transition with full lifecycle guards: repairCoordinationTask accepts the degraded-fill DoR fields plus pre-execution status restorations only under transition validation (ready-target requires the F3 design-readiness envelope, staged-target requires staged_path, and verdict/execution statuses are never repair-restorable), the new restageCoordinationTask op (plan-state restage_coordination_task) sanctions ready→staged for errata-class cards only with repo-relative staged_path validation, staged joined the JS status enum and the canonical task-card schema twins, and the review op's accepted result statuses are an explicit frozen allowlist (ready/reported/blocked/completed/cancelled — the documented /task-review resolutions; staged refused, never derived from the widened enum); both original deadlock instances plus all five review-blocker classes are pinned RED→green. Verified: node --test tests/scripts/task-repair-deadlock-restage.test.js green with new refusal arms, full tests/scripts/ 0 fail (248 pass), go test ./... green with eslink goldens re-pinned, gofmt and go vet clean, make doctor HEALTHY.
- studied:2026-11-16
- decision-item: defer-taskcard-repair-op-deadlock#owner_notes[fnv1a:ccb98ea65f61d64f] (curated)
- resolved-by-operator-ask:2026-11-16 (round-2 guard-completeness go)
Round-2 guard-completeness fixes landed on the same repair/restage transition validation: every explicitly-supplied repair target status outside {draft, ready, staged} is now refused outright independent of offender classification (an invalid-status card — coerced to draft, status an offender — can no longer be repaired to completed/cancelled/reported/working/blocked), staged→ready runs the F3 design-readiness gate like every non-ready→ready crossing (envelope-free only for genuine ready→ready refresh), a staged-target repair enforces the restage op's errata-class card-id requirement plus ready|staged source allowlist (draft cannot jump to staged), `blocked` joined the never-repair-restore verdict/execution source set, and files_in_scope fills are file-scope-normalized before the merged preview so the validated F3 design digest equals the post-save persisted digest for equivalent scope spellings; plus dispatcher-parity (plan-state restage_coordination_task exercised end-to-end through the public tool entry) and domain-free characterization (no repo-local file paths in templates/core comment blocks) arms. All five round-2 guard classes pinned RED→green against the pre-fix rendered twin (RED receipts: invalid-status offender landed completed; staged→ready landed with a stale persisted envelope and no-envelope; draft errata landed staged; ready non-errata landed staged; blocked landed ready). Verified: node --test tests/scripts/task-repair-deadlock-restage.test.js 4/4 green, full tests/scripts/ 0 fail (250 pass), go test ./... green with the eslink corpus digest re-pinned (eighth move, counters and 74-export hub surface identical), gofmt and go vet clean, make update twins + origin-hashes refreshed, make doctor HEALTHY.
- studied:2026-11-16
- decision-item: defer-taskcard-repair-op-deadlock#owner_notes[fnv1a:ccb98ea65f61d64f] (curated)
- resolved-by-operator-ask:2026-11-16 (round-3 laundering-guard go)
Round-3 normalization-laundering guard (b-F1/b-F2) closed on the same repair path: the fault-tolerant read's ownership-metadata rewrite (a stored `working` card whose active_session_alias/claimed_at cannot be recovered — including the single-alias and updated_at fallbacks — is projected to `ready` by normalizeCoordinationTaskRecord) is a READ resilience behavior and can no longer become a WRITE authority. loadCoordinationTask now threads the RAW stored status through the diagnostic boundary (stored_status, canonicalized only by the trim+lower enum fold and only when valid), and EVERY repair lifecycle check — the execution-source refusal, the staged-target source allowlist, and the F3 BUILD-READY crossing detection — authorizes on the stored value, with the normalized projection as the only fallback (stored bytes carrying no valid enum status, where `status` is itself an offending field demanding an explicit payload). This also closes the no-explicit-status vector: a title-only repair on a stored-`working` card would persist the laundered `ready` projection itself (the locked save re-derives it), so the refusal compares the stored source against the target the repair would actually persist. RED receipts against the pre-fix rendered twin: the staged-target laundering repair on a stored-`working` errata card with an empty title LANDED (arm 9a "Missing expected exception"; probe showed stored bytes rewritten working→staged via the projected ready); post-fix all four attempts (staged/draft/ready targets plus title-only) refuse naming the execution status with the stored bytes untouched. Verified: node --test tests/scripts/task-repair-deadlock-restage.test.js 4/4 green (arm 9 laundering matrix + premise pins), full tests/scripts/ 0 fail (250 pass), go test ./... green with the eslink corpus digest re-pinned (ninth move, content-only — counters and 74-export hub surface identical), gofmt and go vet clean, make update twins + origin-hashes refreshed, make doctor HEALTHY.
- studied:2026-11-16
- decision-item: defer-taskcard-repair-op-deadlock#owner_notes[fnv1a:ccb98ea65f61d64f] (curated)
- resolved-by-operator-ask:2026-11-16 (round-4 escalation-disposal go)
Round-4 restage-laundering + cross-platform staged_path guards (B-F1/B-F2 and C-F2, the commit-reviewer's own round-4 shape) closed the last two gaps on the dedicated restage op: (1) restageCoordinationTask now authorizes its ready|staged source allowlist on the STORED status at BOTH guard sites — the pre-lock check reads loaded.stored_status (via coordinationTaskStoredStatus, projection as fallback) instead of the normalized payload status, and the in-lock guard under the mutator re-loads the card and re-derives the stored status, so a NON-degraded stored-`working` card whose claim metadata is unrecoverable (normalizeCoordinationTaskRecord projects it to `ready`, and the projection is NOT a degradation) is refused by name — "is working and cannot be staged for release inject-errata" — with the stored bytes untouched; (2) the shared staged_path validator used by BOTH entry paths (restage AND staged-target repair) now rejects Windows absolute forms — drive-letter /^[A-Za-z]:[\\/]/ and UNC (leading double-backslash) — alongside the existing POSIX '/'-prefix and '..' traversal checks, because release_inject.go honors an ABSOLUTE staged_path at inject time (only non-absolute values are joined to the repo root), making it an out-of-repo read; the JS side is the gate. RED receipts against the pre-fix rendered twin: arm 10 "Missing expected exception: arm 10: restage must refuse a stored-working card whose projection reads ready, naming the stored status" (premise pins passed — projection read `ready`, degraded false — and pre-fix restage LANDED working→staged), arm 11 "Missing expected exception: arm 11 (restage): Windows-absolute staged_path \"C:/outside.md\" must refuse" (pre-fix validator passed it); post-fix arm 10 refuses naming the stored working status with deepEqual bytes-unchanged, and arm 11 refuses C:/outside.md, C:\\outside.md, and \\\\server\\share\\x.md through BOTH entry paths with bytes unchanged. Verified: node --test tests/scripts/task-repair-deadlock-restage.test.js 6/6 green (arms 10-11 + premise pins), full tests/scripts/ 0 fail (252 pass), go test ./... green with the eslink corpus digest re-pinned (tenth move, content-only — counters 61/46/15 · 49/0/5 · 14/3/32 and the 74-export hub surface identical), gofmt and go vet clean, make update twins + origin-hashes refreshed, make doctor HEALTHY (0 problems; the standing cohort-links WARN is pre-existing vendor noise).
- studied:2026-11-16
- decision-item: defer-taskcard-repair-op-deadlock#owner_notes[fnv1a:ccb98ea65f61d64f] (curated)
- resolved-by-operator-ask:2026-11-16 (round-5 narrowing c-prime)
Round-5 operator narrowing (c-prime) reshaped the landed surface: the degraded-FILL DoR-field widening (files_in_scope/success_criteria/validation_plan) is REMOVED from repairDegradedCoordinationTaskCoreFields — a repair payload carrying those fields now refuses with the standard "Unsupported fields for degraded task repair" diagnostic with the stored bytes untouched, and the fill-path digest-canonicalization wiring died with it (normalizeFileScope itself remains wherever computeTaskDesignDigest uses it generally) — dissolving review blocker B-F1 by removal: no execution-contract field replacement exists at all, so the task-update.md freeze (no files_in_scope/success_criteria/validation_plan changes after execution starts) holds trivially. What remains landed: the enum fills (task_type/coordination_mode/primary_lane restore-only plus the transition-validated draft|ready|staged status widening), the unconditional prohibited-target refusal, stored-status authority at the repair site and both restage guard sites, the F3 BUILD-READY gate for every non-ready→ready crossing, the staged-target errata-class + ready|staged source + repo-relative staged_path contract, the shared cross-platform isNonRepoRelativeStagedPath validator, the frozen review result-status allowlist, and the errata-class restageCoordinationTask op; ride-alongs: A-F1 derives intendedTargetStatus from the stored-first current status, C-F3 genericized the release-injector comments (templates/core stays domain-free), and the D-F2 characterization extension also surfaced and fixed two PRE-EXISTING repo-local comment paths in verify-task-registry.js. The DoR-field widening is deliberately deferred as its own card after four review rounds sourced findings there. RED receipts against the pre-narrowing rendered twin: the degraded-completed and validly-claimed-working DoR payloads both LANDED with fields replaced (B-F1's repro); post-narrowing both refuse naming the unsupported fields with deepEqual bytes unchanged (tests/scripts/task-repair-deadlock-restage.test.js round-5 pins; the digest-canonicalization fill arm died with the widening). Verified: node --test tests/scripts/task-repair-deadlock-restage.test.js 7/7 green, full tests/scripts/ 0 fail (253 tests: 241 pass, 12 skipped — the pre-existing go/bash-unavailable environment class), go test ./... green (33 packages) with the eslink corpus digest re-pinned (eleventh move, content-only — counters 61/46/15 · 49/0/5 · 14/3/32 and the 74-export hub surface identical), gofmt and go vet clean, make update twins + origin-hashes refreshed, make doctor HEALTHY (0 problems; the standing cohort-links WARN is pre-existing vendor noise).

## task-2026-11-01t19-32-49-readme.agent.md-cross-reference-skip-explicit-staging-requirement

- studied:2026-11-16
- decision-item: task-2026-11-01t19-32-49-readme.agent.md-cross-reference-skip-explicit-staging-requirement#owner_notes[fnv1a:ccb98ea65f61d64f] (curated)
- resolved-by-operator-ask:2026-11-16 (batch 1-3 go)
- README.agent.md now cross-references the operator escape hatch from the gated-commit capability-condition section: the gate stages nothing implicitly, commits the index as left, refuses an empty index with explicit git-add guidance, and SKIP_COMMIT_GATE=1 remains operator-only, with the full sequence pointed back to the AGENTS.md escape hatch. Verified: go test ./internal/cli/ README-consistency green, tests/scripts/pause-new-work.test.js 17 pass, full battery green.
- Does README.agent.md need a gate-semantics section at all, or only a one-line pointer to the gated-commit skill?

## defer-repair-staged-source-refusal

- source:review-defer (84c1097 cascade leaf A-F1)
- decision-item: defer-repair-staged-source-refusal#owner_notes[fnv1a:48dddf4b265ee50c] (curated)
- debate-verdict:2026-11-16 P3 directional (staged = release-holding; refresh + F3'd ready sanctioned; draft demotion refused)
- studied:2026-11-16
- decision-item: defer-repair-staged-source-refusal#owner_notes[fnv1a:ccb98ea65f61d64f] (curated)
- resolved-by-operator-ask:2026-11-16 (study+debate+go chain)
- decision-item: defer-repair-staged-source-refusal#owner_notes[fnv1a:515bc5795edbcaea] (curated)
Directional guard landed (round-6, staged→draft refusal only — never blanket staged-source refusal): repairDegradedCoordinationTaskCoreFields now refuses a stored-staged degraded card carrying an explicit draft target with the exact directional message (staged is release-holding; refresh staged with staged_path, or restore ready through the F3 design-readiness gate), computed from stored-first currentStatus and the effective persisted targetStatus (targetStatus = repairedStatus !== null ? repairedStatus : loaded.payload.status, so an omitted status cannot bypass), inserted after the execution-source refusal and before the staged-target guards; staged→staged refresh, F3-gated staged→ready, staged-target class/source guards, and staged_path requirement pins stay green untouched, ride-along staged_path or f3_design_readiness does not authorize the demotion (both still refuse as Unsupported fields for a draft target), the omitted-status projected-draft arm was assessed NOT constructible for a staged-source card (a valid stored staged projects as staged; a draft projection only arises from invalid stored status, where the source is not staged — recorded as an honest comment, no fake arm), and every refusal leaves stored bytes deepEqual-unchanged. RED receipt against the pre-fix rendered twin (before make update): the repair RETURNED with no refusal, repaired_fields ["task_type","coordination_mode","primary_lane","status"], RETURNED task.status draft, STORED CARD STATUS AFTER draft (staged card DEMOTED); target test pre-fix failed arm 1 with Missing expected exception (8 tests, pass 7, fail 1). Verified post-fix: node --test tests/scripts/task-repair-deadlock-restage.test.js 8/8 green, full tests/scripts/ 254 pass 0 fail, go test ./... green with the eslink corpus digest re-pinned (twelfth move, counters 61/46/15 · 49/0/5 · 14/3/32 and the 74-export hub identical), gofmt and go vet clean, make update twins + origin-hashes refreshed with token-only twin delta, make doctor HEALTHY.

## defer-carrier-fallback-dir-truncation-note

- source:review-defer (95cd3f7 cascade, B-F1/C-F1 double-corroborated advisory)
- re-derivation: operator-driven — fires when the carrier-doc discovery/walk code is next touched (no mechanical trigger; a bare path predicate would self-fire on unrelated state-lib touches)
- studied:2026-11-16
- decision-item: defer-carrier-fallback-dir-truncation-note#owner_notes[fnv1a:ccb98ea65f61d64f] (curated)
- resolved-by-operator-ask:2026-11-16 (study+go chain)
Directory-truncation fix landed (2026-11-16, Region-B pair of the study+go chain): the directory-diagnosis early return in searchCommittedPathForAnchorAndHandle no longer discards walkTruncated — the truncationSuffix const is hoisted above the directory branch, the directory detail carries it, and the return keeps truncated: walkTruncated, so carrierDocFallbackVerify routes a directory-shaped candidate under a capped (DECISION_RECORD_SEARCH_CAP=200) walk into unexaminedCandidates with zero caller changes (qualified unavailable note naming the candidate, never the universal-absence miss note; fail-closed refusal and card survival unchanged). RED receipt against the pre-fix rendered twin (before make update): the new dirtr arm failed by producing the unqualified miss note — "The retirement-carrier document fallback also found nothing: no committed, branch-reachable file matching *retirement-carrier* under docs/checkpoints/, researches/decisions/, researches/sources/, docs/planning/ carries the card-id anchor and this curated item label together in one blob" — over a walk capped at 200 of 201 revisions. Verified: node --test tests/scripts/task-retire-carrier-doc-fallback.test.js 8/8 green (6 pass 1 fail pre-fix), full tests/scripts/ 256 pass 0 fail, go test ./... green with the eslink corpus digest re-pinned (thirteenth move, content-only — counters 61/46/15 · 49/0/5 · 14/3/32 and the 74-export hub identical), gofmt and go vet clean, make update twins + origin-hashes refreshed with token-only twin delta, make doctor HEALTHY.

## defer-carrier-fallback-artifact-path-fixture

- source:review-defer (cascade commit-review 2026-11-15, a-F3)
- re-derivation: operator-driven — fires when the artifact-carrier branch of verifyDecisionRecordForRetirement is next modified (no mechanical trigger; scoped condition)
- studied:2026-11-15
- decision-item: defer-carrier-fallback-artifact-path-fixture#owner_notes[fnv1a:cbb98d136061d7e2] (curated)
- resolved-by-operator-ask:2026-11-16 (study+go chain)
- premise PARTIALLY STALE: indirect coverage pre-existed (task-retire-decision-record.test.js artifact search-mode arms, including the b3 directory arm, already exercise the helper through the full retire flow); the landed form is the named green pin in the fallback suite making the helper linkage NAMED and LOCAL (locator/how passthrough + helper-unique path_never_committed wording), not new coverage of an uncovered branch.
Named green pin landed (2026-11-16, green-from-birth characterization): the carrier-doc fallback suite now pins that search-mode ARTIFACT carriers verify and diagnose through searchCommittedPathForAnchorAndHandle directly, driven through the exported verifyDecisionRecordForRetirement test seam (no hub export widening — state-lib stays at 74 named exports): the verified locator is the helper's own "<sha>:<path>" shape with how wording "present in the committed blob at reachable revision", and a never-committed artifact path refuses with the helper's deterministic path_never_committed code and helper-unique wording ("no reachable commit carries" — the durable file was never committed). Verified: node --test tests/scripts/task-retire-carrier-doc-fallback.test.js 8/8 green, full tests/scripts/ 256 pass 0 fail, go test ./... green (eslink counters and 74-export hub surface identical), gofmt and go vet clean, make doctor HEALTHY.

## task-2026-09-15t01-20-28-decide-disposition-of-task-contract-self-heal-to-empty-v0-silence-in-ensuresessionmemorynamespace

- studied:2026-11-16
- decision-item: task-2026-09-15t01-20-28-decide-disposition-of-task-contract-self-heal-to-empty-v0-silence-in-ensuresessionmemorynamespace#owner_notes[fnv1a:ccb98ea65f61d64f] (curated)
- resolved-by-record:2026-11-16 (accepted operator disposition researches/decisions/2026-10-18-compaction-ownership-disposition.md §1.2/§6.1 supersedes the card's open question)
- Is self-heal-to-v0 intended resilience or an honesty defect? Should the disposition ride the H-adoption decision (H's source-bound failure channel already names this case and converts it to a counted visible failure) or land standalone as a WARN if H is not adopted?
Self-heal disposition card closed as resolved-by-accepted-record (2026-11-16 restudy): the card's open decision — whether ensureSessionMemoryNamespace's silent self-heal to an empty v0 task contract should warn or fail — was decided by the operator-accepted researches/decisions/2026-10-18-compaction-ownership-disposition.md (§1.2: plugin-layer productionization SHELVED, Strategy-B silent-loss tradeoff accepted until the native engine; §6.1: the native-engine session store will distinguish missing state from initial empty state, refusing to auto-seed default contracts), so no code change ships from this card and the standing direction is the accepted memo; premise verification at HEAD 0b351d0 confirmed the self-heal behavior is unchanged (state-lib.js:2515-2539) and the memo governs. Verified: card retired through the full gate with this carrier section as the decision-record carrier; no test surface touched.


## task-2026-11-02t07-50-18-release-tag-wrapper-support-tagging-when-head-has-advanced-past-m---at-commit-or-tag-before-chore-ordering-guidance

- 2026-11-02 solution-brief pass 2 (ses_e64bc9a36ffeZ8zSt1dJQKYVFe, resumed): SELECTED C+D — derive eligible M from current committed ceremony anchors (uninterrupted-current-ceremony policy), verify via wrapper-owned isolation; --at only as assertion == derived M. Four open questions ANSWERED (see constraints for the policies). Debate correction: full doctor at HEAD + target-code tests is INSUFFICIENT — must cover every target-applicable doctor check against M with per-check input routing. Safety argument (design-level, not proven): identity (reviewer-positioned R/N determine M), eligibility (anchors + uninterrupted protected paths reject superseded ceremonies and edit/revert resurrection — validated against real history: at H=7f94070a original M identifiable; at H=M2 anchors select M2 and --at a9eb029f must refuse), verification (green H cannot substitute for broken M), mutation (wrapper never moves main checkout to manufacture success). Contradiction audit (6, to fix in-slice): README operator-recovery guidance conflicts; wrapper comments overstate captured-SHA verification; doctor vs G7 release-base derivation differs; stale 'all 21 checks' prose; note-immutability tests don't prevent ceremony resurrection; incident readiness attribution != readiness authorship. VERDICT: ready for /task-ready — implementation selected, file scope + test seams + acceptance specified; no further generic research recommended; doctor coverage implementation + runtime evidence are build obligations. Brief (transient): tmp/agent-runs/release-tag-post-m-pass2-2026-11-02/brief.md. No tests/builds/releases performed by the brief.
- studied:2026-11-16
- decision-item: task-2026-11-02t07-50-18-release-tag-wrapper-support-tagging-when-head-has-advanced-past-m---at-commit-or-tag-before-chore-ordering-guidance#owner_notes[fnv1a:ccb98ea65f61d64f] (curated)
- resolved-by-operator-ask:2026-11-16 (restudy item 2+3 go)
Release-tag historical-M support landed (eligible-concurrent tagging is agent-only): the wrapper derives the eligible M from current committed anchors under the uninterrupted-current-ceremony policy (R agreement via evaluator re-anchor, M as R's direct single-parent child on H's first-parent chain, byte-identical manifest/readiness/migration-note blobs, per-intervening-commit protected-path scan that counts edits AND reverts, suffix-merge refusal, superseded-M refusal naming the newer ceremony), --at <full-SHA> is assertion-only (mismatch refuses), verification runs in a wrapper-owned detached worktree at M (G0 inside X, doctor release-context routing target/control/both with coverage_complete fail-closed on unclassified checks, target-class FAIL at M refuses despite green-at-H, control FAILs stay advisory, WARN semantics preserved), the original checkout is never moved and must be clean at pre-isolation and at the mutation boundary, cleanup happens before tagging (failure refuses, stale-run sweep is registered-and-under-tmp-gated), the evaluator gained the release-only --commit re-anchor, and 'worktree' joined GitMutationVerbs (raw worktree/tag authority denied to agents, sanctioned wrapper invocation allowed); 19 new real-git fixture tests plus 5 doctor JSON tests plus permission regression arms all green — including the tagless superseded-M fixture (immutability cannot mask eligibility) and the real-history derivations (at H=7f94070a the original M a9eb029f derives eligible; at H=M2 anchors select M2 and --at a9eb029f refuses) — RED receipt: the pre-fix wrapper from 0c33b80 exits 2 on the eligible scenario the new wrapper accepts (exit 0, tag peels to exactly M); full go test ./... 33 packages green, node tests/scripts/ 256/256 pass, gofmt+vet clean, eslink corpus digest re-pinned twice for the corpus moves in this slice plus once more for the round-1 commit-review G0c exit-contract fix — the G0c release-context target run now captures the doctor envelope regardless of exit code (doctor's real contract is nonzero-on-problems; the deterministic adopted-marker repo case FAILs control-class defer-liveness row 4 on every target run) and refuses only on spawn failure/unparseable output, config-refs reclassified target->control (its agent-model {file:} refs are live .local state structurally absent at a checkout of M), fake-doctor fixtures now emit real nonzero-exit semantics and two new real-binary arms pin the tolerated control-only-FAIL accept and the target-class-FAIL refuse end-to-end (final pin 9e9e925cc35b26f58d4aaa9e30ad0d104b2d375b978bf24e52df864051fa2fad, counters and 74-export hub identical); runtime evidence obligation status: the deterministic real-git fixture demonstrates the operator-commit-after-M flow completing agent-only end-to-end (wrapper accepts, tags M, no detached-HEAD step, no re-ceremony, original HEAD/branch/index unchanged), while a production release exercising it remains to be observed at the next eligible occurrence — designed and fixture-demonstrated, not yet production-observed. Round-2 activation fix (commit-review round-2 blocker + batched advisories): G0c applicability is now PER-SIDE (lineage at H gates the control run; the lineage BLOB at M, via git cat-file -e, gates the target run) with a named ceremony-binary refusal whenever either side needs doctor; .vh-agent-harness/lineage.yml joined the protected-path scan so an M..H edit, deletion, or addition of it refuses eligibility (RED receipt: the lineage-deletion fixture previously ACCEPTED with both doctor runs skipped and the tag cut; now a protected-path hit), the node class-router refuses malformed envelopes (requires schema_version=1, boolean coverage_complete, non-empty checks; RED receipt: a {} envelope and an empty-checks envelope both vacuously accepted pre-fix), the tolerated-arm premise now pins the doctor nonzero exit explicitly (errors.As on a nil error never fired — the old exit-0 pin was dead code), the real-binary target-fail arm was re-targeted from lineage.yml to a poisoned .opencode/skills SKILL.md (its lineage repair is now itself the protected-path refusal), and the duplicate READINESS_REL assignment was removed; eslink pin UNMOVED (release-tag.sh sits outside the ES corpus; internal/eslink green against pin 9e9e925cc35b26f58d4aaa9e30ad0d104b2d375b978bf24e52df864051fa2fad). Round-3/4 clause — round-4 C-F1 (defer-liveness anchor mismatch) fixed wrapper-only: the historical-M G0c phase gained a defer-liveness anchor supplement closing the restored-path escape (an OPEN undisposed path_touched card whose path is touched in the PRIOR_TAG..M arc and then restored to prior-tag bytes by an eligible post-M chore was invisible to both doctor runs — H's control diff is one-ended against the H worktree, and the M-side target run routes defer-liveness control-class away): the release-prep evaluator runs from the detached worktree X (repoRoot()=X turns the one-ended diff into exactly PRIOR_TAG..M, the prior tag resolved to a full 40-hex commit SHA) with --tasks pointed at H's live card registry (cards live at H; X's evaluator and manifest bytes equal H's committed ones by the protected-path scan, so M-manifest dispositions still apply), refusing BEFORE tagging with the firing card named (fail-closed also on missing evaluator, spawn failure, and empty/unparseable/wrong-shaped envelopes; skipped only when PRIOR_TAG is empty, mirroring doctor's greenfield fail-safe; classic flow byte-unchanged — the block is historical-M-only); RED receipt against the true pre-fix baseline (the reconstructed slice-minus-supplement wrapper — git show e865615 predates the entire uncommitted historical-M mode and is NOT the round-4 baseline; premise refusal reported): the escape fixture wrapper ACCEPTED and cut the tag with the card firing and undisposed; post-fix two new real-binary real-git arms pin the named refusal (defer-hist-anchor-escape on docs/scratch-note.md, tag absent, clean repo, one worktree entry) and the disposed-card still-accepts negative control; eslink pin UNMOVED again (release-tag.sh sits outside the ES corpus — pin 9e9e925cc35b26f58d4aaa9e30ad0d104b2d375b978bf24e52df864051fa2fad stands); full go test ./... green, node tests/scripts/ 256/256 pass, gofmt+vet clean, bash -n clean, make doctor HEALTHY; one pre-existing cohort-preflight flake surfaced during the battery and captured as transport (defer-eslink-cohort-preflight-false-broken-flake, reproduced with no round-4 code in play). Round-5 (2026-11-17, rejection #4 blockers C-F1/C-F2): per-entry checks[] validation added to the G0c target-run malformed-envelope guard — every entry must be a non-null object with a non-empty string name, a tier from the closed vocabulary (PASS/WARN/FAIL/SKIP/INFO), and class in target|control|both, refusing through the same fail-closed malformed path and placed AFTER the coverage probe so the real emitter class:""-on-unclassified envelope (coverage_complete:false) keeps its sharper coverage-INCOMPLETE diagnostic; four new refusal arms in the malformed-envelope test (checks:[null], checks:[{}], unknown tier, unknown class) joining the three top-level shapes; RED receipt: the pre-fix wrapper (slice-minus-this-fix baseline, sha256 39e0b3447a15df7e06b60e480fa6cf960af48e8c9c34018114bb1622db1e28ef, bash -n clean) accepted all four per-entry shapes vacuously (exit 0, ok=true, tag cut at M); battery green — full go test ./... 33 packages, node tests/scripts/ 256/256 pass, gofmt+vet clean, bash -n clean, make doctor HEALTHY; eslink pin UNMOVED again (release-tag.sh and the Go test sit outside the ES corpus — pin 9e9e925cc35b26f58d4aaa9e30ad0d104b2d375b978bf24e52df864051fa2fad stands, go test ./internal/eslink/ green). Round-6 resumption (parked 2026-11-16, resumed 2026-11-17; the slice lands in this round's commit): review findings fixed — F6: the concurrent roster-example session's dirty README.agent.md hunks stripped (the configurable-file table row advertising .opencode/repo-configs/review-tiers.json and the Roster-recipes block citing internal/cli/example_review_tiers_recipe_test.go) with this slice's own hunks byte-intact (dirty-diff review-tiers mentions 0, Roster-recipes grep 0, doctor --json grep 1; the two remaining review-tiers mentions are the committed e865615 Roster-override-seam baseline, outside F6's enumerated scope); F7: the historical eligibility protected-path scan now diffs with --no-renames — a rename OUT of a protected path can no longer hide its source behind git's default rename detection (premise pinned inside the arm; the scan is also config-independent now), refusal message and README eligibility bullet name renames, classic flow untouched (the artifact-commit single-path diff is shared with classic and deliberately unchanged), regression arm TestReleaseTag_Historical_ProtectedPathRenameOut_Refuses (seam-render fixture renames .vh-agent-harness/lineage.yml to docs/lineage.yml between M and H) — RED receipt: the pre-fix round-5 wrapper ACCEPTED the rename chore and cut the tag at M; F8: a pre-tag HEAD re-compare (same refusal shape as the final drift check) runs immediately before the tag mutation, closing the cleanup/G0b-recheck interval, driven by the env-gated RELEASE_TAG_TEST_PRETAG_HOOK interleave seam and arm TestReleaseTag_Historical_HeadMovedInPreTagInterval_Refuses — RED receipts: the round-5 wrapper cut the tag with the window unobservable, and the seam-only wrapper (round-5 behavior plus the observation seam, no recheck) accepted with the tag cut at M while HEAD had advanced past H; F11: the lineage record (.vh-agent-harness/lineage.yml) joined the README eligible-historical-M protected-path enumeration; F4: the stale __pycache__ directory under .opencode/sys-scripts/ and its origin-hashes.json row deleted (interpreter cache) — make update re-check found neither re-added; F1: the dead G0_WHERE assignments removed; battery green — node --test tests/scripts/ 256/256, go test ./... 33 packages, gofmt/vet clean, bash -n clean, make doctor HEALTHY (0 problems), eslink pin UNMOVED (9e9e925c…; no templates/core .js/.mjs byte changed this round, go test ./internal/eslink/ green). Round-7 close-out (solution-brief adjudication, landed at commit 5c9f549 after a 4/4-leaf approve): the round-6 RELEASE_TAG_TEST_PRETAG_HOOK eval seam was deleted from the shipped wrapper entirely (no replacement seam of any kind — no env var, marker, pause, or command dispatch; grep receipt: zero hook identifiers in production, zero eval/source/bash -c forms), regression instrumentation moved to a source-derived test copy with byte-equality, reverse-splice, and five exact-block semantic-ordering pins (cleanup < final-G0b < anchor < HEAD re-compare < tag mutation), and a new anti-laundering arm (OldHookChannelRemoved) runs the PRISTINE wrapper with a hostile payload under the old variable name — positive-control-verified, asserting normal historical success, exact permitted ref delta, and unchanged HEAD/branch/index (security claim bounded: the identified channel is removed, not universal env-var safety; RED receipt: the pre-round seam wrapper provably executed the payload and refused on HEAD drift). Known deferred limitation carried honestly: the re-compare→tag window is bounded by the M-pinned target but not atomic (v1 graduation), and a production ceremony exercising the mode remains to be observed.
- Eligibility rule: what distinguishes 'M followed by a legitimate authorized chore' from a 'superseded ceremony' (the v0.29.0 N/R/M vs N2/R2/M2 case)? Is a rule already latent in the repo, or must it be authored?
- Gate-input routing: which gate inputs belong to target M, which to current control state, and which to the execution checkout — including override handling and release-base discovery?
- Fidelity: how can target-code checks run faithfully against historical M while doctor retains required live transport and trusted binary inputs?
- Authority: can wrapper-owned isolation and cleanup stay within sanctioned authority without broadening raw git permissions for agents?

## defer-task-repair-doc-guard-list-sync

- source:review-defer (9a78dc0 cascade, dropped advisory — docs drift)
- re-derivation: operator-driven — pick up with the next task-repair.md touch or as a tiny standalone docs slice
- studied:2026-11-16
- decision-item: defer-task-repair-doc-guard-list-sync#owner_notes[fnv1a:ccb98ea65f61d64f] (curated)
- resolved-by-operator-ask:2026-11-16 (restudy item 2+3 go)
task-repair.md guard-list sync landed (docs-only): the round-6 staged-to-draft directional refusal is now part of the degraded-card lifecycle-transition guards bullet (a staged card is release-HOLDING so demotion to draft is never a repair) with the sanctioned targets kept explicit in the same bullet (staged-to-staged field refresh and the F3-gated staged-to-ready restore), the rendered twin .opencode/commands/task-repair.md is byte-identical to the templates/core source after make update (diff -q clean), no RED receipt exists because the change is docs-only, the eslink corpus digest did NOT move for this slice (md command files sit outside the 61-module ES corpus — TestGate4_CorpusGoldenPins passes against the unchanged pin 7561f9ee6cc9cffe8a3c0f848e3f9fdb934390af1609d108e01da3a336e23981), and go test ./... 33 packages green, node tests/scripts/ 256/256 pass, gofmt+vet clean.

## opencode-v2-compat-study

- studied:2026-11-18
- decision-item: opencode-v2-compat-study#owner_notes[fnv1a:d6b99e646561dfc1] (curated)
- resolved-by-operator-ask:2026-11-18 (follow-up batch go)
- Reopened 2026-11-18 by operator follow-up batch go: completed study card reopened to carry the durable-promotion landing receipts and retire through the full gate.
Findings durably promoted: researches/sources/2026-11-17-opencode-v2-compat-field-study.md landed in commit 1b96881 (Task-Card trailer present) as a labeled second-generation distillation of this card's closeout/review reports — compatible-with-changes verdict, the 8-surface table (plugins and custom tools hard-break; state-dir degraded/informational), honesty limits, contradictions counted-not-itemized; tmp/ primary probe artifacts cleaned after distillation; the v2-migration implementation task remains an operator decision.

## defer-execsandbox-make-doctor-gocache-friction

- source:session-receipt (researcher leg of solution-brief run ses_ff60578c6ffe, 2026-08-16: 'make doctor was attempted but could not build because the strict sandbox lacked Go-cache access; this finding is therefore source/ledger/reachability-derived, not a fresh doctor receipt')
- studied:2026-08-16
- decision-item: defer-execsandbox-make-doctor-gocache-friction#owner_notes[fnv1a:dab1ab58c111eb6d] (curated)
- studied:2026-11-18
- decision-item: defer-execsandbox-make-doctor-gocache-friction#owner_notes[fnv1a:d6b99e646561dfc1] (curated)
- resolved-by-operator-ask:2026-11-18 (follow-up batch go)
Friction recorded and disposition settled: a dated addendum to researches/decisions/2026-07-29-exec-sandbox-level-b-mode-floor.md (record body untouched) plus a README.agent.md exec-sandbox note landed in commit 0a507fd (Task-Card trailer present) — strict-sandbox builds fail three write-surfaces deep (module-cache verify, build cache, link output; reproduced receipt included), no GOCACHE tweak fixes the class without widening the write surface, settled disposition = accepted limitation + verb routing (exec owns compile-then-run, exec-sandbox stays read-code-only), and the memo's Slice-4 HEALTHY line is noted as an unsandboxed receipt.

## defer-release-tag-mdl-greenfield-skip-pin

- source:review-defer (round-4 A-F3 of the release-tag historical-M slice — the supplement landed with escape-arm pins but no greenfield-SKIP pin)
- re-derivation: operator-driven — add the regression arm the next time scripts/release-tag.sh is touched (no mechanical trigger; a bare path predicate would self-fire on unrelated wrapper edits)
- studied:2026-11-17
- decision-item: defer-release-tag-mdl-greenfield-skip-pin#owner_notes[fnv1a:cdb990395e61d4bc] (curated)
- studied:2026-11-18
- decision-item: defer-release-tag-mdl-greenfield-skip-pin#owner_notes[fnv1a:d6b99e646561dfc1] (curated)
- resolved-by-operator-ask:2026-11-18 (follow-up batch go)
Greenfield skip pin landed: the greenfield twin of the anchor-escape fixture in internal/cli/release_tag_historical_test.go (commit 8715469, Task-Card trailer present) drives a zero-prior-tag historical-M scenario where the defer-liveness anchor supplement intentionally SKIPS and the ceremony completes, with a RED receipt via temporary guard-removal proving the supplement refusal fires when the PRIOR_TAG-empty guard is deleted — first releases can no longer be accidentally refused by the supplement.

## defer-staged-draft-omitted-status-watch

- source:review-defer (9a78dc0 cascade, 3 leaves — arm-3 infeasibility assessment)
- re-derivation: operator-driven claim guard — fires only if normalizeCoordinationTaskRecord's coercion behavior is changed or if a behavioral-completeness claim about the omitted-status projection is made (no mechanical trigger; a path predicate on state-lib.js would self-fire)
- studied:2026-11-16
- decision-item: defer-staged-draft-omitted-status-watch#owner_notes[fnv1a:ccb98ea65f61d64f] (curated)
- studied:2026-11-18
- decision-item: defer-staged-draft-omitted-status-watch#owner_notes[fnv1a:d6b99e646561dfc1] (curated)
- resolved-by-operator-ask:2026-11-18 (follow-up batch go)
Watch pin landed: tests/scripts/task-repair-deadlock-restage.test.js (commit 8715469, Task-Card trailer present) now pins that a valid stored staged card never projects to draft on the load path — healthy and unrelated-degraded fixtures, green-from-birth by design — making the round-6 staged-to-draft guard's arm-3 infeasibility premise executable against normalizer coercion-set drift.

## agents-md-compose-dryrun-blind-spot

- studied:2026-11-18
- decision-item: agents-md-compose-dryrun-blind-spot#owner_notes[fnv1a:d6b99e646561dfc1] (curated)
- resolved-by-operator-ask:2026-11-18 (disposition debate ses_e107a98a2ffe; decline-and-close ratified)
Declined-and-closed by operator ratification 2026-11-18 after disposition debate ses_e107a98a2ffe: inspection confirms that dry-run omits root AGENTS.md composition and live composition can overwrite root-only edits; the card also records a consumer loss report whose underlying circumstances were not independently verified in this review. We accept these limitations because the opted-in root is explicitly generated output, while the proposed refusal guard introduces attribution ambiguity and recovery friction, and the evidence does not establish a mandatory protection or complete-preview requirement. Evidence establishing such a requirement, or materially different loss consequences, would warrant a fresh assessment; this disposition creates no trigger, debt item, or future obligation. Recorded cheap-override for re-entry: refusal-only preflight (no flag) if the dry-run visibility expectation is later judged mandatory.
- S2 design decision (operator-relevant): refuse-by-default with an explicit --accept-rendered-loss flag vs loud WARN + full diff listing. Refusal risks breaking consumers whose AGENTS.md legitimately diverges until they port or pass the flag; WARN-only repeats the incident class if unreviewed. Flag addition changes the CLI surface — README.agent.md must be updated in the same change (mission rule).
- Should the doctor 3b WARN message name the drift direction (cheap rider on S1/S2 test work)?
- Consumer-side (vh-solara): incident binary likely predates 4483896 (report's quoted dry-run tokens don't match current code) — operator should update the consumer's harness binary so doctor 3b is active; it would have flagged both drift directions pre-incident.

## defer-repair-inlock-reauth

- source:review-defer (84c1097 cascade leaf C-F1, major-deferred)
- re-derivation: operator-driven claim guard — escalate to BLOCK only on a real interleaving repro; no mechanical trigger
- studied:2026-11-16
- decision-item: defer-repair-inlock-reauth#owner_notes[fnv1a:ccb98ea65f61d64f] (curated)
- studied:2026-11-18
- decision-item: defer-repair-inlock-reauth#owner_notes[fnv1a:d6b99e646561dfc1] (curated)
- resolved-by-operator-ask:2026-11-18 (disposition debate ses_e107a98a2ffe; decline-and-close ratified)
Declined-and-closed by operator ratification 2026-11-18 after disposition debate ses_e107a98a2ffe: repair and the Go errata injector can lose a logical update because the injector does not participate in the JS task-lock protocol; atomic replacement does not eliminate that race. We accept the residual risk for the supplied narrow concurrent-operator scenario, given idempotent reinjection and diagnostic resurfacing, rather than land a partial JS defense or introduce cross-runtime synchronization. Evidence of materially different consequences or an applicable mandatory concurrency guarantee would warrant fresh derivation from current state; no trigger, debt item, or future obligation remains. Recorded cheap-override for re-entry: Go-side sentinel adoption if completed-state integrity is later judged to deserve a mandatory serialization guarantee.

## defer-012-no-head-progress-status-logic-test

- trigger:any(path_touched(internal/cli/commit_gate_closeout_ledger_test.go),path_touched(templates/core/.opencode/scripts/commit-gate.sh))
- triage:2026-08-03 (premise-verified LIVE; no_head_progress branch still untested (grep: no 'no_head_progress' in ledger test); coverage obligation rides next touch)
- source:review-defer
- decision-item: defer-012-no-head-progress-status-logic-test#owner_notes[fnv1a:e8a2f357cf18ba64] (curated)
- studied:2026-07-26
- decision-item: defer-012-no-head-progress-status-logic-test#owner_notes[fnv1a:d569680480cb8051] (curated)
- ORIGIN: Slice B commit-reviewer (6f524bdb) DEFER t1b-F2. Contract MD3 calls for the no_head_progress status-determination logic to be unit-tested. The Go suite has no test exercising the commit_hash == current_head branch directly; the Slice B crux only supplies the separate could_not_land path. The branch is structurally unreachable via normal acquire->commit (commit-tree always mints a new OID; the no_changes guard blocks no-op trees) — honestly documented — but the conditional itself is also unexercised.
- This is transport, not truth (per AGENTS.md DEFER curation rule). Reaches backlog.md only if the trigger fires AND the promotion DoR is met.
- drain:2026-08-07 (investigate-and-decide, operator draining mandate). Decision: PARK (keep draft). Re-derived source post-bb2db20: closeout_status selection at templates/core/.opencode/scripts/commit-gate.sh:1219-1229 (commit_hash == current_head -> no_head_progress else committed), inside the CAS-success arm of cmd_commit. Honest structural-unreachability note intact (commit-tree always mints a new OID parenting current_head; no_changes guard blocks no-op trees). Seam verdict: NO cheap fault-injection seam. commit_hash comes from `git commit-tree ... -p current_head` and can never equal current_head (different parent/content); the only levers are (a) an env override forcing commit_hash (ADDS production surface, violates the honest-reachability-note non-goal), or (b) extracting the inline 2-line conditional into a testable bash function — but a standalone bash [[ test is tautological (~zero regression value; tests the language, not the production path) and extraction-for-testability changes structure for negligible gain. Load-bearing crux for sub-item 3 (could_not_land recording) IS covered (could_not_land_on_merge_conflict subtest). Reactivation: (1) a future gate fault-injection seam makes the branch cheaply reachable, OR (2) a behavioral-completion claim for the no_head_progress sub-item is attempted.
- studied:2026-11-19
- decision-item: defer-012-no-head-progress-status-logic-test#owner_notes[fnv1a:d7b99ff76461de2e] (curated)
- resolved-by-operator-ask:2026-11-19 (anti-parking pile study m0321; decline-and-close ratified)
Declined-and-closed by operator ratification 2026-11-19 after the anti-parking pile study (m0321): the awaited fault-injection seam will never be built because building it adds production surface, violating the card's own non-goal; the target branch (closeout_status no_head_progress at commit-gate.sh:1406-1415) is structurally unreachable in-test — commit-tree mints a new OID every run and the only environment hook in the gate is COMMIT_GATE_TTL_SECONDS; the load-bearing could_not_land crux is already covered elsewhere (TestCommitGate_CloseoutLedger/could_not_land_on_merge_conflict); and the structural-unreachability honesty note stays durable in code + README. No trigger, debt item, or future obligation remains. Recorded cheap-override for re-entry: if commit-gate ever grows a deterministic no-head-progress test seam, re-derive from current state.
- Can a cheap seam inject a synthetic commit_hash to exercise the commit_hash == current_head branch directly (a gate test harness), or is fault-injection the only path?
- Is the conditional's structural unreachability (commit-tree always mints a new OID; no_changes guard blocks no-op trees) a stable enough invariant that a direct test is unnecessary, or should a status-table unit test guard it against future refactor?

## defer-014-closeout-ledger-flock-absent-coverage

- source:review-defer
- decision-item: defer-014-closeout-ledger-flock-absent-coverage#owner_notes[fnv1a:e8a2f357cf18ba64] (curated)
- trigger:path_touched(templates/core/.opencode/scripts/commit-gate.sh)
- studied:2026-07-26
- decision-item: defer-014-closeout-ledger-flock-absent-coverage#owner_notes[fnv1a:d569680480cb8051] (curated)
- ORIGIN: Slice A commit-reviewer round-2 (a4afe6e8) DEFER d-F1. The non-Linux unlocked fallback is structurally sound (no trim => no append-vs-trim race => correct, unbounded) but is NOT behaviorally exercised. The Go test runs only on Linux where flock is present, so the flock-absent else-branch of _closeout_append and the trim-skip guard in _gate_gc_sweep never execute under CI. Per the behavioral-coverage rule this is a load-bearing code path with a correctness claim verified by reading, not by runtime observation.
- Low priority: the production host is Linux with flock verified present. Re-evaluate coverage if _closeout_append's flock-absent branch is touched, or if the gate is ever run on a non-Linux host.
- This is transport, not truth (per AGENTS.md DEFER curation rule). Reaches backlog.md only if the trigger fires AND the promotion DoR is met.
- drain:2026-08-07 (investigate-and-decide, operator draining mandate). Decision: PARK (keep draft). Re-derived source post-bb2db20: _closeout_append flock-absent else-branch at templates/core/.opencode/scripts/commit-gate.sh:347-354 (printf append, no lock); _gate_gc_sweep trim-skip guard at :502-519 (if -f CLOSEOUT_LOG && command -v flock -> trim, else skip). Grounding re-derived LIVE: command -v flock succeeds (/usr/bin/flock present); run-shape backend=host-shell (Linux production host); no COMMIT_GATE_NO_FLOCK / force-commit-hash hook exists in the script (grep clean). Seam verdict: NO cheap clean seam. The only lever to reach the else-branch is a flock-free PATH (enumerate + symlink every transitively-invoked binary minus flock), which is fragile/high-maintenance and breaks misleadingly ('command not found') on any new external call; a COMMIT_GATE_NO_FLOCK=1 production hook would ADD behavior surface, violating the non-goal. Value negligible: Linux-only production, flock always present; branch correctness holds by reading (trim skipped -> no append-vs-trim race -> ledger correct, unbounded) and load-bearing cruxes (committed/could_not_land/rebased_refused recording + concurrent append-vs-trim serialization under flock PRESENT) ARE covered by the existing 7 subtests. Reactivation: (1) flock-absent branch is touched, OR (2) non-Linux host support becomes a requirement (then a first-class flock shim/escape hook makes the PATH-stub test cheap).
- studied:2026-11-19
- decision-item: defer-014-closeout-ledger-flock-absent-coverage#owner_notes[fnv1a:d7b99ff76461de2e] (curated)
- resolved-by-operator-ask:2026-11-19 (anti-parking pile study m0321; decline-and-close ratified)
Declined-and-closed by operator ratification 2026-11-19 after the anti-parking pile study (m0321): the flock-absent branch requires a non-Linux environment to execute, run-shape.yml is host-shell Linux and nothing in-repo will ever exercise it, the in-code comment at commit-gate.sh ~:529 is the durable marker of the intentionally-correct unlocked-append/trim-skip fallback, and a re-open stays cheaply re-derivable from those markers. No trigger, debt item, or future obligation remains. Recorded cheap-override for re-entry: a non-Linux adopter signal (then a first-class flock shim makes the coverage tractable).
- Is the production host always Linux with flock present (current run-shape backend is host-shell on Linux)? If so, the flock-absent branch is defense-in-depth and coverage is low priority.
- If non-Linux support is ever needed: stub/shadow flock (PATH manipulation or a COMMIT_GATE_NO_FLOCK=1 escape hook) to exercise the unlocked append + trim-skip path and assert no record lost + ledger grows unbounded.

## defer-016-pause-new-work-watchdog-auto-engagement

- source:review-defer
- decision-item: defer-016-pause-new-work-watchdog-auto-engagement#owner_notes[fnv1a:e8a2f357cf18ba64] (curated)
- re-derivation: operator-driven — fires only if an operator explicitly requests watchdog auto-engagement (no mechanical trigger)
- studied:2026-08-10
- decision-item: defer-016-pause-new-work-watchdog-auto-engagement#owner_notes[fnv1a:e0b1b4cabb11e1fb] (curated)
- ORIGIN: memo-4 (ESTOP) implementation slice closeout. Invariant 7 ('No auto-engagement in this slice — watchdog/automation is explicitly deferred — do not build detection logic') names this as the explicit out-of-scope item. The shipped slice is operator-driven only: vh-agent-harness pause-new-work {engage|status|disengage} and the /pause-new-work slash command.
- This is transport, not truth (per AGENTS.md DEFER curation rule). Reaches backlog.md only if an operator requests auto-engagement AND the promotion DoR is met.
- resolve-first triage: defer-with-trigger — the work is real future capability but no operator has requested it; the trigger is an explicit operator request for condition-driven engagement.
- drain:2026-08-10 park-future-watch
- studied:2026-11-19
- decision-item: defer-016-pause-new-work-watchdog-auto-engagement#owner_notes[fnv1a:d7b99ff76461de2e] (curated)
- resolved-by-operator-ask:2026-11-19 (anti-parking pile study m0321; decline-and-close ratified)
Declined-and-closed by operator ratification 2026-11-19 after the anti-parking pile study (m0321): watchdog auto-engagement is an unrequested feature that has sat 3+ months silent, the design record is durably in canon (researches/decisions/estop-global-pause.md:200-205 names it a later, separately-authorized layer), and the operator-driven engage/disengage surface is re-creatable in minutes from that canon if the operator ever asks. No trigger, debt item, or future obligation remains. Recorded cheap-override for re-entry: an explicit operator request for auto-engagement.
- What conditions should auto-engage (budget, failure-streak, operator-defined)? The current slice is operator-driven only (engage/disengage are manual). Invariant 7 of memo-4 explicitly deferred ALL detection logic — this card captures that deferral.
- How does auto-engagement preserve the fail-safe contract and the in-flight-untouched guarantee? The engage operation itself is signal-free (it only writes the sentinel), so the safety boundary is the TRIGGER evaluation, not the engage.

## defer-voided-mid-review-recovery-integration

- restudy-2026-08-09 RECOVERY-DRAIN (autonomous, pre-authorized). Verdict: GENUINE FORK, parked via task-update per operator fork clause. FINDINGS (each re-derived from current source before acting): (1) FLAKE PREMISE REFUTED - _heartbeat_age_seconds at commit-gate.sh lines 170-187 computes age as datetime.now(utc) minus fromisoformat(heartbeat_at string), not wall-clock sleep, so a back-dated fixture is deterministic; proven by reading the function AND by a passing test already using it. (2) TARGET PATH IS CONSUMER-SIDE - voided-mid-review per the design memo means a review whose lane is voided while it holds a queued slot, a queue concept; the memo states the queue is NOT in templates/core; re-confirmed by grep of commit-gate.sh for void, mid-review, review-commit, reviewqueue yielding no mechanism (only the word avoid in a comment at line 14); commit-gate.sh has no void code. (3) DESCRIBED TEST ALREADY EXISTS - commit_gate_halfborn_lock_test.go subtest 4 genuinely_stale_still_breakable (lines 380-434) authors a back-dated heartbeat META fixture dated 2020-01-01, invokes commit-gate.sh acquire, asserts stale lock broken plus fresh acquire succeeds, no sleep; the acquire to _is_stale to _stale_break call chain is confirmed at commit-gate.sh lines 801-802; this IS the integration half the unit tests miss, already passing deterministically. (4) A NEW TEST WOULD BE A MISLABELED DUPLICATE OR FORBIDDEN OVER-CLAIM - the only recovery path commit-gate.sh owns is stale-lock-at-acquire via expired heartbeat (subtest 4); no separate mid-review recovery codepath exists in this repo; labeling a new test voided-mid-review exercised end-to-end would violate non-goal 1 and the memo, the exact claim-outrunning-evidence failure the card exists to discipline. CONCLUSION: branch-2 test-authoring ready criterion not achievable against this repo shipped surfaces; branches 1 and 3 qualifier-guarding have nothing to fix because the canon qualifier is intact. Park is the correct drain. No code change, no commit, no F3 envelope (no BUILD-READY transition on a park). Provenance: all line cites verified 2026-08-09 against templates/core commit-gate.sh (2051 lines) and commit_gate_halfborn_lock_test.go (446 lines), both read in full before acting.
- studied:2026-11-19
- decision-item: defer-voided-mid-review-recovery-integration#owner_notes[fnv1a:d7b99ff76461de2e] (curated)
- resolved-by-operator-ask:2026-11-19 (anti-parking pile study m0321; decline-and-close ratified)
Declined-and-closed by operator ratification 2026-11-19 after the anti-parking pile study (m0321): the card has sat 3.5 months dormant with zero void events ever recorded in-repo, there is no prospect of the consumer-side review-commit ticket queue landing in templates/core, the recovery half that does exist in this repo (stale-lock-at-acquire via expired heartbeat) is already integration-covered (commit_gate_halfborn_lock_test.go:380, genuinely_stale_still_breakable), and the design memo preserves every requirement (researches/decisions/2026-08-04-review-commit-ticket-design.md). No trigger, debt item, or future obligation remains. Recorded cheap-override for re-entry: the review-commit ticket queue lands in templates/core, or voids become observed events.
- The review-commit ticket queue implementation is consumer-side (not in templates/core; see researches/decisions/2026-08-04-review-commit-ticket-design.md). Can this integration test be written against this repo's shipped surfaces alone, or does it require the consumer's queue? If the latter, the durable action is qualifier-guarding in canon, not test-authoring here.
- RESOLVED 2026-08-04: doc_claim is NOT a recognized predicate — parsePredicate (check-defer-triggers.mjs:301-316) handles only path_touched/after_tag and returns null for doc_claim; the checker reported unknown-predicate (met:false permanently = a stealth-park at [hold]). CORRECTION 2026-11-19: the earlier follow-on claim that the trigger was 'changed to any(path_touched(...)), recognized via extractTriggers' was WRONG — no trigger: line exists in owner_notes (the checker reads owner_notes ONLY), so this card carried NO machine-readable trigger and sat at no-trigger-line. No mechanical trigger is added: the card's reactivation conditions are canon/queue events (a durable text drops the unit-covered-only qualifier, or the review-commit ticket queue is promoted into templates/core), not path touches.

## defer-018-overlay-docs-shadow-sentinel-vacuous

- source:review-defer
- decision-item: defer-018-overlay-docs-shadow-sentinel-vacuous#owner_notes[fnv1a:e8a2f357cf18ba64] (curated)
- re-derivation: operator-driven — fires when either internal/cli/overlay_docs_test.go or templates/overlays/auto-classifier-pilot/README.md is next touched (no mechanical trigger; grammar supports no OR)
- studied:2026-10-04
- decision-item: defer-018-overlay-docs-shadow-sentinel-vacuous#owner_notes[fnv1a:8ca4a0c431936791] (curated)
- origin:commit-review DEFER F1 from the gated commit 0a72a575 (permission.ask reserve-hook deletion slice, session auto-gate-permask-del-20261004). The negative shadowing sentinel `embeddedOnly = "three-hook plugin"` in TestOverlayDocs project-local shadowing case (internal/cli/overlay_docs_test.go ~L149-156) quotes embedded README line 3 as it read BEFORE the deletion slice renamed it to a two-hook model. Post-edit the embedded README no longer contains "three-hook plugin", so the negative assertion passes even if the embedded FS wrongly won — vacuous. Positive sentinel (PROJECT-LOCAL-SENTINEL) still guards the primary shadowing property; tests stay green.
- resolve-first-triage:defer-with-trigger (one-string fix but outside the deletion slice's declared edit surface; not silently self-expanded per authority rules).
- Transport, not truth. Backlog only on trigger + DoR.
- studied:2026-11-19
- decision-item: defer-018-overlay-docs-shadow-sentinel-vacuous#owner_notes[fnv1a:d7b99ff76461de2e] (curated)
- resolved-by-operator-ask:2026-11-19 (anti-parking pile study m0321; landed-outcome closeout ratified)
Vacuous sentinel de-vacuated and landed at ba43304 (Task-Card trailer present): the test-side sentinel was the stale side — it pinned "three-hook plugin", quoting a pre-rename README, while the embedded auto-classifier-pilot README has said "two-hook plugin" since 0a72a57/v0.28.0 — so the negative assertion passed vacuously whichever FS won; the sentinel is now aligned to the true phrase and a non-vacuous direction is pinned (the test reads the embedded pack via overlay.OpenPack and asserts the sentinel substring exists in the embedded README bytes, so a future phrase rename fails loudly); no RED is constructible because the vacuity passes either way, making the existence pin the regression value. Verified: go test ./internal/cli/ -run TestOverlayDocs 6/6.
- None material — the fix is a one-string swap; only scheduling is open (edit surface belonged to the permission.ask deletion slice, which is already committed).

## defer-eslink-cohort-preflight-false-broken-flake

- source:ops-finding (round-4 C-F1 battery run, 2026-11-17; full receipt in the round-4 slice report)
- trigger:any(path_touched(internal/eslink/graph.go),path_touched(internal/cli/cohort.go))
- fires when either suspect file is next touched; the flake itself was observed at HEAD e865615 + the historical-M working tree (trigger normalized 2026-11-19: the old pseudo-OR line parsed as an unknown predicate, leaving the card unsupported)
- studied:2026-11-17
- decision-item: defer-eslink-cohort-preflight-false-broken-flake#owner_notes[fnv1a:cdb990395e61d4bc] (curated)
- studied:2026-11-19
- decision-item: defer-eslink-cohort-preflight-false-broken-flake#owner_notes[fnv1a:d7b99ff76461de2e] (curated)
- resolved-by-operator-ask:2026-11-19 (anti-parking pile study m0321; landed-outcome closeout ratified)
Cohort-preflight flake root-caused and fixture fix landed at 6c29b31 (Task-Card trailer present, HEAD): reproduced 3/40; root cause is findLiveAuthoredPlatformManagedPath ranging over the ownership-defaults map unsorted — each iteration consumer-stubbed a random platform-managed path, and a hub pick parses to a closed empty surface whose importers are then TRULY proven broken, so the fail-closed cohort-link preflight correctly refused the live update (the card's "false proven-broken" interpretation is REFUTED — the gate was right; all three card suspects cleared: cohort-tree assembly, resolve memoization, parser export determinism); the fix is fixture-side (sorted iteration, authored-path pick preferring non-cohort-graph paths via production isCohortScannedPath, sorted plural helper) with determinism pin TestFindLiveAuthoredPathPickDeterministic; 100/100 clean at -count=50 x2; no production code changed. The card's original suspect list is superseded by these findings — recorded per the retirement contract.

## defer-release-injector-degraded-eligibility

- source:debate-2026-11-16 (staged-class verdict follow-up, evidence release_gate.go:828-836)
- re-derivation: operator-driven — fires at the next release ceremony prep or when release_gate.go/release_inject.go is next touched (no mechanical trigger)
- studied:2026-11-16
- decision-item: defer-release-injector-degraded-eligibility#owner_notes[fnv1a:ccb98ea65f61d64f] (curated)
- studied:2026-11-19 (implemented at d9e7e72+dirty: injector fail-loud slice, RED receipt captured pre-fix in tmp/red-inject.txt)
- resolved:2026-11-19 (RED→GREEN; four surfaces landed; full battery green; state-lib.js twins untouched; no git mutations — slice left dirty for gated commit, trailer Task-Card: defer-release-injector-degraded-eligibility)
- rescope:2026-11-19 (b′ shape-allowlist after 3-rejection escalation; normalizer port dropped)
- round-5:2026-11-19 (staged_path consumed-representation fix + restage canonical-empty contract)
Round-5 (2026-11-19): staged_path consumed-representation fix — whitespace-padded paths refused at the verifier before the gate/injector can consume trimmed out-of-repo forms (RED receipt shows live read attempts at /outside/x.md and parent-traversal /tmp/…/outside.md) — plus restage canonical-empty producer contract: the real restageCoordinationTask output is pinned byte-faithful and accepted by BOTH release surfaces, with execution-era/canonical-default keys empty-state-only and content still refused; verified RED→GREEN per fix — go test ./... 33/33, node --test 257/257, gofmt/vet clean, make doctor HEALTHY; state-lib twins untouched. Landed at e46939d (Task-Card trailer present, branch-reachable; per-acquire review approve after two leaf BLOCK claims were refuted by substring mechanics — the disputed assertion is a strict substring of the emitted refusal footer).

## task-2026-11-01t19-32-57-opencode-context-refusal-test-obtain-one-non-skipped-run

- studied:2026-11-19
- decision-item: task-2026-11-01t19-32-57-opencode-context-refusal-test-obtain-one-non-skipped-run#owner_notes[fnv1a:d7b99ff76461de2e] (curated)
- resolved-by-operator-ask:2026-11-19 (plain-host receipt settles the fork — decline-unobservable)
Declined-and-closed as unobservable-in-practice by operator ratification 2026-11-19: the card wanted one non-skipped run of TestCommitGate_SkipPathRefusedInsideOpenCode, but /proc/self/environ does not reflect exec-time envp in ANY reachable environment — four skipped runs total: three inside the harness/container runtime (two coordinator probes 2026-11-15/16 + one operator run 2026-11-19, each skipping with the named container-masking reason) and, decisively, one in a PLAIN HOST TERMINAL at the operator's own machine (2026-11-19), which also skipped — so the observability premise fails everywhere reachable, not merely under the harness. The skip guard and its named reason remain durable in code (commit_gate_skipgate_test.go ~:300-303) and honestly disclose the limitation on every run. No trigger, debt item, or future obligation remains. Recorded cheap-override for re-entry: if a future environment (different host, CI runner, or kernel/envp behavior) shows COMMIT_GATE_ENVIRON_PROBE visible in /proc/self/environ, the check is a 30-second one-liner re-run — re-derive from that receipt.
- Is there a host in the normal dev loop where /proc/self/environ reflects envp, or does this need a CI environment?
