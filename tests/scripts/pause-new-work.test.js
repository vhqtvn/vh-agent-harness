// Black-box tests for the repo-scoped pause on NEW work (memo-4).
//
// These tests verify the behavioral crux: an ENGAGED sentinel refuses covered
// new-work admissions across all THREE covered seams (coord-task activation in
// state-lib.js, bgshell launch/resume, OpenCode TaskTool + dispatch commands)
// while an ABSENT sentinel permits them, and the fail-safe contract
// (malformed/unreadable -> engaged) holds. They drive the real source paths the
// same way an operator hits them, asserting on observable behavior.
//
// Run:  vh-agent-harness exec node --test tests/scripts/pause-new-work.test.js
//       (or: node --test tests/scripts/pause-new-work.test.js)

import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, writeFileSync, mkdirSync, rmSync, chmodSync, existsSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, dirname } from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const REPO = join(__dirname, "..", "..");
const CONTRACT = join(REPO, "templates", "core", ".opencode", "scripts", "pause-new-work.js");
const BGSHELL = join(REPO, "templates", "core", ".opencode", "skills", "bgshell-job", "scripts", "bgshell_job.py");

// Import the contract module directly (ESM). The module resolves stateRoot()
// from OPENCODE_STATE_ROOT, so we point it at a temp dir to isolate the
// sentinel. We re-import per-test via a cache-busting query so each test sees
// its own env.
async function loadContract(stateRoot) {
    const url = "file://" + CONTRACT + "?t=" + Date.now() + Math.random();
    const mod = await import(url);
    return mod;
}

function freshStateDir() {
    const dir = mkdtempSync(join(tmpdir(), "pause-nw-test-"));
    process.env.OPENCODE_STATE_ROOT = dir;
    return dir;
}

test("contract: absent sentinel -> disengaged", async () => {
    freshStateDir();
    const { readPauseState } = await loadContract();
    const s = readPauseState();
    assert.equal(s.engaged, false);
    assert.equal(s.degraded, false);
    assert.equal(s.meta, null);
});

test("contract: present + valid -> engaged, not degraded", async () => {
    const dir = freshStateDir();
    writeFileSync(join(dir, "pause-new-work.json"), JSON.stringify({ engaged_at: "2026-08-10T00:00:00Z", reason: "x" }));
    const { readPauseState } = await loadContract();
    const s = readPauseState();
    assert.equal(s.engaged, true);
    assert.equal(s.degraded, false);
    assert.equal(s.meta && s.meta.reason, "x");
});

test("contract: present + malformed JSON -> engaged (fail-safe), not degraded", async () => {
    const dir = freshStateDir();
    writeFileSync(join(dir, "pause-new-work.json"), "{not valid json");
    const { readPauseState } = await loadContract();
    const s = readPauseState();
    assert.equal(s.engaged, true, "malformed content must be engaged (fail-safe)");
    assert.equal(s.degraded, false, "malformed content is advisory, not a degraded FS failure");
});

test("contract: present + empty body -> engaged (fail-safe), not degraded", async () => {
    const dir = freshStateDir();
    writeFileSync(join(dir, "pause-new-work.json"), "   \n");
    const { readPauseState } = await loadContract();
    const s = readPauseState();
    assert.equal(s.engaged, true);
    assert.equal(s.degraded, false);
});

test("contract: unreadable (perm denied) -> engaged + degraded", async () => {
    // Only meaningful on POSIX where chmod 000 denies read. Skip on Windows.
    if (process.platform === "win32") {
        return;
    }
    const dir = freshStateDir();
    const sp = join(dir, "pause-new-work.json");
    writeFileSync(sp, JSON.stringify({ reason: "x" }));
    chmodSync(sp, 0o000);
    try {
        const { readPauseState } = await loadContract();
        const s = readPauseState();
        assert.equal(s.engaged, true, "unreadable sentinel must be engaged (fail-safe)");
        assert.equal(s.degraded, true, "unreadable sentinel must report degraded");
    } finally {
        chmodSync(sp, 0o644); // restore so cleanup can delete
    }
});

test("contract: engage writes sentinel, disengage removes it, status reports", async () => {
    const dir = freshStateDir();
    const { engage, disengage, status, readPauseState } = await loadContract();
    assert.equal(readPauseState().engaged, false);
    const e = engage("maintenance window");
    assert.equal(e.engaged, true);
    assert.ok(existsSync(join(dir, "pause-new-work.json")));
    assert.equal(readPauseState().engaged, true);
    assert.match(status().path || "", /pause-new-work\.json$/);
    const d = disengage();
    assert.equal(d.engaged, false);
    assert.equal(d.existed, true);
    assert.equal(existsSync(join(dir, "pause-new-work.json")), false);
    assert.equal(readPauseState().engaged, false);
});

