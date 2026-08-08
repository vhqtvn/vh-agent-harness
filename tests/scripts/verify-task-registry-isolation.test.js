// Regression test for the verify-task-registry.js fixture-leak defect.
//
// DEFECT (closed by this slice): verify-task-registry.js saved its fixture
// cards with { cwd: "/verification" } while state-lib's
// localCoordinatorTasksRoot() resolved to repoRoot()/.local/coordinator/tasks/.
// The cwd option is ACTOR METADATA ONLY — it never redirected storage — so
// every fixture card landed in the REAL coordinator registry. The finally-block
// cleanup removed only the CURRENT run's recorded IDs, so any interruption
// (crash / SIGKILL before finally) orphaned fixture cards in the real registry
// indefinitely (the root cause of the historical P0-REPO-060 orphan). Even on a
// clean run the real registry was polluted mid-run (a concurrent coordinator
// listing tasks would see the fixtures).
//
// FIX: state-lib's localCoordinatorRoot() now honors an opt-in
// OPENCODE_LOCAL_COORDINATOR_ROOT env override (mirroring OPENCODE_STATE_ROOT /
// OPENCODE_RUN_ROOT); the verifier sets it to an isolated temp dir at startup so
// fixtures can NEVER touch the real registry, and removes that dir wholesale in
// finally.
//
// THIS TEST would have caught the leak: it saves a fixture with cwd
// "/verification" (exactly as the verifier does) and asserts the card lands in
// the ISOLATED root, NOT the real .local/coordinator/tasks/. Before the fix, the
// env override did not exist, localCoordinatorRoot() ignored it, and the fixture
// landed in the real registry → the second assertion would FAIL.
//
// Run:  vh-agent-harness exec node --test tests/scripts/verify-task-registry-isolation.test.js
//       (or: node --test tests/scripts/verify-task-registry-isolation.test.js)

import { test } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { pathToFileURL } from "node:url";
import { execFileSync } from "node:child_process";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
// Real repo root, derived from this test file's location (tests/scripts/). Used
// to compute the REAL registry path the fixture must NOT touch. Independent of
// state-lib's internal repoRoot() so the assertion holds whether state-lib is
// imported from the source tree (whose repoRoot() resolves to templates/core/)
// or the rendered tree.
const REPO_ROOT = path.resolve(__dirname, "..", "..");
const REAL_TASKS_DIR = path.join(REPO_ROOT, ".local", "coordinator", "tasks");
const VERIFIER = path.join(
    REPO_ROOT,
    ".opencode",
    "scripts",
    "verify-task-registry.js",
);

// Snapshot the real registry's task file set (file names only), tolerating a
// missing .local/coordinator/tasks/ (gitignored, absent on a fresh clone —
// isolation never creates it). Used to prove the verifier subprocess leaves the
// real registry's file set unchanged — the snapshot compares file-name sets, not
// file content.
function snapshotRegistry() {
    return new Set(
        fs.existsSync(REAL_TASKS_DIR) ? fs.readdirSync(REAL_TASKS_DIR) : [],
    );
}

// Import state-lib from the RENDERED tree (.opencode/scripts/). The source copy
// under templates/core/ still carries the literal {{COORDINATOR_DIR}} token and
// REFUSES to load (assertRenderedNotSource guard); the rendered copy is what the
// verifier actually runs, so testing it is faithful. Run `make update` after
// editing templates/core/ before this test (the normal dev flow).
const STATE_LIB = path.join(
    REPO_ROOT,
    ".opencode",
    "scripts",
    "state-lib.js",
);
const { bindSessionName, saveCoordinationTask } = await import(
    pathToFileURL(STATE_LIB).href
);

// validateIsolatedRoot is exported by the rendered verifier for adversarial
// unit testing. Importing it does NOT run main() (the verifier guards its
// main() invocation behind an ESM entry-point check). It accepts an optional
// baseRoot so this test can drive it against a throwaway temp tree.
const { validateIsolatedRoot } = await import(pathToFileURL(VERIFIER).href);

