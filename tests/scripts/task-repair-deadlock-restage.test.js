// Regression tests for defer-taskcard-repair-op-deadlock (2026-11-16).
//
// The pre-fix op-split deadlock (hit twice on the 2026-11-15 errata pair):
//
//   Instance A — degraded cards could not be repaired by any sanctioned op:
//   `repair` rejected files_in_scope/success_criteria/validation_plan as
//   payload fields ("Unsupported fields for degraded task repair") while
//   `update` rejected task_type/coordination_mode/primary_lane/status under
//   its per-status allowlists — the canonical validator demanded everything
//   at once, so only hand-edits could finish the repair.
//
//   Instance B — ready -> staged was unreachable: `update` never allows a
//   `status` payload field in any allowlist, and the review/closeout status
//   whitelists accept reported|blocked|completed only. Post-fix the
//   dedicated `restageCoordinationTask` op owns that transition (staging is
//   a release mechanic, not a coordinator verdict — the whitelists stay
//   closed), restricted to errata-class cards.
//
// Also pins the review-block repair boundary (2026-11-16 blocked-commit
// round): the deliberate repair-status widening is transition-validated —
// a draft|ready|staged `status` payload is accepted only when the FULL
// transition is legal (ready-target requires the F3 design-readiness
// envelope via the same validator readyCoordinationTask enforces,
// staged-target requires a repo-relative staged_path, and a verdict/
// execution current status is never repair-restorable to a pre-execution
// status), restageCoordinationTask refuses non-errata-class cards, and the
// review op's accepted result statuses are an explicit frozen allowlist
// (reviewing a reported card into `staged` is refused).
//
// Round-2 guard-completeness arms (2026-11-16 second review pass):
//   - offender-status bypass: an explicitly-supplied repair target status
//     outside {draft, ready, staged} is refused INDEPENDENT of offender
//     classification — a card whose STORED status is invalid (coerces to
//     draft, so `status` is an offender) must not be repairable to
//     completed/cancelled/reported/working/blocked or any invalid value.
//   - staged -> ready is a BUILD-READY crossing: the F3 gate fires for
//     every repair entering ready from ANY non-ready current status; the
//     envelope-free exemption survives only for genuine ready -> ready.
//   - staged-target restage guards: a repair targeting staged enforces the
//     dedicated restage op's errata-class card-id requirement and
//     ready|staged source allowlist (draft cannot jump to staged).
//   - blocked source: `blocked` joins the never-repair-restore verdict/
//     execution source set.
//   - dispatcher parity: the PUBLIC plan-state restage_coordination_task
//     operation (tool entry, sandboxed plugin stub) produces the same
//     outcomes as the state-lib-direct arms.
//   - domain-free characterization: the templates/core comment blocks
//     carry no repo-local (dev-repo-only) file paths.
//
// Round-3 laundering-guard arms (2026-11-16 b-F1/b-F2):
//   - normalization-laundered execution status: the fault-tolerant read
//     rewrites a stored `working` card whose ownership/claim metadata
//     cannot be recovered to a PROJECTED `ready`
//     (normalizeCoordinationTaskRecord). Pre-fix the repair guards
//     authorized on that projection, so a stored-`working` errata card
//     with an independently degraded core field (empty title) could be
//     repaired into staged (or draft/ready) — laundering execution state
//     through the diagnostic boundary. The stored status is now threaded
//     through loadCoordinationTask (stored_status) and EVERY repair
//     lifecycle check (execution-source refusal, staging-source
//     allowlist, F3 crossing detection) authorizes on the STORED value;
//     a title-only repair (no explicit status) is equally refused because
//     the save would persist the laundered ready projection itself.
//
// Round-4 escalation-disposal arms (2026-11-16 rejection #3):
//   - restage laundering (B-F1/B-F2): restageCoordinationTask's
//     ready|staged source allowlist authorized on the post-normalization
//     PROJECTION at BOTH guard sites (pre-lock loaded.payload.status,
//     in-lock current.status), so a NON-degraded stored-`working` errata
//     card whose claim metadata cannot be recovered (projects to ready,
//     no diagnostic) restaged into `staged` — laundering the execution
//     status through the read path. Both sites now authorize on the
//     STORED status (loaded.stored_status pre-lock; a locked re-load
//     in-lock), the same authority the round-3 repair guard uses.
//   - Windows staged_path (C-F2): the repo-relative staged_path guard
//     was POSIX-only ('/'-prefix + '..' traversal); the SHARED validator
//     used by BOTH entry paths into staged (dedicated restage +
//     staged-target repair) now also refuses Windows drive-absolute and
//     UNC forms — the release injector honors an absolute staged_path at
//     inject time (out-of-repo read), so the JS write side is the gate.
//
// Round-5 narrowing arms (2026-11-16 operator c-prime, fifth review pass):
//   - the degraded-FILL DoR-field widening is REMOVED from the repair
//     surface: a repair payload carrying files_in_scope/success_criteria/
//     validation_plan refuses with the standard "Unsupported fields for
//     degraded task repair" diagnostic — pinned on a degraded `completed`
//     card AND a validly-claimed `working` card (the B-F1
//     execution-contract-freeze repro; pre-narrowing RED receipt: both
//     payloads LANDED with fields replaced), with the stored bytes
//     untouched. The fill widening is deferred as its own card after
//     four review rounds sourced findings there.
//   - the landed surface is enum fills + guarded transitions + the
//     restage op; a card whose restored status re-attaches DoR demands
//     its stored body cannot satisfy is refused by merged-preview
//     validation (arm 1) — the original op-split deadlock deliberately
//     remains for that shape until the deferred card lands.
//
// Round-6 directional-guard arms (2026-11-16,
// defer-repair-staged-source-refusal — debate verdict P3 directional):
//   - staged -> draft demotion is refused (a staged card is
//     release-HOLDING; the demotion would strand release-ceremony state
//     back in pre-execution holding) while the sanctioned moves out of
//     staged stay open (staged -> staged refresh, F3-gated staged -> ready
//     — pinned by the existing arms above). `staged` deliberately does NOT
//     join the execution-source refusal set.
//   - neither staged_path nor an F3 envelope authorizes the demotion (a
//     draft target admits no supplement; the ride-alongs refuse as
//     unsupported fields), and every refusal leaves the stored bytes
//     deepEqual-unchanged.
//
// Run:  vh-agent-harness exec node --test tests/scripts/task-repair-deadlock-restage.test.js
//       (or: node --test tests/scripts/task-repair-deadlock-restage.test.js)

import { test } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import os from "node:os";
import { fileURLToPath, pathToFileURL } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = path.resolve(__dirname, "..", "..");

// Import state-lib from the RENDERED tree (.opencode/scripts/). The source copy
// under templates/core/ still carries the literal {{COORDINATOR_DIR}} token and
// REFUSES to load (assertRenderedNotSource guard). Run `make update` after
// editing templates/core/ before this test (the normal dev flow).
const STATE_LIB = path.join(
    REPO_ROOT,
    ".opencode",
    "scripts",
    "state-lib.js",
);
const {
    bindSessionName,
    repairCoordinationTask,
    restageCoordinationTask,
    reviewCoordinationTask,
    readCoordinationTask,
    updateCoordinationTaskMetadata,
    computeTaskDesignDigest,
} = await import(pathToFileURL(STATE_LIB).href);

const SESSION_ID = "repair-deadlock-test-session";

// --- isolated coordinator transport helpers ---------------------------------

function cardPath(isoRoot, id) {
    return path.join(isoRoot, "tasks", `${id}.json`);
}
function writeCard(isoRoot, id, fields) {
    fs.mkdirSync(path.dirname(cardPath(isoRoot, id)), { recursive: true });
    fs.writeFileSync(
        cardPath(isoRoot, id),
        JSON.stringify(
            {
                task_id: id,
                id,
                title: `deadlock fixture ${id}`,
                status: "draft",
                active_session_alias: null,
                history: [],
                created_at: "2026-11-16T00:00:00Z",
                updated_at: "2026-11-16T00:00:00Z",
                ...fields,
            },
            null,
            2,
        ),
    );
}
function readRawCard(isoRoot, id) {
    return JSON.parse(fs.readFileSync(cardPath(isoRoot, id), "utf8"));
}

function withIsolatedTransport(fn) {
    // Async-aware: the dispatcher arm's test body is async (dynamic import
    // of the sandboxed tool entry), so the wrapper must AWAIT the body
    // before the finally-block restores the env and removes the transport
    // root — a sync wrapper would tear the sandbox down at the first
    // suspension point while the body is still running.
    return async () => {
        const prevRoot = process.env.OPENCODE_LOCAL_COORDINATOR_ROOT;
        const root = fs.mkdtempSync(path.join(os.tmpdir(), "repair-deadlock-"));
        process.env.OPENCODE_LOCAL_COORDINATOR_ROOT = root;
        bindSessionName(SESSION_ID, "repair-deadlock-test");
        try {
            await fn(root);
        } finally {
            if (prevRoot === undefined) {
                delete process.env.OPENCODE_LOCAL_COORDINATOR_ROOT;
            } else {
                process.env.OPENCODE_LOCAL_COORDINATOR_ROOT = prevRoot;
            }
            fs.rmSync(root, { recursive: true, force: true });
        }
    };
}

