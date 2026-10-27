// Decision-record closeout gate tests (retention preservation,
// owner-approved 2026-10-25; v2 coverage 2026-10-26; v3 item-enumerated
// replacement owner-approved 2026-10-27; content-bound item identity from
// the 2026-10-27 B-F1 fix round).
//
// Pins the CLOSEOUT half of the v3 retention-preservation gate in
// saveCoordinationTaskCloseout: every new closeout must declare exactly ONE
// fenced ```decision-record block (version 3, item-enumerated — one slot
// per decision item; an EMPTY items array is the routine attestation);
// structural validation per slot matching the task-card schema's v3
// variants EXACTLY (unknown keys anywhere, wrong version, duplicate ids,
// bad dispositions, non-canonical curated labels, missing provenance on
// owner-decision/sanitized slots — all refused); the CONTENT-BOUND item
// identity (ids derive from each item's own text via the exported
// decisionItemId factory — owner_notes[fnv1a:<hash>.<n>], never array
// position); the INVENTORY
// COMPLETENESS rule (concern-1's primary fix: two owner notes cannot be
// covered by one slot — every decision item must have its own declared
// slot, no surplus slots, and verbatim handles must bind to their OWN
// item's content); the C2 review classification (a clean-approve review is
// a PROCESS record — routine closeout succeeds; a non-approve verdict is
// decision-class — routine refused naming last_review + the task_reviewed
// event item); the ADD-ONLY amendment rule (a re-closeout may not drop or rewrite a
// stored slot); the parsed record persisting on the card for the
// retirement verifier; and schema conformance of records produced by
// successful closeouts (rendered task-card.schema.json). Dash-prefixed
// ids/revisions/handles are refused at declaration naming the
// option-injection rule (F1); the retirement-side half of that pin lives
// in tests/scripts/task-retire-decision-record.test.js.
//
// The RETIREMENT half (git resolution of declared carriers, the inventory
// baseline, legacy compatibility, force warning) is pinned by
// tests/scripts/task-retire-decision-record.test.js.
//
// Run:  vh-agent-harness exec node --test tests/scripts/task-closeout-decision-record.test.js
//       (or: node --test tests/scripts/task-closeout-decision-record.test.js)

import { test } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import os from "node:os";
import { fileURLToPath, pathToFileURL } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = path.resolve(__dirname, "..", "..");

// Import state-lib from the RENDERED tree (.opencode/scripts/). The source
// copy under templates/core/ still carries the literal {{COORDINATOR_DIR}}
// token and REFUSES to load. Run `make update` after editing templates/core/
// before this test (the normal dev flow).
const STATE_LIB = path.join(REPO_ROOT, ".opencode", "scripts", "state-lib.js");
const {
    bindSessionName,
    saveCoordinationTaskCloseout,
    collectDecisionItems,
    decisionItemId,
} = await import(pathToFileURL(STATE_LIB).href);

// Content-bound item ids (the B-F1 fix): ids derive from the item's own
// text via the gate's exported factory, so these fixtures can never drift
// from the identity grammar.
const noteId = (text, occurrence = 1) =>
    decisionItemId("owner_notes", text, occurrence);
const questionId = (text, occurrence = 1) =>
    decisionItemId("open_questions", text, occurrence);
const measuredId = (text) => decisionItemId("measured_outcome", text);

// --- schema-driven conformance checker ----------------------------------------
// Loads the RENDERED task-card schema (kept in sync with templates/core by
// `make update`) and validates a decision_record against the schema's
// decision_record subschema using exactly the draft-07 subset that subschema
// uses (type, required, properties, additionalProperties:false, const, enum,
// minLength, minItems, anyOf). Pins ONE contract: records the closeout gate
// ACCEPTS must be records the SCHEMA accepts, and the schema checker itself
// rejects non-vacuous negatives (marker-on-legacy-variant, unknown-key,
// wrong-version, bad-disposition records).
const SCHEMA_PATH = path.join(
    REPO_ROOT,
    "docs",
    "coordination",
    "schemas",
    "task-card.schema.json",
);
const DR_SUBSCHEMA =
    JSON.parse(fs.readFileSync(SCHEMA_PATH, "utf8")).properties
        .decision_record;

function schemaErrors(node, schema, at) {
    const errs = [];
    if (schema.anyOf) {
        const matchesSome = schema.anyOf.some(
            (variant) =>
                schemaErrors(node, variant, `${at} (anyOf)`).length === 0,
        );
        if (!matchesSome) {
            errs.push(`${at}: matches no anyOf variant`);
        }
        // With anyOf present the variants fully govern the node; do not
        // double-apply sibling keywords the variants already carry.
        return errs;
    }
    if (schema.const !== undefined) {
        if (node !== schema.const) {
            errs.push(
                `${at}: expected const ${JSON.stringify(schema.const)}`,
            );
        }
        return errs;
    }
    if (schema.enum !== undefined) {
        if (!schema.enum.includes(node)) {
            errs.push(
                `${at}: ${JSON.stringify(node)} not in enum ${JSON.stringify(schema.enum)}`,
            );
        }
        return errs;
    }
    const types = Array.isArray(schema.type)
        ? schema.type
        : schema.type
          ? [schema.type]
          : [];
    if (types.length > 0) {
        const actual =
            node === null
                ? "null"
                : Array.isArray(node)
                  ? "array"
                  : typeof node;
        if (!types.includes(actual)) {
            errs.push(`${at}: type ${actual} not in ${types.join("|")}`);
            return errs;
        }
    }
    if (types.includes("object")) {
        for (const key of schema.required || []) {
            if (!(key in node)) {
                errs.push(`${at}: missing required "${key}"`);
            }
        }
        for (const [key, sub] of Object.entries(schema.properties || {})) {
            if (key in node) {
                errs.push(
                    ...schemaErrors(node[key], sub, `${at}.${key}`),
                );
            }
        }
        if (schema.additionalProperties === false) {
            for (const key of Object.keys(node)) {
                if (!(key in (schema.properties || {}))) {
                    errs.push(`${at}: additionalProperty "${key}"`);
                }
            }
        }
    }
    if (types.includes("array")) {
        if (schema.minItems !== undefined && node.length < schema.minItems) {
            errs.push(`${at}: fewer than ${schema.minItems} item(s)`);
        }
        if (schema.items) {
            node.forEach((item, i) => {
                errs.push(
                    ...schemaErrors(item, schema.items, `${at}[${i}]`),
                );
            });
        }
    }
    if (
        types.includes("string") &&
        schema.minLength !== undefined &&
        node.length < schema.minLength
    ) {
        errs.push(`${at}: shorter than minLength ${schema.minLength}`);
    }
    return errs;
}