test("fixture saved with cwd:/verification lands in the ISOLATED coordinator root, not the real registry", () => {
    const isolatedRoot = path.join(REPO_ROOT, "tmp", "test-vtr-isolation-root");
    fs.rmSync(isolatedRoot, { recursive: true, force: true });

    // Snapshot the real registry BEFORE so the test fails loudly if the fix
    // regresses and a fixture touches it (defense-in-depth beyond the
    // path-existence assertion below).
    const realBefore = new Set(
        fs.existsSync(REAL_TASKS_DIR)
            ? fs.readdirSync(REAL_TASKS_DIR)
            : [],
    );

    process.env.OPENCODE_LOCAL_COORDINATOR_ROOT = isolatedRoot;
    const SESSION_ID = "vtr-isolation-regression-session";
    let fixtureId = null;
    try {
        bindSessionName(SESSION_ID, "vtr-isolation-regression", {
            cwd: "/verification",
        });
        const saved = saveCoordinationTask(
            SESSION_ID,
            {
                title: "isolation-regression-fixture",
                task_type: "implementation",
                coordination_mode: "short",
                primary_lane: "isolation-test",
                files_in_scope: ["tests/fixtures/example-pkg/"],
                constraints: ["Regression fixture only."],
                non_goals: ["No implementation work."],
                success_criteria: [
                    "Fixture stays in the isolated root, never the real registry.",
                ],
                validation_plan: [
                    "Assert isolated path exists and real path does not.",
                ],
            },
            { cwd: "/verification" },
        );
        fixtureId = saved.task.task_id;

        // CRUX (load-bearing): the fixture MUST land in the isolated root and
        // MUST NOT leak into the real .local/coordinator/tasks/ registry. Before
        // the fix (no env override support), this second assertion failed — the
        // fixture landed in the real registry because cwd never redirected
        // storage.
        assert.ok(
            fs.existsSync(path.join(isolatedRoot, "tasks", `${fixtureId}.json`)),
            "fixture must land in the isolated coordinator root (OPENCODE_LOCAL_COORDINATOR_ROOT)",
        );
        assert.ok(
            !fs.existsSync(path.join(REAL_TASKS_DIR, `${fixtureId}.json`)),
            `fixture must NOT leak into the real registry: ${path.join(REAL_TASKS_DIR, `${fixtureId}.json`)}`,
        );

        // The real registry's file set must be completely unchanged. Guard
        // existsSync because on a fresh checkout / CI clone the gitignored
        // .local/coordinator/ tree may not exist at all (isolation never
        // creates it) — readdirSync would throw ENOENT.
        const realAfter = new Set(
            fs.existsSync(REAL_TASKS_DIR)
                ? fs.readdirSync(REAL_TASKS_DIR)
                : [],
        );
        assert.deepEqual(
            [...realAfter].filter((f) => !realBefore.has(f)),
            [],
            "real registry must gain no files",
        );
        assert.deepEqual(
            [...realBefore].filter((f) => !realAfter.has(f)),
            [],
            "real registry must lose no files",
        );
    } finally {
        delete process.env.OPENCODE_LOCAL_COORDINATOR_ROOT;
        fs.rmSync(isolatedRoot, { recursive: true, force: true });
    }
});

// INTEGRATION coverage (closes the reviewer F3/F4 gap): the mechanism test above
// proves state-lib honors OPENCODE_LOCAL_COORDINATOR_ROOT, but a future edit that
// reverts the verifier's wiring (no env set, path helpers back to repoRoot())
// would keep that mechanism test green while RE-OPENING the leak. This test runs
// the verifier as a real subprocess and asserts its fixtures never appear in the
// real registry — exercising the full main() wiring (env set + coordinatorRoot()
// routing + finally wholesale removal). A wiring revert fails THIS test.
test("verify-task-registry subprocess leaves the real registry untouched (integration)", () => {
    // Unique prefix so the run does not collide with a concurrently-running
    // verifier (e.g. an operator invoking it) and so the prefix-self-heal only
    // touches this run's own isolated dir.
    const prefix = `isolation-integ-${process.pid}-${Date.now()}`;
    const isolatedRoot = path.join(
        REPO_ROOT,
        "tmp",
        "verify-isolated",
        prefix,
    );

    const before = snapshotRegistry();

    let stdout;
    try {
        stdout = execFileSync("node", [VERIFIER, "--prefix", prefix], {
            cwd: REPO_ROOT,
            encoding: "utf8",
            timeout: 60000,
        });
    } finally {
        // Defense-in-depth: remove the isolated root even if the subprocess
        // crashed before its own finally (the verifier self-cleans on success,
        // but a SIGKILL mid-run could leave it behind).
        fs.rmSync(isolatedRoot, { recursive: true, force: true });
    }

    // The verifier must have run to completion and reported success.
    assert.match(stdout, /verification:\s*ok/, "verifier must report verification: ok");

    // CRUX (load-bearing): after a full verifier run that creates ~10 fixture
    // cards (4 deliberately degraded), the real registry's file set must be
    // unchanged (file-name set equality, not byte content). Before the fix,
    // fixtures landed here and only the recorded IDs were cleaned in finally — so
    // this assertion failed (and an interruption orphaned them forever).
    const after = snapshotRegistry();
    assert.deepEqual(
        [...after].filter((f) => !before.has(f)),
        [],
        "real registry must gain no files from the verifier run",
    );
    assert.deepEqual(
        [...before].filter((f) => !after.has(f)),
        [],
        "real registry must lose no files from the verifier run",
    );
    assert.ok(
        !fs.existsSync(isolatedRoot),
        "isolated dir must be removed by the verifier's finally",
    );
});