test("contract: formatRefusal mentions the bounded scope (NOT global)", async () => {
    freshStateDir();
    const { engage, formatRefusal } = await loadContract();
    engage("x");
    const msg = formatRefusal("task activation");
    assert.match(msg, /ENGAGED/);
    assert.match(msg, /task activation/);
    assert.match(msg, /repo-scoped pause on NEW work/);
    // naming honesty: must NOT claim global pause
    assert.doesNotMatch(msg, /global pause|ESTOP|abort|kill switch/i, "formatRefusal must not claim global pause");
});

test("contract: dispatch blocklist covers the new-work set, excludes write-task and resume-task", async () => {
    freshStateDir();
    const { isBlockedDispatchCommand, PAUSE_BLOCKED_DISPATCH_COMMANDS } = await loadContract();
    // covered new-work dispatch commands (the "begin new delegated work" class)
    for (const c of ["implement", "implement-goal", "research", "solution-brief"]) {
        assert.equal(isBlockedDispatchCommand(c), true, `${c} should be blocked`);
        assert.equal(isBlockedDispatchCommand("/" + c), true, `/${c} should be blocked`);
    }
    assert.deepEqual([...PAUSE_BLOCKED_DISPATCH_COMMANDS].sort(),
        ["implement", "implement-goal", "research", "solution-brief"]);
    // write-task MUST stay available (creates candidate transport, not execution)
    assert.equal(isBlockedDispatchCommand("write-task"), false, "write-task must NOT be blocked");
    // resume-task is NOT command-level blocked: it is BOTH a new-dispatch AND a
    // continuation entry point; the precise ready->working gate is in JS
    // activateCoordinationTask. Blanket-blocking would forbid continuation.
    assert.equal(isBlockedDispatchCommand("resume-task"), false, "resume-task must NOT be command-blocked");
    assert.equal(isBlockedDispatchCommand("task-list"), false);
    assert.equal(isBlockedDispatchCommand("task-closeout"), false);
    assert.equal(isBlockedDispatchCommand("status"), false);
    assert.equal(isBlockedDispatchCommand("session-start"), false);
    assert.equal(isBlockedDispatchCommand("coordination"), false);
});

// === bgshell gate (Python port) ============================================

function runBgshell(args, env) {
    const fullEnv = { ...process.env, ...(env || {}) };
    const r = spawnSync(process.execPath === "node" ? "python3" : "python3", [BGSHELL, ...args], {
        env: fullEnv,
        encoding: "utf8",
    });
    return r;
}

test("bgshell: absent sentinel -> launch proceeds past the pause gate (refuses on missing command, not pause)", () => {
    const stateDir = freshStateDir();
    // Use a sandbox repo root so bgshell does not touch the real repo. We need
    // a job that fails AFTER the pause gate (e.g. on something past the gate)
    // to prove the gate did not refuse. Launching with an empty command fails
    // before the gate; instead launch a command that exits fast (true) under a
    // throwaway repo root.
    const sandbox = mkdtempSync(join(tmpdir(), "pause-nw-sandbox-"));
    mkdirSync(join(sandbox, ".opencode", "state"), { recursive: true });
    mkdirSync(join(sandbox, "tmp", "agent-runs"), { recursive: true });
    const r = runBgshell(
        ["launch", "--session", "s", "--job", "j", "--", "true"],
        {
            ["{{PROJECT_SLUG}}_BGSHELL_JOB_REPO_ROOT"]: sandbox,
            OPENCODE_STATE_ROOT: stateDir,
            OPENCODE_SESSION_ID: "test-session-1",
        },
    );
    // Absent sentinel -> NOT a pause refusal. (The launch may still fail on
    // environment specifics; the assertion is that the failure is NOT the pause
    // message.)
    const combined = (r.stdout || "") + (r.stderr || "");
    assert.doesNotMatch(combined, /Pause on new work is ENGAGED/, "absent sentinel must not trigger pause refusal");
});

