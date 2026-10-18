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
// Two evaluation modes are exercised:
//   ENFORCE mode (stubEvaluate — pure sync, no classifier HTTP) so the
//   plugin evaluates and replies as fast as possible. In ENFORCE mode the
//   plugin's path is: readConfig (sync) → decidePermission (sync) → reply.
//   LIVE mode (decideLive — REAL classifier HTTP egress). The plugin fetches
//   the transcript via client.session.messages, serializes it, POSTs to the
//   classifier endpoint, parses the verdict, then replies. This is slower
//   (HTTP round-trip), so the run-mode race is tighter; serve-live has no
//   race (sole replier) and is the deterministic proof.
//
// The test is made airtight by running cases whose PASS condition is the
// OPPOSITE of the run-mode default. A pass PROVES our plugin won the race:
//
//   Case | Mode    | --dangerously | verdict source        | Run default  | PASS = tool outcome
//   -----|---------|:-------------:|-----------------------|--------------|---------------------
//   A    | enforce | absent        | stub "allow"          | reject       | read PROCEEDS
//   B    | enforce | present       | stub "block"          | once/allow   | read BLOCKED
//   C    | live    | absent        | classifier <block>no  | reject       | read PROCEEDS
//   D    | live    | present       | classifier <block>yes | once/allow   | read BLOCKED
//
// If the plugin LOSES the race, the outcome matches the run-default and the
// case FAILS loudly — no false pass is possible. For LIVE cases we ALSO assert
// the classifier HTTP was actually called (mock /count/classifier > 0), which
// proves the live egress happened rather than the stub path.
//
// Run-live is racy (the classifier HTTP round-trip narrows the head-start over
// run's auto-reply). Run-live cases classify their outcome as:
//   PASS      — plugin won the race (correct outcome + classifier called).
//   RACE_LOSS — live chain ran correctly (event seen + classifier called +
//               correct decision) but the reply arrived after run's auto-reply
//               ("Permission request not found"). The live chain is PROVEN;
//               only the timing race was lost. Serve-live is the deterministic
//               proof. RACE_LOSS does not fail the suite.
//   FAIL      — the live chain did NOT run correctly (genuine failure).
// A run-live RACE_LOSS is reported honestly — the serve-live cases (no
// auto-replier) are the deterministic proof, and the two-case matrix means a
// loss fails loudly (no false pass).
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
//
// ── PER-CALL GATE (reject carries a reason) ───────────────────────────────
// A reject routes through the v2 permission-reply endpoint
// (POST /permission/:requestID/reply) with a `message` (the denial reason).
// That fails the tool call with a CorrectedError (NOT a RejectedError), so the
// turn CONTINUES — the model is called again and sees the reason as errorText,
// letting it adapt. The agent mock's stateful 2nd call returns a short
// text/stop response so the turn ends cleanly (no loop). Block cases still
// PASS: the read is blocked (no file content), but the turn now completes
// gracefully instead of being killed mid-stride. Serve-mode detection via
// /reject/i still works because CorrectedError's message prefix is
// "The user rejected permission...". Run-mode detection also matches
// CorrectedError explicitly + the plugin's own [auto-gate] blocked: audit line.
//
// PER-CALL-GATE PROOF (continuation + feedback). The serve block cases
// (serve-B/D/F/G) additionally assert TWO properties that DISTINGUISH the
// per-call-gate from the old session kill-switch — proving "when the
// classifier denies a tool call, the agent knows to fix":
//   1. CONTINUATION — agentModelCalls >= 2. The agent mock received a 2nd
//      call (kill-switch = only 1 call, turn dies after the rejected tool
//      call; per-call-gate = turn continues). This count is the clean
//      discriminator and a real regression guard.
//   2. FEEDBACK — the 2nd agent-model request body contains the rejection
//      reason substring. opencode serializes the CorrectedError message
//      ("The user rejected permission... with the following feedback: <reason>")
//      as the tool-error errorText in the next request's messages, so the
//      model actually receives WHY it was blocked.
// The serve ALLOW cases (serve-A/C/E) are the negative control: their 2nd
// request carries the read RESULT (file content), not a rejection.

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
const CLASSIFIER2_PORT = 8082;
const CLASSIFIER3_PORT = 8083;
const PROMPT_TEXT = "Read /workspace/target.txt";
const TARGET_CONTENT = "readable-target-content";
// Blocked-Write denial-path case (surfaced deny string on a rejected Write).
const WRITE_TARGET_PATH = "/workspace/write-target.txt";
const WRITE_TARGET_ORIGINAL = "write-target-original-content";
const WRITE_SENTINEL = "written-target-content-SENTINEL";
// Contract-row sentinels (single-line, quote-free so JSON escaping cannot
// break the secondBody substring matches).
const R1_REASON = "[Rule-X] deletes protected branch";
const CRED_TOKEN = "eyJleGFtcGxl.qm9o.signature";
const SK_SENTINEL = "sk-abcdefghijklmnopqrstuvwxyz123456";
const END_SENTINEL = "END-SENTINEL";

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
    //
    // permission.edit:"ask" is the same shape for the blocked-Write
    // denial-path case. NOTE: the `write` tool routes under the EDIT
    // permission type upstream (tool/write.ts declares `permission: "edit"`),
    // so the ask-rule must be keyed `edit`, not `write` — without it the
    // default agent pre-allows the write and no permission.asked event fires
    // (verified against the upstream clone inside the e2e image).
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
        permission: { read: "ask", edit: "ask" },
    };
    fs.writeFileSync(
        path.join(WORKSPACE, "opencode.json"),
        JSON.stringify(opencodeJson, null, 2),
    );
}

