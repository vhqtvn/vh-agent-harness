// Decision-record RETIREMENT gate tests (retention preservation,
// owner-approved 2026-10-25; v2 coverage rule + review classification
// owner-approved 2026-10-26).
//
// Pins the retirement half of the gate in deleteCoordinationTask (completed
// branch, AFTER the unchanged landing-proof check):
//
//   - E2E (the owner's acceptance test): a completed card whose decision-class
//     content (consequential decision + materially-rejected alternative +
//     owner-authorization provenance) is declared via COVERING commit/artifact
//     entries (carries: field -> verbatim excerpt) retires ONLY after every
//     pointer resolves against committed, branch-reachable git state — and
//     after retirement the content is recoverable FROM SURVIVING GIT STATE
//     ONLY (fresh git processes; the card + report dir are gone), including
//     the C3 chain: a fresh session knowing ONLY the card id retrieves the
//     decision content via two bounded queries (trailer grep -> body). RED
//     against the pre-gate behavior was demonstrated on 2026-10-25 (HEAD
//     b7af07ed): the un-patched retirement deleted the only copy and no git
//     query could recover it.
//   - C1 (owner concern 2026-10-26, trivial satisfaction): v1-shape stored
//     records (bare {kind:commit} whose default card-id marker the landing
//     trailer satisfies, or marker-only entries) with non-empty decision-
//     class fields are REFUSED — coverage binds content, never presence.
//   - C1 negatives: excerpt absent from the carrier, excerpt only in an
//     un-anchored commit (anchor rule), short stored excerpt (length floor).
//   - C2 (owner concern 2026-10-26, routine-after-review burden): a routine
//     task with a clean-approve review (status "completed" — a PROCESS
//     record) retires through the REAL lifecycle with zero promotion; a
//     non-approve review verdict (blocked) still blocks a routine record,
//     naming last_review + history.task_reviewed.
//   - Negative (a): entry pointing at local report content (gitignored path,
//     no committed carrier) -> refused, item NAMED.
//   - Negative (b): artifact file committed WITHOUT the back-ref, back-ref
//     added only in the WORKING TREE -> refused, item NAMED.
//   - Negative (c1): legacy card contradicted by explicit decision-class
//     card fields -> refused, fields NAMED.
//   - Negative (d): force -> deletion proceeds AND the destructive warning
//     NAMES the decision-class content about to be destroyed.
//   - Legacy compatibility: completed card with NO decision_record retires
//     routinely when its decision-class fields are empty.
//   - Ordering pin: landing proof runs FIRST (no trailer -> landing_not_confirmed
//     even when the decision record is unresolved).
//   - Declared-revision entries: exact-sha commit + artifact verified
//     precisely (anchor + excerpts for covering entries); unresolvable and
//     orphaned revisions refused; pinned excerpt miss named.
//   - F1 option injection: dash-prefixed marker refused structurally at
//     retirement AND un-verifiable by the git layer (git grep -e searches
//     the declared tree only); dash-prefixed revision fails closed.
//   - F2: hand-edited stored record with record-level junk keys refused on
//     the RAW shape pre-normalization (junk named, not laundered).
//   - F7: routine attestation contradicted by non-empty open_questions ->
//     refused naming open_questions.
//   - F3/F4: tri-state git read helpers distinguish UNAVAILABLE reads from
//     determinate misses (test-seam exports; non-git-dir injection).
//   - b-F1 (round 3): an artifact entry whose path is a DIRECTORY is a
//     determinate miss NAMED as a directory (path_is_directory_not_file_blob),
//     never verified from the `git show <sha>:<dir>` LISTING — a card-id-NAMED
//     file inside the directory must not verify the marker (filename bait);
//     pinned both through full retirement (card + report survive) and at the
//     committedBlobAt seam (incl. trailing-slash directory pathspecs).
//   - b-F2 (round 3): artifact search-mode walk with an UNAVAILABLE read and
//     no verifying carrier diagnoses verifier_unavailable (incomplete
//     negative search), never an overstated marker_not_in_committed_blob.
//
// Test seam (NOT an env var): mirrors tests/scripts/task-delete-retirement.test.js —
// the retirement verifier's repo is redirected at a hermetic scratch git repo
// via the `landingProofRepo` OPTION (not forwarded by plan-state.js).
//
// Run:  vh-agent-harness exec node --test tests/scripts/task-retire-decision-record.test.js
//       (or: node --test tests/scripts/task-retire-decision-record.test.js)

import { test } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import os from "node:os";
import { fileURLToPath, pathToFileURL } from "node:url";
import { spawnSync } from "node:child_process";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = path.resolve(__dirname, "..", "..");

// Import state-lib from the RENDERED tree (.opencode/scripts/). Run
// `make update` after editing templates/core/ before this test.
const STATE_LIB = path.join(REPO_ROOT, ".opencode", "scripts", "state-lib.js");
const {
    bindSessionName,
    saveCoordinationTask,
    readyCoordinationTask,
    activateCoordinationTask,
    saveCoordinationTaskCloseout,
    reviewCoordinationTask,
    deleteCoordinationTask,
    verifyDecisionRecordForRetirement,
    commitMessageContains,
    committedBlobAt,
    computeTaskDesignDigest,
} = await import(pathToFileURL(STATE_LIB).href);

// --- hermetic scratch git repo helpers (mirrors task-delete-retirement) ------

function makeScratchRepo() {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "dr-retire-"));
    const run = (args) =>
        spawnSync("git", ["-C", dir, ...args], { encoding: "utf8" });
    let r = run(["init", "-q", "-b", "main"]);
    if (r.status !== 0) {
        r = run(["init", "-q"]);
        if (r.status !== 0) {
            throw new Error(`scratch git init failed: ${r.stderr || r.stdout}`);
        }
        run(["branch", "-M", "main"]);
    }
    run(["config", "user.email", "dr-retire@test"]);
    run(["config", "user.name", "dr-retire-test"]);
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
    // gitRaw: non-throwing (git grep exits 1 on no-match — an EXPECTED
    // outcome in the recovery assertions).
    const gitRaw = (args) => run(args);
    return { dir, git, gitRaw };
}

// Stage the current state of `dir` (added/modified files) under `message`.
// Always stages a unique new file first so every commit is a real tree change
// git will accept (mirrors commitIn in task-delete-retirement.test.js).
function commitAll(git, dir, message) {
    const name = `f-${process.pid}-${Math.random().toString(36).slice(2)}.txt`;
    fs.writeFileSync(path.join(dir, name), `content ${name}\n`);
    git(["add", "-A"]);
    git(["commit", "-q", "-m", message]);
    return git(["rev-parse", "HEAD"]).stdout.trim();
}

// --- isolated transport helpers (mirrors the sibling suites) -----------------

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
                schema_version: 1,
                task_id: id,
                title: `retirement fixture ${id}`,
                task_type: "implementation",
                coordination_mode: "short",
                primary_lane: "harness-core",
                status: "draft",
                active_session_alias: null,
                ...fields,
            },
            null,
            2,
        ),
    );
    fs.mkdirSync(reportDir(isoRoot, id), { recursive: true });
    fs.writeFileSync(
        path.join(reportDir(isoRoot, id), "closeout.md"),
        `# evidence for ${id}\n`,
    );
}
function cardExists(isoRoot, id) {
    return fs.existsSync(cardPath(isoRoot, id));
}
function reportExists(isoRoot, id) {
    return fs.existsSync(reportDir(isoRoot, id));
}
// Read back the stored card JSON (lenient).
function readCard(isoRoot, id) {
    return JSON.parse(fs.readFileSync(cardPath(isoRoot, id), "utf8"));
}
function patchCard(isoRoot, id, patch) {
    const card = readCard(isoRoot, id);
    fs.writeFileSync(
        cardPath(isoRoot, id),
        JSON.stringify({ ...card, ...patch }, null, 2),
    );
}

const SESSION_ID = "dr-retire-session";
const ALIAS = "dr-retire";

// A complete valid working card so saveCoordinationTaskCloseout passes.
function writeWorkingCard(isoRoot, id, extra = {}) {
    writeCard(isoRoot, id, {
        status: "working",
        active_session_alias: ALIAS,
        claimed_at: "2026-10-25T00:00:00Z",
        report_envelope: "minimal",
        files_in_scope: ["tests/scripts/task-retire-decision-record.test.js"],
        success_criteria: ["retirement gate pins"],
        validation_plan: [
            "node --test tests/scripts/task-retire-decision-record.test.js",
        ],
        session_aliases: [ALIAS],
        ...extra,
    });
}

function closeout(id, entries, options = {}) {
    const body =
        "# Closeout\n\n```decision-record\n" +
        JSON.stringify({ version: 1, entries }, null, 2) +
        "\n```\n";
    return saveCoordinationTaskCloseout(SESSION_ID, id, {
        cwd: "/verification",
        title: "decision-record closeout",
        body,
        taskStatus: "completed",
        reportEnvelope: "minimal",
        ...options,
    });
}

