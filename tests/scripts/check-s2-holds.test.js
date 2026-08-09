// Unit tests for the G6 deterministic S2-hold evaluator
// (templates/core/.opencode/scripts/check-s2-holds.mjs).
//
// Each test builds a scratch git repo, commits backlog rows (active + archive)
// and evidence packets, then imports `evaluate(scratchRoot)` and asserts the
// classification + records. A small set of CLI invocations verify the
// exit-taxonomy mapping (clear→0, blocker→1, evaluator-error→2).
//
// Run:  vh-agent-harness exec node --test tests/scripts/check-s2-holds.test.js
//       (or: node --test tests/scripts/check-s2-holds.test.js)

import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, writeFileSync, rmSync, copyFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, dirname } from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath, pathToFileURL } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const SCRIPT = join(__dirname, "..", "..", "templates", "core", ".opencode", "scripts", "check-s2-holds.mjs");
const SCRIPT_URL = pathToFileURL(SCRIPT).href;
const { evaluate } = await import(SCRIPT_URL);

// --- scratch-repo helpers ---------------------------------------------------

function git(repo, ...args) {
    const r = spawnSync("git", ["-c", "init.defaultBranch=main", ...args], {
        cwd: repo,
        env: { ...process.env, GIT_CONFIG_GLOBAL: "/dev/null", GIT_CONFIG_NOSYSTEM: "1" },
    });
    if (r.status !== 0) {
        throw new Error(`git ${args.join(" ")} failed in ${repo}: ${r.stderr && r.stderr.toString()} ${r.stdout && r.stdout.toString()}`);
    }
    return (r.stdout && r.stdout.toString().trim()) || "";
}

function makeRepo() {
    const dir = mkdtempSync(join(tmpdir(), "s2-eval-"));
    git(dir, "init");
    git(dir, "config", "user.email", "t@t");
    git(dir, "config", "user.name", "t");
    return dir;
}

function commitFile(repo, relPath, content, msg = "init") {
    const abs = join(repo, relPath);
    mkdirSync(dirname(abs), { recursive: true });
    writeFileSync(abs, content);
    git(repo, "add", relPath);
    git(repo, "commit", "-m", msg);
}

function rowMarkdown(r) {
    const notes = r.notes !== undefined ? r.notes : `s2-hold: ${r.id}`;
    const links = r.links !== undefined ? r.links : (r.packet ? `researches/sources/${r.packet}` : "");
    const task = r.task !== undefined ? r.task : `${r.id} task`;
    return `| ${r.id} | ${r.status} | area | ${task} | owner | ${notes} | ${links} |`;
}

function writeBacklog(repo, rows, opts = {}) {
    const lines = [
        "# Backlog",
        "",
        "## Now",
        "",
        "| ID | Status | Area | Task | Owner | Notes | Links |",
        "| --- | --- | --- | --- | --- | --- | --- |",
    ];
    for (const r of rows) lines.push(rowMarkdown(r));
    commitFile(repo, "docs/planning/backlog.md", `${lines.join("\n")}\n`);
    if (opts.archiveRows && opts.archiveRows.length) {
        const af = opts.archiveFile || "backlog-archive-2026-q3.md";
        const alines = [
            "# Archive",
            "",
            "| ID | Status | Area | Task | Owner | Notes | Links |",
            "| --- | --- | --- | --- | --- | --- | --- |",
        ];
        for (const r of opts.archiveRows) alines.push(rowMarkdown(r));
        commitFile(repo, `docs/planning/archive/${af}`, `${alines.join("\n")}\n`);
    }
}

function recordText(rec) {
    const lines = [`### S2 hold: ${rec.id}`];
    if (rec.verdict !== undefined) lines.push(`- Verdict: ${rec.verdict}`);
    if (rec.skill !== undefined) lines.push(`- Skill: ${rec.skill}`);
    if (rec.pilot !== undefined) lines.push(`- Pilot: ${rec.pilot}`);
    return lines.join("\n");
}

function writePacket(repo, name, records) {
    const body = `# Packet\n\n${records.map(recordText).join("\n\n")}\n`;
    commitFile(repo, `researches/sources/${name}`, body);
}