test("bgshell: engaged sentinel -> launch refused with pause message", () => {
    const stateDir = freshStateDir();
    writeFileSync(join(stateDir, "pause-new-work.json"), JSON.stringify({ reason: "x" }));
    const sandbox = mkdtempSync(join(tmpdir(), "pause-nw-sandbox-"));
    mkdirSync(join(sandbox, ".opencode", "state"), { recursive: true });
    const r = runBgshell(
        ["launch", "--session", "s", "--job", "j", "--", "true"],
        {
            ["{{PROJECT_SLUG}}_BGSHELL_JOB_REPO_ROOT"]: sandbox,
            OPENCODE_STATE_ROOT: stateDir,
            OPENCODE_SESSION_ID: "test-session-2",
        },
    );
    assert.notEqual(r.status, 0, "engaged sentinel: launch must exit non-zero");
    const combined = (r.stdout || "") + (r.stderr || "");
    assert.match(combined, /Pause on new work is ENGAGED/);
    assert.match(combined, /bgshell launch/);
});

test("bgshell: engaged sentinel -> resume refused with pause message", () => {
    const stateDir = freshStateDir();
    writeFileSync(join(stateDir, "pause-new-work.json"), JSON.stringify({ reason: "x" }));
    const sandbox = mkdtempSync(join(tmpdir(), "pause-nw-sandbox-"));
    mkdirSync(join(sandbox, ".opencode", "state"), { recursive: true });
    // resume needs a job file to exist; without one it fails on "Job state not
    // found" BEFORE the gate. So seed a minimal finished job so we reach the
    // resume gate.
    const jobDir = join(sandbox, "tmp", "agent-runs", "s", "bg-jobs", "j");
    mkdirSync(jobDir, { recursive: true });
    writeFileSync(
        join(jobDir, "job.json"),
        JSON.stringify({
            schema_version: 1,
            session_name: "s",
            job_name: "j",
            state: "succeeded",
            attempt: 1,
            cwd: sandbox,
            command: ["true"],
            env_overrides: {},
            job_dir: "tmp/agent-runs/s/bg-jobs/j",
            log_path: "tmp/agent-runs/s/bg-jobs/j/job.log",
            created_at: "2026-01-01T00:00:00Z",
            updated_at: "2026-01-01T00:00:00Z",
            wrapper_pid: null,
            child_pid: null,
            child_pgid: null,
            return_code: 0,
            started_at: "2026-01-01T00:00:00Z",
            finished_at: "2026-01-01T00:00:00Z",
            interrupted_at: null,
            stop_requested_at: null,
        }),
    );
    const r = runBgshell(
        ["resume", "--session", "s", "--job", "j"],
        {
            ["{{PROJECT_SLUG}}_BGSHELL_JOB_REPO_ROOT"]: sandbox,
            OPENCODE_STATE_ROOT: stateDir,
        },
    );
    assert.notEqual(r.status, 0, "engaged sentinel: resume must exit non-zero");
    const combined = (r.stdout || "") + (r.stderr || "");
    assert.match(combined, /Pause on new work is ENGAGED/);
    assert.match(combined, /bgshell resume/);
});

// === plugin contract (logic-level; OpenCode not spun up) ===================

test("plugin: TaskTool blocked when engaged, other tools pass", async () => {
    freshStateDir();
    const { engage } = await loadContract();
    engage("x");
    const pluginUrl = "file://" + join(REPO, "templates", "core", ".opencode", "plugins", "pause-new-work.js") + "?t=" + Date.now() + Math.random();
    const { server } = await import(pluginUrl);
    const handlers = await server();

    // TaskTool when engaged -> throws
    await assert.rejects(
        () => handlers["tool.execute.before"]({ tool: "task", sessionID: "s", callID: "c" }, { args: {} }),
        (err) => {
            assert.match(err.message, /Pause on new work is ENGAGED/);
            assert.match(err.message, /TaskTool dispatch/);
            return true;
        },
    );
    // other tools when engaged -> pass (no throw)
    await handlers["tool.execute.before"]({ tool: "read", sessionID: "s", callID: "c" }, { args: {} });
    await handlers["tool.execute.before"]({ tool: "bash", sessionID: "s", callID: "c" }, { args: {} });
});

test("plugin: TaskTool permitted when disengaged", async () => {
    freshStateDir(); // absent sentinel
    const pluginUrl = "file://" + join(REPO, "templates", "core", ".opencode", "plugins", "pause-new-work.js") + "?t=" + Date.now() + Math.random();
    const { server } = await import(pluginUrl);
    const handlers = await server();
    // no throw
    await handlers["tool.execute.before"]({ tool: "task", sessionID: "s", callID: "c" }, { args: {} });
});

