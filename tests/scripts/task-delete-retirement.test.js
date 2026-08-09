// Landing-gated retirement tests for `completed` coordination task cards.
//
// m0141 Slice 2 (the CODE): a `completed` transport card may be retired as done
// ONLY when a reachable commit carrying the exact `Task-Card: <card-id>` trailer
// line is present (the landing-proof contract, docs/coordination/RECORD_LIFECYCLE.md).
// The verifier is a REACHABILITY check — `git log --branches` over all branch
// tips, NEVER object existence (per the closure-verifier reachability rule,
// researches/decisions/2026-08-04-binding-regression-unification-audit.md
// Addendum). An orphaned / reflog-only commit satisfies object existence while
// being absent from every branch, so it must NOT count as landed.
//
// This suite pins the retirement path end-to-end against a HERMETIC scratch git
// repo (staging reachable vs orphaned trailer commits), so it never depends on
// the live repo's history for the pass-case. It also pins:
//   - in-flight protection is NOT weakened: working/reported/blocked refuse with
//     lifecycle_state_protected EVEN WHEN a reachable trailer commit exists
//     (the in-flight guard runs before the retirement path);
//   - cancelled / draft / ready remain freely disposable (no git check);
//   - `force` is the emergency override for a completed card with no trailer.
//
// Test seam (NOT an env var): the verifier's repo root is redirected at the
// scratch repo via the `landingProofRepo` OPTION on deleteCoordinationTask, which
// the test injects by importing state-lib directly. plan-state.js does NOT
// forward this option from the tool args, so production plan_state calls cannot
// configure it and an ambient env var cannot redirect the landing gate.
//
// Crux (behavioral-closure): a completed card WITH a reachable Task-Card trailer
// commit is deletable via the ordinary path, AND a completed card WITHOUT one is
// refused with landing_not_confirmed.
//
// Run:  vh-agent-harness exec node --test tests/scripts/task-delete-retirement.test.js
//       (or: node --test tests/scripts/task-delete-retirement.test.js)

import { test } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import os from "node:os";
import { fileURLToPath, pathToFileURL } from "node:url";
import { spawnSync } from "node:child_process";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = path.resolve(__dirname, "..", "..");

// Import state-lib from the RENDERED tree (.opencode/scripts/). The source copy
// under templates/core/ still carries the literal {{COORDINATOR_DIR}} token and
// REFUSES to load (assertRenderedNotSource guard). Run `make update` after
// editing templates/core/ before this test (the normal dev flow).
const STATE_LIB = path.join(
    REPO_ROOT,
    ".opencode",
    "scripts",
    "state-lib.js",
);
const {
    bindSessionName,
    deleteCoordinationTask,
    listCoordinationTasks,
} = await import(pathToFileURL(STATE_LIB).href);

// --- hermetic scratch git repo helpers ---------------------------------------

// Build a scratch git repo. Returns { dir, git } where git(args) runs
// `git -C <dir> <args>` and throws on non-zero exit. Deterministic `main`
// branch, gpg off, test identity. Used as the landing-proof repo the verifier
// queries (via the landingProofRepo option) so no case depends on the live
// repo's history.
function makeScratchRepo() {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "retire-landing-"));
    const run = (args) =>
        spawnSync("git", ["-C", dir, ...args], { encoding: "utf8" });
    let r = run(["init", "-q", "-b", "main"]);
    if (r.status !== 0) {
        // Older git without `init -b`: fall back to default branch then rename.
        r = run(["init", "-q"]);
        if (r.status !== 0) {
            throw new Error(`scratch git init failed: ${r.stderr || r.stdout}`);
        }
        run(["branch", "-M", "main"]);
    }
    run(["config", "user.email", "retire@test"]);
    run(["config", "user.name", "retire-test"]);
    run(["config", "commit.gpgsign", "false"]);
    const git = (args) => {
        const res = run(args);
        if (res.status !== 0) {
            throw new Error(
                `git ${args.join(" ")} failed in scratch repo: ${res.stderr || res.stdout}`,
            );
        }
        return res;
    };
    return { dir, git };
}

// Stage a unique file and commit with the given message. The scratch repo only
// ever holds test files, so `add -A` is safe and keeps each commit a real tree
// change git will accept.
function commitIn(git, dir, message) {
    const name = `f-${process.pid}-${Math.random().toString(36).slice(2)}.txt`;
    fs.writeFileSync(path.join(dir, name), `content ${name}\n`);
    git(["add", "-A"]);
    git(["commit", "-q", "-m", message]);
}

