// auto-tool-gate.js — dual-surface plugin: audit + fail-closed enforce + live
// (Phases 1–3b pilot).
//
// This is the opt-in pilot for an auto-classifier-style tool-call gate. It
// hooks BOTH permission surfaces. Behavior is selected by the live config
// `mode` field (default `audit`):
//
//   mode "audit"   (Phase 1, default) — observability only. Both hooks log to
//                  stderr with a verdict PLACEHOLDER. No model call, no real
//                  verdict, no status mutation, no blocking. Zero behavior
//                  change.
//   mode "enforce" (Phase 2)          — permission.ask runs the decision path
//                  (stubEvaluate -> parseVerdict -> matrix) and sets
//                  output.status. Fail-closed: ANY uncertainty (parse failure,
//                  evaluator error, thrown exception) -> deny, NEVER silent
//                  allow. tool.execute.before stays an OBSERVER in every mode.
//                  The Phase 2 evaluator is a DETERMINISTIC STUB, not a real
//                  classifier model.
//   mode "live"    (Phase 3b)         — permission.ask fetches the real
//                  transcript, serializes it to a redacted text-mode string,
//                  and calls a provider-agnostic OpenAI-compatible HTTP
//                  completion endpoint (see ./auto-gate-live.js). The returned
//                  verdict text is fed through the SAME parseVerdict -> decision
//                  matrix as enforce, so the existing fail-closed matrix
//                  applies: any transport error / timeout / non-2xx / malformed
//                  / missing-choices / misconfigured-endpoint / missing-API-key
//                  -> deny, NEVER silent allow. tool.execute.before stays an
//                  OBSERVER. The API key is read from the named env var at call
//                  time; it NEVER lives in the (commitable) config file.
//
// THREE HOOKS, ONE ENFORCEMENT SURFACE (verified against @opencode-ai/plugin
// + sdk types + the upstream's shipped ACP / CLI / TUI reference impls):
//
//   1. tool.execute.before  (input:{tool,sessionID,callID}, output:{args})
//      AUDIT-ONLY observer. Sees EVERY tool call — including ones the
//      permission table auto-allows (those never reach permission.asked).
//      Powers: block (throw) or passthrough (bare return) ONLY. Cannot
//      force-allow or force-ask. We use it purely to observe the full
//      tool-call stream and capture the arg summary. It stays an OBSERVER
//      in EVERY mode — it must NOT throw or block in the new model; the
//      event hook owns enforcement.
//
//   2. permission.ask  (input:Permission, output:{status})
//      DORMANT — OpenCode does not fire `permission.ask` in any stock
//      release as of the studied version. The hook is RETAINED as a RESERVE
//      in case upstream wires it (preserves the Phase 2/3b investment).
//      Do NOT rely on it. No claim of auto-approval rests on this hook.
//
//   3. event  ({ event }) — the PRIMARY ENFORCEMENT SURFACE.
//      Receives EVERY bus event. Acts only on `permission.asked` — the event
//      OpenCode publishes when its ruleset routes a tool call to "ask". The
//      event payload is the Request {id, sessionID, permission, patterns,
//      metadata, always, tool}. The hook classifies it and REPLIES via the
//      SDK client: client.postSessionIdPermissionsPermissionId({path:{id,
//      permissionID}, body:{response}}) → resolves the Deferred Permission.ask
//      is awaiting → tool proceeds (allow) or is blocked (reject). This is
//      the SAME mechanism the upstream ships in its ACP agent,
//      `--dangerously-skip-permissions`, and TUI.
//
//      CRITICAL HEADLESS: if NO ONE replies to permission.asked, the Deferred
//      never resolves and the tool call HANGS. In autonomous modes (enforce/
//      live) the hook MUST reply. Audit mode (observe-only) and
//      onUncertain:"passthrough" (interactive only) are the only no-reply
//      paths.
//
// HARD-FLOOR INVARIANT: the event hook fires ONLY for ask-routed calls
// (table-allow fast-paths past the bus event; table-deny / shell-guard blocks
// before it). The classifier can NEVER override a static deny. It only ever
// decides the ask-routed subset. The static permission table is the first
// gate; the classifier runs strictly after it.
//
// Phase status:
//   Phase 3b (implemented here) — live classifier model wired into
//             permission.ask behind mode:"live" (replaces the enforce stub with
//             a real OpenAI-compatible HTTP call via ./auto-gate-live.js).
//   Phase 4   (later slice)     — promotion review (core-template /
//             README.agent.md).
// Reconciliation rule those phases must preserve: static deny wins; static
// failure denies; LLM allow only valid when no lower layer denied; LLM
// failure/timeout/malformed blocks.
//
// Design source: researches/sources/2026-07-10-auto-mode-classifier-source-packet.local.md
//
// Naming: all identifiers here are GENERIC (auto-tool-gate / auto-gate-audit).
// The upstream mechanism is referred to only as "the reference agent system" /
// "a security-monitor classifier" — never by product name.
//
// Plugin contract (mirrors .opencode/plugins/shell-guard.js + session-state.js):
//   export const server = async ({ client, directory }) => ({
//       // The factory receives the full PluginInput; we close over `client` (the
//       // OpenCode SDK client, used in mode:"live" to fetch the session
//       // transcript) and `directory` (the repo dir, used as the SDK query
//       // param). Same pattern session-state.js uses for client.session.todo().
//       "tool.execute.before": async (input, output) => {
//           // input.tool  → tool name (string)
//           // output.args → { command, workdir, filePath, path, pattern, ... }
//           // throw new Error(reason)        → BLOCKS the tool call
//           // console.error(reason); return; → ASK (passthrough to perm table)
//           // return;                        → ALLOW / passthrough (do nothing)
//       },
//       "permission.ask": async (input, output) => {
//           // input  → Permission {id, type, pattern, sessionID, messageID,
//           //                       callID?, title, metadata:{}, time:{created}}
//           // output → {status:"ask"|"deny"|"allow"} (default "ask")
//           // output.status = "allow" → GRANT + skip user prompt
//           // output.status = "deny"  → BLOCK
//           // output.status = "ask"   → trigger interactive prompt (default)
//           // bare return             → leave status unchanged (Phase 1)
//       }
//   });
//
// OpenCode auto-discovers plugins from .opencode/plugins/*.js — no
// registration in opencode.jsonc is required (confirmed: shell-guard.js,
// session-state.js, and maxoutputtokens.js all load with no "plugins" key).
// This file renders from the auto-classifier-pilot overlay pack's
// plugins/auto-tool-gate.js unit into .opencode/plugins/auto-tool-gate.js.
//
// ---------------------------------------------------------------------------
// Live hot-config substrate (reload-free).
//
// Auto-mode is configurable WITHOUT restarting OpenCode: each hook invocation
// reads a small operator-owned JSON config file from disk, gated by an mtime
// cache so an unchanged file costs only a single `statSync` per call. Editing
// the file takes effect on the NEXT tool call. The OpenCode plugin SDK has no
// native hot-reload config API (the `config` hook and `PluginOptions` are
// load-time, set at server start; env vars are frozen at process start), but
// plugins CAN do file I/O at runtime — same pattern shell-guard.js uses
// (node:fs + node:path, per-call statSync/readFile). See readConfig() below
// and the README's "Live configuration" section.
//
// Fail-safe: a missing / unreadable / invalid config file NEVER throws — the
// plugin falls back to built-in defaults ({enabled:true, mode:"audit"}) and
// emits one console.error audit line per failure-state transition.
// ---------------------------------------------------------------------------

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
// node:test + node:assert imported STATICALLY so the self-test registers
// SYNCHRONOUSLY when run directly (no top-level await). INERT on the import
// path: importing them does not start a test runner — only the test() CALLS do,
// guarded behind __isMain so the plugin-loader path (OpenCode importing this
// module for its `server` export) never fires the suite.
import { test } from "node:test";
import { strict as assert } from "node:assert";

// Pure verdict-parse + decision layer (Phase 2). Mirrors the shell-guard.js ->
// shell-guard-core.js pattern: the plugin imports ONE decision module and
// never re-implements parsing. decidePermission(config) composes
// stubEvaluate(config) -> parseVerdict(raw) -> decision matrix, fail-closed to
// deny on any uncertainty. See ./auto-gate-verdict.js for the contract.
import { decidePermission } from "./auto-gate-verdict.js";

// Live classifier substrate (Phase 3b): transcript serializer + generic
// domain-free system prompt + OpenAI-compatible HTTP adapter + the decideLive
// bridge. Only reachable when config.mode === "live". The audit and enforce
// branches below do NOT touch this module, so they are unchanged by Phase 3b.
import { decideLive, serializeTranscript } from "./auto-gate-live.js";

// Shared credential scrubber (egress-safe): auto-tool-gate.js is the
// AUDIT/STDERR-LOG egress surface. Every tool-call-derived value that reaches a
// console.error line (summarizeArgs output + the permission.ask `pattern`)
// passes through scrubTruncate (scrubCredentials then truncate), NOT truncate
// alone, so a credential embedded in a `command`/`pattern` cannot survive into
// the stderr log. The IDENTICAL scrubber is shared with the HTTP-egress path
// (auto-gate-live.js) via this module — no drift.
import { scrubTruncate } from "./auto-gate-scrub.js";

export const id = "auto-tool-gate";

// ESM does not provide __dirname (the OpenCode plugin runtime loads these as
// ES modules). Derive it the same way shell-guard-core.js / state-lib.js do,
// so repoRoot() + CONFIG_PATH resolve correctly at module-load time.
const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

// Maximum length of any single argument value in the audit summary. Long
// command strings, file bodies, or structured payloads are truncated so the
// audit line stays a one-liner suitable for stderr log scraping.
const MAX_ARG_LEN = 160;

// Build a short, REDACTED argument summary for the audit line. We deliberately
// surface only the load-bearing IDENTIFYING fields (command, path, pattern,
// query, url, workdir) and NEVER dump the full args object — tool inputs can
// carry large file bodies, edit diffs, or sensitive payloads that have no
// place in a one-line audit log. Unknown / unhandled tools get an arg-key
// count summary only, so the audit still records that the tool was called
// without leaking its payload.
//
// SECURITY: every allowlisted field value passes through scrubTruncate
// (scrubCredentials THEN truncate, from the shared auto-gate-scrub.js), NOT
// truncate alone. This audit line lands on stderr — which the OpenCode/server
// process writes to its stderr log — so a Bearer token / API key / DB
// connection string embedded in a judged `command` or `pattern` MUST be
// scrubbed the same way the HTTP-egress path scrubs it. Before this fix a
// `curl -H "Authorization: Bearer <token>"` command leaked the token verbatim
// into the stderr log (truncate-only). Now the token is [redacted] before the
// audit line is ever written.
function summarizeArgs(args) {
    if (!args || typeof args !== "object") return "";
    const parts = [];
    // bash / shell tool: the command string is the identifying input.
    if (typeof args.command === "string") {
        parts.push(`command=${scrubTruncate(args.command, MAX_ARG_LEN)}`);
    }
    // read / edit / write / glob / grep: the target path identifies the call.
    const fp = args.filePath ?? args.path;
    if (typeof fp === "string") {
        parts.push(`path=${scrubTruncate(fp, MAX_ARG_LEN)}`);
    }
    // glob / grep: the pattern/query scopes the call.
    if (typeof args.pattern === "string") {
        parts.push(`pattern=${scrubTruncate(args.pattern, MAX_ARG_LEN)}`);
    }
    if (typeof args.query === "string") {
        parts.push(`query=${scrubTruncate(args.query, MAX_ARG_LEN)}`);
    }
    // webfetch: the url identifies the call.
    if (typeof args.url === "string") {
        parts.push(`url=${scrubTruncate(args.url, MAX_ARG_LEN)}`);
    }
    // workdir disambiguates cwd-sensitive tools (bash).
    if (typeof args.workdir === "string") {
        parts.push(`workdir=${scrubTruncate(args.workdir, MAX_ARG_LEN)}`);
    }
    // If nothing load-bearing matched, emit only an arg-key count so the line
    // still records that the tool was called with structured input.
    if (parts.length === 0) {
        const keys = Object.keys(args);
        parts.push(`args=${keys.length}`);
    }
    return parts.join(" ");
}

