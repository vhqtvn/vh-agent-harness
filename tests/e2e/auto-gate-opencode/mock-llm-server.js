// mock-llm-server.js — multi-port OpenAI-compatible mock for the real-runtime e2e.
//
// PURPOSE: serves FOUR endpoints inside the single container:
//   :8080  AGENT model endpoint   — drives the opencode agent loop
//   :8081  CLASSIFIER endpoint    — drives leaf A of the auto-gate plugin
//   :8082  CLASSIFIER2 endpoint   — drives leaf B (tiered consensus)
//   :8083  CLASSIFIER3 endpoint   — drives leaf C (3-leaf mixed rows)
//
// AGENT (:8080, POST /v1/chat/completions) — STATEFUL per process:
//   1st request → emits a tool_call so opencode executes the tool named by the
//                  AGENT TOOL control file (/tmp/agent-tool):
//                    "read"  (default) → executes the `read` tool (READ_PATH)
//                    "write"          → executes the `write` tool (WRITE_PATH +
//                                        WRITE_CONTENT) — for the blocked-Write
//                                        denial-path proof
//                  either triggers the permission.asked bus event our plugin
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
// CLASSIFIER (:8081/:8082/:8083, POST /v1/chat/completions) — each reads its
// OWN control file and supports these shapes:
//     KEYWORD   : "allow"       → returns <block>no</block>
//                 "block"       → returns <block>yes</block><reason>scope creep</reason>
//                 "error"       → HTTP 500 (simulates a leaf transport/server
//                                 failure for the consensus INCOMPLETE case)
//                 "unparseable" → HTTP 200 with content that has NO <block>
//                                 tag (exercises the parse-error deny path —
//                                 PASSTHROUGH cannot express this because it
//                                 requires a leading "<block>")
//                 "hang"        → NEVER responds (socket held open; the leaf's
//                                 own timeoutMs AbortError fires — the true
//                                 timeout path)
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
const CLASSIFIER3_PORT = parseInt(process.env.CLASSIFIER3_PORT || "8083", 10);
const VERDICT_FILE = process.env.VERDICT_FILE || "/tmp/classifier-verdict";
const VERDICT_FILE_2 = process.env.VERDICT_FILE_2 || "/tmp/classifier-verdict-2";
const VERDICT_FILE_3 = process.env.VERDICT_FILE_3 || "/tmp/classifier-verdict-3";
const READ_PATH = process.env.READ_PATH || "/workspace/target.txt";
// Agent tool control: "read" (default) or "write". The driver flips this to
// "write" for the blocked-Write denial-path case and back afterwards.
const AGENT_TOOL_FILE = process.env.AGENT_TOOL_FILE || "/tmp/agent-tool";
const WRITE_PATH = process.env.WRITE_PATH || "/workspace/write-target.txt";
const WRITE_CONTENT =
    process.env.WRITE_CONTENT || "written-target-content-SENTINEL";
// Optional per-case Write payload control file. When non-empty, its contents
// replace WRITE_CONTENT for the agent's write tool_call args. Used by the
// serve-write-corpus case to emulate a long agent-authored guidance-prose
// Write (the v0.27.0 incident shape) without hardcoding kilobytes of text.
const WRITE_CONTENT_FILE = process.env.WRITE_CONTENT_FILE || "/tmp/write-content";

function effectiveWriteContent() {
    try {
        const v = fs.readFileSync(WRITE_CONTENT_FILE, "utf8");
        if (v.length > 0) return v;
    } catch {
        // no control file — fall through to the sentinel default
    }
    return WRITE_CONTENT;
}

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
// Emits an OpenAI-style tool_call delta for the requested tool (`read` by
// default; `write` when the AGENT TOOL control file says so — used by the
// blocked-Write denial-path case). `argsJson` is the JSON string of the tool
// arguments.

function readAgentTool() {
    try {
        const v = fs.readFileSync(AGENT_TOOL_FILE, "utf8").trim();
        return v === "write" ? "write" : "read";
    } catch {
        return "read"; // fail-safe default
    }
}

function agentToolArgs(tool) {
    if (tool === "write") {
        return JSON.stringify({ filePath: WRITE_PATH, content: effectiveWriteContent() });
    }
    return JSON.stringify({ filePath: READ_PATH });
}

