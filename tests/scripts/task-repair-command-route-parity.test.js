// Regression test for the task-repair command-template route/invocation
// parity defect (2026-11-21): task-repair.md documented
// `restage_coordination_task` (plan-state) as the sanctioned
// ready->staged route for a healthy errata-class card, but the body's
// ONLY plan_state mutation invocation unconditionally called
// `repair_coordination_task` — and repairCoordinationTask refuses a
// healthy non-research card ("does not use the research repair flow"),
// so the documented route dead-ended for exactly the card the template
// routed there. The dispatcher-level behavior (restageCoordinationTask
// guards, plan-state dispatcher parity) is pinned by
// tests/scripts/task-repair-deadlock-restage.test.js; THIS file pins
// the command-template layer: every mutation operation the template
// names as a sanctioned route must also appear as a plan_state
// invocation block, with the argument shape the plan-state dispatcher
// actually reads (restage takes top-level staged_path/note —
// task_payload is ignored for that operation).
//
// Run:  vh-agent-harness exec node --test tests/scripts/task-repair-command-route-parity.test.js
//       (or: node --test tests/scripts/task-repair-command-route-parity.test.js)

import { test } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = path.resolve(__dirname, "..", "..");

// The authoritative corpus source and its rendered twin. The .md command
// template carries no render tokens, so the twin must be byte-identical
// after `make update` (the dev-repo dogfood contract).
const TEMPLATE_SOURCE = path.join(
    REPO_ROOT,
    "templates",
    "core",
    ".opencode",
    "commands",
    "task-repair.md",
);
const RENDERED_TWIN = path.join(
    REPO_ROOT,
    ".opencode",
    "commands",
    "task-repair.md",
);

// A plan_state invocation block is the bullet run that starts at an
// `operation:` bullet and continues through the following bullets at the
// same or deeper indentation — the shape the command templates use for
// "call `plan_state` with:". Returns one entry per invocation, mapping
// each backticked field name of the block to its line.
function invocationBlocks(body) {
    const lines = body.split("\n");
    const blocks = [];
    for (let i = 0; i < lines.length; i++) {
        const m = lines[i].match(/^(\s*)-\s*`operation:\s*([a-z_]+)`\s*$/);
        if (!m) {
            continue;
        }
        const indent = m[1];
        const fields = new Map();
        for (let j = i + 1; j < lines.length; j++) {
            // Field bullets come in two shapes: "- `task_id: $ARGUMENTS`"
            // (name and value inside one backticked segment) and
            // "- `task_payload`: prose" (backticked name, prose outside).
            const fm = lines[j].match(/^(\s*)-\s*`([a-z_]+)[^`]*`/);
            if (!fm || fm[1].length < indent.length) {
                break;
            }
            if (!fields.has(fm[2])) {
                fields.set(fm[2], lines[j]);
            }
        }
        blocks.push({ operation: m[2], fields });
    }
    return blocks;
}

test("task-repair template: every documented route operation has a plan_state invocation block (route/invocation parity)", () => {
    const src = fs.readFileSync(TEMPLATE_SOURCE, "utf8");
    const blocks = invocationBlocks(src);
    const invoked = new Set(blocks.map((b) => b.operation));

    // Premise: the template still documents both mutation routes by name
    // (the restage route was prose-only pre-fix — named in a routing
    // bullet with no invocation block, which is exactly the defect).
    assert.match(
        src,
        /restage_coordination_task/,
        "premise: the template must keep naming the restage route",
    );
    assert.match(
        src,
        /repair_coordination_task/,
        "premise: the template must keep naming the repair routes",
    );

    // Parity: named route => invocation block. The restage assertion is
    // the arm that was RED on the pre-fix template (prose named
    // restage_coordination_task; the only mutation invocation was an
    // unconditional repair_coordination_task call, and the dispatcher
    // refuses healthy non-research cards on that op — the documented
    // ready->staged route dead-ended).
    assert.ok(
        invoked.has("restage_coordination_task"),
        "task-repair.md names restage_coordination_task as a sanctioned route but never invokes it via plan_state — the healthy-errata ready->staged route dead-ends (repair_coordination_task refuses healthy non-research cards)",
    );
    assert.ok(
        invoked.has("repair_coordination_task"),
        "task-repair.md names repair_coordination_task as a sanctioned route but never invokes it via plan_state",
    );
});

test("task-repair template: invocation blocks match the plan-state dispatcher argument contract", () => {
    const src = fs.readFileSync(TEMPLATE_SOURCE, "utf8");
    const blocks = invocationBlocks(src);

    const restage = blocks.find(
        (b) => b.operation === "restage_coordination_task",
    );
    assert.ok(restage, "restage invocation block must exist (parity arm)");
    // The plan-state dispatcher (templates/core/.opencode/tools/plan-state.js)
    // reads task_id / staged_path / note as TOP-LEVEL arguments for
    // restage_coordination_task; task_payload is ignored for that
    // operation, so an invocation routed through task_payload would
    // silently dead-end the same way the prose-only defect did.
    assert.ok(
        restage.fields.has("task_id"),
        "restage invocation must carry task_id",
    );
    assert.ok(
        restage.fields.has("staged_path"),
        "restage invocation must carry staged_path (top-level — the dispatcher does not read it from task_payload)",
    );
    assert.ok(
        !restage.fields.has("task_payload"),
        "restage invocation must NOT carry task_payload (ignored by the dispatcher for restage_coordination_task — a task_payload-shaped invocation is a silent dead route)",
    );

    const repair = blocks.find(
        (b) => b.operation === "repair_coordination_task",
    );
    assert.ok(repair, "repair invocation block must exist (parity arm)");
    assert.ok(
        repair.fields.has("task_id"),
        "repair invocation must carry task_id",
    );
    assert.ok(
        repair.fields.has("task_payload"),
        "repair invocation must carry task_payload (the dispatcher reads the repair fields from task_payload)",
    );
});

test("task-repair template: rendered twin is byte-identical to the templates/core source", () => {
    const src = fs.readFileSync(TEMPLATE_SOURCE, "utf8");
    const twin = fs.readFileSync(RENDERED_TWIN, "utf8");
    // The .md command template is token-free, so a correct `make update`
    // renders it verbatim. Drift here means the shipped twin no longer
    // carries the fixed routing (run `make update`, never a bare
    // `vh-agent-harness update`, in this dev repo).
    assert.equal(
        twin,
        src,
        "rendered .opencode/commands/task-repair.md must be byte-identical to templates/core/.opencode/commands/task-repair.md after make update",
    );
});

test("task-repair template: stays domain-free (no repo-local file paths)", () => {
    const src = fs.readFileSync(TEMPLATE_SOURCE, "utf8");
    // Same dev-repo-only path shapes the deadlock-restage C-F7 arm pins
    // for the JS corpus files; command .md files are not covered by that
    // arm, and this slice edits one.
    const repoLocalPathRe =
        /(?:^|[^\w./])(?:tests|internal|cmd)\/[\w./-]+\.(?:js|go|py|md|json|ya?ml|sh|mjs|ts)\b|\bMakefile\b|\bgo\.(?:mod|sum)\b|[\w./-]*\.test\.js\b/;
    const offenders = src
        .split("\n")
        .filter((line) => repoLocalPathRe.test(line));
    assert.deepEqual(
        offenders,
        [],
        "templates/core/.opencode/commands/task-repair.md must not reference repo-local (dev-repo-only) file paths (domain-free corpus)",
    );
});
