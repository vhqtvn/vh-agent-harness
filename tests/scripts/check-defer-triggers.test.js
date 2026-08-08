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
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, dirname } from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath, pathToFileURL } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const SCRIPT = join(__dirname, "..", "..", "templates", "core", ".opencode", "scripts", "check-defer-triggers.mjs");
const SCRIPT_URL = pathToFileURL(SCRIPT).href;

// Promoter-mode helpers under test (6-state classification + false-READY
// refusal). Top-level dynamic import keeps the same SCRIPT source-of-truth
// pointer the release-mode tests above use, so a stale generated .opencode/
// copy can never shadow the template under test.
const { evaluateCandidate, classifyCardState } = await import(SCRIPT_URL);

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

// ---------------------------------------------------------------------------
// PROMOTER MODE — 6-state classification + false-READY refusal.
//
// The promoter path used to collapse every non-met card into a single [hold]
// flag, so a promoter could not tell a genuine future-watch (valid-waiting)
// from noise (no trigger / unsupported) or a broken compound
// (malformed-compound) without reading every detail parenthetical. These
// tests pin the six distinct `state` values and the false-READY refusal:
// a compound any()/all() with an unparseable member must NOT surface READY
// even when its parseable members are met.
//
// evaluateCandidate takes `changedPaths` as an injected Set, so these cases
// are deterministic and do not touch git.
// ---------------------------------------------------------------------------

// Build a minimal task-card body shaped like /write-task output: only the
// fields the evaluator reads (task_id + owner_notes), plus an optional
// lifecycle status (defaults to omitting the field, matching the pre-status
// cards the rest of this suite uses).
function promoterCard(id, notes, status) {
    const card = { task_id: id, owner_notes: notes };
    if (status !== undefined) card.status = status;
    return card;
}

test("promoter classifyCardState: no trigger line → no-machine-trigger", () => {
    const r = evaluateCandidate("tasks/plain.json", promoterCard("plain", [
        "source:review-defer",
        "studied:2026-04-30",
    ]), "v0.1.0", new Set(["fileA.go"]));
    assert.equal(r.state, "no-machine-trigger");
    assert.equal(r.met, false);
    assert.equal(r.mode, "none");
    assert.deepEqual(r.details, []);
});

test("promoter classifyCardState: recognized path_touched IN diff → valid-fired (READY)", () => {
    const r = evaluateCandidate("tasks/fired.json", promoterCard("fired", [
        "source:review-defer",
        "trigger:path_touched(fileA.go)",
        "studied:2026-04-30",
    ]), "v0.1.0", new Set(["fileA.go"]));
    assert.equal(r.state, "valid-fired");
    assert.equal(r.met, true);
    assert.equal(r.details.length, 1);
    assert.equal(r.details[0].met, true);
    assert.equal(r.details[0].note, "touched");
    assert.equal(r.details[0].parseState, "recognized");
});

test("promoter classifyCardState: recognized path_touched NOT in diff → valid-waiting", () => {
    const r = evaluateCandidate("tasks/waiting.json", promoterCard("waiting", [
        "trigger:path_touched(fileA.go)",
    ]), "v0.1.0", new Set(["other.go"]));
    assert.equal(r.state, "valid-waiting");
    assert.equal(r.met, false);
    assert.equal(r.details[0].note, "not-touched-since-ref");
});

test("promoter classifyCardState: unrecognized predicate → unsupported", () => {
    const r = evaluateCandidate("tasks/unk.json", promoterCard("unk", [
        "trigger:foo_bar(some-arg)",
    ]), "v0.1.0", new Set(["fileA.go"]));
    assert.equal(r.state, "unsupported");
    assert.equal(r.met, false);
    assert.equal(r.details[0].note, "unknown-predicate");
    assert.equal(r.details[0].parseState, "unsupported");
});

test("promoter classifyCardState: single malformed-arg predicate → unsupported (detail keeps malformed-predicate)", () => {
    // A single `||`-joined trigger is a recognized predicate with a bad arg.
    // It is not a compound, so the card-level state is `unsupported`; the
    // per-member detail still surfaces `malformed-predicate` (pinned by the Go
    // promoter-mode test TestCheckDefer_PromoterMode_MalformedOrJoinTrigger).
    const r = evaluateCandidate("tasks/malformed.json", promoterCard("malformed", [
        "trigger:path_touched(fileA.go)||path_touched(fileB.go)",
    ]), "v0.1.0", new Set(["fileA.go"]));
    assert.equal(r.state, "unsupported");
    assert.equal(r.met, false);
    assert.equal(r.details[0].note, "malformed-predicate");
    assert.equal(r.details[0].parseState, "malformed");
});

