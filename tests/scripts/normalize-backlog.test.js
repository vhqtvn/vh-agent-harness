// Black-box tests for templates/core/.opencode/scripts/normalize-backlog.js.
//
// These tests drive the real CLI entry point on isolated temp backlogs so the
// crux paths (refusal on a bad cell count, escape round-trip, no task-ID shape
// in the rendered archive index) are verified exactly the way an operator or
// promoter hits them (per docs/coordination/PROMOTER_RUNBOOK.md). The source
// functions are not exported, so we spawn the script as a child process and
// assert on its observable behavior.
//
// Run:  vh-agent-harness exec node --test tests/scripts/normalize-backlog.test.js
//       (or: node --test tests/scripts/normalize-backlog.test.js)

import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, writeFileSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, dirname } from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const SCRIPT = join(__dirname, "..", "..", "templates", "core", ".opencode", "scripts", "normalize-backlog.js");

const TABLE_HEADER = "| ID | Status | Area | Task | Owner | Notes | Links |";
const TABLE_DIVIDER = "| --- | --- | --- | --- | --- | --- | --- |";

// Build a minimal but valid backlog whose `Now` section holds the given row
// lines. The normalizer tolerates the other task sections being absent (they
// parse empty and render empty), so we only need a preamble + Now.
function writeFixture(nowRows) {
    const dir = mkdtempSync(join(tmpdir(), "normalize-backlog-test-"));
    const backlogPath = join(dir, "backlog.md");
    const body = [
        "# Test Backlog",
        "",
        "## Area Legend",
        "",
        "| Area | Owner hint | Pattern |",
        "| --- | --- | --- |",
        "| api |  |  |",
        "",
        "## Now",
        "",
        TABLE_HEADER,
        TABLE_DIVIDER,
        ...nowRows,
        "",
    ].join("\n");
    writeFileSync(backlogPath, body, "utf8");
    return { dir, backlogPath };
}

// Run the normalizer on the given backlog path. Returns { status, stdout, stderr }.
function runNormalizer(backlogPath) {
    const result = spawnSync(process.execPath, [SCRIPT, "--backlog", backlogPath], {
        encoding: "utf8",
    });
    return {
        status: result.status,
        stdout: result.stdout,
        stderr: result.stderr,
    };
}

function runCheck(backlogPath) {
    const result = spawnSync(
        process.execPath,
        [SCRIPT, "--backlog", backlogPath, "--check"],
        { encoding: "utf8" },
    );
    return {
        status: result.status,
        stdout: result.stdout,
        stderr: result.stderr,
    };
}

// Split a markdown table row on UNescaped pipes (mirrors the script's
// splitMarkdownRow contract). Returns the trimmed cells without the leading/
// trailing border pipes.
function splitRowOnUnescapedPipes(line) {
    const trimmed = line.trim();
    const inner = trimmed.slice(1, -1);
    const cells = [];
    let current = "";
    for (let i = 0; i < inner.length; i += 1) {
        const ch = inner[i];
        const prev = i > 0 ? inner[i - 1] : "";
        if (ch === "|" && prev !== "\\") {
            cells.push(current.trim());
            current = "";
            continue;
        }
        current += ch;
    }
    cells.push(current.trim());
    return cells;
}

// Find the rendered data row for the given task ID in a backlog body.
// Skips the header and divider. Returns the raw line or undefined.
function findRenderedRow(backlogBody, id) {
    const lines = backlogBody.split("\n");
    for (const line of lines) {
        const trimmed = line.trim();
        if (!trimmed.startsWith("|")) continue;
        const cells = splitRowOnUnescapedPipes(trimmed);
        if (cells[0] === id) return trimmed;
    }
    return undefined;
}

test("refuses an unescaped pipe in the Task column and names the row ID", () => {
    // The Task cell `if (added || upgraded)` has an UNescaped pipe pair. The row
    // splits into 9 cells, so the strict 7-cell contract must refuse it and the
    // row ID must appear in the error so an operator can locate the row.
    const badRow = "| P1-TEST-001 | todo | api | if (added || upgraded) | alice | needs both branches |  |";
    const { backlogPath } = writeFixture([badRow]);
    const result = runNormalizer(backlogPath);

    assert.notEqual(result.status, 0, "normalizer must exit non-zero on a stray pipe");
    const message = `${result.stdout}\n${result.stderr}`;
    assert.match(
        message,
        /P1-TEST-001/,
        "the row ID must be present in the error so the operator can find the row",
    );
    assert.match(
        message,
        /Expected 7 cells but found 9/,
        "the error must name the expected vs actual cell count",
    );
    assert.match(
        message,
        /escape stray pipes as \\\|/,
        "the error must state the remedy (escape stray pipes as \\|)",
    );
});

