// auto-gate-live.js — live classifier substrate for the auto-gate (Phase 3b).
//
// This module holds the live-model path: the transcript serializer, the
// binary-served classifier system prompt (consumed via `vh-agent-harness
// sys-prompt`, no longer inlined here), and a provider-agnostic
// OpenAI-compatible HTTP completion adapter. It mirrors the auto-gate-verdict.js
// precedent (a pure decision/substrate module that the plugin imports, and that
// OpenCode tolerates as a non-plugin under .opencode/plugins/ because it does NOT
// export `server`).
//
// It is deliberately split from auto-gate-verdict.js so each module stays
// cohesive:
//   auto-gate-verdict.js = verdict PARSE + decision matrix (pure, model-agnostic,
//                          synchronous). Phase 2. Unchanged by Phase 3b.
//   auto-gate-live.js    = transcript SERIALIZE + system PROMPT (binary-served)
//                          + HTTP ADAPTER (the live-model substrate). Phase 3b+3c.
//
// The two compose at decideLive() (below), which is the single bridge the plugin
// calls for `mode:"live"`.
//
// Naming: all identifiers GENERIC (auto-gate-live / classifyLive / serializeTranscript).
// The upstream is referred to only as "the reference agent system" — never by
// product name. The system prompt (now served by the binary, not inlined here)
// is the harness's OWN domain-free wording; it was written from the STRUCTURE
// described in the source packet's prompt anatomy section, not copied or
// paraphrased from any proprietary bundle.
//
// DUAL-PURPOSE SELF-TEST: like auto-gate-verdict.js, running this file directly
// (`node auto-gate-live.js` or `node --test auto-gate-live.js`) executes the
// node:test suite at the bottom; importing it as a module runs NO tests. The
// __isMain guard uses an explicit __filename comparison so an accidental import
// cannot fire the suite.
//
// Design source (structure only): researches/sources/2026-07-10-auto-mode-classifier-source-packet.local.md
//   - §3 system-prompt anatomy (mirrored in STRUCTURE, original wording)
//   - §6 transcript serialization, text mode (User:/Assistant:/Tool: prefixes,
//     tool name + indented args; classifier sees tool INPUTS not results;
//     the LAST tool call is the action being judged)

import fs from "node:fs";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";
// Static imports (no top-level await) so the self-test registers synchronously.
import { test } from "node:test";
import { strict as assert } from "node:assert";

import { decidePermission } from "./auto-gate-verdict.js";
import { scrubCredentials, scrubTruncate } from "./auto-gate-scrub.js";

// Re-export scrubCredentials so this module keeps its public scrubber surface
// (and its self-test references it). The implementation lives in the shared
// auto-gate-scrub.js so the HTTP-egress path (here) and the audit/stderr-log
// path (auto-tool-gate.js) use the IDENTICAL scrubber with no drift.
export { scrubCredentials };

// ---------------------------------------------------------------------------
// Classifier system prompt — served by the binary via `vh-agent-harness sys-prompt`.
//
// The prompt text is NO LONGER inlined in this module. It lives in the binary as
// an embedded asset (templates/sys-prompts/auto-gate-classifier.md) and is served
// on demand by `vh-agent-harness sys-prompt auto-gate-classifier`. This keeps a
// single source of truth (the binary), lets an overlay or operator override it by
// rendering .opencode/sys-prompts/auto-gate-classifier.md, and removes a large
// text blob from the plugin source.
//
// CLASSIFIER_PROMPT_KEY is the named sys-prompt key this plugin consumes.
export const CLASSIFIER_PROMPT_KEY = "auto-gate-classifier";

// Memoized cache of the binary-served prompt. Read-once per process — a prompt
// changes only on a binary or overlay update, never mid-session. Tests reset this
// via __resetCachedBinaryPrompt; a custom opts.runner bypasses the cache entirely
// (so tests stay isolated).
let _cachedBinaryPrompt = null;

// defaultSpawnPromptRunner shells out SYNCHRONOUSLY to
// `vh-agent-harness sys-prompt <name>` and returns {ok, stdout, reason}.
// spawnSync is available in both Node and Bun runtimes. Synchronous (not async)
// because resolveSystemPrompt must be callable from the synchronous
// decidePermission evaluator path. On any failure (spawn error, non-zero exit,
// empty stdout) it returns ok:false with a reason — the caller decides whether
// to throw.
export function defaultSpawnPromptRunner(name) {
    let r;
    try {
        r = spawnSync("vh-agent-harness", ["sys-prompt", name], {
            encoding: "utf8",
        });
    } catch (e) {
        return {
            ok: false,
            stdout: "",
            reason: `spawn threw: ${(e && e.message) || String(e)}`,
        };
    }
    if (r.error) {
        return {
            ok: false,
            stdout: "",
            reason: `spawn failed: ${r.error.message || r.error}`,
        };
    }
    if (r.status !== 0) {
        const stderr = (r.stderr || "").trim();
        return {
            ok: false,
            stdout: "",
            reason: `non-zero exit ${r.status}${stderr ? `: ${stderr}` : ""}`,
        };
    }
    const stdout = r.stdout || "";
    if (stdout.length === 0) {
        return { ok: false, stdout: "", reason: "empty stdout" };
    }
    return { ok: true, stdout, reason: "" };
}

// ---------------------------------------------------------------------------
// Transcript serialization — pure (no I/O), testable.
//
// Input: the SDK transcript (Array<{info:{role}, parts:Array<Part>}>) + the
// current permission payload. Output: a single text string in "text mode":
// User: / Assistant: / Tool: prefixes, tool name + a SHORT redacted summary of
// args, the most recent permission request emphasized at the end.
//
// Redaction: we surface only the load-bearing IDENTIFYING fields of a tool's
// input (command / path / pattern / query / url / workdir), truncated — NEVER
// full file bodies, command outputs, edit diffs, or secret-shaped values. Tool
// RESULTS are intentionally omitted: per the source packet the classifier sees
// tool INPUTS, not results (results are large, untrusted, and lower-signal).
// ---------------------------------------------------------------------------

