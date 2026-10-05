// session-progress.test.js — behavioral tests for the session-progress-pilot
// pack (Phase 1). Drives the REAL source modules from
// templates/overlays/session-progress-pilot/ (the same bytes `make update`
// renders into .opencode/) with injectable clients/fetch/clock — black-box at
// the hook seam, white-box only through the documented __test seams.
//
// Groups follow the plan's sub-slices:
//   [ss2] hook boilerplate, config, accumulator, fail-open
//   [ss3] spend gate, polling exemption, mechanical fast path
//   [ss4] judge, semantic policy, deadline/staleness, leases
//
// Run: vh-agent-harness exec node --test tests/scripts/session-progress.test.js
//   (or: make test-js)

import { test } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const REPO = join(__dirname, "..", "..");
const PACK = join(REPO, "templates", "overlays", "session-progress-pilot");

const CONFIG_PATH = ".opencode/repo-configs/session-progress.local.json";

// Cache-busting re-import of the plugin (module state is per-instance).
async function loadPlugin() {
    const url = "file://" + join(PACK, "plugins", "session-progress.js") + "?t=" + Math.random();
    return import(url);
}

async function loadConfigModule() {
    const url = "file://" + join(PACK, "scripts", "session-progress-config.js") + "?t=" + Math.random();
    return import(url);
}

async function loadPolicy() {
    const url = "file://" + join(PACK, "scripts", "session-progress-policy.js") + "?t=" + Math.random();
    return import(url);
}

async function loadJudge() {
    const url = "file://" + join(PACK, "scripts", "session-progress-judge.js") + "?t=" + Math.random();
    return import(url);
}

function freshRoot() {
    return mkdtempSync(join(tmpdir(), "sp-test-"));
}

function writeConfig(root, obj) {
    const p = join(root, CONFIG_PATH);
    mkdirSync(dirname(p), { recursive: true });
    // Hermeticity pin (dual-form judge wiring): every fixture pins
    // judge.user_config_path to a nonexistent path so no plugin-path test
    // can ever read the OPERATOR's real user-level
    // ~/.config/vh-agent-harness/session-progress-llm.json — which would
    // inject real endpoint/model/key literals, real HTTP, and real LLM spend
    // into supposedly hermetic tests. Tests exercising the user-file layer
    // pass their own explicit user_config_path (bypassing this helper).
    const cfg = JSON.parse(JSON.stringify(obj));
    cfg.judge = { ...(cfg.judge || {}), user_config_path: join(root, "no-user-judge.json") };
    writeFileSync(p, JSON.stringify(cfg));
    return p;
}

// driveHook — call the plugin's before-hook once. Returns
// {threw: null} on allow (undefined return) or {threw: Error}.
async function driveHook(hooks, { tool = "bash", args = { command: "true" }, sessionID = "s1", callID = "c1" }) {
    const output = { args };
    try {
        await hooks["tool.execute.before"]({ tool, sessionID, callID }, output);
        return { threw: null, output };
    } catch (e) {
        return { threw: e, output };
    }
}

// A no-op client (no session history) — outcomes stay unknown.
function nullClient() {
    return null;
}

// histClient — a client whose session.messages resolves the given message list.
// CAPTURES every messages() request in `client.__requests` so tests can pin
// the request shape (the live-runtime finding: the SDK path key renamed
// `id` -> `sessionID` between opencode 1.18.5 and 1.18.34; the plugin must
// send BOTH eras' keys or enrichment silently 404s to fail-open null).
function histClient(messages) {
    const client = {
        __requests: [],
        session: {
            messages: async (req) => {
                client.__requests.push(req);
                return { data: messages, error: undefined };
            },
        },
    };
    return client;
}

// mkToolPart — one opencode ToolPart (schema E4).
function mkToolPart(callID, tool, state) {
    return { type: "tool", callID, tool, state };
}

// errState / failExitState — adverse outcome states.
function errState(errorText, input = {}) {
    return { status: "error", error: errorText, input, time: { start: 0, end: 1 } };
}
function failExitState(exit = 1, output = "boom") {
    return {
        status: "completed",
        input: {},
        output,
        title: "x",
        metadata: { exit },
        time: { start: 0, end: 1 },
    };
}

// ===========================================================================
// [ss2] Sub-slice 2 — hook boilerplate, config, accumulator, fail-open
// ===========================================================================

test("[ss2] server factory registers ONLY tool.execute.before", async () => {
    const mod = await loadPlugin();
    const hooks = await mod.server({});
    assert.deepEqual(Object.keys(hooks), ["tool.execute.before"]);
});

test("[ss2] defaults: absent config file is silent and yields off mode (fully inert)", async () => {
    const cfgmod = await loadConfigModule();
    cfgmod.__resetConfigCacheForTest();
    const errs = [];
    const orig = console.error;
    console.error = (...a) => errs.push(a.join(" "));
    try {
        const root = freshRoot(); // no config file
        const cfg = cfgmod.loadConfig(root);
        assert.equal(cfg.enabled, true);
        assert.equal(cfg.agents["*"], "off", "absent config: every agent off (opt-in only)");
        // Default pin: the absent-config judge deadline IS the documented
        // 20000 ms ceiling — the ceiling is the shipped default, not just a
        // clamp bound.
        assert.equal(cfgmod.normalizeConfig({}).judge.timeout_ms, 20000,
            "default judge deadline == the documented 20000 ms ceiling");
        assert.deepEqual(errs, [], "absent config must be SILENT");
        const cfg2 = cfgmod.loadConfig(root); // cached path also silent
        assert.deepEqual(errs, [], "cached reload still silent");
    } finally {
        console.error = orig;
    }
});

test("[ss2] valid config file overrides fields; mtime change reloads", async () => {
    const cfgmod = await loadConfigModule();
    cfgmod.__resetConfigCacheForTest();
    const root = freshRoot();
    writeConfig(root, { agents: { "*": "enforce" }, judge: { timeout_ms: 1500 } });
    let cfg = cfgmod.loadConfig(root);
    assert.equal(cfg.agents["*"], "enforce");
    assert.equal(cfg.judge.timeout_ms, 1500);
    // Same mtime: cached object identity (no re-read).
    const again = cfgmod.loadConfig(root);
    assert.equal(again, cfg);
    // Rewrite (new mtime): new value picked up on the next call.
    writeConfig(root, { agents: { "*": "audit" } });
    cfg = cfgmod.loadConfig(root);
    assert.equal(cfg.agents["*"], "audit");
});

test("[ss2] invalid config falls back to defaults with ONE deduped warn, never throws", async () => {
    const cfgmod = await loadConfigModule();
    cfgmod.__resetConfigCacheForTest();
    const root = freshRoot();
    const p = join(root, CONFIG_PATH);
    mkdirSync(dirname(p), { recursive: true });
    writeFileSync(p, "{ not json !!!");
    const errs = [];
    const orig = console.error;
    console.error = (...a) => errs.push(a.join(" "));
    try {
        const cfg = cfgmod.loadConfig(root);
        assert.equal(cfg.agents["*"], "off", "defaults after invalid file (inert, not audit)");
        cfgmod.loadConfig(root);
        cfgmod.loadConfig(root);
        assert.equal(errs.length, 1, "persistent failure warns exactly once");
        assert.ok(errs[0].includes("[session-progress]"), "warn carries plugin prefix");
        // Transition (invalid -> valid) re-warns nothing but picks up config.
        writeConfig(root, { agents: { "*": "enforce" } });
        const cfg2 = cfgmod.loadConfig(root);
        assert.equal(cfg2.agents["*"], "enforce");
    } finally {
        console.error = orig;
    }
});

test("[ss2] normalization clamps every bound field; unknown fields ignored", async () => {
    const cfgmod = await loadConfigModule();
    const cfg = cfgmod.normalizeConfig({
        judge: { timeout_ms: 10, retries: 5, min_looping_confidence: 5 },
        cadence: { min_interval_seconds: -3, min_new_signatures: 99999 },
        mechanical: { repeat_threshold: 1, window_seconds: 1e9, max_denials: 0 },
        concurrency: { per_session: 7, global: 0 },
        nonsense: { deep: { thing: 1 } },
        agents: { "*": "enforce", "build": "wat", "plan": "off" },
    });
    assert.equal(cfg.judge.timeout_ms, 250, "timeout floored");
    assert.equal(cfg.judge.retries, 1, "retries capped at 1");
    assert.equal(cfg.judge.min_looping_confidence, 1, "confidence capped at 1");
    assert.equal(cfg.cadence.min_interval_seconds, 1, "interval floored");
    assert.equal(cfg.cadence.min_new_signatures, 1000, "accrual capped");
    assert.equal(cfg.mechanical.repeat_threshold, 2, "threshold floored at 2");
    assert.equal(cfg.mechanical.window_seconds, 300, "window capped");
    assert.equal(cfg.mechanical.max_denials, 1, "max_denials floored at 1");
    assert.equal(cfg.concurrency.per_session, 1, "per_session pinned to 1");
    assert.equal(cfg.concurrency.global, 1, "global floored at 1");
    assert.equal(cfg.agents["*"], "enforce");
    assert.equal(cfg.agents["build"], undefined, "invalid agent value dropped");
    assert.equal(cfg.agents["plan"], "off", "valid agent value kept");
    assert.equal(cfg.nonsense, undefined, "unknown fields ignored");
});

// [B1-pin] The judge timeout ceiling is operator-set at 20000 ms (decision
// 2026-10-05, up from the original 2000 pin): the documented invariant is
// "deadline hit (<=20000 ms total including retry) -> the call is ALLOWED",
// so the deadline remains a safety property of the hot path — finite,
// bounded, configurable DOWN only, never up. The ceiling number changed
// because measured real-gateway latency (6–18 s across every sampled model,
// the configured kimi judge ~11 s) made the 2000 pin guarantee fail-open
// for all real judges; the invariant CLASS is unchanged. A configured value
// above 20000 must clamp to 20000 (commit-review B1 lineage: a clamp wider
// than the documented ceiling silently violates the invariant).
test("[ss2] judge.timeout_ms clamps DOWN to the pinned 20000 ms ceiling (50000 -> 20000)", async () => {
    const cfgmod = await loadConfigModule();
    const clamp = (v) => cfgmod.normalizeConfig({ judge: { timeout_ms: v } }).judge.timeout_ms;
    assert.equal(clamp(50000), 20000, "configured 50000 clamps to the 20000 ceiling");
    assert.equal(clamp(25000), 20000, "anything above 20000 clamps down");
    assert.equal(clamp(20001), 20000, "just over the ceiling clamps down");
    assert.equal(clamp(20000), 20000, "the ceiling itself is legal");
    assert.equal(clamp(12000), 12000, "in-range values pass through untouched");
    assert.equal(clamp(250), 250, "the floor remains 250");
});

