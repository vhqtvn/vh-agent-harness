// Offline compaction-measurement fixture — deterministic, hermetic, no model calls.
//
// Compares four context-projection strategies against fabricated session state
// written through the REAL state-lib.js persistence primitives (so production
// behaviors — the contract-overwrite defect, deterministic truncation budgets,
// silent degradation on missing/corrupt state — are modeled exactly as they
// ship):
//
//   Control = cold original-payload (no compaction): every obligation-bearing
//             artifact the session persisted, verbatim, in write order —
//             contracts (all versions), checkpoints, decision entries,
//             session/workstream memory files, and ground-truth observations;
//             the information set the strategies compete to preserve, and the
//             cost ceiling. Structural metadata (bindings, indexes, locks,
//             run manifests) is deliberately excluded: it is scaffolding the
//             agent never holds as context, not payload.
//   B       = the CURRENT buildCompactionContext projection (baseline). B's
//             live behavior is untouched — this fixture only CALLS it.
//   H       = B + inline essentials (exact mission / user requirements /
//             must-lists / full final-response format / next action) +
//             source-bound recovery: every historical or versioned reference
//             must resolve to retained exact evidence (digest-verified) or
//             FAIL EXPLICITLY (a counted visible failure — never a silent
//             substitution of the latest version).
//   C       = slug/pointer-only comparator: minimal pointer context; content
//             is obtained by resolving typed pointers to files (retrieval,
//             counted). Pointer resolution has NO version/digest check — a
//             mutable locator silently resolves to whatever is there now.
//
// Determinism contract:
//   - All fabricated inputs are frozen constants; no clock, no RNG, no network.
//   - state-lib stamps real timestamps into persisted files; they are
//     NORMALIZED to fixed placeholders before any token counting, and no
//     assertion anchors on a timestamp. Checkpoint slugs are
//     generation-distinct (gen-1..gen-N) so the lexical latest-first ordering
//     in listSessionDocuments is stable even within one wall-clock second.
//   - Token estimator: tok(s) = Math.ceil(chars / 4) (offline approximation).
//
// Simulated resumed-agent policies (documented, deterministic — NOT model
// calls; the fixture measures what each projection MAKES AVAILABLE):
//   - First-action compliance: the exact expected-first-action string must be
//     present in the strategy's usable context (inline + retrieved +
//     re-derived).
//   - C retrieval: C resolves each declared retrieval need by reading the
//     pointed-to file in full; a dangling pointer returns nothing and emits no
//     failure signal (C has none by design).
//   - Attribution: a user-issued constraint counts as retained only if its
//     verbatim text is in the usable context; a model-forged user-styled line
//     counts as misattributed only if it appears inside a user-labeled section
//     of the usable context (section labels are tracked line-by-line).
//
// Usage:
//   node evaluate.js               # full run; exit 0 iff all self-checks pass
//   node evaluate.js --red-control # verify the checkers can detect defects
//
// Env (hermetic isolation, set by the Go test):
//   OPENCODE_STATE_ROOT, OPENCODE_RUN_ROOT,
//   OPENCODE_LOCAL_COORDINATOR_ROOT, OPENCODE_CLEARED_ASSUMPTIONS_PATH.

import fs from "fs";
import path from "path";
import { createHash } from "crypto";
import {
    StateError,
    ensureSessionBinding,
    bindSessionName,
    initSessionMemory,
    initWorkstreamMemory,
    appendDecision,
    saveTaskContract,
    saveCheckpoint,
    buildCompactionContext,
    repoRoot,
} from "./state-lib.js";

const ARGS = new Set(process.argv.slice(2));
const RED_CONTROL = ARGS.has("--red-control");
for (const arg of ARGS) {
    if (arg !== "--red-control") {
        console.error(`Unexpected argument: ${arg}`);
        process.exit(1);
    }
}

const CWD_OPT = { cwd: "/compaction-fixture" };

// ---------------------------------------------------------------------------
// Deterministic normalization + offline token estimator
// ---------------------------------------------------------------------------

const ISO_TS = /\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z/g;
const DOC_TS = /\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}/g;

function normalize(text) {
    return String(text).replace(ISO_TS, "TS-ISO").replace(DOC_TS, "TS-DOC");
}

function tok(text) {
    return Math.ceil(String(text).length / 4);
}

function sha256(text) {
    return createHash("sha256").update(String(text), "utf8").digest("hex");
}

// ---------------------------------------------------------------------------
// Evidence registry (digests only — H never archives bytes; that would be a
// conversation-archive strategy, not source-bound recovery) + control archive
// (full bytes — the no-compaction ceiling; measurement-only).
// ---------------------------------------------------------------------------

const registry = new Map(); // relPath -> {sha256, length, label, caseId, universe, version, immutable}
const versionedRegistry = new Map(); // `${relPath}@v${version}` -> {sha256}
const controlArchive = []; // ordered [{label, relPath, text, caseId, universe}]

function absOf(relPath) {
    return path.join(repoRoot(), relPath);
}

function digestOfRel(relPath) {
    const abs = absOf(relPath);
    if (!fs.existsSync(abs)) return null;
    const text = fs.readFileSync(abs, "utf8");
    return { sha256: sha256(text), length: text.length };
}

function register(relPath, meta) {
    const d = digestOfRel(relPath);
    if (!d) {
        throw new Error(`fixture bug: registered source vanished immediately: ${relPath}`);
    }
    registry.set(relPath, { ...meta, ...d });
    if (meta.version != null) {
        versionedRegistry.set(`${relPath}@v${meta.version}`, { sha256: d.sha256 });
    }
}

function archive(label, relPath, text, caseId, universe) {
    controlArchive.push({ label, relPath, text, caseId, universe });
}

// ---------------------------------------------------------------------------
// Session-state init + Control archiving
// ---------------------------------------------------------------------------

// Repo-relative state prefix (matches state-lib's stateRoot(), honoring the
// hermetic override; the fallback equals state-lib's own default).
const STATE_PREFIX = path
    .relative(
        repoRoot(),
        (process.env.OPENCODE_STATE_ROOT || "").trim() || path.join(repoRoot(), ".opencode", "state"),
    )
    .replace(/\\/g, "/");

// Content allowlist: the memory files that carry agent-held context. Structural
// siblings (artifacts.json, index/lock/manifest metadata) are excluded from
// the Control payload on purpose — see the Control declaration above.
const MEMORY_FILE_ALLOWLIST = new Set([
    "brief.md",
    "resolved-context.md",
    "open-questions.md",
    "decision-log.md",
    "next-slice.md",
    "rejected-options.md",
    "links.md",
]);

function archiveMemoryTree(w, dirRel, label) {
    const abs = absOf(dirRel);
    if (!fs.existsSync(abs)) return;
    for (const name of fs.readdirSync(abs).sort()) {
        if (!MEMORY_FILE_ALLOWLIST.has(name)) continue;
        const p = path.join(abs, name);
        if (!fs.statSync(p).isFile()) continue;
        archive(label, `${dirRel}/${name}`, fs.readFileSync(p, "utf8"), w.caseId, w.universe);
    }
}

// initSessionState runs the two memory initializers and archives what they
// persisted (in write order), so the Control payload includes the session and
// workstream memory that B's projection also draws on.
function initSessionState(w, workstream, workstreamOpts, memoryOpts) {
    initWorkstreamMemory(w.sid, workstream, { ...workstreamOpts, cwd: CWD_OPT.cwd });
    initSessionMemory(w.sid, { ...memoryOpts, cwd: CWD_OPT.cwd });
    archiveMemoryTree(w, `${STATE_PREFIX}/workstreams/${workstream}`, "workstream-memory");
    archiveMemoryTree(w, `${STATE_PREFIX}/sessions/${w.alias}/memory`, "session-memory");
}

// ---------------------------------------------------------------------------
// state-lib write wrappers (record evidence + archive control bytes)
// ---------------------------------------------------------------------------

function writeContract(sid, body, caseId, universe, gen) {
    const r = saveTaskContract(sid, body, CWD_OPT);
    register(r.path, {
        label: "task-contract",
        caseId,
        universe,
        gen,
        version: r.version,
        immutable: false,
    });
    register(r.json_path, {
        label: "task-contract-json",
        caseId,
        universe,
        gen,
        version: r.version,
        immutable: false,
    });
    archive(`contract v${r.version}`, r.path, fs.readFileSync(absOf(r.path), "utf8"), caseId, universe);
    return r;
}

function writeCheckpoint(sid, slug, body, opts, caseId, universe, gen) {
    const r = saveCheckpoint(sid, slug, body, opts.title || slug, {
        ...CWD_OPT,
        goal: opts.goal || "",
        nextStep: opts.nextStep || "",
    });
    register(r.path, { label: "checkpoint", caseId, universe, gen, immutable: true });
    archive(`checkpoint ${slug}`, r.path, fs.readFileSync(absOf(r.path), "utf8"), caseId, universe);
    return r;
}

function writeDecision(sid, body, title, caseId, universe, gen) {
    const r = appendDecision(sid, body, title, CWD_OPT);
    // The decision log is append-only: re-register the whole-file digest and
    // archive ONLY the new entry (prior entries were archived on their writes).
    register(r.path, { label: "decision-log", caseId, universe, gen, immutable: true });
    archive(`decision: ${title}`, r.path, `## ${r.created_at} - ${r.title}\n\n${body}\n`, caseId, universe);
    return r;
}

function newSession(caseId, universe, alias) {
    const sid = `cfx-${caseId}-${universe}-session`;
    ensureSessionBinding(sid, CWD_OPT);
    bindSessionName(sid, alias, CWD_OPT);
    return sid;
}

// Repo-relative paths are DERIVED from state-lib returns (never hardcoded), so
// the fixture stays correct under the hermetic env-var roots.
function memoryDirOf(contractRel) {
    return contractRel.slice(0, contractRel.lastIndexOf("/"));
}
function siblingOf(contractRel, filename) {
    return `${memoryDirOf(contractRel)}/${filename}`;
}

// ---------------------------------------------------------------------------
// Fabricated-content builders (frozen inputs)
// ---------------------------------------------------------------------------