test("promoter classifyCardState: bare single `|` pseudo-OR arg → unsupported (malformed-predicate)", () => {
    // A bare-single `|` join (the pseudo-OR `path_touched(a|b|c)`) is a
    // recognized predicate whose greedy-captured arg still carries the
    // predicate-structural `|`. Without the isMalformedArg guard extension to
    // a bare `|`, the greedy `(.+)` in the regex would swallow `fileA.go|fileB.go`
    // as ONE literal path operand, the card would silently park as
    // valid-waiting (not-touched-since-ref), and the malformed grammar would
    // never surface. The guard now rejects the bare `|` so the card is visibly
    // flagged malformed-predicate, forcing conversion to a legal `any(...)`.
    const r = evaluateCandidate("tasks/bare-pipe.json", promoterCard("bare-pipe", [
        "trigger:path_touched(fileA.go|fileB.go|fileC.go)",
    ]), "v0.1.0", new Set(["fileA.go"]));
    assert.equal(r.state, "unsupported");
    assert.equal(r.met, false);
    assert.equal(r.details[0].note, "malformed-predicate");
    assert.equal(r.details[0].parseState, "malformed");
});

test("promoter classifyCardState: glob path_touched → cold-glob", () => {
    // A glob operand can never precisely match (exact Set.has lookup), so it
    // is surfaced cold-glob rather than silently parked as valid-waiting.
    const rGlob = evaluateCandidate("tasks/glob.json", promoterCard("glob", [
        "trigger:path_touched(src/*)",
    ]), "v0.1.0", new Set(["src/auth/login.go"]));
    assert.equal(rGlob.state, "cold-glob");
    assert.equal(rGlob.met, false);

    // A directory operand (trailing slash) is cold-glob too.
    const rDir = evaluateCandidate("tasks/dir.json", promoterCard("dir", [
        "trigger:path_touched(src/)",
    ]), "v0.1.0", new Set(["src/auth/login.go"]));
    assert.equal(rDir.state, "cold-glob");
    assert.equal(rDir.met, false);
});

test("promoter false-READY refusal: any() compound with an unparseable member is NOT READY even when a member fires", () => {
    // DEFECT: under the old reduction `met = details.some(d => d.met)`, this
    // card reported READY because path_touched(fileA.go) is met and the
    // garbage_pred(x) member was silently dropped (parsePredicate → null →
    // met:false, ignored by .some()). The promoter would then apply the DoR to
    // a card whose trigger grammar is broken. The fix refuses READY: the card
    // is malformed-compound, met=false.
    const r = evaluateCandidate("tasks/false-ready.json", promoterCard("false-ready", [
        "trigger:any(path_touched(fileA.go), garbage_pred(x))",
    ]), "v0.1.0", new Set(["fileA.go"]));
    assert.equal(r.state, "malformed-compound");
    assert.equal(r.met, false, "malformed-compound must NOT surface READY (false-READY refusal)");
    // The firing member is still reflected in its detail (the compound IS
    // met-able); only the card-level READY is refused.
    const fired = r.details.find((d) => d.trigger === "path_touched(fileA.go)");
    const garbage = r.details.find((d) => d.trigger === "garbage_pred(x)");
    assert.equal(fired.met, true);
    assert.equal(fired.parseState, "recognized");
    assert.equal(garbage.met, false);
    assert.equal(garbage.parseState, "unsupported");
});

test("promoter false-READY refusal: multi-line all() compound with an unparseable member → malformed-compound", () => {
    // The AND form (multiple `trigger:` lines, mode "all") is also a compound.
    // A malformed member here does not produce false-READY (AND already
    // requires all members), but it MUST still be surfaced malformed-compound
    // (not silently valid-waiting) so the broken grammar is visible.
    const r = evaluateCandidate("tasks/all-malformed.json", promoterCard("all-malformed", [
        "trigger:path_touched(fileA.go)",
        "trigger:foo_bar(x)",
    ]), "v0.1.0", new Set(["fileA.go"]));
    assert.equal(r.state, "malformed-compound");
    assert.equal(r.met, false);
    assert.equal(r.mode, "all");
});

