// e2e-driver.mjs -- fully-managed plugin e2e for the auto-classifier-pilot overlay.
//
// This driver closes the 6 gaps the integration test (which imports classifyLive
// directly as a module) does NOT cover:
//
//   1. PLUGIN LOAD  — imports the RENDERED /tmpproj/.opencode/plugins/auto-tool-gate.js
//      (not the overlay source) in a real node process. Catches ESM/import errors.
//   2. OVERLAY RENDER — the render already happened at image build time
//      (vh-agent-harness update --target /tmpproj); this driver asserts the files exist.
//   3. CONFIG-FROM-RENDERED-FILE — writes config to the real
//      /tmpproj/.opencode/repo-configs/*.json paths that the plugin reads.
//   4. SYS-PROMPT BINARY — no promptFile in config, so the plugin shells out to
//      `vh-agent-harness sys-prompt auto-gate-classifier` (the real Go binary on PATH).
//   5. HOOK CONTRACT — drives BOTH hook surfaces the way OpenCode does:
//        a. permission.ask hook (DORMANT regression — upstream does not fire it
//           in stock releases; retained as a reserve).
//        b. event hook (PRIMARY enforcement surface) — receives the
//           permission.asked bus event, classifies, and replies via
//           client.postSessionIdPermissionsPermissionId. This is the surface
//           that makes enforce/live actually auto-approve against stock OpenCode
//           (same pattern the upstream ships in ACP/CLI/TUI).
//   6. TRANSCRIPT FETCH — fake client.session.messages returns a RequestResult-
//      shaped {data, error} the plugin reads.
//
// Run inside the e2e-runner container:
//   node --test e2e-driver.mjs
//
// The mock-llm service is reachable at http://mock-llm:8080 (compose DNS).

import { describe, it, before, after } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { spawnSync } from "node:child_process";

// ---------------------------------------------------------------------------
// Paths — the RENDERED plugin + rendered config dir in the temp project.
// ---------------------------------------------------------------------------

const TMPROJ = "/tmpproj";
const RENDERED_PLUGIN = path.join(TMPROJ, ".opencode", "plugins", "auto-tool-gate.js");
const REPO_CONFIGS_DIR = path.join(TMPROJ, ".opencode", "repo-configs");
const GATE_CONFIG_PATH = path.join(REPO_CONFIGS_DIR, "auto-gate-config.json");
const LLM_CONFIG_PATH = path.join(REPO_CONFIGS_DIR, "auto-gate-llm.json");

const MOCK_LLM_BASE = process.env.MOCK_LLM_BASE || "http://mock-llm:8080";

// The API key the plugin reads from process.env[apiKeyEnv]. The value is
// irrelevant (the mock ignores Authorization); it just must be non-empty so the
// HTTP adapter sends the header.
const API_KEY_ENV = "AUTO_GATE_API_KEY";
const API_KEY_VALUE = "dummy-mock-key-value";

// ---------------------------------------------------------------------------
// Canned transcript — the shape client.session.messages returns. Exercises
// gap #6: the plugin reads r.data / r.error (RequestResult shape).
// ---------------------------------------------------------------------------

const CANNED_TRANSCRIPT = [
    {
        info: { role: "user" },
        parts: [{ type: "text", text: "Please list the files in the current directory." }],
    },
    {
        info: { role: "assistant" },
        parts: [{ type: "text", text: "I will run ls -la to show the directory listing." }],
    },
];

// ---------------------------------------------------------------------------
// Fake OpenCode client — faithful stand-in for the SDK client the plugin
// expects. session.messages returns a RequestResult-shaped object.
// ---------------------------------------------------------------------------

let transcriptCallCount = 0;
let eventReplies = [];

function makeFakeClient() {
    return {
        session: {
            messages: async ({ path: { id } = {}, query = {} } = {}) => {
                transcriptCallCount++;
                return {
                    data: CANNED_TRANSCRIPT,
                    error: undefined,
                };
            },
        },
        // The event hook replies through this SDK method (POST
        // /session/{id}/permissions/{permissionID}). Record the call args so
        // the event-hook scenarios can assert the reply disposition.
        postSessionIdPermissionsPermissionId: async (args = {}) => {
            eventReplies.push({
                path: { ...(args.path || {}) },
                body: { ...(args.body || {}) },
            });
            return { data: { id: (args.path || {}).permissionID }, error: undefined };
        },
    };
}

