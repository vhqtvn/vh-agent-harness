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
import { mkdtempSync, mkdirSync, writeFileSync, rmSync, copyFileSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, dirname } from "node:path";
import { spawnSync } from "node:child_process";
import { PassThrough } from "node:stream";
import vm from "node:vm";

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

// --- json_str regression arms (defer-release-wrapper-refusal-emit-perf) ------
//
// Both arms route payloads through the REAL refusal emit path: version
// validation (scripts/release-tag.sh) lands the operator-supplied version
// string in the JSON `tag` field via json_str and exits 1 BEFORE any git or
// repository access, so no scratch ceremony is needed.

test("json_str perf: large payload escapes through one node invocation (no per-char forks)", { skip: BASH ? false : "bash unavailable" }, () => {
    // The former json_str ran a catch-all loop with ONE subshell fork per
    // character, unconditionally (the fork precedes the "does it need
    // escaping" test). For a large error payload that is ~1 fork per byte —
    // minutes of wall time. Escaping now routes through a single
    // `node -e JSON.stringify` invocation.
    //
    // Fails-by-construction on the old per-char path: 130,000 subshell forks
    // cannot complete inside the 15s spawn guard (even at an implausibly fast
    // 0.1ms/fork that is 13s of pure fork overhead, and real bash subshell
    // forks measure ~0.5-1ms) — the old implementation dies as status=null /
    // SIGTERM under the guard and the status assertion below fails. No wall-
    // time threshold is asserted for the NEW path (a ms threshold would be
    // flaky); completion inside the guard plus full-fidelity round-trip is
    // the deterministic signal.
    const unit = "release-refusal-payload-A"; // 25 chars
    const payload = unit.repeat(5200); // 130,000 chars — under the ~128KiB single-argv kernel limit
    const r = spawnSync("bash", [RELEASE_TAG, payload], {
        cwd: REPO_ROOT,
        timeout: 15000,
        env: { ...process.env },
    });
    assert.equal(
        r.status, 1,
        `refusal must complete inside the 15s guard (status=${r.status} signal=${r.signal}); a timeout kill here means the per-character fork path came back`,
    );
    const emitted = parseEmit((r.stdout && r.stdout.toString()) || "");
    assert.ok(emitted, "refusal JSON line must be emitted");
    assert.equal(emitted.ok, false);
    assert.equal(emitted.tag, payload, "the full 130k payload must survive escaping and round-trip through JSON.parse — no truncation");
});

test("json_str contract: nasty payload round-trips with named-control shorthand and no double escapes", { skip: BASH ? false : "bash unavailable" }, () => {
    // Guards the escaping CONTRACT across the node migration: valid JSON,
    // round-trip identity, \b\t\n\f\r shorthand, \u00XX for other C0 bytes.
    // The no-double-escape assertions are the trap this migration dodged:
    // keeping the old parameter-expansion pre-pass in front of JSON.stringify
    // would emit backslash+backslash+n for a real newline — which round-trips
    // to a literal backslash+n, silently corrupting the payload while still
    // parsing as valid JSON.
    const c0 = [];
    for (let code = 0x01; code <= 0x1f; code++) c0.push(String.fromCharCode(code)); // NUL cannot cross argv
    const payload = [
        c0.join(""),
        "\x7f",
        "héllo 🚡 日本語",
        "'quoted' \"double\"",
        "back\\slash",
        "tab\tnewline\ncr\r",
    ].join("|");
    const r = spawnSync("bash", [RELEASE_TAG, payload], {
        cwd: REPO_ROOT,
        timeout: 15000,
        env: { ...process.env },
    });
    assert.equal(r.status, 1, "version-validation refusal must fire");
    const stdout = (r.stdout && r.stdout.toString()) || "";
    const emitted = parseEmit(stdout);
    assert.ok(emitted, "refusal JSON line must be emitted");
    assert.equal(emitted.tag, payload, "full round-trip identity: all C0 controls, DEL, multi-byte UTF-8, quotes, backslashes");
    // Named-control shorthand for real control bytes...
    assert.ok(stdout.includes("\\n"), "a real newline must escape to the \\n shorthand");
    // ...and the double-escape signature absent. The payload's literal
    // backslash is never adjacent to an n or t ("back\\slash" is followed by
    // '|'), so these detectors are unambiguous.
    assert.ok(!stdout.includes("\\\\n"), "double-escaped \\\\n must not appear (the pre-pass + stringify trap)");
    assert.ok(!stdout.includes("\\\\t"), "double-escaped \\\\t must not appear");
    assert.ok(stdout.includes("\\u0001") && stdout.includes("\\u001f"), "C0 0x01/0x1f must escape to \\u00XX");
});

