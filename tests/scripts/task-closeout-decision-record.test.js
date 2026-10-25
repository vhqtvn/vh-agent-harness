// Decision-record closeout gate tests (retention preservation,
// owner-approved 2026-10-25).
//
// Pins the CLOSEOUT half of the retention-preservation gate in
// saveCoordinationTaskCloseout: every new closeout must declare exactly ONE
// fenced ```decision-record block with entries of kind commit | artifact |
// routine (there is deliberately NO inline kind — content only in the local
// closeout report is not durable); structural validation per kind matching
// the task-card schema's per-kind allowed keys EXACTLY (unknown keys,
// marker-on-routine, and record-level junk refused — B2); JSON bodies that
// parse to null/primitives/arrays refuse with a collected StateError, never
// a raw TypeError (B4); the MECHANICAL routine contradiction check against
// explicit decision-class card fields (owner_notes, measured_outcome incl.
// the incoming closeout option, non-empty open_questions, last_review /
// task_reviewed history); the parsed record persisting on the card for the
// retirement verifier; and schema conformance of records produced by
// successful closeouts (rendered task-card.schema.json). Dash-prefixed
// markers/revisions are refused at declaration naming the option-injection
// rule (F1); the retirement-side half of that pin lives in
// tests/scripts/task-retire-decision-record.test.js.
//
// The RETIREMENT half (git resolution of declared pointers, legacy
// compatibility, force warning) is pinned by
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
} = await import(pathToFileURL(STATE_LIB).href);