function dispose(repo) {
    try { rmSync(repo, { recursive: true, force: true }); } catch (_) { /* best-effort */ }
}

// --- clear cases ------------------------------------------------------------

test("clear: no holds (empty backlog)", () => {
    const repo = makeRepo();
    try {
        writeBacklog(repo, []);
        const r = evaluate(repo);
        assert.equal(r.classification, "clear");
        assert.deepEqual(r.hold_ids, []);
        assert.deepEqual(r.blocking_ids, []);
        assert.equal(r.error, null);
    } finally { dispose(repo); }
});

test("clear: no backlog file at all", () => {
    const repo = makeRepo();
    try {
        commitFile(repo, "README.md", "# scratch\n");
        const r = evaluate(repo);
        assert.equal(r.classification, "clear");
        assert.deepEqual(r.hold_ids, []);
    } finally { dispose(repo); }
});

test("clear: one uniquely-joined SATISFIED (done)", () => {
    const repo = makeRepo();
    try {
        writeBacklog(repo, [{ id: "S2-x-001", status: "done", packet: "p1.md" }]);
        writePacket(repo, "p1.md", [{ id: "S2-x-001", verdict: "SATISFIED", skill: "x", pilot: "scratch (retrospective)" }]);
        const r = evaluate(repo);
        assert.equal(r.classification, "clear");
        assert.deepEqual(r.hold_ids, ["S2-x-001"]);
        assert.deepEqual(r.blocking_ids, []);
    } finally { dispose(repo); }
});

test("clear: multiple SATISFIED holds", () => {
    const repo = makeRepo();
    try {
        writeBacklog(repo, [
            { id: "S2-alpha-001", status: "done", packet: "p1.md" },
            { id: "S2-beta-002", status: "done", packet: "p1.md" },
        ]);
        writePacket(repo, "p1.md", [
            { id: "S2-alpha-001", verdict: "SATISFIED", skill: "alpha", pilot: "scratch (retrospective)" },
            { id: "S2-beta-002", verdict: "SATISFIED", skill: "beta", pilot: "scratch (forward)" },
        ]);
        const r = evaluate(repo);
        assert.equal(r.classification, "clear");
        assert.deepEqual(r.hold_ids, ["S2-alpha-001", "S2-beta-002"]);
    } finally { dispose(repo); }
});

test("clear: archived-but-discoverable hold (cancelled+WITHDRAWN in archive)", () => {
    const repo = makeRepo();
    try {
        writeBacklog(repo, [], {
            archiveRows: [{ id: "S2-gamma-003", status: "cancelled", packet: "p2.md" }],
            archiveFile: "backlog-archive-2026-q3.md",
        });
        writePacket(repo, "p2.md", [{ id: "S2-gamma-003", verdict: "WITHDRAWN", skill: "gamma" }]);
        const r = evaluate(repo);
        assert.equal(r.classification, "clear");
        assert.deepEqual(r.hold_ids, ["S2-gamma-003"]);
        const rec = r.records.find((x) => x.hold_id === "S2-gamma-003");
        assert.equal(rec.backlog.universe, "archive");
    } finally { dispose(repo); }
});

// --- refusal: blocker (structurally valid, legitimate no-release) -----------

test("blocker: PENDING hold (active + PENDING)", () => {
    const repo = makeRepo();
    try {
        writeBacklog(repo, [{ id: "S2-x-001", status: "in_progress", packet: "p1.md" }]);
        writePacket(repo, "p1.md", [{ id: "S2-x-001", verdict: "PENDING", skill: "x" }]);
        const r = evaluate(repo);
        assert.equal(r.classification, "blocker");
        assert.deepEqual(r.blocking_ids, ["S2-x-001"]);
    } finally { dispose(repo); }
});

test("blocker: disagreement — done backlog + PENDING evidence", () => {
    const repo = makeRepo();
    try {
        writeBacklog(repo, [{ id: "S2-x-001", status: "done", packet: "p1.md" }]);
        writePacket(repo, "p1.md", [{ id: "S2-x-001", verdict: "PENDING", skill: "x" }]);
        const r = evaluate(repo);
        assert.equal(r.classification, "blocker");
        assert.deepEqual(r.blocking_ids, ["S2-x-001"]);
    } finally { dispose(repo); }
});