// ---------------------------------------------------------------------------
// Mock helpers — same pattern as the integration test driver.
// ---------------------------------------------------------------------------

async function waitForMockReady(base, timeoutMs = 8000) {
    const deadline = Date.now() + timeoutMs;
    while (Date.now() < deadline) {
        try {
            const res = await fetch(`${base}/healthz`);
            if (res.ok) return;
        } catch (_) {
            // not up yet
        }
        await new Promise((r) => setTimeout(r, 200));
    }
    throw new Error(`mock-llm not ready at ${base}/healthz within ${timeoutMs}ms`);
}

async function resetMockCounts(base) {
    await fetch(`${base}/reset-counts`);
}

async function getMockCount(base, scenario) {
    const res = await fetch(`${base}/count/${scenario}`);
    const body = await res.json();
    return body.count;
}

// ---------------------------------------------------------------------------
// Config writers — write to the REAL rendered config paths (gap #3).
// ---------------------------------------------------------------------------

function ensureConfigDir() {
    fs.mkdirSync(REPO_CONFIGS_DIR, { recursive: true });
}

function writeGateConfig(config) {
    fs.writeFileSync(GATE_CONFIG_PATH, JSON.stringify(config, null, 2) + "\n", "utf8");
}

function writeLlmConfig(config) {
    fs.writeFileSync(LLM_CONFIG_PATH, JSON.stringify(config, null, 2) + "\n", "utf8");
}

function makeLlmConfig(scenario, overrides = {}) {
    return {
        modelEndpoint: `${MOCK_LLM_BASE}/${scenario}`,
        model: "mock-model",
        apiKeyEnv: API_KEY_ENV,
        timeoutMs: 3000,
        maxRetries: 1,
        retryDelayMs: 100,
        ...overrides,
    };
}

// ---------------------------------------------------------------------------
// Hook invocation helper — builds a realistic Permission input + output the
// way OpenCode delivers them to the plugin, then invokes the hook.
// ---------------------------------------------------------------------------

function makePermissionInput() {
    return {
        type: "bash",
        pattern: "ls -la",
        sessionID: "sess-e2e-1",
        messageID: "msg-e2e-1",
        title: "run ls",
    };
}

function makePermissionOutput() {
    // OpenCode initializes output.status to "ask" before calling the hook.
    return { status: "ask" };
}

// ---------------------------------------------------------------------------
// Event-hook helper — builds a fake permission.asked bus event (the payload
// the event hook receives for every ask-routed permission request).
// ---------------------------------------------------------------------------

function makeAskedEvent(opts = {}) {
    return {
        type: "permission.asked",
        properties: {
            id: opts.id || "req-e2e-1",
            sessionID: opts.sessionID || "sess-e2e-1",
            permission: { type: opts.permissionType || "bash" },
            patterns: opts.patterns || ["ls -la"],
            metadata: opts.metadata || {},
            always: opts.always || false,
            tool: opts.tool || "bash",
        },
    };
}

// ===========================================================================
// TEST SUITE
// ===========================================================================

