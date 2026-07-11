// run-e2e.mjs — single-container real-runtime e2e driver.
//
// PROVES: the auto-gate plugin's enforcement path works against a REAL opencode
// runtime (not a synthetic driver). Uses `opencode run` (one-shot CLI) which:
//   - loads external plugins by default (unless --pure / OPENCODE_PURE=1)
//   - runs an in-process server as the SDK fetch fn (NO HTTP listener)
//   - runs one agent turn and exits when the session reaches idle
//   - `--format json` emits structured JSON lines on stdout
//
// ── THE RACE + AIRTIGHT TWO-CASE MATRIX ──────────────────────────────────
//
// `opencode run` ALWAYS auto-replies to `permission.asked`:
//   WITH    --dangerously-skip-permissions → replies "once" (allow)
//   WITHOUT --dangerously-skip-permissions → replies "reject"
// It does NOT short-circuit before the bus publish, so our plugin ALSO sees
// the event. First reply wins; our plugin has a structural head-start (direct
// bus-stream dispatch vs run's SSE→fetch→parse path).
//
// We use ENFORCE mode (stubEvaluate — pure sync, no classifier HTTP) so the
// plugin evaluates and replies as fast as possible. In ENFORCE mode the
// plugin's path is: readConfig (sync) → decidePermission (sync) → reply (SDK).
// This is faster than LIVE mode (which adds a classifier HTTP round-trip).
//
// The test is made airtight by running TWO cases whose PASS condition is the
// OPPOSITE of the run-mode default. A pass PROVES our plugin won the race:
//
//   Case | --dangerously   | stubVerdict | Run default  | PASS = tool outcome
//   ------|:--------------:|-------------|--------------|---------------------
//   A    | absent         | "allow"     | reject       | read PROCEEDS
//   B    | present        | "block"     | once/allow   | read BLOCKED
//
// If the plugin LOSES the race, the outcome matches the run-default and the
// case FAILS loudly — no false pass is possible.
//
// ── WHY BOTH run AND serve ────────────────────────────────────────────────
// `opencode run` uses ONLY Server.Default() (the singleton app) as its
// SDK fetch fn — one app, one middleware chain, one ScopedCache. It also
// auto-replies to permission.asked, creating a race with our plugin that we
// exploit as an airtight two-case proof (see matrix above).
//
// `opencode serve` runs the headless HTTP listener. Current upstream resolves
// the plugin's permission reply correctly OUT OF THE BOX — no source patches
// needed. Two upstream changes retired the bug the e2e USED to patch:
//   (a) the routing layer was rewritten from hand-rolled Hono mounts to Effect
//       HttpApi, eliminating the InstanceMiddleware outlier mount; and
//   (b) the plugin SDK client now threads `directory` via the
//       `x-opencode-directory` header and routes replies over HTTP when a serve
//       listener is active, so the reply resolves the correct pending map
//       regardless of fiber lineage. (plugin/index.ts: `serverUrl?.toString()`
//       baseUrl + conditional in-process fetch override.)
//
// Serve mode has NO auto-reply race (there is no --dangerously-skip-permissions
// equivalent), so the plugin is the SOLE replier. This makes serve-mode
// assertions simpler: allow → read proceeds, block → read rejected. Both
// polarities are tested, proving the plugin's reply resolves under serve
// against current upstream with no patches.