test("[ss2] agents string shorthand and attribution requirements", async () => {
    const cfgmod = await loadConfigModule();
    const short = cfgmod.normalizeConfig({ agents: "enforce" });
    assert.deepEqual(short.agents, { "*": "enforce" });
    assert.equal(cfgmod.needsAgentAttribution(short), false, "wildcard-only needs no attribution");
    const specific = cfgmod.normalizeConfig({ agents: { "*": "audit", "build": "enforce" } });
    assert.equal(cfgmod.needsAgentAttribution(specific), true);
    assert.equal(cfgmod.resolveModeForAgent(specific, "build"), "enforce");
    assert.equal(cfgmod.resolveModeForAgent(specific, "other"), "audit");
});

// [off-fallback pins] The three config-level OFF fallbacks (post-269d9c9
// opt-in flip): every invalid/incomplete agents shape MUST normalize to a
// {"*":"off"} wildcard — the inert fail-open floor. Pinned directly with
// deepEqual on the OFF outcome (not merely "no throw") because these sites
// were previously verified statically by review only.
test("[ss2] agents OFF fallbacks: invalid shorthand, missing/invalid wildcard, defensive resolveModeForAgent", async () => {
    const cfgmod = await loadConfigModule();

    // (1) Invalid string shorthand -> {"*":"off"} (normalizeAgents string arm).
    assert.deepEqual(
        cfgmod.normalizeConfig({ agents: "bogus" }).agents,
        { "*": "off" },
        "invalid shorthand string falls back to off",
    );

    // (2) Object form with missing/invalid wildcard -> wildcard forced "off";
    // valid specific keys survive but the effective default stays off.
    assert.deepEqual(
        cfgmod.normalizeConfig({ agents: {} }).agents,
        { "*": "off" },
        "empty agents object forces the off wildcard",
    );
    assert.deepEqual(
        cfgmod.normalizeConfig({ agents: { build: "enforce" } }).agents,
        { "*": "off", build: "enforce" },
        "missing wildcard forces off (specific key kept)",
    );
    assert.deepEqual(
        cfgmod.normalizeConfig({ agents: { "*": "wat", build: "enforce" } }).agents,
        { "*": "off", build: "enforce" },
        "invalid wildcard value forces off",
    );

    // (3) Defensive resolveModeForAgent fallback (agents["*"] || "off"):
    // reachable only with a hand-built/malformed cfg — normalizeConfig always
    // materializes a valid wildcard — and the guard must still resolve "off".
    assert.equal(cfgmod.resolveModeForAgent({ agents: {} }, "build"), "off",
        "missing wildcard in raw cfg resolves off");
    assert.equal(cfgmod.resolveModeForAgent(null, "build"), "off",
        "null cfg resolves off");
    assert.equal(cfgmod.resolveModeForAgent(undefined, "build"), "off",
        "undefined cfg resolves off");
    assert.equal(cfgmod.resolveModeForAgent({}, "build"), "off",
        "cfg without agents key resolves off");
    assert.equal(cfgmod.resolveModeForAgent({ agents: { build: "enforce" } }, ""),
        "off", "unattributed call never inherits a specific agent's mode");
});

test("[ss2] computeSignature: exact, order-insensitive, difference-sensitive", async () => {
    const policy = await loadPolicy();
    const a = policy.computeSignature("bash", { command: "make test", workdir: "x" });
    const b = policy.computeSignature("bash", { workdir: "x", command: "make test" });
    const c = policy.computeSignature("bash", { command: "make test ", workdir: "x" });
    assert.ok(a.sig && b.sig);
    assert.equal(a.sig, b.sig, "key order does not change the signature");
    assert.notEqual(a.sig, c.sig, "any arg difference changes the signature");
    assert.notEqual(
        policy.computeSignature("bash", { command: "x" }).sig,
        policy.computeSignature("read", { command: "x" }).sig,
        "tool is part of the signature",
    );
});

test("[ss2] computeSignature: oversized/unsupported args are UNENFORCEABLE, never truncated", async () => {
    const policy = await loadPolicy();
    const oversized = policy.computeSignature("bash", { command: "x".repeat(5000) });
    assert.equal(oversized.unsupported, true);
    assert.equal(oversized.reason, "oversized");
    assert.equal(policy.computeSignature("bash", "not-an-object").unsupported, true);
    assert.equal(policy.computeSignature("bash", null).unsupported, true);
    assert.equal(policy.computeSignature("", {}).unsupported, true);
    const weird = { a: { b: undefined } };
    assert.equal(policy.computeSignature("bash", weird).unsupported, true, "non-JSON values unsupported");
    // Two DIFFERENT oversized args must not collide via truncation: both are
    // unsupported => never enforceable => no signature to collide.
    const o1 = policy.computeSignature("bash", { command: "A".repeat(5000) });
    const o2 = policy.computeSignature("bash", { command: "B".repeat(5000) });
    assert.equal(o1.unsupported && o2.unsupported, true);
});

test("[ss2] boundRing caps entries and serialized bytes, dropping oldest first", async () => {
    const policy = await loadPolicy();
    const obs = Array.from({ length: 40 }, (_, i) => ({ seq: i, sig: "s" + i, ts: i }));
    const byEntries = policy.boundRing(obs, 10, 65536);
    assert.equal(byEntries.length, 10);
    assert.equal(byEntries[0].seq, 30, "keeps the NEWEST 10");
    const tiny = policy.boundRing(obs, 40, 60);
    assert.ok(tiny.length < 40, "byte budget drops entries");
    assert.equal(tiny[tiny.length - 1].seq, 39, "newest survives");
});

test("[ss2] hook: audit mode NEVER throws across many identical calls; observations accrue", async () => {
    const mod = await loadPlugin();
    mod.__test.resetState();
    const root = freshRoot();
    process.env.SESSION_PROGRESS_REPO_ROOT = root;
    const cfgmod = await loadConfigModule();
    cfgmod.__resetConfigCacheForTest();
    writeConfig(root, { agents: { "*": "audit" } }); // explicit opt-in (off by default)
    try {
        const hooks = await mod.server({ client: nullClient(), directory: root });
        let clock = 1000;
        mod.__test.setNow(() => clock);
        for (let i = 0; i < 10; i++) {
            clock += 100;
            const r = await driveHook(hooks, {
                args: { command: "make test-js" },
                callID: "c" + i,
            });
            assert.equal(r.threw, null, `call ${i} must allow in audit mode`);
        }
        const st = mod.__test.sessions().get("s1");
        assert.ok(st, "session state exists");
        assert.equal(st.ring.length, 10, "observations recorded");
        assert.equal(st.spend.accrued, 10, "eligible calls accrue");
        assert.ok(st.ring.every((o) => o.outcome === null), "outcomes unknown without history");
    } finally {
        delete process.env.SESSION_PROGRESS_REPO_ROOT;
        mod.__test.setNow(() => Date.now());
        rmSync(root, { recursive: true, force: true });
    }
});

test("[ss2] hook: allow preserves output.args exactly (never mutated)", async () => {
    const mod = await loadPlugin();
    mod.__test.resetState();
    const root = freshRoot();
    process.env.SESSION_PROGRESS_REPO_ROOT = root;
    // Explicit audit fixture (post-269d9c9 opt-in flip): without a config
    // file this test exercised the OFF early-return BEFORE the signature/
    // observe path, making the args-preservation pin vacuous. The audit
    // fixture puts the call on the observing path; the ring assertion below
    // (copied from the [opt-in pin] shape) proves it is not an off-path
    // early return.
    const cfgmod = await loadConfigModule();
    cfgmod.__resetConfigCacheForTest();
    writeConfig(root, { agents: { "*": "audit" } }); // explicit opt-in
    try {
        const hooks = await mod.server({ client: nullClient(), directory: root });
        const args = { command: "echo hi", workdir: "tmp" };
        const snapshot = JSON.parse(JSON.stringify(args));
        const r = await driveHook(hooks, { args });
        assert.equal(r.threw, null);
        assert.equal(r.output.args, args, "same object reference");
        assert.deepEqual(r.output.args, snapshot, "deep-equal content");
        // Proof of path: the call WAS observed (audit path), not the off
        // early-return — the no-observation assertion from [opt-in pin],
        // inverted for the audit fixture.
        const st = mod.__test.sessions().get("s1");
        assert.ok(st, "audit fixture: session state exists");
        assert.equal(st.ring.length, 1,
            "audit fixture: exactly one observation recorded (observing path, not off)");
    } finally {
        delete process.env.SESSION_PROGRESS_REPO_ROOT;
        rmSync(root, { recursive: true, force: true });
    }
});

test("[ss2] hook: malformed hook input fails open (allow, no throw)", async () => {
    const mod = await loadPlugin();
    mod.__test.resetState();
    const root = freshRoot();
    process.env.SESSION_PROGRESS_REPO_ROOT = root;
    try {
        const hooks = await mod.server({ client: nullClient(), directory: root });
        // Missing tool / sessionID / callID / output.
        await assert.doesNotReject(hooks["tool.execute.before"]({ tool: "bash" }, { args: {} }));
        await assert.doesNotReject(hooks["tool.execute.before"]({ sessionID: "s" }, { args: {} }));
        await assert.doesNotReject(hooks["tool.execute.before"]({}, {}));
        // output.args getter that throws -> fail-open allow.
        const badOutput = {
            get args() {
                throw new Error("boom");
            },
        };
        await assert.doesNotReject(
            hooks["tool.execute.before"]({ tool: "bash", sessionID: "s", callID: "c" }, badOutput),
        );
        // Session STATE may exist (attribution needs it) but NOTHING was
        // observed: every ring is empty.
        for (const st of mod.__test.sessions().values()) {
            assert.equal(st.ring.length, 0, "no observation recorded for malformed calls");
        }
    } finally {
        delete process.env.SESSION_PROGRESS_REPO_ROOT;
        rmSync(root, { recursive: true, force: true });
    }
});

test("[ss2] hook: enabled:false no-ops completely; wildcard off is unobserved", async () => {
    const mod = await loadPlugin();
    mod.__test.resetState();
    const root = freshRoot();
    process.env.SESSION_PROGRESS_REPO_ROOT = root;
    try {
        writeConfig(root, { enabled: false });
        const hooks = await mod.server({ client: nullClient(), directory: root });
        await driveHook(hooks, { callID: "c1" });
        assert.equal(mod.__test.sessions().size, 0, "kill switch: no session state");

        writeConfig(root, { enabled: true, agents: { "*": "off" } });
        await driveHook(hooks, { callID: "c2" });
        const stOff = mod.__test.sessions().get("s1");
        assert.ok(!stOff || stOff.ring.length === 0, "off mode: unobserved (empty ring)");
    } finally {
        delete process.env.SESSION_PROGRESS_REPO_ROOT;
        rmSync(root, { recursive: true, force: true });
    }
});