describe("auto-gate-classifier plugin e2e", () => {
    let pluginModule;
    let server;
    let __resetConfigCaches;
    let hooks;

    before(async () => {
        // Gap #2: assert the overlay actually rendered the plugin files.
        assert.ok(
            fs.existsSync(RENDERED_PLUGIN),
            `RENDERED plugin missing at ${RENDERED_PLUGIN} — overlay render failed`,
        );
        for (const sibling of [
            "auto-gate-live.js",
            "auto-gate-verdict.js",
            "auto-gate-scrub.js",
        ]) {
            const p = path.join(TMPROJ, ".opencode", "plugins", sibling);
            assert.ok(fs.existsSync(p), `rendered sibling ${sibling} missing at ${p}`);
        }

        // Gap #4 pre-check: the real Go binary is on PATH and resolves the
        // classifier prompt from the embedded corpus (no promptFile override).
        const sysPromptResult = spawnSync(
            "vh-agent-harness",
            ["sys-prompt", "auto-gate-classifier"],
            { encoding: "utf8", timeout: 5000 },
        );
        assert.equal(
            sysPromptResult.status,
            0,
            `vh-agent-harness sys-prompt exited ${sysPromptResult.status}: ${sysPromptResult.stderr}`,
        );
        assert.ok(
            (sysPromptResult.stdout || "").trim().length > 0,
            "vh-agent-harness sys-prompt returned empty stdout — gap #4 fails",
        );

        // Gap #1: import the RENDERED plugin as ESM in this real node process.
        pluginModule = await import(`file://${RENDERED_PLUGIN}`);
        server = pluginModule.server;
        __resetConfigCaches = pluginModule.__resetConfigCaches;
        assert.equal(typeof server, "function", "plugin does not export server()");
        assert.equal(
            typeof __resetConfigCaches,
            "function",
            "plugin does not export __resetConfigCaches()",
        );

        // Build the plugin instance with the fake client (gap #6 transcript path).
        hooks = await server({ client: makeFakeClient(), directory: TMPROJ });
        assert.ok(hooks && typeof hooks["permission.ask"] === "function");
        assert.ok(
            typeof hooks["event"] === "function",
            "plugin does not expose event hook (PRIMARY enforcement surface)",
        );

        // Set the API key env var the live path reads.
        process.env[API_KEY_ENV] = API_KEY_VALUE;

        ensureConfigDir();

        // Wait for the mock to be reachable over the compose network.
        await waitForMockReady(MOCK_LLM_BASE);
    });

    // -------------------------------------------------------------------------
    // AUDIT mode — gap #5: status stays "ask", no model call.
    // -------------------------------------------------------------------------
    describe("audit mode", () => {
        it('leaves output.status="ask" (unchanged) and makes no model call', async () => {
            await resetMockCounts(MOCK_LLM_BASE);

            writeGateConfig({ enabled: true, mode: "audit" });
            __resetConfigCaches();

            const input = makePermissionInput();
            const output = makePermissionOutput();
            await hooks["permission.ask"](input, output);

            // CRITICAL: audit mode MUST NOT mutate output.status.
            assert.equal(output.status, "ask", "audit mode mutated output.status");

            // No model call: mock count for any scenario stays 0.
            const allowCount = await getMockCount(MOCK_LLM_BASE, "allow");
            assert.equal(allowCount, 0, "audit mode made a model HTTP call");
        });
    });

    // -------------------------------------------------------------------------
    // ENFORCE mode — gap #5: stub decision path, no model call.
    // -------------------------------------------------------------------------
    describe("enforce mode (stub)", () => {
        it('sets output.status="deny" when stubVerdict="block"', async () => {
            await resetMockCounts(MOCK_LLM_BASE);

            writeGateConfig({ enabled: true, mode: "enforce", stubVerdict: "block" });
            __resetConfigCaches();

            const input = makePermissionInput();
            const output = makePermissionOutput();
            await hooks["permission.ask"](input, output);

            assert.equal(output.status, "deny", "enforce+block did not deny");

            const allowCount = await getMockCount(MOCK_LLM_BASE, "allow");
            assert.equal(allowCount, 0, "enforce mode made a model HTTP call");
        });

        it('sets output.status="allow" when stubVerdict="allow"', async () => {
            await resetMockCounts(MOCK_LLM_BASE);

            writeGateConfig({ enabled: true, mode: "enforce", stubVerdict: "allow" });
            __resetConfigCaches();

            const input = makePermissionInput();
            const output = makePermissionOutput();
            await hooks["permission.ask"](input, output);

            assert.equal(output.status, "allow", "enforce+allow did not allow");

            const allowCount = await getMockCount(MOCK_LLM_BASE, "allow");
            assert.equal(allowCount, 0, "enforce mode made a model HTTP call");
        });
    });

    // -------------------------------------------------------------------------
    // LIVE mode — gaps #4 + #6: real HTTP to mock + real sys-prompt binary +
    // transcript fetch + verdict parse.
    // -------------------------------------------------------------------------
    describe("live mode", () => {
        it('returns "allow" for the /allow scenario (real HTTP + binary + transcript)', async () => {
            await resetMockCounts(MOCK_LLM_BASE);

            writeGateConfig({ enabled: true, mode: "live" });
            writeLlmConfig(makeLlmConfig("allow"));
            __resetConfigCaches();

            const beforeCount = transcriptCallCount;
            const input = makePermissionInput();
            const output = makePermissionOutput();
            await hooks["permission.ask"](input, output);

            assert.equal(output.status, "allow", "live /allow did not allow");

            // Gap #6: transcript fetch fired through the fake client.
            assert.ok(
                transcriptCallCount > beforeCount,
                "live mode did not call client.session.messages (transcript fetch path)",
            );

            // Real HTTP path: mock count incremented.
            const count = await getMockCount(MOCK_LLM_BASE, "allow");
            assert.ok(count > 0, `live /allow mock count not > 0 (got ${count})`);
        });

        it('returns "deny" for the /block scenario (real HTTP verdict parse)', async () => {
            await resetMockCounts(MOCK_LLM_BASE);

            writeGateConfig({ enabled: true, mode: "live" });
            writeLlmConfig(makeLlmConfig("block"));
            __resetConfigCaches();

            const input = makePermissionInput();
            const output = makePermissionOutput();
            await hooks["permission.ask"](input, output);

            assert.equal(output.status, "deny", "live /block did not deny");

            const count = await getMockCount(MOCK_LLM_BASE, "block");
            assert.ok(count > 0, `live /block mock count not > 0 (got ${count})`);
        });

        it('returns "allow" after retry for /recover-after-stall (retry-on-idle)', async () => {
            // recover-after-stall: 1st request stalls (held socket), 2nd
            // succeeds. With maxRetries=1 + timeoutMs=1000, the first attempt
            // aborts on timeout, the retry succeeds -> allow.
            await resetMockCounts(MOCK_LLM_BASE);

            writeGateConfig({ enabled: true, mode: "live" });
            writeLlmConfig(
                makeLlmConfig("recover-after-stall", {
                    timeoutMs: 1000,
                    maxRetries: 1,
                    retryDelayMs: 100,
                }),
            );
            __resetConfigCaches();

            const input = makePermissionInput();
            const output = makePermissionOutput();
            await hooks["permission.ask"](input, output);

            assert.equal(
                output.status,
                "allow",
                "live /recover-after-stall did not allow after retry",
            );

            // Exactly 2 model calls: 1st (stall) + 2nd (success).
            const count = await getMockCount(MOCK_LLM_BASE, "recover-after-stall");
            assert.equal(
                count,
                2,
                `recover-after-stall expected count=2 (stall+retry), got ${count}`,
            );
        });
    });

    // -------------------------------------------------------------------------
    // EVENT hook — the PRIMARY enforcement surface. Drives the event hook the
    // way OpenCode delivers it: a permission.asked bus event arrives, the hook
    // classifies, and replies via client.postSessionIdPermissionsPermissionId.
    // The existing permission.ask scenarios above are dormant-hook regression.
    // -------------------------------------------------------------------------
    describe("event hook (enforcement surface)", () => {
        it("audit mode → no reply (observe-only)", async () => {
            eventReplies = [];
            await resetMockCounts(MOCK_LLM_BASE);

            writeGateConfig({ enabled: true, mode: "audit" });
            __resetConfigCaches();

            await hooks["event"]({ event: makeAskedEvent() });

            // CRITICAL: audit mode MUST NOT reply — the human still decides.
            assert.equal(
                eventReplies.length,
                0,
                "audit mode replied (should be observe-only)",
            );
        });

        it('enforce stubVerdict:allow → reply "once" (default replyMode)', async () => {
            eventReplies = [];
            await resetMockCounts(MOCK_LLM_BASE);

            writeGateConfig({ enabled: true, mode: "enforce", stubVerdict: "allow" });
            __resetConfigCaches();

            const event = makeAskedEvent({ id: "req-enf-allow", sessionID: "sess-enf" });
            await hooks["event"]({ event });

            assert.equal(eventReplies.length, 1, "enforce+allow did not reply");
            assert.equal(
                eventReplies[0].body.response,
                "once",
                "default replyMode should be once",
            );
            assert.equal(
                eventReplies[0].path.permissionID,
                "req-enf-allow",
                "path.permissionID mismatch",
            );
            assert.equal(
                eventReplies[0].path.id,
                "sess-enf",
                "path.id (sessionID) mismatch",
            );

            // No model HTTP call in enforce mode.
            const allowCount = await getMockCount(MOCK_LLM_BASE, "allow");
            assert.equal(allowCount, 0, "enforce mode made a model HTTP call");
        });

        it('enforce stubVerdict:allow + replyMode:always → reply "always"', async () => {
            eventReplies = [];
            await resetMockCounts(MOCK_LLM_BASE);

            writeGateConfig({
                enabled: true,
                mode: "enforce",
                stubVerdict: "allow",
                replyMode: "always",
            });
            __resetConfigCaches();

            await hooks["event"]({ event: makeAskedEvent() });

            assert.equal(eventReplies.length, 1, "enforce+allow+always did not reply");
            assert.equal(
                eventReplies[0].body.response,
                "always",
                "replyMode:always should reply always",
            );
        });

        it('enforce stubVerdict:block → reply "reject"', async () => {
            eventReplies = [];
            await resetMockCounts(MOCK_LLM_BASE);

            writeGateConfig({ enabled: true, mode: "enforce", stubVerdict: "block" });
            __resetConfigCaches();

            await hooks["event"]({ event: makeAskedEvent() });

            assert.equal(eventReplies.length, 1, "enforce+block did not reply");
            assert.equal(
                eventReplies[0].body.response,
                "reject",
                "block verdict should reply reject",
            );
        });

        it('live /allow → reply "once" (real HTTP + transcript fetch)', async () => {
            eventReplies = [];
            await resetMockCounts(MOCK_LLM_BASE);

            writeGateConfig({ enabled: true, mode: "live" });
            writeLlmConfig(makeLlmConfig("allow"));
            __resetConfigCaches();

            const beforeCount = transcriptCallCount;
            const event = makeAskedEvent({ id: "req-live-allow" });
            await hooks["event"]({ event });

            assert.equal(eventReplies.length, 1, "live /allow did not reply");
            assert.equal(
                eventReplies[0].body.response,
                "once",
                "live allow should reply once",
            );
            assert.equal(
                eventReplies[0].path.permissionID,
                "req-live-allow",
                "path.permissionID mismatch",
            );

            // Transcript fetch fired through the fake client.
            assert.ok(
                transcriptCallCount > beforeCount,
                "live event did not fetch transcript",
            );

            // Real HTTP path: mock count incremented.
            const count = await getMockCount(MOCK_LLM_BASE, "allow");
            assert.ok(count > 0, `live /allow mock count not > 0 (got ${count})`);
        });

        it('live /block → reply "reject" (real HTTP verdict parse)', async () => {
            eventReplies = [];
            await resetMockCounts(MOCK_LLM_BASE);

            writeGateConfig({ enabled: true, mode: "live" });
            writeLlmConfig(makeLlmConfig("block"));
            __resetConfigCaches();

            await hooks["event"]({ event: makeAskedEvent() });

            assert.equal(eventReplies.length, 1, "live /block did not reply");
            assert.equal(
                eventReplies[0].body.response,
                "reject",
                "live block should reply reject",
            );

            const count = await getMockCount(MOCK_LLM_BASE, "block");
            assert.ok(count > 0, `live /block mock count not > 0 (got ${count})`);
        });
    });

    after(() => {
        // Defensive cleanup: leave no stale env leaking out of this process.
        // (No filesystem cleanup needed — /tmpproj is container-scoped.)
    });
});