const ENUM_FIX = {
    task_type: "implementation",
    coordination_mode: "short",
    primary_lane: "coordination",
};
// Valid DoR list values embedded in FIXTURES (the stored card body) and fed
// to the digest helper as the merged design's DoR inputs. Post-round-5
// these are NEVER repair-payload fields — the degraded-FILL widening is
// removed, and a payload carrying them refuses as unsupported fields.
const FILL_FIX = {
    files_in_scope: ["templates/core/.opencode/scripts/state-lib.js"],
    success_criteria: ["degraded card repairable in one sanctioned op"],
    validation_plan: ["node --test tests/scripts/task-repair-deadlock-restage.test.js"],
};
const FIXTURE_STAGED_PATH = "docs/migration-errata/fixture-a.md";

// Expected merged-post-repair design digest for a fixture written by
// writeCard: derives from the fixture's base design fields plus the repair
// payload's design-bearing inputs (incoming changes), letting the digest
// helper's own file-scope canonicalization normalize './'-prefixed or
// absolute entries exactly as the merged design will carry them.
function expectedMergedDigest(id, incoming) {
    return computeTaskDesignDigest(
        {
            task_id: id,
            title: `deadlock fixture ${id}`,
            task_type: "",
            primary_lane: "",
            files_in_scope: [],
            success_criteria: [],
            constraints: [],
            non_goals: [],
            validation_plan: [],
        },
        incoming,
    );
}