// --- isolated coordinator transport helpers ---------------------------------
// Cards are written directly (explicit, predictable ids) so the trailer commits
// can target them by id. deleteCoordinationTask loads leniently: it needs only a
// parseable JSON object with a `status` field for the guards. Report dirs carry
// a sentinel proven to survive a refusal and vanish on a deletion.

function cardPath(isoRoot, id) {
    return path.join(isoRoot, "tasks", `${id}.json`);
}
function reportDir(isoRoot, id) {
    return path.join(isoRoot, "reports", id);
}
function writeCard(isoRoot, id, fields) {
    fs.mkdirSync(path.dirname(cardPath(isoRoot, id)), { recursive: true });
    fs.writeFileSync(
        cardPath(isoRoot, id),
        JSON.stringify(
            {
                id,
                title: `retirement fixture ${id}`,
                task_type: "implementation",
                coordination_mode: "short",
                status: "draft",
                active_session_alias: null,
                ...fields,
            },
            null,
            2,
        ),
    );
    const rd = reportDir(isoRoot, id);
    fs.mkdirSync(rd, { recursive: true });
    fs.writeFileSync(
        path.join(rd, "closeout.md"),
        `# ${fields.status || "draft"} evidence for ${id}\n`,
    );
}
function cardExists(isoRoot, id) {
    return fs.existsSync(cardPath(isoRoot, id));
}
function reportExists(isoRoot, id) {
    return fs.existsSync(reportDir(isoRoot, id));
}

const SESSION_ID = "retirement-test-session";
let isoRoot;
let prevCoordRoot;
let scratch;