// ---------------------------------------------------------------------------
// Hot-config reader.
//
// Resolves the operator-owned config file relative to the repo root, the same
// way shell-guard-core.js derives repoRoot() (from this file's location —
// .opencode/plugins/auto-tool-gate.js -> two levels up). Never uses
// process.cwd() (unreliable in the plugin server context). No hardcoded
// absolute paths.
function repoRoot() {
    return path.resolve(__dirname, "..", "..");
}

// ---------------------------------------------------------------------------
// Two-file live config model (reload-free).
//
// Config is split across TWO sibling files under .opencode/repo-configs/ so
// that LLM secrets-adjacent settings can NEVER be committed while plugin
// behavior MAY be committed (or not) at the adopter's choice:
//
//   1. Plugin config → auto-gate-config.json (EXISTING path, kept).
//      Holds the plugin-BEHAVIOR fields: {enabled, mode, stubVerdict, promptFile}.
//      Committability: ADOPTER'S CHOICE — a team may commit a shared default
//      (e.g. {"mode":"enforce"}). NOT gitignored. Fail-safe defaults:
//      missing/invalid → {enabled:true, mode:"audit", stubVerdict:"block",
//      promptFile:""}.
//
//   2. LLM config    → auto-gate-llm.json  (NEW sibling file).
//      Holds the LLM fields: {modelEndpoint, model, apiKeyEnv, timeoutMs,
//      maxRetries, retryDelayMs}. Committability: NEVER — gitignored in the
//      dogfood repo (adopters add the pattern to their own .gitignore).
//      Fail-safe defaults: missing/invalid → {modelEndpoint:"", model:"",
//      apiKeyEnv:"AUTO_GATE_API_KEY", timeoutMs:8000, maxRetries:1,
//      retryDelayMs:500}. A MISSING LLM file is NORMAL (only needed for live
//      mode) and is SILENT — no audit spam; audit/enforce modes must NOT fail
//      because the LLM file is absent. In live mode a missing/empty
//      modelEndpoint/model fail-closes to deny via the existing decision path.
//
// Backward-compat (CLEAN CUT): an operator may still have LLM fields in the
// OLD auto-gate-config.json. They are IGNORED entirely — readConfig() returns
// ONLY the four plugin-behavior fields. This is a freshly-shipped pilot with
// no real install base, so a clean cut (no deprecation fallback) is safe and
// keeps the two files strictly disjoint. LLM fields MUST come from
// auto-gate-llm.json.
//
// The API key VALUE is NEVER in either file — only the env-var NAME
// (apiKeyEnv, default AUTO_GATE_API_KEY). The value is read from
// process.env[apiKeyEnv] at call time inside classifyLive.
//
// Merge point: the live branch builds ONE merged object
// ({...readConfig(), ...readLlmConfig()}) so downstream decideLive /
// classifyLive / resolveSystemPrompt see a single config as before. The audit
// and enforce branches only need readConfig() (plugin behavior).
// ---------------------------------------------------------------------------

// Plugin-config path, repo-relative. The `repo-configs/` dir is where the
// harness already keeps operator-facing config-like data
// (allowed-commands.js, forbidden-patterns.js, forbidden-patterns.core.js,
// repo-recon-data.yml). The overlay does NOT render or seed this file — its
// absence is the documented fail-safe default.
const CONFIG_PATH = path.resolve(
    repoRoot(),
    ".opencode",
    "repo-configs",
    "auto-gate-config.json",
);

// LLM-config path — a NEW sibling file. Same repo-configs/ dir. NEVER
// committed (gitignored); only needed for live mode.
const LLM_CONFIG_PATH = path.resolve(
    repoRoot(),
    ".opencode",
    "repo-configs",
    "auto-gate-llm.json",
);

// Plugin-behavior fail-safe defaults (auto-gate-config.json). `enabled` is the
// master live kill-switch; `mode` is the behavior selector (`audit` = Phase 1
// log-only; `enforce` = Phase 2 stub decision path; `live` = Phase 3b real-
// model decision path). `stubVerdict` drives the deterministic stub evaluator
// in enforce mode. `promptFile` optionally overrides the classifier system
// prompt (consulted only in live mode via resolveSystemPrompt, but lives in
// the plugin-config file so it MAY be committed as a shared default).
const DEFAULT_PLUGIN_CONFIG = Object.freeze({
    enabled: true,
    mode: "audit",
    stubVerdict: "block",
    promptFile: "",
    replyMode: "once", // event hook: "once" | "always" — the reply disposition on an allow verdict
    onUncertain: "reject", // event hook: "reject" | "passthrough" — failure/uncertainty disposition
});

// LLM fail-safe defaults (auto-gate-llm.json). `modelEndpoint` and `model`
// default to empty (so a live call with no endpoint/model fail-closes to deny
// instead of hitting a garbage URL). `apiKeyEnv` defaults to the conventional
// env-var NAME (never the value). `timeoutMs` is a conservative bound.
// `maxRetries` / `retryDelayMs` configure retry-on-transient-failure INSIDE
// classifyLive (timeout / network error / 5xx / 2xx-empty). Defaults are
// conservative: 1 retry, 500ms base — enough to recover from a single stall
// without unbounded token cost (each retry is a fresh API call).
const DEFAULT_LLM_CONFIG = Object.freeze({
    modelEndpoint: "", // required for live; empty -> fail-closed deny
    model: "", // required for live; empty -> fail-closed deny
    apiKeyEnv: "AUTO_GATE_API_KEY", // NAME of the env var only (never the value)
    timeoutMs: 8000, // hard timeout for the model HTTP call
    maxRetries: 1, // ADDITIONAL attempts after the first (0 = single attempt)
    retryDelayMs: 500, // base delay; LINEAR backoff (see classifyLive)
});

// mtime cache: stores the last successful parse plus a fallback-warning latch
// so a persistent failure (missing / invalid file) emits exactly ONE stderr
// audit line per failure STATE instead of spamming every tool call. A state
// transition (missing -> present -> invalid) re-warns once. Module-level on
// purpose — survives across hook invocations within one server process.
//
// TWO caches (one per file) are held as MUTABLE const objects (properties
// reassigned, never the binding) so a shared private reader core can update
// either without rebinding a module-level `let`.
const pluginConfigCache = {
    mtime: null, // last mtimeMs parsed successfully (null until first hit)
    parsed: null, // last parsed + merged config object (null until first hit)
    fallbackReason: null, // null | "missing" | "unreadable" | "invalid"
};
const llmConfigCache = {
    mtime: null,
    parsed: null,
    fallbackReason: null,
};

// Normalize a parsed plugin-config object over defaults (field-by-field so a
// partial config like {"enabled": false} still resolves every field). LLM
// fields present in this file are IGNORED (clean cut) — they MUST come from
// auto-gate-llm.json.
function normalizePluginConfig(parsed) {
    return {
        enabled:
            typeof parsed.enabled === "boolean"
                ? parsed.enabled
                : DEFAULT_PLUGIN_CONFIG.enabled,
        mode:
            parsed.mode === "audit" ||
            parsed.mode === "enforce" ||
            parsed.mode === "live"
                ? parsed.mode
                : DEFAULT_PLUGIN_CONFIG.mode,
        stubVerdict:
            parsed.stubVerdict === "allow" ||
            parsed.stubVerdict === "block" ||
            parsed.stubVerdict === "fail"
                ? parsed.stubVerdict
                : DEFAULT_PLUGIN_CONFIG.stubVerdict,
        promptFile:
            typeof parsed.promptFile === "string"
                ? parsed.promptFile
                : DEFAULT_PLUGIN_CONFIG.promptFile,
        replyMode:
            parsed.replyMode === "once" || parsed.replyMode === "always"
                ? parsed.replyMode
                : DEFAULT_PLUGIN_CONFIG.replyMode,
        onUncertain:
            parsed.onUncertain === "reject" ||
            parsed.onUncertain === "passthrough"
                ? parsed.onUncertain
                : DEFAULT_PLUGIN_CONFIG.onUncertain,
    };
}

// Normalize a parsed LLM-config object over defaults. Each field is fail-safe-
// normalized: an invalid type falls back to the default, which for
// endpoint/model is empty (so a misconfigured live call fail-closes to deny,
// not to a garbage request). The API key VALUE is never read here — only the
// env-var NAME, looked up at call time inside classifyLive.
//
// _normNonNegInt — coerce a value to a non-negative integer, else return the
// default. Accepts a finite non-negative number (floored) or a numeric string;
// anything else (negative, NaN, boolean, object, empty) falls back. Used by
// maxRetries / retryDelayMs so an operator typo can never break the live path.
function _normNonNegInt(v, dflt) {
    if (typeof v === "number" && Number.isFinite(v) && v >= 0) {
        return Math.floor(v);
    }
    if (typeof v === "string" && /^\d+$/.test(v.trim())) {
        return Math.floor(Number(v));
    }
    return dflt;
}

function normalizeLlmConfig(parsed) {
    return {
        modelEndpoint:
            typeof parsed.modelEndpoint === "string"
                ? parsed.modelEndpoint
                : DEFAULT_LLM_CONFIG.modelEndpoint,
        model:
            typeof parsed.model === "string"
                ? parsed.model
                : DEFAULT_LLM_CONFIG.model,
        apiKeyEnv:
            typeof parsed.apiKeyEnv === "string" && parsed.apiKeyEnv
                ? parsed.apiKeyEnv
                : DEFAULT_LLM_CONFIG.apiKeyEnv,
        timeoutMs:
            typeof parsed.timeoutMs === "number" && parsed.timeoutMs > 0
                ? parsed.timeoutMs
                : DEFAULT_LLM_CONFIG.timeoutMs,
        maxRetries: _normNonNegInt(parsed.maxRetries, DEFAULT_LLM_CONFIG.maxRetries),
        retryDelayMs: _normNonNegInt(parsed.retryDelayMs, DEFAULT_LLM_CONFIG.retryDelayMs),
    };
}

