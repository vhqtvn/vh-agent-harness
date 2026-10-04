// run-e2e.mjs — session-progress-pilot LIVE-RUNTIME receipt driver (e2e).
//
// PROVES: the session-progress-pilot plugin's behavior inside a REAL
// opencode process (installed binary, real tool pipeline, real
// tool.execute.before hook seam), using the PACK SOURCE bytes from
// templates/overlays/session-progress-pilot/ (byte-identical to what
// `make update` renders — the pack carries no render tokens; the committed
// pack test internal/overlay/session_progress_pilot_pack_test.go governs
// render equivalence of the embedded corpus).
//
// The agent model is a deterministic local mock (OpenAI-compatible), so the
// "looping agent" is scripted, not a real LLM. Judge legs use local servers:
// a dead port (instant connection-refused), a stall-headers server (accepts
// the connection, never responds — true deadline/timeout class), and a
// scripted valid-verdict judge (deterministic, packet-aware). Receipts that
// depend on the scripted judge are labeled "scripted-judge, real-seam".
//
// Legs (card defer-session-progress-live-receipts):
//   A LIVE ENFORCE DENY-THROW — enforce + identical failing loop -> the
//     mechanical deny THROWS at the real hook seam; the deny text must
//     surface in run output AND reach the MODEL as tool-error feedback
//     (asserted from the mock server's captured request bodies — the true
//     model-visible receipt), land in verdicts.jsonl as action:"deny", and
//     prevent >=1 execution (side-effect count < turns).
//   B LIVE JUDGE-TIMEOUT — stall-headers judge endpoint: the connection is
//     accepted but no body ever arrives; the plugin's own AbortController
//     fires at the deadline -> verdicts.jsonl records why:"judge-timeout"
//     (NOT judge-network — that is the dead-port class, leg D).
//   C LIVE SLOW-PATH AUDIT — audit mode + scripted valid-verdict judge:
//     verdicts.jsonl records real assessments across the real hook seam.
//     Receipt label: "scripted-judge, real-seam".
//   D DEAD-ENDPOINT FAIL-OPEN — enforce + mechanical off + dead judge port:
//     every judge call classifies judge-network and the call is ALLOWED
//     (fail-open); no deny ever throws; all turns execute.
//
// Run: vh-agent-harness exec node tests/e2e/session-progress-live/run-e2e.mjs
// Env: LEG=A|B|C|D (single leg), KILL_MS (child kill timer, default 90000),
//      OPENCODE_DEBUG=1 (child --print-logs + DEBUG level), KEEP=1 (keep
//      fixture dirs for inspection).
//
// Receipts land in tmp/agent-runs/session-progress-live/receipts/ (bound to
// the repo git rev; command + outcome summaries, never raw dumps). Scratch
// fixture projects live under tmp/agent-runs/session-progress-live/fixture/.
//
// WHY OUT-OF-PROCESS: each leg runs `opencode run --dir <scratch>` as a
// child. No restart of any interactive session, no repo profile change, no
// pack selection — the scratch project owns its .opencode/ tree, and a
// `git init` inside the scratch fences the config up-walk at the scratch
// root so the repo root's .opencode/ and ~/.opencode are not inherited.

