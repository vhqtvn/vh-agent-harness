// Parity tests for the JS rewrite-parity validator mirror.
//
// The python validator (rewrite-parity-validate.py) is the REFERENCE. This JS
// mirror (rewrite-parity-validate.js) is used by the closeout transition
// (state-lib.js saveCoordinationTaskCloseout). Both must accept/reject the
// SAME golden fixtures under tests/fixtures/rewrite-parity/ identically — that
// is the load-bearing cross-language parity contract.
//
// These tests also cover the closeout-body extraction pipeline: the closeout
// function parses fenced ```rewrite-parity blocks from a markdown body and
// validates each at completion stage. We test that pipeline directly (the
// extraction + validation) against closeout-body-shaped markdown, which is
// exactly what saveCoordinationTaskCloseout exercises internally.
//
// Run:  vh-agent-harness exec node --test tests/scripts/rewrite-parity-validate.test.js
//       (or: node --test tests/scripts/rewrite-parity-validate.test.js)

import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const SCRIPT = join(
    __dirname,
    "..",
    "..",
    "templates",
    "core",
    ".opencode",
    "scripts",
    "rewrite-parity-validate.js",
);
const FIXTURES = join(__dirname, "..", "fixtures", "rewrite-parity");

const {
    extractRewriteParityContract,
    extractAllRewriteParityBlocks,
    validateRewriteParityStructural,
    validateRewriteParityCompletion,
    validateRewriteParityPrecommit,
    REWRITE_PARITY_VALID_MODES,
    REWRITE_PARITY_VALID_RESULTS,
} = await import(SCRIPT);

function loadFixture(name) {
    return JSON.parse(
        readFileSync(join(FIXTURES, name), "utf-8"),
    );
}

// ---------------------------------------------------------------------------
// Fixture parity: the JS validator must agree with the python reference on
// every golden fixture. "valid" fixtures have zero structural errors; the
// "invalid" fixtures have one or more.
// ---------------------------------------------------------------------------

const STRUCTURAL_VALID = [
    "valid-planned.json",
    "valid-completion.json",
    "valid-modify-planned.json",
];

const STRUCTURAL_INVALID = [
    "invalid-bad-mode.json",
    "invalid-no-verifier.json",
    "invalid-bad-result-enum.json",
    "invalid-duplicate-ids.json",
    // Completion-failing fixtures are STRUCTURALLY valid (they fail the
    // completion stage, not the structural core). Verified separately below.
];

test("validateRewriteParityStructural accepts all structurally-valid fixtures", () => {
    for (const name of STRUCTURAL_VALID) {
        const contract = loadFixture(name);
        const errors = validateRewriteParityStructural(contract);
        assert.deepEqual(
            errors,
            [],
            name + " should have zero structural errors (JS==python parity)",
        );
    }
});

test("validateRewriteParityStructural rejects structurally-invalid fixtures", () => {
    for (const name of STRUCTURAL_INVALID) {
        const contract = loadFixture(name);
        const errors = validateRewriteParityStructural(contract);
        assert.ok(
            errors.length > 0,
            name + " should have structural errors (JS==python parity)",
        );
    }
});

// Regression lock for a cross-surface parity bug: Python `True == 1` meant the
// python validator accepted `{"version": true}` while JS (strict !==) and Go
// (decodes to bool) rejected it. The python validator now rejects booleans
// explicitly; this test pins the JS side of that agreement so a future
// relaxation is caught. See defer-rp-fixture-parity for full cross-language
// fixture-driver parity (currently only JS drives all 9 golden fixtures).
test("validateRewriteParityStructural rejects version:true (cross-surface parity with python/Go)", () => {
    const errors = validateRewriteParityStructural({
        version: true,
        applies: "x",
        mode: "deletion_replacement",
        prior_surface: {
            id: "a",
            revision: "sha",
            paths: ["p"],
            inventory_complete: false,
        },
        behaviors: [],
    });
    assert.ok(
        errors.some((e) => e.includes("version")),
        "version:true must be rejected (parity with python/Go)",
    );
});

// Completion-failing fixtures are structurally valid.
test("completion-failing fixtures are structurally valid (fail at completion stage only)", () => {
    for (const name of [
        "invalid-completion-not-demonstrable.json",
        "invalid-completion-no-receipt.json",
    ]) {
        const contract = loadFixture(name);
        const errors = validateRewriteParityStructural(contract);
        assert.deepEqual(errors, [], name + " is structurally valid");
    }
});

// ---------------------------------------------------------------------------
// Completion stage parity.
// ---------------------------------------------------------------------------

test("validateRewriteParityCompletion accepts valid-completion fixture", () => {
    const contract = loadFixture("valid-completion.json");
    const errors = validateRewriteParityCompletion(contract);
    assert.deepEqual(
        errors,
        [],
        "valid-completion should pass the completion stage",
    );
});

