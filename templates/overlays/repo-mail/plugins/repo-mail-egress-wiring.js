// repo-mail-egress-wiring.js — INTEGRATION wiring of the generic domain-free
// egress gate (templates/core) to the shared scrubCredentials seam
// (auto-classifier-pilot/plugins/auto-gate-scrub.js).
//
// ┌──────────────────────────────────────────────────────────────────────────┐
// │ OWNERSHIP: overlay_extension (auto-overwritten while the pack is active).  │
// │ This is the repo-mail overlay pack's integration unit. It is rendered into │
// │ .opencode/plugins/ when the `repo-mail` pack is selected in               │
// │ .vh-agent-harness/vh-harness-profile.yml. It wires the DOMAIN-FREE gate    │
// │ (core) to the real scrubCredentials helper (auto-classifier-pilot).        │
// └──────────────────────────────────────────────────────────────────────────┘
//
// WHY THIS IS AN OVERLAY, NOT CORE
//
// The gate CONTRACT + GENERIC logic lives in templates/core
// (repo-mail-egress-gate.js) and is DOMAIN-FREE: it has NO dependency on any
// overlay, no vendor, no repo identity. The gate accepts `scrubCredentials` as
// an INJECTED dependency precisely so core never imports from an overlay
// (that would be a layering violation — overlays are optional and project-
// specific, core is the always-shipped base).
//
// This module is the wiring layer that lives in the overlay (where the real
// scrub helper also lives) and binds the two. A repo-mail sender (Slice 5/6)
// calls `createEgressGate(...)` once and uses the returned gate function on
// every outbound message.
//
// WHY DEPENDENCY INJECTION (not static cross-pack import)
//
// A static import of the gate from core (via ../repo-configs/...) would use a
// RENDERED-relative path that is correct only under .opencode/ (the rendered
// tree), not under templates/ (the source tree where this self-test runs).
// Rather than carry two path variants, this module takes the gate + scrub as
// INJECTED dependencies and stays path-free. The self-test below imports the
// REAL gate + REAL scrub from their source paths (which resolve at source) to
// prove the composition end-to-end. Production callers inject the same
// dependencies resolved from the rendered tree.
//
// REJECT-not-transform invariant is HONORED here transitively: this module
// passes scrubCredentials to the gate as a DETECTOR only. It NEVER calls
// scrubCredentials to transform a message. The gate's contract enforces this;
// this wiring does not weaken it.
//
// DECISION 2 (Non-Actuation by Construction) HONORED: this module introduces no
// actuation vocabulary. It composes a validator (gate) with a detector (scrub)
// and returns a verdict. No delivery-rule schema, no session/task/work verbs.
//
// Naming: all identifiers GENERIC. No repo name, vendor, transport, or endpoint.
//
// DUAL-PURPOSE SELF-TEST: running this file directly proves the wired gate with
// the REAL scrubCredentials (not a stub) in BOTH directions:
//   - DIRTY → rejected (repo id, credential, endpoint, unknown field).
//   - CLEAN → passes (byte-identity of canonical bytes preserved).
// This complements the gate's own self-test (which uses a stub for isolation).
// Here the REAL auto-gate-scrub.js scrubCredentials exercises the mutation-check
// against the production credential detector.

import { fileURLToPath, pathToFileURL } from "node:url";
import path from "node:path";
import fs from "node:fs";
import { test } from "node:test";
import { strict as assert } from "node:assert";

// OpenCode plugin-loader guard — DO NOT REMOVE. This module lives under
// plugins/ (auto-scanned by the OpenCode loader) but is a pure library, not a
// plugin (no server() export). A single NON-FUNCTION export trips the loader's
// "export is not a function" guard so the whole file is skipped as a non-plugin.
// Mirrors the auto-gate-scrub.js / auto-gate-verdict.js sentinel pattern.
export const __repoMailEgressLibrary = "wiring";

