// Decision-record RETIREMENT gate tests (retention preservation,
// owner-approved 2026-10-25; v2 coverage 2026-10-26; v3 item-enumerated
// replacement owner-approved 2026-10-27).
//
// Pins the retirement half of the v3 gate in deleteCoordinationTask
// (completed branch, AFTER the unchanged landing-proof check):
//
//   - E2E (the owner's acceptance test): a completed card whose decision
//     items are declared one-slot-per-item (verbatim + sanitized + artifact
//     verbatim) retires ONLY after every slot's carrier resolves against
//     committed, branch-reachable git state — and after retirement each
//     item is recoverable FROM SURVIVING GIT STATE ONLY (fresh git
//     processes; the card + report dir are gone): verbatim items recovered
//     verbatim from trailer-hit commit bodies; curated/sanitized items
//     recovered with their honest label visible. Non-vacuity controls
//     mandatory.
//   - PRIMARY regression (concern 1, corrected two-notes reproducer): a
//     stored record declaring ONLY the first note's content-bound id while
//     the card carries TWO notes is refused as a new-item mismatch naming
//     the unslotted note.
//     Pre-v3 this retired with Note B unrecoverable (reproduced 2026-10-26,
//     tmp/repro/repro-concern1-receipts.json at HEAD bb60068f).
//   - INVENTORY BASELINE across the lifecycle (owner condition 2):
//     post-closeout ADDITION (blocked review) -> mismatch refusal naming
//     the new items; post-closeout REMOVAL -> the slot stays and its
//     carrier must STILL verify (baseline; deletion cannot launder); the
//     sanctioned amendment is the add-only re-closeout; blocked→clean
//     review anti-laundering (the probe's pin, adapted to item ids);
//     multiple consequential review events each enumerated.
//   - CONDITION 1 core: a carrier containing ONLY the card id (the landing
//     trailer) satisfies NOTHING; verbatim handle absent, curated label
//     absent, un-anchored carriers, pinned-revision misses — each refused
//     and named.
//   - C2 (routine-after-review): a routine task with a clean-approve
//     review retires through the REAL lifecycle with zero promotion; a
//     non-approve verdict still blocks a routine record naming the review
//     items.
//   - Legacy: v1 bare/marker records and v2 carries records are honored as
//     LEGACY — retire only when the card carries no decision items;
//     null-record cards likewise (refused iff items present).
//   - Negative (a): artifact path never committed -> refused, item NAMED.
//   - Negative (b): label added only in the WORKING TREE -> refused.
//   - Negative (b3/b-F1): artifact path IS A DIRECTORY -> determinate miss
//     named as a directory, never verified from the listing.
//   - Negative (d): force -> deletion proceeds AND the destructive warning
//     NAMES items + slots.
//   - Ordering pin: landing proof runs FIRST.
//   - Declared-revision carriers verified precisely; unresolvable and
//     orphaned revisions refused.
//   - F1 option injection: dash-prefixed stored handle/revision refused
//     structurally; git layer searches the declared tree only (-e).
//   - F2: raw record-level junk named pre-normalization.
//   - F7: routine record + post-closeout open_questions -> mismatch named.
//   - F3/F4 + D2 + b-F2: tri-state git read helpers; verifier_unavailable
//     never overstated as a confident miss.
//   - PROBE PINS (preserved from the 2026-10-26 probe, adapted to v3):
//     the artifact card-id-only recovery chain (pickaxe -> diff-tree path
//     derivation -> blob read), and the blocked→completed anti-laundering
//     refusal naming the earlier consequential review event.
//   - REVIEW-F1 scatter regression (2026-10-27 fix round): anchor-only
//     revision + handle-only revision of one artifact path -> terminal
//     anchor_handle_scattered naming BOTH revisions (refusal + survival);
//     the reunified same-revision flip verifies; commit-mode split evidence
//     (handle only in an UN-anchored message) is a terminal
//     handle_not_in_carrier, never a pass.
//   - (D2) content-bound history identity (B-F1 fix): a hand-REORDER of
//     history events changes no item id (identity is content) and retires;
//     an in-place content REPLACEMENT of a review event mints a new id and
//     refuses naming the replacement (control card retires).
//   - (E) positive two-slots-SAME-carrier: both verbatim slots verify
//     independently from ONE anchored commit body; retirement proceeds.
//
// Test seam (NOT an env var): the retirement verifier's repo is redirected
// at a hermetic scratch git repo via the `landingProofRepo` OPTION (not
// forwarded by plan-state.js).
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
    updateCoordinationTaskMetadata,
    deleteCoordinationTask,
    verifyDecisionRecordForRetirement,
    commitMessageContains,
    committedBlobAt,
    computeTaskDesignDigest,
    collectDecisionItems,
    decisionItemId,
} = await import(pathToFileURL(STATE_LIB).href);

// Content-bound item ids (the B-F1 fix): ids derive from each item's own
// text via the gate's exported factory, so fixtures cannot drift from the
// identity grammar.
const noteId = (text) => decisionItemId("owner_notes", text);
const questionId = (text) => decisionItemId("open_questions", text);

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