// Private reader core: stat → (cache fast-path) → read → parse → normalize →
// cache-latch. NEVER throws. Side effect: emits at most one console.error
// audit line per failure-state transition (de-duped via cache.fallbackReason),
// UNLESS silentOnMissing is true (a missing file is then the normal case and
// emits NOTHING). `label` prefixes the audit line so the operator knows WHICH
// file failed.
//
// `targetPath` is injectable (the public readers default it to the production
// repo-configs path) so the self-tests can point the readers at temp files
// under tmp/ without touching the real config location.
function _readJsonConfig(
    targetPath,
    cache,
    defaults,
    normalize,
    silentOnMissing,
    label,
) {
    let st;
    try {
        st = fs.statSync(targetPath);
    } catch (_) {
        // Missing / unreadable metadata: ENOENT / EACCES / etc.
        if (!silentOnMissing) {
            if (cache.fallbackReason !== "missing") {
                console.error(
                    `[auto-gate-audit] ${label} config not found at ${targetPath}; ` +
                    `using fail-safe defaults ${JSON.stringify(defaults)} ` +
                    `(create the file to override).`,
                );
            }
            cache.fallbackReason = "missing";
        }
        // silentOnMissing: a missing file is the NORMAL case (e.g. no live
        // mode set up). Do NOT spam, do NOT latch a fallback state.
        return defaults;
    }

    const mtimeMs = st.mtimeMs;
    // Fast path: unchanged since last successful parse AND not currently in a
    // fallback state — return the cached parsed object (single statSync cost).
    if (cache.parsed && cache.mtime === mtimeMs && !cache.fallbackReason) {
        return cache.parsed;
    }

    let raw;
    try {
        raw = fs.readFileSync(targetPath, "utf8");
    } catch (_) {
        if (cache.fallbackReason !== "unreadable") {
            console.error(
                `[auto-gate-audit] ${label} config unreadable at ${targetPath}; ` +
                `using fail-safe defaults ${JSON.stringify(defaults)}.`,
            );
            cache.fallbackReason = "unreadable";
        }
        return defaults;
    }

    let parsed;
    // `invalidReason` is set when the JSON is structurally unusable as config
    // (a parse failure OR a successful parse of a non-object — see F3 below).
    // Both flow through the SAME deduped "invalid" fallback path so the operator
    // sees one audit line per failure state, never a throw.
    let invalidReason = null;
    try {
        parsed = JSON.parse(raw);
    } catch (_) {
        invalidReason = "invalid JSON";
    }
    // Fail-safe (F3): a parse that SUCCEEDED but did not yield a plain object
    // (literal `null`, an array, or a bare primitive/string/number/boolean)
    // must NEVER reach the normalizer — `normalize(parsed)` would throw on
    // property access (e.g. `parsed.enabled` on null throws TypeError) instead
    // of returning fail-safe defaults. Treat it exactly like invalid JSON:
    // return defaults via the same "invalid" fallbackReason + deduped audit line.
    if (
        invalidReason === null &&
        (parsed === null ||
            typeof parsed !== "object" ||
            Array.isArray(parsed))
    ) {
        invalidReason = "invalid JSON shape (expected a config object)";
    }
    if (invalidReason !== null) {
        if (cache.fallbackReason !== "invalid") {
            console.error(
                `[auto-gate-audit] ${label} config ${invalidReason} at ${targetPath}; ` +
                `using fail-safe defaults ${JSON.stringify(defaults)}.`,
            );
            cache.fallbackReason = "invalid";
        }
        return defaults;
    }

    // Successful parse of a plain object: normalize + merge over defaults (so
    // partial configs resolve every field), latch the cache, clear any prior
    // fallback state.
    const merged = normalize(parsed);
    cache.mtime = mtimeMs;
    cache.parsed = merged;
    cache.fallbackReason = null;
    return merged;
}

// Read the PLUGIN-BEHAVIOR config (auto-gate-config.json) with an mtime cache.
// Returns {enabled, mode, stubVerdict, promptFile} on every call — never
// throws. Emits one console.error audit line per failure-state transition
// (missing / unreadable / invalid). LLM fields in this file are IGNORED.
// `configPath` is injectable for the self-test; production callers omit it.
export function readConfig(configPath = CONFIG_PATH) {
    return _readJsonConfig(
        configPath,
        pluginConfigCache,
        DEFAULT_PLUGIN_CONFIG,
        normalizePluginConfig,
        false, // NOT silent: a missing plugin config warns once (existing behavior)
        "plugin",
    );
}

// Read the LLM config (auto-gate-llm.json) with its OWN mtime cache. Returns
// {modelEndpoint, model, apiKeyEnv, timeoutMs, maxRetries, retryDelayMs} on
// every call — never throws. A MISSING file is SILENT (no audit spam) — it is
// the normal case when live mode is not set up; audit/enforce modes must NOT
// fail because the LLM file is absent. Only a PRESENT-but-invalid file emits an
// audit line (mirroring the existing invalid-JSON handling). `llmPath` is
// injectable for the self-test; production callers omit it.
export function readLlmConfig(llmPath = LLM_CONFIG_PATH) {
    return _readJsonConfig(
        llmPath,
        llmConfigCache,
        DEFAULT_LLM_CONFIG,
        normalizeLlmConfig,
        true, // SILENT on missing: no live setup = no spam
        "llm",
    );
}

// Test-only: reset BOTH config caches so the self-test's filesystem tests are
// isolated from each other and from any prior production read. Mirrors the
// __resetCachedBinaryPrompt helper pattern in auto-gate-live.js.
export function __resetConfigCaches() {
    pluginConfigCache.mtime = null;
    pluginConfigCache.parsed = null;
    pluginConfigCache.fallbackReason = null;
    llmConfigCache.mtime = null;
    llmConfigCache.parsed = null;
    llmConfigCache.fallbackReason = null;
}