// --- helpers -----------------------------------------------------------------

function decisionRecordBlock(record) {
    return (
        "```decision-record\n" + JSON.stringify(record, null, 2) + "\n```\n"
    );
}

function routineRecord(note) {
    return {
        version: 3,
        items: [],
        note: note || "nothing decision-class",
    };
}

function slot(id, disposition, carrier, handle, extra = {}) {
    return { id, disposition, carrier, handle, ...extra };
}

function cardPath(isoRoot, id) {
    return path.join(isoRoot, "tasks", `${id}.json`);
}

// A complete valid `working` card written directly (every strict core
// field) so the closeout write-back passes ensureCoordinationTaskCoreFields.
// Extra fields (owner_notes, ...) via the `extra` argument.
function writeWorkingCard(isoRoot, id, alias, extra = {}) {
    const card = {
        schema_version: 1,
        task_id: id,
        title: `decision-record fixture ${id}`,
        task_type: "implementation",
        coordination_mode: "short",
        primary_lane: "harness-core",
        report_envelope: "minimal",
        status: "working",
        active_session_alias: alias,
        claimed_at: "2026-10-25T00:00:00Z",
        files_in_scope: ["tests/scripts/task-closeout-decision-record.test.js"],
        success_criteria: ["decision-record closeout gate pins."],
        validation_plan: [
            "node --test tests/scripts/task-closeout-decision-record.test.js",
        ],
        session_aliases: [alias],
        ...extra,
    };
    fs.mkdirSync(path.dirname(cardPath(isoRoot, id)), { recursive: true });
    fs.writeFileSync(cardPath(isoRoot, id), JSON.stringify(card, null, 2));
}

const SESSION_ID = "dr-closeout-session";
const ALIAS = "dr-closeout";

function runCloseout(id, taskStatus, body, options = {}) {
    return saveCoordinationTaskCloseout(SESSION_ID, id, {
        cwd: "/verification",
        title: "decision-record closeout",
        body,
        taskStatus,
        reportEnvelope: "minimal",
        ...options,
    });
}

function expectCloseoutRefusal(id, taskStatus, body, ...needles) {
    let thrown = null;
    try {
        runCloseout(id, taskStatus, body);
    } catch (error) {
        thrown = error;
    }
    assert.ok(thrown, `closeout ${id} should have refused but did not throw`);
    assert.equal(
        thrown && thrown.name,
        "StateError",
        `expected StateError for ${id}`,
    );
    const msg = String((thrown && thrown.message) || "");
    for (const needle of needles) {
        assert.ok(
            msg.includes(needle),
            `refusal for ${id} should include ${JSON.stringify(needle)}; got: ${msg}`,
        );
    }
    return msg;
}

// --- the suite ---------------------------------------------------------------