import { spawn, spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import http from "node:http";

// ── paths (all inside the container) ─────────────────────────────────────
const WORKSPACE = "/workspace";
const OPENCODE_SRC = "/opt/opencode/packages/opencode";
const TEST_DIR = "/opt/test";
const AGENT_PORT = 8080;
const CLASSIFIER_PORT = 8081;
const PROMPT_TEXT = "Read /workspace/target.txt";
const TARGET_CONTENT = "readable-target-content";

// ── serve-mode constants ──────────────────────────────────────────────────
const SERVE_PORT = 3000;
const SERVE_PASSWORD = "test-password";
const SERVE_USERNAME = "opencode";
const SERVE_BASE = `http://127.0.0.1:${SERVE_PORT}`;
const SERVE_CASE_TIMEOUT_MS = 90_000;

// ── utilities ────────────────────────────────────────────────────────────

function log(msg) {
    console.log(`[run-e2e] ${msg}`);
}

function sleep(ms) {
    return new Promise((r) => setTimeout(r, ms));
}

// Wait for an HTTP healthz endpoint to respond 200.
async function waitForHealth(port, label, maxAttempts = 60) {
    for (let i = 0; i < maxAttempts; i++) {
        try {
            const ok = await new Promise((resolve) => {
                const req = http.get(
                    `http://127.0.0.1:${port}/healthz`,
                    (res) => {
                        res.resume();
                        resolve(res.statusCode === 200);
                    },
                );
                req.on("error", () => resolve(false));
                req.setTimeout(1000, () => {
                    req.destroy();
                    resolve(false);
                });
            });
            if (ok) {
                log(`${label} ready on :${port}`);
                return true;
            }
        } catch {
            // not ready yet
        }
        await sleep(500);
    }
    throw new Error(`${label} on :${port} not ready after ${maxAttempts} attempts`);
}

// ── config file writers ──────────────────────────────────────────────────

function writeOpencodeJson() {
    // opencode.json — mock provider + permission.read:"ask" (MANDATORY).
    //
    // Without permission.read:"ask", the default build agent pre-allows
    // read: {"*":"allow"} and the permission evaluator's findLast
    // (last-match-wins) short-circuits on allow. The permission.asked event
    // NEVER fires for a pre-allowed read. Setting permission.read:"ask"
    // appends {read,*,ask} AFTER the default, so findLast picks "ask" and
    // the event fires.
    const opencodeJson = {
        $schema: "https://opencode.ai/config.json",
        model: "mock/mock-model",
        provider: {
            mock: {
                name: "Mock LLM",
                options: {
                    baseURL: `http://127.0.0.1:${AGENT_PORT}/v1`,
                    apiKey: "dummy-key",
                },
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
        permission: { read: "ask" },
    };
    fs.writeFileSync(
        path.join(WORKSPACE, "opencode.json"),
        JSON.stringify(opencodeJson, null, 2),
    );
}

function writeGateConfig(stubVerdict) {
    // auto-gate-config.json — ENFORCE mode with a deterministic stub verdict.
    //
    // ENFORCE mode uses stubEvaluate (pure sync — no classifier HTTP call),
    // which is critical for winning the race against run's auto-reply. The
    // stubVerdict field controls the decision:
    //   "allow" → <block>no</block> → status:allow → reply("once")
    //   "block" → <block>yes</block> → status:deny → reply("reject")
    //
    // The plugin reads this config on EACH event (via readConfig()), so we
    // can change stubVerdict between Case A and Case B.
    const gateConfig = {
        enabled: true,
        mode: "enforce",
        stubVerdict: stubVerdict,
        promptFile: path.join(TEST_DIR, "classifier-prompt.md"),
        replyMode: "once",
        onUncertain: "reject",
    };
    fs.writeFileSync(
        path.join(WORKSPACE, ".opencode", "repo-configs", "auto-gate-config.json"),
        JSON.stringify(gateConfig, null, 2),
    );
}

function writeLlmConfig() {
    // auto-gate-llm.json — classifier endpoint config.
    //
    // Not used in ENFORCE mode (stubEvaluate is pure sync), but written for
    // completeness — if the suite is later switched to LIVE mode the config
    // is already in place.
    const llmConfig = {
        modelEndpoint: `http://127.0.0.1:${CLASSIFIER_PORT}/v1/chat/completions`,
        model: "mock-classifier",
        apiKeyEnv: "AUTO_GATE_API_KEY",
        timeoutMs: 5000,
        maxRetries: 1,
        retryDelayMs: 200,
    };
    fs.writeFileSync(
        path.join(WORKSPACE, ".opencode", "repo-configs", "auto-gate-llm.json"),
        JSON.stringify(llmConfig, null, 2),
    );
}

// ── mock helpers ─────────────────────────────────────────────────────────

function resetAgentCounter() {
    try {
        spawnSync(
            "bun",
            ["--eval", `await fetch("http://127.0.0.1:${AGENT_PORT}/reset").then(r=>r.text())`],
            { encoding: "utf8", timeout: 5000 },
        );
    } catch {
        // best effort
    }
}

// ── serve-mode HTTP helpers ───────────────────────────────────────────────
//
// Plain fetch driver (NOT the SDK) against the opencode serve HTTP listener.
// Every request carries:
//   Authorization: Basic base64(opencode:test-password)
//   x-opencode-directory: %2Fworkspace   (so InstanceMiddleware resolves /workspace)

function serveAuthHeader() {
    return (
        "Basic " +
        Buffer.from(`${SERVE_USERNAME}:${SERVE_PASSWORD}`).toString("base64")
    );
}

async function serveFetch(method, urlPath, body) {
    const headers = {
        Authorization: serveAuthHeader(),
        "x-opencode-directory": encodeURIComponent(WORKSPACE),
    };
    const init = { method, headers };
    if (body !== undefined) {
        headers["Content-Type"] = "application/json";
        init.body = JSON.stringify(body);
    }
    return fetch(`${SERVE_BASE}${urlPath}`, init);
}

// Wait for the serve listener to respond healthy on GET /global/health.
async function waitForServe(maxAttempts = 90) {
    for (let i = 0; i < maxAttempts; i++) {
        try {
            const resp = await serveFetch("GET", "/global/health");
            if (resp.ok) {
                const data = await resp.json();
                if (data.healthy) {
                    log(`serve ready on :${SERVE_PORT}`);
                    return true;
                }
            }
        } catch {
            // not ready yet
        }
        await sleep(500);
    }
    throw new Error(
        `serve on :${SERVE_PORT} not ready after ${maxAttempts} attempts`,
    );
}

// Run one serve-mode case: create session, prompt_async, poll messages.
// Returns { messagesJson, sessionID }.
async function runServeCase(opts) {
    const { stubVerdict, label } = opts;

    writeGateConfig(stubVerdict);
    resetAgentCounter();

    // Create a fresh session for this case.
    const createResp = await serveFetch("POST", "/session", {});
    if (!createResp.ok) {
        throw new Error(
            `[${label}] session create failed: ${createResp.status}`,
        );
    }
    const session = await createResp.json();
    const sessionID = session.id;
    log(`[${label}] serve session created: ${sessionID}`);

    // Fire the prompt asynchronously (returns 204 immediately).
    const promptResp = await serveFetch(
        "POST",
        `/session/${sessionID}/prompt_async`,
        { parts: [{ type: "text", text: PROMPT_TEXT }] },
    );
    if (!promptResp.ok && promptResp.status !== 204) {
        throw new Error(
            `[${label}] prompt_async failed: ${promptResp.status}`,
        );
    }
    log(`[${label}] prompt_async sent`);

    // Poll messages for an outcome (content or rejection) up to timeout.
    const deadline = Date.now() + SERVE_CASE_TIMEOUT_MS;
    let messagesJson = "[]";
    while (Date.now() < deadline) {
        await sleep(1000);
        try {
            const msgResp = await serveFetch(
                "GET",
                `/session/${sessionID}/message`,
            );
            if (msgResp.ok) {
                messagesJson = JSON.stringify(await msgResp.json());
                if (
                    messagesJson.includes(TARGET_CONTENT) ||
                    /reject/i.test(messagesJson)
                ) {
                    break; // outcome reached
                }
            }
        } catch {
            // keep polling
        }
    }

    return { messagesJson, sessionID };
}

// Analyze a serve case's outcome from the polled messages JSON.
function analyzeServeCase(label, messagesJson) {
    const hasContent = messagesJson.includes(TARGET_CONTENT);
    const hasRejection = /reject/i.test(messagesJson);
    return { hasContent, hasRejection };
}

// ── run one opencode run case ────────────────────────────────────────────
//
// Runs `opencode run` as a child process, captures stdout (JSON lines) and
// stderr (plugin audit lines + opencode log), returns { stdout, stderr, status }.

function runCase(opts) {
    const { skipPermissions, stubVerdict, label } = opts;

    // Write the gate config for this case's verdict.
    writeGateConfig(stubVerdict);
    resetAgentCounter();

    const runArgs = [
        "run",
        "--cwd", OPENCODE_SRC,
        "--conditions=browser",
        "src/index.ts",
        "run",
        "--dir", WORKSPACE,
        "--model", "mock/mock-model",
    ];
    if (skipPermissions) {
        runArgs.push("--dangerously-skip-permissions");
    }
    runArgs.push("--format", "json", PROMPT_TEXT);

    log(`[${label}] exec: bun ${runArgs.join(" ")}`);

    const result = spawnSync("bun", runArgs, {
        cwd: OPENCODE_SRC,
        env: {
            ...process.env,
            AUTO_GATE_API_KEY: "dummy-key",
            OPENCODE_LOG_LEVEL: "debug",
        },
        encoding: "utf8",
        timeout: 120000,
        maxBuffer: 10 * 1024 * 1024,
    });

    const stdout = result.stdout || "";
    const stderr = result.stderr || "";
    const status = result.status;

    log(`[${label}] exit=${status} stdout=${stdout.length}b stderr=${stderr.length}b`);

    return { stdout, stderr, status };
}

// ── analyze a case's output ──────────────────────────────────────────────

function analyzeCase(label, stdout, stderr) {
    // (a) Plugin got the real event: stderr has the enforce audit line.
    const eventSeen = /\[auto-gate\] permission\.asked type=read mode=enforce/.test(stderr);

    // (b) Tool outcome: does the file content appear in stdout?
    //     For --format json, the tool_use event's part contains the read
    //     result (file content) when the read succeeded, or an error when
    //     blocked.
    const hasContent = stdout.includes(TARGET_CONTENT);

    // Permission rejection indicators in stdout/stderr.
    const hasRejection =
        /permission.*reject/i.test(stdout) ||
        /RejectedError/i.test(stdout) ||
        /\[auto-gate\] blocked:/.test(stderr);

    return { eventSeen, hasContent, hasRejection };
}

// ── print diagnostics on failure ─────────────────────────────────────────

function printDiagnostics(label, stdout, stderr) {
    log(`--- [${label}] stdout JSON lines (first 3000 chars) ---`);
    log(stdout.slice(0, 3000));
    log(`--- [${label}] auto-gate / permission lines from stderr ---`);
    stderr
        .split("\n")
        .filter((l) => /auto-gate|permission\.asked|mock-agent|mock-classifier|DIAG|reject/i.test(l))
        .slice(0, 40)
        .forEach((l) => log(`  err> ${l}`));

    // Dump opencode dev.log if available.
    try {
        const ocLog = fs.readFileSync(
            "/root/.local/share/opencode/log/dev.log",
            "utf8",
        );
        log(`--- [${label}] opencode dev.log (last 30 lines) ---`);
        ocLog
            .split("\n")
            .slice(-30)
            .forEach((l) => log(`  oclog> ${l}`));
    } catch {
        // no log file
    }
}

// ── main ─────────────────────────────────────────────────────────────────

async function main() {
    let mockProc = null;
    let serveProc = null;
    let serveStderrBuf = "";
    let exitCode = 0;

    try {
        // 1. Write config files. Create the repo-configs dir up front so both
        // writers (writeLlmConfig here + writeGateConfig per case) can write
        // without each needing its own mkdir.
        writeOpencodeJson();
        fs.mkdirSync(
            path.join(WORKSPACE, ".opencode", "repo-configs"),
            { recursive: true },
        );
        writeLlmConfig();
        log("config files written");

        // 2. Start mock-llm server (agent endpoint + classifier endpoint).
        log("starting mock-llm server...");
        mockProc = spawn("bun", [path.join(TEST_DIR, "mock-llm-server.js")], {
            env: {
                ...process.env,
                AGENT_PORT: String(AGENT_PORT),
                CLASSIFIER_PORT: String(CLASSIFIER_PORT),
                VERDICT_FILE: "/tmp/classifier-verdict",
                READ_PATH: path.join(WORKSPACE, "target.txt"),
            },
            stdio: ["ignore", "pipe", "pipe"],
        });
        mockProc.stdout.on("data", (d) =>
            process.stderr.write(`[mock] ${d}`),
        );
        mockProc.stderr.on("data", (d) =>
            process.stderr.write(`[mock] ${d}`),
        );

        await waitForHealth(AGENT_PORT, "agent mock");
        await waitForHealth(CLASSIFIER_PORT, "classifier mock");

        // ── CASE A: ALLOW proof ────────────────────────────────────────
        // stubVerdict="allow", NO --dangerously-skip-permissions.
        // Run default = reject. PASS = read PROCEEDS (plugin's allow wins).
        log("========== CASE A (ALLOW proof) ==========");
        const caseA = runCase({
            label: "A",
            skipPermissions: false,
            stubVerdict: "allow",
        });
        const analysisA = analyzeCase("A", caseA.stdout, caseA.stderr);

        // Case A passes if: plugin saw event AND read proceeded (content in
        // stdout). If the plugin lost the race, run's reject would have won →
        // no content → FAIL.
        const caseA_pass = analysisA.eventSeen && analysisA.hasContent;
        log(
            `Case A: eventSeen=${analysisA.eventSeen} content=${analysisA.hasContent} rejection=${analysisA.hasRejection} → ${caseA_pass ? "PASS" : "FAIL"}`,
        );
        if (!caseA_pass) printDiagnostics("A", caseA.stdout, caseA.stderr);

        // ── CASE B: BLOCK proof ────────────────────────────────────────
        // stubVerdict="block", WITH --dangerously-skip-permissions.
        // Run default = once(allow). PASS = read BLOCKED (plugin's reject wins).
        log("========== CASE B (BLOCK proof) ==========");
        const caseB = runCase({
            label: "B",
            skipPermissions: true,
            stubVerdict: "block",
        });
        const analysisB = analyzeCase("B", caseB.stdout, caseB.stderr);

        // Case B passes if: plugin saw event AND read blocked (no content +
        // rejection). If the plugin lost the race, run's allow would have won
        // → content present → FAIL.
        const caseB_pass =
            analysisB.eventSeen && !analysisB.hasContent && analysisB.hasRejection;
        log(
            `Case B: eventSeen=${analysisB.eventSeen} content=${analysisB.hasContent} rejection=${analysisB.hasRejection} → ${caseB_pass ? "PASS" : "FAIL"}`,
        );
        if (!caseB_pass) printDiagnostics("B", caseB.stdout, caseB.stderr);

        // ── RUN-MODE SUMMARY ───────────────────────────────────────────
        log("========== RUN-MODE SUMMARY ==========");
        log(`Run Case A (ALLOW proof): ${caseA_pass ? "PASS" : "FAIL"}`);
        log(`Run Case B (BLOCK proof): ${caseB_pass ? "PASS" : "FAIL"}`);

        // ── SERVE MODE ─────────────────────────────────────────────────
        //
        // Start `opencode serve` (the long-lived HTTP listener). The plugin is
        // loaded by the serve process. We drive sessions over HTTP and verify
        // the plugin's permission reply resolves (allow → read proceeds,
        // block → read rejected).
        //
        // Serve has NO --dangerously-skip-permissions auto-reply, so the plugin
        // is the SOLE replier — no race. Current upstream resolves the plugin's
        // permission reply under serve out of the box (no patches): the Effect
        // HttpApi routing rewrite + per-request `x-opencode-directory` threading
        // retired the InstanceMiddleware/fiber-lineage bug this suite used to patch.
        log("========== STARTING SERVE MODE ==========");
        serveProc = spawn(
            "bun",
            [
                "run",
                "--cwd", OPENCODE_SRC,
                "--conditions=browser",
                "src/index.ts",
                "serve",
                "--hostname", "127.0.0.1",
                "--port", String(SERVE_PORT),
            ],
            {
                cwd: OPENCODE_SRC,
                env: {
                    ...process.env,
                    OPENCODE_SERVER_PASSWORD: SERVE_PASSWORD,
                    OPENCODE_SERVER_USERNAME: SERVE_USERNAME,
                    AUTO_GATE_API_KEY: "dummy-key",
                    OPENCODE_LOG_LEVEL: "debug",
                },
                stdio: ["ignore", "pipe", "pipe"],
            },
        );
        serveProc.stdout.on("data", (d) =>
            process.stderr.write(`[serve] ${d}`),
        );
        serveProc.stderr.on("data", (d) => {
            const s = d.toString();
            serveStderrBuf += s;
            process.stderr.write(`[serve] ${s}`);
        });

        await waitForServe();

        // ── CASE serve-A: ALLOW proof ──────────────────────────────────
        // stubVerdict="allow". The plugin is the sole replier (no run-default).
        // PASS = read PROCEEDS (plugin's allow reply resolved under serve).
        log("========== CASE serve-A (ALLOW proof) ==========");
        const stderrMarkerA = serveStderrBuf.length;
        const serveA = await runServeCase({
            label: "serve-A",
            stubVerdict: "allow",
        });
        const serveAnalysisA = analyzeServeCase(
            "serve-A",
            serveA.messagesJson,
        );
        const serveStderrA = serveStderrBuf.slice(stderrMarkerA);
        const serveA_eventSeen =
            /\[auto-gate\] permission\.asked type=read mode=enforce/.test(
                serveStderrA,
            );
        const serveA_pass = serveA_eventSeen && serveAnalysisA.hasContent;
        log(
            `Case serve-A: eventSeen=${serveA_eventSeen} content=${serveAnalysisA.hasContent} rejection=${serveAnalysisA.hasRejection} → ${serveA_pass ? "PASS" : "FAIL"}`,
        );
        if (!serveA_pass) {
            log(`--- [serve-A] messages JSON (first 3000 chars) ---`);
            log(serveA.messagesJson.slice(0, 3000));
            log(`--- [serve-A] serve stderr excerpt ---`);
            serveStderrA
                .split("\n")
                .filter((l) =>
                    /auto-gate|permission|reject|error|warn/i.test(l),
                )
                .slice(0, 40)
                .forEach((l) => log(`  err> ${l}`));
        }

        // ── CASE serve-B: BLOCK proof ──────────────────────────────────
        // stubVerdict="block". PASS = read BLOCKED (plugin's reject reply
        // resolved under serve).
        log("========== CASE serve-B (BLOCK proof) ==========");
        const stderrMarkerB = serveStderrBuf.length;
        const serveB = await runServeCase({
            label: "serve-B",
            stubVerdict: "block",
        });
        const serveAnalysisB = analyzeServeCase(
            "serve-B",
            serveB.messagesJson,
        );
        const serveStderrB = serveStderrBuf.slice(stderrMarkerB);
        const serveB_eventSeen =
            /\[auto-gate\] permission\.asked type=read mode=enforce/.test(
                serveStderrB,
            );
        const serveB_pass =
            serveB_eventSeen &&
            !serveAnalysisB.hasContent &&
            serveAnalysisB.hasRejection;
        log(
            `Case serve-B: eventSeen=${serveB_eventSeen} content=${serveAnalysisB.hasContent} rejection=${serveAnalysisB.hasRejection} → ${serveB_pass ? "PASS" : "FAIL"}`,
        );
        if (!serveB_pass) {
            log(`--- [serve-B] messages JSON (first 3000 chars) ---`);
            log(serveB.messagesJson.slice(0, 3000));
            log(`--- [serve-B] serve stderr excerpt ---`);
            serveStderrB
                .split("\n")
                .filter((l) =>
                    /auto-gate|permission|reject|error|warn/i.test(l),
                )
                .slice(0, 40)
                .forEach((l) => log(`  err> ${l}`));
        }

        // ── FULL SUMMARY ───────────────────────────────────────────────
        log("========== FULL SUMMARY ==========");
        log(`Run   Case A (ALLOW proof):   ${caseA_pass ? "PASS" : "FAIL"}`);
        log(`Run   Case B (BLOCK proof):   ${caseB_pass ? "PASS" : "FAIL"}`);
        log(`Serve Case A (ALLOW proof):   ${serveA_pass ? "PASS" : "FAIL"}`);
        log(`Serve Case B (BLOCK proof):   ${serveB_pass ? "PASS" : "FAIL"}`);
        const allPass =
            caseA_pass && caseB_pass && serveA_pass && serveB_pass;
        log(`Overall: ${allPass ? "PASS" : "FAIL"}`);

        exitCode = allPass ? 0 : 1;
    } catch (err) {
        log(`FATAL: ${err.message}`);
        console.error(err.stack);
        exitCode = 1;
    } finally {
        if (serveProc) {
            try {
                serveProc.kill("SIGTERM");
                await sleep(500);
                serveProc.kill("SIGKILL");
            } catch {
                // already dead
            }
        }
        if (mockProc) {
            try {
                mockProc.kill("SIGTERM");
                await sleep(500);
                mockProc.kill("SIGKILL");
            } catch {
                // already dead
            }
        }
    }

    process.exit(exitCode);
}

main();
