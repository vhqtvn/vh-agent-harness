// Regression tests for the release-mode stdout-truncation defect in
// check-defer-triggers.mjs.
//
// CRUX (v0.22.0 release CI red): emitReleaseResult / emitReleasePrepResult write
// the full JSON envelope as a single process.stdout.write() and then call
// process.exit(). When stdout is a PIPE whose reader drains slowly enough to let
// the kernel pipe buffer fill (the release CI step is
// `node ... | tee tmp/release-defer-evaluator.json` — `tee` writes every read to
// a file AND its own stdout, so it is a "slow" reader), and the payload exceeds
// the ~64KiB pipe buffer, write() returns false at the buffer boundary and
// process.exit() fires BEFORE the remainder drains. The captured file is then
// truncated at exactly 65536 bytes and the downstream JSON.parse throws
// "Unterminated string in JSON at position ~65438".
//
// IMPORTANT — why this test spawns `node ... | tee` (a shell pipe) and NOT a
// plain spawnSync pipe capture: a plain spawnSync poll-drains the child's stdout
// pipe so aggressively that libuv pushes the whole payload through in a single
// .write() call and the truncation never appears. The defect only surfaces with
// a slow reader. `tee` is the exact CI reader and reproduces the 65536-byte
// truncation deterministically, so we mirror it here.
//
// Run:  vh-agent-harness exec node --test tests/scripts/check-defer-triggers.test.js
//       (or: node --test tests/scripts/check-defer-triggers.test.js)

import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, writeFileSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, dirname } from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath, pathToFileURL } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const SCRIPT = join(__dirname, "..", "..", "templates", "core", ".opencode", "scripts", "check-defer-triggers.mjs");
const SCRIPT_URL = pathToFileURL(SCRIPT).href;

// Linux default pipe buffer size. The defect truncated the piped capture at
// EXACTLY this many bytes (write() returned false here and the immediate
// process.exit() discarded the undrained remainder).
const PIPE_BUFFER_BYTES = 65536;

// Run `bodySrc` in a child node process whose stdout is piped through `tee` to a
// file — the EXACT CI capture shape (`node ... | tee tmp/release-defer-evaluator.json`).
// `tee` is a slow-enough pipe reader to expose the truncation; a fast poll-read
// (plain spawnSync pipe capture) hides it. Returns the captured file bytes + the
// tee pipeline's exit status. The CLI entry guard means importing the module
// does NOT auto-run main(), so we can invoke the emitters directly.
function emitViaTee(bodySrc) {
    const dir = mkdtempSync(join(tmpdir(), "cdt-emit-"));
    const helper = join(dir, "emit.mjs");
    const captured = join(dir, "captured.json");
    writeFileSync(
        helper,
        `import { emitReleaseResult, emitReleasePrepResult } from ${JSON.stringify(SCRIPT_URL)};\n${bodySrc}\n`,
    );
    // Use bash (not /bin/sh=dash) so `set -o pipefail` is honored — the pipeline
    // must return the NODE child's exit code, not tee's. CI runs under
    // `bash -eo pipefail`; mirroring that here makes the exit-code assertions
    // faithful. (dash lacks pipefail and aborts the whole pipeline on `set -o`.)
    const res = spawnSync(
        "bash",
        ["-c", `set -o pipefail; node ${JSON.stringify(helper)} | tee ${JSON.stringify(captured)} > /dev/null`],
        { encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] },
    );
    return { captured: readFileSync(captured, "utf8"), status: res.status, stderr: res.stderr };
}

// A release-shaped envelope inflated past the 64KiB pipe buffer.
function bigReleaseEnvelope(recordCount) {
    const filler = "release_relevance=no disposition=disclose rationale bytes ".repeat(8);
    const records = [];
    for (let i = 0; i < recordCount; i++) {
        records.push({
            defer_id: `defer-stress-${i}`,
            release_relevance: "no",
            disposition: "disclose",
            metadata_state: "valid",
            rationale: filler,
        });
    }
    return {
        mode: "release",
        manifest_authority: true,
        classification: "clear",
        records,
        disclosures: [],
        accepted_overrides: [],
        refusals: [],
        blocking_ids: [],
        disclose_ids: [],
    };
}