test("validateRewriteParityCompletion rejects planned-stage contract (behaviors not proven)", () => {
    const contract = loadFixture("valid-planned.json");
    const errors = validateRewriteParityCompletion(contract);
    assert.ok(errors.length > 0, "planned-stage contract must fail completion");
    assert.ok(
        errors.every((e) => e.includes("requires every behavior proven")),
        "errors should cite the proven requirement",
    );
});

test("validateRewriteParityCompletion rejects not-demonstrable behavior", () => {
    const contract = loadFixture("invalid-completion-not-demonstrable.json");
    const errors = validateRewriteParityCompletion(contract);
    assert.ok(errors.length > 0, "not-demonstrable must block completion");
    assert.ok(
        errors.some((e) => e.includes("not-demonstrable")),
        "error should name the not-demonstrable status (routes to defer)",
    );
});

test("validateRewriteParityCompletion rejects proven-without-receipt", () => {
    const contract = loadFixture("invalid-completion-no-receipt.json");
    const errors = validateRewriteParityCompletion(contract);
    assert.ok(errors.length > 0, "proven without receipt must block completion");
    assert.ok(
        errors.some((e) => e.includes("receipt")),
        "error should cite the missing receipt",
    );
});

// ---------------------------------------------------------------------------
// Precommit stage parity (cross-check + revision-binding).
// ---------------------------------------------------------------------------

test("validateRewriteParityPrecommit accepts matching diff + revision", () => {
    const contract = loadFixture("valid-planned.json");
    const diffFiles = [
        { status: "D", path: "src/x/a.go" },
        { status: "D", path: "src/x/b.go" },
    ];
    const errors = validateRewriteParityPrecommit(
        contract,
        diffFiles,
        "0000000000000000000000000000000000000001",
    );
    assert.deepEqual(errors, [], "matching diff + revision should pass precommit");
});

test("validateRewriteParityPrecommit rejects revision mismatch", () => {
    const contract = loadFixture("valid-planned.json");
    const diffFiles = [
        { status: "D", path: "src/x/a.go" },
        { status: "D", path: "src/x/b.go" },
    ];
    const errors = validateRewriteParityPrecommit(
        contract,
        diffFiles,
        "deadbeef", // mismatched HEAD
    );
    assert.ok(
        errors.some((e) => e.includes("does not match head_at_acquire")),
        "revision mismatch should be flagged",
    );
});

test("validateRewriteParityPrecommit rejects undeclared deletion (inventory_complete=true)", () => {
    const contract = loadFixture("valid-planned.json"); // declares a.go, b.go
    const diffFiles = [
        { status: "D", path: "src/x/a.go" },
        { status: "D", path: "src/x/b.go" },
        { status: "D", path: "src/x/c.go" }, // undeclared
    ];
    const errors = validateRewriteParityPrecommit(
        contract,
        diffFiles,
        "0000000000000000000000000000000000000001",
    );
    assert.ok(
        errors.some((e) => e.includes("undeclared")),
        "undeclared deletion must be flagged under inventory_complete=true",
    );
});

test("validateRewriteParityPrecommit accepts modification_only_rewrite mode", () => {
    const contract = loadFixture("valid-modify-planned.json");
    const diffFiles = contract.prior_surface.paths.map((p) => ({
        status: "M",
        path: p,
    }));
    const errors = validateRewriteParityPrecommit(
        contract,
        diffFiles,
        contract.prior_surface.revision,
    );
    assert.deepEqual(errors, [], "modification_only_rewrite with matching M diff should pass");
});

// ---------------------------------------------------------------------------
// Extraction parity (raw JSON vs fenced markdown).
// ---------------------------------------------------------------------------

test("extractRewriteParityContract parses raw JSON", () => {
    const raw = JSON.stringify(loadFixture("valid-planned.json"));
    const { contract, error } = extractRewriteParityContract(raw);
    assert.equal(error, null);
    assert.equal(contract.mode, "deletion_replacement");
});

test("extractRewriteParityContract parses fenced ```rewrite-parity block", () => {
    const inner = JSON.stringify(loadFixture("valid-completion.json"), null, 2);
    const markdown =
        "# Closeout\n\nSome prose.\n\n```rewrite-parity\n" + inner + "\n```\n\nMore prose.\n";
    const { contract, error } = extractRewriteParityContract(markdown);
    assert.equal(error, null);
    assert.equal(contract.mode, "deletion_replacement");
    assert.equal(contract.behaviors.length, 2);
});

test("extractRewriteParityContract errors on non-JSON, non-fence input", () => {
    const { contract, error } = extractRewriteParityContract("just some prose, no contract here");
    assert.equal(contract, null);
    assert.ok(error && error.includes("no rewrite-parity contract found"));
});

test("extractRewriteParityContract errors on malformed fence JSON", () => {
    const markdown = "```rewrite-parity\n{not valid json\n```\n";
    const { contract, error } = extractRewriteParityContract(markdown);
    assert.equal(contract, null);
    assert.ok(error && error.includes("malformed"));
});

// ---------------------------------------------------------------------------
// Multi-block extraction (the closeout pipeline).
// ---------------------------------------------------------------------------