test("decision-record closeout gate: block required, structure enforced, inventory enumerated", () => {
    const isoRoot = fs.mkdtempSync(path.join(os.tmpdir(), "dr-closeout-"));
    const prevCoordRoot = process.env.OPENCODE_LOCAL_COORDINATOR_ROOT;
    process.env.OPENCODE_LOCAL_COORDINATOR_ROOT = isoRoot;
    bindSessionName(SESSION_ID, ALIAS);

    try {
        // (1) No decision-record block -> REFUSED (the gate binds EVERY new
        //     closeout, not just completed ones; a reported closeout is
        //     pinned too). There is deliberately no inline fallback.
        writeWorkingCard(isoRoot, "dr-no-block", ALIAS);
        expectCloseoutRefusal(
            "dr-no-block",
            "reported",
            "# Closeout\n\nprose only, no fenced block\n",
            "requires a decision_record block",
            "one slot per decision item",
        );
        assert.equal(
            JSON.parse(fs.readFileSync(cardPath(isoRoot, "dr-no-block"), "utf8"))
                .status,
            "working",
            "(1) card must remain working after refusal",
        );

        // (2) TWO blocks -> REFUSED (exactly one; merge the items).
        writeWorkingCard(isoRoot, "dr-two-blocks", ALIAS);
        expectCloseoutRefusal(
            "dr-two-blocks",
            "reported",
            "# c\n\n" +
                decisionRecordBlock(routineRecord("one")) +
                "\n" +
                decisionRecordBlock(routineRecord("two")),
            "exactly ONE",
        );

        // (3) Malformed JSON fence -> REFUSED with the parse error.
        writeWorkingCard(isoRoot, "dr-malformed", ALIAS);
        expectCloseoutRefusal(
            "dr-malformed",
            "reported",
            "# c\n\n```decision-record\n{ not json\n```\n",
            "could not be parsed",
        );

        // (4) JSON bodies that PARSE but are null / primitives / arrays ->
        //     collected StateError, never a raw TypeError.
        for (const [body, label] of [
            ["null", "null body"],
            ["42", "number body"],
            ['"text"', "string body"],
            ["true", "boolean body"],
            ["[]", "array body"],
        ]) {
            writeWorkingCard(isoRoot, `dr-json-${label.split(" ")[0]}`, ALIAS);
            expectCloseoutRefusal(
                `dr-json-${label.split(" ")[0]}`,
                "reported",
                "# c\n\n```decision-record\n" + body + "\n```\n",
                "must be a JSON object",
            );
        }

        // (5) Wrong version — including the retired v1/v2 shape — is
        //     refused: NEW closeouts declare version 3 only.
        writeWorkingCard(isoRoot, "dr-v1-shape", ALIAS);
        expectCloseoutRefusal(
            "dr-v1-shape",
            "reported",
            decisionRecordBlock({
                version: 1,
                entries: [{ kind: "routine", note: "legacy shape" }],
            }),
            "unknown key \"entries\"",
            "version must be 3",
            "re-declare under v3",
        );

        // (6) Record-level junk key -> refused by name.
        writeWorkingCard(isoRoot, "dr-record-junk", ALIAS);
        expectCloseoutRefusal(
            "dr-record-junk",
            "reported",
            decisionRecordBlock({
                version: 3,
                junk: true,
                items: [],
            }),
            'unknown key "junk"',
        );

        // (7) Slot structure: unknown slot key; unknown carrier kind
        //     (including the deliberately-absent "inline"); artifact
        //     carrier without path; carrier unknown key with the v3
        //     no-marker hint; duplicate ids; id dash / "#" rules;
        //     disposition enum. Slot ids use the CONTENT-BOUND factory so
        //     the fixtures track the gate's identity grammar exactly.
        writeWorkingCard(isoRoot, "dr-slot-junk", ALIAS);
        expectCloseoutRefusal(
            "dr-slot-junk",
            "reported",
            decisionRecordBlock({
                version: 3,
                items: [
                    slot(
                        noteId("OWNER APPROVAL 2026-10-27: approved the slot shape"),
                        "verbatim",
                        { kind: "commit" },
                        "OWNER APPROVAL 2026-10-27: approved the slot shape",
                        { bogus: 1 },
                    ),
                ],
            }),
            "unknown key(s)",
            '"bogus"',
        );
        writeWorkingCard(isoRoot, "dr-inline-carrier", ALIAS, {
            owner_notes: ["x".repeat(40)],
        });
        expectCloseoutRefusal(
            "dr-inline-carrier",
            "reported",
            decisionRecordBlock({
                version: 3,
                items: [slot(noteId("x".repeat(40)), "verbatim", { kind: "inline" }, "x")],
            }),
            'carrier.kind must be one of commit | artifact',
        );
        writeWorkingCard(isoRoot, "dr-artifact-nopath", ALIAS, {
            owner_notes: ["x".repeat(40)],
        });
        expectCloseoutRefusal(
            "dr-artifact-nopath",
            "reported",
            decisionRecordBlock({
                version: 3,
                items: [
                    slot(noteId("x".repeat(40)), "curated", { kind: "artifact" }, "x"),
                ],
            }),
            "artifact carriers require path",
        );
        writeWorkingCard(isoRoot, "dr-carrier-marker", ALIAS, {
            owner_notes: ["x".repeat(40)],
        });
        expectCloseoutRefusal(
            "dr-carrier-marker",
            "reported",
            decisionRecordBlock({
                version: 3,
                items: [
                    slot(
                        noteId("x".repeat(40)),
                        "curated",
                        { kind: "commit", marker: "m" },
                        `decision-item: dr-carrier-marker#${noteId("x".repeat(40))} (curated)`,
                    ),
                ],
            }),
            "carrier unknown key(s)",
            '"marker"',
            "v3 has no marker",
        );
        writeWorkingCard(isoRoot, "dr-dup-ids", ALIAS, {
            owner_notes: ["OWNER APPROVAL 2026-10-27: approved the dup refusal"],
        });
        expectCloseoutRefusal(
            "dr-dup-ids",
            "reported",
            decisionRecordBlock({
                version: 3,
                items: [
                    slot(
                        noteId("OWNER APPROVAL 2026-10-27: approved the dup refusal"),
                        "verbatim",
                        { kind: "commit" },
                        "OWNER APPROVAL 2026-10-27: approved the dup refusal",
                    ),
                    slot(
                        noteId("OWNER APPROVAL 2026-10-27: approved the dup refusal"),
                        "curated",
                        { kind: "commit" },
                        `decision-item: dr-dup-ids#${noteId("OWNER APPROVAL 2026-10-27: approved the dup refusal")} (curated)`,
                    ),
                ],
            }),
            "duplicate id",
        );
        writeWorkingCard(isoRoot, "dr-id-dash-hash", ALIAS, {
            owner_notes: ["OWNER APPROVAL 2026-10-27: approved the id rules"],
        });
        expectCloseoutRefusal(
            "dr-id-dash-hash",
            "reported",
            decisionRecordBlock({
                version: 3,
                items: [
                    slot(
                        "-owner-notes",
                        "verbatim",
                        { kind: "commit" },
                        "OWNER APPROVAL 2026-10-27: approved the id rules",
                    ),
                ],
            }),
            'must not start with "-"',
            "id",
        );
        expectCloseoutRefusal(
            "dr-id-dash-hash",
            "reported",
            decisionRecordBlock({
                version: 3,
                items: [
                    slot(
                        "owner#notes",
                        "verbatim",
                        { kind: "commit" },
                        "OWNER APPROVAL 2026-10-27: approved the id rules",
                    ),
                ],
            }),
            'must not contain "#"',
        );
        writeWorkingCard(isoRoot, "dr-bad-disposition", ALIAS, {
            owner_notes: ["OWNER APPROVAL 2026-10-27: approved the disposition enum"],
        });
        expectCloseoutRefusal(
            "dr-bad-disposition",
            "reported",
            decisionRecordBlock({
                version: 3,
                items: [
                    slot(
                        noteId("OWNER APPROVAL 2026-10-27: approved the disposition enum"),
                        "excerpt",
                        { kind: "commit" },
                        "OWNER APPROVAL 2026-10-27: approved the disposition enum",
                    ),
                ],
            }),
            "disposition must be one of verbatim | curated | sanitized",
        );

        // =====================================================================
        // (8) Verbatim handle guards: too short, multi-line, leading dash,
        //     equals the card id.
        // =====================================================================
        const NOTE = "OWNER APPROVAL 2026-10-27: approved the handle guards";
        const NOTE_ID = noteId(NOTE);
        writeWorkingCard(isoRoot, "dr-handle-guards", ALIAS, {
            owner_notes: [NOTE],
        });
        expectCloseoutRefusal(
            "dr-handle-guards",
            "completed",
            decisionRecordBlock({
                version: 3,
                items: [
                    slot(NOTE_ID, "verbatim", { kind: "commit" }, "too short"),
                ],
            }),
            "at least 24 characters",
        );
        expectCloseoutRefusal(
            "dr-handle-guards",
            "completed",
            decisionRecordBlock({
                version: 3,
                items: [
                    slot(NOTE_ID, "verbatim", { kind: "commit" }, NOTE + "\nsecond line"),
                ],
            }),
            "must be a single line",
        );
        expectCloseoutRefusal(
            "dr-handle-guards",
            "completed",
            decisionRecordBlock({
                version: 3,
                items: [
                    slot(NOTE_ID, "verbatim", { kind: "commit" }, "-v " + NOTE),
                ],
            }),
            'must not start with "-"',
        );
        // Card id >= 24 chars so the card-id rule fires before the length
        // floor (a short id would trip the floor first).
        writeWorkingCard(isoRoot, "dr-handle-guards-cardid-eq", ALIAS, {
            owner_notes: [NOTE],
        });
        expectCloseoutRefusal(
            "dr-handle-guards-cardid-eq",
            "completed",
            decisionRecordBlock({
                version: 3,
                items: [
                    slot(
                        NOTE_ID,
                        "verbatim",
                        { kind: "commit" },
                        "dr-handle-guards-cardid-eq",
                    ),
                ],
            }),
            "must not equal the card id",
        );

        // (9) Curated/sanitized handle must be EXACTLY the canonical label.
        expectCloseoutRefusal(
            "dr-handle-guards",
            "completed",
            decisionRecordBlock({
                version: 3,
                items: [
                    slot(
                        NOTE_ID,
                        "curated",
                        { kind: "commit" },
                        `decision-item: dr-handle-guards#${NOTE_ID} (verbatim)`,
                    ),
                    // (wrong disposition inside the label, not (curated))
                ],
            }),
            "must be EXACTLY the canonical item label",
            `"decision-item: dr-handle-guards#${NOTE_ID} (curated)"`,
        );

        // (10) Provenance required for owner-decision items and sanitized
        //      slots; a non-empty cited string satisfies it structurally.
        expectCloseoutRefusal(
            "dr-handle-guards",
            "completed",
            decisionRecordBlock({
                version: 3,
                items: [
                    slot(
                        NOTE_ID,
                        "curated",
                        { kind: "commit" },
                        `decision-item: dr-handle-guards#${NOTE_ID} (curated)`,
                    ),
                ],
            }),
            "provenance is required for owner-decision items",
        );
        writeWorkingCard(isoRoot, "dr-sanitized-noprov", ALIAS, {
            open_questions: ["Does the sanitized slot need provenance?"],
        });
        expectCloseoutRefusal(
            "dr-sanitized-noprov",
            "completed",
            decisionRecordBlock({
                version: 3,
                items: [
                    slot(
                        questionId("Does the sanitized slot need provenance?"),
                        "sanitized",
                        { kind: "commit" },
                        `decision-item: dr-sanitized-noprov#${questionId("Does the sanitized slot need provenance?")} (sanitized)`,
                    ),
                ],
            }),
            "provenance is required",
            "sanitized items",
        );

        // =====================================================================
        // (11) INVENTORY COMPLETENESS — the primary concern-1 fix. A card
        //      with TWO owner notes and a record declaring ONE slot: under
        //      v2 this shape retired with Note B unrecoverable (reproduced
        //      2026-10-26, tmp/repro/repro-concern1-receipts.json). v3
        //      refuses at closeout naming the unslotted item.
        // =====================================================================
        {
            const NOTE_1 =
                "ARCH APPROVED concern1: bind preservation per decision item so independent choices survive retirement";
            const NOTE_2 =
                "ARCH REJECTED concern1: sidecar decision ledger because a second source of truth drifts";
            writeWorkingCard(isoRoot, "dr-two-notes-one-slot", ALIAS, {
                owner_notes: [NOTE_1, NOTE_2],
            });
            expectCloseoutRefusal(
                "dr-two-notes-one-slot",
                "completed",
                decisionRecordBlock({
                    version: 3,
                    items: [
                        slot(
                            noteId(NOTE_1),
                            "verbatim",
                            { kind: "commit" },
                            "bind preservation per decision item so independent choices survive retirement",
                            { provenance: "owner chat, 2026-10-26 — verbatim note" },
                        ),
                    ],
                }),
                noteId(NOTE_2),
                "present on the card with NO slot",
                "One slot per decision item",
            );
        }

        // (11b) Surplus slot — an id that is neither a current item nor a
        //       stored slot — is a fabricated inventory and refused.
        writeWorkingCard(isoRoot, "dr-surplus-slot", ALIAS);
        expectCloseoutRefusal(
            "dr-surplus-slot",
            "completed",
            decisionRecordBlock({
                version: 3,
                items: [
                    slot(
                        noteId("a slot for an item that does not exist anywhere"),
                        "verbatim",
                        { kind: "commit" },
                        "a slot for an item that does not exist anywhere",
                    ),
                ],
            }),
            "unknown_item",
            noteId("a slot for an item that does not exist anywhere"),
            "matches no decision item",
        );

        // (11c) PER-ITEM binding: a verbatim handle drawn from Note A's
        //       text but declared for Note B's slot is refused (v2 bound
        //       excerpts per FIELD — the concatenation let one note's text
        //       cover another's slot).
        {
            const NOTE_A =
                "NOTE A: approved the per-item binding rule for independent decisions";
            const NOTE_B =
                "NOTE B: rejected the field-concatenation binding as too loose";
            writeWorkingCard(isoRoot, "dr-cross-item-handle", ALIAS, {
                owner_notes: [NOTE_A, NOTE_B],
            });
            expectCloseoutRefusal(
                "dr-cross-item-handle",
                "completed",
                decisionRecordBlock({
                    version: 3,
                    items: [
                        slot(
                            noteId(NOTE_A),
                            "verbatim",
                            { kind: "commit" },
                            "approved the per-item binding rule for independent decisions",
                            { provenance: "owner chat, 2026-10-27 — verbatim note" },
                        ),
                        slot(
                            noteId(NOTE_B),
                            "verbatim",
                            { kind: "commit" },
                            "approved the per-item binding rule for independent decisions",
                            { provenance: "owner chat, 2026-10-27 — verbatim note" },
                        ),
                    ],
                }),
                "NOT drawn from that item's own content",
                noteId(NOTE_B),
            );
        }

        // (12) VALID decision-bearing closeout: both notes declared (one
        //      verbatim, one sanitized-with-provenance), the incoming
        //      measured outcome declared verbatim on an artifact carrier —
        //      closeout SUCCEEDS and the record PERSISTS on the card.
        {
            const MEASURED_TEXT = "decision content preserved in git";
            const VALID_1 =
                "OWNER APPROVAL 2026-10-27: approved the v3 item-enumerated contract.";
            const VALID_2 =
                "REJECTED ALTERNATIVE: per-item excerpts — deepens the privacy conflict it was meant to solve.";
            writeWorkingCard(isoRoot, "dr-valid", ALIAS, {
                owner_notes: [VALID_1, VALID_2],
            });
            const ok = runCloseout(
                "dr-valid",
                "completed",
                "# Closeout\n\n" +
                    decisionRecordBlock({
                        version: 3,
                        items: [
                            slot(
                                noteId(VALID_1),
                                "verbatim",
                                { kind: "commit" },
                                VALID_1,
                                { provenance: "owner chat m0066, 2026-10-27 — verbatim note on the card" },
                            ),
                            slot(
                                noteId(VALID_2),
                                "sanitized",
                                { kind: "artifact", path: "docs/checkpoints/2026-10-27-dr-valid.md" },
                                `decision-item: dr-valid#${noteId(VALID_2)} (sanitized)`,
                                {
                                    provenance:
                                        "rejected-alternative specifics withheld as private; owner chat m0066 authorized the v3 replacement and the withholding",
                                },
                            ),
                            slot(
                                measuredId(MEASURED_TEXT),
                                "verbatim",
                                { kind: "artifact", path: "docs/checkpoints/2026-10-27-dr-valid.md" },
                                MEASURED_TEXT,
                            ),
                        ],
                    }),
                { measuredOutcome: MEASURED_TEXT },
            );
            assert.equal(ok.task.status, "completed", "(12) closeout should complete");
            const stored = JSON.parse(
                fs.readFileSync(cardPath(isoRoot, "dr-valid"), "utf8"),
            );
            assert.equal(stored.status, "completed");
            assert.ok(
                stored.decision_record &&
                    stored.decision_record.version === 3 &&
                    Array.isArray(stored.decision_record.items) &&
                    stored.decision_record.items.length === 3,
                "(12) decision_record must persist on the card with all three slots",
            );
            assert.equal(
                stored.decision_record.items[0].id,
                noteId(VALID_1),
                "(12) the stored slot id is the CONTENT-BOUND owner-note identity",
            );
            assert.equal(stored.decision_record.items[1].disposition, "sanitized");
        }

        // (13) Routine on a clean card -> SUCCEEDS (the empty-inventory
        //      burden for ordinary work — no mandatory docs).
        writeWorkingCard(isoRoot, "dr-routine-clean", ALIAS);
        const routineOk = runCloseout(
            "dr-routine-clean",
            "completed",
            "# Routine closeout\n\n" + decisionRecordBlock(routineRecord()),
        );
        assert.equal(
            routineOk.task.status,
            "completed",
            "(13) routine closeout on a clean card must succeed",
        );
        assert.equal(
            JSON.parse(
                fs.readFileSync(cardPath(isoRoot, "dr-routine-clean"), "utf8"),
            ).decision_record.items.length,
            0,
            "(13) routine attestation persists as an empty inventory",
        );

        // (14) Routine + owner notes -> refused naming the item (the
        //      routine contradiction, item-enumerated).
        {
            const ROUTINE_NOTE = "OWNER APPROVAL 2026-10-27: approved the retention fix.";
            writeWorkingCard(isoRoot, "dr-routine-ownernotes", ALIAS, {
                owner_notes: [ROUTINE_NOTE],
            });
            expectCloseoutRefusal(
                "dr-routine-ownernotes",
                "reported",
                decisionRecordBlock(routineRecord()),
                "empty inventory",
                noteId(ROUTINE_NOTE),
                "OWNER APPROVAL 2026-10-27",
            );
        }

        // (15) Routine + INCOMING measuredOutcome -> refused naming
        //      measured_outcome (content-bound singleton id).
        writeWorkingCard(isoRoot, "dr-routine-measured", ALIAS);
        let thrown = null;
        try {
            runCloseout(
                "dr-routine-measured",
                "reported",
                decisionRecordBlock(routineRecord()),
                { measuredOutcome: "gate blocks 3 destruction cases" },
            );
        } catch (error) {
            thrown = error;
        }
        assert.ok(thrown, "(15) routine + incoming measured_outcome must refuse");
        assert.ok(
            String(thrown.message).includes(
                measuredId("gate blocks 3 destruction cases"),
            ),
            `(15) refusal must name the measured_outcome item's content-bound id; got: ${thrown.message}`,
        );

        // =====================================================================
        // (16) C2 review classification: an ordinary CLEAN-APPROVE review
        //      verdict is a PROCESS record — a routine closeout on a card
        //      that carries one SUCCEEDS. A NON-APPROVE verdict changed the
        //      card's trajectory and is decision-class: a routine closeout
        //      is refused naming last_review + the task_reviewed event item.
        // =====================================================================
        writeWorkingCard(isoRoot, "dr-c2-approve", ALIAS, {
            last_review: {
                path: ".local/coordinator/reports/dr-c2-approve/review.md",
                reviewed_at: "2026-10-27T00:00:00Z",
                title: "Coordinator Review",
                status: "completed",
                summary: "Approve. Routine work; nothing decision-class.",
                next_action: "retire",
            },
            history: [
                {
                    at: "2026-10-27T00:00:00Z",
                    event: "task_reviewed",
                    status: "completed",
                    note: "clean approve recorded",
                },
            ],
        });
        const c2Approve = runCloseout(
            "dr-c2-approve",
            "completed",
            "# Routine closeout\n\n" + decisionRecordBlock(routineRecord()),
        );
        assert.equal(
            c2Approve.task.status,
            "completed",
            "(16) routine closeout after a clean-approve review must succeed (process record, not decision-class)",
        );

        writeWorkingCard(isoRoot, "dr-c2-blocked", ALIAS, {
            last_review: {
                path: ".local/coordinator/reports/dr-c2-blocked/review.md",
                reviewed_at: "2026-10-27T00:00:00Z",
                title: "Coordinator Review",
                status: "blocked",
                summary: "Blocked: ownership question must be answered first",
                next_action: "rework",
            },
            history: [
                {
                    at: "2026-10-27T00:00:00Z",
                    event: "task_reviewed",
                    status: "blocked",
                    note: "blocked: ownership question must be answered first",
                },
            ],
        });
        expectCloseoutRefusal(
            "dr-c2-blocked",
            "completed",
            decisionRecordBlock(routineRecord()),
            "last_review",
            decisionItemId(
                "task_reviewed",
                "blocked: ownership question must be answered first\nstatus: blocked",
            ),
        );

        // (16b) The honest path: slots for the review items -> closeout
        //       succeeds (the promotion the refusal asks for). The history
        //       index is enumerated from the card itself via the exported
        //       classifier, so the fixture cannot drift from the item ids.
        writeWorkingCard(isoRoot, "dr-c2-blocked-covered", ALIAS, {
            last_review: {
                path: ".local/coordinator/reports/dr-c2-blocked-covered/review.md",
                reviewed_at: "2026-10-27T00:00:00Z",
                title: "Coordinator Review",
                status: "blocked",
                summary: "Blocked: ownership question must be answered first",
                next_action: "rework",
            },
            history: [
                {
                    at: "2026-10-27T00:00:00Z",
                    event: "task_reviewed",
                    status: "blocked",
                    note: "blocked: ownership question must be answered first",
                },
            ],
        });
        const reviewItems = collectDecisionItems(
            JSON.parse(
                fs.readFileSync(
                    cardPath(isoRoot, "dr-c2-blocked-covered"),
                    "utf8",
                ),
            ),
        ).map((item) => item.id);
        assert.deepEqual(
            reviewItems.sort(),
            [
                decisionItemId(
                    "last_review",
                    "Coordinator Review\nblocked\nBlocked: ownership question must be answered first\nrework",
                ),
                decisionItemId(
                    "task_reviewed",
                    "blocked: ownership question must be answered first\nstatus: blocked",
                ),
            ].sort(),
            "(16b) item enumeration must classify the blocked review as decision-class (content-bound ids)",
        );
        const c2Covered = runCloseout(
            "dr-c2-blocked-covered",
            "completed",
            "# Closeout\n\n" +
                decisionRecordBlock({
                    version: 3,
                    items: reviewItems.map((id) =>
                        slot(
                            id,
                            "curated",
                            { kind: "commit" },
                            `decision-item: dr-c2-blocked-covered#${id} (curated)`,
                        ),
                    ),
                }),
        );
        assert.equal(
            c2Covered.task.status,
            "completed",
            "(16b) non-approve review covered by per-item slots must close out",
        );

        // =====================================================================
        // (17) ADD-ONLY AMENDMENT (owner condition 2 — the sanctioned
        //      post-closeout amendment path). A card that already stores a
        //      v3 record re-closes out: dropping a stored slot, or
        //      rewriting an immutable field of one, is refused; retaining
        //      stored slots verbatim plus a fresh slot for a NEW item
        //      succeeds. Routine cards never touch this path (their
        //      inventory is empty).
        // =====================================================================
        {
            const id = "dr-amend";
            const NOTE0 =
                "OWNER APPROVAL 2026-10-27: approved the add-only amendment rule";
            const AMEND_EVENT_NOTE =
                "verdict recorded: consequential objection stands; rework required";
            const REVIEW_ITEM_ID = decisionItemId(
                "task_reviewed",
                `${AMEND_EVENT_NOTE}\nstatus: blocked`,
            );
            writeWorkingCard(isoRoot, id, ALIAS, { owner_notes: [NOTE0] });
            const first = runCloseout(
                id,
                "completed",
                decisionRecordBlock({
                    version: 3,
                    items: [
                        slot(noteId(NOTE0), "verbatim", { kind: "commit" }, NOTE0, {
                            provenance: "owner chat m0066, 2026-10-27 — verbatim note",
                        }),
                    ],
                }),
            );
            assert.equal(first.task.status, "completed", "(17) first closeout ok");

            // Simulate a post-closeout sanctioned addition (a blocked review
            // re-opening the card for rework) by patching the stored card,
            // then re-closeout from `blocked`-shaped state (status working).
            const card = JSON.parse(fs.readFileSync(cardPath(isoRoot, id), "utf8"));
            card.status = "working";
            card.active_session_alias = ALIAS;
            card.history.push({
                at: "2026-10-27T00:05:00Z",
                event: "task_reviewed",
                status: "blocked",
                note: AMEND_EVENT_NOTE,
            });
            fs.writeFileSync(cardPath(isoRoot, id), JSON.stringify(card, null, 2));

            // (a) DROPPING the stored slot (declaring only the new review
            //     item) -> refused as amendment_drop.
            expectCloseoutRefusal(
                id,
                "completed",
                decisionRecordBlock({
                    version: 3,
                    items: [
                        slot(
                            REVIEW_ITEM_ID,
                            "curated",
                            { kind: "commit" },
                            `decision-item: ${id}#${REVIEW_ITEM_ID} (curated)`,
                        ),
                    ],
                }),
                "amendment_drop",
                noteId(NOTE0),
                "ADD-ONLY",
            );

            // (b) RETAINING the slot id but REWRITING its handle -> refused
            //     as amendment_rewrite.
            expectCloseoutRefusal(
                id,
                "completed",
                decisionRecordBlock({
                    version: 3,
                    items: [
                        slot(
                            noteId(NOTE0),
                            "verbatim",
                            { kind: "commit" },
                            "approved the add-only amendment rule",
                            {
                                provenance:
                                    "owner chat m0066, 2026-10-27 — verbatim note",
                            },
                        ),
                        slot(
                            REVIEW_ITEM_ID,
                            "curated",
                            { kind: "commit" },
                            `decision-item: ${id}#${REVIEW_ITEM_ID} (curated)`,
                        ),
                    ],
                }),
                "amendment_rewrite",
                noteId(NOTE0),
            );

            // (c) The sanctioned amendment: stored slot VERBATIM + fresh
            //     slot for the new review item -> succeeds.
            const amended = runCloseout(
                id,
                "completed",
                decisionRecordBlock({
                    version: 3,
                    items: [
                        slot(noteId(NOTE0), "verbatim", { kind: "commit" }, NOTE0, {
                            provenance: "owner chat m0066, 2026-10-27 — verbatim note",
                        }),
                        slot(
                            REVIEW_ITEM_ID,
                            "curated",
                            { kind: "commit" },
                            `decision-item: ${id}#${REVIEW_ITEM_ID} (curated)`,
                        ),
                    ],
                }),
            );
            assert.equal(
                amended.task.status,
                "completed",
                "(17c) add-only re-closeout with stored slots verbatim must succeed",
            );
            const storedAmended = JSON.parse(
                fs.readFileSync(cardPath(isoRoot, id), "utf8"),
            );
            assert.equal(
                storedAmended.decision_record.items.length,
                2,
                "(17c) amended inventory persists with both slots",
            );
        }
    } finally {
        if (prevCoordRoot === undefined) {
            delete process.env.OPENCODE_LOCAL_COORDINATOR_ROOT;
        } else {
            process.env.OPENCODE_LOCAL_COORDINATOR_ROOT = prevCoordRoot;
        }
        fs.rmSync(isoRoot, { recursive: true, force: true });
    }
});