test(
    "deadlock A: degraded enum card repairs under full transition validation; DoR fills refused (round-5 narrowing)",
    withIsolatedTransport((root) => {
        // The empty-DoR shape: enums missing entirely, DoR list fields
        // empty, status "staged" (valid post-convergence — the offenders
        // are the missing enums, and the restored status re-attaches the
        // DoR demands through the canonical validator). The id is
        // ERRATA-CLASS for the staged-target arms below.
        const idA = "errata-dlk-a-enum-fill";
        writeCard(root, idA, {
            status: "staged",
            rough_scope: ["fix the plan-state op split deadlock"],
            files_in_scope: [],
            success_criteria: [],
            validation_plan: [],
        });
        const before = readCoordinationTask(SESSION_ID, idA);
        assert.equal(
            before.diagnostics && before.diagnostics.degraded,
            true,
            "fixture must load degraded (missing enums)",
        );

        // Arm 1 — enums-only repair is refused with the INFORMATIVE
        // merged-validation error naming the DoR demands. Post-round-5
        // this refusal is terminal for the sanctioned surface: the DoR
        // fills can no longer ride the repair payload (the widening is
        // deferred as its own card after four review rounds), so such a
        // card completes through the operator hand-edit path until that
        // card lands — never by widening this payload.
        assert.throws(
            () =>
                repairCoordinationTask(SESSION_ID, idA, {
                    ...ENUM_FIX,
                }),
            (err) =>
                err instanceof Error &&
                /Degraded task repair would leave task errata-dlk-a-enum-fill invalid:/.test(
                    err.message,
                ) &&
                /files_in_scope must contain at least one path/.test(err.message) &&
                /success_criteria must contain at least one requirement/.test(
                    err.message,
                ) &&
                /validation_plan/.test(err.message) &&
                !/Unsupported fields for degraded task repair/.test(err.message),
            "arm 1: enums-only repair must name the missing DoR fields via merged validation, not reject the enums as unsupported",
        );

        // The valid-DoR shape for the staged-target arms: the narrowed
        // repair answers status-conditional demands from the card's
        // STORED values only, so 1b/2 fixtures embed the DoR fields in
        // the card body (never the payload).
        const idA2 = "errata-dlk-a-enum-staged";
        writeCard(root, idA2, {
            status: "staged",
            rough_scope: ["staged-target repair without staged_path"],
            files_in_scope: FILL_FIX.files_in_scope,
            success_criteria: FILL_FIX.success_criteria,
            validation_plan: FILL_FIX.validation_plan,
        });

        // Arm 1b — staged-target transition contract (review-block C/B-F1c):
        // repairing to `staged` without a staged_path is REFUSED (same
        // repo-relative staged_path contract the dedicated restage op
        // enforces). Pre-fix this repair succeeded and landed a staged card
        // with no injectable fragment path.
        assert.throws(
            () =>
                repairCoordinationTask(SESSION_ID, idA2, {
                    ...ENUM_FIX,
                    status: "staged",
                }),
            (err) =>
                err instanceof Error &&
                /Repairing task errata-dlk-a-enum-staged to staged requires a staged_path/.test(
                    err.message,
                ),
            "arm 1b: staged-target repair without staged_path must refuse",
        );

        // Arm 2 — the narrowed sanctioned repair: enums + a
        // pre-execution-class status + the staged_path in one request. The
        // DoR fields are NOT in the payload and NOT written — they pass
        // through from the stored card untouched.
        const repaired = repairCoordinationTask(SESSION_ID, idA2, {
            ...ENUM_FIX,
            status: "staged",
            staged_path: FIXTURE_STAGED_PATH,
        });
        assert.ok(
            Array.isArray(repaired.repaired_fields),
            "repair must return repaired_fields",
        );
        for (const field of [
            "task_type",
            "coordination_mode",
            "primary_lane",
            "status",
            "staged_path",
        ]) {
            assert.ok(
                repaired.repaired_fields.includes(field),
                `repaired_fields must include ${field}`,
            );
        }
        for (const dorField of [
            "files_in_scope",
            "success_criteria",
            "validation_plan",
        ]) {
            assert.ok(
                !repaired.repaired_fields.includes(dorField),
                `repaired_fields must NOT include ${dorField} (round-5 narrowing)`,
            );
        }
        const after = readCoordinationTask(SESSION_ID, idA2);
        assert.equal(
            after.diagnostics && after.diagnostics.degraded,
            false,
            "post-repair card must load healthy",
        );
        assert.equal(after.task.status, "staged", "status must be staged");
        assert.equal(
            after.task.staged_path,
            FIXTURE_STAGED_PATH,
            "repair must persist the validated staged_path",
        );
        assert.equal(after.task.task_type, "implementation");
        assert.deepEqual(
            after.task.files_in_scope,
            FILL_FIX.files_in_scope,
            "repair must leave the stored files_in_scope untouched",
        );
        assert.deepEqual(
            after.task.success_criteria,
            FILL_FIX.success_criteria,
            "repair must leave the stored success_criteria untouched",
        );
        assert.deepEqual(
            after.task.validation_plan,
            FILL_FIX.validation_plan,
            "repair must leave the stored validation_plan untouched",
        );

        // Arm 2b — verdict/execution current statuses are never
        // repair-restorable (review-block B-F1b): a degraded card whose
        // stored status is a VALID `reported` cannot be widened back to a
        // pre-execution status. Pre-fix the widening accepted
        // status:"staged" here unconditionally.
        const idVerdict = "dlk-a-verdict-current";
        writeCard(root, idVerdict, {
            task_type: "",
            coordination_mode: "",
            primary_lane: "",
            status: "reported",
            rough_scope: ["verdict-current restore refusal"],
            files_in_scope: FILL_FIX.files_in_scope,
            success_criteria: FILL_FIX.success_criteria,
            validation_plan: FILL_FIX.validation_plan,
        });
        assert.throws(
            () =>
                repairCoordinationTask(SESSION_ID, idVerdict, {
                    ...ENUM_FIX,
                    status: "staged",
                }),
            (err) =>
                err instanceof Error &&
                /is currently reported .* never restores a pre-execution status/.test(
                    err.message,
                ),
            "arm 2b: verdict/execution status must not be repair-restorable to pre-execution",
        );

        // Arm 2c — ready-target requires the F3 design-readiness envelope
        // (review-block B-F1a): repairing a degraded `draft` card to `ready`
        // runs the SAME F3 validation readyCoordinationTask enforces. Pre-fix
        // the widening promoted the card with no envelope at all.
        const idReady = "dlk-a-ready-target";
        writeCard(root, idReady, {
            task_type: "",
            coordination_mode: "",
            primary_lane: "",
            rough_scope: ["ready-target F3 gate"],
            files_in_scope: FILL_FIX.files_in_scope,
            success_criteria: FILL_FIX.success_criteria,
            validation_plan: FILL_FIX.validation_plan,
        });
        assert.throws(
            () =>
                repairCoordinationTask(SESSION_ID, idReady, {
                    ...ENUM_FIX,
                    status: "ready",
                }),
            (err) =>
                err instanceof Error &&
                /F3 design-readiness gate refused .*missing_envelope/.test(
                    err.message,
                ),
            "arm 2c: ready-target repair without an F3 envelope must refuse",
        );
        // The sanctioned ready-target path: same repair plus an explicit-empty
        // envelope bound to the current design digest (the merged post-repair
        // design fields) passes the same validator and persists.
        const readyDesignDigest = expectedMergedDigest(idReady, {
            ...ENUM_FIX,
            ...FILL_FIX,
        });
        const repairedReady = repairCoordinationTask(SESSION_ID, idReady, {
            ...ENUM_FIX,
            status: "ready",
            f3_design_readiness: {
                ownership_hazards: [],
                design_digest: readyDesignDigest,
            },
        });
        assert.equal(
            repairedReady.task.status,
            "ready",
            "ready-target repair with a current envelope must succeed",
        );
        assert.equal(
            repairedReady.task.f3_design_readiness &&
                repairedReady.task.f3_design_readiness.design_digest,
            readyDesignDigest,
            "ready-target repair must persist the validated envelope",
        );

        // Arm 2d — staged -> ready is ALSO a BUILD-READY crossing (round-2
        // B-F3/B-F4): a degraded card whose stored status is a VALID
        // `staged` cannot slide to `ready` without re-earning F3. Pre-fix
        // only draft -> ready was treated as a crossing, so the staged ->
        // ready repair skipped the gate entirely.
        const idStagedReady = "errata-dlk-a-staged-ready";
        writeCard(root, idStagedReady, {
            task_type: "",
            coordination_mode: "",
            primary_lane: "",
            status: "staged",
            rough_scope: ["staged to ready must re-earn BUILD-READY"],
            files_in_scope: FILL_FIX.files_in_scope,
            success_criteria: FILL_FIX.success_criteria,
            validation_plan: FILL_FIX.validation_plan,
        });
        // 2d-1: no envelope anywhere -> missing_envelope refusal.
        assert.throws(
            () =>
                repairCoordinationTask(SESSION_ID, idStagedReady, {
                    ...ENUM_FIX,
                    status: "ready",
                }),
            (err) =>
                err instanceof Error &&
                /F3 design-readiness gate refused .*missing_envelope/.test(
                    err.message,
                ),
            "arm 2d-1: staged->ready repair with no F3 envelope must refuse",
        );
        // 2d-2: a persisted STALE envelope does not satisfy the crossing.
        const idStagedReadyStale = "errata-dlk-a-staged-ready-stale";
        writeCard(root, idStagedReadyStale, {
            task_type: "",
            coordination_mode: "",
            primary_lane: "",
            status: "staged",
            rough_scope: ["staged to ready with a stale persisted envelope"],
            files_in_scope: FILL_FIX.files_in_scope,
            success_criteria: FILL_FIX.success_criteria,
            validation_plan: FILL_FIX.validation_plan,
            f3_design_readiness: {
                ownership_hazards: [],
                design_digest: "0".repeat(64),
            },
        });
        assert.throws(
            () =>
                repairCoordinationTask(SESSION_ID, idStagedReadyStale, {
                    ...ENUM_FIX,
                    status: "ready",
                }),
            (err) =>
                err instanceof Error &&
                /F3 design-readiness gate refused .*stale_design_digest/.test(
                    err.message,
                ),
            "arm 2d-2: staged->ready repair against a stale persisted envelope must refuse",
        );
        // 2d-3: the sanctioned staged -> ready repair carries a fresh
        // envelope bound to the merged design digest.
        const stagedReadyDigest = expectedMergedDigest(idStagedReady, {
            ...ENUM_FIX,
            ...FILL_FIX,
        });
        const stagedToReady = repairCoordinationTask(SESSION_ID, idStagedReady, {
            ...ENUM_FIX,
            status: "ready",
            f3_design_readiness: {
                ownership_hazards: [],
                design_digest: stagedReadyDigest,
            },
        });
        assert.equal(
            stagedToReady.task.status,
            "ready",
            "arm 2d-3: staged->ready repair with a current envelope must succeed",
        );

        // Arm 3 — the update path STILL refuses enum fields (allowlists
        // intact; the widening lives in repair only).
        assert.throws(
            () =>
                updateCoordinationTaskMetadata(SESSION_ID, idA2, {
                    task_type: "implementation",
                }),
            (err) =>
                err instanceof Error &&
                /Unsupported fields for task metadata update while staged: task_type/.test(
                    err.message,
                ),
            "arm 3: update must keep refusing task_type under its allowlist",
        );

        // Arm 4 — prohibited-target refusal on a NON-offender (round-2
        // B-F1): a verdict/terminal status value in the payload is refused
        // outright — independent of offender classification — never merely
        // dropped as an "unsupported field".
        const idB = "errata-dlk-a-verdict-refused";
        writeCard(root, idB, {
            status: "staged",
            rough_scope: ["verdict boundary"],
            files_in_scope: [],
            success_criteria: [],
            validation_plan: [],
        });
        assert.throws(
            () =>
                repairCoordinationTask(SESSION_ID, idB, {
                    ...ENUM_FIX,
                    status: "completed",
                }),
            (err) =>
                err instanceof Error &&
                /repair never accepts a target status outside the pre-execution\/holding class \(draft \| ready \| staged\)/.test(
                    err.message,
                ),
            "arm 4: repair must refuse a verdict status target outright, on a non-offender",
        );

        // Arm 5 — invalid-status offender (round-2 B-F2): a card whose
        // STORED status is invalid (coerces to draft on read, so `status`
        // IS an offending field) must NOT be repairable to any prohibited
        // target. Pre-fix the payload status normalized to a valid enum,
        // the coerced-draft current status cleared the verdict-source
        // check, and the locked save never re-runs transition legality —
        // so each of these targets LANDED. Every attempt must now refuse
        // with NO write.
        const idC = "dlk-a-status-offender";
        writeCard(root, idC, {
            status: "bogus",
            rough_scope: ["status offender restore"],
            files_in_scope: [],
            success_criteria: [],
            validation_plan: [],
        });
        const rawBefore = readRawCard(root, idC);
        for (const prohibited of [
            "completed",
            "cancelled",
            "reported",
            "working",
            "blocked",
            "banana",
        ]) {
            assert.throws(
                () =>
                    repairCoordinationTask(SESSION_ID, idC, {
                        ...ENUM_FIX,
                        status: prohibited,
                        staged_path: FIXTURE_STAGED_PATH,
                    }),
                (err) =>
                    err instanceof Error &&
                    /repair never accepts a target status outside the pre-execution\/holding class/.test(
                        err.message,
                    ),
                `arm 5: prohibited target status ${prohibited} must refuse on an invalid-status offender`,
            );
        }
        assert.deepEqual(
            readRawCard(root, idC),
            rawBefore,
            "arm 5: refused prohibited-target attempts must not write the card",
        );

        // Arm 5b — sanctioned status-offender restore: the bogus-status
        // offender repairs to a VALID pre-execution status (draft) in one
        // op, restore-only semantics preserved.
        const restoredDraft = repairCoordinationTask(SESSION_ID, idC, {
            ...ENUM_FIX,
            status: "draft",
        });
        assert.ok(
            restoredDraft.repaired_fields.includes("status"),
            "arm 5b: status offender must be reported repaired",
        );
        const afterC = readCoordinationTask(SESSION_ID, idC);
        assert.equal(
            afterC.diagnostics && afterC.diagnostics.degraded,
            false,
            "arm 5b: restored card must load healthy",
        );
        assert.equal(afterC.task.status, "draft", "arm 5b: restored status");

        // Arm 5c — sanctioned invalid-status offender -> ready: crossing
        // from the coerced draft, so the F3 gate fires and a fresh envelope
        // satisfies it.
        const idCReady = "dlk-a-status-offender-ready";
        writeCard(root, idCReady, {
            status: "bogus",
            rough_scope: ["status offender to ready"],
            files_in_scope: FILL_FIX.files_in_scope,
            success_criteria: FILL_FIX.success_criteria,
            validation_plan: FILL_FIX.validation_plan,
        });
        const cReadyDigest = expectedMergedDigest(idCReady, {
            ...ENUM_FIX,
            ...FILL_FIX,
        });
        const restoredReady = repairCoordinationTask(SESSION_ID, idCReady, {
            ...ENUM_FIX,
            status: "ready",
            f3_design_readiness: {
                ownership_hazards: [],
                design_digest: cReadyDigest,
            },
        });
        assert.equal(
            restoredReady.task.status,
            "ready",
            "arm 5c: invalid-status offender must repair to ready under a fresh F3 envelope",
        );

        // Arm 6 — blocked source (round-2 C-F2): a degraded card whose
        // stored status is a VALID `blocked` is never repair-restored to a
        // pre-execution status — the review verdict parked it; unblocking
        // is a coordinator decision, not a field-repair side effect.
        const idBlocked = "dlk-a-blocked-source";
        writeCard(root, idBlocked, {
            task_type: "",
            coordination_mode: "",
            primary_lane: "",
            status: "blocked",
            rough_scope: ["blocked source never widens back"],
            files_in_scope: FILL_FIX.files_in_scope,
            success_criteria: FILL_FIX.success_criteria,
            validation_plan: FILL_FIX.validation_plan,
        });
        const blockedRawBefore = readRawCard(root, idBlocked);
        for (const target of ["draft", "ready", "staged"]) {
            const payload = { ...ENUM_FIX, status: target };
            if (target === "staged") {
                payload.staged_path = FIXTURE_STAGED_PATH;
            }
            if (target === "ready") {
                payload.f3_design_readiness = {
                    ownership_hazards: [],
                    design_digest: expectedMergedDigest(idBlocked, {
                        ...ENUM_FIX,
                        ...FILL_FIX,
                    }),
                };
            }
            assert.throws(
                () => repairCoordinationTask(SESSION_ID, idBlocked, payload),
                (err) =>
                    err instanceof Error &&
                    /is currently blocked .* never restores a pre-execution status/.test(
                        err.message,
                    ),
                `arm 6: blocked source must refuse a ${target}-target repair`,
            );
        }
        assert.deepEqual(
            readRawCard(root, idBlocked),
            blockedRawBefore,
            "arm 6: refused blocked-source attempts must not write the card",
        );

        // Arm 7 — staged-target restage guards (round-2 C-F1/C-F4): the
        // staged-target repair enforces the SAME class + source guards as
        // the dedicated restage op. Draft cannot jump to staged (even an
        // errata-class card carrying the full staged_path contract), and a
        // non-errata ready card cannot be staged through repair.
        const idDraftErrata = "errata-dlk-a-draft-to-staged";
        writeCard(root, idDraftErrata, {
            task_type: "",
            coordination_mode: "",
            primary_lane: "",
            status: "draft",
            rough_scope: ["draft cannot jump to staged via repair"],
            files_in_scope: FILL_FIX.files_in_scope,
            success_criteria: FILL_FIX.success_criteria,
            validation_plan: FILL_FIX.validation_plan,
        });
        assert.throws(
            () =>
                repairCoordinationTask(SESSION_ID, idDraftErrata, {
                    ...ENUM_FIX,
                    status: "staged",
                    staged_path: FIXTURE_STAGED_PATH,
                }),
            (err) =>
                err instanceof Error &&
                /is draft and cannot be repaired to staged/.test(err.message),
            "arm 7a: draft errata->staged bypass attempt must refuse (source allowlist)",
        );
        const idReadyNonErrata = "dlk-a-ready-nonerrata-staged";
        writeCard(root, idReadyNonErrata, {
            task_type: "",
            coordination_mode: "",
            primary_lane: "",
            status: "ready",
            rough_scope: ["non-errata card cannot be staged via repair"],
            files_in_scope: FILL_FIX.files_in_scope,
            success_criteria: FILL_FIX.success_criteria,
            validation_plan: FILL_FIX.validation_plan,
        });
        assert.throws(
            () =>
                repairCoordinationTask(SESSION_ID, idReadyNonErrata, {
                    ...ENUM_FIX,
                    status: "staged",
                    staged_path: FIXTURE_STAGED_PATH,
                }),
            (err) =>
                err instanceof Error &&
                /is not an errata-class card .*task_id must begin "errata-"/.test(
                    err.message,
                ),
            "arm 7b: ready non-errata->staged must refuse (errata-class guard)",
        );

        // (Arm 8 — digest canonicalization of files_in_scope FILLS — was
        // REMOVED by the round-5 narrowing: with no fill path there is no
        // fill-path canonicalization to pin. The digest helper still
        // canonicalizes fixture scope fed through expectedMergedDigest, and
        // arm 9's launderF3Digest recomputes a whole-design digest directly.)

        // Arm 9 — normalization-laundered execution status (round-3
        // b-F1/b-F2): a stored-`working` errata card whose ownership/claim
        // metadata cannot be recovered (empty session_aliases, absent
        // active_session_alias + claimed_at) is PROJECTED to `ready` by the
        // fault-tolerant read, and the card carries an independently
        // degraded core field (empty title) so the repair dispatches into
        // the degraded-core branch. Pre-fix the guards authorized on the
        // projected `ready`, so the staged-target repair below LANDED —
        // laundering the execution state through the diagnostic boundary.
        // Every repair lifecycle check now authorizes on the STORED
        // status: all four attempts (staged/draft/ready targets plus the
        // title-only repair that would persist the laundered projection
        // itself) must refuse naming the execution status, with the stored
        // bytes untouched.
        const idLaunder = "errata-dlk-a-working-launder";
        writeCard(root, idLaunder, {
            ...ENUM_FIX,
            title: "",
            status: "working",
            session_aliases: [],
            active_session_alias: null,
            rough_scope: ["stored working must not launder to ready"],
            files_in_scope: FILL_FIX.files_in_scope,
            success_criteria: FILL_FIX.success_criteria,
            validation_plan: FILL_FIX.validation_plan,
        });
        // The laundering premise, pinned: the normalized projection reads
        // `ready` (the ownership-metadata rewrite fired) while the card is
        // degraded by the empty title.
        const launderedRead = readCoordinationTask(SESSION_ID, idLaunder);
        assert.equal(
            launderedRead.task.status,
            "ready",
            "arm 9 premise: unrecoverable ownership metadata must project the stored working card as ready",
        );
        assert.equal(
            launderedRead.diagnostics && launderedRead.diagnostics.degraded,
            true,
            "arm 9 premise: the empty title must keep the card degraded (degraded-branch dispatch)",
        );
        const launderRawBefore = readRawCard(root, idLaunder);
        const launderTitle = "restored title on a stored-working card";
        // staged-target attempt (the headline laundering case): restore
        // title + status staged + staged_path on an errata-class id.
        assert.throws(
            () =>
                repairCoordinationTask(SESSION_ID, idLaunder, {
                    title: launderTitle,
                    status: "staged",
                    staged_path: FIXTURE_STAGED_PATH,
                }),
            (err) =>
                err instanceof Error &&
                /is currently working .* never restores a pre-execution status/.test(
                    err.message,
                ),
            "arm 9a: stored working must refuse a staged-target repair even when the normalized projection reads ready",
        );
        // Execution-source matrix extension (draft-target and ready-target
        // attempts equally refused; the ready attempt carries a fresh F3
        // envelope bound to the merged design so ONLY the execution-source
        // guard can refuse it).
        const launderF3Digest = computeTaskDesignDigest(
            {
                task_id: idLaunder,
                title: launderTitle,
                task_type: ENUM_FIX.task_type,
                primary_lane: ENUM_FIX.primary_lane,
                files_in_scope: FILL_FIX.files_in_scope,
                success_criteria: FILL_FIX.success_criteria,
                constraints: [],
                non_goals: [],
                validation_plan: FILL_FIX.validation_plan,
            },
            {},
        );
        for (const target of ["draft", "ready"]) {
            const payload = { title: launderTitle, status: target };
            if (target === "ready") {
                payload.f3_design_readiness = {
                    ownership_hazards: [],
                    design_digest: launderF3Digest,
                };
            }
            assert.throws(
                () => repairCoordinationTask(SESSION_ID, idLaunder, payload),
                (err) =>
                    err instanceof Error &&
                    /is currently working .* never restores a pre-execution status/.test(
                        err.message,
                    ),
                `arm 9b: stored working must refuse a ${target}-target repair (normalization laundering)`,
            );
        }
        // Title-only repair (no explicit status): the save persists the
        // NORMALIZED projection — for this card that is the laundered
        // `ready` — so it is equally a pre-execution restoration from a
        // stored execution status and must refuse.
        assert.throws(
            () =>
                repairCoordinationTask(SESSION_ID, idLaunder, {
                    title: launderTitle,
                }),
            (err) =>
                err instanceof Error &&
                /is currently working .* never restores a pre-execution status/.test(
                    err.message,
                ),
            "arm 9c: title-only repair must not persist the laundered ready projection",
        );
        assert.deepEqual(
            readRawCard(root, idLaunder),
            launderRawBefore,
            "arm 9: refused laundering attempts must not write the card",
        );
    }),
);