// Truncation limits for the serialized transcript. Bounded so the resulting
// user message stays well under typical model context budgets.
const TX_FIELD_LEN = 240; // identifying field value (command/path/pattern/...)
const TX_TEXT_LEN = 1200; // a single text part
const TX_MAX_MESSAGES = 40; // cap to the most recent N messages

// txScrub — scrub-then-truncate, delegating to the SHARED scrubber
// (auto-gate-scrub.js) so the HTTP-egress path (this module) and the
// audit/stderr-log egress path (auto-tool-gate.js) use the IDENTICAL scrubber
// with no drift. The shared scrubTruncate is scrubCredentials then truncate
// (secrets removed BEFORE truncation so a value split across the truncation
// boundary cannot survive in part). Used for the text / reasoning /
// delegation-description fields in serializeTranscript, the allowlisted
// tool-input fields in redactToolInput, AND the permission `pattern` field —
// every field that lands in the egress payload to the external model passes
// through txScrub so no secret-shaped value can egress.
const txScrub = scrubTruncate;

// Redacted summary of a tool's input args. Mirrors the plugin's summarizeArgs
// identifying-field allowlist: we emit ONLY known load-bearing keys (scrubbed +
// truncated) and NEVER dump the raw args object. Unknown tools get an arg-key
// count only. This is the same redaction posture as the audit line, so the
// model sees the same redacted signal an operator would see in the log.
//
// SECURITY: every field below passes through txScrub (scrubCredentials THEN
// truncate, via the shared scrubber), NOT truncate alone. All of these fields land in the egress
// payload that classifyLive POSTs to an external model endpoint, so a bearer
// token / DB connection string / cloud key embedded in a `command` or `pattern`
// MUST be scrubbed the same way as transcript text — otherwise an identical
// secret redacted in text would leak via a tool input. A bare `command`/`path`
// string CAN carry secrets (e.g. `curl -H "Authorization: Bearer ..."`), so
// scrubbing is mandatory, not optional.
function redactToolInput(input) {
    if (!input || typeof input !== "object") return "";
    const parts = [];
    if (typeof input.command === "string") {
        parts.push(`command=${txScrub(input.command, TX_FIELD_LEN)}`);
    }
    const fp = input.filePath ?? input.path;
    if (typeof fp === "string") {
        parts.push(`path=${txScrub(fp, TX_FIELD_LEN)}`);
    }
    if (typeof input.pattern === "string") {
        parts.push(`pattern=${txScrub(input.pattern, TX_FIELD_LEN)}`);
    }
    if (typeof input.query === "string") {
        parts.push(`query=${txScrub(input.query, TX_FIELD_LEN)}`);
    }
    if (typeof input.url === "string") {
        parts.push(`url=${txScrub(input.url, TX_FIELD_LEN)}`);
    }
    if (typeof input.workdir === "string") {
        parts.push(`workdir=${txScrub(input.workdir, TX_FIELD_LEN)}`);
    }
    if (parts.length === 0) {
        const keys = Object.keys(input);
        parts.push(`args=${keys.length}`);
    }
    return parts.join(" ");
}

function roleLabel(role) {
    if (role === "user") return "User";
    if (role === "assistant") return "Assistant";
    return role || "Unknown";
}

// serializeTranscript(messages, permission) -> string
//
// Pure. Never throws. An empty / non-array transcript yields a non-empty
// fallback string that still describes the permission payload (the action under
// evaluation), so the live path degrades gracefully when the transcript fetch
// fails: the model still gets the type+pattern to judge.
export function serializeTranscript(messages, permission) {
    const lines = [];
    const list = Array.isArray(messages) ? messages : [];
    // Bound prompt size: keep only the most recent messages.
    const trimmed = list.slice(-TX_MAX_MESSAGES);
    if (trimmed.length > 0) lines.push("<transcript>");
    for (const entry of trimmed) {
        if (!entry || typeof entry !== "object") continue;
        const role = roleLabel(entry.info && entry.info.role);
        const parts = Array.isArray(entry.parts) ? entry.parts : [];
        if (parts.length === 0) {
            lines.push(`${role}: (no content)`);
            continue;
        }
        for (const part of parts) {
            if (!part || typeof part !== "object") continue;
            switch (part.type) {
                case "text": {
                    const txt = typeof part.text === "string" ? part.text : "";
                    if (txt.length === 0) break;
                    lines.push(`${role}: ${txScrub(txt, TX_TEXT_LEN)}`);
                    break;
                }
                case "tool": {
                    const toolName =
                        typeof part.tool === "string" ? part.tool : "unknown";
                    const input =
                        part.state && part.state.input
                            ? part.state.input
                            : undefined;
                    const summary = redactToolInput(input);
                    lines.push(
                        summary
                            ? `Tool: ${toolName} ${summary}`
                            : `Tool: ${toolName}`,
                    );
                    break;
                }
                case "reasoning": {
                    // Assistant internal monologue: useful context for judging
                    // composite / scope actions, but heavily truncated and
                    // clearly marked so it is never mistaken for operator
                    // intent. Credential-scrubbed (txScrub) before truncation.
                    const txt = typeof part.text === "string" ? part.text : "";
                    if (txt.length === 0) break;
                    lines.push(
                        `Assistant: [reasoning] ${txScrub(txt, TX_FIELD_LEN)}`,
                    );
                    break;
                }
                case "agent":
                case "subtask": {
                    // Sub-agent delegation marker — the prompt cares about this.
                    // Description is credential-scrubbed (txScrub) in transit.
                    const name = part.name || part.agent || "sub-agent";
                    const desc = part.description || part.prompt || "";
                    lines.push(
                        `Assistant: [delegates to ${name}] ${txScrub(desc, TX_FIELD_LEN)}`,
                    );
                    break;
                }
                default:
                    // step-start / step-finish / snapshot / patch / retry /
                    // compaction / file: metadata noise — omitted to keep the
                    // prompt concise and free of large attachments.
                    break;
            }
        }
    }
    if (trimmed.length > 0) lines.push("</transcript>");

    // The action under evaluation — ALWAYS present, emphasized last. This is the
    // single most recent permission request, the thing being judged.
    // SECURITY: `pattern` is a command/path string that CAN carry secrets (e.g.
    // a Bearer header in a curl pattern, a connection string in a db command),
    // and it lands in the egress payload to the external model. It MUST pass
    // through txScrub (scrubCredentials then truncate), NOT truncate alone.
    // `type` is a fixed enum ("bash"/"edit"/...) with no secret risk, so it is
    // left as-is.
    const type = (permission && permission.type) || "unknown";
    const pattern = txScrub(
        (permission && permission.pattern) || "",
        TX_FIELD_LEN,
    );
    lines.push("=== ACTION UNDER EVALUATION ===");
    lines.push(`Permission request: type=${type} pattern=${pattern}`);
    return lines.join("\n");
}

