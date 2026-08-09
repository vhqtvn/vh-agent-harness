// End-to-end coverage for the rewrite-parity Stage-2 closeout gate.
//
// The Stage-2 gate (rewrite-parity-validate.js validateRewriteParityCompletion)
// is WIRED into saveCoordinationTaskCloseout (state-lib.js): when
// task_status=completed AND the closeout body carries a fenced
// ```rewrite-parity block, every behavior must be proven with a tree-bound
// receipt (structural completeness). The validator-in-isolation is covered by
// rewrite-parity-validate.test.js (it drives the validator directly and also
// SIMULATES the closeout pipeline by calling extractAllRewriteParityBlocks +
// validateRewriteParityCompletion in the test itself). THIS suite drives the
// REAL saveCoordinationTaskCloseout (imported from the rendered
// .opencode/scripts/state-lib.js) end-to-end and asserts the wiring-through-
// closeout path — i.e. that the gate actually fires inside the closeout
// transition, not just that the validator works when a test calls it.
//
// Test seam (mirrors tests/scripts/task-delete-retirement.test.js): import
// state-lib from the RENDERED tree (.opencode/scripts/), redirect the
// coordinator transport to an isolated tmp dir via
// OPENCODE_LOCAL_COORDINATOR_ROOT, bind a session alias, write a complete
// valid `working` card directly, then call saveCoordinationTaskCloseout with
// taskStatus=completed and observe the gate. The rewrite-parity contracts
// come from the golden fixture set (tests/fixtures/rewrite-parity/) so this
// suite stays coupled to the same contracts rewrite-parity-validate.test.js
// pins.
//
// The distinguishing assertion is the wiring-only error prefix that
// saveCoordinationTaskCloseout emits around validator errors
// ("rewrite-parity contract #N failed completion validation"). That string is
// produced ONLY by the closeout wiring (state-lib.js), never by the validator
// in isolation, so matching it proves the error transited the real closeout
// path.
//
// Crux (behavioral-closure): the Stage-2 wiring-through-closeout path —
//   valid all-proven block + completed          -> closeout SUCCEEDS;
//   non-proven (planned/skipped/not-demonstrable) or proven-without-receipt
//                                                -> closeout REFUSED (StateError
//                                                   from the closeout wiring);
//   no block + completed                        -> no Stage-2 fire (backward-
//                                                   compat SUCCEEDS);
//   invalid block + reported                    -> SUCCEEDS (the gate is
//                                                   completion-gated; it does
//                                                   not fire on reported/blocked).
//
// Run:  vh-agent-harness exec node --test tests/scripts/closeout-stage2-rewrite-parity.test.js
//       (or: node --test tests/scripts/closeout-stage2-rewrite-parity.test.js)

import { test } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import os from "node:os";
import { readFileSync } from "node:fs";
import { fileURLToPath, pathToFileURL } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = path.resolve(__dirname, "..", "..");
const FIXTURES = path.join(REPO_ROOT, "tests", "fixtures", "rewrite-parity");

// Import state-lib from the RENDERED tree (.opencode/scripts/). The source
// copy under templates/core/ still carries the literal {{COORDINATOR_DIR}}
// token and REFUSES to load. Run `make update` after editing templates/core/
// before this test (the normal dev flow).
const STATE_LIB = path.join(REPO_ROOT, ".opencode", "scripts", "state-lib.js");
const {
    StateError,
    bindSessionName,
    saveCoordinationTaskCloseout,
} = await import(pathToFileURL(STATE_LIB).href);

// --- fixtures --------------------------------------------------------------

function loadFixture(name) {
    return JSON.parse(readFileSync(path.join(FIXTURES, name), "utf-8"));
}

// Wrap a contract object as the fenced ```rewrite-parity markdown body the
// closeout pipeline extracts from.
function rewriteParityBody(contract) {
    return (
        "# Closeout\n\nDeclared rewrite-parity contract:\n\n```rewrite-parity\n" +
        JSON.stringify(contract, null, 2) +
        "\n```\n"
    );
}