test("[ss2] hook: per-tool observation — read tool recorded but never mechanically relevant", async () => {
    const mod = await loadPlugin();
    mod.__test.resetState();
    const root = freshRoot();
    process.env.SESSION_PROGRESS_REPO_ROOT = root;
    const cfgmod = await loadConfigModule();
    cfgmod.__resetConfigCacheForTest();
    writeConfig(root, { agents: { "*": "audit" } }); // explicit opt-in (off by default)
    try {
        const hooks = await mod.server({ client: nullClient(), directory: root });
        let clock = 5000;
        mod.__test.setNow(() => clock);
        for (let i = 0; i < 12; i++) {
            clock += 10;
            const r = await driveHook(hooks, {
                tool: "read",
                args: { filePath: "/etc/hosts" },
                callID: "r" + i,
            });
            assert.equal(r.threw, null);
        }
        const st = mod.__test.sessions().get("s1");
        assert.equal(st.ring.length, 12);
        assert.ok(st.ring.every((o) => o.tool === "read"));
    } finally {
        delete process.env.SESSION_PROGRESS_REPO_ROOT;
        mod.__test.setNow(() => Date.now());
        rmSync(root, { recursive: true, force: true });
    }
});

// ===========================================================================
// [ss3] Sub-slice 3 — spend gate, polling exemption, mechanical fast path
// ===========================================================================

const CASES = JSON.parse(
    fs.readFileSync(join(REPO, "tests", "scripts", "fixtures", "session-progress", "cases.json"), "utf8"),
);

test("[ss3] polling exemption: exact grammar table (fixtures)", async () => {
    const policy = await loadPolicy();
    const cfg = policy; // not needed by isPollingExemptCommand beyond polling_exemptions
    const norm = (await loadConfigModule()).normalizeConfig({});
    for (const c of CASES.polling) {
        assert.equal(
            policy.isPollingExemptCommand(c.command, norm),
            c.expect,
            `exemption mismatch for: ${c.command}`,
        );
    }
});

test("[ss3] spend gate: interval AND accrual both required; first assessment waits", async () => {
    const policy = await loadPolicy();
    const cfgmod = await loadConfigModule();
    const cfg = cfgmod.normalizeConfig({ cadence: { min_interval_seconds: 60, min_new_signatures: 8 } });
    const st = { createdAt: 0, spend: { lastAttempt: null, accrued: 0 } };
    assert.equal(policy.spendGateOpen(st, cfg, 59_000 + 8), false, "interval not met");
    assert.equal(policy.spendGateOpen(st, cfg, 60_000), false, "accrual not met");
    st.spend.accrued = 8;
    assert.equal(policy.spendGateOpen(st, cfg, 60_000), true, "both met");
    st.spend.lastAttempt = 60_000;
    st.spend.accrued = 8;
    assert.equal(policy.spendGateOpen(st, cfg, 119_999), false, "interval since last attempt");
    assert.equal(policy.spendGateOpen(st, cfg, 120_000), true);
});

test("[ss3] leaseDecision: none / deny / expired / capped", async () => {
    const policy = await loadPolicy();
    assert.equal(policy.leaseDecision(null, 0).action, "none");
    const lease = { expiresAt: 1000, hits: 0, maxHits: 2 };
    assert.equal(policy.leaseDecision(lease, 999).action, "deny");
    assert.equal(policy.leaseDecision(lease, 1000).action, "expired");
    const capped = { expiresAt: 1000, hits: 2, maxHits: 2 };
    assert.equal(policy.leaseDecision(capped, 999).action, "capped");
});

test("[ss3] buildDenyReason: bounded, marked, scoped", async () => {
    const policy = await loadPolicy();
    const reason = policy.buildDenyReason({
        tool: "bash",
        sigId: "abcd1234",
        rule: "mechanical",
        evidenceIds: [12, 15, 18, 21, 24, 27, 30],
        expiresAt: Date.UTC(2026, 9, 3, 12, 0, 5),
        hitsLeft: 1,
    });
    assert.ok(reason.startsWith("[session-progress]"), "own-denial marker");
    assert.ok(reason.includes("rule mechanical"));
    assert.ok(reason.includes("signature abcd1234"));
    assert.ok(reason.includes("12, 15, 18, 21, 24, 27"), "evidence capped at 6 ids");
    assert.ok(reason.includes("other calls remain allowed"), "narrow scope stated");
    assert.ok(reason.length <= 600, "bounded");
});

// runMechScenario — drive the REAL hook through a fixture scenario. The stub
// client serves the history recorded so far (a part appears only AFTER its
// call completes — the current call's outcome is always unknown, matching
// the real runtime).
async function runMechScenario(mod, scenario, modeOverride) {
    mod.__test.resetState();
    const root = freshRoot();
    process.env.SESSION_PROGRESS_REPO_ROOT = root;
    const cfgmod = await loadConfigModule();
    cfgmod.__resetConfigCacheForTest();
    const cfg = { ...scenario.config };
    if (modeOverride) cfg.agents = { "*": modeOverride };
    if (scenario.lease_seconds) cfg.mechanical = { lease_seconds: scenario.lease_seconds };
    writeConfig(root, cfg);
    const parts = [];
    const client = {
        session: {
            messages: async () => ({
                error: undefined,
                data: [{ id: "m1", role: "assistant", agent: "build", parts: parts.slice() }],
            }),
        },
    };
    const hooks = await mod.server({ client, directory: root });
    let clock = 0;
    mod.__test.setNow(() => clock);
    const results = [];
    const tool = scenario.tool || "bash";
    try {
        for (let i = 0; i < scenario.calls.length; i++) {
            const c = scenario.calls[i];
            clock += scenario.step_ms;
            const command =
                c.command ||
                (c.polling
                    ? "vh-agent-harness exec python .opencode/skills/bgshell-job/scripts/bgshell_job.py status --job build --lines 40"
                    : scenario.command);
            const args = c.args || { command };
            const r = await driveHook(hooks, { tool, args, callID: c.callID });
            results.push({ i: i + 1, callID: c.callID, threw: r.threw, polling: !!c.polling });
            // After the call completes, record its outcome in history.
            if (c.polling) {
                parts.push(mkToolPart(c.callID, tool, failExitState(0, "job running")));
            } else if (c.history_error !== undefined) {
                const errText = typeof c.history_error === "string" ? c.history_error : "exit status 1";
                parts.push(mkToolPart(c.callID, tool, errState(errText)));
            } else if (c.history_success) {
                parts.push(mkToolPart(c.callID, tool, failExitState(0, "ok output")));
            }
        }
    } finally {
        delete process.env.SESSION_PROGRESS_REPO_ROOT;
        mod.__test.setNow(() => Date.now());
        rmSync(root, { recursive: true, force: true });
    }
    return results;
}

test("[ss3] mechanical scenarios table (fixtures): enforce denies exactly at threshold", async () => {
    const mod = await loadPlugin();
    for (const sc of CASES.mechanical.scenarios) {
        const results = await runMechScenario(mod, sc, "enforce");
        const denied = results.filter((r) => r.threw);
        if (sc.first_deny_at === null) {
            assert.equal(denied.length, 0, `${sc.name}: no call may deny`);
        } else {
            assert.equal(denied.length, 1, `${sc.name}: exactly one deny`);
            const d = denied[0];
            assert.equal(d.i, sc.first_deny_at, `${sc.name}: deny at call ${sc.first_deny_at}`);
            assert.ok(
                d.threw.message.startsWith("[session-progress]"),
                `${sc.name}: bounded marked reason`,
            );
            assert.ok(d.threw.message.includes("rule mechanical"), `${sc.name}: rule named`);
            assert.ok(d.threw.message.length <= 600, `${sc.name}: reason bounded`);
            // The lease window caps denials: later identical calls ALLOW.
            for (const idx of sc.later_identical_allowed || []) {
                const later = results.find((r) => r.i === idx);
                assert.ok(later, `${sc.name}: call ${idx} exists`);
                assert.equal(later.threw, null, `${sc.name}: call ${idx} allowed (hit cap)`);
            }
        }
        // Polling calls never deny.
        for (const r of results.filter((x) => x.polling)) {
            assert.equal(r.threw, null, `${sc.name}: polling call ${r.callID} never denies`);
        }
    }
});

test("[ss3] identical-failing-bash in AUDIT mode: would-deny recorded, NEVER throws", async () => {
    const mod = await loadPlugin();
    const sc = CASES.mechanical.scenarios[0];
    const results = await runMechScenario(mod, sc, "audit");
    assert.ok(results.every((r) => r.threw === null), "audit never throws");
    const stats = mod.__test.diagStats();
    assert.ok(stats.wouldDeny >= 1, `would-deny recorded (got ${JSON.stringify(stats)})`);
    assert.equal(stats.denied, 0, "no actual denies in audit");
});

test("[ss3] after a mechanical deny, a DIFFERENT command is allowed immediately", async () => {
    const mod = await loadPlugin();
    mod.__test.resetState();
    const root = freshRoot();
    process.env.SESSION_PROGRESS_REPO_ROOT = root;
    const cfgmod = await loadConfigModule();
    cfgmod.__resetConfigCacheForTest();
    writeConfig(root, {
        agents: { "*": "enforce" },
        cadence: { min_interval_seconds: 1, min_new_signatures: 1 },
    });
    const parts = [];
    const client = {
        session: { messages: async () => ({ data: [{ role: "assistant", agent: "build", parts: parts.slice() }] }) },
    };
    const hooks = await mod.server({ client, directory: root });
    let clock = 0;
    mod.__test.setNow(() => clock);
    try {
        let denied = false;
        for (let i = 1; i <= 10 && !denied; i++) {
            clock += 600;
            const r = await driveHook(hooks, { args: { command: "make loop" }, callID: "c" + i });
            parts.push(mkToolPart("c" + i, "bash", errState("exit status 1")));
            if (r.threw) denied = true;
        }
        assert.ok(denied, "looping call eventually denied");
        // A different command right after the deny: allowed (narrow scope).
        clock += 600;
        const other = await driveHook(hooks, { args: { command: "echo alternative" }, callID: "alt1" });
        assert.equal(other.threw, null, "different call NOT blocked by the lease");
        // ...and the polling grammar is exempt even in enforce mode.
        clock += 600;
        const poll = await driveHook(hooks, {
            args: {
                command:
                    "vh-agent-harness exec python .opencode/skills/bgshell-job/scripts/bgshell_job.py status --job build --lines 40",
            },
            callID: "poll1",
        });
        assert.equal(poll.threw, null, "polling exempt in enforce mode");
    } finally {
        delete process.env.SESSION_PROGRESS_REPO_ROOT;
        mod.__test.setNow(() => Date.now());
        rmSync(root, { recursive: true, force: true });
    }
});