// --- B2: schema-driven conformance checker -----------------------------------
// Loads the RENDERED task-card schema (kept in sync with templates/core by
// `make update` — the same discipline as importing the rendered state-lib
// above) and validates a decision_record against the schema's
// decision_record subschema using exactly the draft-07 subset that subschema
// uses (type, required, properties, additionalProperties:false, const,
// minLength, minItems, anyOf). Pins ONE contract: records the closeout gate
// ACCEPTS must be records the SCHEMA accepts, and the schema checker itself
// rejects marker-on-routine / unknown-key records (non-vacuous control).
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
    if (schema.const !== undefined) {
        if (node !== schema.const) {
            errs.push(
                `${at}: expected const ${JSON.stringify(schema.const)}`,
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
                if (schema.items.anyOf) {
                    const matchesSomeVariant = schema.items.anyOf.some(
                        (variant) =>
                            schemaErrors(item, variant, `${at}[${i}]`)
                                .length === 0,
                    );
                    if (!matchesSomeVariant) {
                        errs.push(
                            `${at}[${i}]: matches no schema entry variant`,
                        );
                    }
                } else {
                    errs.push(
                        ...schemaErrors(item, schema.items, `${at}[${i}]`),
                    );
                }
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
        version: 1,
        entries: [{ kind: "routine", note: note || "nothing decision-class" }],
    };
}

function cardPath(isoRoot, id) {
    return path.join(isoRoot, "tasks", `${id}.json`);
}

// Mirrors closeout-stage2-rewrite-parity.test.js: a complete valid `working`
// card written directly (every strict core field) so the closeout write-back
// passes ensureCoordinationTaskCoreFields. Extra fields (owner_notes, ...)
// via the `extra` argument.
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

test("decision-record closeout gate: block required, kinds enforced, routine contradictions named", () => {
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
            "commit | artifact | routine",
        );
        assert.equal(
            JSON.parse(fs.readFileSync(cardPath(isoRoot, "dr-no-block"), "utf8"))
                .status,
            "working",
            "(1) card must remain working after refusal",
        );

        // (2) TWO blocks -> REFUSED (exactly one; merge entries).
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

        // (4) Unknown kind — including the deliberately-absent "inline" —
        //     -> REFUSED naming the no-inline rule.
        writeWorkingCard(isoRoot, "dr-inline-kind", ALIAS);
        expectCloseoutRefusal(
            "dr-inline-kind",
            "reported",
            decisionRecordBlock({
                version: 1,
                entries: [{ kind: "inline", note: "see local report" }],
            }),
            'kind must be one of commit | artifact | routine',
            "no \"inline\" kind",
        );

        // (5) artifact without path -> REFUSED.
        writeWorkingCard(isoRoot, "dr-artifact-nopath", ALIAS);
        expectCloseoutRefusal(
            "dr-artifact-nopath",
            "reported",
            decisionRecordBlock({
                version: 1,
                entries: [{ kind: "artifact", marker: "x" }],
            }),
            "artifact entries require path",
        );

        // (6) routine + card owner_notes -> REFUSED naming owner_notes
        //     (mechanical contradiction, negative (c) of the owner's
        //     acceptance tests).
        writeWorkingCard(isoRoot, "dr-routine-ownernotes", ALIAS, {
            owner_notes: [
                "OWNER APPROVAL 2026-10-25: approved the retention fix.",
            ],
        });
        expectCloseoutRefusal(
            "dr-routine-ownernotes",
            "reported",
            decisionRecordBlock(routineRecord()),
            "routine decision_record",
            "owner_notes",
            "OWNER APPROVAL 2026-10-25",
        );

        // (7) routine + INCOMING measuredOutcome option -> REFUSED naming
        //     measured_outcome (the contradiction check covers the value the
        //     closeout itself is about to persist, not just stored fields).
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
        assert.ok(thrown, "(7) routine + incoming measured_outcome must refuse");
        assert.ok(
            String(thrown.message).includes("measured_outcome"),
            `(7) refusal must name measured_outcome; got: ${thrown.message}`,
        );

        // (8) routine mixed with a commit entry -> REFUSED (exclusivity).
        writeWorkingCard(isoRoot, "dr-routine-mixed", ALIAS);
        expectCloseoutRefusal(
            "dr-routine-mixed",
            "reported",
            decisionRecordBlock({
                version: 1,
                entries: [
                    { kind: "routine" },
                    { kind: "commit", marker: "x" },
                ],
            }),
            "must be the ONLY entry",
        );

        // (9) VALID commit+artifact entries -> closeout SUCCEEDS and the
        //     parsed decision_record PERSISTS on the card (the retirement
        //     verifier's input). Card owner_notes do NOT contradict
        //     commit/artifact entries — the contradiction check is
        //     routine-only.
        writeWorkingCard(isoRoot, "dr-valid", ALIAS, {
            owner_notes: ["OWNER APPROVAL 2026-10-25: approved."],
        });
        const ok = runCloseout(
            "dr-valid",
            "completed",
            "# Closeout\n\n" +
                decisionRecordBlock({
                    version: 1,
                    entries: [
                        {
                            kind: "commit",
                            marker: "OWNER-AUTH dr-valid",
                            note: "authorization + rejected alternative in the landing commit body",
                        },
                        {
                            kind: "artifact",
                            path: "docs/checkpoints/2026-10-25-dr-valid.md",
                            note: "measured outcome in a durable file",
                        },
                    ],
                }),
            { measuredOutcome: "decision content preserved in git" },
        );
        assert.equal(ok.task.status, "completed", "(9) closeout should complete");
        const stored = JSON.parse(
            fs.readFileSync(cardPath(isoRoot, "dr-valid"), "utf8"),
        );
        assert.equal(stored.status, "completed");
        assert.ok(
            stored.decision_record &&
                Array.isArray(stored.decision_record.entries) &&
                stored.decision_record.entries.length === 2,
            "(9) decision_record must persist on the card with both entries",
        );
        assert.equal(stored.decision_record.entries[0].kind, "commit");
        assert.equal(stored.decision_record.entries[1].kind, "artifact");

        // (10) routine on a clean card -> SUCCEEDS (the one-line routine
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
            "(10) routine closeout on a clean card must succeed",
        );
        assert.equal(
            JSON.parse(
                fs.readFileSync(cardPath(isoRoot, "dr-routine-clean"), "utf8"),
            ).decision_record.entries[0].kind,
            "routine",
            "(10) routine attestation persists",
        );

        // (11) B2 — marker on a routine entry -> REFUSED as an unknown key
        //      (schema variant is kind+note only; the gate matches the
        //      schema EXACTLY — marker used to be accepted pre-dispatch).
        writeWorkingCard(isoRoot, "dr-routine-marker", ALIAS);
        expectCloseoutRefusal(
            "dr-routine-marker",
            "reported",
            decisionRecordBlock({
                version: 1,
                entries: [
                    { kind: "routine", note: "x", marker: "dr-routine-marker" },
                ],
            }),
            "unknown key",
            '"marker"',
            "routine entries allow exactly kind | note",
        );

        // (12) B2 — undeclared keys everywhere -> REFUSED: revision on a
        //      routine entry, path on a commit entry, record-level junk.
        writeWorkingCard(isoRoot, "dr-routine-revision", ALIAS);
        expectCloseoutRefusal(
            "dr-routine-revision",
            "reported",
            decisionRecordBlock({
                version: 1,
                entries: [{ kind: "routine", note: "x", revision: "abc" }],
            }),
            "unknown key",
            '"revision"',
        );
        writeWorkingCard(isoRoot, "dr-commit-path", ALIAS);
        expectCloseoutRefusal(
            "dr-commit-path",
            "reported",
            decisionRecordBlock({
                version: 1,
                entries: [
                    { kind: "commit", path: "docs/x.md", marker: "m" },
                ],
            }),
            "unknown key",
            '"path"',
            "commit entries take no path",
        );
        writeWorkingCard(isoRoot, "dr-record-junk", ALIAS);
        expectCloseoutRefusal(
            "dr-record-junk",
            "reported",
            decisionRecordBlock({
                version: 1,
                junk: true,
                entries: [{ kind: "routine", note: "x" }],
            }),
            'unknown key "junk"',
            "exactly version and entries",
        );

        // (13) B4 — JSON bodies that PARSE but are null / primitives /
        //      arrays -> collected StateError, never a raw TypeError (null
        //      used to throw "Cannot read properties of null (reading
        //      'entries')" before the collected errors were thrown).
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
                "must be a JSON object with version and entries",
            );
        }

        // (14) F1 — marker/revision starting with "-" -> REFUSED at
        //      DECLARATION time, naming the option-injection rule. The
        //      retirement verifier hands these strings to git (git grep -e,
        //      rev-parse positional); the class is closed structurally so
        //      no user string that git could parse as an option ever
        //      reaches a positional slot. (A dash marker "-v" on a bare
        //      git grep positional used to invert-match the WORKING TREE
        //      and verify a pointer that never existed.)
        writeWorkingCard(isoRoot, "dr-dash-marker", ALIAS);
        expectCloseoutRefusal(
            "dr-dash-marker",
            "reported",
            decisionRecordBlock({
                version: 1,
                entries: [{ kind: "commit", marker: "-v", note: "dash" }],
            }),
            'must not start with "-"',
            "marker",
        );
        writeWorkingCard(isoRoot, "dr-dash-revision", ALIAS);
        expectCloseoutRefusal(
            "dr-dash-revision",
            "reported",
            decisionRecordBlock({
                version: 1,
                entries: [{ kind: "commit", revision: "-v", marker: "m" }],
            }),
            'must not start with "-"',
            "revision",
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

// B2 — schema conformance: records produced by SUCCESSFUL closeouts pass
// the SCHEMA (loaded from the rendered docs/coordination/schemas/ tree),
// and the schema checker is non-vacuous (it rejects marker-on-routine,
// unknown-key, and empty-entries records — the same shapes the gate
// refuses). One contract: on these axes gate-accepted == schema-accepted.
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
                    version: 1,
                    entries: [{ kind: "routine", note: "x", marker: "m" }],
                },
                DR_SUBSCHEMA,
                "dr",
            ).length > 0,
            "schema checker must reject marker-on-routine",
        );
        assert.ok(
            schemaErrors(
                { version: 1, junk: true, entries: [{ kind: "routine" }] },
                DR_SUBSCHEMA,
                "dr",
            ).length > 0,
            "schema checker must reject record-level unknown keys",
        );
        assert.ok(
            schemaErrors({ version: 1, entries: [] }, DR_SUBSCHEMA, "dr")
                .length > 0,
            "schema checker must reject empty entries",
        );

        // Real closeout producing a commit+artifact record (card
        // owner_notes do not contradict non-routine entries).
        writeWorkingCard(isoRoot, "dr-conf-carriers", ALIAS, {
            owner_notes: ["OWNER APPROVAL 2026-10-25: approved."],
        });
        runCloseout(
            "dr-conf-carriers",
            "completed",
            "# Closeout\n\n" +
                decisionRecordBlock({
                    version: 1,
                    entries: [
                        {
                            kind: "commit",
                            marker: "OWNER-AUTH dr-conf-carriers",
                            note: "authorization in the landing commit body",
                        },
                        {
                            kind: "artifact",
                            path: "docs/checkpoints/2026-10-25-dr-conf.md",
                            note: "measured outcome in a durable file",
                        },
                    ],
                }),
            { measuredOutcome: "decision content preserved in git" },
        );
        const carriers = JSON.parse(
            fs.readFileSync(cardPath(isoRoot, "dr-conf-carriers"), "utf8"),
        ).decision_record;
        assert.deepEqual(
            schemaErrors(carriers, DR_SUBSCHEMA, "decision_record"),
            [],
            "commit+artifact record from a successful closeout must be schema-valid",
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
