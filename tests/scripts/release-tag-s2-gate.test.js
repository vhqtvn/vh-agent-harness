// Release-wrapper integration test for the G6 deterministic S2-hold gate.
//
// CRUX (the behavioral-closure path this test exercises end-to-end): the G6
// gate refuses a PENDING hold BEFORE `git tag -a`, invoked directly through
// scripts/release-tag.sh against a scratch repo whose ceremony satisfies every
// PRIOR gate (G7 DEFER manifest handshake, G0 green tree, G0b clean worktree,
// G1-G5 readiness artifact). A PENDING hold committed in the scratch repo must
// produce a nonzero exit + refusal JSON + NO created tag.
//
// The scratch repo builds the note -> artifact -> manifest handshake
// (HEAD^^=release-prep, HEAD^=readiness artifact, HEAD=DEFER manifest) so the
// wrapper reaches G6 with all prior gates satisfied, then exercises the G6
// block directly.
//
// Run:  vh-agent-harness exec node --test tests/scripts/release-tag-s2-gate.test.js
//       (or: node --test tests/scripts/release-tag-s2-gate.test.js)

import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, writeFileSync, rmSync, copyFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, dirname } from "node:path";
import { spawnSync } from "node:child_process";

const __dirname = dirname(new URL(import.meta.url).pathname.replace(/^\/(?=[A-Za-z]:\/)/, ""));
const REPO_ROOT = join(__dirname, "..", "..");
const RELEASE_TAG = join(REPO_ROOT, "scripts", "release-tag.sh");
const DEFER_EVAL = join(REPO_ROOT, "templates", "core", ".opencode", "scripts", "check-defer-triggers.mjs");
const S2_EVAL = join(REPO_ROOT, "templates", "core", ".opencode", "scripts", "check-s2-holds.mjs");

// Skip the whole file if go or bash is unavailable (the ceremony needs go for
// G0; the wrapper needs bash). A missing toolchain is an environment gap, not a
// regression — the crux is exercised whenever the toolchain is present.
const GO = (() => { try { const r = spawnSync("go", ["version"]); return r.status === 0; } catch (_) { return false; } })();
const BASH = (() => { try { const r = spawnSync("bash", ["-c", "true"]); return r.status === 0; } catch (_) { return false; } })();
const RUN = GO && BASH;

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

function writeAndCommit(repo, files, msg) {
    for (const rel of Object.keys(files)) {
        const abs = join(repo, rel);
        mkdirSync(dirname(abs), { recursive: true });
        writeFileSync(abs, files[rel]);
    }
    git(repo, "add", "-A");
    git(repo, "commit", "-m", msg);
}

function rowMarkdown(r) {
    const notes = r.notes !== undefined ? r.notes : `s2-hold: ${r.id}`;
    const links = r.links !== undefined ? r.links : (r.packet ? `researches/sources/${r.packet}` : "");
    return `| ${r.id} | ${r.status} | area | ${r.id} task | owner | ${notes} | ${links} |`;
}

function backlogMarkdown(rows) {
    const lines = [
        "# Backlog", "", "## Now", "",
        "| ID | Status | Area | Task | Owner | Notes | Links |",
        "| --- | --- | --- | --- | --- | --- | --- |",
    ];
    for (const r of rows) lines.push(rowMarkdown(r));
    return `${lines.join("\n")}\n`;
}

function recordText(rec) {
    const lines = [`### S2 hold: ${rec.id}`];
    if (rec.verdict !== undefined) lines.push(`- Verdict: ${rec.verdict}`);
    if (rec.skill !== undefined) lines.push(`- Skill: ${rec.skill}`);
    if (rec.pilot !== undefined) lines.push(`- Pilot: ${rec.pilot}`);
    return lines.join("\n");
}

function packetMarkdown(records) {
    return `# Packet\n\n${records.map(recordText).join("\n\n")}\n`;
}

