// Release-wrapper integration test for the FAIL-preserving gate-output
// capture (v0.30.0 incident hardening).
//
// CRUX (the behavioral-closure path exercised end-to-end): a G0
// `go test ./...` red preceded by VOLUMINOUS warning output still yields the
// failing test's identity (`--- FAIL:` / `FAIL` package lines) AND the
// durable capture-file path in the refusal JSON — invoked directly through
// the REAL scripts/release-tag.sh against a scratch repo whose ceremony
// satisfies every PRIOR gate (G7 DEFER manifest handshake), so the parsed
// refusal is the actual wrapper refusal an operator/releaser sees at a real
// ceremony.
//
// Incident record: docs/checkpoints/2026-11-19-release-g0-capture-hardening.md
// (v0.30.0 first invocation: the G0 red's FAIL identity was truncated away by
// the caller's head-kept output budget because the refusal inlined the ENTIRE
// gate output unbounded; non-reproducing — three green re-runs at the
// identical tree; the retained tmp/release-g0-capture-v0.30.0.txt is the
// GREEN retry).
//
// Run:  vh-agent-harness exec node --test tests/scripts/release-tag-g0-capture.test.js
//       (or: node --test tests/scripts/release-tag-g0-capture.test.js)

import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, writeFileSync, rmSync, copyFileSync, readFileSync, readdirSync, existsSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, dirname } from "node:path";
import { spawnSync } from "node:child_process";

const __dirname = dirname(new URL(import.meta.url).pathname.replace(/^\/(?=[A-Za-z]:\/)/, ""));
const REPO_ROOT = join(__dirname, "..", "..");
const RELEASE_TAG = join(REPO_ROOT, "scripts", "release-tag.sh");
const DEFER_EVAL = join(REPO_ROOT, "templates", "core", ".opencode", "scripts", "check-defer-triggers.mjs");
const S2_EVAL = join(REPO_ROOT, "templates", "core", ".opencode", "scripts", "check-s2-holds.mjs");

// Skip the whole file if go or bash is unavailable (the ceremony needs go for
// G0; the wrapper needs bash). A missing toolchain is an environment gap, not
// a regression — the crux is exercised whenever the toolchain is present.
const GO = (() => { try { const r = spawnSync("go", ["version"]); return r.status === 0; } catch (_) { return false; } })();
const BASH = (() => { try { const r = spawnSync("bash", ["-c", "true"]); return r.status === 0; } catch (_) { return false; } })();
const RUN = GO && BASH;

// --- scratch-repo helpers (mirror release-tag-s2-gate.test.js) --------------

function git(repo, ...args) {
    const r = spawnSync("git", ["-c", "init.defaultBranch=main", ...args], {
        cwd: repo,
        env: { ...process.env, GIT_CONFIG_GLOBAL: "/dev/null", GIT_CONFIG_NOSYSTEM: "1" },
    });
    if (r.status !== 0) {
        throw new Error(`git ${args.join(" ")} failed in ${repo}: ${r.stderr && r.stderr.toString()} ${r.stdout && r.stdout.toString()}`);
    }
    return (r.stdout && r.stdout.toString().trim()) || "";
}

function writeAndCommit(repo, files, msg) {
    for (const rel of Object.keys(files)) {
        const abs = join(repo, rel);
        mkdirSync(dirname(abs), { recursive: true });
        writeFileSync(abs, files[rel]);
    }
    git(repo, "add", "-A");
    git(repo, "commit", "-m", msg);
}