// ---------------------------------------------------------------------------
// resolveSystemPrompt — choose the system prompt for the live call.
//
// Resolution order:
//   1. If config.promptFile is set and readable -> its contents (operator
//      escape-hatch; kept from Phase 3b).
//   2. Else -> shell out synchronously to `vh-agent-harness sys-prompt
//      auto-gate-classifier` and return its stdout, memoized read-once per
//      process (a prompt changes only on a binary/overlay update, never
//      mid-session).
//
// On a binary failure (spawn error, non-zero exit, empty stdout) this THROWS:
// the caller (classifyLive -> decideLive -> decidePermission) maps a thrown
// evaluator to DENY (fail-closed). A clear audit line is emitted to stderr
// first.
//
// opts.runner is injectable so tests exercise the shell-out path without a real
// `vh-agent-harness` invocation. When a custom runner is passed the memoization
// cache is bypassed (so each test is fully isolated). opts.readFileFn is
// injectable so tests exercise the promptFile override without touching the
// filesystem.
export function resolveSystemPrompt(config, opts = {}) {
    const readFileFn = opts.readFileFn || fs.readFileSync;
    const isTestRunner = !!opts.runner;
    const runner = opts.runner || defaultSpawnPromptRunner;

    // 1. Operator escape-hatch: explicit promptFile overrides everything.
    const pf = config && config.promptFile;
    if (typeof pf === "string" && pf.length > 0) {
        try {
            return readFileFn(pf, "utf8");
        } catch (_) {
            // Unreadable override file: fall through to the binary-served prompt.
            // A bad promptFile must NOT take down the permission hot path.
        }
    }

    // 2. Binary-served prompt (memoized on the production default path only).
    if (!isTestRunner && _cachedBinaryPrompt !== null) {
        return _cachedBinaryPrompt;
    }
    const result = runner(CLASSIFIER_PROMPT_KEY);
    if (!result.ok) {
        const reason = result.reason || "unknown";
        console.error(
            `[auto-gate] failed to load classifier prompt via sys-prompt: ${reason}`,
        );
        throw new Error(
            `failed to load classifier prompt via sys-prompt: ${reason}`,
        );
    }
    if (!isTestRunner) {
        _cachedBinaryPrompt = result.stdout;
    }
    return result.stdout;
}

// __resetCachedBinaryPrompt clears the memoized binary prompt. Test-only; used
// to keep resolveSystemPrompt tests isolated from each other.
export function __resetCachedBinaryPrompt() {
    _cachedBinaryPrompt = null;
}

// __setCachedBinaryPrompt sets the memoized binary prompt directly. Test-only;
// lets a memoization test prime the cache without spawning the real binary.
export function __setCachedBinaryPrompt(value) {
    _cachedBinaryPrompt = value;
}

// ---------------------------------------------------------------------------
// classifyLive — provider-agnostic OpenAI-compatible chat-completions adapter.
//
// Builds a standard OpenAI-compatible request against config.modelEndpoint (the
// FULL URL, e.g. https://api.provider.com/v1/chat/completions), with the system
// prompt + serialized transcript as the two messages. Returns the raw model text
// from choices[0].message.content — the SAME contract stubEvaluate satisfies, so
// it slots into decidePermission()'s evaluator slot.
//
// FAIL-CLOSED BY THROWING: every indeterminate path THROWS, because the caller
// (decideLive -> decidePermission) maps a thrown evaluator to deny. Throws on:
//   - missing modelEndpoint / model          (misconfigured)
//   - missing API key in the named env var   (no credentials)
//   - non-2xx HTTP status                     (provider error)
//   - malformed JSON body                     (provider error)
//   - missing choices[0].message.content      (provider error)
//   - fetch rejection / AbortError (timeout)  (transport / timeout)
//
// API key: read from process.env[config.apiKeyEnv] (default "AUTO_GATE_API_KEY")
// AT CALL TIME. The key NEVER goes in the config FILE (which may be committed) —
// only the env-var NAME goes in config; the value comes from env at call time.
//
// fetchFn is injectable (default globalThis.fetch) so tests never make real
// network calls. runnerFn is injectable so tests never spawn a real
// `vh-agent-harness` process to load the system prompt. The plugin runtime is
// Bun-based, so global fetch + AbortController are available.
export async function classifyLive(config, serializedInput, fetchFn, runnerFn) {
    const fetchImpl = fetchFn || globalThis.fetch;
    if (typeof fetchImpl !== "function") {
        throw new Error("no fetch implementation available");
    }
    const endpoint = config && config.modelEndpoint;
    const model = config && config.model;
    if (typeof endpoint !== "string" || endpoint.length === 0) {
        throw new Error("missing modelEndpoint");
    }
    if (typeof model !== "string" || model.length === 0) {
        throw new Error("missing model");
    }
    const apiKeyEnv =
        config && typeof config.apiKeyEnv === "string" && config.apiKeyEnv
            ? config.apiKeyEnv
            : "AUTO_GATE_API_KEY";
    const apiKey = process.env[apiKeyEnv];
    if (!apiKey || typeof apiKey !== "string") {
        throw new Error(`missing API key in env ${apiKeyEnv}`);
    }
    const timeoutMs =
        config && typeof config.timeoutMs === "number" && config.timeoutMs > 0
            ? config.timeoutMs
            : 8000;

    const prompt = resolveSystemPrompt(config, { runner: runnerFn });
    const body = {
        model,
        messages: [
            { role: "system", content: prompt },
            { role: "user", content: serializedInput },
        ],
        temperature: 1,
        max_tokens: 64,
        stream: false,
    };

    // AbortController + setTimeout gives a hard timeout. On abort the underlying
    // fetch rejects (AbortError) and classifyLive throws — caller fail-closes.
    const ac = new AbortController();
    const timer = setTimeout(() => ac.abort(), timeoutMs);
    let res;
    try {
        res = await fetchImpl(endpoint, {
            method: "POST",
            headers: {
                "Content-Type": "application/json",
                Authorization: `Bearer ${apiKey}`,
            },
            body: JSON.stringify(body),
            signal: ac.signal,
        });
    } finally {
        clearTimeout(timer);
    }

    if (
        !res ||
        typeof res.status !== "number" ||
        res.status < 200 ||
        res.status >= 300
    ) {
        throw new Error(
            `non-2xx response: ${res && res.status !== undefined ? res.status : "no-status"}`,
        );
    }

    let json;
    try {
        json = await res.json();
    } catch (e) {
        throw new Error(
            `malformed JSON response: ${(e && e.message) || String(e)}`,
        );
    }

    const content =
        json &&
        json.choices &&
        json.choices[0] &&
        json.choices[0].message &&
        json.choices[0].message.content;

    if (typeof content !== "string" || content.length === 0) {
        throw new Error("missing choices[0].message.content");
    }
    return content;
}

