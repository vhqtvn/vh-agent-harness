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
//   GET /reset          → resets the call counter + captured bodies (between cases).
//   GET /healthz        → readiness probe.
//   GET /count/agent    → { ok, count } of agent-model (tool-bearing) POSTs.
//   GET /agent-bodies   → { ok, count, bodies:[...] } of each captured
//                          tool-bearing request body (aligned with count, so
//                          bodies[1] is the 2nd agent call). Used by the
//                          per-call-gate continuation + feedback proof: a
//                          block case's 2nd request carries the rejection
//                          reason (CorrectedError errorText) as feedback to the
//                          model; an allow case's 2nd request carries the read
//                          result (file content).
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
const CLASSIFIER2_PORT = parseInt(process.env.CLASSIFIER2_PORT || "8082", 10);
const VERDICT_FILE = process.env.VERDICT_FILE || "/tmp/classifier-verdict";
const VERDICT_FILE_2 = process.env.VERDICT_FILE_2 || "/tmp/classifier-verdict-2";
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
// Captured request bodies for tool-bearing (agent-turn) POSTs. Index-aligned
// with agentCallCount: agentBodies[0] is the 1st agent call, [1] the 2nd. The
// per-call-gate proof inspects bodies[1]: under the per-call-gate the turn
// CONTINUES past a rejected tool call, so the model receives the rejection
// reason as errorText feedback in the 2nd request's messages. Under the old
// session kill-switch the turn died at 1 call and there was no 2nd body.
let agentBodies = [];

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
        agentBodies = [];
        sendJson(res, 200, { ok: true, count: agentCallCount });
        return;
    }

    if (req.method === "GET" && url === "/count/agent") {
        sendJson(res, 200, { ok: true, count: agentCallCount });
        return;
    }

    if (req.method === "GET" && url === "/agent-bodies") {
        sendJson(res, 200, {
            ok: true,
            count: agentBodies.length,
            bodies: agentBodies,
        });
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
            agentBodies.push(body);
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

// ── classifier server factory ─────────────────────────────────────────────
//
// A classifier server reads its OWN verdict control file and serves an
// OpenAI-compatible chat completion. Two instances run on two ports so the
// Phase 2 live-tiered consensus cases can give leaf-A and leaf-B DIFFERENT
// verdicts deterministically (each leaf points at its own endpoint/port).
//
// VERDICT CONTROL FILE — supports these shapes:
//   KEYWORD    : "allow"  → <block>no</block>
//                "block"  → <block>yes</block><reason>scope creep</reason>
//                "error"  → HTTP 500 (simulates a leaf transport/server failure
//                            for the consensus INCOMPLETE case)
//   PASSTHROUGH: any string starting with "<block>" is returned VERBATIM.
//
// COUNTER endpoints (per-port, same path on each instance):
//   GET /count/classifier       → { count } of POSTs received
//   GET /reset-classifier-count → resets the counter

function readVerdictFile(file) {
    try {
        return fs.readFileSync(file, "utf8").trim();
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

function makeClassifierServer(port, verdictFile) {
    let callCount = 0;
    const server = http.createServer(async (req, res) => {
        req.on("error", () => {});
        res.on("error", () => {});
        if (res.socket) res.socket.on("error", () => {});

        const url = (req.url || "").split("?")[0];

        if (req.method === "GET" && url === "/healthz") {
            sendJson(res, 200, { ok: true, port });
            return;
        }

        if (req.method === "GET" && url === "/count/classifier") {
            sendJson(res, 200, { ok: true, count: callCount });
            return;
        }

        if (req.method === "GET" && url === "/reset-classifier-count") {
            callCount = 0;
            sendJson(res, 200, { ok: true, count: callCount });
            return;
        }

        if (req.method === "POST" && url === "/v1/chat/completions") {
            callCount += 1;
            const verdict = readVerdictFile(verdictFile);
            // ERROR mode: simulate a leaf server failure (HTTP 500) so the
            // consensus INCOMPLETE case can exercise a FAIL outcome.
            if (verdict === "error") {
                const body = await readJsonBody(req);
                console.error(`[mock-classifier:${port}] POST call=${callCount} -> 500 (error mode) stream=${body.stream === true}`);
                sendJson(res, 500, {
                    error: { message: "mock classifier error mode", type: "server_error" },
                });
                return;
            }
            const content = verdictContent(verdict);
            const body = await readJsonBody(req);
            console.error(`[mock-classifier:${port}] POST call=${callCount} verdict=${verdict} stream=${body.stream === true}`);
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
    return { server, getCount: () => callCount };
}

const classifier1 = makeClassifierServer(CLASSIFIER_PORT, VERDICT_FILE);
const classifier2 = makeClassifierServer(CLASSIFIER2_PORT, VERDICT_FILE_2);
const classifierServer = classifier1.server;
const classifierServer2 = classifier2.server;

// ── start all servers ────────────────────────────────────────────────────

agentServer.listen(AGENT_PORT, () => {
    console.log(`[mock-llm] agent server on :${AGENT_PORT}`);
});
classifierServer.listen(CLASSIFIER_PORT, () => {
    console.log(`[mock-llm] classifier server on :${CLASSIFIER_PORT}`);
});
classifierServer2.listen(CLASSIFIER2_PORT, () => {
    console.log(`[mock-llm] classifier2 server on :${CLASSIFIER2_PORT}`);
});

process.on("SIGTERM", () => {
    agentServer.close(() =>
        classifierServer.close(() =>
            classifierServer2.close(() => process.exit(0)),
        ),
    );
});
process.on("SIGINT", () => {
    agentServer.close(() =>
        classifierServer.close(() =>
            classifierServer2.close(() => process.exit(0)),
        ),
    );
});