// Build the full release ceremony in a scratch repo so release-tag.sh reaches
// G6 with all prior gates satisfied. `s2` configures the committed S2 hold:
//   { backlogRows, evidence: [{packet, records}], evaluator: "real"|"missing"|"malformed" }
function setupCeremony(s2) {
    const root = mkdtempSync(join(tmpdir(), "s2-rel-"));
    const repo = join(root, "repo");
    mkdirSync(repo, { recursive: true });
    git(repo, "init");
    git(repo, "config", "user.email", "t@t");
    git(repo, "config", "user.name", "t");
    const msgFile = join(root, "msg.txt");
    writeFileSync(msgFile, "release v0.1.0\n");

    // Evaluators (committed in the release-prep commit so G6/G7 can invoke them).
    const evalDir = join(repo, ".opencode", "scripts");
    mkdirSync(evalDir, { recursive: true });
    copyFileSync(DEFER_EVAL, join(evalDir, "check-defer-triggers.mjs"));
    const evalChoice = s2.evaluator || "real";
    if (evalChoice === "real") {
        copyFileSync(S2_EVAL, join(evalDir, "check-s2-holds.mjs"));
    } else if (evalChoice === "malformed") {
        writeFileSync(join(evalDir, "check-s2-holds.mjs"), "throw new Error('malformed evaluator crash');\n");
    } else if (evalChoice === "unparseable") {
        // Emits non-JSON stdout and exits 0 — the wrapper must treat the
        // unparseable output as fail-closed evaluator-error.
        writeFileSync(join(evalDir, "check-s2-holds.mjs"), "process.stdout.write('this is not json at all');\n");
    } else if (evalChoice === "unknown-classification") {
        // Emits valid JSON with a classification outside the closed enum and
        // exits 0 — the wrapper's cross-check must refuse.
        writeFileSync(join(evalDir, "check-s2-holds.mjs"), "process.stdout.write(JSON.stringify({classification:'weird'}));\n");
    }
    // "missing" → do not copy check-s2-holds.mjs at all.

    // --- commit P (release-prep, HEAD^^) ---
    const pFiles = {
        "go.mod": "module scratch\n\ngo 1.25.0\n",
        "pkg/pkg.go": "package pkg\n",
        "docs/planning/backlog.md": backlogMarkdown(s2.backlogRows),
    };
    for (const e of s2.evidence) {
        pFiles[`researches/sources/${e.packet}`] = packetMarkdown(e.records);
    }
    writeAndCommit(repo, pFiles, "release-prep");
    const P_SHA = git(repo, "rev-parse", "HEAD");

    // --- commit R (readiness artifact, HEAD^) ---
    const artifact = {
        schema_version: 1,
        commit_sha: P_SHA,
        model_gates: {
            G1_coverage: "ready",
            G2_significance: "ready",
            G3_docs: "ready",
            G4_visibility: "ready",
            G5_curated_note: "ready",
        },
    };
    writeAndCommit(repo, {
        ".vh-agent-harness/release-readiness-pass.json": `${JSON.stringify(artifact, null, 2)}\n`,
    }, "readiness artifact");
    const R_SHA = git(repo, "rev-parse", "HEAD");
    const R_TREE = git(repo, "rev-parse", "HEAD^{tree}");

    // --- commit M (DEFER manifest, HEAD) ---
    const manifest = {
        schema_version: 1,
        release_base: { kind: "root", value: null },
        evaluated_commit: R_SHA,
        evaluated_tree: R_TREE,
        manifest_parent_commit: R_SHA,
        reconciliation: { status: "clear", scope: "all", zero_records_confirmed: true },
        records: [],
    };
    writeAndCommit(repo, {
        ".vh-agent-harness/release-defer-dispositions.json": `${JSON.stringify(manifest, null, 2)}\n`,
    }, "DEFER manifest");

    return { root, repo, msgFile };
}

function runReleaseTag(repo, msgFile) {
    const r = spawnSync("bash", [RELEASE_TAG, "v0.1.0"], {
        cwd: repo,
        env: {
            ...process.env,
            RELEASE_TAG_MESSAGE_FILE: msgFile,
            GIT_CONFIG_GLOBAL: "/dev/null",
            GIT_CONFIG_NOSYSTEM: "1",
        },
    });
    return {
        status: r.status,
        stdout: (r.stdout && r.stdout.toString()) || "",
        stderr: (r.stderr && r.stderr.toString()) || "",
    };
}

function tagExists(repo, version) {
    const r = spawnSync("git", ["tag", "-l", version], { cwd: repo });
    return Boolean((r.stdout && r.stdout.toString().trim()));
}

function dispose(root) {
    try { rmSync(root, { recursive: true, force: true }); } catch (_) { /* best-effort */ }
}