test("promoter regression: well-formed any() over a firing member stays valid-fired (no over-rejection)", () => {
    // The false-READY fix must not over-reject a clean compound. Two
    // recognized path_touched members, one firing → valid-fired → READY.
    // (Pinned end-to-end by the Go test TestCheckDefer_PromoterMode_MalformedOrJoinTrigger
    // line `[READY] defer-any`; this is the unit-level witness.)
    const r = evaluateCandidate("tasks/any-clean.json", promoterCard("any-clean", [
        "trigger:any(path_touched(fileA.go),path_touched(fileB.go))",
    ]), "v0.1.0", new Set(["fileA.go"]));
    assert.equal(r.state, "valid-fired");
    assert.equal(r.met, true);
    assert.equal(r.mode, "any");
    const b = r.details.find((d) => d.trigger === "path_touched(fileB.go)");
    assert.equal(b.met, false);
    assert.equal(b.note, "not-touched-since-ref");
});

test("promoter regression: well-formed any() with no member firing → valid-waiting (not cold-glob)", () => {
    const r = evaluateCandidate("tasks/any-waiting.json", promoterCard("any-waiting", [
        "trigger:any(path_touched(fileA.go),path_touched(fileB.go))",
    ]), "v0.1.0", new Set(["other.go"]));
    assert.equal(r.state, "valid-waiting");
    assert.equal(r.met, false);
});

test("promoter classifyCardState: precise + glob mix that does not fire → valid-waiting (precise member governs)", () => {
    // A precise path_touched that is waiting + an incidental glob: the precise
    // target is the real watch, so this is valid-waiting, NOT cold-glob.
    const r = evaluateCandidate("tasks/mix.json", promoterCard("mix", [
        "trigger:any(path_touched(fileA.go), path_touched(docs/*))",
    ]), "v0.1.0", new Set(["other.go"]));
    assert.equal(r.state, "valid-waiting");
    assert.equal(r.met, false);
});

test("promoter classifyCardState: glob path_touched + recognized non-path (after_tag absent) → valid-waiting (non-path arm governs, NOT cold-glob)", () => {
    // defer-032 regression. A mixed compound
    // `any(path_touched(docs/*), after_tag(<absent-tag>))`: the path_touched arm
    // is a glob (can never precisely match via exact Set.has), and the after_tag
    // arm is recognized but unmet because the tag does not exist in the repo
    // (after_tag uses live `git rev-parse --verify refs/tags/<tag>`). The
    // after_tag arm is a legitimate future-firing condition, so the card is NOT
    // cold-glob — it stays valid-waiting. Pre-fix the cold-glob branch fired
    // purely because no precise path_touched member existed, silently collapsing
    // the after_tag future-watch into dead-on-arrival.
    const r = evaluateCandidate("tasks/glob-aftertag.json", promoterCard("glob-aftertag", [
        "trigger:any(path_touched(docs/*), after_tag(defer-032-absent-9f3c2a1b))",
    ]), "v0.1.0", new Set(["docs/index.md"]));
    assert.equal(r.state, "valid-waiting", "glob + unmet after_tag must NOT be cold-glob");
    assert.equal(r.met, false);
    // The glob arm is recognized, unmet (no file is literally named docs/*),
    // and flagged path_touched.
    const glob = r.details.find((d) => d.trigger === "path_touched(docs/*)");
    assert.equal(glob.met, false);
    assert.equal(glob.parseState, "recognized");
    assert.equal(glob.kind, "path_touched");
    // The after_tag arm is recognized, unmet (tag absent), note tag-missing.
    const aftertag = r.details.find((d) => d.trigger === "after_tag(defer-032-absent-9f3c2a1b)");
    assert.equal(aftertag.met, false);
    assert.equal(aftertag.parseState, "recognized");
    assert.equal(aftertag.kind, "after_tag");
    assert.equal(aftertag.note, "tag-missing");
});

test("promoter classifyCardState unit: empty items → no-machine-trigger regardless of details", () => {
    assert.equal(classifyCardState([], "all", []), "no-machine-trigger");
    assert.equal(classifyCardState([], "any", []), "no-machine-trigger");
});