test("blocker: disagreement — active backlog + SATISFIED evidence", () => {
    const repo = makeRepo();
    try {
        writeBacklog(repo, [{ id: "S2-x-001", status: "todo", packet: "p1.md" }]);
        writePacket(repo, "p1.md", [{ id: "S2-x-001", verdict: "SATISFIED", skill: "x", pilot: "scratch (retrospective)" }]);
        const r = evaluate(repo);
        assert.equal(r.classification, "blocker");
        assert.deepEqual(r.blocking_ids, ["S2-x-001"]);
    } finally { dispose(repo); }
});

// --- refusal: evaluator-error (structurally invalid) ------------------------

test("evaluator-error: missing evidence record", () => {
    const repo = makeRepo();
    try {
        writeBacklog(repo, [{ id: "S2-x-001", status: "done", packet: "p1.md" }]);
        // no evidence packet committed
        const r = evaluate(repo);
        assert.equal(r.classification, "evaluator-error");
    } finally { dispose(repo); }
});

test("evaluator-error: malformed — missing Verdict", () => {
    const repo = makeRepo();
    try {
        writeBacklog(repo, [{ id: "S2-x-001", status: "done", packet: "p1.md" }]);
        writePacket(repo, "p1.md", [{ id: "S2-x-001", skill: "x" }]); // no verdict
        const r = evaluate(repo);
        assert.equal(r.classification, "evaluator-error");
    } finally { dispose(repo); }
});

test("evaluator-error: unknown verdict", () => {
    const repo = makeRepo();
    try {
        writeBacklog(repo, [{ id: "S2-x-001", status: "done", packet: "p1.md" }]);
        writePacket(repo, "p1.md", [{ id: "S2-x-001", verdict: "MAYBE", skill: "x" }]);
        // The VERDICT_RE regex won't capture MAYBE → verdict stays null → missing-Verdict error
        const r = evaluate(repo);
        assert.equal(r.classification, "evaluator-error");
    } finally { dispose(repo); }
});

test("evaluator-error: SATISFIED missing required Pilot", () => {
    const repo = makeRepo();
    try {
        writeBacklog(repo, [{ id: "S2-x-001", status: "done", packet: "p1.md" }]);
        writePacket(repo, "p1.md", [{ id: "S2-x-001", verdict: "SATISFIED", skill: "x" }]); // no pilot
        const r = evaluate(repo);
        assert.equal(r.classification, "evaluator-error");
    } finally { dispose(repo); }
});

test("evaluator-error: SATISFIED malformed Pilot", () => {
    const repo = makeRepo();
    try {
        writeBacklog(repo, [{ id: "S2-x-001", status: "done", packet: "p1.md" }]);
        writePacket(repo, "p1.md", [{ id: "S2-x-001", verdict: "SATISFIED", skill: "x", pilot: "just-a-repo" }]);
        const r = evaluate(repo);
        assert.equal(r.classification, "evaluator-error");
    } finally { dispose(repo); }
});

test("evaluator-error: missing Skill", () => {
    const repo = makeRepo();
    try {
        writeBacklog(repo, [{ id: "S2-x-001", status: "done", packet: "p1.md" }]);
        writePacket(repo, "p1.md", [{ id: "S2-x-001", verdict: "PENDING" }]);
        const r = evaluate(repo);
        assert.equal(r.classification, "evaluator-error");
    } finally { dispose(repo); }
});

test("evaluator-error: duplicate backlog rows (same id)", () => {
    const repo = makeRepo();
    try {
        writeBacklog(repo, [
            { id: "S2-x-001", status: "done", packet: "p1.md" },
            { id: "S2-x-001", status: "done", packet: "p1.md" },
        ]);
        writePacket(repo, "p1.md", [{ id: "S2-x-001", verdict: "SATISFIED", skill: "x", pilot: "scratch (retrospective)" }]);
        const r = evaluate(repo);
        assert.equal(r.classification, "evaluator-error");
    } finally { dispose(repo); }
});