function writeAgentToolCallStream(res, tool, argsJson) {
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
                    function: { name: tool, arguments: "" },
                }],
            },
            finish_reason: null,
        }],
    }) + "\n\n");

    // chunk 2: tool_call arguments (full JSON args for the tool)
    res.write("data: " + JSON.stringify({
        ...base,
        choices: [{
            index: 0,
            delta: {
                tool_calls: [{
                    index: 0,
                    function: {
                        arguments: argsJson,
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

function sendAgentToolCallJson(res, tool, argsJson) {
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
                        name: tool,
                        arguments: argsJson,
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
            //   1st → tool_call so opencode executes the control-file tool
            //   2nd+ → short text so the session reaches idle
            agentCallCount += 1;
            agentBodies.push(body);
            if (agentCallCount === 1) {
                const tool = readAgentTool();
                const argsJson = agentToolArgs(tool);
                if (wantsStream) writeAgentToolCallStream(res, tool, argsJson);
                else sendAgentToolCallJson(res, tool, argsJson);
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
// OpenAI-compatible chat completion. Three instances run on three ports so the
// Phase 2 live-tiered consensus cases can give each leaf a DIFFERENT verdict
// deterministically (each leaf points at its own endpoint/port).
//
// VERDICT CONTROL FILE — supports these shapes:
//   KEYWORD    : "allow"       → <block>no</block>
//                "block"       → <block>yes</block><reason>scope creep</reason>
//                "error"       → HTTP 500 (simulates a leaf transport/server
//                                failure for the consensus INCOMPLETE case)
//                "unparseable" → HTTP 200 with NO <block> tag in the content
//                                (exercises the parse-error deny path)
//                "hang"        → NEVER responds (socket held open; the leaf's
//                                timeoutMs AbortError fires — the true timeout
//                                path)
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

// UNPARSEABLE_PAYLOAD — 200-content with NO anchored <block> tag. The text is
// a distinctive sentinel so the e2e can ALSO assert the raw classifier output
// never egresses into the surfaced deny string.
const UNPARSEABLE_PAYLOAD =
    "mock unparseable classifier payload with no verdict tag";

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
    if (verdict === "unparseable") {
        return UNPARSEABLE_PAYLOAD;
    }
    return "<block>no</block>";
}

function makeClassifierServer(port, verdictFile) {
    let callCount = 0;
    // Capture of the system-prompt text and the user-role transcript from the
    // most recent POST, so the driver can assert (a) the leaf actually
    // received the resolved classifier prompt (promptFile fixture marker) and
    // (b) what transcript evidence the leaf did/didn't see (dispatch marker
    // present, Write content absent under tool-input redaction).
    let lastPrompt = "";
    let lastTranscript = "";
    const server = http.createServer(async (req, res) => {
        req.on("error", () => {});
        res.on("error", () => {});
        if (res.socket) res.socket.on("error", () => {});

        const url = (req.url || "").split("?")[0];

        if (req.method === "GET" && url === "/healthz") {
            sendJson(res, 200, { ok: true, port });
            return;
        }

        if (req.method === "GET" && url === "/last-prompt") {
            sendJson(res, 200, { ok: true, lastPrompt, lastTranscript });
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
            // HANG mode: consume the request then NEVER respond. The leaf's
            // own AbortController (timeoutMs) fires client-side — the true
            // timeout path. The socket is simply left open.
            if (verdict === "hang") {
                await readJsonBody(req);
                console.error(`[mock-classifier:${port}] POST call=${callCount} -> HANG (no response; leaf timeout fires)`);
                return;
            }
            const content = verdictContent(verdict);
            const body = await readJsonBody(req);
            if (Array.isArray(body.messages)) {
                const sys = body.messages.find((m) => m && m.role === "system");
                if (sys && typeof sys.content === "string") lastPrompt = sys.content;
                const usr = body.messages.find((m) => m && m.role === "user");
                if (usr && typeof usr.content === "string") lastTranscript = usr.content;
            }
            console.error(`[mock-classifier:${port}] POST call=${callCount} verdict=${verdict === content ? verdict : verdict + "(passthrough)"} stream=${body.stream === true}`);
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
const classifier3 = makeClassifierServer(CLASSIFIER3_PORT, VERDICT_FILE_3);
const classifierServer = classifier1.server;
const classifierServer2 = classifier2.server;
const classifierServer3 = classifier3.server;

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
classifierServer3.listen(CLASSIFIER3_PORT, () => {
    console.log(`[mock-llm] classifier3 server on :${CLASSIFIER3_PORT}`);
});

function shutdown() {
    // Close listening sockets; a hung HANG-mode socket may keep a close
    // callback pending — closeIdleConnections() drops those, and the driver's
    // SIGKILL backstop covers anything left. Exit immediately after
    // unregistering (no need to wait for close callbacks).
    for (const s of [agentServer, classifierServer, classifierServer2, classifierServer3]) {
        if (typeof s.closeIdleConnections === "function") s.closeIdleConnections();
        s.close();
    }
    process.exit(0);
}
process.on("SIGTERM", shutdown);
process.on("SIGINT", shutdown);