// ---------------------------------------------------------------------------
// LIFECYCLE DISPOSITION — completed/cancelled re-fire must NOT pollute READY.
//
// DEFECT: classifyCardState derived each card's state purely from trigger
// predicates and NEVER consulted the card's lifecycle `status`, so a
// `completed`/`cancelled` card whose watched path was re-touched (by its own
// fix) re-fired forever as actionable READY, polluting the promotable set
// (9/14 "READY" in a post-phase2 run were completed re-fires).
//
// The fix splits the predicate truth from the promotable-READY REPORTING:
//   - `state` (six-state predicate logic in classifyCardState) is UNCHANGED —
//     a completed card whose trigger fires is still `valid-fired` (the re-fire
//     signal is preserved).
//   - `met` (predicate-READY) is UNCHANGED — still true for valid-fired.
//   - `lifecycle` (NEW) = "open" | "disposed" (completed/cancelled → disposed).
//   - `actionable` (NEW) = met && lifecycle open — the promotable-READY signal
//     the promoter's actionable count uses. A disposed re-fire is actionable=false.
//
// evaluateCandidate takes `changedPaths` as an injected Set, so these cases are
// deterministic and do not touch git.
// ---------------------------------------------------------------------------

test("lifecycle: completed card with a FIRED trigger is valid-fired (predicate unchanged) but NOT actionable (re-fire)", () => {
    // The watched path IS in the diff, so the predicate fires (valid-fired).
    const r = evaluateCandidate("tasks/done.json", promoterCard("done", [
        "source:review-defer",
        "trigger:path_touched(fileA.go)",
        "studied:2026-04-30",
    ], "completed"), "v0.1.0", new Set(["fileA.go"]));
    // Predicate truth preserved (the re-fire regression signal is NOT hidden).
    assert.equal(r.state, "valid-fired", "predicate state must stay valid-fired (evaluation unchanged)");
    assert.equal(r.met, true, "predicate-READY (met) must stay true so the re-fire is still visible");
    // But it is NOT actionable READY — it is already disposed.
    assert.equal(r.lifecycle, "disposed");
    assert.equal(r.actionable, false, "a completed card must NOT be actionable READY even when its trigger fires");
});

test("lifecycle: cancelled card with a FIRED trigger is NOT actionable (re-fire)", () => {
    const r = evaluateCandidate("tasks/cancelled.json", promoterCard("cancelled", [
        "trigger:path_touched(fileA.go)",
    ], "cancelled"), "v0.1.0", new Set(["fileA.go"]));
    assert.equal(r.state, "valid-fired");
    assert.equal(r.met, true);
    assert.equal(r.lifecycle, "disposed");
    assert.equal(r.actionable, false);
});

test("lifecycle: staged card with a FIRED trigger is NOT actionable (re-fire) — closed for recurrence like completed/cancelled", () => {
    // Staged is closed for promotion/recurrence: it mirrors PREP_CLOSED_STATUSES
    // (release-prep) and the Go-side closedStatuses (claim.go CardIsClosed/
    // StatusIsClosed + the release-gate closed set {completed, cancelled,
    // staged}). A staged card (correction queued for the next release) whose
    // trigger fires is a regression signal, NOT fresh actionable READY. Pre-fix
    // the promoter disposed set held only {completed, cancelled} and missed
    // staged → a fired staged card rendered [READY].
    const r = evaluateCandidate("tasks/staged.json", promoterCard("staged", [
        "trigger:path_touched(fileA.go)",
    ], "staged"), "v0.1.0", new Set(["fileA.go"]));
    assert.equal(r.state, "valid-fired", "predicate state must stay valid-fired (evaluation unchanged)");
    assert.equal(r.met, true, "predicate-READY (met) must stay true so the re-fire is still visible");
    assert.equal(r.lifecycle, "disposed", "staged must be lifecycle disposed (closed for recurrence)");
    assert.equal(r.actionable, false, "a staged card must NOT be actionable READY even when its trigger fires");
});

test("lifecycle: completed card status is case-insensitive and trimmed (Completed / Cancelled )", () => {
    const rUpper = evaluateCandidate("tasks/done-u.json", promoterCard("done-u", [
        "trigger:path_touched(fileA.go)",
    ], "Completed"), "v0.1.0", new Set(["fileA.go"]));
    assert.equal(rUpper.lifecycle, "disposed");
    assert.equal(rUpper.actionable, false);

    const rPadded = evaluateCandidate("tasks/cancelled-p.json", promoterCard("cancelled-p", [
        "trigger:path_touched(fileA.go)",
    ], " cancelled "), "v0.1.0", new Set(["fileA.go"]));
    assert.equal(rPadded.lifecycle, "disposed");
    assert.equal(rPadded.actionable, false);
});