test("extractAllRewriteParityBlocks extracts every fenced block from a body", () => {
    const c1 = JSON.stringify(loadFixture("valid-planned.json"), null, 2);
    const c2 = JSON.stringify(loadFixture("valid-completion.json"), null, 2);
    const body =
        "# Closeout\n\n```rewrite-parity\n" + c1 + "\n```\n\nText between.\n\n```rewrite-parity\n" +
        c2 + "\n```\n";
    const blocks = extractAllRewriteParityBlocks(body);
    assert.equal(blocks.length, 2);
    assert.equal(blocks[0].error, null);
    assert.equal(blocks[1].error, null);
    assert.equal(blocks[0].contract.behaviors[0].result.status, "planned");
    assert.equal(blocks[1].contract.behaviors[0].result.status, "proven");
});

test("extractAllRewriteParityBlocks returns empty array for body without blocks", () => {
    assert.deepEqual(extractAllRewriteParityBlocks("no contract here"), []);
    assert.deepEqual(extractAllRewriteParityBlocks(""), []);
});

test("extractAllRewriteParityBlocks surfaces malformed fence as error entry", () => {
    const body = "```rewrite-parity\n{bad json\n```\n";
    const blocks = extractAllRewriteParityBlocks(body);
    assert.equal(blocks.length, 1);
    assert.equal(blocks[0].contract, null);
    assert.ok(blocks[0].error && blocks[0].error.includes("malformed"));
});

// ---------------------------------------------------------------------------
// Closeout-body pipeline: simulate what saveCoordinationTaskCloseout does
// internally (extract blocks from body, validate each at completion stage,
// collect errors). This proves the fenced-block-in-markdown path the closeout
// function exercises.
// ---------------------------------------------------------------------------

test("closeout pipeline: completed + all-proven body yields zero errors", () => {
    const inner = JSON.stringify(loadFixture("valid-completion.json"), null, 2);
    const body = "```rewrite-parity\n" + inner + "\n```\n";
    const blocks = extractAllRewriteParityBlocks(body);
    const collected = [];
    blocks.forEach((blk) => {
        if (blk.error) {
            collected.push(blk.error);
            return;
        }
        collected.push(...validateRewriteParityCompletion(blk.contract));
    });
    assert.deepEqual(collected, []);
});

test("closeout pipeline: completed + not-demonstrable body yields blocking errors", () => {
    const inner = JSON.stringify(
        loadFixture("invalid-completion-not-demonstrable.json"),
        null,
        2,
    );
    const body = "```rewrite-parity\n" + inner + "\n```\n";
    const blocks = extractAllRewriteParityBlocks(body);
    const collected = [];
    blocks.forEach((blk) => {
        if (blk.error) {
            collected.push(blk.error);
            return;
        }
        collected.push(...validateRewriteParityCompletion(blk.contract));
    });
    assert.ok(collected.length > 0, "not-demonstrable must block completion");
    assert.ok(
        collected.some((e) => e.includes("not-demonstrable")),
        "error should name not-demonstrable (routes to defer)",
    );
});

test("closeout pipeline: completed + no-receipt body yields blocking errors", () => {
    const inner = JSON.stringify(
        loadFixture("invalid-completion-no-receipt.json"),
        null,
        2,
    );
    const body = "```rewrite-parity\n" + inner + "\n```\n";
    const blocks = extractAllRewriteParityBlocks(body);
    const collected = [];
    blocks.forEach((blk) => {
        if (blk.error) {
            collected.push(blk.error);
            return;
        }
        collected.push(...validateRewriteParityCompletion(blk.contract));
    });
    assert.ok(collected.length > 0, "missing receipt must block completion");
    assert.ok(
        collected.some((e) => e.includes("receipt")),
        "error should cite the missing receipt",
    );
});

test("closeout pipeline: body without any rewrite-parity block yields zero errors (opt-in)", () => {
    const body = "# Ordinary closeout\n\nNo rewrite-parity contract declared.\n";
    const blocks = extractAllRewriteParityBlocks(body);
    assert.equal(blocks.length, 0, "ordinary closeouts carry zero rewrite-parity burden");
});

// ---------------------------------------------------------------------------
// Frozen vocab exports.
// ---------------------------------------------------------------------------

test("REWRITE_PARITY_VALID_MODES is frozen and complete", () => {
    assert.deepEqual([...REWRITE_PARITY_VALID_MODES], [
        "deletion_replacement",
        "modification_only_rewrite",
    ]);
    assert.ok(Object.isFrozen(REWRITE_PARITY_VALID_MODES));
});

test("REWRITE_PARITY_VALID_RESULTS is frozen and complete", () => {
    assert.deepEqual([...REWRITE_PARITY_VALID_RESULTS], [
        "planned",
        "proven",
        "failed",
        "skipped",
        "not-demonstrable",
    ]);
    assert.ok(Object.isFrozen(REWRITE_PARITY_VALID_RESULTS));
});