test("evaluator-error: duplicate evidence records (same id, two packets)", () => {
    const repo = makeRepo();
    try {
        writeBacklog(repo, [{ id: "S2-x-001", status: "done", packet: "p1.md" }]);
        writePacket(repo, "p1.md", [{ id: "S2-x-001", verdict: "SATISFIED", skill: "x", pilot: "scratch (retrospective)" }]);
        writePacket(repo, "p2.md", [{ id: "S2-x-001", verdict: "SATISFIED", skill: "x", pilot: "scratch (retrospective)" }]);
        const r = evaluate(repo);
        assert.equal(r.classification, "evaluator-error");
    } finally { dispose(repo); }
});

test("evaluator-error: ambiguous join — evidence in wrong packet vs cited Links", () => {
    const repo = makeRepo();
    try {
        // Links cites p1.md but the record is in p2.md
        writeBacklog(repo, [{ id: "S2-x-001", status: "done", links: "researches/sources/p1.md" }]);
        writePacket(repo, "p2.md", [{ id: "S2-x-001", verdict: "SATISFIED", skill: "x", pilot: "scratch (retrospective)" }]);
        const r = evaluate(repo);
        assert.equal(r.classification, "evaluator-error");
        const rec = r.records.find((x) => x.hold_id === "S2-x-001");
        assert.match(rec.reason, /cites researches\/sources\/p1\.md/);
    } finally { dispose(repo); }
});

test("evaluator-error: cancelled + s2-hold unresolved (cancelled + PENDING)", () => {
    const repo = makeRepo();
    try {
        writeBacklog(repo, [{ id: "S2-x-001", status: "cancelled", packet: "p1.md" }]);
        writePacket(repo, "p1.md", [{ id: "S2-x-001", verdict: "PENDING", skill: "x" }]);
        const r = evaluate(repo);
        assert.equal(r.classification, "evaluator-error");
    } finally { dispose(repo); }
});

test("evaluator-error: cancelled + s2-hold unresolved (cancelled + SATISFIED)", () => {
    const repo = makeRepo();
    try {
        writeBacklog(repo, [{ id: "S2-x-001", status: "cancelled", packet: "p1.md" }]);
        writePacket(repo, "p1.md", [{ id: "S2-x-001", verdict: "SATISFIED", skill: "x", pilot: "scratch (retrospective)" }]);
        const r = evaluate(repo);
        assert.equal(r.classification, "evaluator-error");
    } finally { dispose(repo); }
});

test("evaluator-error: backlog token malformed (bad id shape)", () => {
    const repo = makeRepo();
    try {
        // `s2-hold: foo` — the /s2-hold:/ test passes but HOLD_TOKEN_RE doesn't match
        writeBacklog(repo, [{ id: "P1-X-001", status: "done", notes: "s2-hold: foo" }]);
        const r = evaluate(repo);
        assert.equal(r.classification, "evaluator-error");
    } finally { dispose(repo); }
});

test("evaluator-error: backlog row missing Links evidence path", () => {
    const repo = makeRepo();
    try {
        writeBacklog(repo, [{ id: "S2-x-001", status: "done", links: "" }]);
        const r = evaluate(repo);
        assert.equal(r.classification, "evaluator-error");
        const rec = r.records.find((x) => x.hold_id === "S2-x-001");
        assert.match(rec.reason, /no evidence-packet reference in Links/);
    } finally { dispose(repo); }
});

test("evaluator-error: unreadable committed packet (cited path absent from HEAD)", () => {
    const repo = makeRepo();
    try {
        // Cite a packet path, but never commit it. gitShowHeadBlob returns null.
        writeBacklog(repo, [{ id: "S2-x-001", status: "done", links: "researches/sources/missing.md" }]);
        const r = evaluate(repo);
        assert.equal(r.classification, "evaluator-error");
    } finally { dispose(repo); }
});

test("evaluator-error: Skill field does not match the held skill derived from the ID", () => {
    const repo = makeRepo();
    try {
        // Hold ID encodes skill 'alpha', but the evidence Skill says 'beta'.
        writeBacklog(repo, [{ id: "S2-alpha-001", status: "done", packet: "p1.md" }]);
        writePacket(repo, "p1.md", [{ id: "S2-alpha-001", verdict: "SATISFIED", skill: "beta", pilot: "scratch (retrospective)" }]);
        const r = evaluate(repo);
        assert.equal(r.classification, "evaluator-error");
        const rec = r.records.find((x) => x.hold_id === "S2-alpha-001");
        assert.match(rec.reason, /does not match the held skill/);
    } finally { dispose(repo); }
});