const NEXT_CMD =
    "vh-agent-harness exec go test ./tests/integration -run TestCompactionFixture -v";

function contractMarkdown({ mission, userReqs, mustDo, mustNot, requiredOutputs, frf, requiredCommands, checklist }) {
    const lines = [];
    lines.push("## Mission", "", ...mission, "");
    lines.push("## User Requirements", "", ...userReqs, "");
    lines.push("## Must Do", "", ...mustDo.map((i) => `- ${i}`), "");
    if (mustNot.length) lines.push("## Must Not Do", "", ...mustNot.map((i) => `- ${i}`), "");
    lines.push("## Required Outputs", "", ...requiredOutputs.map((i) => `- ${i}`), "");
    lines.push("## Final Response Format", "", ...frf, "");
    lines.push("## Required Commands", "", ...requiredCommands.map((i) => `- ${i}`), "");
    lines.push("## Completion Checklist", "", ...checklist.map((i) => `- ${i}`), "");
    return lines.join("\n");
}

function checkpointBody(progressLines, extra = []) {
    return [...progressLines, ...extra].join("\n");
}

// ---------------------------------------------------------------------------
// Strategy evaluation
// ---------------------------------------------------------------------------

function anchorHits(usable, anchors) {
    return anchors.filter((a) => usable.includes(a.text));
}