// Build the release ceremony in a scratch repo (P -> R -> M handshake) so the
// wrapper passes G7 and reaches G0. `goFiles` carries the Go module content of
// the release-prep commit P — the caller decides whether G0 is green (default
// empty package), a spam-preceded test red, or a vet red.
function setupCeremony(goFiles) {
    const root = mkdtempSync(join(tmpdir(), "g0-cap-"));
    const repo = join(root, "repo");
    mkdirSync(repo, { recursive: true });
    git(repo, "init");
    git(repo, "config", "user.email", "t@t");
    git(repo, "config", "user.name", "t");
    const msgFile = join(root, "msg.txt");
    writeFileSync(msgFile, "release v0.1.0\n");

    // Evaluators (committed in the release-prep commit so G6/G7 can invoke them).
    const evalDir = join(repo, ".opencode", "scripts");
    mkdirSync(evalDir, { recursive: true });
    copyFileSync(DEFER_EVAL, join(evalDir, "check-defer-triggers.mjs"));
    copyFileSync(S2_EVAL, join(evalDir, "check-s2-holds.mjs"));

    // --- commit P (release-prep, HEAD^^) ---
    const pFiles = {
        "go.mod": "module scratch\n\ngo 1.25.0\n",
        "docs/planning/backlog.md": "# Backlog\n\n## Now\n\n| ID | Status | Area | Task | Owner | Notes | Links |\n| --- | --- | --- | --- | --- | --- | --- |\n",
        ...goFiles,
    };
    writeAndCommit(repo, pFiles, "release-prep");
    const P_SHA = git(repo, "rev-parse", "HEAD");

    // --- commit R (readiness artifact, HEAD^) ---
    const artifact = {
        schema_version: 1,
        commit_sha: P_SHA,
        model_gates: {
            G1_coverage: "ready",
            G2_significance: "ready",
            G3_docs: "ready",
            G4_visibility: "ready",
            G5_curated_note: "ready",
        },
    };
    writeAndCommit(repo, {
        ".vh-agent-harness/release-readiness-pass.json": `${JSON.stringify(artifact, null, 2)}\n`,
    }, "readiness artifact");
    const R_SHA = git(repo, "rev-parse", "HEAD");
    const R_TREE = git(repo, "rev-parse", "HEAD^{tree}");

    // --- commit M (DEFER manifest, HEAD) ---
    const manifest = {
        schema_version: 1,
        release_base: { kind: "root", value: null },
        evaluated_commit: R_SHA,
        evaluated_tree: R_TREE,
        manifest_parent_commit: R_SHA,
        reconciliation: { status: "clear", scope: "all", zero_records_confirmed: true },
        records: [],
    };
    writeAndCommit(repo, {
        ".vh-agent-harness/release-defer-dispositions.json": `${JSON.stringify(manifest, null, 2)}\n`,
    }, "DEFER manifest");

    return { root, repo, msgFile };
}

function runReleaseTag(repo, msgFile) {
    const r = spawnSync("bash", [RELEASE_TAG, "v0.1.0"], {
        cwd: repo,
        env: {
            ...process.env,
            RELEASE_TAG_MESSAGE_FILE: msgFile,
            GIT_CONFIG_GLOBAL: "/dev/null",
            GIT_CONFIG_NOSYSTEM: "1",
        },
    });
    return {
        status: r.status,
        stdout: (r.stdout && r.stdout.toString()) || "",
        stderr: (r.stderr && r.stderr.toString()) || "",
    };
}

function tagExists(repo, version) {
    const r = spawnSync("git", ["tag", "-l", version], { cwd: repo });
    return Boolean((r.stdout && r.stdout.toString().trim()));
}

function dispose(root) {
    try { rmSync(root, { recursive: true, force: true }); } catch (_) { /* best-effort */ }
}

// Parse the wrapper's final JSON line from stdout.
function parseEmit(stdout) {
    const lines = stdout.split("\n").filter(Boolean);
    const last = lines[lines.length - 1];
    try { return JSON.parse(last); } catch (_) { return null; }
}