test("evaluator-error: duplicate Verdict field in one record (single-verdict invariant)", () => {
    const repo = makeRepo();
    try {
        // Two -Verdict: lines (PENDING then SATISFIED) — last-wins would clear;
        // the invariant must refuse.
        writeBacklog(repo, [{ id: "S2-x-001", status: "done", packet: "p1.md" }]);
        // Hand-build a packet with duplicate Verdict lines.
        const body = "# Packet\n\n### S2 hold: S2-x-001\n- Verdict: PENDING\n- Verdict: SATISFIED\n- Skill: x\n- Pilot: scratch (retrospective)\n";
        commitFile(repo, "researches/sources/p1.md", body);
        const r = evaluate(repo);
        assert.equal(r.classification, "evaluator-error");
        const rec = r.records.find((x) => x.hold_id === "S2-x-001");
        assert.match(rec.reason, /duplicate field/);
    } finally { dispose(repo); }
});

test("evaluator-error: duplicate Skill field in one record", () => {
    const repo = makeRepo();
    try {
        writeBacklog(repo, [{ id: "S2-x-001", status: "done", packet: "p1.md" }]);
        const body = "# Packet\n\n### S2 hold: S2-x-001\n- Verdict: SATISFIED\n- Skill: x\n- Skill: x\n- Pilot: scratch (retrospective)\n";
        commitFile(repo, "researches/sources/p1.md", body);
        const r = evaluate(repo);
        assert.equal(r.classification, "evaluator-error");
    } finally { dispose(repo); }
});

test("evaluator-error: two s2-hold tokens in one Notes cell (bury bypass)", () => {
    const repo = makeRepo();
    try {
        // A single row carrying TWO tokens must be rejected — otherwise the
        // non-global regex matches only the first, burying the second hold.
        writeBacklog(repo, [{
            id: "P1-X-001",
            status: "done",
            notes: "s2-hold: S2-cleared-001 s2-hold: S2-pending-002",
            links: "researches/sources/p1.md",
        }]);
        writePacket(repo, "p1.md", [
            { id: "S2-cleared-001", verdict: "SATISFIED", skill: "cleared", pilot: "scratch (retrospective)" },
            { id: "S2-pending-002", verdict: "PENDING", skill: "pending" },
        ]);
        const r = evaluate(repo);
        assert.equal(r.classification, "evaluator-error");
        const rec = r.records[0];
        assert.match(rec.reason, /2 s2-hold tokens/);
    } finally { dispose(repo); }
});

test("evaluator-error: malformed token with trailing suffix (boundary bypass)", () => {
    const repo = makeRepo();
    try {
        // `S2-cleared-001junk` must NOT be truncated to the valid prefix
        // `S2-cleared-001` (which would resolve to a false clear via the
        // SATISFIED record). The trailing identifier chars make it malformed.
        writeBacklog(repo, [{
            id: "P1-X-001",
            status: "done",
            notes: "s2-hold: S2-cleared-001junk",
            links: "researches/sources/p1.md",
        }]);
        writePacket(repo, "p1.md", [{ id: "S2-cleared-001", verdict: "SATISFIED", skill: "cleared", pilot: "scratch (retrospective)" }]);
        const r = evaluate(repo);
        assert.equal(r.classification, "evaluator-error");
        const rec = r.records[0];
        assert.match(rec.reason, /malformed s2-hold token/);
    } finally { dispose(repo); }
});

test("escaped-pipe in Notes does not shift the s2-hold token out of the Notes cell", () => {
    const repo = makeRepo();
    try {
        // Canonical backlog format allows escaped in-cell pipes (\|). A naive
        // split would misalign columns and skip the row → false clear. The
        // escape-aware split must keep the token in the Notes cell so a
        // committed PENDING hold is seen (blocker), not invisible (clear).
        writeBacklog(repo, [{
            id: "P1-X-001",
            status: "in_progress",
            notes: "verify `grep a\\|b log`; s2-hold: S2-x-001",
            links: "researches/sources/p1.md",
        }]);
        writePacket(repo, "p1.md", [{ id: "S2-x-001", verdict: "PENDING", skill: "x" }]);
        const r = evaluate(repo);
        assert.equal(r.classification, "blocker");
        assert.deepEqual(r.blocking_ids, ["S2-x-001"]);
    } finally { dispose(repo); }
});