// Parse the wrapper's final JSON line from stdout.
function parseEmit(stdout) {
    const lines = stdout.split("\n").filter(Boolean);
    const last = lines[lines.length - 1];
    try { return JSON.parse(last); } catch (_) { return null; }
}

// --- tests ------------------------------------------------------------------

test("crux: G6 refuses a PENDING hold before git tag -a (end-to-end)", { skip: RUN ? false : "go/bash unavailable" }, () => {
    const { root, repo, msgFile } = setupCeremony({
        backlogRows: [{ id: "S2-x-001", status: "in_progress", packet: "p1.md" }],
        evidence: [{ packet: "p1.md", records: [{ id: "S2-x-001", verdict: "PENDING", skill: "x" }] }],
        evaluator: "real",
    });
    try {
        const { status, stdout } = runReleaseTag(repo, msgFile);
        assert.notEqual(status, 0, "release-tag.sh must exit nonzero on a PENDING S2 hold");
        const emitted = parseEmit(stdout);
        assert.ok(emitted, "wrapper must emit a JSON line");
        assert.equal(emitted.ok, false);
        assert.match(emitted.error, /release-s2-gate: blocker/);
        // The `git tag -a` mutation must NOT have been reached. The wrapper's
        // `tag` field is the version string (populated even on refusal), so the
        // authoritative check is whether the tag ref actually exists.
        assert.equal(tagExists(repo, "v0.1.0"), false, "git tag -a must not have run");
    } finally { dispose(root); }
});

test("evaluator-missing: tag absent (fail-closed)", { skip: RUN ? false : "go/bash unavailable" }, () => {
    const { root, repo, msgFile } = setupCeremony({
        backlogRows: [{ id: "S2-x-001", status: "in_progress", packet: "p1.md" }],
        evidence: [{ packet: "p1.md", records: [{ id: "S2-x-001", verdict: "PENDING", skill: "x" }] }],
        evaluator: "missing",
    });
    try {
        const { status } = runReleaseTag(repo, msgFile);
        assert.notEqual(status, 0);
        assert.equal(tagExists(repo, "v0.1.0"), false, "a missing evaluator must refuse before git tag -a");
    } finally { dispose(root); }
});

test("evaluator-crashed: tag absent (fail-closed)", { skip: RUN ? false : "go/bash unavailable" }, () => {
    const { root, repo, msgFile } = setupCeremony({
        backlogRows: [{ id: "S2-x-001", status: "in_progress", packet: "p1.md" }],
        evidence: [{ packet: "p1.md", records: [{ id: "S2-x-001", verdict: "PENDING", skill: "x" }] }],
        evaluator: "malformed",
    });
    try {
        const { status } = runReleaseTag(repo, msgFile);
        assert.notEqual(status, 0);
        assert.equal(tagExists(repo, "v0.1.0"), false, "a crashed evaluator must refuse before git tag -a");
    } finally { dispose(root); }
});

test("evaluator unparseable output: tag absent (fail-closed)", { skip: RUN ? false : "go/bash unavailable" }, () => {
    const { root, repo, msgFile } = setupCeremony({
        backlogRows: [{ id: "S2-x-001", status: "done", packet: "p1.md" }],
        evidence: [{ packet: "p1.md", records: [{ id: "S2-x-001", verdict: "SATISFIED", skill: "x", pilot: "scratch (retrospective)" }] }],
        evaluator: "unparseable",
    });
    try {
        const { status } = runReleaseTag(repo, msgFile);
        assert.notEqual(status, 0, "unparseable evaluator output must refuse");
        assert.equal(tagExists(repo, "v0.1.0"), false);
    } finally { dispose(root); }
});

test("evaluator unknown classification: tag absent (fail-closed cross-check)", { skip: RUN ? false : "go/bash unavailable" }, () => {
    const { root, repo, msgFile } = setupCeremony({
        backlogRows: [{ id: "S2-x-001", status: "done", packet: "p1.md" }],
        evidence: [{ packet: "p1.md", records: [{ id: "S2-x-001", verdict: "SATISFIED", skill: "x", pilot: "scratch (retrospective)" }] }],
        evaluator: "unknown-classification",
    });
    try {
        const { status } = runReleaseTag(repo, msgFile);
        assert.notEqual(status, 0, "an unknown classification must refuse (cross-check)");
        assert.equal(tagExists(repo, "v0.1.0"), false);
    } finally { dispose(root); }
});