// ---------------------------------------------------------------------------
// decideLive — the live-path composition bridge the plugin calls for mode:"live".
//
// Why this exists: decidePermission() (verdict module) is SYNCHRONOUS — it calls
// the evaluator and treats the return as raw verdict text. classifyLive() is
// inherently ASYNC (HTTP). So we cannot hand classifyLive directly to
// decidePermission as a synchronous evaluator. Instead decideLive:
//   1. awaits classifyLive (capturing success text OR error),
//   2. hands the result to decidePermission via a SYNCHRONOUS evaluator closure
//      that replays the outcome — returning the raw text on success, or THROWING
//      the captured error on failure so decidePermission's fail-closed matrix
//      maps it to "evaluator error -> deny".
//
// Net effect: the existing fail-closed matrix applies unchanged. classifyLive
// success+<block>no</block> -> allow; success+<block>yes</block> -> deny;
// success+unparseable -> deny; any throw (timeout / non-2xx / malformed /
// misconfigured / no key) -> deny. Latency is measured end-to-end for the audit
// line.
//
// fetchFn is injectable so tests drive the whole matrix with a fake transport
// and never touch the network. runnerFn is injectable so tests never spawn a
// real `vh-agent-harness` to load the system prompt.
export async function decideLive(config, serializedInput, fetchFn, runnerFn) {
    let liveError = null;
    let rawText = null;
    const t0 = Date.now();
    try {
        rawText = await classifyLive(config, serializedInput, fetchFn, runnerFn);
    } catch (err) {
        liveError = err;
    }
    const latencyMs = Date.now() - t0;
    const result = decidePermission(config, () => {
        if (liveError) throw liveError;
        return rawText;
    });
    return { status: result.status, audit: result.audit, reason: result.reason, latencyMs };
}

// ===========================================================================
// DUAL-PURPOSE SELF-TEST.
// Run directly (`node auto-gate-live.js` or `node --test auto-gate-live.js`) to
// execute the suite. Import as a module -> NO tests run. Guard is an explicit
// __filename comparison so an accidental import cannot fire the suite.
// ===========================================================================
const __filename = fileURLToPath(import.meta.url);
const __isMain = path.resolve(process.argv[1] ?? "") === __filename;

