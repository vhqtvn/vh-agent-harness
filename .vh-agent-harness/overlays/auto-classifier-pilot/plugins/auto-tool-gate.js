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
// TWO HOOKS, TWO SURFACES (verified against @opencode-ai/plugin + sdk types):
//
//   1. tool.execute.before  (input:{tool,sessionID,callID}, output:{args})
//      Sees EVERY tool call — including ones the permission table auto-allows
//      (those never reach permission.ask). Powers: block (throw) or passthrough
//      (bare return) ONLY. Cannot force-allow or force-ask. We use it purely to
//      observe the full tool-call stream and capture the arg summary. It stays
//      an OBSERVER in both modes.
//
//   2. permission.ask  (input:Permission, output:{status:"ask"|"deny"|"allow"})
//      The AUTHORITATIVE three-way permission decision. Fires only when the
//      permission table resolves to `ask` or no-match (table-`allow` =
//      fast-path that skips this hook; table-`deny`/shell-guard = hard floor
//      that blocks before this hook). Setting output.status: "allow" grants AND
//      skips the user prompt; "deny" blocks; "ask" (default) triggers the
//      interactive prompt. This maps EXACTLY onto the reference classifier's
//      three dispositions — which is why the enforce branch owns this hook.
//
// HARD-FLOOR INVARIANT: because permission.ask only fires for ask-routed calls
// (table-allow fast-paths past it; table-deny / shell-guard blocks before it),
// the classifier can NEVER override a static deny. It only ever decides the
// ask-routed subset. The static permission table is the first gate; the
// classifier runs strictly after it.
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
// Design source: researches/sources/2026-07-10-auto-mode-classifier-source-packet.md
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

// Operator-owned config path, repo-relative. The `repo-configs/` dir is where
// the harness already keeps operator-facing config-like data
// (allowed-commands.js, forbidden-patterns.js, forbidden-patterns.core.js,
// repo-recon-data.yml). The overlay does NOT render or seed this file — its
// absence is the documented fail-safe default.
const CONFIG_PATH = path.resolve(
    repoRoot(),
    ".opencode",
    "repo-configs",
    "auto-gate-config.json",
);

// Fail-safe defaults used when the config file is absent, unreadable, or
// invalid JSON. `enabled` is the master live kill-switch; `mode` is the
// behavior selector (`audit` = Phase 1 log-only; `enforce` = Phase 2 stub
// decision path; `live` = Phase 3b real-model decision path). `stubVerdict`
// drives the deterministic stub evaluator used in enforce mode.
//
// The `live`-mode fields all have FAIL-SAFE defaults: `modelEndpoint` and
// `model` default to empty (so a live call with no endpoint/model fail-closes
// to deny instead of hitting a garbage URL); `apiKeyEnv` defaults to the
// conventional env-var name; `timeoutMs` defaults to a conservative bound;
// `promptFile` defaults to unset (use the built-in generic classifier prompt).
const DEFAULT_CONFIG = Object.freeze({
    enabled: true,
    mode: "audit",
    stubVerdict: "block",
    // Phase 3b live-mode fields (only consulted when mode === "live").
    modelEndpoint: "", // required for live; empty -> fail-closed deny
    model: "", // required for live; empty -> fail-closed deny
    apiKeyEnv: "AUTO_GATE_API_KEY", // NAME of the env var only (never the value)
    timeoutMs: 8000, // hard timeout for the model HTTP call
    promptFile: "", // optional override path for the classifier system prompt
});

// mtime cache: stores the last successful parse plus a fallback-warning latch
// so a persistent failure (missing / invalid file) emits exactly ONE stderr
// audit line per failure STATE instead of spamming every tool call. A state
// transition (missing -> present -> invalid) re-warns once. Module-level on
// purpose — survives across hook invocations within one server process.
let configCache = {
    mtime: null, // last mtimeMs we parsed successfully (null until first hit)
    parsed: null, // last parsed + merged config object (null until first hit)
    fallbackReason: null, // null | "missing" | "unreadable" | "invalid"
};