test(
    "deadlock B: ready -> staged reachable via the dedicated errata-class restage op; review never resolves into staged",
    withIsolatedTransport((root) => {
        const id = "errata-dlk-b-restage";
        writeCard(root, id, {
            task_type: "implementation",
            coordination_mode: "short",
            primary_lane: "release",
            report_envelope: "minimal",
            status: "ready",
            rough_scope: ["staged erratum transport"],
            files_in_scope: ["docs/migration-errata/x.md"],
            success_criteria: ["next release note carries the errata section"],
            validation_plan: ["release ceremony inject-errata step reads the card"],
        });

        // The OLD dead ends stay closed (receipts — both must refuse):
        assert.throws(
            () =>
                updateCoordinationTaskMetadata(SESSION_ID, id, {
                    status: "staged",
                    staged_path: "docs/migration-errata/x.md",
                }),
            (err) =>
                err instanceof Error &&
                /Unsupported fields for task metadata update while ready: status, staged_path/.test(
                    err.message,
                ),
            "update must keep refusing status/staged_path on a ready card",
        );
        assert.throws(
            () =>
                reviewCoordinationTask(SESSION_ID, id, {
                    status: "staged",
                    note: "stage for release injection",
                }),
            (err) =>
                err instanceof Error &&
                /is ready and is not ready for coordinator review/.test(
                    err.message,
                ),
            "review must keep refusing a ready card",
        );

        // The sanctioned path: happy restage (errata-class card).
        const staged = restageCoordinationTask(SESSION_ID, id, {
            staged_path: "docs/migration-errata/x.md",
        });
        assert.equal(staged.task.status, "staged", "restage must land status staged");
        assert.equal(
            staged.task.staged_path,
            "docs/migration-errata/x.md",
            "restage must persist staged_path",
        );
        assert.match(
            String(staged.task.next_action || ""),
            /release/,
            "restage must set the release-oriented next_action",
        );
        assert.ok(
            (staged.task.history || []).some(
                (entry) => entry.event === "task_restaged",
            ),
            "restage must append a task_restaged history event",
        );
        // The persisted bytes carry the transition (not just the return value).
        const raw = readRawCard(root, id);
        assert.equal(raw.status, "staged");
        assert.equal(raw.staged_path, "docs/migration-errata/x.md");

        // Idempotent staged -> staged refresh with a new path.
        const refreshed = restageCoordinationTask(SESSION_ID, id, {
            staged_path: "docs/migration-errata/y.md",
            note: "refreshed fragment",
        });
        assert.equal(refreshed.task.status, "staged");
        assert.equal(
            refreshed.task.staged_path,
            "docs/migration-errata/y.md",
            "re-stage must refresh staged_path",
        );
        assert.equal(
            (refreshed.task.history || []).filter(
                (entry) => entry.event === "task_restaged",
            ).length,
            2,
            "re-stage must append a second task_restaged event",
        );

        // Guard receipts: missing path, absolute path, traversal, wrong
        // source, degraded card, non-errata class.
        assert.throws(
            () => restageCoordinationTask(SESSION_ID, id, {}),
            (err) =>
                err instanceof Error && /requires a staged_path/.test(err.message),
            "restage without staged_path must refuse",
        );
        assert.throws(
            () =>
                restageCoordinationTask(SESSION_ID, id, {
                    staged_path: "/abs/not/allowed.md",
                }),
            (err) =>
                err instanceof Error &&
                /repo-relative path without parent traversal/.test(err.message),
            "absolute staged_path must refuse",
        );
        assert.throws(
            () =>
                restageCoordinationTask(SESSION_ID, id, {
                    staged_path: "docs/../escape.md",
                }),
            (err) =>
                err instanceof Error &&
                /repo-relative path without parent traversal/.test(err.message),
            "parent-traversal staged_path must refuse",
        );
        const idWrong = "errata-dlk-b-wrong-source";
        writeCard(root, idWrong, {
            task_type: "implementation",
            coordination_mode: "short",
            primary_lane: "release",
            status: "draft",
            rough_scope: ["x"],
        });
        assert.throws(
            () =>
                restageCoordinationTask(SESSION_ID, idWrong, {
                    staged_path: "docs/migration-errata/z.md",
                }),
            (err) =>
                err instanceof Error &&
                /is draft and cannot be staged for release inject-errata/.test(
                    err.message,
                ),
            "restage must refuse a draft card",
        );
        const idDeg = "errata-dlk-b-degraded";
        writeCard(root, idDeg, {
            status: "bogus",
            rough_scope: ["x"],
        });
        assert.throws(
            () =>
                restageCoordinationTask(SESSION_ID, idDeg, {
                    staged_path: "docs/migration-errata/z.md",
                }),
            (err) =>
                err instanceof Error &&
                /is degraded .* cannot be staged for release inject-errata/.test(
                    err.message,
                ),
            "restage must refuse a degraded card at the action boundary",
        );

        // Errata-class restriction (review-block C-F5): restage is the
        // release inject-errata transport for errata-class cards only; a
        // healthy non-errata ready card is refused with a pointer at the
        // ordinary lifecycle ops. Pre-fix this restage succeeded.
        const idNotErrata = "dlk-b-not-errata";
        writeCard(root, idNotErrata, {
            task_type: "implementation",
            coordination_mode: "short",
            primary_lane: "release",
            status: "ready",
            rough_scope: ["ordinary card, never errata transport"],
            files_in_scope: ["docs/migration-errata/none.md"],
            success_criteria: ["refused at the class boundary"],
            validation_plan: ["class guard refuses"],
        });
        assert.throws(
            () =>
                restageCoordinationTask(SESSION_ID, idNotErrata, {
                    staged_path: "docs/migration-errata/none.md",
                }),
            (err) =>
                err instanceof Error &&
                /is not an errata-class card .*task_id must begin "errata-"/.test(
                    err.message,
                ),
            "restage must refuse a non-errata-class card",
        );

        // Review result-status allowlist (review-block C-F1): reviewing a
        // REPORTED card into `staged` is refused — the accepted result
        // statuses are an explicit frozen allowlist, never derived from the
        // widened lifecycle enum. Pre-fix the review resolved the card into
        // staged (bypassing the restage op's staged_path contract).
        const idReported = "dlk-b-reported-review";
        writeCard(root, idReported, {
            task_type: "implementation",
            coordination_mode: "short",
            primary_lane: "release",
            status: "reported",
            rough_scope: ["review verdict boundary"],
            files_in_scope: ["docs/migration-errata/none.md"],
            success_criteria: ["review cannot resolve into staged"],
            validation_plan: ["explicit allowlist refuses staged"],
            latest_report: {
                path: ".local/coordinator/reports/dlk-b-reported-review/closeout.md",
                title: "fixture closeout",
                status: "reported",
                report_envelope: "minimal",
                created_at: "2026-11-16T00:00:00Z",
                summary: "fixture report for the review allowlist arm",
                promotion_recommended: false,
            },
        });
        assert.throws(
            () =>
                reviewCoordinationTask(SESSION_ID, idReported, {
                    body: "attempting to resolve the card into staged via review",
                    taskStatus: "staged",
                }),
            (err) =>
                err instanceof Error &&
                /task_review should resolve to ready, reported, blocked, completed, or cancelled/.test(
                    err.message,
                ),
            "review must refuse to resolve a reported card into staged",
        );
        // The card itself is untouched by the refused review (no status move,
        // no review artifact side effects on the card).
        const reportedAfter = readRawCard(root, idReported);
        assert.equal(
            reportedAfter.status,
            "reported",
            "refused review must not move the card status",
        );
        assert.deepEqual(
            reportedAfter.review_paths || [],
            [],
            "refused review must not append a review path",
        );

        // Post-restage metadata updates on a staged card: narrow follow-up
        // class accepted, staged_path NOT updatable through the generic op.
        const updated = updateCoordinationTaskMetadata(SESSION_ID, id, {
            owner_notes: ["holding for release injection"],
        });
        assert.equal(
            updated.task.owner_notes.includes("holding for release injection"),
            true,
            "staged card must accept owner_notes follow-up updates",
        );
        assert.throws(
            () =>
                updateCoordinationTaskMetadata(SESSION_ID, id, {
                    staged_path: "docs/migration-errata/other.md",
                }),
            (err) =>
                err instanceof Error &&
                /Unsupported fields for task metadata update while staged: staged_path/.test(
                    err.message,
                ),
            "generic update must keep refusing staged_path on a staged card",
        );
    }),
);

