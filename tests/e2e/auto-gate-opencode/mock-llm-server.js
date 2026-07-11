// mock-llm-server.js — dual-port OpenAI-compatible mock for the real-runtime e2e.
//
// PURPOSE: serves TWO endpoints inside the single container:
//   :8080  AGENT model endpoint   — drives the opencode agent loop
//   :8081  CLASSIFIER endpoint    — drives the auto-gate plugin's classifyLive
//
// AGENT (:8080, POST /v1/chat/completions) — STATEFUL per process:
//   1st request → emits a tool_call so opencode executes the `read` tool,
//                  which triggers the permission.asked bus event our plugin
//                  hooks.
//   2nd request → short text "Done." (finish_reason=stop) so the session
//                  reaches idle and `opencode run` exits cleanly.
//   GET /reset   → resets the call counter (called between Case A and Case B).
//   GET /healthz → readiness probe.
//
// CLASSIFIER (:8081, POST /v1/chat/completions) — reads a control file:
//   /tmp/classifier-verdict  →  supports TWO shapes:
//     KEYWORD   : "allow"  → returns <block>no</block>
//                 "block"  → returns <block>yes</block><reason>scope creep</reason>
//     PASSTHROUGH: any string starting with "<block>" is returned VERBATIM as the
//                 verdict content. This lets the live-mode cases inject an exact
//                 verdict (e.g. "<block>yes</block><reason>[test-block] ...</reason>")
//                 without changing the keyword fallback.
//   The test driver writes the control file BEFORE each case.
//   GET /healthz                → readiness probe.
//   GET /count/classifier       → { count } of POSTs received (proves live egress).
//   GET /reset-classifier-count → resets the counter (called between cases).
//
// Both endpoints respect the request body's `stream` field: if true, respond
// with SSE chunks (data: {...}\n\n + data: [DONE]\n\n); if false, respond with
// a regular JSON chat-completion object.
//
// Zero external dependencies — uses only node:http. Runs under bun or node.

import http from "node:http";
import fs from "node:fs";

const AGENT_PORT = parseInt(process.env.AGENT_PORT || "8080", 10);
const CLASSIFIER_PORT = parseInt(process.env.CLASSIFIER_PORT || "8081", 10);
const VERDICT_FILE = process.env.VERDICT_FILE || "/tmp/classifier-verdict";
const READ_PATH = process.env.READ_PATH || "/workspace/target.txt";

// ── helpers ──────────────────────────────────────────────────────────────

function sendJson(res, status, obj) {
    const body = JSON.stringify(obj);
    res.writeHead(status, {
        "Content-Type": "application/json",
        "Content-Length": Buffer.byteLength(body),
    });
    res.end(body);
}

function sendSseError(res, status, msg) {
    res.writeHead(status, { "Content-Type": "application/json" });
    res.end(JSON.stringify({ error: { message: msg, type: "server_error" } }));
}

// Read the full request body as a string, then parse as JSON.
async function readJsonBody(req) {
    let raw = "";
    for await (const chunk of req) raw += chunk;
    try {
        return JSON.parse(raw);
    } catch {
        return {};
    }
}

function genId(prefix) {
    return prefix + "-" + Math.random().toString(36).slice(2, 10);
}

// ── agent tool-call response (streaming SSE) ─────────────────────────────
// Emits an OpenAI-style tool_call delta for the `read` tool.

function writeAgentToolCallStream(res) {
    res.writeHead(200, {
        "Content-Type": "text/event-stream",
        "Cache-Control": "no-cache",
        Connection: "keep-alive",
    });
    const id = genId("chatcmpl");
    const model = "mock-model";
    const base = { id, object: "chat.completion.chunk", model, choices: [] };

    // chunk 1: tool_call identity (name + empty args start)
    res.write("data: " + JSON.stringify({
        ...base,
        choices: [{
            index: 0,
            delta: {
                role: "assistant",
                tool_calls: [{
                    index: 0,
                    id: "call_1",
                    type: "function",
                    function: { name: "read", arguments: "" },
                }],
            },
            finish_reason: null,
        }],
    }) + "\n\n");

    // chunk 2: tool_call arguments (the full filePath)
    res.write("data: " + JSON.stringify({
        ...base,
        choices: [{
            index: 0,
            delta: {
                tool_calls: [{
                    index: 0,
                    function: {
                        arguments: JSON.stringify({ filePath: READ_PATH }),
                    },
                }],
            },
            finish_reason: null,
        }],
    }) + "\n\n");

    // chunk 3: finish
    res.write("data: " + JSON.stringify({
        ...base,
        choices: [{ index: 0, delta: {}, finish_reason: "tool_calls" }],
    }) + "\n\n");

    res.write("data: [DONE]\n\n");
    res.end();
}