test("real evaluator-error through the wrapper (malformed evidence): tag absent", { skip: RUN ? false : "go/bash unavailable" }, () => {
    // A structurally INVALID input (SATISFIED record missing required Pilot)
    // makes the real evaluator return evaluator-error (exit 2); the wrapper
    // must propagate the refusal and never reach git tag -a.
    const { root, repo, msgFile } = setupCeremony({
        backlogRows: [{ id: "S2-x-001", status: "done", packet: "p1.md" }],
        evidence: [{ packet: "p1.md", records: [{ id: "S2-x-001", verdict: "SATISFIED", skill: "x" }] }],
        evaluator: "real",
    });
    try {
        const { status, stdout } = runReleaseTag(repo, msgFile);
        assert.notEqual(status, 0);
        assert.equal(tagExists(repo, "v0.1.0"), false);
        const emitted = parseEmit(stdout);
        assert.ok(emitted && emitted.error, "wrapper must surface the evaluator-error reason");
        assert.match(emitted.error, /release-s2-gate: evaluator-error/);
    } finally { dispose(root); }
});

test("crux: a buried second s2-hold token (PENDING) is NOT invisible — wrapper refuses", { skip: RUN ? false : "go/bash unavailable" }, () => {
    // The multi-token bury bypass (commit-reviewer F1): a done row carrying TWO
    // tokens where the first is cleared and the second is PENDING. Without the
    // multi-token rejection, the evaluator would match only the first token
    // (clear) and orphan the second hold's PENDING evidence → false clear → tag
    // created. The evaluator must reject the row as evaluator-error.
    const { root, repo, msgFile } = setupCeremony({
        backlogRows: [{
            id: "P1-X-001",
            status: "done",
            notes: "s2-hold: S2-cleared-001 s2-hold: S2-pending-002",
            links: "researches/sources/p1.md",
        }],
        evidence: [{
            packet: "p1.md",
            records: [
                { id: "S2-cleared-001", verdict: "SATISFIED", skill: "cleared", pilot: "scratch (retrospective)" },
                { id: "S2-pending-002", verdict: "PENDING", skill: "pending" },
            ],
        }],
        evaluator: "real",
    });
    try {
        const { status } = runReleaseTag(repo, msgFile);
        assert.notEqual(status, 0, "a buried PENDING token must refuse (not be made invisible)");
        assert.equal(tagExists(repo, "v0.1.0"), false, "git tag -a must not run despite the first token being cleared");
    } finally { dispose(root); }
});

test("resolved+SATISFIED: G6 passes and the tag may be created", { skip: RUN ? false : "go/bash unavailable" }, () => {
    const { root, repo, msgFile } = setupCeremony({
        backlogRows: [{ id: "S2-x-001", status: "done", packet: "p1.md" }],
        evidence: [{ packet: "p1.md", records: [{ id: "S2-x-001", verdict: "SATISFIED", skill: "x", pilot: "scratch (retrospective)" }] }],
        evaluator: "real",
    });
    try {
        const { status, stdout } = runReleaseTag(repo, msgFile);
        const emitted = parseEmit(stdout);
        // G6 clear → the wrapper reaches git tag -a. With RELEASE_TAG_PUSH unset
        // the wrapper is local-only, so the tag is created and the wrapper emits ok.
        assert.equal(status, 0, `expected success reaching the tag; stdout=${stdout}`);
        assert.ok(emitted && emitted.ok, "wrapper should emit ok when G6 clears");
        assert.equal(tagExists(repo, "v0.1.0"), true, "git tag -a must run when G6 clears");
    } finally { dispose(root); }
});

test("no S2 holds: G6 clear and the tag may be created", { skip: RUN ? false : "go/bash unavailable" }, () => {
    const { root, repo, msgFile } = setupCeremony({
        backlogRows: [],
        evidence: [],
        evaluator: "real",
    });
    try {
        const { status } = runReleaseTag(repo, msgFile);
        assert.equal(status, 0, "an empty backlog (no holds) must clear G6");
        assert.equal(tagExists(repo, "v0.1.0"), true);
    } finally { dispose(root); }
});
