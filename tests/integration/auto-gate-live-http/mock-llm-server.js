// mock-llm-server.js -- scenario-controllable OpenAI-compatible mock LLM server.
//
// PURPOSE: integration-test fixture for the auto-gate live classifier HTTP
// adapter (classifyLive / decideLive in auto-gate-live.js). It speaks the
// OpenAI chat-completions response contract so the REAL fetch path (real
// network, real JSON parsing) can be exercised end-to-end against a real
// socket.
//
// SCENARIO SELECTION: the LAST segment of the request URL path selects the
// scenario. The auto-gate adapter POSTs to config.modelEndpoint (the full URL),
// so the test harness points modelEndpoint at a per-scenario path:
//
//   POST http://host:port/allow                 -> 200, allow verdict
//   POST http://host:port/block                 -> 200, block verdict + reason
//   POST http://host:port/stall                 -> holds connection open (no
//                                                  response); client AbortController
//                                                  fires -> retryable abort
//   POST http://host:port/recover-after-stall   -> 1st request stalls, 2nd+
//                                                  succeeds (proves retry recovery)
//   POST http://host:port/error-5xx             -> 503 (retryable server error)
//   POST http://host:port/error-4xx             -> 404 (non-retryable client error)
//   POST http://host:port/malformed             -> 200 but invalid JSON body
//                                                  (non-retryable parse error)
//   POST http://host:port/empty                 -> 200 but missing content
//                                                  (retryable empty-content)
//
// All identifiers are GENERIC. No real provider names, endpoints, or keys are
// embedded anywhere in this file or its Docker image.

const http = require("node:http");

const PORT = parseInt(process.env.MOCK_PORT || "8080", 10);

// Verdict strings the auto-gate classifier expects (see auto-gate-verdict.js):
//   allow = <block>no</block>
//   block = <block>yes</block><reason>[rule] sentence</reason>
const ALLOW_VERDICT = "<block>no</block>";
const BLOCK_VERDICT =
    "<block>yes</block><reason>[scope-creep] action exceeds the stated task scope</reason>";

// Per-scenario request counter. Serves TWO purposes:
//   1. TEST ASSERTIONS: GET /count/<scenario> returns how many POST requests
//      that scenario path received, so the test can prove retry behavior at the
//      HTTP level (count > 1 => the adapter retried; count === 1 => it did not).
//   2. STATEFUL BEHAVIOR: the recover-after-stall scenario uses the same counter
//      to stall on the first request and succeed on the second.
const counters = new Map();
function getCount(key) {
    return counters.get(key) || 0;
}
function bumpCount(key) {
    const n = (counters.get(key) || 0) + 1;
    counters.set(key, n);
    return n;
}

function sendJson(res, status, obj) {
    const body = JSON.stringify(obj);
    res.writeHead(status, {
        "Content-Type": "application/json",
        "Content-Length": Buffer.byteLength(body),
    });
    res.end(body);
}

// Build a minimal OpenAI-compatible chat-completion response envelope.
function chatResponse(content) {
    return {
        id: "mock-chatcmpl-" + Math.random().toString(36).slice(2, 10),
        object: "chat.completion",
        model: "mock-model",
        choices: [
            {
                index: 0,
                message: { role: "assistant", content: content },
                finish_reason: "stop",
            },
        ],
    };
}