test("[ss3] hook: spend gate closed -> no assessment; skip recorded", async () => {
    const mod = await loadPlugin();
    mod.__test.resetState();
    const root = freshRoot();
    process.env.SESSION_PROGRESS_REPO_ROOT = root;
    const cfgmod = await loadConfigModule();
    cfgmod.__resetConfigCacheForTest();
    // Default cadence (60s / 8 obs) — no assessment can open in this test.
    writeConfig(root, { agents: { "*": "enforce" } });
    let historyReads = 0;
    const client = {
        session: {
            messages: async () => {
                historyReads += 1;
                return { data: [] };
            },
        },
    };
    const hooks = await mod.server({ client, directory: root });
    let clock = 0;
    mod.__test.setNow(() => clock);
    try {
        for (let i = 1; i <= 6; i++) {
            clock += 500;
            await driveHook(hooks, { args: { command: "make x" }, callID: "c" + i });
        }
        assert.equal(historyReads, 0, "spend gate closed: no history reads, no judge spend");
        const st = mod.__test.sessions().get("s1");
        assert.equal(st.spend.lastAttempt, null, "cadence not consumed");
    } finally {
        delete process.env.SESSION_PROGRESS_REPO_ROOT;
        mod.__test.setNow(() => Date.now());
        rmSync(root, { recursive: true, force: true });
    }
});

test("[ss3] hook: unknown attribution with specific agent keys -> audit ceiling (no deny)", async () => {
    const mod = await loadPlugin();
    mod.__test.resetState();
    const root = freshRoot();
    process.env.SESSION_PROGRESS_REPO_ROOT = root;
    const cfgmod = await loadConfigModule();
    cfgmod.__resetConfigCacheForTest();
    // Specific keys present; NO history (attribution unknown): even though the
    // wildcard says enforce, unattributed calls can never deny.
    writeConfig(root, {
        agents: { "*": "enforce", "plan": "off" },
        cadence: { min_interval_seconds: 1, min_new_signatures: 1 },
    });
    const parts = [];
    const client = {
        session: {
            messages: async () => ({
                // No assistant agent field anywhere: attribution unavailable.
                data: [{ role: "assistant", parts: parts.slice() }],
            }),
        },
    };
    const hooks = await mod.server({ client, directory: root });
    let clock = 0;
    mod.__test.setNow(() => clock);
    try {
        let denied = false;
        for (let i = 1; i <= 8; i++) {
            clock += 600;
            const r = await driveHook(hooks, { args: { command: "make unattr" }, callID: "c" + i });
            parts.push(mkToolPart("c" + i, "bash", errState("exit status 1")));
            if (r.threw) denied = true;
        }
        assert.equal(denied, false, "unknown attribution: mechanical can never deny");
        const stats = mod.__test.diagStats();
        assert.ok(stats.wouldDeny >= 1, "would-deny still recorded for observability");
    } finally {
        delete process.env.SESSION_PROGRESS_REPO_ROOT;
        mod.__test.setNow(() => Date.now());
        rmSync(root, { recursive: true, force: true });
    }
});

// [ss4-pin] History request shape — live-runtime regression pin (2026-10-03).
// The opencode SDK renamed the messages path param `id` -> `sessionID`
// between 1.18.5 (refs) and 1.18.34 (installed). readHistory must send BOTH
// eras' keys (each era's serializer picks its declared key and ignores the
// other); dropping one degrades to /session/undefined/message -> 404 ->
// silent fail-open null (no outcomes, no attribution, mechanical never
// fires live). This pin fails the suite if a future edit regresses the
// request shape.
test("[ss4] history request carries BOTH SDK path-key eras (id + sessionID) with the live session id", async () => {
    const mod = await loadPlugin();
    mod.__test.resetState();
    const root = freshRoot();
    process.env.SESSION_PROGRESS_REPO_ROOT = root;
    const cfgmod = await loadConfigModule();
    cfgmod.__resetConfigCacheForTest();
    writeConfig(root, { agents: { "*": "audit" }, cadence: { min_interval_seconds: 1, min_new_signatures: 1 } });
    const parts = [];
    const client = histClient([{ role: "assistant", agent: "build", parts }]);
    const hooks = await mod.server({ client, directory: root });
    let clock = 0;
    mod.__test.setNow(() => clock);
    try {
        for (let i = 1; i <= 3; i++) {
            clock += 600;
            await driveHook(hooks, { args: { command: "make pin" }, callID: "c" + i });
            parts.push(mkToolPart("c" + i, "bash", errState("exit status 1")));
        }
        assert.ok(client.__requests.length >= 1, "at least one history read happened (gate opened)");
        for (const req of client.__requests) {
            assert.equal(req.path?.id, "s1", "path.id carries the live session id (opencode <=1.18.5 SDK era)");
            assert.equal(req.path?.sessionID, "s1", "path.sessionID carries the live session id (opencode >=1.18.34 SDK era)");
            assert.equal(req.query?.limit, 32, "bounded history window");
        }
    } finally {
        delete process.env.SESSION_PROGRESS_REPO_ROOT;
        mod.__test.setNow(() => Date.now());
        rmSync(root, { recursive: true, force: true });
    }
});

// [F1-pin] The history race must never overrun the shared slow-path deadline
// (re-review F1): the old Math.max(25, remaining()) floor let the race wait
// its 25 ms floor even when <=25 ms of budget remained, returning the hook
// AFTER the documented deadline (same invariant family as the pinned
// <=20000 ms judge ceiling). Two pins:
//   (a) near-expiry: ~1 ms of budget + a hanging history client -> the hook
//       allows at/near the deadline (no 25 ms floor wait).
//   (b) healthy budget: the history race still runs and enrichment still
//       happens (attribution observed from history).
// Driving notes: the spend gate needs >= min_interval since session creation,
// so one warmup call (gate closed, no history read) precedes each decisive
// call. The near-expiry clock is scripted POSITIONALLY for the decisive call:
// the segment from the deadlineAt computation to the budget probe is
// synchronous, so that call's _now() probes are, in order: t0, now,
// deadlineAt base, lastAttempt, budget probe.
test("[F1-pin] hook: near-expired deadline skips the history 25ms floor; healthy budget still enriches", async () => {
    const mod = await loadPlugin();
    const cfgmod = await loadConfigModule();

    // (a) near-expiry: warm up at t=10_000, then the decisive call's script
    // sets deadlineAt = 11_000 + 400 = 11_400 with the budget probe (5th
    // clock probe of that call) at 11_399 -> remaining = 1 ms. The pad keeps
    // every later read (enrichNow, diagnostics drain) at the exhausted
    // deadline so the post-race path records budget exhaustion, not a jump.
    mod.__test.resetState();
    const root = freshRoot();
    process.env.SESSION_PROGRESS_REPO_ROOT = root;
    cfgmod.__resetConfigCacheForTest();
    writeConfig(root, {
        agents: { "*": "enforce" },
        cadence: { min_interval_seconds: 1, min_new_signatures: 1 },
        judge: { timeout_ms: 400 },
    });
    let clockMode = "warmup";
    let scriptIdx = 0;
    const script = [11_000, 11_000, 11_000, 11_000, 11_399, 11_400];
    mod.__test.setNow(() => {
        if (clockMode === "warmup") return 10_000;
        return script[Math.min(scriptIdx++, script.length - 1)];
    });
    let hangCalls = 0;
    const hanging = {
        session: {
            messages: () => {
                hangCalls += 1;
                return new Promise(() => {}); // never settles: only the cap ends the race
            },
        },
    };
    const hooks = await mod.server({ client: hanging, directory: root });
    clearJudgeEnv(); // judge unavailable: the post-race path must still allow
    try {
        await driveHook(hooks, { args: { command: "make expire" }, callID: "warm1" });
        assert.equal(hangCalls, 0, "warmup (gate closed) must not read history");
        await new Promise((res) => setTimeout(res, 20)); // let the warmup diag drain settle

        clockMode = "scripted";
        const t0wall = Date.now();
        const r = await driveHook(hooks, { args: { command: "make expire" }, callID: "c1" });
        const elapsed = Date.now() - t0wall;
        assert.equal(r.threw, null, "near-expiry budget fails open (allow)");
        assert.ok(elapsed < 20, `no 25ms floor wait at near-expiry (elapsed ${elapsed}ms)`);
        assert.equal(hangCalls, 1, "the 1ms-capped race consulted history exactly once");
        await new Promise((res) => setTimeout(res, 20)); // let the diag drain settle
    } finally {
        delete process.env.SESSION_PROGRESS_REPO_ROOT;
        mod.__test.setNow(() => Date.now());
        rmSync(root, { recursive: true, force: true });
    }

    // (b) healthy budget: warm up at t=21_000, decisive call at t=22_000 ->
    // budget = the full 400 ms; a responsive history client is read and
    // attribution is enriched.
    mod.__test.resetState();
    const root2 = freshRoot();
    process.env.SESSION_PROGRESS_REPO_ROOT = root2;
    cfgmod.__resetConfigCacheForTest();
    writeConfig(root2, {
        agents: { "*": "enforce" },
        cadence: { min_interval_seconds: 1, min_new_signatures: 1 },
        judge: { timeout_ms: 400 },
    });
    const client = histClient([{ role: "assistant", agent: "build", parts: [] }]);
    const hooks2 = await mod.server({ client, directory: root2 });
    let clock2 = 21_000;
    mod.__test.setNow(() => clock2);
    try {
        await driveHook(hooks2, { args: { command: "make healthy" }, callID: "warm1" });
        assert.equal(client.__requests.length, 0, "warmup (gate closed) must not read history");
        await new Promise((res) => setTimeout(res, 20)); // let the warmup diag drain settle

        clock2 = 22_000; // 1s after creation: the spend gate opens
        const r = await driveHook(hooks2, { args: { command: "make healthy" }, callID: "h1" });
        assert.equal(r.threw, null, "healthy budget allows (judge not configured)");
        assert.ok(client.__requests.length >= 1, "healthy budget: history read still happens");
        const st = mod.__test.sessions().get("s1");
        assert.equal(st.agentName, "build", "healthy budget: attribution enriched from history");
        await new Promise((res) => setTimeout(res, 20)); // let the diag drain settle
    } finally {
        delete process.env.SESSION_PROGRESS_REPO_ROOT;
        mod.__test.setNow(() => Date.now());
        rmSync(root2, { recursive: true, force: true });
    }
});

// ===========================================================================
// [ss4] Sub-slice 4 — semantic judge, policy engine, deadlines, diagnostics
// ===========================================================================

import http from "node:http";