// ---------------------------------------------------------------------------
// createEgressGate(deps) — bind the gate to a scrub detector + private deny-
// rules, returning a ready-to-use gate function.
//
// deps:
//   - gate:            the gateEgressMessage function from the core module
//                      (templates/core/.opencode/repo-configs/repo-mail-egress-gate.js).
//   - scrubCredentials: the real scrub helper (auto-gate-scrub.js), used as a
//                      DETECTOR (mutation => reject, never transform-and-send).
//   - privateDenyRules: array of {id, test, why} — the project's private deny-
//                      rules (repo name, feature names, specific endpoints).
//                      Defaults to [] (generic-only). Loaded by the project from
//                      private config via loadPrivateDenyRules() below.
//
// Returns: (message) => gateEgressMessage(message, { scrubCredentials, privateDenyRules })
//
// The returned function has the SAME return shape as the gate:
//   { verdict: "passed"|"rejected", canonicalBytes: string|null, reasons: string[] }
//
// CAPTURE-ONCE SEMANTICS (load-bearing — read before wiring a live sender):
//   privateDenyRules are captured in the closure ONCE at construction. The
//   returned egressGate(message) reuses the rules captured at construction on
//   every call — it does NOT reload private rules per send. A private deny-rule
//   added/updated AFTER construction is invisible to an existing gate.
//
//   CONSEQUENCE — to pick up rule changes the caller MUST construct a FRESH gate
//   per send (or per send-batch/session):
//       const gate = createEgressGate({
//           gate: gateEgressMessage,
//           scrubCredentials,
//           privateDenyRules: await loadPrivateDenyRules(
//               ".opencode/repo-configs/repo-mail-deny.private.js"),
//       });
//       gate(message);   // consults the rules captured above
//
//   This keeps egressGate(message) SYNCHRONOUS (the core gate is not async and
//   must not become async). Callers wanting always-fresh rules construct a new
//   gate via createEgressGate({ ..., privateDenyRules: await loadPrivateDenyRules(...) })
//   immediately before each send.
// ---------------------------------------------------------------------------

export function createEgressGate(deps) {
    const {
        gate,
        scrubCredentials,
        privateDenyRules = [],
    } = deps || {};
    if (typeof gate !== "function") {
        throw new TypeError("createEgressGate: deps.gate must be the gateEgressMessage function");
    }
    if (typeof scrubCredentials !== "function") {
        throw new TypeError("createEgressGate: deps.scrubCredentials must be a function");
    }
    if (!Array.isArray(privateDenyRules)) {
        throw new TypeError("createEgressGate: deps.privateDenyRules must be an array if provided");
    }
    return function egressGate(message) {
        return gate(message, { scrubCredentials, privateDenyRules });
    };
}

// ---------------------------------------------------------------------------
// resolvePrivateDenyRulesModule(modulePath, repoRoot) — resolve a documented
// private-rule module path to an ABSOLUTE file: URL for `import()`. PURE (no
// I/O, no import) and unit-testable.
//
// THE F1 FIX: a relative specifier passed directly to `import()` resolves
// against the LOADER MODULE's location (this file, rendered under
// .opencode/plugins/), NOT the repo root. So a documented repo-relative path
// like `.opencode/repo-configs/repo-mail-deny.private.js` would silently 404,
// be caught as ERR_MODULE_NOT_FOUND, and return [] — DISABLING the project's
// private identity deny-rules while keeping only generic matching. Resolving
// repo-relative paths against an EXPLICIT repo root BEFORE import() closes the
// silent-404 gap.
//
// Resolution rules:
//   - `file:` URL            → returned unchanged (caller passed an absolute URL).
//   - absolute path          → converted to a file: URL (cross-platform-safe).
//   - relative path (e.g.    → resolved against repoRoot, then converted to a
//     `.opencode/.../x.js`,    file: URL. repoRoot defaults to process.cwd()
//     `./x.js`)                (the conventional repo root when the sender runs
//                              from the repo root; overridable via the opts
//                              parameter on loadPrivateDenyRules).
//
// `modulePath` is a FILE PATH (repo-relative or absolute), NOT a bare package
// specifier — this helper loads a project's gitignored private-rules module by
// path. Returns "" for a non-string/empty input.
// ---------------------------------------------------------------------------

export function resolvePrivateDenyRulesModule(modulePath, repoRoot) {
    if (!modulePath || typeof modulePath !== "string") return "";
    // file: URL → use as-is.
    if (modulePath.startsWith("file:")) return modulePath;
    // Absolute path → convert to a file: URL (cross-platform-safe for import()).
    if (path.isAbsolute(modulePath)) {
        return pathToFileURL(modulePath).href;
    }
    // Relative path → resolve against the repo root, then file: URL.
    const base = repoRoot && typeof repoRoot === "string" ? repoRoot : process.cwd();
    return pathToFileURL(path.resolve(base, modulePath)).href;
}