// Schema conformance: records produced by SUCCESSFUL closeouts pass the
// SCHEMA (loaded from the rendered docs/coordination/schemas/ tree), and the
// schema checker is non-vacuous (it rejects unknown-key, wrong-version,
// bad-disposition, and legacy-shape-under-v3 records). One contract: on
// these axes gate-accepted == schema-accepted.
test("decision-record schema conformance: successful closeouts produce schema-valid records", () => {
    const isoRoot = fs.mkdtempSync(path.join(os.tmpdir(), "dr-schema-"));
    const prevCoordRoot = process.env.OPENCODE_LOCAL_COORDINATOR_ROOT;
    process.env.OPENCODE_LOCAL_COORDINATOR_ROOT = isoRoot;
    bindSessionName(SESSION_ID, ALIAS);

    try {
        // Non-vacuity controls: the schema checker itself must reject the
        // nonconformant shapes (otherwise the conformance assertions below
        // prove nothing).
        assert.ok(
            schemaErrors(
                {
                    version: 3,
                    items: [
                        {
                            id: "owner_notes[fnv1a:0123456789abcdef.1]",
                            disposition: "verbatim",
                            carrier: { kind: "commit" },
                            handle: "x".repeat(30),
                            bogus: 1,
                        },
                    ],
                },
                DR_SUBSCHEMA,
                "dr",
            ).length > 0,
            "schema checker must reject slot-level unknown keys",
        );
        assert.ok(
            schemaErrors(
                { version: 2, items: [] },
                DR_SUBSCHEMA,
                "dr",
            ).length > 0,
            "schema checker must reject wrong versions",
        );
        assert.ok(
            schemaErrors(
                {
                    version: 3,
                    items: [
                        {
                            id: "owner_notes[fnv1a:0123456789abcdef.1]",
                            disposition: "excerpt",
                            carrier: { kind: "commit" },
                            handle: "x".repeat(30),
                        },
                    ],
                },
                DR_SUBSCHEMA,
                "dr",
            ).length > 0,
            "schema checker must reject dispositions outside the enum",
        );
        assert.ok(
            schemaErrors(
                {
                    version: 3,
                    items: [
                        {
                            id: "owner_notes[fnv1a:0123456789abcdef.1]",
                            disposition: "verbatim",
                            carrier: { kind: "commit" },
                        },
                    ],
                },
                DR_SUBSCHEMA,
                "dr",
            ).length > 0,
            "schema checker must reject slots missing required handle",
        );
        // Legacy v1/v2 stored shape remains schema-valid (in-flight cards).
        assert.deepEqual(
            schemaErrors(
                {
                    version: 1,
                    entries: [{ kind: "routine", note: "legacy in-flight" }],
                },
                DR_SUBSCHEMA,
                "dr",
            ),
            [],
            "schema must keep accepting stored legacy version-1 records",
        );

        // Real closeout producing a three-slot record (verbatim + sanitized
        // + artifact verbatim). Slot ids are content-bound (the B-F1 fix):
        // derived from each item's own text via the exported factory.
        {
            const CONF_1 = "OWNER APPROVAL 2026-10-27: approved.";
            const CONF_2 =
                "REJECTED ALTERNATIVE: per-item excerpts for the privacy conflict.";
            const CONF_MEASURED = "decision content preserved in git";
            writeWorkingCard(isoRoot, "dr-conf-slots", ALIAS, {
                owner_notes: [CONF_1, CONF_2],
            });
            runCloseout(
                "dr-conf-slots",
                "completed",
                "# Closeout\n\n" +
                    decisionRecordBlock({
                        version: 3,
                        items: [
                            slot(
                                noteId(CONF_1),
                                "verbatim",
                                { kind: "commit" },
                                CONF_1,
                                { provenance: "owner chat m0066, 2026-10-27" },
                            ),
                            slot(
                                noteId(CONF_2),
                                "sanitized",
                                { kind: "artifact", path: "docs/checkpoints/x.md" },
                                `decision-item: dr-conf-slots#${noteId(CONF_2)} (sanitized)`,
                                { provenance: "withholding authorized in owner chat m0066" },
                            ),
                            slot(
                                measuredId(CONF_MEASURED),
                                "verbatim",
                                { kind: "artifact", path: "docs/checkpoints/x.md" },
                                CONF_MEASURED,
                            ),
                        ],
                    }),
                { measuredOutcome: CONF_MEASURED },
            );
        }
        const slots = JSON.parse(
            fs.readFileSync(cardPath(isoRoot, "dr-conf-slots"), "utf8"),
        ).decision_record;
        assert.deepEqual(
            schemaErrors(slots, DR_SUBSCHEMA, "decision_record"),
            [],
            "three-slot record from a successful closeout must be schema-valid",
        );

        // Real closeout producing a routine record.
        writeWorkingCard(isoRoot, "dr-conf-routine", ALIAS);
        runCloseout(
            "dr-conf-routine",
            "completed",
            "# Routine closeout\n\n" + decisionRecordBlock(routineRecord()),
        );
        const routine = JSON.parse(
            fs.readFileSync(cardPath(isoRoot, "dr-conf-routine"), "utf8"),
        ).decision_record;
        assert.deepEqual(
            schemaErrors(routine, DR_SUBSCHEMA, "decision_record"),
            [],
            "routine record from a successful closeout must be schema-valid",
        );
    } finally {
        if (prevCoordRoot === undefined) {
            delete process.env.OPENCODE_LOCAL_COORDINATOR_ROOT;
        } else {
            process.env.OPENCODE_LOCAL_COORDINATOR_ROOT = prevCoordRoot;
        }
        fs.rmSync(isoRoot, { recursive: true, force: true });
    }
});