// The factory receives the full PluginInput ({client, project, directory,
// worktree, serverUrl, $}) — same contract session-state.js relies on for
// client.session.todo(). We close over `client` (the OpenCode SDK client, used
// in mode:"live" to fetch the session transcript + in mode:"enforce"/"live" of
// the event hook to reply to permission.asked) and `directory` (the repo dir,
// passed as the SDK query param for transcript fetch). The audit and
// enforce branches never touch either.
//
// `configPath` / `llmConfigPath` are optional test-injection points: production
// callers omit them (the hooks default to the production repo-configs paths).
// The self-tests pass temp-file paths under tmp/ to isolate the filesystem.
export const server = async ({ client, directory, configPath, llmConfigPath } = {}) => {
    return {
        "tool.execute.before": async (input, output) => {
            // Live config — read on every call (mtime-cached, single statSync
            // in steady state). The operator can live-disable the plugin by
            // setting `enabled: false` in the config file; no OpenCode
            // restart, no re-render required.
            const config = readConfig(configPath);
            if (config.enabled === false) {
                // Operator kill-switch: the plugin is fully inert (no audit,
                // no behavior change). This is the only branch that short-
                // circuits before the audit log.
                return;
            }
            // AUDIT ONLY — permanently, in EVERY mode. Capture tool name + arg
            // summary + verdict PLACEHOLDER on stderr. Never throw, never block,
            // never mutate. The bare `return` is an unconditional ALLOW /
            // passthrough; this hook changes zero tool-call behavior.
            //
            // This hook sees EVERY tool call — including ones the permission
            // table auto-allows (those never reach permission.ask). That makes
            // it the right place to capture the arg summary, and the
            // complementary surface to permission.ask for the dual-hook audit.
            //
            // WHY THIS HOOK STAYS AN OBSERVER EVEN IN ENFORCE MODE: it can ONLY
            // block (throw) or passthrough (bare return) — it cannot force-allow
            // or force-ask. Because it fires for ALL calls (not just ask-routed
            // ones), running a classifier verdict here would either block calls
            // the table already allowed (wrong) or be redundant with
            // permission.ask. So only permission.ask owns the authoritative
            // decision (Phase 2+); this hook is always an observer.
            const tool = (input && input.tool) || "unknown";
            const summary = summarizeArgs(output && output.args);
            console.error(
                `[auto-gate-audit] tool=${tool} ${summary} verdict=AUDIT_ONLY`,
            );
            return;
        },

        "permission.ask": async (input, output) => {
            // DORMANT — OpenCode does not fire `permission.ask` in any stock
            // release as of the studied version. The `event` hook (below) is the
            // ACTIVE enforcement surface: it receives the `permission.asked`
            // bus event and replies via the SDK client
            // (postSessionIdPermissionsPermissionId). This hook is RETAINED as
            // a RESERVE in case upstream wires permission.ask in a future
            // release — do NOT rely on it, but keep the investment intact.
            // Live config — read on every call (mtime-cached).
            const config = readConfig(configPath);
            if (config.enabled === false) {
                // Operator kill-switch: fully inert, no audit, no behavior
                // change. output.status is left at its default so opencode's
                // normal interactive ask still fires.
                return;
            }

            if (config.mode === "enforce") {
                // ENFORCE (Phase 2): run the verdict decision path.
                //
                // HARD-FLOOR INVARIANT: permission.ask fires ONLY for calls
                // opencode's permission table routes to `ask`. Table-`allow`
                // fast-paths past this hook; table-`deny` / shell-guard blocks
                // BEFORE this hook. Therefore the classifier decision below
                // can only ever lift an `ask` to `allow`/`deny` — it can NEVER
                // override a static deny, because a statically-denied call
                // never reaches this hook. The classifier only ever decides
                // the ask-routed subset.
                //
                // Phase 2 uses a STUB evaluator (stubEvaluate inside
                // decidePermission), NOT a real classifier model. Do NOT run
                // enforce mode against real traffic until Phase 3 wires a live
                // model. The decision path fail-closes to deny on ANY
                // uncertainty (parse failure, evaluator error, thrown
                // exception).
                const type = (input && input.type) || "unknown";
                const pattern = scrubTruncate((input && input.pattern) || "", MAX_ARG_LEN);
                console.error(
                    `[auto-gate] permission.ask type=${type} pattern=${pattern} mode=enforce (deciding)`,
                );
                // Decision path. decidePermission(config) composes
                // stubEvaluate(config) -> parseVerdict(raw) -> decision matrix
                // and NEVER throws (it catches evaluator errors internally and
                // returns a fail-closed deny). We wrap defensively anyway so a
                // future regression fail-closes to deny rather than crashing
                // the hook.
                let result;
                try {
                    result = decidePermission(config);
                } catch (err) {
                    const msg = (err && err.message) || String(err);
                    console.error(
                        `[auto-gate] fail-closed: decision error: ${msg}`,
                    );
                    output.status = "deny";
                    return;
                }
                if (result.audit) {
                    console.error(`[auto-gate] ${result.audit}`);
                }
                output.status = result.status; // "allow" | "deny"
                return;
            }

            if (config.mode === "live") {
                // LIVE (Phase 3b): run the REAL classifier model decision path.
                //
                // The same hard-floor invariant holds: permission.ask only
                // fires for ask-routed calls, so this can only lift an `ask` to
                // allow/deny — never override a static deny. The decision path
                // uses the SAME parseVerdict -> decision matrix as enforce, fed
                // by a real model verdict instead of the stub. The matrix is
                // fail-closed, so the live path inherits that posture:
                // transport error / timeout / non-2xx / malformed / missing-
                // choices / unparseable verdict -> deny, NEVER silent allow.
                //
                // Transcript fetch degrades GRACEFULLY: if the SDK call fails
                // (no client, error wrapper, missing data), we fall back to the
                // permission payload ALONE (serializeTranscript([], input))
                // rather than fail-closed. The model still gets the type+pattern
                // to judge. Only the model-call / decision layer fail-closes.
                const type = (input && input.type) || "unknown";
                const pattern = scrubTruncate((input && input.pattern) || "", MAX_ARG_LEN);
                console.error(
                    `[auto-gate] permission.ask type=${type} pattern=${pattern} mode=live (deciding)`,
                );

                // MERGE POINT: build ONE config object for the live path by
                // merging the plugin-behavior config (already read above into
                // `config` as {enabled, mode, stubVerdict, promptFile}) with the
                // LLM config (auto-gate-llm.json → {modelEndpoint, model,
                // apiKeyEnv, timeoutMs}). A missing LLM file is SILENT here:
                // readLlmConfig() returns empty-string defaults, which flow
                // straight into the fail-closed validation below. Downstream
                // decideLive / classifyLive / resolveSystemPrompt see a single
                // merged object exactly as before the two-file split.
                const liveConfig = { ...config, ...readLlmConfig(llmConfigPath) };

                // (1) Validate live config up front so a misconfigured live
                // mode fail-closes to deny with a CLEAR audit line instead of a
                // cryptic adapter error.
                if (!liveConfig.modelEndpoint) {
                    console.error(
                        "[auto-gate] live mode misconfigured: no modelEndpoint; fail-closed deny",
                    );
                    output.status = "deny";
                    return;
                }
                if (!liveConfig.model) {
                    console.error(
                        "[auto-gate] live mode misconfigured: no model; fail-closed deny",
                    );
                    output.status = "deny";
                    return;
                }

                // (2) Fetch the session transcript. Graceful degradation on any
                // failure: use the permission payload alone. SDK calls return a
                // RequestResult wrapper — read payload via .data and check
                // .error (proven in .opencode/plugins/session-state.js).
                let transcript = [];
                try {
                    if (
                        client &&
                        client.session &&
                        typeof client.session.messages === "function" &&
                        input &&
                        input.sessionID
                    ) {
                        const r = await client.session.messages({
                            path: { id: input.sessionID },
                            query: { directory },
                        });
                        if (r && r.error) throw r.error;
                        if (r && Array.isArray(r.data)) {
                            transcript = r.data;
                        }
                    } else {
                        // No client threaded into the plugin, or no sessionID on
                        // the input: degrade to permission-payload-only. (This
                        // is a soft degradation, NOT a fail-closed condition.)
                        throw new Error("client/session unavailable");
                    }
                } catch (err) {
                    const msg = (err && err.message) || String(err);
                    console.error(
                        `[auto-gate] transcript fetch failed (${msg}); using permission payload only`,
                    );
                    transcript = [];
                }

                // (3) Serialize the transcript to a redacted text-mode string.
                const serialized = serializeTranscript(transcript, input);

                // (4) Run the live model decision path. decideLive() awaits the
                // HTTP adapter and hands the raw verdict text to the SAME
                // synchronous decidePermission() decision matrix (so the
                // existing fail-closed matrix applies unchanged). It returns
                // {status, audit, reason, latencyMs} and never throws.
                let result;
                try {
                    result = await decideLive(liveConfig, serialized);
                } catch (err) {
                    // Defensive: decideLive itself does not throw, but a future
                    // regression must fail-closed rather than crash the hook.
                    const msg = (err && err.message) || String(err);
                    console.error(
                        `[auto-gate] fail-closed: live decision error: ${msg}`,
                    );
                    output.status = "deny";
                    return;
                }
                if (result.audit) {
                    console.error(`[auto-gate] ${result.audit}`);
                }
                // Telemetry: surface the retry count when retries occurred
                // (result.retries is a safe integer; no tool-call content).
                // Egress discipline unchanged — this is the existing audit
                // surface, no new console site.
                const retryTag = result.retries > 0 ? ` retries=${result.retries}` : "";
                console.error(
                    `[auto-gate] live decision status=${result.status} latencyMs=${result.latencyMs}${retryTag}`,
                );
                output.status = result.status; // "allow" | "deny"
                return;
            }

            // AUDIT ONLY (Phase 1, byte-for-byte unchanged). Log the
            // permission-decision request WITHOUT changing the outcome. This
            // hook fires only when opencode's permission table resolves to
            // `ask` or no-match: table-`allow` calls fast-path past it, and
            // table-`deny`/shell-guard blocks before it. We record the request
            // and leave output.status at its default so the normal interactive
            // ask still fires.
            //
            // CRITICAL: this audit branch MUST NOT mutate output.status.
            // Setting it to "allow" would grant + skip the prompt (the enforce
            // branch above does that); setting it to "deny" would block. The
            // audit branch leaves it untouched — audit only, zero behavior
            // change. This is the default mode (`mode: "audit"`).
            const type = (input && input.type) || "unknown";
            const pattern = scrubTruncate((input && input.pattern) || "", MAX_ARG_LEN);
            const incoming = (output && output.status) || "(unset)";
            console.error(
                `[auto-gate-audit] permission.ask type=${type} pattern=${pattern} incoming=${incoming} verdict=AUDIT_ONLY`,
            );
            return; // do NOT set output.status — audit only
        },

        // ===================================================================
        // EVENT HOOK — the PRIMARY enforcement surface.
        //
        // This hook receives EVERY bus event. The only event type it acts on
        // is `permission.asked`, which OpenCode publishes when its ruleset
        // routes a tool call to "ask". The event payload (event.properties)
        // is the Request: {id, sessionID, permission, patterns, metadata,
        // always, tool}. The hook classifies it and REPLIES via the SDK
        // client: client.postSessionIdPermissionsPermissionId({path:{id,
        // permissionID}, body:{response}}). This resolves the Deferred that
        // Permission.ask is awaiting, which unblocks the tool call (allow) or
        // blocks it (reject). This is the SAME mechanism the upstream ships in
        // its ACP agent, `--dangerously-skip-permissions`, and TUI.
        //
        // CRITICAL HEADLESS BEHAVIOR: if NO ONE replies to permission.asked,
        // the Deferred never resolves and the tool call HANGS. In autonomous
        // modes (enforce/live) the hook MUST reply. The only mode that does
        // NOT reply is audit (observe-only, a human is expected to be present
        // to click the prompt) or onUncertain:"passthrough" (interactive
        // only — documented hang risk).
        //
        // EGRESS: event.properties (patterns, metadata) is tool-call-derived
        // and MUST be scrubbed before landing in any audit/log line.
        // ===================================================================
        "event": async ({ event } = {}) => {
            // Early-return on non-permission.asked events — the hook receives
            // EVERY bus event but only acts on this one type.
            if (!event || event.type !== "permission.asked") return;

            const req = event.properties;
            if (!req || !req.id || !req.sessionID) return;

            // Kill-switch: same live-disable as the other two hooks.
            const config = readConfig(configPath);
            if (config.enabled === false) return;

            // Scrubbed audit summary of the event payload. `patterns` and
            // `metadata` are tool-call-derived and MUST pass through the
            // shared scrubber before landing in the log line.
            //
            // PERMISSION-TYPE SHAPE: the SDK wire type for permission.asked
            // carries `permission` as the permission NAME STRING (e.g.
            // "bash"), not as an object with a `.type` field. The earlier
            // `req.permission && req.permission.type` form collapsed to
            // "unknown" on every real event, losing the tool-type signal
            // (fail-closed still fired, but the audit/serialized input said
            // type=unknown). Normalize defensively to accept BOTH the string
            // shape (today's wire type) and the object shape (a hedge against
            // an upstream change).
            const rawPerm = req && req.permission;
            const permType =
                typeof rawPerm === "string"
                    ? rawPerm
                    : ((rawPerm &&
                          typeof rawPerm === "object" &&
                          rawPerm.type) ||
                          "unknown");
            const patternsSummary = Array.isArray(req.patterns)
                ? req.patterns
                      .map((p) => scrubTruncate(String(p), MAX_ARG_LEN))
                      .join(",")
                : "";

            // --- Reply helper (shared by all decision branches) ---
            // Wraps the SDK call so every branch logs r.error uniformly.
            //
            // MUST NEVER THROW. The outer catch path routes failures to
            // handleUncertain -> reply("reject"); if THIS helper could throw
            // (a thrown transport error, or a missing client), the throw
            // inside the catch would reject the hook promise, surface an error
            // to OpenCode, and leave the permission.asked Deferred unresolved
            // -> HEADLESS HANG. So: guard the client, wrap the call in
            // try/catch, and log on any failure. The only residual hang case
            // is a fundamentally-broken transport (then nothing can resolve
            // the Deferred — inherent, now logged rather than crashing).
            const reply = async (responseMode) => {
                if (
                    !client ||
                    typeof client.postSessionIdPermissionsPermissionId !==
                        "function"
                ) {
                    console.error(
                        `[auto-gate] permission reply unavailable: no client ` +
                        `(responseMode=${responseMode})`,
                    );
                    return;
                }
                try {
                    const r =
                        await client.postSessionIdPermissionsPermissionId({
                            path: { id: req.sessionID, permissionID: req.id },
                            body: { response: responseMode },
                        });
                    if (r && r.error) {
                        console.error(
                            `[auto-gate] permission reply failed: ` +
                            `${(r.error && r.error.message) || "unknown"}`,
                        );
                    }
                } catch (e) {
                    console.error(
                        `[auto-gate] permission reply threw: ` +
                        `${(e && e.message) || "unknown"} ` +
                        `(responseMode=${responseMode})`,
                    );
                }
            };

            // --- Uncertainty/failure helper ---
            // Fail-closed default (reject). `passthrough` = no reply — ONLY
            // for interactive mode where a human is present to click the
            // prompt; in headless/autonomous mode passthrough HANGS the tool
            // call (documented risk).
            const handleUncertain = async (reason) => {
                if (config.onUncertain === "passthrough") {
                    console.error(
                        `[auto-gate] uncertain: ${reason}; ` +
                        `onUncertain=passthrough (no reply — interactive only)`,
                    );
                    return;
                }
                console.error(
                    `[auto-gate] uncertain: ${reason}; ` +
                    `onUncertain=reject (fail-closed)`,
                );
                await reply("reject");
            };

            // =============== AUDIT mode (default, observe-only) ===============
            if (config.mode === "audit") {
                console.error(
                    `[auto-gate-audit] permission.asked type=${permType} ` +
                    `patterns=${patternsSummary} verdict=AUDIT_ONLY (no reply)`,
                );
                return; // do NOT reply — the human decides
            }

            // =============== ENFORCE mode (stub classifier) ===================
            if (config.mode === "enforce") {
                console.error(
                    `[auto-gate] permission.asked type=${permType} ` +
                    `mode=enforce (deciding)`,
                );
                let result;
                try {
                    result = decidePermission(config);
                } catch (err) {
                    const msg = (err && err.message) || String(err);
                    await handleUncertain(`decision error: ${msg}`);
                    return;
                }
                if (result.audit) console.error(`[auto-gate] ${result.audit}`);
                if (result.status === "allow") {
                    await reply(config.replyMode); // "once" | "always"
                } else {
                    await reply("reject");
                }
                return;
            }

            // =============== LIVE mode (real classifier model) ================
            if (config.mode === "live") {
                console.error(
                    `[auto-gate] permission.asked type=${permType} ` +
                    `mode=live (deciding)`,
                );
                const liveConfig = { ...config, ...readLlmConfig(llmConfigPath) };

                // (1) Validate live config — missing endpoint/model fail-closes.
                if (!liveConfig.modelEndpoint) {
                    await handleUncertain("no modelEndpoint");
                    return;
                }
                if (!liveConfig.model) {
                    await handleUncertain("no model");
                    return;
                }

                // (2) Fetch the session transcript. Graceful degradation: on
                // any failure, use the permission payload alone (the model
                // still gets type+patterns to judge). The model-call / decision
                // layer owns the fail-closed decision; transcript fetch is soft.
                let transcript = [];
                try {
                    if (
                        client &&
                        client.session &&
                        typeof client.session.messages === "function"
                    ) {
                        const r = await client.session.messages({
                            path: { id: req.sessionID },
                            query: { directory },
                        });
                        if (r && r.error) throw r.error;
                        if (r && Array.isArray(r.data)) transcript = r.data;
                    } else {
                        throw new Error("client/session unavailable");
                    }
                } catch (err) {
                    const msg = (err && err.message) || String(err);
                    console.error(
                        `[auto-gate] transcript fetch failed (${msg}); ` +
                        `using permission payload only`,
                    );
                    transcript = [];
                }

                // (3) Serialize to a redacted text-mode string. Build a
                // permission-like object from the Request payload for the
                // serializer (it reads .type and .pattern).
                const permForSerializer = {
                    type: permType,
                    pattern: Array.isArray(req.patterns)
                        ? req.patterns.join(" ")
                        : "",
                };
                const serialized = serializeTranscript(
                    transcript,
                    permForSerializer,
                );

                // (4) Run the live model decision path. decideLive awaits the
                // HTTP adapter and feeds the raw verdict through the SAME
                // fail-closed decision matrix as enforce.
                let result;
                try {
                    result = await decideLive(liveConfig, serialized);
                } catch (err) {
                    const msg = (err && err.message) || String(err);
                    await handleUncertain(`live decision error: ${msg}`);
                    return;
                }
                if (result.audit) console.error(`[auto-gate] ${result.audit}`);
                const retryTag =
                    result.retries > 0 ? ` retries=${result.retries}` : "";
                console.error(
                    `[auto-gate] live decision status=${result.status} ` +
                    `latencyMs=${result.latencyMs}${retryTag}`,
                );
                if (result.status === "allow") {
                    await reply(config.replyMode); // "once" | "always"
                } else {
                    await reply("reject");
                }
                return;
            }

            // =============== Unknown mode — fail-closed =======================
            await handleUncertain(`unknown mode: ${config.mode}`);
        },
    };
};

