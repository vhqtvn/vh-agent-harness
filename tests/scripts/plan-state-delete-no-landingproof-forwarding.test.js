// Anti-regression pin
// (task-2026-08-09t14-40-15-...-pin-plan-state.js-no-forwarding-of-landingproofrepo-landing-gate-anti-regression):
// the plan_state tool's delete_coordination_task case must NOT forward a
// landingProofRepo (or any repo-override) option to deleteCoordinationTask.
// Production plan_state calls must always root the landing-proof verifier at
// repoRoot(); the repo-override (landingProofRepo) is a TEST-ONLY injection
// point that tests exercise by importing state-lib directly (bypassing
// plan-state.js — see task-delete-retirement.test.js). This STATIC pin lives at
// the plan-state.js forwarding layer: a future edit that reintroduces
// arg/env-forwarding of landingProofRepo inside the delete case is caught here,
// BEFORE the no-ambient-override property is silently lost and production
// starts honoring a caller-supplied (or env-supplied) repo override.
//
// The pin is scoped to the delete_coordination_task case block (not the whole
// file) so it targets the precise forwarding layer the card names; a future
// legitimate use of a repo-override option elsewhere does not trip this guard.
import { test } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = path.resolve(__dirname, "..", "..");
// Pin the SOURCE copy (templates/core/): a developer reintroducing forwarding
// edits the source, and the rendered .opencode/tools/plan-state.js is
// regenerated from it by `make update`.
const PLAN_STATE_SRC = path.join(
    REPO_ROOT,
    "templates",
    "core",
    ".opencode",
    "tools",
    "plan-state.js",
);

test("plan-state.js delete_coordination_task forwards NO landingProofRepo (no-ambient-override pin)", () => {
    const src = fs.readFileSync(PLAN_STATE_SRC, "utf8");
    assert.ok(src.length > 0, "plan-state.js source must be readable");

    // Locate the delete_coordination_task case label.
    const label = 'case "delete_coordination_task":';
    const caseStart = src.indexOf(label);
    assert.notEqual(
        caseStart,
        -1,
        "delete_coordination_task case not found in plan-state.js source",
    );

    // The delete case ends at the next sibling `case "..."` label. There is no
    // nested switch inside this case, so the first `case "` after the delete
    // label is the next sibling case (today: save_skill_proposal).
    const nextCase = src.indexOf('case "', caseStart + label.length);
    const blockEnd = nextCase === -1 ? src.length : nextCase;
    const deleteBlock = src.slice(caseStart, blockEnd);

    assert.equal(
        deleteBlock.includes("landingProofRepo"),
        false,
        "plan-state.js delete_coordination_task case must NOT forward landingProofRepo; " +
            "production must always root the landing-proof verifier at repoRoot() " +
            "(the repo-override is a test-only injection point, never a forwarded option)",
    );
});