// A minimal structurally-valid contract whose single behavior is `skipped`.
// There is no golden skipped fixture (not-demonstrable, planned, and
// no-receipt all have one), so inline this one to cover the remaining
// non-proven result enum value end-to-end through the closeout wiring.
function skippedContract() {
    return {
        version: 1,
        applies: "completion with skipped (must block)",
        mode: "deletion_replacement",
        prior_surface: {
            id: "x",
            revision: "deadbeef",
            paths: ["a"],
            inventory_complete: true,
        },
        behaviors: [
            {
                id: "b",
                description: "d",
                prior_evidence: ["e"],
                verifier: { kind: "go-test", locator: "go test" },
                result: { status: "skipped" },
            },
        ],
    };
}

// A minimal structurally-valid contract whose single behavior is `failed`.
// `failed` is the last non-proven result-status enum value not yet pinned
// through the closeout wiring (planned/skipped/not-demonstrable/no-receipt
// already are). It shares the same non-proven refusal branch: completion
// requires every behavior proven, so `failed` blocks completion just like the
// others. Inlined (not a golden fixture) to mirror skippedContract() exactly.
function failedContract() {
    return {
        version: 1,
        applies: "completion with failed (must block)",
        mode: "deletion_replacement",
        prior_surface: {
            id: "x",
            revision: "deadbeef",
            paths: ["a"],
            inventory_complete: true,
        },
        behaviors: [
            {
                id: "b",
                description: "d",
                prior_evidence: ["e"],
                verifier: { kind: "go-test", locator: "go test" },
                result: { status: "failed" },
            },
        ],
    };
}

// The wiring-only prefix saveCoordinationTaskCloseout emits around each
// validator error. Matching this substring proves the error transited the
// closeout wiring (the validator-in-isolation suite cannot produce it).
const WIRING_PREFIX =
    "rewrite-parity contract #1 failed completion validation";

// --- isolated coordinator transport helpers --------------------------------
// Mirrors tests/scripts/task-delete-retirement.test.js: redirect the
// coordinator transport to a tmp dir via OPENCODE_LOCAL_COORDINATOR_ROOT,
// bind a session alias, and write a complete valid `working` card directly so
// saveCoordinationTaskCloseout passes its preconditions (status=working AND
// the active owner matches the actor session). The card carries every strict
// core field (title/task_type/coordination_mode/primary_lane/report_envelope/
// files_in_scope/success_criteria/validation_plan + working-specific
// active_session_alias+claimed_at) so the closeout's write-back
// (updateCoordinationTask -> ensureCoordinationTaskCoreFields) passes too.

function cardPath(isoRoot, id) {
    return path.join(isoRoot, "tasks", `${id}.json`);
}
function reportDir(isoRoot, id) {
    return path.join(isoRoot, "reports", id);
}
function writeWorkingCard(isoRoot, id, alias) {
    const card = {
        schema_version: 1,
        task_id: id,
        title: `Stage-2 e2e fixture ${id}`,
        task_type: "implementation",
        coordination_mode: "short",
        primary_lane: "substrate",
        report_envelope: "minimal",
        status: "working",
        active_session_alias: alias,
        claimed_at: "2026-08-09T00:00:00Z",
        files_in_scope: [
            "tests/scripts/closeout-stage2-rewrite-parity.test.js",
        ],
        success_criteria: [
            "Stage-2 gate fires through the real saveCoordinationTaskCloseout.",
        ],
        validation_plan: [
            "node --test tests/scripts/closeout-stage2-rewrite-parity.test.js",
        ],
        session_aliases: [alias],
    };
    fs.mkdirSync(path.dirname(cardPath(isoRoot, id)), { recursive: true });
    fs.writeFileSync(cardPath(isoRoot, id), JSON.stringify(card, null, 2));
}
function cardStatus(isoRoot, id) {
    const p = cardPath(isoRoot, id);
    if (!fs.existsSync(p)) return null;
    return JSON.parse(fs.readFileSync(p, "utf-8")).status;
}

const SESSION_ID = "rp-stage2-e2e-session";
const ALIAS = "rp-stage2-e2e";

function runCloseout(id, taskStatus, body) {
    return saveCoordinationTaskCloseout(SESSION_ID, id, {
        cwd: "/verification",
        title: "Stage-2 e2e",
        body,
        taskStatus,
        reportEnvelope: "minimal",
    });
}