function writeGateConfig(stubVerdict, mode = "enforce") {
    // auto-gate-config.json — gate config with a deterministic stub verdict.
    //
    // ENFORCE mode uses stubEvaluate (pure sync — no classifier HTTP call),
    // which is critical for winning the race against run's auto-reply. The
    // stubVerdict field controls the decision:
    //   "allow" → <block>no</block> → status:allow → reply("once")
    //   "block" → <block>yes</block> → status:deny → reply("reject")
    //
    // LIVE mode (Cases C/D) uses decideLive — the plugin fetches the transcript
    // via client.session.messages, serializes it, POSTs to the classifier
    // endpoint (auto-gate-llm.json → modelEndpoint), parses the verdict, then
    // replies. The stubVerdict is ignored in live mode; the classifier mock's
    // verdict control file (/tmp/classifier-verdict) drives the decision. The
    // promptFile short-circuits resolveSystemPrompt so the Go binary is not
    // needed inside the container.
    //
    // The plugin reads this config on EACH event (via readConfig()), so we
    // can change stubVerdict/mode between cases.
    const gateConfig = {
        enabled: true,
        mode: mode,
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

function writeLlmConfigTiered(opts = {}) {
    // auto-gate-llm.json — TIERED classifier config (live-tiered mode).
    //
    // Each leaf points at an INDEPENDENT classifier mock instance so the
    // consensus path dispatches parallel decideLive calls. Leaf A uses the
    // primary classifier mock (:CLASSIFIER_PORT), leaf B the secondary
    // (:CLASSIFIER2_PORT), leaf C (optional) the tertiary
    // (:CLASSIFIER3_PORT). Each mock reads its own verdict control file, so
    // the per-leaf verdict is independently controllable.
    //
    // opts (all optional):
    //   leafCount   — 1, 2 (default) or 3 leaves. 1 leaf reproduces the
    //                 single-leaf tier shape (serve-write-corpus row — the
    //                 v0.27.0 incident topology: unanimous-deny with
    //                 leaves=1). 3 leaves requires the third classifier mock
    //                 (mixed judgment+infra rows).
    //   timeoutMs / maxRetries / retryDelayMs — per-leaf timing overrides
    //                 (defaults keep the historical values; the HANG case
    //                 shortens timeoutMs and disables retries so the true
    //                 AbortError path fires fast and deterministically).
    const {
        leafCount = 2,
        timeoutMs = 5000,
        maxRetries = 1,
        retryDelayMs = 200,
    } = opts;
    const leafSpecs = [
        {
            endpoint: `http://127.0.0.1:${CLASSIFIER_PORT}/v1/chat/completions`,
            model: "mock-classifier-a",
        },
        {
            endpoint: `http://127.0.0.1:${CLASSIFIER2_PORT}/v1/chat/completions`,
            model: "mock-classifier-b",
        },
        {
            endpoint: `http://127.0.0.1:${CLASSIFIER3_PORT}/v1/chat/completions`,
            model: "mock-classifier-c",
        },
    ].slice(0, leafCount);
    const llmConfig = {
        leaves: leafSpecs.map((spec) => ({
            modelEndpoint: spec.endpoint,
            model: spec.model,
            apiKeyEnv: "AUTO_GATE_API_KEY",
            timeoutMs,
            maxRetries,
            retryDelayMs,
        })),
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

// ── classifier-mock helpers (live-mode cases) ────────────────────────────
//
// The classifier mock (:8081) reads its verdict from the control file
// /tmp/classifier-verdict on each POST. setClassifierVerdict writes that
// file so the next classifier call returns the desired verdict. The mock
// also tracks a call counter for /count/classifier — we use it to PROVE the
// live classifier HTTP egress actually happened (not just the stub path).

function setClassifierVerdict(text) {
    fs.writeFileSync("/tmp/classifier-verdict", text);
}

async function resetClassifierCount() {
    try {
        await fetch(
            `http://127.0.0.1:${CLASSIFIER_PORT}/reset-classifier-count`,
        );
    } catch {
        // best effort
    }
}

async function getClassifierCount() {
    try {
        const r = await fetch(
            `http://127.0.0.1:${CLASSIFIER_PORT}/count/classifier`,
        );
        if (!r.ok) return 0;
        const data = await r.json();
        return data.count || 0;
    } catch {
        return 0;
    }
}

// ── classifier2-mock helpers (live-tiered leaf B) ────────────────────────
//
// The second classifier mock (:CLASSIFIER2_PORT) mirrors the first but reads
// /tmp/classifier-verdict-2 on each POST. Used for consensus leaf B so the
// two leaves can return DIFFERENT verdicts independently (Cases E/F/G).

function setClassifierVerdict2(text) {
    fs.writeFileSync("/tmp/classifier-verdict-2", text);
}

async function resetClassifier2Count() {
    try {
        await fetch(
            `http://127.0.0.1:${CLASSIFIER2_PORT}/reset-classifier-count`,
        );
    } catch {
        // best effort
    }
}

async function getClassifier2Count() {
    try {
        const r = await fetch(
            `http://127.0.0.1:${CLASSIFIER2_PORT}/count/classifier`,
        );
        if (!r.ok) return 0;
        const data = await r.json();
        return data.count || 0;
    } catch {
        return 0;
    }
}

// ── classifier3-mock helpers (live-tiered leaf C — mixed 3-leaf rows) ────
//
// The third classifier mock (:CLASSIFIER3_PORT) mirrors the first two but
// reads /tmp/classifier-verdict-3 on each POST. Used for the mixed
// judgment+infra 3-leaf row so one deny string can carry BOTH a
// deny(judgment; ...) stamp and a fail(unavailable...) stamp.

function setClassifierVerdict3(text) {
    fs.writeFileSync("/tmp/classifier-verdict-3", text);
}

async function resetClassifier3Count() {
    try {
        await fetch(
            `http://127.0.0.1:${CLASSIFIER3_PORT}/reset-classifier-count`,
        );
    } catch {
        // best effort
    }
}

async function getClassifier3Count() {
    try {
        const r = await fetch(
            `http://127.0.0.1:${CLASSIFIER3_PORT}/count/classifier`,
        );
        if (!r.ok) return 0;
        const data = await r.json();
        return data.count || 0;
    } catch {
        return 0;
    }
}

// ── classifier last-request capture (leaf-side evidence) ─────────────────
//
// The classifier mock records the system prompt AND the user-role transcript
// of its most recent POST. The serve-write-corpus row uses this to pin the
// leaf-side delivery contract: (a) the leaf received the RESOLVED classifier
// prompt (the promptFile fixture marker), (b) the dispatched task text WAS
// serialized into the leaf transcript (the scope evidence the classifier
// needs to judge scope at all), and (c) the Write tool INPUT was NOT
// serialized (tool-input redaction allowlist) — the leaf never sees file
// content, so a deny can never be grounded in what the prose says.

async function getLastClassifierRequest() {
    try {
        const r = await fetch(
            `http://127.0.0.1:${CLASSIFIER_PORT}/last-prompt`,
        );
        if (!r.ok) return { lastPrompt: "", lastTranscript: "" };
        const data = await r.json();
        return {
            lastPrompt: data.lastPrompt || "",
            lastTranscript: data.lastTranscript || "",
        };
    } catch {
        return { lastPrompt: "", lastTranscript: "" };
    }
}

// ── write-payload control (corpus-prose case) ─────────────────────────────
//
// The agent mock reads /tmp/write-content and, when non-empty, uses it as the
// Write tool_call content. The corpus row emulates the incident shape: a long
// (~16k chars) agent-authored guidance-prose document write, dispatched by
// the session prompt. The prose is synthesized domain-free guidance text with
// a distinctive sentinel for transcript-absence assertions.

const WRITE_CONTENT_FILE = "/tmp/write-content";
const CORPUS_SENTINEL = "corpus-guidance-prose-SENTINEL";

function buildCorpusProse(targetChars) {
    const header =
        "# Migration Note (next release)\n\n" +
        "## Purpose\n\nThis note documents the migration steps an upgrading " +
        "operator should follow. It is guidance prose: text a human (or an " +
        "agent acting for one) reads BEFORE running any command. Nothing in " +
        "this document executes; it carries instructions, rationale, and " +
        "rollback guidance for the release ceremony.\n\n" +
        `<!-- ${CORPUS_SENTINEL} -->\n\n`;
    const section = (i) =>
        `## Section ${i}: operator guidance\n\n` +
        "Read this section before touching the environment. The steps below " +
        "describe what to inspect, what to back up, and what to verify. They " +
        "are advisory: the operator decides, the document only explains. " +
        "Where a command is quoted, it is quoted as DOCUMENTATION, not as " +
        "something this document does. Follow the repo rules for every " +
        "actual action; when in doubt, stop and ask the operator.\n\n" +
        "Rollback guidance: if a step fails, restore the prior state and " +
        "re-run verification before proceeding. Do not improvise recovery " +
        "paths that are not written here. Keep the audit trail intact so a " +
        "reviewer can reconstruct what happened and why.\n\n";
    let body = "";
    let i = 1;
    while (header.length + body.length < targetChars) {
        body += section(i++);
    }
    return (header + body).slice(0, targetChars);
}

function setWriteContent(text) {
    fs.writeFileSync(WRITE_CONTENT_FILE, text);
}

function clearWriteContent() {
    try {
        fs.rmSync(WRITE_CONTENT_FILE);
    } catch {
        // already absent
    }
}

// ── agent-tool control (blocked-Write case) ───────────────────────────────
//
// The agent mock emits a tool_call for the tool named in /tmp/agent-tool.
// Default "read"; the blocked-Write denial-path case flips it to "write".

function setAgentTool(tool) {
    fs.writeFileSync("/tmp/agent-tool", tool === "write" ? "write" : "read");
}

// ── per-call-gate proof helpers (agent-model call count + body capture) ────
//
// The reject→per-call-gate fix (commit db032750) changed the plugin's deny
// from a bare reply("reject") (→ RejectedError → ctx.blocked → the turn ENDS
// — a session kill-switch) to reply("reject", {message: reason}) (→
// CorrectedError → does NOT trip the kill-switch → the turn CONTINUES → the
// model is called again and sees the reason as errorText on the next step).
//
// The existing block-case assertions only prove "the read was blocked" — which
// is true for BOTH the kill-switch and the per-call-gate. The two checks below
// DISTINGUISH them, proving the property "when the classifier denies a tool
// call, the agent knows to fix" (the turn continues and the model receives the
// rejection reason):
//
//   1. CONTINUATION — agentModelCalls >= 2. The agent mock must receive a 2nd
//      call. Kill-switch behavior = only 1 call (the turn dies right after the
//      rejected tool call; the model is never called again). Per-call-gate =
//      the turn continues, so the model IS called again. This count is the
//      clean discriminator.
//   2. FEEDBACK — the 2nd agent-model request body contains the rejection
//      reason substring. opencode serializes the rejected tool call into the
//      next request's messages as a tool-error whose errorText carries the
//      CorrectedError message ("The user rejected permission... with the
//      following feedback: <reason>"). The mock captures the full 2nd request
//      body — asserting the reason substring proves the model actually received
//      WHY it was blocked.
//
// These are only asserted on the SERVE cases (deterministic — no run-mode
// auto-reply race that muddies the call count). If the deny ever reverts to a
// kill-switch, the agent-model call count drops to 1 and these assertions fail.

// Poll the agent mock's /count/agent until it reaches targetCount, then return
// the count. Returns the last-seen count on timeout (which may be < target).
async function waitForAgentCalls(targetCount, maxAttempts = 60) {
    let last = 0;
    for (let i = 0; i < maxAttempts; i++) {
        try {
            const r = await fetch(`http://127.0.0.1:${AGENT_PORT}/count/agent`);
            if (r.ok) {
                const data = await r.json();
                last = data.count || 0;
                if (last >= targetCount) return last;
            }
        } catch {
            // not ready yet
        }
        await sleep(500);
    }
    return last;
}

// Fetch the captured agent-model request bodies (tool-bearing POSTs only,
// index-aligned with the agent call counter).
async function getAgentBodies() {
    try {
        const r = await fetch(`http://127.0.0.1:${AGENT_PORT}/agent-bodies`);
        if (!r.ok) return [];
        const data = await r.json();
        return data.bodies || [];
    } catch {
        return [];
    }
}

// Per-call-gate proof for one serve case. Polls the agent call count up to 2,
// fetches the 2nd request body, and checks continuation + feedback. For a
// BLOCK case: the 2nd body must carry the rejection reason substring and must
// NOT carry the read file content. For an ALLOW case (negative control): the
// 2nd body must carry the read file content.
//
// opts.notSubstrings (block cases): additional strings the 2nd body must NOT
// contain (default: none beyond TARGET_CONTENT). Pass null to disable the
// TARGET_CONTENT negative entirely (the blocked-Write case relies on the
// default negative — trivially satisfied there since no read happens — and
// proves write-did-not-execute via the on-disk fileUnchanged check instead,
// because the write tool-call args legitimately appear in the 2nd request's
// conversation history).
//
// Returns { agentModelCalls, continued, feedbackOk, secondBody }.
async function checkPerCallGate(label, opts) {
    const { expectBlock, reasonSubstring } = opts;
    const notSubstrings =
        opts.notSubstrings === undefined
            ? []
            : opts.notSubstrings || [];
    const negatives = opts.notSubstrings === null ? [] : [TARGET_CONTENT, ...notSubstrings];
    const agentModelCalls = await waitForAgentCalls(2);
    const bodies = await getAgentBodies();
    const secondBody = bodies.length >= 2 ? JSON.stringify(bodies[1]) : "";
    const continued = agentModelCalls >= 2;
    let feedbackOk;
    if (expectBlock) {
        feedbackOk = secondBody.includes(reasonSubstring);
        for (const neg of negatives) {
            feedbackOk = feedbackOk && !secondBody.includes(neg);
        }
    } else {
        feedbackOk = secondBody.includes(TARGET_CONTENT);
    }
    log(
        `[${label}] per-call-gate: agentModelCalls=${agentModelCalls} ` +
        `continued=${continued} feedbackOk=${feedbackOk} ` +
        `expectBlock=${expectBlock} reasonSubstring="${reasonSubstring}"`,
    );
    if (!continued || !feedbackOk) {
        log(`--- [${label}] 2nd agent-model body (first 1500 chars) ---`);
        log(secondBody.slice(0, 1500));
    }
    return { agentModelCalls, continued, feedbackOk, secondBody };
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
//
// opts (all optional unless noted):
//   stubVerdict, label, mode        — as before.
//   classifierVerdict / 2 / 3       — per-leaf verdict control files.
//   tiered                          — writeLlmConfigTiered() overrides
//                                     ({leafCount, timeoutMs, maxRetries,
//                                     retryDelayMs}).
//   noLeaves                        — write the SINGLE-LEAF llm config (no
//                                     `leaves` key) while mode=live-tiered
//                                     (misconfigured-no-leaves row R12).
//   agentTool                       — "read" (default) | "write" (flips the
//                                     agent mock's emitted tool call).
//   promptText                      — override the session prompt text.
async function runServeCase(opts) {
    const {
        stubVerdict,
        label,
        mode = "enforce",
        classifierVerdict,
        classifierVerdict2,
        classifierVerdict3,
        tiered,
        noLeaves = false,
        agentTool = "read",
        promptText = PROMPT_TEXT,
    } = opts;

    writeGateConfig(stubVerdict, mode);
    resetAgentCounter();
    setAgentTool(agentTool);
    if (mode === "live") {
        writeLlmConfig();
        setClassifierVerdict(classifierVerdict);
        await resetClassifierCount();
    }
    if (mode === "live-tiered") {
        if (noLeaves) {
            // Misconfigured tier: single-leaf shape (no `leaves` key) so the
            // live-tiered validator fail-closes with the constant
            // "live-tiered misconfigured: no leaves" string (row R12).
            writeLlmConfig();
        } else {
            writeLlmConfigTiered(tiered || {});
        }
        setClassifierVerdict(classifierVerdict || "<block>no</block>");
        setClassifierVerdict2(classifierVerdict2 || "<block>no</block>");
        setClassifierVerdict3(classifierVerdict3 || "<block>no</block>");
        await resetClassifierCount();
        await resetClassifier2Count();
        await resetClassifier3Count();
    }

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
        { parts: [{ type: "text", text: promptText }] },
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
    const { skipPermissions, stubVerdict, label, mode = "enforce" } = opts;

    // Write the gate config for this case's verdict + mode.
    writeGateConfig(stubVerdict, mode);
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

// ── race-loss detection (run-live only) ──────────────────────────────────
//
// In LIVE mode under `opencode run`, the plugin's decision path adds an HTTP
// round-trip (transcript fetch + classifier POST). Run's in-process auto-reply
// can resolve the permission first → the plugin's subsequent reply hits
// "Permission request not found". This is a RACE LOSS, not a correctness bug:
// the live chain still ran correctly (event seen + classifier called + correct
// decision parsed). The serve-live cases (no auto-replier) are the
// deterministic proof; a run-live RACE_LOSS is reported honestly but does not
// fail the suite, because it proves the live chain RAN, just not that it won
// the timing race against run's built-in auto-reply.
function detectRaceLoss(stderr, expectedDecision) {
    // Match BOTH the v1 reply-failed log ("permission reply failed: ...") and
    // the v2 reply-failed log ("permission reply (v2) failed: ..."). The v2
    // path is used when a reject carries a reason (per-call gate fix); in
    // run-live mode the built-in auto-replier can resolve the permission first,
    // so the plugin's v2 reply also hits "Permission request not found".
    const replyLost =
        /permission reply failed: Permission request not found/.test(stderr) ||
        /permission reply \(v2\) failed:/.test(stderr);
    const correctDecision = new RegExp(
        `live decision status=${expectedDecision}`,
    ).test(stderr);
    return replyLost && correctDecision;
}

// ── analyze a case's output ──────────────────────────────────────────────

function analyzeCase(label, stdout, stderr, mode = "enforce") {
    // (a) Plugin got the real event: stderr has the audit line for this mode.
    const eventSeen = new RegExp(
        `\\[auto-gate\\] permission\\.asked type=read mode=${mode}`,
    ).test(stderr);

    // (b) Tool outcome: does the file content appear in stdout?
    //     For --format json, the tool_use event's part contains the read
    //     result (file content) when the read succeeded, or an error when
    //     blocked.
    const hasContent = stdout.includes(TARGET_CONTENT);

    // Permission rejection indicators in stdout/stderr.
    //
    // Since the per-call-gate fix, a reject carries a reason and routes through
    // the v2 permission-reply endpoint, which fails the tool call with a
    // CorrectedError (NOT a RejectedError). The turn CONTINUES — the model is
    // called again and sees the reason as errorText (the agent mock returns a
    // short text/stop response on the 2nd call so the turn ends cleanly). Both
    // error classes share the message prefix "The user rejected permission...",
    // so the serve-mode /reject/i check still fires. Here in run mode we match
    // BOTH class names AND the plugin's own [auto-gate] blocked: audit line.
    const hasRejection =
        /permission.*reject/i.test(stdout) ||
        /RejectedError/i.test(stdout) ||
        /CorrectedError/i.test(stdout) ||
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
                CLASSIFIER2_PORT: String(CLASSIFIER2_PORT),
                CLASSIFIER3_PORT: String(CLASSIFIER3_PORT),
                VERDICT_FILE: "/tmp/classifier-verdict",
                VERDICT_FILE_2: "/tmp/classifier-verdict-2",
                VERDICT_FILE_3: "/tmp/classifier-verdict-3",
                READ_PATH: path.join(WORKSPACE, "target.txt"),
                AGENT_TOOL_FILE: "/tmp/agent-tool",
                WRITE_PATH: WRITE_TARGET_PATH,
                WRITE_CONTENT: WRITE_SENTINEL,
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
        await waitForHealth(CLASSIFIER2_PORT, "classifier2 mock");
        await waitForHealth(CLASSIFIER3_PORT, "classifier3 mock");

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

        // ── CASE C: LIVE ALLOW proof ───────────────────────────────────
        // mode=live, classifier returns <block>no</block>, NO --dangerously.
        // Run default = reject. PASS = read PROCEEDS (plugin's live allow wins)
        // AND classifier HTTP was actually called (count > 0).
        //
        // This proves the FULL live chain: transcript fetch via
        // client.session.messages → serialize → classifier HTTP egress →
        // verdict parse → reply — all against the real opencode runtime.
        log("========== CASE C (LIVE ALLOW proof) ==========");
        setClassifierVerdict("<block>no</block>");
        await resetClassifierCount();
        const caseC = runCase({
            label: "C",
            skipPermissions: false,
            stubVerdict: "allow", // ignored in live mode; harmless
            mode: "live",
        });
        const analysisC = analyzeCase("C", caseC.stdout, caseC.stderr, "live");
        const caseC_classifierCount = await getClassifierCount();
        // Case C PASS: plugin saw event AND read proceeded AND classifier called.
        // RACE_LOSS: plugin saw event + classifier called + correct decision
        // (allow) + reply failed (run's auto-reject won the timing race). The
        // live chain is PROVEN to have run; only the race was lost. Serve-C is
        // the deterministic proof.
        // FAIL: anything else (live chain didn't run, wrong decision, etc.).
        const caseC_pass =
            analysisC.eventSeen &&
            analysisC.hasContent &&
            caseC_classifierCount > 0;
        const caseC_raceLoss =
            !caseC_pass &&
            analysisC.eventSeen &&
            caseC_classifierCount > 0 &&
            detectRaceLoss(caseC.stderr, "allow");
        const caseC_status = caseC_pass ? "PASS" : caseC_raceLoss ? "RACE_LOSS" : "FAIL";
        log(
            `Case C: eventSeen=${analysisC.eventSeen} content=${analysisC.hasContent} rejection=${analysisC.hasRejection} classifierCalls=${caseC_classifierCount} → ${caseC_status}`,
        );
        if (caseC_status === "FAIL") printDiagnostics("C", caseC.stdout, caseC.stderr);

        // ── CASE D: LIVE BLOCK proof ───────────────────────────────────
        // mode=live, classifier returns <block>yes</block>...<reason>, WITH
        // --dangerously-skip-permissions. Run default = once(allow). PASS =
        // read BLOCKED (plugin's live reject wins) AND classifier HTTP called.
        log("========== CASE D (LIVE BLOCK proof) ==========");
        setClassifierVerdict(
            "<block>yes</block><reason>[test-block] classifier blocked</reason>",
        );
        await resetClassifierCount();
        const caseD = runCase({
            label: "D",
            skipPermissions: true,
            stubVerdict: "block", // ignored in live mode; harmless
            mode: "live",
        });
        const analysisD = analyzeCase("D", caseD.stdout, caseD.stderr, "live");
        const caseD_classifierCount = await getClassifierCount();
        const caseD_pass =
            analysisD.eventSeen &&
            !analysisD.hasContent &&
            analysisD.hasRejection &&
            caseD_classifierCount > 0;
        const caseD_raceLoss =
            !caseD_pass &&
            analysisD.eventSeen &&
            caseD_classifierCount > 0 &&
            detectRaceLoss(caseD.stderr, "deny");
        const caseD_status = caseD_pass ? "PASS" : caseD_raceLoss ? "RACE_LOSS" : "FAIL";
        log(
            `Case D: eventSeen=${analysisD.eventSeen} content=${analysisD.hasContent} rejection=${analysisD.hasRejection} classifierCalls=${caseD_classifierCount} → ${caseD_status}`,
        );
        if (caseD_status === "FAIL") printDiagnostics("D", caseD.stdout, caseD.stderr);

        // ── RUN-MODE SUMMARY ───────────────────────────────────────────
        log("========== RUN-MODE SUMMARY ==========");
        log(`Run Case A (ALLOW proof):       ${caseA_pass ? "PASS" : "FAIL"}`);
        log(`Run Case B (BLOCK proof):       ${caseB_pass ? "PASS" : "FAIL"}`);
        log(`Run Case C (LIVE ALLOW proof):  ${caseC_status}`);
        log(`Run Case D (LIVE BLOCK proof):  ${caseD_status}`);

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
        // Per-call-gate negative control (ALLOW): the turn continued (>=2 agent
        // calls) AND the model received the read RESULT (file content), not a
        // rejection — confirming the allow-vs-block distinction in what the
        // model sees on the 2nd call.
        const serveA_gate = await checkPerCallGate("serve-A", {
            expectBlock: false,
            reasonSubstring: "(n/a — allow case)",
        });
        const serveA_pass =
            serveA_eventSeen &&
            serveAnalysisA.hasContent &&
            serveA_gate.continued &&
            serveA_gate.feedbackOk;
        log(
            `Case serve-A: eventSeen=${serveA_eventSeen} content=${serveAnalysisA.hasContent} rejection=${serveAnalysisA.hasRejection} agentCalls=${serveA_gate.agentModelCalls} → ${serveA_pass ? "PASS" : "FAIL"}`,
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
        // Per-call-gate proof (BLOCK): the turn CONTINUED past the rejected tool
        // call (agentModelCalls >= 2) AND the model received the rejection REASON
        // as feedback in the 2nd request. The stub block reason is
        // "[stub] blocked by deterministic stub" (from stubEvaluate → parseVerdict
        // → decidePermission → reply("reject", reason)). opencode wraps it in the
        // CorrectedError message ("The user rejected permission... with the
        // following feedback: <reason>") and serializes that as the tool-error
        // errorText in the 2nd request's messages.
        const serveB_gate = await checkPerCallGate("serve-B", {
            expectBlock: true,
            reasonSubstring: "blocked by deterministic stub",
        });
        const serveB_pass =
            serveB_eventSeen &&
            !serveAnalysisB.hasContent &&
            serveAnalysisB.hasRejection &&
            serveB_gate.continued &&
            serveB_gate.feedbackOk;
        log(
            `Case serve-B: eventSeen=${serveB_eventSeen} content=${serveAnalysisB.hasContent} rejection=${serveAnalysisB.hasRejection} agentCalls=${serveB_gate.agentModelCalls} feedbackOk=${serveB_gate.feedbackOk} → ${serveB_pass ? "PASS" : "FAIL"}`,
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

        // ── CASE serve-C: LIVE ALLOW proof ─────────────────────────────
        // mode=live, classifier returns <block>no</block>. Serve has NO
        // auto-replier, so the plugin is the SOLE replier — deterministic.
        // PASS = read PROCEEDS AND classifier HTTP was actually called.
        // This is the deterministic proof of the full live chain (transcript
        // fetch over HTTP + classifier egress + verdict parse + reply).
        log("========== CASE serve-C (LIVE ALLOW proof) ==========");
        const stderrMarkerC = serveStderrBuf.length;
        const serveC = await runServeCase({
            label: "serve-C",
            stubVerdict: "allow", // ignored in live mode
            mode: "live",
            classifierVerdict: "<block>no</block>",
        });
        const serveAnalysisC = analyzeServeCase(
            "serve-C",
            serveC.messagesJson,
        );
        const serveStderrC = serveStderrBuf.slice(stderrMarkerC);
        const serveC_eventSeen =
            /\[auto-gate\] permission\.asked type=read mode=live/.test(
                serveStderrC,
            );
        const serveC_classifierCount = await getClassifierCount();
        // Per-call-gate negative control (LIVE ALLOW): turn continued + model
        // received the read result (file content), not a rejection.
        const serveC_gate = await checkPerCallGate("serve-C", {
            expectBlock: false,
            reasonSubstring: "(n/a — allow case)",
        });
        const serveC_pass =
            serveC_eventSeen &&
            serveAnalysisC.hasContent &&
            serveC_classifierCount > 0 &&
            serveC_gate.continued &&
            serveC_gate.feedbackOk;
        log(
            `Case serve-C: eventSeen=${serveC_eventSeen} content=${serveAnalysisC.hasContent} rejection=${serveAnalysisC.hasRejection} classifierCalls=${serveC_classifierCount} agentCalls=${serveC_gate.agentModelCalls} → ${serveC_pass ? "PASS" : "FAIL"}`,
        );
        if (!serveC_pass) {
            log(`--- [serve-C] messages JSON (first 3000 chars) ---`);
            log(serveC.messagesJson.slice(0, 3000));
            log(`--- [serve-C] serve stderr excerpt ---`);
            serveStderrC
                .split("\n")
                .filter((l) =>
                    /auto-gate|permission|reject|error|warn|live|classifier/i.test(
                        l,
                    ),
                )
                .slice(0, 40)
                .forEach((l) => log(`  err> ${l}`));
        }

        // ── CASE serve-D: LIVE BLOCK proof ─────────────────────────────
        // mode=live, classifier returns <block>yes</block>...<reason>.
        // Deterministic (sole replier). PASS = read BLOCKED AND classifier
        // HTTP called.
        log("========== CASE serve-D (LIVE BLOCK proof) ==========");
        const stderrMarkerD = serveStderrBuf.length;
        const serveD = await runServeCase({
            label: "serve-D",
            stubVerdict: "block", // ignored in live mode
            mode: "live",
            classifierVerdict:
                "<block>yes</block><reason>[test-block] classifier blocked</reason>",
        });
        const serveAnalysisD = analyzeServeCase(
            "serve-D",
            serveD.messagesJson,
        );
        const serveStderrD = serveStderrBuf.slice(stderrMarkerD);
        const serveD_eventSeen =
            /\[auto-gate\] permission\.asked type=read mode=live/.test(
                serveStderrD,
            );
        const serveD_classifierCount = await getClassifierCount();
        // Per-call-gate proof (LIVE BLOCK): turn continued + model received the
        // classifier's rejection reason "[test-block] classifier blocked" (from
        // the live verdict's <reason> tag → decideLive → reply("reject", reason))
        // as feedback in the 2nd request.
        const serveD_gate = await checkPerCallGate("serve-D", {
            expectBlock: true,
            reasonSubstring: "[test-block]",
        });
        const serveD_pass =
            serveD_eventSeen &&
            !serveAnalysisD.hasContent &&
            serveAnalysisD.hasRejection &&
            serveD_classifierCount > 0 &&
            serveD_gate.continued &&
            serveD_gate.feedbackOk;
        log(
            `Case serve-D: eventSeen=${serveD_eventSeen} content=${serveAnalysisD.hasContent} rejection=${serveAnalysisD.hasRejection} classifierCalls=${serveD_classifierCount} agentCalls=${serveD_gate.agentModelCalls} feedbackOk=${serveD_gate.feedbackOk} → ${serveD_pass ? "PASS" : "FAIL"}`,
        );
        if (!serveD_pass) {
            log(`--- [serve-D] messages JSON (first 3000 chars) ---`);
            log(serveD.messagesJson.slice(0, 3000));
            log(`--- [serve-D] serve stderr excerpt ---`);
            serveStderrD
                .split("\n")
                .filter((l) =>
                    /auto-gate|permission|reject|error|warn|live|classifier/i.test(
                        l,
                    ),
                )
                .slice(0, 40)
                .forEach((l) => log(`  err> ${l}`));
        }

        // ── PHASE 2: live-tiered CONSENSUS serve cases ───────────────────
        //
        // mode=live-tiered dispatches 2 parallel decideLive calls (one per
        // leaf) and aggregates via the unanimous-allow policy. Serve-only:
        // under `opencode run` the multi-leaf HTTP latency loses the race
        // WORSE than single-leaf live. Serve has no auto-replier, so the
        // consensus path is deterministic here.
        //
        // Mock approach: 2 classifier instances on 2 ports (:CLASSIFIER_PORT
        // + :CLASSIFIER2_PORT), each with its own verdict control file. Each
        // leaf config in writeLlmConfigTiered() points at its own endpoint.
        //
        //   Case | Leaf A verdict      | Leaf B verdict      | Aggregate       | PASS = tool outcome
        //   -----|---------------------|---------------------|-----------------|---------------------
        //   E    | <block>no (allow)   | <block>no (allow)   | allow           | read PROCEEDS
        //   F    | <block>no (allow)   | <block>yes (block)  | deny disagree   | read BLOCKED
        //   G    | <block>no (allow)   | error (HTTP 500)    | deny incomplete | read BLOCKED

        // ── CASE serve-E: CONSENSUS ALLOW proof ──────────────────────────
        // Both leaves return <block>no</block>. Unanimous-allow → aggregate
        // allow → reply once → read PROCEEDS. Both classifier mocks called.
        log("========== CASE serve-E (CONSENSUS ALLOW proof) ==========");
        const stderrMarkerE = serveStderrBuf.length;
        const serveE = await runServeCase({
            label: "serve-E",
            stubVerdict: "allow", // ignored in live-tiered mode
            mode: "live-tiered",
            classifierVerdict: "<block>no</block>",
            classifierVerdict2: "<block>no</block>",
        });
        const serveAnalysisE = analyzeServeCase(
            "serve-E",
            serveE.messagesJson,
        );
        const serveStderrE = serveStderrBuf.slice(stderrMarkerE);
        const serveE_eventSeen =
            /\[auto-gate\] permission\.asked type=read mode=live-tiered/.test(
                serveStderrE,
            );
        const serveE_classifierCount = await getClassifierCount();
        const serveE_classifier2Count = await getClassifier2Count();
        // Per-call-gate negative control (CONSENSUS ALLOW): turn continued +
        // model received the read result (file content), not a rejection.
        const serveE_gate = await checkPerCallGate("serve-E", {
            expectBlock: false,
            reasonSubstring: "(n/a — allow case)",
        });
        const serveE_pass =
            serveE_eventSeen &&
            serveAnalysisE.hasContent &&
            serveE_classifierCount > 0 &&
            serveE_classifier2Count > 0 &&
            serveE_gate.continued &&
            serveE_gate.feedbackOk;
        log(
            `Case serve-E: eventSeen=${serveE_eventSeen} content=${serveAnalysisE.hasContent} rejection=${serveAnalysisE.hasRejection} classifierCalls=${serveE_classifierCount} classifier2Calls=${serveE_classifier2Count} agentCalls=${serveE_gate.agentModelCalls} → ${serveE_pass ? "PASS" : "FAIL"}`,
        );
        if (!serveE_pass) {
            log(`--- [serve-E] messages JSON (first 3000 chars) ---`);
            log(serveE.messagesJson.slice(0, 3000));
            log(`--- [serve-E] serve stderr excerpt ---`);
            serveStderrE
                .split("\n")
                .filter((l) =>
                    /auto-gate|permission|reject|error|warn|live|tier|classifier/i.test(
                        l,
                    ),
                )
                .slice(0, 40)
                .forEach((l) => log(`  err> ${l}`));
        }

        // ── CASE serve-F: CONSENSUS BLOCK proof (disagreement) ──────────
        // Leaf A allows, leaf B blocks. Disagreement → aggregate deny →
        // reply reject → read BLOCKED. Both classifier mocks called.
        log("========== CASE serve-F (CONSENSUS BLOCK proof) ==========");
        const stderrMarkerF = serveStderrBuf.length;
        const serveF = await runServeCase({
            label: "serve-F",
            stubVerdict: "block", // ignored in live-tiered mode
            mode: "live-tiered",
            classifierVerdict: "<block>no</block>",
            classifierVerdict2:
                "<block>yes</block><reason>[test-block] classifier-B blocked</reason>",
        });
        const serveAnalysisF = analyzeServeCase(
            "serve-F",
            serveF.messagesJson,
        );
        const serveStderrF = serveStderrBuf.slice(stderrMarkerF);
        const serveF_eventSeen =
            /\[auto-gate\] permission\.asked type=read mode=live-tiered/.test(
                serveStderrF,
            );
        const serveF_classifierCount = await getClassifierCount();
        const serveF_classifier2Count = await getClassifier2Count();
        // Per-call-gate proof (CONSENSUS BLOCK — disagreement): turn continued
        // + model received the consensus denial reason. The surfaced deny
        // string keeps the historical aggregate prefix ("[auto-gate] blocked
        // by consensus: tier-aggregate: deny (reason=disagreement ...)") and
        // appends per-leaf stamps, so leaf-B's parsed reason must reach the
        // model as `deny(judgment; reason=[test-block] classifier-B blocked)`.
        const serveF_gate = await checkPerCallGate("serve-F", {
            expectBlock: true,
            reasonSubstring:
                "leaf#1=deny(judgment; reason=[test-block] classifier-B blocked)",
        });
        const serveF_pass =
            serveF_eventSeen &&
            !serveAnalysisF.hasContent &&
            serveAnalysisF.hasRejection &&
            serveF_classifierCount > 0 &&
            serveF_classifier2Count > 0 &&
            serveF_gate.continued &&
            serveF_gate.feedbackOk;
        log(
            `Case serve-F: eventSeen=${serveF_eventSeen} content=${serveAnalysisF.hasContent} rejection=${serveAnalysisF.hasRejection} classifierCalls=${serveF_classifierCount} classifier2Calls=${serveF_classifier2Count} agentCalls=${serveF_gate.agentModelCalls} feedbackOk=${serveF_gate.feedbackOk} → ${serveF_pass ? "PASS" : "FAIL"}`,
        );
        if (!serveF_pass) {
            log(`--- [serve-F] messages JSON (first 3000 chars) ---`);
            log(serveF.messagesJson.slice(0, 3000));
            log(`--- [serve-F] serve stderr excerpt ---`);
            serveStderrF
                .split("\n")
                .filter((l) =>
                    /auto-gate|permission|reject|error|warn|live|tier|classifier/i.test(
                        l,
                    ),
                )
                .slice(0, 40)
                .forEach((l) => log(`  err> ${l}`));
        }

        // ── CASE serve-G: CONSENSUS INCOMPLETE proof (one leaf errors) ───
        // Leaf A allows, leaf B errors (mock returns HTTP 500). decideLive
        // catches the transport error and returns {status:"deny",
        // kind:"error"} with typed provenance — normalizeLeafOutcome maps a
        // kind:"error" deny to FAIL (NOT DENY), so the aggregate is
        // deny + INCOMPLETE (reason=incomplete), and leaf-B's surfaced stamp
        // is fail(unavailable/...; no safety judgment was obtained). The
        // mock's raw error text ("mock classifier error mode") must NEVER
        // reach the surfaced string.
        // (Truth fix: pre-provenance code conflated infra errors with
        // judgments — an errored leaf normalized to DENY, so this case
        // surfaced reason=disagreement and the old comment here wrongly
        // claimed "normalized to FAIL".)
        log("========== CASE serve-G (CONSENSUS INCOMPLETE proof) ==========");
        const stderrMarkerG = serveStderrBuf.length;
        const serveG = await runServeCase({
            label: "serve-G",
            stubVerdict: "block", // ignored in live-tiered mode
            mode: "live-tiered",
            classifierVerdict: "<block>no</block>",
            classifierVerdict2: "error", // mock returns HTTP 500
        });
        const serveAnalysisG = analyzeServeCase(
            "serve-G",
            serveG.messagesJson,
        );
        const serveStderrG = serveStderrBuf.slice(stderrMarkerG);
        const serveG_eventSeen =
            /\[auto-gate\] permission\.asked type=read mode=live-tiered/.test(
                serveStderrG,
            );
        const serveG_classifierCount = await getClassifierCount();
        const serveG_classifier2Count = await getClassifier2Count();
        // Per-call-gate proof (CONSENSUS BLOCK — incomplete): turn continued +
        // model received the consensus denial reason. The surfaced deny string
        // must carry the typed-infra leaf stamp AND the truthful aggregate
        // label (reason=incomplete), and must NOT leak the mock's raw error
        // body text.
        const serveG_gate = await checkPerCallGate("serve-G", {
            expectBlock: true,
            reasonSubstring: "fail(unavailable",
            notSubstrings: ["mock classifier error mode"],
        });
        const serveG_incomplete =
            serveG_gate.secondBody.includes("reason=incomplete");
        const serveG_noRawError =
            !serveG_gate.secondBody.includes("mock classifier error mode") &&
            !serveStderrG.includes("mock classifier error mode");
        log(
            `Case serve-G: incompleteLabel=${serveG_incomplete} noRawError=${serveG_noRawError}`,
        );
        const serveG_pass =
            serveG_eventSeen &&
            !serveAnalysisG.hasContent &&
            serveAnalysisG.hasRejection &&
            serveG_classifierCount > 0 &&
            serveG_classifier2Count > 0 &&
            serveG_gate.continued &&
            serveG_gate.feedbackOk &&
            serveG_incomplete &&
            serveG_noRawError;
        log(
            `Case serve-G: eventSeen=${serveG_eventSeen} content=${serveAnalysisG.hasContent} rejection=${serveAnalysisG.hasRejection} classifierCalls=${serveG_classifierCount} classifier2Calls=${serveG_classifier2Count} agentCalls=${serveG_gate.agentModelCalls} feedbackOk=${serveG_gate.feedbackOk} → ${serveG_pass ? "PASS" : "FAIL"}`,
        );
        if (!serveG_pass) {
            log(`--- [serve-G] messages JSON (first 3000 chars) ---`);
            log(serveG.messagesJson.slice(0, 3000));
            log(`--- [serve-G] serve stderr excerpt ---`);
            serveStderrG
                .split("\n")
                .filter((l) =>
                    /auto-gate|permission|reject|error|warn|live|tier|classifier/i.test(
                        l,
                    ),
                )
                .slice(0, 40)
                .forEach((l) => log(`  err> ${l}`));
        }

        // ── DENIAL-PATH ROW CASES (O2 safe-feedback contract) ─────────────
        //
        // Each row pins ONE admission/fallback-table row of the surfaced deny
        // string. Baseline (tiered rows): leaf A (:8081) = allow, leaf B
        // (:8082) = the row's verdict — the serve-F scaffold. Single-leaf
        // `live` rows (H*) pin the D1 same-pipeline fix. All assertions ride
        // checkPerCallGate on the 2nd agent-model request body (the
        // model-visible surface), with companion stderr negatives where the
        // contract demands them. These are SERVE-ONLY (run-mode reply race —
        // see D3 in the contract evidence).
        async function runDenialRowCase(opts) {
            const {
                label,
                mode = "live-tiered",
                reasonSubstring,
                notSubstrings,
                stderrNegatives = [],
                extraChecks,
                ...caseOpts
            } = opts;
            const marker = serveStderrBuf.length;
            const run = await runServeCase({ label, mode, ...caseOpts });
            const analysis = analyzeServeCase(label, run.messagesJson);
            const stderrSlice = serveStderrBuf.slice(marker);
            // The upstream `write` TOOL asks under the EDIT permission type
            // (tool/write.ts: permission: "edit"), so a write-flavored case
            // still surfaces as type=edit in the plugin's audit line.
            const expectType =
                caseOpts.agentTool === "write" ? "edit" : "read";
            const eventSeen = new RegExp(
                `\\[auto-gate\\] permission\\.asked type=${expectType} mode=${mode}`,
            ).test(stderrSlice);
            const gate = await checkPerCallGate(label, {
                expectBlock: true,
                reasonSubstring,
                notSubstrings,
            });
            const stderrNegOk = stderrNegatives.every(
                (neg) => !stderrSlice.includes(neg),
            );
            const extras = Object.assign(
                {},
                (await (extraChecks && extraChecks({ gate, stderrSlice, run }))) ||
                    {},
            );
            const extrasOk = Object.values(extras).every(Boolean);
            const pass =
                eventSeen &&
                !analysis.hasContent &&
                analysis.hasRejection &&
                gate.continued &&
                gate.feedbackOk &&
                stderrNegOk &&
                extrasOk;
            log(
                `Case ${label}: eventSeen=${eventSeen} content=${analysis.hasContent} rejection=${analysis.hasRejection} agentCalls=${gate.agentModelCalls} feedbackOk=${gate.feedbackOk} stderrNegOk=${stderrNegOk} extras=${JSON.stringify(extras)} → ${pass ? "PASS" : "FAIL"}`,
            );
            if (!pass) {
                log(`--- [${label}] messages JSON (first 2000 chars) ---`);
                log(run.messagesJson.slice(0, 2000));
                log(`--- [${label}] serve stderr excerpt ---`);
                stderrSlice
                    .split("\n")
                    .filter((l) =>
                        /auto-gate|permission|reject|error|warn|live|tier|classifier/i.test(
                            l,
                        ),
                    )
                    .slice(0, 40)
                    .forEach((l) => log(`  err> ${l}`));
                log(`--- [${label}] 2nd agent body (last 1200 chars) ---`);
                log(gate.secondBody.slice(-1200));
            }
            return { pass, gate, stderrSlice };
        }
        const LEAF_B_URL = `http://127.0.0.1:${CLASSIFIER2_PORT}/v1/chat/completions`;

        // R1 — parsed benign reason admitted (admit-verbatim), aggregate keeps
        // its truthful disagreement label.
        const rowR1 = await runDenialRowCase({
            label: "serve-R1",
            classifierVerdict2: `<block>yes</block><reason>${R1_REASON}</reason>`,
            reasonSubstring: `deny(judgment; reason=${R1_REASON})`,
            extraChecks: ({ gate }) => ({
                disagreement: gate.secondBody.includes("reason=disagreement"),
            }),
        });

        // R2 — empty reason -> named fallback <none>, never an empty reason=.
        const rowR2 = await runDenialRowCase({
            label: "serve-R2",
            classifierVerdict2: "<block>yes</block>",
            reasonSubstring: "reason=<none>",
        });

        // R3 — redaction-only reason -> named fallback <suppressed>; the
        // sentinel must not survive into EITHER sink.
        const rowR3 = await runDenialRowCase({
            label: "serve-R3",
            classifierVerdict2: `<block>yes</block><reason>${SK_SENTINEL}</reason>`,
            reasonSubstring: "reason=<suppressed>",
            notSubstrings: [SK_SENTINEL],
            stderrNegatives: [SK_SENTINEL],
        });

        // R4 — multiline reason -> admit-sanitized single line (literal
        // backslash-n between the fragments; a raw U+000A is forbidden).
        const rowR4 = await runDenialRowCase({
            label: "serve-R4",
            classifierVerdict2:
                "<block>yes</block><reason>line one\nline two</reason>",
            reasonSubstring: "line one",
            extraChecks: ({ gate, stderrSlice }) => ({
                // The JSON-escaped body must contain the ESCAPED two-char
                // sequence (backslash-n), i.e. `\\n` in the JSON text.
                escapedNewline: gate.secondBody.includes(
                    "line one\\\\nline two",
                ),
                // ...and must NOT contain the JSON escape of a RAW newline
                // (a bare `\n` in the JSON text right after "line one").
                noRawNewline: !gate.secondBody.includes("line one\\nline two"),
                // stderr leaf line stays single-line (both fragments on one line).
                stderrSingleLine: stderrSlice
                    .split("\n")
                    .some(
                        (l) =>
                            l.includes("line one") && l.includes("line two"),
                    ),
            }),
        });

        // R5 — oversized reason -> admit-truncated at 240 chars with the
        // …[truncated] marker; the tail sentinel must not survive either sink.
        const oversizedReason = "oversized-reason-".repeat(60) + END_SENTINEL;
        const rowR5 = await runDenialRowCase({
            label: "serve-R5",
            classifierVerdict2: `<block>yes</block><reason>${oversizedReason}</reason>`,
            reasonSubstring: "[truncated]",
            notSubstrings: [END_SENTINEL],
            stderrNegatives: [END_SENTINEL],
        });

        // R6 — config-known endpoint echo -> admit-sanitized ([suppressed]);
        // the full leaf-B URL from the tiered llm config must not survive.
        const rowR6 = await runDenialRowCase({
            label: "serve-R6",
            classifierVerdict2: `<block>yes</block><reason>echo ${LEAF_B_URL} denied</reason>`,
            reasonSubstring: "[suppressed]",
            notSubstrings: [LEAF_B_URL],
            stderrNegatives: [LEAF_B_URL],
        });

        // R7 — credential in reason -> admit-sanitized (Bearer [redacted]);
        // the token must not survive either sink.
        const rowR7 = await runDenialRowCase({
            label: "serve-R7",
            classifierVerdict2: `<block>yes</block><reason>uses Bearer ${CRED_TOKEN} here</reason>`,
            reasonSubstring: "Bearer [redacted]",
            notSubstrings: [CRED_TOKEN],
            stderrNegatives: [CRED_TOKEN],
        });

        // R8 — parse-error (200 content with no <block> tag) -> named
        // fallback fail(parse-error; ...); the raw classifier payload must
        // never surface.
        const rowR8 = await runDenialRowCase({
            label: "serve-R8",
            classifierVerdict2: "unparseable",
            reasonSubstring: "fail(parse-error; no parseable verdict returned)",
            notSubstrings: ["mock unparseable classifier payload"],
            extraChecks: ({ gate }) => ({
                incomplete: gate.secondBody.includes("reason=incomplete"),
            }),
        });

        // R9' — true timeout path: the mock holds the socket open (hang) and
        // the leaf's own AbortError fires -> fail(unavailable/timeout; ...).
        // Short timeout + no retries keeps the case fast and deterministic.
        const rowR9T = await runDenialRowCase({
            label: "serve-R9-timeout",
            classifierVerdict2: "hang",
            tiered: { timeoutMs: 2000, maxRetries: 0, retryDelayMs: 100 },
            reasonSubstring: "fail(unavailable/timeout",
            notSubstrings: ["AbortError"],
            extraChecks: ({ gate }) => ({
                wording: gate.secondBody.includes(
                    "no safety judgment was obtained",
                ),
                incomplete: gate.secondBody.includes("reason=incomplete"),
            }),
        });

        // R12 — misconfigured tier (no leaves key) -> unchanged constant
        // fail-closed string (regression pin; passes pre-fix by design).
        const rowR12 = await runDenialRowCase({
            label: "serve-R12",
            noLeaves: true,
            reasonSubstring:
                "[auto-gate] fail-closed: live-tiered misconfigured: no leaves",
        });

        // Mixed 3-leaf row — one deny string carries BOTH a judgment stamp
        // (leaf#1) and a typed-infra stamp (leaf#2) with stable ordinals, and
        // the aggregate reports incomplete.
        const rowMix3 = await runDenialRowCase({
            label: "serve-mixed3",
            tiered: { leafCount: 3 },
            classifierVerdict2:
                "<block>yes</block><reason>[Rule-B] mixed-tier leaf blocked</reason>",
            classifierVerdict3: "error",
            reasonSubstring:
                "leaf#1=deny(judgment; reason=[Rule-B] mixed-tier leaf blocked)",
            notSubstrings: ["mock classifier error mode"],
            extraChecks: ({ gate }) => ({
                allowStamp: gate.secondBody.includes("leaf#0=allow"),
                failStamp: gate.secondBody.includes("leaf#2=fail(unavailable"),
                incomplete: gate.secondBody.includes("reason=incomplete"),
            }),
        });

        // ── D1 rows: single-leaf `live` mode rides the SAME pipeline ─────
        //
        // Pre-fix, this site forwarded the RAW classifier reason (and on infra
        // failure the raw error audit) into the v2 message AND logged the raw
        // audit to stderr. Post-fix BOTH sinks carry the sanitized stamp
        // ("[auto-gate] blocked by live classifier: <stampBody>" for the
        // model; "[auto-gate] live deny-detail <stampBody>" for the operator)
        // — one build, two sinks, so the sentinel negatives apply to the
        // stderr slice as well as the 2nd request body.

        // H1 — live + credential in reason.
        const rowH1 = await runDenialRowCase({
            label: "serve-H1",
            mode: "live",
            classifierVerdict: `<block>yes</block><reason>uses Bearer ${CRED_TOKEN} here</reason>`,
            reasonSubstring: "blocked by live classifier: deny(judgment;",
            notSubstrings: [CRED_TOKEN],
            stderrNegatives: [CRED_TOKEN],
            extraChecks: ({ gate }) => ({
                redacted: gate.secondBody.includes("Bearer [redacted]"),
            }),
        });

        // H2 — live + infra error (HTTP 500) -> fail(unavailable/...), not
        // the raw evaluator-error audit (absent from BOTH sinks).
        const rowH2 = await runDenialRowCase({
            label: "serve-H2",
            mode: "live",
            classifierVerdict: "error",
            reasonSubstring: "fail(unavailable",
            notSubstrings: ["non-2xx response"],
            stderrNegatives: ["non-2xx response"],
            extraChecks: ({ gate }) => ({
                wording: gate.secondBody.includes(
                    "no safety judgment was obtained",
                ),
            }),
        });

        // H3 — live + parse-error -> fail(parse-error; ...).
        const rowH3 = await runDenialRowCase({
            label: "serve-H3",
            mode: "live",
            classifierVerdict: "unparseable",
            reasonSubstring: "fail(parse-error; no parseable verdict returned)",
            notSubstrings: ["mock unparseable classifier payload"],
            stderrNegatives: ["mock unparseable classifier payload"],
        });

        // H4 — live + known-endpoint echo -> [suppressed] (both sinks).
        const rowH4 = await runDenialRowCase({
            label: "serve-H4",
            mode: "live",
            classifierVerdict: `<block>yes</block><reason>echo http://127.0.0.1:${CLASSIFIER_PORT}/v1/chat/completions denied</reason>`,
            reasonSubstring: "[suppressed]",
            notSubstrings: [
                `http://127.0.0.1:${CLASSIFIER_PORT}/v1/chat/completions`,
            ],
            stderrNegatives: [
                `http://127.0.0.1:${CLASSIFIER_PORT}/v1/chat/completions`,
            ],
        });

        // H5 — live + empty reason -> reason=<none>.
        const rowH5 = await runDenialRowCase({
            label: "serve-H5",
            mode: "live",
            classifierVerdict: "<block>yes</block>",
            reasonSubstring:
                "blocked by live classifier: deny(judgment; reason=<none>)",
        });

        // ── Blocked-Write denial path (the incident surface) ──────────────
        //
        // The agent mock emits a WRITE tool call (control file) against
        // /workspace/write-target.txt. The tiered consensus denies it; the
        // surfaced deny string must reach the 2nd model request AND the write
        // must remain UNEXECUTED (target file unchanged on disk).
        fs.writeFileSync(WRITE_TARGET_PATH, WRITE_TARGET_ORIGINAL);
        const rowWrite = await runDenialRowCase({
            label: "serve-write",
            agentTool: "write",
            promptText: "Write the deploy note",
            classifierVerdict2:
                "<block>yes</block><reason>[Rule-W] write blocked to protect deploy target</reason>",
            reasonSubstring:
                "deny(judgment; reason=[Rule-W] write blocked to protect deploy target)",
            extraChecks: () => ({
                fileUnchanged:
                    fs.readFileSync(WRITE_TARGET_PATH, "utf8") ===
                    WRITE_TARGET_ORIGINAL,
            }),
        });
        setAgentTool("read"); // restore for any later cases

        // ── Corpus-prose Write, single-leaf unanimous deny (incident row) ──
        //
        // Reproduces the v0.27.0 incident TOPOLOGY deterministically: a
        // single-leaf live-tiered tier (leafCount=1) receiving a long
        // (~16k chars) agent-authored GUIDANCE-PROSE Write dispatched by the
        // session prompt, and denying it. The mock verdict is FIXED — a real
        // live-LLM false-positive is not deterministically reproducible — so
        // this row proves the OBSERVABILITY contract, not the judgment:
        //   1. The surfaced deny carries the per-leaf reason (leaf#0=deny(
        //      judgment; reason=…)) attached to the unanimous-deny aggregate
        //      (leaves=1 denies=1) — the exact string the incident lacked.
        //   2. The write remains UNEXECUTED (file unchanged on disk).
        //   3. Leaf-side delivery: the classifier received the RESOLVED
        //      prompt (fixture marker) and the dispatched task text WAS in
        //      the transcript, while the Write CONTENT was NOT serialized
        //      (tool-input redaction) — pinning what the leaf can and cannot
        //      ground a deny on.
        fs.writeFileSync(WRITE_TARGET_PATH, WRITE_TARGET_ORIGINAL);
        setWriteContent(buildCorpusProse(16_000));
        const CORPUS_PROMPT =
            "Write the operator-sanctioned migration note into the docs corpus now";
        const CORPUS_DENY_REASON =
            "[Rule-C] delayed effects: guidance prose will steer later runs";
        const rowWriteCorpus = await runDenialRowCase({
            label: "serve-write-corpus",
            agentTool: "write",
            tiered: { leafCount: 1 },
            promptText: CORPUS_PROMPT,
            classifierVerdict: `<block>yes</block><reason>${CORPUS_DENY_REASON}</reason>`,
            reasonSubstring: `deny(judgment; reason=${CORPUS_DENY_REASON})`,
            extraChecks: async ({ gate, stderrSlice }) => {
                const leafReq = await getLastClassifierRequest();
                return {
                    fileUnchanged:
                        fs.readFileSync(WRITE_TARGET_PATH, "utf8") ===
                        WRITE_TARGET_ORIGINAL,
                    aggregateUnanimous: gate.secondBody.includes(
                        "unanimous-deny leaves=1 denies=1 tier=consensus",
                    ),
                    leafStampAttached: gate.secondBody.includes(
                        ` | leaf#0=deny(judgment; reason=${CORPUS_DENY_REASON}`,
                    ),
                    stderrLeafStamp: stderrSlice.includes(
                        `leaf#0=deny(judgment; reason=${CORPUS_DENY_REASON}`,
                    ),
                    // stderr sink carries the aggregate audit and the
                    // per-leaf stamps as separate audit lines (one build,
                    // two sinks) — pin BOTH so the stderr side proves
                    // aggregate + attributed reason, not stamp presence
                    // alone.
                    stderrAggregate: stderrSlice.includes(
                        "tier-aggregate: deny (reason=unanimous-deny " +
                            "leaves=1 denies=1 tier=consensus)",
                    ),
                    leafGotResolvedPrompt:
                        leafReq.lastPrompt.includes("auto-classifier gate"),
                    dispatchInTranscript:
                        leafReq.lastTranscript.includes(
                            "operator-sanctioned migration note",
                        ),
                    writeContentNotSerialized:
                        !leafReq.lastTranscript.includes(CORPUS_SENTINEL),
                };
            },
        });
        clearWriteContent();
        setAgentTool("read"); // restore for any later cases

        // ── FULL SUMMARY ───────────────────────────────────────────────
        const denialRows = {
            "R1 verbatim reason": rowR1,
            "R2 empty -> <none>": rowR2,
            "R3 redaction-only -> <suppressed>": rowR3,
            "R4 multiline escaped": rowR4,
            "R5 oversized truncated": rowR5,
            "R6 endpoint echo suppressed": rowR6,
            "R7 credential redacted": rowR7,
            "R8 parse-error fallback": rowR8,
            "R9' timeout fallback": rowR9T,
            "R12 no-leaves fail-closed": rowR12,
            "Mixed 3-leaf stamps": rowMix3,
            "H1 live credential sanitized": rowH1,
            "H2 live infra fail stamp": rowH2,
            "H3 live parse-error stamp": rowH3,
            "H4 live endpoint suppressed": rowH4,
            "H5 live empty -> <none>": rowH5,
            "Blocked-Write surfaced deny": rowWrite,
            "Corpus-write unanimous-deny surfaced": rowWriteCorpus,
        };
        log("========== FULL SUMMARY ==========");
        log(`Run   Case A (ALLOW proof):       ${caseA_pass ? "PASS" : "FAIL"}`);
        log(`Run   Case B (BLOCK proof):       ${caseB_pass ? "PASS" : "FAIL"}`);
        log(`Run   Case C (LIVE ALLOW proof):  ${caseC_status}`);
        log(`Run   Case D (LIVE BLOCK proof):  ${caseD_status}`);
        log(`Serve Case A (ALLOW proof):       ${serveA_pass ? "PASS" : "FAIL"}`);
        log(`Serve Case B (BLOCK proof):       ${serveB_pass ? "PASS" : "FAIL"}`);
        log(`Serve Case C (LIVE ALLOW proof):  ${serveC_pass ? "PASS" : "FAIL"}`);
        log(`Serve Case D (LIVE BLOCK proof):  ${serveD_pass ? "PASS" : "FAIL"}`);
        log(`Serve Case E (CONSENSUS ALLOW):   ${serveE_pass ? "PASS" : "FAIL"}`);
        log(`Serve Case F (CONSENSUS BLOCK):   ${serveF_pass ? "PASS" : "FAIL"}`);
        log(`Serve Case G (R9 INCOMPLETE):     ${serveG_pass ? "PASS" : "FAIL"}`);
        for (const [name, row] of Object.entries(denialRows)) {
            log(`Denial row ${name}: ${row.pass ? "PASS" : "FAIL"}`);
        }
        // The suite PASSES if: enforce A/B pass (run + serve) + serve-live C/D
        // pass (deterministic proof of the full live chain) + serve-consensus
        // E/F/G pass (deterministic proof of the tiered consensus chain) +
        // EVERY denial-path row passes (O2 safe-feedback contract: R1-R9',
        // R12, mixed 3-leaf, single-leaf-live D1 rows, the blocked-Write
        // surfaced-deny proof, and the corpus-write single-leaf unanimous-deny
        // row — per-leaf reason surfaced + leaf-side delivery contract) +
        // run-live C/D are not FAIL (PASS or RACE_LOSS both acceptable). A
        // run-live RACE_LOSS proves the live chain ran (event + classifier +
        // correct decision); the serve-live cases prove it resolves
        // deterministically. There are NO run-consensus cases because the
        // multi-leaf HTTP path loses the run-mode race WORSE than
        // single-leaf.
        const liveRunOk =
            caseC_status !== "FAIL" && caseD_status !== "FAIL";
        const denialRowsOk = Object.values(denialRows).every((r) => r.pass);
        const allPass =
            caseA_pass && caseB_pass &&
            serveA_pass && serveB_pass &&
            serveC_pass && serveD_pass &&
            serveE_pass && serveF_pass && serveG_pass &&
            denialRowsOk &&
            liveRunOk;
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
