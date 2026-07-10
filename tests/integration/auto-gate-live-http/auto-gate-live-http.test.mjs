// auto-gate-live-http.test.mjs -- integration tests for the auto-gate live
// classifier HTTP adapter against a Dockerized OpenAI-compatible mock.
//
// ARCHITECTURE (full Docker isolation)
//   This test runs INSIDE a container (`tester` service in docker-compose.yml)
//   on a private Docker network alongside the `mock-llm` service. The host
//   only runs `docker compose ... run --rm tester` and reads the exit code.
//   ZERO host port publishing (-p): the mock's control + scenario endpoints
//   are reachable ONLY from within the private network, never from the host.
//
//   This is a PURE CLIENT. It does NOT manage containers, ports, or images —
//   docker compose owns all lifecycle. The test reads MOCK_LLM_BASE from env
//   (set by compose to http://mock-llm:8080) and talks to the mock over the
//   private network. The system-prompt binary is avoided via config.promptFile
//   (the classifier prompt is COPY'd into the tester image).
//
// WHAT THIS EXERCISES
//   The REAL HTTP path of classifyLive / decideLive: real globalThis.fetch,
//   real network I/O to the mock container, real JSON parsing. The unit tests
//   in auto-gate-live.js inject a fake fetchFn; this suite deliberately does NOT
//   inject fetchFn (passes undefined) so the production default
//   (globalThis.fetch) is used.
//
// SCENARIO MATRIX (8 tests; each hits a different URL path on the same mock):
//   allow               -> 200 + allow verdict   -> decideLive: status=allow, retries=0
//   block               -> 200 + block verdict   -> decideLive: status=deny,  retries=0
//   stall               -> never responds        -> decideLive: status=deny,  retries>0; HTTP count>1 (RETRY proof)
//   recover-after-stall -> 1st stalls, 2nd ok    -> decideLive: status=allow, retries=1; HTTP count=2 (RECOVERY proof)
//   error-5xx           -> 503 every time        -> decideLive: status=deny,  retries>0; HTTP count>1 (retryable)
//   error-4xx           -> 404                   -> decideLive: status=deny,  retries=0; HTTP count=1 (NON-retryable)
//   malformed           -> 200 + invalid JSON    -> decideLive: status=deny,  retries=0; HTTP count=1 (NON-retryable)
//   empty               -> 200 + missing content -> decideLive: status=deny,  retries>0; HTTP count>1 (retryable)
//
// RETRY PROOF STRATEGY: retry behavior is cross-checked at TWO independent
// levels — the SUT's own r.retries field AND the mock's per-scenario request
// counter (GET /count/<scenario>). The SUT now reports accurate retries on ALL
// paths (including throw/fail-closed, via error-stamping in auto-gate-live.js),
// so r.retries is a trustworthy witness. The HTTP-level count is RETAINED as an
// independent socket-level cross-check (defense in depth): count>1 => retried,
// count===1 => not retried. Two witnesses agreeing is stronger than either alone.
//
// RUN
//   make test-auto-gate-live
//   # or directly:
//   docker compose -f tests/integration/auto-gate-live-http/docker-compose.yml run --rm tester

import { test, describe, before } from "node:test";
import { strict as assert } from "node:assert";

// Import the SYSTEM UNDER TEST. Inside the tester container the SUT lives at
// /app/sut/auto-gate-live.js (COPY'd by Dockerfile.tester from the overlay
// source). The relative import resolves from this test file's location
// (/app/test/) to /app/sut/.
import {
    classifyLive,
    decideLive,
} from "../sut/auto-gate-live.js";

// ---------------------------------------------------------------------------
// Environment
// ---------------------------------------------------------------------------

// MOCK_LLM_BASE is the mock server's base URL ON THE PRIVATE NETWORK. Compose
// sets this to http://mock-llm:8080 (the service name resolves via Docker DNS).
const MOCK_LLM_BASE = process.env.MOCK_LLM_BASE || "http://mock-llm:8080";

// PROMPT_FILE is the classifier system prompt COPY'd into the tester image by
// Dockerfile.tester. Setting config.promptFile to this path makes
// resolveSystemPrompt read the file directly instead of shelling out to
// `vh-agent-harness sys-prompt` (the Go binary, which is NOT present in a
// node:alpine image).
const PROMPT_FILE =
    process.env.PROMPT_FILE || "/app/prompts/auto-gate-classifier.md";

// Dummy API key value. The adapter reads process.env[config.apiKeyEnv] at call
// time. We set it once at module load; every test uses the same dummy value.
const API_KEY_ENV = "MOCK_GATE_LIVE_KEY";
process.env[API_KEY_ENV] = "dummy-mock-key-value";

// ---------------------------------------------------------------------------
// Client-side helpers (NO docker management — compose owns lifecycle)
// ---------------------------------------------------------------------------