// startJudgeServer — a local OpenAI-compatible judge endpoint with scripted
// behavior. Returns {server, url, requests} where requests captures parsed
// POST bodies.
function startJudgeServer(behavior, opts = {}) {
    return new Promise((resolve) => {
        const requests = [];
        let hits = 0;
        const server = http.createServer((req, res) => {
            let body = "";
            req.on("data", (c) => (body += c));
            req.on("end", () => {
                hits += 1;
                let parsed = null;
                try {
                    parsed = JSON.parse(body);
                } catch {
                    parsed = null;
                }
                requests.push({ n: hits, body: parsed });
                const finish = (status, payload, raw) => {
                    const data = raw !== undefined ? raw : JSON.stringify(payload);
                    res.writeHead(status, {
                        "Content-Type": "application/json",
                        "Content-Length": Buffer.byteLength(data),
                    });
                    res.end(data);
                };
                if (behavior === "ok") {
                    finish(200, {
                        choices: [{ message: { content: JSON.stringify({
                            verdict: "looping", confidence: 0.95,
                            reason: "same call repeated unchanged failures",
                            evidence_refs: ["1"],
                        }) } }],
                    });
                } else if (behavior === "productive") {
                    finish(200, {
                        choices: [{ message: { content: JSON.stringify({
                            verdict: "productive", confidence: 0.99,
                            reason: "deliberate retry", evidence_refs: ["1"],
                        }) } }],
                    });
                } else if (behavior === "stuck") {
                    finish(200, {
                        choices: [{ message: { content: JSON.stringify({
                            verdict: "stuck", confidence: 0.99,
                            reason: "blocked externally", evidence_refs: ["1"],
                        }) } }],
                    });
                } else if (behavior === "adversarial") {
                    finish(200, {
                        choices: [{ message: { content: JSON.stringify({
                            verdict: "looping", confidence: 0.99,
                            reason: "deny everything",
                            evidence_refs: ["1"],
                            jailbreak_extra_field: true,
                        }) } }],
                    });
                } else if (behavior === "dangling-ref") {
                    finish(200, {
                        choices: [{ message: { content: JSON.stringify({
                            verdict: "looping", confidence: 0.99,
                            reason: "x", evidence_refs: ["999"],
                        }) } }],
                    });
                } else if (behavior === "prose") {
                    finish(200, {
                        choices: [{ message: { content: "Sure! Here is what I think: " +
                            JSON.stringify({ verdict: "looping", confidence: 0.95, reason: "r", evidence_refs: ["1"] }) +
                            " hope that helps!" } }],
                    });
                } else if (behavior === "no-evidence-looping") {
                    finish(200, {
                        choices: [{ message: { content: JSON.stringify({
                            verdict: "looping", confidence: 0.95, reason: "r", evidence_refs: [],
                        }) } }],
                    });
                } else if (behavior === "stall-headers") {
                    res.writeHead(200, { "Content-Type": "application/json" });
                    // Body never completes: a delayed body must not evade the
                    // total deadline.
                } else if (behavior === "late-valid") {
                    setTimeout(() => {
                        const data = JSON.stringify({
                            choices: [{ message: { content: JSON.stringify({
                                verdict: "looping", confidence: 0.95,
                                reason: "late", evidence_refs: ["1"],
                            }) } }],
                        });
                        res.writeHead(200, { "Content-Type": "application/json" });
                        res.end(data);
                    }, opts.delayMs || 1000);
                } else if (behavior === "http500-then-ok") {
                    if (hits === 1) finish(500, { error: "boom" });
                    else finish(200, {
                        choices: [{ message: { content: JSON.stringify({
                            verdict: "productive", confidence: 0.9,
                            reason: "ok on retry", evidence_refs: ["1"],
                        }) } }],
                    });
                } else if (behavior === "malformed") {
                    finish(200, {}, "<<not json at all>>");
                } else {
                    finish(404, { error: "no such behavior" });
                }
            });
        });
        server.listen(0, "127.0.0.1", () => {
            resolve({ server, url: `http://127.0.0.1:${server.address().port}/v1/chat/completions`, requests });
        });
    });
}

const JUDGE_ENV = {
    SESSION_PROGRESS_JUDGE_MODEL: "mock-judge",
    SESSION_PROGRESS_JUDGE_API_KEY: "test-key",
};

function withJudgeEnv(url) {
    if (url) process.env.SESSION_PROGRESS_JUDGE_ENDPOINT = url;
    else delete process.env.SESSION_PROGRESS_JUDGE_ENDPOINT;
    Object.assign(process.env, JUDGE_ENV);
}

function clearJudgeEnv() {
    delete process.env.SESSION_PROGRESS_JUDGE_ENDPOINT;
    delete process.env.SESSION_PROGRESS_JUDGE_MODEL;
    delete process.env.SESSION_PROGRESS_JUDGE_API_KEY;
}

test("[ss4] runJudge: unavailable when env not configured", async () => {
    const judge = await loadJudge();
    const cfgmod = await loadConfigModule();
    const cfg = cfgmod.normalizeConfig({});
    const r = await judge.runJudge(cfg, { observations: [] }, { env: {} });
    assert.equal(r.status, "unavailable");
});

test("[ss4] runJudge: valid verdict parsed and schema-normalized", async () => {
    const judge = await loadJudge();
    const cfgmod = await loadConfigModule();
    const cfg = cfgmod.normalizeConfig({ judge: { timeout_ms: 1000 } });
    const { server, url } = await startJudgeServer("ok");
    try {
        const packet = { observations: [{ id: "1" }] };
        const r = await judge.runJudge(cfg, packet, { env: { ...JUDGE_ENV, SESSION_PROGRESS_JUDGE_ENDPOINT: url } });
        assert.equal(r.status, "verdict");
        assert.equal(r.verdict.verdict, "looping");
        assert.equal(r.verdict.confidence, 0.95);
        assert.deepEqual(r.verdict.evidence_ids, ["1"]);
    } finally {
        server.close();
    }
});

test("[ss4] runJudge: prose-wrapped JSON accepted; adversarial/dangling/no-evidence outputs REJECTED (fail-open)", async () => {
    const judge = await loadJudge();
    const cfgmod = await loadConfigModule();
    const cfg = cfgmod.normalizeConfig({ judge: { timeout_ms: 1000 } });
    const packet = { observations: [{ id: "1" }] };
    for (const behavior of ["prose", "adversarial", "dangling-ref", "no-evidence-looping", "malformed"]) {
        const { server, url } = await startJudgeServer(behavior);
        try {
            const r = await judge.runJudge(cfg, packet, {
                env: { ...JUDGE_ENV, SESSION_PROGRESS_JUDGE_ENDPOINT: url },
            });
            if (behavior === "prose") {
                assert.equal(r.status, "verdict", "prose-wrapped STRICT json is accepted");
            } else {
                assert.notEqual(r.status, "verdict", `${behavior} must fail open`);
                assert.equal(r.status, "error");
                assert.equal(r.class, behavior === "malformed" ? "malformed" : "invalid-verdict", `${behavior} class`);
            }
        } finally {
            server.close();
        }
    }
});

test("[ss4] runJudge: a stalled body CANNOT evade the total deadline (timeout -> fail-open, bounded wall time)", async () => {
    const judge = await loadJudge();
    const cfgmod = await loadConfigModule();
    const cfg = cfgmod.normalizeConfig({ judge: { timeout_ms: 250 } });
    const { server, url } = await startJudgeServer("stall-headers");
    try {
        const t0 = Date.now();
        const r = await judge.runJudge(cfg, { observations: [{ id: "1" }] }, {
            env: { ...JUDGE_ENV, SESSION_PROGRESS_JUDGE_ENDPOINT: url },
        });
        const elapsed = Date.now() - t0;
        assert.equal(r.status, "timeout");
        assert.ok(elapsed < 1500, `deadline bounded (elapsed ${elapsed}ms)`);
    } finally {
        server.close();
    }
});

test("[ss4] runJudge: LATE valid verdict is stale and discarded (late answers cannot deny)", async () => {
    const judge = await loadJudge();
    const cfgmod = await loadConfigModule();
    const cfg = cfgmod.normalizeConfig({ judge: { timeout_ms: 250 } });
    const { server, url } = await startJudgeServer("late-valid", { delayMs: 800 });
    try {
        const t0 = Date.now();
        const r = await judge.runJudge(cfg, { observations: [{ id: "1" }] }, {
            env: { ...JUDGE_ENV, SESSION_PROGRESS_JUDGE_ENDPOINT: url },
        });
        const elapsed = Date.now() - t0;
        assert.ok(elapsed < 600, `not stalled by late answer (${elapsed}ms)`);
        assert.notEqual(r.status, "verdict", "a verdict after the deadline is discarded");
    } finally {
        server.close();
    }
});

test("[ss4] runJudge: one retry INSIDE the same deadline recovers a transient 5xx", async () => {
    const judge = await loadJudge();
    const cfgmod = await loadConfigModule();
    const cfg = cfgmod.normalizeConfig({ judge: { timeout_ms: 1000, retries: 1 } });
    const { server, url, requests } = await startJudgeServer("http500-then-ok");
    try {
        const r = await judge.runJudge(cfg, { observations: [{ id: "1" }] }, {
            env: { ...JUDGE_ENV, SESSION_PROGRESS_JUDGE_ENDPOINT: url },
        });
        assert.equal(r.status, "verdict");
        assert.equal(r.verdict.verdict, "productive");
        assert.equal(requests.length, 2, "exactly one retry");
    } finally {
        server.close();
    }
});

test("[ss4] validateVerdict: strict schema matrix", async () => {
    const policy = await loadPolicy();
    const packet = { observations: [{ id: "1" }, { id: "2" }] };
    const ok = (v) => policy.validateVerdict(v, packet);
    assert.ok(ok({ verdict: "productive", confidence: 0.1, reason: "", evidence_refs: [] }));
    assert.ok(ok({ verdict: "looping", confidence: 0.9, reason: "r", evidence_refs: ["2", "1"] }));
    assert.equal(ok({ verdict: "LOOPING", confidence: 0.9, evidence_refs: ["1"] }), null, "enum case");
    assert.equal(ok({ verdict: "bogus", confidence: 0.9, evidence_refs: ["1"] }), null, "enum value");
    assert.equal(ok({ verdict: "looping", confidence: 1.2, evidence_refs: ["1"] }), null, "confidence range");
    assert.equal(ok({ verdict: "looping", confidence: "high", evidence_refs: ["1"] }), null, "confidence type");
    assert.equal(ok({ verdict: "looping", confidence: 0.9, evidence_refs: ["1"], extra: 1 }), null, "extra field");
    assert.equal(ok({ verdict: "looping", confidence: 0.9, evidence_refs: ["nope"] }), null, "dangling ref");
    assert.equal(ok({ verdict: "looping", confidence: 0.9, evidence_refs: [] }), null, "looping needs evidence");
    assert.equal(ok({ verdict: "looping", confidence: 0.9 }), null, "missing evidence_refs");
    assert.equal(ok(null), null);
    assert.equal(ok("looping"), null);
});