test("column misalignment (unescaped pipe) carrying a token is evaluator-error, not a silent skip", () => {
    const repo = makeRepo();
    try {
        // An UNescaped pipe in Notes (malformed, non-canonical) splits the row
        // and shifts the token out of the Notes cell. Line-level discovery +
        // the Notes-cell consistency check must catch this and fail closed — a
        // committed PENDING hold can never be made invisible by misalignment.
        writeBacklog(repo, [{
            id: "P1-X-001",
            status: "in_progress",
            notes: "grep a|b log; s2-hold: S2-x-001",
            links: "researches/sources/p1.md",
        }]);
        writePacket(repo, "p1.md", [{ id: "S2-x-001", verdict: "PENDING", skill: "x" }]);
        const r = evaluate(repo);
        assert.equal(r.classification, "evaluator-error");
        const rec = r.records[0];
        assert.match(rec.reason, /outside the Notes cell|did not parse/);
    } finally { dispose(repo); }
});

test("evaluator-error: valid + malformed evidence records for the same hold (conflict)", () => {
    const repo = makeRepo();
    try {
        // One valid SATISFIED record + a second malformed record (same ID,
        // missing -Skill:) must NOT clear via the valid one — the
        // single-record invariant treats valid+malformed as a conflict.
        writeBacklog(repo, [{ id: "S2-x-001", status: "done", packet: "p1.md" }]);
        const body = "# Packet\n\n### S2 hold: S2-x-001\n- Verdict: SATISFIED\n- Skill: x\n- Pilot: scratch (retrospective)\n\n### S2 hold: S2-x-001\n- Verdict: PENDING\n";
        commitFile(repo, "researches/sources/p1.md", body);
        const r = evaluate(repo);
        assert.equal(r.classification, "evaluator-error");
        const rec = r.records.find((x) => x.hold_id === "S2-x-001");
        assert.match(rec.reason, /conflicting evidence/);
    } finally { dispose(repo); }
});

test("evaluator-error: cross-cell token bury (second token in a non-Notes cell)", () => {
    const repo = makeRepo();
    try {
        // One token in Notes (done+SATISFIED → clear on its own) AND a second
        // token buried in the Task cell whose evidence is PENDING. The
        // line-level/Notes reconciliation must catch the out-of-Notes token
        // and fail closed — never a silent skip to a false clear.
        writeBacklog(repo, [{
            id: "P2-API-001",
            status: "done",
            notes: "s2-hold: S2-foo-001",
            // A second token smuggled into the Task cell:
            task: "ship it s2-hold: S2-evil-001",
            links: "researches/sources/p1.md",
        }]);
        writePacket(repo, "p1.md", [
            { id: "S2-foo-001", verdict: "SATISFIED", skill: "foo", pilot: "scratch (retrospective)" },
            { id: "S2-evil-001", verdict: "PENDING", skill: "evil" },
        ]);
        const r = evaluate(repo);
        assert.equal(r.classification, "evaluator-error");
        const rec = r.records[0];
        assert.match(rec.reason, /outside the Notes cell/);
    } finally { dispose(repo); }
});

test("evaluator-error: malformed suffixed evidence heading hides a PENDING record (line-level evidence discovery)", () => {
    const repo = makeRepo();
    try {
        // A valid SATISFIED record for S2-x-001 + a SECOND record for the same
        // ID under a malformed suffixed heading ("### S2 hold: S2-x-001 extra")
        // carrying PENDING. Without line-level evidence discovery the suffixed
        // heading is silently skipped → clear. It must instead be a malformed
        // entry that conflicts with the valid record → evaluator-error.
        writeBacklog(repo, [{ id: "S2-x-001", status: "done", packet: "p1.md" }]);
        const body = "# Packet\n\n### S2 hold: S2-x-001\n- Verdict: SATISFIED\n- Skill: x\n- Pilot: scratch (retrospective)\n\n### S2 hold: S2-x-001 extra\n- Verdict: PENDING\n- Skill: x\n";
        commitFile(repo, "researches/sources/p1.md", body);
        const r = evaluate(repo);
        assert.equal(r.classification, "evaluator-error");
    } finally { dispose(repo); }
});