test("plugin: dispatch commands blocked when engaged, write-task/status pass", async () => {
    freshStateDir();
    const { engage } = await loadContract();
    engage("x");
    const pluginUrl = "file://" + join(REPO, "templates", "core", ".opencode", "plugins", "pause-new-work.js") + "?t=" + Date.now() + Math.random();
    const { server } = await import(pluginUrl);
    const handlers = await server();

    for (const cmd of ["implement", "implement-goal", "research", "solution-brief"]) {
        await assert.rejects(
            () => handlers["command.execute.before"]({ command: cmd, sessionID: "s", arguments: "" }, { parts: [] }),
            (err) => {
                assert.match(err.message, /Pause on new work is ENGAGED/);
                assert.match(err.message, new RegExp(cmd));
                return true;
            },
        );
    }
    // write-task, resume-task, status, task-list, coordination -> pass
    // (resume-task is deliberately NOT command-blocked; its ready->working
    // gate is in JS activateCoordinationTask, so continuation stays available)
    for (const cmd of ["write-task", "resume-task", "status", "task-list", "coordination", "session-start", "task-closeout"]) {
        await handlers["command.execute.before"]({ command: cmd, sessionID: "s", arguments: "" }, { parts: [] });
    }
});

// === state-lib activation gate (seam #1) ====================================
//
// Drives the RENDERED .opencode/scripts/state-lib.js (the template source copy
// carries a render-guard that refuses to load). Proves the gate NARROWS to
// ready->working only: a ready task is refused under an engaged sentinel, while
// a working task (in-flight continuation) is NOT refused by the pause gate
// (it proceeds to other validation). Uses OPENCODE_LOCAL_COORDINATOR_ROOT to
// isolate the coordinator registry from the real one.

test("state-lib: gate narrows to ready->working (in-flight working continuation NOT blocked)", async () => {
    const stateDir = freshStateDir();
    writeFileSync(join(stateDir, "pause-new-work.json"), JSON.stringify({ reason: "x" })); // engaged
    const coordRoot = mkdtempSync(join(tmpdir(), "pause-nw-coord-"));
    process.env.OPENCODE_LOCAL_COORDINATOR_ROOT = coordRoot;
    const tasksDir = join(coordRoot, "tasks");
    mkdirSync(tasksDir, { recursive: true });
    const baseCard = {
        title: "T",
        task_type: "implementation",
        coordination_mode: "short",
    };
    writeFileSync(
        join(tasksDir, "pause-test-ready.json"),
        JSON.stringify({ ...baseCard, task_id: "pause-test-ready", status: "ready" }),
    );
    // The working card needs an active owner + claimed_at, or the normalizer
    // (state-lib.js ~L1560) downgrades an ownerless working card back to ready.
    writeFileSync(
        join(tasksDir, "pause-test-working.json"),
        JSON.stringify({
            ...baseCard,
            task_id: "pause-test-working",
            status: "working",
            active_session_alias: "owner-session",
            claimed_at: "2026-01-01T00:00:00Z",
        }),
    );

    const stateLibUrl = "file://" + join(REPO, ".opencode", "scripts", "state-lib.js") + "?t=" + Date.now() + Math.random();
    const { activateCoordinationTask } = await import(stateLibUrl);

    // ready + engaged -> the pause gate fires (NEW dispatch refused).
    assert.throws(
        () => activateCoordinationTask("test-session", "pause-test-ready", {}),
        (err) => {
            assert.match(err.message, /Pause on new work is ENGAGED/);
            assert.match(err.message, /ready -> working/);
            assert.equal(err.name, "StateError");
            return true;
        },
    );

    // working + engaged -> the pause gate is SKIPPED (continuation, not new
    // work). The call proceeds to downstream validation and throws something
    // that is NOT the pause refusal (here: a session-binding / takeover check).
    assert.throws(
        () => activateCoordinationTask("test-session", "pause-test-working", {}),
        (err) => {
            assert.doesNotMatch(err.message, /Pause on new work is ENGAGED/);
            return true;
        },
    );

    // disengage: now the ready task is no longer pause-refused; it proceeds to
    // downstream validation (which throws a non-pause error), proving the gate
    // passed on the disengaged path too.
    rmSync(join(stateDir, "pause-new-work.json"), { force: true });
    assert.throws(
        () => activateCoordinationTask("test-session", "pause-test-ready", {}),
        (err) => {
            assert.doesNotMatch(err.message, /Pause on new work is ENGAGED/);
            return true;
        },
    );

    delete process.env.OPENCODE_LOCAL_COORDINATOR_ROOT;
});

// cleanup: unset the env var we set globally so other test suites are unaffected
test("cleanup env", () => {
    delete process.env.OPENCODE_STATE_ROOT;
});
