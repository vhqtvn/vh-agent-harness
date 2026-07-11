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
// ── WHY opencode run (NOT serve) ──────────────────────────────────────────
// `opencode run` uses ONLY Server.Default() (the singleton Hono app) as its
// SDK fetch fn. Every code path — session creation, prompt, permission ask,
// permission reply — goes through the SAME app with the SAME middleware chain
// and the SAME ScopedCache. This guarantees the permission reply resolves the
// Deferred (the pending map is shared).
//
// `opencode serve` has TWO separate Hono apps: the HTTP listener (newly
// created per serve call) and Server.Default() (singleton). The test driver's
// HTTP requests hit the listener while the plugin's in-process SDK replies hit
// DefaultHono. Different middleware chains → potentially different ALS context
// → potentially different ScopedCache entries → the pending map mismatch → the
// Deferred hangs forever. Using `opencode run` sidesteps this entirely.

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

        // ── SUMMARY ────────────────────────────────────────────────────
        log("========== SUMMARY ==========");
        log(`Case A (ALLOW proof): ${caseA_pass ? "PASS" : "FAIL"}`);
        log(`Case B (BLOCK proof): ${caseB_pass ? "PASS" : "FAIL"}`);
        log(`Overall: ${caseA_pass && caseB_pass ? "PASS" : "FAIL"}`);

        exitCode = caseA_pass && caseB_pass ? 0 : 1;
    } catch (err) {
        log(`FATAL: ${err.message}`);
        console.error(err.stack);
        exitCode = 1;
    } finally {
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