test("[ss4] evaluateSemantic: only looping+confidence+corroboration fires", async () => {
    const policy = await loadPolicy();
    const cfgmod = await loadConfigModule();
    const cfg = cfgmod.normalizeConfig({});
    const now = 10_000;
    const mkRing = (n) => Array.from({ length: n }, (_, i) => ({
        seq: i + 1, sig: "SIG", ts: now - 1000 * (n - i), ownDenial: false, exempt: false,
        outcome: { callID: "c" + i, cls: "error", digest: "d" },
    }));
    const vd = (verdict, confidence) => ({ verdict, confidence, reason: "r", evidence_ids: ["1"] });
    assert.equal(policy.evaluateSemantic(vd("looping", 0.95), { ring: mkRing(3), sig: "SIG", cfg, now }).fire, true);
    assert.equal(policy.evaluateSemantic(vd("looping", 0.95), { ring: mkRing(2), sig: "SIG", cfg, now }).fire, false, "prior_matches=3 required");
    assert.equal(policy.evaluateSemantic(vd("looping", 0.89), { ring: mkRing(5), sig: "SIG", cfg, now }).fire, false, "confidence floor");
    assert.equal(policy.evaluateSemantic(vd("productive", 0.99), { ring: mkRing(5), sig: "SIG", cfg, now }).fire, false);
    assert.equal(policy.evaluateSemantic(vd("stuck", 0.99), { ring: mkRing(5), sig: "SIG", cfg, now }).fire, false);
    assert.equal(policy.evaluateSemantic(vd("drifting", 0.99), { ring: mkRing(5), sig: "SIG", cfg, now }).fire, false);
    assert.equal(policy.evaluateSemantic(null, { ring: mkRing(5), sig: "SIG", cfg, now }).fire, false);
    const ownRing = mkRing(3).map((o) => ({ ...o, ownDenial: true }));
    assert.equal(policy.evaluateSemantic(vd("looping", 0.95), { ring: ownRing, sig: "SIG", cfg, now }).fire, false, "own denials are not evidence");
    const oldRing = mkRing(3).map((o) => ({ ...o, ts: now - 91_000 }));
    assert.equal(policy.evaluateSemantic(vd("looping", 0.95), { ring: oldRing, sig: "SIG", cfg, now }).fire, false, "outside semantic window");
    const changedRing = mkRing(3).map((o, i) => ({ ...o, outcome: { cls: "error", digest: "d" + i } }));
    assert.equal(policy.evaluateSemantic(vd("looping", 0.95), { ring: changedRing, sig: "SIG", cfg, now }).fire, false, "changed outcomes");
});

test("[ss4] classifyHistoryPart + scrubbing: adapter outcomes and secret masking", async () => {
    const policy = await loadPolicy();
    const own = new Set(["cX"]);
    assert.equal(policy.classifyHistoryPart(mkToolPart("cX", "bash", errState("denied")), own).cls, "own-denial");
    assert.equal(policy.classifyHistoryPart(mkToolPart("c1", "bash", errState("boom")), own).cls, "error");
    assert.equal(policy.classifyHistoryPart(mkToolPart("c2", "bash", failExitState(1)), own).cls, "exit-fail");
    assert.equal(policy.classifyHistoryPart(mkToolPart("c3", "bash", failExitState(0)), own).cls, "ok");
    assert.equal(policy.classifyHistoryPart(mkToolPart("c4", "read", { status: "completed", metadata: {} }), own).cls, "unknown", "unadapted tool");
    assert.equal(policy.classifyHistoryPart(mkToolPart("c5", "bash", { status: "running" }), own).cls, "unknown");
    assert.equal(policy.classifyHistoryPart(null, own), null);
    assert.equal(
        policy.classifyHistoryPart(mkToolPart("a", "bash", failExitState(1, "out")), own).digest,
        policy.classifyHistoryPart(mkToolPart("b", "bash", failExitState(1, "out")), own).digest,
    );
    assert.notEqual(
        policy.classifyHistoryPart(mkToolPart("a", "bash", failExitState(1, "out1")), own).digest,
        policy.classifyHistoryPart(mkToolPart("b", "bash", failExitState(1, "out2")), own).digest,
    );
    const scrubbed = policy.scrubText("curl -H API_TOKEN=sk-abcdefghijklmnop123456 x AWS_SECRET_KEY=zzz y", 200);
    assert.ok(!scrubbed.includes("sk-abcdefghijklmnop123456"), "token value masked");
    assert.ok(!scrubbed.includes("=zzz"), "secret value masked");
    assert.ok(scrubbed.includes("API_TOKEN=***"));
    // Long uniform runs are treated as token-ish and masked (shorter than cap).
    const runMasked = policy.scrubText("a".repeat(3000), 100);
    assert.ok(runMasked.length < 100 && runMasked.includes("[masked]"), "long run masked");
    // Ordinary long prose is capped, not masked.
    const prose = policy.scrubText(("word ").repeat(1000), 100);
    assert.equal(prose.length, 100, "capped");
});

test("[ss4] buildJudgePacket: bounded, no raw args, own-denials labeled, excerpt scrubbed", async () => {
    const policy = await loadPolicy();
    const ring = [
        { seq: 1, callID: "c1", tool: "bash", sig: "SIG", ts: 1000, exempt: false, unsupported: false, ownDenial: false, outcome: { cls: "error" } },
        { seq: 2, callID: "c2", tool: "bash", sig: "SIG", ts: 2000, exempt: false, unsupported: false, ownDenial: true, outcome: null },
        { seq: 3, callID: "c3", tool: "bash", sig: "OTHER", ts: 3000, exempt: true, unsupported: false, ownDenial: false, outcome: null },
    ];
    const packet = policy.buildJudgePacket({ ring, tool: "bash", sig: "SIG", now: 4000, agentName: "build", assessmentId: "a1" });
    const s = JSON.stringify(packet);
    assert.ok(!s.includes("command"), "no raw args in packet");
    assert.equal(packet.observations.length, 3);
    assert.equal(packet.observations[0].outcome, "error");
    assert.equal(packet.observations[1].outcome, "denied-by-detector");
    assert.equal(packet.observations[2].exempt_poll, true);
    assert.equal(packet.observations[0].current, true);
    const withExcerpt = policy.attachAssistantExcerpt(packet, "I will retry with API_KEY=sk-secretvalue123 now");
    assert.ok(!withExcerpt.last_assistant_excerpt.includes("sk-secretvalue123"), "excerpt scrubbed");
});

// semDrive — hook-level semantic scenario driver: identical failing bash calls
// with a scripted judge + history client; mechanical DISABLED so only the
// semantic path can deny.
async function semDrive(mod, behavior, opts = {}) {
    mod.__test.resetState();
    const root = freshRoot();
    process.env.SESSION_PROGRESS_REPO_ROOT = root;
    const cfgmod = await loadConfigModule();
    cfgmod.__resetConfigCacheForTest();
    writeConfig(root, {
        agents: { "*": "enforce" },
        cadence: { min_interval_seconds: 1, min_new_signatures: 1 },
        mechanical: { enabled: false },
        judge: { timeout_ms: opts.timeout_ms || 800 },
    });
    const { server, url, requests } = await startJudgeServer(behavior, opts);
    withJudgeEnv(url);
    const parts = [];
    const client = {
        session: { messages: async () => ({ data: [{ role: "assistant", agent: "build", parts: parts.slice() }] }) },
    };
    const hooks = await mod.server({ client, directory: root });
    let clock = 0;
    mod.__test.setNow(() => clock);
    const results = [];
    try {
        for (let i = 1; i <= (opts.calls || 6); i++) {
            clock += 600;
            const r = await driveHook(hooks, { args: { command: "make boom" }, callID: "c" + i });
            results.push(r.threw);
            parts.push(mkToolPart("c" + i, "bash", errState("exit status 1")));
        }
    } finally {
        clearJudgeEnv();
        delete process.env.SESSION_PROGRESS_REPO_ROOT;
        mod.__test.setNow(() => Date.now());
        server.close();
        rmSync(root, { recursive: true, force: true });
    }
    return { results, requests };
}

test("[ss4] hook: full semantic chain — confident looping verdict + adverse priors -> bounded denies; no raw args egress", async () => {
    const mod = await loadPlugin();
    const { results, requests } = await semDrive(mod, "ok");
    const denied = results.map((t, i) => (t ? i + 1 : 0)).filter((x) => x > 0);
    // semantic.max_denials defaults to 2: the arming deny + one lease deny.
    assert.ok(denied.length >= 1 && denied.length <= 2,
        `1..2 semantic denies (arm + lease hit), got ${denied.length}`);
    assert.ok(denied[0] >= 5, `deny only after enriched adverse priors (got call ${denied[0]})`);
    const err = results[denied[0] - 1];
    assert.ok(err.message.startsWith("[session-progress]"));
    assert.ok(err.message.includes("rule semantic"), "semantic rule named");
    const bodyStr = JSON.stringify(requests[0].body);
    assert.ok(!bodyStr.includes("make boom"), "raw args never sent to the judge");
    assert.ok(bodyStr.includes("current_signature"), "packet shape");
});

test("[ss4] hook: stuck/productive verdicts NEVER deny", async () => {
    const mod = await loadPlugin();
    for (const behavior of ["stuck", "productive"]) {
        const { results } = await semDrive(mod, behavior);
        assert.ok(results.every((t) => t === null), `${behavior} never denies`);
    }
});

test("[ss4] hook: judge timeout fails OPEN (allow)", async () => {
    const mod = await loadPlugin();
    const { results } = await semDrive(mod, "stall-headers", { timeout_ms: 300, calls: 4 });
    assert.ok(results.every((t) => t === null), "timeout -> allow");
});

test("[ss4] hook: judge unavailable -> semantic disabled, everything allows", async () => {
    const mod = await loadPlugin();
    mod.__test.resetState();
    const root = freshRoot();
    process.env.SESSION_PROGRESS_REPO_ROOT = root;
    const cfgmod = await loadConfigModule();
    cfgmod.__resetConfigCacheForTest();
    writeConfig(root, {
        agents: { "*": "enforce" },
        cadence: { min_interval_seconds: 1, min_new_signatures: 1 },
        mechanical: { enabled: false },
    });
    clearJudgeEnv();
    const parts = [];
    const client = {
        session: { messages: async () => ({ data: [{ role: "assistant", agent: "build", parts: parts.slice() }] }) },
    };
    const hooks = await mod.server({ client, directory: root });
    let clock = 0;
    mod.__test.setNow(() => clock);
    try {
        for (let i = 1; i <= 6; i++) {
            clock += 600;
            const r = await driveHook(hooks, { args: { command: "make nojudge" }, callID: "c" + i });
            assert.equal(r.threw, null, `call ${i} allows with no judge configured`);
            parts.push(mkToolPart("c" + i, "bash", errState("exit status 1")));
        }
    } finally {
        delete process.env.SESSION_PROGRESS_REPO_ROOT;
        mod.__test.setNow(() => Date.now());
        rmSync(root, { recursive: true, force: true });
    }
});

test("[ss4] hook: denies stay bounded by lease caps (own-denial echoes are not new evidence)", async () => {
    const mod = await loadPlugin();
    const { results } = await semDrive(mod, "ok", { calls: 10 });
    const denied = results.filter((t) => t).length;
    assert.ok(denied >= 1 && denied <= 2, `denies bounded by lease caps (got ${denied})`);
});