// ADVERSARIAL coverage for validateIsolatedRoot (the path-traversal / symlink
// containment guard). B1 from the commit review turned on the fact that lexical
// containment (path.resolve + startsWith) does NOT catch a pre-planted symlink:
// path.resolve never dereferences symlinks, but fs.rmSync({recursive:true})
// follows the OS-resolved intermediate/leaf symlink and would delete through it
// into the real registry. validateIsolatedRoot now adds PHYSICAL containment
// (fs.realpathSync, refusing any path whose real location differs from its
// lexical path). This test plants a symlink on the isolation path and proves
// the guard refuses — the behavioral claim is not verifiable from the diff
// alone, so this test IS the proof. Uses an injectable baseRoot so the test
// never touches the real repo's tmp/verify-isolated.
test("validateIsolatedRoot refuses a symlinked isolation parent and path-traversal prefixes", () => {
    const tmpBase = fs.mkdtempSync(
        path.join(REPO_ROOT, "tmp", "vtr-symlink-test-"),
    );

    try {
        // --- Happy path: clean tree, no symlink. Returns a valid contained path.
        const ok = validateIsolatedRoot("my-run", tmpBase);
        assert.equal(
            ok,
            path.join(tmpBase, "tmp", "verify-isolated", "my-run"),
            "clean prefix resolves to the contained isolated root",
        );

        // --- Path-traversal prefixes are rejected (lexical guard). `..` hits the
        // explicit segment refusal; `../..` and `a/b` hit the separator refusal
        // (they contain '/'). All are rejected before any mkdir/removal.
        assert.throws(
            () => validateIsolatedRoot("..", tmpBase),
            /must not be/i,
            "'..' prefix rejected",
        );
        assert.throws(
            () => validateIsolatedRoot("../..", tmpBase),
            /single path segment|escapes/i,
            "'../../' prefix rejected by segment guard",
        );
        assert.throws(
            () => validateIsolatedRoot("a/b", tmpBase),
            /single path segment/i,
            "multi-segment prefix rejected",
        );

        // --- B1: a pre-planted symlink at the isolation parent must be refused.
        // Mimics `tmp/verify-isolated -> .local/coordinator`: realpath differs
        // from the lexical path, so the physical-containment guard rejects it
        // BEFORE any mkdir or removal. This is the exact vector that would let a
        // recursive fs.rmSync delete the real registry through the symlink.
        const parentDir = path.join(tmpBase, "tmp", "verify-isolated");
        const decoy = path.join(tmpBase, "decoy-target");
        fs.mkdirSync(decoy, { recursive: true });
        fs.mkdirSync(path.dirname(parentDir), { recursive: true });
        fs.rmSync(parentDir, { recursive: true, force: true });
        fs.symlinkSync(decoy, parentDir);
        assert.throws(
            () => validateIsolatedRoot("tasks", tmpBase),
            /resolves to a different physical location|symlink detected/i,
            "validateIsolatedRoot MUST refuse a symlinked isolation parent (B1)",
        );

        // --- B1 leaf variant: a symlinked candidate leaf is also refused.
        fs.rmSync(parentDir, { recursive: true, force: true });
        fs.mkdirSync(parentDir, { recursive: true });
        const leafSymlink = path.join(parentDir, "run-symlinked");
        fs.symlinkSync(decoy, leafSymlink);
        assert.throws(
            () => validateIsolatedRoot("run-symlinked", tmpBase),
            /resolves to a different physical location|symlink detected/i,
            "validateIsolatedRoot MUST refuse a symlinked candidate leaf (B1)",
        );
    } finally {
        fs.rmSync(tmpBase, { recursive: true, force: true });
    }
});
