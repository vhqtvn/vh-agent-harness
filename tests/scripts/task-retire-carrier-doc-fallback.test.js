// Carrier-DOCUMENT fallback for the retirement gate's commit carriers
// (the 2026-11-14 Class A mismatch fix).
//
// Root cause (7 reproductions — the Class A force-deletions of 2026-11-14,
// see docs/checkpoints/2026-11-14-completed-task-retirement-carriers.md):
// decision-bearing closeouts declared commit carriers PINNED to the slice
// commits (revision set to the work commit). Those commits carry the
// card-id anchor via the Task-Card trailer but NEVER the decision-item
// label lines or verbatim handles — commit messages are authored by the
// committer protocol and do not carry per-item labels — so the retirement
// verifier predictably returned handle_not_in_carrier even though the
// decision content was durably preserved in committed carrier documents
// (the 476bfc8 checkpoint shape).
//
// Fix under test (Fix B, naming-convention constrained): when a COMMIT
// carrier (pinned or search mode) deterministically misses the anchor
// and/or the handle, the verifier ALSO searches committed, branch-reachable
// retirement-carrier DOCUMENTS — files under the durable homes
// (docs/checkpoints/, researches/decisions/, researches/sources/,
// docs/planning/) whose basename contains "retirement-carrier" — for the
// card-id anchor AND the item's handle together in one committed blob.
// Anything else ("any committed doc") stays a non-carrier: the naming
// convention IS the trust boundary.
//
// Pins:
//   - THE reproduction: pinned-commit curated slot (trailer anchor, no
//     labels anywhere in the commit) refuses handle_not_in_carrier before
//     a carrier doc exists; landing a convention-matching carrier doc
//     retires the card cleanly through the ordinary /task-delete flow.
//   - Naming-convention boundary: a committed doc with anchor + handle
//     that does NOT match *retirement-carrier* satisfies NOTHING.
//   - Same-blob rule inside the fallback: anchor-only doc / handle-only
//     doc / anchor-and-handle-scattered-across-revisions all refuse.
//   - Working-tree-only carrier docs never count.
//   - Search-mode commit carriers (revision omitted) get the same
//     fallback.
//   - Anchor-missing pins are rescued by a doc carrying both halves.
//   - Scope boundary: artifact carriers (declared path) do NOT get the
//     fallback — a wrong path declaration is a re-closeout matter.
//   - The refusal detail names the carrier-document search so operators
//     learn the sanctioned preservation route.
//   - Unavailability threading (b-F1/c-F1/d-F2, commit review 2026-11-15
//     of 5db2741): a candidate carrier doc whose search could not
//     COMPLETE (unreadable committed blob) is UNAVAILABILITY, never a
//     confident miss — the refusal still fires (fail-closed unchanged)
//     but its fallback note names the unexamined candidate and never
//     asserts universal absence over it.
//
// Test seam (NOT an env var): the retirement verifier's repo is redirected
// at a hermetic scratch git repo via the `landingProofRepo` OPTION (not
// forwarded by plan-state.js).
//
// Run:  vh-agent-harness exec node --test tests/scripts/task-retire-carrier-doc-fallback.test.js

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
    saveCoordinationTaskCloseout,
    deleteCoordinationTask,
    decisionItemId,
} = await import(pathToFileURL(STATE_LIB).href);

const noteId = (text) => decisionItemId("owner_notes", text);

// --- hermetic scratch git repo helpers (mirrors task-retire-decision-record) --

function makeScratchRepo() {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "dr-docfb-"));
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
    run(["config", "user.email", "dr-docfb@test"]);
    run(["config", "user.name", "dr-docfb-test"]);
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

function commitAll(git, dir, message) {
    const name = `f-${process.pid}-${Math.random().toString(36).slice(2)}.txt`;
    fs.writeFileSync(path.join(dir, name), `content ${name}\n`);
    git(["add", "-A"]);
    git(["commit", "-q", "-m", message]);
    return git(["rev-parse", "HEAD"]).stdout.trim();
}