// Poll the mock's /healthz endpoint until it is ready. Compose starts the
// mock-llm service before the tester (via depends_on), but depends_on only
// guarantees container start order, not service readiness. This poll bridges
// the startup gap. It is a standard client-side readiness check, NOT container
// orchestration.
async function waitForMockReady(base, timeoutMs) {
    const deadline = Date.now() + timeoutMs;
    while (Date.now() < deadline) {
        try {
            const res = await fetch(base + "/healthz");
            if (res.ok) return;
        } catch (_) {
            // connection refused / not up yet — retry
        }
        await new Promise((resolve) => setTimeout(resolve, 200));
    }
    throw new Error(
        "mock LLM server at " + base + " did not become ready within " + timeoutMs + "ms",
    );
}

// Reset the mock's per-scenario request counters. Called once in before() so
// the counts are deterministic regardless of whether the mock-llm container is
// fresh (compose run) or reused from a prior invocation. This is a client-side
// HTTP call, NOT container management.
async function resetMockCounts() {
    await fetch(MOCK_LLM_BASE + "/reset-counts");
}

// Query the mock's per-scenario request counter. Returns the number of POST
// requests the named scenario path has received. Used as an independent
// socket-level cross-check of retry behavior (count > 1 => retried),
// complementing the SUT's own r.retries field which now reports accurately on
// all paths.
async function getMockCount(scenario) {
    const res = await fetch(MOCK_LLM_BASE + "/count/" + scenario);
    if (!res.ok) {
        throw new Error("/count/" + scenario + " returned status " + res.status);
    }
    const body = await res.json();
    return body.count;
}

// ---------------------------------------------------------------------------
// Config builder
// ---------------------------------------------------------------------------

function makeConfig(scenario, overrides) {
    const cfg = {
        modelEndpoint: MOCK_LLM_BASE + "/" + scenario,
        model: "mock-model",
        apiKeyEnv: API_KEY_ENV,
        // promptFile avoids shelling out to the `vh-agent-harness sys-prompt`
        // binary (absent in the node:alpine tester image). resolveSystemPrompt
        // reads this file directly when set.
        promptFile: PROMPT_FILE,
        timeoutMs: 8000,
        maxRetries: 1,
        retryDelayMs: 0,
    };
    return Object.assign({}, cfg, overrides || {});
}

// ---------------------------------------------------------------------------
// Suite
// ---------------------------------------------------------------------------