test("lifecycle: draft/ready/working cards with a FIRED trigger stay actionable READY (no over-rejection)", () => {
    // The fix must not over-reject a genuinely-open card. draft/ready/working
    // are all promotable lifecycle states and must remain actionable READY when
    // their trigger fires.
    for (const status of ["draft", "ready", "working", "reported", "blocked"]) {
        const r = evaluateCandidate(`tasks/${status}.json`, promoterCard(status, [
            "trigger:path_touched(fileA.go)",
        ], status), "v0.1.0", new Set(["fileA.go"]));
        assert.equal(r.state, "valid-fired", `${status} must stay valid-fired`);
        assert.equal(r.met, true, `${status} must stay predicate-READY`);
        assert.equal(r.lifecycle, "open", `${status} must be lifecycle open`);
        assert.equal(r.actionable, true, `${status} must be actionable READY (not over-rejected)`);
    }
});

test("lifecycle: completed card with a NOT-fired trigger is valid-waiting + disposed (not a re-fire)", () => {
    // A disposed card whose trigger has NOT fired is just disposed+waiting — it
    // is not a re-fire (no watched path re-touched). It is still not actionable.
    const r = evaluateCandidate("tasks/done-waiting.json", promoterCard("done-waiting", [
        "trigger:path_touched(fileA.go)",
    ], "completed"), "v0.1.0", new Set(["other.go"]));
    assert.equal(r.state, "valid-waiting");
    assert.equal(r.met, false);
    assert.equal(r.lifecycle, "disposed");
    assert.equal(r.actionable, false);
});

test("lifecycle: no-status card defaults to lifecycle open (backward compatible)", () => {
    // A card that carries NO status field (legacy / pre-status shape) must
    // behave exactly as before: lifecycle open, actionable when fired.
    const r = evaluateCandidate("tasks/nostatus.json", promoterCard("nostatus", [
        "trigger:path_touched(fileA.go)",
    ]), "v0.1.0", new Set(["fileA.go"]));
    assert.equal(r.state, "valid-fired");
    assert.equal(r.lifecycle, "open");
    assert.equal(r.actionable, true);
});

test("lifecycle: no-trigger completed card carries lifecycle + actionable=false on the early return", () => {
    // The no-trigger early-return path must also surface lifecycle/actionable
    // so the report shape is consistent across all states.
    const r = evaluateCandidate("tasks/done-notrigger.json", promoterCard("done-notrigger", [
        "source:review-defer",
        "studied:2026-04-30",
    ], "completed"), "v0.1.0", new Set(["fileA.go"]));
    assert.equal(r.state, "no-machine-trigger");
    assert.equal(r.lifecycle, "disposed");
    assert.equal(r.actionable, false);
});

// ---------------------------------------------------------------------------
// END-TO-END promoter subprocess: a completed card re-fires as [RE-FIRE], is
// absent from [READY], and the actionable count excludes it. Mirrors the Go
// setupReleaseEvalRepo pattern (hermetic scratch git repo) so the crux is
// observed at the OUTCOME layer (rendered stdout), not just the report object.
// ---------------------------------------------------------------------------