// Read the live config with an mtime cache. Returns a parsed config object on
// every call — NEVER throws. Side effect: emits at most one `console.error`
// audit line per failure-state transition, so the operator learns their config
// isn't loading without drowning the log. A valid file is merged over the
// defaults field-by-field, so a partial config like `{"enabled": false}` still
// resolves every field (mode falls back to its default).
function readConfig() {
    let st;
    try {
        st = fs.statSync(CONFIG_PATH);
    } catch (_) {
        // Missing / unreadable metadata: ENOENT / EACCES / etc. Fail safe.
        if (configCache.fallbackReason !== "missing") {
            console.error(
                `[auto-gate-audit] config not found at ${CONFIG_PATH}; ` +
                `using fail-safe defaults ${JSON.stringify(DEFAULT_CONFIG)} ` +
                `(create the file to override).`,
            );
            configCache.fallbackReason = "missing";
        }
        return DEFAULT_CONFIG;
    }

    const mtimeMs = st.mtimeMs;
    // Fast path: unchanged since last successful parse AND not currently in a
    // fallback state — return the cached parsed object (single statSync cost).
    if (
        configCache.parsed &&
        configCache.mtime === mtimeMs &&
        !configCache.fallbackReason
    ) {
        return configCache.parsed;
    }

    let raw;
    try {
        raw = fs.readFileSync(CONFIG_PATH, "utf8");
    } catch (_) {
        if (configCache.fallbackReason !== "unreadable") {
            console.error(
                `[auto-gate-audit] config unreadable at ${CONFIG_PATH}; ` +
                `using fail-safe defaults ${JSON.stringify(DEFAULT_CONFIG)}.`,
            );
            configCache.fallbackReason = "unreadable";
        }
        return DEFAULT_CONFIG;
    }

    let parsed;
    try {
        parsed = JSON.parse(raw);
    } catch (_) {
        if (configCache.fallbackReason !== "invalid") {
            console.error(
                `[auto-gate-audit] config invalid JSON at ${CONFIG_PATH}; ` +
                `using fail-safe defaults ${JSON.stringify(DEFAULT_CONFIG)}.`,
            );
            configCache.fallbackReason = "invalid";
        }
        return DEFAULT_CONFIG;
    }

    // Successful parse: normalize + merge over defaults (so partial configs
    // resolve every field), latch the cache, clear any prior fallback state.
    const merged = {
        enabled:
            typeof parsed.enabled === "boolean"
                ? parsed.enabled
                : DEFAULT_CONFIG.enabled,
        mode:
            parsed.mode === "audit" ||
            parsed.mode === "enforce" ||
            parsed.mode === "live"
                ? parsed.mode
                : DEFAULT_CONFIG.mode,
        stubVerdict:
            parsed.stubVerdict === "allow" ||
            parsed.stubVerdict === "block" ||
            parsed.stubVerdict === "fail"
                ? parsed.stubVerdict
                : DEFAULT_CONFIG.stubVerdict,
        // Phase 3b live-mode fields. Each is fail-safe-normalized: an invalid
        // type falls back to the default, which for endpoint/model is empty
        // (so a misconfigured live call fail-closes to deny, not to a garbage
        // request). The API key VALUE is never read here — only the env-var
        // NAME, looked up at call time inside classifyLive.
        modelEndpoint:
            typeof parsed.modelEndpoint === "string"
                ? parsed.modelEndpoint
                : DEFAULT_CONFIG.modelEndpoint,
        model:
            typeof parsed.model === "string"
                ? parsed.model
                : DEFAULT_CONFIG.model,
        apiKeyEnv:
            typeof parsed.apiKeyEnv === "string" && parsed.apiKeyEnv
                ? parsed.apiKeyEnv
                : DEFAULT_CONFIG.apiKeyEnv,
        timeoutMs:
            typeof parsed.timeoutMs === "number" && parsed.timeoutMs > 0
                ? parsed.timeoutMs
                : DEFAULT_CONFIG.timeoutMs,
        promptFile:
            typeof parsed.promptFile === "string"
                ? parsed.promptFile
                : DEFAULT_CONFIG.promptFile,
    };
    configCache = {
        mtime: mtimeMs,
        parsed: merged,
        fallbackReason: null,
    };
    return merged;
}

// The factory receives the full PluginInput ({client, project, directory,
// worktree, serverUrl, $}) — same contract session-state.js relies on for
// client.session.todo(). We close over `client` (the OpenCode SDK client, used
// ONLY in mode:"live" to fetch the session transcript) and `directory` (the
// repo dir, passed as the SDK query param for transcript fetch). The audit and
// enforce branches never touch either.
export const server = async ({ client, directory } = {}) => {
    return {
        "tool.execute.before": async (input, output) => {
            // Live config — read on every call (mtime-cached, single statSync
            // in steady state). The operator can live-disable the plugin by
            // setting `enabled: false` in the config file; no OpenCode
            // restart, no re-render required.
            const config = readConfig();
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
            // Live config — read on every call (mtime-cached).
            const config = readConfig();
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

                // (1) Validate live config up front so a misconfigured live
                // mode fail-closes to deny with a CLEAR audit line instead of a
                // cryptic adapter error.
                if (!config.modelEndpoint) {
                    console.error(
                        "[auto-gate] live mode misconfigured: no modelEndpoint; fail-closed deny",
                    );
                    output.status = "deny";
                    return;
                }
                if (!config.model) {
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
                    result = await decideLive(config, serialized);
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
                console.error(
                    `[auto-gate] live decision status=${result.status} latencyMs=${result.latencyMs}`,
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
}