test("completed-card retirement: landing-gated path (m0141 Slice 2)", () => {
    // ---- setup: isolated transport + hermetic scratch git repo -------------
    isoRoot = fs.mkdtempSync(path.join(os.tmpdir(), "retire-iso-"));
    prevCoordRoot = process.env.OPENCODE_LOCAL_COORDINATOR_ROOT;
    process.env.OPENCODE_LOCAL_COORDINATOR_ROOT = isoRoot;
    bindSessionName(SESSION_ID, "retirement-test");

    scratch = makeScratchRepo();

    // Test seam: inject the scratch repo as the landing-proof repo via the
    // landingProofRepo OPTION (a direct-import injection point plan-state.js
    // does NOT forward). Defaults to the scratch repo; a case may override it
    // (e.g. the non-git fail-closed case) by passing landingProofRepo explicitly.
    const retire = (id, opts = {}) =>
        deleteCoordinationTask(SESSION_ID, id, {
            cwd: "/verification",
            landingProofRepo: scratch.dir,
            ...opts,
        });

    // Seed the scratch repo's branch history. Order matters: the orphan case
    // relies on `reset --hard HEAD~1` leaving a commit reflog-only.
    //   C0            initial (no trailer)
    //   C-reach       "Task-Card: completed-reachable"      (reachable, main)
    //   C-orphan      "Task-Card: completed-orphan"         (then orphaned)
    //   reset --hard HEAD~1  -> main back at C-reach; C-orphan unreachable
    //   C-reported    "Task-Card: inflight-reported"        (reachable; in-flight guard still wins)
    //   C-incidental  free-text mention of completed-incidental, NO trailer (reachable)
    //   C-prefix      "Task-Card: completed-prefix-extra"   (reachable; must NOT satisfy completed-prefix)
    //   topic branch:
    //   C-sidebranch  "Task-Card: completed-sidebranch"     (reachable via --branches; NOT on main)
    commitIn(scratch.git, scratch.dir, "initial: no trailer");
    commitIn(scratch.git, scratch.dir, "land work\n\nTask-Card: completed-reachable");
    commitIn(scratch.git, scratch.dir, "orphaned work\n\nTask-Card: completed-orphan");
    scratch.git(["reset", "--hard", "HEAD~1"]); // orphan C-orphan
    commitIn(scratch.git, scratch.dir, "reported work\n\nTask-Card: inflight-reported");
    commitIn(
        scratch.git,
        scratch.dir,
        `misc: refers to completed-incidental in prose but no trailer`,
    );
    commitIn(scratch.git, scratch.dir, "land extra\n\nTask-Card: completed-prefix-extra");
    // A side branch carrying a trailer that is NOT on main. Under --branches
    // (all local tips) this IS reachable, so the card retires — pinning the
    // documented branch-generic semantic (a future narrowing to main-only
    // would break this and should be caught here).
    scratch.git(["checkout", "-q", "-b", "topic"]);
    commitIn(scratch.git, scratch.dir, "side branch work\n\nTask-Card: completed-sidebranch");
    scratch.git(["checkout", "-q", "main"]);

    try {
        // ---- case 1: completed + NO trailer -> landing_not_confirmed --------
        writeCard(isoRoot, "completed-no-trailer", { status: "completed" });
        const refused = retire("completed-no-trailer");
        assert.equal(refused.ok, false, "case 1: no-trailer completed card must be refused");
        assert.equal(refused.operation, "delete_coordination_task");
        assert.ok(refused.refusal, "case 1: refusal must be present");
        assert.equal(
            refused.refusal.code,
            "landing_not_confirmed",
            "case 1: refusal code must be landing_not_confirmed",
        );
        assert.equal(refused.refusal.status, "completed");
        assert.equal(
            refused.refusal.reason,
            "no_trailer_match",
            "case 1: reason must be no_trailer_match (git ran, 0 matches)",
        );
        assert.ok(
            refused.refusal.query.includes("Task-Card: completed-no-trailer"),
            "case 1: refusal must name the exact query + card id",
        );
        assert.equal(refused.refusal.force_required, true);
        assert.equal(cardExists(isoRoot, "completed-no-trailer"), true, "case 1: card must survive refusal");
        assert.equal(reportExists(isoRoot, "completed-no-trailer"), true, "case 1: report dir must survive refusal");

        // ---- case 2: completed + REACHABLE trailer -> DELETED --------------
        writeCard(isoRoot, "completed-reachable", { status: "completed" });
        const deleted = retire("completed-reachable");
        assert.equal(deleted.ok, true, "case 2: reachable-trailer completed card must be deleted");
        assert.equal(deleted.operation, "delete_coordination_task");
        assert.equal(deleted.removed.task_id, "completed-reachable");
        assert.equal(deleted.removed.status, "completed");
        assert.equal(deleted.removed.card_removed, true, "case 2: card must be removed");
        assert.equal(deleted.removed.report_dir_removed, true, "case 2: report dir must be removed");
        assert.equal(cardExists(isoRoot, "completed-reachable"), false, "case 2: card must be gone");
        assert.equal(reportExists(isoRoot, "completed-reachable"), false, "case 2: report dir must be gone");
        // enumeration no longer lists it
        const snap2 = listCoordinationTasks(SESSION_ID, { cwd: "/verification" });
        assert.equal(
            snap2.tasks.some((t) => t.task_id === "completed-reachable"),
            false,
            "case 2: deleted id must not appear in tasks[]",
        );

        // ---- case 3: completed + ORPHANED trailer commit -> refused --------
        // C-orphan carries the trailer but was reset away; it is reflog-only
        // (object exists, branch-unreachable). The verifier is reachability,
        // NOT object existence, so this must refuse exactly like case 1.
        writeCard(isoRoot, "completed-orphan", { status: "completed" });
        // Sanity: the orphaned commit object still exists (object-existence
        // would WRONGLY pass). Prove the reflog still has it, then prove the
        // retirement path refuses anyway.
        const reflogHas = scratch.git(["log", "-g", "--format=%H", "--grep=Task-Card: completed-orphan"]);
        assert.ok(
            reflogHas.stdout.trim().length > 0,
            "case 3 sanity: orphaned trailer commit must still exist in the reflog (object-existence would wrongly pass)",
        );
        const branchHas = scratch.git(["log", "--branches", "--format=%H", "--grep=Task-Card: completed-orphan"]);
        assert.equal(
            branchHas.stdout.trim(),
            "",
            "case 3 sanity: orphaned trailer commit must be branch-unreachable",
        );
        const orphanRefused = retire("completed-orphan");
        assert.equal(orphanRefused.ok, false, "case 3: orphaned-trailer completed card must be refused");
        assert.equal(orphanRefused.refusal.code, "landing_not_confirmed");
        assert.equal(
            orphanRefused.refusal.reason,
            "no_trailer_match",
            "case 3: reachability refusal surfaces as no_trailer_match",
        );
        assert.equal(cardExists(isoRoot, "completed-orphan"), true, "case 3: card must survive refusal");
        assert.equal(reportExists(isoRoot, "completed-orphan"), true, "case 3: report dir must survive refusal");

        // ---- case 4: reported + reachable trailer -> STILL lifecycle_state_protected
        // In-flight protection is NOT weakened: a reachable trailer commit
        // exists for this card, but the in-flight guard (4b) runs BEFORE the
        // retirement path (4c) and refuses regardless. The git query must not
        // even be reached for reported/working/blocked.
        writeCard(isoRoot, "inflight-reported", { status: "reported" });
        // Sanity: a reachable trailer commit DOES exist for this id.
        const reportedReachable = scratch.git(["log", "--branches", "--format=%H", "--grep=Task-Card: inflight-reported"]);
        assert.ok(
            reportedReachable.stdout.trim().length > 0,
            "case 4 sanity: a reachable trailer commit must exist for the reported card",
        );
        const inflightRefused = retire("inflight-reported");
        assert.equal(inflightRefused.ok, false, "case 4: reported card must be refused even with a trailer commit");
        assert.equal(
            inflightRefused.refusal.code,
            "lifecycle_state_protected",
            "case 4: in-flight guard must win (lifecycle_state_protected), not landing_not_confirmed",
        );
        assert.equal(inflightRefused.refusal.status, "reported");
        assert.equal(cardExists(isoRoot, "inflight-reported"), true, "case 4: card must survive refusal");
        assert.equal(reportExists(isoRoot, "inflight-reported"), true, "case 4: report dir must survive refusal");

        // ---- case 5: cancelled -> freely DELETED (no git check) ------------
        writeCard(isoRoot, "cancelled-free", { status: "cancelled" });
        const cancelledDeleted = retire("cancelled-free");
        assert.equal(cancelledDeleted.ok, true, "case 5: cancelled card must be freely deletable");
        assert.equal(cancelledDeleted.removed.status, "cancelled");
        assert.equal(cancelledDeleted.removed.card_removed, true);
        assert.equal(cancelledDeleted.removed.report_dir_removed, true);
        assert.equal(cardExists(isoRoot, "cancelled-free"), false);
        assert.equal(reportExists(isoRoot, "cancelled-free"), false);

        // ---- case 6: force on completed + no trailer -> DELETED (emergency)
        // Reuses the case-1 card (still present, refused without force). force
        // is the explicit emergency override — NOT the ordinary cleanup route.
        assert.equal(cardExists(isoRoot, "completed-no-trailer"), true, "case 6: case-1 card must still be present");
        const forced = retire("completed-no-trailer", { force: true });
        assert.equal(forced.ok, true, "case 6: force must override the landing gate");
        assert.equal(forced.removed.forced, true);
        assert.equal(forced.removed.status, "completed");
        assert.equal(forced.removed.card_removed, true);
        assert.equal(forced.removed.report_dir_removed, true);
        assert.equal(cardExists(isoRoot, "completed-no-trailer"), false, "case 6: card must be gone after force");
        assert.equal(reportExists(isoRoot, "completed-no-trailer"), false, "case 6: report dir must be gone after force");

        // ---- case 7: completed + incidental free-text mention (no trailer) -> refused
        // A reachable commit MENTIONS the card id in prose but does NOT carry
        // the `Task-Card: <id>` trailer. Exact-trailer discrimination: the
        // verifier matches the literal trailer line, not a free-text mention.
        writeCard(isoRoot, "completed-incidental", { status: "completed" });
        // Sanity: a reachable commit mentions the id in prose.
        const incidentalReachable = scratch.git(["log", "--branches", "--format=%H", "--grep=completed-incidental"]);
        assert.ok(
            incidentalReachable.stdout.trim().length > 0,
            "case 7 sanity: a reachable commit must mention the id in prose",
        );
        const incidentalRefused = retire("completed-incidental");
        assert.equal(incidentalRefused.ok, false, "case 7: incidental free-text mention must NOT count as a trailer");
        assert.equal(incidentalRefused.refusal.code, "landing_not_confirmed");
        assert.equal(
            incidentalRefused.refusal.reason,
            "no_trailer_match",
            "case 7: exact-trailer discrimination refuses a prose-only mention",
        );
        assert.equal(cardExists(isoRoot, "completed-incidental"), true, "case 7: card must survive refusal");
        assert.equal(reportExists(isoRoot, "completed-incidental"), true, "case 7: report dir must survive refusal");

        // ---- case 8 (F1 regression): prefix collision -> refused for the SHORT id, deleted for the EXACT id
        // A reachable commit carries `Task-Card: completed-prefix-extra`. The
        // verifier must match an EXACT trailer line, not a substring, so:
        //   - card `completed-prefix` is NOT authorized (prefix collision
        //     rejected) -> landing_not_confirmed;
        //   - card `completed-prefix-extra` IS authorized (exact line) -> deleted.
        // This is the two-stage exact-line guard's defining regression.
        writeCard(isoRoot, "completed-prefix", { status: "completed" });
        const prefixRefused = retire("completed-prefix");
        assert.equal(prefixRefused.ok, false, "case 8: prefix collision must NOT authorize the shorter id");
        assert.equal(prefixRefused.refusal.code, "landing_not_confirmed");
        assert.equal(
            prefixRefused.refusal.reason,
            "no_trailer_match",
            "case 8: a `Task-Card: completed-prefix-extra` line must not satisfy card `completed-prefix`",
        );
        assert.equal(cardExists(isoRoot, "completed-prefix"), true, "case 8: shorter-id card must survive");
        writeCard(isoRoot, "completed-prefix-extra", { status: "completed" });
        const prefixExtraDeleted = retire("completed-prefix-extra");
        assert.equal(prefixExtraDeleted.ok, true, "case 8: the exact-id card must be retired by the exact trailer line");
        assert.equal(cardExists(isoRoot, "completed-prefix-extra"), false, "case 8: exact-id card must be gone");
        assert.equal(reportExists(isoRoot, "completed-prefix-extra"), false, "case 8: exact-id report dir must be gone");

        // ---- case 9 (F2 behavior pin): trailer on an UNMERGED side branch -> DELETED under --branches
        // `--branches` walks ALL local tips, so a trailer commit on `topic`
        // (never merged to main) is reachable and authorizes retirement. This
        // pins the documented branch-generic semantic (RECORD_LIFECYCLE.md).
        writeCard(isoRoot, "completed-sidebranch", { status: "completed" });
        // Sanity: the side-branch trailer is NOT on main but IS on a branch tip.
        const onMain = scratch.git(["log", "main", "--format=%H", "--grep=Task-Card: completed-sidebranch"]);
        assert.equal(onMain.stdout.trim(), "", "case 9 sanity: side-branch trailer must NOT be on main");
        const onAnyBranch = scratch.git(["log", "--branches", "--format=%H", "--grep=Task-Card: completed-sidebranch"]);
        assert.ok(onAnyBranch.stdout.trim().length > 0, "case 9 sanity: side-branch trailer must be reachable via --branches");
        const sideDeleted = retire("completed-sidebranch");
        assert.equal(sideDeleted.ok, true, "case 9: --branches must count an unmerged side-branch trailer as reachable");
        assert.equal(cardExists(isoRoot, "completed-sidebranch"), false, "case 9: card must be gone");
        assert.equal(reportExists(isoRoot, "completed-sidebranch"), false, "case 9: report dir must be gone");

        // ---- case 10 (F3): verifier_unavailable fail-closed -> landing_not_confirmed
        // Point the verifier at a dir that is NOT a git repo (override the
        // landingProofRepo option for this one case). The catch block returns
        // proven:false / reason:verifier_unavailable, and the refusal
        // distinguishes it from no_trailer_match. The card + report dir survive.
        writeCard(isoRoot, "completed-nogit", { status: "completed" });
        const nonGitDir = fs.mkdtempSync(path.join(os.tmpdir(), "retire-nogit-"));
        try {
            const unverifiable = retire("completed-nogit", { landingProofRepo: nonGitDir });
            assert.equal(unverifiable.ok, false, "case 10: a completed card on an unverifiable surface must be refused (fail-closed)");
            assert.equal(unverifiable.refusal.code, "landing_not_confirmed");
            assert.equal(
                unverifiable.refusal.reason,
                "verifier_unavailable",
                "case 10: reason must be verifier_unavailable (distinct from no_trailer_match)",
            );
            assert.equal(cardExists(isoRoot, "completed-nogit"), true, "case 10: card must survive fail-closed refusal");
            assert.equal(reportExists(isoRoot, "completed-nogit"), true, "case 10: report dir must survive fail-closed refusal");
        } finally {
            fs.rmSync(nonGitDir, { recursive: true, force: true });
        }
    } finally {
        // Wholesale teardown: env-var restore + remove the isolated transport
        // and scratch git repo so nothing leaks into a sibling test or the real
        // coordinator registry. (A gitignored session binding under
        // .opencode/state/ from bindSessionName may persist — same pattern as
        // verify-task-registry-isolation.test.js; it is transport and harmless.)
        if (prevCoordRoot === undefined) {
            delete process.env.OPENCODE_LOCAL_COORDINATOR_ROOT;
        } else {
            process.env.OPENCODE_LOCAL_COORDINATOR_ROOT = prevCoordRoot;
        }
        if (isoRoot) {
            fs.rmSync(isoRoot, { recursive: true, force: true });
        }
        if (scratch && scratch.dir) {
            fs.rmSync(scratch.dir, { recursive: true, force: true });
        }
    }
});