test("[ss4] diagnostics: verdicts.jsonl written, scrubbed; status.json present", async () => {
    const mod = await loadPlugin();
    mod.__test.resetState();
    const root = freshRoot();
    process.env.SESSION_PROGRESS_REPO_ROOT = root;
    const cfgmod = await loadConfigModule();
    cfgmod.__resetConfigCacheForTest();
    writeConfig(root, {
        agents: { "*": "enforce" },
        cadence: { min_interval_seconds: 1, min_new_signatures: 1 },
    });
    const parts = [];
    const client = {
        session: { messages: async () => ({ data: [{ role: "assistant", agent: "build", parts: parts.slice() }] }) },
    };
    const hooks = await mod.server({ client, directory: root });
    let clock = 0;
    mod.__test.setNow(() => clock);
    let deniedAt = null;
    try {
        for (let i = 1; i <= 10 && deniedAt === null; i++) {
            clock += 600;
            const r = await driveHook(hooks, { args: { command: "make diags" }, callID: "c" + i });
            parts.push(mkToolPart("c" + i, "bash", errState("exit status 1")));
            if (r.threw) deniedAt = i;
        }
        assert.ok(deniedAt !== null, "a deny happened");
        await new Promise((r) => setTimeout(r, 80));
        const dir = join(root, "tmp", "agent-runs", "session-progress-pilot");
        const lines = fs.readFileSync(join(dir, "verdicts.jsonl"), "utf8").trim().split("\n");
        assert.ok(lines.length >= 1, "diagnostic records written");
        const denyRec = lines.map((l) => JSON.parse(l)).find((r) => r.kind === "deny");
        assert.ok(denyRec, "deny record present");
        assert.equal(denyRec.action, "deny");
        assert.ok(!JSON.stringify(lines).includes("make diags"), "no raw command in diagnostics");
        assert.ok(fs.existsSync(join(dir, "status.json")), "status snapshot written");
    } finally {
        delete process.env.SESSION_PROGRESS_REPO_ROOT;
        mod.__test.setNow(() => Date.now());
        rmSync(root, { recursive: true, force: true });
    }
});

// [B2-pin] diagnostics.enabled is a REAL gate, not a documented no-op
// (commit-review B2): false -> the plugin creates NEITHER verdicts.jsonl NOR
// status.json (no diag directory at all, nothing queued); flipping the SAME
// config file to true (mtime reload, no restart) resumes writing BOTH files.
// With default cadence every eligible call records a spend-gate skip, so
// records WOULD flow if the gate were missing — the hook still allows every
// call in both phases (only diagnostics are gated, never the decision).
test("[ss4] diagnostics.enabled=false writes NEITHER verdicts.jsonl NOR status.json; true writes both", async () => {
    const mod = await loadPlugin();
    mod.__test.resetState();
    const root = freshRoot();
    process.env.SESSION_PROGRESS_REPO_ROOT = root;
    const cfgmod = await loadConfigModule();
    cfgmod.__resetConfigCacheForTest();
    writeConfig(root, {
        agents: { "*": "enforce" },
        diagnostics: { enabled: false },
    });
    const parts = [];
    const client = {
        session: { messages: async () => ({ data: [{ role: "assistant", agent: "build", parts: parts.slice() }] }) },
    };
    const hooks = await mod.server({ client, directory: root });
    let clock = 0;
    mod.__test.setNow(() => clock);
    const diagDir = join(root, "tmp", "agent-runs", "session-progress-pilot");
    try {
        // Disabled phase: calls allow, nothing touches disk.
        for (let i = 1; i <= 4; i++) {
            clock += 600;
            const r = await driveHook(hooks, { args: { command: "make quiet" }, callID: "c" + i });
            assert.equal(r.threw, null, `call ${i} still allows (only diagnostics are gated)`);
            parts.push(mkToolPart("c" + i, "bash", errState("exit status 1")));
        }
        await new Promise((r) => setTimeout(r, 80));
        assert.equal(mod.__test.diagStats().records, 0, "no records queued while disabled");
        assert.equal(fs.existsSync(diagDir), false, "diag directory not created while disabled");
        assert.equal(fs.existsSync(join(diagDir, "verdicts.jsonl")), false, "no verdicts.jsonl while disabled");
        assert.equal(fs.existsSync(join(diagDir, "status.json")), false, "no status.json while disabled");

        // Flip to enabled (same file, new mtime -> live reload): the next
        // calls write BOTH files — the gate reads the live config, not a
        // startup snapshot. Clock is well past the 2s status coalesce window.
        writeConfig(root, {
            agents: { "*": "enforce" },
            diagnostics: { enabled: true },
        });
        for (let i = 5; i <= 10; i++) {
            clock += 600;
            const r = await driveHook(hooks, { args: { command: "make quiet" }, callID: "c" + i });
            assert.equal(r.threw, null, `call ${i} still allows once enabled`);
            parts.push(mkToolPart("c" + i, "bash", errState("exit status 1")));
        }
        await new Promise((r) => setTimeout(r, 80));
        assert.ok(mod.__test.diagStats().records >= 1, "records flow again once enabled");
        assert.ok(fs.existsSync(join(diagDir, "verdicts.jsonl")), "verdicts.jsonl written once enabled");
        assert.ok(fs.existsSync(join(diagDir, "status.json")), "status.json written once enabled");
    } finally {
        delete process.env.SESSION_PROGRESS_REPO_ROOT;
        mod.__test.setNow(() => Date.now());
        rmSync(root, { recursive: true, force: true });
    }
});

// [opt-in pin] Off-by-default (operator decision 2026-10-04): selecting the
// pack with NO config file must yield a completely INERT plugin — every call
// allows immediately, nothing is observed (no ring entries, no accrual), no
// diagnostics dir/files are created, and no judge HTTP call is made (a live
// judge endpoint + env are wired so any pipeline execution would be COUNTED;
// history reads are counted too). Writing {"*":"audit"} resumes observation
// on the next call (mtime live reload, no restart) — the explicit opt-in
// path. This pins that the off early-return precedes ALL observation.
test("[opt-in pin] no config => hook fully inert (no records, no diagnostics, no judge calls); {\"*\":\"audit\"} => observing resumes", async () => {
    const mod = await loadPlugin();
    mod.__test.resetState();
    const root = freshRoot(); // NO config file — the shipped default state
    process.env.SESSION_PROGRESS_REPO_ROOT = root;
    const cfgmod = await loadConfigModule();
    cfgmod.__resetConfigCacheForTest();
    const { server, url, requests } = await startJudgeServer("ok");
    withJudgeEnv(url); // judge WOULD be reachable and counted if anything ran
    let historyReads = 0;
    const client = {
        session: {
            messages: async () => {
                historyReads += 1;
                return { data: [] };
            },
        },
    };
    const hooks = await mod.server({ client, directory: root });
    let clock = 0;
    mod.__test.setNow(() => clock);
    const diagDir = join(root, "tmp", "agent-runs", "session-progress-pilot");
    try {
        // Phase 1 — no config: the hook is a pure no-op beyond the allow.
        for (let i = 1; i <= 6; i++) {
            clock += 600;
            const r = await driveHook(hooks, { args: { command: "make inert" }, callID: "c" + i });
            assert.equal(r.threw, null, `no-config call ${i} allows immediately`);
        }
        await new Promise((r) => setTimeout(r, 80));
        for (const st of mod.__test.sessions().values()) {
            assert.equal(st.ring.length, 0, "no-config: nothing observed (empty rings)");
            assert.equal(st.spend.accrued, 0, "no-config: no spend accrual");
        }
        assert.equal(mod.__test.diagStats().records, 0, "no-config: no diagnostic records queued");
        assert.equal(fs.existsSync(diagDir), false, "no-config: diagnostics directory never created");
        assert.equal(fs.existsSync(join(diagDir, "verdicts.jsonl")), false, "no-config: no verdicts.jsonl");
        assert.equal(fs.existsSync(join(diagDir, "status.json")), false, "no-config: no status.json");
        assert.equal(historyReads, 0, "no-config: no history reads");
        assert.equal(requests.length, 0, "no-config: ZERO judge HTTP calls (no LLM spend)");

        // Phase 2 — explicit opt-in {"*":"audit"}: observing resumes on the
        // very next call. The scripted clock keeps the spend gate closed
        // (default 60s/8obs), so audit records observations but never judges.
        writeConfig(root, { agents: { "*": "audit" } });
        for (let i = 7; i <= 12; i++) {
            clock += 600;
            const r = await driveHook(hooks, { args: { command: "make inert" }, callID: "c" + i });
            assert.equal(r.threw, null, `audit call ${i} never throws`);
        }
        const st = mod.__test.sessions().get("s1");
        assert.ok(st, "audit: session observed");
        assert.equal(st.ring.length, 6, "audit: observation resumed and recorded");
        assert.equal(st.spend.accrued, 6, "audit: accrual resumed");
        assert.equal(requests.length, 0, "spend gate closed by design: still zero judge calls");
    } finally {
        clearJudgeEnv();
        delete process.env.SESSION_PROGRESS_REPO_ROOT;
        mod.__test.setNow(() => Date.now());
        server.close();
        rmSync(root, { recursive: true, force: true });
    }
});

// ===========================================================================
// [ss5] Judge target dual-form resolution (operator decision 2026-10-05 —
// literal + user-level file + env fallback, mirroring auto-gate's
// literal-preferred pattern). Pins: literal-key resolution, user-file
// resolution, per-field precedence repo-literal > user-file > env, the
// all-three-required availability rule, and fail-open on missing/garbage
// user JSON.
// ===========================================================================

test("[ss5] defaults carry NO literal judge values; empty literals never suppress env fallback", async () => {
    const cfgmod = await loadConfigModule();
    const judge = await loadJudge();
    const cfg = cfgmod.normalizeConfig({});
    assert.equal(cfg.judge.endpoint, "", "no literal endpoint in shipped defaults");
    assert.equal(cfg.judge.model, "", "no literal model in shipped defaults");
    assert.equal(cfg.judge.api_key, "", "no literal api_key in shipped defaults");
    assert.equal(cfg.judge.user_config_path, "", "no user-config override by default");
    // The non-empty-guard rule: empty literals are "unspecified" — env still
    // resolves (auto-gate's dual-form guard, pinned).
    const t = judge.judgeTarget(cfg, {
        SESSION_PROGRESS_JUDGE_MODEL: "m",
        SESSION_PROGRESS_JUDGE_ENDPOINT: "https://x/y",
        SESSION_PROGRESS_JUDGE_API_KEY: "k",
    });
    assert.deepEqual(t, { endpoint: "https://x/y", model: "m", apiKey: "k" });
});