if (__isMain) {
    // ===== serializeTranscript =====

    test("serialize: empty transcript -> non-empty fallback naming the permission", () => {
        const out = serializeTranscript([], { type: "bash", pattern: "rm -rf x" });
        assert.ok(out.length > 0, "fallback must be non-empty");
        assert.match(out, /ACTION UNDER EVALUATION/);
        assert.match(out, /type=bash/);
        assert.match(out, /pattern=rm -rf x/);
        assert.doesNotMatch(out, /<transcript>/); // no transcript block when empty
    });

    test("serialize: non-array transcript -> non-empty fallback", () => {
        const out = serializeTranscript(undefined, { type: "edit" });
        assert.match(out, /type=edit/);
    });

    test("serialize: multi-message -> User/Assistant/Tool prefixes", () => {
        const msgs = [
            {
                info: { role: "user" },
                parts: [{ type: "text", text: "please clean the tmp dir" }],
            },
            {
                info: { role: "assistant" },
                parts: [
                    { type: "text", text: "sure" },
                    {
                        type: "tool",
                        tool: "bash",
                        state: { input: { command: "rm -rf tmp/" } },
                    },
                ],
            },
        ];
        const out = serializeTranscript(msgs, { type: "bash", pattern: "rm -rf tmp/" });
        assert.match(out, /<transcript>/);
        assert.match(out, /User: please clean the tmp dir/);
        assert.match(out, /Assistant: sure/);
        assert.match(out, /Tool: bash command=rm -rf tmp\//);
        assert.match(out, /ACTION UNDER EVALUATION/);
        assert.match(out, /<\/transcript>/);
    });

    test("serialize: long values are redacted/truncated", () => {
        // Use a long NON-secret-shaped value (a repeated readable phrase) so
        // this exercises the truncation path specifically. A pure high-entropy
        // blob (e.g. "x".repeat(5000)) is now correctly REDACTED to [redacted]
        // by scrubCredentials before truncation — covered by the scrub-egress
        // tests below — so it no longer reaches the truncation marker path.
        const huge = "the quick brown fox jumps over the lazy dog. ".repeat(150);
        const msgs = [
            {
                info: { role: "assistant" },
                parts: [
                    {
                        type: "tool",
                        tool: "write",
                        state: { input: { filePath: huge } },
                    },
                    { type: "text", text: huge },
                ],
            },
        ];
        const out = serializeTranscript(msgs, { type: "write", pattern: huge });
        // The huge value must NOT appear verbatim anywhere in the output.
        assert.equal(out.includes(huge), false, "huge value must be truncated");
        // Truncation marker present (value is long-but-not-secret, so it is
        // truncated, not redacted).
        assert.match(out, /\.\.\./);
    });

    test("serialize: tool results (state.output) are omitted", () => {
        const msgs = [
            {
                info: { role: "assistant" },
                parts: [
                    {
                        type: "tool",
                        tool: "bash",
                        state: {
                            input: { command: "ls" },
                            output: "SECRET-LIVE-CREDenTIAL-VALUE",
                        },
                    },
                ],
            },
        ];
        const out = serializeTranscript(msgs, { type: "bash", pattern: "ls" });
        assert.equal(
            out.includes("SECRET-LIVE-CREDenTIAL-VALUE"),
            false,
            "tool output must not leak into the transcript",
        );
        assert.match(out, /Tool: bash command=ls/);
    });

    test("serialize: unknown tool input -> arg count, no raw dump", () => {
        const msgs = [
            {
                info: { role: "assistant" },
                parts: [
                    {
                        type: "tool",
                        tool: "weird",
                        state: { input: { a: 1, b: 2, secret: "shh" } },
                    },
                ],
            },
        ];
        const out = serializeTranscript(msgs, { type: "weird" });
        assert.match(out, /Tool: weird args=3/);
        assert.equal(out.includes("shh"), false, "raw arg values must not leak");
    });

    test("serialize: sub-agent delegation is marked", () => {
        const msgs = [
            {
                info: { role: "assistant" },
                parts: [
                    {
                        type: "agent",
                        name: "builder",
                        description: "implement the feature",
                    },
                ],
            },
        ];
        const out = serializeTranscript(msgs, { type: "task" });
        assert.match(out, /\[delegates to builder\] implement the feature/);
    });

    test("serialize: permission payload always appears", () => {
        const out = serializeTranscript(
            [{ info: { role: "user" }, parts: [{ type: "text", text: "hi" }] }],
            { type: "webfetch", pattern: "https://evil.example/x" },
        );
        assert.match(out, /type=webfetch/);
        assert.match(out, /pattern=https:\/\/evil\.example\/x/);
    });

    // ===== credential scrubbing (leak-regression for B-F1) =====
    //
    // In live mode the transcript travels to an external classifier endpoint.
    // Text / reasoning / delegation-description fields are credential-scrubbed
    // in transit (txScrub = scrubCredentials then txTruncate). These tests
    // place secret-shaped values in each field and assert the VALUE does not
    // survive into the serialized output — only the [redacted] placeholder
    // (and surrounding context) does. Tool inputs stay on redactToolInput.

    test("scrub: api_key value in text field is redacted", () => {
        const secret = "sk-abcdefghijklmnopqrstuvwxyz123456";
        const msgs = [
            {
                info: { role: "user" },
                parts: [
                    {
                        type: "text",
                        text: `here is my api_key=${secret} for you`,
                    },
                ],
            },
        ];
        const out = serializeTranscript(msgs, { type: "edit", pattern: "x" });
        assert.equal(
            out.includes(secret),
            false,
            "api_key value must not leak into the transcript",
        );
        assert.match(out, /api_key=\[redacted\]/);
    });

    test("scrub: Bearer jwt in reasoning field is redacted", () => {
        const jwt =
            "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.payload.signature";
        const msgs = [
            {
                info: { role: "assistant" },
                parts: [
                    {
                        type: "reasoning",
                        text: `I will use Bearer ${jwt} to authenticate`,
                    },
                ],
            },
        ];
        const out = serializeTranscript(msgs, { type: "edit", pattern: "x" });
        assert.equal(
            out.includes(jwt),
            false,
            "bearer token in reasoning must not leak",
        );
        assert.match(out, /Bearer \[redacted\]/);
    });

    test("scrub: bare 40-char hex blob in text is redacted", () => {
        const hex = "0123456789abcdef0123456789abcdef01234567";
        const msgs = [
            {
                info: { role: "user" },
                parts: [{ type: "text", text: `token blob ${hex} right here` }],
            },
        ];
        const out = serializeTranscript(msgs, { type: "edit", pattern: "x" });
        assert.equal(
            out.includes(hex),
            false,
            "hex blob must not leak into the transcript",
        );
        assert.match(out, /\[redacted\]/);
    });

    test("scrub: password value in text is redacted", () => {
        const secret = "hunter2supersecretvalue1234567890";
        const msgs = [
            {
                info: { role: "user" },
                parts: [
                    {
                        type: "text",
                        text: `login with password: ${secret} please`,
                    },
                ],
            },
        ];
        const out = serializeTranscript(msgs, { type: "edit", pattern: "x" });
        assert.equal(
            out.includes(secret),
            false,
            "password value must not leak into the transcript",
        );
        assert.match(out, /password=\[redacted\]/);
    });

    test("scrub: Authorization Bearer header in pasted tool output within text", () => {
        const token = "sk-deadbeefcafef00dbaadf00dcafebabe1234";
        const msgs = [
            {
                info: { role: "assistant" },
                parts: [
                    {
                        type: "text",
                        text: `ran curl with header Authorization: Bearer ${token} against the api`,
                    },
                ],
            },
        ];
        const out = serializeTranscript(msgs, { type: "bash", pattern: "curl" });
        assert.equal(
            out.includes(token),
            false,
            "bearer token in pasted header must not leak",
        );
        assert.match(out, /Bearer \[redacted\]/);
    });

    test("scrub: delegation description token is redacted", () => {
        const secret = "sk-zyxwvutsrqponmlkjihgfedcba987654";
        const msgs = [
            {
                info: { role: "assistant" },
                parts: [
                    {
                        type: "agent",
                        name: "builder",
                        description: `use token=${secret} for the deploy`,
                    },
                ],
            },
        ];
        const out = serializeTranscript(msgs, { type: "task", pattern: "x" });
        assert.equal(
            out.includes(secret),
            false,
            "token in delegation description must not leak",
        );
        assert.match(out, /token=\[redacted\]/);
    });

    test("scrub: scrubCredentials is a pure exported function", () => {
        // Direct sanity check on the exported scrubber (idempotent + pure).
        assert.equal(
            scrubCredentials("api_key=sk-abcdefghijklmnopqrstuvwxyz123456"),
            "api_key=[redacted]",
        );
        assert.equal(
            scrubCredentials("Bearer eyJ0b2tlbj4.signature"),
            "Bearer [redacted]",
        );
        assert.equal(scrubCredentials("no secrets here"), "no secrets here");
        assert.equal(scrubCredentials(123), "");
    });

    test("scrub: tool-input allowlist redaction still works (no regression)", () => {
        // Tool inputs use redactToolInput (allowlist), NOT scrubCredentials.
        // An unknown tool with a secret arg value still collapses to an arg
        // count with no raw value dumped.
        const msgs = [
            {
                info: { role: "assistant" },
                parts: [
                    {
                        type: "tool",
                        tool: "weird",
                        state: {
                            input: {
                                secret: "sk-leakshouldnotappear12345678",
                            },
                        },
                    },
                ],
            },
        ];
        const out = serializeTranscript(msgs, { type: "weird" });
        assert.match(out, /Tool: weird args=1/);
        assert.equal(
            out.includes("sk-leakshouldnotappear12345678"),
            false,
            "raw tool arg value must not leak",
        );
    });

    // ===== Egress-scrub regression for F1 (tool-input + permission.pattern) =====
    //
    // B-F1: the allowlisted tool-input fields (command/path/pattern/query/url/
    // workdir) and permission.pattern used txTruncate (truncate-only, NO
    // scrubbing), so a secret embedded in a judged `command` or `pattern`
    // egressed UN-SCRUBBED while the identical secret in transcript text would
    // be redacted. Now every egress field passes through txScrub. These tests
    // place secret-shaped values in BOTH a tool `command` AND the permission
    // `pattern` and assert the raw secret is ABSENT from the serialized
    // egress string — only [redacted] survives.

    test("scrub-egress: Bearer jwt in a tool command field is redacted", () => {
        // A Bearer <jwt> embedded in a judged `command` — the allowlisted
        // tool-input path. Before F1 this leaked verbatim (truncate-only).
        const jwt =
            "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.payload.signature";
        const msgs = [
            {
                info: { role: "assistant" },
                parts: [
                    {
                        type: "tool",
                        tool: "bash",
                        state: {
                            input: {
                                command: `curl -H "Authorization: Bearer ${jwt}" https://api.example/v1`,
                            },
                        },
                    },
                ],
            },
        ];
        const out = serializeTranscript(msgs, { type: "bash", pattern: "curl" });
        assert.equal(
            out.includes(jwt),
            false,
            "Bearer jwt in tool command must not leak into egress payload",
        );
        assert.match(out, /Bearer \[redacted\]/);
    });

    test("scrub-egress: api_key in permission pattern is redacted", () => {
        // A secret-shaped value in the permission `pattern` (the action under
        // evaluation). Before F1 this leaked verbatim (truncate-only).
        const secret = "sk-abcdefghij1234567890qrstuvwxyz";
        const out = serializeTranscript(
            [{ info: { role: "user" }, parts: [{ type: "text", text: "hi" }] }],
            { type: "bash", pattern: `export api_key=${secret} && deploy` },
        );
        assert.equal(
            out.includes(secret),
            false,
            "api_key value in permission pattern must not leak into egress payload",
        );
        assert.match(out, /api_key=\[redacted\]/);
    });

    test("scrub-egress: connection string in permission pattern is redacted", () => {
        // A connection-string-style secret in the permission `pattern`.
        // `password` key triggers the key=value scrubber rule.
        const secret = "supersecretpasswordvalue1234567890";
        const out = serializeTranscript(
            [{ info: { role: "user" }, parts: [{ type: "text", text: "hi" }] }],
            {
                type: "bash",
                pattern: `psql postgres://user:password=${secret}@db.example:5432/prod`,
            },
        );
        assert.equal(
            out.includes(secret),
            false,
            "password in connection string (permission pattern) must not leak",
        );
        assert.match(out, /password=\[redacted\]/);
    });

    test("scrub-egress: secret in tool command AND pattern both redacted in one payload", () => {
        // Combined: both leak vectors present in a single egress payload.
        const cmdToken =
            "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.payload.signature";
        const patKey = "AKIAABCDEFGHIJKLMNOP";
        const msgs = [
            {
                info: { role: "assistant" },
                parts: [
                    {
                        type: "tool",
                        tool: "bash",
                        state: {
                            input: {
                                command: `curl -H "Authorization: Bearer ${cmdToken}" https://api.example`,
                            },
                        },
                    },
                ],
            },
        ];
        const out = serializeTranscript(msgs, {
            type: "bash",
            pattern: `aws configure set aws_access_key_id ${patKey}`,
        });
        assert.equal(
            out.includes(cmdToken),
            false,
            "Bearer jwt in command must not leak",
        );
        assert.equal(
            out.includes(patKey),
            false,
            "AWS key in permission pattern must not leak",
        );
        assert.match(out, /Bearer \[redacted\]/);
        assert.match(out, /\[redacted\]/);
    });

    // ===== resolveSystemPrompt =====
    //
    // resolveSystemPrompt now shells out to `vh-agent-harness sys-prompt
    // auto-gate-classifier` when promptFile is unset. An injectable opts.runner
    // keeps these tests free of any real vh-agent-harness invocation, and a
    // custom runner bypasses the memoization cache so tests stay isolated.

    const FAKE_PROMPT = "FAKE PROMPT FROM BINARY";

    function fakeRunnerOk(stdout = FAKE_PROMPT) {
        return () => ({ ok: true, stdout, reason: "" });
    }

    function fakeRunnerFail(reason = "non-zero exit 1") {
        return () => ({ ok: false, stdout: "", reason });
    }

    test("resolveSystemPrompt: unset promptFile -> shells out + returns stdout", () => {
        __resetCachedBinaryPrompt();
        let calls = 0;
        let gotName = "";
        const runner = (name) => {
            calls++;
            gotName = name;
            return { ok: true, stdout: FAKE_PROMPT, reason: "" };
        };
        const out = resolveSystemPrompt({}, { runner });
        assert.equal(out, FAKE_PROMPT);
        assert.equal(calls, 1, "must shell out once when promptFile unset");
        assert.equal(gotName, CLASSIFIER_PROMPT_KEY);
    });

    test("resolveSystemPrompt: memoized on default path (no second shell-out)", () => {
        __resetCachedBinaryPrompt();
        // Prime the cache directly (simulates a prior production call). This is
        // the only way to test memoization without spawning the real binary,
        // since a custom opts.runner bypasses the cache by design.
        __setCachedBinaryPrompt("CACHED-SENTINEL-777");
        // Call with the DEFAULT runner (no opts.runner). The cache MUST
        // short-circuit so the real binary is never spawned. If it were, the
        // real prompt (not "CACHED-SENTINEL-777") would be returned.
        const out = resolveSystemPrompt({});
        assert.equal(out, "CACHED-SENTINEL-777");
        __resetCachedBinaryPrompt();
    });

    test("resolveSystemPrompt: set+readable promptFile -> override (no shell-out)", () => {
        __resetCachedBinaryPrompt();
        let runnerCalls = 0;
        const fakeRead = (p) => `CUSTOM PROMPT from ${p}`;
        const out = resolveSystemPrompt(
            { promptFile: "/x/y.txt" },
            { readFileFn: fakeRead, runner: () => { runnerCalls++; return { ok: true, stdout: "X", reason: "" }; } },
        );
        assert.equal(out, "CUSTOM PROMPT from /x/y.txt");
        assert.equal(runnerCalls, 0, "must not shell out when promptFile is readable");
    });

    test("resolveSystemPrompt: unreadable promptFile -> falls through to binary", () => {
        __resetCachedBinaryPrompt();
        const fakeRead = () => {
            throw new Error("ENOENT");
        };
        const out = resolveSystemPrompt(
            { promptFile: "/missing.txt" },
            { readFileFn: fakeRead, runner: fakeRunnerOk() },
        );
        assert.equal(out, FAKE_PROMPT);
    });

    test("resolveSystemPrompt: binary failure -> throws", () => {
        __resetCachedBinaryPrompt();
        assert.throws(
            () => resolveSystemPrompt({}, { runner: fakeRunnerFail("spawn failed: ENOENT") }),
            /failed to load classifier prompt via sys-prompt: spawn failed: ENOENT/,
        );
        __resetCachedBinaryPrompt();
    });

    // ===== classifyLive (fake fetch; NO real network) =====

    const GOOD_CONFIG = {
        modelEndpoint: "https://provider.example/v1/chat/completions",
        model: "test-model",
        apiKeyEnv: "TEST_GATE_KEY",
        timeoutMs: 5000,
    };

    function fakeFetchOk(content) {
        return async () => ({
            status: 200,
            json: async () => ({
                choices: [{ message: { content } }],
            }),
        });
    }

    // Save/restore the named env var around each classifyLive test so a key
    // left over from another test cannot leak in (and so the missing-key test
    // is deterministic).
    function withKey(fn) {
        return async () => {
            const name = GOOD_CONFIG.apiKeyEnv;
            const prev = process.env[name];
            process.env[name] = "test-key-value";
            try {
                await fn();
            } finally {
                if (prev === undefined) delete process.env[name];
                else process.env[name] = prev;
            }
        };
    }

    test("classifyLive: happy path returns choices[0].message.content", withKey(async () => {
        const out = await classifyLive(
            GOOD_CONFIG,
            "serialised input",
            fakeFetchOk("<block>no</block>"),
            fakeRunnerOk(),
        );
        assert.equal(out, "<block>no</block>");
    }));

    test("classifyLive: sends system + user messages, Bearer key, POST", withKey(async () => {
        let captured;
        const fake = async (url, init) => {
            captured = { url, init };
            return {
                status: 200,
                json: async () => ({
                    choices: [{ message: { content: "<block>no</block>" } }],
                }),
            };
        };
        await classifyLive(GOOD_CONFIG, "the input", fake, fakeRunnerOk());
        assert.equal(captured.url, GOOD_CONFIG.modelEndpoint);
        assert.equal(captured.init.method, "POST");
        assert.equal(
            captured.init.headers.Authorization,
            "Bearer test-key-value",
        );
        assert.equal(captured.init.headers["Content-Type"], "application/json");
        const body = JSON.parse(captured.init.body);
        assert.equal(body.model, "test-model");
        assert.equal(body.temperature, 1);
        assert.equal(body.max_tokens, 64);
        assert.equal(body.stream, false);
        assert.equal(body.messages.length, 2);
        assert.equal(body.messages[0].role, "system");
        assert.equal(body.messages[0].content, FAKE_PROMPT);
        assert.equal(body.messages[1].role, "user");
        assert.equal(body.messages[1].content, "the input");
    }));

    test("classifyLive: non-2xx -> throws", withKey(async () => {
        const fake = async () => ({ status: 500, json: async () => ({}) });
        await assert.rejects(
            () => classifyLive(GOOD_CONFIG, "x", fake, fakeRunnerOk()),
            /non-2xx response: 500/,
        );
    }));

    test("classifyLive: malformed JSON -> throws", withKey(async () => {
        const fake = async () => ({
            status: 200,
            json: async () => {
                throw new Error("bad json");
            },
        });
        await assert.rejects(
            () => classifyLive(GOOD_CONFIG, "x", fake, fakeRunnerOk()),
            /malformed JSON response/,
        );
    }));

    test("classifyLive: missing choices -> throws", withKey(async () => {
        const fake = async () => ({ status: 200, json: async () => ({}) });
        await assert.rejects(
            () => classifyLive(GOOD_CONFIG, "x", fake, fakeRunnerOk()),
            /missing choices\[0\]\.message\.content/,
        );
    }));

    test("classifyLive: empty content string -> throws", withKey(async () => {
        const fake = async () => ({
            status: 200,
            json: async () => ({
                choices: [{ message: { content: "" } }],
            }),
        });
        await assert.rejects(
            () => classifyLive(GOOD_CONFIG, "x", fake, fakeRunnerOk()),
            /missing choices\[0\]\.message\.content/,
        );
    }));

    test("classifyLive: timeout/abort (fetch rejects with AbortError) -> throws", withKey(async () => {
        const fake = async () => {
            const e = new Error("aborted");
            e.name = "AbortError";
            throw e;
        };
        await assert.rejects(
            () => classifyLive(GOOD_CONFIG, "x", fake, fakeRunnerOk()),
            /aborted/,
        );
    }));

    test("classifyLive: missing API key (env unset) -> throws", async () => {
        const name = GOOD_CONFIG.apiKeyEnv;
        const prev = process.env[name];
        delete process.env[name];
        try {
            await assert.rejects(
                () => classifyLive(GOOD_CONFIG, "x", fakeFetchOk("ok")),
                new RegExp(`missing API key in env ${name}`),
            );
        } finally {
            if (prev !== undefined) process.env[name] = prev;
        }
    });

    test("classifyLive: missing modelEndpoint -> throws", withKey(async () => {
        await assert.rejects(
            () => classifyLive({ ...GOOD_CONFIG, modelEndpoint: "" }, "x", fakeFetchOk("ok")),
            /missing modelEndpoint/,
        );
    }));

    test("classifyLive: missing model -> throws", withKey(async () => {
        await assert.rejects(
            () => classifyLive({ ...GOOD_CONFIG, model: "" }, "x", fakeFetchOk("ok")),
            /missing model/,
        );
    }));

    test("classifyLive: default apiKeyEnv is AUTO_GATE_API_KEY", async () => {
        const name = "AUTO_GATE_API_KEY";
        const prev = process.env[name];
        process.env[name] = "default-env-key";
        try {
            const out = await classifyLive(
                { modelEndpoint: "https://x", model: "m" }, // no apiKeyEnv field
                "x",
                fakeFetchOk("<block>no</block>"),
                fakeRunnerOk(),
            );
            assert.equal(out, "<block>no</block>");
        } finally {
            if (prev === undefined) delete process.env[name];
            else process.env[name] = prev;
        }
    });

    // ===== decideLive — fail-closed wiring (fake transport) =====

    test("decideLive: happy <block>no</block> -> allow", withKey(async () => {
        const r = await decideLive(GOOD_CONFIG, "input", fakeFetchOk("<block>no</block>"), fakeRunnerOk());
        assert.equal(r.status, "allow");
        assert.equal(r.audit, "");
        assert.ok(typeof r.latencyMs === "number");
    }));

    test("decideLive: happy <block>yes</block> -> deny with reason", withKey(async () => {
        const r = await decideLive(
            GOOD_CONFIG,
            "input",
            fakeFetchOk("<block>yes</block><reason>[scope-creep] too broad</reason>"),
            fakeRunnerOk(),
        );
        assert.equal(r.status, "deny");
        assert.match(r.audit, /blocked:/);
        assert.equal(r.reason, "[scope-creep] too broad");
    }));

    test("decideLive: adapter throws (timeout) -> deny (fail-closed)", withKey(async () => {
        const fake = async () => {
            const e = new Error("aborted");
            e.name = "AbortError";
            throw e;
        };
        const r = await decideLive(GOOD_CONFIG, "input", fake, fakeRunnerOk());
        assert.equal(r.status, "deny");
        assert.match(r.audit, /fail-closed: evaluator error/);
    }));

    test("decideLive: misconfigured (no modelEndpoint) -> deny (fail-closed)", withKey(async () => {
        const r = await decideLive(
            { ...GOOD_CONFIG, modelEndpoint: "" },
            "input",
            fakeFetchOk("<block>no</block>"),
        );
        assert.equal(r.status, "deny");
        assert.match(r.audit, /missing modelEndpoint/);
    }));

    test("decideLive: misconfigured (no model) -> deny (fail-closed)", withKey(async () => {
        const r = await decideLive(
            { ...GOOD_CONFIG, model: "" },
            "input",
            fakeFetchOk("<block>no</block>"),
        );
        assert.equal(r.status, "deny");
        assert.match(r.audit, /missing model/);
    }));

    test("decideLive: unparseable verdict -> deny (fail-closed)", withKey(async () => {
        const r = await decideLive(
            GOOD_CONFIG,
            "input",
            fakeFetchOk("no block tag here"),
            fakeRunnerOk(),
        );
        assert.equal(r.status, "deny");
        assert.equal(r.audit, "fail-closed: unparseable verdict");
    }));

    test("decideLive: missing API key -> deny (fail-closed)", async () => {
        const name = GOOD_CONFIG.apiKeyEnv;
        const prev = process.env[name];
        delete process.env[name];
        try {
            const r = await decideLive(GOOD_CONFIG, "input", fakeFetchOk("ok"));
            assert.equal(r.status, "deny");
            assert.match(r.audit, /missing API key/);
        } finally {
            if (prev !== undefined) process.env[name] = prev;
        }
    });

    // ===== Default posture: live path is OPT-IN =====

    test("default posture: classifyLive/decideLive are only reachable when explicitly invoked", () => {
        // The plugin gates the live path behind `config.mode === "live"`. audit
        // (default) and enforce branches never call into this module. The verdict
        // module's own suite covers that audit/enforce behavior is unchanged; this
        // test pins that the live composition surface is a pure function that does
        // nothing unless the plugin actually calls it — i.e. there is no module-
        // level side effect that could affect audit mode.
        // (No-op assertion: importing this module executes no hooks and no I/O.)
        assert.equal(typeof classifyLive, "function");
        assert.equal(typeof decideLive, "function");
        assert.equal(typeof serializeTranscript, "function");
    });
}