// --- round-2 C-F6: PUBLIC plan-state dispatcher parity -----------------------
//
// The state-lib-direct arms above prove repairCoordinationTask /
// restageCoordinationTask refuse and land correctly, but the PUBLIC surface
// agents actually drive is the plan_state tool's operation dispatcher. This
// arm exercises `restage_coordination_task` END-TO-END through the real
// tool entry (rendered plan-state.js imported under a sandboxed
// @opencode-ai/plugin stub — the repo has no node_modules, and the stub
// only needs tool() to hand execute(args, context) through), asserting the
// SAME outcomes as the state-lib-direct arms.
test(
    "round-2 C-F6: plan-state restage_coordination_task dispatcher end-to-end (sandboxed tool entry)",
    withIsolatedTransport(async (root) => {
        const sandbox = path.join(root, "plan-state-sandbox");
        const pluginDir = path.join(
            sandbox,
            "node_modules",
            "@opencode-ai",
            "plugin",
        );
        fs.mkdirSync(pluginDir, { recursive: true });
        fs.writeFileSync(
            path.join(pluginDir, "package.json"),
            JSON.stringify(
                {
                    name: "@opencode-ai/plugin",
                    version: "0.0.0-test-stub",
                    type: "module",
                    main: "index.js",
                    exports: { ".": "./index.js" },
                },
                null,
                2,
            ),
        );
        // Minimal permissive tool() factory: schema builders return
        // chainable no-ops (arg validation is the real host's job); execute
        // passes (args, context) straight through to the tool definition.
        fs.writeFileSync(
            path.join(pluginDir, "index.js"),
            [
                "function chainable() {",
                "    const o = {};",
                "    for (const m of [\"optional\", \"describe\", \"minLength\", \"maxLength\", \"minItems\", \"maxItems\", \"items\", \"properties\", \"default\"]) {",
                "        o[m] = () => o;",
                "    }",
                "    return o;",
                "}",
                "export function tool(def) {",
                "    return {",
                "        description: def.description,",
                "        args: def.args,",
                "        async execute(args, context) {",
                "            return def.execute(args, context);",
                "        },",
                "    };",
                "}",
                "tool.schema = {",
                "    string: () => chainable(),",
                "    boolean: () => chainable(),",
                "    number: () => chainable(),",
                "    array: () => chainable(),",
                "    object: () => chainable(),",
                "    enum: () => chainable(),",
                "};",
                "",
            ].join("\n"),
        );
        fs.mkdirSync(path.join(sandbox, "tools"), { recursive: true });
        fs.copyFileSync(
            path.join(REPO_ROOT, ".opencode", "tools", "plan-state.js"),
            path.join(sandbox, "tools", "plan-state.js"),
        );
        // state-lib.js statically imports its sibling script modules
        // (f3-design-readiness.js, rewrite-parity-validate.js,
        // pause-new-work.js) — copy the whole scripts tree so the sandbox
        // module graph resolves exactly like the rendered one.
        fs.cpSync(
            path.join(REPO_ROOT, ".opencode", "scripts"),
            path.join(sandbox, "scripts"),
            { recursive: true },
        );

        const { planStateTool } = await import(
            pathToFileURL(path.join(sandbox, "tools", "plan-state.js")).href
        );
        const context = {
            sessionID: SESSION_ID,
            directory: REPO_ROOT,
            metadata: () => {},
        };

        // Bind the session through the dispatcher itself (same setup the
        // direct arms get from bindSessionName).
        const bound = JSON.parse(
            await planStateTool.execute(
                { operation: "bind_session_name", session_name: "repair-deadlock-dispatch" },
                context,
            ),
        );
        assert.ok(bound.session_name, "bind_session_name must succeed through the dispatcher");

        // Happy path: an errata-class ready card restages end-to-end.
        const idOk = "errata-dlk-c-dispatch";
        writeCard(root, idOk, {
            task_type: "implementation",
            coordination_mode: "short",
            primary_lane: "release",
            status: "ready",
            rough_scope: ["dispatcher parity"],
            files_in_scope: ["docs/migration-errata/x.md"],
            success_criteria: ["dispatcher lands the same outcome"],
            validation_plan: ["plan-state restage_coordination_task arm"],
        });
        const restaged = JSON.parse(
            await planStateTool.execute(
                {
                    operation: "restage_coordination_task",
                    task_id: idOk,
                    staged_path: "docs/migration-errata/x.md",
                },
                context,
            ),
        );
        assert.equal(
            restaged.task.status,
            "staged",
            "dispatcher restage must land status staged",
        );
        assert.equal(
            restaged.task.staged_path,
            "docs/migration-errata/x.md",
            "dispatcher restage must persist staged_path",
        );
        const rawOk = readRawCard(root, idOk);
        assert.equal(rawOk.status, "staged", "persisted bytes must carry the transition");
        assert.ok(
            (rawOk.history || []).some((e) => e.event === "task_restaged"),
            "dispatcher restage must append the task_restaged event",
        );

        // Refusals: the dispatcher carries the SAME guard outcomes as the
        // state-lib-direct arms (missing staged_path, draft source,
        // non-errata class).
        await assert.rejects(
            () =>
                planStateTool.execute(
                    { operation: "restage_coordination_task", task_id: idOk },
                    context,
                ),
            (err) =>
                err instanceof Error && /requires a staged_path/.test(err.message),
            "dispatcher must refuse restage without staged_path",
        );
        const idDraft = "errata-dlk-c-dispatch-draft";
        writeCard(root, idDraft, {
            task_type: "implementation",
            coordination_mode: "short",
            primary_lane: "release",
            status: "draft",
            rough_scope: ["x"],
        });
        await assert.rejects(
            () =>
                planStateTool.execute(
                    {
                        operation: "restage_coordination_task",
                        task_id: idDraft,
                        staged_path: "docs/migration-errata/z.md",
                    },
                    context,
                ),
            (err) =>
                err instanceof Error &&
                /is draft and cannot be staged for release inject-errata/.test(
                    err.message,
                ),
            "dispatcher must refuse a draft card (source allowlist)",
        );
        const idNotErrata = "dlk-c-dispatch-not-errata";
        writeCard(root, idNotErrata, {
            task_type: "implementation",
            coordination_mode: "short",
            primary_lane: "release",
            status: "ready",
            rough_scope: ["x"],
            files_in_scope: ["docs/migration-errata/none.md"],
            success_criteria: ["class guard refuses"],
            validation_plan: ["class guard refuses"],
        });
        await assert.rejects(
            () =>
                planStateTool.execute(
                    {
                        operation: "restage_coordination_task",
                        task_id: idNotErrata,
                        staged_path: "docs/migration-errata/none.md",
                    },
                    context,
                ),
            (err) =>
                err instanceof Error &&
                /is not an errata-class card .*task_id must begin "errata-"/.test(
                    err.message,
                ),
            "dispatcher must refuse a non-errata-class card",
        );
        // No side effects from the refused dispatches.
        assert.equal(
            readRawCard(root, idNotErrata).status,
            "ready",
            "refused dispatcher restage must not move the card",
        );
    }),
);