describe("auto-gate live HTTP integration (real fetch path)", { timeout: 120000 }, () => {
    before(async () => {
        await waitForMockReady(MOCK_LLM_BASE, 30000);
        await resetMockCounts();
    });

    // ----- (a) allow verdict -----

    test("allow verdict: decideLive returns status=allow, retries=0", async () => {
        const r = await decideLive(
            makeConfig("allow"),
            "test transcript",
            undefined, // NO fake fetchFn -> real globalThis.fetch
            undefined, // NO fake runnerFn -> promptFile short-circuits resolveSystemPrompt
        );
        assert.equal(r.status, "allow");
        assert.equal(r.retries, 0);
        assert.ok(typeof r.latencyMs === "number" && r.latencyMs >= 0);
    });

    test("allow verdict: classifyLive returns the raw content string", async () => {
        const content = await classifyLive(
            makeConfig("allow"),
            "test transcript",
            undefined, // real fetch
            undefined, // promptFile short-circuit
        );
        assert.equal(typeof content, "string");
        assert.equal(content, "<block>no</block>");
    });

    // ----- (b) block verdict + reason -----

    test("block verdict: decideLive returns status=deny with reason, retries=0", async () => {
        const r = await decideLive(
            makeConfig("block"),
            "test transcript",
            undefined,
            undefined,
        );
        assert.equal(r.status, "deny");
        assert.equal(r.retries, 0);
        assert.match(r.audit, /blocked:/);
        assert.equal(
            r.reason,
            "[scope-creep] action exceeds the stated task scope",
        );
    });

    // ----- (c) stall/idle -> retry then fail-closed deny -----

    test("stall: retries exhausted -> deny; r.retries>0 + HTTP count>1 (RETRY proof)", async () => {
        // timeoutMs:250 so each attempt aborts quickly; maxRetries:2 so the
        // adapter makes 3 total attempts (1 initial + 2 retries), all stall.
        const r = await decideLive(
            makeConfig("stall", { timeoutMs: 250, maxRetries: 2, retryDelayMs: 30 }),
            "test transcript",
            undefined,
            undefined,
        );
        assert.equal(r.status, "deny", "exhausted retries must fail-closed to deny");
        assert.match(r.audit, /fail-closed/);
        // SUT witness: retries is now accurately stamped on thrown errors
        // (committed in eaa13a5d), so r.retries > 0 is trustworthy here.
        assert.ok(
            r.retries > 0,
            "stall must report r.retries > 0 (got " + r.retries + ")",
        );
        // HTTP witness: independent socket-level cross-check. The adapter must
        // have made multiple POST requests to the stall path (1 initial + retries).
        const httpCount = await getMockCount("stall");
        assert.ok(
            httpCount > 1,
            "stall must trigger multiple HTTP requests (got count=" +
                httpCount + ", expected > 1)",
        );
    });

    // ----- (c-bonus) stall then recover -> retry succeeds -----

    test("recover-after-stall: 1st stalls, retry succeeds -> allow (RECOVERY proof)", async () => {
        // First request stalls (aborted after 250ms); retry succeeds.
        const r = await decideLive(
            makeConfig("recover-after-stall", {
                timeoutMs: 250,
                maxRetries: 1,
                retryDelayMs: 30,
            }),
            "test transcript",
            undefined,
            undefined,
        );
        assert.equal(r.status, "allow", "retry after stall must succeed");
        assert.equal(r.retries, 1, "exactly one retry must have occurred");
        // HTTP proof: 2 requests to this path (1 stall + 1 success).
        const httpCount = await getMockCount("recover-after-stall");
        assert.equal(
            httpCount,
            2,
            "recover-after-stall must see exactly 2 HTTP requests (stall + success); got " +
                httpCount,
        );
    });

    // ----- (d) 5xx server error -> retried -----

    test("error-5xx: retried (retryable), fail-closed deny; r.retries>0 + HTTP count>1", async () => {
        const r = await decideLive(
            makeConfig("error-5xx", { maxRetries: 1, retryDelayMs: 0 }),
            "test transcript",
            undefined,
            undefined,
        );
        assert.equal(r.status, "deny");
        assert.ok(
            r.retries > 0,
            "5xx must report r.retries > 0 (got " + r.retries + ")",
        );
        // HTTP proof: 503 is retryable, so the adapter must have made
        // >1 request (1 initial + at least 1 retry).
        const httpCount = await getMockCount("error-5xx");
        assert.ok(
            httpCount > 1,
            "5xx must be retried at the HTTP level (got count=" +
                httpCount + ", expected > 1)",
        );
    });

    // ----- (e) 4xx client error -> NOT retried (immediate fail) -----

    test("error-4xx: NOT retried -> deny; r.retries=0 + HTTP count === 1 (NON-RETRY proof)", async () => {
        const r = await decideLive(
            makeConfig("error-4xx", { maxRetries: 3, retryDelayMs: 0 }),
            "test transcript",
            undefined,
            undefined,
        );
        assert.equal(r.status, "deny");
        assert.equal(
            r.retries,
            0,
            "4xx must NOT be retried (got retries=" + r.retries + ")",
        );
        // HTTP proof: 4xx is non-retryable, so exactly 1 request was made.
        const httpCount = await getMockCount("error-4xx");
        assert.equal(
            httpCount,
            1,
            "4xx must NOT be retried at the HTTP level (got count=" +
                httpCount + ", expected 1)",
        );
    });

    // ----- (f) malformed JSON -> NOT retried (immediate fail-closed) -----

    test("malformed JSON: NOT retried -> deny; r.retries=0 + HTTP count === 1", async () => {
        const r = await decideLive(
            makeConfig("malformed", { maxRetries: 3, retryDelayMs: 0 }),
            "test transcript",
            undefined,
            undefined,
        );
        assert.equal(r.status, "deny");
        assert.equal(
            r.retries,
            0,
            "malformed JSON must NOT be retried (got retries=" + r.retries + ")",
        );
        // HTTP proof: malformed JSON is non-retryable, so exactly 1 request.
        const httpCount = await getMockCount("malformed");
        assert.equal(
            httpCount,
            1,
            "malformed JSON must NOT be retried at the HTTP level (got count=" +
                httpCount + ", expected 1)",
        );
    });

    // ----- (g) empty/missing content -> retried -----

    test("empty content: retried (retryable), fail-closed deny; r.retries>0 + HTTP count>1", async () => {
        const r = await decideLive(
            makeConfig("empty", { maxRetries: 1, retryDelayMs: 0 }),
            "test transcript",
            undefined,
            undefined,
        );
        assert.equal(r.status, "deny");
        assert.ok(
            r.retries > 0,
            "empty content must report r.retries > 0 (got " + r.retries + ")",
        );
        // HTTP proof: empty-content is retryable, so >1 request was made.
        const httpCount = await getMockCount("empty");
        assert.ok(
            httpCount > 1,
            "empty content must be retried at the HTTP level (got count=" +
                httpCount + ", expected > 1)",
        );
    });
});
