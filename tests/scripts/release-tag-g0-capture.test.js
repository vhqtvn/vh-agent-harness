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
// Arms (each spawns the REAL wrapper end-to-end against a scratch repo):
//   1. classic-flow go-test red, spam-preceded (the crux, above)
//   2. classic-flow go-vet red (small-output summary branch)
//   3. classic-flow green (no capture file written)
//   4. HISTORICAL-flow go-test red — g0_fail_reason invoked from the
//      `(cd "$X_DIR" && ...)` arms; the capture must land in the ORIGINAL
//      repo's tmp/ (G0_REPO_ROOT resolves from the wrapper CWD, not the
//      detached worktree) and survive the EXIT-trap worktree cleanup
//   5. G0c doctor CONTROL-run red — a lineage.yml fixture + a stub
//      ./bin/vh-agent-harness honoring the staleness guard produce a red
//      control run, exercising the tier-FAIL extraction pattern
//   6. capture-write-failure degradation — an unwritable capture
//      destination preserves the original gate verdict + exit code, keeps
//      the refusal JSON parseable, and notes CAPTURE WRITE FAILED
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
import { mkdtempSync, mkdirSync, writeFileSync, rmSync, copyFileSync, readFileSync, readdirSync, existsSync, chmodSync } from "node:fs";
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

function writeAndCommit(repo, files, msg, modes = {}) {
    for (const rel of Object.keys(files)) {
        const abs = join(repo, rel);
        mkdirSync(dirname(abs), { recursive: true });
        writeFileSync(abs, files[rel]);
        // Optional per-path file mode (e.g. 0o755 for an executable stub the
        // wrapper must invoke — git records the exec bit on commit).
        if (modes[rel]) chmodSync(abs, modes[rel]);
    }
    git(repo, "add", "-A");
    git(repo, "commit", "-m", msg);
}

// Build the release ceremony in a scratch repo (P -> R -> M handshake) so the
// wrapper passes G7 and reaches G0. `goFiles` carries the Go module content of
// the release-prep commit P — the caller decides whether G0 is green (default
// empty package), a spam-preceded test red, or a vet red.
//
// opts (all optional):
//   extraPFiles  — extra files merged into commit P (e.g. lineage.yml, the
//                  bin/ doctor stub for the G0c arm).
//   execModes    — { relPath: 0o755 } file modes applied inside commit P.
//   postManifest — when present, commits a NON-PROTECTED post-ceremony commit
//                  H on top of M, switching the wrapper into the HISTORICAL
//                  flow (M != H). Shape: { files?: {...}, remove?: [rel...],
//                  msg?: string }. Historical eligibility requires the
//                  migration note (templates/migrations/<v>.md) to exist at M
//                  and tmp/ to exist AND be gitignored (the pre-isolation G0b
//                  must stay clean while the verification worktree X lives
//                  under tmp/) — both are provided automatically when this
//                  option is used.
function setupCeremony(goFiles, opts = {}) {
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
        ...(opts.extraPFiles || {}),
    };
    if (opts.postManifest) {
        // Historical eligibility (c): the about-to-release migration note must
        // exist at M and stay blob-identical through H. tmp/ must be ignored
        // so the pre-isolation G0b on the ORIGINAL checkout stays clean while
        // X (and the capture file) live under tmp/.
        pFiles[".gitignore"] = "tmp/\n";
        pFiles["templates/migrations/v0.1.0.md"] = "# v0.1.0 migration note (historical-flow derivation fixture)\n";
    }
    writeAndCommit(repo, pFiles, "release-prep", opts.execModes || {});
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

    // --- optional commit H (post-ceremony chore, new HEAD) ---
    // Lands only non-protected paths (the eligibility scan refuses anything
    // else), so the derived manifest commit M stays != HEAD and the wrapper
    // enters the HISTORICAL flow: G0 runs inside the detached verification
    // worktree at M, not against H's tree.
    if (opts.postManifest) {
        const pm = opts.postManifest;
        for (const rel of pm.remove || []) {
            rmSync(join(repo, rel), { force: true });
        }
        if (pm.files && Object.keys(pm.files).length > 0) {
            writeAndCommit(repo, pm.files, pm.msg || "post-ceremony chore");
        } else {
            git(repo, "add", "-A");
            git(repo, "commit", "-m", pm.msg || "post-ceremony chore");
        }
        // The historical isolation block requires <repo>/tmp/ to EXIST (the
        // worktree X is created under it and the canonical-parent check
        // refuses when it is missing). It is gitignored, so the pre-isolation
        // G0b on the ORIGINAL checkout stays clean.
        mkdirSync(join(repo, "tmp"), { recursive: true });
    }

    return { root, repo, msgFile };
}