// ---------------------------------------------------------------------------
// loadPrivateDenyRules(modulePath, opts?) — load a project's private deny-rules
// from a JS module that exports `REPO_MAIL_DENY_RULES` (an array of
// {id, test, why}).
//
// This is the production mechanism: a project drops its private rules in a
// gitignored JS file (e.g. .opencode/repo-configs/repo-mail-deny.private.js)
// and the sender loads them via this helper. The module path is repo-relative
// or absolute (or a file: URL).
//
// opts.repoRoot — optional explicit repo root for resolving repo-relative paths.
//   Defaults to process.cwd() (the conventional repo root when the sender runs
//   from the repo root). Inject it when the sender is invoked from a different
//   directory.
//
// NET GUARANTEE (closes the silent-disable class):
//   `[]` is returned ONLY for (a) a no/invalid modulePath (the early return
//   before any FS work) or (b) ENOENT on the resolved top-level path (a
//   genuinely-absent private-rules file). Every PRESENT-but-broken case
//   SURFACES LOUDLY (throws), never silently disabling the private deny-rules.
//   Specifically:
//   - No/invalid modulePath (non-string/empty) → [] (nothing to load).
//   - Top-level file genuinely absent (ENOENT) → [] (fail-safe: the GENERIC
//     core rules still apply).
//   - Top-level file PRESENT but broken in ANY way (missing dependency →
//     ERR_MODULE_NOT_FOUND, syntax error, runtime error, bad export shape) →
//     THROWS. The previous err.code===ERR_MODULE_NOT_FOUND catch CONFLATED the
//     missing-dependency case with "absent" and silently returned [] — that was
//     the silent-disable bug. The existence-check approach below makes the
//     distinction structural, not code-based.
//   - Top-level file present but does NOT export REPO_MAIL_DENY_RULES as an
//     array → THROWS (a present private-rules module must export the expected
//     shape; silently returning [] would mask a misconfiguration).
//   - fs access failure that is NOT ENOENT (EACCES/EIO/...) → THROWS (cannot
//     determine existence → surface, never silently []).
// ---------------------------------------------------------------------------

export async function loadPrivateDenyRules(modulePath, opts = {}) {
    if (!modulePath || typeof modulePath !== "string") return [];
    const repoRoot = opts && typeof opts.repoRoot === "string" ? opts.repoRoot : process.cwd();
    const resolved = resolvePrivateDenyRulesModule(modulePath, repoRoot);

    // --- Existence check BEFORE import. We use fs.statSync (NOT existsSync)
    //     because existsSync returns false on ANY access error (including
    //     EACCES/EIO), which would silently misclassify a permission/IO problem
    //     as "absent" → []. statSync throws with a .code, so we can distinguish
    //     ENOENT (genuinely absent → []) from every other access error (surface).
    //     This is the ONLY path to [].
    const resolvedPath = fileURLToPath(resolved);
    try {
        fs.statSync(resolvedPath);
    } catch (statErr) {
        if (statErr && statErr.code === "ENOENT") {
            // Genuinely absent top-level rules file → fail-safe → []. Generic
            // core rules still apply. This is the ONLY [] path.
            return [];
        }
        // EACCES / EIO / ... — cannot determine existence → surface loudly.
        throw statErr;
    }

    // --- Top-level file EXISTS. Import it with NO catch-and-return-[]: a
    //     present file broken in ANY way (missing dependency, syntax error,
    //     runtime error) MUST surface. The old err.code===ERR_MODULE_NOT_FOUND
    //     catch lived here and was the silent-disable bug; it is GONE. Any
    //     rejection from import() propagates to the caller.
    const mod = await import(resolved);

    // --- Export-shape check. A present file that does not export
    //     REPO_MAIL_DENY_RULES as an array is a misconfiguration → surface
    //     loudly. The old `Array.isArray(rules) ? rules : []` silently returned
    //     [] here, masking a present-but-malformed module.
    const rules = mod && mod.REPO_MAIL_DENY_RULES;
    if (!Array.isArray(rules)) {
        const got =
            rules === undefined
                ? "undefined (no REPO_MAIL_DENY_RULES export)"
                : rules === null
                  ? "null"
                  : Array.isArray(rules)
                    ? "array"
                    : typeof rules;
        throw new Error(
            `loadPrivateDenyRules: private-rules module present at ${resolvedPath} but REPO_MAIL_DENY_RULES is ${got} (expected an array of {id, test, why}); a present private-rules module must export the expected shape — refusing to silently disable`,
        );
    }
    return rules;
}

// ===========================================================================
// DUAL-PURPOSE SELF-TEST — end-to-end with the REAL scrubCredentials.
//
// Run directly (`node repo-mail-egress-wiring.js` or
// `node --test repo-mail-egress-wiring.js`) to execute the suite. Import as a
// module -> NO tests run. Guard is an explicit __filename comparison.
//
// These tests import the REAL gate (from the core source) and the REAL
// scrubCredentials (from auto-classifier-pilot source) and prove the WIRED
// composition in BOTH directions. This is the production-fidelity proof: the
// gate's own self-test uses a stub; here the real credential detector runs.
// ===========================================================================
const __filename = fileURLToPath(import.meta.url);
const __isMain = path.resolve(process.argv[1] ?? "") === __filename;