// Extract the capture-file path the refusal names (absolute: the wrapper
// writes under <repo-root>/tmp/, resolved via git rev-parse --show-toplevel).
function capturePathFromError(error) {
    const m = error.match(/(\/[^\s"]*tmp\/release-gate-capture-v0\.1\.0-[A-Za-z0-9-]+\.txt)/);
    return m ? m[1] : null;
}

// A go test that emits ~4000 WARNING lines BEFORE failing — the v0.30.0
// incident shape: the FAIL identity lives at the output TAIL, behind spam.
// (gofmt-clean: tabs, canonical formatting.)
const SPAM_TEST_GO = `package pkg

import (
	"fmt"
	"testing"
)

func TestWarnThenFail(t *testing.T) {
	for i := 0; i < 4000; i++ {
		fmt.Println("WARNING spam line", i, "padding consume-display-budget padding")
	}
	t.Fatal("intentional failure: g0 capture-hardening fixture")
}
`;

// A package that compiles and PASSES `go test ./...` (no test files; note
// `go test` itself runs only a small high-confidence vet subset that does NOT
// include copylocks) but reds the wrapper's dedicated `go vet ./...` step —
// exercises the vet arm of the G0 chain plus the small-output summary branch
// of g0_fail_reason.
const VET_RED_GO = `package pkg

import "sync"

type Locked struct {
	mu sync.Mutex
}

func CopyLock(l Locked) {}
`;

// --- tests ------------------------------------------------------------------

test("crux: spam-preceded G0 go-test red still names the failing test + capture path in the refusal", { skip: RUN ? false : "go/bash unavailable" }, () => {
    const { root, repo, msgFile } = setupCeremony({ "pkg/pkg.go": "package pkg\n", "pkg/spam_test.go": SPAM_TEST_GO });
    try {
        const { status, stdout } = runReleaseTag(repo, msgFile);
        assert.notEqual(status, 0, "release-tag.sh must exit nonzero on a G0 test red");
        const emitted = parseEmit(stdout);
        assert.ok(emitted, "wrapper must emit a JSON line");
        assert.equal(emitted.ok, false);
        assert.ok(emitted.error.includes("G0 go test ./... failed"), "the refusal names the failing gate");

        // THE INVARIANT: the failing test's identity survives the spam,
        // independent of any display budget.
        assert.match(emitted.error, /--- FAIL: TestWarnThenFail/, "FAIL identity must survive voluminous preceding warnings");
        assert.match(emitted.error, /FAIL\s+scratch\/pkg/, "the failing package's FAIL line must survive");
        // Identity is placed BEFORE the bounded excerpt — spam volume can
        // never displace it.
        const failIdx = emitted.error.indexOf("--- FAIL: TestWarnThenFail");
        const excerptIdx = emitted.error.indexOf("displayed excerpt");
        assert.ok(failIdx !== -1 && excerptIdx !== -1 && failIdx < excerptIdx, "FAIL summary must precede the excerpt");

        // The refusal names the durable capture file...
        const capPath = capturePathFromError(emitted.error);
        assert.ok(capPath, `refusal must name the capture file path (error head: ${emitted.error.slice(0, 400)})`);
        // ...and the capture file EXISTS and is byte-complete: both the spam
        // head and the FAIL tail are in it, unfiltered.
        assert.ok(existsSync(capPath), `capture file must exist at ${capPath}`);
        const captured = readFileSync(capPath, "utf8");
        assert.match(captured, /WARNING spam line 0 padding/, "capture keeps the spam head");
        assert.match(captured, /WARNING spam line 1000 padding/, "capture keeps the elided middle");
        assert.match(captured, /WARNING spam line 3999 padding/, "capture keeps the spam tail");
        assert.match(captured, /--- FAIL: TestWarnThenFail/, "capture keeps the FAIL identity");

        // The DISPLAYED excerpt is bounded: a middle spam line is elided from
        // the refusal (but present in the capture), the elision marker is
        // present, and head+tail of the excerpt survive.
        assert.doesNotMatch(emitted.error, /WARNING spam line 1000 padding/, "middle spam must be elided from the refusal");
        assert.match(emitted.error, /lines elided/, "the elision marker must be present");
        assert.match(emitted.error, /WARNING spam line 0 padding/, "excerpt head (first 40) survives");
        assert.match(emitted.error, /WARNING spam line 3999 padding/, "excerpt tail (last 40) survives");
        // Bounded payload: the refusal is orders of magnitude smaller than
        // the full output.
        assert.ok(
            emitted.error.split("\n").length < 200 && captured.split("\n").length > 4000,
            `refusal must stay bounded (refusal lines=${emitted.error.split("\n").length}, capture lines=${captured.split("\n").length})`,
        );
    } finally { dispose(root); }
});

test("vet red: refusal carries the vet error + capture path (small-output summary branch)", { skip: RUN ? false : "go/bash unavailable" }, () => {
    const { root, repo, msgFile } = setupCeremony({ "pkg/pkg.go": VET_RED_GO });
    try {
        const { status, stdout } = runReleaseTag(repo, msgFile);
        assert.notEqual(status, 0, "release-tag.sh must exit nonzero on a G0 vet red");
        const emitted = parseEmit(stdout);
        assert.ok(emitted, "wrapper must emit a JSON line");
        assert.equal(emitted.ok, false);
        assert.ok(emitted.error.includes("G0 go vet ./... failed"), "the refusal names the failing gate (vet arm of the chain)");
        // Small output: the full output doubles as the failure summary.
        assert.match(emitted.error, /lock by value/, "the vet diagnostic (failing identity for a non-test gate) survives in the refusal");
        assert.match(emitted.error, /no extraction pattern matched/, "small-output summary branch used");
        const capPath = capturePathFromError(emitted.error);
        assert.ok(capPath, "refusal must name the capture file path");
        assert.ok(existsSync(capPath), `capture file must exist at ${capPath}`);
        assert.match(readFileSync(capPath, "utf8"), /lock by value/, "capture keeps the full vet output");
    } finally { dispose(root); }
});

test("green ceremony: G0 passes and NO capture file is written (G0b never sees wrapper scratch)", { skip: RUN ? false : "go/bash unavailable" }, () => {
    const { root, repo, msgFile } = setupCeremony({ "pkg/pkg.go": "package pkg\n" });
    try {
        const { status, stdout } = runReleaseTag(repo, msgFile);
        assert.equal(status, 0, `green ceremony must complete (stdout=${stdout})`);
        assert.equal(tagExists(repo, "v0.1.0"), true, "the tag must be created when every gate is green");
        // A GREEN run writes no capture file — the capture exists exactly
        // when it has diagnostic value (a red), and G0b's dirty-tree check
        // (which runs AFTER G0 in the classic flow) never sees wrapper
        // scratch even in repos that do not gitignore tmp/.
        const tmpDir = join(repo, "tmp");
        if (existsSync(tmpDir)) {
            const leftovers = readdirSync(tmpDir).filter((f) => f.startsWith("release-gate-capture-"));
            assert.equal(leftovers.length, 0, `green run must not leave capture files (found: ${leftovers.join(", ")})`);
        }
    } finally { dispose(root); }
});