test("[ss5] literal keys: judgeTarget resolves repo-config literals with NO env", async () => {
    const cfgmod = await loadConfigModule();
    const judge = await loadJudge();
    const cfg = cfgmod.normalizeConfig({
        judge: { endpoint: "https://literal/v1/chat/completions", model: "lit-model", api_key: "lit-key" },
    });
    assert.deepEqual(judge.judgeTarget(cfg, {}), {
        endpoint: "https://literal/v1/chat/completions",
        model: "lit-model",
        apiKey: "lit-key",
    });
    // runJudge end-to-end on a literal-only target (env empty).
    const { server, url, requests } = await startJudgeServer("ok");
    try {
        const litCfg = cfgmod.normalizeConfig({
            judge: {
                endpoint: url, model: "lit-model", api_key: "lit-key",
                timeout_ms: 1000,
            },
        });
        const r = await judge.runJudge(litCfg, { observations: [{ id: "1" }] }, { env: {} });
        assert.equal(r.status, "verdict", "literal-only target drives a real judge call");
        assert.equal(r.verdict.verdict, "looping");
        assert.equal(requests.length, 1);
        assert.equal(requests[0].body.model, "lit-model");
    } finally {
        server.close();
    }
});

test("[ss5] per-field precedence: repo literal > user file > env (partial layering is legitimate)", async () => {
    const cfgmod = await loadConfigModule();
    const judge = await loadJudge();
    const root = freshRoot();
    const userPath = join(root, "user-judge.json");
    writeFileSync(userPath, JSON.stringify({
        endpoint: "https://userfile/v1/chat/completions",
        model: "user-model",
        apiKey: "user-key",
    }));
    try {
        // Repo literal endpoint WINS over the user file; user file fills
        // model; env supplies the key (neither repo nor user has it).
        const cfg = cfgmod.normalizeConfig({
            judge: { endpoint: "https://repo-literal/v1/chat/completions", user_config_path: userPath },
        });
        const merged = cfgmod.mergeUserJudgeConfig(cfg, cfgmod.loadUserJudgeConfig(cfg, {}));
        assert.equal(merged.judge.endpoint, "https://repo-literal/v1/chat/completions", "repo literal wins");
        assert.equal(merged.judge.model, "user-model", "user file fills the empty repo field");
        assert.equal(merged.judge.api_key, "user-key", "user file key merged");
        const t = judge.judgeTarget(merged, {});
        assert.deepEqual(t, {
            endpoint: "https://repo-literal/v1/chat/completions",
            model: "user-model",
            apiKey: "user-key",
        });

        // Now a user view without a key: env must supply it — per-field
        // layering across all three levels, auto-gate style.
        const partialUser = { endpoint: "https://userfile/v1/chat/completions", model: "user-model", apiKey: "" };
        const merged2 = cfgmod.mergeUserJudgeConfig(
            cfgmod.normalizeConfig({ judge: { user_config_path: userPath } }), partialUser);
        const t2 = judge.judgeTarget(merged2, { SESSION_PROGRESS_JUDGE_API_KEY: "env-key" });
        assert.deepEqual(t2, {
            endpoint: "https://userfile/v1/chat/completions",
            model: "user-model",
            apiKey: "env-key",
        }, "endpoint+model from user file, key from env resolves");

        // The partial-source rule: ANY unresolved field => unavailable.
        const noKey = judge.judgeTarget(merged2, {});
        assert.equal(noKey, null, "missing key alone makes the judge unavailable (fail-open)");
    } finally {
        rmSync(root, { recursive: true, force: true });
    }
});

test("[ss5] user-level file: auto-gate field aliases accepted (modelEndpoint, api_key)", async () => {
    const cfgmod = await loadConfigModule();
    const root = freshRoot();
    const userPath = join(root, "user-judge.json");
    writeFileSync(userPath, JSON.stringify({
        modelEndpoint: "https://alias/v1/chat/completions",
        model: "alias-model",
        api_key: "alias-key",
    }));
    try {
        const cfg = cfgmod.normalizeConfig({ judge: { user_config_path: userPath } });
        const vals = cfgmod.loadUserJudgeConfig(cfg, {});
        assert.deepEqual(vals, { endpoint: "https://alias/v1/chat/completions", model: "alias-model", apiKey: "alias-key" });
    } finally {
        rmSync(root, { recursive: true, force: true });
    }
});

test("[ss5] missing user file: silent empty values (the normal no-user-config state)", async () => {
    const cfgmod = await loadConfigModule();
    cfgmod.__resetConfigCacheForTest();
    const root = freshRoot(); // no user file anywhere
    const errs = [];
    const orig = console.error;
    console.error = (...a) => errs.push(a.join(" "));
    try {
        const cfg = cfgmod.normalizeConfig({ judge: { user_config_path: join(root, "absent.json") } });
        assert.deepEqual(cfgmod.loadUserJudgeConfig(cfg, {}), { endpoint: "", model: "", apiKey: "" });
        cfgmod.loadUserJudgeConfig(cfg, {}); // cached path also silent
        assert.deepEqual(errs, [], "missing user file must be SILENT");
    } finally {
        console.error = orig;
        rmSync(root, { recursive: true, force: true });
    }
});

test("[ss5] invalid user JSON: empty values + exactly ONE deduped warn; state transition re-warns; mtime reload works", async () => {
    const cfgmod = await loadConfigModule();
    cfgmod.__resetConfigCacheForTest();
    const root = freshRoot();
    const userPath = join(root, "user-judge.json");
    writeFileSync(userPath, "{ not json");
    const errs = [];
    const orig = console.error;
    console.error = (...a) => errs.push(a.join(" "));
    try {
        const cfg = () => cfgmod.normalizeConfig({ judge: { user_config_path: userPath } });
        assert.deepEqual(cfgmod.loadUserJudgeConfig(cfg(), {}), { endpoint: "", model: "", apiKey: "" });
        cfgmod.loadUserJudgeConfig(cfg(), {});
        cfgmod.loadUserJudgeConfig(cfg(), {});
        assert.equal(errs.length, 1, "persistent invalid file warns exactly ONCE");
        assert.ok(errs[0].includes("judge user config invalid"), "warn names the user config");
        // Fix the file (new mtime -> reload): values appear, no new warn.
        writeFileSync(userPath, JSON.stringify({ endpoint: "https://fixed/v1", model: "m2", apiKey: "k2" }));
        assert.deepEqual(
            cfgmod.loadUserJudgeConfig(cfg(), {}),
            { endpoint: "https://fixed/v1", model: "m2", apiKey: "k2" },
            "mtime change reloads the user file");
        assert.equal(errs.length, 1, "recovery adds no warn");
    } finally {
        console.error = orig;
        rmSync(root, { recursive: true, force: true });
    }
});

test("[ss5] default user path resolves via XDG_CONFIG_HOME; judge.user_config_path override wins", async () => {
    const cfgmod = await loadConfigModule();
    const root = freshRoot();
    const xdg = join(root, "xdg");
    mkdirSync(join(xdg, "vh-agent-harness"), { recursive: true });
    const defPath = join(xdg, "vh-agent-harness", "session-progress-llm.json");
    writeFileSync(defPath, JSON.stringify({ endpoint: "https://xdg/v1", model: "xdg-model", apiKey: "xdg-key" }));
    try {
        assert.equal(
            cfgmod.defaultUserJudgeConfigPath({ XDG_CONFIG_HOME: xdg }),
            defPath,
            "default path = <XDG_CONFIG_HOME>/vh-agent-harness/session-progress-llm.json");
        const noOverride = cfgmod.normalizeConfig({});
        assert.deepEqual(
            cfgmod.loadUserJudgeConfig(noOverride, { XDG_CONFIG_HOME: xdg }),
            { endpoint: "https://xdg/v1", model: "xdg-model", apiKey: "xdg-key" },
            "default-path load reads the XDG file");
        const withOverride = cfgmod.normalizeConfig({ judge: { user_config_path: "/elsewhere.json" } });
        assert.equal(cfgmod.judgeUserConfigPath(withOverride, { XDG_CONFIG_HOME: xdg }), "/elsewhere.json");
    } finally {
        rmSync(root, { recursive: true, force: true });
    }
});

test("[ss5] plugin wiring: the hook merges the user-level file (judge call fired from a user-file target, env unset)", async () => {
    const mod = await loadPlugin();
    mod.__test.resetState();
    const cfgmod = await loadConfigModule();
    cfgmod.__resetConfigCacheForTest();
    const root = freshRoot();
    process.env.SESSION_PROGRESS_REPO_ROOT = root;
    const { server, url, requests } = await startJudgeServer("ok");
    // The user-level file supplies the ENTIRE target (env stays unset —
    // withJudgeEnv is deliberately NOT used).
    const userPath = join(root, "user-judge.json");
    writeFileSync(userPath, JSON.stringify({
        endpoint: url, model: "userfile-model", apiKey: "userfile-key",
    }));
    // Repo config: audit + open cadence + the user_config_path override.
    // Written DIRECTLY (not via writeConfig) so the hermeticity pin does not
    // clobber the explicit override this test exercises.
    const p = join(root, CONFIG_PATH);
    mkdirSync(dirname(p), { recursive: true });
    writeFileSync(p, JSON.stringify({
        agents: { "*": "audit" },
        cadence: { min_interval_seconds: 1, min_new_signatures: 1 },
        judge: { timeout_ms: 1500, user_config_path: userPath },
    }));
    const parts = [];
    const client = {
        session: { messages: async () => ({ data: [{ role: "assistant", agent: "build", parts: parts.slice() }] }) },
    };
    const hooks = await mod.server({ client, directory: root });
    let clock = 0;
    mod.__test.setNow(() => clock);
    try {
        for (let i = 1; i <= 4; i++) {
            clock += 600;
            const r = await driveHook(hooks, { args: { command: "make userwired" }, callID: "c" + i });
            assert.equal(r.threw, null, `audit call ${i} never throws`);
            parts.push(mkToolPart("c" + i, "bash", errState("exit status 1")));
        }
        await new Promise((r) => setTimeout(r, 120));
        assert.ok(requests.length >= 1, "judge HTTP call fired from the user-file target");
        assert.equal(requests[0].body.model, "userfile-model", "judge used the user-file model");
        const verdictsPath = join(root, "tmp", "agent-runs", "session-progress-pilot", "verdicts.jsonl");
        const rows = fs.readFileSync(verdictsPath, "utf8").split("\n").filter((l) => l.trim()).map((l) => JSON.parse(l));
        const assessed = rows.filter((r) => r.kind === "assessment" && r.judge === true);
        assert.ok(assessed.length >= 1, "an assessment record with judge:true landed");
        for (const r of rows) {
            assert.ok(!JSON.stringify(r).includes("userfile-key"), "no literal key leaks into diagnostics");
        }
    } finally {
        delete process.env.SESSION_PROGRESS_REPO_ROOT;
        mod.__test.setNow(() => Date.now());
        server.close();
        rmSync(root, { recursive: true, force: true });
    }
});