export const AutoToolGatePlugin = server;

export default {
    id,
    server,
};

// ===========================================================================
// DUAL-PURPOSE SELF-TEST — stderr/audit-egress credential-leak regression.
//
// Run directly (`node auto-tool-gate.js` or `node --test auto-tool-gate.js`) to
// execute the suite. Import as a module -> NO tests run. Guard is an explicit
// __filename comparison so an accidental import (the plugin-loader path) cannot
// fire the suite.
//
// These tests prove a credential embedded in a tool-call-derived value CANNOT
// survive into the stderr audit line. console.error writes to stderr, which the
// OpenCode/server process writes to its stderr log — so we test the PURE
// helpers (summarizeArgs + the pattern-audit value expression) directly rather
// than capturing stderr. Each assert: the secret is ABSENT from the helper
// output, and (where applicable) a safe value is UNCHANGED (no false-positive
// over-redaction).
// ===========================================================================
const __isMain = path.resolve(process.argv[1] ?? "") === __filename;

if (__isMain) {
    // ===== summarizeArgs: tool.execute.before audit-line helper =====

    test("summarizeArgs: Bearer jwt in a Bash command is absent", () => {
        const jwt =
            "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.payload.signature";
        const out = summarizeArgs({
            command: `curl -H "Authorization: Bearer ${jwt}" https://api.example/v1`,
        });
        assert.equal(
            out.includes(jwt),
            false,
            "Bearer jwt in command must not survive into the audit line",
        );
        assert.match(out, /Bearer \[redacted\]/);
    });

    test("summarizeArgs: api_key in a Bash command is absent", () => {
        const secret = "sk-abcdefghij1234567890qrstuvwxyz";
        const out = summarizeArgs({
            command: `export api_key=${secret} && deploy`,
        });
        assert.equal(
            out.includes(secret),
            false,
            "api_key in command must not survive into the audit line",
        );
        assert.match(out, /api_key=\[redacted\]/);
    });

    test("summarizeArgs: secret in each of the 6 allowlisted fields is absent", () => {
        // A context-independent secret shape: a 40-hex-char blob is caught by
        // the standalone high-entropy rule ([0-9a-f]{32,}) regardless of the
        // surrounding field context. This proves the scrubber is APPLIED to
        // every allowlisted field (the regression under test), independent of
        // field-specific context matching.
        const hex = "0123456789abcdef0123456789abcdef01234567";
        const out = summarizeArgs({
            command: `echo ${hex}`,
            filePath: hex,
            pattern: hex,
            query: hex,
            url: `https://api.example/v1?t=${hex}`,
            workdir: hex,
        });
        assert.equal(
            out.includes(hex),
            false,
            "hex blob must be absent from every field in the audit summary",
        );
        assert.match(out, /\[redacted\]/);
    });

    test("summarizeArgs: secret in `path` field (filePath alias) is absent", () => {
        const secret = "sk-zyxwvutsrqponmlkjihgfedcba987654";
        const out = summarizeArgs({
            path: `token=${secret}`,
        });
        assert.equal(
            out.includes(secret),
            false,
            "secret in path field must not survive into the audit line",
        );
        assert.match(out, /token=\[redacted\]/);
    });

    test("summarizeArgs: safe command with no secret is unchanged (no over-redaction)", () => {
        const out = summarizeArgs({
            command: "rm -rf tmp/ && make build",
            workdir: ".",
        });
        assert.match(out, /command=rm -rf tmp\/ && make build/);
        assert.match(out, /workdir=\./);
    });

    test("summarizeArgs: normal file path is unchanged (no false-positive)", () => {
        const fp = "src/internal/runtime/substrate.go";
        const out = summarizeArgs({ filePath: fp });
        assert.equal(out, `path=${fp}`);
    });

    test("summarizeArgs: empty / non-object args -> empty string", () => {
        assert.equal(summarizeArgs(null), "");
        assert.equal(summarizeArgs(undefined), "");
        assert.equal(summarizeArgs({}), "args=0");
    });

    test("summarizeArgs: unknown tool with secret arg -> arg count only, no raw value", () => {
        const out = summarizeArgs({
            secret: "sk-leakshouldnotappear12345678",
            other: "Bearer xyz123abc456def789ghi012jkl345",
        });
        assert.match(out, /args=2/);
        assert.equal(
            out.includes("sk-leakshouldnotappear12345678"),
            false,
            "raw unknown-arg value must not be dumped",
        );
    });

    // ===== permission.ask pattern-audit value (enforce / live / audit branches) =====
    //
    // All three permission.ask branches build the audit line with the SAME
    // expression: scrubTruncate((input && input.pattern) || "", MAX_ARG_LEN).
    // We test that expression directly (it is the value interpolated into the
    // `pattern=${pattern}` field of the stderr audit line) so a secret in a
    // permission pattern cannot survive into any of the three audit lines.

    test("permission pattern: Bearer jwt is absent from the audit value", () => {
        const jwt =
            "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.payload.signature";
        const input = { type: "bash", pattern: `curl -H "Authorization: Bearer ${jwt}"` };
        // The exact expression the three permission.ask branches interpolate.
        const patternVal = scrubTruncate(
            (input && input.pattern) || "",
            MAX_ARG_LEN,
        );
        assert.equal(
            patternVal.includes(jwt),
            false,
            "Bearer jwt in permission pattern must not survive into the audit line",
        );
        assert.match(patternVal, /Bearer \[redacted\]/);
    });

    test("permission pattern: api_key is absent from the audit value", () => {
        const secret = "sk-abcdefghij1234567890qrstuvwxyz";
        const input = { type: "bash", pattern: `export api_key=${secret}` };
        const patternVal = scrubTruncate(
            (input && input.pattern) || "",
            MAX_ARG_LEN,
        );
        assert.equal(patternVal.includes(secret), false);
        assert.match(patternVal, /api_key=\[redacted\]/);
    });

    test("permission pattern: safe pattern with no secret is unchanged", () => {
        const input = { type: "bash", pattern: "rm -rf tmp/" };
        const patternVal = scrubTruncate(
            (input && input.pattern) || "",
            MAX_ARG_LEN,
        );
        assert.equal(patternVal, "rm -rf tmp/");
    });

    test("permission pattern: missing input -> empty string (no crash)", () => {
        assert.equal(scrubTruncate((null && null.pattern) || "", MAX_ARG_LEN), "");
        assert.equal(scrubTruncate((undefined && undefined.pattern) || "", MAX_ARG_LEN), "");
    });

    // ===== Config readers: two-file split model =====
    //
    // Filesystem tests for readConfig() (plugin-behavior) and readLlmConfig()
    // (LLM). Each test writes a temp file under tmp/auto-gate-config-test/,
    // resets BOTH caches via __resetConfigCaches(), and asserts fail-safe
    // behavior. The readers accept an injectable path (default the production
    // repo-configs path) so tests never touch the real config location.
    // Capture console.error to assert audit-spam / audit-line behavior.

    const TEST_CONFIG_DIR = path.resolve(
        repoRoot(),
        "tmp",
        "auto-gate-config-test",
    );

    function writeTestConfig(name, objOrString) {
        const body =
            typeof objOrString === "string"
                ? objOrString
                : JSON.stringify(objOrString);
        fs.writeFileSync(path.join(TEST_CONFIG_DIR, name), body, "utf8");
    }

    function testConfigPath(name) {
        return path.join(TEST_CONFIG_DIR, name);
    }

    // Ensure the test dir exists for the reader tests below (idempotent).
    fs.mkdirSync(TEST_CONFIG_DIR, { recursive: true });

    // Silence + capture console.error so a missing-file / invalid-JSON audit
    // line does not pollute test output, and so we can assert it fired (or not).
    function captureErrors(fn) {
        const errors = [];
        const orig = console.error;
        console.error = (msg) => errors.push(msg);
        try {
            fn(errors);
        } finally {
            console.error = orig;
        }
    }

    test("readConfig (plugin): missing file -> fail-safe defaults {enabled:true, mode:audit}", () => {
        __resetConfigCaches();
        const cfg = readConfig(testConfigPath("no-such-plugin.json"));
        assert.deepEqual(cfg, {
            enabled: true,
            mode: "audit",
            stubVerdict: "block",
            promptFile: "",
            replyMode: "once",
            onUncertain: "reject",
        });
    });

    test("readConfig (plugin): valid partial -> merged over defaults", () => {
        __resetConfigCaches();
        writeTestConfig("plugin-partial.json", { mode: "enforce" });
        const cfg = readConfig(testConfigPath("plugin-partial.json"));
        assert.deepEqual(cfg, {
            enabled: true,
            mode: "enforce",
            stubVerdict: "block",
            promptFile: "",
            replyMode: "once",
            onUncertain: "reject",
        });
    });

    test("readConfig (plugin): ignores LLM fields entirely (clean cut)", () => {
        __resetConfigCaches();
        writeTestConfig("plugin-with-llm.json", {
            enabled: true,
            mode: "live",
            modelEndpoint: "https://should-be-ignored.example",
            model: "ignored",
            apiKeyEnv: "IGNORED_KEY",
            timeoutMs: 9999,
        });
        const cfg = readConfig(testConfigPath("plugin-with-llm.json"));
        // Returns ONLY the 4 plugin-behavior fields; LLM keys absent.
        assert.deepEqual(cfg, {
            enabled: true,
            mode: "live",
            stubVerdict: "block",
            promptFile: "",
            replyMode: "once",
            onUncertain: "reject",
        });
        assert.equal(
            "modelEndpoint" in cfg,
            false,
            "LLM fields must not appear in plugin config",
        );
    });

    test("readConfig (plugin): invalid JSON -> defaults + audit line", () => {
        __resetConfigCaches();
        writeTestConfig("plugin-invalid.json", "{ not valid json");
        captureErrors((errors) => {
            const cfg = readConfig(testConfigPath("plugin-invalid.json"));
            assert.deepEqual(cfg, {
                enabled: true,
                mode: "audit",
                stubVerdict: "block",
                promptFile: "",
                replyMode: "once",
                onUncertain: "reject",
            });
            assert.equal(errors.length, 1, "present-but-invalid must warn once");
            assert.match(errors[0], /invalid JSON/);
        });
    });

    // ===== F3 fail-safe: a JSON parse that does NOT yield a plain object =====
    //
    // A file containing the literal `null` (or an array, or a bare primitive)
    // parses successfully but is not a config object — the normalizer would
    // throw on property access (e.g. `parsed.enabled` on null). The reader must
    // return fail-safe defaults, never throw, and use the SAME deduped "invalid"
    // audit path as a syntactically broken file.

    test("readConfig (plugin): literal null -> defaults, no throw + ONE deduped audit line", () => {
        __resetConfigCaches();
        writeTestConfig("plugin-null.json", "null");
        captureErrors((errors) => {
            const a = readConfig(testConfigPath("plugin-null.json"));
            const b = readConfig(testConfigPath("plugin-null.json"));
            assert.deepEqual(a, {
                enabled: true,
                mode: "audit",
                stubVerdict: "block",
                promptFile: "",
                replyMode: "once",
                onUncertain: "reject",
            });
            assert.deepEqual(b, a, "second read of same bad file still returns defaults");
            // Dedup contract (same as invalid JSON): one audit line across both reads.
            assert.equal(errors.length, 1, "non-object parse must warn once (deduped)");
            assert.match(errors[0], /invalid JSON/);
        });
    });

    test("readConfig (plugin): array / primitive shapes -> defaults, no throw", () => {
        for (const body of ["[]", "42", "\"oops\"", "true"]) {
            __resetConfigCaches();
            writeTestConfig("plugin-shape.json", body);
            captureErrors((errors) => {
                const cfg = readConfig(testConfigPath("plugin-shape.json"));
                assert.deepEqual(cfg, {
                    enabled: true,
                    mode: "audit",
                    stubVerdict: "block",
                    promptFile: "",
                    replyMode: "once",
                    onUncertain: "reject",
                });
                assert.equal(errors.length, 1, `body ${body} must warn once`);
            });
        }
    });

    test("readLlmConfig: missing file -> defaults, NO throw, NO audit spam", () => {
        __resetConfigCaches();
        captureErrors((errors) => {
            const cfg = readLlmConfig(testConfigPath("no-such-llm.json"));
            assert.deepEqual(cfg, {
                modelEndpoint: "",
                model: "",
                apiKeyEnv: "AUTO_GATE_API_KEY",
                timeoutMs: 8000,
                maxRetries: 1,
                retryDelayMs: 500,
            });
            assert.equal(
                errors.length,
                0,
                "missing LLM file is normal — must NOT emit audit spam",
            );
        });
    });

    test("readLlmConfig: valid file -> merged fields", () => {
        __resetConfigCaches();
        writeTestConfig("llm-valid.json", {
            modelEndpoint: "https://provider.example/v1/chat/completions",
            model: "test-model",
            apiKeyEnv: "MY_GATE_KEY",
            timeoutMs: 4000,
            maxRetries: 3,
            retryDelayMs: 250,
        });
        const cfg = readLlmConfig(testConfigPath("llm-valid.json"));
        assert.deepEqual(cfg, {
            modelEndpoint: "https://provider.example/v1/chat/completions",
            model: "test-model",
            apiKeyEnv: "MY_GATE_KEY",
            timeoutMs: 4000,
            maxRetries: 3,
            retryDelayMs: 250,
        });
    });

    test("readLlmConfig: partial config merges over defaults", () => {
        __resetConfigCaches();
        writeTestConfig("llm-partial.json", { modelEndpoint: "https://x" });
        const cfg = readLlmConfig(testConfigPath("llm-partial.json"));
        assert.deepEqual(cfg, {
            modelEndpoint: "https://x",
            model: "",
            apiKeyEnv: "AUTO_GATE_API_KEY",
            timeoutMs: 8000,
            maxRetries: 1,
            retryDelayMs: 500,
        });
    });

    test("readLlmConfig: invalid JSON -> defaults + ONE audit line", () => {
        __resetConfigCaches();
        writeTestConfig("llm-invalid.json", "{ broken json");
        captureErrors((errors) => {
            const cfg = readLlmConfig(testConfigPath("llm-invalid.json"));
            assert.deepEqual(cfg, {
                modelEndpoint: "",
                model: "",
                apiKeyEnv: "AUTO_GATE_API_KEY",
                timeoutMs: 8000,
                maxRetries: 1,
                retryDelayMs: 500,
            });
            assert.equal(
                errors.length,
                1,
                "present-but-invalid LLM file must emit ONE audit line",
            );
            assert.match(errors[0], /invalid JSON/);
        });
    });

    // ===== F3 fail-safe (LLM side): non-object parse results =====

    test("readLlmConfig: literal null -> defaults, no throw + ONE deduped audit line", () => {
        __resetConfigCaches();
        writeTestConfig("llm-null.json", "null");
        captureErrors((errors) => {
            const a = readLlmConfig(testConfigPath("llm-null.json"));
            const b = readLlmConfig(testConfigPath("llm-null.json"));
            assert.deepEqual(a, {
                modelEndpoint: "",
                model: "",
                apiKeyEnv: "AUTO_GATE_API_KEY",
                timeoutMs: 8000,
                maxRetries: 1,
                retryDelayMs: 500,
            });
            assert.deepEqual(b, a, "second read of same bad file still returns defaults");
            // A PRESENT-but-non-object file is not the normal "no live setup" case,
            // so it must emit the audit line once (deduped across both reads).
            assert.equal(errors.length, 1, "non-object parse must warn once (deduped)");
            assert.match(errors[0], /invalid JSON/);
        });
    });

    test("readLlmConfig: array / primitive shapes -> defaults, no throw", () => {
        for (const body of ["[]", "42", "\"oops\"", "true"]) {
            __resetConfigCaches();
            writeTestConfig("llm-shape.json", body);
            captureErrors((errors) => {
                const cfg = readLlmConfig(testConfigPath("llm-shape.json"));
                assert.deepEqual(cfg, {
                    modelEndpoint: "",
                    model: "",
                    apiKeyEnv: "AUTO_GATE_API_KEY",
                    timeoutMs: 8000,
                    maxRetries: 1,
                    retryDelayMs: 500,
                });
                assert.equal(errors.length, 1, `body ${body} must warn once`);
            });
        }
    });

    test("readLlmConfig: apiKeyEnv default is AUTO_GATE_API_KEY", () => {
        __resetConfigCaches();
        writeTestConfig("llm-no-env.json", {
            modelEndpoint: "https://x",
            model: "m",
        });
        const cfg = readLlmConfig(testConfigPath("llm-no-env.json"));
        assert.equal(cfg.apiKeyEnv, "AUTO_GATE_API_KEY");
    });

    test("readLlmConfig: invalid types fall back to defaults", () => {
        __resetConfigCaches();
        // Wrong types: modelEndpoint as number, model as null, apiKeyEnv empty,
        // timeoutMs as negative number, maxRetries as negative, retryDelayMs as
        // a non-numeric string. Each must normalize to its default.
        writeTestConfig("llm-badtypes.json", {
            modelEndpoint: 123,
            model: null,
            apiKeyEnv: "",
            timeoutMs: -5,
            maxRetries: -2,
            retryDelayMs: "fast",
        });
        const cfg = readLlmConfig(testConfigPath("llm-badtypes.json"));
        assert.deepEqual(cfg, {
            modelEndpoint: "",
            model: "",
            apiKeyEnv: "AUTO_GATE_API_KEY",
            timeoutMs: 8000,
            maxRetries: 1,
            retryDelayMs: 500,
        });
    });

    test("readLlmConfig: mtime cache returns SAME object on unchanged file", () => {
        __resetConfigCaches();
        writeTestConfig("llm-cache.json", { model: "cached-model" });
        const a = readLlmConfig(testConfigPath("llm-cache.json"));
        const b = readLlmConfig(testConfigPath("llm-cache.json"));
        assert.equal(
            a,
            b,
            "unchanged file (same mtime) must return the SAME cached object",
        );
    });

    test("readLlmConfig: re-read after file change sees new content", (t, done) => {
        __resetConfigCaches();
        writeTestConfig("llm-mutate.json", { model: "first" });
        const a = readLlmConfig(testConfigPath("llm-mutate.json"));
        assert.equal(a.model, "first");
        // Bump mtime by writing new content, then ensure a fresh mtime
        // (statSync resolution is ms-level; nudge with a tiny delay).
        setTimeout(() => {
            writeTestConfig("llm-mutate.json", { model: "second" });
            const b = readLlmConfig(testConfigPath("llm-mutate.json"));
            assert.equal(b.model, "second", "changed file must re-read");
            done();
        }, 20);
    });

    test("merged call-site: {...readConfig(), ...readLlmConfig()} yields all 12 fields", () => {
        __resetConfigCaches();
        writeTestConfig("merge-plugin.json", {
            enabled: true,
            mode: "live",
            promptFile: "/x",
        });
        writeTestConfig("merge-llm.json", {
            modelEndpoint: "https://x",
            model: "m",
            apiKeyEnv: "K",
            timeoutMs: 3000,
            maxRetries: 2,
            retryDelayMs: 750,
        });
        const merged = {
            ...readConfig(testConfigPath("merge-plugin.json")),
            ...readLlmConfig(testConfigPath("merge-llm.json")),
        };
        assert.deepEqual(merged, {
            enabled: true,
            mode: "live",
            stubVerdict: "block",
            promptFile: "/x",
            replyMode: "once",
            onUncertain: "reject",
            modelEndpoint: "https://x",
            model: "m",
            apiKeyEnv: "K",
            timeoutMs: 3000,
            maxRetries: 2,
            retryDelayMs: 750,
        });
    });

    // ===== maxRetries / retryDelayMs reader tests (new fields) =====

    test("readLlmConfig: missing maxRetries/retryDelayMs default to 1/500", () => {
        __resetConfigCaches();
        writeTestConfig("llm-no-retry.json", {
            modelEndpoint: "https://x",
            model: "m",
        });
        const cfg = readLlmConfig(testConfigPath("llm-no-retry.json"));
        assert.equal(cfg.maxRetries, 1, "default maxRetries is 1");
        assert.equal(cfg.retryDelayMs, 500, "default retryDelayMs is 500");
    });

    test("readLlmConfig: maxRetries:0 is preserved (NOT coerced to default)", () => {
        // 0 is a valid, meaningful value (single attempt, the pre-retry
        // behavior). It must NOT be normalized to the default 1.
        __resetConfigCaches();
        writeTestConfig("llm-no-retry-zero.json", {
            modelEndpoint: "https://x",
            model: "m",
            maxRetries: 0,
            retryDelayMs: 0,
        });
        const cfg = readLlmConfig(testConfigPath("llm-no-retry-zero.json"));
        assert.equal(cfg.maxRetries, 0, "maxRetries:0 must be preserved");
        assert.equal(cfg.retryDelayMs, 0, "retryDelayMs:0 must be preserved");
    });

    test("readLlmConfig: numeric-string maxRetries/retryDelayMs are coerced to ints", () => {
        __resetConfigCaches();
        writeTestConfig("llm-retry-str.json", {
            modelEndpoint: "https://x",
            model: "m",
            maxRetries: "5",
            retryDelayMs: "1200",
        });
        const cfg = readLlmConfig(testConfigPath("llm-retry-str.json"));
        assert.equal(cfg.maxRetries, 5);
        assert.equal(cfg.retryDelayMs, 1200);
    });

    test("readLlmConfig: float maxRetries/retryDelayMs are floored", () => {
        __resetConfigCaches();
        writeTestConfig("llm-retry-float.json", {
            modelEndpoint: "https://x",
            model: "m",
            maxRetries: 2.9,
            retryDelayMs: 500.7,
        });
        const cfg = readLlmConfig(testConfigPath("llm-retry-float.json"));
        assert.equal(cfg.maxRetries, 2);
        assert.equal(cfg.retryDelayMs, 500);
    });

    test("readLlmConfig: retry fields merge into the live call-site config", () => {
        // The production merge is `{...config, ...readLlmConfig()}`. This pins
        // that maxRetries/retryDelayMs survive the spread (the LLM spread wins
        // over plugin config, and these fields only exist on the LLM side).
        __resetConfigCaches();
        writeTestConfig("merge-plugin2.json", {
            enabled: true,
            mode: "live",
        });
        writeTestConfig("merge-llm2.json", {
            modelEndpoint: "https://x",
            model: "m",
            apiKeyEnv: "K",
            timeoutMs: 3000,
            maxRetries: 4,
            retryDelayMs: 1000,
        });
        const liveConfig = {
            ...readConfig(testConfigPath("merge-plugin2.json")),
            ...readLlmConfig(testConfigPath("merge-llm2.json")),
        };
        assert.equal(liveConfig.maxRetries, 4, "maxRetries must reach the live config");
        assert.equal(liveConfig.retryDelayMs, 1000, "retryDelayMs must reach the live config");
        // The two config sources must not collide: plugin config has 6 fields,
        // LLM config has 6; the merged object has all 12 (6 plugin + 6 LLM).
        assert.equal(Object.keys(liveConfig).length, 12);
    });

    // ===================================================================
    // EVENT HOOK TESTS — the PRIMARY enforcement surface.
    //
    // Each test builds hooks via server({client, directory, configPath,
    // llmConfigPath}), invokes hooks["event"]({event}) with a fake
    // permission.asked event, and asserts the reply call args on the fake
    // client's postSessionIdPermissionsPermissionId.
    // ===================================================================

    // Helper: fake SDK client recording permission replies.
    function makeEventClient(opts = {}) {
        const replies = [];
        const transcript =
            opts.transcript ||
            [
                {
                    info: { role: "user" },
                    parts: [{ type: "text", text: "hello world" }],
                },
                {
                    info: { role: "assistant" },
                    parts: [{ type: "text", text: "hi there" }],
                },
            ];
        const client = {
            postSessionIdPermissionsPermissionId: async (args) => {
                replies.push(args);
                return opts.replyError
                    ? {
                          data: undefined,
                          error: { message: "stub reply error" },
                      }
                    : { data: {}, error: undefined };
            },
            session: {
                messages: async () => ({
                    data: transcript,
                    error: undefined,
                }),
            },
        };
        return { client, replies };
    }

    // Helper: fake permission.asked bus event.
    // `rawPermission` (if set) overrides the `permission` field verbatim — used
    // to exercise the SDK wire shape where `permission` is a STRING name (e.g.
    // "bash") rather than an object with a `.type`. When unset, the helper
    // builds the legacy object shape { type: permType }.
    function makeAskedEvent(opts = {}) {
        return {
            type: "permission.asked",
            properties: {
                id: opts.id || "req-evt-1",
                sessionID: opts.sessionID || "sess-evt-1",
                permission:
                    opts.rawPermission !== undefined
                        ? opts.rawPermission
                        : { type: opts.permType || "bash" },
                patterns: opts.patterns || ["ls -la"],
                metadata: opts.metadata || {},
                always: false,
                tool: "bash",
            },
        };
    }

    // Helper: write configs, reset caches, build hooks + client holder.
    async function setupEventTest(pluginCfg, llmCfg, clientOpts) {
        const tag = Math.random().toString(36).slice(2, 8);
        const pName = `evt-p-${tag}.json`;
        const lName = `evt-l-${tag}.json`;
        writeTestConfig(pName, pluginCfg);
        if (llmCfg) writeTestConfig(lName, llmCfg);
        __resetConfigCaches();
        const holder = makeEventClient(clientOpts || {});
        const hooks = await server({
            client: holder.client,
            directory: TEST_CONFIG_DIR,
            configPath: testConfigPath(pName),
            llmConfigPath: testConfigPath(
                llmCfg ? lName : "no-such-llm.json",
            ),
        });
        return { hooks, replies: holder.replies };
    }

    // Mock globalThis.fetch to return a fixed verdict. Returns a restore fn.
    function mockFetchVerdict(verdictText) {
        const orig = globalThis.fetch;
        globalThis.fetch = async () => ({
            ok: true,
            status: 200,
            json: async () => ({
                choices: [{ message: { content: verdictText } }],
            }),
        });
        return () => {
            globalThis.fetch = orig;
        };
    }

    // Mock globalThis.fetch to throw. Returns a restore fn.
    function mockFetchThrow(msg) {
        const orig = globalThis.fetch;
        globalThis.fetch = async () => {
            throw new Error(msg);
        };
        return () => {
            globalThis.fetch = orig;
        };
    }

    // Mock globalThis.fetch to capture the request body (the JSON-stringified
    // OpenAI-compatible payload) while returning a fixed verdict. Used to
    // assert what the live path actually SERIALIZES to the model. Returns a
    // restore fn. The captured array receives each request's `body` string.
    function mockFetchCapture(captured, verdictText = "<block>no</block>") {
        const orig = globalThis.fetch;
        globalThis.fetch = async (_url, opts) => {
            if (opts && typeof opts.body === "string") captured.push(opts.body);
            return {
                ok: true,
                status: 200,
                json: async () => ({
                    choices: [{ message: { content: verdictText } }],
                }),
            };
        };
        return () => {
            globalThis.fetch = orig;
        };
    }

    // One-time setup for live tests: prompt file + API key env.
    writeTestConfig("evt-classifier-prompt.txt", "You are a classifier.");
    const _origApiKey = process.env.AUTO_GATE_API_KEY;
    process.env.AUTO_GATE_API_KEY = "test-key-for-events";

    // --- audit mode ---

    test("event: audit mode → no reply call", async () => {
        const { hooks, replies } = await setupEventTest({ mode: "audit" });
        await hooks["event"]({ event: makeAskedEvent() });
        assert.equal(replies.length, 0, "audit mode must NOT reply");
    });

    // --- enforce mode ---

    test("event: enforce stubVerdict:allow → reply once (default)", async () => {
        const { hooks, replies } = await setupEventTest({
            mode: "enforce",
            stubVerdict: "allow",
        });
        await hooks["event"]({ event: makeAskedEvent() });
        assert.equal(replies.length, 1);
        assert.equal(replies[0].body.response, "once");
        // Verify path args are threaded correctly.
        assert.equal(replies[0].path.id, "sess-evt-1");
        assert.equal(replies[0].path.permissionID, "req-evt-1");
    });

    test("event: enforce stubVerdict:allow + replyMode:always → reply always", async () => {
        const { hooks, replies } = await setupEventTest({
            mode: "enforce",
            stubVerdict: "allow",
            replyMode: "always",
        });
        await hooks["event"]({ event: makeAskedEvent() });
        assert.equal(replies.length, 1);
        assert.equal(replies[0].body.response, "always");
    });

    test("event: enforce stubVerdict:block → reply reject", async () => {
        const { hooks, replies } = await setupEventTest({
            mode: "enforce",
            stubVerdict: "block",
        });
        await hooks["event"]({ event: makeAskedEvent() });
        assert.equal(replies.length, 1);
        assert.equal(replies[0].body.response, "reject");
    });

    test("event: enforce stubVerdict:fail → reply reject (fail-closed)", async () => {
        const { hooks, replies } = await setupEventTest({
            mode: "enforce",
            stubVerdict: "fail",
        });
        await hooks["event"]({ event: makeAskedEvent() });
        assert.equal(replies.length, 1);
        assert.equal(replies[0].body.response, "reject");
    });

    // --- live mode ---

    test("event: live allow → reply once", async () => {
        const { hooks, replies } = await setupEventTest(
            {
                mode: "live",
                promptFile: testConfigPath("evt-classifier-prompt.txt"),
            },
            {
                modelEndpoint: "http://mock-llm",
                model: "test-model",
                maxRetries: 0,
            },
        );
        const restore = mockFetchVerdict("<block>no</block>");
        try {
            await hooks["event"]({ event: makeAskedEvent() });
        } finally {
            restore();
        }
        assert.equal(replies.length, 1);
        assert.equal(replies[0].body.response, "once");
    });

    test("event: live deny → reply reject", async () => {
        const { hooks, replies } = await setupEventTest(
            {
                mode: "live",
                promptFile: testConfigPath("evt-classifier-prompt.txt"),
            },
            {
                modelEndpoint: "http://mock-llm",
                model: "test-model",
                maxRetries: 0,
            },
        );
        const restore = mockFetchVerdict("<block>yes</block>");
        try {
            await hooks["event"]({ event: makeAskedEvent() });
        } finally {
            restore();
        }
        assert.equal(replies.length, 1);
        assert.equal(replies[0].body.response, "reject");
    });

    test("event: live misconfig (no modelEndpoint) → reply reject", async () => {
        const { hooks, replies } = await setupEventTest(
            {
                mode: "live",
                promptFile: testConfigPath("evt-classifier-prompt.txt"),
            },
            { modelEndpoint: "", model: "test-model" },
        );
        await hooks["event"]({ event: makeAskedEvent() });
        assert.equal(replies.length, 1);
        assert.equal(replies[0].body.response, "reject");
    });

    test("event: live fetch throw → reply reject (fail-closed)", async () => {
        const { hooks, replies } = await setupEventTest(
            {
                mode: "live",
                promptFile: testConfigPath("evt-classifier-prompt.txt"),
            },
            {
                modelEndpoint: "http://mock-llm",
                model: "test-model",
                maxRetries: 0,
            },
        );
        const restore = mockFetchThrow("network down");
        try {
            await hooks["event"]({ event: makeAskedEvent() });
        } finally {
            restore();
        }
        assert.equal(replies.length, 1);
        assert.equal(replies[0].body.response, "reject");
    });

    // --- onUncertain behavior ---

    test("event: onUncertain:passthrough + live misconfig → NO reply", async () => {
        const { hooks, replies } = await setupEventTest(
            {
                mode: "live",
                onUncertain: "passthrough",
                promptFile: testConfigPath("evt-classifier-prompt.txt"),
            },
            { modelEndpoint: "", model: "test-model" },
        );
        await hooks["event"]({ event: makeAskedEvent() });
        assert.equal(
            replies.length,
            0,
            "passthrough must NOT reply on misconfig failure",
        );
    });

    test("event: default onUncertain:reject + live misconfig → reply reject", async () => {
        const { hooks, replies } = await setupEventTest(
            {
                mode: "live",
                onUncertain: "reject",
                promptFile: testConfigPath("evt-classifier-prompt.txt"),
            },
            { modelEndpoint: "", model: "test-model" },
        );
        await hooks["event"]({ event: makeAskedEvent() });
        assert.equal(replies.length, 1);
        assert.equal(replies[0].body.response, "reject");
    });

    // --- early-return paths ---

    test("event: non-permission.asked event → early return, no reply", async () => {
        const { hooks, replies } = await setupEventTest({
            mode: "enforce",
            stubVerdict: "allow",
        });
        await hooks["event"]({
            event: { type: "session.created", properties: {} },
        });
        assert.equal(replies.length, 0, "non-asked events must be ignored");
    });

    test("event: enabled:false → early return, no reply", async () => {
        const { hooks, replies } = await setupEventTest({
            enabled: false,
            mode: "enforce",
            stubVerdict: "allow",
        });
        await hooks["event"]({ event: makeAskedEvent() });
        assert.equal(replies.length, 0, "disabled plugin must not reply");
    });

    // --- egress: credential-leak regression ---

    test("event: egress — secret in patterns[0] absent from audit line", async () => {
        const jwt =
            "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.payload.signature";
        const { hooks } = await setupEventTest({ mode: "audit" });
        const errors = [];
        const orig = console.error;
        console.error = (msg) => errors.push(msg);
        try {
            await hooks["event"]({
                event: makeAskedEvent({
                    patterns: [
                        `curl -H "Authorization: Bearer ${jwt}" https://api`,
                    ],
                }),
            });
        } finally {
            console.error = orig;
        }
        const combined = errors.join("\n");
        assert.equal(
            combined.includes(jwt),
            false,
            "jwt must NOT survive into the event audit line",
        );
        assert.match(combined, /\[redacted\]/);
    });

    // --- reply error logging ---

    test("event: reply r.error → logged, does not throw", async () => {
        const { hooks, replies } = await setupEventTest(
            { mode: "enforce", stubVerdict: "allow" },
            null,
            { replyError: true },
        );
        const errors = [];
        const orig = console.error;
        console.error = (msg) => errors.push(msg);
        try {
            // Must NOT throw despite the reply error return.
            await hooks["event"]({ event: makeAskedEvent() });
        } finally {
            console.error = orig;
        }
        assert.equal(replies.length, 1, "reply was still attempted");
        assert.equal(
            errors.some((e) => /permission reply failed/.test(e)),
            true,
            "r.error must be logged",
        );
    });

    // --- new config fields: normalization tests ---

    test("readConfig (plugin): invalid replyMode → default once", () => {
        __resetConfigCaches();
        writeTestConfig("evt-bad-reply.json", { replyMode: "forever" });
        const cfg = readConfig(testConfigPath("evt-bad-reply.json"));
        assert.equal(cfg.replyMode, "once");
    });

    test("readConfig (plugin): invalid onUncertain → default reject", () => {
        __resetConfigCaches();
        writeTestConfig("evt-bad-uncertain.json", { onUncertain: "maybe" });
        const cfg = readConfig(testConfigPath("evt-bad-uncertain.json"));
        assert.equal(cfg.onUncertain, "reject");
    });

    test("readConfig (plugin): replyMode:always + onUncertain:passthrough preserved", () => {
        __resetConfigCaches();
        writeTestConfig("evt-valid-new.json", {
            replyMode: "always",
            onUncertain: "passthrough",
        });
        const cfg = readConfig(testConfigPath("evt-valid-new.json"));
        assert.equal(cfg.replyMode, "always");
        assert.equal(cfg.onUncertain, "passthrough");
    });

    // ===================================================================
    // FOLLOW-UP FIXES (commit 82e37c89 reviewer findings):
    //   F1 — permission-type shape: the SDK wire type carries `permission` as
    //        a STRING name, not an object with `.type`. The defensive
    //        normalizer must yield the real type (not "unknown").
    //   F2 — reply robustness: a thrown reply (or a missing client) must NOT
    //        propagate out of the event hook (which would hang headless mode).
    // ===================================================================

    // F1 — string permission shape (today's real SDK wire type). Asserts BOTH
    // the audit/log line AND the live-mode serialized egress carry the real
    // type (not the collapsed "unknown").
    test("F1: string permission \"bash\" -> audit line + serialized live input carry type=bash", async () => {
        // (a) audit mode: the audit/log line carries type=bash, not unknown.
        {
            const { hooks } = await setupEventTest({ mode: "audit" });
            const errors = [];
            const orig = console.error;
            console.error = (msg) => errors.push(msg);
            try {
                await hooks["event"]({
                    event: makeAskedEvent({ rawPermission: "bash" }),
                });
            } finally {
                console.error = orig;
            }
            const line = errors.find((e) => /permission\.asked/.test(e));
            assert.ok(line, "audit line must be emitted");
            assert.match(line, /type=bash/);
            assert.equal(
                /type=unknown/.test(line),
                false,
                "string permission must NOT collapse to unknown",
            );
        }
        // (b) live mode: the serialized input that egresses to the model carries type=bash.
        {
            const { hooks } = await setupEventTest(
                {
                    mode: "live",
                    promptFile: testConfigPath("evt-classifier-prompt.txt"),
                },
                {
                    modelEndpoint: "http://mock-llm",
                    model: "test-model",
                    maxRetries: 0,
                },
            );
            const captured = [];
            const restore = mockFetchCapture(captured);
            try {
                await hooks["event"]({
                    event: makeAskedEvent({ rawPermission: "bash" }),
                });
            } finally {
                restore();
            }
            assert.ok(captured.length >= 1, "fetch must have been called");
            const body = JSON.parse(captured[0]);
            const userMsg = body.messages.find((m) => m.role === "user");
            assert.ok(userMsg, "user message must be present in the live request");
            assert.match(
                userMsg.content,
                /type=bash/,
                "serialized live input must carry type=bash",
            );
            assert.equal(
                /type=unknown/.test(userMsg.content),
                false,
                "serialized live input must NOT carry type=unknown",
            );
        }
    });

    // F1 — object permission shape (defensive hedge against an upstream change).
    test("F1: object permission {type:\"edit\"} -> audit line type=edit", async () => {
        const { hooks } = await setupEventTest({ mode: "audit" });
        const errors = [];
        const orig = console.error;
        console.error = (msg) => errors.push(msg);
        try {
            await hooks["event"]({
                event: makeAskedEvent({ rawPermission: { type: "edit" } }),
            });
        } finally {
            console.error = orig;
        }
        const line = errors.find((e) => /permission\.asked/.test(e));
        assert.ok(line, "audit line must be emitted");
        assert.match(line, /type=edit/);
    });

    // F1 — missing permission field -> type=unknown, no crash.
    test("F1: missing permission field -> audit line type=unknown", async () => {
        const { hooks } = await setupEventTest({ mode: "audit" });
        const errors = [];
        const orig = console.error;
        console.error = (msg) => errors.push(msg);
        try {
            const ev = makeAskedEvent();
            delete ev.properties.permission;
            await hooks["event"]({ event: ev });
        } finally {
            console.error = orig;
        }
        const line = errors.find((e) => /permission\.asked/.test(e));
        assert.ok(line, "audit line must be emitted");
        assert.match(line, /type=unknown/);
    });

    // F2 — a THROWN reply is caught + logged; the hook completes cleanly
    // (no unhandled rejection, no headless hang).
    test("F2: throwing reply client -> hook completes, throw logged", async () => {
        const tag = Math.random().toString(36).slice(2, 8);
        const pName = `evt-throw-${tag}.json`;
        writeTestConfig(pName, { mode: "enforce", stubVerdict: "allow" });
        __resetConfigCaches();
        const throwingClient = {
            postSessionIdPermissionsPermissionId: async () => {
                throw new Error("transport boom");
            },
            session: { messages: async () => ({ data: [], error: undefined }) },
        };
        const hooks = await server({
            client: throwingClient,
            directory: TEST_CONFIG_DIR,
            configPath: testConfigPath(pName),
            llmConfigPath: testConfigPath("no-such-llm.json"),
        });
        const errors = [];
        const orig = console.error;
        console.error = (msg) => errors.push(msg);
        try {
            // Must NOT throw — the reply helper catches the transport throw.
            await hooks["event"]({ event: makeAskedEvent() });
        } finally {
            console.error = orig;
        }
        assert.equal(
            errors.some((e) => /permission reply threw/.test(e)),
            true,
            "thrown reply must be logged",
        );
        assert.equal(
            errors.some((e) => /transport boom/.test(e)),
            true,
            "the thrown error message must appear in the log",
        );
    });

    // F2 — missing client entirely -> hook completes, "reply unavailable" logged.
    test("F2: no client -> hook completes, reply-unavailable logged", async () => {
        const tag = Math.random().toString(36).slice(2, 8);
        const pName = `evt-noclient-${tag}.json`;
        writeTestConfig(pName, { mode: "enforce", stubVerdict: "allow" });
        __resetConfigCaches();
        // No `client` threaded into the server factory.
        const hooks = await server({
            directory: TEST_CONFIG_DIR,
            configPath: testConfigPath(pName),
            llmConfigPath: testConfigPath("no-such-llm.json"),
        });
        const errors = [];
        const orig = console.error;
        console.error = (msg) => errors.push(msg);
        try {
            // Must NOT throw — the reply helper guards the missing client.
            await hooks["event"]({ event: makeAskedEvent() });
        } finally {
            console.error = orig;
        }
        assert.equal(
            errors.some((e) =>
                /permission reply unavailable: no client/.test(e),
            ),
            true,
            "missing client must log reply unavailable",
        );
    });

    // Restore the API key env after the live tests.
    process.env.AUTO_GATE_API_KEY = _origApiKey;
}