// Line-scanner that tracks the current section label of a flat context so a
// model-forged user-styled line only counts as misattributed when it sits
// inside a section whose label reads as user-issued requirements/constraints.
const HEADERISH = /^(?:#{1,3} .*|[A-Z][A-Za-z _()-]{2,60}:)$/;
const USER_LABEL = /user\s+(requirements|constraints|request)/i;

function misattributions(usable, forgedLines) {
    let hits = 0;
    let inUserSection = false;
    for (const line of usable.split("\n")) {
        const trimmed = line.trim();
        if (HEADERISH.test(trimmed)) {
            inUserSection = USER_LABEL.test(trimmed);
            continue;
        }
        if (inUserSection && forgedLines.some((f) => trimmed.includes(f))) {
            hits += 1;
        }
    }
    return hits;
}

function projectB(sid, todos) {
    // B = the live projection, called exactly as production calls it.
    try {
        return { lines: buildCompactionContext(sid, todos), degraded: null };
    } catch (error) {
        if (error instanceof StateError) {
            // The documented §5.3 visible-degradation path (StateError -> the
            // plugin degrades the injection to the error message).
            return { lines: null, degraded: error.message };
        }
        throw error;
    }
}

function strategyContextB(world) {
    const b = projectB(world.sid, world.todos);
    if (b.degraded !== null) {
        const usable = `Session alias: (unbound-degraded)\n${b.degraded}`;
        return {
            usable,
            inlineTok: tok(normalize(usable)),
            retrievalTok: 0,
            rederivTok: 0,
            retrievalCalls: 0,
            rederivCalls: 0,
            visibleFailures: [`(B-degraded) ${b.degraded}`],
            notes: ["B degraded visibly via StateError (§5.3 path)"],
        };
    }
    const usable = b.lines.join("\n");
    return {
        usable,
        inlineTok: tok(normalize(usable)),
        retrievalTok: 0,
        rederivTok: 0,
        retrievalCalls: 0,
        rederivCalls: 0,
        visibleFailures: [],
        notes: [],
    };
}

function parseFrontmatterShim(md) {
    const lines = md.split("\n");
    if (lines[0] !== "---") return {};
    const close = lines.slice(1).findIndex((l) => l === "---");
    if (close === -1) return {};
    const fm = {};
    for (const line of lines.slice(1, close + 1)) {
        const i = line.indexOf(":");
        if (i === -1) continue;
        fm[line.slice(0, i).trim()] = line.slice(i + 1).trim();
    }
    return fm;
}

function parseContractSectionsForH(md) {
    // Minimal section parser for the headings this fixture writes; H reads
    // the persisted markdown directly (the live parse round-tripped it).
    const sections = { mission: "", userReqs: "", mustDo: [], mustNot: [], frf: "" };
    let current = null;
    let buf = [];
    const flush = () => {
        if (!current) return;
        const joined = buf.join("\n").trim();
        if (current === "mission") sections.mission = joined;
        else if (current === "user") sections.userReqs = joined;
        else if (current === "mustdo") sections.mustDo = buf.map((l) => l.replace(/^\s*-\s+/, "").trim()).filter(Boolean);
        else if (current === "mustnot") sections.mustNot = buf.map((l) => l.replace(/^\s*-\s+/, "").trim()).filter(Boolean);
        else if (current === "frf") sections.frf = joined;
        buf = [];
    };
    for (const line of md.split("\n")) {
        const h = line.match(/^##\s+(.+?)\s*$/);
        if (h) {
            flush();
            const label = h[1].toLowerCase();
            if (label === "mission") current = "mission";
            else if (label === "user requirements") current = "user";
            else if (label === "must do") current = "mustdo";
            else if (label === "must not do") current = "mustnot";
            else if (label === "final response format") current = "frf";
            else current = null;
            continue;
        }
        if (line.trim() === "---") { flush(); current = null; continue; }
        if (current) buf.push(line);
    }
    flush();
    return sections;
}

function strategyContextH(world) {
    const parts = [];
    const failures = [];
    const notes = [];
    let retrievalTok = 0;
    let rederivTok = 0;
    let rederivCalls = 0;

    const b = projectB(world.sid, world.todos);
    if (b.degraded !== null) {
        failures.push(`(B-degraded) ${b.degraded}`);
        parts.push(`Session alias: (unbound-degraded)\n${b.degraded}`);
    } else {
        parts.push(b.lines.join("\n"));
    }

    // ---- inline essentials (exact, from the live sources) ----
    const contractAbs = world.contractRel ? absOf(world.contractRel) : null;
    const contractLive = contractAbs && fs.existsSync(contractAbs)
        ? fs.readFileSync(contractAbs, "utf8")
        : null;
    if (contractLive !== null) {
        const reg = registry.get(world.contractRel);
        const liveDigest = sha256(contractLive);
        const verified = reg && reg.sha256 === liveDigest;
        if (!verified) {
            // Distinguish the namespace self-heal (a deleted contract is
            // re-seeded as an empty v0 default by ensureSessionMemoryNamespace
            // on the next read) from a genuine content divergence.
            const seeded = String(parseFrontmatterShim(contractLive).version || "") === "0";
            failures.push(
                seeded
                    ? `VISIBLE FAILURE: task-contract source missing (${world.contractRel}) — the ` +
                      `namespace seed re-created an EMPTY v0 default on read; the recorded v${reg ? reg.version : "?"} ` +
                      `evidence is unrecoverable and the obligation set is unavailable`
                    : `VISIBLE FAILURE: live task-contract source diverged from recorded evidence ` +
                      `(${world.contractRel}); refusing to project unverified contract content`,
            );
        } else {
            const parsed = parseContractSectionsForH(contractLive);
            parts.push(
                [
                    "## H inline essentials (exact, source-verified)",
                    `- contract: v${reg.version} ${world.contractRel} [digest-verified]`,
                    "### Mission (exact)",
                    parsed.mission,
                    "### User Requirements (exact, user-issued)",
                    parsed.userReqs,
                    "### Must Do (exact)",
                    parsed.mustDo.map((i) => `- ${i}`).join("\n"),
                    "### Must Not Do (exact)",
                    parsed.mustNot.map((i) => `- ${i}`).join("\n"),
                    "### Final Response Format (exact, full — no truncation)",
                    parsed.frf,
                ].join("\n"),
            );
        }
    } else if (world.contractExpected) {
        failures.push(
            `VISIBLE FAILURE: task-contract source missing (${world.contractRel || "unresolved"}); ` +
                `obligation set unavailable — refusing silent continuation without it`,
        );
    }

    // ---- latest checkpoint: source-bound recovery + next action ----
    if (world.latestCkptRel) {
        const ckptAbs = absOf(world.latestCkptRel);
        if (!fs.existsSync(ckptAbs)) {
            failures.push(
                `VISIBLE FAILURE: latest checkpoint source missing (${world.latestCkptRel})`,
            );
        } else {
            const text = fs.readFileSync(ckptAbs, "utf8");
            const reg = registry.get(world.latestCkptRel);
            if (!reg || reg.sha256 !== sha256(text)) {
                failures.push(
                    `VISIBLE FAILURE: latest checkpoint failed digest verification ` +
                        `(${world.latestCkptRel}); retained exact evidence unavailable — ` +
                        `presenting the failure, not unverified bytes`,
                );
            } else {
                const fm = parseFrontmatterShim(text);
                const nextStep = String(fm.next_step || "").trim();
                if (nextStep) {
                    parts.push(`## H next action (latest checkpoint, exact)\n${nextStep}`);
                }
                // Premise 4-tuples: execute the declared re-derivation command.
                for (const m of text.matchAll(/- re_derivation_command: file:(\S+) == "([^"]+)"/g)) {
                    const [, rel, expected] = m;
                    rederivCalls += 1;
                    const gtAbs = absOf(rel);
                    const observed = fs.existsSync(gtAbs)
                        ? fs.readFileSync(gtAbs, "utf8").trim()
                        : null;
                    const checkLine = `premise check: file:${rel} == "${expected}" -> observed "${observed}"`;
                    rederivTok += tok(normalize(checkLine));
                    const key = premiseKeyOf(text);
                    if (observed === null) {
                        failures.push(
                            `VISIBLE FAILURE: premise re-derivation target missing (file:${rel})`,
                        );
                    } else if (observed !== expected) {
                        parts.push(
                            `## H premise supersession\nPREMISE SUPERSEDED: ${key} is ` +
                                `${observed.toUpperCase()} (re-derived from file:${rel}; captured ` +
                                `value "${expected}" is stale)`,
                        );
                    } else {
                        parts.push(`## H premise verified\n${checkLine}`);
                    }
                }
            }
        }
    }

    // ---- historical (versioned) references: exact evidence or fail ----
    for (const href of world.historicalRefs || []) {
        const reg = registry.get(world.contractRel);
        const liveDigest = reg ? reg.sha256 : null;
        const recorded = versionedRegistry.get(`${world.contractRel}@v${href.version}`);
        // The contract path is overwrite-in-place: no immutable v<N> evidence
        // copy exists in the production model, so a versioned reference whose
        // recorded digest no longer matches the live path CANNOT be resolved.
        const immutableCopy = null;
        if (immutableCopy || (recorded && liveDigest === recorded.sha256)) {
            notes.push(`historical contract v${href.version}: digest match`);
        } else {
            failures.push(
                `VISIBLE FAILURE: historical reference contract v${href.version} ` +
                    `(cited by ${href.citedBy}) cannot be resolved to retained exact evidence — ` +
                    `the mutable path now holds v${reg ? reg.version : "?"}; REFUSING to substitute ` +
                    `the latest version for the historical one`,
            );
        }
    }

    // ---- contradiction cross-check (recovered sources only) ----
    if (world.contradiction) {
        const { key, staleClaim, currentClaim } = world.contradiction;
        const openQAbs = absOf(world.openQuestionsRel);
        const openQ = fs.existsSync(openQAbs) ? fs.readFileSync(openQAbs, "utf8") : "";
        const ckptAbs = world.latestCkptRel ? absOf(world.latestCkptRel) : null;
        const ckpt = ckptAbs && fs.existsSync(ckptAbs) ? fs.readFileSync(ckptAbs, "utf8") : "";
        const stalePresent = openQ.includes(staleClaim);
        const currentPresent = ckpt.includes(currentClaim);
        const superseded = /supersed|resolved|no longer blocked/i.test(openQ);
        if (stalePresent && currentPresent && !superseded) {
            failures.push(
                `VISIBLE FAILURE: unresolved contradiction on "${key}" — open-questions carries ` +
                    `"${staleClaim}" while the latest checkpoint carries "${currentClaim}" with no ` +
                    `supersession marker; re-verify ${key} from ground truth before proceeding`,
            );
            parts.push(
                `## H contradiction surfacing\nre-verify ${key} from ground truth before proceeding ` +
                    `(conflicting retained claims, no supersession marker)`,
            );
        }
    }

    const usable = parts.join("\n\n");
    return {
        usable,
        inlineTok: tok(normalize(usable)),
        retrievalTok,
        rederivTok,
        retrievalCalls: 0,
        rederivCalls,
        visibleFailures: failures,
        notes,
    };
}

function premiseKeyOf(checkpointText) {
    const m = checkpointText.match(/- value: (.+?) is (?:ON|OFF|on|off)\s*$/m);
    return m ? m[1].trim() : "premise";
}

function strategyContextC(world) {
    const parts = [];
    const notes = [];
    let retrievalTok = 0;
    let retrievalCalls = 0;
    parts.push(
        [
            "## C pointer context (slugs/pointers only)",
            `- alias: ${world.alias}`,
            `- workstream: ${world.workstream}`,
            `- task-contract: ${world.contractRel} (live version; no history)`,
            `- latest-checkpoint: ${world.latestCkptId || "(none)"} (${world.latestCkptSlug || "-"})`,
            `- decision-log: ${world.decisionLogRel || "(none)"}`,
            `- open-questions: ${world.openQuestionsRel || "(none)"}`,
        ].join("\n"),
    );
    for (const need of world.retrievalNeeds || []) {
        retrievalCalls += 1;
        const rel = need.rel;
        const abs = rel ? absOf(rel) : null;
        if (!abs || !fs.existsSync(abs)) {
            notes.push(`retrieval need "${need.need}": dangling pointer (${rel || "unset"}) — no signal emitted`);
            continue;
        }
        const text = fs.readFileSync(abs, "utf8");
        retrievalTok += tok(normalize(text));
        parts.push(`## C retrieval: ${need.need} -> ${rel}\n${text}`);
    }
    const usable = parts.join("\n\n");
    return {
        usable,
        inlineTok: tok(normalize(parts[0])),
        retrievalTok,
        rederivTok: 0,
        retrievalCalls,
        rederivCalls: 0,
        visibleFailures: [],
        notes,
    };
}

function strategyContextControl(world) {
    const entries = controlArchive.filter(
        (e) => e.caseId === world.caseId && e.universe === world.universe,
    );
    const usable = entries.map((e) => e.text).join("\n\n");
    return {
        usable,
        inlineTok: tok(normalize(usable)),
        retrievalTok: 0,
        rederivTok: 0,
        retrievalCalls: 0,
        rederivCalls: 0,
        visibleFailures: [],
        notes: [],
    };
}

const STRATEGY_RUNNERS = {
    Control: strategyContextControl,
    B: strategyContextB,
    H: strategyContextH,
    C: strategyContextC,
};

// ---------------------------------------------------------------------------
// Case builders. Each returns worlds[] (usually one shared world; case 3
// returns per-strategy universes because the rewrites themselves diverge).
// ---------------------------------------------------------------------------

function baseWorld(caseId, universe, family, title) {
    return {
        caseId,
        universe,
        family,
        title,
        todos: [],
        anchors: [],
        forged: [],
        firstActionAnchor: "",
        premises: [],
        contradiction: null,
        historicalRefs: [],
        retrievalNeeds: [],
        contractExpected: true,
    };
}

function todosFor(nextAction) {
    return [
        { content: `Run the next action: ${nextAction}`, status: "in_progress", priority: "high" },
        { content: "Record the run receipt in the closeout", status: "pending", priority: "medium" },
    ];
}

// --- Case 1: single compaction --------------------------------------------
function buildCase1() {
    const w = baseWorld("c1", "shared", "1", "single compaction");
    const alias = "cfx-alpha-single";
    const ws = "cfx-single-theme";
    w.sid = newSession(w.caseId, w.universe, alias);
    w.alias = alias;
    w.workstream = ws;
    initSessionState(
        w,
        ws,
        {
            briefBody: "Single-compaction continuity probe for the fixture.",
            nextSliceBody: "- Close out the case-1 measurement.",
        },
        {
            briefBody: "Goal: verify obligation retention across one compaction.",
            resolvedContextBody: "- exact: docs/planning/backlog.md",
            openQuestionsBody: "- (none)",
        },
    );
    const mission = [
        "Measure context-projection continuity for the compaction fixture.",
        "The measurement must stay offline and deterministic.",
    ];
    const userReqs = [
        "Keep every fabricated input frozen across runs.",
        "Never anchor an assertion on a timestamp.",
    ];
    const mustDo = ["Run the fixture test", "Record the metrics table"];
    const mustNot = [
        "do not call any live model during measurement",
        "do not mutate templates/core from the fixture",
    ];
    const requiredOutputs = [
        "tmp/agent-runs/compaction-architecture-evaluation/results.md",
    ];
    const frf = [
        "1. files created (exact list)",
        "2. test receipt bound to the worktree HEAD",
        "3. the results table pasted verbatim",
        "4. behavioral-closure token",
        "5. deviations from the brief",
        "6. commit SHA or the exact reason none landed",
        "7. token totals per strategy",
        "8. visible-failure counts per strategy",
    ];
    const contract = writeContract(
        w.sid,
        contractMarkdown({ mission, userReqs, mustDo, mustNot, requiredOutputs, frf, requiredCommands: [NEXT_CMD], checklist: ["fixture green", "table written"] }),
        w.caseId,
        w.universe,
        0,
    );
    w.contractRel = contract.path;
    w.decisionLogRel = siblingOf(contract.path, "decision-log.md");
    w.openQuestionsRel = siblingOf(contract.path, "open-questions.md");
    writeDecision(
        w.sid,
        "Case 1 proceeds with a single compaction generation.",
        "Case 1 shape",
        w.caseId,
        w.universe,
        0,
    );
    const ckpt = writeCheckpoint(
        w.sid,
        "gen-1",
        checkpointBody([
            "Progress: fixture scaffolding complete.",
            "Progress: control archive recording active.",
            "Progress: strategy runners wired.",
            `Next action: ${NEXT_CMD}`,
        ]),
        { title: "Gen 1", goal: "case-1 single compaction", nextStep: NEXT_CMD },
        w.caseId,
        w.universe,
        0,
    );
    w.latestCkptRel = ckpt.path;
    w.latestCkptId = ckpt.id;
    w.latestCkptSlug = ckpt.slug;
    w.todos = todosFor(NEXT_CMD);
    w.firstActionAnchor = NEXT_CMD;
    w.anchors = [
        { id: "alias", text: alias, user: false },
        { id: "workstream", text: ws, user: false },
        { id: "mission", text: mission[0], user: false },
        { id: "user-req-1", text: userReqs[0], user: true },
        { id: "must-not-1", text: mustNot[0], user: true },
        { id: "output-path", text: requiredOutputs[0], user: false },
        { id: "command", text: NEXT_CMD, user: false },
        { id: "frf-8", text: frf[7], user: false },
        { id: "next-action", text: NEXT_CMD, user: false },
    ];
    w.retrievalNeeds = [
        { need: "first-action", rel: ckpt.path },
        { need: "obligations", rel: contract.path },
    ];
    return [w];
}

// --- Case 2: 2-3 compactions, accumulating obligations (faithful rewrites) -
function buildCase2() {
    const w = baseWorld("c2", "shared", "2", "2-3 compactions, accumulating obligations");
    const alias = "cfx-beta-accum";
    const ws = "cfx-accum-theme";
    w.sid = newSession(w.caseId, w.universe, alias);
    w.alias = alias;
    w.workstream = ws;
    initSessionState(
        w,
        ws,
        {
            briefBody: "Obligation accumulation across three compactions.",
            nextSliceBody: "- Verify the fourth constraint survives the final projection.",
        },
        {
            briefBody: "Goal: obligations declared before a compaction still bind after it.",
            openQuestionsBody: "- (none)",
        },
    );
    const mission = ["Accumulate obligations across three compaction generations."];
    const userReqs = [
        "Each constraint added at generation N must still bind at generation N+2.",
    ];
    const mustDo = ["Rewrite the contract faithfully each generation"];
    const constraints = [
        "constraint-alpha always route commits through the committer agent",
        "constraint-beta never bypass the shell-guard deny list",
        "constraint-gamma keep all scratch under repo tmp",
        "constraint-delta never paste secrets on the command line",
    ];
    const frf = ["1. rows for every generation", "2. retained-constraint counts per strategy"];
    // gen 0: v1 with the first constraint; gens add the rest FAITHFULLY (the
    // rewrites are made from ground truth — no drift in this case).
    let contract = null;
    let lastCkpt = null;
    for (let gen = 0; gen < 3; gen += 1) {
        const mustNot = constraints.slice(0, gen + 2);
        contract = writeContract(
            w.sid,
            contractMarkdown({
                mission,
                userReqs,
                mustDo,
                mustNot,
                requiredOutputs: ["tmp/agent-runs/cfx/accum/report.md"],
                frf,
                requiredCommands: [NEXT_CMD],
                checklist: ["generations recorded"],
            }),
            w.caseId,
            w.universe,
            gen,
        );
        lastCkpt = writeCheckpoint(
            w.sid,
            `gen-${gen + 1}`,
            checkpointBody([
                `Progress: generation ${gen + 1} complete (faithful rewrite to v${contract.version}).`,
                `Next action: ${NEXT_CMD}`,
            ]),
            { title: `Gen ${gen + 1}`, goal: "accumulating obligations", nextStep: NEXT_CMD },
            w.caseId,
            w.universe,
            gen,
        );
    }
    w.contractRel = contract.path;
    w.decisionLogRel = siblingOf(contract.path, "decision-log.md");
    w.openQuestionsRel = siblingOf(contract.path, "open-questions.md");
    w.latestCkptRel = lastCkpt.path;
    w.latestCkptId = lastCkpt.id;
    w.latestCkptSlug = lastCkpt.slug;
    w.todos = todosFor(NEXT_CMD);
    w.firstActionAnchor = NEXT_CMD;
    w.anchors = [
        { id: "c-alpha", text: constraints[0], user: true },
        { id: "c-beta", text: constraints[1], user: true },
        { id: "c-gamma", text: constraints[2], user: true },
        { id: "c-delta", text: constraints[3], user: true },
        { id: "next-action", text: NEXT_CMD, user: false },
    ];
    w.retrievalNeeds = [
        { need: "first-action", rel: lastCkpt.path },
        { need: "obligations", rel: contract.path },
    ];
    return [w];
}

// --- Case 3: >=3 generations, progressive obligation drop (drift feedback) -
// The rewrite at each generation is made FROM THE STRATEGY'S OWN POST-
// COMPACTION CONTEXT (the recursive-drift model): an obligation the projection
// truncated away is no longer known at rewrite time and is permanently shed.
// The control universe rewrites faithfully (no compaction -> nothing sheds).
function buildCase3() {
    const worlds = [];
    const obligations = [
        "obligation-one keep the go test receipt bound to the worktree HEAD",
        "obligation-two paste the results table verbatim",
        "obligation-three record deviations from the brief",
        "obligation-four never claim unverified green",
    ];
    const addedLater = [
        "obligation-five include token totals per strategy",
        "obligation-six include visible-failure counts",
    ];
    for (const universe of ["control", "b", "h", "c"]) {
        const w = baseWorld("c3", universe, "3", ">=3 generations, progressive obligation drop");
        const alias = `cfx-gamma-${universe}`;
        const ws = `cfx-drift-${universe}`;
        w.sid = newSession(w.caseId, w.universe, alias);
        w.alias = alias;
        w.workstream = ws;
        initSessionState(
            w,
            ws,
            {
                briefBody: "Progressive-obligation-drop probe (drift feedback model).",
                nextSliceBody: "- Confirm whether obligation-four survives three compactions.",
            },
            {
                briefBody: "Goal: rewrites source from the post-compaction context, not the archive.",
                openQuestionsBody: "- (none)",
            },
        );
        const mission = ["Survive three compaction generations without shedding obligations."];
        const userReqs = ["Obligations declared before the first compaction must survive the third."];
        let known = [...obligations];
        let contract = null;
        let lastCkpt = null;
        for (let gen = 0; gen < 3; gen += 1) {
            contract = writeContract(
                w.sid,
                contractMarkdown({
                    mission,
                    userReqs,
                    mustDo: known,
                    mustNot: [],
                    requiredOutputs: ["tmp/agent-runs/cfx/drift/report.md"],
                    frf: ["1. per-generation obligation counts"],
                    requiredCommands: [NEXT_CMD],
                    checklist: ["drift recorded"],
                }),
                w.caseId,
                w.universe,
                gen,
            );
            // Wire the pointers BEFORE the simulated compaction so mid-scaffold
            // strategy evaluation (the rewrite source) sees the live contract.
            w.contractRel = contract.path;
            w.retrievalNeeds = [{ need: "rewrite-obligations", rel: contract.path }];
            lastCkpt = writeCheckpoint(
                w.sid,
                `gen-${gen + 1}`,
                checkpointBody([
                    `Progress: generation ${gen + 1} (contract v${contract.version}).`,
                    `Next action: ${NEXT_CMD}`,
                ]),
                { title: `Gen ${gen + 1}`, goal: "drift probe", nextStep: NEXT_CMD },
                w.caseId,
                w.universe,
                gen,
            );
            w.latestCkptRel = lastCkpt.path;
            w.latestCkptId = lastCkpt.id;
            w.latestCkptSlug = lastCkpt.slug;
            if (gen < 2) {
                // compaction at this generation: the strategy's usable context
                // becomes the only source for the next rewrite.
                const survived = obligationsThatSurvived(w, known);
                known = [...survived, addedLater[gen]];
            }
        }
        w.decisionLogRel = siblingOf(contract.path, "decision-log.md");
        w.openQuestionsRel = siblingOf(contract.path, "open-questions.md");
        w.todos = todosFor(NEXT_CMD);
        w.firstActionAnchor = NEXT_CMD;
        w.anchors = [
            { id: "o1", text: obligations[0], user: true },
            { id: "o2", text: obligations[1], user: true },
            { id: "o3", text: obligations[2], user: true },
            { id: "o4", text: obligations[3], user: true },
            { id: "o5", text: addedLater[0], user: true },
            { id: "o6", text: addedLater[1], user: true },
            { id: "next-action", text: NEXT_CMD, user: false },
        ];
        // Final first-action retrieval need (the rewrite need stays counted).
        w.retrievalNeeds = [
            { need: "first-action", rel: lastCkpt.path },
            { need: "rewrite-obligations", rel: contract.path },
        ];
        worlds.push(w);
    }
    return worlds;
}

function obligationsThatSurvived(world, known) {
    if (world.universe === "control") {
        return [...known]; // no compaction -> nothing sheds
    }
    const runner = { b: strategyContextB, h: strategyContextH, c: strategyContextC }[world.universe];
    const ctx = runner(world);
    return known.filter((item) => ctx.usable.includes(item));
}

// --- Case 4: premise 4-tuple + re-derivation demand ------------------------
function buildCase4() {
    const w = baseWorld("c4", "shared", "4", "premise 4-tuple + re-derivation demand");
    const alias = "cfx-delta-premise";
    const ws = "cfx-premise-theme";
    w.sid = newSession(w.caseId, w.universe, alias);
    w.alias = alias;
    w.workstream = ws;
    initSessionState(
        w,
        ws,
        {
            briefBody: "Correctness-sensitive premise freshness probe.",
            nextSliceBody: "- Act on the registry flag only after re-derivation.",
        },
        {
            briefBody: "Goal: mutable premises carry an observable re-derivation attempt.",
            openQuestionsBody: "- (none)",
        },
    );
    const mission = ["Act on the registry flag only after re-deriving its value."];
    const userReqs = ["The first resumed action must depend on the re-derived flag value."];
    const contract = writeContract(
        w.sid,
        contractMarkdown({
            mission,
            userReqs,
            mustDo: ["Re-derive the registry flag before acting"],
            mustNot: ["do not act on the captured flag value without re-derivation"],
            requiredOutputs: ["tmp/agent-runs/cfx/premise/report.md"],
            frf: ["1. re-derivation receipt", "2. action taken on the re-derived value"],
            requiredCommands: [NEXT_CMD],
            checklist: ["premise checked"],
        }),
        w.caseId,
        w.universe,
        0,
    );
    w.contractRel = contract.path;
    w.decisionLogRel = siblingOf(contract.path, "decision-log.md");
    w.openQuestionsRel = siblingOf(contract.path, "open-questions.md");
    // Ground truth (mutable external state): the flag is ON; the captured
    // premise (older) says OFF -> stale. Derived from the state root so the
    // fixture stays correct under the hermetic env roots.
    const statePrefix = contract.path.replace(/\/sessions\/.*$/, "");
    const gtRel = `${statePrefix}/groundtruth/c4-registry-flag.txt`;
    const gtAbs = absOf(gtRel);
    fs.mkdirSync(path.dirname(gtAbs), { recursive: true });
    fs.writeFileSync(gtAbs, "on\n", "utf8");
    archive("groundtruth-observation", gtRel, "groundtruth observation: registry flag is ON\n", w.caseId, w.universe);
    const premiseBlock = [
        "Load-bearing premise (4-tuple):",
        "- value: registry flag is OFF",
        "- source: kickoff operator prompt",
        `- re_derivation_command: file:${gtRel} == "off"`,
        "- observed_at: at capture time (normalized)",
    ];
    const ckpt = writeCheckpoint(
        w.sid,
        "gen-1",
        checkpointBody(
            [
                "Progress: premise captured from the kickoff prompt.",
                "Progress: ground-truth file written by the fixture.",
                "Progress: re-derivation demand recorded.",
                "Progress: checkpoint saved.",
                "Progress: session state complete for case 4.",
                `Next action: ${NEXT_CMD}`,
            ],
            premiseBlock,
        ),
        { title: "Gen 1", goal: "premise freshness", nextStep: NEXT_CMD },
        w.caseId,
        w.universe,
        0,
    );
    w.latestCkptRel = ckpt.path;
    w.latestCkptId = ckpt.id;
    w.latestCkptSlug = ckpt.slug;
    w.todos = todosFor(NEXT_CMD);
    w.firstActionAnchor = "registry flag is ON";
    w.anchors = [
        { id: "premise-value", text: "registry flag is OFF", user: false },
        { id: "rederiv-cmd", text: `file:${gtRel} == "off"`, user: false },
        { id: "next-action", text: NEXT_CMD, user: false },
    ];
    w.retrievalNeeds = [
        { need: "first-action", rel: ckpt.path },
        { need: "obligations", rel: contract.path },
    ];
    w.premises = [{ key: "registry flag", rel: gtRel, expected: "off" }];
    return [w];
}

// --- Case 5: overwritten contract (v1 pointer visibility) ------------------
function buildCase5() {
    const w = baseWorld("c5", "shared", "5", "overwritten contract: v1 pointer visibility");
    const alias = "cfx-epsilon-overwrite";
    const ws = "cfx-overwrite-theme";
    w.sid = newSession(w.caseId, w.universe, alias);
    w.alias = alias;
    w.workstream = ws;
    initSessionState(
        w,
        ws,
        {
            briefBody: "Contract-overwrite defect probe (production behavior modeled as-is).",
            nextSliceBody: "- Verify what a v1-era checkpoint reference resolves to after v2 lands.",
        },
        {
            briefBody: "Goal: historical references resolve to exact evidence or fail explicitly.",
            openQuestionsBody: "- (none)",
        },
    );
    const v1 = {
        mission: ["V1 mission: stabilize the widget parser before any refactor."],
        mustNot: ["v1-constraint never refactor the widget parser mid-release"],
        output: "tmp/agent-runs/cfx/overwrite/v1-report.md",
    };
    const v2 = {
        mission: ["V2 mission: ship the gizmo exporter with the revised checklist."],
        mustNot: ["v2-constraint never touch the gizmo exporter public API"],
        output: "tmp/agent-runs/cfx/overwrite/v2-report.md",
    };
    const contract1 = writeContract(
        w.sid,
        contractMarkdown({
            mission: v1.mission,
            userReqs: ["The v1 scope holds until the operator revises it."],
            mustDo: ["Land the v1 report"],
            mustNot: v1.mustNot,
            requiredOutputs: [v1.output],
            frf: ["1. v1 closeout fields"],
            requiredCommands: [NEXT_CMD],
            checklist: ["v1 done"],
        }),
        w.caseId,
        w.universe,
        0,
    );
    const ckpt1 = writeCheckpoint(
        w.sid,
        "gen-1",
        checkpointBody([`Progress: v1 obligations recorded (contract v${contract1.version}).`, `Next action: ${NEXT_CMD}`]),
        { title: "Gen 1", goal: "v1 era", nextStep: NEXT_CMD },
        w.caseId,
        w.universe,
        0,
    );
    const contract2 = writeContract(
        w.sid,
        contractMarkdown({
            mission: v2.mission,
            userReqs: ["The operator revised the scope; v2 supersedes v1 content."],
            mustDo: ["Land the v2 report"],
            mustNot: v2.mustNot,
            requiredOutputs: [v2.output],
            frf: ["1. v2 closeout fields"],
            requiredCommands: [NEXT_CMD],
            checklist: ["v2 done"],
        }),
        w.caseId,
        w.universe,
        1,
    );
    const ckpt2 = writeCheckpoint(
        w.sid,
        "gen-2",
        checkpointBody([`Progress: v2 obligations recorded (contract v${contract2.version}).`, `Next action: ${NEXT_CMD}`]),
        { title: "Gen 2", goal: "v2 era", nextStep: NEXT_CMD },
        w.caseId,
        w.universe,
        1,
    );
    w.contractRel = contract2.path;
    w.decisionLogRel = siblingOf(contract2.path, "decision-log.md");
    w.openQuestionsRel = siblingOf(contract2.path, "open-questions.md");
    w.latestCkptRel = ckpt2.path;
    w.latestCkptId = ckpt2.id;
    w.latestCkptSlug = ckpt2.slug;
    w.todos = todosFor(NEXT_CMD);
    w.firstActionAnchor = NEXT_CMD;
    w.anchors = [
        { id: "v1-mission", text: v1.mission[0], user: true },
        { id: "v1-constraint", text: v1.mustNot[0], user: true },
        { id: "v2-mission", text: v2.mission[0], user: true },
        { id: "v2-constraint", text: v2.mustNot[0], user: true },
    ];
    w.retrievalNeeds = [
        { need: "first-action", rel: ckpt2.path },
        { need: "obligations", rel: contract2.path },
    ];
    w.historicalRefs = [{ version: 1, citedBy: `checkpoint ${ckpt1.slug} (retained, immutable)` }];
    return [w];
}

// --- Case 6: missing (6a) / truncated (6b) source --------------------------
function buildCase6a() {
    const w = baseWorld("c6a", "shared", "6a", "missing source: contract deleted");
    const alias = "cfx-zeta-missing";
    const ws = "cfx-missing-theme";
    w.sid = newSession(w.caseId, w.universe, alias);
    w.alias = alias;
    w.workstream = ws;
    initSessionState(
        w,
        ws,
        {
            briefBody: "Missing-source degradation probe.",
            nextSliceBody: "- Verify the contract absence is signalled, not skipped.",
        },
        {
            briefBody: "Goal: missing state must signal, not silently continue (§5.3).",
            openQuestionsBody: "- (none)",
        },
    );
    const mission = ["Recover cleanly when persisted state goes missing."];
    const userReqs = ["A missing contract must produce a visible degraded signal."];
    const mustNot = ["missing-case-constraint never continue as if the contract still existed"];
    const contract = writeContract(
        w.sid,
        contractMarkdown({
            mission,
            userReqs,
            mustDo: ["Signal the missing contract"],
            mustNot,
            requiredOutputs: ["tmp/agent-runs/cfx/missing/report.md"],
            frf: ["1. degraded-signal receipt"],
            requiredCommands: [NEXT_CMD],
            checklist: ["signal observed"],
        }),
        w.caseId,
        w.universe,
        0,
    );
    const ckpt = writeCheckpoint(
        w.sid,
        "gen-1",
        checkpointBody(["Progress: state complete at gen 1.", `Next action: ${NEXT_CMD}`]),
        { title: "Gen 1", goal: "pre-deletion", nextStep: NEXT_CMD },
        w.caseId,
        w.universe,
        0,
    );
    // Post-fabrication corruption: the contract pair is deleted (the §6-7
    // missing-state shape). saveTaskContract owns these exact fixed paths in
    // production; deletion models loss of the mutable pair.
    fs.rmSync(absOf(contract.path));
    fs.rmSync(absOf(contract.json_path));
    w.contractRel = contract.path; // the pointer checkpoint frontmatter cites
    w.decisionLogRel = siblingOf(contract.path, "decision-log.md");
    w.openQuestionsRel = siblingOf(contract.path, "open-questions.md");
    w.latestCkptRel = ckpt.path;
    w.latestCkptId = ckpt.id;
    w.latestCkptSlug = ckpt.slug;
    w.todos = todosFor(NEXT_CMD);
    w.firstActionAnchor = NEXT_CMD;
    w.anchors = [
        { id: "mission", text: mission[0], user: false },
        { id: "must-not", text: mustNot[0], user: true },
        { id: "next-action", text: NEXT_CMD, user: false },
    ];
    w.retrievalNeeds = [
        { need: "first-action", rel: ckpt.path },
        { need: "obligations", rel: contract.path },
    ];
    return [w];
}

function buildCase6b() {
    const w = baseWorld("c6b", "shared", "6b", "truncated source: latest checkpoint cut mid-file");
    const alias = "cfx-eta-truncated";
    const ws = "cfx-truncated-theme";
    w.sid = newSession(w.caseId, w.universe, alias);
    w.alias = alias;
    w.workstream = ws;
    initSessionState(
        w,
        ws,
        {
            briefBody: "Truncated-source degradation probe.",
            nextSliceBody: "- Verify the truncation is detected, not partially consumed.",
        },
        {
            briefBody: "Goal: corrupt state must signal, not present partial bytes as whole.",
            openQuestionsBody: "- (none)",
        },
    );
    const mission = ["Detect truncation of the latest checkpoint before trusting it."];
    const userReqs = ["A truncated checkpoint must not be presented as complete."];
    const contract = writeContract(
        w.sid,
        contractMarkdown({
            mission,
            userReqs,
            mustDo: ["Digest-verify the latest checkpoint"],
            mustNot: [],
            requiredOutputs: ["tmp/agent-runs/cfx/truncated/report.md"],
            frf: ["1. digest-mismatch receipt"],
            requiredCommands: [NEXT_CMD],
            checklist: ["truncation flagged"],
        }),
        w.caseId,
        w.universe,
        0,
    );
    const ckpt = writeCheckpoint(
        w.sid,
        "gen-1",
        checkpointBody([
            "Progress: checkpoint written whole.",
            "Progress: fixture will truncate after registration.",
            `Next action: ${NEXT_CMD}`,
            "Tail marker: truncation-cut-marker-line-that-must-vanish",
        ]),
        { title: "Gen 1", goal: "pre-truncation", nextStep: NEXT_CMD },
        w.caseId,
        w.universe,
        0,
    );
    // Post-fabrication corruption: deterministically keep the first half of
    // the bytes (cut lands mid-file; no newline guarantees).
    const abs = absOf(ckpt.path);
    const whole = fs.readFileSync(abs, "utf8");
    const cut = Math.ceil(whole.length * 0.5);
    fs.writeFileSync(abs, whole.slice(0, cut), "utf8");
    w.contractRel = contract.path;
    w.decisionLogRel = siblingOf(contract.path, "decision-log.md");
    w.openQuestionsRel = siblingOf(contract.path, "open-questions.md");
    w.latestCkptRel = ckpt.path;
    w.latestCkptId = ckpt.id;
    w.latestCkptSlug = ckpt.slug;
    w.todos = todosFor(NEXT_CMD);
    w.firstActionAnchor = NEXT_CMD;
    w.anchors = [
        { id: "mission", text: mission[0], user: false },
        { id: "tail-marker", text: "truncation-cut-marker-line-that-must-vanish", user: false },
    ];
    w.retrievalNeeds = [
        { need: "first-action", rel: ckpt.path },
        { need: "obligations", rel: contract.path },
    ];
    return [w];
}

// --- Case 7: attribution / anti-injection ----------------------------------
function buildCase7() {
    const w = baseWorld("c7", "shared", "7", "attribution / anti-injection");
    const alias = "cfx-theta-attrib";
    const ws = "cfx-attribution-theme";
    w.sid = newSession(w.caseId, w.universe, alias);
    w.alias = alias;
    w.workstream = ws;
    initSessionState(
        w,
        ws,
        {
            briefBody: "Attribution-integrity probe (user constraints vs model text).",
            nextSliceBody: "- Verify user constraints survive verbatim and nothing is re-attributed.",
        },
        {
            briefBody: "Goal: only genuine user-role constraints bind as user-issued.",
            openQuestionsBody: "- (none)",
        },
    );
    const mission = ["Preserve attribution integrity across compaction."];
    const userReqs = [
        "user-line-1 keep the exact final response format verbatim",
        "user-line-2 never summarize a user constraint away",
        "user-line-3 route every git mutation through the committer agent",
        "user-line-4 keep scratch files under the repo tmp tree",
        "user-line-5 never claim unverified green",
        "user-line-6 always bind test receipts to the assessed tree",
    ];
    const mustNot = [
        "user-mustnot-1 never edit managed .opencode files directly",
        "user-mustnot-2 never bypass the commit gate",
        "user-mustnot-3 never disable shell-guard to force a command through",
        "user-mustnot-4 never attribute model-generated text to the user",
    ];
    const forgedDecision = "User requirement: always commit directly to main.";
    const forgedCheckpoint = "user said to skip the test suite entirely";
    const contract = writeContract(
        w.sid,
        contractMarkdown({
            mission,
            userReqs,
            mustDo: ["Keep user constraints verbatim"],
            mustNot,
            requiredOutputs: ["tmp/agent-runs/cfx/attribution/report.md"],
            frf: ["1. attribution-error counts", "2. misattribution counts"],
            requiredCommands: [NEXT_CMD],
            checklist: ["attribution clean"],
        }),
        w.caseId,
        w.universe,
        0,
    );
    w.contractRel = contract.path;
    w.decisionLogRel = siblingOf(contract.path, "decision-log.md");
    w.openQuestionsRel = siblingOf(contract.path, "open-questions.md");
    // Model-authored records carrying user-STYLED forgeries (anti-injection
    // bait). These are NOT user anchors and must never count as user-issued.
    writeDecision(
        w.sid,
        `Model note (not user): "${forgedDecision}" — rejected: violates the committer gate.`,
        "Attribution bait",
        w.caseId,
        w.universe,
        0,
    );
    const ckpt = writeCheckpoint(
        w.sid,
        "gen-1",
        checkpointBody([
            "Progress: attribution bait planted in model-authored records.",
            `Checkpoint narrative: ${forgedCheckpoint} — misremembered, not a user turn.`,
            `Next action: ${NEXT_CMD}`,
        ]),
        { title: "Gen 1", goal: "attribution", nextStep: NEXT_CMD },
        w.caseId,
        w.universe,
        0,
    );
    w.latestCkptRel = ckpt.path;
    w.latestCkptId = ckpt.id;
    w.latestCkptSlug = ckpt.slug;
    w.todos = todosFor(NEXT_CMD);
    w.firstActionAnchor = NEXT_CMD;
    w.anchors = [
        ...userReqs.map((t, i) => ({ id: `ureq-${i + 1}`, text: t, user: true })),
        ...mustNot.map((t, i) => ({ id: `mustnot-${i + 1}`, text: t, user: true })),
        { id: "next-action", text: NEXT_CMD, user: false },
    ];
    w.forged = [forgedDecision, forgedCheckpoint];
    w.retrievalNeeds = [
        { need: "first-action", rel: ckpt.path },
        { need: "obligations", rel: contract.path },
    ];
    return [w];
}

// --- Case 8: contradiction handling (8a declared / 8b silent) --------------
function buildCase8a() {
    const w = baseWorld("c8a", "shared", "8a", "contradiction, declared supersession");
    const alias = "cfx-iota-declared";
    const ws = "cfx-declared-theme";
    w.sid = newSession(w.caseId, w.universe, alias);
    w.alias = alias;
    w.workstream = ws;
    initSessionState(
        w,
        ws,
        {
            briefBody: "Declared-supersession probe.",
            nextSliceBody: "- Verify both the superseded and current decision stay visible.",
        },
        {
            briefBody: "Goal: declared contradictions surface with their supersession link.",
            openQuestionsBody: "- (none)",
        },
    );
    writeDecision(
        w.sid,
        "Adopted approach-lexicographic for the resolver (initial pick).",
        "Resolver choice",
        w.caseId,
        w.universe,
        0,
    );
    writeDecision(
        w.sid,
        "Supersede the resolver choice: adopt approach-digest-verify instead (operator feedback).",
        "Resolver supersession",
        w.caseId,
        w.universe,
        0,
    );
    const contract = writeContract(
        w.sid,
        contractMarkdown({
            mission: ["Carry declared supersessions through compaction."],
            userReqs: ["The supersession link must remain visible after compaction."],
            mustDo: ["Keep approach-digest-verify as the current resolver approach"],
            mustNot: [],
            requiredOutputs: ["tmp/agent-runs/cfx/declared/report.md"],
            frf: ["1. supersession visibility check"],
            requiredCommands: [NEXT_CMD],
            checklist: ["supersession visible"],
        }),
        w.caseId,
        w.universe,
        0,
    );
    w.contractRel = contract.path;
    w.decisionLogRel = siblingOf(contract.path, "decision-log.md");
    w.openQuestionsRel = siblingOf(contract.path, "open-questions.md");
    const ckpt = writeCheckpoint(
        w.sid,
        "gen-1",
        checkpointBody([
            "Progress: resolver supersession recorded in the decision log.",
            `Next action: ${NEXT_CMD}`,
        ]),
        { title: "Gen 1", goal: "declared contradiction", nextStep: NEXT_CMD },
        w.caseId,
        w.universe,
        0,
    );
    w.latestCkptRel = ckpt.path;
    w.latestCkptId = ckpt.id;
    w.latestCkptSlug = ckpt.slug;
    w.todos = todosFor(NEXT_CMD);
    w.firstActionAnchor = NEXT_CMD;
    w.anchors = [
        { id: "superseded", text: "approach-lexicographic", user: false },
        { id: "current", text: "approach-digest-verify", user: false },
        { id: "supersession-link", text: "Supersede the resolver choice", user: false },
        { id: "next-action", text: NEXT_CMD, user: false },
    ];
    w.retrievalNeeds = [
        { need: "first-action", rel: ckpt.path },
        { need: "decisions", rel: siblingOf(contract.path, "decision-log.md") },
    ];
    return [w];
}

function buildCase8b() {
    const w = baseWorld("c8b", "shared", "8b", "contradiction, silently unresolved");
    const alias = "cfx-kappa-silent";
    const ws = "cfx-silent-theme";
    w.sid = newSession(w.caseId, w.universe, alias);
    w.alias = alias;
    w.workstream = ws;
    initSessionState(
        w,
        ws,
        {
            briefBody: "Silent-contradiction probe.",
            nextSliceBody: "- Verify conflicting claims are not carried unreconciled.",
        },
        {
            briefBody: "Goal: conflicting retained claims must not pass through unreconciled.",
            // The stale claim lives in the REAL open-questions memory file; the
            // init archive captures it for Control, and H's cross-check reads it.
            openQuestionsBody: "- export flag: disabled (BLOCKED: waiting on the build config)",
        },
    );
    const mission = ["Surface silent contradictions instead of carrying both claims."];
    const userReqs = ["Contradictory state must be reconciled or flagged, never carried silently."];
    const contract = writeContract(
        w.sid,
        contractMarkdown({
            mission,
            userReqs,
            mustDo: ["Reconcile the export-flag claims before building"],
            mustNot: [],
            requiredOutputs: ["tmp/agent-runs/cfx/silent/report.md"],
            frf: ["1. contradiction disposition"],
            requiredCommands: [NEXT_CMD],
            checklist: ["contradiction handled"],
        }),
        w.caseId,
        w.universe,
        0,
    );
    w.contractRel = contract.path;
    w.decisionLogRel = siblingOf(contract.path, "decision-log.md");
    w.openQuestionsRel = siblingOf(contract.path, "open-questions.md");
    // Control archive sees the later ground-truth observation (chronology).
    archive(
        "groundtruth-observation",
        "groundtruth/c8b-export-flag",
        "groundtruth observation: export flag now enabled\n",
        w.caseId,
        w.universe,
    );
    const ckpt = writeCheckpoint(
        w.sid,
        "gen-1",
        checkpointBody([
            "Progress: export flag now enabled in the build config.",
            `Next action: ${NEXT_CMD}`,
        ]),
        { title: "Gen 1", goal: "flag enabled", nextStep: NEXT_CMD },
        w.caseId,
        w.universe,
        0,
    );
    w.latestCkptRel = ckpt.path;
    w.latestCkptId = ckpt.id;
    w.latestCkptSlug = ckpt.slug;
    w.todos = todosFor(NEXT_CMD);
    w.firstActionAnchor = "export flag now enabled";
    w.anchors = [
        { id: "stale-claim", text: "export flag: disabled", user: false },
        { id: "current-claim", text: "export flag now enabled", user: false },
    ];
    w.contradiction = {
        key: "export flag",
        staleClaim: "export flag: disabled",
        currentClaim: "export flag now enabled",
    };
    w.retrievalNeeds = [
        { need: "first-action", rel: ckpt.path },
        { need: "open-questions", rel: siblingOf(contract.path, "open-questions.md") },
    ];
    return [w];
}

// --- Case 9: oversized FRF vs deterministic truncation budgets -------------
function buildCase9() {
    const w = baseWorld("c9", "shared", "9", "oversized FRF vs §8.2 truncation budgets");
    const alias = "cfx-lambda-frf";
    const ws = "cfx-frf-theme";
    w.sid = newSession(w.caseId, w.universe, alias);
    w.alias = alias;
    w.workstream = ws;
    initSessionState(
        w,
        ws,
        {
            briefBody: "Oversized-FRF budget probe (12/20-line budgets).",
            nextSliceBody: "- Verify FRF lines beyond both budgets are flagged, not silently dropped.",
        },
        {
            briefBody: "Goal: a critical fact beyond the truncation budget is flagged (§6-6).",
            openQuestionsBody: "- (none)",
        },
    );
    const frf = [];
    for (let i = 1; i <= 30; i += 1) {
        frf.push(`${i}. frf-item-${i} closeout line`);
    }
    frf[4] = "5. paste the full metrics table verbatim";
    frf[15] = "16. state the exact go test command run";
    frf[24] = "25. include the HEAD SHA bound to the verification receipt";
    const contract = writeContract(
        w.sid,
        contractMarkdown({
            mission: ["Carry a 30-line final response format through compaction."],
            userReqs: ["Every FRF line beyond the truncation budgets must survive or be flagged."],
            mustDo: ["Preserve the full FRF"],
            mustNot: [],
            requiredOutputs: ["tmp/agent-runs/cfx/frf/report.md"],
            frf,
            requiredCommands: [NEXT_CMD],
            checklist: ["FRF complete"],
        }),
        w.caseId,
        w.universe,
        0,
    );
    w.contractRel = contract.path;
    w.decisionLogRel = siblingOf(contract.path, "decision-log.md");
    w.openQuestionsRel = siblingOf(contract.path, "open-questions.md");
    const ckpt = writeCheckpoint(
        w.sid,
        "gen-1",
        checkpointBody(["Progress: oversized FRF contract saved.", `Next action: ${NEXT_CMD}`]),
        { title: "Gen 1", goal: "FRF budget", nextStep: NEXT_CMD },
        w.caseId,
        w.universe,
        0,
    );
    w.latestCkptRel = ckpt.path;
    w.latestCkptId = ckpt.id;
    w.latestCkptSlug = ckpt.slug;
    w.todos = todosFor(NEXT_CMD);
    w.firstActionAnchor = NEXT_CMD;
    w.anchors = [
        { id: "frf-5", text: frf[4], user: false },
        { id: "frf-16", text: frf[15], user: false },
        { id: "frf-25", text: frf[24], user: false },
        { id: "frf-30", text: frf[29], user: false },
        { id: "next-action", text: NEXT_CMD, user: false },
    ];
    w.retrievalNeeds = [
        { need: "first-action", rel: ckpt.path },
        { need: "obligations", rel: contract.path },
    ];
    return [w];
}

// ---------------------------------------------------------------------------
// Metric assembly
// ---------------------------------------------------------------------------

function evaluate(world, strategy) {
    const ctx = STRATEGY_RUNNERS[strategy](world);
    const hits = anchorHits(ctx.usable, world.anchors);
    const userAnchors = world.anchors.filter((a) => a.user);
    const userDropped = userAnchors.filter((a) => !ctx.usable.includes(a.text));
    const misattributed = misattributions(ctx.usable, world.forged);
    return {
        case: world.family,
        title: world.title,
        strategy,
        tokenCost: {
            inline: ctx.inlineTok,
            retrieval: ctx.retrievalTok,
            rederiv: ctx.rederivTok,
        },
        anchorsHit: hits.length,
        anchorsTotal: world.anchors.length,
        anchorIdsHit: hits.map((a) => a.id),
        firstActionCompliant: ctx.usable.includes(world.firstActionAnchor),
        attributionErrors: userDropped.length + misattributed,
        misattributed,
        userConstraintsDropped: userDropped.map((a) => a.id),
        visibleFailures: ctx.visibleFailures.length,
        // Diagnostic strings are normalized (timestamps -> placeholders) so the
        // emitted metrics are byte-stable across runs; the raw strings remain
        // in the strategy context where they originated.
        visibleFailureReasons: ctx.visibleFailures.map(normalize),
        retrievalCalls: ctx.retrievalCalls,
        rederivCalls: ctx.rederivCalls,
        notes: ctx.notes.map(normalize),
    };
}

// Pinned per-case behavioral expectations. A violation fails the fixture
// (non-zero exit): these encode the defects/behaviors the fixture exists to
// demonstrate, so a silent regression in the projection budgets, silent
// degradation, or an H/C runner bug fails loudly instead of publishing
// plausible-looking numbers.
function runExpectations(rowsByCase) {
    const problems = [];
    const get = (c, s) => rowsByCase[c].find((r) => r.strategy === s);
    const expect = (cond, msg) => {
        if (!cond) problems.push(msg);
    };

    // Control sanity everywhere: the cold payload retains every anchor, every
    // first action, no attribution loss, no failures.
    for (const [c, rows] of Object.entries(rowsByCase)) {
        const ctl = rows.find((r) => r.strategy === "Control");
        expect(ctl, `internal: control row missing for ${c}`);
        if (!ctl) continue;
        expect(
            ctl.anchorsHit === ctl.anchorsTotal,
            `control lost anchors in ${c}: ${ctl.anchorsHit}/${ctl.anchorsTotal} (hit: ${ctl.anchorIdsHit.join(",")})`,
        );
        expect(ctl.firstActionCompliant, `control first-action non-compliant in ${c}`);
        expect(ctl.attributionErrors === 0, `control attribution errors in ${c}`);
        expect(ctl.visibleFailures === 0, `control visible failures in ${c}`);
    }

    // c1: B's good case — a single compaction within budgets retains everything.
    const c1b = get("1", "B");
    expect(c1b.anchorsHit === c1b.anchorsTotal, `c1: B should retain all anchors in the good case (got ${c1b.anchorsHit}/${c1b.anchorsTotal}: ${c1b.anchorIdsHit.join(",")})`);
    expect(c1b.firstActionCompliant, "c1: B first-action should comply (todo-carried next action)");
    const c1h = get("1", "H");
    expect(c1h.anchorsHit === c1h.anchorsTotal && c1h.visibleFailures === 0, "c1: H should retain all anchors with no failures");

    // c2: the 4th constraint exceeds the 3-item list budget -> B drops it silently.
    const c2b = get("2", "B");
    expect(!c2b.anchorIdsHit.includes("c-delta"), "c2: B should have dropped the 4th constraint (list budget 3)");
    expect(c2b.anchorIdsHit.includes("c-alpha") && c2b.anchorIdsHit.includes("c-gamma"), "c2: B should retain the first three constraints");
    expect(c2b.visibleFailures === 0, "c2: B truncation must be SILENT (the §6-6 defect shape)");
    const c2h = get("2", "H");
    expect(c2h.anchorsHit === c2h.anchorsTotal, `c2: H inline essentials should retain all four constraints (got ${c2h.anchorsHit}/${c2h.anchorsTotal})`);

    // c3: progressive obligation drop under B (drift feedback); H/C retain.
    const c3b = get("3", "B");
    expect(!c3b.anchorIdsHit.includes("o4"), "c3: B should progressively drop obligation-four (declared pre-compaction-1)");
    expect(c3b.anchorIdsHit.includes("o1"), "c3: B retains the first-budget obligations");
    const c3h = get("3", "H");
    expect(c3h.anchorsHit === c3h.anchorsTotal, `c3: H should retain all obligations (no drift) (got ${c3h.anchorsHit}/${c3h.anchorsTotal})`);
    const c3c = get("3", "C");
    expect(c3c.anchorsHit === c3c.anchorsTotal, `c3: C should retain all obligations via rewrite retrieval (got ${c3c.anchorsHit}/${c3c.anchorsTotal})`);
    expect(c3c.retrievalCalls >= 1, "c3: C rewrite retrieval should be counted");

    // c4: premise re-derivation — only H executes the declared command.
    const c4h = get("4", "H");
    expect(c4h.rederivCalls === 1, `c4: H should run exactly one re-derivation (got ${c4h.rederivCalls})`);
    expect(c4h.firstActionCompliant, "c4: H should act on the re-derived value");
    const c4b = get("4", "B");
    expect(c4b.rederivCalls === 0, "c4: B has no re-derivation mechanism");
    expect(!c4b.firstActionCompliant, "c4: B acts on captured/stale premise state (no re-derivation)");
    const c4c = get("4", "C");
    expect(c4c.rederivCalls === 0 && !c4c.firstActionCompliant, "c4: C pointer-only retrieval carries no freshness check");

    // c5: overwritten contract — H fails visibly; B/C silently substitute v2.
    const c5h = get("5", "H");
    expect(c5h.visibleFailures >= 1, "c5: H must count a visible failure for the unresolved v1 reference");
    expect(!c5h.anchorIdsHit.includes("v1-mission"), "c5: H must NOT silently present v2 content as v1");
    const c5b = get("5", "B");
    expect(c5b.visibleFailures === 0, "c5: B must NOT signal (silent substitution is the pinned defect)");
    expect(c5b.anchorIdsHit.includes("v2-mission") && !c5b.anchorIdsHit.includes("v1-mission"), "c5: B silently substitutes the latest contract content");
    const c5c = get("5", "C");
    expect(c5c.visibleFailures === 0 && c5c.anchorIdsHit.includes("v2-constraint"), "c5: C pointer resolves to the live (v2) file silently");

    // c6a: missing contract — B omits silently; H signals.
    const c6ab = get("6a", "B");
    expect(c6ab.visibleFailures === 0 && !c6ab.anchorIdsHit.includes("must-not"), "c6a: B must silently omit the missing contract (§6-7 shape)");
    const c6ah = get("6a", "H");
    expect(c6ah.visibleFailures >= 1, "c6a: H must signal the missing contract source");
    const c6ac = get("6a", "C");
    expect(c6ac.retrievalCalls >= 1 && c6ac.visibleFailures === 0, "c6a: C dangling pointer resolves to nothing, silently");

    // c6b: truncated checkpoint — B presents partial silently; H digest-fails.
    const c6bb = get("6b", "B");
    expect(!c6bb.anchorIdsHit.includes("tail-marker") && c6bb.visibleFailures === 0, "c6b: B must present the truncated checkpoint silently");
    const c6bh = get("6b", "H");
    expect(c6bh.visibleFailures >= 1, "c6b: H digest verification must flag the truncation");

    // c7: attribution — B drops user-line-6 + user-mustnot-4 (budgets); no
    // strategy misattributes the forged model lines into a user section.
    const c7b = get("7", "B");
    expect(c7b.userConstraintsDropped.includes("ureq-6"), `c7: B must drop user-line-6 (5-line budget) (dropped: ${c7b.userConstraintsDropped.join(",")})`);
    expect(c7b.userConstraintsDropped.includes("mustnot-4"), "c7: B must drop user-mustnot-4 (3-item budget)");
    expect(c7b.misattributed === 0, "c7: B must not misattribute the forged lines");
    const c7h = get("7", "H");
    expect(c7h.attributionErrors === 0, `c7: H retains all user constraints verbatim (dropped: ${c7h.userConstraintsDropped.join(",")})`);
    const c7c = get("7", "C");
    expect(c7c.attributionErrors === 0 && c7c.misattributed === 0, "c7: C typed-pointer retrieval keeps provenance labels");

    // c8a: declared supersession visible to everyone.
    for (const s of ["B", "H", "C"]) {
        expect(get("8a", s).anchorIdsHit.includes("supersession-link"), `c8a: ${s} should surface the declared supersession`);
    }

    // c8b: silent contradiction — B carries both unreconciled and silent; H flags.
    const c8bb = get("8b", "B");
    expect(
        c8bb.anchorIdsHit.includes("stale-claim") && c8bb.anchorIdsHit.includes("current-claim") && c8bb.visibleFailures === 0,
        "c8b: B must carry both conflicting claims with no signal (silent carry)",
    );
    const c8bh = get("8b", "H");
    expect(c8bh.visibleFailures >= 1, "c8b: H must flag the unresolved contradiction");
    expect(c8bh.firstActionCompliant, "c8b: H flags and re-verifies instead of acting on unreconciled claims");

    // c9: oversized FRF — B retains through line 20 (dedicated budget) and
    // silently drops line 25; H retains all.
    const c9b = get("9", "B");
    expect(c9b.anchorIdsHit.includes("frf-16"), "c9: B should retain FRF line 16 (dedicated 20-line budget)");
    expect(!c9b.anchorIdsHit.includes("frf-25") && !c9b.anchorIdsHit.includes("frf-30"), "c9: B must silently drop FRF lines beyond the 20-line budget");
    expect(c9b.visibleFailures === 0, "c9: B truncation must be silent (§6-6 defect shape)");
    const c9h = get("9", "H");
    expect(c9h.anchorsHit === c9h.anchorsTotal, `c9: H must retain the full oversized FRF (got ${c9h.anchorsHit}/${c9h.anchorsTotal})`);

    return problems;
}

// ---------------------------------------------------------------------------
// Report rendering
// ---------------------------------------------------------------------------

function renderTable(rows) {
    const header =
        "| Case Family | Strategy | Token Cost (inline+retrieval+re-derivation) | Exact-Anchor Retention | First-Action Compliance | Attribution Errors | Visible Failures |";
    const sep = "|---|---|---|---|---|---|---|";
    const body = rows.map((r) => {
        const cost = `${r.tokenCost.inline}+${r.tokenCost.retrieval}+${r.tokenCost.rederiv}`;
        return `| ${r.case} (${r.title}) | ${r.strategy} | ${cost} | ${r.anchorsHit}/${r.anchorsTotal} | ${r.firstActionCompliant ? "yes" : "NO"} | ${r.attributionErrors} | ${r.visibleFailures} |`;
    });
    return [header, sep, ...body].join("\n");
}

function renderNotes(rows) {
    const notes = [];
    const push = (s) => notes.push(`- ${s}`);
    push("Attribution errors = user-issued constraints no longer carried verbatim + model-forged user-styled lines presented inside a user-labeled section.");
    push("Visible failures = explicit failure signals emitted by the strategy (H's source-bound recovery ledger). A 0 for B/C on cases 5/6/8b/9 is the SILENT defect shape, not a win — the anchor-retention column shows what was silently dropped or substituted.");
    push("c5: B and C resolve the v1-era reference to the live v2 file without any signal (silent substitution); H refuses and counts a visible failure.");
    push("c6a/c6b: B omits the missing contract / presents the truncated checkpoint with no degraded signal (§6-7); H's digest verification fails visibly.");
    push("c8b: B carries both conflicting export-flag claims unreconciled and silent; H flags the unresolved contradiction and re-verifies.");
    push("c9: B's deterministic budgets retain FRF lines 1-20 (dedicated injection) and silently drop 21+ (§6-6); H carries the exact full FRF.");
    push("Token costs are offline estimates (chars/4) over timestamp-normalized text; Control is the no-compaction ceiling.");
    for (const r of rows) {
        for (const n of r.notes) push(`${r.case}/${r.strategy}: ${n}`);
    }
    return notes.join("\n");
}

// ---------------------------------------------------------------------------
// Red control — prove the checkers are not tautologies
// ---------------------------------------------------------------------------

function runRedControl() {
    const problems = [];
    // 1) Anchor-drop detection: sabotage a known-good usable context and
    //    require the anchor checker to notice. (This builds case 1 once here
    //    and again inside the full world set below; the double build is safe
    //    because session-document filenames are timestamp-prefixed — the
    //    second save simply writes a new file — and the registry re-register
    //    for the fixed contract path overwrites in place.)
    const world = buildCase1()[0];
    const ctx = strategyContextB(world);
    const victim = world.anchors[0];
    const sabotaged = ctx.usable.split(victim.text).join("REMOVED-BY-RED-CONTROL");
    if (anchorHits(sabotaged, world.anchors).length === world.anchors.length) {
        problems.push("anchor checker failed to detect an injected anchor drop (tautology)");
    }
    // 2) Expectation inversion: build the FULL row set, confirm the baseline
    //    passes, doctor one row so a pinned expectation is false, and require
    //    runExpectations to report it.
    const worlds = [
        ...buildCase1(),
        ...buildCase2(),
        ...buildCase3(),
        ...buildCase4(),
        ...buildCase5(),
        ...buildCase6a(),
        ...buildCase6b(),
        ...buildCase7(),
        ...buildCase8a(),
        ...buildCase8b(),
        ...buildCase9(),
    ];
    const rowsByCase = collectRows(worlds);
    const baseline = runExpectations(rowsByCase);
    if (baseline.length) {
        problems.push(`baseline expectations already failing (fixture bug, not red-control): ${baseline[0]}`);
    }
    const c1b = rowsByCase["1"].find((r) => r.strategy === "B");
    c1b.anchorsHit = c1b.anchorsTotal - 1;
    c1b.anchorIdsHit = [];
    const doctored = runExpectations(rowsByCase);
    if (doctored.length <= baseline.length) {
        problems.push("expectation checker passed doctored rows (tautology)");
    }
    return problems;
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

function collectRows(worlds) {
    const rowsByCase = {};
    for (const world of worlds) {
        if (world.universe === "control") {
            (rowsByCase[world.family] ||= []).push(evaluate(world, "Control"));
            continue;
        }
        if (world.universe === "shared") {
            for (const strategy of ["Control", "B", "H", "C"]) {
                (rowsByCase[world.family] ||= []).push(evaluate(world, strategy));
            }
            continue;
        }
        // per-strategy universe (case 3): the universe IS that strategy's own
        // diverged session, so its row reflects its universe.
        const map = { b: "B", h: "H", c: "C" };
        (rowsByCase[world.family] ||= []).push(evaluate(world, map[world.universe]));
    }
    return rowsByCase;
}

function main() {
    if (RED_CONTROL) {
        const problems = runRedControl();
        if (problems.length) {
            console.error("red-control: FAILED to detect injected defects:");
            for (const p of problems) console.error(`- ${p}`);
            return 1;
        }
        console.log("red-control: defect detection verified (anchor-drop detected; doctored expectations rejected)");
        return 0;
    }

    const worlds = [
        ...buildCase1(),
        ...buildCase2(),
        ...buildCase3(),
        ...buildCase4(),
        ...buildCase5(),
        ...buildCase6a(),
        ...buildCase6b(),
        ...buildCase7(),
        ...buildCase8a(),
        ...buildCase8b(),
        ...buildCase9(),
    ];

    const rowsByCase = collectRows(worlds);
    const rows = [];
    for (const family of Object.keys(rowsByCase)) {
        rows.push(...rowsByCase[family]);
    }
    // Canonical family count: 6a/6b and 8a/8b are sub-shapes of families 6/8.
    const familyCount = new Set(
        Object.keys(rowsByCase).map((k) => k.replace(/[a-z]$/, "")),
    ).size;

    const problems = runExpectations(rowsByCase);
    if (problems.length) {
        console.error("compaction-fixture: EXPECTATION FAILURES");
        for (const p of problems) console.error(`- ${p}`);
        console.error("metrics-json: " + JSON.stringify(rows));
        return 1;
    }

    const table = renderTable(rows);
    console.log("compaction-fixture: ok");
    console.log(`cases: ${familyCount}`);
    console.log(`strategy-rows: ${rows.length}`);
    console.log("strategy-checks: ok");
    console.log("metrics-json: " + JSON.stringify(rows));
    console.log("table:");
    console.log(table);
    console.log("notes:");
    console.log(renderNotes(rows));
    console.log("results-md-begin");
    console.log("# Compaction fixture — comparative results (offline, deterministic)");
    console.log("");
    console.log(table);
    console.log("");
    console.log(renderNotes(rows));
    console.log("results-md-end");
    return 0;
}

process.exitCode = main();
