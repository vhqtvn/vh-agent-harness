// Regression tests for the deep-research run-packet validator
// (.vh-agent-harness/overlays/native-deep-research-pilot — rendered to
// .opencode/skills/deep-research/scripts/validate-packet.sh).
//
// CRUX (the paths this test exercises end-to-end): the validator's
// fenced-report parsing (a bare ```` >=4-backtick fence is REQUIRED in
// "## Final report"; 3-backtick and language-tagged lines and Markdown
// headings INSIDE an open fence are literal report content — they never
// toggle the fence or terminate locator collection; after the closing
// fence the report is complete and later fenced blocks are excluded
// metadata) and the supported-claim disjunction (single record + full
// literal direct-support note + full depth, OR >=2 cited records
// spanning >=2 distinct source_family values).
//
// The tests spawn the OVERLAY SOURCE script (never the rendered .opencode
// copy, which can go stale) — same source-of-truth convention as
// check-defer-triggers.test.js. Fixtures are written to an OS temp dir.
//
// Run:  vh-agent-harness exec node --test tests/scripts/deep-research-validate-packet.test.js
//       (or: node --test tests/scripts/deep-research-validate-packet.test.js)

import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, dirname } from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const VALIDATOR = join(__dirname, "..", "..",
    ".vh-agent-harness", "overlays", "native-deep-research-pilot",
    "skills", "deep-research", "scripts", "validate-packet.sh");

const DIR = mkdtempSync(join(tmpdir(), "dr-validate-"));
test.after(() => rmSync(DIR, { recursive: true, force: true }));

function run(name, packet) {
    const p = join(DIR, name);
    writeFileSync(p, packet);
    const r = spawnSync("bash", [VALIDATOR, p], { encoding: "utf8" });
    return { status: r.status, out: r.stdout + r.stderr };
}

const EV = (id, fam, depth = "full") =>
    `| ${id} | https://example.test/${id} | "text" | ${depth} | 2026-11-07 | v1 | ${fam} |`;

function packet({ claims, report, families = { E1: "spec-docs", E2: "spec-docs" }, depths = {} }) {
    const evs = Object.keys(families).map((id) => EV(id, families[id], depths[id] || "full")).join("\n");
    const rows = claims.map((c) => `| ${c.claim} | ${c.cls} | ${c.ev} | ${c.notes || ""} |`).join("\n");
    return [
        "# packet",
        "## Evidence records",
        "| id | locator | excerpt | read_depth | retrieved_at | source_version | source_family |",
        "|---|---|---|---|---|---|---|",
        evs,
        "## Claim classifications",
        "| claim | classification | evidence_ids | notes |",
        "|---|---|---|---|",
        rows,
        "## Final report",
        "",
        report,
        "",
        "finalization_check: PASS (test) @ 2026-11-07",
        "",
    ].join("\n");
}

const fenced = (body) => "````\n" + body + "\n````";
const DS_NOTE = "direct support (authoritative spec, single family)";

test("(a) locator appearing only AFTER ##/### headings inside a fenced report is found (exit 0)", () => {
    const r = run("nested.md", packet({
        claims: [{ claim: "c1", cls: "supported", ev: "E1", notes: DS_NOTE }],
        report: fenced("# Report\n\n## Findings\n\n### Detail\n\n- E1 — https://example.test/E1"),
    }));
    assert.equal(r.status, 0, r.out);
});

test("(b) unfenced report section is rejected (exit 1, fence violation)", () => {
    const r = run("unfenced.md", packet({
        claims: [{ claim: "c1", cls: "supported", ev: "E1", notes: DS_NOTE }],
        report: "- E1 — https://example.test/E1",
    }));
    assert.equal(r.status, 1);
    assert.match(r.out, /missing its fenced code block/);
});

test("(c) supported citing two same-family records is rejected (exit 1, family cardinality)", () => {
    const r = run("samefam.md", packet({
        claims: [{ claim: "c1", cls: "supported", ev: "E1, E2", notes: "" }],
        report: fenced("- E1 — https://example.test/E1\n- E2 — https://example.test/E2"),
    }));
    assert.equal(r.status, 1);
    assert.match(r.out, /only 1 distinct non-empty source_family/);
});

test("(d) direct-support single-record supported is accepted (exit 0, exception unchanged)", () => {
    const r = run("direct.md", packet({
        claims: [{ claim: "c1", cls: "supported", ev: "E1", notes: DS_NOTE }],
        report: fenced("- E1 — https://example.test/E1"),
    }));
    assert.equal(r.status, 0, r.out);
});

test("(e1) pseudo-ID E6-derivative-stale in the evidence column is rejected (exit 1)", () => {
    const r = run("pseudo.md", packet({
        claims: [{ claim: "c1", cls: "partial", ev: "E6-derivative-stale", notes: "" }],
        report: fenced("- E1 — https://example.test/E1"),
    }));
    assert.equal(r.status, 1);
    assert.match(r.out, /"E6-derivative-stale" is not a valid E<digits>/);
});

test("(e2) cited locator missing from the fenced report is rejected (exit 1)", () => {
    const r = run("noloc.md", packet({
        claims: [{ claim: "c1", cls: "supported", ev: "E1", notes: DS_NOTE }],
        report: fenced("Report without any locator."),
    }));
    assert.equal(r.status, 1);
    assert.match(r.out, /missing the exact locator of E1/);
});

test("(f1) unclosed report fence (opens ```` and hits EOF) is rejected (exit 1, never closed)", () => {
    const r = run("unclosed-fence.md", packet({
        claims: [{ claim: "c1", cls: "supported", ev: "E1", notes: DS_NOTE }],
        report: "````\n- E1 — https://example.test/E1",
    }));
    assert.equal(r.status, 1);
    assert.match(r.out, /never closed/);
});

test("(f2) pseudo-closing line (```not-a-closing-fence) does not close the report (exit 1, still never closed)", () => {
    const r = run("pseudo-close.md", packet({
        claims: [{ claim: "c1", cls: "supported", ev: "E1", notes: DS_NOTE }],
        report: "````\n- E1 — https://example.test/E1\n```not-a-closing-fence",
    }));
    assert.equal(r.status, 1);
    assert.match(r.out, /never closed/);
});

test("(f3) a second fenced block after the closed report cannot smuggle the locator in (exit 1, missing locator)", () => {
    const r = run("smuggle.md", packet({
        claims: [{ claim: "c1", cls: "supported", ev: "E1", notes: DS_NOTE }],
        report: "````\nReport body without the locator.\n````\n\n````\n- E1 — https://example.test/E1\n````",
    }));
    assert.equal(r.status, 1);
    assert.match(r.out, /missing the exact locator of E1/);
});

test("(f4) inner 3-backtick code block inside the 4-backtick report is literal content (exit 0, locator found through it)", () => {
    const r = run("inner-block.md", packet({
        claims: [{ claim: "c1", cls: "supported", ev: "E1", notes: DS_NOTE }],
        report: fenced("## Findings\n\n```json\n{ \"cite\": \"https://example.test/E1\" }\n```\n\nDone."),
    }));
    assert.equal(r.status, 0, r.out);
});

test("(f5) supported via two records spanning two distinct families is accepted (exit 0, corroboration route)", () => {
    const r = run("twofam.md", packet({
        claims: [{ claim: "c1", cls: "supported", ev: "E1, E2", notes: "" }],
        families: { E1: "spec-docs", E2: "community-forum" },
        report: fenced("- E1 — https://example.test/E1\n- E2 — https://example.test/E2"),
    }));
    assert.equal(r.status, 0, r.out);
});