const server = http.createServer(async (req, res) => {
    // Swallow socket-level errors so an aborted stall connection (the client
    // closing after its AbortController fires) does not crash the server.
    req.on("error", function noop() {});
    res.on("error", function noop() {});
    if (res.socket) res.socket.on("error", function noop() {});

    // --- Health / readiness probe (used by the test harness startup loop) ---
    if (req.method === "GET" && (req.url || "").startsWith("/healthz")) {
        sendJson(res, 200, { ok: true });
        return;
    }

    // --- Per-scenario request counter (GET) ---
    // Tests read this to prove retry behavior at the HTTP level: a count > 1
    // means the adapter made multiple requests to that scenario path (i.e. it
    // retried); a count === 1 means it did not. This is independent of the
    // SUT's own retries telemetry, so it remains valid even if the SUT's
    // retry-counter reporting has a bug.
    if (req.method === "GET") {
        const getSeg = (req.url || "").split("?")[0].split("/").filter(Boolean);
        if (getSeg[0] === "count" && getSeg[1]) {
            sendJson(res, 200, { scenario: getSeg[1], count: getCount(getSeg[1]) });
            return;
        }
        if (getSeg[0] === "reset-counts") {
            counters.clear();
            sendJson(res, 200, { ok: true });
            return;
        }
    }

    // --- Read the full request body ---
    // (The adapter sends the complete JSON body before waiting for a response,
    // so this resolves promptly even for the stall scenario — the stall happens
    // AFTER the body is read, by simply not calling res.end().)
    let rawBody = "";
    for await (const chunk of req) rawBody += chunk;

    // --- Scenario = last path segment (before any query string) ---
    const urlPath = (req.url || "").split("?")[0];
    const segments = urlPath.split("/").filter(Boolean);
    const scenario = segments[segments.length - 1] || "allow";

    // Bump the per-scenario counter for EVERY inbound POST. The test harness
    // reads GET /count/<scenario> to prove retry at the HTTP level.
    const requestNum = bumpCount(scenario);

    switch (scenario) {
        case "allow":
            sendJson(res, 200, chatResponse(ALLOW_VERDICT));
            return;

        case "block":
            sendJson(res, 200, chatResponse(BLOCK_VERDICT));
            return;

        case "stall":
            // Hold the connection open WITHOUT responding. The client's
            // AbortController fires after its timeoutMs, aborting the fetch
            // (AbortError -> retryable). We intentionally do nothing here —
            // the socket closes when the client aborts. Every request to this
            // path stalls, so with finite maxRetries the adapter exhausts its
            // attempts and fail-closes to deny.
            return;

        case "recover-after-stall": {
            // STATEFUL: the FIRST request stalls (no response); subsequent
            // requests succeed. Proves the retry loop recovers on the real
            // HTTP path: attempt 1 aborts -> retry -> attempt 2 succeeds.
            if (requestNum === 1) return; // stall on first request only
            sendJson(res, 200, chatResponse(ALLOW_VERDICT));
            return;
        }

        case "error-5xx":
            // 5xx is a retryable transient server error.
            sendJson(res, 503, {
                error: { message: "mock server error", type: "server_error" },
            });
            return;

        case "error-4xx":
            // 4xx is a non-retryable permanent client error (not-found / auth).
            sendJson(res, 404, {
                error: { message: "mock not found", type: "invalid_request_error" },
            });
            return;

        case "malformed":
            // 200 OK but the body is NOT valid JSON. The adapter's res.json()
            // throws -> non-retryable parse error (fail-closed).
            res.writeHead(200, { "Content-Type": "application/json" });
            res.end(
                '{ "choices": [ { "message": { "content": "BROKEN JSON missing close bracket',
            );
            return;

        case "empty":
            // 2xx but choices[0].message.content is missing (empty-content).
            // The adapter treats this as a retryable transient model hiccup.
            sendJson(res, 200, {
                choices: [{ index: 0, message: { role: "assistant" }, finish_reason: "stop" }],
            });
            return;

        default:
            sendJson(res, 404, { error: "unknown scenario: " + scenario });
    }
});

server.listen(PORT, function () {
    const addr = server.address();
    console.log("mock-llm-server listening on port " + addr.port);
});

// Clean shutdown on container stop signals.
process.on("SIGTERM", function () {
    server.close(function () {
        process.exit(0);
    });
});
process.on("SIGINT", function () {
    server.close(function () {
        process.exit(0);
    });
});