test("revision binding: evaluate(repo, P_SHA) reports the pinned commit's holds, not moving HEAD", () => {
    // Pins the headline ship-review fix #1 (moving-HEAD race). Commit P with a
    // PENDING hold, capture P_SHA, then commit R that RESOLVES it (done +
    // SATISFIED) so HEAD advances. evaluate(repo) (HEAD) → clear; but
    // evaluate(repo, P_SHA) → blocker, because the pinned commit still holds
    // the PENDING state. This is the property that lets the wrapper tag P_SHA
    // safely: G6 evaluates the EXACT commit being tagged.
    const repo = makeRepo();
    try {
        writeBacklog(repo, [{ id: "S2-x-001", status: "in_progress", packet: "p1.md" }]);
        writePacket(repo, "p1.md", [{ id: "S2-x-001", verdict: "PENDING", skill: "x" }]);
        const P_SHA = git(repo, "rev-parse", "HEAD");
        // Advance HEAD by resolving the hold (done + SATISFIED) in a new commit.
        writeBacklog(repo, [{ id: "S2-x-001", status: "done", packet: "p1.md" }]);
        writePacket(repo, "p1.md", [{ id: "S2-x-001", verdict: "SATISFIED", skill: "x", pilot: "scratch (retrospective)" }]);
        // HEAD (newer) is clear; the pinned P_SHA still holds PENDING.
        assert.equal(evaluate(repo).classification, "clear");
        const pinned = evaluate(repo, P_SHA);
        assert.equal(pinned.classification, "blocker");
        assert.deepEqual(pinned.blocking_ids, ["S2-x-001"]);
    } finally { dispose(repo); }
});

// --- authority / determinism ------------------------------------------------

test("determinism: worktree changes do not affect the verdict (reads HEAD:)", () => {
    const repo = makeRepo();
    try {
        writeBacklog(repo, [{ id: "S2-x-001", status: "done", packet: "p1.md" }]);
        writePacket(repo, "p1.md", [{ id: "S2-x-001", verdict: "SATISFIED", skill: "x", pilot: "scratch (retrospective)" }]);
        // Now mutate the WORKTREE (uncommitted) to a PENDING hold.
        writeFileSync(join(repo, "docs/planning/backlog.md"),
            "# Backlog\n\n## Now\n\n| ID | Status | Area | Task | Owner | Notes | Links |\n| --- | --- | --- | --- | --- | --- | --- |\n| P1-X-001 | in_progress | area | t | o | s2-hold: S2-x-001 | researches/sources/p1.md |\n");
        const r = evaluate(repo);
        // Still clear — HEAD is done+SATISFIED.
        assert.equal(r.classification, "clear");
    } finally { dispose(repo); }
});

test("case-sensitivity: lowercase s2-hold id prefix does not match", () => {
    const repo = makeRepo();
    try {
        // `s2-hold: s2-x-001` — lowercase 's2' in the ID does not match HOLD_TOKEN_RE
        writeBacklog(repo, [{ id: "P1-X-001", status: "done", notes: "s2-hold: s2-x-001", links: "researches/sources/p1.md" }]);
        writePacket(repo, "p1.md", [{ id: "S2-x-001", verdict: "SATISFIED", skill: "x", pilot: "scratch (retrospective)" }]);
        const r = evaluate(repo);
        // The token is malformed (lowercase s2 id) → evaluator-error, and the
        // valid evidence record is an orphan (no matching backlog row) → ignored.
        assert.equal(r.classification, "evaluator-error");
    } finally { dispose(repo); }
});