test("emitReleaseResult: >64KiB payload round-trips through a tee-captured pipe (v0.22.0 regression)", () => {
    const payload = bigReleaseEnvelope(220);
    const expectedBytes = Buffer.byteLength(JSON.stringify(payload, null, 2) + "\n", "utf8");
    assert.ok(
        expectedBytes > PIPE_BUFFER_BYTES,
        `payload (${expectedBytes}B) must exceed the pipe buffer (${PIPE_BUFFER_BYTES}B) so the test actually exercises the truncation path`,
    );

    const { captured, stderr, status } = emitViaTee(`emitReleaseResult(${JSON.stringify(payload)});`);

    assert.equal(status, 0, `clear classification must exit 0; stderr: ${stderr}`);
    // Strongest form: the tee-captured file must be byte-equal to the FULL
    // serialized payload. Pre-fix it was exactly PIPE_BUFFER_BYTES (65536) and
    // the next assertion threw "Unterminated string in JSON at position ~65438".
    assert.equal(
        Buffer.byteLength(captured, "utf8"),
        expectedBytes,
        `tee-captured output must carry the full payload (${expectedBytes}B), not be truncated at the ${PIPE_BUFFER_BYTES}B pipe buffer`,
    );
    let parsed;
    assert.doesNotThrow(
        () => { parsed = JSON.parse(captured); },
        "tee-captured output must be complete, parseable JSON (pre-fix: 'Unterminated string in JSON at position ~65438')",
    );
    assert.equal(parsed.classification, "clear");
    assert.equal(parsed.records.length, 220);
});

test("emitReleaseResult: small payload still emits cleanly (write-callback must not regress the common case)", () => {
    const payload = { mode: "release", manifest_authority: true, classification: "clear", records: [] };
    const { captured, status } = emitViaTee(`emitReleaseResult(${JSON.stringify(payload)});`);
    assert.equal(status, 0);
    assert.deepEqual(JSON.parse(captured), payload);
});

test("emitReleaseResult: blocker classification exits 1 even when payload exceeds the pipe buffer", () => {
    const payload = bigReleaseEnvelope(220);
    payload.classification = "blocker";
    const { captured, status } = emitViaTee(`emitReleaseResult(${JSON.stringify(payload)});`);
    assert.equal(status, 1, "blocker must still surface its exit code after the full drain");
    assert.doesNotThrow(() => JSON.parse(captured), "blocker payload must still be complete + parseable through the tee pipe");
});

test("emitReleasePrepResult: >64KiB payload round-trips through a tee-captured pipe (same defect class)", () => {
    // emitReleasePrepResult builds its payload from (since, missing, draftStubRecords, extra);
    // inflate `missing` + `draftStubRecords` past the pipe buffer. With missing.length > 0
    // the classification is "blocker" → exit 1, and the payload must still drain fully.
    const filler = "x".repeat(2048);
    const missing = [];
    for (let i = 0; i < 64; i++) {
        missing.push({ defer_id: `defer-prep-${i}`, file: `defer-prep-${i}.json`, status: "ready", fired_targets: [filler] });
    }
    const bodySrc = `emitReleasePrepResult("HEAD~1", ${JSON.stringify(missing)}, ${JSON.stringify(missing)}, { firedCount: ${missing.length}, fog: [], cold: [] });`;

    const { captured, status } = emitViaTee(bodySrc);
    assert.ok(
        Buffer.byteLength(captured, "utf8") > PIPE_BUFFER_BYTES,
        `prep tee-captured output (${Buffer.byteLength(captured)}B) must exceed the pipe buffer — otherwise this test no longer exercises the path`,
    );
    assert.equal(status, 1, "missing.length > 0 → blocker → exit 1");
    assert.doesNotThrow(() => JSON.parse(captured), "prep tee-captured output must be complete + parseable");
});