import { spawn, spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import http from "node:http";

const REPO = path.resolve(import.meta.dirname, "..", "..", "..");
const RUNROOT = path.join(REPO, "tmp", "agent-runs", "session-progress-live");
const FIXROOT = path.join(RUNROOT, "fixture");
const RECEIPTS = path.join(RUNROOT, "receipts");
const PACK = path.join(REPO, "templates", "overlays", "session-progress-pilot");
const TURNS = 8;
const TURN_DELAY_MS = 300; // space calls so the 1s cadence gate can open
const KILL_MS = parseInt(process.env.KILL_MS || "90000", 10);

const COMMAND = "printf 'LOOP-EVIDENCE-MARK\\n' | tee -a side-effects.txt; false";
const BASE_CFG = {
    enabled: true,
    agents: { "*": "enforce" },
    cadence: { min_interval_seconds: 1, min_new_signatures: 1 },
};

function log(msg) {
    console.log(`[live-e2e] ${msg}`);
}

// ── mock agent model server ────────────────────────────────────────────────
// Deterministic OpenAI-compatible mock. Scripts `turns` identical (or
// varying) failing bash calls, then a terminal text answer. Captures EVERY
// request body (the model-visible surface for leg A's deny feedback).

function startMockAgent({ mode, command, turns }) {
    const bodies = [];
    const events = []; // arrival/response trace for stall diagnosis
    let reqSeq = 0;
    const server = http.createServer((req, res) => {
        let body = "";
        req.on("data", (c) => (body += c));
        req.on("end", () => {
            let parsed = null;
            try {
                parsed = JSON.parse(body);
            } catch {
                parsed = null;
            }
            if (!req.url.includes("/chat/completions")) {
                console.error(`[mock] NON-CHAT URL ${req.url}`);
                res.writeHead(404).end("{}");
                return;
            }
            const hasTools = Array.isArray(parsed && parsed.tools) && parsed.tools.length > 0;
            const n = bodies.filter((b) => Array.isArray(b && b.tools) && b.tools.length > 0).length + 1;
            console.error(
                `[mock] req#${reqSeq} POST ${req.url} stream=${parsed && parsed.stream === true}` +
                ` tools=${hasTools ? parsed.tools.length : 0} msgs=${parsed && parsed.messages ? parsed.messages.length : "?"}`,
            );
            bodies.push(parsed);
            const wantsStream = parsed && parsed.stream === true;
            // Requests WITHOUT tools (title generation, summaries) get a plain
            // text answer and do NOT count toward the scripted tool turns.
            const isToolTurn = hasTools && n <= turns;
            const respond = () => {
                const cmd = mode === "varying" ? `${command} step-${n}` : command;
                const argsJson = JSON.stringify({ command: cmd });
                const callId = `call_${n}`;
                if (wantsStream) {
                    res.writeHead(200, {
                        "Content-Type": "text/event-stream",
                        "Cache-Control": "no-cache",
                        Connection: "keep-alive",
                    });
                    const base = { id: `chatcmpl-${n}`, object: "chat.completion.chunk", model: "mock-model", choices: [] };
                    const chunk = (delta, finish_reason) =>
                        res.write("data: " + JSON.stringify({
                            ...base,
                            created: Math.floor(Date.now() / 1000),
                            choices: [{ index: 0, delta, finish_reason }],
                        }) + "\n\n");
                    if (isToolTurn) {
                        chunk({ role: "assistant", content: null, tool_calls: [{ index: 0, id: callId, type: "function", function: { name: "bash", arguments: "" } }] }, null);
                        chunk({ tool_calls: [{ index: 0, function: { arguments: argsJson } }] }, null);
                        chunk({}, "tool_calls");
                    } else {
                        chunk({ role: "assistant", content: "" }, null);
                        chunk({ content: "all steps attempted" }, null);
                        chunk({}, "stop");
                    }
                    res.write("data: [DONE]\n\n");
                    res.end();
                } else {
                    const payload = isToolTurn
                        ? {
                            id: `chatcmpl-${n}`,
                            object: "chat.completion",
                            model: "mock-model",
                            choices: [{
                                message: {
                                    role: "assistant",
                                    content: null,
                                    tool_calls: [{ id: callId, type: "function", function: { name: "bash", arguments: argsJson } }],
                                },
                                finish_reason: "tool_calls",
                            }],
                        }
                        : {
                            id: `chatcmpl-${n}`,
                            object: "chat.completion",
                            model: "mock-model",
                            choices: [{
                                message: { role: "assistant", content: "all steps attempted" },
                                finish_reason: "stop",
                            }],
                        };
                    const data = JSON.stringify(payload);
                    res.writeHead(200, { "Content-Type": "application/json", "Content-Length": Buffer.byteLength(data) });
                    res.end(data);
                }
                events.push({ t: Date.now(), kind: "resp", reqSeq, isToolTurn, wantsStream });
                console.error(`[mock] resp#${reqSeq} sent (tool-turn=${isToolTurn} stream=${wantsStream})`);
            };
            events.push({ t: Date.now(), kind: "req", reqSeq, hasTools, msgs: parsed && parsed.messages ? parsed.messages.length : 0 });
            reqSeq += 1;
            setTimeout(respond, TURN_DELAY_MS);
        });
    });
    return new Promise((resolve) => {
        server.listen(0, "127.0.0.1", () => {
            resolve({ server, port: server.address().port, bodies: () => bodies, events: () => events });
        });
    });
}

// ── stall-headers judge server (leg B) ─────────────────────────────────────
// Accepts connections and NEVER responds — no headers, no body. The judge's
// own AbortController (deadline) is the only thing that ends the request,
// yielding the true judge-timeout class (a dead port would be judge-network).

function startStallJudge() {
    const conns = [];
    const server = http.createServer(() => {
        // Intentionally never responds.
    });
    server.on("connection", (sock) => conns.push(sock));
    return new Promise((resolve) => {
        server.listen(0, "127.0.0.1", () => {
            resolve({
                server,
                port: server.address().port,
                connCount: () => conns.length,
            });
        });
    });
}

// ── scripted valid-verdict judge (leg C) ───────────────────────────────────
// Deterministic judge: parses the packet from the request (messages[1].content
// is the JSON packet), echoes real observation ids in evidence_refs, and
// returns a strict-schema looping verdict. Receipt label: "scripted-judge,
// real-seam" — the judge model is scripted; the hook seam is real.

function startScriptedJudge() {
    const requests = [];
    const server = http.createServer((req, res) => {
        let body = "";
        req.on("data", (c) => (body += c));
        req.on("end", () => {
            requests.push(body.length);
            let packet = null;
            try {
                const parsed = JSON.parse(body);
                const user = parsed && parsed.messages && parsed.messages[1] && parsed.messages[1].content;
                packet = JSON.parse(user);
            } catch {
                packet = null;
            }
            const ids = (packet && Array.isArray(packet.observations) ? packet.observations : [])
                .map((o) => String(o.id))
                .slice(0, 2);
            const verdict = {
                verdict: "looping",
                confidence: 0.97,
                reason: "scripted judge: identical invocation repeated after adverse results",
                evidence_refs: ids,
            };
            const payload = JSON.stringify({
                id: "judge-scripted",
                object: "chat.completion",
                model: "mock-judge",
                choices: [{ message: { role: "assistant", content: JSON.stringify(verdict) }, finish_reason: "stop" }],
            });
            res.writeHead(200, { "Content-Type": "application/json", "Content-Length": Buffer.byteLength(payload) });
            res.end(payload);
        });
    });
    return new Promise((resolve) => {
        server.listen(0, "127.0.0.1", () => {
            resolve({ server, port: server.address().port, callCount: () => requests.length });
        });
    });
}

// ── fixture project assembly ───────────────────────────────────────────────
// Copies the PACK SOURCE bytes (templates/overlays/session-progress-pilot/)
// into an isolated scratch project. The pack carries no {{tokens}} (verified
// by the committed pack test), so source bytes == rendered bytes. A git init
// inside the scratch fences the config up-walk at the scratch root.

function buildFixture(name, agentPort, pluginCfg) {
    const dir = path.join(FIXROOT, name);
    fs.rmSync(dir, { recursive: true, force: true });
    fs.mkdirSync(path.join(dir, ".opencode", "repo-configs"), { recursive: true });
    fs.mkdirSync(path.join(dir, ".opencode", "plugins"), { recursive: true });
    fs.mkdirSync(path.join(dir, ".opencode", "scripts"), { recursive: true });
    fs.copyFileSync(path.join(PACK, "plugins", "session-progress.js"),
        path.join(dir, ".opencode", "plugins", "session-progress.js"));
    for (const f of ["config", "judge", "policy"]) {
        fs.copyFileSync(
            path.join(PACK, "scripts", `session-progress-${f}.js`),
            path.join(dir, ".opencode", "scripts", `session-progress-${f}.js`));
    }
    // Fence the config up-walk: an isolated git root stops opencode from
    // collecting the REPO root's .opencode/ (plugins, permissions) and keeps
    // the scratch fully self-contained.
    const gitInit = spawnSync("git", ["init", "-q"], { cwd: dir, encoding: "utf8" });
    if (gitInit.status !== 0) {
        throw new Error(`git init failed in scratch: ${gitInit.stderr}`);
    }
    fs.writeFileSync(path.join(dir, "opencode.json"), JSON.stringify({
        $schema: "https://opencode.ai/config.json",
        model: "mock/mock-model",
        provider: {
            mock: {
                name: "Mock LLM",
                npm: "@ai-sdk/openai-compatible",
                options: { baseURL: `http://127.0.0.1:${agentPort}/v1`, apiKey: "dummy-key" },
                models: {
                    "mock-model": {
                        name: "Mock Model",
                        tool_call: true,
                        attachment: false,
                        reasoning: false,
                        temperature: false,
                        limit: { context: 8192, output: 4096 },
                    },
                },
            },
        },
    }, null, 2));
    fs.writeFileSync(
        path.join(dir, ".opencode", "repo-configs", "session-progress.local.json"),
        JSON.stringify(pluginCfg, null, 2));
    return dir;
}

async function runOpencode(dir, judgeEnv, name) {
    const xdg = path.join(dir, ".xdg");
    fs.mkdirSync(xdg, { recursive: true });
    const env = {
        ...process.env,
        XDG_DATA_HOME: path.join(xdg, "data"),
        XDG_CONFIG_HOME: path.join(xdg, "config"),
        XDG_STATE_HOME: path.join(xdg, "state"),
        XDG_CACHE_HOME: path.join(xdg, "cache"),
        OPENCODE_LOG_LEVEL: process.env.OPENCODE_DEBUG ? "DEBUG" : "error",
        ...judgeEnv,
    };
    const args = [
        "run",
        "--dir", dir,
        "--model", "mock/mock-model",
        "--agent", "build",
        "--format", "json",
        "--auto",
        "Perform the prepared steps.",
    ];
    if (process.env.OPENCODE_DEBUG) args.unshift("--print-logs");
    // Stream child output to files as it arrives so a SIGKILL still leaves
    // forensic evidence (the post-first-stream stall leaves stdout EMPTY).
    const outPath = path.join(RUNROOT, `${name}.child-stdout.log`);
    const errPath = path.join(RUNROOT, `${name}.child-stderr.log`);
    const outFile = fs.createWriteStream(outPath);
    const errFile = fs.createWriteStream(errPath);
    const t0 = Date.now();
    const r = await new Promise((resolve) => {
        // stdin MUST be "ignore": `opencode run` reads piped stdin to EOF
        // before prompting (refs run.ts: `await Bun.stdin.text()` when stdin
        // is not a TTY). A default spawn pipe never EOFs -> the child stalls
        // forever before creating the session (the historical "1.18.34
        // post-boot stall" — root cause was stdin, NOT mock-SSE/ai-sdk).
        const child = spawn("opencode", args, { env, cwd: dir, stdio: ["ignore", "pipe", "pipe"] });
        let stdout = "";
        let stderr = "";
        child.stdout.on("data", (d) => {
            stdout += d;
            outFile.write(d);
        });
        child.stderr.on("data", (d) => {
            stderr += d;
            errFile.write(d);
        });
        const timer = setTimeout(() => {
            try {
                child.kill("SIGKILL");
            } catch {
                /* already gone */
            }
        }, KILL_MS);
        child.on("error", (e) => {
            clearTimeout(timer);
            resolve({ stdout, stderr: stderr + String(e), status: -1 });
        });
        child.on("close", (code) => {
            clearTimeout(timer);
            resolve({ stdout, stderr, status: code });
        });
    });
    const settled = new Promise((res) => {
        outFile.end(res);
        errFile.end(res);
    });
    await settled;
    const secs = ((Date.now() - t0) / 1000).toFixed(1);
    return { stdout: r.stdout || "", stderr: r.stderr || "", status: r.status, secs, outPath, errPath, args };
}

function sideEffectCount(dir) {
    const f = path.join(dir, "side-effects.txt");
    try {
        return fs.readFileSync(f, "utf8").split("\n").filter((l) => l.trim()).length;
    } catch {
        return 0;
    }
}

function readVerdicts(dir) {
    try {
        return fs.readFileSync(
            path.join(dir, "tmp", "agent-runs", "session-progress-pilot", "verdicts.jsonl"), "utf8");
    } catch {
        return "";
    }
}

// ── legs ───────────────────────────────────────────────────────────────────

async function caseRun(name, { mode, pluginCfg, judgeEnv = {}, judgeServer }) {
    const mock = await startMockAgent({ mode, command: COMMAND, turns: TURNS });
    try {
        const dir = buildFixture(name, mock.port, pluginCfg);
        const r = await runOpencode(dir, judgeEnv, name);
        log(`${name}: finished in ${r.secs}s (status=${r.status})`);
        const denyInStdout = r.stdout.includes("[session-progress]");
        const bodies = mock.bodies();
        const toolBodies = bodies.filter((b) => Array.isArray(b && b.tools) && b.tools.length > 0);
        const denyInModelFeedback = toolBodies.some((b) =>
            JSON.stringify(b).includes("[session-progress]"));
        const executed = sideEffectCount(dir);
        const verdicts = readVerdicts(dir);
        return {
            name, secs: r.secs, status: r.status, denyInStdout, denyInModelFeedback,
            executed, turns: TURNS, verdicts, stdout: r.stdout, stderr: r.stderr,
            agentModelCalls: toolBodies.length,
            mockEvents: mock.events(),
            judgeServer,
            childOutPath: r.outPath, childErrPath: r.errPath, args: r.args,
        };
    } finally {
        mock.server.close();
    }
}

const results = [];
const LEG_FILTER = (process.env.LEG || process.env.CASE || "").toUpperCase();
const want = (name) => !LEG_FILTER || name.startsWith(LEG_FILTER);

// Leg A — LIVE ENFORCE DENY-THROW: enforce + identical failing loop; judge
// env points at a dead port with a short timeout so the mechanical rule (not
// the judge) supplies the deny. The model-visible surface is asserted from
// the mock's captured request bodies.
if (want("A")) results.push(await caseRun("A-enforce-loop", {
    mode: "identical",
    pluginCfg: { ...BASE_CFG, judge: { timeout_ms: 800 } },
    judgeEnv: {
        SESSION_PROGRESS_JUDGE_MODEL: "mock-judge",
        SESSION_PROGRESS_JUDGE_ENDPOINT: "http://127.0.0.1:1/v1/chat/completions", // dead port
        SESSION_PROGRESS_JUDGE_API_KEY: "k",
    },
}));

// Leg B — LIVE JUDGE-TIMEOUT: audit + stall-headers judge (accepts, never
// responds). The plugin's own AbortController fires at the deadline ->
// why:"judge-timeout" records (the dead-port judge-network class is leg D).
if (want("B")) {
    const stall = await startStallJudge();
    try {
        results.push(await caseRun("B-judge-timeout", {
            mode: "identical",
            pluginCfg: {
                ...BASE_CFG,
                agents: { "*": "audit" },
                mechanical: { enabled: false },
                judge: { timeout_ms: 800 },
            },
            judgeEnv: {
                SESSION_PROGRESS_JUDGE_MODEL: "mock-judge",
                SESSION_PROGRESS_JUDGE_ENDPOINT: `http://127.0.0.1:${stall.port}/v1/chat/completions`,
                SESSION_PROGRESS_JUDGE_API_KEY: "k",
            },
            judgeServer: { kind: "stall-headers", port: stall.port },
        }));
    } finally {
        stall.server.close();
    }
}

// Leg C — LIVE SLOW-PATH AUDIT: audit + scripted valid-verdict judge.
// Receipt label: "scripted-judge, real-seam".
if (want("C")) {
    const judge = await startScriptedJudge();
    try {
        results.push(await caseRun("C-audit-slowpath", {
            mode: "identical",
            pluginCfg: {
                ...BASE_CFG,
                agents: { "*": "audit" },
                judge: { timeout_ms: 1500 },
            },
            judgeEnv: {
                SESSION_PROGRESS_JUDGE_MODEL: "mock-judge",
                SESSION_PROGRESS_JUDGE_ENDPOINT: `http://127.0.0.1:${judge.port}/v1/chat/completions`,
                SESSION_PROGRESS_JUDGE_API_KEY: "k",
            },
            judgeServer: { kind: "scripted-verdict", port: judge.port, calls: judge.callCount },
        }));
    } finally {
        judge.server.close();
    }
}

// Leg D — DEAD-ENDPOINT FAIL-OPEN: enforce + mechanical off + dead judge.
// Semantic-only: every judge call fails judge-network and the call is
// ALLOWED — fail-open under enforce.
if (want("D")) results.push(await caseRun("D-failopen-semantic", {
    mode: "identical",
    pluginCfg: { ...BASE_CFG, mechanical: { enabled: false }, judge: { timeout_ms: 500 } },
    judgeEnv: {
        SESSION_PROGRESS_JUDGE_MODEL: "mock-judge",
        SESSION_PROGRESS_JUDGE_ENDPOINT: "http://127.0.0.1:1/v1/chat/completions",
        SESSION_PROGRESS_JUDGE_API_KEY: "k",
    },
}));

// ── verdicts + receipts ────────────────────────────────────────────────────

let ok = true;
function check(cond, label) {
    console.log(`  ${cond ? "PASS" : "FAIL"}  ${label}`);
    if (!cond) ok = false;
}

const byName = (p) => results.find((r) => r.name.startsWith(p)) || null;
const A = byName("A");
const B = byName("B");
const C = byName("C");
const D = byName("D");

const gitRev = spawnSync("git", ["rev-parse", "HEAD"], { cwd: REPO, encoding: "utf8" });
const REV = (gitRev.stdout || "").trim() || "unknown-rev";

fs.mkdirSync(RECEIPTS, { recursive: true });
function writeReceipt(r, legChecks) {
    if (!r) return;
    const verdictLines = r.verdicts.split("\n").filter((l) => l.trim());
    const kinds = {};
    for (const l of verdictLines) {
        try {
            const rec = JSON.parse(l);
            const key = rec.kind + (rec.why ? `:${rec.why}` : "") + (rec.action ? `/${rec.action}` : "");
            kinds[key] = (kinds[key] || 0) + 1;
        } catch {
            /* skip malformed */
        }
    }
    const denyLine = verdictLines.find((l) => l.includes('"action":"deny"')) || "";
    let denyReason = "";
    try {
        denyReason = denyLine ? JSON.parse(denyLine).reason || "" : "";
    } catch {
        denyReason = "";
    }
    const receipt = {
        leg: r.name,
        label: r.name.startsWith("C") ? "scripted-judge, real-seam" : undefined,
        git_rev: REV,
        command: `vh-agent-harness exec node tests/e2e/session-progress-live/run-e2e.mjs (LEG=${r.name[0]}; invocation LEG=${LEG_FILTER || "ALL"})`,
        child_command: `opencode ${r.args.join(" ")}`,
        outcome: {
            status: r.status,
            seconds: r.secs,
            executed_side_effects: `${r.executed}/${r.turns}`,
            agent_model_calls: r.agentModelCalls,
            deny_in_stdout: r.denyInStdout,
            deny_reached_model_feedback: r.denyInModelFeedback,
            verdicts_lines: verdictLines.length,
            verdict_kinds: kinds,
            deny_reason_excerpt: denyReason.slice(0, 300),
        },
        checks: legChecks,
        evidence_files: {
            child_stdout: path.relative(REPO, r.childOutPath),
            child_stderr: path.relative(REPO, r.childErrPath),
            verdicts_copy: `tmp/agent-runs/session-progress-live/receipts/${r.name}-verdicts.jsonl`,
        },
        judge_server: r.judgeServer || { kind: "dead-port", port: 1 },
        mock_trace_summary: r.mockEvents.slice(0, 6).map((e) => e.kind),
    };
    fs.writeFileSync(path.join(RECEIPTS, `${r.name}-receipt.json`), JSON.stringify(receipt, null, 2));
    if (r.verdicts) {
        fs.writeFileSync(path.join(RECEIPTS, `${r.name}-verdicts.jsonl`), r.verdicts);
    }
    log(`receipt written: tmp/agent-runs/session-progress-live/receipts/${r.name}-receipt.json (rev ${REV.slice(0, 10)})`);
}

console.log("\n=== LIVE-RUNTIME RECEIPTS ===");

if (A) {
    const aChecks = {};
    console.log(`A enforce/loop: status=${A.status} secs=${A.secs} modelCalls=${A.agentModelCalls} executed=${A.executed}/${A.turns}`);
    console.log(`   denyInStdout=${A.denyInStdout} denyReachedModelFeedback=${A.denyInModelFeedback}`);
    aChecks.deny_in_stdout = A.denyInStdout;
    aChecks.deny_reached_model = A.denyInModelFeedback;
    aChecks.executed_lt_turns = A.executed < A.turns;
    aChecks.verdicts_deny = A.verdicts.includes('"action":"deny"');
    aChecks.run_completed = A.status === 0 || A.status === null;
    check(A.denyInStdout, "A: deny reason surfaced in runtime output");
    check(A.denyInModelFeedback, "A: deny reason reached the MODEL as tool-error feedback (next request body)");
    check(A.executed < A.turns, `A: deny prevented >=1 execution (${A.executed}/${A.turns} side effects)`);
    check(A.verdicts.includes('"action":"deny"'), "A: verdicts.jsonl records the deny");
    check(A.status === 0 || A.status === null, `A: opencode run completed (status ${A.status})`);
    writeReceipt(A, aChecks);
}

if (B) {
    const bChecks = {};
    console.log(`B judge-timeout: status=${B.status} secs=${B.secs} modelCalls=${B.agentModelCalls} executed=${B.executed}/${B.turns}`);
    bChecks.judge_timeout_recorded = B.verdicts.includes("judge-timeout");
    bChecks.no_network_class = !B.verdicts.includes("judge-network");
    bChecks.no_deny = !B.denyInStdout;
    bChecks.all_executed = B.executed === B.turns;
    check(B.verdicts.includes("judge-timeout"), "B: verdicts.jsonl records why:judge-timeout (stall-headers, true deadline class)");
    check(!B.verdicts.includes("judge-network"), "B: NOT the dead-port judge-network class");
    check(!B.denyInStdout, "B: timeout fails open (no deny)");
    check(B.executed === B.turns, `B: all turns executed (${B.executed}/${B.turns})`);
    writeReceipt(B, bChecks);
}

if (C) {
    const cChecks = {};
    console.log(`C audit/slowpath: status=${C.status} secs=${C.secs} modelCalls=${C.agentModelCalls} executed=${C.executed}/${C.turns}`);
    cChecks.assessments_with_verdict = /"kind":"assessment"/.test(C.verdicts) && /"verdict":"looping"/.test(C.verdicts);
    cChecks.would_deny = C.verdicts.includes("would-deny");
    cChecks.audit_never_throws = !C.denyInStdout;
    cChecks.all_executed = C.executed === C.turns;
    check(/"kind":"assessment"/.test(C.verdicts) && /"verdict":"looping"/.test(C.verdicts),
        "C: real assessments with parsed verdicts recorded (scripted-judge, real-seam)");
    check(C.verdicts.includes("would-deny"), "C: audit records would-deny");
    check(!C.denyInStdout, "C: audit mode NEVER denies");
    check(C.executed === C.turns, `C: all turns executed (${C.executed}/${C.turns})`);
    writeReceipt(C, cChecks);
}

if (D) {
    const dChecks = {};
    console.log(`D fail-open/semantic: status=${D.status} secs=${D.secs} modelCalls=${D.agentModelCalls} executed=${D.executed}/${D.turns}`);
    dChecks.judge_network_recorded = D.verdicts.includes("judge-network");
    dChecks.no_deny = !D.denyInStdout;
    dChecks.all_executed = D.executed === D.turns;
    check(D.verdicts.includes("judge-network"), "D: judge failure class recorded (dead endpoint -> network)");
    check(!D.denyInStdout, "D: dead judge + semantic-only -> fail-open allow (enforce, NO deny)");
    check(D.executed === D.turns, `D: all turns executed (${D.executed}/${D.turns})`);
    writeReceipt(D, dChecks);
}

if (!results.length) {
    console.log(`no legs ran (LEG=${LEG_FILTER})`);
    process.exit(2);
}
console.log(`\nOVERALL: ${ok ? "ALL LIVE-RUNTIME LEGS PASS" : "FAILURES PRESENT"}`);
if (!ok) {
    for (const r of results) {
        console.log(`\n--- ${r.name} mock trace (first 12) ---`);
        for (const e of r.mockEvents.slice(0, 12)) console.log(`  ${JSON.stringify(e)}`);
        console.log(`--- ${r.name} stderr (first 1500) ---\n${r.stderr.slice(0, 1500)}`);
        console.log(`--- ${r.name} stdout tail (1500) ---\n${r.stdout.slice(-1500)}`);
    }
}
if (!process.env.KEEP) {
    for (const r of results) {
        fs.rmSync(path.join(FIXROOT, r.name), { recursive: true, force: true });
    }
}
process.exit(ok ? 0 : 1);
