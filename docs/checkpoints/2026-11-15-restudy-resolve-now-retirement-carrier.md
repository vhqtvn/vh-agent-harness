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