function closeout(id, items, options = {}) {
    const body =
        "# Closeout\n\n```decision-record\n" +
        JSON.stringify({ version: 3, items }, null, 2) +
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

// Canonical label helper for curated/sanitized slots (mirrors the gate).
function label(taskId, itemId, disposition) {
    return `decision-item: ${taskId}#${itemId} (${disposition})`;
}

test("decision-record retirement gate: E2E per-item recovery + baseline lifecycle matrix + negatives + legacy compat + force", () => {
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
        // E2E — the owner's acceptance test under the v3 item-enumerated
        // contract. Card carries THREE decision items: an owner
        // authorization (verbatim, commit carrier), a rejected alternative
        // (sanitized, artifact carrier with the canonical label), and the
        // measured outcome (verbatim, artifact carrier). One slot per
        // item; retirement verifies every slot's carrier carries the
        // card-id anchor AND that item's handle.
        // =====================================================================
        const ID = "dr-e2e";
        const OWNER_AUTH =
            "OWNER-AUTH dr-e2e: owner approved 2026-10-27, six conditions (chat m0066)";
        const REJECTED_NOTE =
            "REJECTED ALTERNATIVE dr-e2e: per-item excerpts — deepens the privacy conflict (private reasoning withheld)";
        const MEASURED = "Measured outcome: gate refuses unpromoted retirement.";
        const ARTIFACT_REL = "docs/checkpoints/2026-10-27-dr-e2e.md";
        const SANITIZED_LABEL = label(ID, noteId(REJECTED_NOTE), "sanitized");

        writeWorkingCard(isoRoot, ID, {
            owner_notes: [OWNER_AUTH, REJECTED_NOTE],
        });

        // Closeout declares all three slots BEFORE the carriers exist
        // (search mode — the landing commit cannot be sha-pinned at
        // closeout). The sanitized slot carries cited provenance. Slot ids
        // are CONTENT-BOUND (the B-F1 fix), derived via the factory.
        closeout(
            ID,
            [
                {
                    id: noteId(OWNER_AUTH),
                    disposition: "verbatim",
                    carrier: { kind: "commit" },
                    handle: OWNER_AUTH,
                    provenance: "owner chat m0066, 2026-10-27 — verbatim note",
                },
                {
                    id: noteId(REJECTED_NOTE),
                    disposition: "sanitized",
                    carrier: { kind: "artifact", path: ARTIFACT_REL },
                    handle: SANITIZED_LABEL,
                    provenance:
                        "rejected alternative; private reasoning withheld (owner chat m0066 authorized the withholding)",
                },
                {
                    id: decisionItemId("measured_outcome", MEASURED),
                    disposition: "verbatim",
                    carrier: { kind: "artifact", path: ARTIFACT_REL },
                    handle: MEASURED,
                },
            ],
            { measuredOutcome: MEASURED },
        );

        // Landing commit: trailer + the verbatim authorization in the BODY.
        fs.writeFileSync(path.join(scratch.dir, "e2e.txt"), "landed\n");
        const landingSha = commitAll(
            scratch.git,
            scratch.dir,
            `land retention gate v3\n\nTask-Card: ${ID}\n\n${OWNER_AUTH}\n`,
        );
        // Promoted durable file: card-id back-reference, the sanitized
        // label followed by a useful curation, and the measured outcome
        // (wrapped across lines — verbatim handles are
        // whitespace-normalization tolerant).
        fs.mkdirSync(path.dirname(path.join(scratch.dir, ARTIFACT_REL)), {
            recursive: true,
        });
        fs.writeFileSync(
            path.join(scratch.dir, ARTIFACT_REL),
            `# Retention gate v3 decision record\n\nTask-Card: ${ID}\n\n${SANITIZED_LABEL}: the owner REJECTED per-item excerpt binding; reasoning: it preserved count, not meaning, and deepened the privacy conflict. Authority: owner chat m0066 (2026-10-27). Private rationale withheld as sensitive.\nMeasured outcome: gate refuses unpromoted\nretirement.\n`,
        );
        const artifactSha = commitAll(
            scratch.git,
            scratch.dir,
            `docs: promote ${ID} decision record\n\nTask-Card: ${ID}\n`,
        );

        // Retire: landing proven + inventory complete + every carrier
        // carries anchor + handle -> DELETED.
        const done = retire(ID);
        assert.equal(done.ok, true, `E2E retire should succeed: ${JSON.stringify(done)}`);
        assert.equal(cardExists(isoRoot, ID), false, "E2E: card must be gone");
        assert.equal(reportExists(isoRoot, ID), false, "E2E: report dir must be gone");

        // PER-ITEM RECOVERY — from SURVIVING GIT STATE ONLY, in FRESH git
        // processes. Every assertion is an OUTPUT-CONTAINS check on the
        // exact recovered strings (git log exits 0 even on no match, so a
        // bare exit-status check passes vacuously — the non-vacuity
        // fixture below proves these assertions can fail).
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
        // Item 1 — verbatim: card-id only -> trailer grep -> body read ->
        // the item's own text.
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
            "E2E recovery: card-id trailer grep must find at least one carrier commit",
        );
        const c3Bodies = c3Shas.map((sha) =>
            scratch.git(["log", "-1", "--format=%B", sha]).stdout,
        );
        const c3Joined = c3Bodies.join("\n--\n");
        assert.ok(
            c3Joined.includes(OWNER_AUTH),
            "E2E recovery (verbatim item): owner authorization recoverable VERBATIM from a trailer-hit commit body (card-id only, bounded queries)",
        );
        // Items 2+3 — artifact: pickaxe discovery -> path derivation ->
        // committed blob read.
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
            "E2E recovery: card-id pickaxe finds the artifact-introducing commit",
        );
        const artifactBlob = scratch.gitRaw([
            "show",
            `${artifactSha}:${ARTIFACT_REL}`,
        ]);
        assert.ok(
            artifactBlob.status === 0 &&
                artifactBlob.stdout.includes(`Task-Card: ${ID}`) &&
                artifactBlob.stdout.includes("Measured outcome: gate refuses unpromoted"),
            "E2E recovery (verbatim item): measured outcome recoverable from the COMMITTED blob via git show <reachable-sha>:<path> (wrapped in the doc; whitespace-normalized containment is the verified shape)",
        );
        assert.ok(
            artifactBlob.status === 0 &&
                artifactBlob.stdout.includes(SANITIZED_LABEL),
            "E2E recovery (sanitized item): the canonical label is present in the recovered carrier — the item is LOCATABLE post-retirement",
        );
        assert.ok(
            artifactBlob.status === 0 &&
                artifactBlob.stdout.includes(
                    "the owner REJECTED per-item excerpt binding",
                ),
            "E2E recovery (sanitized item): the useful curation (what was rejected, why, authority) is recoverable beside the label",
        );
        // Honest-label distinction: the recovered record distinguishes
        // verbatim from sanitized — the label embeds the disposition, and
        // the verbatim body text carries no label.
        assert.ok(
            SANITIZED_LABEL.includes("(sanitized)") &&
                !c3Joined.includes("(sanitized)"),
            "E2E recovery: curated/sanitized items are honestly labeled in the recovered carrier; verbatim items carry their raw text",
        );

        // NON-VACUITY fixture (negative control): the same OUTPUT-CONTAINS
        // recovery queries against a repo where the content was NEVER
        // committed (trailer landing only) must come back WITHOUT the
        // strings.
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
                for (const needle of [OWNER_AUTH, MEASURED]) {
                    assert.equal(
                        recoveryHit(needle),
                        false,
                        `non-vacuity: recovery assertion must FAIL where the content was never committed (${needle.slice(0, 32)}…)`,
                    );
                }
                assert.equal(
                    bare
                        .gitRaw(["log", "--branches", "--format=%H", "-Sdr-e2e"])
                        .stdout.trim()
                        .split(/\r?\n/)
                        .filter(Boolean).length,
                    0,
                    "non-vacuity: pickaxe finds nothing where no anchored artifact was committed",
                );
            } finally {
                fs.rmSync(bare.dir, { recursive: true, force: true });
            }
        }

        // =====================================================================
        // PRIMARY REGRESSION — the corrected two-notes reproducer (owner
        // condition 1). A card with TWO owner notes whose stored record
        // declares ONLY the first (the shape v2 accepted: one excerpt
        // covers the FIELD). v3: new-item mismatch at retirement, Note B
        // named; card + report survive.
        // =====================================================================
        {
            const id = "dr-two-notes";
            const NOTE_1 =
                "ARCH APPROVED concern1: keep an inventory gate but bind preservation per decision item";
            const NOTE_2 =
                "ARCH REJECTED concern1: sidecar decision ledger because a second source of truth drifts";
            writeCard(isoRoot, id, {
                status: "completed",
                owner_notes: [NOTE_1, NOTE_2],
                decision_record: {
                    version: 3,
                    items: [
                        {
                            id: noteId(NOTE_1),
                            disposition: "verbatim",
                            carrier: { kind: "commit" },
                            handle:
                                "keep an inventory gate but bind preservation per decision item",
                            provenance: "owner chat, 2026-10-26 — verbatim note",
                        },
                    ],
                },
            });
            // Landing commit carries the trailer AND Note A's handle (the
            // v2-honoring carrier) but NOT Note B.
            commitAll(
                scratch.git,
                scratch.dir,
                `land ${id}\n\nTask-Card: ${id}\n\nkeep an inventory gate but bind preservation per decision item\n`,
            );
            const refused = retire(id);
            assert.equal(
                refused.ok,
                false,
                "PRIMARY: a record enumerating one of two owner notes must NOT retire",
            );
            assert.equal(
                refused.refusal.code,
                "decision_content_unpromoted",
                `PRIMARY: refusal code; got ${JSON.stringify(refused.refusal.code)}`,
            );
            assert.ok(
                Array.isArray(refused.refusal.items) &&
                    refused.refusal.items.includes(noteId(NOTE_2)),
                `PRIMARY: refusal must name the unslotted note's content-bound id; got ${JSON.stringify(refused.refusal.items)}`,
            );
            assert.ok(
                !refused.refusal.items.includes(noteId(NOTE_1)),
                `PRIMARY: the declared slot is not the named blocker; got ${JSON.stringify(refused.refusal.items)}`,
            );
            assert.equal(cardExists(isoRoot, id), true, "PRIMARY: card survives");
            assert.equal(
                reportExists(isoRoot, id),
                true,
                "PRIMARY: report survives",
            );
        }

        // =====================================================================
        // CONDITION 1 CORE — a carrier containing only the card id
        // satisfies NOTHING. Complete inventory, but the landing commit
        // body carries ONLY the trailer: every slot's handle is missing.
        // =====================================================================
        {
            const id = "dr-cardid-only";
            const NOTE =
                "OWNER-AUTH dr-cardid-only: the trailer alone must satisfy no item";
            writeWorkingCard(isoRoot, id, { owner_notes: [NOTE] });
            closeout(id, [
                {
                    id: noteId(NOTE),
                    disposition: "verbatim",
                    carrier: { kind: "commit" },
                    handle: NOTE,
                    provenance: "owner chat, 2026-10-27 — verbatim note",
                },
            ]);
            commitAll(
                scratch.git,
                scratch.dir,
                `land ${id}\n\nTask-Card: ${id}\n`,
            );
            const refused = retire(id);
            assert.equal(
                refused.ok,
                false,
                "card-id-only carrier must satisfy NOTHING",
            );
            assert.equal(refused.refusal.code, "decision_record_unresolved");
            assert.ok(
                (refused.refusal.problems || []).some(
                    (p) => p.code === "handle_not_in_carrier",
                ),
                `refusal must name handle_not_in_carrier; got ${JSON.stringify(refused.refusal.problems)}`,
            );
            assert.equal(cardExists(isoRoot, id), true, "card survives");
        }

        // =====================================================================
        // BASELINE LIFECYCLE MATRIX (owner condition 2).
        // =====================================================================
        // (A) Post-closeout ADDITION: a blocked review lands on a completed
        //     card after closeout (last_review + task_reviewed history
        //     event appear) -> new-item mismatch naming BOTH review items.
        //     Item ids are content-bound and derived from the card via the
        //     exported classifier, so the fixture tracks whatever contents
        //     the patched fields carry.
        {
            const id = "dr-add-after-closeout";
            writeWorkingCard(isoRoot, id);
            closeout(id, []);
            patchCard(isoRoot, id, {
                last_review: {
                    path: `.local/coordinator/reports/${id}/review.md`,
                    reviewed_at: "2026-10-27T00:00:00Z",
                    title: "Coordinator Review",
                    status: "blocked",
                    summary: "verdict: consequential objection stands; rework required",
                    next_action: "rework",
                },
                history: [
                    ...(readCard(isoRoot, id).history || []),
                    {
                        at: "2026-10-27T00:00:00Z",
                        event: "task_reviewed",
                        status: "blocked",
                        note: "verdict recorded: consequential objection stands",
                    },
                ],
            });
            const reviewItemIds = collectDecisionItems(readCard(isoRoot, id)).map(
                (item) => item.id,
            );
            const lastReviewId = reviewItemIds.find((x) =>
                x.startsWith("last_review["),
            );
            const reviewedEventId = reviewItemIds.find((x) =>
                x.startsWith("task_reviewed["),
            );
            assert.ok(
                Boolean(lastReviewId) && Boolean(reviewedEventId),
                `(A) classifier must enumerate both review items; got ${JSON.stringify(reviewItemIds)}`,
            );
            commitAll(scratch.git, scratch.dir, `land ${id}\n\nTask-Card: ${id}\n`);
            const refused = retire(id);
            assert.equal(
                refused.ok,
                false,
                "(A) post-closeout blocked review must block routine retirement",
            );
            assert.equal(refused.refusal.code, "decision_content_unpromoted");
            assert.ok(
                refused.refusal.items.includes(lastReviewId) &&
                    refused.refusal.items.includes(reviewedEventId),
                `(A) refusal must name last_review AND the history event (content-bound ids); got ${JSON.stringify(refused.refusal.items)}`,
            );
            assert.equal(cardExists(isoRoot, id), true, "(A) card survives");

            // The sanctioned remedy is the add-only re-closeout: stored
            // (empty) inventory + fresh slots for the review items ->
            // retire succeeds. (The full re-closeout mechanics — drop/
            // rewrite refusal — are pinned in the closeout suite.)
            const card = readCard(isoRoot, id);
            card.status = "working";
            card.active_session_alias = ALIAS;
            fs.writeFileSync(cardPath(isoRoot, id), JSON.stringify(card, null, 2));
            closeout(id, [
                {
                    id: lastReviewId,
                    disposition: "curated",
                    carrier: { kind: "commit" },
                    handle: label(id, lastReviewId, "curated"),
                },
                {
                    id: reviewedEventId,
                    disposition: "curated",
                    carrier: { kind: "commit" },
                    handle: label(id, reviewedEventId, "curated"),
                },
            ]);
            // The curated labels must exist in an anchored carrier: amend
            // the landing body via a new anchored commit.
            commitAll(
                scratch.git,
                scratch.dir,
                `amend ${id} record\n\nTask-Card: ${id}\n\n${label(id, lastReviewId, "curated")}: blocked on a consequential objection; rework required.\n${label(id, reviewedEventId, "curated")}: same verdict recorded in history.\n`,
            );
            const okAfterAmendment = retire(id);
            assert.equal(
                okAfterAmendment.ok,
                true,
                `(A) the sanctioned add-only amendment must unblock retirement; got ${JSON.stringify(okAfterAmendment.refusal || okAfterAmendment)}`,
            );
        }

        // (B) Post-closeout REMOVAL cannot launder: the record declares
        //     BOTH notes; the second is deleted from the card after
        //     closeout; Note B was never promoted -> the baseline slot
        //     still demands its carrier -> refused naming the item.
        {
            const id = "dr-remove-after-closeout";
            const NOTE_A =
                "OWNER-AUTH dr-remove: note A is properly promoted below";
            const NOTE_B =
                "OWNER-AUTH dr-remove: note B is deleted from the card post-closeout";
            writeWorkingCard(isoRoot, id, {
                owner_notes: [NOTE_A, NOTE_B],
            });
            closeout(id, [
                {
                    id: noteId(NOTE_A),
                    disposition: "verbatim",
                    carrier: { kind: "commit" },
                    handle: NOTE_A,
                    provenance: "owner chat, 2026-10-27 — verbatim note",
                },
                {
                    id: noteId(NOTE_B),
                    disposition: "verbatim",
                    carrier: { kind: "commit" },
                    handle: NOTE_B,
                    provenance: "owner chat, 2026-10-27 — verbatim note",
                },
            ]);
            // The laundering attempt: remove Note B from the card AND land
            // only Note A in the carrier.
            patchCard(isoRoot, id, { owner_notes: [NOTE_A] });
            commitAll(
                scratch.git,
                scratch.dir,
                `land ${id}\n\nTask-Card: ${id}\n\n${NOTE_A}\n`,
            );
            const refused = retire(id);
            assert.equal(
                refused.ok,
                false,
                "(B) removing an item from the card must NOT remove its slot",
            );
            assert.equal(refused.refusal.code, "decision_record_unresolved");
            assert.ok(
                (refused.refusal.problems || []).some(
                    (p) =>
                        p.code === "handle_not_in_carrier" &&
                        p.id === noteId(NOTE_B),
                ),
                `(B) refusal must name the removed note's missing carrier (content-bound id); got ${JSON.stringify(refused.refusal.problems)}`,
            );
            assert.equal(cardExists(isoRoot, id), true, "(B) card survives");

            // (B-flip) The honest resolution: promote Note B into an
            // anchored carrier too -> the baseline verifies and retirement
            // proceeds (no false block, no laundering).
            commitAll(
                scratch.git,
                scratch.dir,
                `docs: promote ${id} note B\n\nTask-Card: ${id}\n\n${NOTE_B}\n`,
            );
            const okB = retire(id);
            assert.equal(
                okB.ok,
                true,
                `(B-flip) promoted baseline must retire; got ${JSON.stringify(okB.refusal || okB)}`,
            );
        }

        // (C) blocked→clean review ANTI-LAUNDERING (the probe's pin,
        //     adapted to content-bound ids): an EARLIER blocked review
        //     event followed by a LATER clean approve must still refuse
        //     naming the earlier event's item — a later process record
        //     cannot erase an earlier consequential decision.
        {
            const id = "dr-launder";
            const BLOCKED_NOTE =
                "verdict recorded: consequential objection stands; rework required";
            writeWorkingCard(isoRoot, id);
            closeout(id, []);
            patchCard(isoRoot, id, {
                last_review: {
                    path: `.local/coordinator/reports/${id}/review.md`,
                    reviewed_at: "2026-10-27T00:00:00Z",
                    title: "Coordinator Review",
                    status: "completed",
                    summary: "Approve after rework. Clean now.",
                    next_action: "retire",
                },
                history: [
                    ...(readCard(isoRoot, id).history || []),
                    {
                        at: "2026-10-27T00:00:00Z",
                        event: "task_reviewed",
                        status: "blocked",
                        note: BLOCKED_NOTE,
                    },
                    {
                        at: "2026-10-27T00:10:00Z",
                        event: "task_reviewed",
                        status: "completed",
                        note: "clean approve after rework",
                    },
                ],
            });
            const blockedEventId = decisionItemId(
                "task_reviewed",
                `${BLOCKED_NOTE}\nstatus: blocked`,
            );
            commitAll(scratch.git, scratch.dir, `land ${id}\n\nTask-Card: ${id}\n`);
            const refused = retire(id);
            assert.equal(
                refused.ok,
                false,
                "(C) a later clean approve must NOT launder an earlier blocked review",
            );
            assert.equal(refused.refusal.code, "decision_content_unpromoted");
            assert.ok(
                refused.refusal.items.includes(blockedEventId),
                `(C) refusal must name the earlier blocked event's content-bound item; got ${JSON.stringify(refused.refusal.items)}`,
            );
            assert.ok(
                !refused.refusal.items.some((x) => x.startsWith("last_review[")),
                `(C) the later clean review (process record) must not be a named blocker; got ${JSON.stringify(refused.refusal.items)}`,
            );
            assert.equal(cardExists(isoRoot, id), true, "(C) card survives");
        }

        // (D) MULTIPLE consequential review events: two blocked events with
        //     DIFFERENT notes; the stored record declares ONLY the first ->
        //     the second (and last_review, also undeclared) is named —
        //     per-event enumeration, one declared event item never covers
        //     another.
        {
            const id = "dr-two-blocked";
            const FIRST_NOTE = "first blocked verdict";
            const SECOND_NOTE = "second blocked verdict on a different concern";
            writeCard(isoRoot, id, {
                status: "completed",
                last_review: {
                    path: `.local/coordinator/reports/${id}/review.md`,
                    reviewed_at: "2026-10-27T00:00:00Z",
                    title: "Coordinator Review",
                    status: "blocked",
                    summary: "second blocked verdict on a different concern",
                    next_action: "rework",
                },
                history: [
                    { at: "2026-10-27T00:00:00Z", event: "task_created", note: "x" },
                    {
                        at: "2026-10-27T00:01:00Z",
                        event: "task_reviewed",
                        status: "blocked",
                        note: FIRST_NOTE,
                    },
                    {
                        at: "2026-10-27T00:02:00Z",
                        event: "task_reviewed",
                        status: "blocked",
                        note: SECOND_NOTE,
                    },
                ],
                decision_record: {
                    version: 3,
                    items: [
                        {
                            id: decisionItemId(
                                "task_reviewed",
                                `${FIRST_NOTE}\nstatus: blocked`,
                            ),
                            disposition: "curated",
                            carrier: { kind: "commit" },
                            handle: label(
                                id,
                                decisionItemId(
                                    "task_reviewed",
                                    `${FIRST_NOTE}\nstatus: blocked`,
                                ),
                                "curated",
                            ),
                        },
                    ],
                },
            });
            commitAll(scratch.git, scratch.dir, `land ${id}\n\nTask-Card: ${id}\n`);
            const refused = retire(id);
            assert.equal(
                refused.ok,
                false,
                "(D) each consequential review event is its own item",
            );
            assert.ok(
                refused.refusal.items.includes(
                    decisionItemId(
                        "task_reviewed",
                        `${SECOND_NOTE}\nstatus: blocked`,
                    ),
                ),
                `(D) refusal must name the SECOND blocked event (content-bound id); got ${JSON.stringify(refused.refusal.items)}`,
            );
            assert.ok(
                !refused.refusal.items.includes(
                    decisionItemId(
                        "task_reviewed",
                        `${FIRST_NOTE}\nstatus: blocked`,
                    ),
                ),
                `(D) the DECLARED event item is not a named blocker; got ${JSON.stringify(refused.refusal.items)}`,
            );
        }

        // (D2) CONTENT-BOUND history identity (rewritten for the B-F1 fix;
        //      supersedes the positional-reorder pin). Identity follows
        //      CONTENT, not history index:
        //      (i) a hand-REORDER of history events changes no id (the id
        //          set is order-independent; nothing is lost by
        //          reordering) -> retirement PASSES;
        //      (ii) an in-place content REPLACEMENT of a review event mints
        //          a new id -> the replacement surfaces as an undeclared
        //          item -> retirement REFUSES naming it, card + report
        //          survive. Control (identical card, unedited note)
        //          retires, proving the refusal comes from the replacement,
        //          not a broken fixture.
        {
            const REVIEW_NOTE =
                "verdict recorded: reorder must not launder content identity";
            const REPLACEMENT_NOTE =
                "verdict REPLACED in place: smuggled decision content (B-F1 shape)";
            const reviewEventId = decisionItemId(
                "task_reviewed",
                `${REVIEW_NOTE}\nstatus: blocked`,
            );
            const mkHistory = () => [
                {
                    at: "2026-10-27T00:00:00Z",
                    event: "task_created",
                    note: "created",
                },
                {
                    at: "2026-10-27T00:01:00Z",
                    event: "task_reviewed",
                    status: "blocked",
                    note: REVIEW_NOTE,
                },
                {
                    at: "2026-10-27T00:02:00Z",
                    event: "status_updated",
                    status: "working",
                    note: "status bump between reviews",
                },
            ];
            const slotFor = (id) => ({
                id: reviewEventId,
                disposition: "curated",
                carrier: { kind: "commit" },
                handle: label(id, reviewEventId, "curated"),
            });

            // Control: the card as written -> retires.
            const idClean = "dr-reorder-clean";
            writeCard(isoRoot, idClean, {
                status: "completed",
                history: mkHistory(),
                decision_record: { version: 3, items: [slotFor(idClean)] },
            });
            commitAll(
                scratch.git,
                scratch.dir,
                `land ${idClean}\n\nTask-Card: ${idClean}\n\n${label(idClean, reviewEventId, "curated")}: blocked verdict; rework followed.\n`,
            );
            const okClean = retire(idClean);
            assert.equal(
                okClean.ok,
                true,
                `(D2 control) the unedited card must retire; got ${JSON.stringify(okClean.refusal || okClean)}`,
            );

            // (i) REORDER history[1] <-> history[2] by hand: the same
            // blocked event now sits at a different index, but its
            // CONTENT-BOUND id is unchanged -> retirement still PASSES
            // (identity is content; reordering loses nothing).
            const idSwap = "dr-reorder-swap";
            const h = mkHistory();
            writeCard(isoRoot, idSwap, {
                status: "completed",
                history: [h[0], h[2], h[1]],
                decision_record: { version: 3, items: [slotFor(idSwap)] },
            });
            commitAll(
                scratch.git,
                scratch.dir,
                `land ${idSwap}\n\nTask-Card: ${idSwap}\n\n${label(idSwap, reviewEventId, "curated")}: blocked verdict; rework followed.\n`,
            );
            const okSwap = retire(idSwap);
            assert.equal(
                okSwap.ok,
                true,
                `(D2-i) a hand-reordered history must still retire — identity is content, reordering loses nothing; got ${JSON.stringify(okSwap.refusal || okSwap)}`,
            );
            assert.equal(
                cardExists(isoRoot, idSwap),
                false,
                "(D2-i) reordered card is gone (nothing unpreserved)",
            );

            // (ii) REPLACE the review event's NOTE in place: the edited
            // event's content mints a NEW id; the stored slot's id matches
            // nothing on the card -> refusal names the REPLACEMENT item
            // (the undeclared new content), the card survives.
            const idReplace = "dr-reorder-replace";
            const h2 = mkHistory();
            h2[1].note = REPLACEMENT_NOTE;
            writeCard(isoRoot, idReplace, {
                status: "completed",
                history: h2,
                decision_record: { version: 3, items: [slotFor(idReplace)] },
            });
            commitAll(
                scratch.git,
                scratch.dir,
                `land ${idReplace}\n\nTask-Card: ${idReplace}\n\n${label(idReplace, reviewEventId, "curated")}: blocked verdict; rework followed.\n`,
            );
            const refusedReplace = retire(idReplace);
            assert.equal(
                refusedReplace.ok,
                false,
                "(D2-ii) an in-place content replacement must refuse retirement",
            );
            assert.equal(
                refusedReplace.refusal.code,
                "decision_content_unpromoted",
            );
            const replacementId = decisionItemId(
                "task_reviewed",
                `${REPLACEMENT_NOTE}\nstatus: blocked`,
            );
            assert.ok(
                refusedReplace.refusal.items.includes(replacementId),
                `(D2-ii) refusal must name the REPLACEMENT item's content-bound id; got ${JSON.stringify(refusedReplace.refusal.items)}`,
            );
            assert.ok(
                !refusedReplace.refusal.items.includes(reviewEventId),
                `(D2-ii) the stale declared id is not itself the named blocker; got ${JSON.stringify(refusedReplace.refusal.items)}`,
            );
            assert.equal(
                cardExists(isoRoot, idReplace),
                true,
                "(D2-ii) card survives",
            );
            assert.equal(
                reportExists(isoRoot, idReplace),
                true,
                "(D2-ii) report survives",
            );
        }

        // (E) POSITIVE two-slots-SAME-carrier (deferred pin, landed with
        //     the review-F1 round): two decision items, two verbatim slots,
        //     ONE shared commit carrier in search mode. Both handles sit in
        //     one anchored landing body; each slot verifies INDEPENDENTLY
        //     from the same carrier (owner condition 1: multiple slots may
        //     share one carrier) and retirement proceeds.
        {
            const id = "dr-shared-carrier";
            const NOTE_A =
                "OWNER-AUTH dr-shared-a: one anchored commit body may carry several items";
            const NOTE_B =
                "OWNER-AUTH dr-shared-b: each slot still verifies its own handle independently";
            writeWorkingCard(isoRoot, id, { owner_notes: [NOTE_A, NOTE_B] });
            closeout(id, [
                {
                    id: noteId(NOTE_A),
                    disposition: "verbatim",
                    carrier: { kind: "commit" },
                    handle: NOTE_A,
                    provenance: "owner chat, 2026-10-27 — verbatim note",
                },
                {
                    id: noteId(NOTE_B),
                    disposition: "verbatim",
                    carrier: { kind: "commit" },
                    handle: NOTE_B,
                    provenance: "owner chat, 2026-10-27 — verbatim note",
                },
            ]);
            const sharedSha = commitAll(
                scratch.git,
                scratch.dir,
                `land ${id}\n\nTask-Card: ${id}\n\n${NOTE_A}\n${NOTE_B}\n`,
            );
            // Direct verifier receipt: BOTH slots verified, SAME locator.
            const vres = verifyDecisionRecordForRetirement(
                id,
                readCard(isoRoot, id).decision_record,
                scratch.dir,
            );
            assert.equal(
                vres.ok,
                true,
                `(E) both slots must verify from one carrier; got ${JSON.stringify(vres.problems)}`,
            );
            assert.equal(
                vres.verified.length,
                2,
                `(E) exactly two verified entries; got ${JSON.stringify(vres.verified)}`,
            );
            assert.ok(
                vres.verified.every((v) => v.locator === sharedSha),
                `(E) every slot verified against the SAME commit; got ${JSON.stringify(vres.verified.map((v) => v.locator))}`,
            );
            const ok = retire(id);
            assert.equal(
                ok.ok,
                true,
                `(E) shared-carrier retirement must proceed; got ${JSON.stringify(ok.refusal || ok)}`,
            );
            assert.equal(cardExists(isoRoot, id), false, "(E) card gone");
            assert.equal(reportExists(isoRoot, id), false, "(E) report gone");
        }

        // =====================================================================
        // CARRIER NEGATIVES.
        // =====================================================================
        // (a) artifact path never committed -> path_never_committed, NAMED.
        {
            const id = "dr-neg-inline";
            const QUESTION =
                "Does an uncommitted local report path verify anything?";
            writeWorkingCard(isoRoot, id, {
                open_questions: [QUESTION],
            });
            closeout(id, [
                {
                    id: questionId(QUESTION),
                    disposition: "curated",
                    carrier: {
                        kind: "artifact",
                        path: ".local/coordinator/reports/" + id + "/closeout.md",
                    },
                    handle: label(id, questionId(QUESTION), "curated"),
                },
            ]);
            commitAll(scratch.git, scratch.dir, `land ${id}\n\nTask-Card: ${id}\n`);
            const refused = retire(id);
            assert.equal(refused.ok, false, "(a) local-report carrier must be refused");
            assert.equal(refused.refusal.code, "decision_record_unresolved");
            const msg = refused.refusal.message;
            assert.ok(
                msg.includes("slot #1") && msg.includes(questionId(QUESTION)),
                `(a) refusal must NAME the slot; got: ${msg}`,
            );
            assert.ok(
                msg.includes("path_never_committed") ||
                    msg.includes("never committed"),
                `(a) refusal must say the carrier was never committed; got: ${msg}`,
            );
            assert.equal(cardExists(isoRoot, id), true, "(a) card survives");
            assert.equal(reportExists(isoRoot, id), true, "(a) report survives");
        }

        // (b) label exists only in the WORKING TREE (committed blob lacks
        //     it) -> refused, working-tree content never counts.
        {
            const id = "dr-neg-worktree";
            const rel = "docs/checkpoints/2026-10-27-dr-neg-worktree.md";
            const q = "Is a working-tree back-ref enough for a curated slot?";
            writeWorkingCard(isoRoot, id, { open_questions: [q] });
            closeout(id, [
                {
                    id: questionId(q),
                    disposition: "curated",
                    carrier: { kind: "artifact", path: rel },
                    handle: label(id, questionId(q), "curated"),
                },
            ]);
            fs.mkdirSync(path.dirname(path.join(scratch.dir, rel)), {
                recursive: true,
            });
            fs.writeFileSync(
                path.join(scratch.dir, rel),
                "# checkpoint\n\nno label in the committed blob\n",
            );
            commitAll(
                scratch.git,
                scratch.dir,
                `docs: checkpoint without label\n\nTask-Card: ${id}\n`,
            );
            fs.appendFileSync(
                path.join(scratch.dir, rel),
                `\nTask-Card: ${id}\n${label(id, questionId(q), "curated")}\n`,
            );
            const refused = retire(id);
            assert.equal(refused.ok, false, "(b) working-tree-only label must be refused");
            assert.equal(refused.refusal.code, "decision_record_unresolved");
            assert.ok(
                (refused.refusal.problems || []).some(
                    (p) => p.code === "handle_not_in_carrier",
                ),
                `(b) refusal must name handle_not_in_carrier; got ${JSON.stringify(refused.refusal.problems)}`,
            );
            assert.equal(cardExists(isoRoot, id), true, "(b) card survives");
            scratch.git(["checkout", "--", rel]);
        }

        // (b3) b-F1 regression: artifact path IS A DIRECTORY containing a
        //      card-id-named file — the listing must never verify.
        {
            const id = "dr-neg-dir";
            const dirRel = "docs/coordination";
            const measured = "a directory listing is never a representation";
            writeWorkingCard(isoRoot, id);
            closeout(
                id,
                [
                    {
                        id: decisionItemId("measured_outcome", measured),
                        disposition: "verbatim",
                        carrier: { kind: "artifact", path: dirRel },
                        handle: measured,
                    },
                ],
                { measuredOutcome: measured },
            );
            fs.mkdirSync(path.join(scratch.dir, dirRel), { recursive: true });
            fs.writeFileSync(
                path.join(scratch.dir, dirRel, id),
                "filename bait — committed content carries no handle\n",
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
            assert.ok(
                refused.refusal.message.includes("path_is_directory_not_file_blob"),
                `(b3) refusal must name path_is_directory_not_file_blob; got: ${refused.refusal.message}`,
            );
            assert.equal(cardExists(isoRoot, id), true, "(b3) card survives");
        }

        // (c) F7: routine record + post-closeout open_questions -> mismatch.
        {
            const id = "dr-f7-openq";
            const F7_QUESTION =
                "Does the empty inventory contradict this non-empty question?";
            writeWorkingCard(isoRoot, id);
            closeout(id, []);
            patchCard(isoRoot, id, {
                open_questions: [F7_QUESTION],
            });
            commitAll(scratch.git, scratch.dir, `land ${id}\n\nTask-Card: ${id}\n`);
            const refused = retire(id);
            assert.equal(
                refused.ok,
                false,
                "F7: routine + non-empty open_questions must refuse retirement",
            );
            assert.equal(refused.refusal.code, "decision_content_unpromoted");
            assert.ok(
                refused.refusal.items.includes(questionId(F7_QUESTION)),
                `F7: refusal must name the question's content-bound id; got ${JSON.stringify(refused.refusal.items)}`,
            );
            assert.equal(cardExists(isoRoot, id), true, "F7: card survives");
        }

        // (d) FORCE — deletion proceeds AND the destructive warning NAMES
        //     items + slots, not boilerplate.
        {
            const id = "dr-neg-force";
            const FORCE_NOTE = "OWNER APPROVAL 2026-10-27: approved the retention fix.";
            writeCard(isoRoot, id, {
                status: "completed",
                owner_notes: [FORCE_NOTE],
                decision_record: {
                    version: 3,
                    items: [
                        {
                            id: noteId(FORCE_NOTE),
                            disposition: "verbatim",
                            carrier: {
                                kind: "artifact",
                                path: "docs/checkpoints/never-committed.md",
                            },
                            handle: FORCE_NOTE,
                            provenance: "owner chat, 2026-10-27 — verbatim note",
                        },
                    ],
                },
            });
            const forced = retire(id, { force: true });
            assert.equal(forced.ok, true, "(d) force must proceed");
            assert.equal(cardExists(isoRoot, id), false, "(d) card gone");
            assert.equal(reportExists(isoRoot, id), false, "(d) report gone");
            const destroyed = forced.removed.decision_content_destroyed;
            assert.ok(destroyed, "(d) force result must carry the destruction inventory");
            assert.ok(
                destroyed.items.some((i) => i.id === noteId(FORCE_NOTE)),
                `(d) inventory must name the card item (content-bound id); got ${JSON.stringify(destroyed.items)}`,
            );
            assert.ok(
                destroyed.slots.length === 1 &&
                    destroyed.slots[0].id === noteId(FORCE_NOTE) &&
                    destroyed.slots[0].disposition === "verbatim",
                `(d) inventory must name the declared slot; got ${JSON.stringify(destroyed.slots)}`,
            );
            const warning = forced.removed.destructive_warning;
            assert.ok(
                warning &&
                    warning.includes(noteId(FORCE_NOTE)) &&
                    warning.includes("OWNER APPROVAL 2026-10-27") &&
                    warning.includes("never-committed.md") &&
                    warning.includes("LAST LOCAL COPY"),
                `(d) destructive warning must NAME the content; got: ${warning}`,
            );
        }

        // =====================================================================
        // LEGACY COMPATIBILITY.
        // =====================================================================
        // (l1) Legacy v1 BARE/MARKER records with non-empty owner_notes ->
        //      refused (the C1 pins: presence of a marker never preserved
        //      content), with the re-closeout remedy named.
        {
            const decisionText =
                "OWNER 2026-10-26 (chat m0051): approved the bounded-correction path for the retention gate";
            const cases = [
                { id: "dr-c1-bare", entries: [{ kind: "commit" }] },
                {
                    id: "dr-c1-marker",
                    entries: [
                        { kind: "commit", marker: "dr-c1-marker decision" },
                    ],
                    landingBody: `land dr-c1-marker\n\nTask-Card: dr-c1-marker\n\ndr-c1-marker decision\n`,
                },
            ];
            for (const { id, entries, landingBody } of cases) {
                writeCard(isoRoot, id, {
                    status: "completed",
                    owner_notes: [decisionText],
                    decision_record: { version: 1, entries },
                });
                commitAll(
                    scratch.git,
                    scratch.dir,
                    landingBody || `land ${id}\n\nTask-Card: ${id}\n`,
                );
                const refused = retire(id);
                assert.equal(
                    refused.ok,
                    false,
                    `legacy v1 (${id}): marker presence must not preserve content`,
                );
                assert.equal(refused.refusal.code, "decision_content_unpromoted");
                assert.ok(
                    refused.refusal.items.includes(noteId(decisionText)),
                    `legacy v1 (${id}): refusal must name the item (content-bound id); got ${JSON.stringify(refused.refusal.items)}`,
                );
                assert.ok(
                    refused.refusal.message.includes("legacy version-1"),
                    `legacy v1 (${id}): refusal must name the legacy treatment; got: ${refused.refusal.message}`,
                );
                assert.equal(cardExists(isoRoot, id), true, `legacy v1 (${id}): card survives`);
            }
        }

        // (l2) Legacy v2 CARRIES record (excerpt-honoring) with non-empty
        //      owner_notes -> the SAME legacy rule: refuse with the
        //      re-closeout remedy (the v2 machinery is retired; this never
        //      weakens preservation — it refuses more, not less).
        {
            const id = "dr-legacy-v2-carries";
            const note =
                "OWNER 2026-10-26: approved the excerpt coverage rule temporarily";
            writeCard(isoRoot, id, {
                status: "completed",
                owner_notes: [note],
                decision_record: {
                    version: 1,
                    entries: [
                        {
                            kind: "commit",
                            carries: {
                                owner_notes:
                                    "approved the excerpt coverage rule temporarily",
                            },
                        },
                    ],
                },
            });
            commitAll(
                scratch.git,
                scratch.dir,
                `land ${id}\n\nTask-Card: ${id}\n\napproved the excerpt coverage rule temporarily\n`,
            );
            const refused = retire(id);
            assert.equal(
                refused.ok,
                false,
                "(l2) legacy v2 carries record with items present must refuse (legacy rule)",
            );
            assert.equal(refused.refusal.code, "decision_content_unpromoted");
            assert.ok(
                refused.refusal.message.includes("legacy version-1") &&
                    refused.refusal.message.includes("current v3 gate") &&
                    refused.refusal.message.includes("one slot per item"),
                `(l2) refusal must state the honest legacy treatment; got: ${refused.refusal.message}`,
            );
            assert.equal(cardExists(isoRoot, id), true, "(l2) card survives");
        }

        // (l3) Legacy v1/v2 records on a card with NO decision items ->
        //      retire routinely (routine case honored, zero ceremony).
        {
            const id = "dr-legacy-v1-clean";
            writeCard(isoRoot, id, {
                status: "completed",
                decision_record: {
                    version: 1,
                    entries: [{ kind: "routine", note: "nothing decision-class" }],
                },
            });
            commitAll(scratch.git, scratch.dir, `land ${id}\n\nTask-Card: ${id}\n`);
            const ok = retire(id);
            assert.equal(
                ok.ok,
                true,
                `(l3) legacy routine record on an item-free card must retire; got ${JSON.stringify(ok.refusal || ok)}`,
            );
        }

        // (l4) Legacy NO-record cards: refused iff decision items present;
        //      otherwise routine retirement (owner condition 5).
        {
            const idBad = "dr-neg-legacy";
            const LEGACY_NOTE = "OWNER APPROVAL 2026-10-25: approved.";
            const LEGACY_MEASURED = "gate refuses unpromoted retirement";
            writeCard(isoRoot, idBad, {
                status: "completed",
                owner_notes: [LEGACY_NOTE],
                measured_outcome: LEGACY_MEASURED,
            });
            commitAll(
                scratch.git,
                scratch.dir,
                `land ${idBad}\n\nTask-Card: ${idBad}\n`,
            );
            const refused = retire(idBad);
            assert.equal(refused.ok, false, "(l4) legacy unpromoted must be refused");
            assert.equal(refused.refusal.code, "decision_content_unpromoted");
            assert.deepEqual(
                [...refused.refusal.items].sort(),
                [
                    noteId(LEGACY_NOTE),
                    decisionItemId("measured_outcome", LEGACY_MEASURED),
                ].sort(),
                `(l4) refusal must name both items (content-bound ids); got ${JSON.stringify(refused.refusal.items)}`,
            );
            assert.equal(cardExists(isoRoot, idBad), true, "(l4) card survives");

            const idOk = "dr-legacy-clean";
            writeCard(isoRoot, idOk, { status: "completed" });
            commitAll(
                scratch.git,
                scratch.dir,
                `land ${idOk}\n\nTask-Card: ${idOk}\n`,
            );
            const ok = retire(idOk);
            assert.equal(ok.ok, true, "(l4) legacy clean retirement must proceed");
        }

        // =====================================================================
        // DEGRADED / STRUCTURAL refusals at retirement (raw shape).
        // =====================================================================
        // (s1) Empty v3 inventory + items on the card -> mismatch (the B1
        //      fail-open shape, closed by the inventory check instead of a
        //      non-empty-items structural rule).
        {
            const id = "dr-degraded-empty";
            const S1_NOTE = "OWNER APPROVAL 2026-10-25: approved.";
            writeCard(isoRoot, id, {
                status: "completed",
                owner_notes: [S1_NOTE],
                decision_record: { version: 3, items: [] },
            });
            commitAll(scratch.git, scratch.dir, `land ${id}\n\nTask-Card: ${id}\n`);
            const refused = retire(id);
            assert.equal(
                refused.ok,
                false,
                "(s1) empty inventory with items present must NOT verify",
            );
            assert.equal(refused.refusal.code, "decision_content_unpromoted");
            assert.ok(
                refused.refusal.items.includes(noteId(S1_NOTE)),
                `(s1) refusal must name the item (content-bound id); got ${JSON.stringify(refused.refusal.items)}`,
            );
            assert.equal(cardExists(isoRoot, id), true, "(s1) card survives");

            // Empty inventory on an item-free card retires (the routine
            // attestation is legitimate — that IS the routine path).
            const id2 = "dr-empty-clean";
            writeCard(isoRoot, id2, {
                status: "completed",
                decision_record: { version: 3, items: [] },
            });
            commitAll(
                scratch.git,
                scratch.dir,
                `land ${id2}\n\nTask-Card: ${id2}\n`,
            );
            const ok2 = retire(id2);
            assert.equal(
                ok2.ok,
                true,
                "(s1-flip) empty inventory on an item-free card must retire (routine)",
            );
        }

        // (s2) v3 record with a non-array items / junk keys -> degraded,
        //      refused on the RAW shape with the problem named (F2).
        {
            const id = "dr-degraded-shape";
            writeCard(isoRoot, id, {
                status: "completed",
                decision_record: { version: 3, items: [{ kind: "routine" }] },
            });
            commitAll(scratch.git, scratch.dir, `land ${id}\n\nTask-Card: ${id}\n`);
            const refused = retire(id);
            assert.equal(
                refused.ok,
                false,
                "(s2) malformed slots must be refused by the structural re-check",
            );
            assert.equal(refused.refusal.code, "decision_record_unresolved");
            assert.ok(
                refused.refusal.message.includes("id is required"),
                `(s2) refusal must name the structural problem; got: ${refused.refusal.message}`,
            );
            assert.equal(cardExists(isoRoot, id), true, "(s2) card survives");
        }
        {
            const id = "dr-f2-raw-junk";
            writeCard(isoRoot, id, {
                status: "completed",
                decision_record: {
                    version: 3,
                    items: [],
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
            assert.equal(refused.refusal.code, "decision_record_unresolved");
            assert.ok(
                refused.refusal.message.includes('unknown key "provenance"'),
                `F2: refusal must NAME the junk key on the RAW record; got: ${refused.refusal.message}`,
            );
            assert.equal(cardExists(isoRoot, id), true, "F2: card survives");
        }

        // =====================================================================
        // Ordering pin: landing proof runs FIRST (an unresolvable record
        // with NO trailer commit -> landing_not_confirmed).
        // =====================================================================
        {
            const id = "dr-order";
            writeCard(isoRoot, id, {
                status: "completed",
                decision_record: {
                    version: 3,
                    items: [
                        {
                            id: questionId("ordering-pin carrier question?"),
                            disposition: "curated",
                            carrier: { kind: "artifact", path: "docs/never.md" },
                            handle: label(id, questionId("ordering-pin carrier question?"), "curated"),
                        },
                    ],
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
        // DECLARED-REVISION carriers verified precisely.
        // =====================================================================
        {
            const id = "dr-pin";
            const NOTE = "OWNER APPROVAL: pinned-revision case, committed early.";
            const rel = "docs/checkpoints/2026-10-27-dr-pin.md";
            writeWorkingCard(isoRoot, id, { owner_notes: [NOTE] });
            fs.writeFileSync(path.join(scratch.dir, "pin.txt"), "pin\n");
            const pinSha = commitAll(
                scratch.git,
                scratch.dir,
                `decision content for ${id}\n\nTask-Card: ${id}\n${NOTE}\n`,
            );
            fs.mkdirSync(path.dirname(path.join(scratch.dir, rel)), {
                recursive: true,
            });
            fs.writeFileSync(
                path.join(scratch.dir, rel),
                `# pinned\n\nTask-Card: ${id}\n${NOTE}\n`,
            );
            const artSha = commitAll(
                scratch.git,
                scratch.dir,
                `docs: pinned artifact for ${id}\n\nTask-Card: ${id}\n`,
            );
            closeout(
                id,
                [
                    {
                        id: noteId(NOTE),
                        disposition: "verbatim",
                        carrier: { kind: "commit", revision: pinSha },
                        handle: NOTE,
                        provenance: "owner chat, 2026-10-27 — verbatim note",
                    },
                    {
                        id: decisionItemId("measured_outcome", NOTE),
                        disposition: "verbatim",
                        carrier: { kind: "artifact", path: rel, revision: artSha },
                        handle: NOTE,
                    },
                ],
                { measuredOutcome: NOTE },
            );
            const ok = retire(id);
            assert.equal(ok.ok, true, `pinned-revision retire should succeed: ${JSON.stringify(ok)}`);
            assert.equal(cardExists(isoRoot, id), false, "pinned card gone");

            // Pinned negative: the pinned commit lacks the handle ->
            // handle_not_in_carrier, card survives.
            {
                const idX = "dr-pin-nohandle";
                const noteX = "OWNER APPROVAL: handle-missing pinned-revision case.";
                writeWorkingCard(isoRoot, idX, { owner_notes: [noteX] });
                fs.writeFileSync(path.join(scratch.dir, "pinx.txt"), "pinx\n");
                const pinXSha = commitAll(
                    scratch.git,
                    scratch.dir,
                    `decision content for ${idX}\n\nTask-Card: ${idX}\n`,
                );
                closeout(idX, [
                    {
                        id: noteId(noteX),
                        disposition: "verbatim",
                        carrier: { kind: "commit", revision: pinXSha },
                        handle: noteX,
                        provenance: "owner chat, 2026-10-27 — verbatim note",
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
                    "pinned-revision handle miss must refuse",
                );
                assert.ok(
                    (refusedX.refusal.problems || []).some(
                        (p) => p.code === "handle_not_in_carrier",
                    ),
                    `pinned handle-miss must name handle_not_in_carrier; got ${JSON.stringify(refusedX.refusal.problems)}`,
                );
                assert.equal(
                    cardExists(isoRoot, idX),
                    true,
                    "pinned handle-miss card survives",
                );
            }

            // Unresolvable revision.
            const idU = "dr-pin-unresolvable";
            const UNRESOLVABLE_Q = "unresolvable pinned revision?";
            writeWorkingCard(isoRoot, idU, {
                open_questions: [UNRESOLVABLE_Q],
            });
            closeout(idU, [
                {
                    id: questionId(UNRESOLVABLE_Q),
                    disposition: "curated",
                    carrier: { kind: "commit", revision: "deadbeefdeadbeef" },
                    handle: label(idU, questionId(UNRESOLVABLE_Q), "curated"),
                },
            ]);
            commitAll(scratch.git, scratch.dir, `land ${idU}\n\nTask-Card: ${idU}\n`);
            const refusedU = retire(idU);
            assert.equal(refusedU.ok, false);
            assert.ok(
                refusedU.refusal.message.includes("revision_not_resolvable"),
                `unresolvable must be named; got: ${refusedU.refusal.message}`,
            );

            // Orphaned (reflog-only) revision: object exists, no branch has it.
            const idO = "dr-pin-orphan";
            const ORPHAN_Q = "orphaned carrier question?";
            writeWorkingCard(isoRoot, idO, {
                open_questions: [ORPHAN_Q],
            });
            const orphanNote = "orphaned carrier question?";
            fs.writeFileSync(path.join(scratch.dir, "orphan.txt"), "o\n");
            const orphanSha = commitAll(
                scratch.git,
                scratch.dir,
                `orphan decision for ${idO}\n\nTask-Card: ${idO}\n${label(idO, questionId(ORPHAN_Q), "curated")}\n`,
            );
            scratch.git(["reset", "--hard", "HEAD~1"]); // orphan it
            closeout(idO, [
                {
                    id: questionId(ORPHAN_Q),
                    disposition: "curated",
                    carrier: { kind: "commit", revision: orphanSha },
                    handle: label(idO, questionId(ORPHAN_Q), "curated"),
                },
            ]);
            commitAll(scratch.git, scratch.dir, `land ${idO}\n\nTask-Card: ${idO}\n`);
            const refusedO = retire(idO);
            assert.equal(refusedO.ok, false);
            assert.ok(
                refusedO.refusal.message.includes("revision_not_reachable"),
                `orphan must be named unreachable; got: ${refusedO.refusal.message}`,
            );
            assert.equal(cardExists(isoRoot, idO), true, "orphan card survives");
        }

        // =====================================================================
        // C2 regression (routine-after-review burden), via the REAL
        // sanctioned lifecycle: create -> ready -> working -> closeout
        // (routine) -> clean-approve review -> landing -> retirement.
        // A clean-approve review is a process record: zero promotion.
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
            closeout(id, [], { taskStatus: "reported" });
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

        // (c2 flip) The minimal card shape: routine record + blocked review
        //     -> refused naming BOTH review items; the same shape with a
        //     completed review retires.
        {
            const id = "dr-neg-postreview";
            const C2_NOTE = "verdict recorded: consequential objection stands";
            writeWorkingCard(isoRoot, id);
            closeout(id, []);
            patchCard(isoRoot, id, {
                last_review: {
                    path: `.local/coordinator/reports/${id}/review.md`,
                    reviewed_at: "2026-10-27T00:00:00Z",
                    title: "Coordinator Review",
                    status: "blocked",
                    summary: "verdict: consequential objection stands; rework required",
                    next_action: "rework",
                },
                history: [
                    ...(readCard(isoRoot, id).history || []),
                    {
                        at: "2026-10-27T00:00:00Z",
                        event: "task_reviewed",
                        status: "blocked",
                        note: C2_NOTE,
                    },
                ],
            });
            const c2EventId = decisionItemId(
                "task_reviewed",
                `${C2_NOTE}\nstatus: blocked`,
            );
            commitAll(scratch.git, scratch.dir, `land ${id}\n\nTask-Card: ${id}\n`);
            const refused = retire(id);
            assert.equal(
                refused.ok,
                false,
                "(c2) post-closeout non-approve review verdict must block routine retirement",
            );
            assert.equal(refused.refusal.code, "decision_content_unpromoted");
            assert.ok(
                refused.refusal.items.some((x) => x.startsWith("last_review[")) &&
                    refused.refusal.items.includes(c2EventId),
                `(c2) refusal must name last_review AND the history item (content-bound ids); got ${JSON.stringify(refused.refusal.items)}`,
            );
            assert.equal(cardExists(isoRoot, id), true, "(c2) card survives");

            const idOk = "dr-neg-postreview-approve";
            writeWorkingCard(isoRoot, idOk);
            closeout(idOk, []);
            patchCard(isoRoot, idOk, {
                last_review: {
                    path: `.local/coordinator/reports/${idOk}/review.md`,
                    reviewed_at: "2026-10-27T00:00:00Z",
                    title: "Coordinator Review",
                    status: "completed",
                    summary: "Approve. Routine work; nothing decision-class.",
                    next_action: "retire",
                },
                history: [
                    ...(readCard(isoRoot, idOk).history || []),
                    {
                        at: "2026-10-27T00:00:00Z",
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

// F1 — OPTION-INJECTION regression. A structurally-valid handle with a
// leading dash must be refused by the retirement gate's raw structural
// re-check (closeout refuses it at declaration); the git-layer verifier
// itself, handed a dash marker directly through a pinned revision, searches
// only the declared tree (`git grep -e` binding). Also pins: dash-prefixed
// carrier revisions fail closed (rev-parse refuses; structural gate refuses
// at declaration and retirement).
test("decision-record option injection (F1): dash handle refused structurally; git layer searches the declared tree only", () => {
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
        // literal "-v" anywhere, with a negative-control probe in the same
        // command shape the verifier uses.
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
            `F1 fixture negative control: committed tree at ${sha} must not contain "-v" (git grep exit ${f1FixtureProbe.status})`,
        );

        // (i) RETIREMENT LEVEL — a hand-edited stored record with a
        //     dash-prefixed handle is refused and NAMED, card + report
        //     SURVIVE.
        const id = "dr-f1-handle";
        const F1_NOTE = "OWNER 2026-10-27: approved the dash refusal rule";
        writeCard(isoRoot, id, {
            status: "completed",
            owner_notes: [F1_NOTE],
            decision_record: {
                version: 3,
                items: [
                    {
                        id: noteId(F1_NOTE),
                        disposition: "verbatim",
                        carrier: { kind: "commit", revision: sha },
                        handle: `-v ${F1_NOTE}`,
                        provenance: "owner chat, 2026-10-27 — verbatim note",
                    },
                ],
            },
        });
        commitAll(scratch.git, scratch.dir, `land ${id}\n\nTask-Card: ${id}\n`);
        const refused = retire(id);
        assert.equal(
            refused.ok,
            false,
            "F1(i): dash-prefixed stored handle must be refused",
        );
        assert.equal(
            refused.refusal.code,
            "decision_record_unresolved",
            `F1(i): refusal code; got ${JSON.stringify(refused.refusal.code)}`,
        );
        assert.ok(
            refused.refusal.message.includes('must not start with "-"') &&
                refused.refusal.message.includes("handle"),
            `F1(i): refusal must NAME the handle rule; got: ${refused.refusal.message}`,
        );
        assert.equal(cardExists(isoRoot, id), true, "F1(i): card survives");

        // Audit pin — dash-prefixed REVISION: git fails closed by itself
        // (rev-parse exits non-zero), so the verifier names
        // revision_not_resolvable.
        const dashRev = verifyDecisionRecordForRetirement(
            "dr-f1-rev",
            {
                version: 3,
                items: [
                    {
                        id: "owner_notes[fnv1a:0123456789abcdef]",
                        disposition: "verbatim",
                        carrier: { kind: "commit", revision: "-v" },
                        handle: "OWNER 2026-10-27: approved the dash refusal rule",
                        provenance: "owner chat, 2026-10-27 — verbatim note",
                    },
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

// D2 — git read failures in the decision-record verifier map to
// verifier_unavailable, NEVER to a confident miss. Uses the exported
// verifier directly (test seam): through deleteCoordinationTask these
// branches sit behind a successful landing proof, which cannot be staged in
// a repo whose git reads fail.
test("decision-record verifier: git read failures -> verifier_unavailable, not handle-absent", () => {
    const notARepo = fs.mkdtempSync(path.join(os.tmpdir(), "dr-notrepo-"));
    const corrupt = makeScratchRepo();
    try {
        // (1) Search-mode commit carrier against a NON-git directory: every
        //     `git log` exits 128 there -> verifier_unavailable.
        const searchMode = verifyDecisionRecordForRetirement(
            "dr-d2-search",
            {
                version: 3,
                items: [
                    {
                        id: "owner_notes[fnv1a:0123456789abcdef]",
                        disposition: "verbatim",
                        carrier: { kind: "commit" },
                        handle: "OWNER 2026-10-27: the search-mode carrier must fail closed",
                        provenance: "owner chat, 2026-10-27 — verbatim note",
                    },
                ],
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

        // (2) Declared-revision commit carrier whose commit is reachable
        //     but whose TREE object is unreadable (loose object deleted):
        //     `git grep` on the tree fails with a non-0/1 exit ->
        //     verifier_unavailable.
        fs.writeFileSync(path.join(corrupt.dir, "d2.txt"), "x\n");
        const sha = commitAll(
            corrupt.git,
            corrupt.dir,
            `d2 carrier\n\nTask-Card: dr-d2-tree\n`,
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
                version: 3,
                items: [
                    {
                        id: "owner_notes[fnv1a:0123456789abcdef]",
                        disposition: "verbatim",
                        carrier: { kind: "commit", revision: sha },
                        handle: "OWNER 2026-10-27: the tree read must fail closed",
                        provenance: "owner chat, 2026-10-27 — verbatim note",
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

// F3/F4 — tri-state git read helpers. A failed read must surface as
// available:false (mapped to verifier_unavailable at the call sites), never
// as a confident miss. Exported TEST SEAMS; the unavailable branches are
// injected via a NON-GIT directory.
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
            `F4: untracked path must read available DETERMINATE MISS (tracked:false), never as a failed read; got ${JSON.stringify(untracked)}`,
        );
        const deadBlob = committedBlobAt(notARepo, sha, "blob.txt");
        assert.ok(
            !deadBlob.available && deadBlob.blob === null,
            `F4: failed git read must read as UNAVAILABLE, never overstated as path-not-tracked; got ${JSON.stringify(deadBlob)}`,
        );

        // b-F1 shapes: directory paths are DETERMINATE MISSES carrying a
        // missReason — never tracked content.
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
            `F4(b-F1): directory path must be a DETERMINATE MISS with a directory missReason; got ${JSON.stringify(asDir)}`,
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
            `F4(b-F1): trailing-slash directory path must be a determinate miss; got ${JSON.stringify(asDirSlash)}`,
        );
    } finally {
        fs.rmSync(notARepo, { recursive: true, force: true });
        fs.rmSync(repo.dir, { recursive: true, force: true });
    }
});

// b-F2 — artifact SEARCH-MODE walk diagnosis honesty. When no carrier
// verifies AND at least one committed-blob read was UNAVAILABLE, the final
// diagnosis must be verifier_unavailable — the negative search is
// INCOMPLETE — never an overstated handle-absent claim.
test("decision-record artifact search walk: unavailable read -> verifier_unavailable, not handle-absent (b-F2)", () => {
    const repo = makeScratchRepo();
    try {
        const rel = "docs/decisions/d3.md";
        fs.mkdirSync(path.dirname(path.join(repo.dir, rel)), {
            recursive: true,
        });
        fs.writeFileSync(path.join(repo.dir, rel), "v1 no label\n");
        commitAll(repo.git, repo.dir, "d3 v1\n");
        fs.writeFileSync(path.join(repo.dir, rel), "v2 no label\n");
        const newestSha = commitAll(repo.git, repo.dir, "d3 v2\n");
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
                version: 3,
                items: [
                    {
                        id: "owner_notes[fnv1a:0123456789abcdef]",
                        disposition: "curated",
                        carrier: { kind: "artifact", path: rel },
                        handle: label("dr-bf2-walk", "owner_notes[fnv1a:0123456789abcdef]", "curated"),
                    },
                ],
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
            `b-F2: an incomplete negative search must diagnose verifier_unavailable; got ${JSON.stringify(res.problems[0])}`,
        );
        assert.ok(
            res.problems[0].detail.includes("NOT established"),
            `b-F2: detail must state absence is not established; got: ${res.problems[0].detail}`,
        );
    } finally {
        fs.rmSync(repo.dir, { recursive: true, force: true });
    }
});

// Review F1 (2026-10-27) — CROSS-REVISION SCATTER FAIL-OPEN, closed. The
// artifact search walk verified only when the card-id anchor AND the item
// handle co-occurred in the SAME revision, but its terminal problems fired
// only on !anchorEver / !handleEver — per-revision accumulators. Anchor-alone
// in revision X + handle-alone in revision Y therefore emitted ZERO problems,
// read as ok:true, and deleteCoordinationTask destroyed the card with no
// same-carrier representation. This was the hole left by deleting the v2
// scatter diagnosis (the round-1 report wrongly called that pin
// "semantic-dead"; the review proved otherwise). Pins:
//   (a) the verbatim scatter shape — anchor-only revision + handle-only
//       revision -> anchor_handle_scattered naming BOTH revisions, refusal
//       at retirement, card + report survive; then ONE reunifying revision
//       (anchor + handle together) -> the same fixture verifies (the
//       same-revision positive stays green on the same repo).
//   (b) commit-mode split evidence CANNOT verify: the search is
//       pre-filtered to card-id-anchored messages (--grep=<card-id>), so a
//       handle in an UN-anchored message is never a candidate — the outcome
//       is the terminal handle_not_in_carrier, never ok. The audit
//       conclusion for the declared-revision paths (commit + artifact):
//       both read ONE pinned revision/blob, so per-revision accumulators
//       cannot let split evidence verify there by construction.
test("decision-record review-F1 scatter: split evidence across revisions refuses, never verifies", () => {
    const isoRoot = fs.mkdtempSync(path.join(os.tmpdir(), "dr-f1rev-iso-"));
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
        // (a) ARTIFACT SCATTER — anchor-only revision, then handle-only
        //     revision of the same durable path.
        const idA = "dr-scatter";
        const rel = "docs/checkpoints/2026-10-27-dr-scatter.md";
        const HANDLE =
            "the handle line rides alone in this revision with no anchor";
        fs.mkdirSync(path.dirname(path.join(scratch.dir, rel)), {
            recursive: true,
        });
        fs.writeFileSync(
            path.join(scratch.dir, rel),
            `# scatter v1\n\nTask-Card: ${idA}\n\nno item representation here\n`,
        );
        const anchorSha = commitAll(
            scratch.git,
            scratch.dir,
            "docs: anchor-only revision\n",
        );
        fs.writeFileSync(
            path.join(scratch.dir, rel),
            `# scatter v2\n\n${HANDLE}\n`,
        );
        const handleSha = commitAll(
            scratch.git,
            scratch.dir,
            "docs: handle-only revision\n",
        );

        const direct = verifyDecisionRecordForRetirement(
            idA,
            {
                version: 3,
                items: [
                    {
                        id: "owner_notes[fnv1a:0123456789abcdef]",
                        disposition: "verbatim",
                        carrier: { kind: "artifact", path: rel },
                        handle: HANDLE,
                        provenance: "owner chat, 2026-10-27 — verbatim note",
                    },
                ],
            },
            scratch.dir,
        );
        assert.equal(
            direct.ok,
            false,
            "(a) anchor-only revision + handle-only revision must NOT verify",
        );
        assert.equal(
            direct.problems.length,
            1,
            `(a) exactly one terminal problem; got ${JSON.stringify(direct.problems)}`,
        );
        assert.equal(
            direct.problems[0].code,
            "anchor_handle_scattered",
            `(a) the scatter diagnosis must fire; got ${JSON.stringify(direct.problems[0])}`,
        );
        assert.ok(
            direct.problems[0].detail.includes(anchorSha) &&
                direct.problems[0].detail.includes(handleSha),
            `(a) the diagnosis must NAME both revisions (${anchorSha} / ${handleSha}); got: ${direct.problems[0].detail}`,
        );

        // Full retirement path on the same shape: refusal, survival.
        writeCard(isoRoot, idA, {
            status: "completed",
            owner_notes: [HANDLE],
            decision_record: {
                version: 3,
                items: [
                    {
                        id: noteId(HANDLE),
                        disposition: "verbatim",
                        carrier: { kind: "artifact", path: rel },
                        handle: HANDLE,
                        provenance: "owner chat, 2026-10-27 — verbatim note",
                    },
                ],
            },
        });
        commitAll(
            scratch.git,
            scratch.dir,
            `land ${idA}\n\nTask-Card: ${idA}\n`,
        );
        const refusedA = retire(idA);
        assert.equal(
            refusedA.ok,
            false,
            "(a) retirement through the scatter shape must refuse",
        );
        assert.equal(refusedA.refusal.code, "decision_record_unresolved");
        assert.ok(
            (refusedA.refusal.problems || []).some(
                (p) => p.code === "anchor_handle_scattered",
            ),
            `(a) refusal must carry anchor_handle_scattered; got ${JSON.stringify(refusedA.refusal.problems)}`,
        );
        assert.equal(
            cardExists(isoRoot, idA),
            true,
            "(a) card survives the scatter refusal",
        );
        assert.equal(
            reportExists(isoRoot, idA),
            true,
            "(a) report survives the scatter refusal",
        );

        // (a-flip) The same-revision positive on the SAME repo+path: one
        //     more commit reunites anchor + handle in one committed blob ->
        //     the walk verifies at that revision. Pre-F1 this flip is what
        //     made the hole invisible: the positive was tested, the split
        //     shape was not.
        fs.writeFileSync(
            path.join(scratch.dir, rel),
            `# scatter v3\n\nTask-Card: ${idA}\n\n${HANDLE}\n`,
        );
        const reunitedSha = commitAll(
            scratch.git,
            scratch.dir,
            "docs: reunite anchor and handle in one blob\n",
        );
        const flip = verifyDecisionRecordForRetirement(
            idA,
            {
                version: 3,
                items: [
                    {
                        id: "owner_notes[fnv1a:0123456789abcdef]",
                        disposition: "verbatim",
                        carrier: { kind: "artifact", path: rel },
                        handle: HANDLE,
                        provenance: "owner chat, 2026-10-27 — verbatim note",
                    },
                ],
            },
            scratch.dir,
        );
        assert.equal(
            flip.ok,
            true,
            `(a-flip) a single revision carrying BOTH must verify; got ${JSON.stringify(flip.problems)}`,
        );
        assert.equal(flip.verified.length, 1);
        assert.equal(
            flip.verified[0].locator,
            `${reunitedSha}:${rel}`,
            `(a-flip) verification must bind to the reunited revision; got ${JSON.stringify(flip.verified[0])}`,
        );
        const retired = retire(idA);
        assert.equal(
            retired.ok,
            true,
            `(a-flip) the reunified card must retire; got ${JSON.stringify(retired.refusal || retired)}`,
        );

        // (b) COMMIT-MODE split evidence: the handle exists ONLY in an
        //     un-anchored commit message while an anchored (trailer-bearing)
        //     message exists without it. The candidate set is pre-filtered
        //     by --grep=<card-id>, so the un-anchored handle can never
        //     count: terminal handle_not_in_carrier, never ok. (This pins
        //     the audit conclusion that the commit search mode cannot
        //     scatter-fail-open; the same-revision positive for this mode
        //     is pinned by (E) in the main suite.)
        const idB = "dr-f1b-split";
        // The handle must NOT contain the card id: a handle embedding the
        // id anchors its own carrier by discoverability (by design), which
        // would defeat the un-anchored-carrier shape under test.
        const HANDLE_B =
            "OWNER-AUTH split evidence: this handle is parked in an unanchored message";
        commitAll(
            scratch.git,
            scratch.dir,
            `land ${idB}\n\nTask-Card: ${idB}\n`,
        );
        commitAll(
            scratch.git,
            scratch.dir,
            `unrelated work\n\n${HANDLE_B}\n`,
        );
        const splitB = verifyDecisionRecordForRetirement(
            idB,
            {
                version: 3,
                items: [
                    {
                        id: "owner_notes[fnv1a:0123456789abcdef]",
                        disposition: "verbatim",
                        carrier: { kind: "commit" },
                        handle: HANDLE_B,
                        provenance: "owner chat, 2026-10-27 — verbatim note",
                    },
                ],
            },
            scratch.dir,
        );
        assert.equal(
            splitB.ok,
            false,
            "(b) commit-mode split evidence must NOT verify",
        );
        assert.equal(
            splitB.problems.length,
            1,
            `(b) exactly one terminal problem; got ${JSON.stringify(splitB.problems)}`,
        );
        assert.equal(
            splitB.problems[0].code,
            "handle_not_in_carrier",
            `(b) an un-anchored handle is a handle miss, never a pass; got ${JSON.stringify(splitB.problems[0])}`,
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

// PROBE PIN 1 (preserved from the 2026-10-26 probe, adapted to v3):
// ARTIFACT card-id-only recovery CHAIN — a fresh session knowing ONLY the
// card id (not the path): pickaxe discovery -> changed-file path derivation
// -> committed blob read -> content assert; absent-content control proves
// the assertions can fail (non-vacuous).
test("decision-record recovery: artifact content from card-id alone via pickaxe -> changed-file path derivation -> committed blob", () => {
    const isoRoot = fs.mkdtempSync(path.join(os.tmpdir(), "dr-artrec-iso-"));
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
    const recoverFromCardId = (repo, id) => {
        const shas = repo
            .git([
                "log",
                "--branches",
                "--format=%H",
                `-S${id}`,
            ])
            .stdout.trim()
            .split(/\r?\n/)
            .filter(Boolean);
        const found = [];
        for (const sha of shas) {
            const files = repo
                .git([
                    "diff-tree",
                    "--no-commit-id",
                    "--name-only",
                    "-r",
                    sha,
                ])
                .stdout.trim()
                .split(/\r?\n/)
                .filter(Boolean);
            for (const file of files) {
                const blob = repo.gitRaw(["show", `${sha}:${file}`]);
                if (blob.status === 0 && blob.stdout.includes(id)) {
                    found.push({ sha, path: file, blob: blob.stdout });
                }
            }
        }
        return found;
    };
    try {
        commitAll(scratch.git, scratch.dir, "initial: no trailer");

        const ID = "dr-art-rec";
        const OWNER_AUTH =
            "OWNER-AUTH dr-art-rec: artifact recovery chain owner approved 2026-10-26";
        const DECISION =
            "DECISION dr-art-rec: durable decision content lives in a committed artifact file";
        const ARTIFACT_REL = "researches/decisions/2026-10-27-dr-art-rec.md";

        writeWorkingCard(isoRoot, ID, { owner_notes: [OWNER_AUTH, DECISION] });
        closeout(ID, [
            {
                id: noteId(OWNER_AUTH),
                disposition: "verbatim",
                carrier: { kind: "artifact", path: ARTIFACT_REL },
                handle: OWNER_AUTH,
                provenance: "owner chat, 2026-10-26 — verbatim note",
            },
            {
                id: noteId(DECISION),
                disposition: "verbatim",
                carrier: { kind: "artifact", path: ARTIFACT_REL },
                handle: DECISION,
                provenance: "owner chat, 2026-10-26 — verbatim note",
            },
        ]);
        fs.mkdirSync(
            path.dirname(path.join(scratch.dir, ARTIFACT_REL)),
            { recursive: true },
        );
        fs.writeFileSync(
            path.join(scratch.dir, ARTIFACT_REL),
            `# Decision record ${ID}\n\nTask-Card: ${ID}\n\n${OWNER_AUTH}\n${DECISION}\n`,
        );
        commitAll(
            scratch.git,
            scratch.dir,
            `docs: promote ${ID} decision record\n\nTask-Card: ${ID}\n`,
        );
        commitAll(scratch.git, scratch.dir, `land ${ID}\n\nTask-Card: ${ID}\n`);

        const done = retire(ID);
        assert.equal(
            done.ok,
            true,
            `probe-pin-1 setup: per-item artifact carriers must retire; got ${JSON.stringify(done.refusal || done)}`,
        );
        assert.equal(cardExists(isoRoot, ID), false, "card gone");
        assert.equal(reportExists(isoRoot, ID), false, "report dir gone");

        const recovered = recoverFromCardId(scratch, ID);
        assert.ok(
            recovered.length >= 1,
            "chain: pickaxe -> changed files must discover at least one id-bearing committed blob",
        );
        assert.ok(
            recovered.some(
                (r) =>
                    r.path === ARTIFACT_REL &&
                    r.blob.includes(`Task-Card: ${ID}`) &&
                    r.blob.includes(OWNER_AUTH) &&
                    r.blob.includes(DECISION),
            ),
            `chain: the blob at the DERIVED path must carry back-ref + BOTH items' verbatim content; got ${JSON.stringify(recovered.map((r) => r.path))}`,
        );

        // Absent-content control.
        {
            const bare = makeScratchRepo();
            try {
                commitAll(
                    bare.git,
                    bare.dir,
                    `land dr-art-rec with no preserved content\n\nTask-Card: dr-art-rec\n`,
                );
                const control = recoverFromCardId(bare, "dr-art-rec");
                assert.equal(
                    control.length,
                    0,
                    `control: the chain must recover NOTHING where the artifact was never committed; got ${JSON.stringify(control.map((r) => r.path))}`,
                );
            } finally {
                fs.rmSync(bare.dir, { recursive: true, force: true });
            }
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

// B-F1 (2026-10-27 fix round) — SANCTIONED LAUNDERING CHAIN VIA POSITIONAL
// ITEM IDENTITY, closed by content-bound identity. The pre-fix chain (all
// sanctioned ops, no force, no hand-editing):
//   (1) close out a card `reported` with owner_notes=[A] and a verbatim
//       slot for A;
//   (2) commit A's carrier (landing trailer + A's handle);
//   (3) /task-update REPLACES owner_notes[0] with B (follow-up-allowlisted
//       on reported);
//   (4) /task-review resolves reported -> completed with no re-closeout;
//   (5) retire — pre-fix this PASSED: B matched A's stored slot BY
//       POSITION, the carrier check validated A's old committed handle,
//       and decision B (never preserved) was destroyed with the card.
// Post-fix the item id is a content hash, so step (3) mints a NEW id and
// step (5) refuses naming the undeclared replacement; the sanctioned
// resolution is the add-only re-closeout declaring B (plus the reopen
// review's own decision items), after which retirement succeeds. The whole
// chain runs through the REAL ops — saveCoordinationTask,
// readyCoordinationTask, activateCoordinationTask,
// updateCoordinationTaskMetadata, saveCoordinationTaskCloseout,
// reviewCoordinationTask, deleteCoordinationTask — never card-JSON
// patching.
test("B-F1 content-bound identity: sanctioned /task-update replacement chain refused, add-only amendment retires", () => {
    const isoRoot = fs.mkdtempSync(path.join(os.tmpdir(), "dr-bf1-iso-"));
    const prevCoordRoot = process.env.OPENCODE_LOCAL_COORDINATOR_ROOT;
    process.env.OPENCODE_LOCAL_COORDINATOR_ROOT = isoRoot;
    bindSessionName(SESSION_ID, ALIAS);
    const scratch = makeScratchRepo();

    const retire = (id) =>
        deleteCoordinationTask(SESSION_ID, id, {
            cwd: "/verification",
            landingProofRepo: scratch.dir,
        });

    try {
        commitAll(scratch.git, scratch.dir, "initial: no trailer");

        const NOTE_A =
            "OWNER-AUTH bf1: original authorization approved 2026-10-27 (chat m9999)";
        const NOTE_B =
            "OWNER-AUTH bf1: replacement decision smuggled in via /task-update after closeout";

        // (1) REAL lifecycle into working: create -> ready (F3 envelope) ->
        //     activate -> /task-update sets owner_notes=[A].
        const created = saveCoordinationTask(SESSION_ID, {
            title: "B-F1 laundering chain fixture",
            task_type: "implementation",
            coordination_mode: "short",
            primary_lane: "harness-core",
            status: "draft",
            rough_scope: [".opencode/scripts/state-lib.js"],
            files_in_scope: ["tests/scripts/task-retire-decision-record.test.js"],
            success_criteria: ["the sanctioned replacement chain cannot retire"],
            validation_plan: [
                "node --test tests/scripts/task-retire-decision-record.test.js",
            ],
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
        updateCoordinationTaskMetadata(
            SESSION_ID,
            id,
            { owner_notes: [NOTE_A] },
            { cwd: "/verification" },
        );

        // (2) Reported closeout with a verbatim slot for A; then A's
        //     carrier (landing trailer + A's handle in the body).
        closeout(
            id,
            [
                {
                    id: noteId(NOTE_A),
                    disposition: "verbatim",
                    carrier: { kind: "commit" },
                    handle: NOTE_A,
                    provenance: "owner chat m9999, 2026-10-27 — verbatim note",
                },
            ],
            { taskStatus: "reported" },
        );
        commitAll(
            scratch.git,
            scratch.dir,
            `land ${id}\n\nTask-Card: ${id}\n\n${NOTE_A}\n`,
        );

        // (3) THE LAUNDERING STEP — sanctioned /task-update REPLACES the
        //     note content in place (owner_notes is follow-up-allowlisted
        //     on a reported card). Pre-fix this recycled position 0 onto
        //     different content; post-fix it mints a new content id.
        updateCoordinationTaskMetadata(
            SESSION_ID,
            id,
            { owner_notes: [NOTE_B] },
            { cwd: "/verification" },
        );

        // (4) Sanctioned review resolves reported -> completed with no
        //     re-closeout (last_review.status=completed is a process
        //     record — no new item).
        const review = reviewCoordinationTask(SESSION_ID, id, {
            body: "Approve replacement handling; complete.",
            taskStatus: "completed",
            title: "Coordinator Review",
            cwd: "/verification",
        });
        assert.equal(
            review.task.status,
            "completed",
            "B-F1 setup: review resolves the card to completed",
        );

        // (5) RETIREMENT MUST REFUSE naming the REPLACEMENT (undeclared)
        //     item; the removed original's slot is kept (its carrier still
        //     verifies) and is NOT the named blocker. Card + report
        //     SURVIVE.
        const refused = retire(id);
        assert.equal(
            refused.ok,
            false,
            "B-F1: the sanctioned replacement chain must NOT retire",
        );
        assert.equal(
            refused.refusal.code,
            "decision_content_unpromoted",
            `B-F1: refusal code; got ${JSON.stringify(refused.refusal.code)}`,
        );
        assert.ok(
            Array.isArray(refused.refusal.items) &&
                refused.refusal.items.includes(noteId(NOTE_B)),
            `B-F1: refusal must name the REPLACEMENT item's content-bound id; got ${JSON.stringify(refused.refusal.items)}`,
        );
        assert.ok(
            !refused.refusal.items.includes(noteId(NOTE_A)),
            `B-F1: the removed original (kept-slot) is not the named blocker; got ${JSON.stringify(refused.refusal.items)}`,
        );
        assert.equal(cardExists(isoRoot, id), true, "B-F1: card survives");
        assert.equal(reportExists(isoRoot, id), true, "B-F1: report survives");

        // (6) THE SANCTIONED RESOLUTION — add-only re-closeout declaring B
        //     (with a real carrier), all through real ops. Reopen
        //     completed -> ready (a non-approve verdict: its own decision
        //     items appear), activate ready -> working (the design digest
        //     is unchanged by note edits, so the F3 dispatch backstop
        //     passes), re-closeout carrying the stored A slot VERBATIM plus
        //     fresh slots for B and the reopen-review items.
        reviewCoordinationTask(SESSION_ID, id, {
            body: "Reopen for the add-only amendment: declare the replacement owner note.",
            taskStatus: "ready",
            title: "Coordinator Review",
            cwd: "/verification",
        });
        activateCoordinationTask(SESSION_ID, id, { cwd: "/verification" });
        const card = readCard(isoRoot, id);
        const storedSlots = card.decision_record.items;
        assert.equal(
            storedSlots.length,
            1,
            "B-F1 amendment setup: the stored baseline is exactly A's slot",
        );
        const currentIds = collectDecisionItems(card).map((item) => item.id);
        const bId = currentIds.find((x) => x.startsWith("owner_notes["));
        const lastReviewId = currentIds.find((x) => x.startsWith("last_review["));
        const reviewedEventId = currentIds.find((x) =>
            x.startsWith("task_reviewed["),
        );
        assert.ok(
            Boolean(bId) && Boolean(lastReviewId) && Boolean(reviewedEventId),
            `B-F1 amendment setup: B + both reopen-review items enumerated; got ${JSON.stringify(currentIds)}`,
        );
        closeout(
            id,
            [
                // Stored baseline carried forward VERBATIM (add-only).
                ...storedSlots,
                {
                    id: bId,
                    disposition: "verbatim",
                    carrier: { kind: "commit" },
                    handle: NOTE_B,
                    provenance: "owner chat m9999, 2026-10-27 — replacement note",
                },
                {
                    id: lastReviewId,
                    disposition: "curated",
                    carrier: { kind: "commit" },
                    handle: label(id, lastReviewId, "curated"),
                },
                {
                    id: reviewedEventId,
                    disposition: "curated",
                    carrier: { kind: "commit" },
                    handle: label(id, reviewedEventId, "curated"),
                },
            ],
            { taskStatus: "reported" },
        );
        // One anchored amendment commit carries B's verbatim text and both
        // curated labels.
        commitAll(
            scratch.git,
            scratch.dir,
            `amend ${id} record\n\nTask-Card: ${id}\n\n${NOTE_B}\n${label(id, lastReviewId, "curated")}: reopened to declare the replacement owner note.\n${label(id, reviewedEventId, "curated")}: same reopen verdict recorded in history.\n`,
        );
        const finalReview = reviewCoordinationTask(SESSION_ID, id, {
            body: "Approve. Amendment declared; retire.",
            taskStatus: "completed",
            title: "Coordinator Review",
            cwd: "/verification",
        });
        assert.equal(
            finalReview.task.status,
            "completed",
            "B-F1 amendment: final review resolves to completed",
        );

        // (7) Retirement now succeeds: A preserved by its kept slot, B
        //     declared with a real carrier, the reopen-review items
        //     declared. Card + report are deleted.
        const done = retire(id);
        assert.equal(
            done.ok,
            true,
            `B-F1: the sanctioned amendment must unblock retirement; got ${JSON.stringify(done.refusal || done)}`,
        );
        assert.equal(cardExists(isoRoot, id), false, "B-F1: card retired");
        assert.equal(reportExists(isoRoot, id), false, "B-F1: report retired");
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

// A-F3 (2026-10-27 fix round, folded in with the B-F1 state-lib change) —
// NAMED COMMIT-SEARCH SCATTER, through the FULL retirement op: commit X
// carries the card-id anchor ONLY, commit Y carries the handle ONLY (in an
// un-anchored message). Search mode must require BOTH in the SAME
// reachable commit message: the terminal diagnosis is
// handle_not_in_carrier (never a pass — the split satisfies nothing), the
// card + report SURVIVE, and the reunified flip (one anchored commit
// carrying anchor + handle) retires. Mirrors the review-F1 artifact-mode
// scatter pin for the commit-search mode at the retirement-op level.
test("decision-record A-F3 commit-search scatter: anchor-only commit + handle-only commit -> terminal handle_not_in_carrier, never ok", () => {
    const isoRoot = fs.mkdtempSync(path.join(os.tmpdir(), "dr-af3-iso-"));
    const prevCoordRoot = process.env.OPENCODE_LOCAL_COORDINATOR_ROOT;
    process.env.OPENCODE_LOCAL_COORDINATOR_ROOT = isoRoot;
    bindSessionName(SESSION_ID, ALIAS);
    const scratch = makeScratchRepo();

    const retire = (id) =>
        deleteCoordinationTask(SESSION_ID, id, {
            cwd: "/verification",
            landingProofRepo: scratch.dir,
        });

    try {
        commitAll(scratch.git, scratch.dir, "initial: no trailer");

        const id = "dr-af3-scatter";
        // The handle must NOT contain the card id (an id-embedding handle
        // would anchor its own carrier by discoverability and defeat the
        // split shape under test).
        const HANDLE =
            "OWNER-AUTH af3: split evidence parked across two different commits";
        writeWorkingCard(isoRoot, id, { owner_notes: [HANDLE] });
        // Real closeout (the gate accepts the slot: the handle is drawn
        // from the note's own content); search-mode commit carrier — the
        // carrier cannot be sha-pinned at closeout time.
        closeout(id, [
            {
                id: noteId(HANDLE),
                disposition: "verbatim",
                carrier: { kind: "commit" },
                handle: HANDLE,
                provenance: "owner chat, 2026-10-27 — verbatim note",
            },
        ]);

        // Commit X: card-id anchor ONLY. Commit Y: handle ONLY, un-anchored.
        commitAll(
            scratch.git,
            scratch.dir,
            `land ${id}\n\nTask-Card: ${id}\n`,
        );
        commitAll(
            scratch.git,
            scratch.dir,
            `unrelated work\n\n${HANDLE}\n`,
        );

        const refused = retire(id);
        assert.equal(
            refused.ok,
            false,
            "A-F3: scattered anchor/handle across commits must NOT retire",
        );
        assert.equal(
            refused.refusal.code,
            "decision_record_unresolved",
            `A-F3: refusal code; got ${JSON.stringify(refused.refusal.code)}`,
        );
        assert.ok(
            (refused.refusal.problems || []).some(
                (p) => p.code === "handle_not_in_carrier",
            ),
            `A-F3: refusal must carry the TERMINAL handle_not_in_carrier diagnosis (never ok); got ${JSON.stringify(refused.refusal.problems)}`,
        );
        assert.equal(cardExists(isoRoot, id), true, "A-F3: card survives");
        assert.equal(reportExists(isoRoot, id), true, "A-F3: report survives");

        // Reunified flip: ONE anchored commit carries anchor + handle
        // together -> retirement proceeds.
        commitAll(
            scratch.git,
            scratch.dir,
            `docs: reunify ${id} decision carrier\n\nTask-Card: ${id}\n\n${HANDLE}\n`,
        );
        const done = retire(id);
        assert.equal(
            done.ok,
            true,
            `A-F3 flip: the reunified carrier must retire; got ${JSON.stringify(done.refusal || done)}`,
        );
        assert.equal(cardExists(isoRoot, id), false, "A-F3 flip: card gone");
        assert.equal(reportExists(isoRoot, id), false, "A-F3 flip: report gone");
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