// ── agent tool-call response (non-streaming JSON) ────────────────────────

function sendAgentToolCallJson(res) {
    sendJson(res, 200, {
        id: genId("chatcmpl"),
        object: "chat.completion",
        model: "mock-model",
        choices: [{
            index: 0,
            message: {
                role: "assistant",
                tool_calls: [{
                    id: "call_1",
                    type: "function",
                    function: {
                        name: "read",
                        arguments: JSON.stringify({ filePath: READ_PATH }),
                    },
                }],
            },
            finish_reason: "tool_calls",
        }],
    });
}

// ── agent text response (streaming SSE) ──────────────────────────────────

function writeAgentTextStream(res) {
    res.writeHead(200, {
        "Content-Type": "text/event-stream",
        "Cache-Control": "no-cache",
        Connection: "keep-alive",
    });
    const id = genId("chatcmpl");
    const model = "mock-model";
    const base = { id, object: "chat.completion.chunk", model, choices: [] };

    res.write("data: " + JSON.stringify({
        ...base,
        choices: [{ index: 0, delta: { role: "assistant", content: "" }, finish_reason: null }],
    }) + "\n\n");

    res.write("data: " + JSON.stringify({
        ...base,
        choices: [{ index: 0, delta: { content: "Done." }, finish_reason: null }],
    }) + "\n\n");

    res.write("data: " + JSON.stringify({
        ...base,
        choices: [{ index: 0, delta: {}, finish_reason: "stop" }],
    }) + "\n\n");

    res.write("data: [DONE]\n\n");
    res.end();
}

// ── agent text response (non-streaming JSON) ─────────────────────────────

function sendAgentTextJson(res) {
    sendJson(res, 200, {
        id: genId("chatcmpl"),
        object: "chat.completion",
        model: "mock-model",
        choices: [{
            index: 0,
            message: { role: "assistant", content: "Done." },
            finish_reason: "stop",
        }],
    });
}

// ── agent server (port 8080) ─────────────────────────────────────────────

let agentCallCount = 0;

const agentServer = http.createServer(async (req, res) => {
    req.on("error", () => {});
    res.on("error", () => {});
    if (res.socket) res.socket.on("error", () => {});

    const url = (req.url || "").split("?")[0];

    if (req.method === "GET" && url === "/healthz") {
        sendJson(res, 200, { ok: true, port: AGENT_PORT });
        return;
    }

    if (req.method === "GET" && url === "/reset") {
        agentCallCount = 0;
        sendJson(res, 200, { ok: true, count: agentCallCount });
        return;
    }

    if (req.method === "POST" && url === "/v1/chat/completions") {
        const body = await readJsonBody(req);
        const wantsStream = body.stream === true;
        const hasTools = Array.isArray(body.tools) && body.tools.length > 0;
        console.error(`[mock-agent] POST call=${agentCallCount} stream=${wantsStream} tools=${hasTools} model=${body.model || "?"}`);

        if (hasTools) {
            // Agent call (has tool definitions). Stateful:
            //   1st → tool_call so opencode executes the read tool
            //   2nd+ → short text so the session reaches idle
            agentCallCount += 1;
            if (agentCallCount === 1) {
                if (wantsStream) writeAgentToolCallStream(res);
                else sendAgentToolCallJson(res);
            } else {
                if (wantsStream) writeAgentTextStream(res);
                else sendAgentTextJson(res);
            }
        } else {
            // Title-generation call (no tools). Return short text.
            if (wantsStream) writeAgentTextStream(res);
            else sendAgentTextJson(res);
        }
        return;
    }

    sendJson(res, 404, { error: "not found", path: url });
});