// Run a closeout expected to refuse. Asserts the throw is a StateError, that
// its message carries the wiring-only prefix (proving the error transited the
// real closeout path), and that it names each expected validator substring.
function expectCloseoutRefusal(id, taskStatus, body, ...needles) {
    let thrown = null;
    try {
        runCloseout(id, taskStatus, body);
    } catch (error) {
        thrown = error;
    }
    assert.ok(
        thrown,
        `closeout ${id} should have refused but did not throw`,
    );
    assert.ok(
        thrown && thrown.name === "StateError",
        `expected StateError for ${id}, got ${thrown && thrown.constructor.name}`,
    );
    const msg = String((thrown && thrown.message) || "");
    assert.ok(
        msg.includes(WIRING_PREFIX),
        `refusal for ${id} should transit the closeout wiring (carry "${WIRING_PREFIX}"); got: ${msg}`,
    );
    for (const needle of needles) {
        assert.ok(
            msg.includes(needle),
            `refusal for ${id} should include "${needle}"; got: ${msg}`,
        );
    }
}

test("rewrite-parity Stage-2 gate fires through the real saveCoordinationTaskCloseout", () => {
    const isoRoot = fs.mkdtempSync(path.join(os.tmpdir(), "rp-stage2-e2e-"));
    const prevCoordRoot = process.env.OPENCODE_LOCAL_COORDINATOR_ROOT;
    process.env.OPENCODE_LOCAL_COORDINATOR_ROOT = isoRoot;
    // bindSessionName writes the session binding to the real .opencode/state/
    // (gitignored transport) — same harmless, accepted pattern as
    // tests/scripts/task-delete-retirement.test.js and verify-task-registry.js.
    bindSessionName(SESSION_ID, ALIAS);

    const validCompletion = loadFixture("valid-completion.json");
    const validPlanned = loadFixture("valid-planned.json");
    const notDemonstrable = loadFixture(
        "invalid-completion-not-demonstrable.json",
    );
    const noReceipt = loadFixture("invalid-completion-no-receipt.json");

    try {
        // (a) completed + valid all-proven block -> SUCCEEDS. The closeout
        //     transitions the card working -> completed, writes a report, and
        //     the Stage-2 gate does NOT refuse. This is the clean pass path.
        writeWorkingCard(isoRoot, "rp-a-valid", ALIAS);
        const ok = runCloseout(
            "rp-a-valid",
            "completed",
            rewriteParityBody(validCompletion),
        );
        assert.equal(
            ok.task.status,
            "completed",
            "(a) closeout should move the task to completed",
        );
        assert.equal(
            cardStatus(isoRoot, "rp-a-valid"),
            "completed",
            "(a) card should persist as completed",
        );
        const aReports = fs.existsSync(reportDir(isoRoot, "rp-a-valid"))
            ? fs.readdirSync(reportDir(isoRoot, "rp-a-valid"))
            : [];
        assert.ok(
            aReports.some((f) => /closeout\.md$/.test(f)),
            "(a) closeout should write a report to the isolated report dir",
        );

        // (b1) completed + not-demonstrable behavior -> REFUSED. The gate
        //      requires every behavior proven; not-demonstrable blocks
        //      completion and routes to defer.
        writeWorkingCard(isoRoot, "rp-b-nd", ALIAS);
        expectCloseoutRefusal(
            "rp-b-nd",
            "completed",
            rewriteParityBody(notDemonstrable),
            "not-demonstrable",
        );
        assert.equal(
            cardStatus(isoRoot, "rp-b-nd"),
            "working",
            "(b1) card must remain working after a refused closeout",
        );

        // (b2) completed + planned behavior -> REFUSED. A planned-stage
        //      contract declares no proven behavior, so it cannot satisfy the
        //      completion requirement.
        writeWorkingCard(isoRoot, "rp-b-planned", ALIAS);
        expectCloseoutRefusal(
            "rp-b-planned",
            "completed",
            rewriteParityBody(validPlanned),
            "planned",
        );
        assert.equal(
            cardStatus(isoRoot, "rp-b-planned"),
            "working",
            "(b2) card must remain working after a refused closeout",
        );

        // (b3) completed + skipped behavior -> REFUSED (inline contract; no
        //      golden skipped fixture exists). Covers the remaining non-proven
        //      result enum value through the closeout wiring.
        writeWorkingCard(isoRoot, "rp-b-skipped", ALIAS);
        expectCloseoutRefusal(
            "rp-b-skipped",
            "completed",
            rewriteParityBody(skippedContract()),
            "skipped",
        );
        assert.equal(
            cardStatus(isoRoot, "rp-b-skipped"),
            "working",
            "(b3) card must remain working after a refused closeout",
        );

        // (b4) completed + proven-without-receipt -> REFUSED. Proven requires
        //      a non-empty receipt locator; its absence blocks completion.
        writeWorkingCard(isoRoot, "rp-b-noreceipt", ALIAS);
        expectCloseoutRefusal(
            "rp-b-noreceipt",
            "completed",
            rewriteParityBody(noReceipt),
            "receipt",
        );
        assert.equal(
            cardStatus(isoRoot, "rp-b-noreceipt"),
            "working",
            "(b4) card must remain working after a refused closeout",
        );

        // (b5) completed + failed behavior -> REFUSED (inline contract; no
        //      golden failed fixture exists). `failed` is the last non-proven
        //      result-status enum value; it shares the same refusal branch as
        //      planned/skipped/not-demonstrable (completion requires every
        //      behavior proven). The needle is deliberately the validator-
        //      specific `result.status is "failed"` (double-quoted, the form
        //      JSON.stringify produces in the JS validator's message) rather
        //      than the bare word "failed": "failed" appears in BOTH the
        //      WIRING_PREFIX ("rewrite-parity contract #1 failed completion
        //      validation") AND the enum list
        //      ("planned/failed/skipped/not-demonstrable"), so the bare word
        //      would match even if the status string itself never reached the
        //      message. The `result.status is "failed"` substring is present
        //      ONLY in the per-behavior validator line and so pins that the
        //      failed-status enum value — not just the generic refusal path —
        //      transited the closeout wiring.
        writeWorkingCard(isoRoot, "rp-b-failed", ALIAS);
        expectCloseoutRefusal(
            "rp-b-failed",
            "completed",
            rewriteParityBody(failedContract()),
            'result.status is "failed"',
        );
        assert.equal(
            cardStatus(isoRoot, "rp-b-failed"),
            "working",
            "(b5) card must remain working after a refused closeout",
        );

        // (c) completed + NO rewrite-parity block -> SUCCEEDS (no Stage-2
        //     fire). The gate is opt-in: ordinary closeouts carry no
        //     ```rewrite-parity block and bear zero rewrite-parity burden.
        //     This is the backward-compat guarantee for every task that does
        //     not declare a deletion/rewrite contract.
        writeWorkingCard(isoRoot, "rp-c-noblock", ALIAS);
        const okNoBlock = runCloseout(
            "rp-c-noblock",
            "completed",
            "# Ordinary closeout\n\nNo rewrite-parity contract declared here.\n",
        );
        assert.equal(
            okNoBlock.task.status,
            "completed",
            "(c) ordinary completed closeout should succeed (no Stage-2 fire)",
        );

        // (d) reported + INVALID rewrite-parity block -> SUCCEEDS. The gate is
        //     completion-gated: it fires ONLY for taskStatus=completed. A
        //     reported closeout carries an invalid block through unchallenged
        //     (the gate does not fire on reported/blocked). This pins the
        //     opt-in semantic and guards against an over-broad future wiring
        //     that would challenge reported/blocked closeouts too.
        writeWorkingCard(isoRoot, "rp-d-reported", ALIAS);
        const okReported = runCloseout(
            "rp-d-reported",
            "reported",
            rewriteParityBody(notDemonstrable),
        );
        assert.equal(
            okReported.task.status,
            "reported",
            "(d) gate is completion-gated; an invalid block must NOT block a reported closeout",
        );
    } finally {
        // Wholesale teardown: env-var restore + remove the isolated transport
        // so nothing leaks into the real coordinator registry. (A gitignored
        // session binding under .opencode/state/ from bindSessionName may
        // persist — same harmless pattern as task-delete-retirement.test.js.)
        if (prevCoordRoot === undefined) {
            delete process.env.OPENCODE_LOCAL_COORDINATOR_ROOT;
        } else {
            process.env.OPENCODE_LOCAL_COORDINATOR_ROOT = prevCoordRoot;
        }
        fs.rmSync(isoRoot, { recursive: true, force: true });
    }
});