// --- round-4 B-F1/B-F2: restage source allowlist on the STORED status ------
//
// The round-3 laundering guard covered the REPAIR branch; rejection #3
// (round 4) named the same laundering vector on the dedicated restage op:
// its ready|staged source allowlist authorized on the post-normalization
// projection at BOTH guard sites, so a NON-degraded stored-`working`
// errata card whose ownership/claim metadata cannot be recovered —
// normalizeCoordinationTaskRecord projects it to ready WITHOUT flagging
// degradation (the projection is a read-resilience behavior, not a
// diagnostic) — restaged into `staged`. Both sites now authorize on the
// stored status (loaded.stored_status pre-lock; a locked re-load
// in-lock), the same authority the round-3 repair guard uses.
test(
    "round-4 B-F1/B-F2: restage refuses a stored-working card that projects ready (no laundering through the dedicated op)",
    withIsolatedTransport((root) => {
        const id = "errata-dlk-d-working-launder";
        writeCard(root, id, {
            ...ENUM_FIX,
            status: "working",
            // Unrecoverable claim metadata: empty session_aliases, absent
            // active_session_alias AND claimed_at, and no updated_at/
            // created_at fallback timestamps to re-derive claimed_at from —
            // the ownership-metadata rewrite can recover NEITHER field, so
            // the card projects ready. Everything else is valid: the card
            // is deliberately NON-degraded (only a valid `working` enum
            // plus missing claim metadata — no offending fields).
            session_aliases: [],
            active_session_alias: null,
            claimed_at: null,
            created_at: null,
            updated_at: null,
            rough_scope: [
                "stored working must not restage through its ready projection",
            ],
            files_in_scope: FILL_FIX.files_in_scope,
            success_criteria: FILL_FIX.success_criteria,
            validation_plan: FILL_FIX.validation_plan,
        });
        // Premise pins (so the arm cannot silently rot): the projection
        // reads `ready` while the card is NOT degraded — exactly the
        // laundering shape that passes the pre-lock degraded boundary.
        const premise = readCoordinationTask(SESSION_ID, id);
        assert.equal(
            premise.task.status,
            "ready",
            "arm 10 premise: unrecoverable claim metadata must project the stored working card as ready",
        );
        assert.equal(
            premise.diagnostics && premise.diagnostics.degraded,
            false,
            "arm 10 premise: the card must be NON-degraded (only the claim metadata is unrecoverable)",
        );
        const before = readRawCard(root, id);
        // The refusal names the STORED status (`working`), not the
        // laundered projection (`ready`).
        assert.throws(
            () =>
                restageCoordinationTask(SESSION_ID, id, {
                    staged_path: FIXTURE_STAGED_PATH,
                }),
            (err) =>
                err instanceof Error &&
                /is working and cannot be staged for release inject-errata/.test(
                    err.message,
                ),
            "arm 10: restage must refuse a stored-working card whose projection reads ready, naming the stored status",
        );
        assert.deepEqual(
            readRawCard(root, id),
            before,
            "arm 10: the refused restage must leave the stored bytes unchanged",
        );
    }),
);