test("json_str argv-trap: payload beginning with a dash must not reach node's option parser", { skip: BASH ? false : "bash unavailable" }, () => {
    // Regression pinned after TestReleaseTag_PushOnly_RefusesCombinedWithOverride
    // (internal/cli) caught it live: the first node migration passed the payload
    // as a node ARGUMENT — `node -e '...' "$v"` — so any payload beginning with
    // a dash (an error message naming the flag it refuses: "--push-only cannot
    // be combined with ...") was parsed by node as a bad option, which aborted
    // the emit mid-JSON ("error":, — invalid). The payload now crosses via
    // stdin and never touches node's command line; this arm pins that class by
    // construction (an argv-form json_str emits node's "bad option" on stderr
    // and a broken JSON line here).
    const payload = "--push-only cannot be combined with --override-release-version / --override-manifest-sha (the DEFER gate is skipped in push-only mode)";
    const r = spawnSync("bash", [RELEASE_TAG, payload], {
        cwd: REPO_ROOT,
        timeout: 15000,
        env: { ...process.env },
    });
    assert.equal(r.status, 1, "version-validation refusal must fire");
    const emitted = parseEmit((r.stdout && r.stdout.toString()) || "");
    assert.ok(
        emitted,
        "refusal JSON line must be emitted and parse (an argv-form json_str dies as node 'bad option' and emits a broken line here)",
    );
    assert.equal(emitted.tag, payload, "leading-dash payload must round-trip intact");
});

test("json_str chunk-boundary: multibyte char split across two stdin data events round-trips intact", async () => {
    // F1/F2 (commit-review blocker): the json_str node accumulator formerly
    // concatenated STRING chunks (d+=c), which decodes each Buffer chunk
    // independently — a multi-byte UTF-8 sequence split across a pipe chunk
    // boundary corrupted to U+FFFD on both sides of the split. The snippet now
    // accumulates Buffers and decodes exactly once
    // (Buffer.concat(...).toString("utf8")).
    //
    // Determinism: this arm does NOT rely on OS pipe chunking (size-dependent
    // luck). It extracts the LIVE snippet verbatim from scripts/release-tag.sh,
    // hosts it in a vm sandbox whose process.stdin is a PassThrough, and drives
    // TWO data events by construction — write(chunk1); write(chunk2); end() —
    // with the split point placed INSIDE the 4-byte 🚡 sequence (chunk1 ends
    // with the orphaned lead byte 0xF0; chunk2 opens with the continuation
    // bytes 0x9F 0x9A 0xA1). A data-event counter asserts exactly two events
    // fired, so the arm can never silently degrade to a single-chunk pass.
    // Fails by construction on the string-accumulation pattern (each chunk
    // decodes its orphaned partial sequence to U+FFFD, breaking the round-trip).
    const sh = readFileSync(RELEASE_TAG, "utf8");
    const m = sh.match(/json_str\(\)\s*\{[\s\S]*?node -e '([^']+)'/);
    assert.ok(m, "json_str node program must be extractable from release-tag.sh");
    const program = m[1];

    const payload = "chunk-split 🚡 日本語 roundtrip";
    const full = Buffer.from(payload, "utf8");
    const lead = Buffer.from("🚡", "utf8");
    const leadIdx = full.indexOf(lead);
    assert.ok(leadIdx > 0, "payload must contain the multibyte char");
    const splitAt = leadIdx + 1; // after the lead byte 0xF0 — inside the sequence
    const chunk1 = full.subarray(0, splitAt);
    const chunk2 = full.subarray(splitAt);
    assert.equal(chunk1[chunk1.length - 1], lead[0], "chunk1 must end with the orphaned lead byte");

    const pt = new PassThrough();
    let dataEvents = 0;
    pt.on("data", () => { dataEvents += 1; });
    const chunks = [];
    const sandbox = {
        process: { stdin: pt, stdout: { write: (s) => { chunks.push(Buffer.from(s)); return true; } } },
        Buffer,
        JSON,
    };
    vm.runInNewContext(program, sandbox); // attaches pt data/end listeners (flowing mode)
    const ended = new Promise((resolve) => pt.once("end", resolve));
    pt.write(chunk1);
    pt.write(chunk2);
    pt.end();
    await ended;

    assert.equal(dataEvents, 2, "the split must cross exactly two data events");
    const out = Buffer.concat(chunks).toString("utf8");
    assert.ok(!out.includes("\uFFFD"), "no U+FFFD replacement character may appear in the JSON output");
    assert.equal(JSON.parse(out), payload, "the split multibyte char must round-trip intact through JSON.parse");
});