function commitFile(git, dir, relPath, content, message) {
    const abs = path.join(dir, relPath);
    fs.mkdirSync(path.dirname(abs), { recursive: true });
    fs.writeFileSync(abs, content);
    git(["add", relPath]);
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
                title: `carrier-doc fixture ${id}`,
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

const SESSION_ID = "dr-docfb-session";
const ALIAS = "dr-docfb";

function writeWorkingCard(isoRoot, id, extra = {}) {
    writeCard(isoRoot, id, {
        status: "working",
        active_session_alias: ALIAS,
        claimed_at: "2026-11-14T00:00:00Z",
        report_envelope: "minimal",
        files_in_scope: ["tests/scripts/task-retire-carrier-doc-fallback.test.js"],
        success_criteria: ["carrier-doc fallback pins"],
        validation_plan: [
            "node --test tests/scripts/task-retire-carrier-doc-fallback.test.js",
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

function label(taskId, itemId, disposition) {
    return `decision-item: ${taskId}#${itemId} (${disposition})`;
}

test("carrier-doc fallback: Class A reproduction retires after a convention-matching carrier doc lands; boundary + honesty controls", () => {
    const isoRoot = fs.mkdtempSync(path.join(os.tmpdir(), "dr-docfb-iso-"));
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

        // =================================================================
        // THE REPRODUCTION (Class A shape, 7× on 2026-11-14): a curated
        // slot whose carrier is PINNED to the slice commit. The slice
        // commit carries the card-id anchor (Task-Card trailer) but no
        // decision-item labels — commit messages authored by the committer
        // protocol never carry them.
        // =================================================================
        const ID = "dr-class-a";
        const NOTE =
            "OWNER NOTE dr-class-a: approved the bounded search over named carrier documents (chat m0221)";
        const ITEM_ID = noteId(NOTE);
        const CURATED_LABEL = label(ID, ITEM_ID, "curated");

        writeWorkingCard(isoRoot, ID, { owner_notes: [NOTE] });
        // Slice commit: trailer anchor, NO label/handle anywhere.
        fs.writeFileSync(path.join(scratch.dir, "slice.txt"), "work\n");
        const sliceSha = commitAll(
            scratch.git,
            scratch.dir,
            `fix: the slice this card carries\n\nTask-Card: ${ID}\n`,
        );
        closeout(ID, [
            {
                id: ITEM_ID,
                disposition: "curated",
                carrier: { kind: "commit", revision: sliceSha },
                handle: CURATED_LABEL,
                provenance: "owner chat m0221, 2026-11-14 — curated note",
            },
        ]);

        // Phase 1 — before any carrier doc exists: the mismatch refusal
        // (the deterministic RED baseline: content preserved nowhere the
        // gate can see must refuse).
        const refusedBefore = retire(ID);
        assert.equal(refusedBefore.ok, false, "Class A: must refuse before preservation");
        assert.equal(refusedBefore.refusal.code, "decision_record_unresolved");
        const missBefore = (refusedBefore.refusal.problems || []).find(
            (p) => p.code === "handle_not_in_carrier",
        );
        assert.ok(missBefore, "Class A: refusal must name handle_not_in_carrier");
        // Honesty: the refusal names the carrier-doc search so operators
        // learn the sanctioned preservation route.
        assert.ok(
            (missBefore.detail || "").includes("retirement-carrier"),
            `Class A: refusal detail must name the retirement-carrier document search; got: ${missBefore.detail}`,
        );
        assert.equal(cardExists(isoRoot, ID), true, "Class A: card survives");

        // Phase 2 — the sanctioned preservation lands: a convention-matching
        // carrier document (the 476bfc8 shape: card-id anchor + canonical
        // label + curation beside it), committed and branch-reachable.
        const DOC_REL = `docs/checkpoints/2026-11-14-retirement-carrier-${ID}.md`;
        commitFile(
            scratch.git,
            scratch.dir,
            DOC_REL,
            `# Retirement carrier — ${ID}\n\nTask-Card: ${ID}\n\n${CURATED_LABEL}\napproved bounded search over named carrier documents; authority: owner chat m0221 (2026-11-14)\n`,
            `docs: land retirement carrier for ${ID}`,
        );
        // THE FIX UNDER TEST: the same pinned-carrier shape that refused in
        // Phase 1 now verifies through the carrier document.
        const done = retire(ID);
        assert.equal(
            done.ok,
            true,
            `Class A: retirement must succeed after the carrier doc lands; got ${JSON.stringify(done.refusal || done)}`,
        );
        assert.equal(cardExists(isoRoot, ID), false, "Class A: card retired");

        // =================================================================
        // Naming-convention boundary: anchor + label in a committed doc
        // that does NOT match *retirement-carrier* satisfies NOTHING —
        // "any committed doc" must not become a carrier.
        // =================================================================
        {
            const idB = "dr-docfb-plain-name";
            const noteB =
                "OWNER NOTE dr-docfb-plain-name: plain-named docs are not carriers";
            const itemB = noteId(noteB);
            const labelB = label(idB, itemB, "curated");
            writeWorkingCard(isoRoot, idB, { owner_notes: [noteB] });
            fs.writeFileSync(path.join(scratch.dir, "slice-b.txt"), "work\n");
            const shaB = commitAll(
                scratch.git,
                scratch.dir,
                `fix b\n\nTask-Card: ${idB}\n`,
            );
            closeout(idB, [
                {
                    id: itemB,
                    disposition: "curated",
                    carrier: { kind: "commit", revision: shaB },
                    handle: labelB,
                    provenance: "owner chat, 2026-11-14 — curated note",
                },
            ]);
            // Committed, branch-reachable, anchor + handle together — but
            // the basename lacks the convention token.
            commitFile(
                scratch.git,
                scratch.dir,
                "docs/checkpoints/2026-11-15-dr-docfb-plain-notes.md",
                `Task-Card: ${idB}\n\n${labelB}\nplain-named doc content\n`,
                `docs: plain checkpoint for ${idB}`,
            );
            const refusedB = retire(idB);
            assert.equal(
                refusedB.ok,
                false,
                "boundary: a non-matching doc basename must NOT satisfy the carrier",
            );
            assert.ok(
                (refusedB.refusal.problems || []).some(
                    (p) => p.code === "handle_not_in_carrier",
                ),
                "boundary: refusal must stay handle_not_in_carrier",
            );
            assert.equal(cardExists(isoRoot, idB), true, "boundary: card survives");
        }

        // =================================================================
        // Durable-home boundary: a convention-matching BASENAME outside
        // the four durable homes satisfies nothing.
        // =================================================================
        {
            const idC = "dr-docfb-outside-roots";
            const noteC =
                "OWNER NOTE dr-docfb-outside-roots: scratch docs are not carriers";
            const itemC = noteId(noteC);
            const labelC = label(idC, itemC, "curated");
            writeWorkingCard(isoRoot, idC, { owner_notes: [noteC] });
            fs.writeFileSync(path.join(scratch.dir, "slice-c.txt"), "work\n");
            const shaC = commitAll(
                scratch.git,
                scratch.dir,
                `fix c\n\nTask-Card: ${idC}\n`,
            );
            closeout(idC, [
                {
                    id: itemC,
                    disposition: "curated",
                    carrier: { kind: "commit", revision: shaC },
                    handle: labelC,
                    provenance: "owner chat, 2026-11-14 — curated note",
                },
            ]);
            commitFile(
                scratch.git,
                scratch.dir,
                "tmp/retirement-carrier-outside.md",
                `Task-Card: ${idC}\n\n${labelC}\noutside-the-durable-homes content\n`,
                `chore: tmp carrier-shaped doc for ${idC}`,
            );
            const refusedC = retire(idC);
            assert.equal(
                refusedC.ok,
                false,
                "roots: a convention-matching basename outside the durable homes must NOT satisfy the carrier",
            );
            assert.equal(cardExists(isoRoot, idC), true, "roots: card survives");
        }

        // =================================================================
        // Same-blob rule inside the fallback: anchor-only doc, handle-only
        // doc, and anchor+handle scattered across two revisions of the doc
        // each refuse (split evidence is never a pass).
        // =================================================================
        {
            const idD = "dr-docfb-samesblob";
            const noteD =
                "OWNER NOTE dr-docfb-samesblob: both halves must share one blob";
            const itemD = noteId(noteD);
            const labelD = label(idD, itemD, "curated");
            writeWorkingCard(isoRoot, idD, { owner_notes: [noteD] });
            fs.writeFileSync(path.join(scratch.dir, "slice-d.txt"), "work\n");
            const shaD = commitAll(
                scratch.git,
                scratch.dir,
                `fix d\n\nTask-Card: ${idD}\n`,
            );
            closeout(idD, [
                {
                    id: itemD,
                    disposition: "curated",
                    carrier: { kind: "commit", revision: shaD },
                    handle: labelD,
                    provenance: "owner chat, 2026-11-14 — curated note",
                },
            ]);
            // anchor-only doc
            commitFile(
                scratch.git,
                scratch.dir,
                `docs/checkpoints/2026-11-15-retirement-carrier-${idD}.md`,
                `Task-Card: ${idD}\n\n(no labels here yet)\n`,
                `docs: anchor-only carrier doc for ${idD}`,
            );
            const refusedD1 = retire(idD);
            assert.equal(
                refusedD1.ok,
                false,
                "same-blob: anchor-only carrier doc must not satisfy",
            );
            // handle added in the WORKING TREE only — never counts.
            fs.writeFileSync(
                path.join(scratch.dir, `docs/checkpoints/2026-11-15-retirement-carrier-${idD}.md`),
                `Task-Card: ${idD}\n\n${labelD}\nworking-tree-only label\n`,
            );
            const refusedD2 = retire(idD);
            assert.equal(
                refusedD2.ok,
                false,
                "same-blob: working-tree carrier-doc content must not satisfy",
            );
            assert.equal(cardExists(isoRoot, idD), true, "same-blob: card survives");
        }
        {
            // scatter: handle lands in one committed revision, anchor only
            // in a LATER revision — never together in one blob. Uses a
            // VERBATIM handle: the canonical curated label embeds the card
            // id, so a label-bearing revision always carries the anchor and
            // curated scatter inside one doc is unconstructible.
            const idE = "dr-docfb-scatter";
            const noteE =
                "OWNER NOTE: split carrier-doc evidence across revisions never verifies (scatter guard)";
            const itemE = noteId(noteE);
            writeWorkingCard(isoRoot, idE, { owner_notes: [noteE] });
            fs.writeFileSync(path.join(scratch.dir, "slice-e.txt"), "work\n");
            const shaE = commitAll(
                scratch.git,
                scratch.dir,
                `fix e\n\nTask-Card: ${idE}\n`,
            );
            closeout(idE, [
                {
                    id: itemE,
                    disposition: "verbatim",
                    carrier: { kind: "commit", revision: shaE },
                    handle: noteE,
                    provenance: "owner chat, 2026-11-14 — verbatim note",
                },
            ]);
            const docE = `docs/checkpoints/2026-11-15-retirement-carrier-${idE}.md`;
            commitFile(
                scratch.git,
                scratch.dir,
                docE,
                `${noteE}\nhandle-only first revision (no anchor)\n`,
                `docs: handle-only carrier doc revision for ${idE}`,
            );
            commitFile(
                scratch.git,
                scratch.dir,
                docE,
                `Task-Card: ${idE}\n\nanchor-only second revision (handle removed)\n`,
                `docs: anchor-only carrier doc revision for ${idE}`,
            );
            const refusedE = retire(idE);
            assert.equal(
                refusedE.ok,
                false,
                "scatter: anchor and handle in different carrier-doc revisions must not satisfy",
            );
            assert.equal(cardExists(isoRoot, idE), true, "scatter: card survives");
        }

        // =================================================================
        // Search-mode commit carrier (revision omitted) gets the same
        // fallback; anchor-missing pins are rescued by a doc carrying both
        // halves (the fallback proves preservation independently).
        // =================================================================
        {
            const idF = "dr-docfb-searchmode";
            const noteF =
                "OWNER NOTE dr-docfb-searchmode: search-mode carriers fall back too";
            const itemF = noteId(noteF);
            const labelF = label(idF, itemF, "curated");
            writeWorkingCard(isoRoot, idF, { owner_notes: [noteF] });
            commitAll(
                scratch.git,
                scratch.dir,
                `fix f\n\nTask-Card: ${idF}\n`,
            );
            closeout(idF, [
                {
                    id: itemF,
                    disposition: "curated",
                    carrier: { kind: "commit" },
                    handle: labelF,
                    provenance: "owner chat, 2026-11-14 — curated note",
                },
            ]);
            const refusedF = retire(idF);
            assert.equal(
                refusedF.ok,
                false,
                "search-mode: must refuse before the carrier doc exists",
            );
            commitFile(
                scratch.git,
                scratch.dir,
                `researches/decisions/2026-11-15-retirement-carrier-${idF}.md`,
                `Task-Card: ${idF}\n\n${labelF}\nresearches home is a durable carrier root\n`,
                `docs: researches carrier for ${idF}`,
            );
            const doneF = retire(idF);
            assert.equal(
                doneF.ok,
                true,
                `search-mode: carrier doc in a second durable home must verify; got ${JSON.stringify(doneF.refusal || doneF)}`,
            );
            assert.equal(cardExists(isoRoot, idF), false, "search-mode: card retired");
        }
        {
            // anchor-missing pin + doc carrying BOTH halves -> rescued.
            const idG = "dr-docfb-anchor-missing-pin";
            const noteG =
                "OWNER NOTE dr-docfb-anchor-missing-pin: the pin lacks even the anchor";
            const itemG = noteId(noteG);
            const labelG = label(idG, itemG, "curated");
            writeWorkingCard(isoRoot, idG, { owner_notes: [noteG] });
            fs.writeFileSync(path.join(scratch.dir, "slice-g.txt"), "work\n");
            const shaG = commitAll(
                scratch.git,
                scratch.dir,
                `fix g — a commit that mentions neither the card id nor labels\n`,
            );
            closeout(idG, [
                {
                    id: itemG,
                    disposition: "curated",
                    carrier: { kind: "commit", revision: shaG },
                    handle: labelG,
                    provenance: "owner chat, 2026-11-14 — curated note",
                },
            ]);
            // Landing proof still resolves via a different trailer commit.
            commitAll(
                scratch.git,
                scratch.dir,
                `docs: trailer landing for ${idG}\n\nTask-Card: ${idG}\n`,
            );
            commitFile(
                scratch.git,
                scratch.dir,
                `docs/planning/2026-11-15-retirement-carrier-${idG}.md`,
                `Task-Card: ${idG}\n\n${labelG}\nplanning home is a durable carrier root\n`,
                `docs: planning carrier for ${idG}`,
            );
            const doneG = retire(idG);
            assert.equal(
                doneG.ok,
                true,
                `anchor-missing pin: doc carrying both halves must rescue; got ${JSON.stringify(doneG.refusal || doneG)}`,
            );
            assert.equal(cardExists(isoRoot, idG), false, "anchor-missing pin: retired");
        }

        // =================================================================
        // Positive VERBATIM rescue: the fallback serves verbatim handles
        // too (a verbatim line + anchor together in the carrier doc), not
        // just canonical labels.
        // =================================================================
        {
            const idI = "dr-docfb-verbatim";
            const noteI =
                "OWNER APPROVAL dr-docfb-verbatim: verbatim notes ride the same fallback (chat m0221)";
            const itemI = noteId(noteI);
            writeWorkingCard(isoRoot, idI, { owner_notes: [noteI] });
            fs.writeFileSync(path.join(scratch.dir, "slice-i.txt"), "work\n");
            const shaI = commitAll(
                scratch.git,
                scratch.dir,
                `fix i\n\nTask-Card: ${idI}\n`,
            );
            closeout(idI, [
                {
                    id: itemI,
                    disposition: "verbatim",
                    carrier: { kind: "commit", revision: shaI },
                    handle: noteI,
                    provenance: "owner chat m0221, 2026-11-14 — verbatim note",
                },
            ]);
            commitFile(
                scratch.git,
                scratch.dir,
                `docs/checkpoints/2026-11-15-retirement-carrier-${idI}.md`,
                `Task-Card: ${idI}\n\n${noteI}\nverbatim note preserved in the carrier document\n`,
                `docs: carrier for ${idI}`,
            );
            const doneI = retire(idI);
            assert.equal(
                doneI.ok,
                true,
                `verbatim: carrier doc with the verbatim line must verify; got ${JSON.stringify(doneI.refusal || doneI)}`,
            );
            assert.equal(cardExists(isoRoot, idI), false, "verbatim: card retired");
        }

        // =================================================================
        // Scope boundary: ARTIFACT carriers (declared path) do NOT get the
        // fallback — a wrong-path declaration is a re-closeout matter, and
        // the fallback must not mask it.
        // =================================================================
        {
            const idH = "dr-docfb-artifact-scope";
            const noteH =
                "OWNER NOTE dr-docfb-artifact-scope: artifact carriers keep their own search";
            const itemH = noteId(noteH);
            const labelH = label(idH, itemH, "curated");
            writeWorkingCard(isoRoot, idH, { owner_notes: [noteH] });
            commitAll(
                scratch.git,
                scratch.dir,
                `fix h\n\nTask-Card: ${idH}\n`,
            );
            closeout(idH, [
                {
                    id: itemH,
                    disposition: "curated",
                    carrier: {
                        kind: "artifact",
                        path: "docs/checkpoints/declared-but-never-committed-h.md",
                    },
                    handle: labelH,
                    provenance: "owner chat, 2026-11-14 — curated note",
                },
            ]);
            // A convention-matching carrier doc DOES carry anchor + label…
            commitFile(
                scratch.git,
                scratch.dir,
                `docs/checkpoints/2026-11-15-retirement-carrier-${idH}.md`,
                `Task-Card: ${idH}\n\n${labelH}\ncarrier-doc content\n`,
                `docs: carrier for ${idH}`,
            );
            // …but the declared artifact path was never committed, and the
            // artifact branch must own its own refusal (path_never_committed).
            const refusedH = retire(idH);
            assert.equal(
                refusedH.ok,
                false,
                "artifact scope: declared artifact path keeps its own verification",
            );
            assert.ok(
                (refusedH.refusal.problems || []).some(
                    (p) => p.code === "path_never_committed",
                ),
                "artifact scope: refusal must name path_never_committed",
            );
            assert.equal(cardExists(isoRoot, idH), true, "artifact scope: card survives");
        }
    } finally {
        if (prevCoordRoot === undefined) {
            delete process.env.OPENCODE_LOCAL_COORDINATOR_ROOT;
        } else {
            process.env.OPENCODE_LOCAL_COORDINATOR_ROOT = prevCoordRoot;
        }
        fs.rmSync(isoRoot, { recursive: true, force: true });
        fs.rmSync(scratch.dir, { recursive: true, force: true });
    }
});

// Unavailability threading (b-F1/c-F1/d-F2, commit review 2026-11-15 of
// 5db2741): the fallback's candidate walk used to collapse a candidate
// carrier-doc search that could not COMPLETE (unreadable committed blob /
// history read error) into a confident miss, so the appended note asserted
// universal absence over a document that was never actually examined. The
// gate outcome (fail-closed refusal) was and stays CORRECT — the defect is
// diagnostic honesty only.
//
// Fixture (probe-verified 2026-11-15): removing the carrier doc's loose
// BLOB object leaves every name-level read intact — discovery
// (`git log --name-only -- <roots>/*retirement-carrier*`), the per-path
// revision walk (`git log --format=%H -- <path>`), and the ls-tree
// classification probe all exit 0 — while ONLY `git show <rev>:<path>`
// fails (exit 128, "bad object"), i.e. exactly the per-candidate
// verifier_unavailable shape inside searchCommittedPathForAnchorAndHandle.
test("carrier-doc fallback: an unexamined candidate carrier doc is UNAVAILABILITY, never a confident miss", () => {
    const isoRoot = fs.mkdtempSync(path.join(os.tmpdir(), "dr-docfb-unav-"));
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

        // Class A shape (pinned commit carrier): the slice commit carries
        // the anchor but never the label, so the fallback is the only
        // possible verification — and its ONLY candidate is a carrier doc
        // whose committed blob is made UNREADABLE after the fact.
        const ID = "dr-docfb-unreadable-candidate";
        const NOTE =
            "OWNER NOTE dr-docfb-unreadable-candidate: unreadable candidate docs are unavailability, never confident absence";
        const ITEM_ID = noteId(NOTE);
        const CURATED_LABEL = label(ID, ITEM_ID, "curated");
        writeWorkingCard(isoRoot, ID, { owner_notes: [NOTE] });
        fs.writeFileSync(path.join(scratch.dir, "slice-u.txt"), "work\n");
        const sliceSha = commitAll(
            scratch.git,
            scratch.dir,
            `fix u\n\nTask-Card: ${ID}\n`,
        );
        closeout(ID, [
            {
                id: ITEM_ID,
                disposition: "curated",
                carrier: { kind: "commit", revision: sliceSha },
                handle: CURATED_LABEL,
                provenance: "owner chat, 2026-11-15 — curated note",
            },
        ]);
        // The candidate WOULD verify (anchor + label together in one
        // committed blob); committing it exactly once keeps the candidate
        // list at exactly this doc, so the walk's outcome hinges entirely
        // on whether its search could complete.
        const DOC_REL = `docs/checkpoints/2026-11-15-retirement-carrier-${ID}.md`;
        const DOC_CONTENT = `Task-Card: ${ID}\n\n${CURATED_LABEL}\ncarrier content whose blob becomes unreadable\n`;
        const docSha = commitFile(
            scratch.git,
            scratch.dir,
            DOC_REL,
            DOC_CONTENT,
            `docs: carrier for ${ID}`,
        );
        // Corrupt: remove the committed blob's loose object. rev-parse
        // resolves <rev>:<path> through the TREE (blob not read), so the
        // oid is derivable while the blob is unreadable.
        const blobOid = scratch.git([
            "rev-parse",
            `${docSha}:${DOC_REL}`,
        ]).stdout.trim();
        const objPath = path.join(
            scratch.dir,
            ".git",
            "objects",
            blobOid.slice(0, 2),
            blobOid.slice(2),
        );
        assert.ok(
            fs.existsSync(objPath),
            "fixture: the carrier doc's loose blob object must exist before corruption",
        );
        fs.rmSync(objPath);

        const refused = retire(ID);
        // Fail-closed is UNCHANGED: no carrier verifiably preserves the
        // item, so the refusal still fires and the card survives.
        assert.equal(
            refused.ok,
            false,
            "unavailable candidate: must still refuse (fail-closed unchanged)",
        );
        assert.equal(refused.refusal.code, "decision_record_unresolved");
        const miss = (refused.refusal.problems || []).find(
            (p) => p.code === "handle_not_in_carrier",
        );
        assert.ok(
            miss,
            "unavailable candidate: refusal must still name handle_not_in_carrier for the declared carrier",
        );
        // Honesty (the fix): the fallback note still teaches the route…
        assert.ok(
            (miss.detail || "").includes("retirement-carrier"),
            `unavailable candidate: the note must still teach the carrier-doc route; got: ${miss.detail}`,
        );
        // …names the candidate it could not examine…
        assert.ok(
            (miss.detail || "").includes(DOC_REL),
            `unavailable candidate: the note must name the unexamined candidate; got: ${miss.detail}`,
        );
        // …reflects UNAVAILABILITY and explicitly disclaims absence…
        assert.ok(
            (miss.detail || "").includes("could not") &&
                (miss.detail || "").includes("absence is NOT established"),
            `unavailable candidate: the note must reflect unavailability and disclaim absence; got: ${miss.detail}`,
        );
        // …and never collapses the incomplete walk into a confident miss
        // (the pre-fix defect: universal-absence phrasing).
        assert.ok(
            !(miss.detail || "").includes("also found nothing"),
            `unavailable candidate: the note must not assert a completed search that found nothing; got: ${miss.detail}`,
        );
        assert.ok(
            !(miss.detail || "").includes("no committed, branch-reachable file"),
            `unavailable candidate: the note must not assert universal absence over an unexamined candidate; got: ${miss.detail}`,
        );
        assert.equal(
            cardExists(isoRoot, ID),
            true,
            "unavailable candidate: card survives the fail-closed refusal",
        );

        // Recovery: re-write the exact blob object (identical content →
        // identical oid), the SAME candidate search now completes and
        // verifies, and retirement succeeds — the fixture's only defect
        // was readability, and the fallback's verify path is untouched.
        const restoreFile = path.join(scratch.dir, "restore-carrier-blob.txt");
        fs.writeFileSync(restoreFile, DOC_CONTENT);
        const restoredOid = scratch.git([
            "hash-object",
            "-w",
            restoreFile,
        ]).stdout.trim();
        fs.rmSync(restoreFile);
        assert.equal(
            restoredOid,
            blobOid,
            "fixture: the restored blob must hash to the same oid",
        );
        const done = retire(ID);
        assert.equal(
            done.ok,
            true,
            `unavailable candidate: retirement must succeed once the blob is readable again; got ${JSON.stringify(done.refusal || done)}`,
        );
        assert.equal(
            cardExists(isoRoot, ID),
            false,
            "unavailable candidate: card retires after recovery",
        );
    } finally {
        if (prevCoordRoot === undefined) {
            delete process.env.OPENCODE_LOCAL_COORDINATOR_ROOT;
        } else {
            process.env.OPENCODE_LOCAL_COORDINATOR_ROOT = prevCoordRoot;
        }
        fs.rmSync(isoRoot, { recursive: true, force: true });
        fs.rmSync(scratch.dir, { recursive: true, force: true });
    }
});