// ── classifier server (port 8081) ────────────────────────────────────────

function readVerdict() {
    try {
        const v = fs.readFileSync(VERDICT_FILE, "utf8").trim();
        return v;
    } catch {
        return "allow"; // fail-safe default
    }
}

function verdictContent(verdict) {
    // PASSTHROUGH: if the control file already carries a <block> tag, return it
    // verbatim. This lets the live-mode cases inject an EXACT verdict text
    // (including a test-specific reason) without changing the keyword fallback.
    if (typeof verdict === "string" && verdict.startsWith("<block>")) {
        return verdict;
    }
    if (verdict === "block") {
        return "<block>yes</block><reason>scope creep</reason>";
    }
    return "<block>no</block>";
}

// Per-process counter of classifier POSTs. The live-mode cases query
// GET /count/classifier to PROVE the live classifier HTTP egress actually
// happened (not just the stub evaluator). Reset between cases via
// GET /reset-classifier-count.
let classifierCallCount = 0;

const classifierServer = http.createServer(async (req, res) => {
    req.on("error", () => {});
    res.on("error", () => {});
    if (res.socket) res.socket.on("error", () => {});

    const url = (req.url || "").split("?")[0];

    if (req.method === "GET" && url === "/healthz") {
        sendJson(res, 200, { ok: true, port: CLASSIFIER_PORT });
        return;
    }

    if (req.method === "GET" && url === "/count/classifier") {
        sendJson(res, 200, { ok: true, count: classifierCallCount });
        return;
    }

    if (req.method === "GET" && url === "/reset-classifier-count") {
        classifierCallCount = 0;
        sendJson(res, 200, { ok: true, count: classifierCallCount });
        return;
    }

    if (req.method === "POST" && url === "/v1/chat/completions") {
        // Count EVERY classifier POST so the live-mode driver can prove the
        // live classifier HTTP egress happened (count > 0 = the plugin really
        // called the endpoint, not just the stub path).
        classifierCallCount += 1;
        const verdict = readVerdict();
        const content = verdictContent(verdict);
        // The plugin always sends stream:false, so non-streaming JSON is the
        // primary path. Streaming support is included for robustness.
        const body = await readJsonBody(req);
        console.error(`[mock-classifier] POST call=${classifierCallCount} /v1/chat/completions verdict=${verdict} stream=${body.stream === true}`);
        if (body.stream === true) {
            res.writeHead(200, {
                "Content-Type": "text/event-stream",
                "Cache-Control": "no-cache",
                Connection: "keep-alive",
            });
            const id = genId("chatcmpl");
            const base = { id, object: "chat.completion.chunk", model: "mock-classifier", choices: [] };
            res.write("data: " + JSON.stringify({
                ...base,
                choices: [{ index: 0, delta: { role: "assistant", content }, finish_reason: null }],
            }) + "\n\n");
            res.write("data: " + JSON.stringify({
                ...base,
                choices: [{ index: 0, delta: {}, finish_reason: "stop" }],
            }) + "\n\n");
            res.write("data: [DONE]\n\n");
            res.end();
        } else {
            sendJson(res, 200, {
                id: genId("chatcmpl"),
                object: "chat.completion",
                model: "mock-classifier",
                choices: [{
                    index: 0,
                    message: { role: "assistant", content },
                    finish_reason: "stop",
                }],
            });
        }
        return;
    }

    sendJson(res, 404, { error: "not found", path: url });
});

// ── start both servers ───────────────────────────────────────────────────

agentServer.listen(AGENT_PORT, () => {
    console.log(`[mock-llm] agent server on :${AGENT_PORT}`);
});
classifierServer.listen(CLASSIFIER_PORT, () => {
    console.log(`[mock-llm] classifier server on :${CLASSIFIER_PORT}`);
});

process.on("SIGTERM", () => {
    agentServer.close(() => classifierServer.close(() => process.exit(0)));
});
process.on("SIGINT", () => {
    agentServer.close(() => classifierServer.close(() => process.exit(0)));
});