// --- round-4 C-F2: cross-platform absolute staged_path refusal -------------
//
// The repo-relative staged_path guard was POSIX-only: it refused the
// '/'-prefix and '..' traversal forms but passed Windows drive-absolute
// ("C:/outside.md", "C:\\outside.md") and UNC ("\\server\share\x.md")
// paths. release_inject.go honors an ABSOLUTE staged_path at inject time
// (only a non-absolute path is joined to the repo root), so a staged card
// carrying such a path reads its erratum body from OUTSIDE the repository
// at the release ceremony — the JS write side is the gate. The SHARED
// validator (both entry paths into staged: the dedicated restage op and a
// staged-target repair) refuses all three cross-platform absolute forms
// alongside the existing POSIX checks.
test(
    "round-4 C-F2: Windows drive-absolute and UNC staged_path refused through BOTH entry paths",
    withIsolatedTransport((root) => {
        const windowsAbsolutePaths = [
            "C:/outside.md",
            "C:\\outside.md",
            "\\\\server\\share\\x.md",
        ];
        // Entry path 1 — the dedicated restage op on a healthy ready
        // errata card (class and source guards pass; the staged_path
        // guard is what must refuse). Pre-fix all three paths landed.
        const idReady = "errata-dlk-d-win-ready";
        writeCard(root, idReady, {
            ...ENUM_FIX,
            primary_lane: "release",
            status: "ready",
            rough_scope: ["windows staged_path restage refusal"],
            files_in_scope: ["docs/migration-errata/x.md"],
            success_criteria: ["cross-platform absolute staged_path refused"],
            validation_plan: ["shared validator restage arm"],
        });
        const readyBefore = readRawCard(root, idReady);
        for (const bad of windowsAbsolutePaths) {
            assert.throws(
                () =>
                    restageCoordinationTask(SESSION_ID, idReady, {
                        staged_path: bad,
                    }),
                (err) =>
                    err instanceof Error &&
                    /repo-relative path without parent traversal/.test(
                        err.message,
                    ),
                `arm 11 (restage): Windows-absolute staged_path ${JSON.stringify(bad)} must refuse`,
            );
        }
        assert.deepEqual(
            readRawCard(root, idReady),
            readyBefore,
            "arm 11 (restage): refused Windows-absolute paths must leave the card unchanged",
        );
        // Entry path 2 — a staged-target repair on a degraded errata
        // card whose merged preview is otherwise VALID (enums supplied;
        // the DoR demands answered by the card's STORED values — round-5:
        // fills no longer ride the payload), so only the shared
        // staged_path guard can refuse. Pre-fix the repair landed the
        // card as staged with the out-of-repo fragment path.
        const idDegraded = "errata-dlk-d-win-degraded";
        writeCard(root, idDegraded, {
            status: "staged",
            rough_scope: ["windows staged_path repair refusal"],
            files_in_scope: FILL_FIX.files_in_scope,
            success_criteria: FILL_FIX.success_criteria,
            validation_plan: FILL_FIX.validation_plan,
        });
        const degradedBefore = readRawCard(root, idDegraded);
        for (const bad of windowsAbsolutePaths) {
            assert.throws(
                () =>
                    repairCoordinationTask(SESSION_ID, idDegraded, {
                        ...ENUM_FIX,
                        status: "staged",
                        staged_path: bad,
                    }),
                (err) =>
                    err instanceof Error &&
                    /repo-relative path without parent traversal/.test(
                        err.message,
                    ),
                `arm 11 (repair): Windows-absolute staged_path ${JSON.stringify(bad)} must refuse`,
            );
        }
        assert.deepEqual(
            readRawCard(root, idDegraded),
            degradedBefore,
            "arm 11 (repair): refused Windows-absolute paths must leave the card unchanged",
        );
    }),
);

// --- round-5 narrowing: DoR-field fills refused on execution-state cards --
//
// The B-F1 execution-contract-freeze repro, pinned as the narrowed
// surface's refusal contract. A repair payload carrying
// files_in_scope/success_criteria/validation_plan (status omitted) must
// refuse with the standard "Unsupported fields for degraded task repair"
// diagnostic on BOTH:
//   (i)  a degraded `completed` card (empty title) — the terminal-status
//        shape. Pre-narrowing RED receipt (repro against the pre-narrowing
//        rendered twin, recorded in the carrier doc): the payload LANDED
//        and replaced all three execution-contract fields on the completed
//        card (files_in_scope ["different.js"], success_criteria
//        ["different"], validation_plan ["different"]).
//   (ii) a validly claimed `working` card (empty title; recoverable claim
//        metadata so the projection stays `working` — not the round-3
//        laundered-ready shape) — the active-execution shape. Pre-narrowing
//        RED receipt: same landing.
// With no execution-contract field replacement existing at all, the
// task-update freeze (no files_in_scope/success_criteria/validation_plan
// changes after execution starts) holds trivially. Both refusals leave
// the stored bytes deepEqual-unchanged.
test(
    "round-5 narrowing: DoR-field repair payloads refused on degraded completed and validly-claimed working cards",
    withIsolatedTransport((root) => {
        const payload = {
            title: "replacement title",
            files_in_scope: ["different.js"],
            success_criteria: ["different"],
            validation_plan: ["different"],
        };

        // Pin (i) — degraded completed card (empty title; valid enums and
        // DoR values on the stored body).
        const idCompleted = "dlk-e-completed-dor";
        writeCard(root, idCompleted, {
            title: "",
            task_type: "implementation",
            coordination_mode: "short",
            primary_lane: "coordination",
            report_envelope: "minimal",
            status: "completed",
            rough_scope: ["completed-card DoR fill refusal"],
            files_in_scope: ["original.js"],
            success_criteria: ["original"],
            validation_plan: ["original"],
        });
        const completedBefore = readRawCard(root, idCompleted);
        assert.throws(
            () =>
                repairCoordinationTask(SESSION_ID, idCompleted, { ...payload }),
            (err) =>
                err instanceof Error &&
                /Unsupported fields for degraded task repair: files_in_scope, success_criteria, validation_plan/.test(
                    err.message,
                ),
            "pin (i): a degraded completed card must refuse a DoR-field payload as unsupported fields",
        );
        assert.deepEqual(
            readRawCard(root, idCompleted),
            completedBefore,
            "pin (i): refused DoR-field payload must leave the card bytes unchanged",
        );

        // Pin (ii) — validly claimed working card (empty title; the claim
        // metadata is recoverable so the normalized projection stays the
        // stored `working`).
        const idWorking = "dlk-e-working-dor";
        writeCard(root, idWorking, {
            title: "",
            task_type: "implementation",
            coordination_mode: "short",
            primary_lane: "coordination",
            report_envelope: "minimal",
            status: "working",
            session_aliases: ["dlk-e-owner"],
            active_session_alias: "dlk-e-owner",
            claimed_at: "2026-11-16T00:00:00Z",
            rough_scope: ["working-card DoR fill refusal"],
            files_in_scope: ["original.js"],
            success_criteria: ["original"],
            validation_plan: ["original"],
        });
        const workingBefore = readRawCard(root, idWorking);
        assert.throws(
            () =>
                repairCoordinationTask(SESSION_ID, idWorking, { ...payload }),
            (err) =>
                err instanceof Error &&
                /Unsupported fields for degraded task repair: files_in_scope, success_criteria, validation_plan/.test(
                    err.message,
                ),
            "pin (ii): a validly claimed working card must refuse the same DoR-field payload",
        );
        assert.deepEqual(
            readRawCard(root, idWorking),
            workingBefore,
            "pin (ii): refused DoR-field payload must leave the card bytes unchanged",
        );
    }),
);