// Build a hermetic scratch git repo with a prior tag and a post-tag change to
// fileA.go (so path_touched(fileA.go) fires), then run the TEMPLATE script in
// promoter mode against a tasks dir holding one completed card, one staged
// card, and one open draft card. The completed + staged cards are both
// disposition-satisfied (closed for recurrence) and must re-fire as [RE-FIRE],
// while the open draft card fires as [READY].
function runPromoterWithDisposedRefireCards() {
    const dir = mkdtempSync(join(tmpdir(), "cdt-promoter-"));
    // Copy the template script so __dirname-based repoRoot() resolves to dir.
    const scriptCopy = join(dir, ".opencode", "scripts", "check-defer-triggers.mjs");
    mkdirSync(join(dir, ".opencode", "scripts"), { recursive: true });
    writeFileSync(scriptCopy, readFileSync(SCRIPT, "utf8"));

    const tasksDir = join(dir, "tasks");
    mkdirSync(tasksDir, { recursive: true });
    // A completed card whose watched path (fileA.go) re-fires in the diff.
    writeFileSync(join(tasksDir, "done-refire.json"), JSON.stringify({
        schema_version: 1,
        task_id: "done-refire",
        status: "completed",
        owner_notes: ["source:review-defer", "trigger:path_touched(fileA.go)", "studied:2026-04-30"],
    }));
    // A STAGED card whose watched path (fileA.go) re-fires. Staged is closed
    // for recurrence (mirrors PREP_CLOSED_STATUSES + the Go-side closedStatuses),
    // so it must re-fire as [RE-FIRE] too — NOT [READY]. Pre-fix the promoter
    // disposed set held only {completed, cancelled} and a fired staged card
    // rendered [READY], conflating trigger-fired with promotion-work-remains.
    writeFileSync(join(tasksDir, "staged-refire.json"), JSON.stringify({
        schema_version: 1,
        task_id: "staged-refire",
        status: "staged",
        owner_notes: ["source:review-defer", "trigger:path_touched(fileA.go)", "studied:2026-08-08"],
    }));
    // An open (draft) card that also fires, to prove READY still works alongside.
    writeFileSync(join(tasksDir, "draft-fire.json"), JSON.stringify({
        schema_version: 1,
        task_id: "draft-fire",
        status: "draft",
        owner_notes: ["source:review-defer", "trigger:path_touched(fileA.go)", "studied:2026-04-30"],
    }));

    const git = (args) => spawnSync("git", ["-C", dir, ...args], { encoding: "utf8" });
    git(["init", "-q"]);
    git(["config", "user.email", "t@t"]);
    git(["config", "user.name", "t"]);
    git(["config", "commit.gpgsign", "false"]);
    writeFileSync(join(dir, "fileA.go"), "package main\n");
    git(["add", "-A"]);
    git(["commit", "-q", "-m", "initial"]);
    git(["tag", "v0.1.0"]);
    writeFileSync(join(dir, "fileA.go"), "package main\n// changed in arc\n");
    git(["add", "-A"]);
    git(["commit", "-q", "-m", "changes for release"]);

    const res = spawnSync("node", [scriptCopy, "--tasks", tasksDir, "--since", "v0.1.0"], {
        encoding: "utf8",
        cwd: dir,
    });
    if (res.error) throw res.error;
    return { stdout: res.stdout, status: res.status, stderr: res.stderr };
}

test("promoter e2e: completed + staged cards re-fire as [RE-FIRE] (NOT [READY]) and are excluded from the actionable count", () => {
    const { stdout, status, stderr } = runPromoterWithDisposedRefireCards();
    assert.equal(status, 0, `promoter must exit 0 (never blocking); stderr: ${stderr}`);
    // The completed card is surfaced under the distinct RE-FIRE category.
    assert.ok(stdout.includes("[RE-FIRE] done-refire"), `completed re-fire must render [RE-FIRE]; got:\n${stdout}`);
    // The staged card is ALSO surfaced as [RE-FIRE] (closed for recurrence,
    // mirroring PREP_CLOSED_STATUSES + the Go-side closedStatuses). Pre-fix it
    // rendered [READY] because the promoter disposed set held only
    // {completed, cancelled} and missed staged.
    assert.ok(stdout.includes("[RE-FIRE] staged-refire"), `staged re-fire must render [RE-FIRE]; got:\n${stdout}`);
    // And neither must appear under [READY] (the defect: staged used to).
    assert.ok(!stdout.includes("[READY] done-refire"), `completed card must NOT render [READY]; got:\n${stdout}`);
    assert.ok(!stdout.includes("[READY] staged-refire"), `staged card must NOT render [READY] (defect regressed); got:\n${stdout}`);
    // The open draft card still renders [READY] alongside (no over-rejection).
    assert.ok(stdout.includes("[READY] draft-fire"), `open draft card must still render [READY]; got:\n${stdout}`);
    // The actionable count excludes BOTH disposed re-fires: 1 actionable READY
    // (draft-fire) out of 3 candidates. staged does not increase the actionable
    // count.
    assert.ok(stdout.includes("1/3 candidate(s) are actionable READY"), `actionable count must exclude both disposed re-fires; got:\n${stdout}`);
    // The separate disposed re-fire summary line surfaces the regression signal,
    // and staged DOES increase the re-fire count: 2 (done-refire + staged-refire).
    assert.ok(stdout.includes("Disposed re-fires"), `the re-fire summary line must preserve the regression signal; got:\n${stdout}`);
    assert.ok(stdout.includes(": 2\n"), `re-fire count must be 2 (completed + staged); got:\n${stdout}`);
});