function runReleaseTag(repo, msgFile, extraEnv = {}) {
    const r = spawnSync("bash", [RELEASE_TAG, "v0.1.0"], {
        cwd: repo,
        env: {
            ...process.env,
            RELEASE_TAG_MESSAGE_FILE: msgFile,
            GIT_CONFIG_GLOBAL: "/dev/null",
            GIT_CONFIG_NOSYSTEM: "1",
            ...extraEnv,
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

// A test that reds only at the DERIVED manifest commit M (the historical-flow
// fixture removes it in the post-ceremony H commit, making H's tree green):
// a wrapper that wrongly evaluated H's tree would sail past G0, while the
// correct M-tree evaluation reds with the "at M (verification worktree)"
// refusal. (gofmt-clean: tab indent.)
const HISTORICAL_RED_TEST_GO = `package pkg

import "testing"

func TestHistoricalFlowRed(t *testing.T) {
	t.Fatal("intentional failure: historical-flow G0 capture fixture")
}
`;

// Minimal ceremony-binary stub for the G0c arm, honoring the wrapper's REAL
// ceremony-binary contract (verified in scripts/release-tag.sh's G0c block):
//   1. the binary must be EXECUTABLE at ./bin/vh-agent-harness (guard: -x);
//   2. the STALENESS GUARD runs \`$HARNESS_BIN release inject-errata --help\`
//      and requires exit 0 — the stub's \`release\` arm answers exactly that
//      canary (a stub that did not honor it would produce the staleness
//      refusal, not the control-run refusal the arm asserts);
//   3. the CONTROL RUN runs \`$HARNESS_BIN doctor\` with
//      VH_HARNESS_DEFER_DIFF_SINCE set and requires exit 0 for HEALTHY — the
//      stub reds it with 120 lines of doctor-shaped output whose FAIL-tier
//      lines (matching the tier-FAIL extraction pattern
//      '[[:space:]]FAIL([[:space:]]|$)') sit at lines 61 and 119: line 61 is
//      INSIDE the elided excerpt zone (41..80 for a 120-line output), so its
//      presence in the refusal proves the extraction rescued it from behind
//      the display budget.
const DOCTOR_STUB = `#!/usr/bin/env bash
# g0-capture test stub — NOT the real harness binary.
case "$1" in
  release)
    # Staleness-guard canary: a binary carrying the staged-errata-content
    # enforcement answers \`release inject-errata --help\` with exit 0.
    exit 0
    ;;
  doctor)
    i=1
    while [ "$i" -le 60 ]; do
      printf 'stub doctor: check %03d coordinator-noise PASS ok\n' "$i"
      i=$((i + 1))
    done
    printf 'stub doctor: check 061 defer-liveness FAIL open cards past due\n'
    i=62
    while [ "$i" -le 118 ]; do
      printf 'stub doctor: check %03d coordinator-noise PASS ok\n' "$i"
      i=$((i + 1))
    done
    printf 'stub doctor: check 119 behavioral-closure FAIL inconsistent declaration\n'
    printf 'stub doctor: check 120 managed-drift PASS ok\n'
    exit 1
    ;;
  *)
    exit 2
    ;;
esac
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

test("historical-flow G0 red: capture lands in the ORIGINAL repo's tmp/ and survives the EXIT-trap worktree cleanup", { skip: RUN ? false : "go/bash unavailable" }, () => {
    // M's tree carries a red test; the post-ceremony commit H (non-protected
    // path: it deletes the test file) makes H's tree GREEN. A wrapper that
    // wrongly evaluated H would pass G0; the correct M-tree evaluation reds.
    const { root, repo, msgFile } = setupCeremony(
        { "pkg/pkg.go": "package pkg\n", "pkg/fail_test.go": HISTORICAL_RED_TEST_GO },
        { postManifest: { remove: ["pkg/fail_test.go"], files: { "docs/post-ceremony-chore.md": "chore\n" } } },
    );
    try {
        const { status, stdout } = runReleaseTag(repo, msgFile);
        assert.notEqual(status, 0, "release-tag.sh must exit nonzero on a historical-flow G0 red at M");

        const emitted = parseEmit(stdout);
        assert.ok(emitted, "wrapper must emit a JSON line");
        assert.equal(emitted.ok, false);
        // The refusal must be the HISTORICAL G0 refusal: JSON historical=true
        // and the gate identity names the verification worktree at M. This is
        // the M-tree (not H-tree) evaluation proof: H's tree is green.
        assert.equal(emitted.historical, true, "the refusal must carry historical=true");
        assert.ok(emitted.error.includes("G0 go test ./... failed at M (verification worktree)"), "the refusal names the historical gate arm");
        assert.match(emitted.error, /--- FAIL: TestHistoricalFlowRed/, "the failing test's identity survives in the refusal");
        assert.equal(tagExists(repo, "v0.1.0"), false, "no tag may be cut on a red");

        // THE INVARIANT (arm 1): G0_REPO_ROOT must resolve from the WRAPPER
        // CWD — the ORIGINAL checkout — not from inside the detached worktree
        // X. The capture file must therefore live directly under the
        // ORIGINAL repo's tmp/ and must still exist AFTER the wrapper exited
        // (the EXIT trap has run by the time spawnSync returns).
        const capPath = capturePathFromError(emitted.error);
        assert.ok(capPath, `refusal must name the capture file path (error head: ${emitted.error.slice(0, 400)})`);
        assert.ok(
            capPath.startsWith(join(repo, "tmp") + "/"),
            `capture must land in the ORIGINAL repo's tmp/ (got ${capPath}; repo=${repo})`,
        );
        assert.ok(!capPath.includes("release-verify"), "the capture must never live under the temporary verification worktree");
        assert.ok(existsSync(capPath), `capture file must exist at ${capPath} AFTER the EXIT-trap cleanup ran`);
        assert.match(readFileSync(capPath, "utf8"), /--- FAIL: TestHistoricalFlowRed/, "capture keeps the FAIL identity");

        // The EXIT trap removed the verification worktree (and only it): the
        // capture file is a SIBLING under tmp/, never a child of X, so the
        // cleanup must leave it untouched while removing every release-verify
        // directory.
        const tmpDir = join(repo, "tmp");
        const verifyLeftovers = readdirSync(tmpDir).filter((f) => f.startsWith("release-verify-"));
        assert.equal(verifyLeftovers.length, 0, `the EXIT trap must remove the verification worktree (leftovers: ${verifyLeftovers.join(", ")})`);
        const captures = readdirSync(tmpDir).filter((f) => f.startsWith("release-gate-capture-"));
        assert.ok(captures.length >= 1, "the capture file survives the cleanup");
    } finally { dispose(root); }
});