test("decision-record retirement gate: E2E recovery + negatives + legacy compat + force", () => {
    const isoRoot = fs.mkdtempSync(path.join(os.tmpdir(), "dr-retire-iso-"));
    const prevCoordRoot = process.env.OPENCODE_LOCAL_COORDINATOR_ROOT;
    process.env.OPENCODE_LOCAL_COORDINATOR_ROOT = isoRoot;
    bindSessionName(SESSION_ID, ALIAS);
    const scratch = makeScratchRepo();

    const retire = (id, opts = {}) =>
        deleteCoordinationTask(SESSION_ID, id, {
            cwd: "/verification",
            landingProofRepo: scratch.dir,
            ...opts,
        });

    try {
        commitAll(scratch.git, scratch.dir, "initial: no trailer");

        // =====================================================================
        // E2E — the owner's acceptance test (GREEN on the gated path).
        // Retention gate v2 (owner-approved 2026-10-26): the commit entry
        // COVERS owner_notes via a verbatim excerpt from the field's own
        // content (carries), and the artifact entry covers the measured
        // outcome — the post-landing material pattern. Marker-only entries
        // cover nothing; a bare {kind:commit} is refused when decision-class
        // fields are non-empty (see the C1 regression below).
        // =====================================================================
        const ID = "dr-e2e";
        const DECISION = "DECISION dr-e2e: adopt the retention-preservation gate";
        const REJECTED = "REJECTED ALTERNATIVE dr-e2e: dual-ledger archive (violates the no-new-ledger constraint)";
        const OWNER_AUTH = "OWNER-AUTH dr-e2e: owner approved 2026-10-25, five conditions";
        const MEASURED = "Measured outcome: gate refuses unpromoted retirement.";
        const ARTIFACT_REL = "docs/checkpoints/2026-10-25-dr-e2e.md";

        // Card carries the decision-class provenance up front.
        writeWorkingCard(isoRoot, ID, {
            owner_notes: [`${OWNER_AUTH}; ${REJECTED}`],
        });

        // Closeout declares BOTH carriers BEFORE they exist (revision omitted
        // = search mode — the landing commit cannot be sha-pinned at closeout)
        // and binds CONTENT: each covered field's verbatim excerpt.
        closeout(
            ID,
            [
                {
                    kind: "commit",
                    carries: { owner_notes: OWNER_AUTH },
                    note: "authorization + decision + rejected alternative in the landing commit body",
                },
                {
                    kind: "artifact",
                    path: ARTIFACT_REL,
                    carries: { measured_outcome: MEASURED },
                    note: "post-landing measured outcome in a durable file",
                },
            ],
            { measuredOutcome: MEASURED },
        );

        // Landing commit: trailer + ALL decision content in the BODY.
        fs.writeFileSync(path.join(scratch.dir, "e2e.txt"), "landed\n");
        const landingSha = commitAll(
            scratch.git,
            scratch.dir,
            `land retention gate\n\nTask-Card: ${ID}\n\n${DECISION}\n${REJECTED}\n${OWNER_AUTH}\n`,
        );
        // Promoted durable file, committed WITH the card-id back-reference
        // (and the decision content + measured outcome it preserves). The
        // measured-outcome line is WRAPPED across two lines in the doc — the
        // excerpt check is whitespace-normalized, so wrapped carriers still
        // verify.
        fs.mkdirSync(path.dirname(path.join(scratch.dir, ARTIFACT_REL)), {
            recursive: true,
        });
        fs.writeFileSync(
            path.join(scratch.dir, ARTIFACT_REL),
            `# Retention gate decision record\n\nTask-Card: ${ID}\n\n${DECISION}\n${REJECTED}\nMeasured outcome: gate refuses unpromoted\nretirement.\n`,
        );
        const artifactSha = commitAll(
            scratch.git,
            scratch.dir,
            `docs: promote ${ID} decision record\n\nTask-Card: ${ID}\n`,
        );

        // Retire: landing proven + every pointer resolved + coverage
        // content-bound -> DELETED.
        const done = retire(ID);
        assert.equal(done.ok, true, `E2E retire should succeed: ${JSON.stringify(done)}`);
        assert.equal(cardExists(isoRoot, ID), false, "E2E: card must be gone");
        assert.equal(reportExists(isoRoot, ID), false, "E2E: report dir must be gone");

        // RECOVERY — from SURVIVING GIT STATE ONLY, in FRESH git processes
        // (each spawnSync is a new process; no in-memory or local-file state
        // remains — the transport copy was just deleted). Every assertion is
        // an OUTPUT-CONTAINS check on the exact recovered strings: `git log`
        // exits 0 even on NO match, so a bare exit-status check passes
        // vacuously. Artifact content is recovered from the COMMITTED blob
        // via `git show <reachable-sha>:<path>` — a working-tree `git grep`
        // would read the checkout, not surviving git state (and the
        // non-vacuity fixture at the end of this block proves these
        // assertions can fail).
        const logBodies = scratch.git([
            "log",
            "--branches",
            "--format=%B",
            `--grep=dr-e2e`,
        ]).stdout;
        assert.ok(
            logBodies.includes(`Task-Card: ${ID}`),
            "E2E recovery: trailer reachable",
        );
        const containsLanding = scratch.gitRaw([
            "branch",
            "--contains",
            landingSha,
        ]);
        assert.ok(
            containsLanding.status === 0 &&
                containsLanding.stdout.trim() !== "",
            "E2E recovery: the landing commit itself remains branch-reachable (reachability, not object existence)",
        );
        // C3 (owner concern, 2026-10-26): a FRESH session knowing ONLY the
        // card id must retrieve the decision content via BOUNDED git
        // queries. The chain: (1) trailer grep -> landing sha; (2) read that
        // commit's body -> the anchored excerpt (verbatim owner-decision
        // content) is right there. The card and its decision_record are
        // destroyed; the landing commit body is the self-contained anchor.
        const c3Shas = scratch
            .git([
                "log",
                "--branches",
                "--fixed-strings",
                "--format=%H",
                `--grep=Task-Card: ${ID}`,
            ])
            .stdout.trim()
            .split(/\r?\n/)
            .filter(Boolean);
        assert.ok(
            c3Shas.length >= 1,
            "C3 retrieval: card-id trailer grep must find at least one carrier commit",
        );
        // Bounded chain: read each trailer hit's body until the decision
        // content is found (a fresh session cannot know WHICH trailer-
        // bearing commit carries it — the chain is grep, then per-hit body
        // reads, and the hit count is small by construction).
        const c3Bodies = c3Shas.map((sha) =>
            scratch.git(["log", "-1", "--format=%B", sha]).stdout,
        );
        const c3Joined = c3Bodies.join("\n--\n");
        assert.ok(
            c3Joined.includes(OWNER_AUTH),
            "C3 retrieval: owner-authorization provenance recoverable from a trailer-hit commit body (card-id only, bounded queries)",
        );
        assert.ok(
            c3Joined.includes(REJECTED),
            "C3 retrieval: materially-rejected alternative recoverable from a trailer-hit commit body",
        );
        assert.ok(
            c3Joined.includes(DECISION),
            "C3 retrieval: consequential decision recoverable from a trailer-hit commit body",
        );
        const ownerAuthRecovered = scratch.gitRaw([
            "log",
            "--branches",
            "--format=%B",
            "--fixed-strings",
            `--grep=${OWNER_AUTH}`,
        ]);
        assert.ok(
            ownerAuthRecovered.status === 0 &&
                ownerAuthRecovered.stdout.includes(OWNER_AUTH),
            "E2E recovery: owner-authorization provenance retrievable from a reachable commit body (output contains the exact string)",
        );
        const rejectedLog = scratch.gitRaw([
            "log",
            "--branches",
            "--format=%B",
            "--fixed-strings",
            `--grep=${REJECTED}`,
        ]);
        assert.ok(
            rejectedLog.status === 0 && rejectedLog.stdout.includes(REJECTED),
            "E2E recovery: materially-rejected alternative retrievable from a reachable commit body (output contains the exact string)",
        );
        const decisionLog = scratch.gitRaw([
            "log",
            "--branches",
            "--format=%B",
            "--fixed-strings",
            `--grep=${DECISION}`,
        ]);
        assert.ok(
            decisionLog.status === 0 && decisionLog.stdout.includes(DECISION),
            "E2E recovery: consequential decision retrievable from a reachable commit body (output contains the exact string)",
        );
        const artifactBlob = scratch.gitRaw([
            "show",
            `${artifactSha}:${ARTIFACT_REL}`,
        ]);
        assert.ok(
            artifactBlob.status === 0 &&
                artifactBlob.stdout.includes(`Task-Card: ${ID}`) &&
                artifactBlob.stdout.includes(DECISION) &&
                artifactBlob.stdout.includes(REJECTED) &&
                artifactBlob.stdout.includes(
                    "Measured outcome: gate refuses unpromoted",
                ),
            "E2E recovery: durable-file content (card-id back-ref + decision + rejected alternative + measured outcome) retrievable from the COMMITTED blob via git show <reachable-sha>:<path>",
        );
        // C3 artifact chain (bounded): a fresh session that does not know
        // the artifact path finds the promoted content via the card-id
        // pickaxe: the commit whose tree INTRODUCED the card-id string.
        const pickaxe = scratch.gitRaw([
            "log",
            "--branches",
            "--format=%H",
            `-S${ID}`,
        ]);
        assert.ok(
            pickaxe.status === 0 &&
                pickaxe.stdout
                    .trim()
                    .split(/\r?\n/)
                    .filter(Boolean)
                    .includes(artifactSha),
            "C3 retrieval: card-id pickaxe (-S) finds the commit that introduced the anchored artifact — the bounded discoverability chain for file-carried content",
        );

        // NON-VACUITY fixture (negative control): the same OUTPUT-CONTAINS
        // recovery queries run against a repo where the content was NEVER
        // committed (trailer landing only) must come back WITHOUT the
        // strings — proving the assertions above distinguish presence from
        // absence. A vacuous exit-0 check would "pass" here; these
        // output-contains checks do not.
        {
            const bare = makeScratchRepo();
            try {
                commitAll(
                    bare.git,
                    bare.dir,
                    `land dr-e2e with no preserved content\n\nTask-Card: dr-e2e\n`,
                );
                const recoveryHit = (needle) => {
                    const res = bare.gitRaw([
                        "log",
                        "--branches",
                        "--format=%B",
                        "--fixed-strings",
                        `--grep=${needle}`,
                    ]);
                    return res.status === 0 && res.stdout.includes(needle);
                };
                for (const needle of [DECISION, REJECTED, OWNER_AUTH]) {
                    assert.equal(
                        recoveryHit(needle),
                        false,
                        `non-vacuity: recovery assertion must FAIL where the content was never committed (${needle.slice(0, 32)}…)`,
                    );
                }
            } finally {
                fs.rmSync(bare.dir, { recursive: true, force: true });
            }
        }

        // =====================================================================
        // Negative (a): inline-only — entry pointing at LOCAL REPORT content
        // (gitignored transport path, no committed carrier) -> refused, item
        // NAMED. There is no inline kind; an artifact entry whose path never
        // lands in git is the closest reachable shape.
        // =====================================================================
        {
            const id = "dr-neg-inline";
            writeWorkingCard(isoRoot, id);
            closeout(id, [
                {
                    kind: "artifact",
                    path: ".local/coordinator/reports/" + id + "/closeout.md",
                    note: "content only in the local report",
                },
            ]);
            commitAll(scratch.git, scratch.dir, `land ${id}\n\nTask-Card: ${id}\n`);
            const refused = retire(id);
            assert.equal(refused.ok, false, "(a) inline-only must be refused");
            assert.equal(
                refused.refusal.code,
                "decision_record_unresolved",
                `(a) refusal code; got ${JSON.stringify(refused.refusal)}`,
            );
            const msg = refused.refusal.message;
            assert.ok(
                msg.includes("entry #1") && msg.includes("artifact"),
                `(a) refusal must NAME the entry; got: ${msg}`,
            );
            assert.ok(
                msg.includes("path_never_committed") ||
                    msg.includes("no reachable commit carries"),
                `(a) refusal must say the carrier was never committed; got: ${msg}`,
            );
            assert.equal(cardExists(isoRoot, id), true, "(a) card survives");
            assert.equal(reportExists(isoRoot, id), true, "(a) report survives");
        }

        // =====================================================================
        // Negative (b): artifact file COMMITTED without the back-ref; the
        // back-ref exists only in the WORKING TREE (uncommitted edit) ->
        // refused, item NAMED.
        // =====================================================================
        {
            const id = "dr-neg-worktree";
            const rel = "docs/checkpoints/2026-10-25-dr-neg-worktree.md";
            writeWorkingCard(isoRoot, id);
            closeout(id, [{ kind: "artifact", path: rel }]);
            // Commit the durable file WITHOUT the card-id back-reference.
            fs.mkdirSync(path.dirname(path.join(scratch.dir, rel)), {
                recursive: true,
            });
            fs.writeFileSync(
                path.join(scratch.dir, rel),
                "# checkpoint\n\ndecision prose with NO back-reference yet\n",
            );
            commitAll(scratch.git, scratch.dir, `docs: checkpoint without backref\n\nTask-Card: ${id}\n`);
            // Uncommitted working-tree edit ADDS the back-ref.
            fs.appendFileSync(
                path.join(scratch.dir, rel),
                `\nTask-Card: ${id}\n`,
            );
            const refused = retire(id);
            assert.equal(refused.ok, false, "(b) working-tree-only back-ref must be refused");
            assert.equal(refused.refusal.code, "decision_record_unresolved");
            const msg = refused.refusal.message;
            assert.ok(
                msg.includes("marker_not_in_committed_blob"),
                `(b) refusal must name marker_not_in_committed_blob; got: ${msg}`,
            );
            assert.ok(
                msg.includes("working tree") || msg.includes("committed blob"),
                `(b) refusal must say working-tree content never counts; got: ${msg}`,
            );
            assert.equal(cardExists(isoRoot, id), true, "(b) card survives");
            // Restore the scratch worktree so later cases start clean.
            scratch.git(["checkout", "--", rel]);
        }

        // =====================================================================
        // Negative (b3) — b-F1 regression (round 3): artifact entry whose
        // path IS A DIRECTORY containing a card-id-NAMED file. `git show
        // <sha>:<dir>` exits 0 and prints the directory LISTING, so the
        // pre-fix blob read "verified" the default marker (the card id)
        // from a FILENAME in that listing and retirement DESTROYED the
        // card without ever reading blob content (confirmed live on
        // 2026-10-26). Must now be refused NAMING the directory-not-file
        // problem; card + report survive.
        // =====================================================================
        {
            const id = "dr-neg-dir";
            const dirRel = "docs/coordination";
            writeWorkingCard(isoRoot, id);
            closeout(id, [
                {
                    kind: "artifact",
                    path: dirRel,
                    note: "declared a directory by mistake",
                },
            ]);
            // The directory contains a file NAMED exactly like the card id
            // (the default marker) — the listing bait.
            fs.mkdirSync(path.join(scratch.dir, dirRel), { recursive: true });
            fs.writeFileSync(
                path.join(scratch.dir, dirRel, id),
                "filename bait — committed content carries no marker\n",
            );
            commitAll(
                scratch.git,
                scratch.dir,
                `docs: directory with id-named file\n\nTask-Card: ${id}\n`,
            );
            const refused = retire(id);
            assert.equal(
                refused.ok,
                false,
                "(b3) directory-as-path must NOT verify from a directory listing",
            );
            assert.equal(refused.refusal.code, "decision_record_unresolved");
            const msg = refused.refusal.message;
            assert.ok(
                msg.includes("path_is_directory_not_file_blob"),
                `(b3) refusal must name path_is_directory_not_file_blob; got: ${msg}`,
            );
            assert.ok(
                msg.includes("is a directory") && msg.includes(dirRel),
                `(b3) refusal must name the directory-not-file problem at ${dirRel}; got: ${msg}`,
            );
            assert.equal(cardExists(isoRoot, id), true, "(b3) card survives");
            assert.equal(reportExists(isoRoot, id), true, "(b3) report survives");
        }

        // =====================================================================
        // Negative (c1): LEGACY completed card, no decision_record, but
        // owner_notes + measured_outcome non-empty -> refused, BOTH named.
        // =====================================================================
        {
            const id = "dr-neg-legacy";
            writeCard(isoRoot, id, {
                status: "completed",
                owner_notes: ["OWNER APPROVAL 2026-10-25: approved."],
                measured_outcome: "gate refuses unpromoted retirement",
            });
            commitAll(scratch.git, scratch.dir, `land ${id}\n\nTask-Card: ${id}\n`);
            const refused = retire(id);
            assert.equal(refused.ok, false, "(c1) legacy unpromoted must be refused");
            assert.equal(refused.refusal.code, "decision_content_unpromoted");
            assert.deepEqual(
                refused.refusal.fields.sort(),
                ["measured_outcome", "owner_notes"],
                `(c1) refusal must name both contradicting fields; got ${JSON.stringify(refused.refusal.fields)}`,
            );
            assert.equal(cardExists(isoRoot, id), true, "(c1) card survives");
        }

        // =====================================================================
        // C1 regression (owner concern, 2026-10-26 — TRIVIAL SATISFACTION).
        // Card whose owner_notes carry a consequential owner decision (with
        // provenance) + an important rejected alternative; the stored
        // decision_record is the v1 in-flight shape — a BARE {kind:"commit"}
        // entry whose default marker (the card id) the landing trailer itself
        // satisfies. Pre-v2 (HEAD b788b438, reproduced 2026-10-26) this
        // RETIRED and the reasoning was unrecoverable (no message search or
        // pickaxe found it). V2: the entry covers nothing -> retirement
        // refused naming owner_notes; card + report survive. The same holds
        // for a v1 MARKER-ONLY entry (an explicit marker proves presence of
        // an arbitrary string, never that the field's content survived).
        // =====================================================================
        {
            const decisionText =
                "OWNER 2026-10-26 (chat m0051): approved the bounded-correction path for the retention gate";
            const rejectedText =
                "REJECTED ALTERNATIVE: dual-ledger archive — violates the no-new-ledger constraint";
            // In-flight v1 cards: closed out under the pre-v2 gate (whose
            // closeout accepted these shapes), carrying the stored record as
            // written then, now retiring under v2. A fresh v2 closeout
            // refuses these shapes at declaration (pinned in the closeout
            // suite); this is the retirement-side pin for the in-flight
            // population.
            const cases = [
                { id: "dr-c1-bare", entries: [{ kind: "commit" }] },
                {
                    id: "dr-c1-marker",
                    entries: [
                        { kind: "commit", marker: "dr-c1-marker decision" },
                    ],
                    // The marker RESOLVES (arbitrary string present in the
                    // landing body) — the exact trivial-satisfaction shape:
                    // presence verified, content not covered.
                    landingBody: `land dr-c1-marker\n\nTask-Card: dr-c1-marker\n\ndr-c1-marker decision\n`,
                },
            ];
            for (const { id, entries, landingBody } of cases) {
                writeCard(isoRoot, id, {
                    status: "completed",
                    owner_notes: [decisionText, rejectedText],
                    decision_record: { version: 1, entries },
                });
                // Landing commit carries the trailer (and, for the marker
                // case, the marker string) but NO decision content.
                commitAll(
                    scratch.git,
                    scratch.dir,
                    landingBody || `land ${id}\n\nTask-Card: ${id}\n`,
                );
                const refused = retire(id);
                assert.equal(
                    refused.ok,
                    false,
                    `C1 (${id}): trailer-only landing must NOT satisfy a record with uncovered owner_notes`,
                );
                assert.equal(
                    refused.refusal.code,
                    "decision_content_unpromoted",
                    `C1 (${id}): refusal code; got ${JSON.stringify(refused.refusal)}`,
                );
                assert.ok(
                    Array.isArray(refused.refusal.fields) &&
                        refused.refusal.fields.includes("owner_notes"),
                    `C1 (${id}): refusal must name owner_notes; got ${JSON.stringify(refused.refusal.fields)}`,
                );
                assert.equal(cardExists(isoRoot, id), true, `C1 (${id}): card survives`);
                assert.equal(
                    reportExists(isoRoot, id),
                    true,
                    `C1 (${id}): report survives`,
                );
            }
        }

        // =====================================================================
        // C1 negatives (v2): coverage binds CONTENT.
        //   (n1) carries excerpt declared, closeout-bound, but the landing
        //        carrier never carries it -> decision_record_unresolved /
        //        excerpt_not_in_carrier, field NAMED.
        //   (n2) excerpt present ONLY in an un-anchored commit message (no
        //        card id) -> refused: every covering carrier must be
        //        discoverable from the card id (anchor rule).
        //   (n3) hand-edited stored record with a SHORT excerpt (< floor) ->
        //        refused by the raw structural re-check, naming the floor.
        // =====================================================================
        {
            const id = "dr-c1n-absent";
            writeWorkingCard(isoRoot, id, {
                owner_notes: [
                    "OWNER 2026-10-26: approved the search-mode carve-out for legacy pointers",
                ],
            });
            closeout(id, [
                {
                    kind: "commit",
                    carries: {
                        owner_notes:
                            "approved the search-mode carve-out for legacy pointers",
                    },
                },
            ]);
            commitAll(scratch.git, scratch.dir, `land ${id}\n\nTask-Card: ${id}\n`);
            const refused = retire(id);
            assert.equal(refused.ok, false, "(n1) absent excerpt must refuse");
            assert.equal(
                refused.refusal.code,
                "decision_record_unresolved",
                `(n1) refusal code; got ${JSON.stringify(refused.refusal.code)}`,
            );
            const problems = (refused.refusal.problems || []).map(
                (p) => `${p.code}`,
            );
            assert.ok(
                problems.includes("excerpt_not_in_carrier"),
                `(n1) refusal must name excerpt_not_in_carrier; got ${JSON.stringify(problems)}`,
            );
            assert.ok(
                refused.refusal.message.includes("owner_notes"),
                `(n1) refusal must name the uncovered/missed field; got: ${refused.refusal.message}`,
            );
            assert.equal(cardExists(isoRoot, id), true, "(n1) card survives");

            const id2 = "dr-c1n-unanchored";
            writeWorkingCard(isoRoot, id2, {
                owner_notes: [
                    "OWNER 2026-10-26: approved anchoring every carrier to the card id",
                ],
            });
            closeout(id2, [
                {
                    kind: "commit",
                    carries: {
                        owner_notes: "approved anchoring every carrier to the card id",
                    },
                },
            ]);
            // The excerpt lands in a commit whose message carries NO card id.
            commitAll(
                scratch.git,
                scratch.dir,
                "unrelated commit mentioning: approved anchoring every carrier to the card id\n",
            );
            commitAll(
                scratch.git,
                scratch.dir,
                `land ${id2}\n\nTask-Card: ${id2}\n`,
            );
            const refused2 = retire(id2);
            assert.equal(
                refused2.ok,
                false,
                "(n2) excerpt in an un-anchored commit must NOT verify coverage",
            );
            assert.equal(
                refused2.refusal.code,
                "decision_record_unresolved",
                `(n2) refusal code; got ${JSON.stringify(refused2.refusal.code)}`,
            );
            assert.ok(
                (refused2.refusal.problems || []).some(
                    (p) => p.code === "excerpt_not_in_carrier",
                ),
                `(n2) refusal must name excerpt_not_in_carrier (anchor rule); got ${JSON.stringify(refused2.refusal.problems)}`,
            );
            assert.equal(cardExists(isoRoot, id2), true, "(n2) card survives");

            const id3 = "dr-c1n-short";
            writeCard(isoRoot, id3, {
                status: "completed",
                owner_notes: [
                    "OWNER 2026-10-26: short-excerpt structural refusal case",
                ],
                decision_record: {
                    version: 1,
                    entries: [
                        {
                            kind: "commit",
                            carries: { owner_notes: "too short excerpt" },
                        },
                    ],
                },
            });
            commitAll(
                scratch.git,
                scratch.dir,
                `land ${id3}\n\nTask-Card: ${id3}\n`,
            );
            const refused3 = retire(id3);
            assert.equal(
                refused3.ok,
                false,
                "(n3) short excerpt must be refused by the structural re-check",
            );
            assert.equal(refused3.refusal.code, "decision_record_unresolved");
            assert.ok(
                refused3.refusal.message.includes("at least") &&
                    refused3.refusal.message.includes("characters"),
                `(n3) refusal must name the length floor; got: ${refused3.refusal.message}`,
            );
            assert.equal(cardExists(isoRoot, id3), true, "(n3) card survives");
        }

        // =====================================================================
        // C2 regression (owner concern, 2026-10-26 — ROUTINE-AFTER-REVIEW
        // BURDEN), via the REAL sanctioned lifecycle ops: create -> ready ->
        // working -> closeout (routine record) -> clean-approve review ->
        // landing -> retirement. Pre-v2 (reproduced 2026-10-26 against
        // b788b438) this REFUSED: last_review + history.task_reviewed were
        // unconditionally decision-class, so EVERY reviewed routine task
        // needed promotion or force. V2 classification: a review that
        // resolved the card to "completed" is a PROCESS record — routine
        // tasks retire after a normal review with zero promotion burden.
        // =====================================================================
        {
            const created = saveCoordinationTask(SESSION_ID, {
                title: "Routine follow-up: dependency bump",
                task_type: "implementation",
                coordination_mode: "short",
                primary_lane: "harness-core",
                status: "draft",
                rough_scope: ["go.mod", "go.sum"],
                files_in_scope: ["go.mod"],
                success_criteria: ["go test ./... green after the bump"],
                validation_plan: ["go test ./..."],
                cwd: "/verification",
            });
            const id = created.task.task_id;
            const digest = computeTaskDesignDigest(created.task, {});
            readyCoordinationTask(SESSION_ID, id, {
                f3_design_readiness: {
                    ownership_hazards: [],
                    design_digest: digest,
                },
                cwd: "/verification",
            });
            activateCoordinationTask(SESSION_ID, id, { cwd: "/verification" });
            closeout(
                id,
                [{ kind: "routine", note: "nothing decision-class to preserve" }],
                { taskStatus: "reported" },
            );
            const review = reviewCoordinationTask(SESSION_ID, id, {
                body: "Approve. Routine dependency bump; tests green; nothing decision-class.",
                taskStatus: "completed",
                title: "Coordinator Review",
                cwd: "/verification",
            });
            assert.equal(
                review.task.status,
                "completed",
                "C2 setup: clean-approve review resolves the card to completed",
            );
            commitAll(scratch.git, scratch.dir, `land ${id}\n\nTask-Card: ${id}\n`);
            const done = retire(id);
            assert.equal(
                done.ok,
                true,
                `C2: routine task with a clean-approve review must retire WITHOUT promotion; got ${JSON.stringify(done.refusal || done)}`,
            );
            assert.equal(
                cardExists(isoRoot, id),
                false,
                "C2: reviewed routine card is gone (process record, not decision-class)",
            );
        }

        // =====================================================================
        // Negative (c2): routine record contradicted by a NON-APPROVE review
        // verdict (last_review.status = blocked + task_reviewed history
        // events with non-completed statuses) -> refused, BOTH named. The
        // clean-approve shape (status "completed") is the FLIP pin: the same
        // fixture with a completed-status review RETIRES (see the C2 real-
        // lifecycle regression above; pinned here as the minimal card shape).
        // =====================================================================
        {
            const id = "dr-neg-postreview";
            writeWorkingCard(isoRoot, id);
            closeout(id, [{ kind: "routine", note: "nothing decision-class" }]);
            patchCard(isoRoot, id, {
                last_review: {
                    path: `.local/coordinator/reports/${id}/review.md`,
                    reviewed_at: "2026-10-26T00:00:00Z",
                    title: "Coordinator Review",
                    status: "blocked",
                    summary: "verdict: consequential objection stands; rework required",
                    next_action: "rework",
                },
                history: [
                    ...(readCard(isoRoot, id).history || []),
                    {
                        at: "2026-10-26T00:00:00Z",
                        event: "task_reviewed",
                        status: "blocked",
                        note: "verdict recorded: consequential objection stands",
                    },
                ],
            });
            commitAll(scratch.git, scratch.dir, `land ${id}\n\nTask-Card: ${id}\n`);
            const refused = retire(id);
            assert.equal(refused.ok, false, "(c2) post-closeout non-approve review verdict must block routine retirement");
            assert.equal(refused.refusal.code, "decision_content_unpromoted");
            assert.ok(
                refused.refusal.fields.includes("last_review") &&
                    refused.refusal.fields.includes("history.task_reviewed"),
                `(c2) refusal must name last_review AND history.task_reviewed; got ${JSON.stringify(refused.refusal.fields)}`,
            );
            assert.equal(cardExists(isoRoot, id), true, "(c2) card survives");

            // FLIP pin: the identical card shape with a CLEAN-APPROVE review
            // (status "completed" — process record) retires routinely.
            const idOk = "dr-neg-postreview-approve";
            writeWorkingCard(isoRoot, idOk);
            closeout(idOk, [{ kind: "routine", note: "nothing decision-class" }]);
            patchCard(isoRoot, idOk, {
                last_review: {
                    path: `.local/coordinator/reports/${idOk}/review.md`,
                    reviewed_at: "2026-10-26T00:00:00Z",
                    title: "Coordinator Review",
                    status: "completed",
                    summary: "Approve. Routine work; nothing decision-class.",
                    next_action: "retire",
                },
                history: [
                    ...(readCard(isoRoot, idOk).history || []),
                    {
                        at: "2026-10-26T00:00:00Z",
                        event: "task_reviewed",
                        status: "completed",
                        note: "clean approve recorded",
                    },
                ],
            });
            commitAll(
                scratch.git,
                scratch.dir,
                `land ${idOk}\n\nTask-Card: ${idOk}\n`,
            );
            const okRetire = retire(idOk);
            assert.equal(
                okRetire.ok,
                true,
                `(c2 flip) clean-approve review is a process record; got ${JSON.stringify(okRetire.refusal || okRetire)}`,
            );
            assert.equal(
                cardExists(isoRoot, idOk),
                false,
                "(c2 flip) reviewed routine card retired",
            );
        }

        // =====================================================================
        // Negative (d): FORCE — deletion proceeds AND the destructive warning
        // NAMES the decision-class content (fields + entries), not boilerplate.
        // =====================================================================
        {
            const id = "dr-neg-force";
            writeCard(isoRoot, id, {
                status: "completed",
                owner_notes: ["OWNER APPROVAL 2026-10-25: approved the retention fix."],
                decision_record: {
                    version: 1,
                    entries: [
                        {
                            kind: "artifact",
                            path: "docs/checkpoints/never-committed.md",
                            note: "declared but never promoted",
                        },
                    ],
                },
            });
            // NOTE: no trailer commit for this id — force bypasses landing too.
            const forced = retire(id, { force: true });
            assert.equal(forced.ok, true, "(d) force must proceed");
            assert.equal(cardExists(isoRoot, id), false, "(d) card gone");
            assert.equal(reportExists(isoRoot, id), false, "(d) report gone");
            const destroyed = forced.removed.decision_content_destroyed;
            assert.ok(destroyed, "(d) force result must carry the destruction inventory");
            assert.ok(
                destroyed.fields.some((f) => f.field === "owner_notes"),
                `(d) inventory must name owner_notes; got ${JSON.stringify(destroyed.fields)}`,
            );
            assert.ok(
                destroyed.entries.length === 1 &&
                    destroyed.entries[0].kind === "artifact" &&
                    destroyed.entries[0].locator.includes("never-committed.md"),
                `(d) inventory must name the declared entry; got ${JSON.stringify(destroyed.entries)}`,
            );
            const warning = forced.removed.destructive_warning;
            assert.ok(
                warning &&
                    warning.includes("owner_notes") &&
                    warning.includes("OWNER APPROVAL 2026-10-25") &&
                    warning.includes("never-committed.md") &&
                    warning.includes("LAST LOCAL COPY"),
                `(d) destructive warning must NAME the content; got: ${warning}`,
            );
        }

        // =====================================================================
        // Legacy compatibility POSITIVE: completed card with NO
        // decision_record and NO decision-class fields -> routine retirement
        // proceeds normally (owner condition 5: do not break legitimate
        // routine retirement).
        // =====================================================================
        {
            const id = "dr-legacy-clean";
            writeCard(isoRoot, id, { status: "completed" });
            commitAll(scratch.git, scratch.dir, `land ${id}\n\nTask-Card: ${id}\n`);
            const ok = retire(id);
            assert.equal(ok.ok, true, "legacy clean retirement must proceed");
            assert.equal(cardExists(isoRoot, id), false, "legacy clean card gone");
        }

        // =====================================================================
        // B1 regression — DEGRADED stored record (empty entries) on a landed
        // completed card: the empty inventory must NOT verify. Previously
        // verifyDecisionRecordForRetirement iterated zero entries -> ok, and
        // the contradiction check fired only on exactly-one-routine, so a
        // completed+landed card with non-empty owner_notes/measured_outcome
        // was DESTROYED with no pointer and no force — strictly weaker than
        // the legacy null-record path. Now: refused with
        // decision_record_unresolved naming the degradation AND running the
        // explicit-decision-field check; card + report survive.
        // =====================================================================
        {
            const id = "dr-degraded-empty";
            writeCard(isoRoot, id, {
                status: "completed",
                owner_notes: ["OWNER APPROVAL 2026-10-25: approved."],
                measured_outcome: "degraded-record fail-open case",
                decision_record: { version: 1, entries: [] },
            });
            commitAll(scratch.git, scratch.dir, `land ${id}\n\nTask-Card: ${id}\n`);
            const refused = retire(id);
            assert.equal(
                refused.ok,
                false,
                "B1: degraded empty-entries record must NOT verify",
            );
            assert.equal(
                refused.refusal.code,
                "decision_record_unresolved",
                `B1: refusal code; got ${JSON.stringify(refused.refusal.code)}`,
            );
            assert.ok(
                refused.refusal.message.includes("entries"),
                `B1: refusal must name the degraded-record problem; got: ${refused.refusal.message}`,
            );
            assert.ok(
                refused.refusal.message.includes("owner_notes"),
                `B1: refusal must carry the explicit-decision-field check (at least as strong as legacy null-record); got: ${refused.refusal.message}`,
            );
            assert.ok(
                Array.isArray(refused.refusal.fields) &&
                    refused.refusal.fields.includes("owner_notes"),
                `B1: refusal.fields must name owner_notes; got ${JSON.stringify(refused.refusal.fields)}`,
            );
            assert.equal(cardExists(isoRoot, id), true, "B1: card survives");
            assert.equal(reportExists(isoRoot, id), true, "B1: report survives");

            // Field-clean variant still refuses: the degraded record itself
            // is untrustworthy — an empty inventory is never "verified".
            const id2 = "dr-degraded-clean";
            writeCard(isoRoot, id2, {
                status: "completed",
                decision_record: { version: 1, entries: [] },
            });
            commitAll(
                scratch.git,
                scratch.dir,
                `land ${id2}\n\nTask-Card: ${id2}\n`,
            );
            const refused2 = retire(id2);
            assert.equal(
                refused2.ok,
                false,
                "B1: degraded record refuses even on a field-clean card",
            );
            assert.equal(
                refused2.refusal.code,
                "decision_record_unresolved",
                `B1 (clean) refusal code; got ${JSON.stringify(refused2.refusal.code)}`,
            );
            assert.equal(
                cardExists(isoRoot, id2),
                true,
                "B1: field-clean degraded card survives",
            );
        }

        // =====================================================================
        // Ordering pin: landing proof runs FIRST. A completed card with an
        // UNRESOLVABLE decision record but NO trailer commit is refused with
        // landing_not_confirmed (not decision_record_unresolved).
        // =====================================================================
        {
            const id = "dr-order";
            writeCard(isoRoot, id, {
                status: "completed",
                decision_record: {
                    version: 1,
                    entries: [{ kind: "artifact", path: "docs/never.md" }],
                },
            });
            const refused = retire(id);
            assert.equal(refused.ok, false, "ordering: must refuse");
            assert.equal(
                refused.refusal.code,
                "landing_not_confirmed",
                `ordering: landing proof must run before decision-record resolution; got ${refused.refusal.code}`,
            );
        }

        // =====================================================================
        // Declared-revision entries verified PRECISELY:
        //   - exact-sha commit entry, marker in message -> retires;
        //   - exact-sha artifact entry, back-ref in committed blob -> retires;
        //   - unresolvable revision -> refused naming revision_not_resolvable;
        //   - ORPHANED (reflog-only) revision -> refused naming
        //     revision_not_reachable (reachability, never object existence).
        // =====================================================================
        {
            const id = "dr-pin";
            writeWorkingCard(isoRoot, id, {
                owner_notes: ["OWNER APPROVAL: pinned-revision case."],
            });
            const rel = "docs/checkpoints/2026-10-25-dr-pin.md";
            // Prepare BOTH carriers and pin their shas BEFORE closeout
            // (declared-revision entries are how already-committed content is
            // declared; closeout does not verify, retirement does). The
            // pinned commit message carries the card-id ANCHOR and the
            // owner-notes excerpt — a covering declared-revision entry
            // verifies anchor + excerpt at exactly that revision.
            fs.writeFileSync(path.join(scratch.dir, "pin.txt"), "pin\n");
            const pinSha = commitAll(
                scratch.git,
                scratch.dir,
                `decision content for ${id}\n\nTask-Card: ${id}\nOWNER APPROVAL: pinned-revision case.\nMARKER-dr-pin decision text\n`,
            );
            fs.mkdirSync(path.dirname(path.join(scratch.dir, rel)), {
                recursive: true,
            });
            fs.writeFileSync(
                path.join(scratch.dir, rel),
                `# pinned\n\nTask-Card: ${id}\ndecision prose\n`,
            );
            const artSha = commitAll(
                scratch.git,
                scratch.dir,
                `docs: pinned artifact for ${id}\n\nTask-Card: ${id}\n`,
            );
            closeout(id, [
                {
                    kind: "commit",
                    revision: pinSha,
                    marker: "MARKER-dr-pin",
                    carries: { owner_notes: "OWNER APPROVAL: pinned-revision case." },
                },
                { kind: "artifact", path: rel, revision: artSha },
            ]);
            const ok = retire(id);
            assert.equal(ok.ok, true, `pinned-revision retire should succeed: ${JSON.stringify(ok)}`);
            assert.equal(cardExists(isoRoot, id), false, "pinned card gone");

            // Declared-revision negative: same shape, but the pinned commit
            // does NOT carry the excerpt (only the anchor) -> refused naming
            // excerpt_not_in_carrier; card survives.
            {
                const idX = "dr-pin-noexcerpt";
                writeWorkingCard(isoRoot, idX, {
                    owner_notes: [
                        "OWNER APPROVAL: excerpt-missing pinned-revision case.",
                    ],
                });
                fs.writeFileSync(path.join(scratch.dir, "pinx.txt"), "pinx\n");
                const pinXSha = commitAll(
                    scratch.git,
                    scratch.dir,
                    `decision content for ${idX}\n\nTask-Card: ${idX}\nMARKER-dr-pin-noexcerpt text\n`,
                );
                closeout(idX, [
                    {
                        kind: "commit",
                        revision: pinXSha,
                        carries: {
                            owner_notes:
                                "OWNER APPROVAL: excerpt-missing pinned-revision case.",
                        },
                    },
                ]);
                commitAll(
                    scratch.git,
                    scratch.dir,
                    `land ${idX}\n\nTask-Card: ${idX}\n`,
                );
                const refusedX = retire(idX);
                assert.equal(
                    refusedX.ok,
                    false,
                    "pinned-revision excerpt miss must refuse",
                );
                assert.equal(
                    refusedX.refusal.code,
                    "decision_record_unresolved",
                    `pinned-excerpt-miss refusal code; got ${JSON.stringify(refusedX.refusal)}`,
                );
                assert.ok(
                    (refusedX.refusal.problems || []).some(
                        (p) => p.code === "excerpt_not_in_carrier",
                    ),
                    `pinned-excerpt-miss must name excerpt_not_in_carrier; got ${JSON.stringify(refusedX.refusal.problems)}`,
                );
                assert.equal(
                    cardExists(isoRoot, idX),
                    true,
                    "pinned-excerpt-miss card survives",
                );
            }

            // Commit entry, revision OMITTED (search mode), marker never
            // committed in any reachable message -> refused naming
            // marker_not_in_reachable_history (the closeout-time promise
            // never landed in git).
            const idS = "dr-search-miss";
            writeWorkingCard(isoRoot, idS);
            closeout(idS, [
                { kind: "commit", marker: "MARKER-dr-search-miss-never" },
            ]);
            commitAll(scratch.git, scratch.dir, `land ${idS}\n\nTask-Card: ${idS}\n`);
            const refusedS = retire(idS);
            assert.equal(refusedS.ok, false);
            assert.equal(refusedS.refusal.code, "decision_record_unresolved");
            assert.ok(
                refusedS.refusal.message.includes("marker_not_in_reachable_history"),
                `search-mode miss must be named; got: ${refusedS.refusal.message}`,
            );
            assert.equal(cardExists(isoRoot, idS), true, "search-miss card survives");

            // Search-mode miss covered; now the declared-revision blob path:
            // a commit entry whose marker is ONLY in a FILE BLOB at the
            // declared revision (not the message) -> retires via the
            // tree-at-revision check; and a marker present NOWHERE at the
            // declared revision -> refused with marker_missing.
            const idB = "dr-pin-blob";
            writeWorkingCard(isoRoot, idB);
            const blobRel = "researches/decisions/2026-10-25-dr-pin-blob.md";
            fs.mkdirSync(path.dirname(path.join(scratch.dir, blobRel)), {
                recursive: true,
            });
            fs.writeFileSync(
                path.join(scratch.dir, blobRel),
                "# decision\n\nMARKER-dr-pin-blob lives in the file, not the message\n",
            );
            const blobSha = commitAll(
                scratch.git,
                scratch.dir,
                `docs: blob-located decision for ${idB}\n\nTask-Card: ${idB}\n`,
            );
            closeout(idB, [
                { kind: "commit", revision: blobSha, marker: "MARKER-dr-pin-blob" },
            ]);
            const okB = retire(idB);
            assert.equal(okB.ok, true, `blob-marker retire should succeed: ${JSON.stringify(okB)}`);
            assert.equal(cardExists(isoRoot, idB), false, "blob-marker card gone");

            const idM = "dr-pin-missing";
            writeWorkingCard(isoRoot, idM);
            const missSha = commitAll(
                scratch.git,
                scratch.dir,
                `land ${idM} without decision content\n\nTask-Card: ${idM}\n`,
            );
            closeout(idM, [
                { kind: "commit", revision: missSha, marker: "MARKER-dr-pin-missing" },
            ]);
            const refusedM = retire(idM);
            assert.equal(refusedM.ok, false);
            assert.equal(refusedM.refusal.code, "decision_record_unresolved");
            assert.ok(
                refusedM.refusal.message.includes("marker_missing"),
                `marker_missing must be named; got: ${refusedM.refusal.message}`,
            );
            assert.equal(cardExists(isoRoot, idM), true, "marker-missing card survives");

            // Unresolvable revision.
            const idU = "dr-pin-unresolvable";
            writeWorkingCard(isoRoot, idU);
            closeout(idU, [
                { kind: "commit", revision: "deadbeefdeadbeef", marker: "x" },
            ]);
            commitAll(scratch.git, scratch.dir, `land ${idU}\n\nTask-Card: ${idU}\n`);
            const refusedU = retire(idU);
            assert.equal(refusedU.ok, false);
            assert.equal(refusedU.refusal.code, "decision_record_unresolved");
            assert.ok(
                refusedU.refusal.message.includes("revision_not_resolvable"),
                `unresolvable must be named; got: ${refusedU.refusal.message}`,
            );

            // Orphaned (reflog-only) revision: object exists, no branch has it.
            const idO = "dr-pin-orphan";
            writeWorkingCard(isoRoot, idO);
            fs.writeFileSync(path.join(scratch.dir, "orphan.txt"), "o\n");
            const orphanSha = commitAll(
                scratch.git,
                scratch.dir,
                `orphan decision for ${idO}\n\nMARKER-dr-orphan\n`,
            );
            scratch.git(["reset", "--hard", "HEAD~1"]); // orphan it
            const reflogHas = scratch.git([
                "log",
                "-g",
                "--format=%H",
                `--grep=MARKER-dr-orphan`,
            ]);
            assert.ok(
                reflogHas.stdout.trim().length > 0,
                "orphan sanity: commit object exists in reflog",
            );
            closeout(idO, [
                { kind: "commit", revision: orphanSha, marker: "MARKER-dr-orphan" },
            ]);
            commitAll(scratch.git, scratch.dir, `land ${idO}\n\nTask-Card: ${idO}\n`);
            const refusedO = retire(idO);
            assert.equal(refusedO.ok, false);
            assert.equal(refusedO.refusal.code, "decision_record_unresolved");
            assert.ok(
                refusedO.refusal.message.includes("revision_not_reachable"),
                `orphan must be named unreachable; got: ${refusedO.refusal.message}`,
            );
            assert.equal(cardExists(isoRoot, idO), true, "orphan card survives");
        }
    } finally {
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

// D2 — git read failures in the decision-record verifier map to
// verifier_unavailable, NEVER to a confident marker-not-found. Both paths
// fail closed either way; the requirement is honest diagnosis (a failed
// read must not be reported as "searched and absent"). Uses the exported
// verifier directly (test seam): through deleteCoordinationTask these
// branches sit behind a successful landing proof, which cannot be staged in
// a repo whose git reads fail.
test("decision-record verifier: git read failures -> verifier_unavailable, not marker-not-found", () => {
    const notARepo = fs.mkdtempSync(path.join(os.tmpdir(), "dr-notrepo-"));
    const corrupt = makeScratchRepo();
    try {
        // (1) Search-mode commit entry against a NON-git directory: every
        //     `git log` exits 128 there. `git log` exits 0 even on no-match,
        //     so a non-zero exit is a failed read -> verifier_unavailable
        //     (pre-fix this collapsed to marker_not_in_reachable_history).
        const searchMode = verifyDecisionRecordForRetirement(
            "dr-d2-search",
            {
                version: 1,
                entries: [{ kind: "commit", marker: "MARKER-dr-d2-search" }],
            },
            notARepo,
        );
        assert.equal(searchMode.ok, false, "(1) must not verify");
        assert.equal(
            searchMode.problems.length,
            1,
            `(1) exactly one problem; got ${JSON.stringify(searchMode.problems)}`,
        );
        assert.equal(
            searchMode.problems[0].code,
            "verifier_unavailable",
            `(1) failed git-log read must diagnose verifier_unavailable; got ${JSON.stringify(searchMode.problems[0])}`,
        );

        // (2) Declared-revision commit entry whose commit is reachable but
        //     whose TREE object is unreadable (loose object deleted): the
        //     message carries no marker, then `git grep` on the tree fails
        //     with a non-0/1 exit -> verifier_unavailable (pre-fix
        //     treeAtRevisionContains collapsed any non-zero to false ->
        //     marker_missing).
        fs.writeFileSync(path.join(corrupt.dir, "d2.txt"), "x\n");
        const sha = commitAll(
            corrupt.git,
            corrupt.dir,
            "d2 carrier\n\nMARKER-not-in-message\n",
        );
        const treeSha = corrupt.git([
            "rev-parse",
            `${sha}^{tree}`,
        ]).stdout.trim();
        const treeObjPath = path.join(
            corrupt.dir,
            ".git",
            "objects",
            treeSha.slice(0, 2),
            treeSha.slice(2),
        );
        assert.ok(
            fs.existsSync(treeObjPath),
            "(2) setup: loose tree object exists (fresh repo, no packing)",
        );
        fs.rmSync(treeObjPath);
        const treeRead = verifyDecisionRecordForRetirement(
            "dr-d2-tree",
            {
                version: 1,
                entries: [
                    {
                        kind: "commit",
                        revision: sha,
                        marker: "MARKER-dr-d2-tree",
                    },
                ],
            },
            corrupt.dir,
        );
        assert.equal(treeRead.ok, false, "(2) must not verify");
        assert.ok(
            treeRead.problems.some((p) => p.code === "verifier_unavailable"),
            `(2) failed tree read must diagnose verifier_unavailable; got ${JSON.stringify(treeRead.problems)}`,
        );
    } finally {
        fs.rmSync(notARepo, { recursive: true, force: true });
        fs.rmSync(corrupt.dir, { recursive: true, force: true });
    }
});

// F1 — OPTION-INJECTION regression (round 2). A structurally-valid marker
// with a leading dash used to reach `git grep -F --quiet <marker> <sha> --`
// as a BARE POSITIONAL: git consumed "-v" as --invert-match, the sha became
// the pattern, no revision was searched, the WORKING TREE was scanned
// inverted, and the exit 0 read as contains:true — the gate verified a
// pointer that never existed and retired a completed card (fail-open
// destructive deletion; the bare form's exit 0 was re-confirmed empirically
// on 2026-10-26). Two independent pins, so either layer regressing alone
// cannot reopen the hole:
//   (i)   the retirement gate structural-checks the STORED record and
//         refuses the dash-prefixed marker, NAMING the rule (the
//         declaration-time refusal is pinned in the closeout suite);
//   (ii)  the git-layer verifier itself, handed marker "-v" directly (a
//         caller that skips the structural gate), resolves the declared
//         REACHABLE revision and reports marker_missing — `git grep -e`
//         exit 1 — never a working-tree hit (exit 0).
// Also pins the audit finding for dash-prefixed REVISIONS: rev-parse fails
// closed on them (revision_not_resolvable), and the structural gate refuses
// them at declaration and retirement.
test("decision-record option injection (F1): dash marker refused structurally; git layer searches the declared tree only", () => {
    const isoRoot = fs.mkdtempSync(path.join(os.tmpdir(), "dr-f1-iso-"));
    const prevCoordRoot = process.env.OPENCODE_LOCAL_COORDINATOR_ROOT;
    process.env.OPENCODE_LOCAL_COORDINATOR_ROOT = isoRoot;
    bindSessionName(SESSION_ID, ALIAS);
    const scratch = makeScratchRepo();

    const retire = (id, opts = {}) =>
        deleteCoordinationTask(SESSION_ID, id, {
            cwd: "/verification",
            landingProofRepo: scratch.dir,
            ...opts,
        });

    try {
        // Reachable carrier commit whose message and blobs contain NO
        // literal "-v" anywhere. The carrier is built from CONTROLLED
        // content with negative-control assertions: commitAll() writes
        // `content f-<pid>-<base36>.txt`, and when the base36 run begins
        // with "v" (p = 1/36 per run) the blob legitimately contains
        // "-v" — git grep -F -e -v then CORRECTLY matches it and the
        // verifier ok:trues (observed flake 2026-10-26: the verifier was
        // right; the fixture premise "no -v anywhere" was not).
        const F1_CARRIER_BODY =
            "f1 carrier blob: fixed content, no dash vee sequence\n";
        assert.ok(
            !F1_CARRIER_BODY.includes("-v"),
            "F1 fixture negative control: carrier body must not contain -v",
        );
        fs.writeFileSync(
            path.join(scratch.dir, "f1-carrier.txt"),
            F1_CARRIER_BODY,
        );
        scratch.git(["add", "-A"]);
        scratch.git(["commit", "-q", "-m", "carrier for F1 injection fixture\n"]);
        const sha = scratch.git(["rev-parse", "HEAD"]).stdout.trim();
        // Negative control ON THE COMMITTED TREE, via the same command
        // shape the verifier uses: exit 1 (determinate no-match) proves
        // the fixture is clean, so the marker_missing pinned below is the
        // -e pattern binding working — never a content accident.
        const f1FixtureProbe = scratch.gitRaw([
            "grep",
            "-F",
            "--quiet",
            "-e",
            "-v",
            sha,
            "--",
        ]);
        assert.ok(
            f1FixtureProbe.status === 1,
            `F1 fixture negative control: committed tree at ${sha} must not contain "-v" (git grep exit ${f1FixtureProbe.status}; stderr: ${f1FixtureProbe.stderr})`,
        );

        // (ii) VERIFIER LEVEL — the direct pin on the -e binding. Pre-fix
        // this returned ok:true (inverted working-tree scan, exit 0).
        const verified = verifyDecisionRecordForRetirement(
            "dr-f1",
            {
                version: 1,
                entries: [{ kind: "commit", revision: sha, marker: "-v" }],
            },
            scratch.dir,
        );
        assert.equal(
            verified.ok,
            false,
            `F1(ii): marker "-v" must NOT verify against a reachable revision; got ${JSON.stringify(verified)}`,
        );
        assert.ok(
            verified.problems.some(
                (p) => p.code === "marker_missing",
            ),
            `F1(ii): refusal must be an honest marker_missing (searched the declared tree, found nothing); got ${JSON.stringify(verified.problems)}`,
        );

        // (i) RETIREMENT LEVEL — the stored record is refused and NAMED
        // (hand-edited card: closeout itself refuses this shape, pinned in
        // the closeout suite), and the card + report SURVIVE.
        const id = "dr-f1-marker";
        writeCard(isoRoot, id, {
            status: "completed",
            decision_record: {
                version: 1,
                entries: [{ kind: "commit", revision: sha, marker: "-v" }],
            },
        });
        commitAll(scratch.git, scratch.dir, `land ${id}\n\nTask-Card: ${id}\n`);
        const refused = retire(id);
        assert.equal(
            refused.ok,
            false,
            "F1(i): dash-prefixed stored marker must be refused",
        );
        assert.equal(
            refused.refusal.code,
            "decision_record_unresolved",
            `F1(i): refusal code; got ${JSON.stringify(refused.refusal.code)}`,
        );
        assert.ok(
            refused.refusal.message.includes('must not start with "-"') &&
                refused.refusal.message.includes("marker"),
            `F1(i): refusal must NAME the marker rule; got: ${refused.refusal.message}`,
        );
        assert.equal(cardExists(isoRoot, id), true, "F1(i): card survives");
        assert.equal(
            reportExists(isoRoot, id),
            true,
            "F1(i): report survives",
        );

        // Audit pin — dash-prefixed REVISION: git fails closed by itself
        // (rev-parse exits non-zero on "-v^{commit}"), and the structural
        // gate refuses it at declaration (closeout suite) and retirement.
        const dashRev = verifyDecisionRecordForRetirement(
            "dr-f1-rev",
            {
                version: 1,
                entries: [
                    { kind: "commit", revision: "-v", marker: "MARKER-x" },
                ],
            },
            scratch.dir,
        );
        assert.equal(
            dashRev.ok,
            false,
            "F1 audit: dash-prefixed revision must not resolve",
        );
        assert.ok(
            dashRev.problems.some(
                (p) => p.code === "revision_not_resolvable",
            ),
            `F1 audit: refusal must name revision_not_resolvable; got ${JSON.stringify(dashRev.problems)}`,
        );
    } finally {
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

// F2 — RAW-record structural check at retirement. normalizeStoredDecisionRecord
// (the read path) keeps only {version, entries}, so a hand-edited stored
// record carrying an unknown record-level key used to be laundered into its
// valid subset: 4c-2a passed, the routine attestation verified, and the card
// retired routinely with the junk never named. Now the RAW
// parsedCard.decision_record is structural-checked BEFORE normalization and
// the junk key is refused BY NAME.
//
// F7 — routine attestation contradicted by NON-EMPTY open_questions, the
// only decision-class field source without a direct fixture: refused with
// decision_content_unpromoted naming open_questions (fields may appear after
// closeout, exactly like the c2 review-verdict case).
test("decision-record retirement: raw record-level junk named (F2); routine vs non-empty open_questions named (F7)", () => {
    const isoRoot = fs.mkdtempSync(path.join(os.tmpdir(), "dr-f2f7-iso-"));
    const prevCoordRoot = process.env.OPENCODE_LOCAL_COORDINATOR_ROOT;
    process.env.OPENCODE_LOCAL_COORDINATOR_ROOT = isoRoot;
    bindSessionName(SESSION_ID, ALIAS);
    const scratch = makeScratchRepo();

    const retire = (id, opts = {}) =>
        deleteCoordinationTask(SESSION_ID, id, {
            cwd: "/verification",
            landingProofRepo: scratch.dir,
            ...opts,
        });

    try {
        commitAll(scratch.git, scratch.dir, "initial: no trailer");

        // F2: hand-edited stored record — valid routine entry PLUS an
        // unknown record-level key. No decision-class card fields, so the
        // only thing standing between this card and routine deletion is
        // the raw structural check.
        const id = "dr-f2-raw-junk";
        writeCard(isoRoot, id, {
            status: "completed",
            decision_record: {
                version: 1,
                entries: [
                    { kind: "routine", note: "nothing decision-class" },
                ],
                provenance: "hand-edited after closeout",
            },
        });
        commitAll(scratch.git, scratch.dir, `land ${id}\n\nTask-Card: ${id}\n`);
        const refused = retire(id);
        assert.equal(
            refused.ok,
            false,
            "F2: record-level junk must not be laundered into a valid subset",
        );
        assert.equal(
            refused.refusal.code,
            "decision_record_unresolved",
            `F2: refusal code; got ${JSON.stringify(refused.refusal.code)}`,
        );
        assert.ok(
            refused.refusal.message.includes('unknown key "provenance"'),
            `F2: refusal must NAME the junk key on the RAW record; got: ${refused.refusal.message}`,
        );
        assert.equal(cardExists(isoRoot, id), true, "F2: card survives");
        assert.equal(reportExists(isoRoot, id), true, "F2: report survives");

        // F7: routine record + non-empty open_questions patched in AFTER
        // closeout (the post-closeout field shape the c2 case uses).
        const id7 = "dr-f7-openq";
        writeWorkingCard(isoRoot, id7);
        closeout(id7, [{ kind: "routine", note: "nothing decision-class" }]);
        patchCard(isoRoot, id7, {
            open_questions: [
                "Does the routine attestation contradict this non-empty question?",
            ],
        });
        commitAll(
            scratch.git,
            scratch.dir,
            `land ${id7}\n\nTask-Card: ${id7}\n`,
        );
        const refused7 = retire(id7);
        assert.equal(
            refused7.ok,
            false,
            "F7: routine + non-empty open_questions must refuse retirement",
        );
        assert.equal(
            refused7.refusal.code,
            "decision_content_unpromoted",
            `F7: refusal code; got ${JSON.stringify(refused7.refusal.code)}`,
        );
        assert.ok(
            Array.isArray(refused7.refusal.fields) &&
                refused7.refusal.fields.includes("open_questions"),
            `F7: refusal must name open_questions; got ${JSON.stringify(refused7.refusal.fields)}`,
        );
        assert.equal(cardExists(isoRoot, id7), true, "F7: card survives");
    } finally {
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

// F3/F4 — tri-state git read helpers. A failed read must surface as
// available:false (mapped to verifier_unavailable at the call sites),
// never as a confident marker-absent (F3: commitMessageContains used to
// return bare false) or path-not-tracked (F4: committedBlobAt used to
// return bare null). Exported TEST SEAMS; the unavailable branches are
// injected via a NON-GIT directory because repo corruption cannot
// selectively fail `git log -1` / `git show` while letting the earlier
// rev-parse / reachability reads of the same entry succeed.
test("decision-record git read helpers (F3/F4): unavailable vs determinate miss are distinct states", () => {
    const notARepo = fs.mkdtempSync(path.join(os.tmpdir(), "dr-f34-dead-"));
    const repo = makeScratchRepo();
    try {
        fs.writeFileSync(path.join(repo.dir, "blob.txt"), "MARKER-f34 in blob\n");
        const sha = commitAll(
            repo.git,
            repo.dir,
            "carrier\n\nMARKER-f34 in message\n",
        );

        // F3 commitMessageContains: hit / determinate miss / unavailable.
        const hit = commitMessageContains(
            repo.dir,
            sha,
            "MARKER-f34 in message",
        );
        assert.ok(
            hit.available && hit.contains,
            `F3: message marker must read as available+contains; got ${JSON.stringify(hit)}`,
        );
        const miss = commitMessageContains(repo.dir, sha, "MARKER-absent");
        assert.ok(
            miss.available && !miss.contains,
            `F3: absent marker must read as available+!contains (searched, not failed); got ${JSON.stringify(miss)}`,
        );
        const deadMsg = commitMessageContains(
            notARepo,
            sha,
            "MARKER-f34 in message",
        );
        assert.ok(
            !deadMsg.available &&
                deadMsg.contains === false &&
                typeof deadMsg.stderr === "string" &&
                deadMsg.stderr !== "",
            `F3: failed git-log read must read as UNAVAILABLE with stderr, never a clean miss; got ${JSON.stringify(deadMsg)}`,
        );

        // F4 committedBlobAt: tracked blob / determinate miss / unavailable.
        const blob = committedBlobAt(repo.dir, sha, "blob.txt");
        assert.ok(
            blob.available &&
                blob.tracked === true &&
                typeof blob.blob === "string" &&
                blob.blob.includes("MARKER-f34 in blob"),
            `F4: tracked blob must read available+tracked+content; got ${JSON.stringify(blob)}`,
        );
        const untracked = committedBlobAt(repo.dir, sha, "not-there.md");
        assert.ok(
            untracked.available &&
                untracked.tracked === false &&
                untracked.blob === null,
            `F4: untracked path must read as available DETERMINATE MISS (tracked:false), never as a failed read; got ${JSON.stringify(untracked)}`,
        );
        const deadBlob = committedBlobAt(notARepo, sha, "blob.txt");
        assert.ok(
            !deadBlob.available && deadBlob.blob === null,
            `F4: failed git read must read as UNAVAILABLE, never overstated as path-not-tracked; got ${JSON.stringify(deadBlob)}`,
        );

        // F4 b-F1 shapes (round 3): directory paths are DETERMINATE MISSES
        // carrying a missReason — never tracked content. `git show
        // <sha>:<dir>` exits 0 printing the tree LISTING, so the pre-fix
        // success path returned tracked:true with the listing as "blob"
        // (an id-named FILENAME inside it then falsely verified).
        fs.mkdirSync(path.join(repo.dir, "docs/coordination"), {
            recursive: true,
        });
        fs.writeFileSync(
            path.join(repo.dir, "docs/coordination/blob.txt"),
            "nested blob, not a marker\n",
        );
        const dirSha = commitAll(repo.git, repo.dir, "dir carrier\n");
        const asDir = committedBlobAt(
            repo.dir,
            dirSha,
            "docs/coordination",
        );
        assert.ok(
            asDir.available &&
                asDir.tracked === false &&
                asDir.blob === null &&
                typeof asDir.missReason === "string" &&
                asDir.missReason.includes("is a directory") &&
                asDir.missReason.includes("declare the durable file"),
            `F4(b-F1): directory path must be a DETERMINATE MISS with a directory missReason, never tracked listing content; got ${JSON.stringify(asDir)}`,
        );
        const asDirSlash = committedBlobAt(
            repo.dir,
            dirSha,
            "docs/coordination/",
        );
        assert.ok(
            asDirSlash.available &&
                asDirSlash.tracked === false &&
                asDirSlash.blob === null &&
                typeof asDirSlash.missReason === "string" &&
                asDirSlash.missReason.includes("not a file blob"),
            `F4(b-F1): trailing-slash directory path (ls-tree answers with CHILD entries) must be a determinate miss, never the child's content; got ${JSON.stringify(asDirSlash)}`,
        );
    } finally {
        fs.rmSync(notARepo, { recursive: true, force: true });
        fs.rmSync(repo.dir, { recursive: true, force: true });
    }
});

// b-F2 (round 3) — artifact SEARCH-MODE walk diagnosis honesty. When no
// carrier verifies AND at least one committed-blob read was UNAVAILABLE
// (here: the newest touching revision's loose blob object deleted, so
// ls-tree resolves the entry but `git show` cannot read it), the final
// diagnosis must be verifier_unavailable — the negative search is
// INCOMPLETE — never an overstated marker_not_in_committed_blob (which
// claims a completed negative search). Fail direction unchanged
// (refusal either way); message accuracy only. Uses the exported
// verifier directly (test seam), mirroring the D2 technique.
test("decision-record artifact search walk: unavailable read -> verifier_unavailable, not marker-absent (b-F2)", () => {    const repo = makeScratchRepo();
    try {
        const rel = "docs/decisions/d3.md";
        fs.mkdirSync(path.dirname(path.join(repo.dir, rel)), {
            recursive: true,
        });
        // Two reachable revisions touching the path; NEITHER carries the
        // marker in committed content.
        fs.writeFileSync(path.join(repo.dir, rel), "v1 no marker\n");
        commitAll(repo.git, repo.dir, "d3 v1\n");
        fs.writeFileSync(path.join(repo.dir, rel), "v2 no marker\n");
        const newestSha = commitAll(repo.git, repo.dir, "d3 v2\n");
        // Make the NEWEST revision's blob read FAIL without touching the
        // trees (loose object deleted; ls-tree still resolves the entry).
        const blobSha = repo.git([
            "rev-parse",
            `${newestSha}:${rel}`,
        ]).stdout.trim();
        const blobObjPath = path.join(
            repo.dir,
            ".git",
            "objects",
            blobSha.slice(0, 2),
            blobSha.slice(2),
        );
        assert.ok(
            fs.existsSync(blobObjPath),
            "b-F2 setup: loose blob object exists (fresh repo, no packing)",
        );
        fs.rmSync(blobObjPath);
        const res = verifyDecisionRecordForRetirement(
            "dr-bf2-walk",
            {
                version: 1,
                entries: [{ kind: "artifact", path: rel }],
            },
            repo.dir,
        );
        assert.equal(res.ok, false, "b-F2: must not verify");
        assert.equal(
            res.problems.length,
            1,
            `b-F2: exactly one aggregate problem; got ${JSON.stringify(res.problems)}`,
        );
        assert.equal(
            res.problems[0].code,
            "verifier_unavailable",
            `b-F2: an incomplete negative search must diagnose verifier_unavailable, never marker_not_in_committed_blob; got ${JSON.stringify(res.problems[0])}`,
        );
        assert.ok(
            res.problems[0].detail.includes(
                "marker absence is NOT established",
            ),
            `b-F2: detail must state marker absence is not established; got: ${res.problems[0].detail}`,
        );
    } finally {
        fs.rmSync(repo.dir, { recursive: true, force: true });
    }
});

// Scatter pin (v2 coverage rule): a covering search-mode commit entry whose
// excerpts each appear in a DIFFERENT card-id-anchored message, but never
// together in ONE message, must refuse with carrier_content_scattered — one
// entry is one carrier; the aggregate never reads as a per-excerpt miss
// (each part exists somewhere) nor verifies.
test("decision-record coverage: scattered excerpts across anchored carriers -> carrier_content_scattered", () => {
    const repo = makeScratchRepo();
    try {
        const id = "dr-scatter";
        const e1 = "OWNER APPROVAL scatter: first excerpt line lives in commit one";
        const e2 = "OWNER APPROVAL scatter: second excerpt line lives in commit two";
        commitAll(
            repo.git,
            repo.dir,
            `anchored carrier one\n\nTask-Card: ${id}\n\n${e1}\n`,
        );
        commitAll(
            repo.git,
            repo.dir,
            `anchored carrier two\n\nTask-Card: ${id}\n\n${e2}\n`,
        );
        const res = verifyDecisionRecordForRetirement(
            id,
            {
                version: 1,
                entries: [
                    {
                        kind: "commit",
                        carries: { owner_notes: e1, measured_outcome: e2 },
                    },
                ],
            },
            repo.dir,
        );
        assert.equal(res.ok, false, "scatter: must not verify");
        assert.ok(
            res.problems.some((p) => p.code === "carrier_content_scattered"),
            `scatter: refusal must name carrier_content_scattered; got ${JSON.stringify(res.problems)}`,
        );
    } finally {
        fs.rmSync(repo.dir, { recursive: true, force: true });
    }
});