test("sorted output: hold_ids and records are sorted", () => {
    const repo = makeRepo();
    try {
        writeBacklog(repo, [
            { id: "S2-zeta-009", status: "done", packet: "p1.md" },
            { id: "S2-alpha-001", status: "done", packet: "p1.md" },
            { id: "S2-mid-005", status: "done", packet: "p1.md" },
        ]);
        writePacket(repo, "p1.md", [
            { id: "S2-zeta-009", verdict: "SATISFIED", skill: "z", pilot: "scratch (retrospective)" },
            { id: "S2-alpha-001", verdict: "SATISFIED", skill: "a", pilot: "scratch (retrospective)" },
            { id: "S2-mid-005", verdict: "SATISFIED", skill: "m", pilot: "scratch (retrospective)" },
        ]);
        const r = evaluate(repo);
        assert.deepEqual(r.hold_ids, ["S2-alpha-001", "S2-mid-005", "S2-zeta-009"]);
        assert.deepEqual(r.records.map((x) => x.hold_id), ["S2-alpha-001", "S2-mid-005", "S2-zeta-009"]);
    } finally { dispose(repo); }
});

test("archive file pattern: non-matching archive name is ignored", () => {
    const repo = makeRepo();
    try {
        // A hold in a file NOT matching the archive pattern → not discovered.
        writeBacklog(repo, []);
        const lines = [
            "# Notes",
            "",
            "| ID | Status | Area | Task | Owner | Notes | Links |",
            "| --- | --- | --- | --- | --- | --- | --- |",
            "| S2-x-001 | done | a | t | o | s2-hold: S2-x-001 | researches/sources/p1.md |",
        ];
        commitFile(repo, "docs/planning/archive/notes.md", `${lines.join("\n")}\n`);
        writePacket(repo, "p1.md", [{ id: "S2-x-001", verdict: "SATISFIED", skill: "x", pilot: "scratch (retrospective)" }]);
        const r = evaluate(repo);
        // Not discovered (archive file name doesn't match the pattern) → clear.
        assert.equal(r.classification, "clear");
        assert.deepEqual(r.hold_ids, []);
    } finally { dispose(repo); }
});

// --- exit-taxonomy mapping via the CLI --------------------------------------

function runCliInScratch(repo) {
    // Copy the evaluator into the scratch repo so repoRoot() resolves to it.
    mkdirSync(join(repo, ".opencode", "scripts"), { recursive: true });
    copyFileSync(SCRIPT, join(repo, ".opencode", "scripts", "check-s2-holds.mjs"));
    const r = spawnSync("node", [join(repo, ".opencode", "scripts", "check-s2-holds.mjs")], {
        cwd: repo,
        env: { ...process.env, GIT_CONFIG_GLOBAL: "/dev/null", GIT_CONFIG_NOSYSTEM: "1" },
    });
    let payload = null;
    try { payload = JSON.parse((r.stdout && r.stdout.toString()) || ""); } catch (_) { /* leave null */ }
    return { status: r.status, payload };
}

test("exit taxonomy: clear → exit 0", () => {
    const repo = makeRepo();
    try {
        writeBacklog(repo, [{ id: "S2-x-001", status: "done", packet: "p1.md" }]);
        writePacket(repo, "p1.md", [{ id: "S2-x-001", verdict: "SATISFIED", skill: "x", pilot: "scratch (retrospective)" }]);
        const { status, payload } = runCliInScratch(repo);
        assert.equal(status, 0);
        assert.equal(payload.classification, "clear");
    } finally { dispose(repo); }
});

test("exit taxonomy: blocker → exit 1", () => {
    const repo = makeRepo();
    try {
        writeBacklog(repo, [{ id: "S2-x-001", status: "in_progress", packet: "p1.md" }]);
        writePacket(repo, "p1.md", [{ id: "S2-x-001", verdict: "PENDING", skill: "x" }]);
        const { status, payload } = runCliInScratch(repo);
        assert.equal(status, 1);
        assert.equal(payload.classification, "blocker");
    } finally { dispose(repo); }
});

test("exit taxonomy: evaluator-error → exit 2", () => {
    const repo = makeRepo();
    try {
        writeBacklog(repo, [{ id: "S2-x-001", status: "done", packet: "p1.md" }]);
        // no evidence packet → missing → evaluator-error
        const { status, payload } = runCliInScratch(repo);
        assert.equal(status, 2);
        assert.equal(payload.classification, "evaluator-error");
    } finally { dispose(repo); }
});