if (__isMain) {
    // Source-path imports (this self-test runs from the source tree). The
    // production wiring resolves the same modules from the rendered tree
    // (.opencode/repo-configs/ + .opencode/plugins/) and injects them.
    const { gateEgressMessage } = await import(
        "../../../core/.opencode/repo-configs/repo-mail-egress-gate.js"
    );
    const { scrubCredentials } = await import(
        "../../auto-classifier-pilot/plugins/auto-gate-scrub.js"
    );

    // The wired gate: real scrubCredentials + no private rules (generic-only).
    const gate = createEgressGate({ gate: gateEgressMessage, scrubCredentials });

    // ===== DIRTY → rejected (real scrubCredentials as detector) =====

    test("REAL-SCRUB DIRTY: api_key in a claim → rejected (scrub mutation detected)", () => {
        const msg = {
            message_id: "m1",
            kind: "report",
            sender: { channel_id: "ch_abc123def", channel_class: "report" },
            claims: [{ statement: "verified with api_key=sk-abcdefghijklmnopqrstuvwxyz123456" }],
        };
        const res = gate(msg);
        assert.equal(res.verdict, "rejected");
        assert.ok(
            res.reasons.some((r) => r.includes("scrubCredentials mutation")),
            `mutation reason expected: ${JSON.stringify(res.reasons)}`,
        );
        // REJECT-not-transform: the credential survives UNMODIFIED in the
        // (unsent) canonical bytes — the gate refused, it did not scrub-and-send.
        assert.ok(res.canonicalBytes.includes("sk-abcdefghijklmnopqrstuvwxyz123456"));
        assert.ok(!res.canonicalBytes.includes("[redacted]"));
    });

    test("REAL-SCRUB DIRTY: Bearer token in a claim → rejected", () => {
        const msg = {
            message_id: "m2",
            kind: "report",
            sender: { channel_id: "ch_abc123def", channel_class: "report" },
            claims: [{ statement: "Authorization: Bearer eyJ0b2tlbj4.signature.payload here" }],
        };
        const res = gate(msg);
        assert.equal(res.verdict, "rejected");
        assert.ok(res.reasons.some((r) => r.includes("scrubCredentials mutation")));
    });

    test("REAL-SCRUB DIRTY: high-entropy hex blob → rejected", () => {
        const blob = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef";
        const msg = {
            message_id: "m3",
            kind: "report",
            sender: { channel_id: "ch_abc123def", channel_class: "report" },
            claims: [{ statement: `token blob ${blob}` }],
        };
        const res = gate(msg);
        assert.equal(res.verdict, "rejected");
    });

    test("REAL-SCRUB DIRTY: endpoint URL → rejected", () => {
        const msg = {
            message_id: "m4",
            kind: "report",
            sender: { channel_id: "ch_abc123def", channel_class: "report" },
            claims: [{ statement: "hit https://internal.svc.example.net/v1/debug" }],
        };
        const res = gate(msg);
        assert.equal(res.verdict, "rejected");
        assert.ok(res.reasons.some((r) => r.includes("endpoint-url")));
    });

    test("REAL-SCRUB DIRTY: repo-name slug as channel_id → rejected", () => {
        const msg = {
            message_id: "m5",
            kind: "report",
            sender: { channel_id: "my-cool-project", channel_class: "report" },
        };
        const res = gate(msg);
        assert.equal(res.verdict, "rejected");
        assert.ok(res.reasons.some((r) => r.includes("slug")));
    });

    test("REAL-SCRUB DIRTY: unknown field → rejected", () => {
        const msg = {
            message_id: "m6",
            kind: "report",
            sender: { channel_id: "ch_abc123def", channel_class: "report" },
            repo_url: "https://github.com/x/y",
        };
        const res = gate(msg);
        assert.equal(res.verdict, "rejected");
        assert.ok(res.reasons.some((r) => r.includes("unknown top-level field") && r.includes("repo_url")));
    });

    // ===== CLEAN → passes (real scrubCredentials, byte-identity preserved) =====

    test("REAL-SCRUB CLEAN: well-formed anonymized message passes (byte-identity preserved)", () => {
        const msg = {
            schema_version: "1",
            message_id: "ok1",
            kind: "report",
            thread_id: "thr_01",
            provenance_class: "ci",
            sender: { channel_id: "ch_abc123def", channel_class: "report", key_id: "key_01" },
            recipient: { channel_id: "ch_xyz789ghi", channel_class: "report" },
            issued_at: "2026-07-27T00:00:00Z",
            claims: [{ statement: "build green; no identity content anywhere" }],
            contradictions: ["none detected in checked scope"],
            scrub: { result: "passed" },
        };
        const res = gate(msg);
        assert.equal(res.verdict, "passed");
        assert.deepEqual(res.reasons, []);
        // Byte-identity: the canonical bytes are the unmodified serialization.
        // The caller sends EXACTLY these bytes to the adapter boundary.
        assert.ok(res.canonicalBytes.length > 0);
        assert.equal(
            res.canonicalBytes,
            JSON.stringify(
                (function sort(o) {
                    if (Array.isArray(o)) return o.map(sort);
                    if (o && typeof o === "object") {
                        const r = {};
                        for (const k of Object.keys(o).sort()) r[k] = sort(o[k]);
                        return r;
                    }
                    return o;
                })(msg),
            ),
        );
    });

    test("REAL-SCRUB CLEAN: minimal handoff passes", () => {
        const res = gate({ message_id: "h1", kind: "handoff", scrub: { result: "passed" } });
        assert.equal(res.verdict, "passed");
        assert.deepEqual(res.reasons, []);
    });

    test("REAL-SCRUB CLEAN: scrubCredentials is idempotent on clean text (no false mutation)", () => {
        // Proves the real scrubber does NOT mutate clean message scalars — so a
        // clean message is not falsely rejected by the mutation-check.
        const clean = "build green; no identity content anywhere";
        assert.equal(scrubCredentials(clean), clean, "real scrub must leave clean text unchanged");
    });

    // ===== wiring contract =====

    test("createEgressGate: rejects non-function gate dep", () => {
        assert.throws(
            () => createEgressGate({ gate: "x", scrubCredentials }),
            /deps\.gate must be/,
        );
    });

    test("createEgressGate: rejects non-function scrubCredentials dep", () => {
        assert.throws(
            () => createEgressGate({ gate: gateEgressMessage, scrubCredentials: 42 }),
            /deps\.scrubCredentials must be/,
        );
    });

    test("createEgressGate: rejects non-array privateDenyRules", () => {
        assert.throws(
            () => createEgressGate({ gate: gateEgressMessage, scrubCredentials, privateDenyRules: "bad" }),
            /privateDenyRules must be an array/,
        );
    });

    test("loadPrivateDenyRules: missing module → [] (no/invalid path or genuinely-absent top-level file)", async () => {
        // [] is returned ONLY for a no/invalid modulePath or a genuinely-absent
        // top-level rules file (ENOENT). This is the intended fail-safe: a
        // missing private extension means generic-only matching, not an error.
        const rules = await loadPrivateDenyRules("./nonexistent-private-rules.js");
        assert.deepEqual(rules, []);
    });

    test("wired gate with private deny-rule: repo-name match → rejected", async () => {
        // Simulate a project that has registered its private repo name as a deny-rule.
        const gateWithPrivate = createEgressGate({
            gate: gateEgressMessage,
            scrubCredentials,
            privateDenyRules: [
                { id: "private-repo", test: (t) => /my-secret-repo-name/i.test(t), why: "private" },
            ],
        });
        const msg = {
            message_id: "p1",
            kind: "report",
            sender: { channel_id: "ch_abc123def", channel_class: "report" },
            claims: [{ statement: "the my-secret-repo-name build failed" }],
        };
        const res = gateWithPrivate(msg);
        assert.equal(res.verdict, "rejected");
        assert.ok(res.reasons.some((r) => r.includes("private-repo")));
    });

    // ===== CAPTURE-ONCE semantics (B-F1) — construct a FRESH gate per send to
    //       pick up private-rule changes =====
    //
    // createEgressGate captures privateDenyRules in the closure ONCE at
    // construction; the returned egressGate(message) reuses the captured rules
    // and does NOT reload them per call. To pick up rule changes, the caller
    // constructs a FRESH gate per send (the documented contract above). This test
    // proves that construct-per-send pattern works AND documents that an existing
    // gate does not see later rule changes.

    test("capture-once (B-F1): a fresh gate constructed with updated rules consults the NEW rules", () => {
        // A clean message carrying a given private identifier string in a claim.
        // The identifier is otherwise harmless (no URL/path/email/credential
        // shape), so the ONLY thing that can reject it is a matching private
        // deny-rule — the generic rules + scrub check both pass.
        const msgCarrying = (ident) => ({
            message_id: "m_cap",
            kind: "report",
            sender: { channel_id: "ch_abc123def", channel_class: "report" },
            scrub: { result: "passed" },
            claims: [{ statement: `note about ${ident}` }],
        });

        const ruleX = { id: "private-x", test: (t) => t.includes("secret-leak-1"), why: "ruleX" };
        const ruleY = { id: "private-y", test: (t) => t.includes("secret-leak-2"), why: "ruleY" };

        // Gate A captures [ruleX] at construction.
        const gateA = createEgressGate({
            gate: gateEgressMessage,
            scrubCredentials,
            privateDenyRules: [ruleX],
        });
        // Baseline: the captured ruleX works — "secret-leak-1" is REJECTED.
        const aLeak1 = gateA(msgCarrying("secret-leak-1"));
        assert.equal(aLeak1.verdict, "rejected");
        assert.ok(
            aLeak1.reasons.some((r) => r.includes("private-x")),
            `gate A should reject via captured ruleX: ${JSON.stringify(aLeak1.reasons)}`,
        );

        // Construct a FRESH gate B with the UPDATED rules [ruleY].
        const gateB = createEgressGate({
            gate: gateEgressMessage,
            scrubCredentials,
            privateDenyRules: [ruleY],
        });
        // gate B consults the NEW rules: "secret-leak-1" now PASSES (ruleX is
        // gone), and "secret-leak-2" is REJECTED (ruleY is present). This proves
        // the construct-per-send pattern picks up rule changes.
        assert.equal(gateB(msgCarrying("secret-leak-1")).verdict, "passed");
        const bLeak2 = gateB(msgCarrying("secret-leak-2"));
        assert.equal(bLeak2.verdict, "rejected");
        assert.ok(
            bLeak2.reasons.some((r) => r.includes("private-y")),
            `gate B should reject via the new ruleY: ${JSON.stringify(bLeak2.reasons)}`,
        );

        // Honesty: gate A (the original) does NOT see ruleY — sending
        // "secret-leak-2" through gate A PASSES (it captured only ruleX). This
        // documents the capture-once behavior explicitly: an existing gate does
        // not consult rules added/updated after its construction.
        assert.equal(
            gateA(msgCarrying("secret-leak-2")).verdict,
            "passed",
            "gate A must NOT see ruleY (capture-once): secret-leak-2 should pass through the original gate",
        );
    });

    // ===== F1 FIX: repo-relative private-rule path resolution =====
    //
    // The bug: loadPrivateDenyRules passed a repo-relative path directly to
    // import(), which resolved it against the LOADER MODULE's directory
    // (.opencode/plugins/), NOT the repo root. A documented path like
    // .opencode/repo-configs/repo-mail-deny.private.js silently 404'd → [] →
    // private rules silently disabled. The fix (resolvePrivateDenyRulesModule)
    // resolves repo-relative paths against the repo root BEFORE import().
    //
    // Both tests below FAIL under the OLD behavior and PASS under the fix.

    test("F1 fix (unit): resolvePrivateDenyRulesModule resolves a repo-relative path against repoRoot (NOT the loader dir)", () => {
        const repoRoot = process.cwd();
        // A documented repo-relative private-rule path.
        const resolved = resolvePrivateDenyRulesModule(
            ".opencode/repo-configs/repo-mail-deny.private.js",
            repoRoot,
        );
        // Must be a file: URL ...
        assert.ok(resolved.startsWith("file:"), `must be a file: URL, got ${resolved}`);
        // ... pointing at the REPO ROOT, not the loader module's directory.
        const resolvedPath = fileURLToPath(resolved);
        assert.equal(
            resolvedPath,
            path.join(repoRoot, ".opencode", "repo-configs", "repo-mail-deny.private.js"),
            `resolved path must be <repoRoot>/.opencode/repo-configs/repo-mail-deny.private.js`,
        );
        // CRITICAL: under the OLD behavior the relative specifier resolved
        // against the loader module's directory. This assertion proves it does
        // NOT — the resolved path is at the repo root, not under the loader dir.
        const loaderDir = path.dirname(__filename);
        assert.ok(
            !resolvedPath.startsWith(loaderDir),
            `resolved path must NOT be under the loader module's directory (${loaderDir}); got ${resolvedPath}`,
        );
    });

    test("F1 fix (unit): absolute path and file: URL pass through unchanged; relative honors explicit repoRoot", () => {
        // absolute POSIX path → file: URL
        const abs = resolvePrivateDenyRulesModule("/srv/private/rules.js", "/irrelevant");
        assert.ok(abs.startsWith("file:"));
        assert.equal(fileURLToPath(abs), path.resolve("/srv/private/rules.js"));
        // file: URL → unchanged
        const url = "file:///etc/private/rules.js";
        assert.equal(resolvePrivateDenyRulesModule(url, "/irrelevant"), url);
        // relative + explicit repoRoot → resolved against repoRoot, not cwd
        const rel = resolvePrivateDenyRulesModule("a/b.js", "/explicit/root");
        assert.equal(fileURLToPath(rel), path.join("/explicit/root", "a", "b.js"));
        // non-string / empty → ""
        assert.equal(resolvePrivateDenyRulesModule(null, "/x"), "");
        assert.equal(resolvePrivateDenyRulesModule("", "/x"), "");
    });

    test("F1 fix (integration): loadPrivateDenyRules resolves a repo-relative path and loads private rules end-to-end", async () => {
        // Proves the FIX end-to-end: a repo-relative path resolves against the
        // repo root (process.cwd()), the fixture module loads, and the private
        // rule REJECTS a message carrying the private identifier. Under the OLD
        // behavior the relative path resolved against the loader dir → 404 → []
        // → private rules disabled → the message would PASS (the F1 silent
        // security failure).
        const fs = await import("node:fs");
        const fixtureDir = path.join(process.cwd(), "tmp", "repo-mail-f1-test");
        const fixtureFile = path.join(fixtureDir, "private-rules.js");
        const fixtureRelPath = "tmp/repo-mail-f1-test/private-rules.js"; // repo-relative
        const privateIdentifier = "unique-private-identifier-xyz-f1";
        fs.mkdirSync(fixtureDir, { recursive: true });
        fs.writeFileSync(
            fixtureFile,
            `export const REPO_MAIL_DENY_RULES = [{ id: "private-fixture", test: (t) => t.includes("${privateIdentifier}"), why: "F1 fixture" }];\n`,
            "utf8",
        );
        try {
            // No explicit repoRoot → defaults to process.cwd() (the repo root).
            // Under the OLD behavior this returned [] (404 against the loader dir).
            const rules = await loadPrivateDenyRules(fixtureRelPath);
            assert.ok(Array.isArray(rules) && rules.length === 1, `should load 1 rule, got: ${JSON.stringify(rules)}`);
            assert.equal(rules[0].id, "private-fixture");

            // End-to-end: the loaded private rule, wired into the gate, REJECTS
            // a message carrying the private identifier (which the GENERIC rules
            // cannot know about).
            const gateWithPrivate = createEgressGate({
                gate: gateEgressMessage,
                scrubCredentials,
                privateDenyRules: rules,
            });
            const msg = {
                message_id: "f1_int",
                kind: "report",
                sender: { channel_id: "ch_abc123def", channel_class: "report" },
                scrub: { result: "passed" },
                claims: [{ statement: `leaked ${privateIdentifier} here` }],
            };
            const res = gateWithPrivate(msg);
            assert.equal(res.verdict, "rejected");
            assert.ok(
                res.reasons.some((r) => r.includes("private-fixture")),
                `should name the private-fixture rule: ${JSON.stringify(res.reasons)}`,
            );
        } finally {
            fs.rmSync(fixtureDir, { recursive: true, force: true });
        }
    });

    // ===== silent-disable class: present-but-broken rules files must THROW =====
    //
    // The bug: the err.code===ERR_MODULE_NOT_FOUND catch CONFLATED "top-level
    // rules file absent" with "top-level file present but its dependency
    // missing" → both returned [] → private rules silently disabled. The fix
    // uses an EXISTENCE CHECK before import: [] is returned ONLY for a
    // no/invalid modulePath or a genuinely-absent top-level file (ENOENT); a
    // PRESENT file broken in any way (missing dep, syntax error, no valid
    // export) THROWS.
    //
    // These tests FAIL under the OLD behavior (returned []) and PASS under the
    // fix (throw / surface).

    test("silent-disable: present rules file importing a MISSING dependency → THROWS (no silent [])", async () => {
        // The top-level rules file EXISTS but imports a non-existent dependency.
        // OLD behavior: import() threw ERR_MODULE_NOT_FOUND → caught → [] (silent
        // disable). NEW behavior: existence check passes (file present) → import
        // throws → propagates (surfaces loudly).
        const fsMod = await import("node:fs");
        const fixtureDir = path.join(process.cwd(), "tmp", "repo-mail-a7-missing-dep");
        const fixtureFile = path.join(fixtureDir, "private-rules.js");
        const fixtureRelPath = "tmp/repo-mail-a7-missing-dep/private-rules.js";
        fsMod.mkdirSync(fixtureDir, { recursive: true });
        fsMod.writeFileSync(
            fixtureFile,
            `import "./nonexistent-dep-xyz-a7.js";\nexport const REPO_MAIL_DENY_RULES = [];\n`,
            "utf8",
        );
        try {
            await assert.rejects(
                () => loadPrivateDenyRules(fixtureRelPath),
                // The missing-dependency error is code ERR_MODULE_NOT_FOUND with a
                // "Cannot find module ... imported from ..." message. The core
                // assertion is that it REJECTS (does not silently resolve to []).
                (err) =>
                    !!err &&
                    (err.code === "ERR_MODULE_NOT_FOUND" ||
                        /Cannot find module/.test(err.message || "")),
                "a present rules file with a missing dependency must THROW, not silently return []",
            );
        } finally {
            fsMod.rmSync(fixtureDir, { recursive: true, force: true });
        }
    });

    test("silent-disable: present rules file with NO REPO_MAIL_DENY_RULES export → THROWS (no silent [])", async () => {
        // The top-level rules file EXISTS and imports cleanly but does not export
        // the expected shape. OLD behavior: `Array.isArray(rules) ? rules : []`
        // silently returned []. NEW behavior: throws a descriptive error.
        const fsMod = await import("node:fs");
        const fixtureDir = path.join(process.cwd(), "tmp", "repo-mail-a7-no-export");
        const fixtureFile = path.join(fixtureDir, "private-rules.js");
        const fixtureRelPath = "tmp/repo-mail-a7-no-export/private-rules.js";
        fsMod.mkdirSync(fixtureDir, { recursive: true });
        fsMod.writeFileSync(fixtureFile, `export const SOMETHING_ELSE = 42;\n`, "utf8");
        try {
            await assert.rejects(
                () => loadPrivateDenyRules(fixtureRelPath),
                /REPO_MAIL_DENY_RULES is undefined|must export the expected shape/,
                "a present rules file without the expected export must THROW, not silently return []",
            );
        } finally {
            fsMod.rmSync(fixtureDir, { recursive: true, force: true });
        }
    });

    test("silent-disable: present rules file with WRONG-TYPE REPO_MAIL_DENY_RULES export → THROWS (no silent [])", async () => {
        // The top-level rules file exports REPO_MAIL_DENY_RULES but as a string,
        // not an array. OLD behavior: silently []. NEW behavior: throws.
        const fsMod = await import("node:fs");
        const fixtureDir = path.join(process.cwd(), "tmp", "repo-mail-a7-wrong-type");
        const fixtureFile = path.join(fixtureDir, "private-rules.js");
        const fixtureRelPath = "tmp/repo-mail-a7-wrong-type/private-rules.js";
        fsMod.mkdirSync(fixtureDir, { recursive: true });
        fsMod.writeFileSync(
            fixtureFile,
            `export const REPO_MAIL_DENY_RULES = "not-an-array";\n`,
            "utf8",
        );
        try {
            await assert.rejects(
                () => loadPrivateDenyRules(fixtureRelPath),
                /REPO_MAIL_DENY_RULES is string|must export the expected shape/,
                "a present rules file with a wrong-type export must THROW, not silently return []",
            );
        } finally {
            fsMod.rmSync(fixtureDir, { recursive: true, force: true });
        }
    });

    test("silent-disable: present rules file with a SYNTAX ERROR → THROWS (no silent [])", async () => {
        // The top-level rules file EXISTS but has a syntax error. OLD behavior:
        // caught → fell through to throw (the old code did rethrow non-MODULE_NOT_FOUND,
        // but this confirms the new path also surfaces it). NEW behavior: existence
        // check passes → import throws SyntaxError → propagates.
        const fsMod = await import("node:fs");
        const fixtureDir = path.join(process.cwd(), "tmp", "repo-mail-a7-syntax");
        const fixtureFile = path.join(fixtureDir, "private-rules.js");
        const fixtureRelPath = "tmp/repo-mail-a7-syntax/private-rules.js";
        fsMod.mkdirSync(fixtureDir, { recursive: true });
        fsMod.writeFileSync(fixtureFile, `export const REPO_MAIL_DENY_RULES = [ { ;\n`, "utf8");
        try {
            await assert.rejects(
                () => loadPrivateDenyRules(fixtureRelPath),
                (err) => err instanceof SyntaxError || /SyntaxError|Unexpected/.test(err && err.message ? err.message : String(err)),
                "a present rules file with a syntax error must THROW, not silently return []",
            );
        } finally {
            fsMod.rmSync(fixtureDir, { recursive: true, force: true });
        }
    });
}