test("a short row (fewer than seven cells) names the row ID, the count, and the missing-column remedy", () => {
    // A four-cell row is the OTHER direction of the strict 7-cell contract: the
    // problem is a MISSING column, not a stray pipe. The branched remedy must
    // point at supplying the missing column(s), NOT at escaping pipes (the old
    // single remedy misdirected the operator toward pipes for a short row).
    const shortRow = "| P1-TEST-002 | todo | api | settle the missing-column branch |";
    const { backlogPath } = writeFixture([shortRow]);
    const result = runNormalizer(backlogPath);

    assert.notEqual(result.status, 0, "normalizer must exit non-zero on a short row");
    const message = `${result.stdout}\n${result.stderr}`;
    assert.match(
        message,
        /P1-TEST-002/,
        "the row ID must be present in the error so the operator can find the row",
    );
    assert.match(
        message,
        /Expected 7 cells but found 4/,
        "the error must name the expected vs actual cell count",
    );
    assert.match(
        message,
        /supply the missing column\(s\)/,
        "a short row must direct the operator to supply the missing column(s), not to escape pipes",
    );
    // Negative: the pipe-escape remedy must NOT appear for the short direction
    // (this is the misdirection the branch exists to prevent).
    assert.doesNotMatch(
        message,
        /escape stray pipes/,
        "a short row must not suggest escaping pipes — that remedy belongs to the >7 direction only",
    );
});

test("a properly escaped pipe parses to exactly seven cells and round-trips byte-identically", () => {
    // The Task cell now carries an ESCAPED pipe pair: `if (added \|\| upgraded)`.
    // This must parse into exactly seven cells (the `||` survives as data), the
    // cell must round-trip through render byte-identically, and re-running the
    // normalizer on the rendered output must be a no-op.
    const escapedRow = "| P1-TEST-001 | todo | api | if (added \\|\\| upgraded) | alice | needs both branches |  |";
    const { backlogPath } = writeFixture([escapedRow]);

    // First run: must succeed (exactly seven cells under the strict contract).
    const first = runNormalizer(backlogPath);
    assert.equal(first.status, 0, `normalizer must accept an escaped pipe row: ${first.stderr}`);

    const rendered = readFileSync(backlogPath, "utf8");
    const row = findRenderedRow(rendered, "P1-TEST-001");
    assert.ok(row, "rendered backlog must still contain the task row");

    // Exactly seven cells — no truncation, no `" | "` join artifact.
    const cells = splitRowOnUnescapedPipes(row);
    assert.equal(cells.length, 7, "rendered row must split into exactly seven cells");

    // The Task cell (index 3) must round-trip: after un-escaping the rendered
    // bytes, the original `||` data is intact (it was not treated as a separator).
    const taskCellRendered = cells[3];
    const taskCellUnescaped = taskCellRendered.replace(/\\\|/g, "|").trim();
    assert.equal(
        taskCellUnescaped,
        "if (added || upgraded)",
        "the escaped pipe must round-trip as data, not as a cell separator",
    );
    // And the rendered bytes themselves must carry the escaped form (byte-identity
    // of the cell through parse -> sanitizeCell).
    assert.ok(
        taskCellRendered.includes("if (added \\|\\| upgraded)"),
        `rendered Task cell must preserve the escaped form verbatim: ${taskCellRendered}`,
    );

    // Idempotency: re-running on the rendered output is a no-op (byte-stable).
    const secondCheck = runCheck(backlogPath);
    assert.equal(secondCheck.status, 0, `re-render must be a no-op: ${secondCheck.stderr}`);
    assert.match(
        secondCheck.stdout,
        /Pending file updates: none/,
        "a properly escaped row must be byte-stable across re-renders",
    );
});

test("the rendered archive index contains no task-ID-shaped identifier", () => {
    // Defect two shipped a fabricated task ID (`P0-DOCS-006`) into every adopter's
    // generated archive index as retrieval guidance. The rendered archive index
    // must contain no string matching the task-ID shape, regardless of content.
    const escapedRow = "| P1-TEST-001 | todo | api | settle the round-trip row | alice | needs both branches |  |";
    const { dir, backlogPath } = writeFixture([escapedRow]);
    const first = runNormalizer(backlogPath);
    assert.equal(first.status, 0, `fixture must normalize cleanly: ${first.stderr}`);

    // The archive index is always generated, even with zero archived rows.
    const indexPath = join(dir, "archive", "index.md");
    const indexBody = readFileSync(indexPath, "utf8");

    // Generic task-ID shape used across the harness: <PHASE><N>-<AREA>-<NNN>,
    // e.g. P0-DOCS-006, P1-CORE-001, P2-API-003.
    const taskIdShape = /[A-Z]\d+-[A-Z]+-\d+/g;
    const matches = indexBody.match(taskIdShape);
    assert.deepEqual(
        matches,
        null,
        `archive index must contain no task-ID-shaped identifier; found: ${JSON.stringify(matches)}`,
    );

    // Sanity: the retrieval guidance is still present (the fix replaced the
    // literal, it did not delete the section).
    assert.match(indexBody, /## Retrieval/, "the Retrieval section must still be rendered");
});