// --- round-6 directional guard: staged -> draft demotion refused -----------
//
// defer-repair-staged-source-refusal (2026-11-16 study+debate+go chain;
// 84c1097 cascade leaf A-F1; debate verdict P3 directional): `staged` is
// deliberately ABSENT from the repair op's execution-source refusal set —
// staged is a release-HOLDING status, and the sanctioned moves out of it
// (staged -> staged refresh with staged_path; the F3-gated staged -> ready
// restore, both pinned above by arms 2/2d) must keep working. The ONE
// refused direction is the demotion to draft, which would strand a card
// carrying release-ceremony state (staged_path, the inject-errata
// transport) back in pre-execution holding. The guard authorizes on the
// stored-first currentStatus against the EFFECTIVE PERSISTED target
// (explicit payload status, else the normalized projection the locked
// save re-derives), so an omitted status cannot bypass it.
test(
    "round-6: staged -> draft demotion refused (directional guard); staged_path and F3 envelopes do not authorize it",
    withIsolatedTransport((root) => {
        // Fixture: a stored-staged card degraded by missing enums with a
        // valid stored body otherwise (rough_scope answers the merged-draft
        // demand; the DoR lists answer the staged demands). Pre-fix this is
        // exactly the card a status:"draft" repair DEMOTED (the RED
        // receipt: the repair landed and the stored card became draft).
        const idDemote = "dlk-f-staged-demote";
        writeCard(root, idDemote, {
            status: "staged",
            rough_scope: ["staged source must not demote to draft via repair"],
            files_in_scope: FILL_FIX.files_in_scope,
            success_criteria: FILL_FIX.success_criteria,
            validation_plan: FILL_FIX.validation_plan,
        });
        const demoteRead = readCoordinationTask(SESSION_ID, idDemote);
        assert.equal(
            demoteRead.diagnostics && demoteRead.diagnostics.degraded,
            true,
            "round-6 premise: the fixture must load degraded (missing enums)",
        );
        const demoteBefore = readRawCard(root, idDemote);

        // Arm 1 — explicit status:"draft" on a stored-staged card: the
        // directional refusal, exact message.
        assert.throws(
            () =>
                repairCoordinationTask(SESSION_ID, idDemote, {
                    ...ENUM_FIX,
                    status: "draft",
                }),
            (err) =>
                err instanceof Error &&
                err.message ===
                    "Task dlk-f-staged-demote is staged for release; degraded-card repair cannot demote it to draft. Refresh staged with staged_path, or restore ready through the F3 design-readiness gate.",
            "round-6 arm 1: staged->draft demotion must refuse with the exact directional message",
        );
        assert.deepEqual(
            readRawCard(root, idDemote),
            demoteBefore,
            "round-6 arm 1: refused staged->draft demotion must leave the stored bytes unchanged",
        );

        // Arm 2 — neither release artifact authorizes the demotion: adding
        // the staged-refresh artifact (staged_path) or a ready-crossing F3
        // envelope to the draft-target payload is still refused. A draft
        // target admits no supplement (the accepted-field slots key on the
        // INTENDED target status, fail-closed), so these refuse at the
        // unsupported-fields boundary before any transition check runs —
        // pinned here so the smuggling shape cannot silently become
        // accepted later.
        assert.throws(
            () =>
                repairCoordinationTask(SESSION_ID, idDemote, {
                    ...ENUM_FIX,
                    status: "draft",
                    staged_path: FIXTURE_STAGED_PATH,
                }),
            (err) =>
                err instanceof Error &&
                /Unsupported fields for degraded task repair: staged_path/.test(
                    err.message,
                ),
            "round-6 arm 2a: staged_path must not authorize a staged->draft demotion",
        );
        assert.throws(
            () =>
                repairCoordinationTask(SESSION_ID, idDemote, {
                    ...ENUM_FIX,
                    status: "draft",
                    f3_design_readiness: {
                        ownership_hazards: [],
                        design_digest: "0".repeat(64),
                    },
                }),
            (err) =>
                err instanceof Error &&
                /Unsupported fields for degraded task repair: f3_design_readiness/.test(
                    err.message,
                ),
            "round-6 arm 2b: an F3 envelope must not authorize a staged->draft demotion",
        );
        assert.deepEqual(
            readRawCard(root, idDemote),
            demoteBefore,
            "round-6 arm 2: refused ride-along attempts must leave the stored bytes unchanged",
        );

        // Arm 3 — omitted-status case (ASSESSMENT, deliberately no runtime
        // arm): could a staged-source card with NO explicit status payload
        // carry an effective persisted target of draft? targetStatus falls
        // back to the normalized projection (loaded.payload.status) — the
        // value the locked save re-derives — and the read path rewrites
        // ONLY an unrecoverable `working` to ready
        // (normalizeCoordinationTaskRecord); a draft projection arises only
        // from an invalid/absent stored status. But an invalid stored
        // status makes coordinationTaskStoredStatus return "" (currentStatus
        // then falls to that same draft projection and the source is not
        // staged either), and a valid stored `staged` projects as `staged`
        // itself. So `currentStatus === "staged" && targetStatus ===
        // "draft"` is NOT constructible with an omitted status — the guard
        // fires only on an explicit draft payload (arm 1). Per the
        // adjudicated design, no fake arm is built for this shape.
    }),
);

// --- watch pin (defer-staged-draft-omitted-status-watch, 2026-11-16): ----
// a valid stored `staged` NEVER projects to `draft` on the load path.
//
// The round-6 arm-3 infeasibility assessment (above) rests on one premise
// about the read path: normalizeCoordinationTaskRecord coerces ONLY an
// unrecoverable `working` to `ready` (and an invalid/absent status to
// `draft`); a VALID stored `staged` projects as `staged` itself. If the
// normalizer's coercion set ever widens — e.g. a `staged` card degraded by
// an unrelated field starts projecting `draft` — the omitted-status
// staged->draft bypass becomes constructible and the watch card demands
// re-assessment. This arm pins the premise against the REAL load path
// (readCoordinationTask -> loadCoordinationTask ->
// normalizeCoordinationTaskRecordWithDiagnostics): two stored-staged
// fixtures — (i) healthy, (ii) degraded by an UNRELATED field (missing
// enums, the round-6 demote fixture shape) — must both project `staged`,
// never `draft`. Green-from-birth by design (the watch card is
// assessment-only; this pin exists to catch coercion-set drift, not to
// red-flag the current slice).
test(
    "watch pin: a valid stored staged card never projects to draft on the load path",
    withIsolatedTransport((root) => {
        // (i) healthy stored-staged card: valid enums + report envelope +
        // DoR lists on the stored body, non-degenerate title from the
        // writeCard base.
        const idHealthy = "dlk-w-staged-healthy";
        writeCard(root, idHealthy, {
            status: "staged",
            ...ENUM_FIX,
            report_envelope: "minimal",
            rough_scope: ["healthy staged projection pin"],
            staged_path: FIXTURE_STAGED_PATH,
            files_in_scope: FILL_FIX.files_in_scope,
            success_criteria: FILL_FIX.success_criteria,
            validation_plan: FILL_FIX.validation_plan,
        });
        const healthy = readCoordinationTask(SESSION_ID, idHealthy);
        assert.equal(
            healthy.diagnostics.degraded,
            false,
            "watch pin (i) premise: the healthy staged fixture must not load degraded",
        );
        assert.equal(
            healthy.task.status,
            "staged",
            "watch pin (i): a healthy stored staged card must project staged",
        );

        // (ii) stored-staged card degraded by an UNRELATED field (missing
        // enums — the round-6 demote fixture shape): degradation must NOT
        // flip the status projection.
        const idDegraded = "dlk-w-staged-degraded";
        writeCard(root, idDegraded, {
            status: "staged",
            rough_scope: ["degraded staged projection pin"],
            files_in_scope: FILL_FIX.files_in_scope,
            success_criteria: FILL_FIX.success_criteria,
            validation_plan: FILL_FIX.validation_plan,
        });
        const degraded = readCoordinationTask(SESSION_ID, idDegraded);
        assert.equal(
            degraded.diagnostics.degraded,
            true,
            "watch pin (ii) premise: the fixture must load degraded (missing enums)",
        );
        assert.equal(
            degraded.task.status,
            "staged",
            "watch pin (ii): an unrelated degraded field must not flip a valid stored staged into a draft projection",
        );
    }),
);

// --- round-2 C-F7: domain-free characterization ------------------------------
//
// templates/core/ is the DOMAIN-FREE embedded corpus (tokens only, no
// project specifics). This grep-based characterization pins that its
// comment blocks carry no REPO-LOCAL file paths — paths that exist only in
// this dev repo (tests/, internal/, cmd/, Makefile, go.mod/go.sum,
// *.test.js) and would therefore leak dev-repo specifics into every
// adopter's rendered tree. Green-from-birth pin (the current corpus is
// clean); it exists to catch future drift, not to red-flag the current
// slice.
test("round-2 C-F7: templates/core comment blocks carry no repo-local file paths", () => {
    const templateFiles = [
        path.join(
            REPO_ROOT,
            "templates",
            "core",
            ".opencode",
            "scripts",
            "state-lib.js",
        ),
        path.join(
            REPO_ROOT,
            "templates",
            "core",
            ".opencode",
            "tools",
            "plan-state.js",
        ),
        // round-5 D-F2: the slice's third round-touched template file —
        // verify-task-registry.js carries case-10g/10i repair-boundary
        // comments and pre-existing suite pointers, all of which must
        // stay generic (extending this arm exposed and fixed two
        // pre-existing "tests/scripts/…" comment mentions).
        path.join(
            REPO_ROOT,
            "templates",
            "core",
            ".opencode",
            "scripts",
            "verify-task-registry.js",
        ),
    ];
    // Dev-repo-only path shapes. Deliberately narrow: generic harness
    // surface (.local/coordinator, .opencode/scripts, .vh-agent-harness,
    // docs/coordination, docs/checkpoints, bin/<binary>) ships in every
    // adopter repo and is NOT repo-local; only artifacts exclusive to this
    // repository's own build/test layout match.
    const repoLocalPathRe =
        /(?:^|[^\w./])(?:tests|internal|cmd)\/[\w./-]+\.(?:js|go|py|md|json|ya?ml|sh|mjs|ts)\b|\bMakefile\b|\bgo\.(?:mod|sum)\b|[\w./-]*\.test\.js\b/;
    for (const file of templateFiles) {
        const src = fs.readFileSync(file, "utf8");
        assert.ok(src.length > 0, `${file} must be readable`);
        const commentLines = src.split("\n").filter((line) => {
            const trimmed = line.trim();
            return (
                trimmed.startsWith("//") ||
                trimmed.startsWith("*") ||
                trimmed.startsWith("/*")
            );
        });
        assert.ok(
            commentLines.length > 0,
            `${file} must contain comment lines to scan`,
        );
        const offenders = commentLines.filter((line) =>
            repoLocalPathRe.test(line),
        );
        assert.deepEqual(
            offenders,
            [],
            `${file}: repo-local file paths must not appear in templates/core comment blocks (domain-free corpus)`,
        );
    }
});