test("G0c doctor control-run red: lineage.yml fixture + ceremony-binary stub exercise the tier-FAIL extraction", { skip: RUN ? false : "go/bash unavailable" }, () => {
    // The lineage record turns the G0c CONTROL run on (per-side
    // applicability: the worktree file gates the control run). The stub
    // ./bin/vh-agent-harness (committed executable) answers the staleness
    // canary with exit 0 and reds `doctor` with FAIL-tier lines — reaching
    // the control-run refusal at all proves the staleness guard passed.
    const { root, repo, msgFile } = setupCeremony(
        { "pkg/pkg.go": "package pkg\n" },
        {
            extraPFiles: {
                ".vh-agent-harness/lineage.yml": "schema_version: 1\n",
                "bin/vh-agent-harness": DOCTOR_STUB,
            },
            execModes: { "bin/vh-agent-harness": 0o755 },
        },
    );
    try {
        const { status, stdout } = runReleaseTag(repo, msgFile);
        assert.notEqual(status, 0, "release-tag.sh must exit nonzero on a G0c control-run red");

        const emitted = parseEmit(stdout);
        assert.ok(emitted, "wrapper must emit a JSON line");
        assert.equal(emitted.ok, false);
        // The CONTROL-run refusal (NOT the staleness refusal) — this is the
        // proof the stub honored the guard's real requirements.
        assert.ok(emitted.error.includes("G0c doctor not HEALTHY"), "the refusal names the G0c control gate");
        // THE INVARIANT (arm 2): the tier-FAIL line at output line 61 sits
        // INSIDE the elided excerpt zone, so its presence in the refusal is
        // the extraction pattern rescuing it from behind the display budget.
        assert.match(emitted.error, /defer-liveness FAIL open cards past due/, "the mid-output tier-FAIL line is extracted ahead of the excerpt");
        assert.match(emitted.error, /behavioral-closure FAIL inconsistent declaration/, "the tail tier-FAIL line is extracted too");
        assert.match(emitted.error, /failure summary \(extracted/, "the extracted-summary branch is used (doctor pattern, not the go-test FAIL shape)");
        assert.doesNotMatch(emitted.error, /check 050 /, "mid-excerpt noise is elided from the refusal");
        assert.match(emitted.error, /lines elided/, "the elision marker is present (120-line output)");
        assert.match(emitted.error, /check 120 /, "the excerpt tail survives");
        assert.equal(tagExists(repo, "v0.1.0"), false, "no tag may be cut on a red");

        // Durable capture: the full 120-line stub doctor report, including
        // the noise the refusal elided.
        const capPath = capturePathFromError(emitted.error);
        assert.ok(capPath, "refusal must name the capture file path");
        assert.ok(existsSync(capPath), `capture file must exist at ${capPath}`);
        const captured = readFileSync(capPath, "utf8");
        assert.match(captured, /check 050 /, "capture keeps the elided middle");
        assert.match(captured, /check 061 defer-liveness FAIL/, "capture keeps the mid FAIL-tier line in place");
        assert.match(captured, /check 120 /, "capture keeps the tail");
    } finally { dispose(root); }
});

test("capture-write-failure degradation: gate verdict + exit preserved, parseable refusal, CAPTURE WRITE FAILED note", { skip: RUN ? false : "go/bash unavailable" }, () => {
    const { root, repo, msgFile } = setupCeremony({ "pkg/pkg.go": "package pkg\n", "pkg/spam_test.go": SPAM_TEST_GO });
    try {
        // FIXTURE DESIGN (the card's open question for this arm, resolved):
        // a permission-based "unwritable tmp/" is IMPOSSIBLE here — the DEFER
        // evaluator (check-defer-triggers.mjs, gitCapture/allocCaptureDir)
        // allocates its own scratch SUBDIRECTORIES under the same <repo>/tmp
        // BEFORE G0 runs, and POSIX write permission governs open() and
        // mkdir() symmetrically, so any perm state that fails the wrapper's
        // `> tmp/file` also fails the evaluator's `mkdir tmp/<subdir>` and
        // reds the DEFER gate first (verified empirically: a tmp/ that breaks
        // the evaluator yields `release-defer-gate: evaluator-error`, exit 2,
        // before G0 is ever reached). The deterministic discriminator is the
        // capture FILENAME: bash sources $BASH_ENV at startup, and the only
        // executable `date` call in the wrapper is the capture filename's
        // timestamp component (verified by grep). This guarded `date`
        // function inflates that component past NAME_MAX (255), so the
        // destination is unwritable-by-name (ENAMETOOLONG on open) while the
        // evaluator — a node child that never sees bash functions — keeps a
        // healthy writable tmp/. Every other date invocation falls through to
        // the real binary; no wrapper semantics change.
        const bashEnv = join(root, "bash-env-arm3.sh");
        writeFileSync(bashEnv, `# g0-capture arm-3 fixture: make the capture destination unwritable-by-name.
date() {
  if [ "$1" = "-u" ] && [ "$2" = "+%Y%m%dT%H%M%SZ" ]; then
    printf 'x%.0s' {1..300}
    printf '\n'
    return 0
  fi
  command date "$@"
}
`);

        const { status, stdout } = runReleaseTag(repo, msgFile, { BASH_ENV: bashEnv });
        // The degradation NEVER changes the gate verdict: same nonzero exit
        // as an ordinary G0 test red (the wrapper's exit 1).
        assert.equal(status, 1, "the capture-write failure must not alter the refusal exit code");

        const emitted = parseEmit(stdout);
        // Parseable refusal JSON despite the failed write — the refusal is
        // composed from the in-memory gate output.
        assert.ok(emitted, "wrapper must still emit a parseable JSON line");
        assert.equal(emitted.ok, false);
        assert.ok(emitted.error.includes("G0 go test ./... failed"), "the original gate verdict is preserved");
        assert.match(emitted.error, /--- FAIL: TestWarnThenFail/, "the FAIL identity is preserved from the in-memory output");
        // The degradation note and its honest excerpt provenance.
        assert.match(emitted.error, /CAPTURE WRITE FAILED/, "the refusal notes the capture write failed");
        assert.match(emitted.error, /summary and excerpt below are from the in-memory gate output/, "the note states the provenance of summary + excerpt");
        // The elide fallback no longer promises caller scrollback (wording
        // tightening): it states the elided lines are not preserved.
        assert.match(emitted.error, /lines elided[^\n]*NOT captured/, "the elide fallback fires on the elision-marker line itself (spam output exceeds the excerpt budget)");
        const elideLine = emitted.error.split("\n").find((l) => l.includes("NOT captured"));
        assert.ok(elideLine, "the NOT-captured elide note is present");
        assert.ok(!elideLine.includes("scrollback"), "the elide fallback must not promise caller scrollback");
        // The bounded excerpt still engages (head/tail survive, middle elided).
        assert.match(emitted.error, /WARNING spam line 0 padding/, "excerpt head survives");
        assert.match(emitted.error, /WARNING spam line 3999 padding/, "excerpt tail survives");
        assert.doesNotMatch(emitted.error, /WARNING spam line 1000 padding/, "middle spam stays elided");
        assert.equal(tagExists(repo, "v0.1.0"), false, "no tag may be cut on a red");

        // No capture file was materialized: the named (overlong) path does not
        // exist, and tmp/ carries no capture files. The long-x timestamp
        // component still matches the path extractor ([A-Za-z0-9-]+).
        const capPath = capturePathFromError(emitted.error);
        assert.ok(capPath, "the refusal still names the (unwritten) capture path");
        assert.ok(!existsSync(capPath), "no capture file may exist at the named path");
        const tmpDir = join(repo, "tmp");
        if (existsSync(tmpDir)) {
            const leftovers = readdirSync(tmpDir).filter((f) => f.startsWith("release-gate-capture-"));
            assert.equal(leftovers.length, 0, `no capture files may be materialized (found: ${leftovers.join(", ")})`);
        }
    } finally { dispose(root); }
});
